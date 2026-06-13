package command

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

var (
	ErrDropWorldRequired    = errors.New("drop world required")
	ErrDropActorRequired    = errors.New("drop actor required")
	ErrDropPlayerNotFound   = errors.New("drop player not found")
	ErrDropCreatureRequired = errors.New("drop creature required")
	ErrDropCreatureNotFound = errors.New("drop creature not found")
	ErrDropRoomRequired     = errors.New("drop room required")
	ErrDropRoomNotFound     = errors.New("drop room not found")
)

type DropWorld interface {
	InventoryWorld
	Room(model.RoomID) (model.Room, bool)
	MoveObject(model.ObjectInstanceID, model.ObjectLocation) error

	// B: Persistence hook
	SavePlayer(model.PlayerID) error
}

type dropContainerStoreWorld interface {
	StoreCreatureInventoryObjectInContainer(model.ObjectInstanceID, model.CreatureID, model.ObjectInstanceID, int) (int, bool, bool, error)
}

type dropObjectDestroyWorld interface {
	DestroyObject(model.ObjectInstanceID) error
}

type dropCreatureInventoryDestroyWorld interface {
	DestroyCreatureInventoryObject(model.ObjectInstanceID, model.CreatureID) (bool, error)
}

type dropGoldWorld interface {
	DropCreatureGoldToRoom(model.CreatureID, model.RoomID, int) (model.ObjectInstanceID, int, bool, error)
}

func NewDropHandler(world DropWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		if world == nil {
			return StatusDefault, ErrDropWorldRequired
		}

		playerID := DropPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrDropActorRequired
		}

		player, creature, err := CurrentDropCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}

		target := dropTarget(resolved)
		if target == "" {
			ctx.WriteString("무엇을 버리실려구요?")
			return StatusDefault, nil
		}

		player, creature, err = clearDropActorHidden(world, player, creature)
		if err != nil {
			return StatusDefault, err
		}

		roomID, err := currentDropRoomID(world, player, creature)
		if err != nil {
			return StatusDefault, err
		}

		if secondDropArg(resolved) == "" {
			if amount, ok := parseDropMoneyAmount(target); ok {
				return dropMoneyToRoom(ctx, world, player, creature, roomID, amount)
			}
		}
		detectInvisible := inventoryViewerDetectsInvisible(player, creature)
		if isContainerDropCommand(resolved) && secondDropArg(resolved) != "" {
			if all, filter, ok := allObjectTarget(firstDropArg(resolved)); ok {
				return dropAllObjectsIntoContainer(ctx, world, roomID, player, creature, secondDropArg(resolved), secondDropOrdinal(resolved), all, filter, detectInvisible)
			}
		}
		if all, filter, ok := allObjectTarget(target); ok {
			return dropAllObjectsToRoom(ctx, world, roomID, player, creature, all, filter, detectInvisible)
		}

		if isContainerDropCommand(resolved) && secondDropArg(resolved) != "" {
			objectID, name, ok := selectDropObjectWithVisibility(world, creature.Inventory.ObjectIDs, firstDropArg(resolved), firstDropOrdinal(resolved), detectInvisible)
			if !ok {
				ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
				return StatusDefault, nil
			}
			room, ok := world.Room(roomID)
			if !ok {
				return StatusDefault, fmt.Errorf("%w: %q", ErrDropRoomNotFound, roomID)
			}
			containerID, containerName, ok, failure := selectDropContainer(world, room, creature, secondDropArg(resolved), secondDropOrdinal(resolved), objectID, detectInvisible)
			if !ok {
				ctx.WriteString(failure)
				return StatusDefault, nil
			}
			if full, err := selectedDropContainerFull(world, containerID); err != nil {
				return StatusDefault, err
			} else if full {
				ctx.WriteString(containerName + "안에 더이상 넣을 수 없습니다.\n")
				return StatusDefault, nil
			}
			if dropObjectIDIsContainer(world, objectID) {
				ctx.WriteString("담을수 있는 물건 안에 담을 수 있는 물건은 넣을 수 없습니다.\n")
				return StatusDefault, nil
			}

			if moved, full, devoured, err := storeDropObjectInContainer(world, objectID, creature.ID, containerID, true); err != nil {
				return StatusDefault, fmt.Errorf("put object %q into container %q: %w", objectID, containerID, err)
			} else if full {
				ctx.WriteString(containerName + "안에 더이상 넣을 수 없습니다.\n")
				return StatusDefault, nil
			} else if !moved {
				ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
				return StatusDefault, nil
			} else if devoured {
				queueDropPlayerSave(world, creature.PlayerID)
				_ = roomBroadcast(ctx, roomID, renderPutRoomConfirmation(commandActorDisplayName(player, creature), name, containerName))
				ctx.WriteString(RenderDevourConfirmation(name, containerName))
				return StatusDefault, nil
			}

			queueDropPlayerSave(world, creature.PlayerID)
			ctx.WriteString(RenderPutConfirmation(name, containerName))
			_ = roomBroadcast(ctx, roomID, renderPutRoomConfirmation(commandActorDisplayName(player, creature), name, containerName))
			return StatusDefault, nil
		}

		objectID, name, ok := selectDropObjectWithVisibility(world, creature.Inventory.ObjectIDs, target, firstDropOrdinal(resolved), detectInvisible)
		if !ok {
			if secondDropArg(resolved) != "" {
				if fallbackObjectID, fallbackName, fallbackOK := selectDropObjectWithVisibility(world, creature.Inventory.ObjectIDs, firstDropArg(resolved), firstDropOrdinal(resolved), detectInvisible); fallbackOK {
					room, ok := world.Room(roomID)
					if !ok {
						return StatusDefault, fmt.Errorf("%w: %q", ErrDropRoomNotFound, roomID)
					}
					containerID, containerName, ok, failure := selectDropContainer(world, room, creature, secondDropArg(resolved), secondDropOrdinal(resolved), fallbackObjectID, detectInvisible)
					if !ok {
						ctx.WriteString(failure)
						return StatusDefault, nil
					}
					if full, err := selectedDropContainerFull(world, containerID); err != nil {
						return StatusDefault, err
					} else if full {
						ctx.WriteString(containerName + "안에 더이상 넣을 수 없습니다.\n")
						return StatusDefault, nil
					}
					if dropObjectIDIsContainer(world, fallbackObjectID) {
						ctx.WriteString("담을수 있는 물건 안에 담을 수 있는 물건은 넣을 수 없습니다.\n")
						return StatusDefault, nil
					}
					if moved, full, devoured, err := storeDropObjectInContainer(world, fallbackObjectID, creature.ID, containerID, true); err != nil {
						return StatusDefault, fmt.Errorf("put object %q into container %q: %w", fallbackObjectID, containerID, err)
					} else if full {
						ctx.WriteString(containerName + "안에 더이상 넣을 수 없습니다.\n")
						return StatusDefault, nil
					} else if !moved {
						ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
						return StatusDefault, nil
					} else if devoured {
						queueDropPlayerSave(world, creature.PlayerID)
						_ = roomBroadcast(ctx, roomID, renderPutRoomConfirmation(commandActorDisplayName(player, creature), fallbackName, containerName))
						ctx.WriteString(RenderDevourConfirmation(fallbackName, containerName))
						return StatusDefault, nil
					}
					queueDropPlayerSave(world, creature.PlayerID)
					ctx.WriteString(RenderPutConfirmation(fallbackName, containerName))
					_ = roomBroadcast(ctx, roomID, renderPutRoomConfirmation(commandActorDisplayName(player, creature), fallbackName, containerName))
					return StatusDefault, nil
				}
			}
			ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
			return StatusDefault, nil
		}

		object, ok := world.Object(objectID)
		if !ok {
			return StatusDefault, fmt.Errorf("drop object %q: object not found", objectID)
		}
		if failure := dropSingleRoomProtectedMessage(world, creature, object); failure != "" {
			ctx.WriteString(failure)
			return StatusDefault, nil
		}

		if room, ok := world.Room(roomID); ok && dropRoomIsDump(room) {
			if destroyed, err := destroyDropInventoryObject(world, objectID, creature.ID); err != nil {
				return StatusDefault, err
			} else if !destroyed {
				ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
				return StatusDefault, nil
			}
			if err := applyDropDumpReward(world, creature, 10, 2); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(RenderDropConfirmation(name))
			_ = roomBroadcast(ctx, roomID, renderDropRoomConfirmation(commandActorDisplayName(player, creature), name))
			ctx.WriteString(RenderDropDumpReward(true))
			queueDropPlayerSave(world, creature.PlayerID)
			return StatusDefault, nil
		}

		if err := world.MoveObject(objectID, model.ObjectLocation{RoomID: roomID}); err != nil {
			return StatusDefault, fmt.Errorf("drop object %q: %w", objectID, err)
		}

		ctx.WriteString(RenderDropConfirmation(name))
		_ = roomBroadcast(ctx, roomID, renderDropRoomConfirmation(commandActorDisplayName(player, creature), name))

		queueDropPlayerSave(world, creature.PlayerID)
		return StatusDefault, nil
	}
}

func parseDropMoneyAmount(target string) (int, bool) {
	target = strings.TrimSpace(target)
	if !strings.HasSuffix(target, "냥") {
		return 0, false
	}
	amountText := strings.TrimSpace(strings.TrimSuffix(target, "냥"))
	if amountText == "" {
		return 0, false
	}
	return legacyAtoiPrefix(amountText), true
}

func legacyAtoiPrefix(text string) int {
	text = strings.TrimLeftFunc(text, unicode.IsSpace)
	if text == "" {
		return 0
	}
	sign := 1
	if text[0] == '+' || text[0] == '-' {
		if text[0] == '-' {
			sign = -1
		}
		text = text[1:]
	}
	value := 0
	for _, r := range text {
		if r < '0' || r > '9' {
			break
		}
		value = value*10 + int(r-'0')
	}
	return sign * value
}

func dropMoneyToRoom(ctx *Context, world DropWorld, player model.Player, creature model.Creature, roomID model.RoomID, amount int) (Status, error) {
	if amount < 1 {
		ctx.WriteString("돈의 단위는 음수가 될수 없습니다.")
		return StatusDefault, nil
	}
	dropper, ok := world.(dropGoldWorld)
	if !ok {
		return StatusDefault, fmt.Errorf("drop money: world does not support money objects")
	}
	if _, _, ok, err := dropper.DropCreatureGoldToRoom(creature.ID, roomID, amount); err != nil {
		return StatusDefault, err
	} else if !ok {
		ctx.WriteString("당신은 그만큼의 돈을 가지고 있지 않습니다.")
		return StatusDefault, nil
	}
	ctx.WriteString(fmt.Sprintf("당신은 %d냥을 버렸습니다.\n", amount))
	_ = roomBroadcast(ctx, roomID, renderDropMoneyRoomConfirmation(commandActorDisplayName(player, creature), amount))
	queueDropPlayerSave(world, creature.PlayerID)
	return StatusDefault, nil
}

func storeDropObjectInContainer(world DropWorld, objectID model.ObjectInstanceID, creatureID model.CreatureID, containerID model.ObjectInstanceID, checkFullBeforeDevour bool) (moved bool, full bool, devoured bool, err error) {
	object, ok := world.Object(objectID)
	if !ok {
		return false, false, false, nil
	}
	if objectIsContainer(world, object) {
		return false, false, false, nil
	}
	container, ok := world.Object(containerID)
	if !ok {
		return false, false, false, nil
	}
	if dropContainerDevours(world, container) {
		if checkFullBeforeDevour && dropContainerFull(world, container) {
			return false, true, false, nil
		}
		destroyed, err := destroyDropInventoryObject(world, objectID, creatureID)
		return destroyed, false, destroyed, err
	}
	if storer, ok := world.(dropContainerStoreWorld); ok {
		_, stored, full, err := storer.StoreCreatureInventoryObjectInContainer(objectID, creatureID, containerID, dropContainerMax(world, containerID))
		return stored, full, false, err
	}
	if err := world.MoveObject(objectID, model.ObjectLocation{ContainerID: containerID}); err != nil {
		return false, false, false, err
	}
	return true, false, false, nil
}

func destroyDropInventoryObject(world DropWorld, objectID model.ObjectInstanceID, creatureID model.CreatureID) (bool, error) {
	object, ok := world.Object(objectID)
	if !ok || !objectLocatedInCreatureInventory(object, creatureID) {
		return false, nil
	}
	if destroyer, ok := world.(dropObjectDestroyWorld); ok {
		if err := destroyer.DestroyObject(objectID); err != nil {
			return false, err
		}
		return true, nil
	}
	if destroyer, ok := world.(dropCreatureInventoryDestroyWorld); ok {
		return destroyer.DestroyCreatureInventoryObject(objectID, creatureID)
	}
	return false, fmt.Errorf("world does not support destroying inventory objects")
}

func dropRoomIsDump(room model.Room) bool {
	return roomHasAnyFlag(room, "dump", "dumpRoom", "RDUMPR", "rdumpr")
}

func applyDropDumpReward(world DropWorld, creature model.Creature, goldReward int, experienceReward int) error {
	setter, ok := world.(interface {
		SetCreatureStat(model.CreatureID, string, int) error
	})
	if !ok {
		return fmt.Errorf("drop dump room: world does not support creature stat updates")
	}
	if goldReward != 0 {
		if err := setter.SetCreatureStat(creature.ID, "gold", creatureStat(creature, "gold")+goldReward); err != nil {
			return err
		}
	}
	if experienceReward != 0 {
		if err := setter.SetCreatureStat(creature.ID, "experience", creatureStat(creature, "experience")+experienceReward); err != nil {
			return err
		}
	}
	return nil
}

func clearDropActorHidden(world DropWorld, player model.Player, creature model.Creature) (model.Player, model.Creature, error) {
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

func dropObjectIDIsContainer(world DropWorld, objectID model.ObjectInstanceID) bool {
	object, ok := world.Object(objectID)
	return ok && objectIsContainer(world, object)
}

func dropContainerMax(world InventoryWorld, containerID model.ObjectInstanceID) int {
	container, ok := world.Object(containerID)
	if !ok {
		return 0
	}
	if maxCount, ok := objectIntProperty(world, container, "shotsMax"); ok && maxCount > 0 {
		return maxCount
	}
	return 0
}

func dropContainerCurrent(world InventoryWorld, container model.ObjectInstance) int {
	if count, ok := objectIntProperty(world, container, "shotsCurrent"); ok {
		return count
	}
	return len(container.Contents.ObjectIDs)
}

func dropContainerFull(world InventoryWorld, container model.ObjectInstance) bool {
	maxCount := dropContainerMax(world, container.ID)
	return maxCount > 0 && dropContainerCurrent(world, container) >= maxCount
}

func selectedDropContainerFull(world DropWorld, containerID model.ObjectInstanceID) (bool, error) {
	container, ok := world.Object(containerID)
	if !ok {
		return false, fmt.Errorf("put container %q: object not found", containerID)
	}
	return dropContainerFull(world, container), nil
}

func dropContainerDevours(world InventoryWorld, container model.ObjectInstance) bool {
	return objectHasAnyTag(world, container, "OCNDES", "ocndes", "devours", "containerDevours") ||
		objectHasAnyPropertyFlag(world, container, "OCNDES", "ocndes", "devours", "containerDevours")
}

func dropSingleRoomProtectedMessage(world InventoryWorld, creature model.Creature, object model.ObjectInstance) string {
	if creatureClass(creature) >= model.ClassDM {
		return ""
	}
	if dropObjectHasQuestNumber(world, object) {
		return "임무 아이템은 버리지 못합니다."
	}
	if dropObjectIsEvent(world, object) {
		return "이벤트 아이템은 버리지 못합니다."
	}
	for _, childID := range object.Contents.ObjectIDs {
		child, ok := world.Object(childID)
		if !ok {
			continue
		}
		if dropObjectHasQuestNumber(world, child) {
			return "임무 아이템이 들어있으면 버리지 못합니다."
		}
		if dropObjectIsEvent(world, child) {
			return "이벤트 아이템이 들어있으면 버리지 못합니다."
		}
	}
	return ""
}

func dropObjectHasQuestNumber(world InventoryWorld, object model.ObjectInstance) bool {
	return objectIntPropertyOrZero(world, object, "questNumber") != 0 ||
		objectIntPropertyOrZero(world, object, "questnum") != 0 ||
		objectIntPropertyOrZero(world, object, "questNum") != 0
}

func dropObjectIsEvent(world InventoryWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object, "event", "oevent", "OEVENT") ||
		objectHasAnyPropertyFlag(world, object, "event", "oevent", "OEVENT")
}

func dropBulkObjectMovable(world DropWorld, creature model.Creature, object model.ObjectInstance, detectInvisible bool) bool {
	if !detectInvisible && dropObjectIsInvisible(world, object) {
		return false
	}
	if dropObjectHasQuestNumber(world, object) && creatureClass(creature) < model.ClassDM {
		return false
	}
	if dropObjectIsEvent(world, object) {
		return false
	}
	return true
}

func dropObjectIsInvisible(world InventoryWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object, "invisible", "oinvis", "OINVIS") ||
		objectHasAnyPropertyFlag(world, object, "invisible", "oinvis", "OINVIS")
}

func appendDropObjectGroup(world InventoryWorld, groups []inventoryObjectGroup, object model.ObjectInstance) []inventoryObjectGroup {
	name := objectDisplayName(world, object)
	adjustment := objectIntPropertyOrDefault(world, object, "adjustment", "adjust")
	last := len(groups) - 1
	if last >= 0 && groups[last].Name == name && groups[last].Adjustment == adjustment {
		groups[last].Count++
		return groups
	}
	return append(groups, inventoryObjectGroup{
		Text:       name,
		Count:      1,
		Name:       name,
		Adjustment: adjustment,
	})
}

func renderDropObjectGroups(groups []inventoryObjectGroup) string {
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		if group.Count > 1 {
			names = append(names, fmt.Sprintf("(x%d) %s", group.Count, group.Text))
			continue
		}
		names = append(names, group.Text)
	}
	return strings.Join(names, ", ")
}

func dropAllObjectsToRoom(ctx *Context, world DropWorld, roomID model.RoomID, player model.Player, creature model.Creature, all bool, filter string, detectInvisible bool) (Status, error) {
	groups := make([]inventoryObjectGroup, 0, len(creature.Inventory.ObjectIDs))
	room, ok := world.Room(roomID)
	if !ok {
		return StatusDefault, fmt.Errorf("%w: %q", ErrDropRoomNotFound, roomID)
	}
	dumpRoom := dropRoomIsDump(room)
	dumpCount := 0
	objectIDs := append([]model.ObjectInstanceID(nil), creature.Inventory.ObjectIDs...)
	for _, objectID := range objectIDs {
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInCreatureInventory(object, creature.ID) || !dropBulkObjectMovable(world, creature, object, detectInvisible) {
			continue
		}
		if !all && !dropObjectMatches(world, object, func(name string) bool { return strings.HasPrefix(name, filter) }) {
			continue
		}
		if dumpRoom {
			groups = appendDropObjectGroup(world, groups, object)
			if destroyed, err := destroyDropInventoryObject(world, objectID, creature.ID); err != nil {
				return StatusDefault, err
			} else if destroyed {
				dumpCount++
			}
			continue
		}
		if err := world.MoveObject(objectID, model.ObjectLocation{RoomID: roomID}); err != nil {
			return StatusDefault, fmt.Errorf("drop object %q: %w", objectID, err)
		}
		groups = appendDropObjectGroup(world, groups, object)
	}
	if len(groups) == 0 {
		ctx.WriteString("당신은 아무것도 가지고 있지 않습니다.")
		return StatusDefault, nil
	}
	rendered := renderDropObjectGroups(groups)
	ctx.WriteString(RenderDropConfirmation(rendered))
	_ = roomBroadcast(ctx, roomID, renderDropRoomConfirmation(commandActorDisplayName(player, creature), rendered))
	if dumpCount > 0 {
		if err := applyDropDumpReward(world, creature, dumpCount*10, 0); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(RenderDropDumpReward(false))
	}
	queueDropPlayerSave(world, creature.PlayerID)
	return StatusDefault, nil
}

func dropAllObjectsIntoContainer(
	ctx *Context,
	world DropWorld,
	roomID model.RoomID,
	player model.Player,
	creature model.Creature,
	containerTarget string,
	containerOrdinal int64,
	all bool,
	filter string,
	detectInvisible bool,
) (Status, error) {
	room, ok := world.Room(roomID)
	if !ok {
		return StatusDefault, fmt.Errorf("%w: %q", ErrDropRoomNotFound, roomID)
	}
	containerID, containerName, ok, failure := selectDropContainer(world, room, creature, containerTarget, containerOrdinal, "", detectInvisible)
	if !ok {
		ctx.WriteString(failure)
		return StatusDefault, nil
	}

	groups := make([]inventoryObjectGroup, 0, len(creature.Inventory.ObjectIDs))
	devouredAny := false
	skipped := false
	objectIDs := append([]model.ObjectInstanceID(nil), creature.Inventory.ObjectIDs...)
	for _, objectID := range objectIDs {
		if objectID == containerID {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInCreatureInventory(object, creature.ID) {
			continue
		}
		if !all && !dropObjectMatches(world, object, func(name string) bool { return strings.HasPrefix(name, filter) }) {
			continue
		}
		if !dropBulkObjectMovable(world, creature, object, detectInvisible) || objectIsContainer(world, object) {
			skipped = true
			continue
		}
		if moved, full, devoured, err := storeDropObjectInContainer(world, objectID, creature.ID, containerID, false); err != nil {
			return StatusDefault, fmt.Errorf("put object %q into container %q: %w", objectID, containerID, err)
		} else if full {
			skipped = true
			continue
		} else if !moved {
			continue
		} else if devoured {
			devouredAny = true
			continue
		}
		groups = appendDropObjectGroup(world, groups, object)
	}
	if len(groups) == 0 {
		ctx.WriteString("당신은 그것 안에 넣을 물건을 아무것도 갖고 있지 않습니다.")
		if devouredAny {
			queueDropPlayerSave(world, creature.PlayerID)
		}
		return StatusDefault, nil
	}
	if skipped {
		ctx.WriteString(containerName + "안에 더이상 물건을 넣을 수 없습니다.")
	}
	if len(groups) > 0 {
		rendered := renderDropObjectGroups(groups)
		ctx.WriteString(RenderPutConfirmation(rendered, containerName))
		_ = roomBroadcast(ctx, roomID, renderPutRoomConfirmation(commandActorDisplayName(player, creature), rendered, containerName))
	}
	queueDropPlayerSave(world, creature.PlayerID)
	return StatusDefault, nil
}

func queueDropPlayerSave(world DropWorld, playerID model.PlayerID) {
	if playerID.IsZero() {
		return
	}
	if w, ok := world.(interface {
		MarkPlayerDirty(model.PlayerID)
		QueueSave(model.PlayerID, model.BankID)
	}); ok {
		w.MarkPlayerDirty(playerID)
		w.QueueSave(playerID, "")
		return
	}
	_ = world.SavePlayer(playerID)
}

func DropPlayerIDFromContext(ctx *Context) model.PlayerID {
	if ctx == nil || ctx.ActorID == "" {
		return ""
	}
	return model.PlayerID(ctx.ActorID)
}

func CurrentDropCreature(world DropWorld, playerID model.PlayerID) (model.Player, model.Creature, error) {
	if world == nil {
		return model.Player{}, model.Creature{}, ErrDropWorldRequired
	}
	if playerID.IsZero() {
		return model.Player{}, model.Creature{}, ErrDropActorRequired
	}

	player, ok := world.Player(playerID)
	if !ok {
		return model.Player{}, model.Creature{}, fmt.Errorf("%w: %q", ErrDropPlayerNotFound, playerID)
	}
	if player.CreatureID.IsZero() {
		return player, model.Creature{}, fmt.Errorf("%w: player %q", ErrDropCreatureRequired, playerID)
	}

	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return player, model.Creature{}, fmt.Errorf("%w: %q", ErrDropCreatureNotFound, player.CreatureID)
	}
	return player, creature, nil
}

func RenderDropConfirmation(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "물건"
	}
	return "당신은 " + name + krtext.Particle(name, '3') + " 버렸습니다.\n"
}

func renderDropRoomConfirmation(actorName, objectName string) string {
	actorName = strings.TrimSpace(actorName)
	if actorName == "" {
		actorName = "누군가"
	}
	objectName = strings.TrimSpace(objectName)
	if objectName == "" {
		objectName = "물건"
	}
	return "\n" + actorName + krtext.Particle(actorName, '1') + " " + objectName + krtext.Particle(objectName, '3') + " 버렸습니다."
}

func renderDropMoneyRoomConfirmation(actorName string, amount int) string {
	actorName = strings.TrimSpace(actorName)
	if actorName == "" {
		actorName = "누군가"
	}
	return fmt.Sprintf("\n%s%s %d냥을 버렸습니다.", actorName, krtext.Particle(actorName, '1'), amount)
}

func renderPutRoomConfirmation(actorName, objectName, containerName string) string {
	actorName = strings.TrimSpace(actorName)
	if actorName == "" {
		actorName = "누군가"
	}
	objectName = strings.TrimSpace(objectName)
	if objectName == "" {
		objectName = "물건"
	}
	containerName = strings.TrimSpace(containerName)
	if containerName == "" {
		containerName = "용기"
	}
	return "\n" + actorName + krtext.Particle(actorName, '1') + " " + objectName + krtext.Particle(objectName, '3') + " " + containerName + " 안에 넣습니다."
}

func RenderPutConfirmation(name, containerName string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "물건"
	}
	containerName = strings.TrimSpace(containerName)
	if containerName == "" {
		containerName = "용기"
	}
	return "당신은 " + name + krtext.Particle(name, '3') + " " + containerName + " 안에 넣습니다.\n"
}

func RenderDevourConfirmation(name, containerName string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "물건"
	}
	containerName = strings.TrimSpace(containerName)
	if containerName == "" {
		containerName = "용기"
	}
	return name + krtext.Particle(name, '3') + " " + containerName + krtext.Particle(containerName, '1') + " 삼켜 버려 흔적도 없이 사라집니다!\n"
}

func RenderDropDumpReward(includeExperience bool) string {
	if includeExperience {
		return "당신의 물건을 제물로 바쳤습니다.\n당신은 약간의 상금과 경험을 받았습니다."
	}
	return "당신의 물건을 제물로 바쳤습니다.\n당신은 약간의 상금을 받았습니다."
}

func firstDropArg(resolved ResolvedCommand) string {
	if len(resolved.Args) == 0 {
		return ""
	}
	return strings.TrimSpace(resolved.Args[0])
}

func secondDropArg(resolved ResolvedCommand) string {
	if len(resolved.Args) < 2 {
		return ""
	}
	return strings.TrimSpace(resolved.Args[1])
}

func dropTarget(resolved ResolvedCommand) string {
	if isContainerDropCommand(resolved) && secondDropArg(resolved) != "" {
		return firstDropArg(resolved)
	}
	return joinArgs(resolved.Args)
}

func isContainerDropCommand(resolved ResolvedCommand) bool {
	return resolved.Spec.Name == "넣어" || resolved.Command() == "넣어"
}

func firstDropOrdinal(resolved ResolvedCommand) int64 {
	return dropOrdinal(resolved, 0)
}

func secondDropOrdinal(resolved ResolvedCommand) int64 {
	return dropOrdinal(resolved, 1)
}

func dropOrdinal(resolved ResolvedCommand, index int) int64 {
	if index < 0 || len(resolved.Values) <= index || resolved.Values[index] < 1 {
		return 1
	}
	return resolved.Values[index]
}

func currentDropRoomID(world DropWorld, player model.Player, creature model.Creature) (model.RoomID, error) {
	roomID := player.RoomID
	if roomID.IsZero() {
		roomID = creature.RoomID
	}
	if roomID.IsZero() {
		return "", fmt.Errorf("%w: player %q creature %q", ErrDropRoomRequired, player.ID, creature.ID)
	}
	if _, ok := world.Room(roomID); !ok {
		return "", fmt.Errorf("%w: %q", ErrDropRoomNotFound, roomID)
	}
	return roomID, nil
}

func selectDropObject(world InventoryWorld, ids []model.ObjectInstanceID, target string, ordinal int64) (model.ObjectInstanceID, string, bool) {
	return selectDropObjectWithVisibility(world, ids, target, ordinal, true)
}

func selectDropObjectWithVisibility(world InventoryWorld, ids []model.ObjectInstanceID, target string, ordinal int64, detectInvisible bool) (model.ObjectInstanceID, string, bool) {
	if target == "" {
		return "", "", false
	}
	return selectDropObjectBy(world, ids, ordinal, detectInvisible, func(name string) bool {
		return strings.HasPrefix(name, target)
	})
}

func selectDropObjectBy(world InventoryWorld, ids []model.ObjectInstanceID, ordinal int64, detectInvisible bool, match func(string) bool) (model.ObjectInstanceID, string, bool) {
	var seen int64
	for _, id := range ids {
		if id.IsZero() {
			continue
		}
		object, ok := world.Object(id)
		if !ok {
			continue
		}
		if !detectInvisible && dropObjectIsInvisible(world, object) {
			continue
		}
		if !dropObjectMatches(world, object, match) {
			continue
		}
		seen++
		if seen == ordinal {
			return id, objectDisplayName(world, object), true
		}
	}
	return "", "", false
}

func selectDropContainer(
	world InventoryWorld,
	room model.Room,
	creature model.Creature,
	target string,
	ordinal int64,
	excludeID model.ObjectInstanceID,
	detectInvisible bool,
) (model.ObjectInstanceID, string, bool, string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", "", false, "그런 물건은 없습니다."
	}

	candidates := visibleDropObjects(world, room, creature, detectInvisible)
	if object, name, ok := selectDropVisibleObjectBy(world, candidates, ordinal, func(name string) bool {
		return strings.HasPrefix(name, target)
	}); ok {
		return validateDropContainer(world, object, name, excludeID)
	}
	return "", "", false, "그런 물건은 없습니다."
}

func validateDropContainer(
	world objectNameWorld,
	object model.ObjectInstance,
	name string,
	excludeID model.ObjectInstanceID,
) (model.ObjectInstanceID, string, bool, string) {
	if object.ID == excludeID {
		return "", "", false, "그것을 그것 자신한테는 넣을수 없습니다."
	}
	if !dropObjectIsContainer(world, object) {
		return "", "", false, "그것은 담을수 있는것이 아닙니다."
	}
	return object.ID, name, true, ""
}

func selectDropVisibleObjectBy(
	world objectNameWorld,
	candidates []model.ObjectInstance,
	ordinal int64,
	match func(string) bool,
) (model.ObjectInstance, string, bool) {
	if ordinal < 1 {
		ordinal = 1
	}

	var seen int64
	for _, object := range candidates {
		if !dropObjectMatches(world, object, match) {
			continue
		}
		seen++
		if seen == ordinal {
			return object, objectDisplayName(world, object), true
		}
	}
	return model.ObjectInstance{}, "", false
}

func visibleDropObjects(
	world InventoryWorld,
	room model.Room,
	creature model.Creature,
	detectInvisible bool,
) []model.ObjectInstance {
	candidates := make([]model.ObjectInstance, 0, len(creature.Inventory.ObjectIDs)+len(room.Objects.ObjectIDs))
	seen := map[model.ObjectInstanceID]struct{}{}

	appendVisible := func(ids []model.ObjectInstanceID, visible func(model.ObjectInstance) bool) {
		for _, id := range ids {
			if id.IsZero() {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			object, ok := world.Object(id)
			if !ok || !visible(object) {
				continue
			}
			seen[id] = struct{}{}
			candidates = append(candidates, object)
		}
	}

	appendVisible(creature.Inventory.ObjectIDs, func(object model.ObjectInstance) bool {
		return objectLocatedInCreatureInventory(object, creature.ID) && (detectInvisible || !dropObjectIsInvisible(world, object))
	})
	appendVisible(room.Objects.ObjectIDs, func(object model.ObjectInstance) bool {
		return objectLocatedInRoom(object, room.ID) && (detectInvisible || !dropObjectIsInvisible(world, object))
	})
	appendVisible(getEquipmentObjectIDs(creature), func(object model.ObjectInstance) bool {
		return objectLocatedInCreatureEquipment(object, creature.ID)
	})

	return candidates
}

func objectLocatedInCreatureInventory(object model.ObjectInstance, creatureID model.CreatureID) bool {
	return !creatureID.IsZero() && object.Location.CreatureID == creatureID &&
		(object.Location.Slot == "" || object.Location.Slot == "inventory")
}

func dropObjectIsContainer(world objectNameWorld, object model.ObjectInstance) bool {
	return objectIsContainer(world, object)
}

func dropObjectMatches(world objectNameWorld, object model.ObjectInstance, match func(string) bool) bool {
	for _, term := range legacyObjectEqualTerms(world, object) {
		if match(term) {
			return true
		}
	}
	return false
}
