package command

import (
	"strings"

	"muhan/internal/world/model"
)

type DMSpyWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	SetSpy(spyPlayerID, targetPlayerID model.PlayerID) error
	ClearSpy(spyPlayerID model.PlayerID) error
	IsSpying(spyPlayerID model.PlayerID) (model.PlayerID, bool)
	IsBeingSpiedOn(targetPlayerID model.PlayerID) (model.PlayerID, bool)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	SetCreatureStat(model.CreatureID, string, int) error
}

func NewDMSpyHandler(world DMSpyWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmSpy(ctx, resolved, world)
	}
}

func dmSpy(ctx *Context, resolved ResolvedCommand, world DMSpyWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	casterPlayerID := model.PlayerID(ctx.ActorID)
	var creatureID model.CreatureID
	var ok bool
	if player, ok := world.Player(casterPlayerID); ok {
		creatureID = player.CreatureID
	} else {
		creatureID = model.CreatureID(ctx.ActorID)
	}

	creature, ok := world.Creature(creatureID)
	if !ok {
		return StatusDefault, nil
	}

	// 1. It validates player class permissions: SUB_DM (12+).
	class := creatureClass(creature)
	if class < model.ClassSubDM {
		return StatusPrompt, nil
	}

	// 2. Check if the player currently has the "PSPYON" tag/property enabled. If they do, turn spying off:
	if creatureHasAnyFlag(creature, "PSPYON") {
		// Clear the spy status using world.ClearSpy(casterPlayerID).
		if err := world.ClearSpy(casterPlayerID); err != nil {
			return StatusDefault, err
		}
		if err := world.SetCreatureStat(creatureID, "PSPYON", 0); err != nil {
			return StatusDefault, err
		}
		// Remove "PSPYON" tag from caster.
		if _, err := world.UpdateCreatureTags(creatureID, nil, []string{"PSPYON"}); err != nil {
			return StatusDefault, err
		}
		// Print "감시 끝.\n" to caster.
		ctx.WriteString("감시 끝.\n")
		return StatusDefault, nil
	}

	// 3. If no target player name is given (arg count < 1) and not spying, print "누굴 염탐합니까??\n".
	targetArg, hasTarget := dmSpyTargetArg(resolved)
	if !hasTarget {
		ctx.WriteString("누굴 염탐합니까??\n")
		return StatusDefault, nil
	}

	targetName := legacyLowercizeASCII(targetArg, true)

	// 4. Find target player by name. If not found, print "누굴 감시하려구요.\n".
	targetPlayer, ok := dmSpyFindOnlinePlayer(ctx, world, targetName)
	if !ok {
		ctx.WriteString("누굴 감시하려구요.\n")
		return StatusDefault, nil
	}

	// 5. Check if the target player is already being spied on using world.IsBeingSpiedOn(targetPlayerID).
	// If so, print "그사람을 벌써 감시하고 있습니다.\n".
	if _, beingSpied := world.IsBeingSpiedOn(targetPlayer.ID); beingSpied {
		ctx.WriteString("그사람을 벌써 감시하고 있습니다.\n")
		return StatusDefault, nil
	}

	// 6. If not already spied on, set spy using world.SetSpy(casterPlayerID, targetPlayer.ID).
	if err := world.SetSpy(casterPlayerID, targetPlayer.ID); err != nil {
		return StatusDefault, err
	}
	if err := world.SetCreatureStat(creatureID, "PSPYON", 1); err != nil {
		return StatusDefault, err
	}
	if err := world.SetCreatureStat(creatureID, "PDMINV", 1); err != nil {
		return StatusDefault, err
	}

	// Set "PSPYON" and "PDMINV" tags on the caster.
	if _, err := world.UpdateCreatureTags(creatureID, []string{"PSPYON", "PDMINV"}, nil); err != nil {
		return StatusDefault, err
	}

	// Print "감시 시작.\n".
	ctx.WriteString("감시 시작.\n")
	return StatusDefault, nil
}

func dmSpyFindOnlinePlayer(ctx *Context, world DMSpyWorld, name string) (model.Player, bool) {
	player, _, _, ok := legacyFindWhoActivePlayer(ctx, world, name)
	return player, ok
}

func dmSpyTargetArg(resolved ResolvedCommand) (string, bool) {
	if resolved.Parsed.Num > 0 {
		if resolved.Parsed.Num < 2 {
			return "", false
		}
		return resolved.Parsed.Str[1], true
	}
	if len(resolved.Args) < 1 || strings.TrimSpace(resolved.Args[0]) == "" {
		return "", false
	}
	return resolved.Args[0], true
}
