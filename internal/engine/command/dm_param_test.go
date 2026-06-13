package command

import (
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMParamWorld struct {
	players               map[model.PlayerID]model.Player
	creatures             map[model.CreatureID]model.Creature
	wanderInterval        int
	shutdownTimeRemaining int64
	shipSailingInterval   int64
	timeToSail            int64
	forceShipSailCalled   bool
}

func (m *mockDMParamWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMParamWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMParamWorld) SetWanderInterval(val int) {
	m.wanderInterval = val
}

func (m *mockDMParamWorld) WanderInterval() int {
	return m.wanderInterval
}

func (m *mockDMParamWorld) ShutdownTimeRemaining() int64 {
	return m.shutdownTimeRemaining
}

func (m *mockDMParamWorld) ShipSailingInterval() int64 {
	return m.shipSailingInterval
}

func (m *mockDMParamWorld) TimeToSail() int64 {
	return m.timeToSail
}

func (m *mockDMParamWorld) ForceShipSail() {
	m.forceShipSailCalled = true
}

func (m *mockDMParamWorld) SetShipSailingInterval(val int64) {
	m.shipSailingInterval = val
}

func TestDMParam(t *testing.T) {
	tests := []struct {
		name        string
		actorID     string
		class       int
		parsedNum   int
		parsedStr1  string
		args        []string
		parsedVal   [commandparse.CommandMax]int64
		setupWorld  func(*mockDMParamWorld)
		wantStatus  Status
		wantOutput  string
		verifyWorld func(*testing.T, *mockDMParamWorld)
	}{
		{
			name:       "unauthorized class",
			actorID:    "player:alice",
			class:      12, // caretaker
			wantStatus: StatusPrompt,
			wantOutput: "",
		},
		{
			name:       "insufficient arguments - parsed num too low",
			actorID:    "player:alice",
			class:      13, // DM
			parsedNum:  1,
			args:       []string{},
			wantStatus: StatusDefault,
			wantOutput: "Set what parameter?\n",
		},
		{
			name:       "parsed arg slot empty",
			actorID:    "player:alice",
			class:      13, // DM
			parsedNum:  2,
			args:       []string{},
			wantStatus: StatusDefault,
			wantOutput: "Invalid parameter.\n",
		},
		{
			name:       "empty first argument",
			actorID:    "player:alice",
			class:      13,
			parsedNum:  2,
			args:       []string{""},
			wantStatus: StatusDefault,
			wantOutput: "Invalid parameter.\n",
		},
		{
			name:      "update wander interval case-insensitive 'r'",
			actorID:   "player:alice",
			class:     13,
			parsedNum: 2,
			args:      []string{"r"},
			parsedVal: [commandparse.CommandMax]int64{1, 45},
			setupWorld: func(w *mockDMParamWorld) {
				w.wanderInterval = 10
			},
			wantStatus: StatusPrompt,
			verifyWorld: func(t *testing.T, w *mockDMParamWorld) {
				if w.wanderInterval != 45 {
					t.Errorf("expected wander interval to be 45, got %d", w.wanderInterval)
				}
			},
		},
		{
			name:      "update wander interval case-insensitive 'R'",
			actorID:   "player:alice",
			class:     13,
			parsedNum: 2,
			args:      []string{"R"},
			parsedVal: [commandparse.CommandMax]int64{1, 99},
			setupWorld: func(w *mockDMParamWorld) {
				w.wanderInterval = 10
			},
			wantStatus: StatusPrompt,
			verifyWorld: func(t *testing.T, w *mockDMParamWorld) {
				if w.wanderInterval != 99 {
					t.Errorf("expected wander interval to be 99, got %d", w.wanderInterval)
				}
			},
		},
		{
			name:      "display status information 'd'",
			actorID:   "player:alice",
			class:     13,
			parsedNum: 2,
			args:      []string{"d"},
			setupWorld: func(w *mockDMParamWorld) {
				w.wanderInterval = 15
				w.shutdownTimeRemaining = 3600
				w.shipSailingInterval = 1200
				w.timeToSail = 600
			},
			wantStatus: StatusPrompt,
			wantOutput: "Random Update: 15\nTime to next shutdown: 3600\nShip sailing interval 1200\nTime to Sail: 600\n",
		},
		{
			name:       "display status from parsed slot without args",
			actorID:    "player:alice",
			class:      13,
			parsedNum:  2,
			parsedStr1: "d",
			setupWorld: func(w *mockDMParamWorld) {
				w.wanderInterval = 25
				w.shutdownTimeRemaining = 2400
				w.shipSailingInterval = 900
				w.timeToSail = 300
			},
			wantStatus: StatusPrompt,
			wantOutput: "Random Update: 25\nTime to next shutdown: 2400\nShip sailing interval 900\nTime to Sail: 300\n",
		},
		{
			name:      "display status information 'D'",
			actorID:   "player:alice",
			class:     13,
			parsedNum: 2,
			args:      []string{"D"},
			setupWorld: func(w *mockDMParamWorld) {
				w.wanderInterval = 10
				w.shutdownTimeRemaining = 120
				w.shipSailingInterval = 300
				w.timeToSail = 45
			},
			wantStatus: StatusPrompt,
			wantOutput: "Random Update: 10\nTime to next shutdown: 120\nShip sailing interval 300\nTime to Sail: 45\n",
		},
		{
			name:      "ship sailing trigger immediately 's' 1",
			actorID:   "player:alice",
			class:     13,
			parsedNum: 2,
			args:      []string{"s"},
			parsedVal: [commandparse.CommandMax]int64{1, 1},
			setupWorld: func(w *mockDMParamWorld) {
				w.forceShipSailCalled = false
			},
			wantStatus: StatusPrompt,
			verifyWorld: func(t *testing.T, w *mockDMParamWorld) {
				if !w.forceShipSailCalled {
					t.Errorf("expected ForceShipSail to be called")
				}
			},
		},
		{
			name:      "ship sailing trigger immediately 'S' 1",
			actorID:   "player:alice",
			class:     13,
			parsedNum: 2,
			args:      []string{"S"},
			parsedVal: [commandparse.CommandMax]int64{1, 1},
			setupWorld: func(w *mockDMParamWorld) {
				w.forceShipSailCalled = false
			},
			wantStatus: StatusPrompt,
			verifyWorld: func(t *testing.T, w *mockDMParamWorld) {
				if !w.forceShipSailCalled {
					t.Errorf("expected ForceShipSail to be called")
				}
			},
		},
		{
			name:      "ship sailing set interval 's' 500",
			actorID:   "player:alice",
			class:     13,
			parsedNum: 2,
			args:      []string{"s"},
			parsedVal: [commandparse.CommandMax]int64{1, 500},
			setupWorld: func(w *mockDMParamWorld) {
				w.shipSailingInterval = 100
			},
			wantStatus: StatusPrompt,
			verifyWorld: func(t *testing.T, w *mockDMParamWorld) {
				if w.shipSailingInterval != 500 {
					t.Errorf("expected ship sailing interval to be 500, got %d", w.shipSailingInterval)
				}
				if w.forceShipSailCalled {
					t.Errorf("ForceShipSail should not be called when value is not 1")
				}
			},
		},
		{
			name:       "invalid parameter action",
			actorID:    "player:alice",
			class:      13,
			parsedNum:  2,
			args:       []string{"x"},
			wantStatus: StatusDefault,
			wantOutput: "Invalid parameter.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &mockDMParamWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": tt.class}},
				},
			}
			if tt.setupWorld != nil {
				tt.setupWorld(w)
			}

			ctx := &Context{
				ActorID: tt.actorID,
			}

			parsedCmd := commandparse.Command{
				Num: tt.parsedNum,
				Val: tt.parsedVal,
			}
			if tt.parsedStr1 != "" {
				parsedCmd.Str[1] = tt.parsedStr1
			}
			if len(tt.args) > 0 {
				parsedCmd.Str[1] = tt.args[0]
			}

			resolved := ResolvedCommand{
				Parsed: parsedCmd,
				Spec: commandspec.CommandSpec{
					Name:       "dm_param",
					Handler:    "dm_param",
					Privileged: true,
				},
				Args: tt.args,
			}

			handler := NewDMParamHandler(w)
			status, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}

			if status != tt.wantStatus {
				t.Errorf("status = %v, want %v", status, tt.wantStatus)
			}

			gotOutput := ctx.OutputString()
			if gotOutput != tt.wantOutput {
				t.Errorf("output = %q, want %q", gotOutput, tt.wantOutput)
			}

			if tt.verifyWorld != nil {
				tt.verifyWorld(t, w)
			}
		})
	}
}
