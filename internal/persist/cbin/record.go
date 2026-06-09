package cbin

import (
	"encoding/binary"
	"fmt"

	"muhan/internal/persist/legacykr"
)

// TextField is a decoded legacy C text field. Raw is NUL-trimmed and copied.
// Err is non-nil when EUC-KR/CP949 conversion failed; Raw remains available.
type TextField struct {
	Raw  []byte
	Text string
	Err  error
}

type ObjectRecord struct {
	Raw          []byte
	Name         TextField
	Description  TextField
	Keys         [3]TextField
	UseOutput    TextField
	Value        int32
	Weight       int16
	Type         int8
	Adjustment   int8
	ShotsMax     int16
	ShotsCurrent int16
	NDice        int16
	SDice        int16
	PDice        int16
	Armor        int8
	WearFlag     int8
	MagicPower   int8
	MagicRealm   int8
	Special      int16
	Flags        [8]byte
	QuestNumber  int8
}

type DailyRecord struct {
	Max   byte
	Cur   byte
	LTime int32
}

type LasttimeRecord struct {
	Interval int32
	LTime    int32
	Misc     int16
}

type CreatureRecord struct {
	Raw         []byte
	Name        TextField
	Description TextField
	Talk        TextField
	Password    TextField
	Keys        [3]TextField
	Spells      [16]byte
	Flags       [8]byte
	Carry       [10]int16
	Daily       [10]DailyRecord
}

type RoomRecord struct {
	Number            int16
	Name              TextField
	LoLevel           byte
	HiLevel           byte
	Special           int16
	Trap              byte
	TrapExit          int16
	Track             TextField
	Flags             [8]byte
	Random            [10]int16
	Traffic           byte
	PermMon           [10]LasttimeRecord
	PermObj           [10]LasttimeRecord
	BeenHere          int32
	Established       int32
	ShortDescription  TextField
	LongDescription   TextField
	ObjectDescription TextField
}

type ExitRecord struct {
	Name  TextField
	Room  int16
	Flags [4]byte
	LTime LasttimeRecord
	Key   byte
}

type BoardIndexRecord struct {
	Number    int32
	Uploader  TextField
	Year      int32
	Month     int32
	Day       int32
	Hour      int32
	Minute    int32
	Second    int32
	Line      int32
	ReadCount int32
	Title     TextField
}

const (
	objectNameOff         = 0
	objectDescriptionOff  = 80
	objectKeyOff          = 160
	objectUseOutputOff    = 220
	objectValueOff        = 300
	objectWeightOff       = 304
	objectTypeOff         = 306
	objectAdjustmentOff   = 307
	objectShotsMaxOff     = 308
	objectShotsCurrentOff = 310
	objectNDiceOff        = 312
	objectSDiceOff        = 314
	objectPDiceOff        = 316
	objectArmorOff        = 318
	objectWearFlagOff     = 319
	objectMagicPowerOff   = 320
	objectMagicRealmOff   = 321
	objectSpecialOff      = 322
	objectFlagsOff        = 324
	objectQuestNumberOff  = 332

	creatureNameOff        = 0
	creatureDescriptionOff = 80
	creatureTalkOff        = 160
	creaturePasswordOff    = 240
	creatureKeyOff         = 255
	creatureSpellsOff      = 396
	creatureFlagsOff       = 412
	creatureCarryOff       = 438
	creatureDailyOff       = 540
	dailyMaxOff            = 0
	dailyCurOff            = 1
	dailyLTimeOff          = 4

	roomNumberOff       = 0
	roomNameOff         = 2
	roomLoLevelOff      = 96
	roomHiLevelOff      = 97
	roomSpecialOff      = 98
	roomTrapOff         = 100
	roomTrapExitOff     = 102
	roomTrackOff        = 104
	roomFlagsOff        = 184
	roomRandomOff       = 192
	roomTrafficOff      = 212
	roomPermMonOff      = 216
	roomPermObjOff      = 336
	roomBeenHereOff     = 456
	roomEstablishedOff  = 460
	lasttimeIntervalOff = 0
	lasttimeLTimeOff    = 4
	lasttimeMiscOff     = 8

	exitNameOff  = 0
	exitRoomOff  = 20
	exitFlagsOff = 22
	exitLTimeOff = 28
	exitKeyOff   = 40

	boardNumberOff    = 0
	boardUploaderOff  = 4
	boardYearOff      = 20
	boardMonthOff     = 24
	boardDayOff       = 28
	boardHourOff      = 32
	boardMinuteOff    = 36
	boardSecondOff    = 40
	boardLineOff      = 44
	boardReadCountOff = 48
	boardTitleOff     = 52

	cString80 = 80
	cString40 = 40
	cString20 = 20
	cString16 = 16
	cString15 = 15
)

func DecodeObjectRecord(data []byte) (ObjectRecord, error) {
	if err := requireRecordSize("object", data, ObjectSize); err != nil {
		return ObjectRecord{}, err
	}
	var rec ObjectRecord
	rec.Raw = cloneRecordBytes(data[:ObjectSize])
	rec.Name = decodeCString(data, objectNameOff, cString80, "object.name")
	rec.Description = decodeCString(data, objectDescriptionOff, cString80, "object.description")
	for i := range rec.Keys {
		rec.Keys[i] = decodeCString(data, objectKeyOff+i*cString20, cString20, fmt.Sprintf("object.key[%d]", i))
	}
	rec.UseOutput = decodeCString(data, objectUseOutputOff, cString80, "object.use_output")
	rec.Value = readInt32At(data, objectValueOff)
	rec.Weight = int16(binary.LittleEndian.Uint16(data[objectWeightOff:]))
	rec.Type = int8(data[objectTypeOff])
	rec.Adjustment = int8(data[objectAdjustmentOff])
	rec.ShotsMax = readInt16At(data, objectShotsMaxOff)
	rec.ShotsCurrent = readInt16At(data, objectShotsCurrentOff)
	rec.NDice = readInt16At(data, objectNDiceOff)
	rec.SDice = readInt16At(data, objectSDiceOff)
	rec.PDice = readInt16At(data, objectPDiceOff)
	rec.Armor = int8(data[objectArmorOff])
	rec.WearFlag = int8(data[objectWearFlagOff])
	rec.MagicPower = int8(data[objectMagicPowerOff])
	rec.MagicRealm = int8(data[objectMagicRealmOff])
	rec.Special = readInt16At(data, objectSpecialOff)
	copy(rec.Flags[:], data[objectFlagsOff:objectFlagsOff+len(rec.Flags)])
	rec.QuestNumber = int8(data[objectQuestNumberOff])
	return rec, nil
}

func DecodeObjectRecords(data []byte) ([]ObjectRecord, error) {
	count, err := ValidateObjectPrototypeFile(data)
	if err != nil {
		return nil, err
	}
	records := make([]ObjectRecord, count)
	for i := range records {
		rec, err := DecodeObjectRecord(data[i*ObjectSize:])
		if err != nil {
			return nil, fmt.Errorf("object record %d: %w", i, err)
		}
		records[i] = rec
	}
	return records, nil
}

func DecodeCreatureRecord(data []byte) (CreatureRecord, error) {
	if err := requireRecordSize("creature", data, CreatureSize); err != nil {
		return CreatureRecord{}, err
	}
	var rec CreatureRecord
	rec.Raw = cloneRecordBytes(data[:CreatureSize])
	rec.Name = decodeCString(data, creatureNameOff, cString80, "creature.name")
	rec.Description = decodeCString(data, creatureDescriptionOff, cString80, "creature.description")
	rec.Talk = decodeCString(data, creatureTalkOff, cString80, "creature.talk")
	rec.Password = decodeCString(data, creaturePasswordOff, cString15, "creature.password")
	for i := range rec.Keys {
		rec.Keys[i] = decodeCString(data, creatureKeyOff+i*cString20, cString20, fmt.Sprintf("creature.key[%d]", i))
	}
	copy(rec.Spells[:], data[creatureSpellsOff:creatureSpellsOff+len(rec.Spells)])
	copy(rec.Flags[:], data[creatureFlagsOff:creatureFlagsOff+len(rec.Flags)])
	for i := range rec.Carry {
		rec.Carry[i] = readInt16At(data, creatureCarryOff+i*2)
	}
	for i := range rec.Daily {
		off := creatureDailyOff + i*DailySize
		rec.Daily[i] = DailyRecord{
			Max:   data[off+dailyMaxOff],
			Cur:   data[off+dailyCurOff],
			LTime: readInt32At(data, off+dailyLTimeOff),
		}
	}
	return rec, nil
}

func cloneRecordBytes(data []byte) []byte {
	out := make([]byte, len(data))
	copy(out, data)
	return out
}

func DecodeCreatureRecords(data []byte) ([]CreatureRecord, error) {
	count, err := ValidateCreaturePrototypeFile(data)
	if err != nil {
		return nil, err
	}
	records := make([]CreatureRecord, count)
	for i := range records {
		rec, err := DecodeCreatureRecord(data[i*CreatureSize:])
		if err != nil {
			return nil, fmt.Errorf("creature record %d: %w", i, err)
		}
		records[i] = rec
	}
	return records, nil
}

func DecodeRoomRecord(data []byte) (RoomRecord, error) {
	if err := requireRecordSize("room", data, RoomSize); err != nil {
		return RoomRecord{}, err
	}
	var flags [8]byte
	copy(flags[:], data[roomFlagsOff:roomFlagsOff+len(flags)])
	var random [10]int16
	for i := 0; i < 10; i++ {
		random[i] = readInt16At(data, roomRandomOff+i*2)
	}
	var permMon [10]LasttimeRecord
	for i := 0; i < 10; i++ {
		permMon[i] = readLasttimeAt(data, roomPermMonOff+i*LasttimeSize)
	}
	var permObj [10]LasttimeRecord
	for i := 0; i < 10; i++ {
		permObj[i] = readLasttimeAt(data, roomPermObjOff+i*LasttimeSize)
	}
	return RoomRecord{
		Number:      int16(binary.LittleEndian.Uint16(data[roomNumberOff : roomNumberOff+2])),
		Name:        decodeCString(data, roomNameOff, cString80, "room.name"),
		LoLevel:     data[roomLoLevelOff],
		HiLevel:     data[roomHiLevelOff],
		Special:     int16(binary.LittleEndian.Uint16(data[roomSpecialOff : roomSpecialOff+2])),
		Trap:        data[roomTrapOff],
		TrapExit:    int16(binary.LittleEndian.Uint16(data[roomTrapExitOff : roomTrapExitOff+2])),
		Track:       decodeCString(data, roomTrackOff, cString80, "room.track"),
		Flags:       flags,
		Random:      random,
		Traffic:     data[roomTrafficOff],
		PermMon:     permMon,
		PermObj:     permObj,
		BeenHere:    readInt32At(data, roomBeenHereOff),
		Established: readInt32At(data, roomEstablishedOff),
	}, nil
}

func DecodeRoomFileRecord(data []byte) (RoomRecord, Stats, error) {
	stats, err := DecodeRoomFile(data)
	if err != nil {
		return RoomRecord{}, stats, err
	}
	rec, err := DecodeRoomRecord(data)
	if err != nil {
		return RoomRecord{}, stats, err
	}

	cur := NewCursor(data)
	if err := cur.skip(RoomSize); err != nil {
		return RoomRecord{}, stats, err
	}
	if err := skipRoomContents(cur); err != nil {
		return RoomRecord{}, stats, err
	}

	descriptions, err := readRoomDescriptions(cur)
	if err != nil {
		return RoomRecord{}, stats, err
	}
	rec.ShortDescription = descriptions[0]
	rec.LongDescription = descriptions[1]
	rec.ObjectDescription = descriptions[2]
	return rec, stats, nil
}

func DecodeExitRecord(data []byte) (ExitRecord, error) {
	if err := requireRecordSize("exit", data, ExitSize); err != nil {
		return ExitRecord{}, err
	}
	var flags [4]byte
	copy(flags[:], data[exitFlagsOff:exitFlagsOff+len(flags)])
	return ExitRecord{
		Name:  decodeCString(data, exitNameOff, cString20, "exit.name"),
		Room:  int16(binary.LittleEndian.Uint16(data[exitRoomOff : exitRoomOff+2])),
		Flags: flags,
		LTime: readLasttimeAt(data, exitLTimeOff),
		Key:   data[exitKeyOff],
	}, nil
}

func DecodeBoardIndexRecord(data []byte) (BoardIndexRecord, error) {
	if err := requireRecordSize("board index", data, BoardIndexSize); err != nil {
		return BoardIndexRecord{}, err
	}
	return BoardIndexRecord{
		Number:    readInt32At(data, boardNumberOff),
		Uploader:  decodeCString(data, boardUploaderOff, cString16, "board_index.upload"),
		Year:      readInt32At(data, boardYearOff),
		Month:     readInt32At(data, boardMonthOff),
		Day:       readInt32At(data, boardDayOff),
		Hour:      readInt32At(data, boardHourOff),
		Minute:    readInt32At(data, boardMinuteOff),
		Second:    readInt32At(data, boardSecondOff),
		Line:      readInt32At(data, boardLineOff),
		ReadCount: readInt32At(data, boardReadCountOff),
		Title:     decodeCString(data, boardTitleOff, cString40, "board_index.title"),
	}, nil
}

func DecodeBoardIndexRecords(data []byte) ([]BoardIndexRecord, error) {
	count, err := ValidateBoardIndexFile(data)
	if err != nil {
		return nil, err
	}
	records := make([]BoardIndexRecord, count)
	for i := range records {
		rec, err := DecodeBoardIndexRecord(data[i*BoardIndexSize:])
		if err != nil {
			return nil, fmt.Errorf("board index record %d: %w", i, err)
		}
		records[i] = rec
	}
	return records, nil
}

func readRoomDescriptions(cur *Cursor) ([3]TextField, error) {
	var out [3]TextField
	for i := range out {
		size, err := cur.int32()
		if err != nil {
			return out, fmt.Errorf("room description %d length: %w", i, err)
		}
		if size < 0 || size > MaxDescriptionBytes {
			return out, fmt.Errorf("invalid room description %d length %d at offset %d", i, size, cur.Offset()-4)
		}
		if cur.off+size > len(cur.data) {
			return out, fmt.Errorf("room description %d bytes: need %d bytes at offset %d, remaining %d", i, size, cur.Offset(), cur.Remaining())
		}
		out[i] = decodeCString(cur.data, cur.off, size, fmt.Sprintf("room.description[%d]", i))
		if err := cur.skip(size); err != nil {
			return out, fmt.Errorf("room description %d bytes: %w", i, err)
		}
	}
	return out, nil
}

func skipRoomContents(cur *Cursor) error {
	exitCount, err := cur.int32()
	if err != nil {
		return fmt.Errorf("room exit count: %w", err)
	}
	if exitCount < 0 || exitCount > MaxRoomExits {
		return fmt.Errorf("invalid room exit count %d at offset %d", exitCount, cur.Offset()-4)
	}
	if err := cur.skip(exitCount * ExitSize); err != nil {
		return fmt.Errorf("room exits: %w", err)
	}

	creatureCount, err := cur.int32()
	if err != nil {
		return fmt.Errorf("room creature count: %w", err)
	}
	if creatureCount < 0 || creatureCount > MaxRoomCreatures {
		return fmt.Errorf("invalid room creature count %d at offset %d", creatureCount, cur.Offset()-4)
	}
	for i := 0; i < creatureCount; i++ {
		if err := skipCreatureTree(cur, 1); err != nil {
			return fmt.Errorf("room creature %d/%d: %w", i+1, creatureCount, err)
		}
	}

	objectCount, err := cur.int32()
	if err != nil {
		return fmt.Errorf("room object count: %w", err)
	}
	if objectCount < 0 || objectCount > MaxRoomObjects {
		return fmt.Errorf("invalid room object count %d at offset %d", objectCount, cur.Offset()-4)
	}
	for i := 0; i < objectCount; i++ {
		if err := skipObjectTree(cur, 1); err != nil {
			return fmt.Errorf("room object %d/%d: %w", i+1, objectCount, err)
		}
	}
	return nil
}

func skipObjectTree(cur *Cursor, depth int) error {
	if depth > MaxRecursionDepth {
		return fmt.Errorf("object recursion depth exceeds %d", MaxRecursionDepth)
	}
	if err := cur.skip(ObjectSize); err != nil {
		return fmt.Errorf("object record: %w", err)
	}
	count, err := cur.int32()
	if err != nil {
		return fmt.Errorf("object child count: %w", err)
	}
	if count < 0 || count > MaxObjectChildren {
		return fmt.Errorf("invalid object child count %d at offset %d", count, cur.Offset()-4)
	}
	for i := 0; i < count; i++ {
		if err := skipObjectTree(cur, depth+1); err != nil {
			return fmt.Errorf("object child %d/%d: %w", i+1, count, err)
		}
	}
	return nil
}

func skipCreatureTree(cur *Cursor, depth int) error {
	if depth > MaxRecursionDepth {
		return fmt.Errorf("creature recursion depth exceeds %d", MaxRecursionDepth)
	}
	if err := cur.skip(CreatureSize); err != nil {
		return fmt.Errorf("creature record: %w", err)
	}
	count, err := cur.int32()
	if err != nil {
		return fmt.Errorf("creature inventory count: %w", err)
	}
	if count < 0 || count > MaxCreatureItems {
		return fmt.Errorf("invalid creature inventory count %d at offset %d", count, cur.Offset()-4)
	}
	for i := 0; i < count; i++ {
		if err := skipObjectTree(cur, depth+1); err != nil {
			return fmt.Errorf("creature inventory object %d/%d: %w", i+1, count, err)
		}
	}
	return nil
}

func decodeCString(data []byte, off, size int, field string) TextField {
	raw := legacykr.TrimCString(data[off : off+size])
	copied := make([]byte, len(raw))
	copy(copied, raw)
	text, err := legacykr.DecodeCStringEUCKRContext(legacykr.Context{Field: field}, raw)
	return TextField{Raw: copied, Text: text, Err: err}
}

func requireRecordSize(name string, data []byte, size int) error {
	if len(data) < size {
		return fmt.Errorf("%s record: need %d bytes, got %d", name, size, len(data))
	}
	return nil
}

func readInt32At(data []byte, off int) int32 {
	return int32(binary.LittleEndian.Uint32(data[off : off+4]))
}

func readInt16At(data []byte, off int) int16 {
	return int16(binary.LittleEndian.Uint16(data[off : off+2]))
}

func readLasttimeAt(data []byte, off int) LasttimeRecord {
	return LasttimeRecord{
		Interval: readInt32At(data, off+lasttimeIntervalOff),
		LTime:    readInt32At(data, off+lasttimeLTimeOff),
		Misc:     readInt16At(data, off+lasttimeMiscOff),
	}
}
