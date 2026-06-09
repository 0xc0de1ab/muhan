package game

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"muhan/internal/commandspec"
	enginecmd "muhan/internal/engine/command"
	"muhan/internal/persist/legacykr"
	"muhan/internal/session"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestInviteHandlerTogglesHangulInviteNamesLikeLegacy(t *testing.T) {
	world := marriageInviteTestWorld(t)
	loop := NewLoop(marriageTestDispatcher(t, world, NewMarriageRequests()))
	alice := make(chan session.Command, 5)
	bob := make(chan session.Command, 3)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "홍길동 초대"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "초대 대상에 추가했습니다.\n"})
	assertNoCommand(t, bob)
	invites := world.MarriageInvites(42)
	if len(invites) != 1 || invites[0] != "홍길동" {
		t.Fatalf("invites = %+v, want [홍길동]", invites)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "초대 김철수"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "초대 대상에 추가하였습니다.\n"})
	invites = world.MarriageInvites(42)
	if len(invites) != 2 || invites[0] != "홍길동" || invites[1] != "김철수" {
		t.Fatalf("invites = %+v, want [홍길동 김철수]", invites)
	}

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "초대"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "당신이 초대한 사람들 : \n홍길동\n김철수\n"})

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "초대 홍길동"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "초대 대상에서 삭제하였습니다.\n"})
	assertNoCommand(t, bob)
	invites = world.MarriageInvites(42)
	if len(invites) != 1 || invites[0] != "김철수" {
		t.Fatalf("invites = %+v, want [김철수]", invites)
	}
}

func TestInviteHandlerUsesLegacyInviteGuards(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		mutate func(t *testing.T, world *state.World)
		want   string
	}{
		{name: "empty list", line: "초대", want: "초대한 사람이 없습니다."},
		{name: "non hangul name", line: "초대 Bob", want: "사람의 이름은 한글로 적어야 합니다."},
		{
			name: "no marriage invite authority",
			line: "초대 홍길동",
			mutate: func(t *testing.T, world *state.World) {
				t.Helper()
				if err := world.SetCreatureStat("creature:alice", "marriageID", 0); err != nil {
					t.Fatal(err)
				}
			},
			want: "당신은 사용할 권한이 없습니다.",
		},
		{
			name: "wrong room",
			line: "초대 홍길동",
			mutate: func(t *testing.T, world *state.World) {
				t.Helper()
				if err := world.MovePlayerToRoom("player:alice", "room:away"); err != nil {
					t.Fatal(err)
				}
			},
			want: "당신의 집에서만 가능합니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := marriageInviteTestWorld(t)
			if tt.mutate != nil {
				tt.mutate(t, world)
			}
			loop := NewLoop(marriageTestDispatcher(t, world, NewMarriageRequests()))
			alice := make(chan session.Command, 2)
			bob := make(chan session.Command, 2)
			registerTestSession(t, loop, "s1", alice, "player:alice")
			registerTestSession(t, loop, "s2", bob, "player:bob")

			if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: tt.line}); err != nil {
				t.Fatal(err)
			}
			assertCommand(t, alice, session.Command{Write: tt.want})
			assertNoCommand(t, bob)
			if invites := world.MarriageInvites(42); len(invites) != 0 {
				t.Fatal("invalid invite mutated invite list")
			}
		})
	}
}

func TestMarriageHandlerRequestsAndAcceptsWithRuntimeState(t *testing.T) {
	world := marriageCeremonyTestWorld(t)
	requests := NewMarriageRequests()
	loop := NewLoop(marriageTestDispatcher(t, world, requests))
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "결혼 Alice"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "\nBob님이 당신에게 결혼을 신청합니다."})
	assertCommand(t, bob, session.Command{Write: "당신은 Alice님에게 결혼을 신청하였습니다."})
	assertCreatureMarriageState(t, world, "creature:bob", false, 0, true)
	assertCreatureMarriageState(t, world, "creature:alice", false, 0, false)

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 결혼"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "\nAlice님이 당신의 결혼신청을 받아들였습니다."})
	assertCommand(t, bob, session.Command{Write: "\n### Bob님과 Alice님이 결혼을 하였습니다."})
	assertCommand(t, alice, session.Command{Write: "당신은 Bob님의 결혼신청을 받아들입니다.\n\n### Bob님과 Alice님이 결혼을 하였습니다."})
	assertCreatureMarriageState(t, world, "creature:alice", true, 1, false)
	assertCreatureMarriageState(t, world, "creature:bob", true, 1, false)
}

func TestMarriageHandlerAcceptsPersistedPendingRequestLikeLegacy(t *testing.T) {
	root := t.TempDir()
	world := marriageCeremonyTestWorld(t)
	loop := NewLoop(marriageTestDispatcher(t, world, NewMarriageRequests(), root))
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "결혼 Alice"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "\nBob님이 당신에게 결혼을 신청합니다."})
	assertCommand(t, bob, session.Command{Write: "당신은 Alice님에게 결혼을 신청하였습니다."})
	assertMarriageFile(t, root, "bob", "alice")

	restarted := NewLoop(marriageTestDispatcher(t, world, NewMarriageRequests(), root))
	alice = make(chan session.Command, 4)
	bob = make(chan session.Command, 4)
	registerTestSession(t, restarted, "s1", alice, "player:alice")
	registerTestSession(t, restarted, "s2", bob, "player:bob")

	if err := restarted.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 결혼"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "\nAlice님이 당신의 결혼신청을 받아들였습니다."})
	assertCommand(t, bob, session.Command{Write: "\n### Bob님과 Alice님이 결혼을 하였습니다."})
	assertCommand(t, alice, session.Command{Write: "당신은 Bob님의 결혼신청을 받아들입니다.\n\n### Bob님과 Alice님이 결혼을 하였습니다."})
	assertCreatureMarriageState(t, world, "creature:alice", true, 1, false)
	assertCreatureMarriageState(t, world, "creature:bob", true, 1, false)
	assertMarriageFile(t, root, "alice", "bob")
	assertMarriageFile(t, root, "bob", "alice")
}

func TestMarriageHandlerRejectsPersistedPendingRequestForDifferentPlayerLikeLegacy(t *testing.T) {
	root := t.TempDir()
	world := marriageCeremonyTestWorld(t)
	mustSetMarriageStat(t, world, "creature:bob", "PRDMAR", 1)
	writeRawMarriageFile(t, root, "bob", "carol\n")
	loop := NewLoop(marriageTestDispatcher(t, world, NewMarriageRequests(), root))
	alice := make(chan session.Command, 3)
	bob := make(chan session.Command, 3)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 결혼"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "그사람은 다른 사람과 결혼을 준비중입니다."})
	assertNoCommand(t, bob)
	assertCreatureMarriageState(t, world, "creature:alice", false, 0, false)
	assertCreatureMarriageState(t, world, "creature:bob", false, 0, true)
}

func TestMarriageHandlerCancelsPersistedPendingFlagLikeLegacy(t *testing.T) {
	world := marriageCeremonyTestWorld(t)
	mustSetMarriageStat(t, world, "creature:alice", "PRDMAR", 1)
	loop := NewLoop(marriageTestDispatcher(t, world, NewMarriageRequests()))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "결혼"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, alice, session.Command{Write: "결혼 신청을 취소합니다."})
	assertNoCommand(t, bob)
	assertCreatureMarriageState(t, world, "creature:alice", false, 0, false)
}

func TestMarriageHandlerRejectsInvalidAcceptanceCases(t *testing.T) {
	t.Run("self", func(t *testing.T) {
		world := marriageCeremonyTestWorld(t)
		loop := NewLoop(marriageTestDispatcher(t, world, NewMarriageRequests()))
		alice := make(chan session.Command, 2)
		registerTestSession(t, loop, "s1", alice, "player:alice")

		if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Alice 결혼"}); err != nil {
			t.Fatal(err)
		}
		assertCommand(t, alice, session.Command{Write: "자기 자신과는 결혼할 수 없습니다.\n"})
	})

	t.Run("missing active", func(t *testing.T) {
		world := marriageCeremonyTestWorld(t)
		loop := NewLoop(marriageTestDispatcher(t, world, NewMarriageRequests()))
		alice := make(chan session.Command, 2)
		dave := make(chan session.Command, 2)
		registerTestSession(t, loop, "s1", alice, "player:alice")
		registerTestSession(t, loop, "s4", dave, "player:dave")

		if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Nobody 결혼"}); err != nil {
			t.Fatal(err)
		}
		assertCommand(t, alice, session.Command{Write: "그런 사람을 찾을 수가 없군요."})
		assertNoCommand(t, dave)
	})

	t.Run("different room active target", func(t *testing.T) {
		world := marriageCeremonyTestWorld(t)
		loop := NewLoop(marriageTestDispatcher(t, world, NewMarriageRequests()))
		alice := make(chan session.Command, 2)
		dave := make(chan session.Command, 2)
		registerTestSession(t, loop, "s1", alice, "player:alice")
		registerTestSession(t, loop, "s4", dave, "player:dave")

		if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Dave 결혼"}); err != nil {
			t.Fatal(err)
		}
		assertCommand(t, dave, session.Command{Write: "\nAlice님이 당신에게 결혼을 신청합니다."})
		assertCommand(t, alice, session.Command{Write: "당신은 Dave님에게 결혼을 신청하였습니다."})
		assertCreatureMarriageState(t, world, "creature:alice", false, 0, true)
		assertCreatureMarriageState(t, world, "creature:dave", false, 0, false)
	})

	t.Run("target pending another request", func(t *testing.T) {
		world := marriageCeremonyTestWorld(t)
		requests := NewMarriageRequests()
		loop := NewLoop(marriageTestDispatcher(t, world, requests))
		alice := make(chan session.Command, 3)
		bob := make(chan session.Command, 3)
		carol := make(chan session.Command, 3)
		registerTestSession(t, loop, "s1", alice, "player:alice")
		registerTestSession(t, loop, "s2", bob, "player:bob")
		registerTestSession(t, loop, "s3", carol, "player:carol")

		if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "Carol 결혼"}); err != nil {
			t.Fatal(err)
		}
		assertCommand(t, carol, session.Command{Write: "\nBob님이 당신에게 결혼을 신청합니다."})
		assertCommand(t, bob, session.Command{Write: "당신은 Carol님에게 결혼을 신청하였습니다."})

		if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 결혼"}); err != nil {
			t.Fatal(err)
		}
		assertCommand(t, alice, session.Command{Write: "그사람은 다른 사람과 결혼을 준비중입니다."})
		assertNoCommand(t, bob)
		assertNoCommand(t, carol)
		assertCreatureMarriageState(t, world, "creature:alice", false, 0, false)
		assertCreatureMarriageState(t, world, "creature:bob", false, 0, true)
	})
}

func TestMarriageHandlerRejectsLegacyEligibilityCases(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, world *state.World)
		line   string
		want   string
	}{
		{
			name: "actor too young",
			mutate: func(t *testing.T, world *state.World) {
				mustSetMarriageStat(t, world, "creature:alice", "legacyHoursInterval", 86400)
			},
			line: "Bob 결혼",
			want: "당신은 결혼할 수 있는 나이가 아닙니다.",
		},
		{
			name: "target too young",
			mutate: func(t *testing.T, world *state.World) {
				mustSetMarriageStat(t, world, "creature:bob", "legacyHoursInterval", 86400)
			},
			line: "Bob 결혼",
			want: "그사람은 아직 결혼할 나이가 되지 않았습니다.",
		},
		{
			name: "actor blind",
			mutate: func(t *testing.T, world *state.World) {
				mustSetMarriageStat(t, world, "creature:alice", "PBLIND", 1)
			},
			line: "Bob 결혼",
			want: "그런 사람을 찾을 수가 없군요.",
		},
		{
			name: "target dm invisible",
			mutate: func(t *testing.T, world *state.World) {
				mustSetMarriageStat(t, world, "creature:bob", "PDMINV", 1)
			},
			line: "Bob 결혼",
			want: "그런 사람을 찾을 수가 없군요.",
		},
		{
			name: "target invisible without detect invisible",
			mutate: func(t *testing.T, world *state.World) {
				mustSetMarriageStat(t, world, "creature:bob", "PINVIS", 1)
			},
			line: "Bob 결혼",
			want: "그런 사람을 찾을 수가 없군요.",
		},
		{
			name: "same sex male",
			mutate: func(t *testing.T, world *state.World) {
				mustSetMarriageStat(t, world, "creature:alice", "PMALES", 1)
			},
			line: "Bob 결혼",
			want: "남자끼리 결혼하시려고요?",
		},
		{
			name: "same sex female",
			mutate: func(t *testing.T, world *state.World) {
				mustSetMarriageStat(t, world, "creature:bob", "PMALES", 0)
			},
			line: "Bob 결혼",
			want: "여자끼리 결혼하시려고요?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := marriageCeremonyTestWorld(t)
			tt.mutate(t, world)
			loop := NewLoop(marriageTestDispatcher(t, world, NewMarriageRequests()))
			alice := make(chan session.Command, 2)
			bob := make(chan session.Command, 2)
			registerTestSession(t, loop, "s1", alice, "player:alice")
			registerTestSession(t, loop, "s2", bob, "player:bob")

			if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: tt.line}); err != nil {
				t.Fatal(err)
			}
			assertCommand(t, alice, session.Command{Write: tt.want})
			assertNoCommand(t, bob)
		})
	}
}

func TestMarriageHandlerAllowsInvisibleTargetWithDetectInvisible(t *testing.T) {
	world := marriageCeremonyTestWorld(t)
	mustSetMarriageStat(t, world, "creature:alice", "PDINVI", 1)
	mustSetMarriageStat(t, world, "creature:bob", "PINVIS", 1)
	loop := NewLoop(marriageTestDispatcher(t, world, NewMarriageRequests()))
	alice := make(chan session.Command, 2)
	bob := make(chan session.Command, 2)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 결혼"}); err != nil {
		t.Fatal(err)
	}
	assertCommand(t, bob, session.Command{Write: "\nAlice님이 당신에게 결혼을 신청합니다."})
	assertCommand(t, alice, session.Command{Write: "당신은 Bob님에게 결혼을 신청하였습니다."})
}

func marriageTestDispatcher(t *testing.T, world *state.World, requests *MarriageRequests, roots ...string) enginecmd.Dispatcher {
	t.Helper()
	registry, err := commandspec.NewRegistry([]commandspec.CommandSpec{
		{Name: "초대", Number: 152, Handler: "invite"},
		{Name: "invite", Number: 152, Handler: "invite"},
		{Name: "결혼", Number: 150, Handler: "marriage"},
		{Name: "marriage", Number: 150, Handler: "marriage"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return enginecmd.Dispatcher{
		Registry: registry,
		Handlers: map[string]enginecmd.Handler{
			"invite":   NewInviteHandler(world),
			"marriage": NewMarriageHandler(world, requests, roots...),
		},
	}
}

func marriageInviteTestWorld(t *testing.T) *state.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLoopRoom(t, loaded, model.Room{
		ID:          "room:married",
		DisplayName: "Married Room",
		Properties:  map[string]string{"special": "42"},
		Metadata:    model.Metadata{Tags: []string{"ronmar"}},
	})
	mustAddLoopRoom(t, loaded, model.Room{ID: "room:away", DisplayName: "Away"})
	for _, player := range []model.Player{
		{ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:married"},
		{ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob", RoomID: "room:married"},
		{ID: "player:charlie", DisplayName: "Charlie", CreatureID: "creature:charlie", RoomID: "room:away"},
	} {
		mustAddLoopPlayer(t, loaded, player)
	}
	for _, creature := range []model.Creature{
		{
			ID:          "creature:alice",
			Kind:        model.CreatureKindPlayer,
			DisplayName: "Alice",
			PlayerID:    "player:alice",
			RoomID:      "room:married",
			Stats:       map[string]int{"PMARRI": 1, "marriageID": 42},
		},
		{
			ID:          "creature:bob",
			Kind:        model.CreatureKindPlayer,
			DisplayName: "Bob",
			PlayerID:    "player:bob",
			RoomID:      "room:married",
			Stats:       map[string]int{},
		},
		{
			ID:          "creature:charlie",
			Kind:        model.CreatureKindPlayer,
			DisplayName: "Charlie",
			PlayerID:    "player:charlie",
			RoomID:      "room:away",
			Stats:       map[string]int{},
		},
	} {
		mustAddLoopCreature(t, loaded, creature)
	}
	return state.NewWorld(loaded)
}

func marriageCeremonyTestWorld(t *testing.T) *state.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLoopRoom(t, loaded, model.Room{
		ID:          "room:hall",
		DisplayName: "Marriage Hall",
		Metadata:    model.Metadata{Tags: []string{"rmarri"}},
	})
	mustAddLoopRoom(t, loaded, model.Room{ID: "room:away", DisplayName: "Away"})
	for _, player := range []model.Player{
		{ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:hall"},
		{ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob", RoomID: "room:hall"},
		{ID: "player:carol", DisplayName: "Carol", CreatureID: "creature:carol", RoomID: "room:hall"},
		{ID: "player:dave", DisplayName: "Dave", CreatureID: "creature:dave", RoomID: "room:away"},
	} {
		mustAddLoopPlayer(t, loaded, player)
	}
	for _, creature := range []model.Creature{
		{ID: "creature:alice", Kind: model.CreatureKindPlayer, DisplayName: "Alice", PlayerID: "player:alice", RoomID: "room:hall", Stats: marriageCeremonyStats(false)},
		{ID: "creature:bob", Kind: model.CreatureKindPlayer, DisplayName: "Bob", PlayerID: "player:bob", RoomID: "room:hall", Stats: marriageCeremonyStats(true)},
		{ID: "creature:carol", Kind: model.CreatureKindPlayer, DisplayName: "Carol", PlayerID: "player:carol", RoomID: "room:hall", Stats: marriageCeremonyStats(false)},
		{ID: "creature:dave", Kind: model.CreatureKindPlayer, DisplayName: "Dave", PlayerID: "player:dave", RoomID: "room:away", Stats: marriageCeremonyStats(true)},
	} {
		mustAddLoopCreature(t, loaded, creature)
	}
	return state.NewWorld(loaded)
}

func marriageCeremonyStats(male bool) map[string]int {
	stats := map[string]int{"legacyHoursInterval": 3 * 86400}
	if male {
		stats["PMALES"] = 1
	}
	return stats
}

func TestMarriageRoomSpecialNormalizesPropertyKey(t *testing.T) {
	got, ok := marriageRoomSpecial(model.Room{Properties: map[string]string{"SPECIAL": "42"}})
	if !ok || got != 42 {
		t.Fatalf("marriageRoomSpecial(SPECIAL) = %d/%v, want 42/true", got, ok)
	}
}

func mustSetMarriageStat(t *testing.T, world *state.World, creatureID model.CreatureID, key string, value int) {
	t.Helper()
	if err := world.SetCreatureStat(creatureID, key, value); err != nil {
		t.Fatal(err)
	}
}

func assertCreatureMarriageState(t *testing.T, world *state.World, creatureID model.CreatureID, married bool, marriageID int, pending bool) {
	t.Helper()
	creature, ok := world.Creature(creatureID)
	if !ok {
		t.Fatalf("missing creature %q", creatureID)
	}
	if got := creature.Stats["PMARRI"] != 0; got != married {
		t.Fatalf("%s PMARRI = %v, want %v; stats = %+v", creatureID, got, married, creature.Stats)
	}
	if got := creature.Stats["marriageID"]; got != marriageID {
		t.Fatalf("%s marriageID = %d, want %d; stats = %+v", creatureID, got, marriageID, creature.Stats)
	}
	if got := creature.Stats["PRDMAR"] != 0; got != pending {
		t.Fatalf("%s PRDMAR = %v, want %v; stats = %+v", creatureID, got, pending, creature.Stats)
	}
	if married && !hasTagContaining(creature.Metadata.Tags, "PMARRI") {
		t.Fatalf("%s tags = %+v, want PMARRI", creatureID, creature.Metadata.Tags)
	}
}

func hasTagContaining(tags []string, target string) bool {
	for _, tag := range tags {
		if strings.EqualFold(strings.TrimSpace(tag), target) {
			return true
		}
	}
	return false
}

func TestMarriageHandlerSerialization(t *testing.T) {
	tempDir := t.TempDir()
	world := marriageCeremonyTestWorld(t)
	requests := NewMarriageRequests()
	loop := NewLoop(marriageTestDispatcher(t, world, requests, tempDir))
	alice := make(chan session.Command, 4)
	bob := make(chan session.Command, 4)
	registerTestSession(t, loop, "s1", alice, "player:alice")
	registerTestSession(t, loop, "s2", bob, "player:bob")

	// Alice registers/creates a mock list file under tempDir to test increment logic
	listDir := filepath.Join(tempDir, "player", "marriage")
	if err := os.MkdirAll(listDir, 0755); err != nil {
		t.Fatal(err)
	}
	listPath := filepath.Join(listDir, "list")
	initialList := "2\n1 alice bob\n2 charlie dave\n"
	if err := os.WriteFile(listPath, []byte(initialList), 0644); err != nil {
		t.Fatal(err)
	}

	// 1. Bob requests marriage
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s2", Kind: session.EventLine, Line: "결혼 Alice"}); err != nil {
		t.Fatal(err)
	}
	// Drain commands
	<-alice
	<-bob

	// 2. Alice accepts marriage
	if err := loop.HandleEvent(context.Background(), session.Event{SessionID: "s1", Kind: session.EventLine, Line: "Bob 결혼"}); err != nil {
		t.Fatal(err)
	}
	// Drain commands
	<-bob
	<-bob
	<-alice

	// Verify partner file for Alice (should contain Bob's name "bob")
	alicePartnerFile := filepath.Join(listDir, "alice")
	alicePartner, err := os.ReadFile(alicePartnerFile)
	if err != nil {
		t.Fatalf("failed to read alice partner file: %v", err)
	}
	if strings.TrimSpace(string(alicePartner)) != "bob" {
		t.Fatalf("alice partner = %q, want bob", string(alicePartner))
	}

	// Verify partner file for Bob (should contain Alice's name "alice")
	bobPartnerFile := filepath.Join(listDir, "bob")
	bobPartner, err := os.ReadFile(bobPartnerFile)
	if err != nil {
		t.Fatalf("failed to read bob partner file: %v", err)
	}
	if strings.TrimSpace(string(bobPartner)) != "alice" {
		t.Fatalf("bob partner = %q, want alice", string(bobPartner))
	}

	// Verify global marriage registry list has been incremented and updated
	listContent, err := os.ReadFile(listPath)
	if err != nil {
		t.Fatalf("failed to read list file: %v", err)
	}

	decodedList, err := legacykr.DecodeEUCKR(listContent)
	if err != nil {
		t.Fatalf("failed to decode list file: %v", err)
	}

	expectedList := "3 : bob 님과 alice 님의 결혼\n2\n1 alice bob\n2 charlie dave\n"
	if decodedList != expectedList {
		t.Fatalf("list content =\n%q\nwant:\n%q", decodedList, expectedList)
	}
	assertCreatureMarriageState(t, world, "creature:alice", true, 3, false)
	assertCreatureMarriageState(t, world, "creature:bob", true, 3, false)
}

func assertMarriageFile(t *testing.T, root string, name string, want string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "player", "marriage", name))
	if err != nil {
		t.Fatalf("read marriage file %q: %v", name, err)
	}
	if got := strings.TrimSpace(string(data)); got != want {
		t.Fatalf("marriage file %q = %q, want %q", name, got, want)
	}
}

func writeRawMarriageFile(t *testing.T, root string, name string, content string) {
	t.Helper()
	path := filepath.Join(root, "player", "marriage", name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
