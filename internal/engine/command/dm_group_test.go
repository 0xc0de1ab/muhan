package command

import (
	"strings"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/world/model"
)

type mockDMGroupMemory struct {
	leaders   map[string]string
	followers map[string][]string
}

func (m *mockDMGroupMemory) LeaderOf(id string) (string, bool) {
	l, ok := m.leaders[id]
	return l, ok
}

func (m *mockDMGroupMemory) FollowersOf(id string) []string {
	return m.followers[id]
}

type mockDMGroupWorld struct {
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
	rooms     map[model.RoomID]model.Room
}

func (m *mockDMGroupWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMGroupWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMGroupWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := m.rooms[id]
	return r, ok
}

func (m *mockDMGroupWorld) FindPlayerByName(name string) (model.Player, bool) {
	for _, p := range m.players {
		if strings.EqualFold(p.DisplayName, name) || strings.EqualFold(string(p.ID), name) {
			return p, true
		}
	}
	return model.Player{}, false
}

func (m *mockDMGroupWorld) FindCreatureInRoom(roomID model.RoomID, name string) (model.Creature, bool) {
	for _, crt := range m.creatures {
		if crt.RoomID == roomID && (strings.EqualFold(crt.DisplayName, name) || strings.EqualFold(string(crt.ID), name)) {
			return crt, true
		}
	}
	return model.Creature{}, false
}

func (m *mockDMGroupWorld) FindCreatureByName(roomID model.RoomID, name string, count int) (model.Creature, bool) {
	if count < 1 {
		count = 1
	}
	room, ok := m.rooms[roomID]
	if !ok {
		return model.Creature{}, false
	}
	seen := 0
	for _, creatureID := range room.CreatureIDs {
		crt, ok := m.creatures[creatureID]
		if !ok || crt.RoomID != roomID {
			continue
		}
		if strings.EqualFold(crt.DisplayName, name) || strings.EqualFold(string(crt.ID), name) {
			seen++
			if seen == count {
				return crt, true
			}
		}
	}
	return model.Creature{}, false
}

func TestDMGroup(t *testing.T) {
	t.Run("deny access to non-DM", func(t *testing.T) {
		world := &mockDMGroupWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID:     "creature:caster",
					RoomID: "room:1",
					Stats: map[string]int{
						"class": 9, // < 10 (SUB_DM)
					},
				},
			},
		}

		ctx := &Context{
			ActorID: "player:caster",
		}

		resolved := ResolvedCommand{
			Parsed: commandparse.Command{
				Num: 2,
				Str: [commandparse.CommandMax]string{"*group", "Alice"},
			},
		}

		handler := NewDMGroupHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}

		got := ctx.OutputString()
		if got != "" {
			t.Errorf("output = %q, want no permission output", got)
		}
	})

	t.Run("argument count < 2", func(t *testing.T) {
		world := &mockDMGroupWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID:     "creature:caster",
					RoomID: "room:1",
					Stats: map[string]int{
						"class": model.ClassSubDM,
					},
				},
			},
		}

		ctx := &Context{
			ActorID: "player:caster",
		}

		resolved := ResolvedCommand{
			Parsed: commandparse.Command{
				Num: 1,
				Str: [commandparse.CommandMax]string{"*group"},
			},
		}

		handler := NewDMGroupHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}

		got := ctx.OutputString()
		want := "누구의 그룹을 봅니까?\n"
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("target not found", func(t *testing.T) {
		world := &mockDMGroupWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID:     "creature:caster",
					RoomID: "room:1",
					Stats: map[string]int{
						"class": model.ClassSubDM,
					},
				},
			},
		}

		ctx := &Context{
			ActorID: "player:caster",
		}

		resolved := ResolvedCommand{
			Parsed: commandparse.Command{
				Num: 2,
				Str: [commandparse.CommandMax]string{"*group", "Nobody"},
			},
		}

		handler := NewDMGroupHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("status = %v, want StatusPrompt", status)
		}

		got := ctx.OutputString()
		want := "그런 사람이 없습니다.\n"
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("same-room player creature still requires active find_who match", func(t *testing.T) {
		world := &mockDMGroupWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
				"player:target": {ID: "player:target", CreatureID: "creature:target", DisplayName: "Alice", RoomID: "room:1"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID:     "creature:caster",
					RoomID: "room:1",
					Stats:  map[string]int{"class": model.ClassSubDM},
				},
				"creature:target": {
					ID:          "creature:target",
					Kind:        model.CreatureKindPlayer,
					PlayerID:    "player:target",
					DisplayName: "Alice",
					RoomID:      "room:1",
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {
					ID:          "room:1",
					CreatureIDs: []model.CreatureID{"creature:target"},
				},
			},
		}

		ctx := &Context{
			ActorID: "player:caster",
			Values: map[string]any{
				"game.activeSessions": func() []testActiveSession {
					return []testActiveSession{
						{ID: "session:caster", ActorID: "player:caster"},
					}
				},
			},
		}

		resolved := ResolvedCommand{
			Parsed: commandparse.Command{
				Num: 2,
				Str: [commandparse.CommandMax]string{"*group", "Alice"},
			},
		}

		status, err := NewDMGroupHandler(world)(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Fatalf("status = %v, want StatusPrompt", status)
		}
		if got, want := ctx.OutputString(), "그런 사람이 없습니다.\n"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("target found - no leader, no followers", func(t *testing.T) {
		world := &mockDMGroupWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
				"player:target": {ID: "player:target", CreatureID: "creature:target", DisplayName: "Alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID:     "creature:caster",
					RoomID: "room:1",
					Stats: map[string]int{
						"class": model.ClassSubDM,
					},
				},
				"creature:target": {
					ID:          "creature:target",
					PlayerID:    "player:target",
					DisplayName: "Alice",
					RoomID:      "room:1",
				},
			},
		}

		groupMem := &mockDMGroupMemory{
			leaders:   make(map[string]string),
			followers: make(map[string][]string),
		}

		ctx := &Context{
			ActorID: "player:caster",
			Values: map[string]any{
				"game.groupMemory": groupMem,
				"game.activeSessions": func() []testActiveSession {
					return []testActiveSession{
						{ID: "session:target", ActorID: "player:target"},
					}
				},
			},
		}

		resolved := ResolvedCommand{
			Parsed: commandparse.Command{
				Num: 2,
				Str: [commandparse.CommandMax]string{"*group", "Alice"},
			},
		}

		handler := NewDMGroupHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := ctx.OutputString()
		want := "Alice이 따르고 있는 사람: 없음\nAlice의 그룹: 없음.\n"
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("parsed target ordinal selects duplicate room monster like C", func(t *testing.T) {
		world := &mockDMGroupWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID:     "creature:caster",
					RoomID: "room:1",
					Stats:  map[string]int{"class": model.ClassSubDM},
				},
				"creature:wolf1": {
					ID:          "creature:wolf1",
					DisplayName: "늑대",
					RoomID:      "room:1",
				},
				"creature:wolf2": {
					ID:          "creature:wolf2",
					DisplayName: "늑대",
					RoomID:      "room:1",
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {
					ID:          "room:1",
					CreatureIDs: []model.CreatureID{"creature:wolf1", "creature:wolf2"},
				},
			},
		}
		groupMem := &mockDMGroupMemory{
			leaders:   map[string]string{},
			followers: map[string][]string{},
		}
		ctx := &Context{
			ActorID: "player:caster",
			Values:  map[string]any{"game.groupMemory": groupMem},
		}
		resolved := ResolvedCommand{
			Input: "2 늑대 *group",
			Parsed: commandparse.Command{
				Num: 2,
				Str: [commandparse.CommandMax]string{"*group", "늑대"},
				Val: [commandparse.CommandMax]int64{1, 2},
			},
		}

		status, err := NewDMGroupHandler(world)(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Fatalf("status = %v, want StatusDefault", status)
		}
		if got, want := ctx.OutputString(), "늑대이 따르고 있는 사람: 없음\n늑대의 그룹: 없음.\n"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("target found - has leader and followers", func(t *testing.T) {
		world := &mockDMGroupWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
				"player:leader": {ID: "player:leader", CreatureID: "creature:leader", DisplayName: "Bob"},
				"player:target": {ID: "player:target", CreatureID: "creature:target", DisplayName: "Alice"},
				"player:fol1":   {ID: "player:fol1", CreatureID: "creature:fol1", DisplayName: "Charlie"},
				"player:fol2":   {ID: "player:fol2", CreatureID: "creature:fol2", DisplayName: "David"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID:     "creature:caster",
					RoomID: "room:1",
					Stats: map[string]int{
						"class": model.ClassSubDM,
					},
				},
				"creature:leader": {
					ID:          "creature:leader",
					PlayerID:    "player:leader",
					DisplayName: "Bob",
				},
				"creature:target": {
					ID:          "creature:target",
					PlayerID:    "player:target",
					DisplayName: "Alice",
					RoomID:      "room:1",
				},
				"creature:fol1": {
					ID:          "creature:fol1",
					PlayerID:    "player:fol1",
					DisplayName: "Charlie",
				},
				"creature:fol2": {
					ID:          "creature:fol2",
					PlayerID:    "player:fol2",
					DisplayName: "David",
				},
			},
		}

		groupMem := &mockDMGroupMemory{
			leaders: map[string]string{
				"player:target": "player:leader",
			},
			followers: map[string][]string{
				"player:target": {"player:fol1", "player:fol2"},
			},
		}

		ctx := &Context{
			ActorID: "player:caster",
			Values: map[string]any{
				"game.groupMemory": groupMem,
				"game.activeSessions": func() []testActiveSession {
					return []testActiveSession{
						{ID: "session:target", ActorID: "player:target"},
					}
				},
			},
		}

		resolved := ResolvedCommand{
			Parsed: commandparse.Command{
				Num: 2,
				Str: [commandparse.CommandMax]string{"*group", "Alice"},
			},
		}

		handler := NewDMGroupHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := ctx.OutputString()
		want := "Alice이 따르고 있는 사람: Bob\nAlice의 그룹: Charlie, David.\n"
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("saved player without active session is not found like C find_who", func(t *testing.T) {
		world := &mockDMGroupWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
				"player:target": {ID: "player:target", CreatureID: "creature:target", DisplayName: "Alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID:     "creature:caster",
					RoomID: "room:1",
					Stats:  map[string]int{"class": model.ClassSubDM},
				},
				"creature:target": {
					ID:          "creature:target",
					PlayerID:    "player:target",
					DisplayName: "Alice",
					RoomID:      "room:2",
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {ID: "room:1"},
			},
		}

		ctx := &Context{
			ActorID: "player:caster",
			Values: map[string]any{
				"game.activeSessions": func() []testActiveSession {
					return []testActiveSession{
						{ID: "session:caster", ActorID: "player:caster"},
					}
				},
			},
		}

		resolved := ResolvedCommand{
			Parsed: commandparse.Command{
				Num: 2,
				Str: [commandparse.CommandMax]string{"*group", "Alice"},
			},
		}

		status, err := NewDMGroupHandler(world)(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Fatalf("status = %v, want StatusPrompt", status)
		}
		if got, want := ctx.OutputString(), "그런 사람이 없습니다.\n"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	})

	t.Run("monster ordinal selects matching duplicate creature", func(t *testing.T) {
		world := &mockDMGroupWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID:     "creature:caster",
					RoomID: "room:1",
					Stats:  map[string]int{"class": model.ClassSubDM},
				},
				"creature:goblin1": {
					ID:          "creature:goblin1",
					DisplayName: "Goblin",
					RoomID:      "room:1",
				},
				"creature:goblin2": {
					ID:          "creature:goblin2",
					DisplayName: "Goblin",
					RoomID:      "room:1",
				},
				"creature:follower": {
					ID:          "creature:follower",
					DisplayName: "Minion",
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {
					ID:          "room:1",
					CreatureIDs: []model.CreatureID{"creature:goblin1", "creature:goblin2"},
				},
			},
		}

		groupMem := &mockDMGroupMemory{
			leaders: make(map[string]string),
			followers: map[string][]string{
				"creature:goblin2": {"creature:follower"},
			},
		}

		ctx := &Context{
			ActorID: "player:caster",
			Values: map[string]any{
				"game.groupMemory": groupMem,
			},
		}

		resolved := ResolvedCommand{
			Parsed: commandparse.Command{
				Num: 2,
				Str: [commandparse.CommandMax]string{"*group", "Goblin"},
			},
			Values: []int64{2},
		}

		handler := NewDMGroupHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := ctx.OutputString()
		want := "Goblin이 따르고 있는 사람: 없음\nGoblin의 그룹: Minion.\n"
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})
}
