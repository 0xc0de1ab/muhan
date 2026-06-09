package command

import (
	"errors"
	"testing"

	"muhan/internal/world/model"
)

type mockDMResaveWorld struct {
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
	resaved   model.RoomID
	resaveErr error
}

func (w *mockDMResaveWorld) Player(id model.PlayerID) (model.Player, bool) {
	player, ok := w.players[id]
	return player, ok
}

func (w *mockDMResaveWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	creature, ok := w.creatures[id]
	return creature, ok
}

func (w *mockDMResaveWorld) ResaveRoom(id model.RoomID) error {
	w.resaved = id
	return w.resaveErr
}

func TestDMResaveClassValidation(t *testing.T) {
	tests := []struct {
		name       string
		class      int
		expectDeny bool
		expectSave bool
	}{
		{
			name:       "DM class 13 allowed",
			class:      13,
			expectDeny: false,
			expectSave: true,
		},
		{
			name:       "DM class 14 allowed",
			class:      14,
			expectDeny: false,
			expectSave: true,
		},
		{
			name:       "Caretaker class 10 denied",
			class:      10,
			expectDeny: true,
			expectSave: false,
		},
		{
			name:       "Zone maker class 0 denied",
			class:      0,
			expectDeny: true,
			expectSave: false,
		},
		{
			name:       "Fighter class 1 denied",
			class:      1,
			expectDeny: true,
			expectSave: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMResaveWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {
						ID:     "creature:alice",
						RoomID: "room:100",
						Stats:  map[string]int{"class": tt.class},
					},
				},
			}
			ctx := &Context{
				ActorID: "player:alice",
			}

			status, err := NewDMResaveHandler(world)(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if tt.expectDeny {
				if status != StatusPrompt {
					t.Errorf("status = %v, want StatusPrompt", status)
				}
				if len(ctx.Output) > 0 {
					t.Errorf("expected no output, got %q", ctx.OutputString())
				}
				if world.resaved != "" {
					t.Errorf("ResaveRoom was called but expected deny")
				}
			} else {
				if status != StatusDefault {
					t.Errorf("status = %v, want StatusDefault", status)
				}
				if !tt.expectSave || world.resaved != "room:100" {
					t.Errorf("expected ResaveRoom to be called for room:100, got %q", world.resaved)
				}
			}
		})
	}
}

func TestDMResaveSaveResults(t *testing.T) {
	tests := []struct {
		name           string
		resaveErr      error
		expectedOutput string
	}{
		{
			name:           "save succeeds",
			resaveErr:      nil,
			expectedOutput: "Ok.\n",
		},
		{
			name:           "save fails",
			resaveErr:      errors.New("db error"),
			expectedOutput: "저장 실패.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMResaveWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {
						ID:     "creature:alice",
						RoomID: "room:200",
						Stats:  map[string]int{"class": 13},
					},
				},
				resaveErr: tt.resaveErr,
			}
			ctx := &Context{
				ActorID: "player:alice",
			}

			_, err := NewDMResaveHandler(world)(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if got := ctx.OutputString(); got != tt.expectedOutput {
				t.Errorf("output = %q, want %q", got, tt.expectedOutput)
			}
		})
	}
}

func TestDMResaveActorIDs(t *testing.T) {
	tests := []struct {
		name        string
		actorID     string
		world       *mockDMResaveWorld
		expectError bool
		expectDeny  bool
		expectSave  bool
	}{
		{
			name:    "actor ID is PlayerID",
			actorID: "player:alice",
			world: &mockDMResaveWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {
						ID:     "creature:alice",
						RoomID: "room:300",
						Stats:  map[string]int{"class": 13},
					},
				},
			},
			expectError: false,
			expectDeny:  false,
			expectSave:  true,
		},
		{
			name:    "actor ID is CreatureID",
			actorID: "creature:alice",
			world: &mockDMResaveWorld{
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {
						ID:     "creature:alice",
						RoomID: "room:300",
						Stats:  map[string]int{"class": 13},
					},
				},
			},
			expectError: false,
			expectDeny:  false,
			expectSave:  true,
		},
		{
			name:        "empty actor ID",
			actorID:     "",
			world:       &mockDMResaveWorld{},
			expectError: false,
			expectDeny:  true,
		},
		{
			name:        "missing actor in world",
			actorID:     "player:missing",
			world:       &mockDMResaveWorld{},
			expectError: false,
			expectDeny:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{
				ActorID: tt.actorID,
			}

			status, err := NewDMResaveHandler(tt.world)(ctx, ResolvedCommand{})
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.expectDeny {
				if status != StatusPrompt {
					t.Errorf("status = %v, want StatusPrompt", status)
				}
			} else {
				if status != StatusDefault {
					t.Errorf("status = %v, want StatusDefault", status)
				}
				if tt.expectSave && tt.world.resaved != "room:300" {
					t.Errorf("expected room:300 to be resaved, got %q", tt.world.resaved)
				}
			}
		})
	}
}

func TestDMResaveNoRoomPrintsLegacyFailure(t *testing.T) {
	world := &mockDMResaveWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:    "creature:alice",
				Stats: map[string]int{"class": 13},
			},
		},
	}
	ctx := &Context{ActorID: "player:alice"}

	status, err := NewDMResaveHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if world.resaved != "" {
		t.Fatalf("ResaveRoom called with %q, want no call without room", world.resaved)
	}
	if got := ctx.OutputString(); got != "저장 실패.\n" {
		t.Fatalf("output = %q, want legacy save failure", got)
	}
}
