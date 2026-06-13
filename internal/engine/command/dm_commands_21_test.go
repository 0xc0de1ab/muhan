package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type unifiedDMWorld21 struct {
	players    map[model.PlayerID]model.Player
	creatures  map[model.CreatureID]model.Creature
	rooms      map[model.RoomID]model.Room
	objects    map[model.ObjectInstanceID]model.ObjectInstance
	prototypes map[model.PrototypeID]model.ObjectPrototype

	resavedAllRooms   bool
	resavePermOnly    bool
	shutdownSeconds   int
	shutdownNow       bool
	forcedPlayerID    model.PlayerID
	forcedCmd         string
	flushedCrtObj     bool
	spawnedProtoID    model.CreatureID
	spawnedRoomID     model.RoomID
	spawnedCarryItems bool
}

func (u *unifiedDMWorld21) FlushActivePlayersAndBanks() error { return nil }
func (u *unifiedDMWorld21) SavePlayer(model.PlayerID) error   { return nil }

func (w *unifiedDMWorld21) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *unifiedDMWorld21) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *unifiedDMWorld21) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *unifiedDMWorld21) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	o, ok := w.objects[id]
	return o, ok
}

func (w *unifiedDMWorld21) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	o, ok := w.prototypes[id]
	return o, ok
}

func (w *unifiedDMWorld21) ResaveAllRooms(permOnly bool) error {
	w.resavedAllRooms = true
	w.resavePermOnly = permOnly
	return nil
}

func (w *unifiedDMWorld21) SetShutdown(seconds int, now bool) error {
	w.shutdownSeconds = seconds
	w.shutdownNow = now
	return nil
}

func (w *unifiedDMWorld21) FindPlayerByName(name string) (model.Player, bool) {
	for _, p := range w.players {
		if strings.EqualFold(p.DisplayName, name) {
			return p, true
		}
	}
	return model.Player{}, false
}

func (w *unifiedDMWorld21) ForcePlayerCommand(playerID model.PlayerID, cmd string) error {
	w.forcedPlayerID = playerID
	w.forcedCmd = cmd
	return nil
}

func (w *unifiedDMWorld21) FlushCrtObj() error {
	w.flushedCrtObj = true
	return nil
}

func (w *unifiedDMWorld21) CreaturePrototype(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *unifiedDMWorld21) SpawnCreature(protoID model.CreatureID, roomID model.RoomID, carryItems bool) (model.CreatureID, error) {
	w.spawnedProtoID = protoID
	w.spawnedRoomID = roomID
	w.spawnedCarryItems = carryItems
	return protoID + "_inst", nil
}

func (w *unifiedDMWorld21) FindCreatureByName(roomID model.RoomID, name string, count int) (model.Creature, bool) {
	room, ok := w.rooms[roomID]
	if !ok {
		return model.Creature{}, false
	}
	for _, cid := range room.CreatureIDs {
		if c, ok := w.creatures[cid]; ok && strings.EqualFold(c.DisplayName, name) {
			return c, true
		}
	}
	return model.Creature{}, false
}

func (w *unifiedDMWorld21) FindCreatureByNameGlobal(name string) (model.Creature, bool) {
	for _, c := range w.creatures {
		if strings.EqualFold(c.DisplayName, name) {
			return c, true
		}
	}
	return model.Creature{}, false
}

func (w *unifiedDMWorld21) FindObjectByName(creatureID model.CreatureID, roomID model.RoomID, name string, count int) (model.ObjectInstance, bool) {
	for _, o := range w.objects {
		if strings.EqualFold(o.DisplayNameOverride, name) {
			return o, true
		}
	}
	return model.ObjectInstance{}, false
}

func TestUnifiedDMCommands21(t *testing.T) {
	setupWorld := func() *unifiedDMWorld21 {
		return &unifiedDMWorld21{
			players: map[model.PlayerID]model.Player{
				"player:dm":    {ID: "player:dm", DisplayName: "DMPlayer", CreatureID: "creature:dm", RoomID: "room:100"},
				"player:subdm": {ID: "player:subdm", DisplayName: "SubDMPlayer", CreatureID: "creature:subdm", RoomID: "room:100"},
				"player:alice": {ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:100"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm":    {ID: "creature:dm", DisplayName: "DMPlayer", RoomID: "room:100", Level: 15, Stats: map[string]int{"class": 13}},
				"creature:subdm": {ID: "creature:subdm", DisplayName: "SubDMPlayer", RoomID: "room:100", Level: 12, Stats: map[string]int{"class": model.ClassSubDM}},
				"creature:alice": {ID: "creature:alice", DisplayName: "Alice", RoomID: "room:100", Level: 5, Stats: map[string]int{"class": 1}},
				"creature:m01:5": {ID: "creature:m01:5", DisplayName: "Goblin", RoomID: "room:100"},
			},
			rooms: map[model.RoomID]model.Room{
				"room:100": {ID: "room:100", DisplayName: "광장", CreatureIDs: []model.CreatureID{"creature:dm", "creature:subdm", "creature:alice"}},
			},
			objects:    map[model.ObjectInstanceID]model.ObjectInstance{},
			prototypes: map[model.PrototypeID]model.ObjectPrototype{},
		}
	}

	t.Run("dm_flushsave permission denied", func(t *testing.T) {
		w := setupWorld()
		ctx := &Context{ActorID: "player:alice"}
		handler := NewDMFlushsaveHandler(w)
		resolved := ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "*flushrooms"},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}
		if w.resavedAllRooms {
			t.Error("expected ResaveAllRooms not to be called")
		}
	})

	t.Run("dm_flushsave success standard", func(t *testing.T) {
		w := setupWorld()
		ctx := &Context{ActorID: "player:dm"}
		handler := NewDMFlushsaveHandler(w)
		resolved := ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "*flushrooms"},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		if !w.resavedAllRooms || w.resavePermOnly {
			t.Error("expected ResaveAllRooms(false) to be called")
		}
		if !strings.Contains(ctx.OutputString(), "flushed to disk") {
			t.Errorf("unexpected output: %q", ctx.OutputString())
		}
	})

	t.Run("dm_shutdown success", func(t *testing.T) {
		w := setupWorld()
		ctx := &Context{ActorID: "player:dm"}
		handler := NewDMShutdownHandler(w)
		parsed := commandparse.ParseCommandFirst("*shutdown 5")
		resolved := ResolvedCommand{
			Input:  "*shutdown 5",
			Parsed: parsed,
			Spec:   commandspec.CommandSpec{Name: "*shutdown"},
			Args:   commandArgs(parsed),
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		if w.shutdownSeconds != 301 { // 5 * 60 + 1
			t.Errorf("shutdownSeconds = %d, want 301", w.shutdownSeconds)
		}
	})

	t.Run("dm_force success", func(t *testing.T) {
		w := setupWorld()
		type activeSession struct {
			ID      string
			ActorID string
		}
		ctx := &Context{
			ActorID: "player:subdm",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{
						{ID: "sess:alice", ActorID: "player:alice"},
					}
				},
			},
		}
		handler := NewDMForceHandler(w)
		resolved := ResolvedCommand{
			Input:  "force alice look",
			Args:   []string{"alice", "look"},
			Parsed: commandparse.Parse("alice look force"),
			Spec:   commandspec.CommandSpec{Name: "*force"},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}
		if w.forcedPlayerID != "player:alice" || w.forcedCmd != "look" {
			t.Errorf("forced target/cmd mismatch: player=%s, cmd=%s", w.forcedPlayerID, w.forcedCmd)
		}
	})

	t.Run("dm_flush_crtobj success", func(t *testing.T) {
		w := setupWorld()
		ctx := &Context{ActorID: "player:dm"}
		handler := NewDMFlushCrtObjHandler(w)
		resolved := ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "*flushcrtobj"},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		if !w.flushedCrtObj {
			t.Error("expected FlushCrtObj to be called")
		}
	})

	t.Run("dm_create_crt success", func(t *testing.T) {
		w := setupWorld()
		ctx := &Context{ActorID: "player:subdm"}
		handler := NewDMCreateCrtHandler(w)
		resolved := ResolvedCommand{
			Parsed: commandparse.Parse("105 monster"),
			Spec:   commandspec.CommandSpec{Name: "*monster"},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		if w.spawnedProtoID != "creature:m01:5" {
			t.Errorf("spawnedProtoID = %q, want creature:m01:5", w.spawnedProtoID)
		}
	})

	t.Run("dm_stat success room", func(t *testing.T) {
		w := setupWorld()
		ctx := &Context{ActorID: "player:subdm"}
		handler := NewDMStatHandler(w)
		resolved := ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "*status"},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		if !strings.Contains(ctx.OutputString(), "방번호") {
			t.Errorf("output does not contain room stat: %q", ctx.OutputString())
		}
	})
}
