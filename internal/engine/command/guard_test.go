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

func TestGuardHandlerDamagesRemoteMonstersStartsCooldown(t *testing.T) {
	world := state.NewWorld(guardWorld(t, model.ClassRanger, 50))
	handler := NewGuardHandlerWithRoll(world, fixedRoll(1))

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{Args: []string{"동", "Bob"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "Bob") || !strings.Contains(out, "엄호하여 고블린") {
		t.Fatalf("status/output = %d/%q, want remote guard damage", status, out)
	}

	goblin, _ := world.Creature("creature:goblin")
	orc, _ := world.Creature("creature:orc")
	if got, want := creatureStat(goblin, "hpCurrent"), 15; got != want {
		t.Fatalf("goblin hpCurrent = %d, want %d", got, want)
	}
	if got, want := creatureStat(orc, "hpCurrent"), 25; got != want {
		t.Fatalf("orc hpCurrent = %d, want %d", got, want)
	}
	for _, id := range []model.CreatureID{"creature:goblin", "creature:orc"} {
		enemies, err := world.CreatureEnemies(id)
		if err != nil {
			t.Fatalf("CreatureEnemies(%q) error = %v", id, err)
		}
		if !slicesContains(enemies, "Alice") {
			t.Fatalf("%s enemies = %+v, want Alice from guard", id, enemies)
		}
	}
	actor, _ := world.Creature("creature:alice")
	if hasAnyNormalizedFlag(actor.Metadata.Tags, "guarding") {
		t.Fatalf("actor tags = %+v, C guard should not set local guarding status", actor.Metadata.Tags)
	}
	if !guardBroadcastContains(broadcasts, "room:guard", "고블린") || !guardBroadcastContains(broadcasts, "room:east", "고블린") {
		t.Fatalf("broadcasts = %+v, want actor-room and target-room damage broadcasts", broadcasts)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", guardCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active guard cooldown", used, remaining)
	}
}

func TestGuardHandlerFailurePrimesRemoteMonstersAndStartsCooldown(t *testing.T) {
	world := state.NewWorld(guardWorld(t, model.ClassRanger, 50))
	handler := NewGuardHandlerWithRoll(world, fixedRoll(22))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"동", "Bob"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "엄호에 실패") {
		t.Fatalf("status/output = %d/%q, want guard failure", status, ctx.OutputString())
	}
	goblin, _ := world.Creature("creature:goblin")
	if got, want := creatureStat(goblin, "hpCurrent"), 20; got != want {
		t.Fatalf("goblin hpCurrent = %d, want unchanged %d", got, want)
	}
	enemies, err := world.CreatureEnemies("creature:goblin")
	if err != nil {
		t.Fatalf("CreatureEnemies() error = %v", err)
	}
	if !slicesContains(enemies, "Alice") {
		t.Fatalf("goblin enemies = %+v, want Alice even on failed guard", enemies)
	}
	if remaining, used, err := world.UseCreatureCooldown("creature:alice", guardCooldownKey, time.Now().Unix(), 1); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if used || remaining <= 0 {
		t.Fatalf("cooldown used/remaining = %v/%d, want active guard cooldown", used, remaining)
	}
}

func TestGuardHandlerCooldownPrecedesExitLookup(t *testing.T) {
	world := state.NewWorld(guardWorld(t, model.ClassRanger, 50))
	if err := world.SetCreatureCooldown("creature:alice", guardCooldownKey, time.Now().Unix(), guardCooldownSeconds(model.Creature{Stats: map[string]int{"dexterity": 30}})); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewGuardHandlerWithRoll(world, fixedRoll(1))(ctx, ResolvedCommand{Args: []string{"없는길", "Bob"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "기다리세요.") || strings.Contains(out, "엄호할 수") {
		t.Fatalf("status/output = %d/%q, want cooldown before exit lookup", status, out)
	}
}

func TestGuardHandlerInvalidRemoteStatesDoNotStartCooldown(t *testing.T) {
	tests := []struct {
		name   string
		class  int
		level  int
		args   []string
		mutate func(*worldload.World)
		want   string
	}{
		{name: "missing args", class: model.ClassRanger, level: 50, want: "사용법 : 엄호"},
		{name: "wrong class", class: model.ClassFighter, level: 60, args: []string{"동", "Bob"}, want: "포졸 레벨 50 이상만 사용할 수 있는 기술입니다."},
		{name: "ranger below level", class: model.ClassRanger, level: 49, args: []string{"동", "Bob"}, want: "포졸 레벨 50 이상만 사용할 수 있는 기술입니다."},
		{name: "invincible without ranger training", class: model.ClassInvincible, level: 60, args: []string{"동", "Bob"}, want: "포졸을 무적수련하지 않았습니다."},
		{
			name:  "blind",
			class: model.ClassRanger,
			level: 50,
			args:  []string{"동", "Bob"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = append(alice.Metadata.Tags, "PBLIND")
				loaded.Creatures[alice.ID] = alice
			},
			want: "눈이 멀어 엄호할 수 없습니다.",
		},
		{name: "unknown exit", class: model.ClassRanger, level: 50, args: []string{"없는길", "Bob"}, want: "쪽으로는 엄호할 수 없습니다."},
		{
			name:  "closed exit",
			class: model.ClassRanger,
			level: 50,
			args:  []string{"동", "Bob"},
			mutate: func(loaded *worldload.World) {
				room := loaded.Rooms["room:guard"]
				room.Exits[0].Flags = []string{"closed"}
				loaded.Rooms[room.ID] = room
			},
			want: "그 출구는 닫혀 있습니다.",
		},
		{
			name:  "same room exit",
			class: model.ClassRanger,
			level: 50,
			args:  []string{"동", "Bob"},
			mutate: func(loaded *worldload.World) {
				room := loaded.Rooms["room:guard"]
				room.Exits[0].ToRoomID = "room:guard"
				loaded.Rooms[room.ID] = room
			},
			want: "지도가 없습니다.",
		},
		{
			name:  "private destination",
			class: model.ClassRanger,
			level: 50,
			args:  []string{"동", "Bob"},
			mutate: func(loaded *worldload.World) {
				room := loaded.Rooms["room:east"]
				room.Metadata.Tags = []string{"RONMAR"}
				loaded.Rooms[room.ID] = room
			},
			want: "그 방은 볼 수가 없습니다.",
		},
		{name: "missing remote player", class: model.ClassRanger, level: 50, args: []string{"동", "Nobody"}, want: "존재하지 않습니다."},
		{
			name:  "no missile weapon",
			class: model.ClassRanger,
			level: 50,
			args:  []string{"동", "Bob"},
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Equipment = nil
				loaded.Creatures[alice.ID] = alice
			},
			want: "활종류의 무기가 필요합니다.",
		},
		{
			name:  "no remote monsters",
			class: model.ClassRanger,
			level: 50,
			args:  []string{"동", "Bob"},
			mutate: func(loaded *worldload.World) {
				for _, id := range []model.CreatureID{"creature:goblin", "creature:orc"} {
					creature := loaded.Creatures[id]
					creature.Metadata.Tags = append(creature.Metadata.Tags, "MUNKIL")
					loaded.Creatures[id] = creature
				}
			},
			want: "근처에 당신이 공격할 적이 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := guardWorld(t, tt.class, tt.level)
			if tt.mutate != nil {
				tt.mutate(loaded)
			}
			world := state.NewWorld(loaded)
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewGuardHandlerWithRoll(world, fixedRoll(1))(ctx, ResolvedCommand{Args: tt.args, Values: []int64{1, 1}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			if tt.name != "missing args" && tt.name != "wrong class" && tt.name != "ranger below level" && tt.name != "invincible without ranger training" && tt.name != "blind" {
				if remaining, used, err := world.UseCreatureCooldown("creature:alice", guardCooldownKey, time.Now().Unix(), 0); err != nil {
					t.Fatalf("UseCreatureCooldown() error = %v", err)
				} else if !used || remaining != 0 {
					t.Fatalf("cooldown used/remaining = %v/%d, want no cooldown after invalid guard state", used, remaining)
				}
			}
		})
	}
}

func TestGuardHandlerAllowsTrainedInvincible(t *testing.T) {
	loaded := guardWorld(t, model.ClassInvincible, 60)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SRANGER"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewGuardHandlerWithRoll(world, fixedRoll(1))(ctx, ResolvedCommand{Args: []string{"동", "Bob"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "엄호하여 고블린") {
		t.Fatalf("status/output = %d/%q, want trained invincible guard success", status, ctx.OutputString())
	}
}

func TestGuardHandlerCanBeRegisteredByDispatcherAliases(t *testing.T) {
	for _, line := range []string{"동 Bob 엄호", "엄호 동 Bob", "guard 동 Bob"} {
		t.Run(line, func(t *testing.T) {
			world := state.NewWorld(guardWorld(t, model.ClassRanger, 50))
			dispatcher := guardDispatcher(t, world)

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, line)
			if err != nil {
				t.Fatalf("DispatchLine(%q) error = %v", line, err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), "엄호하여 고블린") {
				t.Fatalf("status/output = %d/%q, want dispatch guard success", status, ctx.OutputString())
			}
		})
	}
}

func guardDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()
	return Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "엄호", Number: 165, Handler: "guard"},
			{Name: "guard", Number: 165, Handler: "guard"},
		}),
		Handlers: map[string]Handler{
			"guard": NewGuardHandlerWithRoll(world, fixedRoll(1)),
		},
	}
}

func guardWorld(t *testing.T, class int, level int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:guard",
		DisplayName: "Guard Room",
		PlayerIDs:   []model.PlayerID{"player:alice"},
		Exits: []model.Exit{
			{Name: "동", ToRoomID: "room:east"},
		},
	})
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:east",
		DisplayName: "East Room",
		PlayerIDs:   []model.PlayerID{"player:bob"},
		CreatureIDs: []model.CreatureID{"creature:goblin", "creature:orc"},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:guard",
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:east",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:guard",
		Level:       level,
		Equipment:   map[string]model.ObjectInstanceID{"wield": "object:bow"},
		Stats: map[string]int{
			"class":     class,
			"level":     level,
			"dexterity": 30,
			"strength":  120,
			"thaco":     0,
			"hpCurrent": 30,
			"hpMax":     30,
		},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:east",
		Stats:       map[string]int{"hpCurrent": 20, "hpMax": 20},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:goblin",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:east",
		Stats:       map[string]int{"hpCurrent": 20, "hpMax": 20, "thaco": 20, "armor": 0, "experience": 10},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:orc",
		Kind:        model.CreatureKindMonster,
		DisplayName: "오크",
		RoomID:      "room:east",
		Stats:       map[string]int{"hpCurrent": 30, "hpMax": 30, "thaco": 20, "armor": 0, "experience": 20},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:bow",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "목궁",
		Properties:  map[string]string{"type": "4", "pDice": "4"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:bow",
		PrototypeID: "prototype:bow",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "wield"},
	})
	return loaded
}

func guardBroadcastContains(broadcasts []roomBroadcastRecord, roomID model.RoomID, text string) bool {
	for _, broadcast := range broadcasts {
		if broadcast.RoomID == string(roomID) && strings.Contains(broadcast.Text, text) {
			return true
		}
	}
	return false
}
