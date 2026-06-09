package protocol

import "testing"

func TestSessionStateQueuesLinesAndFlushesOnInterrupt(t *testing.T) {
	s := NewSessionState()
	s.AppendOutput("prompt> ")
	if s.ShouldFlush() {
		t.Fatal("ShouldFlush() = true before input or high-water output")
	}

	lines := s.FeedInput([]byte("봐\n"))
	if len(lines) != 1 || lines[0] != "봐" {
		t.Fatalf("FeedInput() = %#v, want 봐", lines)
	}
	if !s.InterruptReady() || !s.ShouldFlush() {
		t.Fatal("session should be interrupt-ready and flushable after a complete line")
	}
	if s.PendingLines() != 1 {
		t.Fatalf("PendingLines() = %d, want 1", s.PendingLines())
	}
	line, ok := s.NextLine()
	if !ok || line != "봐" {
		t.Fatalf("NextLine() = %q, %v; want 봐, true", line, ok)
	}
}

func TestSessionStateCallbackAndClose(t *testing.T) {
	s := NewSessionState()
	s.SetCallback("login", 3)
	if got := s.Callback(); got.Name != "login" || got.Param != 3 {
		t.Fatalf("Callback() = %#v, want login/3", got)
	}

	s.Close()
	if !s.Closed() {
		t.Fatal("Closed() = false, want true")
	}
	if got := s.FeedInput([]byte("봐\n")); len(got) != 0 {
		t.Fatalf("FeedInput() after close = %#v, want no lines", got)
	}
	if n := s.AppendOutput("x"); n != 0 {
		t.Fatalf("AppendOutput() after close = %d, want 0", n)
	}
}
