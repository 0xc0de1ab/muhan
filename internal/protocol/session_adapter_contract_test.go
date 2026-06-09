package protocol

import (
	"slices"
	"strings"
	"testing"
)

func TestSessionStateCoalescesCRLFSplitAcrossChunks(t *testing.T) {
	s := NewSessionState()

	if got := s.FeedInput([]byte("봐\r")); !slices.Equal(got, []string{"봐"}) {
		t.Fatalf("first FeedInput() = %#v, want 봐", got)
	}
	got := s.FeedInput([]byte("\n말\n"))
	want := []string{"말"}
	if !slices.Equal(got, want) {
		t.Fatalf("second FeedInput() = %#v, want %#v", got, want)
	}
	if s.PendingLines() != 2 {
		t.Fatalf("PendingLines() = %d, want 2", s.PendingLines())
	}
}

func TestSessionStatePendingUTF8BackspaceThenNewline(t *testing.T) {
	s := NewSessionState()
	split := []byte("한")

	got := s.FeedInput(append([]byte{'x'}, split[:2]...))
	if len(got) != 0 {
		t.Fatalf("first FeedInput() = %#v, want no lines", got)
	}

	got = s.FeedInput([]byte{0x7f, '\n'})
	want := []string{""}
	if !slices.Equal(got, want) {
		t.Fatalf("second FeedInput() = %#v, want %#v", got, want)
	}
}

func TestSessionStatePendingUTF8NewlineDropsIncompleteRune(t *testing.T) {
	s := NewSessionState()
	split := []byte("한")

	got := s.FeedInput(append([]byte{'x'}, split[:2]...))
	if len(got) != 0 {
		t.Fatalf("first FeedInput() = %#v, want no lines", got)
	}

	got = s.FeedInput([]byte{'\n'})
	want := []string{"x"}
	if !slices.Equal(got, want) {
		t.Fatalf("second FeedInput() = %#v, want %#v", got, want)
	}
}

func TestSessionStateHighWaterDrainBehavior(t *testing.T) {
	s := NewSessionState()
	highWaterBytes := (DefaultOutputLimit * 3 / 4) + 1

	if got := s.AppendOutput(strings.Repeat("x", highWaterBytes)); got != highWaterBytes {
		t.Fatalf("AppendOutput() = %d, want %d", got, highWaterBytes)
	}
	if !s.ShouldFlush() {
		t.Fatal("ShouldFlush() = false above output high-water mark, want true")
	}
	if got := len(s.DrainOutput(1)); got != 1 {
		t.Fatalf("DrainOutput(1) length = %d, want 1", got)
	}
	if s.ShouldFlush() {
		t.Fatal("ShouldFlush() = true after draining to high-water boundary, want false")
	}
	if got := len(s.DrainAllOutput()); got != highWaterBytes-1 {
		t.Fatalf("DrainAllOutput() length = %d, want %d", got, highWaterBytes-1)
	}
	if s.ShouldFlush() {
		t.Fatal("ShouldFlush() = true after draining all output, want false")
	}
}

func TestSessionStateClosedKeepsQueuedDrainableStateAndRejectsNewIO(t *testing.T) {
	s := NewSessionState()
	s.AppendOutput("before")
	s.FeedInput([]byte("look\n"))

	s.Close()
	if !s.Closed() {
		t.Fatal("Closed() = false, want true")
	}
	if got := s.AppendOutput("after"); got != 0 {
		t.Fatalf("AppendOutput() after close = %d, want 0", got)
	}
	if got := s.FeedInput([]byte("ignored\n")); len(got) != 0 {
		t.Fatalf("FeedInput() after close = %#v, want no lines", got)
	}

	line, ok := s.NextLine()
	if !ok || line != "look" {
		t.Fatalf("NextLine() after close = %q, %v; want look, true", line, ok)
	}
	if got := string(s.DrainAllOutput()); got != "before" {
		t.Fatalf("DrainAllOutput() after close = %q, want before", got)
	}
}
