package playermap

import (
	"encoding/binary"
	"path/filepath"
	"strings"
	"testing"

	"muhan/internal/persist/cbin"
	"muhan/internal/persist/legacykr"
	"muhan/internal/world/model"
)

func TestMapPlayerFileUTF8Filename(t *testing.T) {
	data := testCreatureFile(t, "가나다")
	data[creatureLevelOff] = 12
	binary.LittleEndian.PutUint16(data[creatureRoomNumberOff:], uint16(345))
	copyLegacyCString(t, data[80:160], "테스트 설명")
	copyLegacyCString(t, data[160:240], "안녕")

	got, err := MapPlayerFile(filepath.Join("player", "가", "가나다"), "가", data)
	if err != nil {
		t.Fatal(err)
	}
	if got.FilenameEncoding != EncodingUTF8 {
		t.Fatalf("filename encoding = %q, want %q", got.FilenameEncoding, EncodingUTF8)
	}
	if got.Player.ID != model.PlayerID("가나다") || got.Player.DisplayName != "가나다" {
		t.Fatalf("player = %+v", got.Player)
	}
	if got.Creature.DisplayName != "가나다" || got.Creature.Kind != model.CreatureKindPlayer {
		t.Fatalf("creature = %+v", got.Creature)
	}
	if got.Creature.Description != "테스트 설명" || got.Creature.Properties["legacyTalk"] != "안녕" {
		t.Fatalf("creature text = description %q properties %#v", got.Creature.Description, got.Creature.Properties)
	}
	if got.Creature.Level != 12 || got.Player.RoomID != model.RoomID("r00345") || got.Creature.RoomID != model.RoomID("r00345") {
		t.Fatalf("level/room = level %d player room %q creature room %q", got.Creature.Level, got.Player.RoomID, got.Creature.RoomID)
	}
	if got.InvalidName || got.RecordNameMismatch || len(got.Warnings) != 0 {
		t.Fatalf("unexpected warnings: invalid=%v mismatch=%v warnings=%v", got.InvalidName, got.RecordNameMismatch, got.Warnings)
	}
}

func TestMapPlayerFileFamilyAndMarriageStats(t *testing.T) {
	data := testCreatureFile(t, "가나")
	setLegacyCreatureFlag(data, legacyPFAMIL)
	setLegacyDailyMax(data, legacyDLMARRI, 12)
	setLegacyDailyMax(data, legacyDLEXPND, 5)

	got, err := MapPlayerFile(filepath.Join("player", "가", "가나"), "가", data)
	if err != nil {
		t.Fatal(err)
	}
	stats := got.Creature.Stats
	if stats["familyFlag"] != 1 || stats["familyID"] != 5 || stats["marriageID"] != 12 {
		t.Fatalf("family/marriage stats = %+v", stats)
	}
}

func TestMapPlayerFilePreservesWhoisStats(t *testing.T) {
	data := testCreatureFile(t, "가나")
	setLegacyCreatureFlag(data, legacyPINVIS)
	setLegacyCreatureFlag(data, legacyPDMINV)
	setLegacyCreatureFlag(data, legacyPMALES)
	setLegacyCreatureFlag(data, legacyPDINVI)
	setLegacyCreatureFlag(data, legacyPBLIND)
	setLegacyCreatureFlag(data, legacyPSILNC)
	setLegacyCreatureFlag(data, legacySUICD)
	setLegacyLasttimeInterval(data, legacyLTHOURS, 3*86400)

	got, err := MapPlayerFile(filepath.Join("player", "가", "가나"), "가", data)
	if err != nil {
		t.Fatal(err)
	}
	stats := got.Creature.Stats
	for _, key := range []string{"PINVIS", "PDMINV", "PMALES", "PDINVI", "PBLIND", "PSILNC", "SUICD"} {
		if stats[key] != 1 {
			t.Fatalf("stat %s = %d, want 1 in %+v", key, stats[key], stats)
		}
	}
	if stats["legacyHoursInterval"] != 3*86400 || stats["legacyAgeYears"] != 21 {
		t.Fatalf("age stats = interval %d age %d", stats["legacyHoursInterval"], stats["legacyAgeYears"])
	}
}

func TestMapPlayerFilePreservesLegacyPasswordHash(t *testing.T) {
	data := testCreatureFile(t, "가나")
	copy(data[240:255], []byte("WOCZU5Ja1Vg\x00"))

	got, err := MapPlayerFile(filepath.Join("player", "가", "가나"), "가", data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Creature.Properties["legacyPasswordHash"] != "WOCZU5Ja1Vg" {
		t.Fatalf("legacy password hash = %q", got.Creature.Properties["legacyPasswordHash"])
	}
	if string(got.Creature.Metadata.RawFields["creature.password"]) != "WOCZU5Ja1Vg" {
		t.Fatalf("raw password = %q", got.Creature.Metadata.RawFields["creature.password"])
	}
}

func TestMapPlayerFileEUCKRFilename(t *testing.T) {
	nameBytes, err := legacykr.EncodeEUCKR("무한")
	if err != nil {
		t.Fatal(err)
	}
	data := testCreatureFile(t, "무한")

	got, err := MapPlayerFile(filepath.Join("player", "마", string(nameBytes)), "마", data)
	if err != nil {
		t.Fatal(err)
	}
	if got.FilenameEncoding != EncodingLegacyKR {
		t.Fatalf("filename encoding = %q, want %q", got.FilenameEncoding, EncodingLegacyKR)
	}
	if got.Player.ID != model.PlayerID("무한") || got.Player.DisplayName != "무한" {
		t.Fatalf("player = %+v", got.Player)
	}
	if string(got.Player.Metadata.RawFields["filename"]) != string(nameBytes) {
		t.Fatalf("raw filename = % X, want % X", got.Player.Metadata.RawFields["filename"], nameBytes)
	}
	if got.InvalidName || got.RecordNameMismatch || len(got.Warnings) != 0 {
		t.Fatalf("unexpected warnings: invalid=%v mismatch=%v warnings=%v", got.InvalidName, got.RecordNameMismatch, got.Warnings)
	}
}

func TestMapPlayerFileRecordNameMismatchWarning(t *testing.T) {
	data := testCreatureFile(t, "다라")

	got, err := MapPlayerFile(filepath.Join("player", "가", "가나"), "가", data)
	if err != nil {
		t.Fatal(err)
	}
	if !got.RecordNameMismatch {
		t.Fatal("RecordNameMismatch = false, want true")
	}
	if !hasWarning(got.Warnings, "creature record name") || !hasWarning(got.Warnings, "does not match filename") {
		t.Fatalf("warnings = %v", got.Warnings)
	}
	if got.Player.DisplayName != "가나" || got.Creature.Properties["legacyRecordName"] != "다라" {
		t.Fatalf("player/record name = %q / %#v", got.Player.DisplayName, got.Creature.Properties)
	}
}

func TestMapPlayerFileInventoryObjects(t *testing.T) {
	data := testCreatureFile(t, "가나")
	binary.LittleEndian.PutUint32(data[cbin.CreatureSize:], 1)
	data = append(data, testObjectTree(t, "가방", testObjectTree(t, "보석"))...)

	got, err := MapPlayerFileWithOptions(filepath.Join("player", "가", "가나"), "가", data, Options{IncludeObjects: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Decoded.Objects != 2 || len(got.Objects) != 2 {
		t.Fatalf("objects = decoded %+v len %d", got.Decoded, len(got.Objects))
	}
	if len(got.Creature.Inventory.ObjectIDs) != 1 {
		t.Fatalf("inventory = %+v", got.Creature.Inventory)
	}
	rootID := got.Creature.Inventory.ObjectIDs[0]
	if got.Objects[0].ID != rootID || got.Objects[0].Location.CreatureID != got.Creature.ID {
		t.Fatalf("root object = %+v, want id %q creature %q", got.Objects[0], rootID, got.Creature.ID)
	}
	if got.Objects[1].Location.ContainerID != rootID {
		t.Fatalf("child object = %+v, want container %q", got.Objects[1], rootID)
	}
}

func testCreatureFile(t *testing.T, recordName string) []byte {
	t.Helper()
	data := make([]byte, cbin.CreatureSize+4)
	copyLegacyCString(t, data[0:80], recordName)
	return data
}

func testObjectTree(t *testing.T, name string, children ...[]byte) []byte {
	t.Helper()
	data := make([]byte, cbin.ObjectSize)
	copyLegacyCString(t, data[0:80], name)
	data = appendInt32(data, len(children))
	for _, child := range children {
		data = append(data, child...)
	}
	return data
}

func appendInt32(data []byte, n int) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(int32(n)))
	return append(data, buf[:]...)
}

func setLegacyCreatureFlag(data []byte, bit int) {
	const testCreatureFlagsOff = 412
	data[testCreatureFlagsOff+bit/8] |= 1 << uint(bit%8)
}

func setLegacyDailyMax(data []byte, index int, value byte) {
	const testCreatureDailyOff = 540
	data[testCreatureDailyOff+index*cbin.DailySize] = value
}

func setLegacyLasttimeInterval(data []byte, index int, value int) {
	const testCreatureLasttimeOff = 620
	binary.LittleEndian.PutUint32(data[testCreatureLasttimeOff+index*cbin.LasttimeSize:], uint32(int32(value)))
}

func copyLegacyCString(t *testing.T, dst []byte, text string) {
	t.Helper()
	encoded, err := legacykr.EncodeEUCKR(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded)+1 > len(dst) {
		t.Fatalf("encoded text %q is too long for %d-byte cstring", text, len(dst))
	}
	copy(dst, encoded)
}

func hasWarning(warnings []Finding, fragment string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning.Message, fragment) {
			return true
		}
	}
	return false
}
