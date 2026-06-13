package command

import (
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type DMAppendWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	UpdateRoomDescription(roomID model.RoomID, field string, val string) error
}

func NewDMAppendHandler(world DMAppendWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmAppend(ctx, resolved, world)
	}
}

func dmAppend(ctx *Context, resolved ResolvedCommand, world DMAppendWorld) (Status, error) {
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

	text, ds, nnl, ok := parseDMAppend(dmCommandArgumentText(resolved))
	if !ok {
		ctx.WriteString("syntax: *append [-sn] <text>\n")
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

	var desc string
	var field string
	if ds {
		desc = room.ShortDescription
		field = "short"
	} else {
		desc = room.LongDescription
		field = "long"
	}

	if desc == "" {
		nnl = true
	}

	var newDesc string
	if nnl {
		newDesc = desc + text
	} else {
		newDesc = desc + "\n" + text
	}

	err := world.UpdateRoomDescription(room.ID, field, newDesc)
	if err != nil {
		return StatusDefault, err
	}

	return StatusDefault, nil
}

func parseDMAppend(input string) (text string, ds bool, nnl bool, ok bool) {
	input = strings.TrimLeft(input, " \t\r\n")
	if strings.TrimSpace(input) == "" {
		return "", false, false, false
	}

	i := 0
	startIdx := i
	if input[startIdx] == '-' {
		if len(input[startIdx:]) < 4 {
			return "", false, false, false
		}
		i++
		optChar1 := input[startIdx+1]
		if optChar1 == 's' {
			ds = true
			if startIdx+2 < len(input) && input[startIdx+2] == 'n' {
				nnl = true
			}
		} else if optChar1 == 'n' {
			nnl = true
			if startIdx+2 < len(input) && input[startIdx+2] == 's' {
				ds = true
			}
		}

		// Loop to find start of text
		for i < len(input) {
			if nnl && input[i] == ' ' {
				break
			}
			if input[i] == ' ' && i+1 < len(input) && input[i+1] != ' ' {
				break
			}
			i++
		}
		i++
		if i >= len(input) {
			return "", false, false, false
		}
		text = input[i:]
	} else {
		text = input[startIdx:]
	}

	if text == "" {
		return "", false, false, false
	}
	return text, ds, nnl, true
}

func dmCommandArgumentText(resolved ResolvedCommand) string {
	input := strings.TrimSpace(resolved.Input)
	if input == "" {
		return ""
	}

	for _, command := range dmCommandNameCandidates(resolved) {
		if stripped, ok := stripCommandAtTextEdge(input, command); ok {
			return stripped
		}
	}

	i := 0
	for i < len(input) && input[i] != ' ' {
		i++
	}
	for i < len(input) && input[i] == ' ' {
		i++
	}
	if i >= len(input) {
		return ""
	}
	return strings.TrimSpace(input[i:])
}

func dmCommandNameCandidates(resolved ResolvedCommand) []string {
	raw := []string{
		resolved.Command(),
		resolved.CmdName,
		resolved.Spec.Name,
		resolved.Spec.Handler,
	}

	seen := map[string]struct{}{}
	candidates := make([]string, 0, len(raw))
	for _, command := range raw {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		key := strings.ToLower(command)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, command)
	}
	return candidates
}

func stripCommandAtTextEdge(input, command string) (string, bool) {
	command = strings.TrimSpace(command)
	if command == "" || len(input) < len(command) {
		return "", false
	}
	if strings.EqualFold(input, command) {
		return "", true
	}

	if len(input) > len(command) && input[len(command)] == ' ' && strings.EqualFold(input[:len(command)], command) {
		return strings.TrimSpace(input[len(command):]), true
	}

	start := len(input) - len(command)
	if start > 0 && input[start-1] == ' ' && strings.EqualFold(input[start:], command) {
		before := input[:start-1]
		if strings.TrimSpace(before) == "" {
			return "", true
		}
		return before, true
	}
	return "", false
}
