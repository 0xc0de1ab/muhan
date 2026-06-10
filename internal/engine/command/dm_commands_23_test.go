package command

import (
	"errors"
	"strings"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type unifiedDMWorld23 struct {
	players    map[model.PlayerID]model.Player
	creatures  map[model.CreatureID]model.Creature
	rooms      map[model.RoomID]model.Room
	broadcasts []string

	// list
	listArgs   []string
	listOutput string

	// wander
	wanderInterval int

	// ship
	shipInterval int64
	forcedSail   bool
	timeToSail   int64

	// silence
	dailyCur map[model.CreatureID]int
	dailyMax map[model.CreatureID]int
}

func (w *unifiedDMWorld23) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *unifiedDMWorld23) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *unifiedDMWorld23) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *unifiedDMWorld23) List(args []string) (string, error) {
	w.listArgs = args
	return w.listOutput, nil
}

func (w *unifiedDMWorld23) CacheStats() (rooms, monsters, objects int) {
	return 10, 20, 30
}

func (w *unifiedDMWorld23) WanderInterval() int {
	return w.wanderInterval
}

func (w *unifiedDMWorld23) SetWanderInterval(val int) {
	w.wanderInterval = val
}

func (w *unifiedDMWorld23) PlayerCounts() (active, queued int) {
	return len(w.players), 1
}

func (w *unifiedDMWorld23) ShutdownTimeRemaining() int64 {
	return 3600
}

func (w *unifiedDMWorld23) ShipSailingInterval() int64 {
	return w.shipInterval
}

func (w *unifiedDMWorld23) SetShipSailingInterval(val int64) {
	w.shipInterval = val
}

func (w *unifiedDMWorld23) TimeToSail() int64 {
	return w.timeToSail
}

func (w *unifiedDMWorld23) ForceShipSail() {
	w.forcedSail = true
}

func (w *unifiedDMWorld23) FindPlayerByName(name string) (model.Player, bool) {
	for _, p := range w.players {
		if strings.EqualFold(p.DisplayName, name) {
			return p, true
		}
	}
	return model.Player{}, false
}

func (w *unifiedDMWorld23) GetDailyBroadcastCount(id model.CreatureID) (cur, max int) {
	c := 10
	m := 10
	if val, ok := w.dailyCur[id]; ok {
		c = val
	}
	if val, ok := w.dailyMax[id]; ok {
		m = val
	}
	return c, m
}

func (w *unifiedDMWorld23) SetDailyBroadcastCount(id model.CreatureID, val int) error {
	if w.dailyCur == nil {
		w.dailyCur = make(map[model.CreatureID]int)
	}
	w.dailyCur[id] = val
	return nil
}

func (w *unifiedDMWorld23) BroadcastAll(message string) error {
	w.broadcasts = append(w.broadcasts, message)
	return nil
}

func (w *unifiedDMWorld23) UpdateRoomDescription(roomID model.RoomID, field, val string) error {
	room, ok := w.rooms[roomID]
	if !ok {
		return errors.New("room not found")
	}
	if field == "short" {
		room.ShortDescription = val
	} else if field == "long" {
		room.LongDescription = val
	}
	w.rooms[roomID] = room
	return nil
}

func TestUnifiedDMCommands23(t *testing.T) {
	setupWorld := func() *unifiedDMWorld23 {
		return &unifiedDMWorld23{
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
				"room:100": {
					ID:               "room:100",
					DisplayName:      "광장",
					ShortDescription: "광장 숏",
					LongDescription:  "광장 롱",
					CreatureIDs:      []model.CreatureID{"creature:dm", "creature:subdm", "creature:alice"},
				},
			},
			wanderInterval: 30,
			shipInterval:   600,
			timeToSail:     300,
			dailyCur:       map[model.CreatureID]int{"creature:alice": 5},
			dailyMax:       map[model.CreatureID]int{"creature:alice": 10},
		}
	}

	t.Run("dm_list success", func(t *testing.T) {
		w := setupWorld()
		w.listOutput = "monster list\n"
		ctx := &Context{ActorID: "player:subdm"}
		handler := NewDMListHandler(w)
		resolved := ResolvedCommand{
			Args: []string{"monster"},
			Parsed: commandparse.Command{
				Num: 2,
				Str: [commandparse.CommandMax]string{"*list", "monster"},
			},
			Spec: commandspec.CommandSpec{Name: "*list"},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		if len(w.listArgs) != 1 || w.listArgs[0] != "monster" {
			t.Errorf("expected list args to be [monster], got %v", w.listArgs)
		}
		if ctx.OutputString() != "monster list\n" {
			t.Errorf("output = %q, want monster list", ctx.OutputString())
		}
	})

	t.Run("dm_info success", func(t *testing.T) {
		w := setupWorld()
		ctx := &Context{ActorID: "player:subdm"}
		handler := NewDMInfoHandler(w)
		resolved := ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "*info"},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		output := ctx.OutputString()
		if !strings.Contains(output, "Internal Cache Queue Sizes:") {
			t.Errorf("unexpected output: %q", output)
		}
	})

	t.Run("dm_param success display", func(t *testing.T) {
		w := setupWorld()
		ctx := &Context{ActorID: "player:dm"}
		handler := NewDMParamHandler(w)
		resolved := ResolvedCommand{
			Args: []string{"d"},
			Parsed: commandparse.Command{
				Num: 2,
				Str: [commandparse.CommandMax]string{"*parameter", "d"},
			},
			Spec: commandspec.CommandSpec{Name: "*parameter"},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}
		output := ctx.OutputString()
		if !strings.Contains(output, "Random Update:") {
			t.Errorf("unexpected output: %q", output)
		}
	})

	t.Run("dm_silence success clear", func(t *testing.T) {
		w := setupWorld()
		ctx := &Context{
			ActorID: "player:dm",
			Values: map[string]any{
				"game.activeSessions": func() []testActiveSession {
					return []testActiveSession{
						{ID: "session:alice", ActorID: "player:alice"},
					}
				},
			},
		}
		handler := NewDMSilenceHandler(w)
		resolved := ResolvedCommand{
			Args: []string{"Alice"},
			Parsed: commandparse.Command{
				Num: 2,
				Str: [commandparse.CommandMax]string{"*silence", "Alice"},
			},
			Spec: commandspec.CommandSpec{Name: "*silence"},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		if w.dailyCur["creature:alice"] != 0 {
			t.Errorf("expected Alice broadcast cur to be 0, got %d", w.dailyCur["creature:alice"])
		}
		if !strings.Contains(ctx.OutputString(), "은 조용해 졌습니다.") {
			t.Errorf("unexpected output: %q", ctx.OutputString())
		}
	})

	t.Run("dm_broadecho success simple", func(t *testing.T) {
		w := setupWorld()
		ctx := &Context{ActorID: "player:subdm"}
		handler := NewDMBroadechoHandler(w)
		resolved := ResolvedCommand{
			Input: "*broad hello all",
			Args:  []string{"hello", "all"},
			Spec:  commandspec.CommandSpec{Name: "*broad"},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		if len(w.broadcasts) != 1 || !strings.Contains(w.broadcasts[0], "hello all") {
			t.Errorf("expected broadcast to be 'hello all', got %v", w.broadcasts)
		}
	})

	t.Run("dm_replace success short", func(t *testing.T) {
		w := setupWorld()
		ctx := &Context{ActorID: "player:dm"}
		handler := NewDMReplaceHandler(w)
		resolved := ResolvedCommand{
			Input: "*replace 숏 replacement",
			Args:  []string{"숏", "replacement"},
			Parsed: commandparse.Command{
				Num: 3,
				Str: [commandparse.CommandMax]string{"*replace", "숏", "replacement"},
			},
			Spec: commandspec.CommandSpec{Name: "*replace"},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("status = %v, want StatusDefault", status)
		}
		room := w.rooms["room:100"]
		if room.ShortDescription != "광장 replacement" {
			t.Errorf("expected short desc to be '광장 replacement', got %q", room.ShortDescription)
		}
	})
}
