package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockListEnmWorld struct {
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
	rooms     map[model.RoomID]model.Room
	enemies   map[model.CreatureID][]string
}

func (m *mockListEnmWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockListEnmWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockListEnmWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := m.rooms[id]
	return r, ok
}

func (m *mockListEnmWorld) FindCreatureInRoom(roomID model.RoomID, name string) (model.Creature, bool) {
	for _, c := range m.creatures {
		if c.RoomID == roomID && strings.EqualFold(c.DisplayName, name) {
			return c, true
		}
	}
	return model.Creature{}, false
}

func (m *mockListEnmWorld) FindCreatureByName(roomID model.RoomID, name string, count int) (model.Creature, bool) {
	if count < 1 {
		count = 1
	}
	room, ok := m.rooms[roomID]
	if !ok {
		return model.Creature{}, false
	}
	seen := 0
	for _, creatureID := range room.CreatureIDs {
		c, ok := m.creatures[creatureID]
		if !ok || c.RoomID != roomID {
			continue
		}
		if strings.EqualFold(c.DisplayName, name) {
			seen++
			if seen == count {
				return c, true
			}
		}
	}
	return model.Creature{}, false
}

func (m *mockListEnmWorld) CreatureEnemies(id model.CreatureID) ([]string, error) {
	return m.enemies[id], nil
}

func TestListEnm(t *testing.T) {
	setupWorld := func() *mockListEnmWorld {
		w := &mockListEnmWorld{
			players:   make(map[model.PlayerID]model.Player),
			creatures: make(map[model.CreatureID]model.Creature),
			rooms:     make(map[model.RoomID]model.Room),
			enemies:   make(map[model.CreatureID][]string),
		}

		w.players["player:caretaker"] = model.Player{
			ID:          "player:caretaker",
			DisplayName: "관리자",
			CreatureID:  "creature:caretaker",
			RoomID:      "room:1",
		}
		w.creatures["creature:caretaker"] = model.Creature{
			ID:          "creature:caretaker",
			DisplayName: "관리자",
			RoomID:      "room:1",
			Stats:       map[string]int{"class": model.ClassSubDM}, // SUB_DM
		}

		w.creatures["creature:monster"] = model.Creature{
			ID:          "creature:monster",
			DisplayName: "늑대",
			RoomID:      "room:1",
		}

		w.rooms["room:1"] = model.Room{
			ID:          "room:1",
			CreatureIDs: []model.CreatureID{"creature:monster"},
		}

		return w
	}

	t.Run("Insufficient privileges", func(t *testing.T) {
		w := setupWorld()
		crt := w.creatures["creature:caretaker"]
		crt.Stats["class"] = 9
		w.creatures["creature:caretaker"] = crt

		handler := NewListEnmHandler(w)
		ctx := &Context{ActorID: "player:caretaker"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"늑대"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected status StatusDefault, got %v", status)
		}
		if got := ctx.OutputString(); got != "" {
			t.Errorf("output = %q, want no permission output", got)
		}
	})

	t.Run("Creature not found", func(t *testing.T) {
		w := setupWorld()
		handler := NewListEnmHandler(w)
		ctx := &Context{ActorID: "player:caretaker"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"호랑이"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected status StatusDefault, got %v", status)
		}
		if !strings.Contains(ctx.OutputString(), "그런 괴물이 없습니다.") {
			t.Errorf("expected monster not found message, got %q", ctx.OutputString())
		}
	})

	t.Run("Player creature is not a monster target", func(t *testing.T) {
		w := setupWorld()
		w.players["player:bob"] = model.Player{
			ID:          "player:bob",
			DisplayName: "홍길동",
			CreatureID:  "creature:bob",
			RoomID:      "room:1",
		}
		w.creatures["creature:bob"] = model.Creature{
			ID:          "creature:bob",
			Kind:        model.CreatureKindPlayer,
			PlayerID:    "player:bob",
			DisplayName: "홍길동",
			RoomID:      "room:1",
		}
		room := w.rooms["room:1"]
		room.CreatureIDs = []model.CreatureID{"creature:bob", "creature:monster"}
		w.rooms["room:1"] = room

		handler := NewListEnmHandler(w)
		ctx := &Context{ActorID: "player:caretaker"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"홍길동"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected status StatusDefault, got %v", status)
		}
		if got := ctx.OutputString(); got != "그런 괴물이 없습니다.\n" {
			t.Errorf("expected monster not found message, got %q", got)
		}
	})

	t.Run("No enemies", func(t *testing.T) {
		w := setupWorld()
		handler := NewListEnmHandler(w)
		ctx := &Context{ActorID: "player:caretaker"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"늑대"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected status StatusDefault, got %v", status)
		}
		expected := "늑대의 적들:\n없음.\n"
		if ctx.OutputString() != expected {
			t.Errorf("expected output %q, got %q", expected, ctx.OutputString())
		}
	})

	t.Run("With enemies", func(t *testing.T) {
		w := setupWorld()
		w.enemies["creature:monster"] = []string{"홍길동", "임꺽정"}

		handler := NewListEnmHandler(w)
		ctx := &Context{ActorID: "player:caretaker"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"늑대"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected status StatusDefault, got %v", status)
		}
		expected := "늑대의 적들:\n홍길동.\n임꺽정.\n"
		if ctx.OutputString() != expected {
			t.Errorf("expected output %q, got %q", expected, ctx.OutputString())
		}
	})

	t.Run("Ordinal selects matching duplicate creature", func(t *testing.T) {
		w := setupWorld()
		w.creatures["creature:monster2"] = model.Creature{
			ID:          "creature:monster2",
			DisplayName: "늑대",
			RoomID:      "room:1",
		}
		room := w.rooms["room:1"]
		room.CreatureIDs = []model.CreatureID{"creature:monster", "creature:monster2"}
		w.rooms["room:1"] = room
		w.enemies["creature:monster2"] = []string{"홍길동"}

		handler := NewListEnmHandler(w)
		ctx := &Context{ActorID: "player:caretaker"}
		status, err := handler(ctx, ResolvedCommand{
			Args:   []string{"늑대"},
			Values: []int64{2},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected status StatusDefault, got %v", status)
		}
		expected := "늑대의 적들:\n홍길동.\n"
		if ctx.OutputString() != expected {
			t.Errorf("expected output %q, got %q", expected, ctx.OutputString())
		}
	})

	t.Run("Parsed target slot and ordinal select duplicate creature like C", func(t *testing.T) {
		w := setupWorld()
		w.creatures["creature:monster2"] = model.Creature{
			ID:          "creature:monster2",
			DisplayName: "늑대",
			RoomID:      "room:1",
		}
		room := w.rooms["room:1"]
		room.CreatureIDs = []model.CreatureID{"creature:monster", "creature:monster2"}
		w.rooms["room:1"] = room
		w.enemies["creature:monster2"] = []string{"홍길동"}

		handler := NewListEnmHandler(w)
		ctx := &Context{ActorID: "player:caretaker"}
		status, err := handler(ctx, ResolvedCommand{
			Input:  "2 늑대 *enemy",
			Parsed: commandparse.Command{Num: 2, Str: [commandparse.CommandMax]string{"*enemy", "늑대"}, Val: [commandparse.CommandMax]int64{1, 2}},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected status StatusDefault, got %v", status)
		}
		expected := "늑대의 적들:\n홍길동.\n"
		if ctx.OutputString() != expected {
			t.Errorf("expected output %q, got %q", expected, ctx.OutputString())
		}
	})
}
