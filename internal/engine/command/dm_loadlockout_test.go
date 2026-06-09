package command

import (
	"errors"
	"testing"

	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type mockDMLoadLockoutWorld struct {
	players    map[model.PlayerID]model.Player
	creatures  map[model.CreatureID]model.Creature
	loadErr    error
	loadCalled bool
}

func (w *mockDMLoadLockoutWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMLoadLockoutWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMLoadLockoutWorld) LoadLockouts() error {
	w.loadCalled = true
	return w.loadErr
}

func TestDMLoadLockout(t *testing.T) {
	tests := []struct {
		name         string
		actorID      string
		players      map[model.PlayerID]model.Player
		creatures    map[model.CreatureID]model.Creature
		loadErr      error
		wantStatus   Status
		wantErr      bool
		wantOutput   string
		expectCalled bool
	}{
		{
			name:       "nil ctx / empty actorID",
			actorID:    "",
			wantStatus: StatusPrompt,
		},
		{
			name:       "missing player and creature",
			actorID:    "player:missing",
			wantStatus: StatusPrompt,
		},
		{
			name:    "class below DM (caretaker 10)",
			actorID: "player:bob",
			players: map[model.PlayerID]model.Player{
				"player:bob": {ID: "player:bob", CreatureID: "creature:bob"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:bob": {ID: "creature:bob", Stats: map[string]int{"class": 10}},
			},
			wantStatus: StatusPrompt,
		},
		{
			name:    "class below DM (caretaker 12)",
			actorID: "player:bob",
			players: map[model.PlayerID]model.Player{
				"player:bob": {ID: "player:bob", CreatureID: "creature:bob"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:bob": {ID: "creature:bob", Stats: map[string]int{"class": 12}},
			},
			wantStatus: StatusPrompt,
		},
		{
			name:    "class DM load success (player actor)",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": 13}},
			},
			wantStatus:   StatusDefault,
			wantOutput:   "Lockout file read in.\n",
			expectCalled: true,
		},
		{
			name:    "class DM load success (creature actor directly)",
			actorID: "creature:dm",
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": 13}},
			},
			wantStatus:   StatusDefault,
			wantOutput:   "Lockout file read in.\n",
			expectCalled: true,
		},
		{
			name:    "class DM load error is hidden like legacy fopen failure",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": 14}},
			},
			loadErr:      errors.New("failed to load lockout file"),
			wantStatus:   StatusDefault,
			wantOutput:   "Lockout file read in.\n",
			expectCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMLoadLockoutWorld{
				players:   tt.players,
				creatures: tt.creatures,
				loadErr:   tt.loadErr,
			}
			ctx := &Context{
				ActorID: tt.actorID,
			}
			resolved := ResolvedCommand{
				Spec: commandspec.CommandSpec{
					Name: "dm_loadlockout",
				},
			}

			handler := NewDMLoadLockoutHandler(world)
			status, err := handler(ctx, resolved)

			if (err != nil) != tt.wantErr {
				t.Fatalf("handler() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if status != tt.wantStatus {
				t.Fatalf("status = %v, want %v", status, tt.wantStatus)
			}
			if tt.wantOutput != "" {
				gotOutput := ctx.OutputString()
				if gotOutput != tt.wantOutput {
					t.Fatalf("output = %q, want %q", gotOutput, tt.wantOutput)
				}
			}
			if world.loadCalled != tt.expectCalled {
				t.Fatalf("loadCalled = %v, want %v", world.loadCalled, tt.expectCalled)
			}
		})
	}
}

func TestDMLoadLockoutNilWorldReturnsPrompt(t *testing.T) {
	ctx := &Context{ActorID: "player:dm"}
	status, err := NewDMLoadLockoutHandler(nil)(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "dm_loadlockout"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusPrompt {
		t.Fatalf("status = %v, want StatusPrompt", status)
	}
	if got := ctx.OutputString(); got != "" {
		t.Fatalf("output = %q, want empty", got)
	}
}
