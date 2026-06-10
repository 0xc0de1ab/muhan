package command

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"muhan/internal/commandspec"
	"muhan/internal/migrate/roommap"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestMoveTrapPitMovesToTrapExitAndDamagesAfterDestinationRender(t *testing.T) {
	withMoveTrapRolls(t, 100, 7)
	loaded := moveTrapWorld(t, "1", "55")
	mustAddLookRoom(t, loaded, model.Room{ID: "room:00055", DisplayName: "구덩이 밑"})
	setMoveTrapActorStats(t, loaded, map[string]int{
		"dexterity": 0,
		"hpCurrent": 20,
		"hpMax":     20,
	})

	world, out := dispatchMoveLine(t, loaded, "동")

	assertMovePlayerRoom(t, world, "room:00055")
	actor, _ := world.Creature("creature:alice")
	if got := actor.Stats["hpCurrent"]; got != 13 {
		t.Fatalf("hpCurrent = %d, want 13", got)
	}
	assertMoveTrapOutputOrder(t, out,
		"\n동쪽\n\n",
		"당신은 구덩이에 빠졌습니다!\n",
		"\n구덩이 밑\n\n",
		"당신은 7점의 피해를 입었습니다.\n",
	)
}

func TestMoveTrapFatalDamageRunsPlayerDeathHookAfterTrapExit(t *testing.T) {
	withMoveTrapRolls(t, 100, 7)
	loaded := moveTrapWorld(t, "1", "55")
	mustAddLookRoom(t, loaded, model.Room{ID: "room:00055", DisplayName: "구덩이 밑"})
	mustAddLookRoom(t, loaded, model.Room{ID: "room:1008", DisplayName: "죽으면 가는 곳"})
	setMoveTrapActorStats(t, loaded, map[string]int{
		"dexterity": 0,
		"hpCurrent": 5,
		"hpMax":     20,
	})
	world := &moveTrapDeathWorld{World: state.NewWorld(loaded)}

	out := dispatchMoveLineWithMoveWorld(t, world, "동")

	if len(world.deaths) != 1 {
		t.Fatalf("death calls = %+v, want one call", world.deaths)
	}
	if world.deaths[0].playerID != "player:alice" || world.deaths[0].attackerID != "creature:alice" {
		t.Fatalf("death call = %+v, want player:alice/creature:alice", world.deaths[0])
	}
	assertMoveWorldPlayerRoom(t, world, "room:1008")
	assertMoveTrapOutputOrder(t, out,
		"당신은 구덩이에 빠졌습니다!\n",
		"\n구덩이 밑\n\n",
		"당신은 7점의 피해를 입었습니다.\n",
		"당신은 죽으면서 몇가지 물건을 떨어뜨렸습니다.\n",
	)
}

func TestMoveTrapDartPoisonsAndDamages(t *testing.T) {
	withMoveTrapRolls(t, 100, 4)
	loaded := moveTrapWorld(t, "2", "")
	setMoveTrapActorStats(t, loaded, map[string]int{
		"dexterity": 0,
		"hpCurrent": 20,
		"hpMax":     20,
	})

	world, out := dispatchMoveLine(t, loaded, "동")

	assertMovePlayerRoom(t, world, "room:east")
	actor, _ := world.Creature("creature:alice")
	if got := actor.Stats["hpCurrent"]; got != 16 {
		t.Fatalf("hpCurrent = %d, want 16", got)
	}
	if !hasAnyNormalizedFlag(actor.Metadata.Tags, "poison", "PPOISN") {
		t.Fatalf("creature tags = %+v, want poison/PPOISN", actor.Metadata.Tags)
	}
	player, _ := world.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "poison", "PPOISN") {
		t.Fatalf("player tags = %+v, want poison/PPOISN", player.Metadata.Tags)
	}
	assertMoveTrapOutputOrder(t, out,
		"\n동쪽\n\n",
		"당신은 숨겨진 독화살에 맞았습니다!\n",
		"당신은 4점의 피해를 입었습니다.\n",
	)
}

func TestMoveTrapFatalDamageRunsPlayerDeathHookForDamageTraps(t *testing.T) {
	tests := []struct {
		name     string
		trapType string
		rolls    []int
		stats    map[string]int
		outputs  []string
	}{
		{
			name:     "dart",
			trapType: "2",
			rolls:    []int{100, 8},
			stats: map[string]int{
				"dexterity": 0,
				"hpCurrent": 5,
				"hpMax":     20,
			},
			outputs: []string{
				"당신은 숨겨진 독화살에 맞았습니다!\n",
				"당신은 8점의 피해를 입었습니다.\n",
				"당신은 죽으면서 몇가지 물건을 떨어뜨렸습니다.\n",
			},
		},
		{
			name:     "block",
			trapType: "3",
			rolls:    []int{100},
			stats: map[string]int{
				"dexterity": 0,
				"hpCurrent": 5,
				"hpMax":     10,
			},
			outputs: []string{
				"당신은 커다란 돌에 맞았습니다!\n",
				"당신은 5점의 피해를 입었습니다.\n",
				"당신은 죽으면서 몇가지 물건을 떨어뜨렸습니다.\n",
			},
		},
		{
			name:     "mp damage",
			trapType: "4",
			rolls:    []int{100, 6},
			stats: map[string]int{
				"intelligence": 0,
				"hpCurrent":    5,
				"hpMax":        20,
				"mpCurrent":    30,
				"mpMax":        40,
			},
			outputs: []string{
				"당신의 마음이 충격을 받았습니다!\n",
				"당신은 20점의 마력을 잃었습니다.\n",
				"당신은 6점의 피해를 입었습니다.\n",
				"당신은 죽으면서 몇가지 물건을 떨어뜨렸습니다.\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withMoveTrapRolls(t, tt.rolls...)
			loaded := moveTrapWorld(t, tt.trapType, "")
			mustAddLookRoom(t, loaded, model.Room{ID: "room:1008", DisplayName: "죽으면 가는 곳"})
			setMoveTrapActorStats(t, loaded, tt.stats)
			world := &moveTrapDeathWorld{World: state.NewWorld(loaded)}

			out := dispatchMoveLineWithMoveWorld(t, world, "동")

			if len(world.deaths) != 1 {
				t.Fatalf("death calls = %+v, want one call", world.deaths)
			}
			if world.deaths[0].playerID != "player:alice" || world.deaths[0].attackerID != "creature:alice" {
				t.Fatalf("death call = %+v, want player:alice/creature:alice", world.deaths[0])
			}
			assertMoveWorldPlayerRoom(t, world, "room:1008")
			assertMoveTrapOutputOrder(t, out, tt.outputs...)
		})
	}
}

func TestMoveTrapPreparedAvoidsAndClearsPrepared(t *testing.T) {
	withMoveTrapRolls(t, 1)
	loaded := moveTrapWorld(t, "3", "")
	setMoveTrapActorStats(t, loaded, map[string]int{
		"dexterity": 20,
		"hpCurrent": 20,
		"hpMax":     20,
	})
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PPREPA", "prepared"}
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"PPREPA", "prepared"}
	loaded.Players[player.ID] = player

	world, out := dispatchMoveLine(t, loaded, "동")

	assertMovePlayerRoom(t, world, "room:east")
	actor, _ := world.Creature("creature:alice")
	if got := actor.Stats["hpCurrent"]; got != 20 {
		t.Fatalf("hpCurrent = %d, want unchanged 20", got)
	}
	if hasAnyNormalizedFlag(actor.Metadata.Tags, "prepared", "PPREPA") {
		t.Fatalf("creature tags = %+v, want prepared cleared", actor.Metadata.Tags)
	}
	player, _ = world.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "prepared", "PPREPA") {
		t.Fatalf("player tags = %+v, want prepared cleared", player.Metadata.Tags)
	}
	if strings.Contains(out, "커다란 돌") {
		t.Fatalf("trap triggered despite prepared avoidance:\n%s", out)
	}
}

func TestMoveTrapPreparedCleanupMatchesLegacyBranches(t *testing.T) {
	tests := []struct {
		name              string
		trapType          string
		rolls             []int
		creatureTags      []string
		playerTags        []string
		wantPrepared      bool
		wantHP            int
		wantRoom          model.RoomID
		forbidOutputParts []string
	}{
		{
			name:              "no trap clears prepared",
			trapType:          "",
			creatureTags:      []string{"PPREPA", "prepared"},
			playerTags:        []string{"PPREPA", "prepared"},
			wantHP:            20,
			wantRoom:          "room:east",
			forbidOutputParts: []string{"구덩이", "독화살", "커다란 돌", "경보장치"},
		},
		{
			name:              "unknown trap preserves prepared",
			trapType:          "99",
			creatureTags:      []string{"PPREPA", "prepared"},
			playerTags:        []string{"PPREPA", "prepared"},
			wantPrepared:      true,
			wantHP:            20,
			wantRoom:          "room:east",
			forbidOutputParts: []string{"구덩이", "독화살", "커다란 돌", "경보장치"},
		},
		{
			name:              "levitating pit clears prepared but does not damage",
			trapType:          "1",
			rolls:             []int{100, 100},
			creatureTags:      []string{"PPREPA", "prepared", "PLEVIT"},
			playerTags:        []string{"PPREPA", "prepared"},
			wantHP:            20,
			wantRoom:          "room:east",
			forbidOutputParts: []string{"구덩이에 빠졌습니다", "피해를 입었습니다"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.rolls) > 0 {
				withMoveTrapRolls(t, tt.rolls...)
			}
			loaded := moveTrapWorld(t, tt.trapType, "")
			if tt.trapType == "" {
				east := loaded.Rooms["room:east"]
				delete(east.Properties, "trap")
				loaded.Rooms[east.ID] = east
			}
			setMoveTrapActorStats(t, loaded, map[string]int{
				"dexterity": 0,
				"hpCurrent": 20,
				"hpMax":     20,
			})
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = tt.creatureTags
			loaded.Creatures[alice.ID] = alice
			player := loaded.Players["player:alice"]
			player.Metadata.Tags = tt.playerTags
			loaded.Players[player.ID] = player

			world, out := dispatchMoveLine(t, loaded, "동")

			actor, _ := world.Creature("creature:alice")
			if got := hasAnyNormalizedFlag(actor.Metadata.Tags, "prepared", "PPREPA"); got != tt.wantPrepared {
				t.Fatalf("creature prepared = %v from tags %+v, want %v", got, actor.Metadata.Tags, tt.wantPrepared)
			}
			player, _ = world.Player("player:alice")
			if got := hasAnyNormalizedFlag(player.Metadata.Tags, "prepared", "PPREPA"); got != tt.wantPrepared {
				t.Fatalf("player prepared = %v from tags %+v, want %v", got, player.Metadata.Tags, tt.wantPrepared)
			}
			if got := actor.Stats["hpCurrent"]; got != tt.wantHP {
				t.Fatalf("hpCurrent = %d, want %d", got, tt.wantHP)
			}
			assertMovePlayerRoom(t, world, tt.wantRoom)
			for _, part := range tt.forbidOutputParts {
				if strings.Contains(out, part) {
					t.Fatalf("output contained %q:\n%s", part, out)
				}
			}
		})
	}
}

func TestMoveTrapMPDamageLosesManaAndHealth(t *testing.T) {
	withMoveTrapRolls(t, 100, 5)
	loaded := moveTrapWorld(t, "4", "")
	setMoveTrapActorStats(t, loaded, map[string]int{
		"intelligence": 0,
		"hpCurrent":    20,
		"hpMax":        20,
		"mpCurrent":    30,
		"mpMax":        40,
	})

	world, out := dispatchMoveLine(t, loaded, "동")

	actor, _ := world.Creature("creature:alice")
	if got := actor.Stats["mpCurrent"]; got != 10 {
		t.Fatalf("mpCurrent = %d, want 10", got)
	}
	if got := actor.Stats["hpCurrent"]; got != 15 {
		t.Fatalf("hpCurrent = %d, want 15", got)
	}
	assertMoveTrapOutputOrder(t, out,
		"당신의 마음이 충격을 받았습니다!\n",
		"당신은 20점의 마력을 잃었습니다.\n",
		"당신은 5점의 피해를 입었습니다.\n",
	)
}

func TestMoveTrapRemoveSpellZerosBuffExpirationsButKeepsFlagsUntilTick(t *testing.T) {
	withMoveTrapRolls(t, 100)
	loaded := moveTrapWorld(t, "5", "")
	setMoveTrapActorStats(t, loaded, map[string]int{"intelligence": 0})
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PBLESS", "PDMAGI", "poison"}
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"PBLESS", "PDMAGI", "poison"}
	loaded.Players[player.ID] = player

	world, out := dispatchMoveLine(t, loaded, "동")

	actor, _ := world.Creature("creature:alice")
	if !hasAnyNormalizedFlag(actor.Metadata.Tags, "PBLESS", "PDMAGI") {
		t.Fatalf("creature tags = %+v, want spell flags retained until player update tick", actor.Metadata.Tags)
	}
	if !hasAnyNormalizedFlag(actor.Metadata.Tags, "poison") {
		t.Fatalf("creature tags = %+v, want non-spell poison kept", actor.Metadata.Tags)
	}
	player, _ = world.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "PBLESS", "PDMAGI") {
		t.Fatalf("player tags = %+v, want spell flags retained until player update tick", player.Metadata.Tags)
	}
	for _, want := range []string{"PBLESS", "PDMAGI"} {
		if got, ok := world.GetEffectExpiration("creature:alice", want); !ok || got != 0 {
			t.Fatalf("expiration %s = %d/%v, want 0/true", want, got, ok)
		}
	}
	if !strings.Contains(out, "당신의 주문이 사라집니다.\n") {
		t.Fatalf("output missing remove-spell message:\n%s", out)
	}
}

func TestMoveTrapRemoveSpellSetsAllSpellExpirationsToZeroWhenHookExists(t *testing.T) {
	withMoveTrapRolls(t, 100)
	loaded := moveTrapWorld(t, "5", "")
	setMoveTrapActorStats(t, loaded, map[string]int{"intelligence": 0})
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PBLESS", "PDMAGI"}
	loaded.Creatures[alice.ID] = alice
	world := &moveTrapEffectExpirationWorldHook{World: state.NewWorld(loaded)}

	_ = dispatchMoveLineWithMoveWorld(t, world, "동")

	for _, want := range moveTrapSpellExpirationTags {
		if got, ok := world.expirations["creature:alice"][want]; !ok || got != 0 {
			t.Fatalf("expiration %s = %d/%v in %+v, want 0/true", want, got, ok, world.expirations)
		}
	}
}

func TestMoveTrapNakedDestroysInventoryAndNonCursedEquipment(t *testing.T) {
	withMoveTrapRolls(t, 100)
	loaded := moveTrapWorld(t, "6", "")
	setMoveTrapActorStats(t, loaded, map[string]int{"dexterity": 0})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:potion",
		PrototypeID: "prototype:coin",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:bag",
		PrototypeID: "prototype:coin",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:gem"}},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:gem",
		PrototypeID: "prototype:coin",
		Location:    model.ObjectLocation{ContainerID: "object:bag"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:sword",
		PrototypeID: "prototype:coin",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "wield"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:cursed-ring",
		PrototypeID: "prototype:coin",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "finger"},
		Metadata:    model.Metadata{Tags: []string{"OCURSE"}},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:cursed-box",
		PrototypeID: "prototype:coin",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "neck"},
		Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:cursed-gem"}},
		Properties:  map[string]string{"cursed": "true"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:cursed-gem",
		PrototypeID: "prototype:coin",
		Location:    model.ObjectLocation{ContainerID: "object:cursed-box"},
	})
	alice := loaded.Creatures["creature:alice"]
	alice.Inventory.ObjectIDs = []model.ObjectInstanceID{"object:potion", "object:bag"}
	alice.Equipment = map[string]model.ObjectInstanceID{
		"wield":  "object:sword",
		"finger": "object:cursed-ring",
		"neck":   "object:cursed-box",
	}
	loaded.Creatures[alice.ID] = alice

	world, out := dispatchMoveLine(t, loaded, "동")

	if _, ok := world.Object("object:potion"); ok {
		t.Fatal("inventory object survived naked trap")
	}
	if _, ok := world.Object("object:bag"); ok {
		t.Fatal("inventory container survived naked trap")
	}
	if _, ok := world.Object("object:gem"); ok {
		t.Fatal("inventory container child survived naked trap")
	}
	if _, ok := world.Object("object:sword"); ok {
		t.Fatal("non-cursed equipment survived naked trap")
	}
	if _, ok := world.Object("object:cursed-ring"); !ok {
		t.Fatal("tag-cursed equipment was destroyed")
	}
	if _, ok := world.Object("object:cursed-box"); !ok {
		t.Fatal("property-cursed equipment was destroyed")
	}
	if child, ok := world.Object("object:cursed-gem"); !ok || child.Location.ContainerID != "object:cursed-box" {
		t.Fatalf("property-cursed equipment child = %+v, %v; want kept inside cursed box", child, ok)
	}
	actor, _ := world.Creature("creature:alice")
	if len(actor.Inventory.ObjectIDs) != 0 {
		t.Fatalf("inventory refs after naked trap = %+v, want empty", actor.Inventory.ObjectIDs)
	}
	if _, ok := actor.Equipment["wield"]; ok {
		t.Fatalf("non-cursed equipment ref survived naked trap: %+v", actor.Equipment)
	}
	if actor.Equipment["finger"] != "object:cursed-ring" || actor.Equipment["neck"] != "object:cursed-box" {
		t.Fatalf("cursed equipment refs after naked trap = %+v", actor.Equipment)
	}
	if !strings.Contains(out, "으악!!! 당신의 장비가 녹아버립니다.\n") {
		t.Fatalf("output missing naked trap message:\n%s", out)
	}
}

func TestMoveTrapAlarmMovesPermanentGuardToTriggeredRoom(t *testing.T) {
	withMoveTrapRolls(t, 100)
	loaded := moveTrapWorld(t, "7", "90")
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:00090",
		DisplayName: "경비실",
		CreatureIDs: []model.CreatureID{
			"creature:sentinel",
		},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:sentinel",
		Kind:        model.CreatureKindMonster,
		DisplayName: "파수꾼",
		RoomID:      "room:00090",
		Metadata:    model.Metadata{Tags: []string{"MPERMT"}},
	})
	setMoveTrapActorStats(t, loaded, map[string]int{"dexterity": 0})

	world := &moveTrapPermanentHookWorld{World: state.NewWorld(loaded)}
	out := dispatchMoveLineWithMoveWorld(t, world, "동")

	guard, ok := world.Creature("creature:sentinel")
	if !ok {
		t.Fatal("missing sentinel")
	}
	if guard.RoomID != "room:east" {
		t.Fatalf("guard room = %q, want room:east", guard.RoomID)
	}
	if creatureHasAnyFlag(guard, "MPERMT", "permanent") {
		t.Fatalf("guard tags = %+v, want permanent cleared", guard.Metadata.Tags)
	}
	if !creatureHasAnyFlag(guard, "MAGGRE", "aggressive") {
		t.Fatalf("guard tags = %+v, want aggressive", guard.Metadata.Tags)
	}
	enemies, err := world.CreatureEnemies("creature:sentinel")
	if err != nil {
		t.Fatalf("CreatureEnemies() error = %v", err)
	}
	if !moveTrapStringSliceContains(enemies, "Alice") {
		t.Fatalf("guard enemies = %+v, want Alice", enemies)
	}
	if len(world.permanentTriggers) != 1 ||
		world.permanentTriggers[0].playerID != "player:alice" ||
		world.permanentTriggers[0].creatureID != "creature:sentinel" {
		t.Fatalf("permanent triggers = %+v, want player:alice/creature:sentinel", world.permanentTriggers)
	}
	if !strings.Contains(out, "경보장치가 울립니다!\n") {
		t.Fatalf("output missing alarm message:\n%s", out)
	}
}

func TestMoveTrapAlarmLoadsAndPopulatesPermanentGuardsBeforeScan(t *testing.T) {
	withMoveTrapRolls(t, 100)
	loaded := moveTrapWorld(t, "7", "91")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:sentinel",
		Kind:        model.CreatureKindMonster,
		DisplayName: "파수꾼",
		RoomID:      "room:00091",
		Metadata:    model.Metadata{Tags: []string{"MPERMT"}},
	})
	setMoveTrapActorStats(t, loaded, map[string]int{"dexterity": 0})

	world := &moveTrapPopulatePermanentHookWorld{
		moveTrapPermanentHookWorld: &moveTrapPermanentHookWorld{World: state.NewWorld(loaded)},
		roomOverrides:              map[model.RoomID]model.Room{},
	}
	out := dispatchMoveLineWithMoveWorld(t, world, "동")

	if len(world.loadedRooms) != 1 || world.loadedRooms[0] != "room:00091" {
		t.Fatalf("loaded rooms = %+v, want room:00091", world.loadedRooms)
	}
	if len(world.populatedRooms) != 1 || world.populatedRooms[0] != "room:00091" {
		t.Fatalf("populated rooms = %+v, want room:00091", world.populatedRooms)
	}
	guard, ok := world.Creature("creature:sentinel")
	if !ok {
		t.Fatal("missing sentinel")
	}
	if guard.RoomID != "room:east" {
		t.Fatalf("guard room = %q, want room:east", guard.RoomID)
	}
	if creatureHasAnyFlag(guard, "MPERMT", "permanent") {
		t.Fatalf("guard tags = %+v, want permanent cleared", guard.Metadata.Tags)
	}
	if !creatureHasAnyFlag(guard, "MAGGRE", "aggressive") {
		t.Fatalf("guard tags = %+v, want aggressive", guard.Metadata.Tags)
	}
	if len(world.permanentTriggers) != 1 ||
		world.permanentTriggers[0].playerID != "player:alice" ||
		world.permanentTriggers[0].creatureID != "creature:sentinel" {
		t.Fatalf("permanent triggers = %+v, want player:alice/creature:sentinel", world.permanentTriggers)
	}
	if !strings.Contains(out, "경보장치가 울립니다!\n") {
		t.Fatalf("output missing alarm message:\n%s", out)
	}
}

func TestMoveTrapLegacyPitRoomUsesTrapExitAndDeathHook(t *testing.T) {
	withMoveTrapRolls(t, 100, 7)
	loaded, _ := moveTrapLegacyWorld(t, 159, 165, 149)
	mustAddLookRoom(t, loaded, model.Room{ID: "room:1008", DisplayName: "죽으면 가는 곳"})
	mustAddMoveTrapPlayer(t, loaded, "room:00159", map[string]int{
		"dexterity": 0,
		"hpCurrent": 5,
		"hpMax":     20,
	})
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PLIGHT"}
	loaded.Creatures[alice.ID] = alice

	trapRoom := loaded.Rooms["room:00165"]
	if trapRoom.Properties["trap"] != "1" || trapRoom.Properties["trapExit"] != "149" {
		t.Fatalf("legacy room 00165 trap properties = %+v, want trap=1 trapExit=149", trapRoom.Properties)
	}
	route, ok := moveTrapFindRoomExit(loaded.Rooms["room:00159"], "room:00165")
	if !ok || route.Name != "동" {
		t.Fatalf("legacy route into pit = %+v/%t, want room:00159 동 -> room:00165", route, ok)
	}

	world := &moveTrapDeathWorld{World: state.NewWorld(loaded)}
	var broadcasts []roomBroadcastRecord
	out := dispatchMoveLineWithMoveWorldAndBroadcast(t, world, "동", &broadcasts)

	if len(world.deaths) != 1 {
		t.Fatalf("death calls = %+v, want one call", world.deaths)
	}
	if world.deaths[0].playerID != "player:alice" || world.deaths[0].attackerID != "creature:alice" {
		t.Fatalf("death call = %+v, want player:alice/creature:alice", world.deaths[0])
	}
	assertMoveWorldPlayerRoom(t, world, "room:1008")
	assertMoveTrapOutputOrder(t, out,
		"\n"+trapRoom.DisplayName+"\n\n",
		"당신은 구덩이에 빠졌습니다!\n",
		"\n"+loaded.Rooms["room:00149"].DisplayName+"\n\n",
		"당신은 7점의 피해를 입었습니다.\n",
		"당신은 죽으면서 몇가지 물건을 떨어뜨렸습니다.\n",
	)
	assertMoveTrapBroadcastOrder(t, broadcasts,
		moveTrapExpectedBroadcast{"room:00165", "Alice가 구덩이에 빠졌습니다!"},
	)
}

func TestMoveTrapLegacyAlarmRoomLoadsPermanentGuardAndStartsHostility(t *testing.T) {
	withMoveTrapRolls(t, 100)
	loaded, root := moveTrapLegacyWorld(t, 226, 227, 231)
	mustAddMoveTrapPlayer(t, loaded, "room:00226", map[string]int{
		"dexterity": 0,
		"hpCurrent": 20,
		"hpMax":     20,
	})
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PLIGHT"}
	loaded.Creatures[alice.ID] = alice
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:m00:2",
		Kind:        model.CreatureKindMonster,
		DisplayName: "경보 경비병",
		Level:       1,
		Stats: map[string]int{
			"dexterity": 10,
			"hpCurrent": 20,
			"hpMax":     20,
		},
	})

	trapRoom := loaded.Rooms["room:00227"]
	if trapRoom.Properties["trap"] != "7" || trapRoom.Properties["trapExit"] != "231" {
		t.Fatalf("legacy room 00227 trap properties = %+v, want trap=7 trapExit=231", trapRoom.Properties)
	}
	route, ok := moveTrapFindRoomExit(loaded.Rooms["room:00226"], "room:00227")
	if !ok || route.Name != "동" {
		t.Fatalf("legacy route into alarm trap = %+v/%t, want room:00226 동 -> room:00227", route, ok)
	}
	guardFile := filepath.Join(root, "rooms", "r00", "r00231")
	data, err := os.ReadFile(guardFile)
	if err != nil {
		t.Fatalf("read %s: %v", guardFile, err)
	}
	if got := moveTrapLegacyPermanentCreatureCounts(data, time.Now().Unix()); got["creature:m00:2"] != 1 {
		t.Fatalf("legacy room 00231 permanent creature counts = %+v, want creature:m00:2 once", got)
	}

	world := state.NewWorld(loaded)
	defer world.Close()
	world.SetDBRoot(root)
	var broadcasts []roomBroadcastRecord
	out := dispatchMoveLineWithMoveWorldAndBroadcast(t, world, "동", &broadcasts)

	guard, ok := moveTrapFindCreatureByNameInRoom(t, world, "room:00227", "경보 경비병")
	if !ok {
		t.Fatal("spawned permanent guard was not moved into triggered trap room 00227")
	}
	if creatureHasAnyFlag(guard, "MPERMT", "permanent") {
		t.Fatalf("guard tags = %+v, want permanent cleared after alarm activation", guard.Metadata.Tags)
	}
	if !creatureHasAnyFlag(guard, "MAGGRE", "aggressive") {
		t.Fatalf("guard tags = %+v, want aggressive", guard.Metadata.Tags)
	}
	if !creatureHasAnyFlag(guard, "was_attacked") {
		t.Fatalf("guard tags = %+v, want was_attacked combat primer", guard.Metadata.Tags)
	}
	enemies, err := world.CreatureEnemies(guard.ID)
	if err != nil {
		t.Fatalf("CreatureEnemies() error = %v", err)
	}
	if !moveTrapStringSliceContains(enemies, "Alice") {
		t.Fatalf("guard enemies = %+v, want Alice", enemies)
	}
	assertMoveTrapOutputOrder(t, out,
		"\n"+trapRoom.DisplayName+"\n\n",
		"경보장치가 울립니다!\n",
		"근처에 경비원들이 없길 바랍니다.\n",
	)
	assertMoveTrapBroadcastOrder(t, broadcasts,
		moveTrapExpectedBroadcast{"room:00227", "Alice가 경보장치를 건드렸습니다!"},
		moveTrapExpectedBroadcast{"room:00231", "경보 경비병이 경보를 듣고 조사하러 갑니다."},
		moveTrapExpectedBroadcast{"room:00227", "경보 경비병이 경보를 듣고 조사하러 왔습니다."},
	)
}

func TestMoveTrapLegacyReachablePitAndAlarmRouteFixtures(t *testing.T) {
	rooms := moveTrapLoadAllLegacyRooms(t)
	routes, trapCounts := moveTrapLegacyReachableTrapRoutes(t, rooms)

	totalTraps := 0
	for _, count := range trapCounts {
		totalTraps += count
	}
	if totalTraps != 80 {
		t.Fatalf("legacy trap room count = %d, want 80; counts=%+v", totalTraps, trapCounts)
	}
	if trapCounts[moveTrapPit] != 13 || trapCounts[moveTrapAlarm] != 25 {
		t.Fatalf("legacy pit/alarm counts = %d/%d, want 13/25; counts=%+v", trapCounts[moveTrapPit], trapCounts[moveTrapAlarm], trapCounts)
	}

	pitRoute := moveTrapLegacyTrapRoute{
		FromRoomID: "room:00159",
		ToRoomID:   "room:00165",
		ExitName:   "동",
		TrapType:   moveTrapPit,
		TrapExit:   "room:00149",
	}
	if !moveTrapLegacyRouteContains(routes, pitRoute) {
		t.Fatalf("missing reachable legacy pit route %+v in %s", pitRoute, moveTrapDescribeLegacyRoutes(routes, moveTrapPit))
	}

	alarmRoute := moveTrapLegacyTrapRoute{
		FromRoomID: "room:00226",
		ToRoomID:   "room:00227",
		ExitName:   "동",
		TrapType:   moveTrapAlarm,
		TrapExit:   "room:00231",
	}
	if !moveTrapLegacyRouteContains(routes, alarmRoute) {
		t.Fatalf("missing reachable legacy alarm route %+v in %s", alarmRoute, moveTrapDescribeLegacyRoutes(routes, moveTrapAlarm))
	}
}

func moveTrapWorld(t *testing.T, trapType string, trapExit string) *worldload.World {
	t.Helper()

	loaded := lookWorld(t)
	east := loaded.Rooms["room:east"]
	east.Properties = map[string]string{"trap": trapType}
	if trapExit != "" {
		east.Properties["trapExit"] = trapExit
	}
	loaded.Rooms[east.ID] = east
	return loaded
}

func moveTrapLegacyWorld(t *testing.T, roomNumbers ...int) (*worldload.World, string) {
	t.Helper()

	root := moveTrapLegacyRoot(t)
	loaded := worldload.NewWorld()
	for _, roomNumber := range roomNumbers {
		moveTrapAddLegacyRoom(t, loaded, root, roomNumber)
	}
	return loaded, root
}

func moveTrapLoadAllLegacyRooms(t *testing.T) map[model.RoomID]model.Room {
	t.Helper()

	root := moveTrapLegacyRoot(t)
	roomsDir := filepath.Join(root, "rooms")
	rooms := map[model.RoomID]model.Room{}
	err := filepath.WalkDir(roomsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasPrefix(d.Name(), "r") || len(d.Name()) != len("r00000") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return nil
		}
		room, _, err := roommap.MapRoomFile(path, data)
		if err != nil {
			return err
		}
		rooms[room.ID] = room
		return nil
	})
	if err != nil {
		t.Fatalf("load legacy rooms: %v", err)
	}
	return rooms
}

type moveTrapLegacyTrapRoute struct {
	FromRoomID model.RoomID
	ToRoomID   model.RoomID
	ExitName   string
	TrapType   int
	TrapExit   model.RoomID
}

func moveTrapLegacyReachableTrapRoutes(t *testing.T, rooms map[model.RoomID]model.Room) ([]moveTrapLegacyTrapRoute, map[int]int) {
	t.Helper()

	trapCounts := map[int]int{}
	for _, room := range rooms {
		trapType, ok := moveRoomTrapType(room)
		if !ok || trapType == 0 {
			continue
		}
		trapCounts[trapType]++
	}

	var routes []moveTrapLegacyTrapRoute
	for _, fromRoom := range rooms {
		for _, exit := range fromRoom.Exits {
			if exitHasAnyFlag(exit, "closed", "xclosd", "xclosed", "locked", "xlockd", "xlocked", "noSee", "xnosee") {
				continue
			}
			toRoom, ok := rooms[exit.ToRoomID]
			if !ok {
				continue
			}
			trapType, ok := moveRoomTrapType(toRoom)
			if !ok || trapType == 0 {
				continue
			}
			routes = append(routes, moveTrapLegacyTrapRoute{
				FromRoomID: fromRoom.ID,
				ToRoomID:   toRoom.ID,
				ExitName:   exit.Name,
				TrapType:   trapType,
				TrapExit:   moveTrapLegacyTrapExitID(rooms, toRoom),
			})
		}
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].TrapType != routes[j].TrapType {
			return routes[i].TrapType < routes[j].TrapType
		}
		if routes[i].FromRoomID != routes[j].FromRoomID {
			return routes[i].FromRoomID < routes[j].FromRoomID
		}
		if routes[i].ToRoomID != routes[j].ToRoomID {
			return routes[i].ToRoomID < routes[j].ToRoomID
		}
		return routes[i].ExitName < routes[j].ExitName
	})
	return routes, trapCounts
}

func moveTrapLegacyTrapExitID(rooms map[model.RoomID]model.Room, room model.Room) model.RoomID {
	raw, ok := moveRoomTrapExitRaw(room)
	if !ok {
		return ""
	}
	for _, roomID := range moveTrapRoomIDCandidates(raw) {
		if _, ok := rooms[roomID]; ok {
			return roomID
		}
	}
	return ""
}

func moveTrapLegacyRouteContains(routes []moveTrapLegacyTrapRoute, want moveTrapLegacyTrapRoute) bool {
	for _, route := range routes {
		if route == want {
			return true
		}
	}
	return false
}

func moveTrapDescribeLegacyRoutes(routes []moveTrapLegacyTrapRoute, trapType int) string {
	var parts []string
	for _, route := range routes {
		if route.TrapType != trapType {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s -> %s exit=%s", route.FromRoomID, route.ExitName, route.ToRoomID, route.TrapExit))
		if len(parts) >= 12 {
			break
		}
	}
	return strings.Join(parts, "; ")
}

func moveTrapLegacyRoot(t *testing.T) string {
	t.Helper()
	if os.Getenv("CI") != "" || testing.Short() {
		t.Skip("skipping test requiring legacy rooms/ in CI or short mode")
	}

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "rooms")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root with go.mod and rooms")
		}
		dir = parent
	}
}

func moveTrapAddLegacyRoom(t *testing.T, loaded *worldload.World, root string, roomNumber int) {
	t.Helper()

	path := filepath.Join(root, "rooms", fmt.Sprintf("r%02d", roomNumber/1000), fmt.Sprintf("r%05d", roomNumber))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read legacy room %s: %v", path, err)
	}
	bundle, err := roommap.MapRoomFileBundle(path, data)
	if err != nil {
		t.Fatalf("map legacy room %s: %v", path, err)
	}
	if err := loaded.AddRoom(bundle.Room); err != nil {
		t.Fatalf("add legacy room %s: %v", bundle.Room.ID, err)
	}
	for _, creature := range bundle.Creatures {
		if err := loaded.AddCreature(creature); err != nil {
			t.Fatalf("add legacy room creature %s: %v", creature.ID, err)
		}
	}
	for _, object := range bundle.Objects {
		if err := loaded.AddObjectInstance(object); err != nil {
			t.Fatalf("add legacy room object %s: %v", object.ID, err)
		}
	}
}

func mustAddMoveTrapPlayer(t *testing.T, loaded *worldload.World, roomID model.RoomID, stats map[string]int) {
	t.Helper()

	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      roomID,
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      roomID,
		Stats:       stats,
	})
}

func moveTrapFindCreatureByNameInRoom(t *testing.T, world MoveWorld, roomID model.RoomID, name string) (model.Creature, bool) {
	t.Helper()

	room, ok := world.Room(roomID)
	if !ok {
		t.Fatalf("room %s not found", roomID)
	}
	for _, creatureID := range room.CreatureIDs {
		creature, ok := world.Creature(creatureID)
		if ok && creature.DisplayName == name {
			return creature, true
		}
	}
	return model.Creature{}, false
}

func moveTrapFindRoomExit(room model.Room, toRoomID model.RoomID) (model.Exit, bool) {
	for _, exit := range room.Exits {
		if exit.ToRoomID == toRoomID {
			return exit, true
		}
	}
	return model.Exit{}, false
}

func dispatchMoveLineWithMoveWorldAndBroadcast(t *testing.T, world MoveWorld, line string, records *[]roomBroadcastRecord) string {
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

	ctx := contextWithRoomBroadcast("player:alice", "session:alice", records)
	status, err := dispatcher.DispatchLine(ctx, line)
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	return ctx.OutputString()
}

type moveTrapExpectedBroadcast struct {
	roomID string
	text   string
}

func assertMoveTrapBroadcastOrder(t *testing.T, records []roomBroadcastRecord, wants ...moveTrapExpectedBroadcast) {
	t.Helper()

	index := 0
	for _, want := range wants {
		found := false
		for index < len(records) {
			record := records[index]
			index++
			if record.RoomID == want.roomID && strings.Contains(record.Text, want.text) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("broadcasts missing %+v after index %d:\n%+v", want, index, records)
		}
	}
}

func setMoveTrapActorStats(t *testing.T, loaded *worldload.World, stats map[string]int) {
	t.Helper()

	alice := loaded.Creatures["creature:alice"]
	if alice.Stats == nil {
		alice.Stats = map[string]int{}
	}
	for key, value := range stats {
		alice.Stats[key] = value
	}
	loaded.Creatures[alice.ID] = alice
}

func withMoveTrapRolls(t *testing.T, rolls ...int) {
	t.Helper()

	previous := attackRoll
	index := 0
	attackRoll = func(min int, max int) int {
		if index >= len(rolls) {
			t.Fatalf("unexpected attackRoll(%d, %d)", min, max)
		}
		value := rolls[index]
		index++
		return value
	}
	t.Cleanup(func() { attackRoll = previous })
}

func assertMoveTrapOutputOrder(t *testing.T, out string, parts ...string) {
	t.Helper()

	pos := 0
	for _, part := range parts {
		next := strings.Index(out[pos:], part)
		if next < 0 {
			t.Fatalf("output missing %q after offset %d:\n%s", part, pos, out)
		}
		pos += next + len(part)
	}
}

type moveTrapDeathCall struct {
	playerID   model.PlayerID
	attackerID model.CreatureID
}

type moveTrapDeathWorld struct {
	*state.World
	deaths []moveTrapDeathCall
}

func (w *moveTrapDeathWorld) PlayerDeath(playerID model.PlayerID, attackerID model.CreatureID) error {
	w.deaths = append(w.deaths, moveTrapDeathCall{playerID: playerID, attackerID: attackerID})
	return w.MovePlayerToRoom(playerID, "room:1008")
}

type moveTrapPermanentTriggerCall struct {
	playerID   model.PlayerID
	creatureID model.CreatureID
}

type moveTrapPermanentHookWorld struct {
	*state.World
	permanentTriggers []moveTrapPermanentTriggerCall
}

func (w *moveTrapPermanentHookWorld) ActivatePermanentCreatureForTrap(playerID model.PlayerID, creatureID model.CreatureID) error {
	w.permanentTriggers = append(w.permanentTriggers, moveTrapPermanentTriggerCall{playerID: playerID, creatureID: creatureID})
	return nil
}

type moveTrapPopulatePermanentHookWorld struct {
	*moveTrapPermanentHookWorld
	loadedRooms    []model.RoomID
	populatedRooms []model.RoomID
	roomOverrides  map[model.RoomID]model.Room
}

func (w *moveTrapPopulatePermanentHookWorld) LoadRoom(roomID model.RoomID) error {
	w.loadedRooms = append(w.loadedRooms, roomID)
	if w.roomOverrides == nil {
		w.roomOverrides = map[model.RoomID]model.Room{}
	}
	w.roomOverrides[roomID] = model.Room{ID: roomID, DisplayName: "경비실"}
	return nil
}

func (w *moveTrapPopulatePermanentHookWorld) AddPermanentCreaturesToRoom(roomID model.RoomID) error {
	w.populatedRooms = append(w.populatedRooms, roomID)
	room, ok := w.Room(roomID)
	if !ok {
		return nil
	}
	room.CreatureIDs = append(room.CreatureIDs, "creature:sentinel")
	if w.roomOverrides == nil {
		w.roomOverrides = map[model.RoomID]model.Room{}
	}
	w.roomOverrides[roomID] = room
	return nil
}

func (w *moveTrapPopulatePermanentHookWorld) Room(roomID model.RoomID) (model.Room, bool) {
	if room, ok := w.roomOverrides[roomID]; ok {
		return room, true
	}
	return w.moveTrapPermanentHookWorld.Room(roomID)
}

type moveTrapEffectExpirationWorldHook struct {
	*state.World
	expirations map[model.CreatureID]map[string]int64
}

func (w *moveTrapEffectExpirationWorldHook) SetEffectExpiration(creatureID model.CreatureID, tag string, expires int64) {
	if w.expirations == nil {
		w.expirations = map[model.CreatureID]map[string]int64{}
	}
	if w.expirations[creatureID] == nil {
		w.expirations[creatureID] = map[string]int64{}
	}
	w.expirations[creatureID][tag] = expires
}

func moveTrapStringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
