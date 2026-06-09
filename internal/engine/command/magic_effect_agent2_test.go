package command

import (
	"strings"
	"testing"

	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestMagicEffectDrainExpSelf(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "48")
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 10
	alice.Stats = map[string]int{
		"class":            legacyClassDM,
		"level":            10,
		"experience":       1000,
		"proficiencySharp": 2000,
	}
	alice.Metadata.Tags = []string{"SDREXP"}
	loaded.Creatures[alice.ID] = alice

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"주문"}},
		magicPowerDrainExp,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled {
		t.Fatalf("Expected handled to be true")
	}
	if !success {
		t.Fatalf("Expected success to be true")
	}

	// Verify experience is reduced
	updatedAlice, _ := runtime.Creature("creature:alice")
	newExp := updatedAlice.Stats["experience"]
	if newExp >= 1000 {
		t.Fatalf("Expected experience to be reduced, got %d", newExp)
	}

	// Verify weapon proficiency is reduced
	newProf := updatedAlice.Stats["proficiencySharp"]
	if newProf >= 2000 {
		t.Fatalf("Expected proficiencySharp to be reduced, got %d", newProf)
	}

	// Verify message
	if !strings.Contains(ctx.OutputString(), "당신은 갑자기 멍청해 지면서") {
		t.Fatalf("Expected self message to be printed, got: %q", ctx.OutputString())
	}
}

func TestMagicEffectDrainExpTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "48")
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 10
	alice.Stats = map[string]int{"class": legacyClassDM, "level": 10, "experience": 1000}
	alice.Metadata.Tags = []string{"SDREXP"}
	loaded.Creatures[alice.ID] = alice

	// Add Bob as a player/creature in the same room
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	bob := model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:library",
		Stats: map[string]int{
			"level":            5,
			"experience":       500,
			"proficiencySharp": 1500,
		},
	}
	loaded.Creatures[bob.ID] = bob

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	// Apply magicPowerDrainExp on Bob
	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"주문", "Bob"}},
		magicPowerDrainExp,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || !success {
		t.Fatalf("Expected handled/success to be true, got %t/%t", handled, success)
	}

	// Verify Bob's experience and proficiency reduced
	updatedBob, _ := runtime.Creature("creature:bob")
	if updatedBob.Stats["experience"] >= 500 {
		t.Fatalf("Expected Bob experience to be reduced")
	}
	if updatedBob.Stats["proficiencySharp"] >= 1500 {
		t.Fatalf("Expected Bob proficiencySharp to be reduced")
	}
}

func TestReduceWeaponProficiencyUsesLegacyLowerProfSlots(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "48")
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{
		"proficiencySharp":   2000,
		"proficiencyThrust":  2000,
		"proficiencyBlunt":   2000,
		"proficiencyPole":    2000,
		"proficiencyMissile": 2000,
		"realmEarth":         2000,
		"realmWind":          2000,
		"realmFire":          2000,
		"realmWater":         2000,
	}
	alice.Properties = map[string]string{
		"proficiency/sharp": "2000",
		"proficiency/0":     "2000",
		"realm/4":           "2000",
	}
	loaded.Creatures[alice.ID] = alice

	runtime := state.NewWorld(loaded)
	reduceWeaponProficiency(runtime, magicEffectTarget{creature: alice}, 90)

	updatedAlice, _ := runtime.Creature(alice.ID)
	if got := updatedAlice.Stats["proficiencySharp"]; got != 1990 {
		t.Fatalf("proficiencySharp = %d, want 1990", got)
	}
	if got := updatedAlice.Stats["realmWater"]; got != 1990 {
		t.Fatalf("realmWater = %d, want 1990", got)
	}
	if got := updatedAlice.Properties["proficiency/sharp"]; got != "1990" {
		t.Fatalf("proficiency/sharp = %q, want 1990", got)
	}
	if got := updatedAlice.Properties["proficiency/0"]; got != "1990" {
		t.Fatalf("proficiency/0 = %q, want 1990", got)
	}
	if got := updatedAlice.Properties["realm/4"]; got != "1990" {
		t.Fatalf("realm/4 = %q, want 1990", got)
	}
}

func TestMagicEffectDrainExpLegacyRestrictions(t *testing.T) {
	tests := []struct {
		name     string
		config   func(*model.Creature) model.ObjectInstance
		resolved ResolvedCommand
		wantOut  string
	}{
		{
			name: "cast requires learned spell first",
			config: func(alice *model.Creature) model.ObjectInstance {
				alice.Stats = map[string]int{"class": legacyClassDM, "level": 10, "experience": 1000}
				return model.ObjectInstance{}
			},
			wantOut: "\n당신은 아직 그런 주문을 터득하지 못했습니다.\n",
		},
		{
			name: "cast is DM only",
			config: func(alice *model.Creature) model.ObjectInstance {
				alice.Stats = map[string]int{"class": legacyClassMage, "level": 10, "experience": 1000}
				alice.Metadata.Tags = []string{"SDREXP"}
				return model.ObjectInstance{}
			},
			wantOut: "\n그런 주문을 외울수 없습니다.\n",
		},
		{
			name: "scroll is rejected",
			config: func(alice *model.Creature) model.ObjectInstance {
				alice.Stats = map[string]int{"class": legacyClassDM, "level": 10, "experience": 1000}
				alice.Metadata.Tags = []string{"SDREXP"}
				return model.ObjectInstance{ID: "object:scroll", Properties: map[string]string{"type": "7"}}
			},
			wantOut: "\n그런 주문을 외울수 없습니다.\n",
		},
		{
			name: "explicit self alias uses target branch and misses",
			config: func(alice *model.Creature) model.ObjectInstance {
				alice.Stats = map[string]int{"class": legacyClassDM, "level": 10, "experience": 1000}
				alice.Metadata.Tags = []string{"SDREXP"}
				return model.ObjectInstance{}
			},
			resolved: ResolvedCommand{Args: []string{"주문", "나"}},
			wantOut:  "\n그런 사람이 존재하지 않습니다 .\n",
		},
		{
			name: "missing target",
			config: func(alice *model.Creature) model.ObjectInstance {
				alice.Stats = map[string]int{"class": legacyClassDM, "level": 10, "experience": 1000}
				alice.Metadata.Tags = []string{"SDREXP"}
				return model.ObjectInstance{}
			},
			resolved: ResolvedCommand{Args: []string{"주문", "Missing"}},
			wantOut:  "\n그런 사람이 존재하지 않습니다 .\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", "48")
			alice := loaded.Creatures["creature:alice"]
			object := tt.config(&alice)
			loaded.Creatures[alice.ID] = alice

			runtime := state.NewWorld(loaded)
			ctx := &Context{ActorID: "player:alice"}

			handled, success, err := ApplyMagicPowerEffectAgent2(
				ctx,
				runtime,
				alice,
				object,
				tt.resolved,
				magicPowerDrainExp,
			)
			if err != nil {
				t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
			}
			if !handled || success {
				t.Fatalf("handled/success = %t/%t, want true/false", handled, success)
			}
			if got := ctx.OutputString(); got != tt.wantOut {
				t.Fatalf("output = %q, want %q", got, tt.wantOut)
			}
		})
	}
}

func TestMagicEffectDrainExpNamedSelfUsesTargetBranchLikeLegacy(t *testing.T) {
	previous := attackRoll
	attackRoll = func(min, max int) int {
		return min
	}
	defer func() { attackRoll = previous }()

	loaded := readScrollWorld(t, "room:library", "1", "48")
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 10
	alice.Stats = map[string]int{
		"class":            legacyClassDM,
		"level":            10,
		"experience":       1000,
		"proficiencySharp": 2000,
	}
	alice.Metadata.Tags = []string{"SDREXP"}
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"주문", "Alice"}},
		magicPowerDrainExp,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || !success {
		t.Fatalf("handled/success = %t/%t, want true/true", handled, success)
	}
	if !strings.Contains(ctx.OutputString(), "\n당신은 Alice에게 백치술의 주문을 외웁니다.\n") {
		t.Fatalf("output = %q, want target-branch caster text", ctx.OutputString())
	}
	updatedAlice, _ := runtime.Creature("creature:alice")
	if got := updatedAlice.Stats["experience"]; got != 880 {
		t.Fatalf("Alice experience = %d, want 880 from target-branch 120 loss", got)
	}
}

func TestMagicEffectDrainExpPotionRejectsTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "48")
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 10
	alice.Stats = map[string]int{"level": 10, "experience": 1000}
	loaded.Creatures[alice.ID] = alice
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	loaded.Creatures["creature:bob"] = model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:library",
		Stats:       map[string]int{"experience": 500},
	}

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}
	potion := model.ObjectInstance{ID: "object:drain-potion", Properties: map[string]string{"type": "6"}}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		potion,
		ResolvedCommand{Args: []string{"주문", "Bob"}},
		magicPowerDrainExp,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || success {
		t.Fatalf("handled/success = %t/%t, want true/false", handled, success)
	}
	if got, want := ctx.OutputString(), "\n이 물건은 자신에게만 사용할수 있습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	updatedBob, _ := runtime.Creature("creature:bob")
	if got := updatedBob.Stats["experience"]; got != 500 {
		t.Fatalf("Bob experience = %d, want unchanged 500", got)
	}
}

func TestMagicEffectDrainExpPotionRejectsMissingTargetBeforeLookup(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "48")
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 10
	alice.Stats = map[string]int{"level": 10, "experience": 1000}
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}
	potion := model.ObjectInstance{ID: "object:drain-potion", Properties: map[string]string{"type": "6"}}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		potion,
		ResolvedCommand{Args: []string{"주문", "Nobody"}},
		magicPowerDrainExp,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || success {
		t.Fatalf("handled/success = %t/%t, want true/false", handled, success)
	}
	if got, want := ctx.OutputString(), "\n이 물건은 자신에게만 사용할수 있습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	updatedAlice, _ := runtime.Creature("creature:alice")
	if got := updatedAlice.Stats["experience"]; got != 1000 {
		t.Fatalf("Alice experience = %d, want unchanged 1000", got)
	}
}

func TestMagicEffectDrainExpWandTargetUsesObjectDice(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "48")
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 40
	alice.Stats = map[string]int{"level": 40, "experience": 1000}
	loaded.Creatures[alice.ID] = alice
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	loaded.Creatures["creature:bob"] = model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:library",
		Stats:       map[string]int{"level": 5, "experience": 100, "proficiencySharp": 1500},
	}

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}
	wand := model.ObjectInstance{
		ID:         "object:drain-wand",
		Properties: map[string]string{"type": "8", "pDice": "37"},
	}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		wand,
		ResolvedCommand{Args: []string{"주문", "Bob"}},
		magicPowerDrainExp,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || !success {
		t.Fatalf("handled/success = %t/%t, want true/true", handled, success)
	}
	updatedBob, _ := runtime.Creature("creature:bob")
	if got := updatedBob.Stats["experience"]; got != 63 {
		t.Fatalf("Bob experience = %d, want 63 from wand mdice pDice 37", got)
	}
}

func TestMagicEffectCharmSelf(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "56")
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{"intelligence": 18}
	loaded.Creatures[alice.ID] = alice

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"주문"}},
		magicPowerCharm,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || !success {
		t.Fatalf("Expected handled/success to be true")
	}

	// C charm(self) records LT_CHRMD but does not set PCHARM.
	updatedAlice, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updatedAlice.Metadata.Tags, "charmed", "PCHARM") {
		t.Fatalf("Unexpected charmed/PCHARM tags on self charm, got %v", updatedAlice.Metadata.Tags)
	}

	// Verify self message is written
	if !strings.Contains(ctx.OutputString(), "당신은 심심해서 거울을 보며 이혼대법을 사용합니다.") {
		t.Fatalf("Expected mirror charm message, got %q", ctx.OutputString())
	}
}

func TestMagicEffectCharmExplicitSelfAliasMissesLikeLegacy(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "56")
	alice := loaded.Creatures["creature:alice"]
	loaded.Creatures[alice.ID] = alice

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"주문", "나"}},
		magicPowerCharm,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || success {
		t.Fatalf("handled/success = %t/%t, want true/false", handled, success)
	}
	if got, want := ctx.OutputString(), "그런 사람이 존재하지 않습니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestMagicEffectCharmNamedSelfUsesTargetBranchLikeLegacy(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "56")
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 10
	alice.Stats = map[string]int{"level": 10, "intelligence": 18}
	loaded.Creatures[alice.ID] = alice

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"주문", "Alice"}},
		magicPowerCharm,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || !success {
		t.Fatalf("handled/success = %t/%t, want true/true", handled, success)
	}
	if !strings.Contains(ctx.OutputString(), "당신은 Alice에게 거울을 비추며 이혼대법을 겁니다.") {
		t.Fatalf("output = %q, want target-branch charm text", ctx.OutputString())
	}
	updatedAlice, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updatedAlice.Metadata.Tags, "charmed", "PCHARM") {
		t.Fatalf("creature tags = %+v, want target-branch charm tags", updatedAlice.Metadata.Tags)
	}
	if !magicEffectTestHasExactTag(updatedAlice.Metadata.Tags, "charm:Alice") {
		t.Fatalf("creature tags = %+v, want target-branch charm list entry", updatedAlice.Metadata.Tags)
	}
}

func TestMagicEffectCharmPotionUsesLegacySelfOutput(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "56")
	alice := loaded.Creatures["creature:alice"]
	loaded.Creatures[alice.ID] = alice

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}
	potion := model.ObjectInstance{
		ID:         "object:charm-potion",
		Properties: map[string]string{"type": "6"},
	}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		potion,
		ResolvedCommand{},
		magicPowerCharm,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || !success {
		t.Fatalf("handled/success = %t/%t, want true/true", handled, success)
	}
	want := "기분이 좋아지면서 괜히 맞아도 황홀한 기분이\n듭니다. 나 좀 때려줘.."
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}

	updatedAlice, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updatedAlice.Metadata.Tags, "charmed", "PCHARM") {
		t.Fatalf("Unexpected charmed/PCHARM tags after charm potion, got %v", updatedAlice.Metadata.Tags)
	}
}

func TestMagicEffectCharmPotionRejectsTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "56")
	alice := loaded.Creatures["creature:alice"]
	loaded.Creatures[alice.ID] = alice
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	loaded.Creatures["creature:bob"] = model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:library",
	}

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}
	potion := model.ObjectInstance{
		ID:         "object:charm-potion",
		Properties: map[string]string{"type": "6"},
	}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		potion,
		ResolvedCommand{Args: []string{"주문", "Bob"}},
		magicPowerCharm,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || success {
		t.Fatalf("handled/success = %t/%t, want true/false", handled, success)
	}
	if got, want := ctx.OutputString(), "그 물건은 자신에게만 사용할수 있습니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestMagicEffectCharmMissingTargetUsesLegacyText(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "56")
	alice := loaded.Creatures["creature:alice"]
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}
	scroll := model.ObjectInstance{
		ID:         "object:charm-scroll",
		Properties: map[string]string{"type": "7"},
	}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		scroll,
		ResolvedCommand{Args: []string{"주문", "Nobody"}},
		magicPowerCharm,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || success {
		t.Fatalf("handled/success = %t/%t, want true/false", handled, success)
	}
	if got, want := ctx.OutputString(), "그런 사람이 존재하지 않습니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestMagicEffectCharmPotionRejectsMissingTargetBeforeLookup(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "56")
	alice := loaded.Creatures["creature:alice"]
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}
	potion := model.ObjectInstance{
		ID:         "object:charm-potion",
		Properties: map[string]string{"type": "6"},
	}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		potion,
		ResolvedCommand{Args: []string{"주문", "Nobody"}},
		magicPowerCharm,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || success {
		t.Fatalf("handled/success = %t/%t, want true/false", handled, success)
	}
	if got, want := ctx.OutputString(), "그 물건은 자신에게만 사용할수 있습니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestMagicEffectCharmSurvivalUsesLegacyText(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "56")
	room := loaded.Rooms["room:library"]
	room.Metadata.Tags = append(room.Metadata.Tags, "RSUVIV")
	loaded.Rooms[room.ID] = room
	alice := loaded.Creatures["creature:alice"]
	loaded.Creatures[alice.ID] = alice

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{},
		magicPowerCharm,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || success {
		t.Fatalf("handled/success = %t/%t, want true/false", handled, success)
	}
	if got, want := ctx.OutputString(), "대련장에서는 이 주문을 사용할 수 없습니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestMagicEffectCharmTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "56")
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 10
	alice.Stats = map[string]int{"level": 10, "intelligence": 18}
	loaded.Creatures[alice.ID] = alice

	// Add Bob as a player/creature in the same room, level 5 (less than Alice)
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	bob := model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:library",
		Level:       5,
		Stats: map[string]int{
			"level": 5,
		},
	}
	loaded.Creatures[bob.ID] = bob

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"주문", "Bob"}},
		magicPowerCharm,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || !success {
		t.Fatalf("Expected handled/success to be true")
	}

	// Verify charmed and PCHARM tags added to Bob
	updatedBob, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(updatedBob.Metadata.Tags, "charmed") || !hasAnyNormalizedFlag(updatedBob.Metadata.Tags, "PCHARM") {
		t.Fatalf("Expected charmed/PCHARM tags on Bob creature")
	}
	updatedAlice, _ := runtime.Creature("creature:alice")
	if !magicEffectTestHasExactTag(updatedAlice.Metadata.Tags, "charm:Bob") {
		t.Fatalf("Expected Alice charm list tag for Bob, got %v", updatedAlice.Metadata.Tags)
	}
	charmed, err := runtime.PlayerCharmedCreatures("player:alice")
	if err != nil {
		t.Fatalf("PlayerCharmedCreatures() error = %v", err)
	}
	if len(charmed) != 1 || charmed[0] != "Bob" {
		t.Fatalf("PlayerCharmedCreatures() = %+v, want Bob", charmed)
	}

	// Verify fail case when caster level is lower
	alice.Level = 3
	alice.Stats["level"] = 3
	alice.Stats["mpCurrent"] = 30
	loaded.Creatures[alice.ID] = alice
	runtime = state.NewWorld(loaded)
	ctx = &Context{ActorID: "player:alice"}

	handled, success, err = ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"주문", "Bob"}},
		magicPowerCharm,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || success {
		t.Fatalf("handled/success = %t/%t, want true/false", handled, success)
	}
	if !strings.Contains(ctx.OutputString(), "상대의 기가 당신의 주문을 반탄시킵니다.") {
		t.Fatalf("Expected repel message, got %q", ctx.OutputString())
	}
	updatedAlice, _ = runtime.Creature("creature:alice")
	if got := updatedAlice.Stats["mpCurrent"]; got != 15 {
		t.Fatalf("mpCurrent after repel = %d, want 15", got)
	}
}

func magicEffectTestHasExactTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

func TestMagicEffectRmGongSelf(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "62")
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{"class": legacyClassBulsa, "mpCurrent": 100}
	alice.Metadata.Tags = []string{"fearful", "SRMGONG"}
	loaded.Creatures[alice.ID] = alice

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"주문"}},
		magicPowerRmGong,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || !success {
		t.Fatalf("Expected handled/success to be true")
	}

	// Verify fearful tag removed
	updatedAlice, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updatedAlice.Metadata.Tags, "fearful") {
		t.Fatalf("Expected fearful tag to be removed from Alice")
	}

	// Verify self message is written
	if !strings.Contains(ctx.OutputString(), "당신이 공포해소 주문을 외우자 주위에 있던 공포가 사라집니다.") {
		t.Fatalf("Expected self message, got %q", ctx.OutputString())
	}
}

func TestMagicEffectRmGongExplicitSelfAliasMissesLikeLegacy(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "62")
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{"class": legacyClassBulsa, "mpCurrent": 100}
	alice.Metadata.Tags = []string{"fearful", "SRMGONG"}
	loaded.Creatures[alice.ID] = alice

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"주문", "나"}},
		magicPowerRmGong,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || success {
		t.Fatalf("handled/success = %t/%t, want true/false", handled, success)
	}
	if got, want := ctx.OutputString(), "그런 사람이 존재하지 않습니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	updatedAlice, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updatedAlice.Metadata.Tags, "fearful") {
		t.Fatalf("creature tags = %+v, want fear unchanged after explicit self alias miss", updatedAlice.Metadata.Tags)
	}
	if got := updatedAlice.Stats["mpCurrent"]; got != 100 {
		t.Fatalf("mpCurrent = %d, want unchanged 100", got)
	}
}

func TestMagicEffectRmGongNamedSelfUsesTargetBranchLikeLegacy(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "62")
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{"class": legacyClassBulsa, "mpCurrent": 100}
	alice.Metadata.Tags = []string{"fearful", "SRMGONG"}
	loaded.Creatures[alice.ID] = alice

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"주문", "Alice"}},
		magicPowerRmGong,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || !success {
		t.Fatalf("handled/success = %t/%t, want true/true", handled, success)
	}
	if !strings.Contains(ctx.OutputString(), "당신은 Alice의 회복을 기원하며 공포해소 주문을 외우자") {
		t.Fatalf("output = %q, want target-branch rm_gong text", ctx.OutputString())
	}
	updatedAlice, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updatedAlice.Metadata.Tags, "fearful", "PFEARS") {
		t.Fatalf("creature tags = %+v, want fear removed by target branch", updatedAlice.Metadata.Tags)
	}
	if got := updatedAlice.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after target-branch 100 MP cost", got)
	}
}

func TestMagicEffectRmGongPotionUsesLegacyOutput(t *testing.T) {
	tests := []struct {
		name    string
		tags    []string
		wantOut string
	}{
		{
			name:    "fearful",
			tags:    []string{"PFEARS"},
			wantOut: "새하얗던 얼굴에 핏기가 돌기 시작합니다.",
		},
		{
			name:    "not fearful",
			wantOut: "아무 반응이 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", "62")
			alice := loaded.Creatures["creature:alice"]
			alice.Stats = map[string]int{"class": legacyClassBulsa}
			alice.Metadata.Tags = tt.tags
			loaded.Creatures[alice.ID] = alice

			runtime := state.NewWorld(loaded)
			ctx := &Context{ActorID: "player:alice"}
			potion := model.ObjectInstance{
				ID:         "object:rm-gong-potion",
				Properties: map[string]string{"type": "6"},
			}

			handled, success, err := ApplyMagicPowerEffectAgent2(
				ctx,
				runtime,
				alice,
				potion,
				ResolvedCommand{},
				magicPowerRmGong,
			)
			if err != nil {
				t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
			}
			if !handled || !success {
				t.Fatalf("handled/success = %t/%t, want true/true", handled, success)
			}
			if got := ctx.OutputString(); got != tt.wantOut {
				t.Fatalf("output = %q, want %q", got, tt.wantOut)
			}

			updatedAlice, _ := runtime.Creature("creature:alice")
			if hasAnyNormalizedFlag(updatedAlice.Metadata.Tags, "PFEARS", "fearful") {
				t.Fatalf("fear tag remains after potion: %+v", updatedAlice.Metadata.Tags)
			}
		})
	}
}

func TestMagicEffectRmGongRejectsWrongClassWithLegacyText(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "62")
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got, want := ctx.OutputString(), "아직 당신의 능력으로는 외울수 없는 주문입니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after rejected rm_gong class gate")
	}
}

func TestMagicEffectRmGongMissingTargetUsesLegacyText(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "62")
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{"class": legacyClassBulsa}
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Nobody"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got, want := ctx.OutputString(), "그런 사람이 존재하지 않습니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after missing rm_gong target")
	}
}

func TestMagicEffectRmGongPotionRejectsExplicitTargetBeforeLookup(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "62")
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{"class": legacyClassBulsa}
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}
	potion := model.ObjectInstance{
		ID:         "object:rm-gong-potion",
		Properties: map[string]string{"type": "6"},
	}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		potion,
		ResolvedCommand{Args: []string{"주문", "Nobody"}},
		magicPowerRmGong,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || success {
		t.Fatalf("handled/success = %t/%t, want true/false", handled, success)
	}
	if got, want := ctx.OutputString(), "이 물건은 자신에게만 사용할수 있습니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestMagicEffectRmGongTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "62")
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{"class": legacyClassBulsa, "mpCurrent": 100}
	alice.Metadata.Tags = []string{"SRMGONG"}
	loaded.Creatures[alice.ID] = alice

	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	bob := model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:library",
		Metadata: model.Metadata{
			Tags: []string{"fearful"},
		},
	}
	loaded.Creatures[bob.ID] = bob

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	handled, success, err := ApplyMagicPowerEffectAgent2(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"주문", "Bob"}},
		magicPowerRmGong,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
	}
	if !handled || !success {
		t.Fatalf("Expected handled/success to be true")
	}

	// Verify Bob's fearful tag removed
	updatedBob, _ := runtime.Creature("creature:bob")
	if hasAnyNormalizedFlag(updatedBob.Metadata.Tags, "fearful") {
		t.Fatalf("Expected fearful tag to be removed from Bob")
	}

	// Verify messages
	if !strings.Contains(ctx.OutputString(), "당신은 Bob의 회복을 기원하며 공포해소 주문을 외우자") {
		t.Fatalf("Expected self message, got %q", ctx.OutputString())
	}
}

func TestMagicEffectAgent2TargetMessagesResolveActiveSession(t *testing.T) {
	tests := []struct {
		name      string
		power     int
		configure func(*model.Creature, *model.Creature)
		want      string
	}{
		{
			name:  "drain exp",
			power: magicPowerDrainExp,
			configure: func(alice, bob *model.Creature) {
				alice.Level = 10
				alice.Stats = map[string]int{"class": legacyClassDM, "level": 10, "experience": 1000}
				alice.Metadata.Tags = []string{"SDREXP"}
				bob.Level = 5
				bob.Stats = map[string]int{"level": 5, "experience": 500, "proficiencySharp": 1500}
			},
			want: "당신에게 백치술의 주문을 외웁니다.",
		},
		{
			name:  "charm",
			power: magicPowerCharm,
			configure: func(alice, bob *model.Creature) {
				alice.Level = 10
				alice.Stats = map[string]int{"level": 10, "intelligence": 18}
				bob.Level = 5
				bob.Stats = map[string]int{"level": 5}
			},
			want: "당신에게 거울을 비추며 이혼대법을 겁니다.",
		},
		{
			name:  "rm gong",
			power: magicPowerRmGong,
			configure: func(alice, bob *model.Creature) {
				alice.Stats = map[string]int{"class": legacyClassBulsa, "level": 10, "mpCurrent": 100}
				alice.Metadata.Tags = []string{"SRMGONG"}
				bob.Stats = map[string]int{"level": 5}
				bob.Metadata.Tags = []string{"fearful"}
			},
			want: "당신에게 공포해소 주문을 외우자 당신의 겁이 사라집니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", "48")
			alice := loaded.Creatures["creature:alice"]
			mustAddLookPlayer(t, loaded, model.Player{
				ID:          "player:bob",
				DisplayName: "Bob",
				CreatureID:  "creature:bob",
				RoomID:      "room:library",
			})
			bob := model.Creature{
				ID:          "creature:bob",
				Kind:        model.CreatureKindPlayer,
				DisplayName: "Bob",
				PlayerID:    "player:bob",
				RoomID:      "room:library",
			}
			tt.configure(&alice, &bob)
			loaded.Creatures[alice.ID] = alice
			loaded.Creatures[bob.ID] = bob

			runtime := state.NewWorld(loaded)
			sent := map[string]string{}
			ctx := &Context{
				ActorID:   "player:alice",
				SessionID: "session:alice",
				Values: map[string]any{
					"game.activeSessions": func() []activeSession {
						return []activeSession{
							{ID: "session:alice", ActorID: "player:alice"},
							{ID: "session:bob", ActorID: "player:bob"},
						}
					},
					"game.sendToSession": func(id string, cmd struct{ Write string }) error {
						sent[id] += cmd.Write
						return nil
					},
				},
			}

			handled, success, err := ApplyMagicPowerEffectAgent2(
				ctx,
				runtime,
				alice,
				model.ObjectInstance{},
				ResolvedCommand{Args: []string{"주문", "Bob"}},
				tt.power,
			)
			if err != nil {
				t.Fatalf("ApplyMagicPowerEffectAgent2 error = %v", err)
			}
			if !handled || !success {
				t.Fatalf("handled/success = %t/%t, want true/true", handled, success)
			}
			if _, ok := sent["player:bob"]; ok {
				t.Fatalf("sent to player id instead of active session: %+v", sent)
			}
			if got := sent["session:bob"]; !strings.Contains(got, tt.want) {
				t.Fatalf("target session message = %q, want %q", got, tt.want)
			}
		})
	}
}
