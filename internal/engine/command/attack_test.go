package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestAttackHandlerAttacksMonsterByPrefixAndOrdinal(t *testing.T) {
	// Pin the crit/fumble gates (attack_crt consumes mrand(1,100) twice per hit,
	// command5.c:280,307) so the single armed swing never fumbles.
	withAttackRolls(t, 30, 100, 100)
	world := state.NewWorld(attackTestWorld(t))
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "고블린 2 때려")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if got := ctx.OutputString(); !strings.Contains(got, "고블린에게 4만큼의 피해") {
		t.Fatalf("output = %q, want attack damage confirmation", got)
	}
	first, _ := world.Creature("creature:goblin-1")
	second, _ := world.Creature("creature:goblin-2")
	if first.Stats["hpCurrent"] != 9 {
		t.Fatalf("first goblin hp = %d, want untouched 9", first.Stats["hpCurrent"])
	}
	if second.Stats["hpCurrent"] != 5 {
		t.Fatalf("second goblin hp = %d, want 5", second.Stats["hpCurrent"])
	}
}

func TestAttackHandlerRejectsZeroClassLikeLegacy(t *testing.T) {
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["class"] = 0
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "고블린 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); !strings.Contains(got, "당신은 전투가 금지된 직업을 갖고 있습니다.") {
		t.Fatalf("output = %q, want legacy class-zero refusal", got)
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 9; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
}

func TestAttackHandlerRespectsAttackCooldownBeforeLookupLikeLegacy(t *testing.T) {
	withFakeMagicEffectTime(t, 1000)
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden")
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	defer world.Close()
	if err := world.SetCreatureCooldown("creature:alice", "attack", 1000, 5); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "고블린 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); got != "5초동안 기다리세요.\n" {
		t.Fatalf("output = %q, want please_wait", got)
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden") {
		t.Fatalf("alice tags = %+v, want hidden retained during cooldown", alice.Metadata.Tags)
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 9; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
}

func TestAttackHandlerBlindActorUsesLegacyMissingTargetMessage(t *testing.T) {
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["PBLIND"] = 1
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "고블린 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); got != "누구를 공격하시려구요?" {
		t.Fatalf("output = %q, want legacy blind attack refusal", got)
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 9; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
}

func TestAttackHandlerPaladinAlreadyFightingUsesEnemyListLikeLegacy(t *testing.T) {
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["class"] = model.ClassPaladin
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	defer world.Close()
	if _, err := world.AddEnemy("creature:goblin-1", "creature:alice"); err != nil {
		t.Fatalf("AddEnemy() error = %v", err)
	}
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "고블린 2 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); got != "당신은 지금 싸우고 있잖아요!" {
		t.Fatalf("output = %q, want legacy already-fighting refusal", got)
	}
	first, _ := world.Creature("creature:goblin-1")
	second, _ := world.Creature("creature:goblin-2")
	if got, want := first.Stats["hpCurrent"], 9; got != want {
		t.Fatalf("first goblin hp = %d, want %d", got, want)
	}
	if got, want := second.Stats["hpCurrent"], 9; got != want {
		t.Fatalf("second goblin hp = %d, want %d", got, want)
	}
}

func TestAttackHandlerSupportsCommandFirstFallback(t *testing.T) {
	withAttackRolls(t, 30, 100, 100) // pin crit/fumble gates so the swing lands
	world := state.NewWorld(attackTestWorld(t))
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "때려 고블린")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if goblin.Stats["hpCurrent"] != 5 {
		t.Fatalf("goblin hp = %d, want 5", goblin.Stats["hpCurrent"])
	}
}

func TestAttackHandlerFinishingBlowFinalizesMonsterDeath(t *testing.T) {
	withAttackRolls(t, 30, 100, 100) // pin crit/fumble gates so the swing lands
	world := state.NewWorld(attackTestWorld(t))
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "생쥐 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); !strings.Contains(got, "생쥐가 쓰러졌습니다.") {
		t.Fatalf("output = %q, want death message", got)
	}
	if _, ok := world.Creature("creature:mouse"); ok {
		t.Fatal("dead creature still exists in world")
	}
	room, _ := world.Room("room:arena")
	if containsCreatureID(room.CreatureIDs, "creature:mouse") {
		t.Fatalf("room creatures = %+v, want mouse removed", room.CreatureIDs)
	}
	rendered := RenderRoomLook(world, room, LookViewer{PlayerID: "player:alice", CreatureID: "creature:alice"})
	if strings.Contains(rendered, "생쥐") {
		t.Fatalf("dead creature should not render in room look:\n%s", rendered)
	}
	cheese, ok := world.Object("object:cheese")
	if !ok || cheese.Location.RoomID != "room:arena" {
		t.Fatalf("cheese = %+v/%v, want dropped in room", cheese, ok)
	}
	foundMoney := false
	for _, objectID := range room.Objects.ObjectIDs {
		object, ok := world.Object(objectID)
		if ok && object.DisplayNameOverride == "5냥" && object.Properties["value"] == "5" {
			foundMoney = true
			break
		}
	}
	if !foundMoney {
		t.Fatalf("room objects = %+v, want 5냥 money drop", room.Objects.ObjectIDs)
	}
	alice, _ := world.Creature("creature:alice")
	if got, want := alice.Stats["experience"], 130; got != want {
		t.Fatalf("alice experience = %d, want %d", got, want)
	}
	if got, want := alice.Stats["alignment"], 0; got != want {
		t.Fatalf("alice alignment = %d, want %d", got, want)
	}
}

func TestAttackHandlerUsesCustomDeathFinalizer(t *testing.T) {
	withAttackRolls(t, 30, 100, 100) // pin crit/fumble gates so the swing lands
	world := state.NewWorld(attackTestWorld(t))
	defer world.Close()
	called := false
	dispatcher := attackTestDispatcherWithHandler(t, NewAttackHandlerWithDeathFinalizer(world, func(_ *Context, attacker model.Creature, victim model.Creature) error {
		called = true
		if attacker.ID != "creature:alice" || victim.ID != "creature:mouse" {
			t.Fatalf("finalizer attacker/victim = %q/%q, want alice/mouse", attacker.ID, victim.ID)
		}
		_, err := world.FinalizeMonsterDeathWithOptions(victim.ID, state.FinalizeMonsterDeathOptions{
			RewardGroup: state.MonsterDeathRewardGroup{
				LeaderID: "creature:alice",
			},
		})
		return err
	}))

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "생쥐 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if !called {
		t.Fatal("custom finalizer was not called")
	}
	if _, ok := world.Creature("creature:mouse"); ok {
		t.Fatal("dead creature still exists in world")
	}
}

func TestAttackHandlerReportsMissWithoutDamage(t *testing.T) {
	withAttackRolls(t, 9)
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["thaco"] = 20
	loaded.Creatures[alice.ID] = alice
	goblin := loaded.Creatures["creature:goblin-1"]
	goblin.Stats["armor"] = 100
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "고블린 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); !strings.Contains(got, "당신의 공격은 빗나갔습니다.") {
		t.Fatalf("output = %q, want miss message", got)
	}
	goblin, _ = world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 9; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
}

func TestAttackHandlerAppliesPaladinAlignmentDamageMessages(t *testing.T) {
	tests := []struct {
		name       string
		alignment  int
		rolls      []int
		wantOutput string
		wantDamage int
	}{
		{
			name:       "negative alignment halves damage",
			alignment:  -1,
			rolls:      []int{30, 100, 100},
			wantOutput: "당신의 악행이 양심을 괴롭힙니다.",
			wantDamage: 2,
		},
		{
			name:       "high alignment adds damage",
			alignment:  300,
			rolls:      []int{30, 2, 100, 100},
			wantOutput: "당신의 선행이 능력을 배가시킵니다.",
			wantDamage: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withAttackRolls(t, tt.rolls...)
			loaded := attackTestWorld(t)
			alice := loaded.Creatures["creature:alice"]
			alice.Stats["class"] = model.ClassPaladin
			alice.Stats["alignment"] = tt.alignment
			loaded.Creatures[alice.ID] = alice
			world := state.NewWorld(loaded)
	defer world.Close()
			dispatcher := attackTestDispatcher(t, world)

			ctx := &Context{ActorID: "player:alice"}
			if _, err := dispatcher.DispatchLine(ctx, "고블린 때려"); err != nil {
				t.Fatalf("DispatchLine() error = %v", err)
			}
			if got := ctx.OutputString(); !strings.Contains(got, tt.wantOutput) {
				t.Fatalf("output = %q, want %q", got, tt.wantOutput)
			}
			goblin, _ := world.Creature("creature:goblin-1")
			if got, want := goblin.Stats["hpCurrent"], 9-tt.wantDamage; got != want {
				t.Fatalf("goblin hp = %d, want %d", got, want)
			}
		})
	}
}

func TestAttackHandlerAppliesAngelExtraDamage(t *testing.T) {
	withAttackRolls(t, 30, 100, 100, 1, 3)
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 20
	alice.Stats["intelligence"] = 30
	alice.Stats["piety"] = 30
	alice.Metadata.Tags = append(alice.Metadata.Tags, "PANGEL")
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "고블린 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); !strings.Contains(got, "당신의 정령이 고블린에게 3만큼의 피해") {
		t.Fatalf("output = %q, want angel damage message", got)
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 2; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
}

func TestAttackHandlerUpDamageCanAttackTwice(t *testing.T) {
	withAttackRolls(t, 0, 30, 100, 100, 30, 100, 100)
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 128
	alice.Stats["class"] = model.ClassInvincible
	alice.Metadata.Tags = append(alice.Metadata.Tags, "PUPDMG")
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "고블린 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := strings.Count(ctx.OutputString(), "고블린에게 4만큼의 피해"); got != 2 {
		t.Fatalf("damage message count = %d, want 2; output = %q", got, ctx.OutputString())
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 1; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
}

func TestAttackHandlerPlayerGateMatchesLegacyPvPConditions(t *testing.T) {
	tests := []struct {
		name       string
		setupLoad  func(*worldload.World)
		setupWorld func(*testing.T, *state.World)
		want       string
		wantBobHP  int
	}{
		{
			name: "safe room without active war rejects",
			setupLoad: func(loaded *worldload.World) {
				room := loaded.Rooms["room:arena"]
				room.Metadata.Tags = append(room.Metadata.Tags, "RNOKIL")
				loaded.Rooms[room.ID] = room
			},
			want:      "이 곳에서는 싸울 수 없습니다.",
			wantBobHP: 20,
		},
		{
			name:      "lawful attacker outside survival rejects",
			want:      "당신은 선하다는걸 아세요.",
			wantBobHP: 20,
		},
		{
			name: "chaotic attacker cannot attack lawful target",
			setupLoad: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = append(alice.Metadata.Tags, "PCHAOS")
				loaded.Creatures[alice.ID] = alice
			},
			want:      "그 사용자는 선해서 공격할 수 없습니다.",
			wantBobHP: 20,
		},
		{
			name: "chaotic players pass gate and apply damage",
			setupLoad: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = append(alice.Metadata.Tags, "PCHAOS")
				loaded.Creatures[alice.ID] = alice
				bob := loaded.Creatures["creature:bob"]
				bob.Metadata.Tags = append(bob.Metadata.Tags, "PCHAOS")
				loaded.Creatures[bob.ID] = bob
			},
			want:      "Bob에게 4만큼의 피해",
			wantBobHP: 16,
		},
		{
			name: "survival room passes gate and applies damage",
			setupLoad: func(loaded *worldload.World) {
				room := loaded.Rooms["room:arena"]
				room.Metadata.Tags = append(room.Metadata.Tags, "RSUVIV")
				loaded.Rooms[room.ID] = room
			},
			want:      "Bob에게 4만큼의 피해",
			wantBobHP: 16,
		},
		{
			name: "family war passes gate even in safe room",
			setupLoad: func(loaded *worldload.World) {
				room := loaded.Rooms["room:arena"]
				room.Metadata.Tags = append(room.Metadata.Tags, "RNOKIL")
				loaded.Rooms[room.ID] = room
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["familyFlag"] = 1
				alice.Stats["familyID"] = 2
				loaded.Creatures[alice.ID] = alice
				bob := loaded.Creatures["creature:bob"]
				bob.Stats["familyFlag"] = 1
				bob.Stats["familyID"] = 5
				loaded.Creatures[bob.ID] = bob
			},
			setupWorld: func(t *testing.T, world *state.World) {
				t.Helper()
				if _, err := world.RequestFamilyWar(2, 5); err != nil {
					t.Fatalf("RequestFamilyWar() error = %v", err)
				}
				if _, err := world.AcceptFamilyWar(5, 2); err != nil {
					t.Fatalf("AcceptFamilyWar() error = %v", err)
				}
			},
			want:      "Bob에게 4만큼의 피해",
			wantBobHP: 16,
		},
		{
			name: "family players not at war still need chaos or survival",
			setupLoad: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Stats["familyFlag"] = 1
				alice.Stats["familyID"] = 2
				loaded.Creatures[alice.ID] = alice
				bob := loaded.Creatures["creature:bob"]
				bob.Stats["familyFlag"] = 1
				bob.Stats["familyID"] = 5
				loaded.Creatures[bob.ID] = bob
			},
			want:      "당신은 선하다는걸 아세요.",
			wantBobHP: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Pin the crit/fumble gates so the cases that land deal deterministic
			// damage; rejected cases never reach the swing and leave them unconsumed.
			withAttackRolls(t, 30, 100, 100)
			loaded := attackTestWorld(t)
			if tt.setupLoad != nil {
				tt.setupLoad(loaded)
			}
			world := state.NewWorld(loaded)
	defer world.Close()
			if tt.setupWorld != nil {
				tt.setupWorld(t, world)
			}
			dispatcher := attackTestDispatcher(t, world)

			ctx := &Context{ActorID: "player:alice"}
			if _, err := dispatcher.DispatchLine(ctx, "Bob 때려"); err != nil {
				t.Fatalf("DispatchLine() error = %v", err)
			}
			if got := ctx.OutputString(); !strings.Contains(got, tt.want) {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
			bob, _ := world.Creature("creature:bob")
			if got, want := bob.Stats["hpCurrent"], tt.wantBobHP; got != want {
				t.Fatalf("bob hp = %d, want %d", got, want)
			}
		})
	}
}

func TestAttackHandlerRejectsCharmedPlayerBeforeRevealLikeLegacy(t *testing.T) {
	withFakeMagicEffectTime(t, 1000)
	loaded := attackTestWorld(t)
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
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "Bob 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); !strings.Contains(got, "너무 사랑해서") {
		t.Fatalf("output = %q, want charm refusal", got)
	}
	bob, _ = world.Creature("creature:bob")
	if got, want := bob.Stats["hpCurrent"], 20; got != want {
		t.Fatalf("bob hp = %d, want %d", got, want)
	}
	if remaining, ready, err := world.UseCreatureCooldown("creature:alice", "attack", 1000, 0); err != nil || !ready || remaining != 0 {
		t.Fatalf("attack cooldown remaining/ready/err = %d/%v/%v, want unused", remaining, ready, err)
	}
	alice, _ = world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS") ||
		alice.Stats["PHIDDN"] != 1 || alice.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained", alice.Metadata.Tags, alice.Stats)
	}
}

func TestAttackHandlerRemovesStaleTargetCharmReferenceLikeLegacy(t *testing.T) {
	withAttackRolls(t, 30, 100, 100)
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PCHAOS"}
	loaded.Creatures[alice.ID] = alice
	bob := loaded.Creatures["creature:bob"]
	bob.Metadata.Tags = []string{"PCHAOS", "charm:Alice", "charmID:creature:alice"}
	loaded.Creatures[bob.ID] = bob
	bobPlayer := loaded.Players["player:bob"]
	bobPlayer.Metadata.Tags = []string{"charm:Alice", "charmID:creature:alice"}
	loaded.Players[bobPlayer.ID] = bobPlayer
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "Bob 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); !strings.Contains(got, "Bob에게 4만큼의 피해") {
		t.Fatalf("output = %q, want attack to continue after stale charm removal", got)
	}
	bob, _ = world.Creature("creature:bob")
	for _, tag := range []string{"charm:Alice", "charmID:creature:alice"} {
		if magicEffectTestHasExactTag(bob.Metadata.Tags, tag) {
			t.Fatalf("bob creature tags = %+v, want %q removed", bob.Metadata.Tags, tag)
		}
	}
	bobPlayer, _ = world.Player("player:bob")
	for _, tag := range []string{"charm:Alice", "charmID:creature:alice"} {
		if magicEffectTestHasExactTag(bobPlayer.Metadata.Tags, tag) {
			t.Fatalf("bob player tags = %+v, want %q removed", bobPlayer.Metadata.Tags, tag)
		}
	}
}

func TestAttackHandlerPlayerTargetDeathClampsHPWithoutMonsterFinalize(t *testing.T) {
	// Pin the crit/fumble gates so the single armed swing lands deterministically.
	withAttackRolls(t, 30, 100, 100)
	loaded := attackTestWorld(t)
	room := loaded.Rooms["room:arena"]
	room.Metadata.Tags = append(room.Metadata.Tags, "RSUVIV")
	loaded.Rooms[room.ID] = room
	bob := loaded.Creatures["creature:bob"]
	bob.Stats["hpCurrent"] = 3
	loaded.Creatures[bob.ID] = bob
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "Bob 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	gotOutput := ctx.OutputString()
	if !strings.Contains(gotOutput, "Bob에게 3만큼의 피해") || !strings.Contains(gotOutput, "쓰러졌습니다.") {
		t.Fatalf("output = %q, want clamped damage and death message", gotOutput)
	}
	bob, ok := world.Creature("creature:bob")
	if !ok {
		t.Fatal("dead player creature was removed")
	}
	if got, want := bob.Stats["hpCurrent"], 0; got != want {
		t.Fatalf("bob hp = %d, want %d", got, want)
	}
	room, _ = world.Room("room:arena")
	if !containsCreatureID(room.CreatureIDs, "creature:bob") {
		t.Fatalf("room creatures = %+v, want bob retained", room.CreatureIDs)
	}
}

func TestAttackHandlerRevealsHiddenInvisibleActor(t *testing.T) {
	withAttackRolls(t, 30, 100, 100)
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS", "dmInvisible", "PDMINV"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	alice.Stats["PDMINV"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS", "dmInvisible", "PDMINV"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	if _, err := dispatcher.DispatchLine(ctx, "고블린 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); !strings.Contains(got, "당신은 모습을 드러냅니다.") {
		t.Fatalf("output = %q, want reveal message", got)
	}
	alice, _ = world.Creature("creature:alice")
	for _, flag := range []string{"hidden", "PHIDDN", "invisible", "PINVIS"} {
		if creatureHasAnyFlag(alice, flag) {
			t.Fatalf("alice tags = %+v, want %q cleared", alice.Metadata.Tags, flag)
		}
	}
	if alice.Stats["PHIDDN"] != 0 || alice.Stats["PINVIS"] != 0 {
		t.Fatalf("alice reveal stats = %+v, want PHIDDN/PINVIS cleared", alice.Stats)
	}
	if !creatureHasAnyFlag(alice, "dmInvisible", "PDMINV") || alice.Stats["PDMINV"] != 1 {
		t.Fatalf("alice DM invis state = tags:%+v stats:%+v, want retained", alice.Metadata.Tags, alice.Stats)
	}
	player, _ = world.Player("player:alice")
	for _, flag := range []string{"hidden", "PHIDDN", "invisible", "PINVIS"} {
		if hasAnyNormalizedFlag(player.Metadata.Tags, flag) {
			t.Fatalf("player tags = %+v, want %q cleared", player.Metadata.Tags, flag)
		}
	}
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "dmInvisible", "PDMINV") {
		t.Fatalf("player DM invis tags = %+v, want retained", player.Metadata.Tags)
	}
	if len(broadcasts) == 0 || !strings.Contains(broadcasts[0].Text, "모습을 드러냅니다") {
		t.Fatalf("broadcasts = %+v, want reveal broadcast", broadcasts)
	}
}

func TestAttackHandlerDoesNotRevealDMInvisibleActorLikeLegacy(t *testing.T) {
	withAttackRolls(t, 30, 100, 100)
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN", "dmInvisible", "PDMINV"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PDMINV"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "dmInvisible", "PDMINV"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	if _, err := dispatcher.DispatchLine(ctx, "고블린 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); strings.Contains(got, "모습을 드러냅니다") {
		t.Fatalf("output = %q, did not want ordinary reveal for PDMINV", got)
	}
	alice, _ = world.Creature("creature:alice")
	if creatureHasAnyFlag(alice, "hidden", "PHIDDN") || alice.Stats["PHIDDN"] != 0 {
		t.Fatalf("alice hidden state = tags:%+v stats:%+v, want cleared", alice.Metadata.Tags, alice.Stats)
	}
	if !creatureHasAnyFlag(alice, "dmInvisible", "PDMINV") || alice.Stats["PDMINV"] != 1 {
		t.Fatalf("alice DM invis state = tags:%+v stats:%+v, want retained", alice.Metadata.Tags, alice.Stats)
	}
	player, _ = world.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "PHIDDN") {
		t.Fatalf("player hidden tags = %+v, want cleared", player.Metadata.Tags)
	}
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "dmInvisible", "PDMINV") {
		t.Fatalf("player DM invis tags = %+v, want retained", player.Metadata.Tags)
	}
	for _, broadcast := range broadcasts {
		if strings.Contains(broadcast.Text, "모습을 드러냅니다") {
			t.Fatalf("broadcasts = %+v, did not want ordinary reveal for PDMINV", broadcasts)
		}
	}
}

func TestAttackHandlerSpentWieldStopsAttackAndUnequips(t *testing.T) {
	loaded := attackTestWorld(t)
	sword := loaded.Objects["object:sword"]
	sword.Properties = map[string]string{"shotsCurrent": "0"}
	loaded.Objects[sword.ID] = sword
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "고블린 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); !strings.Contains(got, "목검이 부서져 버렸습니다.") {
		t.Fatalf("output = %q, want spent weapon message", got)
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 9; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	alice, _ := world.Creature("creature:alice")
	if got := alice.Equipment["wield"]; got != "" {
		t.Fatalf("alice wield = %q, want empty", got)
	}
	sword, _ = world.Object("object:sword")
	if sword.Location.CreatureID != "creature:alice" || sword.Location.Slot != "inventory" {
		t.Fatalf("sword location = %+v, want alice inventory", sword.Location)
	}
}

func TestAttackHandlerSpentWieldCanStopSecondUpDamageAttack(t *testing.T) {
	withAttackRolls(t, 0, 30, 100, 100, 0)
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 128
	alice.Stats["class"] = model.ClassInvincible
	alice.Metadata.Tags = append(alice.Metadata.Tags, "PUPDMG")
	loaded.Creatures[alice.ID] = alice
	sword := loaded.Objects["object:sword"]
	sword.Properties = map[string]string{"shotsCurrent": "1"}
	loaded.Objects[sword.ID] = sword
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "고블린 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); !strings.Contains(got, "고블린에게 4만큼의 피해") ||
		!strings.Contains(got, "목검이 부서져 버렸습니다.") {
		t.Fatalf("output = %q, want first damage and spent weapon message", got)
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 5; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	alice, _ = world.Creature("creature:alice")
	if got := alice.Equipment["wield"]; got != "" {
		t.Fatalf("alice wield = %q, want empty", got)
	}
	sword, _ = world.Object("object:sword")
	if sword.Location.CreatureID != "creature:alice" || sword.Location.Slot != "inventory" {
		t.Fatalf("sword location = %+v, want alice inventory", sword.Location)
	}
}

func TestAttackHandlerWieldChargeCanDecreaseAfterHit(t *testing.T) {
	withAttackRolls(t, 30, 100, 100, 0)
	loaded := attackTestWorld(t)
	sword := loaded.Objects["object:sword"]
	sword.Properties = map[string]string{"shotsCurrent": "2"}
	loaded.Objects[sword.ID] = sword
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "고블린 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	sword, _ = world.Object("object:sword")
	if got := sword.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("sword shotsCurrent = %q, want 1", got)
	}
}

func TestAttackHandlerDeflectsMagicOnlyAndEnchantOnlyTargets(t *testing.T) {
	tests := []struct {
		name             string
		targetTags       []string
		weaponProperties map[string]string
		rolls            []int
		wantOutput       string
		wantHP           int
	}{
		{
			name:       "magic only",
			targetTags: []string{"magicOnly"},
			wantOutput: "그 상대에게는 아무 소용이 없습니다.",
			wantHP:     9,
		},
		{
			name:       "enchant only without adjusted weapon",
			targetTags: []string{"magicOrEnchantedOnly"},
			wantOutput: "그 상대에게는 아무 소용이 없습니다.",
			wantHP:     9,
		},
		{
			name:             "enchant only with adjusted weapon",
			targetTags:       []string{"magicOrEnchantedOnly"},
			weaponProperties: map[string]string{"adjustment": "1"},
			rolls:            []int{30, 100, 100},
			wantOutput:       "고블린에게 4만큼의 피해",
			wantHP:           5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.rolls) > 0 {
				withAttackRolls(t, tt.rolls...)
			}
			loaded := attackTestWorld(t)
			goblin := loaded.Creatures["creature:goblin-1"]
			goblin.Metadata.Tags = tt.targetTags
			loaded.Creatures[goblin.ID] = goblin
			if tt.weaponProperties != nil {
				sword := loaded.Objects["object:sword"]
				sword.Properties = tt.weaponProperties
				loaded.Objects[sword.ID] = sword
			}
			world := state.NewWorld(loaded)
	defer world.Close()
			dispatcher := attackTestDispatcher(t, world)

			ctx := &Context{ActorID: "player:alice"}
			if _, err := dispatcher.DispatchLine(ctx, "고블린 때려"); err != nil {
				t.Fatalf("DispatchLine() error = %v", err)
			}
			if got := ctx.OutputString(); !strings.Contains(got, tt.wantOutput) {
				t.Fatalf("output = %q, want %q", got, tt.wantOutput)
			}
			goblin, _ = world.Creature("creature:goblin-1")
			if got := goblin.Stats["hpCurrent"]; got != tt.wantHP {
				t.Fatalf("goblin hp = %d, want %d", got, tt.wantHP)
			}
		})
	}
}

func TestAttackDamageUsesHitRollArmorDiceStrengthAndProficiency(t *testing.T) {
	withAttackRolls(t, 8, 6, 5, 100, 100)
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["thaco"] = 20
	alice.Stats["strength"] = 20
	alice.Properties = map[string]string{"proficiency/1": "20"}
	loaded.Creatures[alice.ID] = alice
	goblin := loaded.Creatures["creature:goblin-1"]
	goblin.Stats["armor"] = 120
	loaded.Creatures[goblin.ID] = goblin
	swordProto := loaded.ObjectPrototypes["prototype:sword"]
	swordProto.Properties = map[string]string{"type": "1", "nDice": "2", "sDice": "6", "pDice": "1"}
	loaded.ObjectPrototypes[swordProto.ID] = swordProto
	world := state.NewWorld(loaded)
	defer world.Close()

	attacker, _ := world.Creature("creature:alice")
	victim, _ := world.Creature("creature:goblin-1")
	damage, hit := attackDamage(world, attacker, victim)
	if !hit {
		t.Fatal("attackDamage() hit = false, want true")
	}
	// C attack_crt (command5.c:241) adds profic(ply, weapon->type)/10. Raw
	// proficiency 20 is far below the Fighter table's first rank threshold (768),
	// so profic() ranks it to 0% and contributes 0 damage — dice(15) + str + 0.
	// (The pre-fix code used raw 20/10 = 2 here; see combat proficiency bug.)
	if damage != 15 {
		t.Fatalf("attackDamage() damage = %d, want 15", damage)
	}
}

func TestAttackDamageAppliesFearAndBlindHitPenalty(t *testing.T) {
	withAttackRolls(t, 10)
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["thaco"] = 14
	alice.Stats["PFEARS"] = 1
	alice.Metadata.Tags = append(alice.Metadata.Tags, "blind")
	loaded.Creatures[alice.ID] = alice
	goblin := loaded.Creatures["creature:goblin-1"]
	goblin.Stats["armor"] = 100
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	defer world.Close()

	attacker, _ := world.Creature("creature:alice")
	victim, _ := world.Creature("creature:goblin-1")
	damage, hit := attackDamage(world, attacker, victim)
	if hit {
		t.Fatalf("attackDamage() hit = true, want false with fear/blind penalty; damage = %d", damage)
	}
	if damage != 0 {
		t.Fatalf("attackDamage() damage = %d, want 0", damage)
	}
}

func TestAttackDamageAddsHeldWeaponTenthDamage(t *testing.T) {
	withAttackRolls(t, 30, 100, 100)
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Equipment["held"] = "object:dagger"
	loaded.Creatures[alice.ID] = alice
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:dagger",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "단검",
		Properties:  map[string]string{"type": "0", "pDice": "25"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:dagger",
		PrototypeID: "prototype:dagger",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "held"},
	})
	world := state.NewWorld(loaded)
	defer world.Close()

	attacker, _ := world.Creature("creature:alice")
	victim, _ := world.Creature("creature:goblin-1")
	damage, hit := attackDamage(world, attacker, victim)
	if !hit {
		t.Fatal("attackDamage() hit = false, want true")
	}
	if damage != 6 {
		t.Fatalf("attackDamage() damage = %d, want 6", damage)
	}
}

func TestAttackDamageAddsUnarmedLevelBonusForBarbarianAndAboveInvincible(t *testing.T) {
	tests := []struct {
		name  string
		class int
	}{
		{name: "barbarian", class: model.ClassBarbarian},
		{name: "above invincible", class: model.ClassInvincible + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Trailing roll pins the Caretaker+ swing multiplier (attack_crt num =
			// mrand(1,8), command5.c:355) to 1 so this test isolates the unarmed
			// level bonus; the Barbarian case never consumes it.
			withAttackRolls(t, 30, 100, 100, 1)
			loaded := attackTestWorld(t)
			alice := loaded.Creatures["creature:alice"]
			alice.Equipment = nil
			alice.Stats["class"] = tt.class
			alice.Stats["level"] = 9
			alice.Stats["pDice"] = 4
			loaded.Creatures[alice.ID] = alice
			world := state.NewWorld(loaded)
	defer world.Close()

			attacker, _ := world.Creature("creature:alice")
			victim, _ := world.Creature("creature:goblin-1")
			damage, hit := attackDamage(world, attacker, victim)
			if !hit {
				t.Fatal("attackDamage() hit = false, want true")
			}
			if damage != 7 {
				t.Fatalf("attackDamage() damage = %d, want 7", damage)
			}
		})
	}
}

func TestAttackDamageOmitsWeaponProficiencyForMageAndCleric(t *testing.T) {
	tests := []struct {
		name  string
		class int
	}{
		{name: "mage", class: model.ClassMage},
		{name: "cleric", class: model.ClassCleric},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withAttackRolls(t, 30, 100, 100)
			loaded := attackTestWorld(t)
			alice := loaded.Creatures["creature:alice"]
			alice.Stats["class"] = tt.class
			alice.Stats["strength"] = 20
			alice.Properties = map[string]string{"proficiency/1": "100"}
			loaded.Creatures[alice.ID] = alice
			swordProto := loaded.ObjectPrototypes["prototype:sword"]
			swordProto.Properties = map[string]string{"type": "1", "pDice": "4"}
			loaded.ObjectPrototypes[swordProto.ID] = swordProto
			world := state.NewWorld(loaded)
	defer world.Close()

			attacker, _ := world.Creature("creature:alice")
			victim, _ := world.Creature("creature:goblin-1")
			damage, hit := attackDamage(world, attacker, victim)
			if !hit {
				t.Fatal("attackDamage() hit = false, want true")
			}
			if damage != 7 {
				t.Fatalf("attackDamage() damage = %d, want 7", damage)
			}
		})
	}
}

// TestAttackDamageFloorsAgainstDMVictim guards the C attack_crt DM-victim floor
// (command5.c:265 `if(crt_ptr->class >= DM) n = 0;` before MAX(1,n)): however
// large the base damage, a DM-class victim takes exactly 1.
func TestAttackDamageFloorsAgainstDMVictim(t *testing.T) {
	withAttackRolls(t, 30, 100, 100)
	loaded := attackTestWorld(t)
	goblin := loaded.Creatures["creature:goblin-1"]
	goblin.Stats["class"] = model.ClassDM
	loaded.Creatures[goblin.ID] = goblin
	world := state.NewWorld(loaded)
	defer world.Close()

	attacker, _ := world.Creature("creature:alice")
	victim, _ := world.Creature("creature:goblin-1")
	damage, hit := attackDamage(world, attacker, victim)
	if !hit {
		t.Fatal("attackDamage() hit = false, want true")
	}
	if damage != 1 {
		t.Fatalf("attackDamage() damage = %d, want 1 (DM-class victim damage floored)", damage)
	}
}

// TestAttackDamageAppliesCaretakerSwingMultiplier guards the C attack_crt swing
// multiplier for Caretaker+ classes (command5.c:355 num = mrand(1,8), then
// command5.c:361 `n = n * num * 0.9`). Unarmed base is 7 (dice 4 + level bonus
// (9+3)/4 = 3); num rolled as 8 gives int(7 * 8 * 0.9) = 50.
func TestAttackDamageAppliesCaretakerSwingMultiplier(t *testing.T) {
	withAttackRolls(t, 30, 100, 100, 8)
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Equipment = nil
	alice.Stats["class"] = model.ClassCaretaker
	alice.Stats["level"] = 9
	alice.Stats["pDice"] = 4
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	defer world.Close()

	attacker, _ := world.Creature("creature:alice")
	victim, _ := world.Creature("creature:goblin-1")
	damage, hit := attackDamage(world, attacker, victim)
	if !hit {
		t.Fatal("attackDamage() hit = false, want true")
	}
	if damage != 50 {
		t.Fatalf("attackDamage() damage = %d, want 50 (7 x8 x0.9)", damage)
	}
}

// TestAttackHandlerShowsSwingMultiplierPrefix guards the "(xN)" prefix on the
// damage line when the swing multiplier fires (command5.c:369).
func TestAttackHandlerShowsSwingMultiplierPrefix(t *testing.T) {
	withAttackRolls(t, 30, 100, 100, 8)
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["class"] = model.ClassCaretaker
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "고블린 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); !strings.Contains(got, "(x8) 당신은 고블린에게") {
		t.Fatalf("output = %q, want (x8) swing-multiplier prefix", got)
	}
}

// TestAttackHandlerCriticalHitShattersWeapon guards the C attack_crt critical
// stage (command5.c:280-306): when mrand(1,100) <= mod_profic and the weapon
// carries OALCRT, damage is multiplied by mrand(3,6) and the weapon shatters
// (destroyed) unless it is shatterproof or an event item.
func TestAttackHandlerCriticalHitShattersWeapon(t *testing.T) {
	// hit 30, crit gate 1 (<= p=1), crit multiplier mrand(3,6)=3, shatter gate 50.
	withAttackRolls(t, 30, 1, 3, 50)
	loaded := attackTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["proficiency/1"] = 1024 // Fighter rank 20 -> mod_profic p = 1
	loaded.Creatures[alice.ID] = alice
	goblin := loaded.Creatures["creature:goblin-1"]
	goblin.Stats["hpCurrent"] = 40
	goblin.Stats["hpMax"] = 40
	loaded.Creatures[goblin.ID] = goblin
	swordProto := loaded.ObjectPrototypes["prototype:sword"]
	swordProto.Properties = map[string]string{"pDice": "4", "type": "1", "OALCRT": "1"}
	loaded.ObjectPrototypes[swordProto.ID] = swordProto
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "고블린 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "치명타를 날렸습니다") || !strings.Contains(out, "산산히 부서집니다") {
		t.Fatalf("output = %q, want critical and shatter messages", out)
	}
	// base 6 (dice 4 + profic rank 20/10 = 2), crit x3 = 18; goblin 40-18 = 22.
	goblin, _ = world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 22; got != want {
		t.Fatalf("goblin hp = %d, want %d", got, want)
	}
	if _, ok := world.Object("object:sword"); ok {
		t.Fatalf("weapon still exists, want shattered (destroyed)")
	}
}

// TestAttackHandlerFumbleDropsWeapon guards the C attack_crt fumble stage
// (command5.c:307-314): when mrand(1,100) <= (5 - mod_profic) the swing deals no
// damage and the weapon is unequipped to the attacker's inventory.
func TestAttackHandlerFumbleDropsWeapon(t *testing.T) {
	// hit 30, crit gate 50 (no OALCRT anyway), fumble gate 3 (<= 5-0).
	withAttackRolls(t, 30, 50, 3)
	loaded := attackTestWorld(t)
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := attackTestDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "고블린 때려"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "당신은 무기를 떨어뜨렸습니다") || !strings.Contains(out, "고블린에게 0만큼의 피해") {
		t.Fatalf("output = %q, want fumble message and zero damage", out)
	}
	goblin, _ := world.Creature("creature:goblin-1")
	if got, want := goblin.Stats["hpCurrent"], 9; got != want {
		t.Fatalf("goblin hp = %d, want untouched %d", got, want)
	}
	alice, _ := world.Creature("creature:alice")
	if got := alice.Equipment["wield"]; got != "" {
		t.Fatalf("alice wield = %q, want empty after fumble", got)
	}
	sword, ok := world.Object("object:sword")
	if !ok || sword.Location.CreatureID != "creature:alice" || sword.Location.Slot != "inventory" {
		t.Fatalf("sword location = %+v (ok=%v), want alice inventory", sword.Location, ok)
	}
}

func TestAttackHandlerRejectsMissingSelfPlayerAndProtectedTargets(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		want       string
		targetID   model.CreatureID
		wantHP     int
		actorID    string
		activeOnly bool
	}{
		{
			name:     "missing target",
			line:     "때려",
			want:     "누구를 공격하시려구요?",
			targetID: "creature:goblin-1",
			wantHP:   9,
			actorID:  "player:alice",
		},
		{
			name:     "unknown target",
			line:     "허깨비 때려",
			want:     "그런것은 보이지 않습니다.",
			targetID: "creature:goblin-1",
			wantHP:   9,
			actorID:  "player:alice",
		},
		{
			name:     "self target",
			line:     "나 때려",
			want:     "자기 자신은 공격할 수 없습니다.",
			targetID: "creature:goblin-1",
			wantHP:   9,
			actorID:  "player:alice",
		},
		{
			name:     "player target",
			line:     "Bob 때려",
			want:     "당신은 선하다는걸 아세요.",
			targetID: "creature:bob",
			wantHP:   20,
			actorID:  "player:alice",
		},
		{
			name:     "protected creature",
			line:     "수호석 때려",
			want:     "그 상대는 공격할 수 없습니다.",
			targetID: "creature:stone-guardian",
			wantHP:   12,
			actorID:  "player:alice",
		},
		{
			name:     "missing hp stat",
			line:     "상인 때려",
			want:     "그 상대는 공격할 수 없습니다.",
			targetID: "creature:merchant",
			wantHP:   0,
			actorID:  "player:alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := state.NewWorld(attackTestWorld(t))
	defer world.Close()
			dispatcher := attackTestDispatcher(t, world)

			ctx := &Context{ActorID: tt.actorID}
			_, err := dispatcher.DispatchLine(ctx, tt.line)
			if err != nil {
				t.Fatalf("DispatchLine() error = %v", err)
			}
			if got := ctx.OutputString(); !strings.Contains(got, tt.want) {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
			if tt.targetID != "" {
				target, _ := world.Creature(tt.targetID)
				if tt.wantHP == 0 {
					if _, ok := target.Stats["hpCurrent"]; ok {
						t.Fatalf("target stats = %+v, want no hpCurrent", target.Stats)
					}
				} else if target.Stats["hpCurrent"] != tt.wantHP {
					t.Fatalf("%s hp = %d, want %d", tt.targetID, target.Stats["hpCurrent"], tt.wantHP)
				}
			}
		})
	}
}

func attackTestDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()
	return attackTestDispatcherWithHandler(t, NewAttackHandler(world))
}

func attackTestDispatcherWithHandler(t *testing.T, handler Handler) Dispatcher {
	t.Helper()
	return Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "때려", Number: 23, Handler: "attack"},
		}),
		Handlers: map[string]Handler{
			"attack": handler,
		},
	}
}

func attackTestWorld(t *testing.T) *worldload.World {
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
		Stats:       map[string]int{"class": model.ClassFighter, "hpCurrent": 30, "hpMax": 30, "experience": 100, "alignment": 10},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:arena",
		Stats:       map[string]int{"hpCurrent": 20, "hpMax": 20},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:goblin-1",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:arena",
		Stats:       map[string]int{"hpCurrent": 9, "hpMax": 9},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:goblin-2",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:arena",
		Stats:       map[string]int{"hpCurrent": 9, "hpMax": 9},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:mouse",
		Kind:        model.CreatureKindMonster,
		DisplayName: "생쥐",
		RoomID:      "room:arena",
		Stats:       map[string]int{"hpCurrent": 3, "hpMax": 3, "experience": 30, "alignment": 50, "gold": 5},
		Inventory:   model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:cheese"}},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:stone-guardian",
		Kind:        model.CreatureKindMonster,
		DisplayName: "수호석",
		RoomID:      "room:arena",
		Metadata:    model.Metadata{Tags: []string{"unkillable"}},
		Stats:       map[string]int{"hpCurrent": 12, "hpMax": 12},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:arena",
		Stats:       map[string]int{"hpMax": 10},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:sword",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "목검",
		Properties:  map[string]string{"pDice": "4"},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:cheese",
		DisplayName: "치즈",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:sword",
		PrototypeID: "prototype:sword",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "wield"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:cheese",
		PrototypeID: "prototype:cheese",
		Location:    model.ObjectLocation{CreatureID: "creature:mouse", Slot: "inventory"},
	})
	return loaded
}

func withAttackRolls(t *testing.T, rolls ...int) {
	t.Helper()
	previous := attackRoll
	index := 0
	attackRoll = func(min, max int) int {
		if index >= len(rolls) {
			t.Fatalf("attackRoll(%d, %d) called after %d scripted rolls", min, max, len(rolls))
		}
		value := rolls[index]
		index++
		if value < min || value > max {
			t.Fatalf("scripted attack roll %d for range [%d,%d]", value, min, max)
		}
		return value
	}
	t.Cleanup(func() {
		attackRoll = previous
	})
}

func containsCreatureID(ids []model.CreatureID, want model.CreatureID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
