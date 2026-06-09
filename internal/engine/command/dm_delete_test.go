package command

import (
	"testing"

	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type mockDMDeleteWorld struct {
	players       map[model.PlayerID]model.Player
	creatures     map[model.CreatureID]model.Creature
	rooms         map[model.RoomID]model.Room
	updatedRoomID model.RoomID
	updatedFields map[string]string
}

func (w *mockDMDeleteWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMDeleteWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMDeleteWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *mockDMDeleteWorld) UpdateRoomDescription(id model.RoomID, field, val string) error {
	w.updatedRoomID = id
	w.updatedFields[field] = val
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

func TestDMDelete(t *testing.T) {
	tests := []struct {
		name          string
		actorID       string
		input         string
		players       map[model.PlayerID]model.Player
		creatures     map[model.CreatureID]model.Creature
		rooms         map[model.RoomID]model.Room
		wantStatus    Status
		wantOutput    string
		wantFields    map[string]string
		wantRoomID    model.RoomID
		checkRoomDesc bool
		wantShortDesc string
		wantLongDesc  string
	}{
		{
			name:    "permission denied (class < 13)",
			actorID: "player:alice",
			input:   "*delete foo",
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
			name:    "syntax error: no arguments",
			actorID: "player:alice",
			input:   "*delete",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantOutput: "syntax:*delete [-PESLA] <delete_word>\n",
		},
		{
			name:    "syntax error: flag too short",
			actorID: "player:alice",
			input:   "*delete -",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantOutput: "syntax:*delete [-PESLA] <delete_word>\n",
		},
		{
			name:    "syntax error: unrecognized flag",
			actorID: "player:alice",
			input:   "*delete -X word",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantOutput: "syntax:*delete [-PESLA] <delete_word>\n",
		},
		{
			name:    "syntax error: missing pattern for -P",
			actorID: "player:alice",
			input:   "*delete -P",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantOutput: "syntax:*delete [-PESLA] <delete_word>\n",
		},
		{
			name:    "pattern not found for -E without pattern like legacy",
			actorID: "player:alice",
			input:   "*delete -E",
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
			name:    "success: -S clears short description",
			actorID: "player:alice",
			input:   "*delete -S",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantFields: map[string]string{"short": ""},
			wantRoomID: "room:1",
		},
		{
			name:    "success: -L clears long description",
			actorID: "player:alice",
			input:   "*delete -L",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantFields: map[string]string{"long": ""},
			wantRoomID: "room:1",
		},
		{
			name:    "success: -A clears both descriptions",
			actorID: "player:alice",
			input:   "*delete -A",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantFields: map[string]string{"short": "", "long": ""},
			wantRoomID: "room:1",
		},
		{
			name:    "success: none deletes 1st occurrence in short description",
			actorID: "player:alice",
			input:   "*delete foo",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantFields:    map[string]string{"short": "a  room"},
			wantRoomID:    "room:1",
			checkRoomDesc: true,
			wantShortDesc: "a  room",
			wantLongDesc:  "very foo description",
		},
		{
			name:    "success: none deletes 1st occurrence in long description",
			actorID: "player:alice",
			input:   "*delete foo",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a room", LongDescription: "very foo description"},
			},
			wantFields:    map[string]string{"long": "very  description"},
			wantRoomID:    "room:1",
			checkRoomDesc: true,
			wantShortDesc: "a room",
			wantLongDesc:  "very  description",
		},
		{
			name:    "success: none deletes 2nd occurrence in short/long description",
			actorID: "player:alice",
			input:   "*delete foo 2",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantFields:    map[string]string{"long": "very  description"},
			wantRoomID:    "room:1",
			checkRoomDesc: true,
			wantShortDesc: "a foo room",
			wantLongDesc:  "very  description",
		},
		{
			name:    "success: -P deletes occurrence",
			actorID: "player:alice",
			input:   "*delete -P foo",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantFields:    map[string]string{"short": "a  room"},
			wantRoomID:    "room:1",
			checkRoomDesc: true,
			wantShortDesc: "a  room",
			wantLongDesc:  "very foo description",
		},
		{
			name:    "success: -D deletes occurrence",
			actorID: "player:alice",
			input:   "*delete -D foo",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantFields:    map[string]string{"short": "a  room"},
			wantRoomID:    "room:1",
			checkRoomDesc: true,
			wantShortDesc: "a  room",
			wantLongDesc:  "very foo description",
		},
		{
			name:    "success: -E deletes starting from occurrence in short description",
			actorID: "player:alice",
			input:   "*delete -E foo",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantFields:    map[string]string{"short": "a "},
			wantRoomID:    "room:1",
			checkRoomDesc: true,
			wantShortDesc: "a ",
			wantLongDesc:  "very foo description",
		},
		{
			name:    "success: -E deletes starting from occurrence in long description",
			actorID: "player:alice",
			input:   "*delete -E foo",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a room", LongDescription: "very foo description"},
			},
			wantFields:    map[string]string{"long": "very "},
			wantRoomID:    "room:1",
			checkRoomDesc: true,
			wantShortDesc: "a room",
			wantLongDesc:  "very ",
		},
		{
			name:    "success: -E deletes 2nd occurrence in long description",
			actorID: "player:alice",
			input:   "*delete -E 2 foo",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:1"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1", ShortDescription: "a foo room", LongDescription: "very foo description"},
			},
			wantFields:    map[string]string{"long": "very "},
			wantRoomID:    "room:1",
			checkRoomDesc: true,
			wantShortDesc: "a foo room",
			wantLongDesc:  "very ",
		},
		{
			name:    "pattern not found",
			actorID: "player:alice",
			input:   "*delete missing",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMDeleteWorld{
				players:       tt.players,
				creatures:     tt.creatures,
				rooms:         tt.rooms,
				updatedFields: make(map[string]string),
			}
			handler := NewDMDeleteHandler(world)

			ctx := &Context{
				ActorID: tt.actorID,
			}

			resolved := ResolvedCommand{
				Input: tt.input,
				Spec: commandspec.CommandSpec{
					Name: "dm_delete",
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

			if tt.wantFields != nil {
				for field, val := range tt.wantFields {
					gotVal, ok := world.updatedFields[field]
					if !ok {
						t.Errorf("expected field %q to be updated, but it was not", field)
					} else if gotVal != val {
						t.Errorf("field %q = %q, want %q", field, gotVal, val)
					}
				}
			}

			if tt.wantRoomID != "" && world.updatedRoomID != tt.wantRoomID {
				t.Errorf("updated roomID = %q, want %q", world.updatedRoomID, tt.wantRoomID)
			}

			if tt.checkRoomDesc {
				r := world.rooms[tt.wantRoomID]
				if r.ShortDescription != tt.wantShortDesc {
					t.Errorf("short description = %q, want %q", r.ShortDescription, tt.wantShortDesc)
				}
				if r.LongDescription != tt.wantLongDesc {
					t.Errorf("long description = %q, want %q", r.LongDescription, tt.wantLongDesc)
				}
			}
		})
	}
}
