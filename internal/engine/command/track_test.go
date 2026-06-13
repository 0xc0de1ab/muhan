package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestTrackHandlerIgnoresTargetArgsLikeLegacyRoomTrack(t *testing.T) {
	loaded := trackWorld(t, model.ClassRanger)
	room := loaded.Rooms["room:plaza"]
	room.Properties = map[string]string{"track": "east"}
	loaded.Rooms[room.ID] = room
	world := state.NewWorld(loaded)
	defer world.Close()
	handler := newTrackHandler(world, fixedRoll(1))

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "동쪽으로 흔적이 나 있습니다." {
		t.Fatalf("status/output = %d/%q, want legacy room track despite target arg", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0].Text != "\nAlice이 적이 지나간 흔적을 찾았습니다." {
		t.Fatalf("broadcasts = %+v", broadcasts)
	}
}

func TestTrackHandlerDoesNotTrackAdjacentTargetsLikeLegacy(t *testing.T) {
	loaded := trackWorld(t, model.ClassRanger)
	moveTrackPlayerToRoom(loaded, "player:bob", "creature:bob", "room:east")
	world := state.NewWorld(loaded)
	defer world.Close()
	handler := newTrackHandler(world, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "아무런 흔적이 남아있지 않습니다." {
		t.Fatalf("status/output = %d/%q, want C command track to ignore adjacent target", status, ctx.OutputString())
	}
}

func TestTrackHandlerReportsLegacyRoomTrackWithoutTarget(t *testing.T) {
	loaded := trackWorld(t, model.ClassRanger)
	room := loaded.Rooms["room:plaza"]
	room.Properties = map[string]string{"track": "west"}
	loaded.Rooms[room.ID] = room
	world := state.NewWorld(loaded)
	defer world.Close()
	handler := newTrackHandler(world, fixedRoll(1))

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "서쪽으로 흔적이 나 있습니다." {
		t.Fatalf("status/output = %d/%q, want legacy room track", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0].Text != "\nAlice이 적이 지나간 흔적을 찾았습니다." {
		t.Fatalf("broadcasts = %+v", broadcasts)
	}
}

func TestTrackHandlerIgnoresMissingTargetLikeLegacy(t *testing.T) {
	loaded := trackWorld(t, model.ClassRanger)
	moveTrackPlayerToRoom(loaded, "player:bob", "creature:bob", "room:far")
	world := state.NewWorld(loaded)
	defer world.Close()
	handler := newTrackHandler(world, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "아무런 흔적이 남아있지 않습니다." {
		t.Fatalf("status/output = %d/%q, want C command track to ignore missing target", status, ctx.OutputString())
	}
}

func TestTrackHandlerRejectsClassAndBlindActor(t *testing.T) {
	tests := []struct {
		name  string
		class int
		tags  []string
		want  string
	}{
		{name: "wrong class", class: model.ClassFighter, want: "포졸만 쓸수 있는 명령입니다."},
		{name: "blind", class: model.ClassRanger, tags: []string{"blind"}, want: "당신은 눈이 멀어 있습니다. 도저히 추적을 할 수 없습니다."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := trackWorld(t, tt.class)
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = tt.tags
			loaded.Creatures[alice.ID] = alice
			world := state.NewWorld(loaded)
	defer world.Close()
			handler := newTrackHandler(world, fixedRoll(1))

			ctx := &Context{ActorID: "player:alice"}
			status, err := handler(ctx, ResolvedCommand{Args: []string{"Bob"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestTrackHandlerCanBeRegisteredByDispatcher(t *testing.T) {
	loaded := trackWorld(t, model.ClassRanger)
	room := loaded.Rooms["room:plaza"]
	room.Properties = map[string]string{"track": "east"}
	loaded.Rooms[room.ID] = room
	world := state.NewWorld(loaded)
	defer world.Close()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "추적", Number: 21, Handler: "track"},
		{Name: "track", Number: 21, Handler: "track"},
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"track": newTrackHandler(world, fixedRoll(1)),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "Bob 추적")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "동쪽으로 흔적이 나 있습니다." {
		t.Fatalf("status/output = %d/%q, want registered track handler", status, ctx.OutputString())
	}
}

func TestTrackHandlerReportsLegacyNoRoomTrack(t *testing.T) {
	world := state.NewWorld(trackWorld(t, model.ClassRanger))
	defer world.Close()
	handler := newTrackHandler(world, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "아무런 흔적이 남아있지 않습니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
}

func TestTrackHandlerFailureAndCooldownLikeLegacy(t *testing.T) {
	loaded := trackWorld(t, model.ClassRanger)
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 1
	alice.Stats = map[string]int{"class": model.ClassRanger, "level": 1, "dexterity": 10}
	loaded.Creatures[alice.ID] = alice
	world := state.NewWorld(loaded)
	defer world.Close()
	handler := newTrackHandler(world, fixedRoll(100))

	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("first handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "추적 실패!" {
		t.Fatalf("first status/output = %d/%q", status, ctx.OutputString())
	}
	ctx = &Context{ActorID: "player:alice"}
	status, err = handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("second handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "기다리세요.") {
		t.Fatalf("second status/output = %d/%q, want wait", status, ctx.OutputString())
	}
}

func TestTrackHandlerClearsHiddenBeforeCooldownLikeLegacy(t *testing.T) {
	loaded := trackWorld(t, model.ClassRanger)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN"}
	alice.Stats["PHIDDN"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	defer world.Close()
	handler := newTrackHandler(world, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	if _, err := handler(ctx, ResolvedCommand{}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	updated, _ := world.Creature("creature:alice")
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "phiddn") || updated.Stats["PHIDDN"] != 0 {
		t.Fatalf("actor hidden state = tags %+v stat %d, want cleared", updated.Metadata.Tags, updated.Stats["PHIDDN"])
	}
	updatedPlayer, _ := world.Player("player:alice")
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("player tags = %+v, want hidden cleared", updatedPlayer.Metadata.Tags)
	}
}

func trackWorld(t *testing.T, class int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:plaza",
		DisplayName: "광장",
		Exits: []model.Exit{
			{Name: "동", ToRoomID: "room:east"},
			{Name: "서", ToRoomID: "room:west"},
		},
	})
	mustAddLookRoom(t, loaded, model.Room{ID: "room:east", DisplayName: "동쪽"})
	mustAddLookRoom(t, loaded, model.Room{ID: "room:west", DisplayName: "서쪽"})
	mustAddLookRoom(t, loaded, model.Room{ID: "room:far", DisplayName: "먼 곳"})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:plaza",
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:plaza",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:plaza",
		Level:       12,
		Stats:       map[string]int{"class": class, "level": 12, "dexterity": 30},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:plaza",
		Level:       1,
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:guard",
		Kind:        model.CreatureKindNPC,
		DisplayName: "경비병",
		RoomID:      "room:east",
		Level:       1,
	})
	return loaded
}

func moveTrackPlayerToRoom(loaded *worldload.World, playerID model.PlayerID, creatureID model.CreatureID, roomID model.RoomID) {
	player := loaded.Players[playerID]
	player.RoomID = roomID
	loaded.Players[player.ID] = player

	creature := loaded.Creatures[creatureID]
	creature.RoomID = roomID
	loaded.Creatures[creature.ID] = creature
}
