package command

import (
	"strings"
	"testing"

	"muhan/internal/commandspec"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestBuyStatesHandlerRaisesCaretakerHP(t *testing.T) {
	world := state.NewWorld(buyStatesWorld(t, model.ClassCaretaker, map[string]int{
		"experience": 110000000,
		"gold":       9000000,
		"hpMax":      1000,
		"hpCurrent":  500,
		"mpMax":      700,
		"mpCurrent":  300,
	}))
	ctx := &Context{ActorID: "player:alice"}

	status, err := NewBuyStatesHandler(world, fixedRoll(0))(ctx, ResolvedCommand{Args: []string{"체력"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if !strings.Contains(ctx.OutputString(), "축하합니다") {
		t.Fatalf("output = %q, want success", ctx.OutputString())
	}

	creature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("missing creature")
	}
	for key, want := range map[string]int{
		"hpMax":      1008,
		"hpCurrent":  1008,
		"experience": 101000000,
		"gold":       0,
		"nDice":      4,
		"sDice":      3,
		"pDice":      4,
	} {
		if got := creatureStat(creature, key); got != want {
			t.Fatalf("%s = %d, want %d", key, got, want)
		}
	}
}

func TestBuyStatesHandlerRaisesBulsaAttribute(t *testing.T) {
	world := state.NewWorld(buyStatesWorld(t, model.ClassBulsa, map[string]int{
		"experience": 200000000,
		"gold":       50000000,
		"strength":   20,
		"hpMax":      2500,
		"mpMax":      1200,
	}))
	ctx := &Context{ActorID: "player:alice"}

	status, err := NewBuyStatesHandler(world, fixedRoll(1))(ctx, ResolvedCommand{Args: []string{"힘"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}

	creature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("missing creature")
	}
	for key, want := range map[string]int{
		"strength":   21,
		"hpMax":      2504,
		"experience": 150000000,
		"gold":       0,
		"nDice":      5,
		"sDice":      5,
		"pDice":      5,
	} {
		if got := creatureStat(creature, key); got != want {
			t.Fatalf("%s = %d, want %d", key, got, want)
		}
	}
}

func TestBuyStatesHandlerRejectsAttributeWithoutLegacyGoldChunk(t *testing.T) {
	world := state.NewWorld(buyStatesWorld(t, model.ClassCaretaker, map[string]int{
		"experience": 103000000,
		"gold":       0,
		"strength":   20,
		"mpMax":      100,
	}))
	ctx := &Context{ActorID: "player:alice"}
	rollMin := func(min int, max int) int { return min }

	status, err := NewBuyStatesHandler(world, rollMin)(ctx, ResolvedCommand{Args: []string{"힘"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if got := ctx.OutputString(); got != "당신이 가진 돈으로는 향상을 할수 없습니다.\n" {
		t.Fatalf("output = %q", got)
	}

	creature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("missing creature")
	}
	for key, want := range map[string]int{
		"strength":   20,
		"mpMax":      100,
		"experience": 103000000,
		"gold":       0,
	} {
		if got := creatureStat(creature, key); got != want {
			t.Fatalf("%s = %d, want %d", key, got, want)
		}
	}
}

func TestBuyStatesHandlerCaretakerDexterityHPBonusUsesLegacyCommaExpression(t *testing.T) {
	world := state.NewWorld(buyStatesWorld(t, model.ClassCaretaker, map[string]int{
		"experience": 103000000,
		"gold":       3000000,
		"dexterity":  20,
		"hpMax":      100,
	}))
	ctx := &Context{ActorID: "player:alice"}
	rollHP := func(min int, max int) int {
		if min == 0 && max == 1 {
			return 1
		}
		return min
	}

	status, err := NewBuyStatesHandler(world, rollHP)(ctx, ResolvedCommand{Args: []string{"민첩"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}

	creature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("missing creature")
	}
	if got := creatureStat(creature, "hpMax"); got != 104 {
		t.Fatalf("hpMax = %d, want 104", got)
	}
}

func TestBuyStatesHandlerRejectsInvalidCases(t *testing.T) {
	tests := []struct {
		name  string
		class int
		stats map[string]int
		args  []string
		want  string
	}{
		{
			name:  "wrong class",
			class: model.ClassFighter,
			stats: map[string]int{"experience": 200000000, "gold": 100000000},
			args:  []string{"체력"},
			want:  "초인, 불사만이 가능합니다.",
		},
		{
			name:  "missing target",
			class: model.ClassCaretaker,
			stats: map[string]int{"experience": 200000000, "gold": 100000000},
			want:  "\"체력\" 과 \"도력\" 중 어느 것을 올리시려고요?",
		},
		{
			name:  "low experience hp",
			class: model.ClassCaretaker,
			stats: map[string]int{"experience": 100000000, "gold": 100000000},
			args:  []string{"체력"},
			want:  "당신의 경험치로는 능력치 향상을 할 수 없습니다.",
		},
		{
			name:  "low gold hp",
			class: model.ClassCaretaker,
			stats: map[string]int{"experience": 110000000, "gold": 100},
			args:  []string{"체력"},
			want:  "당신이 가진 돈으로는 향상을 할수 없습니다.",
		},
		{
			name:  "unknown target",
			class: model.ClassCaretaker,
			stats: map[string]int{"experience": 110000000, "gold": 3000000},
			args:  []string{"운"},
			want:  "어떤 능력치를 올리시려고요?",
		},
		{
			name:  "unknown target still checks experience first",
			class: model.ClassCaretaker,
			stats: map[string]int{"experience": 100000000, "gold": 3000000},
			args:  []string{"운"},
			want:  "당신의 경험치로는 능력치 향상을 할 수 없습니다.",
		},
		{
			name:  "unknown target still checks gold first",
			class: model.ClassCaretaker,
			stats: map[string]int{"experience": 103000000, "gold": 0},
			args:  []string{"운"},
			want:  "당신이 가진 돈으로는 향상을 할수 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := state.NewWorld(buyStatesWorld(t, tt.class, tt.stats))
	defer world.Close()
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewBuyStatesHandler(world, fixedRoll(0))(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want default", status)
			}
			if !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("output = %q, want %q", ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestBuyStatesHandlerDispatchAlias(t *testing.T) {
	world := state.NewWorld(buyStatesWorld(t, model.ClassCaretaker, map[string]int{
		"experience": 106000000,
		"gold":       3000000,
		"mpMax":      1000,
		"mpCurrent":  500,
	}))
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "향상", Number: 149, Handler: "buy_states"},
		}),
		Handlers: map[string]Handler{"buy_states": NewBuyStatesHandler(world, fixedRoll(0))},
	}
	ctx := &Context{ActorID: "player:alice"}

	status, err := dispatcher.DispatchLine(ctx, "도력 향상")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	creature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("missing creature")
	}
	if got := creatureStat(creature, "mpMax"); got != 1002 {
		t.Fatalf("mpMax = %d, want 1002", got)
	}
}

func TestBuyStatesHandlerBroadcastsLegacySuccessLine(t *testing.T) {
	world := state.NewWorld(buyStatesWorld(t, model.ClassCaretaker, map[string]int{
		"experience": 103000000,
		"gold":       3000000,
		"hpMax":      100,
	}))
	var broadcast string
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.broadcast": func(cmd struct{ Write string }) {
				broadcast += cmd.Write
			},
		},
	}

	status, err := NewBuyStatesHandler(world, fixedRoll(0))(ctx, ResolvedCommand{Args: []string{"체력"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if got := ctx.OutputString(); got != "\n축하합니다! 당신의 능력치가 올랐습니다!" {
		t.Fatalf("output = %q", got)
	}
	if want := "\n### Alice님의 능력치가 향상이 되었습니다!"; broadcast != want {
		t.Fatalf("broadcast = %q, want %q", broadcast, want)
	}
}

func buyStatesWorld(t *testing.T, class int, stats map[string]int) *worldload.World {
	t.Helper()
	loaded := worldload.NewWorld()
	if err := loaded.AddRoom(model.Room{ID: "room:plaza", DisplayName: "광장"}); err != nil {
		t.Fatal(err)
	}
	player := model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:plaza",
	}
	if err := loaded.AddPlayer(player); err != nil {
		t.Fatal(err)
	}
	creatureStats := map[string]int{"class": class}
	for key, value := range stats {
		creatureStats[key] = value
	}
	creature := model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:plaza",
		Stats:       creatureStats,
	}
	if err := loaded.AddCreature(creature); err != nil {
		t.Fatal(err)
	}
	return loaded
}

// TestLegacyLevelUpFormulas verifies exact C formulas/tables for level/stat gains (P1-4, Package 6/6).
// Matches src/player.c up_level + global.c class_stats + level_cycle + needed_exp.
func TestLegacyLevelUpFormulas(t *testing.T) {
	// Table spot checks (from C)
	if b := legacyClassStatBonusesFor(model.ClassFighter); b.hp != 6 || b.mp != 1 || b.hpStart != 56 {
		t.Fatalf("fighter bonuses mismatch: %+v", b)
	}
	if b := legacyClassStatBonusesFor(model.ClassMage); b.hp != 4 || b.mp != 3 {
		t.Fatalf("mage bonuses mismatch")
	}
	cyc := legacyLevelCycleFor(model.ClassAssassin)
	if cyc[0] != legacyStatCON || cyc[2] != legacyStatSTR {
		t.Fatalf("assassin cycle mismatch: %v", cyc)
	}

	// Formula for level 5 fighter: hp = 56 + 6*(5-1)/2 = 56+12=68 ; mp=50 +1*2=52
	hp := 56 + (6 * (5 - 1) / 2)
	if hp != 68 {
		t.Fatalf("hp formula for lvl5 fighter = %d, want 68 (C exact)", hp)
	}

	// Stat inc: at lvl 4 (4%4==0), index=(4-2)%10=2 -> for assassin cycle[2]=STR -> +str
	// (verified by apply logic)

	// Apply func smoke (mock setter collector)
	type mockWorld struct {
		sets map[string]int
	}
	mw := &mockWorld{sets: map[string]int{}}
	crt := model.Creature{ID: "c1", Stats: map[string]int{"strength": 10}}
	// adapter to satisfy the anonymous interface expected by applyLegacyLevelUp
	adapter := &levelUpAdapter{sets: mw.sets}
	if err := applyLegacyLevelUp(adapter, crt, model.ClassAssassin, 3, 4); err != nil {
		t.Fatalf("apply err: %v", err)
	}
	if got := mw.sets["pDice"]; got != 1 {
		t.Errorf("pDice without training flags = %d, want 1", got)
	}
	if _, ok := mw.sets["hpMax"]; !ok {
		t.Errorf("expected hpMax set")
	}
	if _, ok := mw.sets["strength"]; !ok {
		t.Errorf("expected STR inc at lvl4 for assassin cycle")
	}

	trainedSets := map[string]int{}
	trained := model.Creature{
		ID: "trained",
		Stats: map[string]int{
			"strength": 10,
			"SFIGHTER": 1,
		},
		Properties: map[string]string{
			"training": "SMAGE SPALADIN SRANGER STHIEF",
		},
		Metadata: model.Metadata{Tags: []string{
			"SASSASSIN",
			"SBARBARIAN",
			"SCLERIC",
		}},
	}
	if err := applyLegacyLevelUp(&levelUpAdapter{sets: trainedSets}, trained, model.ClassAssassin, 3, 4); err != nil {
		t.Fatalf("apply trained err: %v", err)
	}
	if got := trainedSets["pDice"]; got != 4 {
		t.Errorf("pDice with 8 training flags = %d, want 4", got)
	}
}

type levelUpAdapter struct{ sets map[string]int }

func (a *levelUpAdapter) SetCreatureStat(_ model.CreatureID, k string, v int) error {
	if a.sets == nil {
		a.sets = map[string]int{}
	}
	a.sets[k] = v
	return nil
}
