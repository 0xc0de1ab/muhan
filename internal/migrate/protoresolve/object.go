package protoresolve

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/persist/cbin"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	ObjectFingerprintMethod          = "exact_record_without_pointers"
	ObjectFingerprintAlgorithm       = "sha256"
	ObjectFingerprintComparableBytes = 336
	legacyObjectFileSize             = 100
)

type ObjectResolver struct {
	byFingerprint map[string][]ObjectCandidate
}

type ObjectCandidate struct {
	PrototypeID  model.PrototypeID `json:"prototypeId"`
	Path         string            `json:"path,omitempty"`
	Index        int               `json:"index"`
	LegacyNumber int               `json:"legacyNumber"`
	RecordOffset int64             `json:"recordOffset"`
}

type ObjectResolution struct {
	PrototypeID          model.PrototypeID `json:"prototypeId,omitempty"`
	Method               string            `json:"method,omitempty"`
	Confidence           string            `json:"confidence,omitempty"`
	Fingerprint          string            `json:"fingerprint,omitempty"`
	FingerprintAlgorithm string            `json:"fingerprintAlgorithm,omitempty"`
	ComparableBytes      int               `json:"comparableBytes,omitempty"`
	Candidates           []ObjectCandidate `json:"candidates,omitempty"`
	Resolved             bool              `json:"resolved"`
	Ambiguous            bool              `json:"ambiguous,omitempty"`
}

type ObjectPrototypeResolver interface {
	ResolveObjectPrototype(record cbin.ObjectRecord) ObjectResolution
}

var objectProtoRE = regexp.MustCompile(`^o[0-9][0-9]$`)

func BuildObjectResolver(root string) (*ObjectResolver, error) {
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}

	resolver := NewObjectResolver()
	dir := filepath.Join(absRoot, "objmon")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return resolver, nil
		}
		return nil, fmt.Errorf("read objmon: %w", err)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !objectProtoRE.MatchString(entry.Name()) {
			continue
		}
		fileNumber, _ := strconv.Atoi(strings.TrimPrefix(entry.Name(), "o"))
		path := filepath.Join(dir, entry.Name())
		relPath := displayPath(absRoot, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read object prototypes %s: %w", relPath, err)
		}
		records, err := cbin.DecodeObjectRecords(data)
		if err != nil {
			return nil, fmt.Errorf("decode object prototypes %s: %w", relPath, err)
		}
		for i, record := range records {
			resolver.Add(record, ObjectCandidate{
				PrototypeID:  model.PrototypeID(fmt.Sprintf("object:%s:%d", entry.Name(), i)),
				Path:         relPath,
				Index:        i,
				LegacyNumber: fileNumber*legacyObjectFileSize + i,
				RecordOffset: int64(i * cbin.ObjectSize),
			})
		}
	}
	return resolver, nil
}

func NewObjectResolver() *ObjectResolver {
	return &ObjectResolver{byFingerprint: map[string][]ObjectCandidate{}}
}

func (r *ObjectResolver) Add(record cbin.ObjectRecord, candidate ObjectCandidate) {
	if r == nil {
		return
	}
	if r.byFingerprint == nil {
		r.byFingerprint = map[string][]ObjectCandidate{}
	}
	fp := FingerprintObjectRecord(record)
	if fp == "" {
		return
	}
	r.byFingerprint[fp] = append(r.byFingerprint[fp], candidate)
	sort.SliceStable(r.byFingerprint[fp], func(i, j int) bool {
		return r.byFingerprint[fp][i].PrototypeID < r.byFingerprint[fp][j].PrototypeID
	})
}

func (r *ObjectResolver) ResolveObjectPrototype(record cbin.ObjectRecord) ObjectResolution {
	if r == nil {
		return ObjectResolution{}
	}
	fp := FingerprintObjectRecord(record)
	if fp == "" {
		return ObjectResolution{}
	}
	candidates := r.byFingerprint[fp]
	base := ObjectResolution{
		Method:               ObjectFingerprintMethod,
		Fingerprint:          fp,
		FingerprintAlgorithm: ObjectFingerprintAlgorithm,
		ComparableBytes:      ObjectFingerprintComparableBytes,
	}
	switch len(candidates) {
	case 0:
		base.Confidence = "none"
		return base
	case 1:
		base.PrototypeID = candidates[0].PrototypeID
		base.Confidence = "exact"
		base.Candidates = append([]ObjectCandidate(nil), candidates...)
		base.Resolved = true
		return base
	default:
		base.Confidence = "ambiguous"
		base.Candidates = append([]ObjectCandidate(nil), candidates...)
		base.Ambiguous = true
		return base
	}
}

func FingerprintObjectRecord(record cbin.ObjectRecord) string {
	if len(record.Raw) < ObjectFingerprintComparableBytes {
		return ""
	}
	sum := sha256.Sum256(record.Raw[:ObjectFingerprintComparableBytes])
	return hex.EncodeToString(sum[:])
}

// SyntheticObjectPrototypeFromInstance converts an unresolved object instance
// into a canonical prototype record. Exact prototype matches are left out; this
// is only for object instances that objectmap explicitly marked synthetic.
func SyntheticObjectPrototypeFromInstance(object model.ObjectInstance) (model.ObjectPrototype, bool) {
	if object.PrototypeID.IsZero() || !hasTag(object.Metadata.Tags, "prototype:synthetic") {
		return model.ObjectPrototype{}, false
	}

	displayName := strings.TrimSpace(object.DisplayNameOverride)
	if displayName == "" {
		displayName = strings.TrimSpace(object.Properties["name"])
	}
	if displayName == "" {
		displayName = firstObjectKey(object.Properties)
	}
	if displayName == "" {
		displayName = string(object.PrototypeID)
	}

	keywords := make([]string, 0, 3)
	for _, key := range []string{"key[0]", "key[1]", "key[2]"} {
		if text := strings.TrimSpace(object.Properties[key]); text != "" {
			keywords = append(keywords, text)
		}
	}

	properties := map[string]string{}
	for key, value := range object.Properties {
		switch key {
		case "name", "description", "key[0]", "key[1]", "key[2]":
			continue
		default:
			if value != "" {
				properties[key] = value
			}
		}
	}

	metadata := model.Metadata{
		Source:              object.Metadata.Source,
		LegacyKind:          "syntheticObjectPrototype",
		LegacyID:            string(object.PrototypeID),
		LegacyPath:          object.Metadata.LegacyPath,
		LegacyEncoding:      object.Metadata.LegacyEncoding,
		RecordOffset:        object.Metadata.RecordOffset,
		ObjectTreePath:      object.Metadata.ObjectTreePath,
		RawFields:           cloneRawFields(object.Metadata.RawFields),
		Tags:                appendMaterializedTag(object.Metadata.Tags),
		Notes:               append([]string{fmt.Sprintf("materializedFromObjectInstance=%s", object.ID)}, object.Metadata.Notes...),
		PrototypeResolution: materializedPrototypeResolution(object),
	}

	return model.ObjectPrototype{
		ID:          object.PrototypeID,
		DisplayName: displayName,
		Description: strings.TrimSpace(object.Properties["description"]),
		Keywords:    nilIfEmptyStrings(keywords),
		Properties:  nilIfEmptyProperties(properties),
		Metadata:    metadata,
	}, true
}

func firstObjectKey(properties map[string]string) string {
	for _, key := range []string{"key[0]", "key[1]", "key[2]"} {
		if value := strings.TrimSpace(properties[key]); value != "" {
			return value
		}
	}
	return ""
}

// MaterializeSyntheticObjectPrototypes returns deterministic prototype records
// for all synthetic object instances. Duplicate prototype IDs are collapsed by
// the lowest object instance ID to keep output stable.
func MaterializeSyntheticObjectPrototypes(objects map[model.ObjectInstanceID]model.ObjectInstance) []model.ObjectPrototype {
	ids := make([]model.ObjectInstanceID, 0, len(objects))
	for id := range objects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	seen := map[model.PrototypeID]struct{}{}
	prototypes := make([]model.ObjectPrototype, 0)
	for _, id := range ids {
		proto, ok := SyntheticObjectPrototypeFromInstance(objects[id])
		if !ok {
			continue
		}
		if _, exists := seen[proto.ID]; exists {
			continue
		}
		seen[proto.ID] = struct{}{}
		prototypes = append(prototypes, proto)
	}
	return prototypes
}

func hasTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

func appendMaterializedTag(tags []string) []string {
	out := make([]string, 0, len(tags)+1)
	seen := map[string]struct{}{}
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	if _, ok := seen["prototype:materialized"]; !ok {
		out = append(out, "prototype:materialized")
	}
	return out
}

func cloneRawFields(fields map[string][]byte) map[string][]byte {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string][]byte, len(fields))
	for key, value := range fields {
		cloned := make([]byte, len(value))
		copy(cloned, value)
		out[key] = cloned
	}
	return out
}

func materializedPrototypeResolution(object model.ObjectInstance) *model.PrototypeResolutionMetadata {
	resolution := clonePrototypeResolution(object.Metadata.PrototypeResolution)
	if resolution == nil {
		resolution = &model.PrototypeResolutionMetadata{
			Status:               "synthetic",
			SelectedPrototypeID:  object.PrototypeID,
			SyntheticPrototypeID: object.PrototypeID,
		}
	}
	if resolution.SelectedPrototypeID.IsZero() {
		resolution.SelectedPrototypeID = object.PrototypeID
	}
	if resolution.SyntheticPrototypeID.IsZero() && hasTag(object.Metadata.Tags, "prototype:synthetic") {
		resolution.SyntheticPrototypeID = object.PrototypeID
	}
	resolution.MaterializedFromObjectInstanceID = object.ID
	return resolution
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

func nilIfEmptyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return values
}

func nilIfEmptyProperties(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	return values
}

func displayPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
