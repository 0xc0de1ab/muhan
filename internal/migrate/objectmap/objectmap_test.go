package objectmap

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"muhan/internal/migrate/protoresolve"
	"muhan/internal/persist/cbin"
	"muhan/internal/world/model"
)

const (
	testObjectNameOff        = 0
	testObjectDescriptionOff = 80
	testObjectKeyOff         = 160
	testObjectUseOutputOff   = 220
	testObjectWeightOff      = 304
	testObjectTypeOff        = 306
	testObjectFlagsOff       = 324
	testCString20            = 20
)

func TestMapObjectTreeNestedTwoLevels(t *testing.T) {
	data := objectTree(
		objectRecord{
			name:        "root",
			description: "root desc",
			keys:        [3]string{"open", "take"},
			useOutput:   "root use",
			weight:      17,
			flags:       [8]byte{0x80},
		},
		objectTree(
			objectRecord{name: "box"},
			objectTree(objectRecord{name: "gem"}),
		),
	)
	node, err := cbin.DecodeObjectTree(data, false)
	if err != nil {
		t.Fatal(err)
	}

	objects, warnings := MapObjectTree("objinst:test", model.ObjectLocation{RoomID: "room:00001"}, node)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v", warnings)
	}
	if len(objects) != 3 {
		t.Fatalf("objects len = %d, want 3", len(objects))
	}

	rootOffset := 0
	boxOffset := cbin.ObjectSize + 4
	gemOffset := boxOffset + cbin.ObjectSize + 4
	rootID := model.ObjectInstanceID(fmt.Sprintf("objinst:test:%08d", rootOffset))
	boxID := model.ObjectInstanceID(fmt.Sprintf("objinst:test:%08d", boxOffset))
	gemID := model.ObjectInstanceID(fmt.Sprintf("objinst:test:%08d", gemOffset))

	if objects[0].ID != rootID || objects[1].ID != boxID || objects[2].ID != gemID {
		t.Fatalf("ids = %q / %q / %q", objects[0].ID, objects[1].ID, objects[2].ID)
	}
	if objects[0].Location.RoomID != "room:00001" {
		t.Fatalf("root location = %+v", objects[0].Location)
	}
	if got := objects[0].Contents.ObjectIDs; len(got) != 1 || got[0] != boxID {
		t.Fatalf("root contents = %+v", got)
	}
	if objects[1].Location.ContainerID != rootID {
		t.Fatalf("box location = %+v", objects[1].Location)
	}
	if got := objects[1].Contents.ObjectIDs; len(got) != 1 || got[0] != gemID {
		t.Fatalf("box contents = %+v", got)
	}
	if objects[2].Location.ContainerID != boxID {
		t.Fatalf("gem location = %+v", objects[2].Location)
	}

	root := objects[0]
	if root.DisplayNameOverride != "root" {
		t.Fatalf("display override = %q", root.DisplayNameOverride)
	}
	if root.Properties["name"] != "root" || root.Properties["description"] != "root desc" ||
		root.Properties["key[0]"] != "open" || root.Properties["key[1]"] != "take" ||
		root.Properties["useOutput"] != "root use" || root.Properties["weight"] != "17" {
		t.Fatalf("properties = %+v", root.Properties)
	}
	if !hasTag(root.Metadata.Tags, "weightless") {
		t.Fatalf("tags = %+v, want weightless", root.Metadata.Tags)
	}
	if string(root.Metadata.RawFields["description"]) != "root desc" ||
		string(root.Metadata.RawFields["key[0]"]) != "open" ||
		string(root.Metadata.RawFields["useOutput"]) != "root use" ||
		binary.LittleEndian.Uint16(root.Metadata.RawFields["weight"]) != 17 ||
		string(root.Metadata.RawFields["flags"]) != string([]byte{0x80, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatalf("raw fields = %+v", root.Metadata.RawFields)
	}

	for _, object := range objects {
		if err := object.Validate(); err != nil {
			t.Fatalf("%s validate: %v", object.ID, err)
		}
	}
}

func TestMapObjectFilePropagatesDecodeWarnings(t *testing.T) {
	data := objectTree(objectRecord{nameRaw: []byte{0xff}})

	objects, warnings, err := MapObjectFile("objinst:warn", model.ObjectLocation{CreatureID: "creature:player:test"}, data, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 {
		t.Fatalf("objects len = %d, want 1", len(objects))
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %+v, want 1", warnings)
	}
	if !strings.Contains(warnings[0], "object.name decode failed") || !strings.Contains(warnings[0], "offset 0") {
		t.Fatalf("warning = %q", warnings[0])
	}
	if objects[0].DisplayNameOverride != "" {
		t.Fatalf("display override = %q, want empty for invalid text", objects[0].DisplayNameOverride)
	}
	if len(objects[0].Metadata.RawFields["name"]) != 1 || objects[0].Metadata.RawFields["name"][0] != 0xff {
		t.Fatalf("raw name = % X", objects[0].Metadata.RawFields["name"])
	}
}

func TestObjectFlagNamesMapsVisibilityAndContainerFlags(t *testing.T) {
	var flags [8]byte
	for _, bit := range []int{1, 2, 6, 17, 18} {
		flags[bit/8] |= 1 << uint(bit%8)
	}

	tags := objectFlagNames(flags)
	for _, want := range []string{"hidden", "invisible", "container", "noTake", "scenery"} {
		if !hasTag(tags, want) {
			t.Fatalf("tags = %+v, want %q", tags, want)
		}
	}
}

func TestMapObjectTreeExposesLegacyWandKindAndMagicFlags(t *testing.T) {
	data := objectTree(objectRecord{name: "wand", typ: 8, flags: flagsForBits(28, 31, 44)})
	node, err := cbin.DecodeObjectTree(data, false)
	if err != nil {
		t.Fatal(err)
	}

	objects, warnings := MapObjectTree("objinst:wand", model.ObjectLocation{RoomID: "room:00001"}, node)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v", warnings)
	}
	if len(objects) != 1 {
		t.Fatalf("objects len = %d, want 1", len(objects))
	}
	got := objects[0]
	if got.Properties["type"] != "8" || got.Properties["kind"] != string(model.ObjectKindWand) {
		t.Fatalf("properties = %+v", got.Properties)
	}
	for _, want := range []string{"damageDice", "classSelective", "specialItem"} {
		if !hasTag(got.Metadata.Tags, want) {
			t.Fatalf("tags = %+v, want %q", got.Metadata.Tags, want)
		}
	}
}

func TestMapObjectTreeWithPrototypeResolver(t *testing.T) {
	data := objectTree(objectRecord{name: "sword"})
	node, err := cbin.DecodeObjectTree(data, false)
	if err != nil {
		t.Fatal(err)
	}
	record, err := cbin.DecodeObjectRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	resolver := protoresolve.NewObjectResolver()
	resolver.Add(record, protoresolve.ObjectCandidate{PrototypeID: "object:o00:7"})

	result := MapObjectTreeWithOptions("objinst:test", model.ObjectLocation{RoomID: "room:00001"}, node, Options{PrototypeResolver: resolver})
	if len(result.Objects) != 1 {
		t.Fatalf("objects len = %d, want 1", len(result.Objects))
	}
	if result.Objects[0].PrototypeID != model.PrototypeID("object:o00:7") {
		t.Fatalf("prototype = %q", result.Objects[0].PrototypeID)
	}
	if result.PrototypeResolution.ResolvedExact != 1 || result.PrototypeResolution.Synthetic != 0 {
		t.Fatalf("prototype resolution = %+v", result.PrototypeResolution)
	}
	if !hasTag(result.Objects[0].Metadata.Tags, "prototype:resolved") {
		t.Fatalf("tags = %+v", result.Objects[0].Metadata.Tags)
	}
	evidence := result.Objects[0].Metadata.PrototypeResolution
	if evidence == nil || evidence.Status != "resolved" ||
		evidence.Method != protoresolve.ObjectFingerprintMethod ||
		evidence.Confidence != "exact" ||
		evidence.SelectedPrototypeID != "object:o00:7" ||
		evidence.CandidateCount != 1 ||
		len(evidence.Candidates) != 1 ||
		evidence.Candidates[0].PrototypeID != "object:o00:7" ||
		evidence.Fingerprint == "" ||
		evidence.FingerprintAlgorithm != protoresolve.ObjectFingerprintAlgorithm ||
		evidence.ComparableBytes != protoresolve.ObjectFingerprintComparableBytes {
		t.Fatalf("prototype evidence = %+v", evidence)
	}
}

func TestMapObjectTreeWithUnresolvedPrototypeEvidence(t *testing.T) {
	data := objectTree(objectRecord{name: "shield"})
	node, err := cbin.DecodeObjectTree(data, false)
	if err != nil {
		t.Fatal(err)
	}
	resolver := protoresolve.NewObjectResolver()
	resolver.Add(objectRecordFromTree(t, objectTree(objectRecord{name: "sword"})), protoresolve.ObjectCandidate{PrototypeID: "object:o00:1"})

	result := MapObjectTreeWithOptions("objinst:test", model.ObjectLocation{RoomID: "room:00001"}, node, Options{PrototypeResolver: resolver})
	if result.PrototypeResolution.ResolvedExact != 0 || result.PrototypeResolution.Synthetic != 1 || result.PrototypeResolution.AmbiguousSynthetic != 0 {
		t.Fatalf("prototype resolution = %+v", result.PrototypeResolution)
	}
	object := result.Objects[0]
	if object.PrototypeID != "object:objinst:test:00000000" {
		t.Fatalf("prototype = %q", object.PrototypeID)
	}
	evidence := object.Metadata.PrototypeResolution
	if evidence == nil || evidence.Status != "unresolved" ||
		evidence.Confidence != "fallback" ||
		evidence.SelectedPrototypeID != object.PrototypeID ||
		evidence.SyntheticPrototypeID != object.PrototypeID ||
		evidence.CandidateCount != 0 ||
		len(evidence.Candidates) != 0 ||
		evidence.Fingerprint == "" {
		t.Fatalf("prototype evidence = %+v", evidence)
	}
}

func TestMapObjectTreeWithAmbiguousPrototypeEvidence(t *testing.T) {
	data := objectTree(objectRecord{name: "coin"})
	node, err := cbin.DecodeObjectTree(data, false)
	if err != nil {
		t.Fatal(err)
	}
	record := objectRecordFromTree(t, data)
	resolver := protoresolve.NewObjectResolver()
	resolver.Add(record, protoresolve.ObjectCandidate{PrototypeID: "object:o01:0", Path: "objmon/o01", Index: 0})
	resolver.Add(record, protoresolve.ObjectCandidate{PrototypeID: "object:o00:0", Path: "objmon/o00", Index: 0})

	result := MapObjectTreeWithOptions("objinst:test", model.ObjectLocation{RoomID: "room:00001"}, node, Options{PrototypeResolver: resolver})
	if result.PrototypeResolution.ResolvedExact != 0 || result.PrototypeResolution.Synthetic != 1 || result.PrototypeResolution.AmbiguousSynthetic != 1 {
		t.Fatalf("prototype resolution = %+v", result.PrototypeResolution)
	}
	object := result.Objects[0]
	if !hasTag(object.Metadata.Tags, "prototype:ambiguous") || object.PrototypeID != "object:objinst:test:00000000" {
		t.Fatalf("object = %+v", object)
	}
	evidence := object.Metadata.PrototypeResolution
	if evidence == nil || evidence.Status != "ambiguous" ||
		evidence.Confidence != "ambiguous" ||
		evidence.CandidateCount != 2 ||
		len(evidence.Candidates) != 2 ||
		evidence.Candidates[0].PrototypeID != "object:o00:0" ||
		evidence.Candidates[1].PrototypeID != "object:o01:0" {
		t.Fatalf("prototype evidence = %+v", evidence)
	}
}

func TestMapObjectTreeWithoutResolverUsesStructuredSyntheticEvidence(t *testing.T) {
	data := objectTree(objectRecord{name: "bag"})
	node, err := cbin.DecodeObjectTree(data, false)
	if err != nil {
		t.Fatal(err)
	}

	result := MapObjectTreeWithOptions("objinst:test", model.ObjectLocation{RoomID: "room:00001"}, node, Options{})
	evidence := result.Objects[0].Metadata.PrototypeResolution
	if evidence == nil || evidence.Status != "synthetic" ||
		evidence.Method != "synthetic" ||
		evidence.Confidence != "fallback" ||
		evidence.SelectedPrototypeID != result.Objects[0].PrototypeID ||
		evidence.SyntheticPrototypeID != result.Objects[0].PrototypeID {
		t.Fatalf("prototype evidence = %+v", evidence)
	}
}

func TestMapObjectTreeWithSourcePathPreservesTreePath(t *testing.T) {
	data := objectTree(objectRecord{name: "root"}, objectTree(objectRecord{name: "child"}))
	node, err := cbin.DecodeObjectTree(data, false)
	if err != nil {
		t.Fatal(err)
	}

	result := MapObjectTreeWithOptions("objinst:test", model.ObjectLocation{RoomID: "room:00001"}, node, Options{SourcePath: "rooms/r00/r00001"})
	if len(result.Objects) != 2 {
		t.Fatalf("objects len = %d, want 2", len(result.Objects))
	}
	for _, object := range result.Objects {
		if object.Metadata.LegacyPath != "rooms/r00/r00001" {
			t.Fatalf("legacy path = %q", object.Metadata.LegacyPath)
		}
	}
	if !hasNote(result.Objects[0].Metadata.Notes, "objectTreePath=0") ||
		!hasNote(result.Objects[1].Metadata.Notes, "objectTreePath=0.0") {
		t.Fatalf("notes = root %+v child %+v", result.Objects[0].Metadata.Notes, result.Objects[1].Metadata.Notes)
	}
}

type objectRecord struct {
	name        string
	nameRaw     []byte
	description string
	keys        [3]string
	useOutput   string
	weight      int16
	typ         int8
	flags       [8]byte
}

func hasTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

func hasNote(notes []string, want string) bool {
	for _, note := range notes {
		if note == want {
			return true
		}
	}
	return false
}

func objectTree(record objectRecord, children ...[]byte) []byte {
	data := make([]byte, cbin.ObjectSize)
	if record.nameRaw != nil {
		copy(data[testObjectNameOff:], record.nameRaw)
	} else {
		putCString(data, testObjectNameOff, cbin.ObjectSize, record.name)
	}
	putCString(data, testObjectDescriptionOff, cbin.ObjectSize-testObjectDescriptionOff, record.description)
	for i, key := range record.keys {
		putCString(data, testObjectKeyOff+i*testCString20, testCString20, key)
	}
	putCString(data, testObjectUseOutputOff, cbin.ObjectSize-testObjectUseOutputOff, record.useOutput)
	binary.LittleEndian.PutUint16(data[testObjectWeightOff:], uint16(record.weight))
	data[testObjectTypeOff] = byte(record.typ)
	copy(data[testObjectFlagsOff:], record.flags[:])
	data = appendInt32(data, int32(len(children)))
	for _, child := range children {
		data = append(data, child...)
	}
	return data
}

func objectRecordFromTree(t *testing.T, data []byte) cbin.ObjectRecord {
	t.Helper()
	record, err := cbin.DecodeObjectRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func flagsForBits(bits ...int) [8]byte {
	var flags [8]byte
	for _, bit := range bits {
		flags[bit/8] |= 1 << uint(bit%8)
	}
	return flags
}

func putCString(data []byte, off, size int, value string) {
	if value == "" {
		return
	}
	if len(value) >= size {
		value = value[:size-1]
	}
	copy(data[off:off+size], []byte(value))
}

func appendInt32(data []byte, value int32) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(value))
	return append(data, buf[:]...)
}
