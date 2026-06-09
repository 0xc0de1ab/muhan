package command

import (
	"testing"

	"muhan/internal/world/model"
)

type legacyFindWhoTestWorld struct {
	players   map[model.PlayerID]model.Player
	creatures map[model.CreatureID]model.Creature
}

func (w legacyFindWhoTestWorld) Player(id model.PlayerID) (model.Player, bool) {
	player, ok := w.players[id]
	return player, ok
}

func (w legacyFindWhoTestWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	creature, ok := w.creatures[id]
	return creature, ok
}

func TestLegacyFindWhoActivePlayerMatchesLowercizedDisplayNameOnly(t *testing.T) {
	world := legacyFindWhoTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:victim": {
				ID:          "player:victim",
				DisplayName: "Victim",
				CreatureID:  "creature:victim",
			},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:victim": {
				ID:          "creature:victim",
				DisplayName: "Victim",
			},
		},
	}
	ctx := &Context{
		Values: map[string]any{
			"game.activeSessions": func() []activeSession {
				return []activeSession{{ID: "session:victim", ActorID: "player:victim"}}
			},
		},
	}

	player, creature, session, ok := legacyFindWhoActivePlayer(ctx, world, "VICTIM")
	if !ok {
		t.Fatal("legacyFindWhoActivePlayer did not match lowercized display name")
	}
	if player.ID != "player:victim" || creature.ID != "creature:victim" || session.ID != "session:victim" {
		t.Fatalf("matched %q/%q/%q, want victim player/creature/session", player.ID, creature.ID, session.ID)
	}
}

func TestLegacyFindWhoActivePlayerRejectsPlayerIDAliases(t *testing.T) {
	world := legacyFindWhoTestWorld{
		players: map[model.PlayerID]model.Player{
			"player:victim": {
				ID:          "player:victim",
				DisplayName: "홍길동",
				CreatureID:  "creature:victim",
			},
		},
		creatures: map[model.CreatureID]model.Creature{
			"creature:victim": {
				ID:          "creature:victim",
				DisplayName: "홍길동",
			},
		},
	}
	ctx := &Context{
		Values: map[string]any{
			"game.activeSessions": func() []activeSession {
				return []activeSession{{ID: "session:victim", ActorID: "player:victim"}}
			},
		},
	}

	for _, name := range []string{"victim", "player:victim"} {
		if _, _, _, ok := legacyFindWhoActivePlayer(ctx, world, name); ok {
			t.Fatalf("legacyFindWhoActivePlayer matched Go-only ID alias %q", name)
		}
	}
}
