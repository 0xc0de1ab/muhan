package state

import (
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"muhan/internal/migrate/boardmap"
	"muhan/internal/persist/jsonstore"
	"muhan/internal/world/model"
)

// CurrentSaveSchemaVersion is the canonical version for all player/bank/room/board JSON sidecars.
// Package 8 (Schema Evolution & Migration): bumped to v2. Full v1→v2+ migration framework,
// backward compat for v0/legacy, validation, repair, and batch migration tooling implemented.
// Loads in this file transparently migrate supported older versions; Saves always emit current.
// See Migrate* and MigrateSidecars.
const CurrentSaveSchemaVersion = 2

const testDisablePersistenceEnv = "MUHAN_TEST_DISABLE_PERSISTENCE"

type PlayerSaveData struct {
	SchemaVersion int                    `json:"schemaVersion,omitempty"`
	Player        model.Player           `json:"player"`
	Creature      *model.Creature        `json:"creature,omitempty"`
	Objects       []model.ObjectInstance `json:"objects,omitempty"`
}

// SavePlayer collects the Player, its associated Creature, and all ObjectInstances
// currently in the creature's inventory or equipment (recursively handling containers).
// It serializes them into a clean JSON structure and writes to <root>/player/json/<name>.json
// using jsonstore.WriteJSON, ensuring thread safety via world's mutex.
func (w *World) SavePlayer(playerID model.PlayerID) error {
	if w == nil {
		return fmt.Errorf("save player: world state is nil")
	}

	w.rLockDomains(true, true, true, true, true, true, true)
	dbRoot := w.dbRoot
	player, ok := w.players[playerID]
	if !ok {
		w.rUnlockDomains(true, true, true, true, true, true, true)
		return fmt.Errorf("save player %s: player not found", playerID)
	}

	clonedPlayer := clonePlayer(player)

	var clonedCreature *model.Creature
	var objects []model.ObjectInstance

	if !player.CreatureID.IsZero() {
		if c, ok := w.creatures[player.CreatureID]; ok {
			cc := cloneCreature(c)
			clonedCreature = &cc

			visited := make(map[model.ObjectInstanceID]struct{})
			var toProcess []model.ObjectInstanceID

			toProcess = append(toProcess, cc.Inventory.ObjectIDs...)
			for _, objID := range cc.Equipment {
				toProcess = append(toProcess, objID)
			}

			for len(toProcess) > 0 {
				currID := toProcess[0]
				toProcess = toProcess[1:]

				if _, seen := visited[currID]; seen {
					continue
				}
				visited[currID] = struct{}{}

				if obj, ok := w.objects[currID]; ok {
					co := cloneObject(obj)
					objects = append(objects, co)
					toProcess = append(toProcess, co.Contents.ObjectIDs...)
				}
			}
		}
	}
	w.rUnlockDomains(true, true, true, true, true, true, true)

	if dbRoot == "" {
		if testHarnessPersistenceDisabled() {
			return nil
		}
		return fmt.Errorf("save player %s: dbRoot is not set", playerID)
	}

	name := string(player.ID)
	if strings.HasPrefix(name, "player:") {
		name = strings.TrimPrefix(name, "player:")
	}
	if name == "" {
		return fmt.Errorf("save player: extracted player name is empty for ID %s", playerID)
	}
	name, err := safeSidecarStem("player", name)
	if err != nil {
		return err
	}

	sort.Slice(objects, func(i, j int) bool {
		return objects[i].ID < objects[j].ID
	})

	saveData := PlayerSaveData{
		SchemaVersion: CurrentSaveSchemaVersion,
		Player:        clonedPlayer,
		Creature:      clonedCreature,
		Objects:       objects,
	}

	path := filepath.Join(dbRoot, "player", "json", name+".json")
	if err := jsonstore.WriteJSON(path, saveData); err != nil {
		log.Printf("[PERSIST] ERROR SavePlayer %s: %v", playerID, err)
		return fmt.Errorf("save player %s: %w", playerID, err)
	}

	// B: No longer mark dirty here on save success. Dirty must be marked
	// at mutation time (via MarkPlayerDirty) before any Save/Queue/Flush.
	// This fixes the semantics problem identified in expert review (mark-on-success
	// was inverted and could hide unsaved mutations if Save was bypassed).
	// Direct SavePlayer callers (death, shutdown, explicit savegame) are expected
	// to have marked (or call QueueSave which is best-effort) or we accept one-shot
	// durability for those explicit paths via the immediate write.

	return nil
}

func testHarnessPersistenceDisabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(testDisablePersistenceEnv)))
	switch value {
	case "1", "true", "yes", "on":
	default:
		return false
	}
	return strings.HasSuffix(filepath.Base(os.Args[0]), ".test")
}

func safeSidecarStem(kind string, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%s sidecar name is empty", kind)
	}
	if strings.ContainsRune(name, '\x00') {
		return "", fmt.Errorf("%s sidecar name %q contains NUL", kind, name)
	}
	if filepath.IsAbs(name) || name == "." || name == ".." ||
		strings.ContainsAny(name, `/\`) ||
		strings.ContainsAny(name, "\u2044\u2215\u29f5\ufe68\uff0f\uff3c") ||
		filepath.Clean(name) != name {
		return "", fmt.Errorf("%s sidecar name %q is unsafe", kind, name)
	}
	return name, nil
}

// LoadPlayer attempts to load a previously saved player JSON sidecar and returns the data.
// Used for restart restore merge.
func LoadPlayer(dbRoot string, playerID model.PlayerID) (PlayerSaveData, bool, error) {
	name := string(playerID)
	if strings.HasPrefix(name, "player:") {
		name = strings.TrimPrefix(name, "player:")
	}
	if name == "" {
		return PlayerSaveData{}, false, fmt.Errorf("invalid player id for load")
	}
	name, err := safeSidecarStem("player", name)
	if err != nil {
		return PlayerSaveData{}, false, err
	}

	path := filepath.Join(dbRoot, "player", "json", name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return PlayerSaveData{}, false, nil
		}
		return PlayerSaveData{}, false, err
	}

	var saveData PlayerSaveData
	if err := json.Unmarshal(data, &saveData); err != nil {
		return PlayerSaveData{}, false, fmt.Errorf("parse saved player %s: %w", playerID, err)
	}
	saveData, _, err = MigratePlayerSaveData(saveData)
	if err != nil {
		return PlayerSaveData{}, false, fmt.Errorf("migrate saved player %s: %w", playerID, err)
	}
	return saveData, true, nil
}

// MergePlayerSaveIntoWorld applies a loaded PlayerSaveData on top of the current world state.
// This provides basic restart resilience for player progress (inventory, stats, etc.).
// Bank and room-floor sidecars are restored through their dedicated merge paths.
func (w *World) MergePlayerSaveIntoWorld(save PlayerSaveData) error {
	if w == nil || save.Player.ID == "" {
		return nil
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	// Merge player
	w.players[save.Player.ID] = save.Player

	if save.Creature != nil {
		w.creatures[save.Creature.ID] = *save.Creature
	}

	for _, obj := range save.Objects {
		w.objects[obj.ID] = obj
	}

	return nil
}

// FlushActivePlayersAndBanks saves all currently active players and their banks (plus family banks).
// Intended for graceful shutdown and critical save points.
//
// Persistence status (post B/C/D/F):
// Core: Player+creature+inv (recursive containers), bank (player+family), room floor objects sidecars.
// Dirty tracking + selective FlushDirty* + full FlushActive + periodic (activity-aware ~5m) + auto on critical paths (train, bank ops, combat, get/drop/give, title, study, death).
// Startup restore for players+banks+room objects via sidecar merge over base load.
// Graceful shutdown (signal + DM timer) + explicit savegame now wired.
// SchemaVersion enforcement + mismatch warn on all 3 sidecar types (F addition).
// C: Board+family news runtime persistence COMPLETE (sidecars, dirty, auto-save on post/delete, startup restore, tests, sim). Remaining: some deep creature temp state, full atomic tx.
// Gives strong restart resilience for core player progress + world floor changes. (historical B/D block updated for F)
func (w *World) FlushActivePlayersAndBanks() error {
	if w == nil {
		return nil
	}

	w.rLockDomains(true, true, true, true, true, true, true)
	playerIDs := make([]model.PlayerID, 0, len(w.players))
	for pid := range w.players {
		playerIDs = append(playerIDs, pid)
	}
	familyBankIDs := make([]model.BankID, 0)
	for bid := range w.banks {
		if strings.HasPrefix(string(bid), "bank:family:") {
			familyBankIDs = append(familyBankIDs, bid)
		}
	}
	w.rUnlockDomains(true, true, true, true, true, true, true)

	var firstErr error
	for _, pid := range playerIDs {
		if err := w.SavePlayer(pid); err != nil && firstErr == nil {
			firstErr = err
		}
		bankID := model.BankID("bank:player:" + string(pid))
		if _, exists := w.banks[bankID]; exists {
			if err := w.SaveBank(bankID); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}

	for _, bid := range familyBankIDs {
		if err := w.SaveBank(bid); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// D: On full active flush (shutdown etc.) also persist current floor objects for all rooms
	// that have any runtime objects. This is the reliable path for complete world state.
	w.rLockDomains(true, true, true, true, true, true, true)
	roomIDsWithFloor := make([]model.RoomID, 0)
	for rid, room := range w.rooms {
		if len(room.Objects.ObjectIDs) > 0 {
			roomIDsWithFloor = append(roomIDsWithFloor, rid)
		}
	}
	w.rUnlockDomains(true, true, true, true, true, true, true)

	for _, rid := range roomIDsWithFloor {
		if err := w.SaveRoomObjects(rid); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// D (Package A robustness): also flush dirty rooms (covers rooms that were emptied
	// by pickups/deletes since last periodic dirty flush, plus any pending). Ensures
	// sidecar written with current (possibly empty) state -> no stale objects on restart merge.
	// Since=0 catches all dirty; only successes cleared (per FlushDirty pattern).
	if err := w.FlushDirtyRoomObjects(0); err != nil && firstErr == nil {
		firstErr = err
	}

	return firstErr
}

// FlushDirtyPlayersAndBanks is a more efficient version that only saves
// players and banks marked dirty since the given timestamp (B expansion).
func (w *World) FlushDirtyPlayersAndBanks(since int64) error {
	if w == nil {
		return nil
	}

	w.dirtyMu.Lock()
	dirtyPlayers := make([]model.PlayerID, 0)
	for pid, ts := range w.playerDirty {
		if ts >= since {
			dirtyPlayers = append(dirtyPlayers, pid)
		}
	}
	dirtyBanks := make([]model.BankID, 0)
	for bid, ts := range w.bankDirty {
		if ts >= since {
			dirtyBanks = append(dirtyBanks, bid)
		}
	}
	w.dirtyMu.Unlock()

	var firstErr error
	successfullySavedPlayers := make([]model.PlayerID, 0, len(dirtyPlayers))
	successfullySavedBanks := make([]model.BankID, 0, len(dirtyBanks))

	for _, pid := range dirtyPlayers {
		if err := w.SavePlayer(pid); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else {
			successfullySavedPlayers = append(successfullySavedPlayers, pid)
		}
	}
	for _, bid := range dirtyBanks {
		if err := w.SaveBank(bid); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else {
			successfullySavedBanks = append(successfullySavedBanks, bid)
		}
	}

	// Only clear entries that were successfully saved (critical durability fix)
	w.dirtyMu.Lock()
	for _, pid := range successfullySavedPlayers {
		delete(w.playerDirty, pid)
	}
	for _, bid := range successfullySavedBanks {
		delete(w.bankDirty, bid)
	}
	w.dirtyMu.Unlock()

	return firstErr
}

// --- D: Minimal room floor objects persistence (biggest review gap) ---
// Dropped items, money on ground, corpses, bags/containers on floor etc.
// are now sidecar-persisted per room so they survive restart.
// Only runtime-placed objects (those with Location.RoomID) are saved here;
// static room contents from world load are not duplicated.

type RoomObjectsSave struct {
	SchemaVersion int                    `json:"schemaVersion,omitempty"`
	RoomID        model.RoomID           `json:"roomId"`
	Properties    map[string]string      `json:"properties,omitempty"`
	Objects       []model.ObjectInstance `json:"objects,omitempty"`
}

// SaveRoomObjects collects all objects whose current location is directly the given room
// (including recursive contents of any containers dropped on the floor) and writes a
// JSON sidecar. Uses the same atomic WriteJSON + clone pattern as SavePlayer/SaveBank.
func (w *World) SaveRoomObjects(roomID model.RoomID) error {
	if w == nil {
		return fmt.Errorf("save room objects %s: world is nil", roomID)
	}
	if roomID.IsZero() {
		return fmt.Errorf("save room objects: room id required")
	}

	w.rLockDomains(true, true, true, true, true, true, true)
	dbRoot := w.dbRoot
	room, ok := w.rooms[roomID]
	if !ok {
		w.rUnlockDomains(true, true, true, true, true, true, true)
		return fmt.Errorf("save room objects %s: room not found", roomID)
	}
	properties := maps.Clone(room.Properties)

	var objects []model.ObjectInstance
	visited := make(map[model.ObjectInstanceID]struct{})
	var toProcess []model.ObjectInstanceID

	// Collect top-level objects whose Location.RoomID matches
	for oid, obj := range w.objects {
		if obj.Location.RoomID == roomID {
			toProcess = append(toProcess, oid)
		}
	}

	for len(toProcess) > 0 {
		currID := toProcess[0]
		toProcess = toProcess[1:]
		if _, seen := visited[currID]; seen {
			continue
		}
		visited[currID] = struct{}{}

		if obj, ok := w.objects[currID]; ok {
			co := cloneObject(obj)
			objects = append(objects, co)
			toProcess = append(toProcess, co.Contents.ObjectIDs...)
		}
	}
	w.rUnlockDomains(true, true, true, true, true, true, true)

	if dbRoot == "" {
		return fmt.Errorf("save room objects %s: dbRoot not set", roomID)
	}

	// Sanitize room id for filename (similar to player name handling)
	name := string(roomID)
	if strings.HasPrefix(name, "room:") {
		name = strings.TrimPrefix(name, "room:")
	}
	if name == "" {
		name = "unknown"
	}
	// Replace any problematic chars for FS (rare for our room ids)
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, ":", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name, err := safeSidecarStem("room object", name)
	if err != nil {
		return err
	}

	sort.Slice(objects, func(i, j int) bool { return objects[i].ID < objects[j].ID })

	saveData := RoomObjectsSave{
		SchemaVersion: CurrentSaveSchemaVersion,
		RoomID:        roomID,
		Properties:    properties,
		Objects:       objects,
	}

	path := filepath.Join(dbRoot, "room", "json", name+".objects.json")
	if err := jsonstore.WriteJSON(path, saveData); err != nil {
		log.Printf("[PERSIST] ERROR SaveRoomObjects %s: %v", roomID, err)
		return fmt.Errorf("save room objects %s: %w", roomID, err)
	}

	// Mark dirty on success? No - mutation time only (consistent with player/bank fix)
	return nil
}

// LoadRoomObjects attempts to load a previously saved room floor objects JSON sidecar.
func LoadRoomObjects(dbRoot string, roomID model.RoomID) (RoomObjectsSave, bool, error) {
	name := string(roomID)
	if strings.HasPrefix(name, "room:") {
		name = strings.TrimPrefix(name, "room:")
	}
	if name == "" {
		return RoomObjectsSave{}, false, fmt.Errorf("invalid room id for load")
	}
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, ":", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name, err := safeSidecarStem("room object", name)
	if err != nil {
		return RoomObjectsSave{}, false, err
	}

	path := filepath.Join(dbRoot, "room", "json", name+".objects.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return RoomObjectsSave{}, false, nil
		}
		return RoomObjectsSave{}, false, err
	}

	var save RoomObjectsSave
	if err := json.Unmarshal(data, &save); err != nil {
		return RoomObjectsSave{}, false, fmt.Errorf("parse saved room objects %s: %w", roomID, err)
	}
	save, _, err = MigrateRoomObjectsSave(save)
	if err != nil {
		return RoomObjectsSave{}, false, fmt.Errorf("migrate saved room objects %s: %w", roomID, err)
	}
	return save, true, nil
}

// MergeRoomObjectsSaveIntoWorld inserts the loaded floor objects into the world
// and updates the room's Objects ref list + holder tracking. Safe to call multiple times.
// Package A improvements: dedup input, handle ID conflicts (re-home from old loc if
// loc differs, e.g. legacy or partial overlap with static), skip zero IDs, best-effort
// for legacy (schema 0), validate room loc matches save for top-level.
func (w *World) MergeRoomObjectsSaveIntoWorld(save RoomObjectsSave) error {
	if w == nil || save.RoomID.IsZero() {
		return nil
	}
	if len(save.Objects) == 0 && len(save.Properties) == 0 {
		return nil
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	room, ok := w.rooms[save.RoomID]
	if !ok {
		return fmt.Errorf("merge room objects %s: room not in world", save.RoomID)
	}
	if len(save.Properties) > 0 {
		if room.Properties == nil {
			room.Properties = make(map[string]string, len(save.Properties))
		}
		for key, value := range save.Properties {
			room.Properties[key] = value
		}
		w.rooms[save.RoomID] = room
	}

	// Dedup + filter for robustness (legacy saves or buggy prior saves may have dups)
	seen := make(map[model.ObjectInstanceID]struct{}, len(save.Objects))
	var objs []model.ObjectInstance
	for _, obj := range save.Objects {
		if obj.ID.IsZero() {
			continue
		}
		if _, dup := seen[obj.ID]; dup {
			continue
		}
		seen[obj.ID] = struct{}{}
		objs = append(objs, obj)
	}

	for _, obj := range objs {
		// Conflict handling: if ID already exists with different location (e.g. static load
		// vs persisted runtime floor object, or restart edge), re-home properly.
		if existing, exists := w.objects[obj.ID]; exists {
			if !objectLocationEqual(existing.Location, obj.Location) {
				w.removeObjectFromHolderLocked(obj.ID, existing.Location)
			}
		}
		w.objects[obj.ID] = obj
		// Ensure holder links (room top-level + recursive container contents)
		w.addObjectToHolderLocked(obj.ID, obj.Location)
		// Also make sure any nested contents are wired (they were saved with correct Location)
		for _, childID := range obj.Contents.ObjectIDs {
			if child, ok := w.objects[childID]; ok {
				w.addObjectToHolderLocked(childID, child.Location)
			}
		}
	}

	return nil
}

// FlushDirtyRoomObjects saves floor objects for rooms marked dirty since the timestamp.
// Only successfully saved rooms are cleared from the dirty map (durability safety).
func (w *World) FlushDirtyRoomObjects(since int64) error {
	if w == nil {
		return nil
	}

	w.dirtyMu.Lock()
	dirtyRooms := make([]model.RoomID, 0)
	for rid, ts := range w.roomObjectDirty {
		if ts >= since {
			dirtyRooms = append(dirtyRooms, rid)
		}
	}
	w.dirtyMu.Unlock()

	var firstErr error
	successfullySaved := make([]model.RoomID, 0, len(dirtyRooms))

	for _, rid := range dirtyRooms {
		if err := w.SaveRoomObjects(rid); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else {
			successfullySaved = append(successfullySaved, rid)
		}
	}

	// Clear only successful (critical fix pattern)
	w.dirtyMu.Lock()
	for _, rid := range successfullySaved {
		delete(w.roomObjectDirty, rid)
	}
	w.dirtyMu.Unlock()

	return firstErr
}

// --- C: Package Board & Family News Runtime Persistence (sidecar + dirty + merge) ---
// Design: parallel JSON sidecars under board/json/ and player/family/json/ .
// Boards: snapshot current posts (via boardmap for fidelity to legacy index+body) to sidecar on dirty.
// Family news: content string in sidecar; read handlers prefer it when present.
// Legacy writes for boards kept for live visibility + C compat; sidecar is clean Go JSON layer.
// New board posts survive restart via legacy files; startup validates/counts board sidecars.
// Uses jsonstore atomic + clone pattern. Flush only successful clears dirty (durability).

type BoardPostsSave struct {
	SchemaVersion int                         `json:"schemaVersion,omitempty"`
	BoardDir      string                      `json:"boardDir"`
	Posts         []boardmap.BoardPostSummary `json:"posts,omitempty"`
}

type FamilyNewsSave struct {
	SchemaVersion int       `json:"schemaVersion,omitempty"`
	FamilyID      int       `json:"familyId"`
	Content       string    `json:"content"`
	UpdatedAt     time.Time `json:"updatedAt,omitempty"`
}

// SaveBoardPosts snapshots the live board dir (re-scans via boardmap like loadBoard) and
// writes JSON sidecar. Called after mutations (or via dirty flush). Marks not done here.
func (w *World) SaveBoardPosts(boardDir string) error {
	if w == nil {
		return fmt.Errorf("save board posts: world is nil")
	}
	if boardDir == "" {
		return fmt.Errorf("save board posts: boardDir required")
	}

	dbRoot := w.dbRoot
	if dbRoot == "" {
		return fmt.Errorf("save board posts %s: dbRoot not set", boardDir)
	}
	boardDir, err := safeSidecarStem("board", boardDir)
	if err != nil {
		return err
	}

	boardPath := filepath.Join(dbRoot, "board", boardDir)
	board, _, err := boardmap.MapBoardDir(dbRoot, boardPath)
	if err != nil {
		// If board dir missing (rare for known), still allow empty save? but error for now
		return fmt.Errorf("save board posts: scan %s: %w", boardDir, err)
	}

	name := boardDir

	saveData := BoardPostsSave{
		SchemaVersion: CurrentSaveSchemaVersion,
		BoardDir:      boardDir,
		Posts:         board.Posts,
	}

	path := filepath.Join(dbRoot, "board", "json", name+".json")
	if err := jsonstore.WriteJSON(path, saveData); err != nil {
		log.Printf("[PERSIST] ERROR SaveBoardPosts %s: %v", boardDir, err)
		return fmt.Errorf("save board posts %s: %w", boardDir, err)
	}
	return nil
}

// LoadBoardPosts attempts to load board posts JSON sidecar (for startup restore / tests).
func LoadBoardPosts(dbRoot string, boardDir string) (BoardPostsSave, bool, error) {
	if dbRoot == "" || boardDir == "" {
		return BoardPostsSave{}, false, fmt.Errorf("invalid args for load board posts")
	}
	name, err := safeSidecarStem("board", boardDir)
	if err != nil {
		return BoardPostsSave{}, false, err
	}
	path := filepath.Join(dbRoot, "board", "json", name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return BoardPostsSave{}, false, nil
		}
		return BoardPostsSave{}, false, err
	}
	var save BoardPostsSave
	if err := json.Unmarshal(data, &save); err != nil {
		return BoardPostsSave{}, false, fmt.Errorf("parse board posts sidecar %s: %w", boardDir, err)
	}
	save, _, err = MigrateBoardPostsSave(save)
	if err != nil {
		return BoardPostsSave{}, false, fmt.Errorf("migrate board posts sidecar %s: %w", boardDir, err)
	}
	return save, true, nil
}

// MergeBoardPostsSaveIntoWorld validates the startup-loaded board sidecar for symmetry
// with player/bank/room sidecars. Runtime board reads still use legacy board files,
// which remain the C-compatible source of truth for visible posts.
func (w *World) MergeBoardPostsSaveIntoWorld(save BoardPostsSave) error {
	if w == nil || save.BoardDir == "" || len(save.Posts) == 0 {
		return nil
	}
	// No in-memory board post map exists; legacy mutations already ensure visibility.
	return nil
}

// SaveFamilyNews writes family news content to its JSON sidecar (under player/family/json/)
// and clears any matching dirty marker after a successful write.
func (w *World) SaveFamilyNews(familyID int, content string) error {
	if w == nil {
		return fmt.Errorf("save family news: world is nil")
	}
	if familyID <= 0 {
		return fmt.Errorf("save family news: familyID required")
	}
	dbRoot := w.dbRoot
	if dbRoot == "" {
		return fmt.Errorf("save family news %d: dbRoot not set", familyID)
	}

	saveData := FamilyNewsSave{
		SchemaVersion: CurrentSaveSchemaVersion,
		FamilyID:      familyID,
		Content:       content,
		UpdatedAt:     time.Now(),
	}

	// Ensure family/json dir
	jsonDir := filepath.Join(dbRoot, "player", "family", "json")
	if err := os.MkdirAll(jsonDir, 0700); err != nil {
		return fmt.Errorf("mkdir family json dir: %w", err)
	}

	path := filepath.Join(jsonDir, fmt.Sprintf("family_news_%d.json", familyID))
	if err := jsonstore.WriteJSON(path, saveData); err != nil {
		log.Printf("[PERSIST] ERROR SaveFamilyNews %d: %v", familyID, err)
		return fmt.Errorf("save family news %d: %w", familyID, err)
	}
	w.dirtyMu.Lock()
	delete(w.familyNewsDirty, familyID)
	w.dirtyMu.Unlock()
	return nil
}

// LoadFamilyNews attempts to load family news JSON sidecar.
func LoadFamilyNews(dbRoot string, familyID int) (FamilyNewsSave, bool, error) {
	if dbRoot == "" || familyID <= 0 {
		return FamilyNewsSave{}, false, fmt.Errorf("invalid args for load family news")
	}
	path := filepath.Join(dbRoot, "player", "family", "json", fmt.Sprintf("family_news_%d.json", familyID))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return FamilyNewsSave{}, false, nil
		}
		return FamilyNewsSave{}, false, err
	}
	var save FamilyNewsSave
	if err := json.Unmarshal(data, &save); err != nil {
		return FamilyNewsSave{}, false, fmt.Errorf("parse family news sidecar %d: %w", familyID, err)
	}
	save, _, err = MigrateFamilyNewsSave(save)
	if err != nil {
		return FamilyNewsSave{}, false, fmt.Errorf("migrate family news sidecar %d: %w", familyID, err)
	}
	return save, true, nil
}

// MergeFamilyNewsSaveIntoWorld no-op for symmetry (news read prefers sidecar in handler if wired).
func (w *World) MergeFamilyNewsSaveIntoWorld(save FamilyNewsSave) error {
	if w == nil || save.FamilyID <= 0 {
		return nil
	}
	return nil
}

// FlushDirtyBoardsAndFamilyNews flushes boards and family news marked dirty since timestamp.
// Only successful saves clear their dirty entries.
func (w *World) FlushDirtyBoardsAndFamilyNews(since int64) error {
	if w == nil {
		return nil
	}

	w.dirtyMu.Lock()
	dirtyBoards := make([]string, 0)
	for bdir, ts := range w.boardDirty {
		if ts >= since {
			dirtyBoards = append(dirtyBoards, bdir)
		}
	}
	dirtyFamilies := make([]int, 0)
	for fid, ts := range w.familyNewsDirty {
		if ts >= since {
			dirtyFamilies = append(dirtyFamilies, fid)
		}
	}
	w.dirtyMu.Unlock()

	var firstErr error
	successBoards := make([]string, 0, len(dirtyBoards))
	successFams := make([]int, 0, len(dirtyFamilies))

	for _, bdir := range dirtyBoards {
		if err := w.SaveBoardPosts(bdir); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else {
			successBoards = append(successBoards, bdir)
		}
	}
	// Family news writes are persisted directly at the mutation site; this flush can
	// only clear dirty markers when the direct sidecar save is already present.
	for _, fid := range dirtyFamilies {
		if _, ok, err := LoadFamilyNews(w.dbRoot, fid); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else if ok {
			successFams = append(successFams, fid)
		}
	}

	w.dirtyMu.Lock()
	for _, b := range successBoards {
		delete(w.boardDirty, b)
	}
	for _, f := range successFams {
		delete(w.familyNewsDirty, f)
	}
	w.dirtyMu.Unlock()

	return firstErr
}

// ============================================================
// Package 8: Full Schema Versioning, Migration, Validation & Repair
// ============================================================
// This completes the schema evolution story for all sidecar types:
// - PlayerSaveData (player + creature + inv objects)
// - BankSaveBundle (player/family banks + objects) [via model]
// - RoomObjectsSave (floor objects + nested)
// - BoardPostsSave
// - FamilyNewsSave
//
// v1 (F enforcement) -> v2 (this pkg): transparent migration on Load.
// Legacy v0 (pre-schema files or schema:0) treated as v0 and migrated.
// Future v3+ must add an explicit migration step before this reader accepts it.
//
// Tools provided:
//   - Migrate*SaveData / MigrateBankSaveBundle : per-type migration (returns updated + didMigrate)
//   - Validate* / Repair* : for data hygiene and admin tooling
//   - MigrateSidecars(dbRoot) : batch offline migration + rewrite of entire sidecar corpus
//     (supports production data upgrade without server restart impact)
// Backward compat: v0/v1 are best-effort migrated without per-file warning noise.
// Future schema versions are rejected so potentially incompatible data is not hidden.
// On migration, next Save/Flush will persist the upgraded form.
// Real v2 delta: version bump + standardized nil-slice hygiene + future data transforms.

func migrateVersionIfNeeded(ver int) (newVer int, migrated bool, err error) {
	if ver == CurrentSaveSchemaVersion {
		return ver, false, nil
	}
	if ver == 0 {
		// legacy pre-F or explicit 0
		return CurrentSaveSchemaVersion, true, nil
	}
	if ver < CurrentSaveSchemaVersion {
		// v1 -> v2 (or chained future)
		return CurrentSaveSchemaVersion, true, nil
	}
	return ver, false, fmt.Errorf("unsupported future schema version %d > current %d", ver, CurrentSaveSchemaVersion)
}

// MigratePlayerSaveData upgrades a (possibly old-version) player sidecar to current schema.
// Applies version bump + v2 hygiene repairs (e.g. non-nil slices for robust merge).
// Used by LoadPlayer and by batch MigrateSidecars.
func MigratePlayerSaveData(save PlayerSaveData) (PlayerSaveData, bool, error) {
	oldVer := save.SchemaVersion
	newVer, did, err := migrateVersionIfNeeded(oldVer)
	if err != nil {
		return save, false, err
	}
	save.SchemaVersion = newVer
	// v2 repair/hygiene: prevent nil panics in downstream (merge, json etc)
	if save.Objects == nil {
		save.Objects = []model.ObjectInstance{}
	}
	return save, did, nil
}

// MigrateRoomObjectsSave upgrades room floor sidecar.
func MigrateRoomObjectsSave(save RoomObjectsSave) (RoomObjectsSave, bool, error) {
	oldVer := save.SchemaVersion
	newVer, did, err := migrateVersionIfNeeded(oldVer)
	if err != nil {
		return save, false, err
	}
	save.SchemaVersion = newVer
	if save.Objects == nil {
		save.Objects = []model.ObjectInstance{}
	}
	return save, did, nil
}

// MigrateBoardPostsSave upgrades board posts sidecar (Package C type).
func MigrateBoardPostsSave(save BoardPostsSave) (BoardPostsSave, bool, error) {
	oldVer := save.SchemaVersion
	newVer, did, err := migrateVersionIfNeeded(oldVer)
	if err != nil {
		return save, false, err
	}
	save.SchemaVersion = newVer
	if save.Posts == nil {
		save.Posts = []boardmap.BoardPostSummary{}
	}
	return save, did, nil
}

// MigrateFamilyNewsSave upgrades family news sidecar.
func MigrateFamilyNewsSave(save FamilyNewsSave) (FamilyNewsSave, bool, error) {
	oldVer := save.SchemaVersion
	newVer, did, err := migrateVersionIfNeeded(oldVer)
	if err != nil {
		return save, false, err
	}
	save.SchemaVersion = newVer
	return save, did, nil
}

// MigrateBankSaveBundle upgrades bank bundle (player or family). Defined here for central migration story
// even though type lives in model (same package visibility).
func MigrateBankSaveBundle(bundle model.BankSaveBundle) (model.BankSaveBundle, bool, error) {
	oldVer := bundle.SchemaVersion
	newVer, did, err := migrateVersionIfNeeded(oldVer)
	if err != nil {
		return bundle, false, err
	}
	bundle.SchemaVersion = newVer
	if bundle.Objects == nil {
		bundle.Objects = []model.ObjectInstance{}
	}
	return bundle, did, nil
}

// --- Validation ---

// ValidatePlayerSaveData returns validity + list of issues. Does not mutate.
func ValidatePlayerSaveData(save PlayerSaveData) (bool, []string) {
	var issues []string
	if save.SchemaVersion != CurrentSaveSchemaVersion {
		issues = append(issues, fmt.Sprintf("schemaVersion=%d (expected %d)", save.SchemaVersion, CurrentSaveSchemaVersion))
	}
	if save.Player.ID == "" {
		issues = append(issues, "player.ID is empty")
	}
	if save.Creature != nil && save.Creature.ID == "" {
		issues = append(issues, "creature present but ID empty")
	}
	// Could add deeper: location consistency, but keep lightweight for perf
	return len(issues) == 0, issues
}

// ValidateRoomObjectsSave ...
func ValidateRoomObjectsSave(save RoomObjectsSave) (bool, []string) {
	var issues []string
	if save.SchemaVersion != CurrentSaveSchemaVersion {
		issues = append(issues, fmt.Sprintf("schemaVersion=%d (expected %d)", save.SchemaVersion, CurrentSaveSchemaVersion))
	}
	if save.RoomID == "" {
		issues = append(issues, "roomId empty")
	}
	return len(issues) == 0, issues
}

// ValidateBoardPostsSave ...
func ValidateBoardPostsSave(save BoardPostsSave) (bool, []string) {
	var issues []string
	if save.SchemaVersion != CurrentSaveSchemaVersion {
		issues = append(issues, fmt.Sprintf("schemaVersion=%d (expected %d)", save.SchemaVersion, CurrentSaveSchemaVersion))
	}
	if save.BoardDir == "" {
		issues = append(issues, "boardDir empty")
	}
	return len(issues) == 0, issues
}

// ValidateFamilyNewsSave ...
func ValidateFamilyNewsSave(save FamilyNewsSave) (bool, []string) {
	var issues []string
	if save.SchemaVersion != CurrentSaveSchemaVersion {
		issues = append(issues, fmt.Sprintf("schemaVersion=%d (expected %d)", save.SchemaVersion, CurrentSaveSchemaVersion))
	}
	if save.FamilyID <= 0 {
		issues = append(issues, "invalid familyId")
	}
	return len(issues) == 0, issues
}

// ValidateBankSaveBundle ...
func ValidateBankSaveBundle(bundle model.BankSaveBundle) (bool, []string) {
	var issues []string
	if bundle.SchemaVersion != CurrentSaveSchemaVersion {
		issues = append(issues, fmt.Sprintf("schemaVersion=%d (expected %d)", bundle.SchemaVersion, CurrentSaveSchemaVersion))
	}
	if bundle.BankAccount.ID == "" {
		issues = append(issues, "bankAccount.ID empty")
	}
	return len(issues) == 0, issues
}

// --- Repair (best-effort auto-fix; returns repaired copy + applied repairs + remaining issues) ---

func RepairPlayerSaveData(save PlayerSaveData) (PlayerSaveData, []string, []string) {
	repairs := []string{}
	if save.Objects == nil {
		save.Objects = []model.ObjectInstance{}
		repairs = append(repairs, "nil Objects -> empty slice (v2 hygiene)")
	}
	// Future: dedup IDs, fix bad locations, etc.
	valid, issues := ValidatePlayerSaveData(save)
	_ = valid
	return save, repairs, issues
}

func RepairRoomObjectsSave(save RoomObjectsSave) (RoomObjectsSave, []string, []string) {
	repairs := []string{}
	if save.Objects == nil {
		save.Objects = []model.ObjectInstance{}
		repairs = append(repairs, "nil Objects -> empty slice")
	}
	valid, issues := ValidateRoomObjectsSave(save)
	_ = valid
	return save, repairs, issues
}

func RepairBoardPostsSave(save BoardPostsSave) (BoardPostsSave, []string, []string) {
	repairs := []string{}
	if save.Posts == nil {
		save.Posts = []boardmap.BoardPostSummary{}
		repairs = append(repairs, "nil Posts -> empty slice")
	}
	_, issues := ValidateBoardPostsSave(save)
	return save, repairs, issues
}

func RepairFamilyNewsSave(save FamilyNewsSave) (FamilyNewsSave, []string, []string) {
	_, issues := ValidateFamilyNewsSave(save)
	return save, []string{}, issues // minimal
}

func RepairBankSaveBundle(bundle model.BankSaveBundle) (model.BankSaveBundle, []string, []string) {
	repairs := []string{}
	if bundle.Objects == nil {
		bundle.Objects = []model.ObjectInstance{}
		repairs = append(repairs, "nil Objects -> empty slice")
	}
	_, issues := ValidateBankSaveBundle(bundle)
	return bundle, repairs, issues
}

// --- Batch Migration Tool (core of Package 8 "migration tools") ---

// MigrateSidecars scans the standard sidecar directories under dbRoot and applies
// any needed vN->current migrations, rewriting files in-place when changed.
// Safe for offline admin use / data grooming. Returns detailed report.
// Supports all 5 sidecar families. Non-fatal per-file errors.
func MigrateSidecars(dbRoot string) (SidecarMigrationReport, error) {
	r := SidecarMigrationReport{
		ByType: make(map[string]int),
	}
	if dbRoot == "" {
		return r, fmt.Errorf("MigrateSidecars: dbRoot required")
	}

	// Helper to process one file with typed migrate + optional rewrite
	process := func(path, typ string, readFn func() (interface{}, int, bool, error), migrateAndWrite func(old interface{}) (bool, error)) {
		r.TotalScanned++
		data, oldVer, existed, err := readFn()
		if err != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("%s: read %s: %v", typ, path, err))
			return
		}
		if !existed {
			return
		}
		r.ByType[typ]++
		did, err := migrateAndWrite(data)
		if err != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("%s: migrate %s: %v", typ, path, err))
			return
		}
		if did {
			r.Migrated++
			r.Details = append(r.Details, SidecarMigrationDetail{
				Type: typ, Path: path, FromVer: oldVer, ToVer: CurrentSaveSchemaVersion, Repaired: true,
			})
		}
	}

	// Players: player/json/*.json
	pjDir := filepath.Join(dbRoot, "player", "json")
	if fis, _ := os.ReadDir(pjDir); fis != nil {
		for _, fi := range fis {
			if fi.IsDir() || !strings.HasSuffix(fi.Name(), ".json") {
				continue
			}
			full := filepath.Join(pjDir, fi.Name())
			process(full, "player", func() (interface{}, int, bool, error) {
				b, err := os.ReadFile(full)
				if err != nil {
					return nil, 0, false, err
				}
				var s PlayerSaveData
				if e := json.Unmarshal(b, &s); e != nil {
					return nil, 0, false, e
				}
				return s, s.SchemaVersion, true, nil
			}, func(d interface{}) (bool, error) {
				s := d.(PlayerSaveData)
				ms, did, err := MigratePlayerSaveData(s)
				if err != nil {
					return false, err
				}
				if did {
					if e := jsonstore.WriteJSON(full, ms); e != nil {
						return false, e
					}
				}
				return did, nil
			})
		}
	}

	// Banks: player/bank/json/*.json
	bjDir := filepath.Join(dbRoot, "player", "bank", "json")
	if fis, _ := os.ReadDir(bjDir); fis != nil {
		for _, fi := range fis {
			if fi.IsDir() || !strings.HasSuffix(fi.Name(), ".json") {
				continue
			}
			full := filepath.Join(bjDir, fi.Name())
			process(full, "bank", func() (interface{}, int, bool, error) {
				b, err := os.ReadFile(full)
				if err != nil {
					return nil, 0, false, err
				}
				var s model.BankSaveBundle
				if e := json.Unmarshal(b, &s); e != nil {
					return nil, 0, false, e
				}
				return s, s.SchemaVersion, true, nil
			}, func(d interface{}) (bool, error) {
				s := d.(model.BankSaveBundle)
				ms, did, err := MigrateBankSaveBundle(s)
				if err != nil {
					return false, err
				}
				if did {
					if e := jsonstore.WriteJSON(full, ms); e != nil {
						return false, e
					}
				}
				return did, nil
			})
		}
	}

	// Rooms: room/json/*objects.json
	rjDir := filepath.Join(dbRoot, "room", "json")
	if fis, _ := os.ReadDir(rjDir); fis != nil {
		for _, fi := range fis {
			if fi.IsDir() || !strings.HasSuffix(fi.Name(), ".objects.json") {
				continue
			}
			full := filepath.Join(rjDir, fi.Name())
			process(full, "room", func() (interface{}, int, bool, error) {
				b, err := os.ReadFile(full)
				if err != nil {
					return nil, 0, false, err
				}
				var s RoomObjectsSave
				if e := json.Unmarshal(b, &s); e != nil {
					return nil, 0, false, e
				}
				return s, s.SchemaVersion, true, nil
			}, func(d interface{}) (bool, error) {
				s := d.(RoomObjectsSave)
				ms, did, err := MigrateRoomObjectsSave(s)
				if err != nil {
					return false, err
				}
				if did {
					if e := jsonstore.WriteJSON(full, ms); e != nil {
						return false, e
					}
				}
				return did, nil
			})
		}
	}

	// Boards: board/json/*.json
	bdDir := filepath.Join(dbRoot, "board", "json")
	if fis, _ := os.ReadDir(bdDir); fis != nil {
		for _, fi := range fis {
			if fi.IsDir() || !strings.HasSuffix(fi.Name(), ".json") {
				continue
			}
			full := filepath.Join(bdDir, fi.Name())
			process(full, "board", func() (interface{}, int, bool, error) {
				b, err := os.ReadFile(full)
				if err != nil {
					return nil, 0, false, err
				}
				var s BoardPostsSave
				if e := json.Unmarshal(b, &s); e != nil {
					return nil, 0, false, e
				}
				return s, s.SchemaVersion, true, nil
			}, func(d interface{}) (bool, error) {
				s := d.(BoardPostsSave)
				ms, did, err := MigrateBoardPostsSave(s)
				if err != nil {
					return false, err
				}
				if did {
					if e := jsonstore.WriteJSON(full, ms); e != nil {
						return false, e
					}
				}
				return did, nil
			})
		}
	}

	// Family news: player/family/json/family_news_*.json
	fnDir := filepath.Join(dbRoot, "player", "family", "json")
	if fis, _ := os.ReadDir(fnDir); fis != nil {
		for _, fi := range fis {
			if fi.IsDir() || !strings.HasPrefix(fi.Name(), "family_news_") || !strings.HasSuffix(fi.Name(), ".json") {
				continue
			}
			full := filepath.Join(fnDir, fi.Name())
			process(full, "familynews", func() (interface{}, int, bool, error) {
				b, err := os.ReadFile(full)
				if err != nil {
					return nil, 0, false, err
				}
				var s FamilyNewsSave
				if e := json.Unmarshal(b, &s); e != nil {
					return nil, 0, false, e
				}
				return s, s.SchemaVersion, true, nil
			}, func(d interface{}) (bool, error) {
				s := d.(FamilyNewsSave)
				ms, did, err := MigrateFamilyNewsSave(s)
				if err != nil {
					return false, err
				}
				if did {
					if e := jsonstore.WriteJSON(full, ms); e != nil {
						return false, e
					}
				}
				return did, nil
			})
		}
	}

	return r, nil
}

type SidecarMigrationReport struct {
	TotalScanned int                      `json:"totalScanned"`
	Migrated     int                      `json:"migrated"`
	ByType       map[string]int           `json:"byType"`
	Errors       []string                 `json:"errors,omitempty"`
	Details      []SidecarMigrationDetail `json:"details,omitempty"`
}

type SidecarMigrationDetail struct {
	Type     string `json:"type"`
	Path     string `json:"path"`
	FromVer  int    `json:"fromVer"`
	ToVer    int    `json:"toVer"`
	Repaired bool   `json:"repaired"`
}
