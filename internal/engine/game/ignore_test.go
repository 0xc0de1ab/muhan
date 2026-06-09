package game

import (
	"testing"

	"muhan/internal/commandspec"
	enginecmd "muhan/internal/engine/command"
	"muhan/internal/session"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestIgnoreHandlerAddsActiveTarget(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	memory := NewIgnoreMemory()
	ctx := ignoreTestContext("s1", "player:alice")

	status, err := NewIgnoreHandler(world, memory)(ctx, enginecmd.ResolvedCommand{Args: []string{"bob"}})
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if got, want := ctx.OutputString(), "Bob님을 이야기 듣기 거부 대상에 추가합니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if !memory.Ignored("s1", "Bob") {
		t.Fatal("Bob was not added to Alice ignore list")
	}
}

func TestIgnoreHandlerRemovesExistingTarget(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	memory := NewIgnoreMemory()
	memory.Add("s1", "Bob")
	ctx := ignoreTestContext("s1", "player:alice")

	status, err := NewIgnoreHandler(world, memory)(ctx, enginecmd.ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if got, want := ctx.OutputString(), "Bob님을 이야기 듣기 거부 대상에서 삭제합니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if memory.Ignored("s1", "Bob") {
		t.Fatal("Bob remained on Alice ignore list")
	}
}

func TestIgnoreHandlerListsIgnoredTargets(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	memory := NewIgnoreMemory()
	handler := NewIgnoreHandler(world, memory)

	emptyCtx := ignoreTestContext("s1", "player:alice")
	if _, err := handler(emptyCtx, enginecmd.ResolvedCommand{}); err != nil {
		t.Fatal(err)
	}
	if got, want := emptyCtx.OutputString(), "듣기 거부된 사용자: 없음.\n"; got != want {
		t.Fatalf("empty output = %q, want %q", got, want)
	}

	memory.Add("s1", "Bob")
	memory.Add("s1", "Charlie")
	listCtx := ignoreTestContext("s1", "player:alice")
	if _, err := handler(listCtx, enginecmd.ResolvedCommand{}); err != nil {
		t.Fatal(err)
	}
	if got, want := listCtx.OutputString(), "듣기 거부된 사용자: Charlie, Bob.\n"; got != want {
		t.Fatalf("list output = %q, want %q", got, want)
	}
}

func TestIgnoreHandlerKeepsIgnoreListPerSession(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	memory := NewIgnoreMemory()
	handler := NewIgnoreHandler(world, memory)

	addCtx := ignoreTestContext("s1", "player:alice")
	if _, err := handler(addCtx, enginecmd.ResolvedCommand{Args: []string{"Bob"}}); err != nil {
		t.Fatal(err)
	}

	reconnectCtx := ignoreTestContext("s9", "player:alice")
	if _, err := handler(reconnectCtx, enginecmd.ResolvedCommand{}); err != nil {
		t.Fatal(err)
	}
	if got, want := reconnectCtx.OutputString(), "듣기 거부된 사용자: 없음.\n"; got != want {
		t.Fatalf("reconnect output = %q, want %q", got, want)
	}
}

func TestIgnoreHandlerAddsSelfWhenActive(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	memory := NewIgnoreMemory()
	ctx := ignoreTestContext("s1", "player:alice")

	if _, err := NewIgnoreHandler(world, memory)(ctx, enginecmd.ResolvedCommand{Args: []string{"Alice"}}); err != nil {
		t.Fatal(err)
	}
	if got, want := ctx.OutputString(), "Alice님을 이야기 듣기 거부 대상에 추가합니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if !memory.Ignored("s1", "Alice") {
		t.Fatal("Alice was not added to her own ignore list")
	}
}

func TestIgnoreHandlerRejectsOfflineSavedPlayer(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	memory := NewIgnoreMemory()
	ctx := ignoreTestContext("s1", "player:alice")

	if _, err := NewIgnoreHandler(world, memory)(ctx, enginecmd.ResolvedCommand{Args: []string{"Dave"}}); err != nil {
		t.Fatal(err)
	}
	if got, want := ctx.OutputString(), "그 사용자는 접속중이 아닙니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if memory.Ignored("s1", "Dave") {
		t.Fatal("offline Dave was added to Alice ignore list")
	}
}

func TestIgnoreHandlerRejectsGoOnlyActorIDTargetLikeLegacyFindWho(t *testing.T) {
	loaded := socialWorld(t)
	bob := loaded.Players["player:bob"]
	bob.DisplayName = ""
	loaded.Players["player:bob"] = bob
	world := state.NewWorld(loaded)
	memory := NewIgnoreMemory()
	ctx := ignoreTestContext("s1", "player:alice")

	if _, err := NewIgnoreHandler(world, memory)(ctx, enginecmd.ResolvedCommand{Args: []string{"player:bob"}}); err != nil {
		t.Fatal(err)
	}
	if got, want := ctx.OutputString(), "그 사용자는 접속중이 아닙니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if memory.Ignored("s1", "Player:bob") || memory.Ignored("s1", "Bob") {
		t.Fatal("Go-only actor ID target was added to Alice ignore list")
	}
}

func TestIgnoreHandlerUsesCreatureNameWhenPlayerDisplayNameMissing(t *testing.T) {
	loaded := socialWorld(t)
	bob := loaded.Players["player:bob"]
	bob.DisplayName = ""
	loaded.Players["player:bob"] = bob
	world := state.NewWorld(loaded)
	memory := NewIgnoreMemory()
	ctx := ignoreTestContext("s1", "player:alice")

	if _, err := NewIgnoreHandler(world, memory)(ctx, enginecmd.ResolvedCommand{Args: []string{"Bob"}}); err != nil {
		t.Fatal(err)
	}
	if got, want := ctx.OutputString(), "Bob님을 이야기 듣기 거부 대상에 추가합니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if !memory.Ignored("s1", "Bob") {
		t.Fatal("Bob was not added through creature display name fallback")
	}
}

func TestIgnoreHandlerMatchesLegacyFirstCharacterCaseOnly(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	memory := NewIgnoreMemory()
	ctx := ignoreTestContext("s1", "player:alice")

	if _, err := NewIgnoreHandler(world, memory)(ctx, enginecmd.ResolvedCommand{Args: []string{"bOB"}}); err != nil {
		t.Fatal(err)
	}
	if got, want := ctx.OutputString(), "그 사용자는 접속중이 아닙니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if memory.Ignored("s1", "Bob") {
		t.Fatal("mixed-case Bob was added despite legacy up()-only normalization")
	}
}

func TestIgnoreHandlerDoesNotRemoveWithNonLegacyCaseMatch(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	memory := NewIgnoreMemory()
	memory.Add("s1", "Bob")
	ctx := ignoreTestContext("s1", "player:alice")

	if _, err := NewIgnoreHandler(world, memory)(ctx, enginecmd.ResolvedCommand{Args: []string{"bOB"}}); err != nil {
		t.Fatal(err)
	}
	if got, want := ctx.OutputString(), "그 사용자는 접속중이 아닙니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if !memory.Ignored("s1", "Bob") {
		t.Fatal("mixed-case input removed Bob despite legacy strcmp behavior")
	}
}

func TestIgnoreHandlerRejectsPDMINVActiveTarget(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	setSocialCreatureStat(t, world, "creature:bob", "PDMINV", 1)
	memory := NewIgnoreMemory()
	ctx := ignoreTestContext("s1", "player:alice")

	if _, err := NewIgnoreHandler(world, memory)(ctx, enginecmd.ResolvedCommand{Args: []string{"Bob"}}); err != nil {
		t.Fatal(err)
	}
	if got, want := ctx.OutputString(), "그 사용자는 접속중이 아닙니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if memory.Ignored("s1", "Bob") {
		t.Fatal("PDMINV Bob was added to Alice ignore list")
	}
}

func TestIgnoreHandlerDispatcherAliases(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	memory := NewIgnoreMemory()
	dispatcher := enginecmd.Dispatcher{
		Registry: ignoreRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"ignore": NewIgnoreHandler(world, memory),
		},
	}

	koreanCtx := ignoreTestContext("s1", "player:alice")
	if _, err := dispatcher.DispatchLine(koreanCtx, "Bob 듣기거부"); err != nil {
		t.Fatal(err)
	}
	if got, want := koreanCtx.OutputString(), "Bob님을 이야기 듣기 거부 대상에 추가합니다.\n"; got != want {
		t.Fatalf("korean alias output = %q, want %q", got, want)
	}
	if !memory.Ignored("s1", "Bob") {
		t.Fatal("Korean alias did not add Bob")
	}

	englishCtx := ignoreTestContext("s1", "player:alice")
	if _, err := dispatcher.DispatchLine(englishCtx, "Bob ignore"); err != nil {
		t.Fatal(err)
	}
	if got, want := englishCtx.OutputString(), "Bob님을 이야기 듣기 거부 대상에서 삭제합니다."; got != want {
		t.Fatalf("english alias output = %q, want %q", got, want)
	}
	if memory.Ignored("s1", "Bob") {
		t.Fatal("English alias did not remove Bob")
	}
}

func ignoreRegistry(t *testing.T) commandspec.Registry {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "듣기거부", Number: 68, Handler: "ignore"},
		{Name: "ignore", Number: 68, Handler: "ignore"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func ignoreTestContext(sessionID session.ID, actorID model.PlayerID) *enginecmd.Context {
	active := []ActiveSession{
		{ID: "s1", ActorID: "player:alice"},
		{ID: "s2", ActorID: "player:bob"},
		{ID: "s3", ActorID: "player:charlie"},
	}
	return &enginecmd.Context{
		SessionID: string(sessionID),
		ActorID:   string(actorID),
		Values: map[string]any{
			ContextActiveSessionsKey: func() []ActiveSession {
				out := make([]ActiveSession, len(active))
				copy(out, active)
				return out
			},
		},
	}
}
