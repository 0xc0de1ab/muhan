package command

import (
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockListActWorld struct {
	players         map[model.PlayerID]model.Player
	creatures       map[model.CreatureID]model.Creature
	activeCreatures []model.Creature
}

func (m *mockListActWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockListActWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockListActWorld) ActiveCreatures() []model.Creature {
	return m.activeCreatures
}

func TestListAct_Handler(t *testing.T) {
	t.Run("unauthorized caster class", func(t *testing.T) {
		world := &mockListActWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID: "creature:caster",
					Stats: map[string]int{
						"class": 12, // Below DM (13)
					},
				},
			},
		}

		ctx := &Context{
			ActorID: "player:caster",
		}

		resolved := ResolvedCommand{
			Args: []string{},
			Spec: commandspec.CommandSpec{
				Name:    "*active",
				Handler: "list_act",
			},
		}

		handler := NewListActHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}

		got := ctx.OutputString()
		if got != "" {
			t.Errorf("output = %q, want no permission output", got)
		}
	})

	t.Run("authorized caster - empty active creatures", func(t *testing.T) {
		world := &mockListActWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID: "creature:caster",
					Stats: map[string]int{
						"class": 13, // DM
					},
				},
			},
			activeCreatures: []model.Creature{},
		}

		ctx := &Context{
			ActorID: "player:caster",
		}

		resolved := ResolvedCommand{
			Args: []string{},
			Spec: commandspec.CommandSpec{
				Name:    "*active",
				Handler: "list_act",
			},
		}

		handler := NewListActHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := ctx.OutputString()
		want := "현재 활동중인 괴물\n\n이름:\n"
		if got != want {
			t.Errorf("expected output %q, got %q", want, got)
		}
	})

	t.Run("authorized caster - with active creatures", func(t *testing.T) {
		world := &mockListActWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID: "creature:caster",
					Stats: map[string]int{
						"class": 13, // DM
					},
				},
			},
			activeCreatures: []model.Creature{
				{
					ID:          "creature:monster1",
					DisplayName: "오크",
				},
				{
					ID:          "creature:monster2",
					DisplayName: "고블린",
				},
			},
		}

		ctx := &Context{
			ActorID: "player:caster",
		}

		resolved := ResolvedCommand{
			Args: []string{},
			Spec: commandspec.CommandSpec{
				Name:    "*active",
				Handler: "list_act",
			},
		}

		handler := NewListActHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := ctx.OutputString()
		want := "현재 활동중인 괴물\n\n이름:\n   오크.\n   고블린.\n"
		if got != want {
			t.Errorf("expected output %q, got %q", want, got)
		}
	})
}
