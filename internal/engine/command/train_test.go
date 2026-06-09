package command

import (
	"testing"

	"muhan/internal/commandspec"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestTrainHandlerRejectsRoomAndCostRequirements(t *testing.T) {
	tests := []struct {
		name     string
		roomTags []string
		exp      int
		gold     int
		want     string
	}{
		{
			name: "not training room",
			exp:  128,
			gold: 6,
			want: "이 곳은 수련할 수 있는곳이 아닙니다!",
		},
		{
			name:     "wrong class training room",
			roomTags: []string{"train"},
			exp:      128,
			gold:     6,
			want:     "당신이 수련하는곳은 여기가 아닙니다.",
		},
		{
			name:     "not enough experience",
			roomTags: []string{"train", "trainingBit5", "trainingBit6"},
			exp:      100,
			gold:     6,
			want:     "당신은 28 만큼의 경험치가 더 필요합니다.",
		},
		{
			name:     "not enough gold",
			roomTags: []string{"train", "trainingBit5", "trainingBit6"},
			exp:      128,
			gold:     5,
			want:     "돈도 없으면서... 돈 벌어오세요.\n당신은 수련하는데 6냥이 듭니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := state.NewWorld(trainWorld(t, tt.roomTags, legacyClassFighter, 1, tt.exp, tt.gold))

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewTrainHandler(runtime)(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}

			creature, _ := runtime.Creature("creature:alice")
			if creature.Level != 1 || creature.Stats["level"] != 1 || creature.Stats["gold"] != tt.gold {
				t.Fatalf("creature mutated on reject: level=%d statLevel=%d gold=%d", creature.Level, creature.Stats["level"], creature.Stats["gold"])
			}
		})
	}
}

func TestTrainHandlerLevelsActorAndChargesGold(t *testing.T) {
	runtime := state.NewWorld(trainWorld(t, []string{"train", "trainingBit5", "trainingBit6"}, legacyClassFighter, 1, 128, 6))
	var broadcasts []string

	ctx := trainBroadcastContext(&broadcasts)
	status, err := NewTrainHandler(runtime)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n축하합니다! 당신의 레벨이 올랐습니다!" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0] != "\n### Alice님이 2레벨로 올랐습니다!" {
		t.Fatalf("broadcasts = %+v, want C single-level broadcast", broadcasts)
	}

	creature, ok := runtime.Creature("creature:alice")
	if !ok {
		t.Fatal("missing creature")
	}
	if creature.Level != 2 || creature.Stats["level"] != 2 {
		t.Fatalf("level/stat level = %d/%d, want 2/2", creature.Level, creature.Stats["level"])
	}
	if creature.Stats["gold"] != 0 {
		t.Fatalf("gold = %d, want 0", creature.Stats["gold"])
	}
	if creature.Stats["experience"] != 128 {
		t.Fatalf("experience = %d, want unchanged 128", creature.Stats["experience"])
	}
}

func TestTrainHandlerAppliesLegacyMultiLevelLoop(t *testing.T) {
	runtime := state.NewWorld(trainWorld(t, []string{"train", "trainingBit5", "trainingBit6"}, legacyClassFighter, 1, 384, 18))
	var broadcasts []string

	ctx := trainBroadcastContext(&broadcasts)
	status, err := NewTrainHandler(runtime)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n축하합니다! 당신의 레벨이 올랐습니다!" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0] != "\n### Alice님이 3레벨로 2단계 올랐습니다!" {
		t.Fatalf("broadcasts = %+v, want C multi-level broadcast", broadcasts)
	}
	creature, _ := runtime.Creature("creature:alice")
	if creature.Level != 3 || creature.Stats["level"] != 3 {
		t.Fatalf("level/stat level = %d/%d, want 3/3", creature.Level, creature.Stats["level"])
	}
	if creature.Stats["gold"] != 0 {
		t.Fatalf("gold = %d, want 0 after two C training charges", creature.Stats["gold"])
	}
	if creature.Stats["experience"] != 384 {
		t.Fatalf("experience = %d, want unchanged 384", creature.Stats["experience"])
	}
}

func TestTrainHandlerClearsUpDamageBeforeRoomCheckLikeLegacy(t *testing.T) {
	loaded := trainWorld(t, nil, legacyClassBarbarian, 50, 0, 0)
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"PUPDMG", "upDamage"}
	creature.Stats["pDice"] = 5
	creature.Stats["hpMax"] = 100
	creature.Stats["mpMax"] = 70
	creature.Stats["hpCurrent"] = 25
	creature.Stats["mpCurrent"] = 10
	loaded.Creatures[creature.ID] = creature
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"PUPDMG", "upDamage"}
	loaded.Players[player.ID] = player
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewTrainHandler(runtime)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "이 곳은 수련할 수 있는곳이 아닙니다!" {
		t.Fatalf("status/output = %d/%q, want C room rejection after up-damage clear", status, ctx.OutputString())
	}
	creature, _ = runtime.Creature("creature:alice")
	for key, want := range map[string]int{"pDice": 3, "hpMax": 50, "mpMax": 50, "hpCurrent": 50, "mpCurrent": 50} {
		if got := creature.Stats[key]; got != want {
			t.Fatalf("%s = %d, want %d after C PUPDMG clear", key, got, want)
		}
	}
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "PUPDMG", "upDamage", "upDmg") {
		t.Fatalf("creature tags = %+v, want PUPDMG cleared", creature.Metadata.Tags)
	}
	player, _ = runtime.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "PUPDMG", "upDamage", "upDmg") {
		t.Fatalf("player tags = %+v, want PUPDMG cleared", player.Metadata.Tags)
	}
}

func TestTrainHandlerRejectsInvincibleCaretakerTransitionWithoutAllTrainings(t *testing.T) {
	runtime := state.NewWorld(trainWorld(t, []string{"train"}, legacyClassInvincible, legacyMaxAutoLevel-1, 100000000, 5000000))

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewTrainHandler(runtime)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	want := "\n당신은 자객, 권법가, 불제자, 검사, 도술사, 무사, 포졸, 도둑 직업을 무적수련하지 않았습니다."
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
	creature, _ := runtime.Creature("creature:alice")
	if creature.Stats["class"] != legacyClassInvincible || creature.Level != legacyMaxAutoLevel-1 || creature.Stats["gold"] != 5000000 {
		t.Fatalf("creature mutated on reject: class=%d level=%d gold=%d", creature.Stats["class"], creature.Level, creature.Stats["gold"])
	}
}

func TestTrainHandlerTransitionsInvincibleToCaretakerLikeLegacy(t *testing.T) {
	loaded := trainWorld(t, []string{"train"}, legacyClassInvincible, legacyMaxAutoLevel-1, 100000000, 5000000)
	creature := loaded.Creatures["creature:alice"]
	for _, training := range invinceTrainingSpecs {
		creature.Metadata.Tags = append(creature.Metadata.Tags, training.tag)
	}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)
	var broadcasts []string

	ctx := trainBroadcastContext(&broadcasts)
	status, err := NewTrainHandler(runtime)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n축하합니다! 당신은 초인이 되었습니다!!" {
		t.Fatalf("status/output = %d/%q, want caretaker transition", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0] != "\n### Alice님께서 초인이 되셨습니다!!" {
		t.Fatalf("broadcasts = %+v, want C caretaker transition broadcast", broadcasts)
	}
	creature, _ = runtime.Creature("creature:alice")
	for key, want := range map[string]int{
		"class":     legacyClassCaretaker,
		"level":     legacyMaxAutoLevel - 1,
		"gold":      0,
		"hpMax":     800,
		"hpCurrent": 800,
		"mpMax":     600,
		"mpCurrent": 600,
		"nDice":     4,
		"sDice":     3,
		"pDice":     4,
	} {
		if got := creature.Stats[key]; got != want {
			t.Fatalf("%s = %d, want %d", key, got, want)
		}
	}
	if creature.Level != legacyMaxAutoLevel-1 {
		t.Fatalf("level field = %d, want %d", creature.Level, legacyMaxAutoLevel-1)
	}
}

func TestTrainHandlerTransitionsCaretakerToBulsaLikeLegacy(t *testing.T) {
	loaded := trainWorld(t, []string{"train"}, legacyClassCaretaker, legacyMaxAutoLevel-1, 100000000, 5000000)
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"TRAINBUL"}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)
	var broadcasts []string

	ctx := trainBroadcastContext(&broadcasts)
	status, err := NewTrainHandler(runtime)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n축하합니다! 당신은 불사가 되었습니다!!" {
		t.Fatalf("status/output = %d/%q, want bulsa transition", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0] != "\n### Alice님께서 불사가 되셨습니다!!" {
		t.Fatalf("broadcasts = %+v, want C bulsa transition broadcast", broadcasts)
	}
	creature, _ = runtime.Creature("creature:alice")
	for key, want := range map[string]int{
		"class":     legacyClassBulsa,
		"level":     legacyMaxAutoLevel - 1,
		"gold":      0,
		"hpMax":     3500,
		"hpCurrent": 3500,
		"mpMax":     2500,
		"mpCurrent": 2500,
		"nDice":     5,
		"sDice":     5,
		"pDice":     5,
	} {
		if got := creature.Stats[key]; got != want {
			t.Fatalf("%s = %d, want %d", key, got, want)
		}
	}
	if creature.Level != legacyMaxAutoLevel-1 {
		t.Fatalf("level field = %d, want %d", creature.Level, legacyMaxAutoLevel-1)
	}
}

func TestTrainDispatcherAliases(t *testing.T) {
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "수련", Number: 46, Handler: "train"},
		{Name: "train", Number: 46, Handler: "train"},
	})

	for _, line := range []string{"수련", "train"} {
		t.Run(line, func(t *testing.T) {
			runtime := state.NewWorld(trainWorld(t, []string{"train", "trainingBit5", "trainingBit6"}, legacyClassFighter, 1, 128, 6))
			dispatcher := Dispatcher{
				Registry: registry,
				Handlers: map[string]Handler{
					"train": NewTrainHandler(runtime),
				},
			}

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, line)
			if err != nil {
				t.Fatalf("DispatchLine(%q) error = %v", line, err)
			}
			if status != StatusDefault || ctx.OutputString() != "\n축하합니다! 당신의 레벨이 올랐습니다!" {
				t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
			}
			creature, _ := runtime.Creature("creature:alice")
			if creature.Level != 2 {
				t.Fatalf("level after %q = %d, want 2", line, creature.Level)
			}
		})
	}
}

func trainBroadcastContext(broadcasts *[]string) *Context {
	return &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.broadcast": func(cmd struct{ Write string }) error {
				*broadcasts = append(*broadcasts, cmd.Write)
				return nil
			},
		},
	}
}

func trainWorld(t *testing.T, roomTags []string, class int, level int, experience int, gold int) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:dojo",
		DisplayName: "수련장",
		Metadata:    model.Metadata{Tags: roomTags},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:dojo",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:dojo",
		Level:       level,
		Stats: map[string]int{
			"class":      class,
			"level":      level,
			"experience": experience,
			"gold":       gold,
			"hpCurrent":  10,
			"hpMax":      10,
			"mpCurrent":  10,
			"mpMax":      10,
		},
	})
	return loaded
}
