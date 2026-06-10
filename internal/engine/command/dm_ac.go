package command

import (
	"fmt"
	"strings"

	"muhan/internal/world/model"
)

type DMAcWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	SetCreatureStat(model.CreatureID, string, int) error
}

func NewDMAcHandler(world DMAcWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmAc(ctx, resolved, world)
	}
}

func dmAc(ctx *Context, resolved ResolvedCommand, world DMAcWorld) (Status, error) {
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

	var targetCreature model.Creature
	if targetName, ok := dmAcSingleTargetArg(resolved); ok {
		targetName = legacyLowercizeASCII(targetName, true)
		_, targetCrt, ok := findOnlinePlayer(ctx, world, targetName)
		if !ok {
			ctx.WriteString(fmt.Sprintf("%s가 없습니다.\n", targetName))
			return StatusDefault, nil
		}
		targetCreature = targetCrt
	} else {
		hpMax := creatureStat(creature, "hpMax")
		if err := world.SetCreatureStat(creature.ID, "hpCurrent", hpMax); err != nil {
			return StatusDefault, err
		}
		mpMax := creatureStat(creature, "mpMax")
		if err := world.SetCreatureStat(creature.ID, "mpCurrent", mpMax); err != nil {
			return StatusDefault, err
		}
		targetCreature = creature
	}

	armor := creatureStat(targetCreature, "armor")
	thaco := creatureStat(targetCreature, "thaco")
	ac := (100 - armor) / 2

	ctx.WriteString(fmt.Sprintf("AC: %d  THAC0: %d\n", ac, thaco))
	return StatusDefault, nil
}

func dmAcSingleTargetArg(resolved ResolvedCommand) (string, bool) {
	if resolved.Parsed.Num > 0 {
		if resolved.Parsed.Num != 2 {
			return "", false
		}
		target := strings.TrimSpace(resolved.Parsed.Str[1])
		return target, target != ""
	}

	var target string
	count := 0
	for _, arg := range resolved.Args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		count++
		if count > 1 {
			return "", false
		}
		target = arg
	}
	return target, count == 1
}

func findOnlinePlayer(ctx *Context, world DMAcWorld, name string) (model.Player, model.Creature, bool) {
	player, creature, _, ok := legacyFindWhoActivePlayer(ctx, world, name)
	return player, creature, ok
}
