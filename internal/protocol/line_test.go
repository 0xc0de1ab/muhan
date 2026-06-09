package protocol

import (
	"slices"
	"testing"
)

func TestLineDisciplineEmitsUTF8Lines(t *testing.T) {
	l := NewLineDiscipline()
	got := l.Feed([]byte("고블린 때려\n"))
	want := []string{"고블린 때려"}
	if !slices.Equal(got, want) {
		t.Fatalf("Feed() = %#v, want %#v", got, want)
	}
}

func TestLineDisciplineKeepsUTF8ContinuationByteThatLooksLikeC1CSI(t *testing.T) {
	l := NewLineDiscipline()
	got := l.Feed([]byte("도움\n"))
	want := []string{"도움"}
	if !slices.Equal(got, want) {
		t.Fatalf("Feed() = %#v, want %#v", got, want)
	}
}

func TestLineDisciplineStripsStandaloneC1CSI(t *testing.T) {
	l := NewLineDiscipline()
	got := l.Feed([]byte{'a', 0x9b, 'b', '\n'})
	want := []string{"ab"}
	if !slices.Equal(got, want) {
		t.Fatalf("Feed() = %#v, want %#v", got, want)
	}
}

func TestLineDisciplineCoalescesCRLF(t *testing.T) {
	l := NewLineDiscipline()
	got := l.Feed([]byte("봐\r\n말\n"))
	want := []string{"봐", "말"}
	if !slices.Equal(got, want) {
		t.Fatalf("Feed() = %#v, want %#v", got, want)
	}
}

func TestLineDisciplineKeepsSplitUTF8Rune(t *testing.T) {
	l := NewLineDiscipline()
	bytes := []byte("한\n")
	if got := l.Feed(bytes[:2]); len(got) != 0 {
		t.Fatalf("first Feed() = %#v, want no lines", got)
	}
	got := l.Feed(bytes[2:])
	want := []string{"한"}
	if !slices.Equal(got, want) {
		t.Fatalf("second Feed() = %#v, want %#v", got, want)
	}
}

func TestLineDisciplineBackspaceDeletesOneRune(t *testing.T) {
	l := NewLineDiscipline()
	got := l.Feed([]byte("무한\x7f검\n"))
	want := []string{"무검"}
	if !slices.Equal(got, want) {
		t.Fatalf("Feed() = %#v, want %#v", got, want)
	}
}

func TestLineDisciplineDropsOldestBufferedRunesAtLimit(t *testing.T) {
	l := NewLineDiscipline(WithInputLimit(5))
	got := l.Feed([]byte("가나A\n"))
	want := []string{"나A"}
	if !slices.Equal(got, want) {
		t.Fatalf("Feed() = %#v, want %#v", got, want)
	}
}

func TestLineDisciplineStripsTelnetNegotiation(t *testing.T) {
	l := NewLineDiscipline()
	got := l.Feed([]byte{0xff, 0xfd, 0x01, 0xeb, 0xb4, 0x90, '\n'})
	want := []string{"봐"}
	if !slices.Equal(got, want) {
		t.Fatalf("Feed() = %#v, want %#v", got, want)
	}
}

func TestLineDisciplineStripsSplitTelnetNegotiation(t *testing.T) {
	l := NewLineDiscipline()
	if got := l.Feed([]byte{0xff}); len(got) != 0 {
		t.Fatalf("first Feed() = %#v, want no lines", got)
	}
	got := l.Feed([]byte{0xfd, 0x01, 0xeb, 0xb4, 0x90, '\n'})
	want := []string{"봐"}
	if !slices.Equal(got, want) {
		t.Fatalf("second Feed() = %#v, want %#v", got, want)
	}
}

func TestLineDisciplineCountsInvalidUTF8(t *testing.T) {
	l := NewLineDiscipline()
	got := l.Feed([]byte{'a', 0x80, 'b', '\n'})
	want := []string{"ab"}
	if !slices.Equal(got, want) {
		t.Fatalf("Feed() = %#v, want %#v", got, want)
	}
	if l.InvalidUTF8Bytes() != 1 {
		t.Fatalf("InvalidUTF8Bytes() = %d, want 1", l.InvalidUTF8Bytes())
	}
}
