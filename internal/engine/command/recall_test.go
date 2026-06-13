package command

import (
	"reflect"
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestReturnSquareHandlerMovesActorToLifeTree(t *testing.T) {
	world := state.NewWorld(recallWorld(t, "room:01001"))
	defer world.Close()
	dispatcher := Dispatcher{
		Registry: mustRegistry(t, []commandspec.CommandSpec{
			{Name: "귀환", Number: 81, Handler: "return_square"},
		}),
		Handlers: map[string]Handler{
			"return_square": NewReturnSquareHandler(world),
		},
	}

	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
	status, err := dispatcher.DispatchLine(ctx, "귀환")
	if err != nil {
		t.Fatalf("DispatchLine() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if !strings.Contains(ctx.OutputString(), "귀환!") {
		t.Fatalf("output missing recall message:\n%s", ctx.OutputString())
	}
	player, _ := world.Player("player:alice")
	creature, _ := world.Creature("creature:alice")
	if player.RoomID != defaultReturnRoomID || creature.RoomID != defaultReturnRoomID {
		t.Fatalf("actor room = player %q creature %q, want %q", player.RoomID, creature.RoomID, defaultReturnRoomID)
	}
	lifeTree, _ := world.Room(defaultReturnRoomID)
	if len(lifeTree.PlayerIDs) != 1 || lifeTree.PlayerIDs[0] != "player:alice" {
		t.Fatalf("return room players = %+v", lifeTree.PlayerIDs)
	}
	wantBroadcasts := []roomBroadcastRecord{
		{RoomID: "room:01001", Exclude: "session:alice", Text: "\nAlice님이 갑자기 사라집니다!"},
		{RoomID: string(defaultReturnRoomID), Exclude: "session:alice", Text: "\nAlice님이 갑자기 자욱한 연기와 함께 나타났습니다!"},
	}
	if !reflect.DeepEqual(broadcasts, wantBroadcasts) {
		t.Fatalf("broadcasts = %+v, want %+v", broadcasts, wantBroadcasts)
	}
}

func TestReturnSquareHandlerAlreadyAtLifeTree(t *testing.T) {
	world := state.NewWorld(recallWorld(t, defaultReturnRoomID))
	defer world.Close()
	handler := NewReturnSquareHandler(world)
	ctx := &Context{ActorID: "player:alice"}

	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if got, want := ctx.OutputString(), "당신은 이미 생명의 나무에 와 있습니다!"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestReturnSquareHandlerBlocksLegacyFailureBranches(t *testing.T) {
	tests := []struct {
		name      string
		roomID    model.RoomID
		configure func(*testing.T, *state.World) *Context
		want      string
	}{
		{
			name:   "jail room",
			roomID: "room:00010",
			want:   "사용자 감옥의 지킴이 [김 건모]가 당신을 붙잡으며 말합니다.\n\n\"당신은 잘못된 행동의 결과로 갇혀 있습니다. 참고 기다리십시요.!!\"\n",
		},
		{
			name:   "monster is attacking actor",
			roomID: "room:01001",
			configure: func(t *testing.T, world *state.World) *Context {
				t.Helper()
				if _, err := world.AddEnemy("creature:goblin", "creature:alice"); err != nil {
					t.Fatalf("AddEnemy() error = %v", err)
				}
				return &Context{ActorID: "player:alice"}
			},
			want: "당신은 싸우고 있는 중입니다!!",
		},
		{
			name:   "grouped actor",
			roomID: "room:01001",
			configure: func(t *testing.T, world *state.World) *Context {
				t.Helper()
				return &Context{
					ActorID: "player:alice",
					Values: map[string]any{
						"game.groupMemory": &mockGroupMemory{
							followers: map[string][]string{"player:alice": {"player:bob"}},
						},
					},
				}
			},
			want: "먼저 그룹에서 나오세요.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := state.NewWorld(recallWorld(t, tt.roomID))
	defer world.Close()
			ctx := &Context{ActorID: "player:alice"}
			if tt.configure != nil {
				ctx = tt.configure(t, world)
			}

			status, err := NewReturnSquareHandler(world)(ctx, ResolvedCommand{})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want default", status)
			}
			if got := ctx.OutputString(); got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
			player, _ := world.Player("player:alice")
			if player.RoomID != tt.roomID {
				t.Fatalf("player room = %q, want blocked in %q", player.RoomID, tt.roomID)
			}
		})
	}
}

func TestReturnSquareHandlerIgnoresInternalIDEnemyNamesLikeLegacy(t *testing.T) {
	world := &returnInternalIDEnemyWorld{
		World:   state.NewWorld(recallWorld(t, "room:01001")),
		enemies: []string{"alice", "player:alice", "creature:alice"},
	}
	ctx := &Context{ActorID: "player:alice"}

	status, err := NewReturnSquareHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if strings.Contains(ctx.OutputString(), "싸우고 있는 중") || !strings.Contains(ctx.OutputString(), "귀환!") {
		t.Fatalf("output = %q, want recall success without Go-only ID enemy match", ctx.OutputString())
	}
	player, _ := world.Player("player:alice")
	if player.RoomID != defaultReturnRoomID {
		t.Fatalf("player room = %q, want %q", player.RoomID, defaultReturnRoomID)
	}
}

type returnInternalIDEnemyWorld struct {
	*state.World
	enemies []string
}

func (w *returnInternalIDEnemyWorld) CreatureEnemies(creatureID model.CreatureID) ([]string, error) {
	if creatureID != "creature:goblin" {
		return w.World.CreatureEnemies(creatureID)
	}
	return append([]string(nil), w.enemies...), nil
}

func TestReturnSquareHandlerFamilyReturnUsesLegacyRoomAndSkipsLifeTreeBlock(t *testing.T) {
	world := state.NewWorld(recallWorld(t, defaultReturnRoomID))
	defer world.Close()
	if err := world.SetCreatureStat("creature:alice", "dailyExpndMax", 7); err != nil {
		t.Fatalf("SetCreatureStat(dailyExpndMax) error = %v", err)
	}
	creature, _ := world.UpdateCreatureTags("creature:alice", []string{"PFRTUN"}, nil)
	if !creatureHasAnyFlag(creature, "PFRTUN") {
		t.Fatal("PFRTUN tag was not set")
	}

	status, err := NewReturnSquareHandler(world)(&Context{ActorID: "player:alice"}, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	player, _ := world.Player("player:alice")
	creature, _ = world.Creature("creature:alice")
	if want := model.RoomID("room:03307"); player.RoomID != want || creature.RoomID != want {
		t.Fatalf("actor room = player %q creature %q, want %q", player.RoomID, creature.RoomID, want)
	}
}

func TestReturnSquareHandlerHighLevelMortalLosesMP(t *testing.T) {
	world := state.NewWorld(recallWorld(t, "room:01001"))
	defer world.Close()
	if err := world.SetCreatureStat("creature:alice", "level", 21); err != nil {
		t.Fatalf("SetCreatureStat(level) error = %v", err)
	}
	if err := world.SetCreatureStat("creature:alice", "class", model.ClassFighter); err != nil {
		t.Fatalf("SetCreatureStat(class) error = %v", err)
	}
	if err := world.SetCreatureStat("creature:alice", "mpCurrent", 40); err != nil {
		t.Fatalf("SetCreatureStat(mpCurrent) error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	if _, err := NewReturnSquareHandler(world)(ctx, ResolvedCommand{}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if !strings.Contains(ctx.OutputString(), "흑암의 세력이 당신의 도력을 뺏습니다.") {
		t.Fatalf("output missing MP penalty:\n%s", ctx.OutputString())
	}
	creature, _ := world.Creature("creature:alice")
	if got := creature.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want 0", got)
	}
}

func TestReturnSquareHandlerDMInvisibleSuppressesLegacyBroadcasts(t *testing.T) {
	world := state.NewWorld(recallWorld(t, "room:01001"))
	defer world.Close()
	if err := world.SetCreatureStat("creature:alice", "PDMINV", 1); err != nil {
		t.Fatalf("SetCreatureStat(PDMINV) error = %v", err)
	}
	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)

	if _, err := NewReturnSquareHandler(world)(ctx, ResolvedCommand{}); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if len(broadcasts) != 0 {
		t.Fatalf("broadcasts = %+v, want none for PDMINV", broadcasts)
	}
}

func recallWorld(t *testing.T, actorRoom model.RoomID) *worldload.World {
	t.Helper()

	loaded := emptyInventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{
		ID:          defaultReturnRoomID,
		DisplayName: "생명의 나무",
	})
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:01001",
		DisplayName: "통계 무한 광장",
		CreatureIDs: []model.CreatureID{"creature:alice", "creature:goblin"},
		PlayerIDs:   []model.PlayerID{"player:alice"},
	})
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:00010",
		DisplayName: "사용자 감옥",
		CreatureIDs: []model.CreatureID{"creature:alice"},
		PlayerIDs:   []model.PlayerID{"player:alice"},
	})
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:03307",
		DisplayName: "패거리 존",
	})
	player := loaded.Players["player:alice"]
	player.DisplayName = "Alice"
	player.RoomID = actorRoom
	loaded.Players[player.ID] = player
	creature := loaded.Creatures["creature:alice"]
	creature.DisplayName = "Alice"
	creature.RoomID = actorRoom
	loaded.Creatures[creature.ID] = creature
	loaded.Creatures["creature:goblin"] = model.Creature{
		ID:          "creature:goblin",
		Kind:        model.CreatureKindMonster,
		DisplayName: "고블린",
		RoomID:      "room:01001",
	}
	return loaded
}
