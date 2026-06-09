package commandparse

import "testing"

func TestParseMovesFinalTokenToCommandSlot(t *testing.T) {
	got := Parse("고블린 때려")
	assertSlots(t, got, []slot{
		{str: "때려", val: 1},
		{str: "고블린", val: 1},
	})
}

func TestParseUsesDefaultVerbForTrailingSpaceAndPunctuation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []slot
	}{
		{
			name:  "space",
			input: "고블린 ",
			want: []slot{
				{str: DefaultVerb, val: 1},
				{str: "고블린", val: 1},
			},
		},
		{
			name:  "period",
			input: "고블린.",
			want: []slot{
				{str: DefaultVerb, val: 1},
				{str: "고블린.", val: 1},
			},
		},
		{
			name:  "exclamation",
			input: "고블린!",
			want: []slot{
				{str: DefaultVerb, val: 1},
				{str: "고블린!", val: 1},
			},
		},
		{
			name:  "question",
			input: "고블린?",
			want: []slot{
				{str: DefaultVerb, val: 1},
				{str: "고블린?", val: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSlots(t, Parse(tt.input), tt.want)
		})
	}
}

func TestParseSupportsSpaceAndHashDelimiters(t *testing.T) {
	got := Parse("고블린#2#때려")
	assertSlots(t, got, []slot{
		{str: "때려", val: 1},
		{str: "고블린", val: 2},
	})
}

func TestParseMapsNumericTokensIncludingNegativeValues(t *testing.T) {
	got := Parse("고블린 2 오크 -3 때려")
	assertSlots(t, got, []slot{
		{str: "때려", val: 1},
		{str: "고블린", val: 2},
		{str: "오크", val: -3},
	})
}

func TestParsePreservesUTF8TokensWithoutByteTruncation(t *testing.T) {
	longHangul := "가나다라마바사아자차카타파하"
	got := Parse(longHangul + " 명령")
	assertSlots(t, got, []slot{
		{str: "명령", val: 1},
		{str: longHangul, val: 1},
	})
}

func TestParseEmptyInput(t *testing.T) {
	got := Parse("")
	if got.Num != 0 {
		t.Fatalf("Parse(\"\").Num = %d, want 0", got.Num)
	}
}

func TestParseCommandFirstUsesInitialTokenAsCommand(t *testing.T) {
	got := ParseCommandFirst("봐 사물함")
	assertSlots(t, got, []slot{
		{str: "봐", val: 1},
		{str: "사물함", val: 1},
	})
}

func TestParseCommandFirstPreservesOrdinals(t *testing.T) {
	got := ParseCommandFirst("봐 사물함 2")
	assertSlots(t, got, []slot{
		{str: "봐", val: 1},
		{str: "사물함", val: 2},
	})
}

func TestParseCommandFirstSupportsHashDelimiter(t *testing.T) {
	got := ParseCommandFirst("봐#사물함#2")
	assertSlots(t, got, []slot{
		{str: "봐", val: 1},
		{str: "사물함", val: 2},
	})
}

type slot struct {
	str string
	val int64
}

func assertSlots(t *testing.T, got Command, want []slot) {
	t.Helper()

	if got.Num != len(want) {
		t.Fatalf("Num = %d, want %d; command = %#v", got.Num, len(want), got)
	}
	for i, wantSlot := range want {
		if got.Str[i] != wantSlot.str || got.Val[i] != wantSlot.val {
			t.Fatalf("slot %d = (%q, %d), want (%q, %d); command = %#v",
				i, got.Str[i], got.Val[i], wantSlot.str, wantSlot.val, got)
		}
	}
}
