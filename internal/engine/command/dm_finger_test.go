package command

import (
	"errors"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMFingerWorld struct {
	players       map[model.PlayerID]model.Player
	creatures     map[model.CreatureID]model.Creature
	playersByName map[string]model.Player
	fingerCalls   []fingerCall
	fingerErr     error
	fingerOutput  string
}

type fingerCall struct {
	addr string
	name string
}

func (w *mockDMFingerWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMFingerWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMFingerWorld) FindPlayerByName(name string) (model.Player, bool) {
	p, ok := w.playersByName[name]
	return p, ok
}

func (w *mockDMFingerWorld) Finger(addr, name string) (string, error) {
	w.fingerCalls = append(w.fingerCalls, fingerCall{addr: addr, name: name})
	return w.fingerOutput, w.fingerErr
}

func TestDMFinger(t *testing.T) {
	errDummy := errors.New("finger error")

	tests := []struct {
		name            string
		actorID         string
		args            []string
		players         map[model.PlayerID]model.Player
		creatures       map[model.CreatureID]model.Creature
		playersByName   map[string]model.Player
		activeSessions  []testActiveSession
		fingerErr       error
		fingerOutput    string
		wantStatus      Status
		wantErr         error
		wantOutput      string
		wantFingerCalls []fingerCall
	}{
		{
			name:       "nil context or actor ID empty",
			actorID:    "",
			wantStatus: StatusDefault,
		},
		{
			name:    "permission denied (class below SUB_DM)",
			actorID: "player:charlie",
			players: map[model.PlayerID]model.Player{
				"player:charlie": {ID: "player:charlie", CreatureID: "creature:charlie"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:charlie": {ID: "creature:charlie", Stats: map[string]int{"class": 9}},
			},
			wantStatus: StatusPrompt,
		},
		{
			name:    "permission granted, missing arguments",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": model.ClassSubDM}},
			},
			args:       []string{},
			wantStatus: StatusDefault,
			wantOutput: "누구를 Finger검색 합니까?\n",
		},
		{
			name:    "address with @ prefix, no optional name",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": model.ClassSubDM}},
			},
			args:         []string{"@1.2.3.4"},
			fingerOutput: "finger result\n",
			wantStatus:   StatusDefault,
			wantOutput:   "Forking to 1.2.3.4.\nOutput will arrive shortly.\nfinger result\n",
			wantFingerCalls: []fingerCall{
				{addr: "1.2.3.4", name: ""},
			},
		},
		{
			name:    "address with @ prefix, with optional name",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": model.ClassSubDM}},
			},
			args:       []string{"@1.2.3.4", "bob"},
			wantStatus: StatusDefault,
			wantOutput: "Forking to 1.2.3.4.\nOutput will arrive shortly.\n",
			wantFingerCalls: []fingerCall{
				{addr: "1.2.3.4", name: "bob"},
			},
		},
		{
			name:    "address with @ prefix ignores optional name when extra args are present",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": model.ClassSubDM}},
			},
			args:       []string{"@1.2.3.4", "bob", "extra"},
			wantStatus: StatusDefault,
			wantOutput: "Forking to 1.2.3.4.\nOutput will arrive shortly.\n",
			wantFingerCalls: []fingerCall{
				{addr: "1.2.3.4", name: ""},
			},
		},
		{
			name:    "lookup online player by name: not found",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": model.ClassSubDM}},
			},
			args:       []string{"bob"},
			wantStatus: StatusDefault,
			wantOutput: "완전한 이름을 사용하세요\n",
		},
		{
			name:    "lookup saved player without active session: not found",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm":     {ID: "player:dm", CreatureID: "creature:dm"},
				"player:target": {ID: "player:target", DisplayName: "target", CreatureID: "creature:target"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm":     {ID: "creature:dm", Stats: map[string]int{"class": model.ClassSubDM}},
				"creature:target": {ID: "creature:target", DisplayName: "target", Properties: map[string]string{"address": "8.8.8.8"}},
			},
			activeSessions: []testActiveSession{
				{ID: "session:dm", ActorID: "player:dm"},
			},
			args:       []string{"target"},
			wantStatus: StatusDefault,
			wantOutput: "완전한 이름을 사용하세요\n",
		},
		{
			name:    "lookup online player by name: found, property address",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm":      {ID: "player:dm", CreatureID: "creature:dm"},
				"player:target1": {ID: "player:target1", DisplayName: "target1", CreatureID: "creature:target1"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm":      {ID: "creature:dm", Stats: map[string]int{"class": model.ClassSubDM}},
				"creature:target1": {ID: "creature:target1", DisplayName: "target1", Properties: map[string]string{"address": "8.8.8.8"}},
			},
			activeSessions: []testActiveSession{
				{ID: "session:target1", ActorID: "player:target1"},
			},
			args:       []string{"target1"},
			wantStatus: StatusDefault,
			wantOutput: "Forking to 8.8.8.8.\nOutput will arrive shortly.\n",
			wantFingerCalls: []fingerCall{
				{addr: "8.8.8.8", name: ""},
			},
		},
		{
			name:    "lookup online player by name: found, metadata address",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
				"player:target2": {
					ID:          "player:target2",
					DisplayName: "target2",
					CreatureID:  "creature:target2",
					Metadata: model.Metadata{
						RawFields: map[string][]byte{"address": []byte("9.9.9.9")},
					},
				},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm":      {ID: "creature:dm", Stats: map[string]int{"class": model.ClassSubDM}},
				"creature:target2": {ID: "creature:target2", DisplayName: "target2"},
			},
			activeSessions: []testActiveSession{
				{ID: "session:target2", ActorID: "player:target2"},
			},
			args:       []string{"target2"},
			wantStatus: StatusDefault,
			wantOutput: "Forking to 9.9.9.9.\nOutput will arrive shortly.\n",
			wantFingerCalls: []fingerCall{
				{addr: "9.9.9.9", name: ""},
			},
		},
		{
			name:    "lookup online player by name: found, default/placeholder address",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm":      {ID: "player:dm", CreatureID: "creature:dm"},
				"player:target3": {ID: "player:target3", DisplayName: "target3", CreatureID: "creature:target3"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm":      {ID: "creature:dm", Stats: map[string]int{"class": 13}}, // DM
				"creature:target3": {ID: "creature:target3", DisplayName: "target3"},
			},
			activeSessions: []testActiveSession{
				{ID: "session:target3", ActorID: "player:target3"},
			},
			args:       []string{"target3", "extra_arg"},
			wantStatus: StatusDefault,
			wantOutput: "Forking to 127.0.0.1.\nOutput will arrive shortly.\n",
			wantFingerCalls: []fingerCall{
				{addr: "127.0.0.1", name: "extra_arg"},
			},
		},
		{
			name:    "world.Finger returns error",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": model.ClassSubDM}},
			},
			args:            []string{"@1.1.1.1"},
			fingerErr:       errDummy,
			wantStatus:      StatusDefault,
			wantErr:         errDummy,
			wantOutput:      "Forking to 1.1.1.1.\nOutput will arrive shortly.\n",
			wantFingerCalls: []fingerCall{{addr: "1.1.1.1", name: ""}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMFingerWorld{
				players:       tt.players,
				creatures:     tt.creatures,
				playersByName: tt.playersByName,
				fingerErr:     tt.fingerErr,
				fingerOutput:  tt.fingerOutput,
			}

			handler := NewDMFingerHandler(world)

			ctx := &Context{
				ActorID: tt.actorID,
			}
			if tt.activeSessions != nil {
				ctx.Values = map[string]any{
					"game.activeSessions": func() []testActiveSession {
						return tt.activeSessions
					},
				}
			}

			resolved := ResolvedCommand{
				Spec: commandspec.CommandSpec{
					Name: "dm_finger",
				},
				Args: tt.args,
			}

			status, err := handler(ctx, resolved)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got err: %v, want %v", err, tt.wantErr)
			}
			if status != tt.wantStatus {
				t.Errorf("got status: %v, want %v", status, tt.wantStatus)
			}

			gotOutput := ctx.OutputString()
			if gotOutput != tt.wantOutput {
				t.Errorf("got output: %q, want %q", gotOutput, tt.wantOutput)
			}

			if len(world.fingerCalls) != len(tt.wantFingerCalls) {
				t.Fatalf("got %d finger calls, want %d", len(world.fingerCalls), len(tt.wantFingerCalls))
			}

			for i := range world.fingerCalls {
				if world.fingerCalls[i].addr != tt.wantFingerCalls[i].addr {
					t.Errorf("call[%d] addr: %q, want %q", i, world.fingerCalls[i].addr, tt.wantFingerCalls[i].addr)
				}
				if world.fingerCalls[i].name != tt.wantFingerCalls[i].name {
					t.Errorf("call[%d] name: %q, want %q", i, world.fingerCalls[i].name, tt.wantFingerCalls[i].name)
				}
			}
		})
	}
}

func TestDMFingerUsesParsedSlotsLikeCWhenArgsMissing(t *testing.T) {
	world := &mockDMFingerWorld{
		players: map[model.PlayerID]model.Player{
			"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": model.ClassSubDM}},
		},
	}
	ctx := &Context{ActorID: "player:dm"}
	resolved := ResolvedCommand{
		Input:  "@1.2.3.4 bob *finger",
		Parsed: commandparse.Command{Num: 3, Str: [7]string{"*finger", "@1.2.3.4", "bob"}},
		Spec:   commandspec.CommandSpec{Name: "*finger", Handler: "dm_finger", Privileged: true},
	}

	status, err := NewDMFingerHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if got, want := ctx.OutputString(), "Forking to 1.2.3.4.\nOutput will arrive shortly.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if len(world.fingerCalls) != 1 || world.fingerCalls[0] != (fingerCall{addr: "1.2.3.4", name: "bob"}) {
		t.Fatalf("finger calls = %+v, want address/name from parsed slots", world.fingerCalls)
	}
}
