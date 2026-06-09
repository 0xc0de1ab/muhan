package command

import (
	"errors"
	"strings"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type unifiedDMWorld24 struct {
	players      map[model.PlayerID]model.Player
	creatures    map[model.CreatureID]model.Creature
	rooms        map[model.RoomID]model.Room
	roomPlayers  map[model.RoomID][]model.Player
	movedPlayers map[model.PlayerID]model.RoomID
	broadcasts   []string
}

func (w *unifiedDMWorld24) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *unifiedDMWorld24) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *unifiedDMWorld24) Room(id model.RoomID) (model.Room, bool) {
	r, ok := w.rooms[id]
	return r, ok
}

func (w *unifiedDMWorld24) SetRoomName(roomID model.RoomID, name string) error {
	room, ok := w.rooms[roomID]
	if !ok {
		return errors.New("room not found")
	}
	room.DisplayName = name
	w.rooms[roomID] = room
	return nil
}

func (w *unifiedDMWorld24) UpdateRoomDescription(roomID model.RoomID, field string, val string) error {
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

func (w *unifiedDMWorld24) Players() []model.Player {
	var list []model.Player
	for _, p := range w.players {
		list = append(list, p)
	}
	return list
}

func (w *unifiedDMWorld24) RoomPlayers(roomID model.RoomID) []model.Player {
	return w.roomPlayers[roomID]
}

func (w *unifiedDMWorld24) UpdateCreatureTags(creatureID model.CreatureID, add []string, remove []string) (model.Creature, error) {
	c, ok := w.creatures[creatureID]
	if !ok {
		return model.Creature{}, errors.New("creature not found")
	}
	tags := make(map[string]bool)
	for _, t := range c.Metadata.Tags {
		tags[t] = true
	}
	for _, t := range add {
		tags[t] = true
	}
	for _, t := range remove {
		delete(tags, t)
	}
	var newTags []string
	for t := range tags {
		newTags = append(newTags, t)
	}
	c.Metadata.Tags = newTags
	w.creatures[creatureID] = c
	return c, nil
}

func (w *unifiedDMWorld24) UpdateCreatureStat(creatureID model.CreatureID, stat string, val int) error {
	c, ok := w.creatures[creatureID]
	if !ok {
		return errors.New("creature not found")
	}
	if c.Stats == nil {
		c.Stats = make(map[string]int)
	}
	c.Stats[stat] = val
	w.creatures[creatureID] = c
	return nil
}

func (w *unifiedDMWorld24) MovePlayerToRoom(playerID model.PlayerID, toRoomID model.RoomID) error {
	if w.movedPlayers == nil {
		w.movedPlayers = make(map[model.PlayerID]model.RoomID)
	}
	w.movedPlayers[playerID] = toRoomID
	return nil
}

func (w *unifiedDMWorld24) BroadcastAll(msg string) error {
	w.broadcasts = append(w.broadcasts, msg)
	return nil
}

func (w *unifiedDMWorld24) FindPlayerByName(name string) (model.Player, bool) {
	for _, p := range w.players {
		if strings.EqualFold(p.DisplayName, name) {
			return p, true
		}
	}
	return model.Player{}, false
}

func (w *unifiedDMWorld24) FindCreatureInRoom(roomID model.RoomID, name string) (model.Creature, bool) {
	for _, c := range w.creatures {
		if c.RoomID == roomID && strings.EqualFold(c.DisplayName, name) {
			return c, true
		}
	}
	return model.Creature{}, false
}

func TestUnifiedDMCommands24(t *testing.T) {
	setupWorld := func() *unifiedDMWorld24 {
		return &unifiedDMWorld24{
			players: map[model.PlayerID]model.Player{
				"player:dm":    {ID: "player:dm", DisplayName: "DMPlayer", CreatureID: "creature:dm", RoomID: "room:100"},
				"player:subdm": {ID: "player:subdm", DisplayName: "SubDMPlayer", CreatureID: "creature:subdm", RoomID: "room:100"},
				"player:alice": {ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:100"},
				"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob", RoomID: "room:100"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm":    {ID: "creature:dm", DisplayName: "DMPlayer", RoomID: "room:100", PlayerID: "player:dm", Stats: map[string]int{"class": 13}},
				"creature:subdm": {ID: "creature:subdm", DisplayName: "SubDMPlayer", RoomID: "room:100", PlayerID: "player:subdm", Stats: map[string]int{"class": legacyClassSubDM}},
				"creature:alice": {ID: "creature:alice", DisplayName: "Alice", RoomID: "room:100", PlayerID: "player:alice", Stats: map[string]int{"class": 1}},
				"creature:bob": {
					ID:          "creature:bob",
					DisplayName: "Bob",
					RoomID:      "room:100",
					PlayerID:    "player:bob",
					Stats: map[string]int{
						"class":     1,
						"hpCurrent": 50,
						"hpMax":     100,
					},
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:100": {
					ID:               "room:100",
					DisplayName:      "Old Room",
					ShortDescription: "Short Desc",
					LongDescription:  "Long Desc",
				},
			},
			roomPlayers: map[model.RoomID][]model.Player{
				"room:100": {
					{ID: "player:dm", DisplayName: "DMPlayer", CreatureID: "creature:dm", RoomID: "room:100"},
					{ID: "player:subdm", DisplayName: "SubDMPlayer", CreatureID: "creature:subdm", RoomID: "room:100"},
					{ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:100"},
					{ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob", RoomID: "room:100"},
				},
			},
		}
	}

	t.Run("dm_nameroom", func(t *testing.T) {
		w := setupWorld()
		handler := NewDMNameroomHandler(w)

		ctx := &Context{
			ActorID:   "player:dm",
			SessionID: "session-dm",
		}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "dm_nameroom"},
			Input: "dm_nameroom New Room Name",
			Parsed: commandparse.Command{
				Num: 2,
				Str: [7]string{"dm_nameroom", "New Room Name"},
			},
		}

		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}

		room, _ := w.Room("room:100")
		if room.DisplayName != "New Room Name" {
			t.Errorf("expected room display name 'New Room Name', got %q", room.DisplayName)
		}
		if !strings.Contains(ctx.OutputString(), "이름을 변경하였습니다.\n") {
			t.Errorf("missing success output, got %q", ctx.OutputString())
		}
	})

	t.Run("dm_append", func(t *testing.T) {
		w := setupWorld()
		handler := NewDMAppendHandler(w)

		ctx := &Context{
			ActorID:   "player:dm",
			SessionID: "session-dm",
		}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "dm_append"},
			Input: "dm_append Appended Text",
			Parsed: commandparse.Command{
				Num: 2,
				Str: [7]string{"dm_append", "Appended Text"},
			},
		}

		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}

		room, _ := w.Room("room:100")
		if room.LongDescription != "Long Desc\nAppended Text" {
			t.Errorf("expected 'Long Desc\\nAppended Text', got %q", room.LongDescription)
		}
	})

	t.Run("dm_prepend", func(t *testing.T) {
		w := setupWorld()
		handler := NewDMPrependHandler(w)

		ctx := &Context{
			ActorID:   "player:dm",
			SessionID: "session-dm",
		}
		resolved := ResolvedCommand{
			Spec:  commandspec.CommandSpec{Name: "dm_prepend"},
			Input: "dm_prepend Prepended Text",
			Parsed: commandparse.Command{
				Num: 2,
				Str: [7]string{"dm_prepend", "Prepended Text"},
			},
		}

		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}

		room, _ := w.Room("room:100")
		if room.LongDescription != "Prepended Text\nLong Desc" {
			t.Errorf("expected 'Prepended Text\\nLong Desc', got %q", room.LongDescription)
		}
	})

	t.Run("dm_cast", func(t *testing.T) {
		w := setupWorld()
		handler := NewDMCastHandler(w)

		ctx := &Context{
			ActorID:   "player:subdm",
			SessionID: "session-subdm",
		}

		var roomBroadcasts []string
		var sentMessages = make(map[string][]string)
		ctx.Values = map[string]any{
			ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
				roomBroadcasts = append(roomBroadcasts, text)
				return nil
			}),
			"game.activeSessions": func() []any {
				return []any{
					struct {
						ID      string
						ActorID string
					}{ID: "session-dm", ActorID: "player:dm"},
					struct {
						ID      string
						ActorID string
					}{ID: "session-subdm", ActorID: "player:subdm"},
					struct {
						ID      string
						ActorID string
					}{ID: "session-alice", ActorID: "player:alice"},
					struct {
						ID      string
						ActorID string
					}{ID: "session-bob", ActorID: "player:bob"},
				}
			},
			"game.sendToSession": func(id string, cmd struct{ Write string }) error {
				sentMessages[id] = append(sentMessages[id], cmd.Write)
				return nil
			},
		}

		resolved := ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "dm_cast"},
			Parsed: commandparse.Command{
				Num: 3,
				Str: [7]string{"dm_cast", "-r", "회복"},
			},
		}

		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}

		bobCrt, _ := w.Creature("creature:bob")
		if bobCrt.Stats["hpCurrent"] <= 50 {
			t.Errorf("expected Bob to be healed, but hpCurrent is %d", bobCrt.Stats["hpCurrent"])
		}

		if len(roomBroadcasts) != 1 {
			t.Errorf("expected 1 room broadcast, got %d", len(roomBroadcasts))
		}
	})

	t.Run("dm_group", func(t *testing.T) {
		w := setupWorld()
		handler := NewDMGroupHandler(w)

		ctx := &Context{
			ActorID:   "player:subdm",
			SessionID: "session-subdm",
			Values: map[string]any{
				"game.groupMemory": &mockDMGroupMemory{
					leaders:   map[string]string{"player:bob": "player:alice"},
					followers: map[string][]string{"player:alice": {"player:bob"}},
				},
				"game.activeSessions": func() []testActiveSession {
					return []testActiveSession{
						{ID: "session-alice", ActorID: "player:alice"},
						{ID: "session-bob", ActorID: "player:bob"},
					}
				},
			},
		}

		resolved := ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "dm_group"},
			Parsed: commandparse.Command{
				Num: 2,
				Str: [7]string{"dm_group", "Alice"},
			},
		}

		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}

		output := ctx.OutputString()
		if !strings.Contains(output, "Alice") || !strings.Contains(output, "Bob") {
			t.Errorf("missing group details in output: %q", output)
		}
	})

	t.Run("notepad", func(t *testing.T) {
		w := setupWorld()
		tempDir := t.TempDir()
		handler := NewNotepadHandler(w, tempDir)

		var pending PendingLineHandler
		ctx := &Context{
			ActorID:   "player:subdm",
			SessionID: "session-subdm",
			Values: map[string]any{
				ContextPendingLineKey: func(handler PendingLineHandler) {
					pending = handler
				},
			},
		}

		// 1. View empty notepad (should write nothing to ctx and return DOPROMPT)
		resolved := ResolvedCommand{
			Spec:   commandspec.CommandSpec{Name: "notepad"},
			Parsed: commandparse.Command{Num: 1, Str: [7]string{"notepad"}},
		}
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDoPrompt {
			t.Errorf("expected StatusDoPrompt, got %v", status)
		}
		if ctx.OutputString() != "" {
			t.Errorf("expected empty output, got %q", ctx.OutputString())
		}

		// 2. Append note
		ctx = &Context{
			ActorID:   "player:subdm",
			SessionID: "session-subdm",
			Values: map[string]any{
				ContextPendingLineKey: func(handler PendingLineHandler) {
					pending = handler
				},
			},
		}
		resolved = ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "notepad"},
			Parsed: commandparse.Command{
				Num: 2,
				Str: [7]string{"notepad", "a"},
			},
			Args: []string{"a"},
		}
		status, err = handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDoPrompt {
			t.Errorf("expected StatusDoPrompt, got %v", status)
		}

		// 3. Clear notepad
		ctx = &Context{
			ActorID:   "player:subdm",
			SessionID: "session-subdm",
			Values: map[string]any{
				ContextPendingLineKey: func(handler PendingLineHandler) {
					pending = handler
				},
			},
		}
		resolved = ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "notepad"},
			Parsed: commandparse.Command{
				Num: 2,
				Str: [7]string{"notepad", "d"},
			},
			Args: []string{"d"},
		}
		status, err = handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("expected StatusPrompt, got %v", status)
		}
		_ = pending
	})
}
