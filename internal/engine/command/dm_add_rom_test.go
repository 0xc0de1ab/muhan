package command

import (
	"fmt"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMAddRomWorld struct {
	players    map[model.PlayerID]model.Player
	creatures  map[model.CreatureID]model.Creature
	rooms      map[model.RoomID]model.Room
	created    []model.RoomID
	failCreate bool
}

func (w *mockDMAddRomWorld) Player(id model.PlayerID) (model.Player, bool) {
	player, ok := w.players[id]
	return player, ok
}

func (w *mockDMAddRomWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	creature, ok := w.creatures[id]
	return creature, ok
}

func (w *mockDMAddRomWorld) Room(id model.RoomID) (model.Room, bool) {
	room, ok := w.rooms[id]
	return room, ok
}

func (w *mockDMAddRomWorld) CreateRoom(id model.RoomID) error {
	if w.failCreate {
		return fmt.Errorf("simulated creation error")
	}
	w.created = append(w.created, id)
	w.rooms[id] = model.Room{ID: id}
	return nil
}

func TestDMAddRomClassValidation(t *testing.T) {
	tests := []struct {
		name       string
		class      int
		expectDeny bool
	}{
		{
			name:       "DM class 13 allowed",
			class:      13,
			expectDeny: false,
		},
		{
			name:       "DM class 14 allowed",
			class:      14,
			expectDeny: false,
		},
		{
			name:       "Caretaker class 10 denied",
			class:      10,
			expectDeny: true,
		},
		{
			name:       "Zone maker class 0 denied",
			class:      0,
			expectDeny: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMAddRomWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {
						ID:    "creature:alice",
						Stats: map[string]int{"class": tt.class},
					},
				},
				rooms: map[model.RoomID]model.Room{},
			}
			ctx := &Context{
				ActorID: "player:alice",
			}

			var cmd commandparse.Command
			cmd.Val[0] = 5
			resolved := ResolvedCommand{
				Parsed: cmd,
			}

			status, err := NewDMAddRomHandler(world)(ctx, resolved)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if tt.expectDeny {
				if status != StatusPrompt {
					t.Errorf("status = %v, want StatusPrompt", status)
				}
				if len(world.created) > 0 {
					t.Errorf("expected no room creation, but got %v", world.created)
				}
			} else {
				if status != StatusDefault {
					t.Errorf("status = %v, want StatusDefault", status)
				}
				if len(world.created) != 1 || world.created[0] != "room:00005" {
					t.Errorf("expected room creation of 'room:00005', got %v", world.created)
				}
			}
		})
	}
}

func TestDMAddRomValValidation(t *testing.T) {
	world := &mockDMAddRomWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:    "creature:alice",
				Stats: map[string]int{"class": 13},
			},
		},
		rooms: map[model.RoomID]model.Room{},
	}
	ctx := &Context{
		ActorID: "player:alice",
	}

	var cmd commandparse.Command
	cmd.Val[0] = 1
	resolved := ResolvedCommand{
		Parsed: cmd,
	}

	status, err := NewDMAddRomHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if status != StatusDefault {
		t.Errorf("status = %v, want StatusDefault", status)
	}
	if ctx.OutputString() != "무엇을 만들죠?\n" {
		t.Errorf("output = %q, want '무엇을 만들죠?\\n'", ctx.OutputString())
	}
	if len(world.created) > 0 {
		t.Errorf("expected no room creation, but got %v", world.created)
	}
}

func TestDMAddRomUsesLegacyValOneRoomNumber(t *testing.T) {
	world := &mockDMAddRomWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:    "creature:alice",
				Stats: map[string]int{"class": model.ClassDM},
			},
		},
		rooms: map[model.RoomID]model.Room{},
	}
	ctx := &Context{ActorID: "player:alice"}

	parsed := commandparse.Parse("room 105 *add")
	resolved := ResolvedCommand{
		Input:  "room 105 *add",
		Parsed: parsed,
		Args:   commandArgs(parsed),
		Values: commandValues(parsed),
	}

	status, err := NewDMAddRomHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if status != StatusDefault {
		t.Errorf("status = %v, want StatusDefault", status)
	}
	if ctx.OutputString() != "방번호 #105 만들었습니다.\n" {
		t.Errorf("output = %q, want room creation output", ctx.OutputString())
	}
	if len(world.created) != 1 || world.created[0] != "room:00105" {
		t.Errorf("expected room creation of 'room:00105', got %v", world.created)
	}
}

func TestDMAddRomRoomExists(t *testing.T) {
	world := &mockDMAddRomWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:    "creature:alice",
				Stats: map[string]int{"class": 13},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:00005": {ID: "room:00005"},
		},
	}
	ctx := &Context{
		ActorID: "player:alice",
	}

	var cmd commandparse.Command
	cmd.Val[0] = 5
	resolved := ResolvedCommand{
		Parsed: cmd,
	}

	status, err := NewDMAddRomHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if status != StatusDefault {
		t.Errorf("status = %v, want StatusDefault", status)
	}
	if ctx.OutputString() != "기존의 방이 존재합니다.\n" {
		t.Errorf("output = %q, want '기존의 방이 존재합니다.\\n'", ctx.OutputString())
	}
	if len(world.created) > 0 {
		t.Errorf("expected no room creation, but got %v", world.created)
	}
}

func TestDMAddRomCreateFail(t *testing.T) {
	world := &mockDMAddRomWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:    "creature:alice",
				Stats: map[string]int{"class": 13},
			},
		},
		rooms:      map[model.RoomID]model.Room{},
		failCreate: true,
	}
	ctx := &Context{
		ActorID: "player:alice",
	}

	var cmd commandparse.Command
	cmd.Val[0] = 5
	resolved := ResolvedCommand{
		Parsed: cmd,
	}

	status, err := NewDMAddRomHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if status != StatusDefault {
		t.Errorf("status = %v, want StatusDefault", status)
	}
	if ctx.OutputString() != "에러: Unable open files.\n" {
		t.Errorf("output = %q, want '에러: Unable open files.\\n'", ctx.OutputString())
	}
	if len(world.created) > 0 {
		t.Errorf("expected no room creation, but got %v", world.created)
	}
}

func TestDMAddRomCreateSuccess(t *testing.T) {
	world := &mockDMAddRomWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:    "creature:alice",
				Stats: map[string]int{"class": 13},
			},
		},
		rooms: map[model.RoomID]model.Room{},
	}
	ctx := &Context{
		ActorID: "player:alice",
	}

	var cmd commandparse.Command
	cmd.Val[0] = 5
	resolved := ResolvedCommand{
		Parsed: cmd,
	}

	status, err := NewDMAddRomHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if status != StatusDefault {
		t.Errorf("status = %v, want StatusDefault", status)
	}
	if ctx.OutputString() != "방번호 #5 만들었습니다.\n" {
		t.Errorf("output = %q, want '방번호 #5 만들었습니다.\\n'", ctx.OutputString())
	}
	if len(world.created) != 1 || world.created[0] != "room:00005" {
		t.Errorf("expected room creation of 'room:00005', got %v", world.created)
	}
}
