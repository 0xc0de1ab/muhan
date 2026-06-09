package jsonstore

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	dirMode  os.FileMode = 0700
	fileMode os.FileMode = 0600
)

// ArtifactIndex is the top-level manifest for canonical migration artifacts.
type ArtifactIndex struct {
	GeneratedAt time.Time      `json:"generatedAt"`
	SourceRoot  string         `json:"sourceRoot"`
	Counts      ArtifactCounts `json:"counts"`
	Files       []ArtifactFile `json:"files"`
}

type ArtifactCounts struct {
	Files   int `json:"files"`
	Records int `json:"records"`
}

type ArtifactFile struct {
	Path    string `json:"path"`
	Format  string `json:"format"`
	Records int    `json:"records"`
}

func NewArtifactIndex(sourceRoot string, generatedAt time.Time, files []ArtifactFile) ArtifactIndex {
	copied := append([]ArtifactFile(nil), files...)
	sort.Slice(copied, func(i, j int) bool {
		return copied[i].Path < copied[j].Path
	})

	counts := ArtifactCounts{Files: len(copied)}
	for _, file := range copied {
		counts.Records += file.Records
	}
	if copied == nil {
		copied = []ArtifactFile{}
	}

	return ArtifactIndex{
		GeneratedAt: generatedAt.UTC(),
		SourceRoot:  sourceRoot,
		Counts:      counts,
		Files:       copied,
	}
}

func WriteJSON(path string, v any) error {
	return writeAtomically(path, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	})
}

func WriteJSONL[T any](path string, records []T) error {
	return writeAtomically(path, func(w io.Writer) error {
		bw := bufio.NewWriter(w)
		enc := json.NewEncoder(bw)
		for _, record := range records {
			if err := enc.Encode(record); err != nil {
				return err
			}
		}
		return bw.Flush()
	})
}

func WriteBytes(path string, data []byte) error {
	return writeAtomically(path, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}

func writeAtomically(path string, encode func(io.Writer) error) error {
	if path == "" {
		return errors.New("jsonstore: empty output path")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("create output directory %q: %w", dir, err)
	}

	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file for %q: %w", path, err)
	}

	tempPath := temp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	if err := temp.Chmod(fileMode); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set temp file mode %q: %w", tempPath, err)
	}
	if err := encode(temp); err != nil {
		_ = temp.Close()
		return fmt.Errorf("encode %q: %w", path, err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temp file %q: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temp file %q: %w", tempPath, err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename temp file to %q: %w", path, err)
	}
	committed = true

	if err := syncDir(dir); err != nil {
		return fmt.Errorf("sync output directory %q: %w", dir, err)
	}
	return nil
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
