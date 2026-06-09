package command

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"muhan/internal/world/model"
)

type DMCastWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Players() []model.Player
	RoomPlayers(roomID model.RoomID) []model.Player
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdateCreatureStat(model.CreatureID, string, int) error
	MovePlayerToRoom(model.PlayerID, model.RoomID) error
	BroadcastAll(msg string) error
}

type dmCastEffectExpirationWorld interface {
	SetEffectExpiration(model.CreatureID, string, int64)
}

type dmCastPlayerTagWorld interface {
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
}

func NewDMCastHandler(world DMCastWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmCast(ctx, resolved, world)
	}
}

func dmCast(ctx *Context, resolved ResolvedCommand, world DMCastWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	player, creature, ok := dmCastActor(world, strings.TrimSpace(ctx.ActorID))
	if !ok {
		return StatusDefault, nil
	}

	class := creatureClass(creature)
	if class < legacyClassSubDM {
		return StatusPrompt, nil
	}

	if resolved.Parsed.Num < 2 {
		ctx.WriteString("무엇을 외웁니까?\n")
		return StatusPrompt, nil
	}

	firstArg := resolved.Parsed.Str[1]
	var rcast bool
	var sp string
	if resolved.Parsed.Num > 2 && firstArg == "-r" {
		rcast = true
		sp = resolved.Parsed.Str[2]
	} else if resolved.Parsed.Num > 2 && strings.HasPrefix(firstArg, "-") {
		ctx.WriteString("Invalid cast flag.\n")
		return StatusPrompt, nil
	} else {
		sp = firstArg
	}

	spell, ok, ambiguous := castSpellByName(sp)
	if !ok {
		if ambiguous {
			ctx.WriteString("주문이름이 이상합니다.\n")
			return StatusDefault, nil
		}
		ctx.WriteString("그런 주문은 없습니다.\n")
		return StatusDefault, nil
	}

	if spell.power == magicPowerRecall {
		if rcast {
			roomPlayers := world.RoomPlayers(player.RoomID)
			ctx.WriteString(fmt.Sprintf("당신은 %s 주문을 방에 있는 사람에게 외웠습니다.\n", spell.name))
			broadcastMsg := fmt.Sprintf("%s이 %s 주문을 방에 있는 사람들에게 외웠습니다.\n", creature.DisplayName, spell.name)
			_ = roomBroadcast(ctx, player.RoomID, broadcastMsg)
			for _, targetPlayer := range roomPlayers {
				targetMsg := fmt.Sprintf("%s이 %s를 당신에게 외웠습니다.\n", creature.DisplayName, spell.name)
				_ = sendToPlayerCast(ctx, targetPlayer.ID, targetMsg)
				if err := world.MovePlayerToRoom(targetPlayer.ID, "room:1"); err != nil {
					return StatusDefault, err
				}
			}
			return StatusDefault, nil
		} else {
			ctx.WriteString("그주문을 모두에게 외울수 없습니다.\n")
			return StatusDefault, nil
		}
	}

	if !isSupportedDMSpell(spell.power) {
		if rcast {
			ctx.WriteString("그런 주문은 않됩니다.\n")
		} else {
			ctx.WriteString("그주문을 모두에게 외울수 없습니다.\n")
		}
		return StatusDefault, nil
	}

	if rcast {
		roomPlayers := world.RoomPlayers(player.RoomID)
		for _, targetPlayer := range roomPlayers {
			targetCrt, ok := world.Creature(targetPlayer.CreatureID)
			if !ok {
				continue
			}
			if creatureHasAnyFlag(targetCrt, "PDMINV") {
				continue
			}
			if err := dmGspellEffect(world, targetPlayer.ID, targetCrt, spell.power); err != nil {
				return StatusDefault, err
			}
			targetMsg := fmt.Sprintf("%s이 %s를 당신에게 외웠습니다.\n", creature.DisplayName, spell.name)
			_ = sendToPlayerCast(ctx, targetPlayer.ID, targetMsg)
		}
		ctx.WriteString(fmt.Sprintf("당신은 %s 주문을 방에 있는 사람들에게 외웠습니다.\n", spell.name))
		broadcastMsg := fmt.Sprintf("%s이 %s 주문을 방에 있는 사람들에게 외웠습니다.\n", creature.DisplayName, spell.name)
		_ = roomBroadcast(ctx, player.RoomID, broadcastMsg)
	} else {
		players := world.Players()
		for _, targetPlayer := range players {
			if targetPlayer.ID == player.ID {
				continue
			}
			targetCrt, ok := world.Creature(targetPlayer.CreatureID)
			if !ok {
				continue
			}
			if creatureHasAnyFlag(targetCrt, "PDMINV") {
				continue
			}
			if err := dmGspellEffect(world, targetPlayer.ID, targetCrt, spell.power); err != nil {
				return StatusDefault, err
			}
			targetMsg := fmt.Sprintf("%s이 %s 주문을 당신에게 외웠습니다.\n", creature.DisplayName, spell.name)
			_ = sendToPlayerCast(ctx, targetPlayer.ID, targetMsg)
		}
		ctx.WriteString(fmt.Sprintf("당신은 %s 주문을 모두에게 외웠습니다.\n", spell.name))
		broadcastMsg := fmt.Sprintf("%s이 %s 주문을 모두에게 외웠습니다.\n", creature.DisplayName, spell.name)
		_ = world.BroadcastAll(broadcastMsg)
	}

	return StatusDefault, nil
}

func dmCastActor(world DMCastWorld, actorID string) (model.Player, model.Creature, bool) {
	playerID := model.PlayerID(actorID)
	if player, ok := world.Player(playerID); ok {
		if player.CreatureID.IsZero() {
			return player, model.Creature{}, false
		}
		creature, ok := world.Creature(player.CreatureID)
		if !ok {
			return player, model.Creature{}, false
		}
		if player.RoomID.IsZero() {
			player.RoomID = creature.RoomID
		}
		return player, creature, true
	}

	creatureID := model.CreatureID(actorID)
	creature, ok := world.Creature(creatureID)
	if !ok || creature.PlayerID.IsZero() {
		return model.Player{}, model.Creature{}, false
	}
	player, ok := world.Player(creature.PlayerID)
	if !ok {
		return model.Player{}, model.Creature{}, false
	}
	if player.RoomID.IsZero() {
		player.RoomID = creature.RoomID
	}
	return player, creature, true
}

func isSupportedDMSpell(power int) bool {
	switch power {
	case magicPowerVigor, magicPowerMend, magicPowerRestore, magicPowerFullHeal,
		magicPowerBless, magicPowerProtection, magicPowerInvisibility, magicPowerDetectMagic,
		magicPowerResistFire, magicPowerResistMagic, magicPowerDetectInvisible, magicPowerFly,
		magicPowerLight, magicPowerLevitate, magicPowerKnowAlignment, magicPowerEarthShield,
		magicPowerCurePoison, magicPowerRemoveDisease:
		return true
	}
	return false
}

func dmGspellEffect(world DMCastWorld, playerID model.PlayerID, target model.Creature, power int) error {
	switch power {
	case magicPowerVigor:
		hpMax := creatureStat(target, "hpMax")
		hpCur := creatureStat(target, "hpCurrent")
		rolled := mrand(1, 6) + 4 + 2
		next := hpCur + rolled
		if next > hpMax {
			next = hpMax
		}
		if err := world.UpdateCreatureStat(target.ID, "hpCurrent", next); err != nil {
			return err
		}
	case magicPowerMend:
		hpMax := creatureStat(target, "hpMax")
		hpCur := creatureStat(target, "hpCurrent")
		rolled := mrand(2, 10) + 4 + 4
		next := hpCur + rolled
		if next > hpMax {
			next = hpMax
		}
		if err := world.UpdateCreatureStat(target.ID, "hpCurrent", next); err != nil {
			return err
		}
	case magicPowerRestore:
		hpMax := creatureStat(target, "hpMax")
		mpMax := creatureStat(target, "mpMax")
		if err := world.UpdateCreatureStat(target.ID, "hpCurrent", hpMax); err != nil {
			return err
		}
		if err := world.UpdateCreatureStat(target.ID, "mpCurrent", mpMax); err != nil {
			return err
		}
	case magicPowerFullHeal:
		hpMax := creatureStat(target, "hpMax")
		if err := world.UpdateCreatureStat(target.ID, "hpCurrent", hpMax); err != nil {
			return err
		}
	case magicPowerBless:
		if err := dmCastUpdateTags(world, playerID, target.ID, []string{"PBLESS"}, nil); err != nil {
			return err
		}
		dmCastSetEffectExpiration(world, target.ID, "PBLESS")
	case magicPowerProtection:
		if err := dmCastUpdateTags(world, playerID, target.ID, []string{"PPROTE"}, nil); err != nil {
			return err
		}
		dmCastSetEffectExpiration(world, target.ID, "PPROTE")
	case magicPowerInvisibility:
		if err := dmCastUpdateTags(world, playerID, target.ID, []string{"PINVIS"}, nil); err != nil {
			return err
		}
		dmCastSetEffectExpiration(world, target.ID, "PINVIS")
	case magicPowerDetectMagic:
		if err := dmCastUpdateTags(world, playerID, target.ID, []string{"PDMAGI"}, nil); err != nil {
			return err
		}
		dmCastSetEffectExpiration(world, target.ID, "PDMAGI")
	case magicPowerResistFire:
		if err := dmCastUpdateTags(world, playerID, target.ID, []string{"PRFIRE"}, nil); err != nil {
			return err
		}
		dmCastSetEffectExpiration(world, target.ID, "PRFIRE")
	case magicPowerResistMagic:
		if err := dmCastUpdateTags(world, playerID, target.ID, []string{"PRMAGI"}, nil); err != nil {
			return err
		}
		dmCastSetEffectExpiration(world, target.ID, "PRMAGI")
	case magicPowerDetectInvisible:
		if err := dmCastUpdateTags(world, playerID, target.ID, []string{"PDINVI"}, nil); err != nil {
			return err
		}
		dmCastSetEffectExpiration(world, target.ID, "PDINVI")
	case magicPowerFly:
		if err := dmCastUpdateTags(world, playerID, target.ID, []string{"PFLYSP"}, nil); err != nil {
			return err
		}
		dmCastSetEffectExpiration(world, target.ID, "PFLYSP")
	case magicPowerLight:
		if err := dmCastUpdateTags(world, playerID, target.ID, []string{"PLIGHT"}, nil); err != nil {
			return err
		}
		dmCastSetEffectExpiration(world, target.ID, "PLIGHT")
	case magicPowerLevitate:
		if err := dmCastUpdateTags(world, playerID, target.ID, []string{"PLEVIT"}, nil); err != nil {
			return err
		}
		dmCastSetEffectExpiration(world, target.ID, "PLEVIT")
	case magicPowerKnowAlignment:
		if err := dmCastUpdateTags(world, playerID, target.ID, []string{"PKNOWA"}, nil); err != nil {
			return err
		}
		dmCastSetEffectExpiration(world, target.ID, "PKNOWA")
	case magicPowerEarthShield:
		if err := dmCastUpdateTags(world, playerID, target.ID, []string{"PSSHLD"}, nil); err != nil {
			return err
		}
		dmCastSetEffectExpiration(world, target.ID, "PSSHLD")
	case magicPowerCurePoison:
		if err := dmCastUpdateTags(world, playerID, target.ID, nil, []string{"PPOISN"}); err != nil {
			return err
		}
	case magicPowerRemoveDisease:
		if err := dmCastUpdateTags(world, playerID, target.ID, nil, []string{"PDISEA"}); err != nil {
			return err
		}
	}
	return nil
}

func dmCastUpdateTags(world DMCastWorld, playerID model.PlayerID, creatureID model.CreatureID, add []string, remove []string) error {
	if !creatureID.IsZero() {
		if _, err := world.UpdateCreatureTags(creatureID, add, remove); err != nil {
			return err
		}
	}
	playerTagger, ok := world.(dmCastPlayerTagWorld)
	if ok && !playerID.IsZero() {
		if _, err := playerTagger.UpdatePlayerTags(playerID, add, remove); err != nil {
			return err
		}
	}
	return nil
}

func dmCastSetEffectExpiration(world DMCastWorld, creatureID model.CreatureID, tag string) {
	if creatureID.IsZero() {
		return
	}
	expirer, ok := world.(dmCastEffectExpirationWorld)
	if !ok {
		return
	}
	expirer.SetEffectExpiration(creatureID, tag, time.Now().Unix()+3600)
}

func sendToPlayerCast(ctx *Context, playerID model.PlayerID, text string) error {
	if playerID.IsZero() || ctx == nil || ctx.Values == nil {
		return nil
	}
	activeValue := reflect.ValueOf(ctx.Values["game.activeSessions"])
	sendValue := reflect.ValueOf(ctx.Values["game.sendToSession"])
	if !activeValue.IsValid() || activeValue.Kind() != reflect.Func ||
		activeValue.Type().NumIn() != 0 || activeValue.Type().NumOut() != 1 ||
		!sendValue.IsValid() || sendValue.Kind() != reflect.Func ||
		sendValue.Type().NumIn() != 2 || sendValue.Type().NumOut() != 1 {
		return nil
	}
	if sendValue.Type().Out(0) != reflect.TypeOf((*error)(nil)).Elem() ||
		sendValue.Type().In(1).Kind() != reflect.Struct {
		return nil
	}

	out := activeValue.Call(nil)[0]
	if out.Kind() != reflect.Slice {
		return nil
	}

	send := func(id string, text string) error {
		idValue := reflect.ValueOf(id)
		if !idValue.Type().AssignableTo(sendValue.Type().In(0)) {
			if !idValue.Type().ConvertibleTo(sendValue.Type().In(0)) {
				return fmt.Errorf("magic: send session id type %s is not compatible with %s", idValue.Type(), sendValue.Type().In(0))
			}
			idValue = idValue.Convert(sendValue.Type().In(0))
		}
		commandType := sendValue.Type().In(1)
		if commandType.Kind() != reflect.Struct {
			return fmt.Errorf("magic: send command type %s is not struct", commandType)
		}
		commandValue := reflect.New(commandType).Elem()
		writeField := commandValue.FieldByName("Write")
		if !writeField.IsValid() || !writeField.CanSet() || writeField.Kind() != reflect.String {
			return fmt.Errorf("magic: send command type %s does not expose settable Write string field", commandType)
		}
		writeField.SetString(text)

		results := sendValue.Call([]reflect.Value{idValue, commandValue})
		if errValue := results[0]; !errValue.IsNil() {
			return errValue.Interface().(error)
		}
		return nil
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
			sessionID := fmt.Sprint(idField.Interface())
			if sessionID == ctx.SessionID {
				ctx.WriteString(text)
				return nil
			}
			return send(sessionID, text)
		}
	}
	return nil
}
