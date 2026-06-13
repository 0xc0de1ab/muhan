package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMSendWorld struct {
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
}

func (m *mockDMSendWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMSendWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

type activeSessionMock struct {
	ID      string
	ActorID string
}

func TestDMSend_PermissionDenied(t *testing.T) {
	for _, tt := range []struct {
		name  string
		class int
	}{
		{name: "regular class", class: model.ClassInvincible},
		{name: "caretaker below SUB_DM", class: model.ClassCaretaker},
		{name: "bulsa below SUB_DM", class: model.ClassBulsa},
	} {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMSendWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": tt.class}, DisplayName: "Alice"},
				},
			}

			handler := NewDMSendHandler(world)
			ctx := &Context{
				SessionID: "session:alice",
				ActorID:   "player:alice",
			}

			resolved := ResolvedCommand{
				Input:  "dm_send Hello World",
				Parsed: commandparse.Command{Str: [7]string{"dm_send"}},
				Args:   []string{"Hello", "World"},
			}

			status, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status != StatusPrompt {
				t.Errorf("expected status %v, got %v", StatusPrompt, status)
			}

			out := ctx.OutputString()
			if out != "" {
				t.Errorf("expected no permission output, got %q", out)
			}
		})
	}
}

func TestDMSend_EmptyMessage(t *testing.T) {
	world := &mockDMSendWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}, DisplayName: "Alice"},
		},
	}

	handler := NewDMSendHandler(world)

	// Test case 1: no arguments at all
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
	}
	resolved := ResolvedCommand{
		Input:  "dm_send",
		Parsed: commandparse.Command{Str: [7]string{"dm_send"}},
		Args:   []string{},
	}

	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.OutputString() != "무엇을 공지하려구요?" {
		t.Errorf("expected '무엇을 공지하려구요?', got %q", ctx.OutputString())
	}

	// Test case 2: only spaces
	ctx = &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
	}
	resolved = ResolvedCommand{
		Input:  "dm_send     ",
		Parsed: commandparse.Command{Str: [7]string{"dm_send"}},
		Args:   []string{},
	}

	_, err = handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.OutputString() != "무엇을 공지하려구요?" {
		t.Errorf("expected '무엇을 공지하려구요?', got %q", ctx.OutputString())
	}
}

func TestDMSend_Broadcast(t *testing.T) {
	world := &mockDMSendWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice":   {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":     {ID: "player:bob", CreatureID: "creature:bob"},
			"player:charlie": {ID: "player:charlie", CreatureID: "creature:charlie"},
			"player:dave":    {ID: "player:dave", CreatureID: "creature:dave"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice":   {ID: "creature:alice", Stats: map[string]int{"class": 13}, DisplayName: "Alice"}, // DM
			"creature:bob":     {ID: "creature:bob", Stats: map[string]int{"class": model.ClassCaretaker}, DisplayName: "Bob"},
			"creature:charlie": {ID: "creature:charlie", Stats: map[string]int{"class": model.ClassInvincible}, DisplayName: "Charlie"},
			"creature:dave":    {ID: "creature:dave", Stats: map[string]int{"class": model.ClassSubDM}, DisplayName: "Dave"},
		},
	}

	sessions := []activeSessionMock{
		{ID: "session:alice", ActorID: "player:alice"},
		{ID: "session:bob", ActorID: "player:bob"},
		{ID: "session:charlie", ActorID: "player:charlie"},
		{ID: "session:dave", ActorID: "player:dave"},
	}

	receivedMsgs := make(map[string][]string)

	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []activeSessionMock {
				return sessions
			},
			"game.sendToSession": func(id string, cmd struct{ Write string }) error {
				receivedMsgs[id] = append(receivedMsgs[id], cmd.Write)
				return nil
			},
		},
	}

	resolved := ResolvedCommand{
		Input:  "dm_send   Hello   World!  ",
		Parsed: commandparse.Command{Str: [7]string{"dm_send"}},
		Args:   []string{"Hello", "World!"},
	}

	handler := NewDMSendHandler(world)
	status, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusDefault {
		t.Errorf("expected status %v, got %v", StatusDefault, status)
	}

	// Verify caster's output (accumulated in ctx.Output)
	casterOutput := ctx.OutputString()
	if !strings.Contains(casterOutput, "Ok.\n") {
		t.Errorf("caster output should contain 'Ok.\\n', got %q", casterOutput)
	}
	expectedBroadcastMsg := "\n>>> 공지(Alice): Hello   World!"
	if !strings.Contains(casterOutput, expectedBroadcastMsg) {
		t.Errorf("caster output should contain broadcast message %q, got %q", expectedBroadcastMsg, casterOutput)
	}

	// Verify Bob received the message (class >= 10)
	bobMsgs := receivedMsgs["session:bob"]
	if len(bobMsgs) != 1 || bobMsgs[0] != expectedBroadcastMsg {
		t.Errorf("Bob (class 10) should have received broadcast %q, got %v", expectedBroadcastMsg, bobMsgs)
	}

	// Verify Dave received the message (class >= 10)
	daveMsgs := receivedMsgs["session:dave"]
	if len(daveMsgs) != 1 || daveMsgs[0] != expectedBroadcastMsg {
		t.Errorf("Dave (class 12) should have received broadcast %q, got %v", expectedBroadcastMsg, daveMsgs)
	}

	// Verify Charlie did NOT receive the message (below caretaker broadcast_wiz threshold)
	charlieMsgs := receivedMsgs["session:charlie"]
	if len(charlieMsgs) != 0 {
		t.Errorf("Charlie (class 9) should not have received any broadcast, got %v", charlieMsgs)
	}
}

func TestDMSend_BroadcastVerbFinalRawInput(t *testing.T) {
	world := &mockDMSendWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, DisplayName: "Alice"},
			"creature:bob":   {ID: "creature:bob", Stats: map[string]int{"class": model.ClassCaretaker}, DisplayName: "Bob"},
		},
	}

	receivedMsgs := make(map[string][]string)
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []activeSessionMock {
				return []activeSessionMock{
					{ID: "session:alice", ActorID: "player:alice"},
					{ID: "session:bob", ActorID: "player:bob"},
				}
			},
			"game.sendToSession": func(id string, cmd struct{ Write string }) error {
				receivedMsgs[id] = append(receivedMsgs[id], cmd.Write)
				return nil
			},
		},
	}

	resolved := ResolvedCommand{
		Input:  "Hello   Team *send",
		Parsed: commandparse.Parse("Hello   Team *send"),
		Args:   []string{"Hello", "Team"},
	}

	status, err := NewDMSendHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want %v", status, StatusDefault)
	}

	expected := "\n>>> 공지(Alice): Hello   Team"
	if got := ctx.OutputString(); !strings.Contains(got, expected) {
		t.Fatalf("caster output should contain %q, got %q", expected, got)
	}
	if got := receivedMsgs["session:bob"]; len(got) != 1 || got[0] != expected {
		t.Fatalf("bob messages = %v, want [%q]", got, expected)
	}
}

func TestDMSendVerbFinalRawInputPreservesLegacyCutCommandTrailingSpaces(t *testing.T) {
	world := &mockDMSendWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, DisplayName: "Alice"},
			"creature:bob":   {ID: "creature:bob", Stats: map[string]int{"class": model.ClassCaretaker}, DisplayName: "Bob"},
		},
	}

	receivedMsgs := make(map[string][]string)
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []activeSessionMock {
				return []activeSessionMock{
					{ID: "session:alice", ActorID: "player:alice"},
					{ID: "session:bob", ActorID: "player:bob"},
				}
			},
			"game.sendToSession": func(id string, cmd struct{ Write string }) error {
				receivedMsgs[id] = append(receivedMsgs[id], cmd.Write)
				return nil
			},
		},
	}

	input := "Hello   Team   *send"
	resolved := ResolvedCommand{
		Input:  input,
		Parsed: commandparse.Parse(input),
		Args:   []string{"Hello", "Team"},
	}

	status, err := NewDMSendHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want %v", status, StatusDefault)
	}

	expected := "\n>>> 공지(Alice): Hello   Team  "
	if got := ctx.OutputString(); !strings.Contains(got, expected) {
		t.Fatalf("caster output should contain %q, got %q", expected, got)
	}
	if got := receivedMsgs["session:bob"]; len(got) != 1 || got[0] != expected {
		t.Fatalf("bob messages = %v, want [%q]", got, expected)
	}
}
