package command

import (
	"strings"
	"testing"

	"muhan/internal/commandspec"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestPeekHandlerShowsMonsterInventory(t *testing.T) {
	world := state.NewWorld(peekWorld(t, legacyClassThief))
	dispatcher := peekDispatcher(t, world, fixedPeekRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "상인 엿봐")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "그녀의 소지품: 사과") || strings.Contains(out, "숨은 보석") || strings.Contains(out, "투명 동전") {
		t.Fatalf("output = %q, want visible monster inventory only", out)
	}
}

func TestPeekHandlerShowsPlayerInventoryAfterMonsterSearch(t *testing.T) {
	world := state.NewWorld(peekWorld(t, legacyClassThief))
	dispatcher := peekDispatcher(t, world, fixedPeekRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "Bob 엿봐")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "그녀의 소지품: 목걸이") {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
}

func TestPeekHandlerRejectsClassBlindMissingAndProtectedTargets(t *testing.T) {
	tests := []struct {
		name   string
		class  int
		tags   []string
		target string
		mutate func(*worldload.World)
		want   string
	}{
		{name: "missing arg", class: legacyClassThief, want: "누구의 소지품을 보려구요?"},
		{name: "wrong class", class: legacyClassFighter, target: "상인", want: "당신 직업으로는 다른사람의 소지품을 볼 수 없습니다."},
		{name: "blind", class: legacyClassThief, tags: []string{"blind"}, target: "상인", want: "당신은 눈이 멀어 있습니다!"},
		{name: "missing target", class: legacyClassThief, target: "없는", want: "그런 사람 없어요!"},
		{
			name:   "protected target",
			class:  legacyClassThief,
			target: "상인",
			mutate: func(loaded *worldload.World) {
				merchant := loaded.Creatures["creature:merchant"]
				merchant.Metadata.Tags = []string{"noSteal"}
				loaded.Creatures[merchant.ID] = merchant
			},
			want: "당신은 다른사람의 소지품을 볼 수 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := peekWorld(t, tt.class)
			actor := loaded.Creatures["creature:alice"]
			actor.Metadata.Tags = tt.tags
			loaded.Creatures[actor.ID] = actor
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
			handler := NewPeekHandler(world, fixedPeekRoll(1))
			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{Args: compactArgs(tt.target)})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestPeekHandlerFailureDoesNotShowInventory(t *testing.T) {
	world := state.NewWorld(peekWorld(t, legacyClassThief))
	handler := NewPeekHandler(world, fixedPeekRoll(100))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "실패하였습니다!" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
}

func TestPeekHandlerAppliesLegacyCooldownAfterTargetLookup(t *testing.T) {
	world := state.NewWorld(peekWorld(t, legacyClassThief))
	handler := NewPeekHandler(world, fixedPeekRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	if _, err := handler(ctx, ResolvedCommand{Args: []string{"Bob"}}); err != nil {
		t.Fatalf("first handler() error = %v", err)
	}
	ctx = &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("second handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "기다리세요.") {
		t.Fatalf("status/output = %d/%q, want legacy wait", status, ctx.OutputString())
	}
}

func TestPeekHandlerNotifiesVictimAndRoomWhenCaughtLikeLegacy(t *testing.T) {
	loaded := peekWorld(t, legacyClassThief)
	mustAddLookPlayer(t, loaded, model.Player{ID: "player:charlie", DisplayName: "Charlie", CreatureID: "creature:charlie", RoomID: "room:peek"})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:charlie",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Charlie",
		PlayerID:    "player:charlie",
		RoomID:      "room:peek",
	})
	world := state.NewWorld(loaded)

	type peekSession struct {
		ID      string
		ActorID string
	}
	type peekCommand struct {
		Write string
	}
	sent := map[string][]string{}
	ctx := &Context{
		ActorID:   "player:alice",
		SessionID: "session:alice",
		Values: map[string]any{
			"game.activeSessions": func() []peekSession {
				return []peekSession{
					{ID: "session:alice", ActorID: "player:alice"},
					{ID: "session:bob", ActorID: "player:bob"},
					{ID: "session:charlie", ActorID: "player:charlie"},
				}
			},
			"game.sendToSession": func(id string, command peekCommand) error {
				sent[id] = append(sent[id], command.Write)
				return nil
			},
		},
	}
	handler := NewPeekHandler(world, sequencePeekRoll(1, 100))
	status, err := handler(ctx, ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그녀의 소지품: 목걸이" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	if got := strings.Join(sent["session:bob"], ""); got != "Alice님이 당신의 소지품을 슬쩍 엿봅니다." {
		t.Fatalf("victim send = %q", got)
	}
	if got := strings.Join(sent["session:charlie"], ""); got != "\nAlice이 Bob의 소지품을 슬쩍 엿봅니다." {
		t.Fatalf("room send = %q", got)
	}
	if got := strings.Join(sent["session:alice"], ""); got != "" {
		t.Fatalf("actor send = %q, want none", got)
	}
}

func peekDispatcher(t *testing.T, world *state.World, roll PeekRollFunc) Dispatcher {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "엿봐", Number: 22, Handler: "peek"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return Dispatcher{Registry: registry, Handlers: map[string]Handler{"peek": NewPeekHandler(world, roll)}}
}

func peekWorld(t *testing.T, class int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{ID: "room:peek", DisplayName: "Peek"})
	mustAddLookPlayer(t, loaded, model.Player{ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:peek"})
	mustAddLookPlayer(t, loaded, model.Player{ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob", RoomID: "room:peek"})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:peek",
		Stats:       map[string]int{"class": class, "level": 20},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:peek",
		Stats:       map[string]int{"level": 1},
		Inventory:   model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:necklace"}},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:peek",
		Stats:       map[string]int{"level": 1},
		Inventory: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
			"object:apple",
			"object:hidden-gem",
			"object:invisible-coin",
		}},
	})
	for _, proto := range []model.ObjectPrototype{
		{ID: "prototype:apple", DisplayName: "사과"},
		{ID: "prototype:hidden-gem", DisplayName: "숨은 보석"},
		{ID: "prototype:invisible-coin", DisplayName: "투명 동전"},
		{ID: "prototype:necklace", DisplayName: "목걸이"},
	} {
		mustAddLookPrototype(t, loaded, proto)
	}
	mustAddLookObject(t, loaded, model.ObjectInstance{ID: "object:apple", PrototypeID: "prototype:apple", Location: model.ObjectLocation{CreatureID: "creature:merchant", Slot: "inventory"}})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:hidden-gem",
		PrototypeID: "prototype:hidden-gem",
		Location:    model.ObjectLocation{CreatureID: "creature:merchant", Slot: "inventory"},
		Metadata:    model.Metadata{Tags: []string{"hidden"}},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:invisible-coin",
		PrototypeID: "prototype:invisible-coin",
		Location:    model.ObjectLocation{CreatureID: "creature:merchant", Slot: "inventory"},
		Metadata:    model.Metadata{Tags: []string{"invisible"}},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{ID: "object:necklace", PrototypeID: "prototype:necklace", Location: model.ObjectLocation{CreatureID: "creature:bob", Slot: "inventory"}})
	return loaded
}

func fixedPeekRoll(value int) PeekRollFunc {
	return func(int, int) int {
		return value
	}
}

func sequencePeekRoll(values ...int) PeekRollFunc {
	index := 0
	return func(int, int) int {
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}

func compactArgs(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return []string{value}
}
