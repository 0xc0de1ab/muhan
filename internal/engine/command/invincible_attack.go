package command

import (
	"fmt"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const (
	invincibleKickCooldownKey = "invincible_kick"
	oneKillCooldownKey        = "one_kill"
)

type InvincibleAttackWorld interface {
	AttackWorld
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
}

func NewInvincibleKickHandler(world InvincibleAttackWorld) Handler {
	return NewInvincibleKickHandlerWithDeathFinalizer(world, nil)
}

func NewInvincibleKickHandlerWithDeathFinalizer(world InvincibleAttackWorld, finalizer AttackDeathFinalizer) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("invincible_kick: actor creature %q not found", viewer.CreatureID)
		}

		target, ordinal := lookTarget(resolved)
		if target == "" || attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
			ctx.WriteString("누굴 공격합니까?\n")
			return StatusDefault, nil
		}
		if message := invincibleKickClassRejection(actor); message != "" {
			ctx.WriteString(message)
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, invincibleKickCooldownKey, now, 0); err != nil {
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

		if err := revealKickActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}
		if !victim.hasPlayer {
			if attackCreatureProtected(victim.creature) {
				ctx.WriteString("당신은 그 상대를 해칠 수 없습니다.\n")
				return StatusDefault, nil
			}
			if attackCreatureDeflectsMundane(world, actor, victim.creature) {
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
			advancedCombatPrimeMonsterTarget(world, victim.creature.ID, actor.ID)
		}

		actorName := attackCreatureName(actor)
		victimName := attackCreatureName(victim.creature)
		ctx.WriteString("\n천하권법의 최고라 불리는 백보신권을 펼칩니다.\n")
		ctx.WriteString("당신의 장풍이 상대에게 날아갑니다.\n")

		if attackRoll(1, 100) > invincibleKickChance(actor, victim.creature) {
			if err := world.SetCreatureCooldown(actor.ID, invincibleKickCooldownKey, now, invincibleKickFailureCooldown(actor)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("당신의 백보신권이 실패했습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+"에게 백보신권을 시도합니다.\n")
		}
		if !invincibleKickHits(actor, victim.creature) {
			if err := world.SetCreatureCooldown(actor.ID, invincibleKickCooldownKey, now, invincibleKickMissCooldown(actor)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("당신의 백보신권이 실패했습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+"에게 백보신권을 시도합니다.\n")
		}
		if err := world.SetCreatureCooldown(actor.ID, invincibleKickCooldownKey, now, invincibleKickSuccessCooldown(actor)); err != nil {
			return StatusDefault, err
		}

		hits, total, dead, err := applyInvincibleKickDamage(world, actor, victim)
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(fmt.Sprintf("당신은 백보신권으로 %d연타 %d점의 피해를 %s에게 입혔습니다.\n", hits, total, victimName))
		if dead {
			if err := finalizeInvincibleAttackDeath(ctx, world, finalizer, actor, victim); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(victimName + krtext.Particle(victimName, '1') + " 쓰러졌습니다.\n")
		}
		return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+"에게 백보신권을 날립니다.\n")
	}
}

func NewOneKillHandler(world InvincibleAttackWorld) Handler {
	return NewOneKillHandlerWithDeathFinalizer(world, nil)
}

func NewOneKillHandlerWithDeathFinalizer(world InvincibleAttackWorld, finalizer AttackDeathFinalizer) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("one_kill: actor creature %q not found", viewer.CreatureID)
		}

		target, ordinal := lookTarget(resolved)
		if target == "" {
			ctx.WriteString("\n누구를 공격하시겠습니까?\n")
			return StatusDefault, nil
		}
		if message := oneKillClassRejection(actor); message != "" {
			ctx.WriteString(message)
			return StatusDefault, nil
		}

		victimCreature, ok := findAttackCreatureTarget(world, room, viewer, target, ordinal)
		if !ok {
			ctx.WriteString("\n그런 것은 여기 없습니다.\n")
			return StatusDefault, nil
		}
		victim := kickTarget{creature: victimCreature}
		if !attackCreatureIsPlayer(victim.creature) && sneakMonsterTargetsActor(world, victim.creature.ID, viewer.PlayerID, actor) {
			ctx.WriteString("당신은 " + stealSubjectPronoun(victim.creature) + "와 싸우는 중입니다.\n")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, oneKillCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		weapon, ok := oneKillWieldedWeapon(world, actor)
		if !ok {
			ctx.WriteString("일격필살을 하시려면 날카롭거나 찌르는 무기가 필요합니다.\n")
			return StatusDefault, nil
		}

		if err := revealKickActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}
		if attackCreatureProtected(victim.creature) {
			ctx.WriteString("당신은 그 상대를 해칠 수 없습니다.\n")
			return StatusDefault, nil
		}
		if !invincibleAttackTargetAlive(victim.creature) {
			ctx.WriteString("그 상대는 공격할 수 없습니다.\n")
			return StatusDefault, nil
		}
		if err := world.SetCreatureCooldown(actor.ID, oneKillCooldownKey, now, oneKillPreliminaryCooldownSeconds); err != nil {
			return StatusDefault, err
		}
		if attackCreatureDeflectsMundane(world, actor, victim.creature) {
			name := attackCreatureName(victim.creature)
			ctx.WriteString("당신의 공격이 " + name + "에게 아무 소용이 없는듯 합니다.\n")
			return StatusDefault, nil
		}
		advancedCombatPrimeMonsterTarget(world, victim.creature.ID, actor.ID)

		actorName := attackCreatureName(actor)
		victimName := attackCreatureName(victim.creature)
		ctx.WriteString("\n자객의 비전 일격필살을 펼칩니다.\n")
		ctx.WriteString("당신은 혼신의 힘을 무기에 집중합니다.\n\n")

		if attackRoll(1, 100) > oneKillChance(world, actor, victim.creature, weapon) {
			if err := world.SetCreatureCooldown(actor.ID, oneKillCooldownKey, now, oneKillCooldownSeconds); err != nil {
				return StatusDefault, err
			}
			applied, err := applyOneKillBacklash(world, actor)
			if err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(actorName + krtext.Particle(actorName, '1') + " 일격필살에 실패했습니다.\n")
			if applied > 0 {
				ctx.WriteString(fmt.Sprintf("%s%s 반격의 빈틈을 보여 %d점의 피해를 입었습니다.\n", actorName, krtext.Particle(actorName, '1'), applied))
			}
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+"에게 일격필살을 시도합니다.\n")
		}
		if err := world.SetCreatureCooldown(actor.ID, oneKillCooldownKey, now, oneKillPreliminaryCooldownSeconds); err != nil {
			return StatusDefault, err
		}

		var stopped bool
		weapon, stopped, err = bashMaybeSpendWield(ctx, world, actor, weapon)
		if err != nil || stopped {
			return StatusDefault, err
		}
		if err := world.SetCreatureCooldown(actor.ID, oneKillCooldownKey, now, oneKillCooldownSeconds); err != nil {
			return StatusDefault, err
		}

		damage := oneKillDamage(world, actor, victim.creature, weapon)
		_, applied, dead, err := world.ApplyCreatureDamage(victim.creature.ID, damage)
		if err != nil {
			return StatusDefault, err
		}
		if err := world.RecordCreatureDamage(victim.creature.ID, actor.ID, applied); err != nil {
			return StatusDefault, err
		}

		ctx.WriteString(fmt.Sprintf("당신은 일격필살로 %s에게 %d점의 피해를 입혔습니다.\n", victimName, applied))
		if dead {
			if err := finalizeInvincibleAttackDeath(ctx, world, finalizer, actor, victim); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(victimName + krtext.Particle(victimName, '1') + " 쓰러졌습니다.\n")
		}
		return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+"에게 일격필살을 가합니다.\n")
	}
}

const (
	oneKillCooldownSeconds            = int64(10)
	oneKillPreliminaryCooldownSeconds = int64(5)
)

func invincibleKickClassRejection(actor model.Creature) string {
	if creatureClass(actor) < legacyClassInvincible {
		return "무적 이상만 사용할 수 있는 기술입니다.\n"
	}
	if !kickHasBarbarianTraining(actor) {
		return "\n권법가를 무적수련하지 않았습니다..\n"
	}
	return ""
}

func oneKillClassRejection(actor model.Creature) string {
	if creatureClass(actor) < legacyClassInvincible {
		return "무적 이상만 사용할 수 있는 기술입니다.\n"
	}
	if !attackCreatureHasFlag(actor,
		"SASSASSIN",
		"assassinSpell",
		"assassinTraining",
		"assassinMode",
	) {
		return "\n자객을 무적수련하지 않았습니다..\n"
	}
	return ""
}

func oneKillWieldedWeapon(world InventoryWorld, actor model.Creature) (model.ObjectInstance, bool) {
	weaponID := equippedObjectID(actor, "wield")
	if weaponID.IsZero() {
		return model.ObjectInstance{}, false
	}
	weapon, ok := world.Object(weaponID)
	if !ok {
		return model.ObjectInstance{}, false
	}
	legacyType := objectLegacyType(world, weapon)
	return weapon, legacyType == legacyObjectSharp || legacyType == legacyObjectThrust
}

func invincibleAttackTargetAlive(victim model.Creature) bool {
	if victim.Stats == nil {
		return false
	}
	hp, ok := victim.Stats["hpCurrent"]
	return ok && hp > 0
}

func invincibleKickChance(actor model.Creature, victim model.Creature) int {
	chance := 50 + (((attackCreatureLevel(actor)+3)/4)-((attackCreatureLevel(victim)+3)/4))*2
	chance += legacyStatBonus(creatureStat(actor, "intelligence")) * 3
	chance += legacyStatBonus(creatureStat(actor, "dexterity")) * 3
	chance = minInt(90, chance)
	if attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
		chance = minInt(20, chance)
	}
	return chance
}

func invincibleKickHits(actor model.Creature, victim model.Creature) bool {
	target := creatureStat(actor, "thaco") - creatureStat(victim, "armor")/10
	return attackRoll(1, 20) >= target
}

func invincibleKickStrikeCount(actor model.Creature) int {
	count := (20 - creatureStat(actor, "thaco")) / 9
	if count < 0 {
		count = 0
	}
	rollMax := maxInt(1, (attackCreatureLevel(actor)+23)/30)
	return maxInt(1, count+attackRoll(1, rollMax))
}

func invincibleKickStrikeDamage(actor model.Creature, victim model.Creature) int {
	if creatureClass(victim) > legacyClassCaretaker {
		return 1
	}
	return normalizeAttackDamage(statsDamage(actor)*3 + attackRoll(0, maxInt(0, creatureStat(actor, "strength")))*2)
}

func applyInvincibleKickDamage(world InvincibleAttackWorld, actor model.Creature, victim kickTarget) (int, int, bool, error) {
	strikes := invincibleKickStrikeCount(actor)
	hits := 0
	total := 0
	dead := false
	currentVictim := victim.creature
	for i := 0; i < strikes; i++ {
		damage := invincibleKickStrikeDamage(actor, currentVictim)
		updated, applied, hitDead, err := world.ApplyCreatureDamage(currentVictim.ID, damage)
		if err != nil {
			return hits, total, dead, err
		}
		hits++
		total += applied
		currentVictim = updated
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

func invincibleKickSuccessCooldown(actor model.Creature) int64 {
	return int64(maxInt(1, 20-creatureStat(actor, "dexterity")/7))
}

func invincibleKickMissCooldown(actor model.Creature) int64 {
	return int64(maxInt(1, 15-creatureStat(actor, "dexterity")/6))
}

func invincibleKickFailureCooldown(actor model.Creature) int64 {
	return int64(maxInt(1, 15-minInt(7, creatureStat(actor, "dexterity")/3)))
}

func oneKillChance(world InventoryWorld, actor model.Creature, victim model.Creature, weapon model.ObjectInstance) int {
	chance := (creatureStat(actor, "dexterity")-creatureStat(victim, "dexterity"))*3 +
		(20-creatureStat(actor, "thaco"))*2 +
		objectDamage(world, weapon)*2
	return minInt(chance, 70)
}

func oneKillDamage(world InventoryWorld, actor model.Creature, victim model.Creature, weapon model.ObjectInstance) int {
	if creatureClass(victim) > legacyClassCaretaker {
		return 1
	}
	multiplierMin := 1
	if class := creatureClass(actor); class >= legacyClassBulsa {
		multiplierMin = 5
	} else if class == legacyClassCaretaker {
		multiplierMin = 3
	}
	damage := creatureStat(victim, "hpCurrent")/2 +
		creatureStat(actor, "dexterity")*3 +
		objectDamage(world, weapon)*attackRoll(multiplierMin, 7)
	return normalizeAttackDamage(damage)
}

func applyOneKillBacklash(world InvincibleAttackWorld, actor model.Creature) (int, error) {
	armor := creatureStat(actor, "armor")
	hpCurrent := creatureStat(actor, "hpCurrent")
	hpMax := creatureStat(actor, "hpMax")
	if (100-armor) >= 200 && hpCurrent >= (hpMax/3)*2 {
		return 0, nil
	}
	damage := (hpCurrent / 3) * 2
	if damage <= 0 {
		return 0, nil
	}
	_, applied, _, err := world.ApplyCreatureDamage(actor.ID, damage)
	return applied, err
}

func finalizeInvincibleAttackDeath(ctx *Context, world InvincibleAttackWorld, finalizer AttackDeathFinalizer, actor model.Creature, victim kickTarget) error {
	if victim.hasPlayer {
		return nil
	}
	return finalizeMonsterDeathWithOptionalFinalizer(ctx, world, finalizer, actor, victim.creature)
}
