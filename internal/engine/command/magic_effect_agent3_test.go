package command

import (
	"strings"
	"testing"

	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestMagicEffectEnchant_ClassRestrictions(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{ID: "room:church", DisplayName: "성당"})

	// Set class to fighter (not Mage or Invincible)
	creature := loaded.Creatures["creature:alice"]
	creature.RoomID = "room:church"
	creature.Stats = map[string]int{
		"class": 1, // Fighter
		"level": 10,
	}
	loaded.Creatures[creature.ID] = creature

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice", SessionID: "session-alice"}

	success, err := magicEffectEnchant(ctx, runtime, creature, model.ObjectInstance{}, ResolvedCommand{Args: []string{"주술", "검"}})
	if err != nil {
		t.Fatalf("magicEffectEnchant error: %v", err)
	}
	if success {
		t.Fatalf("expected failure due to class restrictions, but got success")
	}
	if !strings.Contains(ctx.OutputString(), "도술사들만이 주술을 걸수있습니다.") {
		t.Errorf("unexpected output: %q", ctx.OutputString())
	}
}

func TestMagicEffectEnchant_Success(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{ID: "room:church", DisplayName: "성당"})

	// Setup Mage caster
	creature := loaded.Creatures["creature:alice"]
	creature.RoomID = "room:church"
	creature.Stats = map[string]int{
		"class":     legacyClassMage,
		"level":     12,
		"mpCurrent": 25,
	}
	creature.Metadata.Tags = []string{"SENCHA"}

	// Add weapon to inventory
	weaponID := model.ObjectInstanceID("object:sword")
	loaded.Objects[weaponID] = model.ObjectInstance{
		ID:                  weaponID,
		PrototypeID:         "proto:sword",
		DisplayNameOverride: "목검",
		Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties: map[string]string{
			"type":         "1", // weapon
			"shotsMax":     "10",
			"shotsCurrent": "10",
			"pDice":        "2",
			"value":        "100",
		},
	}
	creature.Inventory.ObjectIDs = []model.ObjectInstanceID{weaponID}
	loaded.Creatures[creature.ID] = creature

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice", SessionID: "session-alice"}

	success, err := magicEffectEnchant(ctx, runtime, creature, model.ObjectInstance{}, ResolvedCommand{Args: []string{"주술", "목검"}})
	if err != nil {
		t.Fatalf("magicEffectEnchant error: %v", err)
	}
	if !success {
		t.Fatalf("expected success, but got failure")
	}

	// Verify target weapon properties
	obj, _ := runtime.Object(weaponID)
	if obj.Properties["adjustment"] != "1" {
		t.Errorf("expected adjustment 1, got %q", obj.Properties["adjustment"])
	}
	if obj.Properties["shotsMax"] != "20" { // 10 + 1*10
		t.Errorf("expected shotsMax 20, got %q", obj.Properties["shotsMax"])
	}
	if obj.Properties["pDice"] != "3" { // 2 + 1
		t.Errorf("expected pDice 3, got %q", obj.Properties["pDice"])
	}
	if obj.Properties["value"] != "600" { // 100 + 500*1
		t.Errorf("expected value 600, got %q", obj.Properties["value"])
	}
	if !objectHasAnyTag(runtime, obj, "enchanted") || !objectHasAnyTag(runtime, obj, "oencha") || !objectHasAnyTag(runtime, obj, "OENCHA") {
		t.Errorf("expected enchanted/oencha/OENCHA tags, got %v", obj.Metadata.Tags)
	}
	if _, ok := obj.Properties["enchant_expire_at"]; ok {
		t.Errorf("unexpected enchant_expire_at property on C enchant: %q", obj.Properties["enchant_expire_at"])
	}
	updatedCreature, _ := runtime.Creature(creature.ID)
	if got := creatureStat(updatedCreature, "mpCurrent"); got != 0 {
		t.Fatalf("mpCurrent = %d, want C enchant cost to reduce 25 MP to 0", got)
	}

	// Try enchanting again
	if err := runtime.SetCreatureStat(creature.ID, "mpCurrent", 25); err != nil {
		t.Fatalf("SetCreatureStat(mpCurrent) error = %v", err)
	}
	ctx.Output = nil // Reset output slice
	updatedCreature, _ = runtime.Creature(creature.ID)
	success2, err2 := magicEffectEnchant(ctx, runtime, updatedCreature, model.ObjectInstance{}, ResolvedCommand{Args: []string{"주술", "목검"}})
	if err2 != nil {
		t.Fatalf("second magicEffectEnchant error: %v", err2)
	}
	if !success2 {
		t.Fatalf("expected success (but no-op) on already enchanted item, got failure")
	}
	if !strings.Contains(ctx.OutputString(), "벌써 주술이 걸려있습니다.") {
		t.Errorf("unexpected output on second enchant: %q", ctx.OutputString())
	}
}

func TestMagicEffectRemoveCurse_Self(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{ID: "room:church", DisplayName: "성당"})

	// Setup player/creature
	player := loaded.Players["player:alice"]
	player.RoomID = "room:church"
	loaded.Players[player.ID] = player

	creature := loaded.Creatures["creature:alice"]
	creature.RoomID = "room:church"
	creature.Metadata.Tags = []string{"SREMOV"}
	creature.Stats = map[string]int{
		"class":     legacyClassDM,
		"mpCurrent": 18,
	}

	// Equip a cursed sword
	weaponID := model.ObjectInstanceID("object:cursed_sword")
	loaded.Objects[weaponID] = model.ObjectInstance{
		ID:                  weaponID,
		PrototypeID:         "proto:sword",
		DisplayNameOverride: "저주받은 검",
		Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "wield"},
		Metadata:            model.Metadata{Tags: []string{"cursed", "ocurse"}},
	}
	creature.Equipment = map[string]model.ObjectInstanceID{
		"wield": weaponID,
	}
	loaded.Creatures[creature.ID] = creature

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice", SessionID: "session-alice"}

	success, err := magicEffectRemoveCurse(ctx, runtime, creature, model.ObjectInstance{}, ResolvedCommand{Args: []string{"저주해소"}})
	if err != nil {
		t.Fatalf("magicEffectRemoveCurse error: %v", err)
	}
	if !success {
		t.Fatalf("expected success")
	}

	// Verify tags cleared
	obj, _ := runtime.Object(weaponID)
	if objectHasAnyTag(runtime, obj, "cursed") || objectHasAnyTag(runtime, obj, "ocurse") {
		t.Errorf("expected cursed/ocurse tags to be removed, got %v", obj.Metadata.Tags)
	}

	if !strings.Contains(ctx.OutputString(), "당신의 몸에 걸렸던 저주가 풀리기 시작합니다.") {
		t.Errorf("unexpected output: %q", ctx.OutputString())
	}
}

func TestMagicEffectRemoveCurse_Target(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:church",
		DisplayName: "성당",
		PlayerIDs:   []model.PlayerID{"player:alice", "player:bob"},
	})

	// Setup caster Alice
	alicePlayer := loaded.Players["player:alice"]
	alicePlayer.RoomID = "room:church"
	loaded.Players[alicePlayer.ID] = alicePlayer

	aliceCreature := loaded.Creatures["creature:alice"]
	aliceCreature.RoomID = "room:church"
	aliceCreature.Metadata.Tags = []string{"SREMOV"}
	aliceCreature.Stats = map[string]int{
		"class":     legacyClassDM,
		"mpCurrent": 18,
	}
	loaded.Creatures[aliceCreature.ID] = aliceCreature

	// Setup target Bob
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:church",
	})

	// Bob equips a cursed ring
	ringID := model.ObjectInstanceID("object:cursed_ring")
	loaded.Objects[ringID] = model.ObjectInstance{
		ID:                  ringID,
		PrototypeID:         "proto:ring",
		DisplayNameOverride: "저주받은 반지",
		Location:            model.ObjectLocation{CreatureID: "creature:bob", Slot: "ring"},
		Metadata:            model.Metadata{Tags: []string{"cursed", "ocurse"}},
	}

	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:church",
		Equipment: map[string]model.ObjectInstanceID{
			"ring": ringID,
		},
	})

	runtime := state.NewWorld(loaded)

	// Set active session mocks to test messaging
	ctx := &Context{ActorID: "player:alice", SessionID: "session-alice"}
	var bobMessage string
	ctx.Values = map[string]any{
		"game.activeSessions": func() []struct {
			ID      string
			ActorID string
		} {
			return []struct {
				ID      string
				ActorID string
			}{
				{ID: "session-alice", ActorID: "player:alice"},
				{ID: "session-bob", ActorID: "player:bob"},
			}
		},
		"game.sendToSession": func(id string, cmd struct{ Write string }) error {
			if id == "session-bob" {
				bobMessage = cmd.Write
			}
			return nil
		},
	}

	success, err := magicEffectRemoveCurse(ctx, runtime, aliceCreature, model.ObjectInstance{}, ResolvedCommand{Args: []string{"저주해소", "Bob"}})
	if err != nil {
		t.Fatalf("magicEffectRemoveCurse error: %v", err)
	}
	if !success {
		t.Fatalf("expected success")
	}

	// Verify Bob's ring is no longer cursed
	obj, _ := runtime.Object(ringID)
	if objectHasAnyTag(runtime, obj, "cursed") || objectHasAnyTag(runtime, obj, "ocurse") {
		t.Errorf("expected cursed tags to be removed from Bob's item, got %v", obj.Metadata.Tags)
	}

	if !strings.Contains(ctx.OutputString(), "그의 몸에서 저주가 물러가는 것이 느껴집니다.") {
		t.Errorf("unexpected caster output: %q", ctx.OutputString())
	}

	if !strings.Contains(bobMessage, "당신의 몸에서 저주가 물러가는 것이 느껴집니다.") {
		t.Errorf("unexpected target Bob output: %q", bobMessage)
	}
}

func TestMagicEffectCurse_Self(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{ID: "room:church", DisplayName: "성당"})

	// Setup player/creature
	player := loaded.Players["player:alice"]
	player.RoomID = "room:church"
	loaded.Players[player.ID] = player

	creature := loaded.Creatures["creature:alice"]
	creature.RoomID = "room:church"
	creature.Metadata.Tags = []string{"SCURSE"}
	creature.Stats = map[string]int{
		"class":     legacyClassDM,
		"mpCurrent": 25,
	}

	// Equip a sword
	weaponID := model.ObjectInstanceID("object:sword")
	loaded.Objects[weaponID] = model.ObjectInstance{
		ID:                  weaponID,
		PrototypeID:         "proto:sword",
		DisplayNameOverride: "장검",
		Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "wield"},
	}
	creature.Equipment = map[string]model.ObjectInstanceID{
		"wield": weaponID,
	}
	loaded.Creatures[creature.ID] = creature

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice", SessionID: "session-alice"}

	success, err := magicEffectCurse(ctx, runtime, creature, model.ObjectInstance{}, ResolvedCommand{Args: []string{"저주"}})
	if err != nil {
		t.Fatalf("magicEffectCurse error: %v", err)
	}
	if !success {
		t.Fatalf("expected success")
	}

	// Verify sword is cursed
	obj, _ := runtime.Object(weaponID)
	if !objectHasAnyTag(runtime, obj, "cursed") || !objectHasAnyTag(runtime, obj, "ocurse") {
		t.Errorf("expected sword to be cursed, got %v", obj.Metadata.Tags)
	}

	if !strings.Contains(ctx.OutputString(), "오홋~~ 손이 펴지질 않아.. 당신은 무기를 벗을수 없습니다.") {
		t.Errorf("unexpected output: %q", ctx.OutputString())
	}
}
