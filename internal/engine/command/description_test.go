package command

import (
	"errors"
	"strings"
	"testing"

	"muhan/internal/commandspec"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestDescriptionHandlerStoresLegacyDescriptionField(t *testing.T) {
	runtime := descriptionRuntime(t, "", "")
	dispatcher := descriptionDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "푸른 옷을 입고 묘사")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 이제부터 푸른 옷을 입고 있습니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}

	creature, ok := runtime.Creature("creature:alice")
	if !ok {
		t.Fatal("missing creature")
	}
	if got := creature.Description; got != "푸른 옷을 입고 " {
		t.Fatalf("description field = %q, want trailing-space legacy description", got)
	}
	if got := creature.Properties[legacyDescriptionProperty]; got != "" {
		t.Fatalf("description property = %q, want canonical field storage", got)
	}
}

func TestDescriptionHandlerPreservesVerbFinalCutCommandSpaces(t *testing.T) {
	runtime := descriptionRuntime(t, "", "")
	dispatcher := descriptionDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "푸른 옷   묘사")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 이제부터 푸른 옷   있습니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}

	creature, ok := runtime.Creature("creature:alice")
	if !ok {
		t.Fatal("missing creature")
	}
	if got := creature.Description; got != "푸른 옷   " {
		t.Fatalf("description field = %q, want C cut_command spaces", got)
	}
}

func TestDescriptionHandlerClearsDescriptionForNoText(t *testing.T) {
	runtime := descriptionRuntime(t, "기존 묘사 ", "")

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewDescriptionHandler(runtime)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 서 있습니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}

	creature, _ := runtime.Creature("creature:alice")
	if got := creature.Description; got != "" {
		t.Fatalf("description field after clear = %q, want empty", got)
	}
	if got := creature.Properties[legacyDescriptionProperty]; got != "" {
		t.Fatalf("description property after clear = %q, want removed", got)
	}
}

func TestDescriptionHandlerRejectsTooLongLegacyInput(t *testing.T) {
	runtime := descriptionRuntime(t, "기존 묘사 ", "")
	dispatcher := descriptionDispatcher(t, runtime)
	longDescription := strings.Repeat("가", 29)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, longDescription+" 묘사")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "묘사가 너무 깁니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}

	creature, _ := runtime.Creature("creature:alice")
	if got := creature.Properties[legacyDescriptionProperty]; got != "기존 묘사 " {
		t.Fatalf("description after long reject = %q, want unchanged", got)
	}
}

func TestDescriptionHandlerSupportsCommandFirstFallback(t *testing.T) {
	runtime := descriptionRuntime(t, "", "")
	dispatcher := descriptionDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "묘사 조용히 서서"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}

	creature, _ := runtime.Creature("creature:alice")
	if got := creature.Description; got != "조용히 서서 " {
		t.Fatalf("description field = %q, want command-first text", got)
	}
}

func TestDescriptionHandlerStateWorldUsesCanonicalCreatureDescriptionField(t *testing.T) {
	runtime := descriptionRuntime(t, "", "기존 필드 묘사")
	dispatcher := descriptionDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "새 필드 묘사"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}

	creature, _ := runtime.Creature("creature:alice")
	if got := creature.Properties[legacyDescriptionProperty]; got != "" {
		t.Fatalf("description property = %q, want canonical field storage", got)
	}
	if got := creature.Description; got != "새 필드 " {
		t.Fatalf("creature Description field = %q, want 새 필드", got)
	}
	if got := RenderPlayerLook(runtime, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:plaza",
	}); got != "그녀는 새 필드 있습니다.\n" {
		t.Fatalf("RenderPlayerLook() = %q, want field-backed description", got)
	}
}

func TestDescriptionHandlerClearOverridesStaleCreatureDescriptionField(t *testing.T) {
	runtime := descriptionRuntime(t, "", "기존 필드 묘사")

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewDescriptionHandler(runtime)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 서 있습니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}

	player, _ := runtime.Player("player:alice")
	if got := RenderPlayerLook(runtime, player); got != "그녀는 있습니다.\n" {
		t.Fatalf("RenderPlayerLook() = %q, want stale field hidden", got)
	}
}

func TestDescriptionHandlerRequiresActor(t *testing.T) {
	runtime := descriptionRuntime(t, "", "")

	_, err := NewDescriptionHandler(runtime)(&Context{}, ResolvedCommand{Args: []string{"묘사"}})
	if !errors.Is(err, ErrInventoryActorRequired) {
		t.Fatalf("handler() error = %v, want ErrInventoryActorRequired", err)
	}
}

func descriptionDispatcher(t *testing.T, runtime *state.World) Dispatcher {
	t.Helper()
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "묘사", Number: 73, Handler: "description"},
	})
	return Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"description": NewDescriptionHandler(runtime),
		},
	}
}

func descriptionRuntime(t *testing.T, propertyDescription string, fieldDescription string) *state.World {
	t.Helper()
	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:plaza",
		DisplayName: "광장",
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:plaza",
	})
	properties := map[string]string(nil)
	if propertyDescription != "" {
		properties = map[string]string{legacyDescriptionProperty: propertyDescription}
	}
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		Description: fieldDescription,
		PlayerID:    "player:alice",
		RoomID:      "room:plaza",
		Properties:  properties,
	})
	return state.NewWorld(loaded)
}
