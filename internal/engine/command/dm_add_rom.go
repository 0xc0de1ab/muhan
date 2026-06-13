package command

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

// DMAddRomWorld defines the world dependencies for the dm_add_rom command.
type DMAddRomWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	CreateRoom(model.RoomID) error
}

// NewDMAddRomHandler creates a new command handler for dm_add_rom.
func NewDMAddRomHandler(world DMAddRomWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmAddRom(ctx, resolved, world)
	}
}

func dmAddRom(ctx *Context, resolved ResolvedCommand, world DMAddRomWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	playerID := model.PlayerID(ctx.ActorID)
	var creatureID model.CreatureID
	if player, ok := world.Player(playerID); ok {
		creatureID = player.CreatureID
	} else {
		creatureID = model.CreatureID(ctx.ActorID)
	}

	creature, ok := world.Creature(creatureID)
	if !ok {
		return StatusPrompt, nil
	}

	class := creatureClass(creature)
	if class < model.ClassDM {
		return StatusPrompt, nil
	}

	val := dmAddRomRoomNumber(resolved)
	if val < 2 {
		ctx.WriteString("무엇을 만들죠?\n")
		return StatusDefault, nil
	}

	roomID := dmAddRomRoomID(val)
	if _, ok := world.Room(roomID); ok {
		ctx.WriteString("기존의 방이 존재합니다.\n")
		return StatusDefault, nil
	}

	err := world.CreateRoom(roomID)
	if err != nil {
		ctx.WriteString("에러: Unable open files.\n")
		return StatusDefault, nil
	}

	ctx.WriteString(fmt.Sprintf("방번호 #%d 만들었습니다.\n", val))
	return StatusDefault, nil
}

func dmAddRomRoomNumber(resolved ResolvedCommand) int64 {
	if resolved.Parsed.Num > 1 {
		return resolved.Parsed.Val[1]
	}
	if resolved.Parsed.Val[1] != 0 {
		return resolved.Parsed.Val[1]
	}
	if len(resolved.Values) > 0 && resolved.Values[0] != 0 {
		return resolved.Values[0]
	}
	return resolved.Parsed.Val[0]
}

func dmAddRomRoomID(number int64) model.RoomID {
	if number >= 0 {
		return model.RoomID(fmt.Sprintf("room:%05d", number))
	}
	return model.RoomID("room:" + strconv.FormatInt(number, 10))
}
