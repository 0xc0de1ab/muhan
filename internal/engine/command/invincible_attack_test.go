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

func TestInvincibleKickHandlerHitsMonsterRevealsAndStartsCooldown(t *testing.T) {
	withAttackRolls(t, 1, 20, 1, 0, 0, 0)
	loaded := invincibleAttackWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SBARBARIAN", "hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	dispatcher := invincibleAttackDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "고블린 백보신권")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "당신의 모습이 서서히 드러납니다.") ||
		!strings.Contains(out, "백보신권으로 3연타 27점") {
		t.Fatalf("output = %q, want reveal and invincible kick damage", out)
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 13; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
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
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", invincibleKickCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active cooldown", used, remaining)
	}
}

func TestInvincibleKickHandlerRejectsInvalidInputsOutput(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		mutate func(*worldload.World)
		want   string
	}{
		{name: "missing target", want: "누굴 공격합니까?"},
		{
			name: "wrong class",
			args: []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["class"] = legacyClassFighter
				loaded.Creatures[alice.ID] = alice
			},
			want: "무적 이상만 사용할 수 있는 기술입니다.",
		},
		{
			name: "missing training",
			args: []string{"고블린"},
			want: "권법가를 무적수련하지 않았습니다..",
		},
		{
			name: "unknown target",
			args: []string{"없는"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"SBARBARIAN"}
				loaded.Creatures[alice.ID] = alice
			},
			want: "그런 것은 여기 없습니다.",
		},
		{
			name: "protected monster",
			args: []string{"수호석"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"SBARBARIAN"}
				loaded.Creatures[alice.ID] = alice
			},
			want: "당신은 그 상대를 해칠 수 없습니다.",
		},
		{
			name: "player kill gate",
			args: []string{"Bob"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"SBARBARIAN"}
				loaded.Creatures[alice.ID] = alice
			},
			want: "당신은 선해서 다른 사용자를 공격할 수 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := invincibleAttackWorld(t)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewInvincibleKickHandler(world)(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestInvincibleKickHandlerRejectsCharmedPlayerLikeLegacy(t *testing.T) {
	loaded := invincibleAttackWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SBARBARIAN", "PCHAOS", "hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	alicePlayer := loaded.Players["player:alice"]
	alicePlayer.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[alicePlayer.ID] = alicePlayer
	bob := loaded.Creatures["creature:bob"]
	bob.Metadata.Tags = []string{"PCHAOS", "PCHARM", "charm:Alice"}
	loaded.Creatures[bob.ID] = bob
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewInvincibleKickHandler(world)(ctx, ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "너무 사랑") {
		t.Fatalf("status/output = %d/%q, want legacy charm refusal", status, ctx.OutputString())
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", invincibleKickCooldownKey, time.Now().Unix(), 0); err != nil || !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining/err = %v/%d/%v, want unused", used, remaining, err)
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 1 || alice.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained", alice.Metadata.Tags, alice.Stats)
	}
}

func TestInvincibleKickHandlerCooldownPrecedesTargetAndReveal(t *testing.T) {
	loaded := invincibleAttackWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SBARBARIAN", "hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	if err := world.SetCreatureCooldown("creature:alice", invincibleKickCooldownKey, time.Now().Unix(), invincibleKickSuccessCooldown(alice)); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewInvincibleKickHandler(world)(ctx, ResolvedCommand{Args: []string{"없는"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "기다리세요.") {
		t.Fatalf("status/output = %d/%q, want cooldown wait", status, ctx.OutputString())
	}
	updated, _ := world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "PHIDDN") ||
		!hasAnyNormalizedFlag(updated.Metadata.Tags, "invisible", "PINVIS") ||
		updated.Stats["PHIDDN"] != 1 || updated.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained during cooldown", updated.Metadata.Tags, updated.Stats)
	}
}

func TestInvincibleKickHandlerFailurePrimesMonsterAndStartsShortCooldown(t *testing.T) {
	withAttackRolls(t, 100)
	loaded := invincibleAttackWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SBARBARIAN"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewInvincibleKickHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "백보신권이 실패했습니다") {
		t.Fatalf("status/output = %d/%q, want failure", status, ctx.OutputString())
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 40; got != want {
		t.Fatalf("goblin hp = %d, want unchanged %d", got, want)
	}
	if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "was_attacked") {
		t.Fatalf("goblin tags = %+v, want was_attacked after failed invincible kick", goblin.Metadata.Tags)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", invincibleKickCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active failure cooldown", used, remaining)
	}
}

func TestOneKillHandlerKillsMonsterAndStartsCooldown(t *testing.T) {
	withAttackRolls(t, 1, 1)
	loaded := invincibleAttackWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SASSASSIN", "hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	dispatcher := invincibleAttackDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "고블린 일격필살")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "일격필살로 고블린에게 40점") ||
		!strings.Contains(out, "고블린이 쓰러졌습니다.") {
		t.Fatalf("output = %q, want one kill damage and death", out)
	}
	if _, ok := world.Creature("creature:goblin"); ok {
		t.Fatal("goblin still exists, want finalized monster death")
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
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", oneKillCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active cooldown", used, remaining)
	}
}

func TestOneKillHandlerCooldownPrecedesWeaponAndReveal(t *testing.T) {
	loaded := invincibleAttackWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SASSASSIN", "hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	alice.Equipment = nil
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	if err := world.SetCreatureCooldown("creature:alice", oneKillCooldownKey, time.Now().Unix(), oneKillCooldownSeconds); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewOneKillHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "기다리세요.") {
		t.Fatalf("status/output = %d/%q, want cooldown wait", status, ctx.OutputString())
	}
	updated, _ := world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "PHIDDN") ||
		!hasAnyNormalizedFlag(updated.Metadata.Tags, "invisible", "PINVIS") ||
		updated.Stats["PHIDDN"] != 1 || updated.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained during cooldown", updated.Metadata.Tags, updated.Stats)
	}
}

func TestOneKillHandlerRejectsEnemyMonsterBeforeCooldownAndRevealLikeLegacy(t *testing.T) {
	loaded := invincibleAttackWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SASSASSIN", "hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	if _, err := world.AddEnemy("creature:goblin", "creature:alice"); err != nil {
		t.Fatalf("AddEnemy() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewOneKillHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 그녀와 싸우는 중입니다.\n" {
		t.Fatalf("status/output = %d/%q, want enemy-monster refusal", status, ctx.OutputString())
	}
	updated, _ := world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "PHIDDN") ||
		!hasAnyNormalizedFlag(updated.Metadata.Tags, "invisible", "PINVIS") ||
		updated.Stats["PHIDDN"] != 1 || updated.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained before reveal", updated.Metadata.Tags, updated.Stats)
	}
	if _, ok, err := world.CreatureCooldownExpires("creature:alice", oneKillCooldownKey); err != nil {
		t.Fatalf("CreatureCooldownExpires() error = %v", err)
	} else if ok {
		t.Fatalf("one_kill cooldown was set before enemy-monster refusal")
	}
}

func TestOneKillHandlerWeaponBreakAfterSuccessKeepsPreliminaryCooldown(t *testing.T) {
	withAttackRolls(t, 1)
	loaded := invincibleAttackWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SASSASSIN"}
	loaded.Creatures[alice.ID] = alice
	sword := loaded.Objects["object:sword"]
	if sword.Properties == nil {
		sword.Properties = map[string]string{}
	}
	sword.Properties["shotsCurrent"] = "1"
	loaded.Objects[sword.ID] = sword
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewOneKillHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "부서져 버렸습니다.") {
		t.Fatalf("status/output = %d/%q, want weapon break", status, ctx.OutputString())
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 40; got != want {
		t.Fatalf("goblin hp = %d, want unchanged after weapon break %d", got, want)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", oneKillCooldownKey, time.Now().Unix()+6, 0); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining at +6s = %v/%d, want preliminary cooldown only", used, remaining)
	}
}

func TestOneKillHandlerUsesCustomDeathFinalizer(t *testing.T) {
	withAttackRolls(t, 1, 1)
	loaded := invincibleAttackWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SASSASSIN"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)

	called := false
	finalizer := func(ctx *Context, attacker model.Creature, victim model.Creature) error {
		called = true
		if attacker.ID != "creature:alice" || victim.ID != "creature:goblin" {
			t.Fatalf("finalizer attacker/victim = %q/%q, want alice/goblin", attacker.ID, victim.ID)
		}
		return nil
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewOneKillHandlerWithDeathFinalizer(world, finalizer)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !called || !strings.Contains(ctx.OutputString(), "고블린이 쓰러졌습니다.") {
		t.Fatalf("status/called/output = %d/%v/%q, want custom finalizer death", status, called, ctx.OutputString())
	}
}

func TestOneKillHandlerFailureDamagesActor(t *testing.T) {
	withAttackRolls(t, 100)
	loaded := invincibleAttackWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SASSASSIN"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewOneKillHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "32점의 피해를 입었습니다.") {
		t.Fatalf("status/output = %d/%q, want backlash damage", status, ctx.OutputString())
	}
	alice, _ = world.Creature("creature:alice")
	if got, want := creatureStat(alice, "hpCurrent"), 18; got != want {
		t.Fatalf("alice hp = %d, want %d", got, want)
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 40; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "was_attacked") {
		t.Fatalf("goblin tags = %+v, want was_attacked after failed one kill", goblin.Metadata.Tags)
	}
}

func TestOneKillHandlerRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		mutate func(*worldload.World)
		want   string
	}{
		{name: "missing target", want: "누구를 공격하시겠습니까?"},
		{
			name: "wrong class",
			args: []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["class"] = legacyClassAssassin
				loaded.Creatures[alice.ID] = alice
			},
			want: "무적 이상만 사용할 수 있는 기술입니다.",
		},
		{
			name: "missing training",
			args: []string{"고블린"},
			want: "자객을 무적수련하지 않았습니다..",
		},
		{
			name: "unknown target",
			args: []string{"없는"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"SASSASSIN"}
				loaded.Creatures[alice.ID] = alice
			},
			want: "그런 것은 여기 없습니다.",
		},
		{
			name: "player target is not valid",
			args: []string{"Bob"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"SASSASSIN"}
				loaded.Creatures[alice.ID] = alice
			},
			want: "그런 것은 여기 없습니다.",
		},
		{
			name: "missing weapon",
			args: []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"SASSASSIN"}
				alice.Equipment = nil
				loaded.Creatures[alice.ID] = alice
			},
			want: "날카롭거나 찌르는 무기가 필요합니다.",
		},
		{
			name: "blunt weapon",
			args: []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"SASSASSIN"}
				loaded.Creatures[alice.ID] = alice
				sword := loaded.Objects["object:sword"]
				sword.Properties = map[string]string{"type": "2", "pDice": "5"}
				loaded.Objects[sword.ID] = sword
			},
			want: "날카롭거나 찌르는 무기가 필요합니다.",
		},
		{
			name: "protected monster",
			args: []string{"수호석"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"SASSASSIN"}
				loaded.Creatures[alice.ID] = alice
			},
			want: "당신은 그 상대를 해칠 수 없습니다.",
		},
		{
			name: "magic only monster",
			args: []string{"망령"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"SASSASSIN"}
				loaded.Creatures[alice.ID] = alice
			},
			want: "아무 소용이 없는듯 합니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := invincibleAttackWorld(t)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewOneKillHandler(world)(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func invincibleAttackDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "백보신권", Number: 157, Handler: "invincible_kick"},
		{Name: "invincible_kick", Number: 157, Handler: "invincible_kick"},
		{Name: "일격필살", Number: 158, Handler: "one_kill"},
		{Name: "one_kill", Number: 158, Handler: "one_kill"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"invincible_kick": NewInvincibleKickHandler(world),
			"one_kill":        NewOneKillHandler(world),
		},
	}
}

func invincibleAttackWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:arena",
		DisplayName: "Arena",
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:arena",
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:arena",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:arena",
		Equipment:   map[string]model.ObjectInstanceID{"wield": "object:sword"},
		Stats: map[string]int{
			"class":        legacyClassInvincible,
			"level":        20,
			"strength":     20,
			"dexterity":    20,
			"intelligence": 20,
			"thaco":        0,
			"armor":        0,
			"hpCurrent":    50,
			"hpMax":        50,
			"pDice":        3,
		},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:arena",
		Stats:       map[string]int{"class": legacyClassFighter, "level": 1, "hpCurrent": 30, "hpMax": 30},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:goblin",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:arena",
		Stats:       map[string]int{"level": 1, "dexterity": 10, "armor": 0, "hpCurrent": 40, "hpMax": 40, "experience": 100},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:stone-guardian",
		Kind:        model.CreatureKindMonster,
		DisplayName: "수호석",
		RoomID:      "room:arena",
		Metadata:    model.Metadata{Tags: []string{"unkillable"}},
		Stats:       map[string]int{"level": 1, "dexterity": 10, "armor": 0, "hpCurrent": 40, "hpMax": 40},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:wraith",
		Kind:        model.CreatureKindMonster,
		DisplayName: "망령",
		RoomID:      "room:arena",
		Metadata:    model.Metadata{Tags: []string{"magicOnly"}},
		Stats:       map[string]int{"level": 1, "dexterity": 10, "armor": 0, "hpCurrent": 40, "hpMax": 40},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:sword",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "검",
		Properties:  map[string]string{"type": "0", "pDice": "5"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:sword",
		PrototypeID: "prototype:sword",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "wield"},
	})
	return loaded
}
