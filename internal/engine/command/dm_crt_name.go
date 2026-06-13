package command

import (
	"fmt"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type DMCrtNameWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	FindCreatureInRoom(model.RoomID, string) (model.Creature, bool)
	UpdateCreatureProperty(model.CreatureID, string, string) error
}

func NewDMCrtNameHandler(world DMCrtNameWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmCrtName(ctx, resolved, world)
	}
}

func dmCrtName(ctx *Context, resolved ResolvedCommand, world DMCrtNameWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	playerID := model.PlayerID(ctx.ActorID)
	var creatureID model.CreatureID
	var ok bool
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
	if class < model.ClassDM {
		return StatusPrompt, nil
	}

	cmdName := resolved.Command()
	if cmdName == "" {
		cmdName = resolved.Spec.Name
	}
	stripped := stripCommand(resolved.Input, cmdName)
	recognizedFlags := map[string]bool{
		"-d": true,
		"-t": true,
		"-k": true,
		"-m": true,
	}
	target, ordinal, flag, flagNum, value, missingValue, parseOk := parseCommandArgs(stripped, recognizedFlags)
	if !parseOk {
		ctx.WriteString("어떤 몹을 무슨 이름으로 바꾸시려구요?<몹이름> [#] [-dtmk] <이름> *cname")
		return StatusPrompt, nil
	}

	roomID := creature.RoomID
	if roomID.IsZero() {
		return StatusDefault, nil
	}

	crt, found := dmFindMonsterInRoomForActor(world, creature, roomID, target, int64(ordinal))
	if !found {
		ctx.WriteString("이 방에 그런 것은 없습니다.")
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
		if err := world.UpdateCreatureProperty(crt.ID, updateKey, truncated); err != nil {
			return StatusDefault, err
		}
	} else if flag == "-d" {
		updateKey = "description"
		outputMsg = "\n출력문이 바뀌었습니다."
		truncated := truncateString(value, 79)
		if err := world.UpdateCreatureProperty(crt.ID, updateKey, truncated); err != nil {
			return StatusDefault, err
		}
	} else if flag == "-t" {
		truncated := truncateString(value, 79)
		if err := world.UpdateCreatureProperty(crt.ID, "talk", truncated); err != nil {
			return StatusDefault, err
		}
		if err := world.UpdateCreatureProperty(crt.ID, "talks", truncated); err != nil {
			return StatusDefault, err
		}
		outputMsg = "\n대화문이 바뀌었습니다."
	} else if flag == "-k" {
		if flagNum >= 1 && flagNum <= 3 {
			truncated := truncateString(value, 19)
			key1 := fmt.Sprintf("key[%d]", flagNum-1)
			key2 := fmt.Sprintf("key/%d", flagNum)
			if err := world.UpdateCreatureProperty(crt.ID, key1, truncated); err != nil {
				return StatusDefault, err
			}
			if err := world.UpdateCreatureProperty(crt.ID, key2, truncated); err != nil {
				return StatusDefault, err
			}
			outputMsg = "\n키가 바뀌었습니다."
		} else {
			outputMsg = "바뀌었습니다."
		}
	} else if flag == "-m" {
		if flagNum > 0 {
			ctx.WriteString("\n몹의 위치가 그 방향에는 방이 없습니다.")
			return StatusDefault, nil
		}
		outputMsg = "바뀌었습니다."
	} else {
		updateKey = "name"
		outputMsg = "\n이름이 바뀌었습니다."
		truncated := truncateString(value, 79)
		if err := world.UpdateCreatureProperty(crt.ID, updateKey, truncated); err != nil {
			return StatusDefault, err
		}
	}

	ctx.WriteString(outputMsg)
	return StatusDefault, nil
}
