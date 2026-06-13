package command

import (
	"strings"
	"testing"
	"time"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestCastHandlerCastsNamedSpellAndConsumesMP(t *testing.T) {
	useMaxMagicEffectRoll(t)
	loaded := castWorld(t, "room:dojo", 20)
	actor := loaded.Creatures["creature:alice"]
	actor.Metadata.Tags = append(actor.Metadata.Tags, "hidden", "PHIDDN")
	actor.Stats["PHIDDN"] = 1
	loaded.Creatures[actor.ID] = actor
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Players[player.ID] = player
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:dojo",
		Stats:       map[string]int{"hpCurrent": 4, "hpMax": 20},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"회복", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if !strings.Contains(ctx.OutputString(), "상인의 회복을 기원하는 주문을 외웁니다.") {
		t.Fatalf("output = %q, want effect message", ctx.OutputString())
	}
	if strings.Contains(ctx.OutputString(), "당신은 회복 주문을 외웠습니다.") {
		t.Fatalf("output = %q, want no generic cast success message", ctx.OutputString())
	}
	if strings.Contains(ctx.OutputString(), "당신의 모습이 원래대로 돌아왔습니다.") {
		t.Fatalf("output = %q, want hidden clear without PINVIS reveal text", ctx.OutputString())
	}
	updatedActor, _ := runtime.Creature("creature:alice")
	if got := updatedActor.Stats["mpCurrent"]; got != 15 {
		t.Fatalf("mpCurrent = %d, want 15", got)
	}
	if hasAnyNormalizedFlag(updatedActor.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("actor tags = %+v, want hidden cleared", updatedActor.Metadata.Tags)
	}
	if updatedActor.Stats["PHIDDN"] != 0 {
		t.Fatalf("actor PHIDDN = %d, want 0", updatedActor.Stats["PHIDDN"])
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("player tags = %+v, want hidden cleared", updatedPlayer.Metadata.Tags)
	}
	merchant, _ := runtime.Creature("creature:merchant")
	if got := merchant.Stats["hpCurrent"]; got != 20 {
		t.Fatalf("merchant hpCurrent = %d, want 20", got)
	}
}

func TestCastHandlerCastsNumericMagicPower(t *testing.T) {
	runtime := state.NewWorld(castWorld(t, "room:dojo", 20))
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "주문", Number: 38, Handler: "cast"},
		}),
		Handlers: map[string]Handler{
			"cast": NewCastHandler(runtime, nil),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "11 주문")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	creature, _ := runtime.Creature("creature:alice")
	if got := creature.Stats["mpCurrent"]; got != 10 {
		t.Fatalf("mpCurrent = %d, want 10", got)
	}
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "detectMagic", "pdmagi") {
		t.Fatalf("creature tags = %+v, want detectMagic", creature.Metadata.Tags)
	}
	player, _ := runtime.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "detectMagic", "pdmagi") {
		t.Fatalf("player tags = %+v, want detectMagic", player.Metadata.Tags)
	}
}

func TestCastHandlerCastsNumericMagicPowerAtTarget(t *testing.T) {
	useMaxMagicEffectRoll(t)
	loaded := castWorld(t, "room:dojo", 20)
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:dojo",
		Stats:       map[string]int{"hpCurrent": 4, "hpMax": 20},
	})
	runtime := state.NewWorld(loaded)
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "주문", Number: 38, Handler: "cast"},
		}),
		Handlers: map[string]Handler{
			"cast": NewCastHandler(runtime, nil),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := dispatcher.DispatchLine(ctx, "주문 1 상인")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	merchant, _ := runtime.Creature("creature:merchant")
	if got := merchant.Stats["hpCurrent"]; got != 20 {
		t.Fatalf("merchant hpCurrent = %d, want 20", got)
	}
	creature, _ := runtime.Creature("creature:alice")
	if got := creature.Stats["mpCurrent"]; got != 15 {
		t.Fatalf("mpCurrent = %d, want 15", got)
	}
}

func TestCastHandlerLightUsesLegacyOutputAndExpiration(t *testing.T) {
	useSpellFailRoll(t, 0)
	withFakeMagicEffectTime(t, 4000)
	loaded := castWorld(t, "room:dojo", 20)
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = append(creature.Metadata.Tags, "SLIGHT")
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"발광"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	wantOutput := "당신의 왼손에 발광 주문을 걸었습니다.\n왼손에서 황금빛이 뿜어져 나와 주위를 밝혀 줍니다.\n"
	if ctx.OutputString() != wantOutput {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), wantOutput)
	}
	if len(broadcasts) != 1 || !strings.Contains(broadcasts[0].Text, "한쪽 손에 발광 주문을 걸었습니다") {
		t.Fatalf("broadcasts = %+v, want light broadcast", broadcasts)
	}
	updated, _ := runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "PLIGHT", "light") {
		t.Fatalf("creature tags = %+v, want PLIGHT", updated.Metadata.Tags)
	}
	if got := updated.Stats["mpCurrent"]; got != 15 {
		t.Fatalf("mpCurrent = %d, want 15", got)
	}
	if expires, ok := runtime.GetEffectExpiration("creature:alice", "PLIGHT"); !ok || expires != 4600 {
		t.Fatalf("PLIGHT expiration = %d/%v, want 4600/true", expires, ok)
	}
}

func TestCastHandlerLightSpellFailConsumesMPSilently(t *testing.T) {
	useSpellFailRoll(t, 99)
	loaded := castWorld(t, "room:dojo", 5)
	actor := loaded.Creatures["creature:alice"]
	actor.Metadata.Tags = []string{"SLIGHT"}
	loaded.Creatures[actor.ID] = actor
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"발광"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want silent C spell_fail", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after spell_fail cost", got)
	}
	if magicEffectTestHasExactTag(updated.Metadata.Tags, "PLIGHT") {
		t.Fatalf("creature tags = %+v, want light not applied", updated.Metadata.Tags)
	}
}

func TestCastHandlerDefensiveUtilitySpellFailConsumesMPSilently(t *testing.T) {
	tests := []struct {
		name      string
		spell     string
		tag       string
		cost      int
		effectTag string
	}{
		{name: "cure poison", spell: "해독", tag: "SCUREP", cost: 6},
		{name: "bless", spell: "성현진", tag: "SBLESS", cost: 10, effectTag: "PBLESS"},
		{name: "protection", spell: "수호진", tag: "SPROTE", cost: 10, effectTag: "PPROTE"},
		{name: "invisibility", spell: "은둔법", tag: "SINVIS", cost: 15, effectTag: "PINVIS"},
		{name: "detect invisible", spell: "은둔감지술", tag: "SDINVI", cost: 10, effectTag: "PDINVI"},
		{name: "detect magic", spell: "주문감지술", tag: "SDMAGI", cost: 10, effectTag: "PDMAGI"},
		{name: "befuddle", spell: "혼동", tag: "SBEFUD", cost: 10},
		{name: "levitate", spell: "부양술", tag: "SLEVIT", cost: 10, effectTag: "PLEVIT"},
		{name: "resist fire", spell: "방열진", tag: "SRFIRE", cost: 12, effectTag: "PRFIRE"},
		{name: "fly", spell: "비상술", tag: "SFLYSP", cost: 15, effectTag: "PFLYSP"},
		{name: "resist magic", spell: "보마진", tag: "SRMAGI", cost: 12, effectTag: "PRMAGI"},
		{name: "resist cold", spell: "방한진", tag: "SRCOLD", cost: 12, effectTag: "PRCOLD"},
		{name: "breathe water", spell: "수생술", tag: "SBRWAT", cost: 12, effectTag: "PBRWAT"},
		{name: "earth shield", spell: "지방호", tag: "SSSHLD", cost: 12, effectTag: "PSSHLD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useSpellFailRoll(t, 99)
			loaded := castWorld(t, "room:dojo", tt.cost)
			actor := loaded.Creatures["creature:alice"]
			actor.Metadata.Tags = []string{tt.tag}
			loaded.Creatures[actor.ID] = actor
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{tt.spell}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != "" {
				t.Fatalf("status/output = %d/%q, want silent C spell_fail", status, ctx.OutputString())
			}
			updated, _ := runtime.Creature("creature:alice")
			if got := updated.Stats["mpCurrent"]; got != 0 {
				t.Fatalf("mpCurrent = %d, want 0 after spell_fail cost", got)
			}
			if tt.effectTag != "" && magicEffectTestHasExactTag(updated.Metadata.Tags, tt.effectTag) {
				t.Fatalf("creature tags = %+v, want %s not applied", updated.Metadata.Tags, tt.effectTag)
			}
		})
	}
}

func TestCastHandlerOffensiveSpellFailConsumesMPWithoutDamage(t *testing.T) {
	useSpellFailRoll(t, 99)
	loaded := castWorld(t, "room:dojo", 3)
	actor := loaded.Creatures["creature:alice"]
	actor.Metadata.Tags = []string{"SHURTS"}
	loaded.Creatures[actor.ID] = actor
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:merchant",
		Kind:        model.CreatureKindNPC,
		DisplayName: "상인",
		RoomID:      "room:dojo",
		Stats:       map[string]int{"hpCurrent": 40, "hpMax": 40},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"삭풍", "상인"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want silent C offensive spell_fail", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after spell_fail cost", got)
	}
	merchant, _ := runtime.Creature("creature:merchant")
	if got := merchant.Stats["hpCurrent"]; got != 40 {
		t.Fatalf("merchant hpCurrent = %d, want unchanged 40", got)
	}
}

func TestCastHandlerTeleportSpellFailConsumesMPSilently(t *testing.T) {
	useSpellFailRoll(t, 99)
	loaded := castWorld(t, "room:dojo", 20)
	actor := loaded.Creatures["creature:alice"]
	actor.Metadata.Tags = []string{"STELEP"}
	loaded.Creatures[actor.ID] = actor
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"축지법"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want silent C teleport spell_fail", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after C 20 MP teleport fail cost", got)
	}
}

func TestCastHandlerLocatePlayerSpellFailConsumesLegacyCost(t *testing.T) {
	useSpellFailRoll(t, 99)
	loaded := castWorld(t, "room:dojo", 15)
	actor := loaded.Creatures["creature:alice"]
	actor.Metadata.Tags = []string{"SLOCAT"}
	actor.Stats["intelligence"] = 10
	loaded.Creatures[actor.ID] = actor
	mustAddLookRoom(t, loaded, model.Room{ID: "room:garden", DisplayName: "정원"})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:garden",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:garden",
		Stats:       map[string]int{"level": 1, "intelligence": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []activeSession {
				return []activeSession{
					{ID: "session:alice", ActorID: "player:alice"},
					{ID: "session:bob", ActorID: "player:bob"},
				}
			},
		},
	}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"천리안", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want silent C locate spell_fail", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after C 15 MP locate fail cost", got)
	}
}

func TestCastHandlerLocatePlayerChanceFailureConsumesLegacyCost(t *testing.T) {
	useSpellFailRoll(t, 0)
	oldRand := locatePlayerRandIntn
	locatePlayerRandIntn = func(n int) int {
		return n - 1
	}
	defer func() { locatePlayerRandIntn = oldRand }()

	loaded := castWorld(t, "room:dojo", 15)
	actor := loaded.Creatures["creature:alice"]
	actor.Metadata.Tags = []string{"SLOCAT"}
	actor.Stats["level"] = 10
	actor.Stats["intelligence"] = 10
	loaded.Creatures[actor.ID] = actor
	mustAddLookRoom(t, loaded, model.Room{ID: "room:garden", DisplayName: "정원"})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:garden",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:garden",
		Stats:       map[string]int{"level": 10, "intelligence": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []activeSession {
				return []activeSession{
					{ID: "session:alice", ActorID: "player:alice"},
					{ID: "session:bob", ActorID: "player:bob"},
				}
			},
		},
	}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"천리안", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if got := ctx.OutputString(); got != "\n당신의 마음을 Bob에게 집중했습니다.\n\n당신의 정신은 연결될수 없습니다.\n" {
		t.Fatalf("output = %q, want C focus then chance failure", got)
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after C 15 MP locate chance failure cost", got)
	}
}

func TestCastHandlerDrainExpHasNoLegacyMPCost(t *testing.T) {
	previous := attackRoll
	attackRoll = func(min, max int) int {
		return min
	}
	defer func() { attackRoll = previous }()

	loaded := castWorld(t, "room:dojo", 0)
	actor := loaded.Creatures["creature:alice"]
	actor.Level = 10
	actor.Stats = map[string]int{
		"class":            model.ClassDM,
		"level":            10,
		"experience":       1000,
		"proficiencySharp": 2000,
		"mpCurrent":        0,
		"mpMax":            20,
	}
	actor.Metadata.Tags = []string{"SDREXP"}
	loaded.Creatures[actor.ID] = actor
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"백치술"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want unchanged 0 for C drain_exp", got)
	}
	if got := updated.Stats["experience"]; got != 960 {
		t.Fatalf("experience = %d, want 960 after deterministic self drain", got)
	}
}

func TestCastHandlerCharmSelfSpellFailConsumesLegacyCost(t *testing.T) {
	useSpellFailRoll(t, 99)
	loaded := castWorld(t, "room:dojo", 15)
	actor := loaded.Creatures["creature:alice"]
	actor.Metadata.Tags = []string{"SCHARM"}
	loaded.Creatures[actor.ID] = actor
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"이혼대법"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want silent C charm self spell_fail", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after C 15 MP charm self fail cost", got)
	}
}

func TestCastHandlerFearSpellFailConsumesPrepaidCost(t *testing.T) {
	useSpellFailRoll(t, 99)
	loaded := castWorld(t, "room:dojo", 15)
	actor := loaded.Creatures["creature:alice"]
	actor.Metadata.Tags = []string{"SFEARS"}
	loaded.Creatures[actor.ID] = actor
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"공포"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want silent C fear spell_fail", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after prepaid C 15 MP fear cost", got)
	}
	if magicEffectTestHasExactTag(updated.Metadata.Tags, "PFEARS") {
		t.Fatalf("creature tags = %+v, want fear not applied", updated.Metadata.Tags)
	}
}

func TestCastHandlerRmGongUsesLegacyCostAndClass(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := castWorld(t, "room:dojo", 100)
	actor := loaded.Creatures["creature:alice"]
	actor.Stats["class"] = model.ClassBulsa
	actor.Metadata.Tags = []string{"SRMGONG", "PFEARS"}
	loaded.Creatures[actor.ID] = actor
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"공포해소"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after C 100 MP rm_gong cost", got)
	}
	if magicEffectTestHasExactTag(updated.Metadata.Tags, "PFEARS") {
		t.Fatalf("creature tags = %+v, want fear removed", updated.Metadata.Tags)
	}
}

func TestCastHandlerObjectSendUsesDynamicCostWithoutCommonDeduction(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := castWorld(t, "room:dojo", 9)
	actor := loaded.Creatures["creature:alice"]
	actor.Level = 25
	actor.Stats = map[string]int{
		"class":        model.ClassMage,
		"level":        25,
		"intelligence": 0,
		"mpCurrent":    9,
		"mpMax":        20,
	}
	actor.Metadata.Tags = []string{"STRANO"}
	actor.Inventory.ObjectIDs = []model.ObjectInstanceID{"object:sword"}
	loaded.Creatures[actor.ID] = actor
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:sword",
		DisplayName: "sword",
	})
	loaded.Objects["object:sword"] = model.ObjectInstance{
		ID:          "object:sword",
		PrototypeID: "prototype:sword",
		Location:    model.ObjectLocation{CreatureID: actor.ID, Slot: "inventory"},
		Properties: map[string]string{
			"name":   "sword",
			"weight": "5",
		},
	}
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:dojo",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:dojo",
		Stats:       map[string]int{"strength": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := objectSendTestContext()
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"전송", "sword", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after only dynamic 9 MP cost; output=%q", got, ctx.OutputString())
	}
	object, _ := runtime.Object("object:sword")
	if object.Location.CreatureID != "creature:bob" {
		t.Fatalf("object location = %+v, want Bob inventory", object.Location)
	}
}

func TestCastHandlerObjectSendSpellFailKeepsObjectAndConsumesDynamicCost(t *testing.T) {
	useSpellFailRoll(t, 99)
	loaded := castWorld(t, "room:dojo", 9)
	actor := loaded.Creatures["creature:alice"]
	actor.Level = 25
	actor.Stats = map[string]int{
		"class":        model.ClassMage,
		"level":        25,
		"intelligence": 0,
		"mpCurrent":    9,
		"mpMax":        20,
	}
	actor.Metadata.Tags = []string{"STRANO"}
	actor.Inventory.ObjectIDs = []model.ObjectInstanceID{"object:sword"}
	loaded.Creatures[actor.ID] = actor
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:sword",
		DisplayName: "sword",
	})
	loaded.Objects["object:sword"] = model.ObjectInstance{
		ID:          "object:sword",
		PrototypeID: "prototype:sword",
		Location:    model.ObjectLocation{CreatureID: actor.ID, Slot: "inventory"},
		Properties: map[string]string{
			"name":   "sword",
			"weight": "5",
		},
	}
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:dojo",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:dojo",
		Stats:       map[string]int{"strength": 10},
	})
	runtime := state.NewWorld(loaded)

	ctx := objectSendTestContext()
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"전송", "sword", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want silent C object_send spell_fail", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after dynamic 9 MP spell_fail cost", got)
	}
	object, _ := runtime.Object("object:sword")
	if object.Location.CreatureID != "creature:alice" {
		t.Fatalf("object location = %+v, want still with Alice", object.Location)
	}
}

func TestCastHandlerRejectsInvalidCastStates(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		input    string
		mp       int
		roomTags []string
		tags     []string
		want     string
	}{
		{name: "missing spell", mp: 20, want: "어떤 주술을 펼치실겁니까?"},
		{name: "unknown spell", args: []string{"아브라카다브라"}, mp: 20, want: "그런 주문은 존재하지 않습니다."},
		{name: "ambiguous spell", args: []string{"방"}, mp: 20, want: "펼치실 주문의 이름이 이상하군요."},
		{name: "blind", args: []string{"회복"}, mp: 20, tags: []string{"blind"}, want: "아무것도 보이지 않습니다!"},
		{name: "blind before spell lookup", args: []string{"아브라카다브라"}, mp: 20, tags: []string{"blind"}, want: "아무것도 보이지 않습니다!"},
		{name: "silenced", args: []string{"회복"}, mp: 20, tags: []string{"silenced"}, want: "한마디도 할수 없습니다!"},
		{name: "no magic room", args: []string{"회복"}, mp: 20, roomTags: []string{"noMagic"}, want: "주술을 출수 하는데 실패 하셨습니다."},
		{name: "not enough mp", args: []string{"회복"}, mp: 4, want: "당신의 도력이 부족합니다.\n"},
		{name: "unknown number", input: "999 주문", mp: 20, want: "그런 주문은 존재하지 않습니다."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := castWorld(t, "room:dojo", tt.mp)
			room := loaded.Rooms["room:dojo"]
			room.Metadata.Tags = tt.roomTags
			loaded.Rooms[room.ID] = room
			creature := loaded.Creatures["creature:alice"]
			creature.Metadata.Tags = tt.tags
			loaded.Creatures[creature.ID] = creature
			runtime := state.NewWorld(loaded)

			called := false
			ctx := &Context{ActorID: "player:alice"}
			resolved := ResolvedCommand{Args: tt.args, Input: tt.input}
			status, err := NewCastHandler(runtime, func(*Context, CastWorld, model.Creature, ResolvedCommand, int) (bool, error) {
				called = true
				return true, nil
			})(ctx, resolved)
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			if called {
				t.Fatal("effect was called despite rejection")
			}
			updated, _ := runtime.Creature("creature:alice")
			if got := updated.Stats["mpCurrent"]; got != tt.mp {
				t.Fatalf("mpCurrent = %d, want unchanged %d", got, tt.mp)
			}
		})
	}
}

func TestCastHandlerUsesLegacySpecificFailureMessages(t *testing.T) {
	tests := []struct {
		name string
		args []string
		mp   int
		want string
	}{
		{name: "bless not enough mp", args: []string{"성현진"}, mp: 9, want: "\n당신의 도력이 부족합니다.\n"},
		{name: "bless unlearned", args: []string{"성현진"}, mp: 20, want: "\n당신은 아직 그런 주술을 터득하지 못했습니다.\n"},
		{name: "protection unlearned", args: []string{"수호진"}, mp: 20, want: "\n당신은 아직 그 주술을 터득하지 못했습니다.\n"},
		{name: "enchant not enough mp", args: []string{"빙의", "검"}, mp: 24, want: "\n당신의 도력이 부족합니다.\n"},
		{name: "enchant unlearned", args: []string{"빙의", "검"}, mp: 25, want: "\n당신은 아직 그런 주술을 터득하지 못했습니다.\n"},
		{name: "remove curse not enough mp", args: []string{"저주해소"}, mp: 17, want: "\n당신의 도력이 부족합니다.\n"},
		{name: "remove curse unlearned", args: []string{"저주해소"}, mp: 18, want: "\n당신은 아직 그런 주문을 터득하지 못했습니다.\n"},
		{name: "curse not enough mp", args: []string{"저주"}, mp: 24, want: "\n당신의 도력이 부족합니다.\n"},
		{name: "curse unlearned", args: []string{"저주"}, mp: 25, want: "\n당신은 아직 그런 주문을 터득하지 못했습니다.\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := state.NewWorld(castWorld(t, "room:dojo", tt.mp))
			called := false
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewCastHandler(runtime, func(*Context, CastWorld, model.Creature, ResolvedCommand, int) (bool, error) {
				called = true
				return true, nil
			})(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			if called {
				t.Fatal("effect was called despite cast front-door rejection")
			}
		})
	}
}

func TestCastHandlerRecallUsesCLegacyGates(t *testing.T) {
	tests := []struct {
		name      string
		mp        int
		class     int
		tags      []string
		want      string
		wantMP    int
		wantMoved bool
	}{
		{
			name:   "not enough mp",
			mp:     29,
			class:  model.ClassFighter,
			want:   "\n당신이 도력이 부족합니다.\n",
			wantMP: 29,
		},
		{
			name:   "wrong class before learned check",
			mp:     30,
			class:  model.ClassFighter,
			want:   "\n불제자만이 이 주술을 사용할 수 있습니다.\n",
			wantMP: 30,
		},
		{
			name:   "untrained invincible before learned check",
			mp:     30,
			class:  model.ClassInvincible,
			want:   "\n불제자를 무적수련하지 않았습니다..\n",
			wantMP: 30,
		},
		{
			name:   "unlearned cleric",
			mp:     30,
			class:  model.ClassCleric,
			want:   "\n당신은 아직 그런 주문을 터득하지 못했습니다.\n",
			wantMP: 30,
		},
		{
			name:      "success",
			mp:        30,
			class:     model.ClassCleric,
			tags:      []string{"SRECAL"},
			want:      "귀환 주문을 외웠습니다.\n",
			wantMP:    0,
			wantMoved: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := castWorld(t, "room:dojo", tt.mp)
			mustAddLookRoom(t, loaded, model.Room{ID: magicRecallSelfRoomID, DisplayName: "통계 무한 광장"})
			actor := loaded.Creatures["creature:alice"]
			actor.Stats["class"] = tt.class
			actor.Metadata.Tags = tt.tags
			loaded.Creatures[actor.ID] = actor
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"귀환"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			updated, _ := runtime.Creature("creature:alice")
			if got := updated.Stats["mpCurrent"]; got != tt.wantMP {
				t.Fatalf("mpCurrent = %d, want %d", got, tt.wantMP)
			}
			player, _ := runtime.Player("player:alice")
			if tt.wantMoved {
				if player.RoomID != magicRecallSelfRoomID || updated.RoomID != magicRecallSelfRoomID {
					t.Fatalf("actor room = player %q creature %q, want %q", player.RoomID, updated.RoomID, magicRecallSelfRoomID)
				}
			} else if player.RoomID != "room:dojo" || updated.RoomID != "room:dojo" {
				t.Fatalf("actor moved on failed recall: player %q creature %q", player.RoomID, updated.RoomID)
			}
		})
	}
}

func TestCastHandlerRecallConsumesMPBeforeMissingReturnRoomFailure(t *testing.T) {
	loaded := castWorld(t, "room:dojo", 30)
	actor := loaded.Creatures["creature:alice"]
	actor.Metadata.Tags = []string{"SRECAL"}
	loaded.Creatures[actor.ID] = actor
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"귀환"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	wantOutput := "귀환 주문을 외웠습니다.\n주문이 실패했습니다.\n"
	if status != StatusDefault || ctx.OutputString() != wantOutput {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), wantOutput)
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want C pre-load deduction to 0", got)
	}
	player, _ := runtime.Player("player:alice")
	if player.RoomID != "room:dojo" || updated.RoomID != "room:dojo" {
		t.Fatalf("actor room = player %q creature %q, want unchanged room:dojo", player.RoomID, updated.RoomID)
	}
}

func TestCastHandlerMagicTrackUsesCLegacyGates(t *testing.T) {
	tests := []struct {
		name      string
		mp        int
		class     int
		tags      []string
		args      []string
		want      string
		wantMP    int
		wantMoved bool
	}{
		{
			name:   "not enough mp",
			mp:     12,
			class:  model.ClassRanger,
			tags:   []string{"STRACK"},
			args:   []string{"추적", "Bob"},
			want:   "\n당신의 도력이 부족합니다.\n",
			wantMP: 12,
		},
		{
			name:   "wrong class before learned check",
			mp:     13,
			class:  model.ClassFighter,
			args:   []string{"추적", "Bob"},
			want:   "\n포졸만이 이 주술을 사용할 수 있습니다.\n",
			wantMP: 13,
		},
		{
			name:   "untrained invincible before learned check",
			mp:     13,
			class:  model.ClassInvincible,
			args:   []string{"추적", "Bob"},
			want:   "\n포졸을 무적수련하지 않았습니다..\n",
			wantMP: 13,
		},
		{
			name:   "unlearned ranger",
			mp:     13,
			class:  model.ClassRanger,
			args:   []string{"추적", "Bob"},
			want:   "\n당신은 아직 그런 주술을 터득하지 못했습니다.\n",
			wantMP: 13,
		},
		{
			name:      "success",
			mp:        13,
			class:     model.ClassRanger,
			tags:      []string{"STRACK"},
			args:      []string{"추적", "Bob"},
			want:      "\n!!당신은 Bob의 흔적을 찾아내는데 성공했습니다.!!\n그를 추적하여 달려갑니다.\n",
			wantMP:    0,
			wantMoved: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := castWorld(t, "room:dojo", tt.mp)
			mustAddLookRoom(t, loaded, model.Room{ID: "room:garden", DisplayName: "정원"})
			mustAddLookPlayer(t, loaded, model.Player{
				ID:          "player:bob",
				DisplayName: "Bob",
				CreatureID:  "creature:bob",
				RoomID:      "room:garden",
			})
			mustAddLookCreature(t, loaded, model.Creature{
				ID:          "creature:bob",
				Kind:        model.CreatureKindPlayer,
				DisplayName: "Bob",
				PlayerID:    "player:bob",
				RoomID:      "room:garden",
				Stats:       map[string]int{"class": model.ClassFighter, "level": 5},
			})
			actor := loaded.Creatures["creature:alice"]
			actor.Level = 10
			actor.Stats["class"] = tt.class
			actor.Stats["level"] = 10
			actor.Metadata.Tags = tt.tags
			loaded.Creatures[actor.ID] = actor
			runtime := state.NewWorld(loaded)

			ctx := &Context{
				ActorID: "player:alice",
				Values: map[string]any{
					"game.activeSessions": func() []activeSession {
						return []activeSession{
							{ID: "session:alice", ActorID: "player:alice"},
							{ID: "session:bob", ActorID: "player:bob"},
						}
					},
				},
			}
			status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			updated, _ := runtime.Creature("creature:alice")
			if got := updated.Stats["mpCurrent"]; got != tt.wantMP {
				t.Fatalf("mpCurrent = %d, want %d", got, tt.wantMP)
			}
			player, _ := runtime.Player("player:alice")
			if tt.wantMoved {
				if player.RoomID != "room:garden" || updated.RoomID != "room:garden" {
					t.Fatalf("actor room = player %q creature %q, want room:garden", player.RoomID, updated.RoomID)
				}
			} else if player.RoomID != "room:dojo" || updated.RoomID != "room:dojo" {
				t.Fatalf("actor moved on failed track: player %q creature %q", player.RoomID, updated.RoomID)
			}
		})
	}
}

func TestCastHandlerFullHealUsesCLegacyGatesAndDailyLimit(t *testing.T) {
	tests := []struct {
		name      string
		mp        int
		class     int
		tags      []string
		props     map[string]string
		hp        int
		hpMax     int
		want      string
		wantMP    int
		wantHP    int
		wantDaily string
	}{
		{
			name:   "wrong class before learned check",
			mp:     50,
			class:  model.ClassFighter,
			hp:     3,
			hpMax:  10,
			want:   "\n불제자와 무사만이 이 주술을 사용할 수 있습니다.\n",
			wantMP: 50,
			wantHP: 3,
		},
		{
			name:   "unlearned cleric",
			mp:     50,
			class:  model.ClassCleric,
			hp:     3,
			hpMax:  10,
			want:   "\n당신은 아직 그런 주술을 터득하지 못했습니다.\n",
			wantMP: 50,
			wantHP: 3,
		},
		{
			name:   "daily exhausted before heal",
			mp:     50,
			class:  model.ClassCleric,
			tags:   []string{"SFHEAL"},
			props:  map[string]string{"dailyFullHealMax": "10", "dailyFullHealCur": "0"},
			hp:     3,
			hpMax:  10,
			want:   "\n당신의 몸이 너무 피곤해 이 주술을 더 이상 펼칠 수 없습니다.\n",
			wantMP: 50,
			wantHP: 3,
		},
		{
			name:      "success decrements daily",
			mp:        50,
			class:     model.ClassCleric,
			tags:      []string{"SFHEAL"},
			props:     map[string]string{"dailyFullHealMax": "10", "dailyFullHealCur": "10"},
			hp:        3,
			hpMax:     10,
			want:      "\n당신은 천부공을 끌어올리며 완치 주문을 외웁니다.\n천상의 기운들이 당신의 몸으로 모이면서 체력을 최상으로 \n올려 줍니다.\n",
			wantMP:    0,
			wantHP:    10,
			wantDaily: "9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := castWorld(t, "room:dojo", tt.mp)
			actor := loaded.Creatures["creature:alice"]
			actor.Level = 10
			actor.Stats["class"] = tt.class
			actor.Stats["level"] = 10
			actor.Stats["hpCurrent"] = tt.hp
			actor.Stats["hpMax"] = tt.hpMax
			actor.Metadata.Tags = tt.tags
			actor.Properties = tt.props
			loaded.Creatures[actor.ID] = actor
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"완치"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			updated, _ := runtime.Creature("creature:alice")
			if got := updated.Stats["mpCurrent"]; got != tt.wantMP {
				t.Fatalf("mpCurrent = %d, want %d", got, tt.wantMP)
			}
			if got := updated.Stats["hpCurrent"]; got != tt.wantHP {
				t.Fatalf("hpCurrent = %d, want %d", got, tt.wantHP)
			}
			if tt.wantDaily != "" && updated.Properties["dailyFullHealCur"] != tt.wantDaily {
				t.Fatalf("dailyFullHealCur = %q, want %q", updated.Properties["dailyFullHealCur"], tt.wantDaily)
			}
		})
	}
}

func TestCastHandlerFullHealCaretakerTargetCostsHundredMP(t *testing.T) {
	loaded := castWorld(t, "room:dojo", 100)
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:dojo",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:dojo",
		Stats:       map[string]int{"hpCurrent": 3, "hpMax": 10},
	})
	actor := loaded.Creatures["creature:alice"]
	actor.Stats["class"] = model.ClassCaretaker
	actor.Metadata.Tags = []string{"SCLERIC", "SFHEAL"}
	actor.Properties = map[string]string{"dailyFullHealCur": "3"}
	loaded.Creatures[actor.ID] = actor
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"완치", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after C caretaker target full-heal cost", got)
	}
	if got := updated.Properties["dailyFullHealCur"]; got != "1" {
		t.Fatalf("dailyFullHealCur = %q, want C caretaker target decrement to 1", got)
	}
	bob, _ := runtime.Creature("creature:bob")
	if got := bob.Stats["hpCurrent"]; got != 10 {
		t.Fatalf("bob hpCurrent = %d, want 10", got)
	}
}

func TestCastHandlerFullHealExplicitSelfAliasUsesTargetBranchLikeLegacy(t *testing.T) {
	loaded := castWorld(t, "room:dojo", 50)
	actor := loaded.Creatures["creature:alice"]
	actor.Stats["class"] = model.ClassCleric
	actor.Stats["hpCurrent"] = 3
	actor.Stats["hpMax"] = 10
	actor.Metadata.Tags = []string{"SFHEAL"}
	loaded.Creatures[actor.ID] = actor
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"완치", "나"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "\n그런 사람이 존재하지 습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C target-branch miss", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["hpCurrent"]; got != 3 {
		t.Fatalf("hpCurrent = %d, want unchanged", got)
	}
	if got := updated.Stats["mpCurrent"]; got != 50 {
		t.Fatalf("mpCurrent = %d, want no cost before target miss", got)
	}
}

func TestCastHandlerMagic6StateSpellsUseEffectOwnedCost(t *testing.T) {
	for _, tt := range []struct {
		name      string
		spell     string
		learned   string
		cost      int
		effectTag string
	}{
		{name: "resist magic", spell: "보마진", learned: "SRMAGI", cost: 12, effectTag: "PRMAGI"},
		{name: "know alignment", spell: "선악감지", learned: "SKNOWA", cost: 6, effectTag: "PKNOWA"},
		{name: "resist cold", spell: "방한진", learned: "SRCOLD", cost: 12, effectTag: "PRCOLD"},
		{name: "breathe water", spell: "수생술", learned: "SBRWAT", cost: 12, effectTag: "PBRWAT"},
		{name: "earth shield", spell: "지방호", learned: "SSSHLD", cost: 12, effectTag: "PSSHLD"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			loaded := castWorld(t, "room:dojo", tt.cost)
			actor := loaded.Creatures["creature:alice"]
			actor.Metadata.Tags = []string{tt.learned}
			actor.Stats["class"] = model.ClassCleric
			actor.Stats["level"] = 10
			loaded.Creatures[actor.ID] = actor
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{tt.spell}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}
			updated, _ := runtime.Creature("creature:alice")
			if got := updated.Stats["mpCurrent"]; got != 0 {
				t.Fatalf("mpCurrent = %d, want 0 after C effect-owned cost", got)
			}
			if !hasAnyNormalizedFlag(updated.Metadata.Tags, tt.effectTag) {
				t.Fatalf("tags = %+v, want %s", updated.Metadata.Tags, tt.effectTag)
			}
		})
	}
}

func TestCastHandlerClericUtilitySpellsUseEffectGateOrder(t *testing.T) {
	tests := []struct {
		name  string
		spell string
		class int
		tags  []string
		want  string
	}{
		{
			name:  "remove disease wrong class before learned check",
			spell: "치료",
			class: model.ClassFighter,
			want:  "\n이 주술은 불제자만이 사용할 수 있습니다.\n",
		},
		{
			name:  "remove disease unlearned cleric",
			spell: "치료",
			class: model.ClassCleric,
			want:  "\n당신은 아직 그런 주문을 터득하지 못했습니다.\n",
		},
		{
			name:  "remove blindness wrong class before learned check",
			spell: "개안술",
			class: model.ClassFighter,
			want:  "이 기술은 불제자와 무사만이 사용할 수 있습니다.",
		},
		{
			name:  "remove blindness unlearned cleric",
			spell: "개안술",
			class: model.ClassCleric,
			want:  "당신은 아직 그런 주문을 터득하지 못했습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := castWorld(t, "room:dojo", 12)
			actor := loaded.Creatures["creature:alice"]
			actor.Stats["class"] = tt.class
			actor.Metadata.Tags = tt.tags
			loaded.Creatures[actor.ID] = actor
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{tt.spell}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			updated, _ := runtime.Creature("creature:alice")
			if got := updated.Stats["mpCurrent"]; got != 12 {
				t.Fatalf("mpCurrent = %d, want unchanged 12", got)
			}
		})
	}
}

func TestCastHandlerRemoveDiseaseSpellFailConsumesMPSilently(t *testing.T) {
	useSpellFailRoll(t, 99)
	loaded := castWorld(t, "room:dojo", 12)
	actor := loaded.Creatures["creature:alice"]
	actor.Metadata.Tags = []string{"SRMDIS", "PDISEA"}
	loaded.Creatures[actor.ID] = actor
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"PDISEA"}
	loaded.Players[player.ID] = player
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"치료"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want silent C spell_fail", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after spell_fail cost", got)
	}
	if !hasAnyNormalizedFlag(updated.Metadata.Tags, "PDISEA", "disease", "diseased") {
		t.Fatalf("creature tags = %+v, want disease unchanged", updated.Metadata.Tags)
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if !hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "PDISEA", "disease", "diseased") {
		t.Fatalf("player tags = %+v, want disease unchanged", updatedPlayer.Metadata.Tags)
	}
}

func TestCastHandlerRemoveDiseaseSuccessUsesEffectOwnedCost(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := castWorld(t, "room:dojo", 12)
	actor := loaded.Creatures["creature:alice"]
	actor.Stats["class"] = model.ClassCleric
	actor.Metadata.Tags = []string{"SRMDIS", "PDISEA"}
	loaded.Creatures[actor.ID] = actor
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"PDISEA"}
	loaded.Players[player.ID] = player
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"치료"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after one C remove disease cost", got)
	}
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "PDISEA", "disease", "diseased") {
		t.Fatalf("creature tags = %+v, want disease removed", updated.Metadata.Tags)
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "PDISEA", "disease", "diseased") {
		t.Fatalf("player tags = %+v, want disease removed", updatedPlayer.Metadata.Tags)
	}
}

func TestCastHandlerRemoveBlindnessSpellFailConsumesMPSilently(t *testing.T) {
	useSpellFailRoll(t, 99)
	loaded := castWorld(t, "room:dojo", 12)
	actor := loaded.Creatures["creature:alice"]
	actor.Metadata.Tags = []string{"SRMBLD"}
	loaded.Creatures[actor.ID] = actor
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindNPC,
		DisplayName: "Bob",
		RoomID:      "room:dojo",
		Metadata:    model.Metadata{Tags: []string{"PBLIND"}},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"개안술", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want silent C spell_fail", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after spell_fail cost", got)
	}
	bob, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bob.Metadata.Tags, "PBLIND", "blind", "blinded") {
		t.Fatalf("Bob tags = %+v, want blindness unchanged", bob.Metadata.Tags)
	}
}

func TestCastHandlerRemoveBlindnessSuccessUsesEffectOwnedCost(t *testing.T) {
	useSpellFailRoll(t, 0)
	loaded := castWorld(t, "room:dojo", 12)
	actor := loaded.Creatures["creature:alice"]
	actor.Stats["class"] = model.ClassCleric
	actor.Metadata.Tags = []string{"SRMBLD"}
	loaded.Creatures[actor.ID] = actor
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"개안술"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after one C remove blindness cost", got)
	}
}

func TestCastHandlerDMSkipsSpellCooldownCheck(t *testing.T) {
	loaded := castWorld(t, "room:dojo", 20)
	creature := loaded.Creatures["creature:alice"]
	creature.Stats["class"] = model.ClassDM
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)

	fakeTime := int64(1000)
	previousTimeNow := timeNow
	timeNow = func() time.Time {
		return time.Unix(fakeTime, 0)
	}
	t.Cleanup(func() {
		timeNow = previousTimeNow
	})

	calls := 0
	handler := NewCastHandler(runtime, func(*Context, CastWorld, model.Creature, ResolvedCommand, int) (bool, error) {
		calls++
		return true, nil
	})

	if _, err := handler(&Context{ActorID: "player:alice"}, ResolvedCommand{Args: []string{"회복"}}); err != nil {
		t.Fatalf("first cast error = %v", err)
	}
	ctx := &Context{ActorID: "player:alice"}
	if _, err := handler(ctx, ResolvedCommand{Args: []string{"회복"}}); err != nil {
		t.Fatalf("second cast error = %v", err)
	}
	if strings.Contains(ctx.OutputString(), "기다리세요") {
		t.Fatalf("second cast output = %q, want no cooldown wait", ctx.OutputString())
	}
	if calls != 2 {
		t.Fatalf("effect calls = %d, want 2", calls)
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 10 {
		t.Fatalf("mpCurrent = %d, want 10 after two casts", got)
	}
	remaining, ready, err := runtime.UseCreatureCooldown("creature:alice", "spell", fakeTime, 0)
	if err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	}
	if ready || remaining <= 0 {
		t.Fatalf("cooldown ready/remaining = %v/%d, want cooldown still recorded", ready, remaining)
	}
}

func TestCastHandlerDoesNotConsumeMPWhenEffectDoesNotApply(t *testing.T) {
	loaded := castWorld(t, "room:dojo", 20)
	actor := loaded.Creatures["creature:alice"]
	actor.Metadata.Tags = append(actor.Metadata.Tags, "hidden")
	loaded.Creatures[actor.ID] = actor
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden"}
	loaded.Players[player.ID] = player

	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"회복", "없는대상"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "그런 사람은 존재하지 않습니다.\n" {
		t.Fatalf("status/output = %d/%q, want '그런 사람은 존재하지 않습니다.\\n'", status, ctx.OutputString())
	}
	creature, _ := runtime.Creature("creature:alice")
	if got := creature.Stats["mpCurrent"]; got != 20 {
		t.Fatalf("mpCurrent = %d, want unchanged 20", got)
	}
}

func TestCastHandlerRemoveCurseUsesCLegacyCost(t *testing.T) {
	loaded := castWorld(t, "room:dojo", 18)
	actor := loaded.Creatures["creature:alice"]
	actor.Metadata.Tags = append(actor.Metadata.Tags, "SREMOV")
	actor.Stats["class"] = model.ClassDM
	loaded.Creatures[actor.ID] = actor

	weaponID := model.ObjectInstanceID("object:cursed_sword")
	loaded.Objects[weaponID] = model.ObjectInstance{
		ID:                  weaponID,
		DisplayNameOverride: "저주받은 검",
		Location:            model.ObjectLocation{CreatureID: actor.ID, Slot: "wield"},
		Metadata:            model.Metadata{Tags: []string{"cursed", "ocurse"}},
	}
	actor.Equipment = map[string]model.ObjectInstanceID{"wield": weaponID}
	loaded.Creatures[actor.ID] = actor

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"저주해소"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	updated, _ := runtime.Creature(actor.ID)
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after C 18 MP remove-curse cost", got)
	}
	object, _ := runtime.Object(weaponID)
	if objectHasAnyTag(runtime, object, "cursed", "ocurse") {
		t.Fatalf("object tags = %+v, want curse removed", object.Metadata.Tags)
	}
}

func TestCastHandlerRoomVigorUsesCLegacyCost(t *testing.T) {
	loaded := castWorld(t, "room:dojo", 12)
	room := loaded.Rooms["room:dojo"]
	room.PlayerIDs = []model.PlayerID{"player:alice"}
	loaded.Rooms[room.ID] = room
	actor := loaded.Creatures["creature:alice"]
	actor.Level = 100
	actor.Metadata.Tags = append(actor.Metadata.Tags, "SRVIGO")
	actor.Stats["class"] = model.ClassCleric
	actor.Stats["level"] = 100
	actor.Stats["intelligence"] = 30
	actor.Stats["piety"] = 30
	actor.Stats["hpCurrent"] = 5
	actor.Stats["hpMax"] = 20
	loaded.Creatures[actor.ID] = actor

	runtime := state.NewWorld(loaded)
	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"전회복"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if !strings.Contains(ctx.OutputString(), "당신은 가부좌를 틀고서 전회복 주문을 외웁니다.") {
		t.Fatalf("output = %q, want room vigor message", ctx.OutputString())
	}
	updated, _ := runtime.Creature(actor.ID)
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after C 12 MP room-vigor cost", got)
	}
}

func TestCastHandlerRoomVigorSpellFailConsumesPrepaidCost(t *testing.T) {
	useSpellFailRoll(t, 99)
	loaded := castWorld(t, "room:dojo", 12)
	room := loaded.Rooms["room:dojo"]
	room.PlayerIDs = []model.PlayerID{"player:alice"}
	loaded.Rooms[room.ID] = room
	actor := loaded.Creatures["creature:alice"]
	actor.Metadata.Tags = append(actor.Metadata.Tags, "SRVIGO")
	actor.Stats["class"] = model.ClassCleric
	actor.Stats["mpCurrent"] = 12
	actor.Stats["hpCurrent"] = 5
	actor.Stats["hpMax"] = 20
	loaded.Creatures[actor.ID] = actor
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"전회복"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want silent C room_vigor spell_fail", status, ctx.OutputString())
	}
	updated, _ := runtime.Creature(actor.ID)
	if got := updated.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0 after C pre-spell_fail room-vigor cost", got)
	}
	if got := updated.Stats["hpCurrent"]; got != 5 {
		t.Fatalf("hpCurrent = %d, want unchanged after spell_fail", got)
	}
}

func TestCastHandlerRoomVigorUsesCLegacyGateOrder(t *testing.T) {
	tests := []struct {
		name  string
		mp    int
		class int
		tags  []string
		want  string
	}{
		{
			name:  "unlearned before mp and class",
			mp:    11,
			class: model.ClassFighter,
			want:  "당신은 아직 그런 주문을 터득하지 못했습니다.",
		},
		{
			name:  "class before mp",
			mp:    11,
			class: model.ClassFighter,
			tags:  []string{"SRVIGO"},
			want:  "이 주술은 불제자만이 사용할 수 있습니다.",
		},
		{
			name:  "mp after learned and class",
			mp:    11,
			class: model.ClassCleric,
			tags:  []string{"SRVIGO"},
			want:  "당신의 도력이 부족합니다",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := castWorld(t, "room:dojo", tt.mp)
			actor := loaded.Creatures["creature:alice"]
			actor.Stats["class"] = tt.class
			actor.Metadata.Tags = tt.tags
			loaded.Creatures[actor.ID] = actor
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{"전회복"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			updated, _ := runtime.Creature("creature:alice")
			if got := updated.Stats["mpCurrent"]; got != tt.mp {
				t.Fatalf("mpCurrent = %d, want unchanged %d", got, tt.mp)
			}
		})
	}
}

func TestCastHandlerCurseSpellFailConsumesCMPWithoutMutatingEquipment(t *testing.T) {
	tests := []struct {
		name       string
		spell      string
		tag        string
		mp         int
		wantMP     int
		startCurse bool
		wantCurse  bool
	}{
		{name: "remove curse fail", spell: "저주해소", tag: "SREMOV", mp: 18, wantMP: 0, startCurse: true, wantCurse: true},
		{name: "curse fail", spell: "저주", tag: "SCURSE", mp: 25, wantMP: 0, startCurse: false, wantCurse: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := castWorld(t, "room:dojo", tt.mp)
			actor := loaded.Creatures["creature:alice"]
			actor.Level = 1
			actor.Metadata.Tags = []string{tt.tag}
			actor.Stats["class"] = model.ClassBarbarian
			actor.Stats["level"] = 1
			actor.Stats["intelligence"] = 0

			weaponID := model.ObjectInstanceID("object:sword")
			tags := []string{}
			if tt.startCurse {
				tags = []string{"cursed", "ocurse"}
			}
			loaded.Objects[weaponID] = model.ObjectInstance{
				ID:                  weaponID,
				DisplayNameOverride: "장검",
				Location:            model.ObjectLocation{CreatureID: actor.ID, Slot: "wield"},
				Metadata:            model.Metadata{Tags: tags},
			}
			actor.Equipment = map[string]model.ObjectInstanceID{"wield": weaponID}
			loaded.Creatures[actor.ID] = actor

			runtime := state.NewWorld(loaded)
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewCastHandler(runtime, nil)(ctx, ResolvedCommand{Args: []string{tt.spell}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}
			if ctx.OutputString() != "" {
				t.Fatalf("output = %q, want silent C spell failure", ctx.OutputString())
			}
			updated, _ := runtime.Creature(actor.ID)
			if got := updated.Stats["mpCurrent"]; got != tt.wantMP {
				t.Fatalf("mpCurrent = %d, want %d", got, tt.wantMP)
			}
			object, _ := runtime.Object(weaponID)
			hasCurse := objectHasAnyTag(runtime, object, "cursed", "ocurse")
			if hasCurse != tt.wantCurse {
				t.Fatalf("object cursed = %v, want %v; tags=%+v", hasCurse, tt.wantCurse, object.Metadata.Tags)
			}
		})
	}
}

func TestCastHandlerDispatchesKoreanAndEnglishAliases(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
	}{
		{name: "korean verb final", input: "주문감지술 주문"},
		{name: "english command first", input: "cast 주문감지술"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runtime := state.NewWorld(castWorld(t, "room:dojo", 20))
			dispatcher := Dispatcher{
				Registry: mustRegistry(t, []commandspec.CommandSpec{
					{Name: "주문", Number: 38, Handler: "cast"},
					{Name: "cast", Number: 38, Handler: "cast"},
				}),
				Handlers: map[string]Handler{
					"cast": NewCastHandler(runtime, nil),
				},
			}

			ctx := &Context{ActorID: "player:alice"}
			status, err := dispatcher.DispatchLine(ctx, tt.input)
			if err != nil {
				t.Fatalf("DispatchLine() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want StatusDefault", status)
			}
			creature, _ := runtime.Creature("creature:alice")
			if !hasAnyNormalizedFlag(creature.Metadata.Tags, "detectMagic", "pdmagi") {
				t.Fatalf("creature tags = %+v, want detectMagic", creature.Metadata.Tags)
			}
		})
	}
}

func castWorld(t *testing.T, roomID model.RoomID, mp int) *worldload.World {
	t.Helper()

	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{ID: roomID, DisplayName: "수련장"})
	player := loaded.Players["player:alice"]
	player.RoomID = roomID
	loaded.Players[player.ID] = player
	creature := loaded.Creatures["creature:alice"]
	creature.RoomID = roomID
	creature.Metadata.Tags = []string{"SVIGOR", "SDETEC"}
	creature.Stats = map[string]int{
		"class":     model.ClassCleric,
		"hpCurrent": 10,
		"hpMax":     20,
		"mpCurrent": mp,
		"mpMax":     20,
	}
	loaded.Creatures[creature.ID] = creature
	return loaded
}

func useMaxMagicEffectRoll(t *testing.T) {
	t.Helper()
	previous := attackRoll
	attackRoll = func(min, max int) int {
		if max < min {
			return min
		}
		return max
	}
	t.Cleanup(func() {
		attackRoll = previous
	})
}

func useSpellFailRoll(t *testing.T, roll int) {
	t.Helper()
	previous := spellFailRandIntn
	spellFailRandIntn = func(n int) int {
		if roll < 0 {
			return 0
		}
		if roll >= n {
			return n - 1
		}
		return roll
	}
	t.Cleanup(func() {
		spellFailRandIntn = previous
	})
}
