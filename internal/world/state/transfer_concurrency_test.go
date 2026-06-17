package state_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

// testProtoID is the prototype used for test objects that require a non-zero
// PrototypeID to pass validation.
const testProtoID = model.PrototypeID("prototype:test-item")

// addTestPrototype adds a minimal prototype that satisfies object validation.
func addTestPrototype(t *testing.T, loaded *worldload.World) {
	t.Helper()
	mustAddObjectPrototype(t, loaded, model.ObjectPrototype{
		ID:          testProtoID,
		DisplayName: "Test Item",
	})
}

// setupTransferTestWorld creates a world with two creatures in the same room,
// each starting with a known gold balance, and a room with a known set of floor
// objects. It returns the world plus the IDs needed for transfer-path tests.
func setupTransferTestWorld(t *testing.T) (*state.World, model.CreatureID, model.CreatureID, model.RoomID) {
	t.Helper()
	loaded := worldload.NewWorld()

	roomID := model.RoomID("room:test")
	mustAddRoom(t, loaded, model.Room{
		ID:          roomID,
		DisplayName: "Test Room",
	})

	fromID := model.CreatureID("creature:from")
	toID := model.CreatureID("creature:to")
	startGold := 10000

	mustAddCreature(t, loaded, model.Creature{
		ID:          fromID,
		Kind:        model.CreatureKindPlayer,
		DisplayName: "From",
		RoomID:      roomID,
		PlayerID:    "player:from",
		Stats:       map[string]int{"gold": startGold, "hpCurrent": 100, "hpMax": 100},
	})
	mustAddCreature(t, loaded, model.Creature{
		ID:          toID,
		Kind:        model.CreatureKindPlayer,
		DisplayName: "To",
		RoomID:      roomID,
		PlayerID:    "player:to",
		Stats:       map[string]int{"gold": startGold, "hpCurrent": 100, "hpMax": 100},
	})

	mustAddPlayer(t, loaded, model.Player{
		ID:          "player:from",
		DisplayName: "From",
		CreatureID:  fromID,
		RoomID:      roomID,
	})
	mustAddPlayer(t, loaded, model.Player{
		ID:          "player:to",
		DisplayName: "To",
		CreatureID:  toID,
		RoomID:      roomID,
	})

	w := state.NewWorld(loaded)
	return w, fromID, toID, roomID
}

// TestConcurrentTransferCreatureGoldConservation hammers TransferCreatureGold
// from N goroutines in both directions simultaneously. After all goroutines
// finish, the total gold across both creatures must equal the starting total.
// This catches: (a) lost gold from non-atomic read-modify-write, (b) duplicated
// gold from concurrent credits without debits.
func TestConcurrentTransferCreatureGoldConservation(t *testing.T) {
	w, fromID, toID, _ := setupTransferTestWorld(t)
	defer w.Close()

	const goroutines = 50
	const transfersPerG = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Half the goroutines send from->to, half send to->from, all with
	// amount=1. Every transfer must be atomic. Total gold must be conserved.
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < transfersPerG; j++ {
				_, _, _, _ = w.TransferCreatureGold(fromID, toID, 1)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < transfersPerG; j++ {
				_, _, _, _ = w.TransferCreatureGold(toID, fromID, 1)
			}
		}()
	}
	wg.Wait()

	from, _ := w.Creature(fromID)
	to, _ := w.Creature(toID)
	totalGold := from.Stats["gold"] + to.Stats["gold"]
	if totalGold != 20000 {
		t.Fatalf("gold not conserved: from=%d + to=%d = %d, want 20000",
			from.Stats["gold"], to.Stats["gold"], totalGold)
	}
}

// TestConcurrentStealNoDuplication creates an object in one creature's
// inventory and has N goroutines all try to StealCreatureInventoryObject at
// once. Exactly one should succeed; the object must end up in exactly one
// inventory.
func TestConcurrentStealNoDuplication(t *testing.T) {
	loaded := worldload.NewWorld()
	roomID := model.RoomID("room:steal")
	mustAddRoom(t, loaded, model.Room{ID: roomID, DisplayName: "Steal Room"})

	fromID := model.CreatureID("creature:victim")
	toID := model.CreatureID("creature:thief")

	mustAddCreature(t, loaded, model.Creature{
		ID:          fromID,
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Victim",
		RoomID:      roomID,
		PlayerID:    "player:victim",
		Stats:       map[string]int{"gold": 0, "hpCurrent": 100, "hpMax": 100},
	})
	mustAddCreature(t, loaded, model.Creature{
		ID:          toID,
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Thief",
		RoomID:      roomID,
		PlayerID:    "player:thief",
		Stats:       map[string]int{"gold": 0, "hpCurrent": 100, "hpMax": 100},
	})
	mustAddPlayer(t, loaded, model.Player{ID: "player:victim", DisplayName: "Victim", CreatureID: fromID, RoomID: roomID})
	mustAddPlayer(t, loaded, model.Player{ID: "player:thief", DisplayName: "Thief", CreatureID: toID, RoomID: roomID})

	addTestPrototype(t, loaded)
	objID := model.ObjectInstanceID("object:steal-target")
	mustAddObject(t, loaded, model.ObjectInstance{
		ID:          objID,
		PrototypeID: testProtoID,
		Location:    model.ObjectLocation{CreatureID: fromID, Slot: "inventory"},
	})

	// Put object in victim's inventory.
	fromCreature := loaded.Creatures[fromID]
	fromCreature.Inventory.ObjectIDs = []model.ObjectInstanceID{objID}
	loaded.Creatures[fromID] = fromCreature

	w := state.NewWorld(loaded)
	defer w.Close()

	const goroutines = 50
	var successCount int64
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			ok, _ := w.StealCreatureInventoryObject(objID, fromID, toID)
			if ok {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}
	wg.Wait()

	if successCount != 1 {
		t.Fatalf("steal success count = %d, want exactly 1 (no duplication)", successCount)
	}

	// Verify the object exists in exactly one inventory.
	from, _ := w.Creature(fromID)
	to, _ := w.Creature(toID)
	fromHas := containsObjectID(from.Inventory.ObjectIDs, objID)
	toHas := containsObjectID(to.Inventory.ObjectIDs, objID)
	if fromHas == toHas {
		t.Fatalf("object in both or neither inventory: fromHas=%v toHas=%v", fromHas, toHas)
	}
	if !toHas {
		t.Fatal("object should be in thief (to) inventory after successful steal")
	}
}

// TestCon concurrentSellNoDuplication verifies that when N goroutines
// simultaneously try to sell the same object, exactly one succeeds. The gold
// credit must happen exactly once.
func TestConcurrentSellNoDuplication(t *testing.T) {
	loaded := worldload.NewWorld()
	roomID := model.RoomID("room:sell")
	mustAddRoom(t, loaded, model.Room{ID: roomID, DisplayName: "Shop"})

	cID := model.CreatureID("creature:seller")
	mustAddCreature(t, loaded, model.Creature{
		ID:          cID,
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Seller",
		RoomID:      roomID,
		PlayerID:    "player:seller",
		Stats:       map[string]int{"gold": 0, "hpCurrent": 100, "hpMax": 100},
	})
	mustAddPlayer(t, loaded, model.Player{ID: "player:seller", DisplayName: "Seller", CreatureID: cID, RoomID: roomID})

	addTestPrototype(t, loaded)
	objID := model.ObjectInstanceID("object:sell-target")
	mustAddObject(t, loaded, model.ObjectInstance{
		ID:          objID,
		PrototypeID: testProtoID,
		Location:    model.ObjectLocation{CreatureID: cID, Slot: "inventory"},
	})
	c := loaded.Creatures[cID]
	c.Inventory.ObjectIDs = []model.ObjectInstanceID{objID}
	loaded.Creatures[cID] = c

	w := state.NewWorld(loaded)
	defer w.Close()

	const goroutines = 50
	const sellPrice = 500
	var soldCount int64
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, sold, err := w.SellObjectFromCreatureInventory(objID, cID, sellPrice)
			if err == nil && sold {
				atomic.AddInt64(&soldCount, 1)
			}
		}()
	}
	wg.Wait()

	if soldCount != 1 {
		t.Fatalf("sell count = %d, want exactly 1", soldCount)
	}

	seller, _ := w.Creature(cID)
	if seller.Stats["gold"] != sellPrice {
		t.Fatalf("seller gold = %d, want %d", seller.Stats["gold"], sellPrice)
	}

	// Object should no longer exist.
	_, exists := w.Object(objID)
	if exists {
		t.Fatal("sold object still exists in world state")
	}
}

// TestConcurrentDropGoldConservation hammers DropCreatureGoldToRoom from N
// goroutines. After all goroutines finish, the sum of (creature gold + sum of
// all money objects' value) must equal the starting gold.
func TestConcurrentDropGoldConservation(t *testing.T) {
	w, fromID, _, roomID := setupTransferTestWorld(t)
	defer w.Close()

	const startGold = 10000
	const goroutines = 30
	const amountPerDrop = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_, _, _, _ = w.DropCreatureGoldToRoom(fromID, roomID, amountPerDrop)
			}
		}()
	}
	wg.Wait()

	// Count total gold: creature stat + sum of all money objects in the room.
	from, _ := w.Creature(fromID)
	room, _ := w.Room(roomID)

	creatureGold := from.Stats["gold"]
	floorGold := 0
	for _, oid := range room.Objects.ObjectIDs {
		obj, ok := w.Object(oid)
		if !ok {
			continue
		}
		if obj.Properties["kind"] == string(model.ObjectKindMoney) {
			floorGold += parseTestInt(obj.Properties["value"])
		}
	}

	total := creatureGold + floorGold
	if total != startGold {
		t.Fatalf("gold not conserved: creature=%d + floor=%d = %d, want %d",
			creatureGold, floorGold, total, startGold)
	}
}

// TestConcurrentMoveObjectNoDuplication creates an object on the room floor and
// has N goroutines all try to MoveObjectToCreatureInventory simultaneously.
// The object must end up in exactly one place (the creature inventory) and must
// not exist on the floor. MoveObject is idempotent for same-location moves, so
// all calls may succeed -- the invariant is placement exclusivity, not call count.
func TestConcurrentMoveObjectNoDuplication(t *testing.T) {
	loaded := worldload.NewWorld()
	roomID := model.RoomID("room:floor")
	mustAddRoom(t, loaded, model.Room{ID: roomID, DisplayName: "Floor"})

	targetID := model.CreatureID("creature:taker")
	mustAddCreature(t, loaded, model.Creature{
		ID:          targetID,
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Taker",
		RoomID:      roomID,
		PlayerID:    "player:taker",
		Stats:       map[string]int{"gold": 0, "hpCurrent": 100, "hpMax": 100},
	})
	mustAddPlayer(t, loaded, model.Player{ID: "player:taker", DisplayName: "Taker", CreatureID: targetID, RoomID: roomID})

	addTestPrototype(t, loaded)
	objID := model.ObjectInstanceID("object:floor-item")
	mustAddObject(t, loaded, model.ObjectInstance{
		ID:          objID,
		PrototypeID: testProtoID,
		Location:    model.ObjectLocation{RoomID: roomID},
	})

	room := loaded.Rooms[roomID]
	room.Objects.ObjectIDs = []model.ObjectInstanceID{objID}
	loaded.Rooms[roomID] = room

	w := state.NewWorld(loaded)
	defer w.Close()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = w.MoveObjectToCreatureInventory(objID, targetID)
		}()
	}
	wg.Wait()

	// Object must be in exactly one place: the creature's inventory.
	target, _ := w.Creature(targetID)
	roomAfter, _ := w.Room(roomID)
	inCreature := containsObjectID(target.Inventory.ObjectIDs, objID)
	onFloor := containsObjectID(roomAfter.Objects.ObjectIDs, objID)
	if !inCreature || onFloor {
		t.Fatalf("object placement wrong: inCreature=%v onFloor=%v", inCreature, onFloor)
	}
	// Object must appear exactly once in the inventory.
	count := 0
	for _, id := range target.Inventory.ObjectIDs {
		if id == objID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("object appears %d times in inventory, want exactly 1", count)
	}
}

// TestConcurrentPurchaseGoldConservation hammers
// PurchaseObjectToCreatureInventory from N goroutines with the same creature.
// The creature's gold must be debited correctly for each successful purchase;
// total spent + remaining must equal the starting gold.
func TestConcurrentPurchaseGoldConservation(t *testing.T) {
	loaded := worldload.NewWorld()
	roomID := model.RoomID("room:shop")
	mustAddRoom(t, loaded, model.Room{ID: roomID, DisplayName: "Shop"})

	cID := model.CreatureID("creature:buyer")
	const startGold = 10000
	mustAddCreature(t, loaded, model.Creature{
		ID:          cID,
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Buyer",
		RoomID:      roomID,
		PlayerID:    "player:buyer",
		Stats:       map[string]int{"gold": startGold, "hpCurrent": 100, "hpMax": 100},
	})
	mustAddPlayer(t, loaded, model.Player{ID: "player:buyer", DisplayName: "Buyer", CreatureID: cID, RoomID: roomID})

	// Use a prototype that can be cloned repeatedly.
	protoID := model.PrototypeID("prototype:potion")
	mustAddObjectPrototype(t, loaded, model.ObjectPrototype{
		ID:          protoID,
		DisplayName: "Potion",
		Properties:  map[string]string{"value": "100"},
	})
	// Template object for cloning.
	mustAddObject(t, loaded, model.ObjectInstance{
		ID:          model.ObjectInstanceID(protoID),
		PrototypeID: protoID,
		Location:    model.ObjectLocation{RoomID: roomID},
	})

	w := state.NewWorld(loaded)
	defer w.Close()

	const goroutines = 50
	const price = 100
	var purchaseCount int64
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, _, ok, _ := w.PurchaseObjectToCreatureInventory(model.ObjectInstanceID(protoID), cID, price)
			if ok {
				atomic.AddInt64(&purchaseCount, 1)
			}
		}()
	}
	wg.Wait()

	buyer, _ := w.Creature(cID)
	spent := int(purchaseCount) * price
	total := buyer.Stats["gold"] + spent
	if total != startGold {
		t.Fatalf("gold not conserved: remaining=%d + spent=%d = %d, want %d (purchases=%d)",
			buyer.Stats["gold"], spent, total, startGold, purchaseCount)
	}
}

// TestConcurrentStoreTakeFromContainer exercises the container store/take path
// from N goroutines. Objects placed in a container must be retrievable exactly
// once; container shot counters must stay consistent.
func TestConcurrentStoreTakeFromContainer(t *testing.T) {
	loaded := worldload.NewWorld()
	roomID := model.RoomID("room:container")
	mustAddRoom(t, loaded, model.Room{ID: roomID, DisplayName: "Container Room"})

	cID := model.CreatureID("creature:container-user")
	mustAddCreature(t, loaded, model.Creature{
		ID:          cID,
		Kind:        model.CreatureKindPlayer,
		DisplayName: "ContainerUser",
		RoomID:      roomID,
		PlayerID:    "player:container-user",
		Stats:       map[string]int{"gold": 0, "hpCurrent": 100, "hpMax": 100},
	})
	mustAddPlayer(t, loaded, model.Player{ID: "player:container-user", DisplayName: "ContainerUser", CreatureID: cID, RoomID: roomID})

	addTestPrototype(t, loaded)
	// Create 20 inventory items.
	var itemIDs []model.ObjectInstanceID
	for i := 0; i < 20; i++ {
		oid := model.ObjectInstanceID(fmt.Sprintf("object:item-%d", i))
		mustAddObject(t, loaded, model.ObjectInstance{
			ID:          oid,
			PrototypeID: testProtoID,
			Location:    model.ObjectLocation{CreatureID: cID, Slot: "inventory"},
		})
		itemIDs = append(itemIDs, oid)
	}
	c := loaded.Creatures[cID]
	c.Inventory.ObjectIDs = itemIDs
	loaded.Creatures[cID] = c

	// Create a container in the room.
	containerID := model.ObjectInstanceID("object:chest")
	mustAddObject(t, loaded, model.ObjectInstance{
		ID:          containerID,
		PrototypeID: testProtoID,
		Location:    model.ObjectLocation{RoomID: roomID},
		Properties: map[string]string{
			"OCONTN":       "1",
			"shotsCurrent": "0",
			"shotsMax":     "100",
		},
	})
	room := loaded.Rooms[roomID]
	room.Objects.ObjectIDs = []model.ObjectInstanceID{containerID}
	loaded.Rooms[roomID] = room

	w := state.NewWorld(loaded)
	defer w.Close()

	// Phase 1: Store all items concurrently.
	var wg sync.WaitGroup
	wg.Add(len(itemIDs))
	for _, oid := range itemIDs {
		go func(oid model.ObjectInstanceID) {
			defer wg.Done()
			_, _, _, _ = w.StoreCreatureInventoryObjectInContainer(oid, cID, containerID, 100)
		}(oid)
	}
	wg.Wait()

	// Verify: all items are in container, none in inventory.
	c2, _ := w.Creature(cID)
	if len(c2.Inventory.ObjectIDs) != 0 {
		t.Fatalf("expected 0 items in inventory after store, got %d", len(c2.Inventory.ObjectIDs))
	}
	container, _ := w.Object(containerID)
	if len(container.Contents.ObjectIDs) != 20 {
		t.Fatalf("expected 20 items in container, got %d", len(container.Contents.ObjectIDs))
	}

	// Phase 2: Take all items concurrently.
	wg.Add(len(itemIDs))
	for _, oid := range itemIDs {
		go func(oid model.ObjectInstanceID) {
			defer wg.Done()
			_, _, _ = w.TakeContainerObjectToCreatureInventory(oid, containerID, cID)
		}(oid)
	}
	wg.Wait()

	// Verify: all items back in inventory, container empty.
	c3, _ := w.Creature(cID)
	if len(c3.Inventory.ObjectIDs) != 20 {
		t.Fatalf("expected 20 items in inventory after take, got %d", len(c3.Inventory.ObjectIDs))
	}
	container, _ = w.Object(containerID)
	if len(container.Contents.ObjectIDs) != 0 {
		t.Fatalf("expected 0 items in container after take, got %d", len(container.Contents.ObjectIDs))
	}
}

func containsObjectID(ids []model.ObjectInstanceID, want model.ObjectInstanceID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func parseTestInt(s string) int {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
