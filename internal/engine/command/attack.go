package command

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type AttackWorld interface {
	LookWorld
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
	RecordCreatureDamage(model.CreatureID, model.CreatureID, int) error
	FinalizeMonsterDeath(model.CreatureID) (bool, error)
	MoveObject(model.ObjectInstanceID, model.ObjectLocation) error
	ConsumeCreatureObjectCharge(model.ObjectInstanceID, model.CreatureID, bool) (model.ObjectInstance, bool, bool, error)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
}

type AttackDeathFinalizer func(*Context, model.Creature, model.Creature) error

func NewAttackHandler(world AttackWorld) Handler {
	return NewAttackHandlerWithDeathFinalizer(world, nil)
}

func NewAttackHandlerWithDeathFinalizer(world AttackWorld, finalizer AttackDeathFinalizer) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		attacker, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("attack: attacker creature %q not found", viewer.CreatureID)
		}
		attackerClass := creatureClass(attacker)
		if attackerClass == 0 {
			ctx.WriteString("당신은 전투가 금지된 직업을 갖고 있습니다.")
			return StatusDefault, nil
		}
		now := timeNow().Unix()
		if remaining, ready, err := attackCooldownReady(world, attacker.ID, now); err != nil {
			return StatusDefault, err
		} else if !ready {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		target, ordinal := lookTarget(resolved)
		if target == "" || attackCreatureHasFlag(attacker, "blind", "pblind", "PBLIND") {
			ctx.WriteString("누구를 공격하시려구요?")
			return StatusDefault, nil
		}
		if target == "나" {
			ctx.WriteString("자기 자신은 공격할 수 없습니다.\n")
			return StatusDefault, nil
		}
		if (attackerClass == model.ClassPaladin || attackerClass >= model.ClassInvincible) &&
			attackActorAlreadyFighting(world, room, viewer, attacker) {
			ctx.WriteString("당신은 지금 싸우고 있잖아요!")
			return StatusDefault, nil
		}

		victim, ok := findAttackCreatureTarget(world, room, viewer, target, ordinal)
		if !ok {
			if attackTargetMatchesSelf(world, viewer, attacker, target) {
				ctx.WriteString("자기 자신은 공격할 수 없습니다.\n")
				return StatusDefault, nil
			}
			if player, ok := findAttackPlayerTarget(world, room, viewer, target, ordinal); ok {
				victim, hasVictim := attackPlayerCreature(world, player)
				if !hasVictim {
					ctx.WriteString("그 상대는 공격할 수 없습니다.\n")
					return StatusDefault, nil
				}
				if attackPlayerCharmBlocks(world, attacker, viewer.PlayerID, player, victim) {
					ctx.WriteString(attackCharmedPlayerRefusal(victim))
					return StatusDefault, nil
				}
				if err := attackRemovePlayerCharmReference(world, attacker, viewer.PlayerID, player, victim); err != nil {
					return StatusDefault, err
				}
				if err := revealAttackActor(ctx, world, room.ID, viewer, attacker); err != nil {
					return StatusDefault, err
				}
				if err := attackSetCooldown(world, attacker.ID, now, 1); err != nil {
					return StatusDefault, err
				}
				if gate := attackPlayerCombatGate(world, room, attacker, viewer.PlayerID, player, victim); !gate.Allowed {
					ctx.WriteString(gate.Message + "\n")
					return StatusDefault, nil
				}
				if err := attackSetCooldown(world, attacker.ID, now, 4); err != nil {
					return StatusDefault, err
				}
				if attackCreatureProtected(victim) {
					ctx.WriteString("그 상대는 공격할 수 없습니다.\n")
					return StatusDefault, nil
				}
				currentHP, ok := victim.Stats["hpCurrent"]
				if victim.Stats == nil || !ok {
					ctx.WriteString("그 상대는 공격할 수 없습니다.\n")
					return StatusDefault, nil
				}
				name := attackCreatureName(victim)
				if currentHP <= 0 {
					ctx.WriteString(name + krtext.Particle(name, '1') + " 이미 쓰러져 있습니다.\n")
					return StatusDefault, nil
				}
				count := attackMultiAttackCount(attacker)
				for i := 0; i < count; i++ {
					dead, stopped, err := attackOnePlayerRound(ctx, world, attacker, victim, name)
					if err != nil || stopped {
						return StatusDefault, err
					}
					if dead {
						return StatusDefault, nil
					}
				}
				return StatusDefault, nil
			}
			ctx.WriteString("그런것은 보이지 않습니다.")
			return StatusDefault, nil
		}

		if err := revealAttackActor(ctx, world, room.ID, viewer, attacker); err != nil {
			return StatusDefault, err
		}
		if err := attackSetCooldown(world, attacker.ID, now, 1); err != nil {
			return StatusDefault, err
		}

		if attackCreatureProtected(victim) {
			ctx.WriteString("그 상대는 공격할 수 없습니다.\n")
			return StatusDefault, nil
		}
		currentHP, ok := victim.Stats["hpCurrent"]
		if victim.Stats == nil || !ok {
			ctx.WriteString("그 상대는 공격할 수 없습니다.\n")
			return StatusDefault, nil
		}
		name := attackCreatureName(victim)
		if currentHP <= 0 {
			ctx.WriteString(name + krtext.Particle(name, '1') + " 이미 쓰러져 있습니다.\n")
			return StatusDefault, nil
		}

		if adder, ok := world.(interface {
			AddEnemy(attacker, defender model.CreatureID) (bool, error)
		}); ok {
			wasNew, _ := adder.AddEnemy(victim.ID, attacker.ID)
			_ = wasNew // broadcast of initial aggro gain handled by attack messaging
		}
		_, _ = world.UpdateCreatureTags(victim.ID, []string{"was_attacked"}, nil)

		if attackCreatureDeflectsMundane(world, attacker, victim) {
			ctx.WriteString("그 상대에게는 아무 소용이 없습니다.\n")
			return StatusDefault, nil
		}

		count := attackMultiAttackCount(attacker)
		for i := 0; i < count; i++ {
			dead, stopped, err := attackOneCreatureRound(ctx, world, finalizer, attacker, victim, name)
			if err != nil || stopped {
				return StatusDefault, err
			}
			if dead {
				return StatusDefault, nil
			}
		}
		return StatusDefault, nil
	}
}

func attackCooldownReady(world AttackWorld, creatureID model.CreatureID, now int64) (int64, bool, error) {
	cooldowns, ok := world.(interface {
		UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	})
	if !ok {
		return 0, true, nil
	}
	return cooldowns.UseCreatureCooldown(creatureID, "attack", now, 0)
}

func attackSetCooldown(world AttackWorld, creatureID model.CreatureID, now int64, seconds int64) error {
	cooldowns, ok := world.(interface {
		SetCreatureCooldown(model.CreatureID, string, int64, int64) error
	})
	if !ok {
		return nil
	}
	return cooldowns.SetCreatureCooldown(creatureID, "attack", now, seconds)
}

func attackPlayerCharmBlocks(
	world LookWorld,
	attacker model.Creature,
	attackerPlayerID model.PlayerID,
	victimPlayer model.Player,
	victim model.Creature,
) bool {
	return kickActorHasPCharm(world, attacker, attackerPlayerID) &&
		kickCharmListContainsCreature(world, victimPlayer, victim, attacker, attackerPlayerID)
}

func attackCharmedPlayerRefusal(victim model.Creature) string {
	name := attackCreatureName(victim)
	return "당신은 " + name + krtext.Particle(name, '3') + " 너무 사랑해서 그렇게 못합니다."
}

func attackRemovePlayerCharmReference(
	world AttackWorld,
	attacker model.Creature,
	attackerPlayerID model.PlayerID,
	victimPlayer model.Player,
	victim model.Creature,
) error {
	remove := attackCharmReferenceTags(world, attacker, attackerPlayerID)
	if len(remove) == 0 {
		return nil
	}
	if _, err := world.UpdateCreatureTags(victim.ID, nil, remove); err != nil {
		return err
	}
	if !victimPlayer.ID.IsZero() {
		if _, err := world.UpdatePlayerTags(victimPlayer.ID, nil, remove); err != nil {
			return err
		}
	}
	return nil
}

func attackCharmReferenceTags(world LookWorld, creature model.Creature, playerID model.PlayerID) []string {
	seen := map[string]struct{}{}
	var tags []string
	add := func(tag string) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		tags = append(tags, tag)
	}
	if name := strings.TrimSpace(creature.DisplayName); name != "" {
		add("charm:" + name)
	}
	if !playerID.IsZero() {
		if player, ok := world.Player(playerID); ok {
			if name := strings.TrimSpace(player.DisplayName); name != "" {
				add("charm:" + name)
			}
		}
	}
	if !creature.ID.IsZero() {
		add("charmID:" + string(creature.ID))
	}
	return tags
}

func findAttackCreatureTarget(
	world LookWorld,
	room model.Room,
	viewer LookViewer,
	prefix string,
	ordinal int64,
) (model.Creature, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return model.Creature{}, false
	}
	if ordinal < 1 {
		ordinal = 1
	}

	detectInvisible := viewerDetectsInvisible(world, viewer)
	var seen int64
	for _, id := range room.CreatureIDs {
		if id.IsZero() || id == viewer.CreatureID {
			continue
		}
		creature, ok := world.Creature(id)
		if !ok || creature.RoomID != room.ID || attackCreatureIsPlayer(creature) {
			continue
		}
		if !creatureVisibleInRoomLook(creature, viewer, detectInvisible) || !lookCreatureMatches(creature, prefix) {
			continue
		}
		seen++
		if seen == ordinal {
			return creature, true
		}
	}
	return model.Creature{}, false
}

func findAttackPlayerTarget(
	world LookWorld,
	room model.Room,
	viewer LookViewer,
	prefix string,
	ordinal int64,
) (model.Player, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return model.Player{}, false
	}
	if ordinal < 1 {
		ordinal = 1
	}

	detectInvisible := viewerDetectsInvisible(world, viewer)
	var seen int64
	for _, id := range room.PlayerIDs {
		if id.IsZero() || id == viewer.PlayerID || !viewerAllowsPlayer(viewer, id) {
			continue
		}
		player, ok := world.Player(id)
		if !ok || player.RoomID != room.ID || !playerVisibleInRoomLook(world, player, viewer, detectInvisible) {
			continue
		}
		if !lookPlayerMatches(world, player, prefix) {
			continue
		}
		seen++
		if seen == ordinal {
			return player, true
		}
	}
	return model.Player{}, false
}

func attackTargetMatchesSelf(world LookWorld, viewer LookViewer, attacker model.Creature, prefix string) bool {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return false
	}
	if lookCreatureMatches(attacker, prefix) {
		return true
	}
	if viewer.PlayerID.IsZero() {
		return false
	}
	player, ok := world.Player(viewer.PlayerID)
	return ok && lookPlayerMatches(world, player, prefix)
}

func attackCreatureIsPlayer(creature model.Creature) bool {
	return creature.Kind == model.CreatureKindPlayer || !creature.PlayerID.IsZero()
}

func attackCreatureProtected(creature model.Creature) bool {
	return creatureHasAnyFlag(creature, "unkillable", "cannotKill", "munkil", "MUNKIL")
}

type attackPlayerGateResult struct {
	Allowed bool
	Message string
}

type attackFamilyWarLegacyWorld interface {
	FamilyWarLegacyValues() (atWar int, callWar1 int, callWar2 int)
}

func attackPlayerCreature(world LookWorld, player model.Player) (model.Creature, bool) {
	if player.CreatureID.IsZero() {
		return model.Creature{}, false
	}
	creature, ok := world.Creature(player.CreatureID)
	return creature, ok
}

func attackPlayerCombatGate(
	world LookWorld,
	room model.Room,
	attacker model.Creature,
	attackerPlayerID model.PlayerID,
	victimPlayer model.Player,
	victim model.Creature,
) attackPlayerGateResult {
	atWar := attackLegacyAtWar(world)
	if atWar == 0 && roomHasAnyFlag(room, "noKill", "noPlayerKill", "RNOKIL") {
		return attackPlayerGateResult{Message: "이 곳에서는 싸울 수 없습니다."}
	}

	attackerFamily := attackPlayerHasFlag(world, attackerPlayerID, attacker, "PFAMIL", "familyFlag")
	victimFamily := attackPlayerHasFlag(world, victimPlayer.ID, victim, "PFAMIL", "familyFlag")
	checkChaos := !attackerFamily || !victimFamily
	if !checkChaos {
		checkChaos = attackLegacyCheckWar(atWar, attackCreatureFamilyID(attacker), attackCreatureFamilyID(victim))
	}
	if checkChaos && !roomHasAnyFlag(room, "survival", "RSUVIV") && attackCreatureLevel(attacker) < 128 {
		if !attackPlayerHasFlag(world, attackerPlayerID, attacker, "PCHAOS", "chaos") {
			return attackPlayerGateResult{Message: "당신은 선하다는걸 아세요."}
		}
		if !attackPlayerHasFlag(world, victimPlayer.ID, victim, "PCHAOS", "chaos") {
			return attackPlayerGateResult{Message: "그 사용자는 선해서 공격할 수 없습니다."}
		}
	}
	return attackPlayerGateResult{Allowed: true}
}

func attackActorAlreadyFighting(world LookWorld, room model.Room, viewer LookViewer, actor model.Creature) bool {
	enemyWorld, ok := world.(interface {
		CreatureEnemies(model.CreatureID) ([]string, error)
	})
	if !ok {
		return false
	}
	actorName := strings.TrimSpace(attackCreatureName(actor))
	if actorName == "" {
		return false
	}
	for _, id := range room.CreatureIDs {
		if id.IsZero() || id == viewer.CreatureID {
			continue
		}
		creature, ok := world.Creature(id)
		if !ok || creature.RoomID != room.ID || attackCreatureIsPlayer(creature) {
			continue
		}
		enemies, err := enemyWorld.CreatureEnemies(creature.ID)
		if err != nil {
			continue
		}
		for _, enemy := range enemies {
			if strings.TrimSpace(enemy) == actorName {
				return true
			}
		}
	}
	return false
}

func attackLegacyAtWar(world LookWorld) int {
	if warWorld, ok := world.(attackFamilyWarLegacyWorld); ok {
		atWar, _, _ := warWorld.FamilyWarLegacyValues()
		return atWar
	}
	return 0
}

func attackLegacyCheckWar(atWar int, firstFamily int, secondFamily int) bool {
	if firstFamily == 0 || secondFamily == 0 {
		return true
	}
	if atWar == 0 {
		return true
	}
	warFamily1 := atWar / 16
	warFamily2 := atWar % 16
	return !((firstFamily == warFamily1 && secondFamily == warFamily2) ||
		(secondFamily == warFamily1 && firstFamily == warFamily2))
}

func attackPlayerHasFlag(world LookWorld, playerID model.PlayerID, creature model.Creature, names ...string) bool {
	if attackCreatureHasFlag(creature, names...) {
		return true
	}
	if playerID.IsZero() {
		return false
	}
	player, ok := world.Player(playerID)
	return ok && hasAnyNormalizedFlag(player.Metadata.Tags, names...)
}

func attackCreatureFamilyID(creature model.Creature) int {
	if value, ok := attackCreatureIntValue(creature, "familyID", "dailyExpndMax", "legacyDailyExpndMax"); ok {
		return value
	}
	return 0
}

func attackCreatureIntValue(creature model.Creature, names ...string) (int, bool) {
	targets := normalizedFlagSet(names...)
	for key, value := range creature.Stats {
		if _, ok := targets[normalizeFlagName(key)]; ok {
			return value, true
		}
	}
	for key, raw := range creature.Properties {
		if _, ok := targets[normalizeFlagName(key)]; !ok {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		return value, err == nil
	}
	return 0, false
}

func creatureHasAnyFlag(creature model.Creature, names ...string) bool {
	if hasAnyNormalizedFlag(creature.Metadata.Tags, names...) {
		return true
	}
	targets := normalizedFlagSet(names...)
	for key, value := range creature.Stats {
		if value == 0 {
			continue
		}
		if _, ok := targets[normalizeFlagName(key)]; ok {
			return true
		}
	}
	for key, value := range creature.Properties {
		normalizedKey := normalizeFlagName(key)
		if _, ok := targets[normalizedKey]; ok && propertyFlagEnabled(value) {
			return true
		}
		for _, token := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ';' || r == '|' || r == ' '
		}) {
			if _, ok := targets[normalizeFlagName(token)]; ok {
				return true
			}
		}
	}
	return false
}

func attackCreatureName(creature model.Creature) string {
	if name := cleanDisplayText(creature.DisplayName); name != "" {
		return name
	}
	return string(creature.ID)
}

var attackRoll = func(min, max int) int {
	if max < min {
		min, max = max, min
	}
	return rand.Intn(max-min+1) + min
}

const (
	attackClassBarbarian = 2
	attackClassCleric    = 3
	attackClassMage      = 5
)

type attackDamageResult struct {
	Damage   int
	Hit      bool
	Messages []string
}

func revealAttackActor(ctx *Context, world AttackWorld, roomID model.RoomID, viewer LookViewer, attacker model.Creature) error {
	invisible := attackCreatureHasFlag(attacker, "invisible", "pinvis", "PINVIS")
	if !viewer.PlayerID.IsZero() {
		if player, ok := world.Player(viewer.PlayerID); ok {
			if hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "pinvis", "PINVIS") {
				invisible = true
			}
			if _, err := world.UpdatePlayerTags(viewer.PlayerID, nil, []string{
				"hidden", "phiddn", "PHIDDN",
				"invisible", "pinvis", "PINVIS",
			}); err != nil {
				return err
			}
		}
	}
	if _, err := world.UpdateCreatureTags(attacker.ID, nil, []string{
		"hidden", "phiddn", "PHIDDN",
		"invisible", "pinvis", "PINVIS",
	}); err != nil {
		return err
	}
	for _, key := range []string{"PHIDDN", "PINVIS"} {
		if attacker.Stats[key] != 0 {
			if err := world.SetCreatureStat(attacker.ID, key, 0); err != nil {
				return err
			}
		}
	}
	if !invisible {
		return nil
	}
	name := attackCreatureName(attacker)
	ctx.WriteString("당신은 모습을 드러냅니다.\n")
	return roomBroadcast(ctx, roomID, "\n"+name+krtext.Particle(name, '1')+" 모습을 드러냅니다.\n")
}

func attackCreatureDeflectsMundane(world InventoryWorld, attacker model.Creature, victim model.Creature) bool {
	if attackCreatureHasFlag(victim, "magicOnly", "mmgonl", "MMGONL") {
		return true
	}
	if !attackCreatureHasFlag(victim, "magicOrEnchantedOnly", "enchantOnly", "menonl", "MENONL") {
		return false
	}
	if creatureClass(attacker) >= model.ClassCaretaker {
		return false
	}
	weaponID := equippedObjectID(attacker, "wield")
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

func attackMultiAttackCount(attacker model.Creature) int {
	if !attackCreatureHasFlag(attacker, "PUPDMG", "upDamage", "upDmg") {
		return 1
	}
	class := creatureClass(attacker)
	level := attackCreatureLevel(attacker)
	count := 1
	if (class == model.ClassInvincible && level > 100) || class > model.ClassInvincible {
		if (level-97)/10+attackRoll(0, 3) > 2 {
			count++
		}
	}
	if class > model.ClassInvincible && attackRoll(1, 4) == 1 {
		count++
	}
	return count
}

func attackOneCreatureRound(
	ctx *Context,
	world AttackWorld,
	finalizer AttackDeathFinalizer,
	attacker model.Creature,
	victim model.Creature,
	name string,
) (dead bool, stopped bool, err error) {
	dead, stopped, err = attackOneDamageRound(ctx, world, attacker, victim, name, true)
	if err != nil || stopped || !dead {
		return dead, stopped, err
	}
	if finalizer != nil {
		if err := finalizer(ctx, attacker, victim); err != nil {
			return false, false, err
		}
	} else {
		if _, err := world.FinalizeMonsterDeath(victim.ID); err != nil {
			return false, false, err
		}
	}
	ctx.WriteString(name + krtext.Particle(name, '1') + " 쓰러졌습니다.\n")
	return true, false, nil
}

func attackOnePlayerRound(
	ctx *Context,
	world AttackWorld,
	attacker model.Creature,
	victim model.Creature,
	name string,
) (dead bool, stopped bool, err error) {
	dead, stopped, err = attackOneDamageRound(ctx, world, attacker, victim, name, false)
	if err != nil || stopped || !dead {
		return dead, stopped, err
	}
	ctx.WriteString(name + krtext.Particle(name, '1') + " 쓰러졌습니다.\n")
	return true, false, nil
}

func attackOneDamageRound(
	ctx *Context,
	world AttackWorld,
	attacker model.Creature,
	victim model.Creature,
	name string,
	recordDamage bool,
) (dead bool, stopped bool, err error) {
	stopped, err = attackStopForSpentWield(ctx, world, attacker)
	if err != nil || stopped {
		return false, stopped, err
	}

	// Increment weapon proficiency upon use!
	if weaponID := equippedObjectID(attacker, "wield"); !weaponID.IsZero() {
		if weapon, ok := world.Object(weaponID); ok {
			incrementAmount := 1 + legacyStatBonus(creatureStat(attacker, "dexterity"))/2
			if incrementAmount < 1 {
				incrementAmount = 1
			}
			attacker, _ = incrementWeaponProficiency(world, attacker, weapon, incrementAmount)
		}
	}

	outcome := attackDamageOutcome(world, attacker, victim)
	if !outcome.Hit {
		ctx.WriteString("당신의 공격은 빗나갔습니다.\n")
		return false, false, nil
	}
	for _, message := range outcome.Messages {
		ctx.WriteString(message)
	}
	_, applied, dead, err := world.ApplyCreatureDamage(victim.ID, outcome.Damage)
	if err != nil {
		return false, false, err
	}
	if recordDamage {
		if err := world.RecordCreatureDamage(victim.ID, attacker.ID, applied); err != nil {
			return false, false, err
		}
	}
	ctx.WriteString(fmt.Sprintf("당신은 %s에게 %d만큼의 피해를 주었습니다.\n", name, applied))
	dead, err = attackApplyAngelDamage(ctx, world, attacker, victim.ID, name, outcome.Damage, applied, dead, recordDamage)
	if err != nil {
		return false, false, err
	}
	if err := attackMaybeConsumeWieldCharge(world, attacker); err != nil {
		return false, false, err
	}
	if !dead {
		return false, false, nil
	}
	return true, false, nil
}

func attackDamage(world InventoryWorld, attacker model.Creature, victim model.Creature) (int, bool) {
	outcome := attackDamageOutcome(world, attacker, victim)
	return outcome.Damage, outcome.Hit
}

func attackDamageOutcome(world InventoryWorld, attacker model.Creature, victim model.Creature) attackDamageResult {
	if !attackHits(attacker, victim) {
		return attackDamageResult{}
	}
	class := creatureClass(attacker)
	if objectID := equippedObjectID(attacker, "wield"); !objectID.IsZero() {
		if object, ok := world.Object(objectID); ok {
			damage := objectDamage(world, object) + strengthDamageBonus(attacker)
			if !attackClassIgnoresWeaponProficiency(class) {
				damage += weaponProficiencyDamageBonus(world, attacker, object)
				damage += heldWeaponDamageBonus(world, attacker)
			}
			return attackApplyPaladinAlignment(normalizeAttackDamage(damage), attacker)
		}
	}
	damage := statsDamage(attacker) + strengthDamageBonus(attacker)
	if attackClassAddsUnarmedLevelBonus(class) {
		damage += (attackCreatureLevel(attacker) + 3) / 4
	}
	if damage != 0 {
		return attackApplyPaladinAlignment(normalizeAttackDamage(damage), attacker)
	}
	if attacker.Level > 0 {
		return attackApplyPaladinAlignment(attacker.Level, attacker)
	}
	return attackApplyPaladinAlignment(1, attacker)
}

func attackApplyPaladinAlignment(damage int, attacker model.Creature) attackDamageResult {
	outcome := attackDamageResult{Damage: damage, Hit: true}
	if creatureClass(attacker) != model.ClassPaladin {
		return outcome
	}
	alignment := creatureStat(attacker, "alignment")
	switch {
	case alignment < 0:
		outcome.Damage /= 2
		outcome.Messages = append(outcome.Messages, "당신의 악행이 양심을 괴롭힙니다.\n")
	case alignment > 250:
		outcome.Damage += attackRoll(1, 3)
		outcome.Messages = append(outcome.Messages, "당신의 선행이 능력을 배가시킵니다.\n")
	}
	return outcome
}

func attackHits(attacker model.Creature, victim model.Creature) bool {
	// C attack_crt (command5.c:232-237) uses the precomputed thaco, which already
	// folds weapon proficiency in once via compute_thaco -> mod_profic. There is no
	// second proficiency term here (Go previously double-subtracted raw proficiency).
	thaco := creatureStat(attacker, "thaco")
	target := thaco - creatureStat(victim, "armor")/10
	if attackCreatureHasFlag(attacker, "fear", "fearful", "PFEARS") {
		target += 2
	}
	if attackCreatureHasFlag(attacker, "blind", "pblind", "PBLIND") {
		target += 5
	}
	return attackRoll(1, 30) >= target
}

func attackClassIgnoresWeaponProficiency(class int) bool {
	return class == attackClassMage || class == attackClassCleric
}

func attackClassAddsUnarmedLevelBonus(class int) bool {
	return class == attackClassBarbarian || class > model.ClassInvincible
}

func heldWeaponDamageBonus(world InventoryWorld, attacker model.Creature) int {
	objectID := equippedObjectID(attacker, "held")
	if objectID.IsZero() {
		return 0
	}
	object, ok := world.Object(objectID)
	if !ok {
		return 0
	}
	legacyType := objectLegacyType(world, object)
	if legacyType < 0 || legacyType >= legacyObjectArmor {
		return 0
	}
	return objectDamage(world, object) / 10
}

func attackCreatureLevel(creature model.Creature) int {
	level := creature.Level
	if statsLevel := creatureStat(creature, "level"); statsLevel > level {
		level = statsLevel
	}
	return level
}

func attackCreatureHasFlag(creature model.Creature, names ...string) bool {
	if creatureHasAnyFlag(creature, names...) {
		return true
	}
	targets := normalizedFlagSet(names...)
	for key, value := range creature.Stats {
		if _, ok := targets[normalizeFlagName(key)]; ok && value != 0 {
			return true
		}
	}
	return false
}

func attackStopForSpentWield(ctx *Context, world AttackWorld, attacker model.Creature) (bool, error) {
	weaponID := equippedObjectID(attacker, "wield")
	if weaponID.IsZero() {
		return false, nil
	}
	weapon, ok := world.Object(weaponID)
	if !ok {
		return false, nil
	}
	shots, ok := objectIntProperty(world, weapon, "shotsCurrent")
	if !ok || shots > 0 {
		return false, nil
	}
	name := objectDisplayName(world, weapon)
	if err := world.MoveObject(weapon.ID, model.ObjectLocation{CreatureID: attacker.ID, Slot: "inventory"}); err != nil {
		return false, err
	}
	ctx.WriteString(name + krtext.Particle(name, '1') + " 부서져 버렸습니다.\n")
	return true, nil
}

func attackApplyAngelDamage(
	ctx *Context,
	world AttackWorld,
	attacker model.Creature,
	victimID model.CreatureID,
	victimName string,
	baseDamage int,
	baseApplied int,
	dead bool,
	recordDamage bool,
) (bool, error) {
	if dead || baseDamage <= 0 || baseApplied <= 0 || !attackCreatureHasFlag(attacker, "PANGEL", "angel") {
		return dead, nil
	}
	if attackRoll(1, 160) > attackAngelChance(attacker) {
		return dead, nil
	}
	damage := attackRoll(baseDamage/2, baseApplied)
	if damage <= 0 {
		return dead, nil
	}
	_, applied, angelDead, err := world.ApplyCreatureDamage(victimID, damage)
	if err != nil {
		return dead, err
	}
	if recordDamage {
		if err := world.RecordCreatureDamage(victimID, attacker.ID, applied); err != nil {
			return dead, err
		}
	}
	if applied > 0 {
		ctx.WriteString(fmt.Sprintf("당신의 정령이 %s에게 %d만큼의 피해를 주었습니다.\n", victimName, applied))
	}
	return angelDead, nil
}

func attackAngelChance(attacker model.Creature) int {
	chance := ((attackCreatureLevel(attacker) + 3) / 4) +
		legacyStatBonus(creatureStat(attacker, "intelligence"))*3 +
		legacyStatBonus(creatureStat(attacker, "piety"))*5
	return minInt(80, chance)
}

func attackMaybeConsumeWieldCharge(world AttackWorld, attacker model.Creature) error {
	weaponID := equippedObjectID(attacker, "wield")
	if weaponID.IsZero() {
		return nil
	}
	weapon, ok := world.Object(weaponID)
	if !ok {
		return nil
	}
	if _, ok := objectIntProperty(world, weapon, "shotsCurrent"); !ok {
		return nil
	}
	if attackRoll(0, 5) != 0 {
		return nil
	}
	_, _, _, err := world.ConsumeCreatureObjectCharge(weaponID, attacker.ID, false)
	return err
}

func objectDamage(world InventoryWorld, object model.ObjectInstance) int {
	nDice, _ := objectIntProperty(world, object, "nDice")
	sDice, _ := objectIntProperty(world, object, "sDice")
	pDice, _ := objectIntProperty(world, object, "pDice")
	return rollDice(nDice, sDice, pDice)
}

func statsDamage(creature model.Creature) int {
	if creature.Stats == nil {
		return 0
	}
	return rollDice(creature.Stats["nDice"], creature.Stats["sDice"], creature.Stats["pDice"])
}

func rollDice(nDice, sDice, pDice int) int {
	if nDice < 0 {
		nDice = 0
	}
	if sDice < 0 {
		sDice = 0
	}
	damage := pDice
	if nDice > 0 && sDice > 0 {
		for i := 0; i < nDice; i++ {
			damage += attackRoll(1, sDice)
		}
	}
	if damage < 0 {
		return 0
	}
	return damage
}

func normalizeAttackDamage(damage int) int {
	if damage < 1 {
		return 1
	}
	return damage
}

var attackStrengthBonus = [...]int{
	-4, -4, -4, -3, -3, -2, -2, -1, -1, -1,
	0, 0, 0, 0, 1, 1, 1, 2, 2, 2,
	3, 3, 3, 3, 4, 4, 4, 4, 4, 5,
	5, 5, 5, 5, 5, 6, 6, 6, 6, 6,
	6, 6, 6, 6, 7, 7, 7, 7, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 8, 8, 8,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
	9, 9, 9, 9, 9, 9, 9, 9, 9, 9,
	9, 9, 9, 9, 9, 9,
}

func strengthDamageBonus(attacker model.Creature) int {
	strength, ok := attacker.Stats["strength"]
	if !ok {
		return 0
	}
	if strength < 0 {
		strength = 0
	}
	if strength >= len(attackStrengthBonus) {
		strength = len(attackStrengthBonus) - 1
	}
	return attackStrengthBonus[strength]
}

func weaponProficiencyDamageBonus(world InventoryWorld, attacker model.Creature, weapon model.ObjectInstance) int {
	weaponType, ok := objectStringProperty(world, weapon, "type")
	if !ok {
		return 0
	}
	weaponType = strings.TrimSpace(weaponType)
	if weaponType == "" {
		return 0
	}
	// C attack_crt (command5.c:241): damage bonus is profic(ply, weapon->type)/10,
	// where profic() ranks the raw accumulation to 0-100 first. Consuming the raw
	// value directly inflates the bonus by orders of magnitude.
	class := creatureClass(attacker)
	for _, key := range proficiencyPropertyKeys(weaponType) {
		if value, ok := attacker.Stats[key]; ok {
			return model.WeaponProficiencyPercent(class, value) / 10
		}
		if value, ok := parseObjectInt(attacker.Properties[key]); ok {
			return model.WeaponProficiencyPercent(class, value) / 10
		}
	}
	return 0
}

func proficiencyPropertyKeys(weaponType string) []string {
	keys := []string{
		"proficiency/" + weaponType,
		"proficiency." + weaponType,
		"proficiency_" + weaponType,
	}
	if name := legacyWeaponProficiencyName(weaponType); name != "" {
		keys = append(keys,
			"proficiency/"+name,
			"proficiency."+name,
			"proficiency_"+name,
		)
	}
	return keys
}

func legacyWeaponProficiencyName(weaponType string) string {
	switch weaponType {
	case "0":
		return "sharp"
	case "1":
		return "thrust"
	case "2":
		return "blunt"
	case "3":
		return "pole"
	case "4":
		return "missile"
	default:
		return ""
	}
}

func objectStringProperty(world InventoryWorld, object model.ObjectInstance, key string) (string, bool) {
	if value := strings.TrimSpace(object.Properties[key]); value != "" {
		return value, true
	}
	if object.PrototypeID.IsZero() {
		return "", false
	}
	proto, ok := world.ObjectPrototype(object.PrototypeID)
	if !ok {
		return "", false
	}
	if value := strings.TrimSpace(proto.Properties[key]); value != "" {
		return value, true
	}
	return "", false
}
