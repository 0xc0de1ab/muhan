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

func TestRedEyeHandlerDamagesRemoteMonster(t *testing.T) {
	loaded := perceptionWorld(t, model.ClassInvincible, []string{"SPALADIN", "STHIEF", "hidden", "PHIDDN", "invisible", "PINVIS"})
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	dispatcher := perceptionDispatcher(t, world, fixedPerceptionRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "혈마안 player:bob 고블린")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if out := ctx.OutputString(); !strings.Contains(out, "혈마안이 고블린에게") || !strings.Contains(out, "피해") {
		t.Fatalf("red_eye output = %q, want damage line", out)
	}

	goblin, ok := world.Creature("creature:goblin")
	if !ok {
		t.Fatal("goblin was unexpectedly finalized")
	}
	if got, wantLessThan := creatureStat(goblin, "hpCurrent"), 300; got >= wantLessThan {
		t.Fatalf("goblin hpCurrent = %d, want less than %d", got, wantLessThan)
	}
	enemies, err := world.CreatureEnemies("creature:goblin")
	if err != nil {
		t.Fatalf("CreatureEnemies() error = %v", err)
	}
	if !slicesContains(enemies, "Alice") || !slicesContains(enemies, "Bob") {
		t.Fatalf("goblin enemies = %+v, want Alice and Bob from red_eye", enemies)
	}
	alice, _ = world.Creature("creature:alice")
	if hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 0 || alice.Stats["PINVIS"] != 0 {
		t.Fatalf("alice tags/stats = %+v/%+v, want red_eye reveal cleared hidden/invisible", alice.Metadata.Tags, alice.Stats)
	}
	player, _ = world.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS") {
		t.Fatalf("player tags = %+v, want red_eye reveal cleared hidden/invisible", player.Metadata.Tags)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", redEyeCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active red_eye cooldown", used, remaining)
	}
}

func TestRedEyeHandlerUsesCustomDeathFinalizer(t *testing.T) {
	loaded := perceptionWorld(t, model.ClassInvincible, []string{"SPALADIN", "STHIEF"})
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Stats["hpCurrent"] = 1
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	called := false
	handler := NewRedEyeHandlerWithDeathFinalizer(world, fixedPerceptionRoll(1), func(_ *Context, attacker model.Creature, victim model.Creature) error {
		called = true
		if attacker.ID != "creature:alice" || victim.ID != "creature:goblin" {
			t.Fatalf("finalizer attacker/victim = %q/%q, want alice/goblin", attacker.ID, victim.ID)
		}
		_, err := world.FinalizeMonsterDeath(victim.ID)
		return err
	})

	ctx := &Context{ActorID: "player:alice"}
	if _, err := handler(ctx, ResolvedCommand{Args: []string{"player:bob", "고블린"}}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if !called {
		t.Fatal("custom finalizer was not called")
	}
	if _, ok := world.Creature("creature:goblin"); ok {
		t.Fatal("dead red_eye target still exists in world")
	}
}

func TestRedEyeHandlerRejectsInvalidUse(t *testing.T) {
	tests := []struct {
		name   string
		class  int
		tags   []string
		args   []string
		mutate func(*worldload.World)
		want   string
	}{
		{name: "wrong class", class: model.ClassFighter, tags: []string{"SPALADIN"}, args: []string{"player:bob", "고블린"}, want: "무적 이상만 사용할 수 있는 기술입니다."},
		{name: "missing training", class: model.ClassInvincible, args: []string{"player:bob", "고블린"}, want: "무사를 무적수련하지 않았습니다.."},
		{name: "missing args", class: model.ClassInvincible, tags: []string{"SPALADIN"}, want: "사용법: 혈마안"},
		{name: "missing player", class: model.ClassInvincible, tags: []string{"SPALADIN"}, args: []string{"player:none", "고블린"}, want: "그런 사람은 존재하지 않습니다."},
		{
			name:  "same room target",
			class: model.ClassInvincible,
			tags:  []string{"SPALADIN"},
			args:  []string{"player:bob", "고블린"},
			mutate: func(loaded *worldload.World) {
				bob := loaded.Players["player:bob"]
				bob.RoomID = "room:start"
				loaded.Players[bob.ID] = bob
				bobCreature := loaded.Creatures["creature:bob"]
				bobCreature.RoomID = "room:start"
				loaded.Creatures[bobCreature.ID] = bobCreature
				goblin := loaded.Creatures["creature:goblin"]
				goblin.RoomID = "room:start"
				loaded.Creatures[goblin.ID] = goblin
				start := loaded.Rooms["room:start"]
				start.CreatureIDs = append(start.CreatureIDs, "creature:bob", "creature:goblin")
				loaded.Rooms[start.ID] = start
			},
			want: "같은 방에 있는 사람에게는 혈마안을 사용할 수 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := perceptionWorld(t, tt.class, tt.tags)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewRedEyeHandler(world, fixedPerceptionRoll(1))(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestRedEyeHandlerFailurePrimesEnemyAndStartsCooldown(t *testing.T) {
	loaded := perceptionWorld(t, model.ClassInvincible, []string{"SPALADIN", "STHIEF"})
	world := state.NewWorld(loaded)
	handler := NewRedEyeHandler(world, fixedPerceptionRoll(22))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"player:bob", "고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "혈마안이 빗나갔습니다") {
		t.Fatalf("status/output = %d/%q, want red_eye failure", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got, want := creatureStat(alice, "hpCurrent"), 50; got != want {
		t.Fatalf("alice hpCurrent = %d, want backlash %d", got, want)
	}
	if got, want := creatureStat(alice, "mpCurrent"), 25; got != want {
		t.Fatalf("alice mpCurrent = %d, want backlash %d", got, want)
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 300; got != want {
		t.Fatalf("goblin hpCurrent = %d, want unchanged %d", got, want)
	}
	enemies, err := world.CreatureEnemies("creature:goblin")
	if err != nil {
		t.Fatalf("CreatureEnemies() error = %v", err)
	}
	if !slicesContains(enemies, "Alice") || slicesContains(enemies, "Bob") {
		t.Fatalf("goblin enemies = %+v, want only Alice from failed red_eye", enemies)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", redEyeCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active red_eye cooldown", used, remaining)
	}
}

func TestRedEyeHandlerRejectsActorGroupBeforeRevealAndCooldown(t *testing.T) {
	loaded := perceptionWorld(t, model.ClassInvincible, []string{"SPALADIN", "hidden", "PHIDDN", "invisible", "PINVIS"})
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.groupMemory": &mockGroupMemory{
				followers: map[string][]string{"player:alice": {"player:bob"}},
			},
		},
	}

	status, err := NewRedEyeHandler(world, fixedPerceptionRoll(1))(ctx, ResolvedCommand{Args: []string{"player:bob", "고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그룹원들에게는 혈마를 할 수 없습니다.\n" {
		t.Fatalf("status/output = %d/%q, want actor group rejection", status, ctx.OutputString())
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 1 || alice.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want no reveal before group rejection", alice.Metadata.Tags, alice.Stats)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", redEyeCooldownKey, time.Now().Unix(), 0); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want no cooldown after actor group rejection", used, remaining)
	}
}

func TestRedEyeHandlerRejectsTargetGroupBeforeRevealAndCooldown(t *testing.T) {
	loaded := perceptionWorld(t, model.ClassInvincible, []string{"SPALADIN", "hidden", "PHIDDN", "invisible", "PINVIS"})
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.groupMemory": &mockGroupMemory{
				leaders: map[string]string{"player:bob": "player:charlie"},
			},
		},
	}

	status, err := NewRedEyeHandler(world, fixedPerceptionRoll(1))(ctx, ResolvedCommand{Args: []string{"player:bob", "고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "상대방이 그룹이 있네요. 혈마를 할 수 없어요!\n" {
		t.Fatalf("status/output = %d/%q, want target group rejection", status, ctx.OutputString())
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 1 || alice.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want no reveal before target group rejection", alice.Metadata.Tags, alice.Stats)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", redEyeCooldownKey, time.Now().Unix(), 0); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want no cooldown after target group rejection", used, remaining)
	}
}

func TestRedEyeHandlerCooldownPrecedesTargetLookupAndReveal(t *testing.T) {
	loaded := perceptionWorld(t, model.ClassInvincible, []string{"SPALADIN", "STHIEF", "hidden", "PHIDDN", "invisible", "PINVIS"})
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	if err := world.SetCreatureCooldown("creature:alice", redEyeCooldownKey, time.Now().Unix(), redEyeCooldownSeconds(loaded.Creatures["creature:alice"])); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewRedEyeHandler(world, fixedPerceptionRoll(1))(ctx, ResolvedCommand{Args: []string{"player:none", "없는"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "기다리세요.") || strings.Contains(out, "그런 사람") {
		t.Fatalf("status/output = %d/%q, want cooldown before target lookup", status, out)
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN") ||
		!hasAnyNormalizedFlag(alice.Metadata.Tags, "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 1 || alice.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained during cooldown", alice.Metadata.Tags, alice.Stats)
	}
}

func TestRedEyeHandlerInvalidEnemyRevealsWithoutStartingCooldown(t *testing.T) {
	loaded := perceptionWorld(t, model.ClassInvincible, []string{"SPALADIN", "STHIEF", "hidden", "PHIDDN", "invisible", "PINVIS"})
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewRedEyeHandler(world, fixedPerceptionRoll(1))(ctx, ResolvedCommand{Args: []string{"player:bob", "없는"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "그런 괴물은 없습니다") || !strings.Contains(out, "모습을 드러냅니다") {
		t.Fatalf("status/output = %d/%q, want reveal then missing enemy", status, out)
	}
	alice, _ = world.Creature("creature:alice")
	if hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 0 || alice.Stats["PINVIS"] != 0 {
		t.Fatalf("alice tags/stats = %+v/%+v, want revealed before missing enemy rejection", alice.Metadata.Tags, alice.Stats)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", redEyeCooldownKey, time.Now().Unix(), 0); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want no red_eye cooldown after missing enemy", used, remaining)
	}
}

func TestThiefStatHandlerRendersObjectDetails(t *testing.T) {
	loaded := perceptionWorld(t, model.ClassInvincible, []string{"SPALADIN", "STHIEF", "hidden", "PHIDDN", "invisible", "PINVIS"})
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	dispatcher := perceptionDispatcher(t, world, fixedPerceptionRoll(20))

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "천안술 목검 상인")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	out := ctx.OutputString()
	for _, want := range []string{
		"이름: 목검\n",
		"설명: 낡은 목검\n",
		"효과: 휘두르면 둔탁한 소리가 난다\n",
		"사용회수 2/5\n",
		"종류: 검 무기.\n",
		"타격치: 4면2굴림 더하기 1 (+1)\n",
		"가치: 00150   무게: 03\n",
		"특성: 선한 사람용, 빙의 되있음.\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("thief_stat object output missing %q:\n%s", want, out)
		}
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN") ||
		alice.Stats["PHIDDN"] != 1 ||
		hasAnyNormalizedFlag(alice.Metadata.Tags, "invisible", "PINVIS") ||
		alice.Stats["PINVIS"] != 0 {
		t.Fatalf("alice tags/stats = %+v/%+v, want thief_stat to clear only invisible", alice.Metadata.Tags, alice.Stats)
	}
	player, _ = world.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "PHIDDN") ||
		hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "PINVIS") {
		t.Fatalf("player tags = %+v, want thief_stat to clear only invisible", player.Metadata.Tags)
	}
}

func TestThiefStatHandlerRendersCreatureDetailsAndStatus(t *testing.T) {
	world := state.NewWorld(perceptionWorld(t, model.ClassInvincible, []string{"SPALADIN", "STHIEF"}))
	handler := NewThiefStatHandler(world, fixedPerceptionRoll(20))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"상인"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	out := ctx.OutputString()
	for _, want := range []string{
		"이름: 상인\n",
		"레벨: 8\n",
		"종족: 인간\n",
		"직업: 검사\n",
		"체력: 35/35\n",
		"상태: 은둔감지\n",
		"소지품: 목검\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("thief_stat creature output missing %q:\n%s", want, out)
		}
	}
}

func TestThiefStatHandlerCooldownPrecedesBlindAndReveal(t *testing.T) {
	loaded := perceptionWorld(t, model.ClassInvincible, []string{"STHIEF", "PBLIND", "hidden", "PHIDDN", "invisible", "PINVIS"})
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	if err := world.SetCreatureCooldown("creature:alice", thiefStatCooldownKey, time.Now().Unix(), thiefStatCooldownSeconds(loaded.Creatures["creature:alice"])); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewThiefStatHandler(world, fixedPerceptionRoll(20))(ctx, ResolvedCommand{Args: []string{"상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "기다리세요.") || strings.Contains(out, "눈이 멀어") {
		t.Fatalf("status/output = %d/%q, want cooldown before blind rejection", status, out)
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN") ||
		!hasAnyNormalizedFlag(alice.Metadata.Tags, "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 1 || alice.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained during cooldown", alice.Metadata.Tags, alice.Stats)
	}
}

func TestThiefStatHandlerBlindDoesNotStartCooldown(t *testing.T) {
	world := state.NewWorld(perceptionWorld(t, model.ClassInvincible, []string{"STHIEF", "PBLIND"}))

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewThiefStatHandler(world, fixedPerceptionRoll(20))(ctx, ResolvedCommand{Args: []string{"상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "눈이 멀어") {
		t.Fatalf("status/output = %d/%q, want blind rejection", status, ctx.OutputString())
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", thiefStatCooldownKey, time.Now().Unix(), 0); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want no thief_stat cooldown after blind rejection", used, remaining)
	}
}

func TestThiefStatHandlerBlocksCombatBeforeRevealLikeLegacy(t *testing.T) {
	loaded := perceptionWorld(t, model.ClassInvincible, []string{"STHIEF", "hidden", "PHIDDN", "invisible", "PINVIS"})
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	if _, err := world.AddEnemy("creature:merchant", "creature:alice"); err != nil {
		t.Fatalf("AddEnemy() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewThiefStatHandler(world, fixedPerceptionRoll(20))(ctx, ResolvedCommand{Args: []string{"상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "싸우는 도중에는 천안술을 펼칠 수 없습니다." {
		t.Fatalf("status/output = %d/%q, want legacy combat block", status, ctx.OutputString())
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN") ||
		!hasAnyNormalizedFlag(alice.Metadata.Tags, "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 1 || alice.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained before reveal", alice.Metadata.Tags, alice.Stats)
	}
	if remaining, ready, err := world.UseCreatureCooldown("creature:alice", thiefStatCooldownKey, time.Now().Unix(), 0); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !ready || remaining != 0 {
		t.Fatalf("cooldown ready/remaining = %v/%d, want no thief_stat cooldown after combat block", ready, remaining)
	}
}

func TestThiefStatHandlerFailedMonsterObjectPeekPrimesEnemy(t *testing.T) {
	loaded := perceptionWorld(t, model.ClassInvincible, []string{"STHIEF"})
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 1
	alice.Stats["level"] = 1
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewThiefStatHandler(world, fixedPerceptionRoll(100))(ctx, ResolvedCommand{Args: []string{"목검", "상인"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "소지품을 살피는 데 실패했습니다") {
		t.Fatalf("status/output = %d/%q, want object peek failure", status, ctx.OutputString())
	}
	enemies, err := world.CreatureEnemies("creature:merchant")
	if err != nil {
		t.Fatalf("CreatureEnemies() error = %v", err)
	}
	if len(enemies) != 1 || enemies[0] != "Alice" {
		t.Fatalf("merchant enemies = %+v, want Alice after failed thief_stat object peek", enemies)
	}
}

func TestThiefStatHandlerFailedMonsterCreaturePeekPrimesEnemy(t *testing.T) {
	loaded := perceptionWorld(t, model.ClassInvincible, []string{"STHIEF"})
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 1
	alice.Stats["level"] = 1
	loaded.Creatures[alice.ID] = alice
	merchant := loaded.Creatures["creature:merchant"]
	merchant.Stats["class"] = model.ClassCaretaker
	loaded.Creatures[merchant.ID] = merchant
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewThiefStatHandler(world, fixedPerceptionRoll(100))(ctx, ResolvedCommand{Args: []string{"상인"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "신상정보를 알아보는 데 실패했습니다") {
		t.Fatalf("status/output = %d/%q, want creature peek failure", status, ctx.OutputString())
	}
	enemies, err := world.CreatureEnemies("creature:merchant")
	if err != nil {
		t.Fatalf("CreatureEnemies() error = %v", err)
	}
	if len(enemies) != 1 || enemies[0] != "Alice" {
		t.Fatalf("merchant enemies = %+v, want Alice after failed thief_stat creature peek", enemies)
	}
}

func TestThiefStatHandlerPDMINVOwnerFallsBackToSelfLikeLegacy(t *testing.T) {
	loaded := perceptionWorld(t, model.ClassInvincible, []string{"STHIEF"})
	bobPlayer := loaded.Players["player:bob"]
	bobPlayer.RoomID = "room:start"
	loaded.Players[bobPlayer.ID] = bobPlayer
	bob := loaded.Creatures["creature:bob"]
	bob.RoomID = "room:start"
	bob.Metadata.Tags = []string{"PDMINV"}
	bob.Stats["PDMINV"] = 1
	bob.Inventory.ObjectIDs = []model.ObjectInstanceID{"object:wood-sword"}
	loaded.Creatures[bob.ID] = bob
	woodSword := loaded.Objects["object:wood-sword"]
	woodSword.Location = model.ObjectLocation{CreatureID: bob.ID, Slot: "inventory"}
	loaded.Objects[woodSword.ID] = woodSword
	merchant := loaded.Creatures["creature:merchant"]
	merchant.Inventory.ObjectIDs = nil
	loaded.Creatures[merchant.ID] = merchant
	start := loaded.Rooms["room:start"]
	start.PlayerIDs = append(start.PlayerIDs, "player:bob")
	start.CreatureIDs = append(start.CreatureIDs, bob.ID)
	loaded.Rooms[start.ID] = start
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewThiefStatHandler(world, fixedPerceptionRoll(20))(ctx, ResolvedCommand{
		Args:   []string{"목검", "player:bob"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "그것은 없습니다.") || strings.Contains(out, "이름: 목검") {
		t.Fatalf("status/output = %d/%q, want PDMINV owner to fall back to self", status, out)
	}
}

func TestThiefStatHandlerRejectsClassTrainingBlindAndMissingTarget(t *testing.T) {
	tests := []struct {
		name  string
		class int
		tags  []string
		args  []string
		want  string
	}{
		{name: "wrong class", class: model.ClassThief, tags: []string{"STHIEF"}, args: []string{"상인"}, want: "무적 이상만 사용할 수 있는 기술입니다."},
		{name: "missing training", class: model.ClassInvincible, args: []string{"상인"}, want: "도둑을 무적수련하지 않았습니다.."},
		{name: "missing target", class: model.ClassInvincible, tags: []string{"STHIEF"}, want: "무엇을 분석하시려구요?"},
		{name: "blind", class: model.ClassInvincible, tags: []string{"STHIEF", "PBLIND"}, args: []string{"상인"}, want: "당신은 눈이 멀어 천안술을 펼칠 수 없습니다."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := state.NewWorld(perceptionWorld(t, tt.class, tt.tags))
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewThiefStatHandler(world, fixedPerceptionRoll(20))(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func perceptionDispatcher(t *testing.T, world *state.World, roll SearchRollFunc) Dispatcher {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "혈마안", Number: 161, Handler: "red_eye"},
		{Name: "천안술", Number: 162, Handler: "thief_stat"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"red_eye":    NewRedEyeHandler(world, roll),
			"thief_stat": NewThiefStatHandler(world, roll),
		},
	}
}

func perceptionWorld(t *testing.T, class int, actorTags []string) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{ID: "room:start", DisplayName: "Start"})
	mustAddLookRoom(t, loaded, model.Room{ID: "room:remote", DisplayName: "Remote"})
	mustAddLookPlayer(t, loaded, model.Player{ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:start"})
	mustAddLookPlayer(t, loaded, model.Player{ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob", RoomID: "room:remote"})

	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:start",
		Stats: map[string]int{
			"class": class, "level": 60, "intelligence": 40, "dexterity": 40, "piety": 30,
			"hpCurrent": 100, "hpMax": 100, "mpCurrent": 50, "mpMax": 50, "thaco": 5,
		},
		Metadata: model.Metadata{Tags: append([]string(nil), actorTags...)},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:remote",
		Stats:       map[string]int{"class": model.ClassFighter, "level": 20, "hpCurrent": 80, "hpMax": 80, "thaco": 10},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:goblin",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:remote",
		Stats:       map[string]int{"class": model.ClassFighter, "level": 8, "hpCurrent": 300, "hpMax": 300, "armor": 0},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:start",
		Stats: map[string]int{
			"class": model.ClassFighter, "level": 8, "race": 1, "alignment": 120,
			"hpCurrent": 35, "hpMax": 35, "mpCurrent": 4, "mpMax": 4,
			"experience": 200, "gold": 25, "armor": 80, "sDice": 4, "nDice": 2, "pDice": 1,
			"strength": 12, "dexterity": 13, "constitution": 11, "intelligence": 10, "piety": 9, "thaco": 15,
		},
		Properties: map[string]string{"raceName": "인간"},
		Inventory:  model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:wood-sword"}},
		Metadata:   model.Metadata{Tags: []string{"detectInvisible"}},
	})

	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:wood-sword",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "목검",
		Description: "낡은 목검",
		Keywords:    []string{"목검"},
		Properties: map[string]string{
			"type": "1", "shotsCurrent": "2", "shotsMax": "5",
			"sDice": "4", "nDice": "2", "pDice": "1", "adjustment": "1",
			"value": "150", "weight": "3", "useOutput": "휘두르면 둔탁한 소리가 난다",
		},
		Metadata: model.Metadata{Tags: []string{"goodOnly", "enchanted"}},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:wood-sword",
		PrototypeID: "prototype:wood-sword",
		Location:    model.ObjectLocation{CreatureID: "creature:merchant", Slot: "inventory"},
	})
	return loaded
}

func fixedPerceptionRoll(value int) SearchRollFunc {
	return func(int, int) int {
		return value
	}
}
