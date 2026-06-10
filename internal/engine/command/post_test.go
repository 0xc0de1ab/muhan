package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"muhan/internal/commandspec"
	"muhan/internal/persist/legacykr"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestPostSendHandlerWritesPendingMail(t *testing.T) {
	root := t.TempDir()
	world := state.NewWorld(postTestWorld(t, true))
	defer world.Close()
	var pending PendingLineHandler
	ctx := postTestContext(&pending, "Alice")
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "편지보내기", Number: 53, Handler: "postsend"},
		}),
		Handlers: map[string]Handler{
			"postsend": newPostSendHandler(world, root, func() time.Time {
				return time.Date(2026, 5, 24, 12, 34, 56, 0, time.UTC)
			}),
		},
	}

	status, err := dispatcher.DispatchLine(ctx, "bob 편지보내기")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDoPrompt {
		t.Fatalf("status = %d, want StatusDoPrompt", status)
	}
	if pending == nil {
		t.Fatal("pending handler was not installed")
	}
	if got := ctx.OutputString(); !strings.Contains(got, "편지 내용을 입력하십시요.") || !strings.HasSuffix(got, "-: ") {
		t.Fatalf("initial output = %q", got)
	}

	ctx.Output = nil
	status, err = pending(ctx, "첫 줄")
	if err != nil {
		t.Fatalf("first pending line error: %v", err)
	}
	if status != StatusDoPrompt || ctx.OutputString() != ": " {
		t.Fatalf("first pending status/output = %d/%q", status, ctx.OutputString())
	}

	ctx.Output = nil
	status, err = pending(ctx, "둘째 줄")
	if err != nil {
		t.Fatalf("second pending line error: %v", err)
	}
	if status != StatusDoPrompt || ctx.OutputString() != ": " {
		t.Fatalf("second pending status/output = %d/%q", status, ctx.OutputString())
	}

	ctx.Output = nil
	status, err = pending(ctx, ".")
	if err != nil {
		t.Fatalf("finish pending line error: %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "편지를 보냈습니다.\n" {
		t.Fatalf("finish pending status/output = %d/%q", status, ctx.OutputString())
	}
	if pending != nil {
		t.Fatal("pending handler was not cleared")
	}

	data, err := os.ReadFile(filepath.Join(root, "post", "Bob"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"\n---\nAlice (", ")님에게서의 편지:\n\n", "첫 줄\n", "둘째 줄\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("mail file missing %q:\n%s", want, text)
		}
	}
}

func TestPostSendHandlerDotFirstDoesNotCreateMail(t *testing.T) {
	root := t.TempDir()
	world := state.NewWorld(postTestWorld(t, true))
	defer world.Close()
	var pending PendingLineHandler
	ctx := postTestContext(&pending, "Alice")
	handler := NewPostSendHandler(world, root)

	status, err := handler(ctx, ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDoPrompt || pending == nil {
		t.Fatalf("status/pending = %d/%v, want edit mode", status, pending != nil)
	}

	ctx.Output = nil
	status, err = pending(ctx, ".취소")
	if err != nil {
		t.Fatalf("pending line error: %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "편지를 보냈습니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	if _, err := os.Stat(filepath.Join(root, "post", "Bob")); !os.IsNotExist(err) {
		t.Fatalf("post file stat error = %v, want not exist", err)
	}
}

func TestPostSendHandlerPersistsEachLineBeforeFinalDotLikeC(t *testing.T) {
	root := t.TempDir()
	world := state.NewWorld(postTestWorld(t, true))
	defer world.Close()
	var pending PendingLineHandler
	ctx := postTestContext(&pending, "Alice")
	handler := newPostSendHandler(world, root, func() time.Time {
		return time.Date(2026, 6, 6, 10, 30, 0, 0, time.UTC)
	})

	status, err := handler(ctx, ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDoPrompt || pending == nil {
		t.Fatalf("status/pending = %d/%v, want edit mode", status, pending != nil)
	}

	ctx.Output = nil
	status, err = pending(ctx, "첫 줄")
	if err != nil {
		t.Fatalf("first pending line error: %v", err)
	}
	if status != StatusDoPrompt || pending == nil || ctx.OutputString() != ": " {
		t.Fatalf("first pending status=%d pending=%v output=%q", status, pending != nil, ctx.OutputString())
	}
	postPath := filepath.Join(root, "post", "Bob")
	data, err := os.ReadFile(postPath)
	if err != nil {
		t.Fatalf("post file should exist after first line like C postedit: %v", err)
	}
	text := string(data)
	for _, want := range []string{"\n---\nAlice (", ")님에게서의 편지:\n\n", "첫 줄\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("post after first line missing %q:\n%s", want, text)
		}
	}

	ctx.Output = nil
	status, err = pending(ctx, "둘째 줄")
	if err != nil {
		t.Fatalf("second pending line error: %v", err)
	}
	if status != StatusDoPrompt || pending == nil || ctx.OutputString() != ": " {
		t.Fatalf("second pending status=%d pending=%v output=%q", status, pending != nil, ctx.OutputString())
	}
	data, err = os.ReadFile(postPath)
	if err != nil {
		t.Fatalf("post file read after second line: %v", err)
	}
	text = string(data)
	for _, want := range []string{"첫 줄\n", "둘째 줄\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("post before final dot missing %q:\n%s", want, text)
		}
	}
}

func TestPostReadAndDeleteHandlers(t *testing.T) {
	root := t.TempDir()
	postDir := filepath.Join(root, "post")
	if err := os.MkdirAll(postDir, 0o700); err != nil {
		t.Fatal(err)
	}
	encoded, err := legacykr.EncodeEUCKR("기존 편지\n")
	if err != nil {
		t.Fatal(err)
	}
	postPath := filepath.Join(postDir, "Alice")
	if err := os.WriteFile(postPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	world := state.NewWorld(postTestWorld(t, true))
	defer world.Close()
	read := NewPostReadHandler(world, root)
	ctx := &Context{ActorID: "Alice"}
	status, err := read(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("read handler error: %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "기존 편지\n" {
		t.Fatalf("read status/output = %d/%q", status, ctx.OutputString())
	}

	deleteHandler := NewPostDeleteHandler(world, root)
	ctx = &Context{ActorID: "Alice"}
	status, err = deleteHandler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("delete handler error: %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "편지가 삭제되었습니다.\n" {
		t.Fatalf("delete status/output = %d/%q", status, ctx.OutputString())
	}
	if _, err := os.Stat(postPath); !os.IsNotExist(err) {
		t.Fatalf("post file stat error = %v, want not exist", err)
	}
}

func TestPostReadHandlerPaginatesLongMail(t *testing.T) {
	root := t.TempDir()
	postDir := filepath.Join(root, "post")
	if err := os.MkdirAll(postDir, 0o700); err != nil {
		t.Fatal(err)
	}
	var builder strings.Builder
	for i := 1; i <= 24; i++ {
		builder.WriteString("line ")
		builder.WriteString(string(rune('A' - 1 + i)))
		builder.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(postDir, "Alice"), []byte(builder.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	world := state.NewWorld(postTestWorld(t, true))
	defer world.Close()
	var pending PendingLineHandler
	ctx := postTestContext(&pending, "Alice")
	status, err := NewPostReadHandler(world, root)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("read handler error: %v", err)
	}
	if status != StatusDoPrompt || pending == nil {
		t.Fatalf("status/pending = %d/%v, want paged read", status, pending != nil)
	}
	got := ctx.OutputString()
	if !strings.Contains(got, "line S\n") || strings.Contains(got, "line T\n") {
		t.Fatalf("first page output = %q", got)
	}
	if !strings.HasSuffix(got, postReadContinuePrompt) {
		t.Fatalf("first page prompt = %q", got)
	}

	ctx.Output = nil
	status, err = pending(ctx, "")
	if err != nil {
		t.Fatalf("continue pending line error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("continue status = %d, want StatusDefault", status)
	}
	got = ctx.OutputString()
	if !strings.Contains(got, "line T\n") || !strings.Contains(got, "line X\n") || strings.Contains(got, postReadContinuePrompt) {
		t.Fatalf("second page output = %q", got)
	}
	if pending != nil {
		t.Fatal("pending handler was not cleared")
	}
}

func TestPostReadHandlerCancelsPagination(t *testing.T) {
	root := t.TempDir()
	postDir := filepath.Join(root, "post")
	if err := os.MkdirAll(postDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(postDir, "Alice"), []byte(strings.Repeat("긴 편지\n", 30)), 0o600); err != nil {
		t.Fatal(err)
	}

	world := state.NewWorld(postTestWorld(t, true))
	defer world.Close()
	var pending PendingLineHandler
	ctx := postTestContext(&pending, "Alice")
	status, err := NewPostReadHandler(world, root)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("read handler error: %v", err)
	}
	if status != StatusDoPrompt || pending == nil {
		t.Fatalf("status/pending = %d/%v, want paged read", status, pending != nil)
	}

	ctx.Output = nil
	status, err = pending(ctx, ".그만")
	if err != nil {
		t.Fatalf("cancel pending line error: %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "중단합니다.\n" {
		t.Fatalf("cancel status/output = %d/%q", status, ctx.OutputString())
	}
	if pending != nil {
		t.Fatal("pending handler was not cleared")
	}
}

func TestPostReadHandlerPaginatesLongUTF8Line(t *testing.T) {
	root := t.TempDir()
	postDir := filepath.Join(root, "post")
	if err := os.MkdirAll(postDir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("가", 300) + "\n끝\n"
	if err := os.WriteFile(filepath.Join(postDir, "Alice"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	world := state.NewWorld(postTestWorld(t, true))
	defer world.Close()
	var pending PendingLineHandler
	ctx := postTestContext(&pending, "Alice")
	status, err := NewPostReadHandler(world, root)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("read handler error: %v", err)
	}
	if status != StatusDoPrompt || pending == nil {
		t.Fatalf("status/pending = %d/%v, want paged read", status, pending != nil)
	}
	if got := ctx.OutputString(); !utf8.ValidString(got) || strings.Contains(got, "끝") {
		t.Fatalf("first UTF-8 page valid=%v output=%q", utf8.ValidString(got), got)
	}

	ctx.Output = nil
	status, err = pending(ctx, "")
	if err != nil {
		t.Fatalf("continue pending line error: %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "끝\n") {
		t.Fatalf("second UTF-8 page status/output = %d/%q", status, ctx.OutputString())
	}
}

func TestPostHandlersRequirePostOffice(t *testing.T) {
	root := t.TempDir()
	world := state.NewWorld(postTestWorld(t, false))
	defer world.Close()

	ctx := &Context{ActorID: "Alice"}
	if _, err := NewPostSendHandler(world, root)(ctx, ResolvedCommand{Args: []string{"Bob"}}); err != nil {
		t.Fatalf("send handler error: %v", err)
	}
	if got := ctx.OutputString(); got != "여기는 우체국이 아닙니다.\n" {
		t.Fatalf("send output = %q", got)
	}

	ctx = &Context{ActorID: "Alice"}
	if _, err := NewPostReadHandler(world, root)(ctx, ResolvedCommand{}); err != nil {
		t.Fatalf("read handler error: %v", err)
	}
	if got := ctx.OutputString(); got != "이곳은 우체국이 아닙니다.\n" {
		t.Fatalf("read output = %q", got)
	}

	ctx = &Context{ActorID: "Alice"}
	if _, err := NewPostDeleteHandler(world, root)(ctx, ResolvedCommand{}); err != nil {
		t.Fatalf("delete handler error: %v", err)
	}
	if got := ctx.OutputString(); got != "이곳은 우체국이 아닙니다.\n" {
		t.Fatalf("delete output = %q", got)
	}
}

func TestPostPathSafetyAndLineTruncation(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"../Alice", "a/b", `a\b`, "", " Alice"} {
		if path, ok := safePostPath(root, name); ok {
			t.Fatalf("safePostPath(%q) = %q, true; want false", name, path)
		}
	}
	if path, ok := safePostPath(root, "Alice"); !ok || filepath.Base(path) != "Alice" {
		t.Fatalf("safePostPath(Alice) = %q, %v", path, ok)
	}

	got := firstNBytes("가나다", 5)
	if got != "가" || !utf8.ValidString(got) {
		t.Fatalf("firstNBytes multibyte = %q valid=%v, want one valid rune", got, utf8.ValidString(got))
	}
	got = firstNBytes(strings.Repeat("a", 90), 79)
	if len(got) != 79 {
		t.Fatalf("firstNBytes ascii len = %d, want 79", len(got))
	}
}

func postTestWorld(t *testing.T, postOffice bool) *worldload.World {
	t.Helper()
	loaded := worldload.NewWorld()
	room := model.Room{
		ID:          "room:post",
		DisplayName: "우체국",
	}
	if postOffice {
		room.Metadata = model.Metadata{Tags: []string{"postOffice"}}
	}
	mustAddLookRoom(t, loaded, room)
	for _, spec := range []struct {
		name string
	}{
		{"Alice"},
		{"Bob"},
	} {
		playerID := model.PlayerID(spec.name)
		creatureID := model.CreatureID("creature:" + spec.name)
		mustAddLookPlayer(t, loaded, model.Player{
			ID:          playerID,
			DisplayName: spec.name,
			CreatureID:  creatureID,
			RoomID:      "room:post",
		})
		mustAddLookCreature(t, loaded, model.Creature{
			ID:          creatureID,
			Kind:        model.CreatureKindPlayer,
			DisplayName: spec.name,
			PlayerID:    playerID,
			RoomID:      "room:post",
		})
	}
	return loaded
}

func postTestContext(pending *PendingLineHandler, actorID string) *Context {
	return &Context{
		ActorID: actorID,
		Values: map[string]any{
			ContextPendingLineKey: func(handler PendingLineHandler) {
				*pending = handler
			},
		},
	}
}
