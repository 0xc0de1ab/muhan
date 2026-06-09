package command

import (
	"fmt"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

type PledgeWorld interface {
	InventoryWorld
	Room(model.RoomID) (model.Room, bool)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
}

func NewPledgeHandler(world PledgeWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, actor, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}

		room, ok := world.Room(actor.RoomID)
		if !ok {
			return StatusDefault, fmt.Errorf("pledge: current room not found")
		}

		// Check if already pledged
		if hasAnyNormalizedFlag(player.Metadata.Tags, "pledged", "PPLDGK", "ppldgk") ||
			hasAnyNormalizedFlag(actor.Metadata.Tags, "pledged", "PPLDGK", "ppldgk") {
			ctx.WriteString("당신은 이미 가입되어 있습니다.\n")
			return StatusDefault, nil
		}

		// Check room flag RPLDGK (pledge)
		roomPledgeable := hasAnyNormalizedFlag(room.Metadata.Tags, "pledge", "rpledgk", "RPLDGK")
		if !roomPledgeable {
			for key, val := range room.Properties {
				if normalizeFlagName(key) == "pledge" && propertyFlagEnabled(val) {
					roomPledgeable = true
					break
				}
			}
		}

		// Check monster flag MPLDGK (pledgeKingdom) in the room
		monsterPledgeable := false
		for _, cid := range room.CreatureIDs {
			monster, ok := world.Creature(cid)
			if !ok || monster.Kind != model.CreatureKindMonster {
				continue
			}
			if hasAnyNormalizedFlag(monster.Metadata.Tags, "pledgeKingdom", "mpledgk", "MPLDGK") {
				monsterPledgeable = true
				break
			}
		}

		if !roomPledgeable && !monsterPledgeable {
			ctx.WriteString("이곳에서는 가입할 수 없습니다.\n")
			return StatusDefault, nil
		}

		// Pledge player
		if _, err := world.UpdateCreatureTags(actor.ID, []string{"pledged"}, nil); err != nil {
			return StatusDefault, err
		}
		if _, err := world.UpdatePlayerTags(playerID, []string{"pledged"}, nil); err != nil {
			return StatusDefault, err
		}

		ctx.WriteString("당신은 가입하였습니다.\n")
		actorName := attackCreatureName(actor)
		return StatusDefault, roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 가입하였습니다.\n")
	}
}

func NewRescindHandler(world PledgeWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, actor, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}

		room, ok := world.Room(actor.RoomID)
		if !ok {
			return StatusDefault, fmt.Errorf("rescind: current room not found")
		}

		// Check if pledged
		if !hasAnyNormalizedFlag(player.Metadata.Tags, "pledged", "PPLDGK", "ppldgk") &&
			!hasAnyNormalizedFlag(actor.Metadata.Tags, "pledged", "PPLDGK", "ppldgk") {
			ctx.WriteString("당신은 가입되어 있지 않습니다.\n")
			return StatusDefault, nil
		}

		// Check room flag RRSCND (rescind)
		roomRescindable := hasAnyNormalizedFlag(room.Metadata.Tags, "rescind", "rrscnd", "RRSCND")
		if !roomRescindable {
			for key, val := range room.Properties {
				if normalizeFlagName(key) == "rescind" && propertyFlagEnabled(val) {
					roomRescindable = true
					break
				}
			}
		}

		// Check monster flag MRSCND (rescindKingdom) in the room
		monsterRescindable := false
		for _, cid := range room.CreatureIDs {
			monster, ok := world.Creature(cid)
			if !ok || monster.Kind != model.CreatureKindMonster {
				continue
			}
			if hasAnyNormalizedFlag(monster.Metadata.Tags, "rescindKingdom", "mrscnd", "MRSCND") {
				monsterRescindable = true
				break
			}
		}

		if !roomRescindable && !monsterRescindable {
			ctx.WriteString("이곳에서는 탈퇴할 수 없습니다.\n")
			return StatusDefault, nil
		}

		// Rescind player
		if _, err := world.UpdateCreatureTags(actor.ID, nil, []string{"pledged", "PPLDGK", "ppldgk"}); err != nil {
			return StatusDefault, err
		}
		if _, err := world.UpdatePlayerTags(playerID, nil, []string{"pledged", "PPLDGK", "ppldgk"}); err != nil {
			return StatusDefault, err
		}

		ctx.WriteString("당신은 탈퇴하였습니다.\n")
		actorName := attackCreatureName(actor)
		return StatusDefault, roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 탈퇴하였습니다.\n")
	}
}
