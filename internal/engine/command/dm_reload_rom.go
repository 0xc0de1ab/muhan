package command

import "muhan/internal/world/model"

type DMReloadRomWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	ReloadRoom(model.RoomID) error
}

func NewDMReloadRomHandler(world DMReloadRomWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmReloadRom(ctx, resolved, world)
	}
}

func dmReloadRom(ctx *Context, resolved ResolvedCommand, world DMReloadRomWorld) (Status, error) {
	if ctx == nil || ctx.ActorID == "" {
		return StatusPrompt, nil
	}

	player, creature, ok := dmReloadRomActor(world, ctx.ActorID)
	if !ok {
		return StatusPrompt, nil
	}

	if dmReloadRomCreatureClass(creature) < legacyClassDM {
		return StatusPrompt, nil
	}

	roomID := creature.RoomID
	if roomID.IsZero() {
		roomID = player.RoomID
	}
	if roomID.IsZero() {
		ctx.WriteString("실패했습니다.\n")
		return StatusDefault, nil
	}

	if err := world.ReloadRoom(roomID); err != nil {
		ctx.WriteString("실패했습니다.\n")
	} else {
		ctx.WriteString("Ok.\n")
	}

	return StatusDefault, nil
}

func dmReloadRomActor(world DMReloadRomWorld, actorID string) (model.Player, model.Creature, bool) {
	playerID := model.PlayerID(actorID)
	if player, ok := world.Player(playerID); ok {
		if player.CreatureID.IsZero() {
			return player, model.Creature{}, false
		}
		creature, ok := world.Creature(player.CreatureID)
		return player, creature, ok
	}

	creatureID := model.CreatureID(actorID)
	creature, ok := world.Creature(creatureID)
	if !ok {
		return model.Player{}, model.Creature{}, false
	}
	if !creature.PlayerID.IsZero() {
		if player, ok := world.Player(creature.PlayerID); ok {
			return player, creature, true
		}
	}
	return model.Player{}, creature, true
}

func dmReloadRomCreatureClass(creature model.Creature) int {
	return creatureClass(creature)
}
