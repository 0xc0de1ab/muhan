package command

import (
	"strings"
	"testing"

	"muhan/internal/commandspec"
	"muhan/internal/world/model"
)

type mockDMInfoWorld struct {
	players        map[model.PlayerID]model.Player
	creatures      map[model.CreatureID]model.Creature
	rooms          int
	monsters       int
	objects        int
	wanderInterval int
	active         int
	queued         int
}

func (w *mockDMInfoWorld) Player(id model.PlayerID) (model.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

func (w *mockDMInfoWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	c, ok := w.creatures[id]
	return c, ok
}

func (w *mockDMInfoWorld) CacheStats() (rooms, monsters, objects int) {
	return w.rooms, w.monsters, w.objects
}

func (w *mockDMInfoWorld) WanderInterval() int {
	return w.wanderInterval
}

func (w *mockDMInfoWorld) PlayerCounts() (active, queued int) {
	return w.active, w.queued
}

func TestDMInfo(t *testing.T) {
	tests := []struct {
		name       string
		actorID    string
		players    map[model.PlayerID]model.Player
		creatures  map[model.CreatureID]model.Creature
		rooms      int
		monsters   int
		objects    int
		wanderInt  int
		active     int
		queued     int
		wantStatus Status
		wantOutput string
		wantErr    bool
	}{
		{
			name:       "nil context or empty actor ID or nil world",
			actorID:    "",
			wantStatus: StatusDefault,
		},
		{
			name:    "denied permission (class below SUB_DM)",
			actorID: "player:user1",
			players: map[model.PlayerID]model.Player{
				"player:user1": {ID: "player:user1", CreatureID: "creature:user1"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:user1": {ID: "creature:user1", Stats: map[string]int{"class": 9}},
			},
			wantStatus: StatusPrompt,
		},
		{
			name:    "granted permission (class = SUB_DM) success",
			actorID: "player:user1",
			players: map[model.PlayerID]model.Player{
				"player:user1": {ID: "player:user1", CreatureID: "creature:user1"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:user1": {ID: "creature:user1", Stats: map[string]int{"class": legacyClassSubDM}},
			},
			rooms:      123,
			monsters:   456,
			objects:    789,
			wanderInt:  60,
			active:     5,
			queued:     2,
			wantStatus: StatusDefault,
			wantOutput: "Internal Cache Queue Sizes:\n" +
				"   Rooms: 123     Monsters: 456     Objects: 789  \n\n" +
				"Wander update: 60\n" +
				"      Players: 5  Queued: 2\n\n",
		},
		{
			name:    "granted permission (class = 12) success with formatting check",
			actorID: "player:user2",
			players: map[model.PlayerID]model.Player{
				"player:user2": {ID: "player:user2", CreatureID: "creature:user2"},
			},
			creatures: map[model.CreatureID]model.Creature{
				"creature:user2": {ID: "creature:user2", Stats: map[string]int{"class": 12}},
			},
			rooms:      9999,
			monsters:   888,
			objects:    7,
			wanderInt:  15,
			active:     0,
			queued:     0,
			wantStatus: StatusDefault,
			wantOutput: "Internal Cache Queue Sizes:\n" +
				"   Rooms: 9999    Monsters: 888     Objects: 7    \n\n" +
				"Wander update: 15\n" +
				"      Players: 0  Queued: 0\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := &mockDMInfoWorld{
				players:        tt.players,
				creatures:      tt.creatures,
				rooms:          tt.rooms,
				monsters:       tt.monsters,
				objects:        tt.objects,
				wanderInterval: tt.wanderInt,
				active:         tt.active,
				queued:         tt.queued,
			}
			handler := NewDMInfoHandler(world)

			var ctx *Context
			if tt.name != "nil context or empty actor ID or nil world" {
				ctx = &Context{
					ActorID: tt.actorID,
				}
			}

			resolved := ResolvedCommand{
				Spec: commandspec.CommandSpec{
					Name: "dm_info",
				},
			}

			// If tt.name is testing nil world, let's pass nil
			var w DMInfoWorld = world
			if strings.Contains(tt.name, "nil world") {
				w = nil
				handler = NewDMInfoHandler(w)
			}

			status, err := handler(ctx, resolved)
			if (err != nil) != tt.wantErr {
				t.Fatalf("handler returned error: %v, wantErr: %v", err, tt.wantErr)
			}
			if status != tt.wantStatus {
				t.Errorf("status = %v, want %v", status, tt.wantStatus)
			}

			if ctx != nil {
				gotOutput := ctx.OutputString()
				if gotOutput != tt.wantOutput {
					t.Errorf("output = %q, want %q", gotOutput, tt.wantOutput)
				}
			}
		})
	}
}
