package command

import (
	"fmt"
	"strings"

	"muhan/internal/world/model"
)

type ListCharmWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	PlayerCharmedCreatures(model.PlayerID) ([]string, error)
}

func NewListCharmHandler(world ListCharmWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return listCharm(ctx, resolved, world)
	}
}

func listCharm(ctx *Context, resolved ResolvedCommand, world ListCharmWorld) (Status, error) {
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
	if class < legacyClassSubDM {
		return StatusPrompt, nil
	}

	targetName, ok := listCharmTargetName(resolved)
	if !ok {
		ctx.WriteString("누구의 최면자를 봅니까?\n")
		return StatusPrompt, nil
	}

	targetName = legacyLowercizeASCII(targetName, true)
	targetPlayer, targetCreature, ok := listCharmFindOnlinePlayer(ctx, world, targetName)
	if !ok {
		ctx.WriteString(fmt.Sprintf("%s은 없습니다.\n", targetName))
		return StatusDefault, nil
	}

	displayName := targetPlayer.DisplayName
	if targetCreature.DisplayName != "" {
		displayName = targetCreature.DisplayName
	}

	ctx.WriteString(fmt.Sprintf("%s의 피최면자:\n", displayName))

	charmed, err := world.PlayerCharmedCreatures(targetPlayer.ID)
	if err != nil {
		return StatusDefault, err
	}

	if len(charmed) == 0 {
		ctx.WriteString("없음.\n")
	} else {
		for _, name := range charmed {
			ctx.WriteString(fmt.Sprintf("%s.\n", name))
		}
	}

	return StatusDefault, nil
}

func listCharmFindOnlinePlayer(ctx *Context, world ListCharmWorld, name string) (model.Player, model.Creature, bool) {
	player, creature, _, ok := legacyFindWhoActivePlayer(ctx, world, name)
	return player, creature, ok
}

func listCharmTargetName(resolved ResolvedCommand) (string, bool) {
	if resolved.Parsed.Num > 0 {
		if resolved.Parsed.Num < 2 {
			return "", false
		}
		target := strings.TrimSpace(resolved.Parsed.Str[1])
		return target, target != ""
	}
	target := getArg(resolved, 0)
	return target, target != ""
}
