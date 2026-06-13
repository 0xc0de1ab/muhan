package game

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/session"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

type mockPermanentDeathWorld struct {
	activeSessions []ActiveSession
	players        map[model.PlayerID]model.Player
	creatures      map[model.CreatureID]model.Creature
	rooms          map[model.RoomID]model.Room

	dbRoot         string
	writtenTexts   map[session.ID][]string
	broadcastRooms map[model.RoomID][]string
	spawns         []permanentDeathSpawnCall
	events         []PermanentCreatureDeathEvent
}

type permanentDeathSpawnCall struct {
	protoID    model.CreatureID
	roomID     model.RoomID
	carryItems bool
}

func newMockPermanentDeathWorld() *mockPermanentDeathWorld {
	return &mockPermanentDeathWorld{
		players:        map[model.PlayerID]model.Player{},
		creatures:      map[model.CreatureID]model.Creature{},
		rooms:          map[model.RoomID]model.Room{},
		writtenTexts:   map[session.ID][]string{},
		broadcastRooms: map[model.RoomID][]string{},
	}
}

func (m *mockPermanentDeathWorld) ActiveSessions() []ActiveSession {
	return append([]ActiveSession(nil), m.activeSessions...)
}

func (m *mockPermanentDeathWorld) Player(playerID model.PlayerID) (model.Player, bool) {
	player, ok := m.players[playerID]
	return player, ok
}

func (m *mockPermanentDeathWorld) Creature(creatureID model.CreatureID) (model.Creature, bool) {
	creature, ok := m.creatures[creatureID]
	return creature, ok
}

func (m *mockPermanentDeathWorld) Room(roomID model.RoomID) (model.Room, bool) {
	room, ok := m.rooms[roomID]
	return room, ok
}

func (m *mockPermanentDeathWorld) DBRoot() string {
	return m.dbRoot
}

func (m *mockPermanentDeathWorld) BroadcastRoom(roomID model.RoomID, excludeSessionID session.ID, text string) error {
	m.broadcastRooms[roomID] = append(m.broadcastRooms[roomID], text)
	return nil
}

func (m *mockPermanentDeathWorld) WriteToSession(sessionID session.ID, text string, isPrompt bool) error {
	m.writtenTexts[sessionID] = append(m.writtenTexts[sessionID], text)
	return nil
}

func (m *mockPermanentDeathWorld) SetCreatureStat(creatureID model.CreatureID, name string, val int) error {
	creature, ok := m.creatures[creatureID]
	if !ok {
		return fmt.Errorf("creature %s not found", creatureID)
	}
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats[name] = val
	m.creatures[creatureID] = creature
	return nil
}

func (m *mockPermanentDeathWorld) SetCreatureProperty(creatureID model.CreatureID, key string, value string) (model.Creature, error) {
	creature, ok := m.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("creature %s not found", creatureID)
	}
	if creature.Properties == nil {
		creature.Properties = map[string]string{}
	}
	if value == "" {
		delete(creature.Properties, key)
	} else {
		creature.Properties[key] = value
	}
	m.creatures[creatureID] = creature
	return creature, nil
}

func (m *mockPermanentDeathWorld) UpdateRoomProperty(roomID model.RoomID, key string, value string) error {
	room, ok := m.rooms[roomID]
	if !ok {
		return fmt.Errorf("room %s not found", roomID)
	}
	if room.Properties == nil {
		room.Properties = map[string]string{}
	}
	if value == "" {
		delete(room.Properties, key)
	} else {
		room.Properties[key] = value
	}
	m.rooms[roomID] = room
	return nil
}

func (m *mockPermanentDeathWorld) SpawnCreature(protoID model.CreatureID, roomID model.RoomID, carryItems bool) (model.CreatureID, error) {
	m.spawns = append(m.spawns, permanentDeathSpawnCall{protoID: protoID, roomID: roomID, carryItems: carryItems})
	id := model.CreatureID(fmt.Sprintf("%s:clone:%d", protoID, len(m.spawns)))
	m.creatures[id] = model.Creature{ID: id, Kind: model.CreatureKindMonster, RoomID: roomID}
	return id, nil
}

func (m *mockPermanentDeathWorld) RecordPermanentCreatureDeath(event PermanentCreatureDeathEvent) error {
	m.events = append(m.events, event)
	return nil
}

type statePermanentDeathWorld struct {
	*state.World
	activeSessions []ActiveSession
	writtenTexts   map[session.ID][]string
	broadcastRooms map[model.RoomID][]string
}

func newStatePermanentDeathWorld(runtime *state.World) *statePermanentDeathWorld {
	return &statePermanentDeathWorld{
		World:          runtime,
		writtenTexts:   map[session.ID][]string{},
		broadcastRooms: map[model.RoomID][]string{},
	}
}

func (w *statePermanentDeathWorld) ActiveSessions() []ActiveSession {
	return append([]ActiveSession(nil), w.activeSessions...)
}

func (w *statePermanentDeathWorld) BroadcastRoom(roomID model.RoomID, excludeSessionID session.ID, text string) error {
	w.broadcastRooms[roomID] = append(w.broadcastRooms[roomID], text)
	return nil
}

func (w *statePermanentDeathWorld) WriteToSession(sessionID session.ID, text string, isPrompt bool) error {
	w.writtenTexts[sessionID] = append(w.writtenTexts[sessionID], text)
	return nil
}

func TestHandlePermanentCreatureDeathAppliesLegacySideEffects(t *testing.T) {
	w := newMockPermanentDeathWorld()
	w.activeSessions = []ActiveSession{{ID: "s1", ActorID: "player:alice"}}
	w.players["player:alice"] = model.Player{ID: "player:alice", CreatureID: "creature:alice"}
	w.creatures["creature:alice"] = model.Creature{
		ID:          "creature:alice",
		DisplayName: "앨리스",
		Stats:       map[string]int{"experience": 1000},
		Properties:  map[string]string{"proficiency/sharp": "9"},
	}
	w.rooms["room:1"] = model.Room{
		ID: "room:1",
		Properties: map[string]string{
			"perm_mon.0.name":     "혈귀",
			"perm_mon.0.ltime":    "900",
			"perm_mon.0.interval": "30",
		},
	}
	w.creatures["creature:dead"] = model.Creature{
		ID:          "creature:dead",
		Kind:        model.CreatureKindMonster,
		DisplayName: "혈귀",
		RoomID:      "room:1",
		Stats:       map[string]int{"questNumber": 2, "special": 77},
		Properties: map[string]string{
			"deathDescriptionText": "혈귀가 검은 연기로 흩어집니다.",
			"summonPrototypeID":    "creature:spawn",
		},
		Metadata: model.Metadata{Tags: []string{"permanent", "deathDescription", "summoner"}},
	}

	result, err := HandlePermanentCreatureDeath(w, "player:alice", "creature:dead", 1000)
	if err != nil {
		t.Fatalf("HandlePermanentCreatureDeath() error = %v", err)
	}

	if !result.Permanent || !result.RespawnMarked || !result.DeathDescriptionBroadcast {
		t.Fatalf("result flags = %+v, want permanent respawn/deathDescription", result)
	}
	if !result.QuestCompleted || result.QuestExperience != getQuestExpLocal(1) {
		t.Fatalf("quest result = %+v, want completed with quest 2 exp", result)
	}
	if !result.SummonRequested || result.SummonedCreatureID != "creature:spawn:clone:1" {
		t.Fatalf("summon result = %+v", result)
	}
	if !result.PermanentDeathEventHookCalled || len(w.events) != 1 {
		t.Fatalf("events = %+v, hookCalled=%v", w.events, result.PermanentDeathEventHookCalled)
	}

	if got := w.rooms["room:1"].Properties["perm_mon.0.ltime"]; got != "1000" {
		t.Fatalf("perm_mon.0.ltime = %q, want 1000", got)
	}
	if got := strings.Join(w.broadcastRooms["room:1"], "\n"); !strings.Contains(got, "혈귀가 검은 연기로 흩어집니다.") {
		t.Fatalf("death broadcasts = %q", got)
	}
	if got := strings.Join(w.writtenTexts["s1"], ""); !strings.Contains(got, "임무를 달성") || !strings.Contains(got, "500") {
		t.Fatalf("player messages = %q, want quest completion and exp", got)
	}
	if got := w.creatures["creature:alice"].Properties[questCompletionKey(2)]; got != "1" {
		t.Fatalf("quest flag = %q, want 1", got)
	}
	if got := w.creatures["creature:alice"].Stats["experience"]; got != 1500 {
		t.Fatalf("experience = %d, want 1500", got)
	}
	if got := w.creatures["creature:alice"].Properties["proficiency/sharp"]; got != "64" {
		t.Fatalf("proficiency/sharp = %q, want 64", got)
	}
	if len(w.spawns) != 1 || w.spawns[0].protoID != "creature:spawn" || w.spawns[0].roomID != "room:1" || !w.spawns[0].carryItems {
		t.Fatalf("spawns = %+v", w.spawns)
	}
}

func TestHandlePermanentCreatureDeathSkipsFutureRespawnAndDuplicateQuest(t *testing.T) {
	w := newMockPermanentDeathWorld()
	w.activeSessions = []ActiveSession{{ID: "s1", ActorID: "player:alice"}}
	w.players["player:alice"] = model.Player{ID: "player:alice", CreatureID: "creature:alice"}
	w.creatures["creature:alice"] = model.Creature{
		ID:         "creature:alice",
		Stats:      map[string]int{"experience": 1000},
		Properties: map[string]string{questCompletionKey(1): "1"},
	}
	w.rooms["room:1"] = model.Room{
		ID: "room:1",
		Properties: map[string]string{
			"perm_mon.0.name":     "혈귀",
			"perm_mon.0.ltime":    "990",
			"perm_mon.0.interval": "30",
		},
	}
	w.creatures["creature:dead"] = model.Creature{
		ID:          "creature:dead",
		Kind:        model.CreatureKindMonster,
		DisplayName: "혈귀",
		RoomID:      "room:1",
		Stats:       map[string]int{"questNumber": 1},
		Metadata:    model.Metadata{Tags: []string{"permanent"}},
	}

	result, err := HandlePermanentCreatureDeath(w, "player:alice", "creature:dead", 1000)
	if err != nil {
		t.Fatalf("HandlePermanentCreatureDeath() error = %v", err)
	}

	if result.RespawnMarked {
		t.Fatalf("respawn marked for future slot: %+v", result)
	}
	if !result.QuestAlreadyCompleted || result.QuestCompleted {
		t.Fatalf("quest duplicate result = %+v", result)
	}
	if got := w.rooms["room:1"].Properties["perm_mon.0.ltime"]; got != "990" {
		t.Fatalf("perm_mon.0.ltime = %q, want unchanged 990", got)
	}
	if got := w.creatures["creature:alice"].Stats["experience"]; got != 1000 {
		t.Fatalf("experience = %d, want unchanged 1000", got)
	}
	if got := strings.Join(w.writtenTexts["s1"], ""); !strings.Contains(got, "이미 임무를 달성") {
		t.Fatalf("player messages = %q, want duplicate quest notice", got)
	}
}

func TestHandlePermanentCreatureDeathUsesLegacyRootMDEATHMSUMMOData(t *testing.T) {
	root := permanentDeathProjectRoot(t)
	roomPath := filepath.Join(root, "rooms", "r03", "r03566")
	if _, err := os.Stat(roomPath); err != nil {
		t.Skipf("legacy room fixture missing: %v", err)
	}

	template, found, err := readLegacyCreaturePrototype(root, 98)
	if err != nil {
		t.Fatalf("read legacy creature 98: %v", err)
	}
	if !found {
		t.Fatal("legacy creature 98 not found")
	}
	if template.Name != "타타르의 머리" || template.Level != 80 || template.Special != 99 {
		t.Fatalf("legacy creature 98 = %+v, want 타타르의 머리 level 80 special 99", template)
	}

	w := newMockPermanentDeathWorld()
	w.dbRoot = root
	w.activeSessions = []ActiveSession{{ID: "s1", ActorID: "player:alice"}}
	w.players["player:alice"] = model.Player{ID: "player:alice", CreatureID: "creature:alice"}
	w.creatures["creature:alice"] = model.Creature{
		ID:          "creature:alice",
		DisplayName: "앨리스",
		Stats:       map[string]int{"experience": 1000},
		Properties:  map[string]string{},
	}
	w.rooms["room:03566"] = model.Room{
		ID:          "room:03566",
		DisplayName: "이상한 방",
		Metadata: model.Metadata{
			Source:     "legacy",
			LegacyKind: "room",
			LegacyPath: "rooms/r03/r03566",
		},
		Properties: map[string]string{},
	}
	w.creatures["creature:dead"] = model.Creature{
		ID:          "creature:dead",
		Kind:        model.CreatureKindMonster,
		DisplayName: template.Name,
		Level:       template.Level,
		RoomID:      "room:03566",
		Stats: map[string]int{
			"special":      template.Special,
			"questNumber":  template.QuestNumber,
			"legacyNumber": template.Number,
		},
		Metadata: model.Metadata{
			Source:     "legacy",
			LegacyKind: "room.creature",
			RawFields:  map[string][]byte{"flags": append([]byte(nil), template.Flags...)},
		},
	}

	const nowUnix int64 = 2_000_000_000
	result, err := HandlePermanentCreatureDeath(w, "player:alice", "creature:dead", nowUnix)
	if err != nil {
		t.Fatalf("HandlePermanentCreatureDeath() error = %v", err)
	}

	if !result.Permanent || !result.RespawnMarked || !result.DeathDescriptionBroadcast || !result.SummonRequested {
		t.Fatalf("result = %+v, want permanent respawn, MDEATH ddesc, and MSUMMO summon", result)
	}
	if len(w.spawns) != 1 || w.spawns[0].protoID != "creature:m00:99" || w.spawns[0].roomID != "room:03566" || !w.spawns[0].carryItems {
		t.Fatalf("spawns = %+v, want legacy creature:m00:99 summon with carry items", w.spawns)
	}
	if result.SummonedCreatureID != "creature:m00:99:clone:1" {
		t.Fatalf("summoned id = %q, want creature:m00:99:clone:1", result.SummonedCreatureID)
	}
	roomProps := w.rooms["room:03566"].Properties
	if got := roomProps["perm_mon.0.misc"]; got != "98" {
		t.Fatalf("perm_mon.0.misc = %q, want 98", got)
	}
	if got := roomProps["perm_mon.0.interval"]; got != "720" {
		t.Fatalf("perm_mon.0.interval = %q, want 720", got)
	}
	if got := roomProps["perm_mon.0.name"]; got != "타타르의 머리" {
		t.Fatalf("perm_mon.0.name = %q, want 타타르의 머리", got)
	}
	if got := roomProps["perm_mon.0.ltime"]; got != "2000000000" {
		t.Fatalf("perm_mon.0.ltime = %q, want 2000000000", got)
	}
	if got := strings.Join(w.broadcastRooms["room:03566"], "\n"); !strings.Contains(got, "타타르") {
		t.Fatalf("death broadcasts = %q, want decoded legacy ddesc text", got)
	}
}

func TestHandlePermanentCreatureDeathHydratesRealSummonPrototypeBeforeSpawn(t *testing.T) {
	root := permanentDeathProjectRoot(t)
	summary := loadPermanentDeathWorld(t, root)

	tests := []struct {
		name                  string
		deadNumber            int
		roomID                model.RoomID
		wantSummonNumber      int
		wantCarryMaterialized bool
	}{
		{
			name:             "r03566_tatar_head_special_99",
			deadNumber:       98,
			roomID:           "room:03566",
			wantSummonNumber: 99,
		},
		{
			name:                  "r02807_zero_special_is_valid_load_crt_index",
			deadNumber:            653,
			roomID:                "room:02807",
			wantSummonNumber:      0,
			wantCarryMaterialized: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deadTemplate, found, err := readLegacyCreaturePrototype(root, tt.deadNumber)
			if err != nil {
				t.Fatalf("read dead legacy creature %d: %v", tt.deadNumber, err)
			}
			if !found {
				t.Fatalf("dead legacy creature %d not found", tt.deadNumber)
			}
			if deadTemplate.Special != tt.wantSummonNumber {
				t.Fatalf("dead legacy creature %d special = %d, want %d", tt.deadNumber, deadTemplate.Special, tt.wantSummonNumber)
			}
			summonTemplate, found, err := readLegacyCreaturePrototype(root, tt.wantSummonNumber)
			if err != nil {
				t.Fatalf("read summoned legacy creature %d: %v", tt.wantSummonNumber, err)
			}
			if !found {
				t.Fatalf("summoned legacy creature %d not found", tt.wantSummonNumber)
			}
			if tt.wantCarryMaterialized && !legacyCreaturePrototypeHasCarry(summonTemplate) {
				t.Fatalf("summoned legacy creature %d carry = %+v, want real carry fixture", tt.wantSummonNumber, summonTemplate.Carry)
			}

			runtime := state.NewWorld(summary.World)
			runtime.SetDBRoot(root)
			if _, ok := runtime.Room(tt.roomID); !ok {
				t.Fatalf("room %s not loaded from real legacy data", tt.roomID)
			}
			world := newStatePermanentDeathWorld(runtime)
			world.activeSessions = []ActiveSession{{ID: "s1", ActorID: "player:alice"}}

			killer := model.Creature{
				ID:          "creature:alice",
				Kind:        model.CreatureKindPlayer,
				DisplayName: "앨리스",
				Stats:       map[string]int{"experience": 1000},
				Properties:  map[string]string{},
			}
			if err := runtime.UpdateCreature(killer); err != nil {
				t.Fatalf("UpdateCreature(killer): %v", err)
			}
			if err := runtime.UpdatePlayer(model.Player{ID: "player:alice", DisplayName: "앨리스", CreatureID: killer.ID}); err != nil {
				t.Fatalf("UpdatePlayer(killer): %v", err)
			}

			dead := deadTemplate.Creature
			dead.ID = model.CreatureID(fmt.Sprintf("creature:dead:%d", tt.deadNumber))
			dead.RoomID = tt.roomID
			dead.Metadata.Tags = append(dead.Metadata.Tags, "MPERMT", "permanent")
			if err := runtime.UpdateCreature(dead); err != nil {
				t.Fatalf("UpdateCreature(dead): %v", err)
			}

			const nowUnix int64 = 2_000_000_000
			result, err := HandlePermanentCreatureDeath(world, "player:alice", dead.ID, nowUnix)
			if err != nil {
				t.Fatalf("HandlePermanentCreatureDeath() error = %v", err)
			}
			if !result.Permanent || !result.SummonRequested || result.SummonedCreatureID.IsZero() {
				t.Fatalf("result = %+v, want permanent death with summon", result)
			}

			summoned, ok := runtime.Creature(result.SummonedCreatureID)
			if !ok {
				t.Fatalf("summoned creature %s not found", result.SummonedCreatureID)
			}
			assertSummonedCreatureMatchesLegacyPrototype(t, summoned, summonTemplate, tt.roomID)
			assertSummonedCreatureEnemy(t, runtime, summoned.ID, killer.DisplayName)
			if tt.wantCarryMaterialized {
				assertSummonedCreatureCarryMaterialized(t, runtime, summoned, summonTemplate)
			}
		})
	}
}

func permanentDeathProjectRoot(t *testing.T) string {
	t.Helper()
	if os.Getenv("CI") != "" || testing.Short() {
		t.Skip("skipping slow real root parsing in CI or short mode")
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
	if _, err := os.Stat(filepath.Join(root, "objmon", "m00")); err != nil {
		t.Skipf("legacy data root unavailable: %v", err)
	}
	return root
}

func loadPermanentDeathWorld(t *testing.T, root string) worldload.Summary {
	t.Helper()
	summary, err := worldload.LoadRoot(root)
	if err != nil {
		t.Fatalf("LoadRoot(%q): %v", root, err)
	}
	if len(summary.Errors) > 0 {
		t.Fatalf("LoadRoot(%q) errors = %+v", root, summary.Errors)
	}
	return summary
}

func legacyCreaturePrototypeHasCarry(template legacyCreaturePrototype) bool {
	for _, carry := range template.Carry {
		if carry > 0 {
			return true
		}
	}
	return false
}

func assertSummonedCreatureMatchesLegacyPrototype(t *testing.T, got model.Creature, want legacyCreaturePrototype, roomID model.RoomID) {
	t.Helper()
	if got.DisplayName != want.Name {
		t.Fatalf("summoned name = %q, want legacy load_crt name %q", got.DisplayName, want.Name)
	}
	if got.Level != want.Level {
		t.Fatalf("summoned level = %d, want legacy load_crt level %d", got.Level, want.Level)
	}
	if got.RoomID != roomID {
		t.Fatalf("summoned room = %s, want %s", got.RoomID, roomID)
	}
	for _, key := range []string{"legacyNumber", "hpMax", "hpCurrent", "mpMax", "mpCurrent", "dexterity", "special", "questNumber"} {
		if got.Stats[key] != want.Creature.Stats[key] {
			t.Fatalf("summoned stat %s = %d, want legacy load_crt %d", key, got.Stats[key], want.Creature.Stats[key])
		}
	}
	assertSummonedCreatureGoldMatchesLegacySummonC(t, got, want)
	for i, carry := range want.Carry {
		key := fmt.Sprintf("carry[%d]", i)
		if got.Stats[key] != carry {
			t.Fatalf("summoned %s = %d, want legacy load_crt carry %d", key, got.Stats[key], carry)
		}
	}
}

func assertSummonedCreatureGoldMatchesLegacySummonC(t *testing.T, got model.Creature, want legacyCreaturePrototype) {
	t.Helper()
	wantGold := want.Creature.Stats["gold"]
	gotGold := got.Stats["gold"]
	if wantGold <= 0 {
		if gotGold != 0 {
			t.Fatalf("summoned gold = %d, want 0", gotGold)
		}
		return
	}
	if hasLegacyCreaturePrototypeTag(want, "MNRGLD") {
		if gotGold != wantGold {
			t.Fatalf("summoned fixed gold = %d, want %d", gotGold, wantGold)
		}
		return
	}
	minGold := wantGold / 10
	if gotGold < minGold || gotGold > wantGold {
		t.Fatalf("summoned randomized gold = %d, want C summon_crt range [%d,%d]", gotGold, minGold, wantGold)
	}
}

func hasLegacyCreaturePrototypeTag(template legacyCreaturePrototype, tag string) bool {
	for _, existing := range template.Creature.Metadata.Tags {
		if existing == tag {
			return true
		}
	}
	return false
}

func assertSummonedCreatureEnemy(t *testing.T, runtime *state.World, summonedID model.CreatureID, killerName string) {
	t.Helper()
	enemies, err := runtime.CreatureEnemies(summonedID)
	if err != nil {
		t.Fatalf("CreatureEnemies(%s): %v", summonedID, err)
	}
	for _, enemy := range enemies {
		if enemy == killerName {
			return
		}
	}
	t.Fatalf("summoned enemies = %+v, want %q from C summon_crt add_enm_crt", enemies, killerName)
}

func assertSummonedCreatureCarryMaterialized(t *testing.T, runtime *state.World, summoned model.Creature, template legacyCreaturePrototype) {
	t.Helper()
	if len(summoned.Inventory.ObjectIDs) == 0 {
		t.Fatalf("summoned inventory is empty, want carried object materialized from legacy carry %+v", template.Carry)
	}
	allowed := map[model.PrototypeID]struct{}{}
	for _, carry := range template.Carry {
		if carry <= 0 {
			continue
		}
		allowed[model.PrototypeID(fmt.Sprintf("object:o%02d:%d", carry/100, carry%100))] = struct{}{}
	}
	for _, objectID := range summoned.Inventory.ObjectIDs {
		object, ok := runtime.Object(objectID)
		if !ok {
			t.Fatalf("summoned inventory object %s not found", objectID)
		}
		if object.Location.CreatureID != summoned.ID {
			t.Fatalf("object %s location = %+v, want creature %s inventory", objectID, object.Location, summoned.ID)
		}
		if _, ok := allowed[object.PrototypeID]; !ok {
			t.Fatalf("object %s prototype = %s, want one of legacy carry prototypes %+v", objectID, object.PrototypeID, allowed)
		}
	}
}
