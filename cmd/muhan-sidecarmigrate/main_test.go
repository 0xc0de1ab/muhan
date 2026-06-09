package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestRunDryRunDoesNotRewriteSource(t *testing.T) {
	root := t.TempDir()
	playerPath := filepath.Join(root, "player", "json", "alice.json")
	writeJSONFile(t, playerPath, state.PlayerSaveData{
		SchemaVersion: 1,
		Player:        model.Player{ID: "player:alice"},
	})

	var stdout, stderr bytes.Buffer
	code := run([]string{"-root", root, "-details"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run exit = %d, stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "mode=DRY-RUN") || !strings.Contains(out, "would be rewritten") {
		t.Fatalf("dry-run output did not describe would-rewrite policy:\n%s", out)
	}
	if !strings.Contains(out, playerPath) {
		t.Fatalf("dry-run output path = %q, want source path %q", out, playerPath)
	}

	var got state.PlayerSaveData
	readJSONFile(t, playerPath, &got)
	if got.SchemaVersion != 1 {
		t.Fatalf("dry-run rewrote source schemaVersion = %d, want 1", got.SchemaVersion)
	}
}

func TestRunExecuteRewritesSource(t *testing.T) {
	root := t.TempDir()
	roomPath := filepath.Join(root, "room", "json", "floor1.objects.json")
	writeJSONFile(t, roomPath, state.RoomObjectsSave{
		SchemaVersion: 1,
		RoomID:        "room:floor1",
	})

	var stdout, stderr bytes.Buffer
	code := run([]string{"-root", root, "-execute", "-details"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run exit = %d, stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "mode=EXECUTE") || !strings.Contains(out, "rewritten") {
		t.Fatalf("execute output did not describe rewrite policy:\n%s", out)
	}

	var got state.RoomObjectsSave
	readJSONFile(t, roomPath, &got)
	if got.SchemaVersion != state.CurrentSaveSchemaVersion {
		t.Fatalf("execute schemaVersion = %d, want %d", got.SchemaVersion, state.CurrentSaveSchemaVersion)
	}
}

func TestRunExecuteReportsBoardAndFamilyNewsRewrites(t *testing.T) {
	root := t.TempDir()
	boardPath := filepath.Join(root, "board", "json", "info.json")
	familyNewsPath := filepath.Join(root, "player", "family", "json", "family_news_7.json")
	writeJSONFile(t, boardPath, state.BoardPostsSave{
		SchemaVersion: 1,
		BoardDir:      "info",
	})
	writeJSONFile(t, familyNewsPath, state.FamilyNewsSave{
		SchemaVersion: 1,
		FamilyID:      7,
		Content:       "notice",
	})

	var stdout, stderr bytes.Buffer
	code := run([]string{"-root", root, "-execute", "-json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run exit = %d, stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	var got migrationSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("summary JSON: %v\n%s", err, stdout.String())
	}
	if got.Migrated != 2 {
		t.Fatalf("migrated = %d, want 2; summary = %+v", got.Migrated, got)
	}
	if got.ByType["board"] != 1 {
		t.Fatalf("byType[board] = %d, want 1; summary = %+v", got.ByType["board"], got)
	}
	if got.ByType["familynews"] != 1 {
		t.Fatalf("byType[familynews] = %d, want 1; summary = %+v", got.ByType["familynews"], got)
	}
	if !hasMigrationDetail(got.Details, "board", boardPath) {
		t.Fatalf("details missing board rewrite for %s: %+v", boardPath, got.Details)
	}
	if !hasMigrationDetail(got.Details, "familynews", familyNewsPath) {
		t.Fatalf("details missing family news rewrite for %s: %+v", familyNewsPath, got.Details)
	}

	var board state.BoardPostsSave
	readJSONFile(t, boardPath, &board)
	if board.SchemaVersion != state.CurrentSaveSchemaVersion {
		t.Fatalf("rewritten board schemaVersion = %d, want %d", board.SchemaVersion, state.CurrentSaveSchemaVersion)
	}

	var familyNews state.FamilyNewsSave
	readJSONFile(t, familyNewsPath, &familyNews)
	if familyNews.SchemaVersion != state.CurrentSaveSchemaVersion {
		t.Fatalf("rewritten family news schemaVersion = %d, want %d", familyNews.SchemaVersion, state.CurrentSaveSchemaVersion)
	}
}

func TestRunReportsFutureSchemaAsError(t *testing.T) {
	root := t.TempDir()
	boardPath := filepath.Join(root, "board", "json", "info.json")
	writeJSONFile(t, boardPath, state.BoardPostsSave{
		SchemaVersion: state.CurrentSaveSchemaVersion + 1,
		BoardDir:      "info",
	})

	var stdout, stderr bytes.Buffer
	code := run([]string{"-root", root, "-json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run exit = %d, want 1; stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	var got migrationSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("summary JSON: %v\n%s", err, stdout.String())
	}
	if len(got.Errors) != 1 {
		t.Fatalf("errors = %#v, want one future schema error", got.Errors)
	}
	if !strings.Contains(got.Errors[0], "unsupported future schema version") {
		t.Fatalf("future schema error = %q", got.Errors[0])
	}
	if !strings.Contains(got.Errors[0], boardPath) {
		t.Fatalf("future schema path = %q, want source path %q", got.Errors[0], boardPath)
	}
}

func hasMigrationDetail(details []state.SidecarMigrationDetail, typ, path string) bool {
	for _, detail := range details {
		if detail.Type == typ &&
			detail.Path == path &&
			detail.FromVer == 1 &&
			detail.ToVer == state.CurrentSaveSchemaVersion &&
			detail.Repaired {
			return true
		}
	}
	return false
}

func writeJSONFile(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readJSONFile(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
}
