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

func TestPowerHandlerSuccessAddsStrengthTagsCooldownAndExpiration(t *testing.T) {
	world := state.NewWorld(energySkillWorld(t, model.ClassFighter, nil, false))
	defer world.Close()
	handler := NewPowerHandler(world, fixedRoll(1))
	var broadcasts []roomBroadcastRecord

	before := time.Now().Unix()
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "기를 모으기 시작합니다") {
		t.Fatalf("status/output = %d/%q, want power success", status, ctx.OutputString())
	}
	if got, want := ctx.OutputString(), "당신은 가부좌를 틀고 기를 모으기 시작합니다.\n온몸으로 기가 퍼져나가는것을 느낍니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}

	creature, _ := world.Creature("creature:alice")
	if got, want := creatureStat(creature, "strength"), 33; got != want {
		t.Fatalf("strength = %d, want %d", got, want)
	}
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "PPOWER", "power") {
		t.Fatalf("creature tags = %+v, want power status", creature.Metadata.Tags)
	}
	player, _ := world.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "PPOWER", "power") {
		t.Fatalf("player tags = %+v, want power status", player.Metadata.Tags)
	}
	if len(broadcasts) != 1 || broadcasts[0].Text != "Alice이 가부좌를 틀고 앉아 기를 모읍니다." {
		t.Fatalf("broadcasts = %+v, want power broadcast", broadcasts)
	}
	assertCreatureEffectExpiration(t, world, "PPOWER", before, powerStatusDurationSeconds(creature))
	assertEnergyCooldownActive(t, world, powerCooldownKey, powerSuccessCooldownSeconds)
}

func TestPowerHandlerRejectsInvalidStates(t *testing.T) {
	tests := []struct {
		name  string
		class int
		tags  []string
		setup func(*state.World)
		want  string
	}{
		{name: "wrong class", class: model.ClassThief, want: "검사만 사용할 수 있는 기술입니다."},
		{name: "invincible without training", class: model.ClassInvincible, want: "검사를 무적수련하지 않았습니다.."},
		{name: "already active", class: model.ClassFighter, tags: []string{"PPOWER"}, want: "기공집결을 사용중입니다."},
		{
			name:  "cooldown active",
			class: model.ClassFighter,
			setup: func(world *state.World) {
				if err := world.SetCreatureCooldown("creature:alice", powerCooldownKey, time.Now().Unix(), powerSuccessCooldownSeconds); err != nil {
					t.Fatalf("SetCreatureCooldown() error = %v", err)
				}
			},
			want: "기다리세요.",
		},
		{name: "invincible with fighter training", class: model.ClassInvincible, tags: []string{"SFIGHTER"}, want: "기를 모으기 시작합니다"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := state.NewWorld(energySkillWorld(t, tt.class, tt.tags, false))
	defer world.Close()
			if tt.setup != nil {
				tt.setup(world)
			}
			handler := NewPowerHandler(world, fixedRoll(1))

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestPowerHandlerFailureSetsShortCooldownWithoutStatus(t *testing.T) {
	world := state.NewWorld(energySkillWorld(t, model.ClassFighter, nil, false))
	defer world.Close()
	handler := NewPowerHandler(world, fixedRoll(100))
	var broadcasts []roomBroadcastRecord

	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "기공집결이 실패하였습니다.\n" {
		t.Fatalf("status/output = %d/%q, want power failure", status, ctx.OutputString())
	}
	creature, _ := world.Creature("creature:alice")
	if got, want := creatureStat(creature, "strength"), 30; got != want {
		t.Fatalf("strength = %d, want unchanged %d", got, want)
	}
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "PPOWER", "power") {
		t.Fatalf("creature tags = %+v, want no power status", creature.Metadata.Tags)
	}
	assertNoCreatureEffectExpiration(t, world, "PPOWER")
	if len(broadcasts) != 1 || broadcasts[0].Text != "Alice이 기공집결을 시도합니다." {
		t.Fatalf("broadcasts = %+v, want failure broadcast", broadcasts)
	}
	assertEnergyCooldownActive(t, world, powerCooldownKey, powerFailureCooldownSeconds)
}

func TestAccurateHandlerSuccessAddsThacoTagsCooldownAndExpiration(t *testing.T) {
	world := state.NewWorld(energySkillWorld(t, model.ClassThief, nil, true))
	defer world.Close()
	world.RecalculateTHACOFunc = func(creatureID model.CreatureID) error {
		c, _ := world.Creature(creatureID)
		baseThaco := 10
		if hasAnyNormalizedFlag(c.Metadata.Tags, "PSLAYE", "accurate", "slayer") {
			_ = world.SetCreatureStat(creatureID, "thaco", baseThaco-3)
		} else {
			_ = world.SetCreatureStat(creatureID, "thaco", baseThaco)
		}
		return nil
	}
	world.RecalculateACFunc = func(creatureID model.CreatureID) error {
		return nil
	}
	handler := NewAccurateHandler(world, fixedRoll(1))
	var broadcasts []roomBroadcastRecord

	before := time.Now().Unix()
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "살기") {
		t.Fatalf("status/output = %d/%q, want accurate success", status, ctx.OutputString())
	}
	if got, want := ctx.OutputString(), "당신은 당신의 무기에 피를 먹입니다.\n무기에 살기가 감도는 것을 느낍니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}

	creature, _ := world.Creature("creature:alice")
	if got, want := creatureStat(creature, "thaco"), 7; got != want {
		t.Fatalf("thaco = %d, want %d", got, want)
	}
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "PSLAYE", "accurate", "slayer") {
		t.Fatalf("creature tags = %+v, want accurate status", creature.Metadata.Tags)
	}
	player, _ := world.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "PSLAYE", "accurate", "slayer") {
		t.Fatalf("player tags = %+v, want accurate status", player.Metadata.Tags)
	}
	if len(broadcasts) != 1 || broadcasts[0].Text != "Alice이 그의 무기에 피를 먹입니다." {
		t.Fatalf("broadcasts = %+v, want accurate broadcast", broadcasts)
	}
	assertCreatureEffectExpiration(t, world, "PSLAYE", before, accurateStatusDurationSeconds(creature))
	assertEnergyCooldownActive(t, world, accurateCooldownKey, accurateSuccessCooldownSeconds)
}

func TestAccurateHandlerRejectsInvalidStates(t *testing.T) {
	tests := []struct {
		name  string
		class int
		tags  []string
		wield bool
		setup func(*state.World)
		want  string
	}{
		{name: "wrong class", class: model.ClassFighter, wield: true, want: "자객과 도둑만 사용할 수 있는 기술입니다."},
		{name: "invincible without training", class: model.ClassInvincible, wield: true, want: "자객이나 도둑을 무적수련하지 않았습니다.."},
		{name: "already active", class: model.ClassAssassin, tags: []string{"PSLAYE"}, wield: true, want: "살기충전을 사용중입니다."},
		{name: "missing weapon", class: model.ClassThief, want: "장비하고 있는 무기가 없습니다!"},
		{
			name:  "cooldown active",
			class: model.ClassThief,
			wield: true,
			setup: func(world *state.World) {
				if err := world.SetCreatureCooldown("creature:alice", accurateCooldownKey, time.Now().Unix(), accurateSuccessCooldownSeconds); err != nil {
					t.Fatalf("SetCreatureCooldown() error = %v", err)
				}
			},
			want: "기다리세요.",
		},
		{name: "invincible with thief training", class: model.ClassInvincible, tags: []string{"STHIEF"}, wield: true, want: "살기"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := state.NewWorld(energySkillWorld(t, tt.class, tt.tags, tt.wield))
	defer world.Close()
			if tt.setup != nil {
				tt.setup(world)
			}
			handler := NewAccurateHandler(world, fixedRoll(1))

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestAccurateHandlerFailureSetsShortCooldownWithoutStatus(t *testing.T) {
	world := state.NewWorld(energySkillWorld(t, model.ClassThief, nil, true))
	defer world.Close()
	handler := NewAccurateHandler(world, fixedRoll(100))
	var broadcasts []roomBroadcastRecord

	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "살기충전이 실패하였습니다.\n" {
		t.Fatalf("status/output = %d/%q, want accurate failure", status, ctx.OutputString())
	}
	creature, _ := world.Creature("creature:alice")
	if got, want := creatureStat(creature, "thaco"), 10; got != want {
		t.Fatalf("thaco = %d, want unchanged %d", got, want)
	}
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "PSLAYE", "accurate", "slayer") {
		t.Fatalf("creature tags = %+v, want no accurate status", creature.Metadata.Tags)
	}
	assertNoCreatureEffectExpiration(t, world, "PSLAYE")
	if len(broadcasts) != 1 || broadcasts[0].Text != "Alice이 그의 무기에 살기충전을 시도합니다." {
		t.Fatalf("broadcasts = %+v, want failure broadcast", broadcasts)
	}
	assertEnergyCooldownActive(t, world, accurateCooldownKey, accurateFailureCooldownSeconds)
}

func TestAbsorbHandlerSuccessRevealsDrainsHealsAndStartsCooldown(t *testing.T) {
	loaded := energySkillWorld(t, model.ClassInvincible, []string{"SMAGE", "invisible", "PINVIS"}, false)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	defer world.Close()
	handler := NewAbsorbHandler(world, energyRolls(t, 1, 3))
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
	if !strings.Contains(out, "모습이 나타나기 시작합니다") ||
		!strings.Contains(out, "기를 30만큼 흡수했습니다") {
		t.Fatalf("output = %q, want reveal and absorb damage", out)
	}

	alice, _ = world.Creature("creature:alice")
	if got, want := creatureStat(alice, "hpCurrent"), 60; got != want {
		t.Fatalf("alice hpCurrent = %d, want %d", got, want)
	}
	if hasAnyNormalizedFlag(alice.Metadata.Tags, "invisible", "PINVIS") || alice.Stats["PINVIS"] != 0 {
		t.Fatalf("alice tags/stats = %+v/%+v, want invisible cleared", alice.Metadata.Tags, alice.Stats)
	}
	if hasAnyNormalizedFlag(alice.Metadata.Tags, "PABSORB", "absorb") {
		t.Fatalf("alice tags = %+v, want no C-absent absorb status", alice.Metadata.Tags)
	}
	player, _ = world.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "PINVIS") {
		t.Fatalf("player tags = %+v, want invisible cleared", player.Metadata.Tags)
	}
	if hasAnyNormalizedFlag(player.Metadata.Tags, "PABSORB", "absorb") {
		t.Fatalf("player tags = %+v, want no C-absent absorb status", player.Metadata.Tags)
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 70; got != want {
		t.Fatalf("goblin hpCurrent = %d, want %d", got, want)
	}
	if len(broadcasts) < 2 || !strings.Contains(broadcasts[len(broadcasts)-1].Text, "기를 30만큼 흡수했습니다") {
		t.Fatalf("broadcasts = %+v, want absorb broadcast", broadcasts)
	}
	assertEnergyCooldownActive(t, world, absorbCooldownKey, absorbCooldownSeconds)
}

func TestAbsorbHandlerFailureStartsCooldownWithoutDamage(t *testing.T) {
	world := state.NewWorld(energySkillWorld(t, model.ClassInvincible, []string{"SMAGE"}, false))
	defer world.Close()
	handler := NewAbsorbHandler(world, fixedRoll(100))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "주문을 견뎌냈습니다") {
		t.Fatalf("status/output = %d/%q, want absorb failure", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got, want := creatureStat(alice, "hpCurrent"), 30; got != want {
		t.Fatalf("alice hpCurrent = %d, want unchanged %d", got, want)
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 100; got != want {
		t.Fatalf("goblin hpCurrent = %d, want unchanged %d", got, want)
	}
	if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "was_attacked") {
		t.Fatalf("goblin tags = %+v, want was_attacked after failed absorb", goblin.Metadata.Tags)
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

func TestAbsorbHandlerRejectsInvalidStates(t *testing.T) {
	tests := []struct {
		name   string
		class  int
		tags   []string
		args   []string
		mutate func(*worldload.World)
		want   string
	}{
		{name: "missing target", class: model.ClassInvincible, tags: []string{"SMAGE"}, want: "누구에게 주문을 거실려고요?"},
		{name: "wrong class", class: model.ClassMage, tags: []string{"SMAGE"}, args: []string{"고블린"}, want: "무적이상만 쓸 수 있는 기술입니다."},
		{name: "missing training", class: model.ClassInvincible, args: []string{"고블린"}, want: "도술사를 무적수련하지 않았습니다.."},
		{name: "missing monster", class: model.ClassInvincible, tags: []string{"SMAGE"}, args: []string{"없는"}, want: "그런 괴물은 존재하지 않습니다."},
		{
			name:  "protected monster",
			class: model.ClassInvincible,
			tags:  []string{"SMAGE"},
			args:  []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				goblin := loaded.Creatures["creature:goblin"]
				goblin.Metadata.Tags = []string{"MUNKIL", "MMALES"}
				loaded.Creatures[goblin.ID] = goblin
			},
			want: "당신은 그의 기를 흡수할수 없습니다.",
		},
		{
			name:  "dead target",
			class: model.ClassInvincible,
			tags:  []string{"SMAGE"},
			args:  []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				goblin := loaded.Creatures["creature:goblin"]
				goblin.Stats["hpCurrent"] = 0
				loaded.Creatures[goblin.ID] = goblin
			},
			want: "그 상대는 공격할 수 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := energySkillWorld(t, tt.class, tt.tags, false)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
	defer world.Close()
			handler := NewAbsorbHandler(world, fixedRoll(1))

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

func TestAbsorbHandlerWaitsBeforeProtectedTargetLikeLegacy(t *testing.T) {
	loaded := energySkillWorld(t, model.ClassInvincible, []string{"SMAGE"}, false)
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Metadata.Tags = []string{"MUNKIL", "MMALES"}
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	defer world.Close()
	if err := world.SetCreatureCooldown("creature:alice", absorbCooldownKey, time.Now().Unix(), 5); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewAbsorbHandler(world, fixedRoll(1))(ctx, ResolvedCommand{Args: []string{"고블린"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	output := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(output, "기다리세요") || strings.Contains(output, "흡수할수 없습니다") {
		t.Fatalf("status/output = %d/%q, want cooldown wait before protected refusal", status, output)
	}
	goblin, _ = world.Creature("creature:goblin")
	if hasAnyNormalizedFlag(goblin.Metadata.Tags, "was_attacked") {
		t.Fatalf("goblin tags = %+v, want no enemy marking while waiting", goblin.Metadata.Tags)
	}
}

func TestAbsorbHandlerUndeadSuccessDrainsCasterManaOnly(t *testing.T) {
	loaded := energySkillWorld(t, model.ClassInvincible, []string{"SMAGE"}, false)
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Metadata.Tags = []string{"MUNDED"}
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	defer world.Close()
	handler := NewAbsorbHandler(world, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "더러운 기운") {
		t.Fatalf("status/output = %d/%q, want undead absorb backlash", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got := creatureStat(alice, "mpCurrent"); got != 0 {
		t.Fatalf("alice mpCurrent = %d, want 0", got)
	}
	goblin, _ = world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 100; got != want {
		t.Fatalf("goblin hpCurrent = %d, want unchanged %d", got, want)
	}
}

func TestAbsorbHandlerKillingDrainFinalizesMonster(t *testing.T) {
	loaded := energySkillWorld(t, model.ClassInvincible, []string{"SMAGE"}, false)
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Stats["hpCurrent"] = 20
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	defer world.Close()
	handler := NewAbsorbHandler(world, energyRolls(t, 1, 3))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "쓰러졌습니다") {
		t.Fatalf("status/output = %d/%q, want death message", status, ctx.OutputString())
	}
	if !strings.Contains(ctx.OutputString(), "30만큼 흡수했습니다") {
		t.Fatalf("output = %q, want C rolled damage output on overkill", ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got := creatureStat(alice, "hpCurrent"); got != 60 {
		t.Fatalf("alice hpCurrent = %d, want C rolled-damage heal to 60", got)
	}
	if _, ok := world.Creature("creature:goblin"); ok {
		t.Fatal("goblin still exists, want finalized death")
	}
}

func TestAbsorbHandlerUsesCustomDeathFinalizer(t *testing.T) {
	loaded := energySkillWorld(t, model.ClassInvincible, []string{"SMAGE"}, false)
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Stats["hpCurrent"] = 20
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	defer world.Close()
	called := false
	handler := NewAbsorbHandlerWithDeathFinalizer(world, energyRolls(t, 1, 3), func(_ *Context, attacker model.Creature, victim model.Creature) error {
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
		t.Fatal("dead absorb target still exists in world")
	}
}

func TestEnergySkillHandlersCanBeRegisteredByDispatcherAliases(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		world    *state.World
		handlers map[string]Handler
		want     string
	}{
		{
			name:  "power english",
			line:  "power",
			world: state.NewWorld(energySkillWorld(t, model.ClassFighter, nil, false)),
			want:  "기를 모으기 시작합니다",
		},
		{
			name:  "power korean",
			line:  "기공집결",
			world: state.NewWorld(energySkillWorld(t, model.ClassFighter, nil, false)),
			want:  "기를 모으기 시작합니다",
		},
		{
			name:  "accurate english",
			line:  "accurate",
			world: state.NewWorld(energySkillWorld(t, model.ClassThief, nil, true)),
			want:  "살기",
		},
		{
			name:  "accurate korean",
			line:  "살기충전",
			world: state.NewWorld(energySkillWorld(t, model.ClassThief, nil, true)),
			want:  "살기",
		},
		{
			name:  "absorb english",
			line:  "absorb 고블린",
			world: state.NewWorld(energySkillWorld(t, model.ClassInvincible, []string{"SMAGE"}, false)),
			want:  "흡수했습니다",
		},
		{
			name:  "absorb korean",
			line:  "고블린 흡성대법",
			world: state.NewWorld(energySkillWorld(t, model.ClassInvincible, []string{"SMAGE"}, false)),
			want:  "흡수했습니다",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dispatcher := Dispatcher{
				Registry: mustRegistry(t, []commandspec.CommandSpec{
					{Name: "기공집결", Number: 87, Handler: "power"},
					{Name: "power", Number: 87, Handler: "power"},
					{Name: "살기충전", Number: 88, Handler: "accurate"},
					{Name: "accurate", Number: 88, Handler: "accurate"},
					{Name: "흡성대법", Number: 89, Handler: "absorb"},
					{Name: "absorb", Number: 89, Handler: "absorb"},
				}),
				Handlers: map[string]Handler{
					"power":    NewPowerHandler(tt.world, fixedRoll(1)),
					"accurate": NewAccurateHandler(tt.world, fixedRoll(1)),
					"absorb":   NewAbsorbHandler(tt.world, energyRolls(t, 1, 3)),
				},
			}

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, tt.line)
			if err != nil {
				t.Fatalf("DispatchLine(%q) error = %v", tt.line, err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func energySkillWorld(t *testing.T, class int, tags []string, wield bool) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:energy",
		DisplayName: "수련장",
		PlayerIDs:   []model.PlayerID{"player:alice"},
		CreatureIDs: []model.CreatureID{"creature:goblin"},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:energy",
	})
	equipment := map[string]model.ObjectInstanceID(nil)
	if wield {
		equipment = map[string]model.ObjectInstanceID{"wield": "object:sword"}
		mustAddLookPrototype(t, loaded, model.ObjectPrototype{
			ID:          "prototype:sword",
			Kind:        model.ObjectKindWeapon,
			DisplayName: "검",
		})
		mustAddLookObject(t, loaded, model.ObjectInstance{
			ID:                  "object:sword",
			PrototypeID:         "prototype:sword",
			DisplayNameOverride: "검",
			Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "wield"},
		})
	}
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:energy",
		Level:       40,
		Stats: map[string]int{
			"class":        class,
			"level":        40,
			"dexterity":    40,
			"strength":     30,
			"intelligence": 30,
			"thaco":        10,
			"hpCurrent":    30,
			"hpMax":        100,
			"mpCurrent":    40,
			"mpMax":        80,
		},
		Equipment: equipment,
		Metadata:  model.Metadata{Tags: tags},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:goblin",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:energy",
		Level:       4,
		Stats:       map[string]int{"level": 4, "hpCurrent": 100, "hpMax": 100},
	})
	return loaded
}

func assertEnergyCooldownActive(t *testing.T, world *state.World, key string, maxRemaining int64) {
	t.Helper()

	remaining, used, err := world.UseCreatureCooldown("creature:alice", key, time.Now().Unix(), 1)
	if err != nil {
		t.Fatalf("UseCreatureCooldown(%q) error = %v", key, err)
	}
	if used || remaining < 1 || remaining > maxRemaining {
		t.Fatalf("cooldown %q used/remaining = %v/%d, want active <= %d", key, used, remaining, maxRemaining)
	}
}

func assertCreatureEffectExpiration(t *testing.T, world *state.World, tag string, before int64, duration int64) {
	t.Helper()

	expires, ok := world.GetEffectExpiration("creature:alice", tag)
	if !ok {
		t.Fatalf("%s effect expiration was not set", tag)
	}
	if expires < before+duration || expires > time.Now().Unix()+duration {
		t.Fatalf("%s expiration = %d, want about now+%d", tag, expires, duration)
	}
}

func assertNoCreatureEffectExpiration(t *testing.T, world *state.World, tag string) {
	t.Helper()

	if _, ok := world.GetEffectExpiration("creature:alice", tag); ok {
		t.Fatalf("%s effect expiration was set", tag)
	}
}

func energyRolls(t *testing.T, rolls ...int) SearchRollFunc {
	t.Helper()
	index := 0
	return func(min int, max int) int {
		if index >= len(rolls) {
			t.Fatalf("energy roll(%d, %d) called after %d scripted rolls", min, max, len(rolls))
		}
		value := rolls[index]
		index++
		if value < min || value > max {
			t.Fatalf("scripted energy roll %d for range [%d,%d]", value, min, max)
		}
		return value
	}
}
