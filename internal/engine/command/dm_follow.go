package command

import (
	"fmt"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type DMFollowWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	FindCreatureInRoom(model.RoomID, string) (model.Creature, bool)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	SetCreatureStat(model.CreatureID, string, int) error
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
}

const (
	dmFollowLeaderProperty         = "dmFollowLeader"
	dmFollowLeaderCreatureProperty = "dmFollowLeaderCreature"
)

func NewDMFollowHandler(world DMFollowWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmFollow(ctx, resolved, world)
	}
}

func dmFollow(ctx *Context, resolved ResolvedCommand, world DMFollowWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	actorID := strings.TrimSpace(ctx.ActorID)
	var creatureID model.CreatureID
	player, hasPlayer := world.Player(model.PlayerID(actorID))
	if hasPlayer {
		creatureID = player.CreatureID
	} else {
		creatureID = model.CreatureID(actorID)
	}

	creature, ok := world.Creature(creatureID)
	if !ok {
		return StatusDefault, nil
	}

	class := creatureClass(creature)
	if class < model.ClassDM {
		return StatusPrompt, nil
	}

	targetName := dmFollowTargetArg(resolved)
	if resolved.Parsed.Num < 2 || targetName == "" {
		ctx.WriteString("사용법: <괴물> *따르기\n")
		return StatusDefault, nil
	}

	roomID := creature.RoomID
	if roomID.IsZero() && hasPlayer {
		roomID = player.RoomID
	}
	if roomID.IsZero() {
		return StatusDefault, nil
	}
	targetCreature, ok := dmFindMonsterInRoomForActor(world, creature, roomID, targetName, dmFollowTargetOrdinal(resolved))
	if !ok {
		ctx.WriteString("그런 괴물이 없습니다.\n")
		return StatusDefault, nil
	}

	if creatureHasAnyFlag(targetCreature, "MPERMT") {
		ctx.WriteString("고정된 괴물입니다.\n")
		return StatusDefault, nil
	}

	name := cleanDisplayText(targetCreature.DisplayName)
	if name == "" {
		name = targetCreature.DisplayName
	}

	if creatureHasAnyFlag(targetCreature, "MDMFOL") {
		if err := world.SetCreatureStat(targetCreature.ID, "MDMFOL", 0); err != nil {
			return StatusDefault, err
		}
		if _, err := world.UpdateCreatureTags(targetCreature.ID, nil, []string{"MDMFOL"}); err != nil {
			return StatusDefault, err
		}
		if _, err := world.SetCreatureProperty(targetCreature.ID, dmFollowLeaderProperty, ""); err != nil {
			return StatusDefault, err
		}
		if _, err := world.SetCreatureProperty(targetCreature.ID, dmFollowLeaderCreatureProperty, ""); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(fmt.Sprintf("%s이 당신을 그만 따릅니다.\n", name))
	} else {
		if err := world.SetCreatureStat(targetCreature.ID, "MDMFOL", 1); err != nil {
			return StatusDefault, err
		}
		if _, err := world.UpdateCreatureTags(targetCreature.ID, []string{"MDMFOL"}, nil); err != nil {
			return StatusDefault, err
		}
		if _, err := world.SetCreatureProperty(targetCreature.ID, dmFollowLeaderProperty, actorID); err != nil {
			return StatusDefault, err
		}
		if _, err := world.SetCreatureProperty(targetCreature.ID, dmFollowLeaderCreatureProperty, string(creature.ID)); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(fmt.Sprintf("%s이 당신을 따릅니다.\n", name))
	}

	return StatusDefault, nil
}

func dmFollowTargetArg(resolved ResolvedCommand) string {
	if resolved.Parsed.Num > 1 {
		if target := strings.TrimSpace(resolved.Parsed.Str[1]); target != "" {
			return target
		}
	}
	return getArg(resolved, 0)
}

func dmFollowTargetOrdinal(resolved ResolvedCommand) int64 {
	if resolved.Parsed.Num > 1 && resolved.Parsed.Val[1] > 0 {
		return resolved.Parsed.Val[1]
	}
	return getOrdinal(resolved, 0)
}
