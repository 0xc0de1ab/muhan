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

func TestChangHandlerDamagesVisibleMonstersStartsCooldownAndRevealsActor(t *testing.T) {
	withAttackRolls(t, 1, 2, 1)
	loaded := utilityCombatWorld(t, model.ClassCaretaker, legacyObjectPole)
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

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := NewChangHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "당신의 모습이 서서히 드러납니다.") ||
		!strings.Contains(out, "창격술로 고블린에게 36점의 피해") ||
		!strings.Contains(out, "창격술로 오크에게 18점의 피해") {
		t.Fatalf("output = %q, want reveal and chang damage", out)
	}

	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 44; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	orc, _ := world.Creature("creature:orc")
	if got, want := creatureStat(orc, "hpCurrent"), 62; got != want {
		t.Fatalf("orc hp = %d, want %d", got, want)
	}
	hidden, _ := world.Creature("creature:hidden")
	protected, _ := world.Creature("creature:protected")
	if got, want := creatureStat(hidden, "hpCurrent"), 80; got != want {
		t.Fatalf("hidden hp = %d, want untouched %d", got, want)
	}
	if got, want := creatureStat(protected, "hpCurrent"), 80; got != want {
		t.Fatalf("protected hp = %d, want untouched %d", got, want)
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
	if len(broadcasts) < 3 || !strings.Contains(broadcasts[0].Text, "모습이 서서히 드러납니다") ||
		!strings.Contains(broadcasts[1].Text, "창격술") {
		t.Fatalf("broadcasts = %+v, want reveal and damage broadcasts", broadcasts)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", changCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active chang cooldown", used, remaining)
	}
}

func TestChangHandlerRejectsInvalidStates(t *testing.T) {
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
				alice.Stats["class"] = model.ClassInvincible
				loaded.Creatures[alice.ID] = alice
			},
			want: "초인 이상만 쓸수 있는 기술입니다.",
		},
		{
			name: "wrong weapon",
			mutate: func(loaded *worldload.World) {
				proto := loaded.ObjectPrototypes["prototype:weapon"]
				proto.Properties["type"] = "1"
				loaded.ObjectPrototypes[proto.ID] = proto
			},
			want: "창 종류의 무기가 필요합니다.",
		},
		{
			name: "no targets",
			mutate: func(loaded *worldload.World) {
				for id, creature := range loaded.Creatures {
					if id == "creature:alice" {
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
				if err := world.SetCreatureCooldown("creature:alice", changCooldownKey, time.Now().Unix(), utilityAreaSkillCooldownSeconds(model.Creature{Stats: map[string]int{"dexterity": 24}})); err != nil {
					t.Fatalf("SetCreatureCooldown() error = %v", err)
				}
			},
			want: "기다리세요.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := utilityCombatWorld(t, model.ClassCaretaker, legacyObjectPole)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
	defer world.Close()
			if tt.setup != nil {
				tt.setup(world)
			}
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewChangHandler(world)(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestChangHandlerRespectsCooldownBeforeWeaponTargetAndReveal(t *testing.T) {
	loaded := utilityCombatWorld(t, model.ClassCaretaker, legacyObjectPole)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS")
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	proto := loaded.ObjectPrototypes["prototype:weapon"]
	proto.Properties["type"] = "1"
	loaded.ObjectPrototypes[proto.ID] = proto
	world := state.NewWorld(loaded)
	defer world.Close()
	if err := world.SetCreatureCooldown("creature:alice", changCooldownKey, time.Now().Unix(), 10); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewChangHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "기다리세요") {
		t.Fatalf("status/output = %d/%q, want please-wait before weapon checks", status, out)
	}
	for _, unexpected := range []string{"창 종류", "공격할 적", "모습이 서서히"} {
		if strings.Contains(out, unexpected) {
			t.Fatalf("output = %q, did not want %q before cooldown", out, unexpected)
		}
	}
	updated, _ := world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "PHIDDN") ||
		!hasAnyNormalizedFlag(updated.Metadata.Tags, "invisible", "PINVIS") ||
		updated.Stats["PHIDDN"] != 1 || updated.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want reveal state unchanged", updated.Metadata.Tags, updated.Stats)
	}
}

func TestChangHandlerNoTargetsDoesNotConsumeCooldown(t *testing.T) {
	loaded := utilityCombatWorld(t, model.ClassCaretaker, legacyObjectPole)
	for id, creature := range loaded.Creatures {
		if id == "creature:alice" {
			continue
		}
		creature.Metadata.Tags = []string{"MUNKIL"}
		loaded.Creatures[id] = creature
	}
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewChangHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "공격할 적이 없습니다") {
		t.Fatalf("status/output = %d/%q, want no-target rejection", status, ctx.OutputString())
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", changCooldownKey, time.Now().Unix(), 0); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want no consumed cooldown", used, remaining)
	}
}

func TestChangHandlerFailureUsesCooldownWithoutDamage(t *testing.T) {
	withAttackRolls(t, 22)
	world := state.NewWorld(utilityCombatWorld(t, model.ClassCaretaker, legacyObjectPole))
	defer world.Close()
	handler := NewChangHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "기세에 눌려 실패했습니다") {
		t.Fatalf("status/output = %d/%q, want chang failure", status, ctx.OutputString())
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 80; got != want {
		t.Fatalf("goblin hp = %d, want unchanged %d", got, want)
	}
	enemies, err := world.CreatureEnemies("creature:goblin")
	if err != nil {
		t.Fatalf("CreatureEnemies() error = %v", err)
	}
	if len(enemies) != 1 || enemies[0] != "Alice" {
		t.Fatalf("goblin enemies = %+v, want Alice from failed chang", enemies)
	}
	if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "was_attacked") {
		t.Fatalf("goblin tags = %+v, want was_attacked after failed chang", goblin.Metadata.Tags)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", changCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active chang cooldown", used, remaining)
	}
}

func TestChangHandlerFailureCanBreakDamagedWeaponAndKeepsCooldown(t *testing.T) {
	withAttackRolls(t, 22, 1)
	loaded := utilityCombatWorld(t, model.ClassCaretaker, legacyObjectPole)
	proto := loaded.ObjectPrototypes["prototype:weapon"]
	proto.Properties["shotsCurrent"] = "1"
	proto.Properties["shotsMax"] = "4"
	loaded.ObjectPrototypes[proto.ID] = proto
	world := state.NewWorld(loaded)
	defer world.Close()

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := NewChangHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "기세에 눌려 실패했습니다") || !strings.Contains(out, "무기가 부서집니다") {
		t.Fatalf("status/output = %d/%q, want failure and weapon break", status, out)
	}
	if _, ok := world.Object("object:weapon"); ok {
		t.Fatal("object:weapon still exists, want failure break to delete wielded weapon")
	}
	alice, _ := world.Creature("creature:alice")
	if got := alice.Equipment["wield"]; !got.IsZero() {
		t.Fatalf("alice wield = %q, want cleared after weapon break", got)
	}
	if len(broadcasts) < 2 || !strings.Contains(broadcasts[len(broadcasts)-1].Text, "무기가 부서집니다") {
		t.Fatalf("broadcasts = %+v, want weapon break broadcast", broadcasts)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", changCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active cooldown after failure break", used, remaining)
	}
}

func TestChangHandlerFailureDoesNotBreakShatterproofWeapon(t *testing.T) {
	withAttackRolls(t, 22)
	loaded := utilityCombatWorld(t, model.ClassCaretaker, legacyObjectPole)
	proto := loaded.ObjectPrototypes["prototype:weapon"]
	proto.Properties["shotsCurrent"] = "1"
	proto.Properties["shotsMax"] = "4"
	proto.Properties["ONSHAT"] = "1"
	loaded.ObjectPrototypes[proto.ID] = proto
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewChangHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || strings.Contains(ctx.OutputString(), "무기가 부서집니다") {
		t.Fatalf("status/output = %d/%q, want no shatterproof break", status, ctx.OutputString())
	}
	if _, ok := world.Object("object:weapon"); !ok {
		t.Fatal("object:weapon missing, want shatterproof weapon retained")
	}
}

func TestChangHandlerWeaponBreakOnSuccessDoesNotStartCooldown(t *testing.T) {
	withAttackRolls(t, 1)
	loaded := utilityCombatWorld(t, model.ClassCaretaker, legacyObjectPole)
	proto := loaded.ObjectPrototypes["prototype:weapon"]
	proto.Properties["shotsCurrent"] = "1"
	loaded.ObjectPrototypes[proto.ID] = proto
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewChangHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "부서져 버렸습니다.") {
		t.Fatalf("status/output = %d/%q, want weapon break", status, out)
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 80; got != want {
		t.Fatalf("goblin hp = %d, want unchanged after weapon break %d", got, want)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", changCooldownKey, time.Now().Unix(), 0); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want no cooldown after break-return", used, remaining)
	}
}

func TestChoiHandlerRangerDamagesMonsterConsumesMissileChargeAndStartsCooldown(t *testing.T) {
	withAttackRolls(t, 1, 2, 1, 2, 1)
	loaded := utilityCombatWorld(t, model.ClassRanger, legacyObjectMissile)
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 50
	alice.Stats["level"] = 50
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewChoiHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "최루탄로 고블린에게 13점의 피해") {
		t.Fatalf("status/output = %d/%q, want choi damage", status, ctx.OutputString())
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 67; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	if hasAnyNormalizedFlag(goblin.Metadata.Tags, "PCHOI", "choi") {
		t.Fatalf("goblin tags = %+v, want C choi damage without status marker", goblin.Metadata.Tags)
	}
	weapon, _ := world.Object("object:weapon")
	if got, _ := objectIntProperty(world, weapon, "shotsCurrent"); got != 1 {
		t.Fatalf("weapon shotsCurrent = %d, want 1", got)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", choiCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active choi cooldown", used, remaining)
	}
}

func TestChoiHandlerRejectsInvalidStates(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*worldload.World)
		want   string
	}{
		{
			name: "low ranger level",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["class"] = model.ClassRanger
				alice.Level = 49
				alice.Stats["level"] = 49
				loaded.Creatures[alice.ID] = alice
			},
			want: "포졸 레벨 50이상만 쓸수 있는 기술입니다.",
		},
		{
			name: "untrained invincible",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["class"] = model.ClassInvincible
				alice.Metadata.Tags = nil
				loaded.Creatures[alice.ID] = alice
			},
			want: "아직 포졸을 무적수련하지 않았습니다.",
		},
		{
			name: "wrong weapon",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["class"] = model.ClassRanger
				alice.Level = 50
				alice.Stats["level"] = 50
				loaded.Creatures[alice.ID] = alice
				proto := loaded.ObjectPrototypes["prototype:weapon"]
				proto.Properties["type"] = "3"
				loaded.ObjectPrototypes[proto.ID] = proto
			},
			want: "활종류의 무기가 필요합니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := utilityCombatWorld(t, model.ClassRanger, legacyObjectMissile)
			tt.mutate(loaded)
			world := state.NewWorld(loaded)
	defer world.Close()
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewChoiHandler(world)(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestRmBlind2HandlerClearsBlindCostsMPAndBroadcasts(t *testing.T) {
	loaded := rmBlind2World(t)
	world := state.NewWorld(loaded)
	defer world.Close()
	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)

	status, err := NewRmBlind2Handler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "눈이 다시 떠집니다") {
		t.Fatalf("status/output = %d/%q, want blind cleanse", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got, want := creatureStat(alice, "mpCurrent"), 5; got != want {
		t.Fatalf("mpCurrent = %d, want %d", got, want)
	}
	if statusEffectActive(model.Player{}, alice, "blind", "blinded", "PBLIND", "MBLIND") {
		t.Fatalf("alice tags/stats = %+v/%+v, want blind cleared", alice.Metadata.Tags, alice.Stats)
	}
	player, _ := world.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "blind", "blinded", "PBLIND", "MBLIND") {
		t.Fatalf("player tags = %+v, want blind cleared", player.Metadata.Tags)
	}
	if len(broadcasts) != 1 || !strings.Contains(broadcasts[0].Text, "개안부") {
		t.Fatalf("broadcasts = %+v, want rm_blind2 broadcast", broadcasts)
	}
}

func TestRmBlind2HandlerRejectsInvalidStates(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		mutate func(*worldload.World)
		want   string
	}{
		{
			name: "low mp",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["mpCurrent"] = 19
				loaded.Creatures[alice.ID] = alice
			},
			want: "도력이 부족합니다",
		},
		{
			name: "missing yellowi",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"PBLIND"}
				loaded.Creatures[alice.ID] = alice
			},
			want: "그런능력이 없습니다",
		},
		{
			name: "not blind",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"YELLOWI"}
				alice.Stats["PBLIND"] = 0
				loaded.Creatures[alice.ID] = alice
				player := loaded.Players["player:alice"]
				player.Metadata.Tags = nil
				loaded.Players[player.ID] = player
			},
			want: "실명이 되었을때만",
		},
		{
			name: "target args",
			args: []string{"Bob"},
			want: "자신 치료 기술입니다",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := rmBlind2World(t)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
	defer world.Close()
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewRmBlind2Handler(world)(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestUtilityCombatSkillHandlersCanBeRegisteredByDispatcherAliases(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		loaded func(*testing.T) *worldload.World
		rolls  []int
		want   string
	}{
		{
			name: "chang korean",
			line: "창격술",
			loaded: func(t *testing.T) *worldload.World {
				return utilityCombatWorld(t, model.ClassCaretaker, legacyObjectPole)
			},
			rolls: []int{1, 1, 1},
			want:  "창격술로 고블린에게",
		},
		{
			name: "chang handler",
			line: "chang",
			loaded: func(t *testing.T) *worldload.World {
				return utilityCombatWorld(t, model.ClassCaretaker, legacyObjectPole)
			},
			rolls: []int{1, 1, 1},
			want:  "창격술로 고블린에게",
		},
		{
			name: "choi korean",
			line: "최루탄",
			loaded: func(t *testing.T) *worldload.World {
				return utilityCombatWorld(t, model.ClassRanger, legacyObjectMissile)
			},
			rolls: []int{1, 1, 1, 1, 1},
			want:  "최루탄로 고블린에게",
		},
		{
			name: "choi handler",
			line: "choi",
			loaded: func(t *testing.T) *worldload.World {
				return utilityCombatWorld(t, model.ClassRanger, legacyObjectMissile)
			},
			rolls: []int{1, 1, 1, 1, 1},
			want:  "최루탄로 고블린에게",
		},
		{
			name:   "rm_blind2 korean",
			line:   "실명해소술",
			loaded: rmBlind2World,
			want:   "눈이 다시 떠집니다",
		},
		{
			name:   "rm_blind2 handler",
			line:   "rm_blind2",
			loaded: rmBlind2World,
			want:   "눈이 다시 떠집니다",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.rolls) > 0 {
				withAttackRolls(t, tt.rolls...)
			}
			world := state.NewWorld(tt.loaded(t))
	defer world.Close()
			dispatcher := utilityCombatSkillDispatcher(t, world)
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

func utilityCombatSkillDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()
	return Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "창격술", Number: 174, Handler: "chang"},
			{Name: "chang", Number: 174, Handler: "chang"},
			{Name: "실명해소술", Number: 176, Handler: "rm_blind2"},
			{Name: "rm_blind2", Number: 176, Handler: "rm_blind2"},
			{Name: "최루탄", Number: 177, Handler: "choi"},
			{Name: "choi", Number: 177, Handler: "choi"},
		}),
		Handlers: map[string]Handler{
			"chang":     NewChangHandler(world),
			"choi":      NewChoiHandler(world),
			"rm_blind2": NewRmBlind2Handler(world),
		},
	}
}

func utilityCombatWorld(t *testing.T, class int, weaponType int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:arena",
		DisplayName: "Arena",
		PlayerIDs:   []model.PlayerID{"player:alice"},
		CreatureIDs: []model.CreatureID{
			"creature:goblin",
			"creature:orc",
			"creature:hidden",
			"creature:protected",
		},
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
		Level:       80,
		Equipment:   map[string]model.ObjectInstanceID{"wield": "object:weapon"},
		Stats: map[string]int{
			"class":     class,
			"level":     80,
			"thaco":     0,
			"dexterity": 24,
			"piety":     20,
			"hpCurrent": 100,
			"hpMax":     100,
			"mpCurrent": 50,
			"mpMax":     50,
			"pDice":     5,
		},
		Metadata: model.Metadata{Tags: []string{"SRANGER"}},
	})
	for _, creature := range []model.Creature{
		{
			ID:          "creature:goblin",
			Kind:        model.CreatureKindMonster,
			DisplayName: "고블린",
			RoomID:      "room:arena",
			Stats:       map[string]int{"level": 10, "thaco": 20, "hpCurrent": 80, "hpMax": 80},
		},
		{
			ID:          "creature:orc",
			Kind:        model.CreatureKindMonster,
			DisplayName: "오크",
			RoomID:      "room:arena",
			Stats:       map[string]int{"level": 10, "thaco": 20, "hpCurrent": 80, "hpMax": 80},
		},
		{
			ID:          "creature:hidden",
			Kind:        model.CreatureKindMonster,
			DisplayName: "숨은 적",
			RoomID:      "room:arena",
			Stats:       map[string]int{"level": 10, "thaco": 20, "hpCurrent": 80, "hpMax": 80},
			Metadata:    model.Metadata{Tags: []string{"hidden"}},
		},
		{
			ID:          "creature:protected",
			Kind:        model.CreatureKindMonster,
			DisplayName: "불사 적",
			RoomID:      "room:arena",
			Stats:       map[string]int{"level": 10, "thaco": 20, "hpCurrent": 80, "hpMax": 80},
			Metadata:    model.Metadata{Tags: []string{"MUNKIL"}},
		},
	} {
		mustAddLookCreature(t, loaded, creature)
	}
	weaponPDice := "2"
	properties := map[string]string{
		"type":  formatInt(weaponType),
		"pDice": weaponPDice,
	}
	if weaponType == legacyObjectMissile {
		weaponPDice = "4"
		properties["pDice"] = weaponPDice
		properties["shotsCurrent"] = "2"
	}
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:weapon",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "수련 무기",
		Properties:  properties,
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:weapon",
		PrototypeID: "prototype:weapon",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "wield"},
	})
	return loaded
}

func rmBlind2World(t *testing.T) *worldload.World {
	t.Helper()
	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:temple",
		DisplayName: "사당",
		PlayerIDs:   []model.PlayerID{"player:alice"},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:temple",
		Metadata:    model.Metadata{Tags: []string{"blind"}},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:temple",
		Stats: map[string]int{
			"class":     model.ClassCaretaker,
			"level":     80,
			"hpCurrent": 50,
			"hpMax":     50,
			"mpCurrent": 25,
			"mpMax":     25,
			"PBLIND":    1,
		},
		Metadata: model.Metadata{Tags: []string{"YELLOWI", "blind"}},
	})
	return loaded
}
