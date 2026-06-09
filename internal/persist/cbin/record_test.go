package cbin

import (
	"encoding/binary"
	"testing"
)

func TestDecodeZeroRecords(t *testing.T) {
	obj, err := DecodeObjectRecord(make([]byte, ObjectSize))
	if err != nil {
		t.Fatal(err)
	}
	assertEmptyText(t, "object.name", obj.Name)
	assertEmptyText(t, "object.description", obj.Description)
	assertEmptyText(t, "object.key[0]", obj.Keys[0])
	assertEmptyText(t, "object.use_output", obj.UseOutput)
	if obj.Weight != 0 || obj.Flags != [8]byte{} {
		t.Fatalf("object scalar fields = %+v, want zero values", obj)
	}

	crt, err := DecodeCreatureRecord(make([]byte, CreatureSize))
	if err != nil {
		t.Fatal(err)
	}
	assertEmptyText(t, "creature.name", crt.Name)
	assertEmptyText(t, "creature.description", crt.Description)
	assertEmptyText(t, "creature.talk", crt.Talk)
	assertEmptyText(t, "creature.password", crt.Password)
	assertEmptyText(t, "creature.key[0]", crt.Keys[0])
	if crt.Spells != [16]byte{} || crt.Flags != [8]byte{} || crt.Carry != [10]int16{} || crt.Daily != [10]DailyRecord{} {
		t.Fatalf("creature preserved fields = spells % X flags % X carry %+v daily %+v, want zero values", crt.Spells, crt.Flags, crt.Carry, crt.Daily)
	}

	room, err := DecodeRoomRecord(make([]byte, RoomSize))
	if err != nil {
		t.Fatal(err)
	}
	if room.Number != 0 {
		t.Fatalf("room number = %d, want 0", room.Number)
	}
	if room.LoLevel != 0 || room.HiLevel != 0 || room.Special != 0 ||
		room.Trap != 0 || room.TrapExit != 0 || room.Flags != [8]byte{} ||
		room.Traffic != 0 || room.PermMon != [10]LasttimeRecord{} || room.PermObj != [10]LasttimeRecord{} ||
		room.BeenHere != 0 || room.Established != 0 {
		t.Fatalf("room scalar fields = %+v, want zero values", room)
	}
	assertEmptyText(t, "room.name", room.Name)
	assertEmptyText(t, "room.track", room.Track)

	exit, err := DecodeExitRecord(make([]byte, ExitSize))
	if err != nil {
		t.Fatal(err)
	}
	if exit.Room != 0 || exit.Key != 0 {
		t.Fatalf("exit = %+v, want zero target/key", exit)
	}
	if exit.Flags != [4]byte{} {
		t.Fatalf("exit flags = % X, want zero flags", exit.Flags)
	}
	assertEmptyText(t, "exit.name", exit.Name)

	board, err := DecodeBoardIndexRecord(make([]byte, BoardIndexSize))
	if err != nil {
		t.Fatal(err)
	}
	if board.Number != 0 || board.Line != 0 || board.ReadCount != 0 {
		t.Fatalf("board = %+v, want zero ids", board)
	}
	assertEmptyText(t, "board.upload", board.Uploader)
	assertEmptyText(t, "board.title", board.Title)
}

func TestDecodeObjectRecordWeightAndFlags(t *testing.T) {
	data := make([]byte, ObjectSize)
	copy(data[objectNameOff:], []byte("bag\x00"))
	binary.LittleEndian.PutUint32(data[objectValueOff:], uint32(1234))
	binary.LittleEndian.PutUint16(data[objectWeightOff:], uint16(17))
	data[objectTypeOff] = 5
	data[objectAdjustmentOff] = 2
	binary.LittleEndian.PutUint16(data[objectShotsMaxOff:], uint16(99))
	binary.LittleEndian.PutUint16(data[objectShotsCurrentOff:], uint16(88))
	binary.LittleEndian.PutUint16(data[objectNDiceOff:], uint16(3))
	binary.LittleEndian.PutUint16(data[objectSDiceOff:], uint16(6))
	binary.LittleEndian.PutUint16(data[objectPDiceOff:], uint16(1))
	data[objectArmorOff] = 7
	data[objectWearFlagOff] = 20
	data[objectMagicPowerOff] = 4
	data[objectMagicRealmOff] = 3
	binary.LittleEndian.PutUint16(data[objectSpecialOff:], uint16(11))
	copy(data[objectFlagsOff:], []byte{0x80, 0x02, 0, 0, 0, 0, 0, 0})
	data[objectQuestNumberOff] = 9

	rec, err := DecodeObjectRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Name.Text != "bag" || rec.Weight != 17 {
		t.Fatalf("object = %+v, want bag weight 17", rec)
	}
	if rec.Value != 1234 || rec.Type != 5 || rec.Adjustment != 2 ||
		rec.ShotsMax != 99 || rec.ShotsCurrent != 88 ||
		rec.NDice != 3 || rec.SDice != 6 || rec.PDice != 1 ||
		rec.Armor != 7 || rec.WearFlag != 20 ||
		rec.MagicPower != 4 || rec.MagicRealm != 3 ||
		rec.Special != 11 || rec.QuestNumber != 9 {
		t.Fatalf("object numeric fields = %+v", rec)
	}
	if rec.Flags != [8]byte{0x80, 0x02, 0, 0, 0, 0, 0, 0} {
		t.Fatalf("object flags = % X", rec.Flags)
	}
}

func TestDecodeCreatureRecordFlagsAndDaily(t *testing.T) {
	data := make([]byte, CreatureSize)
	copy(data[creatureNameOff:], []byte("player\x00"))
	copy(data[creatureSpellsOff:], []byte{0x04, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x20, 0})
	copy(data[creatureFlagsOff:], []byte{0x01, 0, 0, 0, 0, 0, 0x80, 0x40})
	binary.LittleEndian.PutUint16(data[creatureCarryOff:], uint16(321))
	binary.LittleEndian.PutUint16(data[creatureCarryOff+2:], uint16(654))

	marriageOff := creatureDailyOff + 8*DailySize
	data[marriageOff+dailyMaxOff] = 11
	data[marriageOff+dailyCurOff] = 2
	binary.LittleEndian.PutUint32(data[marriageOff+dailyLTimeOff:], 123456)

	familyOff := creatureDailyOff + 9*DailySize
	data[familyOff+dailyMaxOff] = 7
	data[familyOff+dailyCurOff] = 3
	binary.LittleEndian.PutUint32(data[familyOff+dailyLTimeOff:], 654321)

	rec, err := DecodeCreatureRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Name.Text != "player" {
		t.Fatalf("creature name = %q, want player", rec.Name.Text)
	}
	if rec.Spells != [16]byte{0x04, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x20, 0} {
		t.Fatalf("creature spells = % X", rec.Spells)
	}
	if rec.Flags != [8]byte{0x01, 0, 0, 0, 0, 0, 0x80, 0x40} {
		t.Fatalf("creature flags = % X", rec.Flags)
	}
	if rec.Carry[0] != 321 || rec.Carry[1] != 654 {
		t.Fatalf("creature carry = %+v", rec.Carry)
	}
	if rec.Daily[8] != (DailyRecord{Max: 11, Cur: 2, LTime: 123456}) {
		t.Fatalf("daily[8] = %+v", rec.Daily[8])
	}
	if rec.Daily[9] != (DailyRecord{Max: 7, Cur: 3, LTime: 654321}) {
		t.Fatalf("daily[9] = %+v", rec.Daily[9])
	}
}

func TestDecodeExitRecordFlags(t *testing.T) {
	data := make([]byte, ExitSize)
	copy(data[exitNameOff:], []byte("east\x00"))
	binary.LittleEndian.PutUint16(data[exitRoomOff:], 22)
	copy(data[exitFlagsOff:], []byte{0x09, 0x02, 0x08, 0})
	binary.LittleEndian.PutUint32(data[exitLTimeOff+lasttimeIntervalOff:], 300)
	binary.LittleEndian.PutUint32(data[exitLTimeOff+lasttimeLTimeOff:], 400)
	binary.LittleEndian.PutUint16(data[exitLTimeOff+lasttimeMiscOff:], 55)
	data[exitKeyOff] = 7

	exit, err := DecodeExitRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	if exit.Name.Text != "east" || exit.Room != 22 || exit.Key != 7 {
		t.Fatalf("exit = %+v", exit)
	}
	if exit.Flags != [4]byte{0x09, 0x02, 0x08, 0} {
		t.Fatalf("exit flags = % X", exit.Flags)
	}
	if exit.LTime != (LasttimeRecord{Interval: 300, LTime: 400, Misc: 55}) {
		t.Fatalf("exit ltime = %+v", exit.LTime)
	}
}

func TestDecodeSampleEUCKRCString(t *testing.T) {
	data := make([]byte, ObjectSize)
	copy(data[objectNameOff:], []byte{0xB0, 0xA1, 0xB3, 0xAA, 0xB4, 0xD9, 0})

	rec, err := DecodeObjectRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Name.Err != nil {
		t.Fatal(rec.Name.Err)
	}
	if rec.Name.Text != "가나다" {
		t.Fatalf("object.name = %q, want %q", rec.Name.Text, "가나다")
	}
	if string(rec.Name.Raw) != string([]byte{0xB0, 0xA1, 0xB3, 0xAA, 0xB4, 0xD9}) {
		t.Fatalf("object.name raw = % X", rec.Name.Raw)
	}
}

func TestDecodeRoomFileRecordDescriptions(t *testing.T) {
	data := make([]byte, RoomSize)
	binary.LittleEndian.PutUint16(data[roomNumberOff:], uint16(1234))
	copy(data[roomNameOff:], []byte("room name\x00"))
	data = appendInt32(data, 0)                           // exits
	data = appendInt32(data, 0)                           // creatures
	data = appendInt32(data, 0)                           // objects
	data = appendDescription(data, []byte{0xB0, 0xA1, 0}) // short: "가"
	data = appendDescription(data, []byte("long text\x00"))
	data = appendDescription(data, []byte("object text\x00"))

	rec, stats, err := DecodeRoomFileRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Number != 1234 || rec.Name.Text != "room name" {
		t.Fatalf("room = %+v", rec)
	}
	if rec.ShortDescription.Err != nil {
		t.Fatal(rec.ShortDescription.Err)
	}
	if rec.ShortDescription.Text != "가" {
		t.Fatalf("short desc = %q, want %q", rec.ShortDescription.Text, "가")
	}
	if rec.LongDescription.Text != "long text" || rec.ObjectDescription.Text != "object text" {
		t.Fatalf("descriptions = %q / %q", rec.LongDescription.Text, rec.ObjectDescription.Text)
	}
	if stats.Descriptions != 3 || stats.DescriptionBytes != 25 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestDecodeRoomRecordScalarFields(t *testing.T) {
	data := make([]byte, RoomSize)
	binary.LittleEndian.PutUint16(data[roomNumberOff:], uint16(1234))
	copy(data[roomNameOff:], []byte("room name\x00"))
	data[roomLoLevelOff] = 7
	data[roomHiLevelOff] = 30
	binary.LittleEndian.PutUint16(data[roomSpecialOff:], uint16(0x1234))
	data[roomTrapOff] = 3
	binary.LittleEndian.PutUint16(data[roomTrapExitOff:], uint16(55))
	copy(data[roomTrackOff:], []byte("east\x00"))
	copy(data[roomFlagsOff:], []byte{0x01, 0xC0, 0x01, 0, 0x22, 0x08, 0, 0})
	data[roomTrafficOff] = 9
	binary.LittleEndian.PutUint32(data[roomPermMonOff+lasttimeIntervalOff:], 300)
	binary.LittleEndian.PutUint32(data[roomPermMonOff+lasttimeLTimeOff:], 400)
	binary.LittleEndian.PutUint16(data[roomPermMonOff+lasttimeMiscOff:], 55)
	binary.LittleEndian.PutUint32(data[roomPermObjOff+LasttimeSize+lasttimeIntervalOff:], 500)
	binary.LittleEndian.PutUint32(data[roomPermObjOff+LasttimeSize+lasttimeLTimeOff:], 600)
	binary.LittleEndian.PutUint16(data[roomPermObjOff+LasttimeSize+lasttimeMiscOff:], 77)
	binary.LittleEndian.PutUint32(data[roomBeenHereOff:], 77)
	binary.LittleEndian.PutUint32(data[roomEstablishedOff:], 88)

	room, err := DecodeRoomRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	if room.Number != 1234 || room.Name.Text != "room name" ||
		room.LoLevel != 7 || room.HiLevel != 30 || room.Special != 0x1234 ||
		room.Trap != 3 || room.TrapExit != 55 || room.Track.Text != "east" ||
		room.Traffic != 9 || room.BeenHere != 77 || room.Established != 88 {
		t.Fatalf("room = %+v", room)
	}
	if room.PermMon[0] != (LasttimeRecord{Interval: 300, LTime: 400, Misc: 55}) {
		t.Fatalf("room perm mon[0] = %+v", room.PermMon[0])
	}
	if room.PermObj[1] != (LasttimeRecord{Interval: 500, LTime: 600, Misc: 77}) {
		t.Fatalf("room perm obj[1] = %+v", room.PermObj[1])
	}
	if string(room.Track.Raw) != "east" {
		t.Fatalf("room track raw = % X", room.Track.Raw)
	}
	if room.Flags != [8]byte{0x01, 0xC0, 0x01, 0, 0x22, 0x08, 0, 0} {
		t.Fatalf("room flags = % X", room.Flags)
	}
}

func TestDecodeBoardIndexRecordsUsesBoardIndexSize(t *testing.T) {
	data := make([]byte, BoardIndexSize*2)
	binary.LittleEndian.PutUint32(data[boardNumberOff:], 7)
	copy(data[boardUploaderOff:], []byte("tester\x00"))
	binary.LittleEndian.PutUint32(data[boardYearOff:], 126)
	binary.LittleEndian.PutUint32(data[boardLineOff:], 12)
	binary.LittleEndian.PutUint32(data[boardReadCountOff:], 34)
	copy(data[boardTitleOff:], []byte("hello board\x00"))

	second := data[BoardIndexSize:]
	binary.LittleEndian.PutUint32(second[boardNumberOff:], 8)
	copy(second[boardUploaderOff:], []byte("second\x00"))
	copy(second[boardTitleOff:], []byte("next title\x00"))

	records, err := DecodeBoardIndexRecords(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("records len = %d, want 2", len(records))
	}
	first := records[0]
	if first.Number != 7 || first.Uploader.Text != "tester" || first.Year != 126 ||
		first.Line != 12 || first.ReadCount != 34 || first.Title.Text != "hello board" {
		t.Fatalf("first board index = %+v", first)
	}
	secondRec := records[1]
	if secondRec.Number != 8 || secondRec.Uploader.Text != "second" || secondRec.Title.Text != "next title" {
		t.Fatalf("second board index = %+v", secondRec)
	}

	if _, err := DecodeBoardIndexRecords(data[:BoardIndexSize+1]); err == nil {
		t.Fatal("DecodeBoardIndexRecords accepted non-multiple board index data")
	}
}

func assertEmptyText(t *testing.T, field string, got TextField) {
	t.Helper()
	if got.Err != nil {
		t.Fatalf("%s unexpected decode error: %v", field, got.Err)
	}
	if got.Text != "" || len(got.Raw) != 0 {
		t.Fatalf("%s = text %q raw % X, want empty", field, got.Text, got.Raw)
	}
}

func appendDescription(data []byte, desc []byte) []byte {
	data = appendInt32(data, int32(len(desc)))
	return append(data, desc...)
}

func appendInt32(data []byte, value int32) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(value))
	return append(data, buf[:]...)
}
