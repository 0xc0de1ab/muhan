package command

import (
	"strings"

	"muhan/internal/world/model"
)

type DMFlushCrtObjWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	FlushCrtObj() error
}

func NewDMFlushCrtObjHandler(world DMFlushCrtObjWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmFlushCrtObj(ctx, resolved, world)
	}
}

func dmFlushCrtObj(ctx *Context, resolved ResolvedCommand, world DMFlushCrtObjWorld) (Status, error) {
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
	if class < legacyClassDM {
		return StatusPrompt, nil
	}

	_ = world.FlushCrtObj()

	ctx.WriteString("메모리의 괴물과 물건을 디스크에서 새로 읽어드립니다.\n")
	return StatusDefault, nil
}
