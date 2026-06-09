package playermap

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"muhan/internal/migrate/protoresolve"
	"muhan/internal/persist/cbin"
)

type Report struct {
	Root         string    `json:"root"`
	Counts       Counts    `json:"counts"`
	InvalidNames []Finding `json:"invalidNames,omitempty"`
	Warnings     []Finding `json:"warnings,omitempty"`
	Errors       []Finding `json:"errors,omitempty"`
}

type Counts struct {
	PlayerFiles          int        `json:"playerFiles"`
	MappedPlayers        int        `json:"mappedPlayers"`
	UTF8Filenames        int        `json:"utf8Filenames"`
	LegacyKRFilenames    int        `json:"legacyKrFilenames"`
	InvalidFilenames     int        `json:"invalidFilenames"`
	InvalidNames         int        `json:"invalidNames"`
	RecordNameMismatches int        `json:"recordNameMismatches"`
	InventoryObjects     int        `json:"inventoryObjects"`
	PrototypeResolved    int        `json:"prototypeResolved"`
	PrototypeSynthetic   int        `json:"prototypeSynthetic"`
	PrototypeAmbiguous   int        `json:"prototypeAmbiguous"`
	SkippedFiles         int        `json:"skippedFiles"`
	Warnings             int        `json:"warnings"`
	Errors               int        `json:"errors"`
	Decoded              cbin.Stats `json:"decoded"`
}

var skippedPlayerDirs = map[string]struct{}{
	"alias":    {},
	"bank":     {},
	"fal":      {},
	"family":   {},
	"invite":   {},
	"marriage": {},
	"simul":    {},
	"temp":     {},
	"vote":     {},
}

func ScanRoot(root string) (Report, error) {
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Report{}, fmt.Errorf("resolve root: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return Report{}, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return Report{}, fmt.Errorf("root is not a directory: %s", absRoot)
	}

	report := Report{
		Root:         absRoot,
		InvalidNames: []Finding{},
		Warnings:     []Finding{},
		Errors:       []Finding{},
	}
	objectResolver, err := protoresolve.BuildObjectResolver(absRoot)
	if err != nil {
		report.Warnings = append(report.Warnings, Finding{Path: displayPath(filepath.Join(absRoot, "objmon")), Message: err.Error()})
	}
	scanPlayerRoot(absRoot, &report, objectResolver)
	report.Counts.Warnings = len(report.Warnings)
	report.Counts.Errors = len(report.Errors)
	return report, nil
}

func EncodeJSON(w io.Writer, report Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func WriteText(w io.Writer, report Report, maxFindings int) {
	fmt.Fprintf(w, "root: %s\n", report.Root)
	fmt.Fprintf(w, "players: %d files, %d mapped\n", report.Counts.PlayerFiles, report.Counts.MappedPlayers)
	fmt.Fprintf(w, "filenames: utf8=%d legacyKr=%d invalidEncoding=%d invalidPolicy=%d\n",
		report.Counts.UTF8Filenames,
		report.Counts.LegacyKRFilenames,
		report.Counts.InvalidFilenames,
		report.Counts.InvalidNames,
	)
	fmt.Fprintf(w, "record name mismatches: %d\n", report.Counts.RecordNameMismatches)
	fmt.Fprintf(w, "inventory objects: %d\n", report.Counts.InventoryObjects)
	fmt.Fprintf(w, "prototype resolution: resolved=%d synthetic=%d ambiguous=%d\n",
		report.Counts.PrototypeResolved,
		report.Counts.PrototypeSynthetic,
		report.Counts.PrototypeAmbiguous,
	)
	fmt.Fprintf(w, "skipped files: %d\n", report.Counts.SkippedFiles)
	fmt.Fprintf(w, "warnings: %d\n", len(report.Warnings))
	writeFindings(w, report.Warnings, maxFindings)
	fmt.Fprintf(w, "errors: %d\n", len(report.Errors))
	writeFindings(w, report.Errors, maxFindings)
}

func scanPlayerRoot(root string, report *Report, objectResolver protoresolve.ObjectPrototypeResolver) {
	playerRoot := filepath.Join(root, "player")
	entries, err := os.ReadDir(playerRoot)
	if err != nil {
		report.Errors = append(report.Errors, Finding{Path: displayPath(playerRoot), Message: err.Error()})
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			report.Counts.SkippedFiles++
			continue
		}

		name := entry.Name()
		if _, skip := skippedPlayerDirs[name]; skip {
			report.Counts.SkippedFiles += countRegularFiles(filepath.Join(playerRoot, name))
			continue
		}
		scanShard(filepath.Join(playerRoot, name), name, report, objectResolver)
	}
}

func scanShard(dir, shard string, report *Report, objectResolver protoresolve.ObjectPrototypeResolver) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		report.Errors = append(report.Errors, Finding{Path: displayPath(dir), Message: err.Error()})
		return
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			report.Counts.SkippedFiles += countRegularFiles(path)
			continue
		}

		report.Counts.PlayerFiles++
		data, err := os.ReadFile(path)
		if err != nil {
			report.Errors = append(report.Errors, Finding{Path: displayPath(path), Message: err.Error()})
			continue
		}

		result, err := MapPlayerFileWithOptions(path, shard, data, Options{IncludeObjects: true, PrototypeResolver: objectResolver})
		if err != nil {
			report.Errors = append(report.Errors, Finding{Path: displayPath(path), Message: err.Error()})
			continue
		}
		report.Counts.MappedPlayers++
		switch result.FilenameEncoding {
		case EncodingUTF8:
			report.Counts.UTF8Filenames++
		case EncodingLegacyKR:
			report.Counts.LegacyKRFilenames++
		case EncodingInvalid:
			report.Counts.InvalidFilenames++
		}
		if result.InvalidName {
			report.Counts.InvalidNames++
			report.InvalidNames = append(report.InvalidNames, Finding{
				Path:    result.Path,
				Message: fmt.Sprintf("invalid player filename %q", result.Filename),
			})
		}
		if result.RecordNameMismatch {
			report.Counts.RecordNameMismatches++
		}
		report.Counts.InventoryObjects += len(result.Objects)
		report.Counts.PrototypeResolved += result.PrototypeResolution.ResolvedExact
		report.Counts.PrototypeSynthetic += result.PrototypeResolution.Synthetic
		report.Counts.PrototypeAmbiguous += result.PrototypeResolution.AmbiguousSynthetic
		report.Counts.addStats(result.Decoded)
		report.Warnings = append(report.Warnings, result.Warnings...)
	}
}

func (c *Counts) addStats(st cbin.Stats) {
	c.Decoded.Objects += st.Objects
	c.Decoded.Creatures += st.Creatures
	c.Decoded.Rooms += st.Rooms
	c.Decoded.Exits += st.Exits
	c.Decoded.Descriptions += st.Descriptions
	c.Decoded.DescriptionBytes += st.DescriptionBytes
	c.Decoded.TrailingBytes += st.TrailingBytes
	if st.MaxDepth > c.Decoded.MaxDepth {
		c.Decoded.MaxDepth = st.MaxDepth
	}
}

func countRegularFiles(dir string) int {
	n := 0
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d != nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func writeFindings(w io.Writer, findings []Finding, maxFindings int) {
	if len(findings) == 0 {
		return
	}
	limit := maxFindings
	if limit <= 0 || limit > len(findings) {
		limit = len(findings)
	}
	for _, finding := range findings[:limit] {
		if finding.Path == "" {
			fmt.Fprintf(w, "  - %s\n", strings.TrimSpace(finding.Message))
			continue
		}
		fmt.Fprintf(w, "  - %s: %s\n", finding.Path, strings.TrimSpace(finding.Message))
	}
	if limit < len(findings) {
		fmt.Fprintf(w, "  ... %d more\n", len(findings)-limit)
	}
}
