package command

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type DMPurgeWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
	DestroyCreature(model.CreatureID) error
	DestroyObject(model.ObjectInstanceID) error
}

func NewDMPurgeHandler(world DMPurgeWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmPurge(ctx, resolved, world)
	}
}

func dmPurge(ctx *Context, resolved ResolvedCommand, world DMPurgeWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	playerID := model.PlayerID(ctx.ActorID)
	player, ok := world.Player(playerID)
	var creatureID model.CreatureID
	if ok {
		creatureID = player.CreatureID
	} else {
		creatureID = model.CreatureID(ctx.ActorID)
	}

	creature, ok := world.Creature(creatureID)
	if !ok {
		return StatusPrompt, nil
	}

	class := creatureClass(creature)
	if class < model.ClassSubDM {
		return StatusPrompt, nil
	}

	roomID := creature.RoomID
	room, ok := world.Room(roomID)
	if !ok {
		return StatusDefault, fmt.Errorf("room not found: %s", roomID)
	}

	// 1. Clean up monsters in the room
	// We copy the creature IDs to safely iterate and destroy them
	creatureIDs := make([]model.CreatureID, len(room.CreatureIDs))
	copy(creatureIDs, room.CreatureIDs)

	for _, crtID := range creatureIDs {
		crt, ok := world.Creature(crtID)
		if !ok {
			continue
		}

		// Skip players
		if crt.Kind == model.CreatureKindPlayer || !crt.PlayerID.IsZero() {
			continue
		}

		// Check MDMFOL follower flag
		if creatureHasAnyFlag(crt, "MDMFOL") {
			if leaderID, ok := unfollowGroup(ctx, string(crtID)); ok {
				crtName := getPurgeCreatureName(crt)
				msg := fmt.Sprintf("%s%s 당신을 그만 따릅니다.\n", crtName, krtext.Particle(crtName, '1'))
				notifyPurgeLeader(ctx, leaderID, msg)
			}
		}

		_ = world.DestroyCreature(crtID)
	}

	// 2. Clean up floor objects in the room
	// We copy the object IDs to safely iterate and destroy them
	objectIDs := make([]model.ObjectInstanceID, len(room.Objects.ObjectIDs))
	copy(objectIDs, room.Objects.ObjectIDs)

	for _, objID := range objectIDs {
		if _, ok := world.Object(objID); !ok {
			continue
		}

		_ = world.DestroyObject(objID)
	}

	ctx.WriteString("청소되었습니다.\n")
	return StatusDefault, nil
}

func getPurgeCreatureName(creature model.Creature) string {
	name := creature.DisplayName
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "[")
	name = strings.TrimSuffix(name, "]")
	if name == "" {
		return string(creature.ID)
	}
	return name
}

func unfollowGroup(ctx *Context, followerID string) (string, bool) {
	if ctx == nil || ctx.Values == nil || followerID == "" {
		return "", false
	}
	groupsVal := ctx.Values["game.groupMemory"]
	if groupsVal == nil {
		groupsVal = ctx.Values["game.groups"]
	}
	if groupsVal == nil {
		return "", false
	}
	val := reflect.ValueOf(groupsVal)
	if !val.IsValid() {
		return "", false
	}
	unfollowMethod := val.MethodByName("Unfollow")
	if unfollowMethod.IsValid() {
		res := unfollowMethod.Call([]reflect.Value{reflect.ValueOf(followerID)})
		if len(res) == 2 && res[1].Kind() == reflect.Bool && res[1].Bool() {
			return res[0].String(), true
		}
	}
	return "", false
}

func notifyPurgeLeader(ctx *Context, leaderID string, text string) {
	sessions := getActiveSessions(ctx)
	for _, s := range sessions {
		if s.ActorID == leaderID {
			if s.ID == ctx.SessionID {
				ctx.WriteString(text)
				return
			}
			sendNotificationToSession(ctx, s.ID, text)
			return
		}
	}
}

func sendNotificationToSession(ctx *Context, sessionID string, text string) {
	if ctx == nil || ctx.Values == nil || sessionID == "" || text == "" {
		return
	}
	sendFn := ctx.Values["game.sendToSession"]
	if sendFn == nil {
		return
	}
	sendVal := reflect.ValueOf(sendFn)
	if !sendVal.IsValid() || sendVal.Type().NumIn() < 2 {
		return
	}

	// Second parameter is session.Command
	cmdType := sendVal.Type().In(1)
	cmdVal := reflect.New(cmdType).Elem()
	writeField := cmdVal.FieldByName("Write")
	if writeField.IsValid() && writeField.CanSet() {
		writeField.SetString(text)
	}

	// First parameter is session.ID (underlying string)
	sessIDType := sendVal.Type().In(0)
	sessIDVal := reflect.ValueOf(sessionID).Convert(sessIDType)

	// Call sendFn(sessionID, command)
	sendVal.Call([]reflect.Value{sessIDVal, cmdVal})
}
