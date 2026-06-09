package command

import (
	"fmt"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const (
	eightCooldownKey = "eight"
	nahanCooldownKey = "nahan"
)

type FormationSkillsWorld interface {
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
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
}

func NewEightHandler(world FormationSkillsWorld) Handler {
	return NewEightHandlerWithDeathFinalizer(world, nil)
}

func NewEightHandlerWithDeathFinalizer(world FormationSkillsWorld, finalizer AttackDeathFinalizer) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("eight: actor creature %q not found", viewer.CreatureID)
		}

		if reject := eightClassRejection(actor); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, eightCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		weapon, ok := eightWieldWeapon(world, actor)
		if !ok {
			ctx.WriteString("영자팔법을 구사하시려면 검종류의 무기가 필요합니다.\n")
			return StatusDefault, nil
		}

		targets := eightTargets(world, room, viewer)
		if len(targets) == 0 {
			ctx.WriteString("이 방에는 당신이 공격할 적이 없습니다.\n")
			return StatusDefault, nil
		}
		advancedCombatPrimeMonsterTargets(world, targets, actor.ID)

		if err := revealKickActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}

		actorName := attackCreatureName(actor)
		ctx.WriteString("검의 모든 방위는 여기서 나오니 이것이 바로 영자팔법이라~ \n")
		ctx.WriteString("\"세상의 어느 누가 영자팔법의 방위를 피하랴!     이야압~~!!!\" \n\n")
		ctx.WriteString("당신은 영자팔법으로 팔방의 모든 방위를 차단하며 공격을 합니다.\n\n")

		chance := eightChance(actor, targets)
		if attackRoll(1, 22) > chance {
			ctx.WriteString("당신의 영자팔법이 적의 기세에 눌려 실패했습니다.\n")
			if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '0')+" 영자팔법이 적의 기세에 눌려 실패했습니다.\n"); err != nil {
				return StatusDefault, err
			}
			if err := utilityCombatMaybeBreakWieldOnFailure(ctx, world, room.ID, actor, weapon); err != nil {
				return StatusDefault, err
			}
			return StatusDefault, world.SetCreatureCooldown(actor.ID, eightCooldownKey, now, eightCooldownSeconds(actor))
		}

		weapon, stopped, err := bashMaybeSpendWield(ctx, world, actor, weapon)
		if err != nil || stopped {
			return StatusDefault, err
		}

		count := minInt((chance+1)/3, len(targets))
		for _, victim := range targets[:count] {
			victimName := attackCreatureName(victim)
			damage := eightDamage(world, actor, weapon)
			if eightMagicOnlyGlances(actor, victim) {
				ctx.WriteString("당신의 검이 " + victimName + "에게 아무런 상처도 내지 못합니다.\n")
				damage = 1
			}
			if eightEnchantedOnlyGlances(world, weapon, victim) {
				ctx.WriteString("당신의 검이 " + victimName + "의 갑옷을 뚫기엔 역부족입니다.\n")
				damage = 1
			}

			_, applied, dead, err := world.ApplyCreatureDamage(victim.ID, damage)
			if err != nil {
				return StatusDefault, err
			}
			if err := world.RecordCreatureDamage(victim.ID, actor.ID, applied); err != nil {
				return StatusDefault, err
			}

			ctx.WriteString(fmt.Sprintf("당신은 영자팔법으로 %s에게 %d의 피해를 입힙니다.\n", victimName, applied))
			if err := roomBroadcast(ctx, room.ID, fmt.Sprintf("\n%s%s 영자팔법으로 %s에게 %d의 피해를 입혔습니다.\n", actorName, krtext.Particle(actorName, '1'), victimName, applied)); err != nil {
				return StatusDefault, err
			}
			if applied/20 > 5 {
				ctx.WriteString("당신에게 입은 상처로 " + victimName + krtext.Particle(victimName, '1') + " 움직이지 못합니다.\n")
			}
			if !dead {
				continue
			}
			if err := finalizeMonsterDeathWithOptionalFinalizer(ctx, world, finalizer, actor, victim); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("\n당신의 뛰어난 영자팔법으로 " + victimName + krtext.Particle(victimName, '3') + " 죽였습니다.\n")
			if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 영자팔법으로 "+victimName+krtext.Particle(victimName, '3')+" 죽였습니다.\n"); err != nil {
				return StatusDefault, err
			}
		}
		return StatusDefault, world.SetCreatureCooldown(actor.ID, eightCooldownKey, now, eightCooldownSeconds(actor))
	}
}

func NewNahanHandler(world FormationSkillsWorld) Handler {
	return NewNahanHandlerWithDeathFinalizer(world, nil)
}

func NewNahanHandlerWithDeathFinalizer(world FormationSkillsWorld, finalizer AttackDeathFinalizer) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("nahan: actor creature %q not found", viewer.CreatureID)
		}

		target, ordinal := lookTarget(resolved)
		if target == "" {
			ctx.WriteString("\n누구를 공격합니까?\n")
			return StatusDefault, nil
		}
		if reject := nahanClassRejection(actor); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}

		victim, ok := findAttackCreatureTarget(world, room, viewer, target, ordinal)
		if !ok {
			ctx.WriteString("\n그런 것은 존재하지 않습니다.\n")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, nahanCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		if err := revealKickActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}

		if attackCreatureProtected(victim) {
			ctx.WriteString("당신은 " + pobackCreaturePronoun(victim) + "를 해칠 수 없습니다.\n")
			return StatusDefault, nil
		}
		if victim.Stats == nil || creatureStat(victim, "hpCurrent") <= 0 {
			ctx.WriteString("그 상대는 공격할 수 없습니다.\n")
			return StatusDefault, nil
		}
		if nahanMagicOnlyGlances(actor, victim) {
			ctx.WriteString("당신의 공격이 " + attackCreatureName(victim) + "에게 아무소용이 없는듯 합니다.\n")
			return StatusDefault, nil
		}

		participants := nahanParticipants(world, room, viewer)
		advancedCombatPrimeMonsterTarget(world, victim.ID, actor.ID)
		for _, participant := range participants {
			advancedCombatPrimeMonsterTarget(world, victim.ID, participant.ID)
		}
		formationCount := len(participants) + 1
		formationMP := creatureStat(actor, "mpCurrent") + nahanParticipantMP(participants)
		if formationCount < 2 {
			ctx.WriteString("당신 혼자서는 나한진을 펼칠 수 없습니다.\n")
			return StatusDefault, nil
		}
		if formationMP < minInt(2000, creatureStat(victim, "mpCurrent"))/formationCount {
			ctx.WriteString("당신과 함께 있는 동료들의 도력이 부족합니다.\n")
			return StatusDefault, nil
		}

		actorName := attackCreatureName(actor)
		victimName := attackCreatureName(victim)
		ctx.WriteString("만다라의 힘을 빌어 세상의 모든 마를 가둘 수 있으니 이것이 나한진이라~~ \n")
		ctx.WriteString("\"아미타불~~ 세상의 모든 마를 소멸 시키리라~~!!\"\n\n")
		ctx.WriteString("당신은 당신의 동료들과 함께 나한진을 펴며 적을 공격합니다.\n\n")

		chance := nahanChance(actor, victim)
		if attackRoll(1, 22) > chance {
			if err := world.SetCreatureStat(actor.ID, "mpCurrent", 0); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("당신과 동료들의 나한진이 실패했습니다.\n")
			if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '0')+" 동료들과 나한진에 실패했습니다.\n"); err != nil {
				return StatusDefault, err
			}
			return StatusDefault, world.SetCreatureCooldown(actor.ID, nahanCooldownKey, now, nahanCooldownSeconds(actor))
		}

		damage := nahanDamage(actor, formationMP, formationCount)
		_, applied, dead, err := world.ApplyCreatureDamage(victim.ID, damage)
		if err != nil {
			return StatusDefault, err
		}
		if err := nahanRecordDamage(world, actor, victim, participants, applied); err != nil {
			return StatusDefault, err
		}

		ctx.WriteString(fmt.Sprintf("당신은 나한진을 펼쳐 %s에게 %d점의 피해를 입혔습니다.\n", victimName, applied))
		if err := roomBroadcast(ctx, room.ID, fmt.Sprintf("\n%s%s 동료들과 함께 나한진을 펼쳐 %s에게 %d의 피해를 입혔습니다.\n", actorName, krtext.Particle(actorName, '1'), victimName, applied)); err != nil {
			return StatusDefault, err
		}
		if !dead {
			return StatusDefault, world.SetCreatureCooldown(actor.ID, nahanCooldownKey, now, nahanCooldownSeconds(actor))
		}

		if err := finalizeMonsterDeathWithOptionalFinalizer(ctx, world, finalizer, actor, victim); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("\n당신은 나한진으로 " + victimName + krtext.Particle(victimName, '3') + " 죽였습니다.\n")
		if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '0')+" 나한진으로 "+victimName+krtext.Particle(victimName, '3')+" 죽였습니다.\n"); err != nil {
			return StatusDefault, err
		}
		return StatusDefault, world.SetCreatureCooldown(actor.ID, nahanCooldownKey, now, nahanCooldownSeconds(actor))
	}
}

func eightClassRejection(actor model.Creature) string {
	if creatureClass(actor) < legacyClassInvincible {
		return "무적이상만 쓸수 있는 기술입니다.\n"
	}
	if !bashHasFighterTraining(actor) {
		return "아직 검사를 무적수련하지 않았습니다.\n"
	}
	return ""
}

func eightWieldWeapon(world InventoryWorld, actor model.Creature) (model.ObjectInstance, bool) {
	weaponID := equippedObjectID(actor, "wield")
	if weaponID.IsZero() {
		return model.ObjectInstance{}, false
	}
	weapon, ok := world.Object(weaponID)
	if !ok {
		return model.ObjectInstance{}, false
	}
	return weapon, objectLegacyType(world, weapon) == legacyObjectThrust
}

func eightTargets(world LookWorld, room model.Room, viewer LookViewer) []model.Creature {
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

func eightChance(actor model.Creature, targets []model.Creature) int {
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

func eightDamage(world InventoryWorld, actor model.Creature, weapon model.ObjectInstance) int {
	return normalizeAttackDamage(statsDamage(actor)*3 + objectDamage(world, weapon)*4)
}

func eightCooldownSeconds(actor model.Creature) int64 {
	interval := 15 - minInt(7, creatureStat(actor, "dexterity")/4)
	return int64(maxInt(1, interval))
}

func eightMagicOnlyGlances(actor model.Creature, victim model.Creature) bool {
	if !attackCreatureHasFlag(victim, "magicOnly", "mmgonl", "MMGONL") {
		return false
	}
	if creatureStat(actor, "piety") >= creatureStat(victim, "piety") {
		return false
	}
	return attackRoll(0, 1) == 1
}

func eightEnchantedOnlyGlances(world InventoryWorld, weapon model.ObjectInstance, victim model.Creature) bool {
	if !attackCreatureHasFlag(victim, "magicOrEnchantedOnly", "enchantOnly", "menonl", "MENONL") {
		return false
	}
	adjustment, ok := objectIntProperty(world, weapon, "adjustment")
	return !ok || adjustment < 1
}

func nahanClassRejection(actor model.Creature) string {
	if creatureClass(actor) < legacyClassInvincible {
		return "무적이상만 쓸수 있는 기술입니다.\n"
	}
	if !nahanHasClericTraining(actor) {
		return "아직 불제자를 무적수련하지 않았습니다.\n"
	}
	return ""
}

func nahanHasClericTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SCLERIC",
		"clericTraining",
		"clericSpell",
		"clericMode",
	)
}

func nahanParticipants(world LookWorld, room model.Room, viewer LookViewer) []model.Creature {
	seen := make(map[model.CreatureID]struct{}, len(room.PlayerIDs))
	participants := make([]model.Creature, 0, len(room.PlayerIDs))
	actor, _ := world.Creature(viewer.CreatureID)
	for _, playerID := range room.PlayerIDs {
		player, ok := world.Player(playerID)
		if !ok || player.CreatureID.IsZero() || player.CreatureID == viewer.CreatureID {
			continue
		}
		if _, ok := seen[player.CreatureID]; ok {
			continue
		}
		creature, ok := world.Creature(player.CreatureID)
		if !ok || creature.RoomID != room.ID || creatureHPDead(creature) {
			continue
		}
		if !nahanParticipantVisibleLikeLegacy(actor, player, creature) {
			continue
		}
		seen[player.CreatureID] = struct{}{}
		participants = append(participants, creature)
	}
	return participants
}

func nahanParticipantVisibleLikeLegacy(actor model.Creature, player model.Player, creature model.Creature) bool {
	if creatureHasAnyFlag(actor, "PDINVI", "detectInvisible", "detectInvis") {
		return true
	}
	return !creatureHasAnyFlag(creature, "invisible", "pinvis", "PINVIS", "minvis", "MINVIS") &&
		!hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "pinvis", "PINVIS", "minvis", "MINVIS")
}

func nahanParticipantMP(participants []model.Creature) int {
	total := 0
	for _, participant := range participants {
		mp := creatureStat(participant, "mpCurrent")
		switch class := creatureClass(participant); {
		case class > legacyClassInvincible:
			total += minInt(1000, mp/2)
		case class == legacyClassInvincible:
			total += mp / 2
		default:
			total += mp
		}
	}
	return total
}

func nahanChance(actor model.Creature, victim model.Creature) int {
	chance := creatureStat(victim, "thaco") - creatureStat(actor, "thaco")
	chance += (attackCreatureLevel(actor) + 29) / 30
	chance += legacyStatBonus(creatureStat(actor, "piety")) * 2
	chance += legacyStatBonus(creatureStat(actor, "intelligence"))
	if chance > 20 {
		return 20
	}
	if chance < 3 {
		return 3
	}
	return chance
}

func nahanDamage(actor model.Creature, formationMP int, formationCount int) int {
	damage := (formationMP / 10) * formationCount
	damage += (attackRoll(1, maxInt(1, creatureStat(actor, "piety"))) + attackRoll(1, maxInt(1, creatureStat(actor, "intelligence")))) * formationCount
	if creatureClass(actor) > legacyClassInvincible {
		damage /= 2
	}
	return normalizeAttackDamage(damage)
}

func nahanMagicOnlyGlances(actor model.Creature, victim model.Creature) bool {
	if !attackCreatureHasFlag(victim, "magicOnly", "mmgonl", "MMGONL") {
		return false
	}
	if creatureStat(actor, "piety") >= creatureStat(victim, "piety") {
		return false
	}
	return attackRoll(0, 1) == 1
}

func nahanCooldownSeconds(actor model.Creature) int64 {
	interval := 25 - minInt(15, creatureStat(actor, "piety")/5+creatureStat(actor, "intelligence")/3)
	return int64(maxInt(1, interval))
}

func nahanRecordDamage(world FormationSkillsWorld, actor model.Creature, victim model.Creature, participants []model.Creature, applied int) error {
	if len(participants) == 0 {
		return world.RecordCreatureDamage(victim.ID, actor.ID, applied)
	}
	actorCredit := applied / 2
	if actorCredit > 0 {
		if err := world.RecordCreatureDamage(victim.ID, actor.ID, actorCredit); err != nil {
			return err
		}
	}
	participantCredit := 0
	if len(participants) > 0 {
		participantCredit = (applied / 2) / len(participants)
	}
	for _, participant := range participants {
		if participantCredit <= 0 {
			continue
		}
		if err := world.RecordCreatureDamage(victim.ID, participant.ID, participantCredit); err != nil {
			return err
		}
	}
	return nil
}
