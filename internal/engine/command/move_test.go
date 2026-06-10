package command

import (
	"errors"
	"strings"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestMoveHandlerDispatchesLegacyGoAndRendersDestination(t *testing.T) {
	world := state.NewWorld(lookWorld(t))
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "가", Number: 30, Handler: "go"},
		}),
		Handlers: map[string]Handler{
			"go": NewMoveHandler(world),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "동 가")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	player, ok := world.Player("player:alice")
	if !ok {
		t.Fatal("missing player")
	}
	if player.RoomID != "room:east" {
		t.Fatalf("player room id = %q, want room:east", player.RoomID)
	}

	out := ctx.OutputString()
	for _, want := range []string{
		"\n동쪽\n\n",
		"[ 출구 : 없음 ]\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "광장") {
		t.Fatalf("output rendered origin room:\n%s", out)
	}
}

func TestMoveHandlerRequiresDirection(t *testing.T) {
	world := state.NewWorld(lookWorld(t))
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "가", Number: 30, Handler: "go"},
		}),
		Handlers: map[string]Handler{
			"go": NewMoveHandler(world),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	_, err := dispatcher.DispatchLine(ctx, "가")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if ctx.OutputString() != "어디로 가고 싶으세요?\n" {
		t.Fatalf("output = %q, want missing direction message", ctx.OutputString())
	}

	player, ok := world.Player("player:alice")
	if !ok {
		t.Fatal("missing player")
	}
	if player.RoomID != "room:plaza" {
		t.Fatalf("player room id = %q, want room:plaza", player.RoomID)
	}
}

func TestMoveHandlerUsesGoPrefixAndOrdinal(t *testing.T) {
	world := state.NewWorld(goOrdinalWorld(t))
	handler := NewMoveHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec:   commandspec.CommandSpec{Handler: "go"},
		Args:   []string{"동"},
		Values: []int64{2},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	player, ok := world.Player("player:alice")
	if !ok {
		t.Fatal("missing player")
	}
	if player.RoomID != "room:east" {
		t.Fatalf("player room id = %q, want room:east", player.RoomID)
	}
}

func TestMoveHandlerDispatchesDirectMoveDirection(t *testing.T) {
	world, out := dispatchMoveLine(t, lookWorld(t), "동")

	assertMovePlayerRoom(t, world, "room:east")
	if !strings.Contains(out, "\n동쪽\n\n") {
		t.Fatalf("output missing destination room:\n%s", out)
	}
}

func TestMoveHandlerRecordsLegacyRoomTrackOnOrigin(t *testing.T) {
	world, _ := dispatchMoveLine(t, lookWorld(t), "동")

	room, ok := world.Room("room:plaza")
	if !ok {
		t.Fatal("origin room missing")
	}
	if got := room.Properties["track"]; got != "동" {
		t.Fatalf("origin track = %q, want 동", got)
	}
}

func TestMoveHandlerMovesOwnedDMFollowerMonsterLikeLegacy(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "direct move",
			line: "동",
			want: "\n나무이 동쪽으로 갔습니다.",
		},
		{
			name: "go",
			line: "동 가",
			want: "\n나무가 방황하다 동쪽으로 갔습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithEastExit(t, nil, "room:east")
			addMoveDMFollower(t, loaded, "creature:follower", "나무", map[string]string{
				dmFollowLeaderProperty:         "player:alice",
				dmFollowLeaderCreatureProperty: "creature:alice",
			})
			world := state.NewWorld(loaded)
			var broadcasts []roomBroadcastRecord
			ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)

			dispatchMoveLineWithContext(t, world, ctx, tt.line)

			assertMoveWorldPlayerRoom(t, world, "room:east")
			follower, ok := world.Creature("creature:follower")
			if !ok {
				t.Fatal("missing follower")
			}
			if follower.RoomID != "room:east" {
				t.Fatalf("follower room = %q, want room:east", follower.RoomID)
			}
			if len(broadcasts) != 1 ||
				broadcasts[0].RoomID != "room:plaza" ||
				broadcasts[0].Exclude != "session:alice" ||
				broadcasts[0].Text != tt.want {
				t.Fatalf("broadcasts = %+v, want one origin follower depart %q", broadcasts, tt.want)
			}
		})
	}
}

func TestMoveHandlerSkipsOtherDMFollowerMonsterLikeLegacy(t *testing.T) {
	loaded := moveWorldWithEastExit(t, nil, "room:east")
	addMoveDMFollower(t, loaded, "creature:follower", "나무", map[string]string{
		dmFollowLeaderProperty:         "player:other",
		dmFollowLeaderCreatureProperty: "creature:other",
	})
	world := state.NewWorld(loaded)
	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)

	dispatchMoveLineWithContext(t, world, ctx, "동")

	assertMoveWorldPlayerRoom(t, world, "room:east")
	follower, ok := world.Creature("creature:follower")
	if !ok {
		t.Fatal("missing follower")
	}
	if follower.RoomID != "room:plaza" {
		t.Fatalf("follower room = %q, want room:plaza", follower.RoomID)
	}
	if len(broadcasts) != 0 {
		t.Fatalf("broadcasts = %+v, want none for another DM follower", broadcasts)
	}
}

func TestMoveHandlerPreservesPermanentTrackRoom(t *testing.T) {
	loaded := lookWorld(t)
	room := loaded.Rooms["room:plaza"]
	room.Metadata.Tags = append(room.Metadata.Tags, "permanentTracks")
	room.Properties = map[string]string{"track": "서"}
	loaded.Rooms[room.ID] = room

	world, _ := dispatchMoveLine(t, loaded, "동")

	room, ok := world.Room("room:plaza")
	if !ok {
		t.Fatal("origin room missing")
	}
	if got := room.Properties["track"]; got != "서" {
		t.Fatalf("origin track = %q, want preserved 서", got)
	}
}

func TestMoveHandlerDispatchesDirectMoveAlias(t *testing.T) {
	world := state.NewWorld(lookWorld(t))
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "6", Number: 1, Handler: "move"},
		}),
		Handlers: map[string]Handler{
			"move": NewMoveHandler(world),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "6"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}

	player, ok := world.Player("player:alice")
	if !ok {
		t.Fatal("missing player")
	}
	if player.RoomID != "room:east" {
		t.Fatalf("player room id = %q, want room:east", player.RoomID)
	}
}

func TestMoveHandlerBlocksClosedExit(t *testing.T) {
	loaded := moveWorldWithEastExit(t, []string{"closed"}, "room:missing")
	world, out := dispatchMoveLine(t, loaded, "동")

	if out != "문이 닫혀 있습니다.\n" {
		t.Fatalf("output = %q, want closed-exit message", out)
	}
	assertMovePlayerRoom(t, world, "room:plaza")
}

func TestMoveHandlerBlocksLockedExit(t *testing.T) {
	loaded := moveWorldWithEastExit(t, []string{"locked"}, "room:missing")
	world, out := dispatchMoveLine(t, loaded, "동 가")

	if out != "그 출구는 잠겨 있습니다.\n" {
		t.Fatalf("output = %q, want locked-exit message", out)
	}
	assertMovePlayerRoom(t, world, "room:plaza")
}

func TestMoveHandlerBlocksCombatBeforeExitValidationLikeLegacy(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		flags []string
	}{
		{name: "direct move closed exit", line: "동", flags: []string{"closed"}},
		{name: "go locked exit", line: "동 가", flags: []string{"locked"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithEastExit(t, tt.flags, "room:missing")
			world := state.NewWorld(loaded)
			if _, err := world.AddEnemy("creature:guard", "creature:alice"); err != nil {
				t.Fatalf("AddEnemy() error = %v", err)
			}
			out := dispatchMoveLineWithMoveWorld(t, world, tt.line)

			if out != "싸우는 중에는 이동할 수 없습니다." {
				t.Fatalf("output = %q, want legacy combat movement block", out)
			}
			assertMovePlayerRoom(t, world, "room:plaza")
		})
	}
}

func TestMoveHandlerBlocksSilencedActorBeforeCombatLikeLegacy(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "direct move", line: "동", want: "당신은 움직일수 없습니다."},
		{name: "go", line: "동 가", want: "당신은 움직일 수가 없습니다."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithEastExit(t, []string{"locked"}, "room:missing")
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = append(alice.Metadata.Tags, "PSILNC")
			loaded.Creatures[alice.ID] = alice
			world := state.NewWorld(loaded)
			if _, err := world.AddEnemy("creature:guard", "creature:alice"); err != nil {
				t.Fatalf("AddEnemy() error = %v", err)
			}
			out := dispatchMoveLineWithMoveWorld(t, world, tt.line)

			if out != tt.want {
				t.Fatalf("output = %q, want %q", out, tt.want)
			}
			assertMovePlayerRoom(t, world, "room:plaza")
		})
	}
}

func TestMoveHandlerUsesDirectMoveMessagesForLockedExit(t *testing.T) {
	loaded := moveWorldWithEastExit(t, []string{"locked"}, "room:missing")
	world, out := dispatchMoveLine(t, loaded, "동")

	if out != "문이 잠겨 있습니다.\n" {
		t.Fatalf("output = %q, want direct locked-exit message", out)
	}
	assertMovePlayerRoom(t, world, "room:plaza")
}

func TestMoveHandlerDoesNotMatchNoSeeOrGoInvisibleExits(t *testing.T) {
	tests := []struct {
		name string
		line string
		flag string
		want string
	}{
		{
			name: "direct noSee",
			line: "동",
			flag: "noSee",
			want: "길이 막혀 있습니다.\n",
		},
		{
			name: "go invisible",
			line: "동 가",
			flag: "invisible",
			want: "그런 출구는 없습니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithEastExit(t, []string{tt.flag}, "room:east")
			world, out := dispatchMoveLine(t, loaded, tt.line)

			if out != tt.want {
				t.Fatalf("output = %q, want %q", out, tt.want)
			}
			assertMovePlayerRoom(t, world, "room:plaza")
		})
	}
}

func TestMoveHandlerDirectMoveCanUseInvisibleExit(t *testing.T) {
	loaded := moveWorldWithEastExit(t, []string{"invisible"}, "room:east")
	world, out := dispatchMoveLine(t, loaded, "동")

	assertMovePlayerRoom(t, world, "room:east")
	if !strings.Contains(out, "\n동쪽\n\n") {
		t.Fatalf("output missing destination room:\n%s", out)
	}
}

func TestMoveHandlerBlocksNakedExitWhenCarryingObjects(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		flag        string
		objectID    model.ObjectInstanceID
		slot        string
		inInventory bool
		inEquipment bool
		wantMessage string
		wantRoomID  model.RoomID
	}{
		{
			name:        "direct inventory",
			line:        "동",
			flag:        "XNAKED",
			objectID:    "object:carried-inventory",
			slot:        "inventory",
			inInventory: true,
			wantMessage: "뭘 가지고는 들어갈수 없습니다.\n",
			wantRoomID:  "room:plaza",
		},
		{
			name:        "go equipment",
			line:        "동 가",
			flag:        "naked",
			objectID:    "object:carried-equipment",
			slot:        "right",
			inEquipment: true,
			wantMessage: "뭘 가지고는 들어갈 수 없습니다.\n",
			wantRoomID:  "room:plaza",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithEastExit(t, []string{tt.flag}, "room:east")
			addMoveCreatureObjectRef(t, loaded, tt.objectID, tt.slot, tt.inInventory, tt.inEquipment)

			world, out := dispatchMoveLine(t, loaded, tt.line)

			if out != tt.wantMessage {
				t.Fatalf("output = %q, want %q", out, tt.wantMessage)
			}
			assertMovePlayerRoom(t, world, tt.wantRoomID)
		})
	}
}

func TestMoveHandlerAllowsNakedExitWithEmptyInventory(t *testing.T) {
	loaded := moveWorldWithEastExit(t, []string{"XNAKED"}, "room:east")
	world, out := dispatchMoveLine(t, loaded, "동")

	assertMovePlayerRoom(t, world, "room:east")
	if !strings.Contains(out, "\n동쪽\n\n") {
		t.Fatalf("output missing destination room:\n%s", out)
	}
}

func TestMoveHandlerBlocksLegacySpecialExitFlags(t *testing.T) {
	tests := []struct {
		name   string
		flag   string
		mutate func(*worldload.World)
		want   string
	}{
		{
			name: "fly required",
			flag: "XFLYSP",
			want: "그 쪽으로는 날아서 가야 될것 같군요.\n",
		},
		{
			name: "female only",
			flag: "XFEMAL",
			mutate: func(loaded *worldload.World) {
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = append(alice.Metadata.Tags, "PMALES")
				loaded.Creatures[alice.ID] = alice
			},
			want: "여성만 들어갈수 있습니다. 여탕인가~~\n",
		},
		{
			name: "male only",
			flag: "XMALES",
			want: "남성만 들어갈수 있습니다.\n",
		},
		{
			name: "passive guard",
			flag: "XPGUAR",
			mutate: func(loaded *worldload.World) {
				mustAddLookCreature(t, loaded, model.Creature{
					ID:          "creature:passive-guard",
					Kind:        model.CreatureKindMonster,
					DisplayName: "고블린",
					RoomID:      "room:plaza",
					Metadata:    model.Metadata{Tags: []string{"MPGUAR"}},
				})
			},
			want: "고블린이 당신의 길을 막습니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithEastExit(t, []string{tt.flag}, "room:east")
			if tt.mutate != nil {
				tt.mutate(loaded)
			}

			world, out := dispatchMoveLine(t, loaded, "동")

			if out != tt.want {
				t.Fatalf("output = %q, want %q", out, tt.want)
			}
			assertMovePlayerRoom(t, world, "room:plaza")
		})
	}
}

func TestMoveHandlerBlocksLegacyTimeRestrictedExits(t *testing.T) {
	tests := []struct {
		name       string
		flag       string
		legacyTime int64
		want       string
	}{
		{
			name:       "night only during day",
			flag:       "XNGHTO",
			legacyTime: 12,
			want:       "그 출구는 밤에만 열려 있습니다.\n",
		},
		{
			name:       "day only during night",
			flag:       "XDAYON",
			legacyTime: 23,
			want:       "그 출구는 밤에는 닫혀 있습니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithEastExit(t, []string{tt.flag}, "room:east")
			world := state.NewWorld(loaded)
			world.SetLegacyTime(tt.legacyTime)

			out := dispatchMoveLineWithMoveWorld(t, world, "동")

			if out != tt.want {
				t.Fatalf("output = %q, want %q", out, tt.want)
			}
			assertMoveWorldPlayerRoom(t, world, "room:plaza")
		})
	}
}

func TestMoveHandlerAllowsLegacyTimeRestrictedExitsAtValidHour(t *testing.T) {
	tests := []struct {
		name       string
		flag       string
		legacyTime int64
	}{
		{name: "night only at night", flag: "XNGHTO", legacyTime: 22},
		{name: "day only at day boundary", flag: "XDAYON", legacyTime: 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithEastExit(t, []string{tt.flag}, "room:east")
			world := state.NewWorld(loaded)
			world.SetLegacyTime(tt.legacyTime)

			out := dispatchMoveLineWithMoveWorld(t, world, "동")

			assertMoveWorldPlayerRoom(t, world, "room:east")
			if !strings.Contains(out, "\n동쪽\n\n") {
				t.Fatalf("output missing destination room:\n%s", out)
			}
		})
	}
}

func TestMoveHandlerClimbFallDamagesAndStopsLikeLegacy(t *testing.T) {
	withMoveTrapRolls(t, 1, 7)
	loaded := moveWorldWithEastExit(t, []string{"XCLIMB"}, "room:east")
	setMoveTrapActorStats(t, loaded, map[string]int{
		"dexterity": 0,
		"hpCurrent": 20,
		"hpMax":     20,
	})

	world, out := dispatchMoveLine(t, loaded, "동")

	assertMovePlayerRoom(t, world, "room:plaza")
	actor, _ := world.Creature("creature:alice")
	if got := actor.Stats["hpCurrent"]; got != 13 {
		t.Fatalf("hpCurrent = %d, want 13", got)
	}
	if out != "당신은 구덩이에 떨어져서 7 만큼의 상처를 입었습니다" {
		t.Fatalf("output = %q, want climb fall damage only", out)
	}
}

func TestMoveHandlerRepelFallDamagesAndContinuesLikeLegacy(t *testing.T) {
	withMoveTrapRolls(t, 1, 7)
	loaded := moveWorldWithEastExit(t, []string{"XREPEL"}, "room:east")
	setMoveTrapActorStats(t, loaded, map[string]int{
		"dexterity": 0,
		"hpCurrent": 20,
		"hpMax":     20,
	})

	world, out := dispatchMoveLine(t, loaded, "동")

	assertMovePlayerRoom(t, world, "room:east")
	actor, _ := world.Creature("creature:alice")
	if got := actor.Stats["hpCurrent"]; got != 13 {
		t.Fatalf("hpCurrent = %d, want 13", got)
	}
	assertMoveTrapOutputOrder(t, out,
		"당신은 구덩이에 떨어져서 7 만큼의 상처를 입었습니다",
		"\n동쪽\n\n",
	)
}

func TestMoveHandlerClimbFallSkippedWithLevitate(t *testing.T) {
	loaded := moveWorldWithEastExit(t, []string{"XCLIMB"}, "room:east")
	setMoveTrapActorStats(t, loaded, map[string]int{
		"dexterity": 0,
		"hpCurrent": 20,
		"hpMax":     20,
	})
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PLEVIT"}
	loaded.Creatures[alice.ID] = alice

	world, out := dispatchMoveLine(t, loaded, "동")

	assertMovePlayerRoom(t, world, "room:east")
	actor, _ := world.Creature("creature:alice")
	if got := actor.Stats["hpCurrent"]; got != 20 {
		t.Fatalf("hpCurrent = %d, want unchanged 20", got)
	}
	if !strings.Contains(out, "\n동쪽\n\n") {
		t.Fatalf("output missing destination room:\n%s", out)
	}
}

func TestMoveHandlerClimbGearReducesFallChanceLikeLegacy(t *testing.T) {
	withMoveTrapRolls(t, 1)
	loaded := moveWorldWithEastExit(t, []string{"XCLIMB"}, "room:east")
	setMoveTrapActorStats(t, loaded, map[string]int{
		"dexterity": 0,
		"hpCurrent": 20,
		"hpMax":     20,
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:climb-rope",
		PrototypeID: "prototype:coin",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "held"},
		Properties:  map[string]string{"pDice": "30"},
		Metadata:    model.Metadata{Tags: []string{"OCLIMB"}},
	})
	alice := loaded.Creatures["creature:alice"]
	alice.Equipment = map[string]model.ObjectInstanceID{"held": "object:climb-rope"}
	loaded.Creatures[alice.ID] = alice

	world, out := dispatchMoveLine(t, loaded, "동")

	assertMovePlayerRoom(t, world, "room:east")
	actor, _ := world.Creature("creature:alice")
	if got := actor.Stats["hpCurrent"]; got != 20 {
		t.Fatalf("hpCurrent = %d, want unchanged 20", got)
	}
	if !strings.Contains(out, "\n동쪽\n\n") {
		t.Fatalf("output missing destination room:\n%s", out)
	}
}

func TestMoveHandlerDifficultClimbRaisesFallChanceLikeLegacy(t *testing.T) {
	withMoveTrapRolls(t, 90, 7)
	loaded := moveWorldWithEastExit(t, []string{"XCLIMB", "XDCLIM"}, "room:east")
	setMoveTrapActorStats(t, loaded, map[string]int{
		"dexterity": 0,
		"hpCurrent": 20,
		"hpMax":     20,
	})

	world, out := dispatchMoveLine(t, loaded, "동")

	assertMovePlayerRoom(t, world, "room:plaza")
	actor, _ := world.Creature("creature:alice")
	if got := actor.Stats["hpCurrent"]; got != 13 {
		t.Fatalf("hpCurrent = %d, want 13", got)
	}
	if out != "당신은 구덩이에 떨어져서 7 만큼의 상처를 입었습니다" {
		t.Fatalf("output = %q, want difficult climb fall damage only", out)
	}
}

func TestMoveHandlerGoWaitsForAttackCooldownAfterFallLikeLegacy(t *testing.T) {
	withFakeMagicEffectTime(t, 1000)
	loaded := moveWorldWithEastExit(t, nil, "room:east")
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	if err := world.SetCreatureCooldown("creature:alice", "attack", 1000, 5); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	out := dispatchMoveLineWithMoveWorld(t, world, "동 가")

	if out != "5초동안 기다리세요.\n" {
		t.Fatalf("output = %q, want go attack cooldown wait", out)
	}
	assertMovePlayerRoom(t, world, "room:plaza")
	actor, _ := world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(actor.Metadata.Tags, "hidden", "PHIDDN") {
		t.Fatalf("hidden tags = %+v, want retained before movement sneak", actor.Metadata.Tags)
	}
}

func TestMoveHandlerDirectMoveIgnoresAttackCooldownLikeLegacyCommentedDelay(t *testing.T) {
	withFakeMagicEffectTime(t, 1000)
	loaded := moveWorldWithEastExit(t, nil, "room:east")
	world := state.NewWorld(loaded)
	if err := world.SetCreatureCooldown("creature:alice", "attack", 1000, 5); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	out := dispatchMoveLineWithMoveWorld(t, world, "동")

	assertMovePlayerRoom(t, world, "room:east")
	if strings.Contains(out, "기다리세요") {
		t.Fatalf("direct move output = %q, want no attack cooldown wait", out)
	}
	if !strings.Contains(out, "\n동쪽\n\n") {
		t.Fatalf("output missing destination room:\n%s", out)
	}
}

func TestMoveHandlerKeepsHiddenOnSuccessfulMovementSneakLikeLegacy(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{name: "direct move", line: "동"},
		{name: "go", line: "동 가"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withMoveTrapRolls(t, 1)
			loaded := moveWorldWithEastExit(t, nil, "room:east")
			setMoveTrapActorStats(t, loaded, map[string]int{
				"class":     model.ClassThief,
				"level":     50,
				"dexterity": 50,
				"PHIDDN":    1,
			})
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = []string{"hidden", "PHIDDN"}
			loaded.Creatures[alice.ID] = alice

			world, out := dispatchMoveLine(t, loaded, tt.line)

			assertMovePlayerRoom(t, world, "room:east")
			actor, _ := world.Creature("creature:alice")
			if !attackCreatureHasFlag(actor, "hidden", "phiddn", "PHIDDN") {
				t.Fatalf("hidden flag was cleared after successful movement sneak: %+v", actor)
			}
			if strings.Contains(out, "은신술을 사용하는데 실패") {
				t.Fatalf("output contains failure message after successful movement sneak:\n%s", out)
			}
		})
	}
}

func TestMoveHandlerClearsHiddenForNonSneakClassLikeLegacy(t *testing.T) {
	loaded := moveWorldWithEastExit(t, nil, "room:east")
	setMoveTrapActorStats(t, loaded, map[string]int{
		"class":  model.ClassFighter,
		"PHIDDN": 1,
	})
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Players[player.ID] = player

	world, out := dispatchMoveLine(t, loaded, "동")

	assertMovePlayerRoom(t, world, "room:east")
	actor, _ := world.Creature("creature:alice")
	if attackCreatureHasFlag(actor, "hidden", "phiddn", "PHIDDN") {
		t.Fatalf("hidden flag was not cleared for non-sneak class: %+v", actor)
	}
	player, _ = world.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "phiddn", "PHIDDN") {
		t.Fatalf("player hidden tag was not cleared: %+v", player.Metadata.Tags)
	}
	if strings.Contains(out, "은신술을 사용하는데 실패") {
		t.Fatalf("output contains failure message for non-sneak class:\n%s", out)
	}
}

func TestMoveHandlerFailedMovementSneakClearsHiddenAndContinuesLikeLegacy(t *testing.T) {
	withMoveTrapRolls(t, 100)
	loaded := moveWorldWithEastExit(t, nil, "room:east")
	setMoveTrapActorStats(t, loaded, map[string]int{
		"class":     model.ClassThief,
		"level":     1,
		"dexterity": 0,
		"PHIDDN":    1,
	})
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Players[player.ID] = player

	world, out := dispatchMoveLine(t, loaded, "동")

	assertMovePlayerRoom(t, world, "room:east")
	actor, _ := world.Creature("creature:alice")
	if attackCreatureHasFlag(actor, "hidden", "phiddn", "PHIDDN") {
		t.Fatalf("hidden flag was not cleared after failed movement sneak: %+v", actor)
	}
	player, _ = world.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "phiddn", "PHIDDN") {
		t.Fatalf("player hidden tag was not cleared: %+v", player.Metadata.Tags)
	}
	assertMoveTrapOutputOrder(t, out,
		"당신은 은신술을 사용하는데 실패하였습니다.\n",
		"\n동쪽\n\n",
	)
}

func TestMoveHandlerFailedMovementSneakCanBeBlockedByEnemyMblockLikeLegacy(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{name: "direct move", line: "동"},
		{name: "go", line: "동 가"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withMoveTrapRolls(t, 100)
			loaded := moveWorldWithEastExit(t, nil, "room:east")
			setMoveTrapActorStats(t, loaded, map[string]int{
				"class":     model.ClassThief,
				"level":     1,
				"dexterity": 0,
				"PHIDDN":    1,
			})
			alice := loaded.Creatures["creature:alice"]
			alice.DisplayName = "AliceCreature"
			alice.Metadata.Tags = []string{"hidden", "PHIDDN"}
			loaded.Creatures[alice.ID] = alice
			mustAddLookCreature(t, loaded, model.Creature{
				ID:          "creature:blocker",
				Kind:        model.CreatureKindMonster,
				DisplayName: "고블린",
				RoomID:      "room:plaza",
				Metadata:    model.Metadata{Tags: []string{"MBLOCK"}},
			})
			world := state.NewWorld(loaded)
			if _, err := world.AddEnemy("creature:blocker", model.CreatureID("player:alice")); err != nil {
				t.Fatalf("AddEnemy() error = %v", err)
			}

			out := dispatchMoveLineWithMoveWorld(t, world, tt.line)

			assertMovePlayerRoom(t, world, "room:plaza")
			if want := "당신은 은신술을 사용하는데 실패하였습니다.\n고블린가 당신의 길을 가로막습니다.\n"; out != want {
				t.Fatalf("output = %q, want %q", out, want)
			}
			actor, _ := world.Creature("creature:alice")
			if attackCreatureHasFlag(actor, "hidden", "phiddn", "PHIDDN") {
				t.Fatalf("hidden flag was not cleared after failed blocked movement sneak: %+v", actor)
			}
		})
	}
}

func TestMoveHandlerFailedMovementSneakIgnoresEnemyMblockWithPINVISLikeLegacy(t *testing.T) {
	withMoveTrapRolls(t, 100)
	loaded := moveWorldWithEastExit(t, nil, "room:east")
	setMoveTrapActorStats(t, loaded, map[string]int{
		"class":     model.ClassThief,
		"level":     1,
		"dexterity": 0,
		"PHIDDN":    1,
	})
	alice := loaded.Creatures["creature:alice"]
	alice.DisplayName = "AliceCreature"
	alice.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible", "PINVIS"}
	loaded.Creatures[alice.ID] = alice
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:blocker",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:plaza",
		Metadata:    model.Metadata{Tags: []string{"MBLOCK"}},
	})
	world := state.NewWorld(loaded)
	if _, err := world.AddEnemy("creature:blocker", model.CreatureID("player:alice")); err != nil {
		t.Fatalf("AddEnemy() error = %v", err)
	}

	out := dispatchMoveLineWithMoveWorld(t, world, "동")

	player, _ := world.Player("player:alice")
	if player.RoomID != "room:east" {
		t.Fatalf("player room id = %q, want room:east; output = %q", player.RoomID, out)
	}
	if strings.Contains(out, "당신의 길을 가로막습니다.") {
		t.Fatalf("unexpected block output: %q", out)
	}
	actor, _ := world.Creature("creature:alice")
	if attackCreatureHasFlag(actor, "hidden", "phiddn", "PHIDDN") {
		t.Fatalf("hidden flag was not cleared after failed movement sneak: %+v", actor)
	}
	if !attackCreatureHasFlag(actor, "invisible", "pinvis", "PINVIS") {
		t.Fatalf("invisible flag was unexpectedly cleared after failed movement sneak: %+v", actor)
	}
}

func TestMoveHandlerIgnoresStaleAndZeroObjectIDsForNakedExitWeight(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		inventory  []model.ObjectInstanceID
		equipment  map[string]model.ObjectInstanceID
		wantRoomID model.RoomID
	}{
		{
			name:       "stale inventory object id has no weight",
			line:       "동",
			inventory:  []model.ObjectInstanceID{"", "object:missing-inventory"},
			wantRoomID: "room:east",
		},
		{
			name: "stale equipment object id has no weight",
			line: "동 가",
			equipment: map[string]model.ObjectInstanceID{
				"empty":   "",
				"missing": "object:missing-equipment",
			},
			wantRoomID: "room:east",
		},
		{
			name:      "zero object ids allow",
			line:      "동 가",
			inventory: []model.ObjectInstanceID{""},
			equipment: map[string]model.ObjectInstanceID{
				"empty": "",
			},
			wantRoomID: "room:east",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithEastExit(t, []string{"naked"}, "room:east")
			alice := loaded.Creatures["creature:alice"]
			alice.Inventory.ObjectIDs = tt.inventory
			alice.Equipment = tt.equipment
			loaded.Creatures[alice.ID] = alice

			world, out := dispatchMoveLine(t, loaded, tt.line)

			assertMovePlayerRoom(t, world, tt.wantRoomID)
			if !strings.Contains(out, "\n동쪽\n\n") {
				t.Fatalf("output missing destination room:\n%s", out)
			}
		})
	}
}

func TestMoveHandlerUsesObjectWeightForNakedExit(t *testing.T) {
	tests := []struct {
		name       string
		root       model.ObjectInstance
		child      *model.ObjectInstance
		weightless bool
		wantOutput string
		wantRoomID model.RoomID
	}{
		{
			name: "zero weight object allows",
			root: model.ObjectInstance{
				ID:          "object:paper",
				PrototypeID: "prototype:coin",
				Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
			},
			wantRoomID: "room:east",
		},
		{
			name: "nested weight blocks",
			root: model.ObjectInstance{
				ID:          "object:bag",
				PrototypeID: "prototype:coin",
				Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
				Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:stone"}},
			},
			child: &model.ObjectInstance{
				ID:          "object:stone",
				PrototypeID: "prototype:coin",
				Location:    model.ObjectLocation{ContainerID: "object:bag"},
				Properties:  map[string]string{"weight": "2"},
			},
			wantOutput: "뭘 가지고는 들어갈수 없습니다.\n",
			wantRoomID: "room:plaza",
		},
		{
			name: "weightless inventory root ignores contents",
			root: model.ObjectInstance{
				ID:          "object:weightless-bag",
				PrototypeID: "prototype:coin",
				Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
				Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:hidden-stone"}},
				Metadata:    model.Metadata{Tags: []string{"weightless"}},
			},
			child: &model.ObjectInstance{
				ID:          "object:hidden-stone",
				PrototypeID: "prototype:coin",
				Location:    model.ObjectLocation{ContainerID: "object:weightless-bag"},
				Properties:  map[string]string{"weight": "3"},
			},
			weightless: true,
			wantRoomID: "room:east",
		},
		{
			name: "flags container weightless root ignores contents",
			root: model.ObjectInstance{
				ID:          "object:flags-weightless-bag",
				PrototypeID: "prototype:coin",
				Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
				Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:flags-hidden-stone"}},
				Properties:  map[string]string{"flags": "weightless"},
			},
			child: &model.ObjectInstance{
				ID:          "object:flags-hidden-stone",
				PrototypeID: "prototype:coin",
				Location:    model.ObjectLocation{ContainerID: "object:flags-weightless-bag"},
				Properties:  map[string]string{"weight": "3"},
			},
			weightless: true,
			wantRoomID: "room:east",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithEastExit(t, []string{"naked"}, "room:east")
			mustAddLookObject(t, loaded, tt.root)
			alice := loaded.Creatures["creature:alice"]
			alice.Inventory.ObjectIDs = []model.ObjectInstanceID{tt.root.ID}
			loaded.Creatures[alice.ID] = alice
			if tt.child != nil {
				mustAddLookObject(t, loaded, *tt.child)
			}
			if tt.weightless {
				world := state.NewWorld(loaded)
				object, ok := world.Object(tt.root.ID)
				if !ok {
					t.Fatalf("root object %q missing", tt.root.ID)
				}
				if !moveObjectWeightless(world, object) {
					t.Fatalf("moveObjectWeightless(%q) = false; properties=%+v tags=%+v", tt.root.ID, object.Properties, object.Metadata.Tags)
				}
			}

			world, out := dispatchMoveLine(t, loaded, "동")

			assertMovePlayerRoom(t, world, tt.wantRoomID)
			if tt.wantOutput != "" {
				if out != tt.wantOutput {
					t.Fatalf("output = %q, want %q", out, tt.wantOutput)
				}
				return
			}
			if !strings.Contains(out, "\n동쪽\n\n") {
				t.Fatalf("output missing destination room:\n%s", out)
			}
		})
	}
}

func TestMoveHandlerReportsMissingDestination(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "direct",
			line: "동",
			want: "그쪽으로 지도가 없습니다. 신에게 연락해 주세요.\n",
		},
		{
			name: "go",
			line: "동 가",
			want: "그 방향의 지도가 없습니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithEastExit(t, nil, "room:missing")
			world, out := dispatchMoveLine(t, loaded, tt.line)

			if out != tt.want {
				t.Fatalf("output = %q, want %q", out, tt.want)
			}
			assertMovePlayerRoom(t, world, "room:plaza")
		})
	}
}

func TestMoveHandlerBlocksDestinationLevelRestrictions(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		properties    map[string]string
		tags          []string
		statsLevel    int
		creatureLevel int
		want          string
	}{
		{
			name:       "direct min level from property uses stats level",
			line:       "동",
			properties: map[string]string{"minLevel": "5"},
			statsLevel: 4,
			want:       "그쪽으로 갈 수 없습니다.\n",
		},
		{
			name:          "go max level from tag falls back to creature level",
			line:          "동 가",
			tags:          []string{"maxLevel:5"},
			creatureLevel: 6,
			want:          "그 방향으로 갈 수 없습니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := lookWorld(t)
			east := loaded.Rooms["room:east"]
			east.Properties = tt.properties
			east.Metadata.Tags = tt.tags
			loaded.Rooms[east.ID] = east

			alice := loaded.Creatures["creature:alice"]
			alice.Level = tt.creatureLevel
			if tt.statsLevel != 0 {
				alice.Stats = map[string]int{"level": tt.statsLevel}
			}
			loaded.Creatures[alice.ID] = alice

			world, out := dispatchMoveLine(t, loaded, tt.line)

			if out != tt.want {
				t.Fatalf("output = %q, want %q", out, tt.want)
			}
			assertMovePlayerRoom(t, world, "room:plaza")
		})
	}
}

func TestMoveHandlerAllowsDestinationLevelRestrictions(t *testing.T) {
	loaded := lookWorld(t)
	east := loaded.Rooms["room:east"]
	east.Properties = map[string]string{
		"minLevel": "5",
		"maxLevel": "10",
	}
	loaded.Rooms[east.ID] = east

	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{"level": 7}
	loaded.Creatures[alice.ID] = alice

	world, out := dispatchMoveLine(t, loaded, "동")

	assertMovePlayerRoom(t, world, "room:east")
	if !strings.Contains(out, "\n동쪽\n\n") {
		t.Fatalf("output missing destination room:\n%s", out)
	}
}

func TestMoveHandlerBlocksDestinationPlayerLimits(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		properties    map[string]string
		tags          []string
		existingCount int
		want          string
	}{
		{
			name:          "direct one player from property",
			line:          "동",
			properties:    map[string]string{"onePlayer": "true"},
			existingCount: 1,
			want:          "그쪽으로 갈 수 없습니다.\n",
		},
		{
			name:          "go two players from tag",
			line:          "동 가",
			tags:          []string{"twoPlayers"},
			existingCount: 2,
			want:          "그 방향으로 갈 수 없습니다.\n",
		},
		{
			name:          "direct three players from property value",
			line:          "동",
			properties:    map[string]string{"restrictions": "threePlayers"},
			existingCount: 3,
			want:          "그쪽으로 갈 수 없습니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := lookWorld(t)
			east := loaded.Rooms["room:east"]
			east.Properties = tt.properties
			east.Metadata.Tags = tt.tags
			loaded.Rooms[east.ID] = east
			addMoveDestinationPlayers(t, loaded, tt.existingCount)

			world, out := dispatchMoveLine(t, loaded, tt.line)

			if out != tt.want {
				t.Fatalf("output = %q, want %q", out, tt.want)
			}
			assertMovePlayerRoom(t, world, "room:plaza")
		})
	}
}

func TestMoveHandlerAllowsDestinationPlayerLimitBelowCapacity(t *testing.T) {
	loaded := lookWorld(t)
	east := loaded.Rooms["room:east"]
	east.Metadata.Tags = []string{"twoPlayers"}
	loaded.Rooms[east.ID] = east
	addMoveDestinationPlayers(t, loaded, 1)

	world, out := dispatchMoveLine(t, loaded, "동 가")

	assertMovePlayerRoom(t, world, "room:east")
	if !strings.Contains(out, "\n동쪽\n\n") {
		t.Fatalf("output missing destination room:\n%s", out)
	}
}

func TestMoveHandlerDestinationPlayerLimitIgnoresPDMINVLikeCountVisPly(t *testing.T) {
	loaded := lookWorld(t)
	east := loaded.Rooms["room:east"]
	east.Properties = map[string]string{"onePlayer": "true"}
	loaded.Rooms[east.ID] = east
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:dest-invis",
		DisplayName: "Invisible",
		CreatureID:  "creature:dest-invis",
		RoomID:      "room:east",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:dest-invis",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Invisible",
		PlayerID:    "player:dest-invis",
		RoomID:      "room:east",
		Metadata:    model.Metadata{Tags: []string{"PDMINV"}},
		Stats:       map[string]int{"PDMINV": 1},
	})

	world, out := dispatchMoveLine(t, loaded, "동")

	assertMovePlayerRoom(t, world, "room:east")
	if !strings.Contains(out, "\n동쪽\n\n") {
		t.Fatalf("output missing destination room:\n%s", out)
	}
}

func TestMoveHandlerBlocksDestinationFamilyRestrictions(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		tags       []string
		properties map[string]string
		stats      map[string]int
		creature   map[string]string
		want       string
	}{
		{
			name: "direct family without family flag",
			line: "동",
			tags: []string{"family"},
			want: "그쪽으로 갈 수 없습니다.\n",
		},
		{
			name:  "direct family without family flag still blocks DM",
			line:  "동",
			tags:  []string{"family"},
			stats: map[string]int{"class": 13},
			want:  "그쪽으로 갈 수 없습니다.\n",
		},
		{
			name: "go family without family flag",
			line: "동 가",
			tags: []string{"family"},
			want: "그 방향으로 갈 수 없습니다.\n",
		},
		{
			name:       "direct only family mismatch",
			line:       "동",
			tags:       []string{"onlyFamily"},
			properties: map[string]string{"special": "7"},
			stats:      map[string]int{"familyID": 6},
			want:       "그쪽으로 갈 수 없습니다.\n",
		},
		{
			name:       "go only family mismatch",
			line:       "동 가",
			tags:       []string{"onlyFamily"},
			properties: map[string]string{"special": "7"},
			creature:   map[string]string{"familyID": "6"},
			want:       "그 방향으로 갈 수 없습니다.\n",
		},
		{
			name:       "direct only married mismatch",
			line:       "동",
			tags:       []string{"onlyMarried"},
			properties: map[string]string{"special": "9"},
			stats:      map[string]int{"marriageID": 8},
			want:       "그쪽으로 갈 수 없습니다.\n",
		},
		{
			name:       "go only married mismatch",
			line:       "동 가",
			tags:       []string{"onlyMarried"},
			properties: map[string]string{"special": "9"},
			creature:   map[string]string{"marriageID": "8"},
			want:       "그 방향으로 갈 수 없습니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithDestinationFamilyRestriction(t, tt.tags, tt.properties, tt.stats, tt.creature)

			world, out := dispatchMoveLine(t, loaded, tt.line)

			if out != tt.want {
				t.Fatalf("output = %q, want %q", out, tt.want)
			}
			assertMovePlayerRoom(t, world, "room:plaza")
		})
	}
}

func TestMoveHandlerAllowsDestinationFamilyRestrictions(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		tags       []string
		properties map[string]string
		stats      map[string]int
		creature   map[string]string
	}{
		{
			name:  "direct family with stats family flag",
			line:  "동",
			tags:  []string{"family"},
			stats: map[string]int{"familyFlag": 1},
		},
		{
			name:     "go family with property PFAMIL",
			line:     "동 가",
			tags:     []string{"family"},
			creature: map[string]string{"PFAMIL": "true"},
		},
		{
			name:       "direct only family with stats family id",
			line:       "동",
			tags:       []string{"onlyFamily"},
			properties: map[string]string{"special": "7"},
			stats:      map[string]int{"familyID": 7},
		},
		{
			name:       "go only family with property daily expnd max",
			line:       "동 가",
			tags:       []string{"onlyFamily"},
			properties: map[string]string{"special": "7"},
			creature:   map[string]string{"dailyExpndMax": "7"},
		},
		{
			name:       "direct only family with stats legacy daily expnd max",
			line:       "동",
			tags:       []string{"onlyFamily"},
			properties: map[string]string{"special": "7"},
			stats:      map[string]int{"legacyDailyExpndMax": 7},
		},
		{
			name:       "direct only family allows DM despite mismatch",
			line:       "동",
			tags:       []string{"onlyFamily"},
			properties: map[string]string{"special": "7"},
			stats:      map[string]int{"class": 13, "familyID": 6},
		},
		{
			name:       "direct only married with stats marriage id",
			line:       "동",
			tags:       []string{"onlyMarried"},
			properties: map[string]string{"special": "9"},
			stats:      map[string]int{"marriageID": 9},
		},
		{
			name:       "go only married with property daily marriage max",
			line:       "동 가",
			tags:       []string{"onlyMarried"},
			properties: map[string]string{"special": "9"},
			creature:   map[string]string{"dailyMarriageMax": "9"},
		},
		{
			name:       "direct only married with stats legacy daily marriage max",
			line:       "동",
			tags:       []string{"onlyMarried"},
			properties: map[string]string{"special": "9"},
			stats:      map[string]int{"legacyDailyMarriageMax": 9},
		},
		{
			name:       "direct only married allows DM despite mismatch",
			line:       "동",
			tags:       []string{"onlyMarried"},
			properties: map[string]string{"special": "9"},
			stats:      map[string]int{"class": 13, "marriageID": 8},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithDestinationFamilyRestriction(t, tt.tags, tt.properties, tt.stats, tt.creature)

			world, out := dispatchMoveLine(t, loaded, tt.line)

			assertMovePlayerRoom(t, world, "room:east")
			if !strings.Contains(out, "\n동쪽\n\n") {
				t.Fatalf("output missing destination room:\n%s", out)
			}
		})
	}
}

func TestMoveHandlerHandlesOnlyMarriedInviteException(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		inviteAllowed bool
		wantRoomID    model.RoomID
		wantOutput    string
	}{
		{
			name:          "direct invite allows only married mismatch",
			line:          "동",
			inviteAllowed: true,
			wantRoomID:    "room:east",
		},
		{
			name:          "go invite allows only married mismatch",
			line:          "동 가",
			inviteAllowed: true,
			wantRoomID:    "room:east",
		},
		{
			name:       "direct no invite blocks only married mismatch",
			line:       "동",
			wantRoomID: "room:plaza",
			wantOutput: "그쪽으로 갈 수 없습니다.\n",
		},
		{
			name:       "go no invite blocks only married mismatch",
			line:       "동 가",
			wantRoomID: "room:plaza",
			wantOutput: "그 방향으로 갈 수 없습니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := moveWorldWithDestinationFamilyRestriction(
				t,
				[]string{"onlyMarried"},
				map[string]string{"special": "9"},
				map[string]int{"marriageID": 8},
				nil,
			)
			world := newMarriageInviteMoveWorld(loaded)
			if tt.inviteAllowed {
				world.allowMarriageInvite(9, "player:alice")
			}

			out := dispatchMoveLineWithMoveWorld(t, world, tt.line)

			assertMoveWorldPlayerRoom(t, world, tt.wantRoomID)
			if tt.wantOutput != "" {
				if out != tt.wantOutput {
					t.Fatalf("output = %q, want %q", out, tt.wantOutput)
				}
				return
			}
			if !strings.Contains(out, "\n동쪽\n\n") {
				t.Fatalf("output missing destination room:\n%s", out)
			}
		})
	}
}

func TestMoveHandlerUserFacingFailuresDoNotError(t *testing.T) {
	world := state.NewWorld(lookWorld(t))
	handler := NewMoveHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec:   commandspec.CommandSpec{Handler: "move"},
		Parsed: commandWithVerb("북"),
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "길이 막혀 있습니다.\n" {
		t.Fatalf("status/output = %d/%q, want blocked path", status, ctx.OutputString())
	}

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{
		Spec:   commandspec.CommandSpec{Handler: "go"},
		Args:   []string{"없는"},
		Values: []int64{1},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그런 출구는 없습니다.\n" {
		t.Fatalf("status/output = %d/%q, want missing exit", status, ctx.OutputString())
	}
}

func TestMoveHandlerRequiresActor(t *testing.T) {
	world := state.NewWorld(lookWorld(t))
	handler := NewMoveHandler(world)

	_, err := handler(&Context{}, ResolvedCommand{Args: []string{"동"}})
	if !errors.Is(err, ErrMoveActorRequired) {
		t.Fatalf("handler() error = %v, want ErrMoveActorRequired", err)
	}
}

func TestMoveHandlerRequiresWorld(t *testing.T) {
	handler := NewMoveHandler(nil)

	_, err := handler(&Context{ActorID: "player:alice"}, ResolvedCommand{Args: []string{"동"}})
	if !errors.Is(err, ErrMoveWorldRequired) {
		t.Fatalf("handler() error = %v, want ErrMoveWorldRequired", err)
	}
}

func commandWithVerb(verb string) commandparse.Command {
	var parsed commandparse.Command
	parsed.Num = 1
	parsed.Str[0] = verb
	parsed.Val[0] = 1
	return parsed
}

func dispatchMoveLine(t *testing.T, loaded *worldload.World, line string) (*state.World, string) {
	t.Helper()

	world := state.NewWorld(loaded)
	out := dispatchMoveLineWithMoveWorld(t, world, line)
	return world, out
}

func dispatchMoveLineWithMoveWorld(t *testing.T, world MoveWorld, line string) string {
	t.Helper()

	ctx := &Context{ActorID: "player:alice"}
	dispatchMoveLineWithContext(t, world, ctx, line)
	return ctx.OutputString()
}

func dispatchMoveLineWithContext(t *testing.T, world MoveWorld, ctx *Context, line string) {
	t.Helper()

	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "가", Number: 30, Handler: "go"},
			{Name: "동", Number: 1, Handler: "move"},
			{Name: "6", Number: 1, Handler: "move"},
		}),
		Handlers: map[string]Handler{
			"go":   NewMoveHandler(world),
			"move": NewMoveHandler(world),
		},
	}

	status, err := dispatcher.DispatchLine(ctx, line)
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
}

func assertMovePlayerRoom(t *testing.T, world *state.World, want model.RoomID) {
	t.Helper()

	assertMoveWorldPlayerRoom(t, world, want)
}

func assertMoveWorldPlayerRoom(t *testing.T, world MoveWorld, want model.RoomID) {
	t.Helper()

	player, ok := world.Player("player:alice")
	if !ok {
		t.Fatal("missing player")
	}
	if player.RoomID != want {
		t.Fatalf("player room id = %q, want %s", player.RoomID, want)
	}
}

type marriageInviteMoveWorld struct {
	*state.World
	playerRooms     map[model.PlayerID]model.RoomID
	marriageInvites map[int]map[model.PlayerID]bool
}

func newMarriageInviteMoveWorld(loaded *worldload.World) *marriageInviteMoveWorld {
	return &marriageInviteMoveWorld{
		World:           state.NewWorld(loaded),
		playerRooms:     map[model.PlayerID]model.RoomID{},
		marriageInvites: map[int]map[model.PlayerID]bool{},
	}
}

func (w *marriageInviteMoveWorld) Player(id model.PlayerID) (model.Player, bool) {
	player, ok := w.World.Player(id)
	if !ok {
		return model.Player{}, false
	}
	if roomID, ok := w.playerRooms[id]; ok {
		player.RoomID = roomID
	}
	return player, true
}

func (w *marriageInviteMoveWorld) MovePlayer(playerID model.PlayerID, exitName string) error {
	player, ok := w.Player(playerID)
	if !ok {
		return errors.New("missing player")
	}
	room, ok := w.Room(player.RoomID)
	if !ok {
		return errors.New("missing player room")
	}
	for _, exit := range room.Exits {
		if exit.Name == exitName {
			w.playerRooms[playerID] = exit.ToRoomID
			return nil
		}
	}
	return errors.New("missing exit")
}

func (w *marriageInviteMoveWorld) HasMarriageInvite(playerID model.PlayerID, specialID model.SpecialID) bool {
	roomSpecial := int(specialID)
	players := w.marriageInvites[roomSpecial]
	return players[playerID]
}

func (w *marriageInviteMoveWorld) allowMarriageInvite(roomSpecial int, playerID model.PlayerID) {
	players := w.marriageInvites[roomSpecial]
	if players == nil {
		players = map[model.PlayerID]bool{}
		w.marriageInvites[roomSpecial] = players
	}
	players[playerID] = true
}

func moveWorldWithEastExit(t *testing.T, flags []string, toRoomID model.RoomID) *worldload.World {
	t.Helper()

	loaded := lookWorld(t)
	room := loaded.Rooms["room:plaza"]
	room.Exits[0].Flags = flags
	room.Exits[0].ToRoomID = toRoomID
	loaded.Rooms[room.ID] = room
	return loaded
}

func addMoveCreatureObjectRef(
	t *testing.T,
	loaded *worldload.World,
	objectID model.ObjectInstanceID,
	slot string,
	inInventory bool,
	inEquipment bool,
) {
	t.Helper()

	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          objectID,
		PrototypeID: "prototype:coin",
		Quantity:    1,
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: slot},
		Properties:  map[string]string{"weight": "1"},
	})

	alice := loaded.Creatures["creature:alice"]
	if inInventory {
		alice.Inventory.ObjectIDs = append(alice.Inventory.ObjectIDs, objectID)
	}
	if inEquipment {
		if alice.Equipment == nil {
			alice.Equipment = map[string]model.ObjectInstanceID{}
		}
		alice.Equipment[slot] = objectID
	}
	loaded.Creatures[alice.ID] = alice
}

func addMoveDMFollower(t *testing.T, loaded *worldload.World, id model.CreatureID, name string, properties map[string]string) {
	t.Helper()

	mustAddLookCreature(t, loaded, model.Creature{
		ID:          id,
		Kind:        model.CreatureKindMonster,
		DisplayName: name,
		RoomID:      "room:plaza",
		Properties:  properties,
		Metadata:    model.Metadata{Tags: []string{"MDMFOL"}},
	})
}

func goOrdinalWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:plaza",
		DisplayName: "광장",
		Exits: []model.Exit{
			{Name: "동문", ToRoomID: "room:first"},
			{Name: "동굴", ToRoomID: "room:east"},
		},
	})
	mustAddLookRoom(t, loaded, model.Room{ID: "room:first", DisplayName: "첫 동쪽"})
	mustAddLookRoom(t, loaded, model.Room{ID: "room:east", DisplayName: "두 번째 동쪽"})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:plaza",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:plaza",
	})
	return loaded
}

func addMoveDestinationPlayers(t *testing.T, loaded *worldload.World, count int) {
	t.Helper()

	for i := 0; i < count; i++ {
		id := model.PlayerID("player:dest" + string(rune('a'+i)))
		mustAddLookPlayer(t, loaded, model.Player{
			ID:          id,
			DisplayName: "Destination player",
			RoomID:      "room:east",
		})
	}
}

func moveWorldWithDestinationFamilyRestriction(
	t *testing.T,
	tags []string,
	properties map[string]string,
	stats map[string]int,
	creatureProperties map[string]string,
) *worldload.World {
	t.Helper()

	loaded := lookWorld(t)
	east := loaded.Rooms["room:east"]
	east.Metadata.Tags = tags
	east.Properties = properties
	loaded.Rooms[east.ID] = east

	alice := loaded.Creatures["creature:alice"]
	alice.Stats = stats
	alice.Properties = creatureProperties
	loaded.Creatures[alice.ID] = alice
	return loaded
}
