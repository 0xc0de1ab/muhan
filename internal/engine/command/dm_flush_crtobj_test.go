package command

import (
	"errors"
	"testing"

	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type mockDMFlushCrtObjWorld struct {
	players     map[model.PlayerID]model.Player
	creatures   map[model.CreatureID]model.Creature
	flushCalled bool
	flushError  error
}

func (m *mockDMFlushCrtObjWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := m.players[id]
	return p, ok
}

func (m *mockDMFlushCrtObjWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := m.creatures[id]
	return c, ok
}

func (m *mockDMFlushCrtObjWorld) FlushCrtObj() error {
	m.flushCalled = true
	return m.flushError
}

func TestDMFlushCrtObj(t *testing.T) {
	tests := []struct {
		name       string
		actorID    string
		class      int
		flushError error
		wantFlush  bool
		wantOutput string
	}{
		{
			name:       "unauthorized player class",
			actorID:    "player:alice",
			class:      12, // SubDM/Caretaker
			wantFlush:  false,
			wantOutput: "",
		},
		{
			name:       "authorized DM success",
			actorID:    "player:alice",
			class:      13, // DM
			wantFlush:  true,
			wantOutput: "메모리의 괴물과 물건을 디스크에서 새로 읽어드립니다.\n",
		},
		{
			name:       "authorized higher class success",
			actorID:    "player:alice",
			class:      15, // Higher DM/Builder/etc.
			wantFlush:  true,
			wantOutput: "메모리의 괴물과 물건을 디스크에서 새로 읽어드립니다.\n",
		},
		{
			name:       "flush error",
			actorID:    "player:alice",
			class:      13,
			flushError: errors.New("db error"),
			wantFlush:  true,
			wantOutput: "메모리의 괴물과 물건을 디스크에서 새로 읽어드립니다.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMFlushCrtObjWorld{
				players: map[model.PlayerID]model.Player{
					"player:alice": {ID: "player:alice", CreatureID: "creature:alice"},
				},
				creatures: map[model.CreatureID]model.Creature{
					"creature:alice": {ID: "creature:alice", Stats: map[string]int{"class": tt.class}},
				},
				flushError: tt.flushError,
			}

			ctx := &Context{
				ActorID: tt.actorID,
			}
			resolved := ResolvedCommand{
				Input: "*flush_crtobj",
				Spec: commandspec.CommandSpec{
					Name:       "*flush_crtobj",
					Handler:    "dm_flush_crtobj",
					Privileged: true,
				},
			}

			handler := NewDMFlushCrtObjHandler(world)
			_, err := handler(ctx, resolved)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if world.flushCalled != tt.wantFlush {
				t.Errorf("world.FlushCrtObj called = %v, want %v", world.flushCalled, tt.wantFlush)
			}

			gotOutput := ctx.OutputString()
			if gotOutput != tt.wantOutput {
				t.Errorf("got output = %q, want %q", gotOutput, tt.wantOutput)
			}
		})
	}
}

func TestDMFlushCrtObj_NilChecks(t *testing.T) {
	world := &mockDMFlushCrtObjWorld{}
	handler := NewDMFlushCrtObjHandler(world)

	// Nil context
	_, err := handler(nil, ResolvedCommand{})
	if err != nil {
		t.Fatalf("unexpected error on nil context: %v", err)
	}

	// Empty ActorID
	ctx := &Context{ActorID: ""}
	_, err = handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("unexpected error on empty ActorID: %v", err)
	}

	// Nil world
	handlerNilWorld := NewDMFlushCrtObjHandler(nil)
	ctx = &Context{ActorID: "player:alice"}
	_, err = handlerNilWorld(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("unexpected error on nil world: %v", err)
	}

	// Missing creature
	ctx = &Context{ActorID: "player:bob"}
	_, err = handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("unexpected error on missing creature: %v", err)
	}
}
