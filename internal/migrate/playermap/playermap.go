package playermap

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"muhan/internal/krtext"
	"muhan/internal/migrate/objectmap"
	"muhan/internal/migrate/protoresolve"
	"muhan/internal/persist/cbin"
	"muhan/internal/persist/legacykr"
	"muhan/internal/world/model"
)

const (
	legacySource = "legacy"

	EncodingUTF8     = "utf-8"
	EncodingLegacyKR = "euc-kr/cp949"
	EncodingInvalid  = "invalid"

	creatureLevelOff        = 318
	creatureTypeOff         = 319
	creatureClassOff        = 320
	creatureRaceOff         = 321
	creatureNumWanderOff    = 322
	creatureAlignmentOff    = 324
	creatureStrengthOff     = 326
	creatureDexterityOff    = 327
	creatureConstitutionOff = 328
	creatureIntelligenceOff = 329
	creaturePietyOff        = 330
	creatureHPMaxOff        = 332
	creatureHPCurOff        = 334
	creatureMPMaxOff        = 336
	creatureMPCurOff        = 338
	creatureArmorOff        = 340
	creatureThacoOff        = 341
	creatureExperienceOff   = 344
	creatureGoldOff         = 348
	creatureNDiceOff        = 352
	creatureSDiceOff        = 354
	creaturePDiceOff        = 356
	creatureSpecialOff      = 358
	creatureQuestNumOff     = 436
	creatureRoomNumberOff   = 458
	creatureLasttimeOff     = 620
	lasttimeIntervalOff     = 0

	legacyPINVIS  = 2
	legacyPDMINV  = 10
	legacyPMALES  = 12
	legacyPDINVI  = 21
	legacyPBLIND  = 42
	legacyPSILNC  = 44
	legacyPFAMIL  = 55
	legacySUICD   = 62
	legacyLTHOURS = 28
	legacyDLMARRI = 8
	legacyDLEXPND = 9
)

type Finding struct {
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type Options struct {
	IncludeObjects    bool
	PrototypeResolver protoresolve.ObjectPrototypeResolver
}

type Result struct {
	Path                string                              `json:"path"`
	Shard               string                              `json:"shard,omitempty"`
	Filename            string                              `json:"filename"`
	FilenameEncoding    string                              `json:"filenameEncoding"`
	Player              model.Player                        `json:"player"`
	Creature            model.Creature                      `json:"creature"`
	Objects             []model.ObjectInstance              `json:"objects,omitempty"`
	PrototypeResolution objectmap.PrototypeResolutionCounts `json:"prototypeResolution"`
	Decoded             cbin.Stats                          `json:"decoded"`
	InvalidName         bool                                `json:"invalidName,omitempty"`
	RecordNameMismatch  bool                                `json:"recordNameMismatch,omitempty"`
	Warnings            []Finding                           `json:"warnings,omitempty"`
}

type decodedComponent struct {
	Text     string
	Encoding string
	Raw      []byte
	Warning  string
}

type creatureNumbers struct {
	Level          int
	Type           int
	Class          int
	Race           int
	NumWander      int
	Alignment      int
	Strength       int
	Dexterity      int
	Constitution   int
	Intelligence   int
	Piety          int
	HPMax          int
	HPCurrent      int
	MPMax          int
	MPCurrent      int
	Armor          int
	Thaco          int
	Experience     int
	Gold           int
	NDice          int
	SDice          int
	PDice          int
	Special        int
	QuestNumber    int
	RoomNumber     int
	InventoryCount int
	Invisible      int
	DMInvisible    int
	Male           int
	DetectInvis    int
	Blind          int
	Silenced       int
	Suicide        int
	HoursInterval  int
	AgeYears       int
	FamilyFlag     int
	FamilyID       int
	MarriageID     int
}

// MapPlayerFile maps one legacy player creature-tree file into draft Player and
// Creature models. The filename is the canonical player display name/id; the
// creature record name is retained as legacy metadata and compared for drift.
func MapPlayerFile(path string, shard string, data []byte) (Result, error) {
	return MapPlayerFileWithOptions(path, shard, data, Options{})
}

func MapPlayerFileWithOptions(path string, shard string, data []byte, opts Options) (Result, error) {
	displayPath := displayPath(path)
	filename := filepath.Base(path)
	if filename == "." || filename == string(filepath.Separator) {
		filename = path
	}

	nameInfo := decodePathComponent(filename, "player filename")
	shardInfo := decodePathComponent(shard, "player shard")

	result := Result{
		Path:             displayPath,
		Shard:            shardInfo.Text,
		Filename:         nameInfo.Text,
		FilenameEncoding: nameInfo.Encoding,
		Warnings:         []Finding{},
	}
	if nameInfo.Warning != "" {
		result.Warnings = append(result.Warnings, Finding{Path: displayPath, Message: nameInfo.Warning})
	}
	if shardInfo.Warning != "" {
		result.Warnings = append(result.Warnings, Finding{Path: displayPath, Message: shardInfo.Warning})
	}

	displayName := nameInfo.Text
	if displayName == "" {
		displayName = rawID("filename", nameInfo.Raw)
		result.Filename = displayName
	}

	if !krtext.IsLegacyName(displayName) {
		result.InvalidName = true
		result.Warnings = append(result.Warnings, Finding{
			Path:    displayPath,
			Message: fmt.Sprintf("player filename %q does not satisfy legacy Korean name policy: 1-%d Hangul syllables", displayName, krtext.LegacyNameMaxSyllables),
		})
	}

	if shardInfo.Text != "" {
		expected := krtext.FirstHangulBucket(displayName)
		if expected != "temp" && shardInfo.Text != expected {
			result.Warnings = append(result.Warnings, Finding{
				Path:    displayPath,
				Message: fmt.Sprintf("player shard %q does not match expected initial bucket %q for filename %q", shardInfo.Text, expected, displayName),
			})
		}
	}

	record, err := cbin.DecodeCreatureRecord(data)
	if err != nil {
		return result, fmt.Errorf("decode creature record: %w", err)
	}
	result.addTextWarnings(displayPath, "creature.name", record.Name)
	result.addTextWarnings(displayPath, "creature.description", record.Description)
	result.addTextWarnings(displayPath, "creature.talk", record.Talk)
	result.addTextWarnings(displayPath, "creature.password", record.Password)

	node, err := cbin.DecodeCreatureTree(data)
	if err != nil {
		return result, fmt.Errorf("decode creature file: %w", err)
	}
	result.Decoded = node.Stats

	numbers := readCreatureNumbers(data, record)
	roomID, roomWarning := roomID(numbers.RoomNumber)
	if roomWarning != "" {
		result.Warnings = append(result.Warnings, Finding{Path: displayPath, Message: roomWarning})
	}

	playerID := model.PlayerID(displayName)
	creatureID := model.CreatureID("creature:player:" + displayName)

	rawFields := map[string][]byte{
		"filename": cloneBytes(nameInfo.Raw),
	}
	if len(shardInfo.Raw) > 0 {
		rawFields["shard"] = cloneBytes(shardInfo.Raw)
	}
	addRawField(rawFields, "creature.name", record.Name.Raw)
	addRawField(rawFields, "creature.description", record.Description.Raw)
	addRawField(rawFields, "creature.talk", record.Talk.Raw)
	addRawField(rawFields, "creature.password", record.Password.Raw)

	playerMetadata := model.Metadata{
		Source:         legacySource,
		LegacyKind:     "player",
		LegacyID:       displayName,
		LegacyPath:     displayPath,
		LegacyEncoding: nameInfo.Encoding,
		RawFields: map[string][]byte{
			"filename": cloneBytes(nameInfo.Raw),
		},
	}
	if len(shardInfo.Raw) > 0 {
		playerMetadata.RawFields["shard"] = cloneBytes(shardInfo.Raw)
	}

	result.Player = model.Player{
		ID:          playerID,
		DisplayName: displayName,
		CreatureID:  creatureID,
		RoomID:      roomID,
		Metadata:    playerMetadata,
	}

	properties := map[string]string{}
	if record.Name.Err == nil && record.Name.Text != "" {
		properties["legacyRecordName"] = record.Name.Text
		if record.Name.Text != displayName {
			result.RecordNameMismatch = true
			result.Warnings = append(result.Warnings, Finding{
				Path:    displayPath,
				Message: fmt.Sprintf("creature record name %q does not match filename %q", record.Name.Text, displayName),
			})
		}
	}
	if record.Talk.Err == nil && record.Talk.Text != "" {
		properties["legacyTalk"] = record.Talk.Text
	}
	if record.Password.Err == nil && record.Password.Text != "" {
		properties["legacyPasswordHash"] = record.Password.Text
	}
	if numbers.InventoryCount != 0 {
		properties["legacyInventoryObjectCount"] = fmt.Sprintf("%d", numbers.InventoryCount)
	}
	if len(properties) == 0 {
		properties = nil
	}

	result.Creature = model.Creature{
		ID:          creatureID,
		Kind:        model.CreatureKindPlayer,
		DisplayName: displayName,
		Description: textIfValid(record.Description),
		Level:       numbers.Level,
		RoomID:      roomID,
		PlayerID:    playerID,
		Stats:       numbers.statsMap(),
		Properties:  properties,
		Metadata: model.Metadata{
			Source:         legacySource,
			LegacyKind:     "player.creature",
			LegacyID:       displayName,
			LegacyPath:     displayPath,
			LegacyEncoding: EncodingLegacyKR,
			RawFields:      rawFields,
		},
	}

	if opts.IncludeObjects {
		for i, item := range node.Inventory {
			prefix := fmt.Sprintf("player:%s:inventory:%d", displayName, i)
			objectResult := objectmap.MapObjectTreeWithOptions(prefix, model.ObjectLocation{CreatureID: creatureID, Slot: "inventory"}, item, objectmap.Options{
				PrototypeResolver: opts.PrototypeResolver,
				SourcePath:        displayPath,
			})
			objects := objectResult.Objects
			if len(objects) == 0 {
				continue
			}
			result.Creature.Inventory.ObjectIDs = append(result.Creature.Inventory.ObjectIDs, objects[0].ID)
			result.Objects = append(result.Objects, objects...)
			result.PrototypeResolution.ResolvedExact += objectResult.PrototypeResolution.ResolvedExact
			result.PrototypeResolution.Synthetic += objectResult.PrototypeResolution.Synthetic
			result.PrototypeResolution.AmbiguousSynthetic += objectResult.PrototypeResolution.AmbiguousSynthetic
			for _, warning := range objectResult.Warnings {
				result.Warnings = append(result.Warnings, Finding{
					Path:    displayPath,
					Message: fmt.Sprintf("player inventory[%d]: %s", i, warning),
				})
			}
		}
	}

	return result, nil
}

func (r *Result) addTextWarnings(path, field string, text cbin.TextField) {
	if text.Err == nil {
		return
	}
	r.Warnings = append(r.Warnings, Finding{
		Path:    path,
		Message: fmt.Sprintf("%s decode failed: %v", field, text.Err),
	})
}

func decodePathComponent(component, field string) decodedComponent {
	raw := []byte(component)
	info := decodedComponent{
		Text:     component,
		Encoding: EncodingUTF8,
		Raw:      cloneBytes(raw),
	}
	if component == "" {
		return info
	}

	utf8Valid := utf8.Valid(raw)
	if utf8Valid && krtext.IsLegacyName(component) {
		return info
	}

	legacyText, legacyErr := legacykr.DecodeEUCKRContext(legacykr.Context{Path: component, Field: field}, raw)
	if legacyErr == nil && isNonEmptyHangul(legacyText) {
		info.Text = legacyText
		info.Encoding = EncodingLegacyKR
		return info
	}
	if utf8Valid {
		return info
	}
	if legacyErr == nil {
		info.Text = legacyText
		info.Encoding = EncodingLegacyKR
		return info
	}

	info.Text = rawID(field, raw)
	info.Encoding = EncodingInvalid
	info.Warning = fmt.Sprintf("%s decode failed: %v", field, legacyErr)
	return info
}

func isNonEmptyHangul(s string) bool {
	return s != "" && krtext.IsAllHangulSyllables(s)
}

func displayPath(path string) string {
	if path == "" {
		return ""
	}
	slashed := filepath.ToSlash(path)
	parts := strings.Split(slashed, "/")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = decodePathComponent(part, "path").Text
	}
	return strings.Join(parts, "/")
}

func textIfValid(text cbin.TextField) string {
	if text.Err != nil {
		return ""
	}
	return text.Text
}

func readCreatureNumbers(data []byte, record cbin.CreatureRecord) creatureNumbers {
	return creatureNumbers{
		Level:          int(readUint8(data, creatureLevelOff)),
		Type:           int(readInt8(data, creatureTypeOff)),
		Class:          int(readInt8(data, creatureClassOff)),
		Race:           int(readInt8(data, creatureRaceOff)),
		NumWander:      int(readInt8(data, creatureNumWanderOff)),
		Alignment:      int(readInt16(data, creatureAlignmentOff)),
		Strength:       int(readInt8(data, creatureStrengthOff)),
		Dexterity:      int(readInt8(data, creatureDexterityOff)),
		Constitution:   int(readInt8(data, creatureConstitutionOff)),
		Intelligence:   int(readInt8(data, creatureIntelligenceOff)),
		Piety:          int(readInt8(data, creaturePietyOff)),
		HPMax:          int(readInt16(data, creatureHPMaxOff)),
		HPCurrent:      int(readInt16(data, creatureHPCurOff)),
		MPMax:          int(readInt16(data, creatureMPMaxOff)),
		MPCurrent:      int(readInt16(data, creatureMPCurOff)),
		Armor:          int(readInt8(data, creatureArmorOff)),
		Thaco:          int(readInt8(data, creatureThacoOff)),
		Experience:     int(readInt32(data, creatureExperienceOff)),
		Gold:           int(readInt32(data, creatureGoldOff)),
		NDice:          int(readInt16(data, creatureNDiceOff)),
		SDice:          int(readInt16(data, creatureSDiceOff)),
		PDice:          int(readInt16(data, creaturePDiceOff)),
		Special:        int(readInt16(data, creatureSpecialOff)),
		QuestNumber:    int(readInt8(data, creatureQuestNumOff)),
		RoomNumber:     int(readInt16(data, creatureRoomNumberOff)),
		InventoryCount: inventoryCount(data),
		Invisible:      legacyFlagValue(record.Flags, legacyPINVIS),
		DMInvisible:    legacyFlagValue(record.Flags, legacyPDMINV),
		Male:           legacyFlagValue(record.Flags, legacyPMALES),
		DetectInvis:    legacyFlagValue(record.Flags, legacyPDINVI),
		Blind:          legacyFlagValue(record.Flags, legacyPBLIND),
		Silenced:       legacyFlagValue(record.Flags, legacyPSILNC),
		Suicide:        legacyFlagValue(record.Flags, legacySUICD),
		HoursInterval:  legacyLasttimeInterval(data, legacyLTHOURS),
		AgeYears:       18 + legacyLasttimeInterval(data, legacyLTHOURS)/86400,
		FamilyFlag:     legacyFlagValue(record.Flags, legacyPFAMIL),
		FamilyID:       int(record.Daily[legacyDLEXPND].Max),
		MarriageID:     int(record.Daily[legacyDLMARRI].Max),
	}
}

func (n creatureNumbers) statsMap() map[string]int {
	return map[string]int{
		"legacyType":          n.Type,
		"class":               n.Class,
		"race":                n.Race,
		"numWander":           n.NumWander,
		"alignment":           n.Alignment,
		"strength":            n.Strength,
		"dexterity":           n.Dexterity,
		"constitution":        n.Constitution,
		"intelligence":        n.Intelligence,
		"piety":               n.Piety,
		"hpMax":               n.HPMax,
		"hpCurrent":           n.HPCurrent,
		"mpMax":               n.MPMax,
		"mpCurrent":           n.MPCurrent,
		"armor":               n.Armor,
		"thaco":               n.Thaco,
		"experience":          n.Experience,
		"gold":                n.Gold,
		"nDice":               n.NDice,
		"sDice":               n.SDice,
		"pDice":               n.PDice,
		"special":             n.Special,
		"questNumber":         n.QuestNumber,
		"roomNumber":          n.RoomNumber,
		"inventoryObjects":    n.InventoryCount,
		"PINVIS":              n.Invisible,
		"PDMINV":              n.DMInvisible,
		"PMALES":              n.Male,
		"PDINVI":              n.DetectInvis,
		"PBLIND":              n.Blind,
		"PSILNC":              n.Silenced,
		"SUICD":               n.Suicide,
		"legacyHoursInterval": n.HoursInterval,
		"legacyAgeYears":      n.AgeYears,
		"familyFlag":          n.FamilyFlag,
		"familyID":            n.FamilyID,
		"marriageID":          n.MarriageID,
	}
}

func roomID(number int) (model.RoomID, string) {
	if number < 0 {
		return "", fmt.Sprintf("legacy room number %d is negative; roomId omitted", number)
	}
	return model.RoomID(fmt.Sprintf("r%05d", number)), ""
}

func inventoryCount(data []byte) int {
	off := cbin.CreatureSize
	if off+4 > len(data) {
		return 0
	}
	return int(int32(binary.LittleEndian.Uint32(data[off : off+4])))
}

func legacyLasttimeInterval(data []byte, index int) int {
	off := creatureLasttimeOff + index*cbin.LasttimeSize + lasttimeIntervalOff
	if off+4 > len(data) {
		return 0
	}
	return int(readInt32(data, off))
}

func readUint8(data []byte, off int) uint8 {
	if off >= len(data) {
		return 0
	}
	return data[off]
}

func readInt8(data []byte, off int) int8 {
	return int8(readUint8(data, off))
}

func readInt16(data []byte, off int) int16 {
	if off+2 > len(data) {
		return 0
	}
	return int16(binary.LittleEndian.Uint16(data[off : off+2]))
}

func readInt32(data []byte, off int) int32 {
	if off+4 > len(data) {
		return 0
	}
	return int32(binary.LittleEndian.Uint32(data[off : off+4]))
}

func legacyFlagValue(flags [8]byte, bit int) int {
	if flags[bit/8]&(1<<uint(bit%8)) == 0 {
		return 0
	}
	return 1
}

func addRawField(fields map[string][]byte, key string, value []byte) {
	if len(value) == 0 {
		return
	}
	fields[key] = cloneBytes(value)
}

func cloneBytes(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	out := make([]byte, len(value))
	copy(out, value)
	return out
}

func rawID(field string, raw []byte) string {
	normalized := strings.NewReplacer(" ", "-", ".", "-", "/", "-").Replace(field)
	return "raw-" + normalized + "-" + hex.EncodeToString(raw)
}
