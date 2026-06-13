package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xc0de1ab/muhan/internal/persist/jsonstore"
)

func TestWriteIndexCreatesIndexJSON(t *testing.T) {
	root := t.TempDir()
	outdir := filepath.Join(t.TempDir(), "export")
	generatedAt := time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC)

	if err := writeIndex(root, outdir, generatedAt); err != nil {
		t.Fatalf("write index: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outdir, "index.json"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var got jsonstore.ArtifactIndex
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode index: %v", err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}
	if got.SourceRoot != absRoot {
		t.Fatalf("sourceRoot = %q, want %q", got.SourceRoot, absRoot)
	}
	if !got.GeneratedAt.Equal(generatedAt) {
		t.Fatalf("generatedAt = %s, want %s", got.GeneratedAt, generatedAt)
	}
	if got.Counts.Files != 0 || got.Counts.Records != 0 || len(got.Files) != 0 {
		t.Fatalf("unexpected artifact inventory: %+v", got)
	}
}
