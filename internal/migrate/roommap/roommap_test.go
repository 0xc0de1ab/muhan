package roommap

import (
	"encoding/binary"
	"strings"
	"testing"

	"muhan/internal/persist/cbin"
	"muhan/internal/persist/legacykr"
)

func TestMapMinimalRoom(t *testing.T) {
	data := makeMinimalRoom(t, 7, "")

	room, warnings, err := MapRoomFile("rooms/r00/r00007", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if room.ID != "room:00007" {
		t.Fatalf("room id = %q, want room:00007", room.ID)
	}
	if room.DisplayName != "room:00007" {
		t.Fatalf("display name = %q, want fallback id", room.DisplayName)
	}
	if room.ShortDescription != "" || room.LongDescription != "" || room.ObjectDescription != "" {
		t.Fatalf("descriptions = %q / %q / %q", room.ShortDescription, room.LongDescription, room.ObjectDescription)
	}
	if len(room.Exits) != 0 {
		t.Fatalf("exits = %v, want none", room.Exits)
	}
}

func TestMapRoomDescriptions(t *testing.T) {
	short, err := legacykr.EncodeEUCKR("가")
	if err != nil {
		t.Fatal(err)
	}

	data := makeRoomRecord(1234, "room name")
	data = appendInt32(data, 0)
	data = appendInt32(data, 0)
	data = appendInt32(data, 0)
	data = appendDescription(data, append(short, 0))
	data = appendDescription(data, []byte("long text\x00"))
	data = appendDescription(data, []byte("object text\x00"))

	room, warnings, err := MapRoomFile("not-a-room-file", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if room.ID != "room:01234" {
		t.Fatalf("room id = %q, want header-derived room:01234", room.ID)
	}
	if room.DisplayName != "room name" {
		t.Fatalf("display name = %q", room.DisplayName)
	}
	if room.ShortDescription != "가" || room.LongDescription != "long text" || room.ObjectDescription != "object text" {
		t.Fatalf("descriptions = %q / %q / %q", room.ShortDescription, room.LongDescription, room.ObjectDescription)
	}
}

func TestMapRoomLegacyScalarFieldsAndFlags(t *testing.T) {
	data := makeRoomRecord(123, "flag room")
	data[96] = 5
	data[97] = 40
	binary.LittleEndian.PutUint16(data[98:], 77)
	data[100] = 3
	binary.LittleEndian.PutUint16(data[102:], 456)
	copy(data[104:], []byte("north\x00"))
	data[184+1] = 0x41
	data[184+4] = 0x40
	data[184+5] = 0x02
	binary.LittleEndian.PutUint16(data[192:], 10)
	binary.LittleEndian.PutUint16(data[194:], 20)
	binary.LittleEndian.PutUint16(data[196:], 0)
	binary.LittleEndian.PutUint16(data[198:], 0)
	binary.LittleEndian.PutUint16(data[200:], 30)
	binary.LittleEndian.PutUint16(data[202:], 0)
	binary.LittleEndian.PutUint16(data[204:], 0)
	binary.LittleEndian.PutUint16(data[206:], 0)
	binary.LittleEndian.PutUint16(data[208:], 0)
	binary.LittleEndian.PutUint16(data[210:], 0)
	data[212] = 9
	binary.LittleEndian.PutUint32(data[216:], 111)
	binary.LittleEndian.PutUint32(data[220:], 222)
	binary.LittleEndian.PutUint16(data[224:], 333)
	binary.LittleEndian.PutUint32(data[336+cbin.LasttimeSize:], 444)
	binary.LittleEndian.PutUint32(data[336+cbin.LasttimeSize+4:], 555)
	binary.LittleEndian.PutUint16(data[336+cbin.LasttimeSize+8:], 666)
	binary.LittleEndian.PutUint32(data[456:], 1001)
	binary.LittleEndian.PutUint32(data[460:], 2002)
	data = appendInt32(data, 0)
	data = appendInt32(data, 0)
	data = appendInt32(data, 0)
	data = appendDescription(data, nil)
	data = appendDescription(data, nil)
	data = appendDescription(data, nil)

	room, warnings, err := MapRoomFile("rooms/r00/r00123", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	wantProperties := map[string]string{
		"minLevel":            "5",
		"maxLevel":            "40",
		"special":             "77",
		"trap":                "3",
		"trapExit":            "456",
		"track":               "north",
		"traffic":             "9",
		"beenHere":            "1001",
		"established":         "2002",
		"random":              "10,20,0,0,30,0,0,0,0,0",
		"perm_mon.0.interval": "111",
		"perm_mon.0.ltime":    "222",
		"perm_mon.0.misc":     "333",
		"perm_obj.1.interval": "444",
		"perm_obj.1.ltime":    "555",
		"perm_obj.1.misc":     "666",
	}
	for key, want := range wantProperties {
		if got := room.Properties[key]; got != want {
			t.Fatalf("property %s = %q, want %q; properties %+v", key, got, want, room.Properties)
		}
	}
	wantTags := []string{"darkAlways", "onePlayer", "onlyFamily", "onlyMarried"}
	if strings.Join(room.Metadata.Tags, ",") != strings.Join(wantTags, ",") {
		t.Fatalf("room tags = %+v, want %+v", room.Metadata.Tags, wantTags)
	}
	if string(room.Metadata.RawFields["track"]) != "north" {
		t.Fatalf("raw track = % X", room.Metadata.RawFields["track"])
	}
	if string(room.Metadata.RawFields["flags"]) != string([]byte{0, 0x41, 0, 0, 0x40, 0x02, 0, 0}) {
		t.Fatalf("raw room flags = % X", room.Metadata.RawFields["flags"])
	}
}

func TestMapRoomExits(t *testing.T) {
	data := makeRoomRecord(1, "start")
	data = appendInt32(data, 1)
	exitRecord := makeExitRecord("north", 2, 3)
	exitRecord[22] = 0x09
	exitRecord[24] = 0x08
	binary.LittleEndian.PutUint32(exitRecord[28:], 300)
	binary.LittleEndian.PutUint32(exitRecord[32:], 400)
	binary.LittleEndian.PutUint16(exitRecord[36:], 55)
	data = append(data, exitRecord...)
	data = appendInt32(data, 0)
	data = appendInt32(data, 0)
	data = appendDescription(data, nil)
	data = appendDescription(data, nil)
	data = appendDescription(data, nil)

	room, warnings, err := MapRoomFile("rooms/r00/r00001", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if len(room.Exits) != 1 {
		t.Fatalf("exits = %d, want 1", len(room.Exits))
	}
	exit := room.Exits[0]
	if exit.Name != "north" || exit.ToRoomID != "room:00002" {
		t.Fatalf("exit = %+v", exit)
	}
	wantFlags := []string{"secret", "closed", "noSee", "key:3"}
	if strings.Join(exit.Flags, ",") != strings.Join(wantFlags, ",") {
		t.Fatalf("exit flags = %v", exit.Flags)
	}
	if string(exit.Metadata.RawFields["flags"]) != string([]byte{0x09, 0, 0x08, 0}) {
		t.Fatalf("raw exit flags = % X", exit.Metadata.RawFields["flags"])
	}
	if got := int32(binary.LittleEndian.Uint32(exit.Metadata.RawFields["ltime.interval"])); got != 300 {
		t.Fatalf("raw exit interval = %d, want 300", got)
	}
	if got := int32(binary.LittleEndian.Uint32(exit.Metadata.RawFields["ltime.ltime"])); got != 400 {
		t.Fatalf("raw exit ltime = %d, want 400", got)
	}
	if got := int16(binary.LittleEndian.Uint16(exit.Metadata.RawFields["ltime.misc"])); got != 55 {
		t.Fatalf("raw exit misc = %d, want 55", got)
	}
}

func TestMapRoomFileBundleContents(t *testing.T) {
	data := makeRoomRecord(10, "loaded room")
	data = appendInt32(data, 0)
	data = appendInt32(data, 1)
	data = append(data, makeCreatureTree("orc", makeObjectTree("knife"))...)
	data = appendInt32(data, 1)
	data = append(data, makeObjectTree("chest", makeObjectTree("coin"))...)
	data = appendDescription(data, []byte("short\x00"))
	data = appendDescription(data, []byte("long\x00"))
	data = appendDescription(data, nil)

	bundle, err := MapRoomFileBundle("rooms/r00/r00010", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Warnings) != 0 {
		t.Fatalf("warnings = %v", bundle.Warnings)
	}
	if !bundle.ContentDecoded || bundle.Decoded.Creatures != 1 || bundle.Decoded.Objects != 3 {
		t.Fatalf("decoded = content %v stats %+v", bundle.ContentDecoded, bundle.Decoded)
	}
	if len(bundle.Creatures) != 1 || len(bundle.Room.CreatureIDs) != 1 {
		t.Fatalf("creatures = %+v room ids = %+v", bundle.Creatures, bundle.Room.CreatureIDs)
	}
	if bundle.Creatures[0].DisplayName != "orc" || len(bundle.Creatures[0].Inventory.ObjectIDs) != 1 {
		t.Fatalf("creature = %+v", bundle.Creatures[0])
	}
	if len(bundle.Objects) != 3 || len(bundle.Room.Objects.ObjectIDs) != 1 {
		t.Fatalf("objects = %+v room objects = %+v", bundle.Objects, bundle.Room.Objects)
	}
	rootRoomObjectID := bundle.Room.Objects.ObjectIDs[0]
	if bundle.Objects[1].ID != rootRoomObjectID || bundle.Objects[1].Location.RoomID != bundle.Room.ID {
		t.Fatalf("room root object = %+v, want id %q in room %q", bundle.Objects[1], rootRoomObjectID, bundle.Room.ID)
	}
	if bundle.Objects[2].Location.ContainerID != rootRoomObjectID {
		t.Fatalf("contained object = %+v, want container %q", bundle.Objects[2], rootRoomObjectID)
	}
}

func TestMapRoomFileBundlePreservesPartialRoomOnContentError(t *testing.T) {
	data := makeRoomRecord(11, "partial room")
	data = appendInt32(data, 0)
	data = appendInt32(data, 0)
	data = appendInt32(data, 1)
	data = append(data, make([]byte, cbin.ObjectSize)...)

	bundle, err := MapRoomFileBundle("rooms/r00/r00011", data)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Room.ID != "room:00011" || bundle.ContentError == "" {
		t.Fatalf("bundle = %+v", bundle)
	}
	if len(bundle.Warnings) == 0 || !strings.Contains(strings.Join(bundle.Warnings, "\n"), "decode room contents") {
		t.Fatalf("warnings = %v", bundle.Warnings)
	}
}

func TestMapMalformedRoomWarningAndError(t *testing.T) {
	if _, _, err := MapRoomFile("rooms/r00/r00001", []byte{1, 2, 3}); err == nil {
		t.Fatal("expected short room record error")
	}

	data := makeRoomRecord(2, "bad exits")
	data = appendInt32(data, -1)

	room, warnings, err := MapRoomFile("rooms/r00/r00002", data)
	if err != nil {
		t.Fatal(err)
	}
	if room.ID != "room:00002" || room.DisplayName != "bad exits" {
		t.Fatalf("room = %+v", room)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warnings for malformed room")
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "invalid room exit count") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func makeCreatureTree(name string, inventory ...[]byte) []byte {
	data := make([]byte, cbin.CreatureSize)
	copy(data[0:], []byte(name+"\x00"))
	data = appendInt32(data, len(inventory))
	for _, item := range inventory {
		data = append(data, item...)
	}
	return data
}

func makeObjectTree(name string, children ...[]byte) []byte {
	data := make([]byte, cbin.ObjectSize)
	copy(data[0:], []byte(name+"\x00"))
	data = appendInt32(data, len(children))
	for _, child := range children {
		data = append(data, child...)
	}
	return data
}

func makeMinimalRoom(t *testing.T, number int, name string) []byte {
	t.Helper()

	data := makeRoomRecord(number, name)
	data = appendInt32(data, 0)
	data = appendInt32(data, 0)
	data = appendInt32(data, 0)
	data = appendDescription(data, nil)
	data = appendDescription(data, nil)
	data = appendDescription(data, nil)
	return data
}

func makeRoomRecord(number int, name string) []byte {
	data := make([]byte, cbin.RoomSize)
	binary.LittleEndian.PutUint16(data, uint16(int16(number)))
	copy(data[2:], []byte(name))
	return data
}

func makeExitRecord(name string, room int, key byte) []byte {
	const (
		exitRoomOff = 20
		exitKeyOff  = 40
	)

	data := make([]byte, cbin.ExitSize)
	copy(data, []byte(name))
	binary.LittleEndian.PutUint16(data[exitRoomOff:], uint16(int16(room)))
	data[exitKeyOff] = key
	return data
}

func appendInt32(data []byte, n int) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(int32(n)))
	return append(data, buf[:]...)
}

func appendDescription(data []byte, desc []byte) []byte {
	data = appendInt32(data, len(desc))
	return append(data, desc...)
}
