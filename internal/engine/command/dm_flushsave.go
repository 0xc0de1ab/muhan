package command

import (
	"strings"

	"muhan/internal/world/model"
)

// DMFlushsaveWorld defines the interface required by the dmFlushsave command.
type DMFlushsaveWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	ResaveAllRooms(permOnly bool) error
}

type dmFlushsaveBoardWorld interface {
	FlushDirtyBoardsAndFamilyNews(since int64) error
}

// NewDMFlushsaveHandler creates a new command handler for dm_flushsave.
func NewDMFlushsaveHandler(world DMFlushsaveWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmFlushsave(ctx, resolved, world)
	}
}

func dmFlushsave(ctx *Context, resolved ResolvedCommand, world DMFlushsaveWorld) (Status, error) {
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

	creature, ok := world.Creature(creatureID)
	if !ok {
		return StatusDefault, nil
	}

	class := creatureClass(creature)
	if class < legacyClassDM {
		return StatusPrompt, nil
	}

	if !dmFlushsavePermOnly(resolved) {
		ctx.WriteString("All rooms and contents flushed to disk.\n")
		_ = world.ResaveAllRooms(false)
	} else {
		ctx.WriteString("All rooms and PERM contents flushed to disk.\n")
		_ = world.ResaveAllRooms(true)
	}

	if boardWorld, ok := world.(dmFlushsaveBoardWorld); ok {
		if err := boardWorld.FlushDirtyBoardsAndFamilyNews(0); err != nil {
			return StatusDefault, err
		}
	}

	return StatusDefault, nil
}

func dmFlushsavePermOnly(resolved ResolvedCommand) bool {
	if resolved.Parsed.Num > 0 {
		return resolved.Parsed.Num >= 2
	}
	return len(resolved.Args) >= 1
}
