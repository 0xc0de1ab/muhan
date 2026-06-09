package command

import (
	"strings"

	"muhan/internal/persist/legacykr"
	"muhan/internal/world/model"
)

// DMNameroomWorld defines the minimum world interface required for the dm_nameroom command.
type DMNameroomWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	SetRoomName(roomID model.RoomID, name string) error
}

// NewDMNameroomHandler creates a new Handler for the dm_nameroom command.
func NewDMNameroomHandler(world DMNameroomWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmNameroom(ctx, resolved, world)
	}
}

func dmNameroom(ctx *Context, resolved ResolvedCommand, world DMNameroomWorld) (Status, error) {
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

	roomID := creature.RoomID
	if roomID.IsZero() {
		return StatusDefault, nil
	}

	_, ok = world.Room(roomID)
	if !ok {
		return StatusDefault, nil
	}

	argument := dmCommandArgumentText(resolved)
	if argument == "" {
		ctx.WriteString("무엇으로 이름을 바꿉니까?\n")
		return StatusDefault, nil
	}

	if byteLen(argument) > 79 {
		ctx.WriteString("이름이 너무 깁니다.\n")
		return StatusDefault, nil
	}

	if err := world.SetRoomName(roomID, argument); err != nil {
		return StatusDefault, err
	}

	ctx.WriteString("이름을 변경하였습니다.\n")
	return StatusDefault, nil
}

func byteLen(s string) int {
	encoded, err := legacykr.EncodeEUCKR(s)
	if err != nil {
		return len(s)
	}
	return len(encoded)
}
