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

func TestAngelHandlerSuccessAddsStatusCooldownAndExpirationWithoutReveal(t *testing.T) {
	loaded := angelWorld(t, legacyClassMage, 50)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PBLIND", "PSILNC"}
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PBLIND", "PSILNC"}
	loaded.Players[player.ID] = player
	runtime := state.NewWorld(loaded)
	handler := NewAngelHandler(runtime, fixedRoll(1))

	before := time.Now().Unix()
	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "정령이 응답합니다") {
		t.Fatalf("status/output = %d/%q, want angel success", status, ctx.OutputString())
	}

	updated, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "PANGEL") ||
		!hasAnyNormalizedFlag(updated.Metadata.Tags, "angel") {
		t.Fatalf("creature tags = %+v, want angel status tags", updated.Metadata.Tags)
	}
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "phiddn") ||
		!hasAnyNormalizedFlag(updated.Metadata.Tags, "PBLIND") ||
		!hasAnyNormalizedFlag(updated.Metadata.Tags, "PSILNC") {
		t.Fatalf("creature tags = %+v, want hidden/blind/silence retained by C angel", updated.Metadata.Tags)
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if !hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "PANGEL") ||
		!hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "angel") {
		t.Fatalf("player tags = %+v, want angel status tags", updatedPlayer.Metadata.Tags)
	}
	if !hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "hidden", "phiddn") ||
		!hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "PBLIND") ||
		!hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "PSILNC") {
		t.Fatalf("player tags = %+v, want hidden/blind/silence retained by C angel", updatedPlayer.Metadata.Tags)
	}
	if len(broadcasts) != 1 || !strings.Contains(broadcasts[0].Text, "정령을 소환합니다") {
		t.Fatalf("broadcasts = %+v, want angel broadcast", broadcasts)
	}
	if expires, ok := runtime.GetEffectExpiration("creature:alice", "PANGEL"); !ok {
		t.Fatal("PANGEL effect expiration was not set")
	} else if expires < before+angelStatusDurationSeconds || expires > time.Now().Unix()+angelStatusDurationSeconds {
		t.Fatalf("PANGEL expiration = %d, want about now+%d", expires, angelStatusDurationSeconds)
	}
	if remaining, used, err := runtime.UseCreatureCooldown("creature:alice", angelCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 || remaining > angelRecastCooldownSeconds {
		t.Fatalf("cooldown used/remaining = %v/%d, want active angel recast cooldown", used, remaining)
	}
}

func TestAngelHandlerAlreadySummonedRejects(t *testing.T) {
	loaded := angelWorld(t, legacyClassMage, 50)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PANGEL"}
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)
	handler := NewAngelHandler(runtime, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "이미 정령소환술을 사용중입니다") {
		t.Fatalf("status/output = %d/%q, want already summoned rejection", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "PANGEL") || len(updated.Metadata.Tags) != 1 {
		t.Fatalf("creature tags = %+v, want unchanged active tag only", updated.Metadata.Tags)
	}
}

func TestAngelHandlerCooldownPrecedesChant(t *testing.T) {
	loaded := angelWorld(t, legacyClassMage, 50)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden"}
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)
	if err := runtime.SetCreatureCooldown("creature:alice", angelCooldownKey, time.Now().Unix(), angelRecastCooldownSeconds); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewAngelHandler(runtime, fixedRoll(1))(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "기다리세요.") || strings.Contains(out, "천상계") {
		t.Fatalf("status/output = %d/%q, want cooldown before chant", status, out)
	}
	updated, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "PANGEL", "angel") || !hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden") {
		t.Fatalf("creature tags = %+v, want no status mutation during cooldown", updated.Metadata.Tags)
	}
}

func TestAngelHandlerFailureDoesNotStartCooldownOrReveal(t *testing.T) {
	loaded := angelWorld(t, legacyClassInvincible, 10)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SMAGE", "hidden"}
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewAngelHandler(runtime, fixedRoll(100))(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "소환하는데 실패") {
		t.Fatalf("status/output = %d/%q, want angel failure", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "PANGEL", "angel") || !hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden") {
		t.Fatalf("creature tags = %+v, want no angel status and hidden retained after failure", updated.Metadata.Tags)
	}
	if _, ok := runtime.GetEffectExpiration("creature:alice", "PANGEL"); ok {
		t.Fatal("PANGEL effect expiration set on failed angel")
	}
	if remaining, used, err := runtime.UseCreatureCooldown("creature:alice", angelCooldownKey, time.Now().Unix(), 0); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want no cooldown after failed angel", used, remaining)
	}
}

func TestAngelHandlerRejectsClassAndUntrainedInvincible(t *testing.T) {
	tests := []struct {
		name   string
		class  int
		level  int
		tags   []string
		want   string
		active bool
	}{
		{
			name:  "wrong class",
			class: legacyClassFighter,
			level: 50,
			want:  "도술사 50 이상만 사용할 수 있는 기술입니다.",
		},
		{
			name:  "mage below level fifty",
			class: legacyClassMage,
			level: 49,
			want:  "도술사 50 이상만 사용할 수 있는 기술입니다.",
		},
		{
			name:  "invincible without mage training",
			class: legacyClassInvincible,
			level: 50,
			want:  "도술사를 무적수련하지 않았습니다..",
		},
		{
			name:   "invincible with mage training",
			class:  legacyClassInvincible,
			level:  10,
			tags:   []string{"SMAGE"},
			want:   "정령이 응답합니다",
			active: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := angelWorld(t, tt.class, tt.level)
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = tt.tags
			loaded.Creatures[alice.ID] = alice
			runtime := state.NewWorld(loaded)
			handler := NewAngelHandler(runtime, fixedRoll(1))

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			updated, _ := runtime.Creature("creature:alice")
			if got := hasAnyNormalizedFlag(updated.Metadata.Tags, "PANGEL", "angel"); got != tt.active {
				t.Fatalf("active angel tags = %v, want %v; tags=%+v", got, tt.active, updated.Metadata.Tags)
			}
		})
	}
}

func TestAngelHandlerCanBeRegisteredByDispatcherAliases(t *testing.T) {
	for _, line := range []string{"정령소환술", "angel"} {
		t.Run(line, func(t *testing.T) {
			runtime := state.NewWorld(angelWorld(t, legacyClassMage, 50))
			dispatcher := Dispatcher{
				Registry: mustRegistry(t, []commandspec.CommandSpec{
					{Name: "정령소환술", Number: 169, Handler: "angel"},
					{Name: "angel", Number: 169, Handler: "angel"},
				}),
				Handlers: map[string]Handler{
					"angel": NewAngelHandler(runtime, fixedRoll(1)),
				},
			}

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, line)
			if err != nil {
				t.Fatalf("DispatchLine(%q) error = %v", line, err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), "정령이 응답합니다") {
				t.Fatalf("status/output = %d/%q, want dispatch success", status, ctx.OutputString())
			}
		})
	}
}

func angelWorld(t *testing.T, class int, level int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:altar",
		DisplayName: "Altar",
		PlayerIDs: []model.PlayerID{
			"player:alice",
		},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:altar",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:altar",
		Level:       level,
		Stats: map[string]int{
			"class":        class,
			"level":        level,
			"intelligence": 30,
		},
	})
	return loaded
}
