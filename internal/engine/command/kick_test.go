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

func TestKickHandlerKicksMonsterAndRevealsActor(t *testing.T) {
	withAttackRolls(t, 1, 20)
	loaded := kickWorld(t, legacyClassBarbarian)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	dispatcher := kickDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "고블린 차기")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "당신의 모습이 서서히 드러납니다.") ||
		!strings.Contains(out, "당신은 발차기로 24점의 공격을 가했습니다.") {
		t.Fatalf("output = %q, want reveal and kick damage", out)
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 16; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	if remaining, ready, err := world.UseCreatureCooldown("creature:alice", kickCooldownKey, time.Now().Unix(), 0); err != nil || ready || remaining <= 0 {
		t.Fatalf("kick cooldown remaining/ready/err = %d/%v/%v, want active hit cooldown", remaining, ready, err)
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

func TestKickHandlerReportsFailureWithoutDamage(t *testing.T) {
	withAttackRolls(t, 100)
	world := state.NewWorld(kickWorld(t, legacyClassBarbarian))
	handler := NewKickHandler(world)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "당신의 발차기가 실패했습니다.") {
		t.Fatalf("status/output = %d/%q, want failure", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0].Text != "\nAlice이 고블린에게 발차기를 하려고 합니다." {
		t.Fatalf("broadcasts = %+v, want failure broadcast", broadcasts)
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 40; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	if remaining, ready, err := world.UseCreatureCooldown("creature:alice", kickCooldownKey, time.Now().Unix(), 0); err != nil || ready || remaining <= 0 {
		t.Fatalf("kick cooldown remaining/ready/err = %d/%v/%v, want active attempt cooldown", remaining, ready, err)
	}
}

func TestKickHandlerRespectsCooldownBeforeReveal(t *testing.T) {
	loaded := kickWorld(t, legacyClassBarbarian)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	if err := world.SetCreatureCooldown("creature:alice", kickCooldownKey, time.Now().Unix(), 2); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewKickHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
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

func TestKickHandlerRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name   string
		class  int
		tags   []string
		args   []string
		mutate func(*worldload.World)
		want   string
	}{
		{name: "missing target", class: legacyClassBarbarian, want: "누굴 공격합니까?"},
		{name: "blind", class: legacyClassBarbarian, tags: []string{"blind"}, args: []string{"고블린"}, want: "누굴 공격합니까?"},
		{name: "wrong class", class: legacyClassFighter, args: []string{"고블린"}, want: "권법가만 쓸수 있는 기술입니다."},
		{name: "invincible without training", class: legacyClassInvincible, args: []string{"고블린"}, want: "권법가를 무적수련하지 않았습니다.."},
		{name: "unknown target", class: legacyClassBarbarian, args: []string{"없는"}, want: "그런 것은 여기 없습니다."},
		{
			name:  "protected monster",
			class: legacyClassBarbarian,
			args:  []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				goblin := loaded.Creatures["creature:goblin-1"]
				goblin.Metadata.Tags = []string{"unkillable"}
				loaded.Creatures[goblin.ID] = goblin
			},
			want: "당신은 그녀를 해칠 수 없습니다.",
		},
		{
			name:  "player kill gate",
			class: legacyClassBarbarian,
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
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
			handler := NewKickHandler(world)

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestKickHandlerRejectsCharmedPlayerLikeLegacy(t *testing.T) {
	loaded := kickWorld(t, legacyClassBarbarian)
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

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewKickHandler(world)(ctx, ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "너무 사랑") {
		t.Fatalf("status/output = %d/%q, want legacy charm refusal", status, ctx.OutputString())
	}
	bob, _ = world.Creature("creature:bob")
	if got, want := bob.Stats["hpCurrent"], 30; got != want {
		t.Fatalf("bob hp = %d, want %d", got, want)
	}
	if remaining, ready, err := world.UseCreatureCooldown("creature:alice", kickCooldownKey, time.Now().Unix(), 0); err != nil || !ready || remaining != 0 {
		t.Fatalf("kick cooldown remaining/ready/err = %d/%v/%v, want unused", remaining, ready, err)
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 1 || alice.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained", alice.Metadata.Tags, alice.Stats)
	}
}

func TestKickHandlerInvincibleWithBarbarianTrainingCanKick(t *testing.T) {
	withAttackRolls(t, 1, 20)
	loaded := kickWorld(t, legacyClassInvincible)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SBARBARIAN"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewKickHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "당신은 발차기로 24점의 공격을 가했습니다.") {
		t.Fatalf("status/output = %d/%q, want successful kick", status, ctx.OutputString())
	}
}

func TestKickHandlerFinalizesMonsterDeathWhenDamageKillsTarget(t *testing.T) {
	withAttackRolls(t, 1, 20)
	loaded := kickWorld(t, legacyClassBarbarian)
	goblin := loaded.Creatures["creature:goblin-1"]
	goblin.Stats["hpCurrent"] = 1
	goblin.Stats["hpMax"] = 1
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewKickHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault ||
		!strings.Contains(ctx.OutputString(), "1점의 공격") ||
		!strings.Contains(ctx.OutputString(), "당신은 고블린을 죽였습니다.") {
		t.Fatalf("status/output = %d/%q, want kick death output", status, ctx.OutputString())
	}
	if _, ok := world.Creature("creature:goblin-1"); ok {
		t.Fatal("goblin still exists, want finalized monster death")
	}
}

func kickDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "차기", Number: 97, Handler: "kick"},
		{Name: "kick", Number: 97, Handler: "kick"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"kick": NewKickHandler(world),
		},
	}
}

func kickWorld(t *testing.T, class int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:arena",
		DisplayName: "Arena",
	})
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
		Stats: map[string]int{
			"class":     class,
			"level":     20,
			"strength":  20,
			"dexterity": 20,
			"thaco":     0,
			"hpCurrent": 50,
			"hpMax":     50,
			"pDice":     3,
		},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:arena",
		Stats:       map[string]int{"class": legacyClassFighter, "level": 1, "hpCurrent": 30, "hpMax": 30},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:goblin-1",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:arena",
		Stats:       map[string]int{"level": 1, "dexterity": 10, "armor": 0, "hpCurrent": 40, "hpMax": 40},
	})
	return loaded
}

func TestKickHandlerDoesNotMutateLegacyProficiency(t *testing.T) {
	withAttackRolls(t, 1, 20)
	loaded := kickWorld(t, legacyClassBarbarian)
	alice := loaded.Creatures["creature:alice"]
	if alice.Properties == nil {
		alice.Properties = map[string]string{}
	}
	alice.Properties["proficiency/kick"] = "10"
	loaded.Creatures[alice.ID] = alice

	world := state.NewWorld(loaded)
	dispatcher := kickDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "고블린 차기"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}

	updated, _ := world.Creature("creature:alice")
	got := updated.Properties["proficiency/kick"]
	if got != "10" {
		t.Fatalf("proficiency/kick = %q, want unchanged legacy value 10", got)
	}
}
