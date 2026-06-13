package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMSilenceWorld struct {
	players              map[model.PlayerID]model.Player
	creatures            map[model.CreatureID]model.Creature
	dailyBroadcastCounts map[model.CreatureID]struct{ cur, max int }
	setBroadcastCounts   map[model.CreatureID][]int
}

func (m *mockDMSilenceWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMSilenceWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMSilenceWorld) FindPlayerByName(name string) (model.Player, bool) {
	for _, p := range m.players {
		if strings.EqualFold(p.DisplayName, name) {
			return p, true
		}
	}
	return model.Player{}, false
}

func (m *mockDMSilenceWorld) GetDailyBroadcastCount(id model.CreatureID) (cur, max int) {
	if counts, ok := m.dailyBroadcastCounts[id]; ok {
		return counts.cur, counts.max
	}
	return 0, 10
}

func (m *mockDMSilenceWorld) SetDailyBroadcastCount(id model.CreatureID, val int) error {
	if m.setBroadcastCounts == nil {
		m.setBroadcastCounts = make(map[model.CreatureID][]int)
	}
	m.setBroadcastCounts[id] = append(m.setBroadcastCounts[id], val)
	if m.dailyBroadcastCounts == nil {
		m.dailyBroadcastCounts = make(map[model.CreatureID]struct{ cur, max int })
	}
	counts := m.dailyBroadcastCounts[id]
	counts.cur = val
	m.dailyBroadcastCounts[id] = counts
	return nil
}

func dmSilenceActiveSessions(actorIDs ...string) map[string]any {
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

func TestDMSilence(t *testing.T) {
	setupWorld := func(casterClass int) *mockDMSilenceWorld {
		world := &mockDMSilenceWorld{
			players: map[model.PlayerID]model.Player{
				"player:dm":    {ID: "player:dm", DisplayName: "DMPlayer", CreatureID: "creature:dm"},
				"player:alice": {ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice"},
				"player:bob":   {ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:dm": {
					ID:          "creature:dm",
					DisplayName: "DMPlayer",
					Stats:       map[string]int{"class": casterClass},
				},
				"creature:alice": {
					ID:          "creature:alice",
					DisplayName: "Alice",
					Stats:       map[string]int{"class": 1},
				},
				"creature:bob": {
					ID:          "creature:bob",
					DisplayName: "Bob",
					Stats:       map[string]int{"class": 1},
					Metadata:    model.Metadata{Tags: []string{"PDMINV"}},
				},
			},
			dailyBroadcastCounts: map[model.CreatureID]struct{ cur, max int }{
				"creature:alice": {cur: 5, max: 10},
			},
		}
		return world
	}

	t.Run("non-DM permission validation", func(t *testing.T) {
		world := setupWorld(12) // Sub-DM (12)
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Input:  "*silence alice",
			Parsed: commandparse.Command{Num: 2, Str: [7]string{"*silence", "alice"}},
			Args:   []string{"alice"},
		}

		handler := NewDMSilenceHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusPrompt {
			t.Errorf("expected StatusPrompt, got %v", status)
		}
		if got := ctx.OutputString(); got != "" {
			t.Errorf("output = %q, want no permission output", got)
		}
	})

	t.Run("argument count < 2", func(t *testing.T) {
		world := setupWorld(13) // DM (13)
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Input:  "*silence",
			Parsed: commandparse.Command{Num: 1, Str: [7]string{"*silence"}},
			Args:   []string{},
		}

		handler := NewDMSilenceHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		wantMsg := "문법: <사용자> [c/m] *벙어리\n"
		if got := ctx.OutputString(); got != wantMsg {
			t.Errorf("expected %q, got %q", wantMsg, got)
		}
	})

	t.Run("target not found", func(t *testing.T) {
		world := setupWorld(13)
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Input:  "*silence charlie",
			Parsed: commandparse.Command{Num: 2, Str: [7]string{"*silence", "charlie"}},
			Args:   []string{"charlie"},
		}

		handler := NewDMSilenceHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		wantMsg := "그런 사용자는 없습니다.\n"
		if got := ctx.OutputString(); got != wantMsg {
			t.Errorf("expected %q, got %q", wantMsg, got)
		}
	})

	t.Run("target is invisible (PDMINV)", func(t *testing.T) {
		world := setupWorld(13)
		ctx := &Context{
			ActorID: "player:dm",
			Values:  dmSilenceActiveSessions("player:bob"),
		}
		resolved := ResolvedCommand{
			Input:  "*silence bob",
			Parsed: commandparse.Command{Num: 2, Str: [7]string{"*silence", "bob"}},
			Args:   []string{"bob"},
		}

		handler := NewDMSilenceHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		wantMsg := "그런 사용자는 없습니다.\n"
		if got := ctx.OutputString(); got != wantMsg {
			t.Errorf("expected %q, got %q", wantMsg, got)
		}
	})

	t.Run("saved target without active session is not found", func(t *testing.T) {
		world := setupWorld(13)
		ctx := &Context{
			ActorID: "player:dm",
			Values:  dmSilenceActiveSessions("player:dm"),
		}
		resolved := ResolvedCommand{
			Input:  "*silence alice",
			Parsed: commandparse.Command{Num: 2, Str: [7]string{"*silence", "alice"}},
			Args:   []string{"alice"},
		}

		handler := NewDMSilenceHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}
		wantMsg := "그런 사용자는 없습니다.\n"
		if got := ctx.OutputString(); got != wantMsg {
			t.Errorf("expected %q, got %q", wantMsg, got)
		}
		if calls := world.setBroadcastCounts["creature:alice"]; len(calls) != 0 {
			t.Errorf("expected no SetDailyBroadcastCount calls, got %v", calls)
		}
	})

	t.Run("behavior branch: arg count < 3 (silence target)", func(t *testing.T) {
		world := setupWorld(13)
		ctx := &Context{
			ActorID: "player:dm",
			Values:  dmSilenceActiveSessions("player:alice"),
		}
		resolved := ResolvedCommand{
			Input:  "*silence alice",
			Parsed: commandparse.Command{Num: 2, Str: [7]string{"*silence", "alice"}},
			Args:   []string{"alice"},
		}

		handler := NewDMSilenceHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}

		// Verify broadcast count set to 0
		vals, ok := world.setBroadcastCounts["creature:alice"]
		if !ok || len(vals) == 0 || vals[0] != 0 {
			t.Errorf("expected SetDailyBroadcastCount(creature:alice, 0), got calls %v", vals)
		}

		wantMsg := "Alice은 조용해 졌습니다.\n"
		if got := ctx.OutputString(); got != wantMsg {
			t.Errorf("expected %q, got %q", wantMsg, got)
		}
	})

	t.Run("behavior branch: second arg starts with 'c' (query remaining)", func(t *testing.T) {
		world := setupWorld(13)
		ctx := &Context{
			ActorID: "player:dm",
			Values:  dmSilenceActiveSessions("player:alice"),
		}
		resolved := ResolvedCommand{
			Input:  "*silence alice c",
			Parsed: commandparse.Command{Num: 3, Str: [7]string{"*silence", "alice", "c"}},
			Args:   []string{"alice", "c"},
		}

		handler := NewDMSilenceHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}

		// Verify SetDailyBroadcastCount was NOT called
		if len(world.setBroadcastCounts["creature:alice"]) > 0 {
			t.Errorf("expected no SetDailyBroadcastCount call, got %v", world.setBroadcastCounts["creature:alice"])
		}

		wantMsg := "Alice has 5 of 10 broadcasts left.\n"
		if got := ctx.OutputString(); got != wantMsg {
			t.Errorf("expected %q, got %q", wantMsg, got)
		}
	})

	t.Run("behavior branch: numeric second arg sets broadcast count", func(t *testing.T) {
		world := setupWorld(13)
		ctx := &Context{
			ActorID: "player:dm",
			Values:  dmSilenceActiveSessions("player:alice"),
		}
		resolved := ResolvedCommand{
			Input:  "*silence alice 7",
			Parsed: commandparse.Command{Num: 3, Str: [7]string{"*silence", "alice", "7"}, Val: [7]int64{1, 1, 7}},
			Args:   []string{"alice", "7"},
		}

		handler := NewDMSilenceHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}

		// Verify broadcast count set to 7 (val[2])
		vals, ok := world.setBroadcastCounts["creature:alice"]
		if !ok || len(vals) == 0 || vals[0] != 7 {
			t.Errorf("expected SetDailyBroadcastCount(creature:alice, 7), got calls %v", vals)
		}

		wantMsg := "Alice is given 7 broadcasts.\n"
		if got := ctx.OutputString(); got != wantMsg {
			t.Errorf("expected %q, got %q", wantMsg, got)
		}
	})

	t.Run("parsed slots set numeric broadcast count when Args missing", func(t *testing.T) {
		world := setupWorld(13)
		ctx := &Context{
			ActorID: "player:dm",
			Values:  dmSilenceActiveSessions("player:alice"),
		}
		resolved := ResolvedCommand{
			Input:  "aLICE 7 *silence",
			Parsed: commandparse.Command{Num: 3, Str: [7]string{"*silence", "aLICE", "7"}, Val: [7]int64{1, 1, 7}},
		}

		handler := NewDMSilenceHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}

		vals, ok := world.setBroadcastCounts["creature:alice"]
		if !ok || len(vals) == 0 || vals[0] != 7 {
			t.Errorf("expected SetDailyBroadcastCount(creature:alice, 7), got calls %v", vals)
		}

		wantMsg := "Alice is given 7 broadcasts.\n"
		if got := ctx.OutputString(); got != wantMsg {
			t.Errorf("expected %q, got %q", wantMsg, got)
		}
	})

	t.Run("behavior branch: m literal uses C val slot zero", func(t *testing.T) {
		world := setupWorld(13)
		ctx := &Context{
			ActorID: "player:dm",
			Values:  dmSilenceActiveSessions("player:alice"),
		}
		resolved := ResolvedCommand{
			Input:  "*silence alice m 7",
			Parsed: commandparse.Command{Num: 4, Str: [7]string{"*silence", "alice", "m", "7"}, Val: [7]int64{1, 1, 0, 7}},
			Args:   []string{"alice", "m", "7"},
		}

		handler := NewDMSilenceHandler(world)
		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}

		vals, ok := world.setBroadcastCounts["creature:alice"]
		if !ok || len(vals) == 0 || vals[0] != 0 {
			t.Errorf("expected SetDailyBroadcastCount(creature:alice, 0), got calls %v", vals)
		}

		wantMsg := "Alice is given 0 broadcasts.\n"
		if got := ctx.OutputString(); got != wantMsg {
			t.Errorf("expected %q, got %q", wantMsg, got)
		}
	})
}
