package command

import (
	"fmt"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

// DMInfoWorld defines the world interface for the dm_info command.
type DMInfoWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	CacheStats() (rooms, monsters, objects int)
	WanderInterval() int
	PlayerCounts() (active, queued int)
}

// NewDMInfoHandler creates a Handler for the dm_info command.
func NewDMInfoHandler(world DMInfoWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmInfo(ctx, resolved, world)
	}
}

func dmInfo(ctx *Context, resolved ResolvedCommand, world DMInfoWorld) (Status, error) {
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
	if class < model.ClassSubDM {
		return StatusPrompt, nil
	}

	rooms, monsters, objects := world.CacheStats()
	wanderInterval := world.WanderInterval()
	active, queued := world.PlayerCounts()

	statsStr := fmt.Sprintf(
		"Internal Cache Queue Sizes:\n"+
			"   Rooms: %-5d   Monsters: %-5d   Objects: %-5d\n\n"+
			"Wander update: %d\n"+
			"      Players: %d  Queued: %d\n\n",
		rooms, monsters, objects,
		wanderInterval,
		active, queued,
	)
	ctx.WriteString(statsStr)

	return StatusDefault, nil
}
