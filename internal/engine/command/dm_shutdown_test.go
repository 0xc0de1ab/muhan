package command

import (
	"errors"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type mockDMShutdownWorld struct {
	players         map[model.PlayerID]model.Player
	creatures       map[model.CreatureID]model.Creature
	shutdownSeconds int
	shutdownNow     bool
	shutdownCalled  bool
	shutdownErr     error
}

func (m *mockDMShutdownWorld) FlushActivePlayersAndBanks() error { return nil }
func (m *mockDMShutdownWorld) SavePlayer(model.PlayerID) error   { return nil }

func (m *mockDMShutdownWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMShutdownWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMShutdownWorld) SetShutdown(seconds int, now bool) error {
	m.shutdownSeconds = seconds
	m.shutdownNow = now
	m.shutdownCalled = true
	return m.shutdownErr
}

func TestDMShutdown(t *testing.T) {
	tests := []struct {
		name        string
		actorID     string
		class       int
		input       string
		shutdownErr error
		finalParse  bool
		wantStatus  Status
		wantOutput  string
		wantCalled  bool
		wantSeconds int
		wantNow     bool
	}{
		{
			name:       "unauthorized player class",
			actorID:    "player:alice",
			class:      12, // Sub-DM/Caretaker, below DM (13)
			input:      "*shutdown 10",
			wantStatus: StatusPrompt,
			wantOutput: "",
			wantCalled: false,
		},
		{
			name:        "authorized DM default shutdown",
			actorID:     "player:alice",
			class:       13, // DM
			input:       "*shutdown",
			wantOutput:  "Ok.\n",
			wantCalled:  true,
			wantSeconds: 61,
			wantNow:     false,
		},
		{
			name:        "authorized DM shutdown 10 minutes",
			actorID:     "player:alice",
			class:       13, // DM
			input:       "*shutdown 10",
			wantOutput:  "Ok.\n",
			wantCalled:  true,
			wantSeconds: 601,
			wantNow:     false,
		},
		{
			name:        "authorized DM shutdown 10 minutes then now slot",
			actorID:     "player:alice",
			class:       13, // DM
			input:       "*shutdown 10 now",
			wantOutput:  "Ok.\n",
			wantCalled:  true,
			wantSeconds: 1,
			wantNow:     true,
		},
		{
			name:        "authorized DM shutdown now first argument",
			actorID:     "player:alice",
			class:       13, // DM
			input:       "*shutdown now",
			wantOutput:  "Ok.\n",
			wantCalled:  true,
			wantSeconds: 1,
			wantNow:     true,
		},
		{
			name:        "authorized DM shutdown numeric suffix is string slot like C parser",
			actorID:     "player:alice",
			class:       13, // DM
			input:       "*shutdown 10분",
			wantOutput:  "Ok.\n",
			wantCalled:  true,
			wantSeconds: 61,
			wantNow:     false,
		},
		{
			name:        "authorized DM shutdown nonnumeric first argument uses default value slot",
			actorID:     "player:alice",
			class:       13, // DM
			input:       "*shutdown later",
			wantOutput:  "Ok.\n",
			wantCalled:  true,
			wantSeconds: 61,
			wantNow:     false,
		},
		{
			name:        "authorized DM shutdown hash delimiter then now slot",
			actorID:     "player:alice",
			class:       13, // DM
			input:       "*shutdown#10#now",
			wantOutput:  "Ok.\n",
			wantCalled:  true,
			wantSeconds: 1,
			wantNow:     true,
		},
		{
			name:        "authorized DM shutdown hash delimiter first now",
			actorID:     "player:alice",
			class:       13, // DM
			input:       "*shutdown#now",
			wantOutput:  "Ok.\n",
			wantCalled:  true,
			wantSeconds: 1,
			wantNow:     true,
		},
		{
			name:        "C-style final command uses first numeric value",
			actorID:     "player:alice",
			class:       13, // DM
			input:       "10 *shutdown",
			finalParse:  true,
			wantOutput:  "Ok.\n",
			wantCalled:  true,
			wantSeconds: 601,
			wantNow:     false,
		},
		{
			name:        "C-style final command nonnumeric suffix remains string",
			actorID:     "player:alice",
			class:       13, // DM
			input:       "10분 *shutdown",
			finalParse:  true,
			wantOutput:  "Ok.\n",
			wantCalled:  true,
			wantSeconds: 61,
			wantNow:     false,
		},
		{
			name:        "world SetShutdown returns error",
			actorID:     "player:alice",
			class:       14, // DM
			input:       "*shutdown 5",
			shutdownErr: errors.New("world error"),
			wantOutput:  "Ok.\n",
			wantCalled:  true,
			wantSeconds: 301,
			wantNow:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMShutdownWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {ID: "creature:alice", RoomID: "room:1", Stats: map[string]int{"class": tt.class}},
				},
				shutdownErr: tt.shutdownErr,
			}

			ctx := &Context{
				ActorID: tt.actorID,
			}
			parsed := commandparse.ParseCommandFirst(tt.input)
			if tt.finalParse {
				parsed = commandparse.Parse(tt.input)
			}
			resolved := ResolvedCommand{
				Input:  tt.input,
				Parsed: parsed,
				Spec: commandspec.CommandSpec{
					Name:       "*shutdown",
					Number:     114,
					Handler:    "dm_shutdown",
					Privileged: true,
				},
				Args: commandArgs(parsed),
			}

			handler := NewDMShutdownHandler(world)
			status, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			wantStatus := tt.wantStatus
			if wantStatus == 0 {
				wantStatus = StatusDefault
			}
			if status != wantStatus {
				t.Fatalf("status = %v, want %v", status, wantStatus)
			}

			gotOutput := ctx.OutputString()
			if gotOutput != tt.wantOutput {
				t.Fatalf("output = %q, want %q", gotOutput, tt.wantOutput)
			}

			if world.shutdownCalled != tt.wantCalled {
				t.Fatalf("shutdownCalled = %v, want %v", world.shutdownCalled, tt.wantCalled)
			}

			if tt.wantCalled {
				if world.shutdownSeconds != tt.wantSeconds {
					t.Fatalf("shutdownSeconds = %d, want %d", world.shutdownSeconds, tt.wantSeconds)
				}
				if world.shutdownNow != tt.wantNow {
					t.Fatalf("shutdownNow = %v, want %v", world.shutdownNow, tt.wantNow)
				}
			}
		})
	}
}

func TestDMShutdownUsesParsedNowSlotWithoutSyntheticArgs(t *testing.T) {
	world := &mockDMShutdownWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", RoomID: "room:1", Stats: map[string]int{"class": model.ClassDM}},
		},
	}
	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Input: "*shutdown now",
		Parsed: commandparse.Command{
			Num: 2,
			Str: [commandparse.CommandMax]string{"*shutdown", "now"},
			Val: [commandparse.CommandMax]int64{1, 1},
		},
		Spec: commandspec.CommandSpec{
			Name:       "*shutdown",
			Number:     114,
			Handler:    "dm_shutdown",
			Privileged: true,
		},
	}

	handler := NewDMShutdownHandler(world)
	status, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want %v", status, StatusDefault)
	}
	if world.shutdownSeconds != 1 {
		t.Fatalf("shutdownSeconds = %d, want 1", world.shutdownSeconds)
	}
	if !world.shutdownNow {
		t.Fatal("shutdownNow = false, want true")
	}
	if got := ctx.OutputString(); got != "Ok.\n" {
		t.Fatalf("output = %q, want %q", got, "Ok.\n")
	}
}
