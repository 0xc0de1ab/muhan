package session

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestTruncateUTF8HandlesKoreanAtLineCap(t *testing.T) {
	// Build a string with Korean syllables that exceeds 4096 bytes.
	// Each Korean syllable is 3 bytes in UTF-8.
	// 1366 syllables = 4098 bytes, which exceeds MaxInputLineBytes (4096).
	syllable := "가" // 3 bytes
	repeated := strings.Repeat(syllable, 1366) // 4098 bytes
	if len(repeated) <= MaxInputLineBytes {
		t.Fatalf("test input must exceed MaxInputLineBytes: got %d bytes", len(repeated))
	}

	got := truncateUTF8(repeated, MaxInputLineBytes)
	if len(got) > MaxInputLineBytes {
		t.Fatalf("truncated length = %d, want <= %d", len(got), MaxInputLineBytes)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("truncated string is not valid UTF-8")
	}
	// Verify the string ends at a complete rune boundary by decoding the last rune.
	if len(got) > 0 {
		// ValidString already guarantees no rune is split, but double-check
		// that we can decode a rune starting at every byte (especially the last).
		for i := len(got) - 1; i >= 0; i-- {
			if utf8.RuneStart(got[i]) {
				_, size := utf8.DecodeRuneInString(got[i:])
				if i+size != len(got) {
					t.Fatalf("truncated string has incomplete rune at byte %d", i)
				}
				break
			}
		}
	}
}

func TestTruncateUTF8ShortStringUnchanged(t *testing.T) {
	got := truncateUTF8("hello", MaxInputLineBytes)
	if got != "hello" {
		t.Fatalf("truncateUTF8 changed a short string: got %q", got)
	}
}

func TestRateLimiterAllowsBurstThenThrottles(t *testing.T) {
	rl := newRateLimiter(5, 30, time.Second)
	base := time.Unix(0, 0)

	// First 5 commands within burst should pass.
	for i := 0; i < 5; i++ {
		if !rl.allow(base) {
			t.Fatalf("command %d should be allowed within burst", i+1)
		}
	}
	// 6th command should be throttled.
	if rl.allow(base) {
		t.Fatal("6th command should be throttled (burst=5)")
	}
}

func TestRateLimiterRecoveryAfterWindow(t *testing.T) {
	rl := newRateLimiter(3, 30, time.Second)
	base := time.Unix(0, 0)

	// Exhaust burst.
	for i := 0; i < 3; i++ {
		if !rl.allow(base) {
			t.Fatalf("command %d should be allowed", i+1)
		}
	}
	// Throttled at same time.
	if rl.allow(base) {
		t.Fatal("4th command should be throttled")
	}
	// After the window passes, commands should be allowed again.
	if !rl.allow(base.Add(time.Second + time.Millisecond)) {
		t.Fatal("command after window expiry should be allowed")
	}
}

func TestRateLimiterSustainedLimit(t *testing.T) {
	// burst=100 (very high), sustain=10 per second.
	// In a 1s window, maxInWindow = 10.
	rl := newRateLimiter(100, 10, time.Second)
	base := time.Unix(0, 0)

	for i := 0; i < 10; i++ {
		if !rl.allow(base) {
			t.Fatalf("command %d should be allowed under sustained limit", i+1)
		}
	}
	// 11th command should be throttled by sustained limit.
	if rl.allow(base) {
		t.Fatal("11th command should be throttled by sustained limit (sustain=10)")
	}
}
