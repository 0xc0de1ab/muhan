package game

import (
	"fmt"
	"testing"

	"muhan/internal/world/model"
)

type spawnCall struct {
	ProtoID    model.CreatureID
	RoomID     model.RoomID
	CarryItems bool
}

type fakeWorld struct {
	rooms      map[model.RoomID]model.Room
	creatures  map[model.CreatureID]model.Creature
	prototypes map[model.CreatureID]model.Creature
	spawned    []spawnCall
}

func (f *fakeWorld) AllRoomIDs() []model.RoomID {
	var ids []model.RoomID
	for id := range f.rooms {
		ids = append(ids, id)
	}
	return ids
}

func (f *fakeWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := f.rooms[id]
	return r, ok
}

func (f *fakeWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := f.creatures[id]
	return c, ok
}

func (f *fakeWorld) CreaturePrototype(id model.CreatureID) (model.Creature, bool) {
	c, ok := f.prototypes[id]
	return c, ok
}

func (f *fakeWorld) SpawnCreature(protoID model.CreatureID, roomID model.RoomID, carryItems bool) (model.CreatureID, error) {
	f.spawned = append(f.spawned, spawnCall{
		ProtoID:    protoID,
		RoomID:     roomID,
		CarryItems: carryItems,
	})
	return model.CreatureID(fmt.Sprintf("spawned:%s", protoID)), nil
}

func TestUpdateRandomSpawns(t *testing.T) {
	// Setup prototype
	protoID := model.CreatureID("creature:m03:0") // 300 / 100 = 3, 300 % 100 = 0
	proto := model.Creature{
		ID:          protoID,
		Kind:        model.CreatureKindMonster,
		DisplayName: "Test Monster",
		Stats:       map[string]int{"numWander": 1},
	}

	tests := []struct {
		name          string
		room          model.Room
		playersInRoom []model.Player
		expectSpawn   bool
		expectedProto model.CreatureID
	}{
		{
			name: "Occupied room with traffic 100 spawns monster",
			room: model.Room{
				ID: "room:00001",
				Properties: map[string]string{
					"traffic": "100",
					"random":  "300,300,300,300,300,300,300,300,300,300",
				},
				PlayerIDs: []model.PlayerID{"player:1"},
			},
			expectSpawn:   true,
			expectedProto: protoID,
		},
		{
			name: "Unoccupied room does not spawn monster",
			room: model.Room{
				ID: "room:00002",
				Properties: map[string]string{
					"traffic": "100",
					"random":  "300,300,300,300,300,300,300,300,300,300",
				},
			},
			expectSpawn: false,
		},
		{
			name: "Occupied room with traffic 0 does not spawn monster",
			room: model.Room{
				ID: "room:00003",
				Properties: map[string]string{
					"traffic": "0",
					"random":  "300,300,300,300,300,300,300,300,300,300",
				},
				PlayerIDs: []model.PlayerID{"player:1"},
			},
			expectSpawn: false,
		},
		{
			name: "Occupied room with no traffic property does not spawn monster",
			room: model.Room{
				ID: "room:00004",
				Properties: map[string]string{
					"random": "300,300,300,300,300,300,300,300,300,300",
				},
				PlayerIDs: []model.PlayerID{"player:1"},
			},
			expectSpawn: false,
		},
		{
			name: "Occupied room with all zeros in random list does not spawn monster",
			room: model.Room{
				ID: "room:00005",
				Properties: map[string]string{
					"traffic": "100",
					"random":  "0,0,0,0,0,0,0,0,0,0",
				},
				PlayerIDs: []model.PlayerID{"player:1"},
			},
			expectSpawn: false,
		},
		{
			name: "Occupied room with player detected by creature list rather than room.PlayerIDs",
			room: model.Room{
				ID:          "room:00006",
				CreatureIDs: []model.CreatureID{"creature:p1"},
				Properties: map[string]string{
					"traffic": "100",
					"random":  "300,300,300,300,300,300,300,300,300,300",
				},
			},
			playersInRoom: []model.Player{
				{
					ID:         "player:p1",
					CreatureID: "creature:p1",
				},
			},
			expectSpawn:   true,
			expectedProto: protoID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			world := &fakeWorld{
				rooms:      map[model.RoomID]model.Room{tc.room.ID: tc.room},
				creatures:  map[model.CreatureID]model.Creature{},
				prototypes: map[model.CreatureID]model.Creature{protoID: proto},
			}

			// Add player creatures to world if specified
			for _, p := range tc.playersInRoom {
				world.creatures[p.CreatureID] = model.Creature{
					ID:       p.CreatureID,
					Kind:     model.CreatureKindPlayer,
					PlayerID: p.ID,
				}
			}

			UpdateRandomSpawns(world, 123456789)

			if tc.expectSpawn {
				if len(world.spawned) == 0 {
					t.Fatalf("expected spawn to happen, but none recorded")
				}
				if world.spawned[0].ProtoID != tc.expectedProto {
					t.Errorf("expected proto %s, got %s", tc.expectedProto, world.spawned[0].ProtoID)
				}
				if world.spawned[0].RoomID != tc.room.ID {
					t.Errorf("expected room %s, got %s", tc.room.ID, world.spawned[0].RoomID)
				}
			} else {
				if len(world.spawned) > 0 {
					t.Fatalf("expected no spawn, but got %d spawns", len(world.spawned))
				}
			}
		})
	}
}

func TestUpdateRandomSpawnsCount(t *testing.T) {
	protoID := model.CreatureID("creature:m03:0")
	proto := model.Creature{
		ID:          protoID,
		Kind:        model.CreatureKindMonster,
		DisplayName: "Wandering Monster",
		Stats:       map[string]int{"numWander": 5},
	}

	// Test spawning with numWander > 1
	room := model.Room{
		ID: "room:00001",
		Properties: map[string]string{
			"traffic": "100",
			"random":  "300,300,300,300,300,300,300,300,300,300",
		},
		PlayerIDs: []model.PlayerID{"player:1"},
	}

	world := &fakeWorld{
		rooms:      map[model.RoomID]model.Room{room.ID: room},
		creatures:  map[model.CreatureID]model.Creature{},
		prototypes: map[model.CreatureID]model.Creature{protoID: proto},
	}

	UpdateRandomSpawns(world, 123456789)

	if len(world.spawned) == 0 {
		t.Fatalf("expected at least 1 spawn")
	}
	if len(world.spawned) > 5 {
		t.Errorf("expected at most 5 spawns, got %d", len(world.spawned))
	}
}

func TestUpdateRandomSpawnsReadsLegacyRandomSlotProperties(t *testing.T) {
	protoID := model.CreatureID("creature:m03:0")
	proto := model.Creature{ID: protoID, Kind: model.CreatureKindMonster, DisplayName: "Wandering Monster"}
	props := map[string]string{"traffic": "100"}
	for i := 0; i < 10; i++ {
		props[fmt.Sprintf("random%d", i)] = "300"
	}
	props["random[9]"] = "300"

	room := model.Room{
		ID:         "room:00001",
		Properties: props,
		PlayerIDs:  []model.PlayerID{"player:1"},
	}
	world := &fakeWorld{
		rooms:      map[model.RoomID]model.Room{room.ID: room},
		creatures:  map[model.CreatureID]model.Creature{},
		prototypes: map[model.CreatureID]model.Creature{protoID: proto},
	}

	UpdateRandomSpawns(world, 123456789)

	if len(world.spawned) == 0 {
		t.Fatalf("expected spawn from legacy randomN/random[N] properties")
	}
	if got := world.spawned[0].ProtoID; got != protoID {
		t.Fatalf("spawn proto = %s, want %s", got, protoID)
	}
}

func TestUpdateRandomSpawnsUsesPropertyBackedPlayerWanderFlag(t *testing.T) {
	protoID := model.CreatureID("creature:m03:0")
	proto := model.Creature{
		ID:          protoID,
		Kind:        model.CreatureKindMonster,
		DisplayName: "Wandering Monster",
		Stats:       map[string]int{"numWander": 10},
	}
	room := model.Room{
		ID: "room:00001",
		Properties: map[string]string{
			"traffic": "100",
			"random":  "300,300,300,300,300,300,300,300,300,300",
			"RPLWAN":  "1",
		},
		PlayerIDs: []model.PlayerID{"player:1", "player:2"},
	}
	world := &fakeWorld{
		rooms:      map[model.RoomID]model.Room{room.ID: room},
		creatures:  map[model.CreatureID]model.Creature{},
		prototypes: map[model.CreatureID]model.Creature{protoID: proto},
	}

	UpdateRandomSpawns(world, 123456789)

	if len(world.spawned) == 0 {
		t.Fatalf("expected at least one spawn")
	}
	if len(world.spawned) > len(room.PlayerIDs) {
		t.Fatalf("spawn count = %d, want capped by visible player count %d for RPLWAN", len(world.spawned), len(room.PlayerIDs))
	}
}
