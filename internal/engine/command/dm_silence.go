package command

import (
	"fmt"
	"strings"

	"muhan/internal/world/model"
)

// DMSilenceWorld defines the interface required by the dm_silence command.
type DMSilenceWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	GetDailyBroadcastCount(model.CreatureID) (cur, max int)
	SetDailyBroadcastCount(model.CreatureID, int) error
}

// NewDMSilenceHandler creates a new command handler for dm_silence.
func NewDMSilenceHandler(world DMSilenceWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmSilence(ctx, resolved, world)
	}
}

func dmSilence(ctx *Context, resolved ResolvedCommand, world DMSilenceWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	// 1. Enforce DM (13+) permission validation.
	playerID := model.PlayerID(ctx.ActorID)
	var creatureID model.CreatureID
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
	if class < legacyClassDM {
		return StatusPrompt, nil
	}

	if dmSilenceMissingArgs(resolved) {
		ctx.WriteString("문법: <사용자> [c/m] *벙어리\n")
		return StatusDefault, nil
	}

	// 3. Search target player. If not found or if target has PDMINV (dmInvisible) enabled, print "그런 사용자는 없습니다.\n" and return StatusDefault.
	targetName := dmSilenceArg(resolved, 0)
	targetName = legacyLowercizeASCII(targetName, true)

	targetPlayer, targetCrt, ok := dmSilenceFindOnlinePlayer(ctx, world, targetName)
	if !ok {
		ctx.WriteString("그런 사용자는 없습니다.\n")
		return StatusDefault, nil
	}

	if creatureHasAnyFlag(targetCrt, "PDMINV", "dmInvisible") {
		ctx.WriteString("그런 사용자는 없습니다.\n")
		return StatusDefault, nil
	}

	if resolved.Parsed.Num < 3 {
		if err := world.SetDailyBroadcastCount(targetPlayer.CreatureID, 0); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(fmt.Sprintf("%s은 조용해 졌습니다.\n", targetCrt.DisplayName))
	} else {
		secondArg := dmSilenceArg(resolved, 1)

		if strings.HasPrefix(strings.ToLower(secondArg), "c") {
			cur, max := world.GetDailyBroadcastCount(targetPlayer.CreatureID)
			ctx.WriteString(fmt.Sprintf("%s has %d of %d broadcasts left.\n", targetCrt.DisplayName, cur, max))
		} else {
			val := int(resolved.Parsed.Val[2])
			if err := world.SetDailyBroadcastCount(targetPlayer.CreatureID, val); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(fmt.Sprintf("%s is given %d broadcasts.\n", targetCrt.DisplayName, val))
		}
	}

	return StatusDefault, nil
}

func dmSilenceFindOnlinePlayer(ctx *Context, world DMSilenceWorld, name string) (model.Player, model.Creature, bool) {
	player, creature, _, ok := legacyFindWhoActivePlayer(ctx, world, name)
	return player, creature, ok
}

func dmSilenceMissingArgs(resolved ResolvedCommand) bool {
	if resolved.Parsed.Num > 0 {
		return resolved.Parsed.Num < 2
	}
	return len(resolved.Args) < 1
}

func dmSilenceArg(resolved ResolvedCommand, index int) string {
	slot := index + 1
	if resolved.Parsed.Num > slot {
		if arg := strings.TrimSpace(resolved.Parsed.Str[slot]); arg != "" {
			return arg
		}
	}
	if index < 0 || index >= len(resolved.Args) {
		return ""
	}
	return strings.TrimSpace(resolved.Args[index])
}
