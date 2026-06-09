package state

import (
	"fmt"
	"os"
)

var exitFunc = os.Exit

func (w *World) TickWorld(t int64) error {
	if w == nil {
		return fmt.Errorf("tick world: world state is nil")
	}

	// 1. Every 1 second: call UpdateActiveMonsters(t)
	var lastActive int64
	w.mu.RLock()
	lastActive = w.lastActiveUpdate
	w.mu.RUnlock()

	if t != lastActive {
		w.mu.Lock()
		w.lastActiveUpdate = t
		w.mu.Unlock()
		if err := w.UpdateActiveMonsters(t); err != nil {
			// handle/log error if needed
		}
	}

	// 2. Every 20 seconds: call UpdatePlayerStatuses(t)
	var lastPlayer int64
	w.mu.RLock()
	lastPlayer = w.lastPlayerUpdate
	w.mu.RUnlock()

	if t-lastPlayer >= 20 {
		w.mu.Lock()
		w.lastPlayerUpdate = t
		w.mu.Unlock()
		if err := w.UpdatePlayerStatuses(t); err != nil {
			// handle/log error if needed
		}
	}

	// 3. Every Random_update_interval: call UpdateRandomSpawns(t)
	var lastRandom, randomInt int64
	w.mu.RLock()
	lastRandom = w.lastRandomUpdate
	randomInt = w.randomUpdateInterval
	if randomInt == 0 {
		randomInt = 20
	}
	w.mu.RUnlock()

	if t-lastRandom >= randomInt {
		w.mu.Lock()
		w.lastRandomUpdate = t
		w.mu.Unlock()
		if err := w.UpdateRandomSpawns(t); err != nil {
			// handle/log error if needed
		}
	}

	// 4. Every 150 seconds: call UpdateTimeClock(t)
	var lastTime int64
	w.mu.RLock()
	lastTime = w.lastTimeUpdate
	w.mu.RUnlock()

	if t-lastTime >= 150 {
		w.mu.Lock()
		w.lastTimeUpdate = t
		w.mu.Unlock()
		if err := w.UpdateTimeClock(t); err != nil {
			// handle/log error if needed
		}
	}

	// 5. Every TX_interval: call UpdateTimedExits(t)
	var lastExit, txInt int64
	w.mu.RLock()
	lastExit = w.lastExitUpdate
	txInt = w.txInterval
	if txInt == 0 {
		txInt = 3600
	}
	w.mu.RUnlock()

	if t-lastExit >= txInt {
		w.mu.Lock()
		w.lastExitUpdate = t
		w.mu.Unlock()
		if err := w.UpdateTimedExits(t); err != nil {
			// handle/log error if needed
		}
	}

	// 6. Every 30 seconds: call UpdateShutdown(t) if shutdown is scheduled
	var lastShutdown, shutdownLTime int64
	w.mu.RLock()
	lastShutdown = w.lastShutdownUpdate
	shutdownLTime = w.shutdownLTime
	w.mu.RUnlock()

	if shutdownLTime != 0 && t-lastShutdown >= 30 {
		w.mu.Lock()
		w.lastShutdownUpdate = t
		w.mu.Unlock()
		if err := w.UpdateShutdown(t); err != nil {
			// handle/log error if needed
		}
	}

	return nil
}

func (w *World) UpdateActiveMonsters(t int64) error {
	w.mu.RLock()
	fn := w.UpdateActiveMonstersFunc
	w.mu.RUnlock()
	if fn != nil {
		return fn(t)
	}
	return nil
}

func (w *World) UpdatePlayerStatuses(t int64) error {
	w.mu.RLock()
	fn := w.UpdatePlayerStatusesFunc
	w.mu.RUnlock()
	if fn != nil {
		return fn(t)
	}
	return nil
}

func (w *World) UpdateRandomSpawns(t int64) error {
	w.mu.RLock()
	fn := w.UpdateRandomSpawnsFunc
	w.mu.RUnlock()
	if fn != nil {
		return fn(t)
	}
	return nil
}

func (w *World) UpdateTimeClock(t int64) error {
	w.mu.RLock()
	fn := w.UpdateTimeClockFunc
	w.mu.RUnlock()
	if fn != nil {
		return fn(t)
	}
	return nil
}

func (w *World) UpdateTimedExits(t int64) error {
	w.mu.RLock()
	fn := w.UpdateTimedExitsFunc
	w.mu.RUnlock()
	if fn != nil {
		return fn(t)
	}
	return nil
}

func (w *World) UpdateShutdown(t int64) error {
	w.mu.RLock()
	fn := w.UpdateShutdownFunc
	w.mu.RUnlock()
	if fn != nil {
		return fn(t)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.shutdownLTime == 0 {
		return nil
	}

	target := w.shutdownLTime + w.shutdownInterval
	if target > t {
		diff := target - t
		if diff > 60 {
			msg := fmt.Sprintf("\n### %d분 %02d초 후에 머드를 종료합니다.", diff/60, diff%60)
			w.mu.Unlock()
			_ = w.BroadcastAll(msg)
			w.mu.Lock()
		} else {
			msg := fmt.Sprintf("\n### %d초 후에 머드를 종료합니다. 모두 나가 주십시요.", diff)
			w.mu.Unlock()
			_ = w.BroadcastAll(msg)
			w.mu.Lock()
		}
	} else {
		w.mu.Unlock()
		_ = w.BroadcastAll("\n### 머드를 종료합니다.")
		w.mu.Lock()
		exitFunc(0)
	}
	return nil
}

// Getters and Setters for scheduling fields

func (w *World) LastActiveUpdate() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastActiveUpdate
}

func (w *World) SetLastActiveUpdate(val int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastActiveUpdate = val
}

func (w *World) LastPlayerUpdate() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastPlayerUpdate
}

func (w *World) SetLastPlayerUpdate(val int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastPlayerUpdate = val
}

func (w *World) LastRandomUpdate() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastRandomUpdate
}

func (w *World) SetLastRandomUpdate(val int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastRandomUpdate = val
}

func (w *World) LastTimeUpdate() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastTimeUpdate
}

func (w *World) SetLastTimeUpdate(val int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastTimeUpdate = val
}

func (w *World) LegacyTime() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.legacyTime
}

func (w *World) SetLegacyTime(val int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.legacyTime = val
}

func (w *World) IncrementTime() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.legacyTime++
	return w.legacyTime
}

func (w *World) LastExitUpdate() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastExitUpdate
}

func (w *World) SetLastExitUpdate(val int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastExitUpdate = val
}

func (w *World) RandomUpdateInterval() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.randomUpdateInterval == 0 {
		return 20
	}
	return w.randomUpdateInterval
}

func (w *World) SetRandomUpdateInterval(val int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.randomUpdateInterval = val
}

func (w *World) TXInterval() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.txInterval == 0 {
		return 3600
	}
	return w.txInterval
}

func (w *World) SetTXInterval(val int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.txInterval = val
}
