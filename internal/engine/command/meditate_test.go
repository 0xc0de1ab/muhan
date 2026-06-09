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

func TestMeditateHandlerSuccessAddsTagsIntelligenceCooldownAndExpiration(t *testing.T) {
	world := state.NewWorld(meditateWorld(t, legacyClassCleric))
	handler := NewMeditateHandler(world, fixedRoll(1))
	var broadcasts []roomBroadcastRecord

	before := time.Now().Unix()
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "참선에 들어갑니다") {
		t.Fatalf("status/output = %d/%q, want meditate success", status, ctx.OutputString())
	}
	if got, want := ctx.OutputString(), "당신은 자리에 앉아 참선에 들어갑니다.\n새롭게 사물을 바라보는 눈이 뜨였습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}

	creature, _ := world.Creature("creature:alice")
	if got, want := creatureStat(creature, "intelligence"), 33; got != want {
		t.Fatalf("intelligence = %d, want %d", got, want)
	}
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "PMEDIT") ||
		!hasAnyNormalizedFlag(creature.Metadata.Tags, "meditate") {
		t.Fatalf("creature tags = %+v, want PMEDIT and meditate", creature.Metadata.Tags)
	}
	player, _ := world.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "PMEDIT") ||
		!hasAnyNormalizedFlag(player.Metadata.Tags, "meditate") {
		t.Fatalf("player tags = %+v, want PMEDIT and meditate", player.Metadata.Tags)
	}
	if len(broadcasts) != 1 || broadcasts[0].Text != "Alice이 자리에 앉아 참선을 행합니다." {
		t.Fatalf("broadcasts = %+v, want meditate broadcast", broadcasts)
	}
	assertCreatureEffectExpiration(t, world, "PMEDIT", before, meditateStatusDurationSeconds(creature))

	remaining, used, err := world.UseCreatureCooldown("creature:alice", meditateCooldownKey, time.Now().Unix(), 1)
	if err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	}
	if used || remaining <= 0 {
		t.Fatalf("cooldown = remaining %d used %v, want active", remaining, used)
	}
}

func TestMeditateHandlerRejectsClassAndAlreadyActive(t *testing.T) {
	tests := []struct {
		name   string
		class  int
		tags   []string
		want   string
		active bool
	}{
		{
			name:  "wrong class",
			class: legacyClassFighter,
			want:  "무사 불제자만 사용할 수 있는 기술입니다.",
		},
		{
			name:  "invincible without training",
			class: legacyClassInvincible,
			want:  "무사나 불제자를 무적수련하지 않았습니다..",
		},
		{
			name:   "already meditating",
			class:  legacyClassPaladin,
			tags:   []string{"PMEDIT"},
			want:   "당신은 벌써 참선을 했습니다.",
			active: true,
		},
		{
			name:   "invincible with paladin training",
			class:  legacyClassInvincible,
			tags:   []string{"SPALADIN"},
			want:   "참선에 들어갑니다",
			active: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := meditateWorld(t, tt.class)
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = tt.tags
			loaded.Creatures[alice.ID] = alice
			world := state.NewWorld(loaded)
			handler := NewMeditateHandler(world, fixedRoll(1))

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			updated, _ := world.Creature("creature:alice")
			if got := hasAnyNormalizedFlag(updated.Metadata.Tags, "PMEDIT", "meditate"); got != tt.active {
				t.Fatalf("active meditate tags = %v, want %v; tags=%+v", got, tt.active, updated.Metadata.Tags)
			}
		})
	}
}

func TestMeditateHandlerFailureSetsShortCooldownWithoutTags(t *testing.T) {
	world := state.NewWorld(meditateWorld(t, legacyClassCleric))
	handler := NewMeditateHandler(world, fixedRoll(100))
	var broadcasts []roomBroadcastRecord

	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "참선도중 주화입마에 빠졌습니다.\n" {
		t.Fatalf("status/output = %d/%q, want meditate failure", status, ctx.OutputString())
	}
	creature, _ := world.Creature("creature:alice")
	if got, want := creatureStat(creature, "intelligence"), 30; got != want {
		t.Fatalf("intelligence = %d, want unchanged %d", got, want)
	}
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "PMEDIT", "meditate") {
		t.Fatalf("creature tags = %+v, want no meditate status", creature.Metadata.Tags)
	}
	assertNoCreatureEffectExpiration(t, world, "PMEDIT")
	if len(broadcasts) != 1 || broadcasts[0].Text != "Alice이 참선을 하다가 주화입마에 빠졌습니다." {
		t.Fatalf("broadcasts = %+v, want failure broadcast", broadcasts)
	}

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() cooldown error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "1분") {
		t.Fatalf("cooldown status/output = %d/%q, want minute wait", status, ctx.OutputString())
	}
}

func TestMeditateHandlerCanBeRegisteredByDispatcherAliases(t *testing.T) {
	for _, line := range []string{"참선", "meditate"} {
		t.Run(line, func(t *testing.T) {
			world := state.NewWorld(meditateWorld(t, legacyClassCleric))
			dispatcher := Dispatcher{
				Registry: mustRegistry(t, []commandspec.CommandSpec{
					{Name: "참선", Number: 90, Handler: "meditate"},
					{Name: "meditate", Number: 90, Handler: "meditate"},
				}),
				Handlers: map[string]Handler{
					"meditate": NewMeditateHandler(world, fixedRoll(1)),
				},
			}

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, line)
			if err != nil {
				t.Fatalf("DispatchLine(%q) error = %v", line, err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), "참선에 들어갑니다") {
				t.Fatalf("status/output = %d/%q, want dispatch success", status, ctx.OutputString())
			}
		})
	}
}

func meditateWorld(t *testing.T, class int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:meditate",
		DisplayName: "사당",
		PlayerIDs:   []model.PlayerID{"player:alice"},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:meditate",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:meditate",
		Level:       12,
		Stats: map[string]int{
			"class":        class,
			"level":        12,
			"piety":        30,
			"intelligence": 30,
		},
	})
	return loaded
}
