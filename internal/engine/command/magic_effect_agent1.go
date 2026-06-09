package command

import (
	"fmt"
	"reflect"
	"strings"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

func mrand(min, max int) int {
	if max <= min {
		return min
	}
	return attackRoll(min, max)
}

func setCreatureStat(world StatusWorld, id model.CreatureID, key string, val int) error {
	updater, ok := world.(interface {
		SetCreatureStat(model.CreatureID, string, int) error
	})
	if !ok {
		return nil
	}
	return updater.SetCreatureStat(id, key, val)
}

func sendToPlayerAgent1(ctx *Context, playerID model.PlayerID, text string) error {
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

func magicEffectRestore(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	targetStr := strings.TrimSpace(getArg(resolved, 1))

	if how == howCast && creatureClass(actor) < legacyClassInvincible {
		ctx.WriteString("\n당신은 그 주술을 사용할 능력이 없습니다.\n")
		return false, nil
	}

	isSelf := targetStr == ""
	if !isSelf && how == howPotion {
		ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	var target magicEffectTarget
	var ok bool
	if isSelf {
		target, ok = magicEffectSelfTarget(ctx, world, actor)
	} else {
		target, ok = magicEffectResolveTarget(ctx, world, actor, getArg(resolved, 1), getOrdinal(resolved, 1))
	}

	if isSelf {
		currMP := creatureStat(actor, "mpCurrent")
		maxMP := creatureStat(actor, "mpMax")
		if maxMP < 1 {
			maxMP = 1
		}
		if currMP == maxMP {
			ctx.WriteString("\n당신은 도주천 주술이 필요없습니다.\n")
			return false, nil
		}
		if creatureClass(actor) == legacyClassInvincible && how == howCast {
			ctx.WriteString("\n자신에게 외울수 없습니다.\n")
			return false, nil
		}

		hpInc := rollDice(2, 10, 0)
		currHP := creatureStat(actor, "hpCurrent")
		maxHP := creatureStat(actor, "hpMax")
		if maxHP < 1 {
			maxHP = 1
		}
		nextHP := currHP + hpInc
		if nextHP > maxHP {
			nextHP = maxHP
		}

		if err := setCreatureStat(world, actor.ID, "hpCurrent", nextHP); err != nil {
			return false, err
		}

		success := mrand(1, 100) < 60
		if success {
			if err := setCreatureStat(world, actor.ID, "mpCurrent", maxMP); err != nil {
				return false, err
			}
		}

		actorName := attackCreatureName(actor)
		if success {
			if how == howCast || how == howWand {
				ctx.WriteString("\n당신은 공중으로 날아 올라  도주천의 주문을 외칩니다.\n천지의 기운이 번개와 폭풍을 동반하면서 몰려와 당신의\n몸에 스며들어와 도력을 회복시킵니다.\n")
				broadcastMsg := "\n" + actorName + krtext.Particle(actorName, '1') + " 공중으로 날아올라 도주천의 주문을 외칩니다.\n천지의 기운이 번개와 폭풍을 동반하면서 몰려와 도력을 \n회복시킵니다.\n"
				if err := roomBroadcast(ctx, actor.RoomID, broadcastMsg); err != nil {
					return false, err
				}
			} else if how == howPotion {
				ctx.WriteString("\n온몸에 진동이 일어나면서 도력이 충만합니다.\n")
			}
		} else {
			if how == howCast || how == howWand {
				ctx.WriteString("\n당신은 공중으로 날아 올라  도주천의 주문을 외칩니다.\n하지만 아무런 반응도 일어나지 않습니다.\n")
				broadcastMsg := "\n" + actorName + krtext.Particle(actorName, '1') + " 공중으로 날아올라 도주천의 주문을 외칩니다.\n하지만 아무런 반응도 일어나지 않습니다.\n"
				if err := roomBroadcast(ctx, actor.RoomID, broadcastMsg); err != nil {
					return false, err
				}
			} else if how == howPotion {
				ctx.WriteString("\n도력 회복에 실패했습니다!!\n")
			}
		}

		return true, nil
	}

	if !ok || target.creature.ID == actor.ID {
		ctx.WriteString("\n그런 사람은 존재하지 않습니다.\n")
		return false, nil
	}

	hpInc := rollDice(2, 10, 0)
	currHP := creatureStat(target.creature, "hpCurrent")
	maxHP := creatureStat(target.creature, "hpMax")
	if maxHP < 1 {
		maxHP = 1
	}
	nextHP := currHP + hpInc
	if nextHP > maxHP {
		nextHP = maxHP
	}

	if err := setCreatureStat(world, target.creature.ID, "hpCurrent", nextHP); err != nil {
		return false, err
	}

	success := mrand(1, 100) < 60
	if success {
		currMP := creatureStat(target.creature, "mpCurrent")
		maxMP := creatureStat(target.creature, "mpMax")
		if maxMP < 1 {
			maxMP = 1
		}
		if currMP < maxMP {
			if err := setCreatureStat(world, target.creature.ID, "mpCurrent", maxMP); err != nil {
				return false, err
			}
		}
	}

	actorName := attackCreatureName(actor)
	targetName := attackCreatureName(target.creature)

	if success {
		ctx.WriteString("\n" + targetName + "에게 무화연 잎을 먹이며 도주천의 주문을 외웁니다.\n그의 단전에 화기가 모이면서 도력이 회복됩니다.\n")
		if target.hasPlayer {
			targetMsg := "\n" + actorName + krtext.Particle(actorName, '1') + " 당신에게 무화연 잎을 먹이며 도주천의 주문을 \n외웁니다.\n당신의 단전에 화기가 모이면서 도력이 회복됩니다.\n"
			if err := sendToPlayerAgent1(ctx, target.player.ID, targetMsg); err != nil {
				return false, err
			}
		}
		broadcastMsg := "\n" + actorName + krtext.Particle(actorName, '1') + " " + targetName + "에게 무화연 잎을 먹이며 도주천의 주문을 \n외웁니다.\n그의 도력이 회복되었습니다.\n"
		if err := roomBroadcast(ctx, actor.RoomID, broadcastMsg); err != nil {
			return false, err
		}
	} else {
		ctx.WriteString("\n" + targetName + "에게 무화연 잎을 먹이며 도주천의 주문을 외웁니다.\n하지만 아무런 반응도 일어나지 않습니다.\n")
		if target.hasPlayer {
			targetMsg := "\n" + actorName + krtext.Particle(actorName, '1') + " 당신에게 무화연 잎을 먹이며 도주천의 주문을 \n외웁니다.\n하지만 아무런 반응도 일어나지 않습니다.\n"
			if err := sendToPlayerAgent1(ctx, target.player.ID, targetMsg); err != nil {
				return false, err
			}
		}
		broadcastMsg := "\n" + actorName + krtext.Particle(actorName, '1') + " " + targetName + "에게 무화연 잎을 먹이며 도주천의 주문을 \n외웁니다.\n하지만 아무런 반응도 일어나지 않습니다.\n"
		if err := roomBroadcast(ctx, actor.RoomID, broadcastMsg); err != nil {
			return false, err
		}
	}

	return true, nil
}
