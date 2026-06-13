package store

import (
	"context"
	"errors"
	"testing"

	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

func TestMemoryStoreLoadBootstrapReturnsDefensiveCopy(t *testing.T) {
	world := worldload.NewWorld()
	mustAddRoom(t, world, model.Room{
		ID:          "room:one",
		DisplayName: "One",
		Properties:  map[string]string{"zone": "start"},
		Objects:     model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:coin"}},
		Metadata: model.Metadata{
			RawFields: map[string][]byte{"raw": []byte("abc")},
			Tags:      []string{"loaded"},
		},
	})
	mustAddProto(t, world, model.ObjectPrototype{ID: "proto:coin", DisplayName: "Coin"})
	mustAddObject(t, world, model.ObjectInstance{
		ID:          "object:coin",
		PrototypeID: "proto:coin",
		Quantity:    1,
		Location:    model.ObjectLocation{RoomID: "room:one"},
	})
	mustAddFamily(t, world, model.Family{
		ID:          2,
		Slot:        2,
		DisplayName: "무영문",
		BossName:    "무영풍",
		JoinSubsidy: 100,
		Members: []model.FamilyMember{{
			DisplayName: "무영풍",
			Class:       10,
			Metadata:    model.Metadata{RawFields: map[string][]byte{"line": []byte("10 boss")}},
		}},
		Metadata: model.Metadata{
			RawFields: map[string][]byte{"line": []byte("2 무영문 무영풍 100")},
		},
	})
	world.MarriageInvites = map[model.SpecialID][]string{7: {"alice", "bob"}}

	store, err := NewMemoryStore(world)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	world.Rooms["room:one"] = model.Room{ID: "room:one", DisplayName: "mutated"}

	loaded, err := store.LoadBootstrap(context.Background())
	if err != nil {
		t.Fatalf("LoadBootstrap: %v", err)
	}
	room := loaded.Rooms["room:one"]
	if room.DisplayName != "One" || room.Properties["zone"] != "start" {
		t.Fatalf("loaded room was not isolated from source mutation: %+v", room)
	}

	room.Properties["zone"] = "changed"
	room.Objects.ObjectIDs[0] = "object:other"
	room.Metadata.RawFields["raw"][0] = 'z'
	loaded.Rooms["room:one"] = room

	reloaded, err := store.LoadBootstrap(context.Background())
	if err != nil {
		t.Fatalf("LoadBootstrap after mutation: %v", err)
	}
	room = reloaded.Rooms["room:one"]
	if room.Properties["zone"] != "start" || room.Objects.ObjectIDs[0] != "object:coin" || string(room.Metadata.RawFields["raw"]) != "abc" {
		t.Fatalf("store leaked returned snapshot mutation: %+v", room)
	}
	family := reloaded.Families[2]
	if family.DisplayName != "무영문" || family.Members[0].DisplayName != "무영풍" ||
		string(family.Members[0].Metadata.RawFields["line"]) != "10 boss" {
		t.Fatalf("family was not loaded defensively: %+v", family)
	}
	family.Members[0].DisplayName = "changed"
	family.Members[0].Metadata.RawFields["line"][0] = 'X'
	reloaded.Families[2] = family

	reloaded, err = store.LoadBootstrap(context.Background())
	if err != nil {
		t.Fatalf("LoadBootstrap after family mutation: %v", err)
	}
	family = reloaded.Families[2]
	if family.Members[0].DisplayName != "무영풍" || string(family.Members[0].Metadata.RawFields["line"]) != "10 boss" {
		t.Fatalf("store leaked returned family mutation: %+v", family)
	}
	reloaded.MarriageInvites[7][0] = "changed"

	reloaded, err = store.LoadBootstrap(context.Background())
	if err != nil {
		t.Fatalf("LoadBootstrap after marriage invite mutation: %v", err)
	}
	if got := reloaded.MarriageInvites[7][0]; got != "alice" {
		t.Fatalf("store leaked returned marriage invite mutation: %+v", reloaded.MarriageInvites)
	}
}

func TestMemoryStoreSaveAndMoveRuntimeEntities(t *testing.T) {
	store := NewEmptyMemoryStore()
	ctx := context.Background()

	err := store.Save(ctx, ChangeSet{
		Rooms: []model.Room{
			{ID: "room:one", DisplayName: "One"},
			{ID: "room:two", DisplayName: "Two"},
		},
		ObjectPrototypes: []model.ObjectPrototype{
			{ID: "proto:sword", DisplayName: "Sword"},
		},
		Players: []model.Player{{
			ID:          "player:alice",
			DisplayName: "Alice",
			CreatureID:  "creature:alice",
			RoomID:      "room:one",
		}},
		Creatures: []model.Creature{{
			ID:          "creature:alice",
			Kind:        model.CreatureKindPlayer,
			DisplayName: "Alice",
			PlayerID:    "player:alice",
			RoomID:      "room:one",
		}},
		Families: []model.Family{{
			ID:          2,
			Slot:        2,
			DisplayName: "무영문",
		}},
		Objects: []model.ObjectInstance{{
			ID:          "object:sword",
			PrototypeID: "proto:sword",
			Quantity:    1,
			Location:    model.ObjectLocation{RoomID: "room:one"},
		}},
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := store.MoveObject(ctx, "object:sword", model.ObjectLocation{CreatureID: "creature:alice", Slot: "right"}); err != nil {
		t.Fatalf("MoveObject: %v", err)
	}
	if err := store.MovePlayer(ctx, "player:alice", "room:two"); err != nil {
		t.Fatalf("MovePlayer: %v", err)
	}

	world, err := store.LoadBootstrap(ctx)
	if err != nil {
		t.Fatalf("LoadBootstrap: %v", err)
	}
	if got := world.Objects["object:sword"].Location; got.CreatureID != "creature:alice" || got.Slot != "right" {
		t.Fatalf("object location = %+v", got)
	}
	if containsObject(world.Rooms["room:one"].Objects.ObjectIDs, "object:sword") {
		t.Fatalf("old room still references moved object: %+v", world.Rooms["room:one"].Objects.ObjectIDs)
	}
	creature := world.Creatures["creature:alice"]
	if !containsObject(creature.Inventory.ObjectIDs, "object:sword") || creature.Equipment["right"] != "object:sword" {
		t.Fatalf("creature references = inventory:%+v equipment:%+v", creature.Inventory.ObjectIDs, creature.Equipment)
	}
	if world.Players["player:alice"].RoomID != "room:two" || world.Creatures["creature:alice"].RoomID != "room:two" {
		t.Fatalf("player/creature rooms = %q/%q", world.Players["player:alice"].RoomID, world.Creatures["creature:alice"].RoomID)
	}
	if containsPlayer(world.Rooms["room:one"].PlayerIDs, "player:alice") || !containsPlayer(world.Rooms["room:two"].PlayerIDs, "player:alice") {
		t.Fatalf("room player refs = one:%+v two:%+v", world.Rooms["room:one"].PlayerIDs, world.Rooms["room:two"].PlayerIDs)
	}
	if containsCreature(world.Rooms["room:one"].CreatureIDs, "creature:alice") || !containsCreature(world.Rooms["room:two"].CreatureIDs, "creature:alice") {
		t.Fatalf("room creature refs = one:%+v two:%+v", world.Rooms["room:one"].CreatureIDs, world.Rooms["room:two"].CreatureIDs)
	}
	if got := world.Families[2].DisplayName; got != "무영문" {
		t.Fatalf("family display name = %q", got)
	}
}

func TestMemoryStoreMoveObjectRejectsMissingDestinationAtomically(t *testing.T) {
	store := NewEmptyMemoryStore()
	ctx := context.Background()
	err := store.Save(ctx, ChangeSet{
		Rooms:            []model.Room{{ID: "room:one", DisplayName: "One"}},
		ObjectPrototypes: []model.ObjectPrototype{{ID: "proto:coin", DisplayName: "Coin"}},
		Objects: []model.ObjectInstance{{
			ID:          "object:coin",
			PrototypeID: "proto:coin",
			Quantity:    1,
			Location:    model.ObjectLocation{RoomID: "room:one"},
		}},
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	err = store.MoveObject(ctx, "object:coin", model.ObjectLocation{RoomID: "room:missing"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("MoveObject error = %v, want ErrNotFound", err)
	}

	world, err := store.LoadBootstrap(ctx)
	if err != nil {
		t.Fatalf("LoadBootstrap: %v", err)
	}
	if got := world.Objects["object:coin"].Location.RoomID; got != "room:one" {
		t.Fatalf("object moved after failed call, room = %q", got)
	}
	if !containsObject(world.Rooms["room:one"].Objects.ObjectIDs, "object:coin") {
		t.Fatalf("room lost object after failed call: %+v", world.Rooms["room:one"].Objects.ObjectIDs)
	}
}

func TestMemoryStoreRejectsObjectContainmentCycle(t *testing.T) {
	store := NewEmptyMemoryStore()
	ctx := context.Background()
	err := store.Save(ctx, ChangeSet{
		Rooms: []model.Room{{ID: "room:one", DisplayName: "One"}},
		ObjectPrototypes: []model.ObjectPrototype{
			{ID: "proto:bag", DisplayName: "Bag"},
			{ID: "proto:gem", DisplayName: "Gem"},
		},
		Objects: []model.ObjectInstance{
			{
				ID:          "object:bag",
				PrototypeID: "proto:bag",
				Quantity:    1,
				Location:    model.ObjectLocation{RoomID: "room:one"},
				Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:gem"}},
			},
			{
				ID:          "object:gem",
				PrototypeID: "proto:gem",
				Quantity:    1,
				Location:    model.ObjectLocation{ContainerID: "object:bag"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	err = store.MoveObject(ctx, "object:bag", model.ObjectLocation{ContainerID: "object:gem"})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("MoveObject error = %v, want ErrInvalid", err)
	}
}

func mustAddRoom(t *testing.T, world *worldload.World, room model.Room) {
	t.Helper()
	if err := world.AddRoom(room); err != nil {
		t.Fatalf("AddRoom: %v", err)
	}
}

func mustAddProto(t *testing.T, world *worldload.World, proto model.ObjectPrototype) {
	t.Helper()
	if err := world.AddObjectPrototype(proto); err != nil {
		t.Fatalf("AddObjectPrototype: %v", err)
	}
}

func mustAddObject(t *testing.T, world *worldload.World, object model.ObjectInstance) {
	t.Helper()
	if err := world.AddObjectInstance(object); err != nil {
		t.Fatalf("AddObjectInstance: %v", err)
	}
}

func mustAddFamily(t *testing.T, world *worldload.World, family model.Family) {
	t.Helper()
	if err := world.AddFamily(family); err != nil {
		t.Fatalf("AddFamily: %v", err)
	}
}

func containsObject(ids []model.ObjectInstanceID, id model.ObjectInstanceID) bool {
	for _, existing := range ids {
		if existing == id {
			return true
		}
	}
	return false
}

func containsPlayer(ids []model.PlayerID, id model.PlayerID) bool {
	for _, existing := range ids {
		if existing == id {
			return true
		}
	}
	return false
}

func containsCreature(ids []model.CreatureID, id model.CreatureID) bool {
	for _, existing := range ids {
		if existing == id {
			return true
		}
	}
	return false
}
