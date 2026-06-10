package command

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type unifiedDMWorld22 struct {
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
	rooms     map[model.RoomID]model.Room
	objects   map[model.ObjectInstanceID]model.ObjectInstance

	// dm_add_rom
	createdRooms []model.RoomID

	// dm_set
	roomProps     map[model.RoomID]map[string]string
	roomRandoms   map[model.RoomID]map[int]int
	roomFlags     map[model.RoomID]map[int]bool
	creatureStats map[model.CreatureID]map[string]int
	creatureProps map[model.CreatureID]map[string]string
	objectProps   map[model.ObjectInstanceID]map[string]string
	linkedExits   []linkedExitCall22
	deletedExits  []deletedExitCall22
	exitFlagsSet  []exitFlagCall22

	// dm_log
	logFileDeleted string
	logFileRead    string
	logData        map[string]string

	// dm_spy
	spies        map[model.PlayerID]model.PlayerID
	beingSpiedOn map[model.PlayerID]model.PlayerID

	// dm_loadlockout
	lockoutsLoaded bool

	// dm_finger
	fingerAddr string
	fingerName string
}

type linkedExitCall22 struct {
	FromRoomID   model.RoomID
	ToRoomID     model.RoomID
	ExitName     string
	OppositeName string
	DoubleWay    bool
}

type deletedExitCall22 struct {
	RoomID   model.RoomID
	ExitName string
}

type exitFlagCall22 struct {
	RoomID   model.RoomID
	ExitName string
	Flag     string
	Enabled  bool
}

func (w *unifiedDMWorld22) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *unifiedDMWorld22) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *unifiedDMWorld22) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *unifiedDMWorld22) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	o, ok := w.objects[id]
	return o, ok
}

func (w *unifiedDMWorld22) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	return model.ObjectPrototype{}, false
}

func (w *unifiedDMWorld22) CreateRoom(id model.RoomID) error {
	w.createdRooms = append(w.createdRooms, id)
	w.rooms[id] = model.Room{ID: id}
	return nil
}

func (w *unifiedDMWorld22) FindPlayerByName(name string) (model.Player, bool) {
	for _, p := range w.players {
		if strings.EqualFold(p.DisplayName, name) {
			return p, true
		}
	}
	return model.Player{}, false
}

func (w *unifiedDMWorld22) FindCreatureInRoom(roomID model.RoomID, name string) (model.Creature, bool) {
	for _, c := range w.creatures {
		if c.RoomID == roomID && strings.EqualFold(c.DisplayName, name) {
			return c, true
		}
	}
	return model.Creature{}, false
}

func (w *unifiedDMWorld22) FindObjectInRoom(roomID model.RoomID, name string) (model.ObjectInstance, bool) {
	for _, o := range w.objects {
		if o.Location.RoomID == roomID && strings.EqualFold(o.DisplayNameOverride, name) {
			return o, true
		}
	}
	return model.ObjectInstance{}, false
}

func (w *unifiedDMWorld22) FindObjectOnCreature(creatureID model.CreatureID, name string) (model.ObjectInstance, bool) {
	for _, o := range w.objects {
		if o.Location.CreatureID == creatureID && strings.EqualFold(o.DisplayNameOverride, name) {
			return o, true
		}
	}
	return model.ObjectInstance{}, false
}

func (w *unifiedDMWorld22) UpdateRoomProperty(id model.RoomID, key, val string) error {
	if w.roomProps == nil {
		w.roomProps = make(map[model.RoomID]map[string]string)
	}
	if w.roomProps[id] == nil {
		w.roomProps[id] = make(map[string]string)
	}
	w.roomProps[id][key] = val
	return nil
}

func (w *unifiedDMWorld22) UpdateRoomRandomCreature(id model.RoomID, idx, val int) error {
	if w.roomRandoms == nil {
		w.roomRandoms = make(map[model.RoomID]map[int]int)
	}
	if w.roomRandoms[id] == nil {
		w.roomRandoms[id] = make(map[int]int)
	}
	w.roomRandoms[id][idx] = val
	return nil
}

func (w *unifiedDMWorld22) UpdateRoomFlag(id model.RoomID, flag int, val bool) error {
	if w.roomFlags == nil {
		w.roomFlags = make(map[model.RoomID]map[int]bool)
	}
	if w.roomFlags[id] == nil {
		w.roomFlags[id] = make(map[int]bool)
	}
	w.roomFlags[id][flag] = val
	return nil
}

func (w *unifiedDMWorld22) UpdateCreatureStat(id model.CreatureID, key string, val int) error {
	if w.creatureStats == nil {
		w.creatureStats = make(map[model.CreatureID]map[string]int)
	}
	if w.creatureStats[id] == nil {
		w.creatureStats[id] = make(map[string]int)
	}
	w.creatureStats[id][key] = val
	if c, ok := w.creatures[id]; ok {
		c.Stats[key] = val
		w.creatures[id] = c
	}
	return nil
}

func (w *unifiedDMWorld22) UpdateCreatureProperty(id model.CreatureID, key, val string) error {
	if w.creatureProps == nil {
		w.creatureProps = make(map[model.CreatureID]map[string]string)
	}
	if w.creatureProps[id] == nil {
		w.creatureProps[id] = make(map[string]string)
	}
	w.creatureProps[id][key] = val
	return nil
}

func (w *unifiedDMWorld22) UpdateObjectProperty(id model.ObjectInstanceID, key, val string) error {
	if w.objectProps == nil {
		w.objectProps = make(map[model.ObjectInstanceID]map[string]string)
	}
	if w.objectProps[id] == nil {
		w.objectProps[id] = make(map[string]string)
	}
	w.objectProps[id][key] = val
	return nil
}

func (w *unifiedDMWorld22) LinkExits(fromRoomID, toRoomID model.RoomID, exitName, oppositeName string, doubleWay bool) error {
	w.linkedExits = append(w.linkedExits, linkedExitCall22{
		FromRoomID:   fromRoomID,
		ToRoomID:     toRoomID,
		ExitName:     exitName,
		OppositeName: oppositeName,
		DoubleWay:    doubleWay,
	})
	return nil
}

func (w *unifiedDMWorld22) DeleteRoomExit(roomID model.RoomID, exitName string) error {
	w.deletedExits = append(w.deletedExits, deletedExitCall22{
		RoomID:   roomID,
		ExitName: exitName,
	})
	return nil
}

func (w *unifiedDMWorld22) SetExitFlag(roomID model.RoomID, exitName string, flag string, enabled bool) (model.Exit, error) {
	w.exitFlagsSet = append(w.exitFlagsSet, exitFlagCall22{
		RoomID:   roomID,
		ExitName: exitName,
		Flag:     flag,
		Enabled:  enabled,
	})
	return model.Exit{Name: exitName}, nil
}

func (w *unifiedDMWorld22) ReadLogFile(name string) (string, error) {
	w.logFileRead = name
	if data, ok := w.logData[name]; ok {
		return data, nil
	}
	return "", fmt.Errorf("file not found")
}

func (w *unifiedDMWorld22) DeleteLogFile(name string) error {
	w.logFileDeleted = name
	return nil
}

func (w *unifiedDMWorld22) SetSpy(spyPlayerID, targetPlayerID model.PlayerID) error {
	if w.spies == nil {
		w.spies = make(map[model.PlayerID]model.PlayerID)
	}
	if w.beingSpiedOn == nil {
		w.beingSpiedOn = make(map[model.PlayerID]model.PlayerID)
	}
	w.spies[spyPlayerID] = targetPlayerID
	w.beingSpiedOn[targetPlayerID] = spyPlayerID
	return nil
}

func (w *unifiedDMWorld22) ClearSpy(spyPlayerID model.PlayerID) error {
	if target, ok := w.spies[spyPlayerID]; ok {
		delete(w.spies, spyPlayerID)
		delete(w.beingSpiedOn, target)
	}
	return nil
}

func (w *unifiedDMWorld22) IsSpying(spyPlayerID model.PlayerID) (model.PlayerID, bool) {
	target, ok := w.spies[spyPlayerID]
	return target, ok
}

func (w *unifiedDMWorld22) IsBeingSpiedOn(targetPlayerID model.PlayerID) (model.PlayerID, bool) {
	spy, ok := w.beingSpiedOn[targetPlayerID]
	return spy, ok
}

func (w *unifiedDMWorld22) UpdateCreatureTags(id model.CreatureID, add, remove []string) (model.Creature, error) {
	creature, ok := w.creatures[id]
	if !ok {
		return model.Creature{}, errors.New("creature not found")
	}
	tags := make(map[string]struct{})
	for _, t := range creature.Metadata.Tags {
		tags[t] = struct{}{}
	}
	for _, t := range remove {
		delete(tags, t)
	}
	for _, t := range add {
		tags[t] = struct{}{}
	}
	newTags := make([]string, 0, len(tags))
	for t := range tags {
		newTags = append(newTags, t)
	}
	creature.Metadata.Tags = newTags
	w.creatures[id] = creature
	return creature, nil
}

func (w *unifiedDMWorld22) SetCreatureStat(id model.CreatureID, key string, val int) error {
	creature, ok := w.creatures[id]
	if !ok {
		return errors.New("creature not found")
	}
	if creature.Stats == nil {
		creature.Stats = make(map[string]int)
	}
	creature.Stats[key] = val
	w.creatures[id] = creature
	return nil
}

func (w *unifiedDMWorld22) LoadLockouts() error {
	w.lockoutsLoaded = true
	return nil
}

func (w *unifiedDMWorld22) Finger(addr, name string) (string, error) {
	w.fingerAddr = addr
	w.fingerName = name
	return "", nil
}

func TestUnifiedDMCommands22(t *testing.T) {
	setupWorld := func() *unifiedDMWorld22 {
		return &unifiedDMWorld22{
			players: map[model.PlayerID]model.Player{
				"player:dm":    {ID: "player:dm", DisplayName: "DMPlayer", CreatureID: "creature:dm", RoomID: "room:100"},
				"player:subdm": {ID: "player:subdm", DisplayName: "SubDMPlayer", CreatureID: "creature:subdm", RoomID: "room:100"},
				"player:alice": {ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:100"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm":    {ID: "creature:dm", DisplayName: "DMPlayer", RoomID: "room:100", Stats: map[string]int{"class": 13}},
				"creature:subdm": {ID: "creature:subdm", DisplayName: "SubDMPlayer", RoomID: "room:100", Stats: map[string]int{"class": model.ClassSubDM}},
				"creature:alice": {ID: "creature:alice", DisplayName: "Alice", RoomID: "room:100", Stats: map[string]int{"class": 1}},
			},
			rooms: map[model.RoomID]model.Room{
				"room:100": {ID: "room:100", DisplayName: "광장", CreatureIDs: []model.CreatureID{"creature:dm", "creature:subdm", "creature:alice"}},
			},
			objects: make(map[model.ObjectInstanceID]model.ObjectInstance),
			logData: map[string]string{
				"log":    "normal log entry",
				"log_fl": "failure log entry",
			},
		}
	}

	t.Run("dm_add_rom success", func(t *testing.T) {
		w := setupWorld()
		ctx := &Context{ActorID: "player:dm"}
		handler := NewDMAddRomHandler(w)

		var cmd commandparse.Command
		cmd.Val[0] = 105
		resolved := ResolvedCommand{
			Parsed: cmd,
			Spec:   commandspec.CommandSpec{Name: "*add"},
		}

		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		if len(w.createdRooms) != 1 || w.createdRooms[0] != "room:00105" {
			t.Errorf("created rooms mismatch: %v", w.createdRooms)
		}
		if !strings.Contains(ctx.OutputString(), "방번호 #105 만들었습니다.") {
			t.Errorf("output mismatch: %q", ctx.OutputString())
		}
	})

	t.Run("dm_set success exits", func(t *testing.T) {
		w := setupWorld()
		// Setup room properties for mapping
		w.rooms["room:00100"] = model.Room{
			ID:         "room:00100",
			Properties: map[string]string{"roomNumber": "100"},
		}
		// Sync creature RoomID
		c := w.creatures["creature:dm"]
		c.RoomID = "room:00100"
		w.creatures["creature:dm"] = c

		w.rooms["room:00200"] = model.Room{
			ID:         "room:00200",
			Properties: map[string]string{"roomNumber": "200"},
		}

		ctx := &Context{ActorID: "player:dm"}
		handler := NewDMSetHandler(w)

		resolved := ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "dm_set"},
			Args: []string{"x", "n", "."},
			Parsed: commandparse.Command{
				Num: 4,
				Str: [commandparse.CommandMax]string{"*set", "x", "n", "."},
				Val: [commandparse.CommandMax]int64{1, 1, 200, 1},
			},
		}

		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		if len(w.linkedExits) != 2 {
			t.Errorf("expected 2 exits linked, got %d", len(w.linkedExits))
		}
	})

	t.Run("dm_log read success", func(t *testing.T) {
		w := setupWorld()
		ctx := &Context{ActorID: "player:dm"}
		handler := NewDMLogHandler(w)

		resolved := ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "*log"},
		}

		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusDoPrompt {
			t.Errorf("status = %v, want StatusDoPrompt", status)
		}
		if w.logFileRead != "log" {
			t.Errorf("expected to read 'log', got %q", w.logFileRead)
		}
		if !strings.Contains(ctx.OutputString(), "normal log entry") {
			t.Errorf("output mismatch: %q", ctx.OutputString())
		}
	})

	t.Run("dm_spy success", func(t *testing.T) {
		w := setupWorld()
		ctx := &Context{
			ActorID: "player:subdm",
			Values: map[string]any{
				"game.activeSessions": func() []testActiveSession {
					return []testActiveSession{
						{ID: "session:alice", ActorID: "player:alice"},
					}
				},
			},
		}
		handler := NewDMSpyHandler(w)

		resolved := ResolvedCommand{
			Args: []string{"Alice"},
			Spec: commandspec.CommandSpec{Name: "*spy"},
		}

		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}

		target, ok := w.IsSpying("player:subdm")
		if !ok || target != "player:alice" {
			t.Errorf("expected player:subdm to be spying on player:alice, got %q", target)
		}
	})

	t.Run("dm_loadlockout success", func(t *testing.T) {
		w := setupWorld()
		ctx := &Context{ActorID: "player:dm"}
		handler := NewDMLoadLockoutHandler(w)

		resolved := ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "*lock"},
		}

		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		if !w.lockoutsLoaded {
			t.Error("expected lockouts to be loaded")
		}
	})

	t.Run("dm_finger success name", func(t *testing.T) {
		w := setupWorld()
		ctx := &Context{
			ActorID: "player:subdm",
			Values: map[string]any{
				"game.activeSessions": func() []testActiveSession {
					return []testActiveSession{
						{ID: "session:alice", ActorID: "player:alice"},
					}
				},
			},
		}
		handler := NewDMFingerHandler(w)

		resolved := ResolvedCommand{
			Args: []string{"Alice"},
			Spec: commandspec.CommandSpec{Name: "*finger"},
		}

		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		if w.fingerAddr == "" {
			t.Error("expected finger address to be resolved")
		}
	})
}
