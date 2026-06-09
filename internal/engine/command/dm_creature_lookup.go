package command

import (
	"strings"

	"muhan/internal/world/model"
)

type dmCreatureInRoomFinder interface {
	FindCreatureInRoom(model.RoomID, string) (model.Creature, bool)
}

type dmCreatureByNameFinder interface {
	FindCreatureByName(model.RoomID, string, int) (model.Creature, bool)
}

type dmRoomCreatureLister interface {
	Room(model.RoomID) (model.Room, bool)
	Creature(model.CreatureID) (model.Creature, bool)
}

func dmFindCreatureInRoom(world dmCreatureInRoomFinder, roomID model.RoomID, name string, ordinal int64) (model.Creature, bool) {
	if finder, ok := any(world).(dmCreatureByNameFinder); ok {
		count := int(ordinal)
		if count < 1 {
			count = 1
		}
		return finder.FindCreatureByName(roomID, name, count)
	}
	return world.FindCreatureInRoom(roomID, name)
}

func dmFindMonsterInRoom(world dmCreatureInRoomFinder, roomID model.RoomID, name string, ordinal int64) (model.Creature, bool) {
	return dmFindMonsterInRoomForActor(world, model.Creature{}, roomID, name, ordinal)
}

func dmFindMonsterInRoomForActor(world dmCreatureInRoomFinder, actor model.Creature, roomID model.RoomID, name string, ordinal int64) (model.Creature, bool) {
	if lister, ok := any(world).(dmRoomCreatureLister); ok {
		if room, found := lister.Room(roomID); found && len(room.CreatureIDs) > 0 {
			return dmFindMonsterInRoomList(lister, actor, room, name, ordinal)
		}
	}

	if len(strings.TrimSpace(name)) < 2 {
		return model.Creature{}, false
	}
	creature, ok := dmFindCreatureInRoom(world, roomID, name, ordinal)
	if !ok || dmCreatureIsPlayer(creature) || !dmFindCrtVisibleForActor(actor, creature) {
		return model.Creature{}, false
	}
	return creature, true
}

func dmFindMonsterInRoomList(world dmRoomCreatureLister, actor model.Creature, room model.Room, name string, ordinal int64) (model.Creature, bool) {
	nameLower := strings.ToLower(strings.TrimSpace(name))
	if len(nameLower) < 2 {
		return model.Creature{}, false
	}
	count := int(ordinal)
	if count < 1 {
		count = 1
	}
	seen := 0
	for _, creatureID := range room.CreatureIDs {
		creature, ok := world.Creature(creatureID)
		if !ok || dmCreatureIsPlayer(creature) {
			continue
		}
		if !dmFindCrtVisibleForActor(actor, creature) || !dmCreatureLookupNameMatches(creature, nameLower) {
			continue
		}
		seen++
		if seen == count {
			return creature, true
		}
	}
	return model.Creature{}, false
}

func dmFindCrtVisibleForActor(actor, target model.Creature) bool {
	return legacyFindCrtVisible(target, creatureHasAnyFlag(actor, "PDINVI", "detectInvisible", "detectInvis"))
}

func dmCreatureIsPlayer(creature model.Creature) bool {
	return creature.Kind == model.CreatureKindPlayer || !creature.PlayerID.IsZero()
}

func dmCreatureLookupNameMatches(creature model.Creature, nameLower string) bool {
	terms := []string{
		creature.DisplayName,
		string(creature.ID),
	}
	for _, key := range []string{"name", "key[0]", "key[1]", "key[2]", "key/1", "key/2", "key/3"} {
		if val := strings.TrimSpace(creature.Properties[key]); val != "" {
			terms = append(terms, val)
		}
	}
	for _, term := range terms {
		termLower := strings.ToLower(strings.TrimSpace(term))
		if termLower == "" {
			continue
		}
		if strings.HasPrefix(termLower, nameLower) {
			return true
		}
		for _, word := range strings.Fields(termLower) {
			if strings.HasPrefix(word, nameLower) {
				return true
			}
		}
	}
	return false
}
