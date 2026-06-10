package command

import (
	"strings"
	"testing"
	"time"

	"muhan/internal/commandspec"
	"muhan/internal/session"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestStealHandlerMovesMonsterInventoryObject(t *testing.T) {
	world := state.NewWorld(stealWorld(t, model.ClassThief))
	dispatcher := stealDispatcher(t, world, fixedStealRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "사과 상인 훔쳐")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "훔쳤습니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	object, ok := world.Object("object:apple")
	if !ok || !objectLocatedInCreatureInventory(object, "creature:alice") {
		t.Fatalf("apple location = %+v, want alice inventory", object.Location)
	}
	merchant, _ := world.Creature("creature:merchant")
	if len(merchant.Inventory.ObjectIDs) != 1 || merchant.Inventory.ObjectIDs[0] != "object:money" {
		t.Fatalf("merchant inventory = %+v, want money only", merchant.Inventory.ObjectIDs)
	}
}

func TestStealHandlerCreditsMoneyObject(t *testing.T) {
	world := state.NewWorld(stealWorld(t, model.ClassThief))
	handler := NewStealHandler(world, fixedStealRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"돈", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "당신은 120냥을 훔쳐 220냥을 갖고 있습니다.") {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got := creatureStat(alice, "gold"); got != 220 {
		t.Fatalf("alice gold = %d, want 220", got)
	}
	if _, ok := world.Object("object:money"); ok {
		t.Fatal("money object still exists after steal")
	}
}

func TestStealHandlerSuccessQueuesLegacySaves(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		mutate func(*worldload.World)
		want   []model.PlayerID
	}{
		{
			name: "money from monster saves thief",
			args: []string{"돈", "상인"},
			want: []model.PlayerID{"player:alice"},
		},
		{
			name: "object from player saves thief and victim",
			args: []string{"목걸이", "Bob"},
			mutate: func(loaded *worldload.World) {
				for _, creatureID := range []model.CreatureID{"creature:alice", "creature:bob"} {
					creature := loaded.Creatures[creatureID]
					creature.Metadata.Tags = []string{"chaos"}
					loaded.Creatures[creatureID] = creature
				}
			},
			want: []model.PlayerID{"player:alice", "player:bob"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := stealWorld(t, model.ClassThief)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := &stealSaveTrackingWorld{World: state.NewWorld(loaded)}
			handler := NewStealHandler(world, fixedStealRoll(1))

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want default", status)
			}
			assertStealPlayerIDs(t, "dirty", world.dirty, tt.want)
			assertStealPlayerIDs(t, "queued", world.queued, tt.want)
		})
	}
}

func TestStealHandlerDMWithThiefSpellCanStealQuestObjectLikeLegacy(t *testing.T) {
	loaded := stealWorld(t, model.ClassDM)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"STHIEF"}
	loaded.Creatures[alice.ID] = alice
	apple := loaded.Objects["object:apple"]
	apple.Properties = map[string]string{"questnum": "7"}
	loaded.Objects[apple.ID] = apple
	world := state.NewWorld(loaded)
	handler := NewStealHandler(world, fixedStealRoll(100))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"사과", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "훔쳤습니다." {
		t.Fatalf("status/output = %d/%q, want DM quest steal success", status, ctx.OutputString())
	}
	object, _ := world.Object("object:apple")
	if !objectLocatedInCreatureInventory(object, "creature:alice") {
		t.Fatalf("apple location = %+v, want alice inventory", object.Location)
	}
}

func TestStealHandlerDMWithThiefSpellStillCannotStealTopLevelEventObjectLikeLegacy(t *testing.T) {
	loaded := stealWorld(t, model.ClassDM)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"STHIEF"}
	loaded.Creatures[alice.ID] = alice
	apple := loaded.Objects["object:apple"]
	apple.Properties = map[string]string{"OEVENT": "1"}
	loaded.Objects[apple.ID] = apple
	world := state.NewWorld(loaded)
	handler := NewStealHandler(world, fixedStealRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"사과", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "실패하였습니다.\n그가 당신을 공격합니다." {
		t.Fatalf("status/output = %d/%q, want event steal failure", status, ctx.OutputString())
	}
	object, _ := world.Object("object:apple")
	if !objectLocatedInCreatureInventory(object, "creature:merchant") {
		t.Fatalf("apple location = %+v, want merchant inventory", object.Location)
	}
}

func TestStealHandlerRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name   string
		class  int
		tags   []string
		args   []string
		mutate func(*worldload.World)
		want   string
	}{
		{name: "missing object", class: model.ClassThief, want: "무엇을 훔치려구요?"},
		{name: "missing target", class: model.ClassThief, args: []string{"사과"}, want: "누구한테서 훔치려구요?"},
		{name: "wrong class", class: model.ClassFighter, args: []string{"사과", "상인"}, want: "도둑만 훔칠수 있습니다."},
		{name: "invincible without thief spell", class: model.ClassInvincible, args: []string{"사과", "상인"}, want: "\n도둑을 무적수련하지 않았습니다.\n"},
		{name: "blind", class: model.ClassThief, tags: []string{"blind"}, args: []string{"사과", "상인"}, want: "당신은 눈이 멀어 훔칠 수 없습니다."},
		{name: "missing target", class: model.ClassThief, args: []string{"사과", "없는"}, want: "그런건 여기 없습니다."},
		{name: "missing object", class: model.ClassThief, args: []string{"없는", "상인"}, want: "그녀는 그런 물건을 갖고 있지 않습니다."},
		{
			name:  "protected monster",
			class: model.ClassThief,
			args:  []string{"사과", "상인"},
			mutate: func(loaded *worldload.World) {
				merchant := loaded.Creatures["creature:merchant"]
				merchant.Metadata.Tags = []string{"unkillable"}
				loaded.Creatures[merchant.ID] = merchant
			},
			want: "당신은 그녀를 해칠수 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := stealWorld(t, tt.class)
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = tt.tags
			loaded.Creatures[alice.ID] = alice
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
			handler := NewStealHandler(world, fixedStealRoll(1))

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestStealHandlerInvincibleWithThiefSpellCanSteal(t *testing.T) {
	loaded := stealWorld(t, model.ClassInvincible)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"STHIEF"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	handler := NewStealHandler(world, fixedStealRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"사과", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "훔쳤습니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
}

func TestStealHandlerRejectsOneByteTargetPrefixLikeLegacyFindCrt(t *testing.T) {
	loaded := stealWorld(t, model.ClassThief)
	merchant := loaded.Creatures["creature:merchant"]
	merchant.DisplayName = "Merchant"
	loaded.Creatures[merchant.ID] = merchant
	world := state.NewWorld(loaded)
	handler := NewStealHandler(world, fixedStealRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"사과", "M"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그런건 여기 없습니다." {
		t.Fatalf("status/output = %d/%q, want missing target", status, ctx.OutputString())
	}
	object, _ := world.Object("object:apple")
	if !objectLocatedInCreatureInventory(object, "creature:merchant") {
		t.Fatalf("apple location = %+v, want merchant inventory", object.Location)
	}
}

func TestStealHandlerRejectsObjectIDTargetLikeLegacyFindObj(t *testing.T) {
	world := state.NewWorld(stealWorld(t, model.ClassThief))
	handler := NewStealHandler(world, fixedStealRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"object:apple", "상인"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그녀는 그런 물건을 갖고 있지 않습니다." {
		t.Fatalf("status/output = %d/%q, want missing object", status, ctx.OutputString())
	}
	object, _ := world.Object("object:apple")
	if !objectLocatedInCreatureInventory(object, "creature:merchant") {
		t.Fatalf("apple location = %+v, want merchant inventory", object.Location)
	}
}

func TestStealHandlerRejectsEnemyMonsterTargetLikeLegacy(t *testing.T) {
	world := state.NewWorld(stealWorld(t, model.ClassThief))
	if _, err := world.AddEnemy("creature:merchant", "creature:alice"); err != nil {
		t.Fatalf("AddEnemy() error = %v", err)
	}
	handler := NewStealHandler(world, fixedStealRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"사과", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그녀는 싸우는 중이 아닙니다." {
		t.Fatalf("status/output = %d/%q, want enemy monster refusal", status, ctx.OutputString())
	}
	object, _ := world.Object("object:apple")
	if !objectLocatedInCreatureInventory(object, "creature:merchant") {
		t.Fatalf("apple location = %+v, want merchant inventory", object.Location)
	}
}

func TestStealHandlerFailureDoesNotMoveObjectAndStartsCooldown(t *testing.T) {
	world := state.NewWorld(stealWorld(t, model.ClassThief))
	handler := NewStealHandler(world, fixedStealRoll(100))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"사과", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "실패하였습니다.\n그가 당신을 공격합니다.") {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	object, _ := world.Object("object:apple")
	if !objectLocatedInCreatureInventory(object, "creature:merchant") {
		t.Fatalf("apple location = %+v, want merchant inventory", object.Location)
	}

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{Args: []string{"사과", "상인"}})
	if err != nil {
		t.Fatalf("handler() cooldown error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "초동안 기다리세요.") {
		t.Fatalf("cooldown status/output = %d/%q", status, ctx.OutputString())
	}
}

func TestStealHandlerDoesNotRevealInvisibleActorDuringCooldown(t *testing.T) {
	world := state.NewWorld(stealWorld(t, model.ClassThief))
	handler := NewStealHandler(world, fixedStealRoll(100))

	if _, err := handler(&Context{ActorID: "player:alice"}, ResolvedCommand{Args: []string{"사과", "상인"}}); err != nil {
		t.Fatalf("handler() first error = %v", err)
	}
	if _, err := world.UpdateCreatureTags("creature:alice", []string{"invisible", "PINVIS"}, nil); err != nil {
		t.Fatalf("UpdateCreatureTags() error = %v", err)
	}
	if err := world.SetCreatureStat("creature:alice", "PINVIS", 1); err != nil {
		t.Fatalf("SetCreatureStat(PINVIS) error = %v", err)
	}
	if _, err := world.UpdatePlayerTags("player:alice", []string{"invisible", "PINVIS"}, nil); err != nil {
		t.Fatalf("UpdatePlayerTags() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"사과", "상인"}})
	if err != nil {
		t.Fatalf("handler() cooldown error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "초동안 기다리세요.") {
		t.Fatalf("status/output = %d/%q, want cooldown", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if !creatureHasAnyFlag(alice, "invisible", "PINVIS") || alice.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want invisible retained during cooldown", alice.Metadata.Tags, alice.Stats)
	}
	player, _ := world.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "PINVIS") {
		t.Fatalf("player tags = %+v, want invisible retained during cooldown", player.Metadata.Tags)
	}
}

func TestStealHandlerRevealsInvisibleActorAfterCooldownPasses(t *testing.T) {
	loaded := stealWorld(t, model.ClassThief)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"invisible", "PINVIS"}
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	handler := NewStealHandler(world, fixedStealRoll(100))

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{Args: []string{"사과", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "당신의 모습이 서서히 드러납니다.") {
		t.Fatalf("status/output = %d/%q, want reveal message", status, ctx.OutputString())
	}
	alice, _ = world.Creature("creature:alice")
	if creatureHasAnyFlag(alice, "invisible", "PINVIS") || alice.Stats["PINVIS"] != 0 {
		t.Fatalf("alice tags/stats = %+v/%+v, want invisible cleared", alice.Metadata.Tags, alice.Stats)
	}
	player, _ = world.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "PINVIS") {
		t.Fatalf("player tags = %+v, want invisible cleared", player.Metadata.Tags)
	}
	if len(broadcasts) == 0 || !strings.Contains(broadcasts[0].Text, "Alice의 모습이 서서히 드러납니다.") {
		t.Fatalf("broadcasts = %+v, want reveal broadcast", broadcasts)
	}
}

func TestStealHandlerClearsHiddenBeforeTargetLookupLikeLegacy(t *testing.T) {
	loaded := stealWorld(t, model.ClassThief)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN"}
	alice.Stats["PHIDDN"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	handler := NewStealHandler(world, fixedStealRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"사과", "없는"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그런건 여기 없습니다." {
		t.Fatalf("status/output = %d/%q, want missing target", status, ctx.OutputString())
	}
	alice, _ = world.Creature("creature:alice")
	if hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "phiddn") || alice.Stats["PHIDDN"] != 0 {
		t.Fatalf("creature hidden state = tags:%+v stats:%+v", alice.Metadata.Tags, alice.Stats)
	}
	player, _ = world.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("player hidden tags = %+v", player.Metadata.Tags)
	}
}

func TestStealHandlerFailureNotifiesPlayerVictimDirectly(t *testing.T) {
	loaded := stealWorld(t, model.ClassThief)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"chaos"}
	loaded.Creatures[alice.ID] = alice
	bob := loaded.Creatures["creature:bob"]
	bob.Metadata.Tags = []string{"chaos"}
	loaded.Creatures[bob.ID] = bob
	world := state.NewWorld(loaded)
	handler := NewStealHandler(world, fixedStealRoll(100))

	var sent []session.Command
	ctx := &Context{
		ActorID:   "player:alice",
		SessionID: "session:alice",
		Values: map[string]any{
			"game.activeSessions": func() []struct {
				ID      session.ID
				ActorID string
			} {
				return []struct {
					ID      session.ID
					ActorID string
				}{
					{ID: "session:alice", ActorID: "player:alice"},
					{ID: "session:bob", ActorID: "player:bob"},
				}
			},
			"game.sendToSession": func(id session.ID, cmd session.Command) error {
				if id != "session:bob" {
					t.Fatalf("sendToSession id = %q, want session:bob", id)
				}
				sent = append(sent, cmd)
				return nil
			},
		},
	}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"목걸이", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "실패하였습니다.") {
		t.Fatalf("status/output = %d/%q, want failure", status, ctx.OutputString())
	}
	if len(sent) != 1 || !strings.Contains(sent[0].Write, "Alice이 당신에게서 목걸이를 훔치려고 합니다.") {
		t.Fatalf("sent = %+v, want direct steal notification", sent)
	}
}

func TestStealHandlerSuccessfulPlayerVictimStartsPlayerKillPenalty(t *testing.T) {
	loaded := stealWorld(t, model.ClassThief)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"chaos"}
	loaded.Creatures[alice.ID] = alice
	bob := loaded.Creatures["creature:bob"]
	bob.Metadata.Tags = []string{"chaos"}
	loaded.Creatures[bob.ID] = bob
	world := state.NewWorld(loaded)
	handler := NewStealHandler(world, fixedStealRoll(1))

	before := time.Now().Unix()
	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"목걸이", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "훔쳤습니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	remaining, used, err := world.UseCreatureCooldown("creature:alice", stealPlayerKillCooldownKey, before, 1)
	if err != nil {
		t.Fatalf("UseCreatureCooldown() penalty check error = %v", err)
	}
	if used {
		t.Fatalf("penalty cooldown was not active")
	}
	if remaining < stealPlayerKillPenaltyMinSec || remaining > stealPlayerKillPenaltyMaxSec+1 {
		t.Fatalf("penalty remaining = %d, want 7-10 days", remaining)
	}
}

func stealDispatcher(t *testing.T, world *state.World, roll StealRollFunc) Dispatcher {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "훔쳐", Number: 36, Handler: "steal"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return Dispatcher{Registry: registry, Handlers: map[string]Handler{"steal": NewStealHandler(world, roll)}}
}

func stealWorld(t *testing.T, class int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{ID: "room:steal", DisplayName: "Steal"})
	mustAddLookPlayer(t, loaded, model.Player{ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:steal"})
	mustAddLookPlayer(t, loaded, model.Player{ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob", RoomID: "room:steal"})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:steal",
		Stats:       map[string]int{"class": class, "level": 20, "dexterity": 40, "gold": 100},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:steal",
		Stats:       map[string]int{"level": 1},
		Inventory:   model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:apple", "object:money"}},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:steal",
		Stats:       map[string]int{"class": model.ClassFighter, "level": 1},
		Inventory:   model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:necklace"}},
	})
	for _, proto := range []model.ObjectPrototype{
		{ID: "prototype:apple", DisplayName: "사과"},
		{ID: "prototype:money", Kind: model.ObjectKindMoney, DisplayName: "돈"},
		{ID: "prototype:necklace", DisplayName: "목걸이"},
	} {
		mustAddLookPrototype(t, loaded, proto)
	}
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:apple",
		PrototypeID: "prototype:apple",
		Location:    model.ObjectLocation{CreatureID: "creature:merchant", Slot: "inventory"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:money",
		PrototypeID: "prototype:money",
		Location:    model.ObjectLocation{CreatureID: "creature:merchant", Slot: "inventory"},
		Properties: map[string]string{
			"kind":  string(model.ObjectKindMoney),
			"type":  "10",
			"value": "120",
		},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:necklace",
		PrototypeID: "prototype:necklace",
		Location:    model.ObjectLocation{CreatureID: "creature:bob", Slot: "inventory"},
	})
	return loaded
}

func fixedStealRoll(value int) StealRollFunc {
	return func(int, int) int {
		return value
	}
}

type stealSaveTrackingWorld struct {
	*state.World
	dirty  []model.PlayerID
	queued []model.PlayerID
}

func (w *stealSaveTrackingWorld) MarkPlayerDirty(playerID model.PlayerID) {
	w.dirty = append(w.dirty, playerID)
}

func (w *stealSaveTrackingWorld) QueueSave(playerID model.PlayerID, _ model.BankID) {
	w.queued = append(w.queued, playerID)
}

func assertStealPlayerIDs(t *testing.T, label string, got []model.PlayerID, want []model.PlayerID) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s players = %+v, want %+v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s players = %+v, want %+v", label, got, want)
		}
	}
}
