package command

import (
	"fmt"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const (
	guardCooldownKey = "guard"
)

type GuardWorld interface {
	LookWorld
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
	RecordCreatureDamage(model.CreatureID, model.CreatureID, int) error
	FinalizeMonsterDeath(model.CreatureID) (bool, error)
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
}

func NewGuardHandler(world GuardWorld) Handler {
	return NewGuardHandlerWithDeathFinalizer(world, nil)
}

func NewGuardHandlerWithDeathFinalizer(world GuardWorld, finalizer AttackDeathFinalizer) Handler {
	return newGuardHandler(world, nil, finalizer)
}

func NewGuardHandlerWithRoll(world GuardWorld, roll SearchRollFunc) Handler {
	return newGuardHandler(world, roll, nil)
}

func newGuardHandler(world GuardWorld, roll SearchRollFunc, finalizer AttackDeathFinalizer) Handler {
	if roll == nil {
		roll = attackRoll
	}
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("guard: actor creature %q not found", viewer.CreatureID)
		}

		if message := guardClassRejection(actor); message != "" {
			ctx.WriteString(message)
			return StatusDefault, nil
		}

		if len(resolved.Args) < 2 {
			ctx.WriteString("\n사용법 : 엄호 <방향> <대상>\n")
			return StatusDefault, nil
		}
		if guardActorBlind(actor) {
			ctx.WriteString("당신은 눈이 멀어 엄호할 수 없습니다.\n")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, guardCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		exitName := getArg(resolved, 0)
		exit, ok := findLookTargetExitForViewer(world, viewer, room.Exits, exitName, getOrdinal(resolved, 0))
		if !ok {
			ctx.WriteString("\n" + exitName + "쪽으로는 엄호할 수 없습니다.\n")
			return StatusDefault, nil
		}
		if exitHasAnyFlag(exit, "closed", "xclosd", "xclosed") {
			ctx.WriteString("그 출구는 닫혀 있습니다.")
			return StatusDefault, nil
		}
		destination, ok := world.Room(exit.ToRoomID)
		if !ok || destination.ID == room.ID {
			ctx.WriteString("지도가 없습니다.")
			return StatusDefault, nil
		}
		if roomHasAnyFlag(destination, "onlyMarried", "marriedOnly", "ronmar", "onlyFamily", "familyOnly", "ronfml") {
			ctx.WriteString("그 방은 볼 수가 없습니다.")
			return StatusDefault, nil
		}

		targetName := getArg(resolved, 1)
		guardedPlayer, guardedCreature, ok := findGuardRemotePlayer(world, destination, viewer, targetName, getOrdinal(resolved, 1))
		if !ok {
			if player, found := findPerceptionPlayerByArgument(world, targetName); found {
				ctx.WriteString("\n" + exit.Name + "쪽에 " + redEyeTargetName(player, model.Creature{}) + krtext.Particle(redEyeTargetName(player, model.Creature{}), '1') + " 존재하지 않습니다.\n")
			} else {
				ctx.WriteString("\n" + exit.Name + "쪽에 " + targetName + krtext.Particle(targetName, '1') + " 존재하지 않습니다.\n")
			}
			return StatusDefault, nil
		}
		weapon, ok := guardWieldWeapon(world, actor)
		if !ok {
			ctx.WriteString("엄호를 사용하시려면 활종류의 무기가 필요합니다.")
			return StatusDefault, nil
		}

		targets := guardAttackableTargets(world, destination, viewer, actor)
		if len(targets) == 0 {
			guardedName := guardRemoteTargetName(guardedPlayer, guardedCreature)
			ctx.WriteString("\n" + guardedName + " 근처에 당신이 공격할 적이 없습니다.")
			return StatusDefault, nil
		}
		actorName := attackCreatureName(actor)
		guardedName := guardRemoteTargetName(guardedPlayer, guardedCreature)
		ctx.WriteString("\n당신은 활시위에 힘을 실어 " + guardedName + krtext.Particle(guardedName, '3') + " 엄호할 준비를 합니다.\n")

		chance := guardChance(actor, targets)
		if roll(1, 22) > chance {
			ctx.WriteString("당신은 엄호에 실패했습니다..\n")
			if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+exit.Name+"쪽에 있는 "+guardedName+krtext.Particle(guardedName, '3')+" 엄호하려 했지만 실패했습니다.\n"); err != nil {
				return StatusDefault, err
			}
			return StatusDefault, world.SetCreatureCooldown(actor.ID, guardCooldownKey, now, guardCooldownSeconds(actor))
		}

		count := minInt(chance/5, len(targets))
		for _, target := range targets[:count] {
			victim, ok := world.Creature(target.ID)
			if !ok || victim.RoomID != destination.ID || creatureHPDead(victim) || attackCreatureProtected(victim) {
				continue
			}
			if err := guardDamageTarget(ctx, world, room.ID, destination.ID, actor, guardedName, victim, weapon, roll, finalizer); err != nil {
				return StatusDefault, err
			}
		}
		return StatusDefault, world.SetCreatureCooldown(actor.ID, guardCooldownKey, now, guardCooldownSeconds(actor))
	}
}

func guardClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	if class < legacyClassInvincible {
		if class == legacyClassRanger && attackCreatureLevel(actor) >= 50 {
			return ""
		}
		return "포졸 레벨 50 이상만 사용할 수 있는 기술입니다.\n"
	}
	if !guardHasRangerTraining(actor) {
		return "\n포졸을 무적수련하지 않았습니다.\n"
	}
	return ""
}

func guardHasRangerTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SRANGER",
		"rangerSpell",
		"rangerTraining",
		"rangerMode",
	)
}

func guardActorBlind(actor model.Creature) bool {
	return attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND")
}

func findGuardRemotePlayer(world LookWorld, room model.Room, viewer LookViewer, prefix string, ordinal int64) (model.Player, model.Creature, bool) {
	player, ok := findAttackPlayerTarget(world, room, viewer, prefix, ordinal)
	if !ok || player.CreatureID.IsZero() {
		return model.Player{}, model.Creature{}, false
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok || creature.RoomID != room.ID {
		return model.Player{}, model.Creature{}, false
	}
	return player, creature, true
}

func guardWieldWeapon(world InventoryWorld, actor model.Creature) (model.ObjectInstance, bool) {
	weaponID := equippedObjectID(actor, "wield")
	if weaponID.IsZero() {
		return model.ObjectInstance{}, false
	}
	weapon, ok := world.Object(weaponID)
	if !ok {
		return model.ObjectInstance{}, false
	}
	return weapon, objectLegacyType(world, weapon) == legacyObjectMissile
}

func guardCooldownSeconds(actor model.Creature) int64 {
	interval := 15 - minInt(10, creatureStat(actor, "dexterity")/3)
	return int64(maxInt(5, interval))
}

func guardRemoteTargetName(player model.Player, creature model.Creature) string {
	name := attackCreatureName(creature)
	if name != "" && name != string(creature.ID) {
		return name
	}
	if name := cleanDisplayText(player.DisplayName); name != "" {
		return name
	}
	return string(player.ID)
}

func guardAttackableTargets(world GuardWorld, room model.Room, viewer LookViewer, actor model.Creature) []model.Creature {
	detectInvisible := viewerDetectsInvisible(world, viewer)
	targets := make([]model.Creature, 0, len(room.CreatureIDs))
	for _, id := range room.CreatureIDs {
		if id.IsZero() || id == actor.ID {
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
		perceptionAddEnemy(world, creature.ID, actor.ID)
	}
	return targets
}

func guardChance(actor model.Creature, targets []model.Creature) int {
	enemyTHACO := 0
	for _, target := range targets {
		enemyTHACO += 20 - creatureStat(target, "thaco")
	}
	chance := 20 - creatureStat(actor, "thaco")
	if len(targets) > 0 {
		chance -= (enemyTHACO / len(targets)) * 2
	}
	chance += attackCreatureLevel(actor) / 10
	chance += legacyStatBonus(creatureStat(actor, "dexterity")) * 5
	if chance > 20 {
		return 20
	}
	if chance < 3 {
		return 3
	}
	return chance
}

func guardDamageTarget(
	ctx *Context,
	world GuardWorld,
	actorRoomID model.RoomID,
	targetRoomID model.RoomID,
	actor model.Creature,
	guardedName string,
	victim model.Creature,
	weapon model.ObjectInstance,
	roll SearchRollFunc,
	finalizer AttackDeathFinalizer,
) error {
	victimName := attackCreatureName(victim)
	damage := roll(1, maxInt(1, creatureStat(actor, "dexterity"))) + objectDamage(world, weapon)
	damage = minInt(creatureStat(victim, "hpCurrent"), damage)
	if guardMagicOnlyGlances(actor, victim, roll) {
		ctx.WriteString("당신의 활은 " + victimName + "에게 아무런 상처도 내지 못합니다.\n")
		damage = 1
	}
	if utilityCombatEnchantedOnlyGlances(world, victim, weapon) {
		ctx.WriteString("당신의 활은 " + victimName + "의 갑옷을 뚫기엔 역부족입니다.\n")
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
	ctx.WriteString(fmt.Sprintf("\n당신은 %s%s 엄호하여 %s에게 %d의 피해를 줍니다.\n", guardedName, krtext.Particle(guardedName, '3'), victimName, applied))
	if err := roomBroadcast(ctx, actorRoomID, fmt.Sprintf("\n%s%s %s%s 엄호하여 %s에게 %d의 피해를 주었습니다.\n", actorName, krtext.Particle(actorName, '1'), guardedName, krtext.Particle(guardedName, '3'), victimName, applied)); err != nil {
		return err
	}
	if err := roomBroadcast(ctx, targetRoomID, fmt.Sprintf("\n%s%s %s%s 엄호하여 %s에게 %d의 피해를 주었습니다.\n", actorName, krtext.Particle(actorName, '1'), guardedName, krtext.Particle(guardedName, '3'), victimName, applied)); err != nil {
		return err
	}
	if !dead {
		return nil
	}
	if err := finalizeMonsterDeathWithOptionalFinalizer(ctx, world, finalizer, actor, victim); err != nil {
		return err
	}
	ctx.WriteString("\n당신의 활에 " + victimName + krtext.Particle(victimName, '1') + " 죽었습니다.")
	if err := roomBroadcast(ctx, actorRoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+guardedName+krtext.Particle(guardedName, '3')+" 엄호하여 "+victimName+krtext.Particle(victimName, '3')+" 죽였습니다."); err != nil {
		return err
	}
	return roomBroadcast(ctx, targetRoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+guardedName+krtext.Particle(guardedName, '3')+" 엄호하여 "+victimName+krtext.Particle(victimName, '3')+" 죽였습니다.")
}

func guardMagicOnlyGlances(actor model.Creature, victim model.Creature, roll SearchRollFunc) bool {
	if !attackCreatureHasFlag(victim, "magicOnly", "mmgonl", "MMGONL") {
		return false
	}
	armorThreshold := 100 - creatureStat(victim, "armor")/10
	return creatureStat(actor, "strength") < armorThreshold && roll(0, 2) == 1
}
