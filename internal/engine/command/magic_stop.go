package command

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	magicStopCooldownKey      = "magic_stop"
	magicStopSpellCooldownKey = "spell"
)

type MagicStopWorld interface {
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

func NewMagicStopHandler(world MagicStopWorld, rng SearchRollFunc) Handler {
	return NewMagicStopHandlerWithDeathFinalizer(world, rng, nil)
}

func NewMagicStopHandlerWithDeathFinalizer(world MagicStopWorld, rng SearchRollFunc, finalizer AttackDeathFinalizer) Handler {
	if rng == nil {
		rng = func(min int, max int) int {
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
			return StatusDefault, fmt.Errorf("magic_stop: actor creature %q not found", viewer.CreatureID)
		}

		if reject := magicStopClassRejection(actor); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}
		target, ordinal := lookTarget(resolved)
		if target == "" || attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
			ctx.WriteString("\n누구의 혈도를 봉쇄하실려구요?\n")
			return StatusDefault, nil
		}

		victim, ok := findAttackCreatureTarget(world, room, viewer, target, ordinal)
		if !ok {
			ctx.WriteString("\n그런 괴물은 존재하지 않습니다.\n")
			return StatusDefault, nil
		}
		if attackCreatureProtected(victim) {
			ctx.WriteString("\n당신은 " + turnCreaturePronoun(victim) + "의 혈도를 봉쇄할수 없습니다.\n")
			return StatusDefault, nil
		}
		if victim.Stats == nil || creatureStat(victim, "hpCurrent") <= 0 {
			ctx.WriteString("그 상대는 공격할 수 없습니다.\n")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, magicStopCooldownKey, now, magicStopCooldownSeconds(actor)); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		if err := revealMagicStopActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}

		actorName := attackCreatureName(actor)
		victimName := attackCreatureName(victim)
		chance := magicStopChance(actor, victim)
		if rng(1, 100) > chance {
			ctx.WriteString("\n당신은 적의 혈도를 재빨리 봉쇄했습니다.\n그러나 적이 살짝 피해 빗나갔습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 적의 혈도를 재빨리 봉쇄했습니다.\n그러나 "+victimName+krtext.Particle(victimName, '1')+" 살짝 피했습니다.\n")
		}

		ctx.WriteString("\n당신은 적의 혈도를 재빨리 봉쇄했습니다.\n적의 혈도를 짚어 주문을 봉쇄했습니다.\n")
		if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 적의 혈도를 재빨리 봉쇄했습니다.\n"+victimName+"의 혈도가 짚혀 주문이 봉쇄되었습니다.\n"); err != nil {
			return StatusDefault, err
		}

		dead := false
		if rng(20, 100) <= chance {
			var applied int
			victim, applied, dead, err = world.ApplyCreatureDamage(victim.ID, magicStopDamage(actor, victim, rng))
			if err != nil {
				return StatusDefault, err
			}
			if err := world.RecordCreatureDamage(victim.ID, actor.ID, applied); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(fmt.Sprintf("%s의 급소를 짚어서 %d의 피해를 입혔습니다.\n", victimName, applied))
			if err := roomBroadcast(ctx, room.ID, actorName+krtext.Particle(actorName, '1')+" "+victimName+"의 급소를 짚어서 "+fmt.Sprintf("%d의 피해를 입혔습니다.\n", applied)); err != nil {
				return StatusDefault, err
			}
		}

		if dead {
			if err := finalizeMonsterDeathWithOptionalFinalizer(ctx, world, finalizer, actor, victim); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("\n당신은 " + victimName + krtext.Particle(victimName, '3') + " 죽였습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+krtext.Particle(victimName, '3')+" 죽였습니다.\n")
		}

		if err := world.SetCreatureCooldown(victim.ID, magicStopSpellCooldownKey, now, magicStopSpellCooldownSeconds(actor, victim, rng)); err != nil {
			return StatusDefault, err
		}
		return StatusDefault, nil
	}
}

func magicStopClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	if class < model.ClassInvincible {
		return "무적이상만 쓸 수 있는 기술입니다.\n"
	}
	if !magicStopHasRangerTraining(actor) {
		return "아직 포졸을 무적수련하지 않았습니다.\n"
	}
	return ""
}

func magicStopHasRangerTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SRANGER",
		"rangerSpell",
		"rangerTraining",
		"rangerMode",
	)
}

func magicStopCooldownSeconds(actor model.Creature) int64 {
	class := creatureClass(actor)
	if class >= model.ClassBulsa {
		return 16
	}
	if class == model.ClassCaretaker {
		return 18
	}
	return 20
}

func magicStopChance(actor model.Creature, target model.Creature) int {
	chance := (((attackCreatureLevel(actor) + 3) / 4) - ((attackCreatureLevel(target) + 3) / 4)) * 20
	chance += legacyStatBonus(creatureStat(actor, "dexterity")) * 6
	return minInt(chance, 80)
}

func magicStopDamage(actor model.Creature, target model.Creature, rng SearchRollFunc) int {
	hpUnit := creatureStat(target, "hpCurrent") / 20
	class := creatureClass(actor)
	switch {
	case class >= model.ClassBulsa:
		return rng(1, class) * hpUnit
	case class == model.ClassCaretaker:
		return rng(3, class) * hpUnit
	default:
		return rng(5, class) * hpUnit
	}
}

func revealMagicStopActor(ctx *Context, world MagicStopWorld, roomID model.RoomID, viewer LookViewer, actor model.Creature) error {
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

func magicStopSpellCooldownSeconds(actor model.Creature, target model.Creature, rng SearchRollFunc) int64 {
	if magicStopTargetResistsSpellBlock(target) {
		return 5
	}
	duration := legacyStatBonus(creatureStat(actor, "dexterity"))*2 + rng(1, 6) + rng(1, 6)
	duration = maxInt(15, duration)
	duration = minInt(20, duration)
	return int64(duration)
}

func magicStopTargetResistsSpellBlock(target model.Creature) bool {
	return attackCreatureHasFlag(target,
		"PRMAGI",
		"MRMAGI",
		"resistMagic",
		"magicResistance",
		"MRBEFD",
		"resistBefuddle",
		"befuddleResistance",
	)
}
