package game

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	enginecmd "muhan/internal/engine/command"
	"muhan/internal/session"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

// Package 6/6 cross-cut verification: talk actions exercise legacy special hooks from C command8.c/files3.c (ACTION/ATTACK/CAST/GIVE).
// GIVE now docs quest duplicate prevention + exp (Q tags + quest_exp table); ATTACK docs enmity registration.
// See talk.go comments + special.go for completeness. Tests here cover player-visible behavior.

func TestTalkHandlerGiveNumericSuffixMissingPrototypeShowsNothing(t *testing.T) {
	loaded := talkTestWorld(t)
	wise := loaded.Creatures["creature:wise"]
	wise.Stats = map[string]int{"gold": 70}
	loaded.Creatures[wise.ID] = wise

	world := state.NewWorld(loaded)
	root := talkActionTestRoot(t, "계석치무", 25, "보상 GIVE 25냥\n받게.\n")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 보상 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"보상\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"받게.\"라고 이야기합니다.\n"})
	assertNoCommand(t, bob)
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"받게.\"라고 이야기합니다.\n계석치무는 당신에게 줄 물건을 아무것도 가지고 있지 않습니다.\n"})
	assertGiveCreatureGold(t, world, "creature:wise", 70)
	assertGiveCreatureGold(t, world, "creature:alice", 0)
}

func TestTalkHandlerIgnoresNonnumericGiveLikeLegacy(t *testing.T) {
	loaded := talkTestWorld(t)
	wise := loaded.Creatures["creature:wise"]
	wise.Inventory = model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:burnt-scroll"}}
	loaded.Creatures[wise.ID] = wise
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          "prototype:scroll",
		DisplayName: "두루마기",
		Keywords:    []string{"두루마기"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddObjectInstance(model.ObjectInstance{
		ID:                  "object:burnt-scroll",
		PrototypeID:         "prototype:scroll",
		DisplayNameOverride: "타버린 두루마기",
		Location:            model.ObjectLocation{CreatureID: "creature:wise", Slot: "inventory"},
	}); err != nil {
		t.Fatal(err)
	}

	world := state.NewWorld(loaded)
	root := talkActionTestRoot(t, "계석치무", 25, "보상 GIVE 두루마기\n받게.\n")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 보상 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"보상\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"받게.\"라고 이야기합니다.\n"})
	assertNoCommand(t, bob)
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"받게.\"라고 이야기합니다.\n"})
	assertGiveObjectLocation(t, world, "object:burnt-scroll", model.ObjectLocation{CreatureID: "creature:wise", Slot: "inventory"})
	assertGiveCreatureInventory(t, world, "creature:wise", "object:burnt-scroll", true)
	assertGiveCreatureInventory(t, world, "creature:alice", "object:burnt-scroll", false)
}

func TestTalkHandlerGivesLegacyNumericQuestPrototypeOnce(t *testing.T) {
	loaded := talkTestWorld(t)
	protoID := legacyTalkGivePrototypeID(123)
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          protoID,
		DisplayName: "봉인된 패",
		Keywords:    []string{"패"},
		Properties:  map[string]string{"questNumber": "1"},
		Metadata:    model.Metadata{Tags: []string{"event"}},
	}); err != nil {
		t.Fatal(err)
	}

	world := state.NewWorld(loaded)
	root := talkActionTestRoot(t, "계석치무", 25, "보상 GIVE 123\n받게.\n")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 6)
	bob := make(chan session.Command, 6)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 보상 대화"}); err != nil {
		t.Fatalf("HandleEvent() first reward error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"보상\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"받게.\"라고 이야기합니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 봉인된 패를 줍니다.\n"})
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"받게.\"라고 이야기합니다.\n당신은 임무를 완수하여 120의 경험치를 받았습니다.\n\n계석치무가 당신에게 봉인된 패를 줍니다.\n"})

	objectID := assertTalkCreatureHasPrototype(t, world, "creature:alice", protoID, 1)
	object, ok := world.Object(objectID)
	if !ok {
		t.Fatalf("Object(%q) missing", objectID)
	}
	if object.Properties["key[2]"] != "Alice" {
		t.Fatalf("event owner key[2] = %q, want Alice", object.Properties["key[2]"])
	}
	assertTalkCreatureStat(t, world, "creature:alice", "experience", 120)

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 보상 대화"}); err != nil {
		t.Fatalf("HandleEvent() duplicate reward error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"보상\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"받게.\"라고 이야기합니다.\n"})
	assertNoCommand(t, bob)
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"받게.\"라고 이야기합니다.\n" + questAlreadyCompletedMessage()})
	assertTalkCreatureHasPrototype(t, world, "creature:alice", protoID, 1)
}

func TestTalkHandlerGivesNumericPrototypeRejectsMissingPrototypeWithLegacyMessage(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	root := talkActionTestRoot(t, "계석치무", 25, "보상 GIVE 999\n받게.\n")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 6)
	bob := make(chan session.Command, 6)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 보상 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"보상\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"받게.\"라고 이야기합니다.\n"})
	assertNoCommand(t, bob)
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"받게.\"라고 이야기합니다.\n계석치무는 당신에게 줄 물건을 아무것도 가지고 있지 않습니다.\n"})
	assertTalkCreatureHasPrototype(t, world, "creature:alice", legacyTalkGivePrototypeID(999), 0)
}

func TestTalkHandlerGivesNumericPrototypeHonorsLegacyCarryCountLimit(t *testing.T) {
	loaded := talkTestWorld(t)
	protoID := legacyTalkGivePrototypeID(124)
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          protoID,
		DisplayName: "보상패",
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          "prototype:held",
		DisplayName: "소지품",
	}); err != nil {
		t.Fatal(err)
	}
	alice := loaded.Creatures["creature:alice"]
	for i := 0; i <= giveInventoryLimit; i++ {
		objectID := model.ObjectInstanceID("object:held-" + strconv.Itoa(i))
		alice.Inventory.ObjectIDs = append(alice.Inventory.ObjectIDs, objectID)
		if err := loaded.AddObjectInstance(model.ObjectInstance{
			ID:          objectID,
			PrototypeID: "prototype:held",
			Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	loaded.Creatures[alice.ID] = alice

	world := state.NewWorld(loaded)
	root := talkActionTestRoot(t, "계석치무", 25, "보상 GIVE 124\n받게.\n")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	aliceCh := make(chan session.Command, 6)
	bob := make(chan session.Command, 6)
	registerTestSession(t, loop, "s1", aliceCh, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 보상 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"보상\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"받게.\"라고 이야기합니다.\n"})
	assertNoCommand(t, bob)
	assertCommand(t, aliceCh, session.Command{Write: "\n계석치무가 당신에게 \"받게.\"라고 이야기합니다.\n당신은 더이상 가질 수 없습니다.\n"})
	assertTalkCreatureHasPrototype(t, world, "creature:alice", protoID, 0)
}

func TestTalkHandlerGivesNumericPrototypeHonorsLegacyWeightLimit(t *testing.T) {
	loaded := talkTestWorld(t)
	protoID := legacyTalkGivePrototypeID(125)
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          protoID,
		DisplayName: "무거운 보상패",
		Properties:  map[string]string{"weight": "21"},
	}); err != nil {
		t.Fatal(err)
	}

	world := state.NewWorld(loaded)
	root := talkActionTestRoot(t, "계석치무", 25, "보상 GIVE 125\n받게.\n")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 6)
	bob := make(chan session.Command, 6)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 보상 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"보상\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"받게.\"라고 이야기합니다.\n"})
	assertNoCommand(t, bob)
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"받게.\"라고 이야기합니다.\n당신은 더이상 가질 수 없습니다.\n"})
	assertTalkCreatureHasPrototype(t, world, "creature:alice", protoID, 0)
}

func TestTalkFileActionFromLineParsesLegacyDirectives(t *testing.T) {
	tests := []struct {
		line string
		want talkFileAction
	}{
		{line: "열쇠 ATTACK", want: talkFileAction{Type: "ATTACK"}},
		{line: "안녕 ACTION 인사 PLAYER", want: talkFileAction{Type: "ACTION", Name: "인사", Target: "PLAYER"}},
		{line: "치료 CAST 치료 PLAYER", want: talkFileAction{Type: "CAST", Name: "치료", Target: "PLAYER"}},
		{line: "감지 CAST detect magic PLAYER", want: talkFileAction{Type: "CAST", Name: "detect", Target: "magic"}},
		{line: "비행 CAST fly", want: talkFileAction{Type: "CAST", Name: "fly"}},
		{line: "보상 GIVE 25냥", want: talkFileAction{Type: "GIVE", Name: "25냥"}},
	}

	for _, tt := range tests {
		if got := talkFileActionFromLine(tt.line); got != tt.want {
			t.Fatalf("talkFileActionFromLine(%q) = %+v, want %+v", tt.line, got, tt.want)
		}
	}
}

func talkActionTestRoot(t *testing.T, name string, level int, content string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "objmon", "talk")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	filename, ok := legacyTalkFilename(name, level)
	if !ok {
		t.Fatal("legacyTalkFilename() failed")
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func assertTalkCreatureHasPrototype(t *testing.T, world *state.World, creatureID model.CreatureID, protoID model.PrototypeID, wantCount int) model.ObjectInstanceID {
	t.Helper()
	creature, ok := world.Creature(creatureID)
	if !ok {
		t.Fatalf("Creature(%q) missing", creatureID)
	}
	var found model.ObjectInstanceID
	var count int
	for _, objectID := range creature.Inventory.ObjectIDs {
		object, ok := world.Object(objectID)
		if !ok {
			continue
		}
		if object.PrototypeID == protoID {
			found = objectID
			count++
		}
	}
	if count != wantCount {
		t.Fatalf("Creature(%q) prototype %q count = %d, want %d", creatureID, protoID, count, wantCount)
	}
	return found
}
