package command

import (
	"strings"
	"testing"

	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestMagicEffectMendSelf(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{ID: "room:church", DisplayName: "성당"})

	// Caster setup
	player := loaded.Players["player:alice"]
	player.RoomID = "room:church"
	loaded.Players[player.ID] = player

	creature := loaded.Creatures["creature:alice"]
	creature.RoomID = "room:church"
	creature.Level = 8
	creature.Metadata.Tags = []string{"SMENDW"}
	creature.Stats = map[string]int{
		"class":        model.ClassCleric,
		"level":        8,
		"intelligence": 15,
		"piety":        15,
		"hpCurrent":    10,
		"hpMax":        50,
		"mpCurrent":    20,
		"mpMax":        20,
	}
	loaded.Creatures[creature.ID] = creature

	runtime := state.NewWorld(loaded)

	// Mock attackRoll
	previous := attackRoll
	attackRoll = func(min, max int) int {
		return min // return minimum rolls for predictable outcome
	}
	defer func() { attackRoll = previous }()

	ctx := &Context{ActorID: "player:alice", SessionID: "session-alice"}
	success, err := magicEffectMend(ctx, runtime, creature, model.ObjectInstance{}, ResolvedCommand{Args: []string{"회복", "자신"}})
	if err != nil {
		t.Fatalf("magicEffectMend error: %v", err)
	}
	if !success {
		t.Fatalf("magicEffectMend failed, want success")
	}

	updated, _ := runtime.Creature("creature:alice")
	if updated.Stats["hpCurrent"] <= 10 {
		t.Errorf("hpCurrent = %d, want > 10", updated.Stats["hpCurrent"])
	}

	out := ctx.OutputString()
	if !strings.Contains(out, "기공팔식의 자세를 취하며 원기회복의 주문을 외웁니다.") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestMagicEffectMendTarget(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:church",
		DisplayName: "성당",
		PlayerIDs:   []model.PlayerID{"player:alice", "player:bob"},
	})

	// Caster setup
	alicePlayer := loaded.Players["player:alice"]
	alicePlayer.RoomID = "room:church"
	loaded.Players[alicePlayer.ID] = alicePlayer

	aliceCreature := loaded.Creatures["creature:alice"]
	aliceCreature.RoomID = "room:church"
	aliceCreature.Level = 8
	aliceCreature.Metadata.Tags = []string{"SMENDW"}
	aliceCreature.Stats = map[string]int{
		"class":        model.ClassCleric,
		"level":        8,
		"intelligence": 15,
		"piety":        15,
		"hpCurrent":    30,
		"hpMax":        50,
		"mpCurrent":    20,
		"mpMax":        20,
		"experience":   100,
	}
	loaded.Creatures[aliceCreature.ID] = aliceCreature

	// Target setup
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:church",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:church",
		Stats: map[string]int{
			"hpCurrent": 5,
			"hpMax":     100,
		},
	})

	runtime := state.NewWorld(loaded)

	// Mock attackRoll to make 1/3 experience chance succeed (e.g. returns 1)
	previous := attackRoll
	attackRoll = func(min, max int) int {
		return min
	}
	defer func() { attackRoll = previous }()

	ctx := &Context{ActorID: "player:alice", SessionID: "session-alice"}
	// Setup session mock values for sendToPlayer
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
			}
		},
		"game.sendToSession": func(id string, cmd struct{ Write string }) error {
			if id == "session-bob" {
				if !strings.Contains(cmd.Write, "내공을 주입하며 원기회복의 주문을 겁니다.") {
					t.Errorf("unexpected target message: %q", cmd.Write)
				}
			}
			return nil
		},
	}

	success, err := magicEffectMend(ctx, runtime, aliceCreature, model.ObjectInstance{}, ResolvedCommand{Args: []string{"회복", "Bob"}})
	if err != nil {
		t.Fatalf("magicEffectMend error: %v", err)
	}
	if !success {
		t.Fatalf("magicEffectMend failed")
	}

	bob, _ := runtime.Creature("creature:bob")
	if bob.Stats["hpCurrent"] <= 5 {
		t.Errorf("bob hpCurrent = %d, want > 5", bob.Stats["hpCurrent"])
	}

	alice, _ := runtime.Creature("creature:alice")
	if alice.Stats["experience"] <= 100 {
		t.Errorf("alice experience = %d, want > 100", alice.Stats["experience"])
	}
}

func TestMagicEffectMendPotionRejectsMissingTargetBeforeLookup(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	alice := loaded.Creatures["creature:alice"]
	runtime := state.NewWorld(loaded)
	potion := model.ObjectInstance{ID: "object:mend-potion", Properties: map[string]string{"type": "6"}}
	ctx := &Context{ActorID: "player:alice"}

	success, err := magicEffectMend(ctx, runtime, alice, potion, ResolvedCommand{Args: []string{"원기회복", "Nobody"}})
	if err != nil {
		t.Fatalf("magicEffectMend error = %v", err)
	}
	if success {
		t.Fatalf("success = true, want false")
	}
	if got, want := ctx.OutputString(), "그 물건은 자신에게만 사용할 수 있습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestMagicEffectRoomVigor(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:church",
		DisplayName: "성당",
		PlayerIDs:   []model.PlayerID{"player:alice", "player:bob"},
	})

	// Caster setup
	alicePlayer := loaded.Players["player:alice"]
	alicePlayer.RoomID = "room:church"
	loaded.Players[alicePlayer.ID] = alicePlayer

	aliceCreature := loaded.Creatures["creature:alice"]
	aliceCreature.RoomID = "room:church"
	aliceCreature.Level = 30
	aliceCreature.Metadata.Tags = []string{"SRVIGO"}
	aliceCreature.Stats = map[string]int{
		"class":        model.ClassCleric,
		"level":        30,
		"intelligence": 30,
		"piety":        30,
		"hpCurrent":    5,
		"hpMax":        20,
		"mpCurrent":    20,
		"mpMax":        20,
	}
	loaded.Creatures[aliceCreature.ID] = aliceCreature

	// Target setup
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:church",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:church",
		Stats: map[string]int{
			"hpCurrent": 5,
			"hpMax":     30,
		},
	})

	runtime := state.NewWorld(loaded)

	previous := attackRoll
	attackRoll = func(min, max int) int {
		return max
	}
	defer func() { attackRoll = previous }()

	ctx := &Context{ActorID: "player:alice", SessionID: "session-alice"}
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
			}
		},
		"game.sendToSession": func(id string, cmd struct{ Write string }) error {
			if id == "session-bob" {
				if !strings.Contains(cmd.Write, "당신의 몸에서도 회복의 기운이 솟아오름을 느낄 수 있습니다.") {
					t.Errorf("unexpected message: %q", cmd.Write)
				}
			}
			return nil
		},
	}

	success, err := magicEffectRoomVigor(ctx, runtime, aliceCreature, model.ObjectInstance{}, ResolvedCommand{Args: []string{"전회복"}})
	if err != nil {
		t.Fatalf("magicEffectRoomVigor error: %v", err)
	}
	if !success {
		t.Fatalf("magicEffectRoomVigor failed. Output: %q", ctx.OutputString())
	}

	alice, _ := runtime.Creature("creature:alice")
	if alice.Stats["hpCurrent"] <= 5 {
		t.Errorf("alice hpCurrent = %d, want > 5", alice.Stats["hpCurrent"])
	}

	bob, _ := runtime.Creature("creature:bob")
	if bob.Stats["hpCurrent"] <= 5 {
		t.Errorf("bob hpCurrent = %d, want > 5", bob.Stats["hpCurrent"])
	}
}

func TestMagicEffectRoomVigorLegacyRestrictions(t *testing.T) {
	tests := []struct {
		name   string
		object model.ObjectInstance
		config func(*model.Creature)
		want   string
	}{
		{
			name:   "potion fails first",
			object: model.ObjectInstance{ID: "object:potion", Properties: map[string]string{"type": "6"}},
			config: func(alice *model.Creature) {},
			want:   "주문이 실패했습니다.",
		},
		{
			name:   "wand still requires learned spell",
			object: model.ObjectInstance{ID: "object:wand", Properties: map[string]string{"type": "8"}},
			config: func(alice *model.Creature) {
				alice.Stats = map[string]int{"class": model.ClassCleric, "mpCurrent": 20}
			},
			want: "당신은 아직 그런 주문을 터득하지 못했습니다.",
		},
		{
			name:   "scroll still requires cleric class",
			object: model.ObjectInstance{ID: "object:scroll", Properties: map[string]string{"type": "7"}},
			config: func(alice *model.Creature) {
				alice.Metadata.Tags = []string{"SRVIGO"}
				alice.Stats = map[string]int{"class": model.ClassMage, "mpCurrent": 20}
			},
			want: "이 주술은 불제자만이 사용할 수 있습니다.",
		},
		{
			name:   "invincible requires cleric training",
			object: model.ObjectInstance{ID: "object:wand", Properties: map[string]string{"type": "8"}},
			config: func(alice *model.Creature) {
				alice.Metadata.Tags = []string{"SRVIGO"}
				alice.Stats = map[string]int{"class": model.ClassInvincible, "mpCurrent": 20}
			},
			want: "\n불제자를 무적수련하지 않았습니다..\n",
		},
		{
			name:   "wand still requires mp",
			object: model.ObjectInstance{ID: "object:wand", Properties: map[string]string{"type": "8"}},
			config: func(alice *model.Creature) {
				alice.Metadata.Tags = []string{"SRVIGO"}
				alice.Stats = map[string]int{"class": model.ClassCleric, "mpCurrent": 11}
			},
			want: "당신의 도력이 부족합니다",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := emptyInventoryWorld(t)
			mustAddLookRoom(t, loaded, model.Room{ID: "room:church", DisplayName: "성당"})
			alice := loaded.Creatures["creature:alice"]
			alice.RoomID = "room:church"
			tt.config(&alice)
			loaded.Creatures[alice.ID] = alice

			runtime := state.NewWorld(loaded)
			ctx := &Context{ActorID: "player:alice"}
			success, err := magicEffectRoomVigor(ctx, runtime, alice, tt.object, ResolvedCommand{})
			if err != nil {
				t.Fatalf("magicEffectRoomVigor error = %v", err)
			}
			if success {
				t.Fatalf("success = true, want false")
			}
			if got := ctx.OutputString(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMagicEffectRestore(t *testing.T) {
	t.Run("success case", func(t *testing.T) {
		loaded := emptyInventoryWorld(t)
		mustAddLookRoom(t, loaded, model.Room{ID: "room:church", DisplayName: "성당"})

		player := loaded.Players["player:alice"]
		player.RoomID = "room:church"
		loaded.Players[player.ID] = player

		creature := loaded.Creatures["creature:alice"]
		creature.RoomID = "room:church"
		creature.Stats = map[string]int{
			"class":     model.ClassCaretaker,
			"hpCurrent": 5,
			"hpMax":     20,
			"mpCurrent": 5,
			"mpMax":     20,
		}
		loaded.Creatures[creature.ID] = creature

		runtime := state.NewWorld(loaded)

		previous := attackRoll
		attackRoll = func(min, max int) int {
			// Mock so restore MP succeeds (e.g. roll <= 60)
			if min == 1 && max == 100 {
				return 50
			}
			return min
		}
		defer func() { attackRoll = previous }()

		ctx := &Context{ActorID: "player:alice", SessionID: "session-alice"}
		success, err := magicEffectRestore(ctx, runtime, creature, model.ObjectInstance{}, ResolvedCommand{Args: []string{"도주천"}})
		if err != nil {
			t.Fatalf("magicEffectRestore error: %v", err)
		}
		if !success {
			t.Fatalf("magicEffectRestore failed")
		}

		updated, _ := runtime.Creature("creature:alice")
		if updated.Stats["hpCurrent"] <= 5 {
			t.Errorf("hp = %d, want > 5", updated.Stats["hpCurrent"])
		}
		if updated.Stats["mpCurrent"] != 20 {
			t.Errorf("mp = %d, want 20", updated.Stats["mpCurrent"])
		}

		if !strings.Contains(ctx.OutputString(), "몸에 스며들어와 도력을 회복시킵니다.") {
			t.Errorf("unexpected success output: %q", ctx.OutputString())
		}
	})

	t.Run("failure case", func(t *testing.T) {
		loaded := emptyInventoryWorld(t)
		mustAddLookRoom(t, loaded, model.Room{ID: "room:church", DisplayName: "성당"})

		player := loaded.Players["player:alice"]
		player.RoomID = "room:church"
		loaded.Players[player.ID] = player

		creature := loaded.Creatures["creature:alice"]
		creature.RoomID = "room:church"
		creature.Stats = map[string]int{
			"class":     model.ClassCaretaker,
			"hpCurrent": 5,
			"hpMax":     20,
			"mpCurrent": 5,
			"mpMax":     20,
		}
		loaded.Creatures[creature.ID] = creature

		runtime := state.NewWorld(loaded)

		previous := attackRoll
		attackRoll = func(min, max int) int {
			// Mock so restore MP fails (e.g. roll > 60)
			if min == 1 && max == 100 {
				return 70
			}
			return min
		}
		defer func() { attackRoll = previous }()

		ctx := &Context{ActorID: "player:alice", SessionID: "session-alice"}
		success, err := magicEffectRestore(ctx, runtime, creature, model.ObjectInstance{}, ResolvedCommand{Args: []string{"도주천"}})
		if err != nil {
			t.Fatalf("magicEffectRestore error: %v", err)
		}
		if !success {
			t.Fatalf("magicEffectRestore failed")
		}

		updated, _ := runtime.Creature("creature:alice")
		if updated.Stats["hpCurrent"] <= 5 {
			t.Errorf("hp = %d, want > 5", updated.Stats["hpCurrent"])
		}
		if updated.Stats["mpCurrent"] != 5 {
			t.Errorf("mp = %d, want unchanged 5", updated.Stats["mpCurrent"])
		}

		if !strings.Contains(ctx.OutputString(), "하지만 아무런 반응도 일어나지 않습니다.") {
			t.Errorf("unexpected failure output: %q", ctx.OutputString())
		}
	})
}

func TestMagicEffectRestoreSelfRejectsLegacyNoOpStates(t *testing.T) {
	tests := []struct {
		name  string
		stats map[string]int
		want  string
	}{
		{
			name: "mp already max",
			stats: map[string]int{
				"class":     model.ClassCaretaker,
				"hpCurrent": 5,
				"hpMax":     20,
				"mpCurrent": 20,
				"mpMax":     20,
			},
			want: "\n당신은 도주천 주술이 필요없습니다.\n",
		},
		{
			name: "invincible self cast",
			stats: map[string]int{
				"class":     model.ClassInvincible,
				"hpCurrent": 5,
				"hpMax":     20,
				"mpCurrent": 5,
				"mpMax":     20,
			},
			want: "\n자신에게 외울수 없습니다.\n",
		},
		{
			name: "low class cast",
			stats: map[string]int{
				"class":     model.ClassFighter,
				"hpCurrent": 5,
				"hpMax":     20,
				"mpCurrent": 5,
				"mpMax":     20,
			},
			want: "\n당신은 그 주술을 사용할 능력이 없습니다.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := emptyInventoryWorld(t)
			mustAddLookRoom(t, loaded, model.Room{ID: "room:church", DisplayName: "성당"})
			player := loaded.Players["player:alice"]
			player.RoomID = "room:church"
			loaded.Players[player.ID] = player
			creature := loaded.Creatures["creature:alice"]
			creature.RoomID = "room:church"
			creature.Stats = tt.stats
			loaded.Creatures[creature.ID] = creature
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			success, err := magicEffectRestore(ctx, runtime, creature, model.ObjectInstance{}, ResolvedCommand{Args: []string{"도주천"}})
			if err != nil {
				t.Fatalf("magicEffectRestore error = %v", err)
			}
			if success {
				t.Fatalf("success = true, want false")
			}
			if got := ctx.OutputString(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
			updated, _ := runtime.Creature("creature:alice")
			if got := updated.Stats["hpCurrent"]; got != 5 {
				t.Fatalf("hpCurrent = %d, want unchanged 5", got)
			}
		})
	}
}

func TestCastRestoreDoesNotDeductGenericMPCostLikeLegacy(t *testing.T) {
	loaded := castWorld(t, "room:dojo", 30)
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "SRESTO")
	creature.Stats["class"] = model.ClassCaretaker
	creature.Stats["hpCurrent"] = 5
	creature.Stats["mpMax"] = 40
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)

	previous := attackRoll
	attackRoll = func(min, max int) int {
		if min == 1 && max == 100 {
			return 50
		}
		return min
	}
	defer func() { attackRoll = previous }()

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{"도주천"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "도력을 회복시킵니다") {
		t.Fatalf("status/output = %d/%q, want restore success", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 40 {
		t.Fatalf("mpCurrent = %d, want restored max MP without generic cost deduction", got)
	}
}

func TestMagicEffectRestorePotionRejectsMissingTargetBeforeLookup(t *testing.T) {
	loaded := emptyInventoryWorld(t)
	alice := loaded.Creatures["creature:alice"]
	runtime := state.NewWorld(loaded)
	potion := model.ObjectInstance{ID: "object:restore-potion", Properties: map[string]string{"type": "6"}}
	ctx := &Context{ActorID: "player:alice"}

	success, err := magicEffectRestore(ctx, runtime, alice, potion, ResolvedCommand{Args: []string{"도주천", "Nobody"}})
	if err != nil {
		t.Fatalf("magicEffectRestore error = %v", err)
	}
	if success {
		t.Fatalf("success = true, want false")
	}
	if got, want := ctx.OutputString(), "\n그 물건은 자신에게만 사용할수 있습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
