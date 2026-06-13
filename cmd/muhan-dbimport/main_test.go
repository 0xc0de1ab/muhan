package main

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xc0de1ab/muhan/internal/migrate/boardmap"
	"github.com/0xc0de1ab/muhan/internal/migrate/dbimport"
	"github.com/0xc0de1ab/muhan/internal/migrate/dbschema"
	"github.com/0xc0de1ab/muhan/internal/migrate/protoaudit"
	"github.com/0xc0de1ab/muhan/internal/persist/cbin"
	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

func TestDryRunBuildsPlanWithoutDB(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	openCalled := false

	code := run([]string{"-root", root, "-run-id", "run:test", "-json"}, &stdout, &stderr, fixedNow, func(string) (*sql.DB, error) {
		openCalled = true
		return nil, nil
	})
	if code != 0 {
		t.Fatalf("exit code = %d stderr=%s", code, stderr.String())
	}
	if openCalled {
		t.Fatal("dry-run opened DB")
	}

	var got planSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode summary: %v\n%s", err, stdout.String())
	}
	if got.RunID != "run:test" || !got.DryRun || got.Executed {
		t.Fatalf("summary state = %+v", got)
	}
	if got.TargetSchema != dbschema.DefaultSchema {
		t.Fatalf("target schema = %q, want %q", got.TargetSchema, dbschema.DefaultSchema)
	}
	if got.Batches != 18 {
		t.Fatalf("batches = %d, want 18", got.Batches)
	}
	if len(got.Tables) != 18 || got.Tables[0].Name != "import_runs" || got.Tables[len(got.Tables)-1].Name != "artifact_files" {
		t.Fatalf("tables = %+v", got.Tables)
	}
	if got.Rows != 4 || got.WorldWarnings != 3 || got.AuditCounts.FindingRecords != 3 {
		t.Fatalf("summary counts = %+v", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestDryRunIncludesBoardPostsWhenBoardDirExists(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	root := t.TempDir()
	writeBoardFixture(t, root)
	var stdout, stderr bytes.Buffer
	openCalled := false

	code := run([]string{"-root", root, "-run-id", "run:test", "-json"}, &stdout, &stderr, fixedNow, func(string) (*sql.DB, error) {
		openCalled = true
		return nil, nil
	})
	if code != 0 {
		t.Fatalf("exit code = %d stderr=%s", code, stderr.String())
	}
	if openCalled {
		t.Fatal("dry-run opened DB")
	}

	var got planSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode summary: %v\n%s", err, stdout.String())
	}
	if got.BoardPosts != 1 || got.BoardCounts.BoardDirs != 1 || got.BoardCounts.IndexRecords != 1 || got.BoardCounts.PostFiles != 1 {
		t.Fatalf("board summary = posts:%d counts:%+v", got.BoardPosts, got.BoardCounts)
	}
	if got.SidecarCounts.BoardPosts != 1 || got.SidecarCounts.EvidenceRecords != got.AuditCounts.EvidenceRecords ||
		got.SidecarCounts.FindingRecords != got.AuditCounts.FindingRecords || got.SidecarCounts.ArtifactFiles != got.AuditArtifacts {
		t.Fatalf("sidecar counts = %+v audit=%+v artifacts=%d", got.SidecarCounts, got.AuditCounts, got.AuditArtifacts)
	}
	if tableRows(got, "board_posts") != 1 {
		t.Fatalf("board_posts rows = %d, want 1", tableRows(got, "board_posts"))
	}
	if strings.Contains(stdout.String(), "게시글 전체 본문") {
		t.Fatalf("summary leaked board body: %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestDryRunFoldsBoardWarningsIntoSidecarFindings(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	root := t.TempDir()
	writeBoardFixtureWithBody(t, root, []byte{0xff})
	var stdout, stderr bytes.Buffer
	openCalled := false

	code := run([]string{"-root", root, "-run-id", "run:test", "-json"}, &stdout, &stderr, fixedNow, func(string) (*sql.DB, error) {
		openCalled = true
		return nil, nil
	})
	if code != 0 {
		t.Fatalf("exit code = %d stderr=%s", code, stderr.String())
	}
	if openCalled {
		t.Fatal("dry-run opened DB")
	}

	var got planSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode summary: %v\n%s", err, stdout.String())
	}
	if got.BoardCounts.Warnings != 1 {
		t.Fatalf("board warnings = %d, want 1", got.BoardCounts.Warnings)
	}
	if got.SidecarCounts.FindingRecords != got.AuditCounts.FindingRecords+1 {
		t.Fatalf("sidecar findings = %d, want audit findings %d + board warning",
			got.SidecarCounts.FindingRecords, got.AuditCounts.FindingRecords)
	}
	if tableRows(got, "worldload_findings") != got.SidecarCounts.FindingRecords {
		t.Fatalf("worldload_findings rows = %d, sidecar findings = %d",
			tableRows(got, "worldload_findings"), got.SidecarCounts.FindingRecords)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestExecuteRequiresDSNBeforeWorldLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	var stdout, stderr bytes.Buffer
	openCalled := false

	code := run([]string{"-root", "/does/not/matter", "-run-id", "run:test", "-execute"}, &stdout, &stderr, fixedNow, func(string) (*sql.DB, error) {
		openCalled = true
		return nil, nil
	})
	if code != 2 {
		t.Fatalf("exit code = %d stderr=%s", code, stderr.String())
	}
	if openCalled {
		t.Fatal("opened DB despite missing DSN")
	}
	if !strings.Contains(stderr.String(), "DSN is required") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestBadRootFailsBeforeDBOpen(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	var stdout, stderr bytes.Buffer
	openCalled := false

	code := run([]string{"-root", filepath.Join(t.TempDir(), "missing"), "-run-id", "run:test", "-json"}, &stdout, &stderr, fixedNow, func(string) (*sql.DB, error) {
		openCalled = true
		return nil, nil
	})
	if code != 2 {
		t.Fatalf("exit code = %d stderr=%s", code, stderr.String())
	}
	if openCalled {
		t.Fatal("opened DB for bad root")
	}
	if !strings.Contains(stderr.String(), "stat root") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestMissingRunIDFailsBeforeDBOpen(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	var stdout, stderr bytes.Buffer
	openCalled := false

	code := run([]string{"-root", t.TempDir(), "-run-id", "   "}, &stdout, &stderr, fixedNow, func(string) (*sql.DB, error) {
		openCalled = true
		return nil, nil
	})
	if code != 2 {
		t.Fatalf("exit code = %d stderr=%s", code, stderr.String())
	}
	if openCalled {
		t.Fatal("opened DB for missing run id")
	}
	if !strings.Contains(stderr.String(), "run id is required") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestUnsupportedDialectFailsBeforeWorldLoadAndDBOpen(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	var stdout, stderr bytes.Buffer
	openCalled := false

	code := run([]string{"-root", "/does/not/matter", "-run-id", "run:test", "-dialect", "sqlite"}, &stdout, &stderr, fixedNow, func(string) (*sql.DB, error) {
		openCalled = true
		return nil, nil
	})
	if code != 2 {
		t.Fatalf("exit code = %d stderr=%s", code, stderr.String())
	}
	if openCalled {
		t.Fatal("opened DB for unsupported dialect")
	}
	if !strings.Contains(stderr.String(), `unsupported dialect "sqlite"`) {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestUnsupportedSchemaModeFailsBeforeWorldLoadAndDBOpen(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	var stdout, stderr bytes.Buffer
	openCalled := false

	code := run([]string{"-root", "/does/not/matter", "-run-id", "run:test", "-schema-mode", "bogus"}, &stdout, &stderr, fixedNow, func(string) (*sql.DB, error) {
		openCalled = true
		return nil, nil
	})
	if code != 2 {
		t.Fatalf("exit code = %d stderr=%s", code, stderr.String())
	}
	if openCalled {
		t.Fatal("opened DB for unsupported schema mode")
	}
	if !strings.Contains(stderr.String(), `unsupported schema mode "bogus"`) {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestInvalidTargetSchemaFailsBeforeWorldLoadAndDBOpen(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	var stdout, stderr bytes.Buffer
	openCalled := false

	code := run([]string{"-root", "/does/not/matter", "-run-id", "run:test", "-target-schema", "bad-schema"}, &stdout, &stderr, fixedNow, func(string) (*sql.DB, error) {
		openCalled = true
		return nil, nil
	})
	if code != 2 {
		t.Fatalf("exit code = %d stderr=%s", code, stderr.String())
	}
	if openCalled {
		t.Fatal("opened DB for unsupported target schema")
	}
	if !strings.Contains(stderr.String(), `target schema "bad-schema"`) {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestExecuteRejectsDSNSearchPathBeforeWorldLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	var stdout, stderr bytes.Buffer
	openCalled := false

	code := run([]string{"-root", "/does/not/matter", "-run-id", "run:test", "-execute", "-dsn", "postgres://example/db?options=-csearch_path%3Dpublic"}, &stdout, &stderr, fixedNow, func(string) (*sql.DB, error) {
		openCalled = true
		return nil, nil
	})
	if code != 2 {
		t.Fatalf("exit code = %d stderr=%s", code, stderr.String())
	}
	if openCalled {
		t.Fatal("opened DB for DSN with search_path")
	}
	if !strings.Contains(stderr.String(), "DSN must not set search_path or options") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestReadDSNFileAndMutualExclusion(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	path := filepath.Join(t.TempDir(), "dsn.txt")
	if err := os.WriteFile(path, []byte("postgres://example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := readDSN("", path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "postgres://example" {
		t.Fatalf("dsn = %q", got)
	}
	if _, err := readDSN("postgres://inline", path); err == nil {
		t.Fatal("readDSN accepted both inline and file DSNs")
	}
}

func TestReadDSNUsesDatabaseURLFallback(t *testing.T) {
	t.Setenv("DATABASE_URL", " postgres://docker/example \n")

	got, err := readDSN("", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "postgres://docker/example" {
		t.Fatalf("dsn = %q", got)
	}
}

func TestReadDSNFlagsOverrideDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://docker/example")
	path := filepath.Join(t.TempDir(), "dsn.txt")
	if err := os.WriteFile(path, []byte("postgres://file/example\n"), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := readDSN(" postgres://inline/example ", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "postgres://inline/example" {
		t.Fatalf("inline dsn = %q", got)
	}

	got, err = readDSN("", path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "postgres://file/example" {
		t.Fatalf("file dsn = %q", got)
	}
}

func TestSplitSQLStatements(t *testing.T) {
	got := splitSQLStatements("CREATE TABLE a (id integer);\n\nCREATE INDEX b ON a (id);\n")
	want := []string{"CREATE TABLE a (id integer)", "CREATE INDEX b ON a (id)"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("statements = %#v, want %#v", got, want)
	}
}

func TestSplitSQLStatementsIgnoresCommentsAndQuotedSemicolons(t *testing.T) {
	got := splitSQLStatements("-- generated DDL; not a statement\nCREATE TABLE a (v text DEFAULT ';');\n-- another; comment\nCREATE INDEX b ON a (v);")
	want := []string{"CREATE TABLE a (v text DEFAULT ';')", "CREATE INDEX b ON a (v)"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("statements = %#v, want %#v", got, want)
	}
}

func TestFormatCatalogIssuesLimitsOutput(t *testing.T) {
	issues := []dbschema.CatalogIssue{
		{Message: "missing table rooms"},
		{Message: "missing column rooms.metadata"},
		{Message: "missing table players"},
	}
	got := formatCatalogIssues(issues, 2)
	want := "missing table rooms; missing column rooms.metadata; ... 1 more"
	if got != want {
		t.Fatalf("issues = %q, want %q", got, want)
	}
}

func TestVerifyImportedRowsWithCounterMatchesExpectedRows(t *testing.T) {
	batches := []dbimport.Batch{
		{Table: "import_runs", Rows: [][]any{{"run:test"}}},
		{Table: "rooms"},
		{Table: "players", Rows: [][]any{{"run:test", "player:1"}, {"run:test", "player:2"}}},
	}
	counts := map[string]int{
		"import_runs": 1,
		"rooms":       0,
		"players":     2,
	}

	verification, err := verifyImportedRowsWithCounter(batches, func(table string) (int, error) {
		return counts[table], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if verification.Rows != 3 || len(verification.Tables) != 3 {
		t.Fatalf("verification = %+v", verification)
	}
	if verification.Tables[1].Name != "rooms" || verification.Tables[1].Planned != 0 || verification.Tables[1].Actual != 0 {
		t.Fatalf("zero-row table verification = %+v", verification.Tables[1])
	}
}

func TestVerifyImportedRowsWithCounterRejectsMismatches(t *testing.T) {
	batches := []dbimport.Batch{{
		Table: "rooms",
		Rows:  [][]any{{"run:test", "room:1"}},
	}}

	verification, err := verifyImportedRowsWithCounter(batches, func(string) (int, error) {
		return 0, nil
	})
	if err == nil {
		t.Fatal("verifyImportedRowsWithCounter accepted a row count mismatch")
	}
	if !strings.Contains(err.Error(), "rooms planned=1 actual=0") {
		t.Fatalf("error = %v", err)
	}
	if verification.Tables[0].Actual != 0 || verification.Tables[0].Planned != 1 {
		t.Fatalf("verification = %+v", verification)
	}
}

func TestRowCountSQLUsesSchemaTableAllowlist(t *testing.T) {
	got, err := rowCountSQL("rooms", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "SELECT count(*) FROM rooms WHERE run_id = $1" {
		t.Fatalf("query = %q", got)
	}
	got, err = rowCountSQL("rooms", "muhan_import")
	if err != nil {
		t.Fatal(err)
	}
	if got != `SELECT count(*) FROM "muhan_import"."rooms" WHERE run_id = $1` {
		t.Fatalf("schema-qualified query = %q", got)
	}
	if _, err := rowCountSQL("rooms;drop", "muhan_import"); err == nil {
		t.Fatal("rowCountSQL accepted unsafe table")
	}
	if _, err := rowCountSQL("rooms", "bad-schema"); err == nil {
		t.Fatal("rowCountSQL accepted unsafe schema")
	}
}

func TestPlannedRowCountsRejectsUnexpectedAndDuplicateTables(t *testing.T) {
	if _, err := plannedRowCounts([]dbimport.Batch{{Table: "unexpected"}}); err == nil {
		t.Fatal("plannedRowCounts accepted unexpected table")
	}
	if _, err := plannedRowCounts([]dbimport.Batch{{Table: "rooms"}, {Table: "rooms"}}); err == nil {
		t.Fatal("plannedRowCounts accepted duplicate table")
	}
}

func TestImportManifestRecordsSidecarCountsAndLogicalRoot(t *testing.T) {
	auditManifest := protoauditManifestForTest("/absolute/secret")
	dbAuditManifest := auditManifest
	dbAuditManifest.Root = "logical-root"
	boardReport := boardmapReportForTest()
	sidecar := dbimportSidecarForTest()

	manifest := buildImportManifest("logical-root", fixedNow(), dbAuditManifest, boardReport, sidecar)
	if manifest.SchemaVersion != importManifestVersion || manifest.SourceRoot != "logical-root" || manifest.Audit.Root != "logical-root" {
		t.Fatalf("manifest root/header = %+v", manifest)
	}
	if manifest.Board.ImportedPosts != 1 || manifest.Board.Counts.Warnings != 1 || len(manifest.Board.Warnings) != 1 {
		t.Fatalf("board manifest = %+v", manifest.Board)
	}
	if manifest.SidecarCounts.BoardPosts != 1 || manifest.SidecarCounts.EvidenceRecords != 2 ||
		manifest.SidecarCounts.FindingRecords != 3 || manifest.SidecarCounts.ArtifactFiles != 1 {
		t.Fatalf("sidecar counts = %+v", manifest.SidecarCounts)
	}

	rows := artifactRows(dbAuditManifest)
	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "/absolute/secret") {
		t.Fatalf("artifact metadata leaked absolute root: %s", data)
	}
}

func tableRows(summary planSummary, table string) int {
	for _, item := range summary.Tables {
		if item.Name == table {
			return item.Rows
		}
	}
	return -1
}

func writeBoardFixture(t *testing.T, root string) {
	t.Helper()

	body, err := legacykr.EncodeEUCKR("게시글 전체 본문")
	if err != nil {
		t.Fatal(err)
	}
	writeBoardFixtureWithBody(t, root, body)
}

func writeBoardFixtureWithBody(t *testing.T, root string, body []byte) {
	t.Helper()

	boardDir := filepath.Join(root, "board", "info")
	if err := os.MkdirAll(boardDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(boardDir, "board_index"), makeBoardIndexRecord(t, 1, "운영자", "공지", 126, 5, 20, 1, 2, 3, 9), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(boardDir, "board.1"), body, 0600); err != nil {
		t.Fatal(err)
	}
}

func protoauditManifestForTest(root string) protoaudit.Manifest {
	return protoaudit.Manifest{
		SchemaVersion:   protoaudit.SchemaVersion,
		ResolverVersion: protoaudit.ResolverVersion,
		Root:            root,
		GeneratedAt:     fixedNow(),
		Counts: protoaudit.Counts{
			EvidenceRecords: 2,
			FindingRecords:  2,
		},
		Files: []protoaudit.ArtifactFile{{
			Path:    "worldload_findings.jsonl",
			Format:  "jsonl",
			Records: 2,
			Bytes:   123,
			SHA256:  strings.Repeat("a", 64),
		}},
	}
}

func boardmapReportForTest() boardmap.Report {
	return boardmap.Report{
		Counts: boardmap.Counts{
			BoardDirs:    1,
			IndexFiles:   1,
			IndexRecords: 1,
			PostFiles:    1,
			Warnings:     1,
		},
		Warnings: []boardmap.Finding{{
			Path:    "board/info/board.1",
			Message: "decode post body",
		}},
	}
}

func dbimportSidecarForTest() dbimport.Sidecar {
	return dbimport.Sidecar{
		BoardPosts: []model.BoardPost{{ID: "post:board:info:000001", BoardID: "board:info"}},
		Evidence: []protoaudit.EvidenceRecord{
			{EvidenceID: "evidence:1"},
			{EvidenceID: "evidence:2"},
		},
		Findings: []protoaudit.FindingRecord{
			{Severity: "warning", Kind: "worldload", Message: "one"},
			{Severity: "warning", Kind: "worldload", Message: "two"},
			{Severity: "warning", Kind: "boardmap", Message: "three"},
		},
		Artifacts: []dbimport.ArtifactFile{{Path: "worldload_findings.jsonl", Format: "jsonl"}},
	}
}

func makeBoardIndexRecord(t *testing.T, number int, uploader, title string, year, month, day, hour, minute, second, readCount int) []byte {
	t.Helper()

	data := make([]byte, cbin.BoardIndexSize)
	binary.LittleEndian.PutUint32(data[0:], uint32(int32(number)))
	copyLegacyText(t, data[4:20], uploader)
	binary.LittleEndian.PutUint32(data[20:], uint32(int32(year)))
	binary.LittleEndian.PutUint32(data[24:], uint32(int32(month)))
	binary.LittleEndian.PutUint32(data[28:], uint32(int32(day)))
	binary.LittleEndian.PutUint32(data[32:], uint32(int32(hour)))
	binary.LittleEndian.PutUint32(data[36:], uint32(int32(minute)))
	binary.LittleEndian.PutUint32(data[40:], uint32(int32(second)))
	binary.LittleEndian.PutUint32(data[44:], 1)
	binary.LittleEndian.PutUint32(data[48:], uint32(int32(readCount)))
	copyLegacyText(t, data[52:92], title)
	return data
}

func copyLegacyText(t *testing.T, dst []byte, text string) {
	t.Helper()

	encoded, err := legacykr.EncodeEUCKR(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > len(dst) {
		t.Fatalf("encoded text %q is %d bytes, max %d", text, len(encoded), len(dst))
	}
	copy(dst, encoded)
}

func fixedNow() time.Time {
	return time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC)
}
