package game

import (
	"fmt"
	"strings"
	"testing"

	"muhan/internal/session"
	"muhan/internal/world/model"
)

type mockUpdatePlyWorld struct {
	activeSessions []ActiveSession
	sessionActors  map[session.ID]struct {
		creatureID model.CreatureID
		playerID   model.PlayerID
		ok         bool
	}
	sessionLastInputTime map[session.ID]struct {
		time int64
		ok   bool
	}
	disconnects           map[session.ID]int
	writtenTexts          map[session.ID][]string
	creatures             map[model.CreatureID]model.Creature
	players               map[model.PlayerID]model.Player
	creatureStats         map[model.CreatureID]map[string]int
	recalculateACCalls    map[model.CreatureID]int
	recalculateTHACOCalls map[model.CreatureID]int
	playerTagChanges      map[model.PlayerID]struct {
		added   []string
		removed []string
	}
	cooldowns        map[model.CreatureID]map[string]int64
	rooms            map[model.RoomID]model.Room
	movedPlayers     map[model.PlayerID]model.RoomID
	broadcastRooms   map[model.RoomID][]string
	broadcastAllMsgs []string
	objectProperties map[model.ObjectInstanceID]map[string]string
	objects          map[model.ObjectInstanceID]model.ObjectInstance
	objectPrototypes map[model.PrototypeID]model.ObjectPrototype
	savedPlayers     map[model.PlayerID]int

	effectExpirations  map[model.CreatureID]map[string]int64
	dispatchedCommands []string
}

func newMockUpdatePlyWorld() *mockUpdatePlyWorld {
	return &mockUpdatePlyWorld{
		sessionActors: make(map[session.ID]struct {
			creatureID model.CreatureID
			playerID   model.PlayerID
			ok         bool
		}),
		sessionLastInputTime: make(map[session.ID]struct {
			time int64
			ok   bool
		}),
		disconnects:           make(map[session.ID]int),
		writtenTexts:          make(map[session.ID][]string),
		creatures:             make(map[model.CreatureID]model.Creature),
		players:               make(map[model.PlayerID]model.Player),
		creatureStats:         make(map[model.CreatureID]map[string]int),
		recalculateACCalls:    make(map[model.CreatureID]int),
		recalculateTHACOCalls: make(map[model.CreatureID]int),
		playerTagChanges: make(map[model.PlayerID]struct {
			added   []string
			removed []string
		}),
		cooldowns:         make(map[model.CreatureID]map[string]int64),
		rooms:             make(map[model.RoomID]model.Room),
		movedPlayers:      make(map[model.PlayerID]model.RoomID),
		broadcastRooms:    make(map[model.RoomID][]string),
		objectProperties:  make(map[model.ObjectInstanceID]map[string]string),
		objects:           make(map[model.ObjectInstanceID]model.ObjectInstance),
		objectPrototypes:  make(map[model.PrototypeID]model.ObjectPrototype),
		savedPlayers:      make(map[model.PlayerID]int),
		effectExpirations: make(map[model.CreatureID]map[string]int64),
	}
}

func (m *mockUpdatePlyWorld) ActiveSessions() []ActiveSession {
	return m.activeSessions
}

func (m *mockUpdatePlyWorld) SessionActor(sessionID session.ID) (model.CreatureID, model.PlayerID, bool) {
	val := m.sessionActors[sessionID]
	return val.creatureID, val.playerID, val.ok
}

func (m *mockUpdatePlyWorld) SessionLastInputTime(sessionID session.ID) (int64, bool) {
	val := m.sessionLastInputTime[sessionID]
	return val.time, val.ok
}

func (m *mockUpdatePlyWorld) DisconnectSession(sessionID session.ID) error {
	m.disconnects[sessionID]++
	return nil
}

func (m *mockUpdatePlyWorld) WriteToSession(sessionID session.ID, text string, isPrompt bool) error {
	m.writtenTexts[sessionID] = append(m.writtenTexts[sessionID], text)
	return nil
}

func (m *mockUpdatePlyWorld) Creature(creatureID model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[creatureID]
	if ok {
		if stats, hasStats := m.creatureStats[creatureID]; hasStats {
			for k, v := range stats {
				c.Stats[k] = v
			}
		}
	}
	return c, ok
}

func (m *mockUpdatePlyWorld) Player(playerID model.PlayerID) (model.Player, bool) {
	p, ok := m.players[playerID]
	return p, ok
}

func (m *mockUpdatePlyWorld) SetCreatureStat(creatureID model.CreatureID, name string, val int) error {
	if _, ok := m.creatureStats[creatureID]; !ok {
		m.creatureStats[creatureID] = make(map[string]int)
	}
	m.creatureStats[creatureID][name] = val
	if c, ok := m.creatures[creatureID]; ok {
		c.Stats[name] = val
		m.creatures[creatureID] = c
	}
	return nil
}

func (m *mockUpdatePlyWorld) RecalculateAC(creatureID model.CreatureID) error {
	m.recalculateACCalls[creatureID]++
	if c, ok := m.Creature(creatureID); ok {
		ac := computeAC(c, m)
		_ = m.SetCreatureStat(creatureID, "armor", ac)
	}
	return nil
}

func (m *mockUpdatePlyWorld) RecalculateTHACO(creatureID model.CreatureID) error {
	m.recalculateTHACOCalls[creatureID]++
	if c, ok := m.Creature(creatureID); ok {
		thaco := computeTHACO(c, m)
		_ = m.SetCreatureStat(creatureID, "thaco", thaco)
	}
	return nil
}

func (m *mockUpdatePlyWorld) UpdatePlayerTags(playerID model.PlayerID, add, remove []string) (model.Player, error) {
	p, ok := m.players[playerID]
	if !ok {
		return model.Player{}, fmt.Errorf("player %s not found", playerID)
	}
	m.playerTagChanges[playerID] = struct {
		added   []string
		removed []string
	}{added: add, removed: remove}

	tagsMap := make(map[string]bool)
	for _, t := range p.Metadata.Tags {
		tagsMap[t] = true
	}
	for _, t := range remove {
		delete(tagsMap, t)
	}
	for _, t := range add {
		tagsMap[t] = true
	}
	var newTags []string
	for t := range tagsMap {
		newTags = append(newTags, t)
	}
	p.Metadata.Tags = newTags
	m.players[playerID] = p

	cID := p.CreatureID
	if cID.IsZero() {
		cID = model.CreatureID(playerID)
	}
	if c, ok := m.creatures[cID]; ok {
		c.Metadata.Tags = newTags
		m.creatures[cID] = c
	}

	return p, nil
}

func (m *mockUpdatePlyWorld) UseCreatureCooldown(creatureID model.CreatureID, key string, nowUnix int64, intervalSeconds int64) (int64, bool, error) {
	if _, ok := m.cooldowns[creatureID]; !ok {
		m.cooldowns[creatureID] = make(map[string]int64)
	}
	lastTime := m.cooldowns[creatureID][key]
	if nowUnix >= lastTime {
		m.cooldowns[creatureID][key] = nowUnix + intervalSeconds
		return lastTime, true, nil
	}
	return lastTime, false, nil
}

func (m *mockUpdatePlyWorld) SetCreatureCooldown(creatureID model.CreatureID, key string, nowUnix int64, intervalSeconds int64) error {
	if _, ok := m.cooldowns[creatureID]; !ok {
		m.cooldowns[creatureID] = make(map[string]int64)
	}
	m.cooldowns[creatureID][key] = nowUnix + intervalSeconds
	return nil
}

func (m *mockUpdatePlyWorld) Room(roomID model.RoomID) (model.Room, bool) {
	r, ok := m.rooms[roomID]
	return r, ok
}

func (m *mockUpdatePlyWorld) MovePlayerToRoom(playerID model.PlayerID, roomID model.RoomID) error {
	m.movedPlayers[playerID] = roomID
	if p, ok := m.players[playerID]; ok {
		p.RoomID = roomID
		m.players[playerID] = p
	}
	p, _ := m.players[playerID]
	cID := p.CreatureID
	if cID.IsZero() {
		cID = model.CreatureID(playerID)
	}
	if c, ok := m.creatures[cID]; ok {
		c.RoomID = roomID
		m.creatures[cID] = c
	}
	return nil
}

func (m *mockUpdatePlyWorld) BroadcastRoom(roomID model.RoomID, excludeSessionID session.ID, text string) error {
	m.broadcastRooms[roomID] = append(m.broadcastRooms[roomID], text)
	return nil
}

func (m *mockUpdatePlyWorld) BroadcastAll(text string) error {
	m.broadcastAllMsgs = append(m.broadcastAllMsgs, text)
	return nil
}

func (m *mockUpdatePlyWorld) SetObjectProperty(objectID model.ObjectInstanceID, key string, value string) (model.ObjectInstance, error) {
	if _, ok := m.objectProperties[objectID]; !ok {
		m.objectProperties[objectID] = make(map[string]string)
	}
	m.objectProperties[objectID][key] = value
	if obj, ok := m.objects[objectID]; ok {
		obj.Properties[key] = value
		m.objects[objectID] = obj
		return obj, nil
	}
	return model.ObjectInstance{}, fmt.Errorf("object %s not found", objectID)
}

func (m *mockUpdatePlyWorld) Object(objectID model.ObjectInstanceID) (model.ObjectInstance, bool) {
	obj, ok := m.objects[objectID]
	return obj, ok
}

func (m *mockUpdatePlyWorld) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	p, ok := m.objectPrototypes[id]
	return p, ok
}

func (m *mockUpdatePlyWorld) SavePlayer(playerID model.PlayerID) error {
	m.savedPlayers[playerID]++
	return nil
}

func (m *mockUpdatePlyWorld) GetEffectExpiration(creatureID model.CreatureID, tag string) (int64, bool) {
	if _, ok := m.effectExpirations[creatureID]; !ok {
		return 0, false
	}
	val, ok := m.effectExpirations[creatureID][tag]
	return val, ok
}

func (m *mockUpdatePlyWorld) SetEffectExpiration(creatureID model.CreatureID, tag string, expires int64) {
	if _, ok := m.effectExpirations[creatureID]; !ok {
		m.effectExpirations[creatureID] = make(map[string]int64)
	}
	m.effectExpirations[creatureID][tag] = expires
}

func (m *mockUpdatePlyWorld) DeleteEffectExpiration(creatureID model.CreatureID, tag string) {
	if _, ok := m.effectExpirations[creatureID]; ok {
		delete(m.effectExpirations[creatureID], tag)
	}
}

func (m *mockUpdatePlyWorld) DispatchCommand(sessionID session.ID, playerID model.PlayerID, line string) error {
	m.dispatchedCommands = append(m.dispatchedCommands, fmt.Sprintf("%s:%s:%s", sessionID, playerID, line))
	return nil
}

func TestCreatureStatNormalizesStatAndPropertyKeys(t *testing.T) {
	creature := model.Creature{
		Stats:      map[string]int{"HP-CURRENT": 12},
		Properties: map[string]string{"mp-current": "5"},
	}

	if got := creatureStat(creature, "hpCurrent"); got != 12 {
		t.Fatalf("normalized stat = %d, want 12", got)
	}
	if got := creatureStat(creature, "mpCurrent"); got != 5 {
		t.Fatalf("normalized property = %d, want 5", got)
	}
	if got := creatureClass(model.Creature{Stats: map[string]int{"CLASS": legacyClassDM}}); got != legacyClassDM {
		t.Fatalf("creatureClass(normalized stat) = %d, want %d", got, legacyClassDM)
	}
}

func TestUpdatePlayerStatuses_IdleTimeout(t *testing.T) {
	w := newMockUpdatePlyWorld()

	// 1. Non-DM player: Idle > 300s
	w.activeSessions = []ActiveSession{
		{ID: "s1", ActorID: "player:alice"},
		{ID: "s2", ActorID: "player:bob"},
	}
	w.sessionActors["s1"] = struct {
		creatureID model.CreatureID
		playerID   model.PlayerID
		ok         bool
	}{creatureID: "creature:alice", playerID: "player:alice", ok: true}
	w.sessionActors["s2"] = struct {
		creatureID model.CreatureID
		playerID   model.PlayerID
		ok         bool
	}{creatureID: "creature:bob", playerID: "player:bob", ok: true}

	w.sessionLastInputTime["s1"] = struct {
		time int64
		ok   bool
	}{time: 1000 - 301, ok: true}
	w.sessionLastInputTime["s2"] = struct {
		time int64
		ok   bool
	}{time: 1000 - 100, ok: true}

	w.creatures["creature:alice"] = model.Creature{
		ID:    "creature:alice",
		Stats: map[string]int{"class": legacyClassBarbarian},
	}
	w.creatures["creature:bob"] = model.Creature{
		ID:    "creature:bob",
		Stats: map[string]int{"class": legacyClassBarbarian},
	}
	w.players["player:alice"] = model.Player{ID: "player:alice", CreatureID: "creature:alice"}
	w.players["player:bob"] = model.Player{ID: "player:bob", CreatureID: "creature:bob"}

	UpdatePlayerStatuses(w, 1000)

	if w.disconnects["s1"] != 1 {
		t.Errorf("expected session s1 to be disconnected, got %d", w.disconnects["s1"])
	}
	if len(w.writtenTexts["s1"]) == 0 || !strings.Contains(w.writtenTexts["s1"][0], "접속이 끊어집니다") {
		t.Errorf("expected warnings written to s1, got %v", w.writtenTexts["s1"])
	}

	if w.disconnects["s2"] != 0 {
		t.Errorf("expected session s2 not to be disconnected, got %d", w.disconnects["s2"])
	}

	// 2. DM player: Idle > 300s
	w = newMockUpdatePlyWorld()
	w.activeSessions = []ActiveSession{
		{ID: "s1", ActorID: "player:admin"},
	}
	w.sessionActors["s1"] = struct {
		creatureID model.CreatureID
		playerID   model.PlayerID
		ok         bool
	}{creatureID: "creature:admin", playerID: "player:admin", ok: true}
	w.sessionLastInputTime["s1"] = struct {
		time int64
		ok   bool
	}{time: 1000 - 301, ok: true}
	w.creatures["creature:admin"] = model.Creature{
		ID:    "creature:admin",
		Stats: map[string]int{"class": legacyClassDM},
	}
	w.players["player:admin"] = model.Player{ID: "player:admin", CreatureID: "creature:admin"}

	UpdatePlayerStatuses(w, 1000)

	if w.disconnects["s1"] != 0 {
		t.Errorf("expected DM session s1 NOT to be disconnected, got %d", w.disconnects["s1"])
	}
}

func TestUpdatePlayerStatuses_StatusExpiration(t *testing.T) {
	w := newMockUpdatePlyWorld()
	w.activeSessions = []ActiveSession{
		{ID: "s1", ActorID: "player:alice"},
	}
	w.sessionActors["s1"] = struct {
		creatureID model.CreatureID
		playerID   model.PlayerID
		ok         bool
	}{creatureID: "creature:alice", playerID: "player:alice", ok: true}

	c := model.Creature{
		ID: "creature:alice",
		Metadata: model.Metadata{
			Tags: []string{"PHASTE", "PPOWER", "PUPDMG", "PSLAYE", "PMEDIT", "PPRAYD", "ANSIC"},
		},
		Stats: map[string]int{
			"class":        legacyClassBarbarian,
			"level":        20,
			"dexterity":    20,
			"strength":     20,
			"pdice":        10,
			"hpMax":        500,
			"mpMax":        200,
			"thaco":        15,
			"intelligence": 15,
			"piety":        15,
		},
	}
	w.creatures["creature:alice"] = c
	w.players["player:alice"] = model.Player{
		ID:         "player:alice",
		CreatureID: "creature:alice",
		Metadata: model.Metadata{
			Tags: []string{"PHASTE", "PPOWER", "PUPDMG", "PSLAYE", "PMEDIT", "PPRAYD", "ANSIC"},
		},
	}

	// Set effect expiration times in the past
	w.SetEffectExpiration("creature:alice", "PHASTE", 900)
	w.SetEffectExpiration("creature:alice", "PPOWER", 900)
	w.SetEffectExpiration("creature:alice", "PUPDMG", 900)
	w.SetEffectExpiration("creature:alice", "PSLAYE", 900)
	w.SetEffectExpiration("creature:alice", "PMEDIT", 900)
	w.SetEffectExpiration("creature:alice", "PPRAYD", 900)

	UpdatePlayerStatuses(w, 1000)

	// Verify tags removed
	p := w.players["player:alice"]
	for _, tag := range []string{"PHASTE", "PPOWER", "PUPDMG", "PSLAYE", "PMEDIT", "PPRAYD"} {
		if hasAnyNormalizedFlag(p.Metadata.Tags, tag) {
			t.Errorf("expected tag %s to be removed", tag)
		}
	}

	// Verify stats updated
	cUpdated, _ := w.Creature("creature:alice")
	if dex := cUpdated.Stats["dexterity"]; dex != 5 {
		t.Errorf("expected dexterity to be 5, got %d", dex)
	}
	if str := cUpdated.Stats["strength"]; str != 17 {
		t.Errorf("expected strength to be 17, got %d", str)
	}
	if pdice := cUpdated.Stats["pdice"]; pdice != 8 {
		t.Errorf("expected pdice to be 8, got %d", pdice)
	}
	if hpMax := cUpdated.Stats["hpMax"]; hpMax != 450 {
		t.Errorf("expected hpMax to be 450, got %d", hpMax)
	}
	if mpMax := cUpdated.Stats["mpMax"]; mpMax != 180 {
		t.Errorf("expected mpMax to be 180, got %d", mpMax)
	}
	if thaco := cUpdated.Stats["thaco"]; thaco != 16 {
		t.Errorf("expected thaco to be 16, got %d", thaco)
	}
	if intel := cUpdated.Stats["intelligence"]; intel != 12 {
		t.Errorf("expected intelligence to be 12, got %d", intel)
	}
	if piety := cUpdated.Stats["piety"]; piety != 10 {
		t.Errorf("expected piety to be 10, got %d", piety)
	}

	// Verify messages written
	msgs := strings.Join(w.writtenTexts["s1"], " ")
	expectedMsgs := []string{
		"느려졌습니다",
		"약해졌습니다",
		"기가 빠져나갑니다",
		"살기를 잃었습니다",
		"참선의 영향력이",
		"믿음이 약해졌습니다",
	}
	for _, msg := range expectedMsgs {
		if !strings.Contains(msgs, msg) {
			t.Errorf("expected output to contain %q, but didn't. Output: %q", msg, msgs)
		}
	}
}

func TestUpdatePlyProficiencySubDMUsesCPrivilegedTables(t *testing.T) {
	subDM := model.Creature{
		Stats: map[string]int{
			"class":            legacyClassSubDM,
			"proficiencySharp": 1024,
			"realmFire":        2048,
		},
		Properties: map[string]string{
			"proficiency/thrust": "1024",
		},
	}
	if got := profic(subDM, 0); got != 20 {
		t.Fatalf("profic(SUB_DM, sharp) = %d, want 20", got)
	}
	if got := profic(subDM, 1); got != 20 {
		t.Fatalf("profic(SUB_DM, thrust) = %d, want 20", got)
	}
	if got := mprofic(subDM, 1); got != 20 {
		t.Fatalf("mprofic(SUB_DM, fire) = %d, want 20", got)
	}

	thief := model.Creature{Stats: map[string]int{
		"class":            legacyClassThief,
		"proficiencySharp": 1024,
	}}
	if got := profic(thief, 0); got != 4 {
		t.Fatalf("profic(thief, sharp) = %d, want 4", got)
	}
}

func TestComputeTHACOSumsCWeaponAndMagicProficiencyTotals(t *testing.T) {
	w := newMockUpdatePlyWorld()
	creature := model.Creature{
		ID: "creature:alice",
		Stats: map[string]int{
			"class":         legacyClassFighter,
			"level":         20,
			"proficiency/0": 1024,
			"proficiency/1": 1024,
			"proficiency/2": 1024,
			"proficiency/3": 1024,
			"proficiency/4": 1024,
			"realmEarth":    1024,
			"realmWind":     1024,
			"realmFire":     1024,
		},
	}

	if got := computeTHACO(creature, w); got != 13 {
		t.Fatalf("computeTHACO() = %d, want 13", got)
	}
}

func TestUpdatePlyCreatureHasAnyFlagReadsStatBackedLegacyFlags(t *testing.T) {
	creature := model.Creature{Stats: map[string]int{"PPOISN": 1}}
	if !creatureHasAnyFlag(creature, "PPOISN", "poison") {
		t.Fatalf("stat-backed PPOISN was not detected")
	}

	creature = model.Creature{Stats: map[string]int{"PPOISN": 0}}
	if creatureHasAnyFlag(creature, "PPOISN", "poison") {
		t.Fatalf("zero stat-backed PPOISN was detected as enabled")
	}

	creature = model.Creature{
		Stats:      map[string]int{"PPOISN": 0},
		Properties: map[string]string{"PPOISN": "true"},
	}
	if !creatureHasAnyFlag(creature, "PPOISN", "poison") {
		t.Fatalf("property-backed PPOISN should remain detected")
	}
}

func TestUpdatePlyCombatCalculationsReadStatBackedLegacyFlags(t *testing.T) {
	w := newMockUpdatePlyWorld()
	baseStats := map[string]int{
		"class":        legacyClassFighter,
		"level":        1,
		"constitution": 50,
		"dexterity":    50,
	}
	plain := model.Creature{ID: "creature:plain", Stats: map[string]int{}}
	for key, value := range baseStats {
		plain.Stats[key] = value
	}
	flagged := model.Creature{ID: "creature:flagged", Stats: map[string]int{}}
	for key, value := range baseStats {
		flagged.Stats[key] = value
	}
	flagged.Stats["PBLESS"] = 1
	flagged.Stats["PCHOI"] = 1
	zero := model.Creature{ID: "creature:zero", Stats: map[string]int{}}
	for key, value := range baseStats {
		zero.Stats[key] = value
	}
	zero.Stats["PBLESS"] = 0
	zero.Stats["PCHOI"] = 0

	if got, want := computeAC(flagged, w)-computeAC(plain, w), 20; got != want {
		t.Fatalf("stat-backed PCHOI armor delta = %d, want %d", got, want)
	}
	if got, want := computeTHACO(flagged, w)-computeTHACO(plain, w), 2; got != want {
		t.Fatalf("stat-backed PBLESS/PCHOI thaco delta = %d, want %d", got, want)
	}
	if computeAC(zero, w) != computeAC(plain, w) || computeTHACO(zero, w) != computeTHACO(plain, w) {
		t.Fatalf("zero stat-backed combat flags affected calculations")
	}
}

func TestUpdatePlayerStatuses_Regeneration(t *testing.T) {
	// 1. Normal room, healthy player
	w := newMockUpdatePlyWorld()
	w.activeSessions = []ActiveSession{{ID: "s1", ActorID: "player:alice"}}
	w.sessionActors["s1"] = struct {
		creatureID model.CreatureID
		playerID   model.PlayerID
		ok         bool
	}{creatureID: "creature:alice", playerID: "player:alice", ok: true}

	w.rooms["room:1"] = model.Room{ID: "room:1"}
	w.creatures["creature:alice"] = model.Creature{
		ID:     "creature:alice",
		RoomID: "room:1",
		Stats: map[string]int{
			"class":        legacyClassBarbarian,
			"constitution": 18,
			"hpCurrent":    50,
			"hpMax":        100,
			"mpCurrent":    10,
			"mpMax":        50,
			"intelligence": 18,
		},
	}
	w.players["player:alice"] = model.Player{ID: "player:alice", CreatureID: "creature:alice"}

	// Trigger tick and check regeneration
	UpdatePlayerStatuses(w, 1000)

	c, _ := w.Creature("creature:alice")
	// con 18 => legacyStatBonus(18) = 2.
	// hpAdd = 5 + 2 = 7. Barbarian gets +2 => 9.
	// hpCurrent: 50 + 9 = 59.
	if hp := c.Stats["hpCurrent"]; hp != 59 {
		t.Errorf("expected regenerated HP to be 59, got %d", hp)
	}
	// mpAdd = 5 + 1 (int > 17) = 6.
	// mpCurrent: 10 + 6 = 16.
	if mp := c.Stats["mpCurrent"]; mp != 16 {
		t.Errorf("expected regenerated MP to be 16, got %d", mp)
	}

	// 2. Healing room (RHEALR flag)
	w = newMockUpdatePlyWorld()
	w.activeSessions = []ActiveSession{{ID: "s1", ActorID: "player:alice"}}
	w.sessionActors["s1"] = struct {
		creatureID model.CreatureID
		playerID   model.PlayerID
		ok         bool
	}{creatureID: "creature:alice", playerID: "player:alice", ok: true}

	w.rooms["room:1"] = model.Room{
		ID: "room:1",
		Metadata: model.Metadata{
			Tags: []string{"RHEALR"},
		},
	}
	w.creatures["creature:alice"] = model.Creature{
		ID:     "creature:alice",
		RoomID: "room:1",
		Stats: map[string]int{
			"class":        legacyClassBarbarian,
			"constitution": 18,
			"hpCurrent":    50,
			"hpMax":        100,
			"mpCurrent":    10,
			"mpMax":        50,
			"intelligence": 18,
		},
	}
	w.players["player:alice"] = model.Player{ID: "player:alice", CreatureID: "creature:alice"}

	UpdatePlayerStatuses(w, 1000)

	c, _ = w.Creature("creature:alice")
	// RHEALR adds +10 to both HP and MP
	// hp: 59 + 10 = 69
	// mp: 16 + 10 = 26
	if hp := c.Stats["hpCurrent"]; hp != 69 {
		t.Errorf("expected regenerated HP to be 69, got %d", hp)
	}
	if mp := c.Stats["mpCurrent"]; mp != 26 {
		t.Errorf("expected regenerated MP to be 26, got %d", mp)
	}
}

func TestUpdatePlayerStatuses_PoisonAndDisease(t *testing.T) {
	// 1. Poison tick - damage applied
	w := newMockUpdatePlyWorld()
	w.activeSessions = []ActiveSession{{ID: "s1", ActorID: "player:alice"}}
	w.sessionActors["s1"] = struct {
		creatureID model.CreatureID
		playerID   model.PlayerID
		ok         bool
	}{creatureID: "creature:alice", playerID: "player:alice", ok: true}

	w.rooms["room:1"] = model.Room{ID: "room:1"}
	w.creatures["creature:alice"] = model.Creature{
		ID:     "creature:alice",
		RoomID: "room:1",
		Metadata: model.Metadata{
			Tags: []string{"PPOISN", "ANSIC"},
		},
		Stats: map[string]int{
			"class":        legacyClassBarbarian,
			"constitution": 15, // bonus = 1
			"hpCurrent":    50,
			"hpMax":        100,
			"mpCurrent":    10,
			"mpMax":        50,
		},
	}
	w.players["player:alice"] = model.Player{
		ID:         "player:alice",
		CreatureID: "creature:alice",
		Metadata: model.Metadata{
			Tags: []string{"PPOISN", "ANSIC"},
		},
	}

	UpdatePlayerStatuses(w, 1000)

	c, _ := w.Creature("creature:alice")
	// Poison damage: mrand(1, hpMax/20) - bonus.
	// hpMax/20 = 5.
	// damage is in [1, 5] - 1. Minimum 1.
	// HP should be between 50 - 4 = 46 and 50 - 1 = 49 (or 50 - 0 = 50 but here conBonus=1)
	if hp := c.Stats["hpCurrent"]; hp < 46 || hp > 49 {
		t.Errorf("expected HP to decrease, got %d", hp)
	}

	msgs := strings.Join(w.writtenTexts["s1"], " ")
	if !strings.Contains(msgs, "독이 당신의 핏줄로 스며듭니다") {
		t.Errorf("expected poison message, got %q", msgs)
	}

	// 2. Disease tick - damage and fatigue
	w = newMockUpdatePlyWorld()
	w.activeSessions = []ActiveSession{{ID: "s1", ActorID: "player:alice"}}
	w.sessionActors["s1"] = struct {
		creatureID model.CreatureID
		playerID   model.PlayerID
		ok         bool
	}{creatureID: "creature:alice", playerID: "player:alice", ok: true}

	w.rooms["room:1"] = model.Room{ID: "room:1"}
	w.creatures["creature:alice"] = model.Creature{
		ID:     "creature:alice",
		RoomID: "room:1",
		Metadata: model.Metadata{
			Tags: []string{"PDISEA", "ANSIC"},
		},
		Stats: map[string]int{
			"class":        legacyClassBarbarian,
			"constitution": 15, // bonus = 1
			"hpCurrent":    50,
			"hpMax":        100,
			"mpCurrent":    10,
			"mpMax":        50,
		},
	}
	w.players["player:alice"] = model.Player{
		ID:         "player:alice",
		CreatureID: "creature:alice",
		Metadata: model.Metadata{
			Tags: []string{"PDISEA", "ANSIC"},
		},
	}

	UpdatePlayerStatuses(w, 1000)

	c, _ = w.Creature("creature:alice")
	// Disease damage: mrand(1, 6) - bonus. Minimum 1.
	if hp := c.Stats["hpCurrent"]; hp < 45 || hp > 49 {
		t.Errorf("expected HP to decrease from disease, got %d", hp)
	}

	msgs = strings.Join(w.writtenTexts["s1"], " ")
	if !strings.Contains(msgs, "병이 당신의 마음을 잠식합니다") || !strings.Contains(msgs, "몸이 피로해 집니다") {
		t.Errorf("expected disease messages, got %q", msgs)
	}
	if _, ok := w.cooldowns["creature:alice"]["cooldown:attack"]; !ok {
		t.Errorf("expected attack cooldown to be set due to disease")
	}
}

func TestUpdatePlayerStatusesReadsStatBackedPoisonFlag(t *testing.T) {
	w := newMockUpdatePlyWorld()
	w.activeSessions = []ActiveSession{{ID: "s1", ActorID: "player:alice"}}
	w.sessionActors["s1"] = struct {
		creatureID model.CreatureID
		playerID   model.PlayerID
		ok         bool
	}{creatureID: "creature:alice", playerID: "player:alice", ok: true}

	w.rooms["room:1"] = model.Room{ID: "room:1"}
	w.creatures["creature:alice"] = model.Creature{
		ID:     "creature:alice",
		RoomID: "room:1",
		Stats: map[string]int{
			"class":        legacyClassBarbarian,
			"constitution": 15,
			"hpCurrent":    50,
			"hpMax":        100,
			"mpCurrent":    10,
			"mpMax":        50,
			"PPOISN":       1,
		},
	}
	w.players["player:alice"] = model.Player{ID: "player:alice", CreatureID: "creature:alice"}

	UpdatePlayerStatuses(w, 1000)

	c, _ := w.Creature("creature:alice")
	if hp := c.Stats["hpCurrent"]; hp >= 50 {
		t.Fatalf("stat-backed PPOISN did not damage player, hp = %d", hp)
	}
	msgs := strings.Join(w.writtenTexts["s1"], " ")
	if !strings.Contains(msgs, "독이 당신의 핏줄로 스며듭니다") {
		t.Fatalf("output = %q, want stat-backed poison message", msgs)
	}
}

func TestUpdatePlayerStatuses_RoomDamage(t *testing.T) {
	// 1. Harm Room with Fire and Earth damage
	w := newMockUpdatePlyWorld()
	w.activeSessions = []ActiveSession{{ID: "s1", ActorID: "player:alice"}}
	w.sessionActors["s1"] = struct {
		creatureID model.CreatureID
		playerID   model.PlayerID
		ok         bool
	}{creatureID: "creature:alice", playerID: "player:alice", ok: true}

	w.rooms["room:1"] = model.Room{
		ID: "room:1",
		Metadata: model.Metadata{
			Tags: []string{"RPHARM", "RFIRER"},
		},
	}
	w.creatures["creature:alice"] = model.Creature{
		ID:     "creature:alice",
		RoomID: "room:1",
		Stats: map[string]int{
			"class":        legacyClassBarbarian,
			"constitution": 15, // bonus = 1
			"hpCurrent":    50,
			"hpMax":        100,
			"mpCurrent":    10,
			"mpMax":        50,
		},
	}
	w.players["player:alice"] = model.Player{ID: "player:alice", CreatureID: "creature:alice"}

	UpdatePlayerStatuses(w, 1000)

	c, _ := w.Creature("creature:alice")
	// Fire damage applied (prot = false). con bonus = 1.
	// damage = 8 - 1 = 7.
	// hp: 50 - 7 = 43.
	if hp := c.Stats["hpCurrent"]; hp != 43 {
		t.Errorf("expected HP after room fire damage to be 43, got %d", hp)
	}
	msgs := strings.Join(w.writtenTexts["s1"], " ")
	if !strings.Contains(msgs, "뜨거운 기운이 당신을 태웁니다") {
		t.Errorf("expected fire damage message, got %q", msgs)
	}
}

func TestUpdatePlayerStatuses_Death(t *testing.T) {
	w := newMockUpdatePlyWorld()
	w.activeSessions = []ActiveSession{{ID: "s1", ActorID: "player:alice"}}
	w.sessionActors["s1"] = struct {
		creatureID model.CreatureID
		playerID   model.PlayerID
		ok         bool
	}{creatureID: "creature:alice", playerID: "player:alice", ok: true}

	w.rooms["room:1"] = model.Room{ID: "room:1"}
	w.creatures["creature:alice"] = model.Creature{
		ID:          "creature:alice",
		RoomID:      "room:1",
		DisplayName: "엘리스",
		Metadata: model.Metadata{
			Tags: []string{"PPOISN"},
		},
		Stats: map[string]int{
			"class":        legacyClassBarbarian,
			"constitution": 10,
			"hpCurrent":    1, // Low HP, next tick of poison will kill
			"hpMax":        100,
			"mpCurrent":    10,
			"mpMax":        50,
		},
	}
	w.players["player:alice"] = model.Player{
		ID:          "player:alice",
		CreatureID:  "creature:alice",
		DisplayName: "엘리스",
		Metadata: model.Metadata{
			Tags: []string{"PPOISN"},
		},
	}

	UpdatePlayerStatuses(w, 1000)

	// Player should be dead and reborn
	c, _ := w.Creature("creature:alice")
	if hp := c.Stats["hpCurrent"]; hp != 100 {
		t.Errorf("expected HP to be reset to hpMax (100), got %d", hp)
	}
	if room := w.movedPlayers["player:alice"]; room != "room:1008" {
		t.Errorf("expected player to be moved to room:1008, got %s", room)
	}
	p := w.players["player:alice"]
	if hasAnyNormalizedFlag(p.Metadata.Tags, "PPOISN") {
		t.Errorf("expected poison tags to be removed on death")
	}

	// Verify death messages
	msgs := strings.Join(w.writtenTexts["s1"], " ")
	if !strings.Contains(msgs, "당신은 죽으면서 몇가지 물건을 떨어뜨렸습니다") {
		t.Errorf("expected death notice message, got %q", msgs)
	}
	if len(w.broadcastAllMsgs) == 0 || !strings.Contains(w.broadcastAllMsgs[0], "애석하게도 엘리스님이 죽었습니다") {
		t.Errorf("expected global death broadcast, got %v", w.broadcastAllMsgs)
	}
}

func TestUpdatePlayerStatuses_LightDecayAndSave(t *testing.T) {
	w := newMockUpdatePlyWorld()
	w.activeSessions = []ActiveSession{{ID: "s1", ActorID: "player:alice"}}
	w.sessionActors["s1"] = struct {
		creatureID model.CreatureID
		playerID   model.PlayerID
		ok         bool
	}{creatureID: "creature:alice", playerID: "player:alice", ok: true}

	w.rooms["room:1"] = model.Room{ID: "room:1"}

	// Equipping a light source
	lightObjID := model.ObjectInstanceID("obj:lantern")
	w.objects[lightObjID] = model.ObjectInstance{
		ID:          lightObjID,
		PrototypeID: "proto:lantern",
		Metadata: model.Metadata{
			Tags: []string{"OLIGHT"},
		},
		Properties: map[string]string{
			"type":         "12", // LIGHTSOURCE
			"shotsCurrent": "5",
			"name":         "랜턴",
		},
	}
	w.objectPrototypes["proto:lantern"] = model.ObjectPrototype{
		ID:          "proto:lantern",
		DisplayName: "랜턴",
		Metadata: model.Metadata{
			Tags: []string{"OLIGHT"},
		},
		Properties: map[string]string{
			"type": "12",
		},
	}

	w.creatures["creature:alice"] = model.Creature{
		ID:     "creature:alice",
		RoomID: "room:1",
		Equipment: map[string]model.ObjectInstanceID{
			"held": lightObjID,
		},
		Stats: map[string]int{
			"class":     legacyClassBarbarian,
			"hpCurrent": 100,
			"hpMax":     100,
		},
	}
	w.players["player:alice"] = model.Player{ID: "player:alice", CreatureID: "creature:alice"}

	UpdatePlayerStatuses(w, 1000)

	// Verify light decay
	chargesStr := w.objectProperties[lightObjID]["shotsCurrent"]
	if chargesStr != "4" {
		t.Errorf("expected charges to decrement to 4, got %s", chargesStr)
	}

	// Verify SavePlayer is triggered (since cooldown is initial/usable)
	if w.savedPlayers["player:alice"] != 1 {
		t.Errorf("expected player to be saved, got %d", w.savedPlayers["player:alice"])
	}
}

func TestUpdatePlayerStatusesLightReadsPropertyBackedLegacyFlags(t *testing.T) {
	for _, tc := range []struct {
		name        string
		objectProps map[string]string
		protoProps  map[string]string
	}{
		{
			name:        "direct object flag",
			objectProps: map[string]string{"OLIGHT": "1"},
		},
		{
			name:        "object flags token",
			objectProps: map[string]string{"flags": "OLIGHT|hidden"},
		},
		{
			name:       "prototype flags token",
			protoProps: map[string]string{"flags": "OLIGHT"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newMockUpdatePlyWorld()
			w.activeSessions = []ActiveSession{{ID: "s1", ActorID: "player:alice"}}
			w.sessionActors["s1"] = struct {
				creatureID model.CreatureID
				playerID   model.PlayerID
				ok         bool
			}{creatureID: "creature:alice", playerID: "player:alice", ok: true}
			w.rooms["room:1"] = model.Room{ID: "room:1"}

			lightObjID := model.ObjectInstanceID("obj:lantern")
			objectProps := map[string]string{
				"type":         "12",
				"shotsCurrent": "5",
				"name":         "랜턴",
			}
			for key, value := range tc.objectProps {
				objectProps[key] = value
			}
			w.objects[lightObjID] = model.ObjectInstance{
				ID:          lightObjID,
				PrototypeID: "proto:lantern",
				Properties:  objectProps,
			}
			w.objectPrototypes["proto:lantern"] = model.ObjectPrototype{
				ID:          "proto:lantern",
				DisplayName: "랜턴",
				Properties:  tc.protoProps,
			}
			w.creatures["creature:alice"] = model.Creature{
				ID:     "creature:alice",
				RoomID: "room:1",
				Equipment: map[string]model.ObjectInstanceID{
					"held": lightObjID,
				},
				Stats: map[string]int{
					"class":     legacyClassBarbarian,
					"hpCurrent": 100,
					"hpMax":     100,
				},
			}
			w.players["player:alice"] = model.Player{ID: "player:alice", CreatureID: "creature:alice"}

			UpdatePlayerStatuses(w, 1000)

			if got := w.objectProperties[lightObjID]["shotsCurrent"]; got != "4" {
				t.Fatalf("shotsCurrent = %q, want 4", got)
			}
		})
	}
}

func TestUpdatePlayerStatuses_CombatSkillsExpiration(t *testing.T) {
	w := newMockUpdatePlyWorld()
	w.activeSessions = []ActiveSession{{ID: "s1", ActorID: "player:alice"}}
	w.sessionActors["s1"] = struct {
		creatureID model.CreatureID
		playerID   model.PlayerID
		ok         bool
	}{creatureID: "creature:alice", playerID: "player:alice", ok: true}

	c := model.Creature{
		ID: "creature:alice",
		Metadata: model.Metadata{
			Tags: []string{"PREFLECT", "PSHADOW", "PABSORB", "PCHOI", "ANSIC"},
		},
		Stats: map[string]int{
			"class":        legacyClassBarbarian,
			"level":        20,
			"dexterity":    20,
			"strength":     20,
			"shadowClones": 5,
		},
	}
	w.creatures["creature:alice"] = c
	w.players["player:alice"] = model.Player{
		ID:         "player:alice",
		CreatureID: "creature:alice",
		Metadata: model.Metadata{
			Tags: []string{"PREFLECT", "PSHADOW", "PABSORB", "PCHOI", "ANSIC"},
		},
	}

	w.SetEffectExpiration("creature:alice", "PREFLECT", 900)
	w.SetEffectExpiration("creature:alice", "PSHADOW", 900)
	w.SetEffectExpiration("creature:alice", "PABSORB", 900)
	w.SetEffectExpiration("creature:alice", "PCHOI", 900)

	UpdatePlayerStatuses(w, 1000)

	// Verify tags removed
	p := w.players["player:alice"]
	for _, tag := range []string{"PREFLECT", "PSHADOW", "PABSORB", "PCHOI"} {
		if hasAnyNormalizedFlag(p.Metadata.Tags, tag) {
			t.Errorf("expected tag %s to be removed", tag)
		}
	}

	// Verify clones count reset
	cUpdated, _ := w.Creature("creature:alice")
	if clones := cUpdated.Stats["shadowClones"]; clones != 0 {
		t.Errorf("expected shadowClones to be 0, got %d", clones)
	}

	// Verify recalculations called
	if w.recalculateACCalls["creature:alice"] == 0 {
		t.Errorf("expected RecalculateAC to be called on expiration")
	}
	if w.recalculateTHACOCalls["creature:alice"] == 0 {
		t.Errorf("expected RecalculateTHACO to be called on expiration")
	}

	// Verify expiration messages
	msgs := strings.Join(w.writtenTexts["s1"], " ")
	expectedMsgs := []string{
		"반탄강기가 풀렸습니다",
		"분신들이 사라졌습니다",
		"흡성대법 기운이 사라졌습니다",
		"최루탄의 매운 기운이 가셨습니다",
	}
	for _, msg := range expectedMsgs {
		if !strings.Contains(msgs, msg) {
			t.Errorf("expected output to contain %q, but didn't. Output: %q", msg, msgs)
		}
	}
}

func TestWimpyAutoFleeTick(t *testing.T) {
	w := newMockUpdatePlyWorld()
	w.activeSessions = []ActiveSession{{ID: "s1", ActorID: "player:alice"}}
	w.sessionActors["s1"] = struct {
		creatureID model.CreatureID
		playerID   model.PlayerID
		ok         bool
	}{creatureID: "creature:alice", playerID: "player:alice", ok: true}

	w.rooms["room:1"] = model.Room{
		ID:          "room:1",
		CreatureIDs: []model.CreatureID{"creature:alice", "creature:monster"},
	}

	w.creatures["creature:alice"] = model.Creature{
		ID:     "creature:alice",
		RoomID: "room:1",
		Metadata: model.Metadata{
			Tags: []string{"PWIMPY"},
		},
		Stats: map[string]int{
			"class":      legacyClassBarbarian,
			"hpCurrent":  15,
			"hpMax":      100,
			"wimpyValue": 20,
		},
	}
	w.players["player:alice"] = model.Player{
		ID:         "player:alice",
		CreatureID: "creature:alice",
		Metadata: model.Metadata{
			Tags: []string{"PWIMPY"},
		},
	}

	w.creatures["creature:monster"] = model.Creature{
		ID:     "creature:monster",
		RoomID: "room:1",
		Kind:   model.CreatureKindMonster,
		Stats: map[string]int{
			"hpCurrent": 50,
		},
	}

	UpdatePlayerStatuses(w, 1000)

	// Since hpCurrent (15) <= wimpyValue (20) and there is a threat (creature:monster),
	// the player should trigger flee command "도망".
	if len(w.dispatchedCommands) != 1 {
		t.Fatalf("expected 1 auto-flee command, got %d", len(w.dispatchedCommands))
	}
	expectedCmd := "s1:player:alice:도망"
	if w.dispatchedCommands[0] != expectedCmd {
		t.Errorf("expected command %q, got %q", expectedCmd, w.dispatchedCommands[0])
	}
}

func TestWimpyAutoFleeTick_NoThreat(t *testing.T) {
	w := newMockUpdatePlyWorld()
	w.activeSessions = []ActiveSession{{ID: "s1", ActorID: "player:alice"}}
	w.sessionActors["s1"] = struct {
		creatureID model.CreatureID
		playerID   model.PlayerID
		ok         bool
	}{creatureID: "creature:alice", playerID: "player:alice", ok: true}

	w.rooms["room:1"] = model.Room{
		ID:          "room:1",
		CreatureIDs: []model.CreatureID{"creature:alice"},
	}

	w.creatures["creature:alice"] = model.Creature{
		ID:     "creature:alice",
		RoomID: "room:1",
		Metadata: model.Metadata{
			Tags: []string{"PWIMPY"},
		},
		Stats: map[string]int{
			"class":      legacyClassBarbarian,
			"hpCurrent":  15,
			"hpMax":      100,
			"wimpyValue": 20,
		},
	}
	w.players["player:alice"] = model.Player{
		ID:         "player:alice",
		CreatureID: "creature:alice",
		Metadata: model.Metadata{
			Tags: []string{"PWIMPY"},
		},
	}

	UpdatePlayerStatuses(w, 1000)

	if len(w.dispatchedCommands) != 0 {
		t.Errorf("expected 0 auto-flee commands without threat, got %d", len(w.dispatchedCommands))
	}
}
