package command

import (
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMAppendWorld struct {
	players       map[model.PlayerID]model.Player
	creatures     map[model.CreatureID]model.Creature
	rooms         map[model.RoomID]model.Room
	updatedRoomID model.RoomID
	updatedField  string
	updatedVal    string
}

func (w *mockDMAppendWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMAppendWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMAppendWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *mockDMAppendWorld) UpdateRoomDescription(id model.RoomID, field, val string) error {
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

func TestDMAppend(t *testing.T) {
	tests := []struct {
		name       string
		actorID    string
		input      string
		parsed     commandparse.Command
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
			input:   "*append foo",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a short description", LongDescription: "a long description"},
			},
			wantStatus: StatusPrompt,
		},
		{
			name:    "syntax error: missing text",
			actorID: "player:alice",
			input:   "*append",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a short description", LongDescription: "a long description"},
			},
			wantOutput: "syntax: *append [-sn] <text>\n",
		},
		{
			name:    "syntax error: spaces only",
			actorID: "player:alice",
			input:   "*append    ",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a short description", LongDescription: "a long description"},
			},
			wantOutput: "syntax: *append [-sn] <text>\n",
		},
		{
			name:    "syntax error: short flag length check",
			actorID: "player:alice",
			input:   "*append -",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a short description", LongDescription: "a long description"},
			},
			wantOutput: "syntax: *append [-sn] <text>\n",
		},
		{
			name:    "syntax error: flag only",
			actorID: "player:alice",
			input:   "*append -sn",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a short description", LongDescription: "a long description"},
			},
			wantOutput: "syntax: *append [-sn] <text>\n",
		},
		{
			name:    "success: append to long description with newline",
			actorID: "player:alice",
			input:   "*append new tail info",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a short description", LongDescription: "a long description"},
			},
			wantField:  "long",
			wantVal:    "a long description\nnew tail info",
			wantRoomID: "room:1",
		},
		{
			name:    "success: verb-final append to long description",
			actorID: "player:alice",
			input:   "verb final tail *append",
			parsed:  commandparse.Parse("verb final tail *append"),
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a short description", LongDescription: "a long description"},
			},
			wantField:  "long",
			wantVal:    "a long description\nverb final tail",
			wantRoomID: "room:1",
		},
		{
			name:    "success: verb-final append preserves legacy cut_command trailing spaces",
			actorID: "player:alice",
			input:   "verb final tail   *append",
			parsed:  commandparse.Parse("verb final tail   *append"),
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a short description", LongDescription: "a long description"},
			},
			wantField:  "long",
			wantVal:    "a long description\nverb final tail  ",
			wantRoomID: "room:1",
		},
		{
			name:    "success: append to long description without newline (empty current long)",
			actorID: "player:alice",
			input:   "*append new tail info",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a short description", LongDescription: ""},
			},
			wantField:  "long",
			wantVal:    "new tail info",
			wantRoomID: "room:1",
		},
		{
			name:    "success: append to short description with -s option",
			actorID: "player:alice",
			input:   "*append -s short tail",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a short description", LongDescription: "a long description"},
			},
			wantField:  "short",
			wantVal:    "a short description\nshort tail",
			wantRoomID: "room:1",
		},
		{
			name:    "success: append to long description without newline with -n option",
			actorID: "player:alice",
			input:   "*append -n no newline",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a short description", LongDescription: "a long description"},
			},
			wantField:  "long",
			wantVal:    "a long descriptionno newline",
			wantRoomID: "room:1",
		},
		{
			name:    "success: append to short description without newline with -sn option",
			actorID: "player:alice",
			input:   "*append -sn short no newline",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a short description", LongDescription: "a long description"},
			},
			wantField:  "short",
			wantVal:    "a short descriptionshort no newline",
			wantRoomID: "room:1",
		},
		{
			name:    "success: append to short description without newline with -ns option",
			actorID: "player:alice",
			input:   "*append -ns short no newline",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a short description", LongDescription: "a long description"},
			},
			wantField:  "short",
			wantVal:    "a short descriptionshort no newline",
			wantRoomID: "room:1",
		},
		{
			name:    "success: verb-final append parses flags before command",
			actorID: "player:alice",
			input:   "-ns verb final short *append",
			parsed:  commandparse.Parse("-ns verb final short *append"),
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a short description", LongDescription: "a long description"},
			},
			wantField:  "short",
			wantVal:    "a short descriptionverb final short",
			wantRoomID: "room:1",
		},
		{
			name:    "success: multiple spaces with nnl false should skip excess spaces",
			actorID: "player:alice",
			input:   "*append -s   spaced text",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a short description", LongDescription: "a long description"},
			},
			wantField:  "short",
			wantVal:    "a short description\nspaced text",
			wantRoomID: "room:1",
		},
		{
			name:    "success: multiple spaces with nnl true should keep excess spaces",
			actorID: "player:alice",
			input:   "*append -sn   spaced text",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a short description", LongDescription: "a long description"},
			},
			wantField:  "short",
			wantVal:    "a short description  spaced text",
			wantRoomID: "room:1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMAppendWorld{
				players:   tt.players,
				creatures: tt.creatures,
				rooms:     tt.rooms,
			}
			handler := NewDMAppendHandler(world)

			ctx := &Context{
				ActorID: tt.actorID,
			}

			resolved := ResolvedCommand{
				Input:  tt.input,
				Parsed: tt.parsed,
				Spec: commandspec.CommandSpec{
					Name: "dm_append",
				},
			}

			status, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}
			if status != tt.wantStatus {
				t.Errorf("status = %v, want %v", status, tt.wantStatus)
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
