package roommap

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/migrate/creaturemap"
	"github.com/0xc0de1ab/muhan/internal/migrate/objectmap"
	"github.com/0xc0de1ab/muhan/internal/migrate/protoresolve"
	"github.com/0xc0de1ab/muhan/internal/persist/cbin"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const legacySource = "legacy"

var roomFileRE = regexp.MustCompile(`^r([0-9]{5})$`)

type recordDecoder interface {
	DecodeRoomRecord([]byte) (cbin.RoomRecord, error)
	DecodeRoomFileRecord([]byte) (cbin.RoomRecord, cbin.Stats, error)
	DecodeExitRecord([]byte) (cbin.ExitRecord, error)
}

type cbinDecoder struct{}

func (cbinDecoder) DecodeRoomRecord(data []byte) (cbin.RoomRecord, error) {
	return cbin.DecodeRoomRecord(data)
}

func (cbinDecoder) DecodeRoomFileRecord(data []byte) (cbin.RoomRecord, cbin.Stats, error) {
	return cbin.DecodeRoomFileRecord(data)
}

func (cbinDecoder) DecodeExitRecord(data []byte) (cbin.ExitRecord, error) {
	return cbin.DecodeExitRecord(data)
}

type Mapper struct {
	decoder recordDecoder
}

type Options struct {
	PrototypeResolver protoresolve.ObjectPrototypeResolver
}

type Bundle struct {
	Room                model.Room                          `json:"room"`
	Creatures           []model.Creature                    `json:"creatures,omitempty"`
	Objects             []model.ObjectInstance              `json:"objects,omitempty"`
	Decoded             cbin.Stats                          `json:"decoded"`
	ContentDecoded      bool                                `json:"contentDecoded"`
	ContentError        string                              `json:"contentError,omitempty"`
	Warnings            []string                            `json:"warnings,omitempty"`
	RootObjectIDs       []model.ObjectInstanceID            `json:"rootObjectIds,omitempty"`
	RootCreatureIDs     []model.CreatureID                  `json:"rootCreatureIds,omitempty"`
	PrototypeResolution objectmap.PrototypeResolutionCounts `json:"prototypeResolution"`
}

func NewMapper() Mapper {
	return Mapper{decoder: cbinDecoder{}}
}

func MapRoomFile(path string, data []byte) (model.Room, []string, error) {
	return NewMapper().MapRoomFile(path, data)
}

func MapRoomFileBundle(path string, data []byte) (Bundle, error) {
	return NewMapper().MapRoomFileBundle(path, data)
}

func MapRoomFileBundleWithOptions(path string, data []byte, opts Options) (Bundle, error) {
	return NewMapper().MapRoomFileBundleWithOptions(path, data, opts)
}

func (m Mapper) MapRoomFile(path string, data []byte) (model.Room, []string, error) {
	decoder := m.decoder
	if decoder == nil {
		decoder = cbinDecoder{}
	}

	header, err := decoder.DecodeRoomRecord(data)
	if err != nil {
		return model.Room{}, nil, err
	}

	id := roomIDFromPathOrNumber(path, int(header.Number))
	warnings := make([]string, 0)

	rec := header
	if full, _, err := decoder.DecodeRoomFileRecord(data); err == nil {
		rec = full
	} else {
		warnings = append(warnings, fmt.Sprintf("decode room descriptions: %v", err))
	}

	displayName := textOrWarn(rec.Name, "room.name", &warnings)
	if strings.TrimSpace(displayName) == "" {
		displayName = string(id)
	}

	rawFields := map[string][]byte{
		"name":  rec.Name.Raw,
		"track": rec.Track.Raw,
		"flags": append([]byte(nil), rec.Flags[:]...),
	}
	addRoomLasttimeRawField(rawFields, "perm_mon", rec.PermMon)
	addRoomLasttimeRawField(rawFields, "perm_obj", rec.PermObj)

	room := model.Room{
		ID:                id,
		DisplayName:       displayName,
		ShortDescription:  textOrWarn(rec.ShortDescription, "room.shortDescription", &warnings),
		LongDescription:   textOrWarn(rec.LongDescription, "room.longDescription", &warnings),
		ObjectDescription: textOrWarn(rec.ObjectDescription, "room.objectDescription", &warnings),
		Properties:        roomProperties(rec, &warnings),
		Metadata: model.Metadata{
			Source:     legacySource,
			LegacyKind: "room",
			LegacyID:   legacyID(path, int(header.Number)),
			LegacyPath: filepath.ToSlash(path),
			Tags:       roomFlagNames(rec.Flags),
			RawFields:  rawFields,
		},
	}

	exits, exitWarnings, err := m.decodeExits(data)
	warnings = append(warnings, exitWarnings...)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("decode room exits: %v", err))
		return room, warnings, nil
	}
	room.Exits = mapExits(exits, &warnings)

	return room, warnings, nil
}

func (m Mapper) MapRoomFileBundle(path string, data []byte) (Bundle, error) {
	return m.MapRoomFileBundleWithOptions(path, data, Options{})
}

func (m Mapper) MapRoomFileBundleWithOptions(path string, data []byte, opts Options) (Bundle, error) {
	room, warnings, err := m.MapRoomFile(path, data)
	if err != nil {
		return Bundle{}, err
	}

	bundle := Bundle{
		Room:     room,
		Warnings: append([]string(nil), warnings...),
	}

	node, err := cbin.DecodeRoomTree(data)
	if err != nil {
		bundle.ContentError = err.Error()
		bundle.Warnings = append(bundle.Warnings, fmt.Sprintf("decode room contents: %v", err))
		return bundle, nil
	}
	bundle.ContentDecoded = true
	bundle.Decoded = node.Stats

	bundle.Room.CreatureIDs = nil
	bundle.Room.Objects.ObjectIDs = nil
	for i, creatureNode := range node.Creatures {
		result := creaturemap.MapCreatureTreeWithOptions(string(room.ID), room.ID, creatureNode, creaturemap.Options{
			PrototypeResolver: opts.PrototypeResolver,
			SourcePath:        path,
		})
		if result.Creature.ID.IsZero() {
			continue
		}
		result.Creature.Metadata.LegacyPath = filepath.ToSlash(path)
		result.Creature.Metadata.RecordIndex = i
		bundle.RootCreatureIDs = append(bundle.RootCreatureIDs, result.Creature.ID)
		bundle.Room.CreatureIDs = append(bundle.Room.CreatureIDs, result.Creature.ID)
		bundle.Creatures = append(bundle.Creatures, result.Creature)
		bundle.Objects = append(bundle.Objects, result.Objects...)
		mergePrototypeResolution(&bundle.PrototypeResolution, result.PrototypeResolution)
		for _, warning := range result.Warnings {
			bundle.Warnings = append(bundle.Warnings, fmt.Sprintf("room creature[%d]: %s", i, warning))
		}
	}

	for i, objectNode := range node.Objects {
		prefix := fmt.Sprintf("%s:object:%d", room.ID, i)
		objectResult := objectmap.MapObjectTreeWithOptions(prefix, model.ObjectLocation{RoomID: room.ID}, objectNode, objectmap.Options{
			PrototypeResolver: opts.PrototypeResolver,
			SourcePath:        path,
		})
		objects := objectResult.Objects
		if len(objects) == 0 {
			continue
		}
		bundle.RootObjectIDs = append(bundle.RootObjectIDs, objects[0].ID)
		bundle.Room.Objects.ObjectIDs = append(bundle.Room.Objects.ObjectIDs, objects[0].ID)
		bundle.Objects = append(bundle.Objects, objects...)
		mergePrototypeResolution(&bundle.PrototypeResolution, objectResult.PrototypeResolution)
		for _, warning := range objectResult.Warnings {
			bundle.Warnings = append(bundle.Warnings, fmt.Sprintf("room object[%d]: %s", i, warning))
		}
	}

	return bundle, nil
}

func mergePrototypeResolution(dst *objectmap.PrototypeResolutionCounts, src objectmap.PrototypeResolutionCounts) {
	dst.ResolvedExact += src.ResolvedExact
	dst.Synthetic += src.Synthetic
	dst.AmbiguousSynthetic += src.AmbiguousSynthetic
}

func (m Mapper) decodeExits(data []byte) ([]cbin.ExitRecord, []string, error) {
	decoder := m.decoder
	if decoder == nil {
		decoder = cbinDecoder{}
	}

	if len(data) < cbin.RoomSize+4 {
		return nil, nil, fmt.Errorf("room exit count: need int32 at offset %d, remaining %d", cbin.RoomSize, max(0, len(data)-cbin.RoomSize))
	}

	exitCount := int(int32(binary.LittleEndian.Uint32(data[cbin.RoomSize : cbin.RoomSize+4])))
	if exitCount < 0 || exitCount > cbin.MaxRoomExits {
		return nil, nil, fmt.Errorf("invalid room exit count %d at offset %d", exitCount, cbin.RoomSize)
	}

	exitsStart := cbin.RoomSize + 4
	exitsEnd := exitsStart + exitCount*cbin.ExitSize
	if exitsEnd > len(data) {
		return nil, nil, fmt.Errorf("room exits: need %d bytes at offset %d, remaining %d", exitCount*cbin.ExitSize, exitsStart, max(0, len(data)-exitsStart))
	}

	warnings := make([]string, 0)
	exits := make([]cbin.ExitRecord, 0, exitCount)
	for i := 0; i < exitCount; i++ {
		off := exitsStart + i*cbin.ExitSize
		exit, err := decoder.DecodeExitRecord(data[off : off+cbin.ExitSize])
		if err != nil {
			return exits, warnings, fmt.Errorf("exit %d: %w", i, err)
		}
		exits = append(exits, exit)
	}
	return exits, warnings, nil
}

func mapExits(records []cbin.ExitRecord, warnings *[]string) []model.Exit {
	if len(records) == 0 {
		return nil
	}

	exits := make([]model.Exit, 0, len(records))
	for i, rec := range records {
		name := textOrWarn(rec.Name, fmt.Sprintf("room.exit[%d].name", i), warnings)
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("exit:%d", i)
			*warnings = append(*warnings, fmt.Sprintf("room.exit[%d].name is empty; using %q", i, name))
		}

		exit := model.Exit{
			Name:     name,
			ToRoomID: roomIDFromNumber(int(rec.Room)),
			Flags:    exitFlagNames(rec.Flags),
			Metadata: model.Metadata{
				Source:      legacySource,
				LegacyKind:  "room_exit",
				RecordIndex: i,
				RawFields: map[string][]byte{
					"name":           rec.Name.Raw,
					"flags":          append([]byte(nil), rec.Flags[:]...),
					"key":            {rec.Key},
					"ltime.interval": int32RawBytes(rec.LTime.Interval),
					"ltime.ltime":    int32RawBytes(rec.LTime.LTime),
					"ltime.misc":     int16RawBytes(rec.LTime.Misc),
				},
			},
		}
		if rec.Key != 0 {
			exit.Flags = append(exit.Flags, fmt.Sprintf("key:%d", rec.Key))
		}
		exits = append(exits, exit)
	}
	return exits
}

func int32RawBytes(value int32) []byte {
	return []byte{
		byte(value),
		byte(value >> 8),
		byte(value >> 16),
		byte(value >> 24),
	}
}

func int16RawBytes(value int16) []byte {
	return []byte{
		byte(value),
		byte(value >> 8),
	}
}

func roomProperties(rec cbin.RoomRecord, warnings *[]string) map[string]string {
	properties := make(map[string]string)
	if rec.LoLevel > 0 {
		properties["minLevel"] = strconv.Itoa(int(rec.LoLevel))
	}
	if rec.HiLevel > 0 {
		properties["maxLevel"] = strconv.Itoa(int(rec.HiLevel))
	}
	if rec.Special != 0 {
		properties["special"] = strconv.Itoa(int(rec.Special))
	}
	if rec.Trap != 0 {
		properties["trap"] = strconv.Itoa(int(rec.Trap))
	}
	if rec.TrapExit != 0 {
		properties["trapExit"] = strconv.Itoa(int(rec.TrapExit))
	}
	if track := textOrWarn(rec.Track, "room.track", warnings); strings.TrimSpace(track) != "" {
		properties["track"] = track
	}
	if rec.Traffic != 0 {
		properties["traffic"] = strconv.Itoa(int(rec.Traffic))
	}
	if rec.BeenHere != 0 {
		properties["beenHere"] = strconv.FormatInt(int64(rec.BeenHere), 10)
	}
	if rec.Established != 0 {
		properties["established"] = strconv.FormatInt(int64(rec.Established), 10)
	}
	var randomStrings []string
	hasRandom := false
	for _, val := range rec.Random {
		randomStrings = append(randomStrings, strconv.Itoa(int(val)))
		if val != 0 {
			hasRandom = true
		}
	}
	if hasRandom {
		properties["random"] = strings.Join(randomStrings, ",")
	}
	addRoomLasttimeProperties(properties, "perm_mon", rec.PermMon)
	addRoomLasttimeProperties(properties, "perm_obj", rec.PermObj)
	if len(properties) == 0 {
		return nil
	}
	return properties
}

func addRoomLasttimeProperties(properties map[string]string, prefix string, records [10]cbin.LasttimeRecord) {
	for i, record := range records {
		if record.Interval == 0 && record.LTime == 0 && record.Misc == 0 {
			continue
		}
		slot := fmt.Sprintf("%s.%d", prefix, i)
		if record.Interval != 0 {
			properties[slot+".interval"] = strconv.FormatInt(int64(record.Interval), 10)
		}
		if record.LTime != 0 {
			properties[slot+".ltime"] = strconv.FormatInt(int64(record.LTime), 10)
		}
		if record.Misc != 0 {
			properties[slot+".misc"] = strconv.Itoa(int(record.Misc))
		}
	}
}

func addRoomLasttimeRawField(fields map[string][]byte, name string, records [10]cbin.LasttimeRecord) {
	raw := make([]byte, len(records)*cbin.LasttimeSize)
	hasAny := false
	for i, record := range records {
		off := i * cbin.LasttimeSize
		binary.LittleEndian.PutUint32(raw[off:], uint32(record.Interval))
		binary.LittleEndian.PutUint32(raw[off+4:], uint32(record.LTime))
		binary.LittleEndian.PutUint16(raw[off+8:], uint16(record.Misc))
		if record.Interval != 0 || record.LTime != 0 || record.Misc != 0 {
			hasAny = true
		}
	}
	if hasAny {
		fields[name] = raw
	}
}

func roomFlagNames(flags [8]byte) []string {
	names := []string{
		"shoppe",
		"dump",
		"pawnShop",
		"train",
		"trainingBit4",
		"trainingBit5",
		"trainingBit6",
		"repair",
		"darkAlways",
		"darkNight",
		"postOffice",
		"noPlayerKill",
		"noTeleport",
		"healFast",
		"onePlayer",
		"twoPlayers",
		"threePlayers",
		"noMagic",
		"permanentTracks",
		"earth",
		"wind",
		"fire",
		"water",
		"playerWander",
		"playerHarm",
		"playerPoison",
		"playerMPDrain",
		"playerBefuddle",
		"noSummonOut",
		"pledge",
		"rescind",
		"noPotion",
		"magicExtend",
		"noLog",
		"election",
		"forge",
		"survival",
		"family",
		"onlyFamily",
		"bank",
		"marriage",
		"onlyMarried",
		"cast",
		"depot",
	}

	out := make([]string, 0, len(names))
	for bit, name := range names {
		if flags[bit/8]&(1<<uint(bit%8)) != 0 {
			out = append(out, name)
		}
	}
	return out
}

func exitFlagNames(flags [4]byte) []string {
	names := []string{
		"secret",
		"invisible",
		"locked",
		"closed",
		"lockable",
		"closable",
		"unpickable",
		"naked",
		"climb",
		"repel",
		"hardClimb",
		"fly",
		"femaleOnly",
		"maleOnly",
		"pledgeOnly",
		"kingdomSelector",
		"nightOnly",
		"dayOnly",
		"guarded",
		"noSee",
		"kingdom1",
		"kingdom2",
	}

	out := make([]string, 0, len(names))
	for bit, name := range names {
		if flags[bit/8]&(1<<uint(bit%8)) != 0 {
			out = append(out, name)
		}
	}
	return out
}

func textOrWarn(field cbin.TextField, name string, warnings *[]string) string {
	if field.Err != nil {
		*warnings = append(*warnings, fmt.Sprintf("%s decode: %v", name, field.Err))
	}
	return field.Text
}

func roomIDFromPathOrNumber(path string, number int) model.RoomID {
	if id, ok := roomIDFromPath(path); ok {
		return id
	}
	return roomIDFromNumber(number)
}

func roomIDFromPath(path string) (model.RoomID, bool) {
	m := roomFileRE.FindStringSubmatch(filepath.Base(path))
	if m == nil {
		return "", false
	}
	return model.RoomID("room:" + m[1]), true
}

func roomIDFromNumber(number int) model.RoomID {
	if number >= 0 {
		return model.RoomID(fmt.Sprintf("room:%05d", number))
	}
	return model.RoomID(fmt.Sprintf("room:%d", number))
}

func legacyID(path string, number int) string {
	base := filepath.Base(path)
	if roomFileRE.MatchString(base) {
		return base
	}
	return strconv.Itoa(number)
}
