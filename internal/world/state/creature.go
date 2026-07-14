package state

import (
	"fmt"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"maps"
	"strconv"
	"strings"
)

// Creature returns a copy of the creature with id.
func (w *World) Creature(id model.CreatureID) (model.Creature, bool) {
	if w == nil {
		return model.Creature{}, false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	creature, ok := w.creatures[id]
	if !ok {
		return model.Creature{}, false
	}
	return cloneCreature(creature), true
}

// GetCreature returns a copy of the creature with id.
func (w *World) GetCreature(id model.CreatureID) (model.Creature, bool) {
	return w.Creature(id)
}

// StealCreatureInventoryObject moves an object from one creature inventory to
// another only if it is still in the expected source inventory under the same
// lock.
func (w *World) StealCreatureInventoryObject(objectID model.ObjectInstanceID, fromCreatureID model.CreatureID, toCreatureID model.CreatureID) (bool, error) {
	if w == nil {
		return false, fmt.Errorf("steal object %q: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return false, fmt.Errorf("steal object: object id is required")
	}
	if fromCreatureID.IsZero() {
		return false, fmt.Errorf("steal object %q: source creature id is required", objectID)
	}
	if toCreatureID.IsZero() {
		return false, fmt.Errorf("steal object %q: target creature id is required", objectID)
	}
	if fromCreatureID == toCreatureID {
		return false, nil
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	object, ok := w.objects[objectID]
	if !ok {
		return false, fmt.Errorf("steal object %q: object not found", objectID)
	}
	if _, ok := w.creatures[fromCreatureID]; !ok {
		return false, fmt.Errorf("steal object %q: source creature %q not found", objectID, fromCreatureID)
	}
	if _, ok := w.creatures[toCreatureID]; !ok {
		return false, fmt.Errorf("steal object %q: target creature %q not found", objectID, toCreatureID)
	}
	if object.Location.CreatureID != fromCreatureID || (object.Location.Slot != "" && object.Location.Slot != "inventory") {
		return false, nil
	}

	location := model.ObjectLocation{CreatureID: toCreatureID, Slot: "inventory"}
	if err := w.validateObjectDestinationLocked(objectID, location); err != nil {
		return false, err
	}
	nextObject := object
	nextObject.Location = location
	if err := nextObject.Validate(); err != nil {
		return false, fmt.Errorf("steal object %q: %w", objectID, err)
	}
	w.removeObjectFromHolderLocked(objectID, object.Location)
	w.objects[objectID] = nextObject
	w.addObjectToHolderLocked(objectID, location)
	return true, nil
}

// CloneObjectToCreatureInventory creates a new object instance copied from
// sourceID and puts it in creatureID's inventory. If sourceID is an object
// instance, its recursive contents are materialized with fresh instance IDs.
// If sourceID names an object prototype, a fresh instance is created from that
// prototype, matching legacy load_obj-style callers that pass object numbers.
func (w *World) CloneObjectToCreatureInventory(sourceID model.ObjectInstanceID, creatureID model.CreatureID) (model.ObjectInstanceID, error) {
	if w == nil {
		return "", fmt.Errorf("clone object %q: world state is nil", sourceID)
	}
	if sourceID.IsZero() {
		return "", fmt.Errorf("clone object: source object id is required")
	}
	if creatureID.IsZero() {
		return "", fmt.Errorf("clone object %q: creature id is required", sourceID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	if _, ok := w.objects[sourceID]; !ok {
		if _, ok := w.prototypeIDFromCloneSourceLocked(sourceID); !ok {
			return "", fmt.Errorf("clone object %q: source object not found", sourceID)
		}
	}
	if _, ok := w.creatures[creatureID]; !ok {
		return "", fmt.Errorf("clone object %q: target creature %q not found", sourceID, creatureID)
	}

	cloneID, err := w.cloneObjectSourceToLocationLocked(sourceID, model.ObjectLocation{CreatureID: creatureID, Slot: "inventory"}, false)
	if err != nil {
		return "", fmt.Errorf("clone object %q: %w", sourceID, err)
	}
	if creature := w.creatures[creatureID]; !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneID, nil
}

// PurchaseObjectToCreatureInventory clones sourceID into creatureID's inventory
// and debits the creature's gold stat under one lock. affordable is false when
// the creature exists but does not have enough gold; in that case state is
// unchanged and err is nil.
func (w *World) PurchaseObjectToCreatureInventory(sourceID model.ObjectInstanceID, creatureID model.CreatureID, price int) (newID model.ObjectInstanceID, remainingGold int, affordable bool, err error) {
	if w == nil {
		return "", 0, false, fmt.Errorf("purchase object %q: world state is nil", sourceID)
	}
	if sourceID.IsZero() {
		return "", 0, false, fmt.Errorf("purchase object: source object id is required")
	}
	if creatureID.IsZero() {
		return "", 0, false, fmt.Errorf("purchase object %q: creature id is required", sourceID)
	}
	if price < 0 {
		return "", 0, false, fmt.Errorf("purchase object %q: price cannot be negative", sourceID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	if _, ok := w.objects[sourceID]; !ok {
		if _, ok := w.prototypeIDFromCloneSourceLocked(sourceID); !ok {
			return "", 0, false, fmt.Errorf("purchase object %q: source object not found", sourceID)
		}
	}
	creature, ok := w.creatures[creatureID]
	if !ok {
		return "", 0, false, fmt.Errorf("purchase object %q: target creature %q not found", sourceID, creatureID)
	}
	gold := creature.Stats["gold"]
	if gold < price {
		return "", gold, false, nil
	}

	// Shops must not re-roll the ORENCH random enchant on purchase: C buy copies
	// the depot object as-is and C purchase load_obj's a raw template, neither
	// rolling rand_enchant (command7.c:169-217, command10.c:492+). Rolling here
	// would both diverge per-purchase and enable buy/sell/rebuy enchant farming.
	newID, err = w.cloneObjectSourceToLocationLocked(sourceID, model.ObjectLocation{CreatureID: creatureID, Slot: "inventory"}, true)
	if err != nil {
		return "", gold, false, fmt.Errorf("purchase object %q: %w", sourceID, err)
	}

	creature = w.creatures[creatureID]
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	remainingGold = gold - price
	creature.Stats["gold"] = remainingGold
	w.creatures[creatureID] = creature

	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}

	return newID, remainingGold, true, nil
}

// SellObjectFromCreatureInventory removes an object owned by creatureID and
// credits the creature's gold stat under one lock.
func (w *World) SellObjectFromCreatureInventory(objectID model.ObjectInstanceID, creatureID model.CreatureID, price int) (newGold int, sold bool, err error) {
	if w == nil {
		return 0, false, fmt.Errorf("sell object %q: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return 0, false, fmt.Errorf("sell object: object id is required")
	}
	if creatureID.IsZero() {
		return 0, false, fmt.Errorf("sell object %q: creature id is required", objectID)
	}
	if price < 0 {
		return 0, false, fmt.Errorf("sell object %q: price cannot be negative", objectID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	object, ok := w.objects[objectID]
	if !ok {
		return 0, false, fmt.Errorf("sell object %q: object not found", objectID)
	}
	creature, ok := w.creatures[creatureID]
	if !ok {
		return 0, false, fmt.Errorf("sell object %q: creature %q not found", objectID, creatureID)
	}
	if object.Location.CreatureID != creatureID {
		return creature.Stats["gold"], false, nil
	}
	if len(object.Contents.ObjectIDs) != 0 {
		return creature.Stats["gold"], false, fmt.Errorf("sell object %q: object has contents", objectID)
	}

	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	newGold = creature.Stats["gold"] + price
	creature.Stats["gold"] = newGold
	w.creatures[creatureID] = creature

	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}

	w.removeObjectFromHolderLocked(objectID, object.Location)
	delete(w.objects, objectID)
	return newGold, true, nil
}

// RepairCreatureInventoryObject debits repair cost and applies object
// property/tag changes for an inventory object under one lock.
func (w *World) RepairCreatureInventoryObject(objectID model.ObjectInstanceID, creatureID model.CreatureID, cost int, properties map[string]string, removeTags []string) (newGold int, repaired bool, affordable bool, err error) {
	if w == nil {
		return 0, false, false, fmt.Errorf("repair object %q: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return 0, false, false, fmt.Errorf("repair object: object id is required")
	}
	if creatureID.IsZero() {
		return 0, false, false, fmt.Errorf("repair object %q: creature id is required", objectID)
	}
	if cost < 0 {
		return 0, false, false, fmt.Errorf("repair object %q: cost cannot be negative", objectID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	object, ok := w.objects[objectID]
	if !ok {
		return 0, false, false, fmt.Errorf("repair object %q: object not found", objectID)
	}
	creature, ok := w.creatures[creatureID]
	if !ok {
		return 0, false, false, fmt.Errorf("repair object %q: creature %q not found", objectID, creatureID)
	}
	if object.Location.CreatureID != creatureID || (object.Location.Slot != "" && object.Location.Slot != "inventory") {
		return creature.Stats["gold"], false, true, nil
	}
	gold := creature.Stats["gold"]
	if gold < cost {
		return gold, false, false, nil
	}

	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	newGold = gold - cost
	creature.Stats["gold"] = newGold
	w.creatures[creatureID] = creature

	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}

	object.Properties = maps.Clone(object.Properties)
	if object.Properties == nil {
		object.Properties = map[string]string{}
	}
	for key, value := range properties {
		object.Properties[key] = value
	}
	object.Metadata.Tags = removeMetadataTags(object.Metadata.Tags, removeTags)
	w.objects[objectID] = object
	return newGold, true, true, nil
}

// DestroyCreatureInventoryObject deletes an inventory object and removes holder refs.
func (w *World) DestroyCreatureInventoryObject(objectID model.ObjectInstanceID, creatureID model.CreatureID) (destroyed bool, err error) {
	if w == nil {
		return false, fmt.Errorf("destroy object %q: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return false, fmt.Errorf("destroy object: object id is required")
	}
	if creatureID.IsZero() {
		return false, fmt.Errorf("destroy object %q: creature id is required", objectID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	object, ok := w.objects[objectID]
	if !ok {
		return false, fmt.Errorf("destroy object %q: object not found", objectID)
	}
	creature, ok := w.creatures[creatureID]
	if !ok {
		return false, fmt.Errorf("destroy object %q: creature %q not found", objectID, creatureID)
	}
	if object.Location.CreatureID != creatureID || (object.Location.Slot != "" && object.Location.Slot != "inventory") {
		return false, nil
	}
	w.removeObjectFromHolderLocked(objectID, object.Location)
	delete(w.objects, objectID)
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return true, nil
}

// ConsumeCreatureObjectCharge decrements an object charge for an object carried
// or equipped by creatureID. When deleteAtZero is true, the object is removed
// after the consumed charge reaches zero.
func (w *World) ConsumeCreatureObjectCharge(objectID model.ObjectInstanceID, creatureID model.CreatureID, deleteAtZero bool) (updated model.ObjectInstance, deleted bool, consumed bool, err error) {
	if w == nil {
		return model.ObjectInstance{}, false, false, fmt.Errorf("consume object %q: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return model.ObjectInstance{}, false, false, fmt.Errorf("consume object: object id is required")
	}
	if creatureID.IsZero() {
		return model.ObjectInstance{}, false, false, fmt.Errorf("consume object %q: creature id is required", objectID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	object, ok := w.objects[objectID]
	if !ok {
		return model.ObjectInstance{}, false, false, fmt.Errorf("consume object %q: object not found", objectID)
	}
	if _, ok := w.creatures[creatureID]; !ok {
		return model.ObjectInstance{}, false, false, fmt.Errorf("consume object %q: creature %q not found", objectID, creatureID)
	}
	if object.Location.CreatureID != creatureID {
		return cloneObject(object), false, false, nil
	}
	charges, ok := w.objectIntPropertyLocked(object, "shotsCurrent")
	if !ok || charges < 1 {
		return cloneObject(object), false, false, nil
	}

	charges--
	if object.Properties == nil {
		object.Properties = map[string]string{}
	}
	object.Properties["shotsCurrent"] = strconv.Itoa(charges)
	if deleteAtZero && charges < 1 {
		w.removeObjectFromHolderLocked(objectID, object.Location)
		delete(w.objects, objectID)
		return model.ObjectInstance{}, true, true, nil
	}
	w.objects[objectID] = object
	return cloneObject(object), false, true, nil
}

// TransferCreatureGold moves gold from one creature stat bucket to another under
// one lock. ok is false when the source creature exists but does not have enough
// gold; in that case state is unchanged and err is nil.
func (w *World) TransferCreatureGold(fromID model.CreatureID, toID model.CreatureID, amount int) (fromGold int, toGold int, ok bool, err error) {
	if w == nil {
		return 0, 0, false, fmt.Errorf("transfer gold from %q to %q: world state is nil", fromID, toID)
	}
	if fromID.IsZero() {
		return 0, 0, false, fmt.Errorf("transfer gold to %q: source creature id is required", toID)
	}
	if toID.IsZero() {
		return 0, 0, false, fmt.Errorf("transfer gold from %q: target creature id is required", fromID)
	}
	if amount < 1 {
		return 0, 0, false, fmt.Errorf("transfer gold from %q to %q: amount must be positive", fromID, toID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	from, ok := w.creatures[fromID]
	if !ok {
		return 0, 0, false, fmt.Errorf("transfer gold from %q: source creature not found", fromID)
	}
	to, ok := w.creatures[toID]
	if !ok {
		return 0, 0, false, fmt.Errorf("transfer gold to %q: target creature not found", toID)
	}
	fromGold = from.Stats["gold"]
	toGold = to.Stats["gold"]
	if fromGold < amount {
		return fromGold, toGold, false, nil
	}

	if from.Stats == nil {
		from.Stats = map[string]int{}
	}
	if to.Stats == nil {
		to.Stats = map[string]int{}
	}
	fromGold -= amount
	toGold += amount
	from.Stats["gold"] = fromGold
	to.Stats["gold"] = toGold
	w.creatures[fromID] = from
	w.creatures[toID] = to

	if !from.PlayerID.IsZero() {
		w.MarkPlayerDirty(from.PlayerID)
	}
	if !to.PlayerID.IsZero() {
		w.MarkPlayerDirty(to.PlayerID)
	}
	return fromGold, toGold, true, nil
}

// PickupMoneyObjectToCreatureGold removes a money object from its current
// holder and credits the creature's gold under one lock.
func (w *World) PickupMoneyObjectToCreatureGold(objectID model.ObjectInstanceID, from model.ObjectLocation, creatureID model.CreatureID) (newGold int, amount int, picked bool, err error) {
	if w == nil {
		return 0, 0, false, fmt.Errorf("pickup money object %q: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return 0, 0, false, fmt.Errorf("pickup money object: object id is required")
	}
	if creatureID.IsZero() {
		return 0, 0, false, fmt.Errorf("pickup money object %q: creature id is required", objectID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	object, ok := w.objects[objectID]
	if !ok {
		return 0, 0, false, fmt.Errorf("pickup money object %q: object not found", objectID)
	}
	creature, ok := w.creatures[creatureID]
	if !ok {
		return 0, 0, false, fmt.Errorf("pickup money object %q: creature %q not found", objectID, creatureID)
	}
	if !objectLocationEqual(object.Location, from) || !w.objectIsMoneyLocked(object) {
		return creature.Stats["gold"], 0, false, nil
	}
	amount, _ = w.objectIntPropertyLocked(object, "value")
	if amount < 1 {
		return creature.Stats["gold"], 0, false, nil
	}

	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	newGold = creature.Stats["gold"] + amount
	creature.Stats["gold"] = newGold
	w.creatures[creatureID] = creature

	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}

	w.removeObjectFromHolderLocked(objectID, object.Location)
	if !object.Location.ContainerID.IsZero() {
		container := w.objects[object.Location.ContainerID]
		current := objectCountPropertyOrLenLocked(w, container, "shotsCurrent")
		if container.Properties == nil {
			container.Properties = map[string]string{}
		}
		if current > 0 {
			current--
		}
		container.Properties["shotsCurrent"] = strconv.Itoa(current)
		w.objects[container.ID] = container
	}
	delete(w.objects, objectID)

	roomToMark := object.Location.RoomID
	if roomToMark.IsZero() && !object.Location.ContainerID.IsZero() {
		if c, ok := w.objects[object.Location.ContainerID]; ok && !c.Location.RoomID.IsZero() {
			roomToMark = c.Location.RoomID
		}
	}
	if !roomToMark.IsZero() {
		w.MarkRoomObjectsDirty(roomToMark)
	}

	if !object.Location.ContainerID.IsZero() {
		if c, ok := w.objects[object.Location.ContainerID]; ok && !c.Location.RoomID.IsZero() {
			w.MarkRoomObjectsDirty(c.Location.RoomID)
		}
	}

	return newGold, amount, true, nil
}

// DepositCreatureGoldToObjectValue debits creature gold and credits an object's
// numeric value property under one lock. ok is false when the creature has
// insufficient gold; withinLimit is false when maxValue would be exceeded.
func (w *World) DepositCreatureGoldToObjectValue(creatureID model.CreatureID, objectID model.ObjectInstanceID, amount int, maxValue int) (remainingGold int, objectValue int, ok bool, withinLimit bool, err error) {
	return w.DepositCreatureGoldToObjectValueScaled(creatureID, objectID, amount, amount, amount, maxValue)
}

// DepositCreatureGoldToObjectValueScaled debits one creature gold amount while
// crediting a potentially different object value amount. limitAmount is the
// delta used for the maxValue check; this preserves legacy callers where the
// stored object unit differs from player gold units.
func (w *World) DepositCreatureGoldToObjectValueScaled(creatureID model.CreatureID, objectID model.ObjectInstanceID, goldAmount int, valueAmount int, limitAmount int, maxValue int) (remainingGold int, objectValue int, ok bool, withinLimit bool, err error) {
	if w == nil {
		return 0, 0, false, false, fmt.Errorf("deposit gold from %q to object %q: world state is nil", creatureID, objectID)
	}
	if creatureID.IsZero() {
		return 0, 0, false, false, fmt.Errorf("deposit gold to object %q: creature id is required", objectID)
	}
	if objectID.IsZero() {
		return 0, 0, false, false, fmt.Errorf("deposit gold from %q: object id is required", creatureID)
	}
	if goldAmount < 0 || valueAmount < 0 || limitAmount < 0 {
		return 0, 0, false, false, fmt.Errorf("deposit gold from %q to object %q: amount cannot be negative", creatureID, objectID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	creature, okCreature := w.creatures[creatureID]
	if !okCreature {
		return 0, 0, false, false, fmt.Errorf("deposit gold from %q: creature not found", creatureID)
	}
	object, okObject := w.objects[objectID]
	if !okObject {
		return 0, 0, false, false, fmt.Errorf("deposit gold to object %q: object not found", objectID)
	}
	gold := creature.Stats["gold"]
	value, _ := w.objectIntPropertyLocked(object, "value")
	if gold < goldAmount {
		return gold, value, false, true, nil
	}
	if maxValue > 0 && value+limitAmount > maxValue {
		return gold, value, true, false, nil
	}

	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	if object.Properties == nil {
		object.Properties = map[string]string{}
	}
	remainingGold = gold - goldAmount
	objectValue = value + valueAmount
	creature.Stats["gold"] = remainingGold
	object.Properties["value"] = strconv.Itoa(objectValue)
	w.creatures[creatureID] = creature
	w.objects[objectID] = object

	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return remainingGold, objectValue, true, true, nil
}

// WithdrawObjectValueToCreatureGold debits an object's numeric value property
// and credits creature gold under one lock. ok is false when object value is
// insufficient; in that case state is unchanged.
func (w *World) WithdrawObjectValueToCreatureGold(objectID model.ObjectInstanceID, creatureID model.CreatureID, amount int) (newGold int, objectValue int, ok bool, err error) {
	return w.WithdrawObjectValueToCreatureGoldScaled(objectID, creatureID, amount, amount)
}

// WithdrawObjectValueToCreatureGoldScaled debits one object value amount while
// crediting a potentially different creature gold amount.
func (w *World) WithdrawObjectValueToCreatureGoldScaled(objectID model.ObjectInstanceID, creatureID model.CreatureID, valueAmount int, goldAmount int) (newGold int, objectValue int, ok bool, err error) {
	if w == nil {
		return 0, 0, false, fmt.Errorf("withdraw gold from object %q to %q: world state is nil", objectID, creatureID)
	}
	if objectID.IsZero() {
		return 0, 0, false, fmt.Errorf("withdraw gold to %q: object id is required", creatureID)
	}
	if creatureID.IsZero() {
		return 0, 0, false, fmt.Errorf("withdraw gold from object %q: creature id is required", objectID)
	}
	if valueAmount < 0 || goldAmount < 0 {
		return 0, 0, false, fmt.Errorf("withdraw gold from object %q to %q: amount cannot be negative", objectID, creatureID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	object, okObject := w.objects[objectID]
	if !okObject {
		return 0, 0, false, fmt.Errorf("withdraw gold from object %q: object not found", objectID)
	}
	creature, okCreature := w.creatures[creatureID]
	if !okCreature {
		return 0, 0, false, fmt.Errorf("withdraw gold to %q: creature not found", creatureID)
	}
	value, _ := w.objectIntPropertyLocked(object, "value")
	gold := creature.Stats["gold"]
	if value < valueAmount {
		return gold, value, false, nil
	}

	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	if object.Properties == nil {
		object.Properties = map[string]string{}
	}
	newGold = gold + goldAmount
	objectValue = value - valueAmount
	creature.Stats["gold"] = newGold
	object.Properties["value"] = strconv.Itoa(objectValue)
	w.creatures[creatureID] = creature
	w.objects[objectID] = object

	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return newGold, objectValue, true, nil
}

// StoreCreatureInventoryObjectInContainer moves an inventory object into a
// container and increments the container's shotsCurrent property under one lock.
func (w *World) StoreCreatureInventoryObjectInContainer(objectID model.ObjectInstanceID, creatureID model.CreatureID, containerID model.ObjectInstanceID, maxCount int) (newCount int, stored bool, full bool, err error) {
	if w == nil {
		return 0, false, false, fmt.Errorf("store object %q in container %q: world state is nil", objectID, containerID)
	}
	if objectID.IsZero() {
		return 0, false, false, fmt.Errorf("store object in container %q: object id is required", containerID)
	}
	if creatureID.IsZero() {
		return 0, false, false, fmt.Errorf("store object %q in container %q: creature id is required", objectID, containerID)
	}
	if containerID.IsZero() {
		return 0, false, false, fmt.Errorf("store object %q: container id is required", objectID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	object, ok := w.objects[objectID]
	if !ok {
		return 0, false, false, fmt.Errorf("store object %q: object not found", objectID)
	}
	if _, ok := w.creatures[creatureID]; !ok {
		return 0, false, false, fmt.Errorf("store object %q: creature %q not found", objectID, creatureID)
	}
	container, ok := w.objects[containerID]
	if !ok {
		return 0, false, false, fmt.Errorf("store object %q: container %q not found", objectID, containerID)
	}
	current := objectCountPropertyOrLenLocked(w, container, "shotsCurrent")
	if maxCount > 0 && current >= maxCount {
		return current, false, true, nil
	}
	if object.Location.CreatureID != creatureID || (object.Location.Slot != "" && object.Location.Slot != "inventory") {
		return current, false, false, nil
	}

	location := model.ObjectLocation{ContainerID: containerID}
	if err := w.validateObjectDestinationLocked(objectID, location); err != nil {
		return 0, false, false, err
	}
	nextObject := object
	nextObject.Location = location
	if err := nextObject.Validate(); err != nil {
		return 0, false, false, fmt.Errorf("store object %q: %w", objectID, err)
	}

	w.removeObjectFromHolderLocked(objectID, object.Location)
	w.objects[objectID] = nextObject
	w.addObjectToHolderLocked(objectID, location)

	container = w.objects[containerID]
	if container.Properties == nil {
		container.Properties = map[string]string{}
	}
	newCount = current + 1
	container.Properties["shotsCurrent"] = strconv.Itoa(newCount)
	w.objects[containerID] = container

	if !container.Location.RoomID.IsZero() {
		w.MarkRoomObjectsDirty(container.Location.RoomID)
	}

	return newCount, true, false, nil
}

// TakeContainerObjectToCreatureInventory moves a direct child from a container
// to creature inventory and decrements the container's shotsCurrent property.
func (w *World) TakeContainerObjectToCreatureInventory(objectID model.ObjectInstanceID, containerID model.ObjectInstanceID, creatureID model.CreatureID) (newCount int, taken bool, err error) {
	if w == nil {
		return 0, false, fmt.Errorf("take object %q from container %q: world state is nil", objectID, containerID)
	}
	if objectID.IsZero() {
		return 0, false, fmt.Errorf("take object from container %q: object id is required", containerID)
	}
	if containerID.IsZero() {
		return 0, false, fmt.Errorf("take object %q: container id is required", objectID)
	}
	if creatureID.IsZero() {
		return 0, false, fmt.Errorf("take object %q from container %q: creature id is required", objectID, containerID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	object, ok := w.objects[objectID]
	if !ok {
		return 0, false, fmt.Errorf("take object %q: object not found", objectID)
	}
	container, ok := w.objects[containerID]
	if !ok {
		return 0, false, fmt.Errorf("take object %q: container %q not found", objectID, containerID)
	}
	if _, ok := w.creatures[creatureID]; !ok {
		return 0, false, fmt.Errorf("take object %q: creature %q not found", objectID, creatureID)
	}
	current := objectCountPropertyOrLenLocked(w, container, "shotsCurrent")
	if object.Location.ContainerID != containerID {
		return current, false, nil
	}

	location := model.ObjectLocation{CreatureID: creatureID, Slot: "inventory"}
	if err := w.validateObjectDestinationLocked(objectID, location); err != nil {
		return 0, false, err
	}
	nextObject := object
	nextObject.Location = location
	if err := nextObject.Validate(); err != nil {
		return 0, false, fmt.Errorf("take object %q: %w", objectID, err)
	}

	w.removeObjectFromHolderLocked(objectID, object.Location)
	w.objects[objectID] = nextObject
	w.addObjectToHolderLocked(objectID, location)

	container = w.objects[containerID]
	if container.Properties == nil {
		container.Properties = map[string]string{}
	}
	if current > 0 {
		newCount = current - 1
	}
	container.Properties["shotsCurrent"] = strconv.Itoa(newCount)
	w.objects[containerID] = container

	if !container.Location.RoomID.IsZero() {
		w.MarkRoomObjectsDirty(container.Location.RoomID)
	}

	return newCount, true, nil
}

// SetCreatureStat sets a numeric creature stat, creating the stat map if needed.
func (w *World) SetCreatureStat(creatureID model.CreatureID, key string, value int) error {
	if w == nil {
		return fmt.Errorf("set creature %q stat %q: world state is nil", creatureID, key)
	}
	if creatureID.IsZero() {
		return fmt.Errorf("set creature stat %q: creature id is required", key)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("set creature %q stat: key is required", creatureID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	creature, ok := w.creatures[creatureID]
	if !ok {
		return fmt.Errorf("set creature %q stat %q: creature not found", creatureID, key)
	}
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats[key] = value
	w.creatures[creatureID] = creature

	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return nil
}

// SetCreatureProperty sets a string creature property, creating the property
// map when needed. An empty value removes the property.
func (w *World) SetCreatureProperty(creatureID model.CreatureID, key string, value string) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("set creature %q property %q: world state is nil", creatureID, key)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("set creature property %q: creature id is required", key)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return model.Creature{}, fmt.Errorf("set creature %q property: key is required", creatureID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("set creature %q property %q: creature not found", creatureID, key)
	}
	if value == "" {
		delete(creature.Properties, key)
		if len(creature.Properties) == 0 {
			creature.Properties = nil
		}
		w.creatures[creatureID] = creature
		if !creature.PlayerID.IsZero() {
			w.MarkPlayerDirty(creature.PlayerID)
		}
		return cloneCreature(creature), nil
	}
	if creature.Properties == nil {
		creature.Properties = map[string]string{}
	}
	creature.Properties[key] = value
	w.creatures[creatureID] = creature
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneCreature(creature), nil
}

// SetCreatureDescription stores the canonical player/creature description on
// the model instead of using the compatibility property override. It removes a
// stale description property so command rendering sees this direct value.
func (w *World) SetCreatureDescription(creatureID model.CreatureID, description string) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("set creature %q description: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("set creature description: creature id is required")
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("set creature %q description: creature not found", creatureID)
	}
	creature.Description = description
	delete(creature.Properties, stateCreatureDescriptionProperty)
	if len(creature.Properties) == 0 {
		creature.Properties = nil
	}
	w.creatures[creatureID] = creature
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneCreature(creature), nil
}

// SetCreaturePasswordHash stores the legacy password hash in the canonical
// property key used by the account command ports. An empty hash removes it.
func (w *World) SetCreaturePasswordHash(creatureID model.CreatureID, hash string) (model.Creature, error) {
	hash = strings.TrimSpace(hash)
	return w.SetCreatureProperty(creatureID, stateCreaturePasswordHashKey, hash)
}

// RecalculateCreatureAC recomputes and stores the runtime armor stat using the
// same inputs as the legacy compute_ac path: attributes, equipped armor, and
// active protection flags.
func (w *World) RecalculateCreatureAC(creatureID model.CreatureID) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("recalculate creature %q ac: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("recalculate creature ac: creature id is required")
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("recalculate creature %q ac: creature not found", creatureID)
	}
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats["armor"] = w.computeCreatureACLocked(creature)
	w.creatures[creatureID] = creature
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneCreature(creature), nil
}

// RecalculateCreatureTHACO recomputes and stores the runtime thaco stat using
// the legacy class/level table plus weapon, proficiency, and active flags.
func (w *World) RecalculateCreatureTHACO(creatureID model.CreatureID) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("recalculate creature %q thaco: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("recalculate creature thaco: creature id is required")
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("recalculate creature %q thaco: creature not found", creatureID)
	}
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats["thaco"] = w.computeCreatureTHACOLocked(creature)
	w.creatures[creatureID] = creature
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneCreature(creature), nil
}

// RecalculateCreatureCombatStats recomputes AC and THACO in one state mutation.
func (w *World) RecalculateCreatureCombatStats(creatureID model.CreatureID) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("recalculate creature %q combat stats: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("recalculate creature combat stats: creature id is required")
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("recalculate creature %q combat stats: creature not found", creatureID)
	}
	w.recalculateCreatureCombatStatsLocked(&creature)
	w.creatures[creatureID] = creature
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneCreature(creature), nil
}

// UpdateCreatureFamilyState replaces the legacy family membership flags for a
// creature. It updates canonical and legacy stat names together and removes
// stale property/tag aliases so command helpers cannot read an old family state.
func (w *World) UpdateCreatureFamilyState(creatureID model.CreatureID, familyID int, member bool, pending bool, boss bool) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("update creature %q family state: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("update creature family state: creature id is required")
	}
	if familyID < 0 {
		return model.Creature{}, fmt.Errorf("update creature %q family state: family id cannot be negative", creatureID)
	}
	if member && pending {
		return model.Creature{}, fmt.Errorf("update creature %q family state: member and pending are exclusive", creatureID)
	}
	if (member || pending) && familyID <= 0 {
		return model.Creature{}, fmt.Errorf("update creature %q family state: active family id is required", creatureID)
	}
	if boss && !member {
		return model.Creature{}, fmt.Errorf("update creature %q family state: boss requires membership", creatureID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("update creature %q family state: creature not found", creatureID)
	}
	updateCreatureFamilyStateLocked(&creature, familyID, member, pending, boss)
	w.creatures[creatureID] = creature
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneCreature(creature), nil
}

// UpdateCreatureGold adds (or subtracts if negative) the amount of gold to/from a creature.
func (w *World) UpdateCreatureGold(creatureID model.CreatureID, amount int) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("update gold: world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("update gold: creature %q not found", creatureID)
	}
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats["gold"] += amount
	w.creatures[creatureID] = creature
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneCreature(creature), nil
}

// UpdateCreature updates a creature in the world state.
func (w *World) UpdateCreature(creature model.Creature) error {
	if w == nil {
		return fmt.Errorf("update creature: world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	w.creatures[creature.ID] = creature
	return nil
}

func updateCreatureFamilyStateLocked(creature *model.Creature, familyID int, member bool, pending bool, boss bool) {
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	memberValue := boolInt(member)
	pendingValue := boolInt(pending)
	bossValue := boolInt(member && boss)
	activeFamilyID := 0
	if member || pending {
		activeFamilyID = familyID
	}

	for _, key := range []string{"familyFlag", "PFAMIL"} {
		creature.Stats[key] = memberValue
	}
	for _, key := range []string{"familyID", "dailyExpndMax", "legacyDailyExpndMax"} {
		creature.Stats[key] = activeFamilyID
	}
	creature.Stats["PRDFML"] = pendingValue
	for _, key := range []string{"PFMBOS", "familyBoss", "familyBossFlag"} {
		creature.Stats[key] = bossValue
	}

	removeCreatureFamilyStateProperties(creature)
	creature.Metadata.Tags = removeMetadataTags(creature.Metadata.Tags, creatureFamilyStateTagNames())
	var addTags []string
	if member {
		addTags = append(addTags, "PFAMIL")
	}
	if pending {
		addTags = append(addTags, "PRDFML")
	}
	if member && boss {
		addTags = append(addTags, "PFMBOS")
	}
	creature.Metadata.Tags = addMetadataTags(creature.Metadata.Tags, addTags)
}

func creatureFamilyStatePropertyNames() []string {
	return []string{
		"familyFlag", "PFAMIL",
		"familyID", "dailyExpndMax", "legacyDailyExpndMax",
		"PRDFML",
		"PFMBOS", "familyBoss", "familyBossFlag",
	}
}

func creatureFamilyStateTagNames() []string {
	return []string{
		"familyFlag", "PFAMIL",
		"PRDFML",
		"PFMBOS", "familyBoss", "familyBossFlag",
	}
}

// UpdateCreatureMarriageState replaces the legacy runtime marriage flags for a
// creature. It updates canonical and legacy stat names together and removes
// stale property/tag aliases so command helpers cannot read an old marriage state.
func (w *World) UpdateCreatureMarriageState(creatureID model.CreatureID, marriageID int, married bool, pending bool) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("update creature %q marriage state: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("update creature marriage state: creature id is required")
	}
	if marriageID < 0 {
		return model.Creature{}, fmt.Errorf("update creature %q marriage state: marriage id cannot be negative", creatureID)
	}
	if married && pending {
		return model.Creature{}, fmt.Errorf("update creature %q marriage state: married and pending are exclusive", creatureID)
	}
	if married && marriageID <= 0 {
		return model.Creature{}, fmt.Errorf("update creature %q marriage state: active marriage id is required", creatureID)
	}
	if !married {
		marriageID = 0
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("update creature %q marriage state: creature not found", creatureID)
	}
	updateCreatureMarriageStateLocked(&creature, marriageID, married, pending)
	w.creatures[creatureID] = creature
	return cloneCreature(creature), nil
}

func updateCreatureMarriageStateLocked(creature *model.Creature, marriageID int, married bool, pending bool) {
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	marriedValue := boolInt(married)
	pendingValue := boolInt(pending)
	for _, key := range []string{"PMARRI", "married", "marriageFlag"} {
		creature.Stats[key] = marriedValue
	}
	for _, key := range []string{"marriageID", "dailyMarriageMax", "legacyDailyMarriageMax"} {
		creature.Stats[key] = marriageID
	}
	for _, key := range []string{"PRDMAR", "marriagePending"} {
		creature.Stats[key] = pendingValue
	}

	removeCreatureMarriageStateProperties(creature)
	creature.Metadata.Tags = removeMetadataTags(creature.Metadata.Tags, creatureMarriageStateTagNames())
	var addTags []string
	if married {
		addTags = append(addTags, "PMARRI", "married")
	}
	if pending {
		addTags = append(addTags, "PRDMAR")
	}
	creature.Metadata.Tags = addMetadataTags(creature.Metadata.Tags, addTags)
}

func creatureMarriageStatePropertyNames() []string {
	return []string{
		"PMARRI", "married", "marriage", "marriageFlag",
		"marriageID", "dailyMarriageMax", "legacyDailyMarriageMax",
		"PRDMAR", "marriagePending",
	}
}

func creatureMarriageStateTagNames() []string {
	return []string{
		"PMARRI", "married", "marriage", "marriageFlag",
		"PRDMAR", "marriagePending",
	}
}

func (w *World) recalculateCreatureCombatStatsLocked(creature *model.Creature) {
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats["armor"] = w.computeCreatureACLocked(*creature)
	creature.Stats["thaco"] = w.computeCreatureTHACOLocked(*creature)
}

func (w *World) computeCreatureACLocked(creature model.Creature) int {
	ac := 100

	constitution := creatureStateInt(creature, "constitution")
	if constitution > 95 {
		ac -= 5 * stateLegacyStatBonus(90)
	} else {
		ac -= 5 * (stateLegacyStatBonus(constitution) + 4)
	}

	dexterity := creatureStateInt(creature, "dexterity")
	if dexterity > 95 {
		ac -= 2 * stateLegacyStatBonus(90)
	} else {
		ac -= 2 * (stateLegacyStatBonus(dexterity) + 4)
	}

	for _, objectID := range creature.Equipment {
		if objectID.IsZero() {
			continue
		}
		object, ok := w.objects[objectID]
		if !ok {
			continue
		}
		if armor, ok := w.objectIntPropertyLocked(object, "armor"); ok {
			ac -= armor
		}
	}

	if creatureHasAnyFlag(creature, "PPROTE", "protect") {
		ac -= 10
	}
	if creatureHasAnyFlag(creature, "PREFLECT", "reflect", "reflection") {
		ac -= 15
	}
	if creatureHasAnyFlag(creature, "PSHADOW", "shadow", "shadowClone") {
		ac -= 20
	}
	if creatureHasAnyFlag(creature, "PABSORB", "absorb") {
		ac -= 10
	}
	if creatureHasAnyFlag(creature, "PCHOI", "choi") {
		ac += 20
	}

	if creatureStateClass(creature) >= model.ClassBulsa {
		ac -= 10
		if constitution > 45 {
			ac -= constitution - 45
		}
	}
	return clampInt(ac, -127, 127)
}

func (w *World) computeCreatureTHACOLocked(creature model.Creature) int {
	level := creatureStateLevel(creature)
	index := (level + 3) / 4
	if index > 20 {
		index = 19
	} else if index > 0 {
		index--
	} else {
		index = 0
	}

	class := creatureStateClass(creature)
	thaco := 20
	if class >= 0 && class < len(stateLegacyTHACOList) {
		thaco = stateLegacyTHACOList[class][index]
	}

	if weapon, ok := w.creatureWieldedObjectLocked(creature); ok {
		if adjustment, ok := w.objectIntPropertyLocked(weapon, "adjustment"); ok {
			thaco -= adjustment
		}
	}
	thaco -= w.creatureModifiedWeaponProficiencyLocked(creature)

	proficiencySum := 0

	for i := 0; i < 4; i++ {
		proficiencySum += stateCreatureWeaponProficiency(creature, i)
	}
	for i := 0; i < 4; i++ {
		proficiencySum += stateCreatureMagicProficiency(creature, i)
	}
	thaco -= proficiencySum / 50

	if creatureHasAnyFlag(creature, "PBLESS", "bless") {
		thaco -= 3
	}
	if creatureHasAnyFlag(creature, "PREFLECT", "reflect", "reflection") {
		thaco -= 1
	}
	if creatureHasAnyFlag(creature, "PSHADOW", "shadow", "shadowClone") {
		thaco -= 3
	}
	if creatureHasAnyFlag(creature, "PABSORB", "absorb") {
		thaco -= 2
	}
	if creatureHasAnyFlag(creature, "PCHOI", "choi") {
		thaco += 5
	}
	if creatureHasAnyFlag(creature, "PSLAYE", "slaye", "accurate", "slayer") {
		thaco -= 3
	}
	if class == model.ClassDM {
		thaco -= 60
	}
	if class == model.ClassBulsa {
		thaco -= 14
	}
	return thaco
}

func (w *World) creatureModifiedWeaponProficiencyLocked(creature model.Creature) int {
	divisor := 40
	switch creatureStateClass(creature) {
	case model.ClassFighter, model.ClassBarbarian, model.ClassInvincible, model.ClassCaretaker:
		divisor = 20
	case model.ClassRanger, model.ClassPaladin:
		divisor = 25
	case model.ClassThief, model.ClassAssassin, model.ClassCleric:
		divisor = 30
	}

	weaponType := 2
	if weapon, ok := w.creatureWieldedObjectLocked(creature); ok {
		if value, ok := w.objectIntPropertyLocked(weapon, "type"); ok && value >= 0 && value <= 4 {
			weaponType = value
		}
	}
	return stateCreatureWeaponProficiency(creature, weaponType) / divisor
}

func (w *World) creatureWieldedObjectLocked(creature model.Creature) (model.ObjectInstance, bool) {
	for _, slot := range []string{"wield", "weapon", "mainHand", "right"} {
		objectID := creature.Equipment[slot]
		if objectID.IsZero() {
			continue
		}
		object, ok := w.objects[objectID]
		if ok {
			return object, true
		}
	}
	return model.ObjectInstance{}, false
}

func creatureStateLevel(creature model.Creature) int {
	if level, ok := creature.Stats["level"]; ok {
		return level
	}
	if creature.Level != 0 {
		return creature.Level
	}
	if level, ok := stateCreatureIntValue(creature, "level"); ok {
		return level
	}
	return 0
}

func creatureStateInt(creature model.Creature, key string) int {
	if value, ok := stateCreatureIntValue(creature, key); ok {
		return value
	}
	return 0
}

func stateCreatureWeaponProficiency(creature model.Creature, index int) int {
	value := stateCreatureProficiencyValue(creature, index)
	var table [12]int64
	switch creatureStateClass(creature) {
	case model.ClassFighter, model.ClassInvincible, model.ClassCaretaker, model.ClassBulsa, model.ClassSubDM, model.ClassDM:
		table = [12]int64{0, 768, 1024, 1440, 1910, 16000, 31214, 167000, 268488, 695000, 934808, 500000000}
	case model.ClassBarbarian:
		table = [12]int64{0, 1536, 2048, 2880, 3820, 32000, 62428, 334000, 536976, 1390000, 1869616, 500000000}
	case model.ClassThief, model.ClassRanger:
		table = [12]int64{0, 2304, 3072, 4320, 5730, 48000, 93642, 501000, 805464, 2085000, 2804424, 500000000}
	case model.ClassCleric, model.ClassPaladin, model.ClassAssassin:
		table = [12]int64{0, 3072, 4096, 5076, 7640, 64000, 124856, 668000, 1073952, 2780000, 3939232, 500000000}
	default:
		table = [12]int64{0, 5376, 7168, 10080, 13370, 112000, 218498, 1169000, 1879416, 4865000, 6543656, 500000000}
	}
	return stateProficiencyRank(value, table)
}

func stateCreatureMagicProficiency(creature model.Creature, index int) int {
	value := 0
	if index == 0 {
		value = stateCreatureProficiencyValue(creature, 4)
	} else if index >= 1 && index <= 4 {
		value = stateCreatureRealm(creature, index-1)
	}

	var table [12]int64
	switch creatureStateClass(creature) {
	case model.ClassMage, model.ClassInvincible, model.ClassCaretaker, model.ClassBulsa, model.ClassSubDM, model.ClassDM:
		table = [12]int64{0, 1024, 2048, 4096, 8192, 16384, 35768, 85536, 140000, 459410, 2073306, 500000000}
	case model.ClassCleric:
		table = [12]int64{0, 1024, 4092, 8192, 16384, 32768, 70536, 119000, 226410, 709410, 2973307, 500000000}
	case model.ClassPaladin, model.ClassRanger:
		table = [12]int64{0, 1024, 8192, 16384, 32768, 65536, 105000, 165410, 287306, 809410, 3538232, 500000000}
	default:
		table = [12]int64{0, 1024, 40000, 80000, 120000, 160000, 205000, 222000, 380000, 965410, 5495000, 500000000}
	}
	return stateProficiencyRank(value, table)
}

func stateCreatureProficiencyValue(creature model.Creature, index int) int {
	if index >= 0 && index < len(weaponProficiencyStatKeys) {
		part := weaponProficiencyPropertyKeys[index]
		for _, key := range []string{
			weaponProficiencyStatKeys[index],
			fmt.Sprintf("proficiency/%s", part),
			fmt.Sprintf("proficiency.%s", part),
			fmt.Sprintf("proficiency_%s", part),
		} {
			if value, ok := stateCreatureIntValue(creature, key); ok {
				return value
			}
		}
	}
	return stateCreatureIndexedValue(creature, "proficiency", index)
}

func stateCreatureIntValue(creature model.Creature, key string) (int, bool) {
	if creature.Stats != nil {
		if value, ok := creature.Stats[key]; ok {
			return value, true
		}
	}
	if creature.Properties != nil {
		if raw, ok := creature.Properties[key]; ok {
			value, err := strconv.Atoi(strings.TrimSpace(raw))
			if err == nil {
				return value, true
			}
		}
	}
	target := normalizeFlagName(key)
	if target == "" {
		return 0, false
	}
	for statKey, value := range creature.Stats {
		if normalizeFlagName(statKey) == target {
			return value, true
		}
	}
	for propertyKey, raw := range creature.Properties {
		if normalizeFlagName(propertyKey) == target {
			value, err := strconv.Atoi(strings.TrimSpace(raw))
			if err == nil {
				return value, true
			}
		}
	}
	return 0, false
}

func stateCreatureRealm(creature model.Creature, index int) int {
	keys := []string{"realmEarth", "realmWind", "realmFire", "realmWater"}
	if index >= 0 && index < len(keys) {
		if value, ok := creature.Stats[keys[index]]; ok {
			return value
		}
	}
	return stateCreatureIndexedValue(creature, "realm", index+1)
}

func stateCreatureIndexedValue(creature model.Creature, prefix string, index int) int {
	keys := []string{
		fmt.Sprintf("%s/%d", prefix, index),
		fmt.Sprintf("%s.%d", prefix, index),
		fmt.Sprintf("%s_%d", prefix, index),
		fmt.Sprintf("%s%d", prefix, index),
	}
	for _, key := range keys {
		if creature.Stats != nil {
			if value, ok := creature.Stats[key]; ok {
				return value
			}
		}
		if creature.Properties != nil {
			if raw, ok := creature.Properties[key]; ok {
				value, err := strconv.Atoi(strings.TrimSpace(raw))
				if err == nil {
					return value
				}
			}
		}
	}
	return 0
}

// SetCreatureLevel sets the canonical creature level and mirrors it into the
// legacy numeric stat map used by many command ports.
func (w *World) SetCreatureLevel(creatureID model.CreatureID, level int) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("set creature %q level: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("set creature level: creature id is required")
	}
	if level < 0 {
		return model.Creature{}, fmt.Errorf("set creature %q level: level must be non-negative", creatureID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("set creature %q level: creature not found", creatureID)
	}
	creature.Level = level
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats["level"] = level
	w.creatures[creatureID] = creature
	return cloneCreature(creature), nil
}

// UseCreatureCooldown starts a runtime cooldown when it is available. It
// returns the remaining seconds and false when the cooldown is still active.
func (w *World) UseCreatureCooldown(creatureID model.CreatureID, key string, nowUnix int64, intervalSeconds int64) (int64, bool, error) {
	if w == nil {
		return 0, false, fmt.Errorf("use creature %q cooldown %q: world state is nil", creatureID, key)
	}
	if creatureID.IsZero() {
		return 0, false, fmt.Errorf("use creature cooldown %q: creature id is required", key)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return 0, false, fmt.Errorf("use creature %q cooldown: key is required", creatureID)
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	if _, ok := w.creatures[creatureID]; !ok {
		return 0, false, fmt.Errorf("use creature %q cooldown %q: creature not found", creatureID, key)
	}
	if w.cooldowns == nil {
		w.cooldowns = map[model.CreatureID]map[string]int64{}
	}
	expiresByKey := w.cooldowns[creatureID]
	if expiresByKey == nil {
		expiresByKey = map[string]int64{}
		w.cooldowns[creatureID] = expiresByKey
	}
	if expires := expiresByKey[key]; expires > nowUnix {
		return expires - nowUnix, false, nil
	}
	if intervalSeconds <= 0 {
		return 0, true, nil
	}
	expiresByKey[key] = nowUnix + intervalSeconds
	return 0, true, nil
}

// SetCreatureCooldown sets or replaces a runtime cooldown for a creature.
func (w *World) SetCreatureCooldown(creatureID model.CreatureID, key string, nowUnix int64, intervalSeconds int64) error {
	if w == nil {
		return fmt.Errorf("set creature %q cooldown %q: world state is nil", creatureID, key)
	}
	if creatureID.IsZero() {
		return fmt.Errorf("set creature cooldown %q: creature id is required", key)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("set creature %q cooldown: key is required", creatureID)
	}
	if intervalSeconds <= 0 {
		return nil
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	if _, ok := w.creatures[creatureID]; !ok {
		return fmt.Errorf("set creature %q cooldown %q: creature not found", creatureID, key)
	}
	if w.cooldowns == nil {
		w.cooldowns = map[model.CreatureID]map[string]int64{}
	}
	expiresByKey := w.cooldowns[creatureID]
	if expiresByKey == nil {
		expiresByKey = map[string]int64{}
		w.cooldowns[creatureID] = expiresByKey
	}
	expiresByKey[key] = nowUnix + intervalSeconds
	return nil
}

// CreatureCooldownExpires reports the stored absolute expiration time for a
// creature cooldown without starting or extending the timer.
func (w *World) CreatureCooldownExpires(creatureID model.CreatureID, key string) (int64, bool, error) {
	if w == nil {
		return 0, false, fmt.Errorf("get creature %q cooldown %q: world state is nil", creatureID, key)
	}
	if creatureID.IsZero() {
		return 0, false, fmt.Errorf("get creature cooldown %q: creature id is required", key)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return 0, false, fmt.Errorf("get creature %q cooldown: key is required", creatureID)
	}

	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	if _, ok := w.creatures[creatureID]; !ok {
		return 0, false, fmt.Errorf("get creature %q cooldown %q: creature not found", creatureID, key)
	}
	if w.cooldowns == nil || w.cooldowns[creatureID] == nil {
		return 0, false, nil
	}
	expires, ok := w.cooldowns[creatureID][key]
	return expires, ok, nil
}

// UpdateCreatureTags adds and removes creature metadata tags under the world
// lock. Tag matching for removals uses the same normalized legacy flag
// comparison as command visibility checks.
func (w *World) UpdateCreatureTags(creatureID model.CreatureID, add []string, remove []string) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("update creature %q tags: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("update creature tags: creature id is required")
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("update creature %q tags: creature not found", creatureID)
	}
	creature.Metadata.Tags = addMetadataTags(removeMetadataTags(creature.Metadata.Tags, remove), add)
	w.creatures[creatureID] = creature
	return cloneCreature(creature), nil
}

// ApplyCreatureDamage subtracts damage from hpCurrent. It does not remove the
// creature from room indexes; death handling such as corpses, drops, and respawn
// belongs to the combat/gameplay layer.
func (w *World) ApplyCreatureDamage(creatureID model.CreatureID, damage int) (model.Creature, int, bool, error) {
	if w == nil {
		return model.Creature{}, 0, false, fmt.Errorf("damage creature %q: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, 0, false, fmt.Errorf("damage creature: creature id is required")
	}
	if damage < 0 {
		return model.Creature{}, 0, false, fmt.Errorf("damage creature %q: damage cannot be negative", creatureID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, 0, false, fmt.Errorf("damage creature %q: creature not found", creatureID)
	}
	next := cloneCreature(creature)
	current, ok := next.Stats["hpCurrent"]
	if next.Stats == nil || !ok {
		return model.Creature{}, 0, false, fmt.Errorf("damage creature %q: hpCurrent stat not found", creatureID)
	}

	actual := damage
	if current < 0 {
		current = 0
	}
	if actual > current {
		actual = current
	}
	remaining := current - actual
	next.Stats["hpCurrent"] = remaining
	dead := remaining <= 0
	w.creatures[next.ID] = next

	if !next.PlayerID.IsZero() {
		w.MarkPlayerDirty(next.PlayerID)
	}

	return cloneCreature(next), actual, dead, nil
}

// RecordCreatureDamage adds damage credit to the monster death reward ledger.
// Most callers pass the actual damage returned by ApplyCreatureDamage; legacy
// commands with separate reward-credit formulas pass the C ledger amount.
func (w *World) RecordCreatureDamage(victimID model.CreatureID, attackerID model.CreatureID, damage int) error {
	if w == nil {
		return fmt.Errorf("record creature damage %q from %q: world state is nil", victimID, attackerID)
	}
	if victimID.IsZero() {
		return fmt.Errorf("record creature damage: victim id is required")
	}
	if attackerID.IsZero() {
		return fmt.Errorf("record creature damage %q: attacker id is required", victimID)
	}
	if damage < 0 {
		return fmt.Errorf("record creature damage %q from %q: damage cannot be negative", victimID, attackerID)
	}
	if damage == 0 {
		return nil
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	if _, ok := w.creatures[victimID]; !ok {
		return fmt.Errorf("record creature damage %q: victim not found", victimID)
	}
	if _, ok := w.creatures[attackerID]; !ok {
		return fmt.Errorf("record creature damage %q from %q: attacker not found", victimID, attackerID)
	}
	if w.monsterDamage == nil {
		w.monsterDamage = map[model.CreatureID]map[model.CreatureID]int{}
	}
	if w.monsterDamage[victimID] == nil {
		w.monsterDamage[victimID] = map[model.CreatureID]int{}
	}
	w.monsterDamage[victimID][attackerID] += damage

	if w.enemies == nil {
		w.enemies = make(map[model.CreatureID][]string)
	}
	var name string
	if c, ok := w.creatures[attackerID]; ok {
		name = c.DisplayName
	} else if p, ok := w.players[model.PlayerID(attackerID)]; ok {
		name = p.DisplayName
	} else {
		name = string(attackerID)
	}
	if name != "" {
		found := false
		for _, existing := range w.enemies[victimID] {
			if existing == name {
				found = true
				break
			}
		}
		if !found {
			w.enemies[victimID] = append(w.enemies[victimID], name)
		}
	}

	if c, ok := w.creatures[victimID]; ok {
		hasTag := false
		for _, tag := range c.Metadata.Tags {
			if strings.EqualFold(tag, "was_attacked") {
				hasTag = true
				break
			}
		}
		if !hasTag {
			c.Metadata.Tags = append(c.Metadata.Tags, "was_attacked")
			w.creatures[victimID] = c
		}
	}

	return nil
}

// FinalizeMonsterDeath removes a dead monster from its room and drops carried
// inventory and gold into that room. It also awards player damage ledger
// experience, alignment shifts, and weapon proficiency when the equipped weapon
// type is available.
func (w *World) FinalizeMonsterDeath(creatureID model.CreatureID) (bool, error) {
	return w.FinalizeMonsterDeathWithOptions(creatureID, FinalizeMonsterDeathOptions{})
}

// FinalizeMonsterDeathWithOptions finalizes a dead monster using external
// reward context such as a group membership snapshot.
func (w *World) FinalizeMonsterDeathWithOptions(creatureID model.CreatureID, options FinalizeMonsterDeathOptions) (bool, error) {
	if w == nil {
		return false, fmt.Errorf("finalize monster death %q: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return false, fmt.Errorf("finalize monster death: creature id is required")
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	creature, ok := w.creatures[creatureID]
	if !ok {
		return false, fmt.Errorf("finalize monster death %q: creature not found", creatureID)
	}
	if creature.Kind == model.CreatureKindPlayer || !creature.PlayerID.IsZero() {
		return false, fmt.Errorf("finalize monster death %q: creature is a player", creatureID)
	}
	if hp, ok := creature.Stats["hpCurrent"]; ok && hp > 0 {
		return false, nil
	}
	if creature.RoomID.IsZero() {
		return false, fmt.Errorf("finalize monster death %q: creature has no room", creatureID)
	}
	room, ok := w.rooms[creature.RoomID]
	if !ok {
		return false, fmt.Errorf("finalize monster death %q: room %q not found", creatureID, creature.RoomID)
	}

	w.awardMonsterDeathRewardsLocked(creature, options)

	drops := carriedCreatureObjectIDs(creature)
	isTradeItems := creatureHasAnyFlag(creature, "tradeItems", "MTRADE")
	if isTradeItems {
		for _, objectID := range drops {
			w.deleteObjectTreeLocked(objectID, map[model.ObjectInstanceID]struct{}{})
		}
	} else {
		seenDrops := make(map[model.ObjectInstanceID]struct{}, len(drops))
		for _, objectID := range drops {
			if objectID.IsZero() {
				continue
			}
			if _, seen := seenDrops[objectID]; seen {
				continue
			}
			seenDrops[objectID] = struct{}{}
			object, ok := w.objects[objectID]
			if !ok || object.Location.CreatureID != creature.ID {
				continue
			}
			w.removeObjectFromHolderLocked(objectID, object.Location)
			object.Location = model.ObjectLocation{RoomID: room.ID}
			w.objects[objectID] = object
			w.addObjectToHolderLocked(objectID, object.Location)
		}
	}

	if gold := creature.Stats["gold"]; gold > 0 {
		moneyID := w.nextObjectCloneIDLocked("object:money")
		moneyPrototypeID := model.PrototypeID("prototype:money")
		if _, ok := w.prototypes[moneyPrototypeID]; !ok {
			w.prototypes[moneyPrototypeID] = model.ObjectPrototype{
				ID:          moneyPrototypeID,
				Kind:        model.ObjectKindMoney,
				DisplayName: "돈",
			}
		}
		money := model.ObjectInstance{
			ID:                  moneyID,
			PrototypeID:         moneyPrototypeID,
			DisplayNameOverride: fmt.Sprintf("%d냥", gold),
			Location:            model.ObjectLocation{RoomID: room.ID},
			Properties: map[string]string{
				"kind":  string(model.ObjectKindMoney),
				"type":  "10",
				"value": strconv.Itoa(gold),
			},
		}
		if err := money.Validate(); err != nil {
			return false, fmt.Errorf("finalize monster death %q: money object: %w", creatureID, err)
		}
		w.objects[money.ID] = money
		w.addObjectToHolderLocked(money.ID, money.Location)
	}

	goldHad := creature.Stats != nil && creature.Stats["gold"] > 0
	itemsDroppedToFloor := !isTradeItems && len(drops) > 0
	if itemsDroppedToFloor || goldHad {
		w.MarkRoomObjectsDirty(room.ID)
	}

	room = w.rooms[room.ID]
	room.CreatureIDs = removeID(room.CreatureIDs, creature.ID)
	w.rooms[room.ID] = room
	delete(w.creatures, creature.ID)
	delete(w.monsterDamage, creature.ID)
	w.pruneCharmReferencesLocked(creature)
	if w.enemies != nil {
		delete(w.enemies, creature.ID)

		deadName := creature.DisplayName
		for id, lst := range w.enemies {
			newLst := make([]string, 0, len(lst))
			for _, n := range lst {
				if n != deadName {
					newLst = append(newLst, n)
				}
			}
			w.enemies[id] = newLst
		}
	}
	return true, nil
}

func (w *World) awardMonsterDeathRewardsLocked(monster model.Creature, options FinalizeMonsterDeathOptions) {
	ledger := w.monsterDamage[monster.ID]
	if len(ledger) == 0 {
		return
	}

	monsterExp := monster.Stats["experience"]
	monsterAlignment := monster.Stats["alignment"]
	hpMax := monster.Stats["hpMax"]
	if hpMax < 1 {
		hpMax = 1
	}

	for attackerID, damage := range ledger {
		if damage <= 0 {
			continue
		}
		attacker, ok := w.creatures[attackerID]
		if !ok {
			continue
		}
		if !w.creatureIsPlayerRewardRecipientLocked(attacker) {
			continue
		}
		if attacker.Stats == nil {
			attacker.Stats = map[string]int{}
		}
		expGain := (monsterExp * damage) / hpMax
		if expGain > monsterExp {
			expGain = monsterExp
		}
		if expGain > 0 {
			attacker.Stats["experience"] += expGain + w.monsterDeathGroupBonusLocked(options.RewardGroup, attacker.ID, monsterExp, expGain)
			if proficiencyKey, ok := w.creatureWeaponProficiencyStatKeyLocked(attacker); ok {
				attacker.Stats[proficiencyKey] += expGain
			}
		}
		attacker.Stats["alignment"] = clampInt(attacker.Stats["alignment"]-monsterAlignment/5, -1000, 1000)
		w.creatures[attackerID] = attacker

		if w.creatureIsPlayerRewardRecipientLocked(attacker) {
			if player, ok := w.players[attacker.PlayerID]; ok {

				_ = player
			}
		}
	}
}

func (w *World) monsterDeathGroupBonusLocked(group MonsterDeathRewardGroup, recipientID model.CreatureID, monsterExp, expGain int) int {
	if group.LeaderID.IsZero() || recipientID.IsZero() || expGain <= 0 || expGain == monsterExp {
		return 0
	}

	followerCount, recipientIsFollower := w.monsterDeathGroupFollowerCountLocked(group, recipientID)
	if followerCount <= 0 {
		return 0
	}

	switch {
	case recipientID == group.LeaderID:
		if expGain+(expGain/2)*followerCount > monsterExp*2 {
			return expGain
		}
		return (expGain / 5) * (followerCount + 1)
	case recipientIsFollower:
		if expGain+(expGain/3)*followerCount > monsterExp*2 {
			return expGain
		}
		return (expGain / 4) * followerCount
	default:
		return 0
	}
}

func (w *World) monsterDeathGroupFollowerCountLocked(group MonsterDeathRewardGroup, recipientID model.CreatureID) (int, bool) {
	seen := map[model.CreatureID]struct{}{}
	count := 0
	recipientIsFollower := false
	for _, followerID := range group.FollowerIDs {
		if followerID.IsZero() || followerID == group.LeaderID {
			continue
		}
		if _, ok := seen[followerID]; ok {
			continue
		}
		seen[followerID] = struct{}{}

		follower, ok := w.creatures[followerID]
		if !ok || w.creatureHasDMInvisibleFlagLocked(follower) {
			continue
		}
		count++
		if followerID == recipientID {
			recipientIsFollower = true
		}
	}
	return count, recipientIsFollower
}

func (w *World) creatureWeaponProficiencyStatKeyLocked(creature model.Creature) (string, bool) {
	if len(creature.Equipment) == 0 {
		return "", false
	}

	for _, slot := range []string{"wield", "weapon", "mainHand", "right"} {
		objectID := creature.Equipment[slot]
		if key, ok := w.objectWeaponProficiencyStatKeyLocked(objectID); ok {
			return key, true
		}
	}

	var foundKey string
	for _, objectID := range creature.Equipment {
		key, ok := w.objectWeaponProficiencyStatKeyLocked(objectID)
		if !ok {
			continue
		}
		if foundKey != "" {
			return "", false
		}
		foundKey = key
	}
	return foundKey, foundKey != ""
}

func carriedCreatureObjectIDs(creature model.Creature) []model.ObjectInstanceID {
	ids := append([]model.ObjectInstanceID(nil), creature.Inventory.ObjectIDs...)
	for _, objectID := range creature.Equipment {
		ids = append(ids, objectID)
	}
	return ids
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
	if len(creature.Properties) == 0 {
		return false
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

func (w *World) creatureHasDMInvisibleFlagLocked(creature model.Creature) bool {
	targets := normalizedFlagSet("PDMINV", "dmInvisible")
	for key, value := range creature.Stats {
		if _, ok := targets[normalizeFlagName(key)]; ok && value != 0 {
			return true
		}
	}
	return creatureHasAnyFlag(creature, "PDMINV", "dmInvisible")
}

func (w *World) sortCreatureIDsLegacyLocked(ids []model.CreatureID) []model.CreatureID {
	var out []model.CreatureID
	for _, id := range ids {
		out = w.insertCreatureIDLegacySortedLocked(out, id)
	}
	return out
}

func (w *World) insertCreatureIDLegacySortedLocked(ids []model.CreatureID, creatureID model.CreatureID) []model.CreatureID {
	for _, existing := range ids {
		if existing == creatureID {
			return ids
		}
	}
	out := make([]model.CreatureID, 0, len(ids)+1)
	inserted := false
	for _, existing := range ids {
		if !inserted && w.creatureIDLegacyLessLocked(creatureID, existing) {
			out = append(out, creatureID)
			inserted = true
		}
		out = append(out, existing)
	}
	if !inserted {
		out = append(out, creatureID)
	}
	return out
}

func (w *World) creatureIDLegacyLessLocked(leftID, rightID model.CreatureID) bool {
	return strings.Compare(w.creatureLegacySortNameLocked(leftID), w.creatureLegacySortNameLocked(rightID)) < 0
}

func (w *World) creatureLegacySortNameLocked(creatureID model.CreatureID) string {
	creature, ok := w.creatures[creatureID]
	if !ok {
		return string(creatureID)
	}
	if name := creatureLegacySortName(creature); name != "" {
		return name
	}
	return string(creature.ID)
}

func creatureLegacySortName(creature model.Creature) string {
	if name := strings.TrimSpace(creature.DisplayName); name != "" {
		return name
	}
	for _, key := range []string{"name", "key[0]", "key[1]", "key[2]", "key/1", "key/2", "key/3"} {
		if name := strings.TrimSpace(creature.Properties[key]); name != "" {
			return name
		}
	}
	return ""
}

func (w *World) creatureCarriedWeightLocked(creature model.Creature) int {
	seen := map[model.ObjectInstanceID]struct{}{}
	weight := 0
	for _, id := range creature.Inventory.ObjectIDs {
		weight += w.carriedObjectWeightLocked(id, true, seen)
	}
	for _, id := range creature.Equipment {
		weight += w.carriedObjectWeightLocked(id, false, seen)
	}
	return weight
}

func cloneCreature(creature model.Creature) model.Creature {
	creature.Inventory = cloneObjectRefList(creature.Inventory)
	creature.Equipment = maps.Clone(creature.Equipment)
	creature.Stats = maps.Clone(creature.Stats)
	creature.Properties = maps.Clone(creature.Properties)
	creature.Metadata = cloneMetadata(creature.Metadata)
	return creature
}
