package command

import (
	"strings"
	"testing"

	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type mockDMObjNameWorld struct {
	players     map[model.PlayerID]model.Player
	creatures   map[model.CreatureID]model.Creature
	rooms       map[model.RoomID]model.Room
	objects     map[model.ObjectInstanceID]model.ObjectInstance
	objectOrder []model.ObjectInstanceID

	// Spies
	objectProps map[model.ObjectInstanceID]map[string]string
}

func (w *mockDMObjNameWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMObjNameWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMObjNameWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *mockDMObjNameWorld) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	o, ok := w.objects[id]
	return o, ok
}

func (w *mockDMObjNameWorld) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	return model.ObjectPrototype{}, false
}

func (w *mockDMObjNameWorld) orderedObjects() []model.ObjectInstance {
	if len(w.objectOrder) == 0 {
		objects := make([]model.ObjectInstance, 0, len(w.objects))
		for _, o := range w.objects {
			objects = append(objects, o)
		}
		return objects
	}

	objects := make([]model.ObjectInstance, 0, len(w.objectOrder))
	for _, id := range w.objectOrder {
		if o, ok := w.objects[id]; ok {
			objects = append(objects, o)
		}
	}
	return objects
}

func (w *mockDMObjNameWorld) FindObjectInRoom(roomID model.RoomID, name string) (model.ObjectInstance, bool) {
	for _, o := range w.orderedObjects() {
		if o.Location.RoomID == roomID && (strings.EqualFold(o.DisplayNameOverride, name) || strings.EqualFold(o.Properties["name"], name)) {
			return o, true
		}
	}
	return model.ObjectInstance{}, false
}

func (w *mockDMObjNameWorld) FindObjectOnCreature(creatureID model.CreatureID, name string) (model.ObjectInstance, bool) {
	for _, o := range w.orderedObjects() {
		if o.Location.CreatureID == creatureID && (strings.EqualFold(o.DisplayNameOverride, name) || strings.EqualFold(o.Properties["name"], name)) {
			return o, true
		}
	}
	return model.ObjectInstance{}, false
}

func (w *mockDMObjNameWorld) FindObjectInRoomByName(roomID model.RoomID, name string, count int) (model.ObjectInstance, bool) {
	if count < 1 {
		count = 1
	}
	seen := 0
	for _, o := range w.orderedObjects() {
		if o.Location.RoomID == roomID && (strings.EqualFold(o.DisplayNameOverride, name) || strings.EqualFold(o.Properties["name"], name)) {
			seen++
			if seen == count {
				return o, true
			}
		}
	}
	return model.ObjectInstance{}, false
}

func (w *mockDMObjNameWorld) FindObjectOnCreatureByName(creatureID model.CreatureID, name string, count int) (model.ObjectInstance, bool) {
	if count < 1 {
		count = 1
	}
	seen := 0
	for _, o := range w.orderedObjects() {
		if o.Location.CreatureID == creatureID && (strings.EqualFold(o.DisplayNameOverride, name) || strings.EqualFold(o.Properties["name"], name)) {
			seen++
			if seen == count {
				return o, true
			}
		}
	}
	return model.ObjectInstance{}, false
}

func (w *mockDMObjNameWorld) UpdateObjectProperty(objectID model.ObjectInstanceID, key, val string) error {
	if w.objectProps[objectID] == nil {
		w.objectProps[objectID] = make(map[string]string)
	}
	w.objectProps[objectID][key] = val
	return nil
}

func TestDMObjName(t *testing.T) {
	world := &mockDMObjNameWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:00001"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:        "creature:alice",
				RoomID:    "room:00001",
				Inventory: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"obj:sword", "obj:sword2"}},
				Stats:     map[string]int{"class": legacyClassSubDM}, // SUB_DM
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:00001": {
				ID:      "room:00001",
				Objects: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"obj:shield"}},
			},
		},
		objects: map[model.ObjectInstanceID]model.ObjectInstance{
			"obj:sword": {
				ID:                  "obj:sword",
				DisplayNameOverride: "sword",
				Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
			},
			"obj:sword2": {
				ID:                  "obj:sword2",
				DisplayNameOverride: "sword",
				Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
			},
			"obj:shield": {
				ID:                  "obj:shield",
				DisplayNameOverride: "shield",
				Location:            model.ObjectLocation{RoomID: "room:00001"},
			},
		},
		objectOrder: []model.ObjectInstanceID{"obj:sword", "obj:sword2", "obj:shield"},
		objectProps: make(map[model.ObjectInstanceID]map[string]string),
	}

	handler := NewDMObjNameHandler(world)

	// Case 1: Denied permission (class below SUB_DM)
	t.Run("Denied permission", func(t *testing.T) {
		world.creatures["creature:alice"].Stats["class"] = 9
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname sword new-sword",
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}
		if got := ctx.OutputString(); got != "" {
			t.Errorf("output = %q, want no permission output", got)
		}
		world.creatures["creature:alice"].Stats["class"] = legacyClassSubDM // restore
	})

	// Case 2: Insufficient arguments
	t.Run("Insufficient arguments", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname",
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}
		want := "어떤 물건을 무슨 이름으로 바꾸고 싶으세요?*oname <object> [#] [-dok] <name>\n"
		if got := ctx.OutputString(); got != want {
			t.Errorf("usage message = %q, want %q", got, want)
		}
	})

	t.Run("Existing target without value returns prompt silently like legacy", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname sword",
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}
		if got := ctx.OutputString(); got != "" {
			t.Errorf("output = %q, want no output", got)
		}
		if got := world.objectProps["obj:sword"]["name"]; got != "" {
			t.Errorf("object name property = %q, want no mutation", got)
		}
	})

	t.Run("Missing target without value still reports legacy not found", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname non_existent",
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}
		want := "그런 아이템은 없어요."
		if got := ctx.OutputString(); got != want {
			t.Errorf("not-found message = %q, want %q", got, want)
		}
	})

	// Case 3: Object not found
	t.Run("Object not found", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname non_existent new_name",
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}
		want := "그런 아이템은 없어요."
		if got := ctx.OutputString(); got != want {
			t.Errorf("not-found message = %q, want %q", got, want)
		}
	})

	// Case 4: Rename (none flag)
	t.Run("Rename none flag", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname sword Excalibur",
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "\n이름이 바뀌었습니다."
		if got := ctx.OutputString(); got != want {
			t.Errorf("success message = %q, want %q", got, want)
		}
		if world.objectProps["obj:sword"]["name"] != "Excalibur" {
			t.Errorf("name property = %q, want Excalibur", world.objectProps["obj:sword"]["name"])
		}
	})

	t.Run("Verb-final name preserves legacy cut_command trailing spaces", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "sword Excalibur   *oname",
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := ctx.OutputString(); got != "\n이름이 바뀌었습니다." {
			t.Errorf("success message = %q, want rename success", got)
		}
		if got := world.objectProps["obj:sword"]["name"]; got != "Excalibur  " {
			t.Errorf("name property = %q, want legacy trailing spaces", got)
		}
	})

	t.Run("Ordinal selects second inventory object", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname sword 2 Silverfang",
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "\n이름이 바뀌었습니다."
		if got := ctx.OutputString(); got != want {
			t.Errorf("success message = %q, want %q", got, want)
		}
		if world.objectProps["obj:sword2"]["name"] != "Silverfang" {
			t.Errorf("second sword name = %q, want Silverfang", world.objectProps["obj:sword2"]["name"])
		}
		if world.objectProps["obj:sword"]["name"] == "Silverfang" {
			t.Errorf("first sword was renamed by ordinal lookup")
		}
	})

	// Case 5: Change description (-d flag)
	t.Run("Change description -d flag", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname sword -d a very shiny sword",
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(ctx.OutputString(), "설명이 바뀌었습니다.") {
			t.Errorf("expected success message, got: %q", ctx.OutputString())
		}
		if world.objectProps["obj:sword"]["description"] != "a very shiny sword" {
			t.Errorf("description property = %q, want 'a very shiny sword'", world.objectProps["obj:sword"]["description"])
		}
	})

	// Case 6: Change use output (-o flag)
	t.Run("Change use output -o flag", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname sword -o The sword glows with holy light.",
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(ctx.OutputString(), "출력문이 바뀌었습니다.") {
			t.Errorf("expected success message, got: %q", ctx.OutputString())
		}
		if world.objectProps["obj:sword"]["use_output"] != "The sword glows with holy light." {
			t.Errorf("use_output = %q, want 'The sword glows with holy light.'", world.objectProps["obj:sword"]["use_output"])
		}
	})

	// Case 7: Change keyword (-k1 flag)
	t.Run("Change keyword -k1 flag", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname sword -k1 holy_blade",
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(ctx.OutputString(), "키가 바뀌었습니다.") {
			t.Errorf("expected success message, got: %q", ctx.OutputString())
		}
		if world.objectProps["obj:sword"]["key[0]"] != "holy_blade" {
			t.Errorf("key[0] = %q, want 'holy_blade'", world.objectProps["obj:sword"]["key[0]"])
		}
	})

	t.Run("Separated keyword number keeps number in value like legacy atoi", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname sword -k 1 holy blade",
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(ctx.OutputString(), "키가 바뀌었습니다.") {
			t.Errorf("expected success message, got: %q", ctx.OutputString())
		}
		if world.objectProps["obj:sword"]["key[0]"] != "1 holy blade" {
			t.Errorf("key[0] = %q, want '1 holy blade'", world.objectProps["obj:sword"]["key[0]"])
		}
	})

	// Case 8: Change keyword out of bounds (-k4 flag)
	t.Run("Change keyword -k4 flag", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname sword -k4 dummy_key",
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(ctx.OutputString(), "바뀌었습니다.") {
			t.Errorf("expected success message, got: %q", ctx.OutputString())
		}
		if val, exists := world.objectProps["obj:sword"]["key[3]"]; exists {
			t.Errorf("key[3] should not exist, but got %q", val)
		}
	})

	// Case 9: Unrecognized flag fallback (handled as part of name)
	t.Run("Unrecognized flag fallback", func(t *testing.T) {
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname sword -x unrecognized flag test",
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(ctx.OutputString(), "이름이 바뀌었습니다.") {
			t.Errorf("expected success message, got: %q", ctx.OutputString())
		}
		if world.objectProps["obj:sword"]["name"] != "-x unrecognized flag test" {
			t.Errorf("name = %q, want '-x unrecognized flag test'", world.objectProps["obj:sword"]["name"])
		}
	})

	// Case 10: Value limits (name limit 79, keywords limit 19)
	t.Run("Value limits", func(t *testing.T) {
		longName := strings.Repeat("A", 100)
		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname sword " + longName,
		}
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(world.objectProps["obj:sword"]["name"]) != 79 {
			t.Errorf("expected name to be truncated to 79 chars, got len %d", len(world.objectProps["obj:sword"]["name"]))
		}

		longKoreanName := strings.Repeat("가", 40)
		ctx = &Context{ActorID: "player:alice"}
		resolved = ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname sword " + longKoreanName,
		}
		_, err = handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := world.objectProps["obj:sword"]["name"], strings.Repeat("가", 39); got != want {
			t.Errorf("name = %q, want legacy 79-byte truncation %q", got, want)
		}

		longKey := strings.Repeat("K", 30)
		ctx = &Context{ActorID: "player:alice"}
		resolved = ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname sword -k2 " + longKey,
		}
		_, err = handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(world.objectProps["obj:sword"]["key[1]"]) != 19 {
			t.Errorf("expected key[1] to be truncated to 19 chars, got len %d", len(world.objectProps["obj:sword"]["key[1]"]))
		}

		longKoreanKey := strings.Repeat("키", 10)
		ctx = &Context{ActorID: "player:alice"}
		resolved = ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname sword -k2 " + longKoreanKey,
		}
		_, err = handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := world.objectProps["obj:sword"]["key[1]"], strings.Repeat("키", 9); got != want {
			t.Errorf("key[1] = %q, want legacy 19-byte truncation %q", got, want)
		}
	})
}

func TestDMObjNameAppliesFindObjInvisibleVisibility(t *testing.T) {
	baseWorld := func(actorTags []string) *mockDMObjNameWorld {
		return &mockDMObjNameWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {
					ID:        "creature:alice",
					RoomID:    "room:00001",
					Inventory: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"obj:hidden-sword"}},
					Stats:     map[string]int{"class": legacyClassSubDM},
					Metadata:  model.Metadata{Tags: actorTags},
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:00001": {ID: "room:00001"},
			},
			objects: map[model.ObjectInstanceID]model.ObjectInstance{
				"obj:hidden-sword": {
					ID:                  "obj:hidden-sword",
					DisplayNameOverride: "sword",
					Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
					Metadata:            model.Metadata{Tags: []string{"OINVIS"}},
				},
			},
			objectProps: make(map[model.ObjectInstanceID]map[string]string),
		}
	}

	t.Run("without PDINVI", func(t *testing.T) {
		world := baseWorld(nil)
		ctx := &Context{ActorID: "player:alice"}
		status, err := NewDMObjNameHandler(world)(ctx, ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname sword Shadowfang",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Fatalf("status = %v, want StatusPrompt", status)
		}
		if got := ctx.OutputString(); got != "그런 아이템은 없어요." {
			t.Fatalf("output = %q, want not found", got)
		}
		if props := world.objectProps["obj:hidden-sword"]; len(props) != 0 {
			t.Fatalf("updated invisible object without PDINVI: %+v", props)
		}
	})

	t.Run("with PDINVI", func(t *testing.T) {
		world := baseWorld([]string{"PDINVI"})
		ctx := &Context{ActorID: "player:alice"}
		status, err := NewDMObjNameHandler(world)(ctx, ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "*oname"},
			Input: "*oname sword Shadowfang",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Fatalf("status = %v, want StatusDefault", status)
		}
		if got := world.objectProps["obj:hidden-sword"]["name"]; got != "Shadowfang" {
			t.Fatalf("name = %q, want Shadowfang", got)
		}
	})
}
