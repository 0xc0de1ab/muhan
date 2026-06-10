package command

import (
	"testing"

	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestStudyHandlerLearnsSpellAndConsumesScroll(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "4")
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN"}
	loaded.Players[player.ID] = player
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"hidden", "PHIDDN"}
	creature.Stats["PHIDDN"] = 1
	loaded.Creatures[creature.ID] = creature
	runtime := state.NewWorld(loaded)

	var roomBroadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &roomBroadcasts)
	status, err := NewStudyHandler(runtime)(ctx, ResolvedCommand{Args: []string{"귀환"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	want := "당신은 해독의 내용을 알아내고 연마하기 시작합니다.\n" +
		"연마를 해 나감에 따라 몸안에서 이상한 기운이 퍼져 나가는\n" +
		"것이 느껴집니다.\n" +
		"이야야~~~~~얍 그 기운이 안정되면서 완전히 당신의 것이 \n되었습니다.\n" +
		"\n연마를 마치자 귀환 주문서의 형체에 화염이 휩싸이며 어디론가 사라져 버렸습니다.\n"
	if ctx.OutputString() != want {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), want)
	}
	if len(roomBroadcasts) != 1 {
		t.Fatalf("len(roomBroadcasts) = %d, want 1", len(roomBroadcasts))
	}
	wantBroadcast := roomBroadcastRecord{
		RoomID:  "room:library",
		Exclude: "session:alice",
		Text:    "\nAlice가 귀환 주문서의 내용을 읽고 연마합니다.",
	}
	if roomBroadcasts[0] != wantBroadcast {
		t.Fatalf("roomBroadcast = %+v, want %+v", roomBroadcasts[0], wantBroadcast)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after successful study")
	}
	creature, _ = runtime.Creature("creature:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "SCUREP") {
		t.Fatalf("creature tags = %+v, want learned SCUREP", creature.Metadata.Tags)
	}
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("creature tags = %+v, want hidden cleared", creature.Metadata.Tags)
	}
	if creature.Stats["PHIDDN"] != 0 {
		t.Fatalf("creature PHIDDN = %d, want 0", creature.Stats["PHIDDN"])
	}
	player, _ = runtime.Player("player:alice")
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "SCUREP") {
		t.Fatalf("player tags = %+v, want learned SCUREP", player.Metadata.Tags)
	}
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("player tags = %+v, want hidden cleared", player.Metadata.Tags)
	}
}

func TestStudyHandlerRejectsInvalidTargets(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		playerTags []string
		creature   func(model.Creature) model.Creature
		magicPower string
		want       string
	}{
		{name: "missing target", magicPower: "4", want: "\n무엇을 연마하실려고요?\n"},
		{name: "missing object", args: []string{"없는"}, magicPower: "4", want: "\n그런 것을 소지하고 있지 않습니다.\n"},
		{name: "blind", args: []string{"귀환"}, playerTags: []string{"blind"}, magicPower: "4", want: "\n당신의 능력으로는 이 비법서를 연마할 수 없습니다.\n"},
		{name: "non scroll", args: []string{"돌"}, magicPower: "4", want: "\n이것은 비법서가 아닙니다.\n"},
		{
			name:       "already learned",
			args:       []string{"귀환"},
			magicPower: "4",
			creature: func(creature model.Creature) model.Creature {
				creature.Metadata.Tags = []string{"SCUREP"}
				return creature
			},
			want: "\n당신이 이미 터득한 주문서입니다.\n",
		},
		{name: "no spell", args: []string{"귀환"}, magicPower: "0", want: "\n이 비법서에는 배울 주문이 없습니다.\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", tt.magicPower)
			player := loaded.Players["player:alice"]
			player.Metadata.Tags = tt.playerTags
			loaded.Players[player.ID] = player
			if tt.creature != nil {
				creature := loaded.Creatures["creature:alice"]
				loaded.Creatures[creature.ID] = tt.creature(creature)
			}
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewStudyHandler(runtime)(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			if _, ok := runtime.Object("object:scroll"); !ok {
				t.Fatal("scroll was consumed on rejected study")
			}
		})
	}
}

func TestStudyHandlerAppliesMagicItemRestrictions(t *testing.T) {
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
			creatureStats: map[string]int{"alignment": -101, "level": 20, "class": model.ClassFighter},
			objectTags:    []string{"goodOnly"},
			want:          "\n연마를 끝마치자 귀환 주문서의 형체가 화염에 휩싸이며 어디론가 사라져\n 버렸습니다.\n",
			wantDropped:   true,
		},
		{
			name:          "class selective rejects unlisted class",
			creatureStats: map[string]int{"level": 20, "class": model.ClassFighter},
			protoProps:    map[string]string{"classSelective": "1", "classMage": "1"},
			want:          "\n당신의 직업상 귀환 주문서의 비법을 연마할 수 없습니다.\n",
		},
		{
			name:          "ndice above actor level rejects scroll",
			creatureStats: map[string]int{"level": 4, "class": model.ClassFighter},
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

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewStudyHandler(runtime)(ctx, ResolvedCommand{Args: []string{"귀환"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			creature, _ = runtime.Creature("creature:alice")
			if hasAnyNormalizedFlag(creature.Metadata.Tags, "SCUREP") {
				t.Fatalf("creature tags = %+v, want no learned spell", creature.Metadata.Tags)
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

func TestStudyHandlerConsumesEquippedScroll(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "4")
	creature := loaded.Creatures["creature:alice"]
	creature.Inventory.ObjectIDs = []model.ObjectInstanceID{"object:stone"}
	creature.Equipment = map[string]model.ObjectInstanceID{"held": "object:scroll"}
	loaded.Creatures[creature.ID] = creature
	scroll := loaded.Objects["object:scroll"]
	scroll.Location = model.ObjectLocation{CreatureID: "creature:alice", Slot: "held"}
	loaded.Objects[scroll.ID] = scroll
	runtime := state.NewWorld(loaded)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewStudyHandler(runtime)(ctx, ResolvedCommand{Args: []string{"귀환"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("equipped scroll still exists after successful study")
	}
	creature, _ = runtime.Creature("creature:alice")
	if _, ok := creature.Equipment["held"]; ok {
		t.Fatalf("equipment = %+v, want held slot cleared", creature.Equipment)
	}
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "SCUREP") {
		t.Fatalf("creature tags = %+v, want learned SCUREP", creature.Metadata.Tags)
	}
}
