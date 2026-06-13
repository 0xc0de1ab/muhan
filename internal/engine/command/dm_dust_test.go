package command

import (
	"errors"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMDustWorld struct {
	players    map[model.PlayerID]model.Player
	creatures  map[model.CreatureID]model.Creature
	dustCalled map[model.PlayerID]bool
	dustErr    error
}

func (m *mockDMDustWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMDustWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMDustWorld) FindPlayerByName(name string) (model.Player, bool) {
	for _, p := range m.players {
		if strings.EqualFold(string(p.ID), name) ||
			strings.EqualFold(strings.TrimPrefix(string(p.ID), "player:"), name) ||
			strings.EqualFold(p.DisplayName, name) {
			return p, true
		}
	}
	return model.Player{}, false
}

func (m *mockDMDustWorld) DustPlayer(id model.PlayerID) error {
	m.dustCalled[id] = true
	return m.dustErr
}

type testSession struct {
	ID      string
	ActorID string
}

func TestDMDust_PermissionDenied(t *testing.T) {
	world := &mockDMDustWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 9}, DisplayName: "Alice"},
		},
	}

	handler := NewDMDustHandler(world)
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
	}

	resolved := ResolvedCommand{
		Input:  "*벼락치기 bob",
		Parsed: commandparse.Command{Str: [7]string{"*벼락치기"}, Num: 2},
		Args:   []string{"bob"},
	}

	status, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusPrompt {
		t.Errorf("expected status %v, got %v", StatusPrompt, status)
	}

	if got := ctx.OutputString(); got != "" {
		t.Errorf("expected no permission output, got %q", got)
	}
}

func TestDMDust_MissingArguments(t *testing.T) {
	world := &mockDMDustWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}, DisplayName: "Alice"},
		},
	}

	handler := NewDMDustHandler(world)
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
	}

	resolved := ResolvedCommand{
		Input:  "*벼락치기",
		Parsed: commandparse.Command{Str: [7]string{"*벼락치기"}, Num: 1},
		Args:   []string{},
	}

	status, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusPrompt {
		t.Errorf("status = %v, want StatusPrompt", status)
	}

	expected := "\n누구에게 번개를 내릴까요?\n"
	if ctx.OutputString() != expected {
		t.Errorf("expected %q, got %q", expected, ctx.OutputString())
	}
}

func TestDMDust_TargetNotFound(t *testing.T) {
	world := &mockDMDustWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}, DisplayName: "Alice"},
		},
	}

	handler := NewDMDustHandler(world)
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
	}

	resolved := ResolvedCommand{
		Input:  "*벼락치기 bob",
		Parsed: commandparse.Command{Str: [7]string{"*벼락치기"}, Num: 2},
		Args:   []string{"bob"},
	}

	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Bob이 없습니다.\n"
	if ctx.OutputString() != expected {
		t.Errorf("expected %q, got %q", expected, ctx.OutputString())
	}
}

func TestDMDust_TargetIsCaster(t *testing.T) {
	world := &mockDMDustWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}, DisplayName: "Alice"},
		},
	}

	handler := NewDMDustHandler(world)
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []testSession {
				return []testSession{
					{ID: "session:alice", ActorID: "player:alice"},
				}
			},
		},
	}

	resolved := ResolvedCommand{
		Input:  "*벼락치기 alice",
		Parsed: commandparse.Command{Str: [7]string{"*벼락치기"}, Num: 2},
		Args:   []string{"alice"},
	}

	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Alice이 없습니다.\n"
	if ctx.OutputString() != expected {
		t.Errorf("expected %q, got %q", expected, ctx.OutputString())
	}
}

func TestDMDust_SavedTargetWithoutActiveSessionIsNotFound(t *testing.T) {
	world := &mockDMDustWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}, DisplayName: "Alice"},
			"creature:bob":   {ID: "creature:bob", Stats: map[string]int{"class": model.ClassSubDM}, DisplayName: "Bob"},
		},
		dustCalled: make(map[model.PlayerID]bool),
	}

	var sent bool
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []testSession {
				return []testSession{
					{ID: "session:alice", ActorID: "player:alice"},
				}
			},
			"game.sendToSession": func(id string, cmd struct {
				Write string
				Close bool
			}) error {
				sent = true
				return nil
			},
		},
	}

	resolved := ResolvedCommand{
		Input:  "*벼락치기 bob",
		Parsed: commandparse.Command{Str: [7]string{"*벼락치기"}, Num: 2},
		Args:   []string{"bob"},
	}

	status, err := NewDMDustHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusDefault {
		t.Errorf("status = %v, want StatusDefault", status)
	}
	expected := "Bob이 없습니다.\n"
	if ctx.OutputString() != expected {
		t.Errorf("expected %q, got %q", expected, ctx.OutputString())
	}
	if world.dustCalled["player:bob"] {
		t.Error("DustPlayer should not have been called on offline Bob")
	}
	if sent {
		t.Error("offline Bob should not receive a DM warning")
	}
}

func TestDMDust_TargetIsDM(t *testing.T) {
	world := &mockDMDustWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}, DisplayName: "Alice"},
			"creature:bob":   {ID: "creature:bob", Stats: map[string]int{"class": model.ClassSubDM}, DisplayName: "Bob"},
		},
		dustCalled: make(map[model.PlayerID]bool),
	}

	sessions := []testSession{
		{ID: "session:alice", ActorID: "player:alice"},
		{ID: "session:bob", ActorID: "player:bob"},
	}

	receivedMsgs := make(map[string][]string)

	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []testSession {
				return sessions
			},
			"game.sendToSession": func(id string, cmd struct {
				Write string
				Close bool
			}) error {
				receivedMsgs[id] = append(receivedMsgs[id], cmd.Write)
				return nil
			},
		},
	}

	resolved := ResolvedCommand{
		Input:  "*벼락치기 bob",
		Parsed: commandparse.Command{Str: [7]string{"*벼락치기"}, Num: 2},
		Args:   []string{"bob"},
	}

	handler := NewDMDustHandler(world)
	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Caster should have empty output
	if ctx.OutputString() != "" {
		t.Errorf("expected empty caster output, got %q", ctx.OutputString())
	}

	// Bob should receive warning in RED
	bobMsgs := receivedMsgs["session:bob"]
	if len(bobMsgs) != 1 {
		t.Fatalf("expected Bob to receive 1 message, got %d", len(bobMsgs))
	}
	if !strings.Contains(bobMsgs[0], "Alice이 당신에게 번개를 내리려 합니다!") {
		t.Errorf("expected warning message to Bob, got %q", bobMsgs[0])
	}

	// DustPlayer should not be called
	if world.dustCalled["player:bob"] {
		t.Error("DustPlayer should not have been called on Bob")
	}
}

func TestDMDust_TargetSuccess(t *testing.T) {
	world := &mockDMDustWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob", RoomID: "room:100"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}, DisplayName: "Alice"},
			"creature:bob":   {ID: "creature:bob", Stats: map[string]int{"class": 1}, DisplayName: "길동", Properties: map[string]string{"PMALES": "1"}},
		},
		dustCalled: make(map[model.PlayerID]bool),
	}

	sessions := []testSession{
		{ID: "session:alice", ActorID: "player:alice"},
		{ID: "session:bob", ActorID: "player:bob"},
	}

	receivedMsgs := make(map[string][]string)
	receivedClose := make(map[string]bool)
	var roomBroadcastRoom model.RoomID
	var roomBroadcastExclude string
	var roomBroadcastMsg string
	var globalBroadcasts []string

	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []testSession {
				return sessions
			},
			"game.sendToSession": func(id string, cmd struct {
				Write string
				Close bool
			}) error {
				receivedMsgs[id] = append(receivedMsgs[id], cmd.Write)
				receivedClose[id] = cmd.Close
				return nil
			},
			ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
				roomBroadcastRoom = roomID
				roomBroadcastExclude = excludeSessionID
				roomBroadcastMsg = text
				return nil
			}),
			"game.broadcast": func(cmd struct{ Write string }) error {
				globalBroadcasts = append(globalBroadcasts, cmd.Write)
				return nil
			},
		},
	}

	resolved := ResolvedCommand{
		Input:  "*벼락치기 길동",
		Parsed: commandparse.Command{Str: [7]string{"*벼락치기"}, Num: 2},
		Args:   []string{"길동"},
	}

	handler := NewDMDustHandler(world)
	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify DustPlayer was called
	if !world.dustCalled["player:bob"] {
		t.Error("expected DustPlayer to be called on Bob")
	}

	// Verify Bob received MAGENTA message and got disconnected (Close = true)
	bobMsgs := receivedMsgs["session:bob"]
	if len(bobMsgs) != 1 {
		t.Fatalf("expected Bob to receive 1 message, got %d", len(bobMsgs))
	}
	if !strings.Contains(bobMsgs[0], "번개가 하늘에서 떨어집니다 신들의 분노가 진동합니다!") {
		t.Errorf("expected MAGENTA message to Bob, got %q", bobMsgs[0])
	}
	if !receivedClose["session:bob"] {
		t.Error("expected Bob's session to be closed")
	}

	// Verify room broadcast
	if roomBroadcastRoom != "room:100" {
		t.Errorf("expected room broadcast to room:100, got %s", roomBroadcastRoom)
	}
	if roomBroadcastExclude != "session:bob" {
		t.Errorf("expected room broadcast to exclude Bob's session, got %s", roomBroadcastExclude)
	}
	if !strings.Contains(roomBroadcastMsg, "번개가 하늘에서 길동에게 떨어집니다.") {
		t.Errorf("expected room broadcast message, got %q", roomBroadcastMsg)
	}

	// Verify global broadcasts
	if len(globalBroadcasts) != 2 {
		t.Fatalf("expected 2 global broadcasts, got %d", len(globalBroadcasts))
	}
	if !strings.Contains(globalBroadcasts[0], "길동이 잿더미가 되버렸습니다! 그에게 조의를 표하십시요.") {
		t.Errorf("expected global ashes message, got %q", globalBroadcasts[0])
	}
	if !strings.Contains(globalBroadcasts[1], "### 멀리서 신들의 분노에 천둥소리가 들려옵니다.") {
		t.Errorf("expected global thunder message, got %q", globalBroadcasts[1])
	}
}

func TestDMDustUsesParsedTargetSlotLikeCWhenArgsMissing(t *testing.T) {
	world := &mockDMDustWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob", RoomID: "room:100"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}, DisplayName: "Alice"},
			"creature:bob":   {ID: "creature:bob", Stats: map[string]int{"class": model.ClassFighter}, DisplayName: "Bob"},
		},
		dustCalled: make(map[model.PlayerID]bool),
	}
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []testSession {
				return []testSession{
					{ID: "session:alice", ActorID: "player:alice"},
					{ID: "session:bob", ActorID: "player:bob"},
				}
			},
			"game.sendToSession": func(id string, cmd struct {
				Write string
				Close bool
			}) error {
				return nil
			},
		},
	}

	resolved := ResolvedCommand{
		Input:  "bOB *벼락치기",
		Parsed: commandparse.Command{Str: [7]string{"*벼락치기", "bOB"}, Num: 2},
		Spec:   commandspec.CommandSpec{Name: "*벼락치기", Handler: "dm_dust", Privileged: true},
	}

	status, err := NewDMDustHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if !world.dustCalled["player:bob"] {
		t.Fatal("DustPlayer was not called for parsed-slot target")
	}
}

func TestDMDust_DustPlayerError(t *testing.T) {
	world := &mockDMDustWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob", RoomID: "room:100"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}, DisplayName: "Alice"},
			"creature:bob":   {ID: "creature:bob", Stats: map[string]int{"class": 1}, DisplayName: "Bob"},
		},
		dustCalled: make(map[model.PlayerID]bool),
		dustErr:    errors.New("db error"),
	}

	sessions := []testSession{
		{ID: "session:alice", ActorID: "player:alice"},
		{ID: "session:bob", ActorID: "player:bob"},
	}

	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []testSession {
				return sessions
			},
			"game.sendToSession": func(id string, cmd struct {
				Write string
				Close bool
			}) error {
				return nil
			},
		},
	}

	resolved := ResolvedCommand{
		Input:  "*벼락치기 bob",
		Parsed: commandparse.Command{Str: [7]string{"*벼락치기"}, Num: 2},
		Args:   []string{"bob"},
	}

	handler := NewDMDustHandler(world)
	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !world.dustCalled["player:bob"] {
		t.Error("expected DustPlayer to be called")
	}
}
