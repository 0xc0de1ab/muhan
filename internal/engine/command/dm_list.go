package command

import (
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

// DMListWorld defines the required interface for the dm_list command.
type DMListWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	List(args []string) (string, error)
}

// NewDMListHandler creates a new Handler for the dm_list command.
func NewDMListHandler(world DMListWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmList(ctx, resolved, world)
	}
}

func dmList(ctx *Context, resolved ResolvedCommand, world DMListWorld) (Status, error) {
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

	// 1. Enforce SUB_DM (12+) permission validation.
	class := creatureClass(creature)
	if class < model.ClassSubDM {
		return StatusPrompt, nil
	}

	// 2. If argument count is < 2 (resolved.Parsed.Num < 2), print "무엇의 리스트를 봅니까?\n" and return StatusDefault.
	if resolved.Parsed.Num < 2 {
		ctx.WriteString("무엇의 리스트를 봅니까?\n")
		return StatusDefault, nil
	}

	// C passes only cmnd->str[1] through cmnd->str[4] to the forked list tool.
	output, err := world.List(dmListArgs(resolved))
	if err != nil {
		return StatusDefault, err
	}
	ctx.WriteString(output)

	return StatusDefault, nil
}

func dmListArgs(resolved ResolvedCommand) []string {
	if resolved.Parsed.Num > 0 {
		limit := resolved.Parsed.Num
		if limit > len(resolved.Parsed.Str) {
			limit = len(resolved.Parsed.Str)
		}
		if limit > 5 {
			limit = 5
		}
		args := make([]string, 0, limit-1)
		hasParsedSlot := false
		for i := 1; i < limit; i++ {
			if resolved.Parsed.Str[i] != "" {
				hasParsedSlot = true
			}
			args = append(args, resolved.Parsed.Str[i])
		}
		if hasParsedSlot || len(resolved.Args) == 0 {
			return args
		}
	}

	args := resolved.Args
	if len(args) > 4 {
		args = args[:4]
	}
	return args
}
