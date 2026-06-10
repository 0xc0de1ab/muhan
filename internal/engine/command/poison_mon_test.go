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

func TestPoisonMonHandlerPoisonsMonsterDamagesStartsCooldownAndRevealsActor(t *testing.T) {
	withAttackRolls(t, 1, 20, 5)
	loaded := poisonMonWorld(t, model.ClassInvincible, true)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS")
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := poisonMonDispatcher(t, world)
	var broadcasts []roomBroadcastRecord

	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := dispatcher.DispatchLine(ctx, "고블린 독살포")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "당신의 모습이 나타나기 시작합니다.") ||
		!strings.Contains(out, "고블린의 몸이 중독되었습니다.") ||
		!strings.Contains(out, "5의 피해를 입혔습니다.") {
		t.Fatalf("output = %q, want reveal, poison, and damage", out)
	}
	if len(broadcasts) != 3 ||
		!strings.Contains(broadcasts[0].Text, "모습이 보이기 시작합니다") ||
		!strings.Contains(broadcasts[1].Text, "몸이 중독되었습니다") ||
		!strings.Contains(broadcasts[2].Text, "피해를 입혔습니다") {
		t.Fatalf("broadcasts = %+v, want reveal, poison, and damage broadcasts", broadcasts)
	}

	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := creatureStat(goblin, "hpCurrent"), 35; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	for _, tag := range []string{"poison", "MPOISN", "befuddled", "MBEFUD"} {
		if !hasAnyNormalizedFlag(goblin.Metadata.Tags, tag) {
			t.Fatalf("goblin tags = %+v, want %q", goblin.Metadata.Tags, tag)
		}
	}
	alice, _ = world.Creature("creature:alice")
	if hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 0 || alice.Stats["PINVIS"] != 0 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible cleared", alice.Metadata.Tags, alice.Stats)
	}
	player, _ = world.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS") {
		t.Fatalf("player tags = %+v, want hidden/invisible cleared", player.Metadata.Tags)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", poisonMonCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active cooldown", used, remaining)
	}

	ctx = &Context{ActorID: "player:alice"}
	status, err = dispatcher.DispatchLine(ctx, "고블린 독살포")
	if err != nil {
		t.Fatalf("DispatchLine() cooldown error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "기다리세요") {
		t.Fatalf("cooldown status/output = %d/%q, want wait message", status, ctx.OutputString())
	}
}

func TestPoisonMonHandlerFailureUsesCooldownWithoutPoisonOrDamage(t *testing.T) {
	withAttackRolls(t, 100)
	world := state.NewWorld(poisonMonWorld(t, model.ClassInvincible, true))
	defer world.Close()
	handler := NewPoisonMonHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "살짝 피해 실패했습니다.") {
		t.Fatalf("status/output = %d/%q, want poison failure", status, ctx.OutputString())
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := creatureStat(goblin, "hpCurrent"), 40; got != want {
		t.Fatalf("goblin hp = %d, want unchanged %d", got, want)
	}
	if hasAnyNormalizedFlag(goblin.Metadata.Tags, "poison", "MPOISN", "befuddled", "MBEFUD") {
		t.Fatalf("goblin tags = %+v, want no poison/befuddle", goblin.Metadata.Tags)
	}
}

func TestPoisonMonHandlerFinalizesMonsterDeath(t *testing.T) {
	withAttackRolls(t, 1, 20, 80)
	loaded := poisonMonWorld(t, 80, true)
	goblin := loaded.Creatures["creature:goblin-1"]
	goblin.Stats["hpCurrent"] = 40
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	defer world.Close()
	handler := NewPoisonMonHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "고블린을 죽였습니다.") {
		t.Fatalf("status/output = %d/%q, want death message", status, ctx.OutputString())
	}
	if _, ok := world.Creature("creature:goblin-1"); ok {
		t.Fatal("dead poison target still exists in world")
	}
	room, _ := world.Room("room:arena")
	if containsCreatureID(room.CreatureIDs, "creature:goblin-1") {
		t.Fatalf("room creatures = %+v, want goblin removed", room.CreatureIDs)
	}
}

func TestPoisonMonHandlerUsesCustomDeathFinalizer(t *testing.T) {
	withAttackRolls(t, 1, 20, 80)
	loaded := poisonMonWorld(t, 80, true)
	goblin := loaded.Creatures["creature:goblin-1"]
	goblin.Stats["hpCurrent"] = 40
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	defer world.Close()
	called := false
	handler := NewPoisonMonHandlerWithDeathFinalizer(world, func(_ *Context, attacker model.Creature, victim model.Creature) error {
		called = true
		if attacker.ID != "creature:alice" || victim.ID != "creature:goblin-1" {
			t.Fatalf("finalizer attacker/victim = %q/%q, want alice/goblin", attacker.ID, victim.ID)
		}
		_, err := world.FinalizeMonsterDeath(victim.ID)
		return err
	})

	ctx := &Context{ActorID: "player:alice"}
	if _, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if !called {
		t.Fatal("custom finalizer was not called")
	}
	if _, ok := world.Creature("creature:goblin-1"); ok {
		t.Fatal("dead poison target still exists in world")
	}
}

func TestPoisonMonHandlerRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name    string
		class   int
		trained bool
		args    []string
		mutate  func(*worldload.World)
		want    string
	}{
		{name: "missing target", class: model.ClassInvincible, trained: true, want: "누구를 중독시키시려고요?"},
		{name: "low class", class: model.ClassAssassin, trained: true, args: []string{"고블린"}, want: "무적이상만 쓸 수 있는 기술입니다."},
		{name: "untrained invincible", class: model.ClassInvincible, args: []string{"고블린"}, want: "아직 자객을무적수련하지 않았습니다."},
		{name: "missing monster", class: model.ClassInvincible, trained: true, args: []string{"없는"}, want: "그런 괴물은 존재하지 않습니다."},
		{name: "player target is not valid", class: model.ClassInvincible, trained: true, args: []string{"Bob"}, want: "그런 괴물은 존재하지 않습니다."},
		{
			name:    "protected male monster",
			class:   model.ClassInvincible,
			trained: true,
			args:    []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				goblin := loaded.Creatures["creature:goblin-1"]
				goblin.Metadata.Tags = []string{"MUNKIL", "MMALES"}
				loaded.Creatures[goblin.ID] = goblin
			},
			want: "당신은 그를 중독시킬수 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := poisonMonWorld(t, tt.class, tt.trained)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
	defer world.Close()
			handler := NewPoisonMonHandler(world)

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

func TestPoisonMonHandlerCanBeRegisteredByDispatcher(t *testing.T) {
	for _, line := range []string{"고블린 독살포", "poison_mon 고블린"} {
		t.Run(line, func(t *testing.T) {
			withAttackRolls(t, 1, 100)
			world := state.NewWorld(poisonMonWorld(t, model.ClassInvincible, true))
	defer world.Close()
			dispatcher := poisonMonDispatcher(t, world)

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, line)
			if err != nil {
				t.Fatalf("DispatchLine(%q) error = %v", line, err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), "몸이 중독되었습니다") {
				t.Fatalf("status/output = %d/%q, want dispatch success", status, ctx.OutputString())
			}
		})
	}
}

func TestPoisonMonLegacyFormulaHelpers(t *testing.T) {
	actor := model.Creature{Stats: map[string]int{"class": model.ClassInvincible, "level": 20, "dexterity": 20}}
	target := model.Creature{Stats: map[string]int{"level": 1, "hpCurrent": 80}}
	if got, want := poisonMonChance(actor, target), 80; got != want {
		t.Fatalf("poisonMonChance() = %d, want capped %d", got, want)
	}
	if got, want := poisonMonCooldownSeconds(actor), int64(20); got != want {
		t.Fatalf("poisonMonCooldownSeconds(invincible) = %d, want %d", got, want)
	}
	actor.Stats["class"] = model.ClassCaretaker
	if got, want := poisonMonCooldownSeconds(actor), int64(18); got != want {
		t.Fatalf("poisonMonCooldownSeconds(caretaker) = %d, want %d", got, want)
	}
	actor.Stats["class"] = model.ClassBulsa
	if got, want := poisonMonCooldownSeconds(actor), int64(16); got != want {
		t.Fatalf("poisonMonCooldownSeconds(bulsa) = %d, want %d", got, want)
	}
}

func poisonMonDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "독살포", Number: 98, Handler: "poison_mon"},
		{Name: "poison_mon", Number: 98, Handler: "poison_mon"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"poison_mon": NewPoisonMonHandler(world),
		},
	}
}

func poisonMonWorld(t *testing.T, class int, trained bool) *worldload.World {
	t.Helper()
	loaded := kickWorld(t, class)
	alice := loaded.Creatures["creature:alice"]
	if trained {
		alice.Metadata.Tags = append(alice.Metadata.Tags, "SASSASSIN")
	}
	loaded.Creatures[alice.ID] = alice
	return loaded
}
