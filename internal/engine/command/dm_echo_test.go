package command

import (
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMEchoWorld struct {
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
}

func (w *mockDMEchoWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMEchoWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func TestDMEcho(t *testing.T) {
	tests := []struct {
		name              string
		actorID           string
		input             string
		args              []string
		players           map[model.PlayerID]model.Player
		creatures         map[model.CreatureID]model.Creature
		wantStatus        Status
		wantOutput        string
		wantBroadcast     string
		wantBroadcastRoom model.RoomID
	}{
		{
			name:       "empty actor ID",
			actorID:    "",
			wantStatus: StatusDefault,
		},
		{
			name:    "class below SUB_DM",
			actorID: "player:alice",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassBulsa}, RoomID: "room:1"},
			},
			wantStatus: StatusPrompt,
			wantOutput: "",
		},
		{
			name:    "SUB_DM class empty message",
			actorID: "player:alice",
			args:    []string{},
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}, RoomID: "room:1"},
			},
			wantStatus: StatusDefault,
			wantOutput: "무슨말을 방의 사람들에게 알리죠?",
		},
		{
			name:    "SUB_DM class success",
			actorID: "player:alice",
			args:    []string{"hello", "world"},
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}, RoomID: "room:1"},
			},
			wantStatus:        StatusDefault,
			wantBroadcast:     "\nhello world",
			wantBroadcastRoom: "room:1",
		},
		{
			name:    "SUB_DM class success from verb-final raw input",
			actorID: "player:alice",
			input:   "hello room *말",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}, RoomID: "room:1"},
			},
			wantStatus:        StatusDefault,
			wantBroadcast:     "\nhello room",
			wantBroadcastRoom: "room:1",
		},
		{
			name:    "verb-final raw input preserves legacy cut_command trailing spaces",
			actorID: "player:alice",
			input:   "hello room   *말",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}, RoomID: "room:1"},
			},
			wantStatus:        StatusDefault,
			wantBroadcast:     "\nhello room  ",
			wantBroadcastRoom: "room:1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMEchoWorld{
				players:   tt.players,
				creatures: tt.creatures,
			}
			handler := NewDMEchoHandler(world)

			var broadcastText string
			var broadcastRoomID model.RoomID
			var broadcastExclude string

			ctx := &Context{
				ActorID: tt.actorID,
				Values: map[string]any{
					ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
						broadcastText = text
						broadcastRoomID = roomID
						broadcastExclude = excludeSessionID
						return nil
					}),
				},
			}

			resolved := ResolvedCommand{
				Input:  tt.input,
				Parsed: commandparse.Parse(tt.input),
				Spec: commandspec.CommandSpec{
					Name: "*말",
				},
				Args: tt.args,
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

			if broadcastText != tt.wantBroadcast {
				t.Errorf("broadcast text = %q, want %q", broadcastText, tt.wantBroadcast)
			}

			if broadcastRoomID != tt.wantBroadcastRoom {
				t.Errorf("broadcast room = %q, want %q", broadcastRoomID, tt.wantBroadcastRoom)
			}

			if broadcastExclude != "" {
				t.Errorf("broadcast exclude = %q, want empty", broadcastExclude)
			}
		})
	}
}
