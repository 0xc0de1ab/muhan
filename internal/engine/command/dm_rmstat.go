package command

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

// DMRmstatWorld defines the data access interface needed for dm_rmstat.
type DMRmstatWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
}

// NewDMRmstatHandler creates a handler for the dm_rmstat command.
func NewDMRmstatHandler(world DMRmstatWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmRmstat(ctx, resolved, world)
	}
}

func dmRmstat(ctx *Context, resolved ResolvedCommand, world DMRmstatWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	_, creature, ok := dmRmstatActor(world, strings.TrimSpace(ctx.ActorID))
	if !ok {
		return StatusDefault, nil
	}

	class := dmRmstatClass(creature)
	if class != legacyClassZoneMaker && class < model.ClassSubDM {
		return StatusPrompt, nil
	}

	roomNum := parseRoomNumber(creature.RoomID)
	ctx.WriteString(fmt.Sprintf("방번호 #%d\n", roomNum))

	return StatusDefault, nil
}

func dmRmstatActor(world DMRmstatWorld, actorID string) (model.Player, model.Creature, bool) {
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

func dmRmstatClass(creature model.Creature) int {
	return creatureClass(creature)
}

func parseRoomNumber(roomID model.RoomID) int {
	idStr := string(roomID)
	if strings.HasPrefix(idStr, "room:") {
		idStr = idStr[len("room:"):]
	}
	if val, err := strconv.Atoi(idStr); err == nil {
		return val
	}
	return 0
}
