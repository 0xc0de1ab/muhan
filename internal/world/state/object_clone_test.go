package state

import (
	"slices"
	"testing"

	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

// TestPurchaseSkipsRandomEnchantButCloneApplies guards that the shop buy /
// monster purchase path does NOT re-roll the ORENCH random enchant (C buy copies
// the depot object as-is; C purchase load_obj's a raw template — neither rolls
// rand_enchant, command7.c:169-217 / command10.c:492+), while the ordinary clone
// path still enchants. enchantRoll is forced to 99 (adjustment 4) so a rolled
// enchant is unmistakable.
func TestPurchaseSkipsRandomEnchantButCloneApplies(t *testing.T) {
	prev := enchantRoll
	enchantRoll = func() int { return 99 }
	defer func() { enchantRoll = prev }()

	loaded := worldload.NewWorld()
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          "prototype:orench",
		DisplayName: "마법검",
		Properties:  map[string]string{"value": "10", "pDice": "2"},
		Metadata:    model.Metadata{Tags: []string{"ORENCH"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddRoom(model.Room{
		ID:          "room:shop",
		DisplayName: "상점",
		Objects:     model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:orench"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddObjectInstance(model.ObjectInstance{
		ID:          "object:orench",
		PrototypeID: "prototype:orench",
		Location:    model.ObjectLocation{RoomID: "room:shop"},
		Metadata:    model.Metadata{Tags: []string{"ORENCH"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddCreature(model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindMonster,
		DisplayName: "구매자",
		RoomID:      "room:shop",
		Stats:       map[string]int{"gold": 1000},
	}); err != nil {
		t.Fatal(err)
	}

	runtime := NewWorld(loaded)
	defer runtime.Close()

	// Ordinary clone path enchants (roll 99 -> enchanted/oencha).
	cloneID, err := runtime.CloneObjectToCreatureInventory("object:orench", "creature:alice")
	if err != nil {
		t.Fatalf("CloneObjectToCreatureInventory() error = %v", err)
	}
	cloned, _ := runtime.Object(cloneID)
	if !slices.Contains(cloned.Metadata.Tags, "enchanted") && !slices.Contains(cloned.Metadata.Tags, "oencha") {
		t.Fatalf("cloned object tags = %+v, want random enchant applied", cloned.Metadata.Tags)
	}

	// Purchase path must NOT enchant, even at roll 99.
	buyID, _, affordable, err := runtime.PurchaseObjectToCreatureInventory("object:orench", "creature:alice", 10)
	if err != nil || !affordable {
		t.Fatalf("PurchaseObjectToCreatureInventory() affordable=%v err=%v", affordable, err)
	}
	bought, _ := runtime.Object(buyID)
	if slices.Contains(bought.Metadata.Tags, "enchanted") || slices.Contains(bought.Metadata.Tags, "oencha") {
		t.Fatalf("purchased object tags = %+v, want NO random enchant on the buy path", bought.Metadata.Tags)
	}
	if bought.Properties["adjustment"] != "" {
		t.Fatalf("purchased object adjustment = %q, want none (no enchant roll)", bought.Properties["adjustment"])
	}
}

func TestLegacyRandomEnchantAdjustmentThresholds(t *testing.T) {
	tests := []struct {
		roll int
		want int
	}{
		{roll: 1, want: 0},
		{roll: 60, want: 0},
		{roll: 61, want: 1},
		{roll: 80, want: 1},
		{roll: 81, want: 2},
		{roll: 90, want: 2},
		{roll: 91, want: 3},
		{roll: 98, want: 3},
		{roll: 99, want: 4},
		{roll: 100, want: 4},
	}

	for _, tt := range tests {
		if got := legacyRandomEnchantAdjustment(tt.roll); got != tt.want {
			t.Fatalf("legacyRandomEnchantAdjustment(%d) = %d, want %d", tt.roll, got, tt.want)
		}
	}
}

func TestApplyLegacyRandomEnchantRollMatchesCFields(t *testing.T) {
	world := &World{}
	object := model.ObjectInstance{
		ID:          "object:sword",
		PrototypeID: "prototype:sword",
		Location:    model.ObjectLocation{CreatureID: "creature:alice"},
		Properties:  map[string]string{"pDice": "2"},
		Metadata:    model.Metadata{Tags: []string{"randomEnchantment"}},
	}

	world.applyLegacyRandomEnchantRollLocked(&object, 99)

	if object.Properties["adjustment"] != "4" {
		t.Fatalf("adjustment = %q, want 4", object.Properties["adjustment"])
	}
	if object.Properties["pDice"] != "6" {
		t.Fatalf("pDice = %q, want 6", object.Properties["pDice"])
	}
	if !slices.Contains(object.Metadata.Tags, "enchanted") || !slices.Contains(object.Metadata.Tags, "oencha") {
		t.Fatalf("tags = %+v, want enchanted/oencha", object.Metadata.Tags)
	}
}

func TestApplyLegacyRandomEnchantRollStillMaxesPDiceWithoutEnchant(t *testing.T) {
	world := &World{}
	object := model.ObjectInstance{
		ID:          "object:staff",
		PrototypeID: "prototype:staff",
		Location:    model.ObjectLocation{CreatureID: "creature:alice"},
		Properties:  map[string]string{"adjustment": "3", "pDice": "1"},
		Metadata:    model.Metadata{Tags: []string{"ORENCH"}},
	}

	world.applyLegacyRandomEnchantRollLocked(&object, 60)

	if object.Properties["adjustment"] != "3" {
		t.Fatalf("adjustment = %q, want existing 3", object.Properties["adjustment"])
	}
	if object.Properties["pDice"] != "3" {
		t.Fatalf("pDice = %q, want max(existing pDice, adjustment)=3", object.Properties["pDice"])
	}
	if slices.Contains(object.Metadata.Tags, "enchanted") || slices.Contains(object.Metadata.Tags, "oencha") {
		t.Fatalf("tags = %+v, did not want enchant tags for roll 60", object.Metadata.Tags)
	}
}
