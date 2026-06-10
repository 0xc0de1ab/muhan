package command

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"muhan/internal/commandspec"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestInventoryHandlerRendersLegacyInventory(t *testing.T) {
	world := state.NewWorld(inventoryWorld(t))
	defer world.Close()
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "소지품", Number: 40, Handler: "inventory"},
		}),
		Handlers: map[string]Handler{
			"inventory": NewInventoryHandler(world),
		},
	}

	beforePlayer, ok := world.Player("player:alice")
	if !ok {
		t.Fatal("missing player")
	}
	beforeCreature, ok := world.Creature(beforePlayer.CreatureID)
	if !ok {
		t.Fatal("missing creature")
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "소지")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	want := "소지품:\n  빛나는 검, 치유 물약, 번개검, object:fallback."
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}

	afterPlayer, ok := world.Player("player:alice")
	if !ok {
		t.Fatal("missing player after inventory")
	}
	afterCreature, ok := world.Creature(beforePlayer.CreatureID)
	if !ok {
		t.Fatal("missing creature after inventory")
	}
	if !reflect.DeepEqual(afterPlayer, beforePlayer) || !reflect.DeepEqual(afterCreature, beforeCreature) {
		t.Fatalf("inventory mutated actor state:\nplayer before=%+v after=%+v\ncreature before=%+v after=%+v",
			beforePlayer, afterPlayer, beforeCreature, afterCreature)
	}
}

func TestInventoryHandlerRendersEmptyInventory(t *testing.T) {
	world := state.NewWorld(emptyInventoryWorld(t))
	defer world.Close()
	handler := NewInventoryHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	want := "소지품:\n  없음."
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestObjectIsContainerReadsPropertyBackedLegacyFlags(t *testing.T) {
	loaded := worldload.NewWorld()
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          "prototype:property-container",
		DisplayName: "가방",
		Properties:  map[string]string{"flags": "OCONTN"},
	}); err != nil {
		t.Fatal(err)
	}
	world := state.NewWorld(loaded)
	defer world.Close()

	tests := []struct {
		name string
		obj  model.ObjectInstance
		want bool
	}{
		{
			name: "direct property flag",
			obj:  model.ObjectInstance{Properties: map[string]string{"OCONTN": "1"}},
			want: true,
		},
		{
			name: "flags token",
			obj:  model.ObjectInstance{Properties: map[string]string{"flags": "OCONTN|OINVIS"}},
			want: true,
		},
		{
			name: "prototype flags token",
			obj:  model.ObjectInstance{PrototypeID: "prototype:property-container"},
			want: true,
		},
		{
			name: "disabled property flag",
			obj:  model.ObjectInstance{Properties: map[string]string{"OCONTN": "false"}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := objectIsContainer(world, tt.obj); got != tt.want {
				t.Fatalf("objectIsContainer(%+v) = %t, want %t", tt.obj, got, tt.want)
			}
		})
	}
}

func TestInventoryHandlerUsesLegacyBlindMessage(t *testing.T) {
	tests := []struct {
		name       string
		playerTags []string
		creature   func(model.Creature) model.Creature
	}{
		{
			name:       "player tag",
			playerTags: []string{"blind"},
		},
		{
			name: "creature stat",
			creature: func(creature model.Creature) model.Creature {
				if creature.Stats == nil {
					creature.Stats = map[string]int{}
				}
				creature.Stats["PBLIND"] = 1
				return creature
			},
		},
		{
			name: "creature tag",
			creature: func(creature model.Creature) model.Creature {
				creature.Metadata.Tags = append(creature.Metadata.Tags, "PBLIND")
				return creature
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := inventoryWorld(t)
			player := loaded.Players["player:alice"]
			player.Metadata.Tags = append(player.Metadata.Tags, tt.playerTags...)
			loaded.Players[player.ID] = player
			if tt.creature != nil {
				creature := loaded.Creatures[player.CreatureID]
				loaded.Creatures[creature.ID] = tt.creature(creature)
			}
			world := state.NewWorld(loaded)
	defer world.Close()
			handler := NewInventoryHandler(world)

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != legacyInventoryBlindMessage {
				t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
			}
		})
	}
}

func TestInventoryHandlerUsesLegacyBlindANSIColor(t *testing.T) {
	loaded := inventoryWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"PBLIND"}
	loaded.Creatures[creature.ID] = creature
	world := state.NewWorld(loaded)
	defer world.Close()
	handler := NewInventoryHandler(world)

	ctx := &Context{
		ActorID: "player:alice",
		Values:  map[string]any{ContextANSIKey: true},
	}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	want := "\x1b[0;34m" + legacyInventoryBlindMessage + "\x1b[0;0m"
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if strings.Contains(ctx.OutputString(), "소지품:") {
		t.Fatalf("blind inventory output exposed inventory:\n%s", ctx.OutputString())
	}
}

func TestInventoryHandlerRendersMagicNamesOnlyForDetector(t *testing.T) {
	tests := []struct {
		name          string
		playerTags    []string
		creatureTags  []string
		creatureStats map[string]int
		want          string
	}{
		{
			name: "not detector",
			want: "소지품:\n  (x2) 반지, 반지, 주문서.",
		},
		{
			name:         "creature detect magic",
			creatureTags: []string{"detectMagic"},
			want:         "소지품:\n  (x2) 반지(+1), 반지(+2), 주문서(주문).",
		},
		{
			name:          "creature stat PDMAGI",
			creatureStats: map[string]int{"PDMAGI": 1},
			want:          "소지품:\n  (x2) 반지(+1), 반지(+2), 주문서(주문).",
		},
		{
			name:          "zero creature stat PDMAGI",
			creatureStats: map[string]int{"PDMAGI": 0},
			want:          "소지품:\n  (x2) 반지, 반지, 주문서.",
		},
		{
			name:       "player PDMAGI",
			playerTags: []string{"PDMAGI"},
			want:       "소지품:\n  (x2) 반지(+1), 반지(+2), 주문서(주문).",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := magicInventoryWorld(t, tt.playerTags, tt.creatureTags)
			if tt.creatureStats != nil {
				creature := loaded.Creatures["creature:alice"]
				creature.Stats = tt.creatureStats
				loaded.Creatures[creature.ID] = creature
			}
			world := state.NewWorld(loaded)
	defer world.Close()
			handler := NewInventoryHandler(world)

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}
			if got := ctx.OutputString(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInventoryHandlerHidesInvisibleObjectsUnlessDetector(t *testing.T) {
	tests := []struct {
		name          string
		playerTags    []string
		creatureTags  []string
		creatureStats map[string]int
		want          string
	}{
		{
			name: "not detector",
			want: "소지품:\n  빛나는 검, 치유 물약, 번개검, object:fallback.",
		},
		{
			name:       "player detects invisible",
			playerTags: []string{"PDINVI"},
			want:       "소지품:\n  빛나는 검, 치유 물약, 번개검, object:fallback, 은신검.",
		},
		{
			name:       "learned detect invisible spell is not active detection",
			playerTags: []string{"SDINVI"},
			want:       "소지품:\n  빛나는 검, 치유 물약, 번개검, object:fallback.",
		},
		{
			name:          "creature stat PDINVI",
			creatureStats: map[string]int{"PDINVI": 1},
			want:          "소지품:\n  빛나는 검, 치유 물약, 번개검, object:fallback, 은신검.",
		},
		{
			name:          "zero creature stat PDINVI",
			creatureStats: map[string]int{"PDINVI": 0},
			want:          "소지품:\n  빛나는 검, 치유 물약, 번개검, object:fallback.",
		},
		{
			name:         "creature detects invisible",
			creatureTags: []string{"detectInvisible"},
			want:         "소지품:\n  빛나는 검, 치유 물약, 번개검, object:fallback, 은신검.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := inventoryWorld(t)
			player := loaded.Players["player:alice"]
			player.Metadata.Tags = append([]string(nil), tt.playerTags...)
			loaded.Players[player.ID] = player

			creature := loaded.Creatures[player.CreatureID]
			creature.Metadata.Tags = append([]string(nil), tt.creatureTags...)
			creature.Stats = tt.creatureStats
			creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:invisible")
			loaded.Creatures[creature.ID] = creature

			mustAddLookPrototype(t, loaded, model.ObjectPrototype{
				ID:          "prototype:invisible",
				DisplayName: "은신검",
			})
			mustAddLookObject(t, loaded, model.ObjectInstance{
				ID:          "object:invisible",
				PrototypeID: "prototype:invisible",
				Quantity:    1,
				Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
				Metadata:    model.Metadata{Tags: []string{"OINVIS"}},
			})

			world := state.NewWorld(loaded)
	defer world.Close()
			handler := NewInventoryHandler(world)
			ctx := &Context{ActorID: "player:alice"}

			status, err := handler(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}
			if got := ctx.OutputString(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInventoryHandlerMatchesLegacyAllInvisibleOutput(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = []model.ObjectInstanceID{"object:invisible"}
	loaded.Creatures[creature.ID] = creature
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:invisible",
		DisplayName: "은신검",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:                  "object:invisible",
		PrototypeID:         "prototype:invisible",
		DisplayNameOverride: "은신검",
		Quantity:            1,
		Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Metadata:            model.Metadata{Tags: []string{"OINVIS"}},
	})

	world := state.NewWorld(loaded)
	defer world.Close()
	handler := NewInventoryHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "" {
		t.Fatalf("output = %q, want no output like C when all carried objects are invisible", got)
	}
}

func TestInventoryHandlerIgnoresTargetsLikeLegacy(t *testing.T) {
	world := state.NewWorld(inventoryWorld(t))
	defer world.Close()
	handler := NewInventoryHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"검"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	want := "소지품:\n  빛나는 검, 치유 물약, 번개검, object:fallback."
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestInventoryHandlerRequiresWorldActorAndCreature(t *testing.T) {
	handler := NewInventoryHandler(nil)
	_, err := handler(&Context{ActorID: "player:alice"}, ResolvedCommand{})
	if !errors.Is(err, ErrInventoryWorldRequired) {
		t.Fatalf("handler() error = %v, want ErrInventoryWorldRequired", err)
	}

	world := state.NewWorld(emptyInventoryWorld(t))
	defer world.Close()
	handler = NewInventoryHandler(world)
	_, err = handler(&Context{}, ResolvedCommand{})
	if !errors.Is(err, ErrInventoryActorRequired) {
		t.Fatalf("handler() error = %v, want ErrInventoryActorRequired", err)
	}

	loaded := worldload.NewWorld()
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
	})
	handler = NewInventoryHandler(state.NewWorld(loaded))
	_, err = handler(&Context{ActorID: "player:alice"}, ResolvedCommand{})
	if !errors.Is(err, ErrInventoryCreatureRequired) {
		t.Fatalf("handler() error = %v, want ErrInventoryCreatureRequired", err)
	}
}

func inventoryWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := emptyInventoryWorld(t)
	creature, ok := loaded.Creatures["creature:alice"]
	if !ok {
		t.Fatal("missing creature")
	}
	creature.Inventory = model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
		"object:override",
		"object:prototype",
		"object:key-fallback",
		"object:fallback",
	}}
	creature.Equipment = map[string]model.ObjectInstanceID{
		"right":  "object:override",
		"left":   "object:prototype",
		"finger": "object:missing-equipped",
	}
	loaded.Creatures[creature.ID] = creature

	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:sword",
		DisplayName: "검",
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:potion",
		DisplayName: "치유 물약",
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:internal",
		DisplayName: "object:player:alice:inventory:0:00001188",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:                  "object:override",
		PrototypeID:         "prototype:sword",
		DisplayNameOverride: "빛나는 검",
		Quantity:            1,
		Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:prototype",
		PrototypeID: "prototype:potion",
		Quantity:    1,
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:key-fallback",
		PrototypeID: "prototype:internal",
		Quantity:    1,
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"key[0]": "번개검"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:fallback",
		PrototypeID: "prototype:missing",
		Quantity:    1,
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	})
	return loaded
}

func magicInventoryWorld(t *testing.T, playerTags, creatureTags []string) *worldload.World {
	t.Helper()

	loaded := emptyInventoryWorld(t)
	player, ok := loaded.Players["player:alice"]
	if !ok {
		t.Fatal("missing player")
	}
	player.Metadata.Tags = append([]string(nil), playerTags...)
	loaded.Players[player.ID] = player

	creature, ok := loaded.Creatures["creature:alice"]
	if !ok {
		t.Fatal("missing creature")
	}
	creature.Metadata.Tags = append([]string(nil), creatureTags...)
	creature.Inventory = model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
		"object:ring-plus-one-a",
		"object:ring-plus-one-b",
		"object:ring-plus-two",
		"object:scroll",
	}}
	creature.Equipment = map[string]model.ObjectInstanceID{
		"right": "object:ring-plus-two",
	}
	loaded.Creatures[creature.ID] = creature

	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:ring",
		DisplayName: "반지",
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:scroll",
		DisplayName: "주문서",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:ring-plus-one-a",
		PrototypeID: "prototype:ring",
		Quantity:    1,
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"adjustment": "1"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:ring-plus-one-b",
		PrototypeID: "prototype:ring",
		Quantity:    1,
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"adjustment": "1"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:ring-plus-two",
		PrototypeID: "prototype:ring",
		Quantity:    1,
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"adjustment": "2"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:scroll",
		PrototypeID: "prototype:scroll",
		Quantity:    1,
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"magicPower": "7"},
	})
	return loaded
}

func emptyInventoryWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
	})
	return loaded
}
