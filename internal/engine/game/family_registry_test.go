package game

import (
	"strings"
	"testing"

	"muhan/internal/commandspec"
	enginecmd "muhan/internal/engine/command"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestListFamilyHandlerRendersRegistryAndBankFallback(t *testing.T) {
	world := state.NewWorld(listFamilyWorld(t))
	dispatcher := enginecmd.Dispatcher{
		Registry: listFamilyRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"list_family": NewListFamilyHandler(world),
		},
	}
	ctx := &enginecmd.Context{ActorID: "player:alice"}

	status, err := dispatcher.DispatchLine(ctx, "모든패거리")
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDefault {
		t.Fatalf("status = %v, want default", status)
	}

	out := ctx.OutputString()
	for _, want := range []string{
		"다음과 같은 패거리가 있습니다.",
		"패거리이름",
		"문주이름",
		"가입축하금",
		"패거리금고",
		"은형문",
		"셀미",
		"100만냥",
		"무영문",
		"무영풍",
		"250만냥",
		"1234만냥",
		"총 2 개의 패거리가 활동중에 있습니다.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("list_family output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "관리파") {
		t.Fatalf("list_family output included slot 0 admin family:\n%s", out)
	}
	if strings.Index(out, "은형문") > strings.Index(out, "무영문") {
		t.Fatalf("list_family output order =\n%s", out)
	}
	if strings.HasSuffix(out, "\n") {
		t.Fatalf("list_family output has trailing newline: %q", out)
	}
}

func TestListFamilyHandlerRegistersEnglishAlias(t *testing.T) {
	world := state.NewWorld(listFamilyWorld(t))
	dispatcher := enginecmd.Dispatcher{
		Registry: listFamilyRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"list_family": NewListFamilyHandler(world),
		},
	}
	ctx := &enginecmd.Context{ActorID: "player:alice"}

	if _, err := dispatcher.DispatchLine(ctx, "list_family"); err != nil {
		t.Fatal(err)
	}
	if out := ctx.OutputString(); !strings.Contains(out, "총 2 개의 패거리가 활동중에 있습니다.") {
		t.Fatalf("list_family alias output =\n%s", out)
	}
}

func listFamilyRegistry(t *testing.T) commandspec.Registry {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "모든패거리", Number: 149, Handler: "list_family"},
		{Name: "list_family", Number: 149, Handler: "list_family"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func listFamilyWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	for _, family := range []model.Family{
		{ID: 0, Slot: 0, DisplayName: "관리파", BossName: "지존마상", JoinSubsidy: 100},
		{ID: 1, Slot: 1, DisplayName: "은형문", BossName: "셀미", JoinSubsidy: 100},
		{ID: 2, Slot: 2, DisplayName: "무영문", BossName: "무영풍", JoinSubsidy: 250},
	} {
		if err := loaded.AddFamily(family); err != nil {
			t.Fatalf("AddFamily(%q) error: %v", family.DisplayName, err)
		}
	}

	bankID := model.BankID("bank:family:무영문_0")
	objectID := model.ObjectInstanceID("object:family-bank")
	if err := loaded.AddObjectInstance(model.ObjectInstance{
		ID:          objectID,
		PrototypeID: "prototype:family-bank",
		Location:    model.ObjectLocation{BankID: bankID},
		Properties:  map[string]string{"value": "1234"},
	}); err != nil {
		t.Fatalf("AddObjectInstance() error: %v", err)
	}
	if err := loaded.AddBank(model.BankAccount{
		ID:        bankID,
		Kind:      "family",
		OwnerName: "무영문_0",
		Objects:   model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{objectID}},
	}); err != nil {
		t.Fatalf("AddBank() error: %v", err)
	}
	return loaded
}
