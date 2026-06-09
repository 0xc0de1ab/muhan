package command

import (
	"strings"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

// unifiedDMWorld mock implements all 6 DM command world interfaces.
type unifiedDMWorld struct {
	players           map[model.PlayerID]model.Player
	creatures         map[model.CreatureID]model.Creature
	rooms             map[model.RoomID]model.Room
	objects           map[model.ObjectInstanceID]model.ObjectInstance
	prototypes        map[model.PrototypeID]model.ObjectPrototype
	reloadedRoom      model.RoomID
	resavedRoom       model.RoomID
	createdProtoID    model.PrototypeID
	createdCreatureID model.CreatureID
	movedPlayerID     model.PlayerID
	movedDestRoomID   model.RoomID
	updatedObjID      model.ObjectInstanceID
	addedTags         []string
}

func (w *unifiedDMWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *unifiedDMWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *unifiedDMWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *unifiedDMWorld) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	o, ok := w.objects[id]
	return o, ok
}

func (w *unifiedDMWorld) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	p, ok := w.prototypes[id]
	return p, ok
}

func (w *unifiedDMWorld) ReloadRoom(id model.RoomID) error {
	w.reloadedRoom = id
	return nil
}

func (w *unifiedDMWorld) ResaveRoom(id model.RoomID) error {
	w.resavedRoom = id
	return nil
}

func (w *unifiedDMWorld) MovePlayerToRoom(playerID model.PlayerID, roomID model.RoomID) error {
	w.movedPlayerID = playerID
	w.movedDestRoomID = roomID
	return nil
}

func (w *unifiedDMWorld) CreateObjectInstanceFromPrototype(protoID model.PrototypeID, creatureID model.CreatureID) (model.ObjectInstance, error) {
	w.createdProtoID = protoID
	w.createdCreatureID = creatureID
	return model.ObjectInstance{ID: "object:new:1"}, nil
}

func (w *unifiedDMWorld) UpdateObjectTags(id model.ObjectInstanceID, add []string, remove []string) (model.ObjectInstance, error) {
	w.updatedObjID = id
	w.addedTags = add
	return model.ObjectInstance{}, nil
}

func TestUnifiedDMCommands(t *testing.T) {
	setupWorld := func(class int) *unifiedDMWorld {
		return &unifiedDMWorld{
			players: map[model.PlayerID]model.Player{
				"player:dm":    {ID: "player:dm", CreatureID: "creature:dm", RoomID: "room:100"},
				"player:alice": {ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:200"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm":    {ID: "creature:dm", RoomID: "room:100", Stats: map[string]int{"class": class}},
				"creature:alice": {ID: "creature:alice", DisplayName: "Alice", RoomID: "room:200", Stats: map[string]int{"class": 1}},
			},
			rooms: map[model.RoomID]model.Room{
				"room:100": {
					ID: "room:100",
					Objects: model.ObjectRefList{
						ObjectIDs: []model.ObjectInstanceID{"object:sword:1"},
					},
				},
				"room:200": {ID: "room:200"},
			},
			objects: map[model.ObjectInstanceID]model.ObjectInstance{
				"object:sword:1": {
					ID:                  "object:sword:1",
					PrototypeID:         "prototype:1",
					DisplayNameOverride: "검",
					Location:            model.ObjectLocation{RoomID: "room:100"},
				},
			},
			prototypes: map[model.PrototypeID]model.ObjectPrototype{
				"prototype:1": {
					ID:          "prototype:1",
					DisplayName: "검",
					Keywords:    []string{"sword"},
				},
			},
		}
	}

	t.Run("dm_teleport", func(t *testing.T) {
		world := setupWorld(13)
		ctx := &Context{
			ActorID: "player:dm",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{
						{ID: "session:dm", ActorID: "player:dm"},
						{ID: "session:alice", ActorID: "player:alice"},
					}
				},
			},
		}
		resolved := ResolvedCommand{
			Input: "*teleport alice .",
			Args:  []string{"alice", "."},
			Spec:  commandspec.CommandSpec{Name: "*teleport", Handler: "dm_teleport"},
		}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if world.movedPlayerID != "player:alice" || world.movedDestRoomID != "room:100" {
			t.Errorf("expected alice moved to room:100, got %s to %s", world.movedPlayerID, world.movedDestRoomID)
		}
	})

	t.Run("dm_rmstat", func(t *testing.T) {
		world := setupWorld(13)
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Input: "*rmstat",
			Spec:  commandspec.CommandSpec{Name: "*rmstat", Handler: "dm_rmstat"},
		}

		handler := NewDMRmstatHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(ctx.OutputString(), "#100") {
			t.Errorf("expected room #100 in output, got %q", ctx.OutputString())
		}
	})

	t.Run("dm_reload_rom", func(t *testing.T) {
		world := setupWorld(13)
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Input: "*reload",
			Spec:  commandspec.CommandSpec{Name: "*reload", Handler: "dm_reload_rom"},
		}

		handler := NewDMReloadRomHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if world.reloadedRoom != "room:100" {
			t.Errorf("expected room:100 reloaded, got %s", world.reloadedRoom)
		}
	})

	t.Run("dm_resave", func(t *testing.T) {
		world := setupWorld(13)
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Input: "*resave",
			Spec:  commandspec.CommandSpec{Name: "*resave", Handler: "dm_resave"},
		}

		handler := NewDMResaveHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if world.resavedRoom != "room:100" {
			t.Errorf("expected room:100 resaved, got %s", world.resavedRoom)
		}
	})

	t.Run("dm_create_obj", func(t *testing.T) {
		world := setupWorld(13)
		world.prototypes["object:o01:23"] = model.ObjectPrototype{
			ID:          "object:o01:23",
			DisplayName: "검",
		}
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Input: "*create_obj 123",
			Parsed: commandparse.Command{
				Val: [commandparse.CommandMax]int64{123},
			},
			Spec: commandspec.CommandSpec{Name: "*create_obj", Handler: "dm_create_obj"},
		}

		handler := NewDMCreateObjHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if world.createdProtoID != "object:o01:23" || world.createdCreatureID != "creature:dm" {
			t.Errorf("expected object:o01:23 created for creature:dm, got %s for %s", world.createdProtoID, world.createdCreatureID)
		}
	})

	t.Run("dm_perm", func(t *testing.T) {
		world := setupWorld(13)
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Input: "*perm sword",
			Args:  []string{"sword"},
			Spec:  commandspec.CommandSpec{Name: "*perm", Handler: "dm_perm"},
		}

		handler := NewDMPermHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if world.updatedObjID != "object:sword:1" {
			t.Errorf("expected object:sword:1 updated, got %s", world.updatedObjID)
		}
		hasTag := func(name string) bool {
			for _, tag := range world.addedTags {
				if strings.EqualFold(tag, name) {
					return true
				}
			}
			return false
		}
		if !hasTag("operm2") || !hasTag("otempp") {
			t.Errorf("expected operm2 and otempp added, got %v", world.addedTags)
		}
	})
}
