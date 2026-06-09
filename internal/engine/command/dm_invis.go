package command

import (
	"strings"

	"muhan/internal/textfmt"
	"muhan/internal/world/model"
)

type DMInvisWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	SetCreatureStat(model.CreatureID, string, int) error
}

func NewDMInvisHandler(world DMInvisWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmInvis(ctx, resolved, world)
	}
}

func dmInvis(ctx *Context, resolved ResolvedCommand, world DMInvisWorld) (Status, error) {
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
	if class < legacyClassSubDM {
		return StatusPrompt, nil
	}

	opts := textOptionsFromContext(ctx)
	isInvis := creatureHasAnyFlag(creature, "PDMINV", "dmInvisible")
	if isInvis {
		if err := world.SetCreatureStat(creatureID, "PDMINV", 0); err != nil {
			return StatusDefault, err
		}
		if err := world.SetCreatureStat(creatureID, "dmInvisible", 0); err != nil {
			return StatusDefault, err
		}
		if _, err := world.UpdateCreatureTags(creatureID, nil, []string{"PDMINV", "dmInvisible"}); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(textfmt.RenderLegacyColors("{보투명 해제.\n}", opts))
	} else {
		if err := world.SetCreatureStat(creatureID, "PDMINV", 1); err != nil {
			return StatusDefault, err
		}
		if _, err := world.UpdateCreatureTags(creatureID, []string{"PDMINV"}, nil); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(textfmt.RenderLegacyColors("{노투명 설정.\n}", opts))
	}

	return StatusDefault, nil
}
