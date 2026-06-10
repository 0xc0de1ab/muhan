package command

import (
	"strings"

	"muhan/internal/world/model"
)

type DMBroadechoWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	BroadcastAll(message string) error
}

func NewDMBroadechoHandler(world DMBroadechoWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmBroadecho(ctx, resolved, world)
	}
}

func dmBroadecho(ctx *Context, resolved ResolvedCommand, world DMBroadechoWorld) (Status, error) {
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
		ctx.WriteString("무얼 방송합니까?\n")
		return StatusDefault, nil
	}

	var broadcastMsg string
	respectNoBroadcast := false
	if message[0] == '-' {
		if len(message) > 3 && message[1] == 'n' {
			broadcastMsg = "\n" + message[3:]
			respectNoBroadcast = true
		} else {
			return StatusDefault, nil
		}
	} else {
		broadcastMsg = "\n### " + message
	}

	dmBroadechoBroadcast(ctx, world, broadcastMsg, respectNoBroadcast)

	return StatusDefault, nil
}

func dmBroadechoBroadcast(ctx *Context, world DMBroadechoWorld, message string, respectNoBroadcast bool) {
	if !respectNoBroadcast || ctx == nil || ctx.Values == nil || ctx.Values["game.sendToSession"] == nil {
		_ = world.BroadcastAll(message)
		return
	}
	sessions := getActiveSessions(ctx)
	if len(sessions) == 0 {
		_ = world.BroadcastAll(message)
		return
	}
	for _, session := range sessions {
		if session.ActorID == "" {
			continue
		}
		creature, ok := resolveActorCreature(world, session.ActorID)
		if !ok || creatureHasAnyFlag(creature, "PNOBRD", "noBroadcast") {
			continue
		}
		_ = dmSendSendToSession(ctx, session.ID, message)
	}
}
