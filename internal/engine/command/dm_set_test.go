package command

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type linkedExitCall struct {
	FromRoomID   model.RoomID
	ToRoomID     model.RoomID
	ExitName     string
	OppositeName string
	DoubleWay    bool
}

type deletedExitCall struct {
	RoomID   model.RoomID
	ExitName string
}

type exitFlagCall struct {
	RoomID   model.RoomID
	ExitName string
	Flag     string
	Enabled  bool
}

func TestDMSetCreatureFlagListCoversLegacyTailFlags(t *testing.T) {
	if len(creatureFlagNamesList) != 63 {
		t.Fatalf("creature flag list len = %d, want 63", len(creatureFlagNamesList))
	}
	for idx, want := range map[int]string{
		57: "king2",
		58: "king3",
		59: "king4",
		60: "sayTalk",
		61: "summoner",
		62: "noCharm",
	} {
		if got := creatureFlagNamesList[idx]; got != want {
			t.Fatalf("creatureFlagNamesList[%d] = %q, want %q", idx, got, want)
		}
	}
}

func TestDMSetObjectFlagListCoversLegacyTailFlags(t *testing.T) {
	if len(objectFlagNamesList) != 50 {
		t.Fatalf("object flag list len = %d, want 50", len(objectFlagNamesList))
	}
	for idx, want := range map[int]string{
		32: "classAssassin",
		39: "classThief",
		43: "customName",
		44: "specialItem",
		46: "eventItem",
		49: "held",
	} {
		if got := objectFlagNamesList[idx]; got != want {
			t.Fatalf("objectFlagNamesList[%d] = %q, want %q", idx, got, want)
		}
	}
}

type mockDMSetWorld struct {
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
	rooms     map[model.RoomID]model.Room
	objects   map[model.ObjectInstanceID]model.ObjectInstance

	// Spies
	roomProps     map[model.RoomID]map[string]string
	roomRandoms   map[model.RoomID]map[int]int
	roomFlags     map[model.RoomID]map[int]bool
	creatureStats map[model.CreatureID]map[string]int
	creatureProps map[model.CreatureID]map[string]string
	objectProps   map[model.ObjectInstanceID]map[string]string
	linkedExits   []linkedExitCall
	deletedExits  []deletedExitCall
	exitFlagsSet  []exitFlagCall
}

func (w *mockDMSetWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMSetWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMSetWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *mockDMSetWorld) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	o, ok := w.objects[id]
	return o, ok
}

func (w *mockDMSetWorld) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	return model.ObjectPrototype{}, false
}

func (w *mockDMSetWorld) FindPlayerByName(name string) (model.Player, bool) {
	for _, p := range w.players {
		if strings.EqualFold(p.DisplayName, name) {
			return p, true
		}
	}
	return model.Player{}, false
}

func (w *mockDMSetWorld) FindCreatureInRoom(roomID model.RoomID, name string) (model.Creature, bool) {
	for _, c := range w.creatures {
		if c.RoomID == roomID && strings.EqualFold(c.DisplayName, name) {
			return c, true
		}
	}
	return model.Creature{}, false
}

func (w *mockDMSetWorld) FindObjectInRoom(roomID model.RoomID, name string) (model.ObjectInstance, bool) {
	for _, o := range w.objects {
		if o.Location.RoomID == roomID && strings.EqualFold(o.DisplayNameOverride, name) {
			return o, true
		}
	}
	return model.ObjectInstance{}, false
}

func (w *mockDMSetWorld) FindObjectOnCreature(creatureID model.CreatureID, name string) (model.ObjectInstance, bool) {
	for _, o := range w.objects {
		if o.Location.CreatureID == creatureID && strings.EqualFold(o.DisplayNameOverride, name) {
			return o, true
		}
	}
	return model.ObjectInstance{}, false
}

func (w *mockDMSetWorld) UpdateRoomProperty(roomID model.RoomID, key string, val string) error {
	if w.roomProps[roomID] == nil {
		w.roomProps[roomID] = make(map[string]string)
	}
	w.roomProps[roomID][key] = val
	return nil
}

func (w *mockDMSetWorld) UpdateRoomRandomCreature(roomID model.RoomID, index int, val int) error {
	if w.roomRandoms[roomID] == nil {
		w.roomRandoms[roomID] = make(map[int]int)
	}
	w.roomRandoms[roomID][index] = val
	return nil
}

func (w *mockDMSetWorld) UpdateRoomFlag(roomID model.RoomID, flag int, val bool) error {
	if w.roomFlags[roomID] == nil {
		w.roomFlags[roomID] = make(map[int]bool)
	}
	w.roomFlags[roomID][flag] = val
	return nil
}

func (w *mockDMSetWorld) UpdateCreatureStat(creatureID model.CreatureID, key string, val int) error {
	if w.creatureStats[creatureID] == nil {
		w.creatureStats[creatureID] = make(map[string]int)
	}
	w.creatureStats[creatureID][key] = val
	return nil
}

func (w *mockDMSetWorld) UpdateCreatureProperty(creatureID model.CreatureID, key string, val string) error {
	if w.creatureProps[creatureID] == nil {
		w.creatureProps[creatureID] = make(map[string]string)
	}
	w.creatureProps[creatureID][key] = val
	return nil
}

func (w *mockDMSetWorld) UpdateObjectProperty(objectID model.ObjectInstanceID, key string, val string) error {
	if w.objectProps[objectID] == nil {
		w.objectProps[objectID] = make(map[string]string)
	}
	if val == "" {
		delete(w.objectProps[objectID], key)
	} else {
		w.objectProps[objectID][key] = val
	}
	obj := w.objects[objectID]
	if obj.Properties == nil {
		obj.Properties = map[string]string{}
	}
	if val == "" {
		delete(obj.Properties, key)
	} else {
		obj.Properties[key] = val
	}
	w.objects[objectID] = obj
	return nil
}

func (w *mockDMSetWorld) UpdateObjectTags(objectID model.ObjectInstanceID, add []string, remove []string) (model.ObjectInstance, error) {
	obj, ok := w.objects[objectID]
	if !ok {
		return model.ObjectInstance{}, errors.New("object not found")
	}
	tagSet := make(map[string]struct{})
	for _, tag := range obj.Metadata.Tags {
		tagSet[tag] = struct{}{}
	}
	for _, tag := range remove {
		for existing := range tagSet {
			if normalizeFlagName(existing) == normalizeFlagName(tag) {
				delete(tagSet, existing)
			}
		}
	}
	for _, tag := range add {
		tagSet[tag] = struct{}{}
	}
	obj.Metadata.Tags = obj.Metadata.Tags[:0]
	for tag := range tagSet {
		obj.Metadata.Tags = append(obj.Metadata.Tags, tag)
	}
	w.objects[objectID] = obj
	return obj, nil
}

func (w *mockDMSetWorld) LinkExits(fromRoomID, toRoomID model.RoomID, exitName, oppositeName string, doubleWay bool) error {
	w.linkedExits = append(w.linkedExits, linkedExitCall{
		FromRoomID:   fromRoomID,
		ToRoomID:     toRoomID,
		ExitName:     exitName,
		OppositeName: oppositeName,
		DoubleWay:    doubleWay,
	})
	return nil
}

func (w *mockDMSetWorld) DeleteRoomExit(roomID model.RoomID, exitName string) error {
	w.deletedExits = append(w.deletedExits, deletedExitCall{
		RoomID:   roomID,
		ExitName: exitName,
	})
	return nil
}

func (w *mockDMSetWorld) SetExitFlag(roomID model.RoomID, exitName string, flag string, enabled bool) (model.Exit, error) {
	w.exitFlagsSet = append(w.exitFlagsSet, exitFlagCall{
		RoomID:   roomID,
		ExitName: exitName,
		Flag:     flag,
		Enabled:  enabled,
	})
	return model.Exit{Name: exitName}, nil
}

func TestDMSetPermissionAndBasics(t *testing.T) {
	world := &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}}, // SUB_DM (12)
		},
	}

	handler := NewDMSetHandler(world)
	ctx := &Context{
		ActorID: "player:alice",
	}

	// 1. Permission Denied for class < 13
	resolved := ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"r", "t", "50"},
		Values: []int64{1, 50},
		Parsed: commandparse.Command{
			Num: 3,
			Str: [commandparse.CommandMax]string{"*set", "r", "t"},
			Val: [commandparse.CommandMax]int64{1, 1, 50},
		},
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

	// 2. Set What message
	ctx = &Context{ActorID: "player:alice"}
	world.creatures["creature:alice"] = model.Creature{
		ID:    "creature:alice",
		Stats: map[string]int{"class": 13}, // DM (13)
	}
	resolvedEmpty := ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{},
		Parsed: commandparse.Command{Num: 1, Str: [commandparse.CommandMax]string{"*set"}},
	}
	_, err = handler(ctx, resolvedEmpty)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.OutputString() != "Set what?\n" {
		t.Errorf("expected Set what? message, got %q", ctx.OutputString())
	}
}

func TestDMSetDispatchUsesLegacyCategoryFirstCharacter(t *testing.T) {
	world := dmSetRoomTestWorld()
	ctx := &Context{ActorID: "player:alice"}
	resolved := dmSetRoomCommand("t", 42)
	resolved.Args[0] = "room"
	resolved.Parsed.Str[1] = "room"

	_, err := NewDMSetHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val := world.roomProps["room:00001"]["traffic"]; val != "42" {
		t.Fatalf("traffic = %q, want 42", val)
	}
	if got := ctx.OutputString(); got != "Traffic is now 42%.\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestDMSetUsesParsedSlotsLikeCWhenArgsMissing(t *testing.T) {
	world := dmSetRoomTestWorld()
	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "dm_set"},
		Parsed: commandparse.Command{
			Num: 3,
			Str: [commandparse.CommandMax]string{"*set", "r", "t"},
			Val: [commandparse.CommandMax]int64{1, 1, 42},
		},
	}

	_, err := NewDMSetHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val := world.roomProps["room:00001"]["traffic"]; val != "42" {
		t.Fatalf("traffic = %q, want 42", val)
	}
	if got, want := ctx.OutputString(), "Traffic is now 42%.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDMSetInvalidCategoryUsesLegacyMessage(t *testing.T) {
	world := dmSetRoomTestWorld()
	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "dm_set"},
		Args: []string{"z"},
		Parsed: commandparse.Command{
			Num: 2,
			Str: [commandparse.CommandMax]string{"*set", "z"},
			Val: [commandparse.CommandMax]int64{1, 1},
		},
	}

	_, err := NewDMSetHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := ctx.OutputString(); got != "Invalid option.  *set <x|r|c|o> <options>\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestDMSetRoom(t *testing.T) {
	world := &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:00001"},
		},
		rooms: map[model.RoomID]model.Room{
			"room:00001": {ID: "room:00001", Properties: map[string]string{}},
		},
		roomProps:   make(map[model.RoomID]map[string]string),
		roomRandoms: make(map[model.RoomID]map[int]int),
		roomFlags:   make(map[model.RoomID]map[int]bool),
	}

	handler := NewDMSetHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	// *set r t 75 -> sets traffic to 75%
	resolved := ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"r", "t"},
		Values: []int64{1, 75},
		Parsed: commandparse.Command{
			Num: 3,
			Str: [commandparse.CommandMax]string{"*set", "r", "t"},
			Val: [commandparse.CommandMax]int64{1, 1, 75},
		},
	}

	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if val := world.roomProps["room:00001"]["traffic"]; val != "75" {
		t.Errorf("traffic = %q, want 75", val)
	}
	if ctx.OutputString() != "Traffic is now 75%.\n" {
		t.Errorf("unexpected output: %q", ctx.OutputString())
	}

	// *set r r2 10 -> sets random index 1 (r2 is 2-nd, 0-indexed index 1) to 10
	ctx = &Context{ActorID: "player:alice"}
	resolvedRand := ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"r", "r2"},
		Values: []int64{1, 10},
		Parsed: commandparse.Command{
			Num: 3,
			Str: [commandparse.CommandMax]string{"*set", "r", "r2"},
			Val: [commandparse.CommandMax]int64{1, 1, 10},
		},
	}
	_, err = handler(ctx, resolvedRand)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val := world.roomRandoms["room:00001"][1]; val != 10 {
		t.Errorf("random 1 = %d, want 10", val)
	}
	if ctx.OutputString() != "Random #2 is now 10.\n" {
		t.Errorf("unexpected output: %q", ctx.OutputString())
	}
}

func TestDMSetRoomFlagClearsPropertyBackedFlagLikeC(t *testing.T) {
	world := dmSetRoomTestWorld()
	room := world.rooms["room:00001"]
	room.Properties["RSHOPP"] = "1"
	world.rooms["room:00001"] = room

	ctx := &Context{ActorID: "player:alice"}
	_, err := NewDMSetHandler(world)(ctx, dmSetRoomCommand("f", 1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := world.roomFlags["room:00001"][1]; got {
		t.Fatalf("room flag 1 enabled = %v, want false", got)
	}
	if got := ctx.OutputString(); got != "Room flag #1 off.\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestDMSetRoomInvalidRangeReturnsPromptSilentlyLikeLegacy(t *testing.T) {
	tests := []ResolvedCommand{
		dmSetRoomCommand("r", 10),
		dmSetRoomCommand("r0", 10),
		dmSetRoomCommand("r11", 10),
		dmSetRoomCommand("f", 0),
		dmSetRoomCommand("f", 65),
		dmSetRoomCommand("b", 10),
		dmSetRoomCommand("bz", 10),
	}

	for _, resolved := range tests {
		t.Run(resolved.Args[1], func(t *testing.T) {
			world := dmSetRoomTestWorld()
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewDMSetHandler(world)(ctx, resolved)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %v, want StatusDefault", status)
			}
			if got := ctx.OutputString(); got != "" {
				t.Fatalf("output = %q, want none", got)
			}
			if len(world.roomProps["room:00001"]) != 0 {
				t.Fatalf("room props mutated: %+v", world.roomProps["room:00001"])
			}
			if len(world.roomRandoms["room:00001"]) != 0 {
				t.Fatalf("room randoms mutated: %+v", world.roomRandoms["room:00001"])
			}
			if len(world.roomFlags["room:00001"]) != 0 {
				t.Fatalf("room flags mutated: %+v", world.roomFlags["room:00001"])
			}
		})
	}
}

func dmSetRoomTestWorld() *mockDMSetWorld {
	return &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, RoomID: "room:00001"},
		},
		rooms: map[model.RoomID]model.Room{
			"room:00001": {ID: "room:00001", Properties: map[string]string{}},
		},
		roomProps:   make(map[model.RoomID]map[string]string),
		roomRandoms: make(map[model.RoomID]map[int]int),
		roomFlags:   make(map[model.RoomID]map[int]bool),
	}
}

func dmSetRoomCommand(option string, value int64) ResolvedCommand {
	return ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"r", option},
		Values: []int64{1, value},
		Parsed: commandparse.Command{
			Num: 3,
			Str: [commandparse.CommandMax]string{"*set", "r", option},
			Val: [commandparse.CommandMax]int64{1, 1, value},
		},
	}
}

func TestDMSetCreature(t *testing.T) {
	world := &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", DisplayName: "bob", CreatureID: "creature:bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:00001"},
			"creature:bob":   {ID: "creature:bob", DisplayName: "bob", RoomID: "room:00001", Stats: map[string]int{"class": 1}},
		},
		creatureStats: make(map[model.CreatureID]map[string]int),
		creatureProps: make(map[model.CreatureID]map[string]string),
	}

	handler := NewDMSetHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	// *set c bob gold 500
	resolved := ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"c", "bob", "gold"},
		Values: []int64{1, 1, 500},
		Parsed: commandparse.Command{
			Num: 4,
			Str: [commandparse.CommandMax]string{"*set", "c", "bob", "gold"},
			Val: [commandparse.CommandMax]int64{1, 1, 1, 500},
		},
	}

	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if val := world.creatureStats["creature:bob"]["gold"]; val != 500 {
		t.Errorf("gold = %d, want 500", val)
	}
	if ctx.OutputString() != "bob has 500 gold.\n" {
		t.Errorf("unexpected output: %q", ctx.OutputString())
	}
}

func TestDMSetCreatureFlagTogglesLegacyStatLikeC(t *testing.T) {
	tests := []struct {
		name        string
		initialStat int
		wantStat    int
		wantProp    string
		wantOutput  string
	}{
		{
			name:       "set followDM on",
			wantStat:   1,
			wantProp:   "true",
			wantOutput: "goblin's flag #47 on.\n",
		},
		{
			name:        "clear stat-backed followDM",
			initialStat: 1,
			wantStat:    0,
			wantProp:    "false",
			wantOutput:  "goblin's flag #47 off.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMSetWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {
						ID:     "creature:alice",
						Stats:  map[string]int{"class": model.ClassDM},
						RoomID: "room:00001",
					},
					"creature:goblin": {
						ID:          "creature:goblin",
						DisplayName: "goblin",
						RoomID:      "room:00001",
						Stats:       map[string]int{"followDM": tt.initialStat},
					},
				},
				creatureStats: make(map[model.CreatureID]map[string]int),
				creatureProps: make(map[model.CreatureID]map[string]string),
			}
			ctx := &Context{ActorID: "player:alice"}

			_, err := NewDMSetHandler(world)(ctx, ResolvedCommand{
				Spec:   commandspec.CommandSpec{Name: "dm_set"},
				Args:   []string{"c", "goblin", "f"},
				Values: []int64{1, 1, 47},
				Parsed: commandparse.Command{
					Num: 4,
					Str: [commandparse.CommandMax]string{"*set", "c", "goblin", "f"},
					Val: [commandparse.CommandMax]int64{1, 1, 1, 47},
				},
			})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if got := world.creatureStats["creature:goblin"]["followDM"]; got != tt.wantStat {
				t.Fatalf("followDM stat = %d, want %d", got, tt.wantStat)
			}
			if got := world.creatureProps["creature:goblin"]["followDM"]; got != tt.wantProp {
				t.Fatalf("followDM property = %q, want %q", got, tt.wantProp)
			}
			if got := ctx.OutputString(); got != tt.wantOutput {
				t.Fatalf("output = %q, want %q", got, tt.wantOutput)
			}
		})
	}
}

func TestDMSetCreatureOnlinePlayerLookupUsesActiveSessions(t *testing.T) {
	world := &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, RoomID: "room:00001"},
			"creature:bob":   {ID: "creature:bob", DisplayName: "Bob", RoomID: "room:00002", Stats: map[string]int{"class": model.ClassFighter}},
		},
		creatureStats: make(map[model.CreatureID]map[string]int),
		creatureProps: make(map[model.CreatureID]map[string]string),
	}

	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{
					{ID: "session:bob", ActorID: "player:bob"},
				}
			},
		},
	}
	resolved := ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"c", "bOB", "gold"},
		Values: []int64{1, 1, 500},
		Parsed: commandparse.Command{
			Num: 4,
			Str: [commandparse.CommandMax]string{"*set", "c", "bOB", "gold"},
			Val: [commandparse.CommandMax]int64{1, 1, 1, 500},
		},
	}

	status, err := NewDMSetHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if val := world.creatureStats["creature:bob"]["gold"]; val != 500 {
		t.Errorf("gold = %d, want 500", val)
	}
	if ctx.OutputString() != "Bob has 500 gold.\n" {
		t.Errorf("unexpected output: %q", ctx.OutputString())
	}
}

func TestDMSetCreatureSavedPlayerWithoutActiveSessionIsNotFound(t *testing.T) {
	world := &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, RoomID: "room:00001"},
			"creature:bob":   {ID: "creature:bob", DisplayName: "Bob", RoomID: "room:00002", Stats: map[string]int{"class": model.ClassFighter}},
		},
		creatureStats: make(map[model.CreatureID]map[string]int),
		creatureProps: make(map[model.CreatureID]map[string]string),
	}

	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{
					{ID: "session:alice", ActorID: "player:alice"},
				}
			},
		},
	}
	resolved := ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"c", "Bob", "gold"},
		Values: []int64{1, 1, 500},
		Parsed: commandparse.Command{
			Num: 4,
			Str: [commandparse.CommandMax]string{"*set", "c", "Bob", "gold"},
			Val: [commandparse.CommandMax]int64{1, 1, 1, 500},
		},
	}

	status, err := NewDMSetHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if got, want := ctx.OutputString(), "Creature not found.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if len(world.creatureStats["creature:bob"]) != 0 {
		t.Fatalf("expected no Bob stat updates, got %v", world.creatureStats["creature:bob"])
	}
}

func TestDMSetCreatureRoomLookupPrefersPlayerBeforeMonsterLikeLegacy(t *testing.T) {
	world := &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice":      {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, RoomID: "room:00001"},
			"creature:bob-mon":    {ID: "creature:bob-mon", Kind: model.CreatureKindMonster, DisplayName: "Bob", RoomID: "room:00001"},
			"creature:bob-player": {ID: "creature:bob-player", Kind: model.CreatureKindPlayer, PlayerID: "player:bob", DisplayName: "Bob", RoomID: "room:00001"},
		},
		rooms: map[model.RoomID]model.Room{
			"room:00001": {ID: "room:00001", CreatureIDs: []model.CreatureID{"creature:bob-mon", "creature:bob-player"}},
		},
		creatureStats: make(map[model.CreatureID]map[string]int),
		creatureProps: make(map[model.CreatureID]map[string]string),
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewDMSetHandler(world)(ctx, dmSetCreatureGoldCommand("Bob", 1, 600))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if val := world.creatureStats["creature:bob-player"]["gold"]; val != 600 {
		t.Fatalf("player gold = %d, want 600", val)
	}
	if val := world.creatureStats["creature:bob-mon"]["gold"]; val != 0 {
		t.Fatalf("monster gold = %d, want unchanged", val)
	}
	if got := ctx.OutputString(); got != "Bob has 600 gold.\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestDMSetCreatureRoomLookupFallsBackToMonsterOrdinalLikeLegacy(t *testing.T) {
	world := &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice":      {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, RoomID: "room:00001"},
			"creature:bob-player": {ID: "creature:bob-player", Kind: model.CreatureKindPlayer, PlayerID: "player:bob", DisplayName: "Bob", RoomID: "room:00001"},
			"creature:bob-mon-1":  {ID: "creature:bob-mon-1", Kind: model.CreatureKindMonster, DisplayName: "Bob", RoomID: "room:00001"},
			"creature:bob-mon-2":  {ID: "creature:bob-mon-2", Kind: model.CreatureKindMonster, DisplayName: "Bob", RoomID: "room:00001"},
		},
		rooms: map[model.RoomID]model.Room{
			"room:00001": {ID: "room:00001", CreatureIDs: []model.CreatureID{"creature:bob-player", "creature:bob-mon-1", "creature:bob-mon-2"}},
		},
		creatureStats: make(map[model.CreatureID]map[string]int),
		creatureProps: make(map[model.CreatureID]map[string]string),
	}

	ctx := &Context{ActorID: "player:alice"}
	_, err := NewDMSetHandler(world)(ctx, dmSetCreatureGoldCommand("Bob", 2, 700))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val := world.creatureStats["creature:bob-mon-2"]["gold"]; val != 700 {
		t.Fatalf("second monster gold = %d, want 700", val)
	}
	if val := world.creatureStats["creature:bob-player"]["gold"]; val != 0 {
		t.Fatalf("player gold = %d, want unchanged", val)
	}
	if val := world.creatureStats["creature:bob-mon-1"]["gold"]; val != 0 {
		t.Fatalf("first monster gold = %d, want unchanged", val)
	}
}

func TestDMSetCreatureRoomLookupSkipsFindCrtInvisibleTargetsLikeLegacy(t *testing.T) {
	t.Run("caretaker PDMINV is skipped", func(t *testing.T) {
		world := &mockDMSetWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, RoomID: "room:00001"},
				"creature:ghost": {
					ID:          "creature:ghost",
					Kind:        model.CreatureKindPlayer,
					PlayerID:    "player:ghost",
					DisplayName: "Ghost",
					RoomID:      "room:00001",
					Stats:       map[string]int{"class": model.ClassCaretaker},
					Metadata:    model.Metadata{Tags: []string{"PDMINV"}},
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:00001": {ID: "room:00001", CreatureIDs: []model.CreatureID{"creature:ghost"}},
			},
			creatureStats: make(map[model.CreatureID]map[string]int),
			creatureProps: make(map[model.CreatureID]map[string]string),
		}

		ctx := &Context{ActorID: "player:alice"}
		_, err := NewDMSetHandler(world)(ctx, dmSetCreatureGoldCommand("Ghost", 1, 800))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := ctx.OutputString(); got != "Creature not found.\n" {
			t.Fatalf("output = %q, want Creature not found", got)
		}
		if val := world.creatureStats["creature:ghost"]["gold"]; val != 0 {
			t.Fatalf("ghost gold = %d, want unchanged", val)
		}
	})

	t.Run("MINVIS requires PDINVI", func(t *testing.T) {
		world := &mockDMSetWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, RoomID: "room:00001"},
				"creature:shade": {
					ID:          "creature:shade",
					Kind:        model.CreatureKindMonster,
					DisplayName: "Shade",
					RoomID:      "room:00001",
					Metadata:    model.Metadata{Tags: []string{"MINVIS"}},
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:00001": {ID: "room:00001", CreatureIDs: []model.CreatureID{"creature:shade"}},
			},
			creatureStats: make(map[model.CreatureID]map[string]int),
			creatureProps: make(map[model.CreatureID]map[string]string),
		}

		ctx := &Context{ActorID: "player:alice"}
		_, err := NewDMSetHandler(world)(ctx, dmSetCreatureGoldCommand("Shade", 1, 900))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := ctx.OutputString(); got != "Creature not found.\n" {
			t.Fatalf("output = %q, want Creature not found", got)
		}
		if val := world.creatureStats["creature:shade"]["gold"]; val != 0 {
			t.Fatalf("shade gold = %d, want unchanged", val)
		}

		alice := world.creatures["creature:alice"]
		alice.Metadata.Tags = []string{"PDINVI"}
		world.creatures[alice.ID] = alice
		world.creatureStats = make(map[model.CreatureID]map[string]int)
		ctx = &Context{ActorID: "player:alice"}
		_, err = NewDMSetHandler(world)(ctx, dmSetCreatureGoldCommand("Shade", 1, 900))
		if err != nil {
			t.Fatalf("unexpected error with PDINVI: %v", err)
		}
		if val := world.creatureStats["creature:shade"]["gold"]; val != 900 {
			t.Fatalf("shade gold with PDINVI = %d, want 900", val)
		}
	})
}

func dmSetCreatureGoldCommand(name string, ordinal, gold int64) ResolvedCommand {
	return ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"c", name, "gold"},
		Values: []int64{1, ordinal, gold},
		Parsed: commandparse.Command{
			Num: 4,
			Str: [commandparse.CommandMax]string{"*set", "c", name, "gold"},
			Val: [commandparse.CommandMax]int64{1, 1, ordinal, gold},
		},
	}
}

func TestDMSetObject(t *testing.T) {
	world := &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:00001"},
		},
		rooms: map[model.RoomID]model.Room{
			"room:00001": {
				ID:      "room:00001",
				Objects: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:sword"}},
			},
		},
		objects: map[model.ObjectInstanceID]model.ObjectInstance{
			"object:sword": {
				ID:                  "object:sword",
				DisplayNameOverride: "sword",
				Location:            model.ObjectLocation{RoomID: "room:00001"},
				Properties:          map[string]string{},
			},
		},
		objectProps: make(map[model.ObjectInstanceID]map[string]string),
	}

	handler := NewDMSetHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	// C checks only flags[0] for v, so val0 still sets object value.
	resolved := ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"o", "sword", "val0"},
		Values: []int64{1, 1, 99},
		Parsed: commandparse.Command{
			Num: 4,
			Str: [commandparse.CommandMax]string{"*set", "o", "sword", "val0"},
			Val: [commandparse.CommandMax]int64{1, 1, 1, 99},
		},
	}

	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if val := world.objectProps["object:sword"]["value"]; val != "99" {
		t.Errorf("value = %q, want 99", val)
	}
	if ctx.OutputString() != "Value set.\n" {
		t.Errorf("unexpected output: %q", ctx.OutputString())
	}
}

func TestDMSetObjectSupportsLegacyCOptions(t *testing.T) {
	tests := []struct {
		name   string
		option string
		value  int64
		key    string
		want   string
		output string
	}{
		{name: "value", option: "v", value: 123, key: "value", want: "123", output: "Value set.\n"},
		{name: "adjustment", option: "ad", value: 4, key: "adjustment", want: "4", output: "Adjustment set.\n"},
		{name: "armor", option: "ar", value: 9, key: "armor", want: "9", output: "Armor set.\n"},
		{name: "dice number", option: "dn", value: 2, key: "nDice", want: "2", output: "Dice # set.\n"},
		{name: "dice sides", option: "ds", value: 8, key: "sDice", want: "8", output: "Dice sides set.\n"},
		{name: "dice plus", option: "dp", value: 3, key: "pDice", want: "3", output: "Dice plus set.\n"},
		{name: "magic power", option: "m", value: 17, key: "magicPower", want: "17", output: "Magic power set.\n"},
		{name: "shots current", option: "s", value: 5, key: "shotsCurrent", want: "5", output: "Current shots set.\n"},
		{name: "shots max", option: "sm", value: 7, key: "shotsMax", want: "7", output: "Max shots set.\n"},
		{name: "flag", option: "f", value: 1, key: "permanent", want: "true", output: "sword's flag #1 on.\n"},
		{name: "type", option: "t", value: 2, key: "type", want: "2", output: "Object is a blunt weapon.\n"},
		{name: "weight", option: "wg", value: 11, key: "weight", want: "11", output: "Weight set.\n"},
		{name: "wear flag", option: "wr", value: 6, key: "wearFlag", want: "6", output: "Wear location set.\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := dmSetObjectTestWorld()
			ctx := &Context{ActorID: "player:alice"}
			_, err := NewDMSetHandler(world)(ctx, dmSetObjectCommand("sword", tt.option, tt.value))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := world.objectProps["object:sword"][tt.key]; got != tt.want {
				t.Fatalf("%s = %q, want %q", tt.key, got, tt.want)
			}
			if got := ctx.OutputString(); got != tt.output {
				t.Fatalf("output = %q, want %q", got, tt.output)
			}
		})
	}
}

func TestDMSetObjectFlagClearsTagBackedFlagLikeC(t *testing.T) {
	world := dmSetObjectTestWorld()
	object := world.objects["object:sword"]
	object.Metadata.Tags = []string{"permanent"}
	world.objects[object.ID] = object
	ctx := &Context{ActorID: "player:alice"}

	_, err := NewDMSetHandler(world)(ctx, dmSetObjectCommand("sword", "f", 1))
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}

	updated := world.objects["object:sword"]
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "permanent") || updated.Properties["permanent"] != "false" {
		t.Fatalf("object flag still enabled: tags=%+v props=%+v", updated.Metadata.Tags, updated.Properties)
	}
	if got, want := ctx.OutputString(), "sword's flag #1 off.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDMSetObjectFlagClearsLegacyAliasBackedFlagLikeC(t *testing.T) {
	world := dmSetObjectTestWorld()
	object := world.objects["object:sword"]
	object.Metadata.Tags = []string{"OPERMT", "other"}
	object.Properties = map[string]string{"OPERMT": "1", "other": "true"}
	world.objects[object.ID] = object
	ctx := &Context{ActorID: "player:alice"}

	_, err := NewDMSetHandler(world)(ctx, dmSetObjectCommand("sword", "f", 1))
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}

	updated := world.objects["object:sword"]
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "OPERMT", "permanent") ||
		updated.Properties["OPERMT"] != "" ||
		updated.Properties["permanent"] != "false" {
		t.Fatalf("object flag still enabled: tags=%+v props=%+v", updated.Metadata.Tags, updated.Properties)
	}
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "other") || updated.Properties["other"] != "true" {
		t.Fatalf("unrelated object state changed: tags=%+v props=%+v", updated.Metadata.Tags, updated.Properties)
	}
	if got, want := ctx.OutputString(), "sword's flag #1 off.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDMSetObjectFlagClearsFlagsContainerTokenLikeC(t *testing.T) {
	world := dmSetObjectTestWorld()
	object := world.objects["object:sword"]
	object.Properties = map[string]string{"flags": "permanent,hidden"}
	world.objects[object.ID] = object
	ctx := &Context{ActorID: "player:alice"}

	_, err := NewDMSetHandler(world)(ctx, dmSetObjectCommand("sword", "f", 1))
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}

	updated := world.objects["object:sword"]
	if hasObjectFlag(updated, "permanent") {
		t.Fatalf("object permanent flag still enabled: props=%+v tags=%+v", updated.Properties, updated.Metadata.Tags)
	}
	if got := updated.Properties["flags"]; got != "hidden" {
		t.Fatalf("flags property = %q, want hidden; props=%+v", got, updated.Properties)
	}
	if updated.Properties["permanent"] != "false" {
		t.Fatalf("permanent property = %q, want false; props=%+v", updated.Properties["permanent"], updated.Properties)
	}
	if got, want := ctx.OutputString(), "sword's flag #1 off.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDMSetObjectFlagUsesLegacyTailFlagNameLikeC(t *testing.T) {
	world := dmSetObjectTestWorld()
	ctx := &Context{ActorID: "player:alice"}

	_, err := NewDMSetHandler(world)(ctx, dmSetObjectCommand("sword", "f", 33))
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}

	updated := world.objects["object:sword"]
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "classAssassin", "OASSNO") ||
		updated.Properties["classAssassin"] != "true" {
		t.Fatalf("tail object flag not enabled through legacy name: tags=%+v props=%+v", updated.Metadata.Tags, updated.Properties)
	}
	if got, want := ctx.OutputString(), "sword's flag #33 on.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDMSetObjectRejectsFormerGoOnlyOptionsLikeLegacy(t *testing.T) {
	tests := []string{"name", "dm1"}
	for _, option := range tests {
		t.Run(option, func(t *testing.T) {
			world := dmSetObjectTestWorld()
			ctx := &Context{ActorID: "player:alice"}
			_, err := NewDMSetHandler(world)(ctx, dmSetObjectCommand("sword", option, 9))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := ctx.OutputString(); got != "Invalid option.\n" {
				t.Fatalf("output = %q, want Invalid option", got)
			}
			if props := world.objectProps["object:sword"]; len(props) != 0 {
				t.Fatalf("unexpected object props for %s: %+v", option, props)
			}
		})
	}
}

func dmSetObjectTestWorld() *mockDMSetWorld {
	return &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, RoomID: "room:00001"},
		},
		rooms: map[model.RoomID]model.Room{
			"room:00001": {
				ID:      "room:00001",
				Objects: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:sword"}},
			},
		},
		objects: map[model.ObjectInstanceID]model.ObjectInstance{
			"object:sword": {
				ID:                  "object:sword",
				DisplayNameOverride: "sword",
				Location:            model.ObjectLocation{RoomID: "room:00001"},
				Properties:          map[string]string{},
			},
		},
		objectProps: make(map[model.ObjectInstanceID]map[string]string),
	}
}

func dmSetObjectCommand(objectName, option string, value int64) ResolvedCommand {
	return ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"o", objectName, option},
		Values: []int64{1, 1, value},
		Parsed: commandparse.Command{
			Num: 4,
			Str: [commandparse.CommandMax]string{"*set", "o", objectName, option},
			Val: [commandparse.CommandMax]int64{1, 1, 1, value},
		},
	}
}

func TestDMSetObjectPrefersActorInventoryBeforeRoomLikeLegacy(t *testing.T) {
	world := &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:        "creature:alice",
				Stats:     map[string]int{"class": model.ClassDM},
				RoomID:    "room:00001",
				Inventory: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:inventory-sword"}},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:00001": {
				ID:      "room:00001",
				Objects: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:room-sword"}},
			},
		},
		objects: map[model.ObjectInstanceID]model.ObjectInstance{
			"object:inventory-sword": {
				ID:                  "object:inventory-sword",
				DisplayNameOverride: "sword",
				Location:            model.ObjectLocation{CreatureID: "creature:alice"},
				Properties:          map[string]string{},
			},
			"object:room-sword": {
				ID:                  "object:room-sword",
				DisplayNameOverride: "sword",
				Location:            model.ObjectLocation{RoomID: "room:00001"},
				Properties:          map[string]string{},
			},
		},
		objectProps: make(map[model.ObjectInstanceID]map[string]string),
	}

	ctx := &Context{ActorID: "player:alice"}
	_, err := NewDMSetHandler(world)(ctx, ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"o", "sword", "val0"},
		Values: []int64{1, 1, 42},
		Parsed: commandparse.Command{
			Num: 4,
			Str: [commandparse.CommandMax]string{"*set", "o", "sword", "val0"},
			Val: [commandparse.CommandMax]int64{1, 1, 1, 42},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val := world.objectProps["object:inventory-sword"]["value"]; val != "42" {
		t.Fatalf("inventory value = %q, want 42", val)
	}
	if val := world.objectProps["object:room-sword"]["value"]; val != "" {
		t.Fatalf("room value = %q, want unchanged", val)
	}
}

func TestDMSetObjectExplicitCreatureTargetLikeLegacy(t *testing.T) {
	world := &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, RoomID: "room:00001"},
			"creature:guard": {
				ID:          "creature:guard",
				Kind:        model.CreatureKindMonster,
				DisplayName: "guard",
				RoomID:      "room:00001",
				Inventory:   model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:guard-sword"}},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:00001": {ID: "room:00001", CreatureIDs: []model.CreatureID{"creature:guard"}},
		},
		objects: map[model.ObjectInstanceID]model.ObjectInstance{
			"object:guard-sword": {
				ID:                  "object:guard-sword",
				DisplayNameOverride: "sword",
				Location:            model.ObjectLocation{CreatureID: "creature:guard"},
				Properties:          map[string]string{},
			},
		},
		objectProps: make(map[model.ObjectInstanceID]map[string]string),
	}

	ctx := &Context{ActorID: "player:alice"}
	_, err := NewDMSetHandler(world)(ctx, dmSetObjectExplicitCommand("sword", "guard", "val0", 77))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val := world.objectProps["object:guard-sword"]["value"]; val != "77" {
		t.Fatalf("guard sword value = %q, want 77", val)
	}
	if got := ctx.OutputString(); got != "Value set.\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestDMSetObjectExplicitCreatureTargetPrefersMonsterBeforeRoomPlayerLikeLegacy(t *testing.T) {
	world := &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, RoomID: "room:00001"},
			"creature:bob-player": {
				ID:          "creature:bob-player",
				Kind:        model.CreatureKindPlayer,
				PlayerID:    "player:bob",
				DisplayName: "Bob",
				RoomID:      "room:00001",
				Inventory:   model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:player-sword"}},
			},
			"creature:bob-mon": {
				ID:          "creature:bob-mon",
				Kind:        model.CreatureKindMonster,
				DisplayName: "Bob",
				RoomID:      "room:00001",
				Inventory:   model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:monster-sword"}},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:00001": {ID: "room:00001", CreatureIDs: []model.CreatureID{"creature:bob-player", "creature:bob-mon"}},
		},
		objects: map[model.ObjectInstanceID]model.ObjectInstance{
			"object:player-sword": {
				ID:                  "object:player-sword",
				DisplayNameOverride: "sword",
				Location:            model.ObjectLocation{CreatureID: "creature:bob-player"},
				Properties:          map[string]string{},
			},
			"object:monster-sword": {
				ID:                  "object:monster-sword",
				DisplayNameOverride: "sword",
				Location:            model.ObjectLocation{CreatureID: "creature:bob-mon"},
				Properties:          map[string]string{},
			},
		},
		objectProps: make(map[model.ObjectInstanceID]map[string]string),
	}

	ctx := &Context{ActorID: "player:alice"}
	_, err := NewDMSetHandler(world)(ctx, dmSetObjectExplicitCommand("sword", "Bob", "val0", 88))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val := world.objectProps["object:monster-sword"]["value"]; val != "88" {
		t.Fatalf("monster sword value = %q, want 88", val)
	}
	if val := world.objectProps["object:player-sword"]["value"]; val != "" {
		t.Fatalf("player sword value = %q, want unchanged", val)
	}
}

func TestDMSetObjectAppliesFindObjInvisibleVisibility(t *testing.T) {
	baseWorld := func(actorTags []string) *mockDMSetWorld {
		return &mockDMSetWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {
					ID:       "creature:alice",
					Stats:    map[string]int{"class": model.ClassDM},
					RoomID:   "room:00001",
					Metadata: model.Metadata{Tags: actorTags},
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:00001": {
					ID:      "room:00001",
					Objects: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:hidden-sword"}},
				},
			},
			objects: map[model.ObjectInstanceID]model.ObjectInstance{
				"object:hidden-sword": {
					ID:                  "object:hidden-sword",
					DisplayNameOverride: "sword",
					Location:            model.ObjectLocation{RoomID: "room:00001"},
					Metadata:            model.Metadata{Tags: []string{"OINVIS"}},
					Properties:          map[string]string{},
				},
			},
			objectProps: make(map[model.ObjectInstanceID]map[string]string),
		}
	}

	t.Run("without PDINVI", func(t *testing.T) {
		world := baseWorld(nil)
		ctx := &Context{ActorID: "player:alice"}
		_, err := NewDMSetHandler(world)(ctx, dmSetObjectCommand("sword", "val0", 55))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := ctx.OutputString(); got != "Object not found.\n" {
			t.Fatalf("output = %q, want Object not found", got)
		}
		if props := world.objectProps["object:hidden-sword"]; len(props) != 0 {
			t.Fatalf("updated invisible object without PDINVI: %+v", props)
		}
	})

	t.Run("with PDINVI", func(t *testing.T) {
		world := baseWorld([]string{"PDINVI"})
		ctx := &Context{ActorID: "player:alice"}
		_, err := NewDMSetHandler(world)(ctx, dmSetObjectCommand("sword", "val0", 55))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := world.objectProps["object:hidden-sword"]["value"]; got != "55" {
			t.Fatalf("value = %q, want 55", got)
		}
	})
}

func TestDMSetObjectExplicitCreatureTargetAppliesFindObjVisibility(t *testing.T) {
	baseWorld := func(actorTags []string) *mockDMSetWorld {
		return &mockDMSetWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {
					ID:       "creature:alice",
					Stats:    map[string]int{"class": model.ClassDM},
					RoomID:   "room:00001",
					Metadata: model.Metadata{Tags: actorTags},
				},
				"creature:guard": {
					ID:          "creature:guard",
					Kind:        model.CreatureKindMonster,
					DisplayName: "guard",
					RoomID:      "room:00001",
					Inventory:   model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:guard-hidden-sword"}},
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:00001": {ID: "room:00001", CreatureIDs: []model.CreatureID{"creature:guard"}},
			},
			objects: map[model.ObjectInstanceID]model.ObjectInstance{
				"object:guard-hidden-sword": {
					ID:                  "object:guard-hidden-sword",
					DisplayNameOverride: "sword",
					Location:            model.ObjectLocation{CreatureID: "creature:guard"},
					Metadata:            model.Metadata{Tags: []string{"OINVIS"}},
					Properties:          map[string]string{},
				},
			},
			objectProps: make(map[model.ObjectInstanceID]map[string]string),
		}
	}

	t.Run("without PDINVI", func(t *testing.T) {
		world := baseWorld(nil)
		ctx := &Context{ActorID: "player:alice"}
		_, err := NewDMSetHandler(world)(ctx, dmSetObjectExplicitCommand("sword", "guard", "val0", 66))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := ctx.OutputString(); got != "Object not found.\n" {
			t.Fatalf("output = %q, want Object not found", got)
		}
	})

	t.Run("with PDINVI", func(t *testing.T) {
		world := baseWorld([]string{"PDINVI"})
		ctx := &Context{ActorID: "player:alice"}
		_, err := NewDMSetHandler(world)(ctx, dmSetObjectExplicitCommand("sword", "guard", "val0", 66))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := world.objectProps["object:guard-hidden-sword"]["value"]; got != "66" {
			t.Fatalf("value = %q, want 66", got)
		}
	})
}

func TestDMSetObjectEquipmentLookupMatchesLegacyContexts(t *testing.T) {
	t.Run("actor equipment is not searched without explicit creature target", func(t *testing.T) {
		world := &mockDMSetWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {
					ID:        "creature:alice",
					Stats:     map[string]int{"class": model.ClassDM},
					RoomID:    "room:00001",
					Equipment: map[string]model.ObjectInstanceID{"held": "object:held-sword"},
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:00001": {ID: "room:00001"},
			},
			objects: map[model.ObjectInstanceID]model.ObjectInstance{
				"object:held-sword": {
					ID:                  "object:held-sword",
					DisplayNameOverride: "sword",
					Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "held"},
					Properties:          map[string]string{},
				},
			},
			objectProps: make(map[model.ObjectInstanceID]map[string]string),
		}

		ctx := &Context{ActorID: "player:alice"}
		_, err := NewDMSetHandler(world)(ctx, dmSetObjectCommand("sword", "val0", 70))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := ctx.OutputString(); got != "Object not found.\n" {
			t.Fatalf("output = %q, want Object not found", got)
		}
	})

	t.Run("explicit creature target uses ready fallback", func(t *testing.T) {
		world := &mockDMSetWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, RoomID: "room:00001"},
				"creature:guard": {
					ID:          "creature:guard",
					Kind:        model.CreatureKindMonster,
					DisplayName: "guard",
					RoomID:      "room:00001",
					Equipment:   map[string]model.ObjectInstanceID{"held": "object:guard-held-sword"},
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:00001": {ID: "room:00001", CreatureIDs: []model.CreatureID{"creature:guard"}},
			},
			objects: map[model.ObjectInstanceID]model.ObjectInstance{
				"object:guard-held-sword": {
					ID:                  "object:guard-held-sword",
					DisplayNameOverride: "sword",
					Location:            model.ObjectLocation{CreatureID: "creature:guard", Slot: "held"},
					Properties:          map[string]string{},
				},
			},
			objectProps: make(map[model.ObjectInstanceID]map[string]string),
		}

		ctx := &Context{ActorID: "player:alice"}
		_, err := NewDMSetHandler(world)(ctx, dmSetObjectExplicitCommand("sword", "guard", "val0", 71))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := world.objectProps["object:guard-held-sword"]["value"]; got != "71" {
			t.Fatalf("value = %q, want 71", got)
		}
	})
}

func dmSetObjectExplicitCommand(objectName, creatureName, option string, value int64) ResolvedCommand {
	return ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"o", objectName, creatureName, option},
		Values: []int64{1, 1, 1, value},
		Parsed: commandparse.Command{
			Num: 5,
			Str: [commandparse.CommandMax]string{"*set", "o", objectName, creatureName, option},
			Val: [commandparse.CommandMax]int64{1, 1, 1, 1, value},
		},
	}
}

func TestDMSetExit(t *testing.T) {
	world := &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, RoomID: "room:00001"},
		},
		rooms: map[model.RoomID]model.Room{
			"room:00001": {
				ID: "room:00001",
				Exits: []model.Exit{
					{Name: "북", ToRoomID: "room:00002"},
				},
				Properties: map[string]string{"roomNumber": "1"},
			},
			"room:00002": {
				ID:         "room:00002",
				Properties: map[string]string{"roomNumber": "2"},
			},
		},
	}

	handler := NewDMSetHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	// *set 1 x 북 2 .  -> Link exits both ways
	resolved := ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"x", "북", "."},
		Values: []int64{1, 1, 2},
		Parsed: commandparse.Command{
			Num: 4,
			Str: [commandparse.CommandMax]string{"*set", "x", "북", "."},
			Val: [commandparse.CommandMax]int64{1, 1, 2, 1},
		},
	}

	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(world.linkedExits) != 2 {
		t.Fatalf("expected 2 LinkExits calls, got %d", len(world.linkedExits))
	}

	forward := world.linkedExits[0]
	if forward.FromRoomID != "room:00001" || forward.ToRoomID != "room:00002" || forward.ExitName != "북" || forward.OppositeName != "" || forward.DoubleWay {
		t.Errorf("unexpected forward link call: %+v", forward)
	}
	reverse := world.linkedExits[1]
	if reverse.FromRoomID != "room:00002" || reverse.ToRoomID != "room:00001" || reverse.ExitName != "남" || reverse.OppositeName != "" || reverse.DoubleWay {
		t.Errorf("unexpected reverse link call: %+v", reverse)
	}

	if ctx.OutputString() != "Room #1 linked to room #2 in 북 direction, both ways.\n" {
		t.Errorf("unexpected output: %q", ctx.OutputString())
	}
}

func TestDMSetExitDoesNotRequireDestinationRoomLikeLegacy(t *testing.T) {
	world := &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, RoomID: "room:00001"},
		},
		rooms: map[model.RoomID]model.Room{
			"room:00001": {
				ID:         "room:00001",
				Properties: map[string]string{"roomNumber": "1"},
			},
		},
	}
	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"x", "북"},
		Values: []int64{1, 1, 999},
		Parsed: commandparse.Command{
			Num: 3,
			Str: [commandparse.CommandMax]string{"*set", "x", "북"},
			Val: [commandparse.CommandMax]int64{1, 1, 999},
		},
	}

	_, err := NewDMSetHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(world.linkedExits) != 1 {
		t.Fatalf("linkedExits len = %d, want 1", len(world.linkedExits))
	}
	call := world.linkedExits[0]
	if call.FromRoomID != "room:00001" || call.ToRoomID != "room:00999" || call.ExitName != "북" || call.DoubleWay {
		t.Fatalf("link call = %+v", call)
	}
	if got := ctx.OutputString(); got != "Room #1 linked to room #999 in 북 direction.\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestDMSetExitDoubleWayKeepsForwardLinkWhenReverseRoomMissingLikeLegacy(t *testing.T) {
	world := &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, RoomID: "room:00001"},
		},
		rooms: map[model.RoomID]model.Room{
			"room:00001": {
				ID:         "room:00001",
				Properties: map[string]string{"roomNumber": "1"},
			},
		},
	}
	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"x", "북", "."},
		Values: []int64{1, 1, 999, 1},
		Parsed: commandparse.Command{
			Num: 4,
			Str: [commandparse.CommandMax]string{"*set", "x", "북", "."},
			Val: [commandparse.CommandMax]int64{1, 1, 999, 1},
		},
	}

	_, err := NewDMSetHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(world.linkedExits) != 1 {
		t.Fatalf("linkedExits len = %d, want only forward link", len(world.linkedExits))
	}
	call := world.linkedExits[0]
	if call.FromRoomID != "room:00001" || call.ToRoomID != "room:00999" || call.ExitName != "북" || call.DoubleWay {
		t.Fatalf("forward link call = %+v", call)
	}
	want := "Room 1 does not exist.\nRoom #1 linked to room #999 in 북 direction, both ways.\n"
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDMSetExitExplicitReverseNameIsNotExpandedLikeLegacy(t *testing.T) {
	world := &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, RoomID: "room:00001"},
		},
		rooms: map[model.RoomID]model.Room{
			"room:00001": {
				ID:         "room:00001",
				Properties: map[string]string{"roomNumber": "1"},
			},
			"room:00002": {
				ID:         "room:00002",
				Properties: map[string]string{"roomNumber": "2"},
			},
		},
	}
	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"x", "n", "s"},
		Values: []int64{1, 1, 2, 1},
		Parsed: commandparse.Command{
			Num: 4,
			Str: [commandparse.CommandMax]string{"*set", "x", "n", "s"},
			Val: [commandparse.CommandMax]int64{1, 1, 2, 1},
		},
	}

	_, err := NewDMSetHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(world.linkedExits) != 2 {
		t.Fatalf("linkedExits len = %d, want 2", len(world.linkedExits))
	}
	reverse := world.linkedExits[1]
	if reverse.FromRoomID != "room:00002" || reverse.ToRoomID != "room:00001" || reverse.ExitName != "s" || reverse.DoubleWay {
		t.Fatalf("reverse link call = %+v", reverse)
	}
}

func TestDMSetExitFlagUsesLegacySecondCategoryCharacter(t *testing.T) {
	world := dmSetExitFlagWorld()
	ctx := &Context{ActorID: "player:alice"}
	resolved := dmSetExitFlagCommand("북", 1)
	resolved.Args[0] = "xflag"
	resolved.Parsed.Str[1] = "xflag"

	_, err := NewDMSetHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(world.exitFlagsSet) != 1 {
		t.Fatalf("exitFlagsSet len = %d, want 1", len(world.exitFlagsSet))
	}
	if got := ctx.OutputString(); got != "북 exit flag #1 on.\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestDMSetExitFlagUsesExactExitNameLikeLegacy(t *testing.T) {
	world := dmSetExitFlagWorld()
	ctx := &Context{ActorID: "player:alice"}

	_, err := NewDMSetHandler(world)(ctx, dmSetExitFlagCommand("북", 1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(world.exitFlagsSet) != 1 {
		t.Fatalf("exitFlagsSet len = %d, want 1", len(world.exitFlagsSet))
	}
	call := world.exitFlagsSet[0]
	if call.RoomID != "room:00001" || call.ExitName != "북" || call.Flag != "secret" || !call.Enabled {
		t.Fatalf("exit flag call = %+v", call)
	}
	if got := ctx.OutputString(); got != "북 exit flag #1 on.\n" {
		t.Fatalf("output = %q", got)
	}

	world = dmSetExitFlagWorld()
	ctx = &Context{ActorID: "player:alice"}
	_, err = NewDMSetHandler(world)(ctx, dmSetExitFlagCommand("n", 1))
	if err != nil {
		t.Fatalf("unexpected error for abbreviated exit: %v", err)
	}
	if len(world.exitFlagsSet) != 0 {
		t.Fatalf("abbreviated exit should not mutate flags: %+v", world.exitFlagsSet)
	}
	if got := ctx.OutputString(); got != "Exit not found.\n" {
		t.Fatalf("abbreviated output = %q", got)
	}
}

func TestDMSetExitFlagClearsLegacyAliasLikeC(t *testing.T) {
	world := dmSetExitFlagWorld()
	room := world.rooms["room:00001"]
	room.Exits[0].Flags = []string{"XSECRT"}
	world.rooms["room:00001"] = room
	ctx := &Context{ActorID: "player:alice"}

	_, err := NewDMSetHandler(world)(ctx, dmSetExitFlagCommand("북", 1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(world.exitFlagsSet) != 1 {
		t.Fatalf("exitFlagsSet len = %d, want 1", len(world.exitFlagsSet))
	}
	call := world.exitFlagsSet[0]
	if call.RoomID != "room:00001" || call.ExitName != "북" || call.Flag != "secret" || call.Enabled {
		t.Fatalf("exit flag call = %+v, want secret disabled", call)
	}
	if got := ctx.OutputString(); got != "북 exit flag #1 off.\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestDMSetExitFlagSkipsLegacyNoSeeExitLikeC(t *testing.T) {
	world := dmSetExitFlagWorld()
	room := world.rooms["room:00001"]
	room.Exits[0].Flags = []string{"XNOSEE"}
	world.rooms["room:00001"] = room
	ctx := &Context{ActorID: "player:alice"}

	_, err := NewDMSetHandler(world)(ctx, dmSetExitFlagCommand("북", 1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(world.exitFlagsSet) != 0 {
		t.Fatalf("XNOSEE exit should not mutate flags: %+v", world.exitFlagsSet)
	}
	if got := ctx.OutputString(); got != "Exit not found.\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestDMSetExitFlagInvalidRangeReturnsPromptSilentlyLikeLegacy(t *testing.T) {
	for _, flagNum := range []int64{0, 33} {
		t.Run(strconv.FormatInt(flagNum, 10), func(t *testing.T) {
			world := dmSetExitFlagWorld()
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewDMSetHandler(world)(ctx, dmSetExitFlagCommand("북", flagNum))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %v, want StatusDefault", status)
			}
			if got := ctx.OutputString(); got != "" {
				t.Fatalf("output = %q, want none", got)
			}
			if len(world.exitFlagsSet) != 0 {
				t.Fatalf("exit flags mutated: %+v", world.exitFlagsSet)
			}
		})
	}
}

func dmSetExitFlagWorld() *mockDMSetWorld {
	return &mockDMSetWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, RoomID: "room:00001"},
		},
		rooms: map[model.RoomID]model.Room{
			"room:00001": {
				ID:    "room:00001",
				Exits: []model.Exit{{Name: "북", ToRoomID: "room:00002"}},
			},
		},
	}
}

func dmSetExitFlagCommand(exitName string, flagNum int64) ResolvedCommand {
	return ResolvedCommand{
		Spec:   commandspec.CommandSpec{Name: "dm_set"},
		Args:   []string{"xf", exitName},
		Values: []int64{1, flagNum},
		Parsed: commandparse.Command{
			Num: 3,
			Str: [commandparse.CommandMax]string{"*set", "xf", exitName},
			Val: [commandparse.CommandMax]int64{1, 1, flagNum},
		},
	}
}
