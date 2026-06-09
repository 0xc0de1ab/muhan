package protoresolve

import (
	"os"
	"path/filepath"
	"testing"

	"muhan/internal/persist/cbin"
	"muhan/internal/world/model"
)

func TestObjectResolverExactMatchIgnoresPointerFields(t *testing.T) {
	proto := objectRecord(t, "sword")
	inst := objectRecord(t, "sword")
	for i := 336; i < cbin.ObjectSize; i++ {
		inst.Raw[i] = 0xff
	}

	resolver := NewObjectResolver()
	resolver.Add(proto, ObjectCandidate{PrototypeID: "object:o00:0", Path: "objmon/o00"})

	got := resolver.ResolveObjectPrototype(inst)
	if !got.Resolved || got.PrototypeID != model.PrototypeID("object:o00:0") ||
		got.Method != "exact_record_without_pointers" {
		t.Fatalf("resolution = %+v", got)
	}
}

func TestObjectResolverAmbiguousMatch(t *testing.T) {
	record := objectRecord(t, "coin")
	resolver := NewObjectResolver()
	resolver.Add(record, ObjectCandidate{PrototypeID: "object:o00:0"})
	resolver.Add(record, ObjectCandidate{PrototypeID: "object:o01:0"})

	got := resolver.ResolveObjectPrototype(record)
	if got.Resolved || !got.Ambiguous || len(got.Candidates) != 2 {
		t.Fatalf("resolution = %+v", got)
	}
}

func TestObjectResolverNoMatch(t *testing.T) {
	resolver := NewObjectResolver()
	resolver.Add(objectRecord(t, "sword"), ObjectCandidate{PrototypeID: "object:o00:0"})

	got := resolver.ResolveObjectPrototype(objectRecord(t, "shield"))
	if got.Resolved || got.Ambiguous || got.Confidence != "none" {
		t.Fatalf("resolution = %+v", got)
	}
}

func TestBuildObjectResolverAddsLegacyCandidateEvidence(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "objmon"), 0700); err != nil {
		t.Fatal(err)
	}
	data := append(objectRecordBytes("first"), objectRecordBytes("second")...)
	if err := os.WriteFile(filepath.Join(root, "objmon", "o02"), data, 0600); err != nil {
		t.Fatal(err)
	}

	resolver, err := BuildObjectResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	got := resolver.ResolveObjectPrototype(objectRecord(t, "second"))
	if !got.Resolved || len(got.Candidates) != 1 {
		t.Fatalf("resolution = %+v", got)
	}
	candidate := got.Candidates[0]
	if candidate.PrototypeID != "object:o02:1" ||
		candidate.Path != "objmon/o02" ||
		candidate.Index != 1 ||
		candidate.LegacyNumber != 201 ||
		candidate.RecordOffset != int64(cbin.ObjectSize) {
		t.Fatalf("candidate = %+v", candidate)
	}
	if got.Fingerprint == "" ||
		got.FingerprintAlgorithm != ObjectFingerprintAlgorithm ||
		got.ComparableBytes != ObjectFingerprintComparableBytes {
		t.Fatalf("fingerprint evidence = %+v", got)
	}
}

func TestSyntheticObjectPrototypeFromInstance(t *testing.T) {
	object := model.ObjectInstance{
		ID:                  "objinst:test:00000000",
		PrototypeID:         "object:objinst:test:00000000",
		DisplayNameOverride: "bag",
		Properties: map[string]string{
			"name":        "bag",
			"description": "holds items",
			"key[0]":      "open",
			"useOutput":   "opened",
		},
		Metadata: model.Metadata{
			Source:         "legacy",
			LegacyPath:     "player/test",
			LegacyEncoding: "euc-kr/cp949",
			RecordOffset:   42,
			RawFields: map[string][]byte{
				"name": []byte("bag"),
			},
			Tags:  []string{"prototype:synthetic"},
			Notes: []string{"prototypeMatch method=exact_record_without_pointers confidence=fallback"},
			PrototypeResolution: &model.PrototypeResolutionMetadata{
				Status:               "unresolved",
				Method:               ObjectFingerprintMethod,
				Confidence:           "fallback",
				SelectedPrototypeID:  "object:objinst:test:00000000",
				SyntheticPrototypeID: "object:objinst:test:00000000",
				Candidates: []model.PrototypeResolutionCandidate{{
					PrototypeID: "object:o00:1",
					Path:        "objmon/o00",
					Index:       1,
				}},
				CandidateCount:       1,
				Fingerprint:          "abcd",
				FingerprintAlgorithm: ObjectFingerprintAlgorithm,
				ComparableBytes:      ObjectFingerprintComparableBytes,
			},
		},
	}

	got, ok := SyntheticObjectPrototypeFromInstance(object)
	if !ok {
		t.Fatal("expected synthetic prototype")
	}
	if got.ID != object.PrototypeID || got.DisplayName != "bag" || got.Description != "holds items" {
		t.Fatalf("prototype = %+v", got)
	}
	if len(got.Keywords) != 1 || got.Keywords[0] != "open" {
		t.Fatalf("keywords = %+v", got.Keywords)
	}
	if got.Properties["useOutput"] != "opened" {
		t.Fatalf("properties = %+v", got.Properties)
	}
	if !hasProtoTag(got.Metadata.Tags, "prototype:materialized") ||
		!hasProtoTag(got.Metadata.Tags, "prototype:synthetic") {
		t.Fatalf("tags = %+v", got.Metadata.Tags)
	}
	if got.Metadata.RawFields["name"][0] != 'b' {
		t.Fatalf("raw fields = %+v", got.Metadata.RawFields)
	}
	object.Metadata.RawFields["name"][0] = 'x'
	if got.Metadata.RawFields["name"][0] != 'b' {
		t.Fatalf("raw fields alias source map")
	}
	if got.Metadata.PrototypeResolution == nil ||
		got.Metadata.PrototypeResolution.MaterializedFromObjectInstanceID != object.ID ||
		got.Metadata.PrototypeResolution.Status != "unresolved" ||
		len(got.Metadata.PrototypeResolution.Candidates) != 1 {
		t.Fatalf("prototype resolution = %+v", got.Metadata.PrototypeResolution)
	}
	object.Metadata.PrototypeResolution.Candidates[0].PrototypeID = "object:o99:9"
	if got.Metadata.PrototypeResolution.Candidates[0].PrototypeID != "object:o00:1" {
		t.Fatalf("prototype resolution candidates alias source slice")
	}
}

func TestSyntheticObjectPrototypeFallsBackToObjectKey(t *testing.T) {
	object := model.ObjectInstance{
		ID:          "objinst:test:00000000",
		PrototypeID: "object:objinst:test:00000000",
		Properties: map[string]string{
			"key[0]": "번개검",
		},
		Metadata: model.Metadata{Tags: []string{"prototype:synthetic"}},
	}

	got, ok := SyntheticObjectPrototypeFromInstance(object)
	if !ok {
		t.Fatal("expected synthetic prototype")
	}
	if got.DisplayName != "번개검" {
		t.Fatalf("display name = %q, want 번개검", got.DisplayName)
	}
}

func TestSyntheticObjectPrototypeFromInstanceSkipsResolved(t *testing.T) {
	object := model.ObjectInstance{
		ID:          "objinst:test:00000000",
		PrototypeID: "object:o00:0",
		Metadata: model.Metadata{
			Tags: []string{"prototype:resolved"},
		},
	}
	if _, ok := SyntheticObjectPrototypeFromInstance(object); ok {
		t.Fatal("resolved object should not materialize a synthetic prototype")
	}
}

func TestMaterializeSyntheticObjectPrototypesIsDeterministic(t *testing.T) {
	objects := map[model.ObjectInstanceID]model.ObjectInstance{
		"objinst:test:2": {
			ID:                  "objinst:test:2",
			PrototypeID:         "object:objinst:test:shared",
			DisplayNameOverride: "second",
			Metadata:            model.Metadata{Tags: []string{"prototype:synthetic"}},
		},
		"objinst:test:1": {
			ID:                  "objinst:test:1",
			PrototypeID:         "object:objinst:test:shared",
			DisplayNameOverride: "first",
			Metadata:            model.Metadata{Tags: []string{"prototype:synthetic"}},
		},
	}

	got := MaterializeSyntheticObjectPrototypes(objects)
	if len(got) != 1 || got[0].DisplayName != "first" {
		t.Fatalf("prototypes = %+v", got)
	}
}

func objectRecord(t *testing.T, name string) cbin.ObjectRecord {
	t.Helper()
	data := objectRecordBytes(name)
	record, err := cbin.DecodeObjectRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func objectRecordBytes(name string) []byte {
	data := make([]byte, cbin.ObjectSize)
	copy(data, []byte(name+"\x00"))
	return data
}

func hasProtoTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}
