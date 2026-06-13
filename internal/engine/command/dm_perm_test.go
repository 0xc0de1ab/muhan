package command

import (
	"strings"
	"testing"

	"github.com/0xc0de1ab/muhan/internal/commandparse"
	"github.com/0xc0de1ab/muhan/internal/commandspec"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type mockDMPermWorld struct {
	players     map[model.PlayerID]model.Player
	creatures   map[model.CreatureID]model.Creature
	rooms       map[model.RoomID]model.Room
	objects     map[model.ObjectInstanceID]model.ObjectInstance
	prototypes  map[model.PrototypeID]model.ObjectPrototype
	updatedTags map[model.ObjectInstanceID][]string
}

func (m *mockDMPermWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMPermWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMPermWorld) Room(id model.RoomID) (model.Room, bool) {
	r, ok := m.rooms[id]
	return r, ok
}

func (m *mockDMPermWorld) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	o, ok := m.objects[id]
	return o, ok
}

func (m *mockDMPermWorld) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	p, ok := m.prototypes[id]
	return p, ok
}

func (m *mockDMPermWorld) UpdateObjectTags(id model.ObjectInstanceID, add []string, remove []string) (model.ObjectInstance, error) {
	if m.updatedTags == nil {
		m.updatedTags = make(map[model.ObjectInstanceID][]string)
	}
	m.updatedTags[id] = add
	obj := m.objects[id]
	obj.Metadata.Tags = append(obj.Metadata.Tags, add...)
	m.objects[id] = obj
	return obj, nil
}

func TestDMPerm_ValidationAndExecution(t *testing.T) {
	tests := []struct {
		name        string
		actorID     string
		class       int
		args        []string
		roomObjects []model.ObjectInstanceID
		objects     map[model.ObjectInstanceID]model.ObjectInstance
		prototypes  map[model.PrototypeID]model.ObjectPrototype
		wantStatus  Status
		wantOutput  string
		wantTags    bool
		targetObjID model.ObjectInstanceID
	}{
		{
			name:       "unauthorized player class",
			actorID:    "player:alice",
			class:      12, // caretaker/sub_dm (below DM)
			wantStatus: StatusPrompt,
			wantOutput: "",
		},
		{
			name:       "authorized DM but object not found",
			actorID:    "player:alice",
			class:      13, // DM
			args:       []string{"sword"},
			wantOutput: "실패.\n",
		},
		{
			name:        "authorized DM object found and tags updated",
			actorID:     "player:alice",
			class:       13, // DM
			args:        []string{"sword"},
			roomObjects: []model.ObjectInstanceID{"object:sword:1"},
			objects: map[model.ObjectInstanceID]model.ObjectInstance{
				"object:sword:1": {
					ID:                  "object:sword:1",
					PrototypeID:         "prototype:1",
					DisplayNameOverride: "장검",
					Location:            model.ObjectLocation{RoomID: "room:1"},
				},
			},
			prototypes: map[model.PrototypeID]model.ObjectPrototype{
				"prototype:1": {
					ID:          "prototype:1",
					DisplayName: "장검",
					Keywords:    []string{"sword"},
				},
			},
			wantOutput:  "성공.\n",
			wantTags:    true,
			targetObjID: "object:sword:1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMPermWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {ID: "creature:alice", RoomID: "room:1", Stats: map[string]int{"class": tt.class}},
				},
				rooms: map[model.RoomID]model.Room{
					"room:1": {
						ID: "room:1",
						Objects: model.ObjectRefList{
							ObjectIDs: tt.roomObjects,
						},
					},
				},
				objects:    tt.objects,
				prototypes: tt.prototypes,
			}

			ctx := &Context{
				ActorID: tt.actorID,
			}
			resolved := ResolvedCommand{
				Input: "*perm",
				Args:  tt.args,
				Spec: commandspec.CommandSpec{
					Name:       "*perm",
					Number:     105,
					Handler:    "dm_perm",
					Privileged: true,
				},
			}

			handler := NewDMPermHandler(world)
			status, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("handler() error = %v", err)
			}
			wantStatus := tt.wantStatus
			if wantStatus == 0 {
				wantStatus = StatusDefault
			}
			if status != wantStatus {
				t.Fatalf("status = %v, want %v", status, wantStatus)
			}

			gotOutput := ctx.OutputString()
			if gotOutput != tt.wantOutput {
				t.Fatalf("output = %q, want %q", gotOutput, tt.wantOutput)
			}

			if tt.wantTags {
				tags, exists := world.updatedTags[tt.targetObjID]
				if !exists {
					t.Fatalf("expected tags to be updated for object %s", tt.targetObjID)
				}
				hasTag := func(name string) bool {
					for _, tag := range tags {
						if strings.EqualFold(tag, name) {
							return true
						}
					}
					return false
				}
				if !hasTag("operm2") || !hasTag("otempp") {
					t.Fatalf("updated tags %v do not contain operm2 and otempp", tags)
				}
			}
		})
	}
}

func TestDMPermUsesParsedTargetSlotAndOrdinalLikeCWhenArgsMissing(t *testing.T) {
	world := &mockDMPermWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", RoomID: "room:1", Stats: map[string]int{"class": model.ClassDM}},
		},
		rooms: map[model.RoomID]model.Room{
			"room:1": {
				ID: "room:1",
				Objects: model.ObjectRefList{
					ObjectIDs: []model.ObjectInstanceID{"object:sword:1", "object:sword:2"},
				},
			},
		},
		objects: map[model.ObjectInstanceID]model.ObjectInstance{
			"object:sword:1": {
				ID:                  "object:sword:1",
				PrototypeID:         "prototype:sword",
				DisplayNameOverride: "장검",
				Location:            model.ObjectLocation{RoomID: "room:1"},
			},
			"object:sword:2": {
				ID:                  "object:sword:2",
				PrototypeID:         "prototype:sword",
				DisplayNameOverride: "장검",
				Location:            model.ObjectLocation{RoomID: "room:1"},
			},
		},
		prototypes: map[model.PrototypeID]model.ObjectPrototype{
			"prototype:sword": {ID: "prototype:sword", DisplayName: "장검", Keywords: []string{"sword"}},
		},
	}
	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Input:  "2 sword *perm",
		Parsed: commandparse.Command{Num: 2, Str: [commandparse.CommandMax]string{"*perm", "sword"}, Val: [commandparse.CommandMax]int64{1, 2}},
		Spec:   commandspec.CommandSpec{Name: "*perm", Number: 105, Handler: "dm_perm", Privileged: true},
	}

	status, err := NewDMPermHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want StatusDefault", status)
	}
	if got, want := ctx.OutputString(), "성공.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if _, ok := world.updatedTags["object:sword:2"]; !ok {
		t.Fatalf("parsed-slot ordinal did not update second sword: %+v", world.updatedTags)
	}
	if _, ok := world.updatedTags["object:sword:1"]; ok {
		t.Fatalf("updated first sword instead of second: %+v", world.updatedTags)
	}
}

func TestDMPermAppliesFindObjInvisibleVisibility(t *testing.T) {
	baseWorld := func(actorTags []string) *mockDMPermWorld {
		return &mockDMPermWorld{
			players: map[model.PlayerID]model.Player{
				"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:alice": {
					ID:       "creature:alice",
					RoomID:   "room:1",
					Stats:    map[string]int{"class": model.ClassDM},
					Metadata: model.Metadata{Tags: actorTags},
				},
			},
			rooms: map[model.RoomID]model.Room{
				"room:1": {
					ID:      "room:1",
					Objects: model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:invis-sword"}},
				},
			},
			objects: map[model.ObjectInstanceID]model.ObjectInstance{
				"object:invis-sword": {
					ID:                  "object:invis-sword",
					PrototypeID:         "prototype:sword",
					DisplayNameOverride: "장검",
					Location:            model.ObjectLocation{RoomID: "room:1"},
					Metadata:            model.Metadata{Tags: []string{"OINVIS"}},
				},
			},
			prototypes: map[model.PrototypeID]model.ObjectPrototype{
				"prototype:sword": {
					ID:          "prototype:sword",
					DisplayName: "장검",
					Keywords:    []string{"sword"},
				},
			},
		}
	}

	t.Run("without PDINVI", func(t *testing.T) {
		world := baseWorld(nil)
		ctx := &Context{ActorID: "player:alice"}
		status, err := NewDMPermHandler(world)(ctx, ResolvedCommand{Args: []string{"sword"}})
		if err != nil {
			t.Fatalf("handler() error = %v", err)
		}
		if status != StatusDefault {
			t.Fatalf("status = %v, want StatusDefault", status)
		}
		if got, want := ctx.OutputString(), "실패.\n"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
		if len(world.updatedTags) != 0 {
			t.Fatalf("updated invisible object without PDINVI: %+v", world.updatedTags)
		}
	})

	t.Run("with PDINVI", func(t *testing.T) {
		world := baseWorld([]string{"PDINVI"})
		ctx := &Context{ActorID: "player:alice"}
		status, err := NewDMPermHandler(world)(ctx, ResolvedCommand{Args: []string{"sword"}})
		if err != nil {
			t.Fatalf("handler() error = %v", err)
		}
		if status != StatusDefault {
			t.Fatalf("status = %v, want StatusDefault", status)
		}
		if got, want := ctx.OutputString(), "성공.\n"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
		if _, ok := world.updatedTags["object:invis-sword"]; !ok {
			t.Fatalf("PDINVI actor did not update invisible object")
		}
	})
}
