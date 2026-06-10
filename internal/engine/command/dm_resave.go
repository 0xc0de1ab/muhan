package command

import (
	"strings"

	"muhan/internal/world/model"
)

// DMResaveWorld defines the interface required by the dmResave command.
type DMResaveWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	ResaveRoom(model.RoomID) error
}

// NewDMResaveHandler creates a new command handler for dm_resave.
func NewDMResaveHandler(world DMResaveWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmResave(ctx, resolved, world)
	}
}

func dmResave(ctx *Context, resolved ResolvedCommand, world DMResaveWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" {
		return StatusPrompt, nil
	}

	player, creature, ok := resolveActor(world, strings.TrimSpace(ctx.ActorID))
	if !ok {
		return StatusPrompt, nil
	}

	class := creatureClass(creature)
	if class < model.ClassDM {
		return StatusPrompt, nil
	}

	roomID := creature.RoomID
	if roomID.IsZero() {
		roomID = player.RoomID
	}
	if roomID.IsZero() {
		ctx.WriteString("저장 실패.\n")
		return StatusDefault, nil
	}

	if err := world.ResaveRoom(roomID); err != nil {
		ctx.WriteString("저장 실패.\n")
	} else {
		ctx.WriteString("Ok.\n")
	}

	return StatusDefault, nil
}

func resolveActor(world DMResaveWorld, actorID string) (model.Player, model.Creature, bool) {
	playerID := model.PlayerID(actorID)
	if player, ok := world.Player(playerID); ok {
		if player.CreatureID.IsZero() {
			return player, model.Creature{}, false
		}
		creature, ok := world.Creature(player.CreatureID)
		return player, creature, ok
	}

	creatureID := model.CreatureID(actorID)
	creature, ok := world.Creature(creatureID)
	if !ok {
		return model.Player{}, model.Creature{}, false
	}
	if !creature.PlayerID.IsZero() {
		if player, ok := world.Player(creature.PlayerID); ok {
			return player, creature, true
		}
	}
	return model.Player{}, creature, true
}
