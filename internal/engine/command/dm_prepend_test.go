package command

import (
	"errors"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMPrependWorld struct {
	players    map[model.PlayerID]model.Player
	creatures  map[model.CreatureID]model.Creature
	rooms      map[model.RoomID]model.Room
	updatedID  model.RoomID
	updatedFld string
	updatedVal string
	updateErr  error
}

func (w *mockDMPrependWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMPrependWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMPrependWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *mockDMPrependWorld) UpdateRoomDescription(roomID model.RoomID, field string, val string) error {
	w.updatedID = roomID
	w.updatedFld = field
	w.updatedVal = val
	return w.updateErr
}

func TestDMPrepend(t *testing.T) {
	tests := []struct {
		name             string
		actorClass       int
		input            string
		playerRoomID     model.RoomID
		zeroCreatureRoom bool
		roomShortDesc    string
		roomLongDesc     string
		updateErr        error
		wantStatus       Status
		wantOutput       string
		wantUpdated      bool
		wantField        string
		wantValue        string
		wantErr          bool
	}{
		{
			name:         "DM class 13 success, default long description with newline",
			actorClass:   13,
			input:        "*prepend Hello World",
			roomLongDesc: "Existing long desc",
			wantStatus:   StatusDefault,
			wantUpdated:  true,
			wantField:    "long",
			wantValue:    "Hello World\nExisting long desc",
		},
		{
			name:         "DM class 14 success, default long description with newline",
			actorClass:   14,
			input:        "*prepend Hello World",
			roomLongDesc: "Existing long desc",
			wantStatus:   StatusDefault,
			wantUpdated:  true,
			wantField:    "long",
			wantValue:    "Hello World\nExisting long desc",
		},
		{
			name:             "does not fallback to player room when creature room is empty",
			actorClass:       13,
			input:            "*prepend Hello World",
			playerRoomID:     "room:100",
			zeroCreatureRoom: true,
			roomLongDesc:     "Existing long desc",
			wantStatus:       StatusDefault,
			wantUpdated:      false,
		},
		{
			name:        "Class < 13 denied",
			actorClass:  12,
			input:       "*prepend Hello World",
			wantStatus:  StatusPrompt,
			wantUpdated: false,
		},
		{
			name:        "No arguments",
			actorClass:  13,
			input:       "*prepend",
			wantStatus:  StatusDefault,
			wantOutput:  "syntax: *prepend [-sn] <text>\n",
			wantUpdated: false,
		},
		{
			name:        "Only space",
			actorClass:  13,
			input:       "*prepend   ",
			wantStatus:  StatusDefault,
			wantOutput:  "syntax: *prepend [-sn] <text>\n",
			wantUpdated: false,
		},
		{
			name:        "Flags but no text 1",
			actorClass:  13,
			input:       "*prepend -s",
			wantStatus:  StatusDefault,
			wantOutput:  "syntax: *prepend [-sn] <text>\n",
			wantUpdated: false,
		},
		{
			name:        "Flags but no text 2",
			actorClass:  13,
			input:       "*prepend -sn",
			wantStatus:  StatusDefault,
			wantOutput:  "syntax: *prepend [-sn] <text>\n",
			wantUpdated: false,
		},
		{
			name:        "Flags but no text 3",
			actorClass:  13,
			input:       "*prepend -sn   ",
			wantStatus:  StatusDefault,
			wantOutput:  "syntax: *prepend [-sn] <text>\n",
			wantUpdated: false,
		},
		{
			name:          "Short description option -s",
			actorClass:    13,
			input:         "*prepend -s Short prepend",
			roomShortDesc: "Existing short desc",
			wantStatus:    StatusDefault,
			wantUpdated:   true,
			wantField:     "short",
			wantValue:     "Short prepend\nExisting short desc",
		},
		{
			name:         "No newline option -n",
			actorClass:   13,
			input:        "*prepend -n Nonewline",
			roomLongDesc: "Existing long desc",
			wantStatus:   StatusDefault,
			wantUpdated:  true,
			wantField:    "long",
			wantValue:    "NonewlineExisting long desc",
		},
		{
			name:          "Combined option -sn",
			actorClass:    13,
			input:         "*prepend -sn ShortNoNewline",
			roomShortDesc: "Existing short desc",
			wantStatus:    StatusDefault,
			wantUpdated:   true,
			wantField:     "short",
			wantValue:     "ShortNoNewlineExisting short desc",
		},
		{
			name:          "Combined option -ns",
			actorClass:    13,
			input:         "*prepend -ns ShortNoNewline",
			roomShortDesc: "Existing short desc",
			wantStatus:    StatusDefault,
			wantUpdated:   true,
			wantField:     "short",
			wantValue:     "ShortNoNewlineExisting short desc",
		},
		{
			name:         "Default long empty desc (automatically no newline)",
			actorClass:   13,
			input:        "*prepend Hello World",
			roomLongDesc: "",
			wantStatus:   StatusDefault,
			wantUpdated:  true,
			wantField:    "long",
			wantValue:    "Hello World",
		},
		{
			name:          "Short empty desc (automatically no newline)",
			actorClass:    13,
			input:         "*prepend -s Hello World",
			roomShortDesc: "",
			wantStatus:    StatusDefault,
			wantUpdated:   true,
			wantField:     "short",
			wantValue:     "Hello World",
		},
		{
			name:         "Update error",
			actorClass:   13,
			input:        "*prepend Hello World",
			roomLongDesc: "Existing",
			updateErr:    errors.New("db error"),
			wantStatus:   StatusDefault,
			wantUpdated:  true,
			wantField:    "long",
			wantValue:    "Hello World\nExisting",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creatureRoomID := model.RoomID("room:100")
			if tt.zeroCreatureRoom {
				creatureRoomID = ""
			}
			world := &mockDMPrependWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: tt.playerRoomID},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {
						ID:     "creature:alice",
						RoomID: creatureRoomID,
						Stats:  map[string]int{"class": tt.actorClass},
					},
				},
				rooms: map[model.RoomID]model.Room{
					"room:100": {
						ID:               "room:100",
						ShortDescription: tt.roomShortDesc,
						LongDescription:  tt.roomLongDesc,
					},
				},
				updateErr: tt.updateErr,
			}

			ctx := &Context{
				ActorID: "player:alice",
			}

			resolved := ResolvedCommand{
				Input: tt.input,
			}

			status, err := NewDMPrependHandler(world)(ctx, resolved)
			if (err != nil) != tt.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}

			if status != tt.wantStatus {
				t.Errorf("status = %v, want %v", status, tt.wantStatus)
			}

			if ctx.OutputString() != tt.wantOutput {
				t.Errorf("output = %q, want %q", ctx.OutputString(), tt.wantOutput)
			}

			if tt.wantUpdated {
				if world.updatedID != "room:100" {
					t.Errorf("updatedID = %q, want \"room:100\"", world.updatedID)
				}
				if world.updatedFld != tt.wantField {
					t.Errorf("updatedFld = %q, want %q", world.updatedFld, tt.wantField)
				}
				if world.updatedVal != tt.wantValue {
					t.Errorf("updatedVal = %q, want %q", world.updatedVal, tt.wantValue)
				}
			} else {
				if !world.updatedID.IsZero() {
					t.Errorf("expected no update, but room %q was updated", world.updatedID)
				}
			}
		})
	}
}
