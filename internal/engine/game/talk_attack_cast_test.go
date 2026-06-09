package game

import (
	"context"
	"strings"
	"testing"
	"time"

	enginecmd "muhan/internal/engine/command"
	"muhan/internal/session"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestTalkAttackPrimesAutomaticCombatState(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	now := time.Now().Unix()
	if err := world.SetCreatureCooldown("creature:wise", "attack", now, 60); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}
	root := talkActionTestRoot(t, "계석치무", 25, "죽어 ATTACK\n건방지군.\n")
	loop := talkAttackCastLoop(t, world, root)
	alice := make(chan session.Command, 6)
	bob := make(chan session.Command, 6)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 죽어 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	enemies, err := world.CreatureEnemies("creature:wise")
	if err != nil {
		t.Fatalf("CreatureEnemies() error = %v", err)
	}
	if !talkStringListContains(enemies, "Alice") {
		t.Fatalf("wise enemies = %+v, want Alice", enemies)
	}
	wise, ok := world.Creature("creature:wise")
	if !ok {
		t.Fatal("Creature(wise) missing")
	}
	if !talkMetadataHasTag(wise.Metadata, "was_attacked") {
		t.Fatalf("wise tags = %+v, want was_attacked combat primer", wise.Metadata.Tags)
	}
	if remaining, usable, err := world.UseCreatureCooldown("creature:wise", "attack", now, 2); err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	} else if !usable {
		t.Fatalf("attack cooldown still active for %d seconds; want expired for automatic combat", remaining)
	}
}

func TestTalkCastRuntimeHookHandlesUnsupportedSpell(t *testing.T) {
	world := &talkCastHookWorld{World: state.NewWorld(talkTestWorld(t))}
	root := talkActionTestRoot(t, "계석치무", 25, "비전 CAST arcane PLAYER\n새겨졌네.\n")
	loop := talkAttackCastLoop(t, world, root)
	alice := make(chan session.Command, 6)
	bob := make(chan session.Command, 6)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 비전 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	if len(world.calls) != 1 {
		t.Fatalf("CastTalkSpell calls = %+v, want one call", world.calls)
	}
	call := world.calls[0]
	if call.casterID != "creature:wise" || call.targetID != "creature:alice" || call.playerID != "player:alice" || call.spell != "arcane" {
		t.Fatalf("CastTalkSpell call = %+v, want wise -> alice/player:alice arcane", call)
	}
	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"비전\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"새겨졌네.\"라고 이야기합니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 arcane 주문을 겁니다.\n"})
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"새겨졌네.\"라고 이야기합니다.\n\n계석치무가 당신에게 arcane 주문을 겁니다.\n"})
	aliceCreature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("Creature(alice) missing")
	}
	if !talkMetadataHasTag(aliceCreature.Metadata, "hooked_cast") {
		t.Fatalf("alice tags = %+v, want hooked_cast", aliceCreature.Metadata.Tags)
	}
}

func TestTalkCastRuntimeHookDoesNotInterceptOffensiveSpellLikeLegacy(t *testing.T) {
	loaded := talkTestWorld(t)
	aliceCreature := loaded.Creatures["creature:alice"]
	aliceCreature.Stats["hpCurrent"] = 80
	aliceCreature.Stats["hpMax"] = 80
	loaded.Creatures[aliceCreature.ID] = aliceCreature
	wise := loaded.Creatures["creature:wise"]
	wise.Stats = map[string]int{"mpCurrent": 15}
	loaded.Creatures[wise.ID] = wise
	world := &talkCastHookWorld{World: state.NewWorld(loaded)}
	root := talkActionTestRoot(t, "계석치무", 25, "벼락 CAST 뇌전 PLAYER\n혼나봐라.\n")
	loop := talkAttackCastLoop(t, world, root)
	alice := make(chan session.Command, 6)
	bob := make(chan session.Command, 6)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 벼락 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	if len(world.calls) != 0 {
		t.Fatalf("CastTalkSpell calls = %+v, want none for offensive spell like C talk_action", world.calls)
	}
	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"벼락\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"혼나봐라.\"라고 이야기합니다.\n"})
	bobCast := <-bob
	if !strings.Contains(bobCast.Write, "계석치무가 Alice에게 뇌전 주문을 외웠습니다.") ||
		!strings.Contains(bobCast.Write, "만큼의 피해를 입힙니다.") {
		t.Fatalf("bob cast output = %q, want offensive talk cast room damage", bobCast.Write)
	}
	aliceCast := <-alice
	if !strings.Contains(aliceCast.Write, "계석치무가 당신에게 \"혼나봐라.\"라고 이야기합니다.") ||
		!strings.Contains(aliceCast.Write, "계석치무가 당신에게 뇌전 주문을 외웠습니다.") ||
		!strings.Contains(aliceCast.Write, "만큼의 상처를 입혔습니다.") {
		t.Fatalf("alice cast output = %q, want offensive talk cast target damage", aliceCast.Write)
	}
	aliceCreature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("Creature(alice) missing")
	}
	if talkMetadataHasTag(aliceCreature.Metadata, "hooked_cast") {
		t.Fatalf("alice tags = %+v, want runtime hook not applied to offensive spell", aliceCreature.Metadata.Tags)
	}
	assertTalkCreatureStat(t, world.World, "creature:wise", "mpCurrent", 0)
}

func TestTalkCastOffensiveSpellDamagesPlayerAndPrimesCombat(t *testing.T) {
	loaded := talkTestWorld(t)
	aliceCreature := loaded.Creatures["creature:alice"]
	aliceCreature.Stats["hpCurrent"] = 80
	aliceCreature.Stats["hpMax"] = 80
	loaded.Creatures[aliceCreature.ID] = aliceCreature
	wise := loaded.Creatures["creature:wise"]
	wise.Stats = map[string]int{"mpCurrent": 15}
	loaded.Creatures[wise.ID] = wise
	world := state.NewWorld(loaded)
	root := talkActionTestRoot(t, "계석치무", 25, "벼락 CAST 뇌전 PLAYER\n혼나봐라.\n")
	loop := talkAttackCastLoop(t, world, root)
	alice := make(chan session.Command, 6)
	bob := make(chan session.Command, 6)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 벼락 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"벼락\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"혼나봐라.\"라고 이야기합니다.\n"})
	bobCast := <-bob
	if !strings.Contains(bobCast.Write, "계석치무가 Alice에게 뇌전 주문을 외웠습니다.") ||
		!strings.Contains(bobCast.Write, "계석치무가 Alice에게") ||
		!strings.Contains(bobCast.Write, "만큼의 피해를 입힙니다.") {
		t.Fatalf("bob cast output = %q, want offensive talk cast room damage", bobCast.Write)
	}
	aliceCast := <-alice
	if !strings.Contains(aliceCast.Write, "계석치무가 당신에게 \"혼나봐라.\"라고 이야기합니다.") ||
		!strings.Contains(aliceCast.Write, "계석치무가 당신에게 뇌전 주문을 외웠습니다.") ||
		!strings.Contains(aliceCast.Write, "만큼의 상처를 입혔습니다.") {
		t.Fatalf("alice cast output = %q, want offensive talk cast target damage", aliceCast.Write)
	}

	aliceCreature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("Creature(alice) missing")
	}
	if got := aliceCreature.Stats["hpCurrent"]; got >= 80 || got <= 30 {
		t.Fatalf("alice hpCurrent = %d, want damaged but alive", got)
	}
	wiseEnemies, err := world.CreatureEnemies("creature:wise")
	if err != nil {
		t.Fatalf("CreatureEnemies(wise) error = %v", err)
	}
	if !talkStringListContains(wiseEnemies, "Alice") {
		t.Fatalf("wise enemies = %+v, want Alice", wiseEnemies)
	}
	aliceEnemies, err := world.CreatureEnemies("creature:alice")
	if err != nil {
		t.Fatalf("CreatureEnemies(alice) error = %v", err)
	}
	if !talkStringListContains(aliceEnemies, "계석치무") {
		t.Fatalf("alice enemies = %+v, want 계석치무", aliceEnemies)
	}
	wise, ok = world.Creature("creature:wise")
	if !ok {
		t.Fatal("Creature(wise) missing")
	}
	if got := wise.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("wise mpCurrent = %d, want 0 after C SLGHTN cost", got)
	}
	if !talkMetadataHasTag(wise.Metadata, "was_attacked") {
		t.Fatalf("wise tags = %+v, want was_attacked combat primer", wise.Metadata.Tags)
	}
}

func TestTalkCastOffensiveSpellUsesLegacyOspellDiceColumns(t *testing.T) {
	loaded := talkTestWorld(t)
	aliceCreature := loaded.Creatures["creature:alice"]
	aliceCreature.Stats["hpCurrent"] = 80
	aliceCreature.Stats["hpMax"] = 80
	loaded.Creatures[aliceCreature.ID] = aliceCreature
	wise := loaded.Creatures["creature:wise"]
	wise.Stats = map[string]int{"mpCurrent": 3}
	loaded.Creatures[wise.ID] = wise
	world := state.NewWorld(loaded)
	root := talkActionTestRoot(t, "계석치무", 25, "화염 CAST 화선도 PLAYER\n뜨겁지.\n")
	loop := talkAttackCastLoop(t, world, root)
	alice := make(chan session.Command, 6)
	bob := make(chan session.Command, 6)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 화염 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	aliceCreature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("Creature(alice) missing")
	}
	applied := 80 - aliceCreature.Stats["hpCurrent"]
	if applied < 2 || applied > 8 {
		t.Fatalf("talk offensive damage = %d, want C SBURNS 1d7+1 range [2,8]", applied)
	}
	assertTalkCreatureStat(t, world, "creature:wise", "mpCurrent", 0)
}

func TestTalkCastOffensiveSpellInsufficientMPUsesLegacyApology(t *testing.T) {
	loaded := talkTestWorld(t)
	aliceCreature := loaded.Creatures["creature:alice"]
	aliceCreature.Stats["hpCurrent"] = 80
	aliceCreature.Stats["hpMax"] = 80
	loaded.Creatures[aliceCreature.ID] = aliceCreature
	wise := loaded.Creatures["creature:wise"]
	wise.Stats = map[string]int{"mpCurrent": 2}
	loaded.Creatures[wise.ID] = wise
	world := state.NewWorld(loaded)
	root := talkActionTestRoot(t, "계석치무", 25, "화염 CAST 화선도 PLAYER\n뜨겁지.\n")
	loop := talkAttackCastLoop(t, world, root)
	alice := make(chan session.Command, 6)
	bob := make(chan session.Command, 6)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 화염 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"화염\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"뜨겁지.\"라고 이야기합니다.\n"})
	assertNoCommand(t, bob)
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"뜨겁지.\"라고 이야기합니다.\n\n계석치무가 지금은 당신에게 주문을 걸어줄 수 없다고 사과합니다.\n"})

	aliceCreature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("Creature(alice) missing")
	}
	if got := aliceCreature.Stats["hpCurrent"]; got != 80 {
		t.Fatalf("alice hpCurrent = %d, want unchanged after insufficient talk offensive MP", got)
	}
	assertTalkCreatureStat(t, world, "creature:wise", "mpCurrent", 2)
	wiseEnemies, err := world.CreatureEnemies("creature:wise")
	if err != nil {
		t.Fatalf("CreatureEnemies(wise) error = %v", err)
	}
	if len(wiseEnemies) != 0 {
		t.Fatalf("wise enemies = %+v, want none after insufficient talk offensive MP", wiseEnemies)
	}
	aliceEnemies, err := world.CreatureEnemies("creature:alice")
	if err != nil {
		t.Fatalf("CreatureEnemies(alice) error = %v", err)
	}
	if len(aliceEnemies) != 0 {
		t.Fatalf("alice enemies = %+v, want none after insufficient talk offensive MP", aliceEnemies)
	}
}

func TestTalkCastRefusesNonOffensiveSpellWhenCasterAlreadyHatesPlayer(t *testing.T) {
	loaded := talkTestWorld(t)
	wise := loaded.Creatures["creature:wise"]
	wise.Stats = map[string]int{"mpCurrent": 20}
	loaded.Creatures[wise.ID] = wise
	world := state.NewWorld(loaded)
	if _, err := world.AddEnemy("creature:wise", "creature:alice"); err != nil {
		t.Fatalf("AddEnemy() error = %v", err)
	}
	root := talkActionTestRoot(t, "계석치무", 25, "축복 CAST bless\n축복하네.\n")
	loop := talkAttackCastLoop(t, world, root)
	alice := make(chan session.Command, 6)
	bob := make(chan session.Command, 6)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 축복 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"축복\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"축복하네.\"라고 이야기합니다.\n"})
	assertNoCommand(t, bob)
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"축복하네.\"라고 이야기합니다.\n\n계석치무가 당신에게 어떤 주문을 거는것을 거부했습니다.\n"})
	aliceCreature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("Creature(alice) missing")
	}
	if talkMetadataHasTag(aliceCreature.Metadata, "blessed") {
		t.Fatalf("alice tags = %+v, want no blessed tag after enemy refusal", aliceCreature.Metadata.Tags)
	}
	assertTalkCreatureStat(t, world, "creature:wise", "mpCurrent", 20)
}

func TestTalkCastEnemyCheckIgnoresInternalIDsLikeLegacy(t *testing.T) {
	loaded := talkTestWorld(t)
	wise := loaded.Creatures["creature:wise"]
	wise.Stats = map[string]int{"mpCurrent": 20}
	loaded.Creatures[wise.ID] = wise
	world := &talkInternalIDEnemyWorld{
		World:   state.NewWorld(loaded),
		enemies: []string{"player:alice", "creature:alice"},
	}
	root := talkActionTestRoot(t, "계석치무", 25, "축복 CAST bless\n축복하네.\n")
	loop := talkAttackCastLoop(t, world, root)
	alice := make(chan session.Command, 6)
	bob := make(chan session.Command, 6)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 축복 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"축복\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"축복하네.\"라고 이야기합니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 bless 주문을 겁니다.\n"})
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"축복하네.\"라고 이야기합니다.\n\n계석치무가 당신에게 bless 주문을 겁니다.\n"})
	assertTalkCreatureTag(t, world.World, "creature:alice", "blessed")
	assertTalkPlayerTag(t, world.World, "player:alice", "blessed")
	assertTalkCreatureStat(t, world.World, "creature:wise", "mpCurrent", 10)
}

type talkInternalIDEnemyWorld struct {
	*state.World
	enemies []string
}

func (w *talkInternalIDEnemyWorld) CreatureEnemies(creatureID model.CreatureID) ([]string, error) {
	if creatureID != "creature:wise" {
		return w.World.CreatureEnemies(creatureID)
	}
	return append([]string(nil), w.enemies...), nil
}

type talkCastHookCall struct {
	casterID model.CreatureID
	targetID model.CreatureID
	playerID model.PlayerID
	spell    string
}

type talkCastHookWorld struct {
	*state.World
	calls []talkCastHookCall
}

func (w *talkCastHookWorld) CastTalkSpell(caster model.Creature, target model.Creature, player model.Player, spell string) (bool, error) {
	w.calls = append(w.calls, talkCastHookCall{
		casterID: caster.ID,
		targetID: target.ID,
		playerID: player.ID,
		spell:    spell,
	})
	_, err := w.UpdateCreatureTags(target.ID, []string{"hooked_cast"}, nil)
	return true, err
}

func talkAttackCastLoop(t *testing.T, world TalkWorld, root string) *Loop {
	t.Helper()
	return NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
}

func talkMetadataHasTag(metadata model.Metadata, tag string) bool {
	for _, got := range metadata.Tags {
		if talkTagMatches(got, tag) {
			return true
		}
	}
	return false
}
