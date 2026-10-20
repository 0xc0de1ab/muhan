package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	enginecmd "github.com/0xc0de1ab/muhan/internal/engine/command"
	"github.com/0xc0de1ab/muhan/internal/session"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

// #19: serverIsDM is the only authorization boundary between regular players
// and the dm_* handlers (serverDispatcher wraps every dm_-prefixed handler
// and every privileged '*' command with it), so pin its decision boundary
// directly instead of trusting mock policy tests alone.
func TestServerIsDM(t *testing.T) {
	inputs := serverTestRuntimeInputs(t)
	defer inputs.world.Close()
	world := inputs.world

	tests := []struct {
		name string
		ctx  *enginecmd.Context
		want bool
	}{
		{"nil context", nil, false},
		{"empty actor", &enginecmd.Context{}, false},
		{"unknown player", &enginecmd.Context{ActorID: "player:nobody"}, false},
		{"player without creature", &enginecmd.Context{ActorID: "player:offline"}, false},
		{"regular player class 8", &enginecmd.Context{ActorID: "player:alice"}, false},
		{"DM player class 13", &enginecmd.Context{ActorID: "player:dm"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serverIsDM(world, tt.ctx); got != tt.want {
				t.Fatalf("serverIsDM = %v, want %v", got, tt.want)
			}
		})
	}

	if serverIsDM(nil, &enginecmd.Context{ActorID: "player:dm"}) {
		t.Fatal("serverIsDM with nil world = true, want false")
	}
}

// serverIsDM must only fire on the exact DM class: adjacent values and the
// class-via-Properties fallback of serverCreatureInt must behave correctly.
func TestServerIsDMClassBoundary(t *testing.T) {
	inputs := serverTestRuntimeInputs(t)
	defer inputs.world.Close()
	world := inputs.world

	// Non-DM classes (below and above the DM class) stay denied.
	for _, class := range []int{0, 12, 14, 99} {
		if err := world.SetCreatureStat("creature:alice", "class", class); err != nil {
			t.Fatalf("set alice class %d: %v", class, err)
		}
		if serverIsDM(world, &enginecmd.Context{ActorID: "player:alice"}) {
			t.Fatalf("serverIsDM(alice, class=%d) = true, want false", class)
		}
	}
	if err := world.SetCreatureStat("creature:alice", "class", legacyLoginDMClass); err != nil {
		t.Fatalf("set alice DM class: %v", err)
	}
	if !serverIsDM(world, &enginecmd.Context{ActorID: "player:alice"}) {
		t.Fatal("serverIsDM(alice, class=13) = false, want true")
	}

	// A class given via Properties (the serverCreatureInt fallback) also counts.
	if _, err := world.SetCreatureProperty("creature:bob", "class", "13"); err != nil {
		t.Fatalf("set bob class property: %v", err)
	}
	if !serverIsDM(world, &enginecmd.Context{ActorID: "player:bob"}) {
		t.Fatal("serverIsDM(bob, property class=13) = false, want true")
	}
	if _, err := world.SetCreatureProperty("creature:bob", "class", "8"); err != nil {
		t.Fatalf("set bob class property: %v", err)
	}
	if serverIsDM(world, &enginecmd.Context{ActorID: "player:bob"}) {
		t.Fatal("serverIsDM(bob, property class=8) = true, want false")
	}
}

// A player whose CreatureID points at a nonexistent creature is not a DM
// even if their player record exists.
func TestServerIsDMMissingCreature(t *testing.T) {
	loaded := worldload.NewWorld()
	mustAddServerTestRoom(t, loaded, model.Room{ID: "room:plaza", DisplayName: "광장"})
	mustAddServerTestPlayer(t, loaded, model.Player{
		ID:          "player:ghost",
		DisplayName: "Ghost",
		CreatureID:  "creature:missing",
		RoomID:      "room:plaza",
	})
	world := state.NewWorld(loaded)
	defer world.Close()

	if serverIsDM(world, &enginecmd.Context{ActorID: "player:ghost"}) {
		t.Fatal("serverIsDM(ghost, missing creature) = true, want false")
	}
}

// #19 negative e2e: a non-DM player (alice, class 8) must be rejected when
// typing DM-only commands, without the dm handler ever running.
func TestServerLoopRejectsDMCommandsForNonDMPlayer(t *testing.T) {
	inputs := withServerTestCommands(t, serverTestRuntimeInputs(t),
		commandspec.CommandSpec{Name: "*log", Number: 121, Handler: "dm_log"},
		commandspec.CommandSpec{Name: "*shutdown", Number: 122, Handler: "dm_shutdown"},
		commandspec.CommandSpec{Name: "*delete", Number: 123, Handler: "dm_delete"},
	)
	defer inputs.world.Close()

	loop := newServerTestLoop(inputs)
	commands := make(chan session.Command, 8)
	registerServerTestSession(t, loop, "s-alice", commands, "player:alice")

	for _, line := range []string{"*log", "*shutdown", "*delete"} {
		handleServerTestLine(t, loop, "s-alice", line)
		assertServerCommand(t, commands, session.Command{
			Write:  "무슨 말인지 모르겠습니다.\n",
			Prompt: "> ",
		})
	}
	assertNoServerCommand(t, commands)
}

// #19 positive control: a DM-class player passes the same gate, so the
// negative test above proves the gate and not a broken command mapping.
func TestServerLoopAllowsDMCommandForDMPlayer(t *testing.T) {
	logRoot := t.TempDir()
	t.Chdir(logRoot)
	if err := os.MkdirAll("log", 0o755); err != nil {
		t.Fatal(err)
	}
	var builder strings.Builder
	for i := 1; i <= 24; i++ {
		builder.WriteString("line ")
		builder.WriteByte(byte('A' - 1 + i))
		builder.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join("log", "log"), []byte(builder.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	inputs := withServerTestCommands(t, serverTestRuntimeInputs(t),
		commandspec.CommandSpec{Name: "*log", Number: 121, Handler: "dm_log"},
	)
	defer inputs.world.Close()
	if err := inputs.world.SetCreatureStat("creature:dm", "class", legacyLoginDMClass); err != nil {
		t.Fatalf("set DM class: %v", err)
	}

	loop := newServerTestLoop(inputs)
	commands := make(chan session.Command, 4)
	registerServerTestSession(t, loop, "s-dm", commands, "player:dm")

	handleServerTestLine(t, loop, "s-dm", "*log")
	got := receiveServerCommand(t, commands)
	assertServerCommandValueContains(t, got, session.Command{},
		"line S\n",
		"[엔터]를 누르세요. 그만보시려면 [.]을 치세요: ",
	)
	if strings.Contains(got.Write, "무슨 말인지 모르겠습니다.") {
		t.Fatalf("DM player was rejected by the DM gate:\n%s", got.Write)
	}
}
