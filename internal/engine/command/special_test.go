package command

import (
	"fmt"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

// mustAddLookRoom, mustAddLookPrototype, mustAddLookObject, mustAddLookPlayer, mustAddLookCreature, mustRegistry, useDispatcher 등은
// command 패키지 내의 다른 테스트 파일(use_test.go, equip_test.go 등)에 정의되어 있으므로 패키지 레벨에서 바로 공유됩니다.

func TestSpecialComboScenario(t *testing.T) {
	previous := comboDamageRoll
	comboDamageRoll = func(min, max int) int { return 30 }
	t.Cleanup(func() { comboDamageRoll = previous })

	t.Run("correct special click opens exit", func(t *testing.T) {
		loaded := specialComboWorld(t, 50, "1", "1")
		runtime := state.NewWorld(loaded)
		ctx := &Context{ActorID: "player:alice"}

		status, err := NewSpecialHandler(runtime, "")(ctx, legacySpecialCombo, ResolvedCommand{Args: []string{"조합"}})
		if err != nil {
			t.Fatalf("special handler error = %v", err)
		}
		want := "Click.\n당신은 서를 열었습니다!\n"
		if status != StatusDefault || ctx.OutputString() != want {
			t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
		}
		exit := mustRuntimeExit(t, runtime, "room:plaza", "서")
		if exitHasAnyFlag(exit, "locked", "closed", "unlocked") {
			t.Fatalf("exit flags = %+v, want C open state without Go-only unlocked flag", exit.Flags)
		}
	})

	t.Run("sdice is converted to raw legacy digit byte", func(t *testing.T) {
		loaded := specialComboWorld(t, 50, "12", "<")
		runtime := state.NewWorld(loaded)
		ctx := &Context{ActorID: "player:alice"}

		status, err := NewSpecialHandler(runtime, "")(ctx, legacySpecialCombo, ResolvedCommand{Args: []string{"조합"}})
		if err != nil {
			t.Fatalf("special handler error = %v", err)
		}
		if status != StatusDefault || ctx.OutputString() != "Click.\n당신은 서를 열었습니다!\n" {
			t.Fatalf("status/output = %d/%q, want legacy sdice+'0' combo success", status, ctx.OutputString())
		}
	})

	t.Run("explicit digits are ignored like legacy combo_box", func(t *testing.T) {
		loaded := specialComboWorld(t, 50, "1", "11")
		runtime := state.NewWorld(loaded)
		ctx := &Context{ActorID: "player:alice"}

		status, err := NewSpecialHandler(runtime, "")(ctx, legacySpecialCombo, ResolvedCommand{
			Args:  []string{"조합", "11"},
			Input: "조합 11 눌러",
		})
		if err != nil {
			t.Fatalf("special handler error = %v", err)
		}
		if status != StatusDefault || ctx.OutputString() != "Click.\n" {
			t.Fatalf("status/output = %d/%q, want one legacy click", status, ctx.OutputString())
		}
		exit := mustRuntimeExit(t, runtime, "room:plaza", "서")
		if !exitHasAnyFlag(exit, "locked", "closed") {
			t.Fatalf("exit flags = %+v, want still locked after one click", exit.Flags)
		}
	})

	t.Run("combo sequence is connection local", func(t *testing.T) {
		loaded := specialComboWorld(t, 50, "1", "11")
		combo := loaded.Objects["object:combo-test"]
		combo.Properties["nDice"] = "0"
		loaded.Objects[combo.ID] = combo
		runtime := state.NewWorld(loaded)
		handler := NewSpecialHandler(runtime, "")

		ctx1 := &Context{SessionID: "s1", ActorID: "player:alice"}
		if status, err := handler(ctx1, legacySpecialCombo, ResolvedCommand{Args: []string{"조합"}}); err != nil {
			t.Fatalf("s1 first click error = %v", err)
		} else if status != StatusDefault || ctx1.OutputString() != "Click.\n" {
			t.Fatalf("s1 first status/output = %d/%q, want one click", status, ctx1.OutputString())
		}
		ctx2 := &Context{SessionID: "s2", ActorID: "player:alice"}
		if status, err := handler(ctx2, legacySpecialCombo, ResolvedCommand{Args: []string{"조합"}}); err != nil {
			t.Fatalf("s2 first click error = %v", err)
		} else if status != StatusDefault || ctx2.OutputString() != "Click.\n" {
			t.Fatalf("s2 first status/output = %d/%q, want separate one click", status, ctx2.OutputString())
		}
		exit := mustRuntimeExit(t, runtime, "room:plaza", "서")
		if !exitHasAnyFlag(exit, "locked", "closed") {
			t.Fatalf("exit flags = %+v, want locked before s1 second click", exit.Flags)
		}

		ctx1.Output = nil
		if status, err := handler(ctx1, legacySpecialCombo, ResolvedCommand{Args: []string{"조합"}}); err != nil {
			t.Fatalf("s1 second click error = %v", err)
		} else if status != StatusDefault || ctx1.OutputString() != "Click.\n당신은 서를 열었습니다!\n" {
			t.Fatalf("s1 second status/output = %d/%q, want unlock", status, ctx1.OutputString())
		}
		creature, _ := runtime.Creature("creature:alice")
		if _, ok := creature.Properties["legacyComboSequence"]; ok {
			t.Fatal("combo sequence leaked into creature properties")
		}
	})

	t.Run("wrong special click damages and can kill", func(t *testing.T) {
		loaded := specialComboWorld(t, 20, "5", "1")
		creature := loaded.Creatures["creature:alice"]
		if creature.Equipment == nil {
			creature.Equipment = map[string]model.ObjectInstanceID{}
		}
		creature.Equipment["body"] = "object:training-armor"
		loaded.Creatures[creature.ID] = creature
		mustAddLookPrototype(t, loaded, model.ObjectPrototype{
			ID:          "prototype:training-armor",
			Kind:        model.ObjectKindArmor,
			DisplayName: "수련 갑옷",
		})
		mustAddLookObject(t, loaded, model.ObjectInstance{
			ID:          "object:training-armor",
			PrototypeID: "prototype:training-armor",
			Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "body"},
		})
		runtime := state.NewWorld(loaded)
		runtime.SetDBRoot(t.TempDir())
		ctx := &Context{ActorID: "player:alice"}

		status, err := NewSpecialHandler(runtime, "")(ctx, legacySpecialCombo, ResolvedCommand{Args: []string{"조합"}})
		if err != nil {
			t.Fatalf("special handler error = %v", err)
		}
		want := "Click.\n당신은 20점의 피해를 입었습니다!\n당신은 죽었습니다.\n"
		if status != StatusDefault || ctx.OutputString() != want {
			t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
		}
		creature, _ = runtime.Creature("creature:alice")
		if got := creature.Stats["hpCurrent"]; got != 20 {
			t.Fatalf("hpCurrent = %d, want respawned hpMax 20", got)
		}
		armor, ok := runtime.Object("object:training-armor")
		if !ok {
			t.Fatal("training armor missing after combo self-death")
		}
		if armor.Location.RoomID != "room:plaza" {
			t.Fatalf("training armor location = %+v, want room drop from C die(ply_ptr, ply_ptr)", armor.Location)
		}
	})
}

func TestSpecialComboEmptyCombinationDamagesLikeLegacyComboBox(t *testing.T) {
	previous := comboDamageRoll
	comboDamageRoll = func(min, max int) int { return 20 }
	t.Cleanup(func() { comboDamageRoll = previous })

	loaded := specialComboWorld(t, 50, "1", "")
	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	status, err := NewSpecialHandler(runtime, "")(ctx, legacySpecialCombo, ResolvedCommand{Args: []string{"조합"}})
	if err != nil {
		t.Fatalf("special handler error = %v", err)
	}
	want := "Click.\n당신은 20점의 피해를 입었습니다!\n"
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
	creature, _ := runtime.Creature("creature:alice")
	if got := creature.Stats["hpCurrent"]; got != 30 {
		t.Fatalf("hpCurrent = %d, want 30 after legacy empty-combination damage", got)
	}
}

func TestSpecialCommandUnsupportedSpecialUsesLegacyDefaultBeforeTargetParsing(t *testing.T) {
	loaded := useWorld(t)
	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	status, err := NewSpecialHandler(runtime, "")(ctx, legacySpecialWar, ResolvedCommand{})
	if err != nil {
		t.Fatalf("special handler error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "아무런 일도 일어나지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C special_cmd default", status, ctx.OutputString())
	}
}

func TestSpecialCommandMissingTargetAlwaysUsesLegacyPressPrompt(t *testing.T) {
	loaded := useWorld(t)
	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	status, err := NewSpecialHandler(runtime, "")(ctx, legacySpecialCombo, ResolvedCommand{
		Input:  "밀어",
		Parsed: commandparse.Parse("밀어"),
	})
	if err != nil {
		t.Fatalf("special handler error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "무얼 누릅니까?\n" {
		t.Fatalf("status/output = %d/%q, want legacy SP_COMBO missing-target prompt", status, ctx.OutputString())
	}
}

func TestSpecialCommandReadsRoomSpecialMapScrollLikeLegacy(t *testing.T) {
	root := t.TempDir()
	writeSpecialMapScrollFixture(t, root, "벽_지도", "방 한가운데에 오래된 표식이 있다.")
	loaded := useWorld(t)
	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:room-map")
	loaded.Rooms[room.ID] = room
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:room-map",
		Kind:        model.ObjectKindMisc,
		DisplayName: "벽 지도",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:room-map",
		PrototypeID: "prototype:room-map",
		Location:    model.ObjectLocation{RoomID: "room:plaza"},
		Properties:  map[string]string{"special": "SP_MAPSC"},
	})
	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	status, err := NewSpecialHandler(runtime, root)(ctx, legacySpecialMapScroll, ResolvedCommand{Args: []string{"벽"}})
	if err != nil {
		t.Fatalf("special handler error = %v", err)
	}
	want := "방 한가운데에 오래된 표식이 있다.\n"
	if status != StatusDoPrompt || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
	if _, ok := runtime.Object("object:room-map"); !ok {
		t.Fatal("room special map scroll was consumed")
	}
}

func TestSpecialCommandInventoryMismatchBlocksEquippedAndRoomFallbackLikeLegacy(t *testing.T) {
	loaded := useWorld(t)
	creature := loaded.Creatures["creature:alice"]
	if creature.Equipment == nil {
		creature.Equipment = map[string]model.ObjectInstanceID{}
	}
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:plain-panel")
	creature.Equipment["held"] = "object:equipped-combo"
	loaded.Creatures[creature.ID] = creature

	room := loaded.Rooms["room:plaza"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:room-combo")
	loaded.Rooms[room.ID] = room

	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:plain-panel",
		DisplayName: "조합돌",
		Properties:  map[string]string{"type": "13"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:plain-panel",
		PrototypeID: "prototype:plain-panel",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:combo-test",
		DisplayName: "조합상자",
		Properties:  map[string]string{"type": "13"},
	})
	for _, object := range []model.ObjectInstance{
		{
			ID:          "object:equipped-combo",
			PrototypeID: "prototype:combo-test",
			Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "held"},
		},
		{
			ID:          "object:room-combo",
			PrototypeID: "prototype:combo-test",
			Location:    model.ObjectLocation{RoomID: "room:plaza"},
		},
	} {
		mustAddLookObject(t, loaded, model.ObjectInstance{
			ID:          object.ID,
			PrototypeID: object.PrototypeID,
			Location:    object.Location,
			Properties: map[string]string{
				"special":   "SP_COMBO",
				"pDice":     "1",
				"nDice":     "1",
				"sDice":     "1",
				"useOutput": "1",
			},
		})
	}

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	status, err := NewSpecialHandler(runtime, "")(ctx, legacySpecialCombo, ResolvedCommand{Args: []string{"조합"}})
	if err != nil {
		t.Fatalf("special handler error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "무얼 하려고 하는데요?.\n" {
		t.Fatalf("status/output = %d/%q, want legacy special mismatch", status, ctx.OutputString())
	}
	exit := mustRuntimeExit(t, runtime, "room:plaza", "서")
	if !exitHasAnyFlag(exit, "locked", "closed") {
		t.Fatalf("exit flags = %+v, want fallback combo not activated", exit.Flags)
	}
}

func TestSpecialCommandDoesNotSearchNestedContainersLikeLegacyFindObj(t *testing.T) {
	tests := []struct {
		name        string
		containerID model.ObjectInstanceID
		setup       func(*worldload.World)
	}{
		{
			name:        "inventory container contents",
			containerID: "object:special-bag",
			setup: func(loaded *worldload.World) {
				creature := loaded.Creatures["creature:alice"]
				creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:special-bag")
				loaded.Creatures[creature.ID] = creature

				mustAddLookObject(t, loaded, model.ObjectInstance{
					ID:          "object:special-bag",
					PrototypeID: "prototype:special-bag",
					Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
					Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:nested-combo"}},
				})
			},
		},
		{
			name:        "room container contents",
			containerID: "object:floor-special-bag",
			setup: func(loaded *worldload.World) {
				room := loaded.Rooms["room:plaza"]
				room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:floor-special-bag")
				loaded.Rooms[room.ID] = room

				mustAddLookObject(t, loaded, model.ObjectInstance{
					ID:          "object:floor-special-bag",
					PrototypeID: "prototype:special-bag",
					Location:    model.ObjectLocation{RoomID: "room:plaza"},
					Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:nested-combo"}},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := useWorld(t)
			mustAddLookPrototype(t, loaded, model.ObjectPrototype{
				ID:          "prototype:special-bag",
				Kind:        model.ObjectKindContainer,
				DisplayName: "낡은 상자",
			})
			mustAddLookPrototype(t, loaded, model.ObjectPrototype{
				ID:          "prototype:nested-combo",
				DisplayName: "비밀 조합상자",
				Properties:  map[string]string{"type": "13"},
			})
			tt.setup(loaded)
			mustAddLookObject(t, loaded, model.ObjectInstance{
				ID:          "object:nested-combo",
				PrototypeID: "prototype:nested-combo",
				Location:    model.ObjectLocation{ContainerID: tt.containerID},
				Properties: map[string]string{
					"special":   "SP_COMBO",
					"pDice":     "1",
					"nDice":     "1",
					"sDice":     "1",
					"useOutput": "1",
				},
			})

			runtime := state.NewWorld(loaded)
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewSpecialHandler(runtime, "")(ctx, legacySpecialCombo, ResolvedCommand{Args: []string{"비밀"}})
			if err != nil {
				t.Fatalf("special handler error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != "그같은 물건이 없습니다.\n" {
				t.Fatalf("status/output = %d/%q, want legacy top-level find_obj miss", status, ctx.OutputString())
			}
			exit := mustRuntimeExit(t, runtime, "room:plaza", "서")
			if !exitHasAnyFlag(exit, "locked", "closed") {
				t.Fatalf("exit flags = %+v, want nested combo not activated", exit.Flags)
			}
		})
	}
}

func specialComboWorld(t *testing.T, hp int, sDice string, useOutput string) *worldload.World {
	t.Helper()
	loaded := useWorld(t)
	creature := loaded.Creatures["creature:alice"]
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats["hpCurrent"] = hp
	creature.Stats["hpMax"] = hp
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:combo-test")
	loaded.Creatures[creature.ID] = creature

	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:combo-test",
		DisplayName: "조합상자",
		Properties:  map[string]string{"type": "13"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:combo-test",
		PrototypeID: "prototype:combo-test",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties: map[string]string{
			"special":   "SP_COMBO",
			"pDice":     "1",
			"nDice":     "1",
			"sDice":     sDice,
			"useOutput": useOutput,
		},
	})
	return loaded
}

func TestSpecialMapScrollPageScenario(t *testing.T) {
	// 2. Page Scroll Rendering (Scroll with > 20 lines)
	root := t.TempDir()

	// 25줄의 텍스트 생성
	var sb strings.Builder
	for i := 1; i <= 25; i++ {
		sb.WriteString(fmt.Sprintf("라인 %d\n", i))
	}

	writeSpecialMapScrollFixture(t, root, "고대_지도", sb.String())

	loaded := useWorld(t)
	proto := loaded.ObjectPrototypes["prototype:scroll"]
	proto.DisplayName = "고대 지도"
	loaded.ObjectPrototypes[proto.ID] = proto
	scroll := loaded.Objects["object:scroll"]
	scroll.Properties["special"] = "SP_MAPSC"
	loaded.Objects[scroll.ID] = scroll

	runtime := state.NewWorld(loaded)
	dispatcher := useDispatcherWithRoot(t, runtime, root)

	var pending PendingLineHandler
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			ContextPendingLineKey: func(handler PendingLineHandler) {
				pending = handler
			},
		},
	}

	status, err := dispatcher.DispatchLine(ctx, "고대 사용")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}

	if status != StatusDoPrompt {
		t.Fatalf("status = %d, want StatusDoPrompt", status)
	}

	output := ctx.OutputString()
	if !strings.Contains(output, "라인 19\n") || strings.Contains(output, "라인 20\n") {
		t.Fatalf("output = %q, want first 19 lines rendered", output)
	}
	if !strings.Contains(output, postReadContinuePrompt) {
		t.Fatalf("output = %q, want continue prompt", output)
	}

	if pending == nil {
		t.Fatal("pending line handler not set")
	}

	// 엔터 입력 시뮬레이션
	ctx = &Context{ActorID: "player:alice"}
	status, err = pending(ctx, "")
	if err != nil {
		t.Fatalf("pending handler error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault after completion", status)
	}
	output = ctx.OutputString()
	if !strings.Contains(output, "라인 20\n") || !strings.Contains(output, "라인 25\n") {
		t.Fatalf("output = %q, want remaining lines rendered", output)
	}

	// 중단 시뮬레이션
	ctx = &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			ContextPendingLineKey: func(handler PendingLineHandler) {
				pending = handler
			},
		},
	}
	// 처음부터 다시 읽기
	status, err = dispatcher.DispatchLine(ctx, "고대 사용")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	ctx = &Context{ActorID: "player:alice"}
	status, err = pending(ctx, ".")
	if err != nil {
		t.Fatalf("pending handler error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if !strings.Contains(ctx.OutputString(), "중단합니다.") {
		t.Fatalf("output = %q, want 중단합니다.", ctx.OutputString())
	}
}

func TestOSPECIPotionSideEffectsScenario(t *testing.T) {
	// 3. OSPECI Potion Side Effects (pdice = 1 to 6)

	// Helper to run drink test
	runDrinkTest := func(t *testing.T, pdice int, setup func(l *worldload.World, c *model.Creature, p *model.Player), verify func(t *testing.T, w *state.World, c model.Creature, p model.Player, output string)) {
		loaded := useWorld(t)
		creature := loaded.Creatures["creature:alice"]
		player := loaded.Players["player:alice"]

		if setup != nil {
			setup(loaded, &creature, &player)
		}

		creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:ospeci-potion")
		loaded.Creatures[creature.ID] = creature
		loaded.Players[player.ID] = player

		mustAddLookPrototype(t, loaded, model.ObjectPrototype{
			ID:          "prototype:ospeci-potion",
			Kind:        model.ObjectKindPotion,
			DisplayName: "신비한물약",
		})
		mustAddLookObject(t, loaded, model.ObjectInstance{
			ID:          "object:ospeci-potion",
			PrototypeID: "prototype:ospeci-potion",
			Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
			Properties: map[string]string{
				"specialItem":  "1",
				"pDice":        fmt.Sprintf("%d", pdice),
				"shotsCurrent": "1",
				"magicPower":   "10",
				"useOutput":    "몸에 변화가 느껴집니다.",
				"value":        "2", // pdice = 5 일때 2번 방 이동용
			},
		})

		tempDir := t.TempDir()
		runtime := state.NewWorld(loaded)
		runtime.SetDBRoot(tempDir)
		dispatcher := useDispatcher(t, runtime)

		ctx := &Context{ActorID: "player:alice"}
		_, err := dispatcher.DispatchLine(ctx, "신비한물약 사용")
		if err != nil {
			t.Fatalf("drink error = %v", err)
		}

		updatedCreature, _ := runtime.Creature("creature:alice")
		updatedPlayer, _ := runtime.Player("player:alice")
		verify(t, runtime, updatedCreature, updatedPlayer, ctx.OutputString())
	}

	// pdice = 1: age/interval 감소
	runDrinkTest(t, 1, func(l *worldload.World, c *model.Creature, p *model.Player) {
		if c.Stats == nil {
			c.Stats = map[string]int{}
		}
		c.Stats["legacyHoursInterval"] = 100000
	}, func(t *testing.T, w *state.World, c model.Creature, p model.Player, output string) {
		if val := c.Stats["legacyHoursInterval"]; val != 100000-86400 {
			t.Fatalf("interval = %d, want %d", val, 100000-86400)
		}
	})

	// pdice = 2: 혼돈(PCHAOS) 상태 토글
	runDrinkTest(t, 2, nil, func(t *testing.T, w *state.World, c model.Creature, p model.Player, output string) {
		if !hasAnyNormalizedFlag(c.Metadata.Tags, "chaos", "pchaos", "PCHAOS") {
			t.Fatalf("tags = %+v, want PCHAOS set", c.Metadata.Tags)
		}
	})

	// pdice = 3: age/interval 증가
	runDrinkTest(t, 3, func(l *worldload.World, c *model.Creature, p *model.Player) {
		if c.Stats == nil {
			c.Stats = map[string]int{}
		}
		c.Stats["legacyHoursInterval"] = 100000
	}, func(t *testing.T, w *state.World, c model.Creature, p model.Player, output string) {
		if val := c.Stats["legacyHoursInterval"]; val != 100000+86400 {
			t.Fatalf("interval = %d, want %d", val, 100000+86400)
		}
	})

	// pdice = 4: 스탯 총합이 limit 이하일 때 기본 스탯 1 상승
	runDrinkTest(t, 4, func(l *worldload.World, c *model.Creature, p *model.Player) {
		if c.Stats == nil {
			c.Stats = map[string]int{}
		}
		c.Stats["level"] = 4
		c.Stats["strength"] = 10
		c.Stats["intelligence"] = 10
		c.Stats["piety"] = 10
		c.Stats["constitution"] = 10
		c.Stats["dexterity"] = 10
	}, func(t *testing.T, w *state.World, c model.Creature, p model.Player, output string) {
		if c.Stats["strength"] != 11 || c.Stats["intelligence"] != 11 {
			t.Fatalf("stats strength/intel = %d/%d, want 11/11", c.Stats["strength"], c.Stats["intelligence"])
		}
	})

	// pdice = 5: 다른 룸으로 이동
	runDrinkTest(t, 5, func(l *worldload.World, c *model.Creature, p *model.Player) {
		room := model.Room{ID: "room:00002", DisplayName: "다른 방"}
		l.Rooms[room.ID] = room
		p.RoomID = "room:plaza"
	}, func(t *testing.T, w *state.World, c model.Creature, p model.Player, output string) {
		if p.RoomID != "room:00002" {
			t.Fatalf("player room = %s, want room:00002", p.RoomID)
		}
	})

	// pdice = 6: 독 걸림
	runDrinkTest(t, 6, nil, func(t *testing.T, w *state.World, c model.Creature, p model.Player, output string) {
		if !hasAnyNormalizedFlag(c.Metadata.Tags, "poison", "ppoisn", "PPOISN") {
			t.Fatalf("tags = %+v, want PPOISN set", c.Metadata.Tags)
		}
	})
}

func TestCheatItemCheckingScenario(t *testing.T) {
	// 4. Cheat Item Checking System (IsBadItem, CheckItem)
	loaded := useWorld(t)
	creature := loaded.Creatures["creature:alice"]
	if creature.Equipment == nil {
		creature.Equipment = map[string]model.ObjectInstanceID{}
	}

	// 정상적인 룸, 프로토타입 세팅
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:cheat-armor",
		DisplayName: "나쁜갑옷",
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:cheat-weapon",
		DisplayName: "나쁜검",
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:cheat-container",
		DisplayName: "나쁜자루",
		Metadata:    model.Metadata{Tags: []string{"container"}},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:good-item",
		DisplayName: "착한검",
	})

	// 아이템 생성 및 배치
	// 1. 방어력 > 50 인 갑옷 (장비 슬롯 장착)
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:cheat-armor",
		PrototypeID: "prototype:cheat-armor",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "body"},
		Properties:  map[string]string{"armor": "51"},
	})
	creature.Equipment["body"] = "object:cheat-armor"

	// 2. 공격력(ndice*sdice+pdice) > 100 인 무기 (인벤토리)
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:cheat-weapon",
		PrototypeID: "prototype:cheat-weapon",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"nDice": "10", "sDice": "10", "pDice": "5"},
	})
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:cheat-weapon")

	// 3. 컨테이너 && shotsmax > 20 인 아이템 (인벤토리)
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:cheat-container",
		PrototypeID: "prototype:cheat-container",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"shotsMax": "21"},
	})
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:cheat-container")

	// 4. 정상적인 착한 아이템
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:good-item",
		PrototypeID: "prototype:good-item",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"nDice": "2", "sDice": "4", "pDice": "1"},
	})
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:good-item")

	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)

	// IsBadItem 함수 확인
	badArmor, _ := runtime.Object("object:cheat-armor")
	if !IsBadItem(runtime, badArmor) {
		t.Fatal("IsBadItem should return true for armor > 50")
	}

	goodItem, _ := runtime.Object("object:good-item")
	if IsBadItem(runtime, goodItem) {
		t.Fatal("IsBadItem should return false for good-item")
	}

	// CheckItem 실행
	err := CheckItem(runtime, "creature:alice")
	if err != nil {
		t.Fatalf("CheckItem error = %v", err)
	}

	// 결과 검증
	// 나쁜갑옷은 삭제되어 없어야 함
	if _, ok := runtime.Object("object:cheat-armor"); ok {
		t.Fatal("cheat-armor was not removed")
	}
	// 나쁜검도 삭제되어 없어야 함
	if _, ok := runtime.Object("object:cheat-weapon"); ok {
		t.Fatal("cheat-weapon was not removed")
	}
	// 나쁜자루도 삭제되어 없어야 함
	if _, ok := runtime.Object("object:cheat-container"); ok {
		t.Fatal("cheat-container was not removed")
	}
	// 착한검은 남아 있어야 함
	if _, ok := runtime.Object("object:good-item"); !ok {
		t.Fatal("good-item was incorrectly removed")
	}

	// 크리처 장비 슬롯에서도 제거되었는지 확인
	updatedCreature, _ := runtime.Creature("creature:alice")
	if updatedCreature.Equipment["body"] != "" {
		t.Fatal("equipment slot body still holds cheat-armor")
	}
}

// TestSPWarItemUsesCSpecialObjectDefault verifies SP_WAR stops generic routing and uses C's special_obj default.
func TestSPWarItemUsesCSpecialObjectDefault(t *testing.T) {
	loaded := useWorld(t)
	creature := loaded.Creatures["creature:alice"]
	// add war item
	warItem := model.ObjectInstance{
		ID:          "object:war-banner",
		PrototypeID: "prototype:war-banner",
		Location:    model.ObjectLocation{CreatureID: creature.ID, Slot: "inventory"},
		Properties:  map[string]string{"special": "SP_WAR", "name": "war-banner"},
	}
	loaded.Objects[warItem.ID] = warItem
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, warItem.ID)
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: string(creature.PlayerID)}
	// simulate use special war
	status, err := NewUseHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"war-banner"}})
	if err != nil {
		t.Fatalf("use war item err: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status %d", status)
	}
	out := ctx.OutputString()
	if out != "아무것도 없습니다.\n" {
		t.Fatalf("war item output = %q, want C special_obj default", out)
	}
	if _, ok := runtime.Object("object:war-banner"); !ok {
		t.Fatal("war item was consumed")
	}
}
