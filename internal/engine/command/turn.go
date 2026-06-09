package command

import (
	"fmt"
	"math/rand"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const (
	turnCooldownKey     = "turn"
	turnCooldownSeconds = int64(20)
)

type TurnWorld interface {
	LookWorld
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
	RecordCreatureDamage(model.CreatureID, model.CreatureID, int) error
	FinalizeMonsterDeath(model.CreatureID) (bool, error)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
}

func NewTurnHandler(world TurnWorld, roll SearchRollFunc) Handler {
	return NewTurnHandlerWithDeathFinalizer(world, roll, nil)
}

func NewTurnHandlerWithDeathFinalizer(world TurnWorld, roll SearchRollFunc, finalizer AttackDeathFinalizer) Handler {
	if roll == nil {
		roll = func(min int, max int) int {
			if max <= min {
				return min
			}
			return min + rand.Intn(max-min+1)
		}
	}
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("turn: actor creature %q not found", viewer.CreatureID)
		}

		target, ordinal := lookTarget(resolved)
		if target == "" {
			ctx.WriteString("\n누구에게 주문을 거실려고요?\n")
			return StatusDefault, nil
		}
		if reject := turnClassRejection(actor); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}

		victim, ok := findAttackCreatureTarget(world, room, viewer, target, ordinal)
		if !ok {
			ctx.WriteString("\n그런 괴물은 존재하지 않습니다.\n")
			return StatusDefault, nil
		}
		if creatureClass(actor) == legacyClassPaladin && !turnCreatureTurnable(victim) {
			ctx.WriteString("\n죽은 괴물에게만 사용가능합니다.\n")
			return StatusDefault, nil
		}

		if err := revealTurnActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, turnCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		if attackCreatureProtected(victim) {
			ctx.WriteString("\n당신은 " + turnCreaturePronoun(victim) + "의 혼을 소멸시킬 수 없습니다.\n")
			return StatusDefault, nil
		}
		if victim.Stats == nil || creatureStat(victim, "hpCurrent") <= 0 {
			ctx.WriteString("그 괴물에게는 아무 소용이 없습니다.\n")
			return StatusDefault, nil
		}

		if adder, ok := world.(interface {
			AddEnemy(attacker, defender model.CreatureID) (bool, error)
		}); ok {
			_, _ = adder.AddEnemy(victim.ID, actor.ID)
		}
		_, _ = world.UpdateCreatureTags(victim.ID, []string{"was_attacked"}, nil)
		if err := world.SetCreatureCooldown(actor.ID, turnCooldownKey, now, turnCooldownSeconds); err != nil {
			return StatusDefault, err
		}

		actorName := attackCreatureName(actor)
		victimName := attackCreatureName(victim)
		chance := turnChance(actor, victim)
		if roll(1, 100) > chance {
			ctx.WriteString("\n부적을 하늘로 날리며 혼을 소환하는 방혼술의 주문을 외쳤습니다.하지만 주문이 튕겨져 나오면서 " + victimName + krtext.Particle(victimName, '1') + " 당신의 주술을 견뎌냈습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 부적을 하늘로 날리며 혼을 소환시키는 방혼술의 주문을 외칩니다.\n하지만 주문이 튕겨져 나오면서 "+victimName+krtext.Particle(victimName, '1')+" 주술을 견뎌냈습니다.\n")
		}
		if turnInstantDisintegrates(actor, victim, roll) {
			_, applied, _, err := world.ApplyCreatureDamage(victim.ID, creatureStat(victim, "hpCurrent"))
			if err != nil {
				return StatusDefault, err
			}
			if err := world.RecordCreatureDamage(victim.ID, actor.ID, applied); err != nil {
				return StatusDefault, err
			}
			if err := finalizeMonsterDeathWithOptionalFinalizer(ctx, world, finalizer, actor, victim); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("\n부적을 하늘로 날리며 혼을 소환시키는 방혼술의 주문을 외칩니다.\n부적이 빙글빙글 돌면서 소용돌이를 일으키자 그 자리에서 저승사자가\n올라와 " + victimName + "의 혼을 소멸시켜 버렸습니다.\n ")
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 부적을 하늘로 날리며 \n혼을 소환시키는 방혼술의 주문을 외칩니다.\n부적이 빙글빙글 돌면서 소용돌이를 일으키자 그 자리에서 \n저승사자가 올라와 "+victimName+"의 혼을소멸시켜 버렸습니다.\n")
		}

		damage := turnDamage(actor, victim, roll)
		_, applied, dead, err := world.ApplyCreatureDamage(victim.ID, damage)
		if err != nil {
			return StatusDefault, err
		}
		if err := world.RecordCreatureDamage(victim.ID, actor.ID, applied); err != nil {
			return StatusDefault, err
		}

		ctx.WriteString(fmt.Sprintf("\n부적을 하늘로 날리며 혼을 소환시키는 방혼술의 주문을 외칩니다.\n부적이 %s의 몸을 공격하며 %d만큼의 타격을 입혔습니다.\n", victimName, applied))
		if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 부적을 하늘로 날리며 혼을 소환시키는 방혼술의 주문을 외칩니다.\n부적이 "+victimName+"의 몸을 공격하며 타격을 입혔습니다.\n"); err != nil {
			return StatusDefault, err
		}

		if !dead {
			return StatusDefault, nil
		}
		if err := finalizeMonsterDeathWithOptionalFinalizer(ctx, world, finalizer, actor, victim); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("\n당신은 " + victimName + "의 혼을 소멸시켰습니다.\n" + victimName + "의 몸에서 혼이 사라지자 녹아버립니다.\n")
		return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+"의 혼을 소멸시켰습니다.\n\n"+victimName+"의 혼이 사라지자 몸이 녹아버립니다.\n")
	}
}

func turnClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	if class != legacyClassCleric && class != legacyClassPaladin && class < legacyClassInvincible {
		return "불제자와 무사만이 방혼술을 사용할 수 있습니다.\n"
	}
	if class >= legacyClassInvincible && !turnHasClericOrPaladinTraining(actor) {
		return "\n불제자나 무사를 무적수련하지 않았습니다.\n"
	}
	return ""
}

func turnHasClericOrPaladinTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SCLERIC",
		"SPALADIN",
		"clericTraining",
		"paladinTraining",
		"clericSpell",
		"paladinSpell",
		"clericMode",
		"paladinMode",
	) || turnCreatureStatEnabled(actor,
		"SCLERIC",
		"SPALADIN",
		"clericTraining",
		"paladinTraining",
		"clericSpell",
		"paladinSpell",
		"clericMode",
		"paladinMode",
	)
}

func turnCreatureTurnable(creature model.Creature) bool {
	return attackCreatureHasFlag(creature, "undead", "turnable", "MUNDED") ||
		turnCreatureStatEnabled(creature, "undead", "turnable", "MUNDED")
}

func turnCreatureStatEnabled(creature model.Creature, names ...string) bool {
	value, ok := attackCreatureIntValue(creature, names...)
	return ok && value != 0
}

func turnCreaturePronoun(creature model.Creature) string {
	if attackCreatureHasFlag(creature, "male", "MMALES") {
		return "그"
	}
	return "그녀"
}

func turnChance(actor model.Creature, target model.Creature) int {
	chance := (((attackCreatureLevel(actor) + 3) / 4) - ((attackCreatureLevel(target) + 3) / 4)) * 20
	chance += legacyStatBonus(creatureStat(actor, "piety")) * 5
	if creatureClass(actor) == legacyClassPaladin {
		chance += 25
	} else {
		chance += 15
	}
	return minInt(chance, 80)
}

func turnInstantDisintegrates(actor model.Creature, target model.Creature, rng SearchRollFunc) bool {
	return turnCreatureTurnable(target) && rng(1, 100) > 90-legacyStatBonus(creatureStat(actor, "piety"))
}

func turnDamage(actor model.Creature, target model.Creature, rng SearchRollFunc) int {
	hpCurrent := creatureStat(target, "hpCurrent")
	hpUnit := hpCurrent / 30
	class := creatureClass(actor)
	var damage int
	switch class {
	case legacyClassCaretaker:
		damage = rng(3, class) * hpUnit
	case legacyClassInvincible:
		damage = rng(5, class) * hpUnit
	default:
		damage = hpCurrent / 3
	}
	if damage < 1 {
		return 1
	}
	return damage
}

func revealTurnActor(ctx *Context, world TurnWorld, roomID model.RoomID, viewer LookViewer, actor model.Creature) error {
	invisible := attackCreatureHasFlag(actor, "invisible", "pinvis", "PINVIS")
	if !viewer.PlayerID.IsZero() {
		if player, ok := world.Player(viewer.PlayerID); ok && hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "pinvis", "PINVIS") {
			invisible = true
		}
		if _, err := world.UpdatePlayerTags(viewer.PlayerID, nil, []string{"invisible", "pinvis", "PINVIS"}); err != nil {
			return err
		}
	}
	if _, err := world.UpdateCreatureTags(actor.ID, nil, []string{"invisible", "pinvis", "PINVIS"}); err != nil {
		return err
	}
	if actor.Stats["PINVIS"] != 0 {
		if err := world.SetCreatureStat(actor.ID, "PINVIS", 0); err != nil {
			return err
		}
	}
	if !invisible {
		return nil
	}
	actorName := attackCreatureName(actor)
	ctx.WriteString("\n당신의 모습이 나타나기 시작합니다.\n")
	return roomBroadcast(ctx, roomID, actorName+"의 모습이 보이기 시작합니다.")
}
