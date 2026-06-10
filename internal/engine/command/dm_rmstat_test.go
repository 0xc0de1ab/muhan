package command

import (
	"testing"

	"muhan/internal/world/model"
)

type dmRmstatTestWorld struct {
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
}

func (w dmRmstatTestWorld) Player(id model.PlayerID) (model.Player, bool) {
	player, ok := w.players[id]
	return player, ok
}

func (w dmRmstatTestWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	creature, ok := w.creatures[id]
	return creature, ok
}

func TestDMRmstatRejectsUnauthorized(t *testing.T) {
	tests := []struct {
		name  string
		class int
	}{
		{name: "fighter", class: 1},
		{name: "thief", class: 2},
		{name: "assassin", class: 3},
		{name: "invalid class 9", class: 9},
		{name: "caretaker below SUB_DM", class: model.ClassCaretaker},
		{name: "bulsa below SUB_DM", class: model.ClassBulsa},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := dmRmstatTestWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": tt.class}},
				},
			}

			handler := NewDMRmstatHandler(world)
			ctx := &Context{ActorID: "player:alice"}

			status, err := handler(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusPrompt {
				t.Fatalf("status = %v, want StatusPrompt", status)
			}

			output := ctx.OutputString()
			if output != "" {
				t.Fatalf("output = %q, want no permission output", output)
			}
		})
	}
}

func TestDMRmstatAcceptsAuthorized(t *testing.T) {
	tests := []struct {
		name       string
		class      int
		roomID     model.RoomID
		wantOutput string
	}{
		{
			name:       "zonemaker",
			class:      0,
			roomID:     "room:00123",
			wantOutput: "방번호 #123\n",
		},
		{
			name:       "sub_dm",
			class:      model.ClassSubDM,
			roomID:     "room:00001",
			wantOutput: "방번호 #1\n",
		},
		{
			name:       "dm class 15",
			class:      15,
			roomID:     "room:01001",
			wantOutput: "방번호 #1001\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := dmRmstatTestWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: tt.roomID},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {
						ID:     "creature:alice",
						Stats:  map[string]int{"class": tt.class},
						RoomID: tt.roomID,
					},
				},
			}

			handler := NewDMRmstatHandler(world)
			ctx := &Context{ActorID: "player:alice"}

			status, err := handler(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %v, want StatusDefault", status)
			}

			output := ctx.OutputString()
			if output != tt.wantOutput {
				t.Fatalf("output = %q, want %q", output, tt.wantOutput)
			}
		})
	}
}

func TestDMRmstatClassNormalizesStatAndPropertyKeys(t *testing.T) {
	if got := dmRmstatClass(model.Creature{Stats: map[string]int{"CLA-SS": model.ClassSubDM}}); got != model.ClassSubDM {
		t.Fatalf("dmRmstatClass(normalized stat) = %d, want %d", got, model.ClassSubDM)
	}
	if got := dmRmstatClass(model.Creature{Properties: map[string]string{"cla ss": "12"}}); got != model.ClassSubDM {
		t.Fatalf("dmRmstatClass(normalized property) = %d, want %d", got, model.ClassSubDM)
	}
}

func TestDMRmstatRejectsPlayerWithoutCreature(t *testing.T) {
	world := dmRmstatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:orphan": {ID: "player:orphan", RoomID: "room:00123"},
		},
	}
	handler := NewDMRmstatHandler(world)
	ctx := &Context{ActorID: "player:orphan"}

	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "" {
		t.Fatalf("output = %q, want no orphan-player room leak", got)
	}
}

func TestDMRmstatMissingActor(t *testing.T) {
	world := dmRmstatTestWorld{}
	handler := NewDMRmstatHandler(world)

	// Nil context
	status, err := handler(nil, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler(nil) error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}

	// Empty actor ID
	ctx := &Context{ActorID: ""}
	status, err = handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler(empty actor) error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
}
