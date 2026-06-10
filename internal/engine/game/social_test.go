package game

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"muhan/internal/commandspec"
	enginecmd "muhan/internal/engine/command"
	"muhan/internal/session"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestWhoHandlerListsActivePlayers(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"who": NewWhoHandler(world),
		},
	})
	commands := make(chan session.Command, 2)
	registerTestSession(t, loop, "s2", make(chan session.Command, 2), "player:bob")
	registerTestSession(t, loop, "s1", commands, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "누구"}); err != nil {
		t.Fatal(err)
	}

	cmd := <-commands
	if got, want := cmd.Write, "접속자:\n - Alice\n - Bob\n"; got != want {
		t.Fatalf("who output = %q, want %q", got, want)
	}
}

func TestWhoHandlerDisplaysActiveFamilyWar(t *testing.T) {
	world := namedFamilyWorld{
		World: state.NewWorld(socialWorld(t)),
		names: map[int]string{2: "은형문", 5: "무영문"},
	}
	if _, err := world.RequestFamilyWar(2, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := world.AcceptFamilyWar(5, 2); err != nil {
		t.Fatal(err)
	}
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"who": NewWhoHandler(world),
		},
	})
	commands := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", commands, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "누구"}); err != nil {
		t.Fatal(err)
	}

	cmd := <-commands
	if want := "무영문 패거리는 은형문 패거리와 전쟁중입니다."; !strings.Contains(cmd.Write, want) {
		t.Fatalf("who war output missing %q:\n%q", want, cmd.Write)
	}
}

func TestWhoisHandlerRendersActivePlayer(t *testing.T) {
	loaded := socialWorld(t)
	bob := loaded.Creatures["creature:bob"]
	bob.Level = 24
	bob.Stats["class"] = 8
	bob.Stats["race"] = 5
	bob.Stats["PMALES"] = 1
	bob.Stats["legacyAgeYears"] = 21
	loaded.Creatures["creature:bob"] = bob

	world := state.NewWorld(loaded)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"whois": NewWhoisHandler(world),
		},
	})
	commands := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", commands, "player:alice")
	registerTestSession(t, loop, "s2", make(chan session.Command, 2), "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "bob 사용자검색"}); err != nil {
		t.Fatal(err)
	}

	cmd := <-commands
	for _, want := range []string{"사용자", "성별", "[레벨]", "Bob", " 남", "[ 24 ]", "도둑", "개도둑", "21", "인간족"} {
		if !strings.Contains(cmd.Write, want) {
			t.Fatalf("whois output missing %q:\n%q", want, cmd.Write)
		}
	}
	if !strings.Contains(cmd.Write, "[ 24 ] 도둑 개도둑") || strings.Contains(cmd.Write, "[ 24 ] 도둑   개도둑") {
		t.Fatalf("whois class/title spacing should use legacy byte width:\n%q", cmd.Write)
	}
}

func TestCreatureIntValueNormalizesStatAndPropertyKeys(t *testing.T) {
	creature := model.Creature{
		Stats:      map[string]int{"LT-HOURS interval": 2 * 86400},
		Properties: map[string]string{"legacy-age-years": "23"},
	}

	if got, ok := creatureIntValue(creature, "LT_HOURS_interval"); !ok || got != 2*86400 {
		t.Fatalf("normalized stat = %d/%v, want %d/true", got, ok, 2*86400)
	}
	if got, ok := creatureIntValue(creature, "legacyAgeYears"); !ok || got != 23 {
		t.Fatalf("normalized property = %d/%v, want 23/true", got, ok)
	}
}

func TestWhoisHandlerMissingAndOfflineCases(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"whois": NewWhoisHandler(world),
		},
	})
	commands := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", commands, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "사용자검색"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, commands, session.Command{Write: "누구를 검색하시려구요?"})

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 사용자검색"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, commands, session.Command{Write: "현재 이용중이 아닙니다."})
}

func TestWhoisHandlerHonorsVisibilityFlags(t *testing.T) {
	loaded := socialWorld(t)
	bob := loaded.Creatures["creature:bob"]
	bob.Stats["PINVIS"] = 1
	loaded.Creatures["creature:bob"] = bob

	world := state.NewWorld(loaded)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"whois": NewWhoisHandler(world),
		},
	})
	commands := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", commands, "player:alice")
	registerTestSession(t, loop, "s2", make(chan session.Command, 2), "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 사용자검색"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, commands, session.Command{Write: "현재 이용중이 아닙니다."})

	alice := loaded.Creatures["creature:alice"]
	alice.Stats["PDINVI"] = 1
	loaded.Creatures["creature:alice"] = alice
	world = state.NewWorld(loaded)
	loop = NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"whois": NewWhoisHandler(world),
		},
	})
	registerTestSession(t, loop, "s1", commands, "player:alice")
	registerTestSession(t, loop, "s2", make(chan session.Command, 2), "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 사용자검색"}); err != nil {
		t.Fatal(err)
	}
	cmd := <-commands
	if !strings.Contains(cmd.Write, "Bob") || !strings.Contains(cmd.Write, "사용자") {
		t.Fatalf("whois visible output = %q", cmd.Write)
	}
}

func TestPfingerHandlerRendersActivePlayer(t *testing.T) {
	loaded := socialWorld(t)
	bob := loaded.Creatures["creature:bob"]
	bob.Stats["class"] = 8
	bob.Stats["race"] = 5
	loaded.Creatures["creature:bob"] = bob

	world := state.NewWorld(loaded)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"pfinger": NewPfingerHandler(world, t.TempDir()),
		},
	})
	commands := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", commands, "player:alice")
	registerTestSession(t, loop, "s2", make(chan session.Command, 2), "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "bOb 사용자정보"}); err != nil {
		t.Fatal(err)
	}

	cmd := <-commands
	for _, want := range []string{"Bob", "인간족", "도둑", "현재 접속 중 입니다.", "받은 편지가 없습니다."} {
		if !strings.Contains(cmd.Write, want) {
			t.Fatalf("pfinger active output missing %q:\n%q", want, cmd.Write)
		}
	}
}

func TestPfingerHandlerRendersOfflinePlayer(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "player", "temp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "player", "temp", "Offline"), []byte("player"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "player", "temp", "Legacy"), []byte("player"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded := socialWorld(t)
	mustAddLoopPlayer(t, loaded, model.Player{
		ID:          "Offline",
		DisplayName: "Offline",
		CreatureID:  "creature:offline",
		Metadata:    model.Metadata{LegacyPath: "player/temp/Offline"},
	})
	mustAddLoopCreature(t, loaded, model.Creature{
		ID:          "creature:offline",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Offline",
		PlayerID:    "Offline",
		Stats:       map[string]int{"class": 8, "race": 5, "SUICD": 1},
	})
	mustAddLoopPlayer(t, loaded, model.Player{
		ID:          "Legacy",
		DisplayName: "RecordName",
		CreatureID:  "creature:legacy",
		Metadata:    model.Metadata{LegacyPath: "player/temp/Legacy"},
	})
	mustAddLoopCreature(t, loaded, model.Creature{
		ID:          "creature:legacy",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "RecordName",
		PlayerID:    "Legacy",
		Stats:       map[string]int{"class": 8, "race": 5},
	})

	world := state.NewWorld(loaded)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"pfinger": NewPfingerHandler(world, root),
		},
	})
	commands := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", commands, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Offline 사용자정보"}); err != nil {
		t.Fatal(err)
	}
	cmd := <-commands
	for _, want := range []string{"Offline", "인간족", "도둑", "마지막 접속시간:", "그 사용자는 자살신청한 사용자입니다.", "받은 편지가 없습니다."} {
		if !strings.Contains(cmd.Write, want) {
			t.Fatalf("pfinger offline output missing %q:\n%q", want, cmd.Write)
		}
	}

	if err := os.MkdirAll(filepath.Join(root, "post"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "post", "Offline"), []byte("mail"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Offline 사용자정보"}); err != nil {
		t.Fatal(err)
	}
	cmd = <-commands
	if !strings.Contains(cmd.Write, "새 편지가 도착한 날짜:") {
		t.Fatalf("pfinger offline post output = %q", cmd.Write)
	}

	if err := os.WriteFile(filepath.Join(root, "post", "RecordName"), []byte("display-name mail"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Legacy 사용자정보"}); err != nil {
		t.Fatal(err)
	}
	cmd = <-commands
	if !strings.Contains(cmd.Write, "받은 편지가 없습니다.") || strings.Contains(cmd.Write, "새 편지가 도착한 날짜:") {
		t.Fatalf("pfinger post lookup should use command target, not record display name:\n%q", cmd.Write)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Nobody 사용자정보"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, commands, session.Command{Write: "그런 사용자는 없습니다.\n"})
}

func TestPfingerHandlerMissingAndPrivateCases(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "player", "temp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "player", "temp", "Hidden"), []byte("player"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded := socialWorld(t)
	bob := loaded.Creatures["creature:bob"]
	bob.Stats["PDMINV"] = 1
	loaded.Creatures["creature:bob"] = bob
	mustAddLoopPlayer(t, loaded, model.Player{
		ID:          "Hidden",
		DisplayName: "Hidden",
		CreatureID:  "creature:hidden",
		Metadata:    model.Metadata{LegacyPath: "player/temp/Hidden"},
	})
	mustAddLoopCreature(t, loaded, model.Creature{
		ID:          "creature:hidden",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Hidden",
		PlayerID:    "Hidden",
		Stats:       map[string]int{"class": model.ClassSubDM, "race": 5},
	})

	world := state.NewWorld(loaded)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"pfinger": NewPfingerHandler(world, root),
		},
	})
	commands := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", commands, "player:alice")
	registerTestSession(t, loop, "s2", make(chan session.Command, 2), "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "사용자정보"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, commands, session.Command{Write: "누구의 정보를 보고 싶으세요?\n"})

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 사용자정보"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, commands, session.Command{Write: "당신은 그 사용자의 정보를 볼 수 없습니다.\n"})

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Hidden 사용자정보"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, commands, session.Command{Write: "당신은 그 사용자의 정보를 볼 수 없습니다.\n"})
}

func TestPfingerHandlerIgnoresWhoisInvisibilityRules(t *testing.T) {
	loaded := socialWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats["PBLIND"] = 1
	loaded.Creatures["creature:alice"] = alice
	charlie := loaded.Creatures["creature:charlie"]
	charlie.Stats["PINVIS"] = 1
	charlie.Stats["class"] = 8
	charlie.Stats["race"] = 5
	loaded.Creatures["creature:charlie"] = charlie

	world := state.NewWorld(loaded)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"pfinger": NewPfingerHandler(world, t.TempDir()),
		},
	})
	commands := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", commands, "player:alice")
	registerTestSession(t, loop, "s2", make(chan session.Command, 2), "player:charlie")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Charlie 사용자정보"}); err != nil {
		t.Fatal(err)
	}
	cmd := <-commands
	for _, want := range []string{"Charlie", "인간족", "도둑", "현재 접속 중 입니다."} {
		if !strings.Contains(cmd.Write, want) {
			t.Fatalf("pfinger invisibility output missing %q:\n%q", want, cmd.Write)
		}
	}
}

func TestSayHandlerEchoesSenderAndSendsToSameRoom(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"say": NewSayHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	charlie := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", charlie, "player:charlie")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "안녕 말"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 \"안녕\"라고 말합니다."})
	assertNoCommand(t, charlie)
	cmd := <-alice
	if cmd.Write != "예. 좋습니다." {
		t.Fatalf("sender output = %q", cmd.Write)
	}
}

func TestSayHandlerPreservesLegacyMessageSpaces(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	setSocialCreatureStat(t, world, "creature:alice", "PLECHO", 1)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"say": NewSayHandler(world),
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "다시   말"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "\nAlice가 \"다시  \"라고 말합니다."})
	assertCommand(t, alice, session.Command{Write: "당신은 \"다시  \"라고 말합니다."})

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "끝공백 "}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "\nAlice가 \"끝공백 \"라고 말합니다."})
	assertCommand(t, alice, session.Command{Write: "당신은 \"끝공백 \"라고 말합니다."})
}

func TestSayHandlerRequiresMessage(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"say": NewSayHandler(world),
		},
	})
	commands := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", commands, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "말"}); err != nil {
		t.Fatal(err)
	}

	cmd := <-commands
	if got, want := cmd.Write, "뭘 말하고 싶으세요?"; got != want {
		t.Fatalf("say output = %q, want %q", got, want)
	}
}

func TestSayHandlerMatchesLegacyRestrictionsEchoAndReveal(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"say": NewSayHandler(world),
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	setSocialCreatureStat(t, world, "creature:alice", "PSILNC", 1)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "안녕 말"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "말을 해 보았지만 이 방 밖의 사람들은 들리지 않는듯 하군요."})
	assertNoCommand(t, bob)

	setSocialCreatureStat(t, world, "creature:alice", "PSILNC", 0)
	setSocialCreatureStat(t, world, "creature:alice", "PLECHO", 1)
	if _, err := world.UpdateCreatureTags("creature:alice", []string{"hidden"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := world.UpdatePlayerTags("player:alice", []string{"hidden"}, nil); err != nil {
		t.Fatal(err)
	}
	setSocialCreatureStat(t, world, "creature:alice", "PHIDDN", 1)

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "다시 말"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "\nAlice가 \"다시\"라고 말합니다."})
	assertCommand(t, alice, session.Command{Write: "당신은 \"다시\"라고 말합니다."})
	aliceCreature, _ := world.Creature("creature:alice")
	if creatureHasNormalizedFlag(aliceCreature, "hidden", "PHIDDN") || aliceCreature.Stats["PHIDDN"] != 0 {
		t.Fatalf("alice creature hidden state = tags:%+v stats:%+v", aliceCreature.Metadata.Tags, aliceCreature.Stats)
	}
	alicePlayer, _ := world.Player("player:alice")
	if hasAnyNormalizedFlag(alicePlayer.Metadata.Tags, "hidden", "PHIDDN") {
		t.Fatalf("alice player hidden tags = %+v", alicePlayer.Metadata.Tags)
	}
}

func TestTellHandlerSendsToNamedActivePlayerAcrossRooms(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	memory := NewTellMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"send": NewTellHandler(world, memory),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	charlie := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", charlie, "player:charlie")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Charlie 멀리서 안녕 이야기"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, charlie, session.Command{Write: "\nAlice가 당신에게 \"멀리서 안녕\"라고 이야기합니다."})
	assertNoCommand(t, bob)
	cmd := <-alice
	if got, want := cmd.Write, "Charlie님에게 말을 전달하였습니다."; got != want {
		t.Fatalf("sender output = %q, want %q", got, want)
	}
	if got, ok := memory.LastSender("player:charlie"); !ok || got != "player:alice" {
		t.Fatalf("last sender = %q/%v, want player:alice/true", got, ok)
	}
}

func TestTellHandlerMatchesLowercizedASCIITargetLikeLegacy(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	memory := NewTellMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"send": NewTellHandler(world, memory),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "bOb 혼합 이야기"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice가 당신에게 \"혼합\"라고 이야기합니다."})
	assertCommand(t, alice, session.Command{Write: "Bob님에게 말을 전달하였습니다."})
	if got, ok := memory.LastSender("player:bob"); !ok || got != "player:alice" {
		t.Fatalf("last sender = %q/%v, want player:alice/true", got, ok)
	}
}

func TestTellHandlerRejectsGoOnlyActorIDTargetLikeLegacy(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	memory := NewTellMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"send": NewTellHandler(world, memory),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "player:bob 비밀 이야기"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, alice, session.Command{Write: "누구에게 말을 전하시려구요?"})
	assertNoCommand(t, bob)
	if got, ok := memory.LastSender("player:bob"); ok || got != "" {
		t.Fatalf("last sender = %q/%v, want empty/false", got, ok)
	}
}

func TestTellHandlerPreservesLegacyMessageSpacesAndEcho(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	setSocialCreatureStat(t, world, "creature:alice", "PLECHO", 1)
	memory := NewTellMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"send": NewTellHandler(world, memory),
		},
	})
	alice := make(chan session.Command, 4)
	charlie := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s3", charlie, "player:charlie")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Charlie   멀리서 안녕   이야기"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, charlie, session.Command{Write: "\nAlice가 당신에게 \"멀리서 안녕  \"라고 이야기합니다."})
	assertCommand(t, alice, session.Command{Write: "당신은 Charlie에게 \"멀리서 안녕  \"라고 이야기합니다."})
	if got, ok := memory.LastSender("player:charlie"); !ok || got != "player:alice" {
		t.Fatalf("last sender = %q/%v, want player:alice/true", got, ok)
	}
}

func TestTellHandlerReportsMissingTargetOrMessage(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"send": NewTellHandler(world, NewTellMemory()),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "이야기"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "누구에게 말을 전하시려구요?"})

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 이야기"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "무슨 말을 전하시려구요?"})
	assertNoCommand(t, bob)
}

func TestTellHandlerBlocksSilencedActor(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	setSocialCreatureStat(t, world, "creature:alice", "PSILNC", 1)
	memory := NewTellMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"send": NewTellHandler(world, memory),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 조용히 이야기"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, alice, session.Command{Write: "당신은 말을 할 수 없습니다."})
	assertNoCommand(t, bob)
	if got, ok := memory.LastSender("player:bob"); ok || got != "" {
		t.Fatalf("last sender = %q/%v, want empty/false", got, ok)
	}
}

func TestTellHandlerBlocksPropertyBackedSilencedActor(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	if _, err := world.SetCreatureProperty("creature:alice", "flags", "PSILNC"); err != nil {
		t.Fatalf("SetCreatureProperty() error = %v", err)
	}
	memory := NewTellMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"send": NewTellHandler(world, memory),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 조용히 이야기"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, alice, session.Command{Write: "당신은 말을 할 수 없습니다."})
	assertNoCommand(t, bob)
	if got, ok := memory.LastSender("player:bob"); ok || got != "" {
		t.Fatalf("last sender = %q/%v, want empty/false", got, ok)
	}
}

func TestTellHandlerBlocksIgnoredSender(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	memory := NewTellMemory()
	ignores := NewIgnoreMemory()
	ignores.Add("s2", "Alice")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"send": NewTellHandler(world, memory, ignores),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 조용히 이야기"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, alice, session.Command{Write: "Bob is ignoring you."})
	assertNoCommand(t, bob)
	if got, ok := memory.LastSender("player:bob"); ok || got != "" {
		t.Fatalf("last sender = %q/%v, want empty/false", got, ok)
	}
}

func TestTellHandlerBlocksPropertyBackedIgnoredTarget(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	if _, err := world.SetCreatureProperty("creature:bob", "flags", "PIGNOR"); err != nil {
		t.Fatalf("SetCreatureProperty() error = %v", err)
	}
	memory := NewTellMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"send": NewTellHandler(world, memory),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 조용히 이야기"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, alice, session.Command{Write: "Bob님은 이야기 듣기 거부 상태입니다."})
	assertNoCommand(t, bob)
	if got, ok := memory.LastSender("player:bob"); ok || got != "" {
		t.Fatalf("last sender = %q/%v, want empty/false", got, ok)
	}
}

func TestTellHandlerMatchesLegacyPIGNOROrderAndDMBYPASS(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	memory := NewTellMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"send": NewTellHandler(world, memory),
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	setSocialCreatureStat(t, world, "creature:bob", "PIGNOR", 1)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 이야기"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "Bob님은 이야기 듣기 거부 상태입니다."})
	assertNoCommand(t, bob)
	if got, ok := memory.LastSender("player:bob"); ok || got != "" {
		t.Fatalf("last sender = %q/%v, want empty/false", got, ok)
	}

	setSocialCreatureStat(t, world, "creature:alice", "class", model.ClassDM)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 관리자 이야기"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "\nAlice가 당신에게 \"관리자\"라고 이야기합니다."})
	assertCommand(t, alice, session.Command{Write: "Bob님에게 말을 전달하였습니다."})
}

func TestReplyHandlerUsesLastTellSender(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	memory := NewTellMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"send":   NewTellHandler(world, memory),
			"resend": NewReplyHandler(world, memory),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 안녕 이야기"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "\nAlice가 당신에게 \"안녕\"라고 이야기합니다."})
	assertCommand(t, alice, session.Command{Write: "Bob님에게 말을 전달하였습니다."})

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "반가워 대답"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "\nBob가 당신에게 \"반가워\"라고 대답합니다."})
	assertCommand(t, bob, session.Command{Write: "Alice님에게 말을 전하였습니다."})
}

func TestReplyHandlerPreservesLegacyMessageSpacesAndEcho(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	setSocialCreatureStat(t, world, "creature:bob", "PLECHO", 1)
	memory := NewTellMemory()
	memory.Remember("player:bob", "player:alice")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"resend": NewReplyHandler(world, memory),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "반가워   대답"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, alice, session.Command{Write: "\nBob가 당신에게 \"반가워  \"라고 대답합니다."})
	assertCommand(t, bob, session.Command{Write: "당신은 Alice에게 \"반가워  \"라고 대답합니다."})
	if got, ok := memory.LastSender("player:alice"); !ok || got != "player:bob" {
		t.Fatalf("last sender = %q/%v, want player:bob/true", got, ok)
	}
}

func TestReplyHandlerBlocksSilencedActor(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	setSocialCreatureStat(t, world, "creature:alice", "PSILNC", 1)
	memory := NewTellMemory()
	memory.Remember("player:alice", "player:bob")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"resend": NewReplyHandler(world, memory),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "조용히 대답"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, alice, session.Command{Write: "당신은 말을 할 수 없습니다."})
	assertNoCommand(t, bob)
	if got, ok := memory.LastSender("player:alice"); !ok || got != "player:bob" {
		t.Fatalf("last sender = %q/%v, want player:bob/true", got, ok)
	}
}

func TestReplyHandlerBlocksIgnoredSender(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	memory := NewTellMemory()
	memory.Remember("player:bob", "player:alice")
	ignores := NewIgnoreMemory()
	ignores.Add("s1", "Bob")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"resend": NewReplyHandler(world, memory, ignores),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "몰래 대답"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, bob, session.Command{Write: "Alice님은 이야기 거부중입니다."})
	assertNoCommand(t, alice)
	if got, ok := memory.LastSender("player:bob"); !ok || got != "player:alice" {
		t.Fatalf("last sender = %q/%v, want player:alice/true", got, ok)
	}
}

func TestReplyHandlerUsesLegacyInvisibleTargetGate(t *testing.T) {
	tests := []struct {
		name       string
		detect     bool
		wantSender string
		wantTarget string
	}{
		{name: "no detect invisible", wantSender: "누구에게 말을 전하시려구요?"},
		{
			name:       "detect invisible",
			detect:     true,
			wantSender: "Alice님에게 말을 전하였습니다.",
			wantTarget: "\nBob가 당신에게 \"몰래\"라고 대답합니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := state.NewWorld(socialWorld(t))
			setSocialCreatureStat(t, world, "creature:alice", "PINVIS", 1)
			if tt.detect {
				setSocialCreatureStat(t, world, "creature:bob", "PDINVI", 1)
			}
			memory := NewTellMemory()
			memory.Remember("player:bob", "player:alice")
			loop := NewLoop(enginecmd.Dispatcher{
				Registry: socialRegistry(t),
				Handlers: map[string]enginecmd.Handler{
					"resend": NewReplyHandler(world, memory),
				},
			})
			alice := make(chan session.Command, 4)
			bob := make(chan session.Command, 4)
			registerTestSession(t, loop, "s1", alice, "player:alice")
			registerTestSession(t, loop, "s2", bob, "player:bob")

			if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "몰래 대답"}); err != nil {
				t.Fatal(err)
			}

			assertCommand(t, bob, session.Command{Write: tt.wantSender})
			if tt.wantTarget == "" {
				assertNoCommand(t, alice)
			} else {
				assertCommand(t, alice, session.Command{Write: tt.wantTarget})
			}
		})
	}
}

func TestBroadcastChatHandlerSendsToAllActivePlayers(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	setSocialCreatureStat(t, world, "creature:alice", "level", 20)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"broadsend": NewBroadcastChatHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	charlie := make(chan session.Command, 4)
	anonymous := make(chan session.Command, 4)
	registerTestSession(t, loop, "s0", anonymous, "")
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", charlie, "player:charlie")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "모두 안녕 잡담"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice> 모두 안녕"})
	assertCommand(t, charlie, session.Command{Write: "\nAlice> 모두 안녕"})
	assertNoCommand(t, anonymous)
	cmd := <-alice
	if got, want := cmd.Write, "\nAlice> 모두 안녕"; got != want {
		t.Fatalf("sender output = %q, want %q", got, want)
	}
	aliceCreature, _ := world.Creature("creature:alice")
	if got, want := aliceCreature.Stats["hpCurrent"], 28; got != want {
		t.Fatalf("alice hpCurrent = %d, want %d", got, want)
	}
}

func TestBroadcastChatHandlerPreservesLegacyCutCommandSpaces(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	setSocialCreatureStat(t, world, "creature:alice", "level", 20)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"broadsend": NewBroadcastChatHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "모두 안녕   잡담"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice> 모두 안녕  "})
	assertCommand(t, alice, session.Command{Write: "\nAlice> 모두 안녕  "})
}

func TestCheerHandlerSendsLegacyCheerLine(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	setSocialCreatureStat(t, world, "creature:alice", "level", 20)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"broadsend2": NewCheerHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "좋다 환호"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice님이 \"좋다\"라고 환호를 합니다."})
	assertCommand(t, alice, session.Command{Write: "\nAlice님이 \"좋다\"라고 환호를 합니다."})
	aliceCreature, _ := world.Creature("creature:alice")
	if got, want := aliceCreature.Stats["hpCurrent"], 28; got != want {
		t.Fatalf("alice hpCurrent = %d, want %d", got, want)
	}
}

func TestCheerHandlerPreservesLegacyCutCommandSpaces(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	setSocialCreatureStat(t, world, "creature:alice", "level", 20)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"broadsend2": NewCheerHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "좋다   환호"}); err != nil {
		t.Fatal(err)
	}

	want := session.Command{Write: "\nAlice님이 \"좋다  \"라고 환호를 합니다."}
	assertCommand(t, bob, want)
	assertCommand(t, alice, want)
}

func TestBroadcastChatHandlerMatchesLegacyRestrictions(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"broadsend": NewBroadcastChatHandler(world),
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "잡담"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "무슨 말을 하시려구요?"})
	assertNoCommand(t, bob)

	setSocialCreatureStat(t, world, "creature:alice", "level", 20)
	setSocialCreatureStat(t, world, "creature:alice", "PSILNC", 1)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "소리 잡담"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "당신의 목소리가 너무 작아 잡담을 할 수 없습니다."})
	assertNoCommand(t, bob)

	setSocialCreatureStat(t, world, "creature:alice", "PSILNC", 0)
	setSocialCreatureStat(t, world, "creature:alice", "level", 19)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "낮다 잡담"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "당신의 레벨로는 잡담을 할 수 없습니다."})
	assertNoCommand(t, bob)

	setSocialCreatureStat(t, world, "creature:alice", "level", 20)
	setSocialCreatureStat(t, world, "creature:alice", "hpCurrent", 2)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "위험 잡담"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "당신의 목숨이 위태로워 잡담을 할 수 없습니다."})
	assertNoCommand(t, bob)

	setSocialCreatureStat(t, world, "creature:alice", "hpCurrent", 30)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "반복 잡담"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "\nAlice> 반복"})
	assertCommand(t, alice, session.Command{Write: "\nAlice> 반복"})
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "반복 잡담"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "\n도배하지 마세요.\n"})
	assertNoCommand(t, bob)
}

func TestCheerHandlerMatchesLegacyRestrictionMessages(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"broadsend2": NewCheerHandler(world),
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "환호"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "무슨 말을 하시려구요?"})
	assertNoCommand(t, bob)

	setSocialCreatureStat(t, world, "creature:alice", "level", 20)
	setSocialCreatureStat(t, world, "creature:alice", "PSILNC", 1)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "조용 환호"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "당신의 목소리가 너무 작아 환호를 할 수 없습니다."})
	assertNoCommand(t, bob)
}

func TestFamilyTalkSendsOnlyToActiveFamilyMembers(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	setSocialCreatureStat(t, world, "creature:alice", "familyFlag", 1)
	setSocialCreatureStat(t, world, "creature:alice", "familyID", 7)
	setSocialCreatureStat(t, world, "creature:bob", "familyFlag", 1)
	setSocialCreatureStat(t, world, "creature:bob", "familyID", 7)
	setSocialCreatureStat(t, world, "creature:charlie", "familyFlag", 1)
	setSocialCreatureStat(t, world, "creature:charlie", "familyID", 7)
	setSocialCreatureStat(t, world, "creature:dave", "familyFlag", 1)
	setSocialCreatureStat(t, world, "creature:dave", "familyID", 8)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"family_talk": NewFamilyTalkHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	charlie := make(chan session.Command, 4)
	dave := make(chan session.Command, 4)
	eve := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", charlie, "player:charlie")
	registerTestSession(t, loop, "s4", dave, "player:dave")
	registerTestSession(t, loop, "s5", eve, "player:eve")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "전원 준비 패거리말"}); err != nil {
		t.Fatal(err)
	}

	want := "\nAlice>>> 전원 준비"
	assertCommand(t, alice, session.Command{Write: want})
	assertCommand(t, bob, session.Command{Write: want})
	assertCommand(t, charlie, session.Command{Write: want})
	assertNoCommand(t, dave)
	assertNoCommand(t, eve)

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "간격 유지   패거리말"}); err != nil {
		t.Fatal(err)
	}
	want = "\nAlice>>> 간격 유지  "
	assertCommand(t, alice, session.Command{Write: want})
	assertCommand(t, bob, session.Command{Write: want})
	assertCommand(t, charlie, session.Command{Write: want})
	assertNoCommand(t, dave)
	assertNoCommand(t, eve)
}

func TestFamilyTalkReportsMissingFamilyMessageAndSilence(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"family_talk": NewFamilyTalkHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "패거리말"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "당신은 패거리에 속해있지 않습니다."})

	setSocialCreatureStat(t, world, "creature:alice", "familyFlag", 1)
	setSocialCreatureStat(t, world, "creature:alice", "familyID", 7)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "패거리말"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "패거리원들에게 무슨 말을 하시려고요?"})

	setSocialCreatureStat(t, world, "creature:alice", "PSILNC", 1)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "조용히 패거리말"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "입이 막혀 말이 나오질 않습니다."})
}

func TestFamilyWhoListsActiveFamilyMembers(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	setSocialCreatureStat(t, world, "creature:alice", "familyFlag", 1)
	setSocialCreatureStat(t, world, "creature:alice", "familyID", 7)
	setSocialCreatureStat(t, world, "creature:bob", "familyFlag", 1)
	setSocialCreatureStat(t, world, "creature:bob", "familyID", 7)
	setSocialCreatureStat(t, world, "creature:charlie", "familyFlag", 1)
	setSocialCreatureStat(t, world, "creature:charlie", "familyID", 7)
	setSocialCreatureStat(t, world, "creature:dave", "familyFlag", 1)
	setSocialCreatureStat(t, world, "creature:dave", "familyID", 8)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"family_who": NewFamilyWhoHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", make(chan session.Command, 4), "player:bob")
	registerTestSession(t, loop, "s3", make(chan session.Command, 4), "player:charlie")
	registerTestSession(t, loop, "s4", make(chan session.Command, 4), "player:dave")
	registerTestSession(t, loop, "s5", make(chan session.Command, 4), "player:eve")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "패거리누구"}); err != nil {
		t.Fatal(err)
	}
	out := (<-alice).Write
	for _, want := range []string{
		"당신은 [패거리7] 패거리에 소속되어 있습니다.",
		"Alice",
		"Bob",
		"Charlie",
		"총 3명의 패거리원들이 이용중입니다.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("family who output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Dave") || strings.Contains(out, "Eve") {
		t.Fatalf("family who output included non-family member:\n%s", out)
	}
}

func TestFamilyWhoMatchesLegacyPendingAndVisibility(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"family_who": NewFamilyWhoHandler(world),
		},
	})
	alice := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", make(chan session.Command, 4), "player:bob")
	registerTestSession(t, loop, "s3", make(chan session.Command, 4), "player:charlie")

	setSocialCreatureStat(t, world, "creature:alice", "PBLIND", 1)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "패거리누구"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "당신은 눈이 멀어 있습니다!"})

	setSocialCreatureStat(t, world, "creature:alice", "PBLIND", 0)
	setSocialCreatureStat(t, world, "creature:alice", "PRDFML", 1)
	setSocialCreatureStat(t, world, "creature:alice", "familyID", 7)
	setSocialCreatureStat(t, world, "creature:bob", "PRDFML", 1)
	setSocialCreatureStat(t, world, "creature:bob", "familyID", 7)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 패거리누구"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "Bob님은 [패거리7] 패거리에 가입을 신청중입니다."})

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "패거리누구"}); err != nil {
		t.Fatal(err)
	}
	out := (<-alice).Write
	for _, want := range []string{
		"당신은 [패거리7] 패거리에 가입을 신청중입니다.",
		"(-)Alice",
		"(-)Bob",
		"총 2명의 패거리원들이 이용중입니다.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("family who pending output missing %q:\n%s", want, out)
		}
	}

	setSocialCreatureStat(t, world, "creature:charlie", "familyFlag", 1)
	setSocialCreatureStat(t, world, "creature:charlie", "familyID", 7)
	setSocialCreatureStat(t, world, "creature:charlie", "PINVIS", 1)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Charlie 패거리누구"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "현재 이용중이 아닙니다."})
}

func TestGroupHandlersFollowTalkAndLose(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	groups := NewGroupMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"follow": NewFollowHandler(world, groups),
			"lose":   NewLoseHandler(world, groups),
			"group":  NewGroupHandler(world, groups),
			"gtalk":  NewGroupTalkHandler(world, groups),
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	dave := make(chan session.Command, 8)
	charlie := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", dave, "player:dave")
	registerTestSession(t, loop, "s4", charlie, "player:charlie")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "Alice 따라"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "당신은 이제부터 Alice님을 따라다닙니다."})
	assertCommand(t, alice, session.Command{Write: "\nBob가 이제부터 당신을 따라다닙니다."})
	assertCommand(t, dave, session.Command{Write: "\nBob가 이제부터 Alice를 따라다닙니다."})
	assertNoCommand(t, charlie)

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "그룹"}); err != nil {
		t.Fatal(err)
	}
	groupOut := (<-alice).Write
	for _, want := range []string{"그룹원:\n", "Alice", "(대장)", "Bob", "체력:  30/  30", "도력:   9/   9"} {
		if !strings.Contains(groupOut, want) {
			t.Fatalf("group output missing %q:\n%s", want, groupOut)
		}
	}
	if strings.HasSuffix(groupOut, "\n") {
		t.Fatalf("group output ended with newline: %q", groupOut)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "준비 그룹말"}); err != nil {
		t.Fatal(err)
	}
	wantTalk := "Bob가 그룹원들에게 \"준비\"라고 말합니다.\n"
	assertCommand(t, alice, session.Command{Write: wantTalk})
	assertCommand(t, bob, session.Command{Write: wantTalk})
	assertNoCommand(t, dave)
	assertNoCommand(t, charlie)

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "간격 유지   그룹말"}); err != nil {
		t.Fatal(err)
	}
	wantTalk = "Bob가 그룹원들에게 \"간격 유지  \"라고 말합니다.\n"
	assertCommand(t, alice, session.Command{Write: wantTalk})
	assertCommand(t, bob, session.Command{Write: wantTalk})
	assertNoCommand(t, dave)
	assertNoCommand(t, charlie)

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "bOB 내보내"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "당신은 Bob가 당신을 못따라 오도록 하였습니다."})
	assertCommand(t, bob, session.Command{Write: "\nAlice가 당신이 못따라 오도록 하였습니다."})
	assertCommand(t, dave, session.Command{Write: "\nAlice가 Bob를 못따라 오도록 하였습니다."})

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "다시 그룹말"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "당신은 그룹에 속해있지 않습니다.\n"})
	assertNoCommand(t, bob)
}

func TestGroupHandlerHidesDMInvisibleFollowersLikeLegacy(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	groups := NewGroupMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"group": NewGroupHandler(world, groups),
		},
	})
	alice := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "그룹"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "당신은 그룹에 속해 있지 않습니다."})

	groups.Follow("player:bob", "player:alice")
	setSocialCreatureStat(t, world, "creature:bob", "PDMINV", 1)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "그룹"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "당신은 그룹에 속해 있지 않습니다."})

	groups.Follow("player:dave", "player:alice")
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "그룹"}); err != nil {
		t.Fatal(err)
	}
	out := (<-alice).Write
	for _, want := range []string{"그룹원:\n", "Alice", "Dave"} {
		if !strings.Contains(out, want) {
			t.Fatalf("group output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Bob") {
		t.Fatalf("group output included PDMINV follower:\n%s", out)
	}
	if strings.HasSuffix(out, "\n") {
		t.Fatalf("group output ended with newline: %q", out)
	}
}

func TestGroupTalkHandlerMatchesLegacySilenceAndDMInvisibleOnlyGroup(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	groups := NewGroupMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"gtalk": NewGroupTalkHandler(world, groups),
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	groups.Follow("player:bob", "player:alice")
	setSocialCreatureStat(t, world, "creature:alice", "PSILNC", 1)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "조용 그룹말"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "입이 막혀 말이 나오질 않습니다.\n"})
	assertNoCommand(t, bob)

	setSocialCreatureStat(t, world, "creature:alice", "PSILNC", 0)
	setSocialCreatureStat(t, world, "creature:bob", "PDMINV", 1)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "숨은 그룹말"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "Alice가 그룹원들에게 \"숨은\"라고 말합니다.\n"})
	assertCommand(t, alice, session.Command{Write: "당신은 그룹에 속해있지 않습니다.\n"})
}

func TestGroupTalkHandlerHonorsLegacyPIGNOR(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	groups := NewGroupMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"gtalk": NewGroupTalkHandler(world, groups),
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	groups.Follow("player:bob", "player:alice")

	setSocialCreatureStat(t, world, "creature:bob", "PIGNOR", 1)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "거부 그룹말"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "Bob는 이야기 듣기 거부 상태입니다.\nAlice가 그룹원들에게 \"거부\"라고 말합니다.\n"})
	assertNoCommand(t, bob)

	setSocialCreatureStat(t, world, "creature:bob", "PIGNOR", 0)
	setSocialCreatureStat(t, world, "creature:alice", "PIGNOR", 1)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "리더 그룹말"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "Bob가 그룹원들에게 \"리더\"라고 말합니다.\nAlice님은 이야기 듣기 거부 상태입니다.\n"})
	assertNoCommand(t, alice)

	setSocialCreatureStat(t, world, "creature:bob", "class", model.ClassCaretaker)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "관리 그룹말"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "Bob가 그룹원들에게 \"관리\"라고 말합니다.\n"})
	assertCommand(t, alice, session.Command{Write: "Bob가 그룹원들에게 \"관리\"라고 말합니다.\n"})
}

func TestFollowHandlerMatchesLegacyFailureOutputAndReveal(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	groups := NewGroupMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"follow": NewFollowHandler(world, groups),
		},
	})
	bob := make(chan session.Command, 8)
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "따라"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "누구를 따라 가시고 싶으세요?"})

	if _, err := world.UpdateCreatureTags("creature:bob", []string{"hidden"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := world.UpdatePlayerTags("player:bob", []string{"hidden"}, nil); err != nil {
		t.Fatal(err)
	}
	setSocialCreatureStat(t, world, "creature:bob", "PHIDDN", 1)

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "나 따라"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "자기자신을 따라 갈순 없습니다."})
	bobCreature, _ := world.Creature("creature:bob")
	if creatureHasNormalizedFlag(bobCreature, "hidden", "PHIDDN") || bobCreature.Stats["PHIDDN"] != 0 {
		t.Fatalf("bob creature hidden state = tags:%+v stats:%+v", bobCreature.Metadata.Tags, bobCreature.Stats)
	}
	bobPlayer, _ := world.Player("player:bob")
	if hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "hidden", "PHIDDN") {
		t.Fatalf("bob player hidden tags = %+v", bobPlayer.Metadata.Tags)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "Nobody 따라"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "그런 사람은 여기 없습니다."})
}

func TestFollowHandlerLoopMessageUsesLegacyGenderPronoun(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	setSocialCreatureStat(t, world, "creature:alice", "PMALES", 1)
	groups := NewGroupMemory()
	groups.Follow("player:alice", "player:bob")
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"follow": NewFollowHandler(world, groups),
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "Alice 따라"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "이미 그는 당신을 따라다니고 있습니다."})
	assertNoCommand(t, alice)
}

func TestFollowHandlerSuppressesTargetAndRoomNoticeWhenDMInvisible(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	setSocialCreatureStat(t, world, "creature:bob", "PDMINV", 1)
	groups := NewGroupMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"follow": NewFollowHandler(world, groups),
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	dave := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", dave, "player:dave")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "Alice 따라"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "당신은 이제부터 Alice님을 따라다닙니다."})
	assertNoCommand(t, alice)
	assertNoCommand(t, dave)
	if leader, ok := groups.LeaderOf("player:bob"); !ok || leader != "player:alice" {
		t.Fatalf("bob leader = %q, %v; want player:alice, true", leader, ok)
	}
}

func TestLoseHandlerSuppressesTargetAndRoomNoticeWhenDMInvisible(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	setSocialCreatureStat(t, world, "creature:alice", "PDMINV", 1)
	groups := NewGroupMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"follow": NewFollowHandler(world, groups),
			"lose":   NewLoseHandler(world, groups),
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	dave := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", dave, "player:dave")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "Alice 따라"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "당신은 이제부터 Alice님을 따라다닙니다."})
	assertCommand(t, alice, session.Command{Write: "\nBob가 이제부터 당신을 따라다닙니다."})
	assertCommand(t, dave, session.Command{Write: "\nBob가 이제부터 Alice를 따라다닙니다."})

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 내보내"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "당신은 Bob가 당신을 못따라 오도록 하였습니다."})
	assertNoCommand(t, bob)
	assertNoCommand(t, dave)
}

func TestLoseHandlerWithTargetNotFollowingMatchesLegacyOutput(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	groups := NewGroupMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"lose": NewLoseHandler(world, groups),
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if _, err := world.UpdateCreatureTags("creature:alice", []string{"hidden", "PHIDDN"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := world.UpdatePlayerTags("player:alice", []string{"hidden", "PHIDDN"}, nil); err != nil {
		t.Fatal(err)
	}
	setSocialCreatureStat(t, world, "creature:alice", "PHIDDN", 1)

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "bOB 내보내"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "그 사람은 당신을 따라다니고 있지 않습니다."})
	assertNoCommand(t, bob)
	aliceCreature, _ := world.Creature("creature:alice")
	if creatureHasNormalizedFlag(aliceCreature, "hidden", "PHIDDN") || aliceCreature.Stats["PHIDDN"] != 0 {
		t.Fatalf("alice creature hidden state = tags:%+v stats:%+v", aliceCreature.Metadata.Tags, aliceCreature.Stats)
	}
	alicePlayer, _ := world.Player("player:alice")
	if hasAnyNormalizedFlag(alicePlayer.Metadata.Tags, "hidden", "PHIDDN") {
		t.Fatalf("alice player hidden tags = %+v", alicePlayer.Metadata.Tags)
	}
}

func TestLoseHandlerWithoutTargetMatchesLegacyOutput(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	groups := NewGroupMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"follow": NewFollowHandler(world, groups),
			"lose":   NewLoseHandler(world, groups),
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "내보내"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "당신은 누구를 따라다니고 있지 않습니다."})

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "Alice 따라"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "당신은 이제부터 Alice님을 따라다닙니다."})
	assertCommand(t, alice, session.Command{Write: "\nBob가 이제부터 당신을 따라다닙니다."})

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "내보내"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "당신은 Alice를 그만 따라다니기로 하였습니다.\n"})
	assertCommand(t, alice, session.Command{Write: "\nBob는 이제 당신을 따라 다니지 않습니다."})
}

func TestLoseHandlerWithoutTargetSuppressesLeaderNoticeWhenDMInvisible(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	groups := NewGroupMemory()
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"follow": NewFollowHandler(world, groups),
			"lose":   NewLoseHandler(world, groups),
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "Alice 따라"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "당신은 이제부터 Alice님을 따라다닙니다."})
	assertCommand(t, alice, session.Command{Write: "\nBob가 이제부터 당신을 따라다닙니다."})

	setSocialCreatureStat(t, world, "creature:bob", "PDMINV", 1)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "내보내"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "당신은 Alice를 그만 따라다니기로 하였습니다.\n"})
	assertNoCommand(t, alice)
}

func TestGroupMoveHandlerMovesActiveFollowersWithLeader(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	groups := NewGroupMemory()
	move := NewGroupMoveHandler(world, groups, enginecmd.NewMoveHandler(world))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"follow": NewFollowHandler(world, groups),
			"move":   move,
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "Alice 따라"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "당신은 이제부터 Alice님을 따라다닙니다."})
	assertCommand(t, alice, session.Command{Write: "\nBob가 이제부터 당신을 따라다닙니다."})

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "동"}); err != nil {
		t.Fatal(err)
	}
	aliceOut := (<-alice).Write
	if !strings.Contains(aliceOut, "\nTwo\n") {
		t.Fatalf("alice move output missing destination:\n%s", aliceOut)
	}
	bobOut := (<-bob).Write
	for _, want := range []string{"\nAlice님을 따라갑니다.\n", "\nTwo\n"} {
		if !strings.Contains(bobOut, want) {
			t.Fatalf("bob follow output missing %q:\n%s", want, bobOut)
		}
	}
	if player, _ := world.Player("player:bob"); player.RoomID != "room:two" {
		t.Fatalf("bob room = %q, want room:two", player.RoomID)
	}
}

func TestActionHandlerBroadcastsUntargetedActionToSameRoom(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"action": NewActionHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	charlie := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", charlie, "player:charlie")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "미소"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, bob, session.Command{Write: "Alice가 밝은 미소를 짓습니다.\n"})
	assertNoCommand(t, charlie)
	cmd := <-alice
	if got, want := cmd.Write, "당신은 밝은 미소를 짓습니다.\n"; got != want {
		t.Fatalf("sender output = %q, want %q", got, want)
	}
}

func TestActionHandlerClearsHiddenLikeLegacy(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	if _, err := world.UpdateCreatureTags("creature:alice", []string{"hidden", "PHIDDN"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := world.UpdatePlayerTags("player:alice", []string{"hidden", "PHIDDN"}, nil); err != nil {
		t.Fatal(err)
	}
	setSocialCreatureStat(t, world, "creature:alice", "PHIDDN", 1)

	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"action": NewActionHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "미소"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "당신은 밝은 미소를 짓습니다.\n"})
	assertCommand(t, bob, session.Command{Write: "Alice가 밝은 미소를 짓습니다.\n"})

	creature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("alice creature missing")
	}
	if creatureHasNormalizedFlag(creature, "hidden", "PHIDDN") || creature.Stats["PHIDDN"] != 0 {
		t.Fatalf("alice creature hidden state = tags:%+v stats:%+v", creature.Metadata.Tags, creature.Stats)
	}
	player, ok := world.Player("player:alice")
	if !ok {
		t.Fatal("alice player missing")
	}
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "PHIDDN") {
		t.Fatalf("alice player hidden tags = %+v", player.Metadata.Tags)
	}
}

func TestActionHandlerRejectsSilencedActorAfterClearingHiddenLikeLegacy(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	if _, err := world.UpdateCreatureTags("creature:alice", []string{"hidden", "PHIDDN"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := world.UpdatePlayerTags("player:alice", []string{"hidden", "PHIDDN"}, nil); err != nil {
		t.Fatal(err)
	}
	setSocialCreatureStat(t, world, "creature:alice", "PHIDDN", 1)
	setSocialCreatureStat(t, world, "creature:alice", "PSILNC", 1)

	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"action": NewActionHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "미소"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "한마디도 할수 없습니다!"})
	assertNoCommand(t, bob)

	creature, ok := world.Creature("creature:alice")
	if !ok {
		t.Fatal("alice creature missing")
	}
	if creatureHasNormalizedFlag(creature, "hidden", "PHIDDN") || creature.Stats["PHIDDN"] != 0 {
		t.Fatalf("alice creature hidden state = tags:%+v stats:%+v", creature.Metadata.Tags, creature.Stats)
	}
	player, ok := world.Player("player:alice")
	if !ok {
		t.Fatal("alice player missing")
	}
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "PHIDDN") {
		t.Fatalf("alice player hidden tags = %+v", player.Metadata.Tags)
	}
}

func TestActionHandlerTargetsActivePlayer(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"action": NewActionHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	dave := make(chan session.Command, 4)
	charlie := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", dave, "player:dave")
	registerTestSession(t, loop, "s4", charlie, "player:charlie")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 미소"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, bob, session.Command{Write: "Alice가 당신에게 미소를 짓습니다.\n"})
	assertCommand(t, dave, session.Command{Write: "Alice가 Bob에게 미소를 짓습니다.\n"})
	assertNoCommand(t, charlie)
	cmd := <-alice
	if got, want := cmd.Write, "당신은 Bob에게 미소를 짓습니다.\n"; got != want {
		t.Fatalf("sender output = %q, want %q", got, want)
	}
}

func TestActionHandlerRejectsGoOnlyIDTargetLikeLegacyFindCrt(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"action": NewActionHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	dave := make(chan session.Command, 4)
	charlie := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", dave, "player:dave")
	registerTestSession(t, loop, "s4", charlie, "player:charlie")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "player:bob 미소"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, bob, session.Command{Write: "Alice가 player:bob 미소를 짓습니다.\n"})
	assertCommand(t, dave, session.Command{Write: "Alice가 player:bob 미소를 짓습니다.\n"})
	assertNoCommand(t, charlie)
	cmd := <-alice
	if got, want := cmd.Write, "당신은 player:bob 미소를 짓습니다.\n"; got != want {
		t.Fatalf("sender output = %q, want %q", got, want)
	}
}

func TestActionHandlerTargetsNPCAndSupportsCommandFirst(t *testing.T) {
	for _, line := range []string{"경비병 안녕", "안녕 경비병"} {
		t.Run(line, func(t *testing.T) {
			world := state.NewWorld(socialWorld(t))
			loop := NewLoop(enginecmd.Dispatcher{
				Registry: socialRegistry(t),
				Handlers: map[string]enginecmd.Handler{
					"action": NewActionHandler(world),
				},
			})
			alice := make(chan session.Command, 4)
			bob := make(chan session.Command, 4)
			charlie := make(chan session.Command, 4)
			registerTestSession(t, loop, "s1", alice, "player:alice")
			registerTestSession(t, loop, "s2", bob, "player:bob")
			registerTestSession(t, loop, "s3", charlie, "player:charlie")

			if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: line}); err != nil {
				t.Fatal(err)
			}

			assertCommand(t, bob, session.Command{Write: "Alice가 경비병에게 인사를 합니다. \"안녕하세요~\"\n"})
			assertNoCommand(t, charlie)
			cmd := <-alice
			if got, want := cmd.Write, "당신은 경비병에게 인사를 합니다. \"안녕하세요~\"\n"; got != want {
				t.Fatalf("sender output = %q, want %q", got, want)
			}
		})
	}
}

func TestEmoteHandlerBroadcastsCustomTextToSameRoom(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"emote": NewEmoteHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	charlie := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", charlie, "player:charlie")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "환하게 웃는다 표현"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, bob, session.Command{Write: "\n:Alice가 환하게 웃는다."})
	assertNoCommand(t, charlie)
	cmd := <-alice
	if got, want := cmd.Write, "예. 좋습니다.\n"; got != want {
		t.Fatalf("sender output = %q, want %q", got, want)
	}
}

func TestEmoteHandlerPreservesLegacyCutCommandSpaces(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	setSocialCreatureStat(t, world, "creature:alice", "PLECHO", 1)
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"emote": NewEmoteHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "고개를 끄덕인다   표현"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, bob, session.Command{Write: "\n:Alice가 고개를 끄덕인다  ."})
	assertCommand(t, alice, session.Command{Write: ":Alice가 고개를 끄덕인다  ."})
}

func TestEmoteHandlerRequiresMessage(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"emote": NewEmoteHandler(world),
		},
	})
	commands := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", commands, "player:alice")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "표현"}); err != nil {
		t.Fatal(err)
	}

	cmd := <-commands
	if got, want := cmd.Write, "무슨말을 표현하시려구요?\n"; got != want {
		t.Fatalf("emote output = %q, want %q", got, want)
	}
}

func TestEmoteHandlerMatchesLegacySilenceAndEcho(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"emote": NewEmoteHandler(world),
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	setSocialCreatureStat(t, world, "creature:alice", "PSILNC", 1)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "움직인다 표현"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "당신은 지금당장 그것을 할 수 없습니다.\n"})
	assertNoCommand(t, bob)

	setSocialCreatureStat(t, world, "creature:alice", "PSILNC", 0)
	setSocialCreatureStat(t, world, "creature:alice", "PLECHO", 1)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "고개를 끄덕인다 표현"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "\n:Alice가 고개를 끄덕인다."})
	assertCommand(t, alice, session.Command{Write: ":Alice가 고개를 끄덕인다."})
}

func TestYellHandlerBroadcastsToSameAndAdjacentRooms(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"yell": NewYellHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	charlie := make(chan session.Command, 4)
	dave := make(chan session.Command, 4)
	eve := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")
	registerTestSession(t, loop, "s3", charlie, "player:charlie")
	registerTestSession(t, loop, "s4", dave, "player:dave")
	registerTestSession(t, loop, "s5", eve, "player:eve")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "도와줘 외쳐"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice이 \"도와줘 외쳐!\"라고 외칩니다."})
	assertCommand(t, dave, session.Command{Write: "\nAlice이 \"도와줘 외쳐!\"라고 외칩니다."})
	assertCommand(t, charlie, session.Command{Write: "\n누군가가 \"도와줘 외쳐!\"라고 외쳤습니다."})
	assertNoCommand(t, eve)
	cmd := <-alice
	if got, want := cmd.Write, "예. 좋습니다."; got != want {
		t.Fatalf("sender output = %q, want %q", got, want)
	}
}

func TestYellHandlerKeepsCommandFirstCompatibility(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"yell": NewYellHandler(world),
		},
	})
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "외쳐 도와줘"}); err != nil {
		t.Fatal(err)
	}

	assertCommand(t, bob, session.Command{Write: "\nAlice이 \"도와줘!\"라고 외칩니다."})
	assertCommand(t, alice, session.Command{Write: "예. 좋습니다."})
}

func TestYellHandlerMatchesLegacyPromptAndSilence(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	loop := NewLoop(enginecmd.Dispatcher{
		Registry: socialRegistry(t),
		Handlers: map[string]enginecmd.Handler{
			"yell": NewYellHandler(world),
		},
	})
	alice := make(chan session.Command, 8)
	bob := make(chan session.Command, 8)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "외쳐"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "무슨말을 외치려구요?"})
	assertNoCommand(t, bob)

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "조용히#외쳐"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "무슨말을 외치려구요?"})
	assertNoCommand(t, bob)

	setSocialCreatureStat(t, world, "creature:alice", "PSILNC", 1)
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "조용히 외쳐"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "당신의 목소리가 너무 약해서 외칠수 없습니다."})
	assertNoCommand(t, bob)
}

func socialRegistry(t *testing.T) commandspec.Registry {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "누구", Number: 8, Handler: "who"},
		{Name: "사용자검색", Number: 69, Handler: "whois"},
		{Name: "사용자정보", Number: 80, Handler: "pfinger"},
		{Name: "말", Number: 4, Handler: "say"},
		{Name: "이야기", Number: 17, Handler: "send"},
		{Name: "얘기", Number: 17, Handler: "send"},
		{Name: "대답", Number: 154, Handler: "resend"},
		{Name: "잡담", Number: 59, Handler: "broadsend"},
		{Name: "환호", Number: 70, Handler: "broadsend2"},
		{Name: "따라", Number: 18, Handler: "follow"},
		{Name: "내보내", Number: 19, Handler: "lose"},
		{Name: "그룹", Number: 20, Handler: "group"},
		{Name: "무리", Number: 20, Handler: "group"},
		{Name: "그룹말", Number: 57, Handler: "gtalk"},
		{Name: "무리말", Number: 57, Handler: "gtalk"},
		{Name: "=", Number: 57, Handler: "gtalk"},
		{Name: "패거리누구", Number: 148, Handler: "family_who"},
		{Name: "패거리말", Number: 148, Handler: "family_talk"},
		{Name: "]", Number: 148, Handler: "family_talk"},
		{Name: "대화", Number: 56, Handler: "talk"},
		{Name: "동", Number: 1, Handler: "move"},
		{Name: "가", Number: 30, Handler: "go"},
		{Name: "표현", Number: 25, Handler: "emote"},
		{Name: "외쳐", Number: 29, Handler: "yell"},
		{Name: "미소", Number: 100, Handler: "action"},
		{Name: "안녕", Number: 100, Handler: "action"},
		{Name: "보아", Number: 100, Handler: "action"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func socialWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLoopRoom(t, loaded, model.Room{
		ID:          "room:one",
		DisplayName: "One",
		Exits:       []model.Exit{{Name: "동", ToRoomID: "room:two"}},
	})
	mustAddLoopRoom(t, loaded, model.Room{ID: "room:two", DisplayName: "Two"})
	mustAddLoopRoom(t, loaded, model.Room{ID: "room:three", DisplayName: "Three"})
	for _, player := range []model.Player{
		{ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:one"},
		{ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob", RoomID: "room:one"},
		{ID: "player:dave", DisplayName: "Dave", CreatureID: "creature:dave", RoomID: "room:one"},
		{ID: "player:charlie", DisplayName: "Charlie", CreatureID: "creature:charlie", RoomID: "room:two"},
		{ID: "player:eve", DisplayName: "Eve", CreatureID: "creature:eve", RoomID: "room:three"},
	} {
		mustAddLoopPlayer(t, loaded, player)
	}
	for _, creature := range []model.Creature{
		{ID: "creature:alice", Kind: model.CreatureKindPlayer, DisplayName: "Alice", PlayerID: "player:alice", RoomID: "room:one", Stats: map[string]int{"hpCurrent": 30, "hpMax": 30, "mpCurrent": 9, "mpMax": 9}},
		{ID: "creature:bob", Kind: model.CreatureKindPlayer, DisplayName: "Bob", PlayerID: "player:bob", RoomID: "room:one", Stats: map[string]int{"hpCurrent": 24, "hpMax": 30, "mpCurrent": 7, "mpMax": 9}},
		{ID: "creature:dave", Kind: model.CreatureKindPlayer, DisplayName: "Dave", PlayerID: "player:dave", RoomID: "room:one", Stats: map[string]int{"hpCurrent": 18, "hpMax": 22, "mpCurrent": 5, "mpMax": 6}},
		{ID: "creature:charlie", Kind: model.CreatureKindPlayer, DisplayName: "Charlie", PlayerID: "player:charlie", RoomID: "room:two", Stats: map[string]int{"hpCurrent": 20, "hpMax": 20, "mpCurrent": 4, "mpMax": 4}},
		{ID: "creature:eve", Kind: model.CreatureKindPlayer, DisplayName: "Eve", PlayerID: "player:eve", RoomID: "room:three", Stats: map[string]int{"hpCurrent": 16, "hpMax": 16, "mpCurrent": 3, "mpMax": 3}},
	} {
		mustAddLoopCreature(t, loaded, creature)
	}
	mustAddLoopCreature(t, loaded, model.Creature{
		ID:          "creature:guard",
		Kind:        model.CreatureKindNPC,
		DisplayName: "경비병",
		RoomID:      "room:one",
	})
	return loaded
}

func setSocialCreatureStat(t *testing.T, world *state.World, creatureID model.CreatureID, key string, value int) {
	t.Helper()
	if err := world.SetCreatureStat(creatureID, key, value); err != nil {
		t.Fatalf("SetCreatureStat(%q, %q) error: %v", creatureID, key, err)
	}
}
