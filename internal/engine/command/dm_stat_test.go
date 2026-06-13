package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type dmStatTestWorld struct {
	players          map[model.PlayerID]model.Player
	creatures        map[model.CreatureID]model.Creature
	rooms            map[model.RoomID]model.Room
	objects          map[model.ObjectInstanceID]model.ObjectInstance
	objectPrototypes map[model.PrototypeID]model.ObjectPrototype
}

func (w dmStatTestWorld) Player(id model.PlayerID) (model.Player, bool) {
	player, ok := w.players[id]
	return player, ok
}

func (w dmStatTestWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	creature, ok := w.creatures[id]
	return creature, ok
}

func (w dmStatTestWorld) Room(id model.RoomID) (model.Room, bool) {
	room, ok := w.rooms[id]
	return room, ok
}

func (w dmStatTestWorld) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	obj, ok := w.objects[id]
	return obj, ok
}

func (w dmStatTestWorld) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	proto, ok := w.objectPrototypes[id]
	return proto, ok
}

func (w dmStatTestWorld) FindPlayerByName(name string) (model.Player, bool) {
	for _, p := range w.players {
		if strings.EqualFold(p.DisplayName, name) {
			return p, true
		}
	}
	return model.Player{}, false
}

func (w dmStatTestWorld) FindCreatureByName(roomID model.RoomID, name string, count int) (model.Creature, bool) {
	seen := 0
	for _, c := range w.creatures {
		if c.RoomID == roomID && strings.Contains(strings.ToLower(c.DisplayName), strings.ToLower(name)) {
			seen++
			if seen == count {
				return c, true
			}
		}
	}
	return model.Creature{}, false
}

func (w dmStatTestWorld) FindCreatureByNameGlobal(name string) (model.Creature, bool) {
	for _, c := range w.creatures {
		if strings.Contains(strings.ToLower(c.DisplayName), strings.ToLower(name)) {
			return c, true
		}
	}
	return model.Creature{}, false
}

func (w dmStatTestWorld) FindObjectByName(creatureID model.CreatureID, roomID model.RoomID, name string, count int) (model.ObjectInstance, bool) {
	seen := 0
	// 1. Check creature inventory
	if c, ok := w.creatures[creatureID]; ok {
		for _, oID := range c.Inventory.ObjectIDs {
			if obj, ok := w.objects[oID]; ok {
				if strings.Contains(strings.ToLower(obj.DisplayNameOverride), strings.ToLower(name)) {
					seen++
					if seen == count {
						return obj, true
					}
				}
			}
		}
		// 2. Check ready worn slots
		for _, oID := range c.Equipment {
			if obj, ok := w.objects[oID]; ok {
				if strings.Contains(strings.ToLower(obj.DisplayNameOverride), strings.ToLower(name)) {
					seen++
					if seen == count {
						return obj, true
					}
				}
			}
		}
	}
	// 3. Check room floor
	if room, ok := w.rooms[roomID]; ok {
		for _, oID := range room.Objects.ObjectIDs {
			if obj, ok := w.objects[oID]; ok {
				if strings.Contains(strings.ToLower(obj.DisplayNameOverride), strings.ToLower(name)) {
					seen++
					if seen == count {
						return obj, true
					}
				}
			}
		}
	}
	return model.ObjectInstance{}, false
}

func dmStatResolved(input string) ResolvedCommand {
	parsed := commandparse.Parse(input)
	if parsed.Num == 0 || parsed.Str[0] != "*status" {
		parsed = commandparse.ParseCommandFirst(input)
	}
	return ResolvedCommand{
		Input:  input,
		Parsed: parsed,
		Args:   commandArgs(parsed),
		Values: commandValues(parsed),
	}
}

func TestDMStatRejectsUnauthorized(t *testing.T) {
	tests := []struct {
		name  string
		class int
	}{
		{name: "regular class", class: model.ClassFighter},
		{name: "caretaker below SUB_DM", class: model.ClassCaretaker},
		{name: "bulsa below SUB_DM", class: model.ClassBulsa},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := dmStatTestWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": tt.class}},
				},
			}

			handler := NewDMStatHandler(world)
			ctx := &Context{ActorID: "player:alice"}

			status, err := handler(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusPrompt {
				t.Fatalf("status = %v, want StatusPrompt", status)
			}

			output := ctx.OutputString()
			if output != "" {
				t.Fatalf("output = %q, want no permission output", output)
			}
		})
	}
}

func TestDMStatAllowsZoneMaker(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": legacyClassZoneMaker},
				RoomID: "room:100",
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {
				ID:          "room:100",
				DisplayName: "시작의 방",
			},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if !strings.Contains(ctx.OutputString(), "방번호 #: 100") {
		t.Fatalf("unexpected output: %q", ctx.OutputString())
	}
}

func TestDMStatRoomNoArgs(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": model.ClassSubDM},
				RoomID: "room:100",
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {
				ID:          "room:100",
				DisplayName: "시작의 방",
				Exits: []model.Exit{{
					Name:     "북",
					ToRoomID: "room:00200",
					Flags:    []string{"secret", "XNOSEE"},
				}},
				Properties: map[string]string{
					"SPE-CIAL": "42",
					"traffic":  "50",
					"shoppe":   "true",
					"RNOTEL":   "1",
				},
			},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}

	output := ctx.OutputString()
	if !strings.Contains(output, "방번호 #: 100") || !strings.Contains(output, "이름: 시작의 방") || !strings.Contains(output, "Special: 42") || !strings.Contains(output, "Traffic: 50%") {
		t.Fatalf("unexpected room output: %q", output)
	}
	if !strings.Contains(output, "Flags set: Shoppe, NoTeleport.") {
		t.Fatalf("unexpected room output: %q", output)
	}
	if !strings.Contains(output, "북: 200, Flags: Secret, No-See.") {
		t.Fatalf("unexpected exit output: %q", output)
	}
}

func TestDMStatRoomByID(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": 12}, // SUB_DM (12)
				RoomID: "room:100",
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:200": {
				ID:          "room:200",
				DisplayName: "두번째 방",
			},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, dmStatResolved("200 *status"))
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}

	output := ctx.OutputString()
	if !strings.Contains(output, "방번호 #: 200") || !strings.Contains(output, "이름: 두번째 방") {
		t.Fatalf("unexpected room output: %q", output)
	}
}

func TestDMStatRoomByIDCommandFirst(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": model.ClassSubDM},
				RoomID: "room:100",
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:200": {
				ID:          "room:200",
				DisplayName: "두번째 방",
			},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, dmStatResolved("*status 200"))
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}

	output := ctx.OutputString()
	if !strings.Contains(output, "방번호 #: 200") || !strings.Contains(output, "이름: 두번째 방") {
		t.Fatalf("unexpected room output: %q", output)
	}
}

func TestDMStatRoomByIDAtRMAXReturnsSilently(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": model.ClassSubDM},
				RoomID: "room:100",
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {ID: "room:100", DisplayName: "시작의 방"},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, dmStatResolved("9000 *status"))
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "" {
		t.Fatalf("output = %q, want silent RMAX return", got)
	}
}

func TestDMStatObject(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": 12},
				RoomID: "room:100",
				Inventory: model.ObjectRefList{
					ObjectIDs: []model.ObjectInstanceID{"obj:sword"},
				},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {
				ID:          "room:100",
				DisplayName: "시작의 방",
				CreatureIDs: []model.CreatureID{"creature:goblin"},
			},
		},
		objects: map[model.ObjectInstanceID]model.ObjectInstance{
			"obj:sword": {
				ID:                  "obj:sword",
				DisplayNameOverride: "전설의 검",
				Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
				Properties: map[string]string{
					"description": "아주 날카로운 검이다.",
					"key[0]":      "검",
					"nDice":       "3",
					"sDice":       "6",
					"pDice":       "2",
					"type":        "1", // Sword (검)
					"armor":       "5",
					"value":       "500",
					"weight":      "10",
				},
			},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"검"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}

	output := ctx.OutputString()
	if !strings.Contains(output, "이름: 전설의 검") || !strings.Contains(output, "설명: 아주 날카로운 검이다.") || !strings.Contains(output, "타격: 3d6 + 2") || !strings.Contains(output, "검 무기.") {
		t.Fatalf("unexpected object output: %q", output)
	}
}

func TestDMStatObjectFlagsExpandLegacyAliases(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": model.ClassSubDM},
				RoomID: "room:100",
				Inventory: model.ObjectRefList{
					ObjectIDs: []model.ObjectInstanceID{"obj:alias"},
				},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {ID: "room:100", DisplayName: "시작의 방"},
		},
		objects: map[model.ObjectInstanceID]model.ObjectInstance{
			"obj:alias": {
				ID:                  "obj:alias",
				PrototypeID:         "proto:alias",
				DisplayNameOverride: "별칭검",
				Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
				Metadata: model.Metadata{
					Tags: []string{"inventoryPermanent"},
				},
				Properties: map[string]string{
					"flags":         "tempPermanent|noBurn",
					"tempPermanent": "true",
				},
			},
		},
		objectPrototypes: map[model.PrototypeID]model.ObjectPrototype{
			"proto:alias": {
				ID:          "proto:alias",
				DisplayName: "별칭검",
				Metadata: model.Metadata{
					Tags: []string{"scene"},
				},
				Properties: map[string]string{
					"eventItem": "yes",
					"flags":     "marriageOnly,held",
				},
			},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"별칭"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}

	output := ctx.OutputString()
	for _, want := range []string{"Iperm", "Tperm", "Scenery", "Marriage", "Event Item", "Noburn", "Held"} {
		if !strings.Contains(output, want) {
			t.Fatalf("dm_stat object output missing %q:\n%s", want, output)
		}
	}
}

func TestDMStatCreature(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": 12},
				RoomID: "room:100",
			},
			"creature:goblin": {
				ID:          "creature:goblin",
				DisplayName: "어리바리 고블린",
				RoomID:      "room:100",
				Level:       5,
				Stats: map[string]int{
					"class":      1, // Fighter
					"alignment":  -150,
					"gold":       100,
					"experience": 450,
					"hpCurrent":  30,
					"hpMax":      30,
					"mpCurrent":  10,
					"mpMax":      10,
					"armor":      20,
					"thaco":      15,
					"strength":   14,
					"dexterity":  12,
				},
				Properties: map[string]string{
					"race": "6", // 도깨비족
					"talk": "크르르!",
				},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {
				ID:          "room:100",
				DisplayName: "시작의 방",
				CreatureIDs: []model.CreatureID{"creature:goblin"},
			},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"고블린"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}

	output := ctx.OutputString()
	if !strings.Contains(output, "이름: 어리바리 고블린") || !strings.Contains(output, "이야기: 크르르!") || !strings.Contains(output, "레벨: 5") || !strings.Contains(output, "종족: 도깨비족") || !strings.Contains(output, "성향: 악 -150") || !strings.Contains(output, "돈: 100") {
		t.Fatalf("unexpected creature output: %q", output)
	}
}

func TestDMStatUsesParsedTargetSlotAndOrdinalLikeCWhenArgsMissing(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": model.ClassSubDM},
				RoomID: "room:100",
			},
			"creature:goblin1": {
				ID:          "creature:goblin1",
				DisplayName: "고블린",
				RoomID:      "room:100",
				Level:       3,
			},
			"creature:goblin2": {
				ID:          "creature:goblin2",
				DisplayName: "고블린",
				RoomID:      "room:100",
				Level:       7,
				Properties: map[string]string{
					"talk": "둘째",
				},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {
				ID:          "room:100",
				DisplayName: "시작의 방",
				CreatureIDs: []model.CreatureID{"creature:goblin1", "creature:goblin2"},
			},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Input:  "2 고블린 *status",
		Parsed: commandparse.Command{Num: 2, Str: [7]string{"*status", "고블린"}, Val: [7]int64{1, 2}},
	}

	status, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}

	output := ctx.OutputString()
	if !strings.Contains(output, "레벨: 7") || !strings.Contains(output, "이야기: 둘째") {
		t.Fatalf("parsed-slot ordinal did not select second creature: %q", output)
	}
}

func TestDMStatCreatureLookupPrefersMonsterBeforeRoomPlayerLikeLegacy(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", DisplayName: "그림자", CreatureID: "creature:bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": model.ClassSubDM},
				RoomID: "room:100",
			},
			"creature:bob": {
				ID:          "creature:bob",
				Kind:        model.CreatureKindPlayer,
				DisplayName: "그림자",
				PlayerID:    "player:bob",
				RoomID:      "room:100",
			},
			"creature:shade": {
				ID:          "creature:shade",
				DisplayName: "그림자",
				Description: "몬스터 우선",
				RoomID:      "room:100",
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {
				ID:          "room:100",
				CreatureIDs: []model.CreatureID{"creature:bob", "creature:shade"},
				PlayerIDs:   []model.PlayerID{"player:bob"},
			},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"그림자"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if output := ctx.OutputString(); !strings.Contains(output, "설명: 몬스터 우선") {
		t.Fatalf("expected monster stat before same-name room player, got: %q", output)
	}
}

func TestDMStatCreatureLookupUsesFindCrtVisibilityLikeLegacy(t *testing.T) {
	baseWorld := func(actorTags []string) dmStatTestWorld {
		return dmStatTestWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {
					ID:       "creature:alice",
					Stats:    map[string]int{"class": model.ClassSubDM},
					RoomID:   "room:100",
					Metadata: model.Metadata{Tags: actorTags},
				},
				"creature:hidden": {
					ID:          "creature:hidden",
					DisplayName: "그림자",
					Description: "숨은 대상",
					RoomID:      "room:100",
					Metadata:    model.Metadata{Tags: []string{"MINVIS"}},
				},
				"creature:visible": {
					ID:          "creature:visible",
					DisplayName: "그림자",
					Description: "보이는 대상",
					RoomID:      "room:100",
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:100": {
					ID:          "room:100",
					CreatureIDs: []model.CreatureID{"creature:hidden", "creature:visible"},
				},
			},
		}
	}

	t.Run("without PDINVI", func(t *testing.T) {
		handler := NewDMStatHandler(baseWorld(nil))
		ctx := &Context{ActorID: "player:alice"}

		status, err := handler(ctx, ResolvedCommand{Args: []string{"그림자"}})
		if err != nil {
			t.Fatalf("handler() error = %v", err)
		}
		if status != StatusDefault {
			t.Fatalf("status = %v, want StatusDefault", status)
		}
		if output := ctx.OutputString(); !strings.Contains(output, "설명: 보이는 대상") || strings.Contains(output, "숨은 대상") {
			t.Fatalf("unexpected non-PDINVI creature lookup output: %q", output)
		}
	})

	t.Run("with PDINVI", func(t *testing.T) {
		handler := NewDMStatHandler(baseWorld([]string{"PDINVI"}))
		ctx := &Context{ActorID: "player:alice"}

		status, err := handler(ctx, ResolvedCommand{Args: []string{"그림자"}})
		if err != nil {
			t.Fatalf("handler() error = %v", err)
		}
		if status != StatusDefault {
			t.Fatalf("status = %v, want StatusDefault", status)
		}
		if output := ctx.OutputString(); !strings.Contains(output, "설명: 숨은 대상") {
			t.Fatalf("unexpected PDINVI creature lookup output: %q", output)
		}
	})
}

func TestDMStatOnlinePlayerLookupUsesActiveSessions(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": model.ClassSubDM},
				RoomID: "room:100",
			},
			"creature:bob": {
				ID:          "creature:bob",
				DisplayName: "Bob",
				PlayerID:    "player:bob",
				RoomID:      "room:200",
				Stats:       map[string]int{"class": model.ClassFighter},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {ID: "room:100", DisplayName: "시작의 방"},
			"room:200": {ID: "room:200", DisplayName: "다른 방"},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{
					{ID: "session:bob", ActorID: "player:bob"},
				}
			},
		},
	}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"bOB"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if !strings.Contains(ctx.OutputString(), "Bob") {
		t.Fatalf("unexpected output: %q", ctx.OutputString())
	}
}

func TestDMStatSavedPlayerWithoutActiveSessionIsNotFound(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": model.ClassSubDM},
				RoomID: "room:100",
			},
			"creature:bob": {
				ID:          "creature:bob",
				DisplayName: "Bob",
				PlayerID:    "player:bob",
				RoomID:      "room:200",
				Stats:       map[string]int{"class": model.ClassFighter},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {ID: "room:100", DisplayName: "시작의 방"},
			"room:200": {ID: "room:200", DisplayName: "다른 방"},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{
					{ID: "session:alice", ActorID: "player:alice"},
				}
			},
		},
	}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if got, want := ctx.OutputString(), "그런건 없습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDMStatSecondaryActorSavedPlayerWithoutActiveSessionFallsBackToSelf(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": model.ClassSubDM},
				RoomID: "room:100",
			},
			"creature:bob": {
				ID:          "creature:bob",
				DisplayName: "Bob",
				PlayerID:    "player:bob",
				RoomID:      "room:200",
				Inventory: model.ObjectRefList{
					ObjectIDs: []model.ObjectInstanceID{"obj:sword"},
				},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {ID: "room:100", DisplayName: "시작의 방"},
			"room:200": {ID: "room:200", DisplayName: "다른 방"},
		},
		objects: map[model.ObjectInstanceID]model.ObjectInstance{
			"obj:sword": {
				ID:                  "obj:sword",
				DisplayNameOverride: "전설의 검",
			},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{
					{ID: "session:alice", ActorID: "player:alice"},
				}
			},
		},
	}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"검", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if got, want := ctx.OutputString(), "그런건 없습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDMStatSecondaryActorLookupPrefersMonsterBeforeRoomPlayerLikeLegacy(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob-player"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": model.ClassSubDM},
				RoomID: "room:100",
			},
			"creature:bob-player": {
				ID:          "creature:bob-player",
				Kind:        model.CreatureKindPlayer,
				DisplayName: "Bob",
				PlayerID:    "player:bob",
				RoomID:      "room:100",
			},
			"creature:bob-monster": {
				ID:          "creature:bob-monster",
				DisplayName: "Bob",
				RoomID:      "room:100",
				Inventory: model.ObjectRefList{
					ObjectIDs: []model.ObjectInstanceID{"obj:amulet"},
				},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {
				ID:          "room:100",
				CreatureIDs: []model.CreatureID{"creature:bob-player", "creature:bob-monster"},
				PlayerIDs:   []model.PlayerID{"player:bob"},
			},
		},
		objects: map[model.ObjectInstanceID]model.ObjectInstance{
			"obj:amulet": {
				ID:                  "obj:amulet",
				DisplayNameOverride: "검은 부적",
				Location:            model.ObjectLocation{CreatureID: "creature:bob-monster", Slot: "inventory"},
				Properties: map[string]string{
					"description": "몬스터 소지품",
					"key[0]":      "부적",
				},
			},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"부적", "Bob"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}

	output := ctx.OutputString()
	if !strings.Contains(output, "이름: 검은 부적") || !strings.Contains(output, "설명: 몬스터 소지품") {
		t.Fatalf("unexpected secondary actor monster-first output: %q", output)
	}
}

func TestDMStatObjectUsesSecondaryActorRoomLikeLegacy(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": model.ClassSubDM},
				RoomID: "room:100",
			},
			"creature:bob": {
				ID:          "creature:bob",
				DisplayName: "Bob",
				PlayerID:    "player:bob",
				RoomID:      "room:200",
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {
				ID:      "room:100",
				Objects: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"obj:actor-room-sword"}},
			},
			"room:200": {
				ID:      "room:200",
				Objects: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"obj:bob-room-sword"}},
			},
		},
		objects: map[model.ObjectInstanceID]model.ObjectInstance{
			"obj:actor-room-sword": {
				ID:                  "obj:actor-room-sword",
				DisplayNameOverride: "실행자 방 검",
				Location:            model.ObjectLocation{RoomID: "room:100"},
				Properties: map[string]string{
					"description": "actor room",
					"key[0]":      "검",
				},
			},
			"obj:bob-room-sword": {
				ID:                  "obj:bob-room-sword",
				DisplayNameOverride: "대상 방 검",
				Location:            model.ObjectLocation{RoomID: "room:200"},
				Properties: map[string]string{
					"description": "bob room",
					"key[0]":      "검",
				},
			},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{
					{ID: "session:bob", ActorID: "player:bob"},
				}
			},
		},
	}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"검", "Bob"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}

	output := ctx.OutputString()
	if !strings.Contains(output, "이름: 대상 방 검") || !strings.Contains(output, "설명: bob room") {
		t.Fatalf("unexpected secondary actor room object output: %q", output)
	}
	if strings.Contains(output, "실행자 방 검") || strings.Contains(output, "actor room") {
		t.Fatalf("searched executor room instead of secondary actor room: %q", output)
	}
}

func TestDMStatCreatureUsesSecondaryActorRoomLikeLegacy(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": model.ClassSubDM},
				RoomID: "room:100",
			},
			"creature:bob": {
				ID:          "creature:bob",
				DisplayName: "Bob",
				PlayerID:    "player:bob",
				RoomID:      "room:200",
			},
			"creature:actor-room-shade": {
				ID:          "creature:actor-room-shade",
				DisplayName: "실행자 방 그림자",
				RoomID:      "room:100",
			},
			"creature:bob-room-shade": {
				ID:          "creature:bob-room-shade",
				DisplayName: "대상 방 그림자",
				RoomID:      "room:200",
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {ID: "room:100", CreatureIDs: []model.CreatureID{"creature:actor-room-shade"}},
			"room:200": {ID: "room:200", CreatureIDs: []model.CreatureID{"creature:bob-room-shade"}},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{
					{ID: "session:bob", ActorID: "player:bob"},
				}
			},
		},
	}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"그림자", "Bob"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}

	output := ctx.OutputString()
	if !strings.Contains(output, "이름: 대상 방 그림자") {
		t.Fatalf("unexpected secondary actor room creature output: %q", output)
	}
	if strings.Contains(output, "실행자 방 그림자") {
		t.Fatalf("searched executor room instead of secondary actor room: %q", output)
	}
}

func TestDMStatObjectFindObjVisibilityUsesPDINVILikeLegacy(t *testing.T) {
	baseWorld := func(creatureTags []string) dmStatTestWorld {
		return dmStatTestWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {
					ID:       "creature:alice",
					Stats:    map[string]int{"class": model.ClassSubDM},
					RoomID:   "room:100",
					Metadata: model.Metadata{Tags: creatureTags},
					Inventory: model.ObjectRefList{
						ObjectIDs: []model.ObjectInstanceID{"obj:invisible-sword"},
					},
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:100": {ID: "room:100"},
			},
			objects: map[model.ObjectInstanceID]model.ObjectInstance{
				"obj:invisible-sword": {
					ID:                  "obj:invisible-sword",
					DisplayNameOverride: "은신검",
					Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
					Metadata:            model.Metadata{Tags: []string{"OINVIS"}},
					Properties: map[string]string{
						"description": "보이지 않는 검",
					},
				},
			},
		}
	}

	t.Run("without PDINVI", func(t *testing.T) {
		handler := NewDMStatHandler(baseWorld(nil))
		ctx := &Context{ActorID: "player:alice"}

		status, err := handler(ctx, ResolvedCommand{Args: []string{"은신"}})
		if err != nil {
			t.Fatalf("handler() error = %v", err)
		}
		if status != StatusDefault {
			t.Fatalf("status = %v, want StatusDefault", status)
		}
		if got, want := ctx.OutputString(), "그런건 없습니다.\n"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("with PDINVI", func(t *testing.T) {
		handler := NewDMStatHandler(baseWorld([]string{"PDINVI"}))
		ctx := &Context{ActorID: "player:alice"}

		status, err := handler(ctx, ResolvedCommand{Args: []string{"은신"}})
		if err != nil {
			t.Fatalf("handler() error = %v", err)
		}
		if status != StatusDefault {
			t.Fatalf("status = %v, want StatusDefault", status)
		}
		output := ctx.OutputString()
		if !strings.Contains(output, "이름: 은신검") || !strings.Contains(output, "설명: 보이지 않는 검") {
			t.Fatalf("unexpected PDINVI object output: %q", output)
		}
	})
}

func TestDMStatNotFound(t *testing.T) {
	world := dmStatTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:     "creature:alice",
				Stats:  map[string]int{"class": 12},
				RoomID: "room:100",
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {
				ID:          "room:100",
				DisplayName: "시작의 방",
			},
		},
	}

	handler := NewDMStatHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"없는대상"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}

	output := ctx.OutputString()
	if output != "그런건 없습니다.\n" {
		t.Fatalf("output = %q, want %q", output, "그런건 없습니다.\n")
	}
}
