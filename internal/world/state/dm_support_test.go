package state

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xc0de1ab/muhan/internal/persist/cbin"
	"github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

func TestPersistence_Smoke(t *testing.T) {
	loaded := load.NewWorld()
	pid := model.PlayerID("player:smoke")
	loaded.Players[pid] = model.Player{ID: pid}

	w := NewWorld(loaded)
	// Just ensure it doesn't panic and basic operations work
	if _, ok := w.Player(pid); !ok {
		t.Error("player not found after load")
	}
}

func TestLogFileAccessRejectsUnsafeNames(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.MkdirAll("log", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("log", "log"), []byte("general log"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("log", "log_fl"), []byte("failure log"), 0o600); err != nil {
		t.Fatal(err)
	}
	escapePath := filepath.Join(root, "escape")
	if err := os.WriteFile(escapePath, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}

	w := NewWorld(nil)
	content, err := w.ReadLogFile("log")
	if err != nil {
		t.Fatalf("ReadLogFile(log) error = %v", err)
	}
	if content != "general log" {
		t.Fatalf("ReadLogFile(log) = %q, want general log", content)
	}
	if err := w.DeleteLogFile("log_fl"); err != nil {
		t.Fatalf("DeleteLogFile(log_fl) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join("log", "log_fl")); !os.IsNotExist(err) {
		t.Fatalf("DeleteLogFile(log_fl) left file status err = %v, want not exist", err)
	}

	for _, name := range []string{
		"",
		".",
		"..",
		"../escape",
		"..\\escape",
		"/tmp/escape",
		"sub/log",
		"..\u2044escape",
		"..\u2215escape",
		"..\uff0fescape",
		"..\uff3cescape",
		"log name",
	} {
		if _, err := w.ReadLogFile(name); err == nil || !strings.Contains(err.Error(), "log file name") {
			t.Fatalf("ReadLogFile(%q) error = %v, want unsafe log file name rejection", name, err)
		}
		if err := w.DeleteLogFile(name); err == nil || !strings.Contains(err.Error(), "log file name") {
			t.Fatalf("DeleteLogFile(%q) error = %v, want unsafe log file name rejection", name, err)
		}
	}
	if data, err := os.ReadFile(escapePath); err != nil || string(data) != "outside" {
		t.Fatalf("unsafe log access touched escape file: data=%q err=%v", data, err)
	}
}

func TestUpdateRoomFlagClearsLegacyPropertyAndTagAliasesLikeC(t *testing.T) {
	loaded := load.NewWorld()
	loaded.Rooms["room:start"] = model.Room{
		ID: "room:start",
		Metadata: model.Metadata{Tags: []string{
			"RSHOPP",
			"otherTag",
		}},
		Properties: map[string]string{
			"RSHOPP": "1",
			"shoppe": "true",
			"other":  "true",
		},
	}

	w := NewWorld(loaded)
	if err := w.UpdateRoomFlag("room:start", 1, false); err != nil {
		t.Fatalf("UpdateRoomFlag() error = %v", err)
	}

	room, ok := w.Room("room:start")
	if !ok {
		t.Fatal("room:start missing after flag update")
	}
	if roomHasAnyFlag(room, "shoppe") {
		t.Fatalf("shoppe flag still active after clear: tags=%+v properties=%+v", room.Metadata.Tags, room.Properties)
	}
	if hasAnyNormalizedFlag(room.Metadata.Tags, "RSHOPP", "shoppe") {
		t.Fatalf("legacy shop tags still present after clear: %+v", room.Metadata.Tags)
	}
	if _, ok := room.Properties["RSHOPP"]; ok {
		t.Fatalf("RSHOPP property still present after clear: %+v", room.Properties)
	}
	if _, ok := room.Properties["shoppe"]; ok {
		t.Fatalf("shoppe property still present after clear: %+v", room.Properties)
	}
	if !hasAnyNormalizedFlag(room.Metadata.Tags, "otherTag") || room.Properties["other"] != "true" {
		t.Fatalf("unrelated room state changed: tags=%+v properties=%+v", room.Metadata.Tags, room.Properties)
	}
}

func TestSetExitFlagClearsLegacyAliasLikeC(t *testing.T) {
	loaded := load.NewWorld()
	loaded.Rooms["room:start"] = model.Room{
		ID: "room:start",
		Exits: []model.Exit{{
			Name:     "north",
			ToRoomID: "room:end",
			Flags:    []string{"XSECRT", "other"},
		}},
	}

	w := NewWorld(loaded)
	exit, err := w.SetExitFlag("room:start", "north", "secret", false)
	if err != nil {
		t.Fatalf("SetExitFlag() error = %v", err)
	}
	if exitHasAnyFlag(exit, "secret") {
		t.Fatalf("secret flag still active after clear: %+v", exit.Flags)
	}
	if hasAnyNormalizedFlag(exit.Flags, "XSECRT", "secret") {
		t.Fatalf("legacy secret flags still present after clear: %+v", exit.Flags)
	}
	if !hasAnyNormalizedFlag(exit.Flags, "other") {
		t.Fatalf("unrelated exit flag removed: %+v", exit.Flags)
	}

	room, ok := w.Room("room:start")
	if !ok {
		t.Fatal("room:start missing after flag update")
	}
	if exitHasAnyFlag(room.Exits[0], "secret") {
		t.Fatalf("stored exit secret flag still active after clear: %+v", room.Exits[0].Flags)
	}
}

func TestPurgeRoomRemovesTemporaryPermanentFloorObjects(t *testing.T) {
	loaded := load.NewWorld()
	loaded.Rooms["room:start"] = model.Room{
		ID: "room:start",
		Objects: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
			"object:normal",
			"object:temporary",
		}},
	}
	loaded.Objects["object:normal"] = model.ObjectInstance{
		ID:          "object:normal",
		PrototypeID: "proto:normal",
		Location:    model.ObjectLocation{RoomID: "room:start"},
	}
	loaded.Objects["object:temporary"] = model.ObjectInstance{
		ID:          "object:temporary",
		PrototypeID: "proto:temporary",
		Location:    model.ObjectLocation{RoomID: "room:start"},
		Metadata:    model.Metadata{Tags: []string{"OTEMPP"}},
	}

	w := NewWorld(loaded)
	if err := w.PurgeRoom("room:start"); err != nil {
		t.Fatalf("PurgeRoom() error = %v", err)
	}
	room, ok := w.Room("room:start")
	if !ok {
		t.Fatal("room:start missing after purge")
	}
	if len(room.Objects.ObjectIDs) != 0 {
		t.Fatalf("room objects = %+v, want empty", room.Objects.ObjectIDs)
	}
	if _, ok := w.Object("object:normal"); ok {
		t.Fatal("normal object survived purge")
	}
	if _, ok := w.Object("object:temporary"); ok {
		t.Fatal("OTEMPP object survived purge")
	}
}

func TestFindObjectByNameOrdinalLookupsKeepCreatureAndRoomCountsSeparate(t *testing.T) {
	loaded := load.NewWorld()
	loaded.Rooms["room:start"] = model.Room{
		ID: "room:start",
		Objects: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
			"object:room-sword-1",
			"object:room-sword-2",
		}},
	}
	loaded.Creatures["creature:alice"] = model.Creature{
		ID:     "creature:alice",
		RoomID: "room:start",
		Inventory: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
			"object:held-sword",
		}},
	}
	loaded.Objects["object:held-sword"] = model.ObjectInstance{
		ID:                  "object:held-sword",
		DisplayNameOverride: "sword",
		Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	}
	loaded.Objects["object:room-sword-1"] = model.ObjectInstance{
		ID:                  "object:room-sword-1",
		DisplayNameOverride: "sword",
		Location:            model.ObjectLocation{RoomID: "room:start"},
	}
	loaded.Objects["object:room-sword-2"] = model.ObjectInstance{
		ID:                  "object:room-sword-2",
		DisplayNameOverride: "sword",
		Location:            model.ObjectLocation{RoomID: "room:start"},
	}

	w := NewWorld(loaded)
	if _, found := w.FindObjectOnCreatureByName("creature:alice", "sword", 2); found {
		t.Fatal("FindObjectOnCreatureByName count 2 unexpectedly found room object")
	}
	obj, found := w.FindObjectInRoomByName("room:start", "sword", 2)
	if !found {
		t.Fatal("FindObjectInRoomByName count 2 did not find room object")
	}
	if obj.ID != "object:room-sword-2" {
		t.Fatalf("FindObjectInRoomByName count 2 = %q, want object:room-sword-2", obj.ID)
	}
}

func TestPlayerCharmedCreaturesReadsCasterCharmListTags(t *testing.T) {
	loaded := load.NewWorld()
	loaded.Players["player:alice"] = model.Player{
		ID:         "player:alice",
		CreatureID: "creature:alice",
	}
	loaded.Creatures["creature:alice"] = model.Creature{
		ID:          "creature:alice",
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		Metadata: model.Metadata{Tags: []string{
			"charm:늑대",
			"charmID:creature:tiger",
			"charm:늑대",
		}},
	}
	loaded.Creatures["creature:tiger"] = model.Creature{
		ID:          "creature:tiger",
		DisplayName: "호랑이",
	}

	w := NewWorld(loaded)
	got, err := w.PlayerCharmedCreatures("player:alice")
	if err != nil {
		t.Fatalf("PlayerCharmedCreatures() error = %v", err)
	}
	want := []string{"늑대", "호랑이"}
	if len(got) != len(want) {
		t.Fatalf("PlayerCharmedCreatures() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("PlayerCharmedCreatures() = %+v, want %+v", got, want)
		}
	}
}

func TestDestroyCreaturePrunesCharmListReferences(t *testing.T) {
	loaded := load.NewWorld()
	loaded.Players["player:alice"] = model.Player{
		ID:         "player:alice",
		CreatureID: "creature:alice",
	}
	loaded.Creatures["creature:alice"] = model.Creature{
		ID:          "creature:alice",
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		Metadata: model.Metadata{Tags: []string{
			"charm:호랑이",
			"charmID:creature:tiger",
		}},
	}
	loaded.Creatures["creature:tiger"] = model.Creature{
		ID:          "creature:tiger",
		DisplayName: "호랑이",
	}

	w := NewWorld(loaded)
	if err := w.DestroyCreature("creature:tiger"); err != nil {
		t.Fatalf("DestroyCreature() error = %v", err)
	}
	got, err := w.PlayerCharmedCreatures("player:alice")
	if err != nil {
		t.Fatalf("PlayerCharmedCreatures() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("PlayerCharmedCreatures() = %+v, want empty after target removal", got)
	}
}

func TestLoadRoomUsesLegacyShardPath(t *testing.T) {
	root := t.TempDir()
	writeStateTestLegacyRoomFile(t, root, 1, "loaded from shard")

	w := NewWorld(load.NewWorld())
	w.SetDBRoot(root)
	if err := w.LoadRoom("room:00001"); err != nil {
		t.Fatalf("LoadRoom() error = %v", err)
	}
	room, ok := w.Room("room:00001")
	if !ok {
		t.Fatal("room:00001 not loaded")
	}
	if room.DisplayName != "loaded from shard" {
		t.Fatalf("room display name = %q, want shard fixture", room.DisplayName)
	}
}

func TestResaveRoomUnloadedRoomIsLegacyNoop(t *testing.T) {
	root := t.TempDir()
	w := NewWorld(load.NewWorld())
	w.SetDBRoot(root)

	if err := w.ResaveRoom("room:00077"); err != nil {
		t.Fatalf("ResaveRoom(unloaded) error = %v, want nil legacy no-op", err)
	}
	if _, err := os.Stat(filepath.Join(root, "rooms", "json", "r00077.json")); !os.IsNotExist(err) {
		t.Fatalf("unloaded ResaveRoom sidecar stat err = %v, want not exist", err)
	}
}

func TestResaveRoomWritesLegacyJSONFileNameForLoadedRoom(t *testing.T) {
	root := t.TempDir()
	loaded := load.NewWorld()
	loaded.Rooms["room:00077"] = model.Room{
		ID:          "room:00077",
		DisplayName: "loaded room",
		Objects:     model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:floor"}},
	}
	loaded.Objects["object:floor"] = model.ObjectInstance{
		ID:       "object:floor",
		Location: model.ObjectLocation{RoomID: "room:00077"},
	}
	w := NewWorld(loaded)
	w.SetDBRoot(root)

	if err := w.ResaveRoom("room:00077"); err != nil {
		t.Fatalf("ResaveRoom(loaded) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "rooms", "json", "r00077.json")); err != nil {
		t.Fatalf("loaded ResaveRoom sidecar stat err = %v, want file", err)
	}
}

func TestResaveRoomPermOnlyExpandsPermanentObjectAliases(t *testing.T) {
	root := t.TempDir()
	loaded := load.NewWorld()
	loaded.Rooms["room:00077"] = model.Room{
		ID: "room:00077",
		Objects: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
			"object:literal",
			"object:canonical-tag",
			"object:canonical-property",
			"object:property-token",
			"object:prototype-property",
			"object:normal",
		}},
	}
	loaded.Objects["object:literal"] = model.ObjectInstance{
		ID:       "object:literal",
		Location: model.ObjectLocation{RoomID: "room:00077"},
		Metadata: model.Metadata{Tags: []string{"OTEMPP"}},
	}
	loaded.Objects["object:canonical-tag"] = model.ObjectInstance{
		ID:       "object:canonical-tag",
		Location: model.ObjectLocation{RoomID: "room:00077"},
		Metadata: model.Metadata{Tags: []string{"inventoryPermanent"}},
	}
	loaded.Objects["object:canonical-property"] = model.ObjectInstance{
		ID:         "object:canonical-property",
		Location:   model.ObjectLocation{RoomID: "room:00077"},
		Properties: map[string]string{"temporaryPermanent": "true"},
	}
	loaded.Objects["object:property-token"] = model.ObjectInstance{
		ID:         "object:property-token",
		Location:   model.ObjectLocation{RoomID: "room:00077"},
		Properties: map[string]string{"flags": "inventoryPermanent,scenery"},
	}
	loaded.ObjectPrototypes["prototype:permanent"] = model.ObjectPrototype{
		ID:         "prototype:permanent",
		Properties: map[string]string{"inventoryPermanent": "yes"},
	}
	loaded.Objects["object:prototype-property"] = model.ObjectInstance{
		ID:          "object:prototype-property",
		PrototypeID: "prototype:permanent",
		Location:    model.ObjectLocation{RoomID: "room:00077"},
	}
	loaded.Objects["object:normal"] = model.ObjectInstance{
		ID:       "object:normal",
		Location: model.ObjectLocation{RoomID: "room:00077"},
	}

	w := NewWorld(loaded)
	w.SetDBRoot(root)

	if err := w.ResaveRoomWithOptions("room:00077", true); err != nil {
		t.Fatalf("ResaveRoomWithOptions(permOnly) error = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "rooms", "json", "r00077.json"))
	if err != nil {
		t.Fatalf("read resave sidecar: %v", err)
	}
	var saved SavedRoomState
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("unmarshal resave sidecar: %v", err)
	}
	got := strings.Join(objectIDsToStrings(saved.FloorObjectIDs), ",")
	want := "object:literal,object:canonical-tag,object:canonical-property,object:property-token,object:prototype-property"
	if got != want {
		t.Fatalf("permOnly floor objects = %q, want %q", got, want)
	}
}

func TestReloadRoomUsesLegacyShardPathAndPreservesRuntimeListsWhenDiskRoomEmpty(t *testing.T) {
	root := t.TempDir()
	writeStateTestLegacyRoomFile(t, root, 1, "reloaded from shard")

	loaded := load.NewWorld()
	loaded.Rooms["room:00001"] = model.Room{
		ID:          "room:00001",
		DisplayName: "runtime room",
		PlayerIDs:   []model.PlayerID{"player:alice"},
		CreatureIDs: []model.CreatureID{"creature:guard"},
		Objects:     model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:floor"}},
	}
	loaded.Players["player:alice"] = model.Player{ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:00001"}
	loaded.Creatures["creature:alice"] = model.Creature{ID: "creature:alice", Kind: model.CreatureKindPlayer, PlayerID: "player:alice", RoomID: "room:00001"}
	loaded.Creatures["creature:guard"] = model.Creature{ID: "creature:guard", Kind: model.CreatureKindMonster, DisplayName: "guard", RoomID: "room:00001"}
	loaded.Objects["object:floor"] = model.ObjectInstance{ID: "object:floor", Location: model.ObjectLocation{RoomID: "room:00001"}}

	w := NewWorld(loaded)
	w.SetDBRoot(root)
	if err := w.ReloadRoom("room:00001"); err != nil {
		t.Fatalf("ReloadRoom() error = %v", err)
	}
	room, ok := w.Room("room:00001")
	if !ok {
		t.Fatal("room:00001 missing after reload")
	}
	if room.DisplayName != "reloaded from shard" {
		t.Fatalf("room display name = %q, want reloaded disk room", room.DisplayName)
	}
	if got := strings.Join(playerIDsToStrings(room.PlayerIDs), ","); got != "player:alice" {
		t.Fatalf("room players = %q, want preserved player:alice", got)
	}
	if got := strings.Join(creatureIDsToStrings(room.CreatureIDs), ","); got != "creature:alice,creature:guard" {
		t.Fatalf("room creatures = %q, want preserved player creature and guard in legacy name order", got)
	}
	if got := strings.Join(objectIDsToStrings(room.Objects.ObjectIDs), ","); got != "object:floor" {
		t.Fatalf("room objects = %q, want preserved object:floor", got)
	}
}

func TestReloadRoomUnloadedRoomIsLegacyNoop(t *testing.T) {
	w := NewWorld(load.NewWorld())
	if err := w.ReloadRoom("room:99999"); err != nil {
		t.Fatalf("ReloadRoom(unloaded) error = %v, want nil legacy no-op", err)
	}
}

func TestFlushCrtObjReloadsPrototypeCachesFromDBRoot(t *testing.T) {
	root := t.TempDir()
	writeFlushPrototypeObjectFile(t, root, "new object")
	writeFlushPrototypeCreatureFile(t, root, "new monster", 77)

	loaded := load.NewWorld()
	loaded.ObjectPrototypes["object:o00:0"] = model.ObjectPrototype{
		ID:          "object:o00:0",
		DisplayName: "old object",
		Metadata:    model.Metadata{LegacyKind: "objectPrototype"},
	}
	loaded.ObjectPrototypes["object:o01:0"] = model.ObjectPrototype{
		ID:          "object:o01:0",
		DisplayName: "stale object",
		Metadata:    model.Metadata{LegacyKind: "objectPrototype"},
	}
	loaded.ObjectPrototypes["object:synthetic"] = model.ObjectPrototype{
		ID:          "object:synthetic",
		DisplayName: "synthetic object",
		Metadata:    model.Metadata{LegacyKind: "syntheticObjectPrototype"},
	}
	loaded.Creatures["creature:m00:0"] = model.Creature{
		ID:          "creature:m00:0",
		DisplayName: "old monster",
		Metadata:    model.Metadata{LegacyKind: "creaturePrototype"},
	}
	loaded.Creatures["creature:m01:0"] = model.Creature{
		ID:          "creature:m01:0",
		DisplayName: "stale monster",
		Metadata:    model.Metadata{LegacyKind: "creaturePrototype"},
	}
	loaded.Creatures["creature:m00:0:clone:000001"] = model.Creature{
		ID:          "creature:m00:0:clone:000001",
		DisplayName: "active clone",
		Metadata:    model.Metadata{LegacyKind: "creaturePrototype"},
	}

	w := NewWorld(loaded)
	w.SetDBRoot(root)
	if err := w.FlushCrtObj(); err != nil {
		t.Fatalf("FlushCrtObj() error = %v", err)
	}

	objectProto, ok := w.ObjectPrototype("object:o00:0")
	if !ok || objectProto.DisplayName != "new object" {
		t.Fatalf("ObjectPrototype(object:o00:0) = %+v, %v; want refreshed new object", objectProto, ok)
	}
	if _, ok := w.ObjectPrototype("object:o01:0"); ok {
		t.Fatal("stale object prototype survived FlushCrtObj")
	}
	synthetic, ok := w.ObjectPrototype("object:synthetic")
	if !ok || synthetic.DisplayName != "synthetic object" {
		t.Fatalf("synthetic prototype = %+v, %v; want preserved", synthetic, ok)
	}

	creatureProto, ok := w.CreaturePrototype("creature:m00:0")
	if !ok || creatureProto.DisplayName != "new monster" || creatureProto.Level != 77 {
		t.Fatalf("CreaturePrototype(creature:m00:0) = %+v, %v; want refreshed new monster level 77", creatureProto, ok)
	}
	if _, ok := w.Creature("creature:m01:0"); ok {
		t.Fatal("stale creature prototype survived FlushCrtObj")
	}
	activeClone, ok := w.Creature("creature:m00:0:clone:000001")
	if !ok || activeClone.DisplayName != "active clone" {
		t.Fatalf("active spawned clone = %+v, %v; want preserved", activeClone, ok)
	}
}

func TestFlushCrtObjRejectsDecodeErrorsWithoutMutating(t *testing.T) {
	root := t.TempDir()
	objmon := filepath.Join(root, "objmon")
	if err := os.MkdirAll(objmon, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(objmon, "o00"), []byte{1}, 0600); err != nil {
		t.Fatal(err)
	}

	loaded := load.NewWorld()
	loaded.ObjectPrototypes["object:o00:0"] = model.ObjectPrototype{
		ID:          "object:o00:0",
		DisplayName: "old object",
		Metadata:    model.Metadata{LegacyKind: "objectPrototype"},
	}
	w := NewWorld(loaded)
	w.SetDBRoot(root)

	err := w.FlushCrtObj()
	if err == nil || !strings.Contains(err.Error(), "object record size") {
		t.Fatalf("FlushCrtObj() error = %v, want object record size error", err)
	}
	objectProto, ok := w.ObjectPrototype("object:o00:0")
	if !ok || objectProto.DisplayName != "old object" {
		t.Fatalf("ObjectPrototype after failed flush = %+v, %v; want unchanged old object", objectProto, ok)
	}
}

func TestLoadLockoutsParsesLegacyPairsAndWildcards(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, "log")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		t.Fatal(err)
	}
	lockoutPath := filepath.Join(logDir, "lockout")
	if err := os.WriteFile(lockoutPath, []byte("192.0.2.* -\n203.0.113.7 sitepass\norphan\n"), 0600); err != nil {
		t.Fatal(err)
	}

	w := NewWorld(load.NewWorld())
	w.SetDBRoot(root)
	if err := w.LoadLockouts(); err != nil {
		t.Fatalf("LoadLockouts() error = %v", err)
	}
	if got := w.Lockouts(); len(got) != 2 || got[0].Password != "" || got[1].Password != "sitepass" {
		t.Fatalf("Lockouts() = %+v, want deny + password entries", got)
	}
	mode, password := w.CheckLockout("192.0.2.44")
	if mode != LockoutDeny || password != "" {
		t.Fatalf("CheckLockout deny = %v, %q; want deny", mode, password)
	}
	mode, password = w.CheckLockout("203.0.113.7")
	if mode != LockoutPassword || password != "sitepass" {
		t.Fatalf("CheckLockout password = %v, %q; want password sitepass", mode, password)
	}
	mode, password = w.CheckLockout("198.51.100.10")
	if mode != LockoutAllow || password != "" {
		t.Fatalf("CheckLockout allow = %v, %q; want allow", mode, password)
	}

	if err := os.Remove(lockoutPath); err != nil {
		t.Fatal(err)
	}
	if err := w.LoadLockouts(); err != nil {
		t.Fatalf("LoadLockouts() after remove error = %v", err)
	}
	if got := w.Lockouts(); len(got) != 0 {
		t.Fatalf("Lockouts() after missing file = %+v, want empty", got)
	}
}

func TestFingerInvalidHostReturnsLegacyMessage(t *testing.T) {
	w := NewWorld(load.NewWorld())
	out, err := w.Finger("bad host name", "")
	if err != nil {
		t.Fatalf("Finger() error = %v", err)
	}
	if out != legacyFingerInvalidHostMessage {
		t.Fatalf("Finger() = %q, want invalid host message", out)
	}
}

func TestFingerConnectsToLegacyFingerServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	oldPort := legacyFingerPort
	oldDialTimeout := legacyFingerDialTimeout
	oldReadTimeout := legacyFingerReadTimeout
	oldLimit := legacyFingerOutputLimit
	legacyFingerPort = port
	legacyFingerDialTimeout = time.Second
	legacyFingerReadTimeout = time.Second
	legacyFingerOutputLimit = 1024
	defer func() {
		legacyFingerPort = oldPort
		legacyFingerDialTimeout = oldDialTimeout
		legacyFingerReadTimeout = oldReadTimeout
		legacyFingerOutputLimit = oldLimit
	}()

	requests := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			requests <- "accept error: " + err.Error()
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		buf := make([]byte, 64)
		n, err := conn.Read(buf)
		if err != nil {
			requests <- "read error: " + err.Error()
			return
		}
		requests <- string(buf[:n])
		_, _ = conn.Write([]byte("finger response\n"))
	}()

	w := NewWorld(load.NewWorld())
	out, err := w.Finger("127.0.0.1", "bob")
	if err != nil {
		t.Fatalf("Finger() error = %v", err)
	}
	if out != "finger response\n" {
		t.Fatalf("Finger() = %q, want remote response", out)
	}
	if got := <-requests; got != "bob\n\r\n\r" {
		t.Fatalf("finger request = %q, want legacy request", got)
	}
}

func TestWorldListRendersLegacyMonsterObjectAndRoomFilters(t *testing.T) {
	loaded := load.NewWorld()
	loaded.Creatures["creature:m01:5"] = model.Creature{
		ID:          "creature:m01:5",
		Kind:        model.CreatureKindMonster,
		DisplayName: "테스트몹",
		Level:       10,
		Stats: map[string]int{
			"class":        3,
			"alignment":    -20,
			"strength":     11,
			"dexterity":    12,
			"constitution": 13,
			"intelligence": 14,
			"piety":        15,
			"hpMax":        99,
			"armor":        7,
			"thaco":        8,
			"experience":   1234,
			"gold":         55,
			"nDice":        2,
			"sDice":        6,
			"pDice":        3,
			"carry[0]":     212,
		},
		Metadata: model.Metadata{
			LegacyKind: "creaturePrototype",
			RawFields: map[string][]byte{
				"flags":  {0x01},
				"spells": {0x04},
			},
		},
	}
	loaded.ObjectPrototypes["object:o02:12"] = model.ObjectPrototype{
		ID:          "object:o02:12",
		DisplayName: "테스트검",
		Properties: map[string]string{
			"value":       "12345",
			"weight":      "7",
			"type":        "5",
			"adjustment":  "1",
			"shotsMax":    "2",
			"nDice":       "1",
			"sDice":       "8",
			"pDice":       "2",
			"armor":       "3",
			"wearFlag":    "4",
			"magicPower":  "5",
			"questNumber": "9",
		},
		Metadata: model.Metadata{
			LegacyKind: "objectPrototype",
			RawFields:  map[string][]byte{"flags": {0x01}},
		},
	}
	loaded.Rooms["room:00123"] = model.Room{
		ID:          "room:00123",
		DisplayName: "테스트방",
		Properties: map[string]string{
			"random":          "105,0,0,0,0,0,0,0,0,0",
			"traffic":         "9",
			"perm_mon.1.misc": "106",
			"perm_obj.0.misc": "212",
		},
		Metadata: model.Metadata{
			LegacyKind: "room",
			RawFields:  map[string][]byte{"flags": {0x01}},
		},
	}

	w := NewWorld(loaded)
	monsterOut, err := w.List([]string{"m", "-r105:105", "-l10:10", "-d15:15", "-D15", "-o212", "-f1", "-S3", "-c3"})
	if err != nil {
		t.Fatalf("List monster error = %v", err)
	}
	if !strings.Contains(monsterOut, "105. 테스트몹") || !strings.Contains(monsterOut, "2d6 +3") {
		t.Fatalf("monster list output = %q, want legacy monster row", monsterOut)
	}
	monsterExcluded, err := w.List([]string{"m", "-r105:105", "-F1"})
	if err != nil {
		t.Fatalf("List monster excluded error = %v", err)
	}
	if strings.Contains(monsterExcluded, "테스트몹") {
		t.Fatalf("monster -F1 output = %q, want filtered out", monsterExcluded)
	}

	objectOut, err := w.List([]string{"o", "-r212:212", "-t5", "-w4", "-q", "-f1"})
	if err != nil {
		t.Fatalf("List object error = %v", err)
	}
	if !strings.Contains(objectOut, "212. 테스트검") || !strings.Contains(objectOut, "012345") || !strings.Contains(objectOut, "009") {
		t.Fatalf("object list output = %q, want legacy object row", objectOut)
	}

	roomOut, err := w.List([]string{"r", "-r123:123", "-m105", "-o212", "-f1"})
	if err != nil {
		t.Fatalf("List room error = %v", err)
	}
	if !strings.Contains(roomOut, "123. 테스트방") || !strings.Contains(roomOut, "105/000/000") || !strings.Contains(roomOut, "009%") {
		t.Fatalf("room list output = %q, want legacy room row", roomOut)
	}
	roomPermMonOut, err := w.List([]string{"r", "-r123:123", "-m106"})
	if err != nil {
		t.Fatalf("List room perm mon error = %v", err)
	}
	if !strings.Contains(roomPermMonOut, "테스트방") {
		t.Fatalf("room -m106 output = %q, want permanent monster match", roomPermMonOut)
	}
}

func TestWorldListInvalidArgumentsReturnLegacyHelp(t *testing.T) {
	w := NewWorld(load.NewWorld())
	out, err := w.List([]string{"m", "bad"})
	if err != nil {
		t.Fatalf("List invalid error = %v", err)
	}
	if !strings.Contains(out, "list <m|o|r> [options]") {
		t.Fatalf("List invalid output = %q, want help", out)
	}
}

func TestCreateRoomWritesLegacyEmptyRoomFile(t *testing.T) {
	root := t.TempDir()
	w := NewWorld(load.NewWorld())
	w.SetDBRoot(root)

	if err := w.CreateRoom("room:105"); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	room, ok := w.Room("room:00105")
	if !ok {
		t.Fatal("canonical room room:00105 was not added to runtime world")
	}
	if room.DisplayName != "Room #105" {
		t.Fatalf("display name = %q, want Room #105", room.DisplayName)
	}

	path := filepath.Join(root, "rooms", "r00", "r00105")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read created room file: %v", err)
	}
	record, stats, err := cbin.DecodeRoomFileRecord(data)
	if err != nil {
		t.Fatalf("DecodeRoomFileRecord() error = %v", err)
	}
	if record.Number != 105 || record.Name.Text != "Room #105" {
		t.Fatalf("record number/name = %d/%q, want 105/Room #105", record.Number, record.Name.Text)
	}
	if stats.Exits != 0 || stats.Creatures != 0 || stats.Objects != 0 || stats.Descriptions != 0 {
		t.Fatalf("created room stats = %+v, want empty room", stats)
	}

	summary, err := load.LoadRoot(root)
	if err != nil {
		t.Fatalf("LoadRoot() error = %v", err)
	}
	if loadedRoom, ok := summary.World.Rooms["room:00105"]; !ok || loadedRoom.DisplayName != "Room #105" {
		t.Fatalf("loaded room = %+v ok=%v, want canonical room from legacy file", loadedRoom, ok)
	}
}

func writeFlushPrototypeObjectFile(t *testing.T, root, name string) {
	t.Helper()
	objmon := filepath.Join(root, "objmon")
	if err := os.MkdirAll(objmon, 0700); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, cbin.ObjectSize)
	copy(data, []byte(name+"\x00"))
	if err := os.WriteFile(filepath.Join(objmon, "o00"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func writeFlushPrototypeCreatureFile(t *testing.T, root, name string, level byte) {
	t.Helper()
	objmon := filepath.Join(root, "objmon")
	if err := os.MkdirAll(objmon, 0700); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, cbin.CreatureSize)
	copy(data, []byte(name+"\x00"))
	data[318] = level
	binary.LittleEndian.PutUint16(data[332:], 100)
	binary.LittleEndian.PutUint16(data[334:], 100)
	if err := os.WriteFile(filepath.Join(objmon, "m00"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func writeStateTestLegacyRoomFile(t *testing.T, root string, number int, name string) {
	t.Helper()
	dir := filepath.Join(root, "rooms", fmt.Sprintf("r%02d", number/1000))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data := stateTestMinimalRoomData(number, name)
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("r%05d", number)), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func stateTestMinimalRoomData(number int, name string) []byte {
	data := make([]byte, cbin.RoomSize)
	binary.LittleEndian.PutUint16(data, uint16(int16(number)))
	copy(data[2:], []byte(name))
	data = appendStateTestInt32(data, 0)
	data = appendStateTestInt32(data, 0)
	data = appendStateTestInt32(data, 0)
	data = appendStateTestDescription(data, nil)
	data = appendStateTestDescription(data, nil)
	data = appendStateTestDescription(data, nil)
	return data
}

func appendStateTestDescription(data []byte, desc []byte) []byte {
	data = appendStateTestInt32(data, len(desc))
	return append(data, desc...)
}

func appendStateTestInt32(data []byte, n int) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(int32(n)))
	return append(data, buf[:]...)
}

func playerIDsToStrings(ids []model.PlayerID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

func creatureIDsToStrings(ids []model.CreatureID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

func objectIDsToStrings(ids []model.ObjectInstanceID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}
