package command

import (
	"fmt"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const poisonMonCooldownKey = "poison_mon"

var poisonMonStatusTags = []string{"poison", "poisoned", "MPOISN", "befuddled", "MBEFUD"}

type PoisonMonWorld interface {
	LookWorld
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
	RecordCreatureDamage(model.CreatureID, model.CreatureID, int) error
	FinalizeMonsterDeath(model.CreatureID) (bool, error)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
}

func NewPoisonMonHandler(world PoisonMonWorld) Handler {
	return NewPoisonMonHandlerWithDeathFinalizer(world, nil)
}

func NewPoisonMonHandlerWithDeathFinalizer(world PoisonMonWorld, finalizer AttackDeathFinalizer) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("poison_mon: actor creature %q not found", viewer.CreatureID)
		}

		if reject := poisonMonClassRejection(actor); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}

		target, ordinal := lookTarget(resolved)
		if target == "" {
			ctx.WriteString("\n누구를 중독시키시려고요?\n")
			return StatusDefault, nil
		}
		victim, ok := findAttackCreatureTarget(world, room, viewer, target, ordinal)
		if !ok {
			ctx.WriteString("\n그런 괴물은 존재하지 않습니다.\n")
			return StatusDefault, nil
		}

		if err := revealPoisonMonActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}
		if attackCreatureProtected(victim) {
			ctx.WriteString("\n당신은 " + poisonMonCreaturePronoun(victim) + "를 중독시킬수 없습니다.\n")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, poisonMonCooldownKey, now, poisonMonCooldownSeconds(actor)); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		actorName := attackCreatureName(actor)
		victimName := attackCreatureName(victim)
		chance := poisonMonChance(actor, victim)
		if attackRoll(1, 100) > chance {
			ctx.WriteString("\n당신은 적에게 독을 뿌렸습니다.\n그러나 적이 살짝 피해 실패했습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 적에게 독을 뿌렸습니다.\n그러나 "+victimName+krtext.Particle(victimName, '1')+" 살짝 피했습니다.\n")
		}

		if _, err := world.UpdateCreatureTags(victim.ID, poisonMonStatusTags, nil); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("\n당신은 적에게 독을 뿌렸습니다.\n" + victimName + "의 몸이 중독되었습니다.\n ")
		if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 적에게 독을 뿌렸습니다.\n"+victimName+"의 몸이 중독되었습니다.\n"); err != nil {
			return StatusDefault, err
		}

		if attackRoll(20, 100) > chance {
			return StatusDefault, nil
		}
		return poisonMonApplyImmediateDamage(ctx, world, room.ID, actor, victim, finalizer)
	}
}

func poisonMonClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	if class < legacyClassInvincible {
		return "무적이상만 쓸 수 있는 기술입니다.\n"
	}
	if !poisonMonHasAssassinTraining(actor) {
		return "아직 자객을무적수련하지 않았습니다.\n"
	}
	return ""
}

func poisonMonHasAssassinTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SASSASSIN",
		"assassinTraining",
		"assassinSpell",
		"assassinMode",
	)
}

func poisonMonCooldownSeconds(actor model.Creature) int64 {
	class := creatureClass(actor)
	switch {
	case class >= legacyClassBulsa:
		return 16
	case class == legacyClassCaretaker:
		return 18
	default:
		return 20
	}
}

func revealPoisonMonActor(ctx *Context, world PoisonMonWorld, roomID model.RoomID, viewer LookViewer, actor model.Creature) error {
	invisible := attackCreatureHasFlag(actor, "invisible", "pinvis", "PINVIS")
	if !viewer.PlayerID.IsZero() {
		if player, ok := world.Player(viewer.PlayerID); ok && hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "pinvis", "PINVIS") {
			invisible = true
		}
		if _, err := world.UpdatePlayerTags(viewer.PlayerID, nil, []string{"hidden", "phiddn", "PHIDDN", "invisible", "pinvis", "PINVIS"}); err != nil {
			return err
		}
	}
	if _, err := world.UpdateCreatureTags(actor.ID, nil, []string{"hidden", "phiddn", "PHIDDN", "invisible", "pinvis", "PINVIS"}); err != nil {
		return err
	}
	for _, key := range []string{"PHIDDN", "PINVIS"} {
		if actor.Stats[key] != 0 {
			if err := world.SetCreatureStat(actor.ID, key, 0); err != nil {
				return err
			}
		}
	}
	if !invisible {
		return nil
	}
	actorName := attackCreatureName(actor)
	ctx.WriteString("\n당신의 모습이 나타나기 시작합니다.\n")
	return roomBroadcast(ctx, roomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 모습이 보이기 시작합니다.\n")
}

func poisonMonCreaturePronoun(creature model.Creature) string {
	if attackCreatureHasFlag(creature, "male", "MMALES") {
		return "그"
	}
	return "그녀"
}

func poisonMonChance(actor model.Creature, target model.Creature) int {
	chance := (((attackCreatureLevel(actor) + 3) / 4) - ((attackCreatureLevel(target) + 3) / 4)) * 20
	chance += legacyStatBonus(creatureStat(actor, "dexterity")) * 6
	return minInt(chance, 80)
}

func poisonMonApplyImmediateDamage(
	ctx *Context,
	world PoisonMonWorld,
	roomID model.RoomID,
	actor model.Creature,
	victim model.Creature,
	finalizer AttackDeathFinalizer,
) (Status, error) {
	damage := poisonMonDamage(actor, victim)
	applied := 0
	dead := false
	if damage > 0 {
		var err error
		_, applied, dead, err = world.ApplyCreatureDamage(victim.ID, damage)
		if err != nil {
			return StatusDefault, err
		}
		if err := world.RecordCreatureDamage(victim.ID, actor.ID, applied); err != nil {
			return StatusDefault, err
		}
	}

	actorName := attackCreatureName(actor)
	victimName := attackCreatureName(victim)
	victimObject := victimName + krtext.Particle(victimName, '3')
	ctx.WriteString(fmt.Sprintf("%s 중독을 시켜서 %d의 피해를 입혔습니다.\n", victimObject, applied))
	if err := roomBroadcast(ctx, roomID, fmt.Sprintf("%s%s %s 중독을 시켜서 %d의 피해를 입혔습니다.\n", actorName, krtext.Particle(actorName, '1'), victimObject, applied)); err != nil {
		return StatusDefault, err
	}
	if !dead {
		return StatusDefault, nil
	}
	if err := finalizeMonsterDeathWithOptionalFinalizer(ctx, world, finalizer, actor, victim); err != nil {
		return StatusDefault, err
	}
	ctx.WriteString("\n당신은 " + victimName + krtext.Particle(victimName, '3') + " 죽였습니다.")
	return StatusDefault, roomBroadcast(ctx, roomID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+krtext.Particle(victimName, '3')+" 죽였습니다.")
}

func poisonMonDamage(actor model.Creature, victim model.Creature) int {
	unit := creatureStat(victim, "hpCurrent") / 40
	if unit < 1 {
		return 0
	}
	class := creatureClass(actor)
	minRoll := 5
	if class == legacyClassCaretaker {
		minRoll = 3
	}
	return attackRoll(minRoll, class) * unit
}
