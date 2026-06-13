package game

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	enginecmd "github.com/0xc0de1ab/muhan/internal/engine/command"
	"github.com/0xc0de1ab/muhan/internal/session"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestTalkHandlerTalksWithRoomNPC(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	charlie := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", charlie, "player:charlie")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무와 이야기를 합니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"생명의 나무는 아직 안전합니다.\"라고 이야기합니다.\n"})
	assertNoCommand(t, charlie)
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"생명의 나무는 아직 안전합니다.\"라고 이야기합니다.\n"})
}

func TestTalkHandlerRevealsHiddenActorLikeLegacy(t *testing.T) {
	loaded := talkTestWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = append(alice.Metadata.Tags, "hidden", "PHIDDN")
	if alice.Stats == nil {
		alice.Stats = map[string]int{}
	}
	alice.Stats["PHIDDN"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = append(player.Metadata.Tags, "hidden", "PHIDDN")
	loaded.Players[player.ID] = player

	world := state.NewWorld(loaded)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandler(world),
		},
	})
	out := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", out, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	updatedCreature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("Creature(alice) missing")
	}
	if creatureHasNormalizedFlag(updatedCreature, "hidden", "PHIDDN") || updatedCreature.Stats["PHIDDN"] != 0 {
		t.Fatalf("creature hidden state = tags:%+v stats:%+v", updatedCreature.Metadata.Tags, updatedCreature.Stats)
	}
	updatedPlayer, ok := world.Player("player:alice")
	if !ok {
		t.Fatal("Player(alice) missing")
	}
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "hidden", "PHIDDN") {
		t.Fatalf("player hidden tags = %+v", updatedPlayer.Metadata.Tags)
	}
}

func TestTalkHandlerSupportsCommandFirstAndMissingCases(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "대화"}); err != nil {
		t.Fatalf("HandleEvent() missing arg error = %v", err)
	}
	assertCommand(t, alice, session.Command{Write: "누구에게 이야기하시려구요?\n"})

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "대화 없는이"}); err != nil {
		t.Fatalf("HandleEvent() missing target error = %v", err)
	}
	assertCommand(t, alice, session.Command{Write: "그런 것은 여기 없습니다.\n"})

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "대화 수호석"}); err != nil {
		t.Fatalf("HandleEvent() command-first error = %v", err)
	}
	assertCommand(t, alice, session.Command{Write: "\n수호석은 단지 당신을 멍하니 바라봅니다.\n"})
}

func TestLoadTalkFileEntryPreservesLegacyBlankResponsePairing(t *testing.T) {
	root := talkActionTestRoot(t, "계석치무", 25,
		"첫\n첫 응답\n"+
			"빈\n\n"+
			"다음 ACTION 미소\n다음 응답\n",
	)
	creature := model.Creature{DisplayName: "계석치무", Level: 25}

	blank, loaded, ok, err := loadTalkFileEntry(root, model.Room{}, creature, "빈")
	if err != nil {
		t.Fatalf("load blank response entry: %v", err)
	}
	if !loaded || !ok {
		t.Fatalf("blank response loaded/ok = %v/%v, want true/true", loaded, ok)
	}
	if blank.Response != "" {
		t.Fatalf("blank response = %q, want empty string", blank.Response)
	}

	next, loaded, ok, err := loadTalkFileEntry(root, model.Room{}, creature, "다음")
	if err != nil {
		t.Fatalf("load entry after blank response: %v", err)
	}
	if !loaded || !ok {
		t.Fatalf("next entry loaded/ok = %v/%v, want true/true", loaded, ok)
	}
	if next.Response != "다음 응답" {
		t.Fatalf("next response = %q, want C two-line pairing", next.Response)
	}
	if next.Action != (talkFileAction{Type: "ACTION", Name: "미소"}) {
		t.Fatalf("next action = %+v, want ACTION 미소", next.Action)
	}
}

func TestTalkHandlerRejectsSingleByteTargetLikeLegacyFindCrt(t *testing.T) {
	loaded := talkTestWorld(t)
	mustAddLoopCreature(t, loaded, model.Creature{
		ID:          "creature:a",
		Kind:        model.CreatureKindNPC,
		DisplayName: "ab",
		RoomID:      "room:one",
		Properties:  map[string]string{"legacyTalk": "hello"},
		Metadata:    model.Metadata{Tags: []string{"talks"}},
	})
	world := state.NewWorld(loaded)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "a 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "그런 것은 여기 없습니다.\n"})
}

func TestTalkHandlerRejectsCreatureIDTargetLikeLegacyFindCrt(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "creature:wise 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "그런 것은 여기 없습니다.\n"})
}

func TestTalkHandlerMatchesSlashKeyAliasLikeLegacyKey(t *testing.T) {
	loaded := talkTestWorld(t)
	mustAddLoopCreature(t, loaded, model.Creature{
		ID:          "creature:archivist",
		Kind:        model.CreatureKindNPC,
		DisplayName: "기록관",
		RoomID:      "room:one",
		Properties: map[string]string{
			"key/1":      "고문관",
			"legacyTalk": "부르셨습니까.",
		},
		Metadata: model.Metadata{Tags: []string{"talks"}},
	})
	world := state.NewWorld(loaded)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "고문관 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "\n기록관이 당신에게 \"부르셨습니까.\"라고 이야기합니다.\n"})
}

func TestTalkHandlerInteractiveTopicFallsBackToShrugWhenTalkFileLacksKey(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	root := talkTestRoot(t)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 모름 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"모름\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 어깨를 으쓱 거립니다.\n"})
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 어깨를 으쓱 거립니다.\n"})
}

func TestTalkHandlerInteractiveTopicWithTalkFlagButNoTalkFileDoesNothing(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 임무 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertNoCommand(t, alice)
	assertNoCommand(t, bob)
}

func TestTalkHandlerMissingTalkFileReturnsPromptLikeLegacy(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	ctx := &enginecmd.Context{
		SessionID: "s1",
		ActorID:   "player:alice",
		Values: map[string]any{
			ContextActiveSessionsKey: func() []ActiveSession {
				return []ActiveSession{{ID: "s1", ActorID: "player:alice"}}
			},
			ContextSendToSessionKey: func(session.ID, session.Command) error {
				return nil
			},
		},
	}

	status, err := NewTalkHandler(world)(ctx, enginecmd.ResolvedCommand{
		Input: "계석치무 임무 대화",
		Args:  []string{"계석치무", "임무"},
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if status != enginecmd.StatusPrompt {
		t.Fatalf("status = %v, want StatusPrompt", status)
	}
	if got := ctx.OutputString(); got != "" {
		t.Fatalf("output = %q, want none", got)
	}
}

func TestTalkHandlerAnswersInteractiveTopicFromTalkFile(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	root := talkTestRoot(t)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 임무 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"임무\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"타버린 두루마기를 찾아오게.\"라고 이야기합니다.\n"})
	assertCommand(t, bob, session.Command{Write: "계석치무가 밝은 미소를 짓습니다.\n"})
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"타버린 두루마기를 찾아오게.\"라고 이야기합니다.\n계석치무가 밝은 미소를 짓습니다.\n"})
}

func TestTalkHandlerUsesLegacySingleWordTopicKey(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	root := talkTestRoot(t)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 임무 자세히 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"임무\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"타버린 두루마기를 찾아오게.\"라고 이야기합니다.\n"})
	assertCommand(t, bob, session.Command{Write: "계석치무가 밝은 미소를 짓습니다.\n"})
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"타버린 두루마기를 찾아오게.\"라고 이야기합니다.\n계석치무가 밝은 미소를 짓습니다.\n"})
}

func TestTalkHandlerExecutesTargetedActionFromTalkFile(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	root := talkTestRoot(t)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 안녕 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"안녕\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"반갑네.\"라고 이야기합니다.\n"})
	assertCommand(t, bob, session.Command{Write: "계석치무가 Alice에게 인사를 합니다. \"안녕하세요~\"\n"})
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"반갑네.\"라고 이야기합니다.\n계석치무가 당신에게 인사를 합니다. \"안녕하세요~\"\n"})
}

func TestTalkHandlerIgnoresTalkFileEntryWithoutTalkFlag(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	root := talkTestRoot(t)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "상인 안녕 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "\n상인은 단지 당신을 멍하니 바라봅니다.\n"})
}

func TestTalkHandlerUsesRoomOwnerTalkFileCandidate(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	root := talkTestRoot(t)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "가게상인 안녕 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "\n가게상인이 당신에게 \"방 주인 응답.\"라고 이야기합니다.\n가게상인이 밝은 미소를 짓습니다.\n"})
}

func TestTalkHandlerUsesSlashKeyAliasTalkFileCandidate(t *testing.T) {
	loaded := talkTestWorld(t)
	mustAddLoopCreature(t, loaded, model.Creature{
		ID:          "creature:hidden-merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "별칭상인",
		Level:       37,
		RoomID:      "room:one",
		Properties:  map[string]string{"key/1": "은둔 상인"},
		Metadata:    model.Metadata{Tags: []string{"talks"}},
	})
	world := state.NewWorld(loaded)
	root := t.TempDir()
	dir := filepath.Join(root, "objmon", "talk")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	filename, ok := legacyTalkFilename("은둔 상인", 37)
	if !ok {
		t.Fatal("legacyTalkFilename() failed")
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte("안녕 ACTION 미소\n왔군.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "별칭상인 안녕 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, alice, session.Command{Write: "\n별칭상인이 당신에게 \"왔군.\"라고 이야기합니다.\n별칭상인이 밝은 미소를 짓습니다.\n"})
}

func TestTalkHandlerExecutesAttackFromTalkFile(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	root := talkActionTestRoot(t, "계석치무", 25, "죽어 ATTACK\n건방지군.\n")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 죽어 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"죽어\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"건방지군.\"라고 이야기합니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice를 공격합니다.\n"})
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"건방지군.\"라고 이야기합니다.\n\n계석치무가 당신을 공격합니다.\n"})
	assertTalkCreatureStat(t, world, "creature:alice", "hpCurrent", 30)
	enemies, err := world.CreatureEnemies("creature:wise")
	if err != nil {
		t.Fatalf("CreatureEnemies() error = %v", err)
	}
	if !talkStringListContains(enemies, "Alice") {
		t.Fatalf("wise enemies = %+v, want Alice", enemies)
	}
}

func TestTalkHandlerCastsMappedSpellFromTalkFile(t *testing.T) {
	loaded := talkTestWorld(t)
	wise := loaded.Creatures["creature:wise"]
	wise.Stats = map[string]int{"mpCurrent": 20}
	loaded.Creatures[wise.ID] = wise

	world := state.NewWorld(loaded)
	root := talkActionTestRoot(t, "계석치무", 25, "축복 CAST bless\n축복하네.\n")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 축복 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"축복\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"축복하네.\"라고 이야기합니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 bless 주문을 겁니다.\n"})
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"축복하네.\"라고 이야기합니다.\n\n계석치무가 당신에게 bless 주문을 겁니다.\n"})
	assertTalkCreatureTag(t, world, "creature:alice", "blessed")
	assertTalkPlayerTag(t, world, "player:alice", "blessed")
	assertTalkCreatureStat(t, world, "creature:wise", "mpCurrent", 10)
}

func TestTalkHandlerMappedSpellWithoutMPSendsLegacyApologyOnlyToPlayer(t *testing.T) {
	tests := []struct {
		name          string
		casterStats   map[string]int
		wantMPCurrent *int
	}{
		{name: "low mp", casterStats: map[string]int{"mpCurrent": 5}, wantMPCurrent: intPtr(5)},
		{name: "missing mp", casterStats: nil, wantMPCurrent: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := talkTestWorld(t)
			wise := loaded.Creatures["creature:wise"]
			wise.Stats = tt.casterStats
			loaded.Creatures[wise.ID] = wise

			world := state.NewWorld(loaded)
			root := talkActionTestRoot(t, "계석치무", 25, "축복 CAST bless\n축복하네.\n")
			loop := NewLoop(enginecmd.Dispatcher{
				Registry: socialRegistry(t),
				Handlers: map[string]enginecmd.Handler{
					"talk": NewTalkHandlerWithRoot(world, root),
				},
			})
			alice := make(chan session.Command, 4)
			bob := make(chan session.Command, 4)
			registerTestSession(t, loop, "s1", alice, "player:alice")
			registerTestSession(t, loop, "s2", bob, "player:bob")

			if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 축복 대화"}); err != nil {
				t.Fatalf("HandleEvent() error = %v", err)
			}

			assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"축복\"에 관해 물어봅니다.\n"})
			assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"축복하네.\"라고 이야기합니다.\n"})
			assertNoCommand(t, bob)
			assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"축복하네.\"라고 이야기합니다.\n\n계석치무가 지금은 당신에게 주문을 걸어줄 수 없다고 사과합니다.\n"})

			aliceCreature, ok := world.Creature("creature:alice")
			if !ok {
				t.Fatal("Creature(alice) missing")
			}
			if talkMetadataHasTag(aliceCreature.Metadata, "blessed") {
				t.Fatalf("alice creature tags = %+v, want no bless on failed talk cast", aliceCreature.Metadata.Tags)
			}
			alicePlayer, ok := world.Player("player:alice")
			if !ok {
				t.Fatal("Player(alice) missing")
			}
			if talkMetadataHasTag(alicePlayer.Metadata, "blessed") {
				t.Fatalf("alice player tags = %+v, want no bless on failed talk cast", alicePlayer.Metadata.Tags)
			}
			wise, ok = world.Creature("creature:wise")
			if !ok {
				t.Fatal("Creature(wise) missing")
			}
			if tt.wantMPCurrent == nil {
				if _, ok := wise.Stats["mpCurrent"]; ok {
					t.Fatalf("wise mpCurrent = %d, want missing", wise.Stats["mpCurrent"])
				}
			} else if got := wise.Stats["mpCurrent"]; got != *tt.wantMPCurrent {
				t.Fatalf("wise mpCurrent = %d, want %d", got, *tt.wantMPCurrent)
			}
		})
	}
}

func TestTalkHandlerIgnoresUnsupportedCastLikeLegacy(t *testing.T) {
	world := state.NewWorld(talkTestWorld(t))
	root := talkActionTestRoot(t, "계석치무", 25, "운석 CAST meteor\n그 주문은 어렵군.\n")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"talk": NewTalkHandlerWithRoot(world, root),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "계석치무 운석 대화"}); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 계석치무에게 \"운석\"에 관해 물어봅니다.\n"})
	assertCommand(t, bob, session.Command{Write: "\n계석치무가 Alice에게 \"그 주문은 어렵군.\"라고 이야기합니다.\n"})
	assertNoCommand(t, bob)
	assertCommand(t, alice, session.Command{Write: "\n계석치무가 당신에게 \"그 주문은 어렵군.\"라고 이야기합니다.\n"})
}

func talkTestWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := socialWorld(t)
	mustAddLoopCreature(t, loaded, model.Creature{
		ID:          "creature:wise",
		Kind:        model.CreatureKindNPC,
		DisplayName: "계석치무",
		Level:       25,
		RoomID:      "room:one",
		Properties:  map[string]string{"legacyTalk": "생명의 나무는 아직 안전합니다."},
		Metadata:    model.Metadata{Tags: []string{"talks"}},
	})
	mustAddLoopCreature(t, loaded, model.Creature{
		ID:          "creature:stone",
		Kind:        model.CreatureKindNPC,
		DisplayName: "수호석",
		RoomID:      "room:one",
	})
	mustAddLoopCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		Level:       36,
		RoomID:      "room:one",
		Properties:  map[string]string{"keywords": "상점 주인"},
	})
	mustAddLoopCreature(t, loaded, model.Creature{
		ID:          "creature:room-owner",
		Kind:        model.CreatureKindNPC,
		DisplayName: "가게상인",
		Level:       36,
		RoomID:      "room:one",
		Metadata:    model.Metadata{Tags: []string{"talks"}},
	})
	return loaded
}

func talkTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "objmon", "talk")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	filename, ok := legacyTalkFilename("계석치무", 25)
	if !ok {
		t.Fatal("legacyTalkFilename() failed")
	}
	content := "임무 ACTION 미소\n타버린 두루마기를 찾아오게.\n안녕 ACTION 안녕 PLAYER\n반갑네.\n"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	filename, ok = legacyTalkFilename("상점 주인", 36)
	if !ok {
		t.Fatal("legacyTalkFilename() failed")
	}
	content = "안녕 ACTION 울어\n어서 오게.\n"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	filename, ok = legacyTalkFilename("One 주인", 36)
	if !ok {
		t.Fatal("legacyTalkFilename() failed")
	}
	content = "안녕 ACTION 미소\n방 주인 응답.\n"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func assertTalkCreatureTag(t *testing.T, world *state.World, creatureID model.CreatureID, tag string) {
	t.Helper()
	creature, ok := world.Creature(creatureID)
	if !ok {
		t.Fatalf("Creature(%q) missing", creatureID)
	}
	for _, got := range creature.Metadata.Tags {
		if talkTagMatches(got, tag) {
			return
		}
	}
	t.Fatalf("Creature(%q) tags = %+v, want %q", creatureID, creature.Metadata.Tags, tag)
}

func assertTalkPlayerTag(t *testing.T, world *state.World, playerID model.PlayerID, tag string) {
	t.Helper()
	player, ok := world.Player(playerID)
	if !ok {
		t.Fatalf("Player(%q) missing", playerID)
	}
	for _, got := range player.Metadata.Tags {
		if talkTagMatches(got, tag) {
			return
		}
	}
	t.Fatalf("Player(%q) tags = %+v, want %q", playerID, player.Metadata.Tags, tag)
}

func assertTalkCreatureStat(t *testing.T, world *state.World, creatureID model.CreatureID, key string, want int) {
	t.Helper()
	creature, ok := world.Creature(creatureID)
	if !ok {
		t.Fatalf("Creature(%q) missing", creatureID)
	}
	if got := creature.Stats[key]; got != want {
		t.Fatalf("Creature(%q).Stats[%q] = %d, want %d", creatureID, key, got, want)
	}
}

func talkStringListContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func intPtr(value int) *int {
	return &value
}
