package state

import "github.com/0xc0de1ab/muhan/internal/world/model"

func (w *World) ShutdownSchedule() (ltime int64, interval int64) {
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	return w.shutdownLTime, w.shutdownInterval
}

func (w *World) LastShutdownUpdate() int64 {
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	return w.lastShutdownUpdate
}

func (w *World) SetLastShutdownUpdate(t int64) {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	w.lastShutdownUpdate = t
}

func (w *World) GetLightTimer(creatureID model.CreatureID, key string) (int64, bool) {
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	if m := w.lightTimers[creatureID]; m != nil {
		v, ok := m[key]
		return v, ok
	}
	return 0, false
}

func (w *World) SetLightTimer(creatureID model.CreatureID, key string, expires int64) {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	if w.lightTimers == nil {
		w.lightTimers = map[model.CreatureID]map[string]int64{}
	}
	m := w.lightTimers[creatureID]
	if m == nil {
		m = map[string]int64{}
		w.lightTimers[creatureID] = m
	}
	m[key] = expires
}

func (w *World) GetTrapState(roomID model.RoomID) TrapState {
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	if w.trapStates == nil {
		return TrapState{}
	}
	return w.trapStates[roomID]
}

func (w *World) SetTrapState(roomID model.RoomID, st TrapState) {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	if w.trapStates == nil {
		w.trapStates = map[model.RoomID]TrapState{}
	}
	w.trapStates[roomID] = st
}
