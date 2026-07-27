package main

import (
	"context"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/session"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

// newLoginNameTestWorld builds a minimal world containing a player whose stored
// name is path-unsafe, so the existing-player login branch can be exercised.
func newLoginNameTestWorld(t *testing.T, playerID model.PlayerID, displayName string) *state.World {
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
	if err := loaded.AddCreature(model.Creature{
		ID:          "creature:mallory",
		Kind:        model.CreatureKindPlayer,
		DisplayName: displayName,
		PlayerID:    playerID,
		RoomID:      "room:plaza",
	}); err != nil {
		t.Fatal(err)
	}
	return state.NewWorld(loaded)
}

// TestLoginExistingPlayerRejectsPathUnsafeName closes gap #1: a stored player
// whose name contains a path separator / traversal token must be refused at
// login on the EXISTING-player branch (not just the create branch), so the name
// never becomes a filename component on save or legacy paths.
func TestLoginExistingPlayerRejectsPathUnsafeName(t *testing.T) {
	for _, bad := range []string{"../evil", "a/b", "..", "evil\x00x"} {
		t.Run(bad, func(t *testing.T) {
			world := newLoginNameTestWorld(t, model.PlayerID(bad), bad)
			mgr := newServerLoginManager(world)
			mgr.Start(session.ID("s1"))
			res, err := mgr.HandleLine(context.Background(), session.ID("s1"), bad)
			if err != nil {
				t.Fatalf("HandleLine(%q) error = %v", bad, err)
			}
			got := res.Command.Write
			if strings.Contains(got, "암호를") {
				t.Fatalf("HandleLine(%q) advanced to password prompt %q; want name rejection", bad, got)
			}
			if !strings.Contains(got, "이름") {
				t.Fatalf("HandleLine(%q) write = %q; want a name rejection message", bad, got)
			}
		})
	}
}

// TestLoginExistingPlayerHangulNameAdvances confirms a valid Hangul existing
// player is NOT wrongly rejected by the new check.
func TestLoginExistingPlayerHangulNameAdvances(t *testing.T) {
	world := newLoginNameTestWorld(t, model.PlayerID("무영문"), "무영문")
	mgr := newServerLoginManager(world)
	mgr.Start(session.ID("s1"))
	res, err := mgr.HandleLine(context.Background(), session.ID("s1"), "무영문")
	if err != nil {
		t.Fatalf("HandleLine error = %v", err)
	}
	// No stored password hash -> "저장된 암호 정보를 찾을 수 없습니다", proving we
	// passed the name check and reached the password-hash branch.
	if want := "저장된 암호 정보를 찾을 수 없습니다"; !strings.Contains(res.Command.Write, want) {
		t.Fatalf("HandleLine(valid) write = %q; want %q", res.Command.Write, want)
	}
}

// TestLoginExistingPlayerASCIINameAdvances confirms an ASCII path-safe existing
// name (the test-fixture convention) is NOT rejected: the gate is path-safety,
// not the Hangul game policy.
func TestLoginExistingPlayerASCIINameAdvances(t *testing.T) {
	world := newLoginNameTestWorld(t, model.PlayerID("player:alice"), "player:alice")
	mgr := newServerLoginManager(world)
	mgr.Start(session.ID("s1"))
	res, err := mgr.HandleLine(context.Background(), session.ID("s1"), "player:alice")
	if err != nil {
		t.Fatalf("HandleLine error = %v", err)
	}
	if strings.Contains(res.Command.Write, "접속할 수 없습니다") {
		t.Fatalf("ASCII path-safe name wrongly rejected: %q", res.Command.Write)
	}
}
