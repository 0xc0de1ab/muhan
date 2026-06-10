package command

import (
	"errors"
	"testing"

	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type mockDMSaveAllPlyWorld struct {
	players        map[model.PlayerID]model.Player
	creatures      map[model.CreatureID]model.Creature
	saveCalled     bool
	saveShouldFail bool
}

func (m *mockDMSaveAllPlyWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMSaveAllPlyWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMSaveAllPlyWorld) SaveAllPlayers() error {
	m.saveCalled = true
	if m.saveShouldFail {
		return errors.New("failed to save")
	}
	return nil
}

func TestDMSaveAllPly_Handler(t *testing.T) {
	for _, tt := range []struct {
		name  string
		class int
	}{
		{name: "ordinary", class: model.ClassFighter},
		{name: "invincible", class: model.ClassInvincible},
		{name: "zonemaker", class: legacyClassZoneMaker},
		{name: "caretaker", class: model.ClassCaretaker},
		{name: "bulsa", class: model.ClassBulsa},
		{name: "sub dm", class: model.ClassSubDM},
		{name: "dm", class: model.ClassDM},
	} {
		t.Run("caster saves all like C - "+tt.name, func(t *testing.T) {
			world := &mockDMSaveAllPlyWorld{
				players: map[model.PlayerID]model.Player{
					"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:caster": {
						ID: "creature:caster",
						Stats: map[string]int{
							"class": tt.class,
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
					Name:    "*사용자저장",
					Handler: "dm_save_all_ply",
				},
			}

			handler := NewDMSaveAllPlyHandler(world)
			status, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %v, want StatusDefault", status)
			}

			if got := ctx.OutputString(); got != "" {
				t.Errorf("expected no C-visible success output, got %q", got)
			}
			if !world.saveCalled {
				t.Error("expected SaveAllPlayers to be called")
			}
		})
	}

	t.Run("player without creature is ignored", func(t *testing.T) {
		world := &mockDMSaveAllPlyWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster"},
			},
		}
		ctx := &Context{ActorID: "player:caster"}
		resolved := ResolvedCommand{
			Spec: commandspec.CommandSpec{
				Name:    "*사용자저장",
				Handler: "dm_save_all_ply",
			},
		}

		status, err := NewDMSaveAllPlyHandler(world)(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Fatalf("status = %v, want StatusDefault", status)
		}
		if world.saveCalled {
			t.Fatal("SaveAllPlayers called without actor creature")
		}
		if got := ctx.OutputString(); got != "" {
			t.Fatalf("output = %q, want none", got)
		}
	})

	t.Run("authorized caster - save failure", func(t *testing.T) {
		world := &mockDMSaveAllPlyWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID: "creature:caster",
					Stats: map[string]int{
						"class": model.ClassDM,
					},
				},
			},
			saveShouldFail: true,
		}

		ctx := &Context{
			ActorID: "player:caster",
		}

		resolved := ResolvedCommand{
			Args: []string{},
			Spec: commandspec.CommandSpec{
				Name:    "*사용자저장",
				Handler: "dm_save_all_ply",
			},
		}

		handler := NewDMSaveAllPlyHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !world.saveCalled {
			t.Error("expected SaveAllPlayers to be called")
		}
		if got := ctx.OutputString(); got != "" {
			t.Errorf("output = %q, want no C-visible save failure output", got)
		}
	})
}
