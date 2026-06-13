package command

import (
	"strings"
	"testing"
	"time"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestCircleHandlerBefuddlesMonsterAndRevealsActor(t *testing.T) {
	withAttackRolls(t, 1, 6)
	loaded := kickWorld(t, model.ClassFighter)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := circleDispatcher(t, world)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := dispatcher.DispatchLine(ctx, "고블린 교란")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "당신의 모습이 서서히 드러납니다.") ||
		!strings.Contains(out, "당신은 이리저리 왔다갔다 하면서 고블린을 교란시킵니다.") {
		t.Fatalf("output = %q, want reveal and circle success", out)
	}
	if len(broadcasts) != 2 ||
		broadcasts[0].Text != "\nAlice의 모습이 서서히 드러납니다." ||
		broadcasts[1].Text != "\nAlice이 고블린 주위를 뱅글뱅글 돕니다." {
		t.Fatalf("broadcasts = %+v, want reveal and circle broadcasts", broadcasts)
	}

	goblin, _ := world.Creature("creature:goblin-1")
	if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "befuddled") ||
		!hasAnyNormalizedFlag(goblin.Metadata.Tags, "MBEFUD") {
		t.Fatalf("goblin tags = %+v, want befuddled and MBEFUD", goblin.Metadata.Tags)
	}
	if remaining, ready, err := world.UseCreatureCooldown(goblin.ID, "befuddled", time.Now().Unix(), 0); err != nil || ready || remaining <= 0 {
		t.Fatalf("befuddled cooldown remaining/ready/err = %d/%v/%v, want active cooldown", remaining, ready, err)
	}
	if remaining, ready, err := world.UseCreatureCooldown(alice.ID, "attack", time.Now().Unix(), 0); err != nil || ready || remaining <= 0 {
		t.Fatalf("attack cooldown remaining/ready/err = %d/%v/%v, want active success cooldown", remaining, ready, err)
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

func TestCircleHandlerReportsFailureWithoutBefuddle(t *testing.T) {
	withAttackRolls(t, 100)
	world := state.NewWorld(kickWorld(t, model.ClassFighter))
	defer world.Close()
	handler := NewCircleHandler(world)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "당신은 적을 교란시키는데 실패하였습니다.") {
		t.Fatalf("status/output = %d/%q, want failure", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0].Text != "\nAlice이 고블린을 교란시키려고 합니다." {
		t.Fatalf("broadcasts = %+v, want failure broadcast", broadcasts)
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if hasAnyNormalizedFlag(goblin.Metadata.Tags, "befuddled", "MBEFUD") {
		t.Fatalf("goblin tags = %+v, want no befuddle", goblin.Metadata.Tags)
	}
	alice, _ := world.Creature("creature:alice")
	if remaining, ready, err := world.UseCreatureCooldown(alice.ID, "attack", time.Now().Unix(), 0); err != nil || ready || remaining <= 0 {
		t.Fatalf("attack cooldown remaining/ready/err = %d/%v/%v, want active failure cooldown", remaining, ready, err)
	}
}

func TestCircleHandlerRespectsAttackCooldownBeforeReveal(t *testing.T) {
	loaded := kickWorld(t, model.ClassFighter)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	defer world.Close()
	if err := world.SetCreatureCooldown("creature:alice", "attack", time.Now().Unix(), 2); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCircleHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "초") || !strings.Contains(ctx.OutputString(), "기다리세요.") {
		t.Fatalf("status/output = %d/%q, want please_wait", status, ctx.OutputString())
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN") ||
		!hasAnyNormalizedFlag(alice.Metadata.Tags, "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 1 || alice.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained during cooldown", alice.Metadata.Tags, alice.Stats)
	}
}

func TestCircleHandlerRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name   string
		class  int
		tags   []string
		args   []string
		mutate func(model.Room, *state.World)
		want   string
	}{
		{name: "missing target", class: model.ClassFighter, want: "누구를 교란시키려구요?"},
		{name: "wrong class", class: model.ClassThief, args: []string{"고블린"}, want: "권법가와 검사만 쓸수 있는 기술입니다."},
		{name: "invincible without training", class: model.ClassInvincible, args: []string{"고블린"}, want: "권법가와 검사를 무적수련하지 않았습니다.."},
		{name: "unknown target", class: model.ClassFighter, args: []string{"없는"}, want: "그런것은 여기 없습니다."},
		{name: "short player target", class: model.ClassFighter, args: []string{"Bo"}, want: "그런것은 여기 없습니다."},
		{
			name:  "protected monster",
			class: model.ClassFighter,
			args:  []string{"고블린"},
			mutate: func(_ model.Room, world *state.World) {
				goblin, _ := world.Creature("creature:goblin-1")
				_, _ = world.UpdateCreatureTags(goblin.ID, []string{"unkillable"}, nil)
			},
			want: "당신은 그녀를 해칠수 없습니다.",
		},
		{
			name:  "player kill gate",
			class: model.ClassFighter,
			args:  []string{"Bob"},
			want:  "당신은 선해서 다른 사용자를 공격할 수 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := kickWorld(t, tt.class)
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = tt.tags
			loaded.Creatures[alice.ID] = alice
			world := state.NewWorld(loaded)
	defer world.Close()
			room, _ := world.Room("room:arena")
			if tt.mutate != nil {
				tt.mutate(room, world)
			}

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewCircleHandler(world)(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestCircleHandlerSetsPlayerBefuddleCooldownWithoutTagsLikeLegacy(t *testing.T) {
	withAttackRolls(t, 1, 6)
	loaded := kickWorld(t, model.ClassFighter)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PCHAOS"}
	loaded.Creatures[alice.ID] = alice
	bob := loaded.Creatures["creature:bob"]
	bob.Metadata.Tags = []string{"PCHAOS"}
	loaded.Creatures[bob.ID] = bob
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCircleHandler(world)(ctx, ResolvedCommand{Args: []string{"bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "Bob를 교란시킵니다.") {
		t.Fatalf("status/output = %d/%q, want player circle success", status, ctx.OutputString())
	}
	bob, _ = world.Creature("creature:bob")
	if hasAnyNormalizedFlag(bob.Metadata.Tags, "befuddled", "MBEFUD") {
		t.Fatalf("bob creature tags = %+v, want no C player befuddle flags", bob.Metadata.Tags)
	}
	player, _ := world.Player("player:bob")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "befuddled", "MBEFUD") {
		t.Fatalf("bob player tags = %+v, want no C player befuddle flags", player.Metadata.Tags)
	}
	if remaining, ready, err := world.UseCreatureCooldown(bob.ID, "befuddled", time.Now().Unix(), 0); err != nil || ready || remaining <= 0 {
		t.Fatalf("bob befuddled cooldown remaining/ready/err = %d/%v/%v, want active cooldown", remaining, ready, err)
	}
}

func TestCircleHandlerRejectsCharmedPlayerLikeLegacy(t *testing.T) {
	loaded := kickWorld(t, model.ClassFighter)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PCHAOS", "hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	alicePlayer := loaded.Players["player:alice"]
	alicePlayer.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[alicePlayer.ID] = alicePlayer
	bob := loaded.Creatures["creature:bob"]
	bob.Metadata.Tags = []string{"PCHAOS", "PCHARM", "charm:Alice"}
	loaded.Creatures[bob.ID] = bob
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCircleHandler(world)(ctx, ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "너무 사랑해서") {
		t.Fatalf("status/output = %d/%q, want legacy charm refusal", status, ctx.OutputString())
	}
	bob, _ = world.Creature("creature:bob")
	if hasAnyNormalizedFlag(bob.Metadata.Tags, "befuddled", "MBEFUD") {
		t.Fatalf("bob tags = %+v, want no befuddle", bob.Metadata.Tags)
	}
	if remaining, ready, err := world.UseCreatureCooldown("creature:alice", "attack", time.Now().Unix(), 0); err != nil || !ready || remaining != 0 {
		t.Fatalf("attack cooldown remaining/ready/err = %d/%v/%v, want unused", remaining, ready, err)
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 1 || alice.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained", alice.Metadata.Tags, alice.Stats)
	}
}

func TestCircleHandlerUsesLegacyByteLengthForPlayerTarget(t *testing.T) {
	withAttackRolls(t, 1, 6)
	loaded := kickWorld(t, model.ClassFighter)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PCHAOS"}
	loaded.Creatures[alice.ID] = alice
	bobPlayer := loaded.Players["player:bob"]
	bobPlayer.DisplayName = "보브"
	loaded.Players[bobPlayer.ID] = bobPlayer
	bob := loaded.Creatures["creature:bob"]
	bob.DisplayName = "보브"
	bob.Metadata.Tags = []string{"PCHAOS"}
	loaded.Creatures[bob.ID] = bob
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCircleHandler(world)(ctx, ResolvedCommand{Args: []string{"보브"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "보브를 교란시킵니다.") {
		t.Fatalf("status/output = %d/%q, want Korean two-rune target accepted", status, ctx.OutputString())
	}
	bob, _ = world.Creature("creature:bob")
	if hasAnyNormalizedFlag(bob.Metadata.Tags, "befuddled", "MBEFUD") {
		t.Fatalf("bob creature tags = %+v, want no C player befuddle flags", bob.Metadata.Tags)
	}
	if remaining, ready, err := world.UseCreatureCooldown(bob.ID, "befuddled", time.Now().Unix(), 0); err != nil || ready || remaining <= 0 {
		t.Fatalf("bob befuddled cooldown remaining/ready/err = %d/%v/%v, want active cooldown", remaining, ready, err)
	}
}

func TestCircleHandlerInvincibleWithTrainingCanCircle(t *testing.T) {
	withAttackRolls(t, 1, 6)
	loaded := kickWorld(t, model.ClassInvincible)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SFIGHTER"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCircleHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "교란시킵니다.") {
		t.Fatalf("status/output = %d/%q, want successful circle", status, ctx.OutputString())
	}
}

func TestCircleChanceMatchesLegacyModifiers(t *testing.T) {
	actor := model.Creature{Stats: map[string]int{"level": 20, "dexterity": 20}}
	victim := model.Creature{Kind: model.CreatureKindMonster, Stats: map[string]int{"level": 4, "dexterity": 10}}
	if got, want := circleChance(actor, victim), 80; got != want {
		t.Fatalf("circleChance() = %d, want capped %d", got, want)
	}
	actor.Stats["level"] = 8
	victim.Metadata.Tags = []string{"MUNDED"}
	if got, want := circleChance(actor, victim), 59; got != want {
		t.Fatalf("circleChance() undead = %d, want %d", got, want)
	}
	victim.Metadata.Tags = []string{"MNOCIR"}
	if got, want := circleChance(actor, victim), 1; got != want {
		t.Fatalf("circleChance() no-circle = %d, want %d", got, want)
	}
	actor.Metadata.Tags = []string{"PBLIND"}
	victim.Metadata.Tags = nil
	if got, want := circleChance(actor, victim), 1; got != want {
		t.Fatalf("circleChance() blind = %d, want %d", got, want)
	}
}

func circleDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "교란", Number: 50, Handler: "circle"},
		{Name: "circle", Number: 50, Handler: "circle"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"circle": NewCircleHandler(world),
		},
	}
}
