package command

import (
	"strings"
	"testing"

	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type mockDMPurgeWorld struct {
	players            map[model.PlayerID]model.Player
	creatures          map[model.CreatureID]model.Creature
	rooms              map[model.RoomID]model.Room
	objects            map[model.ObjectInstanceID]model.ObjectInstance
	objectPrototypes   map[model.PrototypeID]model.ObjectPrototype
	destroyedCreatures []model.CreatureID
	destroyedObjects   []model.ObjectInstanceID
}

func (m *mockDMPurgeWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMPurgeWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMPurgeWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := m.rooms[id]
	return r, ok
}

func (m *mockDMPurgeWorld) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	o, ok := m.objects[id]
	return o, ok
}

func (m *mockDMPurgeWorld) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	p, ok := m.objectPrototypes[id]
	return p, ok
}

func (m *mockDMPurgeWorld) DestroyCreature(id model.CreatureID) error {
	m.destroyedCreatures = append(m.destroyedCreatures, id)
	return nil
}

func (m *mockDMPurgeWorld) DestroyObject(id model.ObjectInstanceID) error {
	m.destroyedObjects = append(m.destroyedObjects, id)
	return nil
}

type dummyPurgeSession struct {
	ID      string
	ActorID string
}

type dummyPurgeSessionID string

type dummyPurgeCommand struct {
	Write string
}

type mockPurgeGroupMemory struct {
	unfollowFunc func(follower string) (string, bool)
}

func (m *mockPurgeGroupMemory) Unfollow(follower string) (string, bool) {
	if m.unfollowFunc != nil {
		return m.unfollowFunc(follower)
	}
	return "", false
}

func TestDMPurge(t *testing.T) {
	// 1. Basic permission checks and errors
	t.Run("empty actor ID", func(t *testing.T) {
		world := &mockDMPurgeWorld{}
		handler := NewDMPurgeHandler(world)
		ctx := &Context{ActorID: ""}
		status, err := handler(ctx, ResolvedCommand{})
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusDefault {
			t.Errorf("got status %v, want StatusDefault", status)
		}
	})

	t.Run("actor not found in world", func(t *testing.T) {
		world := &mockDMPurgeWorld{}
		handler := NewDMPurgeHandler(world)
		ctx := &Context{ActorID: "player:alice"}
		status, err := handler(ctx, ResolvedCommand{})
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusPrompt {
			t.Errorf("got status %v, want StatusPrompt", status)
		}
		if got := ctx.OutputString(); got != "" {
			t.Errorf("got output %q, want no permission output", got)
		}
	})

	t.Run("unauthorized class below SUB_DM", func(t *testing.T) {
		world := &mockDMPurgeWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassBulsa}},
			},
		}
		handler := NewDMPurgeHandler(world)
		ctx := &Context{ActorID: "player:alice"}
		status, err := handler(ctx, ResolvedCommand{})
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusPrompt {
			t.Errorf("got status %v, want StatusPrompt", status)
		}
		if got := ctx.OutputString(); got != "" {
			t.Errorf("got output %q, want no permission output", got)
		}
	})

	t.Run("room not found", func(t *testing.T) {
		world := &mockDMPurgeWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}, RoomID: "room:invalid"},
			},
		}
		handler := NewDMPurgeHandler(world)
		ctx := &Context{ActorID: "player:alice"}
		_, err := handler(ctx, ResolvedCommand{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "room not found") {
			t.Errorf("expected room not found error, got %v", err)
		}
	})

	// 2. Full success scenarios
	t.Run("purge monsters and floor objects, handling OTEMPP and MDMFOL", func(t *testing.T) {
		world := &mockDMPurgeWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
				"player:other":  {ID: "player:other", CreatureID: "creature:other"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID:       "creature:caster",
					Kind:     model.CreatureKindPlayer,
					PlayerID: "player:caster",
					Stats:    map[string]int{"class": model.ClassSubDM},
					RoomID:   "room:100",
				},
				"creature:other": {
					ID:       "creature:other",
					Kind:     model.CreatureKindPlayer, // Player
					RoomID:   "room:100",
					PlayerID: "player:other",
				},
				"creature:monster1": {
					ID:          "creature:monster1",
					Kind:        model.CreatureKindMonster,
					DisplayName: "오크",
					RoomID:      "room:100",
				},
				"creature:follower": {
					ID:          "creature:follower",
					Kind:        model.CreatureKindMonster,
					DisplayName: "미니미",
					RoomID:      "room:100",
					Metadata:    model.Metadata{Tags: []string{"MDMFOL"}},
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:100": {
					ID:          "room:100",
					DisplayName: "광장",
					CreatureIDs: []model.CreatureID{
						"creature:caster",
						"creature:other",
						"creature:monster1",
						"creature:follower",
					},
					Objects: model.ObjectRefList{
						ObjectIDs: []model.ObjectInstanceID{
							"obj:normal",
							"obj:temporary",
						},
					},
				},
			},
			objects: map[model.ObjectInstanceID]model.ObjectInstance{
				"obj:normal": {
					ID:          "obj:normal",
					PrototypeID: "proto:normal",
				},
				"obj:temporary": {
					ID:          "obj:temporary",
					PrototypeID: "proto:temp",
				},
			},
			objectPrototypes: map[model.PrototypeID]model.ObjectPrototype{
				"proto:normal": {
					ID: "proto:normal",
				},
				"proto:temp": {
					ID:       "proto:temp",
					Metadata: model.Metadata{Tags: []string{"OTEMPP"}},
				},
			},
		}

		var notifiedSessionID string
		var notifiedText string
		sendFn := func(sid dummyPurgeSessionID, cmd dummyPurgeCommand) {
			notifiedSessionID = string(sid)
			notifiedText = cmd.Write
		}

		groupMem := &mockPurgeGroupMemory{
			unfollowFunc: func(follower string) (string, bool) {
				if follower == "creature:follower" {
					return "player:other", true // Followed player:other
				}
				return "", false
			},
		}

		ctx := &Context{
			ActorID:   "player:caster",
			SessionID: "session-caster",
			Values: map[string]any{
				"game.activeSessions": func() []dummyPurgeSession {
					return []dummyPurgeSession{
						{ID: "session-caster", ActorID: "player:caster"},
						{ID: "session-other", ActorID: "player:other"},
					}
				},
				"game.groupMemory":   groupMem,
				"game.sendToSession": sendFn,
			},
		}

		handler := NewDMPurgeHandler(world)
		status, err := handler(ctx, ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "dm_purge"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("got status %v, want StatusDefault", status)
		}

		// Verify output to caster
		if ctx.OutputString() != "청소되었습니다.\n" {
			t.Errorf("caster output was %q, want %q", ctx.OutputString(), "청소되었습니다.\n")
		}

		// Verify monster1 and follower were destroyed
		expectedDestroyedCreatures := map[model.CreatureID]bool{
			"creature:monster1": true,
			"creature:follower": true,
		}
		for _, id := range world.destroyedCreatures {
			delete(expectedDestroyedCreatures, id)
		}
		if len(expectedDestroyedCreatures) > 0 {
			t.Errorf("expected creatures not destroyed: %v", expectedDestroyedCreatures)
		}

		// Verify player caster and other were NOT destroyed
		t.Logf("destroyed creatures: %v", world.destroyedCreatures)
		for _, id := range world.destroyedCreatures {
			if id == "creature:caster" || id == "creature:other" {
				t.Errorf("player creature %s was destroyed", id)
			}
		}

		// Verify all floor objects were removed. C clears the room object list
		// before skipping OTEMPP frees, so OTEMPP objects do not remain visible.
		expectedDestroyedObjects := map[model.ObjectInstanceID]bool{
			"obj:normal":    true,
			"obj:temporary": true,
		}
		for _, id := range world.destroyedObjects {
			delete(expectedDestroyedObjects, id)
		}
		if len(expectedDestroyedObjects) > 0 {
			t.Errorf("expected objects not destroyed: %v", expectedDestroyedObjects)
		}

		// Verify notification was sent to follower's leader (player:other)
		if notifiedSessionID != "session-other" {
			t.Errorf("expected leader notification to session-other, got %q", notifiedSessionID)
		}
		expectedMsg := "미니미가 당신을 그만 따릅니다.\n"
		if notifiedText != expectedMsg {
			t.Errorf("leader notification text was %q, want %q", notifiedText, expectedMsg)
		}
	})
}
