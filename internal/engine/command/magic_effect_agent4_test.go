package command

import (
	"strings"
	"testing"

	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestApplyMagicTeleportSelf(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:source",
		DisplayName: "시작마을",
		Properties:  map[string]string{"minLevel": "20"},
	})
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:dest",
		DisplayName: "대상마을",
	})

	player := loaded.Players["player:alice"]
	player.RoomID = "room:source"
	loaded.Players[player.ID] = player

	creature := loaded.Creatures["creature:alice"]
	creature.RoomID = "room:source"
	creature.Level = 10
	loaded.Creatures[creature.ID] = creature

	world := state.NewWorld(loaded)
	defer world.Close()
	ctx := &Context{ActorID: "player:alice"}

	// Test teleport on self
	success, err := ApplyMagicTeleport(ctx, world, creature, model.ObjectInstance{}, ResolvedCommand{
		Args: []string{"공간이동"},
	})
	if err != nil {
		t.Fatalf("ApplyMagicTeleport failed: %v", err)
	}
	if !success {
		t.Fatalf("expected success to be true")
	}

	updatedPlayer, _ := world.Player("player:alice")
	if updatedPlayer.RoomID != "room:dest" {
		t.Errorf("expected player to move to room:dest, got %s", updatedPlayer.RoomID)
	}
	if !strings.Contains(ctx.OutputString(), "발구름질을 시작합니다") {
		t.Errorf("missing self teleport message, got: %s", ctx.OutputString())
	}
}

func TestApplyMagicTeleportOther(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:source",
		DisplayName: "시작마을",
		PlayerIDs:   []model.PlayerID{"player:alice", "player:bob"},
		Properties:  map[string]string{"minLevel": "20"},
	})
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:dest",
		DisplayName: "대상마을",
	})

	// Alice (caster)
	playerA := loaded.Players["player:alice"]
	playerA.RoomID = "room:source"
	loaded.Players[playerA.ID] = playerA

	creatureA := loaded.Creatures["creature:alice"]
	creatureA.RoomID = "room:source"
	creatureA.Level = 10
	loaded.Creatures[creatureA.ID] = creatureA

	// Bob (target)
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "밥",
		RoomID:      "room:source",
		CreatureID:  "creature:bob",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "밥",
		RoomID:      "room:source",
		PlayerID:    "player:bob",
		Level:       5,
	})

	world := state.NewWorld(loaded)
	defer world.Close()
	ctx := &Context{ActorID: "player:alice"}

	// Teleport Bob
	success, err := ApplyMagicTeleport(ctx, world, creatureA, model.ObjectInstance{}, ResolvedCommand{
		Args: []string{"공간이동", "밥"},
	})
	if err != nil {
		t.Fatalf("ApplyMagicTeleport failed: %v", err)
	}
	if !success {
		t.Fatalf("expected success to be true")
	}

	updatedBob, _ := world.Player("player:bob")
	if updatedBob.RoomID != "room:dest" {
		t.Errorf("expected Bob to move to room:dest, got %s", updatedBob.RoomID)
	}
	if !strings.Contains(ctx.OutputString(), "공간이동술 주문을 밥에게 외웁니다") {
		t.Errorf("missing caster message, got: %s", ctx.OutputString())
	}
}

func TestApplyMagicTeleportReadsStatAndPropertyBackedResistMagic(t *testing.T) {
	for _, tc := range []struct {
		name       string
		targetStat map[string]int
		targetProp map[string]string
	}{
		{name: "stat flag", targetStat: map[string]int{"PRMAGI": 1}},
		{name: "property token", targetProp: map[string]string{"flags": "PRMAGI|PBLIND"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loaded := emptyInventoryWorld(t)
			mustAddLookRoom(t, loaded, model.Room{
				ID:          "room:source",
				DisplayName: "시작마을",
				PlayerIDs:   []model.PlayerID{"player:alice", "player:bob"},
			})
			mustAddLookRoom(t, loaded, model.Room{ID: "room:dest", DisplayName: "대상마을"})

			playerA := loaded.Players["player:alice"]
			playerA.RoomID = "room:source"
			loaded.Players[playerA.ID] = playerA
			creatureA := loaded.Creatures["creature:alice"]
			creatureA.RoomID = "room:source"
			creatureA.Level = 10
			creatureA.Stats = map[string]int{"mpCurrent": 40}
			loaded.Creatures[creatureA.ID] = creatureA

			mustAddLookPlayer(t, loaded, model.Player{
				ID:          "player:bob",
				DisplayName: "밥",
				RoomID:      "room:source",
				CreatureID:  "creature:bob",
			})
			mustAddLookCreature(t, loaded, model.Creature{
				ID:          "creature:bob",
				Kind:        model.CreatureKindPlayer,
				DisplayName: "밥",
				RoomID:      "room:source",
				PlayerID:    "player:bob",
				Level:       5,
				Stats:       tc.targetStat,
				Properties:  tc.targetProp,
			})

			world := state.NewWorld(loaded)
	defer world.Close()
			ctx := &Context{ActorID: "player:alice"}
			success, err := ApplyMagicTeleport(ctx, world, creatureA, model.ObjectInstance{}, ResolvedCommand{
				Args: []string{"공간이동", "밥"},
			})
			if err != nil {
				t.Fatalf("ApplyMagicTeleport error = %v", err)
			}
			if success {
				t.Fatalf("success = true, want false")
			}
			if got, want := ctx.OutputString(), "\n밥을 공간이동 시키기엔 당신의 주문이 너무 약합니다.\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
			updatedBob, _ := world.Player("player:bob")
			if updatedBob.RoomID != "room:source" {
				t.Fatalf("bob room = %s, want unchanged room:source", updatedBob.RoomID)
			}
			updatedAlice, _ := world.Creature("creature:alice")
			if got := creatureStat(updatedAlice, "mpCurrent"); got != 20 {
				t.Fatalf("alice mpCurrent = %d, want C weak-resist cost 20", got)
			}
		})
	}
}

func TestApplyMagicTeleportResistMagicAllowsStrongCasterLikeLegacy(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:source",
		DisplayName: "시작마을",
		PlayerIDs:   []model.PlayerID{"player:alice", "player:bob"},
		Metadata:    model.Metadata{Tags: []string{"RNOTEL"}},
	})
	mustAddLookRoom(t, loaded, model.Room{ID: "room:dest", DisplayName: "대상마을"})

	playerA := loaded.Players["player:alice"]
	playerA.RoomID = "room:source"
	loaded.Players[playerA.ID] = playerA
	creatureA := loaded.Creatures["creature:alice"]
	creatureA.RoomID = "room:source"
	creatureA.Level = 80
	creatureA.Stats = map[string]int{"level": 80, "mpCurrent": 40}
	loaded.Creatures[creatureA.ID] = creatureA

	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "밥",
		RoomID:      "room:source",
		CreatureID:  "creature:bob",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "밥",
		RoomID:      "room:source",
		PlayerID:    "player:bob",
		Level:       5,
		Stats:       map[string]int{"level": 5, "PRMAGI": 1},
	})

	world := state.NewWorld(loaded)
	defer world.Close()
	ctx := &Context{ActorID: "player:alice"}
	success, err := ApplyMagicTeleport(ctx, world, creatureA, model.ObjectInstance{}, ResolvedCommand{
		Args: []string{"공간이동", "밥"},
	})
	if err != nil {
		t.Fatalf("ApplyMagicTeleport error = %v", err)
	}
	if !success {
		t.Fatalf("success = false, want C strong-caster PRMAGI success")
	}
	updatedBob, _ := world.Player("player:bob")
	if updatedBob.RoomID != "room:dest" {
		t.Fatalf("bob room = %s, want room:dest", updatedBob.RoomID)
	}
	updatedAlice, _ := world.Creature("creature:alice")
	if got := creatureStat(updatedAlice, "mpCurrent"); got != 20 {
		t.Fatalf("alice mpCurrent = %d, want C teleport success cost 20", got)
	}
}

func TestApplyMagicTeleportExplicitSelfAliasIsMissingTargetLikeLegacy(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:source",
		DisplayName: "시작마을",
		PlayerIDs:   []model.PlayerID{"player:alice"},
	})
	mustAddLookRoom(t, loaded, model.Room{ID: "room:dest", DisplayName: "대상마을"})
	player := loaded.Players["player:alice"]
	player.RoomID = "room:source"
	loaded.Players[player.ID] = player
	creature := loaded.Creatures["creature:alice"]
	creature.RoomID = "room:source"
	creature.Stats = map[string]int{"mpCurrent": 40}
	loaded.Creatures[creature.ID] = creature

	world := state.NewWorld(loaded)
	defer world.Close()
	ctx := &Context{ActorID: "player:alice"}
	success, err := ApplyMagicTeleport(ctx, world, creature, model.ObjectInstance{}, ResolvedCommand{
		Args: []string{"공간이동", "나"},
	})
	if err != nil {
		t.Fatalf("ApplyMagicTeleport error = %v", err)
	}
	if success {
		t.Fatalf("success = true, want C target miss")
	}
	if got, want := ctx.OutputString(), "\n그런 사람이 존재하지 않습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	updatedPlayer, _ := world.Player("player:alice")
	if updatedPlayer.RoomID != "room:source" {
		t.Fatalf("alice room = %s, want unchanged room:source", updatedPlayer.RoomID)
	}
	updatedAlice, _ := world.Creature("creature:alice")
	if got := creatureStat(updatedAlice, "mpCurrent"); got != 40 {
		t.Fatalf("alice mpCurrent = %d, want unchanged 40", got)
	}
}

func TestApplyMagicTeleportPotionRejectsMissingTargetBeforeLookup(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{ID: "room:source", DisplayName: "시작마을"})
	actor := loaded.Creatures["creature:alice"]
	actor.RoomID = "room:source"
	loaded.Creatures[actor.ID] = actor
	world := state.NewWorld(loaded)
	defer world.Close()
	potion := model.ObjectInstance{ID: "object:teleport-potion", Properties: map[string]string{"type": "6"}}
	ctx := &Context{ActorID: "player:alice"}

	success, err := ApplyMagicTeleport(ctx, world, actor, potion, ResolvedCommand{Args: []string{"공간이동", "Nobody"}})
	if err != nil {
		t.Fatalf("ApplyMagicTeleport error = %v", err)
	}
	if success {
		t.Fatalf("success = true, want false")
	}
	if got, want := ctx.OutputString(), "\n그 물건은 자신에게만 사용가능합니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

type mockActiveSession struct {
	ID      string
	ActorID string
}

func TestApplyMagicSummon(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:source",
		DisplayName: "시작마을",
		PlayerIDs:   []model.PlayerID{"player:alice"},
	})
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:other",
		DisplayName: "다른마을",
		PlayerIDs:   []model.PlayerID{"player:bob"},
	})

	// Alice (caster)
	playerA := loaded.Players["player:alice"]
	playerA.RoomID = "room:source"
	loaded.Players[playerA.ID] = playerA

	creatureA := loaded.Creatures["creature:alice"]
	creatureA.RoomID = "room:source"
	creatureA.Level = 10
	creatureA.Metadata.Tags = []string{"SSUMMO"}
	creatureA.Stats = map[string]int{
		"mpCurrent": 100,
	}
	loaded.Creatures[creatureA.ID] = creatureA

	// Bob (target)
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "밥",
		RoomID:      "room:other",
		CreatureID:  "creature:bob",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "밥",
		RoomID:      "room:other",
		PlayerID:    "player:bob",
		Level:       5,
	})

	world := state.NewWorld(loaded)
	defer world.Close()

	activeFunc := func() []mockActiveSession {
		return []mockActiveSession{
			{ID: "session:bob", ActorID: "player:bob"},
		}
	}

	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.activeSessions": activeFunc,
			"game.sendToSession": func(sessionID string, msg any) {
				// mock send
			},
		},
	}

	previousRoll := attackRoll
	attackRoll = func(min, max int) int { return max }
	t.Cleanup(func() { attackRoll = previousRoll })

	success, err := ApplyMagicSummon(ctx, world, creatureA, model.ObjectInstance{}, ResolvedCommand{
		Args: []string{"소환", "밥"},
	})
	if err != nil {
		t.Fatalf("ApplyMagicSummon failed: %v", err)
	}

	if !success {
		t.Fatalf("expected success")
	}

	updatedBob, _ := world.Player("player:bob")
	if updatedBob.RoomID != "room:source" {
		t.Errorf("expected Bob to be summoned to room:source, got %s", updatedBob.RoomID)
	}
	if !strings.Contains(ctx.OutputString(), "을 소환하기 위해 주문을 외웁니다") {
		t.Errorf("missing caster message, got: %s", ctx.OutputString())
	}
}

func TestApplyMagicSummonRejectsActivePlayerIDAliasLikeLegacy(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:source",
		DisplayName: "시작마을",
		PlayerIDs:   []model.PlayerID{"player:alice"},
	})
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:other",
		DisplayName: "다른마을",
		PlayerIDs:   []model.PlayerID{"player:bob"},
	})

	playerA := loaded.Players["player:alice"]
	playerA.RoomID = "room:source"
	loaded.Players[playerA.ID] = playerA
	creatureA := loaded.Creatures["creature:alice"]
	creatureA.RoomID = "room:source"
	creatureA.Metadata.Tags = []string{"SSUMMO"}
	creatureA.Stats = map[string]int{"mpCurrent": 100}
	loaded.Creatures[creatureA.ID] = creatureA

	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "홍길동",
		RoomID:      "room:other",
		CreatureID:  "creature:bob",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "홍길동",
		RoomID:      "room:other",
		PlayerID:    "player:bob",
		Level:       5,
	})

	world := state.NewWorld(loaded)
	defer world.Close()
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []mockActiveSession {
				return []mockActiveSession{
					{ID: "session:alice", ActorID: "player:alice"},
					{ID: "session:bob", ActorID: "player:bob"},
				}
			},
		},
	}
	previousRoll := attackRoll
	attackRoll = func(min, max int) int { return max }
	t.Cleanup(func() { attackRoll = previousRoll })

	success, err := ApplyMagicSummon(ctx, world, creatureA, model.ObjectInstance{}, ResolvedCommand{
		Args: []string{"소환", "player:bob"},
	})
	if err != nil {
		t.Fatalf("ApplyMagicSummon error = %v", err)
	}
	if success {
		t.Fatal("summon succeeded through Go-only player ID alias")
	}
	if got, want := ctx.OutputString(), "\n그런 사람을 못 찾습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	updatedBob, _ := world.Player("player:bob")
	if updatedBob.RoomID != "room:other" {
		t.Fatalf("bob room = %s, want unchanged room:other", updatedBob.RoomID)
	}
	updatedAlice, _ := world.Creature("creature:alice")
	if got := updatedAlice.Stats["mpCurrent"]; got != 100 {
		t.Fatalf("mpCurrent = %d, want no C summon cost before target lookup succeeds", got)
	}
}

func TestApplyMagicSummonPotionRejectsMissingTargetBeforeLookup(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{ID: "room:source", DisplayName: "시작마을"})
	actor := loaded.Creatures["creature:alice"]
	actor.RoomID = "room:source"
	actor.Stats = map[string]int{"mpCurrent": 100}
	loaded.Creatures[actor.ID] = actor
	world := state.NewWorld(loaded)
	defer world.Close()
	previousRoll := attackRoll
	attackRoll = func(min, max int) int { return max }
	t.Cleanup(func() { attackRoll = previousRoll })
	potion := model.ObjectInstance{ID: "object:summon-potion", Properties: map[string]string{"type": "6"}}
	ctx := &Context{ActorID: "player:alice"}

	success, err := ApplyMagicSummon(ctx, world, actor, potion, ResolvedCommand{Args: []string{"소환", "Nobody"}})
	if err != nil {
		t.Fatalf("ApplyMagicSummon error = %v", err)
	}
	if success {
		t.Fatalf("success = true, want false")
	}
	if got, want := ctx.OutputString(), "\n그 물건은 자신에게만 사용할수 있습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestApplyMagicSummonScrollBypassesCastGatesAndAvoidsTargetBroadcastDuplicate(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:source",
		DisplayName: "시작마을",
		PlayerIDs:   []model.PlayerID{"player:alice"},
	})
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:other",
		DisplayName: "다른마을",
		PlayerIDs:   []model.PlayerID{"player:bob"},
	})

	playerA := loaded.Players["player:alice"]
	playerA.RoomID = "room:source"
	loaded.Players[playerA.ID] = playerA
	creatureA := loaded.Creatures["creature:alice"]
	creatureA.RoomID = "room:source"
	creatureA.DisplayName = "Alice"
	creatureA.Stats = map[string]int{"mpCurrent": 0}
	loaded.Creatures[creatureA.ID] = creatureA

	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "밥",
		RoomID:      "room:other",
		CreatureID:  "creature:bob",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "밥",
		RoomID:      "room:other",
		PlayerID:    "player:bob",
		Level:       5,
	})

	world := state.NewWorld(loaded)
	defer world.Close()
	sent := map[string][]string{}
	ctx := &Context{
		ActorID:   "player:alice",
		SessionID: "session:alice",
		Values: map[string]any{
			"game.activeSessions": func() []mockActiveSession {
				return []mockActiveSession{
					{ID: "session:alice", ActorID: "player:alice"},
					{ID: "session:bob", ActorID: "player:bob"},
				}
			},
			"game.sendToSession": func(sessionID string, msg string) {
				sent[sessionID] = append(sent[sessionID], msg)
			},
		},
	}
	previousRoll := attackRoll
	attackRoll = func(min, max int) int { return max }
	t.Cleanup(func() { attackRoll = previousRoll })

	scroll := model.ObjectInstance{ID: "object:scroll", Properties: map[string]string{"type": "7"}}
	success, err := ApplyMagicSummon(ctx, world, creatureA, scroll, ResolvedCommand{Args: []string{"소환", "밥"}})
	if err != nil {
		t.Fatalf("ApplyMagicSummon error = %v", err)
	}
	if !success {
		t.Fatalf("success = false, want true; output=%q", ctx.OutputString())
	}
	updatedBob, _ := world.Player("player:bob")
	if updatedBob.RoomID != "room:source" {
		t.Fatalf("bob room = %s, want room:source", updatedBob.RoomID)
	}
	if len(sent["session:bob"]) != 1 {
		t.Fatalf("bob messages = %+v, want exactly one direct summon message", sent["session:bob"])
	}
	if !strings.Contains(sent["session:bob"][0], "알 수 없는 힘에 이끌려 어디론가 날라갑니다.\n") {
		t.Fatalf("bob message = %q, want C target message without extra space", sent["session:bob"][0])
	}
}

func TestApplyMagicSummonReadsStatAndPropertyBackedNoSummonFlag(t *testing.T) {
	for _, tc := range []struct {
		name       string
		targetStat map[string]int
		targetProp map[string]string
	}{
		{name: "stat flag", targetStat: map[string]int{"PNOSUM": 1}},
		{name: "property token", targetProp: map[string]string{"flags": "PNOSUM|hidden"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loaded := emptyInventoryWorld(t)
			mustAddLookRoom(t, loaded, model.Room{
				ID:          "room:source",
				DisplayName: "시작마을",
				PlayerIDs:   []model.PlayerID{"player:alice"},
			})
			mustAddLookRoom(t, loaded, model.Room{
				ID:          "room:other",
				DisplayName: "다른마을",
				PlayerIDs:   []model.PlayerID{"player:bob"},
			})
			actor := loaded.Creatures["creature:alice"]
			actor.RoomID = "room:source"
			actor.Metadata.Tags = []string{"SSUMMO"}
			actor.Stats = map[string]int{"mpCurrent": 100}
			loaded.Creatures[actor.ID] = actor
			mustAddLookPlayer(t, loaded, model.Player{ID: "player:bob", DisplayName: "밥", RoomID: "room:other", CreatureID: "creature:bob"})
			mustAddLookCreature(t, loaded, model.Creature{
				ID:          "creature:bob",
				Kind:        model.CreatureKindPlayer,
				DisplayName: "밥",
				RoomID:      "room:other",
				PlayerID:    "player:bob",
				Stats:       tc.targetStat,
				Properties:  tc.targetProp,
			})
			world := state.NewWorld(loaded)
	defer world.Close()
			previousRoll := attackRoll
			attackRoll = func(min, max int) int { return max }
			t.Cleanup(func() { attackRoll = previousRoll })
			ctx := &Context{
				ActorID: "player:alice",
				Values: map[string]any{
					"game.activeSessions": func() []mockActiveSession {
						return []mockActiveSession{{ID: "session:bob", ActorID: "player:bob"}}
					},
				},
			}

			success, err := ApplyMagicSummon(ctx, world, actor, model.ObjectInstance{}, ResolvedCommand{Args: []string{"소환", "밥"}})
			if err != nil {
				t.Fatalf("ApplyMagicSummon error = %v", err)
			}
			want := "\n주문이 실패했습니다.\n상대가 소환 거부 중입니다."
			if success || ctx.OutputString() != want {
				t.Fatalf("success/output = %v/%q, want %q", success, ctx.OutputString(), want)
			}
			updatedBob, _ := world.Player("player:bob")
			if updatedBob.RoomID != "room:other" {
				t.Fatalf("bob room = %s, want unchanged room:other", updatedBob.RoomID)
			}
		})
	}
}

func TestApplyMagicSummonLegacyFailureOutputs(t *testing.T) {
	t.Run("self target", func(t *testing.T) {
		loaded := emptyInventoryWorld(t)
		actor := loaded.Creatures["creature:alice"]
		actor.Metadata.Tags = []string{"SSUMMO"}
		actor.Stats = map[string]int{"mpCurrent": 100}
		loaded.Creatures[actor.ID] = actor
		world := state.NewWorld(loaded)
	defer world.Close()
		previousRoll := attackRoll
		attackRoll = func(min, max int) int { return max }
		t.Cleanup(func() { attackRoll = previousRoll })
		ctx := &Context{ActorID: "player:alice"}
		success, err := ApplyMagicSummon(ctx, world, actor, model.ObjectInstance{}, ResolvedCommand{Args: []string{"소환"}})
		if err != nil {
			t.Fatalf("ApplyMagicSummon error = %v", err)
		}
		if success || ctx.OutputString() != "\n자신을 소환하다뇨?.\n" {
			t.Fatalf("success/output = %v/%q, want C self rejection", success, ctx.OutputString())
		}
	})

	t.Run("random failure precedes self target rejection", func(t *testing.T) {
		loaded := emptyInventoryWorld(t)
		actor := loaded.Creatures["creature:alice"]
		actor.Metadata.Tags = []string{"SSUMMO"}
		actor.Stats = map[string]int{"mpCurrent": 100}
		loaded.Creatures[actor.ID] = actor
		world := state.NewWorld(loaded)
	defer world.Close()
		previousRoll := attackRoll
		attackRoll = func(min, max int) int { return min }
		t.Cleanup(func() { attackRoll = previousRoll })

		ctx := &Context{ActorID: "player:alice"}
		success, err := ApplyMagicSummon(ctx, world, actor, model.ObjectInstance{}, ResolvedCommand{Args: []string{"소환"}})
		if err != nil {
			t.Fatalf("ApplyMagicSummon error = %v", err)
		}
		if success || ctx.OutputString() != "\n소환에 실패를 하였습니다.\n" {
			t.Fatalf("success/output = %v/%q, want C random failure before self rejection", success, ctx.OutputString())
		}
		updated, _ := world.Creature(actor.ID)
		if got := updated.Stats["mpCurrent"]; got != 50 {
			t.Fatalf("mpCurrent = %d, want 50 after C random failure cost", got)
		}
	})

	t.Run("random failure", func(t *testing.T) {
		loaded := emptyInventoryWorld(t)
		mustAddLookRoom(t, loaded, model.Room{ID: "room:source", DisplayName: "시작마을"})
		actor := loaded.Creatures["creature:alice"]
		actor.RoomID = "room:source"
		actor.Metadata.Tags = []string{"SSUMMO"}
		actor.Stats = map[string]int{"mpCurrent": 100}
		loaded.Creatures[actor.ID] = actor
		world := state.NewWorld(loaded)
	defer world.Close()
		previousRoll := attackRoll
		attackRoll = func(min, max int) int { return min }
		t.Cleanup(func() { attackRoll = previousRoll })

		ctx := &Context{ActorID: "player:alice"}
		success, err := ApplyMagicSummon(ctx, world, actor, model.ObjectInstance{}, ResolvedCommand{Args: []string{"소환", "없는"}})
		if err != nil {
			t.Fatalf("ApplyMagicSummon error = %v", err)
		}
		if success || ctx.OutputString() != "\n소환에 실패를 하였습니다.\n" {
			t.Fatalf("success/output = %v/%q, want C random failure", success, ctx.OutputString())
		}
		updated, _ := world.Creature(actor.ID)
		if got := updated.Stats["mpCurrent"]; got != 50 {
			t.Fatalf("mpCurrent = %d, want 50 after C failure cost", got)
		}
	})

	t.Run("summon refusal", func(t *testing.T) {
		loaded := emptyInventoryWorld(t)
		mustAddLookRoom(t, loaded, model.Room{
			ID:          "room:source",
			DisplayName: "시작마을",
			PlayerIDs:   []model.PlayerID{"player:alice"},
		})
		mustAddLookRoom(t, loaded, model.Room{
			ID:          "room:other",
			DisplayName: "다른마을",
			PlayerIDs:   []model.PlayerID{"player:bob"},
		})
		actor := loaded.Creatures["creature:alice"]
		actor.RoomID = "room:source"
		actor.Metadata.Tags = []string{"SSUMMO"}
		actor.Stats = map[string]int{"mpCurrent": 100}
		loaded.Creatures[actor.ID] = actor
		mustAddLookPlayer(t, loaded, model.Player{ID: "player:bob", DisplayName: "밥", RoomID: "room:other", CreatureID: "creature:bob"})
		mustAddLookCreature(t, loaded, model.Creature{
			ID:          "creature:bob",
			Kind:        model.CreatureKindPlayer,
			DisplayName: "밥",
			RoomID:      "room:other",
			PlayerID:    "player:bob",
			Metadata:    model.Metadata{Tags: []string{"PNOSUM"}},
		})
		world := state.NewWorld(loaded)
	defer world.Close()
		previousRoll := attackRoll
		attackRoll = func(min, max int) int { return max }
		t.Cleanup(func() { attackRoll = previousRoll })
		ctx := &Context{
			ActorID: "player:alice",
			Values: map[string]any{
				"game.activeSessions": func() []mockActiveSession {
					return []mockActiveSession{{ID: "session:bob", ActorID: "player:bob"}}
				},
			},
		}

		success, err := ApplyMagicSummon(ctx, world, actor, model.ObjectInstance{}, ResolvedCommand{Args: []string{"소환", "밥"}})
		if err != nil {
			t.Fatalf("ApplyMagicSummon error = %v", err)
		}
		want := "\n주문이 실패했습니다.\n상대가 소환 거부 중입니다."
		if success || ctx.OutputString() != want {
			t.Fatalf("success/output = %v/%q, want %q", success, ctx.OutputString(), want)
		}
		updated, _ := world.Creature(actor.ID)
		if got := updated.Stats["mpCurrent"]; got != 50 {
			t.Fatalf("mpCurrent = %d, want C target-found precheck cost before PNOSUM failure", got)
		}
	})

	t.Run("destination RNOTEL consumes cast cost before failure", func(t *testing.T) {
		loaded := emptyInventoryWorld(t)
		mustAddLookRoom(t, loaded, model.Room{
			ID:          "room:source",
			DisplayName: "시작마을",
			PlayerIDs:   []model.PlayerID{"player:alice"},
			Metadata:    model.Metadata{Tags: []string{"RNOTEL"}},
		})
		mustAddLookRoom(t, loaded, model.Room{
			ID:          "room:other",
			DisplayName: "다른마을",
			PlayerIDs:   []model.PlayerID{"player:bob"},
		})
		actor := loaded.Creatures["creature:alice"]
		actor.RoomID = "room:source"
		actor.Metadata.Tags = []string{"SSUMMO"}
		actor.Stats = map[string]int{"mpCurrent": 100}
		loaded.Creatures[actor.ID] = actor
		mustAddLookPlayer(t, loaded, model.Player{ID: "player:bob", DisplayName: "밥", RoomID: "room:other", CreatureID: "creature:bob"})
		mustAddLookCreature(t, loaded, model.Creature{
			ID:          "creature:bob",
			Kind:        model.CreatureKindPlayer,
			DisplayName: "밥",
			RoomID:      "room:other",
			PlayerID:    "player:bob",
		})
		world := state.NewWorld(loaded)
	defer world.Close()
		previousRoll := attackRoll
		attackRoll = func(min, max int) int { return max }
		t.Cleanup(func() { attackRoll = previousRoll })
		ctx := &Context{
			ActorID: "player:alice",
			Values: map[string]any{
				"game.activeSessions": func() []mockActiveSession {
					return []mockActiveSession{{ID: "session:bob", ActorID: "player:bob"}}
				},
			},
		}

		success, err := ApplyMagicSummon(ctx, world, actor, model.ObjectInstance{}, ResolvedCommand{Args: []string{"소환", "밥"}})
		if err != nil {
			t.Fatalf("ApplyMagicSummon error = %v", err)
		}
		if success || ctx.OutputString() != "주문이 공중으로 빨려듭니다.\n" {
			t.Fatalf("success/output = %v/%q, want RNOTEL failure", success, ctx.OutputString())
		}
		updated, _ := world.Creature(actor.ID)
		if got := updated.Stats["mpCurrent"]; got != 50 {
			t.Fatalf("mpCurrent = %d, want C cost before RNOTEL failure", got)
		}
	})

	t.Run("bulsa uses normal fifty mp summon cost", func(t *testing.T) {
		loaded := emptyInventoryWorld(t)
		mustAddLookRoom(t, loaded, model.Room{
			ID:          "room:source",
			DisplayName: "시작마을",
			PlayerIDs:   []model.PlayerID{"player:alice"},
		})
		mustAddLookRoom(t, loaded, model.Room{
			ID:          "room:other",
			DisplayName: "다른마을",
			PlayerIDs:   []model.PlayerID{"player:bob"},
		})
		actor := loaded.Creatures["creature:alice"]
		actor.RoomID = "room:source"
		actor.Metadata.Tags = []string{"SSUMMO"}
		actor.Stats = map[string]int{"class": model.ClassBulsa, "mpCurrent": 50}
		loaded.Creatures[actor.ID] = actor
		mustAddLookPlayer(t, loaded, model.Player{ID: "player:bob", DisplayName: "밥", RoomID: "room:other", CreatureID: "creature:bob"})
		mustAddLookCreature(t, loaded, model.Creature{
			ID:          "creature:bob",
			Kind:        model.CreatureKindPlayer,
			DisplayName: "밥",
			RoomID:      "room:other",
			PlayerID:    "player:bob",
		})
		world := state.NewWorld(loaded)
	defer world.Close()
		previousRoll := attackRoll
		attackRoll = func(min, max int) int { return max }
		t.Cleanup(func() { attackRoll = previousRoll })
		ctx := &Context{
			ActorID: "player:alice",
			Values: map[string]any{
				"game.activeSessions": func() []mockActiveSession {
					return []mockActiveSession{{ID: "session:bob", ActorID: "player:bob"}}
				},
			},
		}

		success, err := ApplyMagicSummon(ctx, world, actor, model.ObjectInstance{}, ResolvedCommand{Args: []string{"소환", "밥"}})
		if err != nil {
			t.Fatalf("ApplyMagicSummon error = %v", err)
		}
		if !success {
			t.Fatalf("success = false, want C BULSA success; output=%q", ctx.OutputString())
		}
		updated, _ := world.Creature(actor.ID)
		if got := updated.Stats["mpCurrent"]; got != 0 {
			t.Fatalf("mpCurrent = %d, want 0 after normal C summon cost", got)
		}
	})

	t.Run("room player limit ignores PDMINV occupants", func(t *testing.T) {
		loaded := emptyInventoryWorld(t)
		mustAddLookRoom(t, loaded, model.Room{
			ID:          "room:source",
			DisplayName: "시작마을",
			PlayerIDs:   []model.PlayerID{"player:alice"},
			Metadata:    model.Metadata{Tags: []string{"RONEPL"}},
		})
		mustAddLookRoom(t, loaded, model.Room{
			ID:          "room:other",
			DisplayName: "다른마을",
			PlayerIDs:   []model.PlayerID{"player:bob"},
		})
		actor := loaded.Creatures["creature:alice"]
		actor.RoomID = "room:source"
		actor.Metadata.Tags = []string{"SSUMMO", "PDMINV"}
		actor.Stats = map[string]int{"mpCurrent": 50}
		loaded.Creatures[actor.ID] = actor
		mustAddLookPlayer(t, loaded, model.Player{ID: "player:bob", DisplayName: "밥", RoomID: "room:other", CreatureID: "creature:bob"})
		mustAddLookCreature(t, loaded, model.Creature{
			ID:          "creature:bob",
			Kind:        model.CreatureKindPlayer,
			DisplayName: "밥",
			RoomID:      "room:other",
			PlayerID:    "player:bob",
		})
		world := state.NewWorld(loaded)
	defer world.Close()
		previousRoll := attackRoll
		attackRoll = func(min, max int) int { return max }
		t.Cleanup(func() { attackRoll = previousRoll })
		ctx := &Context{
			ActorID: "player:alice",
			Values: map[string]any{
				"game.activeSessions": func() []mockActiveSession {
					return []mockActiveSession{{ID: "session:bob", ActorID: "player:bob"}}
				},
			},
		}

		success, err := ApplyMagicSummon(ctx, world, actor, model.ObjectInstance{}, ResolvedCommand{Args: []string{"소환", "밥"}})
		if err != nil {
			t.Fatalf("ApplyMagicSummon error = %v", err)
		}
		if !success {
			t.Fatalf("success = false, want C count_vis_ply to ignore PDMINV caster; output=%q", ctx.OutputString())
		}
		bob, _ := world.Player("player:bob")
		if bob.RoomID != "room:source" {
			t.Fatalf("bob room = %q, want room:source", bob.RoomID)
		}
	})

	t.Run("only-family room blocks dm target mismatch like C", func(t *testing.T) {
		loaded := emptyInventoryWorld(t)
		mustAddLookRoom(t, loaded, model.Room{
			ID:          "room:source",
			DisplayName: "시작마을",
			PlayerIDs:   []model.PlayerID{"player:alice"},
			Metadata:    model.Metadata{Tags: []string{"RONFML"}},
			Properties:  map[string]string{"special": "7"},
		})
		mustAddLookRoom(t, loaded, model.Room{
			ID:          "room:other",
			DisplayName: "다른마을",
			PlayerIDs:   []model.PlayerID{"player:bob"},
		})
		actor := loaded.Creatures["creature:alice"]
		actor.RoomID = "room:source"
		actor.Metadata.Tags = []string{"SSUMMO"}
		actor.Stats = map[string]int{"mpCurrent": 100}
		loaded.Creatures[actor.ID] = actor
		mustAddLookPlayer(t, loaded, model.Player{ID: "player:bob", DisplayName: "밥", RoomID: "room:other", CreatureID: "creature:bob"})
		mustAddLookCreature(t, loaded, model.Creature{
			ID:          "creature:bob",
			Kind:        model.CreatureKindPlayer,
			DisplayName: "밥",
			RoomID:      "room:other",
			PlayerID:    "player:bob",
			Stats:       map[string]int{"class": model.ClassDM, "dailyExpndMax": 8},
		})
		world := state.NewWorld(loaded)
	defer world.Close()
		previousRoll := attackRoll
		attackRoll = func(min, max int) int { return max }
		t.Cleanup(func() { attackRoll = previousRoll })
		ctx := &Context{
			ActorID: "player:alice",
			Values: map[string]any{
				"game.activeSessions": func() []mockActiveSession {
					return []mockActiveSession{{ID: "session:bob", ActorID: "player:bob"}}
				},
			},
		}

		success, err := ApplyMagicSummon(ctx, world, actor, model.ObjectInstance{}, ResolvedCommand{Args: []string{"소환", "밥"}})
		if err != nil {
			t.Fatalf("ApplyMagicSummon error = %v", err)
		}
		if success || ctx.OutputString() != "그사람은 이곳에 올수 없습니다." {
			t.Fatalf("success/output = %v/%q, want C RONFML block", success, ctx.OutputString())
		}
		updated, _ := world.Creature(actor.ID)
		if got := updated.Stats["mpCurrent"]; got != 50 {
			t.Fatalf("mpCurrent = %d, want C cost before RONFML block", got)
		}
	})
}
