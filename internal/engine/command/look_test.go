package command

import (
	"strings"
	"testing"

	"muhan/internal/commandspec"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestLookHandlerRendersCurrentRoom(t *testing.T) {
	world := state.NewWorld(lookWorld(t))
	defer world.Close()
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "봐", Number: 2, Handler: "look"},
	})
	dispatcher := Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"look": NewLookHandler(world),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "봐")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	out := ctx.OutputString()
	for _, want := range []string{
		"\n광장\n\n",
		"넓은 광장이다.\n",
		"사람들이 오가는 오래된 광장이다.\n",
		"[ 출구 : 동, 서 ]\n",
		"Bob님이 서 있습니다.\n",
		"경비병이 서 있다.\n",
		"금화가 놓여져 있습니다.\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Alice님이 있습니다.") {
		t.Fatalf("output includes viewer:\n%s", out)
	}
	if strings.Contains(out, "먼 방 물건") {
		t.Fatalf("output includes stale room object ref:\n%s", out)
	}
}

func TestLookHandlerHonorsLegacyRoomDisplaySuppressFlags(t *testing.T) {
	loaded := lookWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"PNORNM", "PNOSDS", "PNOLDS"}
	loaded.Creatures[creature.ID] = creature

	out := dispatchLookLine(t, loaded, "봐")
	for _, unexpected := range []string{
		"\n광장\n\n",
		"넓은 광장이다.\n",
		"사람들이 오가는 오래된 광장이다.\n",
	} {
		if strings.Contains(out, unexpected) {
			t.Fatalf("output includes suppressed %q:\n%s", unexpected, out)
		}
	}
	for _, want := range []string{
		"[ 출구 : 동, 서 ]\n",
		"Bob님이 서 있습니다.\n",
		"경비병이 서 있다.\n",
		"금화가 놓여져 있습니다.\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestLookHandlerRendersLegacyExitGraphWhenPNOEXTSet(t *testing.T) {
	loaded := lookWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"PNOEXT"}
	loaded.Creatures[creature.ID] = creature
	room := loaded.Rooms["room:plaza"]
	room.Exits = append(room.Exits,
		model.Exit{Name: "북", ToRoomID: "room:north"},
		model.Exit{Name: "사다리", ToRoomID: "room:ladder"},
	)
	loaded.Rooms[room.ID] = room

	out := dispatchLookLine(t, loaded, "봐")
	for _, want := range []string{
		"[    |    ]\n",
		"[ -- O -- ] [ 출구 : 사다리 ]\n",
		"[         ]\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing graph line %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[ 출구 : 동, 서") {
		t.Fatalf("output rendered text exit list instead of graph:\n%s", out)
	}
}

func TestLookHandlerRendersLegacyPlayerDescriptionsWhenPDSCRPSet(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PDSCRP"}
	loaded.Creatures[alice.ID] = alice
	bob := loaded.Players["player:bob"]
	bob.CreatureID = "creature:bob"
	loaded.Players[bob.ID] = bob
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		Description: "서 ",
		PlayerID:    "player:bob",
		RoomID:      "room:plaza",
		Metadata:    model.Metadata{Tags: []string{"PANGEL"}},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:carol",
		DisplayName: "Carol",
		CreatureID:  "creature:carol",
		RoomID:      "room:plaza",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:carol",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Carol",
		Description: "앉아 ",
		PlayerID:    "player:carol",
		RoomID:      "room:plaza",
	})

	out := dispatchLookLine(t, loaded, "봐")
	for _, want := range []string{
		"Bob님이 서 있습니다.\n",
		"Bob의 정령이 주위를 맴돕니다.\n",
		"Carol님이 앉아 있습니다.\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Bob, Carol님이 서 있습니다.") {
		t.Fatalf("output used compact player list in PDSCRP mode:\n%s", out)
	}
}

func TestLookHandlerRendersOnlyActivePlayersWhenContextProvidesFilter(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:carol",
		DisplayName: "Carol",
		RoomID:      "room:plaza",
	})

	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			ContextActiveActorIDsKey: func() []string {
				return []string{"player:alice", "player:bob"}
			},
		},
	}
	out := dispatchLookLineWithContext(t, loaded, "봐", ctx)
	if !strings.Contains(out, "Bob님이 서 있습니다.\n") {
		t.Fatalf("output missing active player:\n%s", out)
	}
	if strings.Contains(out, "Carol") {
		t.Fatalf("output includes inactive player:\n%s", out)
	}
}

func TestRenderCurrentRoomUsesActorLocation(t *testing.T) {
	world := state.NewWorld(lookWorld(t))
	defer world.Close()
	got, err := RenderCurrentRoom(world, LookViewer{PlayerID: "player:alice"})
	if err != nil {
		t.Fatalf("RenderCurrentRoom() error = %v", err)
	}
	for _, want := range []string{"\n광장\n\n", "[ 출구 : 동, 서 ]\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestLookHandlerRendersRoomObjectByUTF8NamePrefix(t *testing.T) {
	if got := dispatchLookLine(t, targetLookWorld(t), "금 봐"); got != "작은 금화가 반짝인다.\n" {
		t.Fatalf("output = %q, want room object look", got)
	}
}

func TestLookHandlerRejectsObjectIDTargetLikeLegacyFindObj(t *testing.T) {
	if got := dispatchLookLine(t, targetLookWorld(t), "object:coin 봐"); got != "그런 건 보이지 않습니다.\n" {
		t.Fatalf("output = %q, want missing object ID target", got)
	}
	if got := dispatchLookLine(t, targetLookWorld(t), "object:talisman 봐"); got != "그런 건 보이지 않습니다.\n" {
		t.Fatalf("inventory output = %q, want missing object ID target", got)
	}
}

func TestLookHandlerRendersLegacyEmptyObjectDescription(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:plain-stone",
		DisplayName: "돌",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:plain-stone",
		PrototypeID: "prototype:plain-stone",
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
	})
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:plain-stone")
	loaded.Rooms[room.ID] = room

	if got := dispatchLookLine(t, loaded, "돌 봐"); got != "특별한 점이 없습니다.\n" {
		t.Fatalf("output = %q, want legacy empty object description", got)
	}
}

func TestLookHandlerStripsLegacyColorMarkers(t *testing.T) {
	loaded := lookWorld(t)
	room := loaded.Rooms["room:plaza"]
	room.ShortDescription = "{흰넓은 광장이다}"
	loaded.Rooms[room.ID] = room
	creature := loaded.Creatures["creature:guard"]
	creature.Description = "{빨경비병이 서 있다.}"
	loaded.Creatures[creature.ID] = creature
	proto := loaded.ObjectPrototypes["prototype:coin"]
	proto.DisplayName = "{보금화}"
	loaded.ObjectPrototypes[proto.ID] = proto

	out := dispatchLookLine(t, loaded, "봐")
	for _, raw := range []string{"{흰", "{빨", "{보", "}"} {
		if strings.Contains(out, raw) {
			t.Fatalf("output contains raw color marker %q:\n%s", raw, out)
		}
	}
	for _, want := range []string{"넓은 광장이다\n", "경비병이 서 있다.\n", "금화가 놓여져 있습니다.\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestLookHandlerPreservesRoomDescriptionLeadingSpaces(t *testing.T) {
	loaded := lookWorld(t)
	room := loaded.Rooms["room:plaza"]
	room.ShortDescription = "    ******       생명의 나무 밑에 펼쳐진\n  **********     통계 무한 광장입니다.\n   "
	loaded.Rooms[room.ID] = room

	out := dispatchLookLine(t, loaded, "봐")
	want := "    ******       생명의 나무 밑에 펼쳐진\n  **********     통계 무한 광장입니다.\n"
	if !strings.Contains(out, want) {
		t.Fatalf("output missing preserved leading spaces %q:\n%s", want, out)
	}
	if strings.Contains(out, "통계 무한 광장입니다.\n\n[ 출구") {
		t.Fatalf("output contains extra blank line after room description:\n%s", out)
	}
}

func TestLookHandlerPreservesObjectAndCreatureDescriptionLeadingSpaces(t *testing.T) {
	loaded := lookWorld(t)
	guard := loaded.Creatures["creature:guard"]
	guard.Description = "  경비병이 서 있다.\n   "
	loaded.Creatures[guard.ID] = guard
	coin := loaded.ObjectPrototypes["prototype:coin"]
	coin.Description = "  작은 금화가 반짝인다.\n   "
	loaded.ObjectPrototypes[coin.ID] = coin

	if got := dispatchLookLine(t, loaded, "경 봐"); got != "당신은 경비병을 봅니다.\n  경비병이 서 있다.\n그녀는 당신과 꼭 맞는 상대입니다!\n" {
		t.Fatalf("creature output = %q, want leading spaces preserved", got)
	}
	if got := dispatchLookLine(t, loaded, "금 봐"); got != "  작은 금화가 반짝인다.\n" {
		t.Fatalf("object output = %q, want leading spaces preserved", got)
	}
}

func TestLookHandlerRendersANSIWhenContextEnablesIt(t *testing.T) {
	loaded := lookWorld(t)
	room := loaded.Rooms["room:plaza"]
	room.ShortDescription = "{빨넓은 광장이다}"
	loaded.Rooms[room.ID] = room

	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			ContextANSIKey: true,
		},
	}
	out := dispatchLookLineWithContext(t, loaded, "봐", ctx)
	for _, want := range []string{
		"\x1b[0;36m광장\x1b[0;0m\n\n",
		"\x1b[0;31m넓은 광장이다\x1b[0;37m\n",
		"\x1b[0;32m[ 출구 : 동, 서 ]\x1b[0;0m\n",
		"\x1b[0;36mBob님이 서 있습니다.\x1b[0;0m\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ANSI output missing %q:\n%q", want, out)
		}
	}
}

func TestLookHandlerUsesLegacyBlindRoomMessage(t *testing.T) {
	loaded := lookWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"PBLIND"}
	loaded.Creatures[creature.ID] = creature

	want := "\n당신은 눈이 멀어 아무것도 볼 수 없습니다.\n너무 어두워서 볼 수가 없습니다.\n"
	if got := dispatchLookLine(t, loaded, "봐"); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestLookHandlerUsesLegacyBlindTargetMessage(t *testing.T) {
	loaded := lookWorld(t)
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"blind"}
	loaded.Players[player.ID] = player

	if got := dispatchLookLine(t, loaded, "금 봐"); got != "당신은 눈이 멀어 있습니다!\n" {
		t.Fatalf("output = %q, want blind target message", got)
	}
}

func TestLookHandlerUsesLegacyBlindANSIColors(t *testing.T) {
	loaded := lookWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Stats = map[string]int{"PBLIND": 1}
	loaded.Creatures[creature.ID] = creature

	ctx := &Context{
		ActorID: "player:alice",
		Values:  map[string]any{ContextANSIKey: true},
	}
	want := "\n" +
		"\x1b[0;31m당신은 눈이 멀어 아무것도 볼 수 없습니다.\n\x1b[0;0m" +
		"\x1b[0;33m너무 어두워서 볼 수가 없습니다.\n\x1b[0;0m"
	if got := dispatchLookLineWithContext(t, loaded, "봐", ctx); got != want {
		t.Fatalf("room output = %q, want %q", got, want)
	}

	ctx = &Context{
		ActorID: "player:alice",
		Values:  map[string]any{ContextANSIKey: true},
	}
	want = "\x1b[1;31m당신은 눈이 멀어 있습니다!\n\x1b[1;0m"
	if got := dispatchLookLineWithContext(t, loaded, "금 봐", ctx); got != want {
		t.Fatalf("target output = %q, want %q", got, want)
	}
}

func TestLookHandlerDarkAlwaysRoomHonorsLegacyLightRules(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, loaded *worldload.World)
		wantVisible bool
	}{
		{
			name:        "no light blocks room",
			wantVisible: false,
		},
		{
			name: "PLIGHT status illuminates",
			setup: func(t *testing.T, loaded *worldload.World) {
				t.Helper()
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"PLIGHT"}
				loaded.Creatures[alice.ID] = alice
			},
			wantVisible: true,
		},
		{
			name: "equipped active light source illuminates",
			setup: func(t *testing.T, loaded *worldload.World) {
				t.Helper()
				addLookEquippedLight(t, loaded, "1")
			},
			wantVisible: true,
		},
		{
			name: "equipped exhausted light source stays dark",
			setup: func(t *testing.T, loaded *worldload.World) {
				t.Helper()
				addLookEquippedLight(t, loaded, "0")
			},
			wantVisible: false,
		},
		{
			name: "nearby player light illuminates",
			setup: func(t *testing.T, loaded *worldload.World) {
				t.Helper()
				bob := loaded.Players["player:bob"]
				bob.CreatureID = "creature:bob"
				loaded.Players[bob.ID] = bob
				mustAddLookCreature(t, loaded, model.Creature{
					ID:          "creature:bob",
					Kind:        model.CreatureKindPlayer,
					DisplayName: "Bob",
					PlayerID:    "player:bob",
					RoomID:      "room:plaza",
					Metadata:    model.Metadata{Tags: []string{"PLIGHT"}},
				})
			},
			wantVisible: true,
		},
		{
			name: "elf race sees in dark",
			setup: func(t *testing.T, loaded *worldload.World) {
				t.Helper()
				alice := loaded.Creatures["creature:alice"]
				alice.Stats = map[string]int{"race": legacyRaceElf}
				loaded.Creatures[alice.ID] = alice
			},
			wantVisible: true,
		},
		{
			name: "caretaker class sees in dark",
			setup: func(t *testing.T, loaded *worldload.World) {
				t.Helper()
				alice := loaded.Creatures["creature:alice"]
				alice.Stats = map[string]int{"class": model.ClassCaretaker}
				loaded.Creatures[alice.ID] = alice
			},
			wantVisible: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := lookWorld(t)
			room := loaded.Rooms["room:plaza"]
			room.Metadata.Tags = []string{"RDARKR"}
			loaded.Rooms[room.ID] = room
			if tt.setup != nil {
				tt.setup(t, loaded)
			}

			got := dispatchLookLine(t, loaded, "봐")
			if tt.wantVisible {
				if !strings.Contains(got, "\n광장\n\n") || strings.Contains(got, "너무 어두워서") {
					t.Fatalf("output = %q, want visible dark-room look", got)
				}
				return
			}
			if got != "\n너무 어두워서 볼 수가 없습니다.\n" {
				t.Fatalf("output = %q, want dark-room refusal", got)
			}
		})
	}
}

func TestLookHandlerDarkNightRoomUsesLegacyTime(t *testing.T) {
	tests := []struct {
		name        string
		legacyTime  int64
		setup       func(t *testing.T, loaded *worldload.World)
		wantVisible bool
	}{
		{name: "hour five is dark", legacyTime: 5, wantVisible: false},
		{name: "hour six is visible", legacyTime: 6, wantVisible: true},
		{name: "hour twenty is visible", legacyTime: 20, wantVisible: true},
		{name: "hour twenty one is dark", legacyTime: 21, wantVisible: false},
		{
			name:       "night with light is visible",
			legacyTime: 21,
			setup: func(t *testing.T, loaded *worldload.World) {
				t.Helper()
				alice := loaded.Creatures["creature:alice"]
				alice.Metadata.Tags = []string{"PLIGHT"}
				loaded.Creatures[alice.ID] = alice
			},
			wantVisible: true,
		},
		{name: "negative time normalizes to night hour", legacyTime: -3, wantVisible: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := lookWorld(t)
			room := loaded.Rooms["room:plaza"]
			room.Metadata.Tags = []string{"RDARKN"}
			loaded.Rooms[room.ID] = room
			if tt.setup != nil {
				tt.setup(t, loaded)
			}

			got := dispatchLookLineWithRuntime(t, loaded, "봐", &Context{ActorID: "player:alice"}, func(world *state.World) {
				world.SetLegacyTime(tt.legacyTime)
			})
			if tt.wantVisible {
				if !strings.Contains(got, "\n광장\n\n") || strings.Contains(got, "너무 어두워서") {
					t.Fatalf("output = %q, want visible dark-night room look", got)
				}
				return
			}
			if got != "\n너무 어두워서 볼 수가 없습니다.\n" {
				t.Fatalf("output = %q, want dark-night refusal", got)
			}
		})
	}
}

func TestLookHandlerCannotTargetInactivePlayerWhenContextProvidesFilter(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:carol",
		DisplayName: "Carol",
		CreatureID:  "creature:carol",
		RoomID:      "room:plaza",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:carol",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Carol",
		PlayerID:    "player:carol",
		RoomID:      "room:plaza",
	})

	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			ContextActiveActorIDsKey: func() []string {
				return []string{"player:alice", "player:bob"}
			},
		},
	}
	if got := dispatchLookLineWithContext(t, loaded, "Car 봐", ctx); got != "그런 건 보이지 않습니다.\n" {
		t.Fatalf("output = %q, want inactive player hidden", got)
	}
}

func TestLookRenderRoomSkipsStaleOccupantRefs(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:carol",
		DisplayName: "Carol",
		RoomID:      "room:east",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:wolf",
		Kind:        model.CreatureKindNPC,
		DisplayName: "늑대",
		Description: "늑대가 있다.",
		RoomID:      "room:east",
	})

	world := state.NewWorld(loaded)
	defer world.Close()
	room, ok := world.Room("room:plaza")
	if !ok {
		t.Fatal("missing room")
	}
	room.PlayerIDs = append(room.PlayerIDs, "player:carol")
	room.CreatureIDs = append(room.CreatureIDs, "creature:wolf")

	got := RenderRoomLook(world, room, LookViewer{
		PlayerID:   "player:alice",
		CreatureID: "creature:alice",
	})
	for _, want := range []string{
		"Bob님이 서 있습니다.\n",
		"경비병이 서 있다.\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	for _, stale := range []string{"Carol님이 있습니다.", "늑대가 있다."} {
		if strings.Contains(got, stale) {
			t.Fatalf("output includes stale occupant %q:\n%s", stale, got)
		}
	}
}

func TestLookRenderRoomGroupsPlayersAndConsecutiveCreatures(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:carol",
		DisplayName: "Carol",
		RoomID:      "room:plaza",
	})
	room := loaded.Rooms["room:plaza"]
	room.PlayerIDs = append(room.PlayerIDs, "player:carol")
	room.CreatureIDs = append(room.CreatureIDs, "creature:guard2", "creature:merchant", "creature:guard3")
	loaded.Rooms[room.ID] = room

	for _, id := range []model.CreatureID{"creature:guard2", "creature:guard3"} {
		mustAddLookCreature(t, loaded, model.Creature{
			ID:          id,
			Kind:        model.CreatureKindNPC,
			DisplayName: "경비병",
			Description: "경비병이 서 있다.",
			RoomID:      "room:plaza",
		})
	}
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		Description: "상인이 물건을 정리하고 있다.",
		RoomID:      "room:plaza",
	})

	got := dispatchLookLine(t, loaded, "봐")
	for _, want := range []string{
		"Bob, Carol님이 서 있습니다.\n",
		"(x3) 경비병이 서 있다.\n",
		"상인이 물건을 정리하고 있다.\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestLookRenderRoomShowsCreatureAurasWithKnowAlignment(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PKNOWA"}
	loaded.Creatures[alice.ID] = alice
	guard := loaded.Creatures["creature:guard"]
	guard.Stats = map[string]int{"alignment": -10}
	loaded.Creatures[guard.ID] = guard
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:guard2",
		Kind:        model.CreatureKindNPC,
		DisplayName: "경비병",
		Description: "경비병이 서 있다.",
		RoomID:      "room:plaza",
		Stats:       map[string]int{"alignment": -5},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		Description: "상인이 있다.",
		RoomID:      "room:plaza",
		Stats:       map[string]int{"alignment": 10},
	})

	got := dispatchLookLine(t, loaded, "봐")
	for _, want := range []string{
		"(x2) 경비병이 서 있다. (붉은 광채)\n",
		"상인이 있다. (푸른 광채)\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestLookRenderRoomShowsLegacyCreatureEnemyLines(t *testing.T) {
	loaded := lookWorld(t)
	got := dispatchLookLineWithRuntime(t, loaded, "봐", &Context{ActorID: "player:alice"}, func(world *state.World) {
		if _, err := world.AddEnemy("creature:guard", "player:bob"); err != nil {
			t.Fatalf("AddEnemy(bob) error = %v", err)
		}
		if _, err := world.AddEnemy("creature:guard", "creature:alice"); err != nil {
			t.Fatalf("AddEnemy(alice) error = %v", err)
		}
	})

	want := "경비병이 Bob와 싸우고 있습니다.\n"
	if !strings.Contains(got, want) {
		t.Fatalf("output missing %q:\n%s", want, got)
	}
	if strings.Contains(got, "매우 화가") || strings.Contains(got, "경비병이 당신과 싸우고 있습니다.") {
		t.Fatalf("output includes target-look enemy text in room look:\n%s", got)
	}
	objectLine := "금화가 놓여져 있습니다.\n"
	if strings.Index(got, objectLine) < 0 || strings.Index(got, want) < strings.Index(got, objectLine) {
		t.Fatalf("enemy line order = %q, want after room objects", got)
	}
}

func TestLookRenderRoomShowsLegacyCreatureEnemyLineForViewer(t *testing.T) {
	loaded := lookWorld(t)
	got := dispatchLookLineWithRuntime(t, loaded, "봐", &Context{ActorID: "player:alice"}, func(world *state.World) {
		if _, err := world.AddEnemy("creature:guard", "creature:alice"); err != nil {
			t.Fatalf("AddEnemy(alice) error = %v", err)
		}
	})

	want := "경비병이 당신과 싸우고 있습니다.\n"
	if !strings.Contains(got, want) {
		t.Fatalf("output missing %q:\n%s", want, got)
	}
}

func TestLookRenderRoomHidesTaggedInvisibleEntities(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:hidden",
		DisplayName: "Hidden",
		RoomID:      "room:plaza",
		Metadata:    model.Metadata{Tags: []string{"hidden"}},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:hidden",
		Kind:        model.CreatureKindNPC,
		DisplayName: "숨은 경비병",
		Description: "숨은 경비병이 있다.",
		RoomID:      "room:plaza",
		Metadata:    model.Metadata{Tags: []string{"hidden"}},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:hidden",
		DisplayName: "숨은 물건",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:hidden",
		PrototypeID: "prototype:hidden",
		Quantity:    1,
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
		Metadata:    model.Metadata{Tags: []string{"hidden"}},
	})
	room := loaded.Rooms["room:plaza"]
	room.PlayerIDs = append(room.PlayerIDs, "player:hidden")
	room.CreatureIDs = append(room.CreatureIDs, "creature:hidden")
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:hidden")
	loaded.Rooms[room.ID] = room

	got := dispatchLookLine(t, loaded, "봐")
	for _, hidden := range []string{"Hidden", "숨은 경비병", "숨은 물건"} {
		if strings.Contains(got, hidden) {
			t.Fatalf("output includes hidden entity %q:\n%s", hidden, got)
		}
	}
}

func TestLookRenderRoomHidesStatAndPropertyBackedInvisibleEntities(t *testing.T) {
	loaded := lookWorld(t)
	bob := loaded.Players["player:bob"]
	bob.CreatureID = "creature:bob"
	loaded.Players[bob.ID] = bob
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		Description: "조용히 ",
		PlayerID:    "player:bob",
		RoomID:      "room:plaza",
		Stats:       map[string]int{"PHIDDN": 1},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:property-hidden",
		Kind:        model.CreatureKindNPC,
		DisplayName: "숨은 경비병",
		Description: "숨은 경비병이 있다.",
		RoomID:      "room:plaza",
		Properties:  map[string]string{"flags": "MHIDDN|MINVIS"},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:property-hidden",
		DisplayName: "숨은 물건",
		Properties:  map[string]string{"flags": "OHIDDN"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:property-hidden",
		PrototypeID: "prototype:property-hidden",
		Quantity:    1,
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
	})
	room := loaded.Rooms["room:plaza"]
	room.PlayerIDs = append(room.PlayerIDs, "player:bob")
	room.CreatureIDs = append(room.CreatureIDs, "creature:property-hidden")
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:property-hidden")
	loaded.Rooms[room.ID] = room

	got := dispatchLookLine(t, loaded, "봐")
	for _, hidden := range []string{"Bob", "숨은 경비병", "숨은 물건"} {
		if strings.Contains(got, hidden) {
			t.Fatalf("output includes stat/property-backed hidden entity %q:\n%s", hidden, got)
		}
	}
}

func TestLookRenderRoomShowsPDMINVPlayersOnlyToDMViewer(t *testing.T) {
	loaded := lookWorld(t)
	bobPlayer := loaded.Players["player:bob"]
	bobPlayer.CreatureID = "creature:bob"
	loaded.Players[bobPlayer.ID] = bobPlayer
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		Description: "조용히 ",
		PlayerID:    "player:bob",
		RoomID:      "room:plaza",
		Metadata:    model.Metadata{Tags: []string{"PDMINV"}},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:dm",
		DisplayName: "DM",
		CreatureID:  "creature:dm",
		RoomID:      "room:plaza",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:dm",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "DM",
		PlayerID:    "player:dm",
		RoomID:      "room:plaza",
		Stats:       map[string]int{"class": model.ClassDM},
	})

	normal := dispatchLookLine(t, loaded, "봐")
	if strings.Contains(normal, "Bob") {
		t.Fatalf("non-DM output includes PDMINV player:\n%s", normal)
	}

	dmCompact := dispatchLookLineWithContext(t, loaded, "봐", &Context{ActorID: "player:dm"})
	if want := "Alice, Bob님이 서 있습니다.\n"; !strings.Contains(dmCompact, want) {
		t.Fatalf("DM output missing %q:\n%s", want, dmCompact)
	}

	dmCreature := loaded.Creatures["creature:dm"]
	dmCreature.Metadata.Tags = []string{"PDSCRP"}
	loaded.Creatures[dmCreature.ID] = dmCreature
	dmDetail := dispatchLookLineWithContext(t, loaded, "봐", &Context{ActorID: "player:dm"})
	if want := "Bob님이 조용히 있습니다.\n"; !strings.Contains(dmDetail, want) {
		t.Fatalf("DM PDSCRP output missing %q:\n%s", want, dmDetail)
	}
}

func TestLookRenderRoomHidesSceneryObjectsAndGroupsConsecutiveObjects(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:stone",
		DisplayName: "작은 돌",
	})
	for _, object := range []model.ObjectInstance{
		{
			ID:          "object:stone-1",
			PrototypeID: "prototype:stone",
			Quantity:    1,
			Location:    model.ObjectLocation{RoomID: "room:plaza"},
		},
		{
			ID:          "object:stone-2",
			PrototypeID: "prototype:stone",
			Quantity:    1,
			Location:    model.ObjectLocation{RoomID: "room:plaza"},
		},
		{
			ID:          "object:scenery-locker",
			PrototypeID: "prototype:stone",
			Quantity:    1,
			Location:    model.ObjectLocation{RoomID: "room:plaza"},
			Metadata:    model.Metadata{Tags: []string{"scenery"}},
		},
	} {
		mustAddLookObject(t, loaded, object)
	}
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:stone-1", "object:stone-2", "object:scenery-locker")
	loaded.Rooms[room.ID] = room

	got := dispatchLookLine(t, loaded, "봐")
	for _, want := range []string{
		"금화, (x2) 작은 돌이 놓여져 있습니다.\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "(x3) 작은 돌") {
		t.Fatalf("output grouped scenery object:\n%s", got)
	}
}

func TestLookRenderRoomShowsInvisibleObjectsWithDetectInvisible(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:invisible-gem",
		DisplayName: "투명한 보석",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:invisible-gem",
		PrototypeID: "prototype:invisible-gem",
		Quantity:    1,
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
		Metadata:    model.Metadata{Tags: []string{"OINVIS"}},
	})
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:invisible-gem")
	loaded.Rooms[room.ID] = room

	normal := dispatchLookLine(t, loaded, "봐")
	if strings.Contains(normal, "투명한 보석") {
		t.Fatalf("non-PDINVI output includes invisible object:\n%s", normal)
	}

	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "PDINVI")
	loaded.Creatures[alice.ID] = alice
	detect := dispatchLookLine(t, loaded, "봐")
	if want := "금화, 투명한 보석이 놓여져 있습니다.\n"; !strings.Contains(detect, want) {
		t.Fatalf("PDINVI output missing %q:\n%s", want, detect)
	}
}

func TestLookHandlerTargetInvisibleRoomObjectRequiresDetectInvisible(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:invisible-gem",
		DisplayName: "투명한 보석",
		Description: "투명한 보석이 희미하게 반짝인다.",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:invisible-gem",
		PrototypeID: "prototype:invisible-gem",
		Quantity:    1,
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
		Metadata:    model.Metadata{Tags: []string{"OINVIS"}},
	})
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:invisible-gem")
	loaded.Rooms[room.ID] = room

	if got := dispatchLookLine(t, loaded, "투명 봐"); got != "그런 건 보이지 않습니다.\n" {
		t.Fatalf("output = %q, want invisible room object hidden without PDINVI", got)
	}

	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "PDINVI")
	loaded.Creatures[alice.ID] = alice
	if got := dispatchLookLine(t, loaded, "투명 봐"); got != "투명한 보석이 희미하게 반짝인다.\n" {
		t.Fatalf("output = %q, want invisible room object description with PDINVI", got)
	}
}

func TestLookHandlerTargetPropertyBackedInvisibleRoomObjectRequiresDetectInvisible(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:property-invisible-gem",
		DisplayName: "투명한 보석",
		Description: "투명한 보석이 희미하게 반짝인다.",
		Properties:  map[string]string{"flags": "OINVIS"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:property-invisible-gem",
		PrototypeID: "prototype:property-invisible-gem",
		Quantity:    1,
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
	})
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:property-invisible-gem")
	loaded.Rooms[room.ID] = room

	if got := dispatchLookLine(t, loaded, "투명 봐"); got != "그런 건 보이지 않습니다.\n" {
		t.Fatalf("output = %q, want property-backed invisible room object hidden without PDINVI", got)
	}

	alice := loaded.Creatures["creature:alice"]
	alice.Properties = map[string]string{"flags": "PDINVI"}
	loaded.Creatures[alice.ID] = alice
	if got := dispatchLookLine(t, loaded, "투명 봐"); got != "투명한 보석이 희미하게 반짝인다.\n" {
		t.Fatalf("output = %q, want property-backed invisible room object visible with PDINVI", got)
	}
}

func TestLookHandlerRendersExitTargetRoomWithoutMovingActor(t *testing.T) {
	world := state.NewWorld(lookWorld(t))
	defer world.Close()
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "봐", Number: 2, Handler: "look"},
	})
	dispatcher := Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"look": NewLookHandler(world),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "동 봐")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got, want := ctx.OutputString(), "\n동쪽\n\n[ 출구 : 없음 ]\n"; got != want {
		t.Fatalf("output = %q, want destination room look", got)
	}

	player, ok := world.Player("player:alice")
	if !ok {
		t.Fatal("missing player")
	}
	if player.RoomID != "room:plaza" {
		t.Fatalf("player room id = %q, want room:plaza", player.RoomID)
	}
	creature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("missing creature")
	}
	if creature.RoomID != "room:plaza" {
		t.Fatalf("creature room id = %q, want room:plaza", creature.RoomID)
	}
}

func TestLookHandlerRendersSecondMatchingExitTarget(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookRoom(t, loaded, model.Room{ID: "room:east2", DisplayName: "동쪽 둘째"})
	room := loaded.Rooms["room:plaza"]
	room.Exits = append(room.Exits, model.Exit{Name: "동문", ToRoomID: "room:east2"})
	loaded.Rooms[room.ID] = room

	got := dispatchLookLine(t, loaded, "동 2 봐")
	if got != "\n동쪽 둘째\n\n[ 출구 : 없음 ]\n" {
		t.Fatalf("output = %q, want second matching exit room look", got)
	}
}

func TestLookHandlerShowsMagicObjectMarkerOnlyInRoomList(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:wand",
		DisplayName: "마법봉",
		Description: "마법봉이 놓여 있다.",
		Properties:  map[string]string{"magicPower": "11", "charges": "3"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:wand",
		PrototypeID: "prototype:wand",
		Quantity:    1,
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
	})
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:wand")
	loaded.Rooms[room.ID] = room

	out := dispatchLookLine(t, loaded, "봐")
	for _, hidden := range []string{"마법봉(주문)", "마법 힘", "남은 충전"} {
		if strings.Contains(out, hidden) {
			t.Fatalf("normal look output includes magic detail %q:\n%s", hidden, out)
		}
	}
	if target := dispatchLookLine(t, loaded, "마법봉 봐"); strings.Contains(target, "마법 힘") {
		t.Fatalf("normal object look includes magic detail:\n%s", target)
	}

	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "detectMagic")
	loaded.Creatures[creature.ID] = creature

	out = dispatchLookLine(t, loaded, "봐")
	if !strings.Contains(out, "마법봉(주문)") {
		t.Fatalf("detect magic room look missing magic marker:\n%s", out)
	}
	if target := dispatchLookLine(t, loaded, "마법봉 봐"); target != "마법봉이 놓여 있다.\n" {
		t.Fatalf("detect magic object look = %q, want C-style description only", target)
	}
}

func TestLookHandlerRendersLegacyObjectAlignmentAuraWithKnowAlignment(t *testing.T) {
	loaded := lookWorld(t)
	for _, object := range []model.ObjectInstance{
		{
			ID:          "object:good",
			PrototypeID: "prototype:good",
			Location:    model.ObjectLocation{RoomID: "room:plaza"},
			Metadata:    model.Metadata{Tags: []string{"OGOODO"}},
		},
		{
			ID:          "object:evil",
			PrototypeID: "prototype:evil",
			Location:    model.ObjectLocation{RoomID: "room:plaza"},
			Metadata:    model.Metadata{Tags: []string{"OEVILO"}},
		},
	} {
		mustAddLookObject(t, loaded, object)
	}
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:good",
		DisplayName: "푸른검",
		Description: "푸른 검이다.",
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:evil",
		DisplayName: "붉은검",
		Description: "붉은 검이다.",
	})
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:good", "object:evil")
	loaded.Rooms[room.ID] = room

	if got := dispatchLookLine(t, loaded, "푸른검 봐"); got != "푸른 검이다.\n" {
		t.Fatalf("without PKNOWA output = %q, want no aura", got)
	}
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "PKNOWA")
	loaded.Creatures[alice.ID] = alice

	if got, want := dispatchLookLine(t, loaded, "푸른검 봐"), "푸른 검이다.\n푸른 광채가 뻗어 나오고 있습니다.\n"; got != want {
		t.Fatalf("good output = %q, want %q", got, want)
	}
	if got, want := dispatchLookLine(t, loaded, "붉은검 봐"), "붉은 검이다.\n붉은 광채가 뻗어 나오고 있습니다.\n"; got != want {
		t.Fatalf("evil output = %q, want %q", got, want)
	}
}

func TestLookHandlerRendersLegacyWeaponTypeDetails(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:blade",
		DisplayName: "검",
		Description: "검은 잘 관리되어 있다.",
		Properties: map[string]string{
			"type":         "1",
			"shotsCurrent": "5",
			"shotsMax":     "20",
		},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:blade",
		PrototypeID: "prototype:blade",
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
	})
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:blade")
	loaded.Rooms[room.ID] = room

	want := "검은 잘 관리되어 있다.\n검은 매우 공격적인 '검'입니다.\n"
	if got := dispatchLookLine(t, loaded, "검 봐"); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestLookHandlerRendersLegacyDurabilityWarnings(t *testing.T) {
	tests := []struct {
		name       string
		properties map[string]string
		want       string
	}{
		{
			name: "broken",
			properties: map[string]string{
				"type":         "5",
				"shotsCurrent": "0",
				"shotsMax":     "10",
			},
			want: "낡은 갑옷이다.\n그것은 부서져 버렸거나 다 써버렸습니다.\n",
		},
		{
			name: "nearly broken",
			properties: map[string]string{
				"type":         "11",
				"shotsCurrent": "1",
				"shotsMax":     "10",
			},
			want: "녹슨 열쇠다.\n그것은 곧 부서질것 같습니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := lookWorld(t)
			description := "낡은 갑옷이다."
			displayName := "낡은 갑옷"
			target := "낡은"
			if tt.name == "nearly broken" {
				description = "녹슨 열쇠다."
				displayName = "녹슨 열쇠"
				target = "녹슨"
			}
			mustAddLookPrototype(t, loaded, model.ObjectPrototype{
				ID:          "prototype:durable",
				DisplayName: displayName,
				Description: description,
				Properties:  tt.properties,
			})
			mustAddLookObject(t, loaded, model.ObjectInstance{
				ID:          "object:durable",
				PrototypeID: "prototype:durable",
				Location:    model.ObjectLocation{RoomID: "room:plaza"},
			})
			room := loaded.Rooms["room:plaza"]
			room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:durable")
			loaded.Rooms[room.ID] = room

			if got := dispatchLookLine(t, loaded, target+" 봐"); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLookHandlerDetectMagicSplitsAdjustedObjectGroups(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:blade",
		DisplayName: "단검",
	})
	for _, object := range []model.ObjectInstance{
		{
			ID:          "object:blade-1",
			PrototypeID: "prototype:blade",
			Quantity:    1,
			Location:    model.ObjectLocation{RoomID: "room:plaza"},
			Properties:  map[string]string{"adjustment": "1"},
		},
		{
			ID:          "object:blade-2",
			PrototypeID: "prototype:blade",
			Quantity:    1,
			Location:    model.ObjectLocation{RoomID: "room:plaza"},
			Properties:  map[string]string{"adjustment": "2"},
		},
	} {
		mustAddLookObject(t, loaded, object)
	}
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:blade-1", "object:blade-2")
	loaded.Rooms[room.ID] = room

	out := dispatchLookLine(t, loaded, "봐")
	if !strings.Contains(out, "(x2) 단검") {
		t.Fatalf("normal room look did not group adjusted objects:\n%s", out)
	}

	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "PDMAGI")
	loaded.Creatures[creature.ID] = creature

	out = dispatchLookLine(t, loaded, "봐")
	for _, want := range []string{"단검(+1)", "단검(+2)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("detect magic room look missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "(x2) 단검") {
		t.Fatalf("detect magic room look grouped adjusted objects:\n%s", out)
	}
}

func TestLookHandlerDetectMagicGroupsSameAdjustedMagicPowerObjects(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:scroll",
		DisplayName: "주문서",
	})
	for _, object := range []model.ObjectInstance{
		{
			ID:          "object:scroll-plain",
			PrototypeID: "prototype:scroll",
			Quantity:    1,
			Location:    model.ObjectLocation{RoomID: "room:plaza"},
		},
		{
			ID:          "object:scroll-magic",
			PrototypeID: "prototype:scroll",
			Quantity:    1,
			Location:    model.ObjectLocation{RoomID: "room:plaza"},
			Properties:  map[string]string{"magicPower": "11"},
		},
	} {
		mustAddLookObject(t, loaded, object)
	}
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:scroll-plain", "object:scroll-magic")
	loaded.Rooms[room.ID] = room

	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "PDMAGI")
	loaded.Creatures[creature.ID] = creature

	out := dispatchLookLine(t, loaded, "봐")
	if want := "(x2) 주문서(주문)"; !strings.Contains(out, want) {
		t.Fatalf("detect magic room look missing grouped magicpower object %q:\n%s", want, out)
	}
	if strings.Contains(out, "주문서, 주문서(주문)") {
		t.Fatalf("detect magic room look split same-adjustment magicpower objects:\n%s", out)
	}
}

func TestLookHandlerExitTargetBeatsSameNamedRoomObject(t *testing.T) {
	got := dispatchLookLine(t, exitObjectConflictLookWorld(t), "동 봐")
	if got != "\n동쪽\n\n[ 출구 : 없음 ]\n" {
		t.Fatalf("output = %q, want exit target room look", got)
	}
	if strings.Contains(got, "동쪽 표지판이다.") {
		t.Fatalf("output rendered same-named object instead of exit:\n%s", got)
	}
}

func TestLookHandlerClosedExitTargetDoesNotFallbackToSameNamedObject(t *testing.T) {
	loaded := exitObjectConflictLookWorld(t)
	room := loaded.Rooms["room:plaza"]
	room.Exits[0].Flags = []string{"closed"}
	loaded.Rooms[room.ID] = room

	got := dispatchLookLine(t, loaded, "동 봐")
	if got != "그 출구는 닫혀 있습니다." {
		t.Fatalf("output = %q, want closed-exit message", got)
	}
	if strings.Contains(got, "동쪽 표지판이다.") {
		t.Fatalf("output fell back to same-named object:\n%s", got)
	}
}

func TestLookHandlerExitTargetUsesLegacyMissingMapMessage(t *testing.T) {
	loaded := lookWorld(t)
	room := loaded.Rooms["room:plaza"]
	room.Exits = append(room.Exits, model.Exit{Name: "북", ToRoomID: "room:missing"})
	loaded.Rooms[room.ID] = room

	if got := dispatchLookLine(t, loaded, "북 봐"); got != "지도가 없습니다." {
		t.Fatalf("output = %q, want legacy missing-map message", got)
	}
}

func TestLookHandlerExitTargetUsesLegacyBlockedRoomMessage(t *testing.T) {
	loaded := lookWorld(t)
	room := loaded.Rooms["room:plaza"]
	room.Exits = append(room.Exits, model.Exit{Name: "혼인", ToRoomID: "room:married"})
	loaded.Rooms[room.ID] = room
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:married",
		DisplayName: "혼인방",
		Metadata:    model.Metadata{Tags: []string{"RONMAR"}},
	})

	if got := dispatchLookLine(t, loaded, "혼인 봐"); got != "그 방은 볼 수가 없습니다." {
		t.Fatalf("output = %q, want legacy blocked-room message", got)
	}
}

func TestLookHandlerRoomLookHidesNonVisibleExits(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookRoom(t, loaded, model.Room{ID: "room:secret", DisplayName: "비밀방"})
	room := loaded.Rooms["room:plaza"]
	room.Exits = append(room.Exits,
		model.Exit{Name: "비밀", ToRoomID: "room:secret", Flags: []string{"secret"}},
		model.Exit{Name: "숨김", ToRoomID: "room:secret", Flags: []string{"noSee"}},
		model.Exit{Name: "은신", ToRoomID: "room:secret", Flags: []string{"invisible"}},
	)
	loaded.Rooms[room.ID] = room

	got := dispatchLookLine(t, loaded, "봐")
	if !strings.Contains(got, "[ 출구 : 동, 서 ]\n") {
		t.Fatalf("output missing visible exit list:\n%s", got)
	}
	for _, hidden := range []string{"비밀", "숨김", "은신"} {
		if strings.Contains(got, hidden) {
			t.Fatalf("output includes hidden exit %q:\n%s", hidden, got)
		}
	}

	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "PDINVI")
	loaded.Creatures[alice.ID] = alice
	got = dispatchLookLine(t, loaded, "봐")
	if !strings.Contains(got, "[ 출구 : 동, 서, 은신 ]\n") {
		t.Fatalf("PDINVI output missing invisible exit:\n%s", got)
	}
	for _, hidden := range []string{"비밀", "숨김"} {
		if strings.Contains(got, hidden) {
			t.Fatalf("PDINVI output includes non-target-list exit %q:\n%s", hidden, got)
		}
	}
}

func TestLookHandlerSecretExitCanBeTargetLooked(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookRoom(t, loaded, model.Room{ID: "room:secret", DisplayName: "비밀방"})
	room := loaded.Rooms["room:plaza"]
	room.Exits = append(room.Exits, model.Exit{Name: "비밀", ToRoomID: "room:secret", Flags: []string{"secret"}})
	loaded.Rooms[room.ID] = room

	got := dispatchLookLine(t, loaded, "비 봐")
	if got != "\n비밀방\n\n[ 출구 : 없음 ]\n" {
		t.Fatalf("output = %q, want secret exit target room look", got)
	}
}

func TestLookHandlerInvisibleExitTargetRequiresDetectInvisible(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookRoom(t, loaded, model.Room{ID: "room:invisible", DisplayName: "은신방"})
	room := loaded.Rooms["room:plaza"]
	room.Exits = append(room.Exits, model.Exit{Name: "은신", ToRoomID: "room:invisible", Flags: []string{"XINVIS"}})
	loaded.Rooms[room.ID] = room

	if got := dispatchLookLine(t, loaded, "은 봐"); got != "그런 건 보이지 않습니다.\n" {
		t.Fatalf("output = %q, want invisible exit target hidden without PDINVI", got)
	}

	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "PDINVI")
	loaded.Creatures[alice.ID] = alice
	if got := dispatchLookLine(t, loaded, "은 봐"); got != "\n은신방\n\n[ 출구 : 없음 ]\n" {
		t.Fatalf("output = %q, want invisible exit target room look with PDINVI", got)
	}
}

func TestLookHandlerRendersInventoryObjectByUTF8KeyPrefix(t *testing.T) {
	if got := dispatchLookLine(t, targetLookWorld(t), "부 봐"); got != "은빛 부적은 따뜻하다.\n" {
		t.Fatalf("output = %q, want inventory object look", got)
	}
}

func TestLookHandlerUsesLegacyEquipmentBeforeInventoryForImplicitObjectOrdinal(t *testing.T) {
	loaded := targetLookWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Equipment = map[string]model.ObjectInstanceID{"held": "object:held-talisman"}
	loaded.Creatures[creature.ID] = creature
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:held-talisman",
		DisplayName: "은빛 목걸이",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:held-talisman",
		PrototypeID: "prototype:held-talisman",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "held"},
		Properties: map[string]string{
			"description": "장착한 부적이다.",
			"key[0]":      "부적",
		},
	})

	if got := dispatchLookLine(t, loaded, "부 봐"); got != "장착한 부적이다.\n" {
		t.Fatalf("implicit ordinal output = %q, want equipped object", got)
	}
	if got := dispatchLookLine(t, loaded, "부 1 봐"); got != "은빛 부적은 따뜻하다.\n" {
		t.Fatalf("explicit ordinal output = %q, want inventory object", got)
	}
}

func TestLookHandlerRendersContainerContents(t *testing.T) {
	want := "낡은 가방이다.\n내용물: 붉은 보석.\n"
	if got := dispatchLookLine(t, targetLookWorld(t), "가 봐"); got != want {
		t.Fatalf("output = %q, want container look", got)
	}
}

func TestLookHandlerGroupsContainerContents(t *testing.T) {
	loaded := targetLookWorld(t)
	bag := loaded.Objects["object:bag"]
	bag.Contents.ObjectIDs = append(bag.Contents.ObjectIDs, "object:gem2", "object:scenery-gem")
	loaded.Objects[bag.ID] = bag
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:gem2",
		PrototypeID: "prototype:gem",
		Quantity:    1,
		Location:    model.ObjectLocation{ContainerID: "object:bag"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:scenery-gem",
		PrototypeID: "prototype:gem",
		Quantity:    1,
		Location:    model.ObjectLocation{ContainerID: "object:bag"},
		Metadata:    model.Metadata{Tags: []string{"scenery"}},
	})

	want := "낡은 가방이다.\n내용물: (x2) 붉은 보석.\n"
	if got := dispatchLookLine(t, loaded, "가 봐"); got != want {
		t.Fatalf("output = %q, want grouped container look", got)
	}
}

func TestLookHandlerContainerContentsHonorDetectInvisible(t *testing.T) {
	loaded := targetLookWorld(t)
	bag := loaded.Objects["object:bag"]
	bag.Contents.ObjectIDs = append(bag.Contents.ObjectIDs, "object:hidden-gem")
	loaded.Objects[bag.ID] = bag
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:hidden-gem",
		PrototypeID: "prototype:gem",
		Quantity:    1,
		Location:    model.ObjectLocation{ContainerID: "object:bag"},
		Metadata:    model.Metadata{Tags: []string{"OINVIS"}},
	})

	normal := dispatchLookLine(t, loaded, "가 봐")
	if normal != "낡은 가방이다.\n내용물: 붉은 보석.\n" {
		t.Fatalf("normal output = %q, want invisible container child hidden", normal)
	}

	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "PDINVI")
	loaded.Creatures[alice.ID] = alice
	detect := dispatchLookLine(t, loaded, "가 봐")
	if detect != "낡은 가방이다.\n내용물: (x2) 붉은 보석.\n" {
		t.Fatalf("PDINVI output = %q, want invisible container child listed", detect)
	}
}

func TestLookHandlerMultiWordTargetUsesLastTargetOrdinalAndSkipsStaleRefs(t *testing.T) {
	loaded := lookWorld(t)
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:small-stone",
		DisplayName: "작은 돌",
	})
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs,
		"object:stale-small-stone",
		"object:small-stone-1",
		"object:small-stone-2",
	)
	loaded.Rooms[room.ID] = room
	for _, object := range []model.ObjectInstance{
		{
			ID:                  "object:stale-small-stone",
			PrototypeID:         "prototype:small-stone",
			DisplayNameOverride: "작은 돌",
			Quantity:            1,
			Location:            model.ObjectLocation{RoomID: "room:east"},
			Properties:          map[string]string{"description": "먼 방의 작은 돌이다."},
		},
		{
			ID:                  "object:small-stone-1",
			PrototypeID:         "prototype:small-stone",
			DisplayNameOverride: "작은 돌",
			Quantity:            1,
			Location:            model.ObjectLocation{RoomID: "room:plaza"},
			Properties:          map[string]string{"description": "첫 번째 작은 돌이다."},
		},
		{
			ID:                  "object:small-stone-2",
			PrototypeID:         "prototype:small-stone",
			DisplayNameOverride: "작은 돌",
			Quantity:            1,
			Location:            model.ObjectLocation{RoomID: "room:plaza"},
			Properties:          map[string]string{"description": "두 번째 작은 돌이다."},
		},
	} {
		mustAddLookObject(t, loaded, object)
	}

	got := dispatchLookLine(t, loaded, "작은 돌 2 봐")
	if got != "두 번째 작은 돌이다.\n" {
		t.Fatalf("output = %q, want second visible small stone", got)
	}
	if strings.Contains(got, "먼 방") || strings.Contains(got, "첫 번째") {
		t.Fatalf("output used stale or wrong ordinal object:\n%s", got)
	}
}

func TestLookHandlerRendersPlayerTargetByDisplayNamePrefix(t *testing.T) {
	if got := dispatchLookLine(t, lookWorld(t), "Bo 봐"); got != "당신은 Bob를 봅니다.\nBob님이 있습니다.\n" {
		t.Fatalf("output = %q, want player target look", got)
	}
}

func TestLookHandlerUsesLegacyPlayerTargetUpperFirstNormalization(t *testing.T) {
	if got := dispatchLookLine(t, lookWorld(t), "bo 봐"); got != "당신은 Bob를 봅니다.\nBob님이 있습니다.\n" {
		t.Fatalf("output = %q, want lowercase player prefix to match legacy up()", got)
	}
}

func TestLookHandlerRejectsPlayerAccountNameTargetLikeLegacyFindCrt(t *testing.T) {
	loaded := lookWorld(t)
	bob := loaded.Players["player:bob"]
	bob.AccountName = "Bobaccount"
	loaded.Players[bob.ID] = bob

	if got := dispatchLookLine(t, loaded, "bobaccount 봐"); got != "그런 건 보이지 않습니다.\n" {
		t.Fatalf("output = %q, want C find_crt miss for account-name alias", got)
	}
}

func TestLookHandlerMatchesPlayerCreatureKeyLikeLegacyFindCrt(t *testing.T) {
	loaded := lookWorld(t)
	bob := loaded.Players["player:bob"]
	bob.CreatureID = "creature:bob"
	loaded.Players[bob.ID] = bob
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:plaza",
		Properties:  map[string]string{"key[0]": "친구"},
	})

	if got := dispatchLookLine(t, loaded, "친구 봐"); got != "당신은 Bob를 봅니다.\n그녀는 있습니다.\n" {
		t.Fatalf("output = %q, want player target look through creature key", got)
	}
}

func TestLookHandlerRendersCreatureTargetByDisplayNamePrefix(t *testing.T) {
	want := "당신은 경비병을 봅니다.\n경비병이 서 있다.\n그녀는 당신과 꼭 맞는 상대입니다!\n"
	if got := dispatchLookLine(t, lookWorld(t), "경 봐"); got != want {
		t.Fatalf("output = %q, want creature target look", got)
	}
}

func TestLookHandlerRejectsCreatureIDTargetLikeLegacyFindCrt(t *testing.T) {
	if got := dispatchLookLine(t, lookWorld(t), "creature:guard 봐"); got != "그런 건 보이지 않습니다.\n" {
		t.Fatalf("output = %q, want C find_crt miss for creature ID alias", got)
	}
}

func TestLookHandlerBroadcastsLegacyCreatureAndPlayerTargetLook(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantText  string
		wantBroad string
	}{
		{
			name:      "creature",
			line:      "경 봐",
			wantText:  "당신은 경비병을 봅니다.\n경비병이 서 있다.\n그녀는 당신과 꼭 맞는 상대입니다!\n",
			wantBroad: "\nAlice가 경비병을 봅니다.",
		},
		{
			name:      "player",
			line:      "Bo 봐",
			wantText:  "당신은 Bob를 봅니다.\nBob님이 있습니다.\n",
			wantBroad: "\nAlice가 Bob를 봅니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var broadcasts []roomBroadcastRecord
			ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
			if got := dispatchLookLineWithContext(t, lookWorld(t), tt.line, ctx); got != tt.wantText {
				t.Fatalf("output = %q, want %q", got, tt.wantText)
			}
			if len(broadcasts) != 1 {
				t.Fatalf("len(broadcasts) = %d, want 1: %+v", len(broadcasts), broadcasts)
			}
			want := roomBroadcastRecord{RoomID: "room:plaza", Exclude: "session:alice", Text: tt.wantBroad}
			if broadcasts[0] != want {
				t.Fatalf("broadcast = %+v, want %+v", broadcasts[0], want)
			}
		})
	}
}

func TestLookHandlerRendersLegacySelfTargetLook(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Description = "서 "
	alice.Stats = map[string]int{"PMALES": 1}
	loaded.Creatures[alice.ID] = alice

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	want := "당신은 거울을 들고 자신을 봅니다.\n" +
		"그는 서 있습니다.\n" +
		"그는 당신과 꼭 맞는 상대입니다!\n"
	if got := dispatchLookLineWithContext(t, loaded, "나 봐", ctx); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	wantBroadcast := roomBroadcastRecord{
		RoomID:  "room:plaza",
		Exclude: "session:alice",
		Text:    "\nAlice가 거울을 들고 자신을 바라 봅니다.",
	}
	if len(broadcasts) != 1 || broadcasts[0] != wantBroadcast {
		t.Fatalf("broadcasts = %+v, want [%+v]", broadcasts, wantBroadcast)
	}
}

func TestLookHandlerRendersLegacyCreatureConsiderDetails(t *testing.T) {
	tests := []struct {
		name        string
		viewerLevel int
		targetLevel int
		want        string
	}{
		{name: "even", viewerLevel: 8, targetLevel: 8, want: "그녀는 당신과 꼭 맞는 상대입니다!\n"},
		{name: "easy clamp", viewerLevel: 24, targetLevel: 1, want: "그녀는 한방에 보낼수 있습니다.\n"},
		{name: "hard clamp", viewerLevel: 1, targetLevel: 24, want: "그녀는 보자마자 도망가는것이 좋을겁니다.\n"},
		{name: "slightly hard", viewerLevel: 4, targetLevel: 8, want: "그녀는 운이 좋으면 이길 수 있습니다..\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := lookWorld(t)
			alice := loaded.Creatures["creature:alice"]
			alice.Level = tt.viewerLevel
			loaded.Creatures[alice.ID] = alice
			guard := loaded.Creatures["creature:guard"]
			guard.Level = tt.targetLevel
			loaded.Creatures[guard.ID] = guard

			want := "당신은 경비병을 봅니다.\n경비병이 서 있다.\n" + tt.want
			if got := dispatchLookLine(t, loaded, "경 봐"); got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestLookHandlerRendersLegacyCreatureEnemyDetails(t *testing.T) {
	loaded := lookWorld(t)
	want := "당신은 경비병을 봅니다.\n" +
		"경비병이 서 있다.\n" +
		"그녀는 당신에게 매우 화가 난것 같습니다.\n" +
		"그녀는 Bob와 싸우고 있습니다.\n" +
		"그녀는 당신과 꼭 맞는 상대입니다!\n"

	got := dispatchLookLineWithRuntime(t, loaded, "경 봐", &Context{ActorID: "player:alice"}, func(world *state.World) {
		if _, err := world.AddEnemy("creature:guard", "player:bob"); err != nil {
			t.Fatalf("AddEnemy(bob) error = %v", err)
		}
		if _, err := world.AddEnemy("creature:guard", "creature:alice"); err != nil {
			t.Fatalf("AddEnemy(alice) error = %v", err)
		}
	})
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestLookHandlerRendersLegacyCreatureEquipmentList(t *testing.T) {
	loaded := lookWorld(t)
	guard := loaded.Creatures["creature:guard"]
	guard.Equipment = map[string]model.ObjectInstanceID{
		"wield": "object:guard-spear",
		"body":  "object:guard-armor",
	}
	loaded.Creatures[guard.ID] = guard

	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:guard-spear",
		DisplayName: "창",
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:guard-armor",
		DisplayName: "갑옷",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:guard-spear",
		PrototypeID: "prototype:guard-spear",
		Location:    model.ObjectLocation{CreatureID: "creature:guard", Slot: "wield"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:guard-armor",
		PrototypeID: "prototype:guard-armor",
		Location:    model.ObjectLocation{CreatureID: "creature:guard", Slot: "body"},
	})

	want := "당신은 경비병을 봅니다.\n" +
		"경비병이 서 있다.\n" +
		"그녀는 당신과 꼭 맞는 상대입니다!\n" +
		"[  몸  ]  갑옷\n" +
		"[ 무기 ]  창\n"
	if got := dispatchLookLine(t, loaded, "경 봐"); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestLookHandlerRendersLegacyCreatureWoundDetails(t *testing.T) {
	tests := []struct {
		name string
		hp   int
		want string
	}{
		{name: "light", hp: 85, want: "그는 가벼운 상처를 입었습니다.\n"},
		{name: "several", hp: 70, want: "그는 여러군데 상처를 입었습니다.\n"},
		{name: "many", hp: 50, want: "그는 많은 상처를 입었습니다.\n"},
		{name: "serious", hp: 30, want: "그는 심각한 상처를 입었습니다.\n"},
		{name: "near death", hp: 10, want: "그는 죽기 직전입니다.\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := lookWorld(t)
			guard := loaded.Creatures["creature:guard"]
			guard.Metadata.Tags = append(guard.Metadata.Tags, "MMALES")
			guard.Stats = map[string]int{"hpCurrent": tt.hp, "hpMax": 100}
			loaded.Creatures[guard.ID] = guard

			want := "당신은 경비병을 봅니다.\n경비병이 서 있다.\n" + tt.want + "그는 당신과 꼭 맞는 상대입니다!\n"
			if got := dispatchLookLine(t, loaded, "경 봐"); got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestLookHandlerRendersLegacyCreatureAlignmentAuraWithKnowAlignment(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "PKNOWA")
	loaded.Creatures[alice.ID] = alice
	guard := loaded.Creatures["creature:guard"]
	guard.Metadata.Tags = append(guard.Metadata.Tags, "MMALES")
	guard.Stats = map[string]int{"alignment": -50}
	loaded.Creatures[guard.ID] = guard

	want := "당신은 경비병을 봅니다.\n" +
		"경비병이 서 있다.\n" +
		"그에게서 붉은 광채가 뻗어 나오고 있습니다.\n" +
		"그는 당신과 꼭 맞는 상대입니다!\n"
	if got := dispatchLookLine(t, loaded, "경 봐"); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestLookHandlerRendersLegacyPlayerWoundDetails(t *testing.T) {
	loaded := lookWorld(t)
	player := loaded.Players["player:bob"]
	player.CreatureID = "creature:bob"
	loaded.Players[player.ID] = player
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:plaza",
		Description: "서 ",
		Stats:       map[string]int{"PMALES": 1, "hpCurrent": 20, "hpMax": 100},
	})

	want := "당신은 Bob를 봅니다.\n그는 서 있습니다.\n그는 가벼운 상처를 입었습니다.\n"
	if got := dispatchLookLine(t, loaded, "Bo 봐"); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestLookHandlerRendersLegacyPlayerEquipmentList(t *testing.T) {
	loaded := lookWorld(t)
	player := loaded.Players["player:bob"]
	player.CreatureID = "creature:bob"
	loaded.Players[player.ID] = player
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:plaza",
		Description: "서 ",
		Equipment: map[string]model.ObjectInstanceID{
			"held": "object:bob-charm",
			"head": "object:bob-hat",
		},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:bob-charm",
		DisplayName: "부적",
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:bob-hat",
		DisplayName: "모자",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:bob-charm",
		PrototypeID: "prototype:bob-charm",
		Location:    model.ObjectLocation{CreatureID: "creature:bob", Slot: "held"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:bob-hat",
		PrototypeID: "prototype:bob-hat",
		Location:    model.ObjectLocation{CreatureID: "creature:bob", Slot: "head"},
	})

	want := "당신은 Bob를 봅니다.\n" +
		"그녀는 서 있습니다.\n" +
		"[ 머리 ]  모자\n" +
		"[쥔물건]  부적\n"
	if got := dispatchLookLine(t, loaded, "Bo 봐"); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestLookHandlerRendersLegacyPlayerAlignmentAuraWithKnowAlignment(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "PKNOWA")
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:bob"]
	player.CreatureID = "creature:bob"
	loaded.Players[player.ID] = player
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:plaza",
		Description: "서 ",
		Stats:       map[string]int{"PMALES": 1, "alignment": 0},
	})

	want := "당신은 Bob를 봅니다.\n그는 서 있습니다.\n그에게서 푸른 광채가 뻗어 나오고 있습니다.\n"
	if got := dispatchLookLine(t, loaded, "Bo 봐"); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func dispatchLookLine(t *testing.T, loaded *worldload.World, line string) string {
	t.Helper()
	return dispatchLookLineWithContext(t, loaded, line, &Context{ActorID: "player:alice"})
}

func dispatchLookLineWithContext(t *testing.T, loaded *worldload.World, line string, ctx *Context) string {
	t.Helper()
	return dispatchLookLineWithRuntime(t, loaded, line, ctx, nil)
}

func dispatchLookLineWithRuntime(
	t *testing.T,
	loaded *worldload.World,
	line string,
	ctx *Context,
	setup func(*state.World),
) string {
	t.Helper()
	world := state.NewWorld(loaded)
	defer world.Close()
	if setup != nil {
		setup(world)
	}
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "봐", Number: 2, Handler: "look"},
		{Name: "보다", Number: 2, Handler: "look"},
		{Name: "조사", Number: 2, Handler: "look"},
	})
	dispatcher := Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"look": NewLookHandler(world),
		},
	}

	status, err := dispatcher.DispatchLine(ctx, line)
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	return ctx.OutputString()
}

func lookWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:               "room:plaza",
		DisplayName:      "광장",
		ShortDescription: "넓은 광장이다.",
		LongDescription:  "사람들이 오가는 오래된 광장이다.",
		Exits: []model.Exit{
			{Name: "동", ToRoomID: "room:east"},
			{Name: "서", ToRoomID: "room:west"},
		},
		Objects: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:coin", "object:stale"}},
	})
	mustAddLookRoom(t, loaded, model.Room{ID: "room:east", DisplayName: "동쪽"})
	mustAddLookRoom(t, loaded, model.Room{ID: "room:west", DisplayName: "서쪽"})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:plaza",
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		RoomID:      "room:plaza",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:plaza",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:guard",
		Kind:        model.CreatureKindNPC,
		DisplayName: "경비병",
		Description: "경비병이 서 있다.",
		RoomID:      "room:plaza",
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:coin",
		DisplayName: "금화",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:coin",
		PrototypeID: "prototype:coin",
		Quantity:    1,
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:                  "object:stale",
		PrototypeID:         "prototype:coin",
		DisplayNameOverride: "먼 방 물건",
		Quantity:            1,
		Location:            model.ObjectLocation{RoomID: "room:east"},
	})
	return loaded
}

func targetLookWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := lookWorld(t)
	coinProto := loaded.ObjectPrototypes["prototype:coin"]
	coinProto.Description = "작은 금화가 반짝인다."
	loaded.ObjectPrototypes[coinProto.ID] = coinProto

	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:bag")
	loaded.Rooms[room.ID] = room

	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:talisman")
	loaded.Creatures[creature.ID] = creature

	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:talisman",
		DisplayName: "은빛 목걸이",
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:bag",
		Kind:        model.ObjectKindContainer,
		DisplayName: "가방",
		Description: "낡은 가방이다.",
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:gem",
		DisplayName: "붉은 보석",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:talisman",
		PrototypeID: "prototype:talisman",
		Quantity:    1,
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties: map[string]string{
			"description": "은빛 부적은 따뜻하다.",
			"key[0]":      "부적",
		},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:bag",
		PrototypeID: "prototype:bag",
		Quantity:    1,
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
		Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:gem"}},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:gem",
		PrototypeID: "prototype:gem",
		Quantity:    1,
		Location:    model.ObjectLocation{ContainerID: "object:bag"},
	})
	return loaded
}

func exitObjectConflictLookWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := lookWorld(t)
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:east-sign")
	loaded.Rooms[room.ID] = room

	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:east-sign",
		DisplayName: "동",
		Description: "동쪽 표지판이다.",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:east-sign",
		PrototypeID: "prototype:east-sign",
		Quantity:    1,
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
	})
	return loaded
}

func mustAddLookRoom(t *testing.T, world *worldload.World, room model.Room) {
	t.Helper()
	if err := world.AddRoom(room); err != nil {
		t.Fatal(err)
	}
}

func mustAddLookPlayer(t *testing.T, world *worldload.World, player model.Player) {
	t.Helper()
	if err := world.AddPlayer(player); err != nil {
		t.Fatal(err)
	}
}

func mustAddLookCreature(t *testing.T, world *worldload.World, creature model.Creature) {
	t.Helper()
	if err := world.AddCreature(creature); err != nil {
		t.Fatal(err)
	}
}

func mustAddLookPrototype(t *testing.T, world *worldload.World, proto model.ObjectPrototype) {
	t.Helper()
	if err := world.AddObjectPrototype(proto); err != nil {
		t.Fatal(err)
	}
}

func mustAddLookObject(t *testing.T, world *worldload.World, object model.ObjectInstance) {
	t.Helper()
	if err := world.AddObjectInstance(object); err != nil {
		t.Fatal(err)
	}
}

func addLookEquippedLight(t *testing.T, loaded *worldload.World, charges string) {
	t.Helper()
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:lantern",
		Kind:        model.ObjectKindLightSource,
		DisplayName: "등불",
		Metadata:    model.Metadata{Tags: []string{"OLIGHT"}},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:lantern",
		PrototypeID: "prototype:lantern",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "held"},
		Properties: map[string]string{
			"shotsCurrent": charges,
		},
	})
	alice := loaded.Creatures["creature:alice"]
	alice.Equipment = map[string]model.ObjectInstanceID{"held": "object:lantern"}
	loaded.Creatures[alice.ID] = alice
}
