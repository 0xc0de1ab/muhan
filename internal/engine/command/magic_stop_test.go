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

func TestMagicStopHandlerBlocksSpellDamageAndStartsCooldowns(t *testing.T) {
	loaded := magicStopWorld(t, legacyClassInvincible)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SRANGER", "hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	handler := NewMagicStopHandler(world, magicStopRolls(t, 1, 20, 5, 1, 1))

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "당신의 모습이 나타나기 시작합니다.") ||
		!strings.Contains(out, "적의 혈도를 짚어 주문을 봉쇄했습니다.") ||
		!strings.Contains(out, "25의 피해를 입혔습니다.") {
		t.Fatalf("output = %q, want reveal, spell block, and damage", out)
	}

	goblin, _ := world.Creature("creature:goblin")
	if got := creatureStat(goblin, "hpCurrent"); got != 75 {
		t.Fatalf("goblin hp = %d, want 75", got)
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
	if len(broadcasts) < 2 || !strings.Contains(broadcasts[1].Text, "주문이 봉쇄되었습니다") {
		t.Fatalf("broadcasts = %+v, want spell block broadcast", broadcasts)
	}

	now := time.Now().Unix()
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", magicStopCooldownKey, now, 1); err != nil {
		t.Fatalf("actor cooldown check error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("actor cooldown used/remaining = %v/%d, want active", used, remaining)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:goblin", magicStopSpellCooldownKey, now, 1); err != nil {
		t.Fatalf("spell cooldown check error = %v", err)
	} else if used || remaining <= 0 || remaining > 15 {
		t.Fatalf("spell cooldown used/remaining = %v/%d, want active <= 15s", used, remaining)
	}
}

func TestMagicStopTargetResistanceIgnoresLearnedSpellFlag(t *testing.T) {
	target := model.Creature{Metadata: model.Metadata{Tags: []string{"SRMAGI"}}}
	if magicStopTargetResistsSpellBlock(target) {
		t.Fatalf("magicStopTargetResistsSpellBlock(%+v) = true, want false", target.Metadata.Tags)
	}

	target.Metadata.Tags = []string{"PRMAGI"}
	if !magicStopTargetResistsSpellBlock(target) {
		t.Fatalf("magicStopTargetResistsSpellBlock(%+v) = false, want true", target.Metadata.Tags)
	}
}

func TestMagicStopHandlerRejectsInvalidStates(t *testing.T) {
	tests := []struct {
		name   string
		class  int
		tags   []string
		args   []string
		mutate func(*worldload.World)
		want   string
	}{
		{
			name:  "wrong class",
			class: legacyClassRanger,
			tags:  []string{"SRANGER"},
			args:  []string{"고블린"},
			want:  "무적이상만 쓸 수 있는 기술입니다.",
		},
		{
			name:  "invincible without ranger training",
			class: legacyClassInvincible,
			args:  []string{"고블린"},
			want:  "포졸을 무적수련하지 않았습니다.",
		},
		{
			name:  "missing target",
			class: legacyClassInvincible,
			tags:  []string{"SRANGER"},
			want:  "누구의 혈도를 봉쇄하실려구요?",
		},
		{
			name:  "blind",
			class: legacyClassInvincible,
			tags:  []string{"SRANGER", "PBLIND"},
			args:  []string{"고블린"},
			want:  "누구의 혈도를 봉쇄하실려구요?",
		},
		{
			name:  "unknown target",
			class: legacyClassInvincible,
			tags:  []string{"SRANGER"},
			args:  []string{"없는"},
			want:  "그런 괴물은 존재하지 않습니다.",
		},
		{
			name:  "protected monster",
			class: legacyClassInvincible,
			tags:  []string{"SRANGER"},
			args:  []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				goblin := loaded.Creatures["creature:goblin"]
				goblin.Metadata.Tags = []string{"MUNKIL", "MMALES"}
				loaded.Creatures[goblin.ID] = goblin
			},
			want: "당신은 그의 혈도를 봉쇄할수 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := magicStopWorld(t, tt.class)
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = tt.tags
			loaded.Creatures[alice.ID] = alice
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
			handler := NewMagicStopHandler(world, fixedRoll(1))

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

func TestMagicStopHandlerAllowsStatBasedRangerTraining(t *testing.T) {
	loaded := magicStopWorld(t, legacyClassInvincible)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = nil
	alice.Stats["SRANGER"] = 1
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	handler := NewMagicStopHandler(world, magicStopRolls(t, 1, 100, 1, 1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "주문을 봉쇄했습니다") {
		t.Fatalf("status/output = %d/%q, want trained success", status, ctx.OutputString())
	}
}

func TestMagicStopHandlerUsesCustomDeathFinalizer(t *testing.T) {
	loaded := magicStopWorld(t, 80)
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Stats["hpCurrent"] = 25
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	called := false
	handler := NewMagicStopHandlerWithDeathFinalizer(world, magicStopRolls(t, 1, 20, 25, 1, 1), func(_ *Context, attacker model.Creature, victim model.Creature) error {
		called = true
		if attacker.ID != "creature:alice" || victim.ID != "creature:goblin" {
			t.Fatalf("finalizer attacker/victim = %q/%q, want alice/goblin", attacker.ID, victim.ID)
		}
		_, err := world.FinalizeMonsterDeath(victim.ID)
		return err
	})

	ctx := &Context{ActorID: "player:alice"}
	if _, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}, Values: []int64{1}}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if !called {
		t.Fatal("custom finalizer was not called")
	}
	if _, ok := world.Creature("creature:goblin"); ok {
		t.Fatal("dead magic_stop target still exists in world")
	}
}

func TestMagicStopHandlerFailureUsesCooldownWithoutDamage(t *testing.T) {
	world := state.NewWorld(magicStopWorld(t, legacyClassInvincible))
	handler := NewMagicStopHandler(world, fixedRoll(100))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "빗나갔습니다") {
		t.Fatalf("status/output = %d/%q, want miss", status, ctx.OutputString())
	}
	goblin, _ := world.Creature("creature:goblin")
	if got := creatureStat(goblin, "hpCurrent"); got != 100 {
		t.Fatalf("goblin hp = %d, want unchanged 100", got)
	}

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{Args: []string{"고블린"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() cooldown error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "기다리세요") {
		t.Fatalf("cooldown status/output = %d/%q, want wait message", status, ctx.OutputString())
	}
}

func TestMagicStopCooldownAndDurationHelpers(t *testing.T) {
	if got := magicStopCooldownSeconds(model.Creature{Stats: map[string]int{"class": legacyClassInvincible}}); got != 20 {
		t.Fatalf("invincible cooldown = %d, want 20", got)
	}
	if got := magicStopCooldownSeconds(model.Creature{Stats: map[string]int{"class": legacyClassCaretaker}}); got != 18 {
		t.Fatalf("caretaker cooldown = %d, want 18", got)
	}
	if got := magicStopCooldownSeconds(model.Creature{Stats: map[string]int{"class": legacyClassBulsa}}); got != 16 {
		t.Fatalf("bulsa cooldown = %d, want 16", got)
	}
	if got := magicStopSpellCooldownSeconds(
		model.Creature{Stats: map[string]int{"dexterity": 20}},
		model.Creature{Metadata: model.Metadata{Tags: []string{"MRMAGI"}}},
		magicStopRolls(t),
	); got != 5 {
		t.Fatalf("resistant spell cooldown = %d, want 5", got)
	}
	if got := magicStopSpellCooldownSeconds(
		model.Creature{Stats: map[string]int{"dexterity": 20}},
		model.Creature{},
		magicStopRolls(t, 1, 1),
	); got != 15 {
		t.Fatalf("normal spell cooldown = %d, want floor 15", got)
	}
}

func TestMagicStopHandlerCanBeRegisteredByDispatcher(t *testing.T) {
	for _, line := range []string{"고블린 혈도봉쇄", "magic_stop 고블린"} {
		t.Run(line, func(t *testing.T) {
			world := state.NewWorld(magicStopWorld(t, legacyClassInvincible))
			dispatcher := magicStopDispatcher(t, world, magicStopRolls(t, 1, 100, 1, 1))

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, line)
			if err != nil {
				t.Fatalf("DispatchLine(%q) error = %v", line, err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), "주문을 봉쇄했습니다") {
				t.Fatalf("status/output = %d/%q, want dispatch success", status, ctx.OutputString())
			}
		})
	}
}

func magicStopDispatcher(t *testing.T, world *state.World, roll SearchRollFunc) Dispatcher {
	t.Helper()
	return Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "혈도봉쇄", Number: 91, Handler: "magic_stop"},
			{Name: "magic_stop", Number: 91, Handler: "magic_stop"},
		}),
		Handlers: map[string]Handler{
			"magic_stop": NewMagicStopHandler(world, roll),
		},
	}
}

func magicStopWorld(t *testing.T, class int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:arena",
		DisplayName: "Arena",
		PlayerIDs:   []model.PlayerID{"player:alice"},
		CreatureIDs: []model.CreatureID{"creature:goblin"},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:arena",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:arena",
		Level:       40,
		Stats: map[string]int{
			"class":     class,
			"level":     40,
			"dexterity": 20,
			"hpCurrent": 50,
			"hpMax":     50,
		},
		Metadata: model.Metadata{Tags: []string{"SRANGER"}},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:goblin",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:arena",
		Level:       4,
		Stats:       map[string]int{"level": 4, "hpCurrent": 100, "hpMax": 100},
	})
	return loaded
}

func magicStopRolls(t *testing.T, rolls ...int) SearchRollFunc {
	t.Helper()
	index := 0
	return func(min int, max int) int {
		if index >= len(rolls) {
			t.Fatalf("magic stop roll(%d, %d) called after %d scripted rolls", min, max, len(rolls))
		}
		value := rolls[index]
		index++
		if value < min || value > max {
			t.Fatalf("scripted magic stop roll %d for range [%d,%d]", value, min, max)
		}
		return value
	}
}
