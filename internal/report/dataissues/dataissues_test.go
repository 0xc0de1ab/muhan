package dataissues

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/persist/cbin"
)

func TestScanReportsZeroByteRoom(t *testing.T) {
	root := t.TempDir()
	roomDir := filepath.Join(root, "rooms", "0")
	if err := os.MkdirAll(roomDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roomDir, "r00000"), nil, 0600); err != nil {
		t.Fatal(err)
	}

	report, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Issues) != 1 {
		t.Fatalf("issues = %d, want 1: %+v", len(report.Issues), report.Issues)
	}
	issue := report.Issues[0]
	if issue.Path != "rooms/0/r00000" {
		t.Fatalf("path = %q, want rooms/0/r00000", issue.Path)
	}
	if issue.Kind != KindRoom {
		t.Fatalf("kind = %q, want %q", issue.Kind, KindRoom)
	}
	if issue.Size != 0 {
		t.Fatalf("size = %d, want 0", issue.Size)
	}
	if !strings.Contains(issue.Hint, "0-byte room") {
		t.Fatalf("hint = %q, want 0-byte room hint", issue.Hint)
	}
}

func TestScanReportsMalformedRoomDescriptionEOF(t *testing.T) {
	root := t.TempDir()
	roomDir := filepath.Join(root, "rooms", "0")
	if err := os.MkdirAll(roomDir, 0700); err != nil {
		t.Fatal(err)
	}

	data := make([]byte, cbin.RoomSize+12+4+2)
	binary.LittleEndian.PutUint32(data[cbin.RoomSize+12:cbin.RoomSize+16], 5)
	if err := os.WriteFile(filepath.Join(roomDir, "r00001"), data, 0600); err != nil {
		t.Fatal(err)
	}

	report, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Issues) != 1 {
		t.Fatalf("issues = %d, want 1: %+v", len(report.Issues), report.Issues)
	}
	issue := report.Issues[0]
	if issue.Path != "rooms/0/r00001" {
		t.Fatalf("path = %q, want rooms/0/r00001", issue.Path)
	}
	if issue.Kind != KindRoom {
		t.Fatalf("kind = %q, want %q", issue.Kind, KindRoom)
	}
	if issue.Size != int64(len(data)) {
		t.Fatalf("size = %d, want %d", issue.Size, len(data))
	}
	if !strings.Contains(issue.Message, "room description 0 bytes") {
		t.Fatalf("message = %q, want description bytes EOF", issue.Message)
	}
	if !strings.Contains(issue.Hint, "length-prefixed description EOF") {
		t.Fatalf("hint = %q, want description EOF hint", issue.Hint)
	}
}

func TestScanReportsTruncatedNestedObject(t *testing.T) {
	root := t.TempDir()
	roomDir := filepath.Join(root, "rooms", "0")
	if err := os.MkdirAll(roomDir, 0700); err != nil {
		t.Fatal(err)
	}

	data := make([]byte, cbin.RoomSize+12+cbin.ObjectSize)
	binary.LittleEndian.PutUint32(data[cbin.RoomSize+8:cbin.RoomSize+12], 1)
	if err := os.WriteFile(filepath.Join(roomDir, "r00002"), data, 0600); err != nil {
		t.Fatal(err)
	}

	report, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Issues) != 1 {
		t.Fatalf("issues = %d, want 1: %+v", len(report.Issues), report.Issues)
	}
	issue := report.Issues[0]
	if issue.Kind != KindRoom {
		t.Fatalf("kind = %q, want %q", issue.Kind, KindRoom)
	}
	if !strings.Contains(issue.Message, "object child count") {
		t.Fatalf("message = %q, want object child count EOF", issue.Message)
	}
	if !strings.Contains(issue.Hint, "truncated nested object") {
		t.Fatalf("hint = %q, want nested object hint", issue.Hint)
	}
}
