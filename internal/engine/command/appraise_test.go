package command

import (
	"strings"
	"testing"

	"muhan/internal/commandspec"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestAppraiseHandlerRendersLegacyObjectDetails(t *testing.T) {
	world := state.NewWorld(appraiseWorld(t, 8))
	defer world.Close()
	dispatcher := appraiseDispatcher(t, world)

	out := dispatchAppraiseLine(t, dispatcher, "목검 감정")

	for _, want := range []string{
		"이름: 목검\n",
		"사용회수 7\n",
		"종류: 검 무기.\n",
		"타격치: 4면2굴림 더하기 1 (+2)\n",
		"특성 : 선한 사람용, 빙의 되있음, 남성 금지.\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("appraise output missing %q:\n%s", want, out)
		}
	}
}

func TestAppraiseHandlerSupportsArmorNoTraitsAndCommandFirst(t *testing.T) {
	world := state.NewWorld(appraiseWorld(t, 9))
	defer world.Close()
	dispatcher := appraiseDispatcher(t, world)

	out := dispatchAppraiseLine(t, dispatcher, "감정 갑옷")

	for _, want := range []string{
		"이름: 갑옷\n",
		"종류: 방어구\n방어력: 12\n",
		"특성 : 특성 없음.\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("appraise armor output missing %q:\n%s", want, out)
		}
	}
}

func TestAppraiseHandlerReportsPermissionAndMissingTarget(t *testing.T) {
	world := state.NewWorld(appraiseWorld(t, 4))
	defer world.Close()
	dispatcher := appraiseDispatcher(t, world)

	if out := dispatchAppraiseLine(t, dispatcher, "목검 감정"); out != "도둑만 물건을 감정할수 있습니다." {
		t.Fatalf("non-thief appraise output = %q", out)
	}

	world = state.NewWorld(appraiseWorld(t, 8))
	defer world.Close()
	dispatcher = appraiseDispatcher(t, world)
	if out := dispatchAppraiseLine(t, dispatcher, "감정"); out != "무엇을 감정하실려구요?" {
		t.Fatalf("missing appraise target output = %q", out)
	}
	if out := dispatchAppraiseLine(t, dispatcher, "없는것 감정"); out != "당신은 그런것을 갖고 있지 않습니다." {
		t.Fatalf("missing appraise object output = %q", out)
	}
}

func TestAppraiseHandlerShowsMagicDetailsOnlyWithDetectMagic(t *testing.T) {
	world := state.NewWorld(appraiseWorld(t, 8))
	defer world.Close()
	dispatcher := appraiseDispatcher(t, world)

	out := dispatchAppraiseLine(t, dispatcher, "부적 감정")
	for _, hidden := range []string{"마법적 기운", "마법 힘", "남은 충전"} {
		if strings.Contains(out, hidden) {
			t.Fatalf("normal appraise output includes magic detail %q:\n%s", hidden, out)
		}
	}

	creature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("missing alice creature")
	}
	if _, err := world.UpdateCreatureTags(creature.ID, []string{"detectMagic"}, nil); err != nil {
		t.Fatal(err)
	}

	out = dispatchAppraiseLine(t, dispatcher, "부적 감정")
	for _, want := range []string{"마법적 기운: 있음\n", "마법 힘: 11\n", "남은 충전: 3\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("detect magic appraise output missing %q:\n%s", want, out)
		}
	}
}

func TestAppraiseAndCompareFindObjVisibilityUsesPDINVI(t *testing.T) {
	loaded := appraiseWorld(t, 8)
	sword := loaded.Objects["object:sword"]
	sword.Metadata.Tags = []string{"OINVIS"}
	loaded.Objects[sword.ID] = sword
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := appraiseDispatcher(t, world)

	if out := dispatchAppraiseLine(t, dispatcher, "목검 감정"); out != "당신은 그런것을 갖고 있지 않습니다." {
		t.Fatalf("invisible appraise output = %q", out)
	}
	if out := dispatchAppraiseLine(t, dispatcher, "목검 비교"); out != "당신은 그런 것을 갖고 있지 않습니다." {
		t.Fatalf("invisible compare output = %q", out)
	}

	loaded = appraiseWorld(t, 8)
	sword = loaded.Objects["object:sword"]
	sword.Metadata.Tags = []string{"OINVIS"}
	loaded.Objects[sword.ID] = sword
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"PDINVI"}
	loaded.Creatures[creature.ID] = creature
	world = state.NewWorld(loaded)
	defer world.Close()
	dispatcher = appraiseDispatcher(t, world)

	if out := dispatchAppraiseLine(t, dispatcher, "목검 감정"); !strings.Contains(out, "이름: 목검\n") {
		t.Fatalf("detect invisible appraise output = %q, want object details", out)
	}
	if out := dispatchAppraiseLine(t, dispatcher, "목검 비교"); out != "목검은 누구나 무장할 수 있습니다." {
		t.Fatalf("detect invisible compare output = %q", out)
	}
}

func TestObjectCompareHandlerRendersArmorAndWeaponLevel(t *testing.T) {
	world := state.NewWorld(appraiseWorld(t, 4))
	defer world.Close()
	dispatcher := appraiseDispatcher(t, world)

	if out := dispatchAppraiseLine(t, dispatcher, "갑옷 비교"); out != "갑옷은 누구나 입을 수 있습니다." {
		t.Fatalf("low armor compare output = %q", out)
	}
	if out := dispatchAppraiseLine(t, dispatcher, "판금갑옷 비교"); out != "판금갑옷은 30 레벨부터 입을 수 있습니다." {
		t.Fatalf("high armor compare output = %q", out)
	}
	if out := dispatchAppraiseLine(t, dispatcher, "대검 비교"); out != "대검은 54 레벨부터 무장할 수 있습니다." {
		t.Fatalf("weapon compare output = %q", out)
	}
}

func TestObjectCompareHandlerRendersInvincibleBreakdownAndFailures(t *testing.T) {
	world := state.NewWorld(appraiseWorld(t, 9))
	defer world.Close()
	dispatcher := appraiseDispatcher(t, world)

	want := "대검은 검사는 54레벨, 자객 도둑은 66레벨, 무사 포졸은 69레벨,\n" +
		"권법가 불제자 도술사는 75레벨부터 사용할 수 있습니다."
	if out := dispatchAppraiseLine(t, dispatcher, "비교 대검"); out != want {
		t.Fatalf("invincible compare output = %q, want %q", out, want)
	}
	if out := dispatchAppraiseLine(t, dispatcher, "부적 비교"); out != "무기나 방어구만 가능합니다." {
		t.Fatalf("misc compare output = %q", out)
	}
	if out := dispatchAppraiseLine(t, dispatcher, "비교"); out != "무엇을 비교하시려고요?" {
		t.Fatalf("missing compare target output = %q", out)
	}
	if out := dispatchAppraiseLine(t, dispatcher, "없는것 비교"); out != "당신은 그런 것을 갖고 있지 않습니다." {
		t.Fatalf("missing compare object output = %q", out)
	}
}

func TestAppraiseAndCompareUseOnlyFirstArgumentLikeLegacy(t *testing.T) {
	world := state.NewWorld(appraiseWorld(t, 8))
	defer world.Close()
	dispatcher := appraiseDispatcher(t, world)

	if out := dispatchAppraiseLine(t, dispatcher, "목검 무시 감정"); !strings.Contains(out, "이름: 목검\n") {
		t.Fatalf("appraise output = %q, want first-argument object details", out)
	}
	if out := dispatchAppraiseLine(t, dispatcher, "목검 무시 비교"); out != "목검은 누구나 무장할 수 있습니다." {
		t.Fatalf("compare output = %q, want first-argument compare result", out)
	}
}

func appraiseDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "감정", Number: 96, Handler: "info_obj"},
		{Name: "비교", Number: 96, Handler: "obj_compare"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"info_obj":    NewAppraiseHandler(world),
			"obj_compare": NewObjectCompareHandler(world),
		},
	}
}

func dispatchAppraiseLine(t *testing.T, dispatcher Dispatcher, line string) string {
	t.Helper()
	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, line)
	if err != nil {
		t.Fatalf("DispatchLine(%q) error = %v", line, err)
	}
	if status != StatusDefault {
		t.Fatalf("DispatchLine(%q) status = %d, want default", line, status)
	}
	return ctx.OutputString()
}

func appraiseWorld(t *testing.T, class int) *worldload.World {
	t.Helper()
	loaded := emptyInventoryWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Stats = map[string]int{"class": class}
	creature.Inventory = model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
		"object:sword",
		"object:big-sword",
		"object:armor",
		"object:plate",
		"object:charm",
	}}
	loaded.Creatures[creature.ID] = creature

	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:sword",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "목검",
		Properties: map[string]string{
			"type": "1", "shotsCurrent": "7", "sDice": "4", "nDice": "2", "pDice": "1", "adjustment": "2",
		},
		Metadata: model.Metadata{Tags: []string{"goodOnly", "enchanted", "femaleOnly"}},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:big-sword",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "대검",
		Properties:  map[string]string{"type": "1", "sDice": "6", "nDice": "4", "pDice": "1"},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:armor",
		Kind:        model.ObjectKindArmor,
		DisplayName: "갑옷",
		Properties:  map[string]string{"type": "5", "armor": "12", "wearFlag": "1"},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:plate",
		Kind:        model.ObjectKindArmor,
		DisplayName: "판금갑옷",
		Properties:  map[string]string{"type": "5", "armor": "15", "wearFlag": "1"},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:charm",
		DisplayName: "부적",
		Properties:  map[string]string{"type": "13", "magicPower": "11", "charges": "3"},
	})

	for _, object := range []model.ObjectInstance{
		{ID: "object:sword", PrototypeID: "prototype:sword"},
		{ID: "object:big-sword", PrototypeID: "prototype:big-sword"},
		{ID: "object:armor", PrototypeID: "prototype:armor"},
		{ID: "object:plate", PrototypeID: "prototype:plate"},
		{ID: "object:charm", PrototypeID: "prototype:charm"},
	} {
		object.Location = model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"}
		mustAddLookObject(t, loaded, object)
	}
	return loaded
}
