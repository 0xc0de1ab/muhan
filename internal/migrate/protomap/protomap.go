package protomap

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/0xc0de1ab/muhan/internal/persist/cbin"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const legacySource = "legacy"

type Options struct {
	Root string
}

type Snapshot struct {
	Root               string                    `json:"root"`
	Counts             Counts                    `json:"counts"`
	ObjectPrototypes   []model.ObjectPrototype   `json:"objectPrototypes"`
	CreaturePrototypes []CreaturePrototypeRecord `json:"creaturePrototypes"`
	Warnings           []Finding                 `json:"warnings"`
	Errors             []Finding                 `json:"errors"`
}

type Counts struct {
	ObjectPrototypeFiles   int `json:"objectPrototypeFiles"`
	ObjectPrototypes       int `json:"objectPrototypes"`
	CreaturePrototypeFiles int `json:"creaturePrototypeFiles"`
	CreaturePrototypes     int `json:"creaturePrototypes"`
	SkippedFiles           int `json:"skippedFiles"`
	TotalPrototypes        int `json:"totalPrototypes"`
}

type Finding struct {
	Path    string `json:"path,omitempty"`
	ID      string `json:"id,omitempty"`
	Message string `json:"message"`
}

type CreaturePrototypeRecord struct {
	ID          model.CreatureID   `json:"id"`
	Kind        model.CreatureKind `json:"kind,omitempty"`
	DisplayName string             `json:"displayName"`
	Description string             `json:"description,omitempty"`
	Level       int                `json:"level,omitempty"`
	Talk        string             `json:"talk,omitempty"`
	Keywords    []string           `json:"keywords,omitempty"`
	Stats       map[string]int     `json:"stats,omitempty"`
	Properties  map[string]string  `json:"properties,omitempty"`
	Metadata    model.Metadata     `json:"metadata,omitempty"`
}

type Report struct {
	Root     string    `json:"root"`
	Counts   Counts    `json:"counts"`
	Warnings []Finding `json:"warnings"`
	Errors   []Finding `json:"errors"`
}

func Build(opts Options) (Snapshot, error) {
	root := opts.Root
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve root: %w", err)
	}

	snapshot := Snapshot{
		Root:     absRoot,
		Warnings: []Finding{},
		Errors:   []Finding{},
	}

	dir := filepath.Join(absRoot, "objmon")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			snapshot.addWarning(displayPath(absRoot, dir), "", "objmon directory not found")
			return snapshot, nil
		}
		snapshot.addError(displayPath(absRoot, dir), "", err)
		return snapshot, nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(dir, name)
		relPath := displayPath(absRoot, path)

		switch {
		case objectProtoRE.MatchString(name):
			snapshot.mapObjectFile(name, path, relPath)
		case creatureProtoRE.MatchString(name):
			snapshot.mapCreatureFile(name, path, relPath)
		default:
			snapshot.Counts.SkippedFiles++
		}
	}

	snapshot.Counts.TotalPrototypes = snapshot.Counts.ObjectPrototypes + snapshot.Counts.CreaturePrototypes
	return snapshot, nil
}

func (s Snapshot) Report() Report {
	return Report{
		Root:     s.Root,
		Counts:   s.Counts,
		Warnings: s.Warnings,
		Errors:   s.Errors,
	}
}

func WriteReportJSON(w io.Writer, opts Options) error {
	snapshot, err := Build(opts)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(snapshot.Report())
}

var (
	objectProtoRE   = regexp.MustCompile(`^o[0-9][0-9]$`)
	creatureProtoRE = regexp.MustCompile(`^m[0-9][0-9]$`)
)

func (s *Snapshot) mapObjectFile(fileName, path, relPath string) {
	data, err := os.ReadFile(path)
	if err != nil {
		s.addError(relPath, "", err)
		return
	}
	records, err := cbin.DecodeObjectRecords(data)
	if err != nil {
		s.addError(relPath, "", fmt.Errorf("decode object prototype records: %w", err))
		return
	}

	s.Counts.ObjectPrototypeFiles++
	s.Counts.ObjectPrototypes += len(records)
	for i, record := range records {
		s.ObjectPrototypes = append(s.ObjectPrototypes, s.mapObjectRecord(fileName, relPath, i, record))
	}
}

func (s *Snapshot) mapCreatureFile(fileName, path, relPath string) {
	data, err := os.ReadFile(path)
	if err != nil {
		s.addError(relPath, "", err)
		return
	}
	records, err := cbin.DecodeCreatureRecords(data)
	if err != nil {
		s.addError(relPath, "", fmt.Errorf("decode creature prototype records: %w", err))
		return
	}

	s.Counts.CreaturePrototypeFiles++
	s.Counts.CreaturePrototypes += len(records)
	for i, record := range records {
		s.CreaturePrototypes = append(s.CreaturePrototypes, s.mapCreatureRecord(fileName, relPath, i, record))
	}
}

func (s *Snapshot) mapObjectRecord(fileName, relPath string, index int, record cbin.ObjectRecord) model.ObjectPrototype {
	id := fmt.Sprintf("object:%s:%d", fileName, index)
	s.warnTextErr(relPath, id, "object.name", record.Name)
	s.warnTextErr(relPath, id, "object.description", record.Description)
	s.warnTextErr(relPath, id, "object.useOutput", record.UseOutput)

	displayName := usableText(record.Name)
	if displayName == "" {
		displayName = id
	}

	keywords := make([]string, 0, len(record.Keys))
	rawFields := rawFieldMap(
		rawField{"name", record.Name},
		rawField{"description", record.Description},
		rawField{"useOutput", record.UseOutput},
	)
	addRawInt32Field(rawFields, "value", record.Value)
	addRawInt16Field(rawFields, "weight", record.Weight)
	addRawInt8Field(rawFields, "type", record.Type)
	addRawInt8Field(rawFields, "adjustment", record.Adjustment)
	addRawInt16Field(rawFields, "shotsMax", record.ShotsMax)
	addRawInt16Field(rawFields, "shotsCurrent", record.ShotsCurrent)
	addRawInt16Field(rawFields, "nDice", record.NDice)
	addRawInt16Field(rawFields, "sDice", record.SDice)
	addRawInt16Field(rawFields, "pDice", record.PDice)
	addRawInt8Field(rawFields, "armor", record.Armor)
	addRawInt8Field(rawFields, "wearFlag", record.WearFlag)
	addRawInt8Field(rawFields, "magicPower", record.MagicPower)
	addRawInt8Field(rawFields, "magicRealm", record.MagicRealm)
	addRawInt16Field(rawFields, "special", record.Special)
	addRawBytesField(rawFields, "flags", record.Flags[:])
	addRawInt8Field(rawFields, "questNumber", record.QuestNumber)
	for i, key := range record.Keys {
		fieldName := fmt.Sprintf("key[%d]", i)
		s.warnTextErr(relPath, id, "object."+fieldName, key)
		if text := usableText(key); text != "" {
			keywords = append(keywords, text)
		}
		addRawField(rawFields, fieldName, key)
	}

	properties := map[string]string{}
	if text := usableText(record.UseOutput); text != "" {
		properties["useOutput"] = text
	}
	addInt32Property(properties, "value", record.Value)
	if record.Weight != 0 {
		properties["weight"] = strconv.Itoa(int(record.Weight))
	}
	addInt8Property(properties, "type", record.Type)
	addInt8Property(properties, "adjustment", record.Adjustment)
	addInt16Property(properties, "shotsMax", record.ShotsMax)
	addInt16Property(properties, "shotsCurrent", record.ShotsCurrent)
	addInt16Property(properties, "nDice", record.NDice)
	addInt16Property(properties, "sDice", record.SDice)
	addInt16Property(properties, "pDice", record.PDice)
	addInt8Property(properties, "armor", record.Armor)
	addInt8Property(properties, "wearFlag", record.WearFlag)
	addInt8Property(properties, "magicPower", record.MagicPower)
	addInt8Property(properties, "magicRealm", record.MagicRealm)
	addInt16Property(properties, "special", record.Special)
	addInt8Property(properties, "questNumber", record.QuestNumber)

	return model.ObjectPrototype{
		ID:          model.PrototypeID(id),
		Kind:        objectKind(record.Type),
		DisplayName: displayName,
		Description: usableText(record.Description),
		Keywords:    keywords,
		Properties:  nilIfEmpty(properties),
		Metadata: model.Metadata{
			Source:         legacySource,
			LegacyKind:     "objectPrototype",
			LegacyID:       fmt.Sprintf("%s:%d", fileName, index),
			LegacyPath:     relPath,
			LegacyEncoding: "euc-kr/cp949",
			RecordIndex:    index,
			RecordOffset:   int64(index * cbin.ObjectSize),
			RawFields:      nilIfEmptyBytes(rawFields),
			Tags:           objectFlagNames(record.Flags),
		},
	}
}

func (s *Snapshot) mapCreatureRecord(fileName, relPath string, index int, record cbin.CreatureRecord) CreaturePrototypeRecord {
	id := fmt.Sprintf("creature:%s:%d", fileName, index)
	s.warnTextErr(relPath, id, "creature.name", record.Name)
	s.warnTextErr(relPath, id, "creature.description", record.Description)
	s.warnTextErr(relPath, id, "creature.talk", record.Talk)
	s.warnTextErr(relPath, id, "creature.password", record.Password)

	displayName := usableText(record.Name)
	if displayName == "" {
		displayName = id
	}

	keywords := make([]string, 0, len(record.Keys))
	rawFields := rawFieldMap(
		rawField{"name", record.Name},
		rawField{"description", record.Description},
		rawField{"talk", record.Talk},
		rawField{"password", record.Password},
	)
	nums := readCreaturePrototypeNumbers(record.Raw)
	addRawInt8Field(rawFields, "level", int8(nums.Level))
	addRawInt8Field(rawFields, "legacyType", int8(nums.Type))
	addRawInt8Field(rawFields, "class", int8(nums.Class))
	addRawInt8Field(rawFields, "race", int8(nums.Race))
	addRawInt8Field(rawFields, "numWander", int8(nums.NumWander))
	addRawInt16Field(rawFields, "alignment", int16(nums.Alignment))
	addRawInt8Field(rawFields, "strength", int8(nums.Strength))
	addRawInt8Field(rawFields, "dexterity", int8(nums.Dexterity))
	addRawInt8Field(rawFields, "constitution", int8(nums.Constitution))
	addRawInt8Field(rawFields, "intelligence", int8(nums.Intelligence))
	addRawInt8Field(rawFields, "piety", int8(nums.Piety))
	addRawInt16Field(rawFields, "hpMax", int16(nums.HPMax))
	addRawInt16Field(rawFields, "hpCurrent", int16(nums.HPCurrent))
	addRawInt16Field(rawFields, "mpMax", int16(nums.MPMax))
	addRawInt16Field(rawFields, "mpCurrent", int16(nums.MPCurrent))
	addRawInt8Field(rawFields, "armor", int8(nums.Armor))
	addRawInt8Field(rawFields, "thaco", int8(nums.Thaco))
	addRawInt32Field(rawFields, "experience", int32(nums.Experience))
	addRawInt32Field(rawFields, "gold", int32(nums.Gold))
	addRawInt16Field(rawFields, "nDice", int16(nums.NDice))
	addRawInt16Field(rawFields, "sDice", int16(nums.SDice))
	addRawInt16Field(rawFields, "pDice", int16(nums.PDice))
	addRawInt16Field(rawFields, "special", int16(nums.Special))
	addRawInt8Field(rawFields, "questNumber", int8(nums.QuestNumber))
	addRawInt16Field(rawFields, "roomNumber", int16(nums.RoomNumber))
	addRawBytesField(rawFields, "spells", record.Spells[:])
	addRawBytesField(rawFields, "flags", record.Flags[:])
	stats := nums.statsMap()
	for i, carry := range record.Carry {
		if carry == 0 {
			continue
		}
		stats[fmt.Sprintf("carry[%d]", i)] = int(carry)
		addRawInt16Field(rawFields, fmt.Sprintf("carry[%d]", i), carry)
	}
	for i, key := range record.Keys {
		fieldName := fmt.Sprintf("key[%d]", i)
		s.warnTextErr(relPath, id, "creature."+fieldName, key)
		if text := usableText(key); text != "" {
			keywords = append(keywords, text)
		}
		addRawField(rawFields, fieldName, key)
	}

	properties := map[string]string{}
	if text := usableText(record.Password); text != "" {
		properties["password"] = text
	}

	return CreaturePrototypeRecord{
		ID:          model.CreatureID(id),
		Kind:        model.CreatureKindMonster,
		DisplayName: displayName,
		Description: usableText(record.Description),
		Level:       nums.Level,
		Talk:        usableText(record.Talk),
		Keywords:    keywords,
		Stats:       nilIfEmptyInt(stats),
		Properties:  nilIfEmpty(properties),
		Metadata: model.Metadata{
			Source:         legacySource,
			LegacyKind:     "creaturePrototype",
			LegacyID:       fmt.Sprintf("%s:%d", fileName, index),
			LegacyPath:     relPath,
			LegacyEncoding: "euc-kr/cp949",
			RecordIndex:    index,
			RecordOffset:   int64(index * cbin.CreatureSize),
			RawFields:      nilIfEmptyBytes(rawFields),
			Tags:           creatureFlagNames(record.Flags),
		},
	}
}

func (s *Snapshot) warnTextErr(path, id, field string, text cbin.TextField) {
	if text.Err != nil {
		s.addWarning(path, id, fmt.Sprintf("%s decode failed: %v", field, text.Err))
	}
}

func (s *Snapshot) addWarning(path, id, message string) {
	s.Warnings = append(s.Warnings, Finding{Path: path, ID: id, Message: message})
}

func (s *Snapshot) addError(path, id string, err error) {
	s.Errors = append(s.Errors, Finding{Path: path, ID: id, Message: err.Error()})
}

func usableText(text cbin.TextField) string {
	if text.Err != nil {
		return ""
	}
	return text.Text
}

type rawField struct {
	name string
	text cbin.TextField
}

func rawFieldMap(fields ...rawField) map[string][]byte {
	out := make(map[string][]byte, len(fields))
	for _, field := range fields {
		addRawField(out, field.name, field.text)
	}
	return out
}

func addRawField(fields map[string][]byte, name string, text cbin.TextField) {
	if len(text.Raw) == 0 {
		return
	}
	raw := make([]byte, len(text.Raw))
	copy(raw, text.Raw)
	fields[name] = raw
}

func addInt32Property(props map[string]string, name string, value int32) {
	if value != 0 {
		props[name] = strconv.FormatInt(int64(value), 10)
	}
}

func addInt16Property(props map[string]string, name string, value int16) {
	if value != 0 {
		props[name] = strconv.Itoa(int(value))
	}
}

func addInt8Property(props map[string]string, name string, value int8) {
	if value != 0 {
		props[name] = strconv.Itoa(int(value))
	}
}

func addRawInt32Field(fields map[string][]byte, name string, value int32) {
	if value == 0 {
		return
	}
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], uint32(value))
	fields[name] = raw[:]
}

func addRawInt16Field(fields map[string][]byte, name string, value int16) {
	if value == 0 {
		return
	}
	var raw [2]byte
	binary.LittleEndian.PutUint16(raw[:], uint16(value))
	fields[name] = raw[:]
}

func addRawInt8Field(fields map[string][]byte, name string, value int8) {
	if value == 0 {
		return
	}
	fields[name] = []byte{byte(value)}
}

func addRawBytesField(fields map[string][]byte, name string, raw []byte) {
	for _, b := range raw {
		if b != 0 {
			fields[name] = append([]byte(nil), raw...)
			return
		}
	}
}

func objectFlagNames(flags [8]byte) []string {
	names := []string{
		"", "", "", "", "", "", "", "weightless",
		"", "", "", "", "", "", "", "",
		"", "", "", "", "", "", "", "",
		"", "", "", "", "damageDice", "", "", "classSelective",
		"classAssassin", "classBarbarian", "classCleric", "classFighter",
		"classMage", "classPaladin", "classRanger", "classThief",
		"stunDice", "neverShatter", "alwaysCritical", "customName",
		"specialItem", "marriageOnly", "eventItem", "named",
		"noBurn", "wheld",
	}

	out := make([]string, 0, len(names))
	for bit, name := range names {
		if name == "" {
			continue
		}
		if flags[bit/8]&(1<<uint(bit%8)) != 0 {
			out = append(out, name)
		}
	}
	return out
}

type creaturePrototypeNumbers struct {
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

const (
	creatureProtoLevelOff        = 318
	creatureProtoTypeOff         = 319
	creatureProtoClassOff        = 320
	creatureProtoRaceOff         = 321
	creatureProtoNumWanderOff    = 322
	creatureProtoAlignmentOff    = 324
	creatureProtoStrengthOff     = 326
	creatureProtoDexterityOff    = 327
	creatureProtoConstitutionOff = 328
	creatureProtoIntelligenceOff = 329
	creatureProtoPietyOff        = 330
	creatureProtoHPMaxOff        = 332
	creatureProtoHPCurOff        = 334
	creatureProtoMPMaxOff        = 336
	creatureProtoMPCurOff        = 338
	creatureProtoArmorOff        = 340
	creatureProtoThacoOff        = 341
	creatureProtoExperienceOff   = 344
	creatureProtoGoldOff         = 348
	creatureProtoNDiceOff        = 352
	creatureProtoSDiceOff        = 354
	creatureProtoPDiceOff        = 356
	creatureProtoSpecialOff      = 358
	creatureProtoQuestNumberOff  = 436
	creatureProtoRoomNumberOff   = 458
)

func readCreaturePrototypeNumbers(data []byte) creaturePrototypeNumbers {
	return creaturePrototypeNumbers{
		Level:        int(readUint8At(data, creatureProtoLevelOff)),
		Type:         int(readInt8At(data, creatureProtoTypeOff)),
		Class:        int(readInt8At(data, creatureProtoClassOff)),
		Race:         int(readInt8At(data, creatureProtoRaceOff)),
		NumWander:    int(readInt8At(data, creatureProtoNumWanderOff)),
		Alignment:    int(readInt16At(data, creatureProtoAlignmentOff)),
		Strength:     int(readInt8At(data, creatureProtoStrengthOff)),
		Dexterity:    int(readInt8At(data, creatureProtoDexterityOff)),
		Constitution: int(readInt8At(data, creatureProtoConstitutionOff)),
		Intelligence: int(readInt8At(data, creatureProtoIntelligenceOff)),
		Piety:        int(readInt8At(data, creatureProtoPietyOff)),
		HPMax:        int(readInt16At(data, creatureProtoHPMaxOff)),
		HPCurrent:    int(readInt16At(data, creatureProtoHPCurOff)),
		MPMax:        int(readInt16At(data, creatureProtoMPMaxOff)),
		MPCurrent:    int(readInt16At(data, creatureProtoMPCurOff)),
		Armor:        int(readInt8At(data, creatureProtoArmorOff)),
		Thaco:        int(readInt8At(data, creatureProtoThacoOff)),
		Experience:   int(readInt32At(data, creatureProtoExperienceOff)),
		Gold:         int(readInt32At(data, creatureProtoGoldOff)),
		NDice:        int(readInt16At(data, creatureProtoNDiceOff)),
		SDice:        int(readInt16At(data, creatureProtoSDiceOff)),
		PDice:        int(readInt16At(data, creatureProtoPDiceOff)),
		Special:      int(readInt16At(data, creatureProtoSpecialOff)),
		QuestNumber:  int(readInt8At(data, creatureProtoQuestNumberOff)),
		RoomNumber:   int(readInt16At(data, creatureProtoRoomNumberOff)),
	}
}

func (n creaturePrototypeNumbers) statsMap() map[string]int {
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
		if bit/8 >= len(flags) {
			break
		}
		if flags[bit/8]&(1<<uint(bit%8)) != 0 {
			out = append(out, legacyNames[bit], alias)
		}
	}
	return out
}

func readUint8At(data []byte, off int) uint8 {
	if off >= len(data) {
		return 0
	}
	return data[off]
}

func readInt8At(data []byte, off int) int8 {
	return int8(readUint8At(data, off))
}

func readInt16At(data []byte, off int) int16 {
	if off+2 > len(data) {
		return 0
	}
	return int16(binary.LittleEndian.Uint16(data[off : off+2]))
}

func readInt32At(data []byte, off int) int32 {
	if off+4 > len(data) {
		return 0
	}
	return int32(binary.LittleEndian.Uint32(data[off : off+4]))
}

func objectKind(legacyType int8) model.ObjectKind {
	switch legacyType {
	case 0, 1, 2, 3, 4:
		return model.ObjectKindWeapon
	case 5:
		return model.ObjectKindArmor
	case 6:
		return model.ObjectKindPotion
	case 7:
		return model.ObjectKindScroll
	case 8:
		return model.ObjectKindWand
	case 9, 14:
		return model.ObjectKindContainer
	case 10:
		return model.ObjectKindMoney
	case 11:
		return model.ObjectKindKey
	case 12:
		return model.ObjectKindLightSource
	default:
		return model.ObjectKindMisc
	}
}

func nilIfEmpty(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	return m
}

func nilIfEmptyInt(m map[string]int) map[string]int {
	if len(m) == 0 {
		return nil
	}
	return m
}

func nilIfEmptyBytes(m map[string][]byte) map[string][]byte {
	if len(m) == 0 {
		return nil
	}
	return m
}

func displayPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
