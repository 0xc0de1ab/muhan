package command

import (
	"errors"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/world/model"
)

type mockDMFlushsaveWorld struct {
	players          map[model.PlayerID]model.Player
	creatures        map[model.CreatureID]model.Creature
	resavedAll       bool
	resavePermOnly   bool
	resaveErr        error
	flushedDirty     bool
	flushDirtySince  int64
	flushDirtyErr    error
	flushAfterResave bool
}

func (w *mockDMFlushsaveWorld) Player(id model.PlayerID) (model.Player, bool) {
	player, ok := w.players[id]
	return player, ok
}

func (w *mockDMFlushsaveWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	creature, ok := w.creatures[id]
	return creature, ok
}

func (w *mockDMFlushsaveWorld) ResaveAllRooms(permOnly bool) error {
	w.resavedAll = true
	w.resavePermOnly = permOnly
	return w.resaveErr
}

func (w *mockDMFlushsaveWorld) FlushDirtyBoardsAndFamilyNews(since int64) error {
	w.flushedDirty = true
	w.flushDirtySince = since
	w.flushAfterResave = w.resavedAll
	return w.flushDirtyErr
}

func TestDMFlushsaveClassValidation(t *testing.T) {
	tests := []struct {
		name       string
		class      int
		expectDeny bool
		expectCall bool
	}{
		{
			name:       "DM class 13 allowed",
			class:      13,
			expectDeny: false,
			expectCall: true,
		},
		{
			name:       "DM class 14 allowed",
			class:      14,
			expectDeny: false,
			expectCall: true,
		},
		{
			name:       "Caretaker class 10 denied",
			class:      10,
			expectDeny: true,
			expectCall: false,
		},
		{
			name:       "Zone maker class 0 denied",
			class:      0,
			expectDeny: true,
			expectCall: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMFlushsaveWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {
						ID:    "creature:alice",
						Stats: map[string]int{"class": tt.class},
					},
				},
			}
			ctx := &Context{
				ActorID: "player:alice",
			}

			status, err := NewDMFlushsaveHandler(world)(ctx, ResolvedCommand{})
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
				if world.resavedAll {
					t.Errorf("ResaveAllRooms was called but expected deny")
				}
				if world.flushedDirty {
					t.Errorf("FlushDirtyBoardsAndFamilyNews was called but expected deny")
				}
			} else {
				if status != StatusDefault {
					t.Errorf("status = %v, want StatusDefault", status)
				}
				if !world.resavedAll {
					t.Errorf("expected ResaveAllRooms to be called")
				}
				if !world.flushedDirty {
					t.Errorf("expected FlushDirtyBoardsAndFamilyNews to be called")
				}
			}
		})
	}
}

func TestDMFlushsaveArguments(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedOutput string
		expectPermOnly bool
	}{
		{
			name:           "no arguments",
			args:           nil,
			expectedOutput: "All rooms and contents flushed to disk.\n",
			expectPermOnly: false,
		},
		{
			name:           "with argument",
			args:           []string{"perm"},
			expectedOutput: "All rooms and PERM contents flushed to disk.\n",
			expectPermOnly: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMFlushsaveWorld{
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
			ctx := &Context{
				ActorID: "player:alice",
			}

			status, err := NewDMFlushsaveHandler(world)(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			if status != StatusDefault {
				t.Errorf("status = %v, want StatusDefault", status)
			}

			if got := ctx.OutputString(); got != tt.expectedOutput {
				t.Errorf("output = %q, want %q", got, tt.expectedOutput)
			}

			if !world.resavedAll {
				t.Errorf("expected ResaveAllRooms to be called")
			}

			if world.resavePermOnly != tt.expectPermOnly {
				t.Errorf("resavePermOnly = %v, want %v", world.resavePermOnly, tt.expectPermOnly)
			}
			if !world.flushedDirty {
				t.Errorf("expected FlushDirtyBoardsAndFamilyNews to be called")
			}
			if world.flushDirtySince != 0 {
				t.Errorf("flushDirtySince = %d, want 0", world.flushDirtySince)
			}
			if !world.flushAfterResave {
				t.Errorf("FlushDirtyBoardsAndFamilyNews ran before ResaveAllRooms")
			}
		})
	}
}

func TestDMFlushsaveUsesParsedArityWithoutSyntheticArgs(t *testing.T) {
	world := &mockDMFlushsaveWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:    "creature:alice",
				Stats: map[string]int{"class": legacyClassDM},
			},
		},
	}
	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Input: "*flushrooms perm",
		Parsed: commandparse.Command{
			Num: 2,
			Str: [commandparse.CommandMax]string{"*flushrooms", "perm"},
		},
	}

	status, err := NewDMFlushsaveHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want %v", status, StatusDefault)
	}
	if got := ctx.OutputString(); got != "All rooms and PERM contents flushed to disk.\n" {
		t.Fatalf("output = %q", got)
	}
	if !world.resavedAll || !world.resavePermOnly {
		t.Fatalf("resave flags = all:%v perm:%v, want all:true perm:true", world.resavedAll, world.resavePermOnly)
	}
}

func TestDMFlushsaveIgnoresRoomSaveErrorLikeLegacyVoidResaveAll(t *testing.T) {
	resaveErr := errors.New("database write failed")
	world := &mockDMFlushsaveWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:    "creature:alice",
				Stats: map[string]int{"class": 13},
			},
		},
		resaveErr: resaveErr,
	}
	ctx := &Context{
		ActorID: "player:alice",
	}

	status, err := NewDMFlushsaveHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "All rooms and contents flushed to disk.\n" {
		t.Fatalf("output = %q, want legacy success message", got)
	}
	if !world.flushedDirty {
		t.Fatal("FlushDirtyBoardsAndFamilyNews was not called after legacy room save error")
	}
	if !world.flushAfterResave {
		t.Fatal("FlushDirtyBoardsAndFamilyNews ran before ResaveAllRooms")
	}
}

func TestDMFlushsaveDirtyBoardFlushError(t *testing.T) {
	expectedErr := errors.New("board write failed")
	world := &mockDMFlushsaveWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:    "creature:alice",
				Stats: map[string]int{"class": 13},
			},
		},
		flushDirtyErr: expectedErr,
	}
	ctx := &Context{
		ActorID: "player:alice",
	}

	_, err := NewDMFlushsaveHandler(world)(ctx, ResolvedCommand{})
	if err != expectedErr {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
	if !world.resavedAll {
		t.Fatal("ResaveAllRooms was not called before dirty board flush")
	}
	if !world.flushedDirty {
		t.Fatal("FlushDirtyBoardsAndFamilyNews was not called")
	}
}
