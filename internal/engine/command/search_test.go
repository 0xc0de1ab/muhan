package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestSearchHandlerFindsHiddenRoomEntitiesAndClearsActorHidden(t *testing.T) {
	loaded := lookWorld(t)
	room := loaded.Rooms["room:plaza"]
	room.Exits[0].Flags = []string{"secret"}
	loaded.Rooms[room.ID] = room

	alice := loaded.Creatures["creature:alice"]
	alice.Level = 12
	alice.Stats = map[string]int{"class": model.ClassRanger, "piety": 30, "PHIDDN": 1}
	alice.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Players[player.ID] = player

	bob := loaded.Players["player:bob"]
	bob.Metadata.Tags = []string{"hidden"}
	loaded.Players[bob.ID] = bob

	guard := loaded.Creatures["creature:guard"]
	guard.Metadata.Tags = []string{"hidden"}
	loaded.Creatures[guard.ID] = guard

	coin := loaded.Objects["object:coin"]
	coin.Metadata.Tags = []string{"hidden"}
	loaded.Objects[coin.ID] = coin

	runtime := state.NewWorld(loaded)
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "검색", Number: 24, Handler: "search"},
		}),
		Handlers: map[string]Handler{
			"search": NewSearchHandler(runtime, fixedRoll(1)),
		},
	}
	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "검색")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	out := ctx.OutputString()
	for _, want := range []string{
		"출구를 찾았습니다: 동.",
		"당신은 금화",
		"숨어있는 Bob",
		"숨어있는 경비병",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	updated, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("actor tags = %+v, want hidden cleared", updated.Metadata.Tags)
	}
	if updated.Stats["PHIDDN"] != 0 {
		t.Fatalf("actor PHIDDN = %d, want 0", updated.Stats["PHIDDN"])
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("player tags = %+v, want hidden cleared", updatedPlayer.Metadata.Tags)
	}
}

func TestSearchHandlerReportsNothingFound(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{"piety": 30}
	loaded.Creatures[alice.ID] = alice

	runtime := state.NewWorld(loaded)
	handler := NewSearchHandler(runtime, fixedRoll(1))
	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 아무것도 찾지 못했습니다.\n" {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
}

func TestSearchHandlerUsesLegacyBlindStatChanceCap(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 12
	alice.Stats = map[string]int{"class": model.ClassRanger, "piety": 30, "PBLIND": 1}
	loaded.Creatures[alice.ID] = alice
	coin := loaded.Objects["object:coin"]
	coin.Metadata.Tags = []string{"hidden"}
	loaded.Objects[coin.ID] = coin
	runtime := state.NewWorld(loaded)

	handler := NewSearchHandler(runtime, fixedRoll(21))
	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 아무것도 찾지 못했습니다.\n" {
		t.Fatalf("status/output = %d/%q, want blind chance cap miss", status, ctx.OutputString())
	}
}

func TestSearchHandlerUsesLegacyDetectInvisibleStat(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 12
	alice.Stats = map[string]int{"class": model.ClassRanger, "piety": 30, "PDINVI": 1}
	loaded.Creatures[alice.ID] = alice
	coin := loaded.Objects["object:coin"]
	coin.Metadata.Tags = []string{"hidden", "invisible"}
	loaded.Objects[coin.ID] = coin
	runtime := state.NewWorld(loaded)

	handler := NewSearchHandler(runtime, fixedRoll(1))
	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if !strings.Contains(ctx.OutputString(), "당신은 금화") {
		t.Fatalf("output missing invisible hidden object with PDINVI stat:\n%s", ctx.OutputString())
	}
}

func TestSearchHandlerUsesCreatureNameForHiddenPlayerLikeLegacy(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 12
	alice.Stats = map[string]int{"class": model.ClassRanger, "piety": 30}
	loaded.Creatures[alice.ID] = alice
	bob := loaded.Players["player:bob"]
	bob.DisplayName = ""
	bob.AccountName = "BobAccount"
	bob.CreatureID = "creature:bob"
	bob.Metadata.Tags = []string{"hidden"}
	loaded.Players[bob.ID] = bob
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "BobCreature",
		PlayerID:    "player:bob",
		RoomID:      "room:plaza",
	})
	runtime := state.NewWorld(loaded)

	handler := NewSearchHandler(runtime, fixedRoll(1))
	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "숨어있는 BobCreature") {
		t.Fatalf("status/output = %d/%q, want linked creature name", status, out)
	}
	if strings.Contains(out, "BobAccount") {
		t.Fatalf("output leaked account name instead of C creature name:\n%s", out)
	}
}

func TestSearchHandlerRoomBroadcastsAttemptAndDiscovery(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 12
	alice.Stats = map[string]int{"class": model.ClassRanger, "piety": 30}
	loaded.Creatures[alice.ID] = alice
	coin := loaded.Objects["object:coin"]
	coin.Metadata.Tags = []string{"hidden"}
	loaded.Objects[coin.ID] = coin
	runtime := state.NewWorld(loaded)

	var broadcasts []roomBroadcastRecord
	handler := NewSearchHandler(runtime, fixedRoll(1))
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if len(broadcasts) != 2 {
		t.Fatalf("broadcasts = %+v, want attempt and discovery", broadcasts)
	}
	if broadcasts[0].RoomID != "room:plaza" || broadcasts[0].Exclude != "session:alice" || broadcasts[0].Text != "\nAlice이 주변을 샅샅이 뒤져봅니다." {
		t.Fatalf("attempt broadcast = %+v", broadcasts[0])
	}
	if broadcasts[1].Text != "\n그녀가 뭘 발견한것 같군요!" {
		t.Fatalf("discovery broadcast = %+v", broadcasts[1])
	}
}

func TestHideHandlerHidesAndUnhidesSelf(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 12
	alice.Stats = map[string]int{"class": model.ClassThief, "dexterity": 40}
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden"}
	loaded.Players[player.ID] = player
	runtime := state.NewWorld(loaded)

	handler := NewHideHandler(runtime, fixedRoll(1))
	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() success error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 애써 숨어보려고 합니다.\n당신은 성공적으로 숨었습니다." {
		t.Fatalf("success status/output = %d/%q", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden") {
		t.Fatalf("actor tags = %+v, want hidden", updated.Metadata.Tags)
	}
	if updated.Stats["PHIDDN"] != 1 {
		t.Fatalf("actor PHIDDN = %d, want hidden flag set", updated.Stats["PHIDDN"])
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if !hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "hidden") {
		t.Fatalf("player tags = %+v, want hidden", updatedPlayer.Metadata.Tags)
	}
	if len(broadcasts) != 1 || !strings.Contains(broadcasts[0].Text, "그림자 사이로 숨었습니다") {
		t.Fatalf("success broadcasts = %+v", broadcasts)
	}

	loaded = lookWorld(t)
	alice = loaded.Creatures["creature:alice"]
	alice.Level = 12
	alice.Stats = map[string]int{"class": model.ClassThief, "dexterity": 40, "PHIDDN": 1}
	alice.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Creatures[alice.ID] = alice
	player = loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Players[player.ID] = player
	runtime = state.NewWorld(loaded)
	handler = NewHideHandler(runtime, fixedRoll(100))
	broadcasts = nil
	ctx = contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err = handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() failure error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("failure status = %d, want StatusDefault", status)
	}
	if ctx.OutputString() != "당신은 애써 숨어보려고 합니다." {
		t.Fatalf("failure output = %q", ctx.OutputString())
	}
	updated, _ = runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("actor tags = %+v, want hidden cleared after failed hide", updated.Metadata.Tags)
	}
	if updated.Stats["PHIDDN"] != 0 {
		t.Fatalf("actor PHIDDN = %d, want 0 after failed hide", updated.Stats["PHIDDN"])
	}
	updatedPlayer, _ = runtime.Player("player:alice")
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("player tags = %+v, want hidden cleared after failed hide", updatedPlayer.Metadata.Tags)
	}
	if len(broadcasts) != 1 || !strings.Contains(broadcasts[0].Text, "애써 숨어보려고 합니다") {
		t.Fatalf("failure broadcasts = %+v", broadcasts)
	}
}

func TestSearchAndHideHandlersApplyLegacyCooldowns(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 12
	alice.Stats = map[string]int{"class": model.ClassRanger, "piety": 30, "dexterity": 40}
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)

	search := NewSearchHandler(runtime, fixedRoll(1))
	ctx := &Context{ActorID: "player:alice"}
	if _, err := search(ctx, ResolvedCommand{}); err != nil {
		t.Fatalf("search first error = %v", err)
	}
	ctx = &Context{ActorID: "player:alice"}
	if _, err := search(ctx, ResolvedCommand{}); err != nil {
		t.Fatalf("search second error = %v", err)
	}
	if !strings.Contains(ctx.OutputString(), "기다리세요.") {
		t.Fatalf("search second output = %q, want wait message", ctx.OutputString())
	}

	hide := NewHideHandler(runtime, fixedRoll(1))
	ctx = &Context{ActorID: "player:alice"}
	if _, err := hide(ctx, ResolvedCommand{}); err != nil {
		t.Fatalf("hide first error = %v", err)
	}
	ctx = &Context{ActorID: "player:alice"}
	if _, err := hide(ctx, ResolvedCommand{}); err != nil {
		t.Fatalf("hide second error = %v", err)
	}
	if !strings.Contains(ctx.OutputString(), "기다리세요.") {
		t.Fatalf("hide second output = %q, want wait message", ctx.OutputString())
	}
}

func TestHideHandlerHidesRoomObject(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 12
	alice.Stats = map[string]int{"class": model.ClassThief, "dexterity": 40}
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)

	handler := NewHideHandler(runtime, fixedRoll(1))
	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := handler(ctx, ResolvedCommand{Args: []string{"금"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "당신은 성공적으로 숨겼습니다.") {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	coin, _ := runtime.Object("object:coin")
	if !hasAnyNormalizedFlag(coin.Metadata.Tags, "hidden") {
		t.Fatalf("coin tags = %+v, want hidden", coin.Metadata.Tags)
	}
	if len(broadcasts) != 2 || broadcasts[0].Text != "\nAlice가 금화를 숨겨보려고 합니다." ||
		broadcasts[1].Text != "\nAlice가 금화를 어딘가 숨깁니다." {
		t.Fatalf("object broadcasts = %+v", broadcasts)
	}
	if out := dispatchLookLineWithContext(t, loaded, "봐", &Context{ActorID: "player:alice"}); !strings.Contains(out, "금화") {
		t.Fatalf("fixture look changed unexpectedly:\n%s", out)
	}
	out, err := RenderCurrentRoom(runtime, LookViewer{PlayerID: "player:alice"})
	if err != nil {
		t.Fatalf("RenderCurrentRoom() error = %v", err)
	}
	if strings.Contains(out, "금화") {
		t.Fatalf("hidden object still rendered:\n%s", out)
	}
}

func TestHideHandlerUsesOnlyFirstArgumentLikeLegacy(t *testing.T) {
	loaded := lookWorld(t)
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 12
	alice.Stats = map[string]int{"class": model.ClassThief, "dexterity": 40}
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)

	handler := NewHideHandler(runtime, fixedRoll(1))
	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"금", "무시"}, Values: []int64{1, 1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "당신은 성공적으로 숨겼습니다.") {
		t.Fatalf("status/output = %d/%q, want first-argument hide success", status, ctx.OutputString())
	}
	coin, _ := runtime.Object("object:coin")
	if !hasAnyNormalizedFlag(coin.Metadata.Tags, "hidden") {
		t.Fatalf("coin tags = %+v, want hidden", coin.Metadata.Tags)
	}
}

func TestHideHandlerUsesLegacyFindObjMatchingAndVisibility(t *testing.T) {
	loaded := lookWorld(t)
	runtime := state.NewWorld(loaded)
	handler := NewHideHandler(runtime, fixedRoll(1))
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{Args: []string{"object:coin"}})
	if err != nil {
		t.Fatalf("handler() object ID error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그런것은 여기 없어요." {
		t.Fatalf("object ID status/output = %d/%q, want missing", status, ctx.OutputString())
	}
	coin, _ := runtime.Object("object:coin")
	if hasAnyNormalizedFlag(coin.Metadata.Tags, "hidden") {
		t.Fatalf("coin tags = %+v, want not hidden by object ID target", coin.Metadata.Tags)
	}

	loaded = lookWorld(t)
	coin = loaded.Objects["object:coin"]
	coin.Metadata.Tags = []string{"OINVIS"}
	loaded.Objects[coin.ID] = coin
	runtime = state.NewWorld(loaded)
	handler = NewHideHandler(runtime, fixedRoll(1))
	ctx = &Context{ActorID: "player:alice"}

	status, err = handler(ctx, ResolvedCommand{Args: []string{"금"}})
	if err != nil {
		t.Fatalf("handler() invisible error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그런것은 여기 없어요." {
		t.Fatalf("invisible status/output = %d/%q, want missing", status, ctx.OutputString())
	}

	loaded = lookWorld(t)
	coin = loaded.Objects["object:coin"]
	coin.Metadata.Tags = []string{"OINVIS"}
	loaded.Objects[coin.ID] = coin
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 12
	alice.Stats = map[string]int{"class": model.ClassThief, "dexterity": 40}
	alice.Metadata.Tags = []string{"PDINVI"}
	loaded.Creatures[alice.ID] = alice
	runtime = state.NewWorld(loaded)
	handler = NewHideHandler(runtime, fixedRoll(1))
	ctx = &Context{ActorID: "player:alice"}

	status, err = handler(ctx, ResolvedCommand{Args: []string{"금"}})
	if err != nil {
		t.Fatalf("handler() detect invisible error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "당신은 성공적으로 숨겼습니다.") {
		t.Fatalf("detect invisible status/output = %d/%q, want hidden", status, ctx.OutputString())
	}
	coin, _ = runtime.Object("object:coin")
	if !hasAnyNormalizedFlag(coin.Metadata.Tags, "hidden") {
		t.Fatalf("coin tags = %+v, want hidden", coin.Metadata.Tags)
	}
}

func TestHideHandlerRejectsNoTakeObject(t *testing.T) {
	loaded := lookWorld(t)
	coin := loaded.Objects["object:coin"]
	coin.Metadata.Tags = []string{"noTake"}
	loaded.Objects[coin.ID] = coin
	runtime := state.NewWorld(loaded)

	handler := NewHideHandler(runtime, fixedRoll(1))
	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{Args: []string{"금"}, Values: []int64{1}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "당신은 그것을 숨길 수 없습니다." {
		t.Fatalf("status/output = %d/%q", status, ctx.OutputString())
	}
	updated, _ := runtime.Object("object:coin")
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden") {
		t.Fatalf("object tags = %+v, want not hidden", updated.Metadata.Tags)
	}
}

func fixedRoll(value int) SearchRollFunc {
	return func(int, int) int {
		return value
	}
}

type roomBroadcastRecord struct {
	RoomID  string
	Exclude string
	Text    string
}

func contextWithRoomBroadcast(actorID string, sessionID string, records *[]roomBroadcastRecord) *Context {
	return &Context{
		ActorID:   actorID,
		SessionID: sessionID,
		Values: map[string]any{
			ContextRoomBroadcastKey: RoomBroadcastFunc(func(roomID model.RoomID, excludeSessionID string, text string) error {
				*records = append(*records, roomBroadcastRecord{
					RoomID:  string(roomID),
					Exclude: excludeSessionID,
					Text:    text,
				})
				return nil
			}),
		},
	}
}
