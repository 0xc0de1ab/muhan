package command

import (
	"strings"
	"testing"
	"time"

	"muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

// mockCastWorld helper to load a fresh state for testing.
func mockCastWorld(t *testing.T) (*state.World, *load.World) {
	t.Helper()
	loaded := &load.World{
		Rooms:            map[model.RoomID]model.Room{},
		Players:          map[model.PlayerID]model.Player{},
		Creatures:        map[model.CreatureID]model.Creature{},
		ObjectPrototypes: map[model.PrototypeID]model.ObjectPrototype{},
		Objects:          map[model.ObjectInstanceID]model.ObjectInstance{},
	}

	// Add room
	loaded.Rooms["room:temple"] = model.Room{
		ID:          "room:temple",
		DisplayName: "신전",
		Metadata:    model.Metadata{Tags: []string{"RCAST", "cast"}},
	}

	// Helper to add player/creature pairs
	addPlayer := func(id string, name string, class int, mp int, hp int) {
		pID := model.PlayerID("player:" + id)
		cID := model.CreatureID("creature:" + id)
		loaded.Players[pID] = model.Player{
			ID:          pID,
			RoomID:      "room:temple",
			CreatureID:  cID,
			DisplayName: name,
		}
		loaded.Creatures[cID] = model.Creature{
			ID:          cID,
			PlayerID:    pID,
			RoomID:      "room:temple",
			DisplayName: name,
			Stats: map[string]int{
				"class":        class,
				"level":        5,
				"hpCurrent":    hp,
				"hpMax":        100,
				"mpCurrent":    mp,
				"mpMax":        100,
				"intelligence": 15,
				"piety":        15,
				"thaco":        20,
				"armor":        100,
			},
		}
	}

	// Add players
	addPlayer("alice", "Alice", legacyClassCleric, 50, 80) // Cleric
	addPlayer("bob", "Bob", legacyClassMage, 50, 80)       // Mage
	addPlayer("charlie", "Charlie", legacyClassDM, 50, 80) // DM
	addPlayer("dave", "Dave", legacyClassFighter, 50, 80)  // Fighter

	w := state.NewWorld(loaded)
	return w, loaded
}

func TestCastInvisibilitySelfRPMEXTMessagePrecedesSpellTextLikeLegacy(t *testing.T) {
	useSpellFailRoll(t, 0)
	world, _ := mockCastWorld(t)
	if err := world.UpdateRoomProperty("room:temple", "RPMEXT", "true"); err != nil {
		t.Fatalf("UpdateRoomProperty(RPMEXT) error = %v", err)
	}
	if _, err := world.UpdateCreatureTags("creature:bob", []string{"SINVIS"}, nil); err != nil {
		t.Fatalf("UpdateCreatureTags(SINVIS) error = %v", err)
	}

	ctx := &Context{ActorID: "player:bob"}
	status, err := NewCastHandler(world, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{"은둔법"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	output := ctx.OutputString()
	boost := strings.Index(output, "이 방의 기운이 당신의 주술력을 강화시킵니다.")
	spell := strings.Index(output, "당신은 소명부를 삼키면서 은둔법의 주문을 외웁니다.")
	if status != StatusDefault || boost < 0 || spell < 0 || boost > spell {
		t.Fatalf("status/output = %d/%q, want C RPMEXT message before invisibility spell text", status, output)
	}
	bob, _ := world.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bob.Metadata.Tags, "invisible", "PINVIS") {
		t.Fatalf("bob tags = %+v, want PINVIS after invisibility", bob.Metadata.Tags)
	}
}

// TestMagicCastingCycleAndCooldowns verifies casting, MP deduction, and class-based cooldowns.
func TestMagicCastingCycleAndCooldowns(t *testing.T) {
	world, _ := mockCastWorld(t)

	// Mock the timeNow function in cast.go
	fakeTime := int64(1000)
	timeNow = func() time.Time {
		return time.Unix(fakeTime, 0)
	}
	defer func() {
		timeNow = time.Now
	}()

	// 1. Cleric Alice casts heal ("회복")
	// Grant her the spell tag
	_, err := world.UpdateCreatureTags("creature:alice", []string{"SVIGOR"}, nil)
	if err != nil {
		t.Fatalf("failed to grant spell: %v", err)
	}

	// Create command dispatcher
	castH := NewCastHandler(world, nil)
	ctx := &Context{
		ActorID: "player:alice",
	}

	// Cast the spell
	status, err := castH(ctx, ResolvedCommand{
		Args: []string{"회복", "Alice"},
	})
	if err != nil {
		t.Fatalf("cast failed: %v", err)
	}
	if status != StatusDefault {
		t.Errorf("expected StatusDefault, got %v", status)
	}

	// Check MP consumption (vigor costs 5)
	cAlice, _ := world.Creature("creature:alice")
	if cAlice.Stats["mpCurrent"] != 45 {
		t.Errorf("expected mpCurrent to be 45, got %d", cAlice.Stats["mpCurrent"])
	}

	// Check success message
	output := ctx.OutputString()
	if !strings.Contains(output, "빛의 정기가 온몸에 스며들면서 체력이 향상되었습니다.") {
		t.Errorf("expected success message, got: %q", output)
	}
	if strings.Contains(output, "당신은 회복 주문을 외웠습니다.") {
		t.Errorf("got non-C generic cast success message: %q", output)
	}

	// 2. Immediately recast. Should fail due to cooldown (Cleric has 3s cooldown)
	ctx2 := &Context{
		ActorID: "player:alice",
	}
	status, err = castH(ctx2, ResolvedCommand{
		Args: []string{"회복", "Alice"},
	})
	if err != nil {
		t.Fatalf("cast check failed: %v", err)
	}
	output2 := ctx2.OutputString()
	if !strings.Contains(output2, "기다리세요") {
		t.Errorf("expected cooldown message, got: %q", output2)
	}

	// 3. Fast-forward time by 3 seconds and recast. Should succeed.
	fakeTime += 3
	ctx3 := &Context{
		ActorID: "player:alice",
	}
	status, err = castH(ctx3, ResolvedCommand{
		Args: []string{"회복", "Alice"},
	})
	if err != nil {
		t.Fatalf("recast failed: %v", err)
	}
	output3 := ctx3.OutputString()
	if !strings.Contains(output3, "빛의 정기가 온몸에 스며들면서 체력이 향상되었습니다.") {
		t.Errorf("expected success message after cooldown, got: %q", output3)
	}

	// 4. Mage Bob casts fireball ("화궁")
	_, err = world.UpdateCreatureTags("creature:alice", []string{"PCHAOS"}, nil)
	if err != nil {
		t.Fatalf("failed to mark Alice chaotic: %v", err)
	}
	_, err = world.UpdateCreatureTags("creature:bob", []string{"SFIREB", "PCHAOS"}, nil)
	if err != nil {
		t.Fatalf("failed to grant spell to Bob: %v", err)
	}
	// Give Bob a dummy creature target in room
	ctxBob := &Context{
		ActorID: "player:bob",
	}
	status, err = castH(ctxBob, ResolvedCommand{
		Args: []string{"화궁", "Alice"},
	})
	if err != nil {
		t.Fatalf("Bob cast failed: %v", err)
	}
	// Mage cooldown is also 3s
	ctxBob2 := &Context{
		ActorID: "player:bob",
	}
	status, err = castH(ctxBob2, ResolvedCommand{
		Args: []string{"화궁", "Alice"},
	})
	if err != nil {
		t.Fatalf("Bob recast check failed: %v", err)
	}
	outputBob2 := ctxBob2.OutputString()
	if !strings.Contains(outputBob2, "기다리세요") {
		t.Errorf("expected Bob to have cooldown, got: %q", outputBob2)
	}

	// 5. DM Charlie casts vigor ("회복"). DM should bypass cooldown
	_, err = world.UpdateCreatureTags("creature:charlie", []string{"SVIGOR"}, nil)
	if err != nil {
		t.Fatalf("failed to grant spell to DM: %v", err)
	}
	ctxDM := &Context{
		ActorID: "player:charlie",
	}
	status, err = castH(ctxDM, ResolvedCommand{
		Args: []string{"회복", "Charlie"},
	})
	if err != nil {
		t.Fatalf("DM cast 1 failed: %v", err)
	}
	ctxDM2 := &Context{
		ActorID: "player:charlie",
	}
	status, err = castH(ctxDM2, ResolvedCommand{
		Args: []string{"회복", "Charlie"},
	})
	if err != nil {
		t.Fatalf("DM cast 2 failed: %v", err)
	}
	outputDM2 := ctxDM2.OutputString()
	if strings.Contains(outputDM2, "기다리세요") {
		t.Errorf("DM should bypass cooldown check, but got: %q", outputDM2)
	}

	// 6. Fighter Dave tries to cast offensive spell "삭풍" (hurt)
	_, err = world.UpdateCreatureTags("creature:dave", []string{"SHURT"}, nil)
	if err != nil {
		t.Fatalf("failed to grant spell to Fighter: %v", err)
	}
	ctxDave := &Context{
		ActorID: "player:dave",
	}
	status, err = castH(ctxDave, ResolvedCommand{
		Args: []string{"삭풍", "Alice"},
	})
	if err != nil {
		t.Fatalf("Fighter cast failed: %v", err)
	}
	outputDave := ctxDave.OutputString()
	if !strings.Contains(outputDave, "공격주문을 쓸 수 없는 직업") {
		t.Errorf("expected Fighter offensive spell restriction message, got: %q", outputDave)
	}
}

// TestMagicTeaching verifies the '가르쳐' (teach) command restrictions and tag propagation.
func TestMagicTeaching(t *testing.T) {
	world, _ := mockCastWorld(t)

	// Alice (Cleric) knows 해독 ("SCUREP")
	_, err := world.UpdateCreatureTags("creature:alice", []string{"SCUREP"}, nil)
	if err != nil {
		t.Fatalf("failed to grant teachable spell to Alice: %v", err)
	}

	teachH := NewTeachHandler(world)
	ctx := &Context{
		ActorID: "player:alice",
	}

	// Alice teaches Bob 해독
	status, err := teachH(ctx, ResolvedCommand{
		Args: []string{"Bob", "해독"},
	})
	if err != nil {
		t.Fatalf("teach command failed: %v", err)
	}
	if status != StatusDefault {
		t.Errorf("expected StatusDefault, got %v", status)
	}

	// Verify Bob has learned the spell tag "SCUREP"
	cBob, _ := world.Creature("creature:bob")
	if !slicesContains(cBob.Metadata.Tags, "SCUREP") {
		t.Errorf("Bob did not receive the spell tag SCUREP, tags = %v", cBob.Metadata.Tags)
	}

	// Alice tries to teach Bob again
	ctx2 := &Context{
		ActorID: "player:alice",
	}
	_, err = teachH(ctx2, ResolvedCommand{
		Args: []string{"Bob", "해독"},
	})
	if err != nil {
		t.Fatalf("duplicate teach failed: %v", err)
	}
	output2 := ctx2.OutputString()
	if !strings.Contains(output2, "이미 터득한 주문") {
		t.Errorf("expected already learned message, got: %q", output2)
	}

	// Dave (Fighter) tries to teach Bob 해독 (should be rejected since only Cleric/Mage/Invincible can teach)
	ctxFighter := &Context{
		ActorID: "player:dave",
	}
	_, err = teachH(ctxFighter, ResolvedCommand{
		Args: []string{"Bob", "해독"},
	})
	if err != nil {
		t.Fatalf("fighter teach failed: %v", err)
	}
	outputFighter := ctxFighter.OutputString()
	if !strings.Contains(outputFighter, "전수시킬 수 있는 능력") {
		t.Errorf("expected class restriction message, got: %q", outputFighter)
	}
}

// TestHealSpellsHPRecovery verifies heal spells correctly restore target's HP.
func TestHealSpellsHPRecovery(t *testing.T) {
	world, _ := mockCastWorld(t)

	// Alice knows vigor ("회복", SVIGOR)
	_, err := world.UpdateCreatureTags("creature:alice", []string{"SVIGOR"}, nil)
	if err != nil {
		t.Fatalf("failed to grant vigor: %v", err)
	}

	// Bob's HP is damaged (e.g. hpCurrent = 10, hpMax = 100)
	err = world.SetCreatureStat("creature:bob", "hpCurrent", 10)
	if err != nil {
		t.Fatalf("failed to set Bob's HP: %v", err)
	}

	castH := NewCastHandler(world, nil)
	ctx := &Context{
		ActorID: "player:alice",
	}

	// Alice casts vigor on Bob
	status, err := castH(ctx, ResolvedCommand{
		Args: []string{"회복", "Bob"},
	})
	if err != nil {
		t.Fatalf("heal cast failed: %v", err)
	}
	if status != StatusDefault {
		t.Errorf("expected StatusDefault, got %v", status)
	}

	// Verify Bob's HP went up
	cBob, _ := world.Creature("creature:bob")
	if cBob.Stats["hpCurrent"] <= 10 {
		t.Errorf("expected Bob's HP to be restored, but it is still %d", cBob.Stats["hpCurrent"])
	}
}

// TestBuffSpellsACTHACOMod verifies buff spells apply tags, which theoretically reduce AC (-10) and THACO (-3).
func TestBuffSpellsACTHACOMod(t *testing.T) {
	world, _ := mockCastWorld(t)

	// Set up RecalculateACFunc and RecalculateTHACOFunc mocks on the world
	world.RecalculateACFunc = func(creatureID model.CreatureID) error {
		c, ok := world.Creature(creatureID)
		if !ok {
			return nil
		}
		ac := 100 // base armor stat is 100 in mockCastWorld
		if creatureHasAnyFlag(c, "PPROTE", "protect", "protection") {
			ac -= 10
		}
		return world.SetCreatureStat(creatureID, "armor", ac)
	}

	world.RecalculateTHACOFunc = func(creatureID model.CreatureID) error {
		c, ok := world.Creature(creatureID)
		if !ok {
			return nil
		}
		thaco := 20 // base thaco stat is 20 in mockCastWorld
		if creatureHasAnyFlag(c, "PBLESS", "bless", "blessed") {
			thaco -= 3
		}
		return world.SetCreatureStat(creatureID, "thaco", thaco)
	}

	fakeTime := int64(1000)
	timeNow = func() time.Time {
		return time.Unix(fakeTime, 0)
	}
	defer func() {
		timeNow = time.Now
	}()

	// Alice knows 성현진 ("SBLESS") and 수호진 ("SPROTE")
	_, err := world.UpdateCreatureTags("creature:alice", []string{"SBLESS", "SPROTE"}, nil)
	if err != nil {
		t.Fatalf("failed to grant buff spells: %v", err)
	}

	castH := NewCastHandler(world, nil)

	// 1. Cast Bless (성현진) on Bob
	ctxBless := &Context{
		ActorID: "player:alice",
	}
	_, err = castH(ctxBless, ResolvedCommand{
		Args: []string{"성현진", "Bob"},
	})
	if err != nil {
		t.Fatalf("bless cast failed: %v", err)
	}

	// Advance time to satisfy the 3-second Cleric cooldown
	fakeTime += 3

	// 2. Cast Protection (수호진) on Bob
	ctxProte := &Context{
		ActorID: "player:alice",
	}
	_, err = castH(ctxProte, ResolvedCommand{
		Args: []string{"수호진", "Bob"},
	})
	if err != nil {
		t.Fatalf("protection cast failed: %v", err)
	}

	// Verify tags are applied on Bob
	cBob, _ := world.Creature("creature:bob")
	if !slicesContains(cBob.Metadata.Tags, "blessed") {
		t.Errorf("Bob missing blessed tag, got %v", cBob.Metadata.Tags)
	}
	if !slicesContains(cBob.Metadata.Tags, "protection") {
		t.Errorf("Bob missing protection tag, got %v", cBob.Metadata.Tags)
	}

	// Verify the business logic modifications of these tags match AC (-10) and THACO (-3) reductions
	if cBob.Stats["armor"] != 90 {
		t.Errorf("expected armor class to be 90, got %d", cBob.Stats["armor"])
	}
	if cBob.Stats["thaco"] != 17 {
		t.Errorf("expected thaco to be 17, got %d", cBob.Stats["thaco"])
	}
}

// TestOffensiveSpellsDiceAndResistance verifies offensive spells roll dice and elemental resistance reduces damage.
func TestOffensiveSpellsDiceAndResistance(t *testing.T) {
	world, _ := mockCastWorld(t)

	// Update Alice's class to Mage so she can cast 동설주 (cold)
	_ = world.SetCreatureStat("creature:alice", "class", legacyClassMage)
	_ = world.SetCreatureStat("creature:alice", "intelligence", 10)
	_ = world.SetCreatureStat("creature:alice", "piety", 10)
	_, _ = world.UpdateCreatureTags("creature:alice", []string{"PCHAOS"}, nil)
	_, _ = world.UpdateCreatureTags("creature:bob", []string{"PCHAOS"}, nil)

	// Mock the timeNow function to avoid cooldown issues between subtests
	fakeTime := int64(1000)
	timeNow = func() time.Time {
		return time.Unix(fakeTime, 0)
	}
	defer func() {
		timeNow = time.Now
	}()

	spells := []struct {
		name      string
		power     int
		tag       string
		minDmg    int
		maxDmg    int
		resistTag string
	}{
		{"삭풍", 2, "SHURTS", 1, 8, "resistMagic"},
		{"화궁", 7, "SFIREB", 10, 18, "resistFire"},
		{"뇌전", 14, "SLGHTN", 21, 30, "resistMagic"},
		{"동설주", 15, "SICEBL", 34, 50, "resistCold"},
	}

	for _, s := range spells {
		t.Run(s.name, func(t *testing.T) {
			// Advance time past any previous cooldown
			fakeTime += 10

			// Grant Alice the spell tag
			_, err := world.UpdateCreatureTags("creature:alice", []string{s.tag}, nil)
			if err != nil {
				t.Fatalf("failed to grant spell tag %s: %v", s.tag, err)
			}

			// Reset Bob's HP to 80
			err = world.SetCreatureStat("creature:bob", "hpCurrent", 80)
			if err != nil {
				t.Fatalf("failed to set Bob HP: %v", err)
			}

			// Reset Alice MP
			_ = world.SetCreatureStat("creature:alice", "mpCurrent", 50)

			castH := NewCastHandler(world, nil)
			ctx := &Context{
				ActorID: "player:alice",
			}

			status, err := castH(ctx, ResolvedCommand{
				CmdName: "cast",
				Args:    []string{s.name, "Bob"},
			})
			if err != nil {
				t.Fatalf("offensive cast failed for %s: %v", s.name, err)
			}
			if status != StatusDefault {
				t.Errorf("expected StatusDefault, got %v", status)
			}

			// Verify Bob took damage
			cBob, _ := world.Creature("creature:bob")
			damageDealt := 80 - cBob.Stats["hpCurrent"]
			if damageDealt <= 0 {
				t.Errorf("expected Bob to take damage from %s, but HP is still %d", s.name, cBob.Stats["hpCurrent"])
			}

			// Verify damage is within bounds (base dice range)
			if damageDealt < s.minDmg || damageDealt > s.maxDmg {
				t.Errorf("expected damage for %s in range [%d, %d], got %d", s.name, s.minDmg, s.maxDmg, damageDealt)
			}

			// Verify target is registered in the enemy list ('AddEnemy') for retaliation
			enemies, err := world.CreatureEnemies("creature:bob")
			if err != nil {
				t.Fatalf("failed to get Bob's enemies: %v", err)
			}
			if !slicesContains(enemies, "Alice") {
				t.Errorf("expected Bob's enemy list to contain Alice after casting %s, got %v", s.name, enemies)
			}

			// Verify elemental resistance reduction logic directly on the creature
			cBob.Metadata.Tags = []string{s.resistTag}
			cBob.Stats["piety"] = 15
			cBob.Stats["intelligence"] = 10 // sum = 25 -> 50% reduction

			// Apply elemental resistance on base damage of 10
			// Reduction = (10 * 2 * 25) / 100 = 5
			// Expected final damage = 10 - 5 = 5
			resDamage := ApplyElementalResistance(cBob, s.power, 10)
			if resDamage != 5 {
				t.Errorf("expected resistance %s to reduce damage of spell %s (power %d) to 5, got %d", s.resistTag, s.name, s.power, resDamage)
			}
		})
	}
}

func TestOffensiveSpellMissingTargetUsesLegacyMessage(t *testing.T) {
	world, _ := mockCastWorld(t)
	_ = world.SetCreatureStat("creature:alice", "class", legacyClassMage)
	_ = world.SetCreatureStat("creature:alice", "mpCurrent", 50)
	_, _ = world.UpdateCreatureTags("creature:alice", []string{"SFIREB"}, nil)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(world, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{"화궁", "없는"}})
	if err != nil {
		t.Fatalf("cast error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 것은 여기에 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want legacy missing target message", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got := alice.Stats["mpCurrent"]; got != 50 {
		t.Fatalf("mpCurrent = %d, want unchanged 50", got)
	}
}

func TestOffensiveSpellRevealsPINVISBeforeTargetLookupLikeLegacy(t *testing.T) {
	world, _ := mockCastWorld(t)
	_ = world.SetCreatureStat("creature:alice", "class", legacyClassMage)
	_ = world.SetCreatureStat("creature:alice", "mpCurrent", 50)
	_ = world.SetCreatureStat("creature:alice", "PINVIS", 1)
	_, _ = world.UpdateCreatureTags("creature:alice", []string{"SFIREB", "PINVIS", "invisible"}, nil)
	_, _ = world.UpdatePlayerTags("player:alice", []string{"PINVIS", "invisible"}, nil)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(world, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{"화궁", "없는"}})
	if err != nil {
		t.Fatalf("cast error = %v", err)
	}
	want := "\n당신의 모습이 원래대로 돌아왔습니다.\n\n그런 것은 여기에 존재하지 않습니다.\n"
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
	alice, _ := world.Creature("creature:alice")
	if creatureHasAnyFlag(alice, "PINVIS", "invisible") || alice.Stats["PINVIS"] != 0 {
		t.Fatalf("alice invisibility = tags:%+v stats:%+v, want cleared before missing-target refusal", alice.Metadata.Tags, alice.Stats)
	}
	player, _ := world.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "PINVIS", "invisible") {
		t.Fatalf("player invisibility tags = %+v, want cleared", player.Metadata.Tags)
	}
	if got := alice.Stats["mpCurrent"]; got != 50 {
		t.Fatalf("mpCurrent = %d, want unchanged 50", got)
	}
}

func TestOffensiveSelfCastSkipsSpellFailLikeLegacy(t *testing.T) {
	useSpellFailRoll(t, 99)
	withAttackRolls(t, 1)
	fakeTime := int64(5000)
	timeNow = func() time.Time {
		return time.Unix(fakeTime, 0)
	}
	defer func() {
		timeNow = time.Now
	}()

	world, _ := mockCastWorld(t)
	_ = world.SetCreatureStat("creature:alice", "class", legacyClassMage)
	_ = world.SetCreatureStat("creature:alice", "mpCurrent", 3)
	_ = world.SetCreatureStat("creature:alice", "hpCurrent", 80)
	_, _ = world.UpdateCreatureTags("creature:alice", []string{"SHURTS"}, nil)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(world, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{"삭풍"}})
	if err != nil {
		t.Fatalf("cast error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "\n주술이 2만큼의 피해를 주었습니다.\n") {
		t.Fatalf("status/output = %d/%q, want self-damage output despite forced spell_fail roll", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got := alice.Stats["hpCurrent"]; got != 78 {
		t.Fatalf("hpCurrent = %d, want 78 after C self offensive damage", got)
	}
	if got := alice.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after C self offensive cost", got)
	}
	expires, ok, err := world.CreatureCooldownExpires("creature:alice", "spell")
	if err != nil {
		t.Fatalf("CreatureCooldownExpires() error = %v", err)
	}
	if !ok || expires != fakeTime+3 {
		t.Fatalf("spell cooldown = %d/%v, want %d/true", expires, ok, fakeTime+3)
	}
}

func TestOffensiveSelfCastLethalDamageLeavesOneHPLikeLegacy(t *testing.T) {
	withAttackRolls(t, 8)
	world, _ := mockCastWorld(t)
	_ = world.SetCreatureStat("creature:alice", "class", legacyClassMage)
	_ = world.SetCreatureStat("creature:alice", "mpCurrent", 3)
	_ = world.SetCreatureStat("creature:alice", "hpCurrent", 2)
	_, _ = world.UpdateCreatureTags("creature:alice", []string{"SHURTS"}, nil)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(world, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{"삭풍"}})
	if err != nil {
		t.Fatalf("cast error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "\n!! 좋아요.. 죽을려면 무슨짓을 못하겠어요. !!\n") {
		t.Fatalf("status/output = %d/%q, want C self-death prevention", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got := alice.Stats["hpCurrent"]; got != 1 {
		t.Fatalf("hpCurrent = %d, want C self offensive floor at 1", got)
	}
}

func TestOffensiveExplicitSelfTargetsAreNotSelfCastLikeLegacy(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{name: "korean self pronoun", target: "나"},
		{name: "actor name", target: "Alice"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world, _ := mockCastWorld(t)
			_ = world.SetCreatureStat("creature:alice", "class", legacyClassMage)
			_ = world.SetCreatureStat("creature:alice", "mpCurrent", 50)
			_ = world.SetCreatureStat("creature:alice", "hpCurrent", 80)
			_, _ = world.UpdateCreatureTags("creature:alice", []string{"SFIREB"}, nil)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewCastHandler(world, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{"화궁", tt.target}})
			if err != nil {
				t.Fatalf("cast error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != "\n그런 것은 여기에 존재하지 않습니다.\n" {
				t.Fatalf("status/output = %d/%q, want C explicit-self missing target", status, ctx.OutputString())
			}
			alice, _ := world.Creature("creature:alice")
			if got := alice.Stats["hpCurrent"]; got != 80 {
				t.Fatalf("hpCurrent = %d, want unchanged 80", got)
			}
			if got := alice.Stats["mpCurrent"]; got != 50 {
				t.Fatalf("mpCurrent = %d, want unchanged 50", got)
			}
		})
	}
}

func TestOffensiveSpellRejectsCharmedPlayerBeforeDamageLikeLegacy(t *testing.T) {
	world, _ := mockCastWorld(t)
	_ = world.SetCreatureStat("creature:alice", "class", legacyClassMage)
	_ = world.SetCreatureStat("creature:alice", "mpCurrent", 50)
	_ = world.SetCreatureStat("creature:bob", "hpCurrent", 80)
	_, _ = world.UpdateCreatureTags("creature:alice", []string{"SFIREB", "PCHARM", "PCHAOS"}, nil)
	_, _ = world.UpdateCreatureTags("creature:bob", []string{"PCHAOS", "charm:Alice"}, nil)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(world, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{"화궁", "Bob"}})
	if err != nil {
		t.Fatalf("cast error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "You just can't bring yourself to do that.\n" {
		t.Fatalf("status/output = %d/%q, want legacy charm refusal", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got := alice.Stats["mpCurrent"]; got != 50 {
		t.Fatalf("mpCurrent = %d, want unchanged 50", got)
	}
	bob, _ := world.Creature("creature:bob")
	if got := bob.Stats["hpCurrent"]; got != 80 {
		t.Fatalf("Bob hpCurrent = %d, want unchanged 80", got)
	}
}

func TestOffensiveSpellPlayerGateMatchesLegacyPvPConditions(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, *state.World)
		want  string
	}{
		{
			name: "safe room without active war rejects",
			setup: func(t *testing.T, world *state.World) {
				t.Helper()
				if err := world.UpdateRoomProperty("room:temple", "RNOKIL", "true"); err != nil {
					t.Fatalf("UpdateRoomProperty() error = %v", err)
				}
				_, _ = world.UpdateCreatureTags("creature:alice", []string{"PCHAOS"}, nil)
				_, _ = world.UpdateCreatureTags("creature:bob", []string{"PCHAOS"}, nil)
			},
			want: "\n이 방에선 전투가 금지되있습니다.\n",
		},
		{
			name: "low-level caster cannot attack far higher player",
			setup: func(t *testing.T, world *state.World) {
				t.Helper()
				_, _ = world.UpdateCreatureTags("creature:alice", []string{"PCHAOS"}, nil)
				_, _ = world.UpdateCreatureTags("creature:bob", []string{"PCHAOS"}, nil)
				_ = world.SetCreatureStat("creature:alice", "level", 5)
				_ = world.SetCreatureStat("creature:bob", "level", 25)
			},
			want: "\n!! 좋아요.. 죽을려면 무슨짓을 못하겠어요. !!\n",
		},
		{
			name: "lawful caster outside survival rejects",
			want: "\n미안하지만 당신은 선합니다.\n",
		},
		{
			name: "chaotic caster cannot attack lawful player",
			setup: func(t *testing.T, world *state.World) {
				t.Helper()
				_, _ = world.UpdateCreatureTags("creature:alice", []string{"PCHAOS"}, nil)
			},
			want: "\n미안하지만 그사람은 선합니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world, _ := mockCastWorld(t)
			_ = world.SetCreatureStat("creature:alice", "class", legacyClassMage)
			_ = world.SetCreatureStat("creature:alice", "mpCurrent", 50)
			_ = world.SetCreatureStat("creature:bob", "hpCurrent", 80)
			_, _ = world.UpdateCreatureTags("creature:alice", []string{"SFIREB"}, nil)
			if tt.setup != nil {
				tt.setup(t, world)
			}

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewCastHandler(world, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{"화궁", "Bob"}})
			if err != nil {
				t.Fatalf("cast error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			alice, _ := world.Creature("creature:alice")
			if got := alice.Stats["mpCurrent"]; got != 50 {
				t.Fatalf("mpCurrent = %d, want unchanged 50", got)
			}
			bob, _ := world.Creature("creature:bob")
			if got := bob.Stats["hpCurrent"]; got != 80 {
				t.Fatalf("Bob hpCurrent = %d, want unchanged 80", got)
			}
		})
	}
}

func TestOffensiveSpellResolvesMonsterBeforePlayerLikeLegacy(t *testing.T) {
	_, loaded := mockCastWorld(t)
	loaded.Creatures["creature:bob-monster"] = model.Creature{
		ID:          "creature:bob-monster",
		RoomID:      "room:temple",
		DisplayName: "Bob",
		Stats: map[string]int{
			"level":        5,
			"hpCurrent":    80,
			"hpMax":        80,
			"mpCurrent":    0,
			"intelligence": 10,
			"piety":        10,
			"armor":        100,
		},
	}
	world := state.NewWorld(loaded)
	_ = world.SetCreatureStat("creature:alice", "class", legacyClassDM)
	_ = world.SetCreatureStat("creature:bob", "hpCurrent", 80)
	_, _ = world.UpdateCreatureTags("creature:alice", []string{"PCHAOS"}, nil)
	_, _ = world.UpdateCreatureTags("creature:bob", []string{"PCHAOS"}, nil)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(world, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{"화궁", "Bob"}})
	if err != nil {
		t.Fatalf("cast error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	playerBob, _ := world.Creature("creature:bob")
	if got := playerBob.Stats["hpCurrent"]; got != 80 {
		t.Fatalf("player Bob hpCurrent = %d, want unchanged 80", got)
	}
	monsterBob, _ := world.Creature("creature:bob-monster")
	if got := monsterBob.Stats["hpCurrent"]; got >= 80 {
		t.Fatalf("monster Bob hpCurrent = %d, want damage from monster-first lookup", got)
	}
}

func TestOffensiveSpellRejectsProtectedMonsterBeforeCostLikeLegacy(t *testing.T) {
	_, loaded := mockCastWorld(t)
	loaded.Creatures["creature:guardian"] = model.Creature{
		ID:          "creature:guardian",
		RoomID:      "room:temple",
		DisplayName: "수호석",
		Metadata:    model.Metadata{Tags: []string{"MUNKIL", "MMALES"}},
		Stats: map[string]int{
			"level":     5,
			"hpCurrent": 80,
			"hpMax":     80,
		},
	}
	world := state.NewWorld(loaded)
	_ = world.SetCreatureStat("creature:alice", "class", legacyClassDM)
	_ = world.SetCreatureStat("creature:alice", "mpCurrent", 50)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(world, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{"화궁", "수호석"}})
	if err != nil {
		t.Fatalf("cast error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n당신은 그를 공격할 수 없습니다.\n" {
		t.Fatalf("status/output = %d/%q, want legacy protected-monster refusal", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got := alice.Stats["mpCurrent"]; got != 50 {
		t.Fatalf("mpCurrent = %d, want unchanged 50", got)
	}
	guardian, _ := world.Creature("creature:guardian")
	if got := guardian.Stats["hpCurrent"]; got != 80 {
		t.Fatalf("guardian hpCurrent = %d, want unchanged 80", got)
	}
}

func TestOffensiveSpellAwardsRealmProficiencyAgainstMonstersLikeLegacy(t *testing.T) {
	withAttackRolls(t, 1, 1)
	_, loaded := mockCastWorld(t)
	loaded.Creatures["creature:dummy"] = model.Creature{
		ID:          "creature:dummy",
		RoomID:      "room:temple",
		DisplayName: "훈련인형",
		Stats: map[string]int{
			"level":        5,
			"hpCurrent":    100,
			"hpMax":        100,
			"experience":   250,
			"intelligence": 10,
			"piety":        10,
		},
	}
	world := state.NewWorld(loaded)
	_ = world.SetCreatureStat("creature:alice", "class", legacyClassDM)
	_ = world.SetCreatureStat("creature:alice", "mpCurrent", 50)
	_ = world.SetCreatureStat("creature:alice", "realmFire", 100)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(world, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{"화궁", "훈련"}})
	if err != nil {
		t.Fatalf("cast error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	alice, _ := world.Creature("creature:alice")
	if got := alice.Stats["realmFire"]; got != 127 {
		t.Fatalf("realmFire = %d, want 127 from C addrealm formula", got)
	}
	if got := alice.Stats["realmEarth"]; got != 0 {
		t.Fatalf("realmEarth = %d, want unchanged 0", got)
	}
	dummy, _ := world.Creature("creature:dummy")
	if got := dummy.Stats["hpCurrent"]; got != 89 {
		t.Fatalf("dummy hpCurrent = %d, want 89 after fixed 11 damage", got)
	}
}

func TestHighTierOffensiveRestrictionMessagesUseLegacyText(t *testing.T) {
	tests := []struct {
		name   string
		spell  string
		tag    string
		mp     int
		wantMP int
		want   string
	}{
		{
			name:   "sisix requires mage invincible training",
			spell:  "천지진동",
			tag:    "SISIX1",
			mp:     80,
			wantMP: 45,
			want:   "\n도술사를 무적수련한 사람만이 쓸 수 있는 마법입니다.\n",
		},
		{
			name:   "xixix requires caretaker",
			spell:  "혈사천",
			tag:    "XIXIX1",
			mp:     100,
			wantMP: 40,
			want:   "\n초인 이상만이 사용할수 있는 마법입니다.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useSpellFailRoll(t, 0)
			previousNow := timeNow
			fakeTime := int64(6000)
			timeNow = func() time.Time {
				return time.Unix(fakeTime, 0)
			}
			t.Cleanup(func() {
				timeNow = previousNow
			})
			world, _ := mockCastWorld(t)
			_ = world.SetCreatureStat("creature:alice", "class", legacyClassMage)
			_ = world.SetCreatureStat("creature:alice", "mpCurrent", tt.mp)
			_, _ = world.UpdateCreatureTags("creature:alice", []string{tt.tag, "PCHAOS"}, nil)
			_, _ = world.UpdateCreatureTags("creature:bob", []string{"PCHAOS"}, nil)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewCastHandler(world, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{tt.spell, "Bob"}})
			if err != nil {
				t.Fatalf("cast error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			alice, _ := world.Creature("creature:alice")
			if got := alice.Stats["mpCurrent"]; got != tt.wantMP {
				t.Fatalf("mpCurrent = %d, want %d after C non-self offensive pre-restriction cost", got, tt.wantMP)
			}
			if expires, ok, err := world.CreatureCooldownExpires("creature:alice", "spell"); err != nil {
				t.Fatalf("CreatureCooldownExpires() error = %v", err)
			} else if ok {
				t.Fatalf("spell cooldown = %d/%v, want no cooldown after C restriction return 0", expires, ok)
			}
		})
	}
}

func TestHighTierOffensiveSpellFailPrecedesTrainingRestrictionLikeLegacy(t *testing.T) {
	useSpellFailRoll(t, 99)
	world, _ := mockCastWorld(t)
	_ = world.SetCreatureStat("creature:alice", "class", legacyClassMage)
	_ = world.SetCreatureStat("creature:alice", "mpCurrent", 35)
	_ = world.SetCreatureStat("creature:bob", "hpCurrent", 80)
	_, _ = world.UpdateCreatureTags("creature:alice", []string{"SISIX1", "PCHAOS"}, nil)
	_, _ = world.UpdateCreatureTags("creature:bob", []string{"PCHAOS"}, nil)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(world, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{"천지진동", "Bob"}})
	if err != nil {
		t.Fatalf("cast error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want silent C spell_fail before training restriction", status, ctx.OutputString())
	}
	alice, _ := world.Creature("creature:alice")
	if got := alice.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after C 35 MP spell_fail cost", got)
	}
	bob, _ := world.Creature("creature:bob")
	if got := bob.Stats["hpCurrent"]; got != 80 {
		t.Fatalf("Bob hpCurrent = %d, want unchanged 80", got)
	}
}

func TestHighTierOffensiveSuccessIncludesLegacyCasterDetail(t *testing.T) {
	tests := []struct {
		name      string
		spell     string
		tag       string
		class     int
		mp        int
		extraTags []string
		want      string
	}{
		{
			name:      "sisix",
			spell:     "천지진동",
			tag:       "SISIX1",
			class:     legacyClassMage,
			mp:        35,
			extraTags: []string{"SMAGE"},
			want:      "천지진동주... 당신은 땅의 지맥을 건들여 적이 있는 곳의 땅이 갈라집니다.",
		},
		{
			name:  "xixix",
			spell: "탄지수통",
			tag:   "XIXIX4",
			class: legacyClassCaretaker,
			mp:    60,
			want:  "탄지수통주... 관음의 눈물이 손 끝에 맺히니 마도 무릎을 꿇으리라.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useSpellFailRoll(t, 0)
			world, _ := mockCastWorld(t)
			_ = world.SetCreatureStat("creature:alice", "class", tt.class)
			_ = world.SetCreatureStat("creature:alice", "mpCurrent", tt.mp)
			_ = world.SetCreatureStat("creature:bob", "hpCurrent", 500)
			actorTags := append([]string{tt.tag, "PCHAOS"}, tt.extraTags...)
			_, _ = world.UpdateCreatureTags("creature:alice", actorTags, nil)
			_, _ = world.UpdateCreatureTags("creature:bob", []string{"PCHAOS"}, nil)

			var broadcasts []roomBroadcastRecord
			ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
			status, err := NewCastHandler(world, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{tt.spell, "Bob"}})
			if err != nil {
				t.Fatalf("cast error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}
			if out := ctx.OutputString(); !strings.Contains(out, tt.want) || !strings.Contains(out, "\n주문이 ") {
				t.Fatalf("output = %q, want high-tier caster detail %q and damage line", out, tt.want)
			}
			if len(broadcasts) != 1 {
				t.Fatalf("broadcasts = %+v, want only initial C broadcast for high-tier offensive spell", broadcasts)
			}
			if strings.Contains(broadcasts[0].Text, tt.want) {
				t.Fatalf("broadcast = %q, want no high-tier room detail broadcast", broadcasts[0].Text)
			}
		})
	}
}

func TestOffensiveSpellMidTierSuccessIncludesLegacyCasterAndRoomDetails(t *testing.T) {
	useSpellFailRoll(t, 0)
	world, _ := mockCastWorld(t)
	_ = world.SetCreatureStat("creature:alice", "class", legacyClassMage)
	_ = world.SetCreatureStat("creature:alice", "mpCurrent", 50)
	_ = world.SetCreatureStat("creature:bob", "hpCurrent", 500)
	_, _ = world.UpdateCreatureTags("creature:alice", []string{"SBURNS", "PCHAOS"}, nil)
	_, _ = world.UpdateCreatureTags("creature:bob", []string{"PCHAOS"}, nil)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := NewCastHandler(world, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{"화선도", "Bob"}})
	if err != nil {
		t.Fatalf("cast error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	output := ctx.OutputString()
	if !strings.Contains(output, "화기선주... 북두의 화두성이 깃발에 실리니 이것으로 모든 것을 태우리라.") ||
		!strings.Contains(output, "등에 숨겨져 있던 붉은 깃발들이 하늘로 날아올라 진을 형성하자 적의 몸이 불타 오릅니다.") ||
		!strings.Contains(output, "\n주문이 ") {
		t.Fatalf("output = %q, want C 화선도 caster detail and damage line", output)
	}
	if len(broadcasts) != 2 {
		t.Fatalf("broadcasts = %+v, want initial cast and C room detail broadcasts", broadcasts)
	}
	if !strings.Contains(broadcasts[0].Text, "Alice이 화선도 주문을 Bob에게 외웁니다.") {
		t.Fatalf("initial broadcast = %q, want C initial cast broadcast", broadcasts[0].Text)
	}
	if !strings.Contains(broadcasts[1].Text, "Alice의 등에 숨겨져 있던 붉은 깃발들이 하늘로 날아올라 진을") ||
		!strings.Contains(broadcasts[1].Text, "형성하자 적의 몸이 불타 오릅니다.") {
		t.Fatalf("detail broadcast = %q, want C 화선도 room detail", broadcasts[1].Text)
	}
}

func TestOffensiveSpellSendsTargetDamageToActiveSession(t *testing.T) {
	world, _ := mockCastWorld(t)
	_ = world.SetCreatureStat("creature:alice", "class", legacyClassDM)
	_ = world.SetCreatureStat("creature:alice", "mpCurrent", 50)
	_ = world.SetCreatureStat("creature:bob", "hpCurrent", 80)
	_, _ = world.UpdateCreatureTags("creature:alice", []string{"PCHAOS"}, nil)
	_, _ = world.UpdateCreatureTags("creature:bob", []string{"PCHAOS"}, nil)

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

	status, err := NewCastHandler(world, nil)(ctx, ResolvedCommand{CmdName: "cast", Args: []string{"화궁", "Bob"}})
	if err != nil {
		t.Fatalf("cast error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := sent["player:bob"]; ok {
		t.Fatalf("sent to player id instead of active session: %+v", sent)
	}
	bobMsg := sent["session:bob"]
	if !strings.Contains(bobMsg, "Alice이 화궁 주술로 당신에게") {
		t.Fatalf("bob message = %q, want direct damage message", bobMsg)
	}
	if strings.Contains(bobMsg, "화궁 주문을 Bob에게 외웁니다") {
		t.Fatalf("bob message = %q, want no room-cast line in direct damage message", bobMsg)
	}
}

func slicesContains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func TestEnchantTemporaryBuff(t *testing.T) {
	world, _ := mockCastWorld(t)

	// Bob is a Mage (legacyClassMage). Give Bob the SENCHA tag so he can cast 빙의.
	_, err := world.UpdateCreatureTags("creature:bob", []string{"SENCHA"}, nil)
	if err != nil {
		t.Fatalf("failed to grant enchant spell tag: %v", err)
	}

	// Create a weapon in Bob's inventory via state.World API
	weaponID := model.ObjectInstanceID("object:test_sword")
	if err := world.UpdateObjectInstance(model.ObjectInstance{
		ID:                  weaponID,
		PrototypeID:         "proto:test_sword",
		DisplayNameOverride: "목검",
		Properties: map[string]string{
			"type":         "1", // weapon
			"shotsMax":     "10",
			"shotsCurrent": "10",
			"pDice":        "2",
			"value":        "100",
		},
	}); err != nil {
		t.Fatalf("failed to add weapon object: %v", err)
	}
	if err := world.MoveObjectToCreatureInventory(weaponID, "creature:bob"); err != nil {
		t.Fatalf("failed to move weapon to bob inventory: %v", err)
	}
	_ = world.SetCreatureStat("creature:bob", "level", 12) // So adj calculation is (((12+3)/4)-5)/5 + 1 = 1

	// Create an armor in Bob's inventory via state.World API
	armorID := model.ObjectInstanceID("object:test_armor")
	if err := world.UpdateObjectInstance(model.ObjectInstance{
		ID:                  armorID,
		PrototypeID:         "proto:test_armor",
		DisplayNameOverride: "가죽갑옷",
		Properties: map[string]string{
			"type":     "5", // armor
			"armor":    "20",
			"value":    "300",
			"wearFlag": "1", // Body wear
		},
	}); err != nil {
		t.Fatalf("failed to add armor object: %v", err)
	}
	if err := world.MoveObjectToCreatureInventory(armorID, "creature:bob"); err != nil {
		t.Fatalf("failed to move armor to bob inventory: %v", err)
	}

	// Mock timeNow function
	fakeTime := int64(1000)
	timeNow = func() time.Time {
		return time.Unix(fakeTime, 0)
	}
	defer func() {
		timeNow = time.Now
	}()

	// Initialize Cast Handler
	castH := NewCastHandler(world, nil)

	// Cast enchant on the weapon (목검)
	ctxWeapon := &Context{
		ActorID: "player:bob",
	}
	_, err = castH(ctxWeapon, ResolvedCommand{
		CmdName: "cast",
		Args:    []string{"빙의", "목검"},
	})
	if err != nil {
		t.Fatalf("enchant cast on weapon failed: %v", err)
	}

	// Verify weapon properties are enchanted permanently like C OENCHA.
	objWeapon, _ := world.Object(weaponID)
	if objWeapon.Properties["adjustment"] != "1" {
		t.Errorf("expected weapon adjustment 1, got %q", objWeapon.Properties["adjustment"])
	}
	if objWeapon.Properties["shotsMax"] != "20" { // 10 + 1*10
		t.Errorf("expected weapon shotsMax 20, got %q", objWeapon.Properties["shotsMax"])
	}
	if objWeapon.Properties["pDice"] != "3" { // 2 + 1
		t.Errorf("expected weapon pDice 3, got %q", objWeapon.Properties["pDice"])
	}
	if objWeapon.Properties["value"] != "600" { // 100 + 500*1
		t.Errorf("expected weapon value 600, got %q", objWeapon.Properties["value"])
	}
	if !objectHasAnyTag(world, objWeapon, "enchanted") || !objectHasAnyTag(world, objWeapon, "OENCHA") {
		t.Errorf("expected enchanted/OENCHA tags on weapon, got %v", objWeapon.Metadata.Tags)
	}
	if _, ok := objWeapon.Properties["enchant_expire_at"]; ok {
		t.Errorf("unexpected enchant_expire_at property on C enchant: %q", objWeapon.Properties["enchant_expire_at"])
	}

	// Advance time past the cooldown (Mage = 3 seconds) for 2nd cast
	fakeTime += 10

	// Cast enchant on the armor (가죽갑옷)
	ctxArmor := &Context{
		ActorID: "player:bob",
	}
	_, err = castH(ctxArmor, ResolvedCommand{
		CmdName: "cast",
		Args:    []string{"빙의", "가죽갑옷"},
	})
	if err != nil {
		t.Fatalf("enchant cast on armor failed: %v", err)
	}

	// Verify armor properties are enchanted permanently like C OENCHA.
	objArmor, _ := world.Object(armorID)
	if objArmor.Properties["adjustment"] != "1" {
		t.Errorf("expected armor adjustment 1, got %q", objArmor.Properties["adjustment"])
	}
	if objArmor.Properties["armor"] != "22" { // 20 + 1*2 (wear flag is body wear)
		t.Errorf("expected armor class 22, got %q", objArmor.Properties["armor"])
	}
	if objArmor.Properties["value"] != "800" { // 300 + 500*1
		t.Errorf("expected armor value 800, got %q", objArmor.Properties["value"])
	}
	if !objectHasAnyTag(world, objArmor, "enchanted") || !objectHasAnyTag(world, objArmor, "OENCHA") {
		t.Errorf("expected enchanted/OENCHA tags on armor, got %v", objArmor.Metadata.Tags)
	}
	if _, ok := objArmor.Properties["enchant_expire_at"]; ok {
		t.Errorf("unexpected enchant_expire_at property on C enchant: %q", objArmor.Properties["enchant_expire_at"])
	}
	updatedBob, _ := world.Creature("creature:bob")
	if updatedBob.Properties["dailyEnchantCur"] != "8" {
		t.Errorf("dailyEnchantCur = %q, want 8 after two enchants", updatedBob.Properties["dailyEnchantCur"])
	}
	if got := creatureStat(updatedBob, "mpCurrent"); got != 0 {
		t.Errorf("mpCurrent = %d, want 0 after two C enchant costs", got)
	}
}

// TestMagicLearnedSpellEnforcement verifies central cast learned check (P0-2) matching C S_ISSET.
func TestMagicLearnedSpellEnforcement(t *testing.T) {
	world, _ := mockCastWorld(t)
	// Alice is cleric, no spells learned yet
	castH := NewCastHandler(world, nil)
	ctx := &Context{ActorID: "player:alice"}

	// Should fail learned check (no SVIGOR tag)
	status, err := castH(ctx, ResolvedCommand{CmdName: "cast", Args: []string{"회복"}})
	if err != nil {
		t.Fatalf("cast err: %v", err)
	}
	if status != StatusDefault {
		t.Error("expected default status")
	}
	// Output should contain not learned msg (from new central check)
	out := strings.Join(ctx.Output, "")
	if !strings.Contains(out, "터득하지 못했습니다") {
		t.Errorf("expected learned enforcement msg, got: %s", out)
	}

	// Grant the spell tag (as if studied)
	_, _ = world.UpdateCreatureTags("creature:alice", []string{"SVIGOR"}, nil)
	_, _ = world.UpdatePlayerTags("player:alice", []string{"SVIGOR"}, nil)

	// Now MP low, but learned check passes (will hit MP 부족 or other)
	_ = world.SetCreatureStat("creature:alice", "mpCurrent", 0)
	ctx2 := &Context{ActorID: "player:alice"}
	status, _ = castH(ctx2, ResolvedCommand{CmdName: "cast", Args: []string{"회복"}})
	// Should reach MP check now
	out2 := strings.Join(ctx2.Output, "")
	if !strings.Contains(out2, "도력이 부족") {
		t.Errorf("after learn, expected MP msg not learned msg, got: %s", out2)
	}
}

// TestHighTierSpellsRegistered verifies sisix/xixix now castable (was missing from supported list).
func TestHighTierSpellsRegistered(t *testing.T) {
	// Just ensure byPower finds them (prevents regression)
	for _, p := range []int{magicPowerSisix1, magicPowerXixix4} {
		sp, ok := castSpellByPower(p)
		if !ok || sp.power != p {
			t.Errorf("high tier power %d not registered in cast spells", p)
		}
	}
}

// TestSpellFailFormula verifies port of C spell_fail rates at least for known class.
func TestSpellFailFormula(t *testing.T) {
	// Low level fighter high int should have low fail rate per formula
	f := model.Creature{Stats: map[string]int{"class": legacyClassFighter, "level": 10, "intelligence": 20}}
	// Run multiple to sample (not deterministic assert, but no panic/crash)
	for i := 0; i < 5; i++ {
		_ = spellFail(f)
	}
	// DM always false (success)
	dm := model.Creature{Stats: map[string]int{"class": legacyClassDM, "level": 100, "intelligence": 30}}
	if spellFail(dm) {
		t.Error("DM should never spellFail per C default")
	}
}

func TestMagicProficiencySubDMUsesCPrivilegedTable(t *testing.T) {
	subDM := model.Creature{Stats: map[string]int{"class": legacyClassSubDM, "realmEarth": 2048}}
	if got := mprofic(subDM, 1); got != 20 {
		t.Fatalf("sub-DM earth mprofic = %d, want 20 from C privileged table", got)
	}

	fighter := model.Creature{Stats: map[string]int{"class": legacyClassFighter, "realmFire": 2048}}
	if got := mprofic(fighter, 3); got != 10 {
		t.Fatalf("fighter fire mprofic = %d, want 10 from C default table", got)
	}
	if got := mprofic(fighter, 1); got != 0 {
		t.Fatalf("fighter earth mprofic = %d, want 0 without realmEarth", got)
	}

	missile := model.Creature{
		Stats:      map[string]int{"class": legacyClassSubDM},
		Properties: map[string]string{"proficiency/missile": "2048"},
	}
	if got := mprofic(missile, 0); got != 20 {
		t.Fatalf("sub-DM missile mprofic = %d, want 20 from legacy proficiency property", got)
	}
}

// TestCurseCostAndFail verifies magic6.c curse/remove-curse CAST costs.
func TestCurseCostAndFail(t *testing.T) {
	// Cost in table
	for _, s := range supportedCastSpells {
		if s.power == magicPowerRemoveCurse && s.cost != 18 {
			t.Errorf("remove-curse cost should be 18 to match C, got %d", s.cost)
		}
		if s.power == magicPowerCurse && s.cost != 25 {
			t.Errorf("curse cost should be 25 to match C, got %d", s.cost)
		}
	}
}
