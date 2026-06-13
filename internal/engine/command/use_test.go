package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestUseHandlerDispatchesByLegacyObjectTypeAndClearsHidden(t *testing.T) {
	loaded := useWorld(t)
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden"}
	loaded.Players[player.ID] = player
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"hidden"}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)
	dispatcher := useDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "목검 사용")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 목검으로 전투태세를 취합니다." {
		t.Fatalf("status/output = %d/%q, want ready confirmation", status, ctx.OutputString())
	}
	assertEquippedSlot(t, runtime, "wield", "object:sword")
	updatedCreature, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updatedCreature.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("creature tags = %+v, want hidden cleared", updatedCreature.Metadata.Tags)
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("player tags = %+v, want hidden cleared", updatedPlayer.Metadata.Tags)
	}

	ctx = &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "갑옷 사용"); err != nil {
		t.Fatalf("DispatchLine() armor error = %v", err)
	}
	if got := ctx.OutputString(); got != "당신은 갑옷을 입었습니다." {
		t.Fatalf("armor output = %q, want wear confirmation", got)
	}
	assertEquippedSlot(t, runtime, "body", "object:armor")

	ctx = &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "횃불 사용"); err != nil {
		t.Fatalf("DispatchLine() light error = %v", err)
	}
	if got := ctx.OutputString(); got != "당신은 횃불을 쥐었습니다." {
		t.Fatalf("light output = %q, want hold confirmation", got)
	}
	assertEquippedSlot(t, runtime, "held", "object:torch")

	ctx = &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "치료약 사용"); err != nil {
		t.Fatalf("DispatchLine() potion error = %v", err)
	}
	if got := ctx.OutputString(); !strings.Contains(got, "당신은 치료약을 먹었습니다.") {
		t.Fatalf("potion output = %q, want drink confirmation", got)
	}
}

func TestUseHandlerRejectsMissingAndUnsupportedObjects(t *testing.T) {
	runtime := state.NewWorld(useWorld(t))
	dispatcher := useDispatcher(t, runtime)

	tests := []struct {
		line string
		want string
	}{
		{line: "사용", want: "무엇을 사용하시려구요?"},
		{line: "없는 사용", want: "무엇을 사용하시려구요?"},
		{line: "돌 사용", want: "그것을 어떻게 사용하시려구요?\n"},
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

func TestUseHandlerHiddenSideEffectsMatchLegacyLookupBoundary(t *testing.T) {
	t.Run("missing object keeps hidden", func(t *testing.T) {
		loaded := useWorld(t)
		seedUseActorHidden(t, loaded)
		runtime := state.NewWorld(loaded)
		dispatcher := useDispatcher(t, runtime)

		ctx := &Context{ActorID: "player:alice"}
		status, err := dispatcher.DispatchLine(ctx, "없는 사용")
		if err != nil {
			t.Fatalf("DispatchLine() error = %v", err)
		}
		if status != StatusDefault || ctx.OutputString() != "무엇을 사용하시려구요?" {
			t.Fatalf("status/output = %d/%q, want missing use prompt", status, ctx.OutputString())
		}
		assertUseActorHiddenRetained(t, runtime)
	})

	t.Run("found unsupported object clears hidden and PHIDDN", func(t *testing.T) {
		loaded := useWorld(t)
		seedUseActorHidden(t, loaded)
		runtime := state.NewWorld(loaded)
		dispatcher := useDispatcher(t, runtime)

		ctx := &Context{ActorID: "player:alice"}
		status, err := dispatcher.DispatchLine(ctx, "돌 사용")
		if err != nil {
			t.Fatalf("DispatchLine() error = %v", err)
		}
		if status != StatusDefault || ctx.OutputString() != "그것을 어떻게 사용하시려구요?\n" {
			t.Fatalf("status/output = %d/%q, want unsupported use prompt", status, ctx.OutputString())
		}
		assertUseActorHiddenCleared(t, runtime)
	})
}

func TestUseHandlerUsesFirstArgumentAsLegacyObjectTarget(t *testing.T) {
	runtime := state.NewWorld(useWorld(t))
	dispatcher := useDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "목검 허수아비 사용")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 목검으로 전투태세를 취합니다." {
		t.Fatalf("status/output = %d/%q, want first-arg ready confirmation", status, ctx.OutputString())
	}
	assertEquippedSlot(t, runtime, "wield", "object:sword")
}

func TestUseHandlerRoutesKeyUseToUnlockWithExitArgument(t *testing.T) {
	runtime := state.NewWorld(useWorld(t))
	dispatcher := useDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "열쇠 서 사용")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "딸깍" {
		t.Fatalf("status/output = %d/%q, want key use output", status, ctx.OutputString())
	}
	exit := mustRuntimeExit(t, runtime, "room:plaza", "서")
	if exitHasAnyFlag(exit, "locked") {
		t.Fatalf("exit flags = %+v, want locked removed", exit.Flags)
	}
	key, _ := runtime.Object("object:key")
	if got := key.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("key shotsCurrent = %q, want 1", got)
	}
}

func TestUseHandlerRoutesScrollUseToReadScroll(t *testing.T) {
	useSpellFailRoll(t, 0)
	runtime := state.NewWorld(useWorld(t))
	dispatcher := useDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "귀환 사용")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	want := "당신의 왼손에 발광 주문을 걸었습니다.\n왼손에서 황금빛이 뿜어져 나와 주위를 밝혀 줍니다.\n주문이 번쩍인다.\n\n모든 것을 읽고 나자 귀환 주문서의 형체가 먼지로 변하면서 바람과 함께 사라져 버렸습니다.\n"
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after use")
	}
}

func TestUseHandlerRoutesSpecialMapScrollToFileView(t *testing.T) {
	root := t.TempDir()
	writeSpecialMapScrollFixture(t, root, "고대_지도", "남쪽 벽에 표시가 있다.")
	loaded := useWorld(t)
	proto := loaded.ObjectPrototypes["prototype:scroll"]
	proto.DisplayName = "고대 지도"
	loaded.ObjectPrototypes[proto.ID] = proto
	scroll := loaded.Objects["object:scroll"]
	scroll.Properties["special"] = "SP_MAPSC"
	loaded.Objects[scroll.ID] = scroll
	runtime := state.NewWorld(loaded)
	dispatcher := useDispatcherWithRoot(t, runtime, root)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "고대 사용")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	want := "남쪽 벽에 표시가 있다.\n"
	if status != StatusDoPrompt || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("special map scroll was consumed")
	}
}

func TestUseHandlerDoesNotRouteRoomSpecialMapScrollWithoutUseFromFloor(t *testing.T) {
	root := t.TempDir()
	writeSpecialMapScrollFixture(t, root, "벽_지도", "바닥의 표식은 서쪽을 가리킨다.")
	loaded := useWorld(t)
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:room-map")
	loaded.Rooms[room.ID] = room
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:room-map",
		Kind:        model.ObjectKindMisc,
		DisplayName: "벽 지도",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:room-map",
		PrototypeID: "prototype:room-map",
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
		Properties:  map[string]string{"special": "SP_MAPSC"},
	})
	runtime := state.NewWorld(loaded)
	dispatcher := useDispatcherWithRoot(t, runtime, root)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "벽 사용")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	want := "무엇을 사용하시려구요?"
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
	if _, ok := runtime.Object("object:room-map"); !ok {
		t.Fatal("room special map scroll was consumed")
	}
}

func TestUseHandlerRoutesWandUseToZap(t *testing.T) {
	useSpellFailRoll(t, 0)
	runtime := state.NewWorld(useWorld(t))
	dispatcher := useDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "마법봉 상인 사용")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	wantOutput := "당신의 왼손에 발광 주문을 걸었습니다.\n왼손에서 황금빛이 뿜어져 나와 주위를 밝혀 줍니다.\n번쩍\n"
	if status != StatusDefault || ctx.OutputString() != wantOutput {
		t.Fatalf("status/output = %d/%q, want wand use output", status, ctx.OutputString())
	}
	creature, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "PLIGHT", "light") {
		t.Fatalf("creature tags = %+v, want PLIGHT", creature.Metadata.Tags)
	}
	wand, _ := runtime.Object("object:wand")
	if got := wand.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("wand shotsCurrent = %q, want consumed 1", got)
	}
}

func TestUseHandlerDoesNotRouteInventorySpecialComboBeforeGenericUse(t *testing.T) {
	loaded := useWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:combo")
	loaded.Creatures[creature.ID] = creature
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:combo",
		DisplayName: "조합상자",
		Properties:  map[string]string{"type": "13"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:combo",
		PrototypeID: "prototype:combo",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties: map[string]string{
			"special":   "2",
			"pDice":     "1",
			"useOutput": "123",
		},
	})

	runtime := state.NewWorld(loaded)
	dispatcher := useDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "조합 사용")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	want := "그것을 어떻게 사용하시려구요?\n"
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
	exit := mustRuntimeExit(t, runtime, "room:plaza", "서")
	if !exitHasAnyFlag(exit, "locked", "closed") {
		t.Fatalf("exit flags = %+v, want still locked and closed", exit.Flags)
	}
}

func TestUseHandlerDoesNotFindEquippedSpecialComboLikeLegacyUse(t *testing.T) {
	loaded := useWorld(t)
	creature := loaded.Creatures["creature:alice"]
	if creature.Equipment == nil {
		creature.Equipment = map[string]model.ObjectInstanceID{}
	}
	creature.Equipment["held"] = "object:combo"
	loaded.Creatures[creature.ID] = creature
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:combo",
		DisplayName: "조합상자",
		Properties:  map[string]string{"type": "13"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:combo",
		PrototypeID: "prototype:combo",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "held"},
		Properties: map[string]string{
			"special":   "SP_COMBO",
			"pDice":     "1",
			"useOutput": "123",
		},
	})

	runtime := state.NewWorld(loaded)
	dispatcher := useDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "조합 123 사용")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	want := "무엇을 사용하시려구요?"
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
	exit := mustRuntimeExit(t, runtime, "room:plaza", "서")
	if !exitHasAnyFlag(exit, "locked", "closed") {
		t.Fatalf("exit flags = %+v, want still locked and closed", exit.Flags)
	}
}

func TestUseHandlerDoesNotFindRoomSpecialComboWithoutUseFromFloor(t *testing.T) {
	loaded := useWorld(t)
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:room-combo")
	loaded.Rooms[room.ID] = room
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:room-combo",
		DisplayName: "조합장치",
		Properties:  map[string]string{"type": "13"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:room-combo",
		PrototypeID: "prototype:room-combo",
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
		Properties: map[string]string{
			"special":   "SP_COMBO",
			"pDice":     "1",
			"useOutput": "123",
		},
	})

	runtime := state.NewWorld(loaded)
	dispatcher := useDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "조합 123 사용")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	want := "무엇을 사용하시려구요?"
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
	exit := mustRuntimeExit(t, runtime, "room:plaza", "서")
	if !exitHasAnyFlag(exit, "locked", "closed") {
		t.Fatalf("exit flags = %+v, want still locked and closed", exit.Flags)
	}
}

func TestUseHandlerLetsSpecialComboScrollFallThroughToReadScroll(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := useWorld(t)
	scroll := loaded.Objects["object:scroll"]
	scroll.Properties["special"] = "2"
	scroll.Properties["useOutput"] = ""
	loaded.Objects[scroll.ID] = scroll
	runtime := state.NewWorld(loaded)
	dispatcher := useDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "귀환 사용")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	want := "당신의 왼손에 발광 주문을 걸었습니다.\n왼손에서 황금빛이 뿜어져 나와 주위를 밝혀 줍니다.\n\n모든 것을 읽고 나자 귀환 주문서의 형체가 먼지로 변하면서 바람과 함께 사라져 버렸습니다.\n"
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("special combo scroll was not consumed by generic readscroll routing")
	}
}

func TestUseHandlerStopsSpecialWarObjectsBeforeGenericRouting(t *testing.T) {
	tests := []struct {
		name       string
		objectProp map[string]string
		protoProp  map[string]string
	}{
		{
			name:       "legacy special number",
			objectProp: map[string]string{"special": "3"},
		},
		{
			name:       "legacy special symbol",
			objectProp: map[string]string{"special": "SP_WAR"},
		},
		{
			name:      "prototype sp war flag",
			protoProp: map[string]string{"SP_WAR": "1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := useWorld(t)
			scroll := loaded.Objects["object:scroll"]
			if scroll.Properties == nil {
				scroll.Properties = map[string]string{}
			}
			for key, value := range tt.objectProp {
				scroll.Properties[key] = value
			}
			loaded.Objects[scroll.ID] = scroll

			proto := loaded.ObjectPrototypes["prototype:scroll"]
			if proto.Properties == nil {
				proto.Properties = map[string]string{}
			}
			for key, value := range tt.protoProp {
				proto.Properties[key] = value
			}
			loaded.ObjectPrototypes[proto.ID] = proto

			runtime := state.NewWorld(loaded)
			dispatcher := useDispatcher(t, runtime)

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, "귀환 사용")
			if err != nil {
				t.Fatalf("DispatchLine() error = %v", err)
			}
			want := "아무것도 없습니다.\n"
			if status != StatusDefault || ctx.OutputString() != want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
			}
			if _, ok := runtime.Object("object:scroll"); !ok {
				t.Fatal("special war object was consumed by generic readscroll routing")
			}
		})
	}
}

func TestUseHandlerDoesNotFindSpecialWarObjectsInEquipmentAndRoom(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*worldload.World)
	}{
		{
			name: "equipped special war",
			setup: func(loaded *worldload.World) {
				creature := loaded.Creatures["creature:alice"]
				if creature.Equipment == nil {
					creature.Equipment = map[string]model.ObjectInstanceID{}
				}
				creature.Equipment["held"] = "object:war-banner"
				loaded.Creatures[creature.ID] = creature
				mustAddLookObject(t, loaded, model.ObjectInstance{
					ID:          "object:war-banner",
					PrototypeID: "prototype:war-banner",
					Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "held"},
					Properties:  map[string]string{"special": "SP_WAR"},
				})
			},
		},
		{
			name: "room special war without use from floor",
			setup: func(loaded *worldload.World) {
				room := loaded.Rooms["room:plaza"]
				room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:war-banner")
				loaded.Rooms[room.ID] = room
				mustAddLookObject(t, loaded, model.ObjectInstance{
					ID:          "object:war-banner",
					PrototypeID: "prototype:war-banner",
					Location:    model.ObjectLocation{RoomID: "room:plaza"},
					Properties:  map[string]string{"special": "SP_WAR"},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := useWorld(t)
			mustAddLookPrototype(t, loaded, model.ObjectPrototype{
				ID:          "prototype:war-banner",
				DisplayName: "전쟁깃발",
				Properties:  map[string]string{"type": "13"},
			})
			tt.setup(loaded)
			runtime := state.NewWorld(loaded)
			dispatcher := useDispatcher(t, runtime)

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, "전쟁 사용")
			if err != nil {
				t.Fatalf("DispatchLine() error = %v", err)
			}
			want := "무엇을 사용하시려구요?"
			if status != StatusDefault || ctx.OutputString() != want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
			}
			if _, ok := runtime.Object("object:war-banner"); !ok {
				t.Fatal("special war object was consumed")
			}
		})
	}
}

func TestUseHandlerAllowsUseFromFloorObjects(t *testing.T) {
	runtime := state.NewWorld(useWorld(t))
	dispatcher := useDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "바닥장치 사용")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그것을 어떻게 사용하시려구요?\n" {
		t.Fatalf("status/output = %d/%q, want unsupported floor use", status, ctx.OutputString())
	}
}

func TestUseHandlerAllowsPropertyBackedUseFromFloorObjects(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*worldload.World)
	}{
		{
			name: "instance_direct_flag",
			setup: func(loaded *worldload.World) {
				proto := loaded.ObjectPrototypes["prototype:floor-device"]
				proto.Metadata.Tags = nil
				loaded.ObjectPrototypes[proto.ID] = proto
				device := loaded.Objects["object:floor-device"]
				device.Properties = map[string]string{"OUSEFL": "1"}
				loaded.Objects[device.ID] = device
			},
		},
		{
			name: "prototype_flags_token",
			setup: func(loaded *worldload.World) {
				proto := loaded.ObjectPrototypes["prototype:floor-device"]
				proto.Metadata.Tags = nil
				proto.Properties["flags"] = "OUSEFL"
				loaded.ObjectPrototypes[proto.ID] = proto
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := useWorld(t)
			tt.setup(loaded)
			runtime := state.NewWorld(loaded)
			dispatcher := useDispatcher(t, runtime)

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, "바닥장치 사용")
			if err != nil {
				t.Fatalf("DispatchLine() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != "그것을 어떻게 사용하시려구요?\n" {
				t.Fatalf("status/output = %d/%q, want unsupported floor use", status, ctx.OutputString())
			}
		})
	}
}

func TestUseHandlerUseFromFloorVisibilityUsesPDINVI(t *testing.T) {
	loaded := useWorld(t)
	device := loaded.Objects["object:floor-device"]
	device.Metadata.Tags = []string{"OINVIS"}
	loaded.Objects[device.ID] = device
	runtime := state.NewWorld(loaded)
	dispatcher := useDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "바닥장치 사용")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "무엇을 사용하시려구요?" {
		t.Fatalf("status/output = %d/%q, want missing invisible floor object", status, ctx.OutputString())
	}

	loaded = useWorld(t)
	device = loaded.Objects["object:floor-device"]
	device.Metadata.Tags = []string{"OINVIS"}
	loaded.Objects[device.ID] = device
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"PDINVI"}
	loaded.Creatures[creature.ID] = creature
	runtime = state.NewWorld(loaded)
	dispatcher = useDispatcher(t, runtime)

	ctx = &Context{ActorID: "player:alice"}
	status, err = dispatcher.DispatchLine(ctx, "바닥장치 사용")
	if err != nil {
		t.Fatalf("DispatchLine() with PDINVI error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그것을 어떻게 사용하시려구요?\n" {
		t.Fatalf("status/output = %d/%q, want visible floor object", status, ctx.OutputString())
	}
}

func TestUseHandlerRoutesUseFromFloorWandToFloorObject(t *testing.T) {
	loaded := useWorld(t)
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:floor-wand")
	loaded.Rooms[room.ID] = room
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:floor-wand",
		Kind:        model.ObjectKindWand,
		DisplayName: "바닥마법봉",
		Properties:  map[string]string{"type": "8"},
		Metadata:    model.Metadata{Tags: []string{"useFromFloor"}},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:floor-wand",
		PrototypeID: "prototype:floor-wand",
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
		Properties:  map[string]string{"shotsCurrent": "2", "magicPower": "0"},
	})
	runtime := state.NewWorld(loaded)
	dispatcher := useDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "바닥마법봉 사용")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want C-style floor wand no-op", status, ctx.OutputString())
	}
	wand, _ := runtime.Object("object:floor-wand")
	if got := wand.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("floor wand shotsCurrent = %q, want pre-decremented 1", got)
	}
}

func TestUseHandlerFloorWandLightUsesZapObjDoubleChargeLikeC(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := useWorld(t)
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:floor-wand")
	loaded.Rooms[room.ID] = room
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:floor-wand",
		Kind:        model.ObjectKindWand,
		DisplayName: "바닥마법봉",
		Properties:  map[string]string{"type": "8"},
		Metadata:    model.Metadata{Tags: []string{"useFromFloor"}},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:floor-wand",
		PrototypeID: "prototype:floor-wand",
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
		Properties:  map[string]string{"shotsCurrent": "2", "magicPower": "3", "useOutput": "바닥번쩍"},
	})
	runtime := state.NewWorld(loaded)
	dispatcher := useDispatcher(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "바닥마법봉 사용")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	wantOutput := "당신의 왼손에 발광 주문을 걸었습니다.\n왼손에서 황금빛이 뿜어져 나와 주위를 밝혀 줍니다.\n바닥번쩍\n"
	if status != StatusDefault || ctx.OutputString() != wantOutput {
		t.Fatalf("status/output = %d/%q, want floor wand use output", status, ctx.OutputString())
	}
	creature, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "PLIGHT", "light") {
		t.Fatalf("creature tags = %+v, want PLIGHT", creature.Metadata.Tags)
	}
	wand, _ := runtime.Object("object:floor-wand")
	if got := wand.Properties["shotsCurrent"]; got != "0" {
		t.Fatalf("floor wand shotsCurrent = %q, want double-decremented 0", got)
	}
}

func useDispatcher(t *testing.T, world *state.World) Dispatcher {
	t.Helper()
	return useDispatcherWithRoot(t, world, "")
}

func useDispatcherWithRoot(t *testing.T, world *state.World, root string) Dispatcher {
	t.Helper()
	return Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "사용", Number: 67, Handler: "use"},
		}),
		Handlers: map[string]Handler{
			"use": NewUseHandlerWithRoot(world, root, nil),
		},
	}
}

func useWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := equipmentWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs,
		"object:potion",
		"object:torch",
		"object:stone",
		"object:key",
		"object:scroll",
		"object:wand",
	)
	loaded.Creatures[creature.ID] = creature
	player := loaded.Players["player:alice"]
	player.RoomID = "room:plaza"
	loaded.Players[player.ID] = player
	creature.RoomID = "room:plaza"
	loaded.Creatures[creature.ID] = creature
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:plaza",
		DisplayName: "광장",
		Objects:     model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:floor-device"}},
		Exits: []model.Exit{
			{Name: "서", ToRoomID: "room:west", Flags: []string{"locked", "closed", "lockable", "key:7"}},
		},
	})
	mustAddLookRoom(t, loaded, model.Room{ID: "room:west", DisplayName: "서쪽"})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:potion",
		Kind:        model.ObjectKindPotion,
		DisplayName: "치료약",
		Properties:  map[string]string{"type": "6"},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:torch",
		Kind:        model.ObjectKindLightSource,
		DisplayName: "횃불",
		Properties:  map[string]string{"type": "12", "wearFlag": "17"},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:stone",
		Kind:        model.ObjectKindMisc,
		DisplayName: "돌",
		Properties:  map[string]string{"type": "13"},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:key",
		Kind:        model.ObjectKindKey,
		DisplayName: "열쇠",
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:scroll",
		Kind:        model.ObjectKindScroll,
		DisplayName: "귀환 주문서",
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:wand",
		Kind:        model.ObjectKindWand,
		DisplayName: "마법봉",
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:floor-device",
		DisplayName: "바닥장치",
		Properties:  map[string]string{"type": "13"},
		Metadata:    model.Metadata{Tags: []string{"useFromFloor"}},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:potion",
		PrototypeID: "prototype:potion",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"shotsCurrent": "2", "magicPower": "1"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:torch",
		PrototypeID: "prototype:torch",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:stone",
		PrototypeID: "prototype:stone",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:key",
		PrototypeID: "prototype:key",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"nDice": "7", "shotsCurrent": "2", "useOutput": "딸깍"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:scroll",
		PrototypeID: "prototype:scroll",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"shotsCurrent": "1", "magicPower": "3", "useOutput": "주문이 번쩍인다."},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:wand",
		PrototypeID: "prototype:wand",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"shotsCurrent": "2", "magicPower": "3", "useOutput": "번쩍"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:floor-device",
		PrototypeID: "prototype:floor-device",
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
	})
	return loaded
}

func seedUseActorHidden(t *testing.T, loaded *worldload.World) {
	t.Helper()
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "hidden", "PHIDDN")
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats["PHIDDN"] = 1
	loaded.Creatures[creature.ID] = creature
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = append(player.Metadata.Tags, "hidden", "PHIDDN")
	loaded.Players[player.ID] = player
}

func assertUseActorHiddenRetained(t *testing.T, runtime *state.World) {
	t.Helper()
	creature, _ := runtime.Creature("creature:alice")
	player, _ := runtime.Player("player:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "hidden", "PHIDDN") || creature.Stats["PHIDDN"] != 1 {
		t.Fatalf("creature hidden state = tags:%+v stats:%+v, want retained", creature.Metadata.Tags, creature.Stats)
	}
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "PHIDDN") {
		t.Fatalf("player hidden tags = %+v, want retained", player.Metadata.Tags)
	}
}

func assertUseActorHiddenCleared(t *testing.T, runtime *state.World) {
	t.Helper()
	creature, _ := runtime.Creature("creature:alice")
	player, _ := runtime.Player("player:alice")
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "hidden", "PHIDDN") || creature.Stats["PHIDDN"] != 0 {
		t.Fatalf("creature hidden state = tags:%+v stats:%+v, want cleared", creature.Metadata.Tags, creature.Stats)
	}
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "PHIDDN") {
		t.Fatalf("player hidden tags = %+v, want cleared", player.Metadata.Tags)
	}
}
