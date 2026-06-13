package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMUsersWorld struct {
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
	rooms     map[model.RoomID]model.Room
}

func (m *mockDMUsersWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMUsersWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMUsersWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := m.rooms[id]
	return r, ok
}

type testActiveSession struct {
	ID      string
	ActorID string
}

func TestDMUsers_Unauthorized(t *testing.T) {
	world := &mockDMUsersWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 1}}, // Ordinary class
		},
	}
	ctx := &Context{ActorID: "player:alice"}
	handler := NewDMUsersHandler(world)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusPrompt {
		t.Errorf("expected StatusPrompt, got %v", status)
	}
	if ctx.OutputString() != "" {
		t.Errorf("expected no output, got %q", ctx.OutputString())
	}
}

func TestDMUsers_DefaultMode(t *testing.T) {
	world := &mockDMUsersWorld{
		players: map[model.PlayerID]model.Player{
			"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			"player:alice":  {ID: "player:alice", CreatureID: "creature:alice", AccountName: "alice_account"},
			"player:bob":    {ID: "player:bob", CreatureID: "creature:bob", AccountName: "bob_account"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:caster": {ID: "creature:caster", Stats: map[string]int{"class": model.ClassSubDM}},
			"creature:alice": {
				ID:          "creature:alice",
				DisplayName: "엘리스",
				RoomID:      "room:100",
				Level:       15,
				Stats:       map[string]int{"class": 4, "ltime": 999999800}, // 200 seconds idle
				Properties:  map[string]string{"address": "192.168.1.5", "lastCommand": "look"},
			},
			"creature:bob": {
				ID:          "creature:bob",
				DisplayName: "밥",
				RoomID:      "room:200",
				Level:       20,
				Stats:       map[string]int{"class": 5, "ltime": 999999940}, // 60 seconds idle
				Properties:  map[string]string{"address": "192.168.1.10", "lastCommand": "say"},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {ID: "room:100", DisplayName: "광장"},
			"room:200": {ID: "room:200", DisplayName: "주막"},
		},
	}

	ctx := &Context{
		ActorID: "player:caster",
		Values: map[string]any{
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{
					{ID: "session-1", ActorID: "player:alice"},
					{ID: "session-2", ActorID: "player:bob"},
				}
			},
			"test.now": int64(1000000000),
		},
	}

	handler := NewDMUsersHandler(world)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusDefault {
		t.Errorf("expected StatusDefault, got %v", status)
	}

	output := ctx.OutputString()
	// Should have the correct headers and details
	if !strings.Contains(output, "Lev  Clas  Player    Room #: Name         Address         Last command    Idle") {
		t.Errorf("missing header in output:\n%s", output)
	}
	if !strings.Contains(output, "-------------------------------------------------------------------------------") {
		t.Errorf("missing dashes in output:\n%s", output)
	}
	// Alice row
	if !strings.Contains(output, "[15] 검사  엘리스       100: 광장         192.168.1.5     look            03:20") {
		t.Errorf("missing alice row or mismatch in output:\n%s", output)
	}
	// Bob row
	if !strings.Contains(output, "[20] 도술  밥           200: 주막         192.168.1.10    say             01:00") {
		t.Errorf("missing bob row or mismatch in output:\n%s", output)
	}
}

func TestDMUsers_UserIDMode(t *testing.T) {
	world := &mockDMUsersWorld{
		players: map[model.PlayerID]model.Player{
			"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			"player:alice":  {ID: "player:alice", CreatureID: "creature:alice", AccountName: "alice_account"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:caster": {ID: "creature:caster", Stats: map[string]int{"class": 12}}, // Sub-DM (12)
			"creature:alice": {
				ID:          "creature:alice",
				DisplayName: "엘리스",
				RoomID:      "room:100",
				Level:       15,
				Stats:       map[string]int{"class": 4, "ltime": 999999800},
				Properties:  map[string]string{"userid": "alice_user", "address": "192.168.1.5", "lastCommand": "look"},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {ID: "room:100", DisplayName: "광장"},
		},
	}

	ctx := &Context{
		ActorID: "player:caster",
		Values: map[string]any{
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{
					{ID: "session-1", ActorID: "player:alice"},
				}
			},
			"test.now": int64(1000000000),
		},
	}

	handler := NewDMUsersHandler(world)
	_, err := handler(ctx, ResolvedCommand{Args: []string{"u"}})
	if err != nil {
		t.Fatal(err)
	}

	output := ctx.OutputString()
	if !strings.Contains(output, "UserID") {
		t.Errorf("expected UserID column in header, got:\n%s", output)
	}
	if !strings.Contains(output, "alice_user") {
		t.Errorf("expected alice_user in output, got:\n%s", output)
	}
}

func TestDMUsers_UserIDModeFromParsedSlotWithoutSyntheticArgs(t *testing.T) {
	world := &mockDMUsersWorld{
		players: map[model.PlayerID]model.Player{
			"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			"player:alice":  {ID: "player:alice", CreatureID: "creature:alice", AccountName: "alice_account"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:caster": {ID: "creature:caster", Stats: map[string]int{"class": model.ClassSubDM}},
			"creature:alice": {
				ID:          "creature:alice",
				DisplayName: "엘리스",
				RoomID:      "room:100",
				Level:       15,
				Stats:       map[string]int{"class": 4, "ltime": 999999800},
				Properties:  map[string]string{"userid": "alice_user", "address": "192.168.1.5", "lastCommand": "look"},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {ID: "room:100", DisplayName: "광장"},
		},
	}
	ctx := &Context{
		ActorID: "player:caster",
		Values: map[string]any{
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{{ID: "session-1", ActorID: "player:alice"}}
			},
			"test.now": int64(1000000000),
		},
	}
	resolved := ResolvedCommand{
		Input: "*users u",
		Parsed: commandparse.Command{
			Num: 2,
			Str: [commandparse.CommandMax]string{"*users", "u"},
		},
	}

	_, err := NewDMUsersHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatal(err)
	}

	output := ctx.OutputString()
	if !strings.Contains(output, "UserID") {
		t.Errorf("expected UserID column in header, got:\n%s", output)
	}
	if !strings.Contains(output, "alice_user") {
		t.Errorf("expected alice_user in output, got:\n%s", output)
	}
}

func TestDMUsers_UppercaseOptionsDoNotMatchLegacyLowercaseFlags(t *testing.T) {
	world := &mockDMUsersWorld{
		players: map[model.PlayerID]model.Player{
			"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			"player:alice":  {ID: "player:alice", CreatureID: "creature:alice", AccountName: "alice_account"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:caster": {ID: "creature:caster", Stats: map[string]int{"class": model.ClassSubDM}},
			"creature:alice": {
				ID:          "creature:alice",
				DisplayName: "엘리스",
				RoomID:      "room:100",
				Level:       15,
				Stats:       map[string]int{"class": 4, "ltime": 999999800},
				Properties:  map[string]string{"userid": "alice_user", "address": "192.168.1.5", "lastCommand": "look"},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {ID: "room:100", DisplayName: "광장"},
		},
	}
	for _, arg := range []string{"U", "F"} {
		t.Run(arg, func(t *testing.T) {
			ctx := &Context{
				ActorID: "player:caster",
				Values: map[string]any{
					"game.activeSessions": func() []testActiveSession {
						return []testActiveSession{{ID: "session-1", ActorID: "player:alice"}}
					},
					"test.now": int64(1000000000),
				},
			}

			_, err := NewDMUsersHandler(world)(ctx, ResolvedCommand{Args: []string{arg}})
			if err != nil {
				t.Fatal(err)
			}
			output := ctx.OutputString()
			if !strings.Contains(output, "Address") || strings.Contains(output, "UserID") || strings.Contains(output, "Email address") {
				t.Fatalf("uppercase %q selected a non-default header:\n%s", arg, output)
			}
			if !strings.Contains(output, "192.168.1.5") || strings.Contains(output, "alice_user@192.168.1.5") {
				t.Fatalf("uppercase %q did not preserve default address output:\n%s", arg, output)
			}
		})
	}
}

func TestDMUsers_FullMode(t *testing.T) {
	world := &mockDMUsersWorld{
		players: map[model.PlayerID]model.Player{
			"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			"player:alice":  {ID: "player:alice", CreatureID: "creature:alice", AccountName: "alice_account"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:caster": {ID: "creature:caster", Stats: map[string]int{"class": 13}}, // DM (13)
			"creature:alice": {
				ID:          "creature:alice",
				DisplayName: "엘리스",
				Level:       15,
				Stats:       map[string]int{"class": 4, "ltime": 999999800},
				Properties:  map[string]string{"userid": "alice_user", "address": "192.168.1.5"},
			},
		},
	}

	ctx := &Context{
		ActorID: "player:caster",
		Values: map[string]any{
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{
					{ID: "session-1", ActorID: "player:alice"},
				}
			},
			"test.now": int64(1000000000),
		},
	}

	handler := NewDMUsersHandler(world)
	_, err := handler(ctx, ResolvedCommand{Args: []string{"f"}})
	if err != nil {
		t.Fatal(err)
	}

	output := ctx.OutputString()
	if !strings.Contains(output, "Email address") {
		t.Errorf("expected Email address in header, got:\n%s", output)
	}
	if !strings.Contains(output, "alice_user@192.168.1.5") {
		t.Errorf("expected alice_user@192.168.1.5 in output, got:\n%s", output)
	}
}

func TestDMUsers_InvisDMFilter(t *testing.T) {
	world := &mockDMUsersWorld{
		players: map[model.PlayerID]model.Player{
			"player:caretaker": {ID: "player:caretaker", CreatureID: "creature:caretaker"},
			"player:subdm":     {ID: "player:subdm", CreatureID: "creature:subdm"},
			"player:invisdm":   {ID: "player:invisdm", CreatureID: "creature:invisdm"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:caretaker": {ID: "creature:caretaker", Stats: map[string]int{"class": model.ClassCaretaker}},
			"creature:subdm":     {ID: "creature:subdm", Stats: map[string]int{"class": 12}}, // Sub-DM
			"creature:invisdm": {
				ID:          "creature:invisdm",
				DisplayName: "운영진",
				RoomID:      "room:100",
				Level:       99,
				Stats:       map[string]int{"class": 13, "ltime": 1000000000}, // DM
				Metadata:    model.Metadata{Tags: []string{"PDMINV"}},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {ID: "room:100", DisplayName: "대궐"},
		},
	}

	activeSessionsFunc := func() []testActiveSession {
		return []testActiveSession{
			{ID: "session-invisdm", ActorID: "player:invisdm"},
		}
	}

	handler := NewDMUsersHandler(world)

	// 1. Caretaker caster -> C rejects before listing users.
	ctx1 := &Context{
		ActorID: "player:caretaker",
		Values: map[string]any{
			"game.activeSessions": activeSessionsFunc,
			"test.now":            int64(1000000000),
		},
	}
	status1, err := handler(ctx1, ResolvedCommand{})
	if err != nil {
		t.Fatal(err)
	}
	if status1 != StatusPrompt {
		t.Fatalf("caretaker status = %v, want StatusPrompt", status1)
	}
	output1 := ctx1.OutputString()
	if output1 != "" {
		t.Errorf("caretaker output = %q, want no permission output", output1)
	}

	// 2. Sub-DM caster -> should see target
	ctx2 := &Context{
		ActorID: "player:subdm",
		Values: map[string]any{
			"game.activeSessions": activeSessionsFunc,
			"test.now":            int64(1000000000),
		},
	}
	_, err = handler(ctx2, ResolvedCommand{})
	if err != nil {
		t.Fatal(err)
	}
	output2 := ctx2.OutputString()
	if !strings.Contains(output2, "운영진") {
		t.Errorf("sub-dm caster should see invisible DM player in output:\n%s", output2)
	}
}

func TestDMUsers_ANSI(t *testing.T) {
	world := &mockDMUsersWorld{
		players: map[model.PlayerID]model.Player{
			"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			"player:alice":  {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:caster": {ID: "creature:caster", Stats: map[string]int{"class": 13}},
			"creature:alice": {
				ID:          "creature:alice",
				DisplayName: "엘리스",
				RoomID:      "room:100",
				Level:       15,
				Stats:       map[string]int{"class": 4, "ltime": 999999800},
				Properties:  map[string]string{"address": "192.168.1.5", "lastCommand": "look"},
			},
		},
		rooms: map[model.RoomID]model.Room{
			"room:100": {ID: "room:100", DisplayName: "광장"},
		},
	}

	ctx := &Context{
		ActorID: "player:caster",
		Values: map[string]any{
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{
					{ID: "session-1", ActorID: "player:alice"},
				}
			},
			"test.now":           int64(1000000000),
			ContextANSIKey:       true,
			ContextANSIBrightKey: true,
		},
	}

	handler := NewDMUsersHandler(world)
	_, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatal(err)
	}

	output := ctx.OutputString()
	// Colors should be present in ANSI mode:
	// Header: bold blue (\x1b[1;34m)
	if !strings.Contains(output, "\x1b[1;34m") {
		t.Errorf("expected blue header color in output:\n%s", output)
	}
	// Player name: yellow (\x1b[1;33m)
	if !strings.Contains(output, "\x1b[1;33m") {
		t.Errorf("expected yellow player name color in output:\n%s", output)
	}
	// Address: cyan (\x1b[1;36m)
	if !strings.Contains(output, "\x1b[1;36m") {
		t.Errorf("expected cyan address color in output:\n%s", output)
	}
	// Last command: green (\x1b[1;32m)
	if !strings.Contains(output, "\x1b[1;32m") {
		t.Errorf("expected green last command color in output:\n%s", output)
	}
}
