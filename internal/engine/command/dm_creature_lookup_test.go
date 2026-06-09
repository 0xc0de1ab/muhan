package command

import (
	"strings"
	"testing"

	"muhan/internal/world/model"
)

type dmMonsterLookupTestWorld struct {
	room     model.Room
	creature map[model.CreatureID]model.Creature
}

func (w dmMonsterLookupTestWorld) Room(id model.RoomID) (model.Room, bool) {
	if w.room.ID != id {
		return model.Room{}, false
	}
	return w.room, true
}

func (w dmMonsterLookupTestWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creature[id]
	return c, ok
}

func (w dmMonsterLookupTestWorld) FindCreatureInRoom(roomID model.RoomID, name string) (model.Creature, bool) {
	for _, c := range w.creature {
		if c.RoomID == roomID && strings.EqualFold(c.DisplayName, name) {
			return c, true
		}
	}
	return model.Creature{}, false
}

func TestDMFindMonsterInRoomSkipsPlayerCreaturesForOrdinal(t *testing.T) {
	world := dmMonsterLookupTestWorld{
		room: model.Room{
			ID: "room:1",
			CreatureIDs: []model.CreatureID{
				"creature:player",
				"creature:monster1",
				"creature:monster2",
			},
		},
		creature: map[model.CreatureID]model.Creature{
			"creature:player": {
				ID:          "creature:player",
				Kind:        model.CreatureKindPlayer,
				PlayerID:    "player:bob",
				DisplayName: "늑대",
				RoomID:      "room:1",
			},
			"creature:monster1": {
				ID:          "creature:monster1",
				DisplayName: "늑대",
				RoomID:      "room:1",
			},
			"creature:monster2": {
				ID:          "creature:monster2",
				DisplayName: "늑대",
				RoomID:      "room:1",
			},
		},
	}

	first, ok := dmFindMonsterInRoom(world, "room:1", "늑대", 1)
	if !ok || first.ID != "creature:monster1" {
		t.Fatalf("first monster = %q/%v, want creature:monster1/true", first.ID, ok)
	}
	second, ok := dmFindMonsterInRoom(world, "room:1", "늑대", 2)
	if !ok || second.ID != "creature:monster2" {
		t.Fatalf("second monster = %q/%v, want creature:monster2/true", second.ID, ok)
	}
}

func TestDMFindMonsterInRoomRejectsSingleByteTargetLikeFindCrt(t *testing.T) {
	world := dmMonsterLookupTestWorld{
		room: model.Room{
			ID:          "room:1",
			CreatureIDs: []model.CreatureID{"creature:monster"},
		},
		creature: map[model.CreatureID]model.Creature{
			"creature:monster": {
				ID:          "creature:monster",
				DisplayName: "ab",
				RoomID:      "room:1",
			},
		},
	}

	if creature, ok := dmFindMonsterInRoom(world, "room:1", "a", 1); ok {
		t.Fatalf("single-byte target matched %q, want no match", creature.ID)
	}
}

func TestDMFindMonsterInRoomAppliesFindCrtVisibility(t *testing.T) {
	world := dmMonsterLookupTestWorld{
		room: model.Room{
			ID: "room:1",
			CreatureIDs: []model.CreatureID{
				"creature:minvis",
				"creature:visible",
				"creature:pdminv",
				"creature:normal",
			},
		},
		creature: map[model.CreatureID]model.Creature{
			"creature:minvis": {
				ID:          "creature:minvis",
				DisplayName: "늑대",
				RoomID:      "room:1",
				Metadata:    model.Metadata{Tags: []string{"MINVIS"}},
			},
			"creature:visible": {
				ID:          "creature:visible",
				DisplayName: "늑대",
				RoomID:      "room:1",
			},
			"creature:pdminv": {
				ID:          "creature:pdminv",
				DisplayName: "수호자",
				RoomID:      "room:1",
				Stats:       map[string]int{"class": legacyClassCaretaker},
				Metadata:    model.Metadata{Tags: []string{"PDMINV"}},
			},
			"creature:normal": {
				ID:          "creature:normal",
				DisplayName: "수호자",
				RoomID:      "room:1",
			},
		},
	}

	actor := model.Creature{ID: "creature:dm", Stats: map[string]int{"class": legacyClassDM}}
	creature, ok := dmFindMonsterInRoomForActor(world, actor, "room:1", "늑대", 1)
	if !ok || creature.ID != "creature:visible" {
		t.Fatalf("visible lookup = %q/%v, want creature:visible/true", creature.ID, ok)
	}

	actor.Metadata.Tags = []string{"PDINVI"}
	creature, ok = dmFindMonsterInRoomForActor(world, actor, "room:1", "늑대", 1)
	if !ok || creature.ID != "creature:minvis" {
		t.Fatalf("PDINVI lookup = %q/%v, want creature:minvis/true", creature.ID, ok)
	}

	creature, ok = dmFindMonsterInRoomForActor(world, actor, "room:1", "수호자", 1)
	if !ok || creature.ID != "creature:normal" {
		t.Fatalf("PDMINV skip lookup = %q/%v, want creature:normal/true", creature.ID, ok)
	}
}
