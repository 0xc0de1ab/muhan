package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMInvisWorld struct {
	players     map[model.PlayerID]model.Player
	creatures   map[model.CreatureID]model.Creature
	addedTags   map[model.CreatureID][]string
	removedTags map[model.CreatureID][]string
	setStats    map[model.CreatureID]map[string]int
}

func (m *mockDMInvisWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMInvisWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMInvisWorld) UpdateCreatureTags(id model.CreatureID, add []string, remove []string) (model.Creature, error) {
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

	c := m.creatures[id]
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

func (m *mockDMInvisWorld) SetCreatureStat(id model.CreatureID, key string, val int) error {
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

func TestDMInvis_Permissions(t *testing.T) {
	tests := []struct {
		name       string
		class      int
		wantStatus Status
		wantDenied bool
	}{
		{
			name:       "class 9 (below SUB_DM)",
			class:      model.ClassInvincible,
			wantStatus: StatusPrompt,
			wantDenied: true,
		},
		{
			name:       "class 10 (caretaker below SUB_DM)",
			class:      model.ClassCaretaker,
			wantStatus: StatusPrompt,
			wantDenied: true,
		},
		{
			name:       "class 11 (bulsa below SUB_DM)",
			class:      model.ClassBulsa,
			wantStatus: StatusPrompt,
			wantDenied: true,
		},
		{
			name:       "class 12 (SUB_DM)",
			class:      model.ClassSubDM,
			wantStatus: StatusDefault,
			wantDenied: false,
		},
		{
			name:       "class 13 (DM)",
			class:      model.ClassDM,
			wantStatus: StatusDefault,
			wantDenied: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMInvisWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": tt.class}},
				},
			}

			ctx := &Context{
				ActorID: "player:alice",
			}
			resolved := ResolvedCommand{
				Input: "*invis",
				Spec: commandspec.CommandSpec{
					Name:    "*invis",
					Handler: "dm_invis",
				},
			}

			handler := NewDMInvisHandler(world)
			status, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			if status != tt.wantStatus {
				t.Fatalf("status = %v, want %v", status, tt.wantStatus)
			}

			output := ctx.OutputString()
			if tt.wantDenied {
				if output != "" {
					t.Errorf("expected no permission output, got %q", output)
				}
			} else {
				if output == dmPlaceholderDeniedMessage {
					t.Errorf("unexpected permission denied message")
				}
			}
		})
	}
}

func TestDMInvis_ToggleLogic(t *testing.T) {
	t.Run("toggle on and off - ANSI disabled", func(t *testing.T) {
		world := &mockDMInvisWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}},
			},
		}

		handler := NewDMInvisHandler(world)

		// 1. Enable invis
		ctx1 := &Context{ActorID: "player:alice"}
		_, err := handler(ctx1, ResolvedCommand{Input: "*invis"})
		if err != nil {
			t.Fatalf("failed to enable: %v", err)
		}
		if got := ctx1.OutputString(); got != "투명 설정.\n" {
			t.Errorf("expected '투명 설정.\\n', got %q", got)
		}
		// check tag added
		alice := world.creatures["creature:alice"]
		if !creatureHasAnyFlag(alice, "PDMINV") {
			t.Errorf("expected PDMINV flag/tag to be set")
		}

		// 2. Disable invis
		ctx2 := &Context{ActorID: "player:alice"}
		_, err = handler(ctx2, ResolvedCommand{Input: "*invis"})
		if err != nil {
			t.Fatalf("failed to disable: %v", err)
		}
		if got := ctx2.OutputString(); got != "투명 해제.\n" {
			t.Errorf("expected '투명 해제.\\n', got %q", got)
		}
		// check tag removed
		alice = world.creatures["creature:alice"]
		if creatureHasAnyFlag(alice, "PDMINV", "dmInvisible") {
			t.Errorf("expected PDMINV flag/tag to be cleared")
		}
	})

	t.Run("toggle on and off - ANSI enabled", func(t *testing.T) {
		world := &mockDMInvisWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}},
			},
		}

		handler := NewDMInvisHandler(world)

		// 1. Enable invis with ANSI
		ctx1 := &Context{
			ActorID: "player:alice",
			Values:  map[string]any{ContextANSIKey: true},
		}
		_, err := handler(ctx1, ResolvedCommand{Input: "*invis"})
		if err != nil {
			t.Fatalf("failed to enable: %v", err)
		}
		// ANSI yellow = 33 -> \x1b[0;33m; ANSI white reset = 37 -> \x1b[0;37m
		want1 := "\x1b[0;33m" + "투명 설정.\n" + "\x1b[0;37m"
		if got := ctx1.OutputString(); got != want1 {
			t.Errorf("expected %q, got %q", want1, got)
		}

		// 2. Disable invis with ANSI
		ctx2 := &Context{
			ActorID: "player:alice",
			Values:  map[string]any{ContextANSIKey: true},
		}
		_, err = handler(ctx2, ResolvedCommand{Input: "*invis"})
		if err != nil {
			t.Fatalf("failed to disable: %v", err)
		}
		// ANSI magenta = 35 -> \x1b[0;35m; ANSI white reset = 37 -> \x1b[0;37m
		want2 := "\x1b[0;35m" + "투명 해제.\n" + "\x1b[0;37m"
		if got := ctx2.OutputString(); got != want2 {
			t.Errorf("expected %q, got %q", want2, got)
		}
	})

	t.Run("support pre-existing dmInvisible tag", func(t *testing.T) {
		world := &mockDMInvisWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {
					ID:    "creature:alice",
					Stats: map[string]int{"class": 12},
					Metadata: model.Metadata{
						Tags: []string{"dmInvisible"},
					},
				},
			},
		}

		handler := NewDMInvisHandler(world)

		ctx := &Context{ActorID: "player:alice"}
		_, err := handler(ctx, ResolvedCommand{Input: "*invis"})
		if err != nil {
			t.Fatalf("failed to disable: %v", err)
		}
		if got := ctx.OutputString(); got != "투명 해제.\n" {
			t.Errorf("expected '투명 해제.\\n', got %q", got)
		}
		alice := world.creatures["creature:alice"]
		if creatureHasAnyFlag(alice, "PDMINV", "dmInvisible") {
			t.Errorf("expected all invis flags to be cleared, got %v", alice.Metadata.Tags)
		}
	})
}

func TestDMInvisClearsLegacyStatFlagLikeC(t *testing.T) {
	world := &mockDMInvisWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {
				ID:    "creature:alice",
				Stats: map[string]int{"class": model.ClassSubDM, "PDMINV": 1},
			},
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	status, err := NewDMInvisHandler(world)(ctx, ResolvedCommand{Input: "*invis"})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if got, want := ctx.OutputString(), "투명 해제.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	alice := world.creatures["creature:alice"]
	if creatureHasAnyFlag(alice, "PDMINV", "dmInvisible") {
		t.Fatalf("PDMINV still enabled after *invis off: tags=%+v stats=%+v", alice.Metadata.Tags, alice.Stats)
	}
	if got := world.setStats["creature:alice"]["PDMINV"]; got != 0 {
		t.Fatalf("SetCreatureStat(PDMINV) = %d, want 0", got)
	}
}
