package command

import (
	"errors"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMReloadRomWorld struct {
	players    map[model.PlayerID]model.Player
	creatures  map[model.CreatureID]model.Creature
	reloadErr  error
	reloadedID model.RoomID
}

func (w *mockDMReloadRomWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMReloadRomWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMReloadRomWorld) ReloadRoom(id model.RoomID) error {
	w.reloadedID = id
	return w.reloadErr
}

func TestDMReloadRom_Validation(t *testing.T) {
	tests := []struct {
		name          string
		actorID       string
		players       map[model.PlayerID]model.Player
		creatures     map[model.CreatureID]model.Creature
		wantStatus    Status
		wantErr       bool
		wantOutput    string
		expectReload  bool
		reloadRoomErr error
	}{
		{
			name:       "empty actor ID",
			actorID:    "",
			wantStatus: StatusPrompt,
		},
		{
			name:       "missing actor",
			actorID:    "player:alice",
			wantStatus: StatusPrompt,
		},
		{
			name:    "class below DM (player stats)",
			actorID: "player:alice",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}, RoomID: "room:1"},
			},
			wantStatus: StatusPrompt,
		},
		{
			name:    "class below DM (creature stats)",
			actorID: "creature:alice",
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}, RoomID: "room:1"},
			},
			wantStatus: StatusPrompt,
		},
		{
			name:    "class DM but no room ID",
			actorID: "player:alice",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}},
			},
			wantStatus: StatusDefault,
			wantOutput: "실패했습니다.\n",
		},
		{
			name:    "class DM reload success (player stats)",
			actorID: "player:alice",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			wantStatus:   StatusDefault,
			wantOutput:   "Ok.\n",
			expectReload: true,
		},
		{
			name:    "class DM reload falls back to player room",
			actorID: "player:alice",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:1"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}},
			},
			wantStatus:   StatusDefault,
			wantOutput:   "Ok.\n",
			expectReload: true,
		},
		{
			name:    "class DM reload success (properties class)",
			actorID: "player:alice",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Properties: map[string]string{"class": "14"}, RoomID: "room:1"},
			},
			wantStatus:   StatusDefault,
			wantOutput:   "Ok.\n",
			expectReload: true,
		},
		{
			name:    "class DM reload failure",
			actorID: "player:alice",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			wantStatus:    StatusDefault,
			wantOutput:    "실패했습니다.\n",
			expectReload:  true,
			reloadRoomErr: errors.New("reload failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMReloadRomWorld{
				players:   tt.players,
				creatures: tt.creatures,
				reloadErr: tt.reloadRoomErr,
			}
			ctx := &Context{
				ActorID: tt.actorID,
			}
			resolved := ResolvedCommand{
				Input: "*reload",
				Spec: commandspec.CommandSpec{
					Name:       "*reload",
					Number:     103,
					Handler:    "dm_reload_rom",
					Privileged: true,
				},
			}

			handler := NewDMReloadRomHandler(world)
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
			if tt.expectReload {
				expectedRoomID := model.RoomID("room:1")
				if world.reloadedID != expectedRoomID {
					t.Fatalf("reloaded room ID = %q, want %q", world.reloadedID, expectedRoomID)
				}
			}
		})
	}
}
