package command

import (
	"strings"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/world/model"
)

type mockDMFollowWorld struct {
	players          map[model.PlayerID]model.Player
	creatures        map[model.CreatureID]model.Creature
	roomCreatures    map[model.RoomID][]model.Creature
	updateTagsCalled bool
	updateTagsCrtID  model.CreatureID
	updateTagsAdd    []string
	updateTagsRemove []string
	setStats         map[model.CreatureID]map[string]int
	propertyWrites   []dmFollowPropertyWrite
}

type dmFollowPropertyWrite struct {
	CreatureID model.CreatureID
	Key        string
	Value      string
}

func (m *mockDMFollowWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMFollowWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMFollowWorld) Room(id model.RoomID) (model.Room, bool) {
	return model.Room{ID: id}, true
}

func (m *mockDMFollowWorld) FindCreatureInRoom(roomID model.RoomID, name string) (model.Creature, bool) {
	for _, c := range m.roomCreatures[roomID] {
		if strings.EqualFold(c.DisplayName, name) ||
			strings.EqualFold(string(c.ID), name) ||
			strings.EqualFold(strings.TrimPrefix(string(c.ID), "creature:"), name) {
			return c, true
		}
	}
	return model.Creature{}, false
}

func (m *mockDMFollowWorld) FindCreatureByName(roomID model.RoomID, name string, count int) (model.Creature, bool) {
	if count < 1 {
		count = 1
	}
	seen := 0
	for _, c := range m.roomCreatures[roomID] {
		if strings.EqualFold(c.DisplayName, name) ||
			strings.EqualFold(string(c.ID), name) ||
			strings.EqualFold(strings.TrimPrefix(string(c.ID), "creature:"), name) {
			seen++
			if seen == count {
				return c, true
			}
		}
	}
	return model.Creature{}, false
}

func (m *mockDMFollowWorld) UpdateCreatureTags(crtID model.CreatureID, addTags, removeTags []string) (model.Creature, error) {
	m.updateTagsCalled = true
	m.updateTagsCrtID = crtID
	m.updateTagsAdd = addTags
	m.updateTagsRemove = removeTags

	c := m.creatures[crtID]
	tagSet := make(map[string]struct{})
	for _, t := range c.Metadata.Tags {
		tagSet[t] = struct{}{}
	}
	for _, t := range addTags {
		tagSet[t] = struct{}{}
	}
	for _, t := range removeTags {
		delete(tagSet, t)
	}

	c.Metadata.Tags = nil
	for t := range tagSet {
		c.Metadata.Tags = append(c.Metadata.Tags, t)
	}
	m.creatures[crtID] = c
	if m.roomCreatures != nil {
		for roomID, list := range m.roomCreatures {
			for idx, rc := range list {
				if rc.ID == crtID {
					m.roomCreatures[roomID][idx] = c
				}
			}
		}
	}

	return c, nil
}

func (m *mockDMFollowWorld) SetCreatureStat(crtID model.CreatureID, key string, value int) error {
	if m.setStats == nil {
		m.setStats = make(map[model.CreatureID]map[string]int)
	}
	if m.setStats[crtID] == nil {
		m.setStats[crtID] = make(map[string]int)
	}
	m.setStats[crtID][key] = value

	c := m.creatures[crtID]
	if c.Stats == nil {
		c.Stats = map[string]int{}
	}
	c.Stats[key] = value
	m.creatures[crtID] = c
	if m.roomCreatures != nil {
		for roomID, list := range m.roomCreatures {
			for idx, rc := range list {
				if rc.ID == crtID {
					m.roomCreatures[roomID][idx] = c
				}
			}
		}
	}
	return nil
}

func (m *mockDMFollowWorld) SetCreatureProperty(crtID model.CreatureID, key string, value string) (model.Creature, error) {
	m.propertyWrites = append(m.propertyWrites, dmFollowPropertyWrite{CreatureID: crtID, Key: key, Value: value})
	c := m.creatures[crtID]
	if value == "" {
		delete(c.Properties, key)
		if len(c.Properties) == 0 {
			c.Properties = nil
		}
	} else {
		if c.Properties == nil {
			c.Properties = map[string]string{}
		}
		c.Properties[key] = value
	}
	m.creatures[crtID] = c
	if m.roomCreatures != nil {
		for roomID, list := range m.roomCreatures {
			for idx, rc := range list {
				if rc.ID == crtID {
					m.roomCreatures[roomID][idx] = c
				}
			}
		}
	}
	return c, nil
}

func TestDMFollow_PermissionDenied(t *testing.T) {
	world := &mockDMFollowWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}, DisplayName: "Alice"}, // Sub-DM (< 13)
		},
	}

	handler := NewDMFollowHandler(world)
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
	}

	resolved := ResolvedCommand{
		Input:  "*따르기 goblin",
		Parsed: commandparse.Command{Str: [7]string{"*따르기"}, Num: 2},
		Args:   []string{"goblin"},
	}

	status, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusPrompt {
		t.Errorf("status = %v, want StatusPrompt", status)
	}

	if got := ctx.OutputString(); got != "" {
		t.Errorf("expected no permission output, got %q", got)
	}
}

func TestDMFollow_Usage(t *testing.T) {
	world := &mockDMFollowWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, DisplayName: "Alice"}, // DM
		},
	}

	handler := NewDMFollowHandler(world)
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
	}

	resolved := ResolvedCommand{
		Input:  "*따르기",
		Parsed: commandparse.Command{Str: [7]string{"*따르기"}, Num: 1},
		Args:   []string{},
	}

	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "사용법: <괴물> *따르기\n"
	if ctx.OutputString() != expected {
		t.Errorf("expected %q, got %q", expected, ctx.OutputString())
	}
}

func TestDMFollow_CreatureNotFound(t *testing.T) {
	world := &mockDMFollowWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:1"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, DisplayName: "Alice"},
		},
		roomCreatures: map[model.RoomID][]model.Creature{
			"room:1": {},
		},
	}

	handler := NewDMFollowHandler(world)
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
	}

	resolved := ResolvedCommand{
		Input:  "goblin *따르기",
		Parsed: commandparse.Command{Str: [7]string{"*따르기"}, Num: 2},
		Args:   []string{"goblin"},
	}

	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "그런 괴물이 없습니다.\n"
	if ctx.OutputString() != expected {
		t.Errorf("expected %q, got %q", expected, ctx.OutputString())
	}
}

func TestDMFollowRejectsPlayerCreatureLikeLegacy(t *testing.T) {
	world := &mockDMFollowWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:1"},
			"player:bob":   {ID: "player:bob", CreatureID: "creature:bob", RoomID: "room:1"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, DisplayName: "Alice"},
			"creature:bob": {
				ID:          "creature:bob",
				Kind:        model.CreatureKindPlayer,
				PlayerID:    "player:bob",
				DisplayName: "Bob",
				RoomID:      "room:1",
			},
		},
		roomCreatures: map[model.RoomID][]model.Creature{
			"room:1": {
				{
					ID:          "creature:bob",
					Kind:        model.CreatureKindPlayer,
					PlayerID:    "player:bob",
					DisplayName: "Bob",
					RoomID:      "room:1",
				},
			},
		},
	}

	handler := NewDMFollowHandler(world)
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
	}

	_, err := handler(ctx, ResolvedCommand{
		Input:  "bob *따르기",
		Parsed: commandparse.Command{Str: [7]string{"*따르기"}, Num: 2},
		Args:   []string{"bob"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := ctx.OutputString(), "그런 괴물이 없습니다.\n"; got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
	if world.updateTagsCalled {
		t.Fatalf("UpdateCreatureTags called for player creature")
	}
	if len(world.propertyWrites) != 0 {
		t.Fatalf("property writes for player creature: %+v", world.propertyWrites)
	}
}

func TestDMFollow_FixedCreature(t *testing.T) {
	world := &mockDMFollowWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:1"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 13}, DisplayName: "Alice"},
		},
		roomCreatures: map[model.RoomID][]model.Creature{
			"room:1": {
				{ID: "creature:goblin", DisplayName: "Goblin", Metadata: model.Metadata{Tags: []string{"MPERMT"}}},
			},
		},
	}

	handler := NewDMFollowHandler(world)
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
	}

	resolved := ResolvedCommand{
		Input:  "goblin *따르기",
		Parsed: commandparse.Command{Str: [7]string{"*따르기"}, Num: 2},
		Args:   []string{"goblin"},
	}

	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "고정된 괴물입니다.\n"
	if ctx.OutputString() != expected {
		t.Errorf("expected %q, got %q", expected, ctx.OutputString())
	}
}

func TestDMFollow_ToggleOnAndOff(t *testing.T) {
	world := &mockDMFollowWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:1"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice":  {ID: "creature:alice", Stats: map[string]int{"class": 13}, DisplayName: "Alice"},
			"creature:goblin": {ID: "creature:goblin", DisplayName: "Goblin"},
		},
		roomCreatures: map[model.RoomID][]model.Creature{
			"room:1": {
				{ID: "creature:goblin", DisplayName: "Goblin"},
			},
		},
	}

	handler := NewDMFollowHandler(world)
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
	}

	resolved := ResolvedCommand{
		Input:  "goblin *따르기",
		Parsed: commandparse.Command{Str: [7]string{"*따르기"}, Num: 2},
		Args:   []string{"goblin"},
	}

	// 1. Toggle On (should add MDMFOL)
	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !world.updateTagsCalled {
		t.Error("expected UpdateCreatureTags to be called")
	}
	if world.updateTagsCrtID != "creature:goblin" {
		t.Errorf("expected creature ID 'creature:goblin', got %q", world.updateTagsCrtID)
	}
	if len(world.updateTagsAdd) != 1 || world.updateTagsAdd[0] != "MDMFOL" {
		t.Errorf("expected MDMFOL to be added, got %v", world.updateTagsAdd)
	}
	if len(world.updateTagsRemove) != 0 {
		t.Errorf("expected no tags removed, got %v", world.updateTagsRemove)
	}
	updated := world.creatures["creature:goblin"]
	if updated.Stats["MDMFOL"] != 1 {
		t.Errorf("MDMFOL stat = %d, want 1", updated.Stats["MDMFOL"])
	}
	if got := updated.Properties[dmFollowLeaderProperty]; got != "player:alice" {
		t.Errorf("%s = %q, want player:alice", dmFollowLeaderProperty, got)
	}
	if got := updated.Properties[dmFollowLeaderCreatureProperty]; got != "creature:alice" {
		t.Errorf("%s = %q, want creature:alice", dmFollowLeaderCreatureProperty, got)
	}

	expectedOn := "Goblin이 당신을 따릅니다.\n"
	if ctx.OutputString() != expectedOn {
		t.Errorf("expected %q, got %q", expectedOn, ctx.OutputString())
	}

	// Reset mock tracks
	world.updateTagsCalled = false
	world.propertyWrites = nil
	ctx.Output = nil

	// 2. Toggle Off (should remove MDMFOL)
	_, err = handler(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !world.updateTagsCalled {
		t.Error("expected UpdateCreatureTags to be called")
	}
	if len(world.updateTagsRemove) != 1 || world.updateTagsRemove[0] != "MDMFOL" {
		t.Errorf("expected MDMFOL to be removed, got %v", world.updateTagsRemove)
	}
	if len(world.updateTagsAdd) != 0 {
		t.Errorf("expected no tags added, got %v", world.updateTagsAdd)
	}
	updated = world.creatures["creature:goblin"]
	if updated.Stats["MDMFOL"] != 0 {
		t.Errorf("MDMFOL stat = %d, want 0 after unfollow", updated.Stats["MDMFOL"])
	}
	if got := updated.Properties[dmFollowLeaderProperty]; got != "" {
		t.Errorf("%s = %q, want empty after unfollow", dmFollowLeaderProperty, got)
	}
	if got := updated.Properties[dmFollowLeaderCreatureProperty]; got != "" {
		t.Errorf("%s = %q, want empty after unfollow", dmFollowLeaderCreatureProperty, got)
	}
	if len(world.propertyWrites) != 2 {
		t.Errorf("property writes on unfollow = %+v, want two deletes", world.propertyWrites)
	}

	expectedOff := "Goblin이 당신을 그만 따릅니다.\n"
	if ctx.OutputString() != expectedOff {
		t.Errorf("expected %q, got %q", expectedOff, ctx.OutputString())
	}
}

func TestDMFollowClearsStatBackedMDMFOLLikeC(t *testing.T) {
	world := &mockDMFollowWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:1"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:          "creature:alice",
				Stats:       map[string]int{"class": model.ClassDM},
				DisplayName: "Alice",
				RoomID:      "room:1",
			},
			"creature:goblin": {
				ID:          "creature:goblin",
				DisplayName: "Goblin",
				RoomID:      "room:1",
				Stats:       map[string]int{"MDMFOL": 1},
				Properties: map[string]string{
					dmFollowLeaderProperty:         "player:alice",
					dmFollowLeaderCreatureProperty: "creature:alice",
				},
			},
		},
		roomCreatures: map[model.RoomID][]model.Creature{
			"room:1": {
				{
					ID:          "creature:goblin",
					DisplayName: "Goblin",
					RoomID:      "room:1",
					Stats:       map[string]int{"MDMFOL": 1},
				},
			},
		},
	}

	ctx := &Context{SessionID: "session:alice", ActorID: "player:alice"}
	_, err := NewDMFollowHandler(world)(ctx, ResolvedCommand{
		Input:  "goblin *따르기",
		Parsed: commandparse.Command{Str: [7]string{"*따르기"}, Num: 2},
		Args:   []string{"goblin"},
	})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}

	updated := world.creatures["creature:goblin"]
	if creatureHasAnyFlag(updated, "MDMFOL") {
		t.Fatalf("MDMFOL still enabled after unfollow: tags=%+v stats=%+v", updated.Metadata.Tags, updated.Stats)
	}
	if got := updated.Properties[dmFollowLeaderProperty]; got != "" {
		t.Fatalf("%s = %q, want empty", dmFollowLeaderProperty, got)
	}
	if got, want := ctx.OutputString(), "Goblin이 당신을 그만 따릅니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDMFollowCreatureActorUsesOwnRoomLikeLegacy(t *testing.T) {
	world := &mockDMFollowWorld{
		players: map[model.PlayerID]model.Player{},
		creatures: map[model.CreatureID]model.Creature{
			"creature:dm":     {ID: "creature:dm", Stats: map[string]int{"class": 13}, DisplayName: "DM", RoomID: "room:1"},
			"creature:goblin": {ID: "creature:goblin", DisplayName: "Goblin", RoomID: "room:1"},
		},
		roomCreatures: map[model.RoomID][]model.Creature{
			"room:1": {
				{ID: "creature:goblin", DisplayName: "Goblin", RoomID: "room:1"},
			},
		},
	}

	handler := NewDMFollowHandler(world)
	ctx := &Context{
		SessionID: "session:dm",
		ActorID:   "creature:dm",
	}

	_, err := handler(ctx, ResolvedCommand{
		Input:  "goblin *따르기",
		Parsed: commandparse.Command{Str: [7]string{"*따르기"}, Num: 2},
		Args:   []string{"goblin"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if world.updateTagsCrtID != "creature:goblin" {
		t.Fatalf("expected goblin to be updated, got %s; output %q", world.updateTagsCrtID, ctx.OutputString())
	}
	updated := world.creatures["creature:goblin"]
	if got := updated.Properties[dmFollowLeaderProperty]; got != "creature:dm" {
		t.Errorf("%s = %q, want creature:dm", dmFollowLeaderProperty, got)
	}
	if got, want := ctx.OutputString(), "Goblin이 당신을 따릅니다.\n"; got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestDMFollowOrdinalSelectsMatchingDuplicateCreature(t *testing.T) {
	world := &mockDMFollowWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:1"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice":   {ID: "creature:alice", Stats: map[string]int{"class": 13}, DisplayName: "Alice", RoomID: "room:1"},
			"creature:goblin1": {ID: "creature:goblin1", DisplayName: "Goblin", RoomID: "room:1", Metadata: model.Metadata{Tags: []string{"MPERMT"}}},
			"creature:goblin2": {ID: "creature:goblin2", DisplayName: "Goblin", RoomID: "room:1"},
		},
		roomCreatures: map[model.RoomID][]model.Creature{
			"room:1": {
				{ID: "creature:goblin1", DisplayName: "Goblin", RoomID: "room:1", Metadata: model.Metadata{Tags: []string{"MPERMT"}}},
				{ID: "creature:goblin2", DisplayName: "Goblin", RoomID: "room:1"},
			},
		},
	}

	handler := NewDMFollowHandler(world)
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
	}

	_, err := handler(ctx, ResolvedCommand{
		Input:  "2 Goblin *따르기",
		Parsed: commandparse.Command{Str: [7]string{"*따르기", "Goblin"}, Num: 2},
		Args:   []string{"Goblin"},
		Values: []int64{2},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if world.updateTagsCrtID != "creature:goblin2" {
		t.Fatalf("expected second Goblin to follow, got %s; output %q", world.updateTagsCrtID, ctx.OutputString())
	}
	if got, want := ctx.OutputString(), "Goblin이 당신을 따릅니다.\n"; got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestDMFollowUsesParsedTargetSlotAndOrdinalLikeCWhenArgsMissing(t *testing.T) {
	world := &mockDMFollowWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice", RoomID: "room:1"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice":   {ID: "creature:alice", Stats: map[string]int{"class": model.ClassDM}, DisplayName: "Alice", RoomID: "room:1"},
			"creature:goblin1": {ID: "creature:goblin1", DisplayName: "Goblin", RoomID: "room:1"},
			"creature:goblin2": {ID: "creature:goblin2", DisplayName: "Goblin", RoomID: "room:1"},
		},
		roomCreatures: map[model.RoomID][]model.Creature{
			"room:1": {
				{ID: "creature:goblin1", DisplayName: "Goblin", RoomID: "room:1"},
				{ID: "creature:goblin2", DisplayName: "Goblin", RoomID: "room:1"},
			},
		},
	}
	ctx := &Context{
		SessionID: "session:alice",
		ActorID:   "player:alice",
	}

	resolved := ResolvedCommand{
		Input: "2 Goblin *따르기",
		Parsed: commandparse.Command{
			Str: [7]string{"*따르기", "Goblin"},
			Val: [7]int64{1, 2},
			Num: 2,
		},
	}

	status, err := NewDMFollowHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if world.updateTagsCrtID != "creature:goblin2" {
		t.Fatalf("follow target = %s, want creature:goblin2; output %q", world.updateTagsCrtID, ctx.OutputString())
	}
	if got, want := ctx.OutputString(), "Goblin이 당신을 따릅니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
