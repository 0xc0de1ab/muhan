package command

import (
	"strings"
	"testing"
	"time"

	"muhan/internal/commandspec"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestFleeHandlerMovesThroughVisibleExitAndClearsHidden(t *testing.T) {
	loaded := lookWorld(t)
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden"}
	loaded.Players[player.ID] = player
	creature := loaded.Creatures["creature:alice"]
	creature.Level = 12
	creature.Metadata.Tags = []string{"hidden", "PHIDDN"}
	creature.Stats = map[string]int{"dexterity": 30, "PHIDDN": 1}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)
	mustAddFleeEnemy(t, runtime)
	dispatcher := fleeDispatcher(t, runtime, fixedRoll(1))

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := dispatcher.DispatchLine(ctx, "도망")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "당신은 줄행랑을 칩니다.") ||
		!strings.Contains(ctx.OutputString(), "동쪽") {
		t.Fatalf("status/output = %d/%q, want flee confirmation and destination", status, ctx.OutputString())
	}
	movedPlayer, _ := runtime.Player("player:alice")
	if movedPlayer.RoomID != "room:east" {
		t.Fatalf("player room = %q, want room:east", movedPlayer.RoomID)
	}
	origin, ok := runtime.Room("room:plaza")
	if !ok {
		t.Fatal("origin room missing")
	}
	if got := origin.Properties["track"]; got != "동" {
		t.Fatalf("origin track = %q, want 동 after C flee track record", got)
	}
	updatedCreature, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updatedCreature.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("creature tags = %+v, want hidden cleared", updatedCreature.Metadata.Tags)
	}
	if updatedCreature.Stats["PHIDDN"] != 0 {
		t.Fatalf("creature PHIDDN = %d, want cleared", updatedCreature.Stats["PHIDDN"])
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("player tags = %+v, want hidden cleared", updatedPlayer.Metadata.Tags)
	}
	if len(broadcasts) != 1 || broadcasts[0].Text != "\nAlice가 동쪽으로 도망을 갑니다." {
		t.Fatalf("broadcasts = %+v", broadcasts)
	}
}

func TestFleeHandlerRequiresThreatAndReportsFailedEscape(t *testing.T) {
	loaded := lookWorld(t)
	delete(loaded.Creatures, "creature:guard")
	room := loaded.Rooms["room:plaza"]
	room.CreatureIDs = nil
	loaded.Rooms[room.ID] = room
	runtime := state.NewWorld(loaded)
	dispatcher := fleeDispatcher(t, runtime, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "도")
	if err != nil {
		t.Fatalf("DispatchLine() no threat error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "누구에게서 도망가시려구요?" {
		t.Fatalf("status/output = %d/%q, want no-threat message", status, ctx.OutputString())
	}

	runtime = state.NewWorld(lookWorld(t))
	mustAddFleeEnemy(t, runtime)
	dispatcher = fleeDispatcher(t, runtime, fixedRoll(100))
	ctx = &Context{ActorID: "player:alice"}
	status, err = dispatcher.DispatchLine(ctx, "도망")
	if err != nil {
		t.Fatalf("DispatchLine() failed flee error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n당신은 겁에 질려 다리가 떨어지지 않습니다!" {
		t.Fatalf("status/output = %d/%q, want failed flee", status, ctx.OutputString())
	}
}

func TestFleeHandlerRequiresEnemyListThreatLikeLegacy(t *testing.T) {
	runtime := state.NewWorld(lookWorld(t))
	dispatcher := fleeDispatcher(t, runtime, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "도망")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "누구에게서 도망가시려구요?" {
		t.Fatalf("status/output = %d/%q, want visible non-enemy monster ignored like C ply_is_attacking", status, ctx.OutputString())
	}
}

func TestFleeWaitsForAttackOrSpellCooldownBeforeThreatLikeLegacy(t *testing.T) {
	loaded := lookWorld(t)
	delete(loaded.Creatures, "creature:guard")
	room := loaded.Rooms["room:plaza"]
	room.CreatureIDs = nil
	loaded.Rooms[room.ID] = room
	runtime := state.NewWorld(loaded)
	if err := runtime.SetCreatureCooldown("creature:alice", "attack", time.Now().Unix(), 10); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}
	dispatcher := fleeDispatcher(t, runtime, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "도망")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "기다리세요.") || strings.Contains(ctx.OutputString(), "누구에게서") {
		t.Fatalf("status/output = %d/%q, want C attack/spell wait before no-threat check", status, ctx.OutputString())
	}
}

func TestFleeWaitUsesLongerAttackOrSpellCooldownLikeLegacy(t *testing.T) {
	runtime := state.NewWorld(lookWorld(t))
	if err := runtime.SetCreatureCooldown("creature:alice", "attack", 100, 3); err != nil {
		t.Fatalf("SetCreatureCooldown(attack) error = %v", err)
	}
	if err := runtime.SetCreatureCooldown("creature:alice", "spell", 100, 8); err != nil {
		t.Fatalf("SetCreatureCooldown(spell) error = %v", err)
	}

	remaining, ready, err := fleeAttackOrSpellReady(runtime, "creature:alice", 100)
	if err != nil {
		t.Fatalf("fleeAttackOrSpellReady() error = %v", err)
	}
	if ready || remaining != 8 {
		t.Fatalf("remaining/ready = %d/%v, want 8/false from max attack/spell wait", remaining, ready)
	}
}

func TestFleeHandlerAppliesPaladinExperiencePenalty(t *testing.T) {
	loaded := lookWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Level = 24
	creature.Stats = map[string]int{"class": legacyClassPaladin, "level": 24, "experience": 1000, "dexterity": 30}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)
	mustAddFleeEnemy(t, runtime)
	dispatcher := fleeDispatcher(t, runtime, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "도망"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	updated, _ := runtime.Creature("creature:alice")
	if updated.Stats["experience"] != 940 {
		t.Fatalf("experience = %d, want 940", updated.Stats["experience"])
	}
	if !strings.Contains(ctx.OutputString(), "60 만큼의 경험치를 잃었습니다.") {
		t.Fatalf("output = %q, want exp penalty", ctx.OutputString())
	}
}

func TestFleeHandlerChecksDestinationRestrictionsAfterSuccessLikeLegacy(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*testing.T, *worldload.World)
		wantBlock string
	}{
		{
			name: "level restriction",
			mutate: func(t *testing.T, loaded *worldload.World) {
				t.Helper()
				east := loaded.Rooms["room:east"]
				east.Properties = map[string]string{"minLevel": "50"}
				loaded.Rooms[east.ID] = east
				alice := loaded.Creatures["creature:alice"]
				alice.Stats = map[string]int{"level": 1, "dexterity": 30}
				loaded.Creatures[alice.ID] = alice
			},
			wantBlock: "어떤 힘에 의해 다시 되돌아 왔습니다.",
		},
		{
			name: "player limit",
			mutate: func(t *testing.T, loaded *worldload.World) {
				t.Helper()
				east := loaded.Rooms["room:east"]
				east.Properties = map[string]string{"onePlayer": "true"}
				loaded.Rooms[east.ID] = east
				addMoveDestinationPlayers(t, loaded, 1)
			},
			wantBlock: "도망갈려는 방의 정원이 가득 찼습니다!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := lookWorld(t)
			tt.mutate(t, loaded)
			runtime := state.NewWorld(loaded)
			mustAddFleeEnemy(t, runtime)
			dispatcher := fleeDispatcher(t, runtime, fixedRoll(1))

			var broadcasts []roomBroadcastRecord
			ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
			status, err := dispatcher.DispatchLine(ctx, "도망")
			if err != nil {
				t.Fatalf("DispatchLine() error = %v", err)
			}
			output := ctx.OutputString()
			if status != StatusDefault || !strings.Contains(output, "당신은 줄행랑을 칩니다.") || !strings.Contains(output, tt.wantBlock) {
				t.Fatalf("status/output = %d/%q, want success flee text then %q", status, output, tt.wantBlock)
			}
			if strings.Contains(output, "겁에 질려 다리가 떨어지지 않습니다") {
				t.Fatalf("output = %q, want destination block after successful flee, not failed-flee branch", output)
			}
			player, _ := runtime.Player("player:alice")
			if player.RoomID != "room:plaza" {
				t.Fatalf("player room = %q, want original room after destination block", player.RoomID)
			}
			origin, _ := runtime.Room("room:plaza")
			if got := origin.Properties["track"]; got != "동" {
				t.Fatalf("origin track = %q, want 동 despite destination block", got)
			}
			if len(broadcasts) != 2 ||
				broadcasts[0].RoomID != "room:plaza" || broadcasts[0].Text != "\nAlice가 동쪽으로 도망을 갑니다." ||
				broadcasts[1].RoomID != "room:east" || broadcasts[1].Text != "\nAlice가 도착하였습니다." {
				t.Fatalf("broadcasts = %+v, want origin flee and destination arrival broadcast", broadcasts)
			}
		})
	}
}

func TestFleeHandlerDestinationPlayerLimitIgnoresPDMINVLikeCountVisPly(t *testing.T) {
	loaded := lookWorld(t)
	east := loaded.Rooms["room:east"]
	east.Properties = map[string]string{"onePlayer": "true"}
	loaded.Rooms[east.ID] = east
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:dest-invis",
		DisplayName: "Invisible",
		CreatureID:  "creature:dest-invis",
		RoomID:      "room:east",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:dest-invis",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Invisible",
		PlayerID:    "player:dest-invis",
		RoomID:      "room:east",
		Metadata:    model.Metadata{Tags: []string{"PDMINV"}},
		Stats:       map[string]int{"PDMINV": 1},
	})
	runtime := state.NewWorld(loaded)
	mustAddFleeEnemy(t, runtime)
	dispatcher := fleeDispatcher(t, runtime, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "도망")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	output := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(output, "당신은 줄행랑을 칩니다.") {
		t.Fatalf("status/output = %d/%q, want successful flee", status, output)
	}
	if strings.Contains(output, "정원이 가득") {
		t.Fatalf("output = %q, want PDMINV occupant ignored for capacity", output)
	}
	player, _ := runtime.Player("player:alice")
	if player.RoomID != "room:east" {
		t.Fatalf("player room = %q, want room:east", player.RoomID)
	}
}

func fleeDispatcher(t *testing.T, world *state.World, roll SearchRollFunc) Dispatcher {
	t.Helper()
	return Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "도망", Number: 37, Handler: "flee"},
			{Name: "도", Number: 37, Handler: "flee"},
		}),
		Handlers: map[string]Handler{
			"flee": NewFleeHandler(world, roll),
		},
	}
}

func TestFleeHandlerFearsMessage(t *testing.T) {
	loaded := lookWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"PFEARS"}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)
	mustAddFleeEnemy(t, runtime)
	dispatcher := fleeDispatcher(t, runtime, fixedRoll(1))

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "도망")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	output := ctx.OutputString()
	if status != StatusDefault ||
		!strings.Contains(output, "당신은 줄행랑을 칩니다.") ||
		!strings.Contains(output, "당신은 겁에 질린듯 얼굴이 창백하게 변해 도망을 갑니다!") {
		t.Fatalf("status/output = %d/%q, want C normal flee message followed by fearful flee message", status, output)
	}
}

func mustAddFleeEnemy(t *testing.T, world *state.World) {
	t.Helper()
	if _, err := world.AddEnemy("creature:guard", "creature:alice"); err != nil {
		t.Fatalf("AddEnemy(guard, alice) error = %v", err)
	}
}
