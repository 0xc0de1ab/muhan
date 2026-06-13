package protoaudit

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xc0de1ab/muhan/internal/persist/cbin"
	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

func TestBuildEvidenceRecordsDeterministicAndHashesSources(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "objects.dat"), []byte("source bytes"))

	objects := map[model.ObjectInstanceID]model.ObjectInstance{
		"objinst:2": objectInstance("objinst:2", "object:synthetic:2", "unresolved", "objects.dat"),
		"objinst:1": objectInstance("objinst:1", "object:o00:1", "resolved", "objects.dat"),
	}
	cache := sourceCache{root: root, files: map[string]SourceEvidence{}}
	var counts Counts

	got := buildEvidenceRecords(objects, &cache, &counts)
	if len(got) != 2 {
		t.Fatalf("records len = %d, want 2", len(got))
	}
	if got[0].ObjectInstanceID != "objinst:1" || got[1].ObjectInstanceID != "objinst:2" {
		t.Fatalf("records not sorted: %+v", got)
	}
	if counts.EvidenceRecords != 2 || counts.Resolved != 1 || counts.Unresolved != 1 {
		t.Fatalf("counts = %+v", counts)
	}
	wantHash := sha256Hex([]byte("source bytes"))
	if got[0].Source.FileSHA256 != wantHash || got[0].Source.FileSize != int64(len("source bytes")) {
		t.Fatalf("source = %+v, want hash %s", got[0].Source, wantHash)
	}
}

func TestBuildEvidenceRecordsCountsDocumentedResolutionStatuses(t *testing.T) {
	objects := map[model.ObjectInstanceID]model.ObjectInstance{
		"objinst:resolved":   objectInstance("objinst:resolved", "object:o00:1", "resolved", ""),
		"objinst:unresolved": objectInstance("objinst:unresolved", "object:synthetic:1", "unresolved", ""),
		"objinst:synthetic":  objectInstance("objinst:synthetic", "object:synthetic:2", "synthetic", ""),
		"objinst:ambiguous":  objectInstance("objinst:ambiguous", "object:synthetic:3", "ambiguous", ""),
		"objinst:missing": {
			ID:          "objinst:missing",
			PrototypeID: "object:synthetic:4",
			Quantity:    1,
			Location:    model.ObjectLocation{RoomID: "room:00001"},
		},
	}
	ambiguous := objects["objinst:ambiguous"]
	ambiguous.Metadata.PrototypeResolution.CandidateCount = 2
	ambiguous.Metadata.PrototypeResolution.Candidates = []model.PrototypeResolutionCandidate{{
		PrototypeID: "object:o00:1",
		Path:        "objmon/o00",
		Index:       1,
	}}
	objects["objinst:ambiguous"] = ambiguous

	cache := sourceCache{root: t.TempDir(), files: map[string]SourceEvidence{}}
	var counts Counts

	records := buildEvidenceRecords(objects, &cache, &counts)
	if len(records) != 4 {
		t.Fatalf("records len = %d, want 4", len(records))
	}
	if counts.ObjectInstances != 5 ||
		counts.EvidenceRecords != 4 ||
		counts.Resolved != 1 ||
		counts.Unresolved != 1 ||
		counts.Ambiguous != 1 ||
		counts.Synthetic != 1 ||
		counts.MissingPrototypeResolution != 1 ||
		counts.CandidateTruncated != 1 {
		t.Fatalf("counts = %+v", counts)
	}

	var ambiguousRecord EvidenceRecord
	for _, record := range records {
		if record.ObjectInstanceID == "objinst:ambiguous" {
			ambiguousRecord = record
			break
		}
	}
	if !ambiguousRecord.CandidatesTruncated || ambiguousRecord.CandidateCap != 1 {
		t.Fatalf("ambiguous record = %+v", ambiguousRecord)
	}
}

func TestBuildLoadsWorldAndExportsPrototypeEvidence(t *testing.T) {
	root := t.TempDir()
	writeObjectPrototypeFile(t, root, "o00", "item")
	writeRoomWithObject(t, root, "r00001", 1, objectTree("item"))
	if err := os.MkdirAll(filepath.Join(root, "player"), 0700); err != nil {
		t.Fatal(err)
	}

	got, err := Build(Options{
		Root:        root,
		GeneratedAt: time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Counts.EvidenceRecords != 1 || got.Counts.Resolved != 1 || got.Counts.SourceFiles != 1 {
		t.Fatalf("counts = %+v", got.Counts)
	}
	record := got.Evidence[0]
	if record.Resolution.Status != "resolved" ||
		record.Resolution.SelectedPrototypeID != "object:o00:0" ||
		record.Source.LegacyPath != "rooms/r00/r00001" ||
		record.Source.ObjectTreePath != "0" ||
		record.Source.FileSHA256 == "" {
		t.Fatalf("record = %+v", record)
	}
}

func TestWriteWritesJSONLAndManifest(t *testing.T) {
	outdir := t.TempDir()
	snapshot := Snapshot{
		Root:        "/tmp/muhan",
		GeneratedAt: time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC),
		Counts: Counts{
			EvidenceRecords: 1,
			FindingRecords:  1,
		},
		Evidence: []EvidenceRecord{{
			SchemaVersion:    SchemaVersion,
			ResolverVersion:  ResolverVersion,
			ObjectInstanceID: "objinst:1",
			PrototypeID:      "object:o00:1",
			Resolution: model.PrototypeResolutionMetadata{
				Status:              "resolved",
				SelectedPrototypeID: "object:o00:1",
			},
		}},
		Findings: []FindingRecord{{
			SchemaVersion: SchemaVersion,
			Severity:      "warning",
			Kind:          "map_room",
			Message:       "example",
		}},
	}

	manifest, err := Write(outdir, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 2 {
		t.Fatalf("files = %+v", manifest.Files)
	}
	if manifest.Files[0].SHA256 == "" || manifest.Files[1].SHA256 == "" {
		t.Fatalf("manifest files missing hashes: %+v", manifest.Files)
	}

	evidence := readJSONL[EvidenceRecord](t, filepath.Join(outdir, evidenceFileName))
	if len(evidence) != 1 || evidence[0].ObjectInstanceID != "objinst:1" {
		t.Fatalf("evidence = %+v", evidence)
	}
	var index Manifest
	readJSON(t, filepath.Join(outdir, indexFileName), &index)
	if index.SchemaVersion != SchemaVersion || index.Counts.EvidenceRecords != 1 {
		t.Fatalf("index = %+v", index)
	}
}

func TestSourceCacheResolvesDecodedLegacyPath(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "player", "bank")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	encoded, err := legacykr.EncodeEUCKR("가나")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, string(encoded)), []byte("bank bytes"))

	cache := sourceCache{root: root, files: map[string]SourceEvidence{}}
	got := cache.source(model.Metadata{LegacyPath: "player/bank/가나"})
	if got.Error != "" || got.FileSHA256 != sha256Hex([]byte("bank bytes")) {
		t.Fatalf("source = %+v", got)
	}
}

func objectInstance(id model.ObjectInstanceID, proto model.PrototypeID, status, path string) model.ObjectInstance {
	return model.ObjectInstance{
		ID:          id,
		PrototypeID: proto,
		Quantity:    1,
		Location:    model.ObjectLocation{RoomID: "room:00001"},
		Metadata: model.Metadata{
			LegacyKind:     "objectTreeObject",
			LegacyID:       string(id),
			LegacyPath:     path,
			LegacyEncoding: "euc-kr/cp949",
			ObjectTreePath: "0",
			PrototypeResolution: &model.PrototypeResolutionMetadata{
				Status:              status,
				Method:              "exact_record_without_pointers",
				Confidence:          "exact",
				SelectedPrototypeID: proto,
			},
		},
	}
}

func writeRoomWithObject(t *testing.T, root, name string, number int, object []byte) {
	t.Helper()
	dir := filepath.Join(root, "rooms", "r00")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, cbin.RoomSize)
	binary.LittleEndian.PutUint16(data, uint16(int16(number)))
	data = appendInt32(data, 0)
	data = appendInt32(data, 0)
	data = appendInt32(data, 1)
	data = append(data, object...)
	data = appendDescription(data, nil)
	data = appendDescription(data, nil)
	data = appendDescription(data, nil)
	writeFile(t, filepath.Join(dir, name), data)
}

func writeObjectPrototypeFile(t *testing.T, root, name, displayName string) {
	t.Helper()
	dir := filepath.Join(root, "objmon")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, cbin.ObjectSize)
	copy(data, []byte(displayName+"\x00"))
	writeFile(t, filepath.Join(dir, name), data)
}

func objectTree(name string) []byte {
	data := make([]byte, cbin.ObjectSize)
	copy(data, []byte(name+"\x00"))
	return appendInt32(data, 0)
}

func appendInt32(data []byte, n int) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(int32(n)))
	return append(data, buf[:]...)
}

func appendDescription(data []byte, desc []byte) []byte {
	data = appendInt32(data, len(desc))
	return append(data, desc...)
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func readJSONL[T any](t *testing.T, path string) []T {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	var records []T
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record T
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return records
}

func readJSON(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
