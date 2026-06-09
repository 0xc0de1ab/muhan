package command

import (
	"errors"
	"strings"
	"testing"

	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestRepairHandlerRepairsDamagedWeapon(t *testing.T) {
	world := state.NewWorld(repairTestWorld(t, true, map[string]string{
		"type":         "1",
		"value":        "100",
		"shotsCurrent": "0",
		"shotsMax":     "10",
	}))
	ctx := &Context{ActorID: "Alice"}

	status, err := NewRepairHandler(world, repairTestRoll)(ctx, ResolvedCommand{Args: []string{"목검"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	got := ctx.OutputString()
	for _, want := range []string{
		"당신은 수리점 주인에게 수리비 25냥을 건네주었습니다.\n",
		"수리점 주인이 당신에게 목검을 되돌려 줍니다.\n",
		"그것은 거의 새것처럼 보입니다.\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	creature, _ := world.Creature("creature:alice")
	if creature.Stats["gold"] != 75 {
		t.Fatalf("gold = %d, want 75", creature.Stats["gold"])
	}
	object, _ := world.Object("object:sword")
	if object.Properties["shotsCurrent"] != "7" {
		t.Fatalf("shotsCurrent = %q, want 7", object.Properties["shotsCurrent"])
	}
}

func TestRepairHandlerBreaksObjectWithoutChargingNetGold(t *testing.T) {
	world := state.NewWorld(repairTestWorld(t, true, map[string]string{
		"type":         "1",
		"value":        "100",
		"shotsCurrent": "0",
		"shotsMax":     "10",
	}))
	ctx := &Context{ActorID: "Alice"}

	status, err := NewRepairHandler(world, func(min, max int) int { return min })(ctx, ResolvedCommand{Args: []string{"목검"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	want := "당신은 수리점 주인에게 수리비 25냥을 건네주었습니다.\n" +
		"수리점 주인이 \"이런~~! 수리를 하다 부러뜨렸네. 미안하네\"라고 말합니다.\n" +
		"수리점주인이 당신에게 돈을 돌려주었습니다.\n"
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	creature, _ := world.Creature("creature:alice")
	if creature.Stats["gold"] != 100 {
		t.Fatalf("gold = %d, want unchanged 100", creature.Stats["gold"])
	}
	if _, ok := world.Object("object:sword"); ok {
		t.Fatal("object:sword still exists after breakage")
	}
	creature, _ = world.Creature("creature:alice")
	if strings.Contains(strings.Join(objectIDStrings(creature.Inventory.ObjectIDs), ","), "object:sword") {
		t.Fatalf("inventory still contains sword: %+v", creature.Inventory.ObjectIDs)
	}
}

func TestRepairHandlerRejectsInvalidConditions(t *testing.T) {
	tests := []struct {
		name       string
		repairRoom bool
		props      map[string]string
		args       []string
		want       string
	}{
		{name: "missing target before room check", repairRoom: false, props: repairWeaponProps(), want: "무엇을 수리하시려구요?\n"},
		{name: "not repair room", repairRoom: false, props: repairWeaponProps(), args: []string{"목검"}, want: "여기서는 수리할 수 없습니다.\n"},
		{name: "no repair", repairRoom: true, props: map[string]string{"type": "1", "value": "100", "shotsCurrent": "0", "shotsMax": "10", "onofix": "1"}, args: []string{"목검"}, want: "그것은 수리할수 없는 물건입니다.\n"},
		{name: "not weapon armor", repairRoom: true, props: map[string]string{"type": "13", "value": "100", "shotsCurrent": "0", "shotsMax": "10"}, args: []string{"목검"}, want: "수리점 주인이 \"무기나 방호구만 수리할 수 있네.\"라고 말합니다.\n"},
		{name: "too healthy", repairRoom: true, props: map[string]string{"type": "1", "value": "100", "shotsCurrent": "5", "shotsMax": "10"}, args: []string{"목검"}, want: "수리점 주인이 \"그건 아직 멀쩡한데...\"라고 말합니다.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := state.NewWorld(repairTestWorld(t, tt.repairRoom, tt.props))
			ctx := &Context{ActorID: "Alice"}
			status, err := NewRepairHandler(world, repairTestRoll)(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestRepairHandlerRejectsObjectIDTargetLikeLegacyFindObj(t *testing.T) {
	world := state.NewWorld(repairTestWorld(t, true, repairWeaponProps()))
	ctx := &Context{ActorID: "Alice"}

	status, err := NewRepairHandler(world, repairTestRoll)(ctx, ResolvedCommand{Args: []string{"object:sword"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 그런 물건을 갖고 있지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want missing object", status, ctx.OutputString())
	}
	creature, _ := world.Creature("creature:alice")
	if creature.Stats["gold"] != 100 {
		t.Fatalf("gold = %d, want unchanged 100", creature.Stats["gold"])
	}
	object, _ := world.Object("object:sword")
	if object.Properties["shotsCurrent"] != "0" {
		t.Fatalf("shotsCurrent = %q, want unchanged 0", object.Properties["shotsCurrent"])
	}
}

func TestRepairHandlerReadsLegacyLowercaseDurabilityFields(t *testing.T) {
	world := state.NewWorld(repairTestWorld(t, true, map[string]string{
		"type":     "1",
		"value":    "100",
		"shotscur": "5",
		"shotsmax": "10",
	}))
	ctx := &Context{ActorID: "Alice"}

	status, err := NewRepairHandler(world, repairTestRoll)(ctx, ResolvedCommand{Args: []string{"목검"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "수리점 주인이 \"그건 아직 멀쩡한데...\"라고 말합니다.\n" {
		t.Fatalf("status/output = %d/%q, want legacy lowercase shots healthy refusal", status, ctx.OutputString())
	}
	creature, _ := world.Creature("creature:alice")
	if creature.Stats["gold"] != 100 {
		t.Fatalf("gold = %d, want unchanged 100", creature.Stats["gold"])
	}
	object, _ := world.Object("object:sword")
	if object.Properties["shotscur"] != "5" || object.Properties["shotsCurrent"] != "" {
		t.Fatalf("object properties = %+v, want lowercase fields left unrepaired", object.Properties)
	}
}

func TestRepairHandlerUsesLegacyPrefixOrderInsteadOfExactFirst(t *testing.T) {
	loaded := repairTestWorld(t, true, repairWeaponProps())
	proto := loaded.ObjectPrototypes["proto:sword"]
	proto.DisplayName = "목검 조각"
	proto.Properties = map[string]string{"name": "목검 조각"}
	loaded.ObjectPrototypes[proto.ID] = proto
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "proto:sword-exact",
		DisplayName: "목검",
		Properties:  map[string]string{"name": "목검"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:sword-exact",
		PrototypeID: "proto:sword-exact",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  repairWeaponProps(),
	})
	alice := loaded.Creatures["creature:alice"]
	alice.Inventory.ObjectIDs = []model.ObjectInstanceID{"object:sword", "object:sword-exact"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	ctx := &Context{ActorID: "Alice"}

	status, err := NewRepairHandler(world, repairTestRoll)(ctx, ResolvedCommand{Args: []string{"목검"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "수리점 주인이 당신에게 목검 조각을 되돌려 줍니다.") {
		t.Fatalf("status/output = %d/%q, want first prefix object repaired", status, ctx.OutputString())
	}
	first, _ := world.Object("object:sword")
	if first.Properties["shotsCurrent"] != "7" {
		t.Fatalf("first shotsCurrent = %q, want repaired 7", first.Properties["shotsCurrent"])
	}
	exact, _ := world.Object("object:sword-exact")
	if exact.Properties["shotsCurrent"] != "0" {
		t.Fatalf("exact shotsCurrent = %q, want unchanged 0", exact.Properties["shotsCurrent"])
	}
}

func TestRepairHandlerUsesOnlyFirstArgumentLikeLegacy(t *testing.T) {
	world := state.NewWorld(repairTestWorld(t, true, repairWeaponProps()))
	ctx := &Context{ActorID: "Alice"}

	status, err := NewRepairHandler(world, repairTestRoll)(ctx, ResolvedCommand{Args: []string{"목검", "무시"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "수리점 주인이 당신에게 목검을 되돌려 줍니다.") {
		t.Fatalf("status/output = %d/%q, want first-argument repair success", status, ctx.OutputString())
	}
	object, _ := world.Object("object:sword")
	if object.Properties["shotsCurrent"] != "7" {
		t.Fatalf("shotsCurrent = %q, want repaired 7", object.Properties["shotsCurrent"])
	}
}

func TestRepairHandlerFindObjVisibilityUsesPDINVILikeLegacy(t *testing.T) {
	props := repairWeaponProps()
	props["OINVIS"] = "1"
	world := state.NewWorld(repairTestWorld(t, true, props))
	ctx := &Context{ActorID: "Alice"}

	status, err := NewRepairHandler(world, repairTestRoll)(ctx, ResolvedCommand{Args: []string{"목검"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 그런 물건을 갖고 있지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want invisible object hidden", status, ctx.OutputString())
	}

	loaded := repairTestWorld(t, true, props)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PDINVI"}
	loaded.Creatures[alice.ID] = alice
	world = state.NewWorld(loaded)
	ctx = &Context{ActorID: "Alice"}

	status, err = NewRepairHandler(world, repairTestRoll)(ctx, ResolvedCommand{Args: []string{"목검"}})
	if err != nil {
		t.Fatalf("handler with PDINVI error: %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "수리점 주인이 당신에게 목검을 되돌려 줍니다.") {
		t.Fatalf("PDINVI status/output = %d/%q, want repair success", status, ctx.OutputString())
	}
	object, _ := world.Object("object:sword")
	if object.Properties["shotsCurrent"] != "7" {
		t.Fatalf("PDINVI shotsCurrent = %q, want repaired 7", object.Properties["shotsCurrent"])
	}
}

func TestRepairHandlerClearsHiddenAfterObjectFound(t *testing.T) {
	world := state.NewWorld(repairTestWorld(t, true, map[string]string{
		"type":         "1",
		"value":        "100",
		"shotsCurrent": "0",
		"shotsMax":     "10",
		"onofix":       "1",
	}))
	if _, err := world.UpdateCreatureTags("creature:alice", []string{"hidden", "PHIDDN"}, nil); err != nil {
		t.Fatalf("UpdateCreatureTags() error = %v", err)
	}
	if _, err := world.UpdatePlayerTags("Alice", []string{"hidden", "PHIDDN"}, nil); err != nil {
		t.Fatalf("UpdatePlayerTags() error = %v", err)
	}
	if err := world.SetCreatureStat("creature:alice", "PHIDDN", 1); err != nil {
		t.Fatalf("SetCreatureStat() error = %v", err)
	}

	ctx := &Context{ActorID: "Alice"}
	status, err := NewRepairHandler(world, repairTestRoll)(ctx, ResolvedCommand{Args: []string{"목검"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그것은 수리할수 없는 물건입니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	creature, _ := world.Creature("creature:alice")
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("creature tags = %+v, want hidden cleared", creature.Metadata.Tags)
	}
	if creature.Stats["PHIDDN"] != 0 {
		t.Fatalf("PHIDDN = %d, want 0", creature.Stats["PHIDDN"])
	}
	player, _ := world.Player("Alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("player tags = %+v, want hidden cleared", player.Metadata.Tags)
	}
}

func TestRepairHandlerStripsEnchantmentWithLegacyMessage(t *testing.T) {
	world := state.NewWorld(repairTestWorld(t, true, map[string]string{
		"type":         "1",
		"value":        "100",
		"shotsCurrent": "0",
		"shotsMax":     "20",
		"pDice":        "3",
		"adjustment":   "1",
	}))
	ctx := &Context{ActorID: "Alice"}

	status, err := NewRepairHandler(world, repairTestRoll)(ctx, ResolvedCommand{Args: []string{"목검"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	for _, want := range []string{
		"당신은 수리점 주인에게 수리비 25냥을 건네주었습니다.\n",
		"수리점 주인이 \"수리가 다되었네.\"라고 말합니다.\n",
		"수리점 주인이 당신에게 목검을 되돌려 줍니다.\n",
	} {
		if !strings.Contains(ctx.OutputString(), want) {
			t.Fatalf("output missing %q:\n%s", want, ctx.OutputString())
		}
	}
	object, _ := world.Object("object:sword")
	if object.Properties["adjustment"] != "0" || object.Properties["pDice"] != "2" ||
		object.Properties["shotsMax"] != "10" || object.Properties["shotsCurrent"] != "7" {
		t.Fatalf("object properties = %+v", object.Properties)
	}
}

func TestRepairHandlerStripsEnchantmentFromLegacyLowercaseFields(t *testing.T) {
	world := state.NewWorld(repairTestWorld(t, true, map[string]string{
		"type":       "1",
		"value":      "100",
		"shotscur":   "0",
		"shotsmax":   "20",
		"pdice":      "3",
		"adjustment": "1",
	}))
	ctx := &Context{ActorID: "Alice"}

	status, err := NewRepairHandler(world, repairTestRoll)(ctx, ResolvedCommand{Args: []string{"목검"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "수리점 주인이 \"수리가 다되었네.\"라고 말합니다.") {
		t.Fatalf("status/output = %d/%q, want enchant strip message", status, ctx.OutputString())
	}
	object, _ := world.Object("object:sword")
	if object.Properties["adjustment"] != "0" || object.Properties["pDice"] != "2" ||
		object.Properties["shotsMax"] != "10" || object.Properties["shotsCurrent"] != "7" {
		t.Fatalf("object properties = %+v, want lowercase C fields used for strip/repair", object.Properties)
	}
}

func TestRepairHandlerUsesLegacyLowercaseWearflagForBodyArmorStrip(t *testing.T) {
	world := state.NewWorld(repairTestWorld(t, true, map[string]string{
		"type":       "5",
		"value":      "100",
		"shotscur":   "0",
		"shotsmax":   "10",
		"armor":      "10",
		"wearflag":   "1",
		"adjustment": "2",
	}))
	ctx := &Context{ActorID: "Alice"}

	status, err := NewRepairHandler(world, repairTestRoll)(ctx, ResolvedCommand{Args: []string{"목검"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "수리점 주인이 \"수리가 다되었네.\"라고 말합니다.") {
		t.Fatalf("status/output = %d/%q, want armor enchant strip message", status, ctx.OutputString())
	}
	object, _ := world.Object("object:sword")
	if object.Properties["armor"] != "6" || object.Properties["adjustment"] != "0" || object.Properties["shotsCurrent"] != "7" {
		t.Fatalf("object properties = %+v, want BODY armor adjustment doubled like C", object.Properties)
	}
}

func TestRepairHandlerBroadcastsPaymentAndBreakage(t *testing.T) {
	world := state.NewWorld(repairTestWorld(t, true, repairWeaponProps()))
	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("Alice", "session:alice", &broadcasts)

	status, err := NewRepairHandler(world, func(min, max int) int { return min })(ctx, ResolvedCommand{Args: []string{"목검"}})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if len(broadcasts) != 2 {
		t.Fatalf("broadcast count = %d, want 2: %+v", len(broadcasts), broadcasts)
	}
	if broadcasts[0].RoomID != "room:repair" || broadcasts[0].Exclude != "session:alice" ||
		broadcasts[0].Text != "\nAlice이 수리를 위해 수리점 주인에게 목검를 건네주었습니다." {
		t.Fatalf("payment broadcast = %+v", broadcasts[0])
	}
	if broadcasts[1].RoomID != "room:repair" || broadcasts[1].Exclude != "session:alice" ||
		broadcasts[1].Text != "이런 주인이 실수를 했습니다." {
		t.Fatalf("breakage broadcast = %+v", broadcasts[1])
	}
}

func TestRepairHandlerIgnoresBroadcastFailuresLikeLegacy(t *testing.T) {
	world := state.NewWorld(repairTestWorld(t, true, repairWeaponProps()))
	ctx := &Context{
		ActorID:   "Alice",
		SessionID: "session:alice",
		Values: map[string]any{
			ContextRoomBroadcastKey: RoomBroadcastFunc(func(model.RoomID, string, string) error {
				return errors.New("session closed")
			}),
		},
	}

	status, err := NewRepairHandler(world, repairTestRoll)(ctx, ResolvedCommand{Args: []string{"목검"}})
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "수리점 주인이 당신에게 목검을 되돌려 줍니다.") {
		t.Fatalf("status/output = %d/%q, want repair success", status, ctx.OutputString())
	}
}

func repairTestRoll(min, max int) int {
	if min == 5 && max == 9 {
		return 7
	}
	return 50
}

func repairWeaponProps() map[string]string {
	return map[string]string{"type": "1", "value": "100", "shotsCurrent": "0", "shotsMax": "10"}
}

func repairTestWorld(t *testing.T, repairRoom bool, objectProps map[string]string) *worldload.World {
	t.Helper()
	loaded := worldload.NewWorld()
	room := model.Room{ID: "room:repair", DisplayName: "수리점"}
	if repairRoom {
		room.Metadata = model.Metadata{Tags: []string{"repair"}}
	}
	mustAddLookRoom(t, loaded, room)
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "Alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:repair",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "Alice",
		RoomID:      "room:repair",
		Stats:       map[string]int{"gold": 100},
		Inventory:   model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:sword"}},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "proto:sword",
		Kind:        model.ObjectKindMisc,
		DisplayName: "목검",
	})
	props := map[string]string{}
	for key, value := range objectProps {
		props[key] = value
	}
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:sword",
		PrototypeID: "proto:sword",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  props,
	})
	return loaded
}
