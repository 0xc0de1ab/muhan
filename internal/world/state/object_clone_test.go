package state

import (
	"slices"
	"testing"

	"muhan/internal/world/model"
)

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
