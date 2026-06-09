package command

import (
	"fmt"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const bashCooldownKey = "bash"

type BashWorld interface {
	KickWorld
	MoveObject(model.ObjectInstanceID, model.ObjectLocation) error
	ConsumeCreatureObjectCharge(model.ObjectInstanceID, model.CreatureID, bool) (model.ObjectInstance, bool, bool, error)
}

func NewBashHandler(world BashWorld) Handler {
	return NewBashHandlerWithDeathFinalizer(world, nil)
}

func NewBashHandlerWithDeathFinalizer(world BashWorld, finalizer AttackDeathFinalizer) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("bash: actor creature %q not found", viewer.CreatureID)
		}

		if message := bashClassRejection(actor); message != "" {
			ctx.WriteString(message)
			return StatusDefault, nil
		}

		target, ordinal := lookTarget(resolved)
		if target == "" || attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
			ctx.WriteString("누굴 공격합니까?\n")
			return StatusDefault, nil
		}

		victim, ok := findBashTarget(world, room, viewer, target, ordinal)
		if !ok {
			ctx.WriteString("그런 것은 여기 없습니다.\n")
			return StatusDefault, nil
		}

		weapon, ok := bashWieldWeapon(world, actor)
		if !ok {
			ctx.WriteString("맹공을 하시려면 도나 검종류의 무기가 필요합니다.")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, bashCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}
		if err := world.SetCreatureCooldown(actor.ID, bashCooldownKey, now, bashInitialCooldown(actor)); err != nil {
			return StatusDefault, err
		}
		if err := revealBashActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}
		if !victim.hasPlayer {
			if attackCreatureProtected(victim.creature) {
				ctx.WriteString("당신은 " + stealObjectPronoun(victim.creature) + " 해칠 수 없습니다.\n")
				return StatusDefault, nil
			}
			if bashCreatureDeflectsMundane(world, actor, victim.creature) {
				name := attackCreatureName(victim.creature)
				ctx.WriteString("당신의 무기가 " + name + "에게 아무 소용이 없는듯 합니다.\n")
				return StatusDefault, nil
			}
		}
		if victim.creature.Stats == nil {
			ctx.WriteString("그 상대는 공격할 수 없습니다.\n")
			return StatusDefault, nil
		}
		currentHP, ok := victim.creature.Stats["hpCurrent"]
		if !ok || currentHP <= 0 {
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

		if attackRoll(1, 100) > bashChance(actor, victim.creature) {
			ctx.WriteString("당신의 맹공이 실패했습니다.\n")
			_ = sendToPlayer(ctx, victim.creature.PlayerID, actorName+"이 당신에게 맹공을 가하려 합니다.\n")
			_ = broadcastRom2(ctx, world, room.ID, viewer.PlayerID, victim.creature.PlayerID, "\n"+actorName+"이 "+victimName+"에게 맹공을 가하려 합니다.")
			return StatusDefault, nil
		}

		var stopped bool
		weapon, stopped, err = bashMaybeSpendWield(ctx, world, actor, weapon)
		if err != nil || stopped {
			return StatusDefault, err
		}
		if !bashHits(actor, victim.creature) {
			ctx.WriteString("당신의 맹공이 실패했습니다.\n")
			_ = sendToPlayer(ctx, victim.creature.PlayerID, "\n"+actorName+"이 당신에게 맹공을 가하려 합니다.\n")
			_ = broadcastRom2(ctx, world, room.ID, viewer.PlayerID, victim.creature.PlayerID, "\n"+actorName+"이 "+victimName+"에게 맹공을 가하려 합니다.")
			return StatusDefault, nil
		}

		if err := bashApplyBefuddle(world, victim, attackRoll(5, 10), now); err != nil {
			return StatusDefault, err
		}
		if err := world.SetCreatureCooldown(actor.ID, bashCooldownKey, now, bashHitCooldown(actor)); err != nil {
			return StatusDefault, err
		}
		damage := bashDamage(world, actor, victim.creature, weapon)
		_, applied, dead, err := world.ApplyCreatureDamage(victim.creature.ID, damage.actual)
		if err != nil {
			return StatusDefault, err
		}
		if !victim.hasPlayer {
			if err := world.RecordCreatureDamage(victim.creature.ID, actor.ID, damage.ledger); err != nil {
				return StatusDefault, err
			}
			if gain := bashLegacyWeaponProficiencyGain(victim.creature, damage.ledger); gain > 0 {
				if _, err := incrementWeaponProficiency(world, actor, weapon, gain); err != nil {
					return StatusDefault, err
				}
			}
		}
		ctx.WriteString(fmt.Sprintf("당신의 칼을 휘둘러 %d점의 맹공을 가했습니다.\n", applied))
		_ = sendToPlayer(ctx, victim.creature.PlayerID, fmt.Sprintf("%s이 칼을 휘둘러 %d점의 맹공을 가했습니다.\n", actorName, applied))
		_ = broadcastRom2(ctx, world, room.ID, viewer.PlayerID, victim.creature.PlayerID, "\n"+actorName+"이 칼을 휘둘러 "+victimName+"에게 맹공을 가합니다.")
		if dead {
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
			_ = broadcastRom2(ctx, world, room.ID, viewer.PlayerID, victim.creature.PlayerID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+krtext.Particle(victimName, '3')+" 죽였습니다.")
			return StatusDefault, nil
		}
		return StatusDefault, nil
	}
}

func revealBashActor(ctx *Context, world revealActorWorld, roomID model.RoomID, viewer LookViewer, actor model.Creature) error {
	invisible := attackCreatureHasFlag(actor, "invisible", "pinvis", "PINVIS")
	if !viewer.PlayerID.IsZero() {
		if player, ok := world.Player(viewer.PlayerID); ok {
			if hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "pinvis", "PINVIS") {
				invisible = true
			}
			if _, err := world.UpdatePlayerTags(viewer.PlayerID, nil, []string{"invisible", "pinvis", "PINVIS"}); err != nil {
				return err
			}
		}
	}
	if _, err := world.UpdateCreatureTags(actor.ID, nil, []string{"invisible", "pinvis", "PINVIS"}); err != nil {
		return err
	}
	if updater, ok := world.(kickStatUpdater); ok && actor.Stats["PINVIS"] != 0 {
		if err := updater.SetCreatureStat(actor.ID, "PINVIS", 0); err != nil {
			return err
		}
	}
	if !invisible {
		return nil
	}
	actorName := attackCreatureName(actor)
	ctx.WriteString("당신의 모습이 서서히 드러납니다.\n")
	return roomBroadcast(ctx, roomID, "\n"+actorName+"의 모습이 서서히 드러납니다.")
}

func bashClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	if class != legacyClassFighter && class < legacyClassInvincible {
		return "검사만 쓸수 있는 기술입니다.\n"
	}
	if class >= legacyClassInvincible && !bashHasFighterTraining(actor) {
		return "\n검사를 무적수련하지 않았습니다..\n"
	}
	return ""
}

func bashHasFighterTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SFIGHTER",
		"fighterSpell",
		"fighterTraining",
		"fighterMode",
	)
}

func findBashTarget(world LookWorld, room model.Room, viewer LookViewer, prefix string, ordinal int64) (kickTarget, bool) {
	if creature, ok := findAttackCreatureTarget(world, room, viewer, prefix, ordinal); ok {
		return kickTarget{creature: creature}, true
	}
	if legacyByteLen(prefix) < 3 {
		return kickTarget{}, false
	}
	player, ok := findAttackPlayerTarget(world, room, viewer, prefix, ordinal)
	if !ok || player.CreatureID.IsZero() {
		return kickTarget{}, false
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok || creature.RoomID != room.ID || creature.ID == viewer.CreatureID {
		return kickTarget{}, false
	}
	return kickTarget{creature: creature, player: player, hasPlayer: true}, true
}

func bashCreatureDeflectsMundane(world InventoryWorld, actor model.Creature, victim model.Creature) bool {
	if attackCreatureHasFlag(victim, "magicOnly", "mmgonl", "MMGONL") {
		return true
	}
	if !attackCreatureHasFlag(victim, "magicOrEnchantedOnly", "enchantOnly", "menonl", "MENONL") {
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

func bashWieldWeapon(world InventoryWorld, actor model.Creature) (model.ObjectInstance, bool) {
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

type spendWieldWorld interface {
	InventoryWorld
	ConsumeCreatureObjectCharge(model.ObjectInstanceID, model.CreatureID, bool) (model.ObjectInstance, bool, bool, error)
	MoveObject(model.ObjectInstanceID, model.ObjectLocation) error
}

func bashMaybeSpendWield(
	ctx *Context,
	world spendWieldWorld,
	actor model.Creature,
	weapon model.ObjectInstance,
) (model.ObjectInstance, bool, error) {
	shots, hasShots := objectIntProperty(world, weapon, "shotsCurrent")
	if hasShots && creatureClass(actor) >= legacyClassInvincible && shots > 0 {
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
	ctx.WriteString(name + krtext.Particle(name, '1') + " 부서져 버렸습니다.\n")
	return weapon, true, nil
}

func bashHits(actor model.Creature, victim model.Creature) bool {
	target := creatureStat(actor, "thaco") - creatureStat(victim, "armor")/10
	return attackRoll(1, 20) >= target
}

func bashChance(actor model.Creature, victim model.Creature) int {
	chance := 50 + (((attackCreatureLevel(actor)+3)/4)-((attackCreatureLevel(victim)+3)/4))*10
	chance += legacyStatBonus(creatureStat(actor, "strength")) * 3
	chance += (legacyStatBonus(creatureStat(actor, "dexterity")) - legacyStatBonus(creatureStat(victim, "dexterity"))) * 2
	if creatureClass(actor) == legacyClassBarbarian {
		chance += 5
	}
	chance = minInt(85, chance)
	if attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
		chance = minInt(20, chance)
	}
	return chance
}

type bashDamageResult struct {
	actual int
	ledger int
}

func bashDamage(world InventoryWorld, actor model.Creature, victim model.Creature, weapon model.ObjectInstance) bashDamageResult {
	damage := normalizeAttackDamage(objectDamage(world, weapon)*3 + statsDamage(actor)*2)
	hp := creatureStat(victim, "hpCurrent")
	actual := minInt(hp/3, damage)
	if creatureClass(victim) > legacyClassCaretaker {
		actual = 1
	}
	return bashDamageResult{
		actual: actual,
		ledger: minInt(hp, damage),
	}
}

func bashLegacyWeaponProficiencyGain(victim model.Creature, rawDamage int) int {
	if rawDamage <= 0 {
		return 0
	}
	experience := creatureStat(victim, "experience")
	hpMax := creatureStat(victim, "hpMax")
	if experience <= 0 || hpMax <= 0 {
		return 0
	}
	gain := rawDamage * experience / hpMax
	if gain > experience {
		gain = experience
	}
	return gain
}

func bashInitialCooldown(actor model.Creature) int64 {
	class := creatureClass(actor)
	switch {
	case class >= legacyClassBulsa:
		return 3
	case class == legacyClassCaretaker:
		return 4
	case class == legacyClassFighter:
		return 3
	default:
		return 5
	}
}

func bashHitCooldown(actor model.Creature) int64 {
	return int64(8 - minInt(5, creatureStat(actor, "dexterity")/5))
}

func bashApplyBefuddle(world BashWorld, victim kickTarget, delay int, nowUnix int64) error {
	if err := world.SetCreatureCooldown(victim.creature.ID, "befuddled", nowUnix, int64(delay)); err != nil {
		return err
	}
	if victim.hasPlayer {
		return nil
	}
	if _, err := world.UpdateCreatureTags(victim.creature.ID, []string{"befuddled", "MBEFUD"}, nil); err != nil {
		return err
	}
	return nil
}
