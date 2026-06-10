package command

import (
	"fmt"
	"strings"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type mockDMCreateCrtWorld struct {
	players          map[model.PlayerID]model.Player
	creatures        map[model.CreatureID]model.Creature
	rooms            map[model.RoomID]model.Room
	prototypes       map[model.CreatureID]model.Creature
	spawnedCreatures []model.CreatureID
	spawnedRooms     []model.RoomID
	spawnErr         error
}

func (w *mockDMCreateCrtWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMCreateCrtWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMCreateCrtWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *mockDMCreateCrtWorld) CreaturePrototype(id model.CreatureID) (model.Creature, bool) {
	p, ok := w.prototypes[id]
	return p, ok
}

func (w *mockDMCreateCrtWorld) SpawnCreature(protoID model.CreatureID, roomID model.RoomID, carryItems bool) (model.CreatureID, error) {
	if w.spawnErr != nil {
		return "", w.spawnErr
	}
	if _, ok := w.prototypes[protoID]; !ok {
		return "", fmt.Errorf("prototype not found: %s", protoID)
	}
	cloneID := model.CreatureID(fmt.Sprintf("%s:clone:%d", protoID, len(w.spawnedCreatures)+1))
	w.spawnedCreatures = append(w.spawnedCreatures, cloneID)
	w.spawnedRooms = append(w.spawnedRooms, roomID)
	return cloneID, nil
}

func TestDMCreateCrt_Permissions(t *testing.T) {
	tests := []struct {
		name        string
		class       int
		wantStatus  Status
		wantOutput  string
		wantSpawned int
	}{
		{
			name:       "regular class below SUB_DM",
			class:      model.ClassInvincible,
			wantStatus: StatusPrompt,
			wantOutput: "",
		},
		{
			name:       "caretaker below SUB_DM",
			class:      model.ClassCaretaker,
			wantStatus: StatusPrompt,
			wantOutput: "",
		},
		{
			name:       "bulsa below SUB_DM",
			class:      model.ClassBulsa,
			wantStatus: StatusPrompt,
			wantOutput: "",
		},
		{
			name:        "class equal to SUB_DM",
			class:       model.ClassSubDM,
			wantStatus:  StatusDefault,
			wantOutput:  "",
			wantSpawned: 1,
		},
		{
			name:        "class above SUB_DM",
			class:       model.ClassDM,
			wantStatus:  StatusDefault,
			wantOutput:  "",
			wantSpawned: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMCreateCrtWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {ID: "creature:alice", RoomID: "room:1", Stats: map[string]int{"class": tt.class}},
				},
				rooms: map[model.RoomID]model.Room{
					"room:1": {ID: "room:1"},
				},
				prototypes: map[model.CreatureID]model.Creature{
					"creature:m01:23": {ID: "creature:m01:23", DisplayName: "몬스터"},
				},
			}

			ctx := &Context{ActorID: "player:alice"}
			resolved := ResolvedCommand{
				Parsed: commandparse.Command{
					Val: [commandparse.CommandMax]int64{123},
				},
				Spec: commandspec.CommandSpec{
					Name:       "*create_crt",
					Handler:    "dm_create_crt",
					Privileged: true,
				},
			}

			handler := NewDMCreateCrtHandler(world)
			status, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}
			if status != tt.wantStatus {
				t.Fatalf("status = %v, want %v", status, tt.wantStatus)
			}

			got := ctx.OutputString()
			if got != tt.wantOutput {
				t.Errorf("output = %q, want %q", got, tt.wantOutput)
			}
			if len(world.spawnedCreatures) != tt.wantSpawned {
				t.Errorf("spawned creatures = %d, want %d", len(world.spawnedCreatures), tt.wantSpawned)
			}
		})
	}
}

func TestDMCreateCrt_PrototypeNotFound(t *testing.T) {
	world := &mockDMCreateCrtWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", RoomID: "room:1", Stats: map[string]int{"class": 12}},
		},
		rooms: map[model.RoomID]model.Room{
			"room:1": {ID: "room:1"},
		},
		prototypes: map[model.CreatureID]model.Creature{},
	}

	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Parsed: commandparse.Command{
			Val: [commandparse.CommandMax]int64{999},
		},
		Spec: commandspec.CommandSpec{
			Name:       "*create_crt",
			Handler:    "dm_create_crt",
			Privileged: true,
		},
	}

	handler := NewDMCreateCrtHandler(world)
	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	got := ctx.OutputString()
	want := "에러 (999)\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestDMCreateCrt_CreatureBackedActorUsesLegacyCreaturePointer(t *testing.T) {
	world := &mockDMCreateCrtWorld{
		creatures: map[model.CreatureID]model.Creature{
			"creature:dm": {ID: "creature:dm", RoomID: "room:1", Stats: map[string]int{"class": model.ClassSubDM}},
		},
		rooms: map[model.RoomID]model.Room{
			"room:1": {ID: "room:1"},
		},
		prototypes: map[model.CreatureID]model.Creature{
			"creature:m01:23": {ID: "creature:m01:23", DisplayName: "몬스터"},
		},
	}

	ctx := &Context{ActorID: "creature:dm"}
	resolved := ResolvedCommand{
		Parsed: commandparse.Command{
			Val: [commandparse.CommandMax]int64{123},
		},
		Spec: commandspec.CommandSpec{
			Name:       "*create_crt",
			Handler:    "dm_create_crt",
			Privileged: true,
		},
	}

	status, err := NewDMCreateCrtHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want %v", status, StatusDefault)
	}
	if len(world.spawnedCreatures) != 1 {
		t.Fatalf("spawned %d creatures, want 1", len(world.spawnedCreatures))
	}
	if len(world.spawnedRooms) != 1 || world.spawnedRooms[0] != "room:1" {
		t.Fatalf("spawned rooms = %v, want [room:1]", world.spawnedRooms)
	}
}

func TestDMCreateCrtFallsBackToPlayerRoomWhenCreatureRoomMissing(t *testing.T) {
	world := &mockDMCreateCrtWorld{
		players: map[model.PlayerID]model.Player{
			"player:dm": {ID: "player:dm", CreatureID: "creature:dm", RoomID: "room:1"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": model.ClassSubDM}},
		},
		rooms: map[model.RoomID]model.Room{
			"room:1": {ID: "room:1"},
		},
		prototypes: map[model.CreatureID]model.Creature{
			"creature:m01:23": {ID: "creature:m01:23", DisplayName: "몬스터"},
		},
	}

	ctx := &Context{ActorID: "player:dm"}
	resolved := ResolvedCommand{
		Parsed: commandparse.Command{
			Val: [commandparse.CommandMax]int64{123},
		},
		Spec: commandspec.CommandSpec{
			Name:       "*create_crt",
			Handler:    "dm_create_crt",
			Privileged: true,
		},
	}

	status, err := NewDMCreateCrtHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want %v", status, StatusDefault)
	}
	if len(world.spawnedCreatures) != 1 {
		t.Fatalf("spawned %d creatures, want 1", len(world.spawnedCreatures))
	}
	if len(world.spawnedRooms) != 1 || world.spawnedRooms[0] != "room:1" {
		t.Fatalf("spawned rooms = %v, want [room:1]", world.spawnedRooms)
	}
}

func TestDMCreateCrt_CountStarts_n(t *testing.T) {
	world := &mockDMCreateCrtWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", RoomID: "room:1", Stats: map[string]int{"class": 12}},
		},
		rooms: map[model.RoomID]model.Room{
			"room:1": {ID: "room:1"},
		},
		prototypes: map[model.CreatureID]model.Creature{
			"creature:m01:23": {ID: "creature:m01:23", DisplayName: "몬스터"},
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Parsed: commandparse.Command{
			Num: 2,
			Val: [commandparse.CommandMax]int64{123, 3},
			Str: [commandparse.CommandMax]string{"123", "n3"},
		},
		Spec: commandspec.CommandSpec{
			Name:       "*create_crt",
			Handler:    "dm_create_crt",
			Privileged: true,
		},
	}

	handler := NewDMCreateCrtHandler(world)
	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if len(world.spawnedCreatures) != 3 {
		t.Errorf("spawned %d creatures, want 3", len(world.spawnedCreatures))
	}
}

func TestDMCreateCrt_CountStarts_g(t *testing.T) {
	world := &mockDMCreateCrtWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", RoomID: "room:1", Stats: map[string]int{"class": 12}},
		},
		rooms: map[model.RoomID]model.Room{
			"room:1": {
				ID:        "room:1",
				PlayerIDs: []model.PlayerID{"player:alice", "player:bob"},
			},
		},
		prototypes: map[model.CreatureID]model.Creature{
			"creature:m01:23": {ID: "creature:m01:23", DisplayName: "몬스터"},
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Parsed: commandparse.Command{
			Num: 2,
			Val: [commandparse.CommandMax]int64{123, 0},
			Str: [commandparse.CommandMax]string{"123", "g"},
		},
		Spec: commandspec.CommandSpec{
			Name:       "*create_crt",
			Handler:    "dm_create_crt",
			Privileged: true,
		},
	}

	handler := NewDMCreateCrtHandler(world)
	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if len(world.spawnedCreatures) < 1 || len(world.spawnedCreatures) > 2 {
		t.Errorf("spawned %d creatures, want 1 or 2", len(world.spawnedCreatures))
	}
}

func TestDMCreateCrt_RandomCreatureFromRoom(t *testing.T) {
	// Set up room properties to have random slots
	props := make(map[string]string)
	for i := 0; i < 10; i++ {
		props[fmt.Sprintf("random[%d]", i)] = "105"
	}

	world := &mockDMCreateCrtWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", RoomID: "room:1", Stats: map[string]int{"class": 12}},
		},
		rooms: map[model.RoomID]model.Room{
			"room:1": {
				ID:         "room:1",
				Properties: props,
			},
		},
		prototypes: map[model.CreatureID]model.Creature{
			"creature:m01:5": {ID: "creature:m01:5", DisplayName: "랜덤몬스터"},
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Parsed: commandparse.Command{
			Val: [commandparse.CommandMax]int64{0}, // < 2 triggers random selection
		},
		Spec: commandspec.CommandSpec{
			Name:       "*create_crt",
			Handler:    "dm_create_crt",
			Privileged: true,
		},
	}

	handler := NewDMCreateCrtHandler(world)
	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if len(world.spawnedCreatures) != 1 {
		t.Fatalf("spawned %d creatures, want 1", len(world.spawnedCreatures))
	}

	gotID := world.spawnedCreatures[0]
	if !strings.Contains(string(gotID), "creature:m01:5") {
		t.Errorf("spawned creature ID = %q, want it to be based on prototype 105", gotID)
	}
}

func TestDMCreateCrt_CountStarts_g_And_Val1(t *testing.T) {
	// Set up room properties to have random slots
	props := make(map[string]string)
	for i := 0; i < 10; i++ {
		props[fmt.Sprintf("random[%d]", i)] = "105"
	}

	world := &mockDMCreateCrtWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", RoomID: "room:1", Stats: map[string]int{"class": 12}},
		},
		rooms: map[model.RoomID]model.Room{
			"room:1": {
				ID:         "room:1",
				Properties: props,
			},
		},
		prototypes: map[model.CreatureID]model.Creature{
			"creature:m01:5": {ID: "creature:m01:5", DisplayName: "랜덤몬스터"},
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Parsed: commandparse.Command{
			Num: 2,
			Val: [commandparse.CommandMax]int64{123, 1}, // Val[1] = 1 and starts with g triggers random room creature again
			Str: [commandparse.CommandMax]string{"123", "g1"},
		},
		Spec: commandspec.CommandSpec{
			Name:       "*create_crt",
			Handler:    "dm_create_crt",
			Privileged: true,
		},
	}

	handler := NewDMCreateCrtHandler(world)
	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if len(world.spawnedCreatures) != 1 {
		t.Fatalf("spawned %d creatures, want 1", len(world.spawnedCreatures))
	}

	gotID := world.spawnedCreatures[0]
	if !strings.Contains(string(gotID), "creature:m01:5") {
		t.Errorf("spawned creature ID = %q, want it to be based on prototype 105", gotID)
	}
}
