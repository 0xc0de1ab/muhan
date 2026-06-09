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

func TestPobackHandlerDamagesAdjacentTargetSetsStatusAndCooldown(t *testing.T) {
	withAttackRolls(t, 1, 1)
	loaded := pobackWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS")
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	handler := NewPobackHandler(world)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{Args: []string{"동", "고블린"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "53점의 피해") {
		t.Fatalf("status/output = %d/%q, want poback damage", status, ctx.OutputString())
	}

	goblin, _ := world.Creature("creature:east-goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 27; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "charmed", "MCHARM") ||
		!hasAnyNormalizedFlag(goblin.Metadata.Tags, "befuddled", "MBEFUD") {
		t.Fatalf("goblin tags = %+v, want poback control statuses", goblin.Metadata.Tags)
	}
	now := time.Now().Unix()
	for _, key := range []string{"charmed", "befuddled", "attack", "spell"} {
		if remaining, ready, err := world.UseCreatureCooldown(goblin.ID, key, now, 0); err != nil {
			t.Fatalf("UseCreatureCooldown(%q) error = %v", key, err)
		} else if ready || remaining <= 0 {
			t.Fatalf("%s cooldown ready/remaining = %v/%d, want active C poback duration", key, ready, remaining)
		}
	}
	if len(broadcasts) < 2 || !strings.Contains(broadcasts[0].Text, "정신을 집중합니다") ||
		!strings.Contains(broadcasts[1].Text, "53점의 피해") {
		t.Fatalf("broadcasts = %+v, want concentration and target-room damage", broadcasts)
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN") ||
		!hasAnyNormalizedFlag(alice.Metadata.Tags, "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 1 || alice.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want poback to retain hidden/invisible like C", alice.Metadata.Tags, alice.Stats)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", pobackCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active poback cooldown", used, remaining)
	}
}

func TestPobackHandlerMagicResistantTargetReducesBefuddleStatus(t *testing.T) {
	withAttackRolls(t, 1, 1)
	loaded := pobackWorld(t)
	goblin := loaded.Creatures["creature:east-goblin"]
	goblin.Metadata.Tags = []string{"MNOCHA"}
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewPobackHandler(world)(ctx, ResolvedCommand{Args: []string{"동", "고블린"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "53점의 피해") {
		t.Fatalf("status/output = %d/%q, want poback damage", status, ctx.OutputString())
	}
	goblin, _ = world.Creature("creature:east-goblin")
	if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "charmed", "MCHARM") {
		t.Fatalf("goblin tags = %+v, want charm from C random branch", goblin.Metadata.Tags)
	}
	if hasAnyNormalizedFlag(goblin.Metadata.Tags, "befuddled", "MBEFUD") {
		t.Fatalf("goblin tags = %+v, want no befuddle after C magic-resist duration reduction", goblin.Metadata.Tags)
	}
}

func TestPobackHandlerFailurePrimesAndMarksTarget(t *testing.T) {
	withAttackRolls(t, 22)
	world := state.NewWorld(pobackWorld(t))
	handler := NewPobackHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"동", "고블린"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "포박술이 실패했습니다") {
		t.Fatalf("status/output = %d/%q, want poback failure", status, ctx.OutputString())
	}
	goblin, _ := world.Creature("creature:east-goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 80; got != want {
		t.Fatalf("goblin hp = %d, want unchanged %d", got, want)
	}
	if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "was_attacked") {
		t.Fatalf("goblin tags = %+v, want was_attacked after failed poback", goblin.Metadata.Tags)
	}
	if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "charmed", "MCHARM") ||
		!hasAnyNormalizedFlag(goblin.Metadata.Tags, "befuddled", "MBEFUD") {
		t.Fatalf("goblin tags = %+v, want C failure status flags on target", goblin.Metadata.Tags)
	}
	enemies, err := world.CreatureEnemies("creature:east-goblin")
	if err != nil {
		t.Fatalf("CreatureEnemies() error = %v", err)
	}
	if len(enemies) != 1 || enemies[0] != "Alice" {
		t.Fatalf("goblin enemies = %+v, want Alice from failed poback", enemies)
	}
	alice, _ := world.Creature("creature:alice")
	if hasAnyNormalizedFlag(alice.Metadata.Tags, "befuddled", "MBEFUD") {
		t.Fatalf("alice tags = %+v, want no actor befuddle flag from C failure path", alice.Metadata.Tags)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", pobackCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active poback cooldown", used, remaining)
	}
}

func TestPobackHandlerFinalizesAdjacentMonsterDeath(t *testing.T) {
	withAttackRolls(t, 1, 1)
	loaded := pobackWorld(t)
	goblin := loaded.Creatures["creature:east-goblin"]
	goblin.Stats["hpCurrent"] = 10
	goblin.Stats["hpMax"] = 10
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := NewPobackHandler(world)(ctx, ResolvedCommand{Args: []string{"동", "고블린"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "죽였습니다") {
		t.Fatalf("status/output = %d/%q, want death message", status, ctx.OutputString())
	}
	if _, ok := world.Creature("creature:east-goblin"); ok {
		t.Fatal("dead poback target still exists in world")
	}
	east, _ := world.Room("room:east")
	if containsCreatureID(east.CreatureIDs, "creature:east-goblin") {
		t.Fatalf("east creatures = %+v, want goblin removed", east.CreatureIDs)
	}
	foundCurrentRoomDeath := false
	for _, broadcast := range broadcasts {
		if broadcast.RoomID == "room:start" && strings.Contains(broadcast.Text, "동쪽에 있는 고블린") {
			foundCurrentRoomDeath = true
		}
	}
	if !foundCurrentRoomDeath {
		t.Fatalf("broadcasts = %+v, want current-room adjacent death broadcast", broadcasts)
	}
}

func TestPobackHandlerUsesCustomDeathFinalizer(t *testing.T) {
	withAttackRolls(t, 1, 1)
	loaded := pobackWorld(t)
	goblin := loaded.Creatures["creature:east-goblin"]
	goblin.Stats["hpCurrent"] = 10
	goblin.Stats["hpMax"] = 10
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)

	called := false
	finalizer := func(ctx *Context, attacker model.Creature, victim model.Creature) error {
		called = true
		if attacker.ID != "creature:alice" || victim.ID != "creature:east-goblin" {
			t.Fatalf("finalizer attacker/victim = %q/%q, want alice/east-goblin", attacker.ID, victim.ID)
		}
		return nil
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewPobackHandlerWithDeathFinalizer(world, finalizer)(ctx, ResolvedCommand{Args: []string{"동", "고블린"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !called || !strings.Contains(ctx.OutputString(), "죽였습니다") {
		t.Fatalf("status/called/output = %d/%v/%q, want custom finalizer death", status, called, ctx.OutputString())
	}
}

func TestPobackHandlerRejectsInvalidStates(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		mutate func(*worldload.World)
		setup  func(*state.World)
		want   string
	}{
		{name: "missing args", args: []string{"동"}, want: "사용법"},
		{
			name: "wrong class",
			args: []string{"동", "고블린"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["class"] = legacyClassRanger
				loaded.Creatures[alice.ID] = alice
			},
			want: "무적이상만 쓸수 있는 기술입니다.",
		},
		{
			name: "invincible without ranger training",
			args: []string{"동", "고블린"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = nil
				loaded.Creatures[alice.ID] = alice
			},
			want: "포졸을 무적수련하지 않았습니다.",
		},
		{
			name: "blind",
			args: []string{"동", "고블린"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = append(alice.Metadata.Tags, "PBLIND")
				loaded.Creatures[alice.ID] = alice
			},
			want: "눈이 멀어 있습니다",
		},
		{
			name: "wrong weapon",
			args: []string{"동", "고블린"},
			mutate: func(loaded *worldload.World) {
				proto := loaded.ObjectPrototypes["prototype:staff"]
				proto.Properties["type"] = "1"
				loaded.ObjectPrototypes[proto.ID] = proto
			},
			want: "봉이나 창종류의 무기가 필요합니다.",
		},
		{
			name: "closed exit",
			args: []string{"동", "고블린"},
			mutate: func(loaded *worldload.World) {
				room := loaded.Rooms["room:start"]
				room.Exits[0].Flags = []string{"closed"}
				loaded.Rooms[room.ID] = room
			},
			want: "그 출구는 닫혀 있습니다.",
		},
		{
			name: "protected destination",
			args: []string{"동", "고블린"},
			mutate: func(loaded *worldload.World) {
				room := loaded.Rooms["room:east"]
				room.Metadata.Tags = []string{"onlyMarried"}
				loaded.Rooms[room.ID] = room
			},
			want: "그 방은 볼 수가 없습니다.",
		},
		{name: "unknown target", args: []string{"동", "없는"}, want: "그런 것은 존재하지 않습니다."},
		{
			name: "cooldown active",
			args: []string{"동", "고블린"},
			setup: func(world *state.World) {
				if err := world.SetCreatureCooldown("creature:alice", pobackCooldownKey, time.Now().Unix(), pobackCooldownSeconds(model.Creature{Stats: map[string]int{"dexterity": 24}})); err != nil {
					t.Fatalf("SetCreatureCooldown() error = %v", err)
				}
			},
			want: "기다리세요.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := pobackWorld(t)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
			if tt.setup != nil {
				tt.setup(world)
			}
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewPobackHandler(world)(ctx, ResolvedCommand{Args: tt.args, Values: []int64{1, 1}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestPobackHandlerCooldownPrecedesExitAndTargetLookup(t *testing.T) {
	loaded := pobackWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS")
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	if err := world.SetCreatureCooldown("creature:alice", pobackCooldownKey, time.Now().Unix(), pobackCooldownSeconds(alice)); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewPobackHandler(world)(ctx, ResolvedCommand{Args: []string{"없는길", "없는몹"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "기다리세요.") || strings.Contains(out, "지도가 없습니다") {
		t.Fatalf("status/output = %d/%q, want cooldown before exit lookup", status, out)
	}
	updated, _ := world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "PHIDDN") ||
		!hasAnyNormalizedFlag(updated.Metadata.Tags, "invisible", "PINVIS") ||
		updated.Stats["PHIDDN"] != 1 || updated.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained during cooldown", updated.Metadata.Tags, updated.Stats)
	}
}

func TestPobackHandlerWeaponBreakOnSuccessDoesNotStartCooldown(t *testing.T) {
	withAttackRolls(t, 1)
	loaded := pobackWorld(t)
	staff := loaded.Objects["object:staff"]
	if staff.Properties == nil {
		staff.Properties = map[string]string{}
	}
	staff.Properties["shotsCurrent"] = "1"
	loaded.Objects[staff.ID] = staff
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewPobackHandler(world)(ctx, ResolvedCommand{Args: []string{"동", "고블린"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "부서져 버렸습니다.") {
		t.Fatalf("status/output = %d/%q, want weapon break", status, out)
	}
	goblin, _ := world.Creature("creature:east-goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 80; got != want {
		t.Fatalf("goblin hp = %d, want unchanged after weapon break %d", got, want)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", pobackCooldownKey, time.Now().Unix(), 0); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want no cooldown after break-return", used, remaining)
	}
}

func TestPobackHandlerCanBeRegisteredByDispatcherAliases(t *testing.T) {
	for _, line := range []string{"동 고블린 포박술", "동 고블린 포박", "poback 동 고블린"} {
		t.Run(line, func(t *testing.T) {
			withAttackRolls(t, 1, 1)
			world := state.NewWorld(pobackWorld(t))
			dispatcher := controlSkillDispatcher(t, world)

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, line)
			if err != nil {
				t.Fatalf("DispatchLine(%q) error = %v", line, err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), "포박술로 고블린에게") {
				t.Fatalf("status/output = %d/%q, want poback dispatch success", status, ctx.OutputString())
			}
		})
	}
}

func TestLionScreamHandlerDamagesRoomTargetsAndBroadcastsAdjacentRooms(t *testing.T) {
	withAttackRolls(t, 1, 5, 6)
	loaded := lionScreamWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS")
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	handler := NewLionScreamHandler(world)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "고블린에게 35점의 피해") ||
		!strings.Contains(ctx.OutputString(), "오크에게 36점의 피해") {
		t.Fatalf("status/output = %d/%q, want lion scream damage", status, ctx.OutputString())
	}

	goblin, _ := world.Creature("creature:goblin")
	orc, _ := world.Creature("creature:orc")
	hidden, _ := world.Creature("creature:hidden")
	protected, _ := world.Creature("creature:protected")
	if got, want := creatureStat(goblin, "hpCurrent"), 5; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	if got, want := creatureStat(orc, "hpCurrent"), 4; got != want {
		t.Fatalf("orc hp = %d, want %d", got, want)
	}
	if got, want := creatureStat(hidden, "hpCurrent"), 40; got != want {
		t.Fatalf("hidden hp = %d, want untouched %d", got, want)
	}
	if got, want := creatureStat(protected, "hpCurrent"), 40; got != want {
		t.Fatalf("protected hp = %d, want untouched %d", got, want)
	}
	foundAdjacent := false
	for _, broadcast := range broadcasts {
		if broadcast.RoomID == "room:echo" && strings.Contains(broadcast.Text, "여기까지 울려퍼집니다") {
			foundAdjacent = true
		}
	}
	if !foundAdjacent {
		t.Fatalf("broadcasts = %+v, want adjacent lion scream broadcast", broadcasts)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", lionScreamCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active lion_scream cooldown", used, remaining)
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

func TestLionScreamHandlerFailureFatiguesActor(t *testing.T) {
	withAttackRolls(t, 22)
	world := state.NewWorld(lionScreamWorld(t))
	handler := NewLionScreamHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "공력을 이기지 못합니다") ||
		!strings.Contains(ctx.OutputString(), "피로해짐") {
		t.Fatalf("status/output = %d/%q, want lion scream failure", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got, want := creatureStat(alice, "hpCurrent"), 90; got != want {
		t.Fatalf("alice hp = %d, want fatigue hp %d", got, want)
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 40; got != want {
		t.Fatalf("goblin hp = %d, want unchanged %d", got, want)
	}
	if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "was_attacked") {
		t.Fatalf("goblin tags = %+v, want was_attacked after failed lion_scream", goblin.Metadata.Tags)
	}
	enemies, err := world.CreatureEnemies("creature:goblin")
	if err != nil {
		t.Fatalf("CreatureEnemies() error = %v", err)
	}
	if len(enemies) != 1 || enemies[0] != "Alice" {
		t.Fatalf("goblin enemies = %+v, want Alice from failed lion_scream", enemies)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", lionScreamCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active lion_scream cooldown", used, remaining)
	}
}

func TestLionScreamHandlerFinalizesMonsterDeath(t *testing.T) {
	withAttackRolls(t, 1, 5, 6)
	loaded := lionScreamWorld(t)
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Stats["hpCurrent"] = 10
	goblin.Stats["hpMax"] = 10
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewLionScreamHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "뛰어난 공력으로 고블린") {
		t.Fatalf("status/output = %d/%q, want lion scream death", status, ctx.OutputString())
	}
	if _, ok := world.Creature("creature:goblin"); ok {
		t.Fatal("dead lion scream target still exists in world")
	}
	start, _ := world.Room("room:start")
	if containsCreatureID(start.CreatureIDs, "creature:goblin") {
		t.Fatalf("start creatures = %+v, want goblin removed", start.CreatureIDs)
	}
}

func TestLionScreamHandlerUsesCustomDeathFinalizer(t *testing.T) {
	withAttackRolls(t, 1, 5, 6)
	loaded := lionScreamWorld(t)
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Stats["hpCurrent"] = 10
	goblin.Stats["hpMax"] = 10
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)

	called := false
	finalizer := func(ctx *Context, attacker model.Creature, victim model.Creature) error {
		called = true
		if attacker.ID != "creature:alice" || victim.ID != "creature:goblin" {
			t.Fatalf("finalizer attacker/victim = %q/%q, want alice/goblin", attacker.ID, victim.ID)
		}
		return nil
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewLionScreamHandlerWithDeathFinalizer(world, finalizer)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !called || !strings.Contains(ctx.OutputString(), "뛰어난 공력으로 고블린") {
		t.Fatalf("status/called/output = %d/%v/%q, want custom finalizer death", status, called, ctx.OutputString())
	}
}

func TestLionScreamHandlerRejectsInvalidStates(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*worldload.World)
		want   string
	}{
		{
			name: "wrong class",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["class"] = legacyClassFighter
				alice.Stats["level"] = 60
				alice.Level = 60
				loaded.Creatures[alice.ID] = alice
			},
			want: "무사 레벨 50이상만 쓸수 있는 기술입니다.",
		},
		{
			name: "paladin below level",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["level"] = 49
				alice.Level = 49
				loaded.Creatures[alice.ID] = alice
			},
			want: "무사 레벨 50이상만 쓸수 있는 기술입니다.",
		},
		{
			name: "invincible without paladin training",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["class"] = legacyClassInvincible
				alice.Metadata.Tags = nil
				loaded.Creatures[alice.ID] = alice
			},
			want: "무사를 무적수련하지 않았습니다.",
		},
		{
			name: "no targets",
			mutate: func(loaded *worldload.World) {
				for id, creature := range loaded.Creatures {
					if id == "creature:alice" || creature.RoomID != "room:start" {
						continue
					}
					creature.Metadata.Tags = []string{"MUNKIL"}
					loaded.Creatures[id] = creature
				}
			},
			want: "이 방에는 당신이 공격할 적이 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := lionScreamWorld(t)
			tt.mutate(loaded)
			world := state.NewWorld(loaded)
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewLionScreamHandler(world)(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestLionScreamHandlerCooldownPrecedesTargetScanAndReveal(t *testing.T) {
	loaded := lionScreamWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS")
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	for id, creature := range loaded.Creatures {
		if id == "creature:alice" || creature.RoomID != "room:start" {
			continue
		}
		creature.Metadata.Tags = []string{"MUNKIL"}
		loaded.Creatures[id] = creature
	}
	world := state.NewWorld(loaded)
	if err := world.SetCreatureCooldown("creature:alice", lionScreamCooldownKey, time.Now().Unix(), lionScreamCooldownSeconds(alice)); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewLionScreamHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "기다리세요.") ||
		strings.Contains(out, "공격할 적이 없습니다") ||
		strings.Contains(out, "서서히 드러납니다") {
		t.Fatalf("status/output = %d/%q, want cooldown before target/reveal", status, out)
	}
	updated, _ := world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "PHIDDN") ||
		!hasAnyNormalizedFlag(updated.Metadata.Tags, "invisible", "PINVIS") ||
		updated.Stats["PHIDDN"] != 1 || updated.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained during cooldown", updated.Metadata.Tags, updated.Stats)
	}
}

func TestLionScreamHandlerNoTargetsDoesNotStartCooldown(t *testing.T) {
	loaded := lionScreamWorld(t)
	for id, creature := range loaded.Creatures {
		if id == "creature:alice" || creature.RoomID != "room:start" {
			continue
		}
		creature.Metadata.Tags = []string{"MUNKIL"}
		loaded.Creatures[id] = creature
	}
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewLionScreamHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "공격할 적이 없습니다") {
		t.Fatalf("status/output = %d/%q, want no targets", status, ctx.OutputString())
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", lionScreamCooldownKey, time.Now().Unix(), 0); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want no cooldown on no-target rejection", used, remaining)
	}
}

func TestLionScreamHandlerCanBeRegisteredByDispatcherAliases(t *testing.T) {
	for _, line := range []string{"사자후", "lion_scream"} {
		t.Run(line, func(t *testing.T) {
			withAttackRolls(t, 1, 5, 6)
			world := state.NewWorld(lionScreamWorld(t))
			dispatcher := controlSkillDispatcher(t, world)

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, line)
			if err != nil {
				t.Fatalf("DispatchLine(%q) error = %v", line, err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), "사자후를 내질러 고블린") {
				t.Fatalf("status/output = %d/%q, want lion scream dispatch success", status, ctx.OutputString())
			}
		})
	}
}

func controlSkillDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()
	return Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "포박술", Number: 163, Handler: "poback"},
			{Name: "포박", Number: 163, Handler: "poback"},
			{Name: "poback", Number: 163, Handler: "poback"},
			{Name: "사자후", Number: 166, Handler: "lion_scream"},
			{Name: "lion_scream", Number: 166, Handler: "lion_scream"},
		}),
		Handlers: map[string]Handler{
			"poback":      NewPobackHandler(world),
			"lion_scream": NewLionScreamHandler(world),
		},
	}
}

func pobackWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := baseControlSkillWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["class"] = legacyClassInvincible
	loaded.Creatures[alice.ID] = alice
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:east",
		DisplayName: "동쪽 방",
		CreatureIDs: []model.CreatureID{
			"creature:east-goblin",
		},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:east-goblin",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:east",
		Stats: map[string]int{
			"level":     10,
			"thaco":     0,
			"hpCurrent": 80,
			"hpMax":     80,
		},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:staff",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "목봉",
		Properties:  map[string]string{"type": "2", "pDice": "5"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:staff",
		PrototypeID: "prototype:staff",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "wield"},
	})
	return loaded
}

func lionScreamWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := baseControlSkillWorld(t)
	mustAddLookRoom(t, loaded, model.Room{ID: "room:east", DisplayName: "동쪽 방"})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:goblin",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:start",
		Stats:       map[string]int{"level": 10, "thaco": 20, "hpCurrent": 40, "hpMax": 40},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:orc",
		Kind:        model.CreatureKindMonster,
		DisplayName: "오크",
		RoomID:      "room:start",
		Stats:       map[string]int{"level": 10, "thaco": 20, "hpCurrent": 40, "hpMax": 40},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:hidden",
		Kind:        model.CreatureKindMonster,
		DisplayName: "숨은 적",
		RoomID:      "room:start",
		Stats:       map[string]int{"level": 10, "thaco": 20, "hpCurrent": 40, "hpMax": 40},
		Metadata:    model.Metadata{Tags: []string{"hidden"}},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:protected",
		Kind:        model.CreatureKindMonster,
		DisplayName: "불사 적",
		RoomID:      "room:start",
		Stats:       map[string]int{"level": 10, "thaco": 20, "hpCurrent": 40, "hpMax": 40},
		Metadata:    model.Metadata{Tags: []string{"MUNKIL"}},
	})
	start := loaded.Rooms["room:start"]
	start.CreatureIDs = []model.CreatureID{
		"creature:goblin",
		"creature:orc",
		"creature:hidden",
		"creature:protected",
	}
	loaded.Rooms[start.ID] = start
	return loaded
}

func baseControlSkillWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:start",
		DisplayName: "시작 방",
		PlayerIDs:   []model.PlayerID{"player:alice"},
		Exits: []model.Exit{
			{Name: "동", ToRoomID: "room:east"},
			{Name: "메아리", ToRoomID: "room:echo"},
		},
	})
	mustAddLookRoom(t, loaded, model.Room{ID: "room:echo", DisplayName: "메아리 방"})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:start",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:start",
		Level:       50,
		Equipment:   map[string]model.ObjectInstanceID{"wield": "object:staff"},
		Stats: map[string]int{
			"class":        legacyClassPaladin,
			"level":        50,
			"thaco":        0,
			"dexterity":    24,
			"intelligence": 24,
			"piety":        30,
			"hpCurrent":    100,
			"hpMax":        100,
		},
		Metadata: model.Metadata{Tags: []string{"SRANGER", "SPALADIN"}},
	})
	return loaded
}
