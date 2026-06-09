package command

import (
	"reflect"
	"strings"
	"testing"

	"muhan/internal/commandspec"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestZapHandlerConsumesWandChargeOnEffectSuccess(t *testing.T) {
	loaded := zapWorld(t, "room:plaza", "2", "3")
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Players[player.ID] = player
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"hidden", "PHIDDN"}
	creature.Stats["PHIDDN"] = 1
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)
	var gotArgs []string
	effect := func(_ *Context, _ ZapWorld, _ model.Creature, _ model.ObjectInstance, resolved ResolvedCommand) (bool, error) {
		gotArgs = append([]string(nil), resolved.Args...)
		return true, nil
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewZapHandler(runtime, effect)(ctx, ResolvedCommand{Args: []string{"마법봉", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if ctx.OutputString() != "번쩍\n" {
		t.Fatalf("output = %q, want useOutput", ctx.OutputString())
	}
	if !reflect.DeepEqual(gotArgs, []string{"마법봉", "상인"}) {
		t.Fatalf("effect args = %+v, want original args", gotArgs)
	}
	wand, ok := runtime.Object("object:wand")
	if !ok {
		t.Fatal("wand deleted, want retained")
	}
	if wand.Properties["shotsCurrent"] != "1" {
		t.Fatalf("shotsCurrent = %q, want 1", wand.Properties["shotsCurrent"])
	}
	updatedCreature, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updatedCreature.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("creature tags = %+v, want hidden cleared", updatedCreature.Metadata.Tags)
	}
	if updatedCreature.Stats["PHIDDN"] != 0 {
		t.Fatalf("creature PHIDDN = %d, want 0", updatedCreature.Stats["PHIDDN"])
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("player tags = %+v, want hidden cleared", updatedPlayer.Metadata.Tags)
	}
}

func TestZapHandlerDefaultEffectDoesNotConsumeUnsupportedMagicPower(t *testing.T) {
	runtime := state.NewWorld(zapWorld(t, "room:plaza", "2", "999"))
	ctx := &Context{ActorID: "player:alice"}

	status, err := NewZapHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"마법봉", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want no effect output", status, ctx.OutputString())
	}
	wand, _ := runtime.Object("object:wand")
	if wand.Properties["shotsCurrent"] != "2" {
		t.Fatalf("shotsCurrent = %q, want unchanged 2", wand.Properties["shotsCurrent"])
	}
}

func TestZapHandlerRejectsInvalidWandStates(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		shots      string
		magicPower string
		roomTags   []string
		creature   []string
		want       string
	}{
		{name: "missing target", want: "\n무엇을 사용합니까?\n"},
		{name: "missing object", args: []string{"없는"}, shots: "1", magicPower: "1", want: "\n그런것이 존재하지 않습니다.\n"},
		{name: "non wand", args: []string{"돌"}, shots: "1", magicPower: "1", want: "\n막대나 지팡이가 아닙니다.\n"},
		{name: "empty shots", args: []string{"마법봉"}, shots: "0", magicPower: "1", want: "\n모두 써버렸습니다.\n"},
		{name: "empty magic", args: []string{"마법봉"}, shots: "1", magicPower: "0", want: "\n아무런 일도 일어나지 않습니다.\n"},
		{name: "no magic room", args: []string{"마법봉"}, shots: "1", magicPower: "1", roomTags: []string{"noMagic"}, want: "\n아무런 일도 일어나지 않습니다.\n"},
		{name: "blind", args: []string{"마법봉"}, shots: "1", magicPower: "1", creature: []string{"blind"}, want: "아무 것도 보이지 않습니다!\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := zapWorld(t, "room:plaza", tt.shots, tt.magicPower)
			room := loaded.Rooms["room:plaza"]
			room.Metadata.Tags = tt.roomTags
			loaded.Rooms[room.ID] = room
			creature := loaded.Creatures["creature:alice"]
			creature.Metadata.Tags = tt.creature
			loaded.Creatures[creature.ID] = creature
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewZapHandler(runtime, func(*Context, ZapWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error) {
				return true, nil
			})(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			if tt.args != nil && tt.args[0] == "마법봉" {
				wand, _ := runtime.Object("object:wand")
				if wand.Properties["shotsCurrent"] != tt.shots {
					t.Fatalf("shotsCurrent = %q, want unchanged %q", wand.Properties["shotsCurrent"], tt.shots)
				}
			}
		})
	}
}

func TestZapHandlerAppliesMagicItemRestrictions(t *testing.T) {
	tests := []struct {
		name          string
		creatureStats map[string]int
		objectTags    []string
		protoProps    map[string]string
		want          string
		wantDropped   bool
	}{
		{
			name:          "evil only rejects good actor and drops wand",
			creatureStats: map[string]int{"alignment": 101, "class": legacyClassFighter},
			objectTags:    []string{"evilOnly"},
			want:          "\n마법봉의 수명이 다한 듯 수증기처럼 증발해 버렸습니다.\n",
			wantDropped:   true,
		},
		{
			name:          "class selective rejects unlisted class",
			creatureStats: map[string]int{"class": legacyClassFighter},
			protoProps:    map[string]string{"classSelective": "1", "classMage": "1"},
			want:          "\n당신직업세계에서 금하는 물건이기에 사용할 수 없습니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := zapWorld(t, "room:plaza", "2", "3")
			creature := loaded.Creatures["creature:alice"]
			creature.Stats = tt.creatureStats
			loaded.Creatures[creature.ID] = creature
			proto := loaded.ObjectPrototypes["prototype:wand"]
			proto.Properties = tt.protoProps
			loaded.ObjectPrototypes[proto.ID] = proto
			wand := loaded.Objects["object:wand"]
			wand.Metadata.Tags = tt.objectTags
			loaded.Objects[wand.ID] = wand
			runtime := state.NewWorld(loaded)

			called := false
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewZapHandler(runtime, func(*Context, ZapWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error) {
				called = true
				return true, nil
			})(ctx, ResolvedCommand{Args: []string{"마법봉"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			if called {
				t.Fatal("effect was called despite restriction")
			}
			wand, ok := runtime.Object("object:wand")
			if !ok {
				t.Fatal("wand was deleted despite restriction")
			}
			if wand.Properties["shotsCurrent"] != "2" {
				t.Fatalf("shotsCurrent = %q, want unchanged 2", wand.Properties["shotsCurrent"])
			}
			if tt.wantDropped {
				if wand.Location.RoomID != "room:plaza" {
					t.Fatalf("wand location = %+v, want room:plaza", wand.Location)
				}
			} else if wand.Location.CreatureID != "creature:alice" {
				t.Fatalf("wand location = %+v, want creature inventory", wand.Location)
			}
		})
	}
}

func TestZapHandlerSpellFailConsumesChargeSilentlyLikeC(t *testing.T) {
	useSpellFailRoll(t, 99)
	loaded := zapWorld(t, "room:plaza", "2", "3")
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"hidden"}
	loaded.Creatures[creature.ID] = creature
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden"}
	loaded.Players[player.ID] = player
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewZapHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"마법봉"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want silent C zap spell_fail", status, ctx.OutputString())
	}
	wand, _ := runtime.Object("object:wand")
	if got := wand.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("shotsCurrent = %q, want one charge consumed by spell_fail", got)
	}
	updated, _ := runtime.Creature("creature:alice")
	if magicEffectTestHasExactTag(updated.Metadata.Tags, "PLIGHT") {
		t.Fatalf("creature tags = %+v, want spell effect skipped", updated.Metadata.Tags)
	}
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("creature tags = %+v, want hidden cleared before spell_fail", updated.Metadata.Tags)
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("player tags = %+v, want hidden cleared before spell_fail", updatedPlayer.Metadata.Tags)
	}
}

func TestZapHandlerAcceptsLegacyTypeEightAndEquippedWand(t *testing.T) {
	loaded := zapWorld(t, "room:plaza", "1", "3")
	proto := loaded.ObjectPrototypes["prototype:wand"]
	proto.Kind = model.ObjectKindMisc
	proto.Properties = map[string]string{"type": "8"}
	loaded.ObjectPrototypes[proto.ID] = proto
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = []model.ObjectInstanceID{"object:stone"}
	creature.Equipment = map[string]model.ObjectInstanceID{"held": "object:wand"}
	loaded.Creatures[creature.ID] = creature
	wand := loaded.Objects["object:wand"]
	wand.Location = model.ObjectLocation{CreatureID: "creature:alice", Slot: "held"}
	loaded.Objects[wand.ID] = wand
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewZapHandler(runtime, func(*Context, ZapWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error) {
		return true, nil
	})(ctx, ResolvedCommand{Args: []string{"마법봉"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "번쩍") {
		t.Fatalf("status/output = %d/%q, want success", status, ctx.OutputString())
	}
	wand, _ = runtime.Object("object:wand")
	if wand.Properties["shotsCurrent"] != "0" {
		t.Fatalf("shotsCurrent = %q, want 0 retained", wand.Properties["shotsCurrent"])
	}
}

func TestZapHandlerDispatchesAlias(t *testing.T) {
	useSpellFailRoll(t, 0)
	runtime := state.NewWorld(zapWorld(t, "room:plaza", "1", "3"))
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "겨냥", Number: 53, Handler: "zap"},
		}),
		Handlers: map[string]Handler{
			"zap": NewZapHandler(runtime, nil),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "마법봉 상인 겨냥")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	wantOutput := "당신의 왼손에 발광 주문을 걸었습니다.\n왼손에서 황금빛이 뿜어져 나와 주위를 밝혀 줍니다.\n번쩍\n"
	if status != StatusDefault || ctx.OutputString() != wantOutput {
		t.Fatalf("status/output = %d/%q, want light wand use output", status, ctx.OutputString())
	}
	creature, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "PLIGHT", "light") {
		t.Fatalf("creature tags = %+v, want PLIGHT", creature.Metadata.Tags)
	}
	wand, _ := runtime.Object("object:wand")
	if wand.Properties["shotsCurrent"] != "0" {
		t.Fatalf("shotsCurrent = %q, want consumed", wand.Properties["shotsCurrent"])
	}
}

func zapWorld(t *testing.T, roomID model.RoomID, shots string, magicPower string) *worldload.World {
	t.Helper()

	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{ID: roomID, DisplayName: "광장"})
	player := loaded.Players["player:alice"]
	player.RoomID = roomID
	loaded.Players[player.ID] = player
	creature := loaded.Creatures["creature:alice"]
	creature.RoomID = roomID
	creature.Inventory = model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:wand", "object:stone"}}
	creature.Stats = map[string]int{
		"class":     legacyClassCleric,
		"hpCurrent": 50,
		"hpMax":     100,
		"mpCurrent": 100,
		"mpMax":     100,
	}
	loaded.Creatures[creature.ID] = creature
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:wand",
		Kind:        model.ObjectKindWand,
		DisplayName: "마법봉",
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:stone",
		Kind:        model.ObjectKindMisc,
		DisplayName: "돌",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:wand",
		PrototypeID: "prototype:wand",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties: map[string]string{
			"type":         "8",
			"shotsCurrent": shots,
			"magicPower":   magicPower,
			"useOutput":    "번쩍",
		},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:stone",
		PrototypeID: "prototype:stone",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	})
	return loaded
}
