package command

import (
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type DMPermWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
	UpdateObjectTags(model.ObjectInstanceID, []string, []string) (model.ObjectInstance, error)
}

func NewDMPermHandler(world DMPermWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmPerm(ctx, resolved, world)
	}
}

func dmPerm(ctx *Context, resolved ResolvedCommand, world DMPermWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	// 1. Validate player class permissions: DM (13+).
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
	if class < model.ClassDM {
		return StatusPrompt, nil
	}

	// 2. Find object on current room's floor
	roomID := creature.RoomID
	if roomID.IsZero() {
		ctx.WriteString("실패.\n")
		return StatusDefault, nil
	}
	room, ok := world.Room(roomID)
	if !ok {
		ctx.WriteString("실패.\n")
		return StatusDefault, nil
	}

	target := dmPermTargetArg(resolved)
	ordinal := dmPermTargetOrdinal(resolved)
	object, found := dmPermFindRoomObject(world, room, target, ordinal, inventoryViewerDetectsInvisible(player, creature))
	if !found {
		ctx.WriteString("실패.\n")
		return StatusDefault, nil
	}

	// 3. Set OPERM2 and OTEMPP tags
	_, err := world.UpdateObjectTags(object.ID, []string{"operm2", "otempp", "OPERM2", "OTEMPP"}, nil)
	if err != nil {
		ctx.WriteString("실패.\n")
		return StatusDefault, nil
	}

	ctx.WriteString("성공.\n")
	return StatusDefault, nil
}

func dmPermTargetArg(resolved ResolvedCommand) string {
	if resolved.Parsed.Num > 1 {
		if target := strings.TrimSpace(resolved.Parsed.Str[1]); target != "" {
			return target
		}
	}
	return getArg(resolved, 0)
}

func dmPermTargetOrdinal(resolved ResolvedCommand) int64 {
	if resolved.Parsed.Num > 1 && resolved.Parsed.Val[1] > 0 {
		return resolved.Parsed.Val[1]
	}
	return getOrdinal(resolved, 0)
}

func dmPermFindRoomObject(world DMPermWorld, room model.Room, prefix string, ordinal int64, detectInvisible bool) (model.ObjectInstance, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return model.ObjectInstance{}, false
	}
	if ordinal < 1 {
		ordinal = 1
	}

	var seen int64
	for _, id := range room.Objects.ObjectIDs {
		if id.IsZero() {
			continue
		}
		object, ok := world.Object(id)
		if !ok || !objectLocatedInRoom(object, room.ID) {
			continue
		}
		if !getObjectVisibleForFindObj(world, object, detectInvisible) {
			continue
		}
		if !getObjectMatches(world, object, prefix) {
			continue
		}
		seen++
		if seen == ordinal {
			return object, true
		}
	}
	return model.ObjectInstance{}, false
}
