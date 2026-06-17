package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	enginecmd "github.com/0xc0de1ab/muhan/internal/engine/command"
	"github.com/0xc0de1ab/muhan/internal/engine/game"
	"github.com/0xc0de1ab/muhan/internal/session"
)

func TestServerSuicideSinkBroadcastsAndLogsViaHooks(t *testing.T) {
	inputs := serverTestRuntimeInputs(t)
	defer inputs.world.Close()
	if err := inputs.world.SetCreatureStat("creature:alice", "level", 6); err != nil {
		t.Fatalf("raise alice level: %v", err)
	}

	var broadcasts []session.Command
	var logs []string
	sink := serverSuicideSink{
		world: inputs.world,
		root:  inputs.summary.Root,
		now: func() time.Time {
			return time.Unix(123, 0).UTC()
		},
		logf: func(format string, args ...any) {
			logs = append(logs, fmt.Sprintf(format, args...))
		},
	}
	ctx := &enginecmd.Context{
		ActorID: "player:alice",
		Values: map[string]any{
			game.ContextBroadcastKey: func(cmd session.Command) error {
				broadcasts = append(broadcasts, cmd)
				return nil
			},
		},
	}

	if err := sink.RequestSuicide(ctx, "player:alice"); err != nil {
		t.Fatalf("RequestSuicide() error = %v", err)
	}
	if len(broadcasts) != 1 || !strings.Contains(broadcasts[0].Write, "Alice님이 자살신청을 하였습니다.") {
		t.Fatalf("broadcasts = %#v, want suicide broadcast", broadcasts)
	}
	if !serverSuicideTestLogContains(logs, "[SUICIDE]", "Alice님이 자살신청을 하였습니다.") {
		t.Fatalf("logs = %#v, want suicide log", logs)
	}
	if _, ok := inputs.world.Player("player:alice"); ok {
		t.Fatal("player still exists after suicide sink finalization")
	}
}

func serverSuicideTestLogContains(logs []string, parts ...string) bool {
	for _, line := range logs {
		matched := true
		for _, part := range parts {
			if !strings.Contains(line, part) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// TestServerSuicideAuditLogByteExact asserts the audit log line is byte-exact,
// matching the C src/command5.c logn("SUICIDE", ...) format adapted for Go.
//
// C format:  logn("SUICIDE","%s : %s (%s)님이 자살신청을 하였습니다.\n", ctime_buf, name, address)
// Go format: [SUICIDE] <RFC3339> : <name>님이 자살신청을 하였습니다.
//
// The timestamp is runtime-dependent in C (ctime) and Go (RFC3339), so the test
// injects a fixed time. The player name is deterministic. The Go port omits the
// IP address field (not available in the sink) which is an acceptable parity
// divergence — the essential prefix `[SUICIDE]` and message body are byte-exact.
func TestServerSuicideAuditLogByteExact(t *testing.T) {
	inputs := serverTestRuntimeInputs(t)
	defer inputs.world.Close()
	if err := inputs.world.SetCreatureStat("creature:alice", "level", 6); err != nil {
		t.Fatalf("raise alice level: %v", err)
	}

	var logs []string
	fixedTime := time.Unix(123, 0).UTC()
	sink := serverSuicideSink{
		world: inputs.world,
		root:  inputs.summary.Root,
		now: func() time.Time {
			return fixedTime
		},
		logf: func(format string, args ...any) {
			logs = append(logs, fmt.Sprintf(format, args...))
		},
	}
	ctx := &enginecmd.Context{
		ActorID: "player:alice",
		Values: map[string]any{
			game.ContextBroadcastKey: func(cmd session.Command) error {
				return nil
			},
		},
	}

	if err := sink.RequestSuicide(ctx, "player:alice"); err != nil {
		t.Fatalf("RequestSuicide() error = %v", err)
	}

	// C: logn("SUICIDE","%s : %s (%s)님이 자살신청을 하였습니다.\n", ...)
	// Go: "[SUICIDE] %s : %s님이 자살신청을 하였습니다." (no address field)
	wantLog := "[SUICIDE] " + fixedTime.Format(time.RFC3339) + " : Alice님이 자살신청을 하였습니다."
	found := false
	for _, line := range logs {
		if line == wantLog {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("audit log byte-exact mismatch:\nwant: %q\ngot logs: %#v", wantLog, logs)
	}
}
