package command

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

func TestGetHandlerMovesRoomObjectIntoLinkedCreatureInventoryAndConfirms(t *testing.T) {
	world := newGetFakeWorld()
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "주워", Number: 5, Handler: "get"},
		}),
		Handlers: map[string]Handler{
			"get": NewGetHandler(world),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "빛 주워")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	if got := ctx.OutputString(); got != "당신은 빛나는 검을 줍습니다.\n" {
		t.Fatalf("output = %q, want get confirmation", got)
	}
	object := world.objects["object:sword"]
	if object.Location.CreatureID != "creature:alice" || object.Location.Slot != "inventory" || !object.Location.RoomID.IsZero() {
		t.Fatalf("object location = %+v, want creature inventory", object.Location)
	}
	room := world.rooms["room:plaza"]
	if slices.Contains(room.Objects.ObjectIDs, model.ObjectInstanceID("object:sword")) {
		t.Fatalf("room still references moved object: %+v", room.Objects.ObjectIDs)
	}
	creature := world.creatures["creature:alice"]
	if !slices.Contains(creature.Inventory.ObjectIDs, model.ObjectInstanceID("object:sword")) {
		t.Fatalf("creature inventory = %+v, want object:sword", creature.Inventory.ObjectIDs)
	}
}

func TestGetHandlerBroadcastsSingleRoomPickupLikeLegacy(t *testing.T) {
	world := newGetFakeWorld()
	handler := NewGetHandler(world)
	var broadcasts []struct {
		roomID         model.RoomID
		excludeSession string
		text           string
	}
	ctx := &Context{
		SessionID: "s1",
		ActorID:   "player:alice",
		Values: map[string]any{
			ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
				broadcasts = append(broadcasts, struct {
					roomID         model.RoomID
					excludeSession string
					text           string
				}{roomID: roomID, excludeSession: excludeSessionID, text: text})
				return errors.New("session closed")
			}),
		},
	}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"빛"}})
	if err != nil {
		t.Fatalf("handler() error = %v, want nil", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 빛나는 검을 줍습니다.\n" {
		t.Fatalf("status/output = %d/%q, want pickup confirmation", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 {
		t.Fatalf("broadcasts = %+v, want one room broadcast", broadcasts)
	}
	if got := broadcasts[0]; got.roomID != "room:plaza" || got.excludeSession != "s1" || got.text != "\nAlice가 빛나는 검을 줍습니다." {
		t.Fatalf("broadcast = %+v, want legacy room pickup", got)
	}
}

func TestGetHandlerRejectsObjectIDTargetLikeLegacyFindObj(t *testing.T) {
	world := newGetFakeWorld()
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"object:sword"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그런건 여기 없어요." {
		t.Fatalf("status/output = %d/%q, want missing object ID target", status, ctx.OutputString())
	}
	object := world.objects["object:sword"]
	if object.Location.RoomID != "room:plaza" {
		t.Fatalf("sword location = %+v, want still in room", object.Location)
	}
}

func TestTakeHandlerUsesFirstArgPrefixAndOrdinalForRoomObject(t *testing.T) {
	world := newGetFakeWorld()
	handler := NewTakeHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"금"},
		Values: []int64{2},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	if got := ctx.OutputString(); got != "당신은 금반지를 줍습니다.\n" {
		t.Fatalf("output = %q, want second 금* object confirmation", got)
	}
	object := world.objects["object:ring"]
	if object.Location.CreatureID != "creature:alice" || object.Location.Slot != "inventory" {
		t.Fatalf("object location = %+v, want creature inventory", object.Location)
	}
	if moved := world.moves[0].id; moved != "object:ring" {
		t.Fatalf("moved object = %q, want object:ring", moved)
	}
}

func TestGetHandlerGetsAllMatchingRoomObjects(t *testing.T) {
	world := newGetFakeWorld()
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"모든금"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 금화, 금반지를 줍습니다.\n" {
		t.Fatalf("output = %q, want bulk get confirmation", got)
	}
	for _, objectID := range []model.ObjectInstanceID{"object:coin", "object:ring"} {
		object := world.objects[objectID]
		if object.Location.CreatureID != "creature:alice" || object.Location.Slot != "inventory" {
			t.Fatalf("%s location = %+v, want creature inventory", objectID, object.Location)
		}
	}
	if len(world.moves) != 2 {
		t.Fatalf("moves = %+v, want two moves", world.moves)
	}
}

func TestGetHandlerBroadcastsBulkRoomPickupLikeLegacy(t *testing.T) {
	world := newGetFakeWorld()
	handler := NewGetHandler(world)
	var broadcasts []string
	ctx := &Context{
		SessionID: "s1",
		ActorID:   "player:alice",
		Values: map[string]any{
			ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
				if roomID != "room:plaza" || excludeSessionID != "s1" {
					t.Fatalf("broadcast room/exclude = %q/%q, want room:plaza/s1", roomID, excludeSessionID)
				}
				broadcasts = append(broadcasts, text)
				return errors.New("session closed")
			}),
		},
	}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"모든금"}})
	if err != nil {
		t.Fatalf("handler() error = %v, want nil", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 금화, 금반지를 줍습니다.\n" {
		t.Fatalf("status/output = %d/%q, want bulk get confirmation", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0] != "\nAlice가 금화, 금반지를 줍습니다." {
		t.Fatalf("broadcasts = %+v, want legacy bulk room get", broadcasts)
	}
}

func TestGetHandlerRejectsSingleRoomObjectOverCarryCapacity(t *testing.T) {
	world := newGetFakeWorld()
	proto := world.prototypes["prototype:sword"]
	proto.Properties = map[string]string{"weight": "25"}
	world.prototypes[proto.ID] = proto
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"검"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 더이상 가질 수 없습니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	if len(world.moves) != 0 {
		t.Fatalf("moves = %+v, want none", world.moves)
	}
	object := world.objects["object:sword"]
	if object.Location.RoomID != "room:plaza" {
		t.Fatalf("object location = %+v, want room", object.Location)
	}
}

func TestGetHandlerRejectsSingleRoomInvisibleAndUntakableObjects(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*getFakeWorld)
		target     string
		wantOutput string
	}{
		{
			name: "invisible",
			setup: func(world *getFakeWorld) {
				addGetRoomObject(world, "object:invisible", "prototype:invisible", "은신검", nil, []string{"OINVIS"})
			},
			target:     "은신",
			wantOutput: "그런건 여기 없어요.",
		},
		{
			name: "no take",
			setup: func(world *getFakeWorld) {
				addGetRoomObject(world, "object:notake", "prototype:notake", "붙박이 석상", map[string]string{"ONOTAK": "1"}, nil)
			},
			target:     "붙박이",
			wantOutput: "주을 수 있는 물건이 아닙니다.",
		},
		{
			name: "scenery",
			setup: func(world *getFakeWorld) {
				addGetRoomObject(world, "object:scene", "prototype:scene", "벽화", nil, []string{"OSCENE"})
			},
			target:     "벽화",
			wantOutput: "주을 수 있는 물건이 아닙니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := newGetFakeWorld()
			tt.setup(world)
			handler := NewGetHandler(world)

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{Args: []string{tt.target}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.wantOutput {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.wantOutput)
			}
			if len(world.moves) != 0 {
				t.Fatalf("moves = %+v, want none", world.moves)
			}
		})
	}
}

func TestGetHandlerGetsAllSkipsObjectsOverCarryCapacity(t *testing.T) {
	world := newGetFakeWorld()
	coinProto := world.prototypes["prototype:coin"]
	coinProto.Properties = map[string]string{"weight": "25"}
	world.prototypes[coinProto.ID] = coinProto
	ringProto := world.prototypes["prototype:ring"]
	ringProto.Properties = map[string]string{"weight": "1"}
	world.prototypes[ringProto.ID] = ringProto
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"모든금"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	want := "가지고 있는 물건이 너무 무거워 들 수가 없습니다.\n당신은 금반지를 줍습니다.\n"
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
	if world.objects["object:coin"].Location.RoomID != "room:plaza" {
		t.Fatalf("coin location = %+v, want skipped in room", world.objects["object:coin"].Location)
	}
	if world.objects["object:ring"].Location.CreatureID != "creature:alice" {
		t.Fatalf("ring location = %+v, want inventory", world.objects["object:ring"].Location)
	}
}

func TestGetHandlerGetsAllGroupsAdjacentDuplicateRoomObjects(t *testing.T) {
	world := newGetFakeWorld()
	addGetRoomObject(world, "object:apple-1", "prototype:apple", "사과", nil, nil)
	addGetRoomObject(world, "object:apple-2", "prototype:apple", "사과", nil, nil)
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"모든사"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 (x2) 사과를 줍습니다.\n" {
		t.Fatalf("status/output = %d/%q, want grouped room get", status, ctx.OutputString())
	}
	for _, objectID := range []model.ObjectInstanceID{"object:apple-1", "object:apple-2"} {
		if world.objects[objectID].Location.CreatureID != "creature:alice" {
			t.Fatalf("%s location = %+v, want inventory", objectID, world.objects[objectID].Location)
		}
	}
}

func TestGetHandlerGetsAllInvisibleRoomObjectsOnlyForDetector(t *testing.T) {
	tests := []struct {
		name         string
		creatureTags []string
		wantOutput   string
		wantMoved    bool
	}{
		{
			name:       "no detect invisible",
			wantOutput: "여기에는 아무것도 없습니다.",
		},
		{
			name:         "detect invisible",
			creatureTags: []string{"PDINVI"},
			wantOutput:   "당신은 은신검을 줍습니다.\n",
			wantMoved:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := newGetFakeWorld()
			creature := world.creatures["creature:alice"]
			creature.Metadata.Tags = append(creature.Metadata.Tags, tt.creatureTags...)
			world.creatures[creature.ID] = creature
			addGetRoomObject(world, "object:invisible", "prototype:invisible", "은신검", nil, []string{"OINVIS"})
			handler := NewGetHandler(world)

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{Args: []string{"모든은신"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.wantOutput {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.wantOutput)
			}
			if tt.wantMoved {
				if world.objects["object:invisible"].Location.CreatureID != "creature:alice" {
					t.Fatalf("invisible location = %+v, want inventory", world.objects["object:invisible"].Location)
				}
			} else if len(world.moves) != 0 {
				t.Fatalf("moves = %+v, want none", world.moves)
			}
		})
	}
}

func TestTakeHandlerCountsContainerContentsForCarryLimit(t *testing.T) {
	world := newGetFakeWorld()
	bag := world.objects["object:bag"]
	bag.Location = model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"}
	bag.Properties["shotsCurrent"] = "151"
	world.objects[bag.ID] = bag
	room := world.rooms["room:plaza"]
	room.Objects.ObjectIDs = getFakeRemoveObjectID(room.Objects.ObjectIDs, bag.ID)
	world.rooms[room.ID] = room
	creature := world.creatures["creature:alice"]
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, bag.ID)
	world.creatures[creature.ID] = creature

	handler := NewGetHandler(world)
	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"보석"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 더이상 가질 수 없습니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	if len(world.moves) != 0 {
		t.Fatalf("moves = %+v, want none", world.moves)
	}
	if world.objects["object:gem"].Location.ContainerID != "object:bag" {
		t.Fatalf("gem location = %+v, want still in bag", world.objects["object:gem"].Location)
	}
}

func TestGetHandlerGetAllRoomObjectsRejectsNoMatches(t *testing.T) {
	world := newGetFakeWorld()
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"모든없는"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "여기에는 아무것도 없습니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	if len(world.moves) != 0 {
		t.Fatalf("moves = %+v, want none", world.moves)
	}
}

func TestGetHandlerPicksUpMoneyAsGold(t *testing.T) {
	world := newGetFakeWorld()
	addGetFakeMoneyToRoom(world, "object:money", "100냥", 100)
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"100냥"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 100냥을 줍습니다.\n당신은 이제 150냥을 가지고 있습니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	if _, ok := world.objects["object:money"]; ok {
		t.Fatal("money object still exists")
	}
	creature := world.creatures["creature:alice"]
	if creature.Stats["gold"] != 150 || slices.Contains(creature.Inventory.ObjectIDs, model.ObjectInstanceID("object:money")) {
		t.Fatalf("creature = gold:%d inv:%+v", creature.Stats["gold"], creature.Inventory.ObjectIDs)
	}
}

func TestGetHandlerBroadcastsMoneyPickupLikeLegacy(t *testing.T) {
	world := newGetFakeWorld()
	addGetFakeMoneyToRoom(world, "object:money", "100냥", 100)
	handler := NewGetHandler(world)
	var broadcasts []string
	ctx := &Context{
		SessionID: "s1",
		ActorID:   "player:alice",
		Values: map[string]any{
			ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
				if roomID != "room:plaza" || excludeSessionID != "s1" {
					t.Fatalf("broadcast room/exclude = %q/%q, want room:plaza/s1", roomID, excludeSessionID)
				}
				broadcasts = append(broadcasts, text)
				return errors.New("session closed")
			}),
		},
	}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"100냥"}})
	if err != nil {
		t.Fatalf("handler() error = %v, want nil", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 100냥을 줍습니다.\n당신은 이제 150냥을 가지고 있습니다.\n" {
		t.Fatalf("status/output = %d/%q, want money pickup", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0] != "\nAlice가 100냥을 줍습니다." {
		t.Fatalf("broadcasts = %+v, want legacy money pickup", broadcasts)
	}
}

func TestGetHandlerCompletesQuestRoomPickup(t *testing.T) {
	world := newGetFakeWorld()
	addGetRoomObject(world, "object:quest", "prototype:quest", "성물", map[string]string{"questnum": "1"}, []string{"OEVENT"})
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"성물"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	output := ctx.OutputString()
	for _, want := range []string{
		"당신은 성물을 줍습니다.\n",
		"임무를 완수하였습니다.",
		"당신은 경험치 120을 받았습니다.\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, missing %q", output, want)
		}
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	creature := world.creatures["creature:alice"]
	if creature.Properties["quest_completed_1"] != "1" {
		t.Fatalf("creature properties = %+v, want quest_completed_1", creature.Properties)
	}
	if creature.Stats["experience"] != 120 {
		t.Fatalf("experience = %d, want 120", creature.Stats["experience"])
	}
	if creature.Properties["proficiency/sharp"] != "13" || creature.Properties["realm/4"] != "13" {
		t.Fatalf("proficiency/realm = %+v, want add_prof split", creature.Properties)
	}
	if world.objects["object:quest"].Properties["key[2]"] != "Alice" {
		t.Fatalf("quest owner key[2] = %q, want Alice", world.objects["object:quest"].Properties["key[2]"])
	}
	if world.objects["object:quest"].Location.CreatureID != "creature:alice" {
		t.Fatalf("quest object location = %+v, want inventory", world.objects["object:quest"].Location)
	}
}

func TestGetHandlerRejectsAlreadyCompletedQuestRoomPickup(t *testing.T) {
	world := newGetFakeWorld()
	creature := world.creatures["creature:alice"]
	creature.Properties = map[string]string{"quest_completed_1": "1"}
	world.creatures[creature.ID] = creature
	addGetRoomObject(world, "object:quest", "prototype:quest", "성물", map[string]string{"questnum": "1"}, nil)
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"성물"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 그것을 주울수 없습니다. 이미 당신은 임무를 완수하였습니다." {
		t.Fatalf("status/output = %d/%q, want already-completed refusal", status, ctx.OutputString())
	}
	if len(world.moves) != 0 {
		t.Fatalf("moves = %+v, want none", world.moves)
	}
	if world.objects["object:quest"].Location.RoomID != "room:plaza" {
		t.Fatalf("quest object location = %+v, want room", world.objects["object:quest"].Location)
	}
}

func TestGetHandlerBulkCompletesQuestRoomPickup(t *testing.T) {
	world := newGetFakeWorld()
	addGetRoomObject(world, "object:quest", "prototype:quest", "성물", map[string]string{"questnum": "1"}, nil)
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"모든성"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	output := ctx.OutputString()
	for _, want := range []string{
		"임무를 완수하였습니다!",
		"당신은 경험치 120 을 받았습니다.",
		"당신은 성물을 줍습니다.\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, missing %q", output, want)
		}
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	creature := world.creatures["creature:alice"]
	if creature.Properties["quest_completed_1"] != "1" || creature.Stats["experience"] != 120 {
		t.Fatalf("creature quest/exp = %+v/%d", creature.Properties, creature.Stats["experience"])
	}
}

func TestGetHandlerRoomPickupClearsHiddenAndTemporaryPermanentFlags(t *testing.T) {
	world := newGetFakeWorld()
	addGetRoomObject(world, "object:temp", "prototype:temp", "숨긴 부적",
		map[string]string{"OHIDDN": "1", "OTEMPP": "1", "OPERM2": "1"},
		[]string{"OHIDDN", "OTEMPP", "OPERM2"})
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"숨긴"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 숨긴 부적을 줍습니다.\n" {
		t.Fatalf("status/output = %d/%q, want get confirmation", status, ctx.OutputString())
	}
	object := world.objects["object:temp"]
	if object.Location.CreatureID != "creature:alice" {
		t.Fatalf("object location = %+v, want inventory", object.Location)
	}
	if hasAnyNormalizedFlag(object.Metadata.Tags, "OHIDDN", "hidden", "OTEMPP", "OPERM2") {
		t.Fatalf("object tags = %+v, want hidden/temp permanent cleared", object.Metadata.Tags)
	}
	for _, key := range []string{"OHIDDN", "OTEMPP", "OPERM2"} {
		if _, ok := object.Properties[key]; ok {
			t.Fatalf("object properties = %+v, want %s removed", object.Properties, key)
		}
	}
}

func TestGetHandlerRoomPickupConvertsPermanentFlag(t *testing.T) {
	world := newGetFakeWorld()
	addGetRoomObject(world, "object:perm", "prototype:perm", "영구검",
		map[string]string{"OPERMT": "1"},
		[]string{"OPERMT"})
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"영구"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 영구검을 줍습니다.\n" {
		t.Fatalf("status/output = %d/%q, want get confirmation", status, ctx.OutputString())
	}
	object := world.objects["object:perm"]
	if hasAnyNormalizedFlag(object.Metadata.Tags, "OPERMT", "permanent") {
		t.Fatalf("object tags = %+v, want room permanent cleared", object.Metadata.Tags)
	}
	if !hasAnyNormalizedFlag(object.Metadata.Tags, "OPERM2") || object.Properties["OPERM2"] != "1" {
		t.Fatalf("object tags/properties = %+v/%+v, want OPERM2 set", object.Metadata.Tags, object.Properties)
	}
	if _, ok := object.Properties["OPERMT"]; ok {
		t.Fatalf("object properties = %+v, want OPERMT removed", object.Properties)
	}
}

func TestGetHandlerBulkRoomPickupClearsPickupFlags(t *testing.T) {
	world := newGetFakeWorld()
	addGetRoomObject(world, "object:temp", "prototype:temp", "부적",
		map[string]string{"OTEMPP": "1", "OPERM2": "1"},
		[]string{"OTEMPP", "OPERM2"})
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"모든부"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "부적") {
		t.Fatalf("status/output = %d/%q, want bulk pickup", status, ctx.OutputString())
	}
	object := world.objects["object:temp"]
	if hasAnyNormalizedFlag(object.Metadata.Tags, "OTEMPP", "OPERM2") {
		t.Fatalf("object tags = %+v, want temp permanent cleared", object.Metadata.Tags)
	}
	if _, ok := object.Properties["OTEMPP"]; ok {
		t.Fatalf("object properties = %+v, want OTEMPP removed", object.Properties)
	}
}

func TestGetHandlerRoomGuardBlocksSingleImplicitAndBulkPickupBelowCaretaker(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "single room", args: []string{"검"}},
		{name: "implicit visible container", args: []string{"보석"}},
		{name: "bulk room", args: []string{"모두"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := newGetFakeWorld()
			addGetRoomGuard(world)
			handler := NewGetHandler(world)

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			want := "경비병이 당신이 어떤것을 줍는 것을 방해합니다."
			if status != StatusDefault || ctx.OutputString() != want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
			}
			if len(world.moves) != 0 {
				t.Fatalf("moves = %+v, want none", world.moves)
			}
			if world.objects["object:sword"].Location.RoomID != "room:plaza" {
				t.Fatalf("sword location = %+v, want room", world.objects["object:sword"].Location)
			}
			if world.objects["object:gem"].Location.ContainerID != "object:bag" {
				t.Fatalf("gem location = %+v, want still in bag", world.objects["object:gem"].Location)
			}
		})
	}
}

func TestGetHandlerRoomGuardAllowsCaretaker(t *testing.T) {
	world := newGetFakeWorld()
	addGetRoomGuard(world)
	creature := world.creatures["creature:alice"]
	creature.Stats["class"] = legacyClassCaretaker
	world.creatures[creature.ID] = creature
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"검"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 빛나는 검을 줍습니다.\n" {
		t.Fatalf("status/output = %d/%q, want get confirmation", status, ctx.OutputString())
	}
	if world.objects["object:sword"].Location.CreatureID != "creature:alice" {
		t.Fatalf("sword location = %+v, want inventory", world.objects["object:sword"].Location)
	}
}

func TestGetHandlerRoomGuardPrecedesBlindBlock(t *testing.T) {
	world := newGetFakeWorld()
	addGetRoomGuard(world)
	creature := world.creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "PBLIND")
	world.creatures[creature.ID] = creature
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"검"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	want := "경비병이 당신이 어떤것을 줍는 것을 방해합니다."
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want guard first", status, ctx.OutputString())
	}
	if len(world.moves) != 0 {
		t.Fatalf("moves = %+v, want none", world.moves)
	}
}

func TestGetHandlerBlindBlocksRoomPickupGate(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "single room", args: []string{"검"}},
		{name: "implicit visible container", args: []string{"보석"}},
		{name: "bulk room", args: []string{"모두"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := newGetFakeWorld()
			creature := world.creatures["creature:alice"]
			creature.Metadata.Tags = append(creature.Metadata.Tags, "PBLIND")
			world.creatures[creature.ID] = creature
			handler := NewGetHandler(world)

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != RenderGetBlindBlock() {
				t.Fatalf("status/output = %d/%q, want blind block", status, ctx.OutputString())
			}
			if len(world.moves) != 0 {
				t.Fatalf("moves = %+v, want none", world.moves)
			}
			if world.objects["object:sword"].Location.RoomID != "room:plaza" {
				t.Fatalf("sword location = %+v, want room", world.objects["object:sword"].Location)
			}
			if world.objects["object:gem"].Location.ContainerID != "object:bag" {
				t.Fatalf("gem location = %+v, want still in bag", world.objects["object:gem"].Location)
			}
		})
	}
}

func TestGetHandlerClearsHiddenBeforeRoomPickupFailure(t *testing.T) {
	world := newGetFakeWorld()
	creature := world.creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "hidden", "PHIDDN", "invisible")
	creature.Stats["PHIDDN"] = 1
	world.creatures[creature.ID] = creature
	player := world.players["player:alice"]
	player.Metadata.Tags = append(player.Metadata.Tags, "hidden", "phiddn", "invisible")
	world.players[player.ID] = player
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"없는"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그런건 여기 없어요." {
		t.Fatalf("status/output = %d/%q, want missing object after hidden clear", status, ctx.OutputString())
	}
	creature = world.creatures["creature:alice"]
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "hidden", "phiddn", "PHIDDN") || creature.Stats["PHIDDN"] != 0 {
		t.Fatalf("creature hidden state = tags:%+v stats:%+v, want cleared", creature.Metadata.Tags, creature.Stats)
	}
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "invisible") {
		t.Fatalf("creature tags = %+v, want invisible retained", creature.Metadata.Tags)
	}
	player = world.players["player:alice"]
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "phiddn", "PHIDDN") {
		t.Fatalf("player tags = %+v, want hidden cleared", player.Metadata.Tags)
	}
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "invisible") {
		t.Fatalf("player tags = %+v, want invisible retained", player.Metadata.Tags)
	}
}

func TestTakeHandlerMovesContainerObjectIntoLinkedCreatureInventory(t *testing.T) {
	world := newGetFakeWorld()
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "꺼내", Number: 5, Handler: "get"},
		}),
		Handlers: map[string]Handler{
			"get": NewGetHandler(world),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "가방 보석 꺼내")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	if got := ctx.OutputString(); got != "당신은 가방에서 보석을 꺼냅니다.\n" {
		t.Fatalf("output = %q, want contained object confirmation", got)
	}
	object := world.objects["object:gem"]
	if object.Location.CreatureID != "creature:alice" || object.Location.Slot != "inventory" ||
		!object.Location.ContainerID.IsZero() {
		t.Fatalf("object location = %+v, want creature inventory", object.Location)
	}
	container := world.objects["object:bag"]
	if slices.Contains(container.Contents.ObjectIDs, model.ObjectInstanceID("object:gem")) {
		t.Fatalf("container still references moved object: %+v", container.Contents.ObjectIDs)
	}
	creature := world.creatures["creature:alice"]
	if !slices.Contains(creature.Inventory.ObjectIDs, model.ObjectInstanceID("object:gem")) {
		t.Fatalf("creature inventory = %+v, want object:gem", creature.Inventory.ObjectIDs)
	}
	if moved := world.moves[0].id; moved != "object:gem" {
		t.Fatalf("moved object = %q, want object:gem", moved)
	}
	if container.Properties["shotsCurrent"] != "0" {
		t.Fatalf("container shotsCurrent = %q, want 0", container.Properties["shotsCurrent"])
	}
}

func TestTakeHandlerBroadcastsContainerPickupLikeLegacy(t *testing.T) {
	world := newGetFakeWorld()
	handler := NewTakeHandler(world)
	var broadcasts []string
	ctx := &Context{
		SessionID: "s1",
		ActorID:   "player:alice",
		Values: map[string]any{
			ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
				if roomID != "room:plaza" || excludeSessionID != "s1" {
					t.Fatalf("broadcast room/exclude = %q/%q, want room:plaza/s1", roomID, excludeSessionID)
				}
				broadcasts = append(broadcasts, text)
				return errors.New("session closed")
			}),
		},
	}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"가방", "보석"}})
	if err != nil {
		t.Fatalf("handler() error = %v, want nil", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 가방에서 보석을 꺼냅니다.\n" {
		t.Fatalf("status/output = %d/%q, want container get confirmation", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0] != "\nAlice가 가방에서 보석을 꺼냅니다." {
		t.Fatalf("broadcasts = %+v, want legacy container get", broadcasts)
	}
}

func TestTakeHandlerGetsAllContainerObjects(t *testing.T) {
	world := newGetFakeWorld()
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"가방", "모두"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 가방에서 보석을 꺼냅니다.\n" {
		t.Fatalf("output = %q, want bulk container get confirmation", got)
	}
	object := world.objects["object:gem"]
	if object.Location.CreatureID != "creature:alice" || object.Location.Slot != "inventory" {
		t.Fatalf("object location = %+v, want creature inventory", object.Location)
	}
	container := world.objects["object:bag"]
	if container.Properties["shotsCurrent"] != "0" {
		t.Fatalf("container shotsCurrent = %q, want 0", container.Properties["shotsCurrent"])
	}
}

func TestTakeHandlerGetsAllGroupsAdjacentDuplicateContainerObjects(t *testing.T) {
	world := newGetFakeWorld()
	addGetContainerObject(world, "object:bag", "object:gem-2", "prototype:gem", "보석", nil, nil)
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"가방", "모든보"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 가방에서 (x2) 보석을 꺼냅니다.\n" {
		t.Fatalf("status/output = %d/%q, want grouped container get", status, ctx.OutputString())
	}
	for _, objectID := range []model.ObjectInstanceID{"object:gem", "object:gem-2"} {
		if world.objects[objectID].Location.CreatureID != "creature:alice" {
			t.Fatalf("%s location = %+v, want inventory", objectID, world.objects[objectID].Location)
		}
	}
}

func TestTakeHandlerBulkContainerEmptyMessageMatchesLegacyNoNewline(t *testing.T) {
	world := newGetFakeWorld()
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"가방", "모든없는"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그 안에는 아무것도 없습니다." {
		t.Fatalf("status/output = %d/%q, want legacy empty container message", status, ctx.OutputString())
	}
	if world.objects["object:gem"].Location.ContainerID != "object:bag" {
		t.Fatalf("gem location = %+v, want still in bag", world.objects["object:gem"].Location)
	}
}

func TestTakeHandlerRejectsNonContainerWithLegacyNoNewline(t *testing.T) {
	world := newGetFakeWorld()
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"금", "보석"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그것은 담는 종류가 아닙니다." {
		t.Fatalf("status/output = %d/%q, want legacy non-container message", status, ctx.OutputString())
	}
}

func TestTakeHandlerPicksUpContainerMoneyAsGoldAndDecrementsCount(t *testing.T) {
	world := newGetFakeWorld()
	bag := world.objects["object:bag"]
	bag.Contents.ObjectIDs = append(bag.Contents.ObjectIDs, "object:money")
	bag.Properties["shotsCurrent"] = "2"
	world.objects[bag.ID] = bag
	world.objects["object:money"] = model.ObjectInstance{
		ID:                  "object:money",
		PrototypeID:         "prototype:money",
		DisplayNameOverride: "100냥",
		Location:            model.ObjectLocation{ContainerID: "object:bag"},
		Properties:          map[string]string{"kind": "money", "type": "10", "value": "100"},
	}
	world.prototypes["prototype:money"] = model.ObjectPrototype{ID: "prototype:money", Kind: model.ObjectKindMoney, DisplayName: "돈"}
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"가방", "100냥"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 가방에서 100냥을 꺼냅니다.\n당신은 이제 150냥을 가지고 있습니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	bag = world.objects["object:bag"]
	if bag.Properties["shotsCurrent"] != "1" || slices.Contains(bag.Contents.ObjectIDs, model.ObjectInstanceID("object:money")) {
		t.Fatalf("bag = contents:%+v props:%+v", bag.Contents.ObjectIDs, bag.Properties)
	}
}

func TestTakeHandlerBulkContainerMoneyOnlyPrintsBalanceLikeLegacy(t *testing.T) {
	world := newGetFakeWorld()
	bag := world.objects["object:bag"]
	bag.Contents.ObjectIDs = append(bag.Contents.ObjectIDs, "object:money")
	bag.Properties["shotsCurrent"] = "2"
	world.objects[bag.ID] = bag
	world.objects["object:money"] = model.ObjectInstance{
		ID:                  "object:money",
		PrototypeID:         "prototype:money",
		DisplayNameOverride: "100냥",
		Location:            model.ObjectLocation{ContainerID: "object:bag"},
		Properties:          map[string]string{"kind": "money", "type": "10", "value": "100"},
	}
	world.prototypes["prototype:money"] = model.ObjectPrototype{ID: "prototype:money", Kind: model.ObjectKindMoney, DisplayName: "돈"}
	handler := NewGetHandler(world)
	var broadcasts []string
	ctx := &Context{
		SessionID: "s1",
		ActorID:   "player:alice",
		Values: map[string]any{
			ContextRoomBroadcastKey: RoomBroadcastFunc(func(_ model.RoomID, _ string, text string) error {
				broadcasts = append(broadcasts, text)
				return errors.New("session closed")
			}),
		},
	}

	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"가방", "모든100냥"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n당신은 이제 150냥을 가지고 있습니다.\n" {
		t.Fatalf("status/output = %d/%q, want legacy money-only bulk container get", status, ctx.OutputString())
	}
	if len(broadcasts) != 0 {
		t.Fatalf("broadcasts = %+v, want none for money-only bulk container get", broadcasts)
	}
	if _, ok := world.objects["object:money"]; ok {
		t.Fatal("money object still exists")
	}
	bag = world.objects["object:bag"]
	if bag.Properties["shotsCurrent"] != "1" || !slices.Contains(bag.Contents.ObjectIDs, model.ObjectInstanceID("object:gem")) {
		t.Fatalf("bag = contents:%+v props:%+v, want gem retained and count decremented", bag.Contents.ObjectIDs, bag.Properties)
	}
}

func TestTakeHandlerBulkContainerMoneyDoesNotEnterObjectListLikeLegacy(t *testing.T) {
	world := newGetFakeWorld()
	bag := world.objects["object:bag"]
	bag.Contents.ObjectIDs = append(bag.Contents.ObjectIDs, "object:money")
	bag.Properties["shotsCurrent"] = "2"
	world.objects[bag.ID] = bag
	world.objects["object:money"] = model.ObjectInstance{
		ID:                  "object:money",
		PrototypeID:         "prototype:money",
		DisplayNameOverride: "100냥",
		Location:            model.ObjectLocation{ContainerID: "object:bag"},
		Properties:          map[string]string{"kind": "money", "type": "10", "value": "100"},
	}
	world.prototypes["prototype:money"] = model.ObjectPrototype{ID: "prototype:money", Kind: model.ObjectKindMoney, DisplayName: "돈"}
	handler := NewGetHandler(world)
	var broadcasts []string
	ctx := &Context{
		SessionID: "s1",
		ActorID:   "player:alice",
		Values: map[string]any{
			ContextRoomBroadcastKey: RoomBroadcastFunc(func(_ model.RoomID, _ string, text string) error {
				broadcasts = append(broadcasts, text)
				return errors.New("session closed")
			}),
		},
	}

	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"가방", "모두"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	want := "\n당신은 이제 150냥을 가지고 있습니다.\n당신은 가방에서 보석을 꺼냅니다.\n"
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
	if len(broadcasts) != 1 || broadcasts[0] != "\nAlice가 가방에서 보석을 꺼냅니다." {
		t.Fatalf("broadcasts = %+v, want only non-money container get broadcast", broadcasts)
	}
	if _, ok := world.objects["object:money"]; ok {
		t.Fatal("money object still exists")
	}
	if world.objects["object:gem"].Location.CreatureID != "creature:alice" {
		t.Fatalf("gem location = %+v, want inventory", world.objects["object:gem"].Location)
	}
}

func TestGetHandlerFallsBackToLegacyContainerShapeForOtherGetAliases(t *testing.T) {
	world := newGetFakeWorld()
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"가방", "보석"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 가방에서 보석을 꺼냅니다.\n" {
		t.Fatalf("output = %q, want fallback container get confirmation", got)
	}
	object := world.objects["object:gem"]
	if object.Location.CreatureID != "creature:alice" || object.Location.Slot != "inventory" {
		t.Fatalf("object location = %+v, want creature inventory", object.Location)
	}
}

func TestTakeHandlerExplicitContainerBypassesRoomGuard(t *testing.T) {
	world := newGetFakeWorld()
	addGetRoomGuard(world)
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"가방", "보석"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 가방에서 보석을 꺼냅니다.\n" {
		t.Fatalf("status/output = %d/%q, want container get confirmation", status, ctx.OutputString())
	}
	if world.objects["object:gem"].Location.CreatureID != "creature:alice" {
		t.Fatalf("gem location = %+v, want inventory", world.objects["object:gem"].Location)
	}
}

func TestTakeHandlerExplicitContainerBypassesBlindBlock(t *testing.T) {
	world := newGetFakeWorld()
	creature := world.creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "PBLIND")
	world.creatures[creature.ID] = creature
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"가방", "보석"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 가방에서 보석을 꺼냅니다.\n" {
		t.Fatalf("status/output = %d/%q, want container get confirmation", status, ctx.OutputString())
	}
	if world.objects["object:gem"].Location.CreatureID != "creature:alice" {
		t.Fatalf("gem location = %+v, want inventory", world.objects["object:gem"].Location)
	}
}

func TestTakeHandlerBulkContainerPickupClearsTemporaryPermanentFlags(t *testing.T) {
	world := newGetFakeWorld()
	addGetContainerObject(world, "object:bag", "object:temp-gem", "prototype:temp-gem", "붉은 보석",
		map[string]string{"OTEMPP": "1", "OPERM2": "1"},
		[]string{"OTEMPP", "OPERM2"})
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"가방", "모든붉은"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 가방에서 붉은 보석을 꺼냅니다.\n" {
		t.Fatalf("status/output = %d/%q, want bulk container get confirmation", status, ctx.OutputString())
	}
	object := world.objects["object:temp-gem"]
	if hasAnyNormalizedFlag(object.Metadata.Tags, "OTEMPP", "OPERM2") {
		t.Fatalf("object tags = %+v, want temp permanent cleared", object.Metadata.Tags)
	}
	if _, ok := object.Properties["OTEMPP"]; ok {
		t.Fatalf("object properties = %+v, want OTEMPP removed", object.Properties)
	}
}

func TestTakeHandlerSingleContainerPickupPreservesTemporaryPermanentFlags(t *testing.T) {
	world := newGetFakeWorld()
	addGetContainerObject(world, "object:bag", "object:temp-gem", "prototype:temp-gem", "붉은 보석",
		map[string]string{"OTEMPP": "1", "OPERM2": "1"},
		[]string{"OTEMPP", "OPERM2"})
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"가방", "붉은"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 가방에서 붉은 보석을 꺼냅니다.\n" {
		t.Fatalf("status/output = %d/%q, want single container get confirmation", status, ctx.OutputString())
	}
	object := world.objects["object:temp-gem"]
	if !hasAnyNormalizedFlag(object.Metadata.Tags, "OTEMPP", "OPERM2") ||
		object.Properties["OTEMPP"] != "1" || object.Properties["OPERM2"] != "1" {
		t.Fatalf("object tags/properties = %+v/%+v, want temp permanent preserved", object.Metadata.Tags, object.Properties)
	}
}

func TestTakeHandlerSingleContainerObjectRequiresDetectInvisible(t *testing.T) {
	world := newGetFakeWorld()
	addGetContainerObject(world, "object:bag", "object:invisible-gem", "prototype:invisible-gem", "은신 보석",
		nil,
		[]string{"OINVIS"})
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"가방", "은신"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그 안에 그런것은 없어요." {
		t.Fatalf("status/output = %d/%q, want invisible contained object hidden", status, ctx.OutputString())
	}
	object := world.objects["object:invisible-gem"]
	if object.Location.ContainerID != "object:bag" {
		t.Fatalf("object location = %+v, want still in bag", object.Location)
	}

	creature := world.creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "PDINVI")
	world.creatures[creature.ID] = creature

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"가방", "은신"},
	})
	if err != nil {
		t.Fatalf("handler() with PDINVI error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 가방에서 은신 보석을 꺼냅니다.\n" {
		t.Fatalf("PDINVI status/output = %d/%q, want contained object confirmation", status, ctx.OutputString())
	}
	object = world.objects["object:invisible-gem"]
	if object.Location.CreatureID != "creature:alice" || object.Location.Slot != "inventory" {
		t.Fatalf("object location = %+v, want creature inventory", object.Location)
	}
}

func TestTakeHandlerRoomContainerRequiresDetectInvisible(t *testing.T) {
	world := newGetFakeWorld()
	bag := world.objects["object:bag"]
	bag.Metadata.Tags = append(bag.Metadata.Tags, "OINVIS")
	world.objects[bag.ID] = bag
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"가방", "보석"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그런것은 보이지 않습니다." {
		t.Fatalf("status/output = %d/%q, want invisible room container hidden", status, ctx.OutputString())
	}
	if world.objects["object:gem"].Location.ContainerID != "object:bag" {
		t.Fatalf("gem location = %+v, want still in invisible bag", world.objects["object:gem"].Location)
	}

	creature := world.creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "PDINVI")
	world.creatures[creature.ID] = creature

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"가방", "보석"},
	})
	if err != nil {
		t.Fatalf("handler() with PDINVI error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 가방에서 보석을 꺼냅니다.\n" {
		t.Fatalf("PDINVI status/output = %d/%q, want contained object confirmation", status, ctx.OutputString())
	}
}

func TestTakeHandlerSingleContainerFindObjAllowsHiddenSceneNoTake(t *testing.T) {
	world := newGetFakeWorld()
	addGetContainerObject(world, "object:bag", "object:decor-gem", "prototype:decor-gem", "장식 보석",
		map[string]string{"ONOTAK": "1"},
		[]string{"OHIDDN", "OSCENE"})
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"가방", "장식"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 가방에서 장식 보석을 꺼냅니다.\n" {
		t.Fatalf("status/output = %d/%q, want C find_obj-style contained pickup", status, ctx.OutputString())
	}
	object := world.objects["object:decor-gem"]
	if object.Location.CreatureID != "creature:alice" || object.Location.Slot != "inventory" {
		t.Fatalf("object location = %+v, want creature inventory", object.Location)
	}
}

func TestGetHandlerFindsObjectInsideVisibleRoomContainerByItemName(t *testing.T) {
	world := newGetFakeWorld()
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"보석"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 가방에서 보석을 꺼냅니다.\n" {
		t.Fatalf("output = %q, want implicit container get confirmation", got)
	}
	object := world.objects["object:gem"]
	if object.Location.CreatureID != "creature:alice" || object.Location.Slot != "inventory" ||
		!object.Location.ContainerID.IsZero() {
		t.Fatalf("object location = %+v, want creature inventory", object.Location)
	}
}

func TestGetHandlerFindsObjectInsideVisibleInventoryContainerByItemName(t *testing.T) {
	world := newGetFakeWorld()
	bag := world.objects["object:bag"]
	bag.Location = model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"}
	world.objects[bag.ID] = bag
	room := world.rooms["room:plaza"]
	room.Objects.ObjectIDs = getFakeRemoveObjectID(room.Objects.ObjectIDs, bag.ID)
	world.rooms[room.ID] = room
	creature := world.creatures["creature:alice"]
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, bag.ID)
	world.creatures[creature.ID] = creature

	handler := NewGetHandler(world)
	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"보석"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 가방에서 보석을 꺼냅니다.\n" {
		t.Fatalf("output = %q, want implicit inventory-container get confirmation", got)
	}
	object := world.objects["object:gem"]
	if object.Location.CreatureID != "creature:alice" || object.Location.Slot != "inventory" {
		t.Fatalf("object location = %+v, want creature inventory", object.Location)
	}
}

func TestGetHandlerFindsObjectInsideEquippedContainerByItemName(t *testing.T) {
	world := newGetFakeWorld()
	bag := world.objects["object:bag"]
	bag.Location = model.ObjectLocation{CreatureID: "creature:alice", Slot: "held"}
	world.objects[bag.ID] = bag
	room := world.rooms["room:plaza"]
	room.Objects.ObjectIDs = getFakeRemoveObjectID(room.Objects.ObjectIDs, bag.ID)
	world.rooms[room.ID] = room
	creature := world.creatures["creature:alice"]
	creature.Equipment = map[string]model.ObjectInstanceID{"held": bag.ID}
	world.creatures[creature.ID] = creature

	handler := NewGetHandler(world)
	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"보석"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 가방에서 보석을 꺼냅니다.\n" {
		t.Fatalf("output = %q, want equipped-container get confirmation", got)
	}
	object := world.objects["object:gem"]
	if object.Location.CreatureID != "creature:alice" || object.Location.Slot != "inventory" {
		t.Fatalf("object location = %+v, want creature inventory", object.Location)
	}
}

func TestGetHandlerUsesJoinedArgsForRoomObjectNames(t *testing.T) {
	world := newGetFakeWorld()
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "주워", Number: 5, Handler: "get"},
		}),
		Handlers: map[string]Handler{
			"get": NewGetHandler(world),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "작은 돌 주워")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 작은 돌을 줍습니다.\n" {
		t.Fatalf("output = %q, want joined-name room pickup", got)
	}

	object := world.objects["object:small-stone"]
	if object.Location.CreatureID != "creature:alice" || object.Location.Slot != "inventory" {
		t.Fatalf("object location = %+v, want creature inventory", object.Location)
	}
}

func TestTakeHandlerDoesNotUseContainerThatIsNotVisibleInRoom(t *testing.T) {
	world := newGetFakeWorld()
	handler := NewTakeHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"상자", "보석"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "그런것은 보이지 않습니다." {
		t.Fatalf("output = %q, want missing visible container message", got)
	}
	if len(world.moves) != 0 {
		t.Fatalf("moves = %+v, want none", world.moves)
	}
	object := world.objects["object:hidden-gem"]
	if object.Location.ContainerID != "object:hidden-box" {
		t.Fatalf("hidden object location = %+v, want hidden container", object.Location)
	}
}

func TestTakeHandlerDoesNotUseFlagsContainerInvisibleContainerLikeLegacy(t *testing.T) {
	world := newGetFakeWorld()
	world.prototypes["prototype:flags-invisible-box"] = model.ObjectPrototype{
		ID:          "prototype:flags-invisible-box",
		Kind:        model.ObjectKindContainer,
		DisplayName: "은신 상자",
	}
	world.objects["object:flags-invisible-box"] = model.ObjectInstance{
		ID:          "object:flags-invisible-box",
		PrototypeID: "prototype:flags-invisible-box",
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
		Properties:  map[string]string{"flags": "invisible"},
		Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:flags-hidden-gem"}},
	}
	room := world.rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:flags-invisible-box")
	world.rooms[room.ID] = room
	addGetContainerObject(world, "object:flags-invisible-box", "object:flags-hidden-gem", "prototype:flags-hidden-gem", "보석", nil, nil)
	handler := NewTakeHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "꺼내", Number: 5, Handler: "get"},
		Args: []string{"은신", "보석"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "그런것은 보이지 않습니다." {
		t.Fatalf("output = %q, want missing visible container message", got)
	}
	if len(world.moves) != 0 {
		t.Fatalf("moves = %+v, want none", world.moves)
	}
	object := world.objects["object:flags-hidden-gem"]
	if object.Location.ContainerID != "object:flags-invisible-box" {
		t.Fatalf("hidden object location = %+v, want hidden container", object.Location)
	}
}

func TestGetHandlerMatchesPrototypeKeywordAsLegacyKeyFallback(t *testing.T) {
	world := newGetFakeWorld()
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"칼"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 빛나는 검을 줍습니다.\n" {
		t.Fatalf("output = %q, want key fallback-selected object", got)
	}
	object := world.objects["object:sword"]
	if object.Location.CreatureID != "creature:alice" {
		t.Fatalf("sword location = %+v, want inventory", object.Location)
	}
}

func TestGetHandlerMatchesUTF8ObjectPropertyKey(t *testing.T) {
	world := newGetFakeWorld()
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"엽"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 금화를 줍습니다.\n" {
		t.Fatalf("output = %q, want property-key-selected object", got)
	}
}

func TestGetHandlerUserFacingFailuresDoNotMove(t *testing.T) {
	world := newGetFakeWorld()
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "뭘 주우시게요?" {
		t.Fatalf("status/output = %d/%q, want missing target prompt", status, ctx.OutputString())
	}
	if len(world.moves) != 0 {
		t.Fatalf("moves = %+v, want none", world.moves)
	}

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{Args: []string{"없는"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그런건 여기 없어요." {
		t.Fatalf("status/output = %d/%q, want missing object message", status, ctx.OutputString())
	}
	if len(world.moves) != 0 {
		t.Fatalf("moves = %+v, want none", world.moves)
	}
}

func TestGetHandlerRequiresWorldActorPlayerAndCreature(t *testing.T) {
	handler := NewGetHandler(nil)
	_, err := handler(&Context{ActorID: "player:alice"}, ResolvedCommand{Args: []string{"검"}})
	if !errors.Is(err, ErrGetWorldRequired) {
		t.Fatalf("handler() error = %v, want ErrGetWorldRequired", err)
	}

	world := newGetFakeWorld()
	handler = NewGetHandler(world)
	_, err = handler(&Context{}, ResolvedCommand{Args: []string{"검"}})
	if !errors.Is(err, ErrGetActorRequired) {
		t.Fatalf("handler() error = %v, want ErrGetActorRequired", err)
	}

	_, err = handler(&Context{ActorID: "player:missing"}, ResolvedCommand{Args: []string{"검"}})
	if !errors.Is(err, ErrGetPlayerNotFound) {
		t.Fatalf("handler() error = %v, want ErrGetPlayerNotFound", err)
	}

	player := world.players["player:alice"]
	player.CreatureID = ""
	world.players[player.ID] = player
	_, err = handler(&Context{ActorID: "player:alice"}, ResolvedCommand{Args: []string{"검"}})
	if !errors.Is(err, ErrGetCreatureRequired) {
		t.Fatalf("handler() error = %v, want ErrGetCreatureRequired", err)
	}
}

func TestGetHandlerReturnsMoveObjectErrors(t *testing.T) {
	world := newGetFakeWorld()
	world.moveErr = errors.New("move failed")
	handler := NewGetHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	_, err := handler(ctx, ResolvedCommand{Args: []string{"검"}})
	if !errors.Is(err, world.moveErr) {
		t.Fatalf("handler() error = %v, want move error", err)
	}
	if got := ctx.OutputString(); got != "" {
		t.Fatalf("output = %q, want no confirmation after move failure", got)
	}
}

type getFakeWorld struct {
	rooms      map[model.RoomID]model.Room
	players    map[model.PlayerID]model.Player
	creatures  map[model.CreatureID]model.Creature
	objects    map[model.ObjectInstanceID]model.ObjectInstance
	prototypes map[model.PrototypeID]model.ObjectPrototype
	moveErr    error
	moves      []getFakeMove
	cooldowns  map[model.CreatureID]map[string]int64
}

func (f *getFakeWorld) SavePlayer(model.PlayerID) error   { return nil }
func (f *getFakeWorld) FlushActivePlayersAndBanks() error { return nil }

type getFakeMove struct {
	id       model.ObjectInstanceID
	location model.ObjectLocation
}

func newGetFakeWorld() *getFakeWorld {
	return &getFakeWorld{
		cooldowns: make(map[model.CreatureID]map[string]int64),
		rooms: map[model.RoomID]model.Room{
			"room:plaza": {
				ID:          "room:plaza",
				DisplayName: "광장",
				Objects: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
					"object:sword",
					"object:coin",
					"object:ring",
					"object:bag",
					"object:small-stone",
					"object:elsewhere",
				}},
			},
			"room:other": {
				ID:          "room:other",
				DisplayName: "다른 방",
				Objects: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
					"object:hidden-box",
				}},
			},
		},
		players: map[model.PlayerID]model.Player{
			"player:alice": {
				ID:          "player:alice",
				DisplayName: "Alice",
				CreatureID:  "creature:alice",
				RoomID:      "room:plaza",
			},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:          "creature:alice",
				Kind:        model.CreatureKindPlayer,
				DisplayName: "Alice",
				PlayerID:    "player:alice",
				RoomID:      "room:plaza",
				Stats:       map[string]int{"gold": 50},
			},
		},
		objects: map[model.ObjectInstanceID]model.ObjectInstance{
			"object:sword": {
				ID:          "object:sword",
				PrototypeID: "prototype:sword",
				Location:    model.ObjectLocation{RoomID: "room:plaza"},
			},
			"object:coin": {
				ID:          "object:coin",
				PrototypeID: "prototype:coin",
				Location:    model.ObjectLocation{RoomID: "room:plaza"},
				Properties: map[string]string{
					"key[0]": "엽전",
				},
			},
			"object:ring": {
				ID:          "object:ring",
				PrototypeID: "prototype:ring",
				Location:    model.ObjectLocation{RoomID: "room:plaza"},
			},
			"object:bag": {
				ID:          "object:bag",
				PrototypeID: "prototype:bag",
				Location:    model.ObjectLocation{RoomID: "room:plaza"},
				Properties: map[string]string{
					"shotsCurrent": "1",
				},
				Contents: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
					"object:gem",
				}},
			},
			"object:small-stone": {
				ID:                  "object:small-stone",
				PrototypeID:         "prototype:stone",
				DisplayNameOverride: "작은 돌",
				Location:            model.ObjectLocation{RoomID: "room:plaza"},
			},
			"object:gem": {
				ID:          "object:gem",
				PrototypeID: "prototype:gem",
				Location:    model.ObjectLocation{ContainerID: "object:bag"},
			},
			"object:elsewhere": {
				ID:          "object:elsewhere",
				PrototypeID: "prototype:sword",
				Location:    model.ObjectLocation{RoomID: "room:other"},
			},
			"object:hidden-box": {
				ID:          "object:hidden-box",
				PrototypeID: "prototype:box",
				Location:    model.ObjectLocation{RoomID: "room:other"},
				Contents: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
					"object:hidden-gem",
				}},
			},
			"object:hidden-gem": {
				ID:          "object:hidden-gem",
				PrototypeID: "prototype:gem",
				Location:    model.ObjectLocation{ContainerID: "object:hidden-box"},
			},
		},
		prototypes: map[model.PrototypeID]model.ObjectPrototype{
			"prototype:sword": {
				ID:          "prototype:sword",
				DisplayName: "빛나는 검",
				Keywords:    []string{"검", "칼"},
				Properties:  map[string]string{"key[0]": "검"},
			},
			"prototype:coin": {
				ID:          "prototype:coin",
				DisplayName: "금화",
				Keywords:    []string{"동전"},
			},
			"prototype:ring": {
				ID:          "prototype:ring",
				DisplayName: "금반지",
				Keywords:    []string{"반지"},
			},
			"prototype:bag": {
				ID:          "prototype:bag",
				Kind:        model.ObjectKindContainer,
				DisplayName: "가방",
				Keywords:    []string{"주머니"},
			},
			"prototype:box": {
				ID:          "prototype:box",
				Kind:        model.ObjectKindContainer,
				DisplayName: "상자",
			},
			"prototype:gem": {
				ID:          "prototype:gem",
				DisplayName: "보석",
			},
			"prototype:stone": {
				ID:          "prototype:stone",
				DisplayName: "돌",
			},
			"prototype:money": {
				ID:          "prototype:money",
				Kind:        model.ObjectKindMoney,
				DisplayName: "돈",
			},
		},
	}
}

func (w *getFakeWorld) Room(id model.RoomID) (model.Room, bool) {
	room, ok := w.rooms[id]
	return room, ok
}

func (w *getFakeWorld) Player(id model.PlayerID) (model.Player, bool) {
	player, ok := w.players[id]
	return player, ok
}

func (w *getFakeWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	creature, ok := w.creatures[id]
	return creature, ok
}

func (w *getFakeWorld) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	object, ok := w.objects[id]
	return object, ok
}

func (w *getFakeWorld) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	proto, ok := w.prototypes[id]
	return proto, ok
}

func (w *getFakeWorld) MoveObject(id model.ObjectInstanceID, location model.ObjectLocation) error {
	w.moves = append(w.moves, getFakeMove{id: id, location: location})
	if w.moveErr != nil {
		return w.moveErr
	}

	object, ok := w.objects[id]
	if !ok {
		return fmt.Errorf("object %q not found", id)
	}

	if !object.Location.RoomID.IsZero() {
		room := w.rooms[object.Location.RoomID]
		room.Objects.ObjectIDs = getFakeRemoveObjectID(room.Objects.ObjectIDs, id)
		w.rooms[room.ID] = room
	}
	if !object.Location.CreatureID.IsZero() {
		creature := w.creatures[object.Location.CreatureID]
		creature.Inventory.ObjectIDs = getFakeRemoveObjectID(creature.Inventory.ObjectIDs, id)
		w.creatures[creature.ID] = creature
	}
	if !object.Location.ContainerID.IsZero() {
		container := w.objects[object.Location.ContainerID]
		container.Contents.ObjectIDs = getFakeRemoveObjectID(container.Contents.ObjectIDs, id)
		w.objects[container.ID] = container
	}

	object.Location = location
	w.objects[id] = object

	if !location.RoomID.IsZero() {
		room := w.rooms[location.RoomID]
		room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, id)
		w.rooms[room.ID] = room
	}
	if !location.CreatureID.IsZero() {
		creature := w.creatures[location.CreatureID]
		creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, id)
		w.creatures[creature.ID] = creature
	}
	if !location.ContainerID.IsZero() {
		container := w.objects[location.ContainerID]
		container.Contents.ObjectIDs = append(container.Contents.ObjectIDs, id)
		w.objects[container.ID] = container
	}

	return nil
}

func (w *getFakeWorld) TakeContainerObjectToCreatureInventory(id model.ObjectInstanceID, containerID model.ObjectInstanceID, creatureID model.CreatureID) (int, bool, error) {
	object, ok := w.objects[id]
	if !ok {
		return 0, false, fmt.Errorf("object %q not found", id)
	}
	if object.Location.ContainerID != containerID {
		return 0, false, nil
	}
	if err := w.MoveObject(id, model.ObjectLocation{CreatureID: creatureID, Slot: "inventory"}); err != nil {
		return 0, false, err
	}
	container := w.objects[containerID]
	current := 0
	if value := container.Properties["shotsCurrent"]; value == "1" {
		current = 1
	}
	if current > 0 {
		current--
	}
	if container.Properties == nil {
		container.Properties = map[string]string{}
	}
	container.Properties["shotsCurrent"] = fmt.Sprintf("%d", current)
	w.objects[containerID] = container
	return current, true, nil
}

func (w *getFakeWorld) PickupMoneyObjectToCreatureGold(id model.ObjectInstanceID, from model.ObjectLocation, creatureID model.CreatureID) (int, int, bool, error) {
	object, ok := w.objects[id]
	if !ok {
		return 0, 0, false, fmt.Errorf("object %q not found", id)
	}
	if object.Location != from || object.Properties["kind"] != "money" {
		creature := w.creatures[creatureID]
		return creature.Stats["gold"], 0, false, nil
	}
	amount := 0
	_, _ = fmt.Sscanf(object.Properties["value"], "%d", &amount)
	if amount < 1 {
		creature := w.creatures[creatureID]
		return creature.Stats["gold"], 0, false, nil
	}
	if !object.Location.RoomID.IsZero() {
		room := w.rooms[object.Location.RoomID]
		room.Objects.ObjectIDs = getFakeRemoveObjectID(room.Objects.ObjectIDs, id)
		w.rooms[room.ID] = room
	}
	if !object.Location.ContainerID.IsZero() {
		container := w.objects[object.Location.ContainerID]
		container.Contents.ObjectIDs = getFakeRemoveObjectID(container.Contents.ObjectIDs, id)
		w.objects[container.ID] = container
	}
	if !object.Location.ContainerID.IsZero() {
		container := w.objects[object.Location.ContainerID]
		current := 0
		_, _ = fmt.Sscanf(container.Properties["shotsCurrent"], "%d", &current)
		if current > 0 {
			current--
		}
		if container.Properties == nil {
			container.Properties = map[string]string{}
		}
		container.Properties["shotsCurrent"] = fmt.Sprintf("%d", current)
		w.objects[container.ID] = container
	}
	delete(w.objects, id)
	creature := w.creatures[creatureID]
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats["gold"] += amount
	w.creatures[creatureID] = creature
	return creature.Stats["gold"], amount, true, nil
}

func (w *getFakeWorld) SetCreatureProperty(id model.CreatureID, key string, value string) (model.Creature, error) {
	creature, ok := w.creatures[id]
	if !ok {
		return model.Creature{}, fmt.Errorf("creature %q not found", id)
	}
	if creature.Properties == nil {
		creature.Properties = map[string]string{}
	}
	creature.Properties[key] = value
	w.creatures[id] = creature
	return creature, nil
}

func (w *getFakeWorld) SetCreatureStat(id model.CreatureID, key string, value int) error {
	creature, ok := w.creatures[id]
	if !ok {
		return fmt.Errorf("creature %q not found", id)
	}
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats[key] = value
	w.creatures[id] = creature
	return nil
}

func (w *getFakeWorld) SetObjectProperty(id model.ObjectInstanceID, key string, value string) (model.ObjectInstance, error) {
	object, ok := w.objects[id]
	if !ok {
		return model.ObjectInstance{}, fmt.Errorf("object %q not found", id)
	}
	if value == "" {
		delete(object.Properties, key)
		w.objects[id] = object
		return object, nil
	}
	if object.Properties == nil {
		object.Properties = map[string]string{}
	}
	object.Properties[key] = value
	w.objects[id] = object
	return object, nil
}

func (w *getFakeWorld) UpdateObjectTags(id model.ObjectInstanceID, add []string, remove []string) (model.ObjectInstance, error) {
	object, ok := w.objects[id]
	if !ok {
		return model.ObjectInstance{}, fmt.Errorf("object %q not found", id)
	}
	object.Metadata.Tags = updateGetFakeTags(object.Metadata.Tags, add, remove)
	w.objects[id] = object
	return object, nil
}

func (w *getFakeWorld) UpdateCreatureTags(id model.CreatureID, add []string, remove []string) (model.Creature, error) {
	creature, ok := w.creatures[id]
	if !ok {
		return model.Creature{}, fmt.Errorf("creature %q not found", id)
	}
	creature.Metadata.Tags = updateGetFakeTags(creature.Metadata.Tags, add, remove)
	w.creatures[id] = creature
	return creature, nil
}

func (w *getFakeWorld) UpdatePlayerTags(id model.PlayerID, add []string, remove []string) (model.Player, error) {
	player, ok := w.players[id]
	if !ok {
		return model.Player{}, fmt.Errorf("player %q not found", id)
	}
	player.Metadata.Tags = updateGetFakeTags(player.Metadata.Tags, add, remove)
	w.players[id] = player
	return player, nil
}

func updateGetFakeTags(tags []string, add []string, remove []string) []string {
	if len(remove) > 0 {
		removeSet := normalizedFlagSet(remove...)
		kept := tags[:0]
		for _, tag := range tags {
			if _, ok := removeSet[normalizeFlagName(tag)]; !ok {
				kept = append(kept, tag)
			}
		}
		tags = kept
	}
	for _, tag := range add {
		if !hasAnyNormalizedFlag(tags, tag) {
			tags = append(tags, tag)
		}
	}
	return tags
}

func addGetFakeMoneyToRoom(world *getFakeWorld, id model.ObjectInstanceID, name string, amount int) {
	world.objects[id] = model.ObjectInstance{
		ID:                  id,
		PrototypeID:         "prototype:money",
		DisplayNameOverride: name,
		Location:            model.ObjectLocation{RoomID: "room:plaza"},
		Properties:          map[string]string{"kind": "money", "type": "10", "value": fmt.Sprintf("%d", amount)},
	}
	room := world.rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, id)
	world.rooms[room.ID] = room
}

func addGetRoomObject(world *getFakeWorld, id model.ObjectInstanceID, protoID model.PrototypeID, name string, properties map[string]string, tags []string) {
	world.prototypes[protoID] = model.ObjectPrototype{ID: protoID, DisplayName: name}
	world.objects[id] = model.ObjectInstance{
		ID:          id,
		PrototypeID: protoID,
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
		Properties:  properties,
		Metadata:    model.Metadata{Tags: tags},
	}
	room := world.rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, id)
	world.rooms[room.ID] = room
}

func addGetRoomGuard(world *getFakeWorld) {
	guard := model.Creature{
		ID:          "creature:guard",
		Kind:        model.CreatureKindMonster,
		DisplayName: "경비병",
		RoomID:      "room:plaza",
		Stats:       map[string]int{"hpCurrent": 10},
		Metadata:    model.Metadata{Tags: []string{"MGUARD"}},
	}
	world.creatures[guard.ID] = guard
	room := world.rooms["room:plaza"]
	room.CreatureIDs = append(room.CreatureIDs, guard.ID)
	world.rooms[room.ID] = room
}

func addGetContainerObject(world *getFakeWorld, containerID model.ObjectInstanceID, id model.ObjectInstanceID, protoID model.PrototypeID, name string, properties map[string]string, tags []string) {
	world.prototypes[protoID] = model.ObjectPrototype{ID: protoID, DisplayName: name}
	world.objects[id] = model.ObjectInstance{
		ID:          id,
		PrototypeID: protoID,
		Location:    model.ObjectLocation{ContainerID: containerID},
		Properties:  properties,
		Metadata:    model.Metadata{Tags: tags},
	}
	container := world.objects[containerID]
	container.Contents.ObjectIDs = append(container.Contents.ObjectIDs, id)
	world.objects[containerID] = container
}

func getFakeRemoveObjectID(ids []model.ObjectInstanceID, id model.ObjectInstanceID) []model.ObjectInstanceID {
	kept := ids[:0]
	for _, existing := range ids {
		if existing != id {
			kept = append(kept, existing)
		}
	}
	return kept
}

func (w *getFakeWorld) SetCreatureCooldown(id model.CreatureID, key string, timeVal int64, interval int64) error {
	if w.cooldowns == nil {
		w.cooldowns = make(map[model.CreatureID]map[string]int64)
	}
	if w.cooldowns[id] == nil {
		w.cooldowns[id] = make(map[string]int64)
	}
	w.cooldowns[id][key] = timeVal + interval
	return nil
}

func TestGetCorpseLooting(t *testing.T) {
	world := newGetFakeWorld()

	world.players["player:bob"] = model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:plaza",
	}
	world.creatures["creature:bob"] = model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:plaza",
	}

	world.objects["object:alice-corpse"] = model.ObjectInstance{
		ID:                  "object:alice-corpse",
		PrototypeID:         "prototype:corpse",
		DisplayNameOverride: "Alice의 시체",
		Location:            model.ObjectLocation{RoomID: "room:plaza"},
		Contents:            model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:alice-item"}},
	}
	world.objects["object:alice-item"] = model.ObjectInstance{
		ID:                  "object:alice-item",
		PrototypeID:         "prototype:sword",
		DisplayNameOverride: "Alice의 명검",
		Location:            model.ObjectLocation{ContainerID: "object:alice-corpse"},
	}

	world.objects["object:bob-corpse"] = model.ObjectInstance{
		ID:                  "object:bob-corpse",
		PrototypeID:         "prototype:corpse",
		DisplayNameOverride: "Bob의 시체",
		Location:            model.ObjectLocation{RoomID: "room:plaza"},
		Contents:            model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:bob-item"}},
	}
	world.objects["object:bob-item"] = model.ObjectInstance{
		ID:                  "object:bob-item",
		PrototypeID:         "prototype:sword",
		DisplayNameOverride: "Bob의 명검",
		Location:            model.ObjectLocation{ContainerID: "object:bob-corpse"},
	}

	room := world.rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:alice-corpse", "object:bob-corpse")
	world.rooms["room:plaza"] = room

	handler := NewGetHandler(world)

	t.Run("loot own corpse", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"Alice의 시체", "Alice의 명검"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Fatalf("status = %d, want default", status)
		}
		if !strings.Contains(ctx.OutputString(), "Alice의 명검") {
			t.Fatalf("output = %q, want success confirmation", ctx.OutputString())
		}
	})

	t.Run("loot another player's corpse with policy block (default)", func(t *testing.T) {
		t.Setenv("MUHAN_CORPSE_LOOT_POLICY", "block")
		ctx := &Context{ActorID: "player:alice"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"Bob의 시체", "Bob의 명검"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Fatalf("status = %d, want default", status)
		}
		if !strings.Contains(ctx.OutputString(), "다른 사람의 시체는 만질 수 없습니다.") {
			t.Fatalf("output = %q, want block message", ctx.OutputString())
		}
	})

	t.Run("loot another player's corpse with policy penalty", func(t *testing.T) {
		t.Setenv("MUHAN_CORPSE_LOOT_POLICY", "penalty")
		world.cooldowns = make(map[model.CreatureID]map[string]int64)

		ctx := &Context{ActorID: "player:alice"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"Bob의 시체", "Bob의 명검"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Fatalf("status = %d, want default", status)
		}
		if !strings.Contains(ctx.OutputString(), "다른 사람의 시체에 손을 대어 범죄자(PK) 대기 시간이 설정되었습니다!") {
			t.Fatalf("output = %q, want penalty message", ctx.OutputString())
		}
		cooldowns := world.cooldowns["creature:alice"]
		if cooldowns == nil || cooldowns["plykl"] == 0 {
			t.Fatalf("plykl cooldown not set: %+v", world.cooldowns)
		}
	})

	t.Run("loot another player's corpse with policy allow", func(t *testing.T) {
		t.Setenv("MUHAN_CORPSE_LOOT_POLICY", "allow")
		world.cooldowns = make(map[model.CreatureID]map[string]int64)

		world.objects["object:bob-item"] = model.ObjectInstance{
			ID:                  "object:bob-item",
			PrototypeID:         "prototype:sword",
			DisplayNameOverride: "Bob의 명검",
			Location:            model.ObjectLocation{ContainerID: "object:bob-corpse"},
		}

		ctx := &Context{ActorID: "player:alice"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"Bob의 시체", "Bob의 명검"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Fatalf("status = %d, want default", status)
		}
		if strings.Contains(ctx.OutputString(), "범죄자") || strings.Contains(ctx.OutputString(), "만질 수 없습니다") {
			t.Fatalf("output = %q, want clean success", ctx.OutputString())
		}
		cooldowns := world.cooldowns["creature:alice"]
		if cooldowns != nil && cooldowns["plykl"] != 0 {
			t.Fatalf("plykl cooldown should not be set: %+v", world.cooldowns)
		}
	})
}
