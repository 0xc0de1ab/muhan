package game

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	enginecmd "github.com/0xc0de1ab/muhan/internal/engine/command"
	"github.com/0xc0de1ab/muhan/internal/session"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestFamilyJoinRequestAndBossApprove(t *testing.T) {
	world := familyMembershipTestWorld(t)
	root := t.TempDir()
	requests := NewFamilyMembershipRequests()
	active := familyMembershipActiveSessions()
	writes := map[session.ID]string{}

	joinCtx := familyMembershipSubmitJoinRequest(t, world, requests, "s-bob", "player:bob", active, writes, "무영문")
	if out := joinCtx.OutputString(); !strings.Contains(out, "가입 신청") || !strings.Contains(out, "허가") {
		t.Fatalf("join output = %q", out)
	}
	if !strings.Contains(writes["s-alice"], "Bob") || !strings.Contains(writes["s-alice"], "가입") {
		t.Fatalf("boss notification = %q", writes["s-alice"])
	}
	bob := familyMembershipCreature(t, world, "creature:bob")
	if bob.Stats["familyFlag"] != 0 || bob.Stats["familyID"] != 2 || bob.Stats["PRDFML"] != 1 {
		t.Fatalf("pending bob stats = %+v", bob.Stats)
	}
	if !familyMembershipTestHasTag(bob, "PRDFML") || familyMembershipTestHasTag(bob, "PFAMIL") {
		t.Fatalf("pending bob tags = %+v", bob.Metadata.Tags)
	}

	approveCtx := familyMembershipTestContext("s-alice", "player:alice", active, writes)
	if _, err := NewFamilyJoinApproveHandler(world, root, requests)(approveCtx, enginecmd.ResolvedCommand{Args: []string{"Bob"}}); err != nil {
		t.Fatal(err)
	}
	if out := approveCtx.OutputString(); !strings.Contains(out, "가입을 허가") {
		t.Fatalf("approve output = %q", out)
	}
	bob = familyMembershipCreature(t, world, "creature:bob")
	if bob.Stats["familyFlag"] != 1 || bob.Stats["PFAMIL"] != 1 || bob.Stats["familyID"] != 2 ||
		bob.Stats["PRDFML"] != 0 || bob.Stats["PFMBOS"] != 0 {
		t.Fatalf("approved bob stats = %+v", bob.Stats)
	}
	if !familyMembershipTestHasTag(bob, "PFAMIL") || familyMembershipTestHasTag(bob, "PRDFML") ||
		familyMembershipTestHasTag(bob, "PFMBOS") {
		t.Fatalf("approved bob tags = %+v", bob.Metadata.Tags)
	}
	if !strings.Contains(writes["s-bob"], "가입축하금") {
		t.Fatalf("applicant notification = %q", writes["s-bob"])
	}
}

func TestFamilyJoinApproveQueuesApplicantSaveLikeLegacy(t *testing.T) {
	baseWorld := familyMembershipTestWorld(t)
	world := &dirtyTrackingFamilyMembershipWorld{World: baseWorld}
	root := t.TempDir()
	requests := NewFamilyMembershipRequests()
	active := familyMembershipActiveSessions()
	writes := map[session.ID]string{}

	familyMembershipSubmitJoinRequest(t, world, requests, "s-bob", "player:bob", active, writes, "무영문")
	approveCtx := familyMembershipTestContext("s-alice", "player:alice", active, writes)
	status, err := NewFamilyJoinApproveHandler(world, root, requests)(approveCtx, enginecmd.ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("approve handler error = %v", err)
	}
	if status != enginecmd.StatusDefault || !strings.Contains(approveCtx.OutputString(), "가입을 허가") {
		t.Fatalf("status/output = %d/%q, want approve success", status, approveCtx.OutputString())
	}
	if !world.markedDirty("player:bob") {
		t.Fatalf("approved applicant was not marked dirty: %+v", world.dirtyPlayers)
	}
	if !world.queuedSave("player:bob") {
		t.Fatalf("approved applicant was not queued for save: %+v", world.queuedPlayers)
	}
}

func TestFamilyJoinRequiresOnlineBossAndRejectsClosedFamilyLikeLegacy(t *testing.T) {
	t.Run("offline boss", func(t *testing.T) {
		world := familyMembershipTestWorld(t)
		requests := NewFamilyMembershipRequests()
		ctx := familyMembershipTestContext("s-bob", "player:bob", familyMembershipActiveSessionsWithout("player:carol"), nil)

		status, err := NewFamilyJoinHandler(world, requests)(ctx, enginecmd.ResolvedCommand{Args: []string{"은형문"}})
		if err != nil {
			t.Fatalf("join handler error = %v", err)
		}
		if want := "패거리의 두목인 Carol님은 현재 이용중이 아닙니다."; status != enginecmd.StatusDefault || ctx.OutputString() != want {
			t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
		}
		bob := familyMembershipCreature(t, world, "creature:bob")
		if bob.Stats["PRDFML"] != 0 || bob.Stats["familyID"] != 0 {
			t.Fatalf("offline boss join mutated bob stats = %+v", bob.Stats)
		}
	})

	t.Run("closed family", func(t *testing.T) {
		world := familyMembershipTestWorld(t)
		family, found := familyMemberFamily(world.Families(), 2)
		if !found {
			t.Fatal("family 2 not found")
		}
		family.BossName = "*단체*"
		if err := world.UpdateFamily(family); err != nil {
			t.Fatal(err)
		}
		ctx := familyMembershipTestContext("s-bob", "player:bob", familyMembershipActiveSessions(), nil)

		status, err := NewFamilyJoinHandler(world, NewFamilyMembershipRequests())(ctx, enginecmd.ResolvedCommand{Args: []string{"무영문"}})
		if err != nil {
			t.Fatalf("join handler error = %v", err)
		}
		if want := "단체의 패거리입니다."; status != enginecmd.StatusDefault || ctx.OutputString() != want {
			t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
		}
		bob := familyMembershipCreature(t, world, "creature:bob")
		if bob.Stats["PRDFML"] != 0 || bob.Stats["familyID"] != 0 {
			t.Fatalf("closed family join mutated bob stats = %+v", bob.Stats)
		}
	})
}

func TestFamilyJoinUsesLegacyConfirmationBeforePendingRequest(t *testing.T) {
	t.Run("direct argument waits for yes", func(t *testing.T) {
		world := familyMembershipTestWorld(t)
		requests := NewFamilyMembershipRequests()
		active := familyMembershipActiveSessions()
		writes := map[session.ID]string{}
		var pending enginecmd.PendingLineHandler
		ctx := familyMembershipTestContextWithPending("s-bob", "player:bob", active, writes, &pending)

		status, err := NewFamilyJoinHandler(world, requests)(ctx, enginecmd.ResolvedCommand{Args: []string{"무영문"}})
		if err != nil {
			t.Fatalf("join handler error = %v", err)
		}
		if status != enginecmd.StatusDoPrompt || pending == nil {
			t.Fatalf("join status/pending = %d/%v, want prompt", status, pending != nil)
		}
		if ctx.OutputString() != "무영문에 가입을 하시겠습니까? (예/아니오) " {
			t.Fatalf("join prompt = %q", ctx.OutputString())
		}
		if _, ok := requests.get("player:bob"); ok {
			t.Fatal("join request was stored before confirmation")
		}
		bob := familyMembershipCreature(t, world, "creature:bob")
		if bob.Stats["PRDFML"] != 0 || bob.Stats["familyID"] != 0 {
			t.Fatalf("join prompt mutated bob stats before confirmation: %+v", bob.Stats)
		}
		if writes["s-alice"] != "" {
			t.Fatalf("boss notified before confirmation: %q", writes["s-alice"])
		}

		ctx.Output = nil
		status, err = pending(ctx, "아니오")
		if err != nil {
			t.Fatalf("join cancel confirmation error = %v", err)
		}
		if status != enginecmd.StatusDefault || ctx.OutputString() != "\n가입 신청을 취소합니다." {
			t.Fatalf("cancel status/output = %d/%q", status, ctx.OutputString())
		}
		if _, ok := requests.get("player:bob"); ok {
			t.Fatal("join request was stored after negative confirmation")
		}
		bob = familyMembershipCreature(t, world, "creature:bob")
		if bob.Stats["PRDFML"] != 0 || bob.Stats["familyID"] != 0 {
			t.Fatalf("negative confirmation mutated bob stats: %+v", bob.Stats)
		}
	})

	t.Run("no argument prompts family name then confirms", func(t *testing.T) {
		world := familyMembershipTestWorld(t)
		requests := NewFamilyMembershipRequests()
		active := familyMembershipActiveSessions()
		writes := map[session.ID]string{}
		var pending enginecmd.PendingLineHandler
		ctx := familyMembershipTestContextWithPending("s-bob", "player:bob", active, writes, &pending)

		status, err := NewFamilyJoinHandler(world, requests)(ctx, enginecmd.ResolvedCommand{})
		if err != nil {
			t.Fatalf("join handler error = %v", err)
		}
		if status != enginecmd.StatusDoPrompt || pending == nil {
			t.Fatalf("list status/pending = %d/%v, want prompt", status, pending != nil)
		}
		if out := ctx.OutputString(); !strings.Contains(out, "다음과 같은 패거리가 있습니다.") ||
			!strings.Contains(out, "패거리의 이름을 입력해 주십시요.  ") {
			t.Fatalf("family list output = %q", out)
		}

		ctx.Output = nil
		status, err = pending(ctx, "무영문")
		if err != nil {
			t.Fatalf("family selection error = %v", err)
		}
		if status != enginecmd.StatusDoPrompt || pending == nil ||
			ctx.OutputString() != "무영문에 가입을 하시겠습니까? (예/아니오) " {
			t.Fatalf("selection status/pending/output = %d/%v/%q", status, pending != nil, ctx.OutputString())
		}
		if _, ok := requests.get("player:bob"); ok {
			t.Fatal("join request was stored before yes confirmation")
		}

		ctx.Output = nil
		status, err = pending(ctx, "예")
		if err != nil {
			t.Fatalf("join yes confirmation error = %v", err)
		}
		if status != enginecmd.StatusDefault ||
			ctx.OutputString() != "\n가입 신청을 하였습니다. \n패거리 두목의 허가를 기다리십시요." {
			t.Fatalf("yes status/output = %d/%q", status, ctx.OutputString())
		}
		if !strings.Contains(writes["s-alice"], "Bob") || !strings.Contains(writes["s-alice"], "가입하기를 원합니다") {
			t.Fatalf("boss notification = %q", writes["s-alice"])
		}
		bob := familyMembershipCreature(t, world, "creature:bob")
		if bob.Stats["PRDFML"] != 1 || bob.Stats["familyID"] != 2 {
			t.Fatalf("confirmed join did not mark pending bob stats: %+v", bob.Stats)
		}
	})
}

func TestFamilyJoinApproveLegacySuccessOutputAndBroadcast(t *testing.T) {
	world := familyMembershipTestWorld(t)
	root := t.TempDir()
	requests := NewFamilyMembershipRequests()
	active := familyMembershipActiveSessions()
	writes := map[session.ID]string{}
	var broadcasts []string

	family, found := familyMemberFamily(world.Families(), 2)
	if !found {
		t.Fatal("family 2 not found")
	}
	family.JoinSubsidy = 5
	if err := world.UpdateFamily(family); err != nil {
		t.Fatal(err)
	}

	familyMembershipSubmitJoinRequest(t, world, requests, "s-bob", "player:bob", active, writes, "무영문")

	ctx := familyMembershipTestContext("s-alice", "player:alice", active, writes)
	ctx.Values[ContextBroadcastKey] = func(cmd session.Command) error {
		broadcasts = append(broadcasts, cmd.Write)
		return nil
	}
	status, err := NewFamilyJoinApproveHandler(world, root, requests)(ctx, enginecmd.ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("approve handler error = %v", err)
	}

	wantOut := "Bob님의 패거리 가입을 허가하였습니다.\n" +
		"Bob님에게 가입축하금을 지급하였습니다." +
		"패거리 금고의 총액이 5만냥이 되었습니다."
	if status != enginecmd.StatusDefault || ctx.OutputString() != wantOut {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), wantOut)
	}
	if got, want := writes["s-bob"], "\n당신은 당신의 패거리로부터 가입축하금을 받았습니다.\n당신은 이제 50000냥을 갖고 있습니다."; got != want {
		t.Fatalf("applicant notification = %q, want %q", got, want)
	}
	if len(broadcasts) != 1 || broadcasts[0] != "\n### Bob님이 무영문에 가입을 하였습니다." {
		t.Fatalf("broadcasts = %+v", broadcasts)
	}
}

func TestFamilyJoinCancelClearsPendingRequest(t *testing.T) {
	world := familyMembershipTestWorld(t)
	requests := NewFamilyMembershipRequests()
	active := familyMembershipActiveSessions()
	writes := map[session.ID]string{}

	familyMembershipSubmitJoinRequest(t, world, requests, "s-bob", "player:bob", active, writes, "무영문")
	cancelCtx := familyMembershipTestContext("s-alice", "player:alice", active, writes)
	if _, err := NewFamilyJoinCancelHandler(world, requests)(cancelCtx, enginecmd.ResolvedCommand{Args: []string{"Bob"}}); err != nil {
		t.Fatal(err)
	}
	if out := cancelCtx.OutputString(); !strings.Contains(out, "가입을 취소") {
		t.Fatalf("cancel output = %q", out)
	}
	bob := familyMembershipCreature(t, world, "creature:bob")
	if bob.Stats["familyFlag"] != 0 || bob.Stats["familyID"] != 0 || bob.Stats["PRDFML"] != 0 {
		t.Fatalf("canceled bob stats = %+v", bob.Stats)
	}
	if familyMembershipTestHasTag(bob, "PRDFML") || familyMembershipTestHasTag(bob, "PFAMIL") {
		t.Fatalf("canceled bob tags = %+v", bob.Metadata.Tags)
	}
	if !strings.Contains(writes["s-bob"], "취소") {
		t.Fatalf("cancel notification = %q", writes["s-bob"])
	}
}

func TestFamilyJoinApproveLegacyFailureOutput(t *testing.T) {
	tests := []struct {
		name   string
		actor  model.PlayerID
		active []ActiveSession
		args   []string
		want   string
	}{
		{
			name:   "non-boss",
			actor:  "player:dave",
			active: familyMembershipActiveSessions(),
			args:   []string{"Bob"},
			want:   "패거리의 문주만이 가능합니다.",
		},
		{
			name:   "missing target",
			actor:  "player:alice",
			active: familyMembershipActiveSessions(),
			want:   "누구의 가입을 허가 하시겠습니까?",
		},
		{
			name:   "inactive target",
			actor:  "player:alice",
			active: familyMembershipActiveSessionsWithout("player:bob"),
			args:   []string{"Bob"},
			want:   "현재 이용중이 아닙니다.",
		},
		{
			name:   "not pending",
			actor:  "player:alice",
			active: familyMembershipActiveSessions(),
			args:   []string{"Dave"},
			want:   "당신의 패거리에 가입신청을 한 사람이 아닙니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := familyMembershipTestWorld(t)
			ctx := familyMembershipTestContext("s-test", tt.actor, tt.active, nil)
			status, err := NewFamilyJoinApproveHandler(world, t.TempDir(), NewFamilyMembershipRequests())(ctx, enginecmd.ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("approve handler error = %v", err)
			}
			if status != enginecmd.StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestFamilyJoinApproveUsesLegacyVisibilityRules(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(t *testing.T, world *state.World)
		want     string
		approved bool
	}{
		{
			name: "boss blind",
			mutate: func(t *testing.T, world *state.World) {
				t.Helper()
				if err := world.SetCreatureStat("creature:alice", "PBLIND", 1); err != nil {
					t.Fatal(err)
				}
			},
			want: "현재 이용중이 아닙니다.",
		},
		{
			name: "target dm invisible",
			mutate: func(t *testing.T, world *state.World) {
				t.Helper()
				if err := world.SetCreatureStat("creature:bob", "PDMINV", 1); err != nil {
					t.Fatal(err)
				}
			},
			want: "현재 이용중이 아닙니다.",
		},
		{
			name: "target invisible without detect",
			mutate: func(t *testing.T, world *state.World) {
				t.Helper()
				if err := world.SetCreatureStat("creature:bob", "PINVIS", 1); err != nil {
					t.Fatal(err)
				}
			},
			want: "현재 이용중이 아닙니다.",
		},
		{
			name: "target invisible with detect",
			mutate: func(t *testing.T, world *state.World) {
				t.Helper()
				if err := world.SetCreatureStat("creature:bob", "PINVIS", 1); err != nil {
					t.Fatal(err)
				}
				if err := world.SetCreatureStat("creature:alice", "PDINVI", 1); err != nil {
					t.Fatal(err)
				}
			},
			want:     "Bob님의 패거리 가입을 허가하였습니다.\nBob님에게 가입축하금을 지급하였습니다.패거리 금고의 총액이 10만냥이 되었습니다.",
			approved: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := familyMembershipTestWorld(t)
			root := t.TempDir()
			requests := NewFamilyMembershipRequests()
			active := familyMembershipActiveSessions()
			writes := map[session.ID]string{}
			familyMembershipSubmitJoinRequest(t, world, requests, "s-bob", "player:bob", active, writes, "무영문")
			writes = map[session.ID]string{}
			tt.mutate(t, world)

			ctx := familyMembershipTestContext("s-alice", "player:alice", active, writes)
			status, err := NewFamilyJoinApproveHandler(world, root, requests)(ctx, enginecmd.ResolvedCommand{Args: []string{"Bob"}})
			if err != nil {
				t.Fatalf("approve handler error = %v", err)
			}
			if status != enginecmd.StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			bob := familyMembershipCreature(t, world, "creature:bob")
			if tt.approved {
				if bob.Stats["PFAMIL"] != 1 || bob.Stats["PRDFML"] != 0 {
					t.Fatalf("approved bob stats = %+v", bob.Stats)
				}
			} else if bob.Stats["PFAMIL"] != 0 || bob.Stats["PRDFML"] != 1 {
				t.Fatalf("blocked approval mutated bob stats = %+v", bob.Stats)
			}
		})
	}
}

func TestFamilyJoinApproveAcceptsPersistedPendingFlagLikeLegacy(t *testing.T) {
	world := familyMembershipTestWorld(t)
	root := t.TempDir()
	if _, err := world.UpdateCreatureFamilyState("creature:bob", 2, false, true, false); err != nil {
		t.Fatal(err)
	}
	ctx := familyMembershipTestContext("s-alice", "player:alice", familyMembershipActiveSessions(), map[session.ID]string{})

	status, err := NewFamilyJoinApproveHandler(world, root, NewFamilyMembershipRequests())(ctx, enginecmd.ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("approve handler error = %v", err)
	}
	want := "Bob님의 패거리 가입을 허가하였습니다.\n" +
		"Bob님에게 가입축하금을 지급하였습니다." +
		"패거리 금고의 총액이 10만냥이 되었습니다."
	if status != enginecmd.StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
	bob := familyMembershipCreature(t, world, "creature:bob")
	if bob.Stats["PFAMIL"] != 1 || bob.Stats["PRDFML"] != 0 || bob.Stats["familyID"] != 2 {
		t.Fatalf("approved persisted pending bob stats = %+v", bob.Stats)
	}
	verifyTestFamilyMemberFile(t, filepath.Join(root, "player", "family", "family_member_2"), []legacyMember{
		{classID: 4, name: "Bob"},
	}, "무영문")
}

func TestFamilyJoinApproveLegacyInsufficientFundsOutput(t *testing.T) {
	world := familyMembershipTestWorld(t)
	requests := NewFamilyMembershipRequests()
	active := familyMembershipActiveSessions()
	writes := map[session.ID]string{}

	family, found := familyMemberFamily(world.Families(), 2)
	if !found {
		t.Fatal("family 2 not found")
	}
	family.JoinSubsidy = 20
	if err := world.UpdateFamily(family); err != nil {
		t.Fatal(err)
	}
	familyMembershipSubmitJoinRequest(t, world, requests, "s-bob", "player:bob", active, writes, "무영문")

	ctx := familyMembershipTestContext("s-alice", "player:alice", active, writes)
	status, err := NewFamilyJoinApproveHandler(world, t.TempDir(), requests)(ctx, enginecmd.ResolvedCommand{Args: []string{"Bob"}})
	if err != nil {
		t.Fatalf("approve handler error = %v", err)
	}
	if want := "당신의 패거리 자금사정으로는 패거리원을 받을수 없습니다."; status != enginecmd.StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
}

func TestFamilyJoinCancelLegacyOutput(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		world := familyMembershipTestWorld(t)
		requests := NewFamilyMembershipRequests()
		active := familyMembershipActiveSessions()
		writes := map[session.ID]string{}

		familyMembershipSubmitJoinRequest(t, world, requests, "s-bob", "player:bob", active, writes, "무영문")
		ctx := familyMembershipTestContext("s-alice", "player:alice", active, writes)
		status, err := NewFamilyJoinCancelHandler(world, requests)(ctx, enginecmd.ResolvedCommand{Args: []string{"Bob"}})
		if err != nil {
			t.Fatalf("cancel handler error = %v", err)
		}
		if status != enginecmd.StatusDefault || ctx.OutputString() != "Bob님의 패거리 가입을 취소하였습니다.\n" {
			t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
		}
		if writes["s-bob"] != "당신의 패거리 가입이 취소되었습니다.\n" {
			t.Fatalf("notification = %q", writes["s-bob"])
		}
	})

	tests := []struct {
		name   string
		actor  model.PlayerID
		active []ActiveSession
		args   []string
		want   string
	}{
		{
			name:   "non-boss",
			actor:  "player:dave",
			active: familyMembershipActiveSessions(),
			args:   []string{"Bob"},
			want:   "패거리의 문주만이 가능합니다.",
		},
		{
			name:   "missing target",
			actor:  "player:alice",
			active: familyMembershipActiveSessions(),
			want:   "누구의 가입의 취소시키려구요?",
		},
		{
			name:   "inactive target",
			actor:  "player:alice",
			active: familyMembershipActiveSessionsWithout("player:bob"),
			args:   []string{"Bob"},
			want:   "현재 이용중이 아닙니다.",
		},
		{
			name:   "not pending",
			actor:  "player:alice",
			active: familyMembershipActiveSessions(),
			args:   []string{"Dave"},
			want:   "당신의 패거리에 가입신청을 한 사람이 아닙니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := familyMembershipTestWorld(t)
			ctx := familyMembershipTestContext("s-test", tt.actor, tt.active, nil)
			status, err := NewFamilyJoinCancelHandler(world, NewFamilyMembershipRequests())(ctx, enginecmd.ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("cancel handler error = %v", err)
			}
			if status != enginecmd.StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestFamilyJoinCancelUsesLegacyVisibilityAndPersistedPending(t *testing.T) {
	t.Run("persisted pending flag", func(t *testing.T) {
		world := familyMembershipTestWorld(t)
		if _, err := world.UpdateCreatureFamilyState("creature:bob", 2, false, true, false); err != nil {
			t.Fatal(err)
		}
		writes := map[session.ID]string{}
		ctx := familyMembershipTestContext("s-alice", "player:alice", familyMembershipActiveSessions(), writes)
		status, err := NewFamilyJoinCancelHandler(world, NewFamilyMembershipRequests())(ctx, enginecmd.ResolvedCommand{Args: []string{"Bob"}})
		if err != nil {
			t.Fatalf("cancel handler error = %v", err)
		}
		if want := "Bob님의 패거리 가입을 취소하였습니다.\n"; status != enginecmd.StatusDefault || ctx.OutputString() != want {
			t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
		}
		if got := writes["s-bob"]; got != "당신의 패거리 가입이 취소되었습니다.\n" {
			t.Fatalf("notification = %q", got)
		}
		bob := familyMembershipCreature(t, world, "creature:bob")
		if bob.Stats["PRDFML"] != 0 || bob.Stats["familyID"] != 0 {
			t.Fatalf("cancel persisted pending bob stats = %+v", bob.Stats)
		}
	})

	t.Run("hidden target", func(t *testing.T) {
		world := familyMembershipTestWorld(t)
		requests := NewFamilyMembershipRequests()
		active := familyMembershipActiveSessions()
		writes := map[session.ID]string{}
		familyMembershipSubmitJoinRequest(t, world, requests, "s-bob", "player:bob", active, writes, "무영문")
		if err := world.SetCreatureStat("creature:bob", "PINVIS", 1); err != nil {
			t.Fatal(err)
		}
		ctx := familyMembershipTestContext("s-alice", "player:alice", active, map[session.ID]string{})
		status, err := NewFamilyJoinCancelHandler(world, requests)(ctx, enginecmd.ResolvedCommand{Args: []string{"Bob"}})
		if err != nil {
			t.Fatalf("cancel handler error = %v", err)
		}
		if want := "현재 이용중이 아닙니다."; status != enginecmd.StatusDefault || ctx.OutputString() != want {
			t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
		}
		bob := familyMembershipCreature(t, world, "creature:bob")
		if bob.Stats["PRDFML"] != 1 || bob.Stats["familyID"] != 2 {
			t.Fatalf("hidden cancel mutated bob stats = %+v", bob.Stats)
		}
	})
}

func TestFamilyLeaveClearsMemberAndPendingStates(t *testing.T) {
	world := familyMembershipTestWorld(t)
	root := t.TempDir()
	requests := NewFamilyMembershipRequests()
	active := familyMembershipActiveSessions()
	writes := map[session.ID]string{}

	var leavePending enginecmd.PendingLineHandler
	leaveCtx := familyMembershipTestContextWithPending("s-dave", "player:dave", active, writes, &leavePending)
	status, err := NewFamilyLeaveHandler(world, root, requests)(leaveCtx, enginecmd.ResolvedCommand{})
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDoPrompt || leavePending == nil {
		t.Fatalf("leave status/pending = %d/%v, want prompt", status, leavePending != nil)
	}
	status, err = leavePending(leaveCtx, "예")
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDefault {
		t.Fatalf("leave confirm status = %d", status)
	}
	if out := leaveCtx.OutputString(); !strings.Contains(out, "탈퇴") {
		t.Fatalf("leave output = %q", out)
	}
	dave := familyMembershipCreature(t, world, "creature:dave")
	if dave.Stats["familyFlag"] != 0 || dave.Stats["familyID"] != 0 || dave.Stats["PFMBOS"] != 0 {
		t.Fatalf("left dave stats = %+v", dave.Stats)
	}

	familyMembershipSubmitJoinRequest(t, world, requests, "s-bob", "player:bob", active, writes, "무영문")
	pendingCtx := familyMembershipTestContext("s-bob", "player:bob", active, writes)
	if _, err := NewFamilyLeaveHandler(world, root, requests)(pendingCtx, enginecmd.ResolvedCommand{}); err != nil {
		t.Fatal(err)
	}
	if out := pendingCtx.OutputString(); !strings.Contains(out, "가입신청을 취소") {
		t.Fatalf("pending leave output = %q", out)
	}
	bob := familyMembershipCreature(t, world, "creature:bob")
	if bob.Stats["familyFlag"] != 0 || bob.Stats["familyID"] != 0 || bob.Stats["PRDFML"] != 0 {
		t.Fatalf("pending canceled bob stats = %+v", bob.Stats)
	}
}

func TestFamilyLeaveLegacyImmediateFailureOutput(t *testing.T) {
	t.Run("pending request cancel", func(t *testing.T) {
		world := familyMembershipTestWorld(t)
		requests := NewFamilyMembershipRequests()
		active := familyMembershipActiveSessions()
		writes := map[session.ID]string{}

		familyMembershipSubmitJoinRequest(t, world, requests, "s-bob", "player:bob", active, writes, "무영문")
		ctx := familyMembershipTestContext("s-bob", "player:bob", active, writes)
		status, err := NewFamilyLeaveHandler(world, t.TempDir(), requests)(ctx, enginecmd.ResolvedCommand{})
		if err != nil {
			t.Fatalf("leave handler error = %v", err)
		}
		if status != enginecmd.StatusDefault || ctx.OutputString() != "패거리 가입신청을 취소합니다." {
			t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
		}
	})

	tests := []struct {
		name  string
		actor model.PlayerID
		want  string
	}{
		{
			name:  "boss",
			actor: "player:alice",
			want:  "패거리의 두목은 탈퇴를 할수 없습니다.",
		},
		{
			name:  "not member",
			actor: "player:erin",
			want:  "당신은 어떤 패거리에도 가입이 되어 있지 않습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := familyMembershipTestWorld(t)
			ctx := familyMembershipTestContext("s-test", tt.actor, familyMembershipActiveSessions(), nil)
			status, err := NewFamilyLeaveHandler(world, t.TempDir(), NewFamilyMembershipRequests())(ctx, enginecmd.ResolvedCommand{})
			if err != nil {
				t.Fatalf("leave handler error = %v", err)
			}
			if status != enginecmd.StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestFamilyLeaveConfirmationUsesLegacyResponses(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		world := familyMembershipTestWorld(t)
		var pending enginecmd.PendingLineHandler
		ctx := familyMembershipTestContextWithPending("s-dave", "player:dave", familyMembershipActiveSessions(), nil, &pending)
		status, err := NewFamilyLeaveHandler(world, t.TempDir(), NewFamilyMembershipRequests())(ctx, enginecmd.ResolvedCommand{})
		if err != nil {
			t.Fatalf("leave handler error = %v", err)
		}
		if status != enginecmd.StatusDoPrompt || pending == nil {
			t.Fatalf("leave status/pending = %d/%v, want prompt", status, pending != nil)
		}
		if want := "당신은 지금 현재의 패거리를 탈퇴하실 생각입니까? (예/아니오) "; ctx.OutputString() != want {
			t.Fatalf("leave prompt = %q, want %q", ctx.OutputString(), want)
		}
		ctx.Output = nil
		status, err = pending(ctx, "아니오")
		if err != nil {
			t.Fatalf("confirm error = %v", err)
		}
		if want := "패거리를 탈퇴하지 않았습니다."; status != enginecmd.StatusDefault || ctx.OutputString() != want {
			t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
		}
		dave := familyMembershipCreature(t, world, "creature:dave")
		if dave.Stats["familyFlag"] == 0 || dave.Stats["familyID"] != 2 {
			t.Fatalf("cancelled leave mutated Dave stats = %+v", dave.Stats)
		}
	})

	t.Run("insufficient funds", func(t *testing.T) {
		world := familyMembershipTestWorld(t)
		family, found := familyMemberFamily(world.Families(), 2)
		if !found {
			t.Fatal("family 2 not found")
		}
		family.JoinSubsidy = 5
		if err := world.UpdateFamily(family); err != nil {
			t.Fatal(err)
		}
		var pending enginecmd.PendingLineHandler
		ctx := familyMembershipTestContextWithPending("s-dave", "player:dave", familyMembershipActiveSessions(), nil, &pending)
		status, err := NewFamilyLeaveHandler(world, t.TempDir(), NewFamilyMembershipRequests())(ctx, enginecmd.ResolvedCommand{})
		if err != nil {
			t.Fatalf("leave handler error = %v", err)
		}
		if status != enginecmd.StatusDoPrompt || pending == nil {
			t.Fatalf("leave status/pending = %d/%v, want prompt", status, pending != nil)
		}
		ctx.Output = nil
		status, err = pending(ctx, "예")
		if err != nil {
			t.Fatalf("confirm error = %v", err)
		}
		want := "당신이 가진 돈으로는 패거리탈퇴비를 낼수 없습니다.\n패거리를 탈퇴하지 않았습니다."
		if status != enginecmd.StatusDefault || ctx.OutputString() != want {
			t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
		}
		dave := familyMembershipCreature(t, world, "creature:dave")
		if dave.Stats["familyFlag"] == 0 || dave.Stats["familyID"] != 2 {
			t.Fatalf("failed leave mutated Dave stats = %+v", dave.Stats)
		}
	})
}

func TestFamilyKickClearsTargetMembership(t *testing.T) {
	baseWorld := familyMembershipTestWorld(t)
	world := &dirtyTrackingFamilyMembershipWorld{World: baseWorld}
	root := t.TempDir()
	active := familyMembershipActiveSessions()
	writes := map[session.ID]string{}

	kickCtx := familyMembershipTestContext("s-alice", "player:alice", active, writes)
	if _, err := NewFamilyKickHandler(world, root)(kickCtx, enginecmd.ResolvedCommand{Args: []string{"Dave"}}); err != nil {
		t.Fatal(err)
	}
	if out := kickCtx.OutputString(); !strings.Contains(out, "추방") {
		t.Fatalf("kick output = %q", out)
	}
	dave := familyMembershipCreature(t, baseWorld, "creature:dave")
	if dave.Stats["familyFlag"] != 0 || dave.Stats["familyID"] != 0 || dave.Stats["PFMBOS"] != 0 {
		t.Fatalf("kicked dave stats = %+v", dave.Stats)
	}
	if !strings.Contains(writes["s-dave"], "추방") {
		t.Fatalf("kick notification = %q", writes["s-dave"])
	}
	if !world.markedDirty("player:dave") {
		t.Fatalf("online kicked player was not marked dirty: %+v", world.dirtyPlayers)
	}
}

func TestFamilyKickLegacyFailureOutput(t *testing.T) {
	tests := []struct {
		name  string
		actor model.PlayerID
		args  []string
		want  string
	}{
		{name: "non-boss", actor: "player:dave", args: []string{"Carol"}, want: "패거리의 두목만이 사용가능한 명령입니다.\n"},
		{name: "missing target", actor: "player:alice", want: "누구를 패거리에서 쫓아내시려고요?\n"},
		{name: "missing user", actor: "player:alice", args: []string{"Nobody"}, want: "그런 사용자는 없습니다.\n"},
		{name: "self", actor: "player:alice", args: []string{"Alice"}, want: "자기 자신을 추방하려고요?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := familyMembershipTestWorld(t)
			ctx := familyMembershipTestContext("s-test", tt.actor, familyMembershipActiveSessions(), nil)
			status, err := NewFamilyKickHandler(world, t.TempDir())(ctx, enginecmd.ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("kick handler error = %v", err)
			}
			if status != enginecmd.StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestFamilyMembershipRejectsUnsafeTransitions(t *testing.T) {
	world := familyMembershipTestWorld(t)
	root := t.TempDir()
	requests := NewFamilyMembershipRequests()
	active := familyMembershipActiveSessions()
	writes := map[session.ID]string{}

	alreadyCtx := familyMembershipTestContext("s-dave", "player:dave", active, writes)
	if _, err := NewFamilyJoinHandler(world, requests)(alreadyCtx, enginecmd.ResolvedCommand{Args: []string{"무영문"}}); err != nil {
		t.Fatal(err)
	}
	if out := alreadyCtx.OutputString(); !strings.Contains(out, "이미") {
		t.Fatalf("already member output = %q", out)
	}

	nonBossApproveCtx := familyMembershipTestContext("s-dave", "player:dave", active, writes)
	if _, err := NewFamilyJoinApproveHandler(world, root, requests)(nonBossApproveCtx, enginecmd.ResolvedCommand{Args: []string{"Bob"}}); err != nil {
		t.Fatal(err)
	}
	if out := nonBossApproveCtx.OutputString(); !strings.Contains(out, "문주") {
		t.Fatalf("non-boss approve output = %q", out)
	}

	nonBossCancelCtx := familyMembershipTestContext("s-dave", "player:dave", active, writes)
	if _, err := NewFamilyJoinCancelHandler(world, requests)(nonBossCancelCtx, enginecmd.ResolvedCommand{Args: []string{"Bob"}}); err != nil {
		t.Fatal(err)
	}
	if out := nonBossCancelCtx.OutputString(); !strings.Contains(out, "문주") {
		t.Fatalf("non-boss cancel output = %q", out)
	}

	nonBossKickCtx := familyMembershipTestContext("s-dave", "player:dave", active, writes)
	if _, err := NewFamilyKickHandler(world, root)(nonBossKickCtx, enginecmd.ResolvedCommand{Args: []string{"Carol"}}); err != nil {
		t.Fatal(err)
	}
	if out := nonBossKickCtx.OutputString(); !strings.Contains(out, "두목") {
		t.Fatalf("non-boss kick output = %q", out)
	}

	notMemberLeaveCtx := familyMembershipTestContext("s-erin", "player:erin", active, writes)
	if _, err := NewFamilyLeaveHandler(world, root, requests)(notMemberLeaveCtx, enginecmd.ResolvedCommand{}); err != nil {
		t.Fatal(err)
	}
	if out := notMemberLeaveCtx.OutputString(); !strings.Contains(out, "가입이 되어 있지") {
		t.Fatalf("not member leave output = %q", out)
	}

	bossLeaveCtx := familyMembershipTestContext("s-alice", "player:alice", active, writes)
	if _, err := NewFamilyLeaveHandler(world, root, requests)(bossLeaveCtx, enginecmd.ResolvedCommand{}); err != nil {
		t.Fatal(err)
	}
	if out := bossLeaveCtx.OutputString(); !strings.Contains(out, "두목") {
		t.Fatalf("boss leave output = %q", out)
	}

	kickOtherFamilyCtx := familyMembershipTestContext("s-alice", "player:alice", active, writes)
	if _, err := NewFamilyKickHandler(world, root)(kickOtherFamilyCtx, enginecmd.ResolvedCommand{Args: []string{"Carol"}}); err != nil {
		t.Fatal(err)
	}
	if out := kickOtherFamilyCtx.OutputString(); !strings.Contains(out, "패거리원이 아닙니다") {
		t.Fatalf("other family kick output = %q", out)
	}
}

func familyMembershipTestWorld(t *testing.T) *state.World {
	t.Helper()
	loaded := worldload.NewWorld()
	for _, family := range []model.Family{
		{ID: 2, Slot: 2, DisplayName: "무영문", BossName: "Alice", Members: []model.FamilyMember{{Class: 10, DisplayName: "Alice"}, {Class: 4, DisplayName: "Dave"}}},
		{ID: 5, Slot: 5, DisplayName: "은형문", BossName: "Carol", Members: []model.FamilyMember{{Class: 10, DisplayName: "Carol"}}},
	} {
		if err := loaded.AddFamily(family); err != nil {
			t.Fatalf("AddFamily(%q): %v", family.DisplayName, err)
		}
	}
	for _, player := range []model.Player{
		{ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:family"},
		{ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob", RoomID: "room:family"},
		{ID: "player:carol", DisplayName: "Carol", CreatureID: "creature:carol", RoomID: "room:family"},
		{ID: "player:dave", DisplayName: "Dave", CreatureID: "creature:dave", RoomID: "room:family"},
		{ID: "player:erin", DisplayName: "Erin", CreatureID: "creature:erin", RoomID: "room:family"},
	} {
		if err := loaded.AddPlayer(player); err != nil {
			t.Fatalf("AddPlayer(%q): %v", player.DisplayName, err)
		}
	}
	for _, creature := range []model.Creature{
		{ID: "creature:alice", Kind: model.CreatureKindPlayer, DisplayName: "Alice", PlayerID: "player:alice", RoomID: "room:family", Stats: map[string]int{"class": 10, "familyFlag": 1, "familyID": 2, "PFMBOS": 1}, Metadata: model.Metadata{Tags: []string{"PFAMIL", "PFMBOS"}}},
		{ID: "creature:bob", Kind: model.CreatureKindPlayer, DisplayName: "Bob", PlayerID: "player:bob", RoomID: "room:family", Stats: map[string]int{"class": 4}},
		{ID: "creature:carol", Kind: model.CreatureKindPlayer, DisplayName: "Carol", PlayerID: "player:carol", RoomID: "room:family", Stats: map[string]int{"class": 10, "familyFlag": 1, "familyID": 5, "PFMBOS": 1}, Metadata: model.Metadata{Tags: []string{"PFAMIL", "PFMBOS"}}},
		{ID: "creature:dave", Kind: model.CreatureKindPlayer, DisplayName: "Dave", PlayerID: "player:dave", RoomID: "room:family", Stats: map[string]int{"class": 4, "familyFlag": 1, "familyID": 2}, Metadata: model.Metadata{Tags: []string{"PFAMIL"}}},
		{ID: "creature:erin", Kind: model.CreatureKindPlayer, DisplayName: "Erin", PlayerID: "player:erin", RoomID: "room:family", Stats: map[string]int{"class": 4}},
	} {
		if err := loaded.AddCreature(creature); err != nil {
			t.Fatalf("AddCreature(%q): %v", creature.DisplayName, err)
		}
	}
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          "proto:bank_root",
		DisplayName: "금고",
	}); err != nil {
		t.Fatalf("AddObjectPrototype: %v", err)
	}
	bankObj := model.ObjectInstance{
		ID:          "object:bank_root",
		PrototypeID: "proto:bank_root",
		Location:    model.ObjectLocation{BankID: "bank:family:무영문_0", Slot: "bank"},
		Properties:  map[string]string{"value": "10"},
	}
	t.Logf("bankObj BankID=%q, Slot=%q, Location=%+v", bankObj.Location.BankID, bankObj.Location.Slot, bankObj.Location)
	if err := loaded.AddObjectInstance(bankObj); err != nil {
		t.Fatalf("AddObjectInstance: %v", err)
	}
	bankAccount := model.BankAccount{
		ID:        "bank:family:무영문_0",
		Kind:      "family",
		OwnerName: "무영문_0",
		Objects:   model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:bank_root"}},
	}
	if err := loaded.AddBank(bankAccount); err != nil {
		t.Fatalf("AddBank: %v", err)
	}
	return state.NewWorld(loaded)
}

func familyMembershipActiveSessions() []ActiveSession {
	return []ActiveSession{
		{ID: "s-alice", ActorID: "player:alice"},
		{ID: "s-bob", ActorID: "player:bob"},
		{ID: "s-carol", ActorID: "player:carol"},
		{ID: "s-dave", ActorID: "player:dave"},
		{ID: "s-erin", ActorID: "player:erin"},
	}
}

func familyMembershipActiveSessionsWithout(actorID string) []ActiveSession {
	var filtered []ActiveSession
	for _, active := range familyMembershipActiveSessions() {
		if active.ActorID != actorID {
			filtered = append(filtered, active)
		}
	}
	return filtered
}

func familyMembershipTestContext(sessionID session.ID, actorID model.PlayerID, active []ActiveSession, writes map[session.ID]string) *enginecmd.Context {
	if writes == nil {
		writes = map[session.ID]string{}
	}
	return &enginecmd.Context{
		SessionID: string(sessionID),
		ActorID:   string(actorID),
		Values: map[string]any{
			ContextActiveSessionsKey: func() []ActiveSession {
				return active
			},
			ContextSendToSessionKey: func(id session.ID, cmd session.Command) error {
				writes[id] += cmd.Write
				return nil
			},
		},
	}
}

func familyMembershipTestContextWithPending(sessionID session.ID, actorID model.PlayerID, active []ActiveSession, writes map[session.ID]string, pending *enginecmd.PendingLineHandler) *enginecmd.Context {
	ctx := familyMembershipTestContext(sessionID, actorID, active, writes)
	ctx.Values[enginecmd.ContextPendingLineKey] = func(handler enginecmd.PendingLineHandler) {
		*pending = handler
	}
	return ctx
}

func familyMembershipSubmitJoinRequest(t *testing.T, world FamilyMembershipWorld, requests *FamilyMembershipRequests, sessionID session.ID, actorID model.PlayerID, active []ActiveSession, writes map[session.ID]string, target string) *enginecmd.Context {
	t.Helper()
	var pending enginecmd.PendingLineHandler
	ctx := familyMembershipTestContextWithPending(sessionID, actorID, active, writes, &pending)
	status, err := NewFamilyJoinHandler(world, requests)(ctx, enginecmd.ResolvedCommand{Args: []string{target}})
	if err != nil {
		t.Fatalf("join handler error = %v", err)
	}
	if status != enginecmd.StatusDoPrompt || pending == nil {
		t.Fatalf("join status/pending = %d/%v, want prompt", status, pending != nil)
	}
	if !strings.Contains(ctx.OutputString(), "가입을 하시겠습니까? (예/아니오)") {
		t.Fatalf("join prompt output = %q", ctx.OutputString())
	}
	ctx.Output = nil
	status, err = pending(ctx, "예")
	if err != nil {
		t.Fatalf("join confirm error = %v", err)
	}
	if status != enginecmd.StatusDefault {
		t.Fatalf("join confirm status = %d, want default", status)
	}
	if !strings.Contains(ctx.OutputString(), "가입 신청을 하였습니다.") {
		t.Fatalf("join confirm output = %q", ctx.OutputString())
	}
	return ctx
}

func familyMembershipCreature(t *testing.T, world *state.World, creatureID model.CreatureID) model.Creature {
	t.Helper()
	creature, ok := world.Creature(creatureID)
	if !ok {
		t.Fatalf("Creature(%q) missing", creatureID)
	}
	return creature
}

func familyMembershipTestHasTag(creature model.Creature, tag string) bool {
	for _, existing := range creature.Metadata.Tags {
		if existing == tag {
			return true
		}
	}
	return false
}

type dirtyTrackingFamilyMembershipWorld struct {
	*state.World
	dirtyPlayers  []model.PlayerID
	queuedPlayers []model.PlayerID
}

func (w *dirtyTrackingFamilyMembershipWorld) MarkPlayerDirty(playerID model.PlayerID) {
	w.dirtyPlayers = append(w.dirtyPlayers, playerID)
	w.World.MarkPlayerDirty(playerID)
}

func (w *dirtyTrackingFamilyMembershipWorld) QueueSave(playerID model.PlayerID, _ model.BankID) {
	w.queuedPlayers = append(w.queuedPlayers, playerID)
}

func (w *dirtyTrackingFamilyMembershipWorld) markedDirty(playerID model.PlayerID) bool {
	for _, dirtyPlayer := range w.dirtyPlayers {
		if dirtyPlayer == playerID {
			return true
		}
	}
	return false
}

func (w *dirtyTrackingFamilyMembershipWorld) queuedSave(playerID model.PlayerID) bool {
	for _, queuedPlayer := range w.queuedPlayers {
		if queuedPlayer == playerID {
			return true
		}
	}
	return false
}

func writeFamilyListFile(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, "player", "family", "family_list")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	data := "" +
		"0 관리파 지존마상 100\n" +
		"1 은형문 셀미 100\n" +
		"2 무영문 Alice 100\n" +
		"16 패거리데이타\n"
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}

func createTestOfflinePlayerFile(t *testing.T, path string, familyID int, hasFamilyFlag bool) {
	t.Helper()
	data := make([]byte, 1184)
	if hasFamilyFlag {
		data[412+6] |= (1 << 7)
	}
	data[612] = byte(familyID)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func verifyTestOfflinePlayerFile(t *testing.T, path string, expectedFamilyID int, expectedFamilyFlag bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 1184 {
		t.Fatalf("file too small: %d", len(data))
	}
	familyID := int(data[612])
	hasFamilyFlag := (data[412+6] & (1 << 7)) != 0
	if familyID != expectedFamilyID {
		t.Errorf("expected familyID %d, got %d", expectedFamilyID, familyID)
	}
	if hasFamilyFlag != expectedFamilyFlag {
		t.Errorf("expected familyFlag %v, got %v", expectedFamilyFlag, hasFamilyFlag)
	}
}

func createTestBankFile(t *testing.T, path string, balance int) {
	t.Helper()
	data := make([]byte, 304)
	binary.LittleEndian.PutUint32(data[300:304], uint32(balance))
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func verifyTestBankFile(t *testing.T, path string, expectedBalance int) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 304 {
		t.Fatalf("bank file too small: %d", len(data))
	}
	balance := int(binary.LittleEndian.Uint32(data[300:304]))
	if balance != expectedBalance {
		t.Errorf("expected bank balance %d, got %d", expectedBalance, balance)
	}
}

func verifyTestFamilyMemberFile(t *testing.T, path string, expected []legacyMember, expectedFamilyName string) {
	t.Helper()
	members, familyName, err := readFamilyMembersFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if familyName != expectedFamilyName {
		t.Errorf("expected family name %q, got %q", expectedFamilyName, familyName)
	}
	if len(members) != len(expected) {
		t.Fatalf("expected %d members, got %d", len(expected), len(members))
	}
	for i, m := range members {
		if m.classID != expected[i].classID || m.name != expected[i].name {
			t.Errorf("member %d: expected %+v, got %+v", i, expected[i], m)
		}
	}
}

func TestFamilyJoinApproveDeductsJoinSubsidyFromInMemoryBank(t *testing.T) {
	world := familyMembershipTestWorld(t)
	root := t.TempDir()
	requests := NewFamilyMembershipRequests()
	active := familyMembershipActiveSessions()
	writes := map[session.ID]string{}

	// Update family 2 (무영문) to have JoinSubsidy = 5 (50,000 gold)
	var fam model.Family
	foundFam := false
	for _, f := range world.Families() {
		if f.ID == 2 {
			fam = f
			foundFam = true
			break
		}
	}
	if !foundFam {
		t.Fatal("family 2 not found")
	}
	fam.JoinSubsidy = 5
	if err := world.UpdateFamily(fam); err != nil {
		t.Fatal(err)
	}

	// Register Bob's join request
	familyMembershipSubmitJoinRequest(t, world, requests, "s-bob", "player:bob", active, writes, "무영문")

	// Initial family bank value = 10만냥 (from familyMembershipTestWorld)
	// Bob initial gold = 0
	bob := familyMembershipCreature(t, world, "creature:bob")
	if bob.Stats["gold"] != 0 {
		t.Fatalf("expected initial bob gold to be 0, got %d", bob.Stats["gold"])
	}

	approveCtx := familyMembershipTestContext("s-alice", "player:alice", active, writes)
	if _, err := NewFamilyJoinApproveHandler(world, root, requests)(approveCtx, enginecmd.ResolvedCommand{Args: []string{"Bob"}}); err != nil {
		t.Fatal(err)
	}

	// Check Bob got 50,000 gold
	bob = familyMembershipCreature(t, world, "creature:bob")
	if bob.Stats["gold"] != 50000 {
		t.Errorf("expected Bob to have 50000 gold, got %d", bob.Stats["gold"])
	}

	// Check bank has 5만냥 left
	bankObj, ok := world.Object("object:bank_root")
	if !ok {
		t.Fatal("bank object missing")
	}
	val, _ := strconv.Atoi(bankObj.Properties["value"])
	if val != 5 {
		t.Errorf("expected bank value 5, got %d", val)
	}

	// Check family member file is updated
	memberPath := filepath.Join(root, "player", "family", "family_member_2")
	verifyTestFamilyMemberFile(t, memberPath, []legacyMember{
		{classID: 4, name: "Bob"},
	}, "무영문")
}

func TestFamilyJoinApproveDeductsJoinSubsidyFromDiskBank(t *testing.T) {
	world := familyMembershipTestWorld(t)
	root := t.TempDir()
	requests := NewFamilyMembershipRequests()
	active := familyMembershipActiveSessions()
	writes := map[session.ID]string{}

	// Update family 2 (무영문) to have JoinSubsidy = 5 (50,000 gold)
	var fam model.Family
	foundFam := false
	for _, f := range world.Families() {
		if f.ID == 2 {
			fam = f
			foundFam = true
			break
		}
	}
	if !foundFam {
		t.Fatal("family 2 not found")
	}
	fam.JoinSubsidy = 5
	if err := world.UpdateFamily(fam); err != nil {
		t.Fatal(err)
	}

	// We remove the in-memory bank account so it falls back to disk
	loaded := worldload.NewWorld()
	for _, family := range []model.Family{
		{ID: 2, Slot: 2, DisplayName: "무영문", BossName: "Alice", JoinSubsidy: 5, Members: []model.FamilyMember{{Class: 10, DisplayName: "Alice"}}},
	} {
		_ = loaded.AddFamily(family)
	}
	_ = loaded.AddPlayer(model.Player{ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:family"})
	_ = loaded.AddPlayer(model.Player{ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob", RoomID: "room:family"})
	_ = loaded.AddCreature(model.Creature{ID: "creature:alice", Kind: model.CreatureKindPlayer, DisplayName: "Alice", PlayerID: "player:alice", RoomID: "room:family", Stats: map[string]int{"class": 10, "familyFlag": 1, "familyID": 2, "PFMBOS": 1}, Metadata: model.Metadata{Tags: []string{"PFAMIL", "PFMBOS"}}})
	_ = loaded.AddCreature(model.Creature{ID: "creature:bob", Kind: model.CreatureKindPlayer, DisplayName: "Bob", PlayerID: "player:bob", RoomID: "room:family", Stats: map[string]int{"class": 4}})
	worldNoBank := state.NewWorld(loaded)

	// Create disk bank file with 10만냥 in legacy family-bank units.
	bankPath := filepath.Join(root, "player", "family", "bank", "무영문_0")
	createTestBankFile(t, bankPath, 10)

	// Register Bob's join request
	familyMembershipSubmitJoinRequest(t, worldNoBank, requests, "s-bob", "player:bob", active, writes, "무영문")

	approveCtx := familyMembershipTestContext("s-alice", "player:alice", active, writes)
	if _, err := NewFamilyJoinApproveHandler(worldNoBank, root, requests)(approveCtx, enginecmd.ResolvedCommand{Args: []string{"Bob"}}); err != nil {
		t.Fatal(err)
	}

	// Verify Bob's gold in-memory
	bob := familyMembershipCreature(t, worldNoBank, "creature:bob")
	if bob.Stats["gold"] != 50000 {
		t.Errorf("expected Bob to have 50000 gold, got %d", bob.Stats["gold"])
	}

	// Verify disk bank balance
	verifyTestBankFile(t, bankPath, 5)
}

func TestFamilyLeaveChargesExitFee(t *testing.T) {
	world := familyMembershipTestWorld(t)
	root := t.TempDir()
	requests := NewFamilyMembershipRequests()
	active := familyMembershipActiveSessions()
	writes := map[session.ID]string{}
	var broadcasts []string

	// Update family 2 to have JoinSubsidy = 5 (fee = 5 * 20,000 = 100,000)
	var fam model.Family
	foundFam := false
	for _, f := range world.Families() {
		if f.ID == 2 {
			fam = f
			foundFam = true
			break
		}
	}
	if !foundFam {
		t.Fatal("family 2 not found")
	}
	fam.JoinSubsidy = 5
	if err := world.UpdateFamily(fam); err != nil {
		t.Fatal(err)
	}

	// Setup Dave's gold = 120,000
	daveCreature := familyMembershipCreature(t, world, "creature:dave")
	daveCreature.Stats["gold"] = 120000
	if _, err := world.UpdateCreatureGold("creature:dave", 120000); err != nil {
		t.Fatal(err)
	}

	// Pre-create family member file
	memberPath := filepath.Join(root, "player", "family", "family_member_2")
	if err := writeFamilyMembersFile(memberPath, []legacyMember{{classID: 4, name: "Dave"}}, "무영문"); err != nil {
		t.Fatal(err)
	}

	var leavePending enginecmd.PendingLineHandler
	leaveCtx := familyMembershipTestContextWithPending("s-dave", "player:dave", active, writes, &leavePending)
	leaveCtx.Values[ContextBroadcastKey] = func(cmd session.Command) error {
		broadcasts = append(broadcasts, cmd.Write)
		return nil
	}
	status, err := NewFamilyLeaveHandler(world, root, requests)(leaveCtx, enginecmd.ResolvedCommand{})
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDoPrompt || leavePending == nil {
		t.Fatalf("leave status/pending = %d/%v, want prompt", status, leavePending != nil)
	}
	status, err = leavePending(leaveCtx, "예")
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDefault {
		t.Fatalf("leave confirm status = %d", status)
	}
	wantOut := "당신은 지금 현재의 패거리를 탈퇴하실 생각입니까? (예/아니오) " +
		"당신은 패거리에서 탈퇴를 하였습니다.\n" +
		"\n당신은 이제 20000냥을 갖고 있습니다."
	if leaveCtx.OutputString() != wantOut {
		t.Fatalf("leave output = %q, want %q", leaveCtx.OutputString(), wantOut)
	}
	if len(broadcasts) != 1 || broadcasts[0] != "\n### Dave님이 무영문에서 탈퇴를 하였습니다." {
		t.Fatalf("leave broadcasts = %+v", broadcasts)
	}

	// Dave should have 20,000 gold left (120,000 - 100,000)
	daveCreature = familyMembershipCreature(t, world, "creature:dave")
	if daveCreature.Stats["gold"] != 20000 {
		t.Errorf("expected Dave to have 20000 gold, got %d", daveCreature.Stats["gold"])
	}

	// Dave should be removed from the family member file
	verifyTestFamilyMemberFile(t, memberPath, []legacyMember{}, "무영문")
}

func TestFamilyKickOfflinePlayer(t *testing.T) {
	world := familyMembershipTestWorld(t)
	root := t.TempDir()
	writes := map[session.ID]string{}

	// Setup offline player Dave (not in active sessions)
	active := []ActiveSession{
		{ID: "s-alice", ActorID: "player:alice"},
	}

	// Configure Dave player with a LegacyPath
	player, ok := world.Player("player:dave")
	if !ok {
		t.Fatal("player:dave not found")
	}
	player.Metadata.LegacyPath = "player/D/Dave"
	if err := world.UpdatePlayer(player); err != nil {
		t.Fatal(err)
	}

	// Create test offline player file
	offlinePath := filepath.Join(root, "player", "D", "Dave")
	createTestOfflinePlayerFile(t, offlinePath, 2, true)

	// Pre-create family member file
	memberPath := filepath.Join(root, "player", "family", "family_member_2")
	if err := writeFamilyMembersFile(memberPath, []legacyMember{{classID: 4, name: "Dave"}}, "무영문"); err != nil {
		t.Fatal(err)
	}

	kickCtx := familyMembershipTestContext("s-alice", "player:alice", active, writes)
	if _, err := NewFamilyKickHandler(world, root)(kickCtx, enginecmd.ResolvedCommand{Args: []string{"Dave"}}); err != nil {
		t.Fatal(err)
	}

	// Verify Dave's offline file has familyID = 0, PFAMIL flag cleared
	verifyTestOfflinePlayerFile(t, offlinePath, 0, false)

	// Dave should be removed from the family member file
	verifyTestFamilyMemberFile(t, memberPath, []legacyMember{}, "무영문")
}

func TestFamilyKickOfflinePlayerRestartVisibleInFamilyMemberList(t *testing.T) {
	world := familyMembershipTestWorld(t)
	root := t.TempDir()
	active := []ActiveSession{{ID: "s-alice", ActorID: "player:alice"}}
	writes := map[session.ID]string{}

	player, ok := world.Player("player:dave")
	if !ok {
		t.Fatal("player:dave not found")
	}
	player.Metadata.LegacyPath = "player/D/Dave"
	if err := world.UpdatePlayer(player); err != nil {
		t.Fatal(err)
	}

	offlinePath := filepath.Join(root, "player", "D", "Dave")
	createTestOfflinePlayerFile(t, offlinePath, 2, true)
	writeFamilyListFile(t, root)
	memberPath := filepath.Join(root, "player", "family", "family_member_2")
	if err := writeFamilyMembersFile(memberPath, []legacyMember{
		{classID: 10, name: "Alice"},
		{classID: 4, name: "Dave"},
	}, "무영문"); err != nil {
		t.Fatal(err)
	}

	kickCtx := familyMembershipTestContext("s-alice", "player:alice", active, writes)
	if _, err := NewFamilyKickHandler(world, root)(kickCtx, enginecmd.ResolvedCommand{Args: []string{"Dave"}}); err != nil {
		t.Fatal(err)
	}

	verifyTestOfflinePlayerFile(t, offlinePath, 0, false)
	verifyTestFamilyMemberFile(t, memberPath, []legacyMember{{classID: 10, name: "Alice"}}, "무영문")
	family, ok := world.Family(2)
	if !ok {
		t.Fatal("family 2 not found")
	}
	if len(family.Members) != 1 || family.Members[0].DisplayName != "Alice" {
		t.Fatalf("runtime family members after kick = %+v", family.Members)
	}

	summary, err := worldload.LoadRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	restartedFamily := summary.World.Families[2]
	if len(restartedFamily.Members) != 1 || restartedFamily.Members[0].DisplayName != "Alice" {
		t.Fatalf("restarted family members = %+v", restartedFamily.Members)
	}
	if err := summary.World.AddPlayer(model.Player{ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice"}); err != nil {
		t.Fatal(err)
	}
	if err := summary.World.AddCreature(model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		Stats:       map[string]int{"class": 10, "familyFlag": 1, "familyID": 2},
		Metadata:    model.Metadata{Tags: []string{"PFAMIL"}},
	}); err != nil {
		t.Fatal(err)
	}
	ctx := &enginecmd.Context{ActorID: "player:alice"}
	if _, err := NewFamilyMemberHandler(state.NewWorld(summary.World))(ctx, enginecmd.ResolvedCommand{}); err != nil {
		t.Fatal(err)
	}
	out := ctx.OutputString()
	if !strings.Contains(out, "Alice") || strings.Contains(out, "Dave") || !strings.Contains(out, "총 1명의 사람들이 가입되어 있습니다.") {
		t.Fatalf("family_member after restart =\n%s", out)
	}
}

func TestFamilyLeaveWithoutMemberFileStillUpdatesRuntimeMembers(t *testing.T) {
	world := familyMembershipTestWorld(t)
	root := t.TempDir()
	requests := NewFamilyMembershipRequests()
	active := familyMembershipActiveSessions()
	writes := map[session.ID]string{}
	memberPath := filepath.Join(root, "player", "family", "family_member_2")

	var leavePending enginecmd.PendingLineHandler
	leaveCtx := familyMembershipTestContextWithPending("s-dave", "player:dave", active, writes, &leavePending)
	status, err := NewFamilyLeaveHandler(world, root, requests)(leaveCtx, enginecmd.ResolvedCommand{})
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDoPrompt || leavePending == nil {
		t.Fatalf("leave status/pending = %d/%v, want prompt", status, leavePending != nil)
	}
	status, err = leavePending(leaveCtx, "예")
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDefault {
		t.Fatalf("leave confirm status = %d", status)
	}
	if _, err := os.Stat(memberPath); !os.IsNotExist(err) {
		t.Fatalf("leave without family_member file should not create one, stat err=%v", err)
	}

	family, ok := world.Family(2)
	if !ok {
		t.Fatal("family 2 not found")
	}
	for _, member := range family.Members {
		if member.DisplayName == "Dave" {
			t.Fatalf("Dave remained in runtime family members: %+v", family.Members)
		}
	}
}

func TestChangeClassPersistsFamilyMemberFileAndRuntimeMembers(t *testing.T) {
	root := t.TempDir()
	memberPath := filepath.Join(root, "player", "family", "family_member_2")
	if err := writeFamilyMembersFile(memberPath, []legacyMember{
		{classID: 4, name: "Alice"},
		{classID: 6, name: "Bob"},
	}, "무영문"); err != nil {
		t.Fatal(err)
	}

	loaded := worldload.NewWorld()
	if err := loaded.AddRoom(model.Room{
		ID:          "room:00610",
		DisplayName: "수련장",
		Metadata:    model.Metadata{Tags: []string{"train", "trainingBit4"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddFamily(model.Family{
		ID:          2,
		Slot:        2,
		DisplayName: "무영문",
		Members: []model.FamilyMember{
			{Class: 4, DisplayName: "Alice"},
			{Class: 6, DisplayName: "Bob"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddPlayer(model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:00610",
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddCreature(model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:00610",
		Level:       20,
		Stats: map[string]int{
			"class":         4,
			"level":         20,
			"experience":    150000,
			"familyFlag":    1,
			"familyID":      2,
			"dailyExpndMax": 2,
			"hpCurrent":     100,
			"hpMax":         100,
			"mpCurrent":     50,
			"mpMax":         50,
		},
		Metadata: model.Metadata{Tags: []string{"PFAMIL"}},
	}); err != nil {
		t.Fatal(err)
	}

	world := state.NewWorld(loaded)
	world.SetDBRoot(root)
	var pending enginecmd.PendingLineHandler
	ctx := &enginecmd.Context{
		ActorID: "player:alice",
		Values: map[string]any{
			enginecmd.ContextPendingLineKey: func(handler enginecmd.PendingLineHandler) {
				pending = handler
			},
		},
	}
	status, err := enginecmd.NewChangeClassHandler(world)(ctx, enginecmd.ResolvedCommand{})
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDoPrompt || pending == nil {
		t.Fatalf("initial status/pending = %d/%v", status, pending != nil)
	}

	ctx.Output = nil
	status, err = pending(ctx, "예")
	if err != nil {
		t.Fatal(err)
	}
	if status != enginecmd.StatusDefault {
		t.Fatalf("confirm status = %d", status)
	}

	verifyTestFamilyMemberFile(t, memberPath, []legacyMember{
		{classID: 5, name: "Alice"},
		{classID: 6, name: "Bob"},
	}, "무영문")
	family, ok := world.Family(2)
	if !ok {
		t.Fatal("family 2 not found")
	}
	if len(family.Members) != 2 || family.Members[0].DisplayName != "Alice" || family.Members[0].Class != 5 || family.Members[1].Class != 6 {
		t.Fatalf("runtime family members = %+v", family.Members)
	}
}
