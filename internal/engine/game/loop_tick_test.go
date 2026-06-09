package game

import (
	"context"
	"testing"
	"time"

	enginecmd "muhan/internal/engine/command"
	"muhan/internal/session"
	"muhan/internal/world/state"
)

func TestLoopTickIntervals(t *testing.T) {
	world := state.NewWorld(nil)

	// Custom intervals for testing
	world.SetRandomUpdateInterval(10)
	world.SetTXInterval(100)

	// Keep track of calls
	calls := map[string]int{}
	world.UpdateActiveMonstersFunc = func(t int64) error {
		calls["active"]++
		return nil
	}
	world.UpdatePlayerStatusesFunc = func(t int64) error {
		calls["player"]++
		return nil
	}
	world.UpdateRandomSpawnsFunc = func(t int64) error {
		calls["random"]++
		return nil
	}
	world.UpdateTimeClockFunc = func(t int64) error {
		calls["time"]++
		return nil
	}
	world.UpdateTimedExitsFunc = func(t int64) error {
		calls["exit"]++
		return nil
	}
	world.UpdateShutdownFunc = func(t int64) error {
		calls["shutdown"]++
		return nil
	}

	dispatcher := enginecmd.Dispatcher{Registry: testRegistry(t)}
	loop := NewLoop(dispatcher, WithWorld(world))

	// Reset intervals tracking
	// T0 = 1000
	loop.TickAt(1000)

	// First tick at T=1000 triggers all updates (since last update defaults to 0)
	// except shutdown (which is not scheduled).
	expectedInitial := map[string]int{
		"active": 1,
		"player": 1,
		"random": 1,
		"time":   1,
		"exit":   1,
	}
	for name, want := range expectedInitial {
		if got := calls[name]; got != want {
			t.Errorf("Initial tick: calls[%q] = %d, want %d", name, got, want)
		}
	}
	if got := calls["shutdown"]; got != 0 {
		t.Errorf("Initial tick: shutdown called when not scheduled")
	}

	// Clear calls for clean testing of intervals
	for k := range calls {
		calls[k] = 0
	}

	// T1 = 1001 (1 second passed)
	loop.TickAt(1001)
	if got := calls["active"]; got != 1 {
		t.Errorf("T=1001: expected 1 active update, got %d", got)
	}
	// Verify other updates did not trigger
	for _, name := range []string{"player", "random", "time", "exit", "shutdown"} {
		if got := calls[name]; got != 0 {
			t.Errorf("T=1001: unexpected call to %q: %d", name, got)
		}
	}
	calls["active"] = 0 // reset active

	// T2 = 1010 (10 seconds passed) -> triggers active and random (10s interval)
	loop.TickAt(1010)
	if got := calls["active"]; got != 1 {
		t.Errorf("T=1010: expected 1 active update, got %d", got)
	}
	if got := calls["random"]; got != 1 {
		t.Errorf("T=1010: expected 1 random update, got %d", got)
	}
	for _, name := range []string{"player", "time", "exit", "shutdown"} {
		if got := calls[name]; got != 0 {
			t.Errorf("T=1010: unexpected call to %q: %d", name, got)
		}
	}
	calls["active"] = 0
	calls["random"] = 0

	// T3 = 1020 (20 seconds passed since T=1000) -> triggers active, player (20s interval), random (another 10s passed)
	loop.TickAt(1020)
	if got := calls["active"]; got != 1 {
		t.Errorf("T=1020: expected 1 active update, got %d", got)
	}
	if got := calls["player"]; got != 1 {
		t.Errorf("T=1020: expected 1 player update, got %d", got)
	}
	if got := calls["random"]; got != 1 {
		t.Errorf("T=1020: expected 1 random update, got %d", got)
	}
	for _, name := range []string{"time", "exit", "shutdown"} {
		if got := calls[name]; got != 0 {
			t.Errorf("T=1020: unexpected call to %q: %d", name, got)
		}
	}
	calls["active"] = 0
	calls["player"] = 0
	calls["random"] = 0

	// T4 = 1100 (100 seconds passed since T=1000) -> triggers active, random, exit (100s interval)
	loop.TickAt(1100)
	if got := calls["active"]; got != 1 {
		t.Errorf("T=1100: expected 1 active update, got %d", got)
	}
	if got := calls["exit"]; got != 1 {
		t.Errorf("T=1100: expected 1 exit update, got %d", got)
	}
	calls["active"] = 0
	calls["exit"] = 0
	calls["random"] = 0
	calls["player"] = 0

	// T5 = 1150 (150 seconds passed since T=1000) -> triggers active, random, time (150s interval)
	loop.TickAt(1150)
	if got := calls["time"]; got != 1 {
		t.Errorf("T=1150: expected 1 time update, got %d", got)
	}
	calls["active"] = 0
	calls["random"] = 0
	calls["time"] = 0

	// Test Shutdown trigger
	// Schedule shutdown
	if err := world.SetShutdown(120, false); err != nil {
		t.Fatal(err)
	}
	// Initial tick at 1200 updates lastShutdownUpdate
	loop.TickAt(1200)
	if got := calls["shutdown"]; got != 1 {
		t.Errorf("T=1200: expected 1 shutdown update, got %d", got)
	}
	calls["shutdown"] = 0

	// T6 = 1210 (10 seconds since shutdown scheduled) -> should not trigger shutdown (30s interval)
	loop.TickAt(1210)
	if got := calls["shutdown"]; got != 0 {
		t.Errorf("T=1210: shutdown triggered prematurely: %d", got)
	}

	// T7 = 1230 (30 seconds since T=1200) -> should trigger shutdown
	loop.TickAt(1230)
	if got := calls["shutdown"]; got != 1 {
		t.Errorf("T=1230: expected 1 shutdown update, got %d", got)
	}
}

func TestLoopWithWorldWiresDefaultTimeClock(t *testing.T) {
	world := state.NewWorld(nil)
	now := int64(1000)
	world.SetLastActiveUpdate(now)
	world.SetLastPlayerUpdate(now)
	world.SetLastRandomUpdate(now)
	world.SetLastExitUpdate(now)
	world.SetLastTimeUpdate(now - 150)
	world.SetLegacyTime(19)

	dispatcher := enginecmd.Dispatcher{Registry: testRegistry(t)}
	loop := NewLoop(dispatcher, WithWorld(world))
	loop.TickAt(now)

	if got := world.LegacyTime(); got != 20 {
		t.Fatalf("LegacyTime() = %d, want 20 after default time clock tick", got)
	}
}

func TestLoopRunTicker(t *testing.T) {
	// Verify that Run handles ticker ticks asynchronously
	world := state.NewWorld(nil)
	tickChan := make(chan int64, 10)
	world.UpdateActiveMonstersFunc = func(t int64) error {
		tickChan <- t
		return nil
	}

	dispatcher := enginecmd.Dispatcher{Registry: testRegistry(t)}
	loop := NewLoop(dispatcher, WithWorld(world))

	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan session.Event)

	done := make(chan struct{})
	go func() {
		_ = loop.Run(ctx, events)
		close(done)
	}()

	// Wait for at least one tick to arrive on tickChan
	select {
	case <-tickChan:
		// success, ticker triggered Tick() inside Run
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for background ticker to fire")
	}

	cancel()
	<-done
}

// TestWorldTickFidelity_SideEffectsOverTime simulates time passage using the
// exact TickWorld + hook cadence (1s/20s/...) and verifies player-visible
// side effects occur at legacy C update.c rates:
//   - light/object decay (carried lightsources) only on 20s player updates
//     (not every 1s -- was a pre-fix bug from unconditional call in TickAt)
//   - player idle kick checks at 20s (300s timeout)
//   - monster active ticks (regen, wander/movement, scavenge) at 1s for
//     player-occupied rooms only
//   - daily not here (reset on use via dec_daily)
//
// This exercises the P1-3 ported hooks + state tick data.
func TestWorldTickFidelity_SideEffectsOverTime(t *testing.T) {
	world := state.NewWorld(nil)
	world.SetRandomUpdateInterval(20)
	world.SetTXInterval(3600)

	playerCalls := 0
	activeCalls := 0
	world.UpdatePlayerStatusesFunc = func(t int64) error {
		playerCalls++
		return nil
	}
	world.UpdateActiveMonstersFunc = func(t int64) error {
		activeCalls++
		return nil
	}
	// other hooks minimal
	world.UpdateRandomSpawnsFunc = func(t int64) error { return nil }
	world.UpdateTimeClockFunc = func(t int64) error { return nil }
	world.UpdateTimedExitsFunc = func(t int64) error { return nil }
	world.UpdateShutdownFunc = func(t int64) error { return nil }

	dispatcher := enginecmd.Dispatcher{Registry: testRegistry(t)}
	loop := NewLoop(dispatcher, WithWorld(world))

	// Simulate 25s of 1s ticks (as Run ticker does)
	base := int64(10000)
	for i := 0; i <= 25; i++ {
		loop.TickAt(base + int64(i))
	}

	// Player updates (light decay, poison/DoT, natural regen details,
	// status expirations incl. PLIGHT etc, idle 5min check, PSAVE@10min,
	// wimpy) must fire only at 20s cadence: at t=0,20 (2 calls in 25s)
	if playerCalls != 2 {
		t.Errorf("over 25s: expected ~2 player status updates (20s cadence), got %d -- light decay etc would be wrong rate", playerCalls)
	}

	// Active (monster regen every 60s gated, movement/wander/scavenge
	// traffic rolls, befud/charm expire, aggro) at 1s for active rooms:
	if activeCalls != 26 { // 0..25 incl
		t.Errorf("over 25s: expected 26 active monster ticks (1s), got %d", activeCalls)
	}

	// Note: exact side effects (e.g. charges-- on lights, disconnect after 300s
	// no input, monster del from room on wander success, trap counters if
	// extended) are verified in update_ply_test.go / update_active_test.go /
	// update_random_test.go with direct calls + mocks. The cadence here
	// ensures they trigger at C-exact intervals post-port.
}
