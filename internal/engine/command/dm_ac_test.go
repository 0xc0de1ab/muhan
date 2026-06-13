package command

import (
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMAcWorld struct {
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
	statSets  map[model.CreatureID]map[string]int
}

func (m *mockDMAcWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMAcWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMAcWorld) SetCreatureStat(id model.CreatureID, name string, val int) error {
	if m.statSets == nil {
		m.statSets = make(map[model.CreatureID]map[string]int)
	}
	if _, ok := m.statSets[id]; !ok {
		m.statSets[id] = make(map[string]int)
	}
	m.statSets[id][name] = val

	if c, ok := m.creatures[id]; ok {
		if c.Stats == nil {
			c.Stats = make(map[string]int)
		}
		c.Stats[name] = val
		m.creatures[id] = c
	}
	return nil
}

func TestDMAc_Handler(t *testing.T) {
	t.Run("unauthorized caster class", func(t *testing.T) {
		world := &mockDMAcWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID: "creature:caster",
					Stats: map[string]int{
						"class": model.ClassBulsa,
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
				Name:    "*ac",
				Handler: "dm_ac",
			},
		}

		handler := NewDMAcHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}

		got := ctx.OutputString()
		if got != "" {
			t.Errorf("output = %q, want no permission output", got)
		}
	})

	t.Run("no target specified - restores hp/mp and prints caster stats", func(t *testing.T) {
		world := &mockDMAcWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID: "creature:caster",
					Stats: map[string]int{
						"class":     model.ClassSubDM,
						"hpMax":     100,
						"hpCurrent": 50,
						"mpMax":     50,
						"mpCurrent": 10,
						"armor":     40,
						"thaco":     15,
					},
				},
			},
		}

		ctx := &Context{
			ActorID: "player:caster",
		}

		resolved := ResolvedCommand{
			Args: []string{},
		}

		handler := NewDMAcHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Check stats are restored
		caster, ok := world.Creature("creature:caster")
		if !ok {
			t.Fatalf("caster creature not found")
		}
		if caster.Stats["hpCurrent"] != 100 {
			t.Errorf("expected hpCurrent to be 100, got %d", caster.Stats["hpCurrent"])
		}
		if caster.Stats["mpCurrent"] != 50 {
			t.Errorf("expected mpCurrent to be 50, got %d", caster.Stats["mpCurrent"])
		}

		// Check output format
		// AC = (100 - armor) / 2 = (100 - 40) / 2 = 30
		// THAC0 = 15
		got := ctx.OutputString()
		want := "AC: 30  THAC0: 15\n"
		if got != want {
			t.Errorf("expected output %q, got %q", want, got)
		}
	})

	t.Run("valid target online player", func(t *testing.T) {
		world := &mockDMAcWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
				"player:target": {ID: "player:target", CreatureID: "creature:target", DisplayName: "Alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID: "creature:caster",
					Stats: map[string]int{
						"class": 12,
					},
				},
				"creature:target": {
					ID:          "creature:target",
					DisplayName: "Alice",
					Stats: map[string]int{
						"armor": 20,
						"thaco": 10,
					},
				},
			},
		}

		ctx := &Context{
			ActorID: "player:caster",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{
						{ID: "session:caster", ActorID: "player:caster"},
						{ID: "session:target", ActorID: "player:target"},
					}
				},
			},
		}

		resolved := ResolvedCommand{
			Args: []string{"Alice"},
		}

		handler := NewDMAcHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// AC = (100 - 20) / 2 = 40
		// THAC0 = 10
		got := ctx.OutputString()
		want := "AC: 40  THAC0: 10\n"
		if got != want {
			t.Errorf("expected output %q, got %q", want, got)
		}
	})

	t.Run("extra target argument restores caster instead of setting target ac", func(t *testing.T) {
		world := &mockDMAcWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
				"player:target": {ID: "player:target", CreatureID: "creature:target", DisplayName: "Alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID: "creature:caster",
					Stats: map[string]int{
						"class":     12,
						"hpMax":     80,
						"hpCurrent": 10,
						"mpMax":     30,
						"mpCurrent": 5,
						"armor":     42,
						"thaco":     13,
					},
				},
				"creature:target": {
					ID:          "creature:target",
					DisplayName: "Alice",
					Stats: map[string]int{
						"armor": 20,
						"thaco": 10,
					},
				},
			},
		}

		ctx := &Context{
			ActorID: "player:caster",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{
						{ID: "session:caster", ActorID: "player:caster"},
						{ID: "session:target", ActorID: "player:target"},
					}
				},
			},
		}

		resolved := ResolvedCommand{
			Args: []string{"Alice", "-5"},
		}

		handler := NewDMAcHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		caster, ok := world.Creature("creature:caster")
		if !ok {
			t.Fatalf("caster creature not found")
		}
		if caster.Stats["hpCurrent"] != 80 {
			t.Errorf("expected hpCurrent to be 80, got %d", caster.Stats["hpCurrent"])
		}
		if caster.Stats["mpCurrent"] != 30 {
			t.Errorf("expected mpCurrent to be 30, got %d", caster.Stats["mpCurrent"])
		}
		if _, ok := world.statSets["creature:target"]; ok {
			t.Fatalf("dm_ac must not mutate target stats with extra args: %#v", world.statSets["creature:target"])
		}

		got := ctx.OutputString()
		want := "AC: 29  THAC0: 13\n"
		if got != want {
			t.Errorf("expected output %q, got %q", want, got)
		}
	})

	t.Run("target not found", func(t *testing.T) {
		world := &mockDMAcWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID: "creature:caster",
					Stats: map[string]int{
						"class": 12,
					},
				},
			},
		}

		ctx := &Context{
			ActorID: "player:caster",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{
						{ID: "session:caster", ActorID: "player:caster"},
					}
				},
			},
		}

		resolved := ResolvedCommand{
			Args: []string{"bOB"},
		}

		handler := NewDMAcHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := ctx.OutputString()
		want := "Bob가 없습니다.\n"
		if got != want {
			t.Errorf("expected output %q, got %q", want, got)
		}
	})

	t.Run("saved player without active session is not found like C find_who", func(t *testing.T) {
		world := &mockDMAcWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
				"player:bob":    {ID: "player:bob", CreatureID: "creature:bob", DisplayName: "Bob"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID: "creature:caster",
					Stats: map[string]int{
						"class": model.ClassSubDM,
					},
				},
				"creature:bob": {
					ID:          "creature:bob",
					DisplayName: "Bob",
					Stats: map[string]int{
						"armor": 20,
						"thaco": 10,
					},
				},
			},
		}

		ctx := &Context{
			ActorID: "player:caster",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{
						{ID: "session:caster", ActorID: "player:caster"},
					}
				},
			},
		}

		resolved := ResolvedCommand{
			Args: []string{"bob"},
		}

		handler := NewDMAcHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := ctx.OutputString()
		want := "Bob가 없습니다.\n"
		if got != want {
			t.Errorf("expected output %q, got %q", want, got)
		}
	})
}
