package textfmt

import (
	"fmt"
	"testing"
)

type resolverFunc func(verb byte, value any, count int) (string, error)

func (f resolverFunc) ResolveText(verb byte, value any, count int) (string, error) {
	return f(verb, value, count)
}

func TestStringParticle(t *testing.T) {
	tests := []struct {
		format string
		args   []any
		want   string
	}{
		{"%S%j", []any{"무한", "0"}, "무한은"},
		{"%S%j", []any{"무기", "0"}, "무기는"},
		{"%S%j", []any{"검", "3"}, "검을"},
		{"%S%j", []any{"도끼", "3"}, "도끼를"},
		{"%S%j", []any{"슬라임", 1}, "슬라임이"},
	}

	for _, tt := range tests {
		got, err := Format(tt.format, tt.args...)
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Fatalf("Format(%q, %v) = %q, want %q", tt.format, tt.args, got, tt.want)
		}
	}
}

func TestStringParticleIgnoresANSITrailingNoise(t *testing.T) {
	tests := []struct {
		name  string
		value string
		kind  any
		want  string
	}{
		{
			name:  "ansi final consonant",
			value: "\x1b[1;31m무한\x1b[0m",
			kind:  "0",
			want:  "\x1b[1;31m무한\x1b[0m은",
		},
		{
			name:  "punctuation and spaces",
			value: "무기!  ",
			kind:  "0",
			want:  "무기!  는",
		},
		{
			name:  "trailing parenthetical",
			value: "무한(검)",
			kind:  "3",
			want:  "무한(검)을",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Format("%S%j", tt.value, tt.kind)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("Format(%q, %q) = %q, want %q", "%S%j", tt.value, got, tt.want)
			}
		})
	}
}

func TestEscapedPercent(t *testing.T) {
	got, err := Format("진행률 100%%")
	if err != nil {
		t.Fatal(err)
	}
	if got != "진행률 100%" {
		t.Fatalf("escaped percent = %q", got)
	}
}

func TestResolverCreatureParticle(t *testing.T) {
	r := Renderer{
		Resolver: resolverFunc(func(verb byte, value any, count int) (string, error) {
			if verb != 'M' {
				return "", fmt.Errorf("verb = %c", verb)
			}
			if count != 2 {
				return "", fmt.Errorf("count = %d", count)
			}
			if value != "mob:slime" {
				return "", fmt.Errorf("value = %v", value)
			}
			return "슬라임", nil
		}),
	}

	got, err := r.Format("%2M%j", "mob:slime", "1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "슬라임이" {
		t.Fatalf("resolver particle = %q", got)
	}
}

func TestDefaultANSIIsEmpty(t *testing.T) {
	got, err := Format("%C빨강%D", "31m", "0m")
	if err != nil {
		t.Fatal(err)
	}
	if got != "빨강" {
		t.Fatalf("default ANSI = %q", got)
	}
}

func TestEnabledANSI(t *testing.T) {
	r := Renderer{Options: Options{ANSI: true, Bright: true}}
	got, err := r.Format("%C빨강%D", "31m", "0m")
	if err != nil {
		t.Fatal(err)
	}
	want := "\x1b[1;31m빨강\x1b[1;0m"
	if got != want {
		t.Fatalf("enabled ANSI = %q, want %q", got, want)
	}
}

func TestANSIResetDefault(t *testing.T) {
	r := Renderer{Options: Options{ANSI: true}}
	got, err := r.Format("%C빨강%D", "31", "")
	if err != nil {
		t.Fatal(err)
	}
	want := "\x1b[0;31m빨강\x1b[0;0m"
	if got != want {
		t.Fatalf("default ANSI reset = %q, want %q", got, want)
	}
}

func TestRenderLegacyColors(t *testing.T) {
	if got := RenderLegacyColors("{빨붉은 검}과 {흰흰 방}", Options{}); got != "붉은 검과 흰 방" {
		t.Fatalf("legacy colors stripped = %q", got)
	}

	got := RenderLegacyColors("{빨붉은 검}", Options{ANSI: true})
	want := "\x1b[0;31m붉은 검\x1b[0;37m"
	if got != want {
		t.Fatalf("legacy colors ansi = %q, want %q", got, want)
	}
}

func TestMixedStandardAndCustomSpec(t *testing.T) {
	got, err := Format("hp:%03d 이름:%S hex:%#x 말:%s", 7, "무한", 255, "끝")
	if err != nil {
		t.Fatal(err)
	}
	want := "hp:007 이름:무한 hex:0xff 말:끝"
	if got != want {
		t.Fatalf("mixed spec = %q, want %q", got, want)
	}
}

func TestMixedStarStandardAndCustomSpec(t *testing.T) {
	got, err := Format("[%*d] %S [%.*s]", 4, 7, "검", 2, "abcdef")
	if err != nil {
		t.Fatal(err)
	}
	want := "[   7] 검 [ab]"
	if got != want {
		t.Fatalf("mixed star spec = %q, want %q", got, want)
	}
}

func TestFormatErrors(t *testing.T) {
	tests := []struct {
		name     string
		renderer Renderer
		format   string
		args     []any
		want     string
	}{
		{
			name:   "dangling percent",
			format: "진행률 %",
			want:   "dangling % at byte 10",
		},
		{
			name:   "unterminated standard spec",
			format: "%10",
			want:   "unterminated format specifier at byte 0",
		},
		{
			name:   "missing custom arg",
			format: "%S",
			want:   "%S at byte 0 missing argument",
		},
		{
			name:   "missing ansi reset arg",
			format: "%D",
			want:   "%D at byte 0 missing argument",
		},
		{
			name:   "missing standard arg",
			format: "%05d",
			want:   "%d at byte 0 missing argument",
		},
		{
			name:   "missing width arg",
			format: "%*d",
			want:   "%* at byte 0 missing argument",
		},
		{
			name:   "missing value after width",
			format: "%*d",
			args:   []any{4},
			want:   "%d at byte 0 missing argument",
		},
		{
			name:   "missing previous rendered text",
			format: "%j",
			args:   []any{"0"},
			want:   "%j at byte 0 has no previous rendered text",
		},
		{
			name:   "bad particle kind",
			format: "%S%j",
			args:   []any{"무한", struct{}{}},
			want:   "%j particle kind must be byte-like, got struct {}",
		},
		{
			name:   "missing resolver",
			format: "%2M",
			args:   []any{"mob:slime"},
			want:   "%M at byte 0 requires a Resolver",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.renderer.Format(tt.format, tt.args...)
			if err == nil {
				t.Fatalf("Format(%q, %v) = %q, want error %q", tt.format, tt.args, got, tt.want)
			}
			if err.Error() != tt.want {
				t.Fatalf("Format(%q, %v) error = %q, want %q", tt.format, tt.args, err.Error(), tt.want)
			}
		})
	}
}
