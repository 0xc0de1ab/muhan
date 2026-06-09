package game

import (
	"context"
	"fmt"
	"testing"

	enginecmd "muhan/internal/engine/command"
	"muhan/internal/session"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestParseTalkGiveObjectNumberMatchesLegacyAtoi(t *testing.T) {
	tests := []struct {
		item      string
		want      int
		wantFound bool
	}{
		{item: "106", want: 106, wantFound: true},
		{item: "106번", want: 106, wantFound: true},
		{item: "106냥", want: 106, wantFound: true},
		{item: "106-보상", want: 106, wantFound: true},
		{item: "보상106", wantFound: false},
		{item: "0", wantFound: false},
		{item: "-1", wantFound: false},
	}

	for _, tt := range tests {
		got, found := parseTalkGiveObjectNumber(tt.item)
		if got != tt.want || found != tt.wantFound {
			t.Fatalf("parseTalkGiveObjectNumber(%q) = (%d, %t), want (%d, %t)", tt.item, got, found, tt.want, tt.wantFound)
		}
	}
}

func TestTalkHandlerGiveLegacyAtoiSuffixPrefersLoadablePrototype(t *testing.T) {
	loaded := talkTestWorld(t)
	wise := loaded.Creatures["creature:wise"]
	wise.Stats = map[string]int{"gold": 70}
	loaded.Creatures[wise.ID] = wise
	protoID := legacyTalkGivePrototypeID(25)
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          protoID,
		DisplayName: "번호 보상",
	}); err != nil {
		t.Fatal(err)
	}

	world := state.NewWorld(loaded)
	root := talkActionTestRoot(t, "계석치무", 25, "보상 GIVE 25냥\n받게.\n")
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
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 번호 보상을 줍니다.\n"})
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"받게.\"라고 이야기합니다.\n\n계석치무가 당신에게 번호 보상을 줍니다.\n"})
	assertGiveCreatureGold(t, world, "creature:wise", 70)
	assertGiveCreatureGold(t, world, "creature:alice", 0)
	assertTalkCreatureHasPrototype(t, world, "creature:alice", protoID, 1)
}

func TestTalkFlagHelpersExpandLegacyAliases(t *testing.T) {
	if !talkCreatureHasFlag(model.Creature{
		Properties: map[string]string{"talkAggressive": "true"},
	}, "MTLKAG") {
		t.Fatal("talkCreatureHasFlag did not match talkAggressive property as MTLKAG")
	}
	if !talkCreatureHasFlag(model.Creature{
		Stats: map[string]int{"MTALKS": 1},
	}, "talks") {
		t.Fatal("talkCreatureHasFlag did not match MTALKS stat as talks")
	}

	loaded := talkTestWorld(t)
	protoID := model.PrototypeID("proto:talk-alias")
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          protoID,
		DisplayName: "별칭물건",
		Properties:  map[string]string{"inventoryPermanent": "yes"},
	}); err != nil {
		t.Fatal(err)
	}
	world := state.NewWorld(loaded)
	if !talkObjectHasFlag(world, model.ObjectInstance{
		Properties: map[string]string{"OWTLES": "1"},
	}, "weightless") {
		t.Fatal("talkObjectHasFlag did not match OWTLES property as weightless")
	}
	if !talkObjectHasFlag(world, model.ObjectInstance{
		PrototypeID: protoID,
	}, "OPERM2") {
		t.Fatal("talkObjectHasFlag did not match prototype inventoryPermanent property as OPERM2")
	}
}

func TestTalkHandlerGiveEventAliasPropertySetsOwnerLikeLegacy(t *testing.T) {
	tests := []struct {
		name       string
		number     int
		properties map[string]string
	}{
		{
			name:       "canonical property",
			number:     128,
			properties: map[string]string{"eventItem": "1"},
		},
		{
			name:       "flags container token",
			number:     129,
			properties: map[string]string{"flags": "eventItem"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := talkTestWorld(t)
			protoID := legacyTalkGivePrototypeID(tt.number)
			if err := loaded.AddObjectPrototype(model.ObjectPrototype{
				ID:          protoID,
				DisplayName: "이벤트 패",
				Properties:  tt.properties,
			}); err != nil {
				t.Fatal(err)
			}

			world := state.NewWorld(loaded)
			root := talkActionTestRoot(t, "계석치무", 25, fmt.Sprintf("보상 GIVE %d\n받게.\n", tt.number))
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
			assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 이벤트 패를 줍니다.\n"})
			assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"받게.\"라고 이야기합니다.\n\n계석치무가 당신에게 이벤트 패를 줍니다.\n"})

			objectID := assertTalkCreatureHasPrototype(t, world, "creature:alice", protoID, 1)
			object, ok := world.Object(objectID)
			if !ok {
				t.Fatalf("Object(%q) missing", objectID)
			}
			if got := object.Properties["key[2]"]; got != "Alice" {
				t.Fatalf("talk gift event owner key = %q, want Alice; properties = %+v", got, object.Properties)
			}
		})
	}
}

func TestTalkHandlerGivePrototypeRollsBackMaterializedObjectWhenNestedWeightFails(t *testing.T) {
	loaded := talkTestWorld(t)
	protoID := legacyTalkGivePrototypeID(126)
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          protoID,
		Kind:        model.ObjectKindContainer,
		DisplayName: "묵직한 주머니",
		Metadata: model.Metadata{PrototypeResolution: &model.PrototypeResolutionMetadata{
			MaterializedFromObjectInstanceID: "object:template-bag",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          "prototype:heavy-gem",
		DisplayName: "무거운 보석",
		Properties:  map[string]string{"weight": "25"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddObjectInstance(model.ObjectInstance{
		ID:          "object:template-bag",
		PrototypeID: protoID,
		Location:    model.ObjectLocation{RoomID: "room:three"},
		Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:template-gem"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddObjectInstance(model.ObjectInstance{
		ID:          "object:template-gem",
		PrototypeID: "prototype:heavy-gem",
		Location:    model.ObjectLocation{ContainerID: "object:template-bag"},
	}); err != nil {
		t.Fatal(err)
	}

	world := state.NewWorld(loaded)
	template, ok := world.Object("object:template-bag")
	if !ok {
		t.Fatal("missing template bag")
	}
	if got := talkObjectTotalWeight(world, template); got != 25 {
		t.Fatalf("template total weight = %d, want 25", got)
	}
	root := talkActionTestRoot(t, "계석치무", 25, "보상 GIVE 126\n받게.\n")
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
	if _, ok := world.Object("object:template-bag"); !ok {
		t.Fatal("template object was removed during rollback")
	}
}

func TestTalkHandlerGivePrototypeRollsBackObjectWhenMaterializedQuestAlreadyComplete(t *testing.T) {
	loaded := talkTestWorld(t)
	aliceCreature := loaded.Creatures["creature:alice"]
	aliceCreature.Properties = map[string]string{questCompletionKey(2): "1"}
	loaded.Creatures[aliceCreature.ID] = aliceCreature
	protoID := legacyTalkGivePrototypeID(127)
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          protoID,
		DisplayName: "비밀 패",
		Metadata: model.Metadata{PrototypeResolution: &model.PrototypeResolutionMetadata{
			MaterializedFromObjectInstanceID: "object:template-token",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddObjectInstance(model.ObjectInstance{
		ID:          "object:template-token",
		PrototypeID: protoID,
		Location:    model.ObjectLocation{RoomID: "room:three"},
		Properties:  map[string]string{"questNumber": "2"},
	}); err != nil {
		t.Fatal(err)
	}

	world := state.NewWorld(loaded)
	root := talkActionTestRoot(t, "계석치무", 25, "보상 GIVE 127\n받게.\n")
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
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"받게.\"라고 이야기합니다.\n" + questAlreadyCompletedMessage()})
	assertTalkCreatureHasPrototype(t, world, "creature:alice", protoID, 0)
	if _, ok := world.Object("object:template-token"); !ok {
		t.Fatal("template object was removed during rollback")
	}
}
