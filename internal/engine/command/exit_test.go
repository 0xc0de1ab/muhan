package command

import (
	"encoding/binary"
	"errors"
	"testing"

	"muhan/internal/commandspec"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestOpenExitHandlerOpensClosedExit(t *testing.T) {
	world := state.NewWorld(exitControlWorld(t))
	defer world.Close()
	dispatcher := exitControlDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "닫힌 열어")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if got := ctx.OutputString(); got != "당신은 닫힌쪽 출구를 열었습니다." {
		t.Fatalf("output = %q, want open confirmation", got)
	}
	exit := mustRuntimeExit(t, world, "room:start", "닫힌")
	if exitHasAnyFlag(exit, "closed", "xclosd", "xclosed") {
		t.Fatalf("exit flags = %+v, want closed removed", exit.Flags)
	}
	if got := exitLegacyLTime(exit); got <= 0 {
		t.Fatalf("exit ltime = %d, want touched", got)
	}
}

func TestOpenExitHandlerSupportsCommandFirstFallback(t *testing.T) {
	world := state.NewWorld(exitControlWorld(t))
	defer world.Close()
	dispatcher := exitControlDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "열어 닫힌"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); got != "당신은 닫힌쪽 출구를 열었습니다." {
		t.Fatalf("output = %q, want open confirmation", got)
	}
}

func TestCloseExitHandlerClosesClosableExit(t *testing.T) {
	world := state.NewWorld(exitControlWorld(t))
	defer world.Close()
	dispatcher := exitControlDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "열린 닫아"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); got != "당신은 열린쪽 출구를 닫습니다." {
		t.Fatalf("output = %q, want close confirmation", got)
	}
	exit := mustRuntimeExit(t, world, "room:start", "열린")
	if !exitHasAnyFlag(exit, "closed") {
		t.Fatalf("exit flags = %+v, want closed added", exit.Flags)
	}
}

func TestUnlockExitHandlerUnlocksAndConsumesKeyCharge(t *testing.T) {
	world := state.NewWorld(exitControlWorld(t))
	defer world.Close()
	dispatcher := exitControlDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "잠긴 열쇠 풀어"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); got != "딸깍" {
		t.Fatalf("output = %q, want key use output", got)
	}
	exit := mustRuntimeExit(t, world, "room:start", "잠긴")
	if exitHasAnyFlag(exit, "locked") {
		t.Fatalf("exit flags = %+v, want locked removed", exit.Flags)
	}
	if !exitHasAnyFlag(exit, "closed") {
		t.Fatalf("exit flags = %+v, want closed retained after unlock", exit.Flags)
	}
	if got := exitLegacyLTime(exit); got <= 0 {
		t.Fatalf("exit ltime = %d, want touched", got)
	}
	key, _ := world.Object("object:key")
	if got := key.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("key shotsCurrent = %q, want 1", got)
	}
}

func TestLockExitHandlerLocksClosedLockableExitWithoutConsumingKey(t *testing.T) {
	world := state.NewWorld(exitControlWorld(t))
	defer world.Close()
	dispatcher := exitControlDispatcher(t, world)

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "잠글 열쇠 잠궈"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); got != "## 찰칵 ##" {
		t.Fatalf("output = %q, want click output", got)
	}
	exit := mustRuntimeExit(t, world, "room:start", "잠글")
	if !exitHasAnyFlag(exit, "locked") {
		t.Fatalf("exit flags = %+v, want locked added", exit.Flags)
	}
	key, _ := world.Object("object:key")
	if got := key.Properties["shotsCurrent"]; got != "2" {
		t.Fatalf("key shotsCurrent = %q, want unchanged 2", got)
	}
}

func TestPicklockHandlerUnlocksLockedExitOnSuccessfulRoll(t *testing.T) {
	world := state.NewWorld(exitControlWorld(t))
	defer world.Close()
	dispatcher := exitControlDispatcherWithPicklock(t, world, func(int, int) int { return 1 })

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "잠긴 따"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); got != "당신은 문을 따는데 성공했습니다." {
		t.Fatalf("output = %q, want picklock success", got)
	}
	exit := mustRuntimeExit(t, world, "room:start", "잠긴")
	if exitHasAnyFlag(exit, "locked") {
		t.Fatalf("exit flags = %+v, want locked removed", exit.Flags)
	}
	if !exitHasAnyFlag(exit, "closed") {
		t.Fatalf("exit flags = %+v, want closed retained", exit.Flags)
	}
	if got := exitLegacyLTime(exit); got != 0 {
		t.Fatalf("exit ltime = %d, want picklock not to touch legacy timer", got)
	}
}

func TestPicklockHandlerLeavesExitLockedOnFailedRoll(t *testing.T) {
	world := state.NewWorld(exitControlWorld(t))
	defer world.Close()
	dispatcher := exitControlDispatcherWithPicklock(t, world, func(int, int) int { return 100 })

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "잠긴 따"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); got != "실패하였습니다!" {
		t.Fatalf("output = %q, want picklock failure", got)
	}
	exit := mustRuntimeExit(t, world, "room:start", "잠긴")
	if !exitHasAnyFlag(exit, "locked") {
		t.Fatalf("exit flags = %+v, want locked retained", exit.Flags)
	}
}

func TestPicklockHandlerCannotPickUnpickableExit(t *testing.T) {
	world := state.NewWorld(exitControlWorld(t))
	defer world.Close()
	dispatcher := exitControlDispatcherWithPicklock(t, world, func(int, int) int { return 1 })

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "안따짐 따"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if got := ctx.OutputString(); got != "실패하였습니다!" {
		t.Fatalf("output = %q, want picklock failure", got)
	}
	exit := mustRuntimeExit(t, world, "room:start", "안따짐")
	if !exitHasAnyFlag(exit, "locked") {
		t.Fatalf("exit flags = %+v, want locked retained", exit.Flags)
	}
}

func TestPicklockHandlerRejectsInvalidAttemptsWithoutMutation(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		setup func(*testing.T, *state.World)
		want  string
	}{
		{
			name: "non thief",
			line: "잠긴 따",
			setup: func(t *testing.T, world *state.World) {
				t.Helper()
				if err := world.SetCreatureStat("creature:alice", "class", 4); err != nil {
					t.Fatal(err)
				}
			},
			want: "도둑만 자물쇠를 딸 수 있습니다.",
		},
		{name: "missing target", line: "따", want: "무엇을 따시려구요?"},
		{name: "missing exit", line: "없는 따", want: "그런건 여기 없습니다."},
		{name: "not locked", line: "열린 따", want: "그것은 잠궈져 있지 않습니다."},
		{
			name: "blind",
			line: "잠긴 따",
			setup: func(t *testing.T, world *state.World) {
				t.Helper()
				if err := world.SetCreatureStat("creature:alice", "PBLIND", 1); err != nil {
					t.Fatal(err)
				}
			},
			want: "당신은 눈이 멀어 있어 딸 수 없습니다.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := state.NewWorld(exitControlWorld(t))
	defer world.Close()
			if tt.setup != nil {
				tt.setup(t, world)
			}
			dispatcher := exitControlDispatcherWithPicklock(t, world, func(int, int) int { return 1 })
			ctx := &Context{ActorID: "player:alice"}
			if _, err := dispatcher.DispatchLine(ctx, tt.line); err != nil {
				t.Fatalf("DispatchLine() error = %v", err)
			}
			if got := ctx.OutputString(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
			exit := mustRuntimeExit(t, world, "room:start", "잠긴")
			if !exitHasAnyFlag(exit, "locked") {
				t.Fatalf("locked exit flags = %+v, want locked retained", exit.Flags)
			}
		})
	}
}

func TestExitControlHandlerRejectsInvalidActionsWithoutMutation(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "open locked", line: "잠긴 열어", want: "그것은 잠겨져 있습니다."},
		{name: "open already open", line: "열린 열어", want: "벌써 열려져 있습니다."},
		{name: "close non closable", line: "안닫힘 닫아", want: "당신은 그 출구를 닫을 수 없습니다."},
		{name: "unlock wrong key", line: "잠긴 틀린열쇠 풀어", want: "열쇠가 맞지 않습니다."},
		{name: "lock open", line: "열린 열쇠 잠궈", want: "먼저 문을 닫아야 될것 같군요."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := state.NewWorld(exitControlWorld(t))
	defer world.Close()
			dispatcher := exitControlDispatcher(t, world)
			ctx := &Context{ActorID: "player:alice"}
			if _, err := dispatcher.DispatchLine(ctx, tt.line); err != nil {
				t.Fatalf("DispatchLine() error = %v", err)
			}
			if got := ctx.OutputString(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExitControlHandlersBroadcastLikeLegacy(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		roll       PicklockRollFunc
		wantOutput string
		wantTexts  []string
	}{
		{
			name:       "open",
			line:       "닫힌 열어",
			wantOutput: "당신은 닫힌쪽 출구를 열었습니다.",
			wantTexts:  []string{"\nAlice이 닫힌쪽 출구를 열었습니다."},
		},
		{
			name:       "close",
			line:       "열린 닫아",
			wantOutput: "당신은 열린쪽 출구를 닫습니다.",
			wantTexts:  []string{"\nAlice이 열린쪽 출구를 닫습니다."},
		},
		{
			name:       "unlock",
			line:       "잠긴 열쇠 풀어",
			wantOutput: "딸깍",
			wantTexts:  []string{"\nAlice이 잠긴쪽 출구를 풀었습니다."},
		},
		{
			name:       "lock",
			line:       "잠글 열쇠 잠궈",
			wantOutput: "## 찰칵 ##",
			wantTexts:  []string{"\nAlice이 잠글쪽 출구를 잠궜습니다."},
		},
		{
			name:       "pick failure",
			line:       "잠긴 따",
			roll:       func(int, int) int { return 100 },
			wantOutput: "실패하였습니다!",
			wantTexts:  []string{"\nAlice이 잠긴쪽 출구를 따려고 합니다."},
		},
		{
			name:       "pick success",
			line:       "잠긴 따",
			roll:       func(int, int) int { return 1 },
			wantOutput: "당신은 문을 따는데 성공했습니다.",
			wantTexts: []string{
				"\nAlice이 잠긴쪽 출구를 따려고 합니다.",
				"\nAlice이 문을 땄습니다.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := state.NewWorld(exitControlWorld(t))
	defer world.Close()
			roll := tt.roll
			if roll == nil {
				roll = func(int, int) int { return 1 }
			}
			dispatcher := exitControlDispatcherWithPicklock(t, world, roll)
			var broadcasts []struct {
				roomID  model.RoomID
				exclude string
				text    string
			}
			ctx := &Context{
				SessionID: "s1",
				ActorID:   "player:alice",
				Values: map[string]any{
					ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
						broadcasts = append(broadcasts, struct {
							roomID  model.RoomID
							exclude string
							text    string
						}{roomID: roomID, exclude: excludeSessionID, text: text})
						return errors.New("closed session")
					}),
				},
			}

			status, err := dispatcher.DispatchLine(ctx, tt.line)
			if err != nil {
				t.Fatalf("DispatchLine() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.wantOutput {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.wantOutput)
			}
			if len(broadcasts) != len(tt.wantTexts) {
				t.Fatalf("broadcasts = %+v, want %d entries", broadcasts, len(tt.wantTexts))
			}
			for i, want := range tt.wantTexts {
				if got := broadcasts[i]; got.roomID != "room:start" || got.exclude != "s1" || got.text != want {
					t.Fatalf("broadcast[%d] = %+v, want room:start/s1/%q", i, got, want)
				}
			}
		})
	}
}

func TestExitControlSuccessRevealsHiddenActorLikeLegacy(t *testing.T) {
	loaded := exitControlWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible"}
	alice.Stats["PHIDDN"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "invisible"}
	loaded.Players[player.ID] = player
	world := state.NewWorld(loaded)
	defer world.Close()
	dispatcher := exitControlDispatcher(t, world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := dispatcher.DispatchLine(ctx, "닫힌 열어")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 닫힌쪽 출구를 열었습니다." {
		t.Fatalf("status/output = %d/%q, want open confirmation", status, ctx.OutputString())
	}
	alice, _ = world.Creature("creature:alice")
	if hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "phiddn") || alice.Stats["PHIDDN"] != 0 {
		t.Fatalf("creature hidden state = tags:%+v stats:%+v", alice.Metadata.Tags, alice.Stats)
	}
	if !hasAnyNormalizedFlag(alice.Metadata.Tags, "invisible") {
		t.Fatalf("creature tags = %+v, want invisible retained", alice.Metadata.Tags)
	}
	player, _ = world.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("player hidden tags = %+v", player.Metadata.Tags)
	}
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "invisible") {
		t.Fatalf("player tags = %+v, want invisible retained", player.Metadata.Tags)
	}
}

func exitControlDispatcher(t *testing.T, world *state.World) Dispatcher {
	return exitControlDispatcherWithPicklock(t, world, func(int, int) int { return 1 })
}

func exitControlDispatcherWithPicklock(t *testing.T, world *state.World, roll PicklockRollFunc) Dispatcher {
	t.Helper()
	return Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "열어", Number: 31, Handler: "openexit"},
			{Name: "닫아", Number: 32, Handler: "closeexit"},
			{Name: "풀어", Number: 33, Handler: "unlock"},
			{Name: "잠궈", Number: 34, Handler: "lock"},
			{Name: "따", Number: 35, Handler: "picklock"},
		}),
		Handlers: map[string]Handler{
			"openexit":  NewOpenExitHandler(world),
			"closeexit": NewCloseExitHandler(world),
			"unlock":    NewUnlockExitHandler(world),
			"lock":      NewLockExitHandler(world),
			"picklock":  NewPicklockHandler(world, roll),
		},
	}
}

func exitControlWorld(t *testing.T) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:start",
		DisplayName: "Start",
		Exits: []model.Exit{
			{Name: "닫힌", ToRoomID: "room:east", Flags: []string{"closed", "closable"}},
			{Name: "열린", ToRoomID: "room:east", Flags: []string{"closable", "lockable", "key:7"}},
			{Name: "잠긴", ToRoomID: "room:east", Flags: []string{"locked", "closed", "lockable", "key:7"}},
			{Name: "안따짐", ToRoomID: "room:east", Flags: []string{"locked", "closed", "lockable", "unpickable"}},
			{Name: "잠글", ToRoomID: "room:east", Flags: []string{"closed", "lockable", "key:7"}},
			{Name: "안닫힘", ToRoomID: "room:east"},
		},
	})
	mustAddLookRoom(t, loaded, model.Room{ID: "room:east", DisplayName: "East"})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:start",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:start",
		Stats:       map[string]int{"class": 8, "level": 4},
		Inventory: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{
			"object:key",
			"object:wrong-key",
		}},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:key",
		Kind:        model.ObjectKindKey,
		DisplayName: "열쇠",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:key",
		PrototypeID: "prototype:key",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"nDice": "7", "shotsCurrent": "2", "useOutput": "딸깍"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:                  "object:wrong-key",
		PrototypeID:         "prototype:key",
		DisplayNameOverride: "틀린열쇠",
		Location:            model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:          map[string]string{"nDice": "8", "shotsCurrent": "1"},
	})
	return loaded
}

func mustRuntimeExit(t *testing.T, world *state.World, roomID model.RoomID, exitName string) model.Exit {
	t.Helper()
	room, ok := world.Room(roomID)
	if !ok {
		t.Fatalf("missing room %q", roomID)
	}
	for _, exit := range room.Exits {
		if exit.Name == exitName {
			return exit
		}
	}
	t.Fatalf("missing exit %q in room %q", exitName, roomID)
	return model.Exit{}
}

func exitLegacyLTime(exit model.Exit) int32 {
	raw := exit.Metadata.RawFields["ltime.ltime"]
	if len(raw) < 4 {
		return 0
	}
	return int32(binary.LittleEndian.Uint32(raw))
}
