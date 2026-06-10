package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestMagicEffectMagicTrackSuccess(t *testing.T) {
	// Setup world: Alice is in room:library, Bob is in room:garden
	loaded := readScrollWorld(t, "room:library", "1", "21")
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 10
	alice.Stats = map[string]int{"class": model.ClassRanger, "level": 10, "mpCurrent": 13}
	alice.Metadata.Tags = []string{"STRACK"}
	loaded.Creatures[alice.ID] = alice

	// Add Bob in a different room
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:garden",
	})
	bob := model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:garden",
		Stats:       map[string]int{"level": 5},
	}
	loaded.Creatures[bob.ID] = bob

	// Add the target room room:garden to world
	loaded.Rooms["room:garden"] = model.Room{
		ID:          "room:garden",
		DisplayName: "정원",
	}

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

	handled, success, err := ApplyMagicPowerEffectAgent5(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"추적", "Bob"}},
		magicPowerMagicTrack,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent5 error = %v", err)
	}
	if !handled {
		t.Fatalf("expected handled = true")
	}
	if !success {
		t.Fatalf("expected success = true")
	}

	// Verify Alice was teleported to room:garden
	updatedAlice, _ := runtime.Creature("creature:alice")
	if updatedAlice.RoomID != "room:garden" {
		t.Fatalf("expected Alice to be in room:garden, got %q", updatedAlice.RoomID)
	}
	if got := updatedAlice.Stats["mpCurrent"]; got != 0 {
		t.Fatalf("mpCurrent = %d, want C magictrack cost to 0", got)
	}

	// Verify success messages
	if !strings.Contains(ctx.OutputString(), "Bob의 흔적을 찾아내는데 성공했습니다.") {
		t.Fatalf("expected success message in caster output, got: %q", ctx.OutputString())
	}
	if _, ok := sent["player:bob"]; ok {
		t.Fatalf("sent to player id instead of active session: %+v", sent)
	}
	if got := sent["session:bob"]; !strings.Contains(got, "당신의 흔적을 찾아 내는데 성공하여 당신을") {
		t.Fatalf("target session message = %q", got)
	}
}

func TestDefaultReadScrollMagicTrackBypassesCastGates(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "21")
	alice := loaded.Creatures["creature:alice"]
	alice.Stats = map[string]int{"class": model.ClassFighter, "level": 10, "mpCurrent": 0}
	alice.Properties = map[string]string{"dailyTrackMax": "10", "dailyTrackCur": "10"}
	loaded.Creatures[alice.ID] = alice

	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:garden",
		DisplayName: "정원",
	})
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
	status, err := NewReadScrollHandler(runtime, "", nil)(ctx, ResolvedCommand{Args: []string{"귀환", "Bob"}})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want StatusDefault", status)
	}
	if !strings.Contains(ctx.OutputString(), "Bob의 흔적을 찾아내는데 성공했습니다.") {
		t.Fatalf("output = %q, want magic track success", ctx.OutputString())
	}
	if _, ok := runtime.Object("object:scroll"); ok {
		t.Fatal("scroll still exists after magic track scroll")
	}
	updatedAlice, _ := runtime.Creature("creature:alice")
	if updatedAlice.RoomID != "room:garden" {
		t.Fatalf("Alice room = %q, want room:garden", updatedAlice.RoomID)
	}
	if got := updatedAlice.Properties["dailyTrackCur"]; got != "9" {
		t.Fatalf("dailyTrackCur = %q, want C scroll-side dec_daily to 9", got)
	}
}

func TestMagicEffectMagicTrackMarriageReject(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "21")
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 10
	alice.Stats = map[string]int{"class": model.ClassRanger, "level": 10, "mpCurrent": 13}
	alice.Metadata.Tags = []string{"STRACK"}
	loaded.Creatures[alice.ID] = alice

	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:garden",
		Metadata:    model.Metadata{Tags: []string{"married"}}, // Married tag
	})
	bob := model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:garden",
	}
	loaded.Creatures[bob.ID] = bob

	loaded.Rooms["room:garden"] = model.Room{
		ID:          "room:garden",
		DisplayName: "정원",
	}

	// Setup fake marriage directory
	tempDir := t.TempDir()
	marriageDir := filepath.Join(tempDir, "player", "marriage")
	if err := os.MkdirAll(marriageDir, 0755); err != nil {
		t.Fatalf("failed to create marriage dir: %v", err)
	}

	// Alice's spouse is someone else (Charlie)
	if err := os.WriteFile(filepath.Join(marriageDir, "alice"), []byte("Charlie"), 0644); err != nil {
		t.Fatalf("failed to write spouse file: %v", err)
	}

	// Set override
	LegacyDataRootOverride = tempDir
	defer func() { LegacyDataRootOverride = "" }()

	runtime := state.NewWorld(loaded)
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
		},
	}

	handled, success, err := ApplyMagicPowerEffectAgent5(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"추적", "Bob"}},
		magicPowerMagicTrack,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent5 error = %v", err)
	}
	if !handled {
		t.Fatalf("expected handled = true")
	}
	if success {
		t.Fatalf("expected success = false due to marriage block")
	}

	// Verify Alice is still in room:library
	updatedAlice, _ := runtime.Creature("creature:alice")
	if updatedAlice.RoomID != "room:library" {
		t.Fatalf("expected Alice to remain in room:library, got %q", updatedAlice.RoomID)
	}

	if !strings.Contains(ctx.OutputString(), "추적할 수 없습니다") {
		t.Fatalf("expected reject message, got: %q", ctx.OutputString())
	}
}

func TestMagicEffectMagicTrackDailyLimit(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "21")
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 10
	alice.Stats = map[string]int{"class": model.ClassRanger, "level": 10, "mpCurrent": 13}
	alice.Metadata.Tags = []string{"STRACK"}
	alice.Properties = map[string]string{
		"dailyTrackMax": "10",
		"dailyTrackCur": "0", // 0 uses left!
	}
	loaded.Creatures[alice.ID] = alice

	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:garden",
	})
	bob := model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:garden",
	}
	loaded.Creatures[bob.ID] = bob

	loaded.Rooms["room:garden"] = model.Room{
		ID:          "room:garden",
		DisplayName: "정원",
	}

	runtime := state.NewWorld(loaded)
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
		},
	}

	handled, success, err := ApplyMagicPowerEffectAgent5(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"추적", "Bob"}},
		magicPowerMagicTrack,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent5 error = %v", err)
	}
	if !handled {
		t.Fatalf("expected handled = true")
	}
	if success {
		t.Fatalf("expected success = false due to daily limit")
	}

	if !strings.Contains(ctx.OutputString(), "더 이상 펼칠 수 없습니다") {
		t.Fatalf("expected daily limit message, got: %q", ctx.OutputString())
	}
}

func TestMagicEffectMagicTrackExplicitSelfTargetsUseFindWhoBranchLikeLegacy(t *testing.T) {
	for _, args := range [][]string{
		{"추적", "나"},
		{"추적", "Alice"},
	} {
		t.Run(args[1], func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", "21")
			alice := loaded.Creatures["creature:alice"]
			alice.Level = 10
			alice.Stats = map[string]int{"class": model.ClassRanger, "level": 10, "mpCurrent": 13}
			alice.Metadata.Tags = []string{"STRACK"}
			loaded.Creatures[alice.ID] = alice
			runtime := state.NewWorld(loaded)
			ctx := &Context{
				ActorID: "player:alice",
				Values: map[string]any{
					"game.activeSessions": func() []activeSession {
						return []activeSession{{ID: "session:alice", ActorID: "player:alice"}}
					},
				},
			}

			handled, success, err := ApplyMagicPowerEffectAgent5(
				ctx,
				runtime,
				alice,
				model.ObjectInstance{},
				ResolvedCommand{Args: args},
				magicPowerMagicTrack,
			)
			if err != nil {
				t.Fatalf("ApplyMagicPowerEffectAgent5 error = %v", err)
			}
			if !handled {
				t.Fatalf("handled = false, want true")
			}
			if success || ctx.OutputString() != "\n그런 사람은 존재하지 않습니다.\n" {
				t.Fatalf("success/output = %v/%q, want C find_who self rejection", success, ctx.OutputString())
			}
			updated, _ := runtime.Creature("creature:alice")
			if got := updated.Stats["mpCurrent"]; got != 13 {
				t.Fatalf("mpCurrent = %d, want no cost before target miss", got)
			}
		})
	}
}

func TestMagicEffectMagicTrackRoomLimitIgnoresPDMINVOccupants(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "21")
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 10
	alice.Stats = map[string]int{"class": model.ClassRanger, "level": 10, "mpCurrent": 13}
	alice.Metadata.Tags = []string{"STRACK"}
	loaded.Creatures[alice.ID] = alice

	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:garden",
		DisplayName: "정원",
		PlayerIDs:   []model.PlayerID{"player:bob", "player:carol"},
		Metadata:    model.Metadata{Tags: []string{"RTWOPL"}},
	})
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
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:carol",
		DisplayName: "Carol",
		CreatureID:  "creature:carol",
		RoomID:      "room:garden",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:carol",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Carol",
		PlayerID:    "player:carol",
		RoomID:      "room:garden",
		Metadata:    model.Metadata{Tags: []string{"PDMINV"}},
		Stats:       map[string]int{"class": model.ClassFighter, "level": 5},
	})
	runtime := state.NewWorld(loaded)
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			"game.activeSessions": func() []activeSession {
				return []activeSession{
					{ID: "session:alice", ActorID: "player:alice"},
					{ID: "session:bob", ActorID: "player:bob"},
					{ID: "session:carol", ActorID: "player:carol"},
				}
			},
		},
	}

	handled, success, err := ApplyMagicPowerEffectAgent5(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"추적", "Bob"}},
		magicPowerMagicTrack,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent5 error = %v", err)
	}
	if !handled {
		t.Fatalf("handled = false, want true")
	}
	if !success {
		t.Fatalf("success = false, want C count_vis_ply to ignore PDMINV occupant; output=%q", ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if updated.RoomID != "room:garden" {
		t.Fatalf("alice room = %q, want room:garden", updated.RoomID)
	}
}

func TestMagicEffectMagicTrackOnlyFamilyBlocksCaretakerMismatchLikeLegacy(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "21")
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 10
	alice.Stats = map[string]int{
		"class":         model.ClassCaretaker,
		"level":         10,
		"mpCurrent":     13,
		"dailyExpndMax": 8,
	}
	alice.Metadata.Tags = []string{"SRANGER", "STRACK"}
	loaded.Creatures[alice.ID] = alice

	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:garden",
		DisplayName: "정원",
		PlayerIDs:   []model.PlayerID{"player:bob"},
		Metadata:    model.Metadata{Tags: []string{"RONFML"}},
		Properties:  map[string]string{"special": "7"},
	})
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

	handled, success, err := ApplyMagicPowerEffectAgent5(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"추적", "Bob"}},
		magicPowerMagicTrack,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent5 error = %v", err)
	}
	if !handled {
		t.Fatalf("handled = false, want true")
	}
	if success || ctx.OutputString() != "그 사람이 있는 곳으로 갈 수가 없습니다." {
		t.Fatalf("success/output = %v/%q, want C RONFML block", success, ctx.OutputString())
	}
	updated, _ := runtime.Creature("creature:alice")
	if got := updated.Stats["mpCurrent"]; got != 13 {
		t.Fatalf("mpCurrent = %d, want no C magictrack cost before RONFML block", got)
	}
}

func TestMagicEffectLocatePlayerSuccess(t *testing.T) {
	oldRand := locatePlayerRandIntn
	locatePlayerRandIntn = func(n int) int {
		return 0 // Guaranteeing success
	}
	defer func() { locatePlayerRandIntn = oldRand }()

	loaded := readScrollWorld(t, "room:library", "1", "47")
	alice := loaded.Creatures["creature:alice"]
	alice.Level = 50
	alice.Stats = map[string]int{"level": 50, "intelligence": 25, "mpCurrent": 15}
	alice.Metadata.Tags = []string{"SLOCAT"}
	loaded.Creatures[alice.ID] = alice

	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:garden",
	})
	bob := model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:garden",
		Stats:       map[string]int{"level": 50, "intelligence": 25},
	}
	loaded.Creatures[bob.ID] = bob

	loaded.Rooms["room:garden"] = model.Room{
		ID:          "room:garden",
		DisplayName: "정원",
	}

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

	handled, success, err := ApplyMagicPowerEffectAgent5(
		ctx,
		runtime,
		alice,
		model.ObjectInstance{},
		ResolvedCommand{Args: []string{"투시", "Bob"}},
		magicPowerLocatePlayer,
	)
	if err != nil {
		t.Fatalf("ApplyMagicPowerEffectAgent5 error = %v", err)
	}
	if !handled {
		t.Fatalf("expected handled = true")
	}
	if !success {
		t.Fatalf("expected success = true")
	}

	// Verify caster sees target room description
	if !strings.Contains(ctx.OutputString(), "정원") {
		t.Fatalf("expected caster to see the target room description, got: %q", ctx.OutputString())
	}
	if _, ok := sent["player:bob"]; ok {
		t.Fatalf("sent to player id instead of active session: %+v", sent)
	}
	if got := sent["session:bob"]; !strings.Contains(got, "당신의 눈으로 주위를 보고 있습니다.") {
		t.Fatalf("target session message = %q", got)
	}
}

func TestMagicEffectLocatePlayerLegacyClassGates(t *testing.T) {
	useSpellFailRoll(t, 0)
	oldRand := locatePlayerRandIntn
	locatePlayerRandIntn = func(n int) int {
		return 0
	}
	defer func() { locatePlayerRandIntn = oldRand }()

	for _, tt := range []struct {
		name        string
		targetClass int
		want        string
		wantRoom    bool
		wantSuccess bool
	}{
		{name: "caretaker can be viewed", targetClass: model.ClassCaretaker, wantRoom: true, wantSuccess: true},
		{name: "sub dm fails by chance branch", targetClass: model.ClassSubDM, want: "\n당신의 마음을 Bob에게 집중했습니다.\n\n당신의 정신은 연결될수 없습니다.\n", wantSuccess: true},
		{name: "dm is mentally blocked", targetClass: model.ClassDM, want: "그 사람의 정신력이 너무 높아 투시를 할 수 없습니다."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			loaded := readScrollWorld(t, "room:library", "1", "47")
			alice := loaded.Creatures["creature:alice"]
			alice.Level = 50
			alice.Stats = map[string]int{"class": model.ClassCleric, "level": 50, "intelligence": 25, "mpCurrent": 15}
			alice.Metadata.Tags = []string{"SLOCAT"}
			loaded.Creatures[alice.ID] = alice
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
				Stats:       map[string]int{"class": tt.targetClass, "level": 50, "intelligence": 25},
			})
			loaded.Rooms["room:garden"] = model.Room{
				ID:          "room:garden",
				DisplayName: "정원",
			}
			runtime := state.NewWorld(loaded)
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
				},
			}

			handled, success, err := ApplyMagicPowerEffectAgent5(
				ctx,
				runtime,
				alice,
				model.ObjectInstance{},
				ResolvedCommand{Args: []string{"투시", "Bob"}},
				magicPowerLocatePlayer,
			)
			if err != nil {
				t.Fatalf("ApplyMagicPowerEffectAgent5 error = %v", err)
			}
			if !handled {
				t.Fatalf("expected handled = true")
			}
			if tt.wantRoom {
				if !success || !strings.Contains(ctx.OutputString(), "정원") {
					t.Fatalf("success/output = %v/%q, want target room", success, ctx.OutputString())
				}
				return
			}
			if success != tt.wantSuccess || ctx.OutputString() != tt.want {
				t.Fatalf("success/output = %v/%q, want %v/%q", success, ctx.OutputString(), tt.wantSuccess, tt.want)
			}
		})
	}
}

func TestFindGlobalPlayerUsesLegacyFindWhoSurface(t *testing.T) {
	loaded := readScrollWorld(t, "room:library", "1", "47")
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "홍길동",
		CreatureID:  "creature:bob",
		RoomID:      "room:garden",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "홍길동",
		PlayerID:    "player:bob",
		RoomID:      "room:garden",
	})
	runtime := state.NewWorld(loaded)

	offlineCtx := &Context{
		Values: map[string]any{
			"game.activeSessions": func() []activeSession {
				return []activeSession{{ID: "session:alice", ActorID: "player:alice"}}
			},
		},
	}
	if player, ok := findGlobalPlayer(offlineCtx, runtime, "홍길동"); ok {
		t.Fatalf("findGlobalPlayer matched saved but offline player %q", player.ID)
	}

	onlineCtx := &Context{
		Values: map[string]any{
			"game.activeSessions": func() []activeSession {
				return []activeSession{
					{ID: "session:alice", ActorID: "player:alice"},
					{ID: "session:bob", ActorID: "player:bob"},
				}
			},
		},
	}
	if player, ok := findGlobalPlayer(onlineCtx, runtime, "player:bob"); ok {
		t.Fatalf("findGlobalPlayer matched Go-only player ID alias %q", player.ID)
	}
	if player, ok := findGlobalPlayer(onlineCtx, runtime, "홍길동"); !ok || player.ID != "player:bob" {
		t.Fatalf("findGlobalPlayer display-name match = %q/%v, want player:bob/true", player.ID, ok)
	}
}
