package command

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

type UseWorld interface {
	DrinkWorld
	EquipWorld
	ExitControlWorld
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
	DestroyCreatureInventoryObject(model.ObjectInstanceID, model.CreatureID) (bool, error)
	SetObjectProperty(model.ObjectInstanceID, string, string) (model.ObjectInstance, error)
	PlayerDeath(model.PlayerID, model.CreatureID) error
}

const (
	legacySpecialMapScroll = 1
	legacySpecialCombo     = 2
	legacySpecialWar       = 3
)

var comboDamageRoll = func(min, max int) int {
	if max <= min {
		return min
	}
	return rand.Intn(max-min+1) + min
}

type specialComboMemory struct {
	mu       sync.Mutex
	sequence map[string]string
}

func newSpecialComboMemory() *specialComboMemory {
	return &specialComboMemory{sequence: map[string]string{}}
}

func (m *specialComboMemory) append(ctx *Context, digit rune, reset bool) string {
	if m == nil {
		return string(digit)
	}
	key := specialComboMemoryKey(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sequence == nil {
		m.sequence = map[string]string{}
	}
	current := m.sequence[key]
	if reset || len(current) > 70 {
		current = string(digit)
	} else {
		current += string(digit)
	}
	m.sequence[key] = current
	return current
}

func (m *specialComboMemory) clear(ctx *Context) {
	if m == nil {
		return
	}
	key := specialComboMemoryKey(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sequence != nil {
		delete(m.sequence, key)
	}
}

func specialComboMemoryKey(ctx *Context) string {
	if ctx == nil {
		return ""
	}
	if sessionID := strings.TrimSpace(ctx.SessionID); sessionID != "" {
		return sessionID
	}
	return strings.TrimSpace(ctx.ActorID)
}

func NewUseHandler(world UseWorld, drinkEffect DrinkEffectFunc) Handler {
	return NewUseHandlerWithRoot(world, "", drinkEffect)
}

func NewUseHandlerWithRoot(world UseWorld, root string, drinkEffect DrinkEffectFunc) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		target := getArg(resolved, 0)
		if target == "" {
			ctx.WriteString("무엇을 사용하시려구요?")
			return StatusDefault, nil
		}
		if target == "모두" {
			return NewWearHandler(world)(ctx, resolved)
		}
		room, ok := world.Room(player.RoomID)
		if !ok {
			return StatusDefault, errUseRoomNotFound(player.RoomID)
		}

		detectInvisible := inventoryViewerDetectsInvisible(player, creature)
		object, _, onFloor, ok := findUseObject(world, creature, room, target, firstGetOrdinal(resolved), detectInvisible)
		if !ok {
			ctx.WriteString("무엇을 사용하시려구요?")
			return StatusDefault, nil
		}
		player, creature, err = clearUseActorHidden(world, player, creature)
		if err != nil {
			return StatusDefault, err
		}

		if special, ok := legacyObjectSpecial(world, object); ok && special == legacySpecialWar {
			// C use() calls special_obj only for SP_WAR before generic type routing.
			ctx.WriteString("아무것도 없습니다.\n")
			return StatusDefault, nil
		}

		switch useObjectType(world, object) {
		case legacyObjectSharp, legacyObjectThrust, legacyObjectBlunt, legacyObjectPole, legacyObjectMissile:
			return NewReadyHandler(world)(ctx, useTargetResolved(resolved))
		case legacyObjectArmor:
			return NewWearHandler(world)(ctx, useTargetResolved(resolved))
		case legacyObjectPotion:
			return NewDrinkHandler(world, drinkEffect)(ctx, useTargetResolved(resolved))
		case legacyObjectScroll:
			return NewReadScrollHandler(world, root, nil)(ctx, useTargetResolved(resolved))
		case legacyObjectWand:
			if onFloor {
				return useFloorWand(ctx, world, creature, object, useTargetResolved(resolved))
			}
			return NewZapHandler(world, nil)(ctx, resolved)
		case legacyObjectLightSource:
			return NewHoldHandler(world)(ctx, useTargetResolved(resolved))
		case legacyObjectKey:
			return NewUnlockExitHandler(world)(ctx, useKeyUnlockResolved(resolved, onFloor))
		default:
			ctx.WriteString("그것을 어떻게 사용하시려구요?\n")
			return StatusDefault, nil
		}
	}
}

func clearUseActorHidden(world UseWorld, player model.Player, creature model.Creature) (model.Player, model.Creature, error) {
	updatedCreature, err := world.UpdateCreatureTags(creature.ID, nil, []string{"hidden", "phiddn", "PHIDDN"})
	if err != nil {
		return player, creature, err
	}
	creature = updatedCreature
	if creature.Stats != nil {
		if _, ok := creature.Stats["PHIDDN"]; ok {
			if err := world.SetCreatureStat(creature.ID, "PHIDDN", 0); err != nil {
				return player, creature, err
			}
			creature.Stats["PHIDDN"] = 0
		}
	}
	if !player.ID.IsZero() {
		updatedPlayer, err := world.UpdatePlayerTags(player.ID, nil, []string{"hidden", "phiddn", "PHIDDN"})
		if err != nil {
			return player, creature, err
		}
		player = updatedPlayer
	}
	return player, creature, nil
}

func useFloorWand(ctx *Context, world UseWorld, creature model.Creature, object model.ObjectInstance, resolved ResolvedCommand) (Status, error) {
	shots := objectIntPropertyOrZero(world, object, "shotsCurrent")
	updated, err := world.SetObjectProperty(object.ID, "shotsCurrent", strconv.Itoa(shots-1))
	if err != nil {
		return StatusDefault, err
	}

	success, err := defaultZapMagicEffect(ctx, world, creature, updated, resolved)
	if err != nil {
		return StatusDefault, err
	}
	if !success {
		return StatusDefault, nil
	}

	if text := zapObjectUseOutput(world, updated); text != "" {
		ctx.WriteString(ensureTrailingNewline(text))
	}
	shots = objectIntPropertyOrZero(world, updated, "shotsCurrent")
	_, err = world.SetObjectProperty(object.ID, "shotsCurrent", strconv.Itoa(shots-1))
	return StatusDefault, err
}

func errUseRoomNotFound(roomID model.RoomID) error {
	return fmt.Errorf("use: room %q not found", roomID)
}

func findUseObject(
	world UseWorld,
	creature model.Creature,
	room model.Room,
	target string,
	ordinal int64,
	detectInvisible bool,
) (model.ObjectInstance, string, bool, bool) {
	if object, name, ok := findEquipInventoryObjectWithVisibility(world, creature, target, ordinal, detectInvisible); ok {
		return object, name, false, true
	}
	object, name, ok := findUseRoomObject(world, room, target, ordinal, detectInvisible)
	return object, name, ok, ok
}

func findUseRoomObject(
	world UseWorld,
	room model.Room,
	target string,
	ordinal int64,
	detectInvisible bool,
) (model.ObjectInstance, string, bool) {
	if ordinal < 1 {
		ordinal = 1
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return model.ObjectInstance{}, "", false
	}
	var seen int64
	for _, objectID := range room.Objects.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInRoom(object, room.ID) {
			continue
		}
		if !objectHasAnyFlagOrProperty(world, object, "useFromFloor", "ousefl", "OUSEFL") {
			continue
		}
		if !detectInvisible && dropObjectIsInvisible(world, object) {
			continue
		}
		if !legacyObjectPrefixMatches(world, object, target) {
			continue
		}
		seen++
		if seen == ordinal {
			return object, objectDisplayName(world, object), true
		}
	}
	return model.ObjectInstance{}, "", false
}

func findLegacySpecialObjectCandidate(
	world InventoryWorld,
	creature model.Creature,
	room model.Room,
	target string,
	ordinal int64,
	detectInvisible bool,
) (model.ObjectInstance, string, bool) {
	if object, name, ok := findEquipInventoryObjectWithVisibility(world, creature, target, ordinal, detectInvisible); ok {
		return object, name, true
	}
	if object, name, ok := findEquippedObject(world, creature, target, ordinal); ok {
		return object, name, true
	}
	return findLegacyRoomObjectCandidate(world, room, target, ordinal, detectInvisible)
}

func findLegacyRoomObjectCandidate(
	world InventoryWorld,
	room model.Room,
	target string,
	ordinal int64,
	detectInvisible bool,
) (model.ObjectInstance, string, bool) {
	if ordinal < 1 {
		ordinal = 1
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return model.ObjectInstance{}, "", false
	}
	var seen int64
	for _, objectID := range room.Objects.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInRoom(object, room.ID) {
			continue
		}
		if !detectInvisible && dropObjectIsInvisible(world, object) {
			continue
		}
		if !legacyObjectPrefixMatches(world, object, target) {
			continue
		}
		seen++
		if seen == ordinal {
			return object, objectDisplayName(world, object), true
		}
	}
	return model.ObjectInstance{}, "", false
}

func useObjectType(world UseWorld, object model.ObjectInstance) int {
	return objectLegacyTypeOrKind(world, object)
}

func legacyObjectSpecial(world InventoryWorld, object model.ObjectInstance) (int, bool) {
	if special, ok := legacyPropertiesSpecial(object.Properties); ok {
		return special, true
	}
	if object.PrototypeID.IsZero() {
		return 0, false
	}
	proto, ok := world.ObjectPrototype(object.PrototypeID)
	if !ok {
		return 0, false
	}
	return legacyPropertiesSpecial(proto.Properties)
}

func legacyPropertiesSpecial(properties map[string]string) (int, bool) {
	for key, value := range properties {
		if normalizeFlagName(key) == "special" {
			return legacySpecialValue(value)
		}
	}
	for key, value := range properties {
		if !propertyFlagEnabled(value) {
			continue
		}
		switch normalizeFlagName(key) {
		case "spmapsc", "mapsc":
			return legacySpecialMapScroll, true
		case "spcombo", "combo":
			return legacySpecialCombo, true
		case "spwar", "war":
			return legacySpecialWar, true
		}
	}
	return 0, false
}

func legacySpecialValue(value string) (int, bool) {
	if special, ok := parseObjectInt(value); ok {
		switch special {
		case legacySpecialMapScroll, legacySpecialCombo, legacySpecialWar:
			return special, true
		default:
			return 0, false
		}
	}
	switch normalizeFlagName(value) {
	case "spmapsc", "mapsc":
		return legacySpecialMapScroll, true
	case "spcombo", "combo":
		return legacySpecialCombo, true
	case "spwar", "war":
		return legacySpecialWar, true
	default:
		return 0, false
	}
}

type comboStepResult struct {
	complete bool
	success  bool
	exit     model.Exit
	damage   int
	dead     bool
}

func useSpecialCombo(
	ctx *Context,
	world UseWorld,
	memory *specialComboMemory,
	creature model.Creature,
	room model.Room,
	object model.ObjectInstance,
) (Status, error) {
	combination := comboObjectUseOutput(world, object)
	sDice := comboObjectIntPropertyOrZero(world, object, "sDice", "sdice")
	input := string(rune('0' + sDice))

	if memory == nil {
		memory = newSpecialComboMemory()
	}

	for _, digit := range input {
		result, err := useSpecialComboDigit(ctx, world, memory, &creature, room, object, combination, digit)
		if err != nil {
			return StatusDefault, err
		}
		if result.complete {
			return StatusDefault, nil
		}
	}
	return StatusDefault, nil
}

func useSpecialComboDigit(
	ctx *Context,
	world UseWorld,
	memory *specialComboMemory,
	creature *model.Creature,
	room model.Room,
	object model.ObjectInstance,
	combination string,
	digit rune,
) (comboStepResult, error) {
	nDice := comboObjectIntPropertyOrZero(world, object, "nDice", "ndice")
	nextSequence := memory.append(ctx, digit, nDice == 1)

	ctx.WriteString("Click.\n")
	actorName := attackCreatureName(*creature)
	objectName := objectDisplayName(world, object)
	if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+objectName+krtext.Particle(objectName, '3')+" 눌렀습니다.\n"); err != nil {
		return comboStepResult{}, err
	}

	if len(nextSequence) < len(combination) {
		return comboStepResult{}, nil
	}

	if nextSequence != combination {
		damage := comboDamageRoll(20, 40)
		updated, applied, dead, err := world.ApplyCreatureDamage(creature.ID, damage)
		if err != nil {
			return comboStepResult{}, err
		}
		*creature = updated

		memory.clear(ctx)
		ctx.WriteString(fmt.Sprintf("당신은 %d점의 피해를 입었습니다!\n", applied))
		if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+objectName+"로부터 피해를 입었습니다!\n"); err != nil {
			return comboStepResult{}, err
		}
		if dead {
			ctx.WriteString("당신은 죽었습니다.\n")
			if !creature.PlayerID.IsZero() {
				if err := world.PlayerDeath(creature.PlayerID, creature.ID); err != nil {
					return comboStepResult{}, err
				}
			}
		}
		return comboStepResult{complete: true, damage: damage, dead: dead}, nil
	}

	exit, ok, err := comboUnlockExit(world, room, object)
	if err != nil {
		return comboStepResult{}, err
	}
	if !ok {
		return comboStepResult{complete: true, success: true}, nil
	}

	ctx.WriteString("당신은 " + exit.Name + krtext.Particle(exit.Name, '3') + " 열었습니다!\n")
	if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+exit.Name+krtext.Particle(exit.Name, '3')+" 열었습니다!\n"); err != nil {
		return comboStepResult{}, err
	}
	return comboStepResult{complete: true, success: true, exit: exit}, nil
}

func comboUnlockExit(world UseWorld, room model.Room, object model.ObjectInstance) (model.Exit, bool, error) {
	exitNumber := comboObjectIntPropertyOrZero(world, object, "pDice", "pdice")
	if exitNumber < 1 {
		exitNumber = 1
	}
	index := exitNumber - 1
	if index < 0 || index >= len(room.Exits) {
		return model.Exit{}, false, nil
	}
	exit := room.Exits[index]
	if _, err := world.SetExitFlag(room.ID, exit.Name, "locked", false); err != nil {
		return model.Exit{}, false, err
	}
	updated, err := world.SetExitFlag(room.ID, exit.Name, "closed", false)
	if err != nil {
		return model.Exit{}, false, err
	}
	if touched, ok, err := touchExitTimerIfSupported(world, room.ID, exit.Name); err != nil {
		return model.Exit{}, false, err
	} else if ok {
		updated = touched
	}
	return updated, true, nil
}

func comboObjectUseOutput(world InventoryWorld, object model.ObjectInstance) string {
	if text := comboStringProperty(object.Properties, "useOutput", "use_output"); text != "" {
		return text
	}
	if object.PrototypeID.IsZero() {
		return ""
	}
	proto, ok := world.ObjectPrototype(object.PrototypeID)
	if !ok {
		return ""
	}
	return comboStringProperty(proto.Properties, "useOutput", "use_output")
}

func comboStringProperty(properties map[string]string, keys ...string) string {
	targets := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		targets[normalizeFlagName(key)] = struct{}{}
	}
	for key, value := range properties {
		if _, ok := targets[normalizeFlagName(key)]; ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func comboObjectIntPropertyOrZero(world InventoryWorld, object model.ObjectInstance, keys ...string) int {
	for _, key := range keys {
		if value, ok := objectIntProperty(world, object, key); ok {
			return value
		}
	}
	return 0
}

func clampInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func useTargetResolved(resolved ResolvedCommand) ResolvedCommand {
	out := resolved
	if len(out.Args) > 1 {
		out.Args = append([]string(nil), out.Args[:1]...)
	}
	if len(out.Values) > 1 {
		out.Values = append([]int64(nil), out.Values[:1]...)
	}
	return out
}

func useKeyUnlockResolved(resolved ResolvedCommand, onFloor bool) ResolvedCommand {
	if onFloor {
		return useTargetResolved(resolved)
	}
	out := resolved
	keyName := getArg(resolved, 0)
	exitName := getArg(resolved, 1)
	keyOrdinal := getOrdinal(resolved, 0)
	exitOrdinal := getOrdinal(resolved, 1)
	out.Args = []string{exitName, keyName}
	out.Values = []int64{exitOrdinal, keyOrdinal}
	return out
}
