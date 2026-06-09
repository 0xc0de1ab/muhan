package protocol

import "testing"

func TestOutputBufferExpandsNewline(t *testing.T) {
	b := NewOutputBuffer()
	if got := b.AppendString("a\nb"); got != 4 {
		t.Fatalf("AppendString() = %d, want 4 expanded bytes", got)
	}
	if got := string(b.DrainAll()); got != "a\r\nb" {
		t.Fatalf("DrainAll() = %q, want CRLF output", got)
	}
}

func TestOutputBufferDropsOldestBytesAtLimit(t *testing.T) {
	b := NewOutputBuffer(WithOutputLimit(5))
	b.AppendString("abcdef")
	if got := string(b.DrainAll()); got != "bcdef" {
		t.Fatalf("DrainAll() = %q, want newest limited bytes", got)
	}
}

func TestOutputBufferHighWater(t *testing.T) {
	b := NewOutputBuffer(WithOutputLimit(8))
	b.AppendString("123456")
	if b.HighWater() {
		t.Fatal("HighWater() = true at exactly 75%, want false")
	}
	b.AppendString("7")
	if !b.HighWater() {
		t.Fatal("HighWater() = false above 75%, want true")
	}
}

func TestOutputBufferPartialDrain(t *testing.T) {
	b := NewOutputBuffer()
	b.AppendString("abcdef")
	if got := string(b.Drain(2)); got != "ab" {
		t.Fatalf("Drain(2) = %q, want ab", got)
	}
	if got := string(b.DrainAll()); got != "cdef" {
		t.Fatalf("DrainAll() = %q, want cdef", got)
	}
}
