package game

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"muhan/internal/commandparse"
	enginecmd "muhan/internal/engine/command"
	"muhan/internal/world/model"
)

func TestMemoHandlerWritesLegacyStyleTargetMemo(t *testing.T) {
	root := t.TempDir()
	world := memoTestWorld("Alice", "Bob")
	now := func() time.Time {
		return time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	}
	ctx := memoTestContext(nil, "Alice")

	status, err := newMemoHandler(world, root, now)(ctx, enginecmd.ResolvedCommand{Args: []string{"bob", "확인", "바람"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != enginecmd.StatusDefault || ctx.OutputString() != "메모를 남겼습니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}

	data, err := os.ReadFile(filepath.Join(root, "player", "fal", "Bob"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	want := formatMemoLegacyCTime(now()) + " 에 [Alice] 님이 남기신 메모 : \n>>>>> 확인 바람\n"
	if text != want {
		t.Fatalf("target memo file = %q, want %q", text, want)
	}
}

func TestMemoHandlerPreservesLegacyCutCommandSpaces(t *testing.T) {
	root := t.TempDir()
	world := memoTestWorld("Alice", "Bob")
	now := func() time.Time {
		return time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	}
	ctx := memoTestContext(nil, "Alice")
	input := "Bob 확인   메모"

	status, err := newMemoHandler(world, root, now)(ctx, enginecmd.ResolvedCommand{
		Input:  input,
		Parsed: commandparse.Parse(input),
		Args:   []string{"Bob", "확인"},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != enginecmd.StatusDefault || ctx.OutputString() != "메모를 남겼습니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}

	data, err := os.ReadFile(filepath.Join(root, "player", "fal", "Bob"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	want := formatMemoLegacyCTime(now()) + " 에 [Alice] 님이 남기신 메모 : \n>>>>> 확인  \n"
	if text != want {
		t.Fatalf("target memo file = %q, want %q", text, want)
	}
}

func TestMemoHandlerFindsRuntimePlayerIDFromLegacyName(t *testing.T) {
	root := t.TempDir()
	world := memoPlayerOnlyWorld{
		"player:alice": {
			ID:          "player:alice",
			DisplayName: "Alice",
		},
		"player:bob": {
			ID:          "player:bob",
			DisplayName: "Bob",
		},
	}
	now := func() time.Time {
		return time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	}
	ctx := memoTestContext(nil, "player:alice")

	status, err := newMemoHandler(world, root, now)(ctx, enginecmd.ResolvedCommand{Args: []string{"bob", "확인"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != enginecmd.StatusDefault || ctx.OutputString() != "메모를 남겼습니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}

	data, err := os.ReadFile(filepath.Join(root, "player", "fal", "Bob"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	want := formatMemoLegacyCTime(now()) + " 에 [Alice] 님이 남기신 메모 : \n>>>>> 확인\n"
	if text != want {
		t.Fatalf("target memo file = %q, want %q", text, want)
	}
}

func TestMemoHandlerFindsLegacyDisplayNameWhenRuntimeIDDiverges(t *testing.T) {
	root := t.TempDir()
	world := memoWorld{
		"runtime-alice": {
			ID:          "runtime-alice",
			DisplayName: "Alice",
		},
		"runtime-bob": {
			ID:          "runtime-bob",
			DisplayName: "Bob",
		},
	}
	now := func() time.Time {
		return time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	}
	ctx := memoTestContext(nil, "runtime-alice")

	status, err := newMemoHandler(world, root, now)(ctx, enginecmd.ResolvedCommand{Args: []string{"bob", "확인"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != enginecmd.StatusDefault || ctx.OutputString() != "메모를 남겼습니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}

	data, err := os.ReadFile(filepath.Join(root, "player", "fal", "Bob"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	want := formatMemoLegacyCTime(now()) + " 에 [Alice] 님이 남기신 메모 : \n>>>>> 확인\n"
	if text != want {
		t.Fatalf("target memo file = %q, want %q", text, want)
	}
}

func TestMemoHandlerMatchesLegacyTargetFailures(t *testing.T) {
	root := t.TempDir()
	world := memoTestWorld("Alice", "Bob")
	handler := newMemoHandler(world, root, time.Now)

	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing all", args: nil, want: "누구에게 어떤 메모를 남기시려고요?\n"},
		{name: "missing message", args: []string{"Bob"}, want: "누구에게 어떤 메모를 남기시려고요?\n"},
		{name: "missing target", args: []string{"Nobody", "확인"}, want: "그런 사용자는 없습니다.\n"},
		{name: "mixed case target preserves rest of name", args: []string{"bOB", "확인"}, want: "그런 사용자는 없습니다.\n"},
		{name: "too long", args: []string{"Bob", strings.Repeat("x", 78)}, want: "메모의 내용이 너무 깁니다."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := memoTestContext(nil, "Alice")
			status, err := handler(ctx, enginecmd.ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if status != enginecmd.StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, "player", "fal", "Bob")); !os.IsNotExist(err) {
		t.Fatalf("target memo file stat error = %v, want not exist", err)
	}
}

func TestMemoHandlerTreatsReadWriteDeleteAsLegacyTargetNames(t *testing.T) {
	root := t.TempDir()
	world := memoTestWorld("Alice")
	handler := newMemoHandler(world, root, time.Now)

	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "read without message", args: []string{"읽기"}, want: "누구에게 어떤 메모를 남기시려고요?\n"},
		{name: "write is missing target", args: []string{"쓰기", "내용"}, want: "그런 사용자는 없습니다.\n"},
		{name: "delete is missing target", args: []string{"삭제", "내용"}, want: "그런 사용자는 없습니다.\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := memoTestContext(nil, "Alice")
			status, err := handler(ctx, enginecmd.ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if status != enginecmd.StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}

	if _, err := os.Stat(filepath.Join(root, "player", "fal", "Alice")); !os.IsNotExist(err) {
		t.Fatalf("memo file stat error = %v, want not exist", err)
	}
}

type memoWorld map[model.PlayerID]model.Player

func (w memoWorld) Player(id model.PlayerID) (model.Player, bool) {
	player, ok := w[id]
	return player, ok
}

func (w memoWorld) Players() []model.Player {
	players := make([]model.Player, 0, len(w))
	for _, player := range w {
		players = append(players, player)
	}
	return players
}

type memoPlayerOnlyWorld map[model.PlayerID]model.Player

func (w memoPlayerOnlyWorld) Player(id model.PlayerID) (model.Player, bool) {
	player, ok := w[id]
	return player, ok
}

func memoTestWorld(names ...string) memoWorld {
	world := memoWorld{}
	for _, name := range names {
		id := model.PlayerID(name)
		world[id] = model.Player{
			ID:          id,
			DisplayName: name,
		}
	}
	return world
}

func memoTestContext(pending *enginecmd.PendingLineHandler, actorID string) *enginecmd.Context {
	ctx := &enginecmd.Context{ActorID: actorID}
	if pending != nil {
		ctx.Values = map[string]any{
			enginecmd.ContextPendingLineKey: func(handler enginecmd.PendingLineHandler) {
				*pending = handler
			},
		}
	}
	return ctx
}
