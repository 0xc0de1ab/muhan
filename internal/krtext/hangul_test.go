package krtext

import "testing"

func TestIsAllHangulSyllables(t *testing.T) {
	tests := map[string]bool{
		"":      true,
		"무한":    true,
		"무한대전":  true,
		"무한2":   false,
		"ㄱ":     false,
		"muhan": false,
	}
	for input, want := range tests {
		if got := IsAllHangulSyllables(input); got != want {
			t.Fatalf("IsAllHangulSyllables(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestIsLegacyName(t *testing.T) {
	tests := map[string]bool{
		"":        false,
		"무한":      true,
		"무한대전":    true,
		"일이삼사오육":  true,
		"일이삼사오육칠": false,
		"무한2":     false,
		"ㄱ":       false,
	}
	for input, want := range tests {
		if got := IsLegacyName(input); got != want {
			t.Fatalf("IsLegacyName(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestHasFinalConsonant(t *testing.T) {
	tests := map[string]bool{
		"가":      false,
		"각":      true,
		"하늘":     true,
		"무한":     true,
		"무기":     false,
		"무한(관리)": true,
		"무기(관리)": false,
		"abc":    false,
	}
	for input, want := range tests {
		if got := HasFinalConsonant(input); got != want {
			t.Fatalf("HasFinalConsonant(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestFirstHangulBucket(t *testing.T) {
	tests := map[string]string{
		"가람": "가",
		"까치": "가",
		"나비": "나",
		"따라": "다",
		"힐러": "하",
		"":   "temp",
		"a가": "temp",
	}
	for input, want := range tests {
		if got := FirstHangulBucket(input); got != want {
			t.Fatalf("FirstHangulBucket(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParticle(t *testing.T) {
	tests := []struct {
		text string
		kind byte
		want string
	}{
		{"무한", '0', "은"},
		{"무기", '0', "는"},
		{"무한", '1', "이"},
		{"무기", '1', "가"},
		{"무한", '2', "과"},
		{"무기", '2', "와"},
		{"무한", '3', "을"},
		{"무기", '3', "를"},
		{"무한", '4', "으로"},
		{"무기", '4', "로"},
		{"무기", '9', ""},
	}
	for _, tt := range tests {
		if got := Particle(tt.text, tt.kind); got != tt.want {
			t.Fatalf("Particle(%q, %q) = %q, want %q", tt.text, tt.kind, got, tt.want)
		}
	}
}
