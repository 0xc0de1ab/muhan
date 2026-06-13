package command

import (
	"fmt"
	"strings"
	"time"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const kickCooldownKey = "kick"

type KickWorld interface {
	LookWorld
	InventoryWorld
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
	RecordCreatureDamage(model.CreatureID, model.CreatureID, int) error
	FinalizeMonsterDeath(model.CreatureID) (bool, error)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
}

type kickStatUpdater interface {
	SetCreatureStat(model.CreatureID, string, int) error
}

type kickTarget struct {
	creature  model.Creature
	player    model.Player
	hasPlayer bool
}

func NewKickHandler(world KickWorld) Handler {
	return NewKickHandlerWithDeathFinalizer(world, nil)
}

func NewKickHandlerWithDeathFinalizer(world KickWorld, finalizer AttackDeathFinalizer) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("kick: actor creature %q not found", viewer.CreatureID)
		}

		target, ordinal := lookTarget(resolved)
		if target == "" || attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
			ctx.WriteString("누굴 공격합니까?\n")
			return StatusDefault, nil
		}
		if message := kickClassRejection(actor); message != "" {
			ctx.WriteString(message)
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

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, kickCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}
		if err := world.SetCreatureCooldown(actor.ID, kickCooldownKey, now, kickInitialCooldown(actor)); err != nil {
			return StatusDefault, err
		}
		if err := revealKickActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}
		if !victim.hasPlayer {
			if attackCreatureProtected(victim.creature) {
				ctx.WriteString("당신은 " + stealObjectPronoun(victim.creature) + " 해칠 수 없습니다.\n")
				return StatusDefault, nil
			}
			if attackCreatureDeflectsMundane(world, actor, victim.creature) {
				name := attackCreatureName(victim.creature)
				ctx.WriteString("당신의 공격이 " + name + "에게 아무 소용이 없는듯 합니다.\n")
				return StatusDefault, nil
			}
		}
		if victim.creature.Stats == nil {
			ctx.WriteString("그 상대는 공격할 수 없습니다.\n")
			return StatusDefault, nil
		}
		if hp, ok := victim.creature.Stats["hpCurrent"]; !ok || hp <= 0 {
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

		if !kickLands(actor, victim.creature) {
			ctx.WriteString("당신의 발차기가 실패했습니다.\n")
			_ = sendToPlayer(ctx, victim.creature.PlayerID, actorName+"이 당신에게 발차기를 하려고 합니다.\n")
			_ = broadcastRom2(ctx, world, room.ID, viewer.PlayerID, victim.creature.PlayerID, "\n"+actorName+"이 "+victimName+"에게 발차기를 하려고 합니다.")
			return StatusDefault, nil
		}

		damage := kickDamage(actor, victim.creature)
		_, applied, dead, err := world.ApplyCreatureDamage(victim.creature.ID, damage.actual)
		if err != nil {
			return StatusDefault, err
		}
		if !victim.hasPlayer {
			if err := world.RecordCreatureDamage(victim.creature.ID, actor.ID, damage.ledger); err != nil {
				return StatusDefault, err
			}
		}
		if err := world.SetCreatureCooldown(actor.ID, kickCooldownKey, now, kickHitCooldown(actor)); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(fmt.Sprintf("당신은 발차기로 %d점의 공격을 가했습니다.\n", applied))
		_ = sendToPlayer(ctx, victim.creature.PlayerID, fmt.Sprintf("%s이 발차기로 %d점의 공격을 가했습니다.\n", actorName, applied))
		_ = broadcastRom2(ctx, world, room.ID, viewer.PlayerID, victim.creature.PlayerID, "\n"+actorName+"이 "+victimName+"에게 발차기로 공격을 가합니다.")
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

func kickClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	if class != model.ClassBarbarian && class < model.ClassInvincible {
		return "권법가만 쓸수 있는 기술입니다.\n"
	}
	if class >= model.ClassInvincible && !kickHasBarbarianTraining(actor) {
		return "\n권법가를 무적수련하지 않았습니다..\n"
	}
	return ""
}

func kickHasBarbarianTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SBARBARIAN",
		"barbarianSpell",
		"barbarianTraining",
		"barbarianMode",
	)
}

func findKickTarget(world LookWorld, room model.Room, viewer LookViewer, prefix string, ordinal int64) (kickTarget, bool) {
	if creature, ok := findAttackCreatureTarget(world, room, viewer, prefix, ordinal); ok {
		return kickTarget{creature: creature}, true
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

type revealActorWorld interface {
	LookWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
}

func revealKickActor(ctx *Context, world revealActorWorld, roomID model.RoomID, viewer LookViewer, actor model.Creature) error {
	invisible := attackCreatureHasFlag(actor, "invisible", "pinvis", "PINVIS")
	if !viewer.PlayerID.IsZero() {
		if player, ok := world.Player(viewer.PlayerID); ok {
			if hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "pinvis", "PINVIS") {
				invisible = true
			}
			if _, err := world.UpdatePlayerTags(viewer.PlayerID, nil, []string{"hidden", "phiddn", "PHIDDN", "invisible", "pinvis", "PINVIS"}); err != nil {
				return err
			}
		}
	}
	if _, err := world.UpdateCreatureTags(actor.ID, nil, []string{"hidden", "phiddn", "PHIDDN", "invisible", "pinvis", "PINVIS"}); err != nil {
		return err
	}
	if updater, ok := world.(kickStatUpdater); ok {
		for _, key := range []string{"PHIDDN", "PINVIS"} {
			if actor.Stats[key] != 0 {
				if err := updater.SetCreatureStat(actor.ID, key, 0); err != nil {
					return err
				}
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

func kickPlayerCombatGate(
	world LookWorld,
	room model.Room,
	actor model.Creature,
	actorPlayerID model.PlayerID,
	victimPlayer model.Player,
	victim model.Creature,
) attackPlayerGateResult {
	atWar := attackLegacyAtWar(world)
	if atWar == 0 && roomHasAnyFlag(room, "noKill", "noPlayerKill", "RNOKIL") {
		return attackPlayerGateResult{Message: "이 방에서는 싸울 수 없습니다."}
	}

	actorFamily := attackPlayerHasFlag(world, actorPlayerID, actor, "PFAMIL", "familyFlag")
	victimFamily := attackPlayerHasFlag(world, victimPlayer.ID, victim, "PFAMIL", "familyFlag")
	checkChaos := !actorFamily || !victimFamily
	if !checkChaos {
		checkChaos = attackLegacyCheckWar(atWar, attackCreatureFamilyID(actor), attackCreatureFamilyID(victim))
	}
	if checkChaos && !roomHasAnyFlag(room, "survival", "RSUVIV") && attackCreatureLevel(actor) < 128 {
		if !attackPlayerHasFlag(world, actorPlayerID, actor, "PCHAOS", "chaos") {
			return attackPlayerGateResult{Message: "당신은 선해서 다른 사용자를 공격할 수 없습니다."}
		}
		if !attackPlayerHasFlag(world, victimPlayer.ID, victim, "PCHAOS", "chaos") {
			return attackPlayerGateResult{Message: "그 사용자는 선해서 보호받고 있습니다."}
		}
	}
	return attackPlayerGateResult{Allowed: true}
}

type kickCharmMessageStyle int

const (
	kickCharmMessageCircle kickCharmMessageStyle = iota
	kickCharmMessageKick
)

func kickPlayerCharmGate(
	world LookWorld,
	actor model.Creature,
	actorPlayerID model.PlayerID,
	victimPlayer model.Player,
	victim model.Creature,
	style kickCharmMessageStyle,
) attackPlayerGateResult {
	if !kickPlayerHasPCharm(world, victimPlayer, victim) {
		return attackPlayerGateResult{Allowed: true}
	}
	if !kickCharmListContainsCreature(world, victimPlayer, victim, actor, actorPlayerID) {
		return attackPlayerGateResult{Allowed: true}
	}
	name := attackCreatureName(victim)
	message := "당신은 " + name + krtext.Particle(name, '3') + " 너무 사랑해 그렇게 할 수 없습니다."
	if style == kickCharmMessageCircle {
		message = "당신은 " + name + krtext.Particle(name, '3') + " 너무 사랑해서 그렇게 할 수 없습니다."
	}
	return attackPlayerGateResult{Message: message}
}

func kickPlayerHasPCharm(world LookWorld, victimPlayer model.Player, victim model.Creature) bool {
	if kickHasLegacyPCharmFlag(victim.Metadata.Tags, victim.Stats, victim.Properties) {
		return true
	}
	if kickHasLegacyPCharmFlag(victimPlayer.Metadata.Tags, nil, nil) {
		return true
	}
	if !victimPlayer.ID.IsZero() {
		if player, ok := world.Player(victimPlayer.ID); ok {
			return kickHasLegacyPCharmFlag(player.Metadata.Tags, nil, nil)
		}
	}
	return false
}

func kickHasLegacyPCharmFlag(tags []string, stats map[string]int, properties map[string]string) bool {
	targets := map[string]struct{}{
		normalizeFlagName("PCHARM"):  {},
		normalizeFlagName("charmed"): {},
		normalizeFlagName("charm"):   {},
	}
	for _, tag := range tags {
		if _, ok := targets[normalizeFlagName(tag)]; ok {
			return true
		}
	}
	for key, value := range stats {
		if value == 0 {
			continue
		}
		if _, ok := targets[normalizeFlagName(key)]; ok {
			return true
		}
	}
	for key, value := range properties {
		if _, ok := targets[normalizeFlagName(key)]; ok && propertyFlagEnabled(value) {
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

func kickActorHasPCharm(world LookWorld, actor model.Creature, actorPlayerID model.PlayerID) bool {
	if kickHasLegacyPCharmFlag(actor.Metadata.Tags, actor.Stats, actor.Properties) {
		return true
	}
	if !actorPlayerID.IsZero() {
		if player, ok := world.Player(actorPlayerID); ok {
			return kickHasLegacyPCharmFlag(player.Metadata.Tags, nil, nil)
		}
	}
	return false
}

func kickCharmListContainsCreature(
	world LookWorld,
	charmOwnerPlayer model.Player,
	charmOwner model.Creature,
	target model.Creature,
	targetPlayerID model.PlayerID,
) bool {
	targetNames := map[string]struct{}{}
	addName := func(name string) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			targetNames[name] = struct{}{}
		}
	}
	addName(target.DisplayName)
	if !targetPlayerID.IsZero() {
		if player, ok := world.Player(targetPlayerID); ok {
			addName(player.DisplayName)
		}
	}
	if len(targetNames) == 0 {
		addName(string(target.ID))
	}

	targetIDs := map[string]struct{}{}
	if !target.ID.IsZero() {
		targetIDs[strings.ToLower(strings.TrimSpace(string(target.ID)))] = struct{}{}
	}

	if kickCharmTagsContainCreature(charmOwner.Metadata.Tags, targetNames, targetIDs) ||
		kickCharmTagsContainCreature(charmOwnerPlayer.Metadata.Tags, targetNames, targetIDs) {
		return true
	}
	if !charmOwnerPlayer.ID.IsZero() {
		if player, ok := world.Player(charmOwnerPlayer.ID); ok {
			return kickCharmTagsContainCreature(player.Metadata.Tags, targetNames, targetIDs)
		}
	}
	return false
}

func kickCharmTagsContainCreature(tags []string, targetNames map[string]struct{}, targetIDs map[string]struct{}) bool {
	for _, tag := range tags {
		raw := strings.TrimSpace(tag)
		lower := strings.ToLower(raw)
		switch {
		case strings.HasPrefix(lower, "charmid:"):
			id := strings.ToLower(strings.TrimSpace(raw[len("charmid:"):]))
			if _, ok := targetIDs[id]; ok {
				return true
			}
		case strings.HasPrefix(lower, "charm:"):
			name := strings.ToLower(strings.TrimSpace(raw[len("charm:"):]))
			if _, ok := targetNames[name]; ok {
				return true
			}
		}
	}
	return false
}

func kickLands(actor model.Creature, victim model.Creature) bool {
	if attackRoll(1, 100) > kickChance(actor, victim) {
		return false
	}
	target := creatureStat(actor, "thaco") - creatureStat(victim, "armor")/10
	return attackRoll(1, 20) >= target
}

func kickChance(actor model.Creature, victim model.Creature) int {
	chance := 50 + (((attackCreatureLevel(actor)+3)/4)-((attackCreatureLevel(victim)+3)/4))*5
	chance += legacyStatBonus(creatureStat(actor, "strength")) * 3
	chance += (legacyStatBonus(creatureStat(actor, "dexterity")) - legacyStatBonus(creatureStat(victim, "dexterity"))) * 2
	if creatureClass(actor) == model.ClassBarbarian {
		chance += 10
	}
	if creatureClass(actor) > model.ClassInvincible {
		chance += 5
	}
	chance = minInt(85, chance)
	if attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
		chance = minInt(20, chance)
	}
	return chance
}

type kickDamageResult struct {
	actual int
	ledger int
}

func kickDamage(actor model.Creature, victim model.Creature) kickDamageResult {
	base := statsDamage(actor) * 8
	if creatureClass(victim) > model.ClassCaretaker {
		return kickDamageResult{actual: 1, ledger: minInt(creatureStat(victim, "hpCurrent"), base)}
	}
	return kickDamageResult{actual: base, ledger: minInt(creatureStat(victim, "hpCurrent"), base)}
}

func kickInitialCooldown(actor model.Creature) int64 {
	class := creatureClass(actor)
	switch {
	case class >= model.ClassBulsa:
		return 3
	case class == model.ClassCaretaker:
		return 4
	case class == model.ClassBarbarian:
		return 3
	default:
		return 5
	}
}

func kickHitCooldown(actor model.Creature) int64 {
	if creatureClass(actor) >= model.ClassInvincible {
		return int64(10 - minInt(7, creatureStat(actor, "dexterity")/5))
	}
	return int64(8 - minInt(5, creatureStat(actor, "dexterity")/5))
}
