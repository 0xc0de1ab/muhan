package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/persist/cbin"
)

func TestScanCountsRoomsWarningsAndErrors(t *testing.T) {
	root := t.TempDir()
	roomDir := filepath.Join(root, "rooms", "r00")
	if err := os.MkdirAll(roomDir, 0700); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(roomDir, "r00001"), minimalRoomData(1, 0))
	writeFile(t, filepath.Join(roomDir, "r00002"), malformedExitRoomData(2))
	writeFile(t, filepath.Join(roomDir, "r00003"), []byte{1, 2, 3})

	summary, err := scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Rooms != 2 {
		t.Fatalf("rooms = %d, want 2", summary.Rooms)
	}
	if summary.Errors != 1 {
		t.Fatalf("errors = %d, want 1", summary.Errors)
	}
	if summary.Warnings == 0 {
		t.Fatal("expected warning count")
	}
}

func minimalRoomData(number int, exitCount int) []byte {
	data := makeRoomRecord(number)
	data = appendInt32(data, exitCount)
	data = appendInt32(data, 0)
	data = appendInt32(data, 0)
	data = appendDescription(data, nil)
	data = appendDescription(data, nil)
	data = appendDescription(data, nil)
	return data
}

func malformedExitRoomData(number int) []byte {
	data := makeRoomRecord(number)
	return appendInt32(data, -1)
}

func makeRoomRecord(number int) []byte {
	data := make([]byte, cbin.RoomSize)
	binary.LittleEndian.PutUint16(data, uint16(int16(number)))
	return data
}

func appendInt32(data []byte, n int) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(int32(n)))
	return append(data, buf[:]...)
}

func appendDescription(data []byte, desc []byte) []byte {
	data = appendInt32(data, len(desc))
	return append(data, desc...)
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()

	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
