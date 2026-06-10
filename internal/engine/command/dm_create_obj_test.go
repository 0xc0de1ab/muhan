package command

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type mockDMCreateObjWorld struct {
	players          map[model.PlayerID]model.Player
	creatures        map[model.CreatureID]model.Creature
	prototypes       map[model.PrototypeID]model.ObjectPrototype
	createdInstances []model.ObjectInstance
	createdFor       model.CreatureID
	createErr        error
}

func (w *mockDMCreateObjWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMCreateObjWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMCreateObjWorld) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	p, ok := w.prototypes[id]
	return p, ok
}

func (w *mockDMCreateObjWorld) CreateObjectInstanceFromPrototype(protoID model.PrototypeID, creatureID model.CreatureID) (model.ObjectInstance, error) {
	if w.createErr != nil {
		return model.ObjectInstance{}, w.createErr
	}
	proto, ok := w.prototypes[protoID]
	if !ok {
		return model.ObjectInstance{}, fmt.Errorf("prototype %q not found", protoID)
	}

	instance := model.ObjectInstance{
		ID:          model.ObjectInstanceID(string(protoID) + ":instance"),
		PrototypeID: protoID,
		Properties:  make(map[string]string),
	}
	for k, v := range proto.Properties {
		instance.Properties[k] = v
	}
	instance.Metadata.Tags = append(instance.Metadata.Tags, proto.Metadata.Tags...)

	if hasRandomEnchantProto(proto) {
		instance.Metadata.Tags = append(instance.Metadata.Tags, "enchanted", "oencha")
		instance.Properties["adjustment"] = "4"
		instance.Properties["pDice"] = "4"
	}

	w.createdFor = creatureID
	w.createdInstances = append(w.createdInstances, instance)
	return instance, nil
}

func hasRandomEnchantProto(proto model.ObjectPrototype) bool {
	for _, tag := range proto.Metadata.Tags {
		t := strings.ToLower(tag)
		if t == "randomenchantment" || t == "orench" {
			return true
		}
	}
	for k, v := range proto.Properties {
		kl := strings.ToLower(k)
		if kl == "randomenchantment" || kl == "orench" {
			if v == "true" || v == "1" {
				return true
			}
		}
	}
	return false
}

func TestDMCreateObj_Permissions(t *testing.T) {
	tests := []struct {
		name        string
		class       int
		wantStatus  Status
		wantOutput  string
		wantCreated int
	}{
		{
			name:       "regular class below SUB_DM",
			class:      model.ClassInvincible,
			wantStatus: StatusPrompt,
			wantOutput: "",
		},
		{
			name:       "caretaker below SUB_DM",
			class:      model.ClassCaretaker,
			wantStatus: StatusPrompt,
			wantOutput: "",
		},
		{
			name:       "bulsa below SUB_DM",
			class:      model.ClassBulsa,
			wantStatus: StatusPrompt,
			wantOutput: "",
		},
		{
			name:        "class equal to SUB_DM",
			class:       model.ClassSubDM,
			wantStatus:  StatusDefault,
			wantOutput:  "검를 소지품에 추가했습니다.\n",
			wantCreated: 1,
		},
		{
			name:        "class above SUB_DM",
			class:       model.ClassDM,
			wantStatus:  StatusDefault,
			wantOutput:  "검를 소지품에 추가했습니다.\n",
			wantCreated: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMCreateObjWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": tt.class}},
				},
				prototypes: map[model.PrototypeID]model.ObjectPrototype{
					"object:o01:23": {ID: "object:o01:23", DisplayName: "검"},
				},
			}

			ctx := &Context{ActorID: "player:alice"}
			resolved := ResolvedCommand{
				Parsed: commandparse.Command{
					Val: [commandparse.CommandMax]int64{123},
				},
				Spec: commandspec.CommandSpec{
					Name:       "*create_obj",
					Handler:    "dm_create_obj",
					Privileged: true,
				},
			}

			handler := NewDMCreateObjHandler(world)
			status, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}
			if status != tt.wantStatus {
				t.Fatalf("status = %v, want %v", status, tt.wantStatus)
			}

			got := ctx.OutputString()
			if got != tt.wantOutput {
				t.Errorf("output = %q, want %q", got, tt.wantOutput)
			}
			if len(world.createdInstances) != tt.wantCreated {
				t.Errorf("created instances = %d, want %d", len(world.createdInstances), tt.wantCreated)
			}
		})
	}
}

func TestDMCreateObj_PrototypeNotFound(t *testing.T) {
	world := &mockDMCreateObjWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}},
		},
		prototypes: map[model.PrototypeID]model.ObjectPrototype{},
	}

	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Parsed: commandparse.Command{
			Val: [commandparse.CommandMax]int64{123},
		},
	}

	handler := NewDMCreateObjHandler(world)
	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	want := "에러 (123)\n"
	got := ctx.OutputString()
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestDMCreateObj_CreatureBackedActorUsesLegacyCreaturePointer(t *testing.T) {
	world := &mockDMCreateObjWorld{
		creatures: map[model.CreatureID]model.Creature{
			"creature:dm": {ID: "creature:dm", Stats: map[string]int{"class": model.ClassSubDM}},
		},
		prototypes: map[model.PrototypeID]model.ObjectPrototype{
			"object:o01:23": {ID: "object:o01:23", DisplayName: "검"},
		},
	}

	ctx := &Context{ActorID: "creature:dm"}
	resolved := ResolvedCommand{
		Parsed: commandparse.Command{
			Val: [commandparse.CommandMax]int64{123},
		},
		Spec: commandspec.CommandSpec{
			Name:       "*create_obj",
			Handler:    "dm_create_obj",
			Privileged: true,
		},
	}

	status, err := NewDMCreateObjHandler(world)(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %v, want %v", status, StatusDefault)
	}
	if got, want := ctx.OutputString(), "검를 소지품에 추가했습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if world.createdFor != "creature:dm" {
		t.Fatalf("createdFor = %q, want creature:dm", world.createdFor)
	}
	if len(world.createdInstances) != 1 {
		t.Fatalf("created instances = %d, want 1", len(world.createdInstances))
	}
}

func TestDMCreateObj_CreateFailure(t *testing.T) {
	world := &mockDMCreateObjWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}},
		},
		prototypes: map[model.PrototypeID]model.ObjectPrototype{
			"object:o01:23": {ID: "object:o01:23", DisplayName: "검"},
		},
		createErr: errors.New("instantiation error"),
	}

	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Parsed: commandparse.Command{
			Val: [commandparse.CommandMax]int64{123},
		},
	}

	handler := NewDMCreateObjHandler(world)
	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	want := "에러 (123)\n"
	got := ctx.OutputString()
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestDMCreateObj_RandomEnchant(t *testing.T) {
	world := &mockDMCreateObjWorld{
		players: map[model.PlayerID]model.Player{
			"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": 12}},
		},
		prototypes: map[model.PrototypeID]model.ObjectPrototype{
			"object:o01:23": {
				ID:          "object:o01:23",
				DisplayName: "마법검",
				Metadata: model.Metadata{
					Tags: []string{"orench"},
				},
			},
		},
	}

	ctx := &Context{ActorID: "player:alice"}
	resolved := ResolvedCommand{
		Parsed: commandparse.Command{
			Val: [commandparse.CommandMax]int64{123},
		},
	}

	handler := NewDMCreateObjHandler(world)
	_, err := handler(ctx, resolved)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	want := "마법검를 소지품에 추가했습니다.\n"
	got := ctx.OutputString()
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}

	if len(world.createdInstances) != 1 {
		t.Fatalf("expected 1 created instance, got %d", len(world.createdInstances))
	}

	inst := world.createdInstances[0]
	hasEnchanted := false
	for _, tag := range inst.Metadata.Tags {
		if tag == "enchanted" {
			hasEnchanted = true
			break
		}
	}
	if !hasEnchanted {
		t.Errorf("expected created instance to be enchanted, tags: %v", inst.Metadata.Tags)
	}
}
