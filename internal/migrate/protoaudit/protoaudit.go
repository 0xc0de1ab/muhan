package protoaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/0xc0de1ab/muhan/internal/migrate/protoresolve"
	"github.com/0xc0de1ab/muhan/internal/persist/cbin"
	"github.com/0xc0de1ab/muhan/internal/persist/jsonstore"
	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	SchemaVersion   = "prototype-resolution-audit/v1"
	ResolverVersion = "object-protoresolve/v1"

	evidenceFileName = "prototype_resolution_evidence.jsonl"
	findingsFileName = "worldload_findings.jsonl"
	indexFileName    = "index.json"
)

type Options struct {
	Root        string
	GeneratedAt time.Time
}

type Snapshot struct {
	Root        string           `json:"root"`
	GeneratedAt time.Time        `json:"generatedAt"`
	Counts      Counts           `json:"counts"`
	Evidence    []EvidenceRecord `json:"evidence,omitempty"`
	Findings    []FindingRecord  `json:"findings,omitempty"`
	Warnings    []Finding        `json:"warnings,omitempty"`
	Errors      []Finding        `json:"errors,omitempty"`
	WorldCounts worldload.Counts `json:"worldCounts"`
}

type Manifest struct {
	SchemaVersion   string           `json:"schemaVersion"`
	ResolverVersion string           `json:"resolverVersion"`
	Root            string           `json:"root"`
	GeneratedAt     time.Time        `json:"generatedAt"`
	Counts          Counts           `json:"counts"`
	WorldCounts     worldload.Counts `json:"worldCounts"`
	Files           []ArtifactFile   `json:"files"`
	Warnings        []Finding        `json:"warnings,omitempty"`
	Errors          []Finding        `json:"errors,omitempty"`
}

type ArtifactFile struct {
	Path    string `json:"path"`
	Format  string `json:"format"`
	Records int    `json:"records"`
	Bytes   int64  `json:"bytes,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
}

type Counts struct {
	EvidenceRecords            int `json:"evidenceRecords"`
	FindingRecords             int `json:"findingRecords"`
	ObjectInstances            int `json:"objectInstances"`
	Resolved                   int `json:"resolved"`
	Unresolved                 int `json:"unresolved"`
	Ambiguous                  int `json:"ambiguous"`
	Synthetic                  int `json:"synthetic"`
	MissingPrototypeResolution int `json:"missingPrototypeResolution"`
	CandidateTruncated         int `json:"candidateTruncated"`
	SourceFiles                int `json:"sourceFiles"`
	SourceHashErrors           int `json:"sourceHashErrors"`
	Warnings                   int `json:"warnings"`
	Errors                     int `json:"errors"`
}

type EvidenceRecord struct {
	SchemaVersion       string                            `json:"schemaVersion"`
	ResolverVersion     string                            `json:"resolverVersion"`
	EvidenceID          string                            `json:"evidenceId"`
	ObjectInstanceID    model.ObjectInstanceID            `json:"objectInstanceId"`
	PrototypeID         model.PrototypeID                 `json:"prototypeId"`
	Location            model.ObjectLocation              `json:"location"`
	Source              SourceEvidence                    `json:"source"`
	CFormat             CFormatEvidence                   `json:"cFormat"`
	Tags                []string                          `json:"tags,omitempty"`
	Resolution          model.PrototypeResolutionMetadata `json:"resolution"`
	CandidateCap        int                               `json:"candidateCap,omitempty"`
	CandidatesTruncated bool                              `json:"candidatesTruncated,omitempty"`
}

type SourceEvidence struct {
	LegacyKind     string `json:"legacyKind,omitempty"`
	LegacyID       string `json:"legacyId,omitempty"`
	LegacyPath     string `json:"legacyPath,omitempty"`
	LegacyEncoding string `json:"legacyEncoding,omitempty"`
	ObjectTreePath string `json:"objectTreePath,omitempty"`
	RecordIndex    int    `json:"recordIndex,omitempty"`
	RecordOffset   int64  `json:"recordOffset,omitempty"`
	FileSize       int64  `json:"fileSize,omitempty"`
	FileSHA256     string `json:"fileSha256,omitempty"`
	Error          string `json:"error,omitempty"`
}

type CFormatEvidence struct {
	ObjectStructSizeBytes        int    `json:"objectStructSizeBytes"`
	TreeChildCountSizeBytes      int    `json:"treeChildCountSizeBytes"`
	PrototypePathPattern         string `json:"prototypePathPattern"`
	PrototypeFileSizeRecords     int    `json:"prototypeFileSizeRecords"`
	ComparableOffset             int    `json:"comparableOffset"`
	ComparableBytes              int    `json:"comparableBytes"`
	ExcludedRuntimeOffset        int    `json:"excludedRuntimeOffset"`
	ExcludedRuntimeBytes         int    `json:"excludedRuntimeBytes"`
	FingerprintAlgorithm         string `json:"fingerprintAlgorithm"`
	FingerprintMethod            string `json:"fingerprintMethod"`
	LegacyNumberFormula          string `json:"legacyNumberFormula"`
	PrototypeRecordOffsetFormula string `json:"prototypeRecordOffsetFormula"`
}

type FindingRecord struct {
	SchemaVersion string         `json:"schemaVersion"`
	Severity      string         `json:"severity"`
	Kind          string         `json:"kind"`
	Path          string         `json:"path,omitempty"`
	ID            string         `json:"id,omitempty"`
	Ref           string         `json:"ref,omitempty"`
	Message       string         `json:"message"`
	Source        SourceEvidence `json:"source,omitempty"`
}

type Finding struct {
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type sourceCache struct {
	root  string
	files map[string]SourceEvidence
}

func Build(opts Options) (Snapshot, error) {
	root := opts.Root
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve root: %w", err)
	}

	summary, err := worldload.LoadRoot(absRoot)
	if err != nil {
		return Snapshot{}, err
	}
	return BuildFromSummary(absRoot, summary, opts.GeneratedAt)
}

func BuildFromSummary(root string, summary worldload.Summary, generatedAt time.Time) (Snapshot, error) {
	if root == "" {
		root = summary.Root
	}
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve root: %w", err)
	}
	if summary.World == nil {
		return Snapshot{}, fmt.Errorf("world summary is missing world data")
	}
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	cache := sourceCache{root: absRoot, files: map[string]SourceEvidence{}}
	snapshot := Snapshot{
		Root:        absRoot,
		GeneratedAt: generatedAt.UTC(),
		WorldCounts: summary.Counts,
		Warnings:    []Finding{},
		Errors:      []Finding{},
	}
	snapshot.Evidence = buildEvidenceRecords(summary.World.Objects, &cache, &snapshot.Counts)
	snapshot.Findings = buildFindingRecords(summary.Warnings, summary.Errors, &cache)
	snapshot.Counts.FindingRecords = len(snapshot.Findings)
	snapshot.Counts.SourceFiles = cache.countHashed()
	snapshot.Counts.Warnings = len(snapshot.Warnings)
	snapshot.Counts.Errors = len(snapshot.Errors)
	return snapshot, nil
}

func Write(outdir string, snapshot Snapshot) (Manifest, error) {
	if outdir == "" {
		return Manifest{}, fmt.Errorf("missing output directory")
	}
	if err := os.MkdirAll(outdir, 0700); err != nil {
		return Manifest{}, fmt.Errorf("create output directory %q: %w", outdir, err)
	}

	evidenceRel := evidenceFileName
	findingsRel := findingsFileName
	evidencePath := filepath.Join(outdir, evidenceRel)
	findingsPath := filepath.Join(outdir, findingsRel)
	if err := jsonstore.WriteJSONL(evidencePath, snapshot.Evidence); err != nil {
		return Manifest{}, err
	}
	if err := jsonstore.WriteJSONL(findingsPath, snapshot.Findings); err != nil {
		return Manifest{}, err
	}

	files := []ArtifactFile{
		artifactFile(evidenceRel, "jsonl", len(snapshot.Evidence), evidencePath),
		artifactFile(findingsRel, "jsonl", len(snapshot.Findings), findingsPath),
	}
	manifest := Manifest{
		SchemaVersion:   SchemaVersion,
		ResolverVersion: ResolverVersion,
		Root:            snapshot.Root,
		GeneratedAt:     snapshot.GeneratedAt.UTC(),
		Counts:          snapshot.Counts,
		WorldCounts:     snapshot.WorldCounts,
		Files:           files,
		Warnings:        snapshot.Warnings,
		Errors:          snapshot.Errors,
	}
	if err := jsonstore.WriteJSON(filepath.Join(outdir, indexFileName), manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func buildEvidenceRecords(objects map[model.ObjectInstanceID]model.ObjectInstance, cache *sourceCache, counts *Counts) []EvidenceRecord {
	ids := make([]model.ObjectInstanceID, 0, len(objects))
	for id := range objects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	records := make([]EvidenceRecord, 0, len(ids))
	counts.ObjectInstances = len(ids)
	for _, id := range ids {
		object := objects[id]
		if object.Metadata.PrototypeResolution == nil {
			counts.MissingPrototypeResolution++
			continue
		}
		resolution := clonePrototypeResolution(object.Metadata.PrototypeResolution)
		source := cache.source(object.Metadata)
		if source.Error != "" {
			counts.SourceHashErrors++
		}
		countStatus(counts, resolution.Status)
		truncated := resolution.CandidateCount > len(resolution.Candidates)
		if truncated {
			counts.CandidateTruncated++
		}
		record := EvidenceRecord{
			SchemaVersion:       SchemaVersion,
			ResolverVersion:     ResolverVersion,
			ObjectInstanceID:    object.ID,
			PrototypeID:         object.PrototypeID,
			Location:            object.Location,
			Source:              source,
			CFormat:             defaultCFormatEvidence(),
			Tags:                append([]string(nil), object.Metadata.Tags...),
			Resolution:          *resolution,
			CandidateCap:        len(resolution.Candidates),
			CandidatesTruncated: truncated,
		}
		record.EvidenceID = evidenceID(record)
		records = append(records, record)
	}
	counts.EvidenceRecords = len(records)
	return records
}

func defaultCFormatEvidence() CFormatEvidence {
	return CFormatEvidence{
		ObjectStructSizeBytes:        cbin.ObjectSize,
		TreeChildCountSizeBytes:      4,
		PrototypePathPattern:         "objmon/o%02d",
		PrototypeFileSizeRecords:     100,
		ComparableOffset:             0,
		ComparableBytes:              protoresolve.ObjectFingerprintComparableBytes,
		ExcludedRuntimeOffset:        protoresolve.ObjectFingerprintComparableBytes,
		ExcludedRuntimeBytes:         cbin.ObjectSize - protoresolve.ObjectFingerprintComparableBytes,
		FingerprintAlgorithm:         protoresolve.ObjectFingerprintAlgorithm,
		FingerprintMethod:            protoresolve.ObjectFingerprintMethod,
		LegacyNumberFormula:          "fileNumber*100 + index",
		PrototypeRecordOffsetFormula: "index*352",
	}
}

func evidenceID(record EvidenceRecord) string {
	parts := []string{
		string(record.ObjectInstanceID),
		string(record.PrototypeID),
		record.Source.LegacyPath,
		record.Source.ObjectTreePath,
		fmt.Sprintf("%d", record.Source.RecordOffset),
		record.Resolution.Status,
		record.Resolution.Fingerprint,
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func buildFindingRecords(warnings, errors []worldload.Finding, cache *sourceCache) []FindingRecord {
	records := make([]FindingRecord, 0, len(warnings)+len(errors))
	for _, finding := range warnings {
		records = append(records, findingRecord("warning", finding, cache))
	}
	for _, finding := range errors {
		records = append(records, findingRecord("error", finding, cache))
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Severity != records[j].Severity {
			return records[i].Severity < records[j].Severity
		}
		if records[i].Path != records[j].Path {
			return records[i].Path < records[j].Path
		}
		if records[i].Kind != records[j].Kind {
			return records[i].Kind < records[j].Kind
		}
		return records[i].Message < records[j].Message
	})
	return records
}

func findingRecord(severity string, finding worldload.Finding, cache *sourceCache) FindingRecord {
	return FindingRecord{
		SchemaVersion: SchemaVersion,
		Severity:      severity,
		Kind:          finding.Kind,
		Path:          finding.Path,
		ID:            finding.ID,
		Ref:           finding.Ref,
		Message:       finding.Message,
		Source:        cache.source(model.Metadata{LegacyPath: finding.Path}),
	}
}

func countStatus(counts *Counts, status string) {
	switch status {
	case "resolved":
		counts.Resolved++
	case "unresolved":
		counts.Unresolved++
	case "ambiguous":
		counts.Ambiguous++
	case "synthetic":
		counts.Synthetic++
	}
}

func clonePrototypeResolution(resolution *model.PrototypeResolutionMetadata) *model.PrototypeResolutionMetadata {
	if resolution == nil {
		return nil
	}
	cloned := *resolution
	if len(resolution.Candidates) > 0 {
		cloned.Candidates = append([]model.PrototypeResolutionCandidate(nil), resolution.Candidates...)
	}
	return &cloned
}

func (c *sourceCache) source(metadata model.Metadata) SourceEvidence {
	source := SourceEvidence{
		LegacyKind:     metadata.LegacyKind,
		LegacyID:       metadata.LegacyID,
		LegacyPath:     metadata.LegacyPath,
		LegacyEncoding: metadata.LegacyEncoding,
		ObjectTreePath: metadata.ObjectTreePath,
		RecordIndex:    metadata.RecordIndex,
		RecordOffset:   metadata.RecordOffset,
	}
	if metadata.LegacyPath == "" {
		return source
	}
	file, err := c.safePath(metadata.LegacyPath)
	if err != nil {
		source.Error = err.Error()
		return source
	}
	if cached, ok := c.files[file]; ok {
		source.FileSize = cached.FileSize
		source.FileSHA256 = cached.FileSHA256
		source.Error = cached.Error
		return source
	}
	fileEvidence := hashFile(file)
	c.files[file] = fileEvidence
	source.FileSize = fileEvidence.FileSize
	source.FileSHA256 = fileEvidence.FileSHA256
	source.Error = fileEvidence.Error
	return source
}

func (c *sourceCache) safePath(path string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(path))
	if filepath.IsAbs(clean) {
		rel, err := filepath.Rel(c.root, clean)
		if err != nil {
			return "", fmt.Errorf("source path outside root: %s", path)
		}
		clean = rel
	}
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("source path outside root: %s", path)
	}
	direct := filepath.Join(c.root, clean)
	if _, err := os.Stat(direct); err == nil {
		return direct, nil
	}
	resolved, err := c.resolveDecodedPath(clean)
	if err == nil {
		return resolved, nil
	}
	return direct, nil
}

func (c *sourceCache) resolveDecodedPath(clean string) (string, error) {
	current := c.root
	for _, part := range strings.Split(filepath.ToSlash(clean), "/") {
		if part == "" || part == "." {
			continue
		}
		entries, err := os.ReadDir(current)
		if err != nil {
			return "", err
		}
		found := ""
		for _, entry := range entries {
			name := entry.Name()
			if name == part {
				found = name
				break
			}
			decoded, err := legacykr.DecodeEUCKRContext(legacykr.Context{
				Path:  filepath.ToSlash(current),
				Field: "path component",
			}, []byte(name))
			if err == nil && decoded == part {
				found = name
				break
			}
		}
		if found == "" {
			return "", fmt.Errorf("source path not found: %s", clean)
		}
		current = filepath.Join(current, found)
	}
	return current, nil
}

func (c *sourceCache) countHashed() int {
	n := 0
	for _, source := range c.files {
		if source.Error == "" {
			n++
		}
	}
	return n
}

func hashFile(path string) SourceEvidence {
	file, err := os.Open(path)
	if err != nil {
		return SourceEvidence{Error: err.Error()}
	}
	defer file.Close()
	h := sha256.New()
	n, err := io.Copy(h, file)
	if err != nil {
		return SourceEvidence{Error: err.Error()}
	}
	return SourceEvidence{
		FileSize:   n,
		FileSHA256: hex.EncodeToString(h.Sum(nil)),
	}
}

func artifactFile(relPath, format string, records int, path string) ArtifactFile {
	info, err := os.Stat(path)
	file := ArtifactFile{
		Path:    filepath.ToSlash(relPath),
		Format:  format,
		Records: records,
	}
	if err == nil {
		file.Bytes = info.Size()
		if hash := hashFile(path); hash.Error == "" {
			file.SHA256 = hash.FileSHA256
		}
	}
	return file
}
