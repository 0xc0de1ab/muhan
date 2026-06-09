package command

import (
	"strings"

	"muhan/internal/world/model"
)

type legacyFindWhoWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
}

func legacyFindWhoActivePlayer(ctx *Context, world legacyFindWhoWorld, name string) (model.Player, model.Creature, activeSession, bool) {
	name = legacyLowercizeASCII(name, true)
	if name == "" || world == nil {
		return model.Player{}, model.Creature{}, activeSession{}, false
	}
	for _, session := range getActiveSessions(ctx) {
		if strings.TrimSpace(session.ActorID) == "" {
			continue
		}
		player, ok := world.Player(model.PlayerID(session.ActorID))
		if !ok || player.CreatureID.IsZero() {
			continue
		}
		creature, ok := world.Creature(player.CreatureID)
		if !ok {
			continue
		}
		if legacyFindWhoNameMatches(player.DisplayName, name) ||
			legacyFindWhoNameMatches(creature.DisplayName, name) {
			return player, creature, session, true
		}
	}
	return model.Player{}, model.Creature{}, activeSession{}, false
}

func legacyFindWhoNameMatches(displayName, target string) bool {
	displayName = strings.TrimSpace(displayName)
	return displayName != "" && legacyLowercizeASCII(displayName, true) == target
}
