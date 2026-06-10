package command

import (
	"errors"
	"fmt"
	"testing"

	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

func TestDropHandlerDispatchesLegacyDropAndMovesInventoryObject(t *testing.T) {
	world := newFakeDropWorld()
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "버려", Number: 7, Handler: "drop"},
		}),
		Handlers: map[string]Handler{
			"drop": NewDropHandler(world),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "검 버려")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 검은 돌을 버렸습니다.\n" {
		t.Fatalf("output = %q, want drop confirmation", got)
	}

	assertDropMovedObject(t, world, "object:dark-stone", "room:plaza")
	if containsDropObjectID(world.creatures["creature:alice"].Inventory.ObjectIDs, "object:dark-stone") {
		t.Fatalf("inventory still contains dropped object: %+v", world.creatures["creature:alice"].Inventory.ObjectIDs)
	}
}

func TestDropHandlerBroadcastsSingleRoomDropLikeLegacy(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)
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

	status, err := handler(ctx, ResolvedCommand{Args: []string{"검"}})
	if err != nil {
		t.Fatalf("handler() error = %v, want nil", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 검은 돌을 버렸습니다.\n" {
		t.Fatalf("status/output = %d/%q, want drop confirmation", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 {
		t.Fatalf("broadcasts = %+v, want one room broadcast", broadcasts)
	}
	if got := broadcasts[0]; got.roomID != "room:plaza" || got.excludeSession != "s1" || got.text != "\nAlice가 검은 돌을 버렸습니다." {
		t.Fatalf("broadcast = %+v, want legacy room drop", got)
	}
	assertDropMovedObject(t, world, "object:dark-stone", "room:plaza")
}

func TestDropHandlerRejectsObjectIDTargetLikeLegacyFindObj(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"object:sword"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 그런것을 갖고 있지 않습니다." {
		t.Fatalf("status/output = %d/%q, want missing object ID target", status, ctx.OutputString())
	}
	if world.objects["object:sword"].Location.CreatureID != "creature:alice" {
		t.Fatalf("sword location = %+v, want still in inventory", world.objects["object:sword"].Location)
	}
}

func TestDropHandlerUsesLegacyPrefixOrderInsteadOfExactFirst(t *testing.T) {
	world := newFakeDropWorld()
	proto := world.prototypes["prototype:sword"]
	proto.DisplayName = "검 조각"
	world.prototypes[proto.ID] = proto
	world.prototypes["prototype:sword-exact"] = model.ObjectPrototype{
		ID:          "prototype:sword-exact",
		DisplayName: "검",
	}
	world.objects["object:sword-exact"] = model.ObjectInstance{
		ID:          "object:sword-exact",
		PrototypeID: "prototype:sword-exact",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	}
	creature := world.creatures["creature:alice"]
	creature.Inventory.ObjectIDs = []model.ObjectInstanceID{"object:sword", "object:sword-exact", "object:potion", "object:apple", "object:bag"}
	world.creatures[creature.ID] = creature
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"검"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 검 조각을 버렸습니다.\n" {
		t.Fatalf("status/output = %d/%q, want first prefix match", status, ctx.OutputString())
	}
	assertDropMovedObject(t, world, "object:sword", "room:plaza")
	if world.objects["object:sword-exact"].Location.CreatureID != "creature:alice" {
		t.Fatalf("exact sword location = %+v, want retained in inventory", world.objects["object:sword-exact"].Location)
	}
}

func TestDropHandlerUsesPrefixFromFirstArg(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Args:   []string{"치"},
		Values: []int64{1},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 치유 물약을 버렸습니다.\n" {
		t.Fatalf("output = %q, want prefix drop confirmation", got)
	}

	assertDropMovedObject(t, world, "object:potion", "room:plaza")
}

func TestDropHandlerDropsMoneyToRoom(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"1,000냥"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 1냥을 버렸습니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	if got := world.creatures["creature:alice"].Stats["gold"]; got != 4999 {
		t.Fatalf("gold = %d, want 4999", got)
	}
	room := world.rooms["room:plaza"]
	if len(room.Objects.ObjectIDs) != 2 {
		t.Fatalf("room objects = %+v, want chest and money", room.Objects.ObjectIDs)
	}
	money := world.objects[room.Objects.ObjectIDs[1]]
	if money.DisplayNameOverride != "1냥" || money.Properties["kind"] != "money" || money.Properties["type"] != "10" || money.Properties["value"] != "1" {
		t.Fatalf("money object = %+v", money)
	}
	assertDropSavedPlayer(t, world, "player:alice")
}

func TestDropHandlerBroadcastsMoneyDropLikeLegacy(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)
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

	status, err := handler(ctx, ResolvedCommand{Args: []string{"12abc냥"}})
	if err != nil {
		t.Fatalf("handler() error = %v, want nil", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 12냥을 버렸습니다.\n" {
		t.Fatalf("status/output = %d/%q, want money drop", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0] != "\nAlice가 12냥을 버렸습니다." {
		t.Fatalf("broadcasts = %+v, want legacy money drop", broadcasts)
	}
}

func TestDropHandlerMoneyUsesLegacyNumericPrefix(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"12abc냥"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 12냥을 버렸습니다.\n" {
		t.Fatalf("status/output = %d/%q, want numeric-prefix money drop", status, ctx.OutputString())
	}
	if got := world.creatures["creature:alice"].Stats["gold"]; got != 4988 {
		t.Fatalf("gold = %d, want 4988", got)
	}
	room := world.rooms["room:plaza"]
	money := world.objects[room.Objects.ObjectIDs[1]]
	if money.DisplayNameOverride != "12냥" || money.Properties["value"] != "12" {
		t.Fatalf("money object = %+v, want 12 money", money)
	}
}

func TestDropHandlerTreatsBareCurrencySuffixAsObjectLikeLegacy(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"냥"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 그런것을 갖고 있지 않습니다." {
		t.Fatalf("status/output = %d/%q, want object lookup failure", status, ctx.OutputString())
	}
	if got := world.creatures["creature:alice"].Stats["gold"]; got != 5000 {
		t.Fatalf("gold = %d, want unchanged", got)
	}
	room := world.rooms["room:plaza"]
	if len(room.Objects.ObjectIDs) != 1 {
		t.Fatalf("room objects = %+v, want no money drop", room.Objects.ObjectIDs)
	}
}

func TestDropHandlerRejectsInvalidMoneyDrop(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"0냥"}})
	if err != nil {
		t.Fatalf("handler() zero money error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "돈의 단위는 음수가 될수 없습니다." {
		t.Fatalf("zero status/output = %d/%q", status, ctx.OutputString())
	}

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{Args: []string{"abc냥"}})
	if err != nil {
		t.Fatalf("handler() nonnumeric money error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "돈의 단위는 음수가 될수 없습니다." {
		t.Fatalf("nonnumeric status/output = %d/%q", status, ctx.OutputString())
	}

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{Args: []string{"-5냥"}})
	if err != nil {
		t.Fatalf("handler() negative money error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "돈의 단위는 음수가 될수 없습니다." {
		t.Fatalf("negative status/output = %d/%q", status, ctx.OutputString())
	}

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{Args: []string{"9,000냥"}})
	if err != nil {
		t.Fatalf("handler() insufficient money error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 9냥을 버렸습니다.\n" {
		t.Fatalf("comma status/output = %d/%q, want C atol prefix drop", status, ctx.OutputString())
	}
	if got := world.creatures["creature:alice"].Stats["gold"]; got != 4991 {
		t.Fatalf("gold = %d, want 4991 after comma-prefix drop", got)
	}

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{Args: []string{"9000냥"}})
	if err != nil {
		t.Fatalf("handler() insufficient money error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 그만큼의 돈을 가지고 있지 않습니다." {
		t.Fatalf("insufficient status/output = %d/%q", status, ctx.OutputString())
	}
	if got := world.creatures["creature:alice"].Stats["gold"]; got != 4991 {
		t.Fatalf("gold = %d, want unchanged 4991 after insufficient drop", got)
	}
}

func TestDropHandlerClearsHiddenBeforeDropFailure(t *testing.T) {
	world := newFakeDropWorld()
	creature := world.creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "hidden", "PHIDDN", "invisible")
	creature.Stats["PHIDDN"] = 1
	world.creatures[creature.ID] = creature
	player := world.players["player:alice"]
	player.Metadata.Tags = append(player.Metadata.Tags, "hidden", "phiddn", "invisible")
	world.players[player.ID] = player
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"없는"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 그런것을 갖고 있지 않습니다." {
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

func TestDropHandlerDoesNotClearHiddenWithoutTarget(t *testing.T) {
	world := newFakeDropWorld()
	creature := world.creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "hidden")
	creature.Stats["PHIDDN"] = 1
	world.creatures[creature.ID] = creature
	player := world.players["player:alice"]
	player.Metadata.Tags = append(player.Metadata.Tags, "hidden")
	world.players[player.ID] = player
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "무엇을 버리실려구요?" {
		t.Fatalf("status/output = %d/%q, want missing target prompt", status, ctx.OutputString())
	}
	if !hasAnyNormalizedFlag(world.creatures["creature:alice"].Metadata.Tags, "hidden") ||
		world.creatures["creature:alice"].Stats["PHIDDN"] != 1 ||
		!hasAnyNormalizedFlag(world.players["player:alice"].Metadata.Tags, "hidden") {
		t.Fatalf("hidden state changed: creature %+v/%+v player %+v",
			world.creatures["creature:alice"].Metadata.Tags,
			world.creatures["creature:alice"].Stats,
			world.players["player:alice"].Metadata.Tags)
	}
}

func TestDropHandlerRejectsSingleRoomDropProtectedQuestEventObjects(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*fakeDropWorld)
		args  []string
		want  string
	}{
		{
			name: "quest object",
			setup: func(world *fakeDropWorld) {
				addDropInventoryObject(world, "object:quest", "prototype:quest", "성물", map[string]string{"questnum": "1"}, nil)
			},
			args: []string{"성물"},
			want: "임무 아이템은 버리지 못합니다.",
		},
		{
			name: "event object",
			setup: func(world *fakeDropWorld) {
				addDropInventoryObject(world, "object:event", "prototype:event", "기념패", nil, []string{"OEVENT"})
			},
			args: []string{"기념패"},
			want: "이벤트 아이템은 버리지 못합니다.",
		},
		{
			name: "quest child",
			setup: func(world *fakeDropWorld) {
				addDropContainerChild(world, "object:bag", "object:quest-child", "prototype:quest-child", "성물", map[string]string{"questnum": "1"}, nil)
			},
			args: []string{"가방"},
			want: "임무 아이템이 들어있으면 버리지 못합니다.",
		},
		{
			name: "event child",
			setup: func(world *fakeDropWorld) {
				addDropContainerChild(world, "object:bag", "object:event-child", "prototype:event-child", "기념패", nil, []string{"OEVENT"})
			},
			args: []string{"가방"},
			want: "이벤트 아이템이 들어있으면 버리지 못합니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := newFakeDropWorld()
			tt.setup(world)
			handler := NewDropHandler(world)

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			if len(world.moves) != 0 {
				t.Fatalf("moves = %+v, want none", world.moves)
			}
		})
	}
}

func TestDropHandlerAllowsDMToDropQuestObject(t *testing.T) {
	world := newFakeDropWorld()
	creature := world.creatures["creature:alice"]
	creature.Stats["class"] = model.ClassDM
	world.creatures[creature.ID] = creature
	addDropInventoryObject(world, "object:quest", "prototype:quest", "성물", map[string]string{"questnum": "1"}, nil)
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"성물"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 성물을 버렸습니다.\n" {
		t.Fatalf("status/output = %d/%q, want DM drop confirmation", status, ctx.OutputString())
	}
	assertDropMovedObject(t, world, "object:quest", "room:plaza")
}

func TestDropHandlerDumpRoomConsumesSingleDropAndRewardsGoldExperience(t *testing.T) {
	world := newFakeDropWorld()
	room := world.rooms["room:plaza"]
	room.Metadata.Tags = append(room.Metadata.Tags, "RDUMPR")
	world.rooms[room.ID] = room
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"검"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	want := "당신은 검은 돌을 버렸습니다.\n당신의 물건을 제물로 바쳤습니다.\n당신은 약간의 상금과 경험을 받았습니다."
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want dump reward", status, ctx.OutputString())
	}
	if _, ok := world.objects["object:dark-stone"]; ok {
		t.Fatal("dumped dark-stone still exists")
	}
	if containsDropObjectID(world.rooms["room:plaza"].Objects.ObjectIDs, "object:dark-stone") {
		t.Fatalf("room objects = %+v, want no dark-stone", world.rooms["room:plaza"].Objects.ObjectIDs)
	}
	creature := world.creatures["creature:alice"]
	if creature.Stats["gold"] != 5010 || creature.Stats["experience"] != 2 {
		t.Fatalf("creature stats = %+v, want gold +10 and exp +2", creature.Stats)
	}
	if len(world.moves) != 0 {
		t.Fatalf("moves = %+v, want none for dump room", world.moves)
	}
}

func TestDropHandlerDispatchesVerbFinalPutIntoInventoryContainer(t *testing.T) {
	world := newFakeDropWorld()
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "넣어", Number: 7, Handler: "drop"},
		}),
		Handlers: map[string]Handler{
			"drop": NewDropHandler(world),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "사과 가방 넣어")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 사과를 가방 안에 넣습니다.\n" {
		t.Fatalf("output = %q, want put confirmation", got)
	}

	assertDropMovedObjectToContainer(t, world, "object:apple", "object:bag")
	if !containsDropObjectID(world.creatures["creature:alice"].Inventory.ObjectIDs, "object:bag") {
		t.Fatalf("inventory = %+v, want bag retained", world.creatures["creature:alice"].Inventory.ObjectIDs)
	}
	if got := world.objects["object:bag"].Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("bag shotsCurrent = %q, want 1", got)
	}
}

func TestDropHandlerTreatsPropertyBackedLegacyContainerFlagAsContainer(t *testing.T) {
	world := newFakeDropWorld()
	proto := world.prototypes["prototype:bag"]
	proto.Kind = ""
	proto.Properties = map[string]string{"flags": "OCONTN"}
	world.prototypes[proto.ID] = proto

	handler := NewDropHandler(world)
	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"사과", "가방"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 사과를 가방 안에 넣습니다.\n" {
		t.Fatalf("status/output = %d/%q, want property-backed OCONTN put success", status, ctx.OutputString())
	}
	assertDropMovedObjectToContainer(t, world, "object:apple", "object:bag")
}

func TestDropHandlerDevouringContainerConsumesObject(t *testing.T) {
	world := newFakeDropWorld()
	bag := world.objects["object:bag"]
	bag.Metadata.Tags = append(bag.Metadata.Tags, "containerDevours")
	world.objects[bag.ID] = bag
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"사과", "가방"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "사과를 가방이 삼켜 버려 흔적도 없이 사라집니다!\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	if _, ok := world.objects["object:apple"]; ok {
		t.Fatal("devoured apple still exists")
	}
	if containsDropObjectID(world.creatures["creature:alice"].Inventory.ObjectIDs, "object:apple") {
		t.Fatalf("inventory still contains devoured object: %+v", world.creatures["creature:alice"].Inventory.ObjectIDs)
	}
	if len(world.objects["object:bag"].Contents.ObjectIDs) != 0 {
		t.Fatalf("bag contents = %+v, want empty", world.objects["object:bag"].Contents.ObjectIDs)
	}
	if got := world.objects["object:bag"].Properties["shotsCurrent"]; got != "0" {
		t.Fatalf("bag shotsCurrent = %q, want unchanged 0", got)
	}
	if len(world.moves) != 0 {
		t.Fatalf("moves = %+v, want no move for devoured object", world.moves)
	}
}

func TestDropHandlerRejectsFullDevouringContainerForSinglePut(t *testing.T) {
	world := newFakeDropWorld()
	bag := world.objects["object:bag"]
	bag.Metadata.Tags = append(bag.Metadata.Tags, "containerDevours")
	bag.Properties["shotsCurrent"] = "2"
	bag.Properties["shotsMax"] = "2"
	world.objects[bag.ID] = bag
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"사과", "가방"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "가방안에 더이상 넣을 수 없습니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	if _, ok := world.objects["object:apple"]; !ok {
		t.Fatal("apple was destroyed even though full devouring container rejected single put")
	}
	if !containsDropObjectID(world.creatures["creature:alice"].Inventory.ObjectIDs, "object:apple") {
		t.Fatalf("inventory = %+v, want apple retained", world.creatures["creature:alice"].Inventory.ObjectIDs)
	}
}

func TestDropHandlerRejectsFullContainerPut(t *testing.T) {
	world := newFakeDropWorld()
	bag := world.objects["object:bag"]
	bag.Properties["shotsCurrent"] = "2"
	bag.Properties["shotsMax"] = "2"
	world.objects[bag.ID] = bag
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"사과", "가방"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "가방안에 더이상 넣을 수 없습니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	if containsDropObjectID(world.objects["object:bag"].Contents.ObjectIDs, "object:apple") {
		t.Fatalf("bag contents = %+v, want apple retained in inventory", world.objects["object:bag"].Contents.ObjectIDs)
	}
}

func TestDropHandlerRejectsSelfPutWithLegacyNoNewline(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"가방", "가방"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그것을 그것 자신한테는 넣을수 없습니다." {
		t.Fatalf("status/output = %d/%q, want legacy self-put rejection", status, ctx.OutputString())
	}
	if containsDropObjectID(world.objects["object:bag"].Contents.ObjectIDs, "object:bag") {
		t.Fatalf("bag contents = %+v, want no self nesting", world.objects["object:bag"].Contents.ObjectIDs)
	}
}

func TestDropHandlerRejectsNonContainerPutTargetWithLegacyNoNewline(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"사과", "검"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그것은 담을수 있는것이 아닙니다." {
		t.Fatalf("status/output = %d/%q, want legacy non-container rejection", status, ctx.OutputString())
	}
	if world.objects["object:apple"].Location.CreatureID != "creature:alice" {
		t.Fatalf("apple location = %+v, want retained in inventory", world.objects["object:apple"].Location)
	}
}

func TestDropHandlerFullContainerPrecedesContainerNestingRejection(t *testing.T) {
	tests := []struct {
		name     string
		resolved ResolvedCommand
	}{
		{
			name: "explicit put",
			resolved: ResolvedCommand{
				Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
				Args: []string{"가방", "상자"},
			},
		},
		{
			name: "fallback put shape",
			resolved: ResolvedCommand{
				Args: []string{"가방", "상자"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := newFakeDropWorld()
			chest := world.objects["object:chest"]
			chest.Properties["shotsCurrent"] = "2"
			chest.Properties["shotsMax"] = "2"
			world.objects[chest.ID] = chest
			handler := NewDropHandler(world)

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, tt.resolved)
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != "상자안에 더이상 넣을 수 없습니다.\n" {
				t.Fatalf("status/output = %d/%q, want full-container rejection", status, ctx.OutputString())
			}
			if len(world.moves) != 0 {
				t.Fatalf("moves = %+v, want none", world.moves)
			}
			if !containsDropObjectID(world.creatures["creature:alice"].Inventory.ObjectIDs, "object:bag") {
				t.Fatalf("inventory = %+v, want bag retained", world.creatures["creature:alice"].Inventory.ObjectIDs)
			}
			if containsDropObjectID(world.objects["object:chest"].Contents.ObjectIDs, "object:bag") {
				t.Fatalf("chest contents = %+v, want no bag", world.objects["object:chest"].Contents.ObjectIDs)
			}
		})
	}
}

func TestDropHandlerDropsAllMatchingObjectsToRoom(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"모든검"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 검은 돌, 검을 버렸습니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	assertDropObjectInRoom(t, world, "object:dark-stone", "room:plaza")
	assertDropObjectInRoom(t, world, "object:sword", "room:plaza")
	assertDropSavedPlayer(t, world, "player:alice")
}

func TestDropHandlerBroadcastsBulkRoomDropLikeLegacy(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)
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

	status, err := handler(ctx, ResolvedCommand{Args: []string{"모든검"}})
	if err != nil {
		t.Fatalf("handler() error = %v, want nil", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 검은 돌, 검을 버렸습니다.\n" {
		t.Fatalf("status/output = %d/%q, want bulk drop confirmation", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0] != "\nAlice가 검은 돌, 검을 버렸습니다." {
		t.Fatalf("broadcasts = %+v, want legacy bulk room drop", broadcasts)
	}
}

func TestDropHandlerDropsAllGroupsAdjacentDuplicateObjects(t *testing.T) {
	world := newFakeDropWorld()
	addDropInventoryObject(world, "object:apple-2", "prototype:apple", "사과", nil, nil)
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"모든사"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 (x2) 사과를 버렸습니다.\n" {
		t.Fatalf("status/output = %d/%q, want grouped duplicate drop", status, ctx.OutputString())
	}
	assertDropObjectInRoom(t, world, "object:apple", "room:plaza")
	assertDropObjectInRoom(t, world, "object:apple-2", "room:plaza")
}

func TestDropHandlerBulkDumpRoomConsumesObjectsAndRewardsGoldOnly(t *testing.T) {
	world := newFakeDropWorld()
	room := world.rooms["room:plaza"]
	room.Metadata.Tags = append(room.Metadata.Tags, "RDUMPR")
	world.rooms[room.ID] = room
	addDropInventoryObject(world, "object:apple-2", "prototype:apple", "사과", nil, nil)
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"모든사"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	want := "당신은 (x2) 사과를 버렸습니다.\n당신의 물건을 제물로 바쳤습니다.\n당신은 약간의 상금을 받았습니다."
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want bulk dump reward", status, ctx.OutputString())
	}
	for _, objectID := range []model.ObjectInstanceID{"object:apple", "object:apple-2"} {
		if _, ok := world.objects[objectID]; ok {
			t.Fatalf("%s still exists after dump room bulk drop", objectID)
		}
		if containsDropObjectID(world.rooms["room:plaza"].Objects.ObjectIDs, objectID) {
			t.Fatalf("room objects = %+v, want no %s", world.rooms["room:plaza"].Objects.ObjectIDs, objectID)
		}
	}
	creature := world.creatures["creature:alice"]
	if creature.Stats["gold"] != 5020 || creature.Stats["experience"] != 0 {
		t.Fatalf("creature stats = %+v, want gold +20 and no bulk exp reward", creature.Stats)
	}
	if len(world.moves) != 0 {
		t.Fatalf("moves = %+v, want none for dump room", world.moves)
	}
	assertDropSavedPlayer(t, world, "player:alice")
}

func TestDropHandlerBulkSkipsQuestObjectsBelowDM(t *testing.T) {
	world := newFakeDropWorld()
	creature := world.creatures["creature:alice"]
	creature.Stats["class"] = model.ClassInvincible
	world.creatures[creature.ID] = creature
	addDropInventoryObject(world, "object:quest", "prototype:quest", "성물", map[string]string{"questnum": "1"}, nil)
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"모든성"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 아무것도 가지고 있지 않습니다." {
		t.Fatalf("status/output = %d/%q, want no movable quest object", status, ctx.OutputString())
	}
	if len(world.moves) != 0 {
		t.Fatalf("moves = %+v, want none", world.moves)
	}
	if !containsDropObjectID(world.creatures["creature:alice"].Inventory.ObjectIDs, "object:quest") {
		t.Fatalf("inventory = %+v, want quest object retained", world.creatures["creature:alice"].Inventory.ObjectIDs)
	}
	if len(world.saves) != 0 {
		t.Fatalf("saves = %+v, want none for skip-only bulk drop", world.saves)
	}
}

func TestDropHandlerBulkAllowsDMQuestObjects(t *testing.T) {
	world := newFakeDropWorld()
	creature := world.creatures["creature:alice"]
	creature.Stats["class"] = model.ClassDM
	world.creatures[creature.ID] = creature
	addDropInventoryObject(world, "object:quest", "prototype:quest", "성물", map[string]string{"questnum": "1"}, nil)
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"모든성"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 성물을 버렸습니다.\n" {
		t.Fatalf("status/output = %d/%q, want DM quest bulk drop", status, ctx.OutputString())
	}
	assertDropObjectInRoom(t, world, "object:quest", "room:plaza")
}

func TestDropHandlerBulkDropsInvisibleObjectsOnlyForDetector(t *testing.T) {
	tests := []struct {
		name         string
		creatureTags []string
		wantOutput   string
		wantMoved    bool
	}{
		{
			name:       "no detect invisible",
			wantOutput: "당신은 아무것도 가지고 있지 않습니다.",
		},
		{
			name:         "detect invisible",
			creatureTags: []string{"PDINVI"},
			wantOutput:   "당신은 은신검을 버렸습니다.\n",
			wantMoved:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := newFakeDropWorld()
			creature := world.creatures["creature:alice"]
			creature.Metadata.Tags = append(creature.Metadata.Tags, tt.creatureTags...)
			world.creatures[creature.ID] = creature
			addDropInventoryObject(world, "object:invisible", "prototype:invisible", "은신검", nil, []string{"OINVIS"})
			handler := NewDropHandler(world)

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{Args: []string{"모든은신"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.wantOutput {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.wantOutput)
			}
			if tt.wantMoved {
				assertDropObjectInRoom(t, world, "object:invisible", "room:plaza")
			} else if len(world.moves) != 0 {
				t.Fatalf("moves = %+v, want none", world.moves)
			}
		})
	}
}

func TestDropHandlerSingleDropRequiresDetectInvisible(t *testing.T) {
	world := newFakeDropWorld()
	addDropInventoryObject(world, "object:invisible", "prototype:invisible", "은신검", nil, []string{"OINVIS"})
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"은신"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 그런것을 갖고 있지 않습니다." {
		t.Fatalf("status/output = %d/%q, want invisible object hidden", status, ctx.OutputString())
	}
	if world.objects["object:invisible"].Location.CreatureID != "creature:alice" {
		t.Fatalf("object location = %+v, want still in inventory", world.objects["object:invisible"].Location)
	}

	creature := world.creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "PDINVI")
	world.creatures[creature.ID] = creature

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{Args: []string{"은신"}})
	if err != nil {
		t.Fatalf("handler() with PDINVI error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 은신검을 버렸습니다.\n" {
		t.Fatalf("PDINVI status/output = %d/%q, want drop confirmation", status, ctx.OutputString())
	}
	assertDropObjectInRoom(t, world, "object:invisible", "room:plaza")
}

func TestDropHandlerPutsAllMatchingObjectsIntoContainer(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"모든사", "가방"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 사과를 가방 안에 넣습니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	assertDropObjectInContainer(t, world, "object:apple", "object:bag")
	if got := world.objects["object:bag"].Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("bag shotsCurrent = %q, want 1", got)
	}
}

func TestDropHandlerBroadcastsContainerPutLikeLegacy(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)
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

	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"모든사", "가방"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v, want nil", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 사과를 가방 안에 넣습니다.\n" {
		t.Fatalf("status/output = %d/%q, want put confirmation", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0] != "\nAlice가 사과를 가방 안에 넣습니다." {
		t.Fatalf("broadcasts = %+v, want legacy container put", broadcasts)
	}
}

func TestDropHandlerSinglePutObjectRequiresDetectInvisible(t *testing.T) {
	world := newFakeDropWorld()
	apple := world.objects["object:apple"]
	apple.Metadata.Tags = append(apple.Metadata.Tags, "OINVIS")
	world.objects[apple.ID] = apple
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"사과", "가방"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 그런것을 갖고 있지 않습니다." {
		t.Fatalf("status/output = %d/%q, want invisible put object hidden", status, ctx.OutputString())
	}
	if world.objects["object:apple"].Location.CreatureID != "creature:alice" {
		t.Fatalf("apple location = %+v, want still in inventory", world.objects["object:apple"].Location)
	}

	creature := world.creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "PDINVI")
	world.creatures[creature.ID] = creature

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"사과", "가방"},
	})
	if err != nil {
		t.Fatalf("handler() with PDINVI error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 사과를 가방 안에 넣습니다.\n" {
		t.Fatalf("PDINVI status/output = %d/%q, want put confirmation", status, ctx.OutputString())
	}
	assertDropObjectInContainer(t, world, "object:apple", "object:bag")
}

func TestDropHandlerPutsAllGroupsAdjacentDuplicateObjects(t *testing.T) {
	world := newFakeDropWorld()
	addDropInventoryObject(world, "object:apple-2", "prototype:apple", "사과", nil, nil)
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"모든사", "가방"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 (x2) 사과를 가방 안에 넣습니다.\n" {
		t.Fatalf("status/output = %d/%q, want grouped duplicate put", status, ctx.OutputString())
	}
	assertDropObjectInContainer(t, world, "object:apple", "object:bag")
	assertDropObjectInContainer(t, world, "object:apple-2", "object:bag")
	if got := world.objects["object:bag"].Properties["shotsCurrent"]; got != "2" {
		t.Fatalf("bag shotsCurrent = %q, want 2", got)
	}
}

func TestDropHandlerBulkPutPartialFullMessageMatchesLegacyNoNewline(t *testing.T) {
	world := newFakeDropWorld()
	addDropInventoryObject(world, "object:apple-2", "prototype:apple", "사과", nil, nil)
	bag := world.objects["object:bag"]
	bag.Properties["shotsCurrent"] = "0"
	bag.Properties["shotsMax"] = "1"
	world.objects[bag.ID] = bag
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"모든사", "가방"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	want := "가방안에 더이상 물건을 넣을 수 없습니다.당신은 사과를 가방 안에 넣습니다.\n"
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want partial full legacy output", status, ctx.OutputString())
	}
	if got := world.objects["object:bag"].Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("bag shotsCurrent = %q, want 1", got)
	}
}

func TestDropHandlerPutsAllMatchingQuestObjectsOnlyForDM(t *testing.T) {
	tests := []struct {
		name       string
		class      int
		wantOutput string
		wantMoved  bool
	}{
		{
			name:       "invincible still below DM",
			class:      model.ClassInvincible,
			wantOutput: "당신은 그것 안에 넣을 물건을 아무것도 갖고 있지 않습니다.",
		},
		{
			name:       "DM",
			class:      model.ClassDM,
			wantOutput: "당신은 성물을 가방 안에 넣습니다.\n",
			wantMoved:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := newFakeDropWorld()
			creature := world.creatures["creature:alice"]
			creature.Stats["class"] = tt.class
			world.creatures[creature.ID] = creature
			addDropInventoryObject(world, "object:quest", "prototype:quest", "성물", map[string]string{"questnum": "1"}, nil)
			handler := NewDropHandler(world)

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{
				Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
				Args: []string{"모든성", "가방"},
			})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.wantOutput {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.wantOutput)
			}
			if tt.wantMoved {
				assertDropObjectInContainer(t, world, "object:quest", "object:bag")
			} else if containsDropObjectID(world.objects["object:bag"].Contents.ObjectIDs, "object:quest") {
				t.Fatalf("bag contents = %+v, want quest object skipped", world.objects["object:bag"].Contents.ObjectIDs)
			}
		})
	}
}

func TestDropHandlerBulkDevouringContainerConsumesObjectsButUsesLegacyNoItemOutput(t *testing.T) {
	world := newFakeDropWorld()
	bag := world.objects["object:bag"]
	bag.Metadata.Tags = append(bag.Metadata.Tags, "containerDevours")
	bag.Properties["shotsCurrent"] = "2"
	bag.Properties["shotsMax"] = "2"
	world.objects[bag.ID] = bag
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"모든사", "가방"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 그것 안에 넣을 물건을 아무것도 갖고 있지 않습니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	if _, ok := world.objects["object:apple"]; ok {
		t.Fatal("devoured apple still exists")
	}
	if len(world.objects["object:bag"].Contents.ObjectIDs) != 0 {
		t.Fatalf("bag contents = %+v, want empty", world.objects["object:bag"].Contents.ObjectIDs)
	}
	if got := world.objects["object:bag"].Properties["shotsCurrent"]; got != "2" {
		t.Fatalf("bag shotsCurrent = %q, want unchanged 2", got)
	}
}

func TestDropHandlerPutsIntoVisibleRoomContainer(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"사과", "상"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 사과를 상자 안에 넣습니다.\n" {
		t.Fatalf("output = %q, want room-container put confirmation", got)
	}

	assertDropMovedObjectToContainer(t, world, "object:apple", "object:chest")
	if !containsDropObjectID(world.rooms["room:plaza"].Objects.ObjectIDs, "object:chest") {
		t.Fatalf("room objects = %+v, want chest retained", world.rooms["room:plaza"].Objects.ObjectIDs)
	}
}

func TestDropHandlerRoomContainerRequiresDetectInvisible(t *testing.T) {
	world := newFakeDropWorld()
	chest := world.objects["object:chest"]
	chest.Metadata.Tags = append(chest.Metadata.Tags, "OINVIS")
	world.objects[chest.ID] = chest
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"사과", "상자"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그런 물건은 없습니다." {
		t.Fatalf("status/output = %d/%q, want invisible room container hidden", status, ctx.OutputString())
	}
	if world.objects["object:apple"].Location.CreatureID != "creature:alice" {
		t.Fatalf("apple location = %+v, want still in inventory", world.objects["object:apple"].Location)
	}

	creature := world.creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "PDINVI")
	world.creatures[creature.ID] = creature

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"사과", "상자"},
	})
	if err != nil {
		t.Fatalf("handler() with PDINVI error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 사과를 상자 안에 넣습니다.\n" {
		t.Fatalf("PDINVI status/output = %d/%q, want room-container put confirmation", status, ctx.OutputString())
	}
	assertDropObjectInContainer(t, world, "object:apple", "object:chest")
}

func TestDropHandlerPutsIntoEquippedContainer(t *testing.T) {
	world := newFakeDropWorld()
	bag := world.objects["object:bag"]
	bag.Metadata.Tags = append(bag.Metadata.Tags, "OINVIS")
	bag.Location = model.ObjectLocation{CreatureID: "creature:alice", Slot: "held"}
	world.objects[bag.ID] = bag
	creature := world.creatures["creature:alice"]
	creature.Inventory.ObjectIDs = removeDropObjectID(creature.Inventory.ObjectIDs, bag.ID)
	creature.Equipment = map[string]model.ObjectInstanceID{"held": bag.ID}
	world.creatures[creature.ID] = creature

	handler := NewDropHandler(world)
	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"사과", "가방"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 사과를 가방 안에 넣습니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	assertDropMovedObjectToContainer(t, world, "object:apple", "object:bag")
	if world.creatures["creature:alice"].Equipment["held"] != "object:bag" {
		t.Fatalf("equipment = %+v, want bag retained as held", world.creatures["creature:alice"].Equipment)
	}
}

func TestDropHandlerFallsBackToLegacyPutShapeForOtherDropAliases(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"사과", "가방"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 사과를 가방 안에 넣습니다.\n" {
		t.Fatalf("output = %q, want fallback put confirmation", got)
	}

	assertDropMovedObjectToContainer(t, world, "object:apple", "object:bag")
}

func TestDropHandlerMatchesUTF8ObjectPropertyKey(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"흑"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 검은 돌을 버렸습니다.\n" {
		t.Fatalf("output = %q, want property-key drop confirmation", got)
	}

	assertDropMovedObject(t, world, "object:dark-stone", "room:plaza")
}

func TestDropHandlerUsesJoinedArgsForInventoryObjectNames(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"작은", "돌"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "당신은 작은 돌을 버렸습니다.\n" {
		t.Fatalf("output = %q, want joined-name drop confirmation", got)
	}

	assertDropMovedObject(t, world, "object:small-stone", "room:plaza")
}

func TestDropHandlerUserFacingFailuresDoNotError(t *testing.T) {
	world := newFakeDropWorld()
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() missing target error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "무엇을 버리실려구요?" {
		t.Fatalf("status/output = %d/%q, want missing target message", status, ctx.OutputString())
	}
	if len(world.moves) != 0 {
		t.Fatalf("moves = %+v, want none", world.moves)
	}

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{Args: []string{"없는"}})
	if err != nil {
		t.Fatalf("handler() missing object error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 그런것을 갖고 있지 않습니다." {
		t.Fatalf("status/output = %d/%q, want missing object message", status, ctx.OutputString())
	}
	if len(world.moves) != 0 {
		t.Fatalf("moves = %+v, want none", world.moves)
	}

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "넣어", Number: 7, Handler: "drop"},
		Args: []string{"검", "없는"},
	})
	if err != nil {
		t.Fatalf("handler() missing container error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그런 물건은 없습니다." {
		t.Fatalf("status/output = %d/%q, want missing container message", status, ctx.OutputString())
	}
	if len(world.moves) != 0 {
		t.Fatalf("moves = %+v, want none", world.moves)
	}
}

func TestDropHandlerPropagatesMoveError(t *testing.T) {
	world := newFakeDropWorld()
	moveErr := errors.New("move failed")
	world.moveErr = moveErr
	handler := NewDropHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	_, err := handler(ctx, ResolvedCommand{Args: []string{"검"}})
	if !errors.Is(err, moveErr) {
		t.Fatalf("handler() error = %v, want move error", err)
	}
	if got := ctx.OutputString(); got != "" {
		t.Fatalf("output = %q, want none on movement error", got)
	}
	if len(world.moves) != 1 || world.moves[0].objectID != "object:dark-stone" {
		t.Fatalf("moves = %+v, want attempted dark-stone move", world.moves)
	}
	if !containsDropObjectID(world.creatures["creature:alice"].Inventory.ObjectIDs, "object:dark-stone") {
		t.Fatalf("inventory = %+v, want dark-stone retained after failed move", world.creatures["creature:alice"].Inventory.ObjectIDs)
	}
}

func TestDropHandlerRequiresWorldActorAndCreature(t *testing.T) {
	handler := NewDropHandler(nil)
	_, err := handler(&Context{ActorID: "player:alice"}, ResolvedCommand{Args: []string{"검"}})
	if !errors.Is(err, ErrDropWorldRequired) {
		t.Fatalf("handler() error = %v, want ErrDropWorldRequired", err)
	}

	world := newFakeDropWorld()
	handler = NewDropHandler(world)
	_, err = handler(&Context{}, ResolvedCommand{Args: []string{"검"}})
	if !errors.Is(err, ErrDropActorRequired) {
		t.Fatalf("handler() error = %v, want ErrDropActorRequired", err)
	}

	player := world.players["player:alice"]
	player.CreatureID = ""
	world.players[player.ID] = player
	_, err = handler(&Context{ActorID: "player:alice"}, ResolvedCommand{Args: []string{"검"}})
	if !errors.Is(err, ErrDropCreatureRequired) {
		t.Fatalf("handler() error = %v, want ErrDropCreatureRequired", err)
	}
}

type dropMove struct {
	objectID model.ObjectInstanceID
	location model.ObjectLocation
}

type fakeDropWorld struct {
	rooms      map[model.RoomID]model.Room
	players    map[model.PlayerID]model.Player
	creatures  map[model.CreatureID]model.Creature
	objects    map[model.ObjectInstanceID]model.ObjectInstance
	prototypes map[model.PrototypeID]model.ObjectPrototype
	moveErr    error
	moves      []dropMove
	saves      []model.PlayerID
}

func (f *fakeDropWorld) SavePlayer(playerID model.PlayerID) error {
	f.saves = append(f.saves, playerID)
	return nil
}
func (f *fakeDropWorld) FlushActivePlayersAndBanks() error { return nil }

func newFakeDropWorld() *fakeDropWorld {
	return &fakeDropWorld{
		rooms: map[model.RoomID]model.Room{
			"room:plaza": {
				ID:          "room:plaza",
				DisplayName: "광장",
				Objects: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
					"object:chest",
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
				Stats:       map[string]int{"gold": 5000},
				Inventory: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
					"object:dark-stone",
					"object:sword",
					"object:potion",
					"object:apple",
					"object:bag",
					"object:small-stone",
				}},
			},
		},
		objects: map[model.ObjectInstanceID]model.ObjectInstance{
			"object:dark-stone": {
				ID:          "object:dark-stone",
				PrototypeID: "prototype:dark-stone",
				Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
				Properties: map[string]string{
					"key[0]": "흑석",
				},
			},
			"object:sword": {
				ID:          "object:sword",
				PrototypeID: "prototype:sword",
				Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
			},
			"object:potion": {
				ID:          "object:potion",
				PrototypeID: "prototype:potion",
				Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
			},
			"object:apple": {
				ID:          "object:apple",
				PrototypeID: "prototype:apple",
				Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
			},
			"object:bag": {
				ID:          "object:bag",
				PrototypeID: "prototype:bag",
				Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
				Properties: map[string]string{
					"shotsCurrent": "0",
					"shotsMax":     "2",
				},
			},
			"object:small-stone": {
				ID:                  "object:small-stone",
				PrototypeID:         "prototype:stone",
				DisplayNameOverride: "작은 돌",
				Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
			},
			"object:chest": {
				ID:          "object:chest",
				PrototypeID: "prototype:chest",
				Location:    model.ObjectLocation{RoomID: "room:plaza"},
				Properties: map[string]string{
					"shotsCurrent": "0",
					"shotsMax":     "2",
				},
			},
		},
		prototypes: map[model.PrototypeID]model.ObjectPrototype{
			"prototype:dark-stone": {ID: "prototype:dark-stone", DisplayName: "검은 돌"},
			"prototype:sword":      {ID: "prototype:sword", DisplayName: "검"},
			"prototype:potion":     {ID: "prototype:potion", DisplayName: "치유 물약"},
			"prototype:apple":      {ID: "prototype:apple", DisplayName: "사과"},
			"prototype:bag":        {ID: "prototype:bag", Kind: model.ObjectKindContainer, DisplayName: "가방"},
			"prototype:chest":      {ID: "prototype:chest", Kind: model.ObjectKindContainer, DisplayName: "상자"},
			"prototype:stone":      {ID: "prototype:stone", DisplayName: "돌"},
		},
	}
}

func addDropInventoryObject(world *fakeDropWorld, objectID model.ObjectInstanceID, protoID model.PrototypeID, name string, properties map[string]string, tags []string) {
	world.prototypes[protoID] = model.ObjectPrototype{ID: protoID, DisplayName: name}
	world.objects[objectID] = model.ObjectInstance{
		ID:          objectID,
		PrototypeID: protoID,
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  properties,
		Metadata:    model.Metadata{Tags: tags},
	}
	creature := world.creatures["creature:alice"]
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, objectID)
	world.creatures[creature.ID] = creature
}

func addDropContainerChild(world *fakeDropWorld, containerID model.ObjectInstanceID, objectID model.ObjectInstanceID, protoID model.PrototypeID, name string, properties map[string]string, tags []string) {
	world.prototypes[protoID] = model.ObjectPrototype{ID: protoID, DisplayName: name}
	world.objects[objectID] = model.ObjectInstance{
		ID:          objectID,
		PrototypeID: protoID,
		Location:    model.ObjectLocation{ContainerID: containerID},
		Properties:  properties,
		Metadata:    model.Metadata{Tags: tags},
	}
	container := world.objects[containerID]
	container.Contents.ObjectIDs = append(container.Contents.ObjectIDs, objectID)
	world.objects[containerID] = container
}

func (w *fakeDropWorld) Room(id model.RoomID) (model.Room, bool) {
	room, ok := w.rooms[id]
	return room, ok
}

func (w *fakeDropWorld) Player(id model.PlayerID) (model.Player, bool) {
	player, ok := w.players[id]
	return player, ok
}

func (w *fakeDropWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	creature, ok := w.creatures[id]
	return creature, ok
}

func (w *fakeDropWorld) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	object, ok := w.objects[id]
	return object, ok
}

func (w *fakeDropWorld) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	proto, ok := w.prototypes[id]
	return proto, ok
}

func (w *fakeDropWorld) MoveObject(id model.ObjectInstanceID, location model.ObjectLocation) error {
	w.moves = append(w.moves, dropMove{objectID: id, location: location})
	if w.moveErr != nil {
		return w.moveErr
	}

	object, ok := w.objects[id]
	if !ok {
		return fmt.Errorf("object %q not found", id)
	}

	w.removeObjectFromLocation(id, object.Location)
	object.Location = location
	w.objects[id] = object
	w.addObjectToLocation(id, location)
	return nil
}

func (w *fakeDropWorld) DestroyObject(id model.ObjectInstanceID) error {
	object, ok := w.objects[id]
	if !ok {
		return fmt.Errorf("object %q not found", id)
	}
	for _, childID := range append([]model.ObjectInstanceID(nil), object.Contents.ObjectIDs...) {
		if err := w.DestroyObject(childID); err != nil {
			return err
		}
	}
	object = w.objects[id]
	w.removeObjectFromLocation(id, object.Location)
	delete(w.objects, id)
	return nil
}

func (w *fakeDropWorld) StoreCreatureInventoryObjectInContainer(objectID model.ObjectInstanceID, creatureID model.CreatureID, containerID model.ObjectInstanceID, maxCount int) (int, bool, bool, error) {
	object, ok := w.objects[objectID]
	if !ok {
		return 0, false, false, fmt.Errorf("object %q not found", objectID)
	}
	if object.Location.CreatureID != creatureID {
		return 0, false, false, nil
	}
	container := w.objects[containerID]
	current := dropFakeObjectInt(container.Properties["shotsCurrent"])
	if maxCount > 0 && current >= maxCount {
		return current, false, true, nil
	}
	if err := w.MoveObject(objectID, model.ObjectLocation{ContainerID: containerID}); err != nil {
		return 0, false, false, err
	}
	container = w.objects[containerID]
	if container.Properties == nil {
		container.Properties = map[string]string{}
	}
	current++
	container.Properties["shotsCurrent"] = fmt.Sprintf("%d", current)
	w.objects[containerID] = container
	return current, true, false, nil
}

func (w *fakeDropWorld) DropCreatureGoldToRoom(creatureID model.CreatureID, roomID model.RoomID, amount int) (model.ObjectInstanceID, int, bool, error) {
	creature, ok := w.creatures[creatureID]
	if !ok {
		return "", 0, false, fmt.Errorf("creature %q not found", creatureID)
	}
	if _, ok := w.rooms[roomID]; !ok {
		return "", 0, false, fmt.Errorf("room %q not found", roomID)
	}
	gold := creature.Stats["gold"]
	if gold < amount {
		return "", gold, false, nil
	}
	gold -= amount
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats["gold"] = gold
	w.creatures[creatureID] = creature
	id := model.ObjectInstanceID(fmt.Sprintf("object:money:%d", len(w.objects)+1))
	w.objects[id] = model.ObjectInstance{
		ID:                  id,
		DisplayNameOverride: fmt.Sprintf("%d냥", amount),
		Location:            model.ObjectLocation{RoomID: roomID},
		Properties: map[string]string{
			"kind":  string(model.ObjectKindMoney),
			"type":  "10",
			"value": fmt.Sprintf("%d", amount),
		},
	}
	room := w.rooms[roomID]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, id)
	w.rooms[roomID] = room
	return id, gold, true, nil
}

func (w *fakeDropWorld) UpdateCreatureTags(id model.CreatureID, add []string, remove []string) (model.Creature, error) {
	creature, ok := w.creatures[id]
	if !ok {
		return model.Creature{}, fmt.Errorf("creature %q not found", id)
	}
	creature.Metadata.Tags = updateDropFakeTags(creature.Metadata.Tags, add, remove)
	w.creatures[id] = creature
	return creature, nil
}

func (w *fakeDropWorld) UpdatePlayerTags(id model.PlayerID, add []string, remove []string) (model.Player, error) {
	player, ok := w.players[id]
	if !ok {
		return model.Player{}, fmt.Errorf("player %q not found", id)
	}
	player.Metadata.Tags = updateDropFakeTags(player.Metadata.Tags, add, remove)
	w.players[id] = player
	return player, nil
}

func (w *fakeDropWorld) SetCreatureStat(id model.CreatureID, key string, value int) error {
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

func updateDropFakeTags(tags []string, add []string, remove []string) []string {
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

func dropFakeObjectInt(value string) int {
	var out int
	_, _ = fmt.Sscanf(value, "%d", &out)
	return out
}

func (w *fakeDropWorld) removeObjectFromLocation(id model.ObjectInstanceID, location model.ObjectLocation) {
	if !location.CreatureID.IsZero() {
		creature := w.creatures[location.CreatureID]
		creature.Inventory.ObjectIDs = removeDropObjectID(creature.Inventory.ObjectIDs, id)
		w.creatures[creature.ID] = creature
	}
	if !location.RoomID.IsZero() {
		room := w.rooms[location.RoomID]
		room.Objects.ObjectIDs = removeDropObjectID(room.Objects.ObjectIDs, id)
		w.rooms[room.ID] = room
	}
	if !location.ContainerID.IsZero() {
		container := w.objects[location.ContainerID]
		container.Contents.ObjectIDs = removeDropObjectID(container.Contents.ObjectIDs, id)
		w.objects[container.ID] = container
	}
}

func (w *fakeDropWorld) addObjectToLocation(id model.ObjectInstanceID, location model.ObjectLocation) {
	if !location.RoomID.IsZero() {
		room := w.rooms[location.RoomID]
		room.Objects.ObjectIDs = appendDropObjectIDOnce(room.Objects.ObjectIDs, id)
		w.rooms[room.ID] = room
	}
	if !location.CreatureID.IsZero() {
		creature := w.creatures[location.CreatureID]
		creature.Inventory.ObjectIDs = appendDropObjectIDOnce(creature.Inventory.ObjectIDs, id)
		w.creatures[creature.ID] = creature
	}
	if !location.ContainerID.IsZero() {
		container := w.objects[location.ContainerID]
		container.Contents.ObjectIDs = appendDropObjectIDOnce(container.Contents.ObjectIDs, id)
		w.objects[container.ID] = container
	}
}

func assertDropObjectInRoom(t *testing.T, world *fakeDropWorld, objectID model.ObjectInstanceID, roomID model.RoomID) {
	t.Helper()

	object := world.objects[objectID]
	if object.Location.RoomID != roomID || !object.Location.CreatureID.IsZero() || object.Location.Slot != "" {
		t.Fatalf("%s location = %+v, want room %q only", objectID, object.Location, roomID)
	}
	if !containsDropObjectID(world.rooms[roomID].Objects.ObjectIDs, objectID) {
		t.Fatalf("room objects = %+v, want %q", world.rooms[roomID].Objects.ObjectIDs, objectID)
	}
}

func assertDropObjectInContainer(t *testing.T, world *fakeDropWorld, objectID model.ObjectInstanceID, containerID model.ObjectInstanceID) {
	t.Helper()

	object := world.objects[objectID]
	if object.Location.ContainerID != containerID ||
		!object.Location.RoomID.IsZero() ||
		!object.Location.CreatureID.IsZero() ||
		object.Location.Slot != "" {
		t.Fatalf("%s location = %+v, want container %q only", objectID, object.Location, containerID)
	}
	if !containsDropObjectID(world.objects[containerID].Contents.ObjectIDs, objectID) {
		t.Fatalf("container contents = %+v, want %q", world.objects[containerID].Contents.ObjectIDs, objectID)
	}
}

func assertDropMovedObject(t *testing.T, world *fakeDropWorld, objectID model.ObjectInstanceID, roomID model.RoomID) {
	t.Helper()

	if len(world.moves) != 1 {
		t.Fatalf("moves = %+v, want one move", world.moves)
	}
	if world.moves[0].objectID != objectID || world.moves[0].location.RoomID != roomID {
		t.Fatalf("move = %+v, want %q to room %q", world.moves[0], objectID, roomID)
	}

	object := world.objects[objectID]
	if object.Location.RoomID != roomID || !object.Location.CreatureID.IsZero() || object.Location.Slot != "" {
		t.Fatalf("object location = %+v, want room %q only", object.Location, roomID)
	}
	if !containsDropObjectID(world.rooms[roomID].Objects.ObjectIDs, objectID) {
		t.Fatalf("room objects = %+v, want %q", world.rooms[roomID].Objects.ObjectIDs, objectID)
	}
}

func assertDropMovedObjectToContainer(
	t *testing.T,
	world *fakeDropWorld,
	objectID model.ObjectInstanceID,
	containerID model.ObjectInstanceID,
) {
	t.Helper()

	if len(world.moves) != 1 {
		t.Fatalf("moves = %+v, want one move", world.moves)
	}
	if world.moves[0].objectID != objectID || world.moves[0].location.ContainerID != containerID {
		t.Fatalf("move = %+v, want %q to container %q", world.moves[0], objectID, containerID)
	}

	object := world.objects[objectID]
	if object.Location.ContainerID != containerID ||
		!object.Location.RoomID.IsZero() ||
		!object.Location.CreatureID.IsZero() ||
		object.Location.Slot != "" {
		t.Fatalf("object location = %+v, want container %q only", object.Location, containerID)
	}
	if containsDropObjectID(world.creatures["creature:alice"].Inventory.ObjectIDs, objectID) {
		t.Fatalf("inventory still contains moved object: %+v", world.creatures["creature:alice"].Inventory.ObjectIDs)
	}
	if !containsDropObjectID(world.objects[containerID].Contents.ObjectIDs, objectID) {
		t.Fatalf("container contents = %+v, want %q", world.objects[containerID].Contents.ObjectIDs, objectID)
	}
}

func assertDropSavedPlayer(t *testing.T, world *fakeDropWorld, playerID model.PlayerID) {
	t.Helper()

	if len(world.saves) != 1 || world.saves[0] != playerID {
		t.Fatalf("saves = %+v, want [%s]", world.saves, playerID)
	}
}

func appendDropObjectIDOnce(ids []model.ObjectInstanceID, id model.ObjectInstanceID) []model.ObjectInstanceID {
	if containsDropObjectID(ids, id) {
		return ids
	}
	return append(ids, id)
}

func removeDropObjectID(ids []model.ObjectInstanceID, id model.ObjectInstanceID) []model.ObjectInstanceID {
	kept := ids[:0]
	for _, existing := range ids {
		if existing != id {
			kept = append(kept, existing)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

func containsDropObjectID(ids []model.ObjectInstanceID, id model.ObjectInstanceID) bool {
	for _, existing := range ids {
		if existing == id {
			return true
		}
	}
	return false
}
