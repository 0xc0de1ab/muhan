package jsonstore

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteJSONTempDirAndPermissions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "export", "index.json")
	generatedAt := time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC)
	index := NewArtifactIndex("/legacy/muhan", generatedAt, []ArtifactFile{
		{Path: "rooms.jsonl", Format: "jsonl", Records: 2},
	})

	if err := WriteJSON(path, index); err != nil {
		t.Fatalf("write JSON: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read JSON: %v", err)
	}
	var got ArtifactIndex
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if got.SourceRoot != "/legacy/muhan" || got.Counts.Files != 1 || got.Counts.Records != 2 {
		t.Fatalf("index = %+v", got)
	}
	if len(got.Files) != 1 || got.Files[0].Path != "rooms.jsonl" {
		t.Fatalf("files = %+v", got.Files)
	}

	assertMode(t, filepath.Join(root, "export"), dirMode)
	assertMode(t, path, fileMode)
}

func TestWriteJSONOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.json")

	if err := WriteJSON(path, map[string]int{"version": 1}); err != nil {
		t.Fatalf("write first JSON: %v", err)
	}
	if err := WriteJSON(path, map[string]int{"version": 2}); err != nil {
		t.Fatalf("overwrite JSON: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read JSON: %v", err)
	}
	if bytes.Contains(data, []byte(`"version": 1`)) {
		t.Fatalf("file still contains old content: %s", data)
	}

	var got map[string]int
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if got["version"] != 2 {
		t.Fatalf("version = %d, want 2", got["version"])
	}
}

func TestWriteBytesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.sql")

	if err := WriteBytes(path, []byte("select 1;\n")); err != nil {
		t.Fatalf("write first bytes: %v", err)
	}
	if err := WriteBytes(path, []byte("select 2;\n")); err != nil {
		t.Fatalf("overwrite bytes: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bytes: %v", err)
	}
	if string(data) != "select 2;\n" {
		t.Fatalf("content = %q", data)
	}
	assertMode(t, path, fileMode)
}

func TestWriteJSONInvalidPath(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(parent, []byte("x"), 0600); err != nil {
		t.Fatalf("create parent file: %v", err)
	}

	err := WriteJSON(filepath.Join(parent, "index.json"), map[string]string{"ok": "false"})
	if err == nil {
		t.Fatal("WriteJSON succeeded for path below a regular file")
	}
}

func TestWriteJSONLLinesCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.jsonl")
	records := []any{
		map[string]any{"id": "room-1"},
		map[string]any{"id": "room-2"},
		map[string]any{"id": "room-3"},
	}

	if err := WriteJSONL(path, records); err != nil {
		t.Fatalf("write JSONL: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read JSONL: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != len(records) {
		t.Fatalf("lines = %d, want %d: %q", len(lines), len(records), data)
	}
	for _, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Fatalf("line is not valid JSON: %q", line)
		}
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %v, want %v", path, got, want)
	}
}
