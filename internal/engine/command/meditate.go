package command

import (
	"fmt"
	"math/rand"
	"time"

	"muhan/internal/world/model"
)

const (
	meditateCooldownKey            = "meditate"
	meditateSuccessCooldownSeconds = int64(700)
	meditateFailureCooldownSeconds = int64(110)
	meditateIntelligenceBonus      = 3
)

var meditateStatusTags = []string{"PMEDIT", "meditate"}

type MeditateWorld interface {
	LookWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
	SetEffectExpiration(model.CreatureID, string, int64)
	RecalculateAC(model.CreatureID) error
}

func NewMeditateHandler(world MeditateWorld, rng SearchRollFunc) Handler {
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
			return StatusDefault, fmt.Errorf("meditate: actor creature %q not found", viewer.CreatureID)
		}
		var player model.Player
		if !viewer.PlayerID.IsZero() {
			player, _ = world.Player(viewer.PlayerID)
		}

		if reject := meditateClassRejection(actor); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}
		if meditateStatusActive(player, actor) {
			ctx.WriteString("당신은 벌써 참선을 했습니다.\n")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		remaining, used, err := world.UseCreatureCooldown(actor.ID, meditateCooldownKey, now, 0)
		if err != nil {
			return StatusDefault, err
		}
		if !used {
			ctx.WriteString(renderMeditateWait(remaining))
			return StatusDefault, nil
		}

		name := attackCreatureName(actor)
		if rng(1, 100) > meditateChance(actor) {
			if err := world.SetCreatureCooldown(actor.ID, meditateCooldownKey, now, meditateFailureCooldownSeconds); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("참선도중 주화입마에 빠졌습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, name+"이 참선을 하다가 주화입마에 빠졌습니다.")
		}

		if err := meditateApplySuccess(world, player, actor); err != nil {
			return StatusDefault, err
		}
		world.SetEffectExpiration(actor.ID, "PMEDIT", now+meditateStatusDurationSeconds(actor))
		if err := world.SetCreatureCooldown(actor.ID, meditateCooldownKey, now, meditateSuccessCooldownSeconds); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("당신은 자리에 앉아 참선에 들어갑니다.\n새롭게 사물을 바라보는 눈이 뜨였습니다.\n")
		return StatusDefault, roomBroadcast(ctx, room.ID, name+"이 자리에 앉아 참선을 행합니다.")
	}
}

func meditateClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	if class != model.ClassPaladin && class != model.ClassCleric && class < model.ClassInvincible {
		return "무사 불제자만 사용할 수 있는 기술입니다.\n"
	}
	if class >= model.ClassInvincible && !meditateHasClericOrPaladinTraining(actor) {
		return "\n무사나 불제자를 무적수련하지 않았습니다..\n"
	}
	return ""
}

func meditateHasClericOrPaladinTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
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

func meditateStatusActive(player model.Player, actor model.Creature) bool {
	return statusEffectActive(player, actor, "PMEDIT", "meditate", "meditation")
}

func meditateChance(actor model.Creature) int {
	return minInt(85, ((attackCreatureLevel(actor)+3)/4)*20+legacyStatBonus(creatureStat(actor, "piety")))
}

func meditateStatusDurationSeconds(actor model.Creature) int64 {
	return int64(150 + 60*(((attackCreatureLevel(actor)+3)/4)/5))
}

func meditateApplySuccess(world MeditateWorld, player model.Player, actor model.Creature) error {
	if err := world.SetCreatureStat(actor.ID, "intelligence", creatureStat(actor, "intelligence")+meditateIntelligenceBonus); err != nil {
		return err
	}
	if _, err := world.UpdateCreatureTags(actor.ID, meditateStatusTags, nil); err != nil {
		return err
	}
	if !player.ID.IsZero() {
		if _, err := world.UpdatePlayerTags(player.ID, meditateStatusTags, nil); err != nil {
			return err
		}
	}
	return world.RecalculateAC(actor.ID)
}

func renderMeditateWait(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%d분 %02d초 기다리세요.\n", seconds/60, seconds%60)
}
