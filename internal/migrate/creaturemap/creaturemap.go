package creaturemap

import (
	"encoding/binary"
	"fmt"
	"strings"

	"muhan/internal/migrate/objectmap"
	"muhan/internal/migrate/protoresolve"
	"muhan/internal/persist/cbin"
	"muhan/internal/world/model"
)

const (
	legacySource   = "legacy"
	legacyEncoding = "euc-kr/cp949"

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
)

type Result struct {
	Creature            model.Creature                      `json:"creature"`
	Objects             []model.ObjectInstance              `json:"objects,omitempty"`
	Warnings            []string                            `json:"warnings,omitempty"`
	PrototypeResolution objectmap.PrototypeResolutionCounts `json:"prototypeResolution"`
}

type Options struct {
	PrototypeResolver protoresolve.ObjectPrototypeResolver
	SourcePath        string
}

type numbers struct {
	Level        int
	Type         int
	Class        int
	Race         int
	NumWander    int
	Alignment    int
	Strength     int
	Dexterity    int
	Constitution int
	Intelligence int
	Piety        int
	HPMax        int
	HPCurrent    int
	MPMax        int
	MPCurrent    int
	Armor        int
	Thaco        int
	Experience   int
	Gold         int
	NDice        int
	SDice        int
	PDice        int
	Special      int
	QuestNumber  int
	RoomNumber   int
}

func MapCreatureTree(rootIDPrefix string, roomID model.RoomID, node cbin.CreatureNode) Result {
	return MapCreatureTreeWithOptions(rootIDPrefix, roomID, node, Options{})
}

func MapCreatureTreeWithOptions(rootIDPrefix string, roomID model.RoomID, node cbin.CreatureNode, opts Options) Result {
	prefix := cleanPrefix(rootIDPrefix)
	id := model.CreatureID(fmt.Sprintf("creature:%s:%08d", prefix, node.Offset))
	record := node.Record
	nums := readNumbers(record.Raw)

	displayName := textIfValid(record.Name)
	if strings.TrimSpace(displayName) == "" {
		displayName = string(id)
	}

	properties := map[string]string{}
	if talk := textIfValid(record.Talk); talk != "" {
		properties["legacyTalk"] = talk
	}
	if password := textIfValid(record.Password); password != "" {
		properties["legacyPassword"] = password
	}
	keywords := keywords(record)
	if len(keywords) > 0 {
		properties["keywords"] = strings.Join(keywords, "\n")
	}
	if len(node.Inventory) != 0 {
		properties["legacyInventoryObjectCount"] = fmt.Sprintf("%d", len(node.Inventory))
	}
	if len(properties) == 0 {
		properties = nil
	}
	stats := nums.statsMap()
	for i, carry := range record.Carry {
		if carry != 0 {
			stats[fmt.Sprintf("carry[%d]", i)] = int(carry)
		}
	}

	result := Result{
		Creature: model.Creature{
			ID:          id,
			Kind:        model.CreatureKindMonster,
			DisplayName: displayName,
			Description: textIfValid(record.Description),
			Level:       nums.Level,
			RoomID:      roomID,
			Stats:       stats,
			Properties:  properties,
			Metadata: model.Metadata{
				Source:         legacySource,
				LegacyKind:     "room.creature",
				LegacyID:       string(id),
				LegacyEncoding: legacyEncoding,
				RecordOffset:   int64(node.Offset),
				RawFields:      rawFields(record),
				Tags:           creatureFlagNames(record.Flags),
			},
		},
		Warnings: treeWarnings(node.Warnings),
	}

	for i, item := range node.Inventory {
		objectPrefix := fmt.Sprintf("%s:creature:%08d:inventory:%d", prefix, node.Offset, i)
		objectResult := objectmap.MapObjectTreeWithOptions(objectPrefix, model.ObjectLocation{CreatureID: id}, item, objectmap.Options{
			PrototypeResolver: opts.PrototypeResolver,
			SourcePath:        opts.SourcePath,
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
	}

	return result
}

func cleanPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.Trim(prefix, ":/")
	if prefix == "" {
		return "creature"
	}
	return prefix
}

func keywords(record cbin.CreatureRecord) []string {
	out := make([]string, 0, len(record.Keys))
	for _, key := range record.Keys {
		if text := textIfValid(key); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func rawFields(record cbin.CreatureRecord) map[string][]byte {
	fields := map[string][]byte{}
	addRawField(fields, "name", record.Name)
	addRawField(fields, "description", record.Description)
	addRawField(fields, "talk", record.Talk)
	addRawField(fields, "password", record.Password)
	for i, key := range record.Keys {
		addRawField(fields, fmt.Sprintf("key[%d]", i), key)
	}
	addRawBytesField(fields, "spells", record.Spells[:])
	addRawBytesField(fields, "flags", record.Flags[:])
	for i, carry := range record.Carry {
		addRawInt16Field(fields, fmt.Sprintf("carry[%d]", i), carry)
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func addRawField(fields map[string][]byte, name string, field cbin.TextField) {
	if len(field.Raw) == 0 {
		return
	}
	raw := make([]byte, len(field.Raw))
	copy(raw, field.Raw)
	fields[name] = raw
}

func addRawInt16Field(fields map[string][]byte, name string, value int16) {
	if value == 0 {
		return
	}
	var raw [2]byte
	binary.LittleEndian.PutUint16(raw[:], uint16(value))
	fields[name] = raw[:]
}

func addRawBytesField(fields map[string][]byte, name string, raw []byte) {
	for _, b := range raw {
		if b != 0 {
			fields[name] = append([]byte(nil), raw...)
			return
		}
	}
}

func textIfValid(field cbin.TextField) string {
	if field.Err != nil {
		return ""
	}
	return field.Text
}

func treeWarnings(warnings []cbin.TreeWarning) []string {
	if len(warnings) == 0 {
		return nil
	}
	out := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, fmt.Sprintf("creature tree offset %d %s decode failed: %s", warning.Offset, warning.Field, warning.Message))
	}
	return out
}

func readNumbers(data []byte) numbers {
	return numbers{
		Level:        int(readUint8(data, creatureLevelOff)),
		Type:         int(readInt8(data, creatureTypeOff)),
		Class:        int(readInt8(data, creatureClassOff)),
		Race:         int(readInt8(data, creatureRaceOff)),
		NumWander:    int(readInt8(data, creatureNumWanderOff)),
		Alignment:    int(readInt16(data, creatureAlignmentOff)),
		Strength:     int(readInt8(data, creatureStrengthOff)),
		Dexterity:    int(readInt8(data, creatureDexterityOff)),
		Constitution: int(readInt8(data, creatureConstitutionOff)),
		Intelligence: int(readInt8(data, creatureIntelligenceOff)),
		Piety:        int(readInt8(data, creaturePietyOff)),
		HPMax:        int(readInt16(data, creatureHPMaxOff)),
		HPCurrent:    int(readInt16(data, creatureHPCurOff)),
		MPMax:        int(readInt16(data, creatureMPMaxOff)),
		MPCurrent:    int(readInt16(data, creatureMPCurOff)),
		Armor:        int(readInt8(data, creatureArmorOff)),
		Thaco:        int(readInt8(data, creatureThacoOff)),
		Experience:   int(readInt32(data, creatureExperienceOff)),
		Gold:         int(readInt32(data, creatureGoldOff)),
		NDice:        int(readInt16(data, creatureNDiceOff)),
		SDice:        int(readInt16(data, creatureSDiceOff)),
		PDice:        int(readInt16(data, creaturePDiceOff)),
		Special:      int(readInt16(data, creatureSpecialOff)),
		QuestNumber:  int(readInt8(data, creatureQuestNumOff)),
		RoomNumber:   int(readInt16(data, creatureRoomNumberOff)),
	}
}

func (n numbers) statsMap() map[string]int {
	return map[string]int{
		"legacyType":   n.Type,
		"class":        n.Class,
		"race":         n.Race,
		"numWander":    n.NumWander,
		"alignment":    n.Alignment,
		"strength":     n.Strength,
		"dexterity":    n.Dexterity,
		"constitution": n.Constitution,
		"intelligence": n.Intelligence,
		"piety":        n.Piety,
		"hpMax":        n.HPMax,
		"hpCurrent":    n.HPCurrent,
		"mpMax":        n.MPMax,
		"mpCurrent":    n.MPCurrent,
		"armor":        n.Armor,
		"thaco":        n.Thaco,
		"experience":   n.Experience,
		"gold":         n.Gold,
		"nDice":        n.NDice,
		"sDice":        n.SDice,
		"pDice":        n.PDice,
		"special":      n.Special,
		"questNumber":  n.QuestNumber,
		"roomNumber":   n.RoomNumber,
	}
}

func creatureFlagNames(flags [8]byte) []string {
	legacyNames := []string{
		"MPERMT", "MHIDDN", "MINVIS", "MTOMEN", "MDROPS", "MNOPRE", "MAGGRE", "MGUARD",
		"MBLOCK", "MFOLLO", "MFLEER", "MSCAVE", "MMALES", "MPOISS", "MUNDED", "MUNSTL",
		"MPOISN", "MMAGIC", "MHASSC", "MBRETH", "MMGONL", "MDINVI", "MENONL", "MTALKS",
		"MUNKIL", "MNRGLD", "MTLKAG", "MRMAGI", "MBRWP1", "MBRWP2", "MENEDR", "MKNGDM",
		"MPLDGK", "MRSCND", "MDISEA", "MDISIT", "MPURIT", "MTRADE", "MPGUAR", "MGAGGR",
		"MEAGGR", "MDEATH", "MMAGIO", "MRBEFD", "MNOCIR", "MBLNDR", "MDMFOL", "MFEARS",
		"MSILNC", "MBLIND", "MCHARM", "MBEFUD", "MKNDM1", "MKNDM2", "MKNDM3", "MKNDM4",
		"MKING1", "MKING2", "MKING3", "MKING4", "MSAYTLK", "MSUMMO", "MNOCHA",
	}
	aliasNames := []string{
		"permanent", "hidden", "invisible", "manToMenPlural", "noPluralSuffix", "noPrefix", "aggressive", "guardTreasure",
		"blocksExits", "followsAttacker", "flees", "scavenger", "male", "poisoner", "undead", "cannotSteal",
		"poisoned", "magicUser", "hasScavenged", "breathWeapon", "magicOnly", "detectInvisible", "magicOrEnchantedOnly", "talks",
		"unkillable", "fixedGold", "talkAggressive", "resistMagic", "breathWeaponType1", "breathWeaponType2", "energyDrain", "kingdom",
		"pledgeKingdom", "rescindKingdom", "disease", "dissolveItems", "purchaseItems", "tradeItems", "passiveExitGuard", "goodAggressive",
		"evilAggressive", "deathDescription", "magicPercent", "resistStunOnly", "cannotCircle", "blind", "followDM", "fearful",
		"silenced", "blinded", "charmed", "befuddled", "kingdom1", "kingdom2", "kingdom3", "kingdom4", "king1",
		"king2", "king3", "king4", "sayTalk", "summoner", "noCharm",
	}
	out := make([]string, 0, len(aliasNames)*2)
	for bit, alias := range aliasNames {
		if flags[bit/8]&(1<<uint(bit%8)) != 0 {
			out = append(out, legacyNames[bit], alias)
		}
	}
	return out
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
