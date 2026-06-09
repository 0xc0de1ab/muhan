package command

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"muhan/internal/commandspec"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestReadScrollHandlerConsumesScrollOnSuccess(t *testing.T) {
	runtime := state.NewWorld(readScrollWorld(t, "room:library", "1", "4"))

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	want := "당신은 오른손으로 혈도를 짚으면서 해독 주문을 외웁니다.\n" +
		"손가락 끝으로 검은 독기운이 빠져나오는것이 보입니다.\n" +
		"당신 몸에 남아 있는 독이 모두 빠져나갔습니다.\n" +
		"주문이 번쩍인다.\n\n" +
		"모든 것을 읽고 나자 귀환 주문서의 형체가 먼지로 변하면서 바람과 함께 사라져 버렸습니다.\n"
	if ctx.OutputString() != want {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), want)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after successful read")
	}
	creature, _ := runtime.Creature("creature:alice")
	for _, id := range creature.Inventory.ObjectIDs {
		if id == "object:scroll" {
			t.Fatalf("inventory still contains consumed scroll: %+v", creature.Inventory.ObjectIDs)
		}
	}
}

func TestReadScrollHandlerUsesLegacyReadCooldown(t *testing.T) {
	t.Run("success records LT_READS cooldown", func(t *testing.T) {
		withFakeMagicEffectTime(t, 1000)
		runtime := state.NewWorld(readScrollWorld(t, "room:library", "1", "4"))

		ctx := &Context{ActorID: "player:alice"}
		status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환"}})
		if err != nil {
			t.Fatalf("handler() error = %v", err)
		}
		if status != StatusDefault {
			t.Fatalf("status = %d, want StatusDefault", status)
		}
		remaining, ready, err := runtime.UseCreatureCooldown("creature:alice", readScrollCooldownKey, 1001, 0)
		if err != nil {
			t.Fatalf("UseCreatureCooldown() error = %v", err)
		}
		if ready || remaining != 2 {
			t.Fatalf("cooldown ready/remaining = %v/%d, want active 2 seconds", ready, remaining)
		}
	})

	t.Run("active cooldown rejects before hidden clear effect and consume", func(t *testing.T) {
		withFakeMagicEffectTime(t, 2000)
		loaded := readScrollWorld(t, "room:library", "1", "4")
		creature := loaded.Creatures["creature:alice"]
		creature.Metadata.Tags = append(creature.Metadata.Tags, "hidden", "PHIDDN")
		creature.Stats["PHIDDN"] = 1
		loaded.Creatures[creature.ID] = creature
		player := loaded.Players["player:alice"]
		player.Metadata.Tags = append(player.Metadata.Tags, "hidden", "PHIDDN")
		loaded.Players[player.ID] = player
		runtime := state.NewWorld(loaded)
		if err := runtime.SetCreatureCooldown("creature:alice", readScrollCooldownKey, 2000, readScrollCooldownSeconds); err != nil {
			t.Fatalf("SetCreatureCooldown() error = %v", err)
		}

		called := false
		ctx := &Context{ActorID: "player:alice"}
		status, err := NewReadScrollHandler(runtime, "", func(*Context, ReadScrollWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error) {
			called = true
			return true, nil
		})(ctx, ResolvedCommand{Args: []string{"귀환"}})
		if err != nil {
			t.Fatalf("handler() error = %v", err)
		}
		if status != StatusDefault || ctx.OutputString() != "3초동안 기다리세요.\n" {
			t.Fatalf("status/output = %d/%q, want wait", status, ctx.OutputString())
		}
		if called {
			t.Fatal("effect was called while read cooldown was active")
		}
		if _, ok := runtime.Object("object:scroll"); !ok {
			t.Fatal("scroll was consumed while read cooldown was active")
		}
		updatedCreature, _ := runtime.Creature("creature:alice")
		if !hasAnyNormalizedFlag(updatedCreature.Metadata.Tags, "hidden", "phiddn") {
			t.Fatalf("creature tags = %+v, want hidden retained", updatedCreature.Metadata.Tags)
		}
		if updatedCreature.Stats["PHIDDN"] != 1 {
			t.Fatalf("creature PHIDDN = %d, want retained", updatedCreature.Stats["PHIDDN"])
		}
		updatedPlayer, _ := runtime.Player("player:alice")
		if !hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "hidden", "phiddn") {
			t.Fatalf("player tags = %+v, want hidden retained", updatedPlayer.Metadata.Tags)
		}
	})
}

func TestReadScrollHandlerSpellFailConsumesScrollLikeC(t *testing.T) {
	useSpellFailRoll(t, 99)
	loaded := readScrollWorld(t, "room:library", "1", "3")
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"hidden", "PHIDDN"}
	creature.Stats["PHIDDN"] = 1
	loaded.Creatures[creature.ID] = creature
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Players[player.ID] = player
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	want := "\n모든 것을 읽고 나자 귀환 주문서의 형체가 먼지로 변하면서 바람과 함께 사라져 버렸습니다.\n"
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want C spell_fail dust", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after C read spell_fail")
	}
	updated, _ := runtime.Creature("creature:alice")
	if magicEffectTestHasExactTag(updated.Metadata.Tags, "PLIGHT") {
		t.Fatalf("creature tags = %+v, want spell effect skipped", updated.Metadata.Tags)
	}
	if hasAnyNormalizedFlag(updated.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("creature tags = %+v, want hidden cleared before spell_fail", updated.Metadata.Tags)
	}
	if updated.Stats["PHIDDN"] != 0 {
		t.Fatalf("creature PHIDDN = %d, want 0", updated.Stats["PHIDDN"])
	}
	updatedPlayer, _ := runtime.Player("player:alice")
	if hasAnyNormalizedFlag(updatedPlayer.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("player tags = %+v, want hidden cleared before spell_fail", updatedPlayer.Metadata.Tags)
	}
}

func TestReadScrollHandlerRejectsInvalidTargets(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		shots      string
		magicPower string
		want       string
	}{
		{name: "missing target", want: "무엇을 읽습니까?\n"},
		{name: "missing object", args: []string{"없는"}, shots: "1", magicPower: "4", want: "\n 그런것이 존재하지 않습니다.\n"},
		{name: "non scroll", args: []string{"돌"}, shots: "1", magicPower: "4", want: "\n이것은 문서구가 아닙니다.\n"},
		{name: "empty magic", args: []string{"귀환"}, shots: "1", magicPower: "0", want: "\n아무런 일도 일어나지 않았습니다.\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := state.NewWorld(readScrollWorld(t, "room:library", tt.shots, tt.magicPower))
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			if _, ok := runtime.Object("object:scroll"); !ok {
				t.Fatal("scroll was consumed on rejected read")
			}
		})
	}
}

func TestReadScrollHandlerIgnoresShotsCurrentLikeC(t *testing.T) {
	runtime := state.NewWorld(readScrollWorld(t, "room:library", "0", "4"))
	called := false
	ctx := &Context{ActorID: "player:alice"}

	status, err := NewReadScrollHandler(runtime, "", func(*Context, ReadScrollWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error) {
		called = true
		return true, nil
	})(ctx, ResolvedCommand{Args: []string{"귀환"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	want := "주문이 번쩍인다.\n\n" +
		"모든 것을 읽고 나자 귀환 주문서의 형체가 먼지로 변하면서 바람과 함께 사라져 버렸습니다.\n"
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want C shotscur-ignored read", status, ctx.OutputString())
	}
	if !called {
		t.Fatal("effect was not called for shotsCurrent=0 scroll")
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after C-style read with shotsCurrent=0")
	}
}

func TestReadScrollHandlerRoomAndActorRestrictionsDoNotConsume(t *testing.T) {
	tests := []struct {
		name       string
		roomTags   []string
		playerTags []string
		want       string
	}{
		{name: "no magic", roomTags: []string{"noMagic"}, want: "\n아무런 일도 일어나지 않았습니다.\n"},
		{name: "blind", playerTags: []string{"blind"}, want: "\n당신은 그것을 읽을 수 있는 능력이 없습니다.\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", "4")
			room := loaded.Rooms["room:library"]
			room.Metadata.Tags = tt.roomTags
			loaded.Rooms[room.ID] = room
			player := loaded.Players["player:alice"]
			player.Metadata.Tags = tt.playerTags
			loaded.Players[player.ID] = player
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			if _, ok := runtime.Object("object:scroll"); !ok {
				t.Fatal("scroll was consumed despite restriction")
			}
		})
	}
}

func TestReadScrollHandlerAppliesMagicItemRestrictions(t *testing.T) {
	tests := []struct {
		name          string
		creatureStats map[string]int
		objectTags    []string
		objectProps   map[string]string
		protoProps    map[string]string
		want          string
		wantDropped   bool
	}{
		{
			name:          "good only rejects evil actor and drops scroll",
			creatureStats: map[string]int{"alignment": -101, "level": 20, "class": legacyClassFighter},
			objectTags:    []string{"goodOnly"},
			want:          "\n모든 것을 읽고 나자 귀환 주문서의 형체가 먼지로 변하면서 바람과 함께 사라져 버렸습니다.\n",
			wantDropped:   true,
		},
		{
			name:          "class selective rejects unlisted class",
			creatureStats: map[string]int{"level": 20, "class": legacyClassFighter},
			protoProps:    map[string]string{"classSelective": "1", "classMage": "1"},
			want:          "\n이것은 당신의 직업에서 금하는 금서이기 때문에 내용을 읽을 수 없습니다.\n",
		},
		{
			name:          "ndice above actor level rejects scroll",
			creatureStats: map[string]int{"level": 4, "class": legacyClassFighter},
			objectProps:   map[string]string{"nDice": "5"},
			want:          "\n당신의 능력으로는 귀환 주문서의 내용을 파악하지 못해 연마할 수 없습니다.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", "4")
			creature := loaded.Creatures["creature:alice"]
			creature.Stats = tt.creatureStats
			loaded.Creatures[creature.ID] = creature
			proto := loaded.ObjectPrototypes["prototype:scroll"]
			proto.Properties = tt.protoProps
			loaded.ObjectPrototypes[proto.ID] = proto
			scroll := loaded.Objects["object:scroll"]
			scroll.Metadata.Tags = tt.objectTags
			for key, value := range tt.objectProps {
				scroll.Properties[key] = value
			}
			loaded.Objects[scroll.ID] = scroll
			runtime := state.NewWorld(loaded)

			called := false
			ctx := &Context{ActorID: "player:alice"}
			status, err := NewReadScrollHandler(runtime, "", func(*Context, ReadScrollWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error) {
				called = true
				return true, nil
			})(ctx, ResolvedCommand{Args: []string{"귀환"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			if called {
				t.Fatal("effect was called despite restriction")
			}
			scroll, ok := runtime.Object("object:scroll")
			if !ok {
				t.Fatal("scroll was consumed despite restriction")
			}
			if tt.wantDropped {
				if scroll.Location.RoomID != "room:library" {
					t.Fatalf("scroll location = %+v, want room:library", scroll.Location)
				}
			} else if scroll.Location.CreatureID != "creature:alice" {
				t.Fatalf("scroll location = %+v, want creature inventory", scroll.Location)
			}
		})
	}
}

func TestReadScrollHandlerDoesNotConsumeWhenEffectFails(t *testing.T) {
	runtime := state.NewWorld(readScrollWorld(t, "room:library", "1", "4"))
	effect := func(*Context, ReadScrollWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error) {
		return false, nil
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", effect)(ctx, ResolvedCommand{Args: []string{"귀환"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want no success output", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("scroll was consumed after failed effect")
	}
}

func TestReadScrollHandlerReadsSpecialMapScrollFile(t *testing.T) {
	root := t.TempDir()
	writeSpecialMapScrollFixture(t, root, "고대_지도", "북쪽으로 길이 이어진다.")
	loaded := readScrollWorld(t, "room:library", "1", "4")
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:map")
	loaded.Creatures[creature.ID] = creature
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:map",
		Kind:        model.ObjectKindMisc,
		DisplayName: "고대 지도",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:map",
		PrototypeID: "prototype:map",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"special": "1"},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, root, func(*Context, ReadScrollWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error) {
		t.Fatal("special map scroll used generic magic effect")
		return true, nil
	})(ctx, ResolvedCommand{Args: []string{"고대"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	want := "북쪽으로 길이 이어진다.\n"
	if status != StatusDoPrompt || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
	if _, ok := runtime.Object("object:map"); !ok {
		t.Fatal("special map scroll was consumed")
	}
}

func TestReadScrollHandlerMissingSpecialMapScrollFileReturnsDoPromptLikeViewFile(t *testing.T) {
	root := t.TempDir()
	loaded := readScrollWorld(t, "room:library", "1", "4")
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:map")
	loaded.Creatures[creature.ID] = creature
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:map",
		Kind:        model.ObjectKindMisc,
		DisplayName: "고대 지도",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:map",
		PrototypeID: "prototype:map",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"special": "1"},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, root, func(*Context, ReadScrollWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error) {
		t.Fatal("special map scroll used generic magic effect")
		return true, nil
	})(ctx, ResolvedCommand{Args: []string{"고대"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDoPrompt || ctx.OutputString() != "화일을 읽을 수 없습니다.\n" {
		t.Fatalf("status/output = %d/%q, want C view_file missing-file surface", status, ctx.OutputString())
	}
	if _, ok := runtime.Object("object:map"); !ok {
		t.Fatal("special map scroll was consumed")
	}
}

func TestReadScrollHandlerDoesNotReadRoomSpecialMapScrollLikeLegacy(t *testing.T) {
	root := t.TempDir()
	writeSpecialMapScrollFixture(t, root, "벽_지도", "방 한가운데에 오래된 표식이 있다.")
	loaded := readScrollWorld(t, "room:library", "1", "4")
	room := loaded.Rooms["room:library"]
	room.Objects.ObjectIDs = append(room.Objects.ObjectIDs, "object:room-map")
	loaded.Rooms[room.ID] = room
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:room-map",
		Kind:        model.ObjectKindMisc,
		DisplayName: "벽 지도",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:room-map",
		PrototypeID: "prototype:room-map",
		Location:    model.ObjectLocation{RoomID: "room:library"},
		Properties:  map[string]string{"special": "SP_MAPSC"},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, root, func(*Context, ReadScrollWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error) {
		t.Fatal("room special map scroll used generic magic effect")
		return true, nil
	})(ctx, ResolvedCommand{Args: []string{"벽"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	want := "\n 그런것이 존재하지 않습니다.\n"
	if status != StatusDefault || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
	if _, ok := runtime.Object("object:room-map"); !ok {
		t.Fatal("room special map scroll was consumed")
	}
}

func TestReadScrollHandlerDoesNotReadSpecialMapScrollHelpFallbackLikeLegacy(t *testing.T) {
	root := t.TempDir()

	dir := filepath.Join(root, "help")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "고대_지도"), []byte("Go-only fallback should not load"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	loaded := readScrollWorld(t, "room:library", "1", "4")
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:map")
	loaded.Creatures[creature.ID] = creature
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:map",
		Kind:        model.ObjectKindMisc,
		DisplayName: "고대 지도",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:map",
		PrototypeID: "prototype:map",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"special": "1"},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, root, func(*Context, ReadScrollWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error) {
		t.Fatal("special map scroll used generic magic effect")
		return true, nil
	})(ctx, ResolvedCommand{Args: []string{"고대"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	want := "화일을 읽을 수 없습니다.\n"
	if status != StatusDoPrompt || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
}

func TestReadScrollHandlerReadsSpecialMapScrollFromLegacyObjpath(t *testing.T) {
	root := t.TempDir()

	writeSpecialMapScrollFixture(t, root, "고대_지도", "OBJPATH 로딩 성공")

	loaded := readScrollWorld(t, "room:library", "1", "4")
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:map")
	loaded.Creatures[creature.ID] = creature
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:map",
		Kind:        model.ObjectKindMisc,
		DisplayName: "고대 지도",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:map",
		PrototypeID: "prototype:map",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"special": "1"},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, root, func(*Context, ReadScrollWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error) {
		t.Fatal("special map scroll used generic magic effect")
		return true, nil
	})(ctx, ResolvedCommand{Args: []string{"고대"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	want := "OBJPATH 로딩 성공\n"
	if status != StatusDoPrompt || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
}

func TestReadScrollHandlerOnlyNormalizesAsciiSpaceInSpecialMapNameLikeC(t *testing.T) {
	root := t.TempDir()
	writeSpecialMapScrollFixture(t, root, "고대_지도", "space path")

	loaded := readScrollWorld(t, "room:library", "1", "4")
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:map")
	loaded.Creatures[creature.ID] = creature
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:map",
		Kind:        model.ObjectKindMisc,
		DisplayName: "고대\t지도",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:map",
		PrototypeID: "prototype:map",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"special": "1"},
	})
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, root, nil)(ctx, ResolvedCommand{Args: []string{"고대"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	want := "화일을 읽을 수 없습니다.\n"
	if status != StatusDoPrompt || ctx.OutputString() != want {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), want)
	}
}

func TestReadScrollHandlerPaginatesLongSpecialMapScrollFile(t *testing.T) {
	root := t.TempDir()
	var builder strings.Builder
	for i := 1; i <= 25; i++ {
		builder.WriteString("지도 줄 ")
		builder.WriteString(strconv.Itoa(i))
		builder.WriteByte('\n')
	}
	writeSpecialMapScrollFixture(t, root, "고대_지도", builder.String())
	loaded := readScrollWorld(t, "room:library", "1", "4")
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:map")
	loaded.Creatures[creature.ID] = creature
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:map",
		Kind:        model.ObjectKindMisc,
		DisplayName: "고대 지도",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:map",
		PrototypeID: "prototype:map",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"special": "1"},
	})
	runtime := state.NewWorld(loaded)

	var pending PendingLineHandler
	ctx := readScrollTestContext(&pending)
	status, err := NewReadScrollHandler(runtime, root, nil)(ctx, ResolvedCommand{Args: []string{"고대"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDoPrompt || pending == nil {
		t.Fatalf("status/pending = %d/%v, want paged read", status, pending != nil)
	}
	got := ctx.OutputString()
	if !strings.Contains(got, "지도 줄 19\n") || strings.Contains(got, "지도 줄 20\n") {
		t.Fatalf("first page output = %q", got)
	}
	if !strings.HasSuffix(got, postReadContinuePrompt) {
		t.Fatalf("first page prompt = %q", got)
	}

	ctx.Output = nil
	status, err = pending(ctx, "")
	if err != nil {
		t.Fatalf("continue pending line error = %v", err)
	}
	got = ctx.OutputString()
	if status != StatusDefault || !strings.Contains(got, "지도 줄 20\n") || !strings.Contains(got, "지도 줄 25\n") || strings.Contains(got, postReadContinuePrompt) {
		t.Fatalf("second page status/output = %d/%q", status, got)
	}
	if pending != nil {
		t.Fatal("pending handler was not cleared")
	}
}

func TestReadScrollHandlerCancelsSpecialMapScrollPagination(t *testing.T) {
	root := t.TempDir()
	writeSpecialMapScrollFixture(t, root, "고대_지도", strings.Repeat("긴 지도\n", 25))
	loaded := readScrollWorld(t, "room:library", "1", "4")
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = append(creature.Inventory.ObjectIDs, "object:map")
	loaded.Creatures[creature.ID] = creature
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:map",
		Kind:        model.ObjectKindMisc,
		DisplayName: "고대 지도",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:map",
		PrototypeID: "prototype:map",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties:  map[string]string{"special": "1"},
	})
	runtime := state.NewWorld(loaded)

	var pending PendingLineHandler
	ctx := readScrollTestContext(&pending)
	status, err := NewReadScrollHandler(runtime, root, nil)(ctx, ResolvedCommand{Args: []string{"고대"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDoPrompt || pending == nil {
		t.Fatalf("status/pending = %d/%v, want paged read", status, pending != nil)
	}

	ctx.Output = nil
	status, err = pending(ctx, ".")
	if err != nil {
		t.Fatalf("cancel pending line error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "중단합니다.\n" {
		t.Fatalf("cancel status/output = %d/%q", status, ctx.OutputString())
	}
	if pending != nil {
		t.Fatal("pending handler was not cleared")
	}
}

func TestReadScrollHandlerLetsSpecialComboFallThroughToScrollRouting(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "4")
	scroll := loaded.Objects["object:scroll"]
	scroll.Properties["special"] = "SP_COMBO"
	loaded.Objects[scroll.ID] = scroll
	runtime := state.NewWorld(loaded)

	called := false
	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", func(*Context, ReadScrollWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error) {
		called = true
		return false, nil
	})(ctx, ResolvedCommand{Args: []string{"귀환"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want generic scroll routing with no effect output", status, ctx.OutputString())
	}
	if !called {
		t.Fatal("SP_COMBO read did not fall through to generic scroll effect")
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("special combo scroll was consumed after failed effect")
	}
}

func TestReadScrollHandlerLetsSpecialWarFallThroughToScrollRouting(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "4")
	scroll := loaded.Objects["object:scroll"]
	scroll.Properties["special"] = "SP_WAR"
	loaded.Objects[scroll.ID] = scroll
	runtime := state.NewWorld(loaded)

	called := false
	ctx := &Context{ActorID: "player:alice"}
	status, err := NewReadScrollHandler(runtime, "", func(*Context, ReadScrollWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error) {
		called = true
		return false, nil
	})(ctx, ResolvedCommand{Args: []string{"귀환"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "" {
		t.Fatalf("status/output = %d/%q, want generic scroll routing with no effect output", status, ctx.OutputString())
	}
	if !called {
		t.Fatal("SP_WAR read did not fall through to generic scroll effect")
	}
	if _, ok := runtime.Object("object:scroll"); !ok {
		t.Fatal("special war scroll was consumed after failed effect")
	}
}

func TestReadScrollHandlerRoutesBareReadToRoomBoard(t *testing.T) {
	root := boardTestRoot(t)
	world := state.NewWorld(boardTestWorld(t, true))
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "게시판", Number: 94, Handler: "look_board"},
			{Name: "읽어", Number: 40, Handler: "readscroll"},
		}),
		Handlers: map[string]Handler{
			"look_board": NewBoardLookHandler(world, root),
			"readscroll": NewReadScrollHandler(world, root, nil),
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	if _, err := dispatcher.DispatchLine(ctx, "읽어"); err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	for _, want := range []string{"번호 올린이", "2 무한", "둘째 공지", "1 운영자", "첫 공지"} {
		if got := ctx.OutputString(); !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func readScrollWorld(t *testing.T, roomID model.RoomID, shots string, magicPower string) *worldload.World {
	t.Helper()

	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{ID: roomID, DisplayName: "서재"})
	player := loaded.Players["player:alice"]
	player.RoomID = roomID
	loaded.Players[player.ID] = player
	creature := loaded.Creatures["creature:alice"]
	creature.RoomID = roomID
	creature.Inventory = model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:scroll", "object:stone"}}
	creature.Stats = map[string]int{
		"class":     legacyClassCleric,
		"hpCurrent": 50,
		"hpMax":     100,
		"mpCurrent": 100,
		"mpMax":     100,
	}
	loaded.Creatures[creature.ID] = creature
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:scroll",
		Kind:        model.ObjectKindScroll,
		DisplayName: "귀환 주문서",
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "prototype:stone",
		Kind:        model.ObjectKindMisc,
		DisplayName: "돌",
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:scroll",
		PrototypeID: "prototype:scroll",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Properties: map[string]string{
			"type":         "7",
			"shotsCurrent": shots,
			"magicPower":   magicPower,
			"useOutput":    "주문이 번쩍인다.",
		},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:stone",
		PrototypeID: "prototype:stone",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	})
	return loaded
}

func writeSpecialMapScrollFixture(t *testing.T, root string, name string, content string) {
	t.Helper()
	dir := filepath.Join(root, "objmon")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func readScrollTestContext(pending *PendingLineHandler) *Context {
	return &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			ContextPendingLineKey: func(handler PendingLineHandler) {
				*pending = handler
			},
		},
	}
}
