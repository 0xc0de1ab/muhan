package game

import (
	"fmt"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockTimeWorld struct {
	timeVal    int64
	broadcasts []string
}

func (m *mockTimeWorld) IncrementTime() int64 {
	m.timeVal++
	return m.timeVal
}

func (m *mockTimeWorld) BroadcastAll(msg string) error {
	m.broadcasts = append(m.broadcasts, msg)
	return nil
}

func TestUpdateTimeClock(t *testing.T) {
	tests := []struct {
		name          string
		initialTime   int64
		expectedTime  int64
		wantBroadcast []string
	}{
		{
			name:          "Morning broadcast at hour 6",
			initialTime:   5,
			expectedTime:  6,
			wantBroadcast: []string{"\n\n아침을 열어 주고 있는 태양, 내 맘을 자극하는 바람"},
		},
		{
			name:          "Night broadcast at hour 20",
			initialTime:   19,
			expectedTime:  20,
			wantBroadcast: []string{"\n\n바로 오늘이 두 개의 달이 떠오르는 밤이야."},
		},
		{
			name:          "No broadcast at hour 7",
			initialTime:   6,
			expectedTime:  7,
			wantBroadcast: nil,
		},
		{
			name:          "No broadcast at hour 21",
			initialTime:   20,
			expectedTime:  21,
			wantBroadcast: nil,
		},
		{
			name:          "Wrap around morning broadcast at hour 30 (6 mod 24)",
			initialTime:   29,
			expectedTime:  30,
			wantBroadcast: []string{"\n\n아침을 열어 주고 있는 태양, 내 맘을 자극하는 바람"},
		},
		{
			name:          "Negative time normalization morning broadcast at hour -18 (-18 mod 24 = 6)",
			initialTime:   -19,
			expectedTime:  -18,
			wantBroadcast: []string{"\n\n아침을 열어 주고 있는 태양, 내 맘을 자극하는 바람"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockTimeWorld{timeVal: tt.initialTime}
			UpdateTimeClock(world, 0)

			if world.timeVal != tt.expectedTime {
				t.Errorf("expected time %d, got %d", tt.expectedTime, world.timeVal)
			}

			if len(world.broadcasts) != len(tt.wantBroadcast) {
				t.Errorf("expected %d broadcasts, got %d", len(tt.wantBroadcast), len(world.broadcasts))
			} else {
				for i, msg := range world.broadcasts {
					if msg != tt.wantBroadcast[i] {
						t.Errorf("broadcast[%d] = %q, want %q", i, msg, tt.wantBroadcast[i])
					}
				}
			}
		})
	}
}

type setFlagCall struct {
	RoomID   model.RoomID
	ExitName string
	Flag     string
	Enabled  bool
}

type broadcastRoomCall struct {
	RoomID  model.RoomID
	Message string
}

type mockExitWorld struct {
	rooms      map[model.RoomID]model.Room
	setFlags   []setFlagCall
	broadcasts []broadcastRoomCall
}

func (m *mockExitWorld) GetRoom(id model.RoomID) (model.Room, bool) {
	r, ok := m.rooms[id]
	return r, ok
}

func (m *mockExitWorld) SetExitFlag(roomID model.RoomID, exitName string, flag string, enabled bool) (model.Exit, error) {
	m.setFlags = append(m.setFlags, setFlagCall{
		RoomID:   roomID,
		ExitName: exitName,
		Flag:     flag,
		Enabled:  enabled,
	})

	if room, ok := m.rooms[roomID]; ok {
		for i, exit := range room.Exits {
			if exit.Name == exitName {
				if enabled {
					found := false
					for _, f := range exit.Flags {
						if f == flag {
							found = true
							break
						}
					}
					if !found {
						exit.Flags = append(exit.Flags, flag)
					}
				} else {
					var newFlags []string
					for _, f := range exit.Flags {
						if f != flag {
							newFlags = append(newFlags, f)
						}
					}
					exit.Flags = newFlags
				}
				room.Exits[i] = exit
				m.rooms[roomID] = room
				return exit, nil
			}
		}
	}
	return model.Exit{}, fmt.Errorf("exit not found")
}

func (m *mockExitWorld) BroadcastRoom(roomID model.RoomID, message string) error {
	m.broadcasts = append(m.broadcasts, broadcastRoomCall{
		RoomID:  roomID,
		Message: message,
	})
	return nil
}

func TestUpdateTimedExits(t *testing.T) {
	// Format: room:02655
	t.Run("Standard ID format room:02655", func(t *testing.T) {
		ResetUpdateExitState()
		world := &mockExitWorld{
			rooms: map[model.RoomID]model.Room{
				"room:02655": {
					ID:          "room:02655",
					DisplayName: "Room 2655",
					Exits: []model.Exit{
						{Name: "입구", ToRoomID: "room:02652"},
						{Name: "출구", ToRoomID: "room:02651"},
					},
				},
			},
		}

		// First Tick
		UpdateTimedExits(world, 0)

		// Verification:
		// "입구" exit destination matches 2652 -> Set XNOSEE to true
		// "출구" exit destination matches 2651 -> Set XNOSEE to false
		if len(world.setFlags) != 2 {
			t.Fatalf("expected 2 SetExitFlag calls, got %d", len(world.setFlags))
		}
		if world.setFlags[0] != (setFlagCall{RoomID: "room:02655", ExitName: "입구", Flag: "XNOSEE", Enabled: true}) {
			t.Errorf("unexpected first setflag call: %+v", world.setFlags[0])
		}
		if world.setFlags[1] != (setFlagCall{RoomID: "room:02655", ExitName: "출구", Flag: "XNOSEE", Enabled: false}) {
			t.Errorf("unexpected second setflag call: %+v", world.setFlags[1])
		}

		if len(world.broadcasts) != 1 {
			t.Fatalf("expected 1 room broadcast, got %d", len(world.broadcasts))
		}
		if world.broadcasts[0] != (broadcastRoomCall{RoomID: "room:02655", Message: "\n자주 저장들 하세요."}) {
			t.Errorf("unexpected broadcast message: %+v", world.broadcasts[0])
		}

		// Now check state swap
		timeXMu.Lock()
		state0 := timeX[0]
		timeXMu.Unlock()
		if state0.Name1 != "출구" || state0.Exit1 != 2651 || state0.Name2 != "입구" || state0.Exit2 != 2652 {
			t.Errorf("expected swap to happen, got state0: %+v", state0)
		}

		// Clear mock records to check the second tick
		world.setFlags = nil
		world.broadcasts = nil

		// Second Tick
		UpdateTimedExits(world, 0)

		// Verification:
		// Now Name1 is "출구" (2651) -> Set XNOSEE to true
		// Name2 is "입구" (2652) -> Set XNOSEE to false
		if len(world.setFlags) != 2 {
			t.Fatalf("expected 2 SetExitFlag calls, got %d", len(world.setFlags))
		}
		if world.setFlags[0] != (setFlagCall{RoomID: "room:02655", ExitName: "입구", Flag: "XNOSEE", Enabled: false}) {
			t.Errorf("unexpected second tick first setflag call: %+v", world.setFlags[0])
		}
		if world.setFlags[1] != (setFlagCall{RoomID: "room:02655", ExitName: "출구", Flag: "XNOSEE", Enabled: true}) {
			t.Errorf("unexpected second tick second setflag call: %+v", world.setFlags[1])
		}

		if len(world.broadcasts) != 1 {
			t.Fatalf("expected 1 room broadcast, got %d", len(world.broadcasts))
		}
		if world.broadcasts[0] != (broadcastRoomCall{RoomID: "room:02655", Message: "\n자주 저장들 하세요."}) {
			t.Errorf("unexpected broadcast message: %+v", world.broadcasts[0])
		}

		// Now state should swap back
		timeXMu.Lock()
		state0Back := timeX[0]
		timeXMu.Unlock()
		if state0Back.Name1 != "입구" || state0Back.Exit1 != 2652 || state0Back.Name2 != "출구" || state0Back.Exit2 != 2651 {
			t.Errorf("expected swap back to happen, got state0: %+v", state0Back)
		}
	})

	// Format: "2655"
	t.Run("Alternative ID format 2655", func(t *testing.T) {
		ResetUpdateExitState()
		world := &mockExitWorld{
			rooms: map[model.RoomID]model.Room{
				"2655": {
					ID:          "2655",
					DisplayName: "Room 2655",
					Exits: []model.Exit{
						{Name: "입구", ToRoomID: "2652"},
						{Name: "출구", ToRoomID: "2651"},
					},
				},
			},
		}

		UpdateTimedExits(world, 0)

		if len(world.setFlags) != 2 {
			t.Fatalf("expected 2 SetExitFlag calls, got %d", len(world.setFlags))
		}
		if world.setFlags[0].RoomID != "2655" || world.setFlags[0].ExitName != "입구" || !world.setFlags[0].Enabled {
			t.Errorf("unexpected setflag: %+v", world.setFlags[0])
		}
	})

	// Handling room load failure
	t.Run("Room load failure does not crash and returns early", func(t *testing.T) {
		ResetUpdateExitState()
		world := &mockExitWorld{
			rooms: map[model.RoomID]model.Room{},
		}

		UpdateTimedExits(world, 0)

		if len(world.setFlags) != 0 {
			t.Errorf("expected 0 setflag calls, got %d", len(world.setFlags))
		}
	})
}
