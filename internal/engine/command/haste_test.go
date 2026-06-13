package command

import (
	"strings"
	"testing"
	"time"

	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestHasteHandlerSuccessSetsStatusDexterityCooldownAndExpiration(t *testing.T) {
	world := state.NewWorld(hasteWorld(t, model.ClassRanger))
	defer world.Close()
	handler := NewHasteHandler(world, fixedHasteRoll(1))

	var broadcasts []roomBroadcastRecord
	before := time.Now().Unix()
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신의 동작이 좀더 민첩해진것 같습니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}

	creature, _ := world.Creature("creature:alice")
	if got := creatureStat(creature, "dexterity"); got != 55 {
		t.Fatalf("dexterity = %d, want 55", got)
	}
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "haste") || !hasAnyNormalizedFlag(creature.Metadata.Tags, "PHASTE") {
		t.Fatalf("creature tags = %+v, want haste and PHASTE", creature.Metadata.Tags)
	}
	player, _ := world.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "haste") || !hasAnyNormalizedFlag(player.Metadata.Tags, "PHASTE") {
		t.Fatalf("player tags = %+v, want haste and PHASTE", player.Metadata.Tags)
	}
	if len(broadcasts) != 1 || broadcasts[0].Text != "Alice이 활보법을 사용하였습니다." {
		t.Fatalf("broadcasts = %+v, want haste broadcast", broadcasts)
	}
	assertCreatureEffectExpiration(t, world, "PHASTE", before, hasteStatusDurationSeconds(creature))

	remaining, used, err := world.UseCreatureCooldown("creature:alice", hasteCooldownKey, time.Now().Unix(), 1)
	if err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	}
	if used || remaining <= 0 {
		t.Fatalf("cooldown = remaining %d used %v, want active", remaining, used)
	}
}

func TestHasteHandlerRejectsInvalidStates(t *testing.T) {
	tests := []struct {
		name   string
		class  int
		mutate func(*worldload.World)
		setup  func(*state.World)
		want   string
	}{
		{
			name:  "wrong class",
			class: model.ClassFighter,
			want:  "포졸만 사용할 수 있는 기술입니다.",
		},
		{
			name:  "invincible without ranger training",
			class: model.ClassInvincible,
			want:  "포졸을 무적수련하지 않았습니다..",
		},
		{
			name:  "already hasted creature",
			class: model.ClassRanger,
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"PHASTE"}
				loaded.Creatures[alice.ID] = alice
			},
			want: "당신은 지금 활보법을 사용중입니다.",
		},
		{
			name:  "cooldown active",
			class: model.ClassRanger,
			setup: func(world *state.World) {
				if err := world.SetCreatureCooldown("creature:alice", hasteCooldownKey, time.Now().Unix(), hasteCooldownSeconds); err != nil {
					t.Fatalf("SetCreatureCooldown() error = %v", err)
				}
			},
			want: "기다리세요.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := hasteWorld(t, tt.class)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
	defer world.Close()
			if tt.setup != nil {
				tt.setup(world)
			}
			handler := NewHasteHandler(world, fixedHasteRoll(1))

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

func TestHasteHandlerInvincibleWithRangerTrainingCanUse(t *testing.T) {
	loaded := hasteWorld(t, model.ClassInvincible)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SRANGER"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	defer world.Close()
	handler := NewHasteHandler(world, fixedHasteRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "민첩해진것 같습니다") {
		t.Fatalf("status/output = %d/%q, want success", status, ctx.OutputString())
	}
}

func TestHasteHandlerFailureStartsShortCooldownWithoutStatus(t *testing.T) {
	world := state.NewWorld(hasteWorld(t, model.ClassRanger))
	defer world.Close()
	handler := NewHasteHandler(world, fixedHasteRoll(100))

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "활보법이 실패하였습니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	creature, _ := world.Creature("creature:alice")
	if got := creatureStat(creature, "dexterity"); got != 40 {
		t.Fatalf("dexterity = %d, want unchanged 40", got)
	}
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "haste", "PHASTE") {
		t.Fatalf("creature tags = %+v, want no haste status", creature.Metadata.Tags)
	}
	assertNoCreatureEffectExpiration(t, world, "PHASTE")
	if len(broadcasts) != 1 || broadcasts[0].Text != "Alice이 활보법을 써봅니다." {
		t.Fatalf("broadcasts = %+v, want failure broadcast", broadcasts)
	}

	remaining, used, err := world.UseCreatureCooldown("creature:alice", hasteCooldownKey, time.Now().Unix(), 1)
	if err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	}
	if used || remaining < 1 || remaining > hasteFailureWaitSeconds {
		t.Fatalf("failure cooldown = remaining %d used %v, want short active cooldown", remaining, used)
	}
}

func hasteWorld(t *testing.T, class int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:haste",
		DisplayName: "Haste",
		PlayerIDs:   []model.PlayerID{"player:alice"},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:haste",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:haste",
		Level:       20,
		Stats:       map[string]int{"class": class, "level": 20, "dexterity": 40},
	})
	return loaded
}

func fixedHasteRoll(value int) HasteRollFunc {
	return func(int, int) int {
		return value
	}
}
