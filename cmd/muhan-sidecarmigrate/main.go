package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/state"
)

type migrationSummary struct {
	Root         string                         `json:"root"`
	DryRun       bool                           `json:"dryRun"`
	Executed     bool                           `json:"executed"`
	TotalScanned int                            `json:"totalScanned"`
	Migrated     int                            `json:"migrated"`
	ByType       map[string]int                 `json:"byType"`
	Errors       []string                       `json:"errors,omitempty"`
	Details      []state.SidecarMigrationDetail `json:"details,omitempty"`
	Message      string                         `json:"message"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("muhan-sidecarmigrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "Muhan data root containing sidecar JSON directories")
	execute := fs.Bool("execute", false, "rewrite supported old sidecar schema files in place")
	jsonOut := fs.Bool("json", false, "print JSON migration summary")
	details := fs.Bool("details", false, "print per-file rewrite details in text mode")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	summary, err := runMigration(*root, *execute)
	if err != nil {
		fmt.Fprintf(stderr, "sidecarmigrate: %v\n", err)
		return 2
	}

	if *jsonOut {
		if err := encodeSummary(stdout, summary); err != nil {
			fmt.Fprintf(stderr, "sidecarmigrate: write summary: %v\n", err)
			return 2
		}
	} else {
		renderText(stdout, summary, *details)
	}

	if len(summary.Errors) > 0 {
		return 1
	}
	return 0
}

func runMigration(root string, execute bool) (migrationSummary, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return migrationSummary{}, fmt.Errorf("resolve root: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return migrationSummary{}, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return migrationSummary{}, fmt.Errorf("root is not a directory: %s", absRoot)
	}

	migrationRoot := absRoot
	cleanup := func() {}
	if !execute {
		tmpRoot, err := os.MkdirTemp("", "muhan-sidecarmigrate-*")
		if err != nil {
			return migrationSummary{}, fmt.Errorf("create dry-run root: %w", err)
		}
		cleanup = func() {
			_ = os.RemoveAll(tmpRoot)
		}
		defer cleanup()
		if err := copySidecarCorpus(absRoot, tmpRoot); err != nil {
			return migrationSummary{}, err
		}
		migrationRoot = tmpRoot
	}

	report, err := state.MigrateSidecars(migrationRoot)
	if err != nil {
		return migrationSummary{}, err
	}
	if !execute {
		normalizeReportPaths(&report, migrationRoot, absRoot)
	}

	summary := migrationSummary{
		Root:         absRoot,
		DryRun:       !execute,
		Executed:     execute,
		TotalScanned: report.TotalScanned,
		Migrated:     report.Migrated,
		ByType:       report.ByType,
		Errors:       report.Errors,
		Details:      report.Details,
	}
	if execute {
		summary.Message = fmt.Sprintf("rewrite executed; %d sidecar file(s) rewritten", report.Migrated)
	} else {
		summary.Message = fmt.Sprintf("dry-run complete; no source files rewritten; %d sidecar file(s) would be rewritten", report.Migrated)
	}
	return summary, nil
}

func encodeSummary(w io.Writer, summary migrationSummary) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(summary)
}

func renderText(w io.Writer, summary migrationSummary, details bool) {
	mode := "DRY-RUN"
	if summary.Executed {
		mode = "EXECUTE"
	}
	fmt.Fprintf(w, "sidecarmigrate: root=%s mode=%s\n", summary.Root, mode)
	fmt.Fprintln(w, summary.Message)
	fmt.Fprintf(w, "scanned=%d migrated=%d errors=%d\n", summary.TotalScanned, summary.Migrated, len(summary.Errors))

	if len(summary.ByType) > 0 {
		keys := make([]string, 0, len(summary.ByType))
		for key := range summary.ByType {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		fmt.Fprintln(w, "by type:")
		for _, key := range keys {
			fmt.Fprintf(w, "  %s: %d\n", key, summary.ByType[key])
		}
	}

	if len(summary.Details) > 0 && !details {
		fmt.Fprintf(w, "details omitted; use -details or -json to list %d file(s)\n", len(summary.Details))
	}

	if len(summary.Details) > 0 && details {
		if summary.DryRun {
			fmt.Fprintln(w, "would rewrite:")
		} else {
			fmt.Fprintln(w, "rewritten:")
		}
		for _, detail := range summary.Details {
			fmt.Fprintf(w, "  %s %s: v%d -> v%d\n", detail.Type, detail.Path, detail.FromVer, detail.ToVer)
		}
	}

	if len(summary.Errors) > 0 {
		fmt.Fprintln(w, "errors:")
		for _, msg := range summary.Errors {
			fmt.Fprintf(w, "  %s\n", msg)
		}
	}
}

var sidecarDirs = []string{
	filepath.Join("player", "json"),
	filepath.Join("player", "bank", "json"),
	filepath.Join("room", "json"),
	filepath.Join("board", "json"),
	filepath.Join("player", "family", "json"),
}

func copySidecarCorpus(srcRoot, dstRoot string) error {
	for _, relDir := range sidecarDirs {
		srcDir := filepath.Join(srcRoot, relDir)
		entries, err := os.ReadDir(srcDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read sidecar dir %s: %w", srcDir, err)
		}
		dstDir := filepath.Join(dstRoot, relDir)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			srcPath := filepath.Join(srcDir, entry.Name())
			dstPath := filepath.Join(dstDir, entry.Name())
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(srcPath, dstPath string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read sidecar %s: %w", srcPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0700); err != nil {
		return fmt.Errorf("create sidecar dry-run dir %s: %w", filepath.Dir(dstPath), err)
	}
	if err := os.WriteFile(dstPath, data, 0600); err != nil {
		return fmt.Errorf("write sidecar dry-run copy %s: %w", dstPath, err)
	}
	return nil
}

func normalizeReportPaths(report *state.SidecarMigrationReport, fromRoot, toRoot string) {
	for i := range report.Details {
		report.Details[i].Path = rebasePath(report.Details[i].Path, fromRoot, toRoot)
	}
	for i := range report.Errors {
		report.Errors[i] = strings.ReplaceAll(report.Errors[i], fromRoot, toRoot)
	}
}

func rebasePath(path, fromRoot, toRoot string) string {
	rel, err := filepath.Rel(fromRoot, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.Join(toRoot, rel)
}
