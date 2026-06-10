package command

import (
	"strings"
	"testing"

	"muhan/internal/commandspec"
	state "muhan/internal/world/state"
)

func TestPledgeAndRescindHandlerTransitions(t *testing.T) {
	loaded := lookWorld(t)
	// Enable room flag RPLDGK (pledge) on plaza, and RRSCND (rescind) on plaza
	plaza := loaded.Rooms["room:plaza"]
	plaza.Metadata.Tags = []string{"pledge", "rescind"}
	loaded.Rooms[plaza.ID] = plaza

	world := state.NewWorld(loaded)
	defer world.Close()
	pledgeHandler := NewPledgeHandler(world)
	rescindHandler := NewRescindHandler(world)

	ctx := &Context{ActorID: "player:alice"}

	// 1. Initial rescind should fail since not pledged
	status, err := rescindHandler(ctx, ResolvedCommand{Spec: commandspec.CommandSpec{Handler: "rescind"}})
	if err != nil {
		t.Fatalf("rescind() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if !strings.Contains(ctx.OutputString(), "가입되어 있지 않습니다") {
		t.Fatalf("output = %q, want not pledged error", ctx.OutputString())
	}
	ctx.Output = nil

	// 2. Pledge should succeed
	status, err = pledgeHandler(ctx, ResolvedCommand{Spec: commandspec.CommandSpec{Handler: "pledge"}})
	if err != nil {
		t.Fatalf("pledge() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if !strings.Contains(ctx.OutputString(), "가입하였습니다") {
		t.Fatalf("output = %q, want pledge success", ctx.OutputString())
	}
	ctx.Output = nil

	// 3. Player tags should contain "pledged"
	player, _ := world.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "pledged") {
		t.Fatalf("player did not get pledged tag")
	}

	// 4. Repeated pledge should fail
	status, err = pledgeHandler(ctx, ResolvedCommand{Spec: commandspec.CommandSpec{Handler: "pledge"}})
	if err != nil {
		t.Fatalf("pledge() repeated error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if !strings.Contains(ctx.OutputString(), "이미 가입되어 있습니다") {
		t.Fatalf("output = %q, want already pledged error", ctx.OutputString())
	}
	ctx.Output = nil

	// 5. Rescind should succeed
	status, err = rescindHandler(ctx, ResolvedCommand{Spec: commandspec.CommandSpec{Handler: "rescind"}})
	if err != nil {
		t.Fatalf("rescind() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if !strings.Contains(ctx.OutputString(), "탈퇴하였습니다") {
		t.Fatalf("output = %q, want rescind success", ctx.OutputString())
	}
	ctx.Output = nil

	// 6. Player tags should not contain "pledged"
	player, _ = world.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "pledged") {
		t.Fatalf("player still has pledged tag")
	}
}

func TestPledgeHandlerFailsIfNoRoomOrMonsterFlag(t *testing.T) {
	loaded := lookWorld(t)
	// No room pledge flags
	world := state.NewWorld(loaded)
	defer world.Close()
	pledgeHandler := NewPledgeHandler(world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := pledgeHandler(ctx, ResolvedCommand{Spec: commandspec.CommandSpec{Handler: "pledge"}})
	if err != nil {
		t.Fatalf("pledge() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if !strings.Contains(ctx.OutputString(), "이곳에서는 가입할 수 없습니다") {
		t.Fatalf("output = %q, want cannot pledge here error", ctx.OutputString())
	}
}
