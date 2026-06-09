package command

import (
	"strings"

	"muhan/internal/world/model"
)

type dmFindObjWorld interface {
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
}

func dmFindObjInRoom(world dmFindObjWorld, room model.Room, name string, ordinal int64, detectInvisible bool) (model.ObjectInstance, bool) {
	return dmFindObjInRefs(world, room.Objects.ObjectIDs, name, ordinal, detectInvisible, func(object model.ObjectInstance) bool {
		return objectLocatedInRoom(object, room.ID)
	})
}

func dmFindObjInCreatureInventory(world dmFindObjWorld, creature model.Creature, name string, ordinal int64, detectInvisible bool) (model.ObjectInstance, bool) {
	return dmFindObjInRefs(world, creature.Inventory.ObjectIDs, name, ordinal, detectInvisible, func(object model.ObjectInstance) bool {
		return objectLocatedInCreatureInventory(object, creature.ID)
	})
}

func dmFindObjInRefs(
	world dmFindObjWorld,
	ids []model.ObjectInstanceID,
	name string,
	ordinal int64,
	detectInvisible bool,
	located func(model.ObjectInstance) bool,
) (model.ObjectInstance, bool) {
	name = strings.TrimSpace(name)
	if world == nil || name == "" {
		return model.ObjectInstance{}, false
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
		if !ok {
			continue
		}
		if located != nil && !located(object) {
			continue
		}
		if !getObjectVisibleForFindObj(world, object, detectInvisible) {
			continue
		}
		if !getObjectMatches(world, object, name) {
			continue
		}
		seen++
		if seen == ordinal {
			return object, true
		}
	}
	return model.ObjectInstance{}, false
}

func dmFindReadyObjOnCreature(world dmFindObjWorld, creature model.Creature, name string, ordinal int64) (model.ObjectInstance, bool) {
	name = strings.TrimSpace(name)
	if world == nil || name == "" {
		return model.ObjectInstance{}, false
	}
	if ordinal < 1 {
		ordinal = 1
	}

	var seen int64
	for _, slot := range orderedEquipmentSlots(creature.Equipment, legacyReadySlotOrder) {
		objectID := creature.Equipment[slot]
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInCreatureEquipment(object, creature.ID) {
			continue
		}
		if !getObjectMatches(world, object, name) {
			continue
		}
		seen++
		if seen == ordinal {
			return object, true
		}
	}
	return model.ObjectInstance{}, false
}
