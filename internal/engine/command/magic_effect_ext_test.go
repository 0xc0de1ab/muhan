package command

import (
	"strings"
	"testing"

	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

type mockMagicWorld struct {
	StatusWorld
	objects   map[model.ObjectInstanceID]model.ObjectInstance
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
	moved     map[model.ObjectInstanceID]model.ObjectLocation
	stats     map[model.CreatureID]map[string]int
	dirty     []model.PlayerID
	queued    []model.PlayerID
}

func (m *mockMagicWorld) Player(id model.PlayerID) (model.Player, bool) {
	if m.players != nil {
		if player, ok := m.players[id]; ok {
			return player, true
		}
	}
	return m.StatusWorld.Player(id)
}

func (m *mockMagicWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	if m.creatures != nil {
		if creature, ok := m.creatures[id]; ok {
			return creature, true
		}
	}
	return m.StatusWorld.Creature(id)
}

func (m *mockMagicWorld) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	obj, ok := m.objects[id]
	return obj, ok
}

func (m *mockMagicWorld) MoveObject(id model.ObjectInstanceID, location model.ObjectLocation) error {
	m.moved[id] = location
	if obj, ok := m.objects[id]; ok {
		obj.Location = location
		m.objects[id] = obj
	}
	return nil
}

func (m *mockMagicWorld) SetCreatureStat(id model.CreatureID, key string, val int) error {
	if m.stats[id] == nil {
		m.stats[id] = make(map[string]int)
	}
	m.stats[id][key] = val
	return nil
}

func (m *mockMagicWorld) MarkPlayerDirty(id model.PlayerID) {
	m.dirty = append(m.dirty, id)
}

func (m *mockMagicWorld) QueueSave(id model.PlayerID, _ model.BankID) {
	m.queued = append(m.queued, id)
}

func objectSendTestContext() *Context {
	return &Context{
		ActorID:   "player:alice",
		SessionID: "session:alice",
		Values: map[string]any{
			"game.activeSessions": func() []activeSession {
				return []activeSession{
					{ID: "session:alice", ActorID: "player:alice"},
					{ID: "session:bob", ActorID: "player:bob"},
				}
			},
		},
	}
}

func objectSendTestContextWithActive(active ...activeSession) *Context {
	return &Context{
		ActorID:   "player:alice",
		SessionID: "session:alice",
		Values: map[string]any{
			"game.activeSessions": func() []activeSession {
				out := make([]activeSession, len(active))
				copy(out, active)
				return out
			},
		},
	}
}

func objectSendWorld(t *testing.T, object model.ObjectInstance, actorClass int, actorMP int) (*mockMagicWorld, model.Creature, model.Creature) {
	t.Helper()
	loaded := emptyInventoryWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{
		"class":        actorClass,
		"level":        25,
		"intelligence": 25,
		"mpCurrent":    actorMP,
	}
	alice.Metadata.Tags = []string{"STRANO"}
	alice.Inventory.ObjectIDs = append(alice.Inventory.ObjectIDs, object.ID)
	loaded.Creatures[alice.ID] = alice

	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	bob := model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:library",
		Stats:       map[string]int{"strength": 10},
	}
	loaded.Creatures[bob.ID] = bob

	if object.Location.CreatureID.IsZero() {
		object.Location = model.ObjectLocation{CreatureID: alice.ID, Slot: "inventory"}
	}
	loaded.Objects[object.ID] = object
	for _, childID := range object.Contents.ObjectIDs {
		if _, ok := loaded.Objects[childID]; !ok {
			loaded.Objects[childID] = model.ObjectInstance{
				ID:       childID,
				Location: model.ObjectLocation{ContainerID: object.ID},
			}
		}
	}

	runtime := state.NewWorld(loaded)
	return &mockMagicWorld{
		StatusWorld: runtime,
		objects:     loaded.Objects,
		moved:       make(map[model.ObjectInstanceID]model.ObjectLocation),
		stats:       make(map[model.CreatureID]map[string]int),
	}, alice, bob
}

func TestMagicEffectObjectSendSuccess(t *testing.T) {
	loaded := emptyInventoryWorld(t)

	// Setup caster (Alice) and target (Bob)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{
		"class":        model.ClassMage,
		"level":        25,
		"intelligence": 25,
		"mpCurrent":    100,
	}
	alice.Metadata.Tags = []string{"STRANO"}
	loaded.Creatures[alice.ID] = alice

	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	bob := model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:library",
		Stats: map[string]int{
			"strength": 10, // Max weight: 20 + 100 = 120
		},
	}
	loaded.Creatures[bob.ID] = bob

	// Add object to alice's inventory
	loaded.Objects["object:sword"] = model.ObjectInstance{
		ID:       "object:sword",
		Location: model.ObjectLocation{CreatureID: alice.ID},
		Properties: map[string]string{
			"name":   "sword",
			"weight": "5",
		},
	}
	alice.Inventory.ObjectIDs = append(alice.Inventory.ObjectIDs, "object:sword")
	loaded.Creatures[alice.ID] = alice

	runtime := state.NewWorld(loaded)

	// Create mock wrapping runtime
	mock := &mockMagicWorld{
		StatusWorld: runtime,
		objects:     loaded.Objects,
		moved:       make(map[model.ObjectInstanceID]model.ObjectLocation),
		stats:       make(map[model.CreatureID]map[string]int),
	}

	ctx := objectSendTestContext()

	// Caster uses "전송 sword Bob"
	resolved := ResolvedCommand{
		Args:   []string{"전송", "sword", "Bob"},
		Values: []int64{1, 1, 1},
	}

	success, err := magicEffectObjectSend(ctx, mock, alice, model.ObjectInstance{}, resolved)
	if err != nil {
		t.Fatalf("magicEffectObjectSend error: %v", err)
	}
	if !success {
		t.Fatalf("expected success = true")
	}

	// Verify object moved to Bob
	movedLoc, found := mock.moved["object:sword"]
	if !found {
		t.Fatalf("expected sword to be moved")
	}
	if movedLoc.CreatureID != bob.ID || movedLoc.Slot != "inventory" {
		t.Errorf("expected sword moved to Bob inventory, got %+v", movedLoc)
	}

	// Verify MP deducted (mpCost = 8 + 5/4 = 9)
	mpCurrentVal := mock.stats[alice.ID]["mpCurrent"]
	if mpCurrentVal != 91 {
		t.Errorf("expected caster MP = 91, got %d", mpCurrentVal)
	}

	if strings.Contains(ctx.OutputString(), "\n.\n") {
		t.Errorf("unexpected extra dot line in output: %q", ctx.OutputString())
	}
	if !strings.Contains(ctx.OutputString(), "당신은 sword를 Bob에게 보냈습니다.") {
		t.Errorf("unexpected output: %q", ctx.OutputString())
	}
}

func TestMagicEffectObjectSendQueuesLegacySaves(t *testing.T) {
	object := model.ObjectInstance{
		ID:       "object:sword",
		Location: model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties: map[string]string{
			"name":   "sword",
			"weight": "5",
		},
	}
	mock, alice, _ := objectSendWorld(t, object, model.ClassMage, 100)
	ctx := objectSendTestContext()
	resolved := ResolvedCommand{
		Args:   []string{"전송", "sword", "Bob"},
		Values: []int64{1, 1, 1},
	}

	success, err := magicEffectObjectSend(ctx, mock, alice, model.ObjectInstance{}, resolved)
	if err != nil {
		t.Fatalf("magicEffectObjectSend error: %v", err)
	}
	if !success {
		t.Fatalf("expected success = true, output=%q", ctx.OutputString())
	}
	for _, playerID := range []model.PlayerID{"player:alice", "player:bob"} {
		if !objectSendPlayerIDSeen(mock.dirty, playerID) {
			t.Fatalf("dirty players = %+v, want %s marked like savegame_nomsg", mock.dirty, playerID)
		}
		if !objectSendPlayerIDSeen(mock.queued, playerID) {
			t.Fatalf("queued players = %+v, want %s queued like savegame_nomsg", mock.queued, playerID)
		}
	}
}

func TestMagicEffectObjectSendFailureWeightLimit(t *testing.T) {
	loaded := emptyInventoryWorld(t)

	// Setup caster (Alice)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{
		"class":        model.ClassMage,
		"level":        25,
		"intelligence": 10, // lower intelligence
		"mpCurrent":    100,
	}
	alice.Metadata.Tags = []string{"STRANO"}
	loaded.Creatures[alice.ID] = alice

	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	bob := model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:library",
		Stats: map[string]int{
			"strength": 10,
		},
	}
	loaded.Creatures[bob.ID] = bob

	// Add very heavy object to alice's inventory
	loaded.Objects["object:heavy"] = model.ObjectInstance{
		ID:       "object:heavy",
		Location: model.ObjectLocation{CreatureID: alice.ID},
		Properties: map[string]string{
			"name":   "heavy",
			"weight": "80", // Too heavy for Alice to send!
		},
	}
	alice.Inventory.ObjectIDs = append(alice.Inventory.ObjectIDs, "object:heavy")
	loaded.Creatures[alice.ID] = alice

	runtime := state.NewWorld(loaded)
	mock := &mockMagicWorld{
		StatusWorld: runtime,
		objects:     loaded.Objects,
		moved:       make(map[model.ObjectInstanceID]model.ObjectLocation),
		stats:       make(map[model.CreatureID]map[string]int),
	}

	ctx := objectSendTestContext()

	resolved := ResolvedCommand{
		Args:   []string{"전송", "heavy", "Bob"},
		Values: []int64{1, 1, 1},
	}

	scroll := model.ObjectInstance{ID: "object:scroll", Properties: map[string]string{"type": "7"}}
	success, err := magicEffectObjectSend(ctx, mock, alice, scroll, resolved)
	if err != nil {
		t.Fatalf("magicEffectObjectSend error: %v", err)
	}
	if success {
		t.Fatalf("expected failure due to weight limit")
	}

	if !strings.Contains(ctx.OutputString(), "너무 무거워 \n보낼 수 없습니다.") {
		t.Errorf("unexpected output: %q", ctx.OutputString())
	}
}

func TestMagicEffectObjectSendScrollBypassesCastClassAndMP(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{
		"class":        model.ClassFighter,
		"level":        25,
		"intelligence": 25,
		"mpCurrent":    0,
	}
	loaded.Creatures[alice.ID] = alice

	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	bob := model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:library",
		Stats:       map[string]int{"strength": 10},
	}
	loaded.Creatures[bob.ID] = bob

	loaded.Objects["object:sword"] = model.ObjectInstance{
		ID:       "object:sword",
		Location: model.ObjectLocation{CreatureID: alice.ID},
		Properties: map[string]string{
			"name":   "sword",
			"weight": "5",
		},
	}
	alice.Inventory.ObjectIDs = append(alice.Inventory.ObjectIDs, "object:sword")
	loaded.Creatures[alice.ID] = alice

	runtime := state.NewWorld(loaded)
	mock := &mockMagicWorld{
		StatusWorld: runtime,
		objects:     loaded.Objects,
		moved:       make(map[model.ObjectInstanceID]model.ObjectLocation),
		stats:       make(map[model.CreatureID]map[string]int),
	}
	ctx := objectSendTestContext()
	scroll := model.ObjectInstance{ID: "object:scroll", Properties: map[string]string{"type": "7"}}
	resolved := ResolvedCommand{
		Args:   []string{"전송", "sword", "Bob"},
		Values: []int64{1, 1, 1},
	}

	success, err := magicEffectObjectSend(ctx, mock, alice, scroll, resolved)
	if err != nil {
		t.Fatalf("magicEffectObjectSend error: %v", err)
	}
	if !success {
		t.Fatalf("expected success = true, output=%q", ctx.OutputString())
	}
	if moved := mock.moved["object:sword"]; moved.CreatureID != bob.ID {
		t.Fatalf("sword moved to %+v, want Bob", moved)
	}
	if _, ok := mock.stats[alice.ID]["mpCurrent"]; ok {
		t.Fatalf("scroll object_send deducted MP: %+v", mock.stats[alice.ID])
	}
}

func TestMagicEffectObjectSendRejectsSavedPlayerWithoutActiveSession(t *testing.T) {
	object := model.ObjectInstance{
		ID:       "object:sword",
		Location: model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties: map[string]string{
			"name":   "sword",
			"weight": "5",
		},
	}
	mock, alice, _ := objectSendWorld(t, object, model.ClassMage, 100)
	ctx := objectSendTestContextWithActive(activeSession{ID: "session:alice", ActorID: "player:alice"})
	resolved := ResolvedCommand{
		Args:   []string{"전송", "sword", "Bob"},
		Values: []int64{1, 1, 1},
	}

	success, err := magicEffectObjectSend(ctx, mock, alice, model.ObjectInstance{}, resolved)
	if err != nil {
		t.Fatalf("magicEffectObjectSend error: %v", err)
	}
	if success {
		t.Fatal("object_send succeeded for saved but offline player")
	}
	if got, want := ctx.OutputString(), "\n그런 사람이 존재하지 않습니다 .\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if len(mock.moved) != 0 {
		t.Fatalf("object moved for offline target: %+v", mock.moved)
	}
	if _, ok := mock.stats[alice.ID]["mpCurrent"]; ok {
		t.Fatalf("MP deducted before active target lookup: %+v", mock.stats[alice.ID])
	}
}

func TestMagicEffectObjectSendRejectsActivePlayerIDAliasLikeLegacy(t *testing.T) {
	object := model.ObjectInstance{
		ID:       "object:sword",
		Location: model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties: map[string]string{
			"name":   "sword",
			"weight": "5",
		},
	}
	mock, alice, _ := objectSendWorld(t, object, model.ClassMage, 100)
	mock.players = map[model.PlayerID]model.Player{
		"player:alice": {ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:library"},
		"player:bob":   {ID: "player:bob", CreatureID: "creature:bob", RoomID: "room:library"},
	}
	mock.creatures = map[model.CreatureID]model.Creature{
		"creature:alice": alice,
		"creature:bob": {
			ID:       "creature:bob",
			Kind:     model.CreatureKindPlayer,
			PlayerID: "player:bob",
			RoomID:   "room:library",
			Stats:    map[string]int{"strength": 10},
		},
	}
	ctx := objectSendTestContext()
	resolved := ResolvedCommand{
		Args:   []string{"전송", "sword", "bob"},
		Values: []int64{1, 1, 1},
	}

	success, err := magicEffectObjectSend(ctx, mock, alice, model.ObjectInstance{}, resolved)
	if err != nil {
		t.Fatalf("magicEffectObjectSend error: %v", err)
	}
	if success {
		t.Fatal("object_send succeeded through Go-only player ID alias")
	}
	if got, want := ctx.OutputString(), "\n그런 사람이 존재하지 않습니다 .\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if len(mock.moved) != 0 {
		t.Fatalf("object moved through player ID alias: %+v", mock.moved)
	}
	if _, ok := mock.stats[alice.ID]["mpCurrent"]; ok {
		t.Fatalf("MP deducted before target alias rejection: %+v", mock.stats[alice.ID])
	}
}

func TestMagicEffectObjectSendUsesRootWeightLikeLegacy(t *testing.T) {
	box := model.ObjectInstance{
		ID:       "object:box",
		Location: model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Contents: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
			"object:stone",
		}},
		Properties: map[string]string{
			"name":   "box",
			"weight": "5",
		},
	}
	mock, alice, bob := objectSendWorld(t, box, model.ClassMage, 100)
	mock.objects["object:stone"] = model.ObjectInstance{
		ID:       "object:stone",
		Location: model.ObjectLocation{ContainerID: "object:box"},
		Properties: map[string]string{
			"name":   "stone",
			"weight": "1000",
		},
	}
	ctx := objectSendTestContext()
	resolved := ResolvedCommand{
		Args:   []string{"전송", "box", "Bob"},
		Values: []int64{1, 1, 1},
	}

	success, err := magicEffectObjectSend(ctx, mock, alice, model.ObjectInstance{}, resolved)
	if err != nil {
		t.Fatalf("magicEffectObjectSend error: %v", err)
	}
	if !success {
		t.Fatalf("object_send failed using total nested weight; output=%q", ctx.OutputString())
	}
	if moved := mock.moved["object:box"]; moved.CreatureID != bob.ID {
		t.Fatalf("box moved to %+v, want Bob", moved)
	}
	if got := mock.stats[alice.ID]["mpCurrent"]; got != 91 {
		t.Fatalf("mpCurrent = %d, want 91 from root weight cost", got)
	}
}

func TestMagicEffectObjectSendUsesRootWeightForTargetCapacityLikeLegacy(t *testing.T) {
	box := model.ObjectInstance{
		ID:       "object:box",
		Location: model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Contents: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
			"object:stone",
		}},
		Properties: map[string]string{
			"name":   "box",
			"weight": "1",
		},
	}
	mock, alice, bob := objectSendWorld(t, box, model.ClassMage, 100)
	mock.objects["object:stone"] = model.ObjectInstance{
		ID:       "object:stone",
		Location: model.ObjectLocation{ContainerID: "object:box"},
		Properties: map[string]string{
			"name":   "stone",
			"weight": "1000",
		},
	}
	mock.objects["object:bob-load"] = model.ObjectInstance{
		ID:       "object:bob-load",
		Location: model.ObjectLocation{CreatureID: bob.ID, Slot: "inventory"},
		Properties: map[string]string{
			"name":   "load",
			"weight": "119",
		},
	}
	bob.Inventory.ObjectIDs = []model.ObjectInstanceID{"object:bob-load"}
	mock.creatures = map[model.CreatureID]model.Creature{
		alice.ID: alice,
		bob.ID:   bob,
	}
	ctx := objectSendTestContext()
	resolved := ResolvedCommand{
		Args:   []string{"전송", "box", "Bob"},
		Values: []int64{1, 1, 1},
	}

	success, err := magicEffectObjectSend(ctx, mock, alice, model.ObjectInstance{}, resolved)
	if err != nil {
		t.Fatalf("magicEffectObjectSend error: %v", err)
	}
	if !success {
		t.Fatalf("object_send failed using nested total for target capacity; output=%q", ctx.OutputString())
	}
	if moved := mock.moved["object:box"]; moved.CreatureID != bob.ID {
		t.Fatalf("box moved to %+v, want Bob", moved)
	}
}

func TestMagicEffectObjectSendRejectsDirectQuestChildLikeLegacy(t *testing.T) {
	box := model.ObjectInstance{
		ID:       "object:box",
		Location: model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Contents: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
			"object:quest",
		}},
		Properties: map[string]string{
			"name":   "box",
			"weight": "1",
		},
	}
	mock, alice, _ := objectSendWorld(t, box, model.ClassMage, 100)
	mock.objects["object:quest"] = model.ObjectInstance{
		ID:       "object:quest",
		Location: model.ObjectLocation{ContainerID: "object:box"},
		Properties: map[string]string{
			"name":     "quest",
			"questnum": "7",
		},
	}
	ctx := objectSendTestContext()
	resolved := ResolvedCommand{
		Args:   []string{"전송", "box", "Bob"},
		Values: []int64{1, 1, 1},
	}

	success, err := magicEffectObjectSend(ctx, mock, alice, model.ObjectInstance{}, resolved)
	if err != nil {
		t.Fatalf("magicEffectObjectSend error: %v", err)
	}
	if success {
		t.Fatal("object_send succeeded with direct quest child")
	}
	if got, want := ctx.OutputString(), "\n전송에 실패했습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if len(mock.moved) != 0 {
		t.Fatalf("object moved despite direct quest child: %+v", mock.moved)
	}
	if _, ok := mock.stats[alice.ID]["mpCurrent"]; ok {
		t.Fatalf("MP deducted before direct quest child rejection: %+v", mock.stats[alice.ID])
	}
}

func TestMagicEffectObjectSendRejectsDirectEventChildFlagsTokenLikeLegacy(t *testing.T) {
	box := model.ObjectInstance{
		ID:       "object:box",
		Location: model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Contents: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
			"object:event-child",
		}},
		Properties: map[string]string{
			"name":   "box",
			"weight": "1",
		},
	}
	mock, alice, _ := objectSendWorld(t, box, model.ClassMage, 100)
	mock.objects["object:event-child"] = model.ObjectInstance{
		ID:       "object:event-child",
		Location: model.ObjectLocation{ContainerID: "object:box"},
		Properties: map[string]string{
			"name":  "event",
			"flags": "eventItem",
		},
	}
	ctx := objectSendTestContext()
	resolved := ResolvedCommand{
		Args:   []string{"전송", "box", "Bob"},
		Values: []int64{1, 1, 1},
	}

	success, err := magicEffectObjectSend(ctx, mock, alice, model.ObjectInstance{}, resolved)
	if err != nil {
		t.Fatalf("magicEffectObjectSend error: %v", err)
	}
	if success {
		t.Fatal("object_send succeeded with direct event child")
	}
	if got, want := ctx.OutputString(), "\n전송에 실패했습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if len(mock.moved) != 0 {
		t.Fatalf("object moved despite direct event child: %+v", mock.moved)
	}
	if _, ok := mock.stats[alice.ID]["mpCurrent"]; ok {
		t.Fatalf("MP deducted before direct event child rejection: %+v", mock.stats[alice.ID])
	}
}

func TestMagicEffectObjectSendAllowsDirectQuestChildForDMLikeLegacy(t *testing.T) {
	box := model.ObjectInstance{
		ID:       "object:box",
		Location: model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Contents: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
			"object:quest",
		}},
		Properties: map[string]string{
			"name":   "box",
			"weight": "1",
		},
	}
	mock, alice, bob := objectSendWorld(t, box, model.ClassDM, 100)
	mock.objects["object:quest"] = model.ObjectInstance{
		ID:       "object:quest",
		Location: model.ObjectLocation{ContainerID: "object:box"},
		Properties: map[string]string{
			"name":     "quest",
			"questnum": "7",
		},
	}
	ctx := objectSendTestContext()
	resolved := ResolvedCommand{
		Args:   []string{"전송", "box", "Bob"},
		Values: []int64{1, 1, 1},
	}

	scroll := model.ObjectInstance{ID: "object:scroll", Properties: map[string]string{"type": "7"}}
	success, err := magicEffectObjectSend(ctx, mock, alice, scroll, resolved)
	if err != nil {
		t.Fatalf("magicEffectObjectSend error: %v", err)
	}
	if !success {
		t.Fatalf("object_send failed for DM direct quest child; output=%q", ctx.OutputString())
	}
	if moved := mock.moved["object:box"]; moved.CreatureID != bob.ID {
		t.Fatalf("box moved to %+v, want Bob", moved)
	}
}

func TestMagicEffectObjectSendRejectsTopLevelQuestNumberLikeLegacy(t *testing.T) {
	object := model.ObjectInstance{
		ID:       "object:quest",
		Location: model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties: map[string]string{
			"name":     "quest",
			"questnum": "7",
			"weight":   "1",
		},
	}
	mock, alice, _ := objectSendWorld(t, object, model.ClassMage, 100)
	ctx := objectSendTestContext()
	resolved := ResolvedCommand{
		Args:   []string{"전송", "quest", "Bob"},
		Values: []int64{1, 1, 1},
	}

	success, err := magicEffectObjectSend(ctx, mock, alice, model.ObjectInstance{}, resolved)
	if err != nil {
		t.Fatalf("magicEffectObjectSend error: %v", err)
	}
	if success {
		t.Fatal("object_send succeeded with top-level quest object")
	}
	if got, want := ctx.OutputString(), "\n임무에 관련되어 다른자에게 보낼 수 없습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if len(mock.moved) != 0 {
		t.Fatalf("object moved despite top-level quest flag: %+v", mock.moved)
	}
	if _, ok := mock.stats[alice.ID]["mpCurrent"]; ok {
		t.Fatalf("MP deducted before top-level quest rejection: %+v", mock.stats[alice.ID])
	}
}

func TestMagicEffectObjectSendRejectsTopLevelEventLikeLegacy(t *testing.T) {
	object := model.ObjectInstance{
		ID:       "object:event",
		Location: model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties: map[string]string{
			"name":   "event",
			"OEVENT": "1",
			"weight": "1",
		},
	}
	mock, alice, _ := objectSendWorld(t, object, model.ClassMage, 100)
	ctx := objectSendTestContext()
	resolved := ResolvedCommand{
		Args:   []string{"전송", "event", "Bob"},
		Values: []int64{1, 1, 1},
	}

	success, err := magicEffectObjectSend(ctx, mock, alice, model.ObjectInstance{}, resolved)
	if err != nil {
		t.Fatalf("magicEffectObjectSend error: %v", err)
	}
	if success {
		t.Fatal("object_send succeeded with top-level event object")
	}
	if got, want := ctx.OutputString(), "\n이벤트 아이템은 다른자에게 보낼 수 없습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if len(mock.moved) != 0 {
		t.Fatalf("object moved despite top-level event flag: %+v", mock.moved)
	}
	if _, ok := mock.stats[alice.ID]["mpCurrent"]; ok {
		t.Fatalf("MP deducted before top-level event rejection: %+v", mock.stats[alice.ID])
	}
}

func TestMagicEffectDispatcherRouting(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SMENDW"}
	alice.Stats = map[string]int{
		"class":        model.ClassMage,
		"level":        100,
		"intelligence": 25,
		"hpCurrent":    10,
		"hpMax":        100,
		"mpCurrent":    10,
		"mpMax":        100,
	}
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)

	ctx := &Context{
		ActorID:   "player:alice",
		SessionID: "session:alice",
	}

	// Test that magicPowerMend (19) is correctly routed and handled
	// Mend on self:
	resolved := ResolvedCommand{
		Args:   []string{"원기회복", "나"},
		Values: []int64{1, 1},
	}

	// Call the main dispatcher
	success, err := applyMagicPowerEffect(ctx, runtime, alice, model.ObjectInstance{}, resolved, magicPowerMend, true)
	if err != nil {
		t.Fatalf("applyMagicPowerEffect Mend error: %v", err)
	}
	if !success {
		t.Fatalf("expected Mend to succeed")
	}

	updated, _ := runtime.Creature(alice.ID)
	if updated.Stats["hpCurrent"] <= 10 {
		t.Errorf("expected HP to be healed, got %d", updated.Stats["hpCurrent"])
	}
}

func objectSendPlayerIDSeen(ids []model.PlayerID, target model.PlayerID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
