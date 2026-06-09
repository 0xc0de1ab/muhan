package game

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"muhan/internal/commandspec"
	enginecmd "muhan/internal/engine/command"
	"muhan/internal/session"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestFamilyNewsHandlerReadsUTF8Notice(t *testing.T) {
	root := t.TempDir()
	mustWriteFamilyNews(t, root, 7, "                      === 패거리 공지 ===\n\n새 공지입니다.\n")
	world := familyNewsWorld(t, 7)
	loop := familyNewsLoop(t, world, root)
	commands := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", commands, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "패거리공지"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, commands, session.Command{Write: "                      === 패거리 공지 ===\n\n새 공지입니다.\n"})
}

func TestFamilyNewsHandlerRegistersEnglishAlias(t *testing.T) {
	root := t.TempDir()
	mustWriteFamilyNews(t, root, 7, "family notice\n")
	world := familyNewsWorld(t, 7)
	ctx := &enginecmd.Context{ActorID: "player:alice"}
	dispatcher := enginecmd.Dispatcher{
		Registry: familyNewsRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"family_news": NewFamilyNewsHandler(world, root),
		},
	}

	status, err := dispatcher.DispatchLine(ctx, "family_news")
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDoPrompt {
		t.Fatalf("family_news status = %v, want do-prompt", status)
	}
	if got := ctx.OutputString(); got != "family notice\n" {
		t.Fatalf("family_news output = %q", got)
	}
}

func TestFamilyNewsHandlerReportsMissingFile(t *testing.T) {
	world := familyNewsWorld(t, 7)
	ctx := &enginecmd.Context{ActorID: "player:alice"}

	status, err := NewFamilyNewsHandler(world, t.TempDir())(ctx, enginecmd.ResolvedCommand{})
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDoPrompt {
		t.Fatalf("status = %v, want do-prompt", status)
	}
	if got, want := ctx.OutputString(), "패거리의 공지사항이 없습니다."; got != want {
		t.Fatalf("missing file output = %q, want %q", got, want)
	}
}

func TestFamilyNewsHandlerReadsStateSidecarNotice(t *testing.T) {
	root := t.TempDir()
	mustWriteFamilyNewsSidecarRaw(t, root, 7, `{"schemaVersion":`+strconv.Itoa(state.CurrentSaveSchemaVersion)+`,"familyId":7,"content":"sidecar notice\n"}`)
	world := familyNewsWorld(t, 7)
	ctx := &enginecmd.Context{ActorID: "player:alice"}

	status, err := NewFamilyNewsHandler(world, root)(ctx, enginecmd.ResolvedCommand{})
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDoPrompt {
		t.Fatalf("sidecar read status = %v, want do-prompt", status)
	}
	if got, want := ctx.OutputString(), "sidecar notice\n"; got != want {
		t.Fatalf("sidecar read output = %q, want %q", got, want)
	}
}

func TestFamilyNewsHandlerPaginatesLongNoticeLikeLegacyViewFile(t *testing.T) {
	root := t.TempDir()
	mustWriteFamilyNews(t, root, 7, familyNewsLongLines(24))
	world := familyNewsWorld(t, 7)
	var pending enginecmd.PendingLineHandler
	ctx := &enginecmd.Context{
		ActorID: "player:alice",
		Values: map[string]any{
			enginecmd.ContextPendingLineKey: func(handler enginecmd.PendingLineHandler) {
				pending = handler
			},
		},
	}

	status, err := NewFamilyNewsHandler(world, root)(ctx, enginecmd.ResolvedCommand{})
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDoPrompt {
		t.Fatalf("first page status = %v, want do-prompt", status)
	}
	out := ctx.OutputString()
	for _, want := range []string{"line S\n", "[엔터]를 누르세요. 그만보시려면 [.]을 치세요: "} {
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
	if status != enginecmd.StatusDefault {
		t.Fatalf("cancel status = %v, want default", status)
	}
	if got := ctx.OutputString(); got != "중단합니다.\n" {
		t.Fatalf("cancel output = %q", got)
	}
	if pending != nil {
		t.Fatal("pending continuation handler was not cleared")
	}
}

func TestFamilyNewsHandlerRejectsUnsupportedSidecarSchema(t *testing.T) {
	root := t.TempDir()
	mustWriteFamilyNewsSidecarRaw(t, root, 7, `{"schemaVersion":`+strconv.Itoa(state.CurrentSaveSchemaVersion+1)+`,"familyId":7,"content":"future"}`)
	world := familyNewsWorld(t, 7)
	ctx := &enginecmd.Context{ActorID: "player:alice"}

	_, err := NewFamilyNewsHandler(world, root)(ctx, enginecmd.ResolvedCommand{})
	if err == nil || !strings.Contains(err.Error(), "unsupported future schema version") {
		t.Fatalf("future sidecar err = %v, want unsupported schema error", err)
	}
}

func TestFamilyNewsHandlerRejectsMismatchedSidecarFamilyID(t *testing.T) {
	root := t.TempDir()
	mustWriteFamilyNewsSidecarRaw(t, root, 7, `{"schemaVersion":`+strconv.Itoa(state.CurrentSaveSchemaVersion)+`,"familyId":8,"content":"wrong family"}`)
	world := familyNewsWorld(t, 7)
	ctx := &enginecmd.Context{ActorID: "player:alice"}

	_, err := NewFamilyNewsHandler(world, root)(ctx, enginecmd.ResolvedCommand{})
	if err == nil || !strings.Contains(err.Error(), "has familyID 8, want 7") {
		t.Fatalf("mismatched sidecar err = %v, want familyID mismatch error", err)
	}
}

func TestFamilyNewsHandlerReportsUnaffiliatedPlayer(t *testing.T) {
	world := familyNewsWorld(t, 7)
	setFamilyNewsCreatureStat(t, world, "creature:alice", "familyFlag", 0)
	ctx := &enginecmd.Context{ActorID: "player:alice"}

	if _, err := NewFamilyNewsHandler(world, t.TempDir())(ctx, enginecmd.ResolvedCommand{}); err != nil {
		t.Fatal(err)
	}
	if got, want := ctx.OutputString(), "당신은 패거리에 가입되어 있지 않습니다."; got != want {
		t.Fatalf("unaffiliated output = %q, want %q", got, want)
	}
}

func TestFamilyNewsHandlerReportsInvalidFamily(t *testing.T) {
	world := familyNewsWorld(t, 0)
	ctx := &enginecmd.Context{ActorID: "player:alice"}

	if _, err := NewFamilyNewsHandler(world, t.TempDir())(ctx, enginecmd.ResolvedCommand{}); err != nil {
		t.Fatal(err)
	}
	if got, want := ctx.OutputString(), "잘못된 패거리입니다.\n"; got != want {
		t.Fatalf("invalid family output = %q, want %q", got, want)
	}
}

func TestFamilyNewsHandlerReportsUnsupportedOption(t *testing.T) {
	world := familyNewsWorld(t, 7)
	ctx := &enginecmd.Context{ActorID: "player:alice"}
	resolved := enginecmd.ResolvedCommand{Args: []string{"x"}}

	status, err := NewFamilyNewsHandler(world, t.TempDir())(ctx, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusPrompt {
		t.Fatalf("unsupported option status = %v, want prompt", status)
	}
	if got, want := ctx.OutputString(), "잘못된 옵션입니다.\n"; got != want {
		t.Fatalf("unsupported option output = %q, want %q", got, want)
	}
}

func TestFamilyNewsHandlerOnlyTreatsSingleArgumentAsOption(t *testing.T) {
	root := t.TempDir()
	mustWriteFamilyNews(t, root, 7, "family notice\n")
	world := familyNewsWorld(t, 7)
	ctx := &enginecmd.Context{ActorID: "player:alice"}

	status, err := NewFamilyNewsHandler(world, root)(ctx, enginecmd.ResolvedCommand{Args: []string{"a", "extra"}})
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDoPrompt {
		t.Fatalf("multi-arg family news status = %v, want do-prompt", status)
	}
	if got, want := ctx.OutputString(), "family notice\n"; got != want {
		t.Fatalf("multi-arg family news output = %q, want %q", got, want)
	}
}

func TestFamilyNewsHandlerAppendsUTF8Notice(t *testing.T) {
	root := t.TempDir()
	world := familyNewsWorld(t, 7)
	world.SetDBRoot(root)
	var pending enginecmd.PendingLineHandler
	ctx := &enginecmd.Context{
		ActorID: "player:alice",
		Values: map[string]any{
			enginecmd.ContextPendingLineKey: func(handler enginecmd.PendingLineHandler) {
				pending = handler
			},
		},
	}

	status, err := NewFamilyNewsHandler(world, root)(ctx, enginecmd.ResolvedCommand{Args: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDoPrompt {
		t.Fatalf("append status = %v, want do-prompt", status)
	}
	if pending == nil {
		t.Fatal("append did not install pending line handler")
	}
	if got, want := ctx.OutputString(), "패거리 공지:\n->"; got != want {
		t.Fatalf("append prompt = %q, want %q", got, want)
	}

	ctx.Output = nil
	status, err = pending(ctx, "첫 번째 공지입니다")
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDoPrompt {
		t.Fatalf("line status = %v, want do-prompt", status)
	}
	if got, want := ctx.OutputString(), "->"; got != want {
		t.Fatalf("line prompt = %q, want %q", got, want)
	}
	if pending == nil {
		t.Fatal("line handler was cleared before completion")
	}

	path := familyNewsPath(root, 7)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), familyNewsHeader+"첫 번째 공지입니다\n"; got != want {
		t.Fatalf("family news file = %q, want %q", got, want)
	}
	loaded, ok, err := state.LoadFamilyNews(root, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("family news sidecar was not saved")
	}
	if got, want := loaded.Content, familyNewsHeader+"첫 번째 공지입니다\n"; got != want {
		t.Fatalf("sidecar content = %q, want %q", got, want)
	}

	ctx.Output = nil
	status, err = pending(ctx, ".")
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDefault {
		t.Fatalf("finish status = %v, want default", status)
	}
	if pending != nil {
		t.Fatal("finish did not clear pending line handler")
	}
	if got, want := ctx.OutputString(), "공지를 남겼습니다.\n"; got != want {
		t.Fatalf("finish output = %q, want %q", got, want)
	}
}

func TestFamilyNewsHandlerRewritesLegacyEncodingOnAppend(t *testing.T) {
	root := t.TempDir()
	world := familyNewsWorld(t, 7)
	var pending enginecmd.PendingLineHandler
	ctx := &enginecmd.Context{
		ActorID: "player:alice",
		Values: map[string]any{
			enginecmd.ContextPendingLineKey: func(handler enginecmd.PendingLineHandler) {
				pending = handler
			},
		},
	}
	dir := filepath.Join(root, "player", "family")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyBytes := []byte{0xc6, 0xd0, 0xb0, 0xc5, 0xb8, 0xae, 0x20, 0xb0, 0xf8, 0xc1, 0xf6, 0x0a}
	if err := os.WriteFile(filepath.Join(dir, "family_news_7"), legacyBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := NewFamilyNewsHandler(world, root)(ctx, enginecmd.ResolvedCommand{Args: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	if pending == nil {
		t.Fatal("append did not install pending line handler")
	}
	ctx.Output = nil
	if _, err := pending(ctx, "새 줄"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "family_news_7"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "패거리 공지\n새 줄\n"; got != want {
		t.Fatalf("rewritten family news = %q, want %q", got, want)
	}
}

func TestFamilyNewsHandlerDeletesNoticeOnlyForBoss(t *testing.T) {
	root := t.TempDir()
	world := familyNewsWorld(t, 7)
	mustWriteFamilyNews(t, root, 7, "family notice\n")

	ctx := &enginecmd.Context{ActorID: "player:alice"}
	status, err := NewFamilyNewsHandler(world, root)(ctx, enginecmd.ResolvedCommand{Args: []string{"d"}})
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusPrompt {
		t.Fatalf("non-boss delete status = %v, want prompt", status)
	}
	if got, want := ctx.OutputString(), "삭제는 두목만 가능합니다."; got != want {
		t.Fatalf("non-boss delete output = %q, want %q", got, want)
	}
	if _, err := os.Stat(familyNewsPath(root, 7)); err != nil {
		t.Fatalf("non-boss delete removed file: %v", err)
	}

	setFamilyNewsCreatureStat(t, world, "creature:alice", "PFMBOS", 1)
	ctx = &enginecmd.Context{ActorID: "player:alice"}
	status, err = NewFamilyNewsHandler(world, root)(ctx, enginecmd.ResolvedCommand{Args: []string{"d"}})
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusPrompt {
		t.Fatalf("boss delete status = %v, want prompt", status)
	}
	if got, want := ctx.OutputString(), "공지 내용을 지웠습니다.\n"; got != want {
		t.Fatalf("boss delete output = %q, want %q", got, want)
	}
	if _, err := os.Stat(familyNewsPath(root, 7)); !os.IsNotExist(err) {
		t.Fatalf("boss delete file stat err = %v, want not exist", err)
	}
}

func familyNewsLoop(t *testing.T, world FamilyWorld, root string) *Loop {
	t.Helper()
	return NewLoop(enginecmd.Dispatcher{
		Registry: familyNewsRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"family_news": NewFamilyNewsHandler(world, root),
		},
	})
}

func familyNewsRegistry(t *testing.T) commandspec.Registry {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "패거리공지", Number: 148, Handler: "family_news"},
		{Name: "family_news", Number: 148, Handler: "family_news"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func familyNewsWorld(t *testing.T, familyID int) *state.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddFamilyNewsRoom(t, loaded, model.Room{ID: "room:one", DisplayName: "One"})
	mustAddFamilyNewsPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:one",
	})
	mustAddFamilyNewsCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:one",
		Stats:       map[string]int{"familyFlag": 1, "familyID": familyID},
	})
	return state.NewWorld(loaded)
}

func mustWriteFamilyNews(t *testing.T, root string, familyID int, text string) {
	t.Helper()
	dir := filepath.Join(root, "player", "family")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "family_news_"+strconv.Itoa(familyID)), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func familyNewsLongLines(n int) string {
	var builder strings.Builder
	for i := 1; i <= n; i++ {
		builder.WriteString("line ")
		builder.WriteByte(byte('A' - 1 + i))
		builder.WriteByte('\n')
	}
	return builder.String()
}

func mustWriteFamilyNewsSidecarRaw(t *testing.T, root string, familyID int, text string) {
	t.Helper()
	dir := filepath.Join(root, "player", "family", "json")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "family_news_"+strconv.Itoa(familyID)+".json"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustAddFamilyNewsRoom(t *testing.T, world *worldload.World, room model.Room) {
	t.Helper()
	if err := world.AddRoom(room); err != nil {
		t.Fatal(err)
	}
}

func mustAddFamilyNewsPlayer(t *testing.T, world *worldload.World, player model.Player) {
	t.Helper()
	if err := world.AddPlayer(player); err != nil {
		t.Fatal(err)
	}
}

func mustAddFamilyNewsCreature(t *testing.T, world *worldload.World, creature model.Creature) {
	t.Helper()
	if err := world.AddCreature(creature); err != nil {
		t.Fatal(err)
	}
}

func setFamilyNewsCreatureStat(t *testing.T, world *state.World, creatureID model.CreatureID, key string, value int) {
	t.Helper()
	if err := world.SetCreatureStat(creatureID, key, value); err != nil {
		t.Fatal(err)
	}
}
