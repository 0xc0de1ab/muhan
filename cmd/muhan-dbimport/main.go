package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"muhan/internal/migrate/boardmap"
	"muhan/internal/migrate/dbimport"
	"muhan/internal/migrate/dbschema"
	"muhan/internal/migrate/protoaudit"
	worldload "muhan/internal/world/load"
)

const importManifestVersion = "muhan-dbimport/v1"

const (
	schemaModeVerify = "verify"
	schemaModeEnsure = "ensure"
	schemaModeSkip   = "skip"
)

type planSummary struct {
	Root             string                `json:"root"`
	RunID            string                `json:"runId"`
	Dialect          string                `json:"dialect"`
	TargetSchema     string                `json:"targetSchema"`
	GeneratedAt      time.Time             `json:"generatedAt"`
	DryRun           bool                  `json:"dryRun"`
	Executed         bool                  `json:"executed"`
	ReplaceRun       bool                  `json:"replaceRun,omitempty"`
	Batches          int                   `json:"batches"`
	Rows             int                   `json:"rows"`
	RowsInserted     int                   `json:"rowsInserted,omitempty"`
	Verification     *rowCountVerification `json:"verification,omitempty"`
	WorldCounts      worldload.Counts      `json:"worldCounts"`
	AuditCounts      protoaudit.Counts     `json:"auditCounts"`
	WorldWarnings    int                   `json:"worldWarnings"`
	WorldErrors      int                   `json:"worldErrors"`
	BoardCounts      boardmap.Counts       `json:"boardCounts"`
	BoardPosts       int                   `json:"boardPosts"`
	SidecarCounts    sidecarCounts         `json:"sidecarCounts"`
	AuditArtifacts   int                   `json:"auditArtifacts"`
	Tables           []tablePlanSummary    `json:"tables"`
	ExecutionMessage string                `json:"executionMessage,omitempty"`
}

type tablePlanSummary struct {
	Name    string `json:"name"`
	Columns int    `json:"columns"`
	Rows    int    `json:"rows"`
}

type executionResult struct {
	Import       dbimport.Result
	Verification rowCountVerification
}

type rowCountVerification struct {
	Rows   int                  `json:"rows"`
	Tables []tableRowCountCheck `json:"tables,omitempty"`
}

type tableRowCountCheck struct {
	Name    string `json:"name"`
	Planned int    `json:"planned"`
	Actual  int    `json:"actual"`
}

type sidecarCounts struct {
	BoardPosts      int `json:"boardPosts"`
	EvidenceRecords int `json:"evidenceRecords"`
	FindingRecords  int `json:"findingRecords"`
	ArtifactFiles   int `json:"artifactFiles"`
}

type importRunManifest struct {
	SchemaVersion string              `json:"schemaVersion"`
	GeneratedAt   time.Time           `json:"generatedAt"`
	SourceRoot    string              `json:"sourceRoot"`
	Audit         protoaudit.Manifest `json:"audit"`
	Board         boardManifest       `json:"board"`
	SidecarCounts sidecarCounts       `json:"sidecarCounts"`
}

type boardManifest struct {
	Counts        boardmap.Counts    `json:"counts"`
	ImportedPosts int                `json:"importedPosts"`
	Warnings      []boardmap.Finding `json:"warnings,omitempty"`
	Errors        []boardmap.Finding `json:"errors,omitempty"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, time.Now, openPostgres))
}

func run(args []string, stdout, stderr io.Writer, now func() time.Time, open func(string) (*sql.DB, error)) int {
	fs := flag.NewFlagSet("muhan-dbimport", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "legacy Muhan data root")
	runID := fs.String("run-id", "", "stable import run id")
	dialect := fs.String("dialect", dbschema.DialectPostgres, "database dialect; currently postgresql")
	dsn := fs.String("dsn", "", "PostgreSQL DSN; required with -execute")
	dsnFile := fs.String("dsn-file", "", "file containing PostgreSQL DSN; used with -execute")
	execute := fs.Bool("execute", false, "execute the import against PostgreSQL")
	replaceRun := fs.Bool("replace-run", false, "delete an existing import_runs row for this run id inside the import transaction")
	sourceRootLabel := fs.String("source-root-label", "", "logical source_root value stored in import_runs instead of the absolute root path")
	auditOutdir := fs.String("audit-outdir", "", "optional directory for protoaudit JSONL artifacts")
	schemaMode := fs.String("schema-mode", schemaModeVerify, "schema preflight for -execute: verify, ensure, or skip")
	targetSchema := fs.String("target-schema", dbschema.DefaultSchema, "PostgreSQL schema that receives import tables")
	jsonOut := fs.Bool("json", false, "print JSON plan or execution summary")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	trimmedSchemaMode := strings.TrimSpace(*schemaMode)
	if err := validateSchemaMode(trimmedSchemaMode); err != nil {
		fmt.Fprintf(stderr, "dbimport: %v\n", err)
		return 2
	}
	if *dialect != dbschema.DialectPostgres {
		fmt.Fprintf(stderr, "dbimport: unsupported dialect %q\n", *dialect)
		return 2
	}
	trimmedTargetSchema := strings.TrimSpace(*targetSchema)
	if err := validateTargetSchema(trimmedTargetSchema); err != nil {
		fmt.Fprintf(stderr, "dbimport: %v\n", err)
		return 2
	}
	trimmedRunID := strings.TrimSpace(*runID)
	if trimmedRunID == "" {
		fmt.Fprintln(stderr, "dbimport: run id is required")
		return 2
	}
	dsnValue, err := readDSN(*dsn, *dsnFile)
	if err != nil {
		fmt.Fprintf(stderr, "dbimport: %v\n", err)
		return 2
	}
	if *execute && dsnValue == "" {
		fmt.Fprintln(stderr, "dbimport: DSN is required with -execute; use -dsn, -dsn-file, or DATABASE_URL")
		return 2
	}
	if *execute {
		if err := rejectDSNSchemaOptions(dsnValue); err != nil {
			fmt.Fprintf(stderr, "dbimport: %v\n", err)
			return 2
		}
	}

	generatedAt := now().UTC()
	worldSummary, err := worldload.LoadRoot(*root)
	if err != nil {
		fmt.Fprintf(stderr, "dbimport: %v\n", err)
		return 2
	}
	auditSnapshot, err := protoaudit.BuildFromSummary(worldSummary.Root, worldSummary, generatedAt)
	if err != nil {
		fmt.Fprintf(stderr, "dbimport: %v\n", err)
		return 2
	}
	auditManifest := protoaudit.Manifest{
		SchemaVersion:   protoaudit.SchemaVersion,
		ResolverVersion: protoaudit.ResolverVersion,
		Root:            auditSnapshot.Root,
		GeneratedAt:     auditSnapshot.GeneratedAt,
		Counts:          auditSnapshot.Counts,
		WorldCounts:     auditSnapshot.WorldCounts,
		Warnings:        auditSnapshot.Warnings,
		Errors:          auditSnapshot.Errors,
	}
	if *auditOutdir != "" {
		auditManifest, err = protoaudit.Write(*auditOutdir, auditSnapshot)
		if err != nil {
			fmt.Fprintf(stderr, "dbimport: %v\n", err)
			return 2
		}
	}
	boardReport, err := loadBoardReport(worldSummary.Root)
	if err != nil {
		fmt.Fprintf(stderr, "dbimport: %v\n", err)
		return 2
	}
	boardPosts := boardmap.BoardPosts(boardReport)
	findings := append([]protoaudit.FindingRecord(nil), auditSnapshot.Findings...)
	findings = append(findings, boardFindingRows(boardReport)...)
	sourceRoot := worldSummary.Root
	if strings.TrimSpace(*sourceRootLabel) != "" {
		sourceRoot = strings.TrimSpace(*sourceRootLabel)
	}
	dbAuditManifest := auditManifest
	dbAuditManifest.Root = sourceRoot

	sidecar := dbimport.Sidecar{
		BoardPosts: boardPosts,
		Evidence:   auditSnapshot.Evidence,
		Findings:   findings,
		Artifacts:  artifactRows(dbAuditManifest),
	}
	importManifest := buildImportManifest(sourceRoot, generatedAt, dbAuditManifest, boardReport, sidecar)
	batches, err := dbimport.BuildBatches(worldSummary.World, sidecar, dbimport.Options{
		RunID:       trimmedRunID,
		SourceRoot:  sourceRoot,
		GeneratedAt: generatedAt,
		Manifest:    importManifest,
	})
	if err != nil {
		fmt.Fprintf(stderr, "dbimport: %v\n", err)
		return 2
	}

	plan := summarizePlan(worldSummary, dbAuditManifest, boardReport, sidecar, trimmedRunID, *dialect, trimmedTargetSchema, generatedAt, !*execute, *replaceRun, batches)
	if len(worldSummary.Errors) > 0 || len(boardReport.Errors) > 0 {
		_ = writeSummary(stdout, plan, *jsonOut)
		return 1
	}

	if *execute {
		result, err := executeImport(context.Background(), dsnValue, trimmedRunID, *replaceRun, trimmedSchemaMode, trimmedTargetSchema, batches, open)
		if err != nil {
			fmt.Fprintf(stderr, "dbimport: %v\n", err)
			return 1
		}
		plan.DryRun = false
		plan.Executed = true
		plan.RowsInserted = result.Import.Rows
		plan.Verification = &result.Verification
		plan.ExecutionMessage = "committed"
	}

	if err := writeSummary(stdout, plan, *jsonOut); err != nil {
		fmt.Fprintf(stderr, "dbimport: %v\n", err)
		return 2
	}
	return 0
}

func executeImport(ctx context.Context, dsn, runID string, replaceRun bool, schemaMode, targetSchema string, batches []dbimport.Batch, open func(string) (*sql.DB, error)) (executionResult, error) {
	db, err := open(dsn)
	if err != nil {
		return executionResult{}, err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return executionResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := hardenSearchPath(ctx, tx); err != nil {
		return executionResult{}, err
	}
	if err := prepareSchema(ctx, tx, schemaMode, targetSchema); err != nil {
		return executionResult{}, err
	}
	if replaceRun {
		importRuns, err := qualifiedTable(targetSchema, "import_runs")
		if err != nil {
			return executionResult{}, err
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE run_id = $1", importRuns), runID); err != nil {
			return executionResult{}, fmt.Errorf("replace run %q: %w", runID, err)
		}
	}
	importResult, err := dbimport.ImportBatchesWithOptions(ctx, tx, batches, dbimport.ImportOptions{Schema: targetSchema})
	if err != nil {
		return executionResult{Import: importResult}, err
	}
	verification, err := verifyImportedRows(ctx, tx, runID, targetSchema, batches)
	if err != nil {
		return executionResult{Import: importResult, Verification: verification}, err
	}
	if err := storeImportVerification(ctx, tx, runID, targetSchema, verification); err != nil {
		return executionResult{Import: importResult, Verification: verification}, err
	}
	if err := tx.Commit(); err != nil {
		return executionResult{Import: importResult, Verification: verification}, err
	}
	committed = true
	return executionResult{Import: importResult, Verification: verification}, nil
}

func validateSchemaMode(mode string) error {
	switch mode {
	case schemaModeVerify, schemaModeEnsure, schemaModeSkip:
		return nil
	default:
		return fmt.Errorf("unsupported schema mode %q", mode)
	}
}

func validateTargetSchema(schema string) error {
	if schema == "" {
		return fmt.Errorf("target schema is required")
	}
	if err := dbschema.ValidateIdentifier(schema); err != nil {
		return fmt.Errorf("target schema %q: %w", schema, err)
	}
	if schema == "pg_catalog" || schema == "information_schema" || strings.HasPrefix(schema, "pg_") {
		return fmt.Errorf("target schema %q is reserved", schema)
	}
	return nil
}

func rejectDSNSchemaOptions(dsn string) error {
	lower := strings.ToLower(dsn)
	if strings.Contains(lower, "search_path") || strings.Contains(lower, "options=") {
		return fmt.Errorf("DSN must not set search_path or options; use -target-schema")
	}
	return nil
}

func hardenSearchPath(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "SET LOCAL search_path = pg_catalog"); err != nil {
		return fmt.Errorf("set import search_path: %w", err)
	}
	return nil
}

func prepareSchema(ctx context.Context, tx *sql.Tx, mode, targetSchema string) error {
	if mode == schemaModeSkip {
		return nil
	}
	manifest, err := dbschema.Build(dbschema.Options{GeneratedAt: time.Now().UTC()})
	if err != nil {
		return fmt.Errorf("build schema manifest: %w", err)
	}
	if mode == schemaModeEnsure {
		ddl, err := dbschema.PostgresDDLForSchema(manifest, targetSchema)
		if err != nil {
			return fmt.Errorf("render schema DDL: %w", err)
		}
		for i, statement := range splitSQLStatements(ddl) {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("ensure schema statement %d: %w", i+1, err)
			}
		}
	}
	catalog, err := dbschema.LoadPostgresCatalogForSchema(ctx, tx, targetSchema)
	if err != nil {
		return fmt.Errorf("load PostgreSQL schema catalog: %w", err)
	}
	if issues := dbschema.ValidateCatalog(manifest, catalog); len(issues) > 0 {
		return fmt.Errorf("schema preflight failed: %s", formatCatalogIssues(issues, 8))
	}
	return nil
}

func splitSQLStatements(sqlText string) []string {
	statements := make([]string, 0)
	var b strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	inLineComment := false
	for i := 0; i < len(sqlText); i++ {
		ch := sqlText[i]
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
				b.WriteByte(ch)
			}
			continue
		}
		if !inSingleQuote && !inDoubleQuote && ch == '-' && i+1 < len(sqlText) && sqlText[i+1] == '-' {
			inLineComment = true
			i++
			continue
		}
		switch ch {
		case '\'':
			b.WriteByte(ch)
			if !inDoubleQuote {
				if inSingleQuote && i+1 < len(sqlText) && sqlText[i+1] == '\'' {
					i++
					b.WriteByte(sqlText[i])
					continue
				}
				inSingleQuote = !inSingleQuote
			}
		case '"':
			b.WriteByte(ch)
			if !inSingleQuote {
				if inDoubleQuote && i+1 < len(sqlText) && sqlText[i+1] == '"' {
					i++
					b.WriteByte(sqlText[i])
					continue
				}
				inDoubleQuote = !inDoubleQuote
			}
		case ';':
			if inSingleQuote || inDoubleQuote {
				b.WriteByte(ch)
				continue
			}
			statement := strings.TrimSpace(b.String())
			if statement != "" {
				statements = append(statements, statement)
			}
			b.Reset()
		default:
			b.WriteByte(ch)
		}
	}
	statement := strings.TrimSpace(b.String())
	if statement != "" {
		statements = append(statements, statement)
	}
	return statements
}

func formatCatalogIssues(issues []dbschema.CatalogIssue, limit int) string {
	if limit <= 0 || limit > len(issues) {
		limit = len(issues)
	}
	parts := make([]string, 0, limit+1)
	for _, issue := range issues[:limit] {
		parts = append(parts, issue.Message)
	}
	if limit < len(issues) {
		parts = append(parts, fmt.Sprintf("... %d more", len(issues)-limit))
	}
	return strings.Join(parts, "; ")
}

func verifyImportedRows(ctx context.Context, tx *sql.Tx, runID, targetSchema string, batches []dbimport.Batch) (rowCountVerification, error) {
	return verifyImportedRowsWithCounter(batches, func(table string) (int, error) {
		query, err := rowCountSQL(table, targetSchema)
		if err != nil {
			return 0, err
		}
		var count int
		if err := tx.QueryRowContext(ctx, query, runID).Scan(&count); err != nil {
			return 0, fmt.Errorf("count %s: %w", table, err)
		}
		return count, nil
	})
}

func verifyImportedRowsWithCounter(batches []dbimport.Batch, countRows func(table string) (int, error)) (rowCountVerification, error) {
	if countRows == nil {
		return rowCountVerification{}, fmt.Errorf("row counter is required")
	}
	checks, err := plannedRowCounts(batches)
	if err != nil {
		return rowCountVerification{}, err
	}
	verification := rowCountVerification{Tables: checks}
	mismatches := make([]tableRowCountCheck, 0)
	for i := range verification.Tables {
		actual, err := countRows(verification.Tables[i].Name)
		if err != nil {
			return verification, err
		}
		verification.Tables[i].Actual = actual
		verification.Rows += actual
		if verification.Tables[i].Actual != verification.Tables[i].Planned {
			mismatches = append(mismatches, verification.Tables[i])
		}
	}
	if len(mismatches) > 0 {
		return verification, fmt.Errorf("post-import row count mismatch: %s", formatRowCountMismatches(mismatches, 8))
	}
	return verification, nil
}

func plannedRowCounts(batches []dbimport.Batch) ([]tableRowCountCheck, error) {
	allowed, err := schemaTableSet()
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	checks := make([]tableRowCountCheck, 0, len(batches))
	for _, batch := range batches {
		if _, ok := allowed[batch.Table]; !ok {
			return nil, fmt.Errorf("unexpected import table %q", batch.Table)
		}
		if _, ok := seen[batch.Table]; ok {
			return nil, fmt.Errorf("duplicate import table %q", batch.Table)
		}
		seen[batch.Table] = struct{}{}
		checks = append(checks, tableRowCountCheck{
			Name:    batch.Table,
			Planned: len(batch.Rows),
		})
	}
	return checks, nil
}

func rowCountSQL(table, targetSchema string) (string, error) {
	allowed, err := schemaTableSet()
	if err != nil {
		return "", err
	}
	if _, ok := allowed[table]; !ok {
		return "", fmt.Errorf("unexpected import table %q", table)
	}
	tableName, err := qualifiedTable(targetSchema, table)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("SELECT count(*) FROM %s WHERE run_id = $1", tableName), nil
}

func qualifiedTable(targetSchema, table string) (string, error) {
	return dbschema.QualifiedName(strings.TrimSpace(targetSchema), table)
}

func schemaTableSet() (map[string]struct{}, error) {
	manifest, err := dbschema.Build(dbschema.Options{})
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(manifest.Tables))
	for _, table := range manifest.Tables {
		out[table.Name] = struct{}{}
	}
	return out, nil
}

func formatRowCountMismatches(mismatches []tableRowCountCheck, limit int) string {
	if limit <= 0 || limit > len(mismatches) {
		limit = len(mismatches)
	}
	parts := make([]string, 0, limit+1)
	for _, mismatch := range mismatches[:limit] {
		parts = append(parts, fmt.Sprintf("%s planned=%d actual=%d", mismatch.Name, mismatch.Planned, mismatch.Actual))
	}
	if limit < len(mismatches) {
		parts = append(parts, fmt.Sprintf("... %d more", len(mismatches)-limit))
	}
	return strings.Join(parts, "; ")
}

func storeImportVerification(ctx context.Context, tx *sql.Tx, runID, targetSchema string, verification rowCountVerification) error {
	data, err := json.Marshal(verification)
	if err != nil {
		return fmt.Errorf("marshal import verification: %w", err)
	}
	importRuns, err := qualifiedTable(targetSchema, "import_runs")
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET manifest = jsonb_set(manifest, '{verification}', $2::jsonb, true) WHERE run_id = $1`, importRuns), runID, string(data))
	if err != nil {
		return fmt.Errorf("store import verification: %w", err)
	}
	if rows, err := result.RowsAffected(); err == nil && rows != 1 {
		return fmt.Errorf("store import verification affected %d rows, want 1", rows)
	}
	return nil
}

func openPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func readDSN(inline, path string) (string, error) {
	inline = strings.TrimSpace(inline)
	path = strings.TrimSpace(path)
	if inline != "" && path != "" {
		return "", fmt.Errorf("-dsn and -dsn-file are mutually exclusive")
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read DSN file: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	if inline != "" {
		return inline, nil
	}
	return strings.TrimSpace(os.Getenv("DATABASE_URL")), nil
}

func loadBoardReport(root string) (boardmap.Report, error) {
	boardRoot := filepath.Join(root, "board")
	info, err := os.Stat(boardRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return boardmap.Report{}, nil
		}
		return boardmap.Report{}, fmt.Errorf("stat board root: %w", err)
	}
	if !info.IsDir() {
		return boardmap.Report{
			Root: root,
			Counts: boardmap.Counts{
				Warnings: 1,
			},
			Warnings: []boardmap.Finding{{
				Path:    "board",
				Message: "board path is not a directory",
			}},
		}, nil
	}
	return boardmap.ScanRoot(root)
}

func boardFindingRows(report boardmap.Report) []protoaudit.FindingRecord {
	rows := make([]protoaudit.FindingRecord, 0, len(report.Warnings)+len(report.Errors))
	appendFinding := func(severity string, finding boardmap.Finding) {
		rows = append(rows, protoaudit.FindingRecord{
			SchemaVersion: protoaudit.SchemaVersion,
			Severity:      severity,
			Kind:          "boardmap",
			Path:          finding.Path,
			Message:       finding.Message,
			Source: protoaudit.SourceEvidence{
				LegacyKind:     "board",
				LegacyPath:     finding.Path,
				LegacyEncoding: "euc-kr/cp949",
			},
		})
	}
	for _, finding := range report.Warnings {
		appendFinding("warning", finding)
	}
	for _, finding := range report.Errors {
		appendFinding("error", finding)
	}
	return rows
}

func buildImportManifest(sourceRoot string, generatedAt time.Time, auditManifest protoaudit.Manifest, boardReport boardmap.Report, sidecar dbimport.Sidecar) importRunManifest {
	return importRunManifest{
		SchemaVersion: importManifestVersion,
		GeneratedAt:   generatedAt.UTC(),
		SourceRoot:    sourceRoot,
		Audit:         auditManifest,
		Board: boardManifest{
			Counts:        boardReport.Counts,
			ImportedPosts: len(sidecar.BoardPosts),
			Warnings:      append([]boardmap.Finding(nil), boardReport.Warnings...),
			Errors:        append([]boardmap.Finding(nil), boardReport.Errors...),
		},
		SidecarCounts: countSidecar(sidecar),
	}
}

func countSidecar(sidecar dbimport.Sidecar) sidecarCounts {
	return sidecarCounts{
		BoardPosts:      len(sidecar.BoardPosts),
		EvidenceRecords: len(sidecar.Evidence),
		FindingRecords:  len(sidecar.Findings),
		ArtifactFiles:   len(sidecar.Artifacts),
	}
}

func summarizePlan(worldSummary worldload.Summary, auditManifest protoaudit.Manifest, boardReport boardmap.Report, sidecar dbimport.Sidecar, runID, dialect, targetSchema string, generatedAt time.Time, dryRun, replaceRun bool, batches []dbimport.Batch) planSummary {
	out := planSummary{
		Root:           worldSummary.Root,
		RunID:          runID,
		Dialect:        dialect,
		TargetSchema:   targetSchema,
		GeneratedAt:    generatedAt.UTC(),
		DryRun:         dryRun,
		ReplaceRun:     replaceRun,
		WorldCounts:    worldSummary.Counts,
		AuditCounts:    auditManifest.Counts,
		WorldWarnings:  len(worldSummary.Warnings),
		WorldErrors:    len(worldSummary.Errors),
		BoardCounts:    boardReport.Counts,
		BoardPosts:     len(sidecar.BoardPosts),
		SidecarCounts:  countSidecar(sidecar),
		AuditArtifacts: len(auditManifest.Files),
		Tables:         make([]tablePlanSummary, 0, len(batches)),
	}
	for _, batch := range batches {
		out.Batches++
		out.Rows += len(batch.Rows)
		out.Tables = append(out.Tables, tablePlanSummary{
			Name:    batch.Table,
			Columns: len(batch.Columns),
			Rows:    len(batch.Rows),
		})
	}
	return out
}

func artifactRows(manifest protoaudit.Manifest) []dbimport.ArtifactFile {
	rows := make([]dbimport.ArtifactFile, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		records := file.Records
		bytes := file.Bytes
		rows = append(rows, dbimport.ArtifactFile{
			Path:    file.Path,
			Format:  file.Format,
			Records: &records,
			Bytes:   &bytes,
			SHA256:  file.SHA256,
			Metadata: map[string]any{
				"source":                "protoaudit.index",
				"manifestSchemaVersion": manifest.SchemaVersion,
				"resolverVersion":       manifest.ResolverVersion,
				"generatedAt":           manifest.GeneratedAt,
				"root":                  manifest.Root,
				"raw":                   file,
			},
		})
	}
	return rows
}

func writeSummary(w io.Writer, summary planSummary, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	}
	fmt.Fprintf(w, "root: %s\n", summary.Root)
	fmt.Fprintf(w, "runId: %s\n", summary.RunID)
	fmt.Fprintf(w, "dialect: %s\n", summary.Dialect)
	fmt.Fprintf(w, "targetSchema: %s\n", summary.TargetSchema)
	fmt.Fprintf(w, "dryRun: %t\n", summary.DryRun)
	fmt.Fprintf(w, "executed: %t\n", summary.Executed)
	fmt.Fprintf(w, "batches: %d\n", summary.Batches)
	fmt.Fprintf(w, "rows: %d\n", summary.Rows)
	if summary.RowsInserted > 0 {
		fmt.Fprintf(w, "rowsInserted: %d\n", summary.RowsInserted)
	}
	if summary.Verification != nil {
		fmt.Fprintf(w, "rowsVerified: %d\n", summary.Verification.Rows)
	}
	fmt.Fprintf(w, "world: rooms=%d players=%d banks=%d creatures=%d objects=%d prototypes=%d warnings=%d errors=%d\n",
		summary.WorldCounts.Rooms,
		summary.WorldCounts.Players,
		summary.WorldCounts.BankAccounts,
		summary.WorldCounts.Creatures,
		summary.WorldCounts.ObjectInstances,
		summary.WorldCounts.ObjectPrototypes,
		summary.WorldWarnings,
		summary.WorldErrors,
	)
	fmt.Fprintf(w, "audit: evidence=%d findings=%d artifacts=%d\n",
		summary.AuditCounts.EvidenceRecords,
		summary.AuditCounts.FindingRecords,
		summary.AuditArtifacts,
	)
	fmt.Fprintf(w, "sidecar: boardPosts=%d evidence=%d findings=%d artifacts=%d\n",
		summary.SidecarCounts.BoardPosts,
		summary.SidecarCounts.EvidenceRecords,
		summary.SidecarCounts.FindingRecords,
		summary.SidecarCounts.ArtifactFiles,
	)
	fmt.Fprintf(w, "board: dirs=%d indexFiles=%d indexRecords=%d postFiles=%d importedPosts=%d warnings=%d errors=%d\n",
		summary.BoardCounts.BoardDirs,
		summary.BoardCounts.IndexFiles,
		summary.BoardCounts.IndexRecords,
		summary.BoardCounts.PostFiles,
		summary.BoardPosts,
		summary.BoardCounts.Warnings,
		summary.BoardCounts.Errors,
	)
	for _, table := range summary.Tables {
		fmt.Fprintf(w, "table: %s rows=%d columns=%d\n", table.Name, table.Rows, table.Columns)
	}
	if summary.ExecutionMessage != "" {
		fmt.Fprintf(w, "execution: %s\n", summary.ExecutionMessage)
	}
	return nil
}
