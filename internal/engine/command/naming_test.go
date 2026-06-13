package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestChangeNameHandlerRenamesCustomNameObject(t *testing.T) {
	world := state.NewWorld(changeNameWorld(t, []model.ObjectInstance{
		changeNameObject("object:token", "prototype:token", map[string]string{"OCNAME": "1"}, nil),
	}))
	var broadcasts []string
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
				broadcasts = append(broadcasts, text)
				return nil
			}),
		},
	}

	status, err := NewChangeNameHandler(world)(ctx, ResolvedCommand{Args: []string{"명패", "새", "이름"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusPrompt {
		t.Fatalf("status = %d, want prompt", status)
	}
	if got, want := ctx.OutputString(), "\n이름 명명 되었습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if len(broadcasts) != 1 || broadcasts[0] != "\nAlice가 자신의 물건에 이름을 명명합니다." {
		t.Fatalf("broadcasts = %+v, want legacy rename broadcast without final newline", broadcasts)
	}

	object, ok := world.Object("object:token")
	if !ok {
		t.Fatal("missing object")
	}
	if object.DisplayNameOverride != "새 이름" || object.Properties["name"] != "새 이름" {
		t.Fatalf("object name = %q / %q, want 새 이름", object.DisplayNameOverride, object.Properties["name"])
	}
	for _, key := range []string{"OCNAME", "customName", "cname"} {
		if got := object.Properties[key]; got != "" {
			t.Fatalf("%s property = %q, want cleared like C F_CLR(OCNAME)", key, got)
		}
	}
	if !objectHasAnyTag(world, object, "named", "ONAMED") {
		t.Fatalf("object tags = %+v, want named", object.Metadata.Tags)
	}
	if objectHasAnyTag(world, object, "customName", "OCNAME") {
		t.Fatalf("object tags still allow rename: %+v", object.Metadata.Tags)
	}
}

func TestChangeNameHandlerPrototypeBackedCustomNameCanRenameOnlyOnce(t *testing.T) {
	loaded := changeNameWorld(t, []model.ObjectInstance{
		changeNameObject("object:token", "prototype:token", nil, nil),
	})
	proto := loaded.ObjectPrototypes["prototype:token"]
	proto.Properties["OCNAME"] = "1"
	loaded.ObjectPrototypes[proto.ID] = proto
	world := state.NewWorld(loaded)
	defer world.Close()
	handler := NewChangeNameHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"명패", "첫", "이름"}})
	if err != nil {
		t.Fatalf("first handler() error = %v", err)
	}
	if status != StatusPrompt {
		t.Fatalf("first status = %d, want prompt", status)
	}

	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{Args: []string{"첫", "두번째"}})
	if err != nil {
		t.Fatalf("second handler() error = %v", err)
	}
	if status != StatusPrompt {
		t.Fatalf("second status = %d, want prompt", status)
	}
	if !strings.Contains(ctx.OutputString(), "명명 할수 있는 물건이 아닙니다.") {
		t.Fatalf("second output = %q, want C one-shot OCNAME refusal", ctx.OutputString())
	}
	object, ok := world.Object("object:token")
	if !ok {
		t.Fatal("missing object")
	}
	if got := object.DisplayNameOverride; got != "첫 이름" {
		t.Fatalf("display name = %q, want first name retained", got)
	}
}

func TestChangeNameHandlerRecordsEventOwner(t *testing.T) {
	world := state.NewWorld(changeNameWorld(t, []model.ObjectInstance{
		changeNameObject("object:event", "prototype:event", map[string]string{"OEVENT": "1", "key[2]": "이벤트"}, nil),
	}))
	ctx := &Context{ActorID: "player:alice"}

	status, err := NewChangeNameHandler(world)(ctx, ResolvedCommand{Args: []string{"이벤트패", "기록"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusPrompt {
		t.Fatalf("status = %d, want prompt", status)
	}
	if got, want := ctx.OutputString(), "이름을 기록하였습니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	object, ok := world.Object("object:event")
	if !ok {
		t.Fatal("missing object")
	}
	if got := object.Properties["key[2]"]; got != "Alice" {
		t.Fatalf("key[2] = %q, want Alice", got)
	}
	if object.DisplayNameOverride != "" {
		t.Fatalf("display name changed to %q, want unchanged", object.DisplayNameOverride)
	}
}

func TestChangeNameHandlerRecordsPrototypeBackedEventOwner(t *testing.T) {
	loaded := changeNameWorld(t, []model.ObjectInstance{
		changeNameObject("object:event", "prototype:event", nil, nil),
	})
	proto := loaded.ObjectPrototypes["prototype:event"]
	proto.Properties = map[string]string{"OEVENT": "1", "key[2]": "이벤트"}
	loaded.ObjectPrototypes[proto.ID] = proto
	world := state.NewWorld(loaded)
	defer world.Close()
	ctx := &Context{ActorID: "player:alice"}

	status, err := NewChangeNameHandler(world)(ctx, ResolvedCommand{Args: []string{"이벤트패", "기록"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusPrompt {
		t.Fatalf("status = %d, want prompt", status)
	}
	if got, want := ctx.OutputString(), "이름을 기록하였습니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	object, ok := world.Object("object:event")
	if !ok {
		t.Fatal("missing object")
	}
	if got := object.Properties["key[2]"]; got != "Alice" {
		t.Fatalf("instance key[2] = %q, want owner recorded from prototype-backed event", got)
	}
	if object.DisplayNameOverride != "" {
		t.Fatalf("display name changed to %q, want unchanged", object.DisplayNameOverride)
	}
}

func TestChangeNameHandlerRejectsInvalidCases(t *testing.T) {
	tests := []struct {
		name    string
		objects []model.ObjectInstance
		args    []string
		want    string
	}{
		{
			name: "missing args",
			args: []string{"명패"},
			want: "어떤 물건을 무슨 이름으로 바꾸고 싶으세요?",
		},
		{
			name:    "missing object",
			objects: []model.ObjectInstance{},
			args:    []string{"명패", "새이름"},
			want:    "그런 물건은 없어요.",
		},
		{
			name: "not nameable",
			objects: []model.ObjectInstance{
				changeNameObject("object:plain", "prototype:plain", nil, nil),
			},
			args: []string{"돌", "새이름"},
			want: "명명 할수 있는 물건이 아닙니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := state.NewWorld(changeNameWorld(t, tt.objects))
	defer world.Close()
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewChangeNameHandler(world)(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusPrompt {
				t.Fatalf("status = %d, want prompt", status)
			}
			if !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("output = %q, want %q", ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestChangeNameHandlerDispatchAlias(t *testing.T) {
	world := state.NewWorld(changeNameWorld(t, []model.ObjectInstance{
		changeNameObject("object:token", "prototype:token", map[string]string{"OCNAME": "1"}, nil),
	}))
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "명명", Number: 95, Handler: "chg_name"},
		}),
		Handlers: map[string]Handler{"chg_name": NewChangeNameHandler(world)},
	}
	ctx := &Context{ActorID: "player:alice"}

	status, err := dispatcher.DispatchLine(ctx, "명패 청룡패 명명")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusPrompt {
		t.Fatalf("status = %d, want prompt", status)
	}
	object, ok := world.Object("object:token")
	if !ok {
		t.Fatal("missing object")
	}
	if object.DisplayNameOverride != "청룡패" {
		t.Fatalf("display name = %q, want 청룡패", object.DisplayNameOverride)
	}
}

func TestChangeNameHandlerPreservesLegacyRawArgumentParsing(t *testing.T) {
	dispatcherFor := func(world ChangeNameWorld) Dispatcher {
		return Dispatcher{
			Registry: mustRegistry(t, []commandspec.CommandSpec{
				{Name: "명명", Number: 95, Handler: "chg_name"},
			}),
			Handlers: map[string]Handler{"chg_name": NewChangeNameHandler(world)},
		}
	}

	t.Run("verb-final name preserves cut_command trailing spaces", func(t *testing.T) {
		world := state.NewWorld(changeNameWorld(t, []model.ObjectInstance{
			changeNameObject("object:token", "prototype:token", map[string]string{"OCNAME": "1"}, nil),
		}))
		ctx := &Context{ActorID: "player:alice"}
		status, err := dispatcherFor(world).DispatchLine(ctx, "명패 새 이름   명명")
		if err != nil {
			t.Fatalf("DispatchLine() error = %v", err)
		}
		if status != StatusPrompt {
			t.Fatalf("status = %d, want prompt", status)
		}
		object, ok := world.Object("object:token")
		if !ok {
			t.Fatal("missing object")
		}
		if got := object.DisplayNameOverride; got != "새 이름  " {
			t.Fatalf("display name = %q, want legacy trailing spaces", got)
		}
	})

	t.Run("glued ordinal selects matching legacy object", func(t *testing.T) {
		world := state.NewWorld(changeNameWorld(t, []model.ObjectInstance{
			changeNameObject("object:first", "prototype:token", map[string]string{"OCNAME": "1"}, nil),
			changeNameObject("object:second", "prototype:token", map[string]string{"OCNAME": "1"}, nil),
		}))
		ctx := &Context{ActorID: "player:alice"}
		status, err := dispatcherFor(world).DispatchLine(ctx, "명패 2청룡패 명명")
		if err != nil {
			t.Fatalf("DispatchLine() error = %v", err)
		}
		if status != StatusPrompt {
			t.Fatalf("status = %d, want prompt", status)
		}
		first, _ := world.Object("object:first")
		second, _ := world.Object("object:second")
		if first.DisplayNameOverride != "" {
			t.Fatalf("first display name = %q, want unchanged", first.DisplayNameOverride)
		}
		if second.DisplayNameOverride != "청룡패" {
			t.Fatalf("second display name = %q, want 청룡패", second.DisplayNameOverride)
		}
	})

	t.Run("zero ordinal behaves like legacy find_obj miss", func(t *testing.T) {
		world := state.NewWorld(changeNameWorld(t, []model.ObjectInstance{
			changeNameObject("object:token", "prototype:token", map[string]string{"OCNAME": "1"}, nil),
		}))
		ctx := &Context{ActorID: "player:alice"}
		status, err := dispatcherFor(world).DispatchLine(ctx, "명패 0새이름 명명")
		if err != nil {
			t.Fatalf("DispatchLine() error = %v", err)
		}
		if status != StatusPrompt {
			t.Fatalf("status = %d, want prompt", status)
		}
		if got, want := ctx.OutputString(), "그런 물건은 없어요."; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
		object, _ := world.Object("object:token")
		if object.DisplayNameOverride != "" {
			t.Fatalf("display name = %q, want unchanged", object.DisplayNameOverride)
		}
	})
}

func TestChangeNameHandlerTruncatesNameByLegacyBytes(t *testing.T) {
	world := state.NewWorld(changeNameWorld(t, []model.ObjectInstance{
		changeNameObject("object:token", "prototype:token", map[string]string{"OCNAME": "1"}, nil),
	}))
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "명명", Number: 95, Handler: "chg_name"},
		}),
		Handlers: map[string]Handler{"chg_name": NewChangeNameHandler(world)},
	}
	ctx := &Context{ActorID: "player:alice"}
	longName := strings.Repeat("가", 40)

	status, err := dispatcher.DispatchLine(ctx, "명패 "+longName+" 명명")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusPrompt {
		t.Fatalf("status = %d, want prompt", status)
	}
	object, ok := world.Object("object:token")
	if !ok {
		t.Fatal("missing object")
	}
	if got, want := object.DisplayNameOverride, strings.Repeat("가", 39); got != want {
		t.Fatalf("display name = %q, want legacy 79-byte truncation %q", got, want)
	}
}

func changeNameWorld(t *testing.T, objects []model.ObjectInstance) *worldload.World {
	t.Helper()
	loaded := worldload.NewWorld()
	if err := loaded.AddRoom(model.Room{ID: "room:plaza", DisplayName: "광장"}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddPlayer(model.Player{ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:plaza"}); err != nil {
		t.Fatal(err)
	}
	inventory := make([]model.ObjectInstanceID, 0, len(objects))
	for _, object := range objects {
		inventory = append(inventory, object.ID)
	}
	if err := loaded.AddCreature(model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:plaza",
		Inventory:   model.ObjectRefList{ObjectIDs: inventory},
	}); err != nil {
		t.Fatal(err)
	}
	for _, proto := range []model.ObjectPrototype{
		{ID: "prototype:token", DisplayName: "명패", Properties: map[string]string{"key[0]": "명패"}},
		{ID: "prototype:event", DisplayName: "이벤트패", Properties: map[string]string{"key[0]": "이벤트패"}},
		{ID: "prototype:plain", DisplayName: "돌", Properties: map[string]string{"key[0]": "돌"}},
	} {
		if err := loaded.AddObjectPrototype(proto); err != nil {
			t.Fatal(err)
		}
	}
	for _, object := range objects {
		if err := loaded.AddObjectInstance(object); err != nil {
			t.Fatal(err)
		}
	}
	return loaded
}

func changeNameObject(id model.ObjectInstanceID, proto model.PrototypeID, properties map[string]string, tags []string) model.ObjectInstance {
	props := map[string]string{}
	for key, value := range properties {
		props[key] = value
	}
	return model.ObjectInstance{
		ID:          id,
		PrototypeID: proto,
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  props,
		Metadata:    model.Metadata{Tags: tags},
	}
}
