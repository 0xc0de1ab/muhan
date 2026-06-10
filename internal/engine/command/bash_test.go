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

func TestBashHandlerBashesMonsterAndRevealsInvisibleActorOnlyLikeLegacy(t *testing.T) {
	withAttackRolls(t, 1, 20, 5)
	loaded := bashWorld(t, model.ClassFighter)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	dispatcher := bashDispatcher(t, world)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := dispatcher.DispatchLine(ctx, "고블린 맹공")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "당신의 모습이 서서히 드러납니다.") ||
		!strings.Contains(out, "당신의 칼을 휘둘러 20점의 맹공을 가했습니다.") {
		t.Fatalf("output = %q, want reveal and bash damage", out)
	}
	if len(broadcasts) != 2 ||
		broadcasts[0].Text != "\nAlice의 모습이 서서히 드러납니다." ||
		broadcasts[1].Text != "\nAlice이 칼을 휘둘러 고블린에게 맹공을 가합니다." {
		t.Fatalf("broadcasts = %+v, want C reveal and bash broadcasts", broadcasts)
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 40; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "befuddled", "MBEFUD") {
		t.Fatalf("goblin tags = %+v, want befuddled", goblin.Metadata.Tags)
	}
	if remaining, ready, err := world.UseCreatureCooldown(goblin.ID, "befuddled", time.Now().Unix(), 0); err != nil || ready || remaining <= 0 {
		t.Fatalf("befuddled cooldown remaining/ready/err = %d/%v/%v, want active cooldown", remaining, ready, err)
	}
	if remaining, ready, err := world.UseCreatureCooldown("creature:alice", bashCooldownKey, time.Now().Unix(), 0); err != nil || ready || remaining <= 0 {
		t.Fatalf("bash cooldown remaining/ready/err = %d/%v/%v, want active hit cooldown", remaining, ready, err)
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN") ||
		hasAnyNormalizedFlag(alice.Metadata.Tags, "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 1 || alice.Stats["PINVIS"] != 0 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden retained and invisible cleared", alice.Metadata.Tags, alice.Stats)
	}
	player, _ = world.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "PHIDDN") ||
		hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "PINVIS") {
		t.Fatalf("player tags = %+v, want hidden retained and invisible cleared", player.Metadata.Tags)
	}
}

func TestBashHandlerSupportsCommandFirstFallback(t *testing.T) {
	withAttackRolls(t, 1, 20, 5)
	world := state.NewWorld(bashWorld(t, model.ClassFighter))
	dispatcher := bashDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "bash 고블린")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 40; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
}

func TestBashHandlerReportsFailureWithoutDamage(t *testing.T) {
	withAttackRolls(t, 100)
	world := state.NewWorld(bashWorld(t, model.ClassFighter))
	handler := NewBashHandler(world)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "당신의 맹공이 실패했습니다.") {
		t.Fatalf("status/output = %d/%q, want failure", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0].Text != "\nAlice이 고블린에게 맹공을 가하려 합니다." {
		t.Fatalf("broadcasts = %+v, want failure broadcast", broadcasts)
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 60; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	if remaining, ready, err := world.UseCreatureCooldown("creature:alice", bashCooldownKey, time.Now().Unix(), 0); err != nil || ready || remaining <= 0 {
		t.Fatalf("bash cooldown remaining/ready/err = %d/%v/%v, want active attempt cooldown", remaining, ready, err)
	}
}

func TestBashHandlerIgnoresSkillProficiencyLikeLegacy(t *testing.T) {
	withAttackRolls(t, 50, 20, 5)
	loaded := bashWorld(t, model.ClassFighter)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["level"] = 1
	alice.Stats["strength"] = 10
	alice.Stats["dexterity"] = 10
	alice.Properties = map[string]string{
		"proficiency/bash":   "2000",
		"proficiency/thrust": "2000",
		"proficiency/1":      "2000",
	}
	loaded.Creatures[alice.ID] = alice
	goblin := loaded.Creatures["creature:goblin-1"]
	goblin.Stats["level"] = 20
	goblin.Stats["dexterity"] = 20
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBashHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "당신의 맹공이 실패했습니다.") {
		t.Fatalf("status/output = %d/%q, want C chance failure", status, ctx.OutputString())
	}
	goblin, _ = world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 60; got != want {
		t.Fatalf("goblin hp = %d, want unchanged %d", got, want)
	}
	alice, _ = world.Creature("creature:alice")
	for _, key := range []string{"proficiency/bash", "proficiency/thrust", "proficiency/1"} {
		if got, want := alice.Properties[key], "2000"; got != want {
			t.Fatalf("%s = %q, want unchanged %q", key, got, want)
		}
	}
}

func TestBashHandlerMonsterHitAddsLegacyWeaponProficiency(t *testing.T) {
	withAttackRolls(t, 1, 20, 5)
	loaded := bashWorld(t, model.ClassFighter)
	goblin := loaded.Creatures["creature:goblin-1"]
	goblin.Stats["experience"] = 120
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBashHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "20점의 맹공") {
		t.Fatalf("status/output = %d/%q, want successful bash", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got, want := alice.Properties["proficiency/thrust"], "42"; got != want {
		t.Fatalf("proficiency/thrust = %q, want %q", got, want)
	}
	if got, want := alice.Properties["proficiency/1"], "42"; got != want {
		t.Fatalf("proficiency/1 = %q, want %q", got, want)
	}
	if got := alice.Properties["proficiency/bash"]; got != "" {
		t.Fatalf("proficiency/bash = %q, want absent", got)
	}
}

func TestBashHandlerHighClassTargetUsesRawDamageForLegacyProficiency(t *testing.T) {
	withAttackRolls(t, 1, 20, 5)
	loaded := bashWorld(t, model.ClassFighter)
	goblin := loaded.Creatures["creature:goblin-1"]
	goblin.Stats["class"] = model.ClassCaretaker + 1
	goblin.Stats["experience"] = 120
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBashHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "1점의 맹공") {
		t.Fatalf("status/output = %d/%q, want C high-class one-point damage", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got, want := alice.Properties["proficiency/thrust"], "42"; got != want {
		t.Fatalf("proficiency/thrust = %q, want raw-damage gain %q", got, want)
	}
	if got, want := alice.Properties["proficiency/1"], "42"; got != want {
		t.Fatalf("proficiency/1 = %q, want raw-damage gain %q", got, want)
	}
}

func TestBashHandlerRespectsCooldownBeforeReveal(t *testing.T) {
	loaded := bashWorld(t, model.ClassFighter)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	if err := world.SetCreatureCooldown("creature:alice", bashCooldownKey, time.Now().Unix(), 2); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBashHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
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

func TestBashHandlerRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name   string
		class  int
		tags   []string
		args   []string
		mutate func(*worldload.World)
		want   string
	}{
		{name: "missing target", class: model.ClassFighter, want: "누굴 공격합니까?"},
		{name: "blind", class: model.ClassFighter, tags: []string{"blind"}, args: []string{"고블린"}, want: "누굴 공격합니까?"},
		{name: "wrong class", class: model.ClassThief, args: []string{"고블린"}, want: "검사만 쓸수 있는 기술입니다."},
		{name: "invincible without training", class: model.ClassInvincible, args: []string{"고블린"}, want: "검사를 무적수련하지 않았습니다.."},
		{name: "unknown target", class: model.ClassFighter, args: []string{"없는"}, want: "그런 것은 여기 없습니다."},
		{
			name:  "missing weapon",
			class: model.ClassFighter,
			args:  []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Equipment = nil
				loaded.Creatures[alice.ID] = alice
			},
			want: "맹공을 하시려면 도나 검종류의 무기가 필요합니다.",
		},
		{
			name:  "blunt weapon",
			class: model.ClassFighter,
			args:  []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				sword := loaded.ObjectPrototypes["prototype:sword"]
				sword.Properties["type"] = "2"
				loaded.ObjectPrototypes[sword.ID] = sword
			},
			want: "맹공을 하시려면 도나 검종류의 무기가 필요합니다.",
		},
		{
			name:  "protected monster",
			class: model.ClassFighter,
			args:  []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				goblin := loaded.Creatures["creature:goblin-1"]
				goblin.Metadata.Tags = []string{"unkillable"}
				loaded.Creatures[goblin.ID] = goblin
			},
			want: "당신은 그녀를 해칠 수 없습니다.",
		},
		{
			name:  "short player target",
			class: model.ClassFighter,
			args:  []string{"Bo"},
			want:  "그런 것은 여기 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := bashWorld(t, tt.class)
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = tt.tags
			loaded.Creatures[alice.ID] = alice
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
			handler := NewBashHandler(world)

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

func TestBashHandlerSkipsPlayerPKGateLikeLegacyElseIf(t *testing.T) {
	withAttackRolls(t, 1, 20, 5)
	world := state.NewWorld(bashWorld(t, model.ClassFighter))

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBashHandler(world)(ctx, ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault ||
		strings.Contains(ctx.OutputString(), "선해서") ||
		!strings.Contains(ctx.OutputString(), "당신의 칼을 휘둘러") {
		t.Fatalf("status/output = %d/%q, want C-style player bash without PK gate", status, ctx.OutputString())
	}
	bob, _ := world.Creature("creature:bob")
	if got := bob.Stats["hpCurrent"]; got >= 30 {
		t.Fatalf("bob hp = %d, want bash damage applied", got)
	}
}

func TestBashHandlerUsesLegacyByteLengthForPlayerTarget(t *testing.T) {
	withAttackRolls(t, 1, 20, 5)
	loaded := bashWorld(t, model.ClassFighter)
	bobPlayer := loaded.Players["player:bob"]
	bobPlayer.DisplayName = "보브"
	loaded.Players[bobPlayer.ID] = bobPlayer
	bob := loaded.Creatures["creature:bob"]
	bob.DisplayName = "보브"
	loaded.Creatures[bob.ID] = bob
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBashHandler(world)(ctx, ResolvedCommand{Args: []string{"보브"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "당신의 칼을 휘둘러") {
		t.Fatalf("status/output = %d/%q, want Korean two-rune target accepted", status, ctx.OutputString())
	}
}

func TestBashHandlerEnchantOnlyMonsterRequiresEnchantedWeaponForCaretakerLikeLegacy(t *testing.T) {
	withAttackRolls(t, 1, 20, 5)
	loaded := bashWorld(t, model.ClassCaretaker)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SFIGHTER"}
	loaded.Creatures[alice.ID] = alice
	goblin := loaded.Creatures["creature:goblin-1"]
	goblin.Metadata.Tags = []string{"MENONL"}
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBashHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "당신의 무기가 고블린에게 아무 소용이 없는듯 합니다.") {
		t.Fatalf("status/output = %d/%q, want C bash enchant-only refusal", status, ctx.OutputString())
	}
	goblin, _ = world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 60; got != want {
		t.Fatalf("goblin hp = %d, want unchanged %d", got, want)
	}
	if remaining, ready, err := world.UseCreatureCooldown("creature:alice", bashCooldownKey, time.Now().Unix(), 0); err != nil || ready || remaining <= 0 {
		t.Fatalf("bash cooldown remaining/ready/err = %d/%v/%v, want consumed before MENONL refusal", remaining, ready, err)
	}
}

func TestBashHandlerInvincibleWithFighterTrainingCanBash(t *testing.T) {
	withAttackRolls(t, 1, 20, 5)
	loaded := bashWorld(t, model.ClassInvincible)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SFIGHTER"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBashHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "당신의 칼을 휘둘러 20점의 맹공을 가했습니다.") {
		t.Fatalf("status/output = %d/%q, want successful bash", status, ctx.OutputString())
	}
}

func TestBashHandlerPreservesLegacyZeroDamageForVeryLowHPTarget(t *testing.T) {
	withAttackRolls(t, 1, 20, 5)
	loaded := bashWorld(t, model.ClassFighter)
	goblin := loaded.Creatures["creature:goblin-1"]
	goblin.Stats["hpCurrent"] = 2
	goblin.Stats["hpMax"] = 2
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBashHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "0점의 맹공") {
		t.Fatalf("status/output = %d/%q, want legacy zero damage", status, ctx.OutputString())
	}
	goblin, _ = world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 2; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
}

func TestBashHandlerFinalizesMonsterDeathWhenDamageKillsTarget(t *testing.T) {
	withAttackRolls(t, 1, 20, 5)
	loaded := bashWorld(t, model.ClassFighter)
	goblin := loaded.Creatures["creature:goblin-1"]
	goblin.Stats["class"] = model.ClassCaretaker + 1
	goblin.Stats["hpCurrent"] = 1
	goblin.Stats["hpMax"] = 1
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBashHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault ||
		!strings.Contains(ctx.OutputString(), "1점의 맹공") ||
		!strings.Contains(ctx.OutputString(), "당신은 고블린을 죽였습니다.") {
		t.Fatalf("status/output = %d/%q, want bash death output", status, ctx.OutputString())
	}
	if _, ok := world.Creature("creature:goblin-1"); ok {
		t.Fatal("goblin still exists, want finalized monster death")
	}
}

func bashDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "맹공", Number: 51, Handler: "bash"},
		{Name: "bash", Number: 51, Handler: "bash"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"bash": NewBashHandler(world),
		},
	}
}

func bashWorld(t *testing.T, class int) *worldload.World {
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
		Equipment:   map[string]model.ObjectInstanceID{"wield": "object:sword"},
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
		Stats:       map[string]int{"class": model.ClassFighter, "level": 1, "hpCurrent": 30, "hpMax": 30},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:goblin-1",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:arena",
		Stats:       map[string]int{"level": 1, "dexterity": 10, "armor": 0, "hpCurrent": 60, "hpMax": 60},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:sword",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "목검",
		Properties:  map[string]string{"type": "1", "pDice": "5"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:sword",
		PrototypeID: "prototype:sword",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "wield"},
	})
	return loaded
}
