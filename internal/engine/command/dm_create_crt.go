package command

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"muhan/internal/world/model"
)

type DMCreateCrtWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	CreaturePrototype(id model.CreatureID) (model.Creature, bool)
	SpawnCreature(protoID model.CreatureID, roomID model.RoomID, carryItems bool) (model.CreatureID, error)
}

func NewDMCreateCrtHandler(world DMCreateCrtWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmCreateCrt(ctx, resolved, world)
	}
}

func dmCreateCrt(ctx *Context, resolved ResolvedCommand, world DMCreateCrtWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	player, creature, ok := dmCreateCrtActor(world, strings.TrimSpace(ctx.ActorID))
	if !ok {
		return StatusPrompt, nil
	}

	class := creatureClass(creature)
	if class < model.ClassSubDM {
		return StatusPrompt, nil
	}

	roomID := creature.RoomID
	if roomID.IsZero() {
		roomID = player.RoomID
	}
	room, ok := world.Room(roomID)
	if !ok {
		return StatusDefault, fmt.Errorf("room not found: %s", roomID)
	}

	num := int(resolved.Parsed.Val[0])
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	if num < 2 {
		num = getRandomMonsterFromRoom(room, rng)
		if num == 0 {
			return StatusDefault, nil
		}
	}

	total := 1
	if resolved.Parsed.Num == 2 {
		arg1 := resolved.Parsed.Str[1]
		if len(arg1) > 0 {
			if arg1[0] == 'n' {
				total = int(resolved.Parsed.Val[1])
			} else if arg1[0] == 'g' {
				numPlayers := countPlayersInRoom(world, room)
				if numPlayers > 0 {
					total = rng.Intn(numPlayers) + 1
				} else {
					total = 1
				}
				if resolved.Parsed.Val[1] == 1 {
					num = getRandomMonsterFromRoom(room, rng)
					if num == 0 {
						return StatusDefault, nil
					}
				}
			}
		}
	}

	protoID := model.CreatureID(fmt.Sprintf("creature:m%02d:%d", num/100, num%100))
	_, ok = world.CreaturePrototype(protoID)
	if !ok {
		ctx.WriteString(fmt.Sprintf("에러 (%d)\n", resolved.Parsed.Val[0]))
		return StatusDefault, nil
	}

	for l := 0; l < total; l++ {
		_, err := world.SpawnCreature(protoID, room.ID, true)
		if err != nil {
			ctx.WriteString(fmt.Sprintf("에러 (%d)\n", resolved.Parsed.Val[0]))
			return StatusDefault, nil
		}
	}

	return StatusDefault, nil
}

func dmCreateCrtActor(world DMCreateCrtWorld, actorID string) (model.Player, model.Creature, bool) {
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

func getRandomMonsterFromRoom(room model.Room, rng *rand.Rand) int {
	index := rng.Intn(10)

	// Check properties: random[0] to random[9]
	if valStr, ok := room.Properties[fmt.Sprintf("random[%d]", index)]; ok {
		if val, err := strconv.Atoi(valStr); err == nil {
			return val
		}
	}
	// Check properties: random0 to random9
	if valStr, ok := room.Properties[fmt.Sprintf("random%d", index)]; ok {
		if val, err := strconv.Atoi(valStr); err == nil {
			return val
		}
	}
	// Check comma-separated string under "random"
	if valStr, ok := room.Properties["random"]; ok {
		parts := strings.Split(valStr, ",")
		if index < len(parts) {
			if val, err := strconv.Atoi(strings.TrimSpace(parts[index])); err == nil {
				return val
			}
		}
	}
	return 0
}

func countPlayersInRoom(world DMCreateCrtWorld, room model.Room) int {
	if len(room.PlayerIDs) > 0 {
		return len(room.PlayerIDs)
	}
	count := 0
	for _, crtID := range room.CreatureIDs {
		if crt, ok := world.Creature(crtID); ok {
			if crt.Kind == model.CreatureKindPlayer || !crt.PlayerID.IsZero() {
				count++
			}
		}
	}
	return count
}
