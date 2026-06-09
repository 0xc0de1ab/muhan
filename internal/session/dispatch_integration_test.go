package session

import (
	"net"
	"testing"
	"time"

	"muhan/internal/commandspec"
	enginecommand "muhan/internal/engine/command"
)

func TestSessionLineDispatchesThroughCommandDispatcherToOutput(t *testing.T) {
	client, _, events, commands, done := startTestSession(t)

	registry := mustSessionCommandRegistry(t, []commandspec.CommandSpec{
		{Name: "봐라", Number: 2, Handler: "look"},
	})
	dispatcher := enginecommand.Dispatcher{
		Registry: registry,
		Handlers: map[string]enginecommand.Handler{
			"look": func(ctx *enginecommand.Context, resolved enginecommand.ResolvedCommand) (enginecommand.Status, error) {
				if ctx.SessionID != "s1" {
					t.Fatalf("SessionID = %q, want s1", ctx.SessionID)
				}
				if resolved.Command() != "봐" {
					t.Fatalf("Command() = %q, want 봐", resolved.Command())
				}
				ctx.WriteString("동쪽 방\n")
				return enginecommand.StatusPrompt, nil
			},
		},
	}

	if _, err := client.Write([]byte("봐\n")); err != nil {
		t.Fatal(err)
	}

	event := recvEvent(t, events)
	if event.Kind != EventLine || event.Line != "봐" {
		t.Fatalf("event = %#v, want line 봐", event)
	}

	ctx := &enginecommand.Context{SessionID: string(event.SessionID)}
	status, err := dispatcher.DispatchLine(ctx, event.Line)
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != enginecommand.StatusPrompt {
		t.Fatalf("status = %d, want StatusPrompt", status)
	}

	commands <- Command{Write: ctx.OutputString()}
	if got := readSessionOutput(t, client); got != "동쪽 방\r\n" {
		t.Fatalf("output = %q, want dispatched session output", got)
	}

	close(commands)
	waitDone(t, done)
	_ = client.Close()
}

func mustSessionCommandRegistry(t *testing.T, specs []commandspec.CommandSpec) commandspec.Registry {
	t.Helper()

	registry, err := commandspec.NewRegistry(specs)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func readSessionOutput(t *testing.T, conn net.Conn) string {
	t.Helper()

	buf := make([]byte, 128)
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	return string(buf[:n])
}
