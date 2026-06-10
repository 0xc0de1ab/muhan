package command

import (
	"fmt"
	"log"
	"strings"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const bankMaxBalance = 300000000

type BankWorld interface {
	StatusWorld
	Bank(model.BankID) (model.BankAccount, bool)
	MoveObject(model.ObjectInstanceID, model.ObjectLocation) error
	DepositCreatureGoldToObjectValue(model.CreatureID, model.ObjectInstanceID, int, int) (int, int, bool, bool, error)
	WithdrawObjectValueToCreatureGold(model.ObjectInstanceID, model.CreatureID, int) (int, int, bool, error)
	StoreCreatureInventoryObjectInContainer(model.ObjectInstanceID, model.CreatureID, model.ObjectInstanceID, int) (int, bool, bool, error)
	TakeContainerObjectToCreatureInventory(model.ObjectInstanceID, model.ObjectInstanceID, model.CreatureID) (int, bool, error)
	PickupMoneyObjectToCreatureGold(model.ObjectInstanceID, model.ObjectLocation, model.CreatureID) (int, int, bool, error)

	// B: Persistence hooks (implemented by *state.World)
	SavePlayer(model.PlayerID) error
	SaveBank(model.BankID) error
}

func NewBankInventoryHandler(world BankWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		if !roomHasAnyFlag(room, "bank", "rbank") {
			ctx.WriteString("은행에서만 가능합니다.")
			return StatusDefault, nil
		}

		target := joinArgs(resolved.Args)
		account, ok := playerBankAccount(world, viewer.PlayerID)
		if !ok {
			if target == "" {
				if _, _, _, err := ensurePlayerBankRoot(world, viewer.PlayerID); err != nil {
					return StatusDefault, err
				}
			} else if all, filter, bulk := bankBulkTarget(target); bulk {
				return storeAllBankObjectsWithEnsuredRoot(ctx, world, viewer.PlayerID, all, filter, room.ID)
			} else {
				return storeBankObjectWithEnsuredRoot(ctx, world, viewer.PlayerID, target, bankInputOrdinal(resolved), room.ID)
			}
			ctx.WriteString("보관하고 있는 물건이 없습니다.")
			return StatusDefault, nil
		}
		root, ok := bankRootObject(world, account)
		if !ok {
			if target == "" {
				if _, _, _, err := ensurePlayerBankRoot(world, viewer.PlayerID); err != nil {
					return StatusDefault, err
				}
			} else if all, filter, bulk := bankBulkTarget(target); bulk {
				return storeAllBankObjectsWithEnsuredRoot(ctx, world, viewer.PlayerID, all, filter, room.ID)
			} else {
				return storeBankObjectWithEnsuredRoot(ctx, world, viewer.PlayerID, target, bankInputOrdinal(resolved), room.ID)
			}
			ctx.WriteString("보관하고 있는 물건이 없습니다.")
			return StatusDefault, nil
		}
		if target != "" {
			if all, filter, ok := bankBulkTarget(target); ok {
				return storeAllBankObjects(ctx, world, viewer.PlayerID, account.ID, root, all, filter, room.ID)
			}
			return storeBankObject(ctx, world, viewer.PlayerID, account.ID, root, target, bankInputOrdinal(resolved), room.ID)
		}
		ctx.WriteString("당신의 이름이 새겨진 보관함입니다.\n")
		player, creature, err := CurrentInventoryCreature(world, viewer.PlayerID)
		if err != nil {
			return StatusDefault, err
		}
		names := bankObjectNames(
			world,
			root,
			inventoryViewerDetectsMagic(player, creature),
			inventoryViewerDetectsInvisible(player, creature),
		)
		if len(names) == 0 {
			ctx.WriteString("보관하고 있는 물건이 없습니다.")
			return StatusDefault, nil
		}
		ctx.WriteString("보관품의 목록 : ")
		ctx.WriteString(strings.Join(names, ", "))
		ctx.WriteString(".\n")
		return StatusDefault, nil
	}
}

func NewBankBalanceHandler(world BankWorld) Handler {
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		if !roomHasAnyFlag(room, "bank", "rbank") {
			ctx.WriteString("은행에서만 가능합니다.")
			return StatusDefault, nil
		}

		balance := 0
		if account, ok := playerBankAccount(world, viewer.PlayerID); ok {
			if root, ok := bankRootObject(world, account); ok {
				balance, _ = objectIntProperty(world, root, "value")
			}
		}
		ctx.WriteString(fmt.Sprintf("당신의 잔고는 %d냥입니다.", balance))
		return StatusDefault, nil
	}
}

func NewBankDepositHandler(world BankWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		if !roomHasAnyFlag(room, "bank", "rbank") {
			ctx.WriteString("은행에서만 가능합니다.")
			return StatusDefault, nil
		}
		if len(resolved.Args) != 1 {
			ctx.WriteString("얼마를 입금하시려고요?")
			return StatusDefault, nil
		}
		_, creature, err := CurrentInventoryCreature(world, viewer.PlayerID)
		if err != nil {
			return StatusDefault, err
		}

		amount, all, ok := parseBankMoneyAmount(resolved.Args[0])
		if !ok {
			ctx.WriteString("사용법 : 몇냥 입금")
			return StatusDefault, nil
		}
		if all {
			amount = creature.Stats["gold"]
		} else if amount < 1 {
			ctx.WriteString("돈의 단위는 음수가 될수 없습니다.")
			return StatusDefault, nil
		}
		if amount > creature.Stats["gold"] {
			ctx.WriteString("당신은 그만큼의 돈을 가지고 있지 않습니다.")
			return StatusDefault, nil
		}

		account, root, ok, err := ensurePlayerBankRoot(world, viewer.PlayerID)
		if err != nil {
			return StatusDefault, err
		}
		if !ok {
			ctx.WriteString("보관하고 있는 물건이 없습니다.")
			return StatusDefault, nil
		}

		_, balance, enough, withinLimit, err := world.DepositCreatureGoldToObjectValue(creature.ID, root.ID, amount, bankMaxBalance)
		if err != nil {
			return StatusDefault, err
		}
		if !enough {
			ctx.WriteString("당신은 그만큼의 돈을 가지고 있지 않습니다.")
			return StatusDefault, nil
		}
		if !withinLimit {
			ctx.WriteString("\n은행에는 3억이상 입금할 수 없습니다. 죄송합니다\n")
			return StatusDefault, nil
		}
		ctx.WriteString(fmt.Sprintf("당신은 %d냥을 입금했습니다.\n", amount))
		ctx.WriteString(fmt.Sprintf("은행의 잔고가 %d냥이 되었습니다.", balance))

		// B: Mark dirty at mutation + queue for background save (A + C)
		bankID := account.ID
		if w, ok := world.(interface {
			MarkPlayerDirty(model.PlayerID)
			MarkBankDirty(model.BankID)
			QueueSave(model.PlayerID, model.BankID)
		}); ok {
			w.MarkPlayerDirty(viewer.PlayerID)
			w.MarkBankDirty(bankID)
			w.QueueSave(viewer.PlayerID, bankID)
		} else {
			// Fallback
			_ = world.SaveBank(bankID)
			_ = world.SavePlayer(viewer.PlayerID)
		}

		return StatusDefault, nil
	}
}

func NewBankWithdrawHandler(world BankWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		if !roomHasAnyFlag(room, "bank", "rbank") {
			ctx.WriteString("은행에서만 가능합니다.")
			return StatusDefault, nil
		}
		if len(resolved.Args) != 1 {
			ctx.WriteString("얼마를 출금하시려고요?")
			return StatusDefault, nil
		}
		_, creature, err := CurrentInventoryCreature(world, viewer.PlayerID)
		if err != nil {
			return StatusDefault, err
		}

		amount, all, ok := parseBankMoneyAmount(resolved.Args[0])
		if !ok {
			ctx.WriteString("사용법 : 몇냥 출금")
			return StatusDefault, nil
		}
		var account model.BankAccount
		var root model.ObjectInstance
		if all {
			var rootOK bool
			account, root, rootOK, err = ensurePlayerBankRoot(world, viewer.PlayerID)
			if err != nil {
				return StatusDefault, err
			}
			if !rootOK {
				ctx.WriteString("당신은 그만큼의 돈을 저금해두지 않았습니다.")
				return StatusDefault, nil
			}
			amount, _ = objectIntProperty(world, root, "value")
		} else if amount < 1 {
			ctx.WriteString("돈의 단위는 음수가 될수 없습니다.")
			return StatusDefault, nil
		} else {
			var accountOK bool
			account, accountOK = playerBankAccount(world, viewer.PlayerID)
			if !accountOK {
				ctx.WriteString("당신은 그만큼의 돈을 저금해두지 않았습니다.")
				return StatusDefault, nil
			}
			var rootOK bool
			root, rootOK = bankRootObject(world, account)
			if !rootOK {
				ctx.WriteString("당신은 그만큼의 돈을 저금해두지 않았습니다.")
				return StatusDefault, nil
			}
		}

		_, balance, enough, err := world.WithdrawObjectValueToCreatureGold(root.ID, creature.ID, amount)
		if err != nil {
			return StatusDefault, err
		}
		if !enough {
			ctx.WriteString("당신은 그만큼의 돈을 저금해두지 않았습니다.")
			return StatusDefault, nil
		}
		ctx.WriteString(fmt.Sprintf("당신은 %d냥을 출금했습니다.\n", amount))
		ctx.WriteString(fmt.Sprintf("은행의 잔고가 %d냥이 되었습니다.", balance))

		// B/C: Mark + Queue after withdraw (Withdraw already marks gold, but ensure + async)
		bankID := account.ID
		if w, ok := world.(interface {
			MarkPlayerDirty(model.PlayerID)
			MarkBankDirty(model.BankID)
			QueueSave(model.PlayerID, model.BankID)
		}); ok {
			w.MarkPlayerDirty(viewer.PlayerID)
			w.MarkBankDirty(bankID)
			w.QueueSave(viewer.PlayerID, bankID)
		} else {
			if err := world.SaveBank(bankID); err != nil {
				log.Printf("[PERSIST] ERROR bank withdraw SaveBank %s: %v", bankID, err)
			}
			if err := world.SavePlayer(viewer.PlayerID); err != nil {
				log.Printf("[PERSIST] ERROR bank withdraw SavePlayer %s: %v", viewer.PlayerID, err)
			}
		}

		return StatusDefault, nil
	}
}

func NewBankOutputHandler(world BankWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		if !roomHasAnyFlag(room, "bank", "rbank") {
			ctx.WriteString("은행에서만 가능합니다.")
			return StatusDefault, nil
		}
		target := joinArgs(resolved.Args)
		if target == "" {
			ctx.WriteString("은행으로부터 무엇을 받으시려고요?")
			return StatusDefault, nil
		}
		account, ok := playerBankAccount(world, viewer.PlayerID)
		if !ok {
			ctx.WriteString("그 안에 그런것은 없어요.")
			return StatusDefault, nil
		}
		root, ok := bankRootObject(world, account)
		if !ok {
			ctx.WriteString("그 안에 그런것은 없어요.")
			return StatusDefault, nil
		}
		player, creature, err := CurrentInventoryCreature(world, viewer.PlayerID)
		if err != nil {
			return StatusDefault, err
		}
		if all, filter, ok := bankBulkTarget(target); ok {
			return takeAllBankObjects(ctx, world, account.ID, root, creature, all, filter, inventoryViewerDetectsInvisible(player, creature), room.ID, commandActorDisplayName(player, creature))
		}

		object, name, ok := findGetContainerObject(world, root, target, bankOutputOrdinal(resolved), inventoryViewerDetectsInvisible(player, creature))
		if !ok {
			ctx.WriteString("그 안에 그런것은 없어요.")
			return StatusDefault, nil
		}
		if bankTakeCapacityExceeded(world, creature, object) {
			ctx.WriteString("당신은 더이상 가질 수 없습니다.")
			return StatusDefault, nil
		}
		object, err = prepareGetPickupObject(world, object, false, false)
		if err != nil {
			return StatusDefault, fmt.Errorf("prepare bank output object %q: %w", object.ID, err)
		}
		if _, taken, err := world.TakeContainerObjectToCreatureInventory(object.ID, root.ID, creature.ID); err != nil {
			return StatusDefault, fmt.Errorf("output bank object %q: %w", object.ID, err)
		} else if !taken {
			ctx.WriteString("그 안에 그런것은 없어요.")
			return StatusDefault, nil
		}
		queueBankObjectSave(world, viewer.PlayerID, account.ID, root)
		ctx.WriteString("당신은 " + name + krtext.Particle(name, '3') + " 받았습니다.")
		_ = roomBroadcast(ctx, room.ID, renderBankOutputRoomConfirmation(commandActorDisplayName(player, creature), name))
		return StatusDefault, nil
	}
}

func storeBankObject(ctx *Context, world BankWorld, playerID model.PlayerID, bankID model.BankID, root model.ObjectInstance, target string, ordinal int64, roomID model.RoomID) (Status, error) {
	player, creature, err := CurrentInventoryCreature(world, playerID)
	if err != nil {
		return StatusDefault, err
	}
	objectID, name, ok := selectDropObjectWithVisibility(world, creature.Inventory.ObjectIDs, target, ordinal, inventoryViewerDetectsInvisible(player, creature))
	if !ok {
		ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
		return StatusDefault, nil
	}
	object, ok := world.Object(objectID)
	if !ok {
		ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
		return StatusDefault, nil
	}
	if objectIsContainer(world, object) {
		ctx.WriteString("보따리 종류는 보관할 수 없습니다.\n")
		return StatusDefault, nil
	}

	if _, stored, full, err := world.StoreCreatureInventoryObjectInContainer(objectID, creature.ID, root.ID, bankRootMax(world, root)); err != nil {
		return StatusDefault, fmt.Errorf("store bank object %q: %w", objectID, err)
	} else if full {
		ctx.WriteString("보관함 안에 더이상 넣을 수 없습니다.\n")
		return StatusDefault, nil
	} else if !stored {
		ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
		return StatusDefault, nil
	}

	queueBankObjectSave(world, playerID, bankID, root)
	ctx.WriteString("당신은 " + name + krtext.Particle(name, '3') + " 보관시켰습니다.\n")
	_ = roomBroadcast(ctx, roomID, renderBankStoreRoomConfirmation(commandActorDisplayName(player, creature), name))
	return StatusDefault, nil
}

func storeBankObjectWithEnsuredRoot(ctx *Context, world BankWorld, playerID model.PlayerID, target string, ordinal int64, roomID model.RoomID) (Status, error) {
	player, creature, err := CurrentInventoryCreature(world, playerID)
	if err != nil {
		return StatusDefault, err
	}
	objectID, name, ok := selectDropObjectWithVisibility(world, creature.Inventory.ObjectIDs, target, ordinal, inventoryViewerDetectsInvisible(player, creature))
	if !ok {
		ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
		return StatusDefault, nil
	}
	object, ok := world.Object(objectID)
	if !ok {
		ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
		return StatusDefault, nil
	}
	if objectIsContainer(world, object) {
		ctx.WriteString("보따리 종류는 보관할 수 없습니다.\n")
		return StatusDefault, nil
	}
	account, root, ok, err := ensurePlayerBankRoot(world, playerID)
	if err != nil {
		return StatusDefault, err
	}
	if !ok {
		ctx.WriteString("보관하고 있는 물건이 없습니다.")
		return StatusDefault, nil
	}
	if _, stored, full, err := world.StoreCreatureInventoryObjectInContainer(objectID, creature.ID, root.ID, bankRootMax(world, root)); err != nil {
		return StatusDefault, fmt.Errorf("store bank object %q: %w", objectID, err)
	} else if full {
		ctx.WriteString("보관함 안에 더이상 넣을 수 없습니다.\n")
		return StatusDefault, nil
	} else if !stored {
		ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
		return StatusDefault, nil
	}
	queueBankObjectSave(world, playerID, account.ID, root)
	ctx.WriteString("당신은 " + name + krtext.Particle(name, '3') + " 보관시켰습니다.\n")
	_ = roomBroadcast(ctx, roomID, renderBankStoreRoomConfirmation(commandActorDisplayName(player, creature), name))
	return StatusDefault, nil
}

func storeAllBankObjects(ctx *Context, world BankWorld, playerID model.PlayerID, bankID model.BankID, root model.ObjectInstance, all bool, filter string, roomID model.RoomID) (Status, error) {
	player, creature, err := CurrentInventoryCreature(world, playerID)
	if err != nil {
		return StatusDefault, err
	}
	detectInvisible := inventoryViewerDetectsInvisible(player, creature)
	groups := make([]objectLookGroup, 0, len(creature.Inventory.ObjectIDs))
	skipped := false
	rootDevours := dropContainerDevours(world, root)
	for _, objectID := range creature.Inventory.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInCreatureInventory(object, creature.ID) || !bankStoreObjectVisibleForDropAll(world, object, detectInvisible) {
			continue
		}
		if !all && !bankObjectMatches(world, object, filter) {
			continue
		}
		if objectIntPropertyOrZero(world, object, "questNumber") != 0 && creatureClass(creature) < model.ClassDM {
			continue
		}
		if objectHasAnyTag(world, object, "event", "oevent") ||
			objectHasAnyPropertyFlag(world, object, "event", "oevent", "OEVENT") {
			continue
		}
		if objectIsContainer(world, object) {
			skipped = true
			continue
		}
		if rootDevours {
			if _, err := destroyBankInventoryObject(world, objectID, creature.ID); err != nil {
				return StatusDefault, fmt.Errorf("store bank object %q: %w", objectID, err)
			}
			continue
		}
		if _, stored, full, err := world.StoreCreatureInventoryObjectInContainer(objectID, creature.ID, root.ID, bankRootMax(world, root)); err != nil {
			return StatusDefault, fmt.Errorf("store bank object %q: %w", objectID, err)
		} else if full {
			skipped = true
			continue
		} else if !stored {
			continue
		}
		groups = appendBankObjectMoveGroup(groups, world, object)
	}
	if len(groups) == 0 {
		ctx.WriteString("당신은 보관시킬 물건을 아무것도 갖고 있지 않습니다.")
		return StatusDefault, nil
	}
	if skipped {
		ctx.WriteString("더이상 물건을 보관시킬 수 없습니다.")
	}
	queueBankObjectSave(world, playerID, bankID, root)
	if len(groups) > 0 {
		rendered := strings.Join(bankObjectMoveGroupNames(groups), ", ")
		ctx.WriteString("당신은 " + rendered + krtext.Particle(rendered, '3') + " 보관시켰습니다.\n")
		_ = roomBroadcast(ctx, roomID, renderBankStoreRoomConfirmation(commandActorDisplayName(player, creature), rendered))
	}
	return StatusDefault, nil
}

func storeAllBankObjectsWithEnsuredRoot(ctx *Context, world BankWorld, playerID model.PlayerID, all bool, filter string, roomID model.RoomID) (Status, error) {
	player, creature, err := CurrentInventoryCreature(world, playerID)
	if err != nil {
		return StatusDefault, err
	}
	detectInvisible := inventoryViewerDetectsInvisible(player, creature)
	hasStorable := false
	for _, objectID := range creature.Inventory.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInCreatureInventory(object, creature.ID) || !bankStoreObjectVisibleForDropAll(world, object, detectInvisible) {
			continue
		}
		if !all && !bankObjectMatches(world, object, filter) {
			continue
		}
		if objectIntPropertyOrZero(world, object, "questNumber") != 0 && creatureClass(creature) < model.ClassDM {
			continue
		}
		if objectHasAnyTag(world, object, "event", "oevent") ||
			objectHasAnyPropertyFlag(world, object, "event", "oevent", "OEVENT") {
			continue
		}
		if objectIsContainer(world, object) {
			continue
		}
		hasStorable = true
		break
	}
	if !hasStorable {
		ctx.WriteString("당신은 보관시킬 물건을 아무것도 갖고 있지 않습니다.")
		return StatusDefault, nil
	}
	account, root, ok, err := ensurePlayerBankRoot(world, playerID)
	if err != nil {
		return StatusDefault, err
	}
	if !ok {
		ctx.WriteString("보관하고 있는 물건이 없습니다.")
		return StatusDefault, nil
	}
	return storeAllBankObjects(ctx, world, playerID, account.ID, root, all, filter, roomID)
}

func destroyBankInventoryObject(world BankWorld, objectID model.ObjectInstanceID, creatureID model.CreatureID) (bool, error) {
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

func takeAllBankObjects(ctx *Context, world BankWorld, bankID model.BankID, root model.ObjectInstance, creature model.Creature, all bool, filter string, detectInvisible bool, roomID model.RoomID, actorName string) (Status, error) {
	groups := make([]objectLookGroup, 0, len(root.Contents.ObjectIDs))
	objectIDs := append([]model.ObjectInstanceID(nil), root.Contents.ObjectIDs...)
	heldCount := bankCreatureHeldCount(world, creature)
	carriedWeight := bankCreatureCarriedWeight(world, creature)
	maxWeight := bankCreatureMaxWeight(creature)
	found := 0
	heavy := false
	creditedMoney := false
	for _, objectID := range objectIDs {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInContainer(object, root.ID) || !bankTakeObjectVisible(world, object, detectInvisible) {
			continue
		}
		if !all && !bankObjectMatches(world, object, filter) {
			continue
		}
		found++
		objectWeight := bankObjectTotalWeight(world, object)
		if heldCount > 150 || carriedWeight+objectWeight > maxWeight {
			heavy = true
			continue
		}
		object, err := prepareGetPickupObject(world, object, false, true)
		if err != nil {
			return StatusDefault, fmt.Errorf("prepare bank output object %q: %w", objectID, err)
		}
		if bankObjectIsMoney(world, object) {
			newGold, _, picked, err := world.PickupMoneyObjectToCreatureGold(objectID, object.Location, creature.ID)
			if err != nil {
				return StatusDefault, fmt.Errorf("output bank money %q: %w", objectID, err)
			}
			if !picked {
				continue
			}
			heldCount++
			creditedMoney = true
			ctx.WriteString(renderBankOutputMoneyBalance(newGold))
			continue
		}
		if _, taken, err := world.TakeContainerObjectToCreatureInventory(objectID, root.ID, creature.ID); err != nil {
			return StatusDefault, fmt.Errorf("output bank object %q: %w", objectID, err)
		} else if !taken {
			continue
		}
		heldCount++
		carriedWeight += objectWeight
		groups = appendBankObjectMoveGroup(groups, world, object)
	}
	if len(groups) == 0 {
		if heavy && found > 0 {
			ctx.WriteString("가지고 있는 물건이 너무 무거워 들 수가 없습니다.\n")
			return StatusDefault, nil
		}
		if creditedMoney {
			queueBankObjectSave(world, creature.PlayerID, bankID, root)
			return StatusDefault, nil
		}
		ctx.WriteString("보관품이 아무것도 없습니다.")
		return StatusDefault, nil
	}
	if heavy {
		ctx.WriteString("가지고 있는 물건이 너무 무거워 들 수가 없습니다.\n")
	}
	queueBankObjectSave(world, creature.PlayerID, bankID, root)
	rendered := strings.Join(bankObjectMoveGroupNames(groups), ", ")
	ctx.WriteString("당신은 " + rendered + krtext.Particle(rendered, '3') + " 받았습니다.")
	_ = roomBroadcast(ctx, roomID, renderBankOutputRoomConfirmation(actorName, rendered))
	return StatusDefault, nil
}

func bankObjectIsMoney(world BankWorld, object model.ObjectInstance) bool {
	return objectKindIs(world, object, model.ObjectKindMoney) ||
		strings.EqualFold(strings.TrimSpace(object.Properties["kind"]), string(model.ObjectKindMoney)) ||
		objectIntPropertyOrZero(world, object, "type") == 10
}

func renderBankOutputMoneyBalance(gold int) string {
	return fmt.Sprintf("\n당신은 이제 %d냥을 가지고 있습니다.", gold)
}

func renderBankStoreRoomConfirmation(actorName, objectName string) string {
	actorName = strings.TrimSpace(actorName)
	if actorName == "" {
		actorName = "누군가"
	}
	objectName = strings.TrimSpace(objectName)
	if objectName == "" {
		objectName = "물건"
	}
	return "\n" + actorName + krtext.Particle(actorName, '1') + " " + objectName + krtext.Particle(objectName, '3') + " 보관시켰습니다."
}

func renderBankOutputRoomConfirmation(actorName, objectName string) string {
	actorName = strings.TrimSpace(actorName)
	if actorName == "" {
		actorName = "누군가"
	}
	objectName = strings.TrimSpace(objectName)
	if objectName == "" {
		objectName = "물건"
	}
	return "\n" + actorName + "이 " + objectName + krtext.Particle(objectName, '3') + " 받았습니다."
}

func bankTakeCapacityExceeded(world BankWorld, creature model.Creature, object model.ObjectInstance) bool {
	return bankCreatureHeldCount(world, creature) > 150 ||
		bankCreatureCarriedWeight(world, creature)+bankObjectTotalWeight(world, object) > bankCreatureMaxWeight(creature)
}

func queueBankObjectSave(world BankWorld, playerID model.PlayerID, bankID model.BankID, root model.ObjectInstance) {
	if bankID.IsZero() {
		bankID = root.Location.BankID
	}
	if bankID.IsZero() && !playerID.IsZero() {
		bankID = model.BankID("bank:player:" + string(playerID))
	}
	if saver, ok := world.(interface {
		MarkPlayerDirty(model.PlayerID)
		MarkBankDirty(model.BankID)
		QueueSave(model.PlayerID, model.BankID)
	}); ok {
		if !playerID.IsZero() {
			saver.MarkPlayerDirty(playerID)
		}
		if !bankID.IsZero() {
			saver.MarkBankDirty(bankID)
		}
		saver.QueueSave(playerID, bankID)
		return
	}
	if !bankID.IsZero() {
		if err := world.SaveBank(bankID); err != nil {
			log.Printf("[PERSIST] ERROR bank object SaveBank %s: %v", bankID, err)
		}
	}
	if !playerID.IsZero() {
		if err := world.SavePlayer(playerID); err != nil {
			log.Printf("[PERSIST] ERROR bank object SavePlayer %s: %v", playerID, err)
		}
	}
}

func bankCreatureHeldCount(world BankWorld, creature model.Creature) int {
	count := 0
	inventoryCount := 0
	for _, objectID := range creature.Inventory.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		inventoryCount++
		if object, ok := world.Object(objectID); ok && objectIsContainer(world, object) {
			inventoryCount += bankObjectContainerCount(world, object)
		}
	}
	if inventoryCount > 200 {
		inventoryCount = 200
	}
	count += inventoryCount
	for _, objectID := range creature.Equipment {
		if !objectID.IsZero() {
			count++
		}
	}
	return count
}

func bankObjectContainerCount(world BankWorld, object model.ObjectInstance) int {
	for _, key := range []string{"shotsCurrent", "shotscur", "shotsCur", "contentsCount"} {
		if count, ok := objectIntProperty(world, object, key); ok {
			return count
		}
	}
	return len(object.Contents.ObjectIDs)
}

func bankCreatureCarriedWeight(world BankWorld, creature model.Creature) int {
	seen := map[model.ObjectInstanceID]struct{}{}
	weight := 0
	for _, objectID := range creature.Inventory.ObjectIDs {
		weight += bankCarriedObjectWeight(world, objectID, true, seen)
	}
	for _, objectID := range creature.Equipment {
		weight += bankCarriedObjectWeight(world, objectID, false, seen)
	}
	return weight
}

func bankCarriedObjectWeight(world BankWorld, objectID model.ObjectInstanceID, skipWeightless bool, seen map[model.ObjectInstanceID]struct{}) int {
	if objectID.IsZero() {
		return 0
	}
	if _, ok := seen[objectID]; ok {
		return 0
	}
	seen[objectID] = struct{}{}
	object, ok := world.Object(objectID)
	if !ok {
		return 0
	}
	if skipWeightless && bankObjectWeightless(world, object) {
		return 0
	}
	weight := bankObjectOwnWeight(world, object)
	for _, childID := range object.Contents.ObjectIDs {
		weight += bankCarriedObjectWeight(world, childID, true, seen)
	}
	return weight
}

func bankObjectTotalWeight(world BankWorld, object model.ObjectInstance) int {
	seen := map[model.ObjectInstanceID]struct{}{}
	weight := bankObjectOwnWeight(world, object)
	for _, childID := range object.Contents.ObjectIDs {
		weight += bankCarriedObjectWeight(world, childID, true, seen)
	}
	return weight
}

func bankObjectOwnWeight(world BankWorld, object model.ObjectInstance) int {
	if weight, ok := objectIntProperty(world, object, "weight"); ok {
		return weight
	}
	return 0
}

func bankObjectWeightless(world BankWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object, "weightless", "owtles") ||
		objectHasAnyPropertyFlag(world, object, "weightless", "owtles")
}

func bankCreatureMaxWeight(creature model.Creature) int {
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

func bankInputOrdinal(resolved ResolvedCommand) int64 {
	if len(resolved.Values) == 0 || resolved.Values[0] < 1 {
		return 1
	}
	return resolved.Values[0]
}

func bankOutputOrdinal(resolved ResolvedCommand) int64 {
	if len(resolved.Values) == 0 || resolved.Values[0] < 1 {
		return 1
	}
	return resolved.Values[0]
}

func parseBankMoneyAmount(text string) (int, bool, bool) {
	text = strings.TrimSpace(text)
	if text == "모두" {
		return 0, true, true
	}
	if !strings.HasSuffix(text, "냥") {
		return 0, false, false
	}
	number := strings.TrimSpace(strings.TrimSuffix(text, "냥"))
	if number == "" {
		return 0, false, false
	}
	return parseBankLegacyAtol(number), false, true
}

func parseBankLegacyAtol(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	sign := 1
	switch text[0] {
	case '-':
		sign = -1
		text = text[1:]
	case '+':
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

func bankBulkTarget(target string) (all bool, filter string, ok bool) {
	return allObjectTarget(target)
}

func allObjectTarget(target string) (all bool, filter string, ok bool) {
	target = strings.TrimSpace(target)
	if target == "모두" || target == "전부" {
		return true, "", true
	}
	if strings.HasPrefix(target, "모든") {
		filter = strings.TrimSpace(strings.TrimPrefix(target, "모든"))
		return filter == "", filter, true
	}
	return false, "", false
}

func bankObjectMatches(world BankWorld, object model.ObjectInstance, filter string) bool {
	filter = strings.TrimSpace(filter)
	return filter == "" || legacyObjectPrefixMatches(world, object, filter)
}

func bankStoreObjectVisibleForDropAll(world BankWorld, object model.ObjectInstance, detectInvisible bool) bool {
	return detectInvisible || !bankObjectHasAnyFlag(world, object, "invisible", "oinvis", "OINVIS")
}

func bankTakeObjectVisible(world BankWorld, object model.ObjectInstance, detectInvisible bool) bool {
	if bankObjectHasAnyFlag(world, object, "hidden", "ohiddn", "OHIDDN", "scenery", "scene", "oscene", "OSCENE") {
		return false
	}
	if !detectInvisible && bankObjectHasAnyFlag(world, object, "invisible", "oinvis", "OINVIS") {
		return false
	}
	return !bankObjectHasAnyFlag(world, object, "noTake", "notTake", "onotak", "notak", "ONOTAK")
}

func bankObjectVisibleForListObj(world BankWorld, object model.ObjectInstance, detectInvisible bool) bool {
	if bankObjectHasAnyFlag(world, object, "hidden", "ohiddn", "OHIDDN", "scenery", "scene", "oscene", "OSCENE") {
		return false
	}
	return detectInvisible || !bankObjectHasAnyFlag(world, object, "invisible", "oinvis", "OINVIS")
}

func appendBankObjectMoveGroup(groups []objectLookGroup, world BankWorld, object model.ObjectInstance) []objectLookGroup {
	name := objectDisplayName(world, object)
	if name == "" {
		return groups
	}
	adjustment := objectIntPropertyOrDefault(world, object, "adjustment", "adjust")
	last := len(groups) - 1
	if last >= 0 && groups[last].BaseName == name && groups[last].Adjustment == adjustment {
		groups[last].Count++
		return groups
	}
	return append(groups, objectLookGroup{
		Text:       name,
		BaseName:   name,
		Adjustment: adjustment,
		Count:      1,
	})
}

func bankObjectMoveGroupNames(groups []objectLookGroup) []string {
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		if group.Count > 1 {
			names = append(names, fmt.Sprintf("(x%d) %s", group.Count, group.Text))
			continue
		}
		names = append(names, group.Text)
	}
	return names
}

func bankObjectHasAnyFlag(world BankWorld, object model.ObjectInstance, names ...string) bool {
	return objectHasAnyTag(world, object, names...) || objectHasAnyPropertyFlag(world, object, names...)
}

func bankRootMax(world BankWorld, root model.ObjectInstance) int {
	if maxCount, ok := objectIntProperty(world, root, "shotsMax"); ok && maxCount > 0 {
		return maxCount
	}
	return 200
}

func playerBankAccount(world BankWorld, playerID model.PlayerID) (model.BankAccount, bool) {
	if world == nil || playerID.IsZero() {
		return model.BankAccount{}, false
	}
	player, ok := world.Player(playerID)
	if !ok {
		return model.BankAccount{}, false
	}
	for _, id := range playerBankIDCandidates(player) {
		if account, ok := world.Bank(id); ok {
			return account, true
		}
	}
	return model.BankAccount{}, false
}

func playerBankIDCandidates(player model.Player) []model.BankID {
	names := []string{
		string(player.ID),
		cleanDisplayText(player.DisplayName),
		strings.TrimSpace(player.AccountName),
		strings.TrimSpace(player.Metadata.LegacyID),
	}
	seen := make(map[string]struct{}, len(names))
	ids := make([]model.BankID, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		ids = append(ids, model.BankID("bank:player:"+name))
	}
	return ids
}

func bankRootObject(world BankWorld, account model.BankAccount) (model.ObjectInstance, bool) {
	for _, objectID := range account.Objects.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if ok {
			return object, true
		}
	}
	return model.ObjectInstance{}, false
}

type playerBankRootEnsurer interface {
	EnsurePlayerBankRoot(model.PlayerID) (model.BankAccount, model.ObjectInstance, error)
}

func ensurePlayerBankRoot(world BankWorld, playerID model.PlayerID) (model.BankAccount, model.ObjectInstance, bool, error) {
	if account, ok := playerBankAccount(world, playerID); ok {
		if root, ok := bankRootObject(world, account); ok {
			return account, root, true, nil
		}
	}
	if ensurer, ok := world.(playerBankRootEnsurer); ok {
		account, root, err := ensurer.EnsurePlayerBankRoot(playerID)
		if err != nil {
			return model.BankAccount{}, model.ObjectInstance{}, false, err
		}
		return account, root, true, nil
	}
	return model.BankAccount{}, model.ObjectInstance{}, false, nil
}

func bankObjectNames(world BankWorld, root model.ObjectInstance, detectMagic bool, detectInvisible bool) []string {
	groups := make([]objectLookGroup, 0, len(root.Contents.ObjectIDs))
	for _, objectID := range root.Contents.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInContainer(object, root.ID) || !bankObjectVisibleForListObj(world, object, detectInvisible) {
			continue
		}
		baseName := objectDisplayName(world, object)
		if baseName == "" {
			continue
		}
		name := objectMagicDisplayName(world, object, detectMagic)
		if name == "" {
			continue
		}
		adjustment := objectIntPropertyOrDefault(world, object, "adjustment", "adjust")
		last := len(groups) - 1
		if last >= 0 && groups[last].BaseName == baseName &&
			(!detectMagic || groups[last].Adjustment == adjustment) {
			groups[last].Count++
			groups[last].Text = name
			groups[last].Adjustment = adjustment
			continue
		}
		groups = append(groups, objectLookGroup{
			Text:       name,
			BaseName:   baseName,
			Adjustment: adjustment,
			Count:      1,
		})
	}

	names := make([]string, 0, len(groups))
	for _, group := range groups {
		if group.Count > 1 {
			names = append(names, fmt.Sprintf("(x%d) %s", group.Count, group.Text))
			continue
		}
		names = append(names, group.Text)
	}
	return names
}
