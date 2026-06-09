package state

import (
	"errors"
	"fmt"
	"log"
	"maps"
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"muhan/internal/world/load"
	"muhan/internal/world/model"
)

const (
	movePlayerDMClass = 13

	legacyObjectRandomEnchantmentFlagBit = 21

	stateLegacyClassAssassin   = 1
	stateLegacyClassBarbarian  = 2
	stateLegacyClassCleric     = 3
	stateLegacyClassFighter    = 4
	stateLegacyClassMage       = 5
	stateLegacyClassPaladin    = 6
	stateLegacyClassRanger     = 7
	stateLegacyClassThief      = 8
	stateLegacyClassInvincible = 9
	stateLegacyClassCaretaker  = 10
	stateLegacyClassBulsa      = 11
	stateLegacyClassSubDM      = 12
	stateLegacyClassDM         = 13

	stateCreatureDescriptionProperty = "description"
	stateCreaturePasswordHashKey     = "legacyPasswordHash"
	stateSuicidePendingTag           = "suicidePending"
	stateSuicideRequestedAtProperty  = "suicideRequestedAt"

	exitLTimeIntervalRawField = "ltime.interval"
	exitLTimeLTimeRawField    = "ltime.ltime"
	exitLTimeMiscRawField     = "ltime.misc"
)

// TrapState holds dynamic per-room trap simulation data for tick fidelity (P1-3).
// Static trap effects are executed by the movement trap handler; this runtime
// slot is reserved for dynamic counters/timers if deeper room.c traffic or
// disarm-window parity needs state that does not belong in static room data.
// Runtime only (not saved), matching legacy transient state.
type TrapState struct {
	TriggeredCount int
	LastTriggered  int64
	DisarmedUntil  int64 // unix time; 0 = armed
	// Extend here for future exact C trap effects (damage pools, auto-reset).
}

// World is a mutable runtime view of the loaded world data.
type World struct {
	mu                sync.RWMutex
	rooms             map[model.RoomID]model.Room
	players           map[model.PlayerID]model.Player
	creatures         map[model.CreatureID]model.Creature
	families          map[int]model.Family
	banks             map[model.BankID]model.BankAccount
	objects           map[model.ObjectInstanceID]model.ObjectInstance
	prototypes        map[model.PrototypeID]model.ObjectPrototype
	marriageInvites   map[model.SpecialID][]string
	cooldowns         map[model.CreatureID]map[string]int64
	monsterDamage     map[model.CreatureID]map[model.CreatureID]int
	familyWar         FamilyWarSnapshot
	spies             map[model.PlayerID]model.PlayerID
	enemies           map[model.CreatureID][]string
	effectExpirations map[model.CreatureID]map[string]int64
	lockouts          []LockoutEntry

	// Tick & Simulation Fidelity extensions (historical P1-3 / Package 3/6 marker cleaned).
	// Small additions ONLY for new tick data as permitted:
	// - lightTimers: per-creature light source decay timers (supplements object
	//   "shotsCurrent" for carried lightsources; allows precise per-tick timing
	//   independent of 20s player update if needed in future).
	// - trapStates: optional dynamic per-room trap state (trigger counts, disarm
	//   timers, reset semantics) for deeper legacy room.c traffic behavior
	//   without polluting static room.Properties.
	// These are runtime-only (not persisted, reset on load), matching C statics
	// + active list side effects. Initialized empty in NewWorld.
	lightTimers map[model.CreatureID]map[string]int64
	trapStates  map[model.RoomID]TrapState

	BroadcastAllFunc         func(string) error
	UpdateActiveMonstersFunc func(t int64) error
	UpdatePlayerStatusesFunc func(t int64) error
	UpdateRandomSpawnsFunc   func(t int64) error
	UpdateTimeClockFunc      func(t int64) error
	UpdateTimedExitsFunc     func(t int64) error
	UpdateShutdownFunc       func(t int64) error
	RecalculateACFunc        func(model.CreatureID) error
	RecalculateTHACOFunc     func(model.CreatureID) error
	dbRoot                   string
	shutdownLTime            int64
	shutdownInterval         int64
	lastShutdownUpdate       int64
	lastActiveUpdate         int64
	lastPlayerUpdate         int64
	lastRandomUpdate         int64
	lastTimeUpdate           int64
	lastExitUpdate           int64
	legacyTime               int64
	randomUpdateInterval     int64
	txInterval               int64

	// B: Dirty tracking for efficient persistence (player/bank last change time).
	// Protected by dedicated dirtyMu (not world.mu) to:
	// - Prevent deadlocks when Mark*Dirty called from inside mutation critical sections (world.mu held).
	// - Reduce contention on hot world lock for high-frequency marks (gold, hp, inventory moves).
	playerDirty map[model.PlayerID]int64
	bankDirty   map[model.BankID]int64
	// D: Room floor objects dirty tracking (biggest remaining durability gap from review).
	// When objects move to/from a room (drop, get from ground, corpse drop, gold drop to room etc.)
	// we mark the room so its floor contents (including containers' contents) get persisted in sidecar.
	roomObjectDirty map[model.RoomID]int64
	// C: Board posts + family news dirty tracking (Package C runtime persistence).
	// Marked at mutation time (new post via appendBoardPost, toggle delete, family news change).
	// Enables FlushDirtyBoardsAndFamilyNews + sidecar JSON + startup restore (like B/D).
	boardDirty      map[string]int64 // legacy board dir e.g. "info", "family1", "user", "notice", "family"
	familyNewsDirty map[int]int64    // family ID (1..N)
	dirtyMu         sync.Mutex       // protects only the *Dirty maps

	// B: Minimal background save queue (C phase start)
	saveQueue chan saveRequest
}

type saveRequest struct {
	playerID model.PlayerID
	bankID   model.BankID
	boardDir string // C: for board posts sidecar
	familyID int    // C: for family news sidecar
	done     chan struct{}
}

// BroadcastAll broadcasts a message to all connected sessions.
func (w *World) BroadcastAll(message string) error {
	w.mu.RLock()
	fn := w.BroadcastAllFunc
	w.mu.RUnlock()
	if fn != nil {
		return fn(message)
	}
	return nil
}

func (w *World) RecalculateAC(creatureID model.CreatureID) error {
	if w == nil {
		return fmt.Errorf("recalculate creature %q ac: world state is nil", creatureID)
	}
	w.mu.RLock()
	fn := w.RecalculateACFunc
	w.mu.RUnlock()
	if fn != nil {
		return fn(creatureID)
	}
	_, err := w.RecalculateCreatureAC(creatureID)
	return err
}

func (w *World) RecalculateTHACO(creatureID model.CreatureID) error {
	if w == nil {
		return fmt.Errorf("recalculate creature %q thaco: world state is nil", creatureID)
	}
	w.mu.RLock()
	fn := w.RecalculateTHACOFunc
	w.mu.RUnlock()
	if fn != nil {
		return fn(creatureID)
	}
	_, err := w.RecalculateCreatureTHACO(creatureID)
	return err
}

func (w *World) GetEffectExpiration(creatureID model.CreatureID, tag string) (int64, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	m, ok := w.effectExpirations[creatureID]
	if !ok {
		return 0, false
	}
	expires, ok := m[tag]
	return expires, ok
}

func (w *World) SetEffectExpiration(creatureID model.CreatureID, tag string, expires int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	m, ok := w.effectExpirations[creatureID]
	if !ok {
		m = map[string]int64{}
		w.effectExpirations[creatureID] = m
	}
	m[tag] = expires
}

func (w *World) ShutdownSchedule() (ltime int64, interval int64) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.shutdownLTime, w.shutdownInterval
}

// MarkPlayerDirty marks a player as needing persistence.
// This should be called at mutation time (preferred over marking inside Save*).
func (w *World) MarkPlayerDirty(pid model.PlayerID) {
	if pid.IsZero() {
		return
	}
	w.dirtyMu.Lock()
	if w.playerDirty == nil {
		w.playerDirty = make(map[model.PlayerID]int64)
	}
	w.playerDirty[pid] = time.Now().Unix()
	w.dirtyMu.Unlock()
}

// MarkBankDirty marks a bank as needing persistence.
func (w *World) MarkBankDirty(bid model.BankID) {
	if bid == "" {
		return
	}
	w.dirtyMu.Lock()
	if w.bankDirty == nil {
		w.bankDirty = make(map[model.BankID]int64)
	}
	w.bankDirty[bid] = time.Now().Unix()
	w.dirtyMu.Unlock()
}

// MarkRoomObjectsDirty marks a room as having its floor objects (including any
// containers dropped on the ground and their recursive contents) changed.
// Called from MoveObject when crossing room boundary, DropCreatureGoldToRoom,
// death corpse/item scatter, etc. (D phase - floor persistence)
func (w *World) MarkRoomObjectsDirty(rid model.RoomID) {
	if rid.IsZero() {
		return
	}
	w.dirtyMu.Lock()
	if w.roomObjectDirty == nil {
		w.roomObjectDirty = make(map[model.RoomID]int64)
	}
	w.roomObjectDirty[rid] = time.Now().Unix()
	w.dirtyMu.Unlock()
}

// MarkBoardDirty marks a board (by its legacy dir name) as having runtime post changes
// (new post or delete toggle). Used for Package C JSON sidecar dirty flush.
func (w *World) MarkBoardDirty(boardDir string) {
	if boardDir == "" {
		return
	}
	w.dirtyMu.Lock()
	if w.boardDirty == nil {
		w.boardDirty = make(map[string]int64)
	}
	w.boardDirty[boardDir] = time.Now().Unix()
	w.dirtyMu.Unlock()
}

// MarkFamilyNewsDirty marks a family's news as changed (for JSON sidecar persistence).
func (w *World) MarkFamilyNewsDirty(familyID int) {
	if familyID <= 0 {
		return
	}
	w.dirtyMu.Lock()
	if w.familyNewsDirty == nil {
		w.familyNewsDirty = make(map[int]int64)
	}
	w.familyNewsDirty[familyID] = time.Now().Unix()
	w.dirtyMu.Unlock()
}

func (w *World) LastShutdownUpdate() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastShutdownUpdate
}

func (w *World) SetLastShutdownUpdate(t int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastShutdownUpdate = t
}

// SetDBRoot sets the database root path for dynamic loading.
func (w *World) SetDBRoot(root string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.dbRoot = root
}

// DBRoot returns the database root path.
func (w *World) DBRoot() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.dbRoot
}

// --- Tick data accessors (light timers, trap state; historical P1-3 cleaned) ---
// These provide the documented small extension point. Currently read/write
// for runtime lighting and optional dynamic trap counters.
// Not used by core tick hooks yet to minimize delta.

func (w *World) GetLightTimer(creatureID model.CreatureID, key string) (int64, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if m := w.lightTimers[creatureID]; m != nil {
		v, ok := m[key]
		return v, ok
	}
	return 0, false
}

func (w *World) SetLightTimer(creatureID model.CreatureID, key string, expires int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.lightTimers == nil {
		w.lightTimers = map[model.CreatureID]map[string]int64{}
	}
	m := w.lightTimers[creatureID]
	if m == nil {
		m = map[string]int64{}
		w.lightTimers[creatureID] = m
	}
	m[key] = expires
}

func (w *World) GetTrapState(roomID model.RoomID) TrapState {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.trapStates == nil {
		return TrapState{}
	}
	return w.trapStates[roomID]
}

func (w *World) SetTrapState(roomID model.RoomID, st TrapState) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.trapStates == nil {
		w.trapStates = map[model.RoomID]TrapState{}
	}
	w.trapStates[roomID] = st
}

var (
	ErrFamilyWarInvalidFamily = errors.New("family war: family id must be positive")
	ErrFamilyWarSelf          = errors.New("family war: family cannot declare war on itself")
	ErrFamilyWarActive        = errors.New("family war: war is already active")
	ErrFamilyWarPending       = errors.New("family war: another war request is pending")
	ErrFamilyWarNoPending     = errors.New("family war: no pending war request")
	ErrFamilyWarNotRequester  = errors.New("family war: family did not request the pending war")
	ErrFamilyWarNotTarget     = errors.New("family war: family is not the target of the pending war")
)

// FamilyWarPair identifies two legacy family numbers participating in a war
// transition. For pending requests First is the caller and Second is the target.
// For active wars the order preserves the legacy AT_WAR encoding.
type FamilyWarPair struct {
	First  int
	Second int
}

// IsZero reports whether pair is unset.
func (p FamilyWarPair) IsZero() bool {
	return p.First == 0 && p.Second == 0
}

// FamilyWarSnapshot is the runtime equivalent of legacy AT_WAR/CALLWAR1/CALLWAR2.
type FamilyWarSnapshot struct {
	Active  FamilyWarPair
	Pending FamilyWarPair
}

// AtWar reports whether a family war is active.
func (s FamilyWarSnapshot) AtWar() bool {
	return !s.Active.IsZero()
}

// HasPending reports whether a family war request is waiting for acceptance.
func (s FamilyWarSnapshot) HasPending() bool {
	return !s.Pending.IsZero()
}

// LegacyValues returns the legacy AT_WAR, CALLWAR1, and CALLWAR2 integer values.
func (s FamilyWarSnapshot) LegacyValues() (atWar int, callWar1 int, callWar2 int) {
	if !s.Active.IsZero() {
		atWar = s.Active.First*16 + s.Active.Second
	}
	if !s.Pending.IsZero() {
		callWar1 = s.Pending.First
		callWar2 = s.Pending.Second
	}
	return atWar, callWar1, callWar2
}

// New returns a runtime world state copied from src.
func New(src *load.World) *World {
	return NewWorld(src)
}

// NewWorld returns a runtime world state copied from src.
func NewWorld(src *load.World) *World {
	w := &World{
		rooms:             map[model.RoomID]model.Room{},
		players:           map[model.PlayerID]model.Player{},
		creatures:         map[model.CreatureID]model.Creature{},
		families:          map[int]model.Family{},
		banks:             map[model.BankID]model.BankAccount{},
		objects:           map[model.ObjectInstanceID]model.ObjectInstance{},
		prototypes:        map[model.PrototypeID]model.ObjectPrototype{},
		marriageInvites:   map[model.SpecialID][]string{},
		cooldowns:         map[model.CreatureID]map[string]int64{},
		monsterDamage:     map[model.CreatureID]map[model.CreatureID]int{},
		spies:             map[model.PlayerID]model.PlayerID{},
		enemies:           map[model.CreatureID][]string{},
		effectExpirations: map[model.CreatureID]map[string]int64{},
		// tick data inits (light timers, trap state) -- see struct docs. (P1-3 historical)
		lightTimers:          map[model.CreatureID]map[string]int64{},
		trapStates:           map[model.RoomID]TrapState{},
		randomUpdateInterval: 20,
		txInterval:           3600,

		// B/D: Dirty tracking init (player, bank, room floor objects)
		playerDirty:     make(map[model.PlayerID]int64),
		bankDirty:       make(map[model.BankID]int64),
		roomObjectDirty: make(map[model.RoomID]int64),
		// C: Board + family news dirty init
		boardDirty:      make(map[string]int64),
		familyNewsDirty: make(map[int]int64),

		// B: Start minimal background saver
		saveQueue: make(chan saveRequest, 128),
	}
	go w.backgroundSaver()
	if src == nil {
		return w
	}

	for id, room := range src.Rooms {
		w.rooms[id] = cloneRoom(room)
	}
	for id, player := range src.Players {
		w.players[id] = clonePlayer(player)
	}
	for id, creature := range src.Creatures {
		w.creatures[id] = cloneCreature(creature)
	}
	for id, family := range src.Families {
		w.families[id] = cloneFamily(family)
	}
	for id, account := range src.Banks {
		w.banks[id] = cloneBankAccount(account)
	}
	for id, object := range src.Objects {
		w.objects[id] = cloneObject(object)
	}
	for id, proto := range src.ObjectPrototypes {
		w.prototypes[id] = cloneObjectPrototype(proto)
	}
	for id, names := range src.MarriageInvites {
		w.marriageInvites[id] = slices.Clone(names)
	}
	w.reconcileRoomOccupants()
	return w
}

// Room returns a copy of the room with id.
func (w *World) Room(id model.RoomID) (model.Room, bool) {
	if w == nil {
		return model.Room{}, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()

	room, ok := w.rooms[id]
	if !ok {
		return model.Room{}, false
	}
	return cloneRoom(room), true
}

// AllRoomIDs returns a slice of all room IDs in the world.
func (w *World) AllRoomIDs() []model.RoomID {
	if w == nil {
		return nil
	}
	w.mu.RLock()
	defer w.mu.RUnlock()

	ids := make([]model.RoomID, 0, len(w.rooms))
	for id := range w.rooms {
		ids = append(ids, id)
	}
	return ids
}

// GetRoom returns a copy of the room with id.
func (w *World) GetRoom(id model.RoomID) (model.Room, bool) {
	return w.Room(id)
}

// Player returns a copy of the player with id.
func (w *World) Player(id model.PlayerID) (model.Player, bool) {
	if w == nil {
		return model.Player{}, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()

	player, ok := w.players[id]
	if !ok {
		return model.Player{}, false
	}
	return clonePlayer(player), true
}

// GetPlayer returns a copy of the player with id.
func (w *World) GetPlayer(id model.PlayerID) (model.Player, bool) {
	return w.Player(id)
}

// Creature returns a copy of the creature with id.
func (w *World) Creature(id model.CreatureID) (model.Creature, bool) {
	if w == nil {
		return model.Creature{}, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()

	creature, ok := w.creatures[id]
	if !ok {
		return model.Creature{}, false
	}
	return cloneCreature(creature), true
}

// GetCreature returns a copy of the creature with id.
func (w *World) GetCreature(id model.CreatureID) (model.Creature, bool) {
	return w.Creature(id)
}

// Family returns a copy of the legacy family registry row with id.
func (w *World) Family(familyID int) (model.Family, bool) {
	if w == nil {
		return model.Family{}, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()

	family, ok := w.families[familyID]
	if !ok {
		return model.Family{}, false
	}
	return cloneFamily(family), true
}

// Families returns all legacy family registry rows sorted by slot and id.
func (w *World) Families() []model.Family {
	if w == nil {
		return nil
	}
	w.mu.RLock()
	defer w.mu.RUnlock()

	families := make([]model.Family, 0, len(w.families))
	for _, family := range w.families {
		families = append(families, cloneFamily(family))
	}
	slices.SortStableFunc(families, func(a, b model.Family) int {
		if a.Slot != b.Slot {
			return a.Slot - b.Slot
		}
		return a.ID - b.ID
	})
	return families
}

// FamilyDisplayName returns the legacy family_str value for familyID.
func (w *World) FamilyDisplayName(familyID int) (string, bool) {
	family, ok := w.Family(familyID)
	if !ok || strings.TrimSpace(family.DisplayName) == "" {
		return "", false
	}
	return family.DisplayName, true
}

// FamilyIDByDisplayName returns the legacy family number for an exact name.
func (w *World) FamilyIDByDisplayName(name string) (int, bool) {
	if w == nil {
		return 0, false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()

	ids := make([]int, 0, len(w.families))
	for id := range w.families {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if strings.TrimSpace(w.families[id].DisplayName) == name {
			return id, true
		}
	}
	return 0, false
}

// FamilyBossName returns the legacy fmboss_str value for familyID.
func (w *World) FamilyBossName(familyID int) (string, bool) {
	family, ok := w.Family(familyID)
	if !ok || strings.TrimSpace(family.BossName) == "" {
		return "", false
	}
	return family.BossName, true
}

// Bank returns a copy of the bank account with id.
func (w *World) Bank(id model.BankID) (model.BankAccount, bool) {
	if w == nil {
		return model.BankAccount{}, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()

	account, ok := w.banks[id]
	if !ok {
		return model.BankAccount{}, false
	}
	return cloneBankAccount(account), true
}

// GetBank returns a copy of the bank account with id.
func (w *World) GetBank(id model.BankID) (model.BankAccount, bool) {
	return w.Bank(id)
}

// EnsurePlayerBankRoot creates the player's value-backed bank root when it does
// not exist, mirroring legacy load_bank() callers that materialize an empty bank.
func (w *World) EnsurePlayerBankRoot(playerID model.PlayerID) (model.BankAccount, model.ObjectInstance, error) {
	if w == nil {
		return model.BankAccount{}, model.ObjectInstance{}, fmt.Errorf("ensure player bank root: world state is nil")
	}
	if playerID.IsZero() {
		return model.BankAccount{}, model.ObjectInstance{}, fmt.Errorf("ensure player bank root: player id is required")
	}

	w.mu.Lock()
	player, ok := w.players[playerID]
	if !ok {
		w.mu.Unlock()
		return model.BankAccount{}, model.ObjectInstance{}, fmt.Errorf("ensure player bank root: player %q not found", playerID)
	}
	ownerName := statePlayerBankOwnerName(player, playerID)
	bankID := model.BankID("bank:player:" + ownerName)
	if account, ok := w.banks[bankID]; ok {
		for _, objectID := range account.Objects.ObjectIDs {
			if objectID.IsZero() {
				continue
			}
			if root, found := w.objects[objectID]; found {
				w.mu.Unlock()
				return cloneBankAccount(account), cloneObject(root), nil
			}
		}
	}

	protoID := model.PrototypeID("proto:bank-root")
	if _, ok := w.prototypes[protoID]; !ok {
		w.prototypes[protoID] = model.ObjectPrototype{
			ID:          protoID,
			Kind:        model.ObjectKindContainer,
			DisplayName: "보관함",
			Properties: map[string]string{
				"kind":   string(model.ObjectKindContainer),
				"OCONTN": "1",
			},
		}
	}
	rootID := w.nextPlayerBankRootIDLocked(ownerName)
	root := model.ObjectInstance{
		ID:          rootID,
		PrototypeID: protoID,
		Location:    model.ObjectLocation{BankID: bankID, Slot: "bank"},
		Properties: map[string]string{
			"value":        "0",
			"shotsCurrent": "0",
			"shotsMax":     "200",
			"kind":         string(model.ObjectKindContainer),
			"OCONTN":       "1",
		},
	}
	account := model.BankAccount{
		ID:            bankID,
		Kind:          "player",
		OwnerName:     ownerName,
		OwnerPlayerID: playerID,
		Objects:       model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{rootID}},
	}
	w.objects[rootID] = root
	w.banks[bankID] = account
	w.mu.Unlock()

	w.MarkBankDirty(bankID)
	return cloneBankAccount(account), cloneObject(root), nil
}

func statePlayerBankOwnerName(player model.Player, fallback model.PlayerID) string {
	for _, candidate := range []string{
		string(player.ID),
		strings.TrimSpace(player.DisplayName),
		strings.TrimSpace(player.AccountName),
		strings.TrimSpace(player.Metadata.LegacyID),
		string(fallback),
	} {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate)
		}
	}
	return string(fallback)
}

func (w *World) nextPlayerBankRootIDLocked(ownerName string) model.ObjectInstanceID {
	base := model.ObjectInstanceID("object:bank-root:" + ownerName)
	if _, exists := w.objects[base]; !exists {
		return base
	}
	for i := 2; ; i++ {
		candidate := model.ObjectInstanceID(fmt.Sprintf("%s:%d", base, i))
		if _, exists := w.objects[candidate]; !exists {
			return candidate
		}
	}
}

// EnsureFamilyBankRoot creates the family bank root for the given special bucket
// when it does not exist, mirroring legacy load_family_bank() materialization.
func (w *World) EnsureFamilyBankRoot(familyID int, special int) (model.BankAccount, model.ObjectInstance, error) {
	if w == nil {
		return model.BankAccount{}, model.ObjectInstance{}, fmt.Errorf("ensure family bank root: world state is nil")
	}
	if familyID <= 0 {
		return model.BankAccount{}, model.ObjectInstance{}, fmt.Errorf("ensure family bank root: family id is required")
	}

	w.mu.Lock()
	ownerName := w.stateFamilyBankOwnerNameLocked(familyID, special)
	bankID := model.BankID("bank:family:" + ownerName)
	if account, ok := w.banks[bankID]; ok {
		for _, objectID := range account.Objects.ObjectIDs {
			if objectID.IsZero() {
				continue
			}
			if root, found := w.objects[objectID]; found {
				w.mu.Unlock()
				return cloneBankAccount(account), cloneObject(root), nil
			}
		}
	}

	protoID := model.PrototypeID("proto:family-bank-root")
	displayName := "패거리 창고"
	if special == 0 {
		displayName = "패거리 금고"
	}
	if _, ok := w.prototypes[protoID]; !ok {
		w.prototypes[protoID] = model.ObjectPrototype{
			ID:          protoID,
			Kind:        model.ObjectKindContainer,
			DisplayName: displayName,
			Properties: map[string]string{
				"kind":   string(model.ObjectKindContainer),
				"OCONTN": "1",
			},
		}
	}
	rootID := w.nextFamilyBankRootIDLocked(ownerName)
	root := model.ObjectInstance{
		ID:          rootID,
		PrototypeID: protoID,
		Location:    model.ObjectLocation{BankID: bankID, Slot: "bank"},
		Properties: map[string]string{
			"value":        "0",
			"shotsCurrent": "0",
			"shotsMax":     "200",
			"kind":         string(model.ObjectKindContainer),
			"OCONTN":       "1",
		},
	}
	account := model.BankAccount{
		ID:        bankID,
		Kind:      "family",
		OwnerName: ownerName,
		Objects:   model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{rootID}},
	}
	w.objects[rootID] = root
	w.banks[bankID] = account
	w.mu.Unlock()

	w.MarkBankDirty(bankID)
	return cloneBankAccount(account), cloneObject(root), nil
}

func (w *World) stateFamilyBankOwnerNameLocked(familyID int, special int) string {
	base := ""
	for _, family := range w.families {
		if family.ID == familyID || family.Slot == familyID {
			base = strings.TrimSpace(family.DisplayName)
			break
		}
	}
	if base == "" {
		base = strconv.Itoa(familyID)
	}
	return fmt.Sprintf("%s_%d", base, special)
}

func (w *World) nextFamilyBankRootIDLocked(ownerName string) model.ObjectInstanceID {
	base := model.ObjectInstanceID("object:family-bank-root:" + ownerName)
	if _, exists := w.objects[base]; !exists {
		return base
	}
	for i := 2; ; i++ {
		candidate := model.ObjectInstanceID(fmt.Sprintf("%s:%d", base, i))
		if _, exists := w.objects[candidate]; !exists {
			return candidate
		}
	}
}

// Object returns a copy of the object instance with id.
func (w *World) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	if w == nil {
		return model.ObjectInstance{}, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()

	object, ok := w.objects[id]
	if !ok {
		return model.ObjectInstance{}, false
	}
	return cloneObject(object), true
}

// GetObject returns a copy of the object instance with id.
func (w *World) GetObject(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	return w.Object(id)
}

// ObjectPrototype returns a copy of the object prototype with id.
func (w *World) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	if w == nil {
		return model.ObjectPrototype{}, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()

	proto, ok := w.prototypes[id]
	if !ok {
		return model.ObjectPrototype{}, false
	}
	return cloneObjectPrototype(proto), true
}

// GetObjectPrototype returns a copy of the object prototype with id.
func (w *World) GetObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	return w.ObjectPrototype(id)
}

// HasMarriageInvite reports whether playerID is invited for a marriage/special room ID.
func (w *World) HasMarriageInvite(playerID model.PlayerID, specialID model.SpecialID) bool {
	if w == nil || specialID.IsZero() {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()

	player, ok := w.players[playerID]
	if !ok {
		return false
	}
	return w.hasMarriageInviteLocked(player, specialID)
}

// MarriageInvites returns a copy of the runtime invite names for a marriage/special room ID.
func (w *World) MarriageInvites(specialID model.SpecialID) []string {
	if w == nil || specialID.IsZero() {
		return nil
	}
	w.mu.RLock()
	defer w.mu.RUnlock()

	return slices.Clone(w.marriageInvites[specialID])
}

// AddMarriageInvite appends name to a runtime marriage/special room invite list.
// It returns false when the trimmed name is already present.
func (w *World) AddMarriageInvite(specialID model.SpecialID, name string) (bool, error) {
	if w == nil {
		return false, fmt.Errorf("add marriage invite %q: world state is nil", specialID)
	}
	if specialID.IsZero() {
		return false, fmt.Errorf("add marriage invite: special id is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false, fmt.Errorf("add marriage invite %q: name is required", specialID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.marriageInvites == nil {
		w.marriageInvites = map[model.SpecialID][]string{}
	}
	for _, existing := range w.marriageInvites[specialID] {
		if strings.TrimSpace(existing) == name {
			return false, nil
		}
	}
	w.marriageInvites[specialID] = append(w.marriageInvites[specialID], name)
	return true, nil
}

// RemoveMarriageInvite removes name from a runtime marriage/special room invite list.
// It returns false when the trimmed name is not present.
func (w *World) RemoveMarriageInvite(specialID model.SpecialID, name string) (bool, error) {
	if w == nil {
		return false, fmt.Errorf("remove marriage invite %q: world state is nil", specialID)
	}
	if specialID.IsZero() {
		return false, fmt.Errorf("remove marriage invite: special id is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false, fmt.Errorf("remove marriage invite %q: name is required", specialID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	invites := w.marriageInvites[specialID]
	match := -1
	for i, existing := range invites {
		if strings.TrimSpace(existing) == name {
			match = i
		}
	}
	if match < 0 {
		return false, nil
	}
	invites = append(invites[:match], invites[match+1:]...)
	if len(invites) == 0 {
		delete(w.marriageInvites, specialID)
	} else {
		w.marriageInvites[specialID] = invites
	}
	return true, nil
}

// FamilyWarSnapshot returns a copy of the current family war state.
func (w *World) FamilyWarSnapshot() FamilyWarSnapshot {
	if w == nil {
		return FamilyWarSnapshot{}
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.familyWar
}

// FamilyWarLegacyValues returns the current AT_WAR/CALLWAR1/CALLWAR2 equivalents.
func (w *World) FamilyWarLegacyValues() (atWar int, callWar1 int, callWar2 int) {
	return w.FamilyWarSnapshot().LegacyValues()
}

// RequestFamilyWar creates a pending family war request under the world lock.
func (w *World) RequestFamilyWar(callerFamily, targetFamily int) (FamilyWarSnapshot, error) {
	if w == nil {
		return FamilyWarSnapshot{}, fmt.Errorf("request family war: world state is nil")
	}
	if err := validateFamilyWarPair(callerFamily, targetFamily); err != nil {
		return FamilyWarSnapshot{}, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.familyWar.Active.IsZero() {
		return w.familyWar, ErrFamilyWarActive
	}
	if !w.familyWar.Pending.IsZero() {
		return w.familyWar, ErrFamilyWarPending
	}
	w.familyWar.Pending = FamilyWarPair{First: callerFamily, Second: targetFamily}
	return w.familyWar, nil
}

// AcceptFamilyWar accepts a pending family war request under the world lock.
// acceptingFamily is the pending target and callerFamily is the pending caller.
func (w *World) AcceptFamilyWar(acceptingFamily, callerFamily int) (FamilyWarSnapshot, error) {
	if w == nil {
		return FamilyWarSnapshot{}, fmt.Errorf("accept family war: world state is nil")
	}
	if err := validateFamilyWarPair(acceptingFamily, callerFamily); err != nil {
		return FamilyWarSnapshot{}, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.familyWar.Active.IsZero() {
		return w.familyWar, ErrFamilyWarActive
	}
	if w.familyWar.Pending.IsZero() {
		return w.familyWar, ErrFamilyWarNoPending
	}
	if w.familyWar.Pending.Second != acceptingFamily {
		return w.familyWar, ErrFamilyWarNotTarget
	}
	if w.familyWar.Pending.First != callerFamily {
		return w.familyWar, ErrFamilyWarPending
	}
	w.familyWar.Active = FamilyWarPair{First: acceptingFamily, Second: callerFamily}
	w.familyWar.Pending = FamilyWarPair{}
	return w.familyWar, nil
}

// CancelFamilyWar cancels a pending family war request by its original caller.
func (w *World) CancelFamilyWar(callerFamily int) (FamilyWarSnapshot, error) {
	if w == nil {
		return FamilyWarSnapshot{}, fmt.Errorf("cancel family war: world state is nil")
	}
	if callerFamily <= 0 {
		return FamilyWarSnapshot{}, ErrFamilyWarInvalidFamily
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.familyWar.Pending.IsZero() {
		return w.familyWar, ErrFamilyWarNoPending
	}
	if w.familyWar.Pending.First != callerFamily {
		return w.familyWar, ErrFamilyWarNotRequester
	}
	w.familyWar.Pending = FamilyWarPair{}
	return w.familyWar, nil
}

// ClearFamilyWar clears both active and pending family war state. This mirrors
// the legacy boss-death reset of AT_WAR/CALLWAR1/CALLWAR2.
func (w *World) ClearFamilyWar() FamilyWarSnapshot {
	if w == nil {
		return FamilyWarSnapshot{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	w.familyWar = FamilyWarSnapshot{}
	return w.familyWar
}

// EndActiveFamilyWar ends current war and returns the final active snapshot.
func (w *World) EndActiveFamilyWar(reason string) FamilyWarSnapshot {
	if w == nil {
		return FamilyWarSnapshot{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.familyWar.Active.IsZero() {
		return w.familyWar
	}
	snap := w.familyWar
	w.familyWar = FamilyWarSnapshot{}
	return snap
}

// FamiliesAtWar reports whether the two family ids match the active war.
func (w *World) FamiliesAtWar(firstFamily, secondFamily int) bool {
	if w == nil || firstFamily <= 0 || secondFamily <= 0 {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()

	active := w.familyWar.Active
	if active.IsZero() {
		return false
	}
	return (firstFamily == active.First && secondFamily == active.Second) ||
		(firstFamily == active.Second && secondFamily == active.First)
}

// FamilyAtWar reports whether family id is one side of the active war.
func (w *World) FamilyAtWar(family int) bool {
	if w == nil || family <= 0 {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()

	active := w.familyWar.Active
	return family == active.First || family == active.Second
}

// MovePlayer moves playerID through the named exit from the player's current room.
func (w *World) MovePlayer(playerID model.PlayerID, exitName string) error {
	if w == nil {
		return fmt.Errorf("move player %q: world state is nil", playerID)
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	player, ok := w.players[playerID]
	if !ok {
		return fmt.Errorf("move player %q: player not found", playerID)
	}
	if player.RoomID.IsZero() {
		return fmt.Errorf("move player %q: player has no room", playerID)
	}

	fromRoom, ok := w.rooms[player.RoomID]
	if !ok {
		return fmt.Errorf("move player %q: current room %q not found", playerID, player.RoomID)
	}

	exit, ok := findExit(fromRoom, exitName)
	if !ok {
		return fmt.Errorf("move player %q: exit %q not found in room %q", playerID, exitName, fromRoom.ID)
	}
	if flag, blocked := blockedMoveExitFlag(exit); blocked {
		return fmt.Errorf("move player %q: exit %q blocked by flag %q", playerID, exitName, flag)
	}
	toRoom, ok := w.rooms[exit.ToRoomID]
	if !ok {
		return fmt.Errorf("move player %q: target room %q not found", playerID, exit.ToRoomID)
	}

	var creature model.Creature
	hasCreature := false
	if !player.CreatureID.IsZero() {
		creature, ok = w.creatures[player.CreatureID]
		if !ok {
			return fmt.Errorf("move player %q: linked creature %q not found", playerID, player.CreatureID)
		}
		if !creature.PlayerID.IsZero() && creature.PlayerID != player.ID {
			return fmt.Errorf("move player %q: linked creature %q belongs to player %q", playerID, creature.ID, creature.PlayerID)
		}
		hasCreature = true
	}

	if err := w.validateMovePlayerRestrictions(player, exitName, exit, toRoom, creature, hasCreature); err != nil {
		return err
	}

	player.RoomID = exit.ToRoomID
	w.players[player.ID] = player

	if hasCreature {
		creature.RoomID = exit.ToRoomID
		w.creatures[creature.ID] = creature
	}

	for roomID, room := range w.rooms {
		room.PlayerIDs = removeID(room.PlayerIDs, player.ID)
		if hasCreature {
			room.CreatureIDs = removeID(room.CreatureIDs, creature.ID)
		}
		w.rooms[roomID] = room
	}

	toRoom = w.rooms[exit.ToRoomID]
	toRoom.PlayerIDs = w.insertPlayerIDLegacySortedLocked(toRoom.PlayerIDs, player.ID)
	if hasCreature {
		toRoom.CreatureIDs = w.insertCreatureIDLegacySortedLocked(toRoom.CreatureIDs, creature.ID)
	}
	w.rooms[toRoom.ID] = toRoom
	nowUnix := time.Now().Unix()
	w.refreshRoomPermanentSpawnsLocked(toRoom.ID, nowUnix)
	w.checkRoomExitsLocked(toRoom.ID, nowUnix)

	return nil
}

// MovePlayerToRoom moves playerID directly to roomID, preserving the linked
// creature and room occupant indexes. It is used by command effects such as
// legacy recall/teleport that do not traverse an exit.
func (w *World) MovePlayerToRoom(playerID model.PlayerID, roomID model.RoomID) error {
	if w == nil {
		return fmt.Errorf("move player %q to room %q: world state is nil", playerID, roomID)
	}
	if playerID.IsZero() {
		return fmt.Errorf("move player to room %q: player id is required", roomID)
	}
	if roomID.IsZero() {
		return fmt.Errorf("move player %q to room: room id is required", playerID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	player, ok := w.players[playerID]
	if !ok {
		return fmt.Errorf("move player %q to room %q: player not found", playerID, roomID)
	}
	toRoom, ok := w.rooms[roomID]
	if !ok {
		return fmt.Errorf("move player %q to room %q: target room not found", playerID, roomID)
	}

	var creature model.Creature
	hasCreature := false
	if !player.CreatureID.IsZero() {
		creature, ok = w.creatures[player.CreatureID]
		if !ok {
			return fmt.Errorf("move player %q to room %q: linked creature %q not found", playerID, roomID, player.CreatureID)
		}
		if !creature.PlayerID.IsZero() && creature.PlayerID != player.ID {
			return fmt.Errorf("move player %q to room %q: linked creature %q belongs to player %q", playerID, roomID, creature.ID, creature.PlayerID)
		}
		hasCreature = true
	}

	player.RoomID = roomID
	w.players[player.ID] = player
	if hasCreature {
		creature.RoomID = roomID
		w.creatures[creature.ID] = creature
	}

	for currentRoomID, room := range w.rooms {
		room.PlayerIDs = removeID(room.PlayerIDs, player.ID)
		if hasCreature {
			room.CreatureIDs = removeID(room.CreatureIDs, creature.ID)
		}
		w.rooms[currentRoomID] = room
	}

	toRoom = w.rooms[toRoom.ID]
	toRoom.PlayerIDs = w.insertPlayerIDLegacySortedLocked(toRoom.PlayerIDs, player.ID)
	if hasCreature {
		toRoom.CreatureIDs = w.insertCreatureIDLegacySortedLocked(toRoom.CreatureIDs, creature.ID)
	}
	w.rooms[toRoom.ID] = toRoom
	nowUnix := time.Now().Unix()
	w.refreshRoomPermanentSpawnsLocked(toRoom.ID, nowUnix)
	w.checkRoomExitsLocked(toRoom.ID, nowUnix)
	return nil
}

type roomPermanentSlot struct {
	index    int
	misc     int
	ltime    int64
	interval int64
}

type roomPermanentGroup struct {
	misc  int
	count int
}

func (w *World) refreshRoomPermanentSpawnsLocked(roomID model.RoomID, nowUnix int64) {
	room, ok := w.rooms[roomID]
	if !ok {
		return
	}
	for _, group := range roomPermanentDueGroups(roomPermanentSlots(room, "perm_mon", "permMon", "permanentCreature"), nowUnix) {
		w.spawnRoomPermanentCreatureGroupLocked(roomID, group.misc, group.count)
	}
	room = w.rooms[roomID]
	for _, group := range roomPermanentDueGroups(roomPermanentSlots(room, "perm_obj", "permObj", "permanentObject"), nowUnix) {
		w.spawnRoomPermanentObjectGroupLocked(roomID, group.misc, group.count)
	}
}

func roomPermanentSlots(room model.Room, prefixes ...string) []roomPermanentSlot {
	if len(room.Properties) == 0 {
		return nil
	}
	slots := make([]roomPermanentSlot, 0, 10)
	for i := 0; i < 10; i++ {
		for _, prefix := range prefixes {
			misc, ok := roomPermanentSlotInt(room.Properties, prefix, i, "misc")
			if !ok || misc == 0 {
				continue
			}
			ltime, _ := roomPermanentSlotInt(room.Properties, prefix, i, "ltime")
			interval, _ := roomPermanentSlotInt(room.Properties, prefix, i, "interval")
			slots = append(slots, roomPermanentSlot{
				index:    i,
				misc:     int(misc),
				ltime:    ltime,
				interval: interval,
			})
			break
		}
	}
	return slots
}

func roomPermanentDueGroups(slots []roomPermanentSlot, nowUnix int64) []roomPermanentGroup {
	if len(slots) == 0 {
		return nil
	}
	checked := make([]bool, len(slots))
	groups := make([]roomPermanentGroup, 0, len(slots))
	for i, slot := range slots {
		if checked[i] || slot.misc == 0 || slot.ltime+slot.interval > nowUnix {
			continue
		}
		count := 1
		for j := i + 1; j < len(slots); j++ {
			other := slots[j]
			if checked[j] || other.misc != slot.misc || other.ltime+other.interval >= nowUnix {
				continue
			}
			count++
			checked[j] = true
		}
		groups = append(groups, roomPermanentGroup{misc: slot.misc, count: count})
	}
	return groups
}

func roomPermanentSlotInt(properties map[string]string, prefix string, index int, field string) (int64, bool) {
	for _, key := range []string{
		fmt.Sprintf("%s.%d.%s", prefix, index, field),
		fmt.Sprintf("%s[%d].%s", prefix, index, field),
		fmt.Sprintf("%s.%02d.%s", prefix, index, field),
		fmt.Sprintf("%s[%02d].%s", prefix, index, field),
	} {
		if value, ok := parseStateInt(properties[key]); ok {
			return int64(value), true
		}
	}
	return 0, false
}

func (w *World) spawnRoomPermanentCreatureGroupLocked(roomID model.RoomID, misc int, wanted int) {
	if misc <= 0 || wanted <= 0 {
		return
	}
	protoID := model.CreatureID(fmt.Sprintf("creature:m%02d:%d", misc/100, misc%100))
	proto, ok := w.creatures[protoID]
	if !ok {
		return
	}
	name := creatureLegacySortName(proto)
	if name == "" {
		name = string(proto.ID)
	}
	current := 0
	if room, ok := w.rooms[roomID]; ok {
		for _, creatureID := range room.CreatureIDs {
			creature, ok := w.creatures[creatureID]
			if !ok || !creatureHasAnyFlag(creature, "MPERMT", "permanent") {
				continue
			}
			if existingName := creatureLegacySortName(creature); existingName == name || (existingName == "" && string(creature.ID) == name) {
				current++
			}
		}
	}
	for i := 0; i < wanted-current; i++ {
		_, _ = w.spawnCreatureLocked(protoID, roomID, true, []string{"MPERMT", "permanent"})
	}
}

func (w *World) spawnRoomPermanentObjectGroupLocked(roomID model.RoomID, misc int, wanted int) {
	if misc <= 0 || wanted <= 0 {
		return
	}
	protoID := legacyCarryObjectPrototypeID(misc)
	if _, ok := w.prototypes[protoID]; !ok {
		return
	}
	name := w.objectPrototypeLegacySortNameLocked(protoID)
	current := 0
	if room, ok := w.rooms[roomID]; ok {
		for _, objectID := range room.Objects.ObjectIDs {
			object, ok := w.objects[objectID]
			if !ok || !w.objectHasAnyLegacyFlagLocked(object, "OPERMT", "roomPermanent", "permanent") {
				continue
			}
			if w.objectLegacySortNameFromObjectLocked(object) == name {
				current++
			}
		}
	}
	for i := 0; i < wanted-current; i++ {
		objectID, err := w.createObjectFromPrototypeLocked(protoID, model.ObjectLocation{RoomID: roomID})
		if err != nil {
			continue
		}
		object := w.objects[objectID]
		object.Metadata.Tags = addMetadataTags(object.Metadata.Tags, []string{"OPERMT", "roomPermanent", "permanent"})
		w.objects[objectID] = object
		w.MarkRoomObjectsDirty(roomID)
	}
}

func (w *World) objectPrototypeLegacySortNameLocked(protoID model.PrototypeID) string {
	proto, ok := w.prototypes[protoID]
	if !ok {
		return string(protoID)
	}
	if name := strings.TrimSpace(proto.Properties["name"]); name != "" {
		return name
	}
	if name := strings.TrimSpace(proto.DisplayName); name != "" {
		return name
	}
	if name := firstStateObjectKeyName(proto.Properties); name != "" {
		return name
	}
	return string(proto.ID)
}

func (w *World) objectHasAnyLegacyFlagLocked(object model.ObjectInstance, names ...string) bool {
	if hasAnyNormalizedFlag(object.Metadata.Tags, names...) || objectHasAnyPropertyFlag(object.Properties, names...) {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := w.prototypes[object.PrototypeID]
	if !ok {
		return false
	}
	return hasAnyNormalizedFlag(proto.Metadata.Tags, names...) || objectHasAnyPropertyFlag(proto.Properties, names...)
}

// MoveCreatureToRoom moves a creature directly to roomID, updating the room occupant lists.
func (w *World) MoveCreatureToRoom(creatureID model.CreatureID, roomID model.RoomID) error {
	if w == nil {
		return fmt.Errorf("move creature %q to room %q: world state is nil", creatureID, roomID)
	}
	if creatureID.IsZero() {
		return fmt.Errorf("move creature to room %q: creature id is required", roomID)
	}
	if roomID.IsZero() {
		return fmt.Errorf("move creature %q to room: room id is required", creatureID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	creature, ok := w.creatures[creatureID]
	if !ok {
		return fmt.Errorf("move creature %q to room %q: creature not found", creatureID, roomID)
	}
	toRoom, ok := w.rooms[roomID]
	if !ok {
		return fmt.Errorf("move creature %q to room %q: target room not found", creatureID, roomID)
	}

	if !creature.PlayerID.IsZero() {
		player, ok := w.players[creature.PlayerID]
		if ok {
			player.RoomID = roomID
			w.players[player.ID] = player
		}
	}

	creature.RoomID = roomID
	w.creatures[creature.ID] = creature

	for currentRoomID, room := range w.rooms {
		if !creature.PlayerID.IsZero() {
			room.PlayerIDs = removeID(room.PlayerIDs, creature.PlayerID)
		}
		room.CreatureIDs = removeID(room.CreatureIDs, creature.ID)
		w.rooms[currentRoomID] = room
	}

	toRoom = w.rooms[toRoom.ID]
	if !creature.PlayerID.IsZero() {
		toRoom.PlayerIDs = w.insertPlayerIDLegacySortedLocked(toRoom.PlayerIDs, creature.PlayerID)
	}
	toRoom.CreatureIDs = w.insertCreatureIDLegacySortedLocked(toRoom.CreatureIDs, creature.ID)
	w.rooms[toRoom.ID] = toRoom
	return nil
}

// MoveObjectToCreatureInventory moves objectID into creatureID's inventory.
func (w *World) MoveObjectToCreatureInventory(objectID model.ObjectInstanceID, creatureID model.CreatureID) error {
	return w.MoveObject(objectID, model.ObjectLocation{CreatureID: creatureID, Slot: "inventory"})
}

// StealCreatureInventoryObject moves an object from one creature inventory to
// another only if it is still in the expected source inventory under the same
// lock.
func (w *World) StealCreatureInventoryObject(objectID model.ObjectInstanceID, fromCreatureID model.CreatureID, toCreatureID model.CreatureID) (bool, error) {
	if w == nil {
		return false, fmt.Errorf("steal object %q: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return false, fmt.Errorf("steal object: object id is required")
	}
	if fromCreatureID.IsZero() {
		return false, fmt.Errorf("steal object %q: source creature id is required", objectID)
	}
	if toCreatureID.IsZero() {
		return false, fmt.Errorf("steal object %q: target creature id is required", objectID)
	}
	if fromCreatureID == toCreatureID {
		return false, nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	object, ok := w.objects[objectID]
	if !ok {
		return false, fmt.Errorf("steal object %q: object not found", objectID)
	}
	if _, ok := w.creatures[fromCreatureID]; !ok {
		return false, fmt.Errorf("steal object %q: source creature %q not found", objectID, fromCreatureID)
	}
	if _, ok := w.creatures[toCreatureID]; !ok {
		return false, fmt.Errorf("steal object %q: target creature %q not found", objectID, toCreatureID)
	}
	if object.Location.CreatureID != fromCreatureID || (object.Location.Slot != "" && object.Location.Slot != "inventory") {
		return false, nil
	}

	location := model.ObjectLocation{CreatureID: toCreatureID, Slot: "inventory"}
	if err := w.validateObjectDestinationLocked(objectID, location); err != nil {
		return false, err
	}
	nextObject := object
	nextObject.Location = location
	if err := nextObject.Validate(); err != nil {
		return false, fmt.Errorf("steal object %q: %w", objectID, err)
	}
	w.removeObjectFromHolderLocked(objectID, object.Location)
	w.objects[objectID] = nextObject
	w.addObjectToHolderLocked(objectID, location)
	return true, nil
}

// MoveObjectToRoom moves objectID into roomID.
func (w *World) MoveObjectToRoom(objectID model.ObjectInstanceID, roomID model.RoomID) error {
	return w.MoveObject(objectID, model.ObjectLocation{RoomID: roomID})
}

// MoveObject moves objectID to location and keeps old and new holder ref lists in sync.
func (w *World) MoveObject(objectID model.ObjectInstanceID, location model.ObjectLocation) error {
	if w == nil {
		return fmt.Errorf("move object %q: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return fmt.Errorf("move object: object id is required")
	}
	if err := location.Validate(); err != nil {
		return fmt.Errorf("move object %q: location: %w", objectID, err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	object, ok := w.objects[objectID]
	if !ok {
		return fmt.Errorf("move object %q: object not found", objectID)
	}
	if err := w.validateObjectDestinationLocked(objectID, location); err != nil {
		return err
	}

	nextObject := object
	nextObject.Location = location
	if err := nextObject.Validate(); err != nil {
		return fmt.Errorf("move object %q: %w", objectID, err)
	}

	w.removeObjectFromHolderLocked(objectID, object.Location)
	w.objects[objectID] = nextObject
	w.addObjectToHolderLocked(objectID, location)

	// B: Mark affected parties as dirty on mutation (preferred over marking only inside Save)
	if !object.Location.CreatureID.IsZero() {
		if c, ok := w.creatures[object.Location.CreatureID]; ok && !c.PlayerID.IsZero() {
			w.MarkPlayerDirty(c.PlayerID)
		}
	}
	if !location.CreatureID.IsZero() {
		if c, ok := w.creatures[location.CreatureID]; ok && !c.PlayerID.IsZero() {
			w.MarkPlayerDirty(c.PlayerID)
		}
	}
	if !object.Location.BankID.IsZero() {
		w.MarkBankDirty(object.Location.BankID)
	}
	if !location.BankID.IsZero() {
		w.MarkBankDirty(location.BankID)
	}

	// D: Room floor objects persistence - mark both old and new room when an object
	// moves into or out of a room (drop to ground, pickup from ground, corpse scatter etc).
	if !object.Location.RoomID.IsZero() {
		w.MarkRoomObjectsDirty(object.Location.RoomID)
	}
	if !location.RoomID.IsZero() {
		w.MarkRoomObjectsDirty(location.RoomID)
	}

	return nil
}

// CloneObjectToCreatureInventory creates a new object instance copied from
// sourceID and puts it in creatureID's inventory. If sourceID is an object
// instance, its recursive contents are materialized with fresh instance IDs.
// If sourceID names an object prototype, a fresh instance is created from that
// prototype, matching legacy load_obj-style callers that pass object numbers.
func (w *World) CloneObjectToCreatureInventory(sourceID model.ObjectInstanceID, creatureID model.CreatureID) (model.ObjectInstanceID, error) {
	if w == nil {
		return "", fmt.Errorf("clone object %q: world state is nil", sourceID)
	}
	if sourceID.IsZero() {
		return "", fmt.Errorf("clone object: source object id is required")
	}
	if creatureID.IsZero() {
		return "", fmt.Errorf("clone object %q: creature id is required", sourceID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if _, ok := w.objects[sourceID]; !ok {
		if _, ok := w.prototypeIDFromCloneSourceLocked(sourceID); !ok {
			return "", fmt.Errorf("clone object %q: source object not found", sourceID)
		}
	}
	if _, ok := w.creatures[creatureID]; !ok {
		return "", fmt.Errorf("clone object %q: target creature %q not found", sourceID, creatureID)
	}

	cloneID, err := w.cloneObjectSourceToLocationLocked(sourceID, model.ObjectLocation{CreatureID: creatureID, Slot: "inventory"})
	if err != nil {
		return "", fmt.Errorf("clone object %q: %w", sourceID, err)
	}
	if creature := w.creatures[creatureID]; !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneID, nil
}

// PurchaseObjectToCreatureInventory clones sourceID into creatureID's inventory
// and debits the creature's gold stat under one lock. affordable is false when
// the creature exists but does not have enough gold; in that case state is
// unchanged and err is nil.
func (w *World) PurchaseObjectToCreatureInventory(sourceID model.ObjectInstanceID, creatureID model.CreatureID, price int) (newID model.ObjectInstanceID, remainingGold int, affordable bool, err error) {
	if w == nil {
		return "", 0, false, fmt.Errorf("purchase object %q: world state is nil", sourceID)
	}
	if sourceID.IsZero() {
		return "", 0, false, fmt.Errorf("purchase object: source object id is required")
	}
	if creatureID.IsZero() {
		return "", 0, false, fmt.Errorf("purchase object %q: creature id is required", sourceID)
	}
	if price < 0 {
		return "", 0, false, fmt.Errorf("purchase object %q: price cannot be negative", sourceID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if _, ok := w.objects[sourceID]; !ok {
		if _, ok := w.prototypeIDFromCloneSourceLocked(sourceID); !ok {
			return "", 0, false, fmt.Errorf("purchase object %q: source object not found", sourceID)
		}
	}
	creature, ok := w.creatures[creatureID]
	if !ok {
		return "", 0, false, fmt.Errorf("purchase object %q: target creature %q not found", sourceID, creatureID)
	}
	gold := creature.Stats["gold"]
	if gold < price {
		return "", gold, false, nil
	}

	newID, err = w.cloneObjectSourceToLocationLocked(sourceID, model.ObjectLocation{CreatureID: creatureID, Slot: "inventory"})
	if err != nil {
		return "", gold, false, fmt.Errorf("purchase object %q: %w", sourceID, err)
	}

	creature = w.creatures[creatureID]
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	remainingGold = gold - price
	creature.Stats["gold"] = remainingGold
	w.creatures[creatureID] = creature

	// B (A push): mark dirty at purchase (gold + inventory mutation)
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}

	return newID, remainingGold, true, nil
}

// SellObjectFromCreatureInventory removes an object owned by creatureID and
// credits the creature's gold stat under one lock.
func (w *World) SellObjectFromCreatureInventory(objectID model.ObjectInstanceID, creatureID model.CreatureID, price int) (newGold int, sold bool, err error) {
	if w == nil {
		return 0, false, fmt.Errorf("sell object %q: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return 0, false, fmt.Errorf("sell object: object id is required")
	}
	if creatureID.IsZero() {
		return 0, false, fmt.Errorf("sell object %q: creature id is required", objectID)
	}
	if price < 0 {
		return 0, false, fmt.Errorf("sell object %q: price cannot be negative", objectID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	object, ok := w.objects[objectID]
	if !ok {
		return 0, false, fmt.Errorf("sell object %q: object not found", objectID)
	}
	creature, ok := w.creatures[creatureID]
	if !ok {
		return 0, false, fmt.Errorf("sell object %q: creature %q not found", objectID, creatureID)
	}
	if object.Location.CreatureID != creatureID {
		return creature.Stats["gold"], false, nil
	}
	if len(object.Contents.ObjectIDs) != 0 {
		return creature.Stats["gold"], false, fmt.Errorf("sell object %q: object has contents", objectID)
	}

	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	newGold = creature.Stats["gold"] + price
	creature.Stats["gold"] = newGold
	w.creatures[creatureID] = creature

	// B (A push): mark dirty at sell (gold + inventory mutation)
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}

	w.removeObjectFromHolderLocked(objectID, object.Location)
	delete(w.objects, objectID)
	return newGold, true, nil
}

// RepairCreatureInventoryObject debits repair cost and applies object
// property/tag changes for an inventory object under one lock.
func (w *World) RepairCreatureInventoryObject(objectID model.ObjectInstanceID, creatureID model.CreatureID, cost int, properties map[string]string, removeTags []string) (newGold int, repaired bool, affordable bool, err error) {
	if w == nil {
		return 0, false, false, fmt.Errorf("repair object %q: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return 0, false, false, fmt.Errorf("repair object: object id is required")
	}
	if creatureID.IsZero() {
		return 0, false, false, fmt.Errorf("repair object %q: creature id is required", objectID)
	}
	if cost < 0 {
		return 0, false, false, fmt.Errorf("repair object %q: cost cannot be negative", objectID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	object, ok := w.objects[objectID]
	if !ok {
		return 0, false, false, fmt.Errorf("repair object %q: object not found", objectID)
	}
	creature, ok := w.creatures[creatureID]
	if !ok {
		return 0, false, false, fmt.Errorf("repair object %q: creature %q not found", objectID, creatureID)
	}
	if object.Location.CreatureID != creatureID || (object.Location.Slot != "" && object.Location.Slot != "inventory") {
		return creature.Stats["gold"], false, true, nil
	}
	gold := creature.Stats["gold"]
	if gold < cost {
		return gold, false, false, nil
	}

	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	newGold = gold - cost
	creature.Stats["gold"] = newGold
	w.creatures[creatureID] = creature

	// B (A push): mark dirty at repair (gold mutation)
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}

	object.Properties = maps.Clone(object.Properties)
	if object.Properties == nil {
		object.Properties = map[string]string{}
	}
	for key, value := range properties {
		object.Properties[key] = value
	}
	object.Metadata.Tags = removeMetadataTags(object.Metadata.Tags, removeTags)
	w.objects[objectID] = object
	return newGold, true, true, nil
}

// DestroyCreatureInventoryObject deletes an inventory object and removes holder refs.
func (w *World) DestroyCreatureInventoryObject(objectID model.ObjectInstanceID, creatureID model.CreatureID) (destroyed bool, err error) {
	if w == nil {
		return false, fmt.Errorf("destroy object %q: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return false, fmt.Errorf("destroy object: object id is required")
	}
	if creatureID.IsZero() {
		return false, fmt.Errorf("destroy object %q: creature id is required", objectID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	object, ok := w.objects[objectID]
	if !ok {
		return false, fmt.Errorf("destroy object %q: object not found", objectID)
	}
	creature, ok := w.creatures[creatureID]
	if !ok {
		return false, fmt.Errorf("destroy object %q: creature %q not found", objectID, creatureID)
	}
	if object.Location.CreatureID != creatureID || (object.Location.Slot != "" && object.Location.Slot != "inventory") {
		return false, nil
	}
	w.removeObjectFromHolderLocked(objectID, object.Location)
	delete(w.objects, objectID)
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return true, nil
}

// ConsumeCreatureObjectCharge decrements an object charge for an object carried
// or equipped by creatureID. When deleteAtZero is true, the object is removed
// after the consumed charge reaches zero.
func (w *World) ConsumeCreatureObjectCharge(objectID model.ObjectInstanceID, creatureID model.CreatureID, deleteAtZero bool) (updated model.ObjectInstance, deleted bool, consumed bool, err error) {
	if w == nil {
		return model.ObjectInstance{}, false, false, fmt.Errorf("consume object %q: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return model.ObjectInstance{}, false, false, fmt.Errorf("consume object: object id is required")
	}
	if creatureID.IsZero() {
		return model.ObjectInstance{}, false, false, fmt.Errorf("consume object %q: creature id is required", objectID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	object, ok := w.objects[objectID]
	if !ok {
		return model.ObjectInstance{}, false, false, fmt.Errorf("consume object %q: object not found", objectID)
	}
	if _, ok := w.creatures[creatureID]; !ok {
		return model.ObjectInstance{}, false, false, fmt.Errorf("consume object %q: creature %q not found", objectID, creatureID)
	}
	if object.Location.CreatureID != creatureID {
		return cloneObject(object), false, false, nil
	}
	charges, ok := w.objectIntPropertyLocked(object, "shotsCurrent")
	if !ok || charges < 1 {
		return cloneObject(object), false, false, nil
	}

	charges--
	if object.Properties == nil {
		object.Properties = map[string]string{}
	}
	object.Properties["shotsCurrent"] = strconv.Itoa(charges)
	if deleteAtZero && charges < 1 {
		w.removeObjectFromHolderLocked(objectID, object.Location)
		delete(w.objects, objectID)
		return model.ObjectInstance{}, true, true, nil
	}
	w.objects[objectID] = object
	return cloneObject(object), false, true, nil
}

// TransferCreatureGold moves gold from one creature stat bucket to another under
// one lock. ok is false when the source creature exists but does not have enough
// gold; in that case state is unchanged and err is nil.
func (w *World) TransferCreatureGold(fromID model.CreatureID, toID model.CreatureID, amount int) (fromGold int, toGold int, ok bool, err error) {
	if w == nil {
		return 0, 0, false, fmt.Errorf("transfer gold from %q to %q: world state is nil", fromID, toID)
	}
	if fromID.IsZero() {
		return 0, 0, false, fmt.Errorf("transfer gold to %q: source creature id is required", toID)
	}
	if toID.IsZero() {
		return 0, 0, false, fmt.Errorf("transfer gold from %q: target creature id is required", fromID)
	}
	if amount < 1 {
		return 0, 0, false, fmt.Errorf("transfer gold from %q to %q: amount must be positive", fromID, toID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	from, ok := w.creatures[fromID]
	if !ok {
		return 0, 0, false, fmt.Errorf("transfer gold from %q: source creature not found", fromID)
	}
	to, ok := w.creatures[toID]
	if !ok {
		return 0, 0, false, fmt.Errorf("transfer gold to %q: target creature not found", toID)
	}
	fromGold = from.Stats["gold"]
	toGold = to.Stats["gold"]
	if fromGold < amount {
		return fromGold, toGold, false, nil
	}

	if from.Stats == nil {
		from.Stats = map[string]int{}
	}
	if to.Stats == nil {
		to.Stats = map[string]int{}
	}
	fromGold -= amount
	toGold += amount
	from.Stats["gold"] = fromGold
	to.Stats["gold"] = toGold
	w.creatures[fromID] = from
	w.creatures[toID] = to

	// B: Mark dirty on gold transfer
	if !from.PlayerID.IsZero() {
		w.MarkPlayerDirty(from.PlayerID)
	}
	if !to.PlayerID.IsZero() {
		w.MarkPlayerDirty(to.PlayerID)
	}
	return fromGold, toGold, true, nil
}

// DropCreatureGoldToRoom debits creature gold and creates a money object in a
// room under one lock. ok is false when the creature has insufficient gold.
func (w *World) DropCreatureGoldToRoom(creatureID model.CreatureID, roomID model.RoomID, amount int) (objectID model.ObjectInstanceID, remainingGold int, ok bool, err error) {
	if w == nil {
		return "", 0, false, fmt.Errorf("drop gold from %q to room %q: world state is nil", creatureID, roomID)
	}
	if creatureID.IsZero() {
		return "", 0, false, fmt.Errorf("drop gold to room %q: creature id is required", roomID)
	}
	if roomID.IsZero() {
		return "", 0, false, fmt.Errorf("drop gold from %q: room id is required", creatureID)
	}
	if amount < 1 {
		return "", 0, false, fmt.Errorf("drop gold from %q to room %q: amount must be positive", creatureID, roomID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	creature, okCreature := w.creatures[creatureID]
	if !okCreature {
		return "", 0, false, fmt.Errorf("drop gold from %q: creature not found", creatureID)
	}
	if _, okRoom := w.rooms[roomID]; !okRoom {
		return "", 0, false, fmt.Errorf("drop gold to room %q: room not found", roomID)
	}
	gold := creature.Stats["gold"]
	if gold < amount {
		return "", gold, false, nil
	}

	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	remainingGold = gold - amount
	creature.Stats["gold"] = remainingGold
	w.creatures[creatureID] = creature

	// B (A push): mark dirty at gold mutation time (player creature)
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}

	objectID = w.nextObjectCloneIDLocked("object:money")
	moneyPrototypeID := model.PrototypeID("prototype:money")
	if _, ok := w.prototypes[moneyPrototypeID]; !ok {
		w.prototypes[moneyPrototypeID] = model.ObjectPrototype{
			ID:          moneyPrototypeID,
			Kind:        model.ObjectKindMoney,
			DisplayName: "돈",
		}
	}
	object := model.ObjectInstance{
		ID:                  objectID,
		PrototypeID:         moneyPrototypeID,
		DisplayNameOverride: fmt.Sprintf("%d냥", amount),
		Location:            model.ObjectLocation{RoomID: roomID},
		Properties: map[string]string{
			"kind":  string(model.ObjectKindMoney),
			"type":  "10",
			"value": strconv.Itoa(amount),
		},
	}
	if err := object.Validate(); err != nil {
		return "", 0, false, fmt.Errorf("drop gold object %q: %w", objectID, err)
	}
	w.objects[objectID] = object
	w.addObjectToHolderLocked(objectID, object.Location)

	// D: Mark the room dirty because a new money object was placed on the floor.
	w.MarkRoomObjectsDirty(roomID)

	return objectID, remainingGold, true, nil
}

// PickupMoneyObjectToCreatureGold removes a money object from its current
// holder and credits the creature's gold under one lock.
func (w *World) PickupMoneyObjectToCreatureGold(objectID model.ObjectInstanceID, from model.ObjectLocation, creatureID model.CreatureID) (newGold int, amount int, picked bool, err error) {
	if w == nil {
		return 0, 0, false, fmt.Errorf("pickup money object %q: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return 0, 0, false, fmt.Errorf("pickup money object: object id is required")
	}
	if creatureID.IsZero() {
		return 0, 0, false, fmt.Errorf("pickup money object %q: creature id is required", objectID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	object, ok := w.objects[objectID]
	if !ok {
		return 0, 0, false, fmt.Errorf("pickup money object %q: object not found", objectID)
	}
	creature, ok := w.creatures[creatureID]
	if !ok {
		return 0, 0, false, fmt.Errorf("pickup money object %q: creature %q not found", objectID, creatureID)
	}
	if !objectLocationEqual(object.Location, from) || !w.objectIsMoneyLocked(object) {
		return creature.Stats["gold"], 0, false, nil
	}
	amount, _ = w.objectIntPropertyLocked(object, "value")
	if amount < 1 {
		return creature.Stats["gold"], 0, false, nil
	}

	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	newGold = creature.Stats["gold"] + amount
	creature.Stats["gold"] = newGold
	w.creatures[creatureID] = creature

	// B (A push): mark dirty at gold mutation time
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}

	w.removeObjectFromHolderLocked(objectID, object.Location)
	if !object.Location.ContainerID.IsZero() {
		container := w.objects[object.Location.ContainerID]
		current := objectCountPropertyOrLenLocked(w, container, "shotsCurrent")
		if container.Properties == nil {
			container.Properties = map[string]string{}
		}
		if current > 0 {
			current--
		}
		container.Properties["shotsCurrent"] = strconv.Itoa(current)
		w.objects[container.ID] = container
	}
	delete(w.objects, objectID)

	// D (Package A): Room lost a floor object (money pickup) - direct or nested inside
	// container (bag/corpse) that is itself on floor. Previous code only handled direct RoomID;
	// nested money in dropped container would not mark room, causing data loss on restart.
	roomToMark := object.Location.RoomID
	if roomToMark.IsZero() && !object.Location.ContainerID.IsZero() {
		if c, ok := w.objects[object.Location.ContainerID]; ok && !c.Location.RoomID.IsZero() {
			roomToMark = c.Location.RoomID
		}
	}
	if !roomToMark.IsZero() {
		w.MarkRoomObjectsDirty(roomToMark)
	}
	// Also ensure container itself (if on floor) is marked due to shotsCurrent update above.
	if !object.Location.ContainerID.IsZero() {
		if c, ok := w.objects[object.Location.ContainerID]; ok && !c.Location.RoomID.IsZero() {
			w.MarkRoomObjectsDirty(c.Location.RoomID)
		}
	}

	return newGold, amount, true, nil
}

// DepositCreatureGoldToObjectValue debits creature gold and credits an object's
// numeric value property under one lock. ok is false when the creature has
// insufficient gold; withinLimit is false when maxValue would be exceeded.
func (w *World) DepositCreatureGoldToObjectValue(creatureID model.CreatureID, objectID model.ObjectInstanceID, amount int, maxValue int) (remainingGold int, objectValue int, ok bool, withinLimit bool, err error) {
	return w.DepositCreatureGoldToObjectValueScaled(creatureID, objectID, amount, amount, amount, maxValue)
}

// DepositCreatureGoldToObjectValueScaled debits one creature gold amount while
// crediting a potentially different object value amount. limitAmount is the
// delta used for the maxValue check; this preserves legacy callers where the
// stored object unit differs from player gold units.
func (w *World) DepositCreatureGoldToObjectValueScaled(creatureID model.CreatureID, objectID model.ObjectInstanceID, goldAmount int, valueAmount int, limitAmount int, maxValue int) (remainingGold int, objectValue int, ok bool, withinLimit bool, err error) {
	if w == nil {
		return 0, 0, false, false, fmt.Errorf("deposit gold from %q to object %q: world state is nil", creatureID, objectID)
	}
	if creatureID.IsZero() {
		return 0, 0, false, false, fmt.Errorf("deposit gold to object %q: creature id is required", objectID)
	}
	if objectID.IsZero() {
		return 0, 0, false, false, fmt.Errorf("deposit gold from %q: object id is required", creatureID)
	}
	if goldAmount < 0 || valueAmount < 0 || limitAmount < 0 {
		return 0, 0, false, false, fmt.Errorf("deposit gold from %q to object %q: amount cannot be negative", creatureID, objectID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	creature, okCreature := w.creatures[creatureID]
	if !okCreature {
		return 0, 0, false, false, fmt.Errorf("deposit gold from %q: creature not found", creatureID)
	}
	object, okObject := w.objects[objectID]
	if !okObject {
		return 0, 0, false, false, fmt.Errorf("deposit gold to object %q: object not found", objectID)
	}
	gold := creature.Stats["gold"]
	value, _ := w.objectIntPropertyLocked(object, "value")
	if gold < goldAmount {
		return gold, value, false, true, nil
	}
	if maxValue > 0 && value+limitAmount > maxValue {
		return gold, value, true, false, nil
	}

	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	if object.Properties == nil {
		object.Properties = map[string]string{}
	}
	remainingGold = gold - goldAmount
	objectValue = value + valueAmount
	creature.Stats["gold"] = remainingGold
	object.Properties["value"] = strconv.Itoa(objectValue)
	w.creatures[creatureID] = creature
	w.objects[objectID] = object

	// B (A push): mark dirty at gold mutation time (player gold change)
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return remainingGold, objectValue, true, true, nil
}

// WithdrawObjectValueToCreatureGold debits an object's numeric value property
// and credits creature gold under one lock. ok is false when object value is
// insufficient; in that case state is unchanged.
func (w *World) WithdrawObjectValueToCreatureGold(objectID model.ObjectInstanceID, creatureID model.CreatureID, amount int) (newGold int, objectValue int, ok bool, err error) {
	return w.WithdrawObjectValueToCreatureGoldScaled(objectID, creatureID, amount, amount)
}

// WithdrawObjectValueToCreatureGoldScaled debits one object value amount while
// crediting a potentially different creature gold amount.
func (w *World) WithdrawObjectValueToCreatureGoldScaled(objectID model.ObjectInstanceID, creatureID model.CreatureID, valueAmount int, goldAmount int) (newGold int, objectValue int, ok bool, err error) {
	if w == nil {
		return 0, 0, false, fmt.Errorf("withdraw gold from object %q to %q: world state is nil", objectID, creatureID)
	}
	if objectID.IsZero() {
		return 0, 0, false, fmt.Errorf("withdraw gold to %q: object id is required", creatureID)
	}
	if creatureID.IsZero() {
		return 0, 0, false, fmt.Errorf("withdraw gold from object %q: creature id is required", objectID)
	}
	if valueAmount < 0 || goldAmount < 0 {
		return 0, 0, false, fmt.Errorf("withdraw gold from object %q to %q: amount cannot be negative", objectID, creatureID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	object, okObject := w.objects[objectID]
	if !okObject {
		return 0, 0, false, fmt.Errorf("withdraw gold from object %q: object not found", objectID)
	}
	creature, okCreature := w.creatures[creatureID]
	if !okCreature {
		return 0, 0, false, fmt.Errorf("withdraw gold to %q: creature not found", creatureID)
	}
	value, _ := w.objectIntPropertyLocked(object, "value")
	gold := creature.Stats["gold"]
	if value < valueAmount {
		return gold, value, false, nil
	}

	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	if object.Properties == nil {
		object.Properties = map[string]string{}
	}
	newGold = gold + goldAmount
	objectValue = value - valueAmount
	creature.Stats["gold"] = newGold
	object.Properties["value"] = strconv.Itoa(objectValue)
	w.creatures[creatureID] = creature
	w.objects[objectID] = object

	// B (A push): mark dirty at gold mutation time
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return newGold, objectValue, true, nil
}

// StoreCreatureInventoryObjectInContainer moves an inventory object into a
// container and increments the container's shotsCurrent property under one lock.
func (w *World) StoreCreatureInventoryObjectInContainer(objectID model.ObjectInstanceID, creatureID model.CreatureID, containerID model.ObjectInstanceID, maxCount int) (newCount int, stored bool, full bool, err error) {
	if w == nil {
		return 0, false, false, fmt.Errorf("store object %q in container %q: world state is nil", objectID, containerID)
	}
	if objectID.IsZero() {
		return 0, false, false, fmt.Errorf("store object in container %q: object id is required", containerID)
	}
	if creatureID.IsZero() {
		return 0, false, false, fmt.Errorf("store object %q in container %q: creature id is required", objectID, containerID)
	}
	if containerID.IsZero() {
		return 0, false, false, fmt.Errorf("store object %q: container id is required", objectID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	object, ok := w.objects[objectID]
	if !ok {
		return 0, false, false, fmt.Errorf("store object %q: object not found", objectID)
	}
	if _, ok := w.creatures[creatureID]; !ok {
		return 0, false, false, fmt.Errorf("store object %q: creature %q not found", objectID, creatureID)
	}
	container, ok := w.objects[containerID]
	if !ok {
		return 0, false, false, fmt.Errorf("store object %q: container %q not found", objectID, containerID)
	}
	current := objectCountPropertyOrLenLocked(w, container, "shotsCurrent")
	if maxCount > 0 && current >= maxCount {
		return current, false, true, nil
	}
	if object.Location.CreatureID != creatureID || (object.Location.Slot != "" && object.Location.Slot != "inventory") {
		return current, false, false, nil
	}

	location := model.ObjectLocation{ContainerID: containerID}
	if err := w.validateObjectDestinationLocked(objectID, location); err != nil {
		return 0, false, false, err
	}
	nextObject := object
	nextObject.Location = location
	if err := nextObject.Validate(); err != nil {
		return 0, false, false, fmt.Errorf("store object %q: %w", objectID, err)
	}

	w.removeObjectFromHolderLocked(objectID, object.Location)
	w.objects[objectID] = nextObject
	w.addObjectToHolderLocked(objectID, location)

	container = w.objects[containerID]
	if container.Properties == nil {
		container.Properties = map[string]string{}
	}
	newCount = current + 1
	container.Properties["shotsCurrent"] = strconv.Itoa(newCount)
	w.objects[containerID] = container

	// D (Package A): If container lives on room floor (dropped bag/pouch/corpse etc),
	// contents mutation means floor objects changed; mark for sidecar persistence.
	// Covers nested containers on floor + put from inv to floor-container.
	if !container.Location.RoomID.IsZero() {
		w.MarkRoomObjectsDirty(container.Location.RoomID)
	}

	return newCount, true, false, nil
}

// TakeContainerObjectToCreatureInventory moves a direct child from a container
// to creature inventory and decrements the container's shotsCurrent property.
func (w *World) TakeContainerObjectToCreatureInventory(objectID model.ObjectInstanceID, containerID model.ObjectInstanceID, creatureID model.CreatureID) (newCount int, taken bool, err error) {
	if w == nil {
		return 0, false, fmt.Errorf("take object %q from container %q: world state is nil", objectID, containerID)
	}
	if objectID.IsZero() {
		return 0, false, fmt.Errorf("take object from container %q: object id is required", containerID)
	}
	if containerID.IsZero() {
		return 0, false, fmt.Errorf("take object %q: container id is required", objectID)
	}
	if creatureID.IsZero() {
		return 0, false, fmt.Errorf("take object %q from container %q: creature id is required", objectID, containerID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	object, ok := w.objects[objectID]
	if !ok {
		return 0, false, fmt.Errorf("take object %q: object not found", objectID)
	}
	container, ok := w.objects[containerID]
	if !ok {
		return 0, false, fmt.Errorf("take object %q: container %q not found", objectID, containerID)
	}
	if _, ok := w.creatures[creatureID]; !ok {
		return 0, false, fmt.Errorf("take object %q: creature %q not found", objectID, creatureID)
	}
	current := objectCountPropertyOrLenLocked(w, container, "shotsCurrent")
	if object.Location.ContainerID != containerID {
		return current, false, nil
	}

	location := model.ObjectLocation{CreatureID: creatureID, Slot: "inventory"}
	if err := w.validateObjectDestinationLocked(objectID, location); err != nil {
		return 0, false, err
	}
	nextObject := object
	nextObject.Location = location
	if err := nextObject.Validate(); err != nil {
		return 0, false, fmt.Errorf("take object %q: %w", objectID, err)
	}

	w.removeObjectFromHolderLocked(objectID, object.Location)
	w.objects[objectID] = nextObject
	w.addObjectToHolderLocked(objectID, location)

	container = w.objects[containerID]
	if container.Properties == nil {
		container.Properties = map[string]string{}
	}
	if current > 0 {
		newCount = current - 1
	}
	container.Properties["shotsCurrent"] = strconv.Itoa(newCount)
	w.objects[containerID] = container

	// D (Package A): If container lives on room floor (dropped bag/pouch/corpse etc),
	// contents mutation (take out) means floor objects changed; mark for sidecar persistence.
	// Covers get from nested in floor-container.
	if !container.Location.RoomID.IsZero() {
		w.MarkRoomObjectsDirty(container.Location.RoomID)
	}

	return newCount, true, nil
}

func (w *World) nextObjectCloneIDLocked(sourceID model.ObjectInstanceID) model.ObjectInstanceID {
	base := strings.TrimSpace(string(sourceID))
	if base == "" {
		base = "object"
	}
	for i := 1; ; i++ {
		id := model.ObjectInstanceID(fmt.Sprintf("%s:clone:%06d", base, i))
		if _, exists := w.objects[id]; !exists {
			return id
		}
	}
}

func (w *World) cloneObjectSourceToLocationLocked(sourceID model.ObjectInstanceID, location model.ObjectLocation) (model.ObjectInstanceID, error) {
	if _, ok := w.objects[sourceID]; ok {
		return w.cloneObjectTreeToLocationLocked(sourceID, location, map[model.ObjectInstanceID]struct{}{})
	}
	protoID, ok := w.prototypeIDFromCloneSourceLocked(sourceID)
	if !ok {
		return "", fmt.Errorf("source object or prototype not found")
	}
	return w.createObjectFromPrototypeLocked(protoID, location)
}

func (w *World) prototypeIDFromCloneSourceLocked(sourceID model.ObjectInstanceID) (model.PrototypeID, bool) {
	protoID := model.PrototypeID(sourceID)
	if _, ok := w.prototypes[protoID]; ok {
		return protoID, true
	}
	if number, ok := legacyCarryNumberFromCloneSource(sourceID); ok {
		protoID = legacyCarryObjectPrototypeID(number)
		if _, ok := w.prototypes[protoID]; ok {
			return protoID, true
		}
	}
	return "", false
}

func legacyCarryNumberFromCloneSource(sourceID model.ObjectInstanceID) (int, bool) {
	raw := strings.TrimSpace(string(sourceID))
	if !strings.HasPrefix(raw, "legacy-carry:") {
		return 0, false
	}
	idx := strings.LastIndex(raw, ":")
	if idx < 0 || idx == len(raw)-1 {
		return 0, false
	}
	number, err := strconv.Atoi(raw[idx+1:])
	return number, err == nil && number >= 0
}

func (w *World) createObjectFromPrototypeLocked(protoID model.PrototypeID, location model.ObjectLocation) (model.ObjectInstanceID, error) {
	proto, ok := w.prototypes[protoID]
	if !ok {
		return "", fmt.Errorf("prototype %q not found", protoID)
	}
	if templateID, ok := w.prototypeTemplateObjectIDLocked(proto); ok {
		return w.cloneObjectTreeToLocationLocked(templateID, location, map[model.ObjectInstanceID]struct{}{})
	}

	objectID := w.nextObjectCloneIDLocked(model.ObjectInstanceID(protoID))
	object := model.ObjectInstance{
		ID:          objectID,
		PrototypeID: protoID,
		Quantity:    1,
		Location:    location,
		Properties:  maps.Clone(proto.Properties),
		Metadata: model.Metadata{
			Tags: slices.Clone(proto.Metadata.Tags),
		},
	}
	w.applyRandomEnchantIfNeededLocked(&object)
	if err := object.Validate(); err != nil {
		return "", err
	}
	w.objects[object.ID] = object
	w.addObjectToHolderLocked(object.ID, object.Location)
	return object.ID, nil
}

func (w *World) prototypeTemplateObjectIDLocked(proto model.ObjectPrototype) (model.ObjectInstanceID, bool) {
	if resolution := proto.Metadata.PrototypeResolution; resolution != nil {
		if templateID := resolution.MaterializedFromObjectInstanceID; !templateID.IsZero() {
			if object, ok := w.objects[templateID]; ok && object.PrototypeID == proto.ID {
				return templateID, true
			}
		}
	}
	if object, ok := w.objects[model.ObjectInstanceID(proto.ID)]; ok && object.PrototypeID == proto.ID {
		return object.ID, true
	}
	return "", false
}

func (w *World) cloneObjectTreeToLocationLocked(sourceID model.ObjectInstanceID, location model.ObjectLocation, seen map[model.ObjectInstanceID]struct{}) (model.ObjectInstanceID, error) {
	if _, ok := seen[sourceID]; ok {
		return "", fmt.Errorf("object tree cycle at %q", sourceID)
	}
	seen[sourceID] = struct{}{}

	source, ok := w.objects[sourceID]
	if !ok {
		return "", fmt.Errorf("source object not found")
	}
	clone := cloneObject(source)
	clone.ID = w.nextObjectCloneIDLocked(sourceID)
	clone.Location = location
	clone.Contents = model.ObjectRefList{}
	w.applyRandomEnchantIfNeededLocked(&clone)
	if err := clone.Validate(); err != nil {
		delete(seen, sourceID)
		return "", err
	}

	w.objects[clone.ID] = clone
	w.addObjectToHolderLocked(clone.ID, clone.Location)
	for _, childID := range source.Contents.ObjectIDs {
		if _, err := w.cloneObjectTreeToLocationLocked(childID, model.ObjectLocation{ContainerID: clone.ID}, seen); err != nil {
			w.deleteObjectTreeLocked(clone.ID, map[model.ObjectInstanceID]struct{}{})
			delete(seen, sourceID)
			return "", fmt.Errorf("clone child %q: %w", childID, err)
		}
	}
	delete(seen, sourceID)
	return clone.ID, nil
}

func (w *World) applyRandomEnchantIfNeededLocked(object *model.ObjectInstance) {
	if object == nil || !w.objectHasRandomEnchantLocked(*object) {
		return
	}
	w.applyLegacyRandomEnchantRollLocked(object, rand.Intn(100)+1)
}

func (w *World) applyLegacyRandomEnchantRollLocked(object *model.ObjectInstance, roll int) {
	if object == nil {
		return
	}
	adjustment := legacyRandomEnchantAdjustment(roll)
	currentAdjustment, _ := w.objectIntPropertyAnyLocked(*object, "adjustment", "adjust")
	pDice, _ := w.objectIntPropertyAnyLocked(*object, "pDice", "pdice")
	if adjustment > 0 {
		object.Metadata.Tags = addMetadataTags(object.Metadata.Tags, []string{"enchanted", "oencha"})
		currentAdjustment = adjustment
		pDice += adjustment
	}
	if pDice < currentAdjustment {
		pDice = currentAdjustment
	}
	if object.Properties == nil {
		object.Properties = map[string]string{}
	}
	if adjustment > 0 || currentAdjustment != 0 {
		object.Properties["adjustment"] = strconv.Itoa(currentAdjustment)
	}
	if pDice != 0 || currentAdjustment != 0 {
		object.Properties["pDice"] = strconv.Itoa(pDice)
	}
}

func legacyRandomEnchantAdjustment(roll int) int {
	switch {
	case roll > 98:
		return 4
	case roll > 90:
		return 3
	case roll > 80:
		return 2
	case roll > 60:
		return 1
	default:
		return 0
	}
}

func (w *World) objectHasRandomEnchantLocked(object model.ObjectInstance) bool {
	if hasAnyNormalizedFlag(object.Metadata.Tags, "ORENCH", "randomEnchantment", "randEnch") ||
		objectHasAnyPropertyFlag(object.Properties, "ORENCH", "randomEnchantment", "randEnch") ||
		metadataHasLegacyObjectFlag(object.Metadata, legacyObjectRandomEnchantmentFlagBit) {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := w.prototypes[object.PrototypeID]
	if !ok {
		return false
	}
	return hasAnyNormalizedFlag(proto.Metadata.Tags, "ORENCH", "randomEnchantment", "randEnch") ||
		objectHasAnyPropertyFlag(proto.Properties, "ORENCH", "randomEnchantment", "randEnch") ||
		metadataHasLegacyObjectFlag(proto.Metadata, legacyObjectRandomEnchantmentFlagBit)
}

func metadataHasLegacyObjectFlag(metadata model.Metadata, bit int) bool {
	if bit < 0 {
		return false
	}
	flags := metadata.RawFields["flags"]
	byteIndex := bit / 8
	if byteIndex >= len(flags) {
		return false
	}
	return flags[byteIndex]&(1<<uint(bit%8)) != 0
}

func objectHasAnyPropertyFlag(properties map[string]string, names ...string) bool {
	if len(properties) == 0 {
		return false
	}
	targets := normalizedFlagSet(names...)
	for key, value := range properties {
		if _, ok := targets[normalizeFlagName(key)]; ok && propertyFlagEnabled(value) {
			return true
		}
		if objectFlagContainerProperty(key) && propertyFlagValueHasAnyToken(value, targets) {
			return true
		}
	}
	return false
}

func (w *World) objectIntPropertyAnyLocked(object model.ObjectInstance, keys ...string) (int, bool) {
	for _, key := range keys {
		if value, ok := w.objectIntPropertyLocked(object, key); ok {
			return value, true
		}
	}
	return 0, false
}

// SetCreatureStat sets a numeric creature stat, creating the stat map if needed.
func (w *World) SetCreatureStat(creatureID model.CreatureID, key string, value int) error {
	if w == nil {
		return fmt.Errorf("set creature %q stat %q: world state is nil", creatureID, key)
	}
	if creatureID.IsZero() {
		return fmt.Errorf("set creature stat %q: creature id is required", key)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("set creature %q stat: key is required", creatureID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	creature, ok := w.creatures[creatureID]
	if !ok {
		return fmt.Errorf("set creature %q stat %q: creature not found", creatureID, key)
	}
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats[key] = value
	w.creatures[creatureID] = creature

	// B: Mark dirty on stat mutation (preferred pattern)
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return nil
}

// SetCreatureProperty sets a string creature property, creating the property
// map when needed. An empty value removes the property.
func (w *World) SetCreatureProperty(creatureID model.CreatureID, key string, value string) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("set creature %q property %q: world state is nil", creatureID, key)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("set creature property %q: creature id is required", key)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return model.Creature{}, fmt.Errorf("set creature %q property: key is required", creatureID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("set creature %q property %q: creature not found", creatureID, key)
	}
	if value == "" {
		delete(creature.Properties, key)
		if len(creature.Properties) == 0 {
			creature.Properties = nil
		}
		w.creatures[creatureID] = creature
		if !creature.PlayerID.IsZero() {
			w.MarkPlayerDirty(creature.PlayerID)
		}
		return cloneCreature(creature), nil
	}
	if creature.Properties == nil {
		creature.Properties = map[string]string{}
	}
	creature.Properties[key] = value
	w.creatures[creatureID] = creature
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneCreature(creature), nil
}

// SetCreatureDescription stores the canonical player/creature description on
// the model instead of using the compatibility property override. It removes a
// stale description property so command rendering sees this direct value.
func (w *World) SetCreatureDescription(creatureID model.CreatureID, description string) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("set creature %q description: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("set creature description: creature id is required")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("set creature %q description: creature not found", creatureID)
	}
	creature.Description = description
	delete(creature.Properties, stateCreatureDescriptionProperty)
	if len(creature.Properties) == 0 {
		creature.Properties = nil
	}
	w.creatures[creatureID] = creature
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneCreature(creature), nil
}

// SetCreaturePasswordHash stores the legacy password hash in the canonical
// property key used by the account command ports. An empty hash removes it.
func (w *World) SetCreaturePasswordHash(creatureID model.CreatureID, hash string) (model.Creature, error) {
	hash = strings.TrimSpace(hash)
	return w.SetCreatureProperty(creatureID, stateCreaturePasswordHashKey, hash)
}

// SetCreatureClass updates the legacy class stat and recomputes combat stats
// that C recalculated after class-sensitive mutations.
func (w *World) SetCreatureClass(creatureID model.CreatureID, class int) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("set creature %q class: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("set creature class: creature id is required")
	}
	if class < 0 {
		return model.Creature{}, fmt.Errorf("set creature %q class: class must be non-negative", creatureID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("set creature %q class: creature not found", creatureID)
	}
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats["class"] = class
	w.recalculateCreatureCombatStatsLocked(&creature)
	w.creatures[creatureID] = creature
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneCreature(creature), nil
}

// RecalculateCreatureAC recomputes and stores the runtime armor stat using the
// same inputs as the legacy compute_ac path: attributes, equipped armor, and
// active protection flags.
func (w *World) RecalculateCreatureAC(creatureID model.CreatureID) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("recalculate creature %q ac: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("recalculate creature ac: creature id is required")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("recalculate creature %q ac: creature not found", creatureID)
	}
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats["armor"] = w.computeCreatureACLocked(creature)
	w.creatures[creatureID] = creature
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneCreature(creature), nil
}

// RecalculateCreatureTHACO recomputes and stores the runtime thaco stat using
// the legacy class/level table plus weapon, proficiency, and active flags.
func (w *World) RecalculateCreatureTHACO(creatureID model.CreatureID) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("recalculate creature %q thaco: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("recalculate creature thaco: creature id is required")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("recalculate creature %q thaco: creature not found", creatureID)
	}
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats["thaco"] = w.computeCreatureTHACOLocked(creature)
	w.creatures[creatureID] = creature
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneCreature(creature), nil
}

// RecalculateCreatureCombatStats recomputes AC and THACO in one state mutation.
func (w *World) RecalculateCreatureCombatStats(creatureID model.CreatureID) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("recalculate creature %q combat stats: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("recalculate creature combat stats: creature id is required")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("recalculate creature %q combat stats: creature not found", creatureID)
	}
	w.recalculateCreatureCombatStatsLocked(&creature)
	w.creatures[creatureID] = creature
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneCreature(creature), nil
}

// PreparePlayerSuicide marks a player as confirmed for deletion without
// removing files or runtime records. The destructive part can be implemented
// behind a command sink after bank/alias/family cleanup is wired.
func (w *World) PreparePlayerSuicide(playerID model.PlayerID, requestedAt int64) (model.Player, model.Creature, error) {
	if w == nil {
		return model.Player{}, model.Creature{}, fmt.Errorf("prepare player %q suicide: world state is nil", playerID)
	}
	if playerID.IsZero() {
		return model.Player{}, model.Creature{}, fmt.Errorf("prepare player suicide: player id is required")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	player, ok := w.players[playerID]
	if !ok {
		return model.Player{}, model.Creature{}, fmt.Errorf("prepare player %q suicide: player not found", playerID)
	}
	if player.CreatureID.IsZero() {
		return model.Player{}, model.Creature{}, fmt.Errorf("prepare player %q suicide: creature id is required", playerID)
	}
	creature, ok := w.creatures[player.CreatureID]
	if !ok {
		return model.Player{}, model.Creature{}, fmt.Errorf("prepare player %q suicide: creature %q not found", playerID, player.CreatureID)
	}

	player.Metadata.Tags = addMetadataTags(player.Metadata.Tags, []string{stateSuicidePendingTag})
	creature.Metadata.Tags = addMetadataTags(creature.Metadata.Tags, []string{stateSuicidePendingTag})
	if creature.Properties == nil {
		creature.Properties = map[string]string{}
	}
	creature.Properties[stateSuicideRequestedAtProperty] = strconv.FormatInt(requestedAt, 10)

	w.players[playerID] = player
	w.creatures[creature.ID] = creature
	w.MarkPlayerDirty(playerID)
	return clonePlayer(player), cloneCreature(creature), nil
}

// UpdateCreatureFamilyState replaces the legacy family membership flags for a
// creature. It updates canonical and legacy stat names together and removes
// stale property/tag aliases so command helpers cannot read an old family state.
func (w *World) UpdateCreatureFamilyState(creatureID model.CreatureID, familyID int, member bool, pending bool, boss bool) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("update creature %q family state: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("update creature family state: creature id is required")
	}
	if familyID < 0 {
		return model.Creature{}, fmt.Errorf("update creature %q family state: family id cannot be negative", creatureID)
	}
	if member && pending {
		return model.Creature{}, fmt.Errorf("update creature %q family state: member and pending are exclusive", creatureID)
	}
	if (member || pending) && familyID <= 0 {
		return model.Creature{}, fmt.Errorf("update creature %q family state: active family id is required", creatureID)
	}
	if boss && !member {
		return model.Creature{}, fmt.Errorf("update creature %q family state: boss requires membership", creatureID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("update creature %q family state: creature not found", creatureID)
	}
	updateCreatureFamilyStateLocked(&creature, familyID, member, pending, boss)
	w.creatures[creatureID] = creature
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneCreature(creature), nil
}

// UpdateCreatureGold adds (or subtracts if negative) the amount of gold to/from a creature.
func (w *World) UpdateCreatureGold(creatureID model.CreatureID, amount int) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("update gold: world is nil")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("update gold: creature %q not found", creatureID)
	}
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats["gold"] += amount
	w.creatures[creatureID] = creature
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}
	return cloneCreature(creature), nil
}

// UpdateFamilyMembers updates the in-memory members list of a family.
func (w *World) UpdateFamilyMembers(familyID int, members []model.FamilyMember) error {
	if w == nil {
		return fmt.Errorf("update family members: world is nil")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	family, ok := w.families[familyID]
	if !ok {
		// Try finding by slot too, just in case
		found := false
		for id, f := range w.families {
			if f.Slot == familyID {
				family = f
				familyID = id
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("update family members: family %d not found", familyID)
		}
	}
	family.Members = members
	w.families[familyID] = family
	return nil
}

// UpdateFamilyMemberAfterClassChange mirrors C edit_member(name, class,
// daily[DL_EXPND].max, 3): update an existing family_member_N row's class
// after a successful class change. Missing family/member rows are safe no-ops
// because the class change itself has already succeeded in the caller.
func (w *World) UpdateFamilyMemberAfterClassChange(name string, class int, dailyExpndMax int) error {
	if w == nil {
		return fmt.Errorf("update family member after class change: world is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" || dailyExpndMax <= 0 {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	familyID, family, ok := w.familyByLegacyNumberLocked(dailyExpndMax)
	if !ok {
		return nil
	}
	for i := range family.Members {
		if strings.TrimSpace(family.Members[i].DisplayName) != name {
			continue
		}
		family.Members[i].Class = class
		if family.Members[i].Metadata.RawFields != nil {
			family.Members[i].Metadata.RawFields["line"] = []byte(fmt.Sprintf("%d %s", class, family.Members[i].DisplayName))
		}
		w.families[familyID] = family
		return nil
	}
	return nil
}

func (w *World) familyByLegacyNumberLocked(familyID int) (int, model.Family, bool) {
	family, ok := w.families[familyID]
	if ok {
		return familyID, family, true
	}
	for id, family := range w.families {
		if family.Slot == familyID {
			return id, family, true
		}
	}
	return 0, model.Family{}, false
}

// UpdateFamily updates a family in the world state.
func (w *World) UpdateFamily(family model.Family) error {
	if w == nil {
		return fmt.Errorf("update family: world is nil")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.families[family.ID] = family
	return nil
}

// UpdatePlayer updates a player in the world state.
func (w *World) UpdatePlayer(player model.Player) error {
	if w == nil {
		return fmt.Errorf("update player: world is nil")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.players[player.ID] = player
	return nil
}

// UpdateObjectInstance updates an object instance in the world state.
func (w *World) UpdateObjectInstance(object model.ObjectInstance) error {
	if w == nil {
		return fmt.Errorf("update object: world is nil")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.objects[object.ID] = object
	return nil
}

// UpdateCreature updates a creature in the world state.
func (w *World) UpdateCreature(creature model.Creature) error {
	if w == nil {
		return fmt.Errorf("update creature: world is nil")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.creatures[creature.ID] = creature
	return nil
}

// CreatePlayerCharacter inserts a newly created player and linked creature,
// then attaches both to their starting room in one runtime-state mutation.
func (w *World) CreatePlayerCharacter(player model.Player, creature model.Creature) error {
	if w == nil {
		return fmt.Errorf("create player character: world is nil")
	}
	if player.ID.IsZero() {
		return fmt.Errorf("create player character: player id is required")
	}
	if creature.ID.IsZero() {
		return fmt.Errorf("create player character %q: creature id is required", player.ID)
	}
	if player.CreatureID.IsZero() {
		player.CreatureID = creature.ID
	}
	if creature.PlayerID.IsZero() {
		creature.PlayerID = player.ID
	}
	if player.CreatureID != creature.ID {
		return fmt.Errorf("create player character %q: player creature %q does not match %q", player.ID, player.CreatureID, creature.ID)
	}
	if creature.PlayerID != player.ID {
		return fmt.Errorf("create player character %q: creature owner %q does not match", player.ID, creature.PlayerID)
	}
	if creature.Kind == "" {
		creature.Kind = model.CreatureKindPlayer
	}
	if creature.Kind != model.CreatureKindPlayer {
		return fmt.Errorf("create player character %q: creature kind must be player", player.ID)
	}
	if player.RoomID.IsZero() {
		player.RoomID = creature.RoomID
	}
	if creature.RoomID.IsZero() {
		creature.RoomID = player.RoomID
	}
	if player.RoomID.IsZero() || creature.RoomID.IsZero() || player.RoomID != creature.RoomID {
		return fmt.Errorf("create player character %q: player and creature must share a starting room", player.ID)
	}
	if err := player.Validate(); err != nil {
		return fmt.Errorf("create player character %q: %w", player.ID, err)
	}
	if err := creature.Validate(); err != nil {
		return fmt.Errorf("create player character %q: %w", player.ID, err)
	}

	w.mu.Lock()

	if _, exists := w.players[player.ID]; exists {
		w.mu.Unlock()
		return fmt.Errorf("create player character %q: player already exists", player.ID)
	}
	if _, exists := w.creatures[creature.ID]; exists {
		w.mu.Unlock()
		return fmt.Errorf("create player character %q: creature %q already exists", player.ID, creature.ID)
	}
	room, ok := w.rooms[player.RoomID]
	if !ok {
		w.mu.Unlock()
		return fmt.Errorf("create player character %q: starting room %q not found", player.ID, player.RoomID)
	}

	player = clonePlayer(player)
	creature = cloneCreature(creature)
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	w.recalculateCreatureCombatStatsLocked(&creature)

	w.players[player.ID] = player
	w.creatures[creature.ID] = creature
	room.PlayerIDs = w.insertPlayerIDLegacySortedLocked(room.PlayerIDs, player.ID)
	room.CreatureIDs = w.insertCreatureIDLegacySortedLocked(room.CreatureIDs, creature.ID)
	w.rooms[room.ID] = room
	w.mu.Unlock()

	w.MarkPlayerDirty(player.ID)
	return nil
}

func updateCreatureFamilyStateLocked(creature *model.Creature, familyID int, member bool, pending bool, boss bool) {
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	memberValue := boolInt(member)
	pendingValue := boolInt(pending)
	bossValue := boolInt(member && boss)
	activeFamilyID := 0
	if member || pending {
		activeFamilyID = familyID
	}

	for _, key := range []string{"familyFlag", "PFAMIL"} {
		creature.Stats[key] = memberValue
	}
	for _, key := range []string{"familyID", "dailyExpndMax", "legacyDailyExpndMax"} {
		creature.Stats[key] = activeFamilyID
	}
	creature.Stats["PRDFML"] = pendingValue
	for _, key := range []string{"PFMBOS", "familyBoss", "familyBossFlag"} {
		creature.Stats[key] = bossValue
	}

	removeCreatureFamilyStateProperties(creature)
	creature.Metadata.Tags = removeMetadataTags(creature.Metadata.Tags, creatureFamilyStateTagNames())
	var addTags []string
	if member {
		addTags = append(addTags, "PFAMIL")
	}
	if pending {
		addTags = append(addTags, "PRDFML")
	}
	if member && boss {
		addTags = append(addTags, "PFMBOS")
	}
	creature.Metadata.Tags = addMetadataTags(creature.Metadata.Tags, addTags)
}

func removeCreatureFamilyStateProperties(creature *model.Creature) {
	if len(creature.Properties) == 0 {
		return
	}
	targets := normalizedFlagSet(creatureFamilyStatePropertyNames()...)
	for key := range creature.Properties {
		if _, ok := targets[normalizeFlagName(key)]; ok {
			delete(creature.Properties, key)
		}
	}
	if len(creature.Properties) == 0 {
		creature.Properties = nil
	}
}

func creatureFamilyStatePropertyNames() []string {
	return []string{
		"familyFlag", "PFAMIL",
		"familyID", "dailyExpndMax", "legacyDailyExpndMax",
		"PRDFML",
		"PFMBOS", "familyBoss", "familyBossFlag",
	}
}

func creatureFamilyStateTagNames() []string {
	return []string{
		"familyFlag", "PFAMIL",
		"PRDFML",
		"PFMBOS", "familyBoss", "familyBossFlag",
	}
}

// UpdateCreatureMarriageState replaces the legacy runtime marriage flags for a
// creature. It updates canonical and legacy stat names together and removes
// stale property/tag aliases so command helpers cannot read an old marriage state.
func (w *World) UpdateCreatureMarriageState(creatureID model.CreatureID, marriageID int, married bool, pending bool) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("update creature %q marriage state: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("update creature marriage state: creature id is required")
	}
	if marriageID < 0 {
		return model.Creature{}, fmt.Errorf("update creature %q marriage state: marriage id cannot be negative", creatureID)
	}
	if married && pending {
		return model.Creature{}, fmt.Errorf("update creature %q marriage state: married and pending are exclusive", creatureID)
	}
	if married && marriageID <= 0 {
		return model.Creature{}, fmt.Errorf("update creature %q marriage state: active marriage id is required", creatureID)
	}
	if !married {
		marriageID = 0
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("update creature %q marriage state: creature not found", creatureID)
	}
	updateCreatureMarriageStateLocked(&creature, marriageID, married, pending)
	w.creatures[creatureID] = creature
	return cloneCreature(creature), nil
}

func updateCreatureMarriageStateLocked(creature *model.Creature, marriageID int, married bool, pending bool) {
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	marriedValue := boolInt(married)
	pendingValue := boolInt(pending)
	for _, key := range []string{"PMARRI", "married", "marriageFlag"} {
		creature.Stats[key] = marriedValue
	}
	for _, key := range []string{"marriageID", "dailyMarriageMax", "legacyDailyMarriageMax"} {
		creature.Stats[key] = marriageID
	}
	for _, key := range []string{"PRDMAR", "marriagePending"} {
		creature.Stats[key] = pendingValue
	}

	removeCreatureMarriageStateProperties(creature)
	creature.Metadata.Tags = removeMetadataTags(creature.Metadata.Tags, creatureMarriageStateTagNames())
	var addTags []string
	if married {
		addTags = append(addTags, "PMARRI", "married")
	}
	if pending {
		addTags = append(addTags, "PRDMAR")
	}
	creature.Metadata.Tags = addMetadataTags(creature.Metadata.Tags, addTags)
}

func removeCreatureMarriageStateProperties(creature *model.Creature) {
	if len(creature.Properties) == 0 {
		return
	}
	targets := normalizedFlagSet(creatureMarriageStatePropertyNames()...)
	for key := range creature.Properties {
		if _, ok := targets[normalizeFlagName(key)]; ok {
			delete(creature.Properties, key)
		}
	}
	if len(creature.Properties) == 0 {
		creature.Properties = nil
	}
}

func creatureMarriageStatePropertyNames() []string {
	return []string{
		"PMARRI", "married", "marriage", "marriageFlag",
		"marriageID", "dailyMarriageMax", "legacyDailyMarriageMax",
		"PRDMAR", "marriagePending",
	}
}

func creatureMarriageStateTagNames() []string {
	return []string{
		"PMARRI", "married", "marriage", "marriageFlag",
		"PRDMAR", "marriagePending",
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (w *World) recalculateCreatureCombatStatsLocked(creature *model.Creature) {
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats["armor"] = w.computeCreatureACLocked(*creature)
	creature.Stats["thaco"] = w.computeCreatureTHACOLocked(*creature)
}

func (w *World) computeCreatureACLocked(creature model.Creature) int {
	ac := 100

	constitution := creatureStateInt(creature, "constitution")
	if constitution > 95 {
		ac -= 5 * stateLegacyStatBonus(90)
	} else {
		ac -= 5 * (stateLegacyStatBonus(constitution) + 4)
	}

	dexterity := creatureStateInt(creature, "dexterity")
	if dexterity > 95 {
		ac -= 2 * stateLegacyStatBonus(90)
	} else {
		ac -= 2 * (stateLegacyStatBonus(dexterity) + 4)
	}

	for _, objectID := range creature.Equipment {
		if objectID.IsZero() {
			continue
		}
		object, ok := w.objects[objectID]
		if !ok {
			continue
		}
		if armor, ok := w.objectIntPropertyLocked(object, "armor"); ok {
			ac -= armor
		}
	}

	if creatureHasAnyFlag(creature, "PPROTE", "protect") {
		ac -= 10
	}
	if creatureHasAnyFlag(creature, "PREFLECT", "reflect", "reflection") {
		ac -= 15
	}
	if creatureHasAnyFlag(creature, "PSHADOW", "shadow", "shadowClone") {
		ac -= 20
	}
	if creatureHasAnyFlag(creature, "PABSORB", "absorb") {
		ac -= 10
	}
	if creatureHasAnyFlag(creature, "PCHOI", "choi") {
		ac += 20
	}

	if creatureStateClass(creature) >= stateLegacyClassBulsa {
		ac -= 10
		if constitution > 45 {
			ac -= constitution - 45
		}
	}
	return clampInt(ac, -127, 127)
}

func (w *World) computeCreatureTHACOLocked(creature model.Creature) int {
	level := creatureStateLevel(creature)
	index := (level + 3) / 4
	if index > 20 {
		index = 19
	} else if index > 0 {
		index--
	} else {
		index = 0
	}

	class := creatureStateClass(creature)
	thaco := 20
	if class >= 0 && class < len(stateLegacyTHACOList) {
		thaco = stateLegacyTHACOList[class][index]
	}

	if weapon, ok := w.creatureWieldedObjectLocked(creature); ok {
		if adjustment, ok := w.objectIntPropertyLocked(weapon, "adjustment"); ok {
			thaco -= adjustment
		}
	}
	thaco -= w.creatureModifiedWeaponProficiencyLocked(creature)

	proficiencySum := 0
	// C sums SHARP..POLE, then mprofic(0)..mprofic(3). mprofic(0)
	// mirrors the legacy realm[-1] memory layout and reads missile proficiency.
	for i := 0; i < 4; i++ {
		proficiencySum += stateCreatureWeaponProficiency(creature, i)
	}
	for i := 0; i < 4; i++ {
		proficiencySum += stateCreatureMagicProficiency(creature, i)
	}
	thaco -= proficiencySum / 50

	if creatureHasAnyFlag(creature, "PBLESS", "bless") {
		thaco -= 3
	}
	if creatureHasAnyFlag(creature, "PREFLECT", "reflect", "reflection") {
		thaco -= 1
	}
	if creatureHasAnyFlag(creature, "PSHADOW", "shadow", "shadowClone") {
		thaco -= 3
	}
	if creatureHasAnyFlag(creature, "PABSORB", "absorb") {
		thaco -= 2
	}
	if creatureHasAnyFlag(creature, "PCHOI", "choi") {
		thaco += 5
	}
	if creatureHasAnyFlag(creature, "PSLAYE", "slaye", "accurate", "slayer") {
		thaco -= 3
	}
	if class == stateLegacyClassDM {
		thaco -= 60
	}
	if class == stateLegacyClassBulsa {
		thaco -= 14
	}
	return thaco
}

func (w *World) creatureModifiedWeaponProficiencyLocked(creature model.Creature) int {
	divisor := 40
	switch creatureStateClass(creature) {
	case stateLegacyClassFighter, stateLegacyClassBarbarian, stateLegacyClassInvincible, stateLegacyClassCaretaker:
		divisor = 20
	case stateLegacyClassRanger, stateLegacyClassPaladin:
		divisor = 25
	case stateLegacyClassThief, stateLegacyClassAssassin, stateLegacyClassCleric:
		divisor = 30
	}

	weaponType := 2
	if weapon, ok := w.creatureWieldedObjectLocked(creature); ok {
		if value, ok := w.objectIntPropertyLocked(weapon, "type"); ok && value >= 0 && value <= 4 {
			weaponType = value
		}
	}
	return stateCreatureWeaponProficiency(creature, weaponType) / divisor
}

func (w *World) creatureWieldedObjectLocked(creature model.Creature) (model.ObjectInstance, bool) {
	for _, slot := range []string{"wield", "weapon", "mainHand", "right"} {
		objectID := creature.Equipment[slot]
		if objectID.IsZero() {
			continue
		}
		object, ok := w.objects[objectID]
		if ok {
			return object, true
		}
	}
	return model.ObjectInstance{}, false
}

func creatureStateLevel(creature model.Creature) int {
	if level, ok := creature.Stats["level"]; ok {
		return level
	}
	if creature.Level != 0 {
		return creature.Level
	}
	if level, ok := stateCreatureIntValue(creature, "level"); ok {
		return level
	}
	return 0
}

func creatureStateClass(creature model.Creature) int {
	return creatureStateInt(creature, "class")
}

func creatureStateInt(creature model.Creature, key string) int {
	if value, ok := stateCreatureIntValue(creature, key); ok {
		return value
	}
	return 0
}

func stateLegacyStatBonus(stat int) int {
	stat = clampInt(stat, 0, len(stateLegacyBonusTable)-1)
	return stateLegacyBonusTable[stat]
}

func stateCreatureWeaponProficiency(creature model.Creature, index int) int {
	value := stateCreatureProficiencyValue(creature, index)
	var table [12]int64
	switch creatureStateClass(creature) {
	case stateLegacyClassFighter, stateLegacyClassInvincible, stateLegacyClassCaretaker, stateLegacyClassBulsa, stateLegacyClassSubDM, stateLegacyClassDM:
		table = [12]int64{0, 768, 1024, 1440, 1910, 16000, 31214, 167000, 268488, 695000, 934808, 500000000}
	case stateLegacyClassBarbarian:
		table = [12]int64{0, 1536, 2048, 2880, 3820, 32000, 62428, 334000, 536976, 1390000, 1869616, 500000000}
	case stateLegacyClassThief, stateLegacyClassRanger:
		table = [12]int64{0, 2304, 3072, 4320, 5730, 48000, 93642, 501000, 805464, 2085000, 2804424, 500000000}
	case stateLegacyClassCleric, stateLegacyClassPaladin, stateLegacyClassAssassin:
		table = [12]int64{0, 3072, 4096, 5076, 7640, 64000, 124856, 668000, 1073952, 2780000, 3939232, 500000000}
	default:
		table = [12]int64{0, 5376, 7168, 10080, 13370, 112000, 218498, 1169000, 1879416, 4865000, 6543656, 500000000}
	}
	return stateProficiencyRank(value, table)
}

func stateCreatureMagicProficiency(creature model.Creature, index int) int {
	value := 0
	if index == 0 {
		value = stateCreatureProficiencyValue(creature, 4)
	} else if index >= 1 && index <= 4 {
		value = stateCreatureRealm(creature, index-1)
	}

	var table [12]int64
	switch creatureStateClass(creature) {
	case stateLegacyClassMage, stateLegacyClassInvincible, stateLegacyClassCaretaker, stateLegacyClassBulsa, stateLegacyClassSubDM, stateLegacyClassDM:
		table = [12]int64{0, 1024, 2048, 4096, 8192, 16384, 35768, 85536, 140000, 459410, 2073306, 500000000}
	case stateLegacyClassCleric:
		table = [12]int64{0, 1024, 4092, 8192, 16384, 32768, 70536, 119000, 226410, 709410, 2973307, 500000000}
	case stateLegacyClassPaladin, stateLegacyClassRanger:
		table = [12]int64{0, 1024, 8192, 16384, 32768, 65536, 105000, 165410, 287306, 809410, 3538232, 500000000}
	default:
		table = [12]int64{0, 1024, 40000, 80000, 120000, 160000, 205000, 222000, 380000, 965410, 5495000, 500000000}
	}
	return stateProficiencyRank(value, table)
}

func stateProficiencyRank(value int, table [12]int64) int {
	rank := 100
	i := 10
	for i = 0; i < 11; i++ {
		if int64(value) < table[i+1] {
			rank = 10 * i
			break
		}
	}
	if table[i+1] > table[i] {
		rank += int((int64(value) - table[i]) * 10 / (table[i+1] - table[i]))
	}
	return rank
}

func stateCreatureProficiencyValue(creature model.Creature, index int) int {
	if index >= 0 && index < len(weaponProficiencyStatKeys) {
		part := weaponProficiencyPropertyKeys[index]
		for _, key := range []string{
			weaponProficiencyStatKeys[index],
			fmt.Sprintf("proficiency/%s", part),
			fmt.Sprintf("proficiency.%s", part),
			fmt.Sprintf("proficiency_%s", part),
		} {
			if value, ok := stateCreatureIntValue(creature, key); ok {
				return value
			}
		}
	}
	return stateCreatureIndexedValue(creature, "proficiency", index)
}

func stateCreatureIntValue(creature model.Creature, key string) (int, bool) {
	if creature.Stats != nil {
		if value, ok := creature.Stats[key]; ok {
			return value, true
		}
	}
	if creature.Properties != nil {
		if raw, ok := creature.Properties[key]; ok {
			value, err := strconv.Atoi(strings.TrimSpace(raw))
			if err == nil {
				return value, true
			}
		}
	}
	target := normalizeFlagName(key)
	if target == "" {
		return 0, false
	}
	for statKey, value := range creature.Stats {
		if normalizeFlagName(statKey) == target {
			return value, true
		}
	}
	for propertyKey, raw := range creature.Properties {
		if normalizeFlagName(propertyKey) == target {
			value, err := strconv.Atoi(strings.TrimSpace(raw))
			if err == nil {
				return value, true
			}
		}
	}
	return 0, false
}

func stateCreatureRealm(creature model.Creature, index int) int {
	keys := []string{"realmEarth", "realmWind", "realmFire", "realmWater"}
	if index >= 0 && index < len(keys) {
		if value, ok := creature.Stats[keys[index]]; ok {
			return value
		}
	}
	return stateCreatureIndexedValue(creature, "realm", index+1)
}

func stateCreatureIndexedValue(creature model.Creature, prefix string, index int) int {
	keys := []string{
		fmt.Sprintf("%s/%d", prefix, index),
		fmt.Sprintf("%s.%d", prefix, index),
		fmt.Sprintf("%s_%d", prefix, index),
		fmt.Sprintf("%s%d", prefix, index),
	}
	for _, key := range keys {
		if creature.Stats != nil {
			if value, ok := creature.Stats[key]; ok {
				return value
			}
		}
		if creature.Properties != nil {
			if raw, ok := creature.Properties[key]; ok {
				value, err := strconv.Atoi(strings.TrimSpace(raw))
				if err == nil {
					return value
				}
			}
		}
	}
	return 0
}

var stateLegacyBonusTable = [...]int{
	-4, -4, -4, -3, -3, -2, -2, -1, -1, -1,
	0, 0, 0, 0, 1, 1, 1, 2, 2, 2,
	3, 3, 3, 3, 4, 4, 4, 4, 4, 5,
	5, 5, 5, 5, 5, 6, 6, 6, 6, 6,
	6, 6, 6, 6, 7, 7, 7, 7, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 8, 8, 8,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
	9, 9, 9, 9, 9, 9,
}

var stateLegacyTHACOList = [...][20]int{
	{20, 20, 20, 20, 20, 20, 20, 20, 20, 20, 20, 20, 20, 20, 20, 20, 20, 20, 20, 20},
	{18, 18, 18, 17, 17, 16, 16, 15, 15, 14, 14, 13, 13, 12, 12, 11, 10, 10, 9, 9},
	{20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 3, 2},
	{20, 20, 19, 18, 18, 17, 16, 16, 15, 14, 14, 13, 13, 12, 12, 11, 10, 10, 9, 8},
	{20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 3, 3},
	{20, 20, 19, 19, 18, 18, 18, 17, 17, 16, 16, 16, 15, 15, 14, 14, 14, 13, 13, 11},
	{19, 19, 18, 18, 17, 16, 16, 15, 15, 14, 14, 13, 13, 12, 11, 11, 10, 9, 8, 7},
	{19, 19, 18, 17, 16, 16, 15, 15, 14, 14, 13, 12, 12, 11, 11, 10, 9, 9, 8, 7},
	{20, 20, 19, 19, 18, 18, 17, 17, 16, 16, 15, 15, 14, 14, 13, 13, 12, 12, 11, 11},
	{15, 15, 14, 14, 13, 13, 12, 12, 11, 11, 10, 10, 9, 9, 8, 8, 7, 6, 5, 5},
	{12, 12, 11, 11, 10, 10, 9, 9, 8, 8, 7, 7, 6, 6, 5, 5, 4, 4, 3, 0},
	{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
	{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
	{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
	{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
}

// SetCreatureLevel sets the canonical creature level and mirrors it into the
// legacy numeric stat map used by many command ports.
func (w *World) SetCreatureLevel(creatureID model.CreatureID, level int) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("set creature %q level: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("set creature level: creature id is required")
	}
	if level < 0 {
		return model.Creature{}, fmt.Errorf("set creature %q level: level must be non-negative", creatureID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("set creature %q level: creature not found", creatureID)
	}
	creature.Level = level
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats["level"] = level
	w.creatures[creatureID] = creature
	return cloneCreature(creature), nil
}

// UseCreatureCooldown starts a runtime cooldown when it is available. It
// returns the remaining seconds and false when the cooldown is still active.
func (w *World) UseCreatureCooldown(creatureID model.CreatureID, key string, nowUnix int64, intervalSeconds int64) (int64, bool, error) {
	if w == nil {
		return 0, false, fmt.Errorf("use creature %q cooldown %q: world state is nil", creatureID, key)
	}
	if creatureID.IsZero() {
		return 0, false, fmt.Errorf("use creature cooldown %q: creature id is required", key)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return 0, false, fmt.Errorf("use creature %q cooldown: key is required", creatureID)
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, ok := w.creatures[creatureID]; !ok {
		return 0, false, fmt.Errorf("use creature %q cooldown %q: creature not found", creatureID, key)
	}
	if w.cooldowns == nil {
		w.cooldowns = map[model.CreatureID]map[string]int64{}
	}
	expiresByKey := w.cooldowns[creatureID]
	if expiresByKey == nil {
		expiresByKey = map[string]int64{}
		w.cooldowns[creatureID] = expiresByKey
	}
	if expires := expiresByKey[key]; expires > nowUnix {
		return expires - nowUnix, false, nil
	}
	if intervalSeconds <= 0 {
		return 0, true, nil
	}
	expiresByKey[key] = nowUnix + intervalSeconds
	return 0, true, nil
}

// SetCreatureCooldown sets or replaces a runtime cooldown for a creature.
func (w *World) SetCreatureCooldown(creatureID model.CreatureID, key string, nowUnix int64, intervalSeconds int64) error {
	if w == nil {
		return fmt.Errorf("set creature %q cooldown %q: world state is nil", creatureID, key)
	}
	if creatureID.IsZero() {
		return fmt.Errorf("set creature cooldown %q: creature id is required", key)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("set creature %q cooldown: key is required", creatureID)
	}
	if intervalSeconds <= 0 {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if _, ok := w.creatures[creatureID]; !ok {
		return fmt.Errorf("set creature %q cooldown %q: creature not found", creatureID, key)
	}
	if w.cooldowns == nil {
		w.cooldowns = map[model.CreatureID]map[string]int64{}
	}
	expiresByKey := w.cooldowns[creatureID]
	if expiresByKey == nil {
		expiresByKey = map[string]int64{}
		w.cooldowns[creatureID] = expiresByKey
	}
	expiresByKey[key] = nowUnix + intervalSeconds
	return nil
}

// CreatureCooldownExpires reports the stored absolute expiration time for a
// creature cooldown without starting or extending the timer.
func (w *World) CreatureCooldownExpires(creatureID model.CreatureID, key string) (int64, bool, error) {
	if w == nil {
		return 0, false, fmt.Errorf("get creature %q cooldown %q: world state is nil", creatureID, key)
	}
	if creatureID.IsZero() {
		return 0, false, fmt.Errorf("get creature cooldown %q: creature id is required", key)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return 0, false, fmt.Errorf("get creature %q cooldown: key is required", creatureID)
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	if _, ok := w.creatures[creatureID]; !ok {
		return 0, false, fmt.Errorf("get creature %q cooldown %q: creature not found", creatureID, key)
	}
	if w.cooldowns == nil || w.cooldowns[creatureID] == nil {
		return 0, false, nil
	}
	expires, ok := w.cooldowns[creatureID][key]
	return expires, ok, nil
}

// UpdateCreatureTags adds and removes creature metadata tags under the world
// lock. Tag matching for removals uses the same normalized legacy flag
// comparison as command visibility checks.
func (w *World) UpdateCreatureTags(creatureID model.CreatureID, add []string, remove []string) (model.Creature, error) {
	if w == nil {
		return model.Creature{}, fmt.Errorf("update creature %q tags: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, fmt.Errorf("update creature tags: creature id is required")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, fmt.Errorf("update creature %q tags: creature not found", creatureID)
	}
	creature.Metadata.Tags = addMetadataTags(removeMetadataTags(creature.Metadata.Tags, remove), add)
	w.creatures[creatureID] = creature
	return cloneCreature(creature), nil
}

// UpdatePlayerTags adds and removes player metadata tags under the world lock.
func (w *World) UpdatePlayerTags(playerID model.PlayerID, add []string, remove []string) (model.Player, error) {
	if w == nil {
		return model.Player{}, fmt.Errorf("update player %q tags: world state is nil", playerID)
	}
	if playerID.IsZero() {
		return model.Player{}, fmt.Errorf("update player tags: player id is required")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	player, ok := w.players[playerID]
	if !ok {
		return model.Player{}, fmt.Errorf("update player %q tags: player not found", playerID)
	}
	player.Metadata.Tags = addMetadataTags(removeMetadataTags(player.Metadata.Tags, remove), add)
	w.players[playerID] = player
	return clonePlayer(player), nil
}

// UpdateObjectTags adds and removes object metadata tags under the world lock.
func (w *World) UpdateObjectTags(objectID model.ObjectInstanceID, add []string, remove []string) (model.ObjectInstance, error) {
	if w == nil {
		return model.ObjectInstance{}, fmt.Errorf("update object %q tags: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return model.ObjectInstance{}, fmt.Errorf("update object tags: object id is required")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	object, ok := w.objects[objectID]
	if !ok {
		return model.ObjectInstance{}, fmt.Errorf("update object %q tags: object not found", objectID)
	}
	object.Metadata.Tags = addMetadataTags(removeMetadataTags(object.Metadata.Tags, remove), add)
	w.objects[objectID] = object
	return cloneObject(object), nil
}

// SetObjectDisplayName sets an instance-specific display name.
func (w *World) SetObjectDisplayName(objectID model.ObjectInstanceID, name string) (model.ObjectInstance, error) {
	if w == nil {
		return model.ObjectInstance{}, fmt.Errorf("set object %q display name: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return model.ObjectInstance{}, fmt.Errorf("set object display name: object id is required")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	object, ok := w.objects[objectID]
	if !ok {
		return model.ObjectInstance{}, fmt.Errorf("set object %q display name: object not found", objectID)
	}
	object.DisplayNameOverride = name
	w.objects[objectID] = object
	return cloneObject(object), nil
}

// SetObjectProperty sets a string object instance property. An empty value
// removes the property.
func (w *World) SetObjectProperty(objectID model.ObjectInstanceID, key string, value string) (model.ObjectInstance, error) {
	if w == nil {
		return model.ObjectInstance{}, fmt.Errorf("set object %q property %q: world state is nil", objectID, key)
	}
	if objectID.IsZero() {
		return model.ObjectInstance{}, fmt.Errorf("set object property %q: object id is required", key)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return model.ObjectInstance{}, fmt.Errorf("set object %q property: key is required", objectID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	object, ok := w.objects[objectID]
	if !ok {
		return model.ObjectInstance{}, fmt.Errorf("set object %q property %q: object not found", objectID, key)
	}
	if value == "" {
		delete(object.Properties, key)
	} else {
		if object.Properties == nil {
			object.Properties = map[string]string{}
		}
		object.Properties[key] = value
	}
	w.objects[objectID] = object
	return cloneObject(object), nil
}

// SetExitFlag enables or disables a room exit flag under the world lock.
func (w *World) SetExitFlag(roomID model.RoomID, exitName string, flag string, enabled bool) (model.Exit, error) {
	if w == nil {
		return model.Exit{}, fmt.Errorf("set exit %q flag %q: world state is nil", exitName, flag)
	}
	if roomID.IsZero() {
		return model.Exit{}, fmt.Errorf("set exit %q flag %q: room id is required", exitName, flag)
	}
	exitName = strings.TrimSpace(exitName)
	if exitName == "" {
		return model.Exit{}, fmt.Errorf("set exit flag %q: exit name is required", flag)
	}
	flag = strings.TrimSpace(flag)
	if flag == "" {
		return model.Exit{}, fmt.Errorf("set exit %q flag: flag is required", exitName)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	room, ok := w.rooms[roomID]
	if !ok {
		return model.Exit{}, fmt.Errorf("set exit %q flag %q: room %q not found", exitName, flag, roomID)
	}
	for i := range room.Exits {
		if room.Exits[i].Name != exitName {
			continue
		}
		exit := room.Exits[i]
		exit.Flags = setExitFlag(exit.Flags, flag, enabled)
		room.Exits[i] = exit
		w.rooms[roomID] = room
		return cloneExit(exit), nil
	}
	return model.Exit{}, fmt.Errorf("set exit %q flag %q: exit not found in room %q", exitName, flag, roomID)
}

// TouchExitTimer updates an exit's legacy ltime.ltime value.
func (w *World) TouchExitTimer(roomID model.RoomID, exitName string, nowUnix int64) (model.Exit, error) {
	if w == nil {
		return model.Exit{}, fmt.Errorf("touch exit %q timer: world state is nil", exitName)
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	room, exitIndex, err := w.findExitForUpdateLocked(roomID, exitName, "touch")
	if err != nil {
		return model.Exit{}, err
	}
	exit := setExitTimerLTime(room.Exits[exitIndex], nowUnix)
	room.Exits[exitIndex] = exit
	w.rooms[roomID] = room
	return cloneExit(exit), nil
}

// CheckRoomExits applies the legacy room.c check_exits() timed relock/reclose pass.
func (w *World) CheckRoomExits(roomID model.RoomID, nowUnix int64) error {
	if w == nil {
		return fmt.Errorf("check exits in room %q: world state is nil", roomID)
	}
	if roomID.IsZero() {
		return fmt.Errorf("check exits: room id is required")
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, ok := w.rooms[roomID]; !ok {
		return fmt.Errorf("check exits in room %q: room not found", roomID)
	}
	w.checkRoomExitsLocked(roomID, nowUnix)
	return nil
}

// UnlockExitWithKey unlocks a locked exit and consumes one key charge.
func (w *World) UnlockExitWithKey(roomID model.RoomID, exitName string, keyID model.ObjectInstanceID) (model.Exit, model.ObjectInstance, error) {
	if w == nil {
		return model.Exit{}, model.ObjectInstance{}, fmt.Errorf("unlock exit %q: world state is nil", exitName)
	}
	if keyID.IsZero() {
		return model.Exit{}, model.ObjectInstance{}, fmt.Errorf("unlock exit %q: key id is required", exitName)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	room, exitIndex, err := w.findExitForUpdateLocked(roomID, exitName, "unlock")
	if err != nil {
		return model.Exit{}, model.ObjectInstance{}, err
	}
	exit := room.Exits[exitIndex]
	if !exitHasAnyFlag(exit, "locked", "xlockd", "xlocked") {
		return model.Exit{}, model.ObjectInstance{}, fmt.Errorf("unlock exit %q: exit is not locked", exitName)
	}

	key, err := w.validateExitKeyLocked(keyID, exit)
	if err != nil {
		return model.Exit{}, model.ObjectInstance{}, err
	}
	charges, _ := w.objectIntPropertyLocked(key, "shotsCurrent")
	if charges < 1 {
		return model.Exit{}, model.ObjectInstance{}, fmt.Errorf("unlock exit %q: key %q is broken", exitName, keyID)
	}

	exit.Flags = setExitFlag(exit.Flags, "locked", false)
	exit = setExitTimerLTime(exit, time.Now().Unix())
	room.Exits[exitIndex] = exit
	w.rooms[roomID] = room

	key.Properties = maps.Clone(key.Properties)
	if key.Properties == nil {
		key.Properties = map[string]string{}
	}
	key.Properties["shotsCurrent"] = strconv.Itoa(charges - 1)
	w.objects[key.ID] = key

	return cloneExit(exit), cloneObject(key), nil
}

// LockExitWithKey locks a closed, lockable exit with a matching key.
func (w *World) LockExitWithKey(roomID model.RoomID, exitName string, keyID model.ObjectInstanceID) (model.Exit, model.ObjectInstance, error) {
	if w == nil {
		return model.Exit{}, model.ObjectInstance{}, fmt.Errorf("lock exit %q: world state is nil", exitName)
	}
	if keyID.IsZero() {
		return model.Exit{}, model.ObjectInstance{}, fmt.Errorf("lock exit %q: key id is required", exitName)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	room, exitIndex, err := w.findExitForUpdateLocked(roomID, exitName, "lock")
	if err != nil {
		return model.Exit{}, model.ObjectInstance{}, err
	}
	exit := room.Exits[exitIndex]
	if exitHasAnyFlag(exit, "locked", "xlockd", "xlocked") {
		return model.Exit{}, model.ObjectInstance{}, fmt.Errorf("lock exit %q: exit is already locked", exitName)
	}
	if !exitHasAnyFlag(exit, "lockable", "xlocks") {
		return model.Exit{}, model.ObjectInstance{}, fmt.Errorf("lock exit %q: exit is not lockable", exitName)
	}
	if !exitHasAnyFlag(exit, "closed", "xclosd", "xclosed") {
		return model.Exit{}, model.ObjectInstance{}, fmt.Errorf("lock exit %q: exit must be closed", exitName)
	}

	key, err := w.validateExitKeyLocked(keyID, exit)
	if err != nil {
		return model.Exit{}, model.ObjectInstance{}, err
	}
	charges, _ := w.objectIntPropertyLocked(key, "shotsCurrent")
	if charges < 1 {
		return model.Exit{}, model.ObjectInstance{}, fmt.Errorf("lock exit %q: key %q is broken", exitName, keyID)
	}

	exit.Flags = setExitFlag(exit.Flags, "locked", true)
	room.Exits[exitIndex] = exit
	w.rooms[roomID] = room
	return cloneExit(exit), cloneObject(key), nil
}

func (w *World) findExitForUpdateLocked(roomID model.RoomID, exitName string, action string) (model.Room, int, error) {
	if roomID.IsZero() {
		return model.Room{}, -1, fmt.Errorf("%s exit %q: room id is required", action, exitName)
	}
	exitName = strings.TrimSpace(exitName)
	if exitName == "" {
		return model.Room{}, -1, fmt.Errorf("%s exit: exit name is required", action)
	}
	room, ok := w.rooms[roomID]
	if !ok {
		return model.Room{}, -1, fmt.Errorf("%s exit %q: room %q not found", action, exitName, roomID)
	}
	for i, exit := range room.Exits {
		if exit.Name == exitName {
			return room, i, nil
		}
	}
	return model.Room{}, -1, fmt.Errorf("%s exit %q: exit not found in room %q", action, exitName, roomID)
}

func (w *World) validateExitKeyLocked(keyID model.ObjectInstanceID, exit model.Exit) (model.ObjectInstance, error) {
	key, ok := w.objects[keyID]
	if !ok {
		return model.ObjectInstance{}, fmt.Errorf("exit key %q: object not found", keyID)
	}
	if !w.objectKindIsLocked(key, model.ObjectKindKey) {
		return model.ObjectInstance{}, fmt.Errorf("exit key %q: object is not a key", keyID)
	}
	keyNumber, ok := exitKeyNumber(exit)
	if !ok {
		return model.ObjectInstance{}, fmt.Errorf("exit %q: key number not found", exit.Name)
	}
	objectKeyNumber, ok := w.objectIntPropertyLocked(key, "nDice")
	if !ok || objectKeyNumber != keyNumber {
		return model.ObjectInstance{}, fmt.Errorf("exit key %q: key does not match", keyID)
	}
	return key, nil
}

func (w *World) objectKindIsLocked(object model.ObjectInstance, kind model.ObjectKind) bool {
	if strings.EqualFold(strings.TrimSpace(object.Properties["kind"]), string(kind)) {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := w.prototypes[object.PrototypeID]
	if !ok {
		return false
	}
	return proto.Kind == kind || strings.EqualFold(strings.TrimSpace(proto.Properties["kind"]), string(kind))
}

func (w *World) objectIntPropertyLocked(object model.ObjectInstance, key string) (int, bool) {
	if value, ok := parseStateInt(object.Properties[key]); ok {
		return value, true
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := w.prototypes[object.PrototypeID]; ok {
			if value, ok := parseStateInt(proto.Properties[key]); ok {
				return value, true
			}
		}
	}
	return 0, false
}

func (w *World) objectIsMoneyLocked(object model.ObjectInstance) bool {
	if strings.EqualFold(strings.TrimSpace(object.Properties["kind"]), string(model.ObjectKindMoney)) {
		return true
	}
	if value, ok := w.objectIntPropertyLocked(object, "type"); ok && value == 10 {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := w.prototypes[object.PrototypeID]
	if !ok {
		return false
	}
	return proto.Kind == model.ObjectKindMoney ||
		strings.EqualFold(strings.TrimSpace(proto.Properties["kind"]), string(model.ObjectKindMoney))
}

func objectLocationEqual(a model.ObjectLocation, b model.ObjectLocation) bool {
	return a.RoomID == b.RoomID &&
		a.CreatureID == b.CreatureID &&
		a.BankID == b.BankID &&
		a.ContainerID == b.ContainerID &&
		a.Slot == b.Slot
}

func objectCountPropertyOrLenLocked(w *World, object model.ObjectInstance, key string) int {
	if w != nil {
		if value, ok := w.objectIntPropertyLocked(object, key); ok && value >= 0 {
			return value
		}
	}
	return len(object.Contents.ObjectIDs)
}

func exitKeyNumber(exit model.Exit) (int, bool) {
	for _, flag := range exit.Flags {
		flag = strings.TrimSpace(flag)
		if flag == "" {
			continue
		}
		key, raw, ok := strings.Cut(flag, ":")
		if !ok || normalizeFlagName(key) != "key" {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err == nil && value > 0 {
			return value, true
		}
	}
	if raw := exit.Metadata.RawFields["key"]; len(raw) > 0 && raw[0] > 0 {
		return int(raw[0]), true
	}
	return 0, false
}

func (w *World) checkRoomExitsLocked(roomID model.RoomID, nowUnix int64) {
	room, ok := w.rooms[roomID]
	if !ok {
		return
	}
	changed := false
	for i := range room.Exits {
		exit := room.Exits[i]
		if !exitTimerExpired(exit, nowUnix) {
			continue
		}
		switch {
		case exitHasAnyFlag(exit, "lockable", "xlocks"):
			exit.Flags = setExitFlag(exit.Flags, "locked", true)
			exit.Flags = setExitFlag(exit.Flags, "closed", true)
		case exitHasAnyFlag(exit, "closable", "xcloss"):
			exit.Flags = setExitFlag(exit.Flags, "closed", true)
		default:
			continue
		}
		room.Exits[i] = exit
		changed = true
	}
	if changed {
		w.rooms[roomID] = room
	}
}

func exitTimerExpired(exit model.Exit, nowUnix int64) bool {
	interval, ltime, ok := exitTimerValues(exit)
	if !ok {
		return false
	}
	return ltime+interval < nowUnix
}

func exitTimerValues(exit model.Exit) (interval int64, ltime int64, ok bool) {
	raw := exit.Metadata.RawFields
	if len(raw) == 0 {
		return 0, 0, false
	}
	interval, hasInterval := rawFieldInt64(raw[exitLTimeIntervalRawField], 4)
	ltime, hasLTime := rawFieldInt64(raw[exitLTimeLTimeRawField], 4)
	if !hasInterval {
		interval, hasInterval = rawFieldInt64(raw["interval"], 4)
	}
	if !hasLTime {
		ltime, hasLTime = rawFieldInt64(raw["lasttime"], 4)
	}
	return interval, ltime, hasInterval || hasLTime
}

func setExitTimerLTime(exit model.Exit, nowUnix int64) model.Exit {
	if exit.Metadata.RawFields == nil {
		exit.Metadata.RawFields = map[string][]byte{}
	} else {
		exit.Metadata.RawFields = cloneRawFields(exit.Metadata.RawFields)
	}
	exit.Metadata.RawFields[exitLTimeLTimeRawField] = int32RawField(nowUnix)
	return exit
}

func rawFieldInt64(raw []byte, width int) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	if text, ok := rawASCIIInteger(raw); ok {
		value, err := strconv.ParseInt(text, 10, 64)
		if err == nil {
			return value, true
		}
	}
	switch width {
	case 2:
		if len(raw) >= 2 {
			return int64(int16(uint16(raw[0]) | uint16(raw[1])<<8)), true
		}
	case 4:
		if len(raw) >= 4 {
			return int64(int32(uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24)), true
		}
	}
	return 0, false
}

func rawASCIIInteger(raw []byte) (string, bool) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", false
	}
	hasDigit := false
	for i, r := range text {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case (r == '-' || r == '+') && i == 0:
		default:
			return "", false
		}
	}
	return text, hasDigit
}

func int32RawField(value int64) []byte {
	v := int32(value)
	return []byte{
		byte(v),
		byte(v >> 8),
		byte(v >> 16),
		byte(v >> 24),
	}
}

func setExitFlag(flags []string, flag string, enabled bool) []string {
	normalized := normalizeFlagName(flag)
	if normalized == "" {
		return slices.Clone(flags)
	}
	out := make([]string, 0, len(flags)+1)
	found := false
	for _, existing := range flags {
		if exitFlagSameKind(existing, normalized) {
			found = true
			continue
		}
		out = append(out, existing)
	}
	if enabled {
		if normalized == "closed" || normalized == "xclosd" || normalized == "xclosed" {
			out = append(out, "closed")
		} else if normalized == "locked" || normalized == "xlockd" || normalized == "xlocked" {
			out = append(out, "locked")
		} else if !found {
			out = append(out, flag)
		} else {
			out = append(out, flag)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func removeMetadataTags(tags []string, remove []string) []string {
	if len(tags) == 0 || len(remove) == 0 {
		return slices.Clone(tags)
	}
	targets := normalizedFlagSet(remove...)
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if _, ok := targets[normalizeFlagName(tag)]; ok {
			continue
		}
		out = append(out, tag)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func addMetadataTags(tags []string, add []string) []string {
	out := slices.Clone(tags)
	seen := make(map[string]struct{}, len(out)+len(add))
	for _, tag := range out {
		if normalized := normalizeFlagName(tag); normalized != "" {
			seen[normalized] = struct{}{}
		}
	}
	for _, tag := range add {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		normalized := normalizeFlagName(tag)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		out = append(out, tag)
		seen[normalized] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func exitFlagSameKind(flag string, normalizedTarget string) bool {
	normalized := normalizeFlagName(flag)
	for _, target := range ExpandFlagNames(normalizedTarget) {
		if normalized == target {
			return true
		}
	}
	return false
}

// ApplyCreatureDamage subtracts damage from hpCurrent. It does not remove the
// creature from room indexes; death handling such as corpses, drops, and respawn
// belongs to the combat/gameplay layer.
func (w *World) ApplyCreatureDamage(creatureID model.CreatureID, damage int) (model.Creature, int, bool, error) {
	if w == nil {
		return model.Creature{}, 0, false, fmt.Errorf("damage creature %q: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return model.Creature{}, 0, false, fmt.Errorf("damage creature: creature id is required")
	}
	if damage < 0 {
		return model.Creature{}, 0, false, fmt.Errorf("damage creature %q: damage cannot be negative", creatureID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, 0, false, fmt.Errorf("damage creature %q: creature not found", creatureID)
	}
	next := cloneCreature(creature)
	current, ok := next.Stats["hpCurrent"]
	if next.Stats == nil || !ok {
		return model.Creature{}, 0, false, fmt.Errorf("damage creature %q: hpCurrent stat not found", creatureID)
	}

	actual := damage
	if current < 0 {
		current = 0
	}
	if actual > current {
		actual = current
	}
	remaining := current - actual
	next.Stats["hpCurrent"] = remaining
	dead := remaining <= 0
	w.creatures[next.ID] = next

	// B (A push): mark player dirty on HP mutation (damage)
	if !next.PlayerID.IsZero() {
		w.MarkPlayerDirty(next.PlayerID)
	}

	return cloneCreature(next), actual, dead, nil
}

// RecordCreatureDamage adds damage credit to the monster death reward ledger.
// Most callers pass the actual damage returned by ApplyCreatureDamage; legacy
// commands with separate reward-credit formulas pass the C ledger amount.
func (w *World) RecordCreatureDamage(victimID model.CreatureID, attackerID model.CreatureID, damage int) error {
	if w == nil {
		return fmt.Errorf("record creature damage %q from %q: world state is nil", victimID, attackerID)
	}
	if victimID.IsZero() {
		return fmt.Errorf("record creature damage: victim id is required")
	}
	if attackerID.IsZero() {
		return fmt.Errorf("record creature damage %q: attacker id is required", victimID)
	}
	if damage < 0 {
		return fmt.Errorf("record creature damage %q from %q: damage cannot be negative", victimID, attackerID)
	}
	if damage == 0 {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if _, ok := w.creatures[victimID]; !ok {
		return fmt.Errorf("record creature damage %q: victim not found", victimID)
	}
	if _, ok := w.creatures[attackerID]; !ok {
		return fmt.Errorf("record creature damage %q from %q: attacker not found", victimID, attackerID)
	}
	if w.monsterDamage == nil {
		w.monsterDamage = map[model.CreatureID]map[model.CreatureID]int{}
	}
	if w.monsterDamage[victimID] == nil {
		w.monsterDamage[victimID] = map[model.CreatureID]int{}
	}
	w.monsterDamage[victimID][attackerID] += damage

	if w.enemies == nil {
		w.enemies = make(map[model.CreatureID][]string)
	}
	var name string
	if c, ok := w.creatures[attackerID]; ok {
		name = c.DisplayName
	} else if p, ok := w.players[model.PlayerID(attackerID)]; ok {
		name = p.DisplayName
	} else {
		name = string(attackerID)
	}
	if name != "" {
		found := false
		for _, existing := range w.enemies[victimID] {
			if existing == name {
				found = true
				break
			}
		}
		if !found {
			w.enemies[victimID] = append(w.enemies[victimID], name)
		}
	}

	if c, ok := w.creatures[victimID]; ok {
		hasTag := false
		for _, tag := range c.Metadata.Tags {
			if strings.EqualFold(tag, "was_attacked") {
				hasTag = true
				break
			}
		}
		if !hasTag {
			c.Metadata.Tags = append(c.Metadata.Tags, "was_attacked")
			w.creatures[victimID] = c
		}
	}

	return nil
}

// FinalizeMonsterDeathOptions carries death-finalization inputs that are not
// owned by world state.
type FinalizeMonsterDeathOptions struct {
	RewardGroup MonsterDeathRewardGroup
}

// MonsterDeathRewardGroup is a snapshot of the final attacker's group at the
// moment a monster dies. FollowerIDs should contain followers only; LeaderID is
// counted separately.
type MonsterDeathRewardGroup struct {
	LeaderID    model.CreatureID
	FollowerIDs []model.CreatureID
}

// FinalizeMonsterDeath removes a dead monster from its room and drops carried
// inventory and gold into that room. It also awards player damage ledger
// experience, alignment shifts, and weapon proficiency when the equipped weapon
// type is available.
func (w *World) FinalizeMonsterDeath(creatureID model.CreatureID) (bool, error) {
	return w.FinalizeMonsterDeathWithOptions(creatureID, FinalizeMonsterDeathOptions{})
}

// FinalizeMonsterDeathWithOptions finalizes a dead monster using external
// reward context such as a group membership snapshot.
func (w *World) FinalizeMonsterDeathWithOptions(creatureID model.CreatureID, options FinalizeMonsterDeathOptions) (bool, error) {
	if w == nil {
		return false, fmt.Errorf("finalize monster death %q: world state is nil", creatureID)
	}
	if creatureID.IsZero() {
		return false, fmt.Errorf("finalize monster death: creature id is required")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	creature, ok := w.creatures[creatureID]
	if !ok {
		return false, fmt.Errorf("finalize monster death %q: creature not found", creatureID)
	}
	if creature.Kind == model.CreatureKindPlayer || !creature.PlayerID.IsZero() {
		return false, fmt.Errorf("finalize monster death %q: creature is a player", creatureID)
	}
	if hp, ok := creature.Stats["hpCurrent"]; ok && hp > 0 {
		return false, nil
	}
	if creature.RoomID.IsZero() {
		return false, fmt.Errorf("finalize monster death %q: creature has no room", creatureID)
	}
	room, ok := w.rooms[creature.RoomID]
	if !ok {
		return false, fmt.Errorf("finalize monster death %q: room %q not found", creatureID, creature.RoomID)
	}

	w.awardMonsterDeathRewardsLocked(creature, options)

	drops := carriedCreatureObjectIDs(creature)
	isTradeItems := creatureHasAnyFlag(creature, "tradeItems", "MTRADE")
	if isTradeItems {
		for _, objectID := range drops {
			w.deleteObjectTreeLocked(objectID, map[model.ObjectInstanceID]struct{}{})
		}
	} else {
		seenDrops := make(map[model.ObjectInstanceID]struct{}, len(drops))
		for _, objectID := range drops {
			if objectID.IsZero() {
				continue
			}
			if _, seen := seenDrops[objectID]; seen {
				continue
			}
			seenDrops[objectID] = struct{}{}
			object, ok := w.objects[objectID]
			if !ok || object.Location.CreatureID != creature.ID {
				continue
			}
			w.removeObjectFromHolderLocked(objectID, object.Location)
			object.Location = model.ObjectLocation{RoomID: room.ID}
			w.objects[objectID] = object
			w.addObjectToHolderLocked(objectID, object.Location)
		}
	}

	if gold := creature.Stats["gold"]; gold > 0 {
		moneyID := w.nextObjectCloneIDLocked("object:money")
		moneyPrototypeID := model.PrototypeID("prototype:money")
		if _, ok := w.prototypes[moneyPrototypeID]; !ok {
			w.prototypes[moneyPrototypeID] = model.ObjectPrototype{
				ID:          moneyPrototypeID,
				Kind:        model.ObjectKindMoney,
				DisplayName: "돈",
			}
		}
		money := model.ObjectInstance{
			ID:                  moneyID,
			PrototypeID:         moneyPrototypeID,
			DisplayNameOverride: fmt.Sprintf("%d냥", gold),
			Location:            model.ObjectLocation{RoomID: room.ID},
			Properties: map[string]string{
				"kind":  string(model.ObjectKindMoney),
				"type":  "10",
				"value": strconv.Itoa(gold),
			},
		}
		if err := money.Validate(); err != nil {
			return false, fmt.Errorf("finalize monster death %q: money object: %w", creatureID, err)
		}
		w.objects[money.ID] = money
		w.addObjectToHolderLocked(money.ID, money.Location)
	}

	// D (Package A): Monster death drops items + gold directly to room floor (bypassing MoveObject).
	// Must mark dirty here so corpse-less loot (items, money) + any nested survive restart.
	// (Player death uses corpse+mark; this is monster equiv in update.c style death handling.)
	// Only mark if we actually placed something (tradeItems monsters delete carried items, only drop gold).
	goldHad := creature.Stats != nil && creature.Stats["gold"] > 0
	itemsDroppedToFloor := !isTradeItems && len(drops) > 0
	if itemsDroppedToFloor || goldHad {
		w.MarkRoomObjectsDirty(room.ID)
	}

	room = w.rooms[room.ID]
	room.CreatureIDs = removeID(room.CreatureIDs, creature.ID)
	w.rooms[room.ID] = room
	delete(w.creatures, creature.ID)
	delete(w.monsterDamage, creature.ID)
	w.pruneCharmReferencesLocked(creature)
	if w.enemies != nil {
		delete(w.enemies, creature.ID)
		// Full aggro management: clear on death - prune dead monster's name from all hatred lists
		deadName := creature.DisplayName
		for id, lst := range w.enemies {
			newLst := make([]string, 0, len(lst))
			for _, n := range lst {
				if n != deadName {
					newLst = append(newLst, n)
				}
			}
			w.enemies[id] = newLst
		}
	}
	return true, nil
}

func (w *World) pruneCharmReferencesLocked(target model.Creature) {
	remove := charmReferenceTags(target)
	if len(remove) == 0 {
		return
	}
	for id, creature := range w.creatures {
		nextTags := removeMetadataTags(creature.Metadata.Tags, remove)
		if slices.Equal(nextTags, creature.Metadata.Tags) {
			continue
		}
		creature.Metadata.Tags = nextTags
		w.creatures[id] = creature
		if !creature.PlayerID.IsZero() {
			w.MarkPlayerDirty(creature.PlayerID)
		}
	}
	for id, player := range w.players {
		nextTags := removeMetadataTags(player.Metadata.Tags, remove)
		if slices.Equal(nextTags, player.Metadata.Tags) {
			continue
		}
		player.Metadata.Tags = nextTags
		w.players[id] = player
		w.MarkPlayerDirty(id)
	}
}

func charmReferenceTags(target model.Creature) []string {
	var tags []string
	if name := strings.TrimSpace(target.DisplayName); name != "" {
		tags = append(tags, "charm:"+name)
	}
	if !target.ID.IsZero() {
		tags = append(tags, "charmID:"+string(target.ID))
	}
	return tags
}

func (w *World) awardMonsterDeathRewardsLocked(monster model.Creature, options FinalizeMonsterDeathOptions) {
	ledger := w.monsterDamage[monster.ID]
	if len(ledger) == 0 {
		return
	}

	monsterExp := monster.Stats["experience"]
	monsterAlignment := monster.Stats["alignment"]
	hpMax := monster.Stats["hpMax"]
	if hpMax < 1 {
		hpMax = 1
	}

	for attackerID, damage := range ledger {
		if damage <= 0 {
			continue
		}
		attacker, ok := w.creatures[attackerID]
		if !ok {
			continue
		}
		if !w.creatureIsPlayerRewardRecipientLocked(attacker) {
			continue
		}
		if attacker.Stats == nil {
			attacker.Stats = map[string]int{}
		}
		expGain := (monsterExp * damage) / hpMax
		if expGain > monsterExp {
			expGain = monsterExp
		}
		if expGain > 0 {
			attacker.Stats["experience"] += expGain + w.monsterDeathGroupBonusLocked(options.RewardGroup, attacker.ID, monsterExp, expGain)
			if proficiencyKey, ok := w.creatureWeaponProficiencyStatKeyLocked(attacker); ok {
				attacker.Stats[proficiencyKey] += expGain
			}
		}
		attacker.Stats["alignment"] = clampInt(attacker.Stats["alignment"]-monsterAlignment/5, -1000, 1000)
		w.creatures[attackerID] = attacker

		// B: Collect players who received rewards for post-death auto-save
		if w.creatureIsPlayerRewardRecipientLocked(attacker) {
			if player, ok := w.players[attacker.PlayerID]; ok {
				// will save after unlock
				_ = player // placeholder; actual save after this function
			}
		}
	}
}

func (w *World) monsterDeathGroupBonusLocked(group MonsterDeathRewardGroup, recipientID model.CreatureID, monsterExp, expGain int) int {
	if group.LeaderID.IsZero() || recipientID.IsZero() || expGain <= 0 || expGain == monsterExp {
		return 0
	}

	followerCount, recipientIsFollower := w.monsterDeathGroupFollowerCountLocked(group, recipientID)
	if followerCount <= 0 {
		return 0
	}

	switch {
	case recipientID == group.LeaderID:
		if expGain+(expGain/2)*followerCount > monsterExp*2 {
			return expGain
		}
		return (expGain / 5) * (followerCount + 1)
	case recipientIsFollower:
		if expGain+(expGain/3)*followerCount > monsterExp*2 {
			return expGain
		}
		return (expGain / 4) * followerCount
	default:
		return 0
	}
}

func (w *World) monsterDeathGroupFollowerCountLocked(group MonsterDeathRewardGroup, recipientID model.CreatureID) (int, bool) {
	seen := map[model.CreatureID]struct{}{}
	count := 0
	recipientIsFollower := false
	for _, followerID := range group.FollowerIDs {
		if followerID.IsZero() || followerID == group.LeaderID {
			continue
		}
		if _, ok := seen[followerID]; ok {
			continue
		}
		seen[followerID] = struct{}{}

		follower, ok := w.creatures[followerID]
		if !ok || w.creatureHasDMInvisibleFlagLocked(follower) {
			continue
		}
		count++
		if followerID == recipientID {
			recipientIsFollower = true
		}
	}
	return count, recipientIsFollower
}

func (w *World) creatureIsPlayerRewardRecipientLocked(creature model.Creature) bool {
	if creature.PlayerID.IsZero() {
		return false
	}
	player, ok := w.players[creature.PlayerID]
	if !ok || player.CreatureID != creature.ID {
		return false
	}
	return creature.Kind == model.CreatureKindPlayer || !creature.PlayerID.IsZero()
}

func (w *World) creatureWeaponProficiencyStatKeyLocked(creature model.Creature) (string, bool) {
	if len(creature.Equipment) == 0 {
		return "", false
	}

	for _, slot := range []string{"wield", "weapon", "mainHand", "right"} {
		objectID := creature.Equipment[slot]
		if key, ok := w.objectWeaponProficiencyStatKeyLocked(objectID); ok {
			return key, true
		}
	}

	var foundKey string
	for _, objectID := range creature.Equipment {
		key, ok := w.objectWeaponProficiencyStatKeyLocked(objectID)
		if !ok {
			continue
		}
		if foundKey != "" {
			return "", false
		}
		foundKey = key
	}
	return foundKey, foundKey != ""
}

func (w *World) objectWeaponProficiencyStatKeyLocked(objectID model.ObjectInstanceID) (string, bool) {
	if objectID.IsZero() {
		return "", false
	}
	object, ok := w.objects[objectID]
	if !ok {
		return "", false
	}
	weaponType, ok := w.objectIntPropertyLocked(object, "type")
	if !ok || weaponType < 0 || weaponType >= len(weaponProficiencyStatKeys) {
		return "", false
	}
	return weaponProficiencyStatKeys[weaponType], true
}

var weaponProficiencyStatKeys = [...]string{
	"proficiencySharp",
	"proficiencyThrust",
	"proficiencyBlunt",
	"proficiencyPole",
	"proficiencyMissile",
}

var weaponProficiencyPropertyKeys = [...]string{
	"sharp",
	"thrust",
	"blunt",
	"pole",
	"missile",
}

func carriedCreatureObjectIDs(creature model.Creature) []model.ObjectInstanceID {
	ids := append([]model.ObjectInstanceID(nil), creature.Inventory.ObjectIDs...)
	for _, objectID := range creature.Equipment {
		ids = append(ids, objectID)
	}
	return ids
}

func (w *World) deleteObjectTreeLocked(objectID model.ObjectInstanceID, seen map[model.ObjectInstanceID]struct{}) {
	if objectID.IsZero() {
		return
	}
	if _, ok := seen[objectID]; ok {
		return
	}
	seen[objectID] = struct{}{}

	object, ok := w.objects[objectID]
	if !ok {
		return
	}
	w.markObjectLocationDirtyLocked(object.Location, map[model.ObjectInstanceID]struct{}{})
	for _, childID := range append([]model.ObjectInstanceID(nil), object.Contents.ObjectIDs...) {
		w.deleteObjectTreeLocked(childID, seen)
	}
	w.removeObjectFromHolderLocked(objectID, object.Location)
	delete(w.objects, objectID)
}

func (w *World) markObjectLocationDirtyLocked(location model.ObjectLocation, seen map[model.ObjectInstanceID]struct{}) {
	switch {
	case !location.RoomID.IsZero():
		w.MarkRoomObjectsDirty(location.RoomID)
	case !location.CreatureID.IsZero():
		if creature, ok := w.creatures[location.CreatureID]; ok && !creature.PlayerID.IsZero() {
			w.MarkPlayerDirty(creature.PlayerID)
		}
	case !location.BankID.IsZero():
		w.MarkBankDirty(location.BankID)
	case !location.ContainerID.IsZero():
		if _, ok := seen[location.ContainerID]; ok {
			return
		}
		seen[location.ContainerID] = struct{}{}
		container, ok := w.objects[location.ContainerID]
		if !ok {
			return
		}
		w.markObjectLocationDirtyLocked(container.Location, seen)
	}
}

func creatureHasAnyFlag(creature model.Creature, names ...string) bool {
	if hasAnyNormalizedFlag(creature.Metadata.Tags, names...) {
		return true
	}
	targets := normalizedFlagSet(names...)
	for key, value := range creature.Stats {
		if value == 0 {
			continue
		}
		if _, ok := targets[normalizeFlagName(key)]; ok {
			return true
		}
	}
	if len(creature.Properties) == 0 {
		return false
	}
	for key, value := range creature.Properties {
		normalizedKey := normalizeFlagName(key)
		if _, ok := targets[normalizedKey]; ok && propertyFlagEnabled(value) {
			return true
		}
		for _, token := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ';' || r == '|' || r == ' '
		}) {
			if _, ok := targets[normalizeFlagName(token)]; ok {
				return true
			}
		}
	}
	return false
}

func (w *World) creatureHasDMInvisibleFlagLocked(creature model.Creature) bool {
	targets := normalizedFlagSet("PDMINV", "dmInvisible")
	for key, value := range creature.Stats {
		if _, ok := targets[normalizeFlagName(key)]; ok && value != 0 {
			return true
		}
	}
	return creatureHasAnyFlag(creature, "PDMINV", "dmInvisible")
}

func (w *World) roomVisiblePlayerCountLocked(room model.Room) int {
	count := 0
	seen := map[model.PlayerID]struct{}{}
	for _, playerID := range room.PlayerIDs {
		if playerID.IsZero() {
			continue
		}
		seen[playerID] = struct{}{}
		player, ok := w.players[playerID]
		if !ok {
			count++
			continue
		}
		if w.playerHasDMInvisibleFlagLocked(player) {
			continue
		}
		count++
	}
	for _, creatureID := range room.CreatureIDs {
		creature, ok := w.creatures[creatureID]
		if !ok || creature.PlayerID.IsZero() {
			continue
		}
		if _, ok := seen[creature.PlayerID]; ok {
			continue
		}
		if w.creatureHasDMInvisibleFlagLocked(creature) {
			continue
		}
		count++
	}
	return count
}

func (w *World) playerHasDMInvisibleFlagLocked(player model.Player) bool {
	if hasAnyNormalizedFlag(player.Metadata.Tags, "PDMINV", "dmInvisible") {
		return true
	}
	if player.CreatureID.IsZero() {
		return false
	}
	creature, ok := w.creatures[player.CreatureID]
	return ok && w.creatureHasDMInvisibleFlagLocked(creature)
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func (w *World) reconcileRoomOccupants() {
	for id, room := range w.rooms {
		room.PlayerIDs = w.sortPlayerIDsLegacyLocked(filterRoomPlayerIDs(id, room.PlayerIDs, w.players))
		room.CreatureIDs = w.sortCreatureIDsLegacyLocked(filterRoomCreatureIDs(id, room.CreatureIDs, w.creatures))
		w.rooms[id] = room
	}

	playerIDs := make([]model.PlayerID, 0, len(w.players))
	for id := range w.players {
		playerIDs = append(playerIDs, id)
	}
	slices.Sort(playerIDs)
	for _, id := range playerIDs {
		player := w.players[id]
		if player.RoomID.IsZero() {
			continue
		}
		room, ok := w.rooms[player.RoomID]
		if !ok {
			continue
		}
		room.PlayerIDs = w.insertPlayerIDLegacySortedLocked(room.PlayerIDs, player.ID)
		w.rooms[player.RoomID] = room
	}

	creatureIDs := make([]model.CreatureID, 0, len(w.creatures))
	for id := range w.creatures {
		creatureIDs = append(creatureIDs, id)
	}
	slices.Sort(creatureIDs)
	for _, id := range creatureIDs {
		creature := w.creatures[id]
		if creature.RoomID.IsZero() {
			continue
		}
		room, ok := w.rooms[creature.RoomID]
		if !ok {
			continue
		}
		room.CreatureIDs = w.insertCreatureIDLegacySortedLocked(room.CreatureIDs, creature.ID)
		w.rooms[creature.RoomID] = room
	}
}

func (w *World) validateObjectDestinationLocked(objectID model.ObjectInstanceID, location model.ObjectLocation) error {
	switch {
	case !location.RoomID.IsZero():
		if _, ok := w.rooms[location.RoomID]; !ok {
			return fmt.Errorf("move object %q: target room %q not found", objectID, location.RoomID)
		}
	case !location.CreatureID.IsZero():
		if _, ok := w.creatures[location.CreatureID]; !ok {
			return fmt.Errorf("move object %q: target creature %q not found", objectID, location.CreatureID)
		}
	case !location.BankID.IsZero():
		if _, ok := w.banks[location.BankID]; !ok {
			return fmt.Errorf("move object %q: target bank %q not found", objectID, location.BankID)
		}
	case !location.ContainerID.IsZero():
		if _, ok := w.objects[location.ContainerID]; !ok {
			return fmt.Errorf("move object %q: target container %q not found", objectID, location.ContainerID)
		}
		if location.ContainerID == objectID {
			return fmt.Errorf("move object %q: object cannot contain itself", objectID)
		}
		if w.containsObjectAncestorLocked(location.ContainerID, objectID) {
			return fmt.Errorf("move object %q: object cannot move into descendant %q", objectID, location.ContainerID)
		}
	}
	return nil
}

func (w *World) containsObjectAncestorLocked(start, want model.ObjectInstanceID) bool {
	seen := map[model.ObjectInstanceID]struct{}{}
	for id := start; !id.IsZero(); {
		if id == want {
			return true
		}
		if _, ok := seen[id]; ok {
			return false
		}
		seen[id] = struct{}{}
		object, ok := w.objects[id]
		if !ok {
			return false
		}
		id = object.Location.ContainerID
	}
	return false
}

func (w *World) sortPlayerIDsLegacyLocked(ids []model.PlayerID) []model.PlayerID {
	var out []model.PlayerID
	for _, id := range ids {
		out = w.insertPlayerIDLegacySortedLocked(out, id)
	}
	return out
}

func (w *World) insertPlayerIDLegacySortedLocked(ids []model.PlayerID, playerID model.PlayerID) []model.PlayerID {
	for _, existing := range ids {
		if existing == playerID {
			return ids
		}
	}
	out := make([]model.PlayerID, 0, len(ids)+1)
	inserted := false
	for _, existing := range ids {
		if !inserted && w.playerIDLegacyLessLocked(playerID, existing) {
			out = append(out, playerID)
			inserted = true
		}
		out = append(out, existing)
	}
	if !inserted {
		out = append(out, playerID)
	}
	return out
}

func (w *World) playerIDLegacyLessLocked(leftID, rightID model.PlayerID) bool {
	return strings.Compare(w.playerLegacySortNameLocked(leftID), w.playerLegacySortNameLocked(rightID)) < 0
}

func (w *World) playerLegacySortNameLocked(playerID model.PlayerID) string {
	player, ok := w.players[playerID]
	if !ok {
		return string(playerID)
	}
	if !player.CreatureID.IsZero() {
		if creature, ok := w.creatures[player.CreatureID]; ok {
			if name := creatureLegacySortName(creature); name != "" {
				return name
			}
		}
	}
	if name := strings.TrimSpace(player.DisplayName); name != "" {
		return name
	}
	return string(player.ID)
}

func (w *World) sortCreatureIDsLegacyLocked(ids []model.CreatureID) []model.CreatureID {
	var out []model.CreatureID
	for _, id := range ids {
		out = w.insertCreatureIDLegacySortedLocked(out, id)
	}
	return out
}

func (w *World) insertCreatureIDLegacySortedLocked(ids []model.CreatureID, creatureID model.CreatureID) []model.CreatureID {
	for _, existing := range ids {
		if existing == creatureID {
			return ids
		}
	}
	out := make([]model.CreatureID, 0, len(ids)+1)
	inserted := false
	for _, existing := range ids {
		if !inserted && w.creatureIDLegacyLessLocked(creatureID, existing) {
			out = append(out, creatureID)
			inserted = true
		}
		out = append(out, existing)
	}
	if !inserted {
		out = append(out, creatureID)
	}
	return out
}

func (w *World) creatureIDLegacyLessLocked(leftID, rightID model.CreatureID) bool {
	return strings.Compare(w.creatureLegacySortNameLocked(leftID), w.creatureLegacySortNameLocked(rightID)) < 0
}

func (w *World) creatureLegacySortNameLocked(creatureID model.CreatureID) string {
	creature, ok := w.creatures[creatureID]
	if !ok {
		return string(creatureID)
	}
	if name := creatureLegacySortName(creature); name != "" {
		return name
	}
	return string(creature.ID)
}

func creatureLegacySortName(creature model.Creature) string {
	if name := strings.TrimSpace(creature.DisplayName); name != "" {
		return name
	}
	for _, key := range []string{"name", "key[0]", "key[1]", "key[2]", "key/1", "key/2", "key/3"} {
		if name := strings.TrimSpace(creature.Properties[key]); name != "" {
			return name
		}
	}
	return ""
}

func (w *World) addObjectToHolderLocked(objectID model.ObjectInstanceID, location model.ObjectLocation) {
	switch {
	case !location.RoomID.IsZero():
		room := w.rooms[location.RoomID]
		room.Objects.ObjectIDs = w.insertObjectIDLegacySortedLocked(room.Objects.ObjectIDs, objectID)
		w.rooms[location.RoomID] = room
	case !location.CreatureID.IsZero():
		creature := w.creatures[location.CreatureID]
		creature.Inventory.ObjectIDs = w.insertObjectIDLegacySortedLocked(creature.Inventory.ObjectIDs, objectID)
		if location.Slot != "" && location.Slot != "inventory" {
			if creature.Equipment == nil {
				creature.Equipment = map[string]model.ObjectInstanceID{}
			}
			creature.Equipment[location.Slot] = objectID
		}
		w.creatures[location.CreatureID] = creature
	case !location.BankID.IsZero():
		account := w.banks[location.BankID]
		account.Objects.ObjectIDs = w.insertObjectIDLegacySortedLocked(account.Objects.ObjectIDs, objectID)
		w.banks[location.BankID] = account
	case !location.ContainerID.IsZero():
		container := w.objects[location.ContainerID]
		container.Contents.ObjectIDs = w.insertObjectIDLegacySortedLocked(container.Contents.ObjectIDs, objectID)
		w.objects[location.ContainerID] = container
	}
}

func (w *World) insertObjectIDLegacySortedLocked(ids []model.ObjectInstanceID, objectID model.ObjectInstanceID) []model.ObjectInstanceID {
	for _, existing := range ids {
		if existing == objectID {
			return ids
		}
	}
	out := make([]model.ObjectInstanceID, 0, len(ids)+1)
	inserted := false
	for _, existing := range ids {
		if !inserted && w.objectIDLegacyLessLocked(objectID, existing) {
			out = append(out, objectID)
			inserted = true
		}
		out = append(out, existing)
	}
	if !inserted {
		out = append(out, objectID)
	}
	return out
}

func (w *World) objectIDLegacyLessLocked(leftID, rightID model.ObjectInstanceID) bool {
	leftName := w.objectLegacySortNameLocked(leftID)
	rightName := w.objectLegacySortNameLocked(rightID)
	if cmp := strings.Compare(leftName, rightName); cmp != 0 {
		return cmp < 0
	}
	leftAdjustment := w.objectLegacyAdjustmentLocked(leftID)
	rightAdjustment := w.objectLegacyAdjustmentLocked(rightID)
	return leftAdjustment < rightAdjustment
}

func (w *World) objectLegacySortNameLocked(objectID model.ObjectInstanceID) string {
	object, ok := w.objects[objectID]
	if !ok {
		return string(objectID)
	}
	return w.objectLegacySortNameFromObjectLocked(object)
}

func (w *World) objectLegacySortNameFromObjectLocked(object model.ObjectInstance) string {
	if name := strings.TrimSpace(object.DisplayNameOverride); name != "" {
		return name
	}
	if name := strings.TrimSpace(object.Properties["name"]); name != "" {
		return name
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := w.prototypes[object.PrototypeID]; ok {
			if name := strings.TrimSpace(proto.Properties["name"]); name != "" {
				return name
			}
			if name := strings.TrimSpace(proto.DisplayName); name != "" {
				return name
			}
			if name := firstStateObjectKeyName(proto.Properties); name != "" {
				return name
			}
		}
	}
	if name := firstStateObjectKeyName(object.Properties); name != "" {
		return name
	}
	return string(object.ID)
}

func firstStateObjectKeyName(properties map[string]string) string {
	for _, key := range []string{"key[0]", "key[1]", "key[2]", "key/1", "key/2", "key/3"} {
		if name := strings.TrimSpace(properties[key]); name != "" {
			return name
		}
	}
	return ""
}

func (w *World) objectLegacyAdjustmentLocked(objectID model.ObjectInstanceID) int {
	object, ok := w.objects[objectID]
	if !ok {
		return 0
	}
	adjustment, _ := w.objectIntPropertyAnyLocked(object, "adjustment", "adjust")
	return adjustment
}

func (w *World) removeObjectFromHolderLocked(objectID model.ObjectInstanceID, location model.ObjectLocation) {
	switch {
	case !location.RoomID.IsZero():
		room, ok := w.rooms[location.RoomID]
		if !ok {
			return
		}
		room.Objects.ObjectIDs = removeID(room.Objects.ObjectIDs, objectID)
		w.rooms[location.RoomID] = room
	case !location.CreatureID.IsZero():
		creature, ok := w.creatures[location.CreatureID]
		if !ok {
			return
		}
		creature.Inventory.ObjectIDs = removeID(creature.Inventory.ObjectIDs, objectID)
		for slot, equippedObjectID := range creature.Equipment {
			if equippedObjectID == objectID {
				delete(creature.Equipment, slot)
			}
		}
		w.creatures[location.CreatureID] = creature
	case !location.BankID.IsZero():
		account, ok := w.banks[location.BankID]
		if !ok {
			return
		}
		account.Objects.ObjectIDs = removeID(account.Objects.ObjectIDs, objectID)
		w.banks[location.BankID] = account
	case !location.ContainerID.IsZero():
		container, ok := w.objects[location.ContainerID]
		if !ok {
			return
		}
		container.Contents.ObjectIDs = removeID(container.Contents.ObjectIDs, objectID)
		w.objects[location.ContainerID] = container
	}
}

func filterRoomPlayerIDs(roomID model.RoomID, ids []model.PlayerID, players map[model.PlayerID]model.Player) []model.PlayerID {
	if len(ids) == 0 {
		return nil
	}
	out := make([]model.PlayerID, 0, len(ids))
	seen := make(map[model.PlayerID]struct{}, len(ids))
	for _, id := range ids {
		if id.IsZero() {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		player, ok := players[id]
		if ok && player.RoomID != roomID {
			continue
		}
		out = append(out, id)
		seen[id] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func filterRoomCreatureIDs(roomID model.RoomID, ids []model.CreatureID, creatures map[model.CreatureID]model.Creature) []model.CreatureID {
	if len(ids) == 0 {
		return nil
	}
	out := make([]model.CreatureID, 0, len(ids))
	seen := make(map[model.CreatureID]struct{}, len(ids))
	for _, id := range ids {
		if id.IsZero() {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		creature, ok := creatures[id]
		if ok && creature.RoomID != roomID {
			continue
		}
		out = append(out, id)
		seen[id] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func findExit(room model.Room, name string) (model.Exit, bool) {
	for _, exit := range room.Exits {
		if exit.Name == name {
			return exit, true
		}
	}
	return model.Exit{}, false
}

func blockedMoveExitFlag(exit model.Exit) (string, bool) {
	blocked := map[string]struct{}{
		"closed":  {},
		"xclosd":  {},
		"xclosed": {},
		"locked":  {},
		"xlockd":  {},
		"xlocked": {},
		"nosee":   {},
		"xnosee":  {},
	}
	for _, flag := range exit.Flags {
		normalized := normalizeFlagName(flag)
		if _, ok := blocked[normalized]; ok {
			return flag, true
		}
	}
	return "", false
}

func (w *World) validateMovePlayerRestrictions(
	player model.Player,
	exitName string,
	exit model.Exit,
	toRoom model.Room,
	creature model.Creature,
	hasCreature bool,
) error {
	level := movePlayerLevel(creature, hasCreature)
	if minLevel, ok := roomMinLevel(toRoom); ok && level < minLevel {
		return fmt.Errorf(
			"move player %q: target room %q minLevel restriction: player level %d below %d",
			player.ID,
			toRoom.ID,
			level,
			minLevel,
		)
	}
	if maxLevel, ok := roomMaxLevel(toRoom); ok && level > maxLevel {
		return fmt.Errorf(
			"move player %q: target room %q maxLevel restriction: player level %d above %d",
			player.ID,
			toRoom.ID,
			level,
			maxLevel,
		)
	}
	playerCount := w.roomVisiblePlayerCountLocked(toRoom)
	if limit, name := roomPlayerLimit(toRoom); limit > 0 && playerCount >= limit {
		return fmt.Errorf(
			"move player %q: target room %q %s restriction: player count %d at limit %d",
			player.ID,
			toRoom.ID,
			name,
			playerCount,
			limit,
		)
	}
	if err := w.validateMovePlayerFamilyRestrictions(player, toRoom, creature, hasCreature); err != nil {
		return err
	}
	if exitHasAnyFlag(exit, "naked", "xnaked") && hasCreature {
		if weight := w.creatureCarriedWeightLocked(creature); weight != 0 {
			return fmt.Errorf(
				"move player %q: exit %q naked restriction: linked creature %q carried weight %d",
				player.ID,
				exitName,
				creature.ID,
				weight,
			)
		}
	}
	return nil
}

func (w *World) validateMovePlayerFamilyRestrictions(
	player model.Player,
	room model.Room,
	creature model.Creature,
	hasCreature bool,
) error {
	if roomHasAnyFlag(room, "family") && !movePlayerCreatureHasFamilyFlag(creature, hasCreature) {
		return fmt.Errorf(
			"move player %q: target room %q family restriction: linked creature missing family flag",
			player.ID,
			room.ID,
		)
	}

	playerClass := movePlayerClass(creature, hasCreature)
	roomSpecial, hasRoomSpecial := roomSpecialValue(room)
	if roomHasAnyFlag(room, "onlyFamily", "familyOnly", "ronfml") && playerClass < movePlayerDMClass {
		if !hasRoomSpecial || !movePlayerCreatureHasIntValue(creature, hasCreature, roomSpecial,
			"familyID",
			"dailyExpndMax",
			"legacyDailyExpndMax",
		) {
			return fmt.Errorf(
				"move player %q: target room %q onlyFamily restriction: linked creature family does not match room special",
				player.ID,
				room.ID,
			)
		}
	}
	if roomHasAnyFlag(room, "onlyMarried", "marriedOnly", "ronmar") && playerClass < movePlayerDMClass {
		matchesMarriage := hasRoomSpecial && movePlayerCreatureHasIntValue(creature, hasCreature, roomSpecial,
			"marriageID",
			"dailyMarriageMax",
			"legacyDailyMarriageMax",
		)
		if !matchesMarriage && (!hasRoomSpecial || !w.hasMarriageInviteLocked(player, model.SpecialID(roomSpecial))) {
			return fmt.Errorf(
				"move player %q: target room %q onlyMarried restriction: linked creature marriage does not match room special",
				player.ID,
				room.ID,
			)
		}
	}
	return nil
}

func (w *World) hasMarriageInviteLocked(player model.Player, specialID model.SpecialID) bool {
	if w == nil || specialID.IsZero() {
		return false
	}
	for _, name := range w.marriageInvites[specialID] {
		if movePlayerInviteNameMatches(player, name) {
			return true
		}
	}
	return false
}

func movePlayerInviteNameMatches(player model.Player, invitedName string) bool {
	invitedName = strings.TrimSpace(invitedName)
	if invitedName == "" {
		return false
	}
	return invitedName == strings.TrimSpace(player.DisplayName) ||
		invitedName == strings.TrimSpace(string(player.ID))
}

func movePlayerLevel(creature model.Creature, hasCreature bool) int {
	if !hasCreature {
		return 0
	}
	if creature.Stats != nil {
		if level, ok := creature.Stats["level"]; ok {
			return level
		}
	}
	if creature.Level != 0 {
		return creature.Level
	}
	if level, ok := stateCreatureIntValue(creature, "level"); ok {
		return level
	}
	return 0
}

func movePlayerClass(creature model.Creature, hasCreature bool) int {
	if !hasCreature {
		return 0
	}
	return creatureStateClass(creature)
}

func roomSpecialValue(room model.Room) (int, bool) {
	if len(room.Properties) == 0 {
		return 0, false
	}
	if raw, ok := room.Properties["special"]; ok {
		return parseMoveIntValue(raw)
	}
	for key, raw := range room.Properties {
		if normalizeFlagName(key) == "special" {
			return parseMoveIntValue(raw)
		}
	}
	return 0, false
}

func movePlayerCreatureHasFamilyFlag(creature model.Creature, hasCreature bool) bool {
	if !hasCreature {
		return false
	}
	targets := normalizedFlagSet("familyFlag", "PFAMIL")
	for key, value := range creature.Stats {
		if _, ok := targets[normalizeFlagName(key)]; ok && value != 0 {
			return true
		}
	}
	for key, value := range creature.Properties {
		if _, ok := targets[normalizeFlagName(key)]; !ok {
			continue
		}
		if propertyFlagEnabled(value) {
			return true
		}
		if parsed, parsedOK := parseMoveIntValue(value); parsedOK && parsed != 0 {
			return true
		}
	}
	return false
}

func movePlayerCreatureHasIntValue(creature model.Creature, hasCreature bool, want int, names ...string) bool {
	if !hasCreature {
		return false
	}
	targets := normalizedFlagSet(names...)
	for key, value := range creature.Stats {
		if _, ok := targets[normalizeFlagName(key)]; ok && value == want {
			return true
		}
	}
	for key, value := range creature.Properties {
		if _, ok := targets[normalizeFlagName(key)]; !ok {
			continue
		}
		parsed, ok := parseMoveIntValue(value)
		if ok && parsed == want {
			return true
		}
	}
	return false
}

func parseMoveIntValue(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func roomMinLevel(room model.Room) (int, bool) {
	levels := roomLevelLimits(room, "minLevel")
	if len(levels) == 0 {
		return 0, false
	}
	minLevel := levels[0]
	for _, level := range levels[1:] {
		if level > minLevel {
			minLevel = level
		}
	}
	return minLevel, true
}

func roomMaxLevel(room model.Room) (int, bool) {
	levels := roomLevelLimits(room, "maxLevel")
	if len(levels) == 0 {
		return 0, false
	}
	maxLevel := levels[0]
	for _, level := range levels[1:] {
		if level < maxLevel {
			maxLevel = level
		}
	}
	return maxLevel, true
}

func roomLevelLimits(room model.Room, name string) []int {
	var levels []int
	target := normalizeFlagName(name)

	for key, value := range room.Properties {
		if normalizeFlagName(key) == target {
			if level, ok := parseMoveLevelValue(value); ok {
				levels = append(levels, level)
			}
		}
		levels = appendNamedMoveLevelLimits(levels, value, target)
	}

	for _, tag := range room.Metadata.Tags {
		levels = appendNamedMoveLevelLimits(levels, tag, target)
	}

	return levels
}

func appendNamedMoveLevelLimits(levels []int, value string, target string) []int {
	if level, ok := parseNamedMoveLevelLimit(value, target); ok {
		levels = append(levels, level)
	}
	for _, token := range moveRestrictionTokens(value) {
		if level, ok := parseNamedMoveLevelLimit(token, target); ok {
			levels = append(levels, level)
		}
	}
	return levels
}

func parseNamedMoveLevelLimit(value string, target string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	for _, sep := range []string{":", "="} {
		key, rawLevel, ok := strings.Cut(value, sep)
		if ok && normalizeFlagName(key) == target {
			return parseMoveLevelValue(rawLevel)
		}
	}

	normalized := normalizeFlagName(value)
	if !strings.HasPrefix(normalized, target) || len(normalized) == len(target) {
		return 0, false
	}
	return parseMoveLevelValue(normalized[len(target):])
}

func parseMoveLevelValue(value string) (int, bool) {
	level, ok := parseStateInt(value)
	if !ok || level < 0 {
		return 0, false
	}
	return level, true
}

func moveRestrictionTokens(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == ' '
	})
}

func roomPlayerLimit(room model.Room) (int, string) {
	limit := 0
	name := ""
	for _, option := range []struct {
		name  string
		limit int
	}{
		{name: "onePlayer", limit: 1},
		{name: "twoPlayers", limit: 2},
		{name: "threePlayers", limit: 3},
	} {
		if roomHasAnyFlag(room, option.name) && (limit == 0 || option.limit < limit) {
			limit = option.limit
			name = option.name
		}
	}
	return limit, name
}

func roomHasAnyFlag(room model.Room, names ...string) bool {
	if hasAnyNormalizedFlag(room.Metadata.Tags, names...) {
		return true
	}
	if len(room.Properties) == 0 {
		return false
	}

	targets := normalizedFlagSet(names...)
	for key, value := range room.Properties {
		normalizedKey := normalizeFlagName(key)
		if _, ok := targets[normalizedKey]; ok && propertyFlagEnabled(value) {
			return true
		}
		for _, token := range moveRestrictionTokens(value) {
			if _, ok := targets[normalizeFlagName(token)]; ok {
				return true
			}
		}
	}
	return false
}

func exitHasAnyFlag(exit model.Exit, names ...string) bool {
	return hasAnyNormalizedFlag(exit.Flags, names...)
}

func hasAnyNormalizedFlag(flags []string, names ...string) bool {
	targets := normalizedFlagSet(names...)
	for _, flag := range flags {
		if _, ok := targets[normalizeFlagName(flag)]; ok {
			return true
		}
	}
	return false
}

func normalizedFlagSet(names ...string) map[string]struct{} {
	targets := make(map[string]struct{})
	for _, name := range ExpandFlagNames(names...) {
		targets[name] = struct{}{}
	}
	return targets
}

// FlagAliasMap maps normalized flag/tag names to their list of aliases.
var FlagAliasMap = map[string][]string{
	"poison":   {"poison", "poisoned", "ppoisn", "mpoisn"},
	"poisoned": {"poison", "poisoned", "ppoisn", "mpoisn"},
	"ppoisn":   {"poison", "poisoned", "ppoisn", "mpoisn"},
	"mpoisn":   {"poison", "poisoned", "ppoisn", "mpoisn"},

	"disease":  {"disease", "diseased", "pdisea", "mdisea"},
	"diseased": {"disease", "diseased", "pdisea", "mdisea"},
	"pdisea":   {"disease", "diseased", "pdisea", "mdisea"},
	"mdisea":   {"disease", "diseased", "pdisea", "mdisea"},

	"blind":   {"blind", "blinded", "pblind", "mblind"},
	"blinded": {"blind", "blinded", "pblind", "mblind"},
	"pblind":  {"blind", "blinded", "pblind", "mblind"},
	"mblind":  {"blind", "blinded", "pblind", "mblind"},

	"fear":    {"fear", "fearful", "pfears", "mfears"},
	"fearful": {"fear", "fearful", "pfears", "mfears"},
	"pfears":  {"fear", "fearful", "pfears", "mfears"},
	"mfears":  {"fear", "fearful", "pfears", "mfears"},

	"charm":   {"charm", "charmed", "pcharm", "mcharm"},
	"charmed": {"charm", "charmed", "pcharm", "mcharm"},
	"pcharm":  {"charm", "charmed", "pcharm", "mcharm"},
	"mcharm":  {"charm", "charmed", "pcharm", "mcharm"},

	"silence":  {"silence", "silenced", "psilnc", "msilnc"},
	"silenced": {"silence", "silenced", "psilnc", "msilnc"},
	"psilnc":   {"silence", "silenced", "psilnc", "msilnc"},
	"msilnc":   {"silence", "silenced", "psilnc", "msilnc"},

	"befuddle":  {"befuddle", "befuddled"},
	"befuddled": {"befuddle", "befuddled"},

	"hidden": {"hidden", "phiddn", "mhiddn"},
	"phiddn": {"hidden", "phiddn", "mhiddn"},
	"mhiddn": {"hidden", "phiddn", "mhiddn"},

	"invisible":    {"invisible", "invisibility", "pinvis", "minvis"},
	"invisibility": {"invisible", "invisibility", "pinvis", "minvis"},
	"pinvis":       {"invisible", "invisibility", "pinvis", "minvis"},
	"minvis":       {"invisible", "invisibility", "pinvis", "minvis"},

	"detectinvisible": {"detectinvisible", "detectinvis", "pdinvi", "mdinvi"},
	"detectinvis":     {"detectinvisible", "detectinvis", "pdinvi", "mdinvi"},
	"pdinvi":          {"detectinvisible", "detectinvis", "pdinvi", "mdinvi"},
	"mdinvi":          {"detectinvisible", "detectinvis", "pdinvi", "mdinvi"},

	"bless":   {"bless", "blessed", "pbless"},
	"blessed": {"bless", "blessed", "pbless"},
	"pbless":  {"bless", "blessed", "pbless"},

	"light":  {"light", "plight"},
	"plight": {"light", "plight"},

	"protection": {"protection", "protect", "protected", "pprote"},
	"protect":    {"protection", "protect", "protected", "pprote"},
	"protected":  {"protection", "protect", "protected", "pprote"},
	"pprote":     {"protection", "protect", "protected", "pprote"},

	"resistfire":     {"resistfire", "fireresistance", "prfire"},
	"fireresistance": {"resistfire", "fireresistance", "prfire"},
	"prfire":         {"resistfire", "fireresistance", "prfire"},

	"resistcold":     {"resistcold", "coldresistance", "prcold"},
	"coldresistance": {"resistcold", "coldresistance", "prcold"},
	"prcold":         {"resistcold", "coldresistance", "prcold"},

	"earthshield": {"earthshield", "stoneshield", "psshld"},
	"stoneshield": {"earthshield", "stoneshield", "psshld"},
	"psshld":      {"earthshield", "stoneshield", "psshld"},

	"resistmagic":     {"resistmagic", "magicresistance", "prmagi", "mrmagi"},
	"magicresistance": {"resistmagic", "magicresistance", "prmagi", "mrmagi"},
	"prmagi":          {"resistmagic", "magicresistance", "prmagi", "mrmagi"},
	"mrmagi":          {"resistmagic", "magicresistance", "prmagi", "mrmagi"},

	"prepared": {"prepared", "prepare", "pprepa"},
	"prepare":  {"prepared", "prepare", "pprepa"},
	"pprepa":   {"prepared", "prepare", "pprepa"},

	"levitate":   {"levitate", "levitation", "plevit"},
	"levitation": {"levitate", "levitation", "plevit"},
	"plevit":     {"levitate", "levitation", "plevit"},

	"fly":    {"fly", "flying", "pflysp"},
	"flying": {"fly", "flying", "pflysp"},
	"pflysp": {"fly", "flying", "pflysp"},

	"breathewater":   {"breathewater", "waterbreathing", "pbrwat"},
	"waterbreathing": {"breathewater", "waterbreathing", "pbrwat"},
	"pbrwat":         {"breathewater", "waterbreathing", "pbrwat"},

	"detectmagic": {"detectmagic", "dmagic", "pdmagi"},
	"dmagic":      {"detectmagic", "dmagic", "pdmagi"},
	"pdmagi":      {"detectmagic", "dmagic", "pdmagi"},

	"haste":  {"haste", "hasted", "phaste"},
	"hasted": {"haste", "hasted", "phaste"},
	"phaste": {"haste", "hasted", "phaste"},

	"prayer": {"prayer", "prayed", "pprayd"},
	"prayed": {"prayer", "prayed", "pprayd"},
	"pprayd": {"prayer", "prayed", "pprayd"},

	"meditate":   {"meditate", "meditation", "pmedit"},
	"meditation": {"meditate", "meditation", "pmedit"},
	"pmedit":     {"meditate", "meditation", "pmedit"},

	"power":  {"power", "ppower"},
	"ppower": {"power", "ppower"},

	"knowalignment":  {"knowalignment", "alignmentsense", "pknowa"},
	"alignmentsense": {"knowalignment", "alignmentsense", "pknowa"},
	"pknowa":         {"knowalignment", "alignmentsense", "pknowa"},

	"updamage": {"updamage", "updmg", "pupdmg"},
	"updmg":    {"updamage", "updmg", "pupdmg"},
	"pupdmg":   {"updamage", "updmg", "pupdmg"},

	"reflect":    {"reflect", "reflection", "preflect"},
	"reflection": {"reflect", "reflection", "preflect"},
	"preflect":   {"reflect", "reflection", "preflect"},

	"slayer": {"slayer", "slay", "pslaye"},
	"slay":   {"slayer", "slay", "pslaye"},
	"pslaye": {"slayer", "slay", "pslaye"},

	"angel":  {"angel", "pangel"},
	"pangel": {"angel", "pangel"},

	"married":  {"married", "marriage", "pmarri"},
	"marriage": {"married", "marriage", "pmarri"},
	"pmarri":   {"married", "marriage", "pmarri"},
}

var legacyCreatureFlagAliasGroups = [][]string{
	{"MPERMT", "permanent"},
	{"MHIDDN", "hidden"},
	{"MINVIS", "invisible"},
	{"MTOMEN", "manToMenPlural"},
	{"MDROPS", "noPluralSuffix"},
	{"MNOPRE", "noPrefix"},
	{"MAGGRE", "aggressive"},
	{"MGUARD", "guardTreasure"},
	{"MBLOCK", "blocksExits"},
	{"MFOLLO", "followsAttacker"},
	{"MFLEER", "flees"},
	{"MSCAVE", "scavenger"},
	{"MMALES", "male"},
	{"MPOISS", "poisoner"},
	{"MUNDED", "undead"},
	{"MUNSTL", "cannotSteal"},
	{"MPOISN", "poisoned", "poison"},
	{"MMAGIC", "magicUser", "magic"},
	{"MHASSC", "hasScavenged"},
	{"MBRETH", "breathWeapon", "breath"},
	{"MMGONL", "magicOnly"},
	{"MDINVI", "detectInvisible", "detectInvis"},
	{"MENONL", "magicOrEnchantedOnly"},
	{"MTALKS", "talks"},
	{"MUNKIL", "unkillable"},
	{"MNRGLD", "fixedGold"},
	{"MTLKAG", "talkAggressive"},
	{"MRMAGI", "resistMagic", "magicResistance"},
	{"MBRWP1", "breathWeaponType1"},
	{"MBRWP2", "breathWeaponType2"},
	{"MENEDR", "energyDrain"},
	{"MKNGDM", "kingdom"},
	{"MPLDGK", "pledgeKingdom"},
	{"MRSCND", "rescindKingdom"},
	{"MDISEA", "disease", "diseased"},
	{"MDISIT", "dissolveItems"},
	{"MPURIT", "purchaseItems"},
	{"MTRADE", "tradeItems"},
	{"MPGUAR", "passiveExitGuard"},
	{"MGAGGR", "goodAggressive"},
	{"MEAGGR", "evilAggressive"},
	{"MDEATH", "deathDescription"},
	{"MMAGIO", "magicPercent"},
	{"MRBEFD", "resistStunOnly"},
	{"MNOCIR", "cannotCircle"},
	{"MBLNDR", "blind"},
	{"MDMFOL", "followDM"},
	{"MFEARS", "fearful", "fear"},
	{"MSILNC", "silenced", "silence"},
	{"MBLIND", "blinded"},
	{"MCHARM", "charmed", "charm"},
	{"MBEFUD", "befuddled", "befuddle"},
	{"MKNDM1", "kingdom1"},
	{"MKNDM2", "kingdom2"},
	{"MKNDM3", "kingdom3"},
	{"MKNDM4", "kingdom4"},
	{"MKING1", "king1"},
	{"MKING2", "king2"},
	{"MKING3", "king3"},
	{"MKING4", "king4"},
	{"MSAYTLK", "sayTalk"},
	{"MSUMMO", "summoner", "summon"},
	{"MNOCHA", "noCharm"},
}

var legacyRoomFlagAliasGroups = [][]string{
	{"RSHOPP", "shoppe", "shop"},
	{"RDUMPR", "dump"},
	{"RPAWNS", "pawnShop", "pawn", "pawns"},
	{"RTRAIN", "train", "training"},
	{"RTRAIN4", "trainingBit4", "trainBit4"},
	{"RTRAIN5", "trainingBit5", "trainBit5"},
	{"RTRAIN6", "trainingBit6", "trainBit6"},
	{"RREPAI", "repair", "repairShop"},
	{"RDARKR", "darkAlways"},
	{"RDARKN", "darkNight"},
	{"RPOSTO", "postOffice"},
	{"RNOKIL", "noPlayerKill", "noKill"},
	{"RNOTEL", "noTeleport"},
	{"RHEALR", "healFast"},
	{"RONEPL", "onePlayer"},
	{"RTWOPL", "twoPlayers"},
	{"RTHREE", "threePlayers"},
	{"RNOMAG", "noMagic"},
	{"RPTRAK", "permanentTracks", "permTrack"},
	{"REARTH", "earth"},
	{"RWINDR", "wind"},
	{"RFIRER", "fire"},
	{"RWATER", "water"},
	{"RPLWAN", "playerWander", "groupWander"},
	{"RPHARM", "playerHarm"},
	{"RPPOIS", "playerPoison"},
	{"RPMPDR", "RPMPRD", "playerMPDrain"},
	{"RPBEFU", "playerBefuddle", "confusion"},
	{"RNOLEA", "noSummonOut", "noSummon"},
	{"RPLDGK", "pledge"},
	{"RRSCND", "rescind"},
	{"RNOPOT", "noPotion"},
	{"RPMEXT", "magicExtend", "pmagic"},
	{"RNOLOG", "noLog"},
	{"RELECT", "election"},
	{"RFORGE", "forge"},
	{"RSUVIV", "survival"},
	{"RFAMIL", "family"},
	{"RONFML", "onlyFamily", "familyOnly"},
	{"RBANK", "bank"},
	{"RMARRI", "marriage"},
	{"RONMAR", "onlyMarried", "marriedOnly"},
	{"RCAST", "cast"},
	{"RDEPOT", "depot"},
}

var legacyExitFlagAliasGroups = [][]string{
	{"XSECRT", "XSECRET", "secret"},
	{"XINVIS", "invisible"},
	{"XLOCKD", "XLOCKED", "locked"},
	{"XCLOSD", "XCLOSED", "closed"},
	{"XLOCKS", "lockable"},
	{"XCLOSS", "closable"},
	{"XUNPCK", "unpickable"},
	{"XNAKED", "naked"},
	{"XCLIMB", "climb"},
	{"XREPEL", "repel"},
	{"XDCLIM", "hardClimb", "difficultClimb"},
	{"XFLYSP", "fly"},
	{"XFEMAL", "femaleOnly"},
	{"XMALES", "maleOnly"},
	{"XPLDGK", "pledgeOnly"},
	{"XKNGDM", "kingdomSelector"},
	{"XNGHTO", "nightOnly"},
	{"XDAYON", "XDATON", "dayOnly"},
	{"XPGUAR", "guarded"},
	{"XNOSEE", "noSee"},
	{"XKNDM1", "kingdom1"},
	{"XKNDM2", "kingdom2"},
}

var legacyObjectFlagAliasGroups = [][]string{
	{"OPERMT", "permanent"},
	{"OHIDDN", "hidden"},
	{"OINVIS", "invisible"},
	{"OSOMEA", "somePrefix"},
	{"ODROPS", "noPluralSuffix"},
	{"ONOPRE", "noPrefix"},
	{"OCONTN", "container"},
	{"OWTLES", "weightless"},
	{"OTEMPP", "temporaryPermanent", "tempPermanent"},
	{"OPERM2", "inventoryPermanent"},
	{"ONOMAG", "noMage"},
	{"OLIGHT", "lightSource"},
	{"OGOODO", "goodOnly"},
	{"OEVILO", "evilOnly"},
	{"OENCHA", "enchanted"},
	{"ONOFIX", "noRepair"},
	{"OCLIMB", "climbGear", "climbing"},
	{"ONOTAK", "noTake", "notTake"},
	{"OSCENE", "scenery", "scene"},
	{"OSIZE1", "sizeSmall"},
	{"OSIZE2", "sizeLarge"},
	{"ORENCH", "randomEnchantment", "randEnch"},
	{"OCURSE", "cursed"},
	{"OWEARS", "worn"},
	{"OUSEFL", "useFromFloor"},
	{"OCNDES", "containerDevours", "devours"},
	{"ONOMAL", "femaleOnly", "noMale"},
	{"ONOFEM", "maleOnly", "noFemale"},
	{"ODDICE", "damageDice"},
	{"OPLDGK", "pledgeOnly", "organization"},
	{"OKNGDM", "kingdomBound", "kngdm"},
	{"OCLSEL", "classSelective", "clsSel"},
	{"OASSNO", "classAssassin", "assassinUsable"},
	{"OBARBO", "classBarbarian", "barbarianUsable"},
	{"OCLERO", "classCleric", "clericUsable"},
	{"OFIGHO", "classFighter", "fighterUsable"},
	{"OMAGEO", "classMage", "mageUsable"},
	{"OPALAO", "classPaladin", "paladinUsable"},
	{"ORNGRO", "classRanger", "rangerUsable"},
	{"OTHIEO", "classThief", "thiefUsable"},
	{"OVBEFD", "stunLengthDice"},
	{"ONSHAT", "neverShatter"},
	{"OALCRT", "alwaysCritical"},
	{"OCNAME", "customName"},
	{"OSPECI", "specialItem"},
	{"OMARRI", "marriageOnly"},
	{"OEVENT", "eventItem", "event"},
	{"ONAMED", "named"},
	{"ONOBUN", "noBurn", "noburn"},
	{"OWHELD", "held"},
}

var legacyCreatureFlagAliasIndex = buildLegacyAliasIndex(legacyCreatureFlagAliasGroups)
var legacyRoomFlagAliasIndex = buildLegacyAliasIndex(legacyRoomFlagAliasGroups)
var legacyExitFlagAliasIndex = buildLegacyAliasIndex(legacyExitFlagAliasGroups)
var legacyObjectFlagAliasIndex = buildLegacyAliasIndex(legacyObjectFlagAliasGroups)

func buildLegacyAliasIndex(groups [][]string) map[string][]string {
	index := make(map[string][]string, len(groups)*2)
	for _, group := range groups {
		normalized := make([]string, 0, len(group))
		seen := make(map[string]struct{}, len(group))
		for _, name := range group {
			norm := normalizeFlagName(name)
			if norm == "" {
				continue
			}
			if _, ok := seen[norm]; ok {
				continue
			}
			seen[norm] = struct{}{}
			normalized = append(normalized, norm)
		}
		for _, norm := range normalized {
			index[norm] = normalized
		}
	}
	return index
}

// ExpandFlagNames expands the given flag/tag names to include all known aliases.
func ExpandFlagNames(names ...string) []string {
	var result []string
	seen := make(map[string]struct{})
	for _, name := range names {
		normalized := normalizeFlagName(name)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; !ok {
			seen[normalized] = struct{}{}
			result = append(result, normalized)
		}
		if aliases, ok := FlagAliasMap[normalized]; ok {
			for _, alias := range aliases {
				normAlias := normalizeFlagName(alias)
				if _, ok := seen[normAlias]; !ok {
					seen[normAlias] = struct{}{}
					result = append(result, normAlias)
				}
			}
		}
		if aliases, ok := legacyCreatureFlagAliasIndex[normalized]; ok {
			for _, alias := range aliases {
				normAlias := normalizeFlagName(alias)
				if _, ok := seen[normAlias]; !ok {
					seen[normAlias] = struct{}{}
					result = append(result, normAlias)
				}
			}
		}
		if aliases, ok := legacyRoomFlagAliasIndex[normalized]; ok {
			for _, alias := range aliases {
				normAlias := normalizeFlagName(alias)
				if _, ok := seen[normAlias]; !ok {
					seen[normAlias] = struct{}{}
					result = append(result, normAlias)
				}
			}
		}
		if aliases, ok := legacyExitFlagAliasIndex[normalized]; ok {
			for _, alias := range aliases {
				normAlias := normalizeFlagName(alias)
				if _, ok := seen[normAlias]; !ok {
					seen[normAlias] = struct{}{}
					result = append(result, normAlias)
				}
			}
		}
		if aliases, ok := legacyObjectFlagAliasIndex[normalized]; ok {
			for _, alias := range aliases {
				normAlias := normalizeFlagName(alias)
				if _, ok := seen[normAlias]; !ok {
					seen[normAlias] = struct{}{}
					result = append(result, normAlias)
				}
			}
		}
	}
	return result
}

func propertyFlagEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}

func (w *World) creatureCarriedWeightLocked(creature model.Creature) int {
	seen := map[model.ObjectInstanceID]struct{}{}
	weight := 0
	for _, id := range creature.Inventory.ObjectIDs {
		weight += w.carriedObjectWeightLocked(id, true, seen)
	}
	for _, id := range creature.Equipment {
		weight += w.carriedObjectWeightLocked(id, false, seen)
	}
	return weight
}

func (w *World) carriedObjectWeightLocked(objectID model.ObjectInstanceID, skipWeightless bool, seen map[model.ObjectInstanceID]struct{}) int {
	if objectID.IsZero() {
		return 0
	}
	if _, ok := seen[objectID]; ok {
		return 0
	}
	seen[objectID] = struct{}{}

	object, ok := w.objects[objectID]
	if !ok {
		return 0
	}
	if skipWeightless && w.objectWeightlessLocked(object) {
		return 0
	}

	weight := w.objectOwnWeightLocked(object)
	for _, childID := range object.Contents.ObjectIDs {
		weight += w.carriedObjectWeightLocked(childID, true, seen)
	}
	return weight
}

func (w *World) objectOwnWeightLocked(object model.ObjectInstance) int {
	if weight, ok := parseMoveObjectWeight(object.Properties["weight"]); ok {
		return weight
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := w.prototypes[object.PrototypeID]; ok {
			if weight, ok := parseMoveObjectWeight(proto.Properties["weight"]); ok {
				return weight
			}
		}
	}
	return 0
}

func parseMoveObjectWeight(value string) (int, bool) {
	return parseStateInt(value)
}

func (w *World) objectWeightlessLocked(object model.ObjectInstance) bool {
	if hasAnyNormalizedFlag(object.Metadata.Tags, "weightless", "owtles") {
		return true
	}
	if objectHasAnyPropertyFlag(object.Properties, "weightless", "owtles") {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := w.prototypes[object.PrototypeID]
	if !ok {
		return false
	}
	return hasAnyNormalizedFlag(proto.Metadata.Tags, "weightless", "owtles") ||
		objectHasAnyPropertyFlag(proto.Properties, "weightless", "owtles")
}

func objectFlagContainerProperty(key string) bool {
	switch normalizeFlagName(key) {
	case "flag", "flags":
		return true
	default:
		return false
	}
}

func propertyFlagValueHasAnyToken(value string, targets map[string]struct{}) bool {
	for _, token := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == ' '
	}) {
		if _, ok := targets[normalizeFlagName(token)]; ok {
			return true
		}
	}
	return false
}

func normalizeFlagName(flag string) string {
	flag = strings.ToLower(strings.TrimSpace(flag))
	flag = strings.ReplaceAll(flag, "-", "")
	flag = strings.ReplaceAll(flag, "_", "")
	flag = strings.ReplaceAll(flag, " ", "")
	return flag
}

func parseStateInt(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	return parsed, err == nil
}

func validateFamilyWarPair(firstFamily, secondFamily int) error {
	if firstFamily <= 0 || secondFamily <= 0 {
		return ErrFamilyWarInvalidFamily
	}
	if firstFamily == secondFamily {
		return ErrFamilyWarSelf
	}
	return nil
}

func appendIDOnce[T comparable](ids []T, id T) []T {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func removeID[T comparable](ids []T, id T) []T {
	kept := ids[:0]
	for _, existing := range ids {
		if existing != id {
			kept = append(kept, existing)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

func cloneRoom(room model.Room) model.Room {
	room.Exits = slices.Clone(room.Exits)
	for i := range room.Exits {
		room.Exits[i] = cloneExit(room.Exits[i])
	}
	room.CreatureIDs = slices.Clone(room.CreatureIDs)
	room.PlayerIDs = slices.Clone(room.PlayerIDs)
	room.Objects = cloneObjectRefList(room.Objects)
	room.Properties = maps.Clone(room.Properties)
	room.Metadata = cloneMetadata(room.Metadata)
	return room
}

func cloneExit(exit model.Exit) model.Exit {
	exit.Flags = slices.Clone(exit.Flags)
	exit.Metadata = cloneMetadata(exit.Metadata)
	return exit
}

func clonePlayer(player model.Player) model.Player {
	player.Metadata = cloneMetadata(player.Metadata)
	return player
}

func cloneCreature(creature model.Creature) model.Creature {
	creature.Inventory = cloneObjectRefList(creature.Inventory)
	creature.Equipment = maps.Clone(creature.Equipment)
	creature.Stats = maps.Clone(creature.Stats)
	creature.Properties = maps.Clone(creature.Properties)
	creature.Metadata = cloneMetadata(creature.Metadata)
	return creature
}

func cloneFamily(family model.Family) model.Family {
	family.Members = slices.Clone(family.Members)
	for i := range family.Members {
		family.Members[i].Metadata = cloneMetadata(family.Members[i].Metadata)
	}
	family.Metadata = cloneMetadata(family.Metadata)
	return family
}

func cloneBankAccount(account model.BankAccount) model.BankAccount {
	account.Objects = cloneObjectRefList(account.Objects)
	account.Metadata = cloneMetadata(account.Metadata)
	return account
}

func cloneObject(object model.ObjectInstance) model.ObjectInstance {
	object.Contents = cloneObjectRefList(object.Contents)
	object.Properties = maps.Clone(object.Properties)
	object.Metadata = cloneMetadata(object.Metadata)
	return object
}

func cloneObjectPrototype(proto model.ObjectPrototype) model.ObjectPrototype {
	proto.Keywords = slices.Clone(proto.Keywords)
	proto.Properties = maps.Clone(proto.Properties)
	proto.Metadata = cloneMetadata(proto.Metadata)
	return proto
}

func cloneObjectRefList(refs model.ObjectRefList) model.ObjectRefList {
	refs.ObjectIDs = slices.Clone(refs.ObjectIDs)
	return refs
}

func cloneMetadata(metadata model.Metadata) model.Metadata {
	metadata.RawFields = cloneRawFields(metadata.RawFields)
	metadata.Tags = slices.Clone(metadata.Tags)
	metadata.Notes = slices.Clone(metadata.Notes)
	metadata.PrototypeResolution = clonePrototypeResolution(metadata.PrototypeResolution)
	return metadata
}

func cloneRawFields(fields map[string][]byte) map[string][]byte {
	if fields == nil {
		return nil
	}
	out := make(map[string][]byte, len(fields))
	for key, value := range fields {
		out[key] = slices.Clone(value)
	}
	return out
}

func clonePrototypeResolution(resolution *model.PrototypeResolutionMetadata) *model.PrototypeResolutionMetadata {
	if resolution == nil {
		return nil
	}
	cloned := *resolution
	cloned.Candidates = slices.Clone(resolution.Candidates)
	return &cloned
}

// PurgeRoom deletes all monsters (non-players) and floor objects from the room.
func (w *World) PurgeRoom(roomID model.RoomID) error {
	if w == nil {
		return fmt.Errorf("purge room %q: world state is nil", roomID)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	room, ok := w.rooms[roomID]
	if !ok {
		return fmt.Errorf("purge room %q: room not found", roomID)
	}

	// 1. Purge creatures (excluding players)
	var remainingCreatures []model.CreatureID
	for _, cID := range room.CreatureIDs {
		crt, exists := w.creatures[cID]
		if !exists {
			continue
		}
		if crt.Kind == model.CreatureKindPlayer || !crt.PlayerID.IsZero() {
			remainingCreatures = append(remainingCreatures, cID)
			continue
		}
		delete(w.creatures, cID)
		delete(w.monsterDamage, cID)
	}
	room.CreatureIDs = remainingCreatures

	// 2. Purge floor objects. The C path clears the room object list before
	// deciding whether to free each object, so OTEMPP objects do not remain
	// visible in the room after purge.
	var objIDs []model.ObjectInstanceID
	objIDs = append(objIDs, room.Objects.ObjectIDs...)

	seen := make(map[model.ObjectInstanceID]struct{})
	for _, oID := range objIDs {
		w.deleteObjectTreeLocked(oID, seen)
	}

	// D (Package A): Purging removes objects from room floor.
	// Mark dirty so removal (and any nested container contents) survives restart via sidecar.
	w.MarkRoomObjectsDirty(roomID)

	// Since deleteObjectTreeLocked mutates w.rooms[roomID] through removeObjectFromHolderLocked,
	// we fetch the updated room state, merge our filtered CreatureIDs, and save it back.
	updatedRoom := w.rooms[roomID]
	updatedRoom.CreatureIDs = room.CreatureIDs
	w.rooms[roomID] = updatedRoom

	return nil
}

// backgroundSaver is a minimal worker to pull I/O off the main game loop (C phase start).
func (w *World) backgroundSaver() {
	for req := range w.saveQueue {
		if req.done != nil {
			close(req.done)
			continue
		}
		if !req.playerID.IsZero() {
			if err := w.SavePlayer(req.playerID); err != nil {
				log.Printf("[PERSIST] background SavePlayer failed: %v", err)
			}
		}
		if req.bankID != "" {
			if err := w.SaveBank(req.bankID); err != nil {
				log.Printf("[PERSIST] background SaveBank failed: %v", err)
			}
		}
		// C: board + family news via extended queue (best effort off-loop)
		if req.boardDir != "" {
			if err := w.SaveBoardPosts(req.boardDir); err != nil {
				log.Printf("[PERSIST] background SaveBoardPosts failed: %v", err)
			}
		}
		if req.familyID > 0 {
			// Note: family news content not in queue req; direct SaveFamilyNews preferred from mutation.
			// If cached in future, would save here. For C, flush or direct handles.
			_ = req.familyID
		}
	}
}

// FlushSaveQueue waits until all save requests already queued for this world
// have been processed by the background saver.
func (w *World) FlushSaveQueue() {
	if w == nil || w.saveQueue == nil {
		return
	}
	done := make(chan struct{})
	w.saveQueue <- saveRequest{done: done}
	<-done
}

// QueueSave enqueues a save request (non-blocking best effort).
func (w *World) QueueSave(playerID model.PlayerID, bankID model.BankID) {
	select {
	case w.saveQueue <- saveRequest{playerID: playerID, bankID: bankID}:
	default:
		// queue full, fall back to sync (rare) - log for observability (B)
		log.Printf("[PERSIST] WARN QueueSave fallback sync (queue full) for player=%s bank=%s", playerID, bankID)
		if !playerID.IsZero() {
			if err := w.SavePlayer(playerID); err != nil {
				log.Printf("[PERSIST] ERROR fallback SavePlayer %s: %v", playerID, err)
			}
		}
		if bankID != "" {
			if err := w.SaveBank(bankID); err != nil {
				log.Printf("[PERSIST] ERROR fallback SaveBank %s: %v", bankID, err)
			}
		}
	}
}

// QueueBoardSave enqueues board posts / family news sidecar save (C Package).
// Non-blocking best effort; falls back to direct SaveBoardPosts (family requires direct SaveFamilyNews from site with content).
func (w *World) QueueBoardSave(boardDir string, familyID int) {
	select {
	case w.saveQueue <- saveRequest{boardDir: boardDir, familyID: familyID}:
	default:
		log.Printf("[PERSIST] WARN QueueBoardSave fallback sync (queue full) for board=%s fam=%d", boardDir, familyID)
		if boardDir != "" {
			if err := w.SaveBoardPosts(boardDir); err != nil {
				log.Printf("[PERSIST] ERROR fallback SaveBoardPosts %s: %v", boardDir, err)
			}
		}
		// familyID: caller must use SaveFamilyNews directly if content available
	}
}
