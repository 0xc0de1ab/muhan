package command

import (
	"strings"
	"testing"
	"time"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

type scratchDeathCall struct {
	playerID   model.PlayerID
	attackerID model.CreatureID
}

type scratchObserverWorld struct {
	*state.World
	deaths        []scratchDeathCall
	allBroadcasts []string
}

func (w *scratchObserverWorld) PlayerDeath(playerID model.PlayerID, attackerID model.CreatureID) error {
	w.deaths = append(w.deaths, scratchDeathCall{playerID: playerID, attackerID: attackerID})
	return nil
}

func (w *scratchObserverWorld) BroadcastAll(message string) error {
	w.allBroadcasts = append(w.allBroadcasts, message)
	return nil
}

func TestScratchHandlerScratchesLotteryAndStartsCooldown(t *testing.T) {
	withAttackRolls(t, 1, 2, 2, 2, 2)
	loaded := finishSkillsWorld(t)
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

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := NewScratchHandler(world)(ctx, ResolvedCommand{Args: []string{"복권"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "당신은 복권을 긁었습니다.") ||
		!strings.Contains(out, "5등에 당첨되었습니다.") ||
		!strings.Contains(out, "2000냥을 받았습니다.") {
		t.Fatalf("output = %q, want lottery win", out)
	}

	alice, _ = world.Creature("creature:alice")
	if got, want := creatureStat(alice, "hpCurrent"), 38; got != want {
		t.Fatalf("alice hp = %d, want %d", got, want)
	}
	if got, want := creatureStat(alice, "experience"), 988; got != want {
		t.Fatalf("alice experience = %d, want %d", got, want)
	}
	if got, want := creatureStat(alice, "gold"), 2010; got != want {
		t.Fatalf("alice gold = %d, want %d", got, want)
	}
	if hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN") || alice.Stats["PHIDDN"] != 0 {
		t.Fatalf("alice hidden state = tags:%+v stats:%+v, want cleared", alice.Metadata.Tags, alice.Stats)
	}
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "invisible", "PINVIS") || alice.Stats["PINVIS"] != 1 {
		t.Fatalf("alice invisible state = tags:%+v stats:%+v, want retained", alice.Metadata.Tags, alice.Stats)
	}
	player, _ = world.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "PHIDDN") {
		t.Fatalf("player hidden tags = %+v, want cleared", player.Metadata.Tags)
	}
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "PINVIS") {
		t.Fatalf("player invisible tags = %+v, want retained", player.Metadata.Tags)
	}
	if _, ok := world.Object("object:ticket"); ok {
		t.Fatal("lottery ticket still exists, want destroyed")
	}
	if len(broadcasts) < 2 {
		t.Fatalf("broadcasts = %+v, want scratch and result broadcasts", broadcasts)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", scratchCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active cooldown", used, remaining)
	}
}

func TestScratchHandlerRespectsCooldownBeforeRoomAndReveal(t *testing.T) {
	loaded := finishSkillsWorld(t)
	room := loaded.Rooms["room:arena"]
	room.Metadata.Tags = []string{"shoppe"}
	loaded.Rooms[room.ID] = room
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden")
	alice.Stats["PHIDDN"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = append(player.Metadata.Tags, "hidden")
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	defer world.Close()
	if err := world.SetCreatureCooldown("creature:alice", scratchCooldownKey, time.Now().Unix(), 10); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewScratchHandler(world)(ctx, ResolvedCommand{Args: []string{"복권"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "기다리세요") {
		t.Fatalf("status/output = %d/%q, want please-wait before room checks", status, ctx.OutputString())
	}
	if strings.Contains(ctx.OutputString(), "밖으로 나가서") {
		t.Fatalf("output = %q, want cooldown before shop rejection", ctx.OutputString())
	}
	updated, _ := world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "PHIDDN") || updated.Stats["PHIDDN"] != 1 {
		t.Fatalf("alice hidden state = tags %+v PHIDDN %d, want unchanged", updated.Metadata.Tags, updated.Stats["PHIDDN"])
	}
	if _, ok := world.Object("object:ticket"); !ok {
		t.Fatal("lottery ticket was consumed while cooldown was active")
	}
}

func TestScratchHandlerLargeWinBroadcastsAndRunsDeathHook(t *testing.T) {
	withAttackRolls(t, 1, 1, 1, 1, 1)
	loaded := finishSkillsWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["hpCurrent"] = 20
	alice.Stats["experience"] = 30
	loaded.Creatures[alice.ID] = alice
	world := &scratchObserverWorld{World: state.NewWorld(loaded)}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewScratchHandler(world)(ctx, ResolvedCommand{Args: []string{"복권"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault ||
		!strings.Contains(out, "1등에 당첨되었습니다.") ||
		!strings.Contains(out, "20000000냥을 받았습니다.") ||
		!strings.Contains(out, "당신은 죽으면서 몇가지 물건을 떨어뜨렸습니다.") {
		t.Fatalf("status/output = %d/%q, want jackpot and self-death output", status, out)
	}
	if len(world.allBroadcasts) != 1 || !strings.Contains(world.allBroadcasts[0], "축하합니다.") || !strings.Contains(world.allBroadcasts[0], "20000000냥") {
		t.Fatalf("global broadcasts = %+v, want jackpot broadcast", world.allBroadcasts)
	}
	if len(world.deaths) != 1 || world.deaths[0].playerID != "player:alice" || world.deaths[0].attackerID != "creature:alice" {
		t.Fatalf("death calls = %+v, want self death", world.deaths)
	}
}

func TestScratchHandlerRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		mutate func(*worldload.World)
		want   string
	}{
		{
			name: "low level",
			args: []string{"복권"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Level = 19
				alice.Stats["level"] = 19
				alice.Stats["class"] = model.ClassFighter
				loaded.Creatures[alice.ID] = alice
			},
			want: "당신의 레벨로는 복권을 긁을 수 없습니다.",
		},
		{name: "missing target", want: "무엇을 긁으시려구요?"},
		{
			name: "legacy square room alias",
			args: []string{"복권"},
			mutate: func(loaded *worldload.World) {
				room := loaded.Rooms["room:arena"]
				delete(loaded.Rooms, room.ID)
				room.ID = "room:01001"
				loaded.Rooms[room.ID] = room
				alice := loaded.Players["player:alice"]
				alice.RoomID = room.ID
				loaded.Players[alice.ID] = alice
				creature := loaded.Creatures["creature:alice"]
				creature.RoomID = room.ID
				loaded.Creatures[creature.ID] = creature
			},
			want: "광장에서는 복권을 긁을 수 없습니다.",
		},
		{
			name: "shop room",
			args: []string{"복권"},
			mutate: func(loaded *worldload.World) {
				room := loaded.Rooms["room:arena"]
				room.Metadata.Tags = []string{"shoppe"}
				loaded.Rooms[room.ID] = room
			},
			want: "밖으로 나가서 긁어 주세요.",
		},
		{name: "unknown object", args: []string{"없는"}, want: "당신은 복권을 갖고 있지 않습니다."},
		{name: "not lottery", args: []string{"돌"}, want: "그것은 복권이 아닙니다."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := finishSkillsWorld(t)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
	defer world.Close()

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewScratchHandler(world)(ctx, ResolvedCommand{Args: tt.args, Values: []int64{1}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestScratchHandlerCanBeRegisteredByDispatcherAliases(t *testing.T) {
	for _, line := range []string{"복권 긁어", "scratch 복권"} {
		t.Run(line, func(t *testing.T) {
			withAttackRolls(t, 2, 2, 2, 2, 2)
			world := state.NewWorld(finishSkillsWorld(t))
	defer world.Close()
			dispatcher := finishSkillsDispatcher(t, world)

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, line)
			if err != nil {
				t.Fatalf("DispatchLine(%q) error = %v", line, err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), "꽝~~ 입니다.") {
				t.Fatalf("status/output = %d/%q, want scratch alias success", status, ctx.OutputString())
			}
		})
	}
}

func TestSasalHandlerPerfectlyKillsWeakMonsterAndStartsCooldown(t *testing.T) {
	withAttackRolls(t, 1, 20)
	loaded := finishSkillsWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"YELLOWI", "hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := finishSkillsDispatcher(t, world)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := dispatcher.DispatchLine(ctx, "약한고블린 확인사살")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "당신의 모습이 서서히 드러납니다.") ||
		!strings.Contains(out, "완벽한 확인사살로 약한고블린에게 10점") ||
		!strings.Contains(out, "당신은 약한고블린을 죽였습니다.") {
		t.Fatalf("output = %q, want perfect sasal kill", out)
	}
	if _, ok := world.Creature("creature:weak-goblin"); ok {
		t.Fatal("weak goblin still exists, want finalized monster death")
	}
	alice, _ = world.Creature("creature:alice")
	if hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 0 || alice.Stats["PINVIS"] != 0 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible cleared", alice.Metadata.Tags, alice.Stats)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", sasalCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active cooldown", used, remaining)
	}
	if len(broadcasts) < 3 {
		t.Fatalf("broadcasts = %+v, want reveal, attack, and death broadcasts", broadcasts)
	}
}

func TestSasalHandlerAwkwardHitDamagesStrongMonster(t *testing.T) {
	withAttackRolls(t, 1, 20, 5)
	loaded := finishSkillsWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"YELLOWI"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewSasalHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "어색한 확인사살로 고블린에게 31점") {
		t.Fatalf("status/output = %d/%q, want awkward sasal damage", status, ctx.OutputString())
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 49; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
}

func TestSasalHandlerMagicOnlyUsesLegacyPietyAndRoll(t *testing.T) {
	withAttackRolls(t, 0, 1, 20, 5)
	loaded := finishSkillsWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"YELLOWI"}
	alice.Stats["piety"] = 10
	loaded.Creatures[alice.ID] = alice
	wraith := loaded.Creatures["creature:wraith"]
	wraith.Stats["piety"] = 99
	loaded.Creatures[wraith.ID] = wraith
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewSasalHandler(world)(ctx, ResolvedCommand{Args: []string{"망령"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "어색한 확인사살로 망령에게 31점") {
		t.Fatalf("status/output = %d/%q, want magicOnly pass-through on legacy roll 0", status, ctx.OutputString())
	}
	wraith, _ = world.Creature("creature:wraith")
	if got, want := creatureStat(wraith, "hpCurrent"), 9; got != want {
		t.Fatalf("wraith hp = %d, want %d after non-deflected sasal", got, want)
	}
}

func TestSasalHandlerEnchantOnlyUsesLegacyRoll(t *testing.T) {
	withAttackRolls(t, 0, 1, 20, 5)
	loaded := finishSkillsWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"YELLOWI"}
	loaded.Creatures[alice.ID] = alice
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Metadata.Tags = []string{"MENONL"}
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewSasalHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "어색한 확인사살로 고블린에게 31점") {
		t.Fatalf("status/output = %d/%q, want MENONL pass-through on legacy roll 0", status, ctx.OutputString())
	}
	goblin, _ = world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 49; got != want {
		t.Fatalf("goblin hp = %d, want %d after non-deflected sasal", got, want)
	}
}

func TestSasalHandlerRejectsCharmedPlayerLikeLegacy(t *testing.T) {
	loaded := finishSkillsWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"YELLOWI", "PCHAOS", "hidden", "PHIDDN", "invisible", "PINVIS"}
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
	status, err := NewSasalHandler(world)(ctx, ResolvedCommand{Args: []string{"Bob"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "너무 사랑") {
		t.Fatalf("status/output = %d/%q, want legacy charm refusal", status, ctx.OutputString())
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", sasalCooldownKey, time.Now().Unix(), 0); err != nil || !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining/err = %v/%d/%v, want unused", used, remaining, err)
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 1 || alice.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained", alice.Metadata.Tags, alice.Stats)
	}
}

func TestSasalHandlerRespectsCooldownBeforeTargetWeaponAndReveal(t *testing.T) {
	loaded := finishSkillsWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"YELLOWI", "hidden", "invisible"}
	alice.Equipment = nil
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "invisible"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	defer world.Close()
	if err := world.SetCreatureCooldown("creature:alice", sasalCooldownKey, time.Now().Unix(), 10); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewSasalHandler(world)(ctx, ResolvedCommand{Args: []string{"없는"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "기다리세요") {
		t.Fatalf("status/output = %d/%q, want please-wait before target lookup", status, out)
	}
	for _, unexpected := range []string{"그런 것은", "궁 종류", "모습이 서서히"} {
		if strings.Contains(out, unexpected) {
			t.Fatalf("output = %q, did not want %q before cooldown", out, unexpected)
		}
	}
	updated, _ := world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "invisible") {
		t.Fatalf("alice tags = %+v, want hidden/invisible unchanged", updated.Metadata.Tags)
	}
}

func TestSasalHandlerInvalidTargetDoesNotConsumeCooldown(t *testing.T) {
	loaded := finishSkillsWorld(t)
	finishSkillsGiveAliceYellow(loaded)
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewSasalHandler(world)(ctx, ResolvedCommand{Args: []string{"없는"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "그런 것은 여기 없습니다.") {
		t.Fatalf("status/output = %d/%q, want missing target", status, ctx.OutputString())
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", sasalCooldownKey, time.Now().Unix(), 0); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want no consumed cooldown", used, remaining)
	}
}

func TestSasalHandlerFailureStartsCooldownWithoutDamage(t *testing.T) {
	withAttackRolls(t, 100)
	loaded := finishSkillsWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"YELLOWI"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewSasalHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "당신의 확인사살이 실패했습니다.") {
		t.Fatalf("status/output = %d/%q, want sasal failure", status, ctx.OutputString())
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 80; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	enemies, err := world.CreatureEnemies("creature:goblin")
	if err != nil {
		t.Fatalf("CreatureEnemies() error = %v", err)
	}
	if len(enemies) != 1 || enemies[0] != "Alice" {
		t.Fatalf("goblin enemies = %+v, want Alice from failed sasal", enemies)
	}
	if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "was_attacked") {
		t.Fatalf("goblin tags = %+v, want was_attacked after failed sasal", goblin.Metadata.Tags)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", sasalCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active cooldown", used, remaining)
	}
}

func TestSasalChanceMatchesLegacyLevelDeltaFormula(t *testing.T) {
	actor := model.Creature{
		Level: 50,
		Stats: map[string]int{
			"level":        50,
			"intelligence": 0,
			"dexterity":    0,
		},
	}
	victim := model.Creature{
		Level: 1,
		Stats: map[string]int{"level": 1},
	}

	if got, want := sasalChance(actor, victim), 38; got != want {
		t.Fatalf("sasalChance() = %d, want C level-delta formula result %d", got, want)
	}
}

func TestSasalHandlerRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		rolls  []int
		mutate func(*worldload.World)
		want   string
	}{
		{name: "missing target", want: "누굴 공격합니까?"},
		{
			name: "blind",
			args: []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"YELLOWI", "PBLIND"}
				loaded.Creatures[alice.ID] = alice
			},
			want: "누굴 공격합니까?",
		},
		{name: "missing yellow training", args: []string{"고블린"}, want: "노랑초인 이상만 쓸수 있는 기술입니다."},
		{
			name: "unknown target",
			args: []string{"없는"},
			mutate: func(loaded *worldload.World) {
				finishSkillsGiveAliceYellow(loaded)
			},
			want: "그런 것은 여기 없습니다.",
		},
		{
			name: "no missile weapon",
			args: []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				finishSkillsGiveAliceYellow(loaded)
				alice := loaded.Creatures["creature:alice"]
				alice.Equipment = nil
				loaded.Creatures[alice.ID] = alice
			},
			want: "궁 종류의 무기가 필요합니다.",
		},
		{
			name: "protected monster",
			args: []string{"수호석"},
			mutate: func(loaded *worldload.World) {
				finishSkillsGiveAliceYellow(loaded)
			},
			want: "당신은 그 상대를 해칠 수 없습니다.",
		},
		{
			name: "magic only monster",
			args: []string{"망령"},
			rolls: []int{
				1,
			},
			mutate: func(loaded *worldload.World) {
				finishSkillsGiveAliceYellow(loaded)
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["piety"] = 10
				loaded.Creatures[alice.ID] = alice
				wraith := loaded.Creatures["creature:wraith"]
				wraith.Stats["piety"] = 99
				loaded.Creatures[wraith.ID] = wraith
			},
			want: "아무 소용이 없는듯 합니다.",
		},
		{
			name: "player kill gate",
			args: []string{"Bob"},
			mutate: func(loaded *worldload.World) {
				finishSkillsGiveAliceYellow(loaded)
			},
			want: "당신은 선해서 다른 사용자를 공격할 수 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.rolls) > 0 {
				withAttackRolls(t, tt.rolls...)
			}
			loaded := finishSkillsWorld(t)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
	defer world.Close()

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewSasalHandler(world)(ctx, ResolvedCommand{Args: tt.args, Values: []int64{1}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func finishSkillsDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()
	return Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "긁어", Number: 155, Handler: "scratch"},
			{Name: "scratch", Number: 155, Handler: "scratch"},
			{Name: "확인사살", Number: 175, Handler: "sasal"},
			{Name: "sasal", Number: 175, Handler: "sasal"},
		}),
		Handlers: map[string]Handler{
			"scratch": NewScratchHandler(world),
			"sasal":   NewSasalHandler(world),
		},
	}
}

func finishSkillsGiveAliceYellow(loaded *worldload.World) {
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "YELLOWI")
	loaded.Creatures[alice.ID] = alice
}

func finishSkillsWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:arena",
		DisplayName: "Arena",
		PlayerIDs:   []model.PlayerID{"player:alice", "player:bob"},
		CreatureIDs: []model.CreatureID{
			"creature:goblin",
			"creature:weak-goblin",
			"creature:stone-guardian",
			"creature:wraith",
		},
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
		Level:       60,
		Inventory:   model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:ticket", "object:stone"}},
		Equipment:   map[string]model.ObjectInstanceID{"wield": "object:bow"},
		Stats: map[string]int{
			"class":        model.ClassInvincible,
			"level":        60,
			"strength":     20,
			"dexterity":    20,
			"intelligence": 20,
			"thaco":        0,
			"hpCurrent":    50,
			"hpMax":        50,
			"experience":   1000,
			"gold":         10,
			"pDice":        3,
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
		ID:          "creature:goblin",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:arena",
		Stats:       map[string]int{"level": 1, "dexterity": 10, "armor": 0, "hpCurrent": 80, "hpMax": 120, "experience": 100},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:weak-goblin",
		Kind:        model.CreatureKindMonster,
		DisplayName: "약한고블린",
		RoomID:      "room:arena",
		Stats:       map[string]int{"level": 1, "dexterity": 10, "armor": 0, "hpCurrent": 10, "hpMax": 30, "experience": 100},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:stone-guardian",
		Kind:        model.CreatureKindMonster,
		DisplayName: "수호석",
		RoomID:      "room:arena",
		Metadata:    model.Metadata{Tags: []string{"unkillable"}},
		Stats:       map[string]int{"level": 1, "dexterity": 10, "armor": 0, "hpCurrent": 40, "hpMax": 40},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:wraith",
		Kind:        model.CreatureKindMonster,
		DisplayName: "망령",
		RoomID:      "room:arena",
		Metadata:    model.Metadata{Tags: []string{"magicOnly"}},
		Stats:       map[string]int{"level": 1, "dexterity": 10, "armor": 0, "hpCurrent": 40, "hpMax": 40},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:bow",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "목궁",
		Properties:  map[string]string{"type": "4", "pDice": "5"},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:ticket",
		Kind:        model.ObjectKindMisc,
		DisplayName: "복권",
		Keywords:    []string{"복권"},
		Properties:  map[string]string{"type": "15", "nDice": "5", "sDice": "2", "pDice": "0", "value": "1000"},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:stone",
		Kind:        model.ObjectKindMisc,
		DisplayName: "돌",
		Keywords:    []string{"돌"},
		Properties:  map[string]string{"type": "13"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:bow",
		PrototypeID: "prototype:bow",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "wield"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:ticket",
		PrototypeID: "prototype:ticket",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:stone",
		PrototypeID: "prototype:stone",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	})
	return loaded
}
