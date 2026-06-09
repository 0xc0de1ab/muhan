package command

import (
	"fmt"
	"strings"

	"muhan/internal/world/model"
)

type ListActWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	ActiveCreatures() []model.Creature
}

func NewListActHandler(world ListActWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return listAct(ctx, resolved, world)
	}
}

func listAct(ctx *Context, resolved ResolvedCommand, world ListActWorld) (Status, error) {
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
	if class < legacyClassDM {
		return StatusDefault, nil
	}

	ctx.WriteString("현재 활동중인 괴물\n\n이름:\n")
	active := world.ActiveCreatures()
	for _, crt := range active {
		ctx.WriteString(fmt.Sprintf("   %s.\n", cleanDisplayText(crt.DisplayName)))
	}

	return StatusDefault, nil
}
