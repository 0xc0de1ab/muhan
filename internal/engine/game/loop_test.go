package game

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	enginecmd "github.com/0xc0de1ab/muhan/internal/engine/command"
	"github.com/0xc0de1ab/muhan/internal/session"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestMain(m *testing.M) {
	const env = "MUHAN_TEST_DISABLE_PERSISTENCE"
	old, hadOld := os.LookupEnv(env)
	_ = os.Setenv(env, "1")
	code := m.Run()
	if hadOld {
		_ = os.Setenv(env, old)
	} else {
		_ = os.Unsetenv(env)
	}
	os.Exit(code)
}

func TestLoopDispatchesSessionLineToCommandChannel(t *testing.T) {
	dispatcher := testDispatcher(t, func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx.SessionID != "s1" || ctx.ActorID != "player:alice" {
			t.Fatalf("context = %+v, want session/actor binding", ctx)
		}
		if resolved.Spec.Handler != "look" {
			t.Fatalf("handler = %q, want look", resolved.Spec.Handler)
		}
		ctx.WriteString("방 출력\n")
		return enginecmd.StatusDefault, nil
	})
	loop := NewLoop(dispatcher)
	commands := make(chan session.Command, 1)
	if err := loop.RegisterSession("s1", commands, "player:alice"); err != nil {
		t.Fatal(err)
	}

	err := loop.HandleEvent(context.Background(), session.Event{
		SessionID: "s1",
		Kind:      session.EventLine,
		Line:      "봐",
	})
	if err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	cmd := <-commands
	if cmd.Write != "방 출력\n" || cmd.Prompt != "" || cmd.Close {
		t.Fatalf("session command = %+v, want write only", cmd)
	}
}

func TestLoopExpandsLegacyLastCommandBang(t *testing.T) {
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "봐라", Number: 2, Handler: "look"},
		{Name: "추가", Number: 3, Handler: "extra"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var inputs []string
	dispatcher := enginecmd.Dispatcher{
		Registry: registry,
		Handlers: map[string]enginecmd.Handler{
			"look": func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
				inputs = append(inputs, resolved.Input)
				ctx.WriteString("look\n")
				return enginecmd.StatusDefault, nil
			},
			"extra": func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
				inputs = append(inputs, resolved.Input)
				ctx.WriteString("extra\n")
				return enginecmd.StatusDefault, nil
			},
		},
	}
	loop := NewLoop(dispatcher)
	commands := make(chan session.Command, 3)
	if err := loop.RegisterSession("s1", commands, "player:alice"); err != nil {
		t.Fatal(err)
	}

	for _, line := range []string{"상자 봐", "!", "! 추가"} {
		if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: line}); err != nil {
			t.Fatalf("HandleEvent(%q) error = %v", line, err)
		}
		<-commands
	}

	want := []string{"상자 봐", "상자 봐", "상자 봐 추가"}
	if len(inputs) != len(want) {
		t.Fatalf("inputs = %#v, want %#v", inputs, want)
	}
	for i := range want {
		if inputs[i] != want[i] {
			t.Fatalf("inputs = %#v, want %#v", inputs, want)
		}
	}
}

func TestLoopBangWithoutLastCommandPrompts(t *testing.T) {
	dispatcher := testDispatcher(t, func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		t.Fatal("dispatcher should not run for empty last-command repeat")
		return enginecmd.StatusDefault, nil
	})
	loop := NewLoop(dispatcher, WithPrompt(func(id session.ID, ctx *enginecmd.Context, status enginecmd.Status) string {
		return "> "
	}))
	commands := make(chan session.Command, 1)
	if err := loop.RegisterSession("s1", commands, "player:alice"); err != nil {
		t.Fatal(err)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "!"}); err != nil {
		t.Fatal(err)
	}
	if got := <-commands; got.Write != "" || got.Prompt != "> " {
		t.Fatalf("command = %+v, want prompt-only repeat", got)
	}
}

func TestLoopBangSuffixTruncatesExecutedCommandLikeLegacy(t *testing.T) {
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "봐", Number: 2, Handler: "look"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var inputs []string
	dispatcher := enginecmd.Dispatcher{
		Registry: registry,
		Handlers: map[string]enginecmd.Handler{
			"look": func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
				inputs = append(inputs, resolved.Input)
				ctx.WriteString("look\n")
				return enginecmd.StatusDefault, nil
			},
		},
	}
	loop := NewLoop(dispatcher)
	commands := make(chan session.Command, 2)
	if err := loop.RegisterSession("s1", commands, "player:alice"); err != nil {
		t.Fatal(err)
	}

	first := "봐 " + strings.Repeat("가", 60)
	for _, line := range []string{first, "! 추가"} {
		if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: line}); err != nil {
			t.Fatalf("HandleEvent(%q) error = %v", line, err)
		}
		<-commands
	}

	wantSecond := legacyTruncateLastCommand(legacyLastCommandText(first)+" 추가", 79)
	if len(inputs) != 2 {
		t.Fatalf("inputs = %#v, want two dispatches", inputs)
	}
	if inputs[1] != wantSecond {
		t.Fatalf("bang suffix input = %q, want %q", inputs[1], wantSecond)
	}
}

func TestLoopHexLineEchoesInputBeforeDispatch(t *testing.T) {
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "look", Number: 2, Handler: "look"},
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := enginecmd.Dispatcher{
		Registry: registry,
		Handlers: map[string]enginecmd.Handler{
			"look": func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
				if resolved.Input != "abc look" {
					t.Fatalf("resolved input = %q, want abc look", resolved.Input)
				}
				ctx.WriteString("look\n")
				return enginecmd.StatusDefault, nil
			},
		},
	}
	world := loopHexLineWorld(t, true)
	loop := NewLoop(dispatcher, WithWorld(world))
	commands := make(chan session.Command, 1)
	if err := loop.RegisterSession("s1", commands, "player:alice"); err != nil {
		t.Fatal(err)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "abc look"}); err != nil {
		t.Fatal(err)
	}

	if got, want := (<-commands).Write, "616263206C6F6F6B\nlook\n"; got != want {
		t.Fatalf("command output = %q, want %q", got, want)
	}
}

func TestLoopHexLineReadsPropertyBackedCreatureFlag(t *testing.T) {
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "look", Number: 2, Handler: "look"},
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := enginecmd.Dispatcher{
		Registry: registry,
		Handlers: map[string]enginecmd.Handler{
			"look": func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
				ctx.WriteString("look\n")
				return enginecmd.StatusDefault, nil
			},
		},
	}
	world := loopHexLineWorld(t, false)
	if _, err := world.SetCreatureProperty("creature:alice", "flags", "PHEXLN"); err != nil {
		t.Fatalf("SetCreatureProperty() error = %v", err)
	}
	loop := NewLoop(dispatcher, WithWorld(world))
	commands := make(chan session.Command, 1)
	if err := loop.RegisterSession("s1", commands, "player:alice"); err != nil {
		t.Fatal(err)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "abc look"}); err != nil {
		t.Fatal(err)
	}

	if got, want := (<-commands).Write, "616263206C6F6F6B\nlook\n"; got != want {
		t.Fatalf("command output = %q, want %q", got, want)
	}
}

func TestLoopAddsPromptForPromptStatus(t *testing.T) {
	dispatcher := testDispatcher(t, func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		ctx.WriteString("확인\n")
		return enginecmd.StatusPrompt, nil
	})
	loop := NewLoop(dispatcher, WithPrompt(func(id session.ID, ctx *enginecmd.Context, status enginecmd.Status) string {
		return "> "
	}))
	commands := make(chan session.Command, 1)
	if err := loop.RegisterSession("s1", commands, "player:alice"); err != nil {
		t.Fatal(err)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "봐"}); err != nil {
		t.Fatal(err)
	}

	cmd := <-commands
	if cmd.Write != "확인\n" || cmd.Prompt != "> " {
		t.Fatalf("session command = %+v, want write plus prompt", cmd)
	}
}

func TestLoopCanPromptForDefaultStatus(t *testing.T) {
	dispatcher := testDispatcher(t, func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		ctx.WriteString("확인\n")
		return enginecmd.StatusDefault, nil
	})
	loop := NewLoop(dispatcher,
		WithPrompt(func(id session.ID, ctx *enginecmd.Context, status enginecmd.Status) string {
			return "> "
		}),
		WithPromptPolicy(func(status enginecmd.Status, err error) bool {
			return status == enginecmd.StatusDefault && err == nil
		}),
	)
	commands := make(chan session.Command, 1)
	if err := loop.RegisterSession("s1", commands, "player:alice"); err != nil {
		t.Fatal(err)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "봐"}); err != nil {
		t.Fatal(err)
	}

	cmd := <-commands
	if cmd.Write != "확인\n" || cmd.Prompt != "> " {
		t.Fatalf("session command = %+v, want default-status prompt", cmd)
	}
}

func TestLoopRoutesPendingLineHandlerBeforeDispatcher(t *testing.T) {
	dispatchCount := 0
	dispatcher := testDispatcher(t, func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		dispatchCount++
		ctx.WriteString("제목: ")
		enginecmd.SetPendingLineHandler(ctx, func(ctx *enginecmd.Context, line string) (enginecmd.Status, error) {
			ctx.WriteString("받음: " + line + "\n")
			enginecmd.ClearPendingLineHandler(ctx)
			return enginecmd.StatusDefault, nil
		})
		return enginecmd.StatusDoPrompt, nil
	})
	loop := NewLoop(dispatcher,
		WithPrompt(func(id session.ID, ctx *enginecmd.Context, status enginecmd.Status) string {
			return "> "
		}),
		WithPromptPolicy(func(status enginecmd.Status, err error) bool {
			return status != enginecmd.StatusDoPrompt && err == nil
		}),
	)
	commands := make(chan session.Command, 2)
	if err := loop.RegisterSession("s1", commands, "player:alice"); err != nil {
		t.Fatal(err)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "봐라"}); err != nil {
		t.Fatal(err)
	}
	if got := <-commands; got.Write != "제목: " || got.Prompt != "" {
		t.Fatalf("first command = %+v, want title prompt without main prompt", got)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "새 공지"}); err != nil {
		t.Fatal(err)
	}
	if got := <-commands; got.Write != "받음: 새 공지\n" || got.Prompt != "> " {
		t.Fatalf("pending command = %+v, want pending handler output", got)
	}
	if dispatchCount != 1 {
		t.Fatalf("dispatch count = %d, want pending input to bypass dispatcher", dispatchCount)
	}
}

func TestLoopFormatsDispatchError(t *testing.T) {
	dispatcher := enginecmd.Dispatcher{Registry: testRegistry(t)}
	loop := NewLoop(dispatcher, WithErrorFormatter(func(error) string {
		return "무슨 말인지 모르겠습니다.\n"
	}))
	commands := make(chan session.Command, 1)
	if err := loop.RegisterSession("s1", commands, "player:alice"); err != nil {
		t.Fatal(err)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "춤춰"}); err != nil {
		t.Fatal(err)
	}

	cmd := <-commands
	if cmd.Write != "무슨 말인지 모르겠습니다.\n" {
		t.Fatalf("session command = %+v, want formatted error", cmd)
	}
}

func TestLoopRoutesUnauthenticatedLineBeforeDispatcher(t *testing.T) {
	dispatchCalled := false
	dispatcher := testDispatcher(t, func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		dispatchCalled = true
		ctx.WriteString("인증됨\n")
		return enginecmd.StatusDefault, nil
	})
	loop := NewLoop(dispatcher,
		WithUnauthenticatedLineHandler(func(ctx context.Context, id session.ID, line string) (UnauthenticatedLineResult, error) {
			if id != "s1" || line != "인제로" {
				t.Fatalf("unauthenticated line = id %q line %q", id, line)
			}
			return UnauthenticatedLineResult{
				ActorID: "인제로",
				Command: session.Command{Write: "로그인 성공\n", Prompt: "> "},
			}, nil
		}),
	)
	commands := make(chan session.Command, 2)
	if err := loop.RegisterSession("s1", commands, ""); err != nil {
		t.Fatal(err)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "인제로"}); err != nil {
		t.Fatal(err)
	}
	if dispatchCalled {
		t.Fatal("dispatcher was called for unauthenticated line")
	}
	if got := <-commands; got != (session.Command{Write: "로그인 성공\n", Prompt: "> "}) {
		t.Fatalf("command = %#v, want login success", got)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "봐"}); err != nil {
		t.Fatal(err)
	}
	if got := <-commands; got != (session.Command{Write: "인증됨\n"}) {
		t.Fatalf("command = %#v, want authenticated dispatch", got)
	}
}

func TestLoopDisconnectStatusClosesSession(t *testing.T) {
	dispatcher := testDispatcher(t, func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		ctx.WriteString("잘 가\n")
		return enginecmd.StatusDisconnect, nil
	})
	loop := NewLoop(dispatcher)
	commands := make(chan session.Command, 1)
	if err := loop.RegisterSession("s1", commands, "player:alice"); err != nil {
		t.Fatal(err)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "봐"}); err != nil {
		t.Fatal(err)
	}

	cmd := <-commands
	if cmd.Write != "잘 가\n" || !cmd.Close {
		t.Fatalf("session command = %+v, want close", cmd)
	}
}

func TestLoopUnregistersClosedSession(t *testing.T) {
	loop := NewLoop(enginecmd.Dispatcher{Registry: testRegistry(t)})
	commands := make(chan session.Command, 1)
	if err := loop.RegisterSession("s1", commands, "player:alice"); err != nil {
		t.Fatal(err)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventClosed}); err != nil {
		t.Fatal(err)
	}
	err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "봐"})
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("HandleEvent() error = %v, want ErrSessionNotFound", err)
	}
}

func TestLoopClosedSessionClearsPendingLineHandler(t *testing.T) {
	dispatcher := testDispatcher(t, func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		ctx.WriteString("제목: ")
		enginecmd.SetPendingLineHandler(ctx, func(ctx *enginecmd.Context, line string) (enginecmd.Status, error) {
			ctx.WriteString("pending should not run\n")
			return enginecmd.StatusDefault, nil
		})
		return enginecmd.StatusDoPrompt, nil
	})
	loop := NewLoop(dispatcher)
	commands := make(chan session.Command, 2)
	if err := loop.RegisterSession("s1", commands, "player:alice"); err != nil {
		t.Fatal(err)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "봐"}); err != nil {
		t.Fatal(err)
	}
	if got := <-commands; got.Write != "제목: " {
		t.Fatalf("first command = %+v, want pending prompt", got)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventClosed}); err != nil {
		t.Fatal(err)
	}
	err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "본문"})
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("HandleEvent after close = %v, want ErrSessionNotFound", err)
	}
	select {
	case got := <-commands:
		t.Fatalf("unexpected command after close: %+v", got)
	default:
	}
}

func TestLoopRunsLookMoveLookSequenceWithRuntimeWorld(t *testing.T) {
	world := state.NewWorld(loopSequenceWorld(t))
	move := enginecmd.NewMoveHandler(world)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: testSequenceRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"look": enginecmd.NewLookHandler(world),
			"go":   move,
			"move": move,
		},
	})

	events := make(chan session.Event, 8)
	commands := make(chan session.Command, 8)
	if err := loop.RegisterSession("s1", commands, "player:alice"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx, events)
	}()
	defer func() {
		cancel()
		close(events)
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("Loop.Run() error = %v", err)
			}
		case <-time.After(time.Second):
			t.Error("timed out waiting for loop shutdown")
		}
	}()

	sendLine(t, events, "봐")
	firstLook := recvCommand(t, commands)
	assertCommandShowsRoom(t, firstLook, "광장", "출발 광장이다.", "[ 출구 : 동 ]\n")
	assertCommandDoesNotContain(t, firstLook, "동쪽")

	sendLine(t, events, "동 가")
	moveOutput := recvCommand(t, commands)
	assertCommandShowsRoom(t, moveOutput, "동쪽", "동쪽 방이다.", "[ 출구 : 서 ]\n")
	assertCommandDoesNotContain(t, moveOutput, "출발 광장이다.")

	sendLine(t, events, "봐")
	secondLook := recvCommand(t, commands)
	assertCommandShowsRoom(t, secondLook, "동쪽", "동쪽 방이다.", "[ 출구 : 서 ]\n")
	assertCommandDoesNotContain(t, secondLook, "출발 광장이다.")

	player, ok := world.Player("player:alice")
	if !ok {
		t.Fatal("missing player:alice")
	}
	if player.RoomID != "room:east" {
		t.Fatalf("player room id = %q, want room:east", player.RoomID)
	}
	creature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("missing creature:alice")
	}
	if creature.RoomID != "room:east" {
		t.Fatalf("creature room id = %q, want room:east", creature.RoomID)
	}
}

func testDispatcher(t *testing.T, handler enginecmd.Handler) enginecmd.Dispatcher {
	t.Helper()
	return enginecmd.Dispatcher{
		Registry: testRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"look": handler,
		},
	}
}

func testRegistry(t *testing.T) commandspec.Registry {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "봐라", Number: 2, Handler: "look"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func testSequenceRegistry(t *testing.T) commandspec.Registry {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "봐라", Number: 2, Handler: "look"},
		{Name: "가", Number: 30, Handler: "go"},
		{Name: "동", Number: 1, Handler: "move"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func loopHexLineWorld(t *testing.T, enabled bool) *state.World {
	t.Helper()
	loaded := worldload.NewWorld()
	if err := loaded.AddRoom(model.Room{ID: "room:plaza", DisplayName: "광장"}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddPlayer(model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:plaza",
	}); err != nil {
		t.Fatal(err)
	}
	stats := map[string]int(nil)
	tags := []string(nil)
	if enabled {
		stats = map[string]int{"PHEXLN": 1}
		tags = []string{"PHEXLN"}
	}
	if err := loaded.AddCreature(model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:plaza",
		Stats:       stats,
		Metadata:    model.Metadata{Tags: tags},
	}); err != nil {
		t.Fatal(err)
	}
	return state.NewWorld(loaded)
}

func loopSequenceWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLoopRoom(t, loaded, model.Room{
		ID:               "room:plaza",
		DisplayName:      "광장",
		ShortDescription: "출발 광장이다.",
		Exits: []model.Exit{{
			Name:     "동",
			ToRoomID: "room:east",
		}},
	})
	mustAddLoopRoom(t, loaded, model.Room{
		ID:               "room:east",
		DisplayName:      "동쪽",
		ShortDescription: "동쪽 방이다.",
		Exits: []model.Exit{{
			Name:     "서",
			ToRoomID: "room:plaza",
		}},
	})
	mustAddLoopPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:plaza",
	})
	mustAddLoopCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:plaza",
	})
	return loaded
}

func sendLine(t *testing.T, events chan<- session.Event, line string) {
	t.Helper()
	select {
	case events <- session.Event{SessionID: "s1", Kind: session.EventLine, Line: line}:
	case <-time.After(time.Second):
		t.Fatalf("timed out sending line %q", line)
	}
}

func recvCommand(t *testing.T, commands <-chan session.Command) session.Command {
	t.Helper()
	select {
	case cmd := <-commands:
		if cmd.Close {
			t.Fatalf("command closed session unexpectedly: %+v", cmd)
		}
		if cmd.Prompt != "" {
			t.Fatalf("command prompt = %q, want none", cmd.Prompt)
		}
		if cmd.Write == "" {
			t.Fatal("command write is empty")
		}
		return cmd
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session command")
		return session.Command{}
	}
}

func assertCommandShowsRoom(t *testing.T, cmd session.Command, title, description, exits string) {
	t.Helper()
	for _, want := range []string{
		"\n" + title + "\n\n",
		description + "\n",
		exits,
	} {
		if !strings.Contains(cmd.Write, want) {
			t.Fatalf("command output missing %q:\n%s", want, cmd.Write)
		}
	}
}

func assertCommandDoesNotContain(t *testing.T, cmd session.Command, unwanted string) {
	t.Helper()
	if strings.Contains(cmd.Write, unwanted) {
		t.Fatalf("command output contains %q:\n%s", unwanted, cmd.Write)
	}
}

func mustAddLoopRoom(t *testing.T, world *worldload.World, room model.Room) {
	t.Helper()
	if err := world.AddRoom(room); err != nil {
		t.Fatal(err)
	}
}

func mustAddLoopPlayer(t *testing.T, world *worldload.World, player model.Player) {
	t.Helper()
	if err := world.AddPlayer(player); err != nil {
		t.Fatal(err)
	}
}

func mustAddLoopCreature(t *testing.T, world *worldload.World, creature model.Creature) {
	t.Helper()
	if err := world.AddCreature(creature); err != nil {
		t.Fatal(err)
	}
}
