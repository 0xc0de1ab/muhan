package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type dummySession20 struct {
	ID      string
	ActorID string
}

type dummyCommand20 struct {
	Write string
}

type unifiedDMWorld20 struct {
	players           map[model.PlayerID]model.Player
	creatures         map[model.CreatureID]model.Creature
	rooms             map[model.RoomID]model.Room
	updatedCreatureID model.CreatureID
	addedTags         []string
	removedTags       []string
	setStatCreatureID model.CreatureID
	setStatKey        string
	setStatValue      int
	purgedRoom        model.RoomID
}

func (w *unifiedDMWorld20) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *unifiedDMWorld20) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *unifiedDMWorld20) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *unifiedDMWorld20) UpdateCreatureTags(id model.CreatureID, add []string, remove []string) (model.Creature, error) {
	w.updatedCreatureID = id
	w.addedTags = add
	w.removedTags = remove
	return w.creatures[id], nil
}

func (w *unifiedDMWorld20) SetCreatureStat(id model.CreatureID, key string, val int) error {
	w.setStatCreatureID = id
	w.setStatKey = key
	w.setStatValue = val
	return nil
}

func (w *unifiedDMWorld20) PurgeRoom(id model.RoomID) error {
	w.purgedRoom = id
	return nil
}

func (w *unifiedDMWorld20) DestroyCreature(id model.CreatureID) error {
	w.purgedRoom = "room:100"
	return nil
}

func (w *unifiedDMWorld20) DestroyObject(id model.ObjectInstanceID) error {
	w.purgedRoom = "room:100"
	return nil
}

func (w *unifiedDMWorld20) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	return model.ObjectInstance{}, false
}

func (w *unifiedDMWorld20) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	return model.ObjectPrototype{}, false
}

func TestUnifiedDMCommands20(t *testing.T) {
	setupWorld := func() *unifiedDMWorld20 {
		return &unifiedDMWorld20{
			players: map[model.PlayerID]model.Player{
				"player:dm":    {ID: "player:dm", CreatureID: "creature:dm", RoomID: "room:100", AccountName: "dm_acc"},
				"player:alice": {ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:100", AccountName: "alice_acc"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {
					ID:     "creature:dm",
					RoomID: "room:100",
					Level:  15,
					Stats: map[string]int{
						"class":     12,
						"hpMax":     120,
						"hpCurrent": 60,
						"mpMax":     70,
						"mpCurrent": 20,
						"armor":     40,
						"thaco":     11,
					},
				},
				"creature:alice": {
					ID:          "creature:alice",
					DisplayName: "Alice",
					RoomID:      "room:100",
					Level:       5,
					Stats:       map[string]int{"class": 10, "armor": 20, "thaco": 9},
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:100": {ID: "room:100", DisplayName: "광장"},
			},
		}
	}

	t.Run("dm_invis", func(t *testing.T) {
		world := setupWorld()
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "*invis", Handler: "dm_invis"},
		}
		handler := NewDMInvisHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if world.updatedCreatureID != "creature:dm" {
			t.Errorf("expected creature:dm updated, got %s", world.updatedCreatureID)
		}
	})

	t.Run("dm_ac", func(t *testing.T) {
		world := setupWorld()
		ctx := &Context{
			ActorID: "player:dm",
			Values: map[string]any{
				"game.activeSessions": func() []dummySession20 {
					return []dummySession20{{ID: "s-alice", ActorID: "player:alice"}}
				},
			},
		}
		resolved := ResolvedCommand{
			Args: []string{"alice", "-5"},
			Spec: commandspec.CommandSpec{Name: "*ac", Handler: "dm_ac"},
		}
		handler := NewDMAcHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if world.setStatCreatureID == "creature:alice" {
			t.Fatalf("dm_ac must not set target AC with extra args; got %s %s %d", world.setStatCreatureID, world.setStatKey, world.setStatValue)
		}
		if world.setStatCreatureID != "creature:dm" || world.setStatKey != "mpCurrent" || world.setStatValue != 70 {
			t.Errorf("expected self mp restore as last stat set, got %s %s %d", world.setStatCreatureID, world.setStatKey, world.setStatValue)
		}
		if got, want := ctx.OutputString(), "AC: 30  THAC0: 11\n"; got != want {
			t.Errorf("expected self AC output %q, got %q", want, got)
		}
	})

	t.Run("dm_send", func(t *testing.T) {
		world := setupWorld()
		if aliceCrt, ok := world.creatures["creature:alice"]; ok {
			aliceCrt.Stats["class"] = 10
			world.creatures["creature:alice"] = aliceCrt
		}
		var sendToSessionCalled bool
		ctx := &Context{
			ActorID: "player:dm",
			Values: map[string]any{
				"game.activeSessions": func() []dummySession20 {
					return []dummySession20{{ID: "s-alice", ActorID: "player:alice"}}
				},
				"game.sendToSession": func(sessionID string, cmd dummyCommand20) error {
					sendToSessionCalled = true
					return nil
				},
			},
		}
		resolved := ResolvedCommand{
			Input:  "*send alice hello",
			Parsed: commandparse.Parse("*send alice hello"),
			Args:   []string{"alice", "hello"},
			Spec:   commandspec.CommandSpec{Name: "*send", Handler: "dm_send"},
		}
		handler := NewDMSendHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if !sendToSessionCalled {
			t.Error("expected sendToSession to be called")
		}
	})

	t.Run("dm_echo", func(t *testing.T) {
		world := setupWorld()
		var broadcastText string
		ctx := &Context{
			ActorID: "player:dm",
			Values: map[string]any{
				ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
					broadcastText = text
					return nil
				}),
			},
		}
		resolved := ResolvedCommand{
			Args: []string{"hello", "room"},
			Spec: commandspec.CommandSpec{Name: "*echo", Handler: "dm_echo"},
		}
		handler := NewDMEchoHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if broadcastText != "\nhello room" {
			t.Errorf("expected broadcast '\nhello room', got %q", broadcastText)
		}
	})

	t.Run("dm_purge", func(t *testing.T) {
		world := setupWorld()
		// Add a monster to the room and creatures map
		room := world.rooms["room:100"]
		room.CreatureIDs = append(room.CreatureIDs, "creature:monster")
		world.rooms["room:100"] = room
		world.creatures["creature:monster"] = model.Creature{
			ID:     "creature:monster",
			Kind:   model.CreatureKindMonster,
			RoomID: "room:100",
		}

		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "*purge", Handler: "dm_purge"},
		}
		handler := NewDMPurgeHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if world.purgedRoom != "room:100" {
			t.Errorf("expected room:100 purged, got %s", world.purgedRoom)
		}
	})

	t.Run("dm_users", func(t *testing.T) {
		world := setupWorld()
		ctx := &Context{
			ActorID: "player:dm",
			Values: map[string]any{
				"game.activeSessions": func() []dummySession20 {
					return []dummySession20{
						{ID: "s-dm", ActorID: "player:dm"},
						{ID: "s-alice", ActorID: "player:alice"},
					}
				},
			},
		}
		resolved := ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "*users", Handler: "dm_users"},
		}
		handler := NewDMUsersHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		output := ctx.OutputString()
		if !strings.Contains(output, "Alice") || !strings.Contains(output, "광장") {
			t.Errorf("expected output to contain Alice and room name, got %q", output)
		}
	})
}
