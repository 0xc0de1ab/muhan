package command

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const (
	scratchCooldownKey  = "scratch"
	sasalCooldownKey    = "sasal"
	legacyObjectLottery = 15
	scratchGoldLimit    = 1000000000
)

type ScratchWorld interface {
	LookWorld
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
	DestroyCreatureInventoryObject(model.ObjectInstanceID, model.CreatureID) (bool, error)
	PlayerDeath(model.PlayerID, model.CreatureID) error
	BroadcastAll(string) error
	SetCreatureStat(model.CreatureID, string, int) error
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
}

type SasalWorld interface {
	LookWorld
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
	RecordCreatureDamage(model.CreatureID, model.CreatureID, int) error
	FinalizeMonsterDeath(model.CreatureID) (bool, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
}

func NewScratchHandler(world ScratchWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("scratch: actor creature %q not found", viewer.CreatureID)
		}

		if attackCreatureLevel(actor) < 20 && creatureClass(actor) < legacyClassInvincible {
			ctx.WriteString("당신의 레벨로는 복권을 긁을 수 없습니다.\n")
			return StatusDefault, nil
		}

		target, ordinal := lookTarget(resolved)
		if target == "" {
			ctx.WriteString("무엇을 긁으시려구요?\n")
			return StatusDefault, nil
		}
		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, scratchCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}
		if scratchSquareRoom(room) {
			ctx.WriteString("광장에서는 복권을 긁을 수 없습니다.\n")
			return StatusDefault, nil
		}
		if roomHasAnyFlag(room, "shoppe", "shop", "RSHOPP") {
			name := attackCreatureName(actor)
			ctx.WriteString(name + "님 밖으로 나가서 긁어 주세요.\n")
			return StatusDefault, nil
		}

		if err := revealScratchActor(world, viewer, actor); err != nil {
			return StatusDefault, err
		}

		object, objectName, ok := findEquipInventoryObjectWithVisibility(world, actor, target, ordinal, viewerDetectsInvisible(world, viewer))
		if !ok {
			ctx.WriteString("당신은 복권을 갖고 있지 않습니다.\n")
			return StatusDefault, nil
		}
		if !scratchObjectIsLottery(world, object) {
			ctx.WriteString("그것은 복권이 아닙니다.\n")
			return StatusDefault, nil
		}

		result := scratchLotteryResult(world, object)
		destroyed, err := world.DestroyCreatureInventoryObject(object.ID, actor.ID)
		if err != nil {
			return StatusDefault, err
		}
		if !destroyed {
			ctx.WriteString("당신은 복권을 갖고 있지 않습니다.\n")
			return StatusDefault, nil
		}

		actorName := attackCreatureName(actor)
		ctx.WriteString("당신은 " + objectName + krtext.Particle(objectName, '3') + " 긁었습니다.\n")
		if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+objectName+krtext.Particle(objectName, '3')+" 긁었습니다.\n"); err != nil {
			return StatusDefault, err
		}

		updatedActor, _, dead, err := world.ApplyCreatureDamage(actor.ID, result.totalPenalty)
		if err != nil {
			return StatusDefault, err
		}
		actor = updatedActor
		newExperience := creatureStat(actor, "experience") - result.totalPenalty
		if err := world.SetCreatureStat(actor.ID, "experience", newExperience); err != nil {
			return StatusDefault, err
		}

		if result.count < 1 {
			ctx.WriteString("\n꽝~~ 입니다.\n")
			if err := roomBroadcast(ctx, room.ID, "\n"+actorName+"의 "+objectName+krtext.Particle(objectName, '1')+" 꽝~~입니다.\n"); err != nil {
				return StatusDefault, err
			}
		} else {
			ctx.WriteString(fmt.Sprintf("\n당신은 %d등에 당첨되었습니다.\n", result.nDice-result.count+1))
			ctx.WriteString(fmt.Sprintf("당신은 %d냥을 받았습니다.\n", result.money))
			if err := roomBroadcast(ctx, room.ID, fmt.Sprintf("\n%s%s %s 당첨금 %d냥을 받았습니다.\n", actorName, krtext.Particle(actorName, '1'), objectName, result.money)); err != nil {
				return StatusDefault, err
			}
			if result.money >= 1000000 {
				if err := world.BroadcastAll(fmt.Sprintf("\n### 축하합니다. %s%s %s 당첨금 %d냥을 받았습니다.\n", actorName, krtext.Particle(actorName, '1'), objectName, result.money)); err != nil {
					return StatusDefault, err
				}
			}
			gold := creatureStat(actor, "gold") + result.money
			if gold > scratchGoldLimit {
				ctx.WriteString("당신의 돈이 너무 많아 10억냥만 남겨 놓고 신이 가져갑니다.\n")
				gold = scratchGoldLimit
			}
			if err := world.SetCreatureStat(actor.ID, "gold", gold); err != nil {
				return StatusDefault, err
			}
			if !viewer.PlayerID.IsZero() && (dead || newExperience < 1) {
				if err := world.PlayerDeath(viewer.PlayerID, actor.ID); err != nil {
					return StatusDefault, err
				}
				ctx.WriteString("당신은 죽으면서 몇가지 물건을 떨어뜨렸습니다.\n")
			}
		}

		return StatusDefault, world.SetCreatureCooldown(actor.ID, scratchCooldownKey, now, int64(5+scratchPow2(result.count)))
	}
}

func NewSasalHandler(world SasalWorld) Handler {
	return NewSasalHandlerWithDeathFinalizer(world, nil)
}

func NewSasalHandlerWithDeathFinalizer(world SasalWorld, finalizer AttackDeathFinalizer) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("sasal: actor creature %q not found", viewer.CreatureID)
		}

		target, ordinal := lookTarget(resolved)
		if target == "" || attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
			ctx.WriteString("누굴 공격합니까?\n")
			return StatusDefault, nil
		}
		if !sasalHasYellowTraining(actor) {
			ctx.WriteString("노랑초인 이상만 쓸수 있는 기술입니다.\n")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, sasalCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		victim, ok := findKickTarget(world, room, viewer, target, ordinal)
		if !ok {
			ctx.WriteString("그런 것은 여기 없습니다.\n")
			return StatusDefault, nil
		}
		if victim.hasPlayer {
			gate := kickPlayerCombatGate(world, room, actor, viewer.PlayerID, victim.player, victim.creature)
			if !gate.Allowed {
				ctx.WriteString(gate.Message + "\n")
				return StatusDefault, nil
			}
			gate = kickPlayerCharmGate(world, actor, viewer.PlayerID, victim.player, victim.creature, kickCharmMessageKick)
			if !gate.Allowed {
				ctx.WriteString(gate.Message + "\n")
				return StatusDefault, nil
			}
		}

		if !sasalWieldsMissile(world, actor) {
			ctx.WriteString("확인사살을 구사하시려면 궁 종류의 무기가 필요합니다.\n")
			return StatusDefault, nil
		}

		if err := revealKickActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}
		if !victim.hasPlayer {
			if attackCreatureProtected(victim.creature) {
				ctx.WriteString("당신은 그 상대를 해칠 수 없습니다.\n")
				return StatusDefault, nil
			}
			if sasalMagicOnlyDeflects(actor, victim.creature) || sasalEnchantOnlyDeflects(world, actor, victim.creature) {
				name := attackCreatureName(victim.creature)
				ctx.WriteString("당신의 공격이 " + name + "에게 아무 소용이 없는듯 합니다.\n")
				return StatusDefault, nil
			}
		}
		if !invincibleAttackTargetAlive(victim.creature) {
			ctx.WriteString("그 상대는 공격할 수 없습니다.\n")
			return StatusDefault, nil
		}

		if !victim.hasPlayer {
			if adder, ok := world.(interface {
				AddEnemy(attacker, defender model.CreatureID) (bool, error)
			}); ok {
				_, _ = adder.AddEnemy(victim.creature.ID, actor.ID)
			}
			_, _ = world.UpdateCreatureTags(victim.creature.ID, []string{"was_attacked"}, nil)
		}

		actorName := attackCreatureName(actor)
		victimName := attackCreatureName(victim.creature)
		ctx.WriteString("\n황씨 가문의 특급기술.. 여기 확인사살이 있나니\n")
		ctx.WriteString("황씨가문의 대가 황충님이시어 나에게 힘을 주소서\n")

		if attackRoll(1, 100) > sasalChance(actor, victim.creature) {
			if err := world.SetCreatureCooldown(actor.ID, sasalCooldownKey, now, sasalFailureCooldown(actor)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("당신의 확인사살이 실패했습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+"에게 확인사살로 공격하려고 합니다.\n")
		}
		if !sasalHits(actor, victim.creature) {
			if err := world.SetCreatureCooldown(actor.ID, sasalCooldownKey, now, sasalMissCooldown(actor)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("당신의 확인사살이 실패했습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+"에게 확인사살로 공격하려고 합니다.\n")
		}

		if err := world.SetCreatureCooldown(actor.ID, sasalCooldownKey, now, sasalSuccessCooldown(actor)); err != nil {
			return StatusDefault, err
		}
		perfect := creatureStat(victim.creature, "hpMax")/3 >= creatureStat(victim.creature, "hpCurrent")
		damage := sasalDamage(actor, victim.creature, perfect)
		_, applied, dead, err := world.ApplyCreatureDamage(victim.creature.ID, damage)
		if err != nil {
			return StatusDefault, err
		}
		if !victim.hasPlayer {
			if err := world.RecordCreatureDamage(victim.creature.ID, actor.ID, applied); err != nil {
				return StatusDefault, err
			}
		}

		quality := "어색한"
		if perfect {
			quality = "완벽한"
		}
		ctx.WriteString(fmt.Sprintf("\n당신은 %s 확인사살로 %s에게 %d점의 공격을 가했습니다.\n", quality, victimName, applied))
		if err := roomBroadcast(ctx, room.ID, fmt.Sprintf("\n%s%s %s에게 %s 확인사살로 %d점의 공격을 가합니다.\n", actorName, krtext.Particle(actorName, '1'), victimName, quality, applied)); err != nil {
			return StatusDefault, err
		}
		if !dead {
			return StatusDefault, nil
		}
		if !victim.hasPlayer {
			if finalizer != nil {
				if err := finalizer(ctx, actor, victim.creature); err != nil {
					return StatusDefault, err
				}
			} else if _, err := world.FinalizeMonsterDeath(victim.creature.ID); err != nil {
				return StatusDefault, err
			}
		}
		ctx.WriteString("당신은 " + victimName + krtext.Particle(victimName, '3') + " 죽였습니다.\n")
		return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+krtext.Particle(victimName, '3')+" 죽였습니다.\n")
	}
}

type scratchLotteryOutcome struct {
	nDice        int
	count        int
	money        int
	totalPenalty int
}

func scratchSquareRoom(room model.Room) bool {
	id := strings.TrimSpace(string(room.ID))
	if id == "1001" || id == "room:1001" {
		return true
	}
	if numeric, ok := strings.CutPrefix(id, "room:"); ok {
		value, err := strconv.Atoi(numeric)
		return err == nil && value == 1001
	}
	return false
}

func revealScratchActor(world ScratchWorld, viewer LookViewer, actor model.Creature) error {
	if _, err := world.UpdateCreatureTags(actor.ID, nil, []string{"hidden", "phiddn", "PHIDDN"}); err != nil {
		return err
	}
	if actor.Stats != nil && actor.Stats["PHIDDN"] != 0 {
		if err := world.SetCreatureStat(actor.ID, "PHIDDN", 0); err != nil {
			return err
		}
	}
	if viewer.PlayerID.IsZero() {
		return nil
	}
	if _, ok := world.Player(viewer.PlayerID); !ok {
		return nil
	}
	_, err := world.UpdatePlayerTags(viewer.PlayerID, nil, []string{"hidden", "phiddn", "PHIDDN"})
	return err
}

func scratchObjectIsLottery(world InventoryWorld, object model.ObjectInstance) bool {
	return objectLegacyType(world, object) == legacyObjectLottery
}

func scratchLotteryResult(world InventoryWorld, object model.ObjectInstance) scratchLotteryOutcome {
	nDice, _ := objectIntProperty(world, object, "nDice")
	if nDice < 5 {
		nDice = 5
	}
	sDice, _ := objectIntProperty(world, object, "sDice")
	if sDice < 1 {
		sDice = 1
	}
	pDice, _ := objectIntProperty(world, object, "pDice")

	count := 0
	for i := 0; i < nDice; i++ {
		if attackRoll(1, maxInt(1, i+10+pDice)) == 1 {
			count++
		}
	}

	totalPenalty := nDice
	money := 0
	if count > 0 {
		bonusPenalty := scratchPow2(count)
		totalPenalty += bonusPenalty + nDice
		money = scratchLotteryMoney(world, object, count, sDice)
	}

	return scratchLotteryOutcome{
		nDice:        nDice,
		count:        count,
		money:        money,
		totalPenalty: totalPenalty,
	}
}

func scratchLotteryMoney(world InventoryWorld, object model.ObjectInstance, count int, sDice int) int {
	money := 100
	for i := 0; i < count; i++ {
		money = multiplyCapped(money, 10, scratchGoldLimit)
	}
	money = multiplyCapped(money, maxInt(1, sDice), scratchGoldLimit)
	if value, ok := objectIntProperty(world, object, "value"); ok && money < value {
		money = value
	}
	return money
}

func scratchPow2(count int) int {
	if count < 0 {
		count = 0
	}
	value := 1
	for i := 0; i < count; i++ {
		value = multiplyCapped(value, 2, scratchGoldLimit)
	}
	return value
}

func multiplyCapped(left int, right int, capValue int) int {
	if left < 0 || right < 0 {
		return 0
	}
	if right != 0 && left > capValue/right {
		return capValue
	}
	return left * right
}

func sasalHasYellowTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"YELLOWI",
		"yellowI",
		"yellowTraining",
		"yellowSpell",
		"yellowMode",
	)
}

func sasalWieldsMissile(world InventoryWorld, actor model.Creature) bool {
	weaponID := equippedObjectID(actor, "wield")
	if weaponID.IsZero() {
		return false
	}
	weapon, ok := world.Object(weaponID)
	if !ok {
		return false
	}
	return objectLegacyType(world, weapon) == legacyObjectMissile
}

func sasalChance(actor model.Creature, victim model.Creature) int {
	chance := 50 + ((((attackCreatureLevel(actor) + 3) / 4) - ((attackCreatureLevel(victim) + 3) / 4)) * 2)
	chance += legacyStatBonus(creatureStat(actor, "intelligence")) * 2
	chance += legacyStatBonus(creatureStat(actor, "dexterity")) * 7
	chance = minInt(90, chance)
	if attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
		chance = minInt(20, chance)
	}
	return chance
}

func sasalHits(actor model.Creature, victim model.Creature) bool {
	target := creatureStat(actor, "thaco") - creatureStat(victim, "armor")/10
	return attackRoll(1, 20) >= target
}

func sasalMagicOnlyDeflects(actor model.Creature, victim model.Creature) bool {
	if !attackCreatureHasFlag(victim, "magicOnly", "mmgonl", "MMGONL") {
		return false
	}
	if creatureStat(actor, "piety") >= creatureStat(victim, "piety") {
		return false
	}
	return attackRoll(0, 1) == 1
}

func sasalEnchantOnlyDeflects(world InventoryWorld, actor model.Creature, victim model.Creature) bool {
	if !attackCreatureHasFlag(victim, "magicOrEnchantedOnly", "enchantOnly", "menonl", "MENONL") {
		return false
	}
	if attackRoll(0, 1) == 0 {
		return false
	}
	weaponID := equippedObjectID(actor, "wield")
	if weaponID.IsZero() {
		return true
	}
	weapon, ok := world.Object(weaponID)
	if !ok {
		return true
	}
	adjustment, ok := objectIntProperty(world, weapon, "adjustment")
	return !ok || adjustment < 1
}

func sasalDamage(actor model.Creature, victim model.Creature, perfect bool) int {
	hp := creatureStat(victim, "hpCurrent")
	if perfect {
		return hp
	}
	damage := statsDamage(actor)*7 + attackRoll(0, maxInt(0, creatureStat(actor, "strength")))*2
	return minInt(hp, damage)
}

func sasalSuccessCooldown(actor model.Creature) int64 {
	return int64(maxInt(1, 30-creatureStat(actor, "dexterity")/7))
}

func sasalMissCooldown(actor model.Creature) int64 {
	return int64(maxInt(1, 20-creatureStat(actor, "dexterity")/6))
}

func sasalFailureCooldown(actor model.Creature) int64 {
	return int64(maxInt(1, 20-minInt(7, creatureStat(actor, "dexterity")/3)))
}
