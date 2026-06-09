package command

import (
	"testing"

	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type mockDMReplaceWorld struct {
	players       map[model.PlayerID]model.Player
	creatures     map[model.CreatureID]model.Creature
	rooms         map[model.RoomID]model.Room
	updatedRoomID model.RoomID
	updatedField  string
	updatedVal    string
}

func (w *mockDMReplaceWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMReplaceWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMReplaceWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *mockDMReplaceWorld) UpdateRoomDescription(id model.RoomID, field, val string) error {
	w.updatedRoomID = id
	w.updatedField = field
	w.updatedVal = val
	if r, ok := w.rooms[id]; ok {
		if field == "short" {
			r.ShortDescription = val
		} else if field == "long" {
			r.LongDescription = val
		}
		w.rooms[id] = r
	}
	return nil
}

func TestDMReplace(t *testing.T) {
	tests := []struct {
		name       string
		actorID    string
		input      string
		players    map[model.PlayerID]model.Player
		creatures  map[model.CreatureID]model.Creature
		rooms      map[model.RoomID]model.Room
		wantStatus Status
		wantOutput string
		wantField  string
		wantVal    string
		wantRoomID model.RoomID
	}{
		{
			name:    "permission denied (class < 13)",
			actorID: "player:alice",
			input:   "*replace foo bar",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantStatus: StatusPrompt,
			wantOutput: "",
		},
		{
			name:    "syntax error: missing pattern and replacement",
			actorID: "player:alice",
			input:   "*replace",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantOutput: "syntax:*replace <pattern> <replacement>\n",
		},
		{
			name:    "syntax error: missing replacement",
			actorID: "player:alice",
			input:   "*replace foo",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantOutput: "syntax:*replace <pattern> <replacement>\n",
		},
		{
			name:    "success: replace 1st occurrence in short description",
			actorID: "player:alice",
			input:   "*replace foo bar",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantField:  "short",
			wantVal:    "a bar room",
			wantRoomID: "room:1",
		},
		{
			name:    "success: replace 1st occurrence in long description",
			actorID: "player:alice",
			input:   "*replace foo bar",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a room", LongDescription: "very foo description"},
			},
			wantField:  "long",
			wantVal:    "very bar description",
			wantRoomID: "room:1",
		},
		{
			name:    "success: replace 2nd occurrence spanning short and long description",
			actorID: "player:alice",
			input:   "*replace foo 2 bar",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantField:  "long",
			wantVal:    "very bar description",
			wantRoomID: "room:1",
		},
		{
			name:    "pattern not found",
			actorID: "player:alice",
			input:   "*replace missing replacement",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantOutput: "Pattern not found.\n",
		},
		{
			name:    "val 0 parses as 1",
			actorID: "player:alice",
			input:   "*replace foo 0 bar",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantField:  "short",
			wantVal:    "a bar room",
			wantRoomID: "room:1",
		},
		{
			name:    "replacement string with spaces",
			actorID: "player:alice",
			input:   "*replace foo brand new bar",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantField:  "short",
			wantVal:    "a brand new bar room",
			wantRoomID: "room:1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMReplaceWorld{
				players:   tt.players,
				creatures: tt.creatures,
				rooms:     tt.rooms,
			}
			handler := NewDMReplaceHandler(world)

			ctx := &Context{
				ActorID: tt.actorID,
			}

			resolved := ResolvedCommand{
				Input: tt.input,
				Spec: commandspec.CommandSpec{
					Name: "dm_replace",
				},
			}

			status, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}
			wantStatus := tt.wantStatus
			if wantStatus == 0 {
				wantStatus = StatusDefault
			}
			if status != wantStatus {
				t.Errorf("status = %v, want %v", status, wantStatus)
			}

			gotOutput := ctx.OutputString()
			if gotOutput != tt.wantOutput {
				t.Errorf("output = %q, want %q", gotOutput, tt.wantOutput)
			}

			if world.updatedField != tt.wantField {
				t.Errorf("updated field = %q, want %q", world.updatedField, tt.wantField)
			}

			if world.updatedVal != tt.wantVal {
				t.Errorf("updated val = %q, want %q", world.updatedVal, tt.wantVal)
			}

			if world.updatedRoomID != tt.wantRoomID {
				t.Errorf("updated roomID = %q, want %q", world.updatedRoomID, tt.wantRoomID)
			}
		})
	}
}
