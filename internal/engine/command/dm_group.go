package command

import (
	"fmt"
	"reflect"
	"strings"

	"muhan/internal/world/model"
)

type DMGroupWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	FindCreatureInRoom(roomID model.RoomID, name string) (model.Creature, bool)
}

func NewDMGroupHandler(world DMGroupWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmGroup(ctx, resolved, world)
	}
}

func dmGroup(ctx *Context, resolved ResolvedCommand, world DMGroupWorld) (Status, error) {
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
		return StatusDefault, nil
	}

	class := creatureClass(creature)
	if class < model.ClassSubDM {
		return StatusPrompt, nil
	}

	if resolved.Parsed.Num < 2 {
		ctx.WriteString("누구의 그룹을 봅니까?\n")
		return StatusPrompt, nil
	}

	targetName := resolved.Parsed.Str[1]
	var targetCreature model.Creature
	found := false

	// Find target creature in room's monster list
	if !creature.RoomID.IsZero() {
		if tc, ok := dmFindMonsterInRoomForActor(world, creature, creature.RoomID, targetName, dmGroupTargetOrdinal(resolved)); ok {
			targetCreature = tc
			found = true
		}
	}

	// If not found, find online player by case-insensitive name
	if !found {
		if tc, ok := dmGroupFindOnlinePlayer(ctx, world, legacyLowercizeASCII(targetName, true)); ok {
			targetCreature = tc
			found = true
		}
	}

	if !found {
		ctx.WriteString("그런 사람이 없습니다.\n")
		return StatusPrompt, nil
	}

	var targetID string
	if !targetCreature.PlayerID.IsZero() {
		targetID = string(targetCreature.PlayerID)
	} else {
		targetID = string(targetCreature.ID)
	}

	displayName := targetCreature.DisplayName
	if displayName == "" {
		displayName = string(targetCreature.ID)
	}

	leaderName := "없음"
	if groupsVal := ctx.Values["game.groupMemory"]; groupsVal != nil {
		val := reflect.ValueOf(groupsVal)
		if val.IsValid() {
			leaderOfMethod := val.MethodByName("LeaderOf")
			if leaderOfMethod.IsValid() {
				res := leaderOfMethod.Call([]reflect.Value{reflect.ValueOf(targetID)})
				if len(res) == 2 && res[1].Kind() == reflect.Bool && res[1].Bool() {
					leaderID := res[0].String()
					if leaderID != "" {
						leaderName = getDisplayName(world, leaderID)
					}
				}
			}
		}
	}

	ctx.WriteString(fmt.Sprintf("%s이 따르고 있는 사람: %s\n", displayName, leaderName))
	ctx.WriteString(fmt.Sprintf("%s의 그룹: ", displayName))

	var followers []string
	if groupsVal := ctx.Values["game.groupMemory"]; groupsVal != nil {
		val := reflect.ValueOf(groupsVal)
		if val.IsValid() {
			followersOfMethod := val.MethodByName("FollowersOf")
			if followersOfMethod.IsValid() {
				res := followersOfMethod.Call([]reflect.Value{reflect.ValueOf(targetID)})
				if len(res) == 1 && res[0].Kind() == reflect.Slice {
					sliceLen := res[0].Len()
					for i := 0; i < sliceLen; i++ {
						followerID := res[0].Index(i).String()
						if followerID != "" {
							followers = append(followers, followerID)
						}
					}
				}
			}
		}
	}

	if len(followers) == 0 {
		ctx.WriteString("없음.\n")
	} else {
		var names []string
		for _, fid := range followers {
			names = append(names, getDisplayName(world, fid))
		}
		ctx.WriteString(strings.Join(names, ", ") + ".\n")
	}

	return StatusDefault, nil
}

func dmGroupFindOnlinePlayer(ctx *Context, world DMGroupWorld, name string) (model.Creature, bool) {
	_, creature, _, ok := legacyFindWhoActivePlayer(ctx, world, name)
	return creature, ok
}

func dmGroupTargetOrdinal(resolved ResolvedCommand) int64 {
	if resolved.Parsed.Num > 1 && resolved.Parsed.Val[1] > 0 {
		return resolved.Parsed.Val[1]
	}
	return getOrdinal(resolved, 0)
}

func getDisplayName(world DMGroupWorld, actorID string) string {
	if player, ok := world.Player(model.PlayerID(actorID)); ok {
		if player.DisplayName != "" {
			return player.DisplayName
		}
		if !player.CreatureID.IsZero() {
			if crt, ok := world.Creature(player.CreatureID); ok {
				if crt.DisplayName != "" {
					return crt.DisplayName
				}
			}
		}
	}
	if crt, ok := world.Creature(model.CreatureID(actorID)); ok {
		if crt.DisplayName != "" {
			return crt.DisplayName
		}
	}
	name := actorID
	if strings.HasPrefix(name, "player:") {
		name = strings.TrimPrefix(name, "player:")
	}
	if strings.HasPrefix(name, "creature:") {
		name = strings.TrimPrefix(name, "creature:")
	}
	return name
}
