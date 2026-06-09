package command

import (
	"errors"
	"reflect"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/world/model"
)

type mockDMListWorld struct {
	players    map[model.PlayerID]model.Player
	creatures  map[model.CreatureID]model.Creature
	listErr    error
	listOutput string
	listArgs   []string
}

func (w *mockDMListWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMListWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMListWorld) List(args []string) (string, error) {
	w.listArgs = args
	return w.listOutput, w.listErr
}

func TestDMList(t *testing.T) {
	tests := []struct {
		name         string
		actorID      string
		parsedNum    int
		args         []string
		players      map[model.PlayerID]model.Player
		creatures    map[model.CreatureID]model.Creature
		listErr      error
		listOutput   string
		wantStatus   Status
		wantErr      bool
		wantOutput   string
		wantListArgs []string
	}{
		{
			name:       "nil or empty actor id",
			actorID:    "",
			wantStatus: StatusDefault,
		},
		{
			name:    "permission denied (class below SUB_DM)",
			actorID: "player:bob",
			players: map[model.PlayerID]model.Player{
				"player:bob": {ID: "player:bob", CreatureID: "creature:bob"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:bob": {ID: "creature:bob", Stats: map[string]int{"class": 9}},
			},
			wantStatus: StatusPrompt,
		},
		{
			name:    "argument count validation failure (Parsed.Num < 2)",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": legacyClassSubDM}},
			},
			parsedNum:  1,
			wantStatus: StatusDefault,
			wantOutput: "무엇의 리스트를 봅니까?\n",
		},
		{
			name:    "successful list execution",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": 15}},
			},
			parsedNum:    2,
			args:         []string{"monster"},
			listOutput:   "monster list\n",
			wantStatus:   StatusDefault,
			wantOutput:   "monster list\n",
			wantListArgs: []string{"monster"},
		},
		{
			name:    "list execution with multiple arguments",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": legacyClassSubDM}},
			},
			parsedNum:    3,
			args:         []string{"monster", "lvl5"},
			listOutput:   "monster lvl5 list\n",
			wantStatus:   StatusDefault,
			wantOutput:   "monster lvl5 list\n",
			wantListArgs: []string{"monster", "lvl5"},
		},
		{
			name:    "list passes at most four arguments",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": legacyClassSubDM}},
			},
			parsedNum:    6,
			args:         []string{"monster", "lvl5", "zone1", "rare", "ignored"},
			listOutput:   "capped list\n",
			wantStatus:   StatusDefault,
			wantOutput:   "capped list\n",
			wantListArgs: []string{"monster", "lvl5", "zone1", "rare"},
		},
		{
			name:    "list returns error",
			actorID: "player:dm",
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": legacyClassSubDM}},
			},
			parsedNum:    2,
			args:         []string{"invalid"},
			listErr:      errors.New("invalid list command"),
			wantStatus:   StatusDefault,
			wantErr:      true,
			wantListArgs: []string{"invalid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMListWorld{
				players:    tt.players,
				creatures:  tt.creatures,
				listErr:    tt.listErr,
				listOutput: tt.listOutput,
			}
			handler := NewDMListHandler(world)

			ctx := &Context{
				ActorID: tt.actorID,
			}

			resolved := ResolvedCommand{
				Parsed: commandparse.Command{
					Num: tt.parsedNum,
				},
				Args: tt.args,
			}

			status, err := handler(ctx, resolved)
			if (err != nil) != tt.wantErr {
				t.Fatalf("handler returned error: %v, wantErr: %v", err, tt.wantErr)
			}
			if status != tt.wantStatus {
				t.Errorf("status = %v, want %v", status, tt.wantStatus)
			}

			gotOutput := ctx.OutputString()
			if gotOutput != tt.wantOutput {
				t.Errorf("output = %q, want %q", gotOutput, tt.wantOutput)
			}

			if !reflect.DeepEqual(world.listArgs, tt.wantListArgs) {
				t.Errorf("list args = %v, want %v", world.listArgs, tt.wantListArgs)
			}
		})
	}
}

func TestDMListUsesParsedSlotsWithoutSyntheticArgs(t *testing.T) {
	world := &mockDMListWorld{
		players: map[model.PlayerID]model.Player{
			"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": legacyClassSubDM}},
		},
		listOutput: "parsed list\n",
	}
	ctx := &Context{ActorID: "player:dm"}
	resolved := ResolvedCommand{
		Input: "*list monster lvl5 zone1 rare ignored",
		Parsed: commandparse.Command{
			Num: 6,
			Str: [commandparse.CommandMax]string{"*list", "monster", "lvl5", "zone1", "rare", "ignored"},
		},
	}

	status, err := NewDMListHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want %v", status, StatusDefault)
	}
	wantArgs := []string{"monster", "lvl5", "zone1", "rare"}
	if !reflect.DeepEqual(world.listArgs, wantArgs) {
		t.Fatalf("list args = %v, want %v", world.listArgs, wantArgs)
	}
	if got := ctx.OutputString(); got != "parsed list\n" {
		t.Fatalf("output = %q", got)
	}
}
