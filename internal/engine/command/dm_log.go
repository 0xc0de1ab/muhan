package command

import (
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type DMLogWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	ReadLogFile(name string) (string, error)
	DeleteLogFile(name string) error
}

func NewDMLogHandler(world DMLogWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmLog(ctx, resolved, world)
	}
}

func dmLog(ctx *Context, resolved ResolvedCommand, world DMLogWorld) (Status, error) {
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

	arg := dmLogArg(resolved)
	if dmLogArgCount(resolved) == 1 && arg == "r" {
		_ = world.DeleteLogFile("log")
		ctx.WriteString("Log파일을 삭제했습니다.\n")
		return StatusDefault, nil
	}

	if dmLogArgCount(resolved) == 1 && arg == "f" {
		content, err := world.ReadLogFile("log_fl")
		if err == nil {
			return renderLegacyViewFile(ctx, content, "DM 로그 읽기 상태를 시작할 수 없습니다")
		}
		return StatusDoPrompt, nil
	}

	content, err := world.ReadLogFile("log")
	if err == nil {
		return renderLegacyViewFile(ctx, content, "DM 로그 읽기 상태를 시작할 수 없습니다")
	}
	return StatusDoPrompt, nil
}

func dmLogArgCount(resolved ResolvedCommand) int {
	if resolved.Parsed.Num > 0 {
		return resolved.Parsed.Num - 1
	}
	return len(resolved.Args)
}

func dmLogArg(resolved ResolvedCommand) string {
	if resolved.Parsed.Num > 1 {
		if arg := strings.TrimSpace(resolved.Parsed.Str[1]); arg != "" {
			return arg
		}
	}
	return getArg(resolved, 0)
}
