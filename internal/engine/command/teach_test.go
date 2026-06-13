package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestTeachHandlerTeachesKnownSpellToRoomPlayer(t *testing.T) {
	loaded := teachWorld(t, model.ClassCleric, []string{"rcast"})
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"hidden", "PHIDDN", "SVIGOR"}
	alice.Stats["PHIDDN"] = 1
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"hidden", "PHIDDN", "SVIGOR"}
	loaded.Players[player.ID] = player
	runtime := state.NewWorld(loaded)

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := NewTeachHandler(runtime)(ctx, ResolvedCommand{Args: []string{"Bob", "회복"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}

	wantOutput := "\n회복 주술을 Bob에게 시범을 보이며 주문 전수를 시킵니다.\n" +
		"오옷~~~ 이 주문을 외우자 주위에 이상한 기운이 모이는 것이\n" +
		"그 사람에게 상당한 도움이 될 것 같습니다.\n"
	if ctx.OutputString() != wantOutput {
		t.Fatalf("output = %q, want %q", ctx.OutputString(), wantOutput)
	}

	bob, _ := runtime.Creature("creature:bob")
	if !hasAnyNormalizedFlag(bob.Metadata.Tags, "SVIGOR") {
		t.Fatalf("bob creature tags = %+v, want SVIGOR", bob.Metadata.Tags)
	}
	bobPlayer, _ := runtime.Player("player:bob")
	if !hasAnyNormalizedFlag(bobPlayer.Metadata.Tags, "SVIGOR") {
		t.Fatalf("bob player tags = %+v, want SVIGOR", bobPlayer.Metadata.Tags)
	}

	alice, _ = runtime.Creature("creature:alice")
	if hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("alice creature tags = %+v, want hidden cleared", alice.Metadata.Tags)
	}
	if alice.Stats["PHIDDN"] != 0 {
		t.Fatalf("alice PHIDDN = %d, want 0", alice.Stats["PHIDDN"])
	}
	player, _ = runtime.Player("player:alice")
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "phiddn") {
		t.Fatalf("alice player tags = %+v, want hidden cleared", player.Metadata.Tags)
	}

	wantBroadcast := roomBroadcastRecord{
		RoomID:  "room:teach",
		Exclude: "session:alice",
		Text: "\nAlice가 Bob에게 회복 주술의 시범을 보이며 주문 전수를 \n" +
			"시킵니다.\n오옷~~~ 이 주문을 외우자 주위에 이상한 기운이 모이는 것이\n" +
			"그 사람에게 상당한 도움이 될 것 같습니다.\n",
	}
	if len(broadcasts) != 1 || broadcasts[0] != wantBroadcast {
		t.Fatalf("broadcasts = %+v, want %+v", broadcasts, wantBroadcast)
	}
}

func TestTeachHandlerSendsTargetMessageAndExcludesTargetFromRoomBroadcast(t *testing.T) {
	loaded := teachWorld(t, model.ClassCleric, []string{"rcast"})
	alice := loaded.Creatures["creature:alice"]
	alice.Metadata.Tags = []string{"SVIGOR"}
	loaded.Creatures[alice.ID] = alice
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"SVIGOR"}
	loaded.Players[player.ID] = player
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:charlie",
		DisplayName: "Charlie",
		CreatureID:  "creature:charlie",
		RoomID:      "room:teach",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:charlie",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Charlie",
		PlayerID:    "player:charlie",
		RoomID:      "room:teach",
	})
	runtime := state.NewWorld(loaded)

	type activeSession struct {
		ID      string
		ActorID string
	}
	sent := map[string]string{}
	ctx := &Context{
		ActorID:   "player:alice",
		SessionID: "session:alice",
		Values: map[string]any{
			"game.activeSessions": func() []activeSession {
				return []activeSession{
					{ID: "session:alice", ActorID: "player:alice"},
					{ID: "session:bob", ActorID: "player:bob"},
					{ID: "session:charlie", ActorID: "player:charlie"},
				}
			},
			"game.sendToSession": func(id string, cmd struct{ Write string }) error {
				sent[id] = cmd.Write
				return nil
			},
		},
	}
	status, err := NewTeachHandler(runtime)(ctx, ResolvedCommand{Args: []string{"Bob", "회복"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	wantTarget := "\nAlice가 회복 주술의 시범을 보이며 주문 전수를 시킵니다.\n" +
		"오옷~~~ 이 주문을 외우자 주위에 이상한 기운이 모이는 것이 그 \n" +
		"그 사람에게 상당한 도움이 될 것 같습니다.\n"
	if sent["session:bob"] != wantTarget {
		t.Fatalf("bob message = %q, want %q", sent["session:bob"], wantTarget)
	}
	wantRoom := "\nAlice가 Bob에게 회복 주술의 시범을 보이며 주문 전수를 \n" +
		"시킵니다.\n오옷~~~ 이 주문을 외우자 주위에 이상한 기운이 모이는 것이\n" +
		"그 사람에게 상당한 도움이 될 것 같습니다.\n"
	if sent["session:charlie"] != wantRoom {
		t.Fatalf("charlie message = %q, want %q", sent["session:charlie"], wantRoom)
	}
	if _, ok := sent["session:alice"]; ok {
		t.Fatalf("actor received room broadcast: %+v", sent)
	}
}

func TestTeachHandlerRejectsInvalidStates(t *testing.T) {
	tests := []struct {
		name       string
		class      int
		roomTags   []string
		args       []string
		actorTags  []string
		playerTags []string
		targetTags []string
		want       string
	}{
		{
			name:  "not casting room",
			class: model.ClassCleric,
			args:  []string{"Bob", "회복"},
			want:  "주문 전수장에서만 가능합니다.",
		},
		{
			name:     "missing args - no target",
			class:    model.ClassCleric,
			roomTags: []string{"rcast"},
			args:     []string{},
			want:     "누구에게 비법을 전수시키실겁니까?",
		},
		{
			name:     "missing args - no spell",
			class:    model.ClassCleric,
			roomTags: []string{"rcast"},
			args:     []string{"Bob"},
			want:     "누구에게 비법을 전수시키실겁니까?",
		},

		{
			name:      "blind",
			class:     model.ClassCleric,
			roomTags:  []string{"rcast"},
			args:      []string{"Bob", "회복"},
			actorTags: []string{"blind"},
			want:      "\x1b[0;31m아무것도 보이지 않습니다!\n\x1b[0;37m",
		},
		{
			name:       "silenced",
			class:      model.ClassCleric,
			roomTags:   []string{"rcast"},
			args:       []string{"Bob", "회복"},
			playerTags: []string{"PSILNC"},
			want:       "\x1b[0;33m당신은 한마디도 할수 없습니다!\n\x1b[0;37m",
		},
		{
			name:     "wrong class",
			class:    model.ClassFighter,
			roomTags: []string{"rcast"},
			args:     []string{"Bob", "회복"},
			want:     "\n도술사와 불제자만이 전수시킬 수 있는 능력이 있습니다.\n",
		},
		{
			name:     "missing target",
			class:    model.ClassCleric,
			roomTags: []string{"rcast"},
			args:     []string{"없는", "회복"},
			want:     "\n그런 사람은 존재하지 않습니다.\n",
		},
		{
			name:     "spell does not exist",
			class:    model.ClassCleric,
			roomTags: []string{"rcast"},
			args:     []string{"Bob", "nonexistent"},
			want:     "\n그런 주문이 존재하지 않습니다.\n",
		},
		{
			name:     "basic group is not a legacy spell",
			class:    model.ClassCleric,
			roomTags: []string{"rcast"},
			args:     []string{"Bob", "기본"},
			want:     "\n그런 주문이 존재하지 않습니다.\n",
		},
		{
			name:     "ambiguous spell name",
			class:    model.ClassCleric,
			roomTags: []string{"rcast"},
			args:     []string{"Bob", "천"},
			want:     "\n주문이름이 이상합니다.\n",
		},
		{
			name:     "actor does not know spell",
			class:    model.ClassCleric,
			roomTags: []string{"rcast"},
			args:     []string{"Bob", "회복"},
			want:     "\n당신은 아직 그런 주문을 터득하지 못했습니다.\n",
		},
		{
			name:       "target already knows spell",
			class:      model.ClassCleric,
			roomTags:   []string{"rcast"},
			args:       []string{"Bob", "회복"},
			actorTags:  []string{"SVIGOR"},
			targetTags: []string{"SVIGOR"},
			want:       "\nBob이 이미 터득한 주문입니다.\n",
		},
		{
			name:      "cleric cannot teach level 2 spell",
			class:     model.ClassCleric,
			roomTags:  []string{"rcast"},
			args:      []string{"Bob", "삭풍"}, // level 2
			actorTags: []string{"SHURTS"},
			want:      "\n그 주문을 다른 사람에게 전수시킬 수 없습니다.\n",
		},
		{
			name:      "mage cannot teach level 1 spell",
			class:     model.ClassMage,
			roomTags:  []string{"rcast"},
			args:      []string{"Bob", "회복"}, // level 1
			actorTags: []string{"SVIGOR"},
			want:      "\n그 주문을 다른 사람에게 전수시킬 수 없습니다.\n",
		},
		{
			name:      "cleric cannot teach level 3 spell",
			class:     model.ClassCleric,
			roomTags:  []string{"rcast"},
			args:      []string{"Bob", "화궁"}, // level 3
			actorTags: []string{"SFIREB"},
			want:      "\n그 주문을 다른 사람에게 전수시킬 수 없습니다.\n",
		},
		{
			name:      "mage cannot teach level 3 spell",
			class:     model.ClassMage,
			roomTags:  []string{"rcast"},
			args:      []string{"Bob", "화궁"}, // level 3
			actorTags: []string{"SFIREB"},
			want:      "\n그 주문을 다른 사람에게 전수시킬 수 없습니다.\n",
		},
		{
			name:      "invincible cannot teach level 4 spell",
			class:     model.ClassInvincible,
			roomTags:  []string{"rcast"},
			args:      []string{"Bob", "도력반"}, // level 4
			actorTags: []string{"SRESTO"},
			want:      "\n그 주문을 다른 사람에게 전수시킬 수 없습니다.\n",
		},
		{
			name:      "caretaker cannot teach level 5 spell",
			class:     model.ClassCaretaker,
			roomTags:  []string{"rcast"},
			args:      []string{"Bob", "완치"}, // level 5
			actorTags: []string{"SFHEAL"},
			want:      "\n그 주문을 다른 사람에게 전수시킬 수 없습니다.\n",
		},
		{
			name:      "level 6 spell not teachable",
			class:     model.ClassDM,
			roomTags:  []string{"rcast"},
			args:      []string{"Bob", "천지진동"}, // level 6
			actorTags: []string{"SISIX1"},
			want:      "\n천상주문은 다른 사람에게 전수시킬 수 없습니다.\n",
		},
		{
			name:      "level 7 spell not teachable",
			class:     model.ClassDM,
			roomTags:  []string{"rcast"},
			args:      []string{"Bob", "혈사천"}, // level 7
			actorTags: []string{"XIXIX1"},
			want:      "\n태극주문은 다른 사람에게 전수시킬 수 없습니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := teachWorld(t, tt.class, tt.roomTags)
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = append([]string{"hidden"}, tt.actorTags...)
			loaded.Creatures[alice.ID] = alice
			player := loaded.Players["player:alice"]
			player.Metadata.Tags = append([]string{"hidden"}, tt.playerTags...)
			loaded.Players[player.ID] = player
			bob := loaded.Creatures["creature:bob"]
			bob.Metadata.Tags = tt.targetTags
			loaded.Creatures[bob.ID] = bob
			runtime := state.NewWorld(loaded)

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewTeachHandler(runtime)(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			bob, _ = runtime.Creature("creature:bob")
			if tt.want != "" && strings.Contains(tt.want, "전수시킬 수 없습니다") {
				if hasAnyNormalizedFlag(bob.Metadata.Tags, "SVIGOR", "SHURTS", "SLIGHT", "SCUREP", "SBLESS", "SPROTE") {
					t.Fatalf("bob tags = %+v, want no taught spell", bob.Metadata.Tags)
				}
			}
			alice, _ = runtime.Creature("creature:alice")
			if !hasAnyNormalizedFlag(alice.Metadata.Tags, "hidden") {
				t.Fatalf("alice tags = %+v, want hidden retained on rejection", alice.Metadata.Tags)
			}
		})
	}
}

func TestTeachHandlerDispatchesKoreanAndEnglishAliases(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "korean verb final", input: "Bob 회복 가르쳐"},
		{name: "english command first", input: "teach Bob 회복"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded := teachWorld(t, model.ClassCleric, []string{"RCAST"})
			alice := loaded.Creatures["creature:alice"]
			alice.Metadata.Tags = []string{"SVIGOR"}
			loaded.Creatures[alice.ID] = alice
			player := loaded.Players["player:alice"]
			player.Metadata.Tags = []string{"SVIGOR"}
			loaded.Players[player.ID] = player
			runtime := state.NewWorld(loaded)
			dispatcher := Dispatcher{
				Registry: mustRegistry(t, []commandspec.CommandSpec{
					{Name: "가르쳐", Number: 71, Handler: "teach"},
					{Name: "teach", Number: 71, Handler: "teach"},
				}),
				Handlers: map[string]Handler{
					"teach": NewTeachHandler(runtime),
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
			bob, _ := runtime.Creature("creature:bob")
			if !hasAnyNormalizedFlag(bob.Metadata.Tags, "SVIGOR") {
				t.Fatalf("bob tags = %+v, want taught spell SVIGOR", bob.Metadata.Tags)
			}
		})
	}
}

func teachWorld(t *testing.T, class int, roomTags []string) *worldload.World {
	t.Helper()

	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:teach",
		DisplayName: "Teach",
		Metadata:    model.Metadata{Tags: roomTags},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:teach",
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:teach",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:teach",
		Stats:       map[string]int{"class": class, "level": 20},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:teach",
		Stats:       map[string]int{"class": model.ClassFighter, "level": 1},
	})
	return loaded
}
