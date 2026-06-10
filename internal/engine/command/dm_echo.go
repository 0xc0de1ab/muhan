package command

import (
	"fmt"
	"strings"

	"muhan/internal/world/model"
)

type DMEchoWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
}

func NewDMEchoHandler(world DMEchoWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmEcho(ctx, resolved, world)
	}
}

func dmEcho(ctx *Context, resolved ResolvedCommand, world DMEchoWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
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
		return StatusPrompt, nil
	}

	message := extractCommandMessage(resolved)
	if message == "" {
		message = strings.TrimSpace(strings.Join(resolved.Args, " "))
	}
	if message == "" {
		ctx.WriteString("무슨말을 방의 사람들에게 알리죠?")
		return StatusDefault, nil
	}

	roomID := creature.RoomID
	if !roomID.IsZero() {
		if fn, ok := ctx.Values[ContextRoomBroadcastKey].(RoomBroadcastFunc); ok && fn != nil {
			_ = fn(roomID, "", fmt.Sprintf("\n%s", message))
		}
	}

	return StatusDefault, nil
}
