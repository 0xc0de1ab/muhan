package game

import (
	"context"
	"errors"
	"reflect"
	"testing"

	enginecmd "github.com/0xc0de1ab/muhan/internal/engine/command"
	"github.com/0xc0de1ab/muhan/internal/session"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

func TestLoopActiveSessionsReturnsSortedSnapshot(t *testing.T) {
	loop := NewLoop(enginecmd.Dispatcher{Registry: testRegistry(t)})
	if err := loop.RegisterSession("s2", make(chan session.Command, 1), "player:bob"); err != nil {
		t.Fatal(err)
	}
	if err := loop.RegisterSession("s1", make(chan session.Command, 1), "player:alice"); err != nil {
		t.Fatal(err)
	}
	if err := loop.BindActor("s2", "player:bob2"); err != nil {
		t.Fatal(err)
	}

	got := loop.ActiveSessions()
	want := []ActiveSession{
		{ID: "s1", ActorID: "player:alice"},
		{ID: "s2", ActorID: "player:bob2"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveSessions() = %#v, want %#v", got, want)
	}

	got[0].ActorID = "player:changed"
	actors := loop.ActiveSessionActors()
	if actors["s1"] != "player:alice" || actors["s2"] != "player:bob2" {
		t.Fatalf("ActiveSessionActors() = %#v, want original actors", actors)
	}

	loop.UnregisterSession("s1")
	got = loop.ActiveSessions()
	want = []ActiveSession{{ID: "s2", ActorID: "player:bob2"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveSessions() after unregister = %#v, want %#v", got, want)
	}
}

func TestLoopSendToSessionAndBroadcastRouteBySessionID(t *testing.T) {
	loop := NewLoop(enginecmd.Dispatcher{Registry: testRegistry(t)})
	s1 := make(chan session.Command, 4)
	s2 := make(chan session.Command, 4)
	s3 := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", s1, "player:alice")
	registerTestSession(t, loop, "s2", s2, "player:bob")
	registerTestSession(t, loop, "s3", s3, "player:charlie")

	if err := loop.SendToSession(context.Background(), "s2", session.Command{Write: "direct\n"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, s2, session.Command{Write: "direct\n"})
	assertNoCommand(t, s1)
	assertNoCommand(t, s3)

	if err := loop.BroadcastExcept(context.Background(), "s2", session.Command{Write: "except\n"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, s1, session.Command{Write: "except\n"})
	assertNoCommand(t, s2)
	assertCommand(t, s3, session.Command{Write: "except\n"})

	if err := loop.Broadcast(context.Background(), session.Command{Write: "all\n"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, s1, session.Command{Write: "all\n"})
	assertCommand(t, s2, session.Command{Write: "all\n"})
	assertCommand(t, s3, session.Command{Write: "all\n"})

	err := loop.SendToSession(context.Background(), "missing", session.Command{Write: "lost\n"})
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("SendToSession() error = %v, want ErrSessionNotFound", err)
	}
}

func TestLoopCommandContextExposesSessionRoutingPrimitives(t *testing.T) {
	dispatcher := testDispatcher(t, func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		activeSessions, ok := ctx.Values[ContextActiveSessionsKey].(func() []ActiveSession)
		if !ok {
			t.Fatalf("%s missing or wrong type", ContextActiveSessionsKey)
		}
		sessionActors, ok := ctx.Values[ContextSessionActorsKey].(func() map[session.ID]string)
		if !ok {
			t.Fatalf("%s missing or wrong type", ContextSessionActorsKey)
		}
		sendToSession, ok := ctx.Values[ContextSendToSessionKey].(func(session.ID, session.Command) error)
		if !ok {
			t.Fatalf("%s missing or wrong type", ContextSendToSessionKey)
		}
		broadcast, ok := ctx.Values[ContextBroadcastKey].(func(session.Command) error)
		if !ok {
			t.Fatalf("%s missing or wrong type", ContextBroadcastKey)
		}
		broadcastExcept, ok := ctx.Values[ContextBroadcastExceptKey].(func(session.ID, session.Command) error)
		if !ok {
			t.Fatalf("%s missing or wrong type", ContextBroadcastExceptKey)
		}

		got := activeSessions()
		want := []ActiveSession{
			{ID: "s1", ActorID: "player:alice"},
			{ID: "s2", ActorID: "player:bob"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("activeSessions() = %#v, want %#v", got, want)
		}
		actors := sessionActors()
		if actors["s1"] != "player:alice" || actors["s2"] != "player:bob" {
			t.Fatalf("sessionActors() = %#v, want s1/s2 actors", actors)
		}

		if err := sendToSession("s2", session.Command{Write: "direct\n"}); err != nil {
			t.Fatal(err)
		}
		if err := broadcastExcept("s1", session.Command{Write: "except\n"}); err != nil {
			t.Fatal(err)
		}
		if err := broadcast(session.Command{Write: "all\n"}); err != nil {
			t.Fatal(err)
		}
		ctx.WriteString("handled\n")
		return enginecmd.StatusDefault, nil
	})
	loop := NewLoop(dispatcher)
	s1 := make(chan session.Command, 4)
	s2 := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", s1, "player:alice")
	registerTestSession(t, loop, "s2", s2, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "봐"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, s2, session.Command{Write: "direct\n"})
	assertCommand(t, s2, session.Command{Write: "except\n"})
	assertCommand(t, s2, session.Command{Write: "all\n"})
	assertCommand(t, s1, session.Command{Write: "all\n"})
	assertCommand(t, s1, session.Command{Write: "handled\n"})
	assertNoCommand(t, s1)
	assertNoCommand(t, s2)
}

func TestLoopRoomBroadcastRoutesOnlySameRoom(t *testing.T) {
	world := roomBroadcastTestWorld{
		"player:alice": {ID: "player:alice", RoomID: "room:plaza"},
		"player:bob":   {ID: "player:bob", RoomID: "room:plaza"},
		"player:carol": {ID: "player:carol", RoomID: "room:elsewhere"},
	}
	loop := NewLoop(enginecmd.Dispatcher{Registry: testRegistry(t)}, WithRoomBroadcastWorld(world))
	s1 := make(chan session.Command, 4)
	s2 := make(chan session.Command, 4)
	s3 := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", s1, "player:alice")
	registerTestSession(t, loop, "s2", s2, "player:bob")
	registerTestSession(t, loop, "s3", s3, "player:carol")

	if err := loop.RoomBroadcast(context.Background(), "room:plaza", "s1", "room event\n"); err != nil {
		t.Fatal(err)
	}

	assertNoCommand(t, s1)
	assertCommand(t, s2, session.Command{Write: "room event\n"})
	assertNoCommand(t, s3)
}

func TestLoopCommandContextExposesRoomBroadcastHook(t *testing.T) {
	world := roomBroadcastTestWorld{
		"player:alice": {ID: "player:alice", RoomID: "room:plaza"},
		"player:bob":   {ID: "player:bob", RoomID: "room:plaza"},
	}
	dispatcher := testDispatcher(t, func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		roomBroadcast, ok := ctx.Values[enginecmd.ContextRoomBroadcastKey].(enginecmd.RoomBroadcastFunc)
		if !ok {
			t.Fatalf("%s missing or wrong type", enginecmd.ContextRoomBroadcastKey)
		}
		if err := roomBroadcast("room:plaza", ctx.SessionID, "context room event\n"); err != nil {
			t.Fatal(err)
		}
		ctx.WriteString("handled\n")
		return enginecmd.StatusDefault, nil
	})
	loop := NewLoop(dispatcher, WithRoomBroadcastWorld(world))
	s1 := make(chan session.Command, 4)
	s2 := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", s1, "player:alice")
	registerTestSession(t, loop, "s2", s2, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "봐"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, s2, session.Command{Write: "context room event\n"})
	assertCommand(t, s1, session.Command{Write: "handled\n"})
	assertNoCommand(t, s1)
	assertNoCommand(t, s2)
}

type roomBroadcastTestWorld map[model.PlayerID]model.Player

func (w roomBroadcastTestWorld) Player(id model.PlayerID) (model.Player, bool) {
	player, ok := w[id]
	return player, ok
}

func registerTestSession(t *testing.T, loop *Loop, id session.ID, commands chan<- session.Command, actorID string) {
	t.Helper()
	if err := loop.RegisterSession(id, commands, actorID); err != nil {
		t.Fatal(err)
	}
}

func assertCommand(t *testing.T, commands <-chan session.Command, want session.Command) {
	t.Helper()
	select {
	case got := <-commands:
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("command = %#v, want %#v", got, want)
		}
	default:
		t.Fatalf("no command received, want %#v", want)
	}
}

func assertNoCommand(t *testing.T, commands <-chan session.Command) {
	t.Helper()
	select {
	case got := <-commands:
		t.Fatalf("unexpected command: %#v", got)
	default:
	}
}
