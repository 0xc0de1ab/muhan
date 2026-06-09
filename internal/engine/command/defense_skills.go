package command

import (
	"fmt"
	"math/rand"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const (
	reflectCooldownKey            = "reflect"
	reflectSuccessCooldownSeconds = int64(600)
	reflectFailureCooldownSeconds = int64(300)
	reflectStatusDurationSeconds  = int64(500)
	shadowCooldownKey             = "shadow"
	shadowNormalCloneCount        = 6
	shadowInvincibleCloneCount    = 8
	shadowCaretakerPlusCloneCount = 10
)

var (
	reflectStatusTags = []string{"PREFLECT", "reflect", "reflection"}
)

type ReflectWorld interface {
	LookWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
	SetEffectExpiration(model.CreatureID, string, int64)
	RecalculateAC(model.CreatureID) error
	RecalculateTHACO(model.CreatureID) error
}

type ShadowWorld interface {
	BashWorld
	SetCreatureStat(model.CreatureID, string, int) error
}

func NewReflectHandler(world ReflectWorld, rng SearchRollFunc) Handler {
	if rng == nil {
		rng = defenseDefaultRoll
	}
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("reflect: actor creature %q not found", viewer.CreatureID)
		}
		var player model.Player
		if !viewer.PlayerID.IsZero() {
			player, _ = world.Player(viewer.PlayerID)
		}

		if reject := reflectClassRejection(actor); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}
		if reflectStatusActive(player, actor) {
			ctx.WriteString("당신은 이미 반탄강기를 사용중입니다.\n")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		remaining, used, err := world.UseCreatureCooldown(actor.ID, reflectCooldownKey, now, 0)
		if err != nil {
			return StatusDefault, err
		}
		if !used {
			ctx.WriteString(renderReflectWait(remaining))
			return StatusDefault, nil
		}

		actorName := attackCreatureName(actor)
		if rng(1, 100) > reflectChance(actor) {
			if err := world.SetCreatureCooldown(actor.ID, reflectCooldownKey, now, reflectFailureCooldownSeconds); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("반탄강기를 사용하는데 실패했습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 반탄강기를 시도합니다.\n")
		}

		if err := reflectApplySuccess(world, player, actor); err != nil {
			return StatusDefault, err
		}
		world.SetEffectExpiration(actor.ID, "PREFLECT", now+reflectStatusDurationSeconds)
		if err := world.SetCreatureCooldown(actor.ID, reflectCooldownKey, now, reflectSuccessCooldownSeconds); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("\n주변의 기를 끌어모아 강철같은 보호막으로 몸을 보호합니다.\n")
		ctx.WriteString("당신의 몸에 반탄강기가 서립니다.\n")
		return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 반탄강기를 사용합니다.\n")
	}
}

func NewShadowHandler(world ShadowWorld, rng SearchRollFunc) Handler {
	return NewShadowHandlerWithDeathFinalizer(world, rng, nil)
}

func NewShadowHandlerWithDeathFinalizer(world ShadowWorld, rng SearchRollFunc, finalizer AttackDeathFinalizer) Handler {
	if rng == nil {
		rng = defenseDefaultRoll
	}
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("shadow: actor creature %q not found", viewer.CreatureID)
		}
		target, ordinal := lookTarget(resolved)
		if target == "" || attackCreatureHasFlag(actor, "blind", "blinded", "PBLIND") {
			ctx.WriteString("누굴 공격합니까?\n")
			return StatusDefault, nil
		}
		if reject := shadowClassRejection(actor); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		remaining, used, err := world.UseCreatureCooldown(actor.ID, shadowCooldownKey, now, 0)
		if err != nil {
			return StatusDefault, err
		}
		if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		weapon, ok := shadowWieldWeapon(world, actor)
		if !ok {
			ctx.WriteString("분신술을 사용하시려면 날카로운 무기가 필요합니다.\n")
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
		if err := defenseRevealActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}
		if !victim.hasPlayer {
			if attackCreatureProtected(victim.creature) {
				ctx.WriteString("당신은 그 상대를 해칠 수 없습니다.\n")
				return StatusDefault, nil
			}
			if shadowMagicOnlyDeflects(actor, victim.creature, rng) {
				victimName := attackCreatureName(victim.creature)
				ctx.WriteString("당신의 분신술이 " + victimName + "에게 아무 소용이 없는듯 합니다.\n")
				return StatusDefault, nil
			}
			if shadowEnchantOnlyDeflects(world, weapon, victim.creature, rng) {
				victimName := attackCreatureName(victim.creature)
				ctx.WriteString("당신의 무기가 " + victimName + "에게 아무 소용이 없는듯 합니다.\n")
				return StatusDefault, nil
			}
		}
		if victim.creature.Stats == nil || creatureStat(victim.creature, "hpCurrent") <= 0 {
			ctx.WriteString("그 상대는 공격할 수 없습니다.\n")
			return StatusDefault, nil
		}
		if !victim.hasPlayer {
			advancedCombatPrimeMonsterTarget(world, victim.creature.ID, actor.ID)
		}

		actorName := attackCreatureName(actor)
		victimName := attackCreatureName(victim.creature)
		ctx.WriteString("\n\"이것이 바로 최고의 검술 분신술이다~ 나의 분신을 그 누구도 피하리라!\"\n")
		ctx.WriteString("당신은 온 몸의 기를 분산시킵니다.\n\n")

		if rng(1, 100) > shadowChance(actor, victim.creature) {
			if err := world.SetCreatureCooldown(actor.ID, shadowCooldownKey, now, shadowFailureCooldownSeconds(actor)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("당신은 분신술에 실패했습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+"에게 분신술을 시도합니다.\n")
		}

		weapon, stopped, err := shadowMaybeSpendWield(ctx, world, actor, weapon)
		if err != nil || stopped {
			return StatusDefault, err
		}
		if !shadowHits(actor, victim.creature, rng) {
			if err := world.SetCreatureCooldown(actor.ID, shadowCooldownKey, now, shadowFailureCooldownSeconds(actor)); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("당신은 분신술에 실패했습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+"에게 분신술을 시도합니다.\n")
		}

		if err := world.SetCreatureCooldown(actor.ID, shadowCooldownKey, now, shadowSuccessCooldownSeconds(actor)); err != nil {
			return StatusDefault, err
		}
		hits, total, dead, err := applyShadowDamage(ctx, world, room.ID, actor, victim, weapon, rng)
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(fmt.Sprintf("당신은 총 %d연타 %d점의 분신술 공격을 %s에게 가했습니다.\n", hits, total, victimName))
		if dead {
			if !victim.hasPlayer {
				if err := finalizeMonsterDeathWithOptionalFinalizer(ctx, world, finalizer, actor, victim.creature); err != nil {
					return StatusDefault, err
				}
			}
			ctx.WriteString("당신은 분신들로 " + victimName + krtext.Particle(victimName, '3') + " 죽였습니다.\n")
		}
		return StatusDefault, nil
	}
}

func defenseDefaultRoll(min int, max int) int {
	if max <= min {
		return min
	}
	return min + rand.Intn(max-min+1)
}

func reflectClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	level := attackCreatureLevel(actor)
	if class < legacyClassInvincible && !(class == legacyClassFighter && level >= 50) {
		return "검사 레벨 50이상만 쓸수 있는 기술입니다.\n"
	}
	if class >= legacyClassInvincible && !reflectHasFighterTraining(actor) {
		return "\n검사를 무적수련하지 않았습니다..\n"
	}
	return ""
}

func reflectHasFighterTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SFIGHTER",
		"fighterSpell",
		"fighterTraining",
		"fighterMode",
	)
}

func reflectStatusActive(player model.Player, actor model.Creature) bool {
	return statusEffectActive(player, actor, reflectStatusTags...)
}

func reflectChance(actor model.Creature) int {
	chance := ((attackCreatureLevel(actor) + 3) / 50 * 2) + legacyStatBonus(20-creatureStat(actor, "thaco"))
	return minInt(20, chance)
}

func renderReflectWait(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%d분 %02d초 기다리세요.\n", seconds/60, seconds%60)
}

func reflectApplySuccess(world ReflectWorld, player model.Player, actor model.Creature) error {
	if _, err := world.UpdateCreatureTags(actor.ID, reflectStatusTags, nil); err != nil {
		return err
	}
	if !player.ID.IsZero() {
		if _, err := world.UpdatePlayerTags(player.ID, reflectStatusTags, nil); err != nil {
			return err
		}
	}
	if err := world.RecalculateAC(actor.ID); err != nil {
		return err
	}
	if err := world.RecalculateTHACO(actor.ID); err != nil {
		return err
	}
	return nil
}

func shadowClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	level := attackCreatureLevel(actor)
	if class < legacyClassInvincible && !(class == legacyClassAssassin && level >= 50) {
		return "자객 레벨 50이상만 쓸수 있는 기술입니다.\n"
	}
	if class >= legacyClassInvincible && !shadowHasAssassinTraining(actor) {
		return "\n자객을 무적수련하지 않았습니다..\n"
	}
	return ""
}

func shadowHasAssassinTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SASSASSIN",
		"assassinSpell",
		"assassinTraining",
		"assassinMode",
	)
}

func shadowWieldWeapon(world InventoryWorld, actor model.Creature) (model.ObjectInstance, bool) {
	weaponID := equippedObjectID(actor, "wield")
	if weaponID.IsZero() {
		return model.ObjectInstance{}, false
	}
	weapon, ok := world.Object(weaponID)
	if !ok {
		return model.ObjectInstance{}, false
	}
	return weapon, objectLegacyType(world, weapon) == legacyObjectSharp
}

func shadowChance(actor model.Creature, victim model.Creature) int {
	chance := 50 + ((((attackCreatureLevel(actor) + 3) / 4) - ((attackCreatureLevel(victim) + 3) / 4)) * 2)
	chance += legacyStatBonus(creatureStat(actor, "intelligence")) * 2
	chance += legacyStatBonus(creatureStat(actor, "dexterity")) * 7
	if attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
		chance = minInt(20, chance)
	}
	return minInt(90, chance)
}

func shadowSuccessCooldownSeconds(actor model.Creature) int64 {
	return int64(maxInt(1, 8-minInt(3, creatureStat(actor, "dexterity")/10)))
}

func shadowFailureCooldownSeconds(actor model.Creature) int64 {
	return int64(maxInt(1, 8-minInt(5, creatureStat(actor, "dexterity")/5)))
}

func shadowCloneCount(actor model.Creature) int {
	class := creatureClass(actor)
	count := shadowNormalCloneCount
	switch {
	case class > legacyClassInvincible:
		count = shadowCaretakerPlusCloneCount
	case class == legacyClassInvincible && shadowHasAssassinTraining(actor):
		count = shadowInvincibleCloneCount
	}
	if attackCreatureHasFlag(actor, "YELLOWI", "yellowI") {
		count++
	}
	if class == legacyClassBulsa {
		count++
	}
	return count
}

func shadowMagicOnlyDeflects(actor model.Creature, victim model.Creature, rng SearchRollFunc) bool {
	if !attackCreatureHasFlag(victim, "magicOnly", "mmgonl", "MMGONL") {
		return false
	}
	if creatureStat(actor, "piety") >= creatureStat(victim, "piety") {
		return false
	}
	return rng(0, 1) == 1
}

func shadowEnchantOnlyDeflects(world InventoryWorld, weapon model.ObjectInstance, victim model.Creature, rng SearchRollFunc) bool {
	if !attackCreatureHasFlag(victim, "magicOrEnchantedOnly", "enchantOnly", "menonl", "MENONL") {
		return false
	}
	if rng(0, 1) == 0 {
		return false
	}
	adjustment, ok := objectIntProperty(world, weapon, "adjustment")
	return !ok || adjustment < 1
}

func shadowHits(actor model.Creature, victim model.Creature, rng SearchRollFunc) bool {
	target := creatureStat(actor, "thaco") - creatureStat(victim, "armor")/10
	return rng(1, 20) >= target
}

func shadowMaybeSpendWield(
	ctx *Context,
	world ShadowWorld,
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

func applyShadowDamage(
	ctx *Context,
	world ShadowWorld,
	roomID model.RoomID,
	actor model.Creature,
	victim kickTarget,
	weapon model.ObjectInstance,
	rng SearchRollFunc,
) (int, int, bool, error) {
	clones := shadowCloneCount(actor)
	hits := 0
	total := 0
	dead := false
	currentVictim := victim.creature
	actorName := attackCreatureName(actor)
	victimName := attackCreatureName(victim.creature)
	ctx.WriteString(fmt.Sprintf("당신은 분신 %d개로 적을 공격합니다.\n", clones))
	for i := 0; i < clones; i++ {
		damage := shadowStrikeDamage(world, actor, weapon, rng)
		updated, applied, hitDead, err := world.ApplyCreatureDamage(currentVictim.ID, damage)
		if err != nil {
			return hits, total, dead, err
		}
		hits++
		total += applied
		currentVictim = updated
		ctx.WriteString(fmt.Sprintf("당신은 %2d번째 분신으로 %s에게 %d점의 피해를 입혔습니다.\n", i+1, victimName, applied))
		if err := roomBroadcast(ctx, roomID, fmt.Sprintf("\n%s%s %s에게 분신술로 %d점의 피해를 입혔습니다.\n", actorName, krtext.Particle(actorName, '1'), victimName, applied)); err != nil {
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

func shadowStrikeDamage(world InventoryWorld, actor model.Creature, weapon model.ObjectInstance, rng SearchRollFunc) int {
	damage := shadowCreatureDice(actor, rng)*rng(1, 2) + shadowObjectDice(world, weapon, rng)*rng(1, 2)
	damage += (attackCreatureLevel(actor) - 50) / 10
	damage /= 2
	if damage < 0 {
		return 0
	}
	return damage
}

func shadowCreatureDice(creature model.Creature, rng SearchRollFunc) int {
	return shadowRollDice(creatureStat(creature, "nDice"), creatureStat(creature, "sDice"), creatureStat(creature, "pDice"), rng)
}

func shadowObjectDice(world InventoryWorld, object model.ObjectInstance, rng SearchRollFunc) int {
	nDice, _ := objectIntProperty(world, object, "nDice")
	sDice, _ := objectIntProperty(world, object, "sDice")
	pDice, _ := objectIntProperty(world, object, "pDice")
	return shadowRollDice(nDice, sDice, pDice, rng)
}

func shadowRollDice(nDice int, sDice int, pDice int, rng SearchRollFunc) int {
	if nDice < 0 {
		nDice = 0
	}
	damage := pDice
	if sDice > 0 {
		for i := 0; i < nDice; i++ {
			damage += rng(1, sDice)
		}
	}
	if damage < 0 {
		return 0
	}
	return damage
}

func defenseRevealActor(ctx *Context, world ShadowWorld, roomID model.RoomID, viewer LookViewer, actor model.Creature) error {
	invisible := attackCreatureHasFlag(actor, "invisible", "pinvis", "PINVIS")
	if !viewer.PlayerID.IsZero() {
		if player, ok := world.Player(viewer.PlayerID); ok && hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "pinvis", "PINVIS") {
			invisible = true
		}
		if _, err := world.UpdatePlayerTags(viewer.PlayerID, nil, []string{"hidden", "phiddn", "PHIDDN", "invisible", "pinvis", "PINVIS"}); err != nil {
			return err
		}
	}
	if _, err := world.UpdateCreatureTags(actor.ID, nil, []string{"hidden", "phiddn", "PHIDDN", "invisible", "pinvis", "PINVIS"}); err != nil {
		return err
	}
	for _, key := range []string{"PHIDDN", "PINVIS"} {
		if actor.Stats[key] != 0 {
			if err := world.SetCreatureStat(actor.ID, key, 0); err != nil {
				return err
			}
		}
	}
	if !invisible {
		return nil
	}
	actorName := attackCreatureName(actor)
	ctx.WriteString("당신의 모습이 서서히 드러납니다.\n")
	return roomBroadcast(ctx, roomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 모습이 서서히 드러납니다.\n")
}
