package command

import (
	"fmt"
	"reflect"
	"strings"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

// Note:
// legacyClassZoneMaker is defined in dm_placeholders.go
// model.ClassSubDM is defined in settings.go
// model.ClassDM is defined in peek.go
// getActiveSessions is defined in magic_effect_agent4.go

type DMTeleportWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	MovePlayerToRoom(model.PlayerID, model.RoomID) error
}

const legacyTeleportRoomMax = 9000

func NewDMTeleportHandler(world DMTeleportWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmTeleport(ctx, resolved, world)
	}
}

func dmTeleport(ctx *Context, resolved ResolvedCommand, world DMTeleportWorld) (Status, error) {
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

	// 1. Validate class permissions (ZONEMAKER or SUB_DM+)
	class := creatureClass(creature)
	if class != legacyClassZoneMaker && class < model.ClassSubDM {
		return StatusPrompt, nil
	}

	// 2. Validate group state: if player has followers or following, print "먼저 그룹에서 나오세요." and return.
	if checkGroupState(ctx, ctx.ActorID) {
		ctx.WriteString("먼저 그룹에서 나오세요.\n")
		return StatusDefault, nil
	}

	// 3. Handle arguments
	argCount := dmTeleportArgCount(resolved)
	isRoomTeleport := argCount == 0
	roomNum := int(resolved.Parsed.Val[0])

	if isRoomTeleport {
		// Case A: Room number teleport
		if roomNum >= legacyTeleportRoomMax {
			return StatusDefault, nil
		}

		targetRoomID := model.RoomID(fmt.Sprintf("room:%d", roomNum))
		targetRoom, ok := world.Room(targetRoomID)
		if !ok {
			ctx.WriteString(fmt.Sprintf("에러 (%d)\n", roomNum))
			return StatusDefault, nil
		}

		// Check if targetRoom has RSHOPP flag and caster is ZONEMAKER or SUB_DM
		if class == legacyClassZoneMaker || class == model.ClassSubDM {
			prevRoomID := model.RoomID(fmt.Sprintf("room:%d", roomNum-1))
			if prevRoom, ok := world.Room(prevRoomID); ok {
				if roomHasAnyFlag(prevRoom, "shoppe", "shop", "RSHOPP") {
					ctx.WriteString("순간이동이 금지된 구역입니다.\n")
					return StatusDefault, nil
				}
			}
		}

		// Move followers
		casterRoom, ok := world.Room(player.RoomID)
		if ok {
			teleportFollowers(ctx, world, casterRoom, targetRoom.ID, false)
		}

		// Move caster
		if !player.ID.IsZero() {
			_ = world.MovePlayerToRoom(player.ID, targetRoom.ID)
		}
		return StatusDefault, nil
	}

	if argCount < 2 {
		// Case B: 1 string argument (target player name)
		targetName := dmTeleportArg(resolved, 0)
		targetPlayer, targetCreature, found := findWho(ctx, world, targetName)

		// Validation: If not found, target is caster, or target is invisible SUB_DM+ and caster is class < SUB_DM
		isTargetSubDMOrHigher := creatureClass(targetCreature) >= model.ClassSubDM
		isCasterBelowSubDM := class < model.ClassSubDM
		isTargetInvisible := isPlayerDMInvisible(targetCreature)

		if !found || targetPlayer.ID == player.ID || (isTargetSubDMOrHigher && isCasterBelowSubDM && isTargetInvisible) {
			ctx.WriteString(targetName + krtext.Particle(targetName, '0') + " 접속중이 아닙니다.\n")
			return StatusDefault, nil
		}

		// Move followers to target's room
		casterRoom, ok := world.Room(player.RoomID)
		if ok {
			teleportFollowers(ctx, world, casterRoom, targetPlayer.RoomID, true)
		}

		// Move caster to target's room
		if !player.ID.IsZero() {
			_ = world.MovePlayerToRoom(player.ID, targetPlayer.RoomID)
		}
		return StatusDefault, nil
	}

	// Case C: 2 or more string arguments (target player name and destination room/player name)
	targetName := dmTeleportArg(resolved, 0)
	destName := dmTeleportArg(resolved, 1)

	targetPlayer, targetCreature, found := findWho(ctx, world, targetName)

	// Validation: if target not found, target is caster, or target is DM and is invisible and caster is not DM
	isTargetDM := creatureClass(targetCreature) == model.ClassDM
	isCasterNotDM := class < model.ClassDM
	isTargetInvisible := isPlayerDMInvisible(targetCreature)

	if !found || targetPlayer.ID == player.ID || (isTargetDM && isCasterNotDM && isTargetInvisible) {
		ctx.WriteString(targetName + krtext.Particle(targetName, '0') + " 접속중이 아닙니다.\n")
		return StatusDefault, nil
	}

	var destRoomID model.RoomID
	if strings.HasPrefix(destName, ".") {
		destRoomID = player.RoomID
	} else {
		destPlayer, _, found := findWho(ctx, world, destName)
		if !found {
			ctx.WriteString(targetName + krtext.Particle(targetName, '0') + " 접속중이 아닙니다.\n")
			return StatusDefault, nil
		}
		destRoomID = destPlayer.RoomID
	}

	if !destRoomID.IsZero() {
		_ = world.MovePlayerToRoom(targetPlayer.ID, destRoomID)
	}

	return StatusDefault, nil
}

func dmTeleportArgCount(resolved ResolvedCommand) int {
	if resolved.Parsed.Num > 0 {
		return resolved.Parsed.Num - 1
	}
	return len(resolved.Args)
}

func dmTeleportArg(resolved ResolvedCommand, index int) string {
	if resolved.Parsed.Num > index+1 {
		return strings.TrimSpace(resolved.Parsed.Str[index+1])
	}
	return getArg(resolved, index)
}

// Reflection-based helper to avoid circular dependency on game package
func checkGroupState(ctx *Context, actorID string) bool {
	if ctx == nil || ctx.Values == nil || actorID == "" {
		return false
	}

	groupsVal := ctx.Values["game.groupMemory"]
	if groupsVal == nil {
		groupsVal = ctx.Values["game.groups"]
	}
	if groupsVal == nil {
		return false
	}

	val := reflect.ValueOf(groupsVal)
	if !val.IsValid() {
		return false
	}

	// Check if following someone: LeaderOf(actorID) -> (string, bool)
	leaderOfMethod := val.MethodByName("LeaderOf")
	if leaderOfMethod.IsValid() {
		res := leaderOfMethod.Call([]reflect.Value{reflect.ValueOf(actorID)})
		if len(res) == 2 && res[1].Kind() == reflect.Bool && res[1].Bool() {
			return true
		}
	}

	// Check if has followers: FollowersOf(actorID) -> []string
	followersOfMethod := val.MethodByName("FollowersOf")
	if followersOfMethod.IsValid() {
		res := followersOfMethod.Call([]reflect.Value{reflect.ValueOf(actorID)})
		if len(res) == 1 && res[0].Kind() == reflect.Slice && res[0].Len() > 0 {
			return true
		}
	}

	return false
}

func findWho(ctx *Context, world DMTeleportWorld, name string) (model.Player, model.Creature, bool) {
	player, creature, _, ok := legacyFindWhoActivePlayer(ctx, world, name)
	return player, creature, ok
}

func teleportFollowers(ctx *Context, world DMTeleportWorld, casterRoom model.Room, destRoomID model.RoomID, isCaseB bool) {
	for _, crtID := range casterRoom.CreatureIDs {
		crt, ok := world.Creature(crtID)
		if !ok {
			continue
		}
		if creatureHasAnyFlag(crt, "MDMFOL") && dmTeleportFollowerBelongsToCaster(ctx, world, crt) {
			crtName := getCreatureName(crt)
			var msg string
			if isCaseB {
				msg = crtName + krtext.Particle(crtName, '1') + " 주위를 두리번 거리며 있습니다.\n"
			} else {
				msg = crtName + krtext.Particle(crtName, '1') + " 주위를 두리번 거립니다.\n"
			}
			_ = roomBroadcast(ctx, casterRoom.ID, msg)

			if !crt.PlayerID.IsZero() {
				_ = world.MovePlayerToRoom(crt.PlayerID, destRoomID)
			} else {
				if mover, ok := world.(interface {
					MoveCreatureToRoom(model.CreatureID, model.RoomID) error
				}); ok {
					_ = mover.MoveCreatureToRoom(crt.ID, destRoomID)
				}
			}
		}
	}
}

func dmTeleportFollowerBelongsToCaster(ctx *Context, world DMTeleportWorld, follower model.Creature) bool {
	leaderID := strings.TrimSpace(follower.Properties[dmFollowLeaderProperty])
	leaderCreatureID := strings.TrimSpace(follower.Properties[dmFollowLeaderCreatureProperty])
	if leaderID == "" && leaderCreatureID == "" {
		return true
	}
	if ctx == nil {
		return false
	}
	actorID := strings.TrimSpace(ctx.ActorID)
	if actorID == "" {
		return false
	}
	if leaderID == actorID || leaderCreatureID == actorID {
		return true
	}
	if player, ok := world.Player(model.PlayerID(actorID)); ok && !player.CreatureID.IsZero() {
		return leaderCreatureID == string(player.CreatureID)
	}
	return false
}

func getCreatureName(creature model.Creature) string {
	name := creature.DisplayName
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "[")
	name = strings.TrimSuffix(name, "]")
	if name == "" {
		return string(creature.ID)
	}
	return name
}

func isPlayerDMInvisible(creature model.Creature) bool {
	return creatureHasAnyFlag(creature, "PDMINV", "dmInvisible")
}
