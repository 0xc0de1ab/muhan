package command

import (
	"fmt"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

// DMFingerWorld defines the required interface for the dm_finger command.
type DMFingerWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Finger(addr, name string) (string, error)
}

// NewDMFingerHandler creates a new Handler for the dm_finger command.
func NewDMFingerHandler(world DMFingerWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmFinger(ctx, resolved, world)
	}
}

func dmFinger(ctx *Context, resolved ResolvedCommand, world DMFingerWorld) (Status, error) {
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

	// 1. Enforce player class permissions: SUB_DM (12+).
	class := creatureClass(creature)
	if class < model.ClassSubDM {
		return StatusPrompt, nil
	}

	// 2. Argument count validation.
	if dmFingerMissingArgs(resolved) {
		ctx.WriteString("누구를 Finger검색 합니까?\n")
		return StatusDefault, nil
	}

	firstArg := dmFingerArg(resolved, 0)
	var address string
	if strings.HasPrefix(firstArg, "@") {
		// 3. Address starts with @
		address = firstArg[1:]
	} else {
		// 4. Otherwise, find online player by name.
		targetPlayer, ok := dmFingerFindOnlinePlayer(ctx, world, legacyLowercizeASCII(firstArg, true))
		if !ok {
			ctx.WriteString("완전한 이름을 사용하세요\n")
			return StatusDefault, nil
		}
		var targetCreature model.Creature
		if !targetPlayer.CreatureID.IsZero() {
			targetCreature, _ = world.Creature(targetPlayer.CreatureID)
		}
		address = getAddress(targetPlayer, targetCreature)
	}

	// 5. Output Forking and output messages.
	ctx.WriteString(fmt.Sprintf("Forking to %s.\n", address))
	ctx.WriteString("Output will arrive shortly.\n")

	// Optional second argument if present
	var name string
	if dmFingerExactOptionalName(resolved) {
		name = dmFingerArg(resolved, 1)
	}

	// 6. Delegate to world.Finger(address, name)
	output, err := world.Finger(address, name)
	if err != nil {
		return StatusDefault, err
	}
	ctx.WriteString(output)

	return StatusDefault, nil
}

func dmFingerFindOnlinePlayer(ctx *Context, world DMFingerWorld, name string) (model.Player, bool) {
	player, _, _, ok := legacyFindWhoActivePlayer(ctx, world, name)
	return player, ok
}

func dmFingerMissingArgs(resolved ResolvedCommand) bool {
	if resolved.Parsed.Num > 0 {
		return resolved.Parsed.Num < 2
	}
	return len(resolved.Args) < 1
}

func dmFingerArg(resolved ResolvedCommand, index int) string {
	slot := index + 1
	if resolved.Parsed.Num > slot {
		if arg := strings.TrimSpace(resolved.Parsed.Str[slot]); arg != "" {
			return arg
		}
	}
	if index < 0 || index >= len(resolved.Args) {
		return ""
	}
	return strings.TrimSpace(resolved.Args[index])
}

func dmFingerExactOptionalName(resolved ResolvedCommand) bool {
	if resolved.Parsed.Num > 0 {
		return resolved.Parsed.Num == 3
	}
	return len(resolved.Args) == 2
}
