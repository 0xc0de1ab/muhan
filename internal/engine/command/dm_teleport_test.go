package command

import (
	"fmt"
	"strings"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type mockDMTeleportWorld struct {
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
	rooms     map[model.RoomID]model.Room

	movedPlayerID   model.PlayerID
	movedDestRoomID model.RoomID

	movedCreatureID   model.CreatureID
	movedCreatureRoom model.RoomID
}

func (m *mockDMTeleportWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMTeleportWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMTeleportWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := m.rooms[id]
	return r, ok
}

func (m *mockDMTeleportWorld) MovePlayerToRoom(playerID model.PlayerID, roomID model.RoomID) error {
	m.movedPlayerID = playerID
	m.movedDestRoomID = roomID
	return nil
}

func (m *mockDMTeleportWorld) MoveCreatureToRoom(creatureID model.CreatureID, roomID model.RoomID) error {
	m.movedCreatureID = creatureID
	m.movedCreatureRoom = roomID
	return nil
}

type mockGroupMemory struct {
	followers map[string][]string
	leaders   map[string]string
}

func (g *mockGroupMemory) FollowersOf(id string) []string {
	return g.followers[id]
}

func (g *mockGroupMemory) LeaderOf(id string) (string, bool) {
	leader, ok := g.leaders[id]
	return leader, ok
}

func setupTestWorld() *mockDMTeleportWorld {
	return &mockDMTeleportWorld{
		players: map[model.PlayerID]model.Player{
			"player:dm":    {ID: "player:dm", CreatureID: "creature:dm", RoomID: "room:100"},
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:200"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob", RoomID: "room:300"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:dm":    {ID: "creature:dm", RoomID: "room:100", Stats: map[string]int{"class": 13}},
			"creature:alice": {ID: "creature:alice", DisplayName: "Alice", RoomID: "room:200", Stats: map[string]int{"class": 1}},
			"creature:bob":   {ID: "creature:bob", DisplayName: "Bob", RoomID: "room:300", Stats: map[string]int{"class": 12}},
		},
		rooms: map[model.RoomID]model.Room{
			"room:0":   {ID: "room:0"},
			"room:1":   {ID: "room:1"},
			"room:100": {ID: "room:100", CreatureIDs: []model.CreatureID{"creature:dm"}},
			"room:101": {ID: "room:101"},
			"room:200": {ID: "room:200", CreatureIDs: []model.CreatureID{"creature:alice"}},
			"room:300": {ID: "room:300", CreatureIDs: []model.CreatureID{"creature:bob"}},
		},
	}
}

func resolvedTeleportCommand(input string) ResolvedCommand {
	return resolvedTeleportParsed(input, commandparse.ParseCommandFirst(input))
}

func resolvedTeleportFinalCommand(input string) ResolvedCommand {
	return resolvedTeleportParsed(input, commandparse.Parse(input))
}

func resolvedTeleportParsed(input string, parsed commandparse.Command) ResolvedCommand {
	return ResolvedCommand{
		Input:  input,
		Parsed: parsed,
		Spec: commandspec.CommandSpec{
			Name:       "*teleport",
			Handler:    "dm_teleport",
			Privileged: true,
		},
		Args:   commandArgs(parsed),
		Values: commandValues(parsed),
	}
}

func TestDMTeleport_Authorization(t *testing.T) {
	for _, tt := range []struct {
		name  string
		class int
	}{
		{name: "regular class", class: legacyClassFighter},
		{name: "caretaker below SUB_DM", class: legacyClassCaretaker},
		{name: "bulsa below SUB_DM", class: legacyClassBulsa},
	} {
		t.Run(tt.name, func(t *testing.T) {
			world := setupTestWorld()
			world.creatures["creature:dm"] = model.Creature{ID: "creature:dm", Stats: map[string]int{"class": tt.class}}

			ctx := &Context{
				ActorID: "player:dm",
			}

			handler := NewDMTeleportHandler(world)
			status, err := handler(ctx, resolvedTeleportCommand("*teleport 200"))
			if err != nil {
				t.Fatal(err)
			}
			if status != StatusPrompt {
				t.Errorf("status = %v, want StatusPrompt", status)
			}

			output := ctx.OutputString()
			if output != "" {
				t.Errorf("expected no permission output, got %q", output)
			}
		})
	}
}

func TestDMTeleport_GroupCheck(t *testing.T) {
	world := setupTestWorld()

	ctx := &Context{
		ActorID: "player:dm",
		Values: map[string]any{
			"game.groupMemory": &mockGroupMemory{
				followers: map[string][]string{"player:dm": {"player:alice"}},
			},
		},
	}

	handler := NewDMTeleportHandler(world)
	_, err := handler(ctx, resolvedTeleportCommand("*teleport 200"))
	if err != nil {
		t.Fatal(err)
	}

	output := ctx.OutputString()
	if !strings.Contains(output, "먼저 그룹에서 나오세요.") {
		t.Errorf("expected group message, got %q", output)
	}
}

func TestDMTeleport_CaseA_RoomTeleport(t *testing.T) {
	t.Run("valid room", func(t *testing.T) {
		world := setupTestWorld()
		ctx := &Context{ActorID: "player:dm"}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, resolvedTeleportCommand("*teleport 200"))
		if err != nil {
			t.Fatal(err)
		}

		if world.movedPlayerID != "player:dm" || world.movedDestRoomID != "room:200" {
			t.Errorf("expected player:dm moved to room:200, got %s to %s", world.movedPlayerID, world.movedDestRoomID)
		}
	})

	t.Run("C-style final command room", func(t *testing.T) {
		world := setupTestWorld()
		ctx := &Context{ActorID: "player:dm"}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, resolvedTeleportFinalCommand("200 *teleport"))
		if err != nil {
			t.Fatal(err)
		}

		if world.movedPlayerID != "player:dm" || world.movedDestRoomID != "room:200" {
			t.Errorf("expected player:dm moved to room:200, got %s to %s", world.movedPlayerID, world.movedDestRoomID)
		}
	})

	t.Run("parsed numeric room without synthetic Args", func(t *testing.T) {
		world := setupTestWorld()
		ctx := &Context{ActorID: "player:dm"}
		resolved := ResolvedCommand{
			Input: "*teleport 200",
			Parsed: commandparse.Command{
				Num: 1,
				Str: [commandparse.CommandMax]string{"*teleport"},
				Val: [commandparse.CommandMax]int64{200},
			},
		}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}

		if world.movedPlayerID != "player:dm" || world.movedDestRoomID != "room:200" {
			t.Errorf("expected player:dm moved to room:200, got %s to %s", world.movedPlayerID, world.movedDestRoomID)
		}
	})

	t.Run("nonnumeric room suffix falls through to player lookup", func(t *testing.T) {
		world := setupTestWorld()
		ctx := &Context{ActorID: "player:dm"}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, resolvedTeleportCommand("*teleport 200번"))
		if err != nil {
			t.Fatal(err)
		}

		if world.movedPlayerID != "" || world.movedDestRoomID != "" {
			t.Fatalf("unexpected move %s to %s", world.movedPlayerID, world.movedDestRoomID)
		}
		if output := ctx.OutputString(); !strings.Contains(output, "200번은 접속중이 아닙니다.") {
			t.Fatalf("output = %q, want player-name lookup miss", output)
		}
	})

	t.Run("no argument defaults to room 1", func(t *testing.T) {
		world := setupTestWorld()
		ctx := &Context{ActorID: "player:dm"}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, resolvedTeleportCommand("*teleport"))
		if err != nil {
			t.Fatal(err)
		}

		if world.movedPlayerID != "player:dm" || world.movedDestRoomID != "room:1" {
			t.Errorf("expected player:dm moved to room:1, got %s to %s", world.movedPlayerID, world.movedDestRoomID)
		}
	})

	t.Run("invalid room error", func(t *testing.T) {
		world := setupTestWorld()
		ctx := &Context{ActorID: "player:dm"}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, resolvedTeleportCommand("*teleport 999"))
		if err != nil {
			t.Fatal(err)
		}

		output := ctx.OutputString()
		if !strings.Contains(output, "에러 (999)") {
			t.Errorf("expected room error message, got %q", output)
		}
	})

	t.Run("room at legacy RMAX or above is ignored without error", func(t *testing.T) {
		world := setupTestWorld()
		ctx := &Context{ActorID: "player:dm"}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, resolvedTeleportCommand("*teleport 9000"))
		if err != nil {
			t.Fatal(err)
		}

		if world.movedPlayerID != "" || world.movedDestRoomID != "" {
			t.Fatalf("unexpected move %s to %s", world.movedPlayerID, world.movedDestRoomID)
		}
		if output := ctx.OutputString(); output != "" {
			t.Fatalf("output = %q, want no C error for room >= RMAX", output)
		}
	})

	t.Run("shop check for SUB_DM caster", func(t *testing.T) {
		world := setupTestWorld()
		// Caster is SUB_DM (12)
		world.creatures["creature:dm"] = model.Creature{ID: "creature:dm", RoomID: "room:100", Stats: map[string]int{"class": 12}}
		// Set target room to 101, and room 100 (101-1) to shop (RSHOPP flag)
		r100 := world.rooms["room:100"]
		r100.Metadata.Tags = []string{"RSHOPP"}
		world.rooms["room:100"] = r100

		ctx := &Context{ActorID: "player:dm"}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, resolvedTeleportCommand("*teleport 101"))
		if err != nil {
			t.Fatal(err)
		}

		output := ctx.OutputString()
		if !strings.Contains(output, "순간이동이 금지된 구역입니다.") {
			t.Errorf("expected shop check error message, got %q", output)
		}
	})

	t.Run("follower teleport and broadcast", func(t *testing.T) {
		world := setupTestWorld()
		// Caster is in room 100
		// Add follower monster with MDMFOL tag in room 100
		world.creatures["creature:follower"] = model.Creature{
			ID:          "creature:follower",
			DisplayName: "충성스런멍멍이",
			RoomID:      "room:100",
			Metadata:    model.Metadata{Tags: []string{"MDMFOL"}},
		}
		r100 := world.rooms["room:100"]
		r100.CreatureIDs = append(r100.CreatureIDs, "creature:follower")
		world.rooms["room:100"] = r100

		var broadcastMsgs []string
		ctx := &Context{
			ActorID:   "player:dm",
			SessionID: "session:dm",
			Values: map[string]any{
				ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excluded string, text string) error {
					broadcastMsgs = append(broadcastMsgs, fmt.Sprintf("%s:%s", roomID, text))
					return nil
				}),
			},
		}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, resolvedTeleportCommand("*teleport 200"))
		if err != nil {
			t.Fatal(err)
		}

		if world.movedCreatureID != "creature:follower" || world.movedCreatureRoom != "room:200" {
			t.Errorf("expected follower moved to room:200, got %s to %s", world.movedCreatureID, world.movedCreatureRoom)
		}

		if len(broadcastMsgs) != 1 || !strings.Contains(broadcastMsgs[0], "충성스런멍멍이가 주위를 두리번 거립니다.") {
			t.Errorf("expected follower look around broadcast, got %v", broadcastMsgs)
		}
	})

	t.Run("skips follower owned by another DM like legacy first_fol", func(t *testing.T) {
		world := setupTestWorld()
		world.creatures["creature:follower"] = model.Creature{
			ID:          "creature:follower",
			DisplayName: "다른추종자",
			RoomID:      "room:100",
			Metadata:    model.Metadata{Tags: []string{"MDMFOL"}},
			Properties: map[string]string{
				dmFollowLeaderProperty:         "player:other-dm",
				dmFollowLeaderCreatureProperty: "creature:other-dm",
			},
		}
		r100 := world.rooms["room:100"]
		r100.CreatureIDs = append(r100.CreatureIDs, "creature:follower")
		world.rooms["room:100"] = r100

		var broadcastMsgs []string
		ctx := &Context{
			ActorID:   "player:dm",
			SessionID: "session:dm",
			Values: map[string]any{
				ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excluded string, text string) error {
					broadcastMsgs = append(broadcastMsgs, fmt.Sprintf("%s:%s", roomID, text))
					return nil
				}),
			},
		}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, resolvedTeleportCommand("*teleport 200"))
		if err != nil {
			t.Fatal(err)
		}

		if world.movedCreatureID != "" || world.movedCreatureRoom != "" {
			t.Fatalf("moved other DM follower %s to %s", world.movedCreatureID, world.movedCreatureRoom)
		}
		if len(broadcastMsgs) != 0 {
			t.Fatalf("broadcasts for skipped follower = %+v, want none", broadcastMsgs)
		}
	})
}

func TestDMTeleport_CaseB_PlayerTeleport(t *testing.T) {
	t.Run("valid target player", func(t *testing.T) {
		world := setupTestWorld()
		ctx := &Context{
			ActorID: "player:dm",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{
						{ID: "session:dm", ActorID: "player:dm"},
						{ID: "session:alice", ActorID: "player:alice"},
					}
				},
			},
		}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, ResolvedCommand{Args: []string{"alice"}})
		if err != nil {
			t.Fatal(err)
		}

		if world.movedPlayerID != "player:dm" || world.movedDestRoomID != "room:200" {
			t.Errorf("expected player:dm moved to room:200 (alice's room), got %s to %s", world.movedPlayerID, world.movedDestRoomID)
		}
	})

	t.Run("parsed target player without synthetic Args", func(t *testing.T) {
		world := setupTestWorld()
		ctx := &Context{
			ActorID: "player:dm",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{
						{ID: "session:dm", ActorID: "player:dm"},
						{ID: "session:alice", ActorID: "player:alice"},
					}
				},
			},
		}
		resolved := ResolvedCommand{
			Input: "*teleport alice",
			Parsed: commandparse.Command{
				Num: 2,
				Str: [commandparse.CommandMax]string{"*teleport", "alice"},
				Val: [commandparse.CommandMax]int64{1, 1},
			},
		}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}

		if world.movedPlayerID != "player:dm" || world.movedDestRoomID != "room:200" {
			t.Errorf("expected player:dm moved to room:200, got %s to %s", world.movedPlayerID, world.movedDestRoomID)
		}
	})

	t.Run("target not found", func(t *testing.T) {
		world := setupTestWorld()
		ctx := &Context{
			ActorID: "player:dm",
		}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, ResolvedCommand{Args: []string{"charlie"}})
		if err != nil {
			t.Fatal(err)
		}

		output := ctx.OutputString()
		if !strings.Contains(output, "charlie는 접속중이 아닙니다.") {
			t.Errorf("expected charlie not online message, got %q", output)
		}
	})

	t.Run("saved target without active session is not found", func(t *testing.T) {
		world := setupTestWorld()
		ctx := &Context{
			ActorID: "player:dm",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{{ID: "session:dm", ActorID: "player:dm"}}
				},
			},
		}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, ResolvedCommand{Args: []string{"alice"}})
		if err != nil {
			t.Fatal(err)
		}

		if world.movedPlayerID != "" || world.movedDestRoomID != "" {
			t.Fatalf("offline saved target moved caster: %s to %s", world.movedPlayerID, world.movedDestRoomID)
		}
		if output := ctx.OutputString(); !strings.Contains(output, "alice는 접속중이 아닙니다.") {
			t.Errorf("expected offline target miss, got %q", output)
		}
	})

	t.Run("invisible SUB_DM target cannot be seen by ZoneMaker caster", func(t *testing.T) {
		world := setupTestWorld()
		// Caster is ZONEMAKER, which passes the top-level C gate but is still below SUB_DM.
		world.creatures["creature:dm"] = model.Creature{ID: "creature:dm", RoomID: "room:100", Stats: map[string]int{"class": legacyClassZoneMaker}}
		// Target Bob is SUB_DM (12) and is invisible (PDMINV tag)
		bobCrt := world.creatures["creature:bob"]
		bobCrt.Metadata.Tags = []string{"PDMINV"}
		world.creatures["creature:bob"] = bobCrt

		ctx := &Context{
			ActorID: "player:dm",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{
						{ID: "session:dm", ActorID: "player:dm"},
						{ID: "session:bob", ActorID: "player:bob"},
					}
				},
			},
		}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, ResolvedCommand{Args: []string{"bob"}})
		if err != nil {
			t.Fatal(err)
		}

		output := ctx.OutputString()
		if !strings.Contains(output, "bob는 접속중이 아닙니다.") {
			t.Errorf("expected bob not online message, got %q", output)
		}
	})

	t.Run("follower teleport and case B broadcast", func(t *testing.T) {
		world := setupTestWorld()
		// Add follower monster with MDMFOL tag in room 100
		world.creatures["creature:follower"] = model.Creature{
			ID:          "creature:follower",
			DisplayName: "충성스런멍멍이",
			RoomID:      "room:100",
			Metadata:    model.Metadata{Tags: []string{"MDMFOL"}},
		}
		r100 := world.rooms["room:100"]
		r100.CreatureIDs = append(r100.CreatureIDs, "creature:follower")
		world.rooms["room:100"] = r100

		var broadcastMsgs []string
		ctx := &Context{
			ActorID:   "player:dm",
			SessionID: "session:dm",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{
						{ID: "session:dm", ActorID: "player:dm"},
						{ID: "session:alice", ActorID: "player:alice"},
					}
				},
				ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excluded string, text string) error {
					broadcastMsgs = append(broadcastMsgs, fmt.Sprintf("%s:%s", roomID, text))
					return nil
				}),
			},
		}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, ResolvedCommand{Args: []string{"alice"}})
		if err != nil {
			t.Fatal(err)
		}

		if world.movedCreatureID != "creature:follower" || world.movedCreatureRoom != "room:200" {
			t.Errorf("expected follower moved to room:200, got %s to %s", world.movedCreatureID, world.movedCreatureRoom)
		}

		if len(broadcastMsgs) != 1 || !strings.Contains(broadcastMsgs[0], "충성스런멍멍이가 주위를 두리번 거리며 있습니다.") {
			t.Errorf("expected Case B follower look around broadcast, got %v", broadcastMsgs)
		}
	})
}

func TestDMTeleport_CaseC_TargetToDestination(t *testing.T) {
	t.Run("teleport target to dot (caster room)", func(t *testing.T) {
		world := setupTestWorld()
		ctx := &Context{
			ActorID: "player:dm",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{
						{ID: "session:dm", ActorID: "player:dm"},
						{ID: "session:alice", ActorID: "player:alice"},
					}
				},
			},
		}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, ResolvedCommand{Args: []string{"alice", "."}})
		if err != nil {
			t.Fatal(err)
		}

		if world.movedPlayerID != "player:alice" || world.movedDestRoomID != "room:100" {
			t.Errorf("expected alice moved to caster room:100, got %s to %s", world.movedPlayerID, world.movedDestRoomID)
		}
	})

	t.Run("parsed target and destination without synthetic Args", func(t *testing.T) {
		world := setupTestWorld()
		ctx := &Context{
			ActorID: "player:dm",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{
						{ID: "session:dm", ActorID: "player:dm"},
						{ID: "session:alice", ActorID: "player:alice"},
					}
				},
			},
		}
		resolved := ResolvedCommand{
			Input: "*teleport alice .",
			Parsed: commandparse.Command{
				Num: 3,
				Str: [commandparse.CommandMax]string{"*teleport", "alice", "."},
				Val: [commandparse.CommandMax]int64{1, 1, 1},
			},
		}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}

		if world.movedPlayerID != "player:alice" || world.movedDestRoomID != "room:100" {
			t.Errorf("expected alice moved to caster room:100, got %s to %s", world.movedPlayerID, world.movedDestRoomID)
		}
	})

	t.Run("teleport target to dot-prefixed destination uses caster room", func(t *testing.T) {
		world := setupTestWorld()
		ctx := &Context{
			ActorID: "player:dm",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{
						{ID: "session:dm", ActorID: "player:dm"},
						{ID: "session:alice", ActorID: "player:alice"},
					}
				},
			},
		}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, ResolvedCommand{Args: []string{"alice", ".here"}})
		if err != nil {
			t.Fatal(err)
		}

		if world.movedPlayerID != "player:alice" || world.movedDestRoomID != "room:100" {
			t.Errorf("expected alice moved to caster room:100, got %s to %s", world.movedPlayerID, world.movedDestRoomID)
		}
	})

	t.Run("teleport target to destination player", func(t *testing.T) {
		world := setupTestWorld()
		ctx := &Context{
			ActorID: "player:dm",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{
						{ID: "session:dm", ActorID: "player:dm"},
						{ID: "session:alice", ActorID: "player:alice"},
						{ID: "session:bob", ActorID: "player:bob"},
					}
				},
			},
		}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, ResolvedCommand{Args: []string{"alice", "bob"}})
		if err != nil {
			t.Fatal(err)
		}

		if world.movedPlayerID != "player:alice" || world.movedDestRoomID != "room:300" {
			t.Errorf("expected alice moved to bob room:300, got %s to %s", world.movedPlayerID, world.movedDestRoomID)
		}
	})

	t.Run("missing destination reports target name like C", func(t *testing.T) {
		world := setupTestWorld()
		ctx := &Context{
			ActorID: "player:dm",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{
						{ID: "session:dm", ActorID: "player:dm"},
						{ID: "session:alice", ActorID: "player:alice"},
					}
				},
			},
		}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, ResolvedCommand{Args: []string{"alice", "charlie"}})
		if err != nil {
			t.Fatal(err)
		}

		output := ctx.OutputString()
		if !strings.Contains(output, "alice는 접속중이 아닙니다.") {
			t.Errorf("expected target-name miss message, got %q", output)
		}
		if strings.Contains(output, "charlie") {
			t.Errorf("output = %q, want C target-name message rather than destination name", output)
		}
		if world.movedPlayerID != "" || world.movedDestRoomID != "" {
			t.Fatalf("unexpected move %s to %s", world.movedPlayerID, world.movedDestRoomID)
		}
	})

	t.Run("target is invisible DM cannot be teleported by sub-DM", func(t *testing.T) {
		world := setupTestWorld()
		// Caster is SUB_DM (12)
		world.creatures["creature:dm"] = model.Creature{ID: "creature:dm", RoomID: "room:100", Stats: map[string]int{"class": 12}}
		// Target Bob is DM (13) and invisible (PDMINV tag)
		bobCrt := world.creatures["creature:bob"]
		bobCrt.Metadata.Tags = []string{"PDMINV"}
		bobCrt.Stats["class"] = 13
		world.creatures["creature:bob"] = bobCrt

		ctx := &Context{
			ActorID: "player:dm",
			Values: map[string]any{
				"game.activeSessions": func() []activeSession {
					return []activeSession{
						{ID: "session:dm", ActorID: "player:dm"},
						{ID: "session:bob", ActorID: "player:bob"},
					}
				},
			},
		}

		handler := NewDMTeleportHandler(world)
		_, err := handler(ctx, ResolvedCommand{Args: []string{"bob", "."}})
		if err != nil {
			t.Fatal(err)
		}

		output := ctx.OutputString()
		if !strings.Contains(output, "bob는 접속중이 아닙니다.") {
			t.Errorf("expected bob not online message, got %q", output)
		}
	})
}
