package command

import (
	"strings"

	"muhan/internal/world/model"
)

// DMReplaceWorld defines the repository/world interface for dm_replace.
type DMReplaceWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	UpdateRoomDescription(id model.RoomID, field, val string) error // field is "short" or "long"
}

// NewDMReplaceHandler creates a new Handler for the dm_replace command.
func NewDMReplaceHandler(world DMReplaceWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmReplace(ctx, resolved, world)
	}
}

func dmReplace(ctx *Context, resolved ResolvedCommand, world DMReplaceWorld) (Status, error) {
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

	pattern, val, replace, ok := txtParse(resolved.Input)
	if !ok {
		ctx.WriteString("syntax:*replace <pattern> <replacement>\n")
		return StatusDefault, nil
	}

	roomID := creature.RoomID
	if roomID.IsZero() {
		return StatusDefault, nil
	}

	room, ok := world.Room(roomID)
	if !ok {
		return StatusDefault, nil
	}

	desc := room.ShortDescription
	domain := "short"
	idx := descSearch(desc, pattern, &val)
	if idx == -1 && val > 0 {
		desc = room.LongDescription
		domain = "long"
		idx = descSearch(desc, pattern, &val)
	}

	if idx < 0 {
		ctx.WriteString("Pattern not found.\n")
		return StatusDefault, nil
	}

	newDesc := desc[:idx] + replace + desc[idx+len(pattern):]

	err := world.UpdateRoomDescription(room.ID, domain, newDesc)
	if err != nil {
		return StatusDefault, err
	}

	return StatusDefault, nil
}

// txtParse parses the input string using the legacy txt_parse rules.
func txtParse(input string) (pattern string, val int, replace string, ok bool) {
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
		return "", 1, "", false
	}

	// Extract search pattern
	patStart := i
	for i < len(input) && input[i] != ' ' {
		i++
	}
	pattern = input[patStart:i]

	// Skip spaces after pattern
	for i < len(input) && input[i] == ' ' {
		i++
	}

	val = 1
	// Check number place of pattern
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

	// Remaining string = replacement
	if i < len(input) {
		replace = input[i:]
	}

	if pattern == "" || replace == "" {
		return pattern, val, replace, false
	}

	return pattern, val, replace, true
}

// descSearch searches the given string (desc) for the pattern, and decrements val
// for each match found. If it reaches the target occurrence (when val becomes 0),
// it returns the byte index of the match. Otherwise, it returns -1.
func descSearch(desc, pattern string, val *int) int {
	if desc == "" || pattern == "" || val == nil {
		return -1
	}

	patLen := len(pattern)
	descLen := len(desc)

	for i := 0; i+patLen <= descLen; i++ {
		if desc[i:i+patLen] == pattern {
			(*val)--
			if *val == 0 {
				return i
			}
		}
	}
	return -1
}
