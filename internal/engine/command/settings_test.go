package command

import (
	"strings"
	"testing"

	"muhan/internal/commandspec"
	"muhan/internal/world/state"
)

func TestSetHandlerListsAndTogglesLegacySettings(t *testing.T) {
	runtime := state.NewWorld(emptyInventoryWorld(t))
	dispatcher := settingsDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "설정")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "이야기듣기:  설정") ||
		!strings.Contains(ctx.OutputString(), "[설정 도움말]") {
		t.Fatalf("status/output = %d/%q, want settings list", status, ctx.OutputString())
	}

	ctx = &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "잡담듣기 설정"); err != nil {
		t.Fatalf("DispatchLine() toggle error = %v", err)
	}
	creature, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "PNOBRD") || creature.Stats["PNOBRD"] != 1 {
		t.Fatalf("creature = %+v, want PNOBRD tag/stat", creature)
	}
	if !strings.Contains(ctx.OutputString(), "잡담듣기  : 미설정") {
		t.Fatalf("toggle output = %q, want no-broadcast state", ctx.OutputString())
	}
}

func TestSetHandlerTogglesStatBackedLegacyFlagLikeC(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Stats = map[string]int{}
	creature.Stats["PNOBRD"] = 1
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)
	dispatcher := settingsDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "잡담듣기 설정"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	updated, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "PNOBRD") || updated.Stats["PNOBRD"] != 0 {
		t.Fatalf("creature = %+v, want stat-backed PNOBRD cleared", updated)
	}
	if !strings.Contains(ctx.OutputString(), "잡담듣기  :  설정 ") {
		t.Fatalf("output = %q, want broadcast setting enabled after toggle off", ctx.OutputString())
	}
}

func TestSetHandlerRendersLegacyCreatureFlags(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"PNOBRD", "PANSIC", "PNOEXT"}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)
	dispatcher := settingsDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "설정"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	for _, want := range []string{"잡담듣기  : 미설정", "색        :  사용 ", "출구      : 그래프"} {
		if !strings.Contains(ctx.OutputString(), want) {
			t.Fatalf("output missing %q:\n%s", want, ctx.OutputString())
		}
	}
}

func TestSetHandlerRendersPropertyBackedLegacyCreatureFlags(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Properties = map[string]string{"flags": "PNOBRD|PANSIC|PNOEXT"}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)
	dispatcher := settingsDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "설정"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	for _, want := range []string{"잡담듣기  : 미설정", "색        :  사용 ", "출구      : 그래프"} {
		if !strings.Contains(ctx.OutputString(), want) {
			t.Fatalf("output missing %q:\n%s", want, ctx.OutputString())
		}
	}
}

func TestSetHandlerClearsPropertyBackedLegacyCreatureFlagsLikeC(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Properties = map[string]string{"PNOBRD": "1", "flags": "PANSIC|PNOEXT"}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)
	dispatcher := settingsDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "잡담듣기 설정"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	updated, _ := runtime.Creature("creature:alice")
	if updated.Properties["PNOBRD"] != "" || updated.Stats["PNOBRD"] != 0 || hasAnyNormalizedFlag(updated.Metadata.Tags, "PNOBRD") {
		t.Fatalf("creature = %+v, want property-backed PNOBRD cleared", updated)
	}
	if !strings.Contains(ctx.OutputString(), "잡담듣기  :  설정 ") {
		t.Fatalf("output = %q, want broadcast setting enabled after property-backed toggle off", ctx.OutputString())
	}

	ctx = &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "색 해제"); err != nil {
		t.Fatalf("DispatchLine() clear color error = %v", err)
	}
	updated, _ = runtime.Creature("creature:alice")
	if strings.Contains(updated.Properties["flags"], "PANSIC") || updated.Stats["PANSIC"] != 0 || hasAnyNormalizedFlag(updated.Metadata.Tags, "PANSIC") {
		t.Fatalf("creature = %+v, want property-backed PANSIC cleared", updated)
	}
	if !strings.Contains(updated.Properties["flags"], "PNOEXT") {
		t.Fatalf("creature flags property = %q, want unrelated PNOEXT preserved", updated.Properties["flags"])
	}
}

func TestSetHandlerSetsAndClearsWimpyValue(t *testing.T) {
	runtime := state.NewWorld(emptyInventoryWorld(t))
	dispatcher := settingsDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "도망수치#5 설정"); err != nil {
		t.Fatalf("DispatchLine() set wimpy error = %v", err)
	}
	creature, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "PWIMPY") || creature.Stats["PWIMPY"] != 1 || creature.Stats["wimpyValue"] != 5 {
		t.Fatalf("creature = %+v, want PWIMPY wimpyValue 5", creature)
	}
	if !strings.Contains(ctx.OutputString(), "도망수치  : 5") {
		t.Fatalf("set output = %q, want wimpy value", ctx.OutputString())
	}

	ctx = &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "도망수치 해제"); err != nil {
		t.Fatalf("DispatchLine() clear wimpy error = %v", err)
	}
	creature, _ = runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "PWIMPY") || creature.Stats["PWIMPY"] != 0 || creature.Stats["wimpyValue"] != 5 {
		t.Fatalf("creature = %+v, want PWIMPY cleared and value retained", creature)
	}
	if got := ctx.OutputString(); got != "도망수치 설정이 해제되었습니다.\n" {
		t.Fatalf("clear output = %q, want legacy clear message", got)
	}
}

func TestSetHandlerFamilyReturnUsesCreatureFamilyFlag(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Stats = map[string]int{"familyFlag": 1}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)
	dispatcher := settingsDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "패거리귀환 설정"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	updated, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "PFRTUN") || updated.Stats["PFRTUN"] != 1 {
		t.Fatalf("creature = %+v, want PFRTUN tag/stat", updated)
	}
	if !strings.Contains(ctx.OutputString(), "패거리 존으로 귀환을 합니다.") ||
		!strings.Contains(ctx.OutputString(), "패거리귀환:  설정 ") {
		t.Fatalf("output = %q, want family return confirmation and state", ctx.OutputString())
	}
}

func TestSetHandlerPrivilegedSettingsUseCreatureClass(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Stats = map[string]int{"class": legacyClassSubDM}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)
	dispatcher := settingsDispatcher(t, runtime)

	for _, tt := range []struct {
		line string
		tag  string
		want string
	}{
		{line: "hexline 설정", tag: "PHEXLN", want: "Hexline enabled.\n"},
		{line: "eavesdropper 설정", tag: "PEAVES", want: "Eavesdropper mode enabled.\n"},
		{line: "~robot~ 설정", tag: "PROBOT", want: "Robot mode on.\n"},
	} {
		t.Run(tt.line, func(t *testing.T) {
			ctx := &Context{ActorID: "player:alice"}
			if _, err := dispatcher.DispatchLine(ctx, tt.line); err != nil {
				t.Fatalf("DispatchLine() error = %v", err)
			}
			updated, _ := runtime.Creature("creature:alice")
			if !hasAnyNormalizedFlag(updated.Metadata.Tags, tt.tag) || updated.Stats[tt.tag] != 1 {
				t.Fatalf("creature = %+v, want %s tag/stat", updated, tt.tag)
			}
			if !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("output = %q, want %q", ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestClearHandlerClearsStatBackedLegacyFlagsLikeC(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Stats = map[string]int{}
	creature.Stats["PIGNOR"] = 1
	creature.Stats["PANSIC"] = 1
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)
	dispatcher := settingsDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "이야기듣기거부 해제"); err != nil {
		t.Fatalf("DispatchLine() clear PIGNOR error = %v", err)
	}
	updated, _ := runtime.Creature("creature:alice")
	if updated.Stats["PIGNOR"] != 0 || hasAnyNormalizedFlag(updated.Metadata.Tags, "PIGNOR") {
		t.Fatalf("creature = %+v, want PIGNOR cleared", updated)
	}

	ctx = &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "색 해제"); err != nil {
		t.Fatalf("DispatchLine() clear PANSIC error = %v", err)
	}
	updated, _ = runtime.Creature("creature:alice")
	if updated.Stats["PANSIC"] != 0 || hasAnyNormalizedFlag(updated.Metadata.Tags, "PANSIC") {
		t.Fatalf("creature = %+v, want PANSIC cleared", updated)
	}
}

func TestClearHandlerReportsMissingAndInvalidTargets(t *testing.T) {
	runtime := state.NewWorld(emptyInventoryWorld(t))
	dispatcher := settingsDispatcher(t, runtime)

	tests := []struct {
		line string
		want string
	}{
		{line: "해제", want: "[해제 도움말]이라고 치시면 모든 설정사항들을 볼 수 있습니다.\n"},
		{line: "없는 해제", want: "잘못 지정되었습니다.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, tt.line)
			if err != nil {
				t.Fatalf("DispatchLine() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func settingsDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()
	return Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "설정", Number: 27, Handler: "set"},
			{Name: "해제", Number: 28, Handler: "clear"},
		}),
		Handlers: map[string]Handler{
			"set":   NewSetHandler(world),
			"clear": NewClearHandler(world),
		},
	}
}
