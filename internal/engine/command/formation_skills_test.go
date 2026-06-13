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

func TestEightHandlerDamagesRoomTargetsRevealsAndStartsCooldown(t *testing.T) {
	withAttackRolls(t, 1)
	loaded := formationSkillsWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SFIGHTER", "hidden", "PHIDDN", "invisible", "PINVIS"}
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
	status, err := NewEightHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "당신의 모습이 서서히 드러납니다.") ||
		!strings.Contains(out, "고블린에게 29의 피해") ||
		!strings.Contains(out, "오크에게 29의 피해") {
		t.Fatalf("output = %q, want reveal and two eight hits", out)
	}

	goblin, _ := world.Creature("creature:goblin")
	orc, _ := world.Creature("creature:orc")
	hidden, _ := world.Creature("creature:hidden")
	protected, _ := world.Creature("creature:protected")
	if got, want := creatureStat(goblin, "hpCurrent"), 51; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	if got, want := creatureStat(orc, "hpCurrent"), 51; got != want {
		t.Fatalf("orc hp = %d, want %d", got, want)
	}
	if got, want := creatureStat(hidden, "hpCurrent"), 80; got != want {
		t.Fatalf("hidden hp = %d, want untouched %d", got, want)
	}
	if got, want := creatureStat(protected, "hpCurrent"), 80; got != want {
		t.Fatalf("protected hp = %d, want untouched %d", got, want)
	}
	if len(broadcasts) < 3 || !strings.Contains(broadcasts[0].Text, "서서히 드러납니다") ||
		!strings.Contains(broadcasts[1].Text, "고블린에게 29") ||
		!strings.Contains(broadcasts[2].Text, "오크에게 29") {
		t.Fatalf("broadcasts = %+v, want reveal and damage broadcasts", broadcasts)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", eightCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active eight cooldown", used, remaining)
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

func TestEightHandlerFinalizesMonsterDeath(t *testing.T) {
	withAttackRolls(t, 1)
	loaded := formationSkillsWorld(t)
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Stats["hpCurrent"] = 10
	goblin.Stats["hpMax"] = 10
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewEightHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "영자팔법으로 고블린을 죽였습니다") {
		t.Fatalf("status/output = %d/%q, want eight death", status, ctx.OutputString())
	}
	if _, ok := world.Creature("creature:goblin"); ok {
		t.Fatal("dead goblin still exists, want finalized monster death")
	}
}

func TestEightHandlerFailurePrimesTargetsAndStartsCooldown(t *testing.T) {
	withAttackRolls(t, 22)
	world := state.NewWorld(formationSkillsWorld(t))
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewEightHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "기세에 눌려 실패했습니다") {
		t.Fatalf("status/output = %d/%q, want eight failure", status, ctx.OutputString())
	}
	for _, id := range []model.CreatureID{"creature:goblin", "creature:orc"} {
		target, _ := world.Creature(id)
		if got, want := creatureStat(target, "hpCurrent"), 80; got != want {
			t.Fatalf("%s hp = %d, want unchanged %d", id, got, want)
		}
		if !hasAnyNormalizedFlag(target.Metadata.Tags, "was_attacked") {
			t.Fatalf("%s tags = %+v, want was_attacked after failed eight", id, target.Metadata.Tags)
		}
		enemies, err := world.CreatureEnemies(id)
		if err != nil {
			t.Fatalf("CreatureEnemies(%q) error = %v", id, err)
		}
		if len(enemies) != 1 || enemies[0] != "Alice" {
			t.Fatalf("%s enemies = %+v, want Alice from failed eight", id, enemies)
		}
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", eightCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active eight cooldown", used, remaining)
	}
}

func TestEightHandlerCooldownPrecedesWeaponTargetScanAndReveal(t *testing.T) {
	loaded := formationSkillsWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS")
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	alice.Equipment = nil
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	for _, id := range []model.CreatureID{"creature:goblin", "creature:orc"} {
		creature := loaded.Creatures[id]
		creature.Metadata.Tags = []string{"MUNKIL"}
		loaded.Creatures[id] = creature
	}
	world := state.NewWorld(loaded)
	defer world.Close()
	if err := world.SetCreatureCooldown("creature:alice", eightCooldownKey, time.Now().Unix(), eightCooldownSeconds(alice)); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewEightHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "기다리세요.") ||
		strings.Contains(out, "검종류의 무기가 필요합니다") ||
		strings.Contains(out, "공격할 적이 없습니다") {
		t.Fatalf("status/output = %d/%q, want cooldown before weapon/target checks", status, out)
	}
	updated, _ := world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "PHIDDN") ||
		!hasAnyNormalizedFlag(updated.Metadata.Tags, "invisible", "PINVIS") ||
		updated.Stats["PHIDDN"] != 1 || updated.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained during cooldown", updated.Metadata.Tags, updated.Stats)
	}
}

func TestEightHandlerPreAttemptRejectionsDoNotStartCooldown(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*worldload.World)
	}{
		{
			name: "missing weapon",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Equipment = nil
				loaded.Creatures[alice.ID] = alice
			},
		},
		{
			name: "no targets",
			mutate: func(loaded *worldload.World) {
				for _, id := range []model.CreatureID{"creature:goblin", "creature:orc"} {
					creature := loaded.Creatures[id]
					creature.Metadata.Tags = []string{"MUNKIL"}
					loaded.Creatures[id] = creature
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := formationSkillsWorld(t)
			tt.mutate(loaded)
			world := state.NewWorld(loaded)
	defer world.Close()

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewEightHandler(world)(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want default", status)
			}
			if remaining, used, err := world.UseCreatureCooldown("creature:alice", eightCooldownKey, time.Now().Unix(), 0); err != nil {
				t.Fatalf("UseCreatureCooldown() error = %v", err)
			} else if !used || remaining != 0 {
				t.Fatalf("cooldown used/remaining = %v/%d, want no eight cooldown", used, remaining)
			}
		})
	}
}

func TestEightHandlerWeaponBreakOnSuccessDoesNotStartCooldown(t *testing.T) {
	withAttackRolls(t, 1)
	loaded := formationSkillsWorld(t)
	sword := loaded.Objects["object:sword"]
	if sword.Properties == nil {
		sword.Properties = map[string]string{}
	}
	sword.Properties["shotsCurrent"] = "1"
	loaded.Objects[sword.ID] = sword
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewEightHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "부서져 버렸습니다.") {
		t.Fatalf("status/output = %d/%q, want weapon break", status, out)
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 80; got != want {
		t.Fatalf("goblin hp = %d, want unchanged after weapon break %d", got, want)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", eightCooldownKey, time.Now().Unix(), 0); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want no cooldown after break-return", used, remaining)
	}
}

func TestEightHandlerRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*worldload.World)
		want   string
	}{
		{
			name: "wrong class",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["class"] = model.ClassFighter
				loaded.Creatures[alice.ID] = alice
			},
			want: "무적이상만 쓸수 있는 기술입니다.",
		},
		{
			name: "missing training",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = nil
				loaded.Creatures[alice.ID] = alice
			},
			want: "검사를 무적수련하지 않았습니다.",
		},
		{
			name: "missing weapon",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Equipment = nil
				loaded.Creatures[alice.ID] = alice
			},
			want: "검종류의 무기가 필요합니다.",
		},
		{
			name: "wrong weapon type",
			mutate: func(loaded *worldload.World) {
				sword := loaded.Objects["object:sword"]
				sword.Properties = map[string]string{"type": "0", "pDice": "5"}
				loaded.Objects[sword.ID] = sword
			},
			want: "검종류의 무기가 필요합니다.",
		},
		{
			name: "no targets",
			mutate: func(loaded *worldload.World) {
				for _, id := range []model.CreatureID{"creature:goblin", "creature:orc"} {
					creature := loaded.Creatures[id]
					creature.Metadata.Tags = []string{"MUNKIL"}
					loaded.Creatures[id] = creature
				}
			},
			want: "이 방에는 당신이 공격할 적이 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := formationSkillsWorld(t)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
	defer world.Close()
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewEightHandler(world)(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestNahanHandlerDamagesTargetWithVisibleCompanionAndStartsCooldown(t *testing.T) {
	withAttackRolls(t, 1, 5, 6)
	loaded := formationSkillsWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS")
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
	status, err := NewNahanHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "고블린에게 52점의 피해") {
		t.Fatalf("status/output = %d/%q, want nahan damage", status, ctx.OutputString())
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 28; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	if len(broadcasts) == 0 || !strings.Contains(broadcasts[len(broadcasts)-1].Text, "나한진을 펼쳐 고블린에게 52") {
		t.Fatalf("broadcasts = %+v, want nahan damage broadcast", broadcasts)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", nahanCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active nahan cooldown", used, remaining)
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

func TestNahanHandlerCountsHiddenCompanionLikeLegacy(t *testing.T) {
	withAttackRolls(t, 1, 5, 6)
	loaded := formationSkillsWorld(t)
	bob := loaded.Creatures["creature:bob"]
	bob.Metadata.Tags = append(bob.Metadata.Tags, "hidden", "PHIDDN")
	bob.Stats["PHIDDN"] = 1
	loaded.Creatures[bob.ID] = bob
	bobPlayer := loaded.Players["player:bob"]
	bobPlayer.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Players[bobPlayer.ID] = bobPlayer
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewNahanHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "고블린에게 52점의 피해") || strings.Contains(out, "혼자서는") {
		t.Fatalf("status/output = %d/%q, want hidden companion to count in nahan", status, out)
	}
	goblin, _ := world.Creature("creature:goblin")
	enemies, err := world.CreatureEnemies(goblin.ID)
	if err != nil {
		t.Fatalf("CreatureEnemies() error = %v", err)
	}
	if !slicesContains(enemies, "Bob") {
		t.Fatalf("goblin enemies = %+v, want hidden Bob credited as participant", enemies)
	}
}

func TestNahanHandlerFailureDrainsActorMP(t *testing.T) {
	withAttackRolls(t, 22)
	world := state.NewWorld(formationSkillsWorld(t))
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewNahanHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "나한진이 실패했습니다") {
		t.Fatalf("status/output = %d/%q, want nahan failure", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got := creatureStat(alice, "mpCurrent"); got != 0 {
		t.Fatalf("alice mpCurrent = %d, want drained", got)
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 80; got != want {
		t.Fatalf("goblin hp = %d, want unchanged %d", got, want)
	}
	if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "was_attacked") {
		t.Fatalf("goblin tags = %+v, want was_attacked after failed nahan", goblin.Metadata.Tags)
	}
	enemies, err := world.CreatureEnemies("creature:goblin")
	if err != nil {
		t.Fatalf("CreatureEnemies() error = %v", err)
	}
	if !slicesContains(enemies, "Alice") || !slicesContains(enemies, "Bob") {
		t.Fatalf("goblin enemies = %+v, want Alice and Bob from failed nahan", enemies)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", nahanCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active nahan cooldown", used, remaining)
	}
}

func TestNahanHandlerFinalizesMonsterDeath(t *testing.T) {
	withAttackRolls(t, 1, 5, 6)
	loaded := formationSkillsWorld(t)
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Stats["hpCurrent"] = 20
	goblin.Stats["hpMax"] = 20
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewNahanHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "나한진으로 고블린을 죽였습니다") {
		t.Fatalf("status/output = %d/%q, want nahan death", status, ctx.OutputString())
	}
	if _, ok := world.Creature("creature:goblin"); ok {
		t.Fatal("dead goblin still exists, want finalized monster death")
	}
}

func TestNahanHandlerRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		mutate func(*worldload.World)
		want   string
	}{
		{name: "missing target", want: "누구를 공격합니까?"},
		{
			name: "wrong class",
			args: []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["class"] = model.ClassCleric
				loaded.Creatures[alice.ID] = alice
			},
			want: "무적이상만 쓸수 있는 기술입니다.",
		},
		{
			name: "missing training",
			args: []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"SFIGHTER"}
				loaded.Creatures[alice.ID] = alice
			},
			want: "불제자를 무적수련하지 않았습니다.",
		},
		{
			name: "unknown target",
			args: []string{"없는"},
			want: "그런 것은 존재하지 않습니다.",
		},
		{
			name: "protected target",
			args: []string{"불사"},
			want: "해칠 수 없습니다.",
		},
		{
			name: "alone",
			args: []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				delete(loaded.Players, "player:bob")
				delete(loaded.Creatures, "creature:bob")
				room := loaded.Rooms["room:dojo"]
				room.PlayerIDs = []model.PlayerID{"player:alice"}
				loaded.Rooms[room.ID] = room
			},
			want: "당신 혼자서는 나한진을 펼칠 수 없습니다.",
		},
		{
			name: "low formation mp",
			args: []string{"고블린"},
			mutate: func(loaded *worldload.World) {
				goblin := loaded.Creatures["creature:goblin"]
				goblin.Stats["mpCurrent"] = 2000
				loaded.Creatures[goblin.ID] = goblin
			},
			want: "동료들의 도력이 부족합니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := formationSkillsWorld(t)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
	defer world.Close()
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewNahanHandler(world)(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestNahanHandlerCooldownPrecedesRevealAndFormationChecks(t *testing.T) {
	loaded := formationSkillsWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS")
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	delete(loaded.Players, "player:bob")
	delete(loaded.Creatures, "creature:bob")
	room := loaded.Rooms["room:dojo"]
	room.PlayerIDs = []model.PlayerID{"player:alice"}
	loaded.Rooms[room.ID] = room
	world := state.NewWorld(loaded)
	defer world.Close()
	if err := world.SetCreatureCooldown("creature:alice", nahanCooldownKey, time.Now().Unix(), nahanCooldownSeconds(alice)); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewNahanHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "기다리세요.") ||
		strings.Contains(out, "서서히 드러납니다") ||
		strings.Contains(out, "혼자서는") {
		t.Fatalf("status/output = %d/%q, want cooldown before reveal/formation checks", status, out)
	}
	updated, _ := world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "PHIDDN") ||
		!hasAnyNormalizedFlag(updated.Metadata.Tags, "invisible", "PINVIS") ||
		updated.Stats["PHIDDN"] != 1 || updated.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained during cooldown", updated.Metadata.Tags, updated.Stats)
	}
	goblin, _ := world.Creature("creature:goblin")
	if hasAnyNormalizedFlag(goblin.Metadata.Tags, "was_attacked") {
		t.Fatalf("goblin tags = %+v, want no target prime during cooldown", goblin.Metadata.Tags)
	}
}

func TestNahanHandlerAloneRevealsPrimesTargetWithoutCooldown(t *testing.T) {
	loaded := formationSkillsWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS")
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	delete(loaded.Players, "player:bob")
	delete(loaded.Creatures, "creature:bob")
	room := loaded.Rooms["room:dojo"]
	room.PlayerIDs = []model.PlayerID{"player:alice"}
	loaded.Rooms[room.ID] = room
	world := state.NewWorld(loaded)
	defer world.Close()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewNahanHandler(world)(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "서서히 드러납니다") || !strings.Contains(out, "혼자서는") {
		t.Fatalf("status/output = %d/%q, want reveal and alone rejection", status, out)
	}
	updated, _ := world.Creature("creature:alice")
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS") ||
		updated.Stats["PHIDDN"] != 0 || updated.Stats["PINVIS"] != 0 {
		t.Fatalf("alice tags/stats = %+v/%+v, want revealed before formation rejection", updated.Metadata.Tags, updated.Stats)
	}
	goblin, _ := world.Creature("creature:goblin")
	if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "was_attacked") {
		t.Fatalf("goblin tags = %+v, want was_attacked before formation rejection", goblin.Metadata.Tags)
	}
	enemies, err := world.CreatureEnemies("creature:goblin")
	if err != nil {
		t.Fatalf("CreatureEnemies() error = %v", err)
	}
	if len(enemies) != 1 || enemies[0] != "Alice" {
		t.Fatalf("goblin enemies = %+v, want Alice before formation rejection", enemies)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", nahanCooldownKey, time.Now().Unix(), 0); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want no nahan cooldown before actual attempt", used, remaining)
	}
}

func TestFormationSkillsCanBeRegisteredByLegacyNames(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{line: "영자팔법", want: "영자팔법으로 고블린에게 29"},
		{line: "영", want: "영자팔법으로 고블린에게 29"},
		{line: "eight", want: "영자팔법으로 고블린에게 29"},
		{line: "고블린 나한진", want: "나한진을 펼쳐 고블린에게 52"},
		{line: "고블린 nahan", want: "나한진을 펼쳐 고블린에게 52"},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			if strings.Contains(tt.line, "나한") || strings.Contains(tt.line, "nahan") {
				withAttackRolls(t, 1, 5, 6)
			} else {
				withAttackRolls(t, 1)
			}
			world := state.NewWorld(formationSkillsWorld(t))
	defer world.Close()
			dispatcher := formationSkillsDispatcher(t, world)
			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, tt.line)
			if err != nil {
				t.Fatalf("DispatchLine(%q) error = %v", tt.line, err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func formationSkillsDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "영자팔법", Number: 159, Handler: "eight"},
		{Name: "영", Number: 159, Handler: "eight"},
		{Name: "eight", Number: 159, Handler: "eight"},
		{Name: "나한진", Number: 160, Handler: "nahan"},
		{Name: "nahan", Number: 160, Handler: "nahan"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"eight": NewEightHandler(world),
			"nahan": NewNahanHandler(world),
		},
	}
}

func formationSkillsWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:dojo",
		DisplayName: "도장",
		PlayerIDs:   []model.PlayerID{"player:alice", "player:bob"},
		CreatureIDs: []model.CreatureID{
			"creature:alice",
			"creature:bob",
			"creature:goblin",
			"creature:orc",
			"creature:hidden",
			"creature:protected",
		},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:dojo",
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:dojo",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:dojo",
		Equipment:   map[string]model.ObjectInstanceID{"wield": "object:sword"},
		Stats: map[string]int{
			"class":        model.ClassInvincible,
			"level":        50,
			"thaco":        0,
			"dexterity":    24,
			"intelligence": 30,
			"piety":        30,
			"hpCurrent":    100,
			"hpMax":        100,
			"mpCurrent":    100,
			"mpMax":        100,
			"pDice":        3,
		},
		Metadata: model.Metadata{Tags: []string{"SFIGHTER", "SCLERIC"}},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:dojo",
		Stats: map[string]int{
			"class":     model.ClassFighter,
			"level":     10,
			"hpCurrent": 50,
			"hpMax":     50,
			"mpCurrent": 50,
			"mpMax":     50,
		},
	})
	for _, creature := range []model.Creature{
		{
			ID:          "creature:goblin",
			Kind:        model.CreatureKindMonster,
			DisplayName: "고블린",
			RoomID:      "room:dojo",
			Stats:       map[string]int{"level": 10, "thaco": 20, "hpCurrent": 80, "hpMax": 80, "mpCurrent": 0, "experience": 100},
		},
		{
			ID:          "creature:orc",
			Kind:        model.CreatureKindMonster,
			DisplayName: "오크",
			RoomID:      "room:dojo",
			Stats:       map[string]int{"level": 10, "thaco": 20, "hpCurrent": 80, "hpMax": 80, "mpCurrent": 0, "experience": 100},
		},
		{
			ID:          "creature:hidden",
			Kind:        model.CreatureKindMonster,
			DisplayName: "숨은 적",
			RoomID:      "room:dojo",
			Stats:       map[string]int{"level": 10, "thaco": 20, "hpCurrent": 80, "hpMax": 80, "mpCurrent": 0},
			Metadata:    model.Metadata{Tags: []string{"hidden"}},
		},
		{
			ID:          "creature:protected",
			Kind:        model.CreatureKindMonster,
			DisplayName: "불사 적",
			RoomID:      "room:dojo",
			Stats:       map[string]int{"level": 10, "thaco": 20, "hpCurrent": 80, "hpMax": 80, "mpCurrent": 0},
			Metadata:    model.Metadata{Tags: []string{"MUNKIL"}},
		},
	} {
		mustAddLookCreature(t, loaded, creature)
	}
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:sword",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "검",
		Properties:  map[string]string{"type": "1", "pDice": "5"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:sword",
		PrototypeID: "prototype:sword",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "wield"},
		Properties:  map[string]string{"type": "1", "pDice": "5"},
	})
	return loaded
}
