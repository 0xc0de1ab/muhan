package command

import (
	"fmt"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

type BackstabWorld interface {
	LookWorld
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
	RecordCreatureDamage(model.CreatureID, model.CreatureID, int) error
	FinalizeMonsterDeath(model.CreatureID) (bool, error)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
}

func NewBackstabHandler(world BackstabWorld) Handler {
	return NewBackstabHandlerWithDeathFinalizer(world, nil)
}

func NewBackstabHandlerWithDeathFinalizer(world BackstabWorld, finalizer AttackDeathFinalizer) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("backstab: actor creature %q not found", viewer.CreatureID)
		}

		if message := backstabClassRejection(actor); message != "" {
			ctx.WriteString(message)
			return StatusDefault, nil
		}
		target, ordinal := lookTarget(resolved)
		if target == "" || attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
			ctx.WriteString("누구를 기습하시려구요?")
			return StatusDefault, nil
		}
		weapon, ok := backstabWieldedWeapon(world, actor)
		if !ok {
			ctx.WriteString("기습을 하시려면 도나 검종류의 무기가 필요합니다.")
			return StatusDefault, nil
		}
		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, "attack", now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		victim, ok := findKickTarget(world, room, viewer, target, ordinal)
		if !ok {
			ctx.WriteString("그런건 여기 없어요.")
			return StatusDefault, nil
		}
		if !victim.hasPlayer && sneakMonsterTargetsActor(world, victim.creature.ID, viewer.PlayerID, actor) {
			ctx.WriteString("당신은 " + stealSubjectPronoun(victim.creature) + "와 싸울 수 없습니다.")
			return StatusDefault, nil
		}
		if victim.hasPlayer {
			gate := kickPlayerCombatGate(world, room, actor, viewer.PlayerID, victim.player, victim.creature)
			if !gate.Allowed {
				ctx.WriteString(gate.Message + "\n")
				return StatusDefault, nil
			}
			if backstabTargetCharmListBlocksActor(world, actor, viewer.PlayerID, victim.player, victim.creature) {
				ctx.WriteString(backstabCharmedPlayerRefusal(victim.creature))
				return StatusDefault, nil
			}
		}
		if backstabActorCharmListBlocksTarget(world, actor, viewer.PlayerID, victim) {
			ctx.WriteString(backstabCharmedTargetRefusal(victim.creature))
			return StatusDefault, nil
		}

		wasHidden := backstabActorHidden(world, viewer, actor)
		if err := revealKickActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}

		if !victim.hasPlayer {
			if attackCreatureProtected(victim.creature) {
				ctx.WriteString("당신은 " + stealObjectPronoun(victim.creature) + " 해칠수 없습니다.")
				return StatusDefault, nil
			}
			if attackCreatureDeflectsMundane(world, actor, victim.creature) {
				name := attackCreatureName(victim.creature)
				ctx.WriteString("당신의 무기는 " + name + "에게 아무 소용이 없습니다.")
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
		baseCooldown := backstabCooldownBase(actor)
		if err := world.SetCreatureCooldown(actor.ID, "attack", now, baseCooldown); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("당신은 " + victimName + "의 뒤로 몰래 기어가 옆구리를 쿡~ 찌릅니다.")
		_ = sendToPlayer(ctx, victim.creature.PlayerID, actorName+"이 당신의 옆구리를 쿡~ 찌릅니다.")
		_ = broadcastRom2(ctx, world, room.ID, viewer.PlayerID, victim.creature.PlayerID, "\n"+actorName+"이 "+victimName+"의 옆구리를 쿡~ 찌릅니다!")

		// Increment proficiency upon use!
		incrementAmount := 1 + legacyStatBonus(creatureStat(actor, "dexterity"))/2
		if incrementAmount < 1 {
			incrementAmount = 1
		}
		actor, _ = incrementCreaturePropertyProficiency(world, actor, "proficiency/backstab", incrementAmount)
		actor, _ = incrementWeaponProficiency(world, actor, weapon, incrementAmount)

		if !wasHidden || !backstabLands(actor, victim.creature, getWeaponProficiency(world, actor, weapon)) {
			if err := world.SetCreatureCooldown(actor.ID, "attack", now, baseCooldown*3); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("\n허공을 쳤습니다.")
			_ = roomBroadcast(ctx, room.ID, "\n"+lookCreaturePronoun(actor)+"가 기습을 시도했지만 상대방이 피했습니다.")
			return StatusDefault, nil
		}

		damage := backstabDamage(world, actor, victim.creature, weapon)
		_, applied, dead, err := world.ApplyCreatureDamage(victim.creature.ID, damage)
		if err != nil {
			return StatusDefault, err
		}
		if !victim.hasPlayer {
			if err := world.RecordCreatureDamage(victim.creature.ID, actor.ID, applied); err != nil {
				return StatusDefault, err
			}
		}

		ctx.WriteString(fmt.Sprintf("\n당신은 %d 만큼의 피해를 주었습니다.", applied))
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
			ctx.WriteString("\n당신은 " + victimName + krtext.Particle(victimName, '3') + " 죽였습니다.")
			_ = broadcastRom2(ctx, world, room.ID, viewer.PlayerID, victim.creature.PlayerID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+krtext.Particle(victimName, '3')+" 죽였습니다.")
		}
		return StatusDefault, nil
	}
}

func backstabTargetCharmListBlocksActor(
	world LookWorld,
	actor model.Creature,
	actorPlayerID model.PlayerID,
	victimPlayer model.Player,
	victim model.Creature,
) bool {
	return kickActorHasPCharm(world, actor, actorPlayerID) &&
		kickCharmListContainsCreature(world, victimPlayer, victim, actor, actorPlayerID)
}

func backstabActorCharmListBlocksTarget(world LookWorld, actor model.Creature, actorPlayerID model.PlayerID, victim kickTarget) bool {
	if !kickActorHasPCharm(world, actor, actorPlayerID) {
		return false
	}
	actorPlayer := model.Player{}
	if !actorPlayerID.IsZero() {
		actorPlayer, _ = world.Player(actorPlayerID)
	}
	return kickCharmListContainsCreature(world, actorPlayer, actor, victim.creature, victim.creature.PlayerID)
}

func backstabCharmedPlayerRefusal(victim model.Creature) string {
	name := attackCreatureName(victim)
	return "당신은 " + name + krtext.Particle(name, '3') + " 너무 사랑해 그를 해칠 수 없습니다."
}

func backstabCharmedTargetRefusal(victim model.Creature) string {
	name := attackCreatureName(victim)
	return "당신은 " + name + krtext.Particle(name, '3') + " 너무 사랑해서 그렇게 할 용기가 나지 않는군요."
}

func backstabClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	if class != model.ClassThief && class != model.ClassAssassin && class < model.ClassInvincible {
		return "도둑이나 자객만 사용할 수 있는 기술입니다.\n"
	}
	if class >= model.ClassInvincible && !backstabHasThiefOrAssassinTraining(actor) {
		return "\n도둑이나 자객을 무적수련하지 않았습니다..\n"
	}
	return ""
}

func backstabHasThiefOrAssassinTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"STHIEF",
		"SASSASSIN",
		"thiefTraining",
		"assassinTraining",
		"thiefSpell",
		"assassinSpell",
		"thiefMode",
		"assassinMode",
	)
}

func backstabWieldedWeapon(world InventoryWorld, actor model.Creature) (model.ObjectInstance, bool) {
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

func backstabActorHidden(world LookWorld, viewer LookViewer, actor model.Creature) bool {
	if attackCreatureHasFlag(actor, "hidden", "phiddn", "PHIDDN") {
		return true
	}
	if viewer.PlayerID.IsZero() {
		return false
	}
	player, ok := world.Player(viewer.PlayerID)
	return ok && hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "phiddn", "PHIDDN")
}

func backstabCooldownBase(actor model.Creature) int64 {
	if creatureStat(actor, "dexterity") > 18 {
		return 1
	}
	return 2
}

func backstabLands(actor model.Creature, victim model.Creature, weaponProf int) bool {
	backstabProf := getCreatureProficiency(actor, "proficiency/backstab")
	dexBonus := legacyStatBonus(creatureStat(actor, "dexterity"))
	levelDiff := (attackCreatureLevel(actor) - attackCreatureLevel(victim)) / 4
	target := (creatureStat(actor, "thaco") - dexBonus - levelDiff - backstabProf/20 - weaponProf/20) - creatureStat(victim, "armor")/10 + 2
	return attackRoll(1, 20) >= target
}

func backstabDamage(world InventoryWorld, actor model.Creature, victim model.Creature, weapon model.ObjectInstance) int {
	if creatureClass(victim) > model.ClassCaretaker {
		return 1
	}
	backstabProf := getCreatureProficiency(actor, "proficiency/backstab")
	weaponProf := getWeaponProficiency(world, actor, weapon)

	damage := objectDamage(world, weapon)
	if damage < 1 {
		damage = 1
	}
	damage += backstabProf/10 + weaponProf/10
	if creatureClass(actor) == model.ClassThief {
		multiplier := attackRoll(20, 35) / 10
		if multiplier < 1 {
			multiplier = 1
		}
		return damage * multiplier
	}
	return damage * 5
}
