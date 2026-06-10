package command

import (
	"strings"
	"testing"
	"time"

	"muhan/internal/commandspec"
	model "muhan/internal/world/model"
	state "muhan/internal/world/state"
)

func TestSneakHandlerSuccessMovesPlayer(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{
		"class":     model.ClassThief,
		"level":     50,
		"dexterity": 40,
	}
	alice.Metadata.Tags = []string{"hidden"}
	loaded.Creatures[alice.ID] = alice

	world := state.NewWorld(loaded)
	defer world.Close()
	// Success roll (roll(1, 100) returns 50, which is <= sneakChance)
	handler := NewSneakHandler(world, fixedRoll(50))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Handler: "sneak"},
		Args: []string{"동"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}

	player, _ := world.Player("player:alice")
	if player.RoomID != "room:east" {
		t.Fatalf("player room id = %q, want room:east", player.RoomID)
	}
	origin, ok := world.Room("room:plaza")
	if !ok {
		t.Fatal("origin room missing")
	}
	if got := origin.Properties["track"]; got != "동" {
		t.Fatalf("origin track = %q, want 동", got)
	}

	// Should still be hidden
	creature, _ := world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "hidden") {
		t.Fatalf("creature lost hidden flag on successful sneak")
	}
}

func TestSneakHandlerSuccessChecksDestinationTrapLikeLegacy(t *testing.T) {
	withMoveTrapRolls(t, 100, 4)
	loaded := moveTrapWorld(t, "2", "")
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{
		"class":     model.ClassThief,
		"level":     50,
		"dexterity": 0,
		"hpCurrent": 20,
		"hpMax":     20,
	}
	alice.Metadata.Tags = []string{"hidden"}
	loaded.Creatures[alice.ID] = alice

	world := state.NewWorld(loaded)
	defer world.Close()
	ctx := &Context{ActorID: "player:alice"}
	status, err := NewSneakHandler(world, fixedRoll(1))(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Handler: "sneak"},
		Args: []string{"동"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}

	player, _ := world.Player("player:alice")
	if player.RoomID != "room:east" {
		t.Fatalf("player room id = %q, want room:east", player.RoomID)
	}
	creature, _ := world.Creature("creature:alice")
	if got := creature.Stats["hpCurrent"]; got != 16 {
		t.Fatalf("hpCurrent = %d, want 16", got)
	}
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "poison", "PPOISN") {
		t.Fatalf("creature tags = %+v, want poison/PPOISN", creature.Metadata.Tags)
	}
	player, _ = world.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "poison", "PPOISN") {
		t.Fatalf("player tags = %+v, want poison/PPOISN", player.Metadata.Tags)
	}
	assertMoveTrapOutputOrder(t, ctx.OutputString(),
		"\n동쪽\n\n",
		"당신은 숨겨진 독화살에 맞았습니다!\n",
		"당신은 4점의 피해를 입었습니다.\n",
	)
}

func TestSneakHandlerFailureBlocksEnemyMonsterOnlyWithoutPINVISLikeLegacy(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{
		"class":     model.ClassThief,
		"level":     10,
		"dexterity": 15,
		"PHIDDN":    1,
	}
	alice.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Players[player.ID] = player

	// Add blocking monster
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:blocker",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:plaza",
		Metadata:    model.Metadata{Tags: []string{"blocksExits"}},
	})

	world := state.NewWorld(loaded)
	defer world.Close()
	if _, err := world.AddEnemy("creature:blocker", "creature:alice"); err != nil {
		t.Fatalf("AddEnemy() error = %v", err)
	}
	// Fail roll (roll(1, 100) returns 90, which is > sneakChance)
	handler := NewSneakHandler(world, fixedRoll(90))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Handler: "sneak"},
		Args: []string{"동"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}

	// Should not have moved
	player, _ = world.Player("player:alice")
	if player.RoomID != "room:plaza" {
		t.Fatalf("player room id = %q, want room:plaza", player.RoomID)
	}

	// Should have lost hidden flag
	creature, _ := world.Creature("creature:alice")
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "hidden") {
		t.Fatalf("creature still has hidden flag on failed sneak")
	}
	if creature.Stats["PHIDDN"] != 0 {
		t.Fatalf("creature PHIDDN = %d, want 0 after failed sneak", creature.Stats["PHIDDN"])
	}
	player, _ = world.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "PHIDDN") {
		t.Fatalf("player tags = %+v, want hidden cleared after failed sneak", player.Metadata.Tags)
	}

	if !strings.Contains(ctx.OutputString(), "고블린가 당신의 길을 가로막습니다.") {
		t.Fatalf("output missing block message: %q", ctx.OutputString())
	}
}

func TestSneakHandlerFailureIgnoresEnemyBlockerWithPINVISLikeLegacy(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{
		"class":     model.ClassThief,
		"level":     10,
		"dexterity": 15,
		"PHIDDN":    1,
	}
	alice.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Creatures[alice.ID] = alice

	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:blocker",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:plaza",
		Metadata:    model.Metadata{Tags: []string{"blocksExits"}},
	})

	world := state.NewWorld(loaded)
	defer world.Close()
	if _, err := world.AddEnemy("creature:blocker", "creature:alice"); err != nil {
		t.Fatalf("AddEnemy() error = %v", err)
	}
	handler := NewSneakHandler(world, fixedRoll(90))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Handler: "sneak"},
		Args: []string{"동"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}

	player, _ := world.Player("player:alice")
	if player.RoomID != "room:east" {
		t.Fatalf("player room id = %q, want room:east", player.RoomID)
	}
	if strings.Contains(ctx.OutputString(), "당신의 길을 가로막습니다.") {
		t.Fatalf("unexpected block message: %q", ctx.OutputString())
	}
}

func TestSneakHandlerFailureIgnoresNonEnemyBlockingMonsterLikeLegacy(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{
		"class":     model.ClassThief,
		"level":     10,
		"dexterity": 15,
		"PHIDDN":    1,
	}
	alice.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Creatures[alice.ID] = alice

	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:blocker",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:plaza",
		Metadata:    model.Metadata{Tags: []string{"blocksExits"}},
	})

	world := state.NewWorld(loaded)
	defer world.Close()
	handler := NewSneakHandler(world, fixedRoll(90))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Handler: "sneak"},
		Args: []string{"동"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}

	player, _ := world.Player("player:alice")
	if player.RoomID != "room:east" {
		t.Fatalf("player room id = %q, want room:east", player.RoomID)
	}
	creature, _ := world.Creature("creature:alice")
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "hidden") {
		t.Fatalf("creature still has hidden flag on failed sneak")
	}
	if strings.Contains(ctx.OutputString(), "당신의 길을 가로막습니다.") {
		t.Fatalf("unexpected block message: %q", ctx.OutputString())
	}
}

func TestSneakHandlerFailureIgnoresInternalIDEnemyNamesLikeLegacy(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{
		"class":     model.ClassThief,
		"level":     10,
		"dexterity": 15,
	}
	alice.Metadata.Tags = []string{"hidden"}
	loaded.Creatures[alice.ID] = alice

	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:blocker",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:plaza",
		Metadata:    model.Metadata{Tags: []string{"blocksExits"}},
	})

	world := &sneakInternalIDEnemyWorld{
		World:   state.NewWorld(loaded),
		enemies: []string{"alice", "player:alice", "creature:alice"},
	}
	handler := NewSneakHandler(world, fixedRoll(90))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Handler: "sneak"},
		Args: []string{"동"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}

	player, _ := world.Player("player:alice")
	if player.RoomID != "room:east" {
		t.Fatalf("player room id = %q, want room:east", player.RoomID)
	}
	if strings.Contains(ctx.OutputString(), "당신의 길을 가로막습니다.") {
		t.Fatalf("unexpected block message: %q", ctx.OutputString())
	}
}

type sneakInternalIDEnemyWorld struct {
	*state.World
	enemies []string
}

func (w *sneakInternalIDEnemyWorld) CreatureEnemies(creatureID model.CreatureID) ([]string, error) {
	if creatureID != "creature:blocker" {
		return w.World.CreatureEnemies(creatureID)
	}
	return append([]string(nil), w.enemies...), nil
}

func TestSneakHandlerAttackCooldownBlocksBeforeHiddenClearLikeLegacy(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{
		"class":     model.ClassThief,
		"level":     10,
		"dexterity": 15,
		"PHIDDN":    1,
	}
	alice.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Creatures[alice.ID] = alice

	world := state.NewWorld(loaded)
	defer world.Close()
	now := time.Now().Unix()
	if err := world.SetCreatureCooldown("creature:alice", "attack", now, 5); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}
	handler := NewSneakHandler(world, fixedRoll(90))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Handler: "sneak"},
		Args: []string{"동"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if got := ctx.OutputString(); !strings.Contains(got, "기다리세요.") || strings.Contains(got, "은신술을 사용하는데 실패") {
		t.Fatalf("output = %q, want only please_wait", got)
	}
	player, _ := world.Player("player:alice")
	if player.RoomID != "room:plaza" {
		t.Fatalf("player room id = %q, want room:plaza", player.RoomID)
	}
	creature, _ := world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "hidden") {
		t.Fatalf("creature lost hidden flag despite active attack cooldown")
	}
	if creature.Stats["PHIDDN"] != 1 {
		t.Fatalf("creature PHIDDN = %d, want retained during attack cooldown", creature.Stats["PHIDDN"])
	}
}

func TestSneakHandlerUsesLegacyExitBlockMessages(t *testing.T) {
	tests := []struct {
		name       string
		flags      []string
		carry      bool
		wantOutput string
	}{
		{
			name:       "locked",
			flags:      []string{"locked"},
			wantOutput: "그 출구는 잠겨져 있습니다.\n",
		},
		{
			name:       "closed",
			flags:      []string{"closed"},
			wantOutput: "먼저 문을 열어야 겠군요.\n",
		},
		{
			name:       "naked",
			flags:      []string{"naked"},
			carry:      true,
			wantOutput: "그 쪽으로는 뭘 들고는 갈 수 없습니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithEastExit(t, tt.flags, "room:east")
			alice := loaded.Creatures["creature:alice"]
			alice.Stats = map[string]int{
				"class":     model.ClassThief,
				"level":     10,
				"dexterity": 15,
			}
			alice.Metadata.Tags = []string{"hidden"}
			loaded.Creatures[alice.ID] = alice
			if tt.carry {
				addMoveCreatureObjectRef(t, loaded, "object:carried-sneak", "inventory", true, false)
			}

			world := state.NewWorld(loaded)
	defer world.Close()
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewSneakHandler(world, fixedRoll(1))(ctx, ResolvedCommand{
				Spec: commandspec.CommandSpec{Handler: "sneak"},
				Args: []string{"동"},
			})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want default", status)
			}
			if got := ctx.OutputString(); got != tt.wantOutput {
				t.Fatalf("output = %q, want %q", got, tt.wantOutput)
			}
			player, _ := world.Player("player:alice")
			if player.RoomID != "room:plaza" {
				t.Fatalf("player room id = %q, want room:plaza", player.RoomID)
			}
		})
	}
}

func TestSneakHandlerUsesLegacySpecialExitBlockMessages(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		maleActor bool
		guard     bool
		want      string
	}{
		{
			name: "fly required",
			flag: "XFLYSP",
			want: "그 쪽에는 날아서만 갈 수 있습니다.\n",
		},
		{
			name:      "female only",
			flag:      "XFEMAL",
			maleActor: true,
			want:      "그 쪽으로는 여성만 갈 수 있습니다.\n",
		},
		{
			name: "male only",
			flag: "XMALES",
			want: "그 쪽으로는 남성만 갈 수 있습니다.\n",
		},
		{
			name:  "passive guard",
			flag:  "XPGUAR",
			guard: true,
			want:  "고블린이 당신의 길을 가로막습니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithEastExit(t, []string{tt.flag}, "room:east")
			alice := loaded.Creatures["creature:alice"]
			alice.Stats = map[string]int{
				"class":     model.ClassThief,
				"level":     10,
				"dexterity": 15,
			}
			alice.Metadata.Tags = []string{"hidden"}
			if tt.maleActor {
				alice.Metadata.Tags = append(alice.Metadata.Tags, "PMALES")
			}
			loaded.Creatures[alice.ID] = alice
			if tt.guard {
				mustAddLookCreature(t, loaded, model.Creature{
					ID:          "creature:passive-guard",
					Kind:        model.CreatureKindMonster,
					DisplayName: "고블린",
					RoomID:      "room:plaza",
					Metadata:    model.Metadata{Tags: []string{"MPGUAR"}},
				})
			}

			world := state.NewWorld(loaded)
	defer world.Close()
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewSneakHandler(world, fixedRoll(1))(ctx, ResolvedCommand{
				Spec: commandspec.CommandSpec{Handler: "sneak"},
				Args: []string{"동"},
			})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			player, _ := world.Player("player:alice")
			if player.RoomID != "room:plaza" {
				t.Fatalf("player room id = %q, want room:plaza", player.RoomID)
			}
		})
	}
}

func TestSneakHandlerUsesLegacyTimeRestrictedExitMessages(t *testing.T) {
	tests := []struct {
		name       string
		flag       string
		legacyTime int64
		want       string
	}{
		{
			name:       "night only during day",
			flag:       "XNGHTO",
			legacyTime: 12,
			want:       "그 출구는 밤에만 갈 수 있습니다.\n",
		},
		{
			name:       "day only during night",
			flag:       "XDAYON",
			legacyTime: 23,
			want:       "그 출구는 낮에만 갈 수 있습니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithEastExit(t, []string{tt.flag}, "room:east")
			alice := loaded.Creatures["creature:alice"]
			alice.Stats = map[string]int{
				"class":     model.ClassThief,
				"level":     10,
				"dexterity": 15,
			}
			alice.Metadata.Tags = []string{"hidden"}
			loaded.Creatures[alice.ID] = alice

			world := state.NewWorld(loaded)
	defer world.Close()
			world.SetLegacyTime(tt.legacyTime)
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewSneakHandler(world, fixedRoll(1))(ctx, ResolvedCommand{
				Spec: commandspec.CommandSpec{Handler: "sneak"},
				Args: []string{"동"},
			})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			player, _ := world.Player("player:alice")
			if player.RoomID != "room:plaza" {
				t.Fatalf("player room id = %q, want room:plaza", player.RoomID)
			}
		})
	}
}

func TestSneakHandlerAllowsLegacyTimeRestrictedExitAtValidHour(t *testing.T) {
	loaded := moveWorldWithEastExit(t, []string{"XNGHTO"}, "room:east")
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{
		"class":     model.ClassThief,
		"level":     50,
		"dexterity": 40,
	}
	alice.Metadata.Tags = []string{"hidden"}
	loaded.Creatures[alice.ID] = alice

	world := state.NewWorld(loaded)
	defer world.Close()
	world.SetLegacyTime(22)
	ctx := &Context{ActorID: "player:alice"}
	status, err := NewSneakHandler(world, fixedRoll(1))(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Handler: "sneak"},
		Args: []string{"동"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	player, _ := world.Player("player:alice")
	if player.RoomID != "room:east" {
		t.Fatalf("player room id = %q, want room:east", player.RoomID)
	}
	if !strings.Contains(ctx.OutputString(), "\n동쪽\n\n") {
		t.Fatalf("output missing destination room:\n%s", ctx.OutputString())
	}
}

func TestSneakHandlerClimbFallDamagesAndStopsBeforeAttackCooldownLikeLegacy(t *testing.T) {
	withMoveTrapRolls(t, 1, 7)
	loaded := moveWorldWithEastExit(t, []string{"XCLIMB"}, "room:east")
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{
		"class":     model.ClassThief,
		"level":     50,
		"dexterity": 0,
		"hpCurrent": 20,
		"hpMax":     20,
	}
	alice.Metadata.Tags = []string{"hidden"}
	loaded.Creatures[alice.ID] = alice

	world := state.NewWorld(loaded)
	defer world.Close()
	now := time.Now().Unix()
	if err := world.SetCreatureCooldown("creature:alice", "attack", now, 5); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewSneakHandler(world, fixedRoll(1))(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Handler: "sneak"},
		Args: []string{"동"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	player, _ := world.Player("player:alice")
	if player.RoomID != "room:plaza" {
		t.Fatalf("player room id = %q, want room:plaza", player.RoomID)
	}
	creature, _ := world.Creature("creature:alice")
	if got := creature.Stats["hpCurrent"]; got != 13 {
		t.Fatalf("hpCurrent = %d, want 13", got)
	}
	if got := ctx.OutputString(); got != "당신은 떨어져서 7만큼의 상처를 입었습니다.\n" {
		t.Fatalf("output = %q, want sneak climb fall damage only", got)
	}
}

func TestSneakHandlerRepelFallDamagesAndContinuesLikeLegacy(t *testing.T) {
	withMoveTrapRolls(t, 1, 7)
	loaded := moveWorldWithEastExit(t, []string{"XREPEL"}, "room:east")
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{
		"class":     model.ClassThief,
		"level":     50,
		"dexterity": 0,
		"hpCurrent": 20,
		"hpMax":     20,
	}
	alice.Metadata.Tags = []string{"hidden"}
	loaded.Creatures[alice.ID] = alice

	world := state.NewWorld(loaded)
	defer world.Close()
	ctx := &Context{ActorID: "player:alice"}
	status, err := NewSneakHandler(world, fixedRoll(1))(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Handler: "sneak"},
		Args: []string{"동"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	player, _ := world.Player("player:alice")
	if player.RoomID != "room:east" {
		t.Fatalf("player room id = %q, want room:east", player.RoomID)
	}
	creature, _ := world.Creature("creature:alice")
	if got := creature.Stats["hpCurrent"]; got != 13 {
		t.Fatalf("hpCurrent = %d, want 13", got)
	}
	assertMoveTrapOutputOrder(t, ctx.OutputString(),
		"당신은 떨어져서 7만큼의 상처를 입었습니다.\n",
		"\n동쪽\n\n",
	)
}

func TestSneakHandlerClimbFallSkippedWithLevitate(t *testing.T) {
	loaded := moveWorldWithEastExit(t, []string{"XCLIMB"}, "room:east")
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{
		"class":     model.ClassThief,
		"level":     50,
		"dexterity": 0,
		"hpCurrent": 20,
		"hpMax":     20,
	}
	alice.Metadata.Tags = []string{"hidden", "PLEVIT"}
	loaded.Creatures[alice.ID] = alice

	world := state.NewWorld(loaded)
	defer world.Close()
	ctx := &Context{ActorID: "player:alice"}
	status, err := NewSneakHandler(world, fixedRoll(1))(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Handler: "sneak"},
		Args: []string{"동"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	player, _ := world.Player("player:alice")
	if player.RoomID != "room:east" {
		t.Fatalf("player room id = %q, want room:east", player.RoomID)
	}
	creature, _ := world.Creature("creature:alice")
	if got := creature.Stats["hpCurrent"]; got != 20 {
		t.Fatalf("hpCurrent = %d, want unchanged 20", got)
	}
	if !strings.Contains(ctx.OutputString(), "\n동쪽\n\n") {
		t.Fatalf("output missing destination room:\n%s", ctx.OutputString())
	}
}
