package command

import (
	"fmt"
	"strings"

	"muhan/internal/world/model"
)

type DMCreateObjWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
	CreateObjectInstanceFromPrototype(model.PrototypeID, model.CreatureID) (model.ObjectInstance, error)
}

func NewDMCreateObjHandler(world DMCreateObjWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmCreateObj(ctx, resolved, world)
	}
}

func dmCreateObj(ctx *Context, resolved ResolvedCommand, world DMCreateObjWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	creature, ok := dmCreateObjActor(world, strings.TrimSpace(ctx.ActorID))
	if !ok {
		return StatusPrompt, nil
	}

	class := creatureClass(creature)
	if class < model.ClassSubDM {
		return StatusPrompt, nil
	}

	idVal := resolved.Parsed.Val[0]
	protoID := model.PrototypeID(fmt.Sprintf("object:o%02d:%d", idVal/100, idVal%100))
	_, ok = world.ObjectPrototype(protoID)
	if !ok {
		ctx.WriteString(fmt.Sprintf("에러 (%d)\n", idVal))
		return StatusDefault, nil
	}

	instance, err := world.CreateObjectInstanceFromPrototype(protoID, creature.ID)
	if err != nil {
		ctx.WriteString(fmt.Sprintf("에러 (%d)\n", idVal))
		return StatusDefault, nil
	}

	name := getObjectInstanceName(world, instance)
	ctx.WriteString(fmt.Sprintf("%s를 소지품에 추가했습니다.\n", name))
	return StatusDefault, nil
}

func dmCreateObjActor(world DMCreateObjWorld, actorID string) (model.Creature, bool) {
	playerID := model.PlayerID(actorID)
	if player, ok := world.Player(playerID); ok {
		if player.CreatureID.IsZero() {
			return model.Creature{}, false
		}
		return world.Creature(player.CreatureID)
	}

	creatureID := model.CreatureID(actorID)
	creature, ok := world.Creature(creatureID)
	return creature, ok
}

func getObjectInstanceName(world DMCreateObjWorld, object model.ObjectInstance) string {
	if name := cleanDisplayText(object.DisplayNameOverride); name != "" {
		return name
	}
	if name := cleanDisplayText(object.Properties["name"]); name != "" {
		return name
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if name := cleanDisplayText(proto.DisplayName); name != "" && !looksLikeInternalObjectID(name) {
				return name
			}
			if name := cleanDisplayText(proto.Properties["name"]); name != "" {
				return name
			}
			if name := firstObjectKeyName(proto.Properties); name != "" {
				return name
			}
		}
	}
	return string(object.ID)
}
