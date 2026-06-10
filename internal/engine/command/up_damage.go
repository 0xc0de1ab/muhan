package command

import (
	"fmt"
	"math/rand"
	"time"

	"muhan/internal/world/model"
)

const (
	upDamageCooldownKey            = "up_dmg"
	upDamageSuccessCooldownSeconds = int64(1200)
	upDamageFailureCooldownSeconds = int64(240)
	upDamageStatusDurationSeconds  = int64(120)
)

var upDamageStatusTags = []string{"PUPDMG", "upDamage", "upDmg"}

type UpDamageWorld interface {
	LookWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
	SetEffectExpiration(model.CreatureID, string, int64)
	RecalculateAC(model.CreatureID) error
}

func NewUpDamageHandler(world UpDamageWorld, rng SearchRollFunc) Handler {
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
			return StatusDefault, fmt.Errorf("up_dmg: actor creature %q not found", viewer.CreatureID)
		}
		var player model.Player
		if !viewer.PlayerID.IsZero() {
			player, _ = world.Player(viewer.PlayerID)
		}

		if reject := upDamageClassRejection(actor); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}
		if upDamageStatusActive(player, actor) {
			ctx.WriteString("당신은 지금 잠력격발을 사용중입니다.\n")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		remaining, used, err := world.UseCreatureCooldown(actor.ID, upDamageCooldownKey, now, 0)
		if err != nil {
			return StatusDefault, err
		}
		if !used {
			ctx.WriteString(renderUpDamageWait(remaining))
			return StatusDefault, nil
		}

		actorName := attackCreatureName(actor)
		if rng(1, 100) > upDamageChance(actor) {
			if err := world.SetCreatureCooldown(actor.ID, upDamageCooldownKey, now, upDamageFailureCooldownSeconds); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("힘을 격발시키는데 실패했습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, actorName+"이 잠력격발을 시도합니다.")
		}

		if err := upDamageApplySuccess(world, player, actor); err != nil {
			return StatusDefault, err
		}
		world.SetEffectExpiration(actor.ID, "PUPDMG", now+upDamageStatusDurationSeconds)
		if err := world.SetCreatureCooldown(actor.ID, upDamageCooldownKey, now, upDamageSuccessCooldownSeconds); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("당신은 자신의 혈도를 짚으며 몸의 잠력을 격발시킵니다.\n온몸으로 기가 퍼져나가는것을 느낍니다.\n")
		return StatusDefault, roomBroadcast(ctx, room.ID, actorName+"이 자신의 혈도를 짚으며 힘을 끌어들입니다.")
	}
}

func upDamageClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	level := attackCreatureLevel(actor)
	if class < model.ClassInvincible && !(class == model.ClassBarbarian && level >= 50) {
		return "권법가 레벨 50이상만 쓸수 있는 기술입니다.\n"
	}
	if class >= model.ClassInvincible && !upDamageHasBarbarianTraining(actor) {
		return "아직 권법가를 무적수련하지 않았습니다.\n"
	}
	return ""
}

func upDamageHasBarbarianTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SBARBARIAN",
		"barbarianSpell",
		"barbarianTraining",
		"barbarianMode",
	)
}

func upDamageStatusActive(player model.Player, actor model.Creature) bool {
	return statusEffectActive(player, actor, upDamageStatusTags...)
}

func upDamageChance(actor model.Creature) int {
	return minInt(85, ((attackCreatureLevel(actor)+3)/4)*5+legacyStatBonus(creatureStat(actor, "dexterity")))
}

func upDamageApplySuccess(world UpDamageWorld, player model.Player, actor model.Creature) error {
	pDiceBonus, hpBonus, mpBonus := upDamageBonuses(actor)
	hpMax := creatureStat(actor, "hpMax") + hpBonus
	mpMax := creatureStat(actor, "mpMax") + mpBonus

	for _, update := range []struct {
		key   string
		value int
	}{
		{key: "pDice", value: creatureStat(actor, "pDice") + pDiceBonus},
		{key: "hpMax", value: hpMax},
		{key: "mpMax", value: mpMax},
		{key: "hpCurrent", value: hpMax},
		{key: "mpCurrent", value: mpMax},
	} {
		if err := world.SetCreatureStat(actor.ID, update.key, update.value); err != nil {
			return err
		}
	}
	if _, err := world.UpdateCreatureTags(actor.ID, upDamageStatusTags, nil); err != nil {
		return err
	}
	if !player.ID.IsZero() {
		if _, err := world.UpdatePlayerTags(player.ID, upDamageStatusTags, nil); err != nil {
			return err
		}
	}
	return world.RecalculateAC(actor.ID)
}

func upDamageBonuses(actor model.Creature) (pDice int, hp int, mp int) {
	if creatureClass(actor) < model.ClassInvincible {
		return 2, 50, 20
	}
	return 3, 100, 100
}

func renderUpDamageWait(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%d분 %02d초 기다리세요.\n", seconds/60, seconds%60)
}
