package command

import (
	"errors"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMNameroomWorld struct {
	players      map[model.PlayerID]model.Player
	creatures    map[model.CreatureID]model.Creature
	rooms        map[model.RoomID]model.Room
	setRoomID    model.RoomID
	setRoomName  string
	setRoomError error
}

func (w *mockDMNameroomWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMNameroomWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMNameroomWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *mockDMNameroomWorld) SetRoomName(roomID model.RoomID, name string) error {
	w.setRoomID = roomID
	w.setRoomName = name
	return w.setRoomError
}

func TestDMNameroom(t *testing.T) {
	tests := []struct {
		name         string
		actorID      string
		input        string
		parsed       commandparse.Command
		args         []string
		players      map[model.PlayerID]model.Player
		creatures    map[model.CreatureID]model.Creature
		rooms        map[model.RoomID]model.Room
		setRoomErr   error
		wantStatus   Status
		wantOutput   string
		wantRoomID   model.RoomID
		wantRoomName string
		wantErr      bool
	}{
		{
			name:       "nil context or empty actor ID",
			actorID:    "",
			wantStatus: StatusDefault,
		},
		{
			name:    "player not found",
			actorID: "player:unknown",
			players: map[model.PlayerID]model.Player{},
			creatures: map[model.CreatureID]model.Creature{
				"player:unknown": {ID: "player:unknown", Stats: map[string]int{"class": 13}},
			},
			wantStatus: StatusDefault,
		},
		{
			name:    "creature not found",
			actorID: "player:alice",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures:  map[model.CreatureID]model.Creature{},
			wantStatus: StatusDefault,
		},
		{
			name:    "class below DM (13)",
			actorID: "player:alice",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}},
			},
			wantStatus: StatusPrompt,
		},
		{
			name:    "room ID is zero",
			actorID: "player:alice",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: ""},
			},
			wantStatus: StatusDefault,
		},
		{
			name:    "room not found",
			actorID: "player:alice",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:10"},
			},
			rooms:      map[model.RoomID]model.Room{},
			wantStatus: StatusDefault,
		},
		{
			name:    "empty argument (no room name provided)",
			actorID: "player:alice",
			input:   "*name  ",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:10"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:10": {ID: "room:10", ShortDescription: "Original Room"},
			},
			wantStatus: StatusDefault,
			wantOutput: "무엇으로 이름을 바꿉니까?\n",
		},
		{
			name:    "argument too long (> 79 bytes in EUC-KR)",
			actorID: "player:alice",
			input:   "*name " + strings.Repeat("이", 40), // 40 * 2 = 80 bytes in EUC-KR
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:10"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:10": {ID: "room:10", ShortDescription: "Original Room"},
			},
			wantStatus: StatusDefault,
			wantOutput: "이름이 너무 깁니다.\n",
		},
		{
			name:    "argument exactly 79 bytes (allowed)",
			actorID: "player:alice",
			input:   "*name " + strings.Repeat("이", 39) + "a", // 39 * 2 + 1 = 79 bytes in EUC-KR
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:10"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:10": {ID: "room:10", ShortDescription: "Original Room"},
			},
			wantStatus:   StatusDefault,
			wantOutput:   "이름을 변경하였습니다.\n",
			wantRoomID:   "room:10",
			wantRoomName: strings.Repeat("이", 39) + "a",
		},
		{
			name:    "successful room name change",
			actorID: "player:alice",
			input:   "*nameroom New Room Name",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:10"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:10": {ID: "room:10", ShortDescription: "Original Room"},
			},
			wantStatus:   StatusDefault,
			wantOutput:   "이름을 변경하였습니다.\n",
			wantRoomID:   "room:10",
			wantRoomName: "New Room Name",
		},
		{
			name:    "successful verb-final room name change",
			actorID: "player:alice",
			input:   "Verb Final Room *name",
			parsed:  commandparse.Parse("Verb Final Room *name"),
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:10"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:10": {ID: "room:10", ShortDescription: "Original Room"},
			},
			wantStatus:   StatusDefault,
			wantOutput:   "이름을 변경하였습니다.\n",
			wantRoomID:   "room:10",
			wantRoomName: "Verb Final Room",
		},
		{
			name:    "successful verb-final room name preserves legacy cut_command trailing spaces",
			actorID: "player:alice",
			input:   "Verb Final Room   *name",
			parsed:  commandparse.Parse("Verb Final Room   *name"),
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:10"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:10": {ID: "room:10", ShortDescription: "Original Room"},
			},
			wantStatus:   StatusDefault,
			wantOutput:   "이름을 변경하였습니다.\n",
			wantRoomID:   "room:10",
			wantRoomName: "Verb Final Room  ",
		},
		{
			name:    "SetRoomName returns error",
			actorID: "player:alice",
			input:   "*name New Name",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:10"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:10": {ID: "room:10", ShortDescription: "Original Room"},
			},
			setRoomErr:   errors.New("db error"),
			wantStatus:   StatusDefault,
			wantRoomID:   "room:10",
			wantRoomName: "New Name",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMNameroomWorld{
				players:      tt.players,
				creatures:    tt.creatures,
				rooms:        tt.rooms,
				setRoomError: tt.setRoomErr,
			}
			handler := NewDMNameroomHandler(world)

			ctx := &Context{
				ActorID: tt.actorID,
			}

			parsed := tt.parsed
			if parsed.Num == 0 {
				parsed = commandparse.ParseCommandFirst(tt.input)
			}
			resolved := ResolvedCommand{
				Input:  tt.input,
				Parsed: parsed,
				Spec: commandspec.CommandSpec{
					Name: "dm_nameroom",
				},
				Args: tt.args,
			}

			status, err := handler(ctx, resolved)
			if (err != nil) != tt.wantErr {
				t.Fatalf("handler returned error %v, wantErr %v", err, tt.wantErr)
			}
			if status != tt.wantStatus {
				t.Errorf("status = %v, want %v", status, tt.wantStatus)
			}

			gotOutput := ctx.OutputString()
			if gotOutput != tt.wantOutput {
				t.Errorf("output = %q, want %q", gotOutput, tt.wantOutput)
			}

			if world.setRoomID != tt.wantRoomID {
				t.Errorf("setRoomID = %q, want %q", world.setRoomID, tt.wantRoomID)
			}
			if world.setRoomName != tt.wantRoomName {
				t.Errorf("setRoomName = %q, want %q", world.setRoomName, tt.wantRoomName)
			}
		})
	}
}
