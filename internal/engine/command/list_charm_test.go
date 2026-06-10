package command

import (
	"strings"
	"testing"

	"muhan/internal/world/model"
)

type mockListCharmWorld struct {
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
	charmed   map[model.PlayerID][]string
}

func (m *mockListCharmWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockListCharmWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockListCharmWorld) FindPlayerByName(name string) (model.Player, bool) {
	for _, p := range m.players {
		if strings.EqualFold(p.DisplayName, name) {
			return p, true
		}
	}
	return model.Player{}, false
}

func (m *mockListCharmWorld) PlayerCharmedCreatures(id model.PlayerID) ([]string, error) {
	return m.charmed[id], nil
}

func listCharmActiveSessions(actorIDs ...string) map[string]any {
	sessions := make([]testActiveSession, 0, len(actorIDs))
	for _, actorID := range actorIDs {
		sessions = append(sessions, testActiveSession{
			ID:      "session:" + strings.TrimPrefix(actorID, "player:"),
			ActorID: actorID,
		})
	}
	return map[string]any{
		"game.activeSessions": func() []testActiveSession {
			return sessions
		},
	}
}

func TestListCharm(t *testing.T) {
	setupWorld := func() *mockListCharmWorld {
		w := &mockListCharmWorld{
			players:   make(map[model.PlayerID]model.Player),
			creatures: make(map[model.CreatureID]model.Creature),
			charmed:   make(map[model.PlayerID][]string),
		}

		w.players["player:caretaker"] = model.Player{
			ID:          "player:caretaker",
			DisplayName: "관리자",
			CreatureID:  "creature:caretaker",
		}
		w.creatures["creature:caretaker"] = model.Creature{
			ID:          "creature:caretaker",
			DisplayName: "관리자",
			Stats:       map[string]int{"class": model.ClassSubDM}, // SUB_DM
		}

		w.players["player:target"] = model.Player{
			ID:          "player:target",
			DisplayName: "홍길동",
			CreatureID:  "creature:target",
		}
		w.creatures["creature:target"] = model.Creature{
			ID:          "creature:target",
			DisplayName: "홍길동",
		}

		return w
	}

	t.Run("Insufficient privileges", func(t *testing.T) {
		w := setupWorld()
		crt := w.creatures["creature:caretaker"]
		crt.Stats["class"] = 9
		w.creatures["creature:caretaker"] = crt

		handler := NewListCharmHandler(w)
		ctx := &Context{ActorID: "player:caretaker"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"홍길동"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("expected status StatusPrompt, got %v", status)
		}
	})

	t.Run("Insufficient arguments", func(t *testing.T) {
		w := setupWorld()
		handler := NewListCharmHandler(w)
		ctx := &Context{ActorID: "player:caretaker"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusPrompt {
			t.Errorf("expected status StatusPrompt, got %v", status)
		}
		if !strings.Contains(ctx.OutputString(), "누구의 최면자를 봅니까?") {
			t.Errorf("expected usage message, got %q", ctx.OutputString())
		}
	})

	t.Run("Player not found", func(t *testing.T) {
		w := setupWorld()
		handler := NewListCharmHandler(w)
		ctx := &Context{ActorID: "player:caretaker"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"임꺽정"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected status StatusDefault, got %v", status)
		}
		if !strings.Contains(ctx.OutputString(), "임꺽정은 없습니다.") {
			t.Errorf("expected player not found message, got %q", ctx.OutputString())
		}
	})

	t.Run("ASCII target name is lowercized before failure output", func(t *testing.T) {
		w := setupWorld()
		handler := NewListCharmHandler(w)
		ctx := &Context{ActorID: "player:caretaker"}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"bOB"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected status StatusDefault, got %v", status)
		}
		if got, want := ctx.OutputString(), "Bob은 없습니다.\n"; got != want {
			t.Errorf("expected player not found message %q, got %q", want, got)
		}
	})

	t.Run("No charmed creatures", func(t *testing.T) {
		w := setupWorld()
		handler := NewListCharmHandler(w)
		ctx := &Context{
			ActorID: "player:caretaker",
			Values:  listCharmActiveSessions("player:target"),
		}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"홍길동"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected status StatusDefault, got %v", status)
		}
		expected := "홍길동의 피최면자:\n없음.\n"
		if ctx.OutputString() != expected {
			t.Errorf("expected output %q, got %q", expected, ctx.OutputString())
		}
	})

	t.Run("saved player without active session is not found like C find_who", func(t *testing.T) {
		w := setupWorld()
		handler := NewListCharmHandler(w)
		ctx := &Context{
			ActorID: "player:caretaker",
			Values:  listCharmActiveSessions("player:caretaker"),
		}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"홍길동"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected status StatusDefault, got %v", status)
		}
		expected := "홍길동은 없습니다.\n"
		if ctx.OutputString() != expected {
			t.Errorf("expected output %q, got %q", expected, ctx.OutputString())
		}
	})

	t.Run("With charmed creatures", func(t *testing.T) {
		w := setupWorld()
		w.charmed["player:target"] = []string{"늑대", "호랑이"}

		handler := NewListCharmHandler(w)
		ctx := &Context{
			ActorID: "player:caretaker",
			Values:  listCharmActiveSessions("player:target"),
		}
		status, err := handler(ctx, ResolvedCommand{Args: []string{"홍길동"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected status StatusDefault, got %v", status)
		}
		expected := "홍길동의 피최면자:\n늑대.\n호랑이.\n"
		if ctx.OutputString() != expected {
			t.Errorf("expected output %q, got %q", expected, ctx.OutputString())
		}
	})
}
