package cbin

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestDecodeObjectTreeNested(t *testing.T) {
	data := treeObject("root",
		treeObject("box", treeObject("gem")),
		treeObject("coin"),
	)

	node, err := DecodeObjectTree(data, false)
	if err != nil {
		t.Fatal(err)
	}
	if node.Offset != 0 || node.Record.Name.Text != "root" {
		t.Fatalf("root = offset %d name %q", node.Offset, node.Record.Name.Text)
	}
	if len(node.Children) != 2 {
		t.Fatalf("children len = %d, want 2", len(node.Children))
	}
	if node.Children[0].Offset != ObjectSize+4 || node.Children[0].Record.Name.Text != "box" {
		t.Fatalf("first child = offset %d name %q", node.Children[0].Offset, node.Children[0].Record.Name.Text)
	}
	if len(node.Children[0].Children) != 1 || node.Children[0].Children[0].Record.Name.Text != "gem" {
		t.Fatalf("nested child = %+v", node.Children[0].Children)
	}
	if node.Children[1].Record.Name.Text != "coin" {
		t.Fatalf("second child name = %q, want coin", node.Children[1].Record.Name.Text)
	}
	if node.Stats.Objects != 4 || node.Stats.MaxDepth != 3 {
		t.Fatalf("stats = %+v", node.Stats)
	}
	if len(node.Warnings) != 0 {
		t.Fatalf("warnings = %+v", node.Warnings)
	}
}

func TestDecodeCreatureTreeInventory(t *testing.T) {
	data := treeCreature("goblin",
		treeObject("bag", treeObject("stone")),
		treeObject("torch"),
	)

	node, err := DecodeCreatureTree(data)
	if err != nil {
		t.Fatal(err)
	}
	if node.Offset != 0 || node.Record.Name.Text != "goblin" {
		t.Fatalf("creature = offset %d name %q", node.Offset, node.Record.Name.Text)
	}
	if len(node.Inventory) != 2 {
		t.Fatalf("inventory len = %d, want 2", len(node.Inventory))
	}
	if node.Inventory[0].Offset != CreatureSize+4 || node.Inventory[0].Record.Name.Text != "bag" {
		t.Fatalf("first item = offset %d name %q", node.Inventory[0].Offset, node.Inventory[0].Record.Name.Text)
	}
	if len(node.Inventory[0].Children) != 1 || node.Inventory[0].Children[0].Record.Name.Text != "stone" {
		t.Fatalf("nested inventory = %+v", node.Inventory[0].Children)
	}
	if node.Inventory[1].Record.Name.Text != "torch" {
		t.Fatalf("second item name = %q, want torch", node.Inventory[1].Record.Name.Text)
	}
	if node.Stats.Creatures != 1 || node.Stats.Objects != 3 || node.Stats.MaxDepth != 3 {
		t.Fatalf("stats = %+v", node.Stats)
	}
}

func TestDecodeRoomTreeExitObjectDescription(t *testing.T) {
	data := treeRoom(123, "hall")
	data = treeAppendInt32(data, 1)
	data = append(data, treeExit("north", 456, 7)...)
	data = treeAppendInt32(data, 0)
	data = treeAppendInt32(data, 1)
	objectOffset := len(data)
	data = append(data, treeObject("key")...)
	shortOffset := len(data) + 4
	data = treeDescription(data, []byte("short\x00"))
	data = treeDescription(data, []byte("long text\x00"))
	data = treeDescription(data, []byte("objects here\x00"))

	node, err := DecodeRoomTree(data)
	if err != nil {
		t.Fatal(err)
	}
	if node.Offset != 0 || node.Record.Number != 123 || node.Record.Name.Text != "hall" {
		t.Fatalf("room = offset %d number %d name %q", node.Offset, node.Record.Number, node.Record.Name.Text)
	}
	if len(node.Exits) != 1 || node.Exits[0].Offset != RoomSize+4 {
		t.Fatalf("exits = %+v", node.Exits)
	}
	if node.Exits[0].Record.Name.Text != "north" || node.Exits[0].Record.Room != 456 || node.Exits[0].Record.Key != 7 {
		t.Fatalf("exit record = %+v", node.Exits[0].Record)
	}
	if len(node.Objects) != 1 || node.Objects[0].Offset != objectOffset || node.Objects[0].Record.Name.Text != "key" {
		t.Fatalf("objects = %+v", node.Objects)
	}
	if node.Descriptions[0].Offset != shortOffset || node.Record.ShortDescription.Text != "short" {
		t.Fatalf("short description = offset %d text %q", node.Descriptions[0].Offset, node.Record.ShortDescription.Text)
	}
	if node.Record.LongDescription.Text != "long text" || node.Record.ObjectDescription.Text != "objects here" {
		t.Fatalf("descriptions = %q / %q", node.Record.LongDescription.Text, node.Record.ObjectDescription.Text)
	}
	if node.Stats.Rooms != 1 || node.Stats.Exits != 1 || node.Stats.Objects != 1 ||
		node.Stats.Descriptions != 3 || node.Stats.DescriptionBytes != 29 || node.Stats.MaxDepth != 2 {
		t.Fatalf("stats = %+v", node.Stats)
	}
}

func TestDecodeObjectTreeTrailingBankBytesAllowDeny(t *testing.T) {
	data := append(treeObject("bank"), []byte{0xde, 0xad, 0xbe, 0xef}...)

	_, err := DecodeObjectTree(data, false)
	if err == nil || !strings.Contains(err.Error(), "trailing 4 bytes after object tree") {
		t.Fatalf("DecodeObjectTree strict error = %v", err)
	}

	node, err := DecodeObjectTree(data, true)
	if err != nil {
		t.Fatal(err)
	}
	if node.Record.Name.Text != "bank" || node.Stats.TrailingBytes != 4 || node.Stats.Objects != 1 {
		t.Fatalf("node = name %q stats %+v", node.Record.Name.Text, node.Stats)
	}
}

func treeObject(name string, children ...[]byte) []byte {
	data := make([]byte, ObjectSize)
	copy(data[objectNameOff:], []byte(name+"\x00"))
	data = treeAppendInt32(data, int32(len(children)))
	for _, child := range children {
		data = append(data, child...)
	}
	return data
}

func treeCreature(name string, inventory ...[]byte) []byte {
	data := make([]byte, CreatureSize)
	copy(data[creatureNameOff:], []byte(name+"\x00"))
	data = treeAppendInt32(data, int32(len(inventory)))
	for _, item := range inventory {
		data = append(data, item...)
	}
	return data
}

func treeRoom(number int16, name string) []byte {
	data := make([]byte, RoomSize)
	binary.LittleEndian.PutUint16(data[roomNumberOff:], uint16(number))
	copy(data[roomNameOff:], []byte(name+"\x00"))
	return data
}

func treeExit(name string, room int16, key byte) []byte {
	data := make([]byte, ExitSize)
	copy(data[exitNameOff:], []byte(name+"\x00"))
	binary.LittleEndian.PutUint16(data[exitRoomOff:], uint16(room))
	data[exitKeyOff] = key
	return data
}

func treeDescription(data []byte, desc []byte) []byte {
	data = treeAppendInt32(data, int32(len(desc)))
	return append(data, desc...)
}

func treeAppendInt32(data []byte, value int32) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(value))
	return append(data, buf[:]...)
}
