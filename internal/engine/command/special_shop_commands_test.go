package command

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

func TestBurnHandlerKeepsHiddenWithoutArgumentLikeLegacy(t *testing.T) {
	runtime := state.NewWorld(burnWorld(t, "room:burn", model.ClassFighter))
	setBurnActorHidden(t, runtime)

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBurnHandler(runtime)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || ctx.OutputString() != "무엇을 태우시려구요?" {
		t.Fatalf("status/output = %d/%q, want missing target prompt", status, ctx.OutputString())
	}
	assertBurnActorHiddenRetained(t, runtime)
}

func TestBurnHandlerChecksCooldownBeforeHiddenAndLookupLikeLegacy(t *testing.T) {
	runtime := state.NewWorld(burnWorld(t, "room:burn", model.ClassFighter))
	setBurnActorHidden(t, runtime)
	if err := runtime.SetCreatureCooldown("creature:alice", burnCooldownKey, time.Now().Unix(), 5); err != nil {
		t.Fatalf("SetCreatureCooldown() error = %v", err)
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewBurnHandler(runtime)(ctx, ResolvedCommand{Args: []string{"없는"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	out := ctx.OutputString()
	if status != StatusDefault || !strings.Contains(out, "기다리세요.") || strings.Contains(out, "갖고 있지") {
		t.Fatalf("status/output = %d/%q, want cooldown before object lookup", status, out)
	}
	assertBurnActorHiddenRetained(t, runtime)
}

func TestBurnHandlerClearsHiddenBeforeFailureBranchesLikeLegacy(t *testing.T) {
	tests := []struct {
		name   string
		roomID model.RoomID
		args   []string
		want   string
		setup  func(t *testing.T, runtime *state.World)
	}{
		{name: "missing object", roomID: "room:burn", args: []string{"없는"}, want: "당신은 그런것을 갖고 있지 않습니다."},
		{name: "plaza", roomID: "1001", args: []string{"나무"}, want: "광장에서는 소각할 수 없습니다."},
		{
			name:   "no burn flag",
			roomID: "room:burn",
			args:   []string{"나무"},
			want:   "소각할수 없는 아이템입니다.",
			setup: func(t *testing.T, runtime *state.World) {
				t.Helper()
				if _, err := runtime.UpdateObjectTags("object:stick", []string{"ONOBUN"}, nil); err != nil {
					t.Fatalf("UpdateObjectTags() error = %v", err)
				}
			},
		},
		{
			name:   "quest object with shots",
			roomID: "room:burn",
			args:   []string{"나무"},
			want:   "임무 아이템은 태우지 못합니다.",
			setup: func(t *testing.T, runtime *state.World) {
				t.Helper()
				if _, err := runtime.SetObjectProperty("object:stick", "questnum", "1"); err != nil {
					t.Fatalf("SetObjectProperty(questnum) error = %v", err)
				}
				if _, err := runtime.SetObjectProperty("object:stick", "shotscur", "1"); err != nil {
					t.Fatalf("SetObjectProperty(shotscur) error = %v", err)
				}
			},
		},
		{
			name:   "event object with shots",
			roomID: "room:burn",
			args:   []string{"나무"},
			want:   "이벤트 아이템은 소각할수 없습니다.",
			setup: func(t *testing.T, runtime *state.World) {
				t.Helper()
				if _, err := runtime.UpdateObjectTags("object:stick", []string{"OEVENT"}, nil); err != nil {
					t.Fatalf("UpdateObjectTags() error = %v", err)
				}
				if _, err := runtime.SetObjectProperty("object:stick", "shotscur", "1"); err != nil {
					t.Fatalf("SetObjectProperty(shotscur) error = %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := state.NewWorld(burnWorld(t, tt.roomID, model.ClassFighter))
			setBurnActorHidden(t, runtime)
			if tt.setup != nil {
				tt.setup(t, runtime)
			}

			ctx := &Context{ActorID: "player:alice"}
			status, err := NewBurnHandler(runtime)(ctx, ResolvedCommand{Args: tt.args})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || ctx.OutputString() != tt.want {
				t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), tt.want)
			}
			assertBurnActorHiddenCleared(t, runtime)
			assertBurnCooldownInactive(t, runtime)
			if _, ok := runtime.Object("object:stick"); !ok {
				t.Fatalf("stick object removed on %s failure", tt.name)
			}
		})
	}
}

func TestBurnHandlerSuccessBroadcastsDestroysTreeAndSetsCooldownLikeLegacy(t *testing.T) {
	withLegacyBurnRoll(t, fixedRoll(3000))
	runtime := state.NewWorld(burnWorld(t, "room:burn", model.ClassFighter))
	setBurnActorHidden(t, runtime)
	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)

	status, err := NewBurnHandler(runtime)(ctx, ResolvedCommand{Args: []string{"가방"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	wantOut := "당신은 가방을 태웠습니다.\n당신은 약간의 상금과 경험을 받았습니다."
	if status != StatusDefault || ctx.OutputString() != wantOut {
		t.Fatalf("status/output = %d/%q, want burn success", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0].RoomID != "room:burn" || broadcasts[0].Exclude != "session:alice" ||
		broadcasts[0].Text != "\nAlice가 가방을 태웠습니다." {
		t.Fatalf("broadcasts = %+v, want legacy room burn", broadcasts)
	}
	if _, ok := runtime.Object("object:bag"); ok {
		t.Fatalf("burned bag still exists")
	}
	if _, ok := runtime.Object("object:gem"); ok {
		t.Fatalf("contained object still exists after recursive burn")
	}
	creature, _ := runtime.Creature("creature:alice")
	if containsBurnObjectID(creature.Inventory.ObjectIDs, "object:bag") {
		t.Fatalf("inventory still contains burned bag: %+v", creature.Inventory.ObjectIDs)
	}
	if got := creature.Stats["gold"]; got != 11 {
		t.Fatalf("gold = %d, want 11", got)
	}
	if got := creature.Stats["experience"]; got != 101 {
		t.Fatalf("experience = %d, want 101", got)
	}
	assertBurnActorHiddenCleared(t, runtime)
	assertBurnCooldownActive(t, runtime)
}

func TestBurnHandlerUsesOnlyFirstArgumentLikeLegacy(t *testing.T) {
	withLegacyBurnRoll(t, fixedRoll(3000))
	runtime := state.NewWorld(burnWorld(t, "room:burn", model.ClassFighter))
	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)

	status, err := NewBurnHandler(runtime)(ctx, ResolvedCommand{Args: []string{"나무", "무시"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	wantOut := "당신은 나무막대를 태웠습니다.\n당신은 약간의 상금과 경험을 받았습니다."
	if status != StatusDefault || ctx.OutputString() != wantOut {
		t.Fatalf("status/output = %d/%q, want first argument burn success", status, ctx.OutputString())
	}
	if len(broadcasts) != 1 || broadcasts[0].RoomID != "room:burn" || broadcasts[0].Exclude != "session:alice" ||
		broadcasts[0].Text != "\nAlice가 나무막대를 태웠습니다." {
		t.Fatalf("broadcasts = %+v, want legacy room burn", broadcasts)
	}
	if _, ok := runtime.Object("object:stick"); ok {
		t.Fatalf("burned stick still exists")
	}
	if _, ok := runtime.Object("object:bag"); !ok {
		t.Fatalf("second argument affected unrelated bag")
	}
	assertBurnCooldownActive(t, runtime)
}

func TestBurnHandlerUnlinksLegacyMailScrollLikeC(t *testing.T) {
	withLegacyBurnRoll(t, fixedRoll(3000))
	root := t.TempDir()
	postDir := filepath.Join(root, "post")
	if err := os.MkdirAll(postDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(post) error = %v", err)
	}
	mailPath := filepath.Join(postDir, "Alice_mail")
	if err := os.WriteFile(mailPath, []byte("mail body"), 0o600); err != nil {
		t.Fatalf("WriteFile(mail) error = %v", err)
	}

	runtime := state.NewWorld(burnWorld(t, "room:burn", model.ClassFighter))
	if _, err := runtime.SetObjectProperty("object:stick", "type", strconv.Itoa(legacyObjectScroll)); err != nil {
		t.Fatalf("SetObjectProperty(type) error = %v", err)
	}
	if _, err := runtime.SetObjectProperty("object:stick", "adjustment", "-100"); err != nil {
		t.Fatalf("SetObjectProperty(adjustment) error = %v", err)
	}
	if _, err := runtime.SetObjectProperty("object:stick", "useOutput", "Alice_mail"); err != nil {
		t.Fatalf("SetObjectProperty(useOutput) error = %v", err)
	}
	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)

	status, err := NewBurnHandlerWithRoot(runtime, root)(ctx, ResolvedCommand{Args: []string{"나무"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault || !strings.Contains(ctx.OutputString(), "당신은 나무막대를 태웠습니다.") {
		t.Fatalf("status/output = %d/%q, want legacy mail scroll burn success", status, ctx.OutputString())
	}
	if _, err := os.Stat(mailPath); !os.IsNotExist(err) {
		t.Fatalf("Stat(mail) error = %v, want removed mail file", err)
	}
	if _, ok := runtime.Object("object:stick"); ok {
		t.Fatalf("burned mail scroll object still exists")
	}
}

func TestBurnHandlerLegacyJackpotRewardsAndBroadcasts(t *testing.T) {
	tests := []struct {
		name             string
		class            int
		wantGold         int
		wantExperience   int
		wantOutput       string
		wantGlobalNotice string
	}{
		{
			name:             "ordinary class",
			class:            model.ClassFighter,
			wantGold:         100011,
			wantExperience:   10101,
			wantOutput:       "신이 당신의 정성이 갸륵해서 경험치와 돈벼락을 내립니다.",
			wantGlobalNotice: "\n### 신이 Alice에게 경험치와 돈벼락을 내립니다.\n",
		},
		{
			name:             "invincible class",
			class:            model.ClassInvincible,
			wantGold:         3000011,
			wantExperience:   300101,
			wantOutput:       "신이 당신의 정성이 갸륵해서 엄청난 경험치와 돈벼락을 내립니다.",
			wantGlobalNotice: "\n### 신이 Alice에게 엄청난 경험치와 돈벼락을 내립니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withLegacyBurnRoll(t, fixedRoll(1))
			runtime := state.NewWorld(burnWorld(t, "room:burn", tt.class))
			var broadcasts []roomBroadcastRecord
			ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)
			var global []string
			ctx.Values["game.broadcast"] = func(cmd struct{ Write string }) error {
				global = append(global, cmd.Write)
				return nil
			}

			status, err := NewBurnHandler(runtime)(ctx, ResolvedCommand{Args: []string{"나무"}})
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != StatusDefault || !strings.Contains(ctx.OutputString(), tt.wantOutput) {
				t.Fatalf("status/output = %d/%q, want jackpot output %q", status, ctx.OutputString(), tt.wantOutput)
			}
			creature, _ := runtime.Creature("creature:alice")
			if got := creature.Stats["gold"]; got != tt.wantGold {
				t.Fatalf("gold = %d, want %d", got, tt.wantGold)
			}
			if got := creature.Stats["experience"]; got != tt.wantExperience {
				t.Fatalf("experience = %d, want %d", got, tt.wantExperience)
			}
			if len(global) != 1 || global[0] != tt.wantGlobalNotice {
				t.Fatalf("global broadcast = %+v, want %q", global, tt.wantGlobalNotice)
			}
		})
	}
}

func TestMonsterPurchaseHandlerBroadcastsLegacyRoomMessage(t *testing.T) {
	runtime := state.NewWorld(monsterPurchaseWorld(t))
	var broadcasts []roomBroadcastRecord
	ctx := contextWithRoomBroadcast("player:alice", "session:alice", &broadcasts)

	status, err := NewMonsterPurchaseHandler(runtime)(ctx, ResolvedCommand{
		Args:   []string{"상인", "사과"},
		Values: []int64{1, 1},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	wantOut := "당신은 상인에게 25냥을 줍니다.\n" +
		"상인이 \"고맙습니다. 여기 사과가 있습니다.\"라고 말합니다.\n"
	if status != StatusDefault || ctx.OutputString() != wantOut {
		t.Fatalf("status/output = %d/%q, want %q", status, ctx.OutputString(), wantOut)
	}
	if len(broadcasts) != 1 || broadcasts[0].RoomID != "room:market" ||
		broadcasts[0].Exclude != "session:alice" ||
		broadcasts[0].Text != "Alice이 상인에게 사과를 구입할 돈 25냥을 줍니다.\n" {
		t.Fatalf("broadcasts = %+v, want legacy monster purchase broadcast", broadcasts)
	}

	creature, _ := runtime.Creature("creature:alice")
	if got := creature.Stats["gold"]; got != 75 {
		t.Fatalf("gold = %d, want 75", got)
	}
	if len(creature.Inventory.ObjectIDs) != 1 {
		t.Fatalf("inventory = %+v, want one purchased clone", creature.Inventory.ObjectIDs)
	}
	purchased, ok := runtime.Object(creature.Inventory.ObjectIDs[0])
	if !ok {
		t.Fatalf("purchased clone %q missing", creature.Inventory.ObjectIDs[0])
	}
	if purchased.PrototypeID != "object:o01:1" || purchased.Location.CreatureID != "creature:alice" {
		t.Fatalf("purchased object = proto:%q location:%+v, want carry clone in inventory", purchased.PrototypeID, purchased.Location)
	}
}

func monsterPurchaseWorld(t *testing.T) *worldload.World {
	t.Helper()
	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:market",
		DisplayName: "시장",
		PlayerIDs:   []model.PlayerID{"player:alice"},
		CreatureIDs: []model.CreatureID{"creature:vendor"},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      "room:market",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      "room:market",
		Stats: map[string]int{
			"gold":     100,
			"strength": 10,
			"level":    20,
		},
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:vendor",
		Kind:        model.CreatureKindMonster,
		DisplayName: "상인",
		RoomID:      "room:market",
		Metadata:    model.Metadata{Tags: []string{"MPURIT"}},
		Stats: map[string]int{
			"carry[0]": 101,
		},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{
		ID:          "object:o01:1",
		DisplayName: "사과",
		Properties: map[string]string{
			"value":  "25",
			"weight": "1",
		},
	})
	return loaded
}

func burnWorld(t *testing.T, roomID model.RoomID, class int) *worldload.World {
	t.Helper()
	loaded := worldload.NewWorld()
	mustAddLookRoom(t, loaded, model.Room{
		ID:          roomID,
		DisplayName: "소각장",
		PlayerIDs:   []model.PlayerID{"player:alice"},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:alice",
		DisplayName: "Alice",
		CreatureID:  "creature:alice",
		RoomID:      roomID,
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Alice",
		PlayerID:    "player:alice",
		RoomID:      roomID,
		Stats: map[string]int{
			"class":      class,
			"gold":       10,
			"experience": 100,
		},
		Inventory: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:stick", "object:bag"}},
	})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{ID: "proto:stick", DisplayName: "나무막대"})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{ID: "proto:bag", Kind: model.ObjectKindContainer, DisplayName: "가방"})
	mustAddLookPrototype(t, loaded, model.ObjectPrototype{ID: "proto:gem", DisplayName: "보석"})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:stick",
		PrototypeID: "proto:stick",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:bag",
		PrototypeID: "proto:bag",
		Location:    model.ObjectLocation{CreatureID: "creature:alice", Slot: "inventory"},
		Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:gem"}},
	})
	mustAddLookObject(t, loaded, model.ObjectInstance{
		ID:          "object:gem",
		PrototypeID: "proto:gem",
		Location:    model.ObjectLocation{ContainerID: "object:bag"},
	})
	return loaded
}

func setBurnActorHidden(t *testing.T, runtime *state.World) {
	t.Helper()
	if _, err := runtime.UpdateCreatureTags("creature:alice", []string{"hidden", "PHIDDN"}, nil); err != nil {
		t.Fatalf("UpdateCreatureTags() error = %v", err)
	}
	if _, err := runtime.UpdatePlayerTags("player:alice", []string{"hidden", "PHIDDN"}, nil); err != nil {
		t.Fatalf("UpdatePlayerTags() error = %v", err)
	}
	if err := runtime.SetCreatureStat("creature:alice", "PHIDDN", 1); err != nil {
		t.Fatalf("SetCreatureStat() error = %v", err)
	}
}

func assertBurnActorHiddenRetained(t *testing.T, runtime *state.World) {
	t.Helper()
	creature, _ := runtime.Creature("creature:alice")
	player, _ := runtime.Player("player:alice")
	if !hasAnyNormalizedFlag(creature.Metadata.Tags, "hidden", "PHIDDN") || creature.Stats["PHIDDN"] != 1 {
		t.Fatalf("creature hidden state = tags:%+v stats:%+v, want retained", creature.Metadata.Tags, creature.Stats)
	}
	if !hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "PHIDDN") {
		t.Fatalf("player hidden tags = %+v, want retained", player.Metadata.Tags)
	}
}

func assertBurnActorHiddenCleared(t *testing.T, runtime *state.World) {
	t.Helper()
	creature, _ := runtime.Creature("creature:alice")
	player, _ := runtime.Player("player:alice")
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "hidden", "PHIDDN") || creature.Stats["PHIDDN"] != 0 {
		t.Fatalf("creature hidden state = tags:%+v stats:%+v, want cleared", creature.Metadata.Tags, creature.Stats)
	}
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "PHIDDN") {
		t.Fatalf("player hidden tags = %+v, want cleared", player.Metadata.Tags)
	}
}

func assertBurnCooldownInactive(t *testing.T, runtime *state.World) {
	t.Helper()
	remaining, available, err := runtime.UseCreatureCooldown("creature:alice", burnCooldownKey, time.Now().Unix(), 0)
	if err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	}
	if !available || remaining != 0 {
		t.Fatalf("cooldown remaining/available = %d/%v, want inactive", remaining, available)
	}
}

func assertBurnCooldownActive(t *testing.T, runtime *state.World) {
	t.Helper()
	remaining, available, err := runtime.UseCreatureCooldown("creature:alice", burnCooldownKey, time.Now().Unix(), 0)
	if err != nil {
		t.Fatalf("UseCreatureCooldown() error = %v", err)
	}
	if available || remaining <= 0 {
		t.Fatalf("cooldown remaining/available = %d/%v, want active", remaining, available)
	}
}

func withLegacyBurnRoll(t *testing.T, roll SearchRollFunc) {
	t.Helper()
	previous := legacyBurnRoll
	legacyBurnRoll = roll
	t.Cleanup(func() {
		legacyBurnRoll = previous
	})
}

func containsBurnObjectID(ids []model.ObjectInstanceID, target model.ObjectInstanceID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
