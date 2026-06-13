package command

import (
	"fmt"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const dmNameLegacyRMAX = 9000

type DMObjNameWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
	UpdateObjectProperty(model.ObjectInstanceID, string, string) error
}

func NewDMObjNameHandler(world DMObjNameWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmObjName(ctx, resolved, world)
	}
}

func dmObjName(ctx *Context, resolved ResolvedCommand, world DMObjNameWorld) (Status, error) {
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
	if class < model.ClassSubDM {
		return StatusPrompt, nil
	}

	cmdName := resolved.Command()
	if cmdName == "" {
		cmdName = resolved.Spec.Name
	}
	stripped := stripCommand(resolved.Input, cmdName)
	recognizedFlags := map[string]bool{
		"-d": true,
		"-o": true,
		"-k": true,
	}
	target, ordinal, flag, flagNum, value, missingValue, parseOk := parseCommandArgs(stripped, recognizedFlags)
	if !parseOk {
		ctx.WriteString("어떤 물건을 무슨 이름으로 바꾸고 싶으세요?*oname <object> [#] [-dok] <name>\n")
		return StatusPrompt, nil
	}

	obj, found := dmFindObjectForObjName(world, creature, target, ordinal, inventoryViewerDetectsInvisible(player, creature))

	if !found {
		ctx.WriteString("그런 아이템은 없어요.")
		return StatusPrompt, nil
	}
	if missingValue {
		return StatusPrompt, nil
	}

	var updateKey string
	var outputMsg string

	if flag == "" {
		updateKey = "name"
		outputMsg = "\n이름이 바뀌었습니다."
		truncated := truncateString(value, 79)
		if err := world.UpdateObjectProperty(obj.ID, updateKey, truncated); err != nil {
			return StatusDefault, err
		}
	} else if flag == "-d" {
		updateKey = "description"
		outputMsg = "\n설명이 바뀌었습니다."
		truncated := truncateString(value, 79)
		if err := world.UpdateObjectProperty(obj.ID, updateKey, truncated); err != nil {
			return StatusDefault, err
		}
	} else if flag == "-o" {
		truncated := truncateString(value, 79)
		if err := world.UpdateObjectProperty(obj.ID, "use_output", truncated); err != nil {
			return StatusDefault, err
		}
		if err := world.UpdateObjectProperty(obj.ID, "useOutput", truncated); err != nil {
			return StatusDefault, err
		}
		outputMsg = "\n출력문이 바뀌었습니다."
	} else if flag == "-k" {
		if flagNum >= 1 && flagNum <= 3 {
			truncated := truncateString(value, 19)
			key1 := fmt.Sprintf("key[%d]", flagNum-1)
			key2 := fmt.Sprintf("key/%d", flagNum)
			if err := world.UpdateObjectProperty(obj.ID, key1, truncated); err != nil {
				return StatusDefault, err
			}
			if err := world.UpdateObjectProperty(obj.ID, key2, truncated); err != nil {
				return StatusDefault, err
			}
			outputMsg = "\n키가 바뀌었습니다."
		} else {
			outputMsg = "바뀌었습니다."
		}
	} else {
		updateKey = "name"
		outputMsg = "\n이름이 바뀌었습니다."
		truncated := truncateString(value, 79)
		if err := world.UpdateObjectProperty(obj.ID, updateKey, truncated); err != nil {
			return StatusDefault, err
		}
	}

	ctx.WriteString(outputMsg)
	return StatusDefault, nil
}

// Shared helpers for dm_obj_name and dm_crt_name command parsing.

func dmFindObjectForObjName(world DMObjNameWorld, creature model.Creature, name string, ordinal int, detectInvisible bool) (model.ObjectInstance, bool) {
	count := ordinal
	if count < 1 {
		count = 1
	}

	if obj, found := dmFindObjInCreatureInventory(world, creature, name, int64(count), detectInvisible); found {
		return obj, true
	}

	if creature.RoomID.IsZero() {
		return model.ObjectInstance{}, false
	}
	room, ok := world.Room(creature.RoomID)
	if !ok {
		return model.ObjectInstance{}, false
	}
	return dmFindObjInRoom(world, room, name, int64(count), detectInvisible)
}

func stripCommand(input, verb string) string {
	input = strings.TrimSpace(input)
	verb = strings.TrimSpace(verb)
	if verb == "" {
		return input
	}
	if stripped, ok := stripCommandAtTextEdge(input, verb); ok {
		return stripped
	}
	return input
}

func parseCommandArgs(stripped string, recognizedFlags map[string]bool) (target string, ordinal int, flag string, flagNum int, value string, missingValue bool, ok bool) {
	if len(strings.Fields(stripped)) < 1 {
		return "", 0, "", 0, "", false, false
	}

	idx := 0
	for idx < len(stripped) && isSpaceRune(rune(stripped[idx])) {
		idx++
	}
	targetStart := idx
	for idx < len(stripped) && !isSpaceRune(rune(stripped[idx])) {
		idx++
	}
	target = stripped[targetStart:idx]
	for idx < len(stripped) && isSpaceRune(rune(stripped[idx])) {
		idx++
	}

	ordinal = 1
	if idx < len(stripped) && stripped[idx] >= '0' && stripped[idx] <= '9' {
		ordinal = legacyAtoiPrefix(stripped[idx:])
		if ordinal < 1 {
			ordinal = 1
		}
		for idx < len(stripped) && stripped[idx] >= '0' && stripped[idx] <= '9' {
			idx++
		}
		for idx < len(stripped) && isSpaceRune(rune(stripped[idx])) {
			idx++
		}
	}

	if idx < len(stripped) && stripped[idx] == '-' {
		currentFlag := ""
		if idx+1 < len(stripped) {
			currentFlag = stripped[idx : idx+2]
		} else {
			currentFlag = stripped[idx:]
		}
		if recognizedFlags[currentFlag] {
			flag = currentFlag
			idx += len(currentFlag)
			switch flag {
			case "-k":
				flagNum = legacyAtoiPrefix(stripped[idx:])
				if flagNum < 1 || flagNum > 3 {
					flagNum = 0
				}
				for idx < len(stripped) && stripped[idx] >= '0' && stripped[idx] <= '9' {
					idx++
				}
			case "-m":
				flagNum = legacyAtoiPrefix(stripped[idx:])
				if flagNum < 1 || flagNum > dmNameLegacyRMAX {
					flagNum = 0
				}
				for idx < len(stripped) && stripped[idx] >= '0' && stripped[idx] <= '9' {
					idx++
				}
			}
			for idx < len(stripped) && isSpaceRune(rune(stripped[idx])) {
				idx++
			}
		}
	}

	if idx < len(stripped) {
		value = stripped[idx:]
	}
	if value == "" {
		return target, ordinal, flag, flagNum, "", true, true
	}
	return target, ordinal, flag, flagNum, value, false, true
}

func isSpaceRune(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func truncateString(s string, limit int) string {
	return legacyTruncateBytes(s, limit)
}
