package command

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"muhan/internal/commandspec"
	"muhan/internal/persist/legacykr"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestSetTitleHandlerStoresAndReportsCustomTitle(t *testing.T) {
	root := t.TempDir()
	runtime := titleRuntime(t, "")

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewSetTitleHandler(root, runtime)(ctx, ResolvedCommand{Args: []string{"무림", "고수"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 이제부터 무림 고수 Alice입니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}

	creature, ok := runtime.Creature("creature:alice")
	if !ok {
		t.Fatal("missing creature")
	}
	if got := creature.Properties[legacyTitleProperty]; got != "무림 고수" {
		t.Fatalf("legacy title = %q, want 무림 고수", got)
	}
}

func TestSetTitleHandlerShowsCurrentTitleForEmptyInput(t *testing.T) {
	root := t.TempDir()
	runtime := titleRuntime(t, "기존칭호")
	handler := NewSetTitleHandler(root, runtime)

	for _, resolved := range []ResolvedCommand{
		{},
		{Args: []string{"   "}},
	} {
		ctx := &Context{ActorID: "player:alice"}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler() error = %v", err)
		}
		if status != StatusDefault || ctx.OutputString() != "당신은 기존칭호 Alice입니다.\n" {
			t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
		}
	}

	creature, _ := runtime.Creature("creature:alice")
	if got := creature.Properties[legacyTitleProperty]; got != "기존칭호" {
		t.Fatalf("legacy title after empty input = %q, want unchanged", got)
	}
}

func TestSetTitleHandlerRejectsTooLongLegacyTitle(t *testing.T) {
	root := t.TempDir()
	runtime := titleRuntime(t, "기존칭호")
	longTitle := strings.Repeat("가", 40)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewSetTitleHandler(root, runtime)(ctx, ResolvedCommand{Args: []string{longTitle}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "칭호가 너무 깁니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}

	creature, _ := runtime.Creature("creature:alice")
	if got := creature.Properties[legacyTitleProperty]; got != "기존칭호" {
		t.Fatalf("legacy title after long reject = %q, want unchanged", got)
	}
}

func TestClearTitleHandlerDeletesCustomTitle(t *testing.T) {
	root := t.TempDir()
	runtime := titleRuntime(t, "별칭")

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewClearTitleHandler(root, runtime)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 이제부터 백정 Alice입니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}

	creature, _ := runtime.Creature("creature:alice")
	if _, ok := creature.Properties[legacyTitleProperty]; ok {
		t.Fatalf("legacy title still set: %+v", creature.Properties)
	}
}

func TestClearTitleHandlerReportsMissingCustomTitle(t *testing.T) {
	root := t.TempDir()
	runtime := titleRuntime(t, "")

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewClearTitleHandler(root, runtime)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "칭호가 설정되어 있지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
}

func TestTitleHandlersRequireActor(t *testing.T) {
	root := t.TempDir()
	runtime := titleRuntime(t, "")
	tests := []struct {
		name    string
		handler Handler
	}{
		{name: "set", handler: NewSetTitleHandler(root, runtime)},
		{name: "clear", handler: NewClearTitleHandler(root, runtime)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.handler(&Context{}, ResolvedCommand{Args: []string{"칭호"}})
			if !errors.Is(err, ErrInventoryActorRequired) {
				t.Fatalf("handler() error = %v, want ErrInventoryActorRequired", err)
			}
		})
	}
}

func TestTitleDispatcherAliases(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "칭호", Number: 84, Handler: "set_title"},
		{Name: "칭호삭제", Number: 84, Handler: "clear_title"},
		{Name: "set_title", Number: 84, Handler: "set_title"},
		{Name: "clear_title", Number: 84, Handler: "clear_title"},
	})

	tests := []struct {
		name      string
		line      string
		initial   string
		wantTitle string
	}{
		{name: "korean set", line: "전설 칭호", wantTitle: "전설"},
		{name: "korean set preserves cut_command spaces", line: "무림  지존   칭호", wantTitle: "무림  지존  "},
		{name: "korean clear", line: "칭호삭제", initial: "전설"},
		{name: "english set", line: "set_title 영웅", wantTitle: "영웅"},
		{name: "english clear", line: "clear_title", initial: "영웅"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			runtime := titleRuntime(t, tt.initial)
			dispatcher := Dispatcher{
				Registry: registry,
				Handlers: map[string]Handler{
					"set_title":   NewSetTitleHandler(root, runtime),
					"clear_title": NewClearTitleHandler(root, runtime),
				},
			}

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, tt.line)
			if err != nil {
				t.Fatalf("DispatchLine(%q) error = %v", tt.line, err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}

			creature, _ := runtime.Creature("creature:alice")
			got := creature.Properties[legacyTitleProperty]
			if got != tt.wantTitle {
				t.Fatalf("legacy title after %q = %q, want %q", tt.line, got, tt.wantTitle)
			}
			if tt.wantTitle != "" {
				filePath := filepath.Join(root, "player", "alias", "Alice")
				data, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("ReadFile(%q) error = %v", filePath, err)
				}
				decodedText, err := legacykr.ValidUTF8OrDecodeContext(legacykr.Context{Path: filePath, Field: "alias"}, data)
				if err != nil {
					t.Fatalf("failed to decode alias file: %v", err)
				}
				wantText := "~!\n" + tt.wantTitle + "\n~!\n"
				if strings.ReplaceAll(decodedText, "\r\n", "\n") != wantText {
					t.Fatalf("alias file text = %q, want %q", decodedText, wantText)
				}
			}
		})
	}
}

func TestTitleHandlerDiskSerialization(t *testing.T) {
	root := t.TempDir()
	runtime := titleRuntime(t, "")

	// 1. Set title when file does not exist
	ctx := &Context{ActorID: "player:alice"}
	status, err := NewSetTitleHandler(root, runtime)(ctx, ResolvedCommand{Args: []string{"무림", "지존"}})
	if err != nil {
		t.Fatalf("NewSetTitleHandler error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}

	// Verify alias file contents. It should be:
	// ~!\n무림 지존\n~!\n
	filePath := filepath.Join(root, "player", "alias", "Alice")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read alias file: %v", err)
	}
	decodedText, err := legacykr.ValidUTF8OrDecodeContext(legacykr.Context{Path: filePath, Field: "alias"}, data)
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	wantText := "~!\n무림 지존\n~!\n"
	if strings.ReplaceAll(decodedText, "\r\n", "\n") != wantText {
		t.Fatalf("file content = %q, want %q", decodedText, wantText)
	}

	// 2. Clear title. It should write back with empty title block: ~!\n~!\n after aliases.
	status, err = NewClearTitleHandler(root, runtime)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("NewClearTitleHandler error = %v", err)
	}
	data, err = os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read alias file: %v", err)
	}
	decodedText, err = legacykr.ValidUTF8OrDecodeContext(legacykr.Context{Path: filePath, Field: "alias"}, data)
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	wantText = "~!\n~!\n"
	if strings.ReplaceAll(decodedText, "\r\n", "\n") != wantText {
		t.Fatalf("file content = %q, want %q", decodedText, wantText)
	}

	// 3. Set aliases and set title. Verify aliases are preserved.
	store := NewFileAliasStore(root, runtime)
	err = store.SaveAliases("player:alice", []PlayerAlias{
		{Alias: "a", Process: "go north"},
		{Alias: "b", Process: "get all"},
	})
	if err != nil {
		t.Fatalf("failed to save aliases: %v", err)
	}

	// Now set title again
	status, err = NewSetTitleHandler(root, runtime)(ctx, ResolvedCommand{Args: []string{"영웅"}})
	if err != nil {
		t.Fatalf("NewSetTitleHandler error = %v", err)
	}

	data, err = os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read alias file: %v", err)
	}
	decodedText, err = legacykr.ValidUTF8OrDecodeContext(legacykr.Context{Path: filePath, Field: "alias"}, data)
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	wantText = "a\ngo north\nb\nget all\n~!\n영웅\n~!\n"
	if strings.ReplaceAll(decodedText, "\r\n", "\n") != wantText {
		t.Fatalf("file content = %q, want %q", decodedText, wantText)
	}
}

func titleRuntime(t *testing.T, customTitle string) *state.World {
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
	if customTitle != "" {
		properties = map[string]string{legacyTitleProperty: customTitle}
	}
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:plaza",
		Level:       1,
		Stats:       map[string]int{"class": model.ClassFighter, "level": 1},
		Properties:  properties,
	})
	return state.NewWorld(loaded)
}
