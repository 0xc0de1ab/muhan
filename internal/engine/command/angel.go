package command

import (
	"fmt"
	"math/rand"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const (
	angelCooldownKey           = "angel"
	angelRecastCooldownSeconds = int64(500)
	angelStatusDurationSeconds = int64(300)
)

var angelStatusTags = []string{"PANGEL", "angel"}

type AngelWorld interface {
	LookWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
	SetEffectExpiration(model.CreatureID, string, int64)
}

func NewAngelHandler(world AngelWorld, rng SearchRollFunc) Handler {
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
			return StatusDefault, fmt.Errorf("angel: actor creature %q not found", viewer.CreatureID)
		}
		var player model.Player
		if !viewer.PlayerID.IsZero() {
			player, _ = world.Player(viewer.PlayerID)
		}

		if reject := angelClassRejection(actor); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}
		if angelStatusActive(player, actor) {
			ctx.WriteString("당신은 이미 정령소환술을 사용중입니다.\n")
			return StatusDefault, nil
		}
		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, angelCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderAngelWait(remaining))
			return StatusDefault, nil
		}

		actorName := attackCreatureName(actor)
		ctx.WriteString("\n천상계의 정령이여, 나의 부름에 응답하라!\n")
		if rng(1, 100) > angelChance(actor) {
			ctx.WriteString("당신은 정령을 소환하는데 실패했습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 정령소환술을 시도합니다.\n")
		}

		if err := angelApplySuccess(world, player, actor); err != nil {
			return StatusDefault, err
		}
		world.SetEffectExpiration(actor.ID, "PANGEL", now+angelStatusDurationSeconds)
		if err := world.SetCreatureCooldown(actor.ID, angelCooldownKey, now, angelRecastCooldownSeconds); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("\n당신의 부름에 정령이 응답합니다.\n")
		return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 정령을 소환합니다.\n")
	}
}

func angelClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	level := attackCreatureLevel(actor)
	if class < model.ClassInvincible && !(class == model.ClassMage && level >= 50) {
		return "도술사 50 이상만 사용할 수 있는 기술입니다.\n"
	}
	if class >= model.ClassInvincible && !angelHasMageTraining(actor) {
		return "\n도술사를 무적수련하지 않았습니다..\n"
	}
	return ""
}

func angelHasMageTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SMAGE",
		"mageSpell",
		"mageTraining",
		"mageMode",
	)
}

func angelStatusActive(player model.Player, actor model.Creature) bool {
	return statusEffectActive(player, actor, angelStatusTags...)
}

func angelApplySuccess(world AngelWorld, player model.Player, actor model.Creature) error {
	if _, err := world.UpdateCreatureTags(actor.ID, angelStatusTags, nil); err != nil {
		return err
	}
	if !player.ID.IsZero() {
		if _, err := world.UpdatePlayerTags(player.ID, angelStatusTags, nil); err != nil {
			return err
		}
	}
	return nil
}

func angelChance(actor model.Creature) int {
	return minInt(85, ((attackCreatureLevel(actor)+3)/4)*3+legacyStatBonus(creatureStat(actor, "intelligence"))) * 5
}

func renderAngelWait(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%d분 %02d초 기다리세요.\n", seconds/60, seconds%60)
}
