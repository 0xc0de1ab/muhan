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

func TestPrepareHandlerSuccessSetsPreparedTagsAndCooldown(t *testing.T) {
	runtime := state.NewWorld(prepareWorld(t, model.ClassFighter))
	handler := NewPrepareHandler(runtime)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 이제부터 함정이 있나 살펴보며 갑니다." {
		t.Fatalf("status/output = %d/%q, want prepare success", status, ctx.OutputString())
	}

	creature, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "prepared") || !hasAnyNormalizedFlag(creature.Metadata.Tags, "PPREPA") {
		t.Fatalf("creature tags = %+v, want prepared and PPREPA", creature.Metadata.Tags)
	}
	player, _ := runtime.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "prepared") || !hasAnyNormalizedFlag(player.Metadata.Tags, "PPREPA") {
		t.Fatalf("player tags = %+v, want prepared and PPREPA", player.Metadata.Tags)
	}
	if len(broadcasts) != 1 ||
		broadcasts[0].RoomID != "room:prepare" ||
		broadcasts[0].Exclude != "session:alice" ||
		broadcasts[0].Text != "\nAlice가 함정을 조심하며 갑니다." {
		t.Fatalf("broadcasts = %+v, want prepare room broadcast", broadcasts)
	}

	remaining, used, err := runtime.UseCreatureCooldown("creature:alice", prepareCooldownKey, time.Now().Unix(), 1)
	if err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	}
	if used || remaining <= 0 {
		t.Fatalf("cooldown = remaining %d used %v, want active", remaining, used)
	}
}

func TestPrepareHandlerRejectsDuplicateAndInvalidStates(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*worldload.World)
		setup  func(*state.World)
		want   string
	}{
		{
			name: "already prepared creature",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"PPREPA"}
				loaded.Creatures[alice.ID] = alice
			},
			want: "당신은 이미 함정들을 주의하고 있습니다.",
		},
		{
			name: "already prepared player",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Players["player:alice"]
				alice.Metadata.Tags = []string{"prepared"}
				loaded.Players[alice.ID] = alice
			},
			want: "당신은 이미 함정들을 주의하고 있습니다.",
		},
		{
			name: "cooldown active",
			setup: func(runtime *state.World) {
				if err := runtime.SetCreatureCooldown("creature:alice", prepareCooldownKey, time.Now().Unix(), prepareCooldownSeconds); err != nil {
					t.Fatalf("SetCreatureCooldown() error = %v", err)
				}
			},
			want: "기다리세요",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := prepareWorld(t, model.ClassFighter)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			runtime := state.NewWorld(loaded)
			if tt.setup != nil {
				tt.setup(runtime)
			}

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewPrepareHandler(runtime)(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			got := ctx.OutputString()
			matches := got == tt.want
			if strings.Contains(tt.name, "cooldown") {
				matches = strings.Contains(got, tt.want)
			}
			if status != StatusDefault || !matches {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}

			if strings.Contains(tt.name, "cooldown") {
				creature, _ := runtime.Creature("creature:alice")
				player, _ := runtime.Player("player:alice")
				if hasAnyNormalizedFlag(creature.Metadata.Tags, "prepared", "PPREPA") ||
					hasAnyNormalizedFlag(player.Metadata.Tags, "prepared", "PPREPA") {
					t.Fatalf("prepared tags after rejection = creature %+v player %+v", creature.Metadata.Tags, player.Metadata.Tags)
				}
			}
		})
	}
}

func TestPrepareHandlerBlindActorConsumesCooldownWithoutPreparedStatus(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*worldload.World)
	}{
		{
			name: "blind creature",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"PBLIND"}
				loaded.Creatures[alice.ID] = alice
			},
		},
		{
			name: "blind player",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Players["player:alice"]
				alice.Metadata.Tags = []string{"blind"}
				loaded.Players[alice.ID] = alice
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := prepareWorld(t, model.ClassFighter)
			tt.mutate(loaded)
			runtime := state.NewWorld(loaded)
			var broadcasts []roomBroadcastRecord

			ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
			status, err := NewPrepareHandler(runtime)(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != "당신은 이제부터 함정이 있나 살펴보며 갑니다." {
				t.Fatalf("status/output = %d/%q, want C-style prepare success", status, ctx.OutputString())
			}
			if len(broadcasts) != 1 || broadcasts[0].Text != "\nAlice가 함정을 조심하며 갑니다." {
				t.Fatalf("broadcasts = %+v, want prepare broadcast", broadcasts)
			}

			creature, _ := runtime.Creature("creature:alice")
			player, _ := runtime.Player("player:alice")
			if hasAnyNormalizedFlag(creature.Metadata.Tags, "prepared", "PPREPA") ||
				hasAnyNormalizedFlag(player.Metadata.Tags, "prepared", "PPREPA") {
				t.Fatalf("prepared tags = creature %+v player %+v, want cleared for blind actor", creature.Metadata.Tags, player.Metadata.Tags)
			}
			if remaining, used, err := runtime.UseCreatureCooldown("creature:alice", prepareCooldownKey, time.Now().Unix(), 1); err != nil {
				t.Fatalf("UseCreatureCooldown() error = %v", err)
			} else if used || remaining < 1 || remaining > prepareCooldownSeconds {
				t.Fatalf("cooldown used/remaining = %v/%d, want active prepare cooldown", used, remaining)
			}
		})
	}
}

func TestPrepareDispatcherAliases(t *testing.T) {
	for _, line := range []string{"경계", "prepare"} {
		t.Run(line, func(t *testing.T) {
			runtime := state.NewWorld(prepareWorld(t, model.ClassFighter))
			dispatcher := Dispatcher{
				Registry: mustRegistry(t, []commandspec.CommandSpec{
					{Name: "경계", Number: 66, Handler: "prepare"},
					{Name: "prepare", Number: 66, Handler: "prepare"},
				}),
				Handlers: map[string]Handler{
					"prepare": NewPrepareHandler(runtime),
				},
			}

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, line)
			if err != nil {
				t.Fatalf("DispatchLine(%q) error = %v", line, err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), "함정이 있나 살펴보며 갑니다") {
				t.Fatalf("status/output = %d/%q, want prepare success", status, ctx.OutputString())
			}
		})
	}
}

func prepareWorld(t *testing.T, class int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:prepare",
		DisplayName: "Prepare",
		PlayerIDs:   []model.PlayerID{"player:alice"},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:prepare",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:prepare",
		Stats:       map[string]int{"class": class, "level": 10, "dexterity": 40},
	})
	return loaded
}
