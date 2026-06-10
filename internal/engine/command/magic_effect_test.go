package command

import (
	"strings"
	"testing"
	"time"

	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestDefaultReadScrollMagicEffectCurePoison(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "4")
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"poisoned", "ppoisn", "hidden"}
	loaded.Creatures[creature.ID] = creature
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"poison"}
	loaded.Players[player.ID] = player
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after curepoison scroll")
	}
	if want := "당신은 오른손으로 혈도를 짚으면서 해독 주문을 외웁니다.\n손가락 끝으로 검은 독기운이 빠져나오는것이 보입니다.\n당신 몸에 남아 있는 독이 모두 빠져나갔습니다.\n"; !strings.Contains(ctx.OutputString(), want) {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), want)
	}
	updatedCreature, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updatedCreature.Metadata.Tags, "poison", "poisoned", "ppoisn") {
		t.Fatalf("creature tags = %+v, want poison removed", updatedCreature.Metadata.Tags)
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "poison", "poisoned", "ppoisn") {
		t.Fatalf("player tags = %+v, want poison removed", updatedPlayer.Metadata.Tags)
	}
}

func TestMagicObjectWeightlessReadsFlagsContainerLikeLegacy(t *testing.T) {
	loaded := lookWorld(t)
	protoID := model.PrototypeID("prototype:weightless-flags")
	if err := loaded.AddObjectPrototype(model.ObjectPrototype{
		ID:          protoID,
		DisplayName: "깃털 자루",
		Properties:  map[string]string{"flags": "weightless"},
	}); err != nil {
		t.Fatal(err)
	}
	world := state.NewWorld(loaded)

	if !magicObjectWeightless(world, model.ObjectInstance{PrototypeID: protoID}) {
		t.Fatal("magicObjectWeightless did not match prototype flags container as OWTLES")
	}
	if !magicObjectWeightless(world, model.ObjectInstance{Properties: map[string]string{"flags": "OWTLES"}}) {
		t.Fatal("magicObjectWeightless did not match object flags container as OWTLES")
	}
}

func TestDefaultDrinkMagicEffectCurePoison(t *testing.T) {
	loaded := drinkWorld(t, "room:tavern", "2", "4")
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"poisoned", "hidden"}
	loaded.Creatures[creature.ID] = creature
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"ppoisn"}
	loaded.Players[player.ID] = player
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewDrinkHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"치료"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if want := "\n독기운이 중화되는 것을 느낄 수 있습니다.\n"; !strings.Contains(ctx.OutputString(), want) {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), want)
	}
	updatedCreature, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updatedCreature.Metadata.Tags, "poison", "poisoned", "ppoisn") {
		t.Fatalf("creature tags = %+v, want poison removed", updatedCreature.Metadata.Tags)
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "poison", "poisoned", "ppoisn") {
		t.Fatalf("player tags = %+v, want poison removed", updatedPlayer.Metadata.Tags)
	}
	potion, _ := runtime.Object("object:potion")
	if got := potion.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("potion shotsCurrent = %q, want 1", got)
	}
}

func TestDefaultReadScrollMagicEffectCurePoisonTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "4")
	actor := loaded.Creatures["creature:alice"]
	actor.Metadata.Tags = []string{"poisoned"}
	loaded.Creatures[actor.ID] = actor
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Metadata:    model.Metadata{Tags: []string{"ppoisn"}},
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted curepoison scroll")
	}
	if !strings.Contains(ctx.OutputString(), "상인이 혈도를 짚으면서 해독 주문을 외웁니다.\n그의 손가락 끝으로 검은 독기운이 빠져나오는 것이 보입니다.\n") {
		t.Fatalf("output = %q, want targeted curepoison text", ctx.OutputString())
	}
	updatedTarget, _ := runtime.Creature("creature:merchant")
	if hasAnyNormalizedFlag(updatedTarget.Metadata.Tags, "poison", "poisoned", "ppoisn") {
		t.Fatalf("target tags = %+v, want poison removed", updatedTarget.Metadata.Tags)
	}
	updatedActor, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updatedActor.Metadata.Tags, "poison", "poisoned", "ppoisn") {
		t.Fatalf("actor tags = %+v, want actor poison unchanged", updatedActor.Metadata.Tags)
	}
}

func TestDefaultZapMagicEffectDetectInvisible(t *testing.T) {
	runtime := state.NewWorld(zapWorld(t, "room:plaza", "2", "10"))

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewZapHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"마법봉"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if !strings.Contains(ctx.OutputString(), "번쩍") {
		t.Fatalf("output = %q, want wand use output", ctx.OutputString())
	}
	creature, _ := runtime.Creature("creature:alice")
	if !viewerHasDetectInvisibleTag(creature) {
		t.Fatalf("creature tags = %+v, want detectInvisible", creature.Metadata.Tags)
	}
	wand, _ := runtime.Object("object:wand")
	if got := wand.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("shotsCurrent = %q, want 1", got)
	}
}

func TestDefaultZapMagicEffectLight(t *testing.T) {
	useSpellFailRoll(t, 0)
	withFakeMagicEffectTime(t, 1000)
	runtime := state.NewWorld(zapWorld(t, "room:plaza", "2", "3"))

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := NewZapHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"마법봉"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	wantOutput := "당신의 왼손에 발광 주문을 걸었습니다.\n왼손에서 황금빛이 뿜어져 나와 주위를 밝혀 줍니다.\n번쩍\n"
	if ctx.OutputString() != wantOutput {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), wantOutput)
	}
	wantBroadcast := roomBroadcastRecord{
		RoomID:  "room:plaza",
		Exclude: "session:alice",
		Text:    "\nAlice가 한쪽 손에 발광 주문을 걸었습니다.\n그의 손에서 황금색의 찬란한 빛이 뿜어져 나옵니다.\n",
	}
	if len(broadcasts) != 1 || broadcasts[0] != wantBroadcast {
		t.Fatalf("broadcasts = %+v, want [%+v]", broadcasts, wantBroadcast)
	}
	creature, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "PLIGHT", "light") {
		t.Fatalf("creature tags = %+v, want light", creature.Metadata.Tags)
	}
	if expires, ok := runtime.GetEffectExpiration("creature:alice", "PLIGHT"); !ok || expires != 1600 {
		t.Fatalf("PLIGHT expiration = %d/%v, want 1600/true", expires, ok)
	}
	wand, _ := runtime.Object("object:wand")
	if got := wand.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("shotsCurrent = %q, want 1", got)
	}
}

func TestDefaultReadScrollMagicEffectLight(t *testing.T) {
	useSpellFailRoll(t, 0)
	withFakeMagicEffectTime(t, 2000)
	runtime := state.NewWorld(readScrollWorld(t, "room:library", "1", "3"))

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	wantOutput := "당신의 왼손에 발광 주문을 걸었습니다.\n왼손에서 황금빛이 뿜어져 나와 주위를 밝혀 줍니다.\n주문이 번쩍인다.\n\n모든 것을 읽고 나자 귀환 주문서의 형체가 먼지로 변하면서 바람과 함께 사라져 버렸습니다.\n"
	if ctx.OutputString() != wantOutput {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), wantOutput)
	}
	if len(broadcasts) != 1 || !strings.Contains(broadcasts[0].Text, "한쪽 손에 발광 주문을 걸었습니다") {
		t.Fatalf("broadcasts = %+v, want light broadcast", broadcasts)
	}
	creature, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "PLIGHT", "light") {
		t.Fatalf("creature tags = %+v, want light", creature.Metadata.Tags)
	}
	if expires, ok := runtime.GetEffectExpiration("creature:alice", "PLIGHT"); !ok || expires != 2600 {
		t.Fatalf("PLIGHT expiration = %d/%v, want 2600/true", expires, ok)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after light scroll")
	}
}

func TestDefaultDrinkMagicEffectLight(t *testing.T) {
	useSpellFailRoll(t, 0)
	withFakeMagicEffectTime(t, 3000)
	runtime := state.NewWorld(drinkWorld(t, "room:tavern", "2", "3"))

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := NewDrinkHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"치료"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	wantOutput := "당신의 왼손에 발광 주문을 걸었습니다.\n왼손에서 황금빛이 뿜어져 나와 주위를 밝혀 줍니다.\n몸이 따뜻해진다.\n\n당신은 치료약을 먹었습니다.\n"
	if ctx.OutputString() != wantOutput {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), wantOutput)
	}
	if len(broadcasts) != 2 ||
		!strings.Contains(broadcasts[0].Text, "한쪽 손에 발광 주문을 걸었습니다") ||
		broadcasts[1].Text != "\nAlice가 치료약을 먹었습니다." {
		t.Fatalf("broadcasts = %+v, want light then potion-consumption broadcasts", broadcasts)
	}
	creature, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "PLIGHT", "light") {
		t.Fatalf("creature tags = %+v, want light", creature.Metadata.Tags)
	}
	if expires, ok := runtime.GetEffectExpiration("creature:alice", "PLIGHT"); !ok || expires != 3600 {
		t.Fatalf("PLIGHT expiration = %d/%v, want 3600/true", expires, ok)
	}
	potion, _ := runtime.Object("object:potion")
	if got := potion.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("potion shotsCurrent = %q, want 1", got)
	}
}

func TestDefaultDrinkMagicEffectDetectMagic(t *testing.T) {
	runtime := state.NewWorld(drinkWorld(t, "room:tavern", "2", "11"))

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewDrinkHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"치료"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	creature, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "detectMagic", "pdmagi") {
		t.Fatalf("creature tags = %+v, want detectMagic", creature.Metadata.Tags)
	}
	player, _ := runtime.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "detectMagic", "pdmagi") {
		t.Fatalf("player tags = %+v, want detectMagic", player.Metadata.Tags)
	}
	potion, _ := runtime.Object("object:potion")
	if got := potion.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("potion shotsCurrent = %q, want 1", got)
	}
}

func TestDefaultDrinkMagicEffectRemoveBlindness(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := drinkWorld(t, "room:tavern", "2", "50")
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"PBLIND", "hidden"}
	loaded.Creatures[creature.ID] = creature
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"blind"}
	loaded.Players[player.ID] = player
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewDrinkHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"치료"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if !strings.Contains(ctx.OutputString(), "감겼던 눈이 움찔거리다가 갑자기 눈앞이 밝아집니다.") {
		t.Fatalf("output = %q, want blind potion text", ctx.OutputString())
	}
	updatedCreature, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updatedCreature.Metadata.Tags, "blind", "pblind") {
		t.Fatalf("creature tags = %+v, want blind removed", updatedCreature.Metadata.Tags)
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "blind", "pblind") {
		t.Fatalf("player tags = %+v, want blind removed", updatedPlayer.Metadata.Tags)
	}
	potion, _ := runtime.Object("object:potion")
	if got := potion.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("potion shotsCurrent = %q, want 1", got)
	}
}

func TestDefaultDrinkMagicEffectRemoveBlindnessNotBlindText(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := drinkWorld(t, "room:tavern", "2", "50")
	runtime := state.NewWorld(loaded)
	creature := loaded.Creatures["creature:alice"]
	potion := loaded.Objects["object:potion"]

	ctx := &Context{ActorID: "player:alice"}
	success, err := defaultDrinkMagicEffect(ctx, runtime, creature, potion, ResolvedCommand{})
	if err != nil {
		t.Fatalf("defaultDrinkMagicEffect error = %v", err)
	}
	if !success {
		t.Fatalf("defaultDrinkMagicEffect success = false")
	}
	want := "약을 먹자 당신 눈에 걸린 주술이 스르르 풀리는 것을 느낍니다."
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestMagicEffectRemoveBlindnessPotionRejectsTarget(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := readScrollWorld(t, "room:library", "1", "50")
	alice := loaded.Creatures["creature:alice"]
	loaded.Creatures[alice.ID] = alice
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	loaded.Creatures["creature:bob"] = model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:library",
		Metadata:    model.Metadata{Tags: []string{"PBLIND"}},
	}
	runtime := state.NewWorld(loaded)
	potion := model.ObjectInstance{ID: "object:blind-potion", Properties: map[string]string{"type": "6"}}
	ctx := &Context{ActorID: "player:alice"}

	success, err := magicEffectRemoveBlindness(ctx, runtime, alice, potion, ResolvedCommand{Args: []string{"주문", "Bob"}})
	if err != nil {
		t.Fatalf("magicEffectRemoveBlindness error = %v", err)
	}
	if success {
		t.Fatalf("success = true, want false")
	}
	if got, want := ctx.OutputString(), "이 물건은 자신에게만 사용할수 있습니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	updatedBob, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(updatedBob.Metadata.Tags, "PBLIND") {
		t.Fatalf("Bob blind tag removed on rejected potion: %+v", updatedBob.Metadata.Tags)
	}
}

func TestMagicEffectRemoveBlindnessPotionRejectsMissingTargetBeforeLookup(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := readScrollWorld(t, "room:library", "1", "50")
	alice := loaded.Creatures["creature:alice"]
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)
	potion := model.ObjectInstance{ID: "object:blind-potion", Properties: map[string]string{"type": "6"}}
	ctx := &Context{ActorID: "player:alice"}

	success, err := magicEffectRemoveBlindness(ctx, runtime, alice, potion, ResolvedCommand{Args: []string{"주문", "Nobody"}})
	if err != nil {
		t.Fatalf("magicEffectRemoveBlindness error = %v", err)
	}
	if success {
		t.Fatalf("success = true, want false")
	}
	if got, want := ctx.OutputString(), "이 물건은 자신에게만 사용할수 있습니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDefaultReadScrollMagicEffectRemoveBlindnessTarget(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := readScrollWorld(t, "room:library", "1", "50")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Metadata:    model.Metadata{Tags: []string{"pblind"}},
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted remove blindness scroll")
	}
	if !strings.Contains(ctx.OutputString(), "당신은 상인의 이마에 개안부를 붙히고서 주문을") {
		t.Fatalf("output = %q, want targeted remove blindness text", ctx.OutputString())
	}
	updatedTarget, _ := runtime.Creature("creature:merchant")
	if hasAnyNormalizedFlag(updatedTarget.Metadata.Tags, "blind", "pblind") {
		t.Fatalf("target tags = %+v, want blind removed", updatedTarget.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectRemoveBlindnessExplicitSelfAliasMissesLikeLegacy(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := readScrollWorld(t, "room:library", "1", "50")
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "나"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그런 사람이 존재하지 않습니다." {
		t.Fatalf("status/output = %d/%q, want C target-branch miss", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after explicit self alias miss")
	}
}

func TestDefaultReadScrollMagicEffectRemoveBlindnessNamedSelfUsesTargetBranchLikeLegacy(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := readScrollWorld(t, "room:library", "1", "50")
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Alice"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if !strings.Contains(ctx.OutputString(), "당신은 Alice의 이마에 개안부를 붙히고서 주문을") {
		t.Fatalf("output = %q, want target-branch remove blindness text", ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after named-self target branch success")
	}
}

func TestDefaultReadScrollMagicEffectRecall(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "17")
	mustAddLookRoom(t, loaded, model.Room{
		ID:          magicRecallSelfRoomID,
		DisplayName: "통계 무한 광장",
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after recall scroll")
	}
	wantOutput := "귀환 주문을 외웠습니다.\n주문이 번쩍인다.\n\n모든 것을 읽고 나자 귀환 주문서의 형체가 먼지로 변하면서 바람과 함께 사라져 버렸습니다.\n"
	if got := ctx.OutputString(); got != wantOutput {
		t.Fatalf("output = %q, want %q", got, wantOutput)
	}
	player, _ := runtime.Player("player:alice")
	creature, _ := runtime.Creature("creature:alice")
	if player.RoomID != magicRecallSelfRoomID || creature.RoomID != magicRecallSelfRoomID {
		t.Fatalf("actor room = player %q creature %q, want %q", player.RoomID, creature.RoomID, magicRecallSelfRoomID)
	}
}

func TestDefaultReadScrollMagicEffectRecallTargetPlayer(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "17")
	mustAddLookRoom(t, loaded, model.Room{
		ID:          defaultReturnRoomID,
		DisplayName: "생명의 나무",
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	sent := map[string]string{}
	ctx := &Context{
		ActorID:   "player:alice",
		SessionID: "session:alice",
		Values: map[string]any{
			"game.activeSessions": func() []activeSession {
				return []activeSession{
					{ID: "session:alice", ActorID: "player:alice"},
					{ID: "session:bob", ActorID: "player:bob"},
				}
			},
			"game.sendToSession": func(id string, cmd struct{ Write string }) error {
				sent[id] += cmd.Write
				return nil
			},
		},
	}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	wantOutput := "귀환 주문을 Bob에게 외웠습니다.\n주문이 번쩍인다.\n\n모든 것을 읽고 나자 귀환 주문서의 형체가 먼지로 변하면서 바람과 함께 사라져 버렸습니다.\n"
	if got := ctx.OutputString(); got != wantOutput {
		t.Fatalf("output = %q, want %q", got, wantOutput)
	}
	if got := sent["session:bob"]; got != "Alice가 당신에게 귀환 주문을 외웠습니다.\n" {
		t.Fatalf("bob message = %q, want C target message", got)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted recall scroll")
	}
	bobPlayer, _ := runtime.Player("player:bob")
	bobCreature, _ := runtime.Creature("creature:bob")
	if bobPlayer.RoomID != defaultReturnRoomID || bobCreature.RoomID != defaultReturnRoomID {
		t.Fatalf("bob room = player %q creature %q, want %q", bobPlayer.RoomID, bobCreature.RoomID, defaultReturnRoomID)
	}
	alice, _ := runtime.Player("player:alice")
	if alice.RoomID != "room:library" {
		t.Fatalf("alice room = %q, want original room", alice.RoomID)
	}
}

func TestDefaultReadScrollMagicEffectRecallRejectsPlayerOutsideRoom(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "17")
	mustAddLookRoom(t, loaded, model.Room{ID: defaultReturnRoomID, DisplayName: "생명의 나무"})
	mustAddLookRoom(t, loaded, model.Room{ID: "room:elsewhere", DisplayName: "먼 방"})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:elsewhere",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:elsewhere",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람이 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C same-room miss", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after out-of-room recall target")
	}
	bob, _ := runtime.Player("player:bob")
	if bob.RoomID != "room:elsewhere" {
		t.Fatalf("bob room = %q, want unchanged room:elsewhere", bob.RoomID)
	}
}

func TestDefaultReadScrollMagicEffectRecallMatchesPlayerCreatureName(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "17")
	mustAddLookRoom(t, loaded, model.Room{ID: defaultReturnRoomID, DisplayName: "생명의 나무"})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "AccountBob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Shadow",
		PlayerID:    "player:bob",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Shadow"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); !strings.Contains(got, "귀환 주문을 Shadow에게 외웠습니다.") {
		t.Fatalf("output = %q, want creature-name recall target", got)
	}
	bob, _ := runtime.Player("player:bob")
	if bob.RoomID != defaultReturnRoomID {
		t.Fatalf("bob room = %q, want %q", bob.RoomID, defaultReturnRoomID)
	}
}

func TestDefaultReadScrollMagicEffectRecallNamedSelfUsesTargetBranch(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "17")
	mustAddLookRoom(t, loaded, model.Room{ID: defaultReturnRoomID, DisplayName: "생명의 나무"})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Alice"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	wantOutput := "귀환 주문을 Alice에게 외웠습니다.\n주문이 번쩍인다.\n\n모든 것을 읽고 나자 귀환 주문서의 형체가 먼지로 변하면서 바람과 함께 사라져 버렸습니다.\n"
	if got := ctx.OutputString(); got != wantOutput {
		t.Fatalf("output = %q, want %q", got, wantOutput)
	}
	alice, _ := runtime.Player("player:alice")
	if alice.RoomID != defaultReturnRoomID {
		t.Fatalf("alice room = %q, want %q", alice.RoomID, defaultReturnRoomID)
	}
}

func TestDefaultReadScrollMagicEffectRecallRejectsMonsterTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "17")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람이 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after invalid recall target")
	}
}

func TestDefaultDrinkMagicEffectRecallSelf(t *testing.T) {
	loaded := drinkWorld(t, "room:tavern", "2", "17")
	mustAddLookRoom(t, loaded, model.Room{
		ID:          magicRecallSelfRoomID,
		DisplayName: "통계 무한 광장",
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewDrinkHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"치료"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	wantOutput := "당신의 모습이 어지러이 흔들립니다.\n몸이 따뜻해진다.\n\n당신은 치료약을 먹었습니다.\n"
	if got := ctx.OutputString(); got != wantOutput {
		t.Fatalf("output = %q, want %q", got, wantOutput)
	}
	player, _ := runtime.Player("player:alice")
	creature, _ := runtime.Creature("creature:alice")
	if player.RoomID != magicRecallSelfRoomID || creature.RoomID != magicRecallSelfRoomID {
		t.Fatalf("actor room = player %q creature %q, want %q", player.RoomID, creature.RoomID, magicRecallSelfRoomID)
	}
	potion, _ := runtime.Object("object:potion")
	if got := potion.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("potion shotsCurrent = %q, want 1", got)
	}
}

func TestDefaultDrinkMagicEffectVigorRestoresPartialHP(t *testing.T) {
	useMaxMagicEffectRoll(t)
	loaded := drinkWorld(t, "room:tavern", "2", "1")
	creature := loaded.Creatures["creature:alice"]
	creature.Stats = map[string]int{"hpCurrent": 23, "hpMax": 30}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewDrinkHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"치료"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	updatedCreature, _ := runtime.Creature("creature:alice")
	if got := updatedCreature.Stats["hpCurrent"]; got != 30 {
		t.Fatalf("hpCurrent = %d, want capped 30", got)
	}
	potion, _ := runtime.Object("object:potion")
	if got := potion.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("potion shotsCurrent = %q, want 1", got)
	}
}

func TestMagicEffectVigorPotionRejectsMissingTargetBeforeLookup(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "1")
	alice := loaded.Creatures["creature:alice"]
	runtime := state.NewWorld(loaded)
	potion := model.ObjectInstance{ID: "object:vigor-potion", Properties: map[string]string{"type": "6"}}
	ctx := &Context{ActorID: "player:alice"}

	success, err := magicEffectVigor(ctx, runtime, alice, potion, ResolvedCommand{Args: []string{"회복", "Nobody"}})
	if err != nil {
		t.Fatalf("magicEffectVigor error = %v", err)
	}
	if success {
		t.Fatalf("success = true, want false")
	}
	if got, want := ctx.OutputString(), "그 물건은 자신에게만 사용할 수 있습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDefaultReadScrollMagicEffectVigorTarget(t *testing.T) {
	useMaxMagicEffectRoll(t)
	loaded := readScrollWorld(t, "room:library", "1", "1")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 3, "hpMax": 20},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted vigor scroll")
	}
	updatedTarget, _ := runtime.Creature("creature:merchant")
	if got := updatedTarget.Stats["hpCurrent"]; got != 13 {
		t.Fatalf("target hpCurrent = %d, want 13", got)
	}
}

func TestDefaultReadScrollMagicEffectFullHealTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "20")
	alice := loaded.Creatures["creature:alice"]
	alice.Properties = map[string]string{"dailyFullHealMax": "10", "dailyFullHealCur": "10"}
	loaded.Creatures[alice.ID] = alice
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 4, "hpMax": 20},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted full heal scroll")
	}
	updatedTarget, _ := runtime.Creature("creature:merchant")
	if got := updatedTarget.Stats["hpCurrent"]; got != 20 {
		t.Fatalf("target hpCurrent = %d, want 20", got)
	}
	updatedAlice, _ := runtime.Creature("creature:alice")
	if got := updatedAlice.Properties["dailyFullHealCur"]; got != "9" {
		t.Fatalf("dailyFullHealCur = %q, want C scroll-side dec_daily to 9", got)
	}
}

func TestMagicEffectFullHealPotionRejectsMissingTargetBeforeLookup(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "20")
	alice := loaded.Creatures["creature:alice"]
	runtime := state.NewWorld(loaded)
	potion := model.ObjectInstance{ID: "object:full-heal-potion", Properties: map[string]string{"type": "6"}}
	ctx := &Context{ActorID: "player:alice"}

	success, err := magicEffectHeal(ctx, runtime, alice, potion, ResolvedCommand{Args: []string{"완치", "Nobody"}})
	if err != nil {
		t.Fatalf("magicEffectHeal error = %v", err)
	}
	if success {
		t.Fatalf("success = true, want false")
	}
	if got, want := ctx.OutputString(), "그 물건은 자신에게만 사용가능합니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDefaultReadScrollMagicEffectDamageTargets(t *testing.T) {
	useMaxMagicEffectRoll(t)

	tests := []struct {
		name       string
		magicPower string
		damage     int
	}{
		{name: "hurt", magicPower: "2", damage: 8},
		{name: "fireball", magicPower: "7", damage: 18},
		{name: "shockbolt", magicPower: "26", damage: 23},
		{name: "dust gust", magicPower: "30", damage: 17},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", tt.magicPower)
			mustAddLookCreature(t, loaded, model.Creature{
				ID:          "creature:merchant",
				Kind:        model.CreatureKindNPC,
				DisplayName: "상인",
				RoomID:      "room:library",
				Stats:       map[string]int{"hpCurrent": 40, "hpMax": 40},
			})
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}
			if _, ok := runtime.Object("object:scroll"); ok {
				t.Fatal("scroll still exists after damage scroll")
			}
			merchant, _ := runtime.Creature("creature:merchant")
			if got := merchant.Stats["hpCurrent"]; got != 40-tt.damage {
				t.Fatalf("merchant hpCurrent = %d, want %d", got, 40-tt.damage)
			}
		})
	}
}

func TestMagicEffectOffensivePotionsRejectMissingTargetBeforeLookup(t *testing.T) {
	tests := []struct {
		name  string
		apply func(*Context, StatusWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error)
	}{
		{
			name: "basic offensive",
			apply: func(ctx *Context, world StatusWorld, actor model.Creature, object model.ObjectInstance, resolved ResolvedCommand) (bool, error) {
				return magicEffectApplyBasicOffensiveDamage(ctx, world, actor, object, resolved, magicPowerFireball)
			},
		},
		{
			name: "damage dice",
			apply: func(ctx *Context, world StatusWorld, actor model.Creature, object model.ObjectInstance, resolved ResolvedCommand) (bool, error) {
				return applyMagicPowerEffect(ctx, world, actor, object, resolved, magicPowerBurn, true)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, target := range []string{"Nobody", "나"} {
				t.Run(target, func(t *testing.T) {
					loaded := readScrollWorld(t, "room:library", "1", "7")
					alice := loaded.Creatures["creature:alice"]
					runtime := state.NewWorld(loaded)
					potion := model.ObjectInstance{ID: "object:offensive-potion", Properties: map[string]string{"type": "6"}}
					ctx := &Context{ActorID: "player:alice"}

					success, err := tt.apply(ctx, runtime, alice, potion, ResolvedCommand{Args: []string{"주문", target}})
					if err != nil {
						t.Fatalf("effect error = %v", err)
					}
					if success {
						t.Fatalf("success = true, want false")
					}
					if got, want := ctx.OutputString(), "\n당신에게만 사용할수 있습니다.\n"; got != want {
						t.Fatalf("output = %q, want %q", got, want)
					}
				})
			}
		})
	}
}

func TestMagicEffectDamageDiceMissingTargetUsesLegacyMessage(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "28")
	alice := loaded.Creatures["creature:alice"]
	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}

	success, err := applyMagicPowerEffect(ctx, runtime, alice, model.ObjectInstance{}, ResolvedCommand{Args: []string{"화염주", "Nobody"}}, magicPowerBurn, true)
	if err != nil {
		t.Fatalf("applyMagicPowerEffect error = %v", err)
	}
	if success {
		t.Fatalf("success = true, want false")
	}
	if got, want := ctx.OutputString(), "\n그런 것은 여기에 존재하지 않습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDefaultZapMagicEffectDamageTarget(t *testing.T) {
	useMaxMagicEffectRoll(t)

	loaded := zapWorld(t, "room:plaza", "2", "31")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:plaza",
		Stats:       map[string]int{"hpCurrent": 30, "hpMax": 30},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewZapHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"마법봉", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if !strings.Contains(ctx.OutputString(), "번쩍") {
		t.Fatalf("output = %q, want wand use output", ctx.OutputString())
	}
	merchant, _ := runtime.Creature("creature:merchant")
	if got := merchant.Stats["hpCurrent"]; got != 12 {
		t.Fatalf("merchant hpCurrent = %d, want 12", got)
	}
	wand, _ := runtime.Object("object:wand")
	if got := wand.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("shotsCurrent = %q, want 1", got)
	}
}

func TestDefaultDrinkMagicEffectDamageSelf(t *testing.T) {
	useMaxMagicEffectRoll(t)

	loaded := drinkWorld(t, "room:tavern", "2", "28")
	creature := loaded.Creatures["creature:alice"]
	creature.Stats = map[string]int{"hpCurrent": 12, "hpMax": 12}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewDrinkHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"치료"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	updatedCreature, _ := runtime.Creature("creature:alice")
	if got := updatedCreature.Stats["hpCurrent"]; got != 4 {
		t.Fatalf("hpCurrent = %d, want 4", got)
	}
	potion, _ := runtime.Object("object:potion")
	if got := potion.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("potion shotsCurrent = %q, want 1", got)
	}
}

func TestDefaultDrinkMagicEffectFullHealDoesNotConsumeAtMax(t *testing.T) {
	loaded := drinkWorld(t, "room:tavern", "2", "20")
	creature := loaded.Creatures["creature:alice"]
	creature.Stats = map[string]int{"hpCurrent": 30, "hpMax": 30}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewDrinkHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"치료"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n당신은 완치술이 필요없습니다.\n" {
		t.Fatalf("status/output = %d/%q, want '\\n당신은 완치술이 필요없습니다.\\n'", status, ctx.OutputString())
	}
	potion, _ := runtime.Object("object:potion")
	if got := potion.Properties["shotsCurrent"]; got != "2" {
		t.Fatalf("potion shotsCurrent = %q, want unchanged 2", got)
	}
}

func TestDefaultDrinkMagicEffectDefensiveTags(t *testing.T) {
	useSpellFailRoll(t, 0)
	tests := []struct {
		name       string
		magicPower string
		wantTag    string
	}{
		{name: "light", magicPower: "3", wantTag: "light"},
		{name: "bless", magicPower: "5", wantTag: "blessed"},
		{name: "protection", magicPower: "6", wantTag: "protection"},
		{name: "invisibility", magicPower: "8", wantTag: "invisible"},
		{name: "levitate", magicPower: "22", wantTag: "levitate"},
		{name: "resist fire", magicPower: "23", wantTag: "resistFire"},
		{name: "fly", magicPower: "24", wantTag: "fly"},
		{name: "resist magic", magicPower: "25", wantTag: "resistMagic"},
		{name: "know alignment", magicPower: "42", wantTag: "knowAlignment"},
		{name: "resist cold", magicPower: "44", wantTag: "resistCold"},
		{name: "breathe water", magicPower: "45", wantTag: "breatheWater"},
		{name: "earth shield", magicPower: "46", wantTag: "earthShield"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := state.NewWorld(drinkWorld(t, "room:tavern", "2", tt.magicPower))

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewDrinkHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"치료"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}
			creature, _ := runtime.Creature("creature:alice")
			if !hasAnyNormalizedFlag(creature.Metadata.Tags, tt.wantTag) {
				t.Fatalf("creature tags = %+v, want %s", creature.Metadata.Tags, tt.wantTag)
			}
			player, _ := runtime.Player("player:alice")
			if !hasAnyNormalizedFlag(player.Metadata.Tags, tt.wantTag) {
				t.Fatalf("player tags = %+v, want %s", player.Metadata.Tags, tt.wantTag)
			}
			potion, _ := runtime.Object("object:potion")
			if got := potion.Properties["shotsCurrent"]; got != "1" {
				t.Fatalf("potion shotsCurrent = %q, want 1", got)
			}
		})
	}
}

func TestDefaultReadScrollMagicEffectPlayerOnlyStateTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "22")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted levitate scroll")
	}
	if !strings.Contains(ctx.OutputString(), "Bob가 떠다니기 시작합니다.") {
		t.Fatalf("output = %q, want targeted levitate text", ctx.OutputString())
	}
	bobCreature, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCreature.Metadata.Tags, "levitate") {
		t.Fatalf("bob creature tags = %+v, want levitate", bobCreature.Metadata.Tags)
	}
	bobPlayer, _ := runtime.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "levitate") {
		t.Fatalf("bob player tags = %+v, want levitate", bobPlayer.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectPlayerOnlyStateRejectsPlayerIDAlias(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "22")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "player:bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람은 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C find_crt miss", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after player ID alias target")
	}
	bob, _ := runtime.Creature("creature:bob")
	if hasAnyNormalizedFlag(bob.Metadata.Tags, "levitate", "PLEVIT") {
		t.Fatalf("bob tags = %+v, want levitate absent", bob.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectPlayerOnlyStateMatchesCreatureName(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "22")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "AccountBob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Shadow",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Shadow"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); !strings.Contains(got, "Shadow가 떠다니기 시작합니다.") {
		t.Fatalf("output = %q, want creature-name levitate target", got)
	}
	bob, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bob.Metadata.Tags, "levitate", "PLEVIT") {
		t.Fatalf("bob tags = %+v, want levitate", bob.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectPlayerOnlyStateDoesNotConsumeOnMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "22")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람은 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after invalid levitate target")
	}
	merchant, _ := runtime.Creature("creature:merchant")
	if hasAnyNormalizedFlag(merchant.Metadata.Tags, "levitate") {
		t.Fatalf("merchant tags = %+v, want levitate absent", merchant.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectInvisibilityTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "8")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted invisibility scroll")
	}
	if !strings.Contains(ctx.OutputString(), "\nBob에게 소명부를 먹이고 은둔법의 주문을 겁니다.\nBob의 몸이 눈부실 정도로 강렬한 빛을 내다가 갑자기 \n사라졌습니다.\n") {
		t.Fatalf("output = %q, want targeted invisibility text", ctx.OutputString())
	}
	bobCreature, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCreature.Metadata.Tags, "invisible", "PINVIS") {
		t.Fatalf("bob creature tags = %+v, want invisible", bobCreature.Metadata.Tags)
	}
	bobPlayer, _ := runtime.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "invisible", "PINVIS") {
		t.Fatalf("bob player tags = %+v, want invisible", bobPlayer.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectGenericTargetRejectsPlayerIDAlias(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "8")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "player:bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람은 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C find_crt miss", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after player ID alias target")
	}
	bob, _ := runtime.Creature("creature:bob")
	if hasAnyNormalizedFlag(bob.Metadata.Tags, "invisible", "PINVIS") {
		t.Fatalf("bob tags = %+v, want invisibility absent", bob.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectGenericTargetMatchesCreatureName(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "8")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "AccountBob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Shadow",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Shadow"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); !strings.Contains(got, "\nShadow에게 소명부를 먹이고 은둔법의 주문을 겁니다.") {
		t.Fatalf("output = %q, want creature-name invisibility target", got)
	}
	bob, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bob.Metadata.Tags, "invisible", "PINVIS") {
		t.Fatalf("bob tags = %+v, want invisibility", bob.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectInvisibilityRejectsMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "8")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람은 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after invalid invisibility target")
	}
}

func TestDefaultReadScrollMagicEffectInvisibilityRejectsWhileFighting(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "8")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:goblin",
		Kind:        model.CreatureKindNPC,
		DisplayName: "고블린",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)
	if _, err := runtime.AddEnemy("creature:goblin", "creature:alice"); err != nil {
		t.Fatalf("AddEnemy() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n지금 싸우고 있잖아요..!!.\n" {
		t.Fatalf("status/output = %d/%q, want fighting refusal", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after fighting invisibility refusal")
	}
}

func TestDefaultReadScrollMagicEffectDetectInvisibleTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "10")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted detect-invisible scroll")
	}
	if !strings.Contains(ctx.OutputString(), "\n당신은 Bob의 인당혈을 찍으며 은둔감지술을 외웁니다.\n그의 눈에서 푸른광안이 떠오릅니다.\n") {
		t.Fatalf("output = %q, want targeted detect-invisible text", ctx.OutputString())
	}
	bobCreature, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCreature.Metadata.Tags, "detectInvisible", "PDINVI") {
		t.Fatalf("bob creature tags = %+v, want detectInvisible", bobCreature.Metadata.Tags)
	}
	bobPlayer, _ := runtime.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "detectInvisible", "PDINVI") {
		t.Fatalf("bob player tags = %+v, want detectInvisible", bobPlayer.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectDetectInvisibleRejectsMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "10")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람이 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after invalid detect-invisible target")
	}
}

func TestDefaultReadScrollMagicEffectDetectMagicTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "11")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted detect-magic scroll")
	}
	if !strings.Contains(ctx.OutputString(), "\n당신은 Bob의 백회혈을 찍으며 주문감지술의 \n주문을 외웁니다.\n갑자기 그의 두눈에 은빛광안이 떠오릅니다.\n") {
		t.Fatalf("output = %q, want targeted detect-magic text", ctx.OutputString())
	}
	bobCreature, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCreature.Metadata.Tags, "detectMagic", "PDMAGI") {
		t.Fatalf("bob creature tags = %+v, want detectMagic", bobCreature.Metadata.Tags)
	}
	bobPlayer, _ := runtime.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "detectMagic", "PDMAGI") {
		t.Fatalf("bob player tags = %+v, want detectMagic", bobPlayer.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectDetectMagicRejectsMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "11")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람이 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after invalid detect-magic target")
	}
}

func TestDefaultReadScrollMagicEffectBlessRejectsMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "5")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람이 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after invalid bless target")
	}
}

func TestDefaultReadScrollMagicEffectProtectionRejectsMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "6")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그런 사람은 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after invalid protection target")
	}
}

func TestMagicEffectBlessProtectionPotionsRejectMissingTargetBeforeLookup(t *testing.T) {
	tests := []struct {
		name   string
		effect func(*Context, StatusWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error)
	}{
		{name: "bless", effect: magicEffectBless},
		{name: "protection", effect: magicEffectProtection},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useSpellFailRoll(t, 0)
			loaded := readScrollWorld(t, "room:library", "1", "1")
			alice := loaded.Creatures["creature:alice"]
			loaded.Creatures[alice.ID] = alice
			runtime := state.NewWorld(loaded)
			potion := model.ObjectInstance{ID: "object:buff-potion", Properties: map[string]string{"type": "6"}}
			ctx := &Context{ActorID: "player:alice"}

			success, err := tt.effect(ctx, runtime, alice, potion, ResolvedCommand{Args: []string{"주문", "Nobody"}})
			if err != nil {
				t.Fatalf("effect error = %v", err)
			}
			if success {
				t.Fatalf("success = true, want false")
			}
			if got, want := ctx.OutputString(), "그 물건은 자신에게만 사용할 수 있습니다.\n"; got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestDefaultDrinkMagicEffectRemoveDisease(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := drinkWorld(t, "room:tavern", "2", "49")
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"PDISEA", "hidden"}
	loaded.Creatures[creature.ID] = creature
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"diseased"}
	loaded.Players[player.ID] = player
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewDrinkHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"치료"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if !strings.Contains(ctx.OutputString(), "\n병마에 시달리던 당신의 몸이 활기를 띄기 시작합니다.\n") {
		t.Fatalf("output = %q, want disease potion text", ctx.OutputString())
	}
	updatedCreature, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updatedCreature.Metadata.Tags, "disease", "diseased", "pdisea") {
		t.Fatalf("creature tags = %+v, want disease removed", updatedCreature.Metadata.Tags)
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "disease", "diseased", "pdisea") {
		t.Fatalf("player tags = %+v, want disease removed", updatedPlayer.Metadata.Tags)
	}
	potion, _ := runtime.Object("object:potion")
	if got := potion.Properties["shotsCurrent"]; got != "1" {
		t.Fatalf("potion shotsCurrent = %q, want 1", got)
	}
}

func TestDefaultDrinkMagicEffectRemoveDiseaseNotDiseasedText(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := drinkWorld(t, "room:tavern", "2", "49")
	runtime := state.NewWorld(loaded)
	creature := loaded.Creatures["creature:alice"]
	potion := loaded.Objects["object:potion"]

	ctx := &Context{ActorID: "player:alice"}
	success, err := defaultDrinkMagicEffect(ctx, runtime, creature, potion, ResolvedCommand{})
	if err != nil {
		t.Fatalf("defaultDrinkMagicEffect error = %v", err)
	}
	if !success {
		t.Fatalf("defaultDrinkMagicEffect success = false")
	}
	want := "\n당신의 병이 해소되어 몸이 거뜬해집니다.\n"
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestMagicEffectRemoveDiseasePotionRejectsTarget(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := readScrollWorld(t, "room:library", "1", "49")
	alice := loaded.Creatures["creature:alice"]
	loaded.Creatures[alice.ID] = alice
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	loaded.Creatures["creature:bob"] = model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:library",
		Metadata:    model.Metadata{Tags: []string{"PDISEA"}},
	}
	runtime := state.NewWorld(loaded)
	potion := model.ObjectInstance{ID: "object:disease-potion", Properties: map[string]string{"type": "6"}}
	ctx := &Context{ActorID: "player:alice"}

	success, err := magicEffectRemoveDisease(ctx, runtime, alice, potion, ResolvedCommand{Args: []string{"주문", "Bob"}})
	if err != nil {
		t.Fatalf("magicEffectRemoveDisease error = %v", err)
	}
	if success {
		t.Fatalf("success = true, want false")
	}
	if got, want := ctx.OutputString(), "\n이 물건은 자신에게만 사용할수 있습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	updatedBob, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(updatedBob.Metadata.Tags, "PDISEA") {
		t.Fatalf("Bob disease tag removed on rejected potion: %+v", updatedBob.Metadata.Tags)
	}
}

func TestMagicEffectRemoveDiseasePotionRejectsMissingTargetBeforeLookup(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := readScrollWorld(t, "room:library", "1", "49")
	alice := loaded.Creatures["creature:alice"]
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)
	potion := model.ObjectInstance{ID: "object:disease-potion", Properties: map[string]string{"type": "6"}}
	ctx := &Context{ActorID: "player:alice"}

	success, err := magicEffectRemoveDisease(ctx, runtime, alice, potion, ResolvedCommand{Args: []string{"주문", "Nobody"}})
	if err != nil {
		t.Fatalf("magicEffectRemoveDisease error = %v", err)
	}
	if success {
		t.Fatalf("success = true, want false")
	}
	if got, want := ctx.OutputString(), "\n이 물건은 자신에게만 사용할수 있습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDefaultReadScrollMagicEffectRemoveDiseaseTarget(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := readScrollWorld(t, "room:library", "1", "49")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Metadata:    model.Metadata{Tags: []string{"PDISEA"}},
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted remove disease scroll")
	}
	if !strings.Contains(ctx.OutputString(), "\n당신은 상인의 혈도를 누르고 내공의 힘을 통해\n치료를 시작합니다.") {
		t.Fatalf("output = %q, want targeted remove disease text", ctx.OutputString())
	}
	updatedTarget, _ := runtime.Creature("creature:merchant")
	if hasAnyNormalizedFlag(updatedTarget.Metadata.Tags, "disease", "diseased", "pdisea") {
		t.Fatalf("target tags = %+v, want disease removed", updatedTarget.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectRemoveDiseaseExplicitSelfAliasMissesLikeLegacy(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := readScrollWorld(t, "room:library", "1", "49")
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PDISEA"}
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "나"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런사람이 존재하지 않습니다 .\n" {
		t.Fatalf("status/output = %d/%q, want C target-branch miss", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after explicit self alias miss")
	}
	updated, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "PDISEA", "disease", "diseased") {
		t.Fatalf("actor tags = %+v, want disease unchanged", updated.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectRemoveDiseaseNamedSelfUsesTargetBranchLikeLegacy(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := readScrollWorld(t, "room:library", "1", "49")
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"PDISEA"}
	loaded.Creatures[alice.ID] = alice
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Alice"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if !strings.Contains(ctx.OutputString(), "\n당신은 Alice의 혈도를 누르고 내공의 힘을 통해\n치료를 시작합니다.\n") {
		t.Fatalf("output = %q, want target-branch remove disease text", ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after named-self target branch success")
	}
	updated, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "PDISEA", "disease", "diseased") {
		t.Fatalf("actor tags = %+v, want disease removed", updated.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectResistFireTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "23")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted resist-fire scroll")
	}
	if !strings.Contains(ctx.OutputString(), "\n오행중 수의 수호령들이 나타나 Bob의 주위에 \n진을 형성합니다.\n") {
		t.Fatalf("output = %q, want targeted resist-fire text", ctx.OutputString())
	}
	bobCreature, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCreature.Metadata.Tags, "resistFire", "PRFIRE") {
		t.Fatalf("bob creature tags = %+v, want resistFire", bobCreature.Metadata.Tags)
	}
	bobPlayer, _ := runtime.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "resistFire", "PRFIRE") {
		t.Fatalf("bob player tags = %+v, want resistFire", bobPlayer.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectResistFireRejectsMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "23")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람이 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after invalid resist-fire target")
	}
}

func TestDefaultReadScrollMagicEffectFlyTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "24")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted fly scroll")
	}
	if !strings.Contains(ctx.OutputString(), "Bob의 몸이 하늘로 떠오르며 날기 시작합니다.\n") {
		t.Fatalf("output = %q, want targeted fly text", ctx.OutputString())
	}
	bobCreature, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCreature.Metadata.Tags, "fly", "PFLYSP") {
		t.Fatalf("bob creature tags = %+v, want fly", bobCreature.Metadata.Tags)
	}
	bobPlayer, _ := runtime.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "fly", "PFLYSP") {
		t.Fatalf("bob player tags = %+v, want fly", bobPlayer.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectFlyRejectsMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "24")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람이 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after invalid fly target")
	}
}

func TestDefaultReadScrollMagicEffectResistMagicTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "25")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted resist-magic scroll")
	}
	if !strings.Contains(ctx.OutputString(), "\n땅속에서 금의 수호령들이 올라와 Bob의 주위에 \n보마진을 형성합니다.\n") {
		t.Fatalf("output = %q, want targeted resist-magic text", ctx.OutputString())
	}
	bobCreature, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCreature.Metadata.Tags, "resistMagic", "PRMAGI") {
		t.Fatalf("bob creature tags = %+v, want resistMagic", bobCreature.Metadata.Tags)
	}
	bobPlayer, _ := runtime.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "resistMagic", "PRMAGI") {
		t.Fatalf("bob player tags = %+v, want resistMagic", bobPlayer.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectResistMagicRejectsMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "25")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람은 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after invalid resist-magic target")
	}
}

func TestDefaultReadScrollMagicEffectKnowAlignmentTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "42")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted know-alignment scroll")
	}
	if !strings.Contains(ctx.OutputString(), "Bob이 선악을 감지할 수 있는 식별력이 높아졌습니다.\n") {
		t.Fatalf("output = %q, want targeted know-alignment text", ctx.OutputString())
	}
	bobCreature, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCreature.Metadata.Tags, "knowAlignment", "PKNOWA") {
		t.Fatalf("bob creature tags = %+v, want knowAlignment", bobCreature.Metadata.Tags)
	}
	bobPlayer, _ := runtime.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "knowAlignment", "PKNOWA") {
		t.Fatalf("bob player tags = %+v, want knowAlignment", bobPlayer.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectKnowAlignmentRejectsMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "42")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람이 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after invalid know-alignment target")
	}
}

func TestDefaultReadScrollMagic6PlayerStateExplicitSelfAliasMissesLikeLegacy(t *testing.T) {
	for _, tt := range []struct {
		name       string
		magicPower string
		want       string
	}{
		{name: "resist magic", magicPower: "25", want: "\n그런 사람은 존재하지 않습니다.\n"},
		{name: "know alignment", magicPower: "42", want: "\n그런 사람이 존재하지 않습니다.\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", tt.magicPower)
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "나"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want C target-branch miss", status, ctx.OutputString())
			}
			if _, ok := runtime.Object("object:scroll"); !ok {
				t.Fatal("scroll was consumed after explicit self alias miss")
			}
			actor, _ := runtime.Creature("creature:alice")
			if hasAnyNormalizedFlag(actor.Metadata.Tags, "PRMAGI", "PKNOWA", "resistMagic", "knowAlignment") {
				t.Fatalf("actor tags = %+v, want no self-cast state", actor.Metadata.Tags)
			}
		})
	}
}

func TestDefaultReadScrollMagic6PlayerStateNamedSelfUsesTargetBranchLikeLegacy(t *testing.T) {
	for _, tt := range []struct {
		name       string
		magicPower string
		want       string
	}{
		{name: "resist magic", magicPower: "25", want: "Alice의 주위에 \n보마진을 형성합니다.\n"},
		{name: "know alignment", magicPower: "42", want: "Alice이 선악을 감지할 수 있는 식별력이 높아졌습니다.\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", tt.magicPower)
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Alice"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}
			if !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("output = %q, want target-branch text containing %q", ctx.OutputString(), tt.want)
			}
			if _, ok := runtime.Object("object:scroll"); ok {
				t.Fatal("scroll still exists after named-self target branch success")
			}
		})
	}
}

func TestDefaultReadScrollMagic7PlayerStateExplicitSelfAliasMissesLikeLegacy(t *testing.T) {
	for _, tt := range []struct {
		name       string
		magicPower string
		want       string
	}{
		{name: "resist cold", magicPower: "44", want: "\n그런 사람이 존재하지 않습니다 .\n"},
		{name: "breathe water", magicPower: "45", want: "\n그런 사람이 존재하지 않습니다 .\n"},
		{name: "earth shield", magicPower: "46", want: "\n그런 사람이 존재하지 않습니다.\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", tt.magicPower)
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "나"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want C target-branch miss", status, ctx.OutputString())
			}
			if _, ok := runtime.Object("object:scroll"); !ok {
				t.Fatal("scroll was consumed after explicit self alias miss")
			}
			actor, _ := runtime.Creature("creature:alice")
			if hasAnyNormalizedFlag(actor.Metadata.Tags, "PRCOLD", "PBRWAT", "PSSHLD", "resistCold", "breatheWater", "earthShield") {
				t.Fatalf("actor tags = %+v, want no self-cast state", actor.Metadata.Tags)
			}
		})
	}
}

func TestDefaultReadScrollMagic7PlayerStateNamedSelfUsesTargetBranchLikeLegacy(t *testing.T) {
	for _, tt := range []struct {
		name       string
		magicPower string
		want       string
	}{
		{name: "resist cold", magicPower: "44", want: "\nAlice의 주위에 화의 수호령들이 진을 형성하며\n주위를 둘러쌉니다.\n"},
		{name: "breathe water", magicPower: "45", want: "\nAlice의 가슴이 평소보다 두배 커져 물속에서 \n오랫동안 견딜수 있을 것 같습니다.\n"},
		{name: "earth shield", magicPower: "46", want: "\n땅에서 오행중 토의 수호령들이 올라와 Alice의\n주위에 진을 형성합니다.\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", tt.magicPower)
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Alice"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}
			if !strings.Contains(ctx.OutputString(), tt.want) {
				t.Fatalf("output = %q, want target-branch text containing %q", ctx.OutputString(), tt.want)
			}
			if _, ok := runtime.Object("object:scroll"); ok {
				t.Fatal("scroll still exists after named-self target branch success")
			}
		})
	}
}

func TestMagicEffectUtilityPotionsRejectMissingTargetBeforeLookup(t *testing.T) {
	tests := []struct {
		name   string
		effect func(*Context, StatusWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error)
		want   string
	}{
		{name: "invisibility", effect: magicEffectInvisibility, want: "\n그 물건은 자신에게만 사용할수 있습니다.\n"},
		{name: "detect invisible", effect: magicEffectDetectInvisible, want: "\n그 물건은 자신에게만 사용할수 있습니다.\n"},
		{name: "detect magic", effect: magicEffectDetectMagic, want: "\n그 물건은 자신에게만 사용할수 있습니다.\n"},
		{name: "resist magic", effect: magicEffectResistMagic, want: "\n그 물건은 약병은 자신에게만 사용할수 있습니다.\n"},
		{name: "know alignment", effect: magicEffectKnowAlignment, want: "\n그 물건은 자신에게만 사용할수 있습니다.\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useSpellFailRoll(t, 0)
			loaded := readScrollWorld(t, "room:library", "1", "1")
			alice := loaded.Creatures["creature:alice"]
			loaded.Creatures[alice.ID] = alice
			runtime := state.NewWorld(loaded)
			potion := model.ObjectInstance{ID: "object:utility-potion", Properties: map[string]string{"type": "6"}}
			ctx := &Context{ActorID: "player:alice"}

			success, err := tt.effect(ctx, runtime, alice, potion, ResolvedCommand{Args: []string{"주문", "Nobody"}})
			if err != nil {
				t.Fatalf("effect error = %v", err)
			}
			if success {
				t.Fatalf("success = true, want false")
			}
			if got := ctx.OutputString(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMagicEffectDefensivePotionsRejectMissingTargetBeforeLookup(t *testing.T) {
	tests := []struct {
		name   string
		effect func(*Context, StatusWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error)
		want   string
	}{
		{name: "cure poison", effect: magicEffectCurePoison, want: "그 물건은 자신에게만 사용할 수 있습니다.\n"},
		{name: "resist cold", effect: magicEffectResistCold, want: "\n그 물건은 자신에게만 사용할수 있습니다.\n"},
		{name: "breathe water", effect: magicEffectBreatheWater, want: "\n이 물건은 자신에게만 사용할수 있습니다.\n"},
		{name: "earth shield", effect: magicEffectEarthShield, want: "\n이 물건은 자신에게만 사용할수 있습니다.\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useSpellFailRoll(t, 0)
			loaded := readScrollWorld(t, "room:library", "1", "1")
			alice := loaded.Creatures["creature:alice"]
			runtime := state.NewWorld(loaded)
			potion := model.ObjectInstance{ID: "object:defensive-potion", Properties: map[string]string{"type": "6"}}
			ctx := &Context{ActorID: "player:alice"}

			success, err := tt.effect(ctx, runtime, alice, potion, ResolvedCommand{Args: []string{"주문", "Nobody"}})
			if err != nil {
				t.Fatalf("effect error = %v", err)
			}
			if success {
				t.Fatalf("success = true, want false")
			}
			if got := ctx.OutputString(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultReadScrollMagicEffectResistColdTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "44")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted resist-cold scroll")
	}
	if !strings.Contains(ctx.OutputString(), "\nBob의 주위에 화의 수호령들이 진을 형성하며\n주위를 둘러쌉니다.\n") {
		t.Fatalf("output = %q, want targeted resist-cold text", ctx.OutputString())
	}
	bobCreature, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCreature.Metadata.Tags, "resistCold", "PRCOLD") {
		t.Fatalf("bob creature tags = %+v, want resistCold", bobCreature.Metadata.Tags)
	}
	bobPlayer, _ := runtime.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "resistCold", "PRCOLD") {
		t.Fatalf("bob player tags = %+v, want resistCold", bobPlayer.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectResistColdRejectsMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "44")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람이 존재하지 않습니다 .\n" {
		t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after invalid resist-cold target")
	}
}

func TestDefaultReadScrollMagicEffectBreatheWaterTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "45")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted breathe-water scroll")
	}
	if !strings.Contains(ctx.OutputString(), "\nBob의 가슴이 평소보다 두배 커져 물속에서 \n오랫동안 견딜수 있을 것 같습니다.\n") {
		t.Fatalf("output = %q, want targeted breathe-water text", ctx.OutputString())
	}
	bobCreature, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCreature.Metadata.Tags, "breatheWater", "PBRWAT") {
		t.Fatalf("bob creature tags = %+v, want breatheWater", bobCreature.Metadata.Tags)
	}
	bobPlayer, _ := runtime.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "breatheWater", "PBRWAT") {
		t.Fatalf("bob player tags = %+v, want breatheWater", bobPlayer.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectBreatheWaterRejectsMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "45")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람이 존재하지 않습니다 .\n" {
		t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after invalid breathe-water target")
	}
}

func TestDefaultReadScrollMagicEffectEarthShieldTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "46")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted earth-shield scroll")
	}
	if !strings.Contains(ctx.OutputString(), "\n땅에서 오행중 토의 수호령들이 올라와 Bob의\n주위에 진을 형성합니다.\n") {
		t.Fatalf("output = %q, want targeted earth-shield text", ctx.OutputString())
	}
	bobCreature, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCreature.Metadata.Tags, "earthShield", "PSSHLD") {
		t.Fatalf("bob creature tags = %+v, want earthShield", bobCreature.Metadata.Tags)
	}
	bobPlayer, _ := runtime.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "earthShield", "PSSHLD") {
		t.Fatalf("bob player tags = %+v, want earthShield", bobPlayer.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectEarthShieldRejectsMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "46")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람이 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after invalid earth-shield target")
	}
}

func TestDefaultReadScrollMagicEffectHostileStateTargets(t *testing.T) {
	tests := []struct {
		name       string
		magicPower string
		wantTag    string
		config     func(*model.Creature)
	}{
		{name: "befuddle", magicPower: "13", wantTag: "befuddled"},
		{name: "fear", magicPower: "51", wantTag: "fearful"},
		{
			name:       "blind",
			magicPower: "54",
			wantTag:    "blind",
			config: func(alice *model.Creature) {
				if alice.Stats == nil {
					alice.Stats = map[string]int{}
				}
				alice.Stats["class"] = model.ClassSubDM
			},
		},
		{
			name:       "silence",
			magicPower: "55",
			wantTag:    "silenced",
			config: func(alice *model.Creature) {
				if alice.Stats == nil {
					alice.Stats = map[string]int{}
				}
				alice.Stats["class"] = model.ClassSubDM
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", tt.magicPower)
			mustAddLookCreature(t, loaded, model.Creature{
				ID:          "creature:merchant",
				Kind:        model.CreatureKindNPC,
				DisplayName: "상인",
				RoomID:      "room:library",
				Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
			})
			if tt.config != nil {
				alice := loaded.Creatures["creature:alice"]
				tt.config(&alice)
				loaded.Creatures[alice.ID] = alice
			}
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}
			if _, ok := runtime.Object("object:scroll"); ok {
				t.Fatal("scroll still exists after hostile state scroll")
			}
			if tt.name == "fear" && !strings.Contains(ctx.OutputString(), "당신은 지옥구술을 상인에게 던졌습니다.") {
				t.Fatalf("output = %q, want fear target text", ctx.OutputString())
			}
			merchant, _ := runtime.Creature("creature:merchant")
			if !hasAnyNormalizedFlag(merchant.Metadata.Tags, tt.wantTag) {
				t.Fatalf("merchant tags = %+v, want %s", merchant.Metadata.Tags, tt.wantTag)
			}
		})
	}
}

func TestDefaultReadScrollMagicEffectBefuddlePlayerTargetDoesNotSetPlayerFlag(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "13")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted befuddle scroll")
	}
	if !strings.Contains(ctx.OutputString(), "당신은 흑기를 땅에 꼿으며 Bob에게 일종인 흑안법을 걸었습니다.") {
		t.Fatalf("output = %q, want targeted befuddle text", ctx.OutputString())
	}
	bobCreature, _ := runtime.Creature("creature:bob")
	if hasAnyNormalizedFlag(bobCreature.Metadata.Tags, "befuddled", "MBEFUD") {
		t.Fatalf("bob creature tags = %+v, want no C player befuddle flag", bobCreature.Metadata.Tags)
	}
	bobPlayer, _ := runtime.Player("player:bob")
	if hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "befuddled", "MBEFUD") {
		t.Fatalf("bob player tags = %+v, want no C player befuddle flag", bobPlayer.Metadata.Tags)
	}
	if remaining, ready, err := runtime.UseCreatureCooldown("creature:bob", "befuddled", time.Now().Unix(), 0); err != nil || ready || remaining <= 0 {
		t.Fatalf("bob befuddled cooldown remaining/ready/err = %d/%v/%v, want active cooldown", remaining, ready, err)
	}
}

func TestDefaultReadScrollMagicEffectBefuddleRejectsProtectedMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "13")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:guardian",
		Kind:        model.CreatureKindNPC,
		DisplayName: "수호자",
		RoomID:      "room:library",
		Metadata:    model.Metadata{Tags: []string{"MUNKIL", "MMALES"}},
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "수호자"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n당신은 그를 해칠수 없습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C protected output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after protected befuddle target")
	}
}

func TestMagicEffectBlindRejectsUnauthorizedNonCastUse(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "54")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got, want := ctx.OutputString(), "당신은 사용할 권한이 없는 주문입니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after unauthorized blind effect")
	}
	merchant, _ := runtime.Creature("creature:merchant")
	if hasAnyNormalizedFlag(merchant.Metadata.Tags, "blind", "PBLIND", "MBLIND") {
		t.Fatalf("merchant tags = %+v, want no blind", merchant.Metadata.Tags)
	}
}

func TestDefaultDrinkMagicEffectBlindSelfRequiresSubDM(t *testing.T) {
	loaded := drinkWorld(t, "room:tavern", "2", "54")
	creature := loaded.Creatures["creature:alice"]
	creature.Stats = map[string]int{"class": model.ClassSubDM}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)
	potion := loaded.Objects["object:potion"]

	ctx := &Context{ActorID: "player:alice"}
	success, err := defaultDrinkMagicEffect(ctx, runtime, creature, potion, ResolvedCommand{})
	if err != nil {
		t.Fatalf("defaultDrinkMagicEffect error = %v", err)
	}
	if !success {
		t.Fatalf("defaultDrinkMagicEffect success = false")
	}
	if got, want := ctx.OutputString(), "갑자기 당신의 눈이 감기더니 눈이 떠지질 않습니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	updated, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "blind", "PBLIND") {
		t.Fatalf("alice tags = %+v, want blind", updated.Metadata.Tags)
	}
}

func TestMagicEffectSilenceRejectsUnauthorizedNonCastUse(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "55")
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got, want := ctx.OutputString(), "그 주문을 펼치기엔 당신의 능력이 부족합니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after unauthorized silence effect")
	}
	merchant, _ := runtime.Creature("creature:merchant")
	if hasAnyNormalizedFlag(merchant.Metadata.Tags, "silenced", "PSILNC", "MSILNC") {
		t.Fatalf("merchant tags = %+v, want no silence", merchant.Metadata.Tags)
	}
}

func TestDefaultDrinkMagicEffectSilenceSelfRequiresSubDM(t *testing.T) {
	loaded := drinkWorld(t, "room:tavern", "2", "55")
	creature := loaded.Creatures["creature:alice"]
	creature.Stats = map[string]int{"class": model.ClassSubDM}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)
	potion := loaded.Objects["object:potion"]

	ctx := &Context{ActorID: "player:alice"}
	success, err := defaultDrinkMagicEffect(ctx, runtime, creature, potion, ResolvedCommand{})
	if err != nil {
		t.Fatalf("defaultDrinkMagicEffect error = %v", err)
	}
	if !success {
		t.Fatalf("defaultDrinkMagicEffect success = false")
	}
	if got, want := ctx.OutputString(), "\n당신이 먹은것이 목에 걸려 목소리가 나오지 않습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	updated, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "silenced", "PSILNC") {
		t.Fatalf("alice tags = %+v, want silence", updated.Metadata.Tags)
	}
}

func TestDefaultDrinkMagicEffectFearSelf(t *testing.T) {
	loaded := drinkWorld(t, "room:tavern", "2", "51")
	runtime := state.NewWorld(loaded)
	creature := loaded.Creatures["creature:alice"]
	potion := loaded.Objects["object:potion"]

	ctx := &Context{ActorID: "player:alice"}
	success, err := defaultDrinkMagicEffect(ctx, runtime, creature, potion, ResolvedCommand{})
	if err != nil {
		t.Fatalf("defaultDrinkMagicEffect error = %v", err)
	}
	if !success {
		t.Fatalf("defaultDrinkMagicEffect success = false")
	}
	want := "갑자기 당신이 무서워하던 것들이 나타나 당신을 둘러쌉니다.\n악~~~ 저리가~~ 당신은 공포에 떨기 시작합니다."
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	updated, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "fearful", "PFEARS") {
		t.Fatalf("alice tags = %+v, want fear", updated.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectStatusTargetsMonsterBeforePlayer(t *testing.T) {
	tests := []struct {
		name       string
		power      string
		monsterTag []string
		playerTag  []string
	}{
		{name: "blind", power: "54", monsterTag: []string{"blind", "MBLIND"}, playerTag: []string{"blind", "PBLIND"}},
		{name: "silence", power: "55", monsterTag: []string{"silenced", "MSILNC"}, playerTag: []string{"silenced", "PSILNC"}},
		{name: "fear", power: "51", monsterTag: []string{"fearful", "MFEARS"}, playerTag: []string{"fearful", "PFEARS"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", tt.power)
			alice := loaded.Creatures["creature:alice"]
			alice.Stats["class"] = model.ClassSubDM
			loaded.Creatures[alice.ID] = alice
			mustAddLookCreature(t, loaded, model.Creature{
				ID:          "creature:shade-monster",
				Kind:        model.CreatureKindMonster,
				DisplayName: "그림자",
				RoomID:      "room:library",
			})
			mustAddLookPlayer(t, loaded, model.Player{
				ID:          "player:bob",
				DisplayName: "그림자",
				CreatureID:  "creature:bob",
				RoomID:      "room:library",
			})
			loaded.Creatures["creature:bob"] = model.Creature{
				ID:          "creature:bob",
				Kind:        model.CreatureKindPlayer,
				DisplayName: "그림자",
				PlayerID:    "player:bob",
				RoomID:      "room:library",
			}
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "그림자"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}
			monster, _ := runtime.Creature("creature:shade-monster")
			if !hasAnyNormalizedFlag(monster.Metadata.Tags, tt.monsterTag...) {
				t.Fatalf("monster tags = %+v, want any of %+v", monster.Metadata.Tags, tt.monsterTag)
			}
			bob, _ := runtime.Creature("creature:bob")
			if hasAnyNormalizedFlag(bob.Metadata.Tags, tt.playerTag...) {
				t.Fatalf("player tags = %+v, want no player status tag %+v", bob.Metadata.Tags, tt.playerTag)
			}
		})
	}
}

func TestMagicEffectMagic8HostileStatesExplicitSelfAliasMissesLikeLegacy(t *testing.T) {
	tests := []struct {
		name   string
		effect func(*Context, StatusWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error)
		class  int
		want   string
	}{
		{name: "blind", effect: magicEffectBlind, class: model.ClassSubDM, want: "그런 사람이 존재하지 않습니다."},
		{name: "silence", effect: magicEffectSilence, class: model.ClassSubDM, want: "\n그런 사람이 존재하지 않습니다.\n"},
		{name: "fear", effect: magicEffectFear, class: model.ClassCleric, want: "그런 사람이 존재 하지 않습니다."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useSpellFailRoll(t, 0)
			loaded := readScrollWorld(t, "room:library", "1", "51")
			alice := loaded.Creatures["creature:alice"]
			alice.Stats = map[string]int{"class": tt.class, "mpCurrent": 100}
			loaded.Creatures[alice.ID] = alice
			runtime := state.NewWorld(loaded)
			ctx := &Context{ActorID: "player:alice"}
			scroll := model.ObjectInstance{ID: "object:scroll", Properties: map[string]string{"type": "7"}}

			success, err := tt.effect(ctx, runtime, alice, scroll, ResolvedCommand{Args: []string{"주문", "나"}})
			if err != nil {
				t.Fatalf("effect error = %v", err)
			}
			if success {
				t.Fatalf("success = true, want false")
			}
			if got := ctx.OutputString(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
			updated, _ := runtime.Creature("creature:alice")
			if hasAnyNormalizedFlag(updated.Metadata.Tags, "blind", "PBLIND", "fearful", "PFEARS", "silenced", "PSILNC") {
				t.Fatalf("actor tags = %+v, want no self status", updated.Metadata.Tags)
			}
		})
	}
}

func TestMagicEffectStatusPotionsRejectMissingTargetBeforeLookup(t *testing.T) {
	tests := []struct {
		name   string
		power  string
		effect func(*Context, StatusWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error)
		want   string
	}{
		{name: "blind", power: "54", effect: magicEffectBlind, want: "그 물건은 자신에게만 사용할수 있습니다."},
		{name: "silence", power: "55", effect: magicEffectSilence, want: "\n그 물건은 자신에게만 사용할수 있습니다.\n"},
		{name: "fear", power: "51", effect: magicEffectFear, want: "그 물건은 자신에게만 사용할수 있습니다."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", tt.power)
			alice := loaded.Creatures["creature:alice"]
			alice.Stats["class"] = model.ClassSubDM
			loaded.Creatures[alice.ID] = alice
			runtime := state.NewWorld(loaded)
			potion := model.ObjectInstance{ID: "object:status-potion", Properties: map[string]string{"type": "6"}}
			ctx := &Context{ActorID: "player:alice"}

			success, err := tt.effect(ctx, runtime, alice, potion, ResolvedCommand{Args: []string{"주문", "Nobody"}})
			if err != nil {
				t.Fatalf("effect error = %v", err)
			}
			if success {
				t.Fatalf("success = true, want false")
			}
			if got := ctx.OutputString(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMagicEffectFearPotionRejectsTarget(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "51")
	alice := loaded.Creatures["creature:alice"]
	loaded.Creatures[alice.ID] = alice
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	loaded.Creatures["creature:bob"] = model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:library",
	}
	runtime := state.NewWorld(loaded)
	potion := model.ObjectInstance{ID: "object:fear-potion", Properties: map[string]string{"type": "6"}}
	ctx := &Context{ActorID: "player:alice"}

	success, err := magicEffectFear(ctx, runtime, alice, potion, ResolvedCommand{Args: []string{"주문", "Bob"}})
	if err != nil {
		t.Fatalf("magicEffectFear error = %v", err)
	}
	if success {
		t.Fatalf("success = true, want false")
	}
	if got, want := ctx.OutputString(), "그 물건은 자신에게만 사용할수 있습니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	updatedBob, _ := runtime.Creature("creature:bob")
	if hasAnyNormalizedFlag(updatedBob.Metadata.Tags, "fearful", "PFEARS") {
		t.Fatalf("Bob fear tag added on rejected potion: %+v", updatedBob.Metadata.Tags)
	}
}

func TestMagicEffectFearRejectsPermanentMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "51")
	alice := loaded.Creatures["creature:alice"]
	loaded.Creatures[alice.ID] = alice
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:statue",
		Kind:        model.CreatureKindMonster,
		DisplayName: "석상",
		RoomID:      "room:library",
		Metadata:    model.Metadata{Tags: []string{"MPERMT"}},
	})
	runtime := state.NewWorld(loaded)
	wand := model.ObjectInstance{ID: "object:fear-wand", Properties: map[string]string{"type": "8"}}
	ctx := &Context{ActorID: "player:alice"}

	success, err := magicEffectFear(ctx, runtime, alice, wand, ResolvedCommand{Args: []string{"주문", "석상"}})
	if err != nil {
		t.Fatalf("magicEffectFear error = %v", err)
	}
	if success {
		t.Fatalf("success = true, want false")
	}
	if got, want := ctx.OutputString(), "석상의 주위에 공포의 기운이 둘러쌉니다.\n하지만, 그가 기합을 지르자 금새 그 기운이 사라졌습니다."; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	updated, _ := runtime.Creature("creature:statue")
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "fearful", "MFEARS") {
		t.Fatalf("permanent monster fear tag added: %+v", updated.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectBlessTargetPlayer(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "5")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted bless scroll")
	}
	wantOutput := "\n당신은 한쪽손을 Bob의 머리에 얹으며 성현주를 외웁니다.\n그의 머리에서 삼매광이 뿜어져 나와 성스러운 기운이 몸을 휘감습니다.\n"
	if !strings.Contains(ctx.OutputString(), wantOutput) {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), wantOutput)
	}
	bobCreature, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCreature.Metadata.Tags, "blessed") {
		t.Fatalf("bob creature tags = %+v, want blessed", bobCreature.Metadata.Tags)
	}
	bobPlayer, _ := runtime.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "blessed") {
		t.Fatalf("bob player tags = %+v, want blessed", bobPlayer.Metadata.Tags)
	}
	aliceCreature, _ := runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(aliceCreature.Metadata.Tags, "blessed") {
		t.Fatalf("alice creature tags = %+v, want actor unchanged", aliceCreature.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectProtectionTargetPlayer(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "6")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:library",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		RoomID:      "room:library",
		PlayerID:    "player:bob",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after targeted protection scroll")
	}
	wantOutput := "Bob의 몸에 수호인을 그리며 수호진의 주문을 걸었습니다.\n빛의 수호령들이 그의 주위를 둘러싸며 방어의 진을 형성했습니다.\n"
	if !strings.Contains(ctx.OutputString(), wantOutput) {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), wantOutput)
	}
	bobCreature, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bobCreature.Metadata.Tags, "protection", "PPROTE") {
		t.Fatalf("bob creature tags = %+v, want protection", bobCreature.Metadata.Tags)
	}
	bobPlayer, _ := runtime.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "protection", "PPROTE") {
		t.Fatalf("bob player tags = %+v, want protection", bobPlayer.Metadata.Tags)
	}
}

func TestDefaultDrinkMagicEffectRemoveCursePotionSelf(t *testing.T) {
	loaded := drinkWorld(t, "room:tavern", "2", "43")
	creature := loaded.Creatures["creature:alice"]
	creature.Stats["class"] = model.ClassDM
	weaponID := model.ObjectInstanceID("object:cursed-sword")
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:                  weaponID,
		PrototypeID:         "prototype:stone",
		DisplayNameOverride: "저주받은 검",
		Location:            model.ObjectLocation{CreatureID: creature.ID, Slot: "wield"},
		Metadata:            model.Metadata{Tags: []string{"cursed", "ocurse"}},
		Properties:          map[string]string{"OCURSE": "1"},
	})
	creature.Equipment = map[string]model.ObjectInstanceID{"wield": weaponID}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewDrinkHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"치료"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if want := "\n물건안에 담겨있던 성스러운 기운이 당신의 \n저주를 풀어줍니다.\n"; !strings.Contains(ctx.OutputString(), want) {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), want)
	}
	weapon, _ := runtime.Object(weaponID)
	if objectIsCursed(runtime, weapon) {
		t.Fatalf("weapon = tags %+v properties %+v, want curse removed", weapon.Metadata.Tags, weapon.Properties)
	}
}

func TestDefaultDrinkMagicEffectCursePotionSelf(t *testing.T) {
	loaded := drinkWorld(t, "room:tavern", "2", "57")
	creature := loaded.Creatures["creature:alice"]
	creature.Stats["class"] = model.ClassDM
	weaponID := model.ObjectInstanceID("object:sword")
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:                  weaponID,
		PrototypeID:         "prototype:stone",
		DisplayNameOverride: "장검",
		Location:            model.ObjectLocation{CreatureID: creature.ID, Slot: "wield"},
	})
	creature.Equipment = map[string]model.ObjectInstanceID{"wield": weaponID}
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewDrinkHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"치료"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if want := "\n물건안에 담겨있던 .\n"; !strings.Contains(ctx.OutputString(), want) {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), want)
	}
	weapon, _ := runtime.Object(weaponID)
	if !objectHasAnyTag(runtime, weapon, "cursed", "ocurse") {
		t.Fatalf("weapon tags = %+v, want cursed", weapon.Metadata.Tags)
	}
}

func TestDefaultReadScrollMagicEffectRemoveCurseRejectsMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "43")
	creature := loaded.Creatures["creature:alice"]
	creature.Stats["class"] = model.ClassDM
	loaded.Creatures[creature.ID] = creature
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람이 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after invalid remove-curse target")
	}
}

func TestDefaultReadScrollMagicEffectCurseRejectsMonster(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "57")
	creature := loaded.Creatures["creature:alice"]
	creature.Stats["class"] = model.ClassDM
	loaded.Creatures[creature.ID] = creature
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:library",
		Stats:       map[string]int{"hpCurrent": 10, "hpMax": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람이 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after invalid curse target")
	}
}

func TestDefaultReadScrollMagic6CurseEffectsExplicitSelfAliasMissesLikeLegacy(t *testing.T) {
	for _, tt := range []struct {
		name       string
		magicPower string
		startTags  []string
		wantCursed bool
	}{
		{name: "remove curse", magicPower: "43", startTags: []string{"cursed", "ocurse"}, wantCursed: true},
		{name: "curse", magicPower: "57", wantCursed: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", tt.magicPower)
			actor := loaded.Creatures["creature:alice"]
			actor.Stats["class"] = model.ClassDM
			weaponID := model.ObjectInstanceID("object:alias-sword")
			mustAddLookObject(t, loaded, model.ObjectInstance{
				ID:                  weaponID,
				PrototypeID:         "prototype:stone",
				DisplayNameOverride: "장검",
				Location:            model.ObjectLocation{CreatureID: actor.ID, Slot: "wield"},
				Metadata:            model.Metadata{Tags: tt.startTags},
			})
			actor.Equipment = map[string]model.ObjectInstanceID{"wield": weaponID}
			loaded.Creatures[actor.ID] = actor
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "나"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != "\n그런 사람이 존재하지 않습니다.\n" {
				t.Fatalf("status/output = %d/%q, want C target-branch miss", status, ctx.OutputString())
			}
			if _, ok := runtime.Object("object:scroll"); !ok {
				t.Fatal("scroll was consumed after explicit self alias miss")
			}
			weapon, _ := runtime.Object(weaponID)
			hasCurse := objectHasAnyTag(runtime, weapon, "cursed", "ocurse")
			if hasCurse != tt.wantCursed {
				t.Fatalf("weapon cursed = %v, want %v; tags=%+v", hasCurse, tt.wantCursed, weapon.Metadata.Tags)
			}
		})
	}
}

func TestDefaultMagicEffectDoesNotConsumeWhenTargetIsMissing(t *testing.T) {
	t.Run("scroll", func(t *testing.T) {
		runtime := state.NewWorld(readScrollWorld(t, "room:library", "1", "4"))

		ctx := &Context{ActorID: "player:alice"}
		status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "없는"}})
		if err != nil {
			t.Fatalf("handler() error = %v", err)
		}
		if status != StatusDefault || ctx.OutputString() != "그런 것은 존재하지 않습니다.\n" {
			t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
		}
		if _, ok := runtime.Object("object:scroll"); !ok {
			t.Fatal("scroll was consumed after missing effect target")
		}
	})

	t.Run("wand", func(t *testing.T) {
		runtime := state.NewWorld(zapWorld(t, "room:plaza", "2", "11"))

		ctx := &Context{ActorID: "player:alice"}
		status, err := NewZapHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"마법봉", "없는"}})
		if err != nil {
			t.Fatalf("handler() error = %v", err)
		}
		if status != StatusDefault || ctx.OutputString() != "\n그런 사람이 존재하지 않습니다.\n" {
			t.Fatalf("status/output = %d/%q, want C missing target output", status, ctx.OutputString())
		}
		wand, _ := runtime.Object("object:wand")
		if got := wand.Properties["shotsCurrent"]; got != "2" {
			t.Fatalf("wand shotsCurrent = %q, want unchanged 2", got)
		}
	})

	t.Run("potion recall missing return room", func(t *testing.T) {
		runtime := state.NewWorld(drinkWorld(t, "room:tavern", "2", "17"))

		ctx := &Context{ActorID: "player:alice"}
		status, err := NewDrinkHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"치료"}})
		if err != nil {
			t.Fatalf("handler() error = %v", err)
		}
		wantOutput := "당신의 모습이 어지러이 흔들립니다.\n주문이 실패했습니다.\n"
		if status != StatusDefault || ctx.OutputString() != wantOutput {
			t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), wantOutput)
		}
		potion, _ := runtime.Object("object:potion")
		if got := potion.Properties["shotsCurrent"]; got != "2" {
			t.Fatalf("potion shotsCurrent = %q, want unchanged 2", got)
		}
	})
}

func withFakeMagicEffectTime(t *testing.T, unix int64) {
	t.Helper()
	previous := timeNow
	timeNow = func() time.Time {
		return time.Unix(unix, 0)
	}
	t.Cleanup(func() {
		timeNow = previous
	})
}
