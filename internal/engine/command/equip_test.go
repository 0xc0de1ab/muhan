package command

import (
	"strconv"
	"strings"
	"testing"

	"muhan/internal/commandspec"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestEquipmentMutationHandlers(t *testing.T) {
	loaded := equipmentWorld(t)
	world := state.NewWorld(loaded)
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "입어", Number: 9, Handler: "wear"},
			{Name: "벗어", Number: 10, Handler: "remove_obj"},
			{Name: "쥐어", Number: 12, Handler: "hold"},
			{Name: "무장", Number: 13, Handler: "ready"},
			{Name: "장비", Number: 11, Handler: "equipment"},
		}),
		Handlers: map[string]Handler{
			"wear":       NewWearHandler(world),
			"remove_obj": NewRemoveObjectHandler(world),
			"hold":       NewHoldHandler(world),
			"ready":      NewReadyHandler(world),
			"equipment":  NewEquipmentHandler(world),
		},
	}

	assertEquipDispatchContains(t, dispatcher, "목검 무장", "당신은 목검으로 전투태세를 취합니다.")
	assertEquippedSlot(t, world, "wield", "object:sword")
	assertObjectHasLegacyEquipFlag(t, world, "object:sword", "OWEARS")

	assertEquipDispatchContains(t, dispatcher, "갑옷 입어", "당신은 갑옷을 입었습니다.")
	assertEquippedSlot(t, world, "body", "object:armor")
	assertObjectHasLegacyEquipFlag(t, world, "object:armor", "OWEARS")

	assertEquipDispatchContains(t, dispatcher, "부적 쥐어", "당신은 부적을 쥐었습니다.")
	assertEquippedSlot(t, world, "held", "object:charm")
	assertObjectHasLegacyEquipFlag(t, world, "object:charm", "OWEARS")
	assertObjectLacksLegacyEquipFlag(t, world, "object:charm", "OWHELD")

	assertEquipDispatchContains(t, dispatcher, "장비",
		"  <<<  착용 장비  >>>  \n",
		"[  몸  ]  갑옷\n",
		"[쥔물건]  부적\n",
		"[ 무기 ]  목검\n",
	)

	assertEquipDispatchContains(t, dispatcher, "목검 벗어", "당신은 목검을 벗었습니다.")
	assertInventorySlot(t, world, "object:sword")
	assertObjectLacksLegacyEquipFlag(t, world, "object:sword", "OWEARS")
}

func TestEquipHandlersClearHiddenAfterArgumentLikeLegacy(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		setup func(*testing.T, *state.World)
		want  string
	}{
		{name: "wear", line: "갑옷 입어", want: "당신은 갑옷을 입었습니다."},
		{name: "ready", line: "목검 무장", want: "당신은 목검으로 전투태세를 취합니다."},
		{name: "hold", line: "부적 쥐어", want: "당신은 부적을 쥐었습니다."},
		{
			name: "remove",
			line: "갑옷 벗어",
			setup: func(t *testing.T, world *state.World) {
				t.Helper()
				if err := world.MoveObject("object:armor", model.ObjectLocation{CreatureID: "creature:alice", Slot: "body"}); err != nil {
					t.Fatalf("MoveObject() error = %v", err)
				}
			},
			want: "당신은 갑옷을 벗었습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := state.NewWorld(equipmentWorld(t))
			if tt.setup != nil {
				tt.setup(t, world)
			}
			if _, err := world.UpdateCreatureTags("creature:alice", []string{"hidden", "PHIDDN"}, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := world.UpdatePlayerTags("player:alice", []string{"hidden", "PHIDDN"}, nil); err != nil {
				t.Fatal(err)
			}
			if err := world.SetCreatureStat("creature:alice", "PHIDDN", 1); err != nil {
				t.Fatal(err)
			}
			dispatcher := Dispatcher{
				Registry: mustRegistry(t, []commandspec.CommandSpec{
					{Name: "입어", Number: 9, Handler: "wear"},
					{Name: "벗어", Number: 10, Handler: "remove_obj"},
					{Name: "쥐어", Number: 12, Handler: "hold"},
					{Name: "무장", Number: 13, Handler: "ready"},
				}),
				Handlers: map[string]Handler{
					"wear":       NewWearHandler(world),
					"remove_obj": NewRemoveObjectHandler(world),
					"hold":       NewHoldHandler(world),
					"ready":      NewReadyHandler(world),
				},
			}

			assertEquipDispatchContains(t, dispatcher, tt.line, tt.want)

			creature, ok := world.Creature("creature:alice")
			if !ok {
				t.Fatal("alice creature missing")
			}
			if hasAnyNormalizedFlag(creature.Metadata.Tags, "hidden", "phiddn", "PHIDDN") || creature.Stats["PHIDDN"] != 0 {
				t.Fatalf("creature hidden state = tags:%+v stats:%+v", creature.Metadata.Tags, creature.Stats)
			}
			player, ok := world.Player("player:alice")
			if !ok {
				t.Fatal("alice player missing")
			}
			if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "phiddn", "PHIDDN") {
				t.Fatalf("player hidden tags = %+v", player.Metadata.Tags)
			}
		})
	}
}

func TestReadyRejectsNonWeaponAndWearRejectsWeapon(t *testing.T) {
	world := state.NewWorld(equipmentWorld(t))
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "입어", Number: 9, Handler: "wear"},
			{Name: "무장", Number: 13, Handler: "ready"},
		}),
		Handlers: map[string]Handler{
			"wear":  NewWearHandler(world),
			"ready": NewReadyHandler(world),
		},
	}

	assertEquipDispatchContains(t, dispatcher, "목검 입어", "목검은 입는 물건이 아닙니다.")
	assertEquipDispatchContains(t, dispatcher, "갑옷 무장", "당신은 그것을 무장할 수 없습니다.")
}

func TestEquipHandlersUseLegacyMissingArgumentNewlines(t *testing.T) {
	world := state.NewWorld(equipmentWorld(t))
	dispatcher := fullEquipDispatcher(t, world)

	assertEquipDispatchExact(t, dispatcher, "입어", "뭘 입으실려구요?\n")
	assertEquipDispatchExact(t, dispatcher, "벗어", "뭘 벗고 싶으세요?")
	assertEquipDispatchExact(t, dispatcher, "무장", "무엇을 무장하시려구요?")
	assertEquipDispatchExact(t, dispatcher, "쥐어", "무엇을 쥐실려구요?")
}

func TestWearAllNoEligibleObjectsUsesLegacyNoNewlineMessage(t *testing.T) {
	loaded := equipmentWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = []model.ObjectInstanceID{"object:sword", "object:charm"}
	loaded.Creatures[creature.ID] = creature
	world := state.NewWorld(loaded)
	dispatcher := fullEquipDispatcher(t, world)

	assertEquipDispatchExact(t, dispatcher, "모두 입어", "당신은 입을 물건을 가지고 있지 않습니다.")
	assertInventorySlot(t, world, "object:sword")
	assertInventorySlot(t, world, "object:charm")
}

func TestReadyRejectsObjectIDTargetLikeLegacyFindObj(t *testing.T) {
	loaded := equipmentWorld(t)
	world := state.NewWorld(loaded)
	dispatcher := fullEquipDispatcher(t, world)

	assertEquipDispatchExact(t, dispatcher, "object:sword 무장", "당신은 그런 물건을 가지고 있지 않습니다.")
	assertInventorySlot(t, world, "object:sword")
}

func TestReadyUsesLegacyPrefixOrderInsteadOfExactFirst(t *testing.T) {
	loaded := equipmentWorld(t)
	proto := loaded.ObjectPrototypes["prototype:sword"]
	proto.DisplayName = "목검 조각"
	proto.Properties = map[string]string{"name": "목검 조각", "type": "1", "wearFlag": "20"}
	loaded.ObjectPrototypes[proto.ID] = proto
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:sword-exact",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "목검",
		Properties:  map[string]string{"name": "목검", "type": "1", "wearFlag": "20"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:sword-exact",
		PrototypeID: "prototype:sword-exact",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	})
	alice := loaded.Creatures["creature:alice"]
	alice.Inventory.ObjectIDs = []model.ObjectInstanceID{"object:sword", "object:sword-exact", "object:armor", "object:charm"}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	dispatcher := fullEquipDispatcher(t, world)

	assertEquipDispatchExact(t, dispatcher, "목검 무장", "당신은 목검 조각으로 전투태세를 취합니다.")
	assertEquippedSlot(t, world, "wield", "object:sword")
	assertInventorySlot(t, world, "object:sword-exact")
}

func TestReadyFindObjVisibilityUsesPDINVI(t *testing.T) {
	loaded := equipmentWorld(t)
	sword := loaded.Objects["object:sword"]
	sword.Metadata.Tags = []string{"OINVIS"}
	loaded.Objects[sword.ID] = sword
	world := state.NewWorld(loaded)
	dispatcher := fullEquipDispatcher(t, world)

	assertEquipDispatchExact(t, dispatcher, "목검 무장", "당신은 그런 물건을 가지고 있지 않습니다.")
	assertInventorySlot(t, world, "object:sword")

	loaded = equipmentWorld(t)
	sword = loaded.Objects["object:sword"]
	sword.Metadata.Tags = []string{"OINVIS"}
	loaded.Objects[sword.ID] = sword
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"PDINVI"}
	loaded.Creatures[creature.ID] = creature
	world = state.NewWorld(loaded)
	dispatcher = fullEquipDispatcher(t, world)

	assertEquipDispatchExact(t, dispatcher, "목검 무장", "당신은 목검으로 전투태세를 취합니다.")
	assertEquippedSlot(t, world, "wield", "object:sword")
}

func TestRemoveFindObjVisibilityUsesPDINVI(t *testing.T) {
	loaded := equipmentWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = removeEquipObjectID(creature.Inventory.ObjectIDs, "object:armor")
	creature.Equipment = map[string]model.ObjectInstanceID{"body": "object:armor"}
	loaded.Creatures[creature.ID] = creature
	armor := loaded.Objects["object:armor"]
	armor.Location = model.ObjectLocation{CreatureID: "creature:alice", Slot: "body"}
	armor.Metadata.Tags = []string{"OINVIS"}
	loaded.Objects[armor.ID] = armor
	world := state.NewWorld(loaded)
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{{Name: "벗어", Number: 10, Handler: "remove_obj"}}),
		Handlers: map[string]Handler{"remove_obj": NewRemoveObjectHandler(world)},
	}

	assertEquipDispatchExact(t, dispatcher, "갑옷 벗어", "당신은 그것을 입고 있지 않습니다.")
	assertEquippedSlot(t, world, "body", "object:armor")

	loaded = equipmentWorld(t)
	creature = loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = removeEquipObjectID(creature.Inventory.ObjectIDs, "object:armor")
	creature.Equipment = map[string]model.ObjectInstanceID{"body": "object:armor"}
	creature.Metadata.Tags = []string{"PDINVI"}
	loaded.Creatures[creature.ID] = creature
	armor = loaded.Objects["object:armor"]
	armor.Location = model.ObjectLocation{CreatureID: "creature:alice", Slot: "body"}
	armor.Metadata.Tags = []string{"OINVIS"}
	loaded.Objects[armor.ID] = armor
	world = state.NewWorld(loaded)
	dispatcher = Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{{Name: "벗어", Number: 10, Handler: "remove_obj"}}),
		Handlers: map[string]Handler{"remove_obj": NewRemoveObjectHandler(world)},
	}

	assertEquipDispatchExact(t, dispatcher, "갑옷 벗어", "당신은 갑옷을 벗었습니다.")
	assertInventorySlot(t, world, "object:armor")
}

func TestWearHandlerUsesLegacySlotSpecificMessages(t *testing.T) {
	tests := []struct {
		name     string
		objectID model.ObjectInstanceID
		display  string
		flag     int
		line     string
		want     string
		slot     string
	}{
		{name: "arms", objectID: "object:armguard", display: "팔갑", flag: legacyWearArms, line: "팔갑 입어", want: "당신은 팔갑을 팔에 장착합니다.", slot: "arms"},
		{name: "legs", objectID: "object:pants", display: "바지", flag: legacyWearLegs, line: "바지 입어", want: "당신은 바지를 입습니다.", slot: "legs"},
		{name: "neck", objectID: "object:necklace", display: "목걸이", flag: legacyWearNeck, line: "목걸이 입어", want: "당신은 목걸이를 목에 두릅니다.", slot: "neck1"},
		{name: "hands", objectID: "object:gloves", display: "장갑", flag: legacyWearHands, line: "장갑 입어", want: "당신은 장갑을 손에 끼웁니다.", slot: "hands"},
		{name: "head", objectID: "object:helmet", display: "투구", flag: legacyWearHead, line: "투구 입어", want: "당신은 투구를 머리에 씁니다.", slot: "head"},
		{name: "feet", objectID: "object:boots", display: "신발", flag: legacyWearFeet, line: "신발 입어", want: "당신은 신발을 신었습니다.", slot: "feet"},
		{name: "finger", objectID: "object:ring", display: "반지", flag: legacyWearFinger, line: "반지 입어", want: "당신은 반지를 손가락에 끼웁니다.", slot: "finger1"},
		{name: "shield", objectID: "object:shield", display: "방패", flag: legacyWearShield, line: "방패 입어", want: "당신은 방패를 방패로 삼습니다.", slot: "shield"},
		{name: "face", objectID: "object:mask", display: "가면", flag: legacyWearFace, line: "가면 입어", want: "당신은 가면을 얼굴에 씁니다.", slot: "face"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := equipmentWorld(t)
			addEquipTestObject(t, loaded, tt.objectID, tt.display, tt.flag, "inventory")
			world := state.NewWorld(loaded)
			dispatcher := equipOnlyDispatcher(t, world)

			assertEquipDispatchContains(t, dispatcher, tt.line, tt.want)
			assertEquippedSlot(t, world, tt.slot, tt.objectID)
		})
	}
}

func TestWearHandlerUsesLegacyOccupiedAndFullSlotMessages(t *testing.T) {
	t.Run("occupied head", func(t *testing.T) {
		loaded := equipmentWorld(t)
		addEquipTestObject(t, loaded, "object:old-helmet", "헌투구", legacyWearHead, "head")
		addEquipTestObject(t, loaded, "object:new-helmet", "새투구", legacyWearHead, "inventory")
		world := state.NewWorld(loaded)
		dispatcher := equipOnlyDispatcher(t, world)

		assertEquipDispatchContains(t, dispatcher, "새투구 입어", "당신은 이미 헌투구를 머리에 쓰고 있습니다.")
		assertEquippedSlot(t, world, "head", "object:old-helmet")
		assertInventorySlot(t, world, "object:new-helmet")
	})

	t.Run("full neck", func(t *testing.T) {
		loaded := equipmentWorld(t)
		addEquipTestObject(t, loaded, "object:neck-1", "첫목걸이", legacyWearNeck, "neck1")
		addEquipTestObject(t, loaded, "object:neck-2", "둘목걸이", legacyWearNeck, "neck2")
		addEquipTestObject(t, loaded, "object:neck-new", "새목걸이", legacyWearNeck, "inventory")
		world := state.NewWorld(loaded)
		dispatcher := equipOnlyDispatcher(t, world)

		assertEquipDispatchContains(t, dispatcher, "새목걸이 입어", "더이상 목에 걸 수 없습니다.")
		assertInventorySlot(t, world, "object:neck-new")
	})

	t.Run("full fingers", func(t *testing.T) {
		loaded := equipmentWorld(t)
		for i := 1; i <= 8; i++ {
			addEquipTestObject(t, loaded, model.ObjectInstanceID("object:ring-"+strconv.Itoa(i)), "반지"+strconv.Itoa(i), legacyWearFinger, "finger"+strconv.Itoa(i))
		}
		addEquipTestObject(t, loaded, "object:ring-new", "새반지", legacyWearFinger, "inventory")
		world := state.NewWorld(loaded)
		dispatcher := equipOnlyDispatcher(t, world)

		assertEquipDispatchContains(t, dispatcher, "새반지 입어", "더이상 손가락에 낄 수 없습니다.")
		assertInventorySlot(t, world, "object:ring-new")
	})
}

func TestEquipHandlersPrintLegacyUseOutput(t *testing.T) {
	loaded := equipmentWorld(t)
	addEquipTestObjectWithProperties(t, loaded, "object:robe", "의복", map[string]string{
		"type":      "5",
		"wearFlag":  "1",
		"useOutput": "의복이 빛난다.",
	}, "inventory")
	addEquipTestObjectWithProperties(t, loaded, "object:singing-sword", "노래검", map[string]string{
		"type":       "1",
		"wearFlag":   "20",
		"use_output": "검이 운다.",
	}, "inventory")
	addEquipTestObjectWithProperties(t, loaded, "object:warm-charm", "온부적", map[string]string{
		"type":      "13",
		"wearFlag":  "17",
		"useOutput": "부적이 따뜻하다.",
	}, "inventory")
	addEquipTestObjectWithProperties(t, loaded, "object:potion-charm", "물약부적", map[string]string{
		"type":      strconv.Itoa(legacyObjectPotion),
		"wearFlag":  "17",
		"useOutput": "이 문구는 쥐기에서 출력되지 않는다.",
	}, "inventory")

	world := state.NewWorld(loaded)
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "입어", Number: 9, Handler: "wear"},
			{Name: "쥐어", Number: 12, Handler: "hold"},
			{Name: "무장", Number: 13, Handler: "ready"},
		}),
		Handlers: map[string]Handler{
			"wear":  NewWearHandler(world),
			"hold":  NewHoldHandler(world),
			"ready": NewReadyHandler(world),
		},
	}

	assertEquipDispatchContains(t, dispatcher, "의복 입어", "당신은 의복을 입었습니다.\n의복이 빛난다.")
	assertEquipDispatchContains(t, dispatcher, "노래검 무장", "당신은 노래검으로 전투태세를 취합니다.\n검이 운다.")
	assertEquipDispatchContains(t, dispatcher, "온부적 쥐어", "당신은 온부적을 쥐었습니다.\n부적이 따뜻하다.")

	holdWorld := state.NewWorld(loaded)
	holdDispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{{Name: "쥐어", Number: 12, Handler: "hold"}}),
		Handlers: map[string]Handler{
			"hold": NewHoldHandler(holdWorld),
		},
	}
	assertEquipDispatchExact(t, holdDispatcher, "물약부적 쥐어", "당신은 물약부적을 쥐었습니다.")
}

func TestWearAllPrintsLegacyUseOutput(t *testing.T) {
	loaded := equipmentWorld(t)
	addEquipTestObjectWithProperties(t, loaded, "object:glowing-sleeve", "광휘소매", map[string]string{
		"type":      "5",
		"wearFlag":  strconv.Itoa(legacyWearArms),
		"useOutput": "소매가 환하게 빛난다.",
	}, "inventory")
	world := state.NewWorld(loaded)
	dispatcher := equipOnlyDispatcher(t, world)

	assertEquipDispatchContains(t, dispatcher, "모두 입어",
		"소매가 환하게 빛난다.\n",
		"당신은 입을 수 있는 장비를 모두 입었습니다.",
	)
	assertEquippedSlot(t, world, "arms", "object:glowing-sleeve")
}

func TestEquipHandlersRejectLegacyWearReadyHoldRestrictions(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		objectID   model.ObjectInstanceID
		display    string
		properties map[string]string
		stats      map[string]int
		tags       []string
		want       string
	}{
		{
			name:     "wear no mage armor",
			line:     "금갑옷 입어",
			objectID: "object:no-mage-armor",
			display:  "금갑옷",
			properties: map[string]string{
				"type": "5", "wearFlag": "1", "noMage": "1",
			},
			stats: map[string]int{"class": model.ClassMage},
			want:  "도술사, 불제자들은 사용할수 없습니다.",
		},
		{
			name:     "wear male only armor rejects non-male",
			line:     "남갑옷 입어",
			objectID: "object:male-armor",
			display:  "남갑옷",
			properties: map[string]string{
				"type": "5", "wearFlag": "1", "maleOnly": "1",
			},
			want: "남갑옷은 남자들만 입을 수 있습니다.",
		},
		{
			name:     "wear female only armor rejects male",
			line:     "여갑옷 입어",
			objectID: "object:female-armor",
			display:  "여갑옷",
			properties: map[string]string{
				"type": "5", "wearFlag": "1", "femaleOnly": "1",
			},
			tags: []string{"PMALES"},
			want: "여갑옷은 여자들만 입을 수 있습니다.",
		},
		{
			name:     "wear marriage only armor rejects unmarried",
			line:     "예복 입어",
			objectID: "object:marriage-armor",
			display:  "예복",
			properties: map[string]string{
				"type": "5", "wearFlag": "1", "marriageOnly": "1",
			},
			want: "예복은 결혼한 사람들만 입을 수 있습니다.",
		},
		{
			name:     "wear class selective armor rejects unlisted class",
			line:     "직업갑 입어",
			objectID: "object:class-armor",
			display:  "직업갑",
			properties: map[string]string{
				"type": "5", "wearFlag": "1", "classSelective": "1", "classMage": "1",
			},
			stats: map[string]int{"class": model.ClassFighter},
			want:  "직업갑은 당신의 직업에 맞지 않습니다.",
		},
		{
			name:     "wear size armor rejects wrong race",
			line:     "작은갑 입어",
			objectID: "object:small-armor",
			display:  "작은갑",
			properties: map[string]string{
				"type": "5", "wearFlag": "1", "OSIZE2": "1",
			},
			stats: map[string]int{"race": legacyRaceHuman},
			want:  "작은갑이 당신 몸에 맞지 않습니다.",
		},
		{
			name:     "wear broken armor rejects zero shots",
			line:     "깨진갑 입어",
			objectID: "object:broken-armor",
			display:  "깨진갑",
			properties: map[string]string{
				"type": "5", "wearFlag": "1", "shotsCurrent": "0",
			},
			want: "깨진갑은 부서져서 입을 수 없게 되었습니다.",
		},
		{
			name:     "ready high sharp weapon rejects mage",
			line:     "대검 무장",
			objectID: "object:high-sword",
			display:  "대검",
			properties: map[string]string{
				"type": "1", "wearFlag": "20", "nDice": "3", "sDice": "5", "pDice": "0",
			},
			stats: map[string]int{"class": model.ClassMage},
			want:  "도술사, 불제자는 사용할수 없습니다.",
		},
		{
			name:     "hold quest item rejects",
			line:     "임무부적 쥐어",
			objectID: "object:quest-charm",
			display:  "임무부적",
			properties: map[string]string{
				"type": "13", "wearFlag": "17", "questNumber": "4",
			},
			want: "당신은 그것을 쥘 수 없습니다.",
		},
		{
			name:     "hold class selective item rejects unlisted class",
			line:     "직업부적 쥐어",
			objectID: "object:class-charm",
			display:  "직업부적",
			properties: map[string]string{
				"type": "13", "wearFlag": "17", "classSelective": "1", "classMage": "1",
			},
			stats: map[string]int{"class": model.ClassFighter},
			want:  "직업부적은 당신의 직업에 맞지 않습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := equipmentWorld(t)
			addEquipTestObjectWithProperties(t, loaded, tt.objectID, tt.display, tt.properties, "inventory")
			setEquipCreatureStatsAndTags(t, loaded, tt.stats, tt.tags)
			world := state.NewWorld(loaded)
			dispatcher := fullEquipDispatcher(t, world)

			assertEquipDispatchExact(t, dispatcher, tt.line, tt.want)
			assertInventorySlot(t, world, tt.objectID)
		})
	}
}

func TestEquipAlignmentRestrictionDropsObjectToRoom(t *testing.T) {
	loaded := equipmentWorld(t)
	mustAddLookRoom(t, loaded, model.Room{ID: "room:plaza", DisplayName: "광장"})
	player := loaded.Players["player:alice"]
	player.RoomID = "room:plaza"
	loaded.Players[player.ID] = player
	creature := loaded.Creatures["creature:alice"]
	creature.RoomID = "room:plaza"
	creature.Stats = map[string]int{"alignment": -75}
	loaded.Creatures[creature.ID] = creature
	addEquipTestObjectWithProperties(t, loaded, "object:holy-sword", "성검", map[string]string{
		"type": "1", "wearFlag": "20", "goodOnly": "1",
	}, "inventory")
	world := state.NewWorld(loaded)
	dispatcher := fullEquipDispatcher(t, world)

	assertEquipDispatchExact(t, dispatcher, "성검 무장", "성검이 당신의 몸에서 튕겨져 나가 바닥에 떨어집니다.")
	assertObjectRoomSlot(t, world, "object:holy-sword", "room:plaza")
}

func TestRemoveRejectsLegacyPropertyCursedEquipment(t *testing.T) {
	loaded := equipmentWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = removeEquipObjectID(creature.Inventory.ObjectIDs, "object:armor")
	creature.Equipment = map[string]model.ObjectInstanceID{"body": "object:armor"}
	loaded.Creatures[creature.ID] = creature
	armor := loaded.Objects["object:armor"]
	armor.Location = model.ObjectLocation{CreatureID: "creature:alice", Slot: "body"}
	armor.Properties = map[string]string{"OCURSE": "1"}
	loaded.Objects[armor.ID] = armor
	world := state.NewWorld(loaded)
	handler := NewRemoveObjectHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"갑옷"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "이쿠! 그것이 몸에서 떨어지지 않습니다! 저주받은 물건 같군요." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	assertEquippedSlot(t, world, "body", "object:armor")
}

func TestRemoveAllSkipsLegacyPropertyCursedEquipment(t *testing.T) {
	loaded := equipmentWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = removeEquipObjectID(creature.Inventory.ObjectIDs, "object:armor")
	creature.Equipment = map[string]model.ObjectInstanceID{"body": "object:armor"}
	loaded.Creatures[creature.ID] = creature
	armor := loaded.Objects["object:armor"]
	armor.Location = model.ObjectLocation{CreatureID: "creature:alice", Slot: "body"}
	armor.Properties = map[string]string{"ocurse": "true"}
	loaded.Objects[armor.ID] = armor
	world := state.NewWorld(loaded)
	handler := NewRemoveObjectHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"모두"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != legacyEquipmentEmptyMessage {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	assertEquippedSlot(t, world, "body", "object:armor")
}

func TestRemoveObjectClearsOnlyWornFlagFromHeldItemLikeLegacy(t *testing.T) {
	loaded := equipmentWorld(t)
	addEquipTestObjectWithProperties(t, loaded, "object:held-dagger", "단검", map[string]string{
		"type":     strconv.Itoa(legacyObjectThrust),
		"wearFlag": strconv.Itoa(legacyWearHeld),
	}, "inventory")
	world := state.NewWorld(loaded)
	dispatcher := fullEquipDispatcher(t, world)

	assertEquipDispatchExact(t, dispatcher, "단검 쥐어", "당신은 단검을 쥐었습니다.")
	assertObjectHasLegacyEquipFlag(t, world, "object:held-dagger", "OWEARS")
	assertObjectHasLegacyEquipFlag(t, world, "object:held-dagger", "OWHELD")

	assertEquipDispatchExact(t, dispatcher, "단검 벗어", "당신은 단검을 벗었습니다.")
	assertInventorySlot(t, world, "object:held-dagger")
	assertObjectLacksLegacyEquipFlag(t, world, "object:held-dagger", "OWEARS")
	assertObjectHasLegacyEquipFlag(t, world, "object:held-dagger", "OWHELD")
}

func TestRemoveAllClearsWornAndHeldFlagsLikeLegacy(t *testing.T) {
	loaded := equipmentWorld(t)
	addEquipTestObjectWithProperties(t, loaded, "object:held-dagger", "단검", map[string]string{
		"type":     strconv.Itoa(legacyObjectThrust),
		"wearFlag": strconv.Itoa(legacyWearHeld),
	}, "inventory")
	world := state.NewWorld(loaded)
	dispatcher := fullEquipDispatcher(t, world)

	assertEquipDispatchExact(t, dispatcher, "단검 쥐어", "당신은 단검을 쥐었습니다.")
	assertEquipDispatchExact(t, dispatcher, "모두 벗어", "당신은 단검을 벗었습니다.")
	assertInventorySlot(t, world, "object:held-dagger")
	assertObjectLacksLegacyEquipFlag(t, world, "object:held-dagger", "OWEARS")
	assertObjectLacksLegacyEquipFlag(t, world, "object:held-dagger", "OWHELD")
}

func TestRemoveAllUsesLegacyReadySlotOrder(t *testing.T) {
	loaded := equipmentWorld(t)
	objects := []struct {
		id      model.ObjectInstanceID
		display string
		flag    int
		slot    string
	}{
		{id: "object:remove-shield", display: "방패", flag: legacyWearShield, slot: "shield"},
		{id: "object:remove-face", display: "가면", flag: legacyWearFace, slot: "face"},
		{id: "object:remove-wield", display: "목검", flag: legacyWearWield, slot: "wield"},
		{id: "object:remove-held", display: "부적", flag: legacyWearHeld, slot: "held"},
		{id: "object:remove-ring", display: "반지", flag: legacyWearFinger, slot: "finger1"},
		{id: "object:remove-feet", display: "신발", flag: legacyWearFeet, slot: "feet"},
		{id: "object:remove-head", display: "투구", flag: legacyWearHead, slot: "head"},
		{id: "object:remove-hands", display: "장갑", flag: legacyWearHands, slot: "hands"},
		{id: "object:remove-neck", display: "목걸이", flag: legacyWearNeck, slot: "neck1"},
		{id: "object:remove-legs", display: "바지", flag: legacyWearLegs, slot: "legs"},
		{id: "object:remove-arms", display: "팔갑", flag: legacyWearArms, slot: "arms"},
		{id: "object:remove-body", display: "갑옷", flag: legacyWearBody, slot: "body"},
	}
	for _, object := range objects {
		addEquipTestObject(t, loaded, object.id, object.display, object.flag, object.slot)
	}
	world := state.NewWorld(loaded)
	handler := NewRemoveObjectHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"모두"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	want := "당신은 갑옷, 팔갑, 바지, 목걸이, 장갑, 투구, 신발, 반지, 부적, 방패, 가면, 목검을 벗었습니다."
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %d/%q", status, ctx.OutputString(), StatusDefault, want)
	}
	for _, object := range objects {
		assertInventorySlot(t, world, object.id)
	}
}

func TestRemoveUsesLegacyReadySlotOrderForDuplicateNames(t *testing.T) {
	loaded := equipmentWorld(t)
	addEquipTestObject(t, loaded, "object:second-ring", "동반지", legacyWearFinger, "finger2")
	addEquipTestObject(t, loaded, "object:first-ring", "동반지", legacyWearFinger, "finger1")
	world := state.NewWorld(loaded)
	handler := NewRemoveObjectHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"동반지"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 동반지를 벗었습니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	assertInventorySlot(t, world, "object:first-ring")
	assertEquippedSlot(t, world, "finger2", "object:second-ring")
}

func TestEquipmentHandlerUsesLegacyDisplaySlotOrder(t *testing.T) {
	loaded := equipmentWorld(t)
	objects := []struct {
		id      model.ObjectInstanceID
		display string
		flag    int
		slot    string
	}{
		{id: "object:display-wield", display: "목검", flag: legacyWearWield, slot: "wield"},
		{id: "object:display-shield", display: "방패", flag: legacyWearShield, slot: "shield"},
		{id: "object:display-held", display: "부적", flag: legacyWearHeld, slot: "held"},
		{id: "object:display-feet", display: "신발", flag: legacyWearFeet, slot: "feet"},
		{id: "object:display-legs", display: "바지", flag: legacyWearLegs, slot: "legs"},
		{id: "object:display-ring", display: "반지", flag: legacyWearFinger, slot: "finger1"},
		{id: "object:display-hands", display: "장갑", flag: legacyWearHands, slot: "hands"},
		{id: "object:display-arms", display: "팔갑", flag: legacyWearArms, slot: "arms"},
		{id: "object:display-body", display: "갑옷", flag: legacyWearBody, slot: "body"},
		{id: "object:display-neck", display: "목걸이", flag: legacyWearNeck, slot: "neck1"},
		{id: "object:display-face", display: "가면", flag: legacyWearFace, slot: "face"},
		{id: "object:display-head", display: "투구", flag: legacyWearHead, slot: "head"},
	}
	for _, object := range objects {
		addEquipTestObject(t, loaded, object.id, object.display, object.flag, object.slot)
	}
	world := state.NewWorld(loaded)
	creature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("missing creature")
	}

	want := "  <<<  착용 장비  >>>  \n" +
		"[ 머리 ]  투구\n" +
		"[ 얼굴 ]  가면\n" +
		"[  목  ]  목걸이\n" +
		"[  몸  ]  갑옷\n" +
		"[  팔  ]  팔갑\n" +
		"[  손  ]  장갑\n" +
		"[손가락]  반지\n" +
		"[ 다리 ]  바지\n" +
		"[  발  ]  신발\n" +
		"[쥔물건]  부적\n" +
		"[ 방패 ]  방패\n" +
		"[ 무기 ]  목검\n"
	if got := RenderEquipment(world, creature); got != want {
		t.Fatalf("RenderEquipment() = %q, want %q", got, want)
	}
}

func TestEquipmentHandlerUsesLegacyBlindMessage(t *testing.T) {
	loaded := equipmentWorld(t)
	addEquipTestObject(t, loaded, "object:blind-armor", "갑옷", legacyWearBody, "body")
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "PBLIND")
	loaded.Creatures[creature.ID] = creature
	world := state.NewWorld(loaded)
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "장비", Number: 11, Handler: "equipment"},
		}),
		Handlers: map[string]Handler{
			"equipment": NewEquipmentHandler(world),
		},
	}

	assertEquipDispatchExact(t, dispatcher, "장비", legacyEquipmentBlindMessage)
}

func TestEquipmentHandlerUsesLegacyANSIColors(t *testing.T) {
	loaded := equipmentWorld(t)
	addEquipTestObject(t, loaded, "object:ansi-armor", "갑옷", legacyWearBody, "body")
	world := state.NewWorld(loaded)
	handler := NewEquipmentHandler(world)

	ctx := &Context{
		ActorID: "player:alice",
		Values:  map[string]any{ContextANSIKey: true},
	}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	out := ctx.OutputString()
	for _, want := range []string{
		"\x1b[0;34m  <<<  착용 장비  >>>  \n\x1b[0;0m",
		"\x1b[0;33m[  몸  ]\x1b[0;0m  갑옷\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ANSI output missing %q:\n%q", want, out)
		}
	}
}

func equipOnlyDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()

	return Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "입어", Number: 9, Handler: "wear"},
		}),
		Handlers: map[string]Handler{
			"wear": NewWearHandler(world),
		},
	}
}

func fullEquipDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()

	return Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "입어", Number: 9, Handler: "wear"},
			{Name: "벗어", Number: 10, Handler: "remove_obj"},
			{Name: "쥐어", Number: 12, Handler: "hold"},
			{Name: "무장", Number: 13, Handler: "ready"},
		}),
		Handlers: map[string]Handler{
			"wear":       NewWearHandler(world),
			"remove_obj": NewRemoveObjectHandler(world),
			"hold":       NewHoldHandler(world),
			"ready":      NewReadyHandler(world),
		},
	}
}

func equipmentWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := emptyInventoryWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory = model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:sword", "object:armor", "object:charm"}}
	loaded.Creatures[creature.ID] = creature

	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:sword",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "목검",
		Properties:  map[string]string{"type": "1", "wearFlag": "20"},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:armor",
		Kind:        model.ObjectKindArmor,
		DisplayName: "갑옷",
		Properties:  map[string]string{"type": "5", "wearFlag": "1"},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:charm",
		DisplayName: "부적",
		Properties:  map[string]string{"wearFlag": "17"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:sword",
		PrototypeID: "prototype:sword",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:armor",
		PrototypeID: "prototype:armor",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:charm",
		PrototypeID: "prototype:charm",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	})
	return loaded
}

func addEquipTestObject(t *testing.T, loaded *worldload.World, objectID model.ObjectInstanceID, display string, wearFlag int, slot string) {
	t.Helper()

	addEquipTestObjectWithProperties(t, loaded, objectID, display, map[string]string{
		"type":     "5",
		"wearFlag": strconv.Itoa(wearFlag),
	}, slot)
}

func addEquipTestObjectWithProperties(t *testing.T, loaded *worldload.World, objectID model.ObjectInstanceID, display string, properties map[string]string, slot string) {
	t.Helper()

	protoID := model.PrototypeID("prototype:" + strings.TrimPrefix(string(objectID), "object:"))
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          protoID,
		Kind:        model.ObjectKindArmor,
		DisplayName: display,
		Properties:  cloneStringMap(properties),
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          objectID,
		PrototypeID: protoID,
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: slot},
	})

	creature := loaded.Creatures["creature:alice"]
	if slot == "inventory" {
		creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, objectID)
	} else {
		if creature.Equipment == nil {
			creature.Equipment = make(map[string]model.ObjectInstanceID)
		}
		creature.Equipment[slot] = objectID
	}
	loaded.Creatures[creature.ID] = creature
}

func setEquipCreatureStatsAndTags(t *testing.T, loaded *worldload.World, stats map[string]int, tags []string) {
	t.Helper()

	creature := loaded.Creatures["creature:alice"]
	if len(stats) > 0 && creature.Stats == nil {
		creature.Stats = make(map[string]int, len(stats))
	}
	for key, value := range stats {
		creature.Stats[key] = value
	}
	if len(tags) > 0 {
		creature.Metadata.Tags = append(creature.Metadata.Tags, tags...)
	}
	loaded.Creatures[creature.ID] = creature
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func removeEquipObjectID(ids []model.ObjectInstanceID, id model.ObjectInstanceID) []model.ObjectInstanceID {
	out := ids[:0]
	for _, existing := range ids {
		if existing != id {
			out = append(out, existing)
		}
	}
	return out
}

func assertEquipDispatchContains(t *testing.T, dispatcher Dispatcher, line string, wants ...string) {
	t.Helper()

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, line)
	if err != nil {
		t.Fatalf("DispatchLine(%q) error = %v", line, err)
	}
	if status != StatusDefault {
		t.Fatalf("DispatchLine(%q) status = %d, want default", line, status)
	}
	out := ctx.OutputString()
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Fatalf("DispatchLine(%q) output missing %q:\n%s", line, want, out)
		}
	}
}

func assertEquipDispatchExact(t *testing.T, dispatcher Dispatcher, line string, want string) {
	t.Helper()

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, line)
	if err != nil {
		t.Fatalf("DispatchLine(%q) error = %v", line, err)
	}
	if status != StatusDefault {
		t.Fatalf("DispatchLine(%q) status = %d, want default", line, status)
	}
	if got := ctx.OutputString(); got != want {
		t.Fatalf("DispatchLine(%q) output = %q, want %q", line, got, want)
	}
}

func assertEquippedSlot(t *testing.T, world *state.World, slot string, want model.ObjectInstanceID) {
	t.Helper()

	creature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("missing creature")
	}
	if got := creature.Equipment[slot]; got != want {
		t.Fatalf("equipment[%q] = %q, want %q", slot, got, want)
	}
	object, ok := world.Object(want)
	if !ok {
		t.Fatalf("missing object %q", want)
	}
	if object.Location.CreatureID != "creature:alice" || object.Location.Slot != slot {
		t.Fatalf("object location = %+v, want slot %q", object.Location, slot)
	}
}

func assertInventorySlot(t *testing.T, world *state.World, objectID model.ObjectInstanceID) {
	t.Helper()

	object, ok := world.Object(objectID)
	if !ok {
		t.Fatalf("missing object %q", objectID)
	}
	if object.Location.CreatureID != "creature:alice" || object.Location.Slot != "inventory" {
		t.Fatalf("object location = %+v, want inventory", object.Location)
	}
	creature, _ := world.Creature("creature:alice")
	for slot, equippedID := range creature.Equipment {
		if equippedID == objectID {
			t.Fatalf("object %q still equipped in slot %q", objectID, slot)
		}
	}
}

func assertObjectHasLegacyEquipFlag(t *testing.T, world *state.World, objectID model.ObjectInstanceID, flag string) {
	t.Helper()

	object, ok := world.Object(objectID)
	if !ok {
		t.Fatalf("missing object %q", objectID)
	}
	if !objectHasAnyTag(world, object, flag) {
		t.Fatalf("object %q tags = %+v, want %s", objectID, object.Metadata.Tags, flag)
	}
}

func assertObjectLacksLegacyEquipFlag(t *testing.T, world *state.World, objectID model.ObjectInstanceID, flag string) {
	t.Helper()

	object, ok := world.Object(objectID)
	if !ok {
		t.Fatalf("missing object %q", objectID)
	}
	if objectHasAnyTag(world, object, flag) {
		t.Fatalf("object %q tags = %+v, want no %s", objectID, object.Metadata.Tags, flag)
	}
}

func assertObjectRoomSlot(t *testing.T, world *state.World, objectID model.ObjectInstanceID, roomID model.RoomID) {
	t.Helper()

	object, ok := world.Object(objectID)
	if !ok {
		t.Fatalf("missing object %q", objectID)
	}
	if object.Location.RoomID != roomID || object.Location.CreatureID != "" || object.Location.Slot != "" {
		t.Fatalf("object location = %+v, want room %q", object.Location, roomID)
	}
}
