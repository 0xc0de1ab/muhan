package game

import (
	"fmt"
	"log"
)

// UpdateShutdownWorld defines the interface required by UpdateShutdown.
type UpdateShutdownWorld interface {
	// ShutdownSchedule returns ltime and interval. ltime is 0 if no shutdown is scheduled.
	ShutdownSchedule() (ltime int64, interval int64)

	// LastShutdownUpdate returns the time (t) when the last shutdown message was sent.
	LastShutdownUpdate() int64

	// SetLastShutdownUpdate sets the last shutdown message time.
	SetLastShutdownUpdate(t int64)

	// BroadcastAll broadcasts a message to all active players/sessions.
	BroadcastAll(message string) error

	// SaveAllPlayers saves all player files.
	SaveAllPlayers() error

	// ResaveAllRooms saves all room files.
	ResaveAllRooms(permOnly bool) error

	// DisconnectAll closes all player connections/sessions.
	DisconnectAll()

	// Terminate terminates the process.
	Terminate()
}

// UpdateShutdown handles game shutdown timer warnings, file serialization, and safe exit.
func UpdateShutdown(world UpdateShutdownWorld, t int64) {
	ltime, interval := world.ShutdownSchedule()
	if ltime == 0 {
		return
	}

	lastUpdate := world.LastShutdownUpdate()

	// In C, the update runs every 30 seconds:
	// if(Shutdown.ltime && t - last_shutdown_update >= 30)
	if t-lastUpdate < 30 {
		return
	}

	i := ltime + interval
	// In C, it checks if(Shutdown.ltime + Shutdown.interval <= t+500)
	if i > t+500 {
		return
	}

	world.SetLastShutdownUpdate(t)

	if i > t {
		remaining := i - t
		if remaining > 60 {
			msg := fmt.Sprintf("\n### %d분 %02d초 후에 머드를 종료합니다.", remaining/60, remaining%60)
			_ = world.BroadcastAll(msg)
		} else {
			msg := fmt.Sprintf("\n### %d초 후에 머드를 종료합니다. 모두 나가 주십시요.", remaining)
			_ = world.BroadcastAll(msg)
		}
	} else {
		_ = world.BroadcastAll("\n### 머드를 종료합니다.")
		// Package B: Use FlushActivePlayersAndBanks as the SINGLE reliable full-flush path
		// for DM timer shutdown (integrates with signal/DM*shutdown). It covers players+banks
		// + full room floor objects (runtime drops/corpses, unlike ResaveAllRooms(permOnly=true)
		// which only does perm-tagged). C equiv was resave_all_rom(1)+save_all_ply before abrupt exit.
		if flusher, ok := world.(interface{ FlushActivePlayersAndBanks() error }); ok {
			if err := flusher.FlushActivePlayersAndBanks(); err != nil {
				log.Printf("[PERSIST] ERROR timer shutdown FlushActivePlayersAndBanks: %v", err)
			} else {
				log.Printf("[PERSIST] INFO timer shutdown full flush complete (players+banks+rooms via FlushActive)")
			}
		} else {
			// fallback (for some tests/mocks)
			_ = world.ResaveAllRooms(true)
			_ = world.SaveAllPlayers()
		}
		world.DisconnectAll()
		world.Terminate()
	}
}
