package game

import (
	"context"
	"errors"
	"testing"

	enginecmd "github.com/0xc0de1ab/muhan/internal/engine/command"
	"github.com/0xc0de1ab/muhan/internal/session"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestTalkActionUntargetedRunsAfterResponseAndBroadcastsToRoom(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	root := talkActionTestRoot(t, "계석치무", 25, "임무 ACTION 미소\n찾아오게.\n")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 6)
	bob := make(chan session.Command, 6)
	dave := make(chan session.Command, 6)
	charlie := make(chan session.Command, 6)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", dave, "player:dave")
	registerTestSession(t, loop, "s4", charlie, "player:charlie")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 임무 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"임무\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"찾아오게.\"라고 이야기합니다.\n"})
	assertCommand(t, bob, session.Command{Write: "계석치무가 밝은 미소를 짓습니다.\n"})
	assertCommand(t, dave, session.Command{Write: "\nAlice가 계석치무에게 \"임무\"에 관해 물어봅니다.\n"})
	assertCommand(t, dave, session.Command{Write: "\n계석치무가 Alice에게 \"찾아오게.\"라고 이야기합니다.\n"})
	assertCommand(t, dave, session.Command{Write: "계석치무가 밝은 미소를 짓습니다.\n"})
	assertNoCommand(t, charlie)
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"찾아오게.\"라고 이야기합니다.\n계석치무가 밝은 미소를 짓습니다.\n"})
}

func TestTalkActionPlayerTargetsQuestioningPlayerOnly(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	root := talkActionTestRoot(t, "계석치무", 25, "안녕 ACTION 안 PLAYER\n반갑네.\n")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 6)
	bob := make(chan session.Command, 6)
	dave := make(chan session.Command, 6)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", dave, "player:dave")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 안녕 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"안녕\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"반갑네.\"라고 이야기합니다.\n"})
	assertCommand(t, bob, session.Command{Write: "계석치무가 Alice에게 인사를 합니다. \"안녕하세요~\"\n"})
	assertCommand(t, dave, session.Command{Write: "\nAlice가 계석치무에게 \"안녕\"에 관해 물어봅니다.\n"})
	assertCommand(t, dave, session.Command{Write: "\n계석치무가 Alice에게 \"반갑네.\"라고 이야기합니다.\n"})
	assertCommand(t, dave, session.Command{Write: "계석치무가 Alice에게 인사를 합니다. \"안녕하세요~\"\n"})
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"반갑네.\"라고 이야기합니다.\n계석치무가 당신에게 인사를 합니다. \"안녕하세요~\"\n"})
}

func TestTalkActionUnknownLegacyCommandIsNoop(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	root := talkActionTestRoot(t, "계석치무", 25, "모름 ACTION 없는감정 PLAYER\n모르겠군.\n")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 6)
	bob := make(chan session.Command, 6)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 모름 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"모름\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"모르겠군.\"라고 이야기합니다.\n"})
	assertNoCommand(t, bob)
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"모르겠군.\"라고 이야기합니다.\n"})
}

func TestTalkActionIgnoresRoomSendErrorsLikeLegacy(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	root := talkActionTestRoot(t, "계석치무", 25, "임무 ACTION 미소\n찾아오게.\n")
	ctx := talkSendErrorContext()

	status, err := NewTalkHandlerWithRoot(world, root)(ctx, enginecmd.ResolvedCommand{
		Input: "계석치무 임무 대화",
		Args:  []string{"계석치무", "임무"},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != enginecmd.StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	want := "\n계석치무가 당신에게 \"찾아오게.\"라고 이야기합니다.\n계석치무가 밝은 미소를 짓습니다.\n"
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestTalkCastIgnoresRoomSendErrorsAfterApplyingEffectLikeLegacy(t *testing.T) {
	loaded := talkTestWorld(t)
	wise := loaded.Creatures["creature:wise"]
	wise.Stats = map[string]int{"mpCurrent": 20}
	loaded.Creatures[wise.ID] = wise

	world := state.NewWorld(loaded)
	root := talkActionTestRoot(t, "계석치무", 25, "축복 CAST bless\n축복하네.\n")
	ctx := talkSendErrorContext()

	status, err := NewTalkHandlerWithRoot(world, root)(ctx, enginecmd.ResolvedCommand{
		Input: "계석치무 축복 대화",
		Args:  []string{"계석치무", "축복"},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != enginecmd.StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	want := "\n계석치무가 당신에게 \"축복하네.\"라고 이야기합니다.\n\n계석치무가 당신에게 bless 주문을 겁니다.\n"
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	assertTalkCreatureTag(t, world, "creature:alice", "blessed")
	assertTalkPlayerTag(t, world, "player:alice", "blessed")
	assertTalkCreatureStat(t, world, "creature:wise", "mpCurrent", 10)
}

func talkSendErrorContext() *enginecmd.Context {
	return &enginecmd.Context{
		SessionID: "s1",
		ActorID:   "player:alice",
		Values: map[string]any{
			ContextActiveSessionsKey: func() []ActiveSession {
				return []ActiveSession{
					{ID: "s1", ActorID: "player:alice"},
					{ID: "s2", ActorID: "player:bob"},
				}
			},
			ContextSendToSessionKey: func(session.ID, session.Command) error {
				return errors.New("session closed")
			},
		},
	}
}
