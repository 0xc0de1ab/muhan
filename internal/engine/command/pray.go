package command

import (
	"math/rand"
	"time"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	prayCooldownKey            = "pray"
	praySuccessCooldownSeconds = int64(600)
	prayFailureCooldownSeconds = int64(10)
	prayStatusDurationSeconds  = int64(500)
)

type PrayWorld interface {
	InventoryWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
	SetEffectExpiration(model.CreatureID, string, int64)
}

func NewPrayHandler(world PrayWorld, rng SearchRollFunc) Handler {
	if rng == nil {
		rng = func(min int, max int) int {
			if max <= min {
				return min
			}
			return min + rand.Intn(max-min+1)
		}
	}
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		player, creature, err := CurrentInventoryCreature(world, InventoryPlayerIDFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		if reject := prayClassRejection(creature); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}
		if prayStatusActive(player, creature) {
			ctx.WriteString("당신은 이미 신에게 빌었습니다.\n")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		remaining, used, err := world.UseCreatureCooldown(creature.ID, prayCooldownKey, now, 0)
		if err != nil {
			return StatusDefault, err
		}
		if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		if rng(1, 100) <= prayChance(creature) {
			if err := prayApplySuccess(world, player, creature, now); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("당신은 매우 신앙심이 깊어지는 것을 느낄 수 있습니다.\n")
			return StatusDefault, prayBroadcast(ctx, player, creature)
		}

		if err := world.SetCreatureCooldown(creature.ID, prayCooldownKey, now, prayFailureCooldownSeconds); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("당신의 기원에 대한 신의 응답이 없습니다.\n")
		return StatusDefault, prayBroadcast(ctx, player, creature)
	}
}

func prayClassRejection(creature model.Creature) string {
	class := creatureClass(creature)
	if class != model.ClassCleric && class != model.ClassPaladin && class < model.ClassInvincible {
		return "불제자와 무사만이 신께 기원할 수 있습니다.\n"
	}
	if class >= model.ClassInvincible && !prayHasClericOrPaladinTraining(creature) {
		return "\n불제자나 무사를 무적수련하지 않았습니다..\n"
	}
	return ""
}

func prayHasClericOrPaladinTraining(creature model.Creature) bool {
	return attackCreatureHasFlag(creature,
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

func prayStatusActive(player model.Player, creature model.Creature) bool {
	return statusEffectActive(player, creature, "PPRAYD", "prayer", "prayed", "pray")
}

func prayChance(creature model.Creature) int {
	return minInt(85, ((attackCreatureLevel(creature)+3)/4)*20+legacyStatBonus(creatureStat(creature, "piety")))
}

func prayApplySuccess(world PrayWorld, player model.Player, creature model.Creature, now int64) error {
	if err := world.SetCreatureStat(creature.ID, "piety", creatureStat(creature, "piety")+5); err != nil {
		return err
	}
	tags := []string{"PPRAYD", "pray"}
	if _, err := world.UpdateCreatureTags(creature.ID, tags, nil); err != nil {
		return err
	}
	if !player.ID.IsZero() {
		if _, err := world.UpdatePlayerTags(player.ID, tags, nil); err != nil {
			return err
		}
	}
	world.SetEffectExpiration(creature.ID, "PPRAYD", now+prayStatusDurationSeconds)
	return world.SetCreatureCooldown(creature.ID, prayCooldownKey, now, praySuccessCooldownSeconds)
}

func prayBroadcast(ctx *Context, player model.Player, creature model.Creature) error {
	roomID := player.RoomID
	if roomID.IsZero() {
		roomID = creature.RoomID
	}
	name := attackCreatureName(creature)
	return roomBroadcast(ctx, roomID, name+"이 신에게 기원합니다.")
}
