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

func TestPrayHandlerSuccessAddsPietyTagsCooldownAndExpiration(t *testing.T) {
	loaded := prayWorld(t, legacyClassCleric)
	runtime := state.NewWorld(loaded)
	handler := NewPrayHandler(runtime, fixedRoll(1))
	var broadcasts []roomBroadcastRecord

	before := time.Now().Unix()
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "신앙심이 깊어지는") {
		t.Fatalf("status/output = %d/%q, want successful prayer", status, ctx.OutputString())
	}

	updated, _ := runtime.Creature("creature:alice")
	if got, want := updated.Stats["piety"], 35; got != want {
		t.Fatalf("piety = %d, want %d", got, want)
	}
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "PPRAYD") ||
		!hasAnyNormalizedFlag(updated.Metadata.Tags, "pray") {
		t.Fatalf("creature tags = %+v, want PPRAYD and pray", updated.Metadata.Tags)
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if !hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "PPRAYD") ||
		!hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "pray") {
		t.Fatalf("player tags = %+v, want PPRAYD and pray", updatedPlayer.Metadata.Tags)
	}
	assertCreatureEffectExpiration(t, runtime, "PPRAYD", before, prayStatusDurationSeconds)
	if remaining, used, err := runtime.UseCreatureCooldown("creature:alice", prayCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active cooldown", used, remaining)
	}
	if len(broadcasts) != 1 || broadcasts[0].Text != "Alice이 신에게 기원합니다." {
		t.Fatalf("broadcasts = %+v, want prayer broadcast", broadcasts)
	}
}

func TestPrayHandlerRejectsClassAndUntrainedInvincible(t *testing.T) {
	tests := []struct {
		name   string
		class  int
		tags   []string
		want   string
		piety  int
		active bool
	}{
		{
			name:  "wrong class",
			class: legacyClassFighter,
			want:  "불제자와 무사만이 신께 기원할 수 있습니다.",
			piety: 30,
		},
		{
			name:  "invincible without training",
			class: legacyClassInvincible,
			want:  "불제자나 무사를 무적수련하지 않았습니다..",
			piety: 30,
		},
		{
			name:   "invincible with cleric training",
			class:  legacyClassInvincible,
			tags:   []string{"SCLERIC"},
			want:   "신앙심이 깊어지는",
			piety:  35,
			active: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := prayWorld(t, tt.class)
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = tt.tags
			loaded.Creatures[alice.ID] = alice
			runtime := state.NewWorld(loaded)
			handler := NewPrayHandler(runtime, fixedRoll(1))

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			updated, _ := runtime.Creature("creature:alice")
			if got := updated.Stats["piety"]; got != tt.piety {
				t.Fatalf("piety = %d, want %d", got, tt.piety)
			}
			if got := hasAnyNormalizedFlag(updated.Metadata.Tags, "PPRAYD", "pray"); got != tt.active {
				t.Fatalf("active prayer tags = %v, want %v; tags=%+v", got, tt.active, updated.Metadata.Tags)
			}
		})
	}
}

func TestPrayHandlerAlreadyPrayedSkipsCooldownAndPiety(t *testing.T) {
	loaded := prayWorld(t, legacyClassPaladin)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PPRAYD"}
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)
	handler := NewPrayHandler(runtime, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "이미 신에게 빌었습니다") {
		t.Fatalf("status/output = %d/%q, want already-prayed message", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if got, want := updated.Stats["piety"], 30; got != want {
		t.Fatalf("piety = %d, want %d", got, want)
	}
	if remaining, used, err := runtime.UseCreatureCooldown("creature:alice", prayCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want unused cooldown", used, remaining)
	}
}

func TestPrayHandlerFailureSetsShortCooldownWithoutTags(t *testing.T) {
	runtime := state.NewWorld(prayWorld(t, legacyClassCleric))
	handler := NewPrayHandler(runtime, fixedRoll(100))

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "신의 응답이 없습니다") {
		t.Fatalf("status/output = %d/%q, want failed prayer", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if got, want := updated.Stats["piety"], 30; got != want {
		t.Fatalf("piety = %d, want %d", got, want)
	}
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "PPRAYD", "pray") {
		t.Fatalf("creature tags = %+v, want no prayer tags", updated.Metadata.Tags)
	}
	assertNoCreatureEffectExpiration(t, runtime, "PPRAYD")
	if len(broadcasts) != 1 || broadcasts[0].Text != "Alice이 신에게 기원합니다." {
		t.Fatalf("broadcasts = %+v, want failure prayer broadcast", broadcasts)
	}

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() cooldown error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "기다리세요") {
		t.Fatalf("cooldown status/output = %d/%q, want wait message", status, ctx.OutputString())
	}
}

func TestPrayHandlerCanBeRegisteredByDispatcher(t *testing.T) {
	runtime := state.NewWorld(prayWorld(t, legacyClassCleric))
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "신원법", Number: 65, Handler: "pray"},
			{Name: "pray", Number: 65, Handler: "pray"},
		}),
		Handlers: map[string]Handler{
			"pray": NewPrayHandler(runtime, fixedRoll(1)),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "신원법")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "신앙심이 깊어지는") {
		t.Fatalf("status/output = %d/%q, want Korean command success", status, ctx.OutputString())
	}

	runtime = state.NewWorld(prayWorld(t, legacyClassCleric))
	dispatcher.Handlers["pray"] = NewPrayHandler(runtime, fixedRoll(1))
	ctx = &Context{ActorID: "player:alice"}
	status, err = dispatcher.DispatchLine(ctx, "pray")
	if err != nil {
		t.Fatalf("DispatchLine(pray) error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "신앙심이 깊어지는") {
		t.Fatalf("status/output = %d/%q, want English command success", status, ctx.OutputString())
	}
}

func prayWorld(t *testing.T, class int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{ID: "room:temple", DisplayName: "사당"})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:temple",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:temple",
		Level:       12,
		Stats:       map[string]int{"class": class, "level": 12, "piety": 30},
	})
	return loaded
}
