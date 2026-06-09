package state

import (
	"testing"

	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
)

func TestUpdateCreatureFamilyStateMarksLinkedPlayerDirty(t *testing.T) {
	world := familyDirtyWorld(t)

	if _, err := world.UpdateCreatureFamilyState("creature:alice", 2, true, false, false); err != nil {
		t.Fatalf("UpdateCreatureFamilyState() error = %v", err)
	}

	assertFamilyDirtyPlayer(t, world, "player:alice")
}

func TestUpdateCreatureGoldMarksLinkedPlayerDirty(t *testing.T) {
	world := familyDirtyWorld(t)

	if _, err := world.UpdateCreatureGold("creature:alice", 50000); err != nil {
		t.Fatalf("UpdateCreatureGold() error = %v", err)
	}

	assertFamilyDirtyPlayer(t, world, "player:alice")
}

func familyDirtyWorld(t *testing.T) *World {
	t.Helper()
	loaded := worldload.NewWorld()
	if err := loaded.AddPlayer(model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:start",
	}); err != nil {
		t.Fatalf("AddPlayer() error = %v", err)
	}
	if err := loaded.AddCreature(model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:start",
		Stats:       map[string]int{"gold": 10},
	}); err != nil {
		t.Fatalf("AddCreature() error = %v", err)
	}
	return NewWorld(loaded)
}

func assertFamilyDirtyPlayer(t *testing.T, world *World, playerID model.PlayerID) {
	t.Helper()
	world.dirtyMu.Lock()
	_, ok := world.playerDirty[playerID]
	world.dirtyMu.Unlock()
	if !ok {
		t.Fatalf("player %s was not marked dirty", playerID)
	}
}
