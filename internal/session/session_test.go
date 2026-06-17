package session

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestSessionEmitsUTF8Lines(t *testing.T) {
	client, server, events, commands, done := startTestSession(t)

	if _, err := client.Write([]byte("고블린 때려\n")); err != nil {
		t.Fatal(err)
	}

	event := recvEvent(t, events)
	if event.Kind != EventLine || event.Line != "고블린 때려" {
		t.Fatalf("event = %#v, want line 고블린 때려", event)
	}

	close(commands)
	waitDone(t, done)
	_ = client.Close()
	_ = server.Close()
}

func TestSessionWritesCRLFOutput(t *testing.T) {
	client, _, _, commands, done := startTestSession(t)

	commands <- Command{Write: "안녕\n"}
	buf := make([]byte, 16)
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	n, err := client.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "안녕\r\n" {
		t.Fatalf("read = %q, want CRLF output", got)
	}

	close(commands)
	waitDone(t, done)
	_ = client.Close()
}

func TestSessionStripsTelnetBeforeLineEvent(t *testing.T) {
	client, _, events, commands, done := startTestSession(t)

	if _, err := client.Write([]byte{0xff, 0xfd, 0x01, 0xeb, 0xb4, 0x90, '\n'}); err != nil {
		t.Fatal(err)
	}

	event := recvEvent(t, events)
	if event.Kind != EventLine || event.Line != "봐" {
		t.Fatalf("event = %#v, want line 봐", event)
	}

	close(commands)
	waitDone(t, done)
	_ = client.Close()
}

func TestSessionCloseCommandEmitsClosed(t *testing.T) {
	client, _, events, commands, done := startTestSession(t)

	commands <- Command{Write: "잘 가\n", Close: true}
	buf := make([]byte, 32)
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	n, err := client.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "잘 가\r\n" {
		t.Fatalf("read = %q, want close output", got)
	}

	event := recvEvent(t, events)
	if event.Kind != EventClosed {
		t.Fatalf("event = %#v, want closed", event)
	}
	waitDone(t, done)
	_ = client.Close()
}

func startTestSession(t *testing.T) (net.Conn, net.Conn, chan Event, chan Command, chan error) {
	return startTestSessionWithOpts(t)
}

func startTestSessionWithOpts(t *testing.T, opts ...Option) (net.Conn, net.Conn, chan Event, chan Command, chan error) {
	t.Helper()

	client, server := net.Pipe()
	events := make(chan Event, 128)
	commands := make(chan Command, 128)
	s, err := New("s1", server, events, commands, opts...)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		_ = client.Close()
		_ = server.Close()
	})
	return client, server, events, commands, done
}

func recvEvent(t *testing.T, events <-chan Event) Event {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}

func waitDone(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session shutdown")
	}
}

func TestSessionTruncatesLongLine(t *testing.T) {
	client, _, events, commands, done := startTestSession(t)

	// Build a line that exceeds MaxInputLineBytes (4096).
	// Each Korean syllable "가" is 3 UTF-8 bytes. 1400 syllables = 4200 bytes.
	longLine := strings.Repeat("가", 1400) + "\n"
	if _, err := client.Write([]byte(longLine)); err != nil {
		t.Fatal(err)
	}

	event := recvEvent(t, events)
	if event.Kind != EventLine {
		t.Fatalf("event kind = %q, want line", event.Kind)
	}
	if len(event.Line) > MaxInputLineBytes {
		t.Fatalf("line length = %d, want <= %d", len(event.Line), MaxInputLineBytes)
	}
	if !utf8.ValidString(event.Line) {
		t.Fatal("truncated line is not valid UTF-8")
	}

	close(commands)
	waitDone(t, done)
	_ = client.Close()
}

func TestSessionRateLimiterThrottles(t *testing.T) {
	// Use a tight limiter: burst=2, sustain=5, window=1s.
	rl := newRateLimiter(2, 5, time.Second)
	client, _, events, commands, done := startTestSessionWithOpts(t, WithRateLimiter(rl))

	// Drain throttle messages from the client side so writes don't block.
	go func() {
		buf := make([]byte, 256)
		for {
			if _, err := client.Read(buf); err != nil {
				return
			}
		}
	}()

	// Send 5 commands rapidly. Only 2 should produce events (burst limit).
	for i := 0; i < 5; i++ {
		if _, err := client.Write([]byte("봐\n")); err != nil {
			t.Fatal(err)
		}
	}

	// We expect at most burst (2) line events. Collect what arrives in a short window.
	received := 0
	timeout := time.After(500 * time.Millisecond)
	for received < 5 {
		select {
		case ev := <-events:
			if ev.Kind == EventLine {
				received++
			}
		case <-timeout:
			goto check
		}
	}
check:
	if received > 2 {
		t.Fatalf("received %d line events, expected at most %d (burst)", received, 2)
	}

	close(commands)
	waitDone(t, done)
	_ = client.Close()
}
