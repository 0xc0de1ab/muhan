package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

// The suicide sink deletes files by player display/account name. Those
// names come from player records (including legacy data), so a name with
// "../" segments must not be able to delete files outside the data root.
// These tests pin that confinement: traversal names are refused while
// legitimate in-root files are still removed.

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %q to exist: %v", path, err)
	}
}

func assertFileRemoved(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file %q to be removed, stat err = %v", path, err)
	}
}

func TestServerSuicideSinkPlayerFilesConfinedToRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "data")

	// Legitimate in-root files for a normal player name.
	legitSave := filepath.Join(root, "player", "temp", "hero")
	legitJSON := filepath.Join(root, "player", "json", "hero.json")
	writeTestFile(t, legitSave)
	writeTestFile(t, legitJSON)

	// Canary outside the root. The traversal display name below resolves
	// exactly here under an unconfined filepath.Join.
	canary := filepath.Join(base, "canary_player")
	writeTestFile(t, canary)

	var logs []string
	sink := serverSuicideSink{
		root: root,
		logf: func(format string, args ...any) {
			logs = append(logs, format)
		},
	}

	hero := model.Player{ID: "player:hero", DisplayName: "hero"}
	if err := sink.removePlayerFiles("player:hero", hero); err != nil {
		t.Fatalf("removePlayerFiles(hero) error = %v", err)
	}
	assertFileRemoved(t, legitSave)
	assertFileRemoved(t, legitJSON)

	evil := model.Player{ID: "player:evil", DisplayName: "../../../canary_player"}
	if err := sink.removePlayerFiles("player:evil", evil); err != nil {
		t.Fatalf("removePlayerFiles(evil) error = %v", err)
	}
	assertFileExists(t, canary)
	if !serverSuicideTestLogContains(logs, "refusing to remove path outside root") {
		t.Fatalf("logs = %#v, want an outside-root refusal warning", logs)
	}
}

func TestServerSuicideSinkBankFilesConfinedToRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "data")

	legitBank := filepath.Join(root, "player", "bank", "hero")
	legitBankJSON := filepath.Join(root, "player", "bank", "json", "hero.json")
	writeTestFile(t, legitBank)
	writeTestFile(t, legitBankJSON)

	canary := filepath.Join(base, "canary_bank")
	writeTestFile(t, canary)

	sink := serverSuicideSink{
		root: root,
		logf: func(string, ...any) {},
	}

	hero := model.Player{ID: "player:hero", DisplayName: "hero"}
	if err := sink.removeBankFiles("player:hero", hero); err != nil {
		t.Fatalf("removeBankFiles(hero) error = %v", err)
	}
	assertFileRemoved(t, legitBank)
	assertFileRemoved(t, legitBankJSON)

	evil := model.Player{ID: "player:evil", DisplayName: "../../../canary_bank"}
	if err := sink.removeBankFiles("player:evil", evil); err != nil {
		t.Fatalf("removeBankFiles(evil) error = %v", err)
	}
	assertFileExists(t, canary)
}

func TestServerSuicideSinkConfinedPlayerPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	sink := serverSuicideSink{root: root, logf: func(string, ...any) {}}

	if path, ok := sink.confinedPlayerPath(root, "player", "temp", "hero"); !ok || path != filepath.Join(root, "player", "temp", "hero") {
		t.Fatalf("confinedPlayerPath(normal) = %q, %v; want in-root path, true", path, ok)
	}
	for _, parts := range [][]string{
		{"player", "temp", "../../../canary"},
		{"player", "bank", "..", "..", "..", "canary"},
		{"..", "canary"},
		{"/etc", "passwd"},
	} {
		if path, ok := sink.confinedPlayerPath(root, parts...); ok {
			t.Fatalf("confinedPlayerPath(%s) = %q, true; want refusal", strings.Join(parts, "/"), path)
		}
	}
}
