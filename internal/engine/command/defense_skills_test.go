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

func TestReflectHandlerSuccessAddsStatusCooldownAndExpiration(t *testing.T) {
	runtime := state.NewWorld(reflectWorld(t, model.ClassFighter, 50))
	handler := NewReflectHandler(runtime, fixedRoll(1))
	var broadcasts []roomBroadcastRecord

	before := time.Now().Unix()
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "반탄강기") {
		t.Fatalf("status/output = %d/%q, want reflect success", status, ctx.OutputString())
	}

	updated, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "PREFLECT") ||
		!hasAnyNormalizedFlag(updated.Metadata.Tags, "reflect") {
		t.Fatalf("creature tags = %+v, want reflect tags", updated.Metadata.Tags)
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if !hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "PREFLECT") ||
		!hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "reflect") {
		t.Fatalf("player tags = %+v, want reflect tags", updatedPlayer.Metadata.Tags)
	}
	if expires, ok := runtime.GetEffectExpiration("creature:alice", "PREFLECT"); !ok {
		t.Fatal("PREFLECT effect expiration was not set")
	} else if expires < before+reflectStatusDurationSeconds || expires > time.Now().Unix()+reflectStatusDurationSeconds {
		t.Fatalf("PREFLECT expiration = %d, want about now+%d", expires, reflectStatusDurationSeconds)
	}
	if remaining, used, err := runtime.UseCreatureCooldown("creature:alice", reflectCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 || remaining > reflectSuccessCooldownSeconds {
		t.Fatalf("cooldown used/remaining = %v/%d, want active cooldown", used, remaining)
	}
	if len(broadcasts) != 1 || !strings.Contains(broadcasts[0].Text, "반탄강기를 사용합니다") {
		t.Fatalf("broadcasts = %+v, want reflect broadcast", broadcasts)
	}
}

func TestReflectHandlerRejectsInvalidStates(t *testing.T) {
	tests := []struct {
		name   string
		class  int
		level  int
		tags   []string
		setup  func(*state.World)
		want   string
		active bool
	}{
		{
			name:  "wrong class",
			class: model.ClassAssassin,
			level: 50,
			want:  "검사 레벨 50이상만 쓸수 있는 기술입니다.",
		},
		{
			name:  "fighter below level fifty",
			class: model.ClassFighter,
			level: 49,
			want:  "검사 레벨 50이상만 쓸수 있는 기술입니다.",
		},
		{
			name:  "invincible without fighter training",
			class: model.ClassInvincible,
			level: 60,
			want:  "검사를 무적수련하지 않았습니다..",
		},
		{
			name:  "already reflecting",
			class: model.ClassFighter,
			level: 50,
			tags:  []string{"PREFLECT"},
			setup: func(world *state.World) {
				if err := world.SetCreatureCooldown("creature:alice", reflectCooldownKey, time.Now().Unix(), reflectSuccessCooldownSeconds); err != nil {
					t.Fatalf("SetCreatureCooldown() error = %v", err)
				}
			},
			want:   "이미 반탄강기를 사용중입니다.",
			active: true,
		},
		{
			name:  "cooldown active",
			class: model.ClassFighter,
			level: 50,
			setup: func(world *state.World) {
				if err := world.SetCreatureCooldown("creature:alice", reflectCooldownKey, time.Now().Unix(), reflectSuccessCooldownSeconds); err != nil {
					t.Fatalf("SetCreatureCooldown() error = %v", err)
				}
			},
			want: "기다리세요.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := reflectWorld(t, tt.class, tt.level)
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = tt.tags
			loaded.Creatures[alice.ID] = alice
			runtime := state.NewWorld(loaded)
			if tt.setup != nil {
				tt.setup(runtime)
			}
			handler := NewReflectHandler(runtime, fixedRoll(1))

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			updated, _ := runtime.Creature("creature:alice")
			if got := hasAnyNormalizedFlag(updated.Metadata.Tags, "PREFLECT", "reflect"); got != tt.active {
				t.Fatalf("active reflect tags = %v, want %v; tags=%+v", got, tt.active, updated.Metadata.Tags)
			}
		})
	}
}

func TestReflectHandlerFailureSetsShortCooldownWithoutStatus(t *testing.T) {
	runtime := state.NewWorld(reflectWorld(t, model.ClassFighter, 50))
	handler := NewReflectHandler(runtime, fixedRoll(100))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "실패했습니다") {
		t.Fatalf("status/output = %d/%q, want reflect failure", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "PREFLECT", "reflect") {
		t.Fatalf("creature tags = %+v, want no reflect status", updated.Metadata.Tags)
	}
	if _, ok := runtime.GetEffectExpiration("creature:alice", "PREFLECT"); ok {
		t.Fatal("PREFLECT effect expiration set on failed reflect")
	}
	if remaining, used, err := runtime.UseCreatureCooldown("creature:alice", reflectCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining < 1 || remaining > reflectFailureCooldownSeconds {
		t.Fatalf("failure cooldown used/remaining = %v/%d, want short active cooldown", used, remaining)
	}
}

func TestReflectHandlerInvincibleWithFighterTrainingCanUse(t *testing.T) {
	loaded := reflectWorld(t, model.ClassInvincible, 60)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SFIGHTER"}
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReflectHandler(runtime, fixedRoll(1))(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "반탄강기") {
		t.Fatalf("status/output = %d/%q, want trained invincible success", status, ctx.OutputString())
	}
}

func TestShadowHandlerSuccessDamagesTargetAndStartsCooldown(t *testing.T) {
	loaded := shadowWorld(t, model.ClassAssassin, 50, true)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	runtime := state.NewWorld(loaded)
	var broadcasts []roomBroadcastRecord

	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := NewShadowHandler(runtime, fixedRoll(1))(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "분신술") {
		t.Fatalf("status/output = %d/%q, want shadow success", status, ctx.OutputString())
	}

	updated, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS") ||
		updated.Stats["PHIDDN"] != 0 || updated.Stats["PINVIS"] != 0 {
		t.Fatalf("creature tags/stats = %+v/%+v, want hidden/invisible cleared", updated.Metadata.Tags, updated.Stats)
	}
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "shadow", "shadowClone", "PSHADOW") {
		t.Fatalf("creature tags = %+v, want no non-C shadow status marker", updated.Metadata.Tags)
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS") {
		t.Fatalf("player tags = %+v, want hidden/invisible cleared", updatedPlayer.Metadata.Tags)
	}
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "shadow", "shadowClone", "PSHADOW") {
		t.Fatalf("player tags = %+v, want no non-C shadow status marker", updatedPlayer.Metadata.Tags)
	}
	if remaining, used, err := runtime.UseCreatureCooldown("creature:alice", shadowCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active cooldown", used, remaining)
	}
	goblin, _ := runtime.Creature("creature:goblin")
	if got, want := goblin.Stats["hpCurrent"], 22; got != want {
		t.Fatalf("goblin hpCurrent = %d, want shadow damage result %d", got, want)
	}
	if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "was_attacked") {
		t.Fatalf("goblin tags = %+v, want was_attacked after shadow", goblin.Metadata.Tags)
	}
	damageBroadcasts := 0
	for _, broadcast := range broadcasts {
		if strings.Contains(broadcast.Text, "분신술로") {
			damageBroadcasts++
		}
	}
	if damageBroadcasts != shadowNormalCloneCount {
		t.Fatalf("broadcasts = %+v, want shadow damage broadcasts", broadcasts)
	}
	if !strings.Contains(ctx.OutputString(), "6연타 18점") {
		t.Fatalf("output = %q, want shadow total damage", ctx.OutputString())
	}
}

func TestShadowHandlerRejectsInvalidStates(t *testing.T) {
	tests := []struct {
		name       string
		class      int
		level      int
		tags       []string
		args       []string
		withWeapon bool
		setup      func(*state.World)
		want       string
	}{
		{
			name:       "missing target",
			class:      model.ClassAssassin,
			level:      50,
			withWeapon: true,
			want:       "누굴 공격합니까?",
		},
		{
			name:       "blind",
			class:      model.ClassAssassin,
			level:      50,
			tags:       []string{"PBLIND"},
			args:       []string{"고블린"},
			withWeapon: true,
			want:       "누굴 공격합니까?",
		},
		{
			name:       "wrong class",
			class:      model.ClassFighter,
			level:      50,
			args:       []string{"고블린"},
			withWeapon: true,
			want:       "자객 레벨 50이상만 쓸수 있는 기술입니다.",
		},
		{
			name:       "assassin below level fifty",
			class:      model.ClassAssassin,
			level:      49,
			args:       []string{"고블린"},
			withWeapon: true,
			want:       "자객 레벨 50이상만 쓸수 있는 기술입니다.",
		},
		{
			name:       "invincible without assassin training",
			class:      model.ClassInvincible,
			level:      60,
			args:       []string{"고블린"},
			withWeapon: true,
			want:       "자객을 무적수련하지 않았습니다..",
		},
		{
			name:  "missing sharp weapon",
			class: model.ClassAssassin,
			level: 50,
			args:  []string{"고블린"},
			want:  "날카로운 무기가 필요합니다.",
		},
		{
			name:       "missing target creature",
			class:      model.ClassAssassin,
			level:      50,
			args:       []string{"없는"},
			withWeapon: true,
			want:       "그런 것은 여기 없습니다.",
		},
		{
			name:       "cooldown active",
			class:      model.ClassAssassin,
			level:      50,
			args:       []string{"고블린"},
			withWeapon: true,
			setup: func(world *state.World) {
				if err := world.SetCreatureCooldown("creature:alice", shadowCooldownKey, time.Now().Unix(), shadowSuccessCooldownSeconds(model.Creature{Stats: map[string]int{"dexterity": 40}})); err != nil {
					t.Fatalf("SetCreatureCooldown() error = %v", err)
				}
			},
			want: "기다리세요.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := shadowWorld(t, tt.class, tt.level, tt.withWeapon)
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = tt.tags
			loaded.Creatures[alice.ID] = alice
			runtime := state.NewWorld(loaded)
			if tt.setup != nil {
				tt.setup(runtime)
			}
			handler := NewShadowHandler(runtime, fixedRoll(1))

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

func TestShadowHandlerCooldownPrecedesWeaponTargetAndReveal(t *testing.T) {
	loaded := shadowWorld(t, model.ClassAssassin, 50, false)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden", "PHIDDN", "invisible", "PINVIS")
	alice.Stats["PHIDDN"] = 1
	alice.Stats["PINVIS"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Players[player.ID] = player
	runtime := state.NewWorld(loaded)
	if err := runtime.SetCreatureCooldown("creature:alice", shadowCooldownKey, time.Now().Unix(), shadowSuccessCooldownSeconds(alice)); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewShadowHandler(runtime, fixedRoll(1))(ctx, ResolvedCommand{Args: []string{"없는"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "기다리세요.") {
		t.Fatalf("status/output = %d/%q, want cooldown wait", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "PHIDDN") ||
		!hasAnyNormalizedFlag(updated.Metadata.Tags, "invisible", "PINVIS") ||
		updated.Stats["PHIDDN"] != 1 || updated.Stats["PINVIS"] != 1 {
		t.Fatalf("alice tags/stats = %+v/%+v, want hidden/invisible retained during cooldown", updated.Metadata.Tags, updated.Stats)
	}
}

func TestShadowHandlerFailureSetsShortCooldownWithoutMarkers(t *testing.T) {
	runtime := state.NewWorld(shadowWorld(t, model.ClassAssassin, 50, true))
	handler := NewShadowHandler(runtime, fixedRoll(100))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "분신술에 실패했습니다") {
		t.Fatalf("status/output = %d/%q, want shadow failure", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "shadow", "shadowClone", "PSHADOW") {
		t.Fatalf("creature tags = %+v, want no shadow status", updated.Metadata.Tags)
	}
	if remaining, used, err := runtime.UseCreatureCooldown("creature:alice", shadowCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining < 1 || remaining > shadowFailureCooldownSeconds(updated) {
		t.Fatalf("failure cooldown used/remaining = %v/%d, want short active cooldown", used, remaining)
	}
	goblin, _ := runtime.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 40; got != want {
		t.Fatalf("goblin hp = %d, want unchanged %d", got, want)
	}
	if !hasAnyNormalizedFlag(goblin.Metadata.Tags, "was_attacked") {
		t.Fatalf("goblin tags = %+v, want was_attacked after failed shadow", goblin.Metadata.Tags)
	}
}

func TestShadowChanceMatchesLegacyLevelDeltaFormula(t *testing.T) {
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

	if got, want := shadowChance(actor, victim), 38; got != want {
		t.Fatalf("shadowChance() = %d, want C level-delta formula result %d", got, want)
	}
}

func TestShadowHandlerWeaponBreakOnSuccessDoesNotStartCooldown(t *testing.T) {
	loaded := shadowWorld(t, model.ClassAssassin, 50, true)
	blade := loaded.Objects["object:blade"]
	blade.Properties["shotsCurrent"] = "1"
	loaded.Objects[blade.ID] = blade
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewShadowHandler(runtime, fixedRoll(1))(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "부서져 버렸습니다.") {
		t.Fatalf("status/output = %d/%q, want weapon break", status, ctx.OutputString())
	}
	goblin, _ := runtime.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 40; got != want {
		t.Fatalf("goblin hp = %d, want unchanged after weapon break %d", got, want)
	}
	if remaining, used, err := runtime.UseCreatureCooldown("creature:alice", shadowCooldownKey, time.Now().Unix(), 0); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !used || remaining != 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want no cooldown after break-return", used, remaining)
	}
}

func TestShadowHandlerFinalizesMonsterDeath(t *testing.T) {
	loaded := shadowWorld(t, model.ClassAssassin, 50, true)
	goblin := loaded.Creatures["creature:goblin"]
	goblin.Stats["hpCurrent"] = 3
	goblin.Stats["hpMax"] = 3
	loaded.Creatures[goblin.ID] = goblin
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewShadowHandler(runtime, fixedRoll(1))(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "죽였습니다") {
		t.Fatalf("status/output = %d/%q, want shadow death", status, ctx.OutputString())
	}
	if _, ok := runtime.Creature("creature:goblin"); ok {
		t.Fatal("dead shadow target still exists in world")
	}
	shadow, _ := runtime.Room("room:shadow")
	if containsCreatureID(shadow.CreatureIDs, "creature:goblin") {
		t.Fatalf("shadow room creatures = %+v, want goblin removed", shadow.CreatureIDs)
	}
}

func TestShadowHandlerInvincibleWithAssassinTrainingCanUse(t *testing.T) {
	loaded := shadowWorld(t, model.ClassInvincible, 60, true)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SASSASSIN"}
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewShadowHandler(runtime, fixedRoll(1))(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "분신술") {
		t.Fatalf("status/output = %d/%q, want trained invincible success", status, ctx.OutputString())
	}
	goblin, _ := runtime.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 16; got != want {
		t.Fatalf("goblin hp = %d, want %d after invincible shadow", got, want)
	}
}

func TestDefenseSkillHandlersCanBeRegisteredByDispatcherAliases(t *testing.T) {
	for _, tt := range []struct {
		name  string
		line  string
		world *state.World
		want  string
	}{
		{
			name:  "reflect Korean",
			line:  "반탄강기",
			world: state.NewWorld(reflectWorld(t, model.ClassFighter, 50)),
			want:  "반탄강기",
		},
		{
			name:  "shadow Korean",
			line:  "고블린 분신술",
			world: state.NewWorld(shadowWorld(t, model.ClassAssassin, 50, true)),
			want:  "분신술",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
				{Name: "반탄강기", Number: 170, Handler: "reflect"},
				{Name: "reflect", Number: 170, Handler: "reflect"},
				{Name: "분신술", Number: 171, Handler: "shadow"},
				{Name: "shadow", Number: 171, Handler: "shadow"},
			})
			if err != nil {
				t.Fatal(err)
			}
			dispatcher := Dispatcher{
				Registry: registry,
				Handlers: map[string]Handler{
					"reflect": NewReflectHandler(tt.world, fixedRoll(1)),
					"shadow":  NewShadowHandler(tt.world, fixedRoll(1)),
				},
			}

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

func reflectWorld(t *testing.T, class int, level int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{ID: "room:defense", DisplayName: "Defense"})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:defense",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:defense",
		Level:       level,
		Stats: map[string]int{
			"class": class,
			"level": level,
			"thaco": 0,
		},
	})
	return loaded
}

func shadowWorld(t *testing.T, class int, level int, withWeapon bool) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{ID: "room:shadow", DisplayName: "Shadow"})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:shadow",
	})
	equipment := map[string]model.ObjectInstanceID{}
	if withWeapon {
		equipment["wield"] = "object:blade"
		mustAddLookPrototype(t, loaded, model.ObjectPrototype{
			ID:          "prototype:blade",
			Kind:        model.ObjectKindWeapon,
			DisplayName: "날카로운 검",
		})
	}
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:shadow",
		Level:       level,
		Equipment:   equipment,
		Stats: map[string]int{
			"class":        class,
			"level":        level,
			"intelligence": 30,
			"dexterity":    40,
			"thaco":        0,
			"hpCurrent":    80,
			"hpMax":        80,
			"pDice":        2,
		},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:goblin",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:shadow",
		Stats: map[string]int{
			"class":     model.ClassFighter,
			"level":     1,
			"armor":     0,
			"hpCurrent": 40,
			"hpMax":     40,
		},
	})
	if withWeapon {
		mustAddLookObject(t, loaded, model.ObjectInstance{
			ID:          "object:blade",
			PrototypeID: "prototype:blade",
			Quantity:    1,
			Location: model.ObjectLocation{
				CreatureID: "creature:alice",
				Slot:       "wield",
			},
			Properties: map[string]string{"type": "0", "pDice": "4"},
		})
	}
	return loaded
}
