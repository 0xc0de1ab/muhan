package command

import (
	"errors"
	"strings"
	"testing"

	"muhan/internal/commandspec"
)

func TestDispatcherDispatchesByHandlerName(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "봐라", Number: 2, Handler: "look"},
	})
	dispatcher := Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"look": func(ctx *Context, resolved ResolvedCommand) (Status, error) {
				if ctx.SessionID != "s1" {
					t.Fatalf("SessionID = %q, want s1", ctx.SessionID)
				}
				if resolved.Spec.Handler != "look" || resolved.Command() != "봐" {
					t.Fatalf("resolved = %+v, want look/봐", resolved)
				}
				return StatusPrompt, nil
			},
		},
	}

	status, err := dispatcher.DispatchLine(&Context{SessionID: "s1"}, "봐")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusPrompt {
		t.Fatalf("status = %d, want StatusPrompt", status)
	}
}

func TestDispatcherUsesNumberFallbackOnlyWhenHandlerMissing(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "누구", Number: 8, Handler: "who"},
	})
	dispatcher := Dispatcher{
		Registry: registry,
		NumberHandlers: map[int]Handler{
			8: func(*Context, ResolvedCommand) (Status, error) {
				return StatusDoPrompt, nil
			},
		},
	}

	status, err := dispatcher.DispatchLine(nil, "누")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDoPrompt {
		t.Fatalf("status = %d, want StatusDoPrompt", status)
	}
}

func TestDispatcherDispatchesSpecialCommand(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "눌러", Number: 2, Handler: "0", Special: true},
	})
	dispatcher := Dispatcher{
		Registry: registry,
		Special: func(_ *Context, number int, resolved ResolvedCommand) (Status, error) {
			if number != 2 || !resolved.Spec.Special {
				t.Fatalf("special = %d/%v, want 2/true", number, resolved.Spec.Special)
			}
			return StatusDefault, nil
		},
	}

	status, err := dispatcher.DispatchLine(nil, "버튼 눌러")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
}

func TestDispatcherExpandsLegacyPlayerAliases(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "봐", Number: 2, Handler: "look"},
		{Name: "말", Number: 3, Handler: "say"},
	})
	store := NewMemoryAliasStore()
	if err := store.SaveAliases("player:alice", []PlayerAlias{
		{Alias: "검사", Process: "$1 봐;$* 말"},
	}); err != nil {
		t.Fatalf("SaveAliases() error = %v", err)
	}

	var inputs []string
	dispatcher := Dispatcher{
		Registry:   registry,
		AliasStore: store,
		Handlers: map[string]Handler{
			"look": func(_ *Context, resolved ResolvedCommand) (Status, error) {
				inputs = append(inputs, resolved.Input)
				return StatusDefault, nil
			},
			"say": func(_ *Context, resolved ResolvedCommand) (Status, error) {
				inputs = append(inputs, resolved.Input)
				return StatusDefault, nil
			},
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "상자   주변   검사")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	want := []string{"상자 봐", "상자   주변   말"}
	if len(inputs) != len(want) {
		t.Fatalf("expanded inputs = %#v, want %#v", inputs, want)
	}
	for i := range want {
		if inputs[i] != want[i] {
			t.Fatalf("expanded inputs = %#v, want %#v", inputs, want)
		}
	}
	if got := ctx.OutputString(); got != "\n\n" {
		t.Fatalf("alias expansion output = %q, want two legacy newlines", got)
	}
}

func TestDispatcherMatchesASCIIAliasesCaseInsensitivelyLikeLegacy(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "look", Number: 2, Handler: "look"},
	})
	store := NewMemoryAliasStore()
	if err := store.SaveAliases("player:alice", []PlayerAlias{
		{Alias: "quick", Process: "target look"},
	}); err != nil {
		t.Fatalf("SaveAliases() error = %v", err)
	}

	called := false
	dispatcher := Dispatcher{
		Registry:   registry,
		AliasStore: store,
		Handlers: map[string]Handler{
			"look": func(_ *Context, resolved ResolvedCommand) (Status, error) {
				called = true
				if resolved.Input != "target look" {
					t.Fatalf("resolved input = %q, want target look", resolved.Input)
				}
				return StatusDefault, nil
			},
		},
	}

	if _, err := dispatcher.DispatchLine(&Context{ActorID: "player:alice"}, "QUICK"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if !called {
		t.Fatal("expanded alias command was not dispatched")
	}
}

func TestDispatcherDoesNotReexpandExpandedAliasCommands(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "보기", Number: 2, Handler: "look"},
	})
	store := NewMemoryAliasStore()
	if err := store.SaveAliases("player:alice", []PlayerAlias{
		{Alias: "검사", Process: "보기"},
		{Alias: "보기", Process: "재확장"},
	}); err != nil {
		t.Fatalf("SaveAliases() error = %v", err)
	}

	var command string
	dispatcher := Dispatcher{
		Registry:   registry,
		AliasStore: store,
		Handlers: map[string]Handler{
			"look": func(_ *Context, resolved ResolvedCommand) (Status, error) {
				command = resolved.Command()
				return StatusDefault, nil
			},
		},
	}

	status, err := dispatcher.DispatchLine(&Context{ActorID: "player:alice"}, "검사")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if command != "보기" {
		t.Fatalf("expanded command = %q, want 보기 without recursive alias expansion", command)
	}
}

func TestDispatcherLimitsLegacyAliasExpandedCommandBuffer(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "봐", Number: 2, Handler: "look"},
	})
	store := NewMemoryAliasStore()
	if err := store.SaveAliases("player:alice", []PlayerAlias{
		{Alias: "반복", Process: strings.Repeat("봐;", legacyAliasMaxExpandedCommands+6)},
	}); err != nil {
		t.Fatalf("SaveAliases() error = %v", err)
	}

	count := 0
	dispatcher := Dispatcher{
		Registry:   registry,
		AliasStore: store,
		Handlers: map[string]Handler{
			"look": func(_ *Context, _ ResolvedCommand) (Status, error) {
				count++
				return StatusDefault, nil
			},
		},
	}

	if _, err := dispatcher.DispatchLine(&Context{ActorID: "player:alice"}, "반복"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if count != legacyAliasMaxExpandedCommands {
		t.Fatalf("expanded command count = %d, want %d", count, legacyAliasMaxExpandedCommands)
	}
}

func TestLegacyAliasExpansionTruncatesToCommandBuffer(t *testing.T) {
	process := strings.Repeat("가", 130)
	expanded := legacyExpandAliasCommands(process, "반복")
	if len(expanded) != 1 {
		t.Fatalf("expanded = %#v, want one command", expanded)
	}
	if got := legacyAliasInputByteLen(expanded[0]); got > legacyAliasExpandedCommandMaxBytes {
		t.Fatalf("expanded byte length = %d, want <= %d", got, legacyAliasExpandedCommandMaxBytes)
	}
	want := legacyTruncateBytes(process, legacyAliasExpandedCommandMaxBytes)
	if expanded[0] != want {
		t.Fatalf("expanded = %q, want %q", expanded[0], want)
	}
}

func TestDispatcherAppliesPrivilegePolicyBeforeHandler(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "*공지", Number: 102, Handler: "dm_broadcast", Privileged: true},
	})
	dispatcher := Dispatcher{
		Registry:  registry,
		Privilege: func(ResolvedCommand) bool { return false },
		Handlers: map[string]Handler{
			"dm_broadcast": func(*Context, ResolvedCommand) (Status, error) {
				t.Fatal("handler should not run")
				return StatusDefault, nil
			},
		},
	}

	_, err := dispatcher.DispatchLine(nil, "*공지")
	if !errors.Is(err, ErrPrivilegedCommand) {
		t.Fatalf("DispatchLine() error = %v, want ErrPrivilegedCommand", err)
	}
}

func TestDispatcherReportsUnhandledCommand(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "때려", Number: 23, Handler: "attack"},
	})
	dispatcher := Dispatcher{Registry: registry}

	_, err := dispatcher.DispatchLine(nil, "고블린 때려")
	if !errors.Is(err, ErrUnhandledCommand) {
		t.Fatalf("DispatchLine() error = %v, want ErrUnhandledCommand", err)
	}
}
