package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMSpyWorld struct {
	players     map[model.PlayerID]model.Player
	creatures   map[model.CreatureID]model.Creature
	addedTags   map[model.CreatureID][]string
	removedTags map[model.CreatureID][]string
	setStats    map[model.CreatureID]map[string]int
	spies       map[model.PlayerID]model.PlayerID // spy -> target
}

func (m *mockDMSpyWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMSpyWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMSpyWorld) FindPlayerByName(name string) (model.Player, bool) {
	for _, p := range m.players {
		if strings.EqualFold(p.DisplayName, name) ||
			strings.EqualFold(string(p.ID), name) ||
			strings.EqualFold(strings.TrimPrefix(string(p.ID), "player:"), name) {
			return p, true
		}
	}
	return model.Player{}, false
}

func (m *mockDMSpyWorld) SetSpy(spyPlayerID, targetPlayerID model.PlayerID) error {
	if m.spies == nil {
		m.spies = make(map[model.PlayerID]model.PlayerID)
	}
	m.spies[spyPlayerID] = targetPlayerID
	return nil
}

func (m *mockDMSpyWorld) ClearSpy(spyPlayerID model.PlayerID) error {
	if m.spies != nil {
		delete(m.spies, spyPlayerID)
	}
	return nil
}

func (m *mockDMSpyWorld) IsSpying(spyPlayerID model.PlayerID) (model.PlayerID, bool) {
	if m.spies == nil {
		return "", false
	}
	target, ok := m.spies[spyPlayerID]
	return target, ok
}

func (m *mockDMSpyWorld) IsBeingSpiedOn(targetPlayerID model.PlayerID) (model.PlayerID, bool) {
	if m.spies == nil {
		return "", false
	}
	for spy, target := range m.spies {
		if target == targetPlayerID {
			return spy, true
		}
	}
	return "", false
}

func (m *mockDMSpyWorld) UpdateCreatureTags(id model.CreatureID, add []string, remove []string) (model.Creature, error) {
	if m.addedTags == nil {
		m.addedTags = make(map[model.CreatureID][]string)
	}
	if m.removedTags == nil {
		m.removedTags = make(map[model.CreatureID][]string)
	}
	if len(add) > 0 {
		m.addedTags[id] = append(m.addedTags[id], add...)
	}
	if len(remove) > 0 {
		m.removedTags[id] = append(m.removedTags[id], remove...)
	}

	c, ok := m.creatures[id]
	if !ok {
		return model.Creature{}, nil
	}
	var newTags []string
	for _, tag := range c.Metadata.Tags {
		removed := false
		for _, r := range remove {
			if strings.EqualFold(tag, r) {
				removed = true
				break
			}
		}
		if !removed {
			newTags = append(newTags, tag)
		}
	}
	for _, a := range add {
		exists := false
		for _, tag := range newTags {
			if strings.EqualFold(tag, a) {
				exists = true
				break
			}
		}
		if !exists {
			newTags = append(newTags, a)
		}
	}
	c.Metadata.Tags = newTags
	m.creatures[id] = c
	return c, nil
}

func (m *mockDMSpyWorld) SetCreatureStat(id model.CreatureID, key string, val int) error {
	if m.setStats == nil {
		m.setStats = make(map[model.CreatureID]map[string]int)
	}
	if m.setStats[id] == nil {
		m.setStats[id] = make(map[string]int)
	}
	m.setStats[id][key] = val

	c := m.creatures[id]
	if c.Stats == nil {
		c.Stats = make(map[string]int)
	}
	c.Stats[key] = val
	m.creatures[id] = c
	return nil
}

func TestDMSpy(t *testing.T) {
	tests := []struct {
		name           string
		actorID        string
		args           []string
		setupTags      []string
		setupSpies     map[model.PlayerID]model.PlayerID
		activeSessions []testActiveSession
		players        map[model.PlayerID]model.Player
		creatures      map[model.CreatureID]model.Creature
		wantStatus     Status
		wantOutput     string
		wantSpyState   map[model.PlayerID]model.PlayerID
		wantAddedTags  map[model.CreatureID][]string
		wantRemTags    map[model.CreatureID][]string
	}{
		{
			name:    "permission denied (class below SUB_DM)",
			actorID: "player:caster",
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {ID: "creature:caster", Stats: map[string]int{"class": model.ClassBulsa}},
			},
			wantStatus: StatusPrompt,
			wantOutput: "",
		},
		{
			name:      "turn spy off",
			actorID:   "player:caster",
			setupTags: []string{"PSPYON"},
			setupSpies: map[model.PlayerID]model.PlayerID{
				"player:caster": "player:target",
			},
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID:    "creature:caster",
					Stats: map[string]int{"class": model.ClassSubDM},
					Metadata: model.Metadata{
						Tags: []string{"PSPYON"},
					},
				},
			},
			wantStatus:   StatusDefault,
			wantOutput:   "감시 끝.\n",
			wantSpyState: map[model.PlayerID]model.PlayerID{}, // empty
			wantRemTags: map[model.CreatureID][]string{
				"creature:caster": {"PSPYON"},
			},
		},
		{
			name:    "no target name given",
			actorID: "player:caster",
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {ID: "creature:caster", Stats: map[string]int{"class": model.ClassSubDM}},
			},
			wantStatus: StatusDefault,
			wantOutput: "누굴 염탐합니까??\n",
		},
		{
			name:    "target not found",
			actorID: "player:caster",
			args:    []string{"missing"},
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {ID: "creature:caster", Stats: map[string]int{"class": model.ClassSubDM}},
			},
			wantStatus: StatusDefault,
			wantOutput: "누굴 감시하려구요.\n",
		},
		{
			name:    "saved target without active session is not found",
			actorID: "player:caster",
			args:    []string{"target"},
			activeSessions: []testActiveSession{
				{ID: "session:caster", ActorID: "player:caster"},
			},
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
				"player:target": {ID: "player:target", DisplayName: "target", CreatureID: "creature:target"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {ID: "creature:caster", Stats: map[string]int{"class": model.ClassSubDM}},
				"creature:target": {ID: "creature:target", DisplayName: "target"},
			},
			wantStatus:   StatusDefault,
			wantOutput:   "누굴 감시하려구요.\n",
			wantSpyState: map[model.PlayerID]model.PlayerID{},
		},
		{
			name:    "target already spied on by someone",
			actorID: "player:caster",
			args:    []string{"target"},
			setupSpies: map[model.PlayerID]model.PlayerID{
				"player:other": "player:target",
			},
			activeSessions: []testActiveSession{
				{ID: "session:target", ActorID: "player:target"},
			},
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
				"player:target": {ID: "player:target", DisplayName: "target", CreatureID: "creature:target"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {ID: "creature:caster", Stats: map[string]int{"class": model.ClassSubDM}},
				"creature:target": {ID: "creature:target"},
			},
			wantStatus: StatusDefault,
			wantOutput: "그사람을 벌써 감시하고 있습니다.\n",
			wantSpyState: map[model.PlayerID]model.PlayerID{
				"player:other": "player:target",
			},
		},
		{
			name:    "start spying successfully",
			actorID: "player:caster",
			args:    []string{"target"},
			activeSessions: []testActiveSession{
				{ID: "session:target", ActorID: "player:target"},
			},
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
				"player:target": {ID: "player:target", DisplayName: "target", CreatureID: "creature:target"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {ID: "creature:caster", Stats: map[string]int{"class": model.ClassSubDM}},
				"creature:target": {ID: "creature:target"},
			},
			wantStatus: StatusDefault,
			wantOutput: "감시 시작.\n",
			wantSpyState: map[model.PlayerID]model.PlayerID{
				"player:caster": "player:target",
			},
			wantAddedTags: map[model.CreatureID][]string{
				"creature:caster": {"PSPYON", "PDMINV"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMSpyWorld{
				players:     tt.players,
				creatures:   tt.creatures,
				spies:       tt.setupSpies,
				addedTags:   make(map[model.CreatureID][]string),
				removedTags: make(map[model.CreatureID][]string),
			}

			handler := NewDMSpyHandler(world)
			ctx := &Context{
				ActorID: tt.actorID,
			}
			if tt.activeSessions != nil {
				ctx.Values = map[string]any{
					"game.activeSessions": func() []testActiveSession {
						return tt.activeSessions
					},
				}
			}
			resolved := ResolvedCommand{
				Spec: commandspec.CommandSpec{
					Name: "dm_spy",
				},
				Args: tt.args,
			}

			status, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status != tt.wantStatus {
				t.Errorf("status = %v, want %v", status, tt.wantStatus)
			}

			if gotOutput := ctx.OutputString(); gotOutput != tt.wantOutput {
				t.Errorf("output = %q, want %q", gotOutput, tt.wantOutput)
			}

			if tt.wantSpyState != nil {
				if len(world.spies) != len(tt.wantSpyState) {
					t.Errorf("spy state size mismatch: got %v, want %v", world.spies, tt.wantSpyState)
				} else {
					for k, v := range tt.wantSpyState {
						if got, ok := world.spies[k]; !ok || got != v {
							t.Errorf("spy state for %s = %s, want %s", k, got, v)
						}
					}
				}
			}

			if tt.wantAddedTags != nil {
				for k, v := range tt.wantAddedTags {
					got := world.addedTags[k]
					if !sliceEqual(got, v) {
						t.Errorf("added tags for %s = %v, want %v", k, got, v)
					}
				}
			}

			if tt.wantRemTags != nil {
				for k, v := range tt.wantRemTags {
					got := world.removedTags[k]
					if !sliceEqual(got, v) {
						t.Errorf("removed tags for %s = %v, want %v", k, got, v)
					}
				}
			}
		})
	}
}

func TestDMSpyTogglesLegacyStatFlagsLikeC(t *testing.T) {
	t.Run("stop clears PSPYON stat only", func(t *testing.T) {
		world := &mockDMSpyWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {
					ID:    "creature:caster",
					Stats: map[string]int{"class": model.ClassSubDM, "PSPYON": 1, "PDMINV": 1},
				},
			},
			spies: map[model.PlayerID]model.PlayerID{
				"player:caster": "player:target",
			},
		}
		ctx := &Context{ActorID: "player:caster"}

		status, err := NewDMSpyHandler(world)(ctx, ResolvedCommand{Input: "*spy"})
		if err != nil {
			t.Fatalf("handler() error = %v", err)
		}
		if status != StatusDefault {
			t.Fatalf("status = %v, want StatusDefault", status)
		}
		if got, want := ctx.OutputString(), "감시 끝.\n"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
		caster := world.creatures["creature:caster"]
		if creatureHasAnyFlag(caster, "PSPYON") {
			t.Fatalf("PSPYON still enabled after spy stop: tags=%+v stats=%+v", caster.Metadata.Tags, caster.Stats)
		}
		if !creatureHasAnyFlag(caster, "PDMINV") {
			t.Fatalf("PDMINV was cleared, but C dm_spy stop keeps it set: stats=%+v", caster.Stats)
		}
	})

	t.Run("start sets PSPYON and PDMINV stats", func(t *testing.T) {
		world := &mockDMSpyWorld{
			players: map[model.PlayerID]model.Player{
				"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
				"player:target": {ID: "player:target", DisplayName: "Target", CreatureID: "creature:target"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:caster": {ID: "creature:caster", Stats: map[string]int{"class": model.ClassSubDM}},
				"creature:target": {ID: "creature:target", DisplayName: "Target"},
			},
		}
		ctx := &Context{
			ActorID: "player:caster",
			Values: map[string]any{
				"game.activeSessions": func() []testActiveSession {
					return []testActiveSession{{ID: "session:target", ActorID: "player:target"}}
				},
			},
		}

		status, err := NewDMSpyHandler(world)(ctx, ResolvedCommand{Input: "*spy Target", Args: []string{"Target"}})
		if err != nil {
			t.Fatalf("handler() error = %v", err)
		}
		if status != StatusDefault {
			t.Fatalf("status = %v, want StatusDefault", status)
		}
		caster := world.creatures["creature:caster"]
		if !creatureHasAnyFlag(caster, "PSPYON") || !creatureHasAnyFlag(caster, "PDMINV") {
			t.Fatalf("legacy spy flags not set: tags=%+v stats=%+v", caster.Metadata.Tags, caster.Stats)
		}
	})
}

func TestDMSpyUsesParsedTargetSlotLikeCWhenArgsMissing(t *testing.T) {
	world := &mockDMSpyWorld{
		players: map[model.PlayerID]model.Player{
			"player:caster": {ID: "player:caster", CreatureID: "creature:caster"},
			"player:target": {ID: "player:target", DisplayName: "Target", CreatureID: "creature:target"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:caster": {ID: "creature:caster", Stats: map[string]int{"class": model.ClassSubDM}},
			"creature:target": {ID: "creature:target", DisplayName: "Target"},
		},
	}
	ctx := &Context{
		ActorID: "player:caster",
		Values: map[string]any{
			"game.activeSessions": func() []testActiveSession {
				return []testActiveSession{{ID: "session:target", ActorID: "player:target"}}
			},
		},
	}

	var parsed commandparse.Command
	parsed.Num = 2
	parsed.Str[0] = "*spy"
	parsed.Str[1] = "target"
	resolved := ResolvedCommand{
		Parsed: parsed,
		Spec:   commandspec.CommandSpec{Name: "*spy", Handler: "dm_spy", Privileged: true},
	}

	status, err := NewDMSpyHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if got, want := ctx.OutputString(), "감시 시작.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if got := world.spies["player:caster"]; got != "player:target" {
		t.Fatalf("spy target = %q, want player:target", got)
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
