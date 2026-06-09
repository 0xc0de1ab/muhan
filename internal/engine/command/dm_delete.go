package command

import (
	"strings"

	"muhan/internal/world/model"
)

// DMDeleteWorld defines the repository/world interface for dm_delete.
type DMDeleteWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	UpdateRoomDescription(roomID model.RoomID, field, val string) error
}

// NewDMDeleteHandler creates a new Handler for the dm_delete command.
func NewDMDeleteHandler(world DMDeleteWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmDelete(ctx, resolved, world)
	}
}

func dmDelete(ctx *Context, resolved ResolvedCommand, world DMDeleteWorld) (Status, error) {
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

	room, ok := world.Room(roomID)
	if !ok {
		return StatusDefault, nil
	}

	flag, value, pattern := parseDeleteInput(resolved.Input)
	if flag == "" && pattern == "" {
		ctx.WriteString("syntax:*delete [-PESLA] <delete_word>\n")
		return StatusDefault, nil
	}

	var dcase int // 0: delete pattern, 1: delete from pattern onwards
	if strings.HasPrefix(flag, "-") {
		if len(flag) <= 1 {
			ctx.WriteString("syntax:*delete [-PESLA] <delete_word>\n")
			return StatusDefault, nil
		}
		switch flag[1] {
		case 'S':
			err := world.UpdateRoomDescription(room.ID, "short", "")
			if err != nil {
				return StatusDefault, err
			}
			return StatusDefault, nil
		case 'L':
			err := world.UpdateRoomDescription(room.ID, "long", "")
			if err != nil {
				return StatusDefault, err
			}
			return StatusDefault, nil
		case 'A':
			err := world.UpdateRoomDescription(room.ID, "short", "")
			if err != nil {
				return StatusDefault, err
			}
			err = world.UpdateRoomDescription(room.ID, "long", "")
			if err != nil {
				return StatusDefault, err
			}
			return StatusDefault, nil
		case 'E':
			dcase = 1
		case 'P', 'D':
			dcase = 0
			if pattern == "" {
				ctx.WriteString("syntax:*delete [-PESLA] <delete_word>\n")
				return StatusDefault, nil
			}
		default:
			ctx.WriteString("syntax:*delete [-PESLA] <delete_word>\n")
			return StatusDefault, nil
		}
	} else {
		// flag does not start with '-'
		pattern = flag
		dcase = 0
	}

	if pattern == "" && dcase != 1 {
		ctx.WriteString("syntax:*delete [-PESLA] <delete_word>\n")
		return StatusDefault, nil
	}

	desc := room.ShortDescription
	domain := "short"
	idx := descSearch(desc, pattern, &value)
	if idx == -1 && value > 0 {
		desc = room.LongDescription
		domain = "long"
		idx = descSearch(desc, pattern, &value)
	}

	if idx < 0 {
		ctx.WriteString("Pattern not found.\n")
		return StatusDefault, nil
	}

	var newDesc string
	if dcase == 1 {
		newDesc = desc[:idx]
	} else {
		newDesc = desc[:idx] + desc[idx+len(pattern):]
	}

	err := world.UpdateRoomDescription(room.ID, domain, newDesc)
	if err != nil {
		return StatusDefault, err
	}

	return StatusDefault, nil
}

// parseDeleteInput parses the command line input for the dm_delete command.
func parseDeleteInput(input string) (flag string, val int, pattern string) {
	i := 0
	// Skip the command word
	for i < len(input) && input[i] != ' ' {
		i++
	}
	// Skip spaces after the command word
	for i < len(input) && input[i] == ' ' {
		i++
	}

	if i >= len(input) {
		return "", 1, ""
	}

	// Extract flag (1st argument)
	flagStart := i
	for i < len(input) && input[i] != ' ' {
		i++
	}
	flag = input[flagStart:i]

	// Skip spaces after flag
	for i < len(input) && input[i] == ' ' {
		i++
	}

	val = 1
	// Check if there is an occurrence number next
	if i < len(input) && input[i] >= '0' && input[i] <= '9' {
		parsedVal := 0
		for i < len(input) && input[i] >= '0' && input[i] <= '9' {
			parsedVal = parsedVal*10 + int(input[i]-'0')
			i++
		}
		if parsedVal < 1 {
			val = 1
		} else {
			val = parsedVal
		}

		// Skip spaces after number
		for i < len(input) && input[i] == ' ' {
			i++
		}
	}

	// Remaining string = pattern
	if i < len(input) {
		pattern = input[i:]
	}

	return flag, val, pattern
}
