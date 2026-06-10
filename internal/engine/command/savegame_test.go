package command

import (
	"errors"
	"testing"

	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

func TestSaveGameHandlerRequiresActor(t *testing.T) {
	handler := NewSaveGameHandler()

	tests := []struct {
		name string
		ctx  *Context
	}{
		{name: "nil context"},
		{name: "empty actor", ctx: &Context{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := handler(tt.ctx, ResolvedCommand{})
			if !errors.Is(err, ErrSaveGameActorRequired) {
				t.Fatalf("handler() error = %v, want ErrSaveGameActorRequired", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}
			if tt.ctx != nil && tt.ctx.OutputString() != "" {
				t.Fatalf("output = %q, want empty", tt.ctx.OutputString())
			}
		})
	}
}

func TestSaveGameHandlerWritesLegacySavedMessage(t *testing.T) {
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "저장", Number: 52, Handler: "savegame"},
		}),
		Handlers: map[string]Handler{
			"savegame": NewSaveGameHandler(),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "저장")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if ctx.OutputString() != saveGameSavedMessage {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), saveGameSavedMessage)
	}
}

func TestSaveGameHandlerUsesLegacyLevelMessagesWhenWorldAvailable(t *testing.T) {
	tests := []struct {
		name  string
		class int
		level int
		want  string
	}{
		{name: "low level regular class", class: model.ClassFighter, level: 5, want: saveGameLowLevelMessage},
		{name: "level six regular class", class: model.ClassFighter, level: 6, want: ""},
		{name: "level seven regular class", class: model.ClassFighter, level: 7, want: saveGameSavedMessage},
		{name: "low level invincible", class: model.ClassInvincible, level: 1, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewSaveGameHandler()
			ctx := &Context{
				ActorID: "player:alice",
				Values: map[string]any{
					"game.world": saveGameMessageWorldStub{
						player: model.Player{ID: "player:alice", CreatureID: "creature:alice"},
						creature: model.Creature{
							ID:    "creature:alice",
							Stats: map[string]int{"class": tt.class, "level": tt.level},
						},
					},
				},
			}

			status, err := handler(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}
			if ctx.OutputString() != tt.want {
				t.Fatalf("output = %q, want %q", ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestSaveGameHandlerCallsOptionalSink(t *testing.T) {
	var calls int
	var gotCtx *Context
	var gotPlayerID model.PlayerID

	handler := NewSaveGameHandler(WithSaveGameSink(SaveGameSinkFunc(func(ctx *Context, playerID model.PlayerID) error {
		calls++
		gotCtx = ctx
		gotPlayerID = playerID
		return nil
	})))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if calls != 1 {
		t.Fatalf("sink calls = %d, want 1", calls)
	}
	if gotCtx != ctx {
		t.Fatalf("sink ctx = %p, want %p", gotCtx, ctx)
	}
	if gotPlayerID != "player:alice" {
		t.Fatalf("sink player id = %q, want player:alice", gotPlayerID)
	}
	if ctx.OutputString() != saveGameSavedMessage {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), saveGameSavedMessage)
	}
}

type saveGameMessageWorldStub struct {
	player   model.Player
	creature model.Creature
}

func (w saveGameMessageWorldStub) Player(id model.PlayerID) (model.Player, bool) {
	if id == w.player.ID {
		return w.player, true
	}
	return model.Player{}, false
}

func (w saveGameMessageWorldStub) Creature(id model.CreatureID) (model.Creature, bool) {
	if id == w.creature.ID {
		return w.creature, true
	}
	return model.Creature{}, false
}
