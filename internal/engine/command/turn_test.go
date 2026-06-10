package command

import (
	"strings"
	"testing"
	"time"

	"muhan/internal/commandspec"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestTurnHandlerDamagesUndeadTargetAndStartsCooldown(t *testing.T) {
	world := state.NewWorld(turnWorld(t, model.ClassCleric))
	dispatcher := turnDispatcher(t, world, fixedRoll(1))
	var broadcasts []roomBroadcastRecord

	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := dispatcher.DispatchLine(ctx, "해골 2 방혼술")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "4만큼의 타격") {
		t.Fatalf("status/output = %d/%q, want turn damage", status, ctx.OutputString())
	}

	first, _ := world.Creature("creature:skeleton-1")
	second, _ := world.Creature("creature:skeleton-2")
	if got := creatureStat(first, "hpCurrent"); got != 12 {
		t.Fatalf("first skeleton hp = %d, want untouched 12", got)
	}
	if got := creatureStat(second, "hpCurrent"); got != 8 {
		t.Fatalf("second skeleton hp = %d, want 8", got)
	}
	if len(broadcasts) != 1 || !strings.Contains(broadcasts[0].Text, "방혼술의 주문을 외칩니다") {
		t.Fatalf("broadcasts = %+v, want turn broadcast", broadcasts)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", turnCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active cooldown", used, remaining)
	}
}

func TestTurnHandlerFinalizesMonsterDeath(t *testing.T) {
	loaded := turnWorld(t, model.ClassCleric)
	mouse := loaded.Creatures["creature:mouse"]
	mouse.Metadata.Tags = []string{"turnable"}
	loaded.Creatures[mouse.ID] = mouse
	world := state.NewWorld(loaded)
	handler := NewTurnHandler(world, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"생쥐"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "혼을 소멸시켰습니다") {
		t.Fatalf("status/output = %d/%q, want death message", status, ctx.OutputString())
	}
	if _, ok := world.Creature("creature:mouse"); ok {
		t.Fatal("dead turn target still exists in world")
	}
	room, _ := world.Room("room:crypt")
	if containsCreatureID(room.CreatureIDs, "creature:mouse") {
		t.Fatalf("room creatures = %+v, want mouse removed", room.CreatureIDs)
	}
}

func TestTurnHandlerUsesCustomDeathFinalizer(t *testing.T) {
	loaded := turnWorld(t, model.ClassCleric)
	mouse := loaded.Creatures["creature:mouse"]
	mouse.Metadata.Tags = []string{"turnable"}
	loaded.Creatures[mouse.ID] = mouse
	world := state.NewWorld(loaded)
	called := false
	handler := NewTurnHandlerWithDeathFinalizer(world, fixedRoll(1), func(_ *Context, attacker model.Creature, victim model.Creature) error {
		called = true
		if attacker.ID != "creature:alice" || victim.ID != "creature:mouse" {
			t.Fatalf("finalizer attacker/victim = %q/%q, want alice/mouse", attacker.ID, victim.ID)
		}
		_, err := world.FinalizeMonsterDeath(victim.ID)
		return err
	})

	ctx := &Context{ActorID: "player:alice"}
	if _, err := handler(ctx, ResolvedCommand{Args: []string{"생쥐"}, Values: []int64{1}}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if !called {
		t.Fatal("custom finalizer was not called")
	}
	if _, ok := world.Creature("creature:mouse"); ok {
		t.Fatal("dead turn target still exists in world")
	}
}

func TestTurnHandlerRejectsInvalidStates(t *testing.T) {
	tests := []struct {
		name   string
		class  int
		args   []string
		mutate func(*worldload.World)
		want   string
	}{
		{
			name:  "missing target",
			class: model.ClassCleric,
			want:  "누구에게 주문을 거실려고요?",
		},
		{
			name:  "wrong class",
			class: model.ClassFighter,
			args:  []string{"해골"},
			want:  "불제자와 무사만이 방혼술을 사용할 수 있습니다.",
		},
		{
			name:  "invincible without cleric or paladin training",
			class: model.ClassInvincible,
			args:  []string{"해골"},
			want:  "불제자나 무사를 무적수련하지 않았습니다.",
		},
		{
			name:  "missing monster",
			class: model.ClassCleric,
			args:  []string{"없는"},
			want:  "그런 괴물은 존재하지 않습니다.",
		},
		{
			name:  "paladin living monster",
			class: model.ClassPaladin,
			args:  []string{"고블린"},
			want:  "죽은 괴물에게만 사용가능합니다.",
		},
		{
			name:  "protected undead",
			class: model.ClassCleric,
			args:  []string{"해골"},
			mutate: func(loaded *worldload.World) {
				skeleton := loaded.Creatures["creature:skeleton-1"]
				skeleton.Metadata.Tags = append(skeleton.Metadata.Tags, "MUNKIL", "MMALES")
				loaded.Creatures[skeleton.ID] = skeleton
			},
			want: "당신은 그의 혼을 소멸시킬 수 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := turnWorld(t, tt.class)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
			handler := NewTurnHandler(world, fixedRoll(1))

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{Args: tt.args, Values: []int64{1}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestTurnHandlerClericCanAffectLivingMonsterLikeLegacy(t *testing.T) {
	world := state.NewWorld(turnWorld(t, model.ClassCleric))
	handler := NewTurnHandler(world, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "타격을 입혔습니다") {
		t.Fatalf("status/output = %d/%q, want C cleric living-target damage", status, ctx.OutputString())
	}
	goblin, _ := world.Creature("creature:goblin")
	if got := creatureStat(goblin, "hpCurrent"); got != 8 {
		t.Fatalf("goblin hp = %d, want 8", got)
	}
}

func TestTurnHandlerRevealsPinvisBeforeCooldownAndKeepsHiddenLikeLegacy(t *testing.T) {
	loaded := turnWorld(t, model.ClassCleric)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	if err := world.SetCreatureCooldown("creature:alice", turnCooldownKey, time.Now().Unix(), 5); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}
	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)

	status, err := NewTurnHandler(world, fixedRoll(1))(ctx, ResolvedCommand{Args: []string{"해골"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	output := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(output, "당신의 모습이 나타나기 시작합니다.") || !strings.Contains(output, "기다리세요") {
		t.Fatalf("status/output = %d/%q, want reveal before cooldown wait", status, output)
	}
	if len(broadcasts) != 1 || broadcasts[0].Text != "Alice의 모습이 보이기 시작합니다." {
		t.Fatalf("broadcasts = %+v, want C PINVIS reveal broadcast", broadcasts)
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN") ||
		hasAnyNormalizedFlag(alice.Metadata.Tags, "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 1 || alice.Stats["PINVIS"] != 0 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden retained and PINVIS cleared", alice.Metadata.Tags, alice.Stats)
	}
	player, _ = world.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "PHIDDN") ||
		hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "PINVIS") {
		t.Fatalf("player tags = %+v, want hidden retained and PINVIS cleared", player.Metadata.Tags)
	}
	skeleton, _ := world.Creature("creature:skeleton-1")
	if hasAnyNormalizedFlag(skeleton.Metadata.Tags, "was_attacked") {
		t.Fatalf("skeleton tags = %+v, want no enemy marking before cooldown clears", skeleton.Metadata.Tags)
	}
}

func TestTurnHandlerInstantDisintegratesUndeadLikeLegacy(t *testing.T) {
	world := state.NewWorld(turnWorld(t, model.ClassCleric))
	handler := NewTurnHandler(world, turnRolls(t, 1, 100))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"해골"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "저승사자가") {
		t.Fatalf("status/output = %d/%q, want C instant disintegration", status, ctx.OutputString())
	}
	if _, ok := world.Creature("creature:skeleton-1"); ok {
		t.Fatal("instant-disintegrated skeleton still exists")
	}
}

func TestTurnHandlerInvincibleDamageUsesLegacyClassFormula(t *testing.T) {
	loaded := turnWorld(t, model.ClassInvincible)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SCLERIC"}
	loaded.Creatures[alice.ID] = alice
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Stats["hpMax"] = 90
	goblin.Stats["hpCurrent"] = 90
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	handler := NewTurnHandler(world, turnRolls(t, 1, 5))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "15만큼의 타격") {
		t.Fatalf("status/output = %d/%q, want C invincible damage", status, ctx.OutputString())
	}
	goblin, _ = world.Creature("creature:goblin")
	if got := creatureStat(goblin, "hpCurrent"); got != 75 {
		t.Fatalf("goblin hp = %d, want 75", got)
	}
}

func TestTurnHandlerCaretakerDamageUsesLegacyClassFormula(t *testing.T) {
	loaded := turnWorld(t, model.ClassCaretaker)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SCLERIC"}
	loaded.Creatures[alice.ID] = alice
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Stats["hpMax"] = 90
	goblin.Stats["hpCurrent"] = 90
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	handler := NewTurnHandler(world, turnRolls(t, 1, 3))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "9만큼의 타격") {
		t.Fatalf("status/output = %d/%q, want C caretaker damage", status, ctx.OutputString())
	}
	goblin, _ = world.Creature("creature:goblin")
	if got := creatureStat(goblin, "hpCurrent"); got != 81 {
		t.Fatalf("goblin hp = %d, want 81", got)
	}
}

func TestTurnHandlerBulsaDamageKeepsLegacyDanglingElseResult(t *testing.T) {
	loaded := turnWorld(t, model.ClassBulsa)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SCLERIC"}
	loaded.Creatures[alice.ID] = alice
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Stats["hpMax"] = 90
	goblin.Stats["hpCurrent"] = 90
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	handler := NewTurnHandler(world, turnRolls(t, 1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "30만큼의 타격") {
		t.Fatalf("status/output = %d/%q, want C BULSA hp/3 damage", status, ctx.OutputString())
	}
	goblin, _ = world.Creature("creature:goblin")
	if got := creatureStat(goblin, "hpCurrent"); got != 60 {
		t.Fatalf("goblin hp = %d, want 60", got)
	}
}

func TestTurnHandlerAllowsTrainedInvincibleAndTurnableAlias(t *testing.T) {
	loaded := turnWorld(t, model.ClassInvincible)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SCLERIC"}
	loaded.Creatures[alice.ID] = alice
	skeleton := loaded.Creatures["creature:skeleton-1"]
	skeleton.Metadata.Tags = []string{"turnable"}
	loaded.Creatures[skeleton.ID] = skeleton
	world := state.NewWorld(loaded)
	handler := NewTurnHandler(world, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"해골"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "타격을 입혔습니다") {
		t.Fatalf("status/output = %d/%q, want trained invincible success", status, ctx.OutputString())
	}
}

func TestTurnHandlerFailureUsesCooldownWithoutDamage(t *testing.T) {
	world := state.NewWorld(turnWorld(t, model.ClassPaladin))
	handler := NewTurnHandler(world, fixedRoll(100))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"해골"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "주술을 견뎌냈습니다") {
		t.Fatalf("status/output = %d/%q, want failed turn", status, ctx.OutputString())
	}
	skeleton, _ := world.Creature("creature:skeleton-1")
	if got := creatureStat(skeleton, "hpCurrent"); got != 12 {
		t.Fatalf("skeleton hp = %d, want unchanged 12", got)
	}

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{Args: []string{"해골"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() cooldown error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "기다리세요") {
		t.Fatalf("cooldown status/output = %d/%q, want wait message", status, ctx.OutputString())
	}
}

func TestTurnHandlerCanBeRegisteredByDispatcher(t *testing.T) {
	for _, line := range []string{"해골 방혼술", "turn 해골"} {
		t.Run(line, func(t *testing.T) {
			world := state.NewWorld(turnWorld(t, model.ClassCleric))
			dispatcher := turnDispatcher(t, world, fixedRoll(1))

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, line)
			if err != nil {
				t.Fatalf("DispatchLine(%q) error = %v", line, err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), "타격을 입혔습니다") {
				t.Fatalf("status/output = %d/%q, want dispatch success", status, ctx.OutputString())
			}
		})
	}
}

func turnDispatcher(t *testing.T, world *state.World, roll SearchRollFunc) Dispatcher {
	t.Helper()
	return Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "방혼술", Number: 66, Handler: "turn"},
			{Name: "turn", Number: 66, Handler: "turn"},
		}),
		Handlers: map[string]Handler{
			"turn": NewTurnHandler(world, roll),
		},
	}
}

func turnWorld(t *testing.T, class int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:crypt",
		DisplayName: "Crypt",
		PlayerIDs:   []model.PlayerID{"player:alice"},
		CreatureIDs: []model.CreatureID{
			"creature:skeleton-1",
			"creature:skeleton-2",
			"creature:goblin",
			"creature:mouse",
		},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:crypt",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:crypt",
		Level:       20,
		Stats: map[string]int{
			"class":     class,
			"level":     20,
			"piety":     30,
			"hpMax":     30,
			"hpCurrent": 30,
		},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:skeleton-1",
		Kind:        model.CreatureKindMonster,
		DisplayName: "해골",
		RoomID:      "room:crypt",
		Level:       8,
		Stats:       map[string]int{"level": 8, "hpMax": 12, "hpCurrent": 12},
		Metadata:    model.Metadata{Tags: []string{"undead"}},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:skeleton-2",
		Kind:        model.CreatureKindMonster,
		DisplayName: "해골",
		RoomID:      "room:crypt",
		Level:       8,
		Stats:       map[string]int{"level": 8, "hpMax": 12, "hpCurrent": 12},
		Metadata:    model.Metadata{Tags: []string{"undead"}},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:goblin",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:crypt",
		Level:       8,
		Stats:       map[string]int{"level": 8, "hpMax": 12, "hpCurrent": 12},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:mouse",
		Kind:        model.CreatureKindMonster,
		DisplayName: "생쥐",
		RoomID:      "room:crypt",
		Level:       1,
		Stats:       map[string]int{"level": 1, "hpMax": 1, "hpCurrent": 1},
	})
	return loaded
}

func turnRolls(t *testing.T, rolls ...int) SearchRollFunc {
	t.Helper()
	index := 0
	return func(min int, max int) int {
		if index >= len(rolls) {
			t.Fatalf("turn roll(%d, %d) called after %d scripted rolls", min, max, len(rolls))
		}
		value := rolls[index]
		index++
		if value < min || value > max {
			t.Fatalf("scripted turn roll %d for range [%d,%d]", value, min, max)
		}
		return value
	}
}
