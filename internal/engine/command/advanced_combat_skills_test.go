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

func TestBnahanHandlerDamagesRoomTargetsAndStartsCooldown(t *testing.T) {
	withAttackRolls(t, 1, 5, 6)
	loaded := advancedCombatWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS")
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	handler := NewBnahanHandler(world)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "고블린에게 13점의 피해") ||
		!strings.Contains(out, "오크에게 14점의 피해") {
		t.Fatalf("status/output = %d/%q, want bnahan damage", status, out)
	}

	goblin, _ := world.Creature("creature:goblin")
	orc, _ := world.Creature("creature:orc")
	hidden, _ := world.Creature("creature:hidden")
	protected, _ := world.Creature("creature:protected")
	if got, want := creatureStat(goblin, "hpCurrent"), 67; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	if got, want := creatureStat(orc, "hpCurrent"), 66; got != want {
		t.Fatalf("orc hp = %d, want %d", got, want)
	}
	if got, want := creatureStat(hidden, "hpCurrent"), 80; got != want {
		t.Fatalf("hidden hp = %d, want untouched %d", got, want)
	}
	if got, want := creatureStat(protected, "hpCurrent"), 80; got != want {
		t.Fatalf("protected hp = %d, want untouched %d", got, want)
	}
	foundBnahanBroadcast := false
	for _, broadcast := range broadcasts {
		if strings.Contains(broadcast.Text, "변수나한권") {
			foundBnahanBroadcast = true
			break
		}
	}
	if !foundBnahanBroadcast {
		t.Fatalf("broadcasts = %+v, want bnahan room damage", broadcasts)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", bnahanCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active bnahan cooldown", used, remaining)
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
}

func TestBnahanHandlerFailureFatiguesActor(t *testing.T) {
	withAttackRolls(t, 100)
	world := state.NewWorld(advancedCombatWorld(t))

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBnahanHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "피로해짐") {
		t.Fatalf("status/output = %d/%q, want bnahan fatigue", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got, want := creatureStat(alice, "hpCurrent"), 90; got != want {
		t.Fatalf("alice hp = %d, want %d", got, want)
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 80; got != want {
		t.Fatalf("goblin hp = %d, want unchanged %d", got, want)
	}
	orc, _ := world.Creature("creature:orc")
	for _, target := range []model.Creature{goblin, orc} {
		if !hasAnyNormalizedFlag(target.Metadata.Tags, "was_attacked") {
			t.Fatalf("%s tags = %+v, want was_attacked after failed bnahan", target.ID, target.Metadata.Tags)
		}
	}
}

func TestBnahanHandlerFinalizesMonsterDeath(t *testing.T) {
	withAttackRolls(t, 1, 5, 6)
	loaded := advancedCombatWorld(t)
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Stats["hpCurrent"] = 10
	goblin.Stats["hpMax"] = 10
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBnahanHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "죽였습니다") {
		t.Fatalf("status/output = %d/%q, want bnahan death", status, ctx.OutputString())
	}
	if _, ok := world.Creature("creature:goblin"); ok {
		t.Fatal("dead bnahan target still exists in world")
	}
	arena, _ := world.Room("room:arena")
	if containsCreatureID(arena.CreatureIDs, "creature:goblin") {
		t.Fatalf("arena creatures = %+v, want goblin removed", arena.CreatureIDs)
	}
}

func TestBnahanHandlerRejectsInvalidStates(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*worldload.World)
		setup  func(*state.World)
		want   string
	}{
		{
			name: "wrong class",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["class"] = legacyClassFighter
				loaded.Creatures[alice.ID] = alice
			},
			want: "권법가 레벨 50이상만 쓸수 있는 기술입니다.",
		},
		{
			name: "barbarian below level",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["class"] = legacyClassBarbarian
				alice.Stats["level"] = 49
				alice.Level = 49
				loaded.Creatures[alice.ID] = alice
			},
			want: "권법가 레벨 50이상만 쓸수 있는 기술입니다.",
		},
		{
			name: "invincible without barbarian training",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"STHIEF"}
				loaded.Creatures[alice.ID] = alice
			},
			want: "권법가를 무적수련하지 않았습니다.",
		},
		{
			name: "no targets",
			mutate: func(loaded *worldload.World) {
				for id, creature := range loaded.Creatures {
					if creature.Kind != model.CreatureKindMonster {
						continue
					}
					creature.Metadata.Tags = []string{"MUNKIL"}
					loaded.Creatures[id] = creature
				}
			},
			want: "이 방에는 당신이 공격할 적이 없습니다.",
		},
		{
			name: "cooldown active",
			setup: func(world *state.World) {
				if err := world.SetCreatureCooldown("creature:alice", bnahanCooldownKey, time.Now().Unix(), bnahanCooldownSeconds(model.Creature{Stats: map[string]int{"dexterity": 24}})); err != nil {
					t.Fatalf("SetCreatureCooldown() error = %v", err)
				}
			},
			want: "기다리세요.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := advancedCombatWorld(t)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
			if tt.setup != nil {
				tt.setup(world)
			}
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewBnahanHandler(world)(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestBnahanHandlerCooldownPrecedesTargetScanAndReveal(t *testing.T) {
	loaded := advancedCombatWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS")
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	for id, creature := range loaded.Creatures {
		if creature.Kind != model.CreatureKindMonster {
			continue
		}
		creature.Metadata.Tags = []string{"MUNKIL"}
		loaded.Creatures[id] = creature
	}
	world := state.NewWorld(loaded)
	if err := world.SetCreatureCooldown("creature:alice", bnahanCooldownKey, time.Now().Unix(), bnahanCooldownSeconds(alice)); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBnahanHandler(world)(ctx, ResolvedCommand{})
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

func TestTaguHandlerHitsMonsterAndStartsCooldown(t *testing.T) {
	withAttackRolls(t, 1, 20, 1)
	loaded := advancedCombatWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS")
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	handler := NewTaguHandler(world)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "3연타 33점") {
		t.Fatalf("status/output = %d/%q, want tagu damage", status, out)
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 47; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	foundTaguBroadcast := false
	for _, broadcast := range broadcasts {
		if strings.Contains(broadcast.Text, "타구봉법") {
			foundTaguBroadcast = true
			break
		}
	}
	if !foundTaguBroadcast {
		t.Fatalf("broadcasts = %+v, want tagu room damage", broadcasts)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", taguCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active tagu cooldown", used, remaining)
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
}

func TestTaguHandlerCooldownPrecedesWeaponTargetAndReveal(t *testing.T) {
	loaded := advancedCombatWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS")
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	proto := loaded.ObjectPrototypes["prototype:staff"]
	proto.Properties["type"] = "1"
	loaded.ObjectPrototypes[proto.ID] = proto
	world := state.NewWorld(loaded)
	if err := world.SetCreatureCooldown("creature:alice", taguCooldownKey, time.Now().Unix(), taguSuccessCooldownSeconds(alice)); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewTaguHandler(world)(ctx, ResolvedCommand{Args: []string{"없는"}})
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

func TestTaguHandlerFailureAndMissSetShorterCooldowns(t *testing.T) {
	t.Run("chance failure", func(t *testing.T) {
		withAttackRolls(t, 100)
		world := state.NewWorld(advancedCombatWorld(t))

		ctx := &Context{ActorID: "player:alice"}
		status, err := NewTaguHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
		if err != nil {
			t.Fatalf("handler() error = %v", err)
		}
		if status != StatusDefault || !strings.Contains(ctx.OutputString(), "타구봉법에 실패했습니다") {
			t.Fatalf("status/output = %d/%q, want tagu failure", status, ctx.OutputString())
		}
		goblin, _ := world.Creature("creature:goblin")
		if got, want := creatureStat(goblin, "hpCurrent"), 80; got != want {
			t.Fatalf("goblin hp = %d, want unchanged %d", got, want)
		}
		if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "was_attacked") {
			t.Fatalf("goblin tags = %+v, want was_attacked after failed tagu", goblin.Metadata.Tags)
		}
		if remaining, used, err := world.UseCreatureCooldown("creature:alice", taguCooldownKey, time.Now().Unix(), 1); err != nil {
			t.Fatalf("UseCreatureCooldown() error = %v", err)
		} else if used || remaining <= 0 {
			t.Fatalf("cooldown used/remaining = %v/%d, want active tagu failure cooldown", used, remaining)
		}
	})

	t.Run("attack miss", func(t *testing.T) {
		withAttackRolls(t, 1, 1)
		loaded := advancedCombatWorld(t)
		alice := loaded.Creatures["creature:alice"]
		alice.Stats["thaco"] = 20
		loaded.Creatures[alice.ID] = alice
		world := state.NewWorld(loaded)

		ctx := &Context{ActorID: "player:alice"}
		status, err := NewTaguHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
		if err != nil {
			t.Fatalf("handler() error = %v", err)
		}
		if status != StatusDefault || !strings.Contains(ctx.OutputString(), "타구봉법에 실패했습니다") {
			t.Fatalf("status/output = %d/%q, want tagu miss", status, ctx.OutputString())
		}
		goblin, _ := world.Creature("creature:goblin")
		if got, want := creatureStat(goblin, "hpCurrent"), 80; got != want {
			t.Fatalf("goblin hp = %d, want unchanged %d", got, want)
		}
		if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "was_attacked") {
			t.Fatalf("goblin tags = %+v, want was_attacked after missed tagu", goblin.Metadata.Tags)
		}
	})
}

func TestTaguChanceMatchesLegacyLevelDeltaFormula(t *testing.T) {
	actor := model.Creature{
		Level: 50,
		Stats: map[string]int{
			"level":        50,
			"intelligence": 0,
			"dexterity":    0,
		},
	}
	victim := model.Creature{
		Level: 1,
		Stats: map[string]int{"level": 1},
	}

	if got, want := taguChance(actor, victim), 38; got != want {
		t.Fatalf("taguChance() = %d, want C level-delta formula result %d", got, want)
	}
}

func TestTaguHandlerWeaponBreakOnSuccessDoesNotStartCooldown(t *testing.T) {
	withAttackRolls(t, 1)
	loaded := advancedCombatWorld(t)
	proto := loaded.ObjectPrototypes["prototype:staff"]
	proto.Properties["shotsCurrent"] = "1"
	loaded.ObjectPrototypes[proto.ID] = proto
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewTaguHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "부서져 버렸습니다.") {
		t.Fatalf("status/output = %d/%q, want weapon break", status, ctx.OutputString())
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 80; got != want {
		t.Fatalf("goblin hp = %d, want unchanged after break %d", got, want)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", taguCooldownKey, time.Now().Unix(), 0); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want no cooldown after break-return", used, remaining)
	}
}

func TestTaguHandlerFinalizesMonsterDeath(t *testing.T) {
	withAttackRolls(t, 1, 20, 1)
	loaded := advancedCombatWorld(t)
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Stats["hpCurrent"] = 10
	goblin.Stats["hpMax"] = 10
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewTaguHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "죽였습니다") {
		t.Fatalf("status/output = %d/%q, want tagu death", status, ctx.OutputString())
	}
	if _, ok := world.Creature("creature:goblin"); ok {
		t.Fatal("dead tagu target still exists in world")
	}
	arena, _ := world.Room("room:arena")
	if containsCreatureID(arena.CreatureIDs, "creature:goblin") {
		t.Fatalf("arena creatures = %+v, want goblin removed", arena.CreatureIDs)
	}
}

func TestTaguHandlerRejectsInvalidStates(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		mutate func(*worldload.World)
		setup  func(*state.World)
		want   string
	}{
		{name: "missing target", want: "누굴 공격합니까?"},
		{
			name: "blind",
			args: []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = append(alice.Metadata.Tags, "PBLIND")
				loaded.Creatures[alice.ID] = alice
			},
			want: "누굴 공격합니까?",
		},
		{
			name: "wrong class",
			args: []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["class"] = legacyClassFighter
				loaded.Creatures[alice.ID] = alice
			},
			want: "도둑 레벨 50이상만 쓸수 있는 기술입니다.",
		},
		{
			name: "thief below level",
			args: []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["class"] = legacyClassThief
				alice.Stats["level"] = 49
				alice.Level = 49
				loaded.Creatures[alice.ID] = alice
			},
			want: "도둑 레벨 50이상만 쓸수 있는 기술입니다.",
		},
		{
			name: "invincible without thief training",
			args: []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"SBARBARIAN"}
				loaded.Creatures[alice.ID] = alice
			},
			want: "도둑을 무적수련하지 않았습니다.",
		},
		{
			name: "wrong weapon",
			args: []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				proto := loaded.ObjectPrototypes["prototype:staff"]
				proto.Properties["type"] = "1"
				loaded.ObjectPrototypes[proto.ID] = proto
			},
			want: "둔탁한 무기가 필요합니다.",
		},
		{name: "unknown target", args: []string{"없는"}, want: "그런 것은 여기 없습니다."},
		{name: "player kill gate", args: []string{"Bob"}, want: "당신은 선해서 다른 사용자를 공격할 수 없습니다."},
		{name: "protected monster", args: []string{"수호석"}, want: "당신은 그 상대를 해칠 수 없습니다."},
		{
			name: "cooldown active",
			args: []string{"고블린"},
			setup: func(world *state.World) {
				if err := world.SetCreatureCooldown("creature:alice", taguCooldownKey, time.Now().Unix(), taguSuccessCooldownSeconds(model.Creature{Stats: map[string]int{"dexterity": 24}})); err != nil {
					t.Fatalf("SetCreatureCooldown() error = %v", err)
				}
			},
			want: "기다리세요.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := advancedCombatWorld(t)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
			if tt.setup != nil {
				tt.setup(world)
			}
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewTaguHandler(world)(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestAdvancedCombatHandlersCanBeRegisteredByDispatcherAliases(t *testing.T) {
	t.Run("bnahan", func(t *testing.T) {
		for _, line := range []string{"변수나한권", "bnahan"} {
			t.Run(line, func(t *testing.T) {
				withAttackRolls(t, 1, 5, 6)
				world := state.NewWorld(advancedCombatWorld(t))
				dispatcher := advancedCombatDispatcher(t, world)

				ctx := &Context{ActorID: "player:alice"}
				status, err := dispatcher.DispatchLine(ctx, line)
				if err != nil {
					t.Fatalf("DispatchLine(%q) error = %v", line, err)
				}
				if status != StatusDefault || !strings.Contains(ctx.OutputString(), "변수나한권으로 고블린") {
					t.Fatalf("status/output = %d/%q, want bnahan dispatch success", status, ctx.OutputString())
				}
			})
		}
	})

	t.Run("tagu", func(t *testing.T) {
		for _, line := range []string{"고블린 타구봉법", "tagu 고블린"} {
			t.Run(line, func(t *testing.T) {
				withAttackRolls(t, 1, 20, 1)
				world := state.NewWorld(advancedCombatWorld(t))
				dispatcher := advancedCombatDispatcher(t, world)

				ctx := &Context{ActorID: "player:alice"}
				status, err := dispatcher.DispatchLine(ctx, line)
				if err != nil {
					t.Fatalf("DispatchLine(%q) error = %v", line, err)
				}
				if status != StatusDefault || !strings.Contains(ctx.OutputString(), "타구봉법으로 고블린") {
					t.Fatalf("status/output = %d/%q, want tagu dispatch success", status, ctx.OutputString())
				}
			})
		}
	})
}

func advancedCombatDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "변수나한권", Number: 167, Handler: "bnahan"},
		{Name: "bnahan", Number: 167, Handler: "bnahan"},
		{Name: "타구봉법", Number: 168, Handler: "tagu"},
		{Name: "tagu", Number: 168, Handler: "tagu"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"bnahan": NewBnahanHandler(world),
			"tagu":   NewTaguHandler(world),
		},
	}
}

func advancedCombatWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{ID: "room:arena", DisplayName: "Arena"})
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
		Level:       50,
		Equipment:   map[string]model.ObjectInstanceID{"wield": "object:staff"},
		Stats: map[string]int{
			"class":        legacyClassInvincible,
			"level":        50,
			"strength":     24,
			"dexterity":    24,
			"intelligence": 24,
			"piety":        20,
			"thaco":        0,
			"armor":        0,
			"hpCurrent":    100,
			"hpMax":        100,
			"pDice":        2,
		},
		Metadata: model.Metadata{Tags: []string{"SBARBARIAN", "STHIEF"}},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:arena",
		Stats:       map[string]int{"class": legacyClassFighter, "level": 1, "hpCurrent": 50, "hpMax": 50},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:goblin",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:arena",
		Stats:       map[string]int{"level": 10, "armor": 0, "hpCurrent": 80, "hpMax": 80, "piety": 10, "experience": 100},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:hidden",
		Kind:        model.CreatureKindMonster,
		DisplayName: "숨은 적",
		RoomID:      "room:arena",
		Stats:       map[string]int{"level": 10, "armor": 0, "hpCurrent": 80, "hpMax": 80},
		Metadata:    model.Metadata{Tags: []string{"hidden"}},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:orc",
		Kind:        model.CreatureKindMonster,
		DisplayName: "오크",
		RoomID:      "room:arena",
		Stats:       map[string]int{"level": 10, "armor": 0, "hpCurrent": 80, "hpMax": 80, "piety": 10, "experience": 100},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:protected",
		Kind:        model.CreatureKindMonster,
		DisplayName: "수호석",
		RoomID:      "room:arena",
		Stats:       map[string]int{"level": 10, "armor": 0, "hpCurrent": 80, "hpMax": 80},
		Metadata:    model.Metadata{Tags: []string{"MUNKIL"}},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:staff",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "목봉",
		Properties:  map[string]string{"type": "2", "pDice": "3"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:staff",
		PrototypeID: "prototype:staff",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "wield"},
	})
	return loaded
}
