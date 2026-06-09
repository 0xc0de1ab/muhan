package protomap

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"muhan/internal/persist/cbin"
	"muhan/internal/world/model"
)

const (
	testObjectTypeOff      = 306
	testObjectFlagsOff     = 324
	testCreatureLevelOff   = 318
	testCreatureHPMaxOff   = 332
	testCreatureHPCurOff   = 334
	testCreatureSpecialOff = 358
	testCreatureFlagsOff   = 412
	testCreatureCarryOff   = 438
)

func TestBuildMapsTempObjectFileAndEUCKRName(t *testing.T) {
	root := t.TempDir()
	objmon := filepath.Join(root, "objmon")
	if err := os.MkdirAll(objmon, 0700); err != nil {
		t.Fatal(err)
	}

	data := make([]byte, cbin.ObjectSize)
	copy(data, []byte{0xB0, 0xA1, 0}) // "가"
	if err := os.WriteFile(filepath.Join(objmon, "o00"), data, 0600); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Build(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	if snapshot.Counts.ObjectPrototypeFiles != 1 || snapshot.Counts.ObjectPrototypes != 1 ||
		snapshot.Counts.TotalPrototypes != 1 {
		t.Fatalf("counts = %+v", snapshot.Counts)
	}
	if len(snapshot.Errors) != 0 || len(snapshot.Warnings) != 0 {
		t.Fatalf("findings = warnings %+v errors %+v", snapshot.Warnings, snapshot.Errors)
	}
	if len(snapshot.ObjectPrototypes) != 1 {
		t.Fatalf("object prototypes len = %d, want 1", len(snapshot.ObjectPrototypes))
	}

	got := snapshot.ObjectPrototypes[0]
	if got.ID != model.PrototypeID("object:o00:0") {
		t.Fatalf("id = %q, want object:o00:0", got.ID)
	}
	if got.DisplayName != "가" {
		t.Fatalf("displayName = %q, want 가", got.DisplayName)
	}
	if got.Metadata.LegacyPath != "objmon/o00" || got.Metadata.RecordIndex != 0 ||
		got.Metadata.RecordOffset != 0 {
		t.Fatalf("metadata = %+v", got.Metadata)
	}
	if string(got.Metadata.RawFields["name"]) != string([]byte{0xB0, 0xA1}) {
		t.Fatalf("raw name = % X", got.Metadata.RawFields["name"])
	}
}

func TestBuildMapsLegacyWandTypeAndMagicFlags(t *testing.T) {
	root := t.TempDir()
	objmon := filepath.Join(root, "objmon")
	if err := os.MkdirAll(objmon, 0700); err != nil {
		t.Fatal(err)
	}

	data := make([]byte, cbin.ObjectSize)
	copy(data, []byte("wand\x00"))
	data[testObjectTypeOff] = 8
	for _, bit := range []int{28, 31, 44} {
		data[testObjectFlagsOff+bit/8] |= 1 << uint(bit%8)
	}
	if err := os.WriteFile(filepath.Join(objmon, "o00"), data, 0600); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Build(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.ObjectPrototypes) != 1 {
		t.Fatalf("object prototypes len = %d, want 1", len(snapshot.ObjectPrototypes))
	}
	got := snapshot.ObjectPrototypes[0]
	if got.Kind != model.ObjectKindWand {
		t.Fatalf("kind = %q, want %q", got.Kind, model.ObjectKindWand)
	}
	for _, want := range []string{"damageDice", "classSelective", "specialItem"} {
		if !hasTag(got.Metadata.Tags, want) {
			t.Fatalf("tags = %+v, want %q", got.Metadata.Tags, want)
		}
	}
}

func TestBuildMapsCreaturePrototype(t *testing.T) {
	root := t.TempDir()
	objmon := filepath.Join(root, "objmon")
	if err := os.MkdirAll(objmon, 0700); err != nil {
		t.Fatal(err)
	}

	data := make([]byte, cbin.CreatureSize)
	copy(data, []byte("monster\x00"))
	data[testCreatureLevelOff] = 80
	binary.LittleEndian.PutUint16(data[testCreatureHPMaxOff:], 1234)
	binary.LittleEndian.PutUint16(data[testCreatureHPCurOff:], 1200)
	binary.LittleEndian.PutUint16(data[testCreatureSpecialOff:], 99)
	binary.LittleEndian.PutUint16(data[testCreatureCarryOff:], 212)
	for _, bit := range []int{0, 41, 61} {
		data[testCreatureFlagsOff+bit/8] |= 1 << uint(bit%8)
	}
	if err := os.WriteFile(filepath.Join(objmon, "m00"), data, 0600); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Build(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	if snapshot.Counts.CreaturePrototypeFiles != 1 || snapshot.Counts.CreaturePrototypes != 1 ||
		snapshot.Counts.TotalPrototypes != 1 {
		t.Fatalf("counts = %+v", snapshot.Counts)
	}
	if len(snapshot.CreaturePrototypes) != 1 {
		t.Fatalf("creature prototypes len = %d, want 1", len(snapshot.CreaturePrototypes))
	}
	got := snapshot.CreaturePrototypes[0]
	if got.ID != model.CreatureID("creature:m00:0") || got.Kind != model.CreatureKindMonster {
		t.Fatalf("creature prototype = %+v", got)
	}
	if got.DisplayName != "monster" {
		t.Fatalf("displayName = %q, want monster", got.DisplayName)
	}
	if got.Level != 80 || got.Stats["hpMax"] != 1234 || got.Stats["hpCurrent"] != 1200 ||
		got.Stats["special"] != 99 || got.Stats["carry[0]"] != 212 {
		t.Fatalf("creature numeric fields = level %d stats %+v", got.Level, got.Stats)
	}
	for _, want := range []string{"MPERMT", "permanent", "MDEATH", "deathDescription", "MSUMMO", "summoner"} {
		if !hasTag(got.Metadata.Tags, want) {
			t.Fatalf("tags = %+v, want %q", got.Metadata.Tags, want)
		}
	}
	if len(got.Metadata.RawFields["flags"]) != 8 {
		t.Fatalf("raw flags = % X, want 8 bytes", got.Metadata.RawFields["flags"])
	}
}

func TestBuildWarnsOnTextDecodeErrorAndContinues(t *testing.T) {
	root := t.TempDir()
	objmon := filepath.Join(root, "objmon")
	if err := os.MkdirAll(objmon, 0700); err != nil {
		t.Fatal(err)
	}

	data := make([]byte, cbin.ObjectSize)
	copy(data, []byte{0xff, 0}) // invalid CP949
	if err := os.WriteFile(filepath.Join(objmon, "o00"), data, 0600); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Build(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	if len(snapshot.ObjectPrototypes) != 1 {
		t.Fatalf("object prototypes len = %d, want 1", len(snapshot.ObjectPrototypes))
	}
	if snapshot.ObjectPrototypes[0].DisplayName != "object:o00:0" {
		t.Fatalf("displayName fallback = %q", snapshot.ObjectPrototypes[0].DisplayName)
	}
	if len(snapshot.Warnings) != 1 {
		t.Fatalf("warnings = %+v, want 1", snapshot.Warnings)
	}
	if !strings.Contains(snapshot.Warnings[0].Message, "object.name decode failed") {
		t.Fatalf("warning = %+v", snapshot.Warnings[0])
	}
}

func TestBuildReportsInvalidPrototypeSize(t *testing.T) {
	root := t.TempDir()
	objmon := filepath.Join(root, "objmon")
	if err := os.MkdirAll(objmon, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(objmon, "o00"), []byte{1}, 0600); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Build(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	if len(snapshot.ObjectPrototypes) != 0 {
		t.Fatalf("object prototypes len = %d, want 0", len(snapshot.ObjectPrototypes))
	}
	if len(snapshot.Errors) != 1 {
		t.Fatalf("errors = %+v, want 1", snapshot.Errors)
	}
	if !strings.Contains(snapshot.Errors[0].Message, "not a multiple of object record size") {
		t.Fatalf("error = %+v", snapshot.Errors[0])
	}
}

func hasTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}
