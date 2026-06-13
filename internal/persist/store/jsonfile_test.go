package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/persist/jsonstore"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

func TestJSONFileStoreLoadsExistingJSONAndReturnsDefensiveCopy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime", "world.json")
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
	world.MarriageInvites = map[model.SpecialID][]string{7: {"alice", "bob"}}
	if err := jsonstore.WriteJSON(path, world); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	store, err := NewJSONFileStore(path)
	if err != nil {
		t.Fatalf("NewJSONFileStore: %v", err)
	}
	loaded, err := store.LoadBootstrap(context.Background())
	if err != nil {
		t.Fatalf("LoadBootstrap: %v", err)
	}

	room := loaded.Rooms["room:one"]
	room.Properties["zone"] = "changed"
	room.Objects.ObjectIDs[0] = "object:other"
	room.Metadata.RawFields["raw"][0] = 'z'
	loaded.Rooms["room:one"] = room
	loaded.MarriageInvites[7][0] = "changed"

	reloaded, err := store.LoadBootstrap(context.Background())
	if err != nil {
		t.Fatalf("LoadBootstrap after mutation: %v", err)
	}
	room = reloaded.Rooms["room:one"]
	if room.Properties["zone"] != "start" || room.Objects.ObjectIDs[0] != "object:coin" || string(room.Metadata.RawFields["raw"]) != "abc" {
		t.Fatalf("store leaked returned snapshot mutation: %+v", room)
	}
	if got := reloaded.MarriageInvites[7][0]; got != "alice" {
		t.Fatalf("marriage invite leaked returned snapshot mutation: %q", got)
	}
}

func TestJSONFileStorePersistsSaveAndMovesAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime", "world.json")
	store, err := NewJSONFileStore(path)
	if err != nil {
		t.Fatalf("NewJSONFileStore: %v", err)
	}
	ctx := context.Background()

	err = store.Save(ctx, ChangeSet{
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
		Creatures: []model.Creature{
			{
				ID:          "creature:alice",
				Kind:        model.CreatureKindPlayer,
				DisplayName: "Alice",
				PlayerID:    "player:alice",
				RoomID:      "room:one",
			},
			{
				ID:          "creature:guard",
				Kind:        model.CreatureKindNPC,
				DisplayName: "Guard",
				RoomID:      "room:one",
			},
		},
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
	if err := store.MoveCreature(ctx, "creature:guard", "room:two"); err != nil {
		t.Fatalf("MoveCreature: %v", err)
	}
	if err := store.MovePlayer(ctx, "player:alice", "room:two"); err != nil {
		t.Fatalf("MovePlayer: %v", err)
	}

	restarted, err := NewJSONFileStore(path)
	if err != nil {
		t.Fatalf("NewJSONFileStore after restart: %v", err)
	}
	loaded, err := restarted.LoadBootstrap(ctx)
	if err != nil {
		t.Fatalf("LoadBootstrap after restart: %v", err)
	}

	if got := loaded.Objects["object:sword"].Location; got.CreatureID != "creature:alice" || got.Slot != "right" {
		t.Fatalf("object location = %+v", got)
	}
	creature := loaded.Creatures["creature:alice"]
	if !containsObject(creature.Inventory.ObjectIDs, "object:sword") || creature.Equipment["right"] != "object:sword" {
		t.Fatalf("creature references = inventory:%+v equipment:%+v", creature.Inventory.ObjectIDs, creature.Equipment)
	}
	if loaded.Players["player:alice"].RoomID != "room:two" || loaded.Creatures["creature:alice"].RoomID != "room:two" {
		t.Fatalf("player/creature rooms = %q/%q", loaded.Players["player:alice"].RoomID, loaded.Creatures["creature:alice"].RoomID)
	}
	if loaded.Creatures["creature:guard"].RoomID != "room:two" {
		t.Fatalf("guard room = %q", loaded.Creatures["creature:guard"].RoomID)
	}
	if containsObject(loaded.Rooms["room:one"].Objects.ObjectIDs, "object:sword") {
		t.Fatalf("old room still references moved object: %+v", loaded.Rooms["room:one"].Objects.ObjectIDs)
	}
	if containsPlayer(loaded.Rooms["room:one"].PlayerIDs, "player:alice") || !containsPlayer(loaded.Rooms["room:two"].PlayerIDs, "player:alice") {
		t.Fatalf("room player refs = one:%+v two:%+v", loaded.Rooms["room:one"].PlayerIDs, loaded.Rooms["room:two"].PlayerIDs)
	}
	if containsCreature(loaded.Rooms["room:one"].CreatureIDs, "creature:guard") || !containsCreature(loaded.Rooms["room:two"].CreatureIDs, "creature:guard") {
		t.Fatalf("room creature refs = one:%+v two:%+v", loaded.Rooms["room:one"].CreatureIDs, loaded.Rooms["room:two"].CreatureIDs)
	}
}

func TestJSONFileStoreSaveRejectsInvalidChangeSetWithoutPersisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "world.json")
	store, err := NewJSONFileStore(path)
	if err != nil {
		t.Fatalf("NewJSONFileStore: %v", err)
	}
	ctx := context.Background()

	if err := store.Save(ctx, ChangeSet{
		Rooms: []model.Room{{ID: "room:one", DisplayName: "One"}},
	}); err != nil {
		t.Fatalf("initial Save: %v", err)
	}
	before := mustReadFile(t, path)

	err = store.Save(ctx, ChangeSet{
		Rooms: []model.Room{{ID: "room:two", DisplayName: "Two"}},
		Players: []model.Player{{
			ID:          "player:alice",
			DisplayName: "Alice",
			RoomID:      "room:missing",
		}},
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Save error = %v, want ErrNotFound", err)
	}

	after := mustReadFile(t, path)
	if string(after) != string(before) {
		t.Fatalf("file changed after rejected Save\nbefore:\n%s\nafter:\n%s", before, after)
	}
	restarted, err := NewJSONFileStore(path)
	if err != nil {
		t.Fatalf("NewJSONFileStore after rejected Save: %v", err)
	}
	loaded, err := restarted.LoadBootstrap(ctx)
	if err != nil {
		t.Fatalf("LoadBootstrap: %v", err)
	}
	if _, ok := loaded.Rooms["room:two"]; ok {
		t.Fatalf("partial ChangeSet persisted room: %+v", loaded.Rooms["room:two"])
	}
	if _, ok := loaded.Players["player:alice"]; ok {
		t.Fatalf("partial ChangeSet persisted player: %+v", loaded.Players["player:alice"])
	}
}

func TestJSONFileStoreRejectsInvalidExistingJSONWorld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "world.json")
	data, err := json.Marshal(worldload.World{
		Rooms: map[model.RoomID]model.Room{
			"room:bad": {ID: "room:bad"},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = NewJSONFileStore(path)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("NewJSONFileStore error = %v, want ErrInvalid", err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	return data
}
