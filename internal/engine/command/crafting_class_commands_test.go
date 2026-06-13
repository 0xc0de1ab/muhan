package command

import (
	"errors"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestForgeHandlerValidatesForgeRoom(t *testing.T) {
	world := state.NewWorld(craftingClassWorld(t, model.Room{
		ID:          "room:00610",
		DisplayName: "광장",
	}, model.ClassFighter, 100000))

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewForgeHandler(world, nil)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "여기는 대장간이 아닙니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
}

func TestForgeHandlerCallsStarterInForgeRoom(t *testing.T) {
	world := state.NewWorld(craftingClassWorld(t, model.Room{
		ID:          "room:00610",
		DisplayName: "대장간",
		Metadata:    model.Metadata{Tags: []string{"forge"}},
	}, model.ClassFighter, 100000))
	starter := &recordingWeaponForgeStarter{status: StatusDoPrompt}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewForgeHandler(world, starter)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDoPrompt || ctx.OutputString() != "forge started\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	if !starter.called || starter.request.Mode != WeaponForgeModeForge || starter.request.RoomID != "room:00610" {
		t.Fatalf("starter request = called:%v %+v", starter.called, starter.request)
	}
}

func TestForgeHandlerDefaultConversationCreatesWeapon(t *testing.T) {
	world := state.NewWorld(craftingClassWorld(t, model.Room{
		ID:          "room:00610",
		DisplayName: "대장간",
		Metadata:    model.Metadata{Tags: []string{"forge"}},
	}, model.ClassFighter, 100000))

	var pending PendingLineHandler
	ctx := craftingClassPromptContext(&pending)
	status, err := NewForgeHandler(world, nil)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDoPrompt || pending == nil || !strings.Contains(ctx.OutputString(), "어떤 종류의 무기를 원하십니까?") {
		t.Fatalf("initial status/output/pending = %d/%q/%v", status, ctx.OutputString(), pending != nil)
	}

	runCraftingClassPending(t, ctx, &pending, "2", StatusDoPrompt)
	if !strings.Contains(ctx.OutputString(), "강 철 오만냥") {
		t.Fatalf("material prompt = %q", ctx.OutputString())
	}
	runCraftingClassPending(t, ctx, &pending, "3", StatusDoPrompt)
	if !strings.Contains(ctx.OutputString(), "담금질") {
		t.Fatalf("tempering prompt = %q", ctx.OutputString())
	}
	runCraftingClassPending(t, ctx, &pending, "4", StatusDoPrompt)
	if !strings.Contains(ctx.OutputString(), "무기의 이름") {
		t.Fatalf("name prompt = %q", ctx.OutputString())
	}
	runCraftingClassPending(t, ctx, &pending, "용검검", StatusDoPrompt)
	if !strings.Contains(ctx.OutputString(), "모든것에 만족하십니까?") {
		t.Fatalf("confirm prompt = %q", ctx.OutputString())
	}
	runCraftingClassPending(t, ctx, &pending, "예", StatusDefault)
	if pending != nil {
		t.Fatalf("pending not cleared")
	}
	if ctx.OutputString() != "\n주인이 당신에게 새로 제작된 무기를 건네줍니다." {
		t.Fatalf("final output = %q", ctx.OutputString())
	}

	creature, _ := world.Creature("creature:alice")
	if creature.Stats["gold"] != 8700000 {
		t.Fatalf("gold = %d, want 8700000", creature.Stats["gold"])
	}
	if len(creature.Inventory.ObjectIDs) != 1 {
		t.Fatalf("inventory = %+v, want one forged weapon", creature.Inventory.ObjectIDs)
	}
	object, ok := world.Object(creature.Inventory.ObjectIDs[0])
	if !ok {
		t.Fatalf("forged object %q not found", creature.Inventory.ObjectIDs[0])
	}
	if object.PrototypeID != legacyWeaponForgePrototypeID(2) || objectDisplayName(world, object) != "용검검" {
		t.Fatalf("forged object = proto:%q name:%q", object.PrototypeID, objectDisplayName(world, object))
	}
	if objectIntPropertyOrZero(world, object, "sDice") != 5 ||
		objectIntPropertyOrZero(world, object, "shotsMax") != 400 ||
		objectIntPropertyOrZero(world, object, "shotsCurrent") != 400 {
		t.Fatalf("forged properties = %+v", object.Properties)
	}
	if !objectHasAnyTag(world, object, "OCLSEL") || !objectHasAnyTag(world, object, "OBARBO") {
		t.Fatalf("diamond material class restriction tags missing: %+v", object.Metadata.Tags)
	}
}

func TestForgeHandlerRejectsDiamondMaterialForMagicClasses(t *testing.T) {
	world := state.NewWorld(craftingClassWorld(t, model.Room{
		ID:          "room:00610",
		DisplayName: "대장간",
		Metadata:    model.Metadata{Tags: []string{"forge"}},
	}, model.ClassMage, 100000))

	var pending PendingLineHandler
	ctx := craftingClassPromptContext(&pending)
	if _, err := NewForgeHandler(world, nil)(ctx, ResolvedCommand{}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	runCraftingClassPending(t, ctx, &pending, "1", StatusDoPrompt)
	runCraftingClassPending(t, ctx, &pending, "3", StatusDoPrompt)
	if ctx.OutputString() != "당신은 이런 무기를 사용할 능력이 없습니다.\n다른재료를 선택하십시요\n" {
		t.Fatalf("output = %q", ctx.OutputString())
	}
}

func TestForgeHandlerChoiceParsingMatchesCFirstByte(t *testing.T) {
	world := state.NewWorld(craftingClassWorld(t, model.Room{
		ID:          "room:00610",
		DisplayName: "대장간",
		Metadata:    model.Metadata{Tags: []string{"forge"}},
	}, model.ClassFighter, 100000))

	var pending PendingLineHandler
	ctx := craftingClassPromptContext(&pending)
	if _, err := NewForgeHandler(world, nil)(ctx, ResolvedCommand{}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}

	runCraftingClassPending(t, ctx, &pending, " 2", StatusDoPrompt)
	if ctx.OutputString() != "하나를 선택하시오: " {
		t.Fatalf("leading-space choice output = %q", ctx.OutputString())
	}
	runCraftingClassPending(t, ctx, &pending, "2", StatusDoPrompt)
	if !strings.Contains(ctx.OutputString(), "타격치에 영향을 주는 재료") {
		t.Fatalf("valid retry output = %q", ctx.OutputString())
	}
}

func TestForgeHandlerNameValidationMatchesLegacyBytesAndPreservesText(t *testing.T) {
	world := state.NewWorld(craftingClassWorld(t, model.Room{
		ID:          "room:00610",
		DisplayName: "대장간",
		Metadata:    model.Metadata{Tags: []string{"forge"}},
	}, model.ClassFighter, 100000))

	var pending PendingLineHandler
	ctx := craftingClassPromptContext(&pending)
	if _, err := NewForgeHandler(world, nil)(ctx, ResolvedCommand{}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	runCraftingClassPending(t, ctx, &pending, "1", StatusDoPrompt)
	runCraftingClassPending(t, ctx, &pending, "1", StatusDoPrompt)
	runCraftingClassPending(t, ctx, &pending, "1", StatusDoPrompt)

	runCraftingClassPending(t, ctx, &pending, strings.Repeat("한", 11), StatusDoPrompt)
	if !strings.Contains(ctx.OutputString(), "입력된 이름이 너무  깁니다.") {
		t.Fatalf("too-long name output = %q", ctx.OutputString())
	}
	runCraftingClassPending(t, ctx, &pending, "검", StatusDoPrompt)
	if !strings.Contains(ctx.OutputString(), "입력된 이름이 너무  짧습니다.") {
		t.Fatalf("too-short name output = %q", ctx.OutputString())
	}
	runCraftingClassPending(t, ctx, &pending, " 검a ", StatusDoPrompt)
	if !strings.Contains(ctx.OutputString(), "모든것에 만족하십니까?") {
		t.Fatalf("accepted name output = %q", ctx.OutputString())
	}
	runCraftingClassPending(t, ctx, &pending, "예", StatusDefault)

	creature, _ := world.Creature("creature:alice")
	object, ok := world.Object(creature.Inventory.ObjectIDs[0])
	if !ok {
		t.Fatalf("forged object not found")
	}
	if got := object.DisplayNameOverride; got != " 검a " {
		t.Fatalf("forged object raw name = %q, want preserved spaces", got)
	}
	if got := objectDisplayName(world, object); got != "검a" {
		t.Fatalf("forged object display name = %q, want trimmed display", got)
	}
}

func TestForgeHandlerConfirmMatchesCLiteralYes(t *testing.T) {
	world := state.NewWorld(craftingClassWorld(t, model.Room{
		ID:          "room:00610",
		DisplayName: "대장간",
		Metadata:    model.Metadata{Tags: []string{"forge"}},
	}, model.ClassFighter, 100000))

	var pending PendingLineHandler
	ctx := craftingClassPromptContext(&pending)
	if _, err := NewForgeHandler(world, nil)(ctx, ResolvedCommand{}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	runCraftingClassPending(t, ctx, &pending, "1", StatusDoPrompt)
	runCraftingClassPending(t, ctx, &pending, "1", StatusDoPrompt)
	runCraftingClassPending(t, ctx, &pending, "1", StatusDoPrompt)
	runCraftingClassPending(t, ctx, &pending, "검a", StatusDoPrompt)
	runCraftingClassPending(t, ctx, &pending, " 예", StatusDefault)
	if ctx.OutputString() != "무기 제련을 취소하였습니다." {
		t.Fatalf("leading-space yes output = %q", ctx.OutputString())
	}

	creature, _ := world.Creature("creature:alice")
	if len(creature.Inventory.ObjectIDs) != 0 {
		t.Fatalf("inventory after literal-yes cancel = %+v", creature.Inventory.ObjectIDs)
	}
}

func TestNewForgeHandlerRequiresLegacyRoom611(t *testing.T) {
	world := state.NewWorld(craftingClassWorld(t, model.Room{
		ID:          "room:00610",
		DisplayName: "대장간",
		Metadata:    model.Metadata{Tags: []string{"forge"}},
	}, model.ClassFighter, 100000))

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewNewForgeHandler(world, nil)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "여기서는 무기를 만들 수가 없습니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
}

func TestNewForgeHandlerCallsStarterInLegacyRoom611(t *testing.T) {
	world := state.NewWorld(craftingClassWorld(t, model.Room{
		ID:          "room:00611",
		DisplayName: "특별 대장간",
		Metadata:    model.Metadata{Tags: []string{"forge"}},
	}, model.ClassFighter, 100000))
	starter := &recordingWeaponForgeStarter{status: StatusDoPrompt}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewNewForgeHandler(world, starter)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDoPrompt || ctx.OutputString() != "newforge started\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	if !starter.called || starter.request.Mode != WeaponForgeModeNewForge || starter.request.RoomID != "room:00611" {
		t.Fatalf("starter request = called:%v %+v", starter.called, starter.request)
	}
}

func TestNewForgeHandlerDefaultConversationCreatesHighGradeWeapon(t *testing.T) {
	world := state.NewWorld(craftingClassWorld(t, model.Room{
		ID:          "room:00611",
		DisplayName: "특별 대장간",
		Metadata:    model.Metadata{Tags: []string{"forge"}},
	}, model.ClassFighter, 100000))

	var pending PendingLineHandler
	ctx := craftingClassPromptContext(&pending)
	status, err := NewNewForgeHandler(world, nil)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDoPrompt || pending == nil {
		t.Fatalf("initial status/pending = %d/%v", status, pending != nil)
	}

	runCraftingClassPending(t, ctx, &pending, "5", StatusDoPrompt)
	runCraftingClassPending(t, ctx, &pending, "3", StatusDoPrompt)
	runCraftingClassPending(t, ctx, &pending, "5", StatusDoPrompt)
	runCraftingClassPending(t, ctx, &pending, "환궁궁", StatusDoPrompt)
	runCraftingClassPending(t, ctx, &pending, "예", StatusDefault)

	creature, _ := world.Creature("creature:alice")
	if creature.Stats["gold"] != 2000000 {
		t.Fatalf("gold = %d, want 2000000", creature.Stats["gold"])
	}
	object, ok := world.Object(creature.Inventory.ObjectIDs[0])
	if !ok {
		t.Fatalf("forged object not found")
	}
	if object.PrototypeID != legacyWeaponForgePrototypeID(5) ||
		objectIntPropertyOrZero(world, object, "nDice") != 4 ||
		objectIntPropertyOrZero(world, object, "sDice") != 7 ||
		objectIntPropertyOrZero(world, object, "pDice") != 3 ||
		objectIntPropertyOrZero(world, object, "shotsMax") != 900 ||
		objectIntPropertyOrZero(world, object, "shotsCurrent") != 900 {
		t.Fatalf("newforge object = proto:%q props:%+v", object.PrototypeID, object.Properties)
	}
}

func TestChangeClassHandlerRejectsLegacyConditions(t *testing.T) {
	tests := []struct {
		name       string
		roomTags   []string
		class      int
		experience int
		playerTags []string
		want       string
	}{
		{
			name:       "blind",
			roomTags:   []string{"train", "trainingBit4"},
			class:      model.ClassFighter,
			experience: 100000,
			playerTags: []string{"blind"},
			want:       "당신은 눈이 멀어 직업전환을 할 수 없습니다!\n",
		},
		{
			name:       "not training room",
			class:      model.ClassFighter,
			experience: 100000,
			want:       "이 곳은 수련장이 아닙니다!\n",
		},
		{
			name:       "unsupported class",
			roomTags:   []string{"train", "trainingBit4"},
			class:      model.ClassInvincible,
			experience: 100000,
			want:       "당신은 직업전환을 할 수 없는 직업을 갖고 있습니다.\n",
		},
		{
			name:       "same class training room",
			roomTags:   []string{"train", "trainingBit4"},
			class:      model.ClassMage,
			experience: 100000,
			want:       "직업전환을 하려면 자신이 수련하는곳에서는 할 수 없습니다.\n",
		},
		{
			name:       "not enough experience",
			roomTags:   []string{"train", "trainingBit4"},
			class:      model.ClassFighter,
			experience: 99999,
			want:       "직업전환을 하려면 경험치 10만이 필요합니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := state.NewWorld(craftingClassWorldWithTags(t, model.Room{
				ID:          "room:00610",
				DisplayName: "수련장",
				Metadata:    model.Metadata{Tags: tt.roomTags},
			}, tt.class, tt.experience, tt.playerTags))

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewChangeClassHandler(world, nil)(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}

			creature, _ := world.Creature("creature:alice")
			if creature.Stats["class"] != tt.class || creature.Stats["experience"] != tt.experience {
				t.Fatalf("creature mutated on reject: class=%d exp=%d", creature.Stats["class"], creature.Stats["experience"])
			}
		})
	}
}

func TestChangeClassHandlerCallsStarterAfterValidation(t *testing.T) {
	world := state.NewWorld(craftingClassWorld(t, model.Room{
		ID:          "room:00610",
		DisplayName: "수련장",
		Metadata:    model.Metadata{Tags: []string{"train", "trainingBit4"}},
	}, model.ClassFighter, 100000))
	starter := &recordingClassChangeStarter{status: StatusDoPrompt}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewChangeClassHandler(world, starter)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDoPrompt || ctx.OutputString() != "class change started\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	if !starter.called || starter.request.CurrentClass != model.ClassFighter || starter.request.TargetClass != model.ClassMage {
		t.Fatalf("starter request = called:%v %+v", starter.called, starter.request)
	}

	creature, _ := world.Creature("creature:alice")
	if creature.Stats["class"] != model.ClassFighter || creature.Stats["experience"] != 100000 {
		t.Fatalf("default handler path mutated creature: class=%d exp=%d", creature.Stats["class"], creature.Stats["experience"])
	}
}

func TestChangeClassHandlerDefaultConversationMutatesOnYes(t *testing.T) {
	world := state.NewWorld(craftingClassWorld(t, model.Room{
		ID:          "room:00610",
		DisplayName: "수련장",
		Metadata:    model.Metadata{Tags: []string{"train", "trainingBit4"}},
	}, model.ClassFighter, 150000))
	if _, err := world.SetCreatureLevel("creature:alice", 40); err != nil {
		t.Fatalf("SetCreatureLevel() error = %v", err)
	}

	var pending PendingLineHandler
	ctx := craftingClassPromptContext(&pending)
	status, err := NewChangeClassHandler(world, nil)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDoPrompt || pending == nil {
		t.Fatalf("initial status/pending = %d/%v", status, pending != nil)
	}
	if ctx.OutputString() != "직업전환을 하려면 경험치 10만이 필요합니다.\n정말로 직업전환을 하시겠습니까?(예/아니오): " {
		t.Fatalf("prompt = %q", ctx.OutputString())
	}

	runCraftingClassPending(t, ctx, &pending, "예", StatusDefault)
	creature, _ := world.Creature("creature:alice")
	if creature.Stats["class"] != model.ClassMage || creature.Stats["experience"] != 50000 {
		t.Fatalf("class/exp = %d/%d", creature.Stats["class"], creature.Stats["experience"])
	}
	if wantLevel := legacyExperienceToLevel(50000); creature.Stats["level"] != wantLevel || creature.Level != wantLevel {
		t.Fatalf("level = stat:%d canonical:%d, want %d", creature.Stats["level"], creature.Level, wantLevel)
	}
	if ctx.OutputString() != "\n당신의 직업이 전환되었습니다." {
		t.Fatalf("final output = %q", ctx.OutputString())
	}
}

func TestChangeClassHandlerCallsLegacyEffectHooks(t *testing.T) {
	base := state.NewWorld(craftingClassWorld(t, model.Room{
		ID:          "room:00610",
		DisplayName: "수련장",
		Metadata:    model.Metadata{Tags: []string{"train", "trainingBit4"}},
	}, model.ClassFighter, 150000))
	if err := base.SetCreatureStat("creature:alice", "familyFlag", 1); err != nil {
		t.Fatalf("SetCreatureStat(familyFlag) error = %v", err)
	}
	if err := base.SetCreatureStat("creature:alice", "dailyExpndMax", 7); err != nil {
		t.Fatalf("SetCreatureStat(dailyExpndMax) error = %v", err)
	}
	world := &craftingClassHookWorld{World: base}

	var pending PendingLineHandler
	ctx := craftingClassPromptContext(&pending)
	if _, err := NewChangeClassHandler(world, nil)(ctx, ResolvedCommand{}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	runCraftingClassPending(t, ctx, &pending, "예", StatusDefault)

	if world.combatStatsCreatureID != "creature:alice" {
		t.Fatalf("combat stats hook creature = %q", world.combatStatsCreatureID)
	}
	if world.familyName != "Alice" || world.familyClass != model.ClassMage || world.familyDailyExpndMax != 7 {
		t.Fatalf("family hook = name:%q class:%d daily:%d", world.familyName, world.familyClass, world.familyDailyExpndMax)
	}
}

func TestChangeClassHandlerFamilyHookFailureDoesNotRollbackLegacyMutation(t *testing.T) {
	base := state.NewWorld(craftingClassWorld(t, model.Room{
		ID:          "room:00610",
		DisplayName: "수련장",
		Metadata:    model.Metadata{Tags: []string{"train", "trainingBit4"}},
	}, model.ClassFighter, 150000))
	if err := base.SetCreatureStat("creature:alice", "familyFlag", 1); err != nil {
		t.Fatalf("SetCreatureStat(familyFlag) error = %v", err)
	}
	if err := base.SetCreatureStat("creature:alice", "dailyExpndMax", 7); err != nil {
		t.Fatalf("SetCreatureStat(dailyExpndMax) error = %v", err)
	}
	wantErr := errors.New("family member update failed")
	world := &craftingClassHookWorld{World: base, familyErr: wantErr}

	var pending PendingLineHandler
	ctx := craftingClassPromptContext(&pending)
	if _, err := NewChangeClassHandler(world, nil)(ctx, ResolvedCommand{}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	ctx.Output = nil
	status, err := pending(ctx, "예")
	if !errors.Is(err, wantErr) {
		t.Fatalf("pending yes error = %v, want %v", err, wantErr)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if ctx.OutputString() != "" {
		t.Fatalf("output on hook failure = %q, want empty", ctx.OutputString())
	}

	creature, _ := world.Creature("creature:alice")
	if creature.Stats["class"] != model.ClassMage || creature.Stats["experience"] != 50000 {
		t.Fatalf("class/exp after hook failure = %d/%d, want %d/50000", creature.Stats["class"], creature.Stats["experience"], model.ClassMage)
	}
	if world.familyName != "Alice" || world.familyClass != model.ClassMage || world.familyDailyExpndMax != 7 {
		t.Fatalf("family hook = name:%q class:%d daily:%d", world.familyName, world.familyClass, world.familyDailyExpndMax)
	}
}

func TestClassChangeDailyExpndMaxFallsBackToFamilyID(t *testing.T) {
	tests := []struct {
		name     string
		creature model.Creature
		want     int
	}{
		{
			name:     "familyID stat",
			creature: model.Creature{Stats: map[string]int{"familyID": 9}},
			want:     9,
		},
		{
			name:     "familyID property",
			creature: model.Creature{Properties: map[string]string{"familyID": "11"}},
			want:     11,
		},
		{
			name: "dailyExpndMax keeps C daily family slot precedence",
			creature: model.Creature{
				Stats: map[string]int{"dailyExpndMax": 7, "familyID": 9},
			},
			want: 7,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classChangeDailyExpndMax(tt.creature); got != tt.want {
				t.Fatalf("classChangeDailyExpndMax() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestChangeClassHandlerDefaultConversationCancelsOnNo(t *testing.T) {
	world := state.NewWorld(craftingClassWorld(t, model.Room{
		ID:          "room:00610",
		DisplayName: "수련장",
		Metadata:    model.Metadata{Tags: []string{"train", "trainingBit4"}},
	}, model.ClassFighter, 150000))

	var pending PendingLineHandler
	ctx := craftingClassPromptContext(&pending)
	if _, err := NewChangeClassHandler(world, nil)(ctx, ResolvedCommand{}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	runCraftingClassPending(t, ctx, &pending, "아니오", StatusDefault)
	creature, _ := world.Creature("creature:alice")
	if creature.Stats["class"] != model.ClassFighter || creature.Stats["experience"] != 150000 {
		t.Fatalf("class/exp mutated on cancel = %d/%d", creature.Stats["class"], creature.Stats["experience"])
	}
	if ctx.OutputString() != "직업전환이 되지 않았습니다" {
		t.Fatalf("output = %q", ctx.OutputString())
	}
}

func TestCraftingClassDispatcherKeys(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "제련", Number: 85, Handler: "forge"},
		{Name: "무기만들기", Number: 85, Handler: "newforge"},
		{Name: "직업전환", Number: 86, Handler: "change_class"},
	})
	world := state.NewWorld(craftingClassWorld(t, model.Room{
		ID:          "room:00611",
		DisplayName: "수련장 겸 대장간",
		Metadata:    model.Metadata{Tags: []string{"forge", "train", "trainingBit4"}},
	}, model.ClassFighter, 100000))
	dispatcher := Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"forge":        NewForgeHandler(world, nil),
			"newforge":     NewNewForgeHandler(world, nil),
			"change_class": NewChangeClassHandler(world, nil),
		},
	}

	tests := []struct {
		line string
		want string
	}{
		{line: "제련", want: "\n\n어떤 종류의 무기를 원하십니까?\n1. 도 2. 검 3. 봉 4. 창 5. 궁 ?\n:"},
		{line: "무기만들기", want: "\n\n어떤 종류의 무기를 원하십니까?\n1. 도 2. 검 3. 봉 4. 창 5. 궁 ?\n:"},
		{line: "직업전환", want: "직업전환을 하려면 경험치 10만이 필요합니다.\n정말로 직업전환을 하시겠습니까?(예/아니오): "},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			var pending PendingLineHandler
			ctx := craftingClassPromptContext(&pending)
			status, err := dispatcher.DispatchLine(ctx, tt.line)
			if err != nil {
				t.Fatalf("DispatchLine(%q) error = %v", tt.line, err)
			}
			if status != StatusDoPrompt || pending == nil || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

type recordingWeaponForgeStarter struct {
	called  bool
	status  Status
	request WeaponForgeRequest
}

func (s *recordingWeaponForgeStarter) BeginWeaponForge(ctx *Context, request WeaponForgeRequest) (Status, error) {
	s.called = true
	s.request = request
	if request.Mode == WeaponForgeModeNewForge {
		ctx.WriteString("newforge started\n")
	} else {
		ctx.WriteString("forge started\n")
	}
	return s.status, nil
}

type recordingClassChangeStarter struct {
	called  bool
	status  Status
	request ClassChangeRequest
}

func (s *recordingClassChangeStarter) BeginClassChange(ctx *Context, request ClassChangeRequest) (Status, error) {
	s.called = true
	s.request = request
	ctx.WriteString("class change started\n")
	return s.status, nil
}

type craftingClassHookWorld struct {
	*state.World
	combatStatsCreatureID model.CreatureID
	familyName            string
	familyClass           int
	familyDailyExpndMax   int
	familyErr             error
}

func (w *craftingClassHookWorld) RecalculateCreatureCombatStats(creatureID model.CreatureID) (model.Creature, error) {
	w.combatStatsCreatureID = creatureID
	creature, _ := w.Creature(creatureID)
	return creature, nil
}

func (w *craftingClassHookWorld) UpdateFamilyMemberAfterClassChange(name string, class int, dailyExpndMax int) error {
	w.familyName = name
	w.familyClass = class
	w.familyDailyExpndMax = dailyExpndMax
	return w.familyErr
}

func craftingClassPromptContext(pending *PendingLineHandler) *Context {
	return &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			ContextPendingLineKey: func(handler PendingLineHandler) {
				*pending = handler
			},
		},
	}
}

func runCraftingClassPending(t *testing.T, ctx *Context, pending *PendingLineHandler, line string, wantStatus Status) {
	t.Helper()
	if *pending == nil {
		t.Fatalf("pending handler is nil before input %q", line)
	}
	handler := *pending
	ctx.Output = nil
	status, err := handler(ctx, line)
	if err != nil {
		t.Fatalf("pending(%q) error = %v", line, err)
	}
	if status != wantStatus {
		t.Fatalf("pending(%q) status = %d, want %d; output %q", line, status, wantStatus, ctx.OutputString())
	}
}

func craftingClassWorld(t *testing.T, room model.Room, class int, experience int) *worldload.World {
	t.Helper()
	return craftingClassWorldWithTags(t, room, class, experience, nil)
}

func craftingClassWorldWithTags(t *testing.T, room model.Room, class int, experience int, playerTags []string) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, room)
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      room.ID,
		Metadata:    model.Metadata{Tags: playerTags},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      room.ID,
		Level:       20,
		Stats: map[string]int{
			"class":      class,
			"level":      20,
			"experience": experience,
			"gold":       10000000,
			"hpCurrent":  100,
			"hpMax":      100,
			"mpCurrent":  50,
			"mpMax":      50,
		},
	})
	addCraftingClassWeaponPrototypes(t, loaded)
	return loaded
}

func addCraftingClassWeaponPrototypes(t *testing.T, loaded *worldload.World) {
	t.Helper()
	weapons := []struct {
		name       string
		legacyType string
	}{
		{name: "도", legacyType: "0"},
		{name: "검", legacyType: "1"},
		{name: "봉", legacyType: "2"},
		{name: "창", legacyType: "3"},
		{name: "궁", legacyType: "4"},
	}
	for i, weapon := range weapons {
		mustAddLookPrototype(t, loaded, model.ObjectPrototype{
			ID:          legacyWeaponForgePrototypeID(i + 1),
			Kind:        model.ObjectKindWeapon,
			DisplayName: weapon.name,
			Properties: map[string]string{
				"type":         weapon.legacyType,
				"nDice":        "1",
				"sDice":        "1",
				"pDice":        "0",
				"shotsMax":     "0",
				"shotsCurrent": "0",
			},
		})
	}
}
