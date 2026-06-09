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

func TestFamilyMemberHandlerRendersLoadedFamilyMembers(t *testing.T) {
	dispatcher := familyMemberDispatcher(t, state.NewWorld(familyMemberWorld(t)))
	ctx := &enginecmd.Context{ActorID: "player:alice"}

	status, err := dispatcher.DispatchLine(ctx, "패거리원")
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDefault {
		t.Fatalf("status = %v, want default", status)
	}

	out := ctx.OutputString()
	for _, want := range []string{
		"당신은 [무영문] 패거리에 가입되어 있습니다.",
		"초인",
		"무영풍",
		"검사",
		"은검",
		"도둑",
		"초록",
		"자객",
		"백호",
		"총 4명의 사람들이 가입되어 있습니다.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("family_member output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "은형문") || strings.Contains(out, "다른문주") {
		t.Fatalf("family_member output included another family:\n%s", out)
	}
	if strings.Contains(out, "[초인  ]") || strings.Contains(out, "[도둑  ]") {
		t.Fatalf("family_member output used Go rune-width class padding:\n%s", out)
	}
	for _, want := range []string{"[초인]  무영풍", "[도둑]  초록"} {
		if !strings.Contains(out, want) {
			t.Fatalf("family_member output missing legacy class label %q:\n%s", want, out)
		}
	}
}

func TestFamilyMemberHandlerRegistersEnglishAlias(t *testing.T) {
	dispatcher := familyMemberDispatcher(t, state.NewWorld(familyMemberWorld(t)))
	ctx := &enginecmd.Context{ActorID: "player:alice"}

	if _, err := dispatcher.DispatchLine(ctx, "family_member"); err != nil {
		t.Fatal(err)
	}
	if out := ctx.OutputString(); !strings.Contains(out, "총 4명의 사람들이 가입되어 있습니다.") {
		t.Fatalf("family_member alias output =\n%s", out)
	}
}

func TestFamilyMemberHandlerRequiresFamilyMembership(t *testing.T) {
	dispatcher := familyMemberDispatcher(t, state.NewWorld(familyMemberWorld(t)))
	ctx := &enginecmd.Context{ActorID: "player:eve"}

	if _, err := dispatcher.DispatchLine(ctx, "패거리원"); err != nil {
		t.Fatal(err)
	}
	if out := ctx.OutputString(); out != "당신은 패거리에 가입되어 있지 않습니다." {
		t.Fatalf("non-family output = %q", out)
	}
}

func familyMemberDispatcher(t *testing.T, world *state.World) enginecmd.Dispatcher {
	t.Helper()
	return enginecmd.Dispatcher{
		Registry: familyMemberRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"family_member": NewFamilyMemberHandler(world),
		},
	}
}

func familyMemberRegistry(t *testing.T) commandspec.Registry {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "패거리원", Number: 148, Handler: "family_member"},
		{Name: "family_member", Number: 148, Handler: "family_member"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func familyMemberWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	for _, family := range []model.Family{
		{
			ID:          2,
			Slot:        2,
			DisplayName: "무영문",
			BossName:    "무영풍",
			Members: []model.FamilyMember{
				{Class: 10, DisplayName: "무영풍"},
				{Class: 4, DisplayName: "은검"},
				{Class: 8, DisplayName: "초록"},
				{Class: 1, DisplayName: "백호"},
			},
		},
		{
			ID:          5,
			Slot:        5,
			DisplayName: "은형문",
			BossName:    "다른문주",
			Members: []model.FamilyMember{
				{Class: 10, DisplayName: "다른문주"},
			},
		},
	} {
		if err := loaded.AddFamily(family); err != nil {
			t.Fatalf("AddFamily(%q) error: %v", family.DisplayName, err)
		}
	}
	for _, player := range []model.Player{
		{ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice"},
		{ID: "player:eve", DisplayName: "Eve", CreatureID: "creature:eve"},
	} {
		if err := loaded.AddPlayer(player); err != nil {
			t.Fatalf("AddPlayer(%q) error: %v", player.DisplayName, err)
		}
	}
	for _, creature := range []model.Creature{
		{
			ID:          "creature:alice",
			Kind:        model.CreatureKindPlayer,
			DisplayName: "Alice",
			PlayerID:    "player:alice",
			Stats:       map[string]int{"familyFlag": 1, "familyID": 2},
		},
		{
			ID:          "creature:eve",
			Kind:        model.CreatureKindPlayer,
			DisplayName: "Eve",
			PlayerID:    "player:eve",
			Stats:       map[string]int{},
		},
	} {
		if err := loaded.AddCreature(creature); err != nil {
			t.Fatalf("AddCreature(%q) error: %v", creature.DisplayName, err)
		}
	}
	return loaded
}
