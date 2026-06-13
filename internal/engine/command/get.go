package command

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

var (
	ErrGetWorldRequired    = errors.New("get world required")
	ErrGetActorRequired    = errors.New("get actor required")
	ErrGetPlayerNotFound   = errors.New("get player not found")
	ErrGetCreatureRequired = errors.New("get creature required")
	ErrGetCreatureNotFound = errors.New("get creature not found")
)

type GetWorld interface {
	Room(model.RoomID) (model.Room, bool)
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
	MoveObject(model.ObjectInstanceID, model.ObjectLocation) error
	TakeContainerObjectToCreatureInventory(model.ObjectInstanceID, model.ObjectInstanceID, model.CreatureID) (int, bool, error)
	PickupMoneyObjectToCreatureGold(model.ObjectInstanceID, model.ObjectLocation, model.CreatureID) (int, int, bool, error)

	// B: Persistence hook
	SavePlayer(model.PlayerID) error
}

func NewGetHandler(world GetWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		if world == nil {
			return StatusDefault, ErrGetWorldRequired
		}

		playerID := GetPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrGetActorRequired
		}

		target := getArg(resolved, 0)
		if target == "" {
			ctx.WriteString("뭘 주우시게요?")
			return StatusDefault, nil
		}

		player, creature, room, err := CurrentGetActor(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		player, creature, err = clearGetActorHidden(world, player, creature)
		if err != nil {
			return StatusDefault, err
		}
		detectInvisible := inventoryViewerDetectsInvisible(player, creature)

		if len(resolved.Args) >= 2 {
			if all, filter, ok := allObjectTarget(getArg(resolved, 1)); ok {
				return getAllObjectsFromContainer(ctx, world, player, creature, room, getArg(resolved, 0), getOrdinal(resolved, 0), all, filter, detectInvisible)
			}
		}
		if getUsesRoomPickupGate(world, room, resolved) {
			if guard, ok := getRoomPickupGuard(world, room, creature.ID); ok && creatureClass(creature) < model.ClassCaretaker {
				ctx.WriteString(RenderGetRoomGuardBlock(guard))
				return StatusDefault, nil
			}
			if getActorBlind(player, creature) {
				ctx.WriteString(RenderGetBlindBlock())
				return StatusDefault, nil
			}
		}
		if all, filter, ok := allObjectTarget(joinArgs(resolved.Args)); ok {
			return getAllRoomObjects(ctx, world, player, creature, room, all, filter, detectInvisible)
		}

		object, container, name, containerName, fromContainer, failure := findGetTargetObject(world, creature, room, resolved, detectInvisible)
		if failure != "" {
			ctx.WriteString(failure)
			return StatusDefault, nil
		}

		if fromContainer {
			if allowed, err := handleCorpseLootCheck(ctx, world, creature, container); err != nil {
				return StatusDefault, err
			} else if !allowed {
				return StatusDefault, nil
			}
		} else {
			if getObjectIsInvisible(world, object) {
				ctx.WriteString("그런건 여기 없어요.")
				return StatusDefault, nil
			}
			if getObjectIsNoTakeOrScenery(world, object) {
				ctx.WriteString("주을 수 있는 물건이 아닙니다.")
				return StatusDefault, nil
			}
		}
		includeWeight := !fromContainer || objectLocatedInRoom(container, room.ID)
		if getTakeCapacityExceeded(world, creature, object, includeWeight) {
			ctx.WriteString(getInventoryFullMessage())
			return StatusDefault, nil
		}
		if !fromContainer && getObjectQuestNumber(world, object) > 0 && getCreatureQuestCompleted(creature, getObjectQuestNumber(world, object)) {
			ctx.WriteString(getQuestAlreadyCompletedMessage())
			return StatusDefault, nil
		}

		if !fromContainer {
			var err error
			object, err = prepareGetPickupObject(world, object, true, true)
			if err != nil {
				return StatusDefault, err
			}
		} else if getObjectIsPermanent(world, object) {
			var err error
			object, err = prepareGetPickupObject(world, object, false, false)
			if err != nil {
				return StatusDefault, err
			}
		}
		if result, err := moveGetObjectToInventory(world, object, creature.ID, fromContainer); err != nil {
			return StatusDefault, err
		} else if !result.Moved {
			ctx.WriteString("그 안에 그런것은 없어요.")
			return StatusDefault, nil
		} else if result.Money {
			if fromContainer {
				ctx.WriteString(RenderGetFromContainerConfirmation(containerName, name))
				_ = roomBroadcast(ctx, room.ID, renderGetFromContainerRoomConfirmation(commandActorDisplayName(player, creature), containerName, name))
			} else {
				ctx.WriteString(RenderGetConfirmation(name))
				_ = roomBroadcast(ctx, room.ID, renderGetRoomConfirmation(commandActorDisplayName(player, creature), name))
			}
			ctx.WriteString(RenderGetMoneyBalance(result.NewGold))
			return StatusDefault, nil
		}

		if fromContainer {
			ctx.WriteString(RenderGetFromContainerConfirmation(containerName, name))
			_ = roomBroadcast(ctx, room.ID, renderGetFromContainerRoomConfirmation(commandActorDisplayName(player, creature), containerName, name))
		} else {
			ctx.WriteString(RenderGetConfirmation(name))
			_ = roomBroadcast(ctx, room.ID, renderGetRoomConfirmation(commandActorDisplayName(player, creature), name))
			getCompleteQuestPickup(ctx, world, creature, player, object, false)
		}

		// B/C: Queue async save after successful get (mutation already marked dirty in Pickup/Drop paths)
		if creature.PlayerID != "" {
			if w, ok := world.(interface {
				MarkPlayerDirty(model.PlayerID)
				QueueSave(model.PlayerID, model.BankID)
			}); ok {
				w.MarkPlayerDirty(creature.PlayerID)
				w.QueueSave(creature.PlayerID, "")
			} else {
				// fallback direct for test mocks without Queue
				_ = world.SavePlayer(creature.PlayerID)
			}
		}
		return StatusDefault, nil
	}
}

type getMoveResult struct {
	Moved   bool
	Money   bool
	Amount  int
	NewGold int
}

func moveGetObjectToInventory(world GetWorld, object model.ObjectInstance, creatureID model.CreatureID, fromContainer bool) (getMoveResult, error) {
	if newGold, amount, picked, err := world.PickupMoneyObjectToCreatureGold(object.ID, object.Location, creatureID); err != nil {
		return getMoveResult{}, err
	} else if picked {
		return getMoveResult{Moved: true, Money: true, Amount: amount, NewGold: newGold}, nil
	}
	if fromContainer && !object.Location.ContainerID.IsZero() {
		_, taken, err := world.TakeContainerObjectToCreatureInventory(object.ID, object.Location.ContainerID, creatureID)
		return getMoveResult{Moved: taken}, err
	}
	location := model.ObjectLocation{CreatureID: creatureID, Slot: "inventory"}
	if err := world.MoveObject(object.ID, location); err != nil {
		return getMoveResult{}, err
	}
	return getMoveResult{Moved: true}, nil
}

func getAllRoomObjects(ctx *Context, world GetWorld, player model.Player, creature model.Creature, room model.Room, all bool, filter string, detectInvisible bool) (Status, error) {
	groups := make([]inventoryObjectGroup, 0, len(room.Objects.ObjectIDs))
	moneyGold := 0
	heldCount := getTakeCreatureHeldCount(world, creature)
	carriedWeight := getTakeCreatureCarriedWeight(world, creature)
	maxWeight := getTakeCreatureMaxWeight(creature)
	heavy := false
	completedQuests := getCreatureCompletedQuestSet(creature)
	objectIDs := append([]model.ObjectInstanceID(nil), room.Objects.ObjectIDs...)
	for _, objectID := range objectIDs {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInRoom(object, room.ID) || !getObjectCanBeTaken(world, object, detectInvisible) {
			continue
		}
		if !all && !getObjectMatches(world, object, filter) {
			continue
		}
		objectWeight := getTakeObjectTotalWeight(world, object)
		if heldCount > 150 || carriedWeight+objectWeight > maxWeight {
			heavy = true
			continue
		}
		questNum := getObjectQuestNumber(world, object)
		if questNum > 0 && completedQuests[questNum] {
			heavy = true
			continue
		}
		object, err := prepareGetPickupObject(world, object, true, true)
		if err != nil {
			return StatusDefault, err
		}
		if result, err := moveGetObjectToInventory(world, object, creature.ID, false); err != nil {
			return StatusDefault, err
		} else if !result.Moved {
			continue
		} else if result.Money {
			moneyGold = result.NewGold
			heldCount++
			groups = appendGetObjectGroup(world, groups, object, true)
		} else {
			heldCount++
			carriedWeight += objectWeight
			groups = appendGetObjectGroup(world, groups, object, false)
			if getCompleteQuestPickup(ctx, world, creature, model.Player{}, object, true) {
				completedQuests[questNum] = true
			}
		}
	}
	if len(groups) == 0 {
		if heavy {
			ctx.WriteString(getInventoryHeavyMessage())
			return StatusDefault, nil
		}
		ctx.WriteString("여기에는 아무것도 없습니다.")
		return StatusDefault, nil
	}
	if heavy {
		ctx.WriteString(getInventoryHeavyMessage())
	}
	rendered := renderDropObjectGroups(groups)
	ctx.WriteString(RenderGetConfirmation(rendered))
	_ = roomBroadcast(ctx, room.ID, renderGetRoomConfirmation(commandActorDisplayName(player, creature), rendered))
	if moneyGold > 0 {
		ctx.WriteString(RenderGetMoneyBalance(moneyGold))
	}
	return StatusDefault, nil
}

func getAllObjectsFromContainer(
	ctx *Context,
	world GetWorld,
	player model.Player,
	creature model.Creature,
	room model.Room,
	containerTarget string,
	containerOrdinal int64,
	all bool,
	filter string,
	detectInvisible bool,
) (Status, error) {
	container, containerName, ok := findGetVisibleObject(world, creature, room, containerTarget, containerOrdinal, detectInvisible)
	if !ok {
		ctx.WriteString("그런것은 보이지 않습니다.")
		return StatusDefault, nil
	}
	if !isContainerOrCorpse(world, container) {
		ctx.WriteString("그것은 담는 종류가 아닙니다.")
		return StatusDefault, nil
	}
	if allowed, err := handleCorpseLootCheck(ctx, world, creature, container); err != nil {
		return StatusDefault, err
	} else if !allowed {
		return StatusDefault, nil
	}

	groups := make([]inventoryObjectGroup, 0, len(container.Contents.ObjectIDs))
	moneyPicked := false
	heldCount := getTakeCreatureHeldCount(world, creature)
	carriedWeight := getTakeCreatureCarriedWeight(world, creature)
	maxWeight := getTakeCreatureMaxWeight(creature)
	heavy := false
	objectIDs := append([]model.ObjectInstanceID(nil), container.Contents.ObjectIDs...)
	for _, objectID := range objectIDs {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInContainer(object, container.ID) || !getObjectCanBeTaken(world, object, detectInvisible) {
			continue
		}
		if !all && !getObjectMatches(world, object, filter) {
			continue
		}
		objectWeight := getTakeObjectTotalWeight(world, object)
		if heldCount > 150 || carriedWeight+objectWeight > maxWeight {
			heavy = true
			continue
		}
		object, err := prepareGetPickupObject(world, object, false, true)
		if err != nil {
			return StatusDefault, err
		}
		if result, err := moveGetObjectToInventory(world, object, creature.ID, true); err != nil {
			return StatusDefault, err
		} else if !result.Moved {
			continue
		} else if result.Money {
			moneyPicked = true
			heldCount++
			ctx.WriteString("\n" + RenderGetMoneyBalance(result.NewGold))
		} else {
			heldCount++
			carriedWeight += objectWeight
			groups = appendGetObjectGroup(world, groups, object, false)
		}
	}
	if len(groups) == 0 {
		if heavy {
			ctx.WriteString(getInventoryHeavyMessage())
			return StatusDefault, nil
		}
		if moneyPicked {
			return StatusDefault, nil
		}
		ctx.WriteString("그 안에는 아무것도 없습니다.")
		return StatusDefault, nil
	}
	if heavy {
		ctx.WriteString(getInventoryHeavyMessage())
	}
	rendered := renderDropObjectGroups(groups)
	ctx.WriteString(RenderGetFromContainerConfirmation(containerName, rendered))
	_ = roomBroadcast(ctx, room.ID, renderGetFromContainerRoomConfirmation(commandActorDisplayName(player, creature), containerName, rendered))
	return StatusDefault, nil
}

func NewTakeHandler(world GetWorld) Handler {
	return NewGetHandler(world)
}

func GetPlayerIDFromContext(ctx *Context) model.PlayerID {
	if ctx == nil || ctx.ActorID == "" {
		return ""
	}
	return model.PlayerID(ctx.ActorID)
}

func CurrentGetActor(world GetWorld, playerID model.PlayerID) (model.Player, model.Creature, model.Room, error) {
	if world == nil {
		return model.Player{}, model.Creature{}, model.Room{}, ErrGetWorldRequired
	}
	if playerID.IsZero() {
		return model.Player{}, model.Creature{}, model.Room{}, ErrGetActorRequired
	}

	player, ok := world.Player(playerID)
	if !ok {
		return model.Player{}, model.Creature{}, model.Room{}, fmt.Errorf("%w: %q", ErrGetPlayerNotFound, playerID)
	}
	if player.RoomID.IsZero() {
		return player, model.Creature{}, model.Room{}, fmt.Errorf("get: player %q has no room", playerID)
	}
	room, ok := world.Room(player.RoomID)
	if !ok {
		return player, model.Creature{}, model.Room{}, fmt.Errorf("get: room %q not found", player.RoomID)
	}
	if player.CreatureID.IsZero() {
		return player, model.Creature{}, room, fmt.Errorf("%w: player %q", ErrGetCreatureRequired, playerID)
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return player, model.Creature{}, room, fmt.Errorf("%w: %q", ErrGetCreatureNotFound, player.CreatureID)
	}
	if !creature.PlayerID.IsZero() && creature.PlayerID != player.ID {
		return player, model.Creature{}, room,
			fmt.Errorf("get: linked creature %q belongs to player %q", creature.ID, creature.PlayerID)
	}
	return player, creature, room, nil
}

func RenderGetConfirmation(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "물건"
	}
	return "당신은 " + name + krtext.Particle(name, '3') + " 줍습니다.\n"
}

func renderGetRoomConfirmation(actorName, objectName string) string {
	actorName = strings.TrimSpace(actorName)
	if actorName == "" {
		actorName = "누군가"
	}
	objectName = strings.TrimSpace(objectName)
	if objectName == "" {
		objectName = "물건"
	}
	return "\n" + actorName + krtext.Particle(actorName, '1') + " " + objectName + krtext.Particle(objectName, '3') + " 줍습니다."
}

func renderGetFromContainerRoomConfirmation(actorName, containerName, objectName string) string {
	actorName = strings.TrimSpace(actorName)
	if actorName == "" {
		actorName = "누군가"
	}
	containerName = strings.TrimSpace(containerName)
	if containerName == "" {
		containerName = "용기"
	}
	objectName = strings.TrimSpace(objectName)
	if objectName == "" {
		objectName = "물건"
	}
	return "\n" + actorName + krtext.Particle(actorName, '1') + " " + containerName + "에서 " + objectName + krtext.Particle(objectName, '3') + " 꺼냅니다."
}

func RenderGetFromContainerConfirmation(containerName, name string) string {
	containerName = strings.TrimSpace(containerName)
	if containerName == "" {
		containerName = "용기"
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "물건"
	}
	return "당신은 " + containerName + "에서 " + name + krtext.Particle(name, '3') + " 꺼냅니다.\n"
}

func RenderGetMoneyBalance(gold int) string {
	return fmt.Sprintf("당신은 이제 %d냥을 가지고 있습니다.\n", gold)
}

func RenderGetRoomGuardBlock(guard model.Creature) string {
	name := attackCreatureName(guard)
	return name + krtext.Particle(name, '1') + " 당신이 어떤것을 줍는 것을 방해합니다."
}

func RenderGetBlindBlock() string {
	return "그런 건 보이지 않습니다."
}

func commandActorDisplayName(player model.Player, creature model.Creature) string {
	if name := strings.TrimSpace(player.DisplayName); name != "" {
		return name
	}
	if name := strings.TrimSpace(creature.DisplayName); name != "" {
		return name
	}
	if !player.ID.IsZero() {
		return string(player.ID)
	}
	if !creature.ID.IsZero() {
		return string(creature.ID)
	}
	return "누군가"
}

func clearGetActorHidden(world GetWorld, player model.Player, creature model.Creature) (model.Player, model.Creature, error) {
	if updater, ok := world.(interface {
		UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
		UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	}); ok {
		updated, err := updater.UpdateCreatureTags(creature.ID, nil, []string{"hidden", "phiddn", "PHIDDN"})
		if err != nil {
			return player, creature, err
		}
		creature = updated
		if !player.ID.IsZero() {
			updatedPlayer, err := updater.UpdatePlayerTags(player.ID, nil, []string{"hidden", "phiddn", "PHIDDN"})
			if err != nil {
				return player, creature, err
			}
			player = updatedPlayer
		}
	}
	if setter, ok := world.(interface {
		SetCreatureStat(model.CreatureID, string, int) error
	}); ok && creature.Stats != nil {
		if _, ok := creature.Stats["PHIDDN"]; ok {
			if err := setter.SetCreatureStat(creature.ID, "PHIDDN", 0); err != nil {
				return player, creature, err
			}
			creature.Stats["PHIDDN"] = 0
		}
	}
	return player, creature, nil
}

func getActorBlind(player model.Player, creature model.Creature) bool {
	return statusEffectActive(player, creature, "blind", "blinded", "PBLIND", "MBLIND")
}

func getUsesRoomPickupGate(world GetWorld, room model.Room, resolved ResolvedCommand) bool {
	if len(resolved.Args) < 2 {
		return true
	}
	if isContainerGetCommand(resolved) {
		return false
	}
	if _, _, ok := allObjectTarget(getArg(resolved, 1)); ok {
		return false
	}
	if _, _, ok := allObjectTarget(joinArgs(resolved.Args)); ok {
		return true
	}
	_, _, ok := findGetRoomObject(world, room, joinArgs(resolved.Args), getOrdinal(resolved, 0))
	return ok
}

func getRoomPickupGuard(world GetWorld, room model.Room, actorID model.CreatureID) (model.Creature, bool) {
	for _, id := range room.CreatureIDs {
		if id.IsZero() || id == actorID {
			continue
		}
		guard, ok := world.Creature(id)
		if !ok || guard.RoomID != room.ID || attackCreatureIsPlayer(guard) || creatureHPDead(guard) {
			continue
		}
		if creatureHasAnyFlag(guard, "MGUARD", "guardTreasure") {
			return guard, true
		}
	}
	return model.Creature{}, false
}

func firstGetArg(resolved ResolvedCommand) string {
	return getArg(resolved, 0)
}

func getArg(resolved ResolvedCommand, index int) string {
	if index < 0 || index >= len(resolved.Args) {
		return ""
	}
	return strings.TrimSpace(resolved.Args[index])
}

func firstGetOrdinal(resolved ResolvedCommand) int64 {
	return getOrdinal(resolved, 0)
}

func getOrdinal(resolved ResolvedCommand, index int) int64 {
	if index < 0 || index >= len(resolved.Values) || resolved.Values[index] < 1 {
		return 1
	}
	return resolved.Values[index]
}

func findGetTargetObject(
	world objectNameWorld,
	creature model.Creature,
	room model.Room,
	resolved ResolvedCommand,
	detectInvisible bool,
) (model.ObjectInstance, model.ObjectInstance, string, string, bool, string) {
	if len(resolved.Args) == 0 {
		return model.ObjectInstance{}, model.ObjectInstance{}, "", "", false, "뭘 주우시게요?"
	}

	if isContainerGetCommand(resolved) && len(resolved.Args) >= 2 {
		return findGetObjectFromContainer(world, creature, room, getArg(resolved, 0), getOrdinal(resolved, 0), getArg(resolved, 1), getOrdinal(resolved, 1), detectInvisible)
	}

	target := joinArgs(resolved.Args)
	if object, name, ok := findGetRoomObject(world, room, target, getOrdinal(resolved, 0)); ok {
		return object, model.ObjectInstance{}, name, "", false, ""
	}

	if len(resolved.Args) >= 2 {
		object, container, name, containerName, ok, failure := tryGetObjectFromContainer(world, creature, room, getArg(resolved, 0), getOrdinal(resolved, 0), getArg(resolved, 1), getOrdinal(resolved, 1), detectInvisible)
		if ok || failure == "그 안에 그런것은 없어요." || failure == "그것은 담는 종류가 아닙니다." {
			return object, container, name, containerName, ok, failure
		}
	}

	if object, container, name, containerName, ok := findGetObjectFromVisibleContainers(world, creature, room, target, getOrdinal(resolved, 0), detectInvisible); ok {
		return object, container, name, containerName, true, ""
	}

	return model.ObjectInstance{}, model.ObjectInstance{}, "", "", false, "그런건 여기 없어요."
}

func isContainerGetCommand(resolved ResolvedCommand) bool {
	return resolved.Spec.Name == "꺼내" || resolved.Command() == "꺼내"
}

func findGetObjectFromContainer(
	world objectNameWorld,
	creature model.Creature,
	room model.Room,
	containerTarget string,
	containerOrdinal int64,
	target string,
	targetOrdinal int64,
	detectInvisible bool,
) (model.ObjectInstance, model.ObjectInstance, string, string, bool, string) {
	object, container, name, containerName, ok, failure := tryGetObjectFromContainer(world, creature, room, containerTarget, containerOrdinal, target, targetOrdinal, detectInvisible)
	if ok || failure != "" {
		return object, container, name, containerName, ok, failure
	}
	return model.ObjectInstance{}, model.ObjectInstance{}, "", "", false, "그런것은 보이지 않습니다."
}

func tryGetObjectFromContainer(
	world objectNameWorld,
	creature model.Creature,
	room model.Room,
	containerTarget string,
	containerOrdinal int64,
	target string,
	targetOrdinal int64,
	detectInvisible bool,
) (model.ObjectInstance, model.ObjectInstance, string, string, bool, string) {
	container, containerName, ok := findGetVisibleObject(world, creature, room, containerTarget, containerOrdinal, detectInvisible)
	if !ok {
		return model.ObjectInstance{}, model.ObjectInstance{}, "", "", false, ""
	}
	if !isContainerOrCorpse(world, container) {
		return model.ObjectInstance{}, model.ObjectInstance{}, "", "", false, "그것은 담는 종류가 아닙니다."
	}
	object, name, ok := findGetContainerObject(world, container, target, targetOrdinal, detectInvisible)
	if !ok {
		return model.ObjectInstance{}, model.ObjectInstance{}, "", "", false, "그 안에 그런것은 없어요."
	}
	return object, container, name, containerName, true, ""
}

func findGetRoomObject(world objectNameWorld, room model.Room, prefix string, ordinal int64) (model.ObjectInstance, string, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return model.ObjectInstance{}, "", false
	}
	if ordinal < 1 {
		ordinal = 1
	}

	var seen int64
	for _, id := range room.Objects.ObjectIDs {
		if id.IsZero() {
			continue
		}
		object, ok := world.Object(id)
		if !ok || !objectLocatedInRoom(object, room.ID) {
			continue
		}
		if !getObjectMatches(world, object, prefix) {
			continue
		}
		seen++
		if seen == ordinal {
			return object, objectDisplayName(world, object), true
		}
	}
	return model.ObjectInstance{}, "", false
}

func findGetVisibleObject(
	world objectNameWorld,
	creature model.Creature,
	room model.Room,
	prefix string,
	ordinal int64,
	detectInvisible bool,
) (model.ObjectInstance, string, bool) {
	if object, name, ok := findGetObjectInList(world, creature.Inventory.ObjectIDs, prefix, ordinal, func(object model.ObjectInstance) bool {
		return objectLocatedInCreatureInventory(object, creature.ID) && getObjectVisibleForFindObj(world, object, detectInvisible)
	}); ok {
		return object, name, true
	}
	if object, name, ok := findGetObjectInList(world, room.Objects.ObjectIDs, prefix, ordinal, func(object model.ObjectInstance) bool {
		return objectLocatedInRoom(object, room.ID) && getObjectVisibleForFindObj(world, object, detectInvisible)
	}); ok {
		return object, name, true
	}
	return findGetObjectInList(world, getEquipmentObjectIDs(creature), prefix, ordinal, func(object model.ObjectInstance) bool {
		return objectLocatedInCreatureEquipment(object, creature.ID)
	})
}

func findGetObjectFromVisibleContainers(
	world objectNameWorld,
	creature model.Creature,
	room model.Room,
	target string,
	targetOrdinal int64,
	detectInvisible bool,
) (model.ObjectInstance, model.ObjectInstance, string, string, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return model.ObjectInstance{}, model.ObjectInstance{}, "", "", false
	}
	if targetOrdinal < 1 {
		targetOrdinal = 1
	}

	var seen int64
	for _, container := range getVisibleContainers(world, creature, room, detectInvisible) {
		if !isContainerOrCorpse(world, container) {
			continue
		}
		for _, id := range container.Contents.ObjectIDs {
			if id.IsZero() {
				continue
			}
			object, ok := world.Object(id)
			if !ok || !objectLocatedInContainer(object, container.ID) || !getObjectVisibleForFindObj(world, object, detectInvisible) {
				continue
			}
			if !getObjectMatches(world, object, target) {
				continue
			}
			seen++
			if seen == targetOrdinal {
				return object, container, objectDisplayName(world, object), objectDisplayName(world, container), true
			}
		}
	}
	return model.ObjectInstance{}, model.ObjectInstance{}, "", "", false
}

func getVisibleContainers(world objectNameWorld, creature model.Creature, room model.Room, detectInvisible bool) []model.ObjectInstance {
	containers := make([]model.ObjectInstance, 0)
	containers = appendGetVisibleContainers(containers, world, creature.Inventory.ObjectIDs, func(object model.ObjectInstance) bool {
		return objectLocatedInCreatureInventory(object, creature.ID) && getObjectVisibleForFindObj(world, object, detectInvisible)
	})
	containers = appendGetVisibleContainers(containers, world, room.Objects.ObjectIDs, func(object model.ObjectInstance) bool {
		return objectLocatedInRoom(object, room.ID) && getObjectVisibleForFindObj(world, object, detectInvisible)
	})
	containers = appendGetVisibleContainers(containers, world, getEquipmentObjectIDs(creature), func(object model.ObjectInstance) bool {
		return objectLocatedInCreatureEquipment(object, creature.ID)
	})
	return containers
}

func getEquipmentObjectIDs(creature model.Creature) []model.ObjectInstanceID {
	if len(creature.Equipment) == 0 {
		return nil
	}
	slots := make([]string, 0, len(creature.Equipment))
	for slot := range creature.Equipment {
		slots = append(slots, slot)
	}
	slices.Sort(slots)
	ids := make([]model.ObjectInstanceID, 0, len(slots))
	for _, slot := range slots {
		if id := creature.Equipment[slot]; !id.IsZero() {
			ids = append(ids, id)
		}
	}
	return ids
}

func appendGetVisibleContainers(
	containers []model.ObjectInstance,
	world objectNameWorld,
	ids []model.ObjectInstanceID,
	visible func(model.ObjectInstance) bool,
) []model.ObjectInstance {
	for _, id := range ids {
		if id.IsZero() {
			continue
		}
		object, ok := world.Object(id)
		if !ok || !visible(object) || !getObjectIsContainer(world, object) {
			continue
		}
		containers = append(containers, object)
	}
	return containers
}

func findGetObjectInList(
	world objectNameWorld,
	ids []model.ObjectInstanceID,
	prefix string,
	ordinal int64,
	visible func(model.ObjectInstance) bool,
) (model.ObjectInstance, string, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return model.ObjectInstance{}, "", false
	}
	if ordinal < 1 {
		ordinal = 1
	}

	var seen int64
	for _, id := range ids {
		if id.IsZero() {
			continue
		}
		object, ok := world.Object(id)
		if !ok || !visible(object) {
			continue
		}
		if !getObjectMatches(world, object, prefix) {
			continue
		}
		seen++
		if seen == ordinal {
			return object, objectDisplayName(world, object), true
		}
	}
	return model.ObjectInstance{}, "", false
}

func findGetContainerObject(world objectNameWorld, container model.ObjectInstance, prefix string, ordinal int64, detectInvisible bool) (model.ObjectInstance, string, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return model.ObjectInstance{}, "", false
	}
	if ordinal < 1 {
		ordinal = 1
	}

	var seen int64
	for _, id := range container.Contents.ObjectIDs {
		if id.IsZero() {
			continue
		}
		object, ok := world.Object(id)
		if !ok || !objectLocatedInContainer(object, container.ID) || !getObjectVisibleForFindObj(world, object, detectInvisible) {
			continue
		}
		if !getObjectMatches(world, object, prefix) {
			continue
		}
		seen++
		if seen == ordinal {
			return object, objectDisplayName(world, object), true
		}
	}
	return model.ObjectInstance{}, "", false
}

func objectLocatedInRoom(object model.ObjectInstance, roomID model.RoomID) bool {
	return !roomID.IsZero() && object.Location.RoomID == roomID
}

func objectLocatedInContainer(object model.ObjectInstance, containerID model.ObjectInstanceID) bool {
	return !containerID.IsZero() && object.Location.ContainerID == containerID
}

func objectLocatedInCreatureEquipment(object model.ObjectInstance, creatureID model.CreatureID) bool {
	return !creatureID.IsZero() && object.Location.CreatureID == creatureID &&
		object.Location.Slot != "" && object.Location.Slot != "inventory"
}

func getObjectIsContainer(world objectNameWorld, object model.ObjectInstance) bool {
	return objectIsContainer(world, object)
}

func getObjectMatches(world objectNameWorld, object model.ObjectInstance, prefix string) bool {
	return legacyObjectPrefixMatches(world, object, prefix)
}

func getObjectCanBeTaken(world InventoryWorld, object model.ObjectInstance, detectInvisible bool) bool {
	if getObjectIsHidden(world, object) || getObjectIsNoTakeOrScenery(world, object) {
		return false
	}
	if !detectInvisible && getObjectIsInvisible(world, object) {
		return false
	}
	return true
}

func prepareGetPickupObject(world GetWorld, object model.ObjectInstance, clearHidden bool, clearTemporary bool) (model.ObjectInstance, error) {
	removeTags := make([]string, 0, 8)
	if clearHidden {
		removeTags = append(removeTags, "hidden", "ohiddn", "OHIDDN")
	}
	if clearTemporary && getObjectIsTemporaryPermanent(world, object) {
		removeTags = append(removeTags, "temporaryPermanent", "tempPermanent", "otempp", "OTEMPP")
		removeTags = append(removeTags, "inventoryPermanent", "permanent2", "operm2", "OPERM2")
	}
	addTags := []string(nil)
	if getObjectIsPermanent(world, object) {
		removeTags = append(removeTags, "roomPermanent", "permanent", "opermt", "OPERMT")
		addTags = append(addTags, "OPERM2")
	}
	if len(addTags) > 0 || len(removeTags) > 0 {
		if updater, ok := world.(interface {
			UpdateObjectTags(model.ObjectInstanceID, []string, []string) (model.ObjectInstance, error)
		}); ok {
			updated, err := updater.UpdateObjectTags(object.ID, addTags, removeTags)
			if err != nil {
				return object, err
			}
			object = updated
		}
	}
	if setter, ok := world.(interface {
		SetObjectProperty(model.ObjectInstanceID, string, string) (model.ObjectInstance, error)
	}); ok {
		if clearHidden {
			var err error
			object, err = clearGetObjectProperties(setter, object, "hidden", "ohiddn", "OHIDDN")
			if err != nil {
				return object, err
			}
		}
		if clearTemporary && getObjectIsTemporaryPermanent(world, object) {
			var err error
			object, err = clearGetObjectProperties(setter, object,
				"temporaryPermanent", "tempPermanent", "otempp", "OTEMPP",
				"inventoryPermanent", "permanent2", "operm2", "OPERM2",
			)
			if err != nil {
				return object, err
			}
		}
		if getObjectIsPermanent(world, object) {
			var err error
			object, err = clearGetObjectProperties(setter, object, "roomPermanent", "permanent", "opermt", "OPERMT")
			if err != nil {
				return object, err
			}
			object, err = setter.SetObjectProperty(object.ID, "OPERM2", "1")
			if err != nil {
				return object, err
			}
		}
	}
	return object, nil
}

func clearGetObjectProperties(setter interface {
	SetObjectProperty(model.ObjectInstanceID, string, string) (model.ObjectInstance, error)
}, object model.ObjectInstance, keys ...string) (model.ObjectInstance, error) {
	for _, key := range keys {
		if _, ok := object.Properties[key]; !ok {
			continue
		}
		updated, err := setter.SetObjectProperty(object.ID, key, "")
		if err != nil {
			return object, err
		}
		object = updated
	}
	return object, nil
}

func getObjectIsTemporaryPermanent(world InventoryWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object, "temporaryPermanent", "tempPermanent", "otempp", "OTEMPP") ||
		objectHasAnyPropertyFlag(world, object, "temporaryPermanent", "tempPermanent", "otempp", "OTEMPP")
}

func getObjectIsPermanent(world InventoryWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object, "roomPermanent", "permanent", "opermt", "OPERMT") ||
		objectHasAnyPropertyFlag(world, object, "roomPermanent", "permanent", "opermt", "OPERMT")
}

func appendGetObjectGroup(world InventoryWorld, groups []inventoryObjectGroup, object model.ObjectInstance, single bool) []inventoryObjectGroup {
	if !single {
		return appendDropObjectGroup(world, groups, object)
	}
	name := objectDisplayName(world, object)
	return append(groups, inventoryObjectGroup{
		Text:       name,
		Count:      1,
		Name:       fmt.Sprintf("%s\x00%s", name, object.ID),
		Adjustment: objectIntPropertyOrDefault(world, object, "adjustment", "adjust"),
	})
}

func getObjectQuestNumber(world InventoryWorld, object model.ObjectInstance) int {
	return objectIntPropertyOrDefault(world, object, "questNumber", "questnum", "questNum")
}

func getCreatureCompletedQuestSet(creature model.Creature) map[int]bool {
	completed := make(map[int]bool)
	if creature.Properties == nil {
		return completed
	}
	for key, value := range creature.Properties {
		if !strings.HasPrefix(key, "quest_completed_") || value == "" || value == "0" || value == "false" {
			continue
		}
		var questNum int
		if _, err := fmt.Sscanf(key, "quest_completed_%d", &questNum); err == nil && questNum > 0 {
			completed[questNum] = true
		}
	}
	return completed
}

func getCreatureQuestCompleted(creature model.Creature, questNum int) bool {
	if questNum < 1 || creature.Properties == nil {
		return false
	}
	value := creature.Properties[fmt.Sprintf("quest_completed_%d", questNum)]
	return value != "" && value != "0" && value != "false"
}

func getQuestAlreadyCompletedMessage() string {
	return "당신은 그것을 주울수 없습니다. 이미 당신은 임무를 완수하였습니다."
}

func getCompleteQuestPickup(ctx *Context, world GetWorld, creature model.Creature, player model.Player, object model.ObjectInstance, bulk bool) bool {
	questNum := getObjectQuestNumber(world, object)
	if questNum < 1 || getCreatureQuestCompleted(creature, questNum) {
		return false
	}
	if !bulk && dropObjectIsEvent(world, object) && !player.ID.IsZero() {
		if setter, ok := world.(interface {
			SetObjectProperty(model.ObjectInstanceID, string, string) (model.ObjectInstance, error)
		}); ok {
			ownerName := strings.TrimSpace(player.DisplayName)
			if ownerName == "" {
				ownerName = strings.TrimPrefix(string(player.ID), "player:")
			}
			_, _ = setter.SetObjectProperty(object.ID, "key[2]", ownerName)
		}
	}
	if setter, ok := world.(interface {
		SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
	}); ok {
		if updated, err := setter.SetCreatureProperty(creature.ID, fmt.Sprintf("quest_completed_%d", questNum), "1"); err == nil {
			creature = updated
		}
	}
	xp := getQuestExp(questNum - 1)
	if latest, ok := world.Creature(creature.ID); ok {
		creature = latest
	}
	if setter, ok := world.(interface {
		SetCreatureStat(model.CreatureID, string, int) error
	}); ok {
		_ = setter.SetCreatureStat(creature.ID, "experience", creatureStat(creature, "experience")+xp)
	}
	if latest, ok := world.Creature(creature.ID); ok {
		creature = latest
	}
	_ = getAddProf(world, creature, xp)

	if bulk {
		ctx.WriteString("임무를 완수하였습니다! 버리지 마십시요!.\n")
		ctx.WriteString("버리면 다시 주울 수 없습니다.")
		ctx.WriteString(fmt.Sprintf("\n당신은 경험치 %d 을 받았습니다.", xp))
	} else {
		ctx.WriteString("임무를 완수하였습니다. 버리지 마십시요!.")
		ctx.WriteString("버리면 다시는 주울 수 없습니다.")
		ctx.WriteString(fmt.Sprintf("당신은 경험치 %d을 받았습니다.\n", xp))
	}
	return true
}

func getAddProf(world GetWorld, creature model.Creature, exp int) error {
	if exp <= 0 {
		return nil
	}
	part := exp / 9
	if part <= 0 {
		return nil
	}
	setter, ok := world.(interface {
		SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
	})
	if !ok {
		return nil
	}
	for _, key := range []string{
		"proficiency/sharp",
		"proficiency/thrust",
		"proficiency/blunt",
		"proficiency/pole",
		"proficiency/missile",
		"realm/1",
		"realm/2",
		"realm/3",
		"realm/4",
	} {
		current, _ := parseObjectInt(creature.Properties[key])
		if _, err := setter.SetCreatureProperty(creature.ID, key, fmt.Sprintf("%d", current+part)); err != nil {
			return err
		}
	}
	return nil
}

func getObjectIsHidden(world InventoryWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object, "hidden", "ohiddn", "OHIDDN") ||
		objectHasAnyPropertyFlag(world, object, "hidden", "ohiddn", "OHIDDN")
}

func getObjectIsInvisible(world InventoryWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object, "invisible", "oinvis", "OINVIS") ||
		objectHasAnyPropertyFlag(world, object, "invisible", "oinvis", "OINVIS")
}

func getObjectVisibleForFindObj(world objectNameWorld, object model.ObjectInstance, detectInvisible bool) bool {
	if detectInvisible {
		return true
	}
	if hasAnyNormalizedFlag(object.Metadata.Tags, "invisible", "oinvis", "OINVIS") {
		return false
	}
	targets := normalizedFlagSet("invisible", "oinvis", "OINVIS")
	if objectPropertiesHaveAnyFlag(object.Properties, targets) {
		return false
	}
	if object.PrototypeID.IsZero() {
		return true
	}
	proto, ok := world.ObjectPrototype(object.PrototypeID)
	if !ok {
		return true
	}
	if hasAnyNormalizedFlag(proto.Metadata.Tags, "invisible", "oinvis", "OINVIS") {
		return false
	}
	return !objectPropertiesHaveAnyFlag(proto.Properties, targets)
}

func getObjectIsNoTakeOrScenery(world InventoryWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object,
		"noTake", "notTake", "onotak", "notak", "ONOTAK",
		"scenery", "scene", "oscene", "OSCENE",
	) || objectHasAnyPropertyFlag(world, object,
		"noTake", "notTake", "onotak", "notak", "ONOTAK",
		"scenery", "scene", "oscene", "OSCENE",
	)
}

func getTakeCapacityExceeded(world InventoryWorld, creature model.Creature, object model.ObjectInstance, includeWeight bool) bool {
	if getTakeCreatureHeldCount(world, creature) > 150 {
		return true
	}
	if !includeWeight {
		return false
	}
	return getTakeCreatureCarriedWeight(world, creature)+getTakeObjectTotalWeight(world, object) > getTakeCreatureMaxWeight(creature)
}

func getInventoryFullMessage() string {
	return "당신은 더이상 가질 수 없습니다.\n"
}

func getInventoryHeavyMessage() string {
	return "가지고 있는 물건이 너무 무거워 들 수가 없습니다.\n"
}

func getTakeCreatureHeldCount(world InventoryWorld, creature model.Creature) int {
	inventoryCount := 0
	for _, objectID := range creature.Inventory.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		inventoryCount++
		if object, ok := world.Object(objectID); ok && objectIsContainer(world, object) {
			inventoryCount += getTakeObjectContainerCount(world, object)
		}
	}
	if inventoryCount > 200 {
		inventoryCount = 200
	}
	equipmentCount := 0
	for _, objectID := range creature.Equipment {
		if !objectID.IsZero() {
			equipmentCount++
		}
	}
	return inventoryCount + equipmentCount
}

func getTakeObjectContainerCount(world InventoryWorld, object model.ObjectInstance) int {
	for _, key := range []string{"shotsCurrent", "shotscur", "shotsCur", "contentsCount"} {
		if count, ok := objectIntProperty(world, object, key); ok {
			return count
		}
	}
	return len(object.Contents.ObjectIDs)
}

func getTakeCreatureCarriedWeight(world InventoryWorld, creature model.Creature) int {
	seen := map[model.ObjectInstanceID]struct{}{}
	weight := 0
	for _, objectID := range creature.Inventory.ObjectIDs {
		weight += getTakeCarriedObjectWeight(world, objectID, true, seen)
	}
	for _, objectID := range creature.Equipment {
		weight += getTakeCarriedObjectWeight(world, objectID, false, seen)
	}
	return weight
}

func getTakeCarriedObjectWeight(world InventoryWorld, objectID model.ObjectInstanceID, skipWeightless bool, seen map[model.ObjectInstanceID]struct{}) int {
	if objectID.IsZero() {
		return 0
	}
	if _, exists := seen[objectID]; exists {
		return 0
	}
	seen[objectID] = struct{}{}
	object, ok := world.Object(objectID)
	if !ok {
		return 0
	}
	if skipWeightless && getTakeObjectWeightless(world, object) {
		return 0
	}
	weight := getTakeObjectOwnWeight(world, object)
	for _, childID := range object.Contents.ObjectIDs {
		weight += getTakeCarriedObjectWeight(world, childID, true, seen)
	}
	return weight
}

func getTakeObjectTotalWeight(world InventoryWorld, object model.ObjectInstance) int {
	seen := map[model.ObjectInstanceID]struct{}{}
	weight := getTakeObjectOwnWeight(world, object)
	for _, childID := range object.Contents.ObjectIDs {
		weight += getTakeCarriedObjectWeight(world, childID, true, seen)
	}
	return weight
}

func getTakeObjectOwnWeight(world InventoryWorld, object model.ObjectInstance) int {
	if weight, ok := objectIntProperty(world, object, "weight"); ok {
		return weight
	}
	return 0
}

func getTakeObjectWeightless(world InventoryWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object, "weightless", "owtles") ||
		objectHasAnyPropertyFlag(world, object, "weightless", "owtles")
}

func getTakeCreatureMaxWeight(creature model.Creature) int {
	strength := creatureStat(creature, "strength")
	level := creature.Level
	if level == 0 {
		level = creatureStat(creature, "level")
	}
	maxWeight := 20 + strength*10
	if creatureStat(creature, "class") == model.ClassBarbarian {
		maxWeight += ((level + 3) / 4) * 10
	}
	return maxWeight
}

// CorpseLootPolicy determines how to handle looting another player's corpse.
// Supported values:
// - "block" (default): block the action entirely.
// - "penalty": allow looting but apply the legacy PK flag/penalty (plykl cooldown).
// - "allow": allow looting without penalty.
var CorpseLootPolicy = "block"

func getCorpseLootPolicy() string {
	if env := os.Getenv("MUHAN_CORPSE_LOOT_POLICY"); env != "" {
		return env
	}
	if env := os.Getenv("CORPSE_LOOT_POLICY"); env != "" {
		return env
	}
	return CorpseLootPolicy
}

func isContainerOrCorpse(world objectNameWorld, object model.ObjectInstance) bool {
	if getObjectIsContainer(world, object) {
		return true
	}
	name := strings.ToLower(objectDisplayName(world, object))
	return strings.Contains(name, "시체") || strings.Contains(name, "corpse")
}

func handleCorpseLootCheck(
	ctx *Context,
	world GetWorld,
	actorCreature model.Creature,
	container model.ObjectInstance,
) (allowed bool, err error) {
	displayName := objectDisplayName(world, container)
	if !strings.HasSuffix(displayName, "의 시체") {
		return true, nil
	}

	possibleOwner := strings.TrimSuffix(displayName, "의 시체")

	ownerPlayerID := model.PlayerID("player:" + strings.ToLower(possibleOwner))
	ownerPlayer, exists := world.Player(ownerPlayerID)
	if !exists {
		ownerPlayerID = model.PlayerID(possibleOwner)
		ownerPlayer, exists = world.Player(ownerPlayerID)
	}

	if !exists {
		return true, nil
	}

	actorPlayerID := model.PlayerID(actorCreature.PlayerID)
	if actorPlayerID.IsZero() {
		ctx.WriteString("플레이어가 아니면 시체를 만질 수 없습니다.\n")
		return false, nil
	}
	actorPlayer, ok := world.Player(actorPlayerID)
	if !ok {
		return true, nil
	}

	if ownerPlayer.ID == actorPlayer.ID {
		return true, nil
	}

	policy := getCorpseLootPolicy()
	switch strings.ToLower(policy) {
	case "penalty":
		if cw, ok := world.(interface {
			SetCreatureCooldown(model.CreatureID, string, int64, int64) error
		}); ok {
			minSec := int64(7 * 86400)
			maxSec := int64(10 * 86400)
			interval := rand.Int63n(maxSec-minSec+1) + minSec
			if err := cw.SetCreatureCooldown(actorCreature.ID, "plykl", time.Now().Unix(), interval); err != nil {
				return false, err
			}
			ctx.WriteString("다른 사람의 시체에 손을 대어 범죄자(PK) 대기 시간이 설정되었습니다!\n")
		}
		return true, nil
	case "allow":
		return true, nil
	default: // "block"
		ctx.WriteString("다른 사람의 시체는 만질 수 없습니다.\n")
		return false, nil
	}
}
