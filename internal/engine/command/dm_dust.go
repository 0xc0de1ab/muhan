package command

import (
	"fmt"
	"reflect"
	"strings"

	"muhan/internal/krtext"
	"muhan/internal/textfmt"
	"muhan/internal/world/model"
)

type DMDustWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	DustPlayer(model.PlayerID) error
}

func NewDMDustHandler(world DMDustWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmDust(ctx, resolved, world)
	}
}

func dmDust(ctx *Context, resolved ResolvedCommand, world DMDustWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	actorID := strings.TrimSpace(ctx.ActorID)
	var creatureID model.CreatureID
	if player, ok := world.Player(model.PlayerID(actorID)); ok {
		creatureID = player.CreatureID
	} else {
		creatureID = model.CreatureID(actorID)
	}

	creature, ok := world.Creature(creatureID)
	if !ok {
		return StatusDefault, nil
	}

	class := creatureClass(creature)
	if class < model.ClassSubDM {
		return StatusPrompt, nil
	}

	targetArg := dmDustTargetArg(resolved)
	if resolved.Parsed.Num < 2 || targetArg == "" {
		ctx.WriteString("\n누구에게 번개를 내릴까요?\n")
		return StatusPrompt, nil
	}

	targetName := legacyLowercizeASCII(targetArg, true)
	targetPlayer, targetCreature, targetSessionID, ok := dmDustFindOnlinePlayer(ctx, world, targetName)
	if !ok || string(targetPlayer.ID) == actorID {
		ctx.WriteString(fmt.Sprintf("%s이 없습니다.\n", targetName))
		return StatusDefault, nil
	}

	// Check target's DM level
	targetClass := creatureClass(targetCreature)
	if targetClass >= model.ClassSubDM {
		casterName := cleanDisplayText(creature.DisplayName)
		if casterName == "" {
			casterName = creature.DisplayName
		}
		targetMsg := textfmt.RenderLegacyColors(fmt.Sprintf("{빨%s이 당신에게 번개를 내리려 합니다!\n}", casterName), textfmt.Options{ANSI: true})
		_ = sendToSessionWithClose(ctx, targetSessionID, targetMsg, false)
		return StatusDefault, nil
	}

	// Print to target in MAGENTA
	targetMsg := textfmt.RenderLegacyColors("{보번개가 하늘에서 떨어집니다 신들의 분노가 진동합니다!\n}", textfmt.Options{ANSI: true})

	// Broadcast to target's room
	roomMsg := fmt.Sprintf("번개가 하늘에서 %s에게 떨어집니다.\n", targetCreature.DisplayName)
	if fn, ok := ctx.Values[ContextRoomBroadcastKey].(RoomBroadcastFunc); ok && fn != nil {
		_ = fn(targetPlayer.RoomID, targetSessionID, roomMsg)
	}

	// Broadcast globally about ashes
	pronoun := "그녀"
	if creatureHasAnyFlag(targetCreature, "PMALES") {
		pronoun = "그"
	}
	tNameClean := cleanDisplayText(targetCreature.DisplayName)
	if tNameClean == "" {
		tNameClean = targetCreature.DisplayName
	}
	globalMsg1 := fmt.Sprintf("\n### %s%s 잿더미가 되버렸습니다! %s에게 조의를 표하십시요.\n",
		tNameClean,
		krtext.Particle(tNameClean, '1'),
		pronoun,
	)
	invokeBroadcast(ctx, globalMsg1)

	// C disconnects first and does not inspect the subsequent filesystem moves.
	_ = world.DustPlayer(targetPlayer.ID)

	// Disconnect target session by sending targetMsg and closing
	_ = sendToSessionWithClose(ctx, targetSessionID, targetMsg, true)

	// Broadcast globally about thunder sound
	globalMsg2 := "### 멀리서 신들의 분노에 천둥소리가 들려옵니다.\n"
	invokeBroadcast(ctx, globalMsg2)

	return StatusDefault, nil
}

func dmDustFindOnlinePlayer(ctx *Context, world DMDustWorld, name string) (model.Player, model.Creature, string, bool) {
	player, creature, session, ok := legacyFindWhoActivePlayer(ctx, world, name)
	return player, creature, session.ID, ok
}

func dmDustTargetArg(resolved ResolvedCommand) string {
	if resolved.Parsed.Num > 1 {
		if target := strings.TrimSpace(resolved.Parsed.Str[1]); target != "" {
			return target
		}
	}
	return getArg(resolved, 0)
}

func sendToSessionWithClose(ctx *Context, sessionID string, text string, closeSession bool) error {
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
			return fmt.Errorf("dm_dust: send session id type %s is not compatible with %s", idValue.Type(), sendValue.Type().In(0))
		}
		idValue = idValue.Convert(sendValue.Type().In(0))
	}
	commandType := sendValue.Type().In(1)
	if commandType.Kind() != reflect.Struct {
		return fmt.Errorf("dm_dust: send command type %s is not struct", commandType)
	}
	commandValue := reflect.New(commandType).Elem()
	writeField := commandValue.FieldByName("Write")
	if writeField.IsValid() && writeField.CanSet() && writeField.Kind() == reflect.String {
		writeField.SetString(text)
	}
	if closeSession {
		closeField := commandValue.FieldByName("Close")
		if closeField.IsValid() && closeField.CanSet() && closeField.Kind() == reflect.Bool {
			closeField.SetBool(true)
		}
	}

	results := sendValue.Call([]reflect.Value{idValue, commandValue})
	if errValue := results[0]; !errValue.IsNil() {
		return errValue.Interface().(error)
	}
	return nil
}

func legacyLowercizeASCII(value string, capitalizeFirst bool) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	buf := []byte(value)
	for i, ch := range buf {
		if ch >= 'A' && ch <= 'Z' {
			buf[i] = ch + ('a' - 'A')
		}
	}
	if capitalizeFirst && buf[0] >= 'a' && buf[0] <= 'z' {
		buf[0] -= 'a' - 'A'
	}
	return string(buf)
}
