package command

import (
	"fmt"
	"reflect"
	"strings"

	"muhan/internal/world/model"
)

type DMSendWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
}

func NewDMSendHandler(world DMSendWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmSend(ctx, resolved, world)
	}
}

func dmSend(ctx *Context, resolved ResolvedCommand, world DMSendWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	actorID := strings.TrimSpace(ctx.ActorID)
	creature, ok := resolveActorCreature(world, actorID)
	if !ok {
		return StatusDefault, nil
	}

	class := creatureClass(creature)
	if class < model.ClassSubDM {
		return StatusPrompt, nil
	}

	msg := extractCommandMessage(resolved)
	if msg == "" {
		ctx.WriteString("무엇을 공지하려구요?")
		return StatusDefault, nil
	}

	ctx.WriteString("Ok.\n")

	casterName := cleanDisplayText(creature.DisplayName)
	broadcastMsg := fmt.Sprintf("\n>>> 공지(%s): %s", casterName, msg)

	active := getActiveSessions(ctx)
	for _, s := range active {
		if s.ActorID == "" {
			continue
		}
		targetCrt, ok := resolveActorCreature(world, s.ActorID)
		if !ok {
			continue
		}
		if creatureClass(targetCrt) >= model.ClassCaretaker {
			if s.ID == ctx.SessionID {
				ctx.WriteString(broadcastMsg)
			} else {
				_ = dmSendSendToSession(ctx, s.ID, broadcastMsg)
			}
		}
	}

	return StatusDefault, nil
}

func resolveActorCreature(world DMSendWorld, actorID string) (model.Creature, bool) {
	playerID := model.PlayerID(actorID)
	if player, ok := world.Player(playerID); ok {
		if !player.CreatureID.IsZero() {
			if creature, ok := world.Creature(player.CreatureID); ok {
				return creature, ok
			}
		}
	}
	creatureID := model.CreatureID(actorID)
	creature, ok := world.Creature(creatureID)
	return creature, ok
}

func extractCommandMessage(resolved ResolvedCommand) string {
	input := strings.TrimSpace(resolved.Input)
	command := strings.TrimSpace(resolved.Command())
	if command == "" {
		command = strings.TrimSpace(resolved.Spec.Name)
	}
	if input == "" || command == "" {
		return ""
	}

	fields := strings.Fields(input)
	if len(fields) <= 1 {
		return ""
	}
	if strings.EqualFold(fields[0], command) {
		after, ok := textAfterFirstToken(input)
		if !ok {
			return ""
		}
		return strings.TrimSpace(after)
	}
	if strings.EqualFold(fields[len(fields)-1], command) {
		before, ok := textBeforeLastToken(input)
		if !ok {
			return ""
		}
		if strings.TrimSpace(before) == "" {
			return ""
		}
		return before
	}
	return joinArgs(resolved.Args)
}

func dmSendSendToSession(ctx *Context, sessionID string, text string) error {
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
	if !idValue.Type().AssignableTo(sendValue.Type().In(0)) {
		if !idValue.Type().ConvertibleTo(sendValue.Type().In(0)) {
			return fmt.Errorf("dm_send: send session id type %s is not compatible with %s", idValue.Type(), sendValue.Type().In(0))
		}
		idValue = idValue.Convert(sendValue.Type().In(0))
	}
	commandType := sendValue.Type().In(1)
	if commandType.Kind() != reflect.Struct {
		return fmt.Errorf("dm_send: send command type %s is not struct", commandType)
	}
	commandValue := reflect.New(commandType).Elem()
	writeField := commandValue.FieldByName("Write")
	if !writeField.IsValid() || !writeField.CanSet() || writeField.Kind() != reflect.String {
		return fmt.Errorf("dm_send: send command type %s does not expose settable Write string field", commandType)
	}
	writeField.SetString(text)

	results := sendValue.Call([]reflect.Value{idValue, commandValue})
	if errValue := results[0]; !errValue.IsNil() {
		return errValue.Interface().(error)
	}
	return nil
}
