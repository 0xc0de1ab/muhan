package game

import (
	"fmt"
	"strconv"
	"sync"

	"muhan/internal/world/model"
)

type UpdateExitWorld interface {
	GetRoom(id model.RoomID) (model.Room, bool)
	SetExitFlag(roomID model.RoomID, exitName string, flag string, enabled bool) (model.Exit, error)
	BroadcastRoom(roomID model.RoomID, message string) error
}

type UpdateTimeWorld interface {
	IncrementTime() int64
	BroadcastAll(message string) error
}

type txTag struct {
	Room  int
	Name1 string
	Name2 string
	Exit1 int
	Exit2 int
	Mess1 string
	Mess2 string
}

var (
	timeXMu sync.Mutex
	timeX   = [5]txTag{
		{
			Room:  2655,
			Name1: "입구",
			Name2: "출구",
			Exit1: 2652,
			Exit2: 2651,
			Mess1: "자주 저장들 하세요.",
			Mess2: "자주 저장들 하세요.",
		},
	}
	tToggle bool
)

// ResetUpdateExitState resets the static variables to their initial state.
// This is primarily used for testing.
func ResetUpdateExitState() {
	timeXMu.Lock()
	defer timeXMu.Unlock()
	timeX = [5]txTag{
		{
			Room:  2655,
			Name1: "입구",
			Name2: "출구",
			Exit1: 2652,
			Exit2: 2651,
			Mess1: "자주 저장들 하세요.",
			Mess2: "자주 저장들 하세요.",
		},
	}
	tToggle = false
}

// UpdateTimedExits toggles exits configured in timeX array.
func UpdateTimedExits(world UpdateExitWorld, t int64) {
	timeXMu.Lock()
	defer timeXMu.Unlock()

	for i := 0; i < len(timeX); i++ {
		tx := &timeX[i]
		if tx.Room == 0 {
			// In C, load_rom on room 0 fails/returns, terminating the loop.
			return
		}

		room, actualRoomID, ok := findRoom(world, tx.Room)
		if !ok {
			// Room failed to load, return as in C
			return
		}

		// Find exits in the room and set/clear flags
		for _, exit := range room.Exits {
			if tx.Name1 != "" && matchToRoom(exit.ToRoomID, tx.Exit1) && exit.Name == tx.Name1 {
				_, _ = world.SetExitFlag(actualRoomID, exit.Name, "XNOSEE", true)
			}
			if tx.Name2 != "" && matchToRoom(exit.ToRoomID, tx.Exit2) && exit.Name == tx.Name2 {
				_, _ = world.SetExitFlag(actualRoomID, exit.Name, "XNOSEE", false)
			}
		}

		// Swap name1/exit1 with name2/exit2
		tx.Name1, tx.Name2 = tx.Name2, tx.Name1
		tx.Exit1, tx.Exit2 = tx.Exit2, tx.Exit1

		// Determine message
		var msg string
		if !tToggle {
			msg = tx.Mess1
		} else {
			msg = tx.Mess2
		}

		if msg != "" {
			_ = world.BroadcastRoom(actualRoomID, "\n"+msg)
		}

		// Toggle global toggle state
		tToggle = !tToggle
	}
}

// UpdateTimeClock increments the global MUD time and broadcasts messages.
func UpdateTimeClock(world UpdateTimeWorld, t int64) {
	newTime := world.IncrementTime()
	daytime := newTime % 24
	// In case time is negative, normalize daytime
	if daytime < 0 {
		daytime += 24
	}

	if daytime == 6 {
		_ = world.BroadcastAll("\n\n아침을 열어 주고 있는 태양, 내 맘을 자극하는 바람")
	} else if daytime == 20 {
		_ = world.BroadcastAll("\n\n바로 오늘이 두 개의 달이 떠오르는 밤이야.")
	}
}

func findRoom(world UpdateExitWorld, roomNum int) (model.Room, model.RoomID, bool) {
	candidates := []model.RoomID{
		model.RoomID(fmt.Sprintf("room:%05d", roomNum)),
		model.RoomID(fmt.Sprintf("room:%d", roomNum)),
		model.RoomID(strconv.Itoa(roomNum)),
	}
	for _, c := range candidates {
		if room, ok := world.GetRoom(c); ok {
			return room, c, true
		}
	}
	return model.Room{}, "", false
}

func matchToRoom(toRoomID model.RoomID, targetRoomNum int) bool {
	candidates := []model.RoomID{
		model.RoomID(fmt.Sprintf("room:%05d", targetRoomNum)),
		model.RoomID(fmt.Sprintf("room:%d", targetRoomNum)),
		model.RoomID(strconv.Itoa(targetRoomNum)),
	}
	for _, c := range candidates {
		if toRoomID == c {
			return true
		}
	}
	return false
}
