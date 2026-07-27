package state

import (
	"strings"
	"testing"
)

// TestSafeSidecarStemHardening covers the path-traversal boundary of
// safeSidecarStem: player names become filename stems, so separators, control
// chars, traversal tokens, and overlong names must be rejected.
func TestSafeSidecarStemHardening(t *testing.T) {
	maxLen := strings.Repeat("가", safeSidecarStemMaxRunes)
	overLen := strings.Repeat("가", safeSidecarStemMaxRunes+1)

	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"empty rejected", "", false},
		{"valid hangul", "무영문", true},
		{"single hangul", "가", true},
		{"max length hangul ok", maxLen, true},
		{"over length rejected", overLen, false},
		{"NUL rejected", "무\x00영", false},
		{"control BEL rejected", "무\x07영", false},
		{"control US rejected", "무\x1f영", false},
		{"DEL rejected", "무\x7f영", false},
		{"C1 control U+0085 rejected", "무영", false},
		{"slash rejected", "무/영", false},
		{"backslash rejected", `무\영`, false},
		{"dotdot rejected", "..", false},
		{"dot rejected", ".", false},
		{"traversal prefix rejected", "../etc", false},
		{"absolute rejected", "/etc/passwd", false},
		{"U+2044 fraction slash rejected", "무⁄영", false},
		{"U+2215 division slash rejected", "무∕영", false},
		{"fullwidth comma not-separator ok", "무，영", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := safeSidecarStem("test", c.in)
			switch {
			case c.ok && err != nil:
				t.Fatalf("safeSidecarStem(%q) = err %v, want nil", c.in, err)
			case !c.ok && err == nil:
				t.Fatalf("safeSidecarStem(%q) = nil, want error", c.in)
			}
		})
	}
}
