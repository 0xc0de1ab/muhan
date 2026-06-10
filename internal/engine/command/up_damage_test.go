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

func TestUpDamageHandlerSuccessBoostsBarbarianStartsCooldownAndExpiration(t *testing.T) {
	world := state.NewWorld(upDamageWorld(t, model.ClassBarbarian, 50))
	defer world.Close()
	handler := NewUpDamageHandler(world, fixedRoll(1))
	var broadcasts []roomBroadcastRecord

	before := time.Now().Unix()
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "잠력을 격발시킵니다") {
		t.Fatalf("status/output = %d/%q, want up damage success", status, ctx.OutputString())
	}
	if got, want := ctx.OutputString(), "당신은 자신의 혈도를 짚으며 몸의 잠력을 격발시킵니다.\n온몸으로 기가 퍼져나가는것을 느낍니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}

	creature, _ := world.Creature("creature:alice")
	wantStats := map[string]int{
		"pDice":     5,
		"hpMax":     150,
		"mpMax":     70,
		"hpCurrent": 150,
		"mpCurrent": 70,
	}
	for key, want := range wantStats {
		if got := creatureStat(creature, key); got != want {
			t.Fatalf("%s = %d, want %d", key, got, want)
		}
	}
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "PUPDMG", "upDamage", "upDmg") {
		t.Fatalf("creature tags = %+v, want up damage status", creature.Metadata.Tags)
	}
	player, _ := world.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "PUPDMG", "upDamage", "upDmg") {
		t.Fatalf("player tags = %+v, want up damage status", player.Metadata.Tags)
	}
	if len(broadcasts) != 1 || broadcasts[0].Text != "Alice이 자신의 혈도를 짚으며 힘을 끌어들입니다." {
		t.Fatalf("broadcasts = %+v, want up damage broadcast", broadcasts)
	}
	assertCreatureEffectExpiration(t, world, "PUPDMG", before, upDamageStatusDurationSeconds)

	remaining, used, err := world.UseCreatureCooldown("creature:alice", upDamageCooldownKey, time.Now().Unix(), 1)
	if err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	}
	if used || remaining <= 0 {
		t.Fatalf("cooldown = remaining %d used %v, want active", remaining, used)
	}
}

func TestUpDamageHandlerSuccessBoostsInvincibleWithTraining(t *testing.T) {
	loaded := upDamageWorld(t, model.ClassInvincible, 80)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SBARBARIAN"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	defer world.Close()
	handler := NewUpDamageHandler(world, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "잠력을 격발시킵니다") {
		t.Fatalf("status/output = %d/%q, want success", status, ctx.OutputString())
	}
	creature, _ := world.Creature("creature:alice")
	wantStats := map[string]int{
		"pDice":     6,
		"hpMax":     200,
		"mpMax":     150,
		"hpCurrent": 200,
		"mpCurrent": 150,
	}
	for key, want := range wantStats {
		if got := creatureStat(creature, key); got != want {
			t.Fatalf("%s = %d, want %d", key, got, want)
		}
	}
}

func TestUpDamageHandlerRejectsInvalidStates(t *testing.T) {
	tests := []struct {
		name  string
		class int
		level int
		tags  []string
		setup func(*state.World)
		want  string
	}{
		{
			name:  "wrong class",
			class: model.ClassFighter,
			level: 50,
			want:  "권법가 레벨 50이상만 쓸수 있는 기술입니다.",
		},
		{
			name:  "barbarian below level 50",
			class: model.ClassBarbarian,
			level: 49,
			want:  "권법가 레벨 50이상만 쓸수 있는 기술입니다.",
		},
		{
			name:  "invincible without barbarian training",
			class: model.ClassInvincible,
			level: 80,
			want:  "아직 권법가를 무적수련하지 않았습니다.",
		},
		{
			name:  "already active",
			class: model.ClassBarbarian,
			level: 50,
			tags:  []string{"PUPDMG"},
			want:  "당신은 지금 잠력격발을 사용중입니다.",
		},
		{
			name:  "cooldown active",
			class: model.ClassBarbarian,
			level: 50,
			setup: func(world *state.World) {
				if err := world.SetCreatureCooldown("creature:alice", upDamageCooldownKey, time.Now().Unix(), upDamageSuccessCooldownSeconds); err != nil {
					t.Fatalf("SetCreatureCooldown() error = %v", err)
				}
			},
			want: "20분",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := upDamageWorld(t, tt.class, tt.level)
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = tt.tags
			loaded.Creatures[alice.ID] = alice
			world := state.NewWorld(loaded)
	defer world.Close()
			if tt.setup != nil {
				tt.setup(world)
			}
			handler := NewUpDamageHandler(world, fixedRoll(1))

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

func TestUpDamageHandlerFailureSetsShortCooldownWithoutBoost(t *testing.T) {
	world := state.NewWorld(upDamageWorld(t, model.ClassBarbarian, 50))
	defer world.Close()
	handler := NewUpDamageHandler(world, fixedRoll(100))
	var broadcasts []roomBroadcastRecord

	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "힘을 격발시키는데 실패했습니다.\n" {
		t.Fatalf("status/output = %d/%q, want failure", status, ctx.OutputString())
	}
	creature, _ := world.Creature("creature:alice")
	for key, want := range map[string]int{"pDice": 3, "hpMax": 100, "mpMax": 50, "hpCurrent": 80, "mpCurrent": 30} {
		if got := creatureStat(creature, key); got != want {
			t.Fatalf("%s = %d, want unchanged %d", key, got, want)
		}
	}
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "PUPDMG", "upDamage", "upDmg") {
		t.Fatalf("creature tags = %+v, want no up damage status", creature.Metadata.Tags)
	}
	assertNoCreatureEffectExpiration(t, world, "PUPDMG")
	if len(broadcasts) != 1 || broadcasts[0].Text != "Alice이 잠력격발을 시도합니다." {
		t.Fatalf("broadcasts = %+v, want failure broadcast", broadcasts)
	}

	remaining, used, err := world.UseCreatureCooldown("creature:alice", upDamageCooldownKey, time.Now().Unix(), 1)
	if err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	}
	if used || remaining < 1 || remaining > upDamageFailureCooldownSeconds {
		t.Fatalf("failure cooldown = remaining %d used %v, want short active cooldown", remaining, used)
	}
}

func TestUpDamageHandlerCanBeRegisteredByDispatcherAliases(t *testing.T) {
	for _, line := range []string{"잠력격발", "up_dmg"} {
		t.Run(line, func(t *testing.T) {
			world := state.NewWorld(upDamageWorld(t, model.ClassBarbarian, 50))
	defer world.Close()
			dispatcher := Dispatcher{
				Registry: mustRegistry(t, []commandspec.CommandSpec{
					{Name: "잠력격발", Number: 99, Handler: "up_dmg"},
					{Name: "up_dmg", Number: 99, Handler: "up_dmg"},
				}),
				Handlers: map[string]Handler{
					"up_dmg": NewUpDamageHandler(world, fixedRoll(1)),
				},
			}

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, line)
			if err != nil {
				t.Fatalf("DispatchLine(%q) error = %v", line, err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), "잠력을 격발시킵니다") {
				t.Fatalf("status/output = %d/%q, want dispatch success", status, ctx.OutputString())
			}
		})
	}
}

func upDamageWorld(t *testing.T, class int, level int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:up-damage",
		DisplayName: "수련장",
		PlayerIDs:   []model.PlayerID{"player:alice"},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:up-damage",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:up-damage",
		Level:       level,
		Stats: map[string]int{
			"class":     class,
			"level":     level,
			"dexterity": 40,
			"pDice":     3,
			"hpMax":     100,
			"hpCurrent": 80,
			"mpMax":     50,
			"mpCurrent": 30,
		},
	})
	return loaded
}
