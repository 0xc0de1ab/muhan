package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
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

func TestRunExecuteActualByteRewrite(t *testing.T) {
	root := t.TempDir()
	playerPath := filepath.Join(root, "player", "json", "alice.json")
	writeJSONFile(t, playerPath, state.PlayerSaveData{
		SchemaVersion: 1,
		Player:        model.Player{ID: "player:alice"},
	})

	beforeBytes, err := os.ReadFile(playerPath)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"-root", root, "-execute", "-json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run exit = %d, stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	var summary migrationSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("summary JSON: %v\n%s", err, stdout.String())
	}
	if summary.Migrated != 1 {
		t.Fatalf("migrated = %d, want 1; summary = %+v", summary.Migrated, summary)
	}

	afterBytes, err := os.ReadFile(playerPath)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}

	if bytes.Equal(beforeBytes, afterBytes) {
		t.Fatal("file bytes unchanged after -execute; expected on-disk rewrite")
	}

	var got state.PlayerSaveData
	if err := json.Unmarshal(afterBytes, &got); err != nil {
		t.Fatalf("post-execute file failed to parse: %v", err)
	}
	if got.SchemaVersion != state.CurrentSaveSchemaVersion {
		t.Fatalf("post-execute schemaVersion = %d, want %d", got.SchemaVersion, state.CurrentSaveSchemaVersion)
	}
}

func TestRunExecuteIdempotent(t *testing.T) {
	root := t.TempDir()
	playerPath := filepath.Join(root, "player", "json", "alice.json")
	writeJSONFile(t, playerPath, state.PlayerSaveData{
		SchemaVersion: 1,
		Player:        model.Player{ID: "player:alice"},
	})

	// First -execute: should migrate 1 file.
	var stdout1, stderr1 bytes.Buffer
	code := run([]string{"-root", root, "-execute", "-json"}, &stdout1, &stderr1)
	if code != 0 {
		t.Fatalf("first run exit = %d, stderr=%s stdout=%s", code, stderr1.String(), stdout1.String())
	}
	var summary1 migrationSummary
	if err := json.Unmarshal(stdout1.Bytes(), &summary1); err != nil {
		t.Fatalf("first summary JSON: %v\n%s", err, stdout1.String())
	}
	if summary1.Migrated != 1 {
		t.Fatalf("first run migrated = %d, want 1", summary1.Migrated)
	}

	bytesAfterFirst, err := os.ReadFile(playerPath)
	if err != nil {
		t.Fatalf("read after first: %v", err)
	}

	// Second -execute: should be a no-op.
	var stdout2, stderr2 bytes.Buffer
	code = run([]string{"-root", root, "-execute", "-json"}, &stdout2, &stderr2)
	if code != 0 {
		t.Fatalf("second run exit = %d, stderr=%s stdout=%s", code, stderr2.String(), stdout2.String())
	}
	var summary2 migrationSummary
	if err := json.Unmarshal(stdout2.Bytes(), &summary2); err != nil {
		t.Fatalf("second summary JSON: %v\n%s", err, stdout2.String())
	}
	if summary2.Migrated != 0 {
		t.Fatalf("second run migrated = %d, want 0 (idempotent)", summary2.Migrated)
	}

	bytesAfterSecond, err := os.ReadFile(playerPath)
	if err != nil {
		t.Fatalf("read after second: %v", err)
	}

	if !bytes.Equal(bytesAfterFirst, bytesAfterSecond) {
		t.Fatal("file bytes changed on second -execute; expected idempotent no-op")
	}
}
