package cbin

import "testing"

func TestLayoutSizes(t *testing.T) {
	tests := map[string]int{
		"object":      ObjectSize,
		"creature":    CreatureSize,
		"room":        RoomSize,
		"exit":        ExitSize,
		"daily":       DailySize,
		"lasttime":    LasttimeSize,
		"board_index": BoardIndexSize,
	}
	want := map[string]int{
		"object":      352,
		"creature":    1184,
		"room":        480,
		"exit":        44,
		"daily":       8,
		"lasttime":    12,
		"board_index": 256,
	}
	for name, got := range tests {
		if got != want[name] {
			t.Fatalf("%s size = %d, want %d", name, got, want[name])
		}
	}
}

func TestDecodeMinimalObject(t *testing.T) {
	st, err := DecodeObjectFile(make([]byte, ObjectSize+4))
	if err != nil {
		t.Fatal(err)
	}
	if st.Objects != 1 || st.MaxDepth != 1 {
		t.Fatalf("stats = %+v", st)
	}
}

func TestDecodeObjectAllowTrailing(t *testing.T) {
	data := make([]byte, ObjectSize+4+12)
	st, err := DecodeObjectFileAllowTrailing(data)
	if err != nil {
		t.Fatal(err)
	}
	if st.Objects != 1 || st.TrailingBytes != 12 {
		t.Fatalf("stats = %+v", st)
	}
}

func TestDecodeMinimalCreature(t *testing.T) {
	st, err := DecodeCreatureFile(make([]byte, CreatureSize+4))
	if err != nil {
		t.Fatal(err)
	}
	if st.Creatures != 1 || st.MaxDepth != 1 {
		t.Fatalf("stats = %+v", st)
	}
}

func TestDecodeMinimalRoom(t *testing.T) {
	// room record + exit count + creature count + object count +
	// short/long/object description lengths.
	st, err := DecodeRoomFile(make([]byte, RoomSize+6*4))
	if err != nil {
		t.Fatal(err)
	}
	if st.Rooms != 1 || st.MaxDepth != 1 {
		t.Fatalf("stats = %+v", st)
	}
}
