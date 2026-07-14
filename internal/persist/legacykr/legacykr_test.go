package legacykr

import (
	"errors"
	"testing"
)

func TestDecodeEUCKR(t *testing.T) {
	got, err := DecodeEUCKR([]byte{0xB0, 0xA1, 0xB3, 0xAA, 0xB4, 0xD9})
	if err != nil {
		t.Fatal(err)
	}
	if got != "가나다" {
		t.Fatalf("DecodeEUCKR = %q, want %q", got, "가나다")
	}
}

func TestDecodeEUCKRStripsPersistedTerminalControls(t *testing.T) {
	// The C server writes colour codes as the 7-bit ESC '[' CSI form (ESC[1;33m).
	got, err := DecodeEUCKR([]byte{0x1b, '[', '1', ';', '3', '3', 'm', 0xB9, 0xF8, 0xB0, 0xB3})
	if err != nil {
		t.Fatal(err)
	}
	if got != "번개" {
		t.Fatalf("DecodeEUCKR with controls = %q, want 번개", got)
	}
}

// TestDecodeEUCKRPreservesCP949LeadByte0x9b guards against treating 0x9b as an
// 8-bit CSI introducer: 0x9b is a valid CP949 lead byte (the 0x9bXX Hangul rows
// in the legacy kstbl.h), and 0x9b61 decodes to "쌳". The old stripper deleted it
// because 0x61 falls in the CSI final-byte range.
func TestDecodeEUCKRPreservesCP949LeadByte0x9b(t *testing.T) {
	got, err := DecodeEUCKR([]byte{0x9b, 0x61, 0xB9, 0xF8, 0xB0, 0xB3})
	if err != nil {
		t.Fatal(err)
	}
	if got != "쌳번개" {
		t.Fatalf("DecodeEUCKR = %q, want 쌳번개 (0x9b-led CP949 char must survive)", got)
	}
}

func TestEncodeEUCKR(t *testing.T) {
	got, err := EncodeEUCKR("가나다")
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0xB0, 0xA1, 0xB3, 0xAA, 0xB4, 0xD9}
	if string(got) != string(want) {
		t.Fatalf("EncodeEUCKR = % X, want % X", got, want)
	}
}

func TestRoundTrip(t *testing.T) {
	input := "무한대전"
	encoded, err := EncodeEUCKR(input)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeEUCKR(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != input {
		t.Fatalf("round trip = %q, want %q", decoded, input)
	}
}

func TestDecodePathBytes(t *testing.T) {
	got, err := DecodePathBytes([]byte{'p', 'l', 'a', 'y', 'e', 'r', '/', 0xB0, 0xA1})
	if err != nil {
		t.Fatal(err)
	}
	if got != "player/가" {
		t.Fatalf("DecodePathBytes = %q, want %q", got, "player/가")
	}
}

func TestValidUTF8OrDecode(t *testing.T) {
	got, err := ValidUTF8OrDecode([]byte("무한"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "무한" {
		t.Fatalf("ValidUTF8OrDecode UTF-8 = %q, want %q", got, "무한")
	}

	got, err = ValidUTF8OrDecode([]byte{0xB9, 0xAB, 0xC7, 0xD1})
	if err != nil {
		t.Fatal(err)
	}
	if got != "무한" {
		t.Fatalf("ValidUTF8OrDecode EUC-KR = %q, want %q", got, "무한")
	}
}

func TestDecodeInvalidByte(t *testing.T) {
	_, err := DecodeEUCKRContext(Context{Path: "player/raw", Field: "name"}, []byte{0xB0})
	if err == nil {
		t.Fatal("expected error")
	}
	var conv *ConversionError
	if !errors.As(err, &conv) {
		t.Fatalf("error type = %T, want *ConversionError", err)
	}
	if conv.Path != "player/raw" || conv.Field != "name" || conv.Offset != 0 {
		t.Fatalf("context = %+v", conv)
	}
}

func TestEncodeInvalidUTF8(t *testing.T) {
	_, err := EncodeEUCKR(string([]byte{0xff}))
	if err == nil {
		t.Fatal("expected error")
	}
	var conv *ConversionError
	if !errors.As(err, &conv) {
		t.Fatalf("error type = %T, want *ConversionError", err)
	}
	if conv.Op != "encode" {
		t.Fatalf("op = %q, want encode", conv.Op)
	}
}

func TestTrimCString(t *testing.T) {
	got := TrimCString([]byte{'a', 'b', 0, 'c'})
	if string(got) != "ab" {
		t.Fatalf("TrimCString = %q, want %q", got, "ab")
	}
}
