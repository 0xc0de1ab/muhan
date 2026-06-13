package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMForceWorld struct {
	players            map[model.PlayerID]model.Player
	creatures          map[model.CreatureID]model.Creature
	forcedCommands     map[model.PlayerID][]string
	findPlayerByName   func(name string) (model.Player, bool)
	forcePlayerCommand func(playerID model.PlayerID, cmd string) error
	canForce           map[model.PlayerID]bool
}

func (w *mockDMForceWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMForceWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMForceWorld) FindPlayerByName(name string) (model.Player, bool) {
	if w.findPlayerByName != nil {
		return w.findPlayerByName(name)
	}
	for _, player := range w.players {
		if strings.EqualFold(player.DisplayName, name) ||
			strings.EqualFold(string(player.ID), name) ||
			strings.EqualFold(strings.TrimPrefix(string(player.ID), "player:"), name) {
			return player, true
		}
	}
	return model.Player{}, false
}

func (w *mockDMForceWorld) ForcePlayerCommand(playerID model.PlayerID, cmd string) error {
	if w.forcePlayerCommand != nil {
		return w.forcePlayerCommand(playerID, cmd)
	}
	if w.forcedCommands == nil {
		w.forcedCommands = make(map[model.PlayerID][]string)
	}
	w.forcedCommands[playerID] = append(w.forcedCommands[playerID], cmd)
	return nil
}

func (w *mockDMForceWorld) CanForcePlayerCommand(playerID model.PlayerID) bool {
	if w.canForce == nil {
		return true
	}
	canForce, ok := w.canForce[playerID]
	return ok && canForce
}

func TestDMForce(t *testing.T) {
	tests := []struct {
		name             string
		actorID          string
		args             []string
		numTokens        int
		inputLine        string
		players          map[model.PlayerID]model.Player
		creatures        map[model.CreatureID]model.Creature
		activeSessions   []testActiveSession
		canForce         map[model.PlayerID]bool
		wantStatus       Status
		wantOutput       string
		wantForcedCmd    string
		wantForcedTarget model.PlayerID
	}{
		{
			name:       "empty actor ID",
			actorID:    "",
			wantStatus: StatusDefault,
		},
		{
			name:    "caster below SUB_DM (class 9)",
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
			name:    "caster below SUB_DM (class 11)",
			actorID: "player:alice",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassBulsa}},
			},
			wantStatus: StatusPrompt,
			wantOutput: "",
		},
		{
			name:    "missing arguments",
			actorID: "player:alice",
			args:    []string{},
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}},
			},
			wantStatus: StatusPrompt,
		},
		{
			name:      "target player not found",
			actorID:   "player:alice",
			args:      []string{"cHARLIE"},
			numTokens: 2,
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}},
			},
			wantStatus: StatusDefault,
			wantOutput: "Charlie가 없습니다.\n",
		},
		{
			name:      "DM protection: caster is Sub-DM, target is DM",
			actorID:   "player:alice",
			args:      []string{"bob"},
			numTokens: 2,
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}},
				"creature:bob":   {ID: "creature:bob", Stats: map[string]int{"class": 13}},
			},
			activeSessions: []testActiveSession{
				{ID: "sess2", ActorID: "player:bob"},
			},
			wantStatus: StatusPrompt,
		},
		{
			name:      "C protects exact DM class only",
			actorID:   "player:alice",
			args:      []string{"bob", "say", "hello"},
			numTokens: 4,
			inputLine: "*force bob say hello",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}},
				"creature:bob":   {ID: "creature:bob", Stats: map[string]int{"class": model.ClassDM + 1}},
			},
			activeSessions: []testActiveSession{
				{ID: "sess1", ActorID: "player:alice"},
				{ID: "sess2", ActorID: "player:bob"},
			},
			wantStatus:       StatusPrompt,
			wantForcedTarget: "player:bob",
			wantForcedCmd:    "say hello",
		},
		{
			name:      "saved target without active session is not found",
			actorID:   "player:alice",
			args:      []string{"bob"},
			numTokens: 2,
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}},
				"creature:bob":   {ID: "creature:bob", Stats: map[string]int{"class": 1}},
			},
			activeSessions: []testActiveSession{
				{ID: "sess1", ActorID: "player:alice"},
			},
			wantStatus: StatusDefault,
			wantOutput: "Bob가 없습니다.\n",
		},
		{
			name:      "target pending prompt cannot be forced like C command-state check",
			actorID:   "player:alice",
			args:      []string{"bob", "say", "hello"},
			numTokens: 4,
			inputLine: "*force bob say hello",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}},
				"creature:bob":   {ID: "creature:bob", Stats: map[string]int{"class": 1}},
			},
			activeSessions: []testActiveSession{
				{ID: "sess1", ActorID: "player:alice"},
				{ID: "sess2", ActorID: "player:bob"},
			},
			canForce:   map[model.PlayerID]bool{"player:bob": false},
			wantStatus: StatusDefault,
			wantOutput: "Bob를 현재 강요할수 없습니다.\n",
		},
		{
			name:      "success forcing command (Sub-DM forces player)",
			actorID:   "player:alice",
			args:      []string{"bob", "say", "hello"},
			numTokens: 4,
			inputLine: "*force bob say hello",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}},
				"creature:bob":   {ID: "creature:bob", Stats: map[string]int{"class": 1}},
			},
			activeSessions: []testActiveSession{
				{ID: "sess1", ActorID: "player:alice"},
				{ID: "sess2", ActorID: "player:bob"},
			},
			wantStatus:       StatusPrompt,
			wantForcedTarget: "player:bob",
			wantForcedCmd:    "say hello",
		},
		{
			name:      "success forcing command (DM forces DM)",
			actorID:   "player:alice",
			args:      []string{"bob", "say", "hello"},
			numTokens: 4,
			inputLine: "*force bob say hello",
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}},
				"creature:bob":   {ID: "creature:bob", Stats: map[string]int{"class": 13}},
			},
			activeSessions: []testActiveSession{
				{ID: "sess1", ActorID: "player:alice"},
				{ID: "sess2", ActorID: "player:bob"},
			},
			wantStatus:       StatusPrompt,
			wantForcedTarget: "player:bob",
			wantForcedCmd:    "say hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMForceWorld{
				players:   tt.players,
				creatures: tt.creatures,
				canForce:  tt.canForce,
			}
			handler := NewDMForceHandler(world)

			ctx := &Context{
				ActorID: tt.actorID,
				Values: map[string]any{
					"game.activeSessions": func() []testActiveSession {
						return tt.activeSessions
					},
				},
			}

			// Build ResolvedCommand
			var parsed commandparse.Command
			parsed.Num = tt.numTokens
			if len(tt.args) > 0 {
				parsed.Str[0] = "dm_force"
				for i, arg := range tt.args {
					if i+1 < len(parsed.Str) {
						parsed.Str[i+1] = arg
					}
				}
			}
			resolved := ResolvedCommand{
				Input:  tt.inputLine,
				Parsed: parsed,
				Spec: commandspec.CommandSpec{
					Name: "dm_force",
				},
				Args: tt.args,
			}

			status, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("handler returned unexpected error: %v", err)
			}
			if status != tt.wantStatus {
				t.Errorf("status = %v, want %v", status, tt.wantStatus)
			}

			gotOutput := ctx.OutputString()
			if gotOutput != tt.wantOutput {
				t.Errorf("output = %q, want %q", gotOutput, tt.wantOutput)
			}

			if tt.wantForcedTarget != "" {
				cmds := world.forcedCommands[tt.wantForcedTarget]
				if len(cmds) != 1 {
					t.Fatalf("expected 1 forced command for %s, got %d", tt.wantForcedTarget, len(cmds))
				}
				if cmds[0] != tt.wantForcedCmd {
					t.Errorf("forced command = %q, want %q", cmds[0], tt.wantForcedCmd)
				}
			} else if len(world.forcedCommands) > 0 {
				t.Errorf("expected no forced commands, but got some: %v", world.forcedCommands)
			}
		})
	}
}

func TestDMForceExtractsVerbFinalForcedCommand(t *testing.T) {
	world := &mockDMForceWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob", DisplayName: "Bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}},
			"creature:bob":   {ID: "creature:bob", Stats: map[string]int{"class": model.ClassFighter}},
		},
	}
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{
					{ID: "sess1", ActorID: "player:alice"},
					{ID: "sess2", ActorID: "player:bob"},
				}
			},
		},
	}

	input := "bob   say hello *force"
	parsed := commandparse.Parse(input)
	resolved := ResolvedCommand{
		Input:  input,
		Parsed: parsed,
		Spec:   commandspec.CommandSpec{Name: "*force", Handler: "dm_force", Privileged: true},
		Args:   commandArgs(parsed),
		Values: commandValues(parsed),
	}

	status, err := NewDMForceHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	if status != StatusPrompt {
		t.Fatalf("status = %v, want StatusPrompt", status)
	}
	cmds := world.forcedCommands["player:bob"]
	if len(cmds) != 1 || cmds[0] != "say hello" {
		t.Fatalf("forced commands = %v, want [say hello]", cmds)
	}
}

func TestDMForceUsesParsedTargetSlotLikeCWhenArgsMissing(t *testing.T) {
	world := &mockDMForceWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob", DisplayName: "Bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}},
			"creature:bob":   {ID: "creature:bob", Stats: map[string]int{"class": model.ClassFighter}},
		},
	}
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{
					{ID: "sess1", ActorID: "player:alice"},
					{ID: "sess2", ActorID: "player:bob"},
				}
			},
		},
	}

	var parsed commandparse.Command
	parsed.Num = 4
	parsed.Str[0] = "*force"
	parsed.Str[1] = "bOB"
	parsed.Str[2] = "say"
	parsed.Str[3] = "hello"
	resolved := ResolvedCommand{
		Input:  "bOB say hello *force",
		Parsed: parsed,
		Spec:   commandspec.CommandSpec{Name: "*force", Handler: "dm_force", Privileged: true},
	}

	status, err := NewDMForceHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	if status != StatusPrompt {
		t.Fatalf("status = %v, want StatusPrompt", status)
	}
	cmds := world.forcedCommands["player:bob"]
	if len(cmds) != 1 || cmds[0] != "say hello" {
		t.Fatalf("forced commands = %v, want [say hello]", cmds)
	}
}
