package command

import (
	"testing"

	"muhan/internal/commandspec"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestQuitHandlerWritesGoodbyeAndDisconnects(t *testing.T) {
	handler := NewQuitHandler()
	ctx := &Context{}

	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDisconnect {
		t.Fatalf("status = %d, want StatusDisconnect", status)
	}
	if ctx.OutputString() != quitGoodbye {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), quitGoodbye)
	}
}

func TestQuitHandlerDispatchesLegacyQuitCommand(t *testing.T) {
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "끝", Number: 3, Handler: "quit"},
		}),
		Handlers: map[string]Handler{
			"quit": NewQuitHandler(),
		},
	}

	ctx := &Context{}
	status, err := dispatcher.DispatchLine(ctx, "끝")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDisconnect {
		t.Fatalf("status = %d, want StatusDisconnect", status)
	}
	if ctx.OutputString() != quitGoodbye {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), quitGoodbye)
	}
}

func TestQuitHandlerBlocksCombatUntilAttackCooldownPlusTwentyLikeLegacy(t *testing.T) {
	withFakeMagicEffectTime(t, 1000)
	world := state.NewWorld(lookWorld(t))
	if _, err := world.AddEnemy("creature:guard", "creature:alice"); err != nil {
		t.Fatalf("AddEnemy() error = %v", err)
	}
	if err := world.SetCreatureCooldown("creature:alice", "attack", 1000, 1); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewQuitHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault while combat wait is active", status)
	}
	if ctx.OutputString() != "21초동안 기다리세요.\n" {
		t.Fatalf("output = %q, want legacy please_wait", ctx.OutputString())
	}
}

func TestQuitHandlerAllowsCombatAfterAttackCooldownPlusTwentyLikeLegacy(t *testing.T) {
	withFakeMagicEffectTime(t, 1022)
	world := state.NewWorld(lookWorld(t))
	if _, err := world.AddEnemy("creature:guard", "creature:alice"); err != nil {
		t.Fatalf("AddEnemy() error = %v", err)
	}
	if err := world.SetCreatureCooldown("creature:alice", "attack", 1000, 1); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewQuitHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDisconnect {
		t.Fatalf("status = %d, want StatusDisconnect after combat wait expires", status)
	}
	if ctx.OutputString() != quitGoodbye {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), quitGoodbye)
	}
}

func TestQuitHandlerCleansUpLowLevelNonInvinciblePlayerLikeLegacy(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{"class": legacyClassFighter, "level": 5}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	sink := &recordingQuitLowLevelSink{}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewQuitHandlerWithOptions(world, WithQuitLowLevelSink(sink))(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDisconnect {
		t.Fatalf("status = %d, want StatusDisconnect", status)
	}
	if ctx.OutputString() != quitGoodbye {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), quitGoodbye)
	}
	if len(sink.calls) != 1 || sink.calls[0] != "player:alice" {
		t.Fatalf("cleanup calls = %v, want [player:alice]", sink.calls)
	}
}

func TestQuitHandlerKeepsHighLevelOrInvinciblePlayerLikeLegacy(t *testing.T) {
	tests := []struct {
		name  string
		class int
		level int
	}{
		{name: "level six regular class", class: legacyClassFighter, level: 6},
		{name: "low level invincible", class: legacyClassInvincible, level: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := lookWorld(t)
			alice := loaded.Creatures["creature:alice"]
			alice.Stats = map[string]int{"class": tt.class, "level": tt.level}
			loaded.Creatures[alice.ID] = alice
			world := state.NewWorld(loaded)
			sink := &recordingQuitLowLevelSink{}

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewQuitHandlerWithOptions(world, WithQuitLowLevelSink(sink))(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDisconnect {
				t.Fatalf("status = %d, want StatusDisconnect", status)
			}
			if len(sink.calls) != 0 {
				t.Fatalf("cleanup calls = %v, want none", sink.calls)
			}
		})
	}
}

func TestQuitHandlerAllowsNilContext(t *testing.T) {
	status, err := NewQuitHandler()(nil, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDisconnect {
		t.Fatalf("status = %d, want StatusDisconnect", status)
	}
}

type recordingQuitLowLevelSink struct {
	calls []model.PlayerID
}

func (s *recordingQuitLowLevelSink) CleanupLowLevelQuit(_ *Context, playerID model.PlayerID) error {
	s.calls = append(s.calls, playerID)
	return nil
}
