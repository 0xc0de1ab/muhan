package command

import (
	"fmt"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type DMParamWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	SetWanderInterval(int)
	WanderInterval() int
	ShutdownTimeRemaining() int64
	ShipSailingInterval() int64
	TimeToSail() int64
	ForceShipSail()
	SetShipSailingInterval(int64)
}

func NewDMParamHandler(world DMParamWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmParam(ctx, resolved, world)
	}
}

func dmParam(ctx *Context, resolved ResolvedCommand, world DMParamWorld) (Status, error) {
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
	if class < model.ClassDM {
		return StatusPrompt, nil
	}

	firstArg, hasArg := dmParamFirstArg(resolved)
	if !hasArg {
		ctx.WriteString("Set what parameter?\n")
		return StatusDefault, nil
	}

	if len(firstArg) == 0 {
		ctx.WriteString("Invalid parameter.\n")
		return StatusDefault, nil
	}

	switch strings.ToLower(firstArg[:1]) {
	case "r":
		val := int(resolved.Parsed.Val[1])
		world.SetWanderInterval(val)
		return StatusPrompt, nil
	case "d":
		ctx.WriteString(fmt.Sprintf(
			"Random Update: %d\nTime to next shutdown: %d\nShip sailing interval %d\nTime to Sail: %d\n",
			world.WanderInterval(),
			world.ShutdownTimeRemaining(),
			world.ShipSailingInterval(),
			world.TimeToSail(),
		))
		return StatusPrompt, nil
	case "s":
		val := resolved.Parsed.Val[1]
		if val == 1 {
			world.ForceShipSail()
		} else {
			world.SetShipSailingInterval(val)
		}
		return StatusPrompt, nil
	default:
		ctx.WriteString("Invalid parameter.\n")
		return StatusDefault, nil
	}
}

func dmParamFirstArg(resolved ResolvedCommand) (string, bool) {
	if resolved.Parsed.Num > 0 {
		if resolved.Parsed.Num < 2 {
			return "", false
		}
		return resolved.Parsed.Str[1], true
	}
	if len(resolved.Args) < 1 {
		return "", false
	}
	return resolved.Args[0], true
}
