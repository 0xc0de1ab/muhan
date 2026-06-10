package command

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const (
	howCast   = 1
	howPotion = 2
	howScroll = 3
	howWand   = 4
)

// spellFail is provided centrally by magic_resistance.go (Package 2 port of C magic8.c formula).
// The old 2-arg duplicate implementation was removed for build health.

func determineHow(world StatusWorld, object model.ObjectInstance) int {
	if object.ID.IsZero() {
		return howCast
	}
	legacyType := objectLegacyTypeOrKind(world, object)
	switch legacyType {
	case legacyObjectPotion:
		return howPotion
	case legacyObjectScroll:
		return howScroll
	case legacyObjectWand:
		return howWand
	}
	return howCast
}

func sendToPlayer(ctx *Context, playerID model.PlayerID, text string) error {
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

func broadcastRom2(ctx *Context, world StatusWorld, roomID model.RoomID, actorPlayerID, targetPlayerID model.PlayerID, text string) error {
	if ctx == nil || ctx.Values == nil || roomID.IsZero() || text == "" {
		return nil
	}

	activeValue := reflect.ValueOf(ctx.Values["game.activeSessions"])
	sendValue := reflect.ValueOf(ctx.Values["game.sendToSession"])
	if !activeValue.IsValid() || activeValue.Kind() != reflect.Func ||
		activeValue.Type().NumIn() != 0 || activeValue.Type().NumOut() != 1 ||
		!sendValue.IsValid() || sendValue.Kind() != reflect.Func ||
		sendValue.Type().NumIn() != 2 || sendValue.Type().NumOut() != 1 {
		return roomBroadcast(ctx, roomID, text)
	}

	room, ok := world.Room(roomID)
	if !ok {
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

	out := activeValue.Call(nil)[0]
	if out.Kind() != reflect.Slice {
		return roomBroadcast(ctx, roomID, text)
	}

	targets := make(map[model.PlayerID]struct{})
	for _, pid := range room.PlayerIDs {
		if pid != actorPlayerID && pid != targetPlayerID {
			targets[pid] = struct{}{}
		}
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
		pid := model.PlayerID(actorField.String())
		if _, exists := targets[pid]; exists {
			sessionID := fmt.Sprint(idField.Interface())
			_ = send(sessionID, text)
		}
	}

	return nil
}

func magicEffectVigor(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)

	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 5 {
			ctx.WriteString("당신의 도력이 부족합니다.\n")
			return false, nil
		}

		class := creatureClass(actor)
		if class != model.ClassCleric && class != model.ClassPaladin && class < model.ClassInvincible {
			ctx.WriteString("불제자와 무사만 쓸 수 있는 마법입니다.\n")
			return false, nil
		}

		if class >= model.ClassInvincible &&
			!creatureHasAnyFlag(actor, "SCLERIC", "clericTraining", "clericSpell", "clericMode") &&
			!creatureHasAnyFlag(actor, "SPALADIN", "paladinTraining", "paladinSpell", "paladinMode") {
			ctx.WriteString("\n불제자나 무사를 무적수련하지 않았습니다..\n")
			return false, nil
		}
	}

	if how == howCast && !creatureHasAnyFlag(actor, "SVIGOR", "vigorSpell") {
		ctx.WriteString("당신은 아직 그 주술을 터득하지 못했습니다.\n")
		return false, nil
	}

	class := creatureClass(actor)
	if class == model.ClassBarbarian || class == model.ClassFighter {
		if how != howPotion {
			if spellFail(actor) {
				if how == howCast {
					mp := creatureStat(actor, "mpCurrent")
					_ = setCreatureStat(world, actor.ID, "mpCurrent", maxInt(0, mp-2))
				}
				return false, nil
			}
		}
	}

	targetStr := strings.TrimSpace(getArg(resolved, 1))
	isSelf := targetStr == "" || targetStr == "나" || targetStr == "자신"
	if !isSelf && how == howPotion {
		ctx.WriteString("그 물건은 자신에게만 사용할 수 있습니다.\n")
		return false, nil
	}

	var target magicEffectTarget
	var ok bool
	if isSelf {
		target, ok = magicEffectSelfTarget(ctx, world, actor)
	} else {
		target, ok = magicEffectResolveTarget(ctx, world, actor, getArg(resolved, 1), getOrdinal(resolved, 1))
		if ok && target.creature.ID == actor.ID {
			isSelf = true
		}
	}

	if isSelf {
		var heal int
		if how == howCast {
			intelBonus := legacyStatBonus(creatureStat(actor, "intelligence"))
			pietyBonus := legacyStatBonus(creatureStat(actor, "piety"))
			maxBonus := maxInt(intelBonus, pietyBonus)

			var clericBonus int
			if class == model.ClassCleric {
				clericBonus = (actor.Level + 3) / 4
				clericBonus += mrand(1, 1+clericBonus/2)
			}

			var paladinBonus int
			if class == model.ClassPaladin {
				paladinBonus = ((actor.Level + 3) / 4) / 2
				paladinBonus += mrand(1, 1+((actor.Level+3)/4)/4)
			}

			num := maxBonus + 10
			size := clericBonus + paladinBonus + 1
			plus := mrand(1, 6)

			heal = rollDice(num, size, plus)

			if room, rOk := world.Room(actor.RoomID); rOk && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
				heal += mrand(1, 10)
				ctx.WriteString("이 방의 기운이 당신의 주술력을 강화시킵니다.\n")
			}
		} else {
			heal = mrand(1, 10)
		}

		heal = maxInt(1, heal)

		currentHP := creatureStat(actor, "hpCurrent")
		maxHP := creatureStat(actor, "hpMax")
		if maxHP < 1 {
			maxHP = 1
		}
		nextHP := minInt(maxHP, currentHP+heal)
		if err := setCreatureStat(world, actor.ID, "hpCurrent", nextHP); err != nil {
			return false, err
		}

		if how == howCast || how == howScroll {
			ctx.WriteString("당신은 합장을 하고서 회복 주문을 외웁니다.\n빛의 정기가 온몸에 스며들면서 체력이 향상되었습니다.\n")
			actorName := attackCreatureName(actor)
			broadcastMsg := "\n" + actorName + krtext.Particle(actorName, '1') + " 합장을 하고서 주문을 외웁니다.\n빛의 정기가 그의 몸으로 모이는 것이 보입니다.\n"
			if err := roomBroadcast(ctx, actor.RoomID, broadcastMsg); err != nil {
				return false, err
			}
		} else {
			ctx.WriteString("당신의 체력이 향상되었습니다.\n")
		}
		return true, nil
	}

	if how == howPotion {
		ctx.WriteString("그 물건은 자신에게만 사용할 수 있습니다.\n")
		return false, nil
	}

	if !ok || target.creature.ID.IsZero() {
		ctx.WriteString("그런 사람은 존재하지 않습니다.\n")
		return false, nil
	}

	targetHP := creatureStat(target.creature, "hpCurrent")
	targetMaxHP := creatureStat(target.creature, "hpMax")
	if targetMaxHP < 1 {
		targetMaxHP = 1
	}

	if how == howCast {
		if targetHP == targetMaxHP {
			targetName := attackCreatureName(target.creature)
			ctx.WriteString("\n" + targetName + krtext.Particle(targetName, '0') + " 회복이 필요없습니다.\n")
			return false, nil
		}
	}

	var heal int
	if how == howCast {
		intelBonus := legacyStatBonus(creatureStat(actor, "intelligence"))
		pietyBonus := legacyStatBonus(creatureStat(actor, "piety"))
		maxBonus := maxInt(intelBonus, pietyBonus)

		var clericBonus int
		if class == model.ClassCleric {
			clericBonus = (actor.Level + 3) / 4
			clericBonus += mrand(1, 1+clericBonus/2)
		}

		var paladinBonus int
		if class == model.ClassPaladin {
			paladinBonus = ((actor.Level + 3) / 4) / 2
			paladinBonus += mrand(1, 1+((actor.Level+3)/4)/4)
		}

		num := maxBonus + 10
		size := clericBonus + paladinBonus + 1
		plus := mrand(1, 10)

		heal = rollDice(num, size, plus)

		if room, rOk := world.Room(actor.RoomID); rOk && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			heal += mrand(1, 10)
			ctx.WriteString("이 방의 기운이 당신의 주술력을 강화시킵니다.\n")
		}
	} else {
		heal = mrand(1, 10)
	}

	heal = maxInt(1, heal)
	nextHP := targetHP + heal
	expadd := heal

	if nextHP > targetMaxHP {
		nextHP = targetMaxHP
		expadd = 0
	}

	if err := setCreatureStat(world, target.creature.ID, "hpCurrent", nextHP); err != nil {
		return false, err
	}

	if how == howCast || how == howScroll || how == howWand {
		actorName := attackCreatureName(actor)
		targetName := attackCreatureName(target.creature)

		ctx.WriteString("당신은 합장을 하고서 " + targetName + "의 회복을 기원하는 주문을 외웁니다.\n빛의 정기가 그의 몸으로 스며들고 있습니다.\n")

		if target.hasPlayer {
			targetMsg := actorName + krtext.Particle(actorName, '1') + " 합장을 하고서 당신의 회복을 기원하는 주문을 외웁니다.\n빛의 정기가 당신의 몸에 스며들면서 체력이 향상되었습니다.\n"
			_ = sendToPlayer(ctx, target.player.ID, targetMsg)
		}

		broadcastMsg := "\n" + actorName + krtext.Particle(actorName, '1') + " " + targetName + "에게 합장을 하고서 회복을 기원하는 주문을 외웁니다.\n빛의 정기가 당신의 몸을 스치며 그의 몸으로 모이는\n것이 느껴집니다.\n"
		if err := broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, broadcastMsg); err != nil {
			return false, err
		}

		if expadd > 0 && target.creature.Kind != model.CreatureKindMonster && target.creature.ID != actor.ID {
			if mrand(1, 2) == 1 {
				casterExp := creatureStat(actor, "experience")
				_ = setCreatureStat(world, actor.ID, "experience", casterExp+expadd)

				opts := textOptionsFromContext(ctx)
				expMsg := fmt.Sprintf("\n당신의 선행으로 신에게서 경험치 %d점을 받았습니다.\n", expadd)
				ctx.WriteString(colorText(opts, "36", expMsg))
			}
		}
	}

	return true, nil
}

func magicEffectMend(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)

	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 10 {
			ctx.WriteString("당신의 도력이 부족합니다.\n")
			return false, nil
		}
	}

	if how == howCast && !creatureHasAnyFlag(actor, "SMENDW", "mendSpell") {
		ctx.WriteString("당신은 아직 그 주술을 터득하지 못했습니다.\n")
		return false, nil
	}

	class := creatureClass(actor)
	if class == model.ClassBarbarian || class == model.ClassFighter || class == model.ClassAssassin {
		if how != howPotion {
			if spellFail(actor) {
				if how == howCast {
					mp := creatureStat(actor, "mpCurrent")
					_ = setCreatureStat(world, actor.ID, "mpCurrent", maxInt(0, mp-4))
				}
				return false, nil
			}
		}
	}

	targetStr := strings.TrimSpace(getArg(resolved, 1))
	isSelf := targetStr == "" || targetStr == "나" || targetStr == "자신"
	if !isSelf && how == howPotion {
		ctx.WriteString("그 물건은 자신에게만 사용할 수 있습니다.\n")
		return false, nil
	}

	var target magicEffectTarget
	var ok bool
	if isSelf {
		target, ok = magicEffectSelfTarget(ctx, world, actor)
	} else {
		target, ok = magicEffectResolveTarget(ctx, world, actor, getArg(resolved, 1), getOrdinal(resolved, 1))
		if ok && target.creature.ID == actor.ID {
			isSelf = true
		}
	}

	if isSelf {
		var heal int
		if how == howCast {
			var levelFactor int
			if class >= model.ClassInvincible {
				levelFactor = mrand(1, (actor.Level+24)/25)
			} else if class == model.ClassCleric {
				clericBonus := (actor.Level + 3) / 4
				levelFactor = clericBonus*2 + mrand(1, 1+clericBonus/2)
			} else if class == model.ClassPaladin {
				paladinBonus := (actor.Level + 3) / 4
				levelFactor = paladinBonus + mrand(1, 1+paladinBonus/3)
			}

			intelBonus := legacyStatBonus(creatureStat(actor, "intelligence"))
			pietyBonus := legacyStatBonus(creatureStat(actor, "piety"))

			num := intelBonus + pietyBonus + 20
			size := levelFactor + 1
			plus := rollDice(2, 6, 5)

			heal = rollDice(num, size, plus)

			if room, rOk := world.Room(actor.RoomID); rOk && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
				heal += mrand(1, 10) + 1
				ctx.WriteString("이 방의 기운이 당신의 주술력을 강화시킵니다.\n")
			}
		} else {
			heal = rollDice(6, mrand(1, (actor.Level+24)/25), 5)
		}

		heal = maxInt(1, heal)
		if actor.Kind != model.CreatureKindPlayer {
			heal /= 2
		}
		heal = maxInt(1, heal)

		currentHP := creatureStat(actor, "hpCurrent")
		maxHP := creatureStat(actor, "hpMax")
		if maxHP < 1 {
			maxHP = 1
		}
		nextHP := minInt(maxHP, currentHP+heal)
		if err := setCreatureStat(world, actor.ID, "hpCurrent", nextHP); err != nil {
			return false, err
		}

		if how == howCast || how == howScroll {
			ctx.WriteString("당신은 기공팔식의 자세를 취하며 원기회복의 주문을 외웁니다.\n지기의 뜨거운 기운이 당신의 몸에 가득차 체력을 향상시킵니다.\n")
			actorName := attackCreatureName(actor)
			broadcastMsg := "\n" + actorName + krtext.Particle(actorName, '1') + " 기공팔식의 자세를 취하며 원기회복의 주문을 외웁니다.\n지기의 뜨거운 기운이 그에게 흘러가는 것이 느껴집니다.\n"
			if err := roomBroadcast(ctx, actor.RoomID, broadcastMsg); err != nil {
				return false, err
			}
		} else {
			ctx.WriteString("몸의 체력이 많이 회복되었습니다.\n")
		}
		return true, nil
	}

	if how == howPotion {
		ctx.WriteString("그 물건은 자신에게만 사용할 수 있습니다.\n")
		return false, nil
	}

	if !ok || target.creature.ID.IsZero() {
		ctx.WriteString("그러한 사람은 존재하지 않습니다.\n")
		return false, nil
	}

	targetHP := creatureStat(target.creature, "hpCurrent")
	targetMaxHP := creatureStat(target.creature, "hpMax")
	if targetMaxHP < 1 {
		targetMaxHP = 1
	}

	if how == howCast {
		if targetHP == targetMaxHP {
			targetName := attackCreatureName(target.creature)
			ctx.WriteString("\n" + targetName + krtext.Particle(targetName, '0') + " 원기회복이 필요없습니다.\n")
			return false, nil
		}
	}

	var heal int
	if how == howCast {
		var levelFactor int
		if class >= model.ClassInvincible {
			levelFactor = mrand(1, (actor.Level+19)/20)
		} else if class == model.ClassCleric {
			clericBonus := (actor.Level + 3) / 4
			levelFactor = clericBonus*2 + mrand(1, 1+clericBonus/2)
		} else if class == model.ClassPaladin {
			paladinBonus := (actor.Level + 3) / 4
			levelFactor = paladinBonus + mrand(1, 1+paladinBonus/3)
		}

		intelBonus := legacyStatBonus(creatureStat(actor, "intelligence"))
		pietyBonus := legacyStatBonus(creatureStat(actor, "piety"))

		num := intelBonus + pietyBonus + 30
		size := levelFactor + 1
		plus := rollDice(2, 6, 10)

		heal = rollDice(num, size, plus)

		if room, rOk := world.Room(actor.RoomID); rOk && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			heal += mrand(1, 15) + 1
			ctx.WriteString("이 방의 기운이 당신의 주술력을 강화시킵니다.\n")
		}
	} else {
		heal = rollDice(6, mrand(1, (actor.Level+19)/20), 10)
	}

	heal = maxInt(1, heal)
	nextHP := targetHP + heal
	expadd := heal

	if nextHP > targetMaxHP {
		nextHP = targetMaxHP
		expadd = 0
	}

	if err := setCreatureStat(world, target.creature.ID, "hpCurrent", nextHP); err != nil {
		return false, err
	}

	if how == howCast || how == howScroll || how == howWand {
		actorName := attackCreatureName(actor)
		targetName := attackCreatureName(target.creature)

		ctx.WriteString("당신은 " + targetName + "에게 내공을 주입하며 원기회복의 주문을 겁니다.\n그의 몸안에서 지기의 뜨거운 기운이 느껴집니다.")

		if target.hasPlayer {
			targetMsg := actorName + krtext.Particle(actorName, '1') + " 당신에게 내공을 주입하며 원기회복의 주문을 겁니다.\n당신의 몸안에서 지기의 뜨거운 기운과 체력이 많이 향상되는\n것이 느껴집니다.\n"
			_ = sendToPlayer(ctx, target.player.ID, targetMsg)
		}

		broadcastMsg := "\n" + actorName + krtext.Particle(actorName, '1') + " " + targetName + "에게 내공을 주입하며 원기회복의 주문을 겁니다.\n그에게 뜨거운 지기의 기운이 흘러가는 것이 느껴집니다.\n"
		if err := broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, broadcastMsg); err != nil {
			return false, err
		}

		if expadd > 0 && target.creature.Kind != model.CreatureKindMonster && target.creature.ID != actor.ID {
			if mrand(1, 3) == 1 {
				casterExp := creatureStat(actor, "experience")
				_ = setCreatureStat(world, actor.ID, "experience", casterExp+expadd)

				opts := textOptionsFromContext(ctx)
				expMsg := fmt.Sprintf("\n당신의 선행으로 신에게서 경험치 %d점을 받았습니다.\n", expadd)
				ctx.WriteString(colorText(opts, "36", expMsg))
			}
		}
	}

	return true, nil
}

func magicEffectHeal(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)

	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 50 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}

		class := creatureClass(actor)
		if class != model.ClassCleric && class != model.ClassPaladin && class < model.ClassInvincible {
			ctx.WriteString("\n불제자와 무사만이 이 주술을 사용할 수 있습니다.\n")
			return false, nil
		}

		if class >= model.ClassInvincible &&
			!creatureHasAnyFlag(actor, "SCLERIC", "clericTraining", "clericSpell", "clericMode") &&
			!creatureHasAnyFlag(actor, "SPALADIN", "paladinTraining", "paladinSpell", "paladinMode") {
			ctx.WriteString("\n불제자나 무사를 무적수련하지 않았습니다..\n")
			return false, nil
		}
	}

	if how == howCast && !creatureHasAnyFlag(actor, "SFHEAL", "fullHealSpell") {
		ctx.WriteString("\n당신은 아직 그런 주술을 터득하지 못했습니다.\n")
		return false, nil
	}

	targetStr := strings.TrimSpace(getArg(resolved, 1))
	isSelf := targetStr == ""
	if !isSelf && how == howPotion {
		ctx.WriteString("그 물건은 자신에게만 사용가능합니다.\n")
		return false, nil
	}

	var target magicEffectTarget
	var ok bool
	if isSelf {
		target, ok = magicEffectSelfTarget(ctx, world, actor)
	} else {
		target, ok = magicEffectResolveFullHealTarget(ctx, world, actor, getArg(resolved, 1), getOrdinal(resolved, 1))
	}

	if isSelf {
		class := creatureClass(actor)
		if class != model.ClassBulsa {
			ok, err := decrementDailyFullHealLimit(world, actor)
			if err != nil {
				return false, err
			}
			if !ok && how == howCast && class < model.ClassSubDM {
				ctx.WriteString("\n당신의 몸이 너무 피곤해 이 주술을 더 이상 펼칠 수 없습니다.\n")
				if class == model.ClassCaretaker {
					ctx.WriteString("다른 사용자에게 완치 주문을 사용하면 회복이 가능합니다.\n")
				}
				return false, nil
			}
		}

		currentHP := creatureStat(actor, "hpCurrent")
		maxHP := creatureStat(actor, "hpMax")
		if maxHP < 1 {
			maxHP = 1
		}
		if currentHP == maxHP {
			ctx.WriteString("\n당신은 완치술이 필요없습니다.\n")
			return false, nil
		}

		if err := setCreatureStat(world, actor.ID, "hpCurrent", maxHP); err != nil {
			return false, err
		}

		if how == howCast {
			if err := magicEffectDeductMPLegacy(world, actor, 50); err != nil {
				return false, err
			}
		}

		if how == howCast || how == howScroll {
			ctx.WriteString("\n당신은 천부공을 끌어올리며 완치 주문을 외웁니다.\n천상의 기운들이 당신의 몸으로 모이면서 체력을 최상으로 \n올려 줍니다.\n")
			actorName := attackCreatureName(actor)
			broadcastMsg := "\n" + actorName + krtext.Particle(actorName, '1') + " 천부공 자세를 취하면서 완치주문을 외웠습니다.\n천상의 기운들이 그에게로 모이는 것이 느껴집니다.\n"
			if err := roomBroadcast(ctx, actor.RoomID, broadcastMsg); err != nil {
				return false, err
			}
		} else {
			ctx.WriteString("\n갑자기 몸이 펴지면서 몸의 체력이 최고의 상태로 올라갑니다.\n\"이야~~얍.. 힘이 넘친다.\"\n")
		}
		return true, nil
	}

	if how == howPotion {
		ctx.WriteString("그 물건은 자신에게만 사용가능합니다.\n")
		return false, nil
	}

	if !ok || target.creature.ID.IsZero() {
		ctx.WriteString("\n그런 사람이 존재하지 습니다.\n")
		return false, nil
	}

	if creatureClass(actor) == model.ClassCaretaker {
		if err := decrementCaretakerFullHealCur(world, actor); err != nil {
			return false, err
		}
		if refreshed, ok := world.Creature(actor.ID); ok {
			actor = refreshed
		}
	}

	dailyOK, err := decrementDailyFullHealLimit(world, actor)
	if err != nil {
		return false, err
	}
	if !dailyOK && how == howCast && creatureClass(actor) < model.ClassCaretaker && target.hasPlayer {
		ctx.WriteString("\n당신의 몸이 너무 피곤해 이 주술을 더 이상 펼칠 수 없습니다.\n")
		return false, nil
	}

	targetHP := creatureStat(target.creature, "hpCurrent")
	targetMaxHP := creatureStat(target.creature, "hpMax")
	if targetMaxHP < 1 {
		targetMaxHP = 1
	}

	if targetHP == targetMaxHP {
		targetName := attackCreatureName(target.creature)
		ctx.WriteString("\n" + targetName + krtext.Particle(targetName, '0') + " 완치술이 필요없습니다.\n")
		return false, nil
	}

	if err := setCreatureStat(world, target.creature.ID, "hpCurrent", targetMaxHP); err != nil {
		return false, err
	}

	if how == howCast {
		class := creatureClass(actor)
		if err := magicEffectDeductMPLegacy(world, actor, 50); err != nil {
			return false, err
		}
		if class >= model.ClassCaretaker {
			if err := magicEffectDeductMPLegacy(world, actor, 50); err != nil {
				return false, err
			}
		}
	}

	if how == howCast || how == howScroll || how == howWand {
		actorName := attackCreatureName(actor)
		targetName := attackCreatureName(target.creature)

		ctx.WriteString("\n당신은 " + targetName + "에게 완치부적을 먹이며 주문을 외웁니다.\n갑자기 그의 몸에서 심한 진동이 일어나면서 체력이 \n회복되는 것이 느껴집니다.")

		if target.hasPlayer {
			targetMsg := "\n" + actorName + krtext.Particle(actorName, '1') + " 당신에게 완치부적을 먹이며 주문을 외웁니다.\n갑자기 당신의 몸에서 심한 진동이 일어나면서 체력이 \n회복되는 것이 느껴집니다."
			_ = sendToPlayer(ctx, target.player.ID, targetMsg)
		}

		broadcastMsg := "\n" + actorName + krtext.Particle(actorName, '1') + " " + targetName + "에게 완치부적을 먹이며 주문을 외웁니다.\n그의 몸이 심한 진동이 일으키다가 체력이 회복었는지\n펄쩍펄쩍 뛰어 다닙니다.\n"
		if err := broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, broadcastMsg); err != nil {
			return false, err
		}
	}

	return true, nil
}

func magicEffectResolveFullHealTarget(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	target string,
	ordinal int64,
) (magicEffectTarget, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return magicEffectTarget{}, false
	}
	if target, ok := magicEffectResolveRoomPlayerTarget(ctx, world, actor, target, ordinal); ok {
		return target, true
	}

	viewer := LookViewer{
		PlayerID:   InventoryPlayerIDFromContext(ctx),
		CreatureID: actor.ID,
	}
	roomID := actor.RoomID
	if roomID.IsZero() && !viewer.PlayerID.IsZero() {
		if player, ok := world.Player(viewer.PlayerID); ok {
			roomID = player.RoomID
		}
	}
	room, ok := world.Room(roomID)
	if !ok {
		return magicEffectTarget{}, false
	}
	if creature, ok := findAttackCreatureTarget(world, room, viewer, legacyLowerFirstASCII(target), ordinal); ok {
		return magicEffectTarget{creature: creature}, true
	}
	return magicEffectTarget{}, false
}

func legacyLowerFirstASCII(text string) string {
	if text == "" {
		return ""
	}
	if text[0] >= 'A' && text[0] <= 'Z' {
		return string(text[0]+('a'-'A')) + text[1:]
	}
	return text
}

func decrementCaretakerFullHealCur(world StatusWorld, caster model.Creature) error {
	updater, ok := world.(interface {
		SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
	})
	if !ok {
		return nil
	}
	dailyCur := 0
	if valStr, ok := caster.Properties["dailyFullHealCur"]; ok {
		if val, err := strconv.Atoi(valStr); err == nil {
			dailyCur = val
		}
	}
	_, err := updater.SetCreatureProperty(caster.ID, "dailyFullHealCur", strconv.Itoa(dailyCur-1))
	return err
}

func decrementDailyFullHealLimit(world StatusWorld, caster model.Creature) (bool, error) {
	level := getCreatureLevel(caster)
	maxUses := 10 + (((level+3)/4)-5)/3
	if maxUses < 10 {
		maxUses = 10
	}

	dailyMax := maxUses
	if creatureClass(caster) >= model.ClassCaretaker {
		dailyMax = 0
	}
	if valStr, ok := caster.Properties["dailyFullHealMax"]; ok {
		if val, err := strconv.Atoi(valStr); err == nil {
			dailyMax = val
		}
	}

	now := timeNow().In(seoulLocation())
	lastTime := now
	if ltimeStr, ok := caster.Properties["dailyFullHealLTime"]; ok {
		if ltimeVal, err := strconv.ParseInt(ltimeStr, 10, 64); err == nil {
			lastTime = time.Unix(ltimeVal, 0).In(seoulLocation())
		}
	}

	dailyCur := dailyMax
	if valStr, ok := caster.Properties["dailyFullHealCur"]; ok {
		if val, err := strconv.Atoi(valStr); err == nil {
			dailyCur = val
		}
	}
	if now.Year() != lastTime.Year() || now.YearDay() != lastTime.YearDay() {
		dailyCur = dailyMax
	}
	if dailyCur <= 0 {
		return false, nil
	}
	dailyCur--

	updater, ok := world.(interface {
		SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
	})
	if !ok {
		return true, nil
	}
	if _, err := updater.SetCreatureProperty(caster.ID, "dailyFullHealMax", strconv.Itoa(dailyMax)); err != nil {
		return false, err
	}
	if _, err := updater.SetCreatureProperty(caster.ID, "dailyFullHealCur", strconv.Itoa(dailyCur)); err != nil {
		return false, err
	}
	if _, err := updater.SetCreatureProperty(caster.ID, "dailyFullHealLTime", strconv.FormatInt(now.Unix(), 10)); err != nil {
		return false, err
	}
	return true, nil
}

func seoulLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		return time.UTC
	}
	return loc
}

func magicEffectRoomVigor(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)

	if how == howPotion {
		ctx.WriteString("주문이 실패했습니다.")
		return false, nil
	}

	if !creatureHasAnyFlag(actor, "SRVIGO", "roomVigorSpell") {
		ctx.WriteString("당신은 아직 그런 주문을 터득하지 못했습니다.")
		return false, nil
	}

	class := creatureClass(actor)
	if class != model.ClassCleric && class < model.ClassInvincible {
		ctx.WriteString("이 주술은 불제자만이 사용할 수 있습니다.")
		return false, nil
	}

	if class >= model.ClassInvincible && !creatureHasAnyFlag(actor, "SCLERIC", "clericTraining", "clericSpell", "clericMode") {
		ctx.WriteString("\n불제자를 무적수련하지 않았습니다..\n")
		return false, nil
	}

	if creatureStat(actor, "mpCurrent") < 12 {
		ctx.WriteString("당신의 도력이 부족합니다")
		return false, nil
	}

	if how == howCast {
		if err := magicEffectDeductMPLegacy(world, actor, 12); err != nil {
			return false, err
		}
	}

	if spellFail(actor) {
		return false, nil
	}

	ctx.WriteString("당신은 가부좌를 틀고서 전회복 주문을 외웁니다.\n방안에 눈이 뜰 수 없을 정도의 빛이 가득차다가 사라집니다.\n방안의 모든사람이 체력이 회복되었는 것을 느낄수 있습니다.\n")

	actorName := attackCreatureName(actor)
	broadcastMsg := "\n" + actorName + krtext.Particle(actorName, '1') + " 가부좌를 틀고서 전회복 주문을 외웁니다.\n방안에 눈이 뜰 수 없을 정도의 빛이 가득차다가 사라집니다.\n방안의 모든사람이 체력이 회복되었는 것을 느낄수 있습니다.\n"
	if err := roomBroadcast(ctx, actor.RoomID, broadcastMsg); err != nil {
		return false, err
	}

	heal := mrand(1, 6) + legacyStatBonus(creatureStat(actor, "piety"))

	if room, rOk := world.Room(actor.RoomID); rOk && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
		heal += mrand(1, 3)
		ctx.WriteString("\n이 방의 기운의 당신의 주술력을 강화시킵니다\n")
	}

	room, ok := world.Room(actor.RoomID)
	if !ok {
		return true, nil
	}

	for _, pid := range room.PlayerIDs {
		player, pOk := world.Player(pid)
		if !pOk {
			continue
		}
		creature, cOk := world.Creature(player.CreatureID)
		if !cOk {
			continue
		}

		if creature.Kind != model.CreatureKindMonster {
			if creature.ID != actor.ID {
				_ = sendToPlayer(ctx, player.ID, "당신의 몸에서도 회복의 기운이 솟아오름을 느낄 수 있습니다.\n")
			}

			currHP := creatureStat(creature, "hpCurrent")
			maxHP := creatureStat(creature, "hpMax")
			if maxHP < 1 {
				maxHP = 1
			}
			nextHP := minInt(maxHP, currHP+heal)
			_ = setCreatureStat(world, creature.ID, "hpCurrent", nextHP)
		}
	}

	return true, nil
}
