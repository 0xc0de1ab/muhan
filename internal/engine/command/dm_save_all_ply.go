package command

import (
	"strings"

	"muhan/internal/world/model"
)

type DMSaveAllPlyWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	SaveAllPlayers() error
}

func NewDMSaveAllPlyHandler(world DMSaveAllPlyWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmSaveAllPly(ctx, resolved, world)
	}
}

func dmSaveAllPly(ctx *Context, resolved ResolvedCommand, world DMSaveAllPlyWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	playerID := model.PlayerID(ctx.ActorID)
	var creatureID model.CreatureID
	if player, ok := world.Player(playerID); ok {
		creatureID = player.CreatureID
	} else {
		creatureID = model.CreatureID(ctx.ActorID)
	}

	_, ok := world.Creature(creatureID)
	if !ok {
		return StatusDefault, nil
	}

	_ = world.SaveAllPlayers()

	return StatusDefault, nil
}
