package command

import (
	"errors"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMLogWorld struct {
	players    map[model.PlayerID]model.Player
	creatures  map[model.CreatureID]model.Creature
	logDeleted string
	logRead    string
	logFiles   map[string]string
}

func (m *mockDMLogWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMLogWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMLogWorld) ReadLogFile(name string) (string, error) {
	m.logRead = name
	content, ok := m.logFiles[name]
	if !ok {
		return "", errors.New("file not found")
	}
	return content, nil
}

func (m *mockDMLogWorld) DeleteLogFile(name string) error {
	m.logDeleted = name
	delete(m.logFiles, name)
	return nil
}

func TestDMLog(t *testing.T) {
	t.Run("permission denied for non-DM", func(t *testing.T) {
		world := &mockDMLogWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}}, // below DM (13)
			},
		}
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Input: "*log",
			Spec:  commandspec.CommandSpec{Name: "*log", Handler: "dm_log"},
		}

		handler := NewDMLogHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusPrompt {
			t.Errorf("expected status %d, got %d", StatusPrompt, status)
		}
		if ctx.OutputString() != "" {
			t.Errorf("expected no permission output, got %q", ctx.OutputString())
		}
	})

	t.Run("success with r (delete log)", func(t *testing.T) {
		world := &mockDMLogWorld{
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": 13}},
			},
			logFiles: map[string]string{
				"log": "some log content",
			},
		}
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Input: "*log r",
			Args:  []string{"r"},
			Spec:  commandspec.CommandSpec{Name: "*log", Handler: "dm_log"},
		}

		handler := NewDMLogHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusDefault {
			t.Errorf("expected status %d, got %d", StatusDefault, status)
		}
		if world.logDeleted != "log" {
			t.Errorf("expected world.DeleteLogFile to be called with 'log', got %q", world.logDeleted)
		}
		if !strings.Contains(ctx.OutputString(), "Log파일을 삭제했습니다.") {
			t.Errorf("expected success message, got %q", ctx.OutputString())
		}
	})

	t.Run("parsed r slot deletes log without synthetic Args", func(t *testing.T) {
		world := &mockDMLogWorld{
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": 13}},
			},
			logFiles: map[string]string{
				"log": "some log content",
			},
		}
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Input: "*log r",
			Parsed: commandparse.Command{
				Num: 2,
				Str: [commandparse.CommandMax]string{"*log", "r"},
			},
			Spec: commandspec.CommandSpec{Name: "*log", Handler: "dm_log"},
		}

		handler := NewDMLogHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusDefault {
			t.Errorf("expected status %d, got %d", StatusDefault, status)
		}
		if world.logDeleted != "log" {
			t.Errorf("expected world.DeleteLogFile to be called with 'log', got %q", world.logDeleted)
		}
		if !strings.Contains(ctx.OutputString(), "Log파일을 삭제했습니다.") {
			t.Errorf("expected success message, got %q", ctx.OutputString())
		}
	})

	t.Run("extra argument prevents r delete shortcut", func(t *testing.T) {
		world := &mockDMLogWorld{
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": 13}},
			},
			logFiles: map[string]string{
				"log": "general log details",
			},
		}
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Input: "*log r extra",
			Args:  []string{"r", "extra"},
			Spec:  commandspec.CommandSpec{Name: "*log", Handler: "dm_log"},
		}

		handler := NewDMLogHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusDoPrompt {
			t.Errorf("expected status %d, got %d", StatusDoPrompt, status)
		}
		if world.logDeleted != "" {
			t.Errorf("expected no delete with extra args, deleted %q", world.logDeleted)
		}
		if world.logRead != "log" {
			t.Errorf("expected world.ReadLogFile to be called with 'log', got %q", world.logRead)
		}
		if !strings.Contains(ctx.OutputString(), "general log details") {
			t.Errorf("expected general log contents, got %q", ctx.OutputString())
		}
	})

	t.Run("success with f (read fail log)", func(t *testing.T) {
		world := &mockDMLogWorld{
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": 14}},
			},
			logFiles: map[string]string{
				"log_fl": "fail log details\nsecond line",
			},
		}
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Input: "*log f",
			Args:  []string{"f"},
			Spec:  commandspec.CommandSpec{Name: "*log", Handler: "dm_log"},
		}

		handler := NewDMLogHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusDoPrompt {
			t.Errorf("expected status %d, got %d", StatusDoPrompt, status)
		}
		if world.logRead != "log_fl" {
			t.Errorf("expected world.ReadLogFile to be called with 'log_fl', got %q", world.logRead)
		}
		if !strings.Contains(ctx.OutputString(), "fail log details") {
			t.Errorf("expected fail log contents, got %q", ctx.OutputString())
		}
	})

	t.Run("success other (read general log)", func(t *testing.T) {
		world := &mockDMLogWorld{
			players: map[model.PlayerID]model.Player{
				"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": 13}},
			},
			logFiles: map[string]string{
				"log": "general log details",
			},
		}
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Input: "*log",
			Args:  []string{""},
			Spec:  commandspec.CommandSpec{Name: "*log", Handler: "dm_log"},
		}

		handler := NewDMLogHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusDoPrompt {
			t.Errorf("expected status %d, got %d", StatusDoPrompt, status)
		}
		if world.logRead != "log" {
			t.Errorf("expected world.ReadLogFile to be called with 'log', got %q", world.logRead)
		}
		if !strings.Contains(ctx.OutputString(), "general log details") {
			t.Errorf("expected general log contents, got %q", ctx.OutputString())
		}
	})
}

func TestDMLogPaginatesLongLogLikeViewFile(t *testing.T) {
	world := &mockDMLogWorld{
		players: map[model.PlayerID]model.Player{
			"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": 13}},
		},
		logFiles: map[string]string{
			"log": dmLogLongLines(24),
		},
	}
	var pending PendingLineHandler
	ctx := &Context{
		ActorID: "player:dm",
		Values: map[string]any{
			ContextPendingLineKey: func(handler PendingLineHandler) {
				pending = handler
			},
		},
	}
	resolved := ResolvedCommand{
		Input: "*log",
		Spec:  commandspec.CommandSpec{Name: "*log", Handler: "dm_log"},
	}

	handler := NewDMLogHandler(world)
	status, err := handler(ctx, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusDoPrompt {
		t.Errorf("first page status = %d, want %d", status, StatusDoPrompt)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "line S\n") {
		t.Fatalf("first page output missing final first-page line:\n%s", out)
	}
	if strings.Contains(out, "line T\n") {
		t.Fatalf("first page output included continuation line:\n%s", out)
	}
	if !strings.Contains(out, postReadContinuePrompt) {
		t.Fatalf("first page output missing continue prompt:\n%s", out)
	}
	if pending == nil {
		t.Fatal("pending continuation handler was not installed")
	}

	ctx.Output = nil
	status, err = pending(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusDefault {
		t.Errorf("continuation status = %d, want %d", status, StatusDefault)
	}
	out = ctx.OutputString()
	if !strings.Contains(out, "line T\n") || !strings.Contains(out, "line X\n") {
		t.Fatalf("continuation output missing remaining lines:\n%s", out)
	}
	if strings.Contains(out, postReadContinuePrompt) {
		t.Fatalf("final page output unexpectedly prompted again:\n%s", out)
	}
	if pending != nil {
		t.Fatal("pending continuation handler was not cleared after final page")
	}
}

func TestDMLogCancelPendingLogRead(t *testing.T) {
	world := &mockDMLogWorld{
		players: map[model.PlayerID]model.Player{
			"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": 13}},
		},
		logFiles: map[string]string{
			"log": dmLogLongLines(24),
		},
	}
	var pending PendingLineHandler
	ctx := &Context{
		ActorID: "player:dm",
		Values: map[string]any{
			ContextPendingLineKey: func(handler PendingLineHandler) {
				pending = handler
			},
		},
	}
	resolved := ResolvedCommand{
		Input: "*log",
		Spec:  commandspec.CommandSpec{Name: "*log", Handler: "dm_log"},
	}

	handler := NewDMLogHandler(world)
	if _, err := handler(ctx, resolved); err != nil {
		t.Fatal(err)
	}
	if pending == nil {
		t.Fatal("pending continuation handler was not installed")
	}

	ctx.Output = nil
	status, err := pending(ctx, ".")
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusDefault {
		t.Errorf("cancel status = %d, want %d", status, StatusDefault)
	}
	if got := ctx.OutputString(); got != "중단합니다.\n" {
		t.Fatalf("cancel output = %q, want 중단합니다.", got)
	}
	if pending != nil {
		t.Fatal("pending continuation handler was not cleared after cancel")
	}
}

func dmLogLongLines(n int) string {
	var builder strings.Builder
	for i := 1; i <= n; i++ {
		builder.WriteString("line ")
		builder.WriteByte(byte('A' - 1 + i))
		builder.WriteByte('\n')
	}
	return builder.String()
}
