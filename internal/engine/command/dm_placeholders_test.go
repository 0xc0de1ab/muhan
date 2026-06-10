package command

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"muhan/internal/commandspec"
	"muhan/internal/commandspec/extract"
	"muhan/internal/world/model"
)

func TestNewDMPlaceholderHandlersRegistersDefaultKeys(t *testing.T) {
	handlers := NewDMPlaceholderHandlers(dmPlaceholderTestWorld{})
	if len(handlers) != len(DefaultDMPlaceholderHandlerKeys) {
		t.Fatalf("handlers = %d, want %d", len(handlers), len(DefaultDMPlaceholderHandlerKeys))
	}
	for _, key := range DefaultDMPlaceholderHandlerKeys {
		if handlers[key] == nil {
			t.Fatalf("handler %q is not registered", key)
		}
	}
}

func TestNewDMPlaceholderHandlersRegistersRequestedKeys(t *testing.T) {
	handlers := NewDMPlaceholderHandlers(dmPlaceholderTestWorld{}, "dm_teleport", " ", "dm_echo", "dm_echo")
	if len(handlers) != 2 {
		t.Fatalf("handlers = %d, want 2", len(handlers))
	}
	for _, key := range []string{"dm_teleport", "dm_echo"} {
		if handlers[key] == nil {
			t.Fatalf("handler %q is not registered", key)
		}
	}
}

func TestDMPlaceholderHandlerRejectsUnprivilegedActor(t *testing.T) {
	world := dmPlaceholderWorldWithClass(model.ClassFighter)
	handler := NewDMPlaceholderHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, dmPlaceholderResolved("dm_teleport"))
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusPrompt {
		t.Fatalf("status = %d, want StatusPrompt", status)
	}
	if got := ctx.OutputString(); got != "" {
		t.Fatalf("output = %q, want no permission output", got)
	}
}

func TestDMPlaceholderHandlerRejectsWhenAuthorityCannotBeVerified(t *testing.T) {
	tests := []struct {
		name  string
		ctx   *Context
		world DMPlaceholderWorld
	}{
		{name: "nil context", world: dmPlaceholderWorldWithClass(model.ClassCaretaker)},
		{name: "missing actor", ctx: &Context{}, world: dmPlaceholderWorldWithClass(model.ClassCaretaker)},
		{name: "nil world", ctx: &Context{ActorID: "player:alice"}},
		{name: "missing player", ctx: &Context{ActorID: "player:alice"}, world: dmPlaceholderTestWorld{}},
		{
			name: "missing class",
			ctx:  &Context{ActorID: "player:alice"},
			world: dmPlaceholderTestWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {ID: "creature:alice"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := NewDMPlaceholderHandler(tt.world)(tt.ctx, dmPlaceholderResolved("dm_echo"))
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusPrompt {
				t.Fatalf("status = %d, want StatusPrompt", status)
			}
			if tt.ctx != nil && tt.ctx.OutputString() != "" {
				t.Fatalf("output = %q, want no permission output", tt.ctx.OutputString())
			}
		})
	}
}

func TestDMPlaceholderHandlerReportsUnimplementedForPrivilegedActor(t *testing.T) {
	tests := []struct {
		name  string
		class int
	}{
		{name: "zonemaker", class: legacyClassZoneMaker},
		{name: "caretaker", class: model.ClassCaretaker},
		{name: "sub dm", class: model.ClassSubDM},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewDMPlaceholderHandler(dmPlaceholderWorldWithClass(tt.class))(ctx, dmPlaceholderResolved("dm_shutdown"))
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}
			got := ctx.OutputString()
			if !strings.Contains(got, "dm_shutdown") || !strings.Contains(got, "아직 구현되지 않았습니다") {
				t.Fatalf("output = %q, want unimplemented dm_shutdown", got)
			}
		})
	}
}

func TestDMPlaceholderHandlerUsesPropertyClass(t *testing.T) {
	world := dmPlaceholderTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:         "creature:alice",
				Properties: map[string]string{"class": "10"},
			},
		},
	}
	ctx := &Context{ActorID: "player:alice"}

	_, err := NewDMPlaceholderHandler(world)(ctx, dmPlaceholderResolved("dm_users"))
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if strings.Contains(ctx.OutputString(), "권한") {
		t.Fatalf("output = %q, want property class accepted as privileged placeholder", ctx.OutputString())
	}
}

func TestDMPlaceholderTargetHandlersArePrivilegedInCCommandTable(t *testing.T) {
	entries, err := extract.ExtractRoot(dmPlaceholderRepoRoot(t))
	if err != nil {
		t.Fatalf("ExtractRoot(repo root) error = %v", err)
	}

	targets := make(map[string]struct{}, len(DefaultDMPlaceholderHandlerKeys))
	for _, key := range DefaultDMPlaceholderHandlerKeys {
		targets[key] = struct{}{}
	}
	seen := make(map[string]struct{}, len(targets))
	for _, entry := range entries {
		if _, ok := targets[entry.Handler]; !ok {
			continue
		}
		seen[entry.Handler] = struct{}{}
		if !entry.Privileged {
			t.Fatalf("C command %q handler %q is not privileged", entry.Name, entry.Handler)
		}
	}

	for _, key := range DefaultDMPlaceholderHandlerKeys {
		if _, ok := seen[key]; !ok {
			t.Fatalf("handler %q was not found in C command table", key)
		}
	}
}

func dmPlaceholderResolved(handler string) ResolvedCommand {
	return ResolvedCommand{
		Input:  "*" + handler,
		Parsed: commandWithVerb("*" + handler),
		Spec: commandspec.CommandSpec{
			Name:       "*" + handler,
			Number:     101,
			Handler:    handler,
			Privileged: true,
		},
	}
}

func dmPlaceholderWorldWithClass(class int) dmPlaceholderTestWorld {
	return dmPlaceholderTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": class}},
		},
	}
}

type dmPlaceholderTestWorld struct {
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
}

func (w dmPlaceholderTestWorld) Player(id model.PlayerID) (model.Player, bool) {
	player, ok := w.players[id]
	return player, ok
}

func (w dmPlaceholderTestWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	creature, ok := w.creatures[id]
	return creature, ok
}

func dmPlaceholderRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
}
