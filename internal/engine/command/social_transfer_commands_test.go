package command

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/persist/legacykr"
	"muhan/internal/world/model"
)

type fakeSocialTransferWorld struct {
	rooms      map[model.RoomID]model.Room
	players    map[model.PlayerID]model.Player
	creatures  map[model.CreatureID]model.Creature
	objects    map[model.ObjectInstanceID]model.ObjectInstance
	prototypes map[model.PrototypeID]model.ObjectPrototype

	active       []string
	activeActors map[string]string

	destroys []model.ObjectInstanceID
	clones   []model.PrototypeID
}

func (w *fakeSocialTransferWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *fakeSocialTransferWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *fakeSocialTransferWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *fakeSocialTransferWorld) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	o, ok := w.objects[id]
	return o, ok
}

func (w *fakeSocialTransferWorld) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	p, ok := w.prototypes[id]
	return p, ok
}

func (w *fakeSocialTransferWorld) MoveObjectToCreatureInventory(objID model.ObjectInstanceID, crtID model.CreatureID) error {
	return nil
}

func (w *fakeSocialTransferWorld) CloneObjectToCreatureInventory(sourceID model.ObjectInstanceID, creatureID model.CreatureID) (model.ObjectInstanceID, error) {
	protoID := model.PrototypeID(sourceID)
	w.clones = append(w.clones, protoID)

	cloneID := model.ObjectInstanceID(fmt.Sprintf("%s-clone", sourceID))
	w.objects[cloneID] = model.ObjectInstance{
		ID:          cloneID,
		PrototypeID: protoID,
		Location:    model.ObjectLocation{CreatureID: creatureID, Slot: "inventory"},
	}

	crt := w.creatures[creatureID]
	crt.Inventory.ObjectIDs = append(crt.Inventory.ObjectIDs, cloneID)
	w.creatures[creatureID] = crt

	return cloneID, nil
}

func (w *fakeSocialTransferWorld) DestroyCreatureInventoryObject(objID model.ObjectInstanceID, creatureID model.CreatureID) (bool, error) {
	w.destroys = append(w.destroys, objID)

	crt := w.creatures[creatureID]
	var kept []model.ObjectInstanceID
	for _, id := range crt.Inventory.ObjectIDs {
		if id != objID {
			kept = append(kept, id)
		}
	}
	crt.Inventory.ObjectIDs = kept
	w.creatures[creatureID] = crt

	return true, nil
}

func (w *fakeSocialTransferWorld) SetCreatureProperty(creatureID model.CreatureID, key, value string) (model.Creature, error) {
	crt := w.creatures[creatureID]
	if crt.Properties == nil {
		crt.Properties = map[string]string{}
	}
	crt.Properties[key] = value
	w.creatures[creatureID] = crt
	return crt, nil
}

func (w *fakeSocialTransferWorld) SetCreatureStat(creatureID model.CreatureID, key string, val int) error {
	crt := w.creatures[creatureID]
	if crt.Stats == nil {
		crt.Stats = map[string]int{}
	}
	crt.Stats[key] = val
	w.creatures[creatureID] = crt
	return nil
}

func (w *fakeSocialTransferWorld) SetObjectProperty(objectID model.ObjectInstanceID, key, value string) (model.ObjectInstance, error) {
	obj := w.objects[objectID]
	if obj.Properties == nil {
		obj.Properties = map[string]string{}
	}
	obj.Properties[key] = value
	w.objects[objectID] = obj
	return obj, nil
}

func (w *fakeSocialTransferWorld) ActiveSessions() []string {
	return w.active
}

func (w *fakeSocialTransferWorld) SessionActor(id string) (string, bool) {
	val, ok := w.activeActors[id]
	return val, ok
}

func tradeTestWorld() *fakeSocialTransferWorld {
	return &fakeSocialTransferWorld{
		rooms: map[model.RoomID]model.Room{
			"room:plaza": {
				ID:          "room:plaza",
				DisplayName: "광장",
				CreatureIDs: []model.CreatureID{"creature:alice", "creature:merchant"},
				PlayerIDs:   []model.PlayerID{"player:alice"},
			},
		},
		players: map[model.PlayerID]model.Player{
			"player:alice": {
				ID:          "player:alice",
				DisplayName: "Alice",
				CreatureID:  "creature:alice",
				RoomID:      "room:plaza",
			},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:          "creature:alice",
				DisplayName: "Alice",
				Kind:        model.CreatureKindPlayer,
				PlayerID:    "player:alice",
				RoomID:      "room:plaza",
				Inventory: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
					"object:wanted-item",
				}},
				Stats: map[string]int{"experience": 1000},
			},
			"creature:merchant": {
				ID:          "creature:merchant",
				DisplayName: "Merchant",
				Kind:        model.CreatureKindMonster,
				RoomID:      "room:plaza",
				Metadata:    model.Metadata{Tags: []string{"MTRADE"}},
				Stats: map[string]int{
					"carry[0]": 110,
					"carry[5]": 220,
				},
			},
		},
		objects: map[model.ObjectInstanceID]model.ObjectInstance{
			"object:wanted-item": {
				ID:          "object:wanted-item",
				PrototypeID: "object:o01:10",
				Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
			},
		},
		prototypes: map[model.PrototypeID]model.ObjectPrototype{
			"object:o01:10": {
				ID:          "object:o01:10",
				DisplayName: "사과",
				Properties: map[string]string{
					"name":   "사과",
					"key[0]": "사과",
				},
			},
			"object:o02:20": {
				ID:          "object:o02:20",
				DisplayName: "바나나",
				Properties: map[string]string{
					"name":        "바나나",
					"key[0]":      "바나나",
					"questNumber": "1",
				},
			},
		},
	}
}

func TestTradeHandler_Basic(t *testing.T) {
	world := &fakeSocialTransferWorld{
		rooms: map[model.RoomID]model.Room{
			"room:plaza": {
				ID:          "room:plaza",
				DisplayName: "광장",
				CreatureIDs: []model.CreatureID{"creature:alice", "creature:merchant"},
				PlayerIDs:   []model.PlayerID{"player:alice"},
			},
		},
		players: map[model.PlayerID]model.Player{
			"player:alice": {
				ID:          "player:alice",
				DisplayName: "Alice",
				CreatureID:  "creature:alice",
				RoomID:      "room:plaza",
			},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:          "creature:alice",
				DisplayName: "Alice",
				Kind:        model.CreatureKindPlayer,
				PlayerID:    "player:alice",
				RoomID:      "room:plaza",
				Inventory: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
					"object:wanted-item",
				}},
				Stats: map[string]int{"experience": 1000},
			},
			"creature:merchant": {
				ID:          "creature:merchant",
				DisplayName: "Merchant",
				Kind:        model.CreatureKindMonster,
				RoomID:      "room:plaza",
				Metadata:    model.Metadata{Tags: []string{"MTRADE"}},
				Stats: map[string]int{
					"carry[0]": 110,
					"carry[5]": 220,
				},
			},
		},
		objects: map[model.ObjectInstanceID]model.ObjectInstance{
			"object:wanted-item": {
				ID:          "object:wanted-item",
				PrototypeID: "object:o01:10",
				Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
				Properties:  map[string]string{"shotsCurrent": "10", "shotsMax": "10"},
			},
		},
		prototypes: map[model.PrototypeID]model.ObjectPrototype{
			"object:o01:10": {
				ID:          "object:o01:10",
				DisplayName: "사과",
				Properties: map[string]string{
					"name":   "사과",
					"key[0]": "사과",
				},
			},
			"object:o02:20": {
				ID:          "object:o02:20",
				DisplayName: "바나나",
				Properties: map[string]string{
					"name":        "바나나",
					"key[0]":      "바나나",
					"questNumber": "1",
				},
			},
		},
	}

	handler := NewTradeHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"사과", "Merchant"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler trade basic error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	if !slices.Contains(world.destroys, model.ObjectInstanceID("object:wanted-item")) {
		t.Fatalf("expected object:wanted-item to be destroyed")
	}

	if !slices.Contains(world.clones, model.PrototypeID("object:o02:20")) {
		t.Fatalf("expected reward item object:o02:20 to be cloned")
	}

	aliceCrt := world.creatures["creature:alice"]
	if aliceCrt.Properties["quest_completed_1"] != "1" {
		t.Fatalf("expected quest completion flag to be set")
	}

	if aliceCrt.Stats["experience"] != 1000+120 {
		t.Fatalf("experience = %d, want 1120", aliceCrt.Stats["experience"])
	}
}

func TestTradeHandlerReadsPropertyBackedTradeFlagLikeC(t *testing.T) {
	world := tradeTestWorld()
	merchant := world.creatures["creature:merchant"]
	merchant.Metadata.Tags = nil
	merchant.Properties = map[string]string{"flags": "MTRADE"}
	world.creatures[merchant.ID] = merchant
	item := world.objects["object:wanted-item"]
	item.Properties = map[string]string{"shotsCurrent": "10", "shotsMax": "10"}
	world.objects[item.ID] = item
	handler := NewTradeHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"사과", "Merchant"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if !slices.Contains(world.destroys, model.ObjectInstanceID("object:wanted-item")) {
		t.Fatalf("expected property-backed trade flag to allow wanted item destruction")
	}
	if !slices.Contains(world.clones, model.PrototypeID("object:o02:20")) {
		t.Fatalf("expected property-backed trade flag to allow reward clone")
	}
}

func TestTradeHandlerRendersLegacyQuestRewardOutput(t *testing.T) {
	world := tradeTestWorld()
	item := world.objects["object:wanted-item"]
	item.Properties = map[string]string{"shotsCurrent": "10", "shotsMax": "10"}
	world.objects[item.ID] = item
	handler := NewTradeHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"사과", "Merchant"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	want := "Merchant가 \"고맙습니다. 절 위해 사과를 찾아주시다니..\n" +
		"당신에게 바나나로 보답을 하고싶습니다.\"라고 말합니다.\n" +
		"Merchant가 당신에게 바나나를 줍니다.\n" +
		"임무를 완수했습니다! 버리지 마십시요!\n" +
		"당신은 버리면 그걸 다시 주울 수 없습니다.\n" +
		"당신은 경험치 120 를 얻었습니다.\n"
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestTradeHandlerRendersLegacyNoRewardOutput(t *testing.T) {
	world := tradeTestWorld()
	merchant := world.creatures["creature:merchant"]
	merchant.Stats = map[string]int{"carry[0]": 110}
	world.creatures[merchant.ID] = merchant
	item := world.objects["object:wanted-item"]
	item.Properties = map[string]string{"shotsCurrent": "10", "shotsMax": "10"}
	world.objects[item.ID] = item
	handler := NewTradeHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"사과", "Merchant"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	want := "Merchant가 \"고맙습니다! 사과가 필요했는데 잘됐군요.\n" +
		"그런데 당신에게 줄게 없는데..\"라고 말합니다.\n"
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if !slices.Contains(world.destroys, model.ObjectInstanceID("object:wanted-item")) {
		t.Fatalf("expected wanted item to be destroyed for no-reward trade")
	}
	if len(world.clones) != 0 {
		t.Fatalf("unexpected clones for no-reward trade: %+v", world.clones)
	}
}

func TestTradeHandlerBroadcastsTradeToRoomOnlyLikeLegacy(t *testing.T) {
	world := tradeTestWorld()
	item := world.objects["object:wanted-item"]
	item.Properties = map[string]string{"shotsCurrent": "10", "shotsMax": "10"}
	world.objects[item.ID] = item
	handler := NewTradeHandler(world)

	var roomBroadcasts []roomBroadcastRecord
	var globalBroadcasts []string
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &roomBroadcasts)
	ctx.Values["game.broadcast"] = func(cmd struct{ Write string }) error {
		globalBroadcasts = append(globalBroadcasts, cmd.Write)
		return nil
	}

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"사과", "Merchant"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if len(globalBroadcasts) != 0 {
		t.Fatalf("global broadcasts = %+v, want none", globalBroadcasts)
	}
	if len(roomBroadcasts) != 1 {
		t.Fatalf("room broadcasts = %+v, want one", roomBroadcasts)
	}
	want := roomBroadcastRecord{
		RoomID:  "room:plaza",
		Exclude: "session:alice",
		Text:    "Alice이 Merchant에게 사과를 교환합니다.\n",
	}
	if roomBroadcasts[0] != want {
		t.Fatalf("room broadcast = %+v, want %+v", roomBroadcasts[0], want)
	}
}

func TestTradeHandlerRejectsOneByteTargetPrefixLikeLegacyFindCrt(t *testing.T) {
	world := tradeTestWorld()
	item := world.objects["object:wanted-item"]
	item.Properties = map[string]string{"shotsCurrent": "10", "shotsMax": "10"}
	world.objects[item.ID] = item
	handler := NewTradeHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"사과", "M"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그것은 여기 없습니다.\n" {
		t.Fatalf("status/output = %d/%q, want missing target", status, ctx.OutputString())
	}
	if len(world.destroys) != 0 || len(world.clones) != 0 {
		t.Fatalf("trade mutated state for rejected one-byte target: destroys=%+v clones=%+v", world.destroys, world.clones)
	}
}

func TestTradeHandlerRendersLegacyNonTradeFlagDebugLine(t *testing.T) {
	world := tradeTestWorld()
	merchant := world.creatures["creature:merchant"]
	merchant.Metadata = model.Metadata{RawFields: map[string][]byte{"flags": {0, 0, 0, 0, 0x10}}}
	world.creatures[merchant.ID] = merchant
	handler := NewTradeHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"사과", "Merchant"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	want := "당신은 Merchant와 교역할 수 없습니다.\n10 1 0 0\n"
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if len(world.destroys) != 0 || len(world.clones) != 0 {
		t.Fatalf("trade mutated state for non-trade target: destroys=%+v clones=%+v", world.destroys, world.clones)
	}
}

func TestTradeHandlerRendersLegacyNonTradeParticle(t *testing.T) {
	world := tradeTestWorld()
	merchant := world.creatures["creature:merchant"]
	merchant.DisplayName = "상인"
	merchant.Metadata = model.Metadata{}
	world.creatures[merchant.ID] = merchant
	handler := NewTradeHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"사과", "상인"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	want := "당신은 상인과 교역할 수 없습니다.\n0 0 0 0\n"
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestTradeHandlerHidesInvisibleInventoryItemWithoutPDINVI(t *testing.T) {
	world := tradeTestWorld()
	item := world.objects["object:wanted-item"]
	item.Metadata.Tags = append(item.Metadata.Tags, "OINVIS")
	item.Properties = map[string]string{"shotsCurrent": "10", "shotsMax": "10"}
	world.objects[item.ID] = item
	handler := NewTradeHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"사과", "Merchant"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 그런 물건을 갖고 있지 않습니다.\n" {
		t.Fatalf("output = %q", got)
	}
	if len(world.destroys) != 0 || len(world.clones) != 0 {
		t.Fatalf("trade mutated state for hidden item: destroys=%+v clones=%+v", world.destroys, world.clones)
	}
}

func TestTradeHandlerAllowsInvisibleInventoryItemWithPDINVI(t *testing.T) {
	world := tradeTestWorld()
	item := world.objects["object:wanted-item"]
	item.Metadata.Tags = append(item.Metadata.Tags, "OINVIS")
	item.Properties = map[string]string{"shotsCurrent": "10", "shotsMax": "10"}
	world.objects[item.ID] = item
	alice := world.creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "PDINVI")
	world.creatures[alice.ID] = alice
	handler := NewTradeHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"사과", "Merchant"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if !slices.Contains(world.destroys, model.ObjectInstanceID("object:wanted-item")) {
		t.Fatalf("expected invisible wanted item to be destroyed with PDINVI")
	}
	if !slices.Contains(world.clones, model.PrototypeID("object:o02:20")) {
		t.Fatalf("expected reward clone with PDINVI")
	}
}

func TestTradeHandlerUsesLegacyCarryMaxItemSlots(t *testing.T) {
	world := tradeTestWorld()
	merchant := world.creatures["creature:merchant"]
	merchant.Stats = map[string]int{
		"carry[1]": 110,
		"carry[6]": 220,
	}
	world.creatures[merchant.ID] = merchant
	item := world.objects["object:wanted-item"]
	item.Properties = map[string]string{"shotsCurrent": "10", "shotsMax": "10"}
	world.objects[item.ID] = item
	handler := NewTradeHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"사과", "Merchant"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "Merchant가 \"난 그런거 필요없어요!\"라고 말합니다.\n" {
		t.Fatalf("output = %q", got)
	}
	if len(world.destroys) != 0 || len(world.clones) != 0 {
		t.Fatalf("trade mutated state for sparse carry stock: destroys=%+v clones=%+v", world.destroys, world.clones)
	}
}

func TestTradeHandlerRejectsSpentNonMiscItemLikeLegacy(t *testing.T) {
	world := tradeTestWorld()
	handler := NewTradeHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"사과", "Merchant"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "Merchant가 \"난 그런거 필요없어요!\"라고 말합니다.\n" {
		t.Fatalf("output = %q", got)
	}
	if len(world.destroys) != 0 || len(world.clones) != 0 {
		t.Fatalf("trade mutated state for spent item: destroys=%+v clones=%+v", world.destroys, world.clones)
	}
}

func TestTradeHandlerRequiresActualNameAndKeyMatchLikeLegacy(t *testing.T) {
	tests := []struct {
		name                string
		target              string
		displayNameOverride string
		properties          map[string]string
	}{
		{
			name:   "same prototype but changed object name",
			target: "썩은",
			properties: map[string]string{
				"name":         "썩은 사과",
				"key[0]":       "사과",
				"shotsCurrent": "10",
				"shotsMax":     "10",
			},
		},
		{
			name:                "same prototype but changed display name",
			target:              "썩은",
			displayNameOverride: "썩은 사과",
			properties: map[string]string{
				"key[0]":       "사과",
				"shotsCurrent": "10",
				"shotsMax":     "10",
			},
		},
		{
			name:   "same prototype but changed primary key",
			target: "사과",
			properties: map[string]string{
				"name":         "사과",
				"key[0]":       "풋사과",
				"shotsCurrent": "10",
				"shotsMax":     "10",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := tradeTestWorld()
			item := world.objects["object:wanted-item"]
			item.DisplayNameOverride = tt.displayNameOverride
			item.Properties = tt.properties
			world.objects[item.ID] = item
			handler := NewTradeHandler(world)
			ctx := &Context{ActorID: "player:alice"}

			status, err := handler(ctx, ResolvedCommand{
				Args:   []string{tt.target, "Merchant"},
				Values: []int64{1, 1},
			})
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}
			if got := ctx.OutputString(); got != "Merchant가 \"난 그런거 필요없어요!\"라고 말합니다.\n" {
				t.Fatalf("output = %q", got)
			}
			if len(world.destroys) != 0 || len(world.clones) != 0 {
				t.Fatalf("trade mutated state for mismatched object: destroys=%+v clones=%+v", world.destroys, world.clones)
			}
		})
	}
}

func TestTradeHandlerUsesLegacyPrefixOrderInsteadOfExactFirst(t *testing.T) {
	world := tradeTestWorld()
	alice := world.creatures["creature:alice"]
	alice.Inventory.ObjectIDs = []model.ObjectInstanceID{"object:apple-peel", "object:wanted-item"}
	world.creatures[alice.ID] = alice
	world.objects["object:apple-peel"] = model.ObjectInstance{
		ID:          "object:apple-peel",
		PrototypeID: "object:o01:11",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties: map[string]string{
			"name":         "사과껍질",
			"key[0]":       "사과껍질",
			"shotsCurrent": "10",
			"shotsMax":     "10",
		},
	}
	world.prototypes["object:o01:11"] = model.ObjectPrototype{
		ID:          "object:o01:11",
		DisplayName: "사과껍질",
		Properties: map[string]string{
			"name":   "사과껍질",
			"key[0]": "사과껍질",
		},
	}
	item := world.objects["object:wanted-item"]
	item.Properties = map[string]string{"shotsCurrent": "10", "shotsMax": "10"}
	world.objects[item.ID] = item
	handler := NewTradeHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"사과", "Merchant"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "Merchant가 \"난 그런거 필요없어요!\"라고 말합니다.\n" {
		t.Fatalf("output = %q", got)
	}
	if len(world.destroys) != 0 || len(world.clones) != 0 {
		t.Fatalf("trade mutated state for prefix-order mismatch: destroys=%+v clones=%+v", world.destroys, world.clones)
	}
}

func TestTradeHandlerStampsEventRewardOwner(t *testing.T) {
	world := tradeTestWorld()
	item := world.objects["object:wanted-item"]
	item.Properties = map[string]string{"shotsCurrent": "10", "shotsMax": "10"}
	world.objects[item.ID] = item
	proto := world.prototypes["object:o02:20"]
	proto.Metadata.Tags = []string{"OEVENT"}
	world.prototypes[proto.ID] = proto
	handler := NewTradeHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"사과", "Merchant"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	reward, ok := world.objects["object:o02:20-clone"]
	if !ok {
		t.Fatal("reward clone missing")
	}
	if got := reward.Properties["key[2]"]; got != "Alice" {
		t.Fatalf("reward owner key[2] = %q, want Alice", got)
	}
}

func TestTransExpHandler_Basic(t *testing.T) {
	world := &fakeSocialTransferWorld{
		rooms: map[model.RoomID]model.Room{
			"room:plaza": {
				ID:          "room:plaza",
				DisplayName: "광장",
				PlayerIDs:   []model.PlayerID{"player:donor", "player:receiver"},
			},
		},
		players: map[model.PlayerID]model.Player{
			"player:donor": {
				ID:          "player:donor",
				DisplayName: "Donor",
				CreatureID:  "creature:donor",
				RoomID:      "room:plaza",
			},
			"player:receiver": {
				ID:          "player:receiver",
				DisplayName: "Receiver",
				CreatureID:  "creature:receiver",
				RoomID:      "room:plaza",
			},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:donor": {
				ID:          "creature:donor",
				DisplayName: "Donor",
				Kind:        model.CreatureKindPlayer,
				PlayerID:    "player:donor",
				RoomID:      "room:plaza",
				Stats: map[string]int{
					"class":      10, // Caretaker
					"experience": 200000000,
					"familyID":   2,
				},
				Metadata: model.Metadata{Tags: []string{"PFAMIL"}},
			},
			"creature:receiver": {
				ID:          "creature:receiver",
				DisplayName: "Receiver",
				Kind:        model.CreatureKindPlayer,
				PlayerID:    "player:receiver",
				RoomID:      "room:plaza",
				Stats: map[string]int{
					"level":      10,
					"experience": 100,
					"familyID":   2,
				},
				Metadata: model.Metadata{Tags: []string{"PFAMIL"}},
			},
		},
	}

	handler := NewTransExpHandler(world)
	sent := map[string]string{}
	ctx := &Context{
		ActorID:   "player:donor",
		SessionID: "session:donor",
		Values: map[string]any{
			"game.activeSessions": func() []activeSession {
				return []activeSession{
					{ID: "session:donor", ActorID: "player:donor"},
					{ID: "session:receiver", ActorID: "player:receiver"},
				}
			},
			"game.sendToSession": func(id string, cmd struct{ Write string }) error {
				sent[id] += cmd.Write
				return nil
			},
		},
	}

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"1000000", "Receiver"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler trans_exp basic error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	donorExp := world.creatures["creature:donor"].Stats["experience"]
	receiverExp := world.creatures["creature:receiver"].Stats["experience"]

	if donorExp != 199000000 {
		t.Fatalf("donor exp = %d, want 199000000", donorExp)
	}

	expectedGain := 1000000 / 30
	if receiverExp != 100+expectedGain {
		t.Fatalf("receiver exp = %d, want %d", receiverExp, 100+expectedGain)
	}
	if _, ok := sent["player:receiver"]; ok {
		t.Fatalf("sent to player id instead of active session: %+v", sent)
	}
	if got := sent["session:receiver"]; !strings.Contains(got, "경험치 33333점을 나눠주었습니다.") {
		t.Fatalf("receiver session message = %q", got)
	}
}

func TestTransExpHandlerUsesLegacyAmountSlot(t *testing.T) {
	world := transExpTestWorld()
	handler := NewTransExpHandler(world)
	ctx := transExpTestContext()

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"Receiver", "1000"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "1000에서 1000000점 사이만 가능합니다." {
		t.Fatalf("output = %q", got)
	}
	if got := world.creatures["creature:donor"].Stats["experience"]; got != 200000000 {
		t.Fatalf("donor exp mutated to %d", got)
	}
}

func TestTransExpHandlerRejectsOneByteTargetPrefixLikeLegacyFindCrt(t *testing.T) {
	world := transExpTestWorld()
	handler := NewTransExpHandler(world)
	ctx := transExpTestContext()

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"1000", "R"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그런 사람은 여기 없어요!" {
		t.Fatalf("status/output = %d/%q, want missing receiver", status, ctx.OutputString())
	}
	if got := world.creatures["creature:donor"].Stats["experience"]; got != 200000000 {
		t.Fatalf("donor exp mutated to %d", got)
	}
	if got := world.creatures["creature:receiver"].Stats["experience"]; got != 100 {
		t.Fatalf("receiver exp mutated to %d", got)
	}
}

func TestTransExpHandlerUsesLegacyAtolPrefix(t *testing.T) {
	world := transExpTestWorld()
	handler := NewTransExpHandler(world)
	ctx := transExpTestContext()

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"1000점", "Receiver"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if want := "당신은 Receiver님에게 자신의 경험치 1000점을 나눠주었습니다."; !strings.Contains(ctx.OutputString(), want) {
		t.Fatalf("output missing %q:\n%s", want, ctx.OutputString())
	}
	if got := world.creatures["creature:donor"].Stats["experience"]; got != 199999000 {
		t.Fatalf("donor exp = %d, want 199999000", got)
	}
	if got := world.creatures["creature:receiver"].Stats["experience"]; got != 100+(1000/30) {
		t.Fatalf("receiver exp = %d", got)
	}
}

func TestTransExpHandlerUsesLegacyDailyExpndMaxBeforeFamilyID(t *testing.T) {
	world := transExpTestWorld()
	donor := world.creatures["creature:donor"]
	donor.Stats["familyID"] = 2
	donor.Stats["dailyExpndMax"] = 7
	world.creatures[donor.ID] = donor
	receiver := world.creatures["creature:receiver"]
	receiver.Stats["familyID"] = 2
	receiver.Stats["dailyExpndMax"] = 8
	world.creatures[receiver.ID] = receiver
	handler := NewTransExpHandler(world)
	ctx := transExpTestContext()

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"1000", "Receiver"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신의 패거리사람에게만 경험치전수가 가능합니다." {
		t.Fatalf("status/output = %d/%q, want legacy family mismatch", status, ctx.OutputString())
	}
	if got := world.creatures["creature:donor"].Stats["experience"]; got != 200000000 {
		t.Fatalf("donor exp mutated to %d", got)
	}
	if got := world.creatures["creature:receiver"].Stats["experience"]; got != 100 {
		t.Fatalf("receiver exp mutated to %d", got)
	}
}

func TestTransExpHandlerAcceptsLegacyDailyFamilyWithoutFamilyID(t *testing.T) {
	world := transExpTestWorld()
	donor := world.creatures["creature:donor"]
	delete(donor.Stats, "familyID")
	donor.Stats["dailyExpndMax"] = 7
	world.creatures[donor.ID] = donor
	receiver := world.creatures["creature:receiver"]
	delete(receiver.Stats, "familyID")
	receiver.Properties = map[string]string{"legacyDailyExpndMax": "7"}
	world.creatures[receiver.ID] = receiver
	handler := NewTransExpHandler(world)
	ctx := transExpTestContext()

	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"1000", "Receiver"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if want := "당신은 Receiver님에게 자신의 경험치 1000점을 나눠주었습니다."; !strings.Contains(ctx.OutputString(), want) {
		t.Fatalf("output missing %q:\n%s", want, ctx.OutputString())
	}
	if got := world.creatures["creature:donor"].Stats["experience"]; got != 199999000 {
		t.Fatalf("donor exp = %d, want 199999000", got)
	}
	if got := world.creatures["creature:receiver"].Stats["experience"]; got != 100+(1000/30) {
		t.Fatalf("receiver exp = %d", got)
	}
}

func transExpTestWorld() *fakeSocialTransferWorld {
	return &fakeSocialTransferWorld{
		rooms: map[model.RoomID]model.Room{
			"room:plaza": {
				ID:          "room:plaza",
				DisplayName: "광장",
				PlayerIDs:   []model.PlayerID{"player:donor", "player:receiver"},
			},
		},
		players: map[model.PlayerID]model.Player{
			"player:donor": {
				ID:          "player:donor",
				DisplayName: "Donor",
				CreatureID:  "creature:donor",
				RoomID:      "room:plaza",
			},
			"player:receiver": {
				ID:          "player:receiver",
				DisplayName: "Receiver",
				CreatureID:  "creature:receiver",
				RoomID:      "room:plaza",
			},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:donor": {
				ID:          "creature:donor",
				DisplayName: "Donor",
				Kind:        model.CreatureKindPlayer,
				PlayerID:    "player:donor",
				RoomID:      "room:plaza",
				Stats: map[string]int{
					"class":      10,
					"experience": 200000000,
					"familyID":   2,
				},
				Metadata: model.Metadata{Tags: []string{"PFAMIL"}},
			},
			"creature:receiver": {
				ID:          "creature:receiver",
				DisplayName: "Receiver",
				Kind:        model.CreatureKindPlayer,
				PlayerID:    "player:receiver",
				RoomID:      "room:plaza",
				Stats: map[string]int{
					"level":      10,
					"experience": 100,
					"familyID":   2,
				},
				Metadata: model.Metadata{Tags: []string{"PFAMIL"}},
			},
		},
	}
}

func transExpTestContext() *Context {
	return &Context{
		ActorID:   "player:donor",
		SessionID: "session:donor",
		Values: map[string]any{
			"game.activeSessions": func() []activeSession {
				return []activeSession{
					{ID: "session:donor", ActorID: "player:donor"},
					{ID: "session:receiver", ActorID: "player:receiver"},
				}
			},
			"game.sendToSession": func(string, struct{ Write string }) error {
				return nil
			},
		},
	}
}

func TestMarriageSendHandler_Basic(t *testing.T) {
	tempDir := t.TempDir()

	marriageDir := filepath.Join(tempDir, "player", "marriage")
	if err := os.MkdirAll(marriageDir, 0755); err != nil {
		t.Fatalf("failed to create temp marriage dir: %v", err)
	}

	aliceSpouseFile := filepath.Join(marriageDir, "alice")
	encodedBob, err := legacykr.EncodeEUCKR("Bob")
	if err != nil {
		t.Fatalf("failed to encode Bob: %v", err)
	}
	if err := os.WriteFile(aliceSpouseFile, encodedBob, 0644); err != nil {
		t.Fatalf("failed to write spouse file: %v", err)
	}

	world := &fakeSocialTransferWorld{
		rooms: map[model.RoomID]model.Room{
			"room:plaza": {
				ID:        "room:plaza",
				PlayerIDs: []model.PlayerID{"player:alice", "player:bob"},
			},
		},
		players: map[model.PlayerID]model.Player{
			"player:alice": {
				ID:          "player:alice",
				DisplayName: "Alice",
				CreatureID:  "creature:alice",
				RoomID:      "room:plaza",
				Metadata: model.Metadata{
					Tags: []string{"PMARRI"},
				},
			},
			"player:bob": {
				ID:          "player:bob",
				DisplayName: "Bob",
				CreatureID:  "creature:bob",
				RoomID:      "room:plaza",
			},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:          "creature:alice",
				DisplayName: "Alice",
				Kind:        model.CreatureKindPlayer,
				PlayerID:    "player:alice",
				RoomID:      "room:plaza",
			},
			"creature:bob": {
				ID:          "creature:bob",
				DisplayName: "Bob",
				Kind:        model.CreatureKindPlayer,
				PlayerID:    "player:bob",
				RoomID:      "room:plaza",
			},
		},
		active: []string{"session:bob"},
		activeActors: map[string]string{
			"session:bob": "player:bob",
		},
	}

	handler := NewMarriageSendHandler(world, tempDir)

	var sentTo string
	var sentMsg string
	sendFunc := func(id string, msg string) error {
		sentTo = id
		sentMsg = msg
		return nil
	}

	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.sendToSession": sendFunc,
		},
	}

	status, err := handler(ctx, ResolvedCommand{
		Input:   "사랑말 사랑해요",
		Args:    []string{"사랑해요"},
		CmdName: "사랑말",
	})
	if err != nil {
		t.Fatalf("handler marriage_send basic error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	if sentTo != "session:bob" {
		t.Fatalf("sent to session %q, want session:bob", sentTo)
	}

	expectedMsg := "\nAlice가 당신에게 \"사랑해요\"라고 이야기합니다."
	if sentMsg != expectedMsg {
		t.Fatalf("sent message = %q, want %q", sentMsg, expectedMsg)
	}
	if got := ctx.OutputString(); got != "Bob님에게 말을 전달하였습니다." {
		t.Fatalf("sender output = %q", got)
	}
}

func TestMarriageSendHandlerUsesLegacyEchoOutput(t *testing.T) {
	tempDir := t.TempDir()
	marriageDir := filepath.Join(tempDir, "player", "marriage")
	if err := os.MkdirAll(marriageDir, 0755); err != nil {
		t.Fatalf("failed to create temp marriage dir: %v", err)
	}
	encodedBob, err := legacykr.EncodeEUCKR("Bob")
	if err != nil {
		t.Fatalf("failed to encode Bob: %v", err)
	}
	if err := os.WriteFile(filepath.Join(marriageDir, "alice"), encodedBob, 0644); err != nil {
		t.Fatalf("failed to write spouse file: %v", err)
	}

	world := marriageSendTestWorld()
	alice := world.creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "PLECHO")
	world.creatures[alice.ID] = alice
	handler := NewMarriageSendHandler(world, tempDir)
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.sendToSession": func(string, string) error { return nil },
		},
	}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"사랑해요"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 Bob에게 \"사랑해요\"라고 이야기합니다." {
		t.Fatalf("output = %q", got)
	}
}

func TestMarriageSendHandlerReadsPropertyBackedMarriageAndEchoFlags(t *testing.T) {
	tempDir := t.TempDir()
	marriageDir := filepath.Join(tempDir, "player", "marriage")
	if err := os.MkdirAll(marriageDir, 0755); err != nil {
		t.Fatalf("failed to create temp marriage dir: %v", err)
	}
	encodedBob, err := legacykr.EncodeEUCKR("Bob")
	if err != nil {
		t.Fatalf("failed to encode Bob: %v", err)
	}
	if err := os.WriteFile(filepath.Join(marriageDir, "alice"), encodedBob, 0644); err != nil {
		t.Fatalf("failed to write spouse file: %v", err)
	}

	world := marriageSendTestWorld()
	alicePlayer := world.players["player:alice"]
	alicePlayer.Metadata.Tags = nil
	world.players[alicePlayer.ID] = alicePlayer
	alice := world.creatures["creature:alice"]
	alice.Properties = map[string]string{"flags": "PMARRI|PLECHO"}
	world.creatures[alice.ID] = alice
	var sentTo string
	var sentMsg string
	handler := NewMarriageSendHandler(world, tempDir)
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.sendToSession": func(id string, msg string) error {
				sentTo = id
				sentMsg = msg
				return nil
			},
		},
	}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"사랑해요"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if sentTo != "session:bob" || sentMsg != "\nAlice가 당신에게 \"사랑해요\"라고 이야기합니다." {
		t.Fatalf("sent = %q/%q, want spouse message", sentTo, sentMsg)
	}
	if got := ctx.OutputString(); got != "당신은 Bob에게 \"사랑해요\"라고 이야기합니다." {
		t.Fatalf("output = %q", got)
	}
}

func TestMarriageSendHandlerPreservesLegacyCutCommandSpaces(t *testing.T) {
	tempDir := t.TempDir()
	marriageDir := filepath.Join(tempDir, "player", "marriage")
	if err := os.MkdirAll(marriageDir, 0755); err != nil {
		t.Fatalf("failed to create temp marriage dir: %v", err)
	}
	encodedBob, err := legacykr.EncodeEUCKR("Bob")
	if err != nil {
		t.Fatalf("failed to encode Bob: %v", err)
	}
	if err := os.WriteFile(filepath.Join(marriageDir, "alice"), encodedBob, 0644); err != nil {
		t.Fatalf("failed to write spouse file: %v", err)
	}

	world := marriageSendTestWorld()
	alice := world.creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "PLECHO")
	world.creatures[alice.ID] = alice

	var sentTo string
	var sentMsg string
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.sendToSession": func(id string, msg string) error {
				sentTo = id
				sentMsg = msg
				return nil
			},
		},
	}
	input := "사랑해요   사랑말"
	status, err := NewMarriageSendHandler(world, tempDir)(ctx, ResolvedCommand{
		Input:  input,
		Parsed: commandparse.Parse(input),
		Args:   []string{"사랑해요"},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if sentTo != "session:bob" {
		t.Fatalf("sent to session %q, want session:bob", sentTo)
	}
	if want := "\nAlice가 당신에게 \"사랑해요  \"라고 이야기합니다."; sentMsg != want {
		t.Fatalf("sent message = %q, want %q", sentMsg, want)
	}
	if got, want := ctx.OutputString(), "당신은 Bob에게 \"사랑해요  \"라고 이야기합니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestMarriageSendHandlerChecksMessageAfterSpouseLookupLikeLegacy(t *testing.T) {
	tempDir := t.TempDir()
	marriageDir := filepath.Join(tempDir, "player", "marriage")
	if err := os.MkdirAll(marriageDir, 0755); err != nil {
		t.Fatalf("failed to create temp marriage dir: %v", err)
	}
	encodedBob, err := legacykr.EncodeEUCKR("Bob")
	if err != nil {
		t.Fatalf("failed to encode Bob: %v", err)
	}
	if err := os.WriteFile(filepath.Join(marriageDir, "alice"), encodedBob, 0644); err != nil {
		t.Fatalf("failed to write spouse file: %v", err)
	}

	world := marriageSendTestWorld()
	handler := NewMarriageSendHandler(world, tempDir)
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.sendToSession": func(string, string) error { return nil },
		},
	}

	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "무슨 말을 전하시려고요?" {
		t.Fatalf("output = %q", got)
	}
}

func marriageSendTestWorld() *fakeSocialTransferWorld {
	return &fakeSocialTransferWorld{
		rooms: map[model.RoomID]model.Room{
			"room:plaza": {
				ID:        "room:plaza",
				PlayerIDs: []model.PlayerID{"player:alice", "player:bob"},
			},
		},
		players: map[model.PlayerID]model.Player{
			"player:alice": {
				ID:          "player:alice",
				DisplayName: "Alice",
				CreatureID:  "creature:alice",
				RoomID:      "room:plaza",
				Metadata:    model.Metadata{Tags: []string{"PMARRI"}},
			},
			"player:bob": {
				ID:          "player:bob",
				DisplayName: "Bob",
				CreatureID:  "creature:bob",
				RoomID:      "room:plaza",
			},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:          "creature:alice",
				DisplayName: "Alice",
				Kind:        model.CreatureKindPlayer,
				PlayerID:    "player:alice",
				RoomID:      "room:plaza",
			},
			"creature:bob": {
				ID:          "creature:bob",
				DisplayName: "Bob",
				Kind:        model.CreatureKindPlayer,
				PlayerID:    "player:bob",
				RoomID:      "room:plaza",
			},
		},
		active: []string{"session:bob"},
		activeActors: map[string]string{
			"session:bob": "player:bob",
		},
	}
}
