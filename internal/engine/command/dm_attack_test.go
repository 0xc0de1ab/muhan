package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/session"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMAttackWorld struct {
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
	rooms     map[model.RoomID]model.Room
	enemies   [][2]model.CreatureID
}

func (m *mockDMAttackWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMAttackWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMAttackWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := m.rooms[id]
	return r, ok
}

func (m *mockDMAttackWorld) FindCreatureInRoom(roomID model.RoomID, name string) (model.Creature, bool) {
	for _, c := range m.creatures {
		if c.RoomID == roomID && strings.EqualFold(c.DisplayName, name) {
			return c, true
		}
	}
	return model.Creature{}, false
}

func (m *mockDMAttackWorld) FindCreatureByName(roomID model.RoomID, name string, count int) (model.Creature, bool) {
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

func (m *mockDMAttackWorld) FindPlayerByName(name string) (model.Player, bool) {
	for _, p := range m.players {
		if strings.EqualFold(p.DisplayName, name) {
			return p, true
		}
	}
	return model.Player{}, false
}

func (m *mockDMAttackWorld) AddEnemy(attacker, defender model.CreatureID) (bool, error) {
	m.enemies = append(m.enemies, [2]model.CreatureID{attacker, defender})
	return true, nil
}

func dmAttackActiveSessions(actorIDs ...string) map[string]any {
	sessions := make([]struct {
		ID      session.ID
		ActorID string
	}, 0, len(actorIDs))
	for _, actorID := range actorIDs {
		sessions = append(sessions, struct {
			ID      session.ID
			ActorID string
		}{
			ID:      session.ID("session:" + strings.TrimPrefix(actorID, "player:")),
			ActorID: actorID,
		})
	}
	return map[string]any{
		"game.activeSessions": func() []struct {
			ID      session.ID
			ActorID string
		} {
			return sessions
		},
	}
}

func TestDMAttack(t *testing.T) {
	// Common test setup helper
	setupWorld := func() *mockDMAttackWorld {
		w := &mockDMAttackWorld{
			players:   make(map[model.PlayerID]model.Player),
			creatures: make(map[model.CreatureID]model.Creature),
			rooms:     make(map[model.RoomID]model.Room),
		}

		// DM actor
		w.players["player:dm"] = model.Player{
			ID:          "player:dm",
			DisplayName: "대마왕",
			CreatureID:  "creature:dm",
			RoomID:      "room:1",
		}
		w.creatures["creature:dm"] = model.Creature{
			ID:          "creature:dm",
			DisplayName: "대마왕",
			RoomID:      "room:1",
			Stats:       map[string]int{"class": 13}, // DM class
		}

		// Attacker monster
		w.creatures["creature:monster"] = model.Creature{
			ID:          "creature:monster",
			DisplayName: "늑대",
			RoomID:      "room:1",
		}

		// Defender player
		w.players["player:victim"] = model.Player{
			ID:          "player:victim",
			DisplayName: "홍길동",
			CreatureID:  "creature:victim",
			RoomID:      "room:1",
		}
		w.creatures["creature:victim"] = model.Creature{
			ID:          "creature:victim",
			Kind:        model.CreatureKindPlayer,
			PlayerID:    "player:victim",
			DisplayName: "홍길동",
			RoomID:      "room:1",
		}

		// Room
		w.rooms["room:1"] = model.Room{
			ID:          "room:1",
			CreatureIDs: []model.CreatureID{"creature:monster"},
			PlayerIDs:   []model.PlayerID{"player:dm", "player:victim"},
		}

		return w
	}

	t.Run("Insufficient privileges", func(t *testing.T) {
		w := setupWorld()
		// Demote DM to caretaker (class 10)
		dmCrt := w.creatures["creature:dm"]
		dmCrt.Stats["class"] = 10
		w.creatures["creature:dm"] = dmCrt

		handler := NewDMAttackHandler(w)
		ctx := &Context{ActorID: "player:dm"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"늑대", "홍길동"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("expected status StatusPrompt, got %v", status)
		}
	})

	t.Run("Insufficient arguments", func(t *testing.T) {
		w := setupWorld()
		handler := NewDMAttackHandler(w)
		ctx := &Context{ActorID: "player:dm"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"늑대"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		if !strings.Contains(ctx.OutputString(), "사용법:") {
			t.Errorf("expected usage output, got %q", ctx.OutputString())
		}
	})

	t.Run("Attacker monster not found", func(t *testing.T) {
		w := setupWorld()
		handler := NewDMAttackHandler(w)
		ctx := &Context{ActorID: "player:dm"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"호랑이", "홍길동"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		if !strings.Contains(ctx.OutputString(), "그런 괴물이 없습니다.") {
			t.Errorf("expected monster not found, got %q", ctx.OutputString())
		}
	})

	t.Run("Player creature cannot be attacker monster", func(t *testing.T) {
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

		handler := NewDMAttackHandler(w)
		ctx := &Context{ActorID: "player:dm"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"홍길동", "늑대"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		if got := ctx.OutputString(); !strings.Contains(got, "그런 괴물이 없습니다.") {
			t.Errorf("expected monster not found, got %q", got)
		}
		if len(w.enemies) != 0 {
			t.Errorf("expected no enemies to be added, got %v", w.enemies)
		}
	})

	t.Run("Attacker is fixed (MPERMT)", func(t *testing.T) {
		w := setupWorld()
		mon := w.creatures["creature:monster"]
		mon.Metadata.Tags = []string{"mpermt"}
		w.creatures["creature:monster"] = mon

		handler := NewDMAttackHandler(w)
		ctx := &Context{ActorID: "player:dm"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"늑대", "홍길동"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		if !strings.Contains(ctx.OutputString(), "고정된 괴물입니다.") {
			t.Errorf("expected MPERMT block output, got %q", ctx.OutputString())
		}
	})

	t.Run("Defender player not found", func(t *testing.T) {
		w := setupWorld()
		handler := NewDMAttackHandler(w)
		ctx := &Context{ActorID: "player:dm"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"늑대", "임꺽정"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		if !strings.Contains(ctx.OutputString(), "그런 사람이 없습니다.") {
			t.Errorf("expected player not found, got %q", ctx.OutputString())
		}
	})

	t.Run("Defender is fixed (MPERMT)", func(t *testing.T) {
		w := setupWorld()
		victim := w.creatures["creature:victim"]
		victim.Metadata.Tags = []string{"MPERMT"}
		w.creatures["creature:victim"] = victim

		handler := NewDMAttackHandler(w)
		ctx := &Context{
			ActorID: "player:dm",
			Values:  dmAttackActiveSessions("player:victim"),
		}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"늑대", "홍길동"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		if !strings.Contains(ctx.OutputString(), "고정된 괴물입니다.") {
			t.Errorf("expected MPERMT block output, got %q", ctx.OutputString())
		}
	})

	t.Run("Saved defender without active session is not found", func(t *testing.T) {
		w := setupWorld()
		handler := NewDMAttackHandler(w)
		ctx := &Context{
			ActorID: "player:dm",
			Values:  dmAttackActiveSessions("player:dm"),
		}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"늑대", "홍길동"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		if !strings.Contains(ctx.OutputString(), "그런 사람이 없습니다.") {
			t.Errorf("expected player not found, got %q", ctx.OutputString())
		}
		if len(w.enemies) != 0 {
			t.Errorf("expected no enemies to be added, got %v", w.enemies)
		}
	})

	t.Run("Room player creature defender still requires active find_who match", func(t *testing.T) {
		w := setupWorld()
		room := w.rooms["room:1"]
		room.CreatureIDs = []model.CreatureID{"creature:monster", "creature:victim"}
		w.rooms["room:1"] = room

		handler := NewDMAttackHandler(w)
		ctx := &Context{
			ActorID: "player:dm",
			Values:  dmAttackActiveSessions("player:dm"),
		}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"늑대", "홍길동"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		if got := ctx.OutputString(); !strings.Contains(got, "그런 사람이 없습니다.") {
			t.Errorf("expected player not found, got %q", got)
		}
		if len(w.enemies) != 0 {
			t.Errorf("expected no enemies to be added, got %v", w.enemies)
		}
	})

	t.Run("Successful attack (player target)", func(t *testing.T) {
		w := setupWorld()
		handler := NewDMAttackHandler(w)

		var broadcastedText string
		var broadcastedRoomID model.RoomID
		var broadcastExclude string
		var sentDirectText string

		ctx := &Context{
			ActorID:   "player:dm",
			SessionID: "session:dm",
			Values: map[string]any{
				ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
					broadcastedRoomID = roomID
					broadcastExclude = excludeSessionID
					broadcastedText = text
					return nil
				}),
				"game.activeSessions": func() []struct {
					ID      session.ID
					ActorID string
				} {
					return []struct {
						ID      session.ID
						ActorID string
					}{
						{ID: "session:dm", ActorID: "player:dm"},
						{ID: "session:victim", ActorID: "player:victim"},
					}
				},
				"game.sendToSession": func(id session.ID, cmd session.Command) error {
					if id == "session:victim" {
						sentDirectText = cmd.Write
					}
					return nil
				},
			},
		}

		status, err := handler(ctx, ResolvedCommand{Args: []string{"늑대", "홍길동"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}

		// Check DM output
		if !strings.Contains(ctx.OutputString(), "홍길동가 늑대를 공격합니다.") {
			t.Errorf("unexpected DM message: %q", ctx.OutputString())
		}

		// Check room broadcast
		if broadcastedText != "늑대이 홍길동을 공격합니다." {
			t.Errorf("unexpected room broadcast: %q", broadcastedText)
		}
		if broadcastedRoomID != "room:1" {
			t.Errorf("broadcast room = %q, want room:1", broadcastedRoomID)
		}
		if broadcastExclude != "session:victim" {
			t.Errorf("broadcast exclude = %q, want session:victim", broadcastExclude)
		}

		// Check direct message to player
		if !strings.Contains(sentDirectText, "늑대이 당신을 공격합니다!") {
			t.Errorf("unexpected direct message: %q", sentDirectText)
		}

		// Check AddEnemy call
		if len(w.enemies) != 1 || w.enemies[0][0] != "creature:monster" || w.enemies[0][1] != "creature:victim" {
			t.Errorf("expected world.AddEnemy(monster, victim), got %v", w.enemies)
		}
	})

	t.Run("Remote online player defender broadcasts in defender room like legacy", func(t *testing.T) {
		w := setupWorld()
		victimPlayer := w.players["player:victim"]
		victimPlayer.RoomID = "room:2"
		w.players["player:victim"] = victimPlayer
		victimCreature := w.creatures["creature:victim"]
		victimCreature.RoomID = "room:2"
		w.creatures["creature:victim"] = victimCreature
		w.rooms["room:2"] = model.Room{
			ID:        "room:2",
			PlayerIDs: []model.PlayerID{"player:victim"},
		}

		handler := NewDMAttackHandler(w)

		var broadcastedRoomID model.RoomID
		var broadcastExclude string
		var sentDirectText string

		ctx := &Context{
			ActorID:   "player:dm",
			SessionID: "session:dm",
			Values: map[string]any{
				ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
					broadcastedRoomID = roomID
					broadcastExclude = excludeSessionID
					return nil
				}),
				"game.activeSessions": func() []struct {
					ID      session.ID
					ActorID string
				} {
					return []struct {
						ID      session.ID
						ActorID string
					}{
						{ID: "session:dm", ActorID: "player:dm"},
						{ID: "session:victim", ActorID: "player:victim"},
					}
				},
				"game.sendToSession": func(id session.ID, cmd session.Command) error {
					if id == "session:victim" {
						sentDirectText = cmd.Write
					}
					return nil
				},
			},
		}

		status, err := handler(ctx, ResolvedCommand{Args: []string{"늑대", "홍길동"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		if broadcastedRoomID != "room:2" {
			t.Errorf("broadcast room = %q, want defender room:2", broadcastedRoomID)
		}
		if broadcastExclude != "session:victim" {
			t.Errorf("broadcast exclude = %q, want session:victim", broadcastExclude)
		}
		if !strings.Contains(sentDirectText, "늑대이 당신을 공격합니다!") {
			t.Errorf("unexpected direct message: %q", sentDirectText)
		}
		if len(w.enemies) != 1 || w.enemies[0][0] != "creature:monster" || w.enemies[0][1] != "creature:victim" {
			t.Errorf("expected world.AddEnemy(monster, victim), got %v", w.enemies)
		}
	})

	t.Run("Attacker ordinal selects matching duplicate monster", func(t *testing.T) {
		w := setupWorld()
		first := w.creatures["creature:monster"]
		first.Metadata.Tags = []string{"MPERMT"}
		w.creatures["creature:monster"] = first
		w.creatures["creature:monster2"] = model.Creature{
			ID:          "creature:monster2",
			DisplayName: "늑대",
			RoomID:      "room:1",
		}
		room := w.rooms["room:1"]
		room.CreatureIDs = []model.CreatureID{"creature:monster", "creature:monster2"}
		w.rooms["room:1"] = room

		handler := NewDMAttackHandler(w)
		ctx := &Context{
			ActorID: "player:dm",
			Values:  dmAttackActiveSessions("player:victim"),
		}
		status, err := handler(ctx, ResolvedCommand{
			Args:   []string{"늑대", "홍길동"},
			Values: []int64{2, 1},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		if len(w.enemies) != 1 || w.enemies[0][0] != "creature:monster2" || w.enemies[0][1] != "creature:victim" {
			t.Fatalf("expected second wolf to attack victim, got enemies %v output %q", w.enemies, ctx.OutputString())
		}
	})

	t.Run("Parsed slots select attacker and defender like C when Args missing", func(t *testing.T) {
		w := setupWorld()
		first := w.creatures["creature:monster"]
		first.Metadata.Tags = []string{"MPERMT"}
		w.creatures["creature:monster"] = first
		w.creatures["creature:monster2"] = model.Creature{
			ID:          "creature:monster2",
			DisplayName: "늑대",
			RoomID:      "room:1",
		}
		room := w.rooms["room:1"]
		room.CreatureIDs = []model.CreatureID{"creature:monster", "creature:monster2"}
		w.rooms["room:1"] = room

		handler := NewDMAttackHandler(w)
		ctx := &Context{
			ActorID: "player:dm",
			Values:  dmAttackActiveSessions("player:victim"),
		}
		status, err := handler(ctx, ResolvedCommand{
			Input: "2 늑대 홍길동 *공격",
			Parsed: commandparse.Command{
				Str: [7]string{"*공격", "늑대", "홍길동"},
				Val: [7]int64{1, 2, 1},
				Num: 3,
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		if len(w.enemies) != 1 || w.enemies[0][0] != "creature:monster2" || w.enemies[0][1] != "creature:victim" {
			t.Fatalf("expected parsed second wolf to attack victim, got enemies %v output %q", w.enemies, ctx.OutputString())
		}
	})

	t.Run("Defender ordinal selects matching duplicate room creature", func(t *testing.T) {
		w := setupWorld()
		w.creatures["creature:target1"] = model.Creature{
			ID:          "creature:target1",
			DisplayName: "산적",
			RoomID:      "room:1",
			Metadata:    model.Metadata{Tags: []string{"MPERMT"}},
		}
		w.creatures["creature:target2"] = model.Creature{
			ID:          "creature:target2",
			DisplayName: "산적",
			RoomID:      "room:1",
		}
		room := w.rooms["room:1"]
		room.CreatureIDs = []model.CreatureID{"creature:monster", "creature:target1", "creature:target2"}
		w.rooms["room:1"] = room

		handler := NewDMAttackHandler(w)
		ctx := &Context{ActorID: "player:dm"}
		status, err := handler(ctx, ResolvedCommand{
			Args:   []string{"늑대", "산적"},
			Values: []int64{1, 2},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		if len(w.enemies) != 1 || w.enemies[0][0] != "creature:monster" || w.enemies[0][1] != "creature:target2" {
			t.Fatalf("expected wolf to attack second bandit, got enemies %v output %q", w.enemies, ctx.OutputString())
		}
	})
}
