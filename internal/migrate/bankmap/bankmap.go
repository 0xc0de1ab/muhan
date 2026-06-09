package bankmap

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"muhan/internal/migrate/objectmap"
	"muhan/internal/migrate/protoresolve"
	"muhan/internal/persist/cbin"
	"muhan/internal/persist/legacykr"
	"muhan/internal/world/model"
)

const (
	KindPlayer = "player"
	KindFamily = "family"
)

type Options struct {
	Root              string
	IncludeObjects    bool
	PrototypeResolver protoresolve.ObjectPrototypeResolver
	SkipBankIDs       map[string]bool
}

type BankRecord struct {
	ID            string                   `json:"id"`
	Kind          string                   `json:"kind"`
	OwnerName     string                   `json:"ownerName"`
	Path          string                   `json:"path"`
	ObjectCount   int                      `json:"objectCount"`
	ObjectIDs     []model.ObjectInstanceID `json:"objectIds,omitempty"`
	TrailingBytes int                      `json:"trailingBytes"`
	Warnings      []string                 `json:"warnings"`
}

type Snapshot struct {
	Root     string                 `json:"root"`
	Counts   Counts                 `json:"counts"`
	Banks    []BankRecord           `json:"banks"`
	Objects  []model.ObjectInstance `json:"objects,omitempty"`
	Warnings []Finding              `json:"warnings"`
	Errors   []Finding              `json:"errors"`
}

type Counts struct {
	PlayerBanks        int `json:"playerBanks"`
	FamilyBanks        int `json:"familyBanks"`
	TotalBanks         int `json:"totalBanks"`
	Objects            int `json:"objects"`
	TrailingBytes      int `json:"trailingBytes"`
	MaxDepth           int `json:"maxDepth"`
	PrototypeResolved  int `json:"prototypeResolved"`
	PrototypeSynthetic int `json:"prototypeSynthetic"`
	PrototypeAmbiguous int `json:"prototypeAmbiguous"`
	Warnings           int `json:"warnings"`
	Errors             int `json:"errors"`
}

type Finding struct {
	Path    string `json:"path,omitempty"`
	ID      string `json:"id,omitempty"`
	Message string `json:"message"`
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

	snapshot := Snapshot{
		Root:     absRoot,
		Warnings: []Finding{},
		Errors:   []Finding{},
	}
	if opts.IncludeObjects && opts.PrototypeResolver == nil {
		resolver, err := protoresolve.BuildObjectResolver(absRoot)
		if err != nil {
			snapshot.addWarning(displayRelPath(absRoot, filepath.Join(absRoot, "objmon")), "", err.Error())
		} else {
			opts.PrototypeResolver = resolver
		}
	}

	snapshot.scanBankDir(absRoot, filepath.Join(absRoot, "player", "bank"), KindPlayer, opts)
	snapshot.scanBankDir(absRoot, filepath.Join(absRoot, "player", "family", "bank"), KindFamily, opts)

	sort.SliceStable(snapshot.Banks, func(i, j int) bool {
		if snapshot.Banks[i].Kind != snapshot.Banks[j].Kind {
			return snapshot.Banks[i].Kind < snapshot.Banks[j].Kind
		}
		return snapshot.Banks[i].Path < snapshot.Banks[j].Path
	})

	snapshot.Counts.TotalBanks = snapshot.Counts.PlayerBanks + snapshot.Counts.FamilyBanks
	snapshot.Counts.Warnings = len(snapshot.Warnings)
	snapshot.Counts.Errors = len(snapshot.Errors)
	return snapshot, nil
}

func EncodeJSON(w io.Writer, snapshot Snapshot) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(snapshot)
}

func WriteText(w io.Writer, snapshot Snapshot, maxFindings int) {
	fmt.Fprintf(w, "root: %s\n", snapshot.Root)
	fmt.Fprintf(w, "playerBanks: %d\n", snapshot.Counts.PlayerBanks)
	fmt.Fprintf(w, "familyBanks: %d\n", snapshot.Counts.FamilyBanks)
	fmt.Fprintf(w, "totalBanks: %d\n", snapshot.Counts.TotalBanks)
	fmt.Fprintf(w, "objects: %d\n", snapshot.Counts.Objects)
	fmt.Fprintf(w, "trailingBytes: %d\n", snapshot.Counts.TrailingBytes)
	fmt.Fprintf(w, "maxDepth: %d\n", snapshot.Counts.MaxDepth)
	fmt.Fprintf(w, "prototype resolution: resolved=%d synthetic=%d ambiguous=%d\n",
		snapshot.Counts.PrototypeResolved,
		snapshot.Counts.PrototypeSynthetic,
		snapshot.Counts.PrototypeAmbiguous,
	)
	writeFindings(w, "warnings", snapshot.Warnings, maxFindings)
	writeFindings(w, "errors", snapshot.Errors, maxFindings)
}

func writeFindings(w io.Writer, label string, findings []Finding, max int) {
	fmt.Fprintf(w, "%s: %d\n", label, len(findings))
	limit := max
	if limit <= 0 || limit > len(findings) {
		limit = len(findings)
	}
	for _, finding := range findings[:limit] {
		fmt.Fprintf(w, "%s: %s %s\n", strings.TrimSuffix(label, "s"), findingLocation(finding), finding.Message)
	}
	if limit < len(findings) {
		fmt.Fprintf(w, "  ... %d more\n", len(findings)-limit)
	}
}

func findingLocation(f Finding) string {
	switch {
	case f.Path != "" && f.ID != "":
		return f.Path + " " + f.ID
	case f.Path != "":
		return f.Path
	case f.ID != "":
		return f.ID
	default:
		return "-"
	}
}

func (s *Snapshot) scanBankDir(root, dir, kind string, opts Options) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		s.addError(displayRelPath(root, dir), "", err)
		return
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			s.addWarning(displayRelPath(root, path), "", "bank entry is a directory; skipped")
			continue
		}
		s.mapBankFile(path, entry.Name(), kind, opts)
	}
}

func (s *Snapshot) mapBankFile(path, filename, kind string, opts Options) {
	ownerName, nameWarnings := decodeBankFilename(path, filename)
	relPath := bankRelPath(kind, ownerName)
	id := bankID(kind, ownerName)

	if opts.SkipBankIDs != nil && opts.SkipBankIDs[id] {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		s.addError(relPath, id, err)
		return
	}

	node, err := cbin.DecodeObjectTree(data, true)
	if err != nil {
		s.addError(relPath, id, fmt.Errorf("decode bank object tree: %w", err))
		return
	}

	warnings := make([]string, 0, len(nameWarnings)+len(node.Warnings))
	warnings = append(warnings, nameWarnings...)
	for _, warning := range node.Warnings {
		warnings = append(warnings, fmt.Sprintf("%s at offset %d: %s", warning.Field, warning.Offset, warning.Message))
	}

	record := BankRecord{
		ID:            id,
		Kind:          kind,
		OwnerName:     ownerName,
		Path:          relPath,
		ObjectCount:   node.Stats.Objects,
		TrailingBytes: node.Stats.TrailingBytes,
		Warnings:      warnings,
	}
	if opts.IncludeObjects {
		objectResult := objectmap.MapObjectTreeWithOptions(id, model.ObjectLocation{BankID: model.BankID(id), Slot: "bank"}, node, objectmap.Options{
			PrototypeResolver: opts.PrototypeResolver,
			SourcePath:        relPath,
		})
		objects := objectResult.Objects
		if len(objects) > 0 {
			record.ObjectIDs = []model.ObjectInstanceID{objects[0].ID}
			s.Objects = append(s.Objects, objects...)
		}
		s.Counts.PrototypeResolved += objectResult.PrototypeResolution.ResolvedExact
		s.Counts.PrototypeSynthetic += objectResult.PrototypeResolution.Synthetic
		s.Counts.PrototypeAmbiguous += objectResult.PrototypeResolution.AmbiguousSynthetic
	}
	s.Banks = append(s.Banks, record)

	switch kind {
	case KindPlayer:
		s.Counts.PlayerBanks++
	case KindFamily:
		s.Counts.FamilyBanks++
	}
	s.Counts.Objects += node.Stats.Objects
	s.Counts.TrailingBytes += node.Stats.TrailingBytes
	if node.Stats.MaxDepth > s.Counts.MaxDepth {
		s.Counts.MaxDepth = node.Stats.MaxDepth
	}
	for _, warning := range warnings {
		s.addWarning(relPath, id, warning)
	}
}

func (r BankRecord) BankAccount() model.BankAccount {
	account := model.BankAccount{
		ID:        model.BankID(r.ID),
		Kind:      r.Kind,
		OwnerName: r.OwnerName,
		Objects:   model.ObjectRefList{ObjectIDs: append([]model.ObjectInstanceID(nil), r.ObjectIDs...)},
		Metadata: model.Metadata{
			Source:     "legacy",
			LegacyKind: "bank",
			LegacyID:   r.ID,
			LegacyPath: r.Path,
		},
	}
	if r.Kind == KindPlayer && r.OwnerName != "" {
		account.OwnerPlayerID = model.PlayerID(r.OwnerName)
	}
	return account
}

func (s *Snapshot) addWarning(path, id, message string) {
	s.Warnings = append(s.Warnings, Finding{Path: path, ID: id, Message: message})
}

func (s *Snapshot) addError(path, id string, err error) {
	s.Errors = append(s.Errors, Finding{Path: path, ID: id, Message: err.Error()})
}

func decodeBankFilename(path, filename string) (string, []string) {
	if filename == "" {
		return "raw-filename-", []string{"bank filename is empty"}
	}

	decoded, err := legacykr.DecodeEUCKRContext(legacykr.Context{
		Path:  filepath.ToSlash(path),
		Field: "bank filename",
	}, []byte(filename))
	if err == nil {
		return decoded, nil
	}

	if utf8.ValidString(filename) {
		return filename, []string{
			fmt.Sprintf("bank filename decode failed as euc-kr/cp949: %v; using UTF-8 filename", err),
		}
	}

	fallback := "raw-filename-" + hex.EncodeToString([]byte(filename))
	return fallback, []string{fmt.Sprintf("bank filename decode failed: %v", err)}
}

func bankID(kind, ownerName string) string {
	if ownerName == "" {
		ownerName = "raw-filename-"
	}
	return "bank:" + kind + ":" + ownerName
}

func bankRelPath(kind, ownerName string) string {
	switch kind {
	case KindFamily:
		return strings.Join([]string{"player", "family", "bank", ownerName}, "/")
	default:
		return strings.Join([]string{"player", "bank", ownerName}, "/")
	}
}

func displayRelPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, part := range parts {
		decoded, _ := decodeBankFilename(path, part)
		parts[i] = decoded
	}
	return strings.Join(parts, "/")
}
