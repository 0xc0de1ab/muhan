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

func TestBackstabHandlerBackstabsMonsterAndRevealsActor(t *testing.T) {
	withAttackRolls(t, 20, 30)
	loaded := backstabWorld(t, legacyClassThief)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	dispatcher := backstabDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "고블린 기습")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "당신의 모습이 서서히 드러납니다.") ||
		!strings.Contains(out, "당신은 고블린의 뒤로 몰래 기어가 옆구리를 쿡~ 찌릅니다.") ||
		!strings.Contains(out, "당신은 15 만큼의 피해를 주었습니다.") {
		t.Fatalf("output = %q, want reveal and backstab damage", out)
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 25; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
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
	if remaining, ready, err := world.UseCreatureCooldown("creature:alice", "attack", time.Now().Unix(), 0); err != nil || ready || remaining <= 0 {
		t.Fatalf("attack cooldown remaining/ready/err = %d/%v/%v, want active backstab cooldown", remaining, ready, err)
	}
}

func TestBackstabHandlerFailsWhenActorIsNotHidden(t *testing.T) {
	world := state.NewWorld(backstabWorld(t, legacyClassThief))
	handler := NewBackstabHandler(world)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "허공을 쳤습니다.") {
		t.Fatalf("status/output = %d/%q, want hidden failure", status, ctx.OutputString())
	}
	if len(broadcasts) != 2 || broadcasts[1].Text != "\n그녀가 기습을 시도했지만 상대방이 피했습니다." {
		t.Fatalf("broadcasts = %+v, want legacy miss broadcast", broadcasts)
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 40; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
}

func TestBackstabHandlerRejectsEnemyMonsterBeforeRevealLikeLegacy(t *testing.T) {
	loaded := backstabWorld(t, legacyClassThief)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "invisible"}
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "invisible"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	if _, err := world.AddEnemy("creature:goblin-1", "creature:alice"); err != nil {
		t.Fatalf("AddEnemy() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBackstabHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 그녀와 싸울 수 없습니다." {
		t.Fatalf("status/output = %d/%q, want enemy-monster refusal", status, ctx.OutputString())
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden") || !hasAnyNormalizedFlag(alice.Metadata.Tags, "invisible") {
		t.Fatalf("alice tags = %+v, want hidden/invisible retained before reveal", alice.Metadata.Tags)
	}
}

func TestBackstabHandlerRejectsWhenTargetCharmListContainsCharmedActorLikeLegacy(t *testing.T) {
	loaded := backstabWorld(t, legacyClassThief)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PCHAOS", "PCHARM", "hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	alicePlayer := loaded.Players["player:alice"]
	alicePlayer.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[alicePlayer.ID] = alicePlayer
	bob := loaded.Creatures["creature:bob"]
	bob.Metadata.Tags = []string{"PCHAOS", "charm:Alice"}
	loaded.Creatures[bob.ID] = bob
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBackstabHandler(world)(ctx, ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault ||
		!strings.Contains(ctx.OutputString(), "너무 사랑해") ||
		!strings.Contains(ctx.OutputString(), "해칠 수 없습니다") {
		t.Fatalf("status/output = %d/%q, want target charm-list refusal", status, ctx.OutputString())
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

func TestBackstabHandlerRejectsActorsOwnCharmedTargetLikeLegacy(t *testing.T) {
	loaded := backstabWorld(t, legacyClassThief)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PCHARM", "hidden", "PHIDDN", "invisible", "PINVIS", "charm:고블린"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	alicePlayer := loaded.Players["player:alice"]
	alicePlayer.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[alicePlayer.ID] = alicePlayer
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBackstabHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault ||
		!strings.Contains(ctx.OutputString(), "너무 사랑해서") ||
		!strings.Contains(ctx.OutputString(), "용기가") {
		t.Fatalf("status/output = %d/%q, want actor charm-list refusal", status, ctx.OutputString())
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 40; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
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

func TestBackstabHandlerRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name   string
		class  int
		tags   []string
		args   []string
		mutate func(*worldload.World)
		want   string
	}{
		{name: "missing target", class: legacyClassThief, want: "누구를 기습하시려구요?"},
		{name: "blind", class: legacyClassThief, tags: []string{"blind"}, args: []string{"고블린"}, want: "누구를 기습하시려구요?"},
		{name: "wrong class", class: legacyClassFighter, args: []string{"고블린"}, want: "도둑이나 자객만 사용할 수 있는 기술입니다."},
		{name: "invincible without training", class: legacyClassInvincible, args: []string{"고블린"}, want: "도둑이나 자객을 무적수련하지 않았습니다.."},
		{name: "unknown target", class: legacyClassThief, args: []string{"없는"}, want: "그런건 여기 없어요."},
		{
			name:  "missing weapon",
			class: legacyClassThief,
			args:  []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Equipment = nil
				loaded.Creatures[alice.ID] = alice
			},
			want: "기습을 하시려면 도나 검종류의 무기가 필요합니다.",
		},
		{
			name:  "blunt weapon",
			class: legacyClassThief,
			args:  []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				dagger := loaded.Objects["object:dagger"]
				dagger.Properties = map[string]string{"type": "2", "pDice": "5"}
				loaded.Objects[dagger.ID] = dagger
			},
			want: "기습을 하시려면 도나 검종류의 무기가 필요합니다.",
		},
		{
			name:  "protected monster",
			class: legacyClassThief,
			args:  []string{"수호석"},
			tags:  []string{"hidden"},
			want:  "당신은 그녀를 해칠수 없습니다.",
		},
		{
			name:  "magic only monster",
			class: legacyClassThief,
			args:  []string{"망령"},
			tags:  []string{"hidden"},
			want:  "아무 소용이 없습니다.",
		},
		{
			name:  "player kill gate",
			class: legacyClassThief,
			args:  []string{"Bob"},
			tags:  []string{"hidden"},
			want:  "당신은 선해서 다른 사용자를 공격할 수 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := backstabWorld(t, tt.class)
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = tt.tags
			loaded.Creatures[alice.ID] = alice
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
			handler := NewBackstabHandler(world)

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

func TestBackstabHandlerInvincibleWithTrainingUsesAssassinDamage(t *testing.T) {
	withAttackRolls(t, 20)
	loaded := backstabWorld(t, legacyClassInvincible)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "SASSASSIN"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBackstabHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "25 만큼의 피해") {
		t.Fatalf("status/output = %d/%q, want trained invincible backstab", status, ctx.OutputString())
	}
}

func TestBackstabHandlerCapsDamageForCaretakerClassTarget(t *testing.T) {
	withAttackRolls(t, 20)
	loaded := backstabWorld(t, legacyClassAssassin)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBackstabHandler(world)(ctx, ResolvedCommand{Args: []string{"관리자"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "1 만큼의 피해") {
		t.Fatalf("status/output = %d/%q, want capped damage", status, ctx.OutputString())
	}
	caretaker, _ := world.Creature("creature:caretaker")
	if got, want := caretaker.Stats["hpCurrent"], 9; got != want {
		t.Fatalf("caretaker hp = %d, want %d", got, want)
	}
}

func backstabDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "기습", Number: 45, Handler: "backstab"},
		{Name: "backstab", Number: 45, Handler: "backstab"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"backstab": NewBackstabHandler(world),
		},
	}
}

func backstabWorld(t *testing.T, class int) *worldload.World {
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
		Equipment:   map[string]model.ObjectInstanceID{"wield": "object:dagger"},
		Stats: map[string]int{
			"class":     class,
			"level":     20,
			"thaco":     0,
			"hpCurrent": 50,
			"hpMax":     50,
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
		Stats:       map[string]int{"level": 1, "armor": 0, "hpCurrent": 40, "hpMax": 40, "experience": 100},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:stone-guardian",
		Kind:        model.CreatureKindMonster,
		DisplayName: "수호석",
		RoomID:      "room:arena",
		Metadata:    model.Metadata{Tags: []string{"unkillable"}},
		Stats:       map[string]int{"level": 1, "armor": 0, "hpCurrent": 40, "hpMax": 40},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:wraith",
		Kind:        model.CreatureKindMonster,
		DisplayName: "망령",
		RoomID:      "room:arena",
		Metadata:    model.Metadata{Tags: []string{"magicOnly"}},
		Stats:       map[string]int{"level": 1, "armor": 0, "hpCurrent": 40, "hpMax": 40},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:caretaker",
		Kind:        model.CreatureKindMonster,
		DisplayName: "관리자",
		RoomID:      "room:arena",
		Stats:       map[string]int{"class": legacyClassCaretaker + 1, "level": 1, "armor": 0, "hpCurrent": 10, "hpMax": 10},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:dagger",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "단검",
		Properties:  map[string]string{"type": "0", "pDice": "5"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:dagger",
		PrototypeID: "prototype:dagger",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "wield"},
	})
	return loaded
}
