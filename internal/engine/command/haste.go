package command

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	hasteCooldownKey        = "haste"
	hasteCooldownSeconds    = int64(600)
	hasteFailureWaitSeconds = int64(10)
	hasteDexterityBonus     = 15
)

var hasteStatusTags = []string{"haste", "PHASTE"}

type HasteWorld interface {
	LookWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
	SetEffectExpiration(model.CreatureID, string, int64)
	RecalculateAC(model.CreatureID) error
}

type HasteRollFunc func(min int, max int) int

func NewHasteHandler(world HasteWorld, rng HasteRollFunc) Handler {
	if rng == nil {
		rng = func(min int, max int) int {
			if max <= min {
				return min
			}
			return min + rand.Intn(max-min+1)
		}
	}
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("haste: actor creature %q not found", viewer.CreatureID)
		}

		if message := hasteClassRejection(actor); message != "" {
			ctx.WriteString(message)
			return StatusDefault, nil
		}
		if hasteActorHasStatus(world, viewer, actor) {
			ctx.WriteString("당신은 지금 활보법을 사용중입니다.\n")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, hasteCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		actorName := attackCreatureName(actor)
		if rng(1, 100) > hasteChance(actor) {
			if err := world.SetCreatureCooldown(actor.ID, hasteCooldownKey, now, hasteFailureWaitSeconds); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("활보법이 실패하였습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, actorName+"이 활보법을 써봅니다.")
		}

		if err := world.SetCreatureStat(actor.ID, "dexterity", creatureStat(actor, "dexterity")+hasteDexterityBonus); err != nil {
			return StatusDefault, err
		}
		if _, err := world.UpdateCreatureTags(actor.ID, hasteStatusTags, nil); err != nil {
			return StatusDefault, err
		}
		if !viewer.PlayerID.IsZero() {
			if _, err := world.UpdatePlayerTags(viewer.PlayerID, hasteStatusTags, nil); err != nil {
				return StatusDefault, err
			}
		}
		if err := world.RecalculateAC(actor.ID); err != nil {
			return StatusDefault, err
		}
		world.SetEffectExpiration(actor.ID, "PHASTE", now+hasteStatusDurationSeconds(actor))
		if err := world.SetCreatureCooldown(actor.ID, hasteCooldownKey, now, hasteCooldownSeconds); err != nil {
			return StatusDefault, err
		}

		ctx.WriteString("당신의 동작이 좀더 민첩해진것 같습니다.\n")
		return StatusDefault, roomBroadcast(ctx, room.ID, actorName+"이 활보법을 사용하였습니다.")
	}
}

func hasteClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	if class != model.ClassRanger && class < model.ClassInvincible {
		return "포졸만 사용할 수 있는 기술입니다.\n"
	}
	if class >= model.ClassInvincible && !hasteHasRangerTraining(actor) {
		return "\n포졸을 무적수련하지 않았습니다..\n"
	}
	return ""
}

func hasteHasRangerTraining(actor model.Creature) bool {
	return creatureHasAnyFlag(actor, "SRANGER", "rangerSpell", "rangerTraining", "rangerMode") ||
		hasteCreatureStatEnabled(actor, "SRANGER", "rangerSpell", "rangerTraining", "rangerMode")
}

func hasteActorHasStatus(world HasteWorld, viewer LookViewer, actor model.Creature) bool {
	if creatureHasAnyFlag(actor, "haste", "hasted", "PHASTE") ||
		hasteCreatureStatEnabled(actor, "haste", "hasted", "PHASTE") {
		return true
	}
	if viewer.PlayerID.IsZero() {
		return false
	}
	player, ok := world.Player(viewer.PlayerID)
	return ok && hasAnyNormalizedFlag(player.Metadata.Tags, "haste", "hasted", "PHASTE")
}

func hasteCreatureStatEnabled(actor model.Creature, names ...string) bool {
	value, ok := attackCreatureIntValue(actor, names...)
	return ok && value != 0
}

func hasteChance(actor model.Creature) int {
	return minInt(85, ((creatureLevel(actor)+3)/4)*20+legacyStatBonus(creatureStat(actor, "dexterity")))
}

func hasteStatusDurationSeconds(actor model.Creature) int64 {
	return int64(120 + 60*(((creatureLevel(actor)+3)/4)/5))
}
