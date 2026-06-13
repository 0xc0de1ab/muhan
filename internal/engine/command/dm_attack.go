package command

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type DMAttackWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
	FindCreatureInRoom(model.RoomID, string) (model.Creature, bool)
	AddEnemy(attacker model.CreatureID, defender model.CreatureID) (bool, error)
}

func NewDMAttackHandler(world DMAttackWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmAttack(ctx, resolved, world)
	}
}

func dmAttack(ctx *Context, resolved ResolvedCommand, world DMAttackWorld) (Status, error) {
	if ctx == nil || ctx.ActorID == "" || world == nil {
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

	attackerName := dmAttackArg(resolved, 0)
	defenderName := dmAttackArg(resolved, 1)
	if dmAttackMissingArgs(resolved) || attackerName == "" || defenderName == "" {
		ctx.WriteString("사용법: <괴물> <사용자> *공격\n")
		return StatusDefault, nil
	}

	roomID := creature.RoomID
	if roomID.IsZero() {
		return StatusDefault, nil
	}
	room, ok := world.Room(roomID)
	if !ok {
		return StatusDefault, nil
	}

	attacker, ok := dmFindMonsterInRoomForActor(world, creature, room.ID, attackerName, dmAttackOrdinal(resolved, 0))
	if !ok {
		ctx.WriteString("그런 괴물이 없습니다.\n")
		return StatusDefault, nil
	}

	if creatureHasAnyFlag(attacker, "MPERMT", "mpermt") {
		ctx.WriteString("고정된 괴물입니다.\n")
		return StatusDefault, nil
	}

	var defender model.Creature
	var defenderIsPlayer bool
	var defenderPlayer model.Player
	var defenderSessionID string

	if defCrt, ok := dmFindMonsterInRoomForActor(world, creature, room.ID, defenderName, dmAttackOrdinal(resolved, 1)); ok {
		defender = defCrt
	} else if defPly, defCrt, sessionID, ok := dmAttackFindOnlinePlayer(ctx, world, legacyLowercizeASCII(defenderName, true)); ok {
		defender = defCrt
		defenderIsPlayer = true
		defenderPlayer = defPly
		defenderSessionID = sessionID
	}

	if defender.ID.IsZero() {
		ctx.WriteString("그런 사람이 없습니다.\n")
		return StatusDefault, nil
	}

	if creatureHasAnyFlag(defender, "MPERMT", "mpermt") {
		ctx.WriteString("고정된 괴물입니다.\n")
		return StatusDefault, nil
	}

	ctx.WriteString(fmt.Sprintf("%s가 %s를 공격합니다.\n", defender.DisplayName, attacker.DisplayName))

	if _, err := world.AddEnemy(attacker.ID, defender.ID); err != nil {
		return StatusDefault, err
	}

	broadcastRoomID := defender.RoomID
	if broadcastRoomID.IsZero() {
		broadcastRoomID = room.ID
	}
	_ = dmAttackRoomBroadcast(ctx, broadcastRoomID, defenderSessionID, fmt.Sprintf("%s이 %s을 공격합니다.", attacker.DisplayName, defender.DisplayName))

	if defenderIsPlayer {
		dmAttackSendToPlayer(ctx, defenderPlayer.ID, fmt.Sprintf("%s이 당신을 공격합니다!\n", attacker.DisplayName))
	}

	return StatusDefault, nil
}

func dmAttackMissingArgs(resolved ResolvedCommand) bool {
	if resolved.Parsed.Num > 0 {
		return resolved.Parsed.Num < 3
	}
	return len(resolved.Args) < 2
}

func dmAttackArg(resolved ResolvedCommand, index int) string {
	if resolved.Parsed.Num > index+1 {
		if arg := strings.TrimSpace(resolved.Parsed.Str[index+1]); arg != "" {
			return arg
		}
	}
	return getArg(resolved, index)
}

func dmAttackOrdinal(resolved ResolvedCommand, index int) int64 {
	slot := index + 1
	if resolved.Parsed.Num > slot && resolved.Parsed.Val[slot] > 0 {
		return resolved.Parsed.Val[slot]
	}
	return getOrdinal(resolved, index)
}

func dmAttackFindOnlinePlayer(ctx *Context, world DMAttackWorld, name string) (model.Player, model.Creature, string, bool) {
	player, creature, session, ok := legacyFindWhoActivePlayer(ctx, world, name)
	return player, creature, session.ID, ok
}

func dmAttackRoomBroadcast(ctx *Context, roomID model.RoomID, excludeSessionID string, text string) error {
	if ctx == nil || ctx.Values == nil || roomID.IsZero() || text == "" {
		return nil
	}
	fn, ok := ctx.Values[ContextRoomBroadcastKey].(RoomBroadcastFunc)
	if !ok || fn == nil {
		return nil
	}
	return fn(roomID, excludeSessionID, text)
}

func dmAttackSendToPlayer(ctx *Context, playerID model.PlayerID, text string) {
	if ctx == nil || ctx.Values == nil || playerID.IsZero() || text == "" {
		return
	}
	activeVal := reflect.ValueOf(ctx.Values["game.activeSessions"])
	if !activeVal.IsValid() || activeVal.Kind() != reflect.Func {
		return
	}
	out := activeVal.Call(nil)[0]
	if out.Kind() != reflect.Slice {
		return
	}
	for i := 0; i < out.Len(); i++ {
		item := out.Index(i)
		if item.Kind() == reflect.Interface {
			item = item.Elem()
		}
		if item.Kind() == reflect.Pointer {
			if item.IsNil() {
				continue
			}
			item = item.Elem()
		}
		if item.Kind() != reflect.Struct {
			continue
		}
		idField := item.FieldByName("ID")
		actorField := item.FieldByName("ActorID")
		if !idField.IsValid() || !actorField.IsValid() || actorField.Kind() != reflect.String {
			continue
		}
		if actorField.String() == string(playerID) {
			_ = dmAttackSendToSession(ctx, idField.Interface(), text)
			return
		}
	}
}

func dmAttackSendToSession(ctx *Context, sessionID any, text string) error {
	if ctx == nil || ctx.Values == nil {
		return nil
	}
	sendValue := reflect.ValueOf(ctx.Values["game.sendToSession"])
	if !sendValue.IsValid() || sendValue.Kind() != reflect.Func ||
		sendValue.Type().NumIn() != 2 || sendValue.Type().NumOut() != 1 {
		return nil
	}
	if sendValue.Type().Out(0) != reflect.TypeOf((*error)(nil)).Elem() ||
		sendValue.Type().In(1).Kind() != reflect.Struct {
		return nil
	}

	idValue := reflect.ValueOf(sessionID)
	targetType := sendValue.Type().In(0)
	if !idValue.Type().AssignableTo(targetType) {
		if !idValue.Type().ConvertibleTo(targetType) {
			return fmt.Errorf("send session id type %s is not compatible with %s", idValue.Type(), targetType)
		}
		idValue = idValue.Convert(targetType)
	}

	// Construct message struct
	msgType := sendValue.Type().In(1)
	msgVal := reflect.New(msgType).Elem()
	writeField := msgVal.FieldByName("Write")
	if writeField.IsValid() && writeField.CanSet() && writeField.Kind() == reflect.String {
		writeField.SetString(text)
	}

	sendValue.Call([]reflect.Value{idValue, msgVal})
	return nil
}
