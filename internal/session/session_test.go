package session

import (
	"context"
	"net"
	"strings"
	"sync"
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
	output := drainClientOutput(client)

	// Send 5 commands rapidly. Exactly burst (2) should produce line events;
	// the other 3 must be rejected with a throttle notice. Asserting exact
	// counts matters: the previous "<= burst" form also passed when every
	// command was dropped (received == 0), which would hide a rate-limiter
	// regression silently swallowing all player input.
	const sent = 5
	for i := 0; i < sent; i++ {
		if _, err := client.Write([]byte("봐\n")); err != nil {
			t.Fatal(err)
		}
	}

	received, throttled := awaitRateLimitOutcome(events, output, 2, sent-2)
	if received != 2 {
		t.Fatalf("received %d line events, want exactly %d (burst)", received, 2)
	}
	if throttled != sent-2 {
		t.Fatalf("client received %d throttle messages, want %d", throttled, sent-2)
	}

	close(commands)
	waitDone(t, done)
	_ = client.Close()
}

func TestSessionDefaultRateLimiterWiring(t *testing.T) {
	// Production wiring (cmd/muhan-server) constructs sessions with no
	// rate-limiter option, so New must install the default limiter and the
	// DefaultBurstRate must actually be enforced end to end. This guards
	// against the defaults silently becoming no-ops (e.g. limiter left nil
	// or burst wired to a different constant).
	client, _, events, commands, done := startTestSession(t)
	output := drainClientOutput(client)

	const extra = 5
	sent := DefaultBurstRate + extra
	for i := 0; i < sent; i++ {
		if _, err := client.Write([]byte("봐\n")); err != nil {
			t.Fatal(err)
		}
	}

	received, throttled := awaitRateLimitOutcome(events, output, DefaultBurstRate, extra)
	if received != DefaultBurstRate {
		t.Fatalf("received %d line events, want exactly DefaultBurstRate (%d)", received, DefaultBurstRate)
	}
	if throttled != extra {
		t.Fatalf("client received %d throttle messages, want %d", throttled, extra)
	}

	close(commands)
	waitDone(t, done)
	_ = client.Close()
}

// drainClientOutput copies everything the session writes to the client
// connection into a buffer and returns an accessor for its contents. The
// reader goroutine also keeps session writes (e.g. throttle notices) from
// blocking on the unbuffered net.Pipe.
func drainClientOutput(client net.Conn) func() string {
	var mu sync.Mutex
	var buf strings.Builder
	go func() {
		tmp := make([]byte, 256)
		for {
			n, err := client.Read(tmp)
			if n > 0 {
				mu.Lock()
				buf.Write(tmp[:n])
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		return buf.String()
	}
}

// awaitRateLimitOutcome polls until the session has emitted wantEvents line
// events and written wantThrottles throttle notices, or a deadline passes.
// It returns the observed counts so callers can assert exact values.
func awaitRateLimitOutcome(events <-chan Event, output func() string, wantEvents, wantThrottles int) (int, int) {
	received := 0
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case ev := <-events:
			if ev.Kind == EventLine {
				received++
			}
			continue
		default:
		}
		throttled := strings.Count(output(), ThrottleMessage)
		if received >= wantEvents && throttled >= wantThrottles {
			return received, throttled
		}
		if received > wantEvents {
			return received, throttled
		}
		time.Sleep(10 * time.Millisecond)
	}
	return received, strings.Count(output(), ThrottleMessage)
}
