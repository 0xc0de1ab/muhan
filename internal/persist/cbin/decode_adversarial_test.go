package cbin

import (
	"encoding/binary"
	"strings"
	"testing"
)

// These tests feed hostile buffers to the cbin decoders. The decoders parse
// legacy data files that may be corrupted or attacker-controlled, so every
// malformed input must produce an error from the intended guard — never a
// panic, an out-of-bounds access, or unbounded recursion/allocation.

func putCount(buf []byte, off int, v int32) {
	binary.LittleEndian.PutUint32(buf[off:], uint32(v))
}

// objectWithChildren builds a buffer holding one object record whose child
// count field is count, with nothing after it.
func objectWithCount(count int32) []byte {
	buf := make([]byte, ObjectSize+4)
	putCount(buf, ObjectSize, count)
	return buf
}

func creatureWithCount(count int32) []byte {
	buf := make([]byte, CreatureSize+4)
	putCount(buf, CreatureSize, count)
	return buf
}

// roomHeader builds a room record followed by the exit/creature/object
// count fields (all zero unless overridden by the caller afterwards), plus
// room for the three description length fields.
func roomHeader() []byte {
	return make([]byte, RoomSize+4+4+4+3*4)
}

func deeplyNestedObjects(depth int) []byte {
	var buf []byte
	for i := 0; i < depth; i++ {
		rec := make([]byte, ObjectSize+4)
		putCount(rec, ObjectSize, 1)
		buf = append(buf, rec...)
	}
	// Innermost object with no children so a decoder that ignored the
	// depth guard could terminate cleanly instead of running out of data.
	buf = append(buf, objectWithCount(0)...)
	return buf
}

func TestDecodeAdversarialInputs(t *testing.T) {
	tests := []struct {
		name    string
		decode  func([]byte) (Stats, error)
		data    []byte
		wantErr string
	}{
		// --- object decoder ---
		{
			name:    "object empty input",
			decode:  DecodeObjectFile,
			data:    nil,
			wantErr: "object record",
		},
		{
			name:    "object truncated record",
			decode:  DecodeObjectFile,
			data:    make([]byte, ObjectSize-1),
			wantErr: "object record",
		},
		{
			name:    "object truncated child count",
			decode:  DecodeObjectFile,
			data:    make([]byte, ObjectSize+2),
			wantErr: "object child count",
		},
		{
			name:    "object negative child count",
			decode:  DecodeObjectFile,
			data:    objectWithCount(-1),
			wantErr: "invalid object child count",
		},
		{
			name:    "object child count above max",
			decode:  DecodeObjectFile,
			data:    objectWithCount(MaxObjectChildren + 1),
			wantErr: "invalid object child count",
		},
		{
			name:    "object child declared but missing",
			decode:  DecodeObjectFile,
			data:    objectWithCount(1),
			wantErr: "object child 1/1",
		},
		{
			name:    "object recursion depth exceeded",
			decode:  DecodeObjectFile,
			data:    deeplyNestedObjects(MaxRecursionDepth + 8),
			wantErr: "recursion depth exceeds",
		},
		{
			name:    "object trailing garbage rejected",
			decode:  DecodeObjectFile,
			data:    append(objectWithCount(0), 0xAA, 0xBB, 0xCC),
			wantErr: "trailing",
		},
		// --- creature decoder ---
		{
			name:    "creature truncated record",
			decode:  DecodeCreatureFile,
			data:    make([]byte, CreatureSize-1),
			wantErr: "creature record",
		},
		{
			name:    "creature negative inventory count",
			decode:  DecodeCreatureFile,
			data:    creatureWithCount(-1),
			wantErr: "invalid creature inventory count",
		},
		{
			name:    "creature inventory count above max",
			decode:  DecodeCreatureFile,
			data:    creatureWithCount(MaxCreatureItems + 1),
			wantErr: "invalid creature inventory count",
		},
		{
			name:    "creature inventory object truncated",
			decode:  DecodeCreatureFile,
			data:    creatureWithCount(1),
			wantErr: "creature inventory object 1/1",
		},
		// --- room decoder ---
		{
			name:    "room truncated record",
			decode:  DecodeRoomFile,
			data:    make([]byte, RoomSize/2),
			wantErr: "room record",
		},
		{
			name: "room negative exit count",
			decode: func(b []byte) (Stats, error) {
				putCount(b, RoomSize, -1)
				return DecodeRoomFile(b)
			},
			data:    roomHeader(),
			wantErr: "invalid room exit count",
		},
		{
			name: "room exit count above max",
			decode: func(b []byte) (Stats, error) {
				putCount(b, RoomSize, MaxRoomExits+1)
				return DecodeRoomFile(b)
			},
			data:    roomHeader(),
			wantErr: "invalid room exit count",
		},
		{
			name: "room negative creature count",
			decode: func(b []byte) (Stats, error) {
				putCount(b, RoomSize+4, -1)
				return DecodeRoomFile(b)
			},
			data:    roomHeader(),
			wantErr: "invalid room creature count",
		},
		{
			name: "room negative object count",
			decode: func(b []byte) (Stats, error) {
				putCount(b, RoomSize+8, -1)
				return DecodeRoomFile(b)
			},
			data:    roomHeader(),
			wantErr: "invalid room object count",
		},
		{
			name: "room negative description length",
			decode: func(b []byte) (Stats, error) {
				putCount(b, RoomSize+12, -1)
				return DecodeRoomFile(b)
			},
			data:    roomHeader(),
			wantErr: "invalid room description 0 length",
		},
		{
			name: "room description length above max",
			decode: func(b []byte) (Stats, error) {
				putCount(b, RoomSize+12, MaxDescriptionBytes+1)
				return DecodeRoomFile(b)
			},
			data:    roomHeader(),
			wantErr: "invalid room description 0 length",
		},
		{
			name: "room description bytes truncated",
			decode: func(b []byte) (Stats, error) {
				putCount(b, RoomSize+12, 64)
				return DecodeRoomFile(b)
			},
			data:    roomHeader(),
			wantErr: "room description 0 bytes",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("decoder panicked on adversarial input: %v", r)
				}
			}()
			_, err := tc.decode(tc.data)
			if err == nil {
				t.Fatal("expected error for adversarial input, got nil")
			}
			if tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

// FuzzDecodeObjectFile mutates seed buffers through all three strict
// decoders. The only assertion is the security invariant: no input may ever
// panic a decoder. Errors are always acceptable.
func FuzzDecodeObjectFile(f *testing.F) {
	f.Add(objectWithCount(0))
	f.Add(objectWithCount(-1))
	f.Add(objectWithCount(MaxObjectChildren + 1))
	f.Add(creatureWithCount(0))
	f.Add(roomHeader())
	f.Add(deeplyNestedObjects(4))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		// Cap the input so fuzz iterations stay fast; the guards under
		// test are size-independent.
		if len(data) > 1<<16 {
			data = data[:1<<16]
		}
		_, _ = DecodeObjectFile(data)
		_, _ = DecodeCreatureFile(data)
		_, _ = DecodeRoomFile(data)
	})
}
