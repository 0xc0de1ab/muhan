package load

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"muhan/internal/persist/cbin"
	"muhan/internal/persist/legacykr"
	"muhan/internal/world/model"
)

func TestAddRoomReportsDuplicateID(t *testing.T) {
	world := NewWorld()
	room := model.Room{ID: "room:00001", DisplayName: "one"}

	if err := world.AddRoom(room); err != nil {
		t.Fatal(err)
	}
	err := world.AddRoom(room)
	if err == nil {
		t.Fatal("expected duplicate room id error")
	}
	if !strings.Contains(err.Error(), "duplicate room id") {
		t.Fatalf("error = %v, want duplicate room id", err)
	}
}

func TestValidateRefsReportsMissingReferences(t *testing.T) {
	world := NewWorld()
	mustAddRoom(t, world, model.Room{
		ID:          "room:00001",
		DisplayName: "one",
		Exits: []model.Exit{{
			Name:     "north",
			ToRoomID: "room:99999",
		}},
		CreatureIDs: []model.CreatureID{"creature:missing"},
		PlayerIDs:   []model.PlayerID{"player:missing"},
		Objects:     model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:missing"}},
	})
	mustAddPlayer(t, world, model.Player{
		ID:          "player:one",
		DisplayName: "one",
		CreatureID:  "creature:missing",
		RoomID:      "room:99998",
	})
	mustAddCreature(t, world, model.Creature{
		ID:          "creature:one",
		Kind:        model.CreatureKindNPC,
		DisplayName: "one",
		RoomID:      "room:99997",
		PlayerID:    "player:missing",
	})
	mustAddBank(t, world, model.BankAccount{
		ID:            "bank:player:missing",
		Kind:          "player",
		OwnerName:     "missing",
		OwnerPlayerID: "player:missing",
		Objects:       model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:bank:missing"}},
	})
	mustAddObject(t, world, model.ObjectInstance{
		ID:          "object:orphan-bank",
		PrototypeID: "object:prototype:missing",
		Location:    model.ObjectLocation{BankID: "bank:missing"},
	})

	report := world.ValidateRefs()
	if len(report.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", report.Errors)
	}
	for _, kind := range []string{"missing_room_ref", "missing_creature_ref", "missing_player_ref", "missing_object_ref", "missing_bank_ref", "missing_object_prototype_ref"} {
		if !hasFindingKind(report.Warnings, kind) {
			t.Fatalf("warnings = %+v, want kind %q", report.Warnings, kind)
		}
	}
}

func TestLoadRootMinimal(t *testing.T) {
	if os.Getenv("CI") != "" || testing.Short() {
		t.Skip("skipping slow real root parsing in CI or short mode")
	}
	root := t.TempDir()
	writeMinimalRoomFile(t, root, "r00000", 0)
	writePlayerFile(t, root, "가", "가나", 0)
	writeObjectPrototypeFile(t, root, "o00", "item")

	summary, err := LoadRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Errors) != 0 || len(summary.Warnings) != 0 {
		t.Fatalf("findings = warnings %+v errors %+v", summary.Warnings, summary.Errors)
	}
	if summary.Counts.Rooms != 1 || summary.Counts.Players != 1 ||
		summary.Counts.Creatures != 1 || summary.Counts.ObjectPrototypes != 1 {
		t.Fatalf("counts = %+v", summary.Counts)
	}
	player := summary.World.Players[model.PlayerID("가나")]
	if player.RoomID != model.RoomID("room:00000") {
		t.Fatalf("player room id = %q, want normalized room:00000", player.RoomID)
	}
}

func TestLoadRootIncludesMarriageInvites(t *testing.T) {
	if os.Getenv("CI") != "" || testing.Short() {
		t.Skip("skipping slow real root parsing in CI or short mode")
	}
	root := t.TempDir()
	writeMinimalRoomFile(t, root, "r00000", 0)
	writePlayerFile(t, root, "가", "가나", 0)
	writeObjectPrototypeFile(t, root, "o00", "item")
	dir := filepath.Join(root, "player", "invite")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "invite_84"), []byte("Ivan\nJudy\n0\nIgnored\n"), 0600); err != nil {
		t.Fatal(err)
	}

	summary, err := LoadRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Errors) != 0 || len(summary.Warnings) != 0 {
		t.Fatalf("findings = warnings %+v errors %+v", summary.Warnings, summary.Errors)
	}
	if summary.Counts.MarriageInviteFiles != 1 || summary.Counts.MarriageInviteNames != 2 {
		t.Fatalf("invite counts = %+v", summary.Counts)
	}
	names := summary.World.MarriageInvites[model.SpecialID(84)]
	if len(names) != 2 || names[0] != "Ivan" || names[1] != "Judy" {
		t.Fatalf("marriage invites = %+v", summary.World.MarriageInvites)
	}
}

func TestLoadRootIncludesFamilies(t *testing.T) {
	if os.Getenv("CI") != "" || testing.Short() {
		t.Skip("skipping slow real root parsing in CI or short mode")
	}
	root := t.TempDir()
	writeMinimalRoomFile(t, root, "r00000", 0)
	writePlayerFile(t, root, "가", "가나", 0)
	writeObjectPrototypeFile(t, root, "o00", "item")
	writeFamilyTextFile(t, root, "family_list",
		"0 관리파 지존마상 100\n"+
			"1 은형문 셀미 100\n"+
			"2 무영문 무영풍 250\n"+
			"16 패거리데이타\n"+
			"\n패거리번호, 패거리이름, 두목의 이름, 가입보조금의 순으로\n")
	writeFamilyTextFile(t, root, "family_member_1",
		"10 셀미\n"+
			"9 멋쟁이\n"+
			"0 은형문\n")
	writeFamilyTextFile(t, root, "family_member_2",
		"10 무영풍\n"+
			"0 무영문\n")

	summary, err := LoadRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Errors) != 0 || len(summary.Warnings) != 0 {
		t.Fatalf("findings = warnings %+v errors %+v", summary.Warnings, summary.Errors)
	}
	if summary.Counts.Families != 3 || summary.Counts.FamilyMemberFiles != 2 || summary.Counts.FamilyMembers != 3 {
		t.Fatalf("family counts = %+v", summary.Counts)
	}
	family := summary.World.Families[2]
	if family.DisplayName != "무영문" || family.BossName != "무영풍" ||
		family.JoinSubsidy != 250 || family.Slot != 2 {
		t.Fatalf("family = %+v", family)
	}
	if len(family.Members) != 1 || family.Members[0].DisplayName != "무영풍" ||
		family.Members[0].Class != 10 {
		t.Fatalf("family members = %+v", family.Members)
	}
	if admin := summary.World.Families[0]; admin.DisplayName != "관리파" || admin.BossName != "지존마상" {
		t.Fatalf("admin family = %+v", admin)
	}
}

func TestLoadRootIncludesRoomContents(t *testing.T) {
	if os.Getenv("CI") != "" || testing.Short() {
		t.Skip("skipping slow real root parsing in CI or short mode")
	}
	root := t.TempDir()
	writeRoomContentFile(t, root, "r00001", 1)
	writePlayerFile(t, root, "가", "가나", 1)
	writeObjectPrototypeFile(t, root, "o00", "item")

	summary, err := LoadRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Errors) != 0 || len(summary.Warnings) != 0 {
		t.Fatalf("findings = warnings %+v errors %+v", summary.Warnings, summary.Errors)
	}
	if summary.Counts.RoomCreatures != 1 || summary.Counts.RoomObjects != 3 ||
		summary.Counts.ObjectInstances != 3 || summary.Counts.Creatures != 2 {
		t.Fatalf("counts = %+v", summary.Counts)
	}
	room := summary.World.Rooms["room:00001"]
	if len(room.CreatureIDs) != 1 || len(room.Objects.ObjectIDs) != 1 {
		t.Fatalf("room contents = creatures %+v objects %+v", room.CreatureIDs, room.Objects)
	}
	creature := summary.World.Creatures[room.CreatureIDs[0]]
	if creature.RoomID != room.ID || len(creature.Inventory.ObjectIDs) != 1 {
		t.Fatalf("creature = %+v", creature)
	}
	object := summary.World.Objects[room.Objects.ObjectIDs[0]]
	if object.Location.RoomID != room.ID || len(object.Contents.ObjectIDs) != 1 {
		t.Fatalf("room object = %+v", object)
	}
}

func TestLoadRootIncludesPlayerAndBankObjects(t *testing.T) {
	if os.Getenv("CI") != "" || testing.Short() {
		t.Skip("skipping slow real root parsing in CI or short mode")
	}
	root := t.TempDir()
	writeMinimalRoomFile(t, root, "r00000", 0)
	writePlayerFileWithInventory(t, root, "가", "가나", 0, roomTestObject("bag", roomTestObject("gem")))
	writePlayerBankFile(t, root, "가나", roomTestObject("vault", roomTestObject("coin")))
	writeObjectPrototypeFile(t, root, "o00", "item")

	summary, err := LoadRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Errors) != 0 || len(summary.Warnings) != 0 {
		t.Fatalf("findings = warnings %+v errors %+v", summary.Warnings, summary.Errors)
	}
	if summary.Counts.PlayerObjects != 2 || summary.Counts.BankAccounts != 1 ||
		summary.Counts.BankObjects != 2 || summary.Counts.ObjectInstances != 4 {
		t.Fatalf("counts = %+v", summary.Counts)
	}
	if summary.Counts.PrototypeSynthetic != 4 ||
		summary.Counts.SyntheticObjectPrototypes != 4 ||
		summary.Counts.ObjectPrototypes != 5 {
		t.Fatalf("prototype counts = %+v", summary.Counts)
	}

	player := summary.World.Players["가나"]
	creature := summary.World.Creatures[player.CreatureID]
	if len(creature.Inventory.ObjectIDs) != 1 {
		t.Fatalf("player creature inventory = %+v", creature.Inventory)
	}
	playerObject := summary.World.Objects[creature.Inventory.ObjectIDs[0]]
	if playerObject.Location.CreatureID != creature.ID || len(playerObject.Contents.ObjectIDs) != 1 {
		t.Fatalf("player object = %+v", playerObject)
	}

	bank := summary.World.Banks["bank:player:가나"]
	if bank.OwnerPlayerID != player.ID || len(bank.Objects.ObjectIDs) != 1 {
		t.Fatalf("bank = %+v", bank)
	}
	bankObject := summary.World.Objects[bank.Objects.ObjectIDs[0]]
	if bankObject.Location.BankID != bank.ID || len(bankObject.Contents.ObjectIDs) != 1 {
		t.Fatalf("bank object = %+v", bankObject)
	}
	if _, ok := summary.World.ObjectPrototypes[playerObject.PrototypeID]; !ok {
		t.Fatalf("missing synthetic prototype for %q", playerObject.PrototypeID)
	}
	if proto := summary.World.ObjectPrototypes[playerObject.PrototypeID]; proto.DisplayName != "bag" {
		t.Fatalf("synthetic prototype = %+v", proto)
	}
	if proto := summary.World.ObjectPrototypes[playerObject.PrototypeID]; proto.Metadata.PrototypeResolution == nil ||
		proto.Metadata.PrototypeResolution.MaterializedFromObjectInstanceID != playerObject.ID ||
		proto.Metadata.PrototypeResolution.SelectedPrototypeID != playerObject.PrototypeID {
		t.Fatalf("synthetic prototype evidence = %+v", proto.Metadata.PrototypeResolution)
	}
}

func TestLoadRootIncludesJSONPlayers(t *testing.T) {
	if os.Getenv("CI") != "" || testing.Short() {
		t.Skip("skipping slow real root parsing in CI or short mode")
	}
	root := t.TempDir()
	writeMinimalRoomFile(t, root, "r00001", 1)
	writeObjectPrototypeFile(t, root, "o00", "item")

	jsonDir := filepath.Join(root, "player", "json")
	if err := os.MkdirAll(jsonDir, 0700); err != nil {
		t.Fatal(err)
	}

	jsonPayload := `{
  "player": {
    "id": "가나",
    "displayName": "가나",
    "creatureId": "creature:player:가나",
    "roomId": "r00001"
  },
  "creature": {
    "id": "creature:player:가나",
    "kind": "player",
    "displayName": "가나",
    "roomId": "r00001",
    "playerId": "가나"
  },
  "objects": [
    {
      "id": "object:player:가나:inventory:0",
      "prototypeId": "object:o00:0",
      "location": {
        "creatureId": "creature:player:가나",
        "slot": "inventory"
      }
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(jsonDir, "가나.json"), []byte(jsonPayload), 0600); err != nil {
		t.Fatal(err)
	}

	writePlayerFile(t, root, "가", "가나", 0)

	summary, err := LoadRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Errors) != 0 || len(summary.Warnings) != 0 {
		t.Fatalf("findings = warnings %+v errors %+v", summary.Warnings, summary.Errors)
	}

	if summary.Counts.Players != 1 {
		t.Fatalf("expected 1 player, got %d", summary.Counts.Players)
	}
	player, ok := summary.World.Players["가나"]
	if !ok {
		t.Fatalf("player '가나' not found in world")
	}
	if player.RoomID != "room:00001" {
		t.Fatalf("expected player room room:00001, got %q (skipping legacy failed)", player.RoomID)
	}

	if len(summary.World.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(summary.World.Objects))
	}
}

func TestLoadRootIncludesJSONRooms(t *testing.T) {
	if os.Getenv("CI") != "" || testing.Short() {
		t.Skip("skipping slow real root parsing in CI or short mode")
	}
	root := t.TempDir()
	writeMinimalRoomFile(t, root, "r00001", 1)
	writePlayerFile(t, root, "가", "가나", 1)
	writeObjectPrototypeFile(t, root, "o00", "item")

	jsonDir := filepath.Join(root, "rooms", "json")
	if err := os.MkdirAll(jsonDir, 0700); err != nil {
		t.Fatal(err)
	}

	jsonPayload := `{
  "roomId": "room:00001",
  "floorObjectIds": [
    "object:floor:1"
  ],
  "objects": [
    {
      "id": "object:floor:1",
      "prototypeId": "object:o00:0",
      "location": {
        "roomId": "room:00001"
      },
      "quantity": 1
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(jsonDir, "r00001.json"), []byte(jsonPayload), 0600); err != nil {
		t.Fatal(err)
	}

	summary, err := LoadRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Errors) != 0 || len(summary.Warnings) != 0 {
		t.Fatalf("findings = warnings %+v errors %+v", summary.Warnings, summary.Errors)
	}

	room, ok := summary.World.Rooms["room:00001"]
	if !ok {
		t.Fatalf("room room:00001 not found")
	}

	if len(room.Objects.ObjectIDs) != 1 || room.Objects.ObjectIDs[0] != "object:floor:1" {
		t.Fatalf("expected room floor objects to contain 'object:floor:1', got %+v", room.Objects.ObjectIDs)
	}

	obj, ok := summary.World.Objects["object:floor:1"]
	if !ok {
		t.Fatalf("object:floor:1 not found in world objects")
	}
	if obj.PrototypeID != "object:o00:0" {
		t.Fatalf("expected prototype object:o00:0, got %q", obj.PrototypeID)
	}
}

func mustAddRoom(t *testing.T, world *World, room model.Room) {
	t.Helper()
	if err := world.AddRoom(room); err != nil {
		t.Fatal(err)
	}
}

func mustAddPlayer(t *testing.T, world *World, player model.Player) {
	t.Helper()
	if err := world.AddPlayer(player); err != nil {
		t.Fatal(err)
	}
}

func mustAddCreature(t *testing.T, world *World, creature model.Creature) {
	t.Helper()
	if err := world.AddCreature(creature); err != nil {
		t.Fatal(err)
	}
}

func mustAddBank(t *testing.T, world *World, account model.BankAccount) {
	t.Helper()
	if err := world.AddBank(account); err != nil {
		t.Fatal(err)
	}
}

func mustAddObject(t *testing.T, world *World, object model.ObjectInstance) {
	t.Helper()
	if err := world.AddObjectInstance(object); err != nil {
		t.Fatal(err)
	}
}

func hasFindingKind(findings []Finding, kind string) bool {
	for _, finding := range findings {
		if finding.Kind == kind {
			return true
		}
	}
	return false
}

func writeMinimalRoomFile(t *testing.T, root, name string, number int) {
	t.Helper()
	dir := filepath.Join(root, "rooms", "r00")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, cbin.RoomSize)
	binary.LittleEndian.PutUint16(data, uint16(int16(number)))
	data = appendInt32(data, 0)
	data = appendInt32(data, 0)
	data = appendInt32(data, 0)
	data = appendDescription(data, nil)
	data = appendDescription(data, nil)
	data = appendDescription(data, nil)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func writeRoomContentFile(t *testing.T, root, name string, number int) {
	t.Helper()
	dir := filepath.Join(root, "rooms", "r00")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, cbin.RoomSize)
	binary.LittleEndian.PutUint16(data, uint16(int16(number)))
	copy(data[2:], []byte("content room\x00"))
	data = appendInt32(data, 0)
	data = appendInt32(data, 1)
	data = append(data, roomTestCreature("orc", roomTestObject("knife"))...)
	data = appendInt32(data, 1)
	data = append(data, roomTestObject("chest", roomTestObject("coin"))...)
	data = appendDescription(data, nil)
	data = appendDescription(data, nil)
	data = appendDescription(data, nil)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func roomTestCreature(name string, inventory ...[]byte) []byte {
	data := make([]byte, cbin.CreatureSize)
	copy(data[0:], []byte(name+"\x00"))
	data = appendInt32(data, len(inventory))
	for _, item := range inventory {
		data = append(data, item...)
	}
	return data
}

func roomTestObject(name string, children ...[]byte) []byte {
	data := make([]byte, cbin.ObjectSize)
	copy(data[0:], []byte(name+"\x00"))
	data = appendInt32(data, len(children))
	for _, child := range children {
		data = append(data, child...)
	}
	return data
}

func writePlayerFile(t *testing.T, root, shard, name string, roomNumber int) {
	t.Helper()
	dir := filepath.Join(root, "player", shard)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, cbin.CreatureSize+4)
	copyLegacyCString(t, data[0:80], name)
	binary.LittleEndian.PutUint16(data[458:], uint16(int16(roomNumber)))
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func writePlayerFileWithInventory(t *testing.T, root, shard, name string, roomNumber int, inventory ...[]byte) {
	t.Helper()
	dir := filepath.Join(root, "player", shard)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, cbin.CreatureSize)
	copyLegacyCString(t, data[0:80], name)
	binary.LittleEndian.PutUint16(data[458:], uint16(int16(roomNumber)))
	data = appendInt32(data, len(inventory))
	for _, item := range inventory {
		data = append(data, item...)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func writePlayerBankFile(t *testing.T, root, owner string, objectTree []byte) {
	t.Helper()
	dir := filepath.Join(root, "player", "bank")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	encodedOwner, err := legacykr.EncodeEUCKR(owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, string(encodedOwner)), objectTree, 0600); err != nil {
		t.Fatal(err)
	}
}

func writeFamilyTextFile(t *testing.T, root, name, text string) {
	t.Helper()
	dir := filepath.Join(root, "player", "family")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	encoded, err := legacykr.EncodeEUCKR(text)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), encoded, 0600); err != nil {
		t.Fatal(err)
	}
}

func writeObjectPrototypeFile(t *testing.T, root, name, displayName string) {
	t.Helper()
	dir := filepath.Join(root, "objmon")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, cbin.ObjectSize)
	copy(data, []byte(displayName))
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
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

func appendInt32(data []byte, n int) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(int32(n)))
	return append(data, buf[:]...)
}

func appendDescription(data []byte, desc []byte) []byte {
	data = appendInt32(data, len(desc))
	return append(data, desc...)
}
