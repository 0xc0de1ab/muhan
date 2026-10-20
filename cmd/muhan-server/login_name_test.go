package main

import (
	"context"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/persist/legacycrypt"
	"github.com/0xc0de1ab/muhan/internal/session"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

// newLoginNameTestWorld builds a minimal world containing a player whose
// account has no stored password hash.
func newLoginNameTestWorld(t *testing.T, playerID model.PlayerID, displayName string) *state.World {
	t.Helper()
	return newLoginNameTestWorldWithHash(t, playerID, displayName, "")
}

// newLoginNameTestWorldWithHash is newLoginNameTestWorld but stores hash as
// the account's legacy password hash.
func newLoginNameTestWorldWithHash(t *testing.T, playerID model.PlayerID, displayName string, hash string) *state.World {
	t.Helper()
	loaded := worldload.NewWorld()
	if err := loaded.AddRoom(model.Room{ID: "room:plaza", DisplayName: "광장"}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddPlayer(model.Player{
		ID:          playerID,
		DisplayName: displayName,
		CreatureID:  "creature:mallory",
		RoomID:      "room:plaza",
	}); err != nil {
		t.Fatal(err)
	}
	creature := model.Creature{
		ID:          "creature:mallory",
		Kind:        model.CreatureKindPlayer,
		DisplayName: displayName,
		PlayerID:    playerID,
		RoomID:      "room:plaza",
	}
	if hash != "" {
		creature.Properties = map[string]string{"legacyPasswordHash": hash}
	}
	if err := loaded.AddCreature(creature); err != nil {
		t.Fatal(err)
	}
	return state.NewWorld(loaded)
}

func TestLoginNameStepIsIndistinguishable(t *testing.T) {
	// #15 contract: the name step must answer identically whether the name
	// belongs to an account with a password hash, an account without one, a
	// path-unsafe stored name, or no account at all.
	pwHash, err := legacycrypt.HashBcrypt("비밀1234")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		input string
		world func() *state.World
	}{
		{"registered with hash", "해시영웅", func() *state.World {
			return newLoginNameTestWorldWithHash(t, "해시영웅", "해시영웅", pwHash)
		}},
		{"registered without hash", "무영문", func() *state.World {
			return newLoginNameTestWorld(t, "무영문", "무영문")
		}},
		{"path-unsafe stored name", "../evil", func() *state.World {
			return newLoginNameTestWorld(t, "../evil", "../evil")
		}},
		{"no account", "없는이름", func() *state.World {
			return newLoginNameTestWorld(t, "irrelevant", "irrelevant")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newServerLoginManager(tc.world())
			mgr.Start(session.ID("s1"))
			res, err := mgr.HandleLine(context.Background(), session.ID("s1"), tc.input)
			if err != nil {
				t.Fatalf("HandleLine(%q) error = %v", tc.input, err)
			}
			if res.Command.Write != loginPasswordPrompt {
				t.Fatalf("HandleLine(%q) name-step write = %q; want uniform %q",
					tc.input, res.Command.Write, loginPasswordPrompt)
			}
			if res.ActorID != "" {
				t.Fatalf("HandleLine(%q) ActorID = %q after name step; want unauthenticated", tc.input, res.ActorID)
			}
		})
	}
}

func TestLoginPathUnsafeStoredNameCanNeverAuthenticate(t *testing.T) {
	// The path-safety floor survives #15: the name step answers like every
	// other name, but the account can never authenticate — attempts fail
	// exactly like wrong passwords and never reach a file operation.
	world := newLoginNameTestWorld(t, "../evil", "../evil")
	mgr := newServerLoginManager(world)
	mgr.Start(session.ID("s1"))
	res, err := mgr.HandleLine(context.Background(), session.ID("s1"), "../evil")
	if err != nil {
		t.Fatalf("HandleLine error = %v", err)
	}
	if res.Command.Write != loginPasswordPrompt {
		t.Fatalf("name-step write = %q; want uniform %q", res.Command.Write, loginPasswordPrompt)
	}
	for i := 0; i < 3; i++ {
		res, err = mgr.HandleLine(context.Background(), session.ID("s1"), "guess")
		if err != nil {
			t.Fatalf("password attempt %d error = %v", i+1, err)
		}
		if res.ActorID != "" {
			t.Fatalf("path-unsafe account authenticated on attempt %d", i+1)
		}
		if !strings.Contains(res.Command.Write, "암호가 틀립니다") {
			t.Fatalf("attempt %d write = %q; want wrong-password failure", i+1, res.Command.Write)
		}
	}
	if !res.Command.Close {
		t.Fatal("want session closed after three failed attempts")
	}
}

func TestLoginNoHashAccountFailsLikeWrongPassword(t *testing.T) {
	// #15: an existing account with no stored password hash can never
	// authenticate; the password step must behave exactly like a
	// wrong-password attempt on a healthy account instead of announcing the
	// missing hash.
	world := newLoginNameTestWorld(t, "무영문", "무영문")
	mgr := newServerLoginManager(world)
	mgr.Start(session.ID("s1"))
	res, err := mgr.HandleLine(context.Background(), session.ID("s1"), "무영문")
	if err != nil {
		t.Fatalf("HandleLine error = %v", err)
	}
	if res.Command.Write != loginPasswordPrompt {
		t.Fatalf("name-step write = %q; want uniform %q", res.Command.Write, loginPasswordPrompt)
	}
	res, err = mgr.HandleLine(context.Background(), session.ID("s1"), "guess")
	if err != nil {
		t.Fatalf("HandleLine error = %v", err)
	}
	if strings.Contains(res.Command.Write, "저장된 암호 정보") {
		t.Fatalf("password-step write = %q; must not announce the missing hash", res.Command.Write)
	}
	if !strings.Contains(res.Command.Write, "암호가 틀립니다") {
		t.Fatalf("password-step write = %q; want wrong-password failure", res.Command.Write)
	}
	if res.ActorID != "" {
		t.Fatal("no-hash account authenticated; want failure")
	}
}

func TestLoginCreateCandidateProceedsToConfirm(t *testing.T) {
	// The creation flow still works under #15: a new name passes the name
	// step and the password gate, then reaches the creation confirmation.
	world := newLoginNameTestWorld(t, "irrelevant", "irrelevant")
	mgr := newServerLoginManager(world)
	mgr.Start(session.ID("s1"))
	res, err := mgr.HandleLine(context.Background(), session.ID("s1"), "새영웅")
	if err != nil {
		t.Fatalf("HandleLine error = %v", err)
	}
	if res.Command.Write != loginPasswordPrompt {
		t.Fatalf("name-step write = %q; want uniform %q", res.Command.Write, loginPasswordPrompt)
	}
	res, err = mgr.HandleLine(context.Background(), session.ID("s1"), "임의암호")
	if err != nil {
		t.Fatalf("HandleLine error = %v", err)
	}
	if want := "새영웅으로 하시겠습니까(예/아니오)? "; !strings.Contains(res.Command.Write, want) {
		t.Fatalf("gate write = %q; want creation confirm %q", res.Command.Write, want)
	}
}

func TestLoginCreateCandidateProbeCap(t *testing.T) {
	// #15 throttle: cycling names (password gate -> confirm -> "아니오" ->
	// next name) within one session is capped; the session is dropped after
	// maxLoginNameProbes candidates.
	world := newLoginNameTestWorld(t, "irrelevant", "irrelevant")
	mgr := newServerLoginManager(world)
	mgr.Start(session.ID("s1"))
	for i := 0; i < maxLoginNameProbes; i++ {
		res, err := mgr.HandleLine(context.Background(), session.ID("s1"), "새영웅")
		if err != nil {
			t.Fatalf("probe %d name error = %v", i, err)
		}
		if res.Command.Write != loginPasswordPrompt {
			t.Fatalf("probe %d name-step write = %q", i, res.Command.Write)
		}
		res, err = mgr.HandleLine(context.Background(), session.ID("s1"), "임의암호")
		if err != nil {
			t.Fatalf("probe %d gate error = %v", i, err)
		}
		if !strings.Contains(res.Command.Write, "하시겠습니까(예/아니오)? ") {
			t.Fatalf("probe %d gate write = %q; want creation confirm", i, res.Command.Write)
		}
		res, err = mgr.HandleLine(context.Background(), session.ID("s1"), "아니오")
		if err != nil {
			t.Fatalf("probe %d decline error = %v", i, err)
		}
	}
	res, err := mgr.HandleLine(context.Background(), session.ID("s1"), "새영웅")
	if err != nil {
		t.Fatalf("overflow name error = %v", err)
	}
	res, err = mgr.HandleLine(context.Background(), session.ID("s1"), "임의암호")
	if err != nil {
		t.Fatalf("overflow gate error = %v", err)
	}
	if !res.Command.Close {
		t.Fatalf("overflow gate write = %q; want the session dropped", res.Command.Write)
	}
}
