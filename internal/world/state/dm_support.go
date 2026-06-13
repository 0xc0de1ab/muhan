package state

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/0xc0de1ab/muhan/internal/migrate/protomap"
	"github.com/0xc0de1ab/muhan/internal/migrate/roommap"
	"github.com/0xc0de1ab/muhan/internal/persist/cbin"
	"github.com/0xc0de1ab/muhan/internal/persist/jsonstore"
	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	legacyFingerInvalidHostMessage   = "\n\rInvalid host address\n\r"
	legacyFingerUnableConnectMessage = "\n\rUnable to connect to finger server.\n\r"
	legacyFingerTimeoutMessage       = "\n\rFinger request timed out.\n\r"
	legacyFingerTruncatedMessage     = "\n\rFinger output truncated.\n\r"
)

var (
	legacyFingerPort        = "79"
	legacyFingerDialTimeout = 3 * time.Second
	legacyFingerReadTimeout = 5 * time.Second
	legacyFingerOutputLimit = int64(64 * 1024)
)

type SavedRoomState struct {
	RoomID         model.RoomID             `json:"roomId"`
	FloorObjectIDs []model.ObjectInstanceID `json:"floorObjectIds,omitempty"`
	Properties     map[string]string        `json:"properties,omitempty"`
	Objects        []model.ObjectInstance   `json:"objects,omitempty"`
}

func (w *World) ResaveRoom(roomID model.RoomID) error {
	return w.ResaveRoomWithOptions(roomID, false)
}

func (w *World) ResaveRoomWithOptions(roomID model.RoomID, permOnly bool) error {
	if w == nil {
		return fmt.Errorf("resave room: world state is nil")
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	room, ok := w.rooms[roomID]
	if !ok {
		return nil
	}

	if w.dbRoot == "" {
		return fmt.Errorf("resave room %s: dbRoot is not set", roomID)
	}

	dir := filepath.Join(w.dbRoot, "rooms", "json")
	path := filepath.Join(dir, legacyRoomJSONFileName(roomID))

	var floorObjectIDs []model.ObjectInstanceID
	if room.Objects.ObjectIDs != nil {
		for _, id := range room.Objects.ObjectIDs {
			if permOnly {
				obj, exists := w.objects[id]
				if !exists {
					continue
				}
				if !w.resaveRoomObjectIsPermOnlyLocked(obj) {
					continue
				}
			}
			floorObjectIDs = append(floorObjectIDs, id)
		}
	} else {
		floorObjectIDs = []model.ObjectInstanceID{}
	}

	var objects []model.ObjectInstance
	visited := make(map[model.ObjectInstanceID]bool)
	var gather func(id model.ObjectInstanceID)
	gather = func(id model.ObjectInstanceID) {
		if visited[id] {
			return
		}
		visited[id] = true
		obj, ok := w.objects[id]
		if !ok {
			return
		}
		objects = append(objects, obj)
		for _, childID := range obj.Contents.ObjectIDs {
			gather(childID)
		}
	}

	for _, id := range floorObjectIDs {
		gather(id)
	}

	saved := SavedRoomState{
		RoomID:         roomID,
		FloorObjectIDs: floorObjectIDs,
		Properties:     maps.Clone(room.Properties),
		Objects:        objects,
	}

	if err := jsonstore.WriteJSON(path, saved); err != nil {
		return fmt.Errorf("write room json file %q: %w", path, err)
	}

	return nil
}

func (w *World) resaveRoomObjectIsPermOnlyLocked(object model.ObjectInstance) bool {
	if hasAnyNormalizedFlag(object.Metadata.Tags, "OTEMPP", "OPERM2") ||
		resaveRoomPropertiesHaveAnyFlag(object.Properties, "OTEMPP", "OPERM2") {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := w.prototypes[object.PrototypeID]
	if !ok {
		return false
	}
	return hasAnyNormalizedFlag(proto.Metadata.Tags, "OTEMPP", "OPERM2") ||
		resaveRoomPropertiesHaveAnyFlag(proto.Properties, "OTEMPP", "OPERM2")
}

func resaveRoomPropertiesHaveAnyFlag(properties map[string]string, names ...string) bool {
	if len(properties) == 0 {
		return false
	}
	targets := normalizedFlagSet(names...)
	for key, value := range properties {
		if _, ok := targets[normalizeFlagName(key)]; ok && propertyFlagEnabled(value) {
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

func (w *World) ReloadRoom(roomID model.RoomID) error {
	if w == nil {
		return fmt.Errorf("reload room: world state is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	oldRoom, loaded := w.rooms[roomID]
	if !loaded {
		return nil
	}

	if w.dbRoot == "" {
		return fmt.Errorf("reload room %s: dbRoot is not set", roomID)
	}

	path := legacyRoomDataPath(w.dbRoot, roomID)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read room file %q: %w", path, err)
	}

	bundle, err := roommap.MapRoomFileBundle(path, data)
	if err != nil {
		return fmt.Errorf("map room bundle %q: %w", path, err)
	}

	jsonDir := filepath.Join(w.dbRoot, "rooms", "json")
	jsonPath := filepath.Join(jsonDir, legacyRoomJSONFileName(roomID))
	if jsonData, err := os.ReadFile(jsonPath); err == nil {
		var saved SavedRoomState
		if err := json.Unmarshal(jsonData, &saved); err == nil {
			bundle.Room.Objects.ObjectIDs = saved.FloorObjectIDs
			if len(saved.Properties) > 0 {
				if bundle.Room.Properties == nil {
					bundle.Room.Properties = make(map[string]string, len(saved.Properties))
				}
				for key, value := range saved.Properties {
					bundle.Room.Properties[key] = value
				}
			}
			for _, obj := range saved.Objects {
				w.objects[obj.ID] = obj
			}
		}
	}

	bundle.Room.PlayerIDs = append([]model.PlayerID(nil), oldRoom.PlayerIDs...)
	if len(bundle.Room.CreatureIDs) == 0 {
		bundle.Room.CreatureIDs = append([]model.CreatureID(nil), oldRoom.CreatureIDs...)
	}
	if len(bundle.Room.Objects.ObjectIDs) == 0 {
		bundle.Room.Objects = cloneObjectRefList(oldRoom.Objects)
	}

	w.rooms[bundle.Room.ID] = bundle.Room
	for _, cr := range bundle.Creatures {
		w.creatures[cr.ID] = cr
	}
	for _, obj := range bundle.Objects {
		if _, ok := w.objects[obj.ID]; !ok {
			w.objects[obj.ID] = obj
		}
	}

	return nil
}

func (w *World) LoadRoom(roomID model.RoomID) error {
	if w == nil {
		return fmt.Errorf("load room: world state is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	if _, ok := w.rooms[roomID]; ok {
		return nil
	}

	if w.dbRoot == "" {
		return fmt.Errorf("load room %s: dbRoot is not set", roomID)
	}

	path := legacyRoomDataPath(w.dbRoot, roomID)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read room file %q: %w", path, err)
	}

	bundle, err := roommap.MapRoomFileBundle(path, data)
	if err != nil {
		return fmt.Errorf("map room bundle %q: %w", path, err)
	}

	jsonDir := filepath.Join(w.dbRoot, "rooms", "json")
	jsonPath := filepath.Join(jsonDir, legacyRoomJSONFileName(roomID))
	if jsonData, err := os.ReadFile(jsonPath); err == nil {
		var saved SavedRoomState
		if err := json.Unmarshal(jsonData, &saved); err == nil {
			bundle.Room.Objects.ObjectIDs = saved.FloorObjectIDs
			if len(saved.Properties) > 0 {
				if bundle.Room.Properties == nil {
					bundle.Room.Properties = make(map[string]string, len(saved.Properties))
				}
				for key, value := range saved.Properties {
					bundle.Room.Properties[key] = value
				}
			}
			for _, obj := range saved.Objects {
				w.objects[obj.ID] = obj
			}
		}
	}

	w.rooms[bundle.Room.ID] = bundle.Room
	for _, cr := range bundle.Creatures {
		w.creatures[cr.ID] = cr
	}
	for _, obj := range bundle.Objects {
		if _, ok := w.objects[obj.ID]; !ok {
			w.objects[obj.ID] = obj
		}
	}

	return nil
}

func legacyRoomDataPath(root string, roomID model.RoomID) string {
	numericPart := strings.TrimPrefix(string(roomID), "room:")
	num, err := strconv.Atoi(numericPart)
	if err != nil {
		return filepath.Join(root, "rooms", "r"+numericPart)
	}
	return filepath.Join(root, "rooms", fmt.Sprintf("r%02d", num/1000), fmt.Sprintf("r%05d", num))
}

func legacyRoomJSONFileName(roomID model.RoomID) string {
	numericPart := strings.TrimPrefix(string(roomID), "room:")
	num, err := strconv.Atoi(numericPart)
	if err != nil {
		return "r" + numericPart + ".json"
	}
	return fmt.Sprintf("r%05d.json", num)
}

func (w *World) CreateObjectFromPrototype(protoID model.PrototypeID, creatureID model.CreatureID) (model.ObjectInstanceID, error) {
	if w == nil {
		return "", fmt.Errorf("create object from prototype %q: world state is nil", protoID)
	}
	if protoID.IsZero() {
		return "", fmt.Errorf("create object: prototype id is required")
	}
	if creatureID.IsZero() {
		return "", fmt.Errorf("create object %q: creature id is required", protoID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	proto, ok := w.prototypes[protoID]
	if !ok {
		return "", fmt.Errorf("create object from prototype %q: prototype not found", protoID)
	}
	if _, ok := w.creatures[creatureID]; !ok {
		return "", fmt.Errorf("create object from prototype %q: creature %q not found", protoID, creatureID)
	}

	sourceID := model.ObjectInstanceID("object:" + string(protoID))
	cloneID := w.nextObjectCloneIDLocked(sourceID)

	inst := model.ObjectInstance{
		ID:                  cloneID,
		PrototypeID:         protoID,
		Location:            model.ObjectLocation{CreatureID: creatureID, Slot: "inventory"},
		DisplayNameOverride: proto.DisplayName,
		Properties:          make(map[string]string),
	}
	for k, v := range proto.Properties {
		inst.Properties[k] = v
	}
	inst.Metadata.Tags = make([]string, len(proto.Metadata.Tags))
	copy(inst.Metadata.Tags, proto.Metadata.Tags)

	if err := inst.Validate(); err != nil {
		return "", fmt.Errorf("create object from prototype %q: %w", protoID, err)
	}

	w.objects[inst.ID] = inst
	w.addObjectToHolderLocked(inst.ID, inst.Location)
	return inst.ID, nil
}

func (w *World) CreateObjectInstanceFromPrototype(protoID model.PrototypeID, creatureID model.CreatureID) (model.ObjectInstance, error) {
	if w == nil {
		return model.ObjectInstance{}, fmt.Errorf("create object: world state is nil")
	}
	if protoID.IsZero() {
		return model.ObjectInstance{}, fmt.Errorf("create object: prototype id is required")
	}
	if creatureID.IsZero() {
		return model.ObjectInstance{}, fmt.Errorf("create object: creature id is required")
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	proto, ok := w.prototypes[protoID]
	if !ok {
		return model.ObjectInstance{}, fmt.Errorf("prototype %q not found", protoID)
	}

	creature, ok := w.creatures[creatureID]
	if !ok {
		return model.ObjectInstance{}, fmt.Errorf("creature %q not found", creatureID)
	}

	// Create unique object instance ID
	objectID := w.nextObjectCloneIDLocked(model.ObjectInstanceID(protoID))

	instance := model.ObjectInstance{
		ID:          objectID,
		PrototypeID: protoID,
		Location: model.ObjectLocation{
			CreatureID: creatureID,
			Slot:       "inventory",
		},
		Quantity:   1,
		Properties: make(map[string]string),
	}

	// Copy properties from prototype
	if proto.Properties != nil {
		for k, v := range proto.Properties {
			instance.Properties[k] = v
		}
	}

	// Copy metadata tags
	if len(proto.Metadata.Tags) > 0 {
		instance.Metadata.Tags = make([]string, len(proto.Metadata.Tags))
		copy(instance.Metadata.Tags, proto.Metadata.Tags)
	}

	w.applyRandomEnchantIfNeededLocked(&instance)

	if err := instance.Validate(); err != nil {
		return model.ObjectInstance{}, fmt.Errorf("invalid object instance: %w", err)
	}

	// Add to w.objects
	w.objects[objectID] = instance

	// Update creature inventory
	w.addObjectToHolderLocked(objectID, instance.Location)
	if !creature.PlayerID.IsZero() {
		w.MarkPlayerDirty(creature.PlayerID)
	}

	return cloneObject(instance), nil
}

func (w *World) DestroyCreature(creatureID model.CreatureID) error {
	if w == nil {
		return fmt.Errorf("destroy creature: world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	creature, ok := w.creatures[creatureID]
	if !ok {
		return nil
	}

	// Delete all carried objects
	for _, objID := range carriedCreatureObjectIDs(creature) {
		w.deleteObjectTreeLocked(objID, map[model.ObjectInstanceID]struct{}{})
	}

	// Remove from its room
	if !creature.RoomID.IsZero() {
		if room, ok := w.rooms[creature.RoomID]; ok {
			room.CreatureIDs = removeID(room.CreatureIDs, creatureID)
			w.rooms[creature.RoomID] = room
		}
	}

	w.pruneCharmReferencesLocked(creature)
	delete(w.creatures, creatureID)
	delete(w.monsterDamage, creatureID)
	return nil
}

func (w *World) DestroyObject(objectID model.ObjectInstanceID) error {
	if w == nil {
		return fmt.Errorf("destroy object: world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	w.deleteObjectTreeLocked(objectID, map[model.ObjectInstanceID]struct{}{})
	return nil
}

func (w *World) ResaveAllRooms(permOnly bool) error {
	if w == nil {
		return fmt.Errorf("resave all rooms: world state is nil")
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	var roomIDs []model.RoomID
	for id := range w.rooms {
		roomIDs = append(roomIDs, id)
	}
	w.rUnlockDomains(true, true, true, true, true, true, true)

	for _, roomID := range roomIDs {
		if err := w.ResaveRoomWithOptions(roomID, permOnly); err != nil {
			return err
		}
	}
	return nil
}

func (w *World) FlushCrtObj() error {
	if w == nil {
		return fmt.Errorf("flush crt obj: world state is nil")
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	dbRoot := strings.TrimSpace(w.dbRoot)
	w.rUnlockDomains(true, true, true, true, true, true, true)
	if dbRoot == "" {
		return fmt.Errorf("flush crt obj: dbRoot is not set")
	}

	snapshot, err := protomap.Build(protomap.Options{Root: dbRoot})
	if err != nil {
		return fmt.Errorf("flush crt obj: load prototypes: %w", err)
	}
	if len(snapshot.Errors) > 0 {
		return fmt.Errorf("flush crt obj: %s", protomapErrorSummary(snapshot.Errors))
	}

	objectPrototypes := make(map[model.PrototypeID]model.ObjectPrototype, len(snapshot.ObjectPrototypes))
	for _, proto := range snapshot.ObjectPrototypes {
		objectPrototypes[proto.ID] = cloneObjectPrototype(proto)
	}
	creaturePrototypes := make(map[model.CreatureID]model.Creature, len(snapshot.CreaturePrototypes))
	for _, proto := range snapshot.CreaturePrototypes {
		creature := creatureFromProtoMapRecord(proto)
		creaturePrototypes[creature.ID] = creature
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	for id, proto := range w.prototypes {
		if proto.Metadata.LegacyKind == "objectPrototype" {
			delete(w.prototypes, id)
		}
	}
	for id, proto := range objectPrototypes {
		w.prototypes[id] = proto
	}
	for id, creature := range w.creatures {
		if isLegacyCreaturePrototypeEntry(id, creature) {
			delete(w.creatures, id)
		}
	}
	for id, creature := range creaturePrototypes {
		w.creatures[id] = cloneCreature(creature)
	}
	return nil
}

func protomapErrorSummary(errors []protomap.Finding) string {
	parts := make([]string, 0, min(len(errors), 3))
	for _, finding := range errors {
		location := strings.TrimSpace(finding.Path)
		message := strings.TrimSpace(finding.Message)
		if location != "" && message != "" {
			parts = append(parts, location+": "+message)
		} else if message != "" {
			parts = append(parts, message)
		} else if location != "" {
			parts = append(parts, location)
		}
		if len(parts) == 3 {
			break
		}
	}
	if len(errors) > len(parts) {
		parts = append(parts, fmt.Sprintf("and %d more", len(errors)-len(parts)))
	}
	if len(parts) == 0 {
		return "prototype load failed"
	}
	return strings.Join(parts, "; ")
}

func creatureFromProtoMapRecord(proto protomap.CreaturePrototypeRecord) model.Creature {
	properties := map[string]string{}
	for key, value := range proto.Properties {
		properties[key] = value
	}
	if proto.Talk != "" {
		properties["legacyTalk"] = proto.Talk
	}
	if len(proto.Keywords) > 0 {
		properties["keywords"] = strings.Join(proto.Keywords, "\n")
	}
	if len(properties) == 0 {
		properties = nil
	}
	stats := map[string]int{}
	for key, value := range proto.Stats {
		stats[key] = value
	}
	if len(stats) == 0 {
		stats = nil
	}
	return model.Creature{
		ID:          proto.ID,
		Kind:        proto.Kind,
		DisplayName: proto.DisplayName,
		Description: proto.Description,
		Level:       proto.Level,
		Stats:       stats,
		Properties:  properties,
		Metadata:    proto.Metadata,
	}
}

func isLegacyCreaturePrototypeEntry(id model.CreatureID, creature model.Creature) bool {
	if creature.Metadata.LegacyKind != "creaturePrototype" {
		return false
	}
	parts := strings.Split(string(id), ":")
	if len(parts) != 3 || parts[0] != "creature" || len(parts[1]) != 3 || parts[1][0] != 'm' {
		return false
	}
	for _, ch := range parts[1][1:] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	if _, err := strconv.Atoi(parts[2]); err != nil {
		return false
	}
	return true
}

func (w *World) SetShutdown(seconds int, now bool) error {
	if w == nil {
		return fmt.Errorf("set shutdown: world state is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	w.shutdownLTime = time.Now().Unix()
	if now {
		w.shutdownInterval = 1
	} else {
		w.shutdownInterval = int64(seconds)
	}
	return nil
}

func (w *World) FindPlayerByName(name string) (model.Player, bool) {
	if w == nil {
		return model.Player{}, false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	for _, player := range w.players {
		if strings.EqualFold(player.DisplayName, name) ||
			strings.EqualFold(string(player.ID), name) ||
			strings.EqualFold(strings.TrimPrefix(string(player.ID), "player:"), name) {
			return player, true
		}
	}
	return model.Player{}, false
}

func (w *World) ForcePlayerCommand(playerID model.PlayerID, cmd string) error {
	return fmt.Errorf("force player %s: state world has no session dispatcher", playerID)
}

func (w *World) nextCreatureCloneIDLocked(sourceID model.CreatureID) model.CreatureID {
	base := strings.TrimSpace(string(sourceID))
	if base == "" {
		base = "creature"
	}
	for i := 1; ; i++ {
		id := model.CreatureID(fmt.Sprintf("%s:clone:%06d", base, i))
		if _, exists := w.creatures[id]; !exists {
			return id
		}
	}
}

func (w *World) CreaturePrototype(id model.CreatureID) (model.Creature, bool) {
	if w == nil {
		return model.Creature{}, false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	proto, ok := w.creatures[id]
	if !ok {
		return model.Creature{}, false
	}
	return cloneCreature(proto), true
}

func (w *World) SpawnCreature(protoID model.CreatureID, roomID model.RoomID, carryItems bool) (model.CreatureID, error) {
	if w == nil {
		return "", fmt.Errorf("spawn creature: world state is nil")
	}
	if protoID.IsZero() {
		return "", fmt.Errorf("spawn creature: prototype id is required")
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	return w.spawnCreatureLocked(protoID, roomID, carryItems, nil)
}

func (w *World) spawnCreatureLocked(protoID model.CreatureID, roomID model.RoomID, carryItems bool, addTags []string) (model.CreatureID, error) {
	proto, ok := w.creatures[protoID]
	if !ok {
		return "", fmt.Errorf("prototype creature %q not found", protoID)
	}

	clone := cloneCreature(proto)
	cloneID := w.nextCreatureCloneIDLocked(protoID)
	clone.ID = cloneID
	clone.RoomID = roomID
	clone.Inventory = model.ObjectRefList{}
	clone.Equipment = make(map[string]model.ObjectInstanceID)
	if len(addTags) > 0 {
		clone.Metadata.Tags = addMetadataTags(clone.Metadata.Tags, addTags)
	}

	now := time.Now()
	nowUnix := now.Unix()
	if w.cooldowns == nil {
		w.cooldowns = make(map[model.CreatureID]map[string]int64)
	}
	if w.cooldowns[cloneID] == nil {
		w.cooldowns[cloneID] = make(map[string]int64)
	}
	w.cooldowns[cloneID]["attack"] = nowUnix
	w.cooldowns[cloneID]["scavenge"] = nowUnix
	w.cooldowns[cloneID]["wander"] = nowUnix

	dex := 0
	if clone.Stats != nil {
		dex = clone.Stats["dexterity"]
	} else {
		clone.Stats = make(map[string]int)
	}
	interval := 2
	if dex < 20 {
		interval = 3
	}
	clone.Stats["attackInterval"] = interval

	if carryItems {
		jVal := rand.Intn(100) + 1
		var numItems int
		if jVal < 90 {
			numItems = 1
		} else if jVal < 96 {
			numItems = 2
		} else {
			numItems = 3
		}

		for k := 0; k < numItems; k++ {
			mIndex := rand.Intn(10)
			if hasCreatureFlagLocked(clone, "MNRGLD") {
				mIndex = rand.Intn(50)
				if mIndex > 9 {
					continue
				}
			}
			carryNum := clone.Stats[fmt.Sprintf("carry[%d]", mIndex)]
			if carryNum > 0 {
				carryProtoID := legacyCarryObjectPrototypeID(carryNum)
				protoObj, ok := w.prototypes[carryProtoID]
				if ok {
					objID := w.nextObjectCloneIDLocked(model.ObjectInstanceID(carryProtoID))
					instance := model.ObjectInstance{
						ID:          objID,
						PrototypeID: carryProtoID,
						Location: model.ObjectLocation{
							CreatureID: cloneID,
							Slot:       "inventory",
						},
						Quantity:   1,
						Properties: make(map[string]string),
					}

					if protoObj.Properties != nil {
						for pk, pv := range protoObj.Properties {
							instance.Properties[pk] = pv
						}
					}
					if len(protoObj.Metadata.Tags) > 0 {
						instance.Metadata.Tags = make([]string, len(protoObj.Metadata.Tags))
						copy(instance.Metadata.Tags, protoObj.Metadata.Tags)
					}

					w.applyRandomEnchantIfNeededLocked(&instance)

					val := 0
					if v, ok := instance.Properties["value"]; ok {
						if parsed, err := strconv.Atoi(v); err == nil {
							val = parsed
						}
					}
					if val > 0 {
						minVal := (val * 9) / 10
						maxVal := (val * 11) / 10
						var randVal int
						if maxVal > minVal {
							randVal = rand.Intn(maxVal-minVal+1) + minVal
						} else {
							randVal = minVal
						}
						instance.Properties["value"] = strconv.Itoa(randVal)
					}

					if err := instance.Validate(); err == nil {
						w.objects[objID] = instance
						clone.Inventory.ObjectIDs = w.insertObjectIDLegacySortedLocked(clone.Inventory.ObjectIDs, objID)
					}
				}
			}
		}
	}

	if !hasCreatureFlagLocked(clone, "MNRGLD") && clone.Stats["gold"] > 0 {
		gold := clone.Stats["gold"]
		minGold := gold / 10
		maxGold := gold
		var randGold int
		if maxGold > minGold {
			randGold = rand.Intn(maxGold-minGold+1) + minGold
		} else {
			randGold = minGold
		}
		clone.Stats["gold"] = randGold
	}

	if clone.Stats == nil {
		clone.Stats = make(map[string]int)
	}
	w.creatures[cloneID] = clone

	room, ok := w.rooms[roomID]
	if !ok {
		return "", fmt.Errorf("room %q not found", roomID)
	}
	room.CreatureIDs = w.insertCreatureIDLegacySortedLocked(room.CreatureIDs, cloneID)
	w.rooms[roomID] = room

	return cloneID, nil
}

func legacyCarryObjectPrototypeID(number int) model.PrototypeID {
	return model.PrototypeID(fmt.Sprintf("object:o%02d:%d", number/100, number%100))
}

func hasCreatureFlagLocked(creature model.Creature, name string) bool {
	targets := make(map[string]struct{})
	for _, expanded := range ExpandFlagNames(name) {
		targets[expanded] = struct{}{}
	}
	if len(targets) == 0 {
		return false
	}
	for _, tag := range creature.Metadata.Tags {
		if _, ok := targets[normalizeFlagName(tag)]; ok {
			return true
		}
	}
	for k, v := range creature.Properties {
		if _, ok := targets[normalizeFlagName(k)]; ok {
			if v == "true" || v == "1" || strings.ToLower(v) == "yes" {
				return true
			}
		}
	}
	if creature.Stats != nil {
		for k, v := range creature.Stats {
			if _, ok := targets[normalizeFlagName(k)]; ok && v != 0 {
				return true
			}
		}
	}
	return false
}

func (w *World) FindCreatureByName(roomID model.RoomID, name string, count int) (model.Creature, bool) {
	if w == nil {
		return model.Creature{}, false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	room, ok := w.rooms[roomID]
	if !ok {
		return model.Creature{}, false
	}

	nameLower := strings.ToLower(strings.TrimSpace(name))
	if nameLower == "" {
		return model.Creature{}, false
	}
	if count < 1 {
		count = 1
	}

	seen := 0
	for _, crtID := range room.CreatureIDs {
		c, ok := w.creatures[crtID]
		if !ok {
			continue
		}
		if creatureMatchesNameLocked(c, nameLower) {
			seen++
			if seen == count {
				return cloneCreature(c), true
			}
		}
	}
	return model.Creature{}, false
}

func creatureMatchesNameLocked(c model.Creature, nameLower string) bool {
	terms := []string{c.DisplayName, string(c.ID)}
	keys := []string{"name", "key[0]", "key[1]", "key[2]", "key/1", "key/2", "key/3"}
	for _, key := range keys {
		if val, ok := c.Properties[key]; ok {
			terms = append(terms, val)
		}
	}
	for _, term := range terms {
		termClean := strings.ToLower(strings.TrimSpace(term))
		if termClean == "" {
			continue
		}
		if strings.HasPrefix(termClean, nameLower) {
			return true
		}
		for _, word := range strings.Fields(termClean) {
			if strings.HasPrefix(word, nameLower) {
				return true
			}
		}
	}
	return false
}

func (w *World) FindCreatureByNameGlobal(name string) (model.Creature, bool) {
	if w == nil {
		return model.Creature{}, false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	nameLower := strings.ToLower(strings.TrimSpace(name))
	if nameLower == "" {
		return model.Creature{}, false
	}

	for _, player := range w.players {
		if strings.EqualFold(player.DisplayName, nameLower) || strings.EqualFold(string(player.ID), nameLower) || strings.EqualFold(strings.TrimPrefix(string(player.ID), "player:"), nameLower) {
			if c, ok := w.creatures[player.CreatureID]; ok {
				return cloneCreature(c), true
			}
		}
	}

	for _, c := range w.creatures {
		if creatureMatchesNameLocked(c, nameLower) {
			return cloneCreature(c), true
		}
	}

	return model.Creature{}, false
}

func (w *World) FindObjectByName(creatureID model.CreatureID, roomID model.RoomID, name string, count int) (model.ObjectInstance, bool) {
	if w == nil {
		return model.ObjectInstance{}, false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	nameLower := strings.ToLower(strings.TrimSpace(name))
	if nameLower == "" {
		return model.ObjectInstance{}, false
	}
	if count < 1 {
		count = 1
	}

	seen := 0

	if !creatureID.IsZero() {
		if c, ok := w.creatures[creatureID]; ok {
			for _, objID := range c.Inventory.ObjectIDs {
				obj, ok := w.objects[objID]
				if !ok {
					continue
				}
				if w.objectMatchesNameLocked(obj, nameLower) {
					seen++
					if seen == count {
						return cloneObject(obj), true
					}
				}
			}

			for _, objID := range c.Equipment {
				obj, ok := w.objects[objID]
				if !ok {
					continue
				}
				if w.objectMatchesNameLocked(obj, nameLower) {
					seen++
					if seen == count {
						return cloneObject(obj), true
					}
				}
			}
		}
	}

	if !roomID.IsZero() {
		if room, ok := w.rooms[roomID]; ok {
			for _, objID := range room.Objects.ObjectIDs {
				obj, ok := w.objects[objID]
				if !ok {
					continue
				}
				if w.objectMatchesNameLocked(obj, nameLower) {
					seen++
					if seen == count {
						return cloneObject(obj), true
					}
				}
			}
		}
	}

	return model.ObjectInstance{}, false
}

func (w *World) objectMatchesNameLocked(obj model.ObjectInstance, nameLower string) bool {
	terms := []string{obj.DisplayNameOverride, string(obj.ID)}
	if !obj.PrototypeID.IsZero() {
		if proto, ok := w.prototypes[obj.PrototypeID]; ok {
			terms = append(terms, proto.DisplayName)
			terms = append(terms, proto.Keywords...)
			keys := []string{"name", "key[0]", "key[1]", "key[2]", "key/1", "key/2", "key/3"}
			for _, key := range keys {
				if val, ok := proto.Properties[key]; ok {
					terms = append(terms, val)
				}
			}
		}
	}
	keys := []string{"name", "key[0]", "key[1]", "key[2]", "key/1", "key/2", "key/3"}
	for _, key := range keys {
		if val, ok := obj.Properties[key]; ok {
			terms = append(terms, val)
		}
	}
	for _, term := range terms {
		termClean := strings.ToLower(strings.TrimSpace(term))
		if termClean == "" {
			continue
		}
		if strings.HasPrefix(termClean, nameLower) {
			return true
		}
		for _, word := range strings.Fields(termClean) {
			if strings.HasPrefix(word, nameLower) {
				return true
			}
		}
	}
	return false
}

type LockoutMode int

const (
	LockoutAllow LockoutMode = iota
	LockoutDeny
	LockoutPassword
)

type LockoutEntry struct {
	Address  string
	Password string
}

func (w *World) LoadLockouts() error {
	if w == nil {
		return fmt.Errorf("load lockouts: world state is nil")
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	dbRoot := strings.TrimSpace(w.dbRoot)
	w.rUnlockDomains(true, true, true, true, true, true, true)
	if dbRoot == "" {
		return fmt.Errorf("load lockouts: dbRoot is not set")
	}

	path := filepath.Join(dbRoot, "log", "lockout")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			w.lockDomains(true, true, true, true, true, true, true)
			w.lockouts = nil
			w.unlockDomains(true, true, true, true, true, true, true)
			return nil
		}
		return fmt.Errorf("load lockouts: read %s: %w", path, err)
	}

	fields := strings.Fields(string(data))
	entries := make([]LockoutEntry, 0, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		entry := LockoutEntry{
			Address:  fields[i],
			Password: fields[i+1],
		}
		if entry.Password == "-" {
			entry.Password = ""
		}
		entries = append(entries, entry)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	w.lockouts = entries
	w.unlockDomains(true, true, true, true, true, true, true)
	return nil
}

func (w *World) CheckLockout(address string) (LockoutMode, string) {
	if w == nil {
		return LockoutAllow, ""
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return LockoutAllow, ""
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	for _, entry := range w.lockouts {
		if !legacyAddressEqual(entry.Address, address) {
			continue
		}
		if entry.Password != "" {
			return LockoutPassword, entry.Password
		}
		return LockoutDeny, ""
	}
	return LockoutAllow, ""
}

func (w *World) Lockouts() []LockoutEntry {
	if w == nil {
		return nil
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	out := make([]LockoutEntry, len(w.lockouts))
	copy(out, w.lockouts)
	return out
}

func legacyAddressEqual(pattern, address string) bool {
	pattern = strings.TrimSpace(pattern)
	address = strings.TrimSpace(address)
	for len(pattern) > 0 && len(address) > 0 {
		if pattern[0] == '*' {
			for len(address) > 0 && address[0] != '.' {
				address = address[1:]
			}
			pattern = pattern[1:]
			continue
		}
		if pattern[0] != address[0] {
			return false
		}
		pattern = pattern[1:]
		address = address[1:]
	}
	return pattern == "" && address == ""
}

func (w *World) SetSpy(spyPlayerID, targetPlayerID model.PlayerID) error {
	if w == nil {
		return fmt.Errorf("set spy: world state is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	if w.spies == nil {
		w.spies = make(map[model.PlayerID]model.PlayerID)
	}
	w.spies[spyPlayerID] = targetPlayerID
	return nil
}

func (w *World) ClearSpy(spyPlayerID model.PlayerID) error {
	if w == nil {
		return fmt.Errorf("clear spy: world state is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	if w.spies != nil {
		delete(w.spies, spyPlayerID)
	}
	return nil
}

func (w *World) IsSpying(spyPlayerID model.PlayerID) (model.PlayerID, bool) {
	if w == nil {
		return "", false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	if w.spies == nil {
		return "", false
	}
	target, ok := w.spies[spyPlayerID]
	return target, ok
}

func (w *World) IsBeingSpiedOn(targetPlayerID model.PlayerID) (model.PlayerID, bool) {
	if w == nil {
		return "", false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	if w.spies == nil {
		return "", false
	}
	for spy, target := range w.spies {
		if target == targetPlayerID {
			return spy, true
		}
	}
	return "", false
}

func (w *World) ReadLogFile(name string) (string, error) {
	name, err := safeLogFileName(name)
	if err != nil {
		return "", err
	}
	path := filepath.Join("log", name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text, err := legacykr.ValidUTF8OrDecodeContext(legacykr.Context{
		Path:  path,
		Field: "log file",
	}, data)
	if err != nil {
		return "", err
	}
	return text, nil
}

func (w *World) DeleteLogFile(name string) error {
	name, err := safeLogFileName(name)
	if err != nil {
		return err
	}
	path := filepath.Join("log", name)
	return os.Remove(path)
}

func safeLogFileName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("log file name is empty")
	}
	if strings.ContainsRune(name, '\x00') {
		return "", fmt.Errorf("log file name %q contains NUL", name)
	}
	if filepath.IsAbs(name) || name == "." || name == ".." || filepath.Clean(name) != name {
		return "", fmt.Errorf("unsafe log file name %q", name)
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '_', '-', '.':
			continue
		}
		return "", fmt.Errorf("unsafe log file name %q", name)
	}
	return name, nil
}

// CreateRoom creates a new room in-memory.
func (w *World) CreateRoom(roomID model.RoomID) error {
	if w == nil {
		return fmt.Errorf("create room: world state is nil")
	}
	roomID = canonicalStateRoomID(roomID)
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	if _, ok := w.rooms[roomID]; ok {
		return fmt.Errorf("room %s already exists", roomID)
	}

	displayName := legacyCreatedRoomDisplayName(roomID)
	if w.dbRoot != "" {
		if err := writeLegacyEmptyRoomFile(w.dbRoot, roomID, displayName); err != nil {
			return err
		}
	}

	w.rooms[roomID] = model.Room{
		ID:          roomID,
		DisplayName: displayName,
	}
	return nil
}

func canonicalStateRoomID(roomID model.RoomID) model.RoomID {
	raw := strings.TrimSpace(string(roomID))
	if raw == "" {
		return roomID
	}
	numeric := strings.TrimPrefix(raw, "room:")
	number, err := strconv.Atoi(numeric)
	if err != nil || number < 0 {
		return roomID
	}
	return model.RoomID(fmt.Sprintf("room:%05d", number))
}

func legacyCreatedRoomDisplayName(roomID model.RoomID) string {
	number, ok := legacyRoomNumberFromID(roomID)
	if !ok {
		return "Room #" + strings.TrimPrefix(string(roomID), "room:")
	}
	return fmt.Sprintf("Room #%d", number)
}

func legacyRoomNumberFromID(roomID model.RoomID) (int, bool) {
	numeric := strings.TrimPrefix(strings.TrimSpace(string(roomID)), "room:")
	number, err := strconv.Atoi(numeric)
	if err != nil {
		return 0, false
	}
	return number, true
}

func writeLegacyEmptyRoomFile(root string, roomID model.RoomID, displayName string) error {
	number, ok := legacyRoomNumberFromID(roomID)
	if !ok {
		return fmt.Errorf("create room %s: legacy room number required", roomID)
	}
	if number < -32768 || number > 32767 {
		return fmt.Errorf("create room %s: legacy room number out of int16 range", roomID)
	}

	path := legacyRoomDataPath(root, roomID)
	if err := os.MkdirAll(filepath.Dir(path), 0770); err != nil {
		return fmt.Errorf("create room directory %q: %w", filepath.Dir(path), err)
	}

	data := make([]byte, cbin.RoomSize+6*4)
	binary.LittleEndian.PutUint16(data[0:2], uint16(int16(number)))
	copy(data[2:2+80], []byte(displayName))

	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0660)
	if err != nil {
		return fmt.Errorf("create legacy room file %q: %w", path, err)
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write legacy room file %q: %w", path, err)
	}
	return nil
}

func (w *World) UpdateRoomProperty(roomID model.RoomID, key, val string) error {
	if w == nil {
		return fmt.Errorf("update room property: world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	room, ok := w.rooms[roomID]
	if !ok {
		return fmt.Errorf("room %s not found", roomID)
	}

	if room.Properties == nil {
		room.Properties = make(map[string]string)
	}
	room.Properties[key] = val
	w.rooms[roomID] = room
	w.MarkRoomObjectsDirty(roomID)
	return nil
}

func (w *World) UpdateRoomRandomCreature(roomID model.RoomID, idx, val int) error {
	if w == nil {
		return fmt.Errorf("update room random creature: world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	room, ok := w.rooms[roomID]
	if !ok {
		return fmt.Errorf("room %s not found", roomID)
	}

	if room.Properties == nil {
		room.Properties = make(map[string]string)
	}
	key := fmt.Sprintf("random%d", idx)
	room.Properties[key] = strconv.Itoa(val)
	w.rooms[roomID] = room
	return nil
}

func (w *World) UpdateRoomFlag(roomID model.RoomID, flag int, enabled bool) error {
	if w == nil {
		return fmt.Errorf("update room flag: world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	room, ok := w.rooms[roomID]
	if !ok {
		return fmt.Errorf("room %s not found", roomID)
	}

	var flagName string
	roomFlagNamesList := []string{
		"shoppe", "dump", "pawnShop", "train", "trainingBit4", "trainingBit5", "trainingBit6",
		"repair", "darkAlways", "darkNight", "postOffice", "noPlayerKill", "noTeleport", "healFast",
		"onePlayer", "twoPlayers", "threePlayers", "noMagic", "permanentTracks", "earth", "wind",
		"fire", "water", "playerWander", "playerHarm", "playerPoison", "playerMPDrain", "playerBefuddle",
		"noSummonOut", "pledge", "rescind", "noPotion", "magicExtend", "noLog", "election",
		"forge", "survival", "family", "onlyFamily", "bank", "marriage", "onlyMarried", "cast", "depot",
	}

	if flag-1 >= 0 && flag-1 < len(roomFlagNamesList) {
		flagName = roomFlagNamesList[flag-1]
	} else {
		flagName = strconv.Itoa(flag)
	}

	targets := normalizedFlagSet(flagName)
	tags := make(map[string]struct{})
	for _, t := range room.Metadata.Tags {
		if !enabled {
			if _, ok := targets[normalizeFlagName(t)]; ok {
				continue
			}
		}
		tags[t] = struct{}{}
	}

	if enabled {
		tags[flagName] = struct{}{}
	} else if len(room.Properties) > 0 {
		for key := range room.Properties {
			if _, ok := targets[normalizeFlagName(key)]; ok {
				delete(room.Properties, key)
			}
		}
	}

	newTags := make([]string, 0, len(tags))
	for t := range tags {
		newTags = append(newTags, t)
	}
	room.Metadata.Tags = newTags
	w.rooms[roomID] = room
	return nil
}

func (w *World) UpdateCreatureStat(creatureID model.CreatureID, key string, val int) error {
	if w == nil {
		return fmt.Errorf("update creature stat: world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	c, ok := w.creatures[creatureID]
	if !ok {
		return fmt.Errorf("creature %s not found", creatureID)
	}

	if c.Stats == nil {
		c.Stats = make(map[string]int)
	}
	c.Stats[key] = val

	if key == "level" {
		c.Level = val
	}

	w.creatures[creatureID] = c
	return nil
}

func (w *World) UpdateCreatureProperty(creatureID model.CreatureID, key, val string) error {
	if w == nil {
		return fmt.Errorf("update creature property: world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	c, ok := w.creatures[creatureID]
	if !ok {
		return fmt.Errorf("creature %s not found", creatureID)
	}

	if c.Properties == nil {
		c.Properties = make(map[string]string)
	}
	c.Properties[key] = val
	w.creatures[creatureID] = c
	return nil
}

func (w *World) UpdateObjectProperty(objectID model.ObjectInstanceID, key, val string) error {
	if w == nil {
		return fmt.Errorf("update object property: world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	obj, ok := w.objects[objectID]
	if !ok {
		return fmt.Errorf("object %s not found", objectID)
	}

	if obj.Properties == nil {
		obj.Properties = make(map[string]string)
	}
	obj.Properties[key] = val
	w.objects[objectID] = obj
	return nil
}

func (w *World) LinkExits(fromRoomID, toRoomID model.RoomID, exitName, oppositeName string, doubleWay bool) error {
	if w == nil {
		return fmt.Errorf("link exits: world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	link := func(from, to model.RoomID, name string) error {
		room, ok := w.rooms[from]
		if !ok {
			return fmt.Errorf("room %s not found", from)
		}
		found := false
		for i, exit := range room.Exits {
			if strings.EqualFold(exit.Name, name) {
				room.Exits[i].ToRoomID = to
				found = true
				break
			}
		}
		if !found {
			room.Exits = append(room.Exits, model.Exit{
				Name:     name,
				ToRoomID: to,
			})
		}
		w.rooms[from] = room
		return nil
	}

	if err := link(fromRoomID, toRoomID, exitName); err != nil {
		return err
	}

	if doubleWay && oppositeName != "" {
		if err := link(toRoomID, fromRoomID, oppositeName); err != nil {
			return err
		}
	}
	return nil
}

func (w *World) DeleteRoomExit(roomID model.RoomID, exitName string) error {
	if w == nil {
		return fmt.Errorf("delete room exit: world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	room, ok := w.rooms[roomID]
	if !ok {
		return fmt.Errorf("room %s not found", roomID)
	}

	newExits := make([]model.Exit, 0, len(room.Exits))
	for _, exit := range room.Exits {
		if !strings.EqualFold(exit.Name, exitName) {
			newExits = append(newExits, exit)
		}
	}
	room.Exits = newExits
	w.rooms[roomID] = room
	return nil
}

func (w *World) Finger(addr, name string) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" || !legacyFingerResolvable(addr) {
		return legacyFingerInvalidHostMessage, nil
	}

	dialer := net.Dialer{Timeout: legacyFingerDialTimeout}
	conn, err := dialer.Dial("tcp", net.JoinHostPort(addr, legacyFingerPort))
	if err != nil {
		return legacyFingerUnableConnectMessage, nil
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(legacyFingerReadTimeout)); err != nil {
		return legacyFingerUnableConnectMessage, nil
	}
	if _, err := io.WriteString(conn, name+"\n\r\n\r"); err != nil {
		return legacyFingerUnableConnectMessage, nil
	}

	var b strings.Builder
	buf := make([]byte, 80)
	for int64(b.Len()) < legacyFingerOutputLimit {
		n, err := conn.Read(buf)
		if n > 0 {
			remaining := int(legacyFingerOutputLimit) - b.Len()
			if n > remaining {
				b.Write(buf[:remaining])
				b.WriteString(legacyFingerTruncatedMessage)
				return b.String(), nil
			}
			b.Write(buf[:n])
		}
		if err == nil {
			continue
		}
		if err == io.EOF {
			return b.String(), nil
		}
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			if b.Len() == 0 {
				return legacyFingerTimeoutMessage, nil
			}
			return b.String(), nil
		}
		if b.Len() > 0 {
			return b.String(), nil
		}
		return legacyFingerUnableConnectMessage, nil
	}
	b.WriteString(legacyFingerTruncatedMessage)
	return b.String(), nil
}

func legacyFingerResolvable(address string) bool {
	if net.ParseIP(address) != nil {
		return true
	}
	_, err := net.LookupHost(address)
	return err == nil
}

func (w *World) FindCreatureInRoom(roomID model.RoomID, name string) (model.Creature, bool) {
	if w == nil {
		return model.Creature{}, false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	room, ok := w.rooms[roomID]
	if !ok {
		return model.Creature{}, false
	}

	nameLower := strings.ToLower(strings.TrimSpace(name))
	if nameLower == "" {
		return model.Creature{}, false
	}

	for _, crtID := range room.CreatureIDs {
		c, ok := w.creatures[crtID]
		if !ok {
			continue
		}
		if creatureMatchesNameLocked(c, nameLower) {
			return cloneCreature(c), true
		}
	}
	return model.Creature{}, false
}

func (w *World) FindObjectInRoom(roomID model.RoomID, name string) (model.ObjectInstance, bool) {
	if w == nil {
		return model.ObjectInstance{}, false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	room, ok := w.rooms[roomID]
	if !ok {
		return model.ObjectInstance{}, false
	}

	nameLower := strings.ToLower(strings.TrimSpace(name))
	if nameLower == "" {
		return model.ObjectInstance{}, false
	}

	for _, objID := range room.Objects.ObjectIDs {
		obj, ok := w.objects[objID]
		if !ok {
			continue
		}
		if w.objectMatchesNameLocked(obj, nameLower) {
			return cloneObject(obj), true
		}
	}
	return model.ObjectInstance{}, false
}

func (w *World) FindObjectInRoomByName(roomID model.RoomID, name string, count int) (model.ObjectInstance, bool) {
	if w == nil {
		return model.ObjectInstance{}, false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	room, ok := w.rooms[roomID]
	if !ok {
		return model.ObjectInstance{}, false
	}

	nameLower := strings.ToLower(strings.TrimSpace(name))
	if nameLower == "" {
		return model.ObjectInstance{}, false
	}
	if count < 1 {
		count = 1
	}

	seen := 0
	for _, objID := range room.Objects.ObjectIDs {
		obj, ok := w.objects[objID]
		if !ok {
			continue
		}
		if w.objectMatchesNameLocked(obj, nameLower) {
			seen++
			if seen == count {
				return cloneObject(obj), true
			}
		}
	}
	return model.ObjectInstance{}, false
}

func (w *World) FindObjectOnCreature(creatureID model.CreatureID, name string) (model.ObjectInstance, bool) {
	if w == nil {
		return model.ObjectInstance{}, false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	c, ok := w.creatures[creatureID]
	if !ok {
		return model.ObjectInstance{}, false
	}

	nameLower := strings.ToLower(strings.TrimSpace(name))
	if nameLower == "" {
		return model.ObjectInstance{}, false
	}

	// Inventory
	for _, objID := range c.Inventory.ObjectIDs {
		obj, ok := w.objects[objID]
		if !ok {
			continue
		}
		if w.objectMatchesNameLocked(obj, nameLower) {
			return cloneObject(obj), true
		}
	}

	// Equipment
	for _, objID := range c.Equipment {
		obj, ok := w.objects[objID]
		if !ok {
			continue
		}
		if w.objectMatchesNameLocked(obj, nameLower) {
			return cloneObject(obj), true
		}
	}

	return model.ObjectInstance{}, false
}

func (w *World) FindObjectOnCreatureByName(creatureID model.CreatureID, name string, count int) (model.ObjectInstance, bool) {
	if w == nil {
		return model.ObjectInstance{}, false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	c, ok := w.creatures[creatureID]
	if !ok {
		return model.ObjectInstance{}, false
	}

	nameLower := strings.ToLower(strings.TrimSpace(name))
	if nameLower == "" {
		return model.ObjectInstance{}, false
	}
	if count < 1 {
		count = 1
	}

	seen := 0
	for _, objID := range c.Inventory.ObjectIDs {
		obj, ok := w.objects[objID]
		if !ok {
			continue
		}
		if w.objectMatchesNameLocked(obj, nameLower) {
			seen++
			if seen == count {
				return cloneObject(obj), true
			}
		}
	}

	for _, objID := range c.Equipment {
		obj, ok := w.objects[objID]
		if !ok {
			continue
		}
		if w.objectMatchesNameLocked(obj, nameLower) {
			seen++
			if seen == count {
				return cloneObject(obj), true
			}
		}
	}

	return model.ObjectInstance{}, false
}

// UpdateRoomDescription updates the short or long description of a room.
func (w *World) UpdateRoomDescription(roomID model.RoomID, field, val string) error {
	if w == nil {
		return fmt.Errorf("update room description: world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	room, ok := w.rooms[roomID]
	if !ok {
		return fmt.Errorf("room %s not found", roomID)
	}

	if field == "short" {
		room.ShortDescription = val
	} else if field == "long" {
		room.LongDescription = val
	} else {
		return fmt.Errorf("invalid room description field %q", field)
	}

	w.rooms[roomID] = room
	return nil
}

func (w *World) List(args []string) (string, error) {
	if w == nil {
		return "", fmt.Errorf("list: world state is nil")
	}
	opts, ok := parseLegacyListOptions(args)
	if !ok {
		return legacyListHelp(), nil
	}

	var b strings.Builder
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	switch opts.mode {
	case 'm':
		w.appendLegacyCreatureListLocked(&b, opts)
	case 'o':
		w.appendLegacyObjectListLocked(&b, opts)
	case 'r':
		w.appendLegacyRoomListLocked(&b, opts)
	case 'a':
		w.appendLegacyObjectListLocked(&b, opts)
		w.appendLegacyCreatureListLocked(&b, opts)
		w.appendLegacyRoomListLocked(&b, opts)
	default:
		return legacyListHelp(), nil
	}
	return b.String(), nil
}

type legacyListOptions struct {
	mode       byte
	rlo        int
	rhi        int
	levlo      int
	levhi      int
	dmglo      int
	dmghi      int
	dmgMin     int
	objectType int
	wear       int
	questOnly  bool
	object     int
	monster    int
	flags      []int
	notFlags   []int
	spells     []int
	classes    []int
	notClasses []int
}

type legacyCreatureListEntry struct {
	number   int
	creature model.Creature
}

type legacyObjectListEntry struct {
	number int
	proto  model.ObjectPrototype
}

type legacyRoomListEntry struct {
	number int
	room   model.Room
}

func parseLegacyListOptions(args []string) (legacyListOptions, bool) {
	opts := legacyListOptions{
		rlo:        0,
		rhi:        10000,
		levlo:      1,
		levhi:      127,
		dmglo:      0,
		dmghi:      1000,
		objectType: -1,
		wear:       -1,
		object:     -1,
		monster:    -1,
	}
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return opts, false
	}
	mode := strings.TrimSpace(args[0])
	opts.mode = strings.ToLower(mode[:1])[0]
	if opts.mode != 'm' && opts.mode != 'o' && opts.mode != 'r' && opts.mode != 'a' {
		return opts, false
	}
	if len(args) > 1 {
		opts.rlo = 0
		opts.rhi = 32000
	}

	for _, arg := range args[1:] {
		if arg == "" {
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			return opts, false
		}
		if len(arg) < 2 {
			continue
		}
		payload := ""
		if len(arg) > 2 {
			payload = arg[2:]
		}
		switch arg[1] {
		case 'r':
			lo, hi, ok := parseLegacyListRange(payload)
			if !ok {
				return opts, false
			}
			opts.rlo, opts.rhi = lo, hi
		case 'l':
			lo, hi, ok := parseLegacyListRange(payload)
			if !ok {
				return opts, false
			}
			opts.levlo, opts.levhi = lo, hi
		case 'd':
			lo, hi, ok := parseLegacyListRange(payload)
			if !ok {
				return opts, false
			}
			opts.dmglo, opts.dmghi = lo, hi
		case 't':
			opts.objectType = legacyAtoi(payload)
		case 'w':
			opts.wear = legacyAtoi(payload)
		case 'f':
			opts.flags = append(opts.flags, legacyAtoi(payload))
		case 'F':
			opts.notFlags = append(opts.notFlags, legacyAtoi(payload))
		case 'D':
			opts.dmgMin = legacyAtoi(payload)
		case 'c':
			opts.classes = append(opts.classes, legacyAtoi(payload))
		case 'C':
			opts.notClasses = append(opts.notClasses, legacyAtoi(payload))
		case 'q':
			opts.questOnly = true
		case 'o':
			opts.object = legacyAtoi(payload)
		case 'm':
			opts.monster = legacyAtoi(payload)
		case 'S':
			opts.spells = append(opts.spells, legacyAtoi(payload))
		case 's':
			// C uses this to redirect utility output to a socket fd. The Go server
			// returns output through the command context, so the descriptor is ignored.
		default:
			// The legacy utility ignored unknown dash options.
		}
	}
	return opts, true
}

func parseLegacyListRange(value string) (int, int, bool) {
	lo := -1
	hi := -1
	for i := 1; i < len(value); i++ {
		if value[i] != ':' {
			continue
		}
		lo = legacyAtoi(value[:i])
		hi = legacyAtoi(value[i+1:])
		break
	}
	return lo, hi, lo != -1 && hi != -1 && lo <= hi
}

func legacyAtoi(value string) int {
	value = strings.TrimLeft(value, " \t\r\n\v\f")
	if value == "" {
		return 0
	}
	sign := 1
	switch value[0] {
	case '-':
		sign = -1
		value = value[1:]
	case '+':
		value = value[1:]
	}
	n := 0
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			break
		}
		n = n*10 + int(value[i]-'0')
	}
	return sign * n
}

func legacyListHelp() string {
	return "list <m|o|r> [options]\n\r" +
		"[options]:  -r#:#     index range\n\r" +
		"            -s#       descriptor for output\n\r" +
		"            -l#:#     level range\n\r" +
		"            -t#       object type\n\r" +
		"            -w#       object wearflag\n\r" +
		"            -f#       flag set\n\r" +
		"            -F#       flag NOT set\n\r" +
		"            -S#       spell set\n\r" +
		"\t\t\t-d#:#\t  damage range\n\r" +
		"\t\t\t-D#\t\t  damage set\n\r" +
		"\t\t\t-c#\t\t  class\n\r" +
		"\t\t\t-C#\t\t  not class\n\r" +
		"            -q        quest objects only\n\r" +
		"\t    -o#       monsters/rooms carrying object\n\r" +
		"            -m#       rooms with monster\n"
}

func (w *World) appendLegacyCreatureListLocked(b *strings.Builder, opts legacyListOptions) {
	entries := make(map[int]legacyCreatureListEntry)
	for id, creature := range w.creatures {
		number, ok := legacyCreaturePrototypeNumber(id, creature)
		if !ok {
			continue
		}
		entries[number] = legacyCreatureListEntry{number: number, creature: creature}
	}

	writeLegacyCreatureListHeader(b, true)
	printed := 0
	missing := 0
	for i := opts.rlo; i <= opts.rhi; i++ {
		entry, ok := entries[i]
		if !ok {
			missing++
			if missing > 500 {
				break
			}
			continue
		}
		if !legacyCreatureListMatches(entry, opts) {
			continue
		}
		c := entry.creature
		fmt.Fprintf(b, "%3d. %-20s %02d %4d %02d/%02d/%02d/%02d/%02d %03d %03d %02d %05d %04d %2dd%-2d+%-2d\n\r",
			entry.number,
			legacyListName(c.DisplayName),
			c.Level,
			legacyCreatureStat(c, "alignment"),
			legacyCreatureStat(c, "strength"),
			legacyCreatureStat(c, "dexterity"),
			legacyCreatureStat(c, "constitution"),
			legacyCreatureStat(c, "intelligence"),
			legacyCreatureStat(c, "piety"),
			legacyCreatureStat(c, "hpMax"),
			legacyCreatureStat(c, "armor"),
			legacyCreatureStat(c, "thaco"),
			legacyCreatureStat(c, "experience"),
			legacyCreatureStat(c, "gold"),
			legacyCreatureStat(c, "nDice"),
			legacyCreatureStat(c, "sDice"),
			legacyCreatureStat(c, "pDice"),
		)
		printed++
		if printed%50 == 0 {
			writeLegacyCreatureListHeader(b, false)
		}
	}
}

func (w *World) appendLegacyObjectListLocked(b *strings.Builder, opts legacyListOptions) {
	entries := make(map[int]legacyObjectListEntry)
	for id, proto := range w.prototypes {
		number, ok := legacyObjectPrototypeNumber(id, proto)
		if !ok {
			continue
		}
		entries[number] = legacyObjectListEntry{number: number, proto: proto}
	}

	writeLegacyObjectListHeader(b)
	printed := 0
	missing := 0
	for i := opts.rlo; i <= opts.rhi; i++ {
		entry, ok := entries[i]
		if !ok {
			missing++
			if missing > 500 {
				break
			}
			continue
		}
		if !legacyObjectListMatches(entry, opts) {
			continue
		}
		proto := entry.proto
		fmt.Fprintf(b, "%3d. %-20s %06d %03d %02d +%-2d %03d %2dd%-2d+%-2d %02d %02d %02d %03d\n\r",
			entry.number,
			legacyListName(proto.DisplayName),
			legacyObjectProtoInt(proto, "value"),
			legacyObjectProtoInt(proto, "weight"),
			legacyObjectProtoInt(proto, "type"),
			legacyObjectProtoInt(proto, "adjustment"),
			legacyObjectProtoInt(proto, "shotsMax"),
			legacyObjectProtoInt(proto, "nDice"),
			legacyObjectProtoInt(proto, "sDice"),
			legacyObjectProtoInt(proto, "pDice"),
			legacyObjectProtoInt(proto, "armor"),
			legacyObjectProtoInt(proto, "wearFlag"),
			legacyObjectProtoInt(proto, "magicPower"),
			legacyObjectProtoInt(proto, "questNumber"),
		)
		printed++
		if printed%50 == 0 {
			writeLegacyObjectListHeader(b)
		}
	}
}

func (w *World) appendLegacyRoomListLocked(b *strings.Builder, opts legacyListOptions) {
	entries := make(map[int]legacyRoomListEntry)
	for _, room := range w.rooms {
		number, ok := legacyRoomNumber(room)
		if !ok {
			continue
		}
		entries[number] = legacyRoomListEntry{number: number, room: room}
	}

	writeLegacyRoomListHeader(b)
	printed := 0
	missing := 0
	for i := opts.rlo; i <= opts.rhi; i++ {
		entry, ok := entries[i]
		if !ok {
			missing++
			if missing > 500 && i > 2999 {
				break
			}
			continue
		}
		if !legacyRoomListMatches(entry, opts) {
			continue
		}
		room := entry.room
		random := legacyRoomRandomValues(room)
		fmt.Fprintf(b, "%3d. %-20s %03d/%03d/%03d/%03d/%03d/%03d/%03d/%03d/%03d/%03d %03d%%\n\r",
			entry.number,
			legacyListName(room.DisplayName),
			random[0], random[1], random[2], random[3], random[4],
			random[5], random[6], random[7], random[8], random[9],
			legacyRoomIntProperty(room, "traffic"),
		)
		printed++
		if printed%50 == 0 {
			writeLegacyRoomListHeader(b)
		}
	}
}

func writeLegacyCreatureListHeader(b *strings.Builder, carriageReturnDivider bool) {
	fmt.Fprintf(b, "%c\n%4s %-20s %-2s %4s %-14s %-3s %-3s %-2s %-5s %-4s %-8s\n\r", 12, "  # ", "Name", "Lv", "Algn", "Stats", "HP", "AC", "T0", "Exp", "Gold", " Dice")
	if carriageReturnDivider {
		b.WriteString("------------------------------------------------------------------------------\n\r")
	} else {
		b.WriteString("------------------------------------------------------------------------------\n")
	}
}

func writeLegacyObjectListHeader(b *strings.Builder) {
	fmt.Fprintf(b, "%c\n%4s %-20.20s %-6s %-3s %-2s %-3s %-3s %-8s %-2s %-2s %-2s %-3s\n\r", 12, "  # ", "Name", "Value", "Wgt", "Tp", "Adj", "Sht", " Dice", "AC", "Wr", "Mg", "Qst")
	b.WriteString("------------------------------------------------------------------------------\n\r")
}

func writeLegacyRoomListHeader(b *strings.Builder) {
	fmt.Fprintf(b, "%c\n%4s %-20s %-39s %-4s\n\r", 12, "  # ", "Name", "Random Monsters", "Traf")
	b.WriteString("------------------------------------------------------------------------------\n\r")
}

func legacyCreatureListMatches(entry legacyCreatureListEntry, opts legacyListOptions) bool {
	c := entry.creature
	if entry.number < opts.rlo || entry.number > opts.rhi {
		return false
	}
	if c.Level < opts.levlo || c.Level > opts.levhi {
		return false
	}
	damage := legacyCreatureStat(c, "nDice")*legacyCreatureStat(c, "sDice") + legacyCreatureStat(c, "pDice")
	if damage < opts.dmglo || damage > opts.dmghi || damage < opts.dmgMin {
		return false
	}
	if opts.object != -1 && !legacyCreatureCarriesObject(c, opts.object) {
		return false
	}
	if !legacyMetadataHasAllFlags(c.Metadata, opts.flags) || legacyMetadataHasAnyFlag(c.Metadata, opts.notFlags) {
		return false
	}
	if !legacyRawFieldHasAllBits(c.Metadata.RawFields["spells"], opts.spells) {
		return false
	}
	class := legacyCreatureStat(c, "class")
	for _, want := range opts.classes {
		if class != want {
			return false
		}
	}
	for _, blocked := range opts.notClasses {
		if class == blocked {
			return false
		}
	}
	return true
}

func legacyObjectListMatches(entry legacyObjectListEntry, opts legacyListOptions) bool {
	proto := entry.proto
	if entry.number < opts.rlo || entry.number > opts.rhi {
		return false
	}
	if opts.objectType != -1 && legacyObjectProtoInt(proto, "type") != opts.objectType {
		return false
	}
	damage := legacyObjectProtoInt(proto, "nDice")*legacyObjectProtoInt(proto, "sDice") + legacyObjectProtoInt(proto, "pDice")
	if damage < opts.dmglo || damage > opts.dmghi || damage < opts.dmgMin {
		return false
	}
	if opts.wear != -1 && legacyObjectProtoInt(proto, "wearFlag") != opts.wear {
		return false
	}
	if opts.questOnly && legacyObjectProtoInt(proto, "questNumber") == 0 {
		return false
	}
	if !legacyMetadataHasAllFlags(proto.Metadata, opts.flags) || legacyMetadataHasAnyFlag(proto.Metadata, opts.notFlags) {
		return false
	}
	return true
}

func legacyRoomListMatches(entry legacyRoomListEntry, opts legacyListOptions) bool {
	room := entry.room
	if entry.number < opts.rlo || entry.number > opts.rhi {
		return false
	}
	if !legacyMetadataHasAllFlags(room.Metadata, opts.flags) || legacyMetadataHasAnyFlag(room.Metadata, opts.notFlags) {
		return false
	}
	if opts.object != -1 && !legacyRoomPermanentObject(room, opts.object) {
		return false
	}
	if opts.monster != -1 && !legacyRoomHasMonster(room, opts.monster) {
		return false
	}
	return true
}

func legacyCreaturePrototypeNumber(id model.CreatureID, creature model.Creature) (int, bool) {
	if !isLegacyCreaturePrototypeEntry(id, creature) {
		return 0, false
	}
	return legacyCreatureNumberFromID(id)
}

func legacyCreatureNumberFromID(id model.CreatureID) (int, bool) {
	parts := strings.Split(string(id), ":")
	if len(parts) != 3 || parts[0] != "creature" || len(parts[1]) != 3 || parts[1][0] != 'm' {
		return 0, false
	}
	fileNo, err := strconv.Atoi(parts[1][1:])
	if err != nil {
		return 0, false
	}
	index, err := strconv.Atoi(parts[2])
	if err != nil || index < 0 {
		return 0, false
	}
	return fileNo*100 + index, true
}

func legacyObjectPrototypeNumber(id model.PrototypeID, proto model.ObjectPrototype) (int, bool) {
	if proto.Metadata.LegacyKind != "objectPrototype" {
		return 0, false
	}
	return legacyObjectNumberFromPrototypeID(id)
}

func legacyObjectNumberFromPrototypeID(id model.PrototypeID) (int, bool) {
	parts := strings.Split(string(id), ":")
	if len(parts) != 3 || parts[0] != "object" || len(parts[1]) != 3 || parts[1][0] != 'o' {
		return 0, false
	}
	fileNo, err := strconv.Atoi(parts[1][1:])
	if err != nil {
		return 0, false
	}
	index, err := strconv.Atoi(parts[2])
	if err != nil || index < 0 {
		return 0, false
	}
	return fileNo*100 + index, true
}

func legacyRoomNumber(room model.Room) (int, bool) {
	if value := strings.TrimPrefix(string(room.ID), "room:"); value != string(room.ID) {
		if n, err := strconv.Atoi(value); err == nil {
			return n, true
		}
	}
	for _, value := range []string{room.Metadata.LegacyID, filepath.Base(room.Metadata.LegacyPath)} {
		value = strings.TrimSpace(value)
		value = strings.TrimPrefix(value, "r")
		if n, err := strconv.Atoi(value); err == nil {
			return n, true
		}
	}
	return 0, false
}

func legacyCreatureStat(creature model.Creature, key string) int {
	if creature.Stats == nil {
		return 0
	}
	return creature.Stats[key]
}

func legacyObjectProtoInt(proto model.ObjectPrototype, key string) int {
	if proto.Properties == nil {
		return 0
	}
	if value, ok := parseStateInt(proto.Properties[key]); ok {
		return value
	}
	return 0
}

func legacyRoomIntProperty(room model.Room, key string) int {
	if room.Properties == nil {
		return 0
	}
	if value, ok := parseStateInt(room.Properties[key]); ok {
		return value
	}
	return 0
}

func legacyCreatureCarriesObject(creature model.Creature, objectNumber int) bool {
	for i := 0; i < 10; i++ {
		if legacyCreatureStat(creature, fmt.Sprintf("carry[%d]", i)) == objectNumber {
			return true
		}
	}
	return false
}

func legacyRoomRandomValues(room model.Room) [10]int {
	var out [10]int
	if room.Properties == nil {
		return out
	}
	if random := strings.TrimSpace(room.Properties["random"]); random != "" {
		parts := strings.Split(random, ",")
		for i := 0; i < len(parts) && i < len(out); i++ {
			if value, ok := parseStateInt(parts[i]); ok {
				out[i] = value
			}
		}
	}
	for i := range out {
		for _, key := range []string{fmt.Sprintf("random[%d]", i), fmt.Sprintf("random%d", i)} {
			if value, ok := parseStateInt(room.Properties[key]); ok {
				out[i] = value
			}
		}
	}
	return out
}

func legacyRoomHasMonster(room model.Room, monsterNumber int) bool {
	for _, value := range legacyRoomRandomValues(room) {
		if value == monsterNumber {
			return true
		}
	}
	return legacyRoomLasttimeHasMisc(room, monsterNumber, "perm_mon", "permMon", "permanentCreature")
}

func legacyRoomPermanentObject(room model.Room, objectNumber int) bool {
	return legacyRoomLasttimeHasMisc(room, objectNumber, "perm_obj", "permObj", "permanentObject")
}

func legacyRoomLasttimeHasMisc(room model.Room, want int, prefixes ...string) bool {
	if room.Properties == nil {
		return false
	}
	for i := 0; i < 10; i++ {
		for _, prefix := range prefixes {
			for _, key := range []string{
				fmt.Sprintf("%s.%d.misc", prefix, i),
				fmt.Sprintf("%s[%d].misc", prefix, i),
				fmt.Sprintf("%s.%02d.misc", prefix, i),
				fmt.Sprintf("%s[%02d].misc", prefix, i),
			} {
				if value, ok := parseStateInt(room.Properties[key]); ok && value == want {
					return true
				}
			}
		}
	}
	return false
}

func legacyMetadataHasAllFlags(metadata model.Metadata, flags []int) bool {
	for _, flag := range flags {
		if !metadataHasLegacyObjectFlag(metadata, flag-1) {
			return false
		}
	}
	return true
}

func legacyMetadataHasAnyFlag(metadata model.Metadata, flags []int) bool {
	for _, flag := range flags {
		if metadataHasLegacyObjectFlag(metadata, flag-1) {
			return true
		}
	}
	return false
}

func legacyRawFieldHasAllBits(raw []byte, bits []int) bool {
	for _, bit := range bits {
		if !legacyRawFieldHasBit(raw, bit-1) {
			return false
		}
	}
	return true
}

func legacyRawFieldHasBit(raw []byte, bit int) bool {
	if bit < 0 {
		return false
	}
	byteIndex := bit / 8
	if byteIndex >= len(raw) {
		return false
	}
	return raw[byteIndex]&(1<<uint(bit%8)) != 0
}

func legacyListName(name string) string {
	runes := []rune(name)
	if len(runes) > 20 {
		runes = runes[:20]
	}
	return string(runes)
}

func (w *World) CacheStats() (rooms, monsters, objects int) {
	if w == nil {
		return 0, 0, 0
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	return len(w.rooms), len(w.creatures), len(w.objects)
}

func (w *World) SaveAllPlayers() error {
	if w == nil {
		return fmt.Errorf("save all players: world state is nil")
	}

	w.rLockDomains(true, true, true, true, true, true, true)
	var playerIDs []model.PlayerID
	for id := range w.players {
		playerIDs = append(playerIDs, id)
	}
	w.rUnlockDomains(true, true, true, true, true, true, true)

	var errs []string
	for _, playerID := range playerIDs {
		if err := w.SavePlayer(playerID); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("save all players: %s", strings.Join(errs, "; "))
	}
	return nil
}

var globalWanderInterval int32 = 30

func (w *World) WanderInterval() int {
	return int(atomic.LoadInt32(&globalWanderInterval))
}

func (w *World) SetWanderInterval(val int) {
	atomic.StoreInt32(&globalWanderInterval, int32(val))
}

func (w *World) PlayerCounts() (active, queued int) {
	if w == nil {
		return 0, 0
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	return len(w.players), 0
}

func (w *World) ShutdownTimeRemaining() int64 {
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	if w.shutdownLTime == 0 {
		return 0
	}
	now := time.Now().Unix()
	rem := (w.shutdownLTime + w.shutdownInterval) - now
	if rem < 0 {
		return 0
	}
	return rem
}

var globalShipSailingInterval int64 = 600
var globalTimeToSail int64 = 300

func (w *World) ShipSailingInterval() int64 {
	return atomic.LoadInt64(&globalShipSailingInterval)
}

func (w *World) SetShipSailingInterval(val int64) {
	atomic.StoreInt64(&globalShipSailingInterval, val)
}

func (w *World) TimeToSail() int64 {
	return atomic.LoadInt64(&globalTimeToSail)
}

func (w *World) ForceShipSail() {
	atomic.StoreInt64(&globalTimeToSail, 0)
}

func (w *World) GetDailyBroadcastCount(creatureID model.CreatureID) (cur, max int) {
	if w == nil {
		return 0, 10
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	c, ok := w.creatures[creatureID]
	if !ok {
		return 0, 10
	}
	cur = 10
	max = 10
	if v, ok := c.Properties["dailyBroadcastCur"]; ok {
		if val, err := strconv.Atoi(v); err == nil {
			cur = val
		}
	}
	if v, ok := c.Properties["dailyBroadcastMax"]; ok {
		if val, err := strconv.Atoi(v); err == nil {
			max = val
		}
	}
	return cur, max
}

func (w *World) SetDailyBroadcastCount(creatureID model.CreatureID, val int) error {
	if w == nil {
		return fmt.Errorf("world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	c, ok := w.creatures[creatureID]
	if !ok {
		return fmt.Errorf("creature %s not found", creatureID)
	}
	if c.Properties == nil {
		c.Properties = make(map[string]string)
	}
	c.Properties["dailyBroadcastCur"] = strconv.Itoa(val)
	now := strconv.FormatInt(time.Now().Unix(), 10)
	c.Properties["dailyBroadcastLTime"] = now
	c.Properties["dailyBroadcastLastTime"] = now
	w.creatures[creatureID] = c
	return nil
}

func (w *World) SetRoomName(roomID model.RoomID, name string) error {
	if w == nil {
		return fmt.Errorf("world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	room, ok := w.rooms[roomID]
	if !ok {
		return fmt.Errorf("room %s not found", roomID)
	}
	room.DisplayName = name
	w.rooms[roomID] = room
	return nil
}

func (w *World) RoomPlayers(roomID model.RoomID) []model.Player {
	if w == nil {
		return nil
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	var list []model.Player
	for _, player := range w.players {
		if player.RoomID == roomID {
			list = append(list, player)
		}
	}
	return list
}

func (w *World) Players() []model.Player {
	if w == nil {
		return nil
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	var list []model.Player
	for _, player := range w.players {
		list = append(list, player)
	}
	return list
}

func (w *World) ActiveCreatures() []model.Creature {
	if w == nil {
		return nil
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	var list []model.Creature
	roomIDs := make([]model.RoomID, 0, len(w.rooms))
	for id, room := range w.rooms {
		if w.roomHasActivePlayerLocked(room) {
			roomIDs = append(roomIDs, id)
		}
	}
	sort.Slice(roomIDs, func(i, j int) bool {
		return string(roomIDs[i]) < string(roomIDs[j])
	})

	seen := make(map[model.CreatureID]struct{})
	for _, roomID := range roomIDs {
		room := w.rooms[roomID]
		for _, creatureID := range room.CreatureIDs {
			crt, ok := w.creatures[creatureID]
			if !ok || crt.Kind == model.CreatureKindPlayer || !crt.PlayerID.IsZero() {
				continue
			}
			if _, ok := seen[crt.ID]; ok {
				continue
			}
			seen[crt.ID] = struct{}{}
			list = append(list, cloneCreature(crt))
		}
	}
	return list
}

func (w *World) roomHasActivePlayerLocked(room model.Room) bool {
	if len(room.PlayerIDs) > 0 {
		return true
	}
	for _, creatureID := range room.CreatureIDs {
		crt, ok := w.creatures[creatureID]
		if !ok {
			continue
		}
		if crt.Kind == model.CreatureKindPlayer || !crt.PlayerID.IsZero() {
			return true
		}
	}
	return false
}

func (w *World) DustPlayer(playerID model.PlayerID) error {
	if w == nil {
		return fmt.Errorf("world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	player, ok := w.players[playerID]
	if !ok {
		return fmt.Errorf("player %s not found", playerID)
	}

	creature, hasCreature := w.creatures[player.CreatureID]
	roomIDs := []model.RoomID{player.RoomID}
	if hasCreature && creature.RoomID != player.RoomID {
		roomIDs = append(roomIDs, creature.RoomID)
	}
	for _, roomID := range roomIDs {
		if roomID.IsZero() {
			continue
		}
		room, ok := w.rooms[roomID]
		if !ok {
			continue
		}
		room.PlayerIDs = removeID(room.PlayerIDs, player.ID)
		if hasCreature {
			room.CreatureIDs = removeID(room.CreatureIDs, creature.ID)
		}
		w.rooms[room.ID] = room
	}
	if hasCreature {
		w.pruneCharmReferencesLocked(creature)
	}
	if w.cooldowns != nil {
		delete(w.cooldowns, player.CreatureID)
	}
	if w.monsterDamage != nil {
		delete(w.monsterDamage, player.CreatureID)
	}
	if w.enemies != nil {
		delete(w.enemies, player.CreatureID)
	}
	delete(w.players, playerID)
	if !player.CreatureID.IsZero() {
		delete(w.creatures, player.CreatureID)
	}
	return nil
}

func (w *World) AddEnemy(attacker, defender model.CreatureID) (bool, error) {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	if w.enemies == nil {
		w.enemies = make(map[model.CreatureID][]string)
	}
	var name string
	if c, ok := w.creatures[defender]; ok {
		name = c.DisplayName
	} else if p, ok := w.players[model.PlayerID(defender)]; ok {
		name = p.DisplayName
	} else {
		name = string(defender)
	}
	if name == "" {
		return false, nil
	}
	for _, existing := range w.enemies[attacker] {
		if existing == name {
			return false, nil
		}
	}
	w.enemies[attacker] = append(w.enemies[attacker], name)
	return true, nil
}

func (w *World) CreatureEnemies(creatureID model.CreatureID) ([]string, error) {
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	if w.enemies == nil {
		return nil, nil
	}
	return w.enemies[creatureID], nil
}

func (w *World) RemoveEnemy(creatureID model.CreatureID, enemyName string) error {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	if w.enemies == nil {
		return nil
	}
	list := w.enemies[creatureID]
	for i, name := range list {
		if name == enemyName {
			w.enemies[creatureID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	return nil
}

func (w *World) ClearCreatureEnemies(creatureID model.CreatureID) error {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	if w.enemies != nil {
		delete(w.enemies, creatureID)
	}
	return nil
}

func (w *World) RemoveCreature(creatureID model.CreatureID) error {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	creature, ok := w.creatures[creatureID]
	if !ok {
		return nil
	}
	if !creature.RoomID.IsZero() {
		if room, ok := w.rooms[creature.RoomID]; ok {
			room.CreatureIDs = removeID(room.CreatureIDs, creature.ID)
			w.rooms[room.ID] = room
		}
	}
	w.pruneCharmReferencesLocked(creature)
	delete(w.creatures, creatureID)
	if w.cooldowns != nil {
		delete(w.cooldowns, creatureID)
	}
	if w.monsterDamage != nil {
		delete(w.monsterDamage, creatureID)
	}
	if w.enemies != nil {
		delete(w.enemies, creatureID)
	}
	return nil
}

func (w *World) PlayerCharmedCreatures(playerID model.PlayerID) ([]string, error) {
	if w == nil {
		return nil, fmt.Errorf("list charmed creatures: world state is nil")
	}
	if playerID.IsZero() {
		return nil, nil
	}

	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	player, ok := w.players[playerID]
	if !ok {
		return nil, nil
	}

	var tags []string
	tags = append(tags, player.Metadata.Tags...)
	if !player.CreatureID.IsZero() {
		if creature, ok := w.creatures[player.CreatureID]; ok {
			tags = append(tags, creature.Metadata.Tags...)
		}
	}

	names := make([]string, 0)
	seen := map[string]struct{}{}
	addName := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		names = append(names, name)
	}

	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		lower := strings.ToLower(tag)
		switch {
		case strings.HasPrefix(lower, "charmid:"):
			rawID := strings.TrimSpace(tag[len("charmid:"):])
			if target, ok := w.creatures[model.CreatureID(rawID)]; ok {
				addName(target.DisplayName)
			}
		case strings.HasPrefix(lower, "charm:"):
			addName(tag[len("charm:"):])
		}
	}

	sort.Strings(names)
	return names, nil
}

func (w *World) SaveBank(bankID model.BankID) error {
	if w == nil {
		return fmt.Errorf("save bank: world state is nil")
	}

	w.rLockDomains(true, true, true, true, true, true, true)
	bank, ok := w.banks[bankID]
	if !ok {
		w.rUnlockDomains(true, true, true, true, true, true, true)
		return fmt.Errorf("save bank: bank account %q not found", bankID)
	}

	// Recursively collect objects (with deep clones for snapshot safety).
	var objects []model.ObjectInstance
	visited := make(map[model.ObjectInstanceID]bool)
	var collect func(ids []model.ObjectInstanceID)
	collect = func(ids []model.ObjectInstanceID) {
		for _, id := range ids {
			if id.IsZero() || visited[id] {
				continue
			}
			visited[id] = true
			if obj, exists := w.objects[id]; exists {
				objects = append(objects, cloneObject(obj))
				if len(obj.Contents.ObjectIDs) > 0 {
					collect(obj.Contents.ObjectIDs)
				}
			}
		}
	}
	collect(bank.Objects.ObjectIDs)

	// Clone the bank account itself
	bankClone := cloneBankAccount(bank)
	dbRoot := w.dbRoot
	w.rUnlockDomains(true, true, true, true, true, true, true)

	if dbRoot == "" {
		return fmt.Errorf("save bank %q: dbRoot is not set", bankID)
	}

	name := bank.OwnerName
	if name == "" {
		parts := strings.Split(string(bankID), ":")
		if len(parts) >= 3 {
			name = strings.Join(parts[2:], ":")
		} else {
			name = string(bankID)
		}
	}
	name, err := safeSidecarStem("bank", name)
	if err != nil {
		return err
	}

	path := filepath.Join(dbRoot, "player", "bank", "json", name+".json")

	bundle := model.BankSaveBundle{
		SchemaVersion: CurrentSaveSchemaVersion,
		BankAccount:   bankClone,
		Objects:       objects,
	}

	if err := jsonstore.WriteJSON(path, bundle); err != nil {
		log.Printf("[PERSIST] ERROR SaveBank %s: %v", bankID, err)
		return fmt.Errorf("save bank to JSON %q: %w", path, err)
	}

	// B: No longer mark dirty on save success (mutation-time MarkBankDirty required).
	// See SavePlayer for rationale (expert review durability fix).

	return nil
}

// LoadBank attempts to load a previously saved bank JSON sidecar for runtime restore.
func (w *World) LoadBank(bankID model.BankID) (model.BankSaveBundle, bool, error) {
	if w == nil {
		return model.BankSaveBundle{}, false, fmt.Errorf("load bank: world is nil")
	}
	dbRoot := w.dbRoot
	if dbRoot == "" {
		return model.BankSaveBundle{}, false, fmt.Errorf("load bank: dbRoot not set")
	}

	name := ""
	parts := strings.Split(string(bankID), ":")
	if len(parts) >= 3 {
		name = strings.Join(parts[2:], ":")
	} else {
		name = string(bankID)
	}
	name, err := safeSidecarStem("bank", name)
	if err != nil {
		return model.BankSaveBundle{}, false, err
	}

	path := filepath.Join(dbRoot, "player", "bank", "json", name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return model.BankSaveBundle{}, false, nil
		}
		return model.BankSaveBundle{}, false, err
	}

	var bundle model.BankSaveBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return model.BankSaveBundle{}, false, fmt.Errorf("parse bank JSON %s: %w", bankID, err)
	}
	bundle, _, err = MigrateBankSaveBundle(bundle)
	if err != nil {
		return model.BankSaveBundle{}, false, fmt.Errorf("migrate bank JSON %s: %w", bankID, err)
	}
	return bundle, true, nil
}

// MergeBankSave merges a loaded bank bundle into the current world state (for startup restore).
func (w *World) MergeBankSave(bundle model.BankSaveBundle) error {
	if w == nil || bundle.BankAccount.ID == "" {
		return nil
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	w.banks[bundle.BankAccount.ID] = bundle.BankAccount
	for _, obj := range bundle.Objects {
		w.objects[obj.ID] = obj
	}
	return nil
}
