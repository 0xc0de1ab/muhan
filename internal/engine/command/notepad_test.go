package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockNotepadWorld struct {
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
}

func (m *mockNotepadWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockNotepadWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func notepadTestContext(pending *PendingLineHandler, actorID string) *Context {
	return &Context{
		ActorID: actorID,
		Values: map[string]any{
			ContextPendingLineKey: func(handler PendingLineHandler) {
				*pending = handler
			},
		},
	}
}

func TestNotepad_Unauthorized(t *testing.T) {
	world := &mockNotepadWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 1}}, // Ordinary class
		},
	}
	ctx := &Context{ActorID: "player:alice"}
	handler := NewNotepadHandler(world, t.TempDir())
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusPrompt {
		t.Errorf("expected StatusPrompt, got %v", status)
	}
	if ctx.OutputString() != "" {
		t.Errorf("expected no output, got %q", ctx.OutputString())
	}
}

func TestNotepad_CRUD(t *testing.T) {
	tempDir := t.TempDir()
	world := &mockNotepadWorld{
		players: map[model.PlayerID]model.Player{
			"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": 10}}, // Caretaker
		},
	}

	var pending PendingLineHandler
	ctx := notepadTestContext(&pending, "player:dm")
	handler := NewNotepadHandler(world, tempDir)

	// 1. View empty notepad (file does not exist)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("unexpected view error: %v", err)
	}
	if status != StatusDoPrompt {
		t.Errorf("expected StatusDoPrompt, got %v", status)
	}
	if got := ctx.OutputString(); got != "" {
		t.Errorf("expected empty output, got %q", got)
	}

	// 2. Append option start
	ctx = notepadTestContext(&pending, "player:dm")
	status, err = handler(ctx, ResolvedCommand{Args: []string{"a"}})
	if err != nil {
		t.Fatalf("unexpected append start error: %v", err)
	}
	if status != StatusDoPrompt {
		t.Errorf("expected StatusDoPrompt, got %v", status)
	}
	if pending == nil {
		t.Fatal("expected pending line handler to be set")
	}
	if got := ctx.OutputString(); got != "DM notepad:\n->" {
		t.Errorf("expected prompt output, got %q", got)
	}

	// 3. Append first line
	ctx = notepadTestContext(&pending, "player:dm")
	status, err = pending(ctx, "line1")
	if err != nil {
		t.Fatalf("unexpected append line1 error: %v", err)
	}
	if status != StatusDoPrompt {
		t.Errorf("expected StatusDoPrompt, got %v", status)
	}
	if pending == nil {
		t.Fatal("expected pending line handler to remain set")
	}
	if got := ctx.OutputString(); got != "->" {
		t.Errorf("expected prompt ->, got %q", got)
	}

	// 4. Append second line (very long line to test truncation)
	longLine := strings.Repeat("a", 100)
	ctx = notepadTestContext(&pending, "player:dm")
	status, err = pending(ctx, longLine)
	if err != nil {
		t.Fatalf("unexpected append line2 error: %v", err)
	}
	if status != StatusDoPrompt {
		t.Errorf("expected StatusDoPrompt, got %v", status)
	}
	if pending == nil {
		t.Fatal("expected pending line handler to remain set")
	}
	if got := ctx.OutputString(); got != "->" {
		t.Errorf("expected prompt ->, got %q", got)
	}

	// 5. Append "." to finish
	ctx = notepadTestContext(&pending, "player:dm")
	status, err = pending(ctx, ".")
	if err != nil {
		t.Fatalf("unexpected append finish error: %v", err)
	}
	if status != StatusDefault {
		t.Errorf("expected StatusDefault, got %v", status)
	}
	if pending != nil {
		t.Fatal("expected pending line handler to be cleared")
	}
	if got := ctx.OutputString(); got != "Message appended.\n" {
		t.Errorf("expected finish message, got %q", got)
	}

	// 6. View notepad content
	ctx = notepadTestContext(&pending, "player:dm")
	status, err = handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("unexpected view error: %v", err)
	}
	if status != StatusDoPrompt {
		t.Errorf("expected StatusDoPrompt, got %v", status)
	}
	expectedContent := "            === DM Notepad ===\n\nline1\n" + strings.Repeat("a", 79) + "\n"
	if got := ctx.OutputString(); got != expectedContent {
		t.Errorf("expected notepad content %q, got %q", expectedContent, got)
	}

	// 7. Delete option
	ctx = notepadTestContext(&pending, "player:dm")
	status, err = handler(ctx, ResolvedCommand{Args: []string{"d"}})
	if err != nil {
		t.Fatalf("unexpected delete error: %v", err)
	}
	if status != StatusPrompt {
		t.Errorf("expected StatusPrompt, got %v", status)
	}
	if got := ctx.OutputString(); got != "Clearing DM notepad\n" {
		t.Errorf("expected delete message, got %q", got)
	}

	// 8. View empty notepad again after delete
	ctx = notepadTestContext(&pending, "player:dm")
	status, err = handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("unexpected view error: %v", err)
	}
	if status != StatusDoPrompt {
		t.Errorf("expected StatusDoPrompt, got %v", status)
	}
	if got := ctx.OutputString(); got != "" {
		t.Errorf("expected empty output after delete, got %q", got)
	}

	// 9. Invalid option
	ctx = notepadTestContext(&pending, "player:dm")
	status, err = handler(ctx, ResolvedCommand{Args: []string{"xyz"}})
	if err != nil {
		t.Fatalf("unexpected invalid option error: %v", err)
	}
	if status != StatusPrompt {
		t.Errorf("expected StatusPrompt, got %v", status)
	}
	if got := ctx.OutputString(); got != "invalid option.\n" {
		t.Errorf("expected invalid option message, got %q", got)
	}
}

func TestNotepadPaginatesPlainReadLikeLegacyViewFile(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "post"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "post", "DM_pad"), []byte(helpLongLines(24)), 0o644); err != nil {
		t.Fatal(err)
	}
	world := &mockNotepadWorld{
		players: map[model.PlayerID]model.Player{
			"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": model.ClassCaretaker}},
		},
	}

	var pending PendingLineHandler
	ctx := notepadTestContext(&pending, "player:dm")
	status, err := NewNotepadHandler(world, tempDir)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDoPrompt {
		t.Fatalf("first page status = %d, want StatusDoPrompt", status)
	}
	out := ctx.OutputString()
	for _, want := range []string{"line S\n", postReadContinuePrompt} {
		if !strings.Contains(out, want) {
			t.Fatalf("first page output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "line T\n") {
		t.Fatalf("first page output included continuation line:\n%s", out)
	}
	if pending == nil {
		t.Fatal("pending continuation handler was not installed")
	}

	ctx.Output = nil
	status, err = pending(ctx, ".")
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusDefault {
		t.Fatalf("cancel status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "중단합니다.\n" {
		t.Fatalf("cancel output = %q", got)
	}
	if pending != nil {
		t.Fatal("pending continuation handler was not cleared")
	}
}

func TestNotepadDeleteIgnoresUnlinkFailureLikeC(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "post"), []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("seed post path: %v", err)
	}
	world := &mockNotepadWorld{
		players: map[model.PlayerID]model.Player{
			"player:dm": {ID: "player:dm", CreatureID: "creature:dm"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": model.ClassCaretaker}},
		},
	}
	ctx := &Context{ActorID: "player:dm"}

	status, err := NewNotepadHandler(world, tempDir)(ctx, ResolvedCommand{Args: []string{"d"}})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if status != StatusPrompt {
		t.Fatalf("status = %v, want StatusPrompt", status)
	}
	if got := ctx.OutputString(); got != "Clearing DM notepad\n" {
		t.Fatalf("output = %q, want legacy delete message", got)
	}
}
