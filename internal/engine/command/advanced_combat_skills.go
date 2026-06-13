package command

import (
	"fmt"
	"time"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	bnahanCooldownKey = "bnahan"
	taguCooldownKey   = "tagu"
)

type AdvancedCombatSkillWorld interface {
	BashWorld
	FinalizeMonsterDeath(model.CreatureID) (bool, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
}

func NewBnahanHandler(world AdvancedCombatSkillWorld) Handler {
	return NewBnahanHandlerWithDeathFinalizer(world, nil)
}

func NewBnahanHandlerWithDeathFinalizer(world AdvancedCombatSkillWorld, finalizer AttackDeathFinalizer) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("bnahan: actor creature %q not found", viewer.CreatureID)
		}

		if message := bnahanClassRejection(actor); message != "" {
			ctx.WriteString(message)
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, bnahanCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		targets := bnahanTargets(world, room, viewer)
		if len(targets) == 0 {
			ctx.WriteString("이 방에는 당신이 공격할 적이 없습니다.\n")
			return StatusDefault, nil
		}
		advancedCombatPrimeMonsterTargets(world, targets, actor.ID)
		if err := revealKickActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}

		actorName := attackCreatureName(actor)
		ctx.WriteString("\n\"변수나한권~~!! 이 앞의 녀석들.. 모두 날려버리고 말겠다.\"\n")
		ctx.WriteString("당신은 변수나한권을 펼쳐 주위의 적을 향해 돌진합니다.\n")

		chance := bnahanChance(actor, targets)
		if attackRoll(1, 100) > chance {
			if err := lionScreamApplyFatigue(world, actor); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("당신은 변수나한권의 묘리를 살리지 못했습니다.\n")
			ctx.WriteString("당신은 약간 피로해짐을 느낍니다.\n")
			if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '0')+" 변수나한권을 제대로 펼치지 못합니다.\n"); err != nil {
				return StatusDefault, err
			}
			if err := world.SetCreatureCooldown(actor.ID, bnahanCooldownKey, now, bnahanCooldownSeconds(actor)); err != nil {
				return StatusDefault, err
			}
			return StatusDefault, nil
		}

		count := minInt((chance+1)/3, len(targets))
		for _, victim := range targets[:count] {
			victimName := attackCreatureName(victim)
			damage := bnahanDamage(actor)
			if bnahanMagicOnlyGlances(actor, victim) {
				ctx.WriteString("당신의 변수나한권이 " + victimName + "에게 아무런 상처도 내지 못합니다.\n")
				damage = 1
			}
			_, applied, dead, err := world.ApplyCreatureDamage(victim.ID, damage)
			if err != nil {
				return StatusDefault, err
			}
			if err := world.RecordCreatureDamage(victim.ID, actor.ID, applied); err != nil {
				return StatusDefault, err
			}

			ctx.WriteString(fmt.Sprintf("당신은 변수나한권으로 %s에게 %d점의 피해를 입혔습니다.\n", victimName, applied))
			if err := roomBroadcast(ctx, room.ID, fmt.Sprintf("\n%s%s 변수나한권으로 %s에게 %d점의 피해를 입혔습니다.\n", actorName, krtext.Particle(actorName, '1'), victimName, applied)); err != nil {
				return StatusDefault, err
			}
			if !dead {
				continue
			}
			if err := finalizeMonsterDeathWithOptionalFinalizer(ctx, world, finalizer, actor, victim); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("\n당신은 뛰어난 변수나한권으로 " + victimName + krtext.Particle(victimName, '3') + " 죽였습니다.\n")
			if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 변수나한권으로 "+victimName+krtext.Particle(victimName, '3')+" 죽였습니다.\n"); err != nil {
				return StatusDefault, err
			}
		}
		if err := world.SetCreatureCooldown(actor.ID, bnahanCooldownKey, now, bnahanCooldownSeconds(actor)); err != nil {
			return StatusDefault, err
		}
		return StatusDefault, nil
	}
}

func NewTaguHandler(world AdvancedCombatSkillWorld) Handler {
	return NewTaguHandlerWithDeathFinalizer(world, nil)
}

func NewTaguHandlerWithDeathFinalizer(world AdvancedCombatSkillWorld, finalizer AttackDeathFinalizer) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("tagu: actor creature %q not found", viewer.CreatureID)
		}

		target, ordinal := lookTarget(resolved)
		if target == "" || attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
			ctx.WriteString("누굴 공격합니까?\n")
			return StatusDefault, nil
		}
		if message := taguClassRejection(actor); message != "" {
			ctx.WriteString(message)
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, taguCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		weapon, ok := taguWieldWeapon(world, actor)
		if !ok {
			ctx.WriteString("타구봉법을 구사하시려면 둔탁한 무기가 필요합니다.\n")
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

		if err := revealKickActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}
		if !victim.hasPlayer {
			if attackCreatureProtected(victim.creature) {
				ctx.WriteString("당신은 그 상대를 해칠 수 없습니다.\n")
				return StatusDefault, nil
			}
			if taguMagicOnlyDeflects(actor, victim.creature) {
				ctx.WriteString("당신의 타구봉법이 " + attackCreatureName(victim.creature) + "에게 아무 소용이 없는듯 합니다.\n")
				return StatusDefault, nil
			}
			if taguEnchantOnlyDeflects(world, weapon, victim.creature) {
				ctx.WriteString("당신의 무기가 " + attackCreatureName(victim.creature) + "에게 아무 소용이 없는듯 합니다.\n")
				return StatusDefault, nil
			}
		}
		if !invincibleAttackTargetAlive(victim.creature) {
			ctx.WriteString("그 상대는 공격할 수 없습니다.\n")
			return StatusDefault, nil
		}
		if !victim.hasPlayer {
			advancedCombatPrimeMonsterTarget(world, victim.creature.ID, actor.ID)
		}

		actorName := attackCreatureName(actor)
		victimName := attackCreatureName(victim.creature)
		ctx.WriteString("\n\"천하의 모든 황견들이시여. 내 봉 앞에 나타난 적을 물어 주소서~\"\n")
		ctx.WriteString("당신은 힘차게 봉을 휘둘러 타구봉법을 시작합니다.\n")

		if attackRoll(1, 100) > taguChance(actor, victim.creature) {
			if err := world.SetCreatureCooldown(actor.ID, taguCooldownKey, now, taguFailureCooldownSeconds(actor)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("당신은 타구봉법에 실패했습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+"에게 타구봉법을 시도합니다.\n")
		}

		weapon, stopped, err := bashMaybeSpendWield(ctx, world, actor, weapon)
		if err != nil || stopped {
			return StatusDefault, err
		}
		if !taguHits(actor, victim.creature) {
			if err := world.SetCreatureCooldown(actor.ID, taguCooldownKey, now, taguMissCooldownSeconds(actor)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("당신은 타구봉법에 실패했습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+"에게 타구봉법을 시도합니다.\n")
		}
		if err := world.SetCreatureCooldown(actor.ID, taguCooldownKey, now, taguSuccessCooldownSeconds(actor)); err != nil {
			return StatusDefault, err
		}

		hits, total, dead, err := applyTaguDamage(ctx, world, room.ID, actor, victim, weapon)
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(fmt.Sprintf("당신은 총 %d연타 %d점의 공격을 %s에게 가했습니다.\n", hits, total, victimName))
		if dead {
			if !victim.hasPlayer {
				if err := finalizeMonsterDeathWithOptionalFinalizer(ctx, world, finalizer, actor, victim.creature); err != nil {
					return StatusDefault, err
				}
			}
			ctx.WriteString("당신은 " + victimName + krtext.Particle(victimName, '3') + " 죽였습니다.\n")
		}
		return StatusDefault, nil
	}
}

func bnahanClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	if class < model.ClassInvincible {
		if class == model.ClassBarbarian && attackCreatureLevel(actor) >= 50 {
			return ""
		}
		return "권법가 레벨 50이상만 쓸수 있는 기술입니다.\n"
	}
	if !kickHasBarbarianTraining(actor) {
		return "아직 권법가를 무적수련하지 않았습니다.\n"
	}
	return ""
}

func taguClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	if class < model.ClassInvincible {
		if class == model.ClassThief && attackCreatureLevel(actor) >= 50 {
			return ""
		}
		return "도둑 레벨 50이상만 쓸수 있는 기술입니다.\n"
	}
	if !thiefStatHasTraining(actor) {
		return "아직 도둑을 무적수련하지 않았습니다.\n"
	}
	return ""
}

func taguWieldWeapon(world InventoryWorld, actor model.Creature) (model.ObjectInstance, bool) {
	weaponID := equippedObjectID(actor, "wield")
	if weaponID.IsZero() {
		return model.ObjectInstance{}, false
	}
	weapon, ok := world.Object(weaponID)
	if !ok {
		return model.ObjectInstance{}, false
	}
	return weapon, objectLegacyType(world, weapon) == legacyObjectBlunt
}

func bnahanTargets(world LookWorld, room model.Room, viewer LookViewer) []model.Creature {
	detectInvisible := viewerDetectsInvisible(world, viewer)
	targets := make([]model.Creature, 0, len(room.CreatureIDs))
	for _, id := range room.CreatureIDs {
		if id.IsZero() || id == viewer.CreatureID {
			continue
		}
		creature, ok := world.Creature(id)
		if !ok || creature.RoomID != room.ID || attackCreatureIsPlayer(creature) || creatureHPDead(creature) {
			continue
		}
		if attackCreatureProtected(creature) || !creatureVisibleInRoomLook(creature, viewer, detectInvisible) {
			continue
		}
		targets = append(targets, creature)
	}
	return targets
}

type combatTargetPrimerWorld interface {
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
}

func advancedCombatPrimeMonsterTargets(world combatTargetPrimerWorld, targets []model.Creature, actorID model.CreatureID) {
	for _, target := range targets {
		advancedCombatPrimeMonsterTarget(world, target.ID, actorID)
	}
}

func advancedCombatPrimeMonsterTarget(world combatTargetPrimerWorld, targetID model.CreatureID, actorID model.CreatureID) {
	if adder, ok := any(world).(interface {
		AddEnemy(attacker, defender model.CreatureID) (bool, error)
	}); ok {
		_, _ = adder.AddEnemy(targetID, actorID)
	}
	_, _ = world.UpdateCreatureTags(targetID, []string{"was_attacked"}, nil)
}

func bnahanChance(actor model.Creature, targets []model.Creature) int {
	totalLevel := 0
	for _, target := range targets {
		totalLevel += attackCreatureLevel(target)
	}
	averageLevel := 0
	if len(targets) > 0 {
		averageLevel = totalLevel / len(targets)
	}
	chance := 50 + ((attackCreatureLevel(actor) + 3) / 4) - (averageLevel/4)*2
	chance += legacyStatBonus(creatureStat(actor, "strength")) * 5
	chance += legacyStatBonus(creatureStat(actor, "dexterity")) * 7
	chance = minInt(90, chance)
	if attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
		chance = minInt(20, chance)
	}
	return chance
}

func bnahanDamage(actor model.Creature) int {
	maxRoll := minInt(30, 20-creatureStat(actor, "thaco"))
	if maxRoll < 1 {
		maxRoll = 1
	}
	return normalizeAttackDamage(attackRoll(1, maxRoll) + statsDamage(actor)*4)
}

func bnahanCooldownSeconds(actor model.Creature) int64 {
	interval := 15 - minInt(7, creatureStat(actor, "dexterity")/4)
	return int64(maxInt(1, interval))
}

func bnahanMagicOnlyGlances(actor model.Creature, victim model.Creature) bool {
	if !attackCreatureHasFlag(victim, "magicOnly", "mmgonl", "MMGONL") {
		return false
	}
	if creatureStat(actor, "piety") >= creatureStat(victim, "piety") {
		return false
	}
	return attackRoll(0, 1) == 1
}

func taguChance(actor model.Creature, victim model.Creature) int {
	chance := 50 + ((((attackCreatureLevel(actor) + 3) / 4) - ((attackCreatureLevel(victim) + 3) / 4)) * 2)
	chance += legacyStatBonus(creatureStat(actor, "intelligence")) * 2
	chance += legacyStatBonus(creatureStat(actor, "dexterity")) * 7
	chance = minInt(90, chance)
	if attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
		chance = minInt(20, chance)
	}
	return chance
}

func taguHits(actor model.Creature, victim model.Creature) bool {
	target := creatureStat(actor, "thaco") - creatureStat(victim, "armor")/10
	return attackRoll(1, 20) >= target
}

func taguStrikeCount(actor model.Creature) int {
	base := (20 - creatureStat(actor, "thaco")) / 10
	rollMax := maxInt(1, (attackCreatureLevel(actor)+29)/30)
	return maxInt(1, base+attackRoll(1, rollMax))
}

func taguStrikeDamage(world InventoryWorld, actor model.Creature, weapon model.ObjectInstance) int {
	return normalizeAttackDamage(statsDamage(actor) + objectDamage(world, weapon)*3)
}

func taguSuccessCooldownSeconds(actor model.Creature) int64 {
	return int64(maxInt(1, 20-creatureStat(actor, "dexterity")/7))
}

func taguMissCooldownSeconds(actor model.Creature) int64 {
	return int64(maxInt(1, 15-creatureStat(actor, "dexterity")/6))
}

func taguFailureCooldownSeconds(actor model.Creature) int64 {
	return int64(maxInt(1, 15-minInt(7, creatureStat(actor, "dexterity")/3)))
}

func taguMagicOnlyDeflects(actor model.Creature, victim model.Creature) bool {
	if !attackCreatureHasFlag(victim, "magicOnly", "mmgonl", "MMGONL") {
		return false
	}
	if creatureStat(actor, "piety") >= creatureStat(victim, "piety") {
		return false
	}
	return attackRoll(0, 1) == 1
}

func taguEnchantOnlyDeflects(world InventoryWorld, weapon model.ObjectInstance, victim model.Creature) bool {
	if !attackCreatureHasFlag(victim, "magicOrEnchantedOnly", "enchantOnly", "menonl", "MENONL") {
		return false
	}
	if attackRoll(0, 1) == 0 {
		return false
	}
	adjustment, ok := objectIntProperty(world, weapon, "adjustment")
	return !ok || adjustment < 1
}

func applyTaguDamage(
	ctx *Context,
	world AdvancedCombatSkillWorld,
	roomID model.RoomID,
	actor model.Creature,
	victim kickTarget,
	weapon model.ObjectInstance,
) (int, int, bool, error) {
	strikes := taguStrikeCount(actor)
	hits := 0
	total := 0
	dead := false
	currentVictim := victim.creature
	actorName := attackCreatureName(actor)
	victimName := attackCreatureName(victim.creature)
	for i := 0; i < strikes; i++ {
		damage := taguStrikeDamage(world, actor, weapon)
		updated, applied, hitDead, err := world.ApplyCreatureDamage(currentVictim.ID, damage)
		if err != nil {
			return hits, total, dead, err
		}
		hits++
		total += applied
		currentVictim = updated
		ctx.WriteString(fmt.Sprintf("당신은 타구봉법으로 %s에게 %d점의 피해를 입혔습니다.\n", victimName, applied))
		if err := roomBroadcast(ctx, roomID, fmt.Sprintf("\n%s%s 타구봉법으로 %s에게 %d점의 피해를 입혔습니다.\n", actorName, krtext.Particle(actorName, '1'), victimName, applied)); err != nil {
			return hits, total, dead, err
		}
		if hitDead {
			dead = true
			break
		}
	}
	if !victim.hasPlayer && total > 0 {
		if err := world.RecordCreatureDamage(victim.creature.ID, actor.ID, total); err != nil {
			return hits, total, dead, err
		}
	}
	return hits, total, dead, nil
}
