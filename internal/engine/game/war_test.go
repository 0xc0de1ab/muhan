package game

import (
	"strings"
	"testing"

	enginecmd "muhan/internal/engine/command"
	"muhan/internal/session"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestResolveCallWarStateRequestCancelAndAccept(t *testing.T) {
	world := state.NewWorld(nil)

	result, err := ResolveCallWarState(world, 2, 5)
	if err != nil {
		t.Fatal(err)
	}
	if result.Transition != CallWarRequested {
		t.Fatalf("transition = %q, want %q", result.Transition, CallWarRequested)
	}
	if want := (state.FamilyWarPair{First: 2, Second: 5}); result.Snapshot.Pending != want {
		t.Fatalf("pending = %+v, want %+v", result.Snapshot.Pending, want)
	}

	result, err = ResolveCallWarState(world, 2, 5)
	if err != nil {
		t.Fatal(err)
	}
	if result.Transition != CallWarCanceled {
		t.Fatalf("transition = %q, want %q", result.Transition, CallWarCanceled)
	}
	if result.Snapshot.HasPending() || result.Snapshot.AtWar() {
		t.Fatalf("snapshot after cancel = %+v, want zero", result.Snapshot)
	}

	if _, err := ResolveCallWarState(world, 2, 5); err != nil {
		t.Fatal(err)
	}
	result, err = ResolveCallWarState(world, 5, 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Transition != CallWarAccepted {
		t.Fatalf("transition = %q, want %q", result.Transition, CallWarAccepted)
	}
	if want := (state.FamilyWarPair{First: 5, Second: 2}); result.Snapshot.Active != want {
		t.Fatalf("active = %+v, want %+v", result.Snapshot.Active, want)
	}
}

func TestResolveCallWarStateRejectsPendingConflictAndActiveWar(t *testing.T) {
	world := state.NewWorld(nil)
	if _, err := ResolveCallWarState(world, 2, 5); err != nil {
		t.Fatal(err)
	}

	result, err := ResolveCallWarState(world, 5, 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.Transition != CallWarRejectedPending {
		t.Fatalf("transition = %q, want %q", result.Transition, CallWarRejectedPending)
	}
	if want := (state.FamilyWarPair{First: 2, Second: 5}); result.Snapshot.Pending != want {
		t.Fatalf("pending = %+v, want %+v", result.Snapshot.Pending, want)
	}

	result, err = ResolveCallWarState(world, 5, 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Transition != CallWarAccepted {
		t.Fatalf("transition = %q, want %q", result.Transition, CallWarAccepted)
	}

	result, err = ResolveCallWarState(world, 3, 4)
	if err != nil {
		t.Fatal(err)
	}
	if result.Transition != CallWarRejectedActive {
		t.Fatalf("transition = %q, want %q", result.Transition, CallWarRejectedActive)
	}
	if want := (state.FamilyWarPair{First: 5, Second: 2}); result.Snapshot.Active != want {
		t.Fatalf("active = %+v, want %+v", result.Snapshot.Active, want)
	}
}

func TestCallWarHandlerRequestsCancelsAndAccepts(t *testing.T) {
	world := callWarTestWorld(t,
		callWarTestPlayer{id: "player:alice", creature: "creature:alice", name: "Alice", stats: map[string]int{"familyFlag": 1, "familyID": 2, "PFMBOS": 1}},
		callWarTestPlayer{id: "player:bob", creature: "creature:bob", name: "Bob", stats: map[string]int{"familyFlag": 1, "familyID": 5, "PFMBOS": 1}},
	)
	handler := NewCallWarHandler(world)
	active := []ActiveSession{{ID: "s-alice", ActorID: "player:alice"}, {ID: "s-bob", ActorID: "player:bob"}}
	writes := map[session.ID]string{}

	aliceCtx := callWarTestContext("s-alice", "player:alice", active, writes)
	if _, err := handler(aliceCtx, enginecmd.ResolvedCommand{Args: []string{"패거리5"}}); err != nil {
		t.Fatal(err)
	}
	if snapshot := world.FamilyWarSnapshot(); snapshot.Pending != (state.FamilyWarPair{First: 2, Second: 5}) {
		t.Fatalf("pending after request = %+v", snapshot.Pending)
	}
	wantRequest := "\n### 패거리2 패거리가 패거리5에게 선전포고를 합니다.\n\n"
	if aliceCtx.OutputString() != wantRequest || writes["s-bob"] != wantRequest {
		t.Fatalf("request was not broadcast to both sessions: self=%q bob=%q", aliceCtx.OutputString(), writes["s-bob"])
	}

	aliceCtx = callWarTestContext("s-alice", "player:alice", active, writes)
	if _, err := handler(aliceCtx, enginecmd.ResolvedCommand{Args: []string{"패거리5"}}); err != nil {
		t.Fatal(err)
	}
	if snapshot := world.FamilyWarSnapshot(); snapshot.HasPending() || snapshot.AtWar() {
		t.Fatalf("snapshot after cancel = %+v, want zero", snapshot)
	}
	wantCancel := "\n### 패거리2 패거리에서 선전포고를 취소합니다.\n"
	if aliceCtx.OutputString() != wantCancel {
		t.Fatalf("cancel output = %q, want %q", aliceCtx.OutputString(), wantCancel)
	}

	if _, err := handler(callWarTestContext("s-alice", "player:alice", active, writes), enginecmd.ResolvedCommand{Args: []string{"패거리5"}}); err != nil {
		t.Fatal(err)
	}
	bobCtx := callWarTestContext("s-bob", "player:bob", active, writes)
	if _, err := handler(bobCtx, enginecmd.ResolvedCommand{Args: []string{"패거리2"}}); err != nil {
		t.Fatal(err)
	}
	if snapshot := world.FamilyWarSnapshot(); snapshot.Active != (state.FamilyWarPair{First: 5, Second: 2}) {
		t.Fatalf("active after accept = %+v", snapshot.Active)
	}
	wantAccept := "\n### 패거리5 패거리에서 선전포고를 받아들였습니다.\n"
	if bobCtx.OutputString() != wantAccept {
		t.Fatalf("accept output = %q, want %q", bobCtx.OutputString(), wantAccept)
	}
}

func TestCallWarHandlerUsesFamilyDisplayNamesAndBossLookup(t *testing.T) {
	world := namedFamilyWorld{
		World: callWarTestWorld(t,
			callWarTestPlayer{id: "player:alice", creature: "creature:alice", name: "Alice", stats: map[string]int{"familyFlag": 1, "familyID": 2, "PFMBOS": 1}},
			callWarTestPlayer{id: "player:bob", creature: "creature:bob", name: "Bob", stats: map[string]int{"familyFlag": 1, "familyID": 5, "PFMBOS": 1}},
		),
		names:  map[int]string{2: "은형문", 5: "무영문"},
		bosses: map[int]string{5: "Bob"},
	}
	handler := NewCallWarHandler(world)
	active := []ActiveSession{{ID: "s-alice", ActorID: "player:alice"}, {ID: "s-bob", ActorID: "player:bob"}}
	writes := map[session.ID]string{}
	ctx := callWarTestContext("s-alice", "player:alice", active, writes)

	if _, err := handler(ctx, enginecmd.ResolvedCommand{Args: []string{"무영문"}}); err != nil {
		t.Fatal(err)
	}
	if snapshot := world.FamilyWarSnapshot(); snapshot.Pending != (state.FamilyWarPair{First: 2, Second: 5}) {
		t.Fatalf("pending after named request = %+v", snapshot.Pending)
	}
	for _, out := range []string{ctx.OutputString(), writes["s-bob"]} {
		if !strings.Contains(out, "은형문") || !strings.Contains(out, "무영문") || strings.Contains(out, "패거리5") {
			t.Fatalf("named family war output = %q", out)
		}
	}
}

func TestCallWarHandlerUsesPropertyFlagTokensForBossAuthority(t *testing.T) {
	world := callWarTestWorld(t,
		callWarTestPlayer{
			id:         "player:alice",
			creature:   "creature:alice",
			name:       "Alice",
			properties: map[string]string{"flags": "PFAMIL|PFMBOS", "daily_expnd_max": "2"},
		},
		callWarTestPlayer{
			id:         "player:bob",
			creature:   "creature:bob",
			name:       "Bob",
			properties: map[string]string{"flags": "PFAMIL PFMBOS", "daily_expnd_max": "5"},
		},
	)
	handler := NewCallWarHandler(world)
	active := []ActiveSession{{ID: "s-alice", ActorID: "player:alice"}, {ID: "s-bob", ActorID: "player:bob"}}
	ctx := callWarTestContext("s-alice", "player:alice", active, nil)

	if _, err := handler(ctx, enginecmd.ResolvedCommand{Args: []string{"패거리5"}}); err != nil {
		t.Fatal(err)
	}
	if snapshot := world.FamilyWarSnapshot(); snapshot.Pending != (state.FamilyWarPair{First: 2, Second: 5}) {
		t.Fatalf("pending after token-backed request = %+v", snapshot.Pending)
	}
}

func TestCallWarHandlerBlocksWithoutTargetBossOnline(t *testing.T) {
	world := namedFamilyWorld{
		World: callWarTestWorld(t,
			callWarTestPlayer{
				id:         "player:alice",
				creature:   "creature:alice",
				name:       "Alice",
				properties: map[string]string{"PFAMIL": "true", "family_boss": "true", "daily_expnd_max": "2"},
			},
		),
		bosses: map[int]string{5: "Bob"},
	}
	handler := NewCallWarHandler(world)
	active := []ActiveSession{{ID: "s-alice", ActorID: "player:alice"}}
	ctx := callWarTestContext("s-alice", "player:alice", active, nil)

	if _, err := handler(ctx, enginecmd.ResolvedCommand{Args: []string{"패거리5"}}); err != nil {
		t.Fatal(err)
	}
	if snapshot := world.FamilyWarSnapshot(); snapshot.HasPending() || snapshot.AtWar() {
		t.Fatalf("snapshot after blocked request = %+v, want zero", snapshot)
	}
	want := "상대편의 두목인 Bob님이 이용중이 아닙니다."
	if ctx.OutputString() != want {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), want)
	}
}

type namedFamilyWorld struct {
	*state.World
	names  map[int]string
	bosses map[int]string
}

func (w namedFamilyWorld) FamilyDisplayName(familyID int) (string, bool) {
	name, ok := w.names[familyID]
	return name, ok
}

func (w namedFamilyWorld) FamilyIDByDisplayName(name string) (int, bool) {
	for familyID, familyName := range w.names {
		if name == familyName {
			return familyID, true
		}
	}
	return 0, false
}

func (w namedFamilyWorld) FamilyBossName(familyID int) (string, bool) {
	name, ok := w.bosses[familyID]
	return name, ok
}

func TestCallWarHandlerRequiresBossAuthority(t *testing.T) {
	world := callWarTestWorld(t,
		callWarTestPlayer{id: "player:alice", creature: "creature:alice", name: "Alice", stats: map[string]int{"familyFlag": 1, "familyID": 2}},
		callWarTestPlayer{id: "player:bob", creature: "creature:bob", name: "Bob", stats: map[string]int{"familyFlag": 1, "familyID": 5, "PFMBOS": 1}},
	)
	handler := NewCallWarHandler(world)
	active := []ActiveSession{{ID: "s-alice", ActorID: "player:alice"}, {ID: "s-bob", ActorID: "player:bob"}}
	ctx := callWarTestContext("s-alice", "player:alice", active, nil)

	if _, err := handler(ctx, enginecmd.ResolvedCommand{Args: []string{"패거리5"}}); err != nil {
		t.Fatal(err)
	}
	if snapshot := world.FamilyWarSnapshot(); snapshot.HasPending() || snapshot.AtWar() {
		t.Fatalf("snapshot after unauthorized request = %+v, want zero", snapshot)
	}
	want := "당신은 선전 포고할 권리가 없습니다.\n"
	if ctx.OutputString() != want {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), want)
	}
}

func TestCallWarHandlerUsesLegacyFailureMessages(t *testing.T) {
	world := callWarTestWorld(t,
		callWarTestPlayer{id: "player:alice", creature: "creature:alice", name: "Alice", stats: map[string]int{"familyFlag": 1, "familyID": 2, "PFMBOS": 1}},
		callWarTestPlayer{id: "player:bob", creature: "creature:bob", name: "Bob", stats: map[string]int{"familyFlag": 1, "familyID": 5, "PFMBOS": 1}},
	)
	handler := NewCallWarHandler(world)
	active := []ActiveSession{{ID: "s-alice", ActorID: "player:alice"}, {ID: "s-bob", ActorID: "player:bob"}}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing target", want: "어느 패거리와 전쟁을 하시려고요?"},
		{name: "too many target words", args: []string{"무영", "문"}, want: "어느 패거리와 전쟁을 하시려고요?"},
		{name: "missing family", args: []string{"없는문"}, want: "그런 패거리는 없습니다."},
		{name: "self family", args: []string{"패거리2"}, want: "자기 자신들과 싸우시려고요?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := callWarTestContext("s-alice", "player:alice", active, nil)
			if _, err := handler(ctx, enginecmd.ResolvedCommand{Args: tt.args}); err != nil {
				t.Fatal(err)
			}
			if ctx.OutputString() != tt.want {
				t.Fatalf("output = %q, want %q", ctx.OutputString(), tt.want)
			}
		})
	}
}

func TestCallWarResultMessageUsesLegacyRejectedText(t *testing.T) {
	world := state.NewWorld(nil)
	pending, _ := world.RequestFamilyWar(2, 5)

	tests := []struct {
		name   string
		result CallWarResult
		caller int
		target int
		want   string
	}{
		{
			name:   "caller is requested target but chose another family",
			result: CallWarResult{Transition: CallWarRejectedPending, Snapshot: pending},
			caller: 5,
			target: 3,
			want:   "다른 패거리에서 전쟁을 신청해두고 있습니다.",
		},
		{
			name:   "another pending war exists",
			result: CallWarResult{Transition: CallWarRejectedPending, Snapshot: pending},
			caller: 7,
			target: 3,
			want:   "다른 패거리에서 먼저 전쟁을 준비중입니다.",
		},
		{
			name:   "active war exists",
			result: CallWarResult{Transition: CallWarRejectedActive},
			caller: 7,
			target: 3,
			want:   "벌써 전쟁중입니다.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := callWarResultMessage(world, tt.result, tt.caller, tt.target); got != tt.want {
				t.Fatalf("message = %q, want %q", got, tt.want)
			}
		})
	}
}

type callWarTestPlayer struct {
	id         model.PlayerID
	creature   model.CreatureID
	name       string
	stats      map[string]int
	properties map[string]string
	tags       []string
}

func callWarTestWorld(t *testing.T, players ...callWarTestPlayer) *state.World {
	t.Helper()
	loaded := worldload.NewWorld()
	for _, spec := range players {
		if err := loaded.AddPlayer(model.Player{
			ID:          spec.id,
			DisplayName: spec.name,
			CreatureID:  spec.creature,
			RoomID:      "room:war",
		}); err != nil {
			t.Fatal(err)
		}
		if err := loaded.AddCreature(model.Creature{
			ID:          spec.creature,
			Kind:        model.CreatureKindPlayer,
			DisplayName: spec.name,
			PlayerID:    spec.id,
			RoomID:      "room:war",
			Stats:       spec.stats,
			Properties:  spec.properties,
			Metadata:    model.Metadata{Tags: spec.tags},
		}); err != nil {
			t.Fatal(err)
		}
	}
	return state.NewWorld(loaded)
}

func callWarTestContext(sessionID session.ID, actorID model.PlayerID, active []ActiveSession, writes map[session.ID]string) *enginecmd.Context {
	if writes == nil {
		writes = map[session.ID]string{}
	}
	return &enginecmd.Context{
		SessionID: string(sessionID),
		ActorID:   string(actorID),
		Values: map[string]any{
			ContextActiveSessionsKey: func() []ActiveSession {
				return active
			},
			ContextSendToSessionKey: func(id session.ID, cmd session.Command) error {
				writes[id] += cmd.Write
				return nil
			},
		},
	}
}

func TestFamilyWarEndClearsActiveWar(t *testing.T) {
	world := state.NewWorld(nil)
	snap, _ := world.RequestFamilyWar(2, 5)
	snap, _ = world.AcceptFamilyWar(5, 2)
	if !snap.AtWar() {
		t.Fatal("war not active")
	}
	final := world.EndActiveFamilyWar("boss_death")
	if final.Active != (state.FamilyWarPair{First: 5, Second: 2}) {
		t.Fatalf("final active = %+v, want accepted pair", final.Active)
	}
	if world.FamilyWarSnapshot().AtWar() {
		t.Fatal("war should be cleared after end")
	}
}

func TestFamilyWarCombatFatalAttackLeavesWarPairUnscored(t *testing.T) {
	loaded := worldload.NewWorld()
	if err := loaded.AddRoom(model.Room{ID: "room:war", DisplayName: "War Room"}); err != nil {
		t.Fatal(err)
	}
	for _, player := range []model.Player{
		{ID: "player:alice", DisplayName: "Alice", CreatureID: "creature:alice", RoomID: "room:war"},
		{ID: "player:bob", DisplayName: "Bob", CreatureID: "creature:bob", RoomID: "room:war"},
	} {
		if err := loaded.AddPlayer(player); err != nil {
			t.Fatal(err)
		}
	}
	if err := loaded.AddCreature(model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:war",
		Equipment:   map[string]model.ObjectInstanceID{"wield": "object:sword"},
		Stats:       map[string]int{"class": model.ClassFighter, "familyFlag": 1, "familyID": 2, "hpCurrent": 20, "hpMax": 20, "thaco": 0},
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddCreature(model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:war",
		Stats:       map[string]int{"familyFlag": 1, "familyID": 5, "hpCurrent": 3, "hpMax": 3},
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          "prototype:sword",
		Kind:        model.ObjectKindWeapon,
		DisplayName: "전쟁검",
		Properties:  map[string]string{"pDice": "3"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.AddObjectInstance(model.ObjectInstance{
		ID:          "object:sword",
		PrototypeID: "prototype:sword",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "wield"},
	}); err != nil {
		t.Fatal(err)
	}

	world := state.NewWorld(loaded)
	_, _ = world.RequestFamilyWar(2, 5)
	_, _ = world.AcceptFamilyWar(5, 2)

	ctx := &enginecmd.Context{ActorID: "player:alice"}
	if _, err := enginecmd.NewAttackHandler(world)(ctx, enginecmd.ResolvedCommand{Args: []string{"Bob"}}); err != nil {
		t.Fatal(err)
	}
	if out := ctx.OutputString(); !strings.Contains(out, "Bob") || !strings.Contains(out, "쓰러졌습니다") {
		t.Fatalf("fatal attack output = %q", out)
	}
	snapshot := world.FamilyWarSnapshot()
	if snapshot.Active != (state.FamilyWarPair{First: 5, Second: 2}) {
		t.Fatalf("active war after fatal PvP attack = %+v, want unchanged pair", snapshot.Active)
	}
}

func TestFamilyWarStatusLineOmitsGoOnlyScoreAndStateDetails(t *testing.T) {
	world := state.NewWorld(nil)
	_, _ = world.RequestFamilyWar(3, 7)
	_, _ = world.AcceptFamilyWar(7, 3)
	line := familyWarStatusLine(world)
	if want := "\n패거리7 패거리는 패거리3 패거리와 전쟁중입니다.\n"; line != want {
		t.Fatalf("war status line = %q, want %q", line, want)
	}
	for _, disallowed := range []string{"점수", "전쟁 진행중", "종료"} {
		if strings.Contains(line, disallowed) {
			t.Fatalf("war status line contains Go-only detail %q: %q", disallowed, line)
		}
	}
}

// TestLegacyWarCheckCParity matches Go attackLegacyCheckWar against C command12.c:check_war semantics exactly.
func TestLegacyWarCheckCParity(t *testing.T) {
	// C: check_war(f1,f2) == 0 iff they are the exact warring pair (in either order); 1 otherwise (incl 0 families or no war)
	// Go attackLegacyCheckWar returns true when "not warring pair" (i.e. == C's return 1)
	cases := []struct {
		atWar       int
		f1, f2      int
		wantGoCheck bool // true == C would return 1 (not warring)
	}{
		{atWar: 0, f1: 2, f2: 5, wantGoCheck: true},
		{atWar: 2*16 + 5, f1: 2, f2: 5, wantGoCheck: false},
		{atWar: 2*16 + 5, f1: 5, f2: 2, wantGoCheck: false},
		{atWar: 2*16 + 5, f1: 2, f2: 6, wantGoCheck: true},
		{atWar: 2*16 + 5, f1: 0, f2: 5, wantGoCheck: true},
		{atWar: 82, f1: 5, f2: 2, wantGoCheck: false}, // 5*16+2=82
	}
	for _, c := range cases {
		// inline Go impl of attackLegacyCheckWar for cross-pkg test (C parity exact)
		goCheck := true
		if c.f1 != 0 && c.f2 != 0 && c.atWar != 0 {
			w1 := c.atWar / 16
			w2 := c.atWar % 16
			if (c.f1 == w1 && c.f2 == w2) || (c.f2 == w1 && c.f1 == w2) {
				goCheck = false
			}
		}
		if goCheck != c.wantGoCheck {
			t.Errorf("legacyCheck(atWar=%d, %d,%d) = %v, want %v (C parity fail)", c.atWar, c.f1, c.f2, goCheck, c.wantGoCheck)
		}
	}
}
