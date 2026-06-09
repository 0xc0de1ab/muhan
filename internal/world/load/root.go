package load

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"muhan/internal/krtext"
	"muhan/internal/migrate/bankmap"
	"muhan/internal/migrate/invitemap"
	"muhan/internal/migrate/playermap"
	"muhan/internal/migrate/protomap"
	"muhan/internal/migrate/protoresolve"
	"muhan/internal/migrate/roommap"
	"muhan/internal/persist/legacykr"
	"muhan/internal/world/model"
)

type Options struct {
	Root string
}

type Summary struct {
	Root     string    `json:"root"`
	Counts   Counts    `json:"counts"`
	Warnings []Finding `json:"warnings,omitempty"`
	Errors   []Finding `json:"errors,omitempty"`
	World    *World    `json:"-"`
}

type Counts struct {
	RoomFiles                 int `json:"roomFiles"`
	Rooms                     int `json:"rooms"`
	SkippedRooms              int `json:"skippedRooms"`
	RoomContentErrors         int `json:"roomContentErrors"`
	RoomExits                 int `json:"roomExits"`
	RoomCreatures             int `json:"roomCreatures"`
	RoomObjects               int `json:"roomObjects"`
	RoomDescriptions          int `json:"roomDescriptions"`
	RoomDescriptionBytes      int `json:"roomDescriptionBytes"`
	RoomMaxDepth              int `json:"roomMaxDepth"`
	PlayerFiles               int `json:"playerFiles"`
	Players                   int `json:"players"`
	PlayerObjects             int `json:"playerObjects"`
	Families                  int `json:"families"`
	FamilyMemberFiles         int `json:"familyMemberFiles"`
	FamilyMembers             int `json:"familyMembers"`
	MarriageInviteFiles       int `json:"marriageInviteFiles"`
	MarriageInviteNames       int `json:"marriageInviteNames"`
	BankAccounts              int `json:"bankAccounts"`
	BankObjects               int `json:"bankObjects"`
	BankTrailingBytes         int `json:"bankTrailingBytes"`
	PrototypeResolved         int `json:"prototypeResolved"`
	PrototypeSynthetic        int `json:"prototypeSynthetic"`
	PrototypeAmbiguous        int `json:"prototypeAmbiguous"`
	SyntheticObjectPrototypes int `json:"syntheticObjectPrototypes"`
	Creatures                 int `json:"creatures"`
	ObjectInstances           int `json:"objectInstances"`
	ObjectPrototypeFiles      int `json:"objectPrototypeFiles"`
	ObjectPrototypes          int `json:"objectPrototypes"`
	CreaturePrototypeFiles    int `json:"creaturePrototypeFiles"`
	CreaturePrototypes        int `json:"creaturePrototypes"`
	Warnings                  int `json:"warnings"`
	Errors                    int `json:"errors"`
}

var (
	roomFileRE       = regexp.MustCompile(`^r[0-9]{5}$`)
	legacyRoomIDRE   = regexp.MustCompile(`^r([0-9]{5})$`)
	skippedPlayerDir = map[string]struct{}{
		"alias":    {},
		"bank":     {},
		"fal":      {},
		"family":   {},
		"invite":   {},
		"json":     {},
		"marriage": {},
		"simul":    {},
		"temp":     {},
		"vote":     {},
	}
)

func LoadRoot(root string) (Summary, error) {
	return Load(Options{Root: root})
}

func Load(opts Options) (Summary, error) {
	root := opts.Root
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Summary{}, fmt.Errorf("resolve root: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return Summary{}, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return Summary{}, fmt.Errorf("root is not a directory: %s", absRoot)
	}

	summary := Summary{
		Root:     absRoot,
		Warnings: []Finding{},
		Errors:   []Finding{},
		World:    NewWorld(),
	}

	objectResolver, err := protoresolve.BuildObjectResolver(absRoot)
	if err != nil {
		summary.addWarning("build_object_resolver", "objmon", "", "", err.Error())
	}

	loadRooms(absRoot, &summary, objectResolver)
	loadPlayers(absRoot, &summary, objectResolver)
	loadFamilies(absRoot, &summary)
	loadMarriageInvites(absRoot, &summary)
	loadBanks(absRoot, &summary, objectResolver)
	loadPrototypes(absRoot, &summary)
	materializeSyntheticObjectPrototypes(&summary)

	refReport := summary.World.ValidateRefs()
	summary.Warnings = append(summary.Warnings, refReport.Warnings...)
	summary.Errors = append(summary.Errors, refReport.Errors...)

	summary.Counts.Rooms = len(summary.World.Rooms)
	summary.Counts.Players = len(summary.World.Players)
	summary.Counts.Families = len(summary.World.Families)
	summary.Counts.BankAccounts = len(summary.World.Banks)
	summary.Counts.Creatures = len(summary.World.Creatures)
	summary.Counts.ObjectInstances = len(summary.World.Objects)
	summary.Counts.ObjectPrototypes = len(summary.World.ObjectPrototypes)
	summary.Counts.Warnings = len(summary.Warnings)
	summary.Counts.Errors = len(summary.Errors)
	return summary, nil
}

func loadRooms(root string, summary *Summary, objectResolver protoresolve.ObjectPrototypeResolver) {
	base := filepath.Join(root, "rooms")
	if _, err := os.Stat(base); err != nil {
		summary.addWarning("missing_rooms_dir", displayPath(root, base), "", "", err.Error())
		return
	}

	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		relPath := displayPath(root, path)
		if err != nil {
			summary.addWarning("read_room_path", relPath, "", "", err.Error())
			return nil
		}
		if d.IsDir() || !roomFileRE.MatchString(d.Name()) {
			return nil
		}

		summary.Counts.RoomFiles++
		data, err := os.ReadFile(path)
		if err != nil {
			summary.Counts.SkippedRooms++
			summary.addWarning("read_room", relPath, "", "", err.Error())
			return nil
		}

		bundle, err := roommap.MapRoomFileBundleWithOptions(relPath, data, roommap.Options{PrototypeResolver: objectResolver})
		if err != nil {
			summary.Counts.SkippedRooms++
			summary.addWarning("map_room", relPath, "", "", err.Error())
			return nil
		}
		room := bundle.Room
		for _, warning := range bundle.Warnings {
			summary.addWarning("map_room", relPath, string(room.ID), "", warning)
		}
		if bundle.ContentError != "" {
			summary.Counts.RoomContentErrors++
		} else {
			summary.Counts.RoomExits += bundle.Decoded.Exits
			summary.Counts.RoomDescriptions += bundle.Decoded.Descriptions
			summary.Counts.RoomDescriptionBytes += bundle.Decoded.DescriptionBytes
			if bundle.Decoded.MaxDepth > summary.Counts.RoomMaxDepth {
				summary.Counts.RoomMaxDepth = bundle.Decoded.MaxDepth
			}
		}
		summary.Counts.RoomCreatures += len(bundle.Creatures)
		summary.Counts.RoomObjects += len(bundle.Objects)
		addPrototypeResolution(&summary.Counts, bundle.PrototypeResolution.ResolvedExact, bundle.PrototypeResolution.Synthetic, bundle.PrototypeResolution.AmbiguousSynthetic)
		if err := summary.World.AddRoom(room); err != nil {
			summary.addError("add_room", relPath, string(room.ID), "", err.Error())
		}
		for _, creature := range bundle.Creatures {
			if err := summary.World.AddCreature(creature); err != nil {
				summary.addError("add_room_creature", relPath, string(creature.ID), "", err.Error())
			}
		}
		for _, object := range bundle.Objects {
			if err := summary.World.AddObjectInstance(object); err != nil {
				summary.addError("add_room_object", relPath, string(object.ID), "", err.Error())
			}
		}
		return nil
	})
	if err != nil {
		summary.addError("scan_rooms", displayPath(root, base), "", "", err.Error())
	}

	// Scan `<root>/rooms/json/*.json` and merge or overwrite the room's floor ObjectIDs and insert object instances
	jsonDir := filepath.Join(root, "rooms", "json")
	if _, err := os.Stat(jsonDir); err == nil {
		jsonFiles, err := filepath.Glob(filepath.Join(jsonDir, "*.json"))
		if err == nil {
			sort.Strings(jsonFiles)
			for _, file := range jsonFiles {
				relPath := displayPath(root, file)
				data, err := os.ReadFile(file)
				if err != nil {
					summary.addError("read_room_json", relPath, "", "", err.Error())
					continue
				}

				type SavedRoomState struct {
					RoomID         model.RoomID             `json:"roomId"`
					FloorObjectIDs []model.ObjectInstanceID `json:"floorObjectIds,omitempty"`
					Properties     map[string]string        `json:"properties,omitempty"`
					Objects        []model.ObjectInstance   `json:"objects,omitempty"`
				}

				var saved SavedRoomState
				if err := json.Unmarshal(data, &saved); err != nil {
					summary.addError("decode_room_json", relPath, "", "", err.Error())
					continue
				}

				if saved.RoomID.IsZero() {
					summary.addError("map_room_json", relPath, "", "", "roomId is zero or empty")
					continue
				}

				room, ok := summary.World.Rooms[saved.RoomID]
				if !ok {
					summary.addWarning("json_room_not_found", relPath, string(saved.RoomID), "", "room not found in legacy world load")
					continue
				}

				// Overwrite the floor ObjectIDs
				room.Objects.ObjectIDs = saved.FloorObjectIDs
				if len(saved.Properties) > 0 {
					if room.Properties == nil {
						room.Properties = make(map[string]string, len(saved.Properties))
					}
					for key, value := range saved.Properties {
						room.Properties[key] = value
					}
				}
				summary.World.Rooms[saved.RoomID] = room

				// Insert/overwrite the object instances
				for _, obj := range saved.Objects {
					if err := obj.Validate(); err != nil {
						summary.addError("validate_json_room_object", relPath, string(obj.ID), "", err.Error())
						continue
					}
					summary.World.Objects[obj.ID] = obj
				}
			}
		}
	}
}

type playerJSON struct {
	Player          *model.Player          `json:"player"`
	Creature        *model.Creature        `json:"creature"`
	Objects         []model.ObjectInstance `json:"objects"`
	ObjectInstances []model.ObjectInstance `json:"objectInstances"`
}

func decodePlayerFilename(filename string) string {
	raw := []byte(filename)
	if filename == "" {
		return ""
	}
	utf8Valid := utf8.Valid(raw)
	if utf8Valid && krtext.IsLegacyName(filename) {
		return filename
	}
	legacyText, legacyErr := legacykr.DecodeEUCKRContext(legacykr.Context{Path: filename, Field: "player filename"}, raw)
	if legacyErr == nil {
		return legacyText
	}
	return filename
}

func isPlayerLoaded(loaded map[model.PlayerID]struct{}, id model.PlayerID) bool {
	if _, ok := loaded[id]; ok {
		return true
	}
	s := string(id)
	var alt model.PlayerID
	if strings.HasPrefix(s, "player:") {
		alt = model.PlayerID(strings.TrimPrefix(s, "player:"))
	} else {
		alt = model.PlayerID("player:" + s)
	}
	_, ok := loaded[alt]
	return ok
}

func loadPlayers(root string, summary *Summary, objectResolver protoresolve.ObjectPrototypeResolver) {
	loadedPlayers := make(map[model.PlayerID]struct{})

	// Scan and load `<root>/player/json/*.json` files first.
	jsonDir := filepath.Join(root, "player", "json")
	if jsonFiles, err := filepath.Glob(filepath.Join(jsonDir, "*.json")); err == nil && len(jsonFiles) > 0 {
		sort.Strings(jsonFiles)
		for _, file := range jsonFiles {
			relPath := displayPath(root, file)
			summary.Counts.PlayerFiles++
			data, err := os.ReadFile(file)
			if err != nil {
				summary.addError("read_player_json", relPath, "", "", err.Error())
				continue
			}

			var pData playerJSON
			if err := json.Unmarshal(data, &pData); err != nil {
				summary.addError("decode_player_json", relPath, "", "", err.Error())
				continue
			}
			if pData.Player == nil {
				summary.addError("map_player_json", relPath, "", "", "missing player object")
				continue
			}
			if pData.Player.ID == "" {
				summary.addError("map_player_json", relPath, "", "", "empty player ID")
				continue
			}
			if pData.Creature == nil {
				summary.addError("map_player_json", relPath, string(pData.Player.ID), "", "missing creature object")
				continue
			}

			pData.Player.RoomID = canonicalRoomID(pData.Player.RoomID)
			pData.Creature.RoomID = canonicalRoomID(pData.Creature.RoomID)

			if err := summary.World.AddPlayer(*pData.Player); err != nil {
				summary.addError("add_player", relPath, string(pData.Player.ID), "", err.Error())
				continue
			}
			if err := summary.World.AddCreature(*pData.Creature); err != nil {
				summary.addError("add_creature", relPath, string(pData.Creature.ID), "", err.Error())
				continue
			}

			objects := pData.Objects
			if len(objects) == 0 {
				objects = pData.ObjectInstances
			}
			summary.Counts.PlayerObjects += len(objects)
			for _, obj := range objects {
				if obj.Location.RoomID != "" {
					obj.Location.RoomID = canonicalRoomID(obj.Location.RoomID)
				}
				if err := summary.World.AddObjectInstance(obj); err != nil {
					summary.addError("add_player_object", relPath, string(obj.ID), "", err.Error())
				}
			}

			loadedPlayers[pData.Player.ID] = struct{}{}
		}
	}

	playerRoot := filepath.Join(root, "player")
	entries, err := os.ReadDir(playerRoot)
	if err != nil {
		summary.addWarning("missing_player_dir", displayPath(root, playerRoot), "", "", err.Error())
		return
	}
	sortDirEntries(entries)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		shard := entry.Name()
		if _, skip := skippedPlayerDir[shard]; skip {
			continue
		}
		loadPlayerShard(root, filepath.Join(playerRoot, shard), shard, summary, objectResolver, loadedPlayers)
	}
}

func loadPlayerShard(root, dir, shard string, summary *Summary, objectResolver protoresolve.ObjectPrototypeResolver, loadedPlayers map[model.PlayerID]struct{}) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		summary.addError("read_player_shard", displayPath(root, dir), "", "", err.Error())
		return
	}
	sortDirEntries(entries)

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		relPath := displayPath(root, path)
		if entry.IsDir() {
			continue
		}

		legacyName := decodePlayerFilename(entry.Name())
		legacyID := model.PlayerID(legacyName)
		if isPlayerLoaded(loadedPlayers, legacyID) {
			continue
		}

		summary.Counts.PlayerFiles++
		data, err := os.ReadFile(path)
		if err != nil {
			summary.addError("read_player", relPath, "", "", err.Error())
			continue
		}

		result, err := playermap.MapPlayerFileWithOptions(relPath, shard, data, playermap.Options{IncludeObjects: true, PrototypeResolver: objectResolver})
		if err != nil {
			summary.addError("map_player", relPath, "", "", err.Error())
			continue
		}
		for _, warning := range result.Warnings {
			summary.addWarning("map_player", warning.Path, string(result.Player.ID), "", warning.Message)
		}

		result.Player.RoomID = canonicalRoomID(result.Player.RoomID)
		result.Creature.RoomID = canonicalRoomID(result.Creature.RoomID)
		if err := summary.World.AddPlayer(result.Player); err != nil {
			summary.addError("add_player", relPath, string(result.Player.ID), "", err.Error())
		}
		if err := summary.World.AddCreature(result.Creature); err != nil {
			summary.addError("add_creature", relPath, string(result.Creature.ID), "", err.Error())
		}
		summary.Counts.PlayerObjects += len(result.Objects)
		addPrototypeResolution(&summary.Counts, result.PrototypeResolution.ResolvedExact, result.PrototypeResolution.Synthetic, result.PrototypeResolution.AmbiguousSynthetic)
		for _, object := range result.Objects {
			if err := summary.World.AddObjectInstance(object); err != nil {
				summary.addError("add_player_object", relPath, string(object.ID), "", err.Error())
			}
		}
	}
}

func loadMarriageInvites(root string, summary *Summary) {
	base := filepath.Join(root, "player", "invite")
	if _, err := os.Stat(base); err != nil {
		if os.IsNotExist(err) {
			return
		}
		summary.addWarning("read_marriage_invites", displayPath(root, base), "", "", err.Error())
		return
	}

	report, err := invitemap.ScanDir(base)
	if err != nil {
		summary.addWarning("read_marriage_invites", displayPath(root, base), "", "", err.Error())
		return
	}

	summary.Counts.MarriageInviteFiles = report.Counts.MappedFiles
	summary.Counts.MarriageInviteNames = report.Counts.Names
	for _, warning := range report.Warnings {
		summary.addWarning("map_marriage_invite", displayPath(root, warning.Path), "", "", warning.Message)
	}
	for _, finding := range report.Errors {
		summary.addError("map_marriage_invite", displayPath(root, finding.Path), "", "", finding.Message)
	}
	for _, invite := range report.Invites {
		summary.World.MarriageInvites[model.SpecialID(invite.Number)] = append([]string(nil), invite.Names...)
	}
}

func loadJSONBanks(root string, summary *Summary) map[string]bool {
	loaded := make(map[string]bool)
	jsonDir := filepath.Join(root, "player", "bank", "json")
	entries, err := os.ReadDir(jsonDir)
	if err != nil {
		if os.IsNotExist(err) {
			return loaded
		}
		summary.addWarning("read_json_banks_dir", displayPath(root, jsonDir), "", "", err.Error())
		return loaded
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(jsonDir, entry.Name())
		relPath := displayPath(root, path)

		data, err := os.ReadFile(path)
		if err != nil {
			summary.addError("read_json_bank", relPath, "", "", err.Error())
			continue
		}

		var bundle model.BankSaveBundle
		if err := json.Unmarshal(data, &bundle); err != nil {
			summary.addError("decode_json_bank", relPath, "", "", err.Error())
			continue
		}

		if bundle.BankAccount.ID.IsZero() {
			summary.addError("invalid_json_bank", relPath, "", "", "bank account ID is missing or empty")
			continue
		}

		if err := summary.World.AddBank(bundle.BankAccount); err != nil {
			summary.addError("add_json_bank", relPath, string(bundle.BankAccount.ID), "", err.Error())
			continue
		}

		for _, obj := range bundle.Objects {
			if err := summary.World.AddObjectInstance(obj); err != nil {
				summary.addError("add_json_bank_object", relPath, string(obj.ID), "", err.Error())
			} else {
				summary.Counts.BankObjects++
			}
		}

		loaded[string(bundle.BankAccount.ID)] = true
	}
	return loaded
}

func loadBanks(root string, summary *Summary, objectResolver protoresolve.ObjectPrototypeResolver) {
	loadedJSONBanks := loadJSONBanks(root, summary)

	snapshot, err := bankmap.Build(bankmap.Options{
		Root:              root,
		IncludeObjects:    true,
		PrototypeResolver: objectResolver,
		SkipBankIDs:       loadedJSONBanks,
	})
	if err != nil {
		summary.addError("build_bankmap", "", "", "", err.Error())
		return
	}
	summary.Counts.BankObjects += len(snapshot.Objects)
	summary.Counts.BankTrailingBytes += snapshot.Counts.TrailingBytes
	addPrototypeResolution(&summary.Counts, snapshot.Counts.PrototypeResolved, snapshot.Counts.PrototypeSynthetic, snapshot.Counts.PrototypeAmbiguous)

	for _, warning := range snapshot.Warnings {
		summary.addWarning("map_bank", warning.Path, warning.ID, "", warning.Message)
	}
	for _, finding := range snapshot.Errors {
		summary.addError("map_bank", finding.Path, finding.ID, "", finding.Message)
	}
	for _, bank := range snapshot.Banks {
		account := bank.BankAccount()
		if err := summary.World.AddBank(account); err != nil {
			summary.addError("add_bank", bank.Path, bank.ID, "", err.Error())
		}
	}
	for _, object := range snapshot.Objects {
		if err := summary.World.AddObjectInstance(object); err != nil {
			summary.addError("add_bank_object", object.Metadata.LegacyPath, string(object.ID), "", err.Error())
		}
	}
}

func addPrototypeResolution(counts *Counts, resolved, synthetic, ambiguous int) {
	counts.PrototypeResolved += resolved
	counts.PrototypeSynthetic += synthetic
	counts.PrototypeAmbiguous += ambiguous
}

func materializeSyntheticObjectPrototypes(summary *Summary) {
	for _, proto := range protoresolve.MaterializeSyntheticObjectPrototypes(summary.World.Objects) {
		if _, exists := summary.World.ObjectPrototypes[proto.ID]; exists {
			summary.addWarning("add_synthetic_object_prototype", proto.Metadata.LegacyPath, string(proto.ID), "", "prototype id already exists; skipped synthetic materialization")
			continue
		}
		if err := summary.World.AddObjectPrototype(proto); err != nil {
			summary.addError("add_synthetic_object_prototype", proto.Metadata.LegacyPath, string(proto.ID), "", err.Error())
			continue
		}
		summary.Counts.SyntheticObjectPrototypes++
	}
}

func loadPrototypes(root string, summary *Summary) {
	snapshot, err := protomap.Build(protomap.Options{Root: root})
	if err != nil {
		summary.addError("build_protomap", "", "", "", err.Error())
		return
	}
	summary.Counts.ObjectPrototypeFiles = snapshot.Counts.ObjectPrototypeFiles
	summary.Counts.CreaturePrototypeFiles = snapshot.Counts.CreaturePrototypeFiles
	summary.Counts.CreaturePrototypes = snapshot.Counts.CreaturePrototypes

	for _, warning := range snapshot.Warnings {
		summary.addWarning("map_prototype", warning.Path, warning.ID, "", warning.Message)
	}
	for _, finding := range snapshot.Errors {
		summary.addError("map_prototype", finding.Path, finding.ID, "", finding.Message)
	}
	for _, proto := range snapshot.ObjectPrototypes {
		if err := summary.World.AddObjectPrototype(proto); err != nil {
			summary.addError("add_object_prototype", proto.Metadata.LegacyPath, string(proto.ID), "", err.Error())
		}
	}
	for _, proto := range snapshot.CreaturePrototypes {
		creature := creatureFromPrototype(proto)
		if err := summary.World.AddCreature(creature); err != nil {
			summary.addError("add_creature_prototype", proto.Metadata.LegacyPath, string(proto.ID), "", err.Error())
		}
	}
}

func creatureFromPrototype(proto protomap.CreaturePrototypeRecord) model.Creature {
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

func canonicalRoomID(id model.RoomID) model.RoomID {
	if id.IsZero() {
		return id
	}
	m := legacyRoomIDRE.FindStringSubmatch(string(id))
	if m == nil {
		return id
	}
	return model.RoomID("room:" + m[1])
}

func (s *Summary) addWarning(kind, path, id, ref, message string) {
	s.Warnings = append(s.Warnings, Finding{
		Kind:    kind,
		Path:    path,
		ID:      id,
		Ref:     ref,
		Message: message,
	})
}

func (s *Summary) addError(kind, path, id, ref, message string) {
	s.Errors = append(s.Errors, Finding{
		Kind:    kind,
		Path:    path,
		ID:      id,
		Ref:     ref,
		Message: message,
	})
}

func sortDirEntries(entries []os.DirEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
}

func displayPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
