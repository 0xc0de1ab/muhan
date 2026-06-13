package command

import (
	"fmt"
	"time"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	changCooldownKey = "chang"
	choiCooldownKey  = "choi"
	rmBlind2MPCost   = 20
)

type UtilityCombatSkillWorld interface {
	LookWorld
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
	RecordCreatureDamage(model.CreatureID, model.CreatureID, int) error
	FinalizeMonsterDeath(model.CreatureID) (bool, error)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
	MoveObject(model.ObjectInstanceID, model.ObjectLocation) error
	DestroyObject(model.ObjectInstanceID) error
	ConsumeCreatureObjectCharge(model.ObjectInstanceID, model.CreatureID, bool) (model.ObjectInstance, bool, bool, error)
	RecalculateAC(model.CreatureID) error
	RecalculateTHACO(model.CreatureID) error
}

type RmBlind2World interface {
	LookWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
}

type utilityAreaSkill struct {
	name                  string
	cooldownKey           string
	failMessage           string
	weaponRequiredMessage string
	preChanceText         []string
	successText           []string
	weaponType            int
	deathFinalizer        AttackDeathFinalizer
	classRejection        func(model.Creature) string
	damage                func(UtilityCombatSkillWorld, model.Creature, model.ObjectInstance) int
}

func NewChangHandler(world UtilityCombatSkillWorld) Handler {
	return NewChangHandlerWithDeathFinalizer(world, nil)
}

func NewChangHandlerWithDeathFinalizer(world UtilityCombatSkillWorld, finalizer AttackDeathFinalizer) Handler {
	return newUtilityAreaSkillHandler(world, utilityAreaSkill{
		name:                  "창격술",
		cooldownKey:           changCooldownKey,
		failMessage:           "당신의 창격술이 적의 기세에 눌려 실패했습니다.\n",
		weaponRequiredMessage: "창격술을 구사하시려면 창 종류의 무기가 필요합니다.\n",
		weaponType:            legacyObjectPole,
		classRejection:        changClassRejection,
		preChanceText: []string{
			"\n창격술이란 길고 긴 창으로 적의 사지를 막는다..\n",
			"\"지금 내가 보여주는 이 기술은... 그 창격술이니.. 잘보아라\"\n\n",
			"당신은 창격술로 적의 사지를 동시에 공격합니다..\n\n",
		},
		deathFinalizer: finalizer,
		damage:         changDamage,
	})
}

func NewChoiHandler(world UtilityCombatSkillWorld) Handler {
	return NewChoiHandlerWithDeathFinalizer(world, nil)
}

func NewChoiHandlerWithDeathFinalizer(world UtilityCombatSkillWorld, finalizer AttackDeathFinalizer) Handler {
	return newUtilityAreaSkillHandler(world, utilityAreaSkill{
		name:                  "최루탄",
		cooldownKey:           choiCooldownKey,
		failMessage:           "당신의 최루탄이 적의 기세에 눌려 실패했습니다.\n",
		weaponRequiredMessage: "최루탄을 구사하시려면 활종류의 무기가 필요합니다.\n",
		weaponType:            legacyObjectMissile,
		classRejection:        choiClassRejection,
		successText: []string{
			"최루탄이란 나의 모든 기로 적의 힘을 소모하는 것이다.\n",
			"받아라 나의 작고 매운 최루탄을 ....\n",
		},
		deathFinalizer: finalizer,
		damage:         choiDamage,
	})
}

func NewRmBlind2Handler(world RmBlind2World) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("rm_blind2: actor creature %q not found", viewer.CreatureID)
		}
		player, hasPlayer := model.Player{}, false
		if !viewer.PlayerID.IsZero() {
			player, hasPlayer = world.Player(viewer.PlayerID)
		}

		if creatureStat(actor, "mpCurrent") < rmBlind2MPCost {
			ctx.WriteString("당신의 도력이 부족합니다.\n")
			return StatusDefault, nil
		}
		if !rmBlind2HasYellowI(actor) {
			ctx.WriteString("아직 당신에게 그런능력이 없습니다.\n")
			return StatusDefault, nil
		}
		if !statusEffectActive(player, actor, "blind", "blinded", "PBLIND", "MBLIND") {
			ctx.WriteString("실명이 되었을때만 사용할수 있습니다.\n")
			return StatusDefault, nil
		}
		if len(resolved.Args) > 0 {
			ctx.WriteString("실명해소술은 자신 치료 기술입니다.\n")
			return StatusDefault, nil
		}

		if err := rmBlind2ClearBlind(world, viewer, actor, hasPlayer); err != nil {
			return StatusDefault, err
		}
		if err := world.SetCreatureStat(actor.ID, "mpCurrent", creatureStat(actor, "mpCurrent")-rmBlind2MPCost); err != nil {
			return StatusDefault, err
		}

		ctx.WriteString("\n당신은 손에서 개안부를 만들어 눈을 찾습니다.\n")
		ctx.WriteString("그 개안부를 눈에 붙이니 당신의 눈이 다시 떠집니다.\n")
		actorName := attackCreatureName(actor)
		return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 손에서 개안부를 만들어 눈에 붙입니다.\n감겼던 눈이 다시 떠집니다.\n")
	}
}

func newUtilityAreaSkillHandler(world UtilityCombatSkillWorld, skill utilityAreaSkill) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("%s: actor creature %q not found", skill.cooldownKey, viewer.CreatureID)
		}

		if reject := skill.classRejection(actor); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}
		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, skill.cooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		weapon, ok := utilityCombatWieldWeapon(world, actor, skill.weaponType)
		if !ok {
			ctx.WriteString(skill.weaponRequiredMessage)
			return StatusDefault, nil
		}
		targets := utilityCombatTargets(world, room, viewer)
		if len(targets) == 0 {
			ctx.WriteString("이 방에는 당신이 공격할 적이 없습니다.\n")
			return StatusDefault, nil
		}
		utilityAreaSkillPrimeTargets(world, actor, targets)
		if err := revealKickActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}
		for _, line := range skill.preChanceText {
			ctx.WriteString(line)
		}

		actorName := attackCreatureName(actor)
		chance := utilityAreaSkillChance(actor, targets)
		if attackRoll(1, 22) > chance {
			ctx.WriteString(skill.failMessage)
			if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '0')+" "+skill.name+"이 적의 기세에 눌려 실패했습니다.\n"); err != nil {
				return StatusDefault, err
			}
			if err := utilityCombatMaybeBreakWieldOnFailure(ctx, world, room.ID, actor, weapon); err != nil {
				return StatusDefault, err
			}
			return StatusDefault, world.SetCreatureCooldown(actor.ID, skill.cooldownKey, now, utilityAreaSkillCooldownSeconds(actor))
		}

		updatedWeapon, stopped, err := utilityCombatMaybeSpendWield(ctx, world, actor, weapon)
		if err != nil || stopped {
			return StatusDefault, err
		}
		weapon = updatedWeapon
		for _, line := range skill.successText {
			ctx.WriteString(line)
		}

		count := minInt((chance+1)/3, len(targets))
		for _, target := range targets[:count] {
			victim, ok := world.Creature(target.ID)
			if !ok || victim.RoomID != room.ID || creatureHPDead(victim) || attackCreatureProtected(victim) {
				continue
			}
			if err := utilityAreaSkillDamageTarget(ctx, world, room.ID, actor, victim, weapon, skill); err != nil {
				return StatusDefault, err
			}
		}
		return StatusDefault, world.SetCreatureCooldown(actor.ID, skill.cooldownKey, now, utilityAreaSkillCooldownSeconds(actor))
	}
}

func changClassRejection(actor model.Creature) string {
	if creatureClass(actor) < model.ClassCaretaker {
		return "초인 이상만 쓸수 있는 기술입니다.\n"
	}
	return ""
}

func choiClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	if class < model.ClassInvincible {
		if class == model.ClassRanger && attackCreatureLevel(actor) >= 50 {
			return ""
		}
		return "포졸 레벨 50이상만 쓸수 있는 기술입니다.\n"
	}
	if !guardHasRangerTraining(actor) {
		return "아직 포졸을 무적수련하지 않았습니다.\n"
	}
	return ""
}

func rmBlind2HasYellowI(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"YELLOWI",
		"yellowI",
		"yellowInvincible",
		"yellowTraining",
		"yellowMode",
	)
}

func rmBlind2ClearBlind(world RmBlind2World, viewer LookViewer, actor model.Creature, hasPlayer bool) error {
	remove := []string{"blind", "blinded", "PBLIND", "MBLIND"}
	if _, err := world.UpdateCreatureTags(actor.ID, nil, remove); err != nil {
		return err
	}
	if hasPlayer {
		if _, err := world.UpdatePlayerTags(viewer.PlayerID, nil, remove); err != nil {
			return err
		}
	}
	targets := normalizedFlagSet(remove...)
	for key, value := range actor.Stats {
		if value == 0 {
			continue
		}
		if _, ok := targets[normalizeFlagName(key)]; ok {
			if err := world.SetCreatureStat(actor.ID, key, 0); err != nil {
				return err
			}
		}
	}
	return nil
}

func utilityCombatWieldWeapon(world InventoryWorld, actor model.Creature, legacyType int) (model.ObjectInstance, bool) {
	weaponID := equippedObjectID(actor, "wield")
	if weaponID.IsZero() {
		return model.ObjectInstance{}, false
	}
	weapon, ok := world.Object(weaponID)
	if !ok {
		return model.ObjectInstance{}, false
	}
	return weapon, objectLegacyType(world, weapon) == legacyType
}

func utilityCombatTargets(world LookWorld, room model.Room, viewer LookViewer) []model.Creature {
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

func utilityAreaSkillPrimeTargets(world UtilityCombatSkillWorld, actor model.Creature, targets []model.Creature) {
	adder, hasAdder := any(world).(interface {
		AddEnemy(attacker, defender model.CreatureID) (bool, error)
	})
	for _, target := range targets {
		if hasAdder {
			_, _ = adder.AddEnemy(target.ID, actor.ID)
		}
		_, _ = world.UpdateCreatureTags(target.ID, []string{"was_attacked"}, nil)
	}
}

func utilityAreaSkillCooldownSeconds(actor model.Creature) int64 {
	interval := 18 - minInt(7, creatureStat(actor, "dexterity")/4)
	return int64(maxInt(1, interval))
}

func utilityAreaSkillChance(actor model.Creature, targets []model.Creature) int {
	enmTHACOTotal := 0
	for _, target := range targets {
		enmTHACOTotal += 20 - creatureStat(target, "thaco")
	}
	chance := 20 - creatureStat(actor, "thaco")
	chance += legacyStatBonus(creatureStat(actor, "dexterity")) * 2
	if len(targets) > 0 {
		chance -= enmTHACOTotal / len(targets)
	}
	chance += (attackCreatureLevel(actor) + 29) / 30
	if chance > 20 {
		return 20
	}
	if chance < 5 {
		return 5
	}
	return chance
}

func utilityCombatMaybeSpendWield(
	ctx *Context,
	world UtilityCombatSkillWorld,
	actor model.Creature,
	weapon model.ObjectInstance,
) (model.ObjectInstance, bool, error) {
	shots, hasShots := objectIntProperty(world, weapon, "shotsCurrent")
	if hasShots && shots > 0 {
		updated, _, consumed, err := world.ConsumeCreatureObjectCharge(weapon.ID, actor.ID, false)
		if err != nil {
			return model.ObjectInstance{}, false, err
		}
		if consumed {
			weapon = updated
			shots, hasShots = objectIntProperty(world, weapon, "shotsCurrent")
		}
	}
	if !hasShots || shots > 0 {
		return weapon, false, nil
	}
	name := objectDisplayName(world, weapon)
	if err := world.MoveObject(weapon.ID, model.ObjectLocation{CreatureID: actor.ID, Slot: "inventory"}); err != nil {
		return model.ObjectInstance{}, false, err
	}
	ctx.WriteString("\n" + name + krtext.Particle(name, '1') + " 부서져 버렸습니다.\n")
	return weapon, true, nil
}

type combatWieldFailureBreakWorld interface {
	InventoryWorld
	DestroyObject(model.ObjectInstanceID) error
}

func utilityCombatMaybeBreakWieldOnFailure(
	ctx *Context,
	world combatWieldFailureBreakWorld,
	roomID model.RoomID,
	actor model.Creature,
	weapon model.ObjectInstance,
) error {
	if objectHasAnyFlagOrProperty(world, weapon, "ONSHAT", "onshat", "shatterproof") {
		return nil
	}
	shotsCurrent, hasCurrent := objectIntProperty(world, weapon, "shotsCurrent")
	shotsMax, hasMax := objectIntProperty(world, weapon, "shotsMax")
	if !hasCurrent || !hasMax || shotsMax <= 0 || shotsCurrent >= shotsMax/2 {
		return nil
	}
	if attackRoll(1, 5) > 2 {
		return nil
	}
	name := objectDisplayName(world, weapon)
	ctx.WriteString("\n" + name + krtext.Particle(name, '1') + " 무기가 부서집니다.\n")
	actorName := attackCreatureName(actor)
	if err := roomBroadcast(ctx, roomID, "\n"+actorName+"의 "+name+krtext.Particle(name, '1')+" 무기가 부서집니다.\n"); err != nil {
		return err
	}
	return world.DestroyObject(weapon.ID)
}

func utilityAreaSkillDamageTarget(
	ctx *Context,
	world UtilityCombatSkillWorld,
	roomID model.RoomID,
	actor model.Creature,
	victim model.Creature,
	weapon model.ObjectInstance,
	skill utilityAreaSkill,
) error {
	damage := skill.damage(world, actor, weapon)
	victimName := attackCreatureName(victim)
	if utilityCombatMagicOnlyGlances(actor, victim) {
		ctx.WriteString("당신의 공격은 " + victimName + "에게 아무런 상처도 내지 못합니다.\n")
		damage = 1
	}
	if utilityCombatEnchantedOnlyGlances(world, victim, weapon) {
		ctx.WriteString("당신의 공격은 " + victimName + "의 갑옷을 뚫기엔 역부족입니다.\n")
		damage = 1
	}

	_, applied, dead, err := world.ApplyCreatureDamage(victim.ID, damage)
	if err != nil {
		return err
	}
	if err := world.RecordCreatureDamage(victim.ID, actor.ID, applied); err != nil {
		return err
	}

	actorName := attackCreatureName(actor)
	ctx.WriteString(fmt.Sprintf("당신은 %s로 %s에게 %d점의 피해를 입힙니다.\n", skill.name, victimName, applied))
	if err := roomBroadcast(ctx, roomID, fmt.Sprintf("\n%s%s %s으로 %s에게 %d점의 피해를 입혔습니다.\n", actorName, krtext.Particle(actorName, '1'), skill.name, victimName, applied)); err != nil {
		return err
	}
	if !dead {
		return nil
	}
	if err := finalizeMonsterDeathWithOptionalFinalizer(ctx, world, skill.deathFinalizer, actor, victim); err != nil {
		return err
	}
	ctx.WriteString("\n당신의 매서운 " + skill.name + "으로 " + victimName + krtext.Particle(victimName, '3') + " 죽였습니다.\n")
	return roomBroadcast(ctx, roomID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+skill.name+"으로 "+victimName+krtext.Particle(victimName, '3')+" 죽였습니다.\n")
}

func changDamage(world UtilityCombatSkillWorld, actor model.Creature, weapon model.ObjectInstance) int {
	damage := (statsDamage(actor)*2 + objectDamage(world, weapon)*4) * attackRoll(1, 3)
	return normalizeAttackDamage(damage)
}

func choiDamage(world UtilityCombatSkillWorld, actor model.Creature, weapon model.ObjectInstance) int {
	damage := (statsDamage(actor)*2 + objectDamage(world, weapon)*4) / attackRoll(1, 5)
	damage *= attackRoll(1, 2)
	return normalizeAttackDamage(damage)
}

func utilityCombatMagicOnlyGlances(actor model.Creature, victim model.Creature) bool {
	if !attackCreatureHasFlag(victim, "magicOnly", "mmgonl", "MMGONL") {
		return false
	}
	if creatureStat(actor, "piety") >= creatureStat(victim, "piety") {
		return false
	}
	return attackRoll(0, 1) == 1
}

func utilityCombatEnchantedOnlyGlances(world InventoryWorld, victim model.Creature, weapon model.ObjectInstance) bool {
	if !attackCreatureHasFlag(victim, "magicOrEnchantedOnly", "enchantOnly", "menonl", "MENONL") {
		return false
	}
	adjustment, ok := objectIntProperty(world, weapon, "adjustment")
	return !ok || adjustment < 1
}
