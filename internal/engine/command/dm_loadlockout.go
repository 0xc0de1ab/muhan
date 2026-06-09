package command

import (
	"strings"

	"muhan/internal/world/model"
)

// DMLoadLockoutWorld defines the interface required by the dm_loadlockout command.
type DMLoadLockoutWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	LoadLockouts() error
}

// NewDMLoadLockoutHandler creates a new command handler for dm_loadlockout.
func NewDMLoadLockoutHandler(world DMLoadLockoutWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmLoadLockout(ctx, resolved, world)
	}
}

func dmLoadLockout(ctx *Context, resolved ResolvedCommand, world DMLoadLockoutWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusPrompt, nil
	}

	playerID := model.PlayerID(ctx.ActorID)
	var creatureID model.CreatureID
	if player, ok := world.Player(playerID); ok {
		creatureID = player.CreatureID
	} else {
		creatureID = model.CreatureID(ctx.ActorID)
	}

	creature, ok := world.Creature(creatureID)
	if !ok {
		return StatusPrompt, nil
	}

	if creatureClass(creature) < legacyClassDM {
		return StatusPrompt, nil
	}

	_ = world.LoadLockouts()

	ctx.WriteString("Lockout file read in.\n")
	return StatusDefault, nil
}
