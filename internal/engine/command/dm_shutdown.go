package command

import (
	"log"
	"strings"

	"muhan/internal/world/model"
)

type DMShutdownWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	SetShutdown(seconds int, now bool) error

	// B: Optional flush before shutdown for persistence
	FlushActivePlayersAndBanks() error
}

func NewDMShutdownHandler(world DMShutdownWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmShutdown(ctx, resolved, world)
	}
}

func dmShutdown(ctx *Context, resolved ResolvedCommand, world DMShutdownWorld) (Status, error) {
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

	seconds := int(resolved.Parsed.Val[0])*60 + 1
	now := dmShutdownNow(resolved)
	if now {
		seconds = 1
	}

	ctx.WriteString("Ok.\n")

	// B: Flush persistence before initiating shutdown (best effort) - uses single reliable FlushActive path
	if flusher, ok := world.(interface{ FlushActivePlayersAndBanks() error }); ok {
		if err := flusher.FlushActivePlayersAndBanks(); err != nil {
			log.Printf("[PERSIST] ERROR DM *shutdown FlushActivePlayersAndBanks: %v", err)
		} else {
			log.Printf("[PERSIST] INFO DM *shutdown pre-flush complete (players+banks+rooms)")
		}
	}

	_ = world.SetShutdown(seconds, now)

	return StatusDefault, nil
}

func dmShutdownNow(resolved ResolvedCommand) bool {
	if resolved.Parsed.Num > 0 {
		return resolved.Parsed.Num >= 2 && resolved.Parsed.Str[1] == "now"
	}
	return len(resolved.Args) > 0 && strings.TrimSpace(resolved.Args[0]) == "now"
}
