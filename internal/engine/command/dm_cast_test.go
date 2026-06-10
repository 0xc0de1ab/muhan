package command

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type mockDMCastWorld struct {
	players      map[model.PlayerID]model.Player
	creatures    map[model.CreatureID]model.Creature
	roomPlayers  map[model.RoomID][]model.Player
	movedPlayers map[model.PlayerID]model.RoomID
	roomMessages []roomBroadcastMsg
	allMessages  []string
	broadcastErr error
	expirations  map[model.CreatureID]map[string]int64
}

type roomBroadcastMsg struct {
	excludePlayerID model.PlayerID
	roomID          model.RoomID
	msg             string
}

func (w *mockDMCastWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMCastWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMCastWorld) Players() []model.Player {
	var list []model.Player
	for _, p := range w.players {
		list = append(list, p)
	}
	return list
}

func (w *mockDMCastWorld) RoomPlayers(roomID model.RoomID) []model.Player {
	return w.roomPlayers[roomID]
}

func (w *mockDMCastWorld) UpdateCreatureTags(id model.CreatureID, add []string, remove []string) (model.Creature, error) {
	c, ok := w.creatures[id]
	if !ok {
		return model.Creature{}, fmt.Errorf("creature %s not found", id)
	}
	tagsMap := make(map[string]struct{})
	for _, t := range c.Metadata.Tags {
		tagsMap[t] = struct{}{}
	}
	for _, t := range add {
		tagsMap[t] = struct{}{}
	}
	for _, t := range remove {
		delete(tagsMap, t)
	}
	var newTags []string
	for t := range tagsMap {
		newTags = append(newTags, t)
	}
	c.Metadata.Tags = newTags
	w.creatures[id] = c
	return c, nil
}

func (w *mockDMCastWorld) UpdatePlayerTags(id model.PlayerID, add []string, remove []string) (model.Player, error) {
	p, ok := w.players[id]
	if !ok {
		return model.Player{}, fmt.Errorf("player %s not found", id)
	}
	tagsMap := make(map[string]struct{})
	for _, t := range p.Metadata.Tags {
		tagsMap[t] = struct{}{}
	}
	for _, t := range add {
		tagsMap[t] = struct{}{}
	}
	for _, t := range remove {
		delete(tagsMap, t)
	}
	var newTags []string
	for t := range tagsMap {
		newTags = append(newTags, t)
	}
	p.Metadata.Tags = newTags
	w.players[id] = p
	return p, nil
}

func (w *mockDMCastWorld) UpdateCreatureStat(id model.CreatureID, stat string, val int) error {
	c, ok := w.creatures[id]
	if !ok {
		return fmt.Errorf("creature %s not found", id)
	}
	if c.Stats == nil {
		c.Stats = make(map[string]int)
	}
	c.Stats[stat] = val
	w.creatures[id] = c
	return nil
}

func (w *mockDMCastWorld) MovePlayerToRoom(id model.PlayerID, roomID model.RoomID) error {
	if w.movedPlayers == nil {
		w.movedPlayers = make(map[model.PlayerID]model.RoomID)
	}
	w.movedPlayers[id] = roomID
	p := w.players[id]
	p.RoomID = roomID
	w.players[id] = p
	return nil
}

func (w *mockDMCastWorld) BroadcastAll(msg string) error {
	w.allMessages = append(w.allMessages, msg)
	return w.broadcastErr
}

func (w *mockDMCastWorld) SetEffectExpiration(id model.CreatureID, tag string, expires int64) {
	if w.expirations == nil {
		w.expirations = make(map[model.CreatureID]map[string]int64)
	}
	if w.expirations[id] == nil {
		w.expirations[id] = make(map[string]int64)
	}
	w.expirations[id][tag] = expires
}

func TestDMCast_PermissionDenied(t *testing.T) {
	world := &mockDMCastWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 9}},
		},
	}

	handler := NewDMCastHandler(world)
	ctx := &Context{
		ActorID: "player:alice",
	}
	resolved := ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "dm_cast"},
		Parsed: commandparse.Command{
			Num: 2,
			Str: [7]string{"dm_cast", "회복"},
		},
	}

	status, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusPrompt {
		t.Errorf("expected StatusPrompt, got %v", status)
	}

	gotOutput := ctx.OutputString()
	if gotOutput != "" {
		t.Errorf("expected no permission output, got %q", gotOutput)
	}
}

func TestDMCast_MissingArguments(t *testing.T) {
	world := &mockDMCastWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}},
		},
	}

	handler := NewDMCastHandler(world)
	ctx := &Context{
		ActorID: "player:alice",
	}
	resolved := ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "dm_cast"},
		Parsed: commandparse.Command{
			Num: 1,
			Str: [7]string{"dm_cast"},
		},
	}

	status, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusPrompt {
		t.Errorf("expected StatusPrompt, got %v", status)
	}

	gotOutput := ctx.OutputString()
	wantOutput := "무엇을 외웁니까?\n"
	if gotOutput != wantOutput {
		t.Errorf("expected %q, got %q", wantOutput, gotOutput)
	}
}

func TestDMCast_InvalidFlag(t *testing.T) {
	world := &mockDMCastWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}},
		},
	}

	handler := NewDMCastHandler(world)

	tests := []struct {
		name       string
		parsed     commandparse.Command
		wantStatus Status
		wantOutput string
	}{
		{
			name:       "invalid flag with spell argument",
			parsed:     commandparse.Command{Num: 3, Str: [7]string{"dm_cast", "-x", "회복"}},
			wantStatus: StatusPrompt,
			wantOutput: "Invalid cast flag.\n",
		},
		{
			name:       "prefix is not room flag",
			parsed:     commandparse.Command{Num: 3, Str: [7]string{"dm_cast", "-room", "회복"}},
			wantStatus: StatusPrompt,
			wantOutput: "Invalid cast flag.\n",
		},
		{
			name:       "single dash argument is treated as spell name",
			parsed:     commandparse.Command{Num: 2, Str: [7]string{"dm_cast", "-x"}},
			wantStatus: StatusDefault,
			wantOutput: "그런 주문은 없습니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{ActorID: "player:alice"}
			resolved := ResolvedCommand{
				Spec:   commandspec.CommandSpec{Name: "dm_cast"},
				Parsed: tt.parsed,
			}

			status, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status != tt.wantStatus {
				t.Errorf("status = %v, want %v", status, tt.wantStatus)
			}
			if got := ctx.OutputString(); got != tt.wantOutput {
				t.Errorf("output = %q, want %q", got, tt.wantOutput)
			}
		})
	}
}

func TestDMCast_SpellLookup(t *testing.T) {
	world := &mockDMCastWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}},
		},
	}

	handler := NewDMCastHandler(world)

	tests := []struct {
		name       string
		spellName  string
		wantOutput string
	}{
		{
			name:       "non-existent spell",
			spellName:  "이상한주문",
			wantOutput: "그런 주문은 없습니다.\n",
		},
		{
			name:       "ambiguous spell",
			spellName:  "지",
			wantOutput: "주문이름이 이상합니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{
				ActorID: "player:alice",
			}
			resolved := ResolvedCommand{
				Spec: commandspec.CommandSpec{Name: "dm_cast"},
				Parsed: commandparse.Command{
					Num: 2,
					Str: [7]string{"dm_cast", tt.spellName},
				},
			}
			status, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status != StatusDefault {
				t.Errorf("expected StatusDefault, got %v", status)
			}
			gotOutput := ctx.OutputString()
			if gotOutput != tt.wantOutput {
				t.Errorf("expected %q, got %q", tt.wantOutput, gotOutput)
			}
		})
	}
}

func TestDMCast_Recall(t *testing.T) {
	t.Run("rcast is true (room recall)", func(t *testing.T) {
		world := &mockDMCastWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:100"},
				"player:bob":   {ID: "player:bob", CreatureID: "creature:bob", RoomID: "room:100"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", DisplayName: "Alice", Stats: map[string]int{"class": model.ClassSubDM}},
				"creature:bob":   {ID: "creature:bob", DisplayName: "Bob", Stats: map[string]int{"class": 1}},
			},
			roomPlayers: map[model.RoomID][]model.Player{
				"room:100": {
					{ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:100"},
					{ID: "player:bob", CreatureID: "creature:bob", RoomID: "room:100"},
				},
			},
			movedPlayers: make(map[model.PlayerID]model.RoomID),
		}

		handler := NewDMCastHandler(world)
		ctx := &Context{
			ActorID:   "player:alice",
			SessionID: "session-alice",
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
				Str: [7]string{"dm_cast", "-r", "귀환"},
			},
		}

		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}

		gotOutput := ctx.OutputString()
		wantOutput := "당신은 귀환 주문을 방에 있는 사람에게 외웠습니다.\nAlice이 귀환를 당신에게 외웠습니다.\n"
		if gotOutput != wantOutput {
			t.Errorf("expected %q, got %q", wantOutput, gotOutput)
		}

		// Verify moves
		if dest, ok := world.movedPlayers["player:alice"]; !ok || dest != "room:1" {
			t.Errorf("expected alice moved to room:1, got %v", dest)
		}
		if dest, ok := world.movedPlayers["player:bob"]; !ok || dest != "room:1" {
			t.Errorf("expected bob moved to room:1, got %v", dest)
		}

		// Verify target messages sent
		bobMsgs := sentMessages["session-bob"]
		if len(bobMsgs) == 0 || !strings.Contains(bobMsgs[0], "Alice이 귀환를 당신에게 외웠습니다.") {
			t.Errorf("expected bob target message, got %v", bobMsgs)
		}

		// Verify broadcast message
		if len(roomBroadcasts) != 1 {
			t.Fatalf("expected 1 room broadcast, got %d", len(roomBroadcasts))
		}
		if !strings.Contains(roomBroadcasts[0], "Alice이 귀환 주문을 방에 있는 사람들에게 외웠습니다.") {
			t.Errorf("unexpected broadcast message: %q", roomBroadcasts[0])
		}
	})

	t.Run("rcast is false (global recall - forbidden)", func(t *testing.T) {
		world := &mockDMCastWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:100"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", DisplayName: "Alice", Stats: map[string]int{"class": model.ClassSubDM}},
			},
		}

		handler := NewDMCastHandler(world)
		ctx := &Context{
			ActorID: "player:alice",
		}
		resolved := ResolvedCommand{
			Spec: commandspec.CommandSpec{Name: "dm_cast"},
			Parsed: commandparse.Command{
				Num: 2,
				Str: [7]string{"dm_cast", "귀환"},
			},
		}

		status, err := handler(ctx, resolved)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status != StatusDefault {
			t.Errorf("expected StatusDefault, got %v", status)
		}

		gotOutput := ctx.OutputString()
		wantOutput := "그주문을 모두에게 외울수 없습니다.\n"
		if gotOutput != wantOutput {
			t.Errorf("expected %q, got %q", wantOutput, gotOutput)
		}
	})
}

func TestDMCast_RoomCastSpells(t *testing.T) {
	world := &mockDMCastWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:100"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob", RoomID: "room:100"},
			"player:clara": {ID: "player:clara", CreatureID: "creature:clara", RoomID: "room:100"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:          "creature:alice",
				DisplayName: "Alice",
				Stats:       map[string]int{"class": 12},
			},
			"creature:bob": {
				ID:          "creature:bob",
				DisplayName: "Bob",
				Stats: map[string]int{
					"hpCurrent": 50,
					"hpMax":     100,
				},
				Metadata: model.Metadata{Tags: []string{}},
			},
			"creature:clara": { // invisible/dminv: should be skipped
				ID:          "creature:clara",
				DisplayName: "Clara",
				Metadata:    model.Metadata{Tags: []string{"PDMINV"}},
			},
		},
		roomPlayers: map[model.RoomID][]model.Player{
			"room:100": {
				{ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:100"},
				{ID: "player:bob", CreatureID: "creature:bob", RoomID: "room:100"},
				{ID: "player:clara", CreatureID: "creature:clara", RoomID: "room:100"},
			},
		},
	}

	handler := NewDMCastHandler(world)
	ctx := &Context{
		ActorID:   "player:alice",
		SessionID: "session-alice",
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
				}{ID: "session-alice", ActorID: "player:alice"},
				struct {
					ID      string
					ActorID string
				}{ID: "session-bob", ActorID: "player:bob"},
				struct {
					ID      string
					ActorID string
				}{ID: "session-clara", ActorID: "player:clara"},
			}
		},
		"game.sendToSession": func(id string, cmd struct{ Write string }) error {
			sentMessages[id] = append(sentMessages[id], cmd.Write)
			return nil
		},
	}

	// 1. Cast heal (회복) in room
	resolved := ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "dm_cast"},
		Parsed: commandparse.Command{
			Num: 3,
			Str: [7]string{"dm_cast", "-r", "회복"},
		},
	}

	// Mock attackRoll to return fixed values
	previous := attackRoll
	attackRoll = func(min, max int) int {
		return min // return min value for predictability
	}
	defer func() {
		attackRoll = previous
	}()

	status, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusDefault {
		t.Errorf("expected StatusDefault, got %v", status)
	}

	// Bob should be healed.
	// magicPowerVigor = hpCur + rolled (mrand(1, 6) + 4 + 2)
	// mrand(1, 6) returns 1 (since attackRoll returns min).
	// So 50 + 1 + 4 + 2 = 57.
	bobCrt, _ := world.Creature("creature:bob")
	if hp := bobCrt.Stats["hpCurrent"]; hp != 57 {
		t.Errorf("expected bob hpCurrent = 57, got %d", hp)
	}

	// Clara should be skipped (has PDMINV)
	claraMsgs := sentMessages["session-clara"]
	if len(claraMsgs) > 0 {
		t.Errorf("expected Clara to be skipped, but got messages: %v", claraMsgs)
	}

	// Alice (caster) should get caster message
	gotOutput := ctx.OutputString()
	wantOutput := "당신은 회복 주문을 방에 있는 사람들에게 외웠습니다.\n"
	if !strings.Contains(gotOutput, wantOutput) {
		t.Errorf("expected output to contain %q, got %q", wantOutput, gotOutput)
	}

	// Room broadcast excluding caster
	if len(roomBroadcasts) != 1 {
		t.Fatalf("expected 1 room broadcast, got %d", len(roomBroadcasts))
	}
	if !strings.Contains(roomBroadcasts[0], "Alice이 회복 주문을 방에 있는 사람들에게 외웠습니다.\n") {
		t.Errorf("unexpected room broadcast: %q", roomBroadcasts[0])
	}
}

func TestDMCastCreatureBackedActorUsesLegacyCreaturePointer(t *testing.T) {
	world := &mockDMCastWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob", RoomID: "room:100"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:          "creature:alice",
				PlayerID:    "player:alice",
				RoomID:      "room:100",
				DisplayName: "Alice",
				Stats:       map[string]int{"class": model.ClassSubDM},
			},
			"creature:bob": {
				ID:          "creature:bob",
				DisplayName: "Bob",
				Metadata:    model.Metadata{Tags: []string{}},
			},
		},
		roomPlayers: map[model.RoomID][]model.Player{
			"room:100": {
				{ID: "player:bob", CreatureID: "creature:bob", RoomID: "room:100"},
			},
		},
	}

	handler := NewDMCastHandler(world)
	var broadcastRoom model.RoomID
	ctx := &Context{
		ActorID: "creature:alice",
		Values: map[string]any{
			ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
				broadcastRoom = roomID
				return nil
			}),
		},
	}
	resolved := ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "dm_cast"},
		Parsed: commandparse.Command{
			Num: 3,
			Str: [7]string{"dm_cast", "-r", "성현진"},
		},
	}

	status, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusDefault {
		t.Errorf("status = %v, want %v", status, StatusDefault)
	}
	if got, want := ctx.OutputString(), "당신은 성현진 주문을 방에 있는 사람들에게 외웠습니다.\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if broadcastRoom != "room:100" {
		t.Fatalf("broadcast room = %q, want room:100", broadcastRoom)
	}
	bobCrt, _ := world.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCrt.Metadata.Tags, "PBLESS") {
		t.Errorf("expected Bob creature to have PBLESS tag, got %v", bobCrt.Metadata.Tags)
	}
	bobPlayer, _ := world.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "PBLESS") {
		t.Errorf("expected Bob player to have PBLESS tag, got %v", bobPlayer.Metadata.Tags)
	}
}

func TestDMCastIgnoresRoomBroadcastAndSendErrorsLikeLegacy(t *testing.T) {
	world := &mockDMCastWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:100"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob", RoomID: "room:100"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:          "creature:alice",
				DisplayName: "Alice",
				Stats:       map[string]int{"class": 12},
			},
			"creature:bob": {
				ID:          "creature:bob",
				DisplayName: "Bob",
				Metadata:    model.Metadata{Tags: []string{}},
			},
		},
		roomPlayers: map[model.RoomID][]model.Player{
			"room:100": {
				{ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:100"},
				{ID: "player:bob", CreatureID: "creature:bob", RoomID: "room:100"},
			},
		},
	}

	handler := NewDMCastHandler(world)
	var roomBroadcasts []string
	var sentMessages []string
	ctx := &Context{
		ActorID:   "player:alice",
		SessionID: "session-alice",
		Values: map[string]any{
			ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
				roomBroadcasts = append(roomBroadcasts, text)
				return errors.New("closed room broadcast")
			}),
			"game.activeSessions": func() []any {
				return []any{
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
				sentMessages = append(sentMessages, cmd.Write)
				return errors.New("session closed")
			},
		},
	}
	resolved := ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "dm_cast"},
		Parsed: commandparse.Command{
			Num: 3,
			Str: [7]string{"dm_cast", "-r", "성현진"},
		},
	}

	status, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusDefault {
		t.Errorf("status = %v, want %v", status, StatusDefault)
	}
	if got, want := ctx.OutputString(), "Alice이 성현진를 당신에게 외웠습니다.\n당신은 성현진 주문을 방에 있는 사람들에게 외웠습니다.\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if len(roomBroadcasts) != 1 {
		t.Fatalf("expected one attempted room broadcast, got %d", len(roomBroadcasts))
	}
	if len(sentMessages) != 1 || !strings.Contains(sentMessages[0], "Alice이 성현진를 당신에게 외웠습니다.\n") {
		t.Fatalf("expected Bob direct send attempt, got %v", sentMessages)
	}
	bobCrt, _ := world.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCrt.Metadata.Tags, "PBLESS") {
		t.Errorf("expected Bob creature to have PBLESS tag, got %v", bobCrt.Metadata.Tags)
	}
	bobPlayer, _ := world.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "PBLESS") {
		t.Errorf("expected Bob player to have PBLESS tag, got %v", bobPlayer.Metadata.Tags)
	}
}

func TestDMCast_GlobalCastSpells(t *testing.T) {
	world := &mockDMCastWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob"},
			"player:clara": {ID: "player:clara", CreatureID: "creature:clara"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:          "creature:alice",
				DisplayName: "Alice",
				Stats:       map[string]int{"class": 12},
			},
			"creature:bob": {
				ID:          "creature:bob",
				DisplayName: "Bob",
				Metadata:    model.Metadata{Tags: []string{}},
			},
			"creature:clara": {
				ID:          "creature:clara",
				DisplayName: "Clara",
				Metadata:    model.Metadata{Tags: []string{"PDMINV"}},
			},
		},
	}

	handler := NewDMCastHandler(world)
	ctx := &Context{
		ActorID:   "player:alice",
		SessionID: "session-alice",
	}

	var sentMessages = make(map[string][]string)
	ctx.Values = map[string]any{
		"game.activeSessions": func() []any {
			return []any{
				struct {
					ID      string
					ActorID string
				}{ID: "session-alice", ActorID: "player:alice"},
				struct {
					ID      string
					ActorID string
				}{ID: "session-bob", ActorID: "player:bob"},
				struct {
					ID      string
					ActorID string
				}{ID: "session-clara", ActorID: "player:clara"},
			}
		},
		"game.sendToSession": func(id string, cmd struct{ Write string }) error {
			sentMessages[id] = append(sentMessages[id], cmd.Write)
			return nil
		},
	}

	// Cast bless (성현진) globally
	before := time.Now().Unix()
	resolved := ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "dm_cast"},
		Parsed: commandparse.Command{
			Num: 2,
			Str: [7]string{"dm_cast", "성현진"},
		},
	}

	status, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusDefault {
		t.Errorf("expected StatusDefault, got %v", status)
	}

	// Bob should have PBLESS tag added
	bobCrt, _ := world.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCrt.Metadata.Tags, "PBLESS") {
		t.Errorf("expected Bob to have PBLESS tag, got %v", bobCrt.Metadata.Tags)
	}
	bobPlayer, _ := world.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "PBLESS") {
		t.Errorf("expected Bob player to have PBLESS tag, got %v", bobPlayer.Metadata.Tags)
	}
	if got := world.expirations["creature:bob"]["PBLESS"]; got < before+3600 || got > time.Now().Unix()+3600 {
		t.Errorf("expected Bob PBLESS expiration about one hour from now, got %d", got)
	}

	// Clara should be skipped
	claraCrt, _ := world.Creature("creature:clara")
	if hasAnyNormalizedFlag(claraCrt.Metadata.Tags, "PBLESS") {
		t.Errorf("expected Clara to NOT have PBLESS tag, got %v", claraCrt.Metadata.Tags)
	}

	// Bob target message
	bobMsgs := sentMessages["session-bob"]
	if len(bobMsgs) == 0 || !strings.Contains(bobMsgs[0], "Alice이 성현진 주문을 당신에게 외웠습니다.\n") {
		t.Errorf("expected Bob target message, got %v", bobMsgs)
	}

	// Alice caster message
	gotOutput := ctx.OutputString()
	wantOutput := "당신은 성현진 주문을 모두에게 외웠습니다.\n"
	if !strings.Contains(gotOutput, wantOutput) {
		t.Errorf("expected output to contain %q, got %q", wantOutput, gotOutput)
	}

	// Global broadcast
	if len(world.allMessages) != 1 {
		t.Fatalf("expected 1 global broadcast, got %d", len(world.allMessages))
	}
	if !strings.Contains(world.allMessages[0], "Alice이 성현진 주문을 모두에게 외웠습니다.\n") {
		t.Errorf("unexpected global broadcast: %q", world.allMessages[0])
	}
}

func TestDMCastIgnoresGlobalBroadcastAndSendErrorsLikeLegacy(t *testing.T) {
	world := &mockDMCastWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:          "creature:alice",
				DisplayName: "Alice",
				Stats:       map[string]int{"class": 12},
			},
			"creature:bob": {
				ID:          "creature:bob",
				DisplayName: "Bob",
				Metadata:    model.Metadata{Tags: []string{}},
			},
		},
		broadcastErr: errors.New("broken global broadcast"),
	}

	handler := NewDMCastHandler(world)
	var sentMessages []string
	ctx := &Context{
		ActorID:   "player:alice",
		SessionID: "session-alice",
		Values: map[string]any{
			"game.activeSessions": func() []any {
				return []any{
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
				sentMessages = append(sentMessages, cmd.Write)
				return errors.New("session closed")
			},
		},
	}
	resolved := ResolvedCommand{
		Spec: commandspec.CommandSpec{Name: "dm_cast"},
		Parsed: commandparse.Command{
			Num: 2,
			Str: [7]string{"dm_cast", "성현진"},
		},
	}

	status, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusDefault {
		t.Errorf("status = %v, want %v", status, StatusDefault)
	}
	if got, want := ctx.OutputString(), "당신은 성현진 주문을 모두에게 외웠습니다.\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if len(sentMessages) != 1 || !strings.Contains(sentMessages[0], "Alice이 성현진 주문을 당신에게 외웠습니다.\n") {
		t.Fatalf("expected Bob direct send attempt, got %v", sentMessages)
	}
	if len(world.allMessages) != 1 || !strings.Contains(world.allMessages[0], "Alice이 성현진 주문을 모두에게 외웠습니다.\n") {
		t.Fatalf("expected one attempted global broadcast, got %v", world.allMessages)
	}
	bobCrt, _ := world.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCrt.Metadata.Tags, "PBLESS") {
		t.Errorf("expected Bob creature to have PBLESS tag, got %v", bobCrt.Metadata.Tags)
	}
	bobPlayer, _ := world.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "PBLESS") {
		t.Errorf("expected Bob player to have PBLESS tag, got %v", bobPlayer.Metadata.Tags)
	}
}

func TestDMCast_UnsupportedSpell(t *testing.T) {
	world := &mockDMCastWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": model.ClassSubDM}},
		},
	}

	handler := NewDMCastHandler(world)

	// magicPowerShockbolt is NOT in the supported list of DM spells
	tests := []struct {
		name       string
		args       []string
		wantOutput string
	}{
		{
			name:       "unsupported room cast",
			args:       []string{"dm_cast", "-r", "삭풍"},
			wantOutput: "그런 주문은 않됩니다.\n",
		},
		{
			name:       "unsupported global cast",
			args:       []string{"dm_cast", "삭풍"},
			wantOutput: "그주문을 모두에게 외울수 없습니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &Context{
				ActorID: "player:alice",
			}
			resolved := ResolvedCommand{
				Spec: commandspec.CommandSpec{Name: "dm_cast"},
				Parsed: commandparse.Command{
					Num: len(tt.args),
					Str: [7]string{tt.args[0], tt.args[1], getArgAt(tt.args, 2)},
				},
			}
			status, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status != StatusDefault {
				t.Errorf("expected StatusDefault, got %v", status)
			}
			gotOutput := ctx.OutputString()
			if gotOutput != tt.wantOutput {
				t.Errorf("expected %q, got %q", tt.wantOutput, gotOutput)
			}
		})
	}
}

func getArgAt(args []string, idx int) string {
	if idx < len(args) {
		return args[idx]
	}
	return ""
}
