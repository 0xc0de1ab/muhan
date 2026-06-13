package command

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/0xc0de1ab/muhan/internal/engine/legacy"
	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

// ApplyMagicPowerEffectAgent2 dispatches Agent 2's spell effects.
// Returns (handled, success, error).
func ApplyMagicPowerEffectAgent2(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
	magicPower int,
) (bool, bool, error) {
	switch magicPower {
	case magicPowerDrainExp:
		success, err := magicEffectDrainExp(ctx, world, actor, object, resolved)
		return true, success, err
	case magicPowerCharm:
		success, err := magicEffectCharm(ctx, world, actor, object, resolved)
		return true, success, err
	case magicPowerRmGong:
		success, err := magicEffectRmGong(ctx, world, actor, object, resolved)
		return true, success, err
	default:
		return false, false, nil
	}
}

func magicEffectDrainExp(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast && !creatureHasAnyFlag(actor, "SDREXP", "drainExpSpell") {
		ctx.WriteString("\n당신은 아직 그런 주문을 터득하지 못했습니다.\n")
		return false, nil
	}
	if how == howCast && creatureClass(actor) < model.ClassDM {
		ctx.WriteString("\n그런 주문을 외울수 없습니다.\n")
		return false, nil
	}
	if how == howScroll {
		ctx.WriteString("\n그런 주문을 외울수 없습니다.\n")
		return false, nil
	}

	isSelf := false
	targetArg := getArg(resolved, 1)
	var target magicEffectTarget
	var ok bool
	if targetArg == "" {
		isSelf = true
		target, ok = magicEffectSelfTarget(ctx, world, actor)
	} else {
		if how == howPotion {
			ctx.WriteString("\n이 물건은 자신에게만 사용할수 있습니다.\n")
			return false, nil
		}
		target, ok = magicEffectResolveRoomPlayerThenMonsterTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}

	if !ok || target.creature.ID.IsZero() {
		if targetArg != "" {
			ctx.WriteString("\n그런 사람이 존재하지 않습니다 .\n")
		}
		return false, nil
	}

	level := magicCreatureLevel(actor)

	var loss int
	if isSelf {
		if how == howPotion || how == howWand {
			loss = rollDice((level+3)/4, (level+3)/4, ((level+3)/4)*10)
		} else { // Cast
			loss = rollDice((level+3)/4, (level+3)/4, 1) * 10
		}

		exp := creatureStat(target.creature, "experience")
		if loss > exp {
			loss = exp
		}
		newExp := exp - loss
		if newExp < 0 {
			newExp = 0
		}

		if updater, ok := world.(magicCreatureStatWorld); ok {
			_ = updater.SetCreatureStat(target.creature.ID, "experience", newExp)
			reduceWeaponProficiency(updater, target, loss)
		}

		// Self Message
		ctx.WriteString(fmt.Sprintf("\n당신은 갑자기 멍청해 지면서 지금까지 싸워왔던 경험들이\n생각나지 않습니다.\n!!악~~~ 경험치가 얼마인지도 모르겠다.!!\n\n당신은 %d만큼의 경험들이 생각나지 않습니다.\n", loss))

		// Broadcast Message
		if how == howCast || how == howWand {
			actorName := attackCreatureName(actor)
			_ = roomBroadcast(ctx, actor.RoomID, fmt.Sprintf("\n%s%s 자신에게 백치술 주문을 외웁니다.\n그는 갑자기 멍청해진듯 주위를 두리번 거립니다.\n", actorName, krtext.Particle(actorName, '1')))
		}

	} else {
		if how == howPotion {
			ctx.WriteString("\n이 물건은 자신에게만 사용할수 있습니다.\n")
			return false, nil
		}

		if how == howWand {
			loss = objectDamage(world, object)
		} else {
			loss = rollDice((level+3)/4, (level+3)/4, 1) * 30
		}
		exp := creatureStat(target.creature, "experience")
		if loss > exp {
			loss = exp
		}
		newExp := exp - loss
		if newExp < 0 {
			newExp = 0
		}

		if updater, ok := world.(magicCreatureStatWorld); ok {
			_ = updater.SetCreatureStat(target.creature.ID, "experience", newExp)
			reduceWeaponProficiency(updater, target, loss)
		}

		casterName := attackCreatureName(actor)
		targetName := attackCreatureName(target.creature)

		// Self Message (Caster output)
		ctx.WriteString(fmt.Sprintf("\n당신은 %s에게 백치술의 주문을 외웁니다.\n그는 갑자기 멍청해진듯 주위를 두리번 거립니다.\n", targetName))

		// Broadcast Message
		_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, fmt.Sprintf("\n%s%s %s에게 백치술의 주문을 외웁니다.\n그는 갑자기 멍청해진듯 주위를 두리번 거립니다.\n", casterName, krtext.Particle(casterName, '1'), targetName))

		// Target Message
		if target.hasPlayer {
			targetMsg := fmt.Sprintf("\n%s%s 당신에게 백치술의 주문을 외웁니다.\n당신은 갑자기 멍청해지면서 지금까지 싸워왔던 경험들이\n생각나지 않습니다.\n!!악~~~ 경험치가 얼마인지도 모르겠다.!!\n\n당신은 %d만큼의 경험들이 생각나지 않습니다.\n", casterName, krtext.Particle(casterName, '1'), loss)
			_ = sendToPlayer(ctx, target.player.ID, targetMsg)
		}

		// Caster output about target
		ctx.WriteString(fmt.Sprintf("\n%s%s 싸웠던 얼마간의 경험들을 잊어버린것\n같습니다.\n", targetName, krtext.Particle(targetName, '1')))
	}

	return true, nil
}

func magicEffectResolveRoomPlayerThenMonsterTarget(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	target string,
	ordinal int64,
) (magicEffectTarget, bool) {
	target = strings.TrimSpace(target)
	if target == "" || target == "나" {
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
	if creature, ok := findLegacyMonsterTarget(world, room, viewer, legacyLowerFirstASCII(target), ordinal); ok {
		return magicEffectTarget{creature: creature}, true
	}
	return magicEffectTarget{}, false
}

func magicEffectCharm(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	room, roomOK := world.Room(actor.RoomID)
	if roomOK && roomHasAnyFlag(room, "survival", "rsuviv", "RSUVIV") {
		ctx.WriteString("대련장에서는 이 주문을 사용할 수 없습니다.")
		return false, nil
	}

	how := determineHow(world, object)
	isSelf := false
	targetArg := getArg(resolved, 1)
	var target magicEffectTarget
	var ok bool
	if targetArg == "" {
		isSelf = true
		target, ok = magicEffectSelfTarget(ctx, world, actor)
	} else {
		if how == howPotion {
			ctx.WriteString("그 물건은 자신에게만 사용할수 있습니다.")
			return false, nil
		}
		target, ok = magicEffectResolveRoomPlayerThenMonsterTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}

	if !ok || target.creature.ID.IsZero() {
		ctx.WriteString("그런 사람이 존재하지 않습니다.")
		return false, nil
	}

	// Calculate duration
	var dur int
	intelBonus := legacyStatBonus(creatureStat(actor, "intelligence"))
	if how == howCast {
		dur = 100 + attackRoll(1, 30)*5 + intelBonus*20
	} else if how == howScroll {
		dur = 50 + attackRoll(1, 15)*5 + intelBonus*20
	} else { // Wand or potion
		dur = 50 + attackRoll(1, 15)*5
	}

	if isSelf {
		if how == howCast {
			if err := magicEffectDeductMPLegacy(world, actor, 15); err != nil {
				return false, err
			}
		}
		if spellFail(actor) {
			return false, nil
		}
		if updater, ok := world.(interface {
			SetCreatureCooldown(model.CreatureID, string, int64, int64) error
		}); ok {
			_ = updater.SetCreatureCooldown(target.creature.ID, "charmed", time.Now().Unix(), int64(dur))
		}

		if how == howPotion {
			ctx.WriteString("기분이 좋아지면서 괜히 맞아도 황홀한 기분이\n듭니다. 나 좀 때려줘..")
			return true, nil
		}

		ctx.WriteString("당신은 심심해서 거울을 보며 이혼대법을 사용합니다.\n기분이 좋아지면서 괜히 맞아도 황홀한 기분이\n듭니다. 나 좀 때려줘..")

		actorName := attackCreatureName(actor)
		_ = roomBroadcast(ctx, actor.RoomID, fmt.Sprintf("\n%s%s 이혼대법의 주술을 거는 거울을 봅니다.\n거울을 보고나자 당신을 보면서 괜히 히죽히죽\n거립니다. 이 자식 미쳤나?", actorName, krtext.Particle(actorName, '1')))

	} else {
		if how == howPotion {
			ctx.WriteString("그 물건은 자신에게만 사용할수 있습니다.")
			return false, nil
		}
		if how == howCast {
			if err := magicEffectDeductMPLegacy(world, actor, 15); err != nil {
				return false, err
			}
		}

		if creatureHasAnyFlag(target.creature, "resistMagic", "PRMAGI", "MRMAGI") {
			dur = dur / 2
		}

		casterLevel := magicCreatureLevel(actor)
		targetLevel := magicCreatureLevel(target.creature)
		if casterLevel < targetLevel || creatureHasAnyFlag(target.creature, "MNOCHA") {
			ctx.WriteString("상대의 기가 당신의 주문을 반탄시킵니다.")
			if target.hasPlayer {
				_ = sendToPlayer(ctx, target.player.ID, fmt.Sprintf("%s%s 당신에게 이혼대법을 걸으려 합니다.\n", attackCreatureName(actor), krtext.Particle(attackCreatureName(actor), '1')))
			}
			_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, fmt.Sprintf("%s%s 이혼대법을 %s에게 걸려고 합니다.\n", attackCreatureName(actor), krtext.Particle(attackCreatureName(actor), '1'), attackCreatureName(target.creature)))
			return false, nil
		}

		casterName := attackCreatureName(actor)
		targetName := attackCreatureName(target.creature)

		var charmTags []string
		if target.hasPlayer {
			charmTags = []string{"charmed", "PCHARM"}
		} else {
			charmTags = []string{"charmed", "MCHARM"}
		}

		_ = magicEffectUpdateTags(world, target, charmTags, nil)
		if creatureClass(target.creature) < model.ClassDM {
			_ = magicEffectAddCharmListEntry(world, actor.ID, target.creature)
		}

		if updater, ok := world.(interface {
			SetCreatureCooldown(model.CreatureID, string, int64, int64) error
		}); ok {
			_ = updater.SetCreatureCooldown(target.creature.ID, "charmed", time.Now().Unix(), int64(dur))
		}

		// Self Message
		ctx.WriteString(fmt.Sprintf("당신은 %s에게 거울을 비추며 이혼대법을 겁니다.\n거울을 보고나자 당신을 보면서 괜히 히죽히죽\n거립니다. 드디어 맛 갔군.", targetName))

		// Broadcast Message
		_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, fmt.Sprintf("\n%s%s %s에게 거울을 비추며 이혼대법을 겁니다.\n거울을 보고나자 당신을 보면서 괜히 히죽히죽\n거립니다. 저 자식이 미쳤나?", casterName, krtext.Particle(casterName, '1'), targetName))

		// Target Message
		if target.hasPlayer {
			targetMsg := fmt.Sprintf("\n%s%s 당신에게 거울을 비추며 이혼대법을 겁니다.\n괜히 기분이 좋아지면서 맞아도 황홀한 기분이\n듭니다. 나 좀 때려줘..\n", casterName, krtext.Particle(casterName, '1'))
			_ = sendToPlayer(ctx, target.player.ID, targetMsg)
		}
	}

	return true, nil
}

func magicEffectAddCharmListEntry(world StatusWorld, actorID model.CreatureID, target model.Creature) error {
	targetName := strings.TrimSpace(target.DisplayName)
	if targetName == "" {
		targetName = string(target.ID)
	}
	tags := []string{"charm:" + targetName}
	if !target.ID.IsZero() {
		tags = append(tags, "charmID:"+string(target.ID))
	}
	updater, ok := world.(interface {
		UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	})
	if !ok {
		return nil
	}
	_, err := updater.UpdateCreatureTags(actorID, tags, nil)
	return err
}

func magicEffectConsumeActorMP(world StatusWorld, actor model.Creature, cost int) error {
	updater, ok := world.(magicCreatureStatWorld)
	if !ok || cost <= 0 {
		return nil
	}
	next := creatureStat(actor, "mpCurrent") - cost
	if next < 0 {
		next = 0
	}
	return updater.SetCreatureStat(actor.ID, "mpCurrent", next)
}

func magicEffectRmGong(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if creatureClass(actor) < model.ClassBulsa {
		ctx.WriteString("아직 당신의 능력으로는 외울수 없는 주문입니다.")
		return false, nil
	}
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 100 {
			ctx.WriteString("당신의 도력이 부족합니다")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SRMGONG", "rmGongSpell") {
			ctx.WriteString("당신은 아직 그런 주문을 터득하지 못했습니다.")
			return false, nil
		}
	}
	if failed, err := magicEffectSpellFail(world, actor, how, 100); failed || err != nil {
		return false, err
	}

	isPotion := how == howPotion
	isSelf := false
	targetArg := getArg(resolved, 1)
	var target magicEffectTarget
	var ok bool
	if targetArg == "" {
		isSelf = true
		target, ok = magicEffectSelfTarget(ctx, world, actor)
	} else {
		if isPotion {
			ctx.WriteString("이 물건은 자신에게만 사용할수 있습니다.")
			return false, nil
		}
		target, ok = magicEffectResolveRoomPlayerThenMonsterTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}

	if !ok || target.creature.ID.IsZero() {
		ctx.WriteString("그런 사람이 존재하지 않습니다.")
		return false, nil
	}
	if isPotion && !isSelf {
		ctx.WriteString("이 물건은 자신에게만 사용할수 있습니다.")
		return false, nil
	}

	wasFearful := creatureHasAnyFlag(target.creature, "fear", "fearful", "PFEARS", "MFEARS")
	_ = magicEffectUpdateTags(world, target, nil, []string{"fear", "fearful", "PFEARS", "MFEARS"})

	actorName := attackCreatureName(actor)

	if isSelf {
		if how == howCast {
			if err := magicEffectDeductMPLegacy(world, actor, 100); err != nil {
				return false, err
			}
		}
		if isPotion {
			if wasFearful {
				ctx.WriteString("새하얗던 얼굴에 핏기가 돌기 시작합니다.")
			} else {
				ctx.WriteString("아무 반응이 없습니다.")
			}
			return true, nil
		}
		ctx.WriteString("당신이 공포해소 주문을 외우자 주위에 있던 공포가 사라집니다.")
		_ = roomBroadcast(ctx, actor.RoomID, fmt.Sprintf("\n%s%s 공포해소 주문을 위우자 그의 주위에 있던 공포가 사라집니다.", actorName, krtext.Particle(actorName, '1')))
	} else {
		if how == howCast {
			if err := magicEffectDeductMPLegacy(world, actor, 100); err != nil {
				return false, err
			}
		}
		targetName := attackCreatureName(target.creature)

		// Self Message
		ctx.WriteString(fmt.Sprintf("당신은 %s의 회복을 기원하며 공포해소 주문을 외우자\n그의 공포가 사라짐을 느낍니다.\n", targetName))

		// Broadcast Message
		_ = roomBroadcast(ctx, actor.RoomID, fmt.Sprintf("\n%s%s %s의 회복을 기원하며 공포해소 주문을 외우자\n%s의 공포가 사라짐을 느낍니다.\n", actorName, krtext.Particle(actorName, '1'), targetName, targetName))

		// Target Message
		if target.hasPlayer {
			targetMsg := fmt.Sprintf("%s%s 당신에게 공포해소 주문을 외우자 당신의 겁이 사라집니다.\n", actorName, krtext.Particle(actorName, '1'))
			_ = sendToPlayer(ctx, target.player.ID, targetMsg)
		}
	}

	return true, nil
}

func reduceWeaponProficiency(world magicCreatureStatWorld, target magicEffectTarget, loss int) {
	proficiency, realms := legacyLowerProficiencyValues(target.creature)
	proficiency, realms = legacy.LowerProficiency(proficiency, realms, loss)
	writeLegacyLowerProficiencyValues(world, target.creature, proficiency, realms)
}

type magicCreaturePropertyWorld interface {
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
}

var legacyLowerProfWeaponKeys = [5][]string{
	{"proficiencySharp", "proficiency/sharp", "proficiency.sharp", "proficiency_sharp", "proficiency/0", "proficiency.0", "proficiency_0", "proficiency0"},
	{"proficiencyThrust", "proficiency/thrust", "proficiency.thrust", "proficiency_thrust", "proficiency/1", "proficiency.1", "proficiency_1", "proficiency1"},
	{"proficiencyBlunt", "proficiency/blunt", "proficiency.blunt", "proficiency_blunt", "proficiency/2", "proficiency.2", "proficiency_2", "proficiency2"},
	{"proficiencyPole", "proficiency/pole", "proficiency.pole", "proficiency_pole", "proficiency/3", "proficiency.3", "proficiency_3", "proficiency3"},
	{"proficiencyMissile", "proficiency/missile", "proficiency.missile", "proficiency_missile", "proficiency/4", "proficiency.4", "proficiency_4", "proficiency4"},
}

var legacyLowerProfRealmKeys = [4][]string{
	{"realmEarth", "realm/1", "realm.1", "realm_1", "realm1"},
	{"realmWind", "realm/2", "realm.2", "realm_2", "realm2"},
	{"realmFire", "realm/3", "realm.3", "realm_3", "realm3"},
	{"realmWater", "realm/4", "realm.4", "realm_4", "realm4"},
}

func legacyLowerProficiencyValues(creature model.Creature) ([5]int, [4]int) {
	var proficiency [5]int
	var realms [4]int
	for i, keys := range legacyLowerProfWeaponKeys {
		proficiency[i] = legacyCreatureIntValue(creature, keys)
	}
	for i, keys := range legacyLowerProfRealmKeys {
		realms[i] = legacyCreatureIntValue(creature, keys)
	}
	return proficiency, realms
}

func legacyCreatureIntValue(creature model.Creature, keys []string) int {
	for _, key := range keys {
		if val, ok := creature.Stats[key]; ok {
			return val
		}
		if valStr, ok := creature.Properties[key]; ok {
			if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
				return val
			}
		}
	}
	return 0
}

func writeLegacyLowerProficiencyValues(world magicCreatureStatWorld, creature model.Creature, proficiency [5]int, realms [4]int) {
	propWorld, _ := world.(magicCreaturePropertyWorld)
	for i, keys := range legacyLowerProfWeaponKeys {
		writeLegacyLowerProficiencySlot(world, propWorld, creature, keys, keys[0], proficiency[i])
	}
	for i, keys := range legacyLowerProfRealmKeys {
		writeLegacyLowerProficiencySlot(world, propWorld, creature, keys, keys[0], realms[i])
	}
}

func writeLegacyLowerProficiencySlot(world magicCreatureStatWorld, propWorld magicCreaturePropertyWorld, creature model.Creature, keys []string, defaultStatKey string, value int) {
	wrote := false
	for _, key := range keys {
		if _, ok := creature.Stats[key]; ok {
			_ = world.SetCreatureStat(creature.ID, key, value)
			wrote = true
		}
	}
	if propWorld != nil {
		for _, key := range keys {
			if _, ok := creature.Properties[key]; ok {
				_, _ = propWorld.SetCreatureProperty(creature.ID, key, strconv.Itoa(value))
				wrote = true
			}
		}
	}
	if !wrote && value != 0 {
		_ = world.SetCreatureStat(creature.ID, defaultStatKey, value)
	}
}
