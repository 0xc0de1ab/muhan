package command

import (
	"strings"

	"muhan/internal/world/model"
)

// DMPrependWorld defines the minimum dependencies needed for the dm_prepend command.
type DMPrependWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	UpdateRoomDescription(roomID model.RoomID, field string, val string) error
}

// NewDMPrependHandler creates a new command handler for dm_prepend.
func NewDMPrependHandler(world DMPrependWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmPrepend(ctx, resolved, world)
	}
}

func dmPrepend(ctx *Context, resolved ResolvedCommand, world DMPrependWorld) (Status, error) {
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
	if class < model.ClassDM {
		return StatusPrompt, nil
	}

	input := resolved.Input
	lenInput := len(input)
	i := 0
	// Skip command name
	for i < lenInput {
		if i+1 < lenInput && input[i] == ' ' && input[i+1] != ' ' {
			break
		}
		i++
	}
	i++

	if i >= lenInput {
		ctx.WriteString("syntax: *prepend [-sn] <text>\n")
		return StatusDefault, nil
	}

	ds := false
	nnl := false

	if input[i] == '-' {
		if lenInput-i < 4 {
			ctx.WriteString("syntax: *prepend [-sn] <text>\n")
			return StatusDefault, nil
		}
		i++ // Skip '-'

		if input[i] == 's' {
			ds = true
			if i+1 < lenInput && input[i+1] == 'n' {
				nnl = true
			}
		} else if input[i] == 'n' {
			nnl = true
			if i+1 < lenInput && input[i+1] == 's' {
				ds = true
			}
		}

		// Find next space followed by non-space to get the remaining text
		for i < lenInput {
			if i+1 < lenInput && input[i] == ' ' && input[i+1] != ' ' {
				break
			}
			i++
		}
		i++
		if i >= lenInput {
			ctx.WriteString("syntax: *prepend [-sn] <text>\n")
			return StatusDefault, nil
		}
	}

	roomID := creature.RoomID
	if roomID.IsZero() {
		return StatusDefault, nil
	}

	room, ok := world.Room(roomID)
	if !ok {
		return StatusDefault, nil
	}

	var desc string
	var field string
	if ds {
		field = "short"
		desc = room.ShortDescription
	} else {
		field = "long"
		desc = room.LongDescription
	}

	if desc == "" {
		nnl = true
	}

	text := input[i:]
	var newDesc string
	if nnl {
		newDesc = text + desc
	} else {
		newDesc = text + "\n" + desc
	}

	if err := world.UpdateRoomDescription(roomID, field, newDesc); err != nil {
		return StatusDefault, err
	}

	return StatusDefault, nil
}
