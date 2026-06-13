package snapshot

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/0xc0de1ab/muhan/internal/persist/cbin"
	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
)

func TestWriteJSONLPlayerFilenameEUCKR(t *testing.T) {
	root := t.TempDir()
	nameBytes, err := legacykr.EncodeEUCKR("가")
	if err != nil {
		t.Fatal(err)
	}
	shard := "가"
	writeFile(t, filepath.Join(root, "player", shard, string(nameBytes)), nil)

	records := writeAndDecode(t, root)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}

	got := records[0]
	if got.Kind != "player" || got.ID != "가" || got.DisplayName != "가" {
		t.Fatalf("player record = %+v", got)
	}
	if got.Metadata.LegacyEncoding != "euc-kr/cp949" {
		t.Fatalf("legacy encoding = %q", got.Metadata.LegacyEncoding)
	}
	if !bytes.Equal(got.Metadata.RawFields["filename"], nameBytes) {
		t.Fatalf("raw filename = % X, want % X", got.Metadata.RawFields["filename"], nameBytes)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("warnings = %v", got.Warnings)
	}
}

func TestWriteJSONLPlayerFilenameUTF8(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "player", "가", "가나다"), nil)

	records := writeAndDecode(t, root)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}

	got := records[0]
	if got.Kind != "player" || got.ID != "가나다" || got.DisplayName != "가나다" {
		t.Fatalf("player record = %+v", got)
	}
	if got.Metadata.LegacyEncoding != "utf-8" {
		t.Fatalf("legacy encoding = %q", got.Metadata.LegacyEncoding)
	}
	if !bytes.Equal(got.Metadata.RawFields["filename"], []byte("가나다")) {
		t.Fatalf("raw filename = % X", got.Metadata.RawFields["filename"])
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("warnings = %v", got.Warnings)
	}
}

func TestWriteJSONLRoomDecodeWarningContinues(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "rooms", "r00", "r00001"), []byte{1, 2, 3})
	writeFile(t, filepath.Join(root, "rooms", "r00", "r00002"), make([]byte, cbin.RoomSize+6*4))

	records := writeAndDecode(t, root)
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if records[0].Kind != "room" || records[0].ID != "r00001" {
		t.Fatalf("first room = %+v", records[0])
	}
	if len(records[0].Warnings) != 1 || !strings.Contains(records[0].Warnings[0], "decode room file") {
		t.Fatalf("first room warnings = %v", records[0].Warnings)
	}
	if records[1].Kind != "room" || records[1].ID != "r00002" {
		t.Fatalf("second room = %+v", records[1])
	}
	if len(records[1].Warnings) != 0 {
		t.Fatalf("second room warnings = %v", records[1].Warnings)
	}
}

func writeAndDecode(t *testing.T, root string) []Record {
	t.Helper()

	var buf bytes.Buffer
	if err := WriteJSONL(&buf, Options{Root: root}); err != nil {
		t.Fatalf("write JSONL: %v", err)
	}
	if !utf8.Valid(buf.Bytes()) {
		t.Fatalf("JSONL is not valid UTF-8: % X", buf.Bytes())
	}

	trimmed := bytes.TrimSpace(buf.Bytes())
	if len(trimmed) == 0 {
		return nil
	}

	var records []Record
	for _, line := range bytes.Split(trimmed, []byte("\n")) {
		if !json.Valid(line) {
			t.Fatalf("line is not valid JSON: %q", line)
		}
		var record Record
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("decode JSONL line: %v", err)
		}
		records = append(records, record)
	}
	return records
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
