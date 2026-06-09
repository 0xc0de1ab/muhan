package command

import (
	"fmt"
	"reflect"
	"strconv"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

// magicEffectEnchant implements Enchantment (power 16)
func magicEffectEnchant(ctx *Context, world StatusWorld, actor model.Creature, sourceObject model.ObjectInstance, resolved ResolvedCommand) (bool, error) {
	how := determineHow(world, sourceObject)
	class := creatureStat(actor, "class")
	if how == howCast {
		if class != legacyClassMage && class < legacyClassInvincible {
			ctx.WriteString("\n도술사들만이 주술을 걸수있습니다.\n")
			return false, nil
		}
		if class >= legacyClassInvincible && !attackCreatureHasFlag(actor, "SMAGE", "smage") {
			ctx.WriteString("\n도술사를 무적수련하지 않았습니다..\n")
			return false, nil
		}
		if creatureStat(actor, "mpCurrent") < 25 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SENCHA", "sencha") {
			ctx.WriteString("\n당신은 아직 그런 주술을 터득하지 못했습니다.\n")
			return false, nil
		}
	}

	targetName := getArg(resolved, 1)
	if targetName == "" {
		ctx.WriteString("\n무엇에다 주술을 겁니다?\n")
		return false, nil
	}

	object, objectName, ok := findEquipInventoryObjectWithVisibility(world, actor, targetName, getOrdinal(resolved, 1), viewerHasDetectInvisibleCreatureTag(actor))
	if !ok {
		ctx.WriteString("\n당신 소지품에 그런 물건이 없습니다.\n")
		return false, nil
	}

	if objectHasAnyTag(world, object, "enchanted", "oencha") ||
		objectHasAnyPropertyFlag(world, object, "enchanted", "oencha", "OENCHA") {
		ctx.WriteString("\n벌써 주술이 걸려있습니다.\n")
		return true, nil
	}

	if how == howCast {
		available, err := decrementDailyEnchantLimit(world, actor)
		if err != nil {
			return false, err
		}
		if !available && class < legacyClassCaretaker {
			ctx.WriteString("\n당신은 탈진해서 더 이상 주술을 걸수 없습니다.\n좀 쉬어야겠는데요?\n")
			return false, nil
		}
		if err := setCreatureStat(world, actor.ID, "mpCurrent", maxInt(0, creatureStat(actor, "mpCurrent")-25)); err != nil {
			return false, err
		}
	}

	var adj int
	level := creatureStat(actor, "level")
	if how == howCast && (class == legacyClassMage || class >= legacyClassInvincible) {
		adj = (((level+3)/4)-5)/5 + 1
		if adj > 3 {
			adj = 3
		}
	} else {
		adj = 2
	}

	if attackCreatureHasFlag(actor, "YELLOWI", "yellowI") {
		adj = 4
	}
	if class >= legacyClassBulsa {
		adj = 5
	}

	objWorld, ok := world.(interface {
		UpdateObjectTags(model.ObjectInstanceID, []string, []string) (model.ObjectInstance, error)
		SetObjectProperty(model.ObjectInstanceID, string, string) (model.ObjectInstance, error)
	})
	if !ok {
		return false, fmt.Errorf("world does not support object modification")
	}

	// Set object adjustment = max(adj, current_adjustment).
	currentAdj := objectIntPropertyOrZero(world, object, "adjustment")
	newAdj := adj
	if currentAdj > newAdj {
		newAdj = currentAdj
	}
	if _, err := objWorld.SetObjectProperty(object.ID, "adjustment", strconv.Itoa(newAdj)); err != nil {
		return false, err
	}

	// Apply adjustment to object properties:
	if isArmor(world, object) {
		wearFlag := objectWearFlag(world, object)
		armorInc := adj
		if wearFlag == legacyWearBody { // legacyWearBody is 1
			armorInc = adj * 2
		}
		currentArmor := objectIntPropertyOrZero(world, object, "armor")
		if _, err := objWorld.SetObjectProperty(object.ID, "armor", strconv.Itoa(currentArmor+armorInc)); err != nil {
			return false, err
		}
	} else if isWeapon(world, object) {
		currentShotsMax := objectIntPropertyOrZero(world, object, "shotsMax")
		currentShotsCur := objectIntPropertyOrZero(world, object, "shotsCurrent")
		currentPDice := objectIntPropertyOrZero(world, object, "pDice")

		if _, err := objWorld.SetObjectProperty(object.ID, "shotsMax", strconv.Itoa(currentShotsMax+adj*10)); err != nil {
			return false, err
		}
		if _, err := objWorld.SetObjectProperty(object.ID, "shotsCurrent", strconv.Itoa(currentShotsCur+adj*10)); err != nil {
			return false, err
		}
		if _, err := objWorld.SetObjectProperty(object.ID, "pDice", strconv.Itoa(currentPDice+adj)); err != nil {
			return false, err
		}
	}

	// Increase object value by 500 * adj.
	currentVal := objectIntPropertyOrZero(world, object, "value")
	if _, err := objWorld.SetObjectProperty(object.ID, "value", strconv.Itoa(currentVal+500*adj)); err != nil {
		return false, err
	}

	if _, err := objWorld.UpdateObjectTags(object.ID, []string{"enchanted", "oencha", "OENCHA"}, nil); err != nil {
		return false, err
	}

	actorName := attackCreatureName(actor)
	ctx.WriteString(fmt.Sprintf("\n%s에다가 주술을 걸자 갑자기 밝은 빛을 내다가 사라졌습니다.\n", objectName))

	roomBroadcast(ctx, actor.RoomID, actorName+krtext.Particle(actorName, '1')+" "+objectName+"에다가 주술을 걸었습니다.")

	return true, nil
}

func decrementDailyEnchantLimit(world StatusWorld, caster model.Creature) (bool, error) {
	dailyMax := 10
	if valStr, ok := caster.Properties["dailyEnchantMax"]; ok {
		if val, err := strconv.Atoi(valStr); err == nil {
			dailyMax = val
		}
	}

	now := timeNow()
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.UTC
	}
	now = now.In(loc)

	lastTime := now
	if ltimeStr, ok := caster.Properties["dailyEnchantLTime"]; ok {
		if ltimeVal, err := strconv.ParseInt(ltimeStr, 10, 64); err == nil {
			lastTime = time.Unix(ltimeVal, 0).In(loc)
		}
	}

	dailyCur := dailyMax
	if valStr, ok := caster.Properties["dailyEnchantCur"]; ok {
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
	if _, err := updater.SetCreatureProperty(caster.ID, "dailyEnchantMax", strconv.Itoa(dailyMax)); err != nil {
		return false, err
	}
	if _, err := updater.SetCreatureProperty(caster.ID, "dailyEnchantCur", strconv.Itoa(dailyCur)); err != nil {
		return false, err
	}
	if _, err := updater.SetCreatureProperty(caster.ID, "dailyEnchantLTime", strconv.FormatInt(now.Unix(), 10)); err != nil {
		return false, err
	}
	return true, nil
}

// magicEffectRemoveCurse implements Remove Curse (power 43)
func magicEffectRemoveCurse(ctx *Context, world StatusWorld, actor model.Creature, object model.ObjectInstance, resolved ResolvedCommand) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 18 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SREMOV", "sremov") {
			ctx.WriteString("\n당신은 아직 그런 주문을 터득하지 못했습니다.\n")
			return false, nil
		}
	}
	if spellFail(actor) {
		if how == howCast {
			if err := magicEffectConsumeMP(world, actor, 18); err != nil {
				return false, err
			}
		}
		return false, nil
	}

	// Cast remove-curse on self or target
	targetName := getArg(resolved, 1)
	isSelf := targetName == ""

	objWorld, ok := world.(interface {
		UpdateObjectTags(model.ObjectInstanceID, []string, []string) (model.ObjectInstance, error)
	})
	if !ok {
		return false, fmt.Errorf("world does not support object tag modification")
	}

	actorName := attackCreatureName(actor)

	if isSelf {
		if how == howCast {
			if err := magicEffectDeductMPLegacy(world, actor, 18); err != nil {
				return false, err
			}
		}

		if how == howPotion {
			ctx.WriteString("\n물건안에 담겨있던 성스러운 기운이 당신의 \n저주를 풀어줍니다.\n")
		} else {
			ctx.WriteString("\n당신은 오른손에 성스러운 기운을 모으자 붉은\n빛이 퍼져나갑니다.\n당신의 몸에 걸렸던 저주가 풀리기 시작합니다.\n")
			broadcastMsg := "\n" + actorName + krtext.Particle(actorName, '1') + " 오른손에 성스러운 기운을 모으자 붉은\n빛이 퍼져나갑니다.\n성스러운 기운이 그의 몸에 걸렸던 저주가 푸는것이 느껴집니다.\n "
			roomBroadcast(ctx, actor.RoomID, broadcastMsg)
		}

		for _, objectID := range actor.Equipment {
			if objectID.IsZero() {
				continue
			}
			if _, err := objWorld.UpdateObjectTags(objectID, nil, []string{"cursed", "ocurse"}); err != nil {
				return false, err
			}
			if err := magicEffectClearCurseProperties(world, objectID); err != nil {
				return false, err
			}
		}
		return true, nil
	}

	// Cast remove-curse on another player/creature
	if how == howPotion {
		ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	target, ok := magicEffectResolveRoomPlayerTarget(ctx, world, actor, targetName, getOrdinal(resolved, 1))
	if !ok || !target.hasPlayer {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다.\n")
		return false, nil
	}

	if how == howCast {
		if err := magicEffectDeductMPLegacy(world, actor, 18); err != nil {
			return false, err
		}
	}

	targetDispName := attackCreatureName(target.creature)

	// Send message to caster
	ctx.WriteString(fmt.Sprintf("\n손을 통해 %s의 몸에 성스러운 기운을\n주입합니다.\n그의 몸에서 저주가 물러가는 것이 느껴집니다.\n", targetDispName))

	// Send message to target if they are a player
	if target.hasPlayer && !target.player.ID.IsZero() {
		targetMsg := fmt.Sprintf("\n%s%s 당신의 몸에 손을 통해 성스러운 기운을\n주입합니다.\n당신의 몸에서 저주가 물러가는 것이 느껴집니다.\n", actorName, krtext.Particle(actorName, '1'))
		sendToPlayerAgent3(ctx, target.player.ID, targetMsg)
	}

	// Room broadcast excluding caster and target
	broadcastMsg := fmt.Sprintf("\n%s%s %s의 등에 손을 대고 성스러운 \n기운을 주입합니다.\n그의 몸에서 느껴졌던 저주의 기운이 사라지는 것을\n느낄수 있습니다.\n", actorName, krtext.Particle(actorName, '1'), targetDispName)
	var targetPlayerID model.PlayerID
	if target.hasPlayer {
		targetPlayerID = target.player.ID
	}
	roomBroadcast2(ctx, world, actor.RoomID, ctx.SessionID, targetPlayerID, broadcastMsg)

	for _, objectID := range target.creature.Equipment {
		if objectID.IsZero() {
			continue
		}
		if _, err := objWorld.UpdateObjectTags(objectID, nil, []string{"cursed", "ocurse"}); err != nil {
			return false, err
		}
		if err := magicEffectClearCurseProperties(world, objectID); err != nil {
			return false, err
		}
	}

	return true, nil
}

func magicEffectClearCurseProperties(world StatusWorld, objectID model.ObjectInstanceID) error {
	setter, ok := world.(interface {
		SetObjectProperty(model.ObjectInstanceID, string, string) (model.ObjectInstance, error)
	})
	if !ok {
		return nil
	}
	for _, key := range []string{"cursed", "ocurse", "OCURSE"} {
		if _, err := setter.SetObjectProperty(objectID, key, ""); err != nil {
			return err
		}
	}
	return nil
}

// magicEffectCurse implements Curse (power 57)
func magicEffectCurse(ctx *Context, world StatusWorld, actor model.Creature, object model.ObjectInstance, resolved ResolvedCommand) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 25 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SCURSE", "scurse") {
			ctx.WriteString("\n당신은 아직 그런 주문을 터득하지 못했습니다.\n")
			return false, nil
		}
	}
	if spellFail(actor) {
		if how == howCast {
			if err := magicEffectConsumeMP(world, actor, 25); err != nil {
				return false, err
			}
		}
		return false, nil
	}

	targetName := getArg(resolved, 1)
	isSelf := targetName == ""

	objWorld, ok := world.(interface {
		UpdateObjectTags(model.ObjectInstanceID, []string, []string) (model.ObjectInstance, error)
	})
	if !ok {
		return false, fmt.Errorf("world does not support object tag modification")
	}

	actorName := attackCreatureName(actor)

	if isSelf {
		if how == howCast {
			if err := magicEffectDeductMPLegacy(world, actor, 25); err != nil {
				return false, err
			}
		}

		if how == howPotion {
			ctx.WriteString("\n물건안에 담겨있던 .\n")
		} else {
			ctx.WriteString("\n당신이 오른손에 기운을 모우자 붉은 기운이 몰려와\n감당할수 없는 힘이 느껴집니다. \n\n오홋~~ 손이 펴지질 않아.. 당신은 무기를 벗을수 없습니다. \n")
			broadcastMsg := "\n" + actorName + krtext.Particle(actorName, '1') + " 저주 주문을 외웁니다.\n "
			roomBroadcast(ctx, actor.RoomID, broadcastMsg)
		}

		for _, objectID := range actor.Equipment {
			if objectID.IsZero() {
				continue
			}
			if _, err := objWorld.UpdateObjectTags(objectID, []string{"cursed", "ocurse"}, nil); err != nil {
				return false, err
			}
		}
		return true, nil
	}

	// Cast curse on another player/creature
	if how == howPotion {
		ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	target, ok := magicEffectResolveRoomPlayerTarget(ctx, world, actor, targetName, getOrdinal(resolved, 1))
	if !ok || !target.hasPlayer {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다.\n")
		return false, nil
	}

	if how == howCast {
		if err := magicEffectDeductMPLegacy(world, actor, 25); err != nil {
			return false, err
		}
	}

	targetDispName := attackCreatureName(target.creature)

	// Send message to caster
	ctx.WriteString(fmt.Sprintf("\n손을 통해 %s의 몸에 저주의 기운을 불어넣습니다.\n", targetDispName))

	// Send message to target if they are a player
	if target.hasPlayer && !target.player.ID.IsZero() {
		targetMsg := fmt.Sprintf("\n%s%s 당신의 몸에 손을 통해 저주의 기운을 불어 넣습니다.\n", actorName, krtext.Particle(actorName, '1'))
		sendToPlayerAgent3(ctx, target.player.ID, targetMsg)
	}

	// Room broadcast excluding caster and target
	broadcastMsg := fmt.Sprintf("\n%s%s %s의 등에 손을 대고 저주의 기운을 불어 넣습니다.\n", actorName, krtext.Particle(actorName, '1'), targetDispName)
	var targetPlayerID model.PlayerID
	if target.hasPlayer {
		targetPlayerID = target.player.ID
	}
	roomBroadcast2(ctx, world, actor.RoomID, ctx.SessionID, targetPlayerID, broadcastMsg)

	for _, objectID := range target.creature.Equipment {
		if objectID.IsZero() {
			continue
		}
		if _, err := objWorld.UpdateObjectTags(objectID, []string{"cursed", "ocurse"}, nil); err != nil {
			return false, err
		}
	}

	return true, nil
}

func magicEffectConsumeMP(world StatusWorld, actor model.Creature, cost int) error {
	mp := creatureStat(actor, "mpCurrent")
	next := mp - cost
	if next < 0 {
		next = 0
	}
	return setCreatureStat(world, actor.ID, "mpCurrent", next)
}

func magicEffectDeductMPLegacy(world StatusWorld, actor model.Creature, cost int) error {
	if cost <= 0 {
		return nil
	}
	mp := creatureStat(actor, "mpCurrent")
	if refreshed, ok := world.Creature(actor.ID); ok {
		mp = creatureStat(refreshed, "mpCurrent")
	}
	return setCreatureStat(world, actor.ID, "mpCurrent", mp-cost)
}

func isArmor(world InventoryWorld, object model.ObjectInstance) bool {
	legacyType := objectLegacyType(world, object)
	return legacyType == 5 || objectKindIs(world, object, model.ObjectKindArmor)
}

func isWeapon(world InventoryWorld, object model.ObjectInstance) bool {
	legacyType := objectLegacyType(world, object)
	return (legacyType >= 0 && legacyType <= 4) || objectKindIs(world, object, model.ObjectKindWeapon)
}

func sendToPlayerAgent3(ctx *Context, playerID model.PlayerID, text string) error {
	if ctx == nil || ctx.Values == nil || playerID.IsZero() || text == "" {
		return nil
	}
	activeSessionsVal := ctx.Values["game.activeSessions"]
	sendToSessionVal := ctx.Values["game.sendToSession"]
	if activeSessionsVal == nil || sendToSessionVal == nil {
		return nil
	}

	activeFn := reflect.ValueOf(activeSessionsVal)
	sendFn := reflect.ValueOf(sendToSessionVal)
	if activeFn.Kind() != reflect.Func || sendFn.Kind() != reflect.Func {
		return nil
	}

	out := activeFn.Call(nil)[0]
	if out.Kind() != reflect.Slice {
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
		actorIDVal := item.FieldByName("ActorID")
		idVal := item.FieldByName("ID")
		if !actorIDVal.IsValid() || !idVal.IsValid() {
			continue
		}
		if actorIDVal.String() == string(playerID) {
			sessionID := idVal.Interface()
			idValue := reflect.ValueOf(sessionID)
			commandType := sendFn.Type().In(1)
			commandValue := reflect.New(commandType).Elem()
			writeField := commandValue.FieldByName("Write")
			if writeField.IsValid() && writeField.CanSet() && writeField.Kind() == reflect.String {
				writeField.SetString(text)
			}
			sendFn.Call([]reflect.Value{idValue, commandValue})
			return nil
		}
	}
	return nil
}

func roomBroadcast2(ctx *Context, world StatusWorld, roomID model.RoomID, excludeCasterSession string, excludeTargetPlayerID model.PlayerID, text string) error {
	if ctx == nil || ctx.Values == nil || roomID.IsZero() || text == "" {
		return nil
	}
	activeSessionsVal := ctx.Values["game.activeSessions"]
	sendToSessionVal := ctx.Values["game.sendToSession"]
	if activeSessionsVal == nil || sendToSessionVal == nil {
		return nil
	}

	activeFn := reflect.ValueOf(activeSessionsVal)
	sendFn := reflect.ValueOf(sendToSessionVal)
	if activeFn.Kind() != reflect.Func || sendFn.Kind() != reflect.Func {
		return nil
	}

	out := activeFn.Call(nil)[0]
	if out.Kind() != reflect.Slice {
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
		idVal := item.FieldByName("ID")
		actorIDVal := item.FieldByName("ActorID")
		if !idVal.IsValid() || !actorIDVal.IsValid() {
			continue
		}

		sessionID := idVal.Interface()
		sessionIDStr := fmt.Sprint(sessionID)
		actorIDStr := actorIDVal.String()

		if sessionIDStr == excludeCasterSession {
			continue
		}
		if actorIDStr == string(excludeTargetPlayerID) {
			continue
		}
		if actorIDStr == "" {
			continue
		}

		playerID := model.PlayerID(actorIDStr)
		player, ok := world.Player(playerID)
		if !ok || player.RoomID != roomID {
			continue
		}

		idValue := reflect.ValueOf(sessionID)
		commandType := sendFn.Type().In(1)
		commandValue := reflect.New(commandType).Elem()
		writeField := commandValue.FieldByName("Write")
		if writeField.IsValid() && writeField.CanSet() && writeField.Kind() == reflect.String {
			writeField.SetString(text)
		}
		sendFn.Call([]reflect.Value{idValue, commandValue})
	}
	return nil
}
