package command

import (
	"fmt"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type ListEnmWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	FindCreatureInRoom(model.RoomID, string) (model.Creature, bool)
	CreatureEnemies(model.CreatureID) ([]string, error)
}

func NewListEnmHandler(world ListEnmWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return listEnm(ctx, resolved, world)
	}
}

func listEnm(ctx *Context, resolved ResolvedCommand, world ListEnmWorld) (Status, error) {
	if ctx == nil || ctx.ActorID == "" || world == nil {
		return StatusDefault, nil
	}

	playerID := model.PlayerID(ctx.ActorID)
	var creatureID model.CreatureID
	var player model.Player
	var ok bool
	if player, ok = world.Player(playerID); ok {
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
		return StatusDefault, nil
	}

	roomID := creature.RoomID
	if roomID.IsZero() {
		return StatusDefault, nil
	}
	room, ok := world.Room(roomID)
	if !ok {
		return StatusDefault, nil
	}

	targetName := listEnmTargetArg(resolved)
	crt, ok := dmFindMonsterInRoomForActor(world, creature, room.ID, targetName, listEnmTargetOrdinal(resolved))
	if !ok {
		ctx.WriteString("그런 괴물이 없습니다.\n")
		return StatusDefault, nil
	}

	ctx.WriteString(fmt.Sprintf("%s의 적들:\n", crt.DisplayName))

	enemies, err := world.CreatureEnemies(crt.ID)
	if err != nil {
		return StatusDefault, err
	}

	if len(enemies) == 0 {
		ctx.WriteString("없음.\n")
	} else {
		for _, enemy := range enemies {
			ctx.WriteString(fmt.Sprintf("%s.\n", enemy))
		}
	}

	return StatusDefault, nil
}

func listEnmTargetArg(resolved ResolvedCommand) string {
	if resolved.Parsed.Num > 1 {
		if target := strings.TrimSpace(resolved.Parsed.Str[1]); target != "" {
			return target
		}
	}
	return getArg(resolved, 0)
}

func listEnmTargetOrdinal(resolved ResolvedCommand) int64 {
	if resolved.Parsed.Num > 1 && resolved.Parsed.Val[1] > 0 {
		return resolved.Parsed.Val[1]
	}
	return getOrdinal(resolved, 0)
}
