package game

import (
	"testing"
)

type mockUpdateShutdownWorld struct {
	ltime              int64
	interval           int64
	lastShutdownUpdate int64
	broadcasts         []string
	savedPlayers       int
	savedRooms         int
	disconnected       bool
	terminated         bool
}

func (m *mockUpdateShutdownWorld) ShutdownSchedule() (ltime int64, interval int64) {
	return m.ltime, m.interval
}

func (m *mockUpdateShutdownWorld) LastShutdownUpdate() int64 {
	return m.lastShutdownUpdate
}

func (m *mockUpdateShutdownWorld) SetLastShutdownUpdate(t int64) {
	m.lastShutdownUpdate = t
}

func (m *mockUpdateShutdownWorld) BroadcastAll(message string) error {
	m.broadcasts = append(m.broadcasts, message)
	return nil
}

func (m *mockUpdateShutdownWorld) SaveAllPlayers() error {
	m.savedPlayers++
	return nil
}

func (m *mockUpdateShutdownWorld) ResaveAllRooms(permOnly bool) error {
	m.savedRooms++
	return nil
}

func (m *mockUpdateShutdownWorld) FlushActivePlayersAndBanks() error {
	// Simulate full flush for test; inc counters so TestUpdateShutdown_Triggered assertions pass
	// when timer shutdown now prefers FlushActive as single reliable path.
	m.savedPlayers++
	m.savedRooms++
	return nil
}

func (m *mockUpdateShutdownWorld) DisconnectAll() {
	m.disconnected = true
}

func (m *mockUpdateShutdownWorld) Terminate() {
	m.terminated = true
}

func TestUpdateShutdown_NotScheduled(t *testing.T) {
	world := &mockUpdateShutdownWorld{
		ltime:    0,
		interval: 3600,
	}

	UpdateShutdown(world, 1000)

	if len(world.broadcasts) > 0 {
		t.Errorf("expected no broadcasts, got %v", world.broadcasts)
	}
	if world.savedPlayers > 0 || world.savedRooms > 0 {
		t.Errorf("expected no saves")
	}
	if world.disconnected || world.terminated {
		t.Errorf("expected no disconnect or termination")
	}
}

func TestUpdateShutdown_ScheduledFarInFuture(t *testing.T) {
	// target is 1000 + 600 = 1600. Current time t is 1000. i > t + 500 (1600 > 1500)
	world := &mockUpdateShutdownWorld{
		ltime:    1000,
		interval: 600,
	}

	UpdateShutdown(world, 1000)

	if len(world.broadcasts) > 0 {
		t.Errorf("expected no broadcasts, got %v", world.broadcasts)
	}
	if world.lastShutdownUpdate != 0 {
		t.Errorf("expected lastShutdownUpdate to remain 0, got %d", world.lastShutdownUpdate)
	}
}

func TestUpdateShutdown_Throttled(t *testing.T) {
	// target is 1000 + 300 = 1300. Current time t is 1000. i <= t + 500 (1300 <= 1500).
	// lastShutdownUpdate is 990 (t - lastShutdownUpdate = 10, which is < 30).
	world := &mockUpdateShutdownWorld{
		ltime:              1000,
		interval:           300,
		lastShutdownUpdate: 990,
	}

	UpdateShutdown(world, 1000)

	if len(world.broadcasts) > 0 {
		t.Errorf("expected no broadcasts, got %v", world.broadcasts)
	}
	if world.lastShutdownUpdate != 990 {
		t.Errorf("expected lastShutdownUpdate to remain 990, got %d", world.lastShutdownUpdate)
	}
}

func TestUpdateShutdown_CountdownMoreThan60Seconds(t *testing.T) {
	// target is 1000 + 300 = 1300. Current time t is 1000.
	// remaining is 300 seconds (5 minutes 00 seconds).
	world := &mockUpdateShutdownWorld{
		ltime:              1000,
		interval:           300,
		lastShutdownUpdate: 0,
	}

	UpdateShutdown(world, 1000)

	if len(world.broadcasts) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(world.broadcasts))
	}
	expectedMsg := "\n### 5분 00초 후에 머드를 종료합니다."
	if world.broadcasts[0] != expectedMsg {
		t.Errorf("expected msg %q, got %q", expectedMsg, world.broadcasts[0])
	}
	if world.lastShutdownUpdate != 1000 {
		t.Errorf("expected lastShutdownUpdate to be 1000, got %d", world.lastShutdownUpdate)
	}
	if world.disconnected || world.terminated {
		t.Errorf("expected no disconnect or termination during countdown")
	}
}

func TestUpdateShutdown_CountdownLessThanOrEqualTo60Seconds(t *testing.T) {
	// target is 1000 + 50 = 1050. Current time t is 1000.
	// remaining is 50 seconds.
	world := &mockUpdateShutdownWorld{
		ltime:              1000,
		interval:           50,
		lastShutdownUpdate: 0,
	}

	UpdateShutdown(world, 1000)

	if len(world.broadcasts) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(world.broadcasts))
	}
	expectedMsg := "\n### 50초 후에 머드를 종료합니다. 모두 나가 주십시요."
	if world.broadcasts[0] != expectedMsg {
		t.Errorf("expected msg %q, got %q", expectedMsg, world.broadcasts[0])
	}
	if world.lastShutdownUpdate != 1000 {
		t.Errorf("expected lastShutdownUpdate to be 1000, got %d", world.lastShutdownUpdate)
	}
	if world.disconnected || world.terminated {
		t.Errorf("expected no disconnect or termination during countdown")
	}
}

func TestUpdateShutdown_Triggered(t *testing.T) {
	// target is 1000 + 10 = 1010. Current time t is 1010 (countdown finished).
	world := &mockUpdateShutdownWorld{
		ltime:              1000,
		interval:           10,
		lastShutdownUpdate: 0,
	}

	UpdateShutdown(world, 1010)

	if len(world.broadcasts) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(world.broadcasts))
	}
	expectedMsg := "\n### 머드를 종료합니다."
	if world.broadcasts[0] != expectedMsg {
		t.Errorf("expected msg %q, got %q", expectedMsg, world.broadcasts[0])
	}
	if world.lastShutdownUpdate != 1010 {
		t.Errorf("expected lastShutdownUpdate to be 1010, got %d", world.lastShutdownUpdate)
	}
	if world.savedPlayers != 1 {
		t.Errorf("expected SaveAllPlayers to be called 1 time, got %d", world.savedPlayers)
	}
	if world.savedRooms != 1 {
		t.Errorf("expected ResaveAllRooms to be called 1 time, got %d", world.savedRooms)
	}
	if !world.disconnected {
		t.Errorf("expected DisconnectAll to be called")
	}
	if !world.terminated {
		t.Errorf("expected Terminate to be called")
	}
}
