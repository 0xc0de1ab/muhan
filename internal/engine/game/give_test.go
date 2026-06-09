package game

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
	enginecmd "muhan/internal/engine/command"
	"muhan/internal/session"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestGiveHandlerTransfersInventoryObjectToActivePlayerInSameRoom(t *testing.T) {
	world := giveTestWorld(t)
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	charlie := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", charlie, "player:charlie")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "작은 돌 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 당신에게 작은 돌을 줍니다.\n"})
	assertCommand(t, charlie, session.Command{Write: "\nAlice가 Bob에게 작은 돌을 줍니다.\n"})
	assertCommand(t, alice, session.Command{Write: "당신은 Bob에게 작은 돌을 줍니다.\n"})
	assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:bob", Slot: "inventory"})
	assertGiveCreatureInventory(t, world, "creature:alice", "object:stone", false)
	assertGiveCreatureInventory(t, world, "creature:bob", "object:stone", true)
}

func TestGiveHandlerTransfersGoldToActivePlayerInSameRoom(t *testing.T) {
	world := giveTestWorld(t)
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	charlie := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", charlie, "player:charlie")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "40냥 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 당신에게 40냥을 주었습니다.\n"})
	assertCommand(t, charlie, session.Command{Write: "\nAlice가 Bob에게 40냥을 주었습니다.\n"})
	assertCommand(t, alice, session.Command{Write: "당신은 Bob에게 40냥을 주었습니다.\n"})
	assertGiveCreatureGold(t, world, "creature:alice", 60)
	assertGiveCreatureGold(t, world, "creature:bob", 45)
}

func TestGiveHandlerMatchesLowercizedASCIITargetLikeLegacy(t *testing.T) {
	world := giveTestWorld(t)
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	charlie := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", charlie, "player:charlie")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "40냥 bOb 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 당신에게 40냥을 주었습니다.\n"})
	assertCommand(t, charlie, session.Command{Write: "\nAlice가 Bob에게 40냥을 주었습니다.\n"})
	assertCommand(t, alice, session.Command{Write: "당신은 Bob에게 40냥을 주었습니다.\n"})
	assertGiveCreatureGold(t, world, "creature:alice", 60)
	assertGiveCreatureGold(t, world, "creature:bob", 45)
}

func TestGiveHandlerIgnoresSendErrorsAfterObjectTransferLikeLegacy(t *testing.T) {
	world := giveTestWorld(t)
	ctx := &enginecmd.Context{
		SessionID: "s1",
		ActorID:   "player:alice",
		Values: map[string]any{
			ContextActiveSessionsKey: func() []ActiveSession {
				return []ActiveSession{
					{ID: "s1", ActorID: "player:alice"},
					{ID: "s2", ActorID: "player:bob"},
					{ID: "s3", ActorID: "player:charlie"},
				}
			},
			ContextSendToSessionKey: func(session.ID, session.Command) error {
				return errors.New("session closed")
			},
		},
	}

	status, err := NewGiveHandler(world)(ctx, enginecmd.ResolvedCommand{
		Input: "작은 돌 Bob 줘",
		Spec:  commandspec.CommandSpec{Name: "줘", Handler: "give"},
		Args:  []string{"작은", "돌", "Bob"},
	})
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if status != enginecmd.StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:bob", Slot: "inventory"})
	assertGiveCreatureInventory(t, world, "creature:alice", "object:stone", false)
	assertGiveCreatureInventory(t, world, "creature:bob", "object:stone", true)
	if got, want := ctx.OutputString(), "당신은 Bob에게 작은 돌을 줍니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestGiveHandlerIgnoresSendErrorsAfterMoneyTransferLikeLegacy(t *testing.T) {
	world := giveTestWorld(t)
	ctx := &enginecmd.Context{
		SessionID: "s1",
		ActorID:   "player:alice",
		Values: map[string]any{
			ContextActiveSessionsKey: func() []ActiveSession {
				return []ActiveSession{
					{ID: "s1", ActorID: "player:alice"},
					{ID: "s2", ActorID: "player:bob"},
					{ID: "s3", ActorID: "player:charlie"},
				}
			},
			ContextSendToSessionKey: func(session.ID, session.Command) error {
				return errors.New("session closed")
			},
		},
	}

	status, err := NewGiveHandler(world)(ctx, enginecmd.ResolvedCommand{
		Input: "40냥 Bob 줘",
		Spec:  commandspec.CommandSpec{Name: "줘", Handler: "give"},
		Args:  []string{"40냥", "Bob"},
	})
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if status != enginecmd.StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	assertGiveCreatureGold(t, world, "creature:alice", 60)
	assertGiveCreatureGold(t, world, "creature:bob", 45)
	if got, want := ctx.OutputString(), "당신은 Bob에게 40냥을 주었습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestGiveHandlerUsesParsedSlotsWhenArgsAreAbsent(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantActor  string
		wantTarget string
		check      func(t *testing.T, world *state.World)
	}{
		{
			name:       "object",
			line:       "작은 돌 Bob 줘",
			wantActor:  "당신은 Bob에게 작은 돌을 줍니다.\n",
			wantTarget: "\nAlice가 당신에게 작은 돌을 줍니다.\n",
			check: func(t *testing.T, world *state.World) {
				t.Helper()
				assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:bob", Slot: "inventory"})
				assertGiveCreatureInventory(t, world, "creature:alice", "object:stone", false)
				assertGiveCreatureInventory(t, world, "creature:bob", "object:stone", true)
			},
		},
		{
			name:       "money",
			line:       "40냥 Bob 줘",
			wantActor:  "당신은 Bob에게 40냥을 주었습니다.\n",
			wantTarget: "\nAlice가 당신에게 40냥을 주었습니다.\n",
			check: func(t *testing.T, world *state.World) {
				t.Helper()
				assertGiveCreatureGold(t, world, "creature:alice", 60)
				assertGiveCreatureGold(t, world, "creature:bob", 45)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := giveTestWorld(t)
			bob := make(chan session.Command, 1)
			ctx := &enginecmd.Context{
				SessionID: "s1",
				ActorID:   "player:alice",
				Values: map[string]any{
					ContextActiveSessionsKey: func() []ActiveSession {
						return []ActiveSession{
							{ID: "s1", ActorID: "player:alice"},
							{ID: "s2", ActorID: "player:bob"},
						}
					},
					ContextSendToSessionKey: func(id session.ID, cmd session.Command) error {
						if id == "s2" {
							bob <- cmd
						}
						return nil
					},
				},
			}

			status, err := NewGiveHandler(world)(ctx, enginecmd.ResolvedCommand{
				Input:  tt.line,
				Parsed: commandparse.Parse(tt.line),
				Spec:   commandspec.CommandSpec{Name: "줘", Handler: "give"},
			})
			if err != nil {
				t.Fatalf("handler error = %v", err)
			}
			if status != enginecmd.StatusDefault {
				t.Fatalf("status = %v, want StatusDefault", status)
			}
			if got := ctx.OutputString(); got != tt.wantActor {
				t.Fatalf("actor output = %q, want %q", got, tt.wantActor)
			}
			assertCommand(t, bob, session.Command{Write: tt.wantTarget})
			tt.check(t, world)
		})
	}
}

func TestGiveHandlerParsesMoneyAmountLikeLegacyAtol(t *testing.T) {
	world := giveTestWorld(t)
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "1,000냥 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 당신에게 1냥을 주었습니다.\n"})
	assertCommand(t, alice, session.Command{Write: "당신은 Bob에게 1냥을 주었습니다.\n"})
	assertGiveCreatureGold(t, world, "creature:alice", 99)
	assertGiveCreatureGold(t, world, "creature:bob", 6)
}

func TestParseGiveMoneyMatchesLegacyAtolPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  int
		money bool
	}{
		{input: "25냥", want: 25, money: true},
		{input: "1,000냥", want: 1, money: true},
		{input: "12abc냥", want: 12, money: true},
		{input: "-5냥", want: -5, money: true},
		{input: "냥", want: 0, money: false},
		{input: "25", want: 0, money: false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, money := parseGiveMoney(tt.input)
			if got != tt.want || money != tt.money {
				t.Fatalf("parseGiveMoney(%q) = (%d, %v), want (%d, %v)", tt.input, got, money, tt.want, tt.money)
			}
		})
	}
}

func TestGiveHandlerTreatsBareCurrencySuffixAsObjectLikeLegacy(t *testing.T) {
	world := giveTestWorld(t)
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "냥 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "당신은 그런것을 갖고 있지 않습니다.\n"})
	assertNoCommand(t, bob)
	assertGiveCreatureGold(t, world, "creature:alice", 100)
	assertGiveCreatureGold(t, world, "creature:bob", 5)
}

func TestGiveHandlerTransfersGoldToRoomNPC(t *testing.T) {
	world := giveTestWorld(t)
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "25냥 경비병 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 경비병에게 25냥을 주었습니다.\n"})
	assertCommand(t, alice, session.Command{Write: "당신은 경비병에게 25냥을 주었습니다.\n"})
	assertGiveCreatureGold(t, world, "creature:alice", 75)
	assertGiveCreatureGold(t, world, "creature:guard", 25)
}

func TestGiveHandlerMoneySelfPlayerFallsBackToSameNameNPCLikeLegacy(t *testing.T) {
	world := giveTestWorldWithOptions(t, giveTestWorldOptions{
		extraCreatures: []model.Creature{
			{
				ID:          "creature:alice-npc",
				Kind:        model.CreatureKindNPC,
				DisplayName: "alice",
				RoomID:      "room:one",
				Stats:       map[string]int{"gold": 0},
			},
		},
	})
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "10냥 Alice 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 alice에게 10냥을 주었습니다.\n"})
	assertCommand(t, alice, session.Command{Write: "당신은 alice에게 10냥을 주었습니다.\n"})
	assertGiveCreatureGold(t, world, "creature:alice", 90)
	assertGiveCreatureGold(t, world, "creature:alice-npc", 10)
}

func TestGiveHandlerRejectsInvalidObjectTransfersWithoutMutation(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "missing args", line: "줘", want: "누구에게 주시려구요?\n"},
		{name: "missing object", line: "없는것 Bob 줘", want: "당신은 그런것을 갖고 있지 않습니다.\n"},
		{name: "missing target", line: "작은 돌 Nobody 줘", want: "그런 사람은 여기 없어요!\n"},
		{name: "self object", line: "작은 돌 Alice 줘", want: "자신에게는 물건을 줄 수 없습니다.\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := giveTestWorld(t)
			loop := NewLoop(giveTestDispatcher(t, world))
			alice := make(chan session.Command, 2)
			bob := make(chan session.Command, 2)
			registerTestSession(t, loop, "s1", alice, "player:alice")
			registerTestSession(t, loop, "s2", bob, "player:bob")

			if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: tt.line}); err != nil {
				t.Fatalf("HandleEvent() error = %v", err)
			}

			assertCommand(t, alice, session.Command{Write: tt.want})
			assertNoCommand(t, bob)
			assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"})
			assertGiveCreatureInventory(t, world, "creature:alice", "object:stone", true)
			assertGiveCreatureInventory(t, world, "creature:bob", "object:stone", false)
		})
	}
}

func TestGiveHandlerRejectsObjectIDTargetLikeLegacyFindObj(t *testing.T) {
	world := giveTestWorld(t)
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "object:stone Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "당신은 그런것을 갖고 있지 않습니다.\n"})
	assertNoCommand(t, bob)
	assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"})
	assertGiveCreatureInventory(t, world, "creature:alice", "object:stone", true)
	assertGiveCreatureInventory(t, world, "creature:bob", "object:stone", false)
}

func TestGiveHandlerRejectsOneByteTargetPrefixLikeLegacyFindCrt(t *testing.T) {
	world := giveTestWorld(t)
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "작은 돌 B 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "그런 사람은 여기 없어요!\n"})
	assertNoCommand(t, bob)
	assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"})
	assertGiveCreatureInventory(t, world, "creature:alice", "object:stone", true)
	assertGiveCreatureInventory(t, world, "creature:bob", "object:stone", false)
}

func TestSelectGiveObjectUsesLegacyPrefixOrderInsteadOfExactFirst(t *testing.T) {
	loaded := worldload.NewWorld()
	mustAddLoopCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindNPC,
		DisplayName: "Alice",
		Inventory: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
			"object:apple-slice",
			"object:apple",
		}},
	})
	for _, proto := range []model.ObjectPrototype{
		{ID: "prototype:apple-slice", DisplayName: "사과 조각", Properties: map[string]string{"name": "사과 조각"}},
		{ID: "prototype:apple", DisplayName: "사과", Properties: map[string]string{"name": "사과"}},
	} {
		if err := loaded.AddObjectPrototype(proto); err != nil {
			t.Fatal(err)
		}
	}
	for _, object := range []model.ObjectInstance{
		{ID: "object:apple-slice", PrototypeID: "prototype:apple-slice", Location: model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"}},
		{ID: "object:apple", PrototypeID: "prototype:apple", Location: model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"}},
	} {
		if err := loaded.AddObjectInstance(object); err != nil {
			t.Fatal(err)
		}
	}
	world := state.NewWorld(loaded)

	object, name, ok := selectGiveObject(world, []model.ObjectInstanceID{"object:apple-slice", "object:apple"}, "사과", 1, true)
	if !ok || object.ID != "object:apple-slice" || name != "사과 조각" {
		t.Fatalf("selectGiveObject() = (%q, %q, %v), want first prefix object", object.ID, name, ok)
	}
}

func TestGiveHandlerAllowsPlayerAtLegacyInventoryCountLimit(t *testing.T) {
	world := giveTestWorldWithOptions(t, giveTestWorldOptions{bobInventoryCount: giveInventoryLimit})
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "작은 돌 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 당신에게 작은 돌을 줍니다.\n"})
	assertCommand(t, alice, session.Command{Write: "당신은 Bob에게 작은 돌을 줍니다.\n"})
	assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:bob", Slot: "inventory"})
	assertGiveCreatureInventory(t, world, "creature:bob", "object:stone", true)
}

func TestGiveHandlerRejectsPlayerOverLegacyInventoryCountLimit(t *testing.T) {
	world := giveTestWorldWithOptions(t, giveTestWorldOptions{bobInventoryCount: giveInventoryLimit + 1})
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "작은 돌 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "Bob님은 더이상 가질 수 없습니다.\n"})
	assertNoCommand(t, bob)
	assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"})
	assertGiveCreatureInventory(t, world, "creature:bob", "object:stone", false)
}

func TestGiveHandlerRejectsPlayerOverLegacyWeightLimit(t *testing.T) {
	world := giveTestWorldWithOptions(t, giveTestWorldOptions{stoneProperties: map[string]string{"weight": "25"}})
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "작은 돌 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "Bob님은 더이상 가질 수 없습니다.\n"})
	assertNoCommand(t, bob)
	assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"})
	assertGiveCreatureInventory(t, world, "creature:bob", "object:stone", false)
}

func TestGiveHandlerDetectInvisibleUsesPropertyBackedCreatureFlags(t *testing.T) {
	world := giveTestWorld(t)
	if _, err := world.SetObjectProperty("object:stone", "OINVIS", "1"); err != nil {
		t.Fatalf("SetObjectProperty() error = %v", err)
	}
	if _, err := world.SetCreatureProperty("creature:alice", "flags", "PDINVI"); err != nil {
		t.Fatalf("SetCreatureProperty() error = %v", err)
	}
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "작은 돌 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 당신에게 작은 돌을 줍니다.\n"})
	assertCommand(t, alice, session.Command{Write: "당신은 Bob에게 작은 돌을 줍니다.\n"})
	assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:bob", Slot: "inventory"})
	assertGiveCreatureInventory(t, world, "creature:alice", "object:stone", false)
	assertGiveCreatureInventory(t, world, "creature:bob", "object:stone", true)
}

func TestGiveHandlerTargetVisibilityUsesPropertyBackedCreatureFlags(t *testing.T) {
	world := giveTestWorldWithOptions(t, giveTestWorldOptions{bobStats: map[string]int{"class": legacyClassCaretaker}})
	if _, err := world.SetCreatureProperty("creature:bob", "flags", "PDMINV"); err != nil {
		t.Fatalf("SetCreatureProperty() error = %v", err)
	}
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "작은 돌 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "그런 사람은 여기 없어요!\n"})
	assertNoCommand(t, bob)
	assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"})
	assertGiveCreatureInventory(t, world, "creature:bob", "object:stone", false)
}

func TestGiveHandlerRejectsPropertyBackedEventObjectLikeLegacy(t *testing.T) {
	world := giveTestWorldWithOptions(t, giveTestWorldOptions{stoneProperties: map[string]string{"OEVENT": "1", "key[2]": "Alice"}})
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "작은 돌 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "이벤트 아이템은 타인에게 줄 수 없습니다.\n"})
	assertNoCommand(t, bob)
	assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"})
	assertGiveCreatureInventory(t, world, "creature:bob", "object:stone", false)
}

func TestGiveHandlerRejectsCanonicalEventItemAliasLikeLegacy(t *testing.T) {
	world := giveTestWorldWithOptions(t, giveTestWorldOptions{stoneProperties: map[string]string{"eventItem": "1", "key[2]": "Alice"}})
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "작은 돌 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "이벤트 아이템은 타인에게 줄 수 없습니다.\n"})
	assertNoCommand(t, bob)
	assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"})
	assertGiveCreatureInventory(t, world, "creature:bob", "object:stone", false)
}

func TestGiveHandlerRejectsEventItemFlagsPropertyTokenLikeLegacy(t *testing.T) {
	world := giveTestWorldWithOptions(t, giveTestWorldOptions{stoneProperties: map[string]string{"flags": "eventItem", "key[2]": "Alice"}})
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "작은 돌 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "이벤트 아이템은 타인에게 줄 수 없습니다.\n"})
	assertNoCommand(t, bob)
	assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"})
	assertGiveCreatureInventory(t, world, "creature:bob", "object:stone", false)
}

func TestGiveHandlerAllowsTopLevelEventMarkerObjectLikeLegacy(t *testing.T) {
	world := giveTestWorldWithOptions(t, giveTestWorldOptions{stoneProperties: map[string]string{"OEVENT": "1", "key[2]": "이벤트"}})
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "작은 돌 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 당신에게 작은 돌을 줍니다.\n"})
	assertCommand(t, alice, session.Command{Write: "당신은 Bob에게 작은 돌을 줍니다.\n"})
	assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:bob", Slot: "inventory"})
	assertGiveCreatureInventory(t, world, "creature:alice", "object:stone", false)
	assertGiveCreatureInventory(t, world, "creature:bob", "object:stone", true)
}

func TestGiveHandlerRejectsContainedEventMarkerObjectLikeLegacy(t *testing.T) {
	world := giveTestWorldWithOptions(t, giveTestWorldOptions{
		extraPrototypes: []model.ObjectPrototype{
			{ID: "prototype:bag", Kind: model.ObjectKindContainer, DisplayName: "가방", Keywords: []string{"가방"}, Properties: map[string]string{"name": "가방"}},
			{ID: "prototype:event", DisplayName: "기념패", Keywords: []string{"기념패"}, Properties: map[string]string{"OEVENT": "1", "key[2]": "이벤트"}},
		},
		extraAliceObjects: []model.ObjectInstanceID{"object:bag"},
		extraObjects: []model.ObjectInstance{
			{
				ID:          "object:bag",
				PrototypeID: "prototype:bag",
				Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
				Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:event"}},
			},
			{
				ID:          "object:event",
				PrototypeID: "prototype:event",
				Location:    model.ObjectLocation{ContainerID: "object:bag"},
			},
		},
	})
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "가방 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "이벤트 아이템이 들어있으면 타인에게 줄 수 없습니다.\n"})
	assertNoCommand(t, bob)
	assertGiveObjectLocation(t, world, "object:bag", model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"})
	assertGiveObjectLocation(t, world, "object:event", model.ObjectLocation{ContainerID: "object:bag"})
	assertGiveCreatureInventory(t, world, "creature:alice", "object:bag", true)
	assertGiveCreatureInventory(t, world, "creature:bob", "object:bag", false)
}

func TestGiveHandlerRejectsDirectQuestObjectInsideContainerLikeLegacy(t *testing.T) {
	world := giveTestWorldWithOptions(t, giveTestWorldOptions{
		extraPrototypes: []model.ObjectPrototype{
			{ID: "prototype:bag", Kind: model.ObjectKindContainer, DisplayName: "가방", Keywords: []string{"가방"}, Properties: map[string]string{"name": "가방"}},
			{ID: "prototype:relic", DisplayName: "성물", Keywords: []string{"성물"}, Properties: map[string]string{"questNumber": "2"}},
		},
		extraAliceObjects: []model.ObjectInstanceID{"object:bag"},
		extraObjects: []model.ObjectInstance{
			{
				ID:          "object:bag",
				PrototypeID: "prototype:bag",
				Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
				Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:relic"}},
			},
			{
				ID:          "object:relic",
				PrototypeID: "prototype:relic",
				Location:    model.ObjectLocation{ContainerID: "object:bag"},
			},
		},
	})
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "가방 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "임무에 관련되는 물건이 들어있으면 타인에게 줄 수 없습니다.\n"})
	assertNoCommand(t, bob)
	assertGiveObjectLocation(t, world, "object:bag", model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"})
	assertGiveCreatureInventory(t, world, "creature:alice", "object:bag", true)
	assertGiveCreatureInventory(t, world, "creature:bob", "object:bag", false)
}

func TestGiveHandlerAllowsNestedQuestGrandchildContainerLikeLegacy(t *testing.T) {
	world := giveTestWorldWithOptions(t, giveTestWorldOptions{
		extraPrototypes: []model.ObjectPrototype{
			{ID: "prototype:bag", Kind: model.ObjectKindContainer, DisplayName: "가방", Keywords: []string{"가방"}, Properties: map[string]string{"name": "가방"}},
			{ID: "prototype:pouch", Kind: model.ObjectKindContainer, DisplayName: "주머니", Keywords: []string{"주머니"}, Properties: map[string]string{"name": "주머니"}},
			{ID: "prototype:relic", DisplayName: "성물", Keywords: []string{"성물"}, Properties: map[string]string{"questNumber": "2"}},
		},
		extraAliceObjects: []model.ObjectInstanceID{"object:bag"},
		extraObjects: []model.ObjectInstance{
			{
				ID:          "object:bag",
				PrototypeID: "prototype:bag",
				Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
				Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:pouch"}},
			},
			{
				ID:          "object:pouch",
				PrototypeID: "prototype:pouch",
				Location:    model.ObjectLocation{ContainerID: "object:bag"},
				Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:relic"}},
			},
			{
				ID:          "object:relic",
				PrototypeID: "prototype:relic",
				Location:    model.ObjectLocation{ContainerID: "object:pouch"},
			},
		},
	})
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "가방 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 당신에게 가방을 줍니다.\n"})
	assertCommand(t, alice, session.Command{Write: "당신은 Bob에게 가방을 줍니다.\n"})
	assertGiveObjectLocation(t, world, "object:bag", model.ObjectLocation{CreatureID: "creature:bob", Slot: "inventory"})
	assertGiveObjectLocation(t, world, "object:pouch", model.ObjectLocation{ContainerID: "object:bag"})
	assertGiveObjectLocation(t, world, "object:relic", model.ObjectLocation{ContainerID: "object:pouch"})
	assertGiveCreatureInventory(t, world, "creature:alice", "object:bag", false)
	assertGiveCreatureInventory(t, world, "creature:bob", "object:bag", true)
}

func TestGiveHandlerClearsHiddenAfterFindingObjectLikeLegacy(t *testing.T) {
	world := giveTestWorld(t)
	if _, err := world.UpdateCreatureTags("creature:alice", []string{"hidden"}, nil); err != nil {
		t.Fatalf("UpdateCreatureTags() error = %v", err)
	}
	if _, err := world.UpdatePlayerTags("player:alice", []string{"hidden"}, nil); err != nil {
		t.Fatalf("UpdatePlayerTags() error = %v", err)
	}
	if err := world.SetCreatureStat("creature:alice", "PHIDDN", 1); err != nil {
		t.Fatalf("SetCreatureStat() error = %v", err)
	}
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "작은 돌 Nobody 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "그런 사람은 여기 없어요!\n"})
	assertNoCommand(t, bob)
	assertGiveHiddenCleared(t, world)
	assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"})
}

func TestGiveHandlerKeepsHiddenWhenObjectMissingLikeLegacy(t *testing.T) {
	world := giveTestWorld(t)
	if _, err := world.UpdateCreatureTags("creature:alice", []string{"hidden"}, nil); err != nil {
		t.Fatalf("UpdateCreatureTags() error = %v", err)
	}
	if _, err := world.UpdatePlayerTags("player:alice", []string{"hidden"}, nil); err != nil {
		t.Fatalf("UpdatePlayerTags() error = %v", err)
	}
	if err := world.SetCreatureStat("creature:alice", "PHIDDN", 1); err != nil {
		t.Fatalf("SetCreatureStat() error = %v", err)
	}
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "없는것 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "당신은 그런것을 갖고 있지 않습니다.\n"})
	assertNoCommand(t, bob)
	assertGiveHiddenPresent(t, world)
	assertGiveObjectLocation(t, world, "object:stone", model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"})
}

func TestGiveHandlerMoneyKeepsHiddenLikeLegacy(t *testing.T) {
	world := giveTestWorld(t)
	if _, err := world.UpdateCreatureTags("creature:alice", []string{"hidden"}, nil); err != nil {
		t.Fatalf("UpdateCreatureTags() error = %v", err)
	}
	if _, err := world.UpdatePlayerTags("player:alice", []string{"hidden"}, nil); err != nil {
		t.Fatalf("UpdatePlayerTags() error = %v", err)
	}
	if err := world.SetCreatureStat("creature:alice", "PHIDDN", 1); err != nil {
		t.Fatalf("SetCreatureStat() error = %v", err)
	}
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "10냥 Bob 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 당신에게 10냥을 주었습니다.\n"})
	assertCommand(t, alice, session.Command{Write: "당신은 Bob에게 10냥을 주었습니다.\n"})
	assertGiveHiddenPresent(t, world)
	assertGiveCreatureGold(t, world, "creature:alice", 90)
	assertGiveCreatureGold(t, world, "creature:bob", 15)
}

func TestGiveHandlerRejectsInvalidGoldTransfersWithoutMutation(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "non positive", line: "0냥 Bob 줘", want: "돈의 단위는 음수가 될수 없습니다.\n"},
		{name: "insufficient", line: "150냥 Bob 줘", want: "당신은 그만큼의 돈을 가지고 있지 않습니다.\n"},
		{name: "insufficient before missing target", line: "150냥 Nobody 줘", want: "당신은 그만큼의 돈을 가지고 있지 않습니다.\n"},
		{name: "self", line: "10냥 Alice 줘", want: "그런 사람은 여기 없어요!\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := giveTestWorld(t)
			loop := NewLoop(giveTestDispatcher(t, world))
			alice := make(chan session.Command, 2)
			bob := make(chan session.Command, 2)
			registerTestSession(t, loop, "s1", alice, "player:alice")
			registerTestSession(t, loop, "s2", bob, "player:bob")

			if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: tt.line}); err != nil {
				t.Fatalf("HandleEvent() error = %v", err)
			}

			assertCommand(t, alice, session.Command{Write: tt.want})
			assertNoCommand(t, bob)
			assertGiveCreatureGold(t, world, "creature:alice", 100)
			assertGiveCreatureGold(t, world, "creature:bob", 5)
		})
	}
}

type giveTestWorldOptions struct {
	bobInventoryCount int
	bobStats          map[string]int
	stoneProperties   map[string]string
	extraPrototypes   []model.ObjectPrototype
	extraCreatures    []model.Creature
	extraObjects      []model.ObjectInstance
	extraAliceObjects []model.ObjectInstanceID
}

func giveTestDispatcher(t *testing.T, world *state.World) enginecmd.Dispatcher {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "줘", Number: 47, Handler: "give"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return enginecmd.Dispatcher{
		Registry: registry,
		Handlers: map[string]enginecmd.Handler{
			"give": NewGiveHandler(world),
		},
	}
}

func giveTestWorld(t *testing.T) *state.World {
	t.Helper()
	return giveTestWorldWithOptions(t, giveTestWorldOptions{})
}

func giveTestWorldWithOptions(t *testing.T, opts giveTestWorldOptions) *state.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLoopRoom(t, loaded, model.Room{ID: "room:one", DisplayName: "One"})
	for _, player := range []model.Player{
		{ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:one"},
		{ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob", RoomID: "room:one"},
		{ID: "player:charlie", DisplayName: "Charlie", CreatureID: "creature:charlie", RoomID: "room:one"},
	} {
		mustAddLoopPlayer(t, loaded, player)
	}
	bobInventory := make([]model.ObjectInstanceID, 0, opts.bobInventoryCount)
	for i := 0; i < opts.bobInventoryCount; i++ {
		bobInventory = append(bobInventory, model.ObjectInstanceID("object:bob-filler-"+strconv.Itoa(i)))
	}
	bobStats := map[string]int{"gold": 5}
	for key, value := range opts.bobStats {
		bobStats[key] = value
	}
	for _, creature := range []model.Creature{
		{
			ID:          "creature:alice",
			Kind:        model.CreatureKindPlayer,
			DisplayName: "Alice",
			PlayerID:    "player:alice",
			RoomID:      "room:one",
			Stats:       map[string]int{"gold": 100},
			Inventory: model.ObjectRefList{ObjectIDs: append([]model.ObjectInstanceID{
				"object:stone",
				"object:quest-scroll-1",
				"object:quest-scroll-2",
			}, opts.extraAliceObjects...)},
		},
		{
			ID:          "creature:bob",
			Kind:        model.CreatureKindPlayer,
			DisplayName: "Bob",
			PlayerID:    "player:bob",
			RoomID:      "room:one",
			Stats:       bobStats,
			Inventory:   model.ObjectRefList{ObjectIDs: bobInventory},
		},
		{
			ID:          "creature:charlie",
			Kind:        model.CreatureKindPlayer,
			DisplayName: "Charlie",
			PlayerID:    "player:charlie",
			RoomID:      "room:one",
			Stats:       map[string]int{"gold": 0},
		},
		{
			ID:          "creature:guard",
			Kind:        model.CreatureKindNPC,
			DisplayName: "경비병",
			RoomID:      "room:one",
			Stats:       map[string]int{"gold": 0},
		},
	} {
		mustAddLoopCreature(t, loaded, creature)
	}
	for _, creature := range opts.extraCreatures {
		mustAddLoopCreature(t, loaded, creature)
	}
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          "prototype:stone",
		DisplayName: "돌",
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          "prototype:filler",
		DisplayName: "조약돌",
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          "prototype:quest-scroll",
		DisplayName: "임무 두루마기",
		Keywords:    []string{"임무", "두루마기"},
		Properties:  map[string]string{"questNumber": "1"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, proto := range opts.extraPrototypes {
		if err := loaded.AddObjectPrototype(proto); err != nil {
			t.Fatal(err)
		}
	}
	if err := loaded.AddObjectInstance(model.ObjectInstance{
		ID:                  "object:stone",
		PrototypeID:         "prototype:stone",
		DisplayNameOverride: "작은 돌",
		Properties:          opts.stoneProperties,
		Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range bobInventory {
		if err := loaded.AddObjectInstance(model.ObjectInstance{
			ID:          id,
			PrototypeID: "prototype:filler",
			Location:    model.ObjectLocation{CreatureID: "creature:bob", Slot: "inventory"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []model.ObjectInstanceID{"object:quest-scroll-1", "object:quest-scroll-2"} {
		if err := loaded.AddObjectInstance(model.ObjectInstance{
			ID:          id,
			PrototypeID: "prototype:quest-scroll",
			Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, object := range opts.extraObjects {
		if err := loaded.AddObjectInstance(object); err != nil {
			t.Fatal(err)
		}
	}
	return state.NewWorld(loaded)
}

func assertGiveObjectLocation(t *testing.T, world *state.World, objectID model.ObjectInstanceID, want model.ObjectLocation) {
	t.Helper()
	object, ok := world.Object(objectID)
	if !ok {
		t.Fatalf("missing object %q", objectID)
	}
	if object.Location != want {
		t.Fatalf("object %q location = %+v, want %+v", objectID, object.Location, want)
	}
}

func assertGiveCreatureInventory(t *testing.T, world *state.World, creatureID model.CreatureID, objectID model.ObjectInstanceID, want bool) {
	t.Helper()
	creature, ok := world.Creature(creatureID)
	if !ok {
		t.Fatalf("missing creature %q", creatureID)
	}
	if got := giveObjectListContains(creature.Inventory.ObjectIDs, objectID); got != want {
		t.Fatalf("creature %q inventory contains %q = %v, want %v; inventory = %+v", creatureID, objectID, got, want, creature.Inventory.ObjectIDs)
	}
}

func giveObjectListContains(ids []model.ObjectInstanceID, id model.ObjectInstanceID) bool {
	for _, existing := range ids {
		if existing == id {
			return true
		}
	}
	return false
}

func assertGiveCreatureGold(t *testing.T, world *state.World, creatureID model.CreatureID, want int) {
	t.Helper()
	creature, ok := world.Creature(creatureID)
	if !ok {
		t.Fatalf("missing creature %q", creatureID)
	}
	if got := creature.Stats["gold"]; got != want {
		t.Fatalf("creature %q gold = %d, want %d", creatureID, got, want)
	}
}

func assertGiveHiddenCleared(t *testing.T, world *state.World) {
	t.Helper()
	creature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("missing alice creature")
	}
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "hidden", "phiddn") || creature.Stats["PHIDDN"] != 0 {
		t.Fatalf("alice creature hidden state = tags:%+v stats:%+v", creature.Metadata.Tags, creature.Stats)
	}
	player, ok := world.Player("player:alice")
	if !ok {
		t.Fatal("missing alice player")
	}
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("alice player hidden tags = %+v", player.Metadata.Tags)
	}
}

func assertGiveHiddenPresent(t *testing.T, world *state.World) {
	t.Helper()
	creature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("missing alice creature")
	}
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "hidden", "phiddn") || creature.Stats["PHIDDN"] == 0 {
		t.Fatalf("alice creature hidden state = tags:%+v stats:%+v, want hidden", creature.Metadata.Tags, creature.Stats)
	}
	player, ok := world.Player("player:alice")
	if !ok {
		t.Fatal("missing alice player")
	}
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("alice player hidden tags = %+v, want hidden", player.Metadata.Tags)
	}
}

func TestGiveHandlerRejectsObjectTransferToNPCLikeLegacy(t *testing.T) {
	world := giveTestWorld(t)
	if _, err := world.UpdateCreatureTags("creature:alice", []string{"hidden"}, nil); err != nil {
		t.Fatalf("UpdateCreatureTags() error = %v", err)
	}
	if _, err := world.UpdatePlayerTags("player:alice", []string{"hidden"}, nil); err != nil {
		t.Fatalf("UpdatePlayerTags() error = %v", err)
	}
	if err := world.SetCreatureStat("creature:alice", "PHIDDN", 1); err != nil {
		t.Fatalf("SetCreatureStat() error = %v", err)
	}
	loop := NewLoop(giveTestDispatcher(t, world))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "임무 경비병 줘"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "그런 사람은 여기 없어요!\n"})
	assertNoCommand(t, bob)
	assertGiveHiddenCleared(t, world)
	assertGiveObjectLocation(t, world, "object:quest-scroll-1", model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"})
	assertGiveCreatureInventory(t, world, "creature:alice", "object:quest-scroll-1", true)
	assertGiveCreatureInventory(t, world, "creature:guard", "object:quest-scroll-1", false)
	assertGiveObjectLocation(t, world, "object:quest-scroll-2", model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"})
	aliceCreature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("missing alice creature")
	}
	if aliceCreature.Properties[questCompletionKey(1)] != "" {
		t.Fatalf("quest flag = %q, want empty", aliceCreature.Properties[questCompletionKey(1)])
	}
	if got := aliceCreature.Stats["experience"]; got != 0 {
		t.Fatalf("experience = %d, want 0", got)
	}
	for _, key := range []string{
		"proficiency/sharp",
		"proficiency/thrust",
		"proficiency/blunt",
		"proficiency/pole",
		"proficiency/missile",
		"realm/1",
		"realm/2",
		"realm/3",
		"realm/4",
	} {
		if got := aliceCreature.Properties[key]; got != "" {
			t.Fatalf("%s = %q, want empty", key, got)
		}
	}
}
