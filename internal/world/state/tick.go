package state

import (
	"fmt"
	"os"
)

var exitFunc = os.Exit

func SetExitFunc(fn func(code int)) {
	exitFunc = fn
}

func (w *World) TickWorld(t int64) error {
	if w == nil {
		return fmt.Errorf("tick world: world state is nil")
	}

	// 1. Every 1 second: call UpdateActiveMonsters(t)
	w.lockDomains(true, true, true, true, true, true, true)
	if t != w.lastActiveUpdate {
		w.lastActiveUpdate = t
		w.unlockDomains(true, true, true, true, true, true, true)
		if err := w.UpdateActiveMonsters(t); err != nil {
			// handle/log error if needed
		}
	} else {
		w.unlockDomains(true, true, true, true, true, true, true)
	}

	// 2. Every 20 seconds: call UpdatePlayerStatuses(t)
	w.lockDomains(true, true, true, true, true, true, true)
	if t-w.lastPlayerUpdate >= 20 {
		w.lastPlayerUpdate = t
		w.unlockDomains(true, true, true, true, true, true, true)
		if err := w.UpdatePlayerStatuses(t); err != nil {
			// handle/log error if needed
		}
	} else {
		w.unlockDomains(true, true, true, true, true, true, true)
	}

	// 3. Every Random_update_interval: call UpdateRandomSpawns(t)
	w.lockDomains(true, true, true, true, true, true, true)
	randomInt := w.randomUpdateInterval
	if randomInt == 0 {
		randomInt = 20
	}
	if t-w.lastRandomUpdate >= randomInt {
		w.lastRandomUpdate = t
		w.unlockDomains(true, true, true, true, true, true, true)
		if err := w.UpdateRandomSpawns(t); err != nil {
			// handle/log error if needed
		}
	} else {
		w.unlockDomains(true, true, true, true, true, true, true)
	}

	// 4. Every 150 seconds: call UpdateTimeClock(t)
	w.lockDomains(true, true, true, true, true, true, true)
	if t-w.lastTimeUpdate >= 150 {
		w.lastTimeUpdate = t
		w.unlockDomains(true, true, true, true, true, true, true)
		if err := w.UpdateTimeClock(t); err != nil {
			// handle/log error if needed
		}
	} else {
		w.unlockDomains(true, true, true, true, true, true, true)
	}

	// 5. Every TX_interval: call UpdateTimedExits(t)
	w.lockDomains(true, true, true, true, true, true, true)
	txInt := w.txInterval
	if txInt == 0 {
		txInt = 3600
	}
	if t-w.lastExitUpdate >= txInt {
		w.lastExitUpdate = t
		w.unlockDomains(true, true, true, true, true, true, true)
		if err := w.UpdateTimedExits(t); err != nil {
			// handle/log error if needed
		}
	} else {
		w.unlockDomains(true, true, true, true, true, true, true)
	}

	// 6. Every 30 seconds: call UpdateShutdown(t) if shutdown is scheduled
	w.lockDomains(true, true, true, true, true, true, true)
	if w.shutdownLTime != 0 && t-w.lastShutdownUpdate >= 30 {
		w.lastShutdownUpdate = t
		w.unlockDomains(true, true, true, true, true, true, true)
		if err := w.UpdateShutdown(t); err != nil {
			// handle/log error if needed
		}
	} else {
		w.unlockDomains(true, true, true, true, true, true, true)
	}

	return nil
}

func (w *World) UpdateActiveMonsters(t int64) error {
	w.rLockDomains(true, true, true, true, true, true, true)
	fn := w.UpdateActiveMonstersFunc
	w.rUnlockDomains(true, true, true, true, true, true, true)
	if fn != nil {
		return fn(t)
	}
	return nil
}

func (w *World) UpdatePlayerStatuses(t int64) error {
	w.rLockDomains(true, true, true, true, true, true, true)
	fn := w.UpdatePlayerStatusesFunc
	w.rUnlockDomains(true, true, true, true, true, true, true)
	if fn != nil {
		return fn(t)
	}
	return nil
}

func (w *World) UpdateRandomSpawns(t int64) error {
	w.rLockDomains(true, true, true, true, true, true, true)
	fn := w.UpdateRandomSpawnsFunc
	w.rUnlockDomains(true, true, true, true, true, true, true)
	if fn != nil {
		return fn(t)
	}
	return nil
}

func (w *World) UpdateTimeClock(t int64) error {
	w.rLockDomains(true, true, true, true, true, true, true)
	fn := w.UpdateTimeClockFunc
	w.rUnlockDomains(true, true, true, true, true, true, true)
	if fn != nil {
		return fn(t)
	}
	return nil
}

func (w *World) UpdateTimedExits(t int64) error {
	w.rLockDomains(true, true, true, true, true, true, true)
	fn := w.UpdateTimedExitsFunc
	w.rUnlockDomains(true, true, true, true, true, true, true)
	if fn != nil {
		return fn(t)
	}
	return nil
}

func (w *World) UpdateShutdown(t int64) error {
	w.rLockDomains(true, true, true, true, true, true, true)
	fn := w.UpdateShutdownFunc
	w.rUnlockDomains(true, true, true, true, true, true, true)
	if fn != nil {
		return fn(t)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	if w.shutdownLTime == 0 {
		return nil
	}

	target := w.shutdownLTime + w.shutdownInterval
	if target > t {
		diff := target - t
		if diff > 60 {
			msg := fmt.Sprintf("\n### %d분 %02d초 후에 머드를 종료합니다.", diff/60, diff%60)
			w.unlockDomains(true, true, true, true, true, true, true)
			_ = w.BroadcastAll(msg)
			w.lockDomains(true, true, true, true, true, true, true)
		} else {
			msg := fmt.Sprintf("\n### %d초 후에 머드를 종료합니다. 모두 나가 주십시요.", diff)
			w.unlockDomains(true, true, true, true, true, true, true)
			_ = w.BroadcastAll(msg)
			w.lockDomains(true, true, true, true, true, true, true)
		}
	} else {
		w.unlockDomains(true, true, true, true, true, true, true)
		_ = w.BroadcastAll("\n### 머드를 종료합니다.")
		w.lockDomains(true, true, true, true, true, true, true)
		exitFunc(0)
	}
	return nil
}

// Getters and Setters for scheduling fields

func (w *World) LastActiveUpdate() int64 {
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	return w.lastActiveUpdate
}

func (w *World) SetLastActiveUpdate(val int64) {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	w.lastActiveUpdate = val
}

func (w *World) LastPlayerUpdate() int64 {
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	return w.lastPlayerUpdate
}

func (w *World) SetLastPlayerUpdate(val int64) {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	w.lastPlayerUpdate = val
}

func (w *World) LastRandomUpdate() int64 {
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	return w.lastRandomUpdate
}

func (w *World) SetLastRandomUpdate(val int64) {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	w.lastRandomUpdate = val
}

func (w *World) LastTimeUpdate() int64 {
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	return w.lastTimeUpdate
}

func (w *World) SetLastTimeUpdate(val int64) {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	w.lastTimeUpdate = val
}

func (w *World) LegacyTime() int64 {
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	return w.legacyTime
}

func (w *World) SetLegacyTime(val int64) {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	w.legacyTime = val
}

func (w *World) IncrementTime() int64 {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	w.legacyTime++
	return w.legacyTime
}

func (w *World) LastExitUpdate() int64 {
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	return w.lastExitUpdate
}

func (w *World) SetLastExitUpdate(val int64) {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	w.lastExitUpdate = val
}

func (w *World) RandomUpdateInterval() int64 {
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	if w.randomUpdateInterval == 0 {
		return 20
	}
	return w.randomUpdateInterval
}

func (w *World) SetRandomUpdateInterval(val int64) {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	w.randomUpdateInterval = val
}

func (w *World) TXInterval() int64 {
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	if w.txInterval == 0 {
		return 3600
	}
	return w.txInterval
}

func (w *World) SetTXInterval(val int64) {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	w.txInterval = val
}
