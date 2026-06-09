package command

import (
	"errors"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type mockDMBroadechoWorld struct {
	players          map[model.PlayerID]model.Player
	creatures        map[model.CreatureID]model.Creature
	broadcastMessage string
	broadcastError   error
}

func (w *mockDMBroadechoWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMBroadechoWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMBroadechoWorld) BroadcastAll(message string) error {
	w.broadcastMessage = message
	return w.broadcastError
}

func TestDMBroadecho(t *testing.T) {
	tests := []struct {
		name          string
		actorID       string
		input         string
		args          []string
		players       map[model.PlayerID]model.Player
		creatures     map[model.CreatureID]model.Creature
		broadcastErr  error
		wantStatus    Status
		wantOutput    string
		wantBroadcast string
	}{
		{
			name:       "empty actor ID",
			actorID:    "",
			wantStatus: StatusDefault,
		},
		{
			name:    "class below sub_dm (12)",
			actorID: "player:alice",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 9}},
			},
			wantStatus: StatusPrompt,
			wantOutput: "",
		},
		{
			name:    "sub_dm class empty message",
			actorID: "player:alice",
			args:    []string{},
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": legacyClassSubDM}},
			},
			wantStatus: StatusDefault,
			wantOutput: "무얼 방송합니까?\n",
		},
		{
			name:    "sub_dm class empty message with spaces",
			actorID: "player:alice",
			args:    []string{" ", "   "},
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": legacyClassSubDM}},
			},
			wantStatus: StatusDefault,
			wantOutput: "무얼 방송합니까?\n",
		},
		{
			name:    "sub_dm class success without -n",
			actorID: "player:alice",
			args:    []string{"hello", "world"},
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": legacyClassSubDM}},
			},
			wantStatus:    StatusDefault,
			wantBroadcast: "\n### hello world",
		},
		{
			name:    "sub_dm class success from verb-final raw input",
			actorID: "player:alice",
			input:   "hello   world *broad",
			args:    []string{"hello", "world"},
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": legacyClassSubDM}},
			},
			wantStatus:    StatusDefault,
			wantBroadcast: "\n### hello   world",
		},
		{
			name:    "verb-final raw input preserves legacy cut_command trailing spaces",
			actorID: "player:alice",
			input:   "hello   world   *broad",
			args:    []string{"hello", "world"},
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": legacyClassSubDM}},
			},
			wantStatus:    StatusDefault,
			wantBroadcast: "\n### hello   world  ",
		},
		{
			name:    "sub_dm class success with -n prefix",
			actorID: "player:alice",
			args:    []string{"-n", "hello", "world"},
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": legacyClassSubDM}},
			},
			wantStatus:    StatusDefault,
			wantBroadcast: "\nhello world",
		},
		{
			name:    "sub_dm class success with -n prefix and no other body",
			actorID: "player:alice",
			args:    []string{"-n"},
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": legacyClassSubDM}},
			},
			wantStatus: StatusDefault,
		},
		{
			name:    "sub_dm class ignores unknown dash prefix",
			actorID: "player:alice",
			args:    []string{"-x", "hello"},
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": legacyClassSubDM}},
			},
			wantStatus: StatusDefault,
		},
		{
			name:    "sub_dm class broadcast error is not user-visible",
			actorID: "player:alice",
			args:    []string{"hello", "world"},
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": legacyClassSubDM}},
			},
			broadcastErr:  errors.New("broadcast failed"),
			wantStatus:    StatusDefault,
			wantBroadcast: "\n### hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMBroadechoWorld{
				players:        tt.players,
				creatures:      tt.creatures,
				broadcastError: tt.broadcastErr,
			}
			handler := NewDMBroadechoHandler(world)

			ctx := &Context{
				ActorID: tt.actorID,
			}

			resolved := ResolvedCommand{
				Input:  tt.input,
				Parsed: commandparse.Parse(tt.input),
				Spec: commandspec.CommandSpec{
					Name: "dm_broadecho",
				},
				Args: tt.args,
			}

			status, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status != tt.wantStatus {
				t.Errorf("status = %v, want %v", status, tt.wantStatus)
			}

			gotOutput := ctx.OutputString()
			if gotOutput != tt.wantOutput {
				t.Errorf("output = %q, want %q", gotOutput, tt.wantOutput)
			}

			if world.broadcastMessage != tt.wantBroadcast {
				t.Errorf("broadcast message = %q, want %q", world.broadcastMessage, tt.wantBroadcast)
			}
		})
	}
}

func TestDMBroadechoDashNRespectsPNOBRDLikeLegacyBroadcast(t *testing.T) {
	world := &mockDMBroadechoWorld{
		players: map[model.PlayerID]model.Player{
			"player:subdm": {ID: "player:subdm", CreatureID: "creature:subdm"},
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:subdm": {ID: "creature:subdm", Stats: map[string]int{"class": legacyClassSubDM}},
			"creature:alice": {ID: "creature:alice", DisplayName: "Alice"},
			"creature:bob": {
				ID:          "creature:bob",
				DisplayName: "Bob",
				Metadata:    model.Metadata{Tags: []string{"PNOBRD"}},
			},
		},
	}
	writes := map[string]string{}
	ctx := &Context{
		ActorID: "player:subdm",
		Values: map[string]any{
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{
					{ID: "session:subdm", ActorID: "player:subdm"},
					{ID: "session:alice", ActorID: "player:alice"},
					{ID: "session:bob", ActorID: "player:bob"},
				}
			},
			"game.sendToSession": func(id string, cmd struct{ Write string }) error {
				writes[id] += cmd.Write
				return nil
			},
		},
	}

	resolved := ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "*broad"},
		Args: []string{"-n", "hello", "world"},
	}
	status, err := NewDMBroadechoHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if world.broadcastMessage != "" {
		t.Fatalf("BroadcastAll was used for -n path: %q", world.broadcastMessage)
	}
	want := "\nhello world"
	if writes["session:subdm"] != want {
		t.Fatalf("subdm write = %q, want %q", writes["session:subdm"], want)
	}
	if writes["session:alice"] != want {
		t.Fatalf("alice write = %q, want %q", writes["session:alice"], want)
	}
	if _, ok := writes["session:bob"]; ok {
		t.Fatalf("PNOBRD target received -n broadcast: %q", writes["session:bob"])
	}
}
