package creaturemap

import (
	"encoding/binary"
	"testing"

	"muhan/internal/persist/cbin"
	"muhan/internal/world/model"
)

func TestMapCreatureTreeInventoryAndStats(t *testing.T) {
	data := testCreature("goblin", "a small goblin", "hello", 12,
		testObject("bag", testObject("gem")),
	)
	for _, bit := range []int{0, 41, 61} {
		data[testCreatureFlagsOff+bit/8] |= 1 << uint(bit%8)
	}
	node, err := cbin.DecodeCreatureTree(data)
	if err != nil {
		t.Fatal(err)
	}

	result := MapCreatureTree("room:00007", "room:00007", node)
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %v", result.Warnings)
	}
	if result.Creature.ID != model.CreatureID("creature:room:00007:00000000") {
		t.Fatalf("creature id = %q", result.Creature.ID)
	}
	if result.Creature.DisplayName != "goblin" || result.Creature.Description != "a small goblin" {
		t.Fatalf("creature text = %+v", result.Creature)
	}
	if result.Creature.Level != 12 || result.Creature.Stats["roomNumber"] != 7 {
		t.Fatalf("stats = %+v", result.Creature.Stats)
	}
	for _, want := range []string{"MPERMT", "permanent", "MDEATH", "deathDescription", "MSUMMO", "summoner"} {
		if !hasTag(result.Creature.Metadata.Tags, want) {
			t.Fatalf("tags = %+v, want %q", result.Creature.Metadata.Tags, want)
		}
	}
	if len(result.Creature.Inventory.ObjectIDs) != 1 || len(result.Objects) != 2 {
		t.Fatalf("inventory = %+v objects = %+v", result.Creature.Inventory, result.Objects)
	}
	rootObjectID := result.Creature.Inventory.ObjectIDs[0]
	if result.Objects[0].ID != rootObjectID || result.Objects[0].Location.CreatureID != result.Creature.ID {
		t.Fatalf("root object = %+v", result.Objects[0])
	}
	if result.Objects[1].Location.ContainerID != rootObjectID {
		t.Fatalf("child object location = %+v, want container %q", result.Objects[1].Location, rootObjectID)
	}
}

const testCreatureFlagsOff = 412

func testCreature(name, description, talk string, level byte, inventory ...[]byte) []byte {
	data := make([]byte, cbin.CreatureSize)
	copy(data[0:], []byte(name+"\x00"))
	copy(data[80:], []byte(description+"\x00"))
	copy(data[160:], []byte(talk+"\x00"))
	data[creatureLevelOff] = level
	binary.LittleEndian.PutUint16(data[creatureRoomNumberOff:], uint16(7))
	data = appendInt32(data, len(inventory))
	for _, item := range inventory {
		data = append(data, item...)
	}
	return data
}

func testObject(name string, children ...[]byte) []byte {
	data := make([]byte, cbin.ObjectSize)
	copy(data[0:], []byte(name+"\x00"))
	data = appendInt32(data, len(children))
	for _, child := range children {
		data = append(data, child...)
	}
	return data
}

func appendInt32(data []byte, n int) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(int32(n)))
	return append(data, buf[:]...)
}

func hasTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}
