package command

import (
	"fmt"
	"strings"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const (
	magicPowerVigor           = 1
	magicPowerHurt            = 2
	magicPowerLight           = 3
	magicPowerCurePoison      = 4
	magicPowerBless           = 5
	magicPowerProtection      = 6
	magicPowerFireball        = 7
	magicPowerInvisibility    = 8
	magicPowerDetectInvisible = 10
	magicPowerDetectMagic     = 11
	magicPowerBefuddle        = 13
	magicPowerLightning       = 14
	magicPowerIceBlade        = 15
	magicPowerRecall          = 17
	magicPowerFullHeal        = 20
	magicPowerLevitate        = 22
	magicPowerResistFire      = 23
	magicPowerFly             = 24
	magicPowerResistMagic     = 25
	magicPowerShockbolt       = 26
	magicPowerRumble          = 27
	magicPowerBurn            = 28
	magicPowerBlister         = 29
	magicPowerDustGust        = 30
	magicPowerWaterBolt       = 31
	magicPowerStoneCrush      = 32
	magicPowerKnowAlignment   = 42
	magicPowerResistCold      = 44
	magicPowerBreatheWater    = 45
	magicPowerEarthShield     = 46
	magicPowerRemoveDisease   = 49
	magicPowerRemoveBlindness = 50
	magicPowerFear            = 51
	magicPowerBlind           = 54
	magicPowerSilence         = 55
	magicPowerEnchant         = 16
	magicPowerRemoveCurse     = 43
	magicPowerCurse           = 57
	magicPowerMagicTrack      = 21
	magicPowerLocatePlayer    = 47

	magicPowerRestore    = 9
	magicPowerTeleport   = 12
	magicPowerSummon     = 18
	magicPowerMend       = 19
	magicPowerDrainExp   = 48
	magicPowerRoomVigor  = 52
	magicPowerObjectSend = 53
	magicPowerCharm      = 56

	// Tier 3 offensive spells (level 5 / 10 MP in C)
	magicPowerEngulf     = 33 // SENGUL - 낙석 (Earth)
	magicPowerBurstFlame = 34 // SBURST - 화풍술 (Fire)
	magicPowerSteamBlast = 35 // SSTEAM - 화룡대천 (Water)

	// Tier 4 offensive spells (level 4 / 15 MP in C)
	magicPowerShatterStone = 36 // SSHATT - 토합술 (Earth)
	magicPowerImmolate     = 37 // SIMMOL - 주작현 (Fire)
	magicPowerBloodBoil    = 38 // SBLOOD - 열사천 (Water)

	// Tier 5 offensive spells (level 5 / 25 MP in C)
	magicPowerThunderbolt = 39 // STHUND - 파천풍 (Wind)
	magicPowerEarthquake  = 40 // SEQUAK - 지옥패 (Earth)
	magicPowerFlameFill   = 41 // SFLFIL - 태양안 (Fire)

	// Sixiang tier (level 6 / 35 MP in C)
	magicPowerSisix1 = 58 // SISIX1 - 천지진동 (Earth)
	magicPowerSisix2 = 59 // SISIX2 - 천상풍 (Wind)
	magicPowerSisix3 = 60 // SISIX3 - 천마강기 (Fire)
	magicPowerSisix4 = 61 // SISIX4 - 빙천파 (Water)

	magicPowerRmGong = 62

	// Xixix tier (level 7 / 60 MP in C)
	magicPowerXixix1 = 63 // XIXIX1 - 혈사천 (Earth)
	magicPowerXixix2 = 64 // XIXIX2 - 빙설검 (Wind)
	magicPowerXixix3 = 65 // XIXIX3 - 멸겁화궁 (Fire)
	magicPowerXixix4 = 66 // XIXIX4 - 탄지수통 (Water)

	magicLightDurationSeconds = 600
)

type magicMoveWorld interface {
	MovePlayerToRoom(model.PlayerID, model.RoomID) error
}

type magicCreatureStatWorld interface {
	SetCreatureStat(model.CreatureID, string, int) error
}

type magicEffectExpirationWorld interface {
	SetEffectExpiration(model.CreatureID, string, int64)
}

type magicCreatureDamageWorld interface {
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
}

type magicEffectTarget struct {
	creature  model.Creature
	player    model.Player
	hasPlayer bool
}

type magicEffectDamageDice struct {
	nDice int
	sDice int
	pDice int
}

func defaultReadScrollMagicEffect(
	ctx *Context,
	world ReadScrollWorld,
	creature model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	return applyMagicPowerEffect(ctx, world, creature, object, resolved, readScrollMagicPower(world, object), true)
}

func defaultZapMagicEffect(
	ctx *Context,
	world ZapWorld,
	creature model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	return applyMagicPowerEffect(ctx, world, creature, object, resolved, zapMagicPower(world, object), true)
}

func defaultDrinkMagicEffect(
	ctx *Context,
	world DrinkWorld,
	creature model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	magicPower := drinkMagicPower(world, object)
	if !magicPowerEffectSupported(magicPower) {
		return true, nil
	}
	return applyMagicPowerEffect(ctx, world, creature, object, resolved, magicPower, true)
}

func magicPowerEffectSupported(magicPower int) bool {
	if _, ok := magicEffectDamageDiceForPower(magicPower); ok {
		return true
	}
	switch magicPower {
	case magicPowerVigor, magicPowerLight, magicPowerCurePoison, magicPowerBless,
		magicPowerProtection, magicPowerInvisibility, magicPowerDetectInvisible, magicPowerDetectMagic,
		magicPowerRecall, magicPowerFullHeal, magicPowerLevitate, magicPowerResistFire, magicPowerFly,
		magicPowerResistMagic, magicPowerBefuddle, magicPowerKnowAlignment, magicPowerResistCold, magicPowerBreatheWater,
		magicPowerEarthShield, magicPowerRemoveDisease, magicPowerRemoveBlindness, magicPowerFear,
		magicPowerBlind, magicPowerSilence, magicPowerEnchant, magicPowerRemoveCurse, magicPowerCurse,
		magicPowerMagicTrack, magicPowerLocatePlayer, magicPowerRestore, magicPowerTeleport, magicPowerSummon,
		magicPowerMend, magicPowerDrainExp, magicPowerRoomVigor, magicPowerObjectSend, magicPowerCharm, magicPowerRmGong,
		magicPowerEngulf, magicPowerBurstFlame, magicPowerSteamBlast,
		magicPowerShatterStone, magicPowerImmolate, magicPowerBloodBoil,
		magicPowerThunderbolt, magicPowerEarthquake, magicPowerFlameFill,
		magicPowerSisix1, magicPowerSisix2, magicPowerSisix3, magicPowerSisix4,
		magicPowerXixix1, magicPowerXixix2, magicPowerXixix3, magicPowerXixix4:
		return true
	default:
		return false
	}
}

func applyMagicPowerEffect(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
	magicPower int,
	lightCompatibility bool,
) (bool, error) {
	if _, ok := magicEffectDamageDiceForPower(magicPower); ok {
		return magicEffectApplyBasicOffensiveDamage(ctx, world, actor, object, resolved, magicPower)
	}

	if handled, success, err := ApplyMagicPowerEffectAgent5(ctx, world, actor, object, resolved, magicPower); handled {
		return success, err
	}
	if handled, success, err := ApplyMagicPowerEffectAgent2(ctx, world, actor, object, resolved, magicPower); handled {
		return success, err
	}

	switch magicPower {
	case magicPowerVigor:
		return magicEffectVigor(ctx, world, actor, object, resolved)
	case magicPowerLight:
		if !lightCompatibility {
			return false, nil
		}
		return magicEffectLight(ctx, world, actor, object)
	case magicPowerCurePoison:
		return magicEffectCurePoison(ctx, world, actor, object, resolved)
	case magicPowerBless:
		return magicEffectBless(ctx, world, actor, object, resolved)
	case magicPowerProtection:
		return magicEffectProtection(ctx, world, actor, object, resolved)
	case magicPowerInvisibility:
		return magicEffectInvisibility(ctx, world, actor, object, resolved)
	case magicPowerDetectInvisible:
		return magicEffectDetectInvisible(ctx, world, actor, object, resolved)
	case magicPowerDetectMagic:
		return magicEffectDetectMagic(ctx, world, actor, object, resolved)
	case magicPowerBefuddle:
		return magicEffectBefuddle(ctx, world, actor, object, resolved)
	case magicPowerRecall:
		return magicEffectRecall(ctx, world, actor, object, resolved)
	case magicPowerFullHeal:
		return magicEffectHeal(ctx, world, actor, object, resolved)
	case magicPowerLevitate:
		return magicEffectLevitate(ctx, world, actor, object, resolved)
	case magicPowerResistFire:
		return magicEffectResistFire(ctx, world, actor, object, resolved)
	case magicPowerFly:
		return magicEffectFly(ctx, world, actor, object, resolved)
	case magicPowerResistMagic:
		return magicEffectResistMagic(ctx, world, actor, object, resolved)
	case magicPowerKnowAlignment:
		return magicEffectKnowAlignment(ctx, world, actor, object, resolved)
	case magicPowerResistCold:
		return magicEffectResistCold(ctx, world, actor, object, resolved)
	case magicPowerBreatheWater:
		return magicEffectBreatheWater(ctx, world, actor, object, resolved)
	case magicPowerEarthShield:
		return magicEffectEarthShield(ctx, world, actor, object, resolved)
	case magicPowerRemoveDisease:
		return magicEffectRemoveDisease(ctx, world, actor, object, resolved)
	case magicPowerRemoveBlindness:
		return magicEffectRemoveBlindness(ctx, world, actor, object, resolved)
	case magicPowerFear:
		return magicEffectFear(ctx, world, actor, object, resolved)
	case magicPowerBlind:
		return magicEffectBlind(ctx, world, actor, object, resolved)
	case magicPowerSilence:
		return magicEffectSilence(ctx, world, actor, object, resolved)
	case magicPowerEnchant:
		return magicEffectEnchant(ctx, world, actor, object, resolved)
	case magicPowerRemoveCurse:
		return magicEffectRemoveCurse(ctx, world, actor, object, resolved)
	case magicPowerCurse:
		return magicEffectCurse(ctx, world, actor, object, resolved)
	case magicPowerRestore:
		return magicEffectRestore(ctx, world, actor, model.ObjectInstance{}, resolved)
	case magicPowerTeleport:
		return ApplyMagicTeleport(ctx, world, actor, object, resolved)
	case magicPowerSummon:
		return ApplyMagicSummon(ctx, world, actor, object, resolved)
	case magicPowerMend:
		return magicEffectMend(ctx, world, actor, object, resolved)
	case magicPowerRoomVigor:
		return magicEffectRoomVigor(ctx, world, actor, object, resolved)
	case magicPowerObjectSend:
		return magicEffectObjectSend(ctx, world, actor, object, resolved)
	default:
		return false, nil
	}
}

func magicEffectRecall(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 30 {
			ctx.WriteString("\n당신이 도력이 부족합니다.\n")
			return false, nil
		}
		class := creatureClass(actor)
		if class != model.ClassCleric && class < model.ClassInvincible {
			ctx.WriteString("\n불제자만이 이 주술을 사용할 수 있습니다.\n")
			return false, nil
		}
		if class >= model.ClassInvincible && !creatureHasAnyFlag(actor, "SCLERIC", "clericTraining", "clericSpell", "clericMode") {
			ctx.WriteString("\n불제자를 무적수련하지 않았습니다..\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SRECAL", "recallSpell") {
			ctx.WriteString("\n당신은 아직 그런 주문을 터득하지 못했습니다.\n")
			return false, nil
		}
	}

	targetArg := strings.TrimSpace(getArg(resolved, 1))
	if targetArg == "" {
		return magicEffectRecallSelf(ctx, world, actor, how)
	}
	if how == howPotion {
		ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	target, ok := magicEffectResolveRoomPlayerTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	if !ok {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다.\n")
		return false, nil
	}

	if how == howCast {
		if err := magicEffectConsumeMP(world, actor, 30); err != nil {
			return false, err
		}
	}

	actorName := attackCreatureName(actor)
	targetName := magicEffectRecallTargetName(target)
	ctx.WriteString("귀환 주문을 " + targetName + "에게 외웠습니다.\n")
	_ = sendToPlayer(ctx, target.player.ID, actorName+krtext.Particle(actorName, '1')+" 당신에게 귀환 주문을 외웠습니다.\n")
	_ = roomBroadcast2(ctx, world, actor.RoomID, ctx.SessionID, target.player.ID, actorName+krtext.Particle(actorName, '1')+" "+targetName+"에게 귀환 주문을 외웠습니다.")

	if !magicEffectRecallCanMove(ctx, world, defaultReturnRoomID) {
		return false, nil
	}
	mover := world.(magicMoveWorld)
	if err := mover.MovePlayerToRoom(target.player.ID, defaultReturnRoomID); err != nil {
		return false, err
	}
	return true, nil
}

func magicEffectRecallSelf(ctx *Context, world StatusWorld, actor model.Creature, how int) (bool, error) {
	target, ok := magicEffectSelfTarget(ctx, world, actor)
	if !ok || !target.hasPlayer {
		ctx.WriteString("주문이 실패했습니다.\n")
		return false, nil
	}

	if how == howCast {
		if err := magicEffectConsumeMP(world, actor, 30); err != nil {
			return false, err
		}
	}

	actorName := attackCreatureName(actor)
	if how == howPotion {
		ctx.WriteString("당신의 모습이 어지러이 흔들립니다.\n")
		_ = roomBroadcast(ctx, actor.RoomID, actorName+krtext.Particle(actorName, '1')+" 사라졌습니다.")
	} else {
		pronoun := "그녀"
		if creatureHasAnyFlag(actor, "PMALES", "male") {
			pronoun = "그"
		}
		ctx.WriteString("귀환 주문을 외웠습니다.\n")
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+pronoun+" 자신에게 귀환 주문을 외웠습니다.\n")
	}

	if !magicEffectRecallCanMove(ctx, world, magicRecallSelfRoomID) {
		return false, nil
	}
	mover := world.(magicMoveWorld)
	if err := mover.MovePlayerToRoom(target.player.ID, magicRecallSelfRoomID); err != nil {
		return false, err
	}
	return true, nil
}

func magicEffectRecallCanMove(ctx *Context, world StatusWorld, roomID model.RoomID) bool {
	if _, ok := world.Room(roomID); !ok {
		ctx.WriteString("주문이 실패했습니다.\n")
		return false
	}
	if _, ok := world.(magicMoveWorld); !ok {
		ctx.WriteString("주문이 실패했습니다.\n")
		return false
	}
	return true
}

func magicEffectVisiblePlayerCount(world StatusWorld, room model.Room) int {
	n := 0
	for _, id := range room.PlayerIDs {
		if id.IsZero() {
			continue
		}
		player, ok := world.Player(id)
		if !ok || player.RoomID != room.ID || player.CreatureID.IsZero() {
			continue
		}
		creature, ok := world.Creature(player.CreatureID)
		if !ok || creature.RoomID != room.ID || creatureHasAnyFlag(creature, "PDMINV", "pdminv", "dmInvisible") {
			continue
		}
		n++
	}
	return n
}

func magicEffectResolveRoomPlayerTarget(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	target string,
	ordinal int64,
) (magicEffectTarget, bool) {
	target = legacyUpperFirstASCII(strings.TrimSpace(target))
	if target == "" {
		return magicEffectTarget{}, false
	}
	if ordinal < 1 {
		ordinal = 1
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

	detectInvisible := viewerDetectsInvisible(world, viewer)
	var seen int64
	for _, id := range room.PlayerIDs {
		if id.IsZero() {
			continue
		}
		player, ok := world.Player(id)
		if !ok || player.RoomID != room.ID || player.CreatureID.IsZero() {
			continue
		}
		creature, ok := world.Creature(player.CreatureID)
		if !ok || creature.RoomID != room.ID {
			continue
		}
		if creature.DisplayName == "" {
			creature.DisplayName = player.DisplayName
		}
		if !legacyFindCrtVisible(creature, detectInvisible) || !legacyCreaturePrefixMatches(creature, target) {
			continue
		}
		seen++
		if seen == ordinal {
			return magicEffectTarget{creature: creature, player: player, hasPlayer: true}, true
		}
	}
	return magicEffectTarget{}, false
}

func magicEffectResolveMonsterFirstTarget(
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
	if creature, ok := findLegacyMonsterTarget(world, room, viewer, target, ordinal); ok {
		return magicEffectTarget{creature: creature}, true
	}
	return magicEffectResolveRoomPlayerTarget(ctx, world, actor, target, ordinal)
}

func magicEffectRejectPlayerFallbackTarget(actor model.Creature, targetArg string, target magicEffectTarget) bool {
	if !target.hasPlayer {
		return false
	}
	if target.creature.ID == actor.ID {
		return true
	}
	return len(cleanDisplayText(legacyUpperFirstASCII(strings.TrimSpace(targetArg)))) < 3
}

func magicEffectRecallTargetName(target magicEffectTarget) string {
	if target.creature.DisplayName != "" {
		return target.creature.DisplayName
	}
	if target.player.DisplayName != "" {
		return target.player.DisplayName
	}
	if !target.player.ID.IsZero() {
		return string(target.player.ID)
	}
	return attackCreatureName(target.creature)
}

func magicEffectSpellFail(world StatusWorld, actor model.Creature, how int, castCost int) (bool, error) {
	if !spellFail(actor) {
		return false, nil
	}
	if how == howCast && castCost > 0 {
		if err := magicEffectConsumeMP(world, actor, castCost); err != nil {
			return false, err
		}
	}
	return true, nil
}

func magicEffectPrepayCastMP(world StatusWorld, actor model.Creature, how int, cost int) error {
	if how != howCast || cost <= 0 {
		return nil
	}
	return magicEffectConsumeMP(world, actor, cost)
}

func magicEffectResistFire(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 12 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SRFIRE", "resistFireSpell") {
			ctx.WriteString("\n당신은 아직 그런 주술을 터득하지 못했습니다.\n")
			return false, nil
		}
	}
	if failed, err := magicEffectSpellFail(world, actor, how, 12); failed || err != nil {
		return false, err
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
			ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
			return false, nil
		}
		target, ok = magicEffectResolveRoomPlayerTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}
	if !ok || target.creature.ID.IsZero() || (!isSelf && !target.hasPlayer) {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다.\n")
		return false, nil
	}

	interval := int64(1200)
	if how == howCast {
		interval = int64(maxInt(300, 1200+legacyStatBonus(creatureStat(actor, "intelligence"))*600))
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			interval += 800
		}
	}

	if err := magicEffectUpdateTags(world, target, []string{"resistFire", "PRFIRE"}, nil); err != nil {
		return false, err
	}
	if expUpdater, ok := world.(magicEffectExpirationWorld); ok {
		expUpdater.SetEffectExpiration(target.creature.ID, "PRFIRE", timeNow().Unix()+interval)
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 방열진의 금패를 들어올리면서 주문을 외웁습니다.\n오행중 수의 수호령들이 나타나 그의 주위에 진을 형성합니다.\n")
		if how == howCast {
			ctx.WriteString("\n당신은 방열진의 금패를 들어올리면서 주문을 외웁니다.\n오행중 수의 수호령들이 나타나 당신주위에서 진을 형성합니다.\n")
			if err := magicEffectDeductMPLegacy(world, actor, 12); err != nil {
				return false, err
			}
			if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
				ctx.WriteString("\n이 방의 기운이 당신의 주문을 강화시킵니다.\n")
			}
		} else {
			ctx.WriteString("\n갑자기 오행중 수의 수호령들이 나타나 당신주위에 \n진을 형성합니다.\n")
		}
		return true, nil
	}

	targetName := attackCreatureName(target.creature)
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, actorName+krtext.Particle(actorName, '1')+" "+targetName+"에게 방열부적을 붙이며 주문을 외웁니다.\n오행중 수의 수호령들이 나타나 그의 주위에 진을 형성합니다.\n")
	_ = sendToPlayer(ctx, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 당신에게 방열부적을 붙이며 주문을 외웁니다.\n갑자기 오행중 수의 수호령들이 나타나 당신주위에 \n진을 형성합니다.\n")
	if how == howCast {
		ctx.WriteString("당신은 " + targetName + "에게 방열부적을 붙이며 주문을 외웁니다.\n오행중 수의 수호령들이 나타나 그의 주위에 진을 형성합니다.\n")
		if err := magicEffectDeductMPLegacy(world, actor, 12); err != nil {
			return false, err
		}
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			ctx.WriteString("\n이방의 기운이 당신의 주문을 강화시킵니다.\n")
		}
	} else {
		ctx.WriteString("\n오행중 수의 수호령들이 나타나 " + targetName + "의 주위에 \n진을 형성합니다.\n")
	}
	return true, nil
}

func magicEffectFly(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 15 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SFLYSP", "flySpell") {
			ctx.WriteString("\n당신은 아직 그런 주술을 터득하지 못했습니다.\n")
			return false, nil
		}
	}
	if failed, err := magicEffectSpellFail(world, actor, how, 15); failed || err != nil {
		return false, err
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
			ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
			return false, nil
		}
		target, ok = magicEffectResolveRoomPlayerTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}
	if !ok || target.creature.ID.IsZero() || (!isSelf && !target.hasPlayer) {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다.\n")
		return false, nil
	}

	interval := int64(1200)
	if how == howCast {
		interval = int64(maxInt(300, 1200+legacyStatBonus(creatureStat(actor, "intelligence"))*600))
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			interval += 600
		}
	}

	if err := magicEffectUpdateTags(world, target, []string{"fly", "PFLYSP"}, nil); err != nil {
		return false, err
	}
	if expUpdater, ok := world.(magicEffectExpirationWorld); ok {
		expUpdater.SetEffectExpiration(target.creature.ID, "PFLYSP", timeNow().Unix()+interval)
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 비상술의 주문을 외우자 몸이 떠오르며 \n하늘로 날기 시작합니다.\n")
		if how == howCast {
			ctx.WriteString("\n당신은 비상술의 주문을 외우자 몸이 공기처럼 가벼워지며\n하늘로 떠올라 날기 시작합니다.\n")
			if err := magicEffectDeductMPLegacy(world, actor, 15); err != nil {
				return false, err
			}
			if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
				ctx.WriteString("\n이 방의 기운이 당신의 주문을 강화시킵니다.\n")
			}
		} else {
			ctx.WriteString("당신의 몸이 하늘로 떠오르며 날수 있습니다!\n")
		}
		return true, nil
	}

	targetName := attackCreatureName(target.creature)
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+targetName+"에게 비상부를 붙히며 주문을 외웠습니다.\n그의 몸이 하늘로 떠오르며 날기 시작합니다.\n")
	_ = sendToPlayer(ctx, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 당신에게 비상부를 붙히며 주문을 외웠습니다.\n갑자기 당신의 몸이 공기처럼 가벼워지며 하늘로 떠올라\n날기 시작합니다.\n")
	if how == howCast {
		ctx.WriteString("\n당신은 " + targetName + "에게 비상부를 붙히며 주문을 외웁니다.\n그의 몸이 하늘로 떠오르며 날기 시작합니다.\n")
		if err := magicEffectDeductMPLegacy(world, actor, 15); err != nil {
			return false, err
		}
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			ctx.WriteString("\n이 방의 기운이 당신의 주문을 강화시킵니다.\n")
		}
	} else {
		ctx.WriteString(targetName + "의 몸이 하늘로 떠오르며 날기 시작합니다.\n")
	}
	return true, nil
}

func magicEffectResistMagic(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 12 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SRMAGI", "resistMagicSpell") {
			ctx.WriteString("\n당신은 아직 그런 주문을 터득하지 못했습니다.\n")
			return false, nil
		}
	}
	if failed, err := magicEffectSpellFail(world, actor, how, 12); failed || err != nil {
		return false, err
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
			ctx.WriteString("\n그 물건은 약병은 자신에게만 사용할수 있습니다.\n")
			return false, nil
		}
		target, ok = magicEffectResolveRoomPlayerTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}
	if !ok || target.creature.ID.IsZero() || (!isSelf && !target.hasPlayer) {
		ctx.WriteString("\n그런 사람은 존재하지 않습니다.\n")
		return false, nil
	}

	interval := int64(1200)
	if how == howCast {
		interval = int64(maxInt(300, 1200+legacyStatBonus(creatureStat(actor, "intelligence"))*600))
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			interval += 800
		}
	}

	if err := magicEffectUpdateTags(world, target, []string{"resistMagic", "PRMAGI"}, nil); err != nil {
		return false, err
	}
	if expUpdater, ok := world.(magicEffectExpirationWorld); ok {
		expUpdater.SetEffectExpiration(target.creature.ID, "PRMAGI", timeNow().Unix()+interval)
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 허공에 보마부을 그리며 주문을 외웠습니다.\n갑자기 땅속에서 오행중 금의 정령들이 올라와 \n보마진을 형성합니다.\n")
		if how == howCast {
			ctx.WriteString("당신은 허공에 보마부를 그리며 주문을 외웁니다.\n땅속에서 오행중 금의 수호령들이 올라와 보마진을 형성합니다.\n")
			if err := magicEffectDeductMPLegacy(world, actor, 12); err != nil {
				return false, err
			}
			if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
				ctx.WriteString("\n이 방의 기운이 당신의 주문을 강화시킵니다.\n")
			}
		} else {
			ctx.WriteString("\n갑자기 땅속에서 오행중 금의 수호령들이 올라와 \n보마진을 형성합니다.\n")
		}
		return true, nil
	}

	targetName := attackCreatureName(target.creature)
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, actorName+krtext.Particle(actorName, '1')+" "+targetName+"의 몸에 보마부를 그리며 주문을\n외웠습니다.\n갑자기 땅속에서 금의 수호령들이 올라와 보마진을 \n형성합니다.\n")
	_ = sendToPlayer(ctx, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 당신의 몸에 보마부를 그리며 주문을\n외웠습니다.\n갑자기 땅속에서 금의 수호령들이 올라와 보마진을 \n형성합니다.\n")
	if how == howCast {
		ctx.WriteString("\n당신은 " + targetName + "의 몸에 보마부를 그리며 주문을\n외웠습니다.\n땅속에서 금의 수호령들이 올라와 보마진을 형성합니다.\n")
		if err := magicEffectDeductMPLegacy(world, actor, 12); err != nil {
			return false, err
		}
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			ctx.WriteString("\n이 방의 기운이 당신의 주술력을 강화시킵니다.\n")
		}
	} else {
		ctx.WriteString("\n땅속에서 금의 수호령들이 올라와 " + targetName + "의 주위에 \n보마진을 형성합니다.\n")
	}
	return true, nil
}

func magicEffectKnowAlignment(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 6 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SKNOWA", "knowAlignmentSpell") {
			ctx.WriteString("\n당신은 아직 그런 주문을 터득하지 못했습니다.\n")
			return false, nil
		}
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
			ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
			return false, nil
		}
		target, ok = magicEffectResolveRoomPlayerTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}
	if !ok || target.creature.ID.IsZero() || (!isSelf && !target.hasPlayer) {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다.\n")
		return false, nil
	}

	interval := int64(1200)
	if how == howCast {
		interval = int64(maxInt(300, 1200+legacyStatBonus(creatureStat(actor, "intelligence"))*600))
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			interval += 800
		}
	}

	if err := magicEffectUpdateTags(world, target, []string{"knowAlignment", "PKNOWA"}, nil); err != nil {
		return false, err
	}
	if expUpdater, ok := world.(magicEffectExpirationWorld); ok {
		expUpdater.SetEffectExpiration(target.creature.ID, "PKNOWA", timeNow().Unix()+interval)
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 선악감지 주문을 외웁니다.")
		ctx.WriteString("\n당신은 선악을 감지할 수 있는 식별력이 높아졌습니다.\n")
		if how == howCast {
			if err := magicEffectDeductMPLegacy(world, actor, 6); err != nil {
				return false, err
			}
			if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
				ctx.WriteString("\n이 방의 기운이 당신의 주술력을 강화시킵니다.\n")
			}
		}
		return true, nil
	}

	targetName := attackCreatureName(target.creature)
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+targetName+"에게 선악감지 주문을 외웁니다.\n그는 선악을 감지할 수 있는 식별력이 높아졌습니다.\n")
	_ = sendToPlayer(ctx, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 당신에게 선악감지 주문을 외웁니다.\n당신은 선악을 감지할 수 있는 식별력이 높아졌습니다.\n")
	if how == howCast {
		ctx.WriteString("당신은 " + targetName + "에게 선악감지 주문을 외웁니다.\n")
		if err := magicEffectDeductMPLegacy(world, actor, 6); err != nil {
			return false, err
		}
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			ctx.WriteString("\n이 방의 기운이 당신의 주술력을 강화시킵니다.\n")
		}
	} else {
		ctx.WriteString(targetName + "이 선악을 감지할 수 있는 식별력이 높아졌습니다.\n")
	}
	return true, nil
}

func magicEffectBefuddle(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 10 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SBEFUD", "befuddleSpell") {
			ctx.WriteString("\n당신은 아직 그 주술을 터득하지 못했습니다.\n")
			return false, nil
		}
	}

	actorName := attackCreatureName(actor)
	selfTarget, _ := magicEffectSelfTarget(ctx, world, actor)
	if magicEffectTargetHasAnyFlag(selfTarget, "PINVIS", "invisible") {
		if err := magicEffectUpdateTags(world, selfTarget, nil, []string{"PINVIS", "invisible"}); err != nil {
			return false, err
		}
		ctx.WriteString("\n당신의 모습이 보이기 시작합니다.\n")
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 보이기 시작합니다.\n")
	}
	if failed, err := magicEffectSpellFail(world, actor, how, 10); failed || err != nil {
		return false, err
	}

	targetArg := getArg(resolved, 1)
	if targetArg == "" || targetArg == "나" {
		dur := magicEffectBefuddleSelfDuration(actor, how)
		magicEffectSetCooldown(world, actor.ID, "attack", dur)
		if how == howPotion {
			ctx.WriteString("\n당신의 온몸의 피가 역류하면서 주화입마에 빠집니다.\n혼수상태에 빠졌습니다.\n")
		} else {
			ctx.WriteString("\n당신은 흑기를 땅에 꼿으며 혼동술의 일종인 흑안법을 \n자신에게 걸었습니다.\n주술을 걸자 갑자기 흑기에서 검은기류가 피어올라 당신의 \n정신을 혼수상태에 빠뜨립니다.")
			_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 흑기를 땅에 꼿으며 혼동술의 일종인 흑안법을 \n자신에게 걸었습니다.\n주문을 걸자 갑자기 흑기에서 검은기류가 피어올라 그의 \n정신을 혼수상태에 빠뜨립니다.")
		}
		return true, nil
	}

	if how == howPotion {
		ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	target, ok := magicEffectResolveBefuddleTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	if !ok {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다.\n")
		return false, nil
	}
	if !target.hasPlayer && creatureHasAnyFlag(target.creature, "MUNKIL", "unkillable", "cannotKill") {
		pronoun := "그녀"
		if creatureHasAnyFlag(target.creature, "MMALES", "male") {
			pronoun = "그"
		}
		ctx.WriteString("\n당신은 " + pronoun + "를 해칠수 없습니다.\n")
		return false, nil
	}

	dur := magicEffectBefuddleTargetDuration(actor, target, how)
	magicEffectSetCooldown(world, target.creature.ID, "befuddled", dur)
	spellDur := minInt(9, dur)
	magicEffectSetCooldown(world, target.creature.ID, "spell", spellDur)
	if target.hasPlayer {
		magicEffectSetCooldown(world, target.creature.ID, "attack", spellDur)
	} else {
		if err := magicEffectUpdateTags(world, target, []string{"befuddled", "MBEFUD"}, nil); err != nil {
			return false, err
		}
	}

	targetName := attackCreatureName(target.creature)
	ctx.WriteString("당신은 흑기를 땅에 꼿으며 " + targetName + "에게 일종인 흑안법을 걸었습니다.\n주술을 걸자 갑자기 흑기에서 검은기류가 피어올라 그의\n정신을 혼수상태에 빠뜨립니다.\n")
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 흑기를 땅에 꼿으며 혼동술의 일종인 흑안법을 \n"+targetName+"에게 걸었습니다.\n주술을 걸자 갑자기 흑기에서 검은기류가 피어올라 그의\n정신을 혼수상태에 빠뜨립니다.\n")
	if target.hasPlayer {
		_ = sendToPlayer(ctx, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 흑기를 땅에 꼿으며 혼동술의 일종인 흑안법을 당신에게 걸었습니다.\n주술을 걸자 갑자기 흑기에서 검은기류가 피어올라 당신의\n정신을 혼수상태에 빠뜨립니다.\n")
	} else {
		RegisterSpellAggro(world, target.creature.ID, actor.ID)
	}
	return true, nil
}

func magicEffectResolveBefuddleTarget(
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

	viewer := LookViewer{
		PlayerID:   InventoryPlayerIDFromContext(ctx),
		CreatureID: actor.ID,
	}
	if attackTargetMatchesSelf(world, viewer, actor, target) {
		return magicEffectTarget{}, false
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
	if creature, ok := findAttackCreatureTarget(world, room, viewer, target, ordinal); ok {
		return magicEffectTarget{creature: creature}, true
	}
	if len(target) < 3 {
		return magicEffectTarget{}, false
	}
	if player, creature, ok := findLegacyPlayerCreatureTarget(world, room, viewer, legacyUpperFirstASCII(target), ordinal); ok {
		return magicEffectTarget{creature: creature, player: player, hasPlayer: true}, true
	}
	return magicEffectTarget{}, false
}

func magicEffectBefuddleSelfDuration(actor model.Creature, how int) int {
	dur := mrand(1, 6) + mrand(1, 6)
	if how == howCast {
		dur += legacyStatBonus(creatureStat(actor, "intelligence")) * 2
		if creatureClass(actor) == model.ClassMage {
			dur += ((creatureStat(actor, "level") + 3) / 4) / 2
		}
	}
	return maxInt(6, dur)
}

func magicEffectBefuddleTargetDuration(actor model.Creature, target magicEffectTarget, how int) int {
	dur := mrand(1, 5) + mrand(1, 5)
	if how == howCast {
		dur = legacyStatBonus(creatureStat(actor, "intelligence")) + mrand(1, 6) + mrand(1, 6)
	}
	if (target.hasPlayer && magicEffectTargetHasAnyFlag(target, "PRMAGI", "resistMagic")) ||
		(!target.hasPlayer && magicEffectTargetHasAnyFlag(target, "MRMAGI", "MRBEFD", "resistMagic", "befuddleResistance")) {
		return 3
	}
	return maxInt(5, dur)
}

func magicEffectInvisibility(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 15 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SINVIS", "invisibilitySpell") {
			ctx.WriteString("\n당신은 아직 그 주술을 터득하지 못했습니다.\n")
			return false, nil
		}
	}
	if failed, err := magicEffectSpellFail(world, actor, how, 15); failed || err != nil {
		return false, err
	}
	if magicEffectActorHasRoomEnemy(world, actor) {
		ctx.WriteString("\n지금 싸우고 있잖아요..!!.\n")
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
			ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
			return false, nil
		}
		target, ok = magicEffectResolveRoomPlayerTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}
	if !ok || target.creature.ID.IsZero() || (!isSelf && !target.hasPlayer) {
		ctx.WriteString("\n그런 사람은 존재하지 않습니다.\n")
		return false, nil
	}
	if how == howPotion && !isSelf {
		ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	interval := int64(1200)
	if how == howCast {
		interval = int64(1200 + legacyStatBonus(creatureStat(actor, "intelligence"))*600)
		if creatureClass(actor) == model.ClassMage {
			interval += int64(60 * ((creatureStat(actor, "level") + 3) / 4))
		}
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			interval += 600
		}
	}
	if err := magicEffectUpdateTags(world, target, []string{"invisible", "PINVIS"}, nil); err != nil {
		return false, err
	}
	if expUpdater, ok := world.(magicEffectExpirationWorld); ok {
		expUpdater.SetEffectExpiration(target.creature.ID, "PINVIS", timeNow().Unix()+interval)
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		if how == howCast {
			if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
				ctx.WriteString("\n이 방의 기운이 당신의 주술력을 강화시킵니다.\n")
			}
		}
		if how == howPotion {
			ctx.WriteString("\n당신의 몸이 눈부실 정도로 강렬한 빛을 내다가 갑자기 사라졌습니다.\n")
		} else {
			ctx.WriteString("\n당신은 소명부를 삼키면서 은둔법의 주문을 외웁니다.\n몸이 눈부실 정도로 강렬한 빛을 내다가 갑자기 사라졌습니다.\n")
			_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 소명부를 삼키면서  은둔법의 주문을 외웁니다.\n그의 몸이 눈부실 정도로 강렬한 빛을 내다가 갑자기 \n사라졌습니다.\n ")
		}
		return true, nil
	}

	targetName := attackCreatureName(target.creature)
	if how == howCast {
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			ctx.WriteString("\n이 방의 기운이 당신의 주술력을 강화시킵니다.\n")
		}
	}
	ctx.WriteString("\n" + targetName + "에게 소명부를 먹이고 은둔법의 주문을 겁니다.\n" + targetName + "의 몸이 눈부실 정도로 강렬한 빛을 내다가 갑자기 \n사라졌습니다.\n")
	_ = sendToPlayer(ctx, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 당신에게 소명부를 먹이고 은둔법의 주문을 겁니다.\n몸이 눈부실 정도로 강렬한 빛을 내다가 갑자기 사라졌습니다.\n")
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+targetName+"에게 소명부를 먹이고 은둔법의 주문을 겁니다.\n그의 몸이 눈부실 정도로 강렬한 빛을 내다가 갑자기 사라졌습니다.\n")
	return true, nil
}

func magicEffectActorHasRoomEnemy(world StatusWorld, actor model.Creature) bool {
	room, ok := world.Room(actor.RoomID)
	if !ok {
		return false
	}
	lister, ok := world.(interface {
		CreatureEnemies(model.CreatureID) ([]string, error)
	})
	if !ok {
		return false
	}
	actorName := strings.TrimSpace(attackCreatureName(actor))
	if actorName == "" {
		return false
	}
	for _, id := range room.CreatureIDs {
		if id.IsZero() || id == actor.ID {
			continue
		}
		creature, ok := world.Creature(id)
		if !ok || creature.RoomID != room.ID || attackCreatureIsPlayer(creature) {
			continue
		}
		enemies, err := lister.CreatureEnemies(id)
		if err != nil {
			continue
		}
		for _, enemy := range enemies {
			if strings.TrimSpace(enemy) == actorName {
				return true
			}
		}
	}
	return false
}

func magicEffectDetectInvisible(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 10 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SDINVI", "detectInvisibleSpell") {
			ctx.WriteString("\n당신은 아직 그런 주술을 터득하지 못했습니다.\n")
			return false, nil
		}
	}
	if failed, err := magicEffectSpellFail(world, actor, how, 10); failed || err != nil {
		return false, err
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
			ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
			return false, nil
		}
		target, ok = magicEffectResolveRoomPlayerTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}
	if !ok || target.creature.ID.IsZero() || (!isSelf && !target.hasPlayer) {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다.\n")
		return false, nil
	}
	if how == howPotion && !isSelf {
		ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	interval := magicEffectDetectInterval(world, actor, how)
	if err := magicEffectUpdateTags(world, target, []string{"detectInvisible", "PDINVI"}, nil); err != nil {
		return false, err
	}
	if expUpdater, ok := world.(magicEffectExpirationWorld); ok {
		expUpdater.SetEffectExpiration(target.creature.ID, "PDINVI", timeNow().Unix()+interval)
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		if how == howPotion {
			ctx.WriteString("\n.갑자기 두눈에 푸른광안이 떠오르며 숨어있는 자들을 볼수 있게 되었습니다.\n")
		} else {
			if how == howCast && magicEffectRoomExtendsMagic(world, actor) {
				ctx.WriteString("\n이 방의 기운이 당신의 주문을 강화시킵니다.\n")
			}
			ctx.WriteString("\n당신은 버들잎을 두눈에 비비며 은둔감지술의 주문을\n외웁니다.\n두눈에 푸른광안이 떠오르며 숨어있는 자들을 볼수\n있게되었습니다.\n")
			_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 버들잎을 두눈에 비비며 자신에게 은둔감지술의\n주문을 외웠습니다.\n 그의 눈에서 푸른광안이 떠오릅니다.\n")
		}
		return true, nil
	}

	if how == howCast && magicEffectRoomExtendsMagic(world, actor) {
		ctx.WriteString("\n이 방의 기운이 당신의 주문을 강화시킵니다.\n")
	}
	targetName := attackCreatureName(target.creature)
	ctx.WriteString("\n당신은 " + targetName + "의 인당혈을 찍으며 은둔감지술을 외웁니다.\n그의 눈에서 푸른광안이 떠오릅니다.\n")
	_ = sendToPlayer(ctx, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 당신의 인당혈을 찍으며 은둔감지술을 외웠습니다.\n갑자기 두눈에 푸른광안이 떠오르며 숨어있는 자들을 볼수\n있게 되었습니다.\n")
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+targetName+"의 인당혈을 찍으며 은둔감지술을 외웁니다.\n그의 눈에서 푸른광안이 떠오릅니다.\n")
	return true, nil
}

func magicEffectDetectMagic(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 10 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SDMAGI", "SDETEC", "detectMagicSpell") {
			ctx.WriteString("\n당신은 아직 그런 주술을 터득하지 못했습니다.\n")
			return false, nil
		}
	}
	if failed, err := magicEffectSpellFail(world, actor, how, 10); failed || err != nil {
		return false, err
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
			ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
			return false, nil
		}
		target, ok = magicEffectResolveTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
		if ok && target.creature.ID == actor.ID {
			isSelf = true
		}
	}
	if !ok || target.creature.ID.IsZero() || (!isSelf && !target.hasPlayer) {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다.\n")
		return false, nil
	}
	if how == howPotion && !isSelf {
		ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	interval := magicEffectDetectInterval(world, actor, how)
	if err := magicEffectUpdateTags(world, target, []string{"detectMagic", "PDMAGI"}, nil); err != nil {
		return false, err
	}
	if expUpdater, ok := world.(magicEffectExpirationWorld); ok {
		expUpdater.SetEffectExpiration(target.creature.ID, "PDMAGI", timeNow().Unix()+interval)
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		if how == howPotion {
			ctx.WriteString("\n갑자기 두눈에 은빛광안이 떠오르며 주술에 관한 안목이 \n넓어졌습니다.\n")
		} else {
			if how == howCast && magicEffectRoomExtendsMagic(world, actor) {
				ctx.WriteString("\n이 방의 기운이 당신의 주문을 강화시킵니다.\n")
			}
			ctx.WriteString("\n당신은 됴화잎을 눈에 비비며 주문감지술을 외웁니다.\n당신의 눈에서 은빛광안이 떠오르며 주술에 관한 안목이\n넓어졌습니다.\n")
			_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 됴화잎을 눈에 비비며 주문감지술을 외웠습니다.\n갑자기 그의 눈에서 은빛광안이 떠오릅니다.\n")
		}
		return true, nil
	}

	if how == howCast && magicEffectRoomExtendsMagic(world, actor) {
		ctx.WriteString("\n이 방의 기운이 당신의 주문을 강화시킵니다.\n")
	}
	targetName := attackCreatureName(target.creature)
	ctx.WriteString("\n당신은 " + targetName + "의 백회혈을 찍으며 주문감지술의 \n주문을 외웁니다.\n갑자기 그의 두눈에 은빛광안이 떠오릅니다.\n")
	_ = sendToPlayer(ctx, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 당신의 백회혈을 찍으며 주문감지술의 \n주문을 외웁니다.\n갑자기 두눈에 은빛광안이 떠오르며 주술에 관한 안목이 넓어졌습니다.\n")
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+targetName+"의 백회혈을 찍으며 주문감지술의 \n주문을 외웁니다.\n갑자기 그의 두눈에 은빛광안이 떠오릅니다.\n.")
	return true, nil
}

func magicEffectDetectInterval(world StatusWorld, actor model.Creature, how int) int64 {
	if how != howCast {
		return 1200
	}
	interval := int64(maxInt(300, 1200+legacyStatBonus(creatureStat(actor, "intelligence"))*600))
	if creatureClass(actor) == model.ClassMage {
		interval += int64(60 * ((creatureStat(actor, "level") + 3) / 4))
	}
	if magicEffectRoomExtendsMagic(world, actor) {
		interval += 600
	}
	return interval
}

func magicEffectRoomExtendsMagic(world StatusWorld, actor model.Creature) bool {
	room, ok := world.Room(actor.RoomID)
	return ok && roomHasAnyFlag(room, "RPMEXT", "rpmext")
}

func magicEffectCurePoison(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 6 {
			ctx.WriteString("당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SCUREP", "curePoisonSpell") {
			ctx.WriteString("당신은 아직 그 주술을 터득하지 못했습니다.\n")
			return false, nil
		}
	}
	if how != howPotion {
		if failed, err := magicEffectSpellFail(world, actor, how, 6); failed || err != nil {
			return false, err
		}
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
			ctx.WriteString("그 물건은 자신에게만 사용할 수 있습니다.\n")
			return false, nil
		}
		target, ok = magicEffectResolveTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
		if ok && target.creature.ID == actor.ID {
			isSelf = true
		}
	}
	if !ok || target.creature.ID.IsZero() {
		ctx.WriteString("그런 것은 존재하지 않습니다.\n")
		return false, nil
	}
	if how == howPotion && !isSelf {
		ctx.WriteString("그 물건은 자신에게만 사용할 수 있습니다.\n")
		return false, nil
	}

	wasPoisoned := magicEffectTargetHasAnyFlag(target, "poison", "poisoned", "PPOISN", "MPOISN")
	remove := []string{"poison", "poisoned", "ppoisn", "PPOISN", "MPOISN"}
	if err := magicEffectUpdateTags(world, target, nil, remove); err != nil {
		return false, err
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		if how == howPotion {
			if wasPoisoned {
				ctx.WriteString("\n독기운이 중화되는 것을 느낄 수 있습니다.\n")
			} else {
				ctx.WriteString("\n해독에 실패하셨습니다.\n")
			}
			return true, nil
		}
		ctx.WriteString("당신은 오른손으로 혈도를 짚으면서 해독 주문을 외웁니다.\n손가락 끝으로 검은 독기운이 빠져나오는것이 보입니다.\n")
		ctx.WriteString("당신 몸에 남아 있는 독이 모두 빠져나갔습니다.\n")
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 혈도를 짚으면서 해독 주문을 외웁니다.\n그의 손가락 끝으로 검은 독기운이 빠져나오는 것이 보입니다.\n")
		return true, nil
	}

	targetName := attackCreatureName(target.creature)
	ctx.WriteString(targetName + krtext.Particle(targetName, '1') + " 혈도를 짚으면서 해독 주문을 외웁니다.\n그의 손가락 끝으로 검은 독기운이 빠져나오는 것이 보입니다.\n")
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+targetName+"의 혈도를 짚으면서 해독 주문을 외웁니다.\n그의 손가락 끝으로 검은 독기운이 빠져나오는 것이 보입니다.\n")
	if target.hasPlayer {
		_ = sendToPlayer(ctx, target.player.ID, actorName+krtext.Particle(actorName, '1')+" 당신의 혈도를 짚으면서 해독 주문을 외웁니다.\n당신의 손가락 끝으로 독기운이 빠져나가는 것이 느껴집니다.\n")
	}
	return true, nil
}

func magicEffectResistCold(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 12 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SRCOLD", "resistColdSpell") {
			ctx.WriteString("\n당신은 아직 그런 주문을 터득하지 못했습니다.\n")
			return false, nil
		}
	}
	if failed, err := magicEffectSpellFail(world, actor, how, 12); failed || err != nil {
		return false, err
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
			ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
			return false, nil
		}
		target, ok = magicEffectResolveRoomPlayerTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}
	if !ok || target.creature.ID.IsZero() || (!isSelf && !target.hasPlayer) {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다 .\n")
		return false, nil
	}
	if how == howPotion && !isSelf {
		ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	interval := int64(1200)
	if how == howCast {
		interval = int64(maxInt(300, 1200+legacyStatBonus(creatureStat(actor, "intelligence"))*600))
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			interval += 800
		}
	}

	if err := magicEffectUpdateTags(world, target, []string{"resistCold", "PRCOLD"}, nil); err != nil {
		return false, err
	}
	if expUpdater, ok := world.(magicEffectExpirationWorld); ok {
		expUpdater.SetEffectExpiration(target.creature.ID, "PRCOLD", timeNow().Unix()+interval)
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 불타오르는 부적을 삼키며 방한주를 외웁니다.\n그의 주위에 오행중 화의 수호령들이 진을 형성하며 \n주위를 둘러쌉니다.")
		if how == howCast {
			ctx.WriteString("\n당신은 불타오르는 부적을 삼키며 방한주룰 외웁니다.\n당신의 주위에 오행중 화의 수호령들이 진을 형성하며\n주위를 둘러쌉니다.\n")
			if err := magicEffectDeductMPLegacy(world, actor, 12); err != nil {
				return false, err
			}
			if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
				ctx.WriteString("\n이 방의 기운이 당신의 주문을 강화시킵니다.\n")
			}
		} else {
			ctx.WriteString("\n당신의 주위에 오행중 화의 수호령들이 진을 형성하며\n주위를 둘러쌉니다.\n")
		}
		return true, nil
	}

	targetName := attackCreatureName(target.creature)
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+targetName+"의 입에 불타오르는 부적을 집어넣으며 \n방한진 주문을 외웁니다.")
	_ = sendToPlayer(ctx, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 당신의 입에 불타오르는 부적을 집어 넣으며\n방한주룰 외웁니다.\n당신의 주위에 오행중 화의 수호령들이 진을 형성하며\n주위를 둘러쌉니다.\n")
	if how == howCast {
		ctx.WriteString("\n당신은 " + targetName + "의 입에 불타오르는 부적을 집어\n넣으며 방한주룰 외웁니다.\n그의 주위에 화의 수호령들이 진을 형성하며 주위를\n둘러쌉니다.\n")
		if err := magicEffectDeductMPLegacy(world, actor, 12); err != nil {
			return false, err
		}
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			ctx.WriteString("\n이 방의 기운이 당신의 주술력을 강화시킵니다.\n")
		}
	} else {
		ctx.WriteString("\n" + targetName + "의 주위에 화의 수호령들이 진을 형성하며\n주위를 둘러쌉니다.\n")
	}
	return true, nil
}

func magicEffectBreatheWater(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 12 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SBRWAT", "breatheWaterSpell") {
			ctx.WriteString("\n당신은 아직 그런 주문을 터득하지 못했습니다.\n")
			return false, nil
		}
	}
	if failed, err := magicEffectSpellFail(world, actor, how, 12); failed || err != nil {
		return false, err
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
		target, ok = magicEffectResolveRoomPlayerTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}
	if !ok || target.creature.ID.IsZero() || (!isSelf && !target.hasPlayer) {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다 .\n")
		return false, nil
	}
	if how == howPotion && !isSelf {
		ctx.WriteString("\n이 물건은 자신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	interval := int64(1200)
	if how == howCast {
		interval = int64(maxInt(300, 1200+legacyStatBonus(creatureStat(actor, "intelligence"))*600))
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			interval += 800
		}
	}

	if err := magicEffectUpdateTags(world, target, []string{"breatheWater", "PBRWAT"}, nil); err != nil {
		return false, err
	}
	if expUpdater, ok := world.(magicEffectExpirationWorld); ok {
		expUpdater.SetEffectExpiration(target.creature.ID, "PBRWAT", timeNow().Unix()+interval)
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 크게 호흡을 들이키며서 수생술의 \n주문을 외웠습니다.\n그의 가슴이 평소보다 두배나 커져 물속에서 오랫동안\n견딜 수 있을 것 같습니다.\n")
		if how == howCast {
			ctx.WriteString("\n당신은 크게 호흡을 들이키며서 수생술의 주문을 외웁니다.\n가슴이 평소보다 두배나 커져 물속에서 오랫동안\n견딜수 있을 것 같습니다.\n")
			if err := magicEffectDeductMPLegacy(world, actor, 12); err != nil {
				return false, err
			}
			if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
				ctx.WriteString("\n이 방의 기운이 당신의 주술력을 강화시킵니다.\n")
			}
		} else {
			ctx.WriteString("\n당신의 가슴이 평소보다 두배나 커져 물속에서 오랫동안\n견딜수 있을 것 같습니다.\n")
		}
		return true, nil
	}

	targetName := attackCreatureName(target.creature)
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+targetName+"에게 수생부를 먹이며 주문을 외웠습니다.\n그의 가슴이 평소보다 두배나 커져 물속에서 오랫동안\n견딜수 있을 것 같습니다.\n")
	_ = sendToPlayer(ctx, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 당신에게 수생부를 먹이며 주문을 외웠습니다.\n당신의 가슴이 평소보다 두배나 커져 물속에서 오랫동안\n견딜 수 있을 것 같습니다.\n")
	if how == howCast {
		ctx.WriteString("\n당신은 " + targetName + "에게 수생부를 먹이며 주문을 외웁니다.\n그의 가슴이 평소보다 두배나 커져 물속에서 오랫동안\n견딜 수 있을 것 같습니다.\n")
		if err := magicEffectDeductMPLegacy(world, actor, 12); err != nil {
			return false, err
		}
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			ctx.WriteString("\n이 방의 기운이 당신의 주문을 강화시킵니다.\n")
		}
	} else {
		ctx.WriteString("\n" + targetName + "의 가슴이 평소보다 두배 커져 물속에서 \n오랫동안 견딜수 있을 것 같습니다.\n")
	}
	return true, nil
}

func magicEffectEarthShield(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 12 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SSSHLD", "earthShieldSpell", "stoneShieldSpell") {
			ctx.WriteString("\n당신은 아직 그런 주문을 터득하지 못했습니다.\n")
			return false, nil
		}
	}
	if failed, err := magicEffectSpellFail(world, actor, how, 12); failed || err != nil {
		return false, err
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
		target, ok = magicEffectResolveRoomPlayerTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}
	if !ok || target.creature.ID.IsZero() || (!isSelf && !target.hasPlayer) {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다.\n")
		return false, nil
	}
	if how == howPotion && !isSelf {
		ctx.WriteString("\n이 물건은 자신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	interval := int64(1200)
	if how == howCast {
		interval = int64(maxInt(300, 1200+legacyStatBonus(creatureStat(actor, "intelligence"))*600))
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			interval += 800
		}
	}

	if err := magicEffectUpdateTags(world, target, []string{"earthShield", "PSSHLD"}, nil); err != nil {
		return false, err
	}
	if expUpdater, ok := world.(magicEffectExpirationWorld); ok {
		expUpdater.SetEffectExpiration(target.creature.ID, "PSSHLD", timeNow().Unix()+interval)
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 벽부선을 던지며 지방호 주문을 외웠습니다.\n땅에서 오행중 토의 수호령들이 올라와 그의 주위에\n진을 형성했습니다.\n")
		if how == howCast {
			ctx.WriteString("\n당신은 벽부선을 던지며 지방호 주문을 외웁니다.\n땅에서 오행중 토의 수호령들이 올라와 당신주위에\n진을 형성합니다.\n")
			if err := magicEffectDeductMPLegacy(world, actor, 12); err != nil {
				return false, err
			}
			if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
				ctx.WriteString("\n이 방의 기운이 당신의 주술력을 강화시킵니다.\n")
			}
		} else {
			ctx.WriteString("\n땅에서 오행중 토의 수호령들이 올라와 당신주위에\n진을 형성합니다.\n")
		}
		return true, nil
	}

	targetName := attackCreatureName(target.creature)
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+targetName+"에게 토흙을 뿌리며 지방호 주문을 외웁니다.\n땅에서 오행중 토의 수호령들이 올라와 그의 주위에\n진을 형성합니다.\n")
	_ = sendToPlayer(ctx, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 당신에게 토흙을 뿌리며 지방호 주문을 외웠습니다.\n갑자기 땅에서 오행중 토의 수호령들이 올라와 당신주위에\n진을 형성했습니다.\n")
	if how == howCast {
		ctx.WriteString("\n당신은 " + targetName + "에게 땅방패 주문을 외웠습니다.\n")
		if err := magicEffectDeductMPLegacy(world, actor, 12); err != nil {
			return false, err
		}
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			ctx.WriteString("\n이 방의 기운이 당신의 주문을 강화시킵니다.\n")
		}
	} else {
		ctx.WriteString("\n땅에서 오행중 토의 수호령들이 올라와 " + targetName + "의\n주위에 진을 형성합니다.\n")
	}
	return true, nil
}

func magicEffectLevitate(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 10 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SLEVIT", "levitateSpell") {
			ctx.WriteString("\n당신은 아직 그런 주술을 터득하지 못했습니다.\n")
			return false, nil
		}
	}
	if failed, err := magicEffectSpellFail(world, actor, how, 10); failed || err != nil {
		return false, err
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
			ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
			return false, nil
		}
		target, ok = magicEffectResolveRoomPlayerTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}
	if !ok || target.creature.ID.IsZero() || (!isSelf && !target.hasPlayer) {
		ctx.WriteString("\n그런 사람은 존재하지 않습니다.\n")
		return false, nil
	}

	interval := int64(1200)
	if how == howCast {
		interval = int64(2400 + legacyStatBonus(creatureStat(actor, "intelligence"))*600)
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			interval += 800
		}
	}

	if err := magicEffectUpdateTags(world, target, []string{"levitate", "PLEVIT"}, nil); err != nil {
		return false, err
	}
	if expUpdater, ok := world.(magicEffectExpirationWorld); ok {
		expUpdater.SetEffectExpiration(target.creature.ID, "PLEVIT", timeNow().Unix()+interval)
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 기공을 끌어 모으며 부양술의 주문을 외우자\n몸이 살짝 떠오릅니다.")
		if how == howCast {
			ctx.WriteString("\n당신은 기공을 모으며 부양술의 주문을 외우자 몸이\n살짝 떠오릅니다.\n")
			if err := magicEffectDeductMPLegacy(world, actor, 10); err != nil {
				return false, err
			}
			if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
				ctx.WriteString("\n이 방의 기운이 당신의 주문을 강화시킵니다.\n")
			}
		} else {
			ctx.WriteString("\n당신의 몸이 공중으로 살짝 떠오릅니다.\n")
		}
		return true, nil
	}

	targetName := attackCreatureName(target.creature)
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+targetName+"에게 부양부적을 붙히며 주문을 외웁니다.\n주문을 외우자 그의 몸이 살짝 떠오릅니다.\n")
	_ = sendToPlayer(ctx, target.player.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 당신에게 부양부적을 붙히며 주문을 외웁니다.\n당신의 몸이 살짝 떠오르기 시작 합니다.\n")
	if how == howCast {
		ctx.WriteString("\n당신은 " + targetName + "에게 부양술을 걸 수 없습니다.\n")
		if err := magicEffectDeductMPLegacy(world, actor, 10); err != nil {
			return false, err
		}
		if room, ok := world.Room(actor.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			ctx.WriteString("\n이 방의 기운이 당신의 주문을 강화시킵니다.\n")
		}
	} else {
		ctx.WriteString(targetName + krtext.Particle(targetName, '1') + " 떠다니기 시작합니다.\n")
	}
	return true, nil
}

func magicEffectSilence(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 12 {
			ctx.WriteString("당신의 도력이 부족합니다.")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SSILNC", "silenceSpell") {
			ctx.WriteString("\n당신은 아직 그런 주문을 터득하지 못했습니다.\n")
			return false, nil
		}
	}
	if creatureClass(actor) < model.ClassSubDM {
		ctx.WriteString("그 주문을 펼치기엔 당신의 능력이 부족합니다.")
		return false, nil
	}

	dur := 300 + mrand(1, 15)*10
	if how == howCast {
		dur = 3600
	} else if how == howScroll {
		dur = 300 + mrand(1, 15)*10 + legacyStatBonus(creatureStat(actor, "intelligence"))*75
	}

	if err := magicEffectPrepayCastMP(world, actor, how, 12); err != nil {
		return false, err
	}
	if spellFail(actor) {
		return false, nil
	}

	isSelf := false
	targetArg := getArg(resolved, 1)
	var target magicEffectTarget
	var ok bool
	targetFromArg := false
	if targetArg == "" {
		isSelf = true
		target, ok = magicEffectSelfTarget(ctx, world, actor)
	} else {
		if how == howPotion {
			ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
			return false, nil
		}
		targetFromArg = true
		target, ok = magicEffectResolveMonsterFirstTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}
	if !ok || target.creature.ID.IsZero() || (targetFromArg && magicEffectRejectPlayerFallbackTarget(actor, targetArg, target)) {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다.\n")
		return false, nil
	}
	if how == howPotion && !isSelf {
		ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		if creatureHasAnyFlag(target.creature, "PRMAGI", "resistMagic") {
			dur /= 2
		}
		if err := magicEffectUpdateTags(world, target, []string{"silenced", "PSILNC"}, nil); err != nil {
			return false, err
		}
		magicEffectSetCooldown(world, target.creature.ID, "silenced", dur)
		if how == howPotion {
			ctx.WriteString("\n당신이 먹은것이 목에 걸려 목소리가 나오지 않습니다.\n")
			return true, nil
		}
		ctx.WriteString("\n당신은 실수로 봉합구 주문을 자신에게 걸었습니다.\n억... 당신은 입을 벌려 말을 하려 하지만 목소리가\n사라졌습니다.\n")
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 실수로 봉합구 주문을 자신에게 걸었습니다.\n그는 입을 벌려 말을 하려 하지만 목소리가 들리지 않습니다.\n")
		return true, nil
	}

	if target.creature.Kind != model.CreatureKindPlayer && creatureHasAnyFlag(target.creature, "MUNKIL", "unkillable", "cannotKill") {
		ctx.WriteString(fmt.Sprintf("\n당신은 %s에게 주술을 걸 수 없습니다.\n", getCreaturePronoun(target.creature)))
		return false, nil
	}
	if creatureHasAnyFlag(target.creature, "PRMAGI", "MRMAGI", "resistMagic") {
		dur /= 2
	}

	tags := []string{"silenced", "MSILNC"}
	if target.hasPlayer {
		tags = []string{"silenced", "PSILNC"}
	}
	if err := magicEffectUpdateTags(world, target, tags, nil); err != nil {
		return false, err
	}
	magicEffectSetCooldown(world, target.creature.ID, "silenced", dur)

	targetName := attackCreatureName(target.creature)
	ctx.WriteString(fmt.Sprintf("\n당신은 잽싸게 쫓아가 %s의 목을 치면서 \n봉합구 주문을 외웁니다.\n그는 입을 벌려 말을 하려 하지만 목소리가 들이지 않습니다.\n", targetName))
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, fmt.Sprintf("\n%s%s 잽싸게 쫓아가 %s의 목을 치면서 \n봉합구 주문을 외웁니다.\n그는 입을 벌려 말을 하려 하지만 목소리가 들이지 않습니다.\n", actorName, krtext.Particle(actorName, '1'), targetName))
	if target.hasPlayer {
		_ = sendToPlayer(ctx, target.player.ID, fmt.Sprintf("\n%s%s 잽싸게 쫓아와 당신의 목을 치면서 봉합구\n주문을 외웁니다.\n당신은 입을 벌려 말을 하려 하지만 목소리가 들이지 않습니다.\n", actorName, krtext.Particle(actorName, '1')))
	}
	return true, nil
}

func magicEffectBlind(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 15 {
			ctx.WriteString("당신의 도력이 부족합니다")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SBLIND", "blindSpell") {
			ctx.WriteString("당신은 아직 그런 주문을 터득하지 못했습니다.")
			return false, nil
		}
	}
	if creatureClass(actor) < model.ClassSubDM {
		ctx.WriteString("당신은 사용할 권한이 없는 주문입니다.")
		return false, nil
	}

	if err := magicEffectPrepayCastMP(world, actor, how, 15); err != nil {
		return false, err
	}
	if spellFail(actor) {
		return false, nil
	}

	isSelf := false
	targetArg := getArg(resolved, 1)
	var target magicEffectTarget
	var ok bool
	targetFromArg := false
	if targetArg == "" {
		isSelf = true
		target, ok = magicEffectSelfTarget(ctx, world, actor)
	} else {
		if how == howPotion {
			ctx.WriteString("그 물건은 자신에게만 사용할수 있습니다.")
			return false, nil
		}
		targetFromArg = true
		target, ok = magicEffectResolveMonsterFirstTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}
	if !ok || target.creature.ID.IsZero() || (targetFromArg && magicEffectRejectPlayerFallbackTarget(actor, targetArg, target)) {
		ctx.WriteString("그런 사람이 존재하지 않습니다.")
		return false, nil
	}
	if how == howPotion && !isSelf {
		ctx.WriteString("그 물건은 자신에게만 사용할수 있습니다.")
		return false, nil
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		if err := magicEffectUpdateTags(world, target, []string{"blind", "PBLIND"}, nil); err != nil {
			return false, err
		}
		if how == howPotion {
			ctx.WriteString("갑자기 당신의 눈이 감기더니 눈이 떠지질 않습니다.")
			return true, nil
		}
		ctx.WriteString("당신은 두손가락을 독수리 발톱모양을 하고서 실명\n주문을 걸었습니다.\n손가락에서 검은안개같은 기운이 나와 당신의 눈을 찌릅니다.\n악~~~ 내눈.. 당신의 눈이 떠지질 않습니다.")
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 두손가락을 독수리 발톱모양을 하고서 실명\n주문을 걸었습니다.\n손가락에서 검은안개같은 기운이 그의 눈을 찌르자\n괴성을 지릅니다. 악~~ 내눈..")
		return true, nil
	}

	if target.creature.Kind != model.CreatureKindPlayer && creatureHasAnyFlag(target.creature, "MUNKIL", "unkillable", "cannotKill") {
		ctx.WriteString(fmt.Sprintf("당신은 %s에게 주술을 걸 수 없습니다.", getCreaturePronoun(target.creature)))
		return false, nil
	}

	tags := []string{"blind", "MBLIND"}
	if target.hasPlayer {
		tags = []string{"blind", "PBLIND"}
	}
	if err := magicEffectUpdateTags(world, target, tags, nil); err != nil {
		return false, err
	}

	targetName := attackCreatureName(target.creature)
	ctx.WriteString(fmt.Sprintf("당신의 손가락을 %s의 눈을 향하고서 실명 \n주문를 외웁니다.\n검은안개같은 기운이 손가락에서 나와 그의 눈을\n찌르자 괴성을 지릅니다. 악~~ 내눈..\n", targetName))
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, fmt.Sprintf("\n%s%s 손가락을 %s의 눈을 향하고서 실명\n주문를 외웠습니다.\n검은안개같은 기운이 손가락에서 나와 그의 눈을 \n찌르자 괴성을 지릅니다. 악~~ 내눈..\n", actorName, krtext.Particle(actorName, '1'), targetName))
	if target.hasPlayer {
		_ = sendToPlayer(ctx, target.player.ID, fmt.Sprintf("\n%s%s 손가락을 당신의 눈을 향하고서 실명 주문를 외웁니다.\n검은안개같은 기운이 손가락에서 나와 당신의 눈을\n 찌르자 괴성을 지릅니다. 악~~ 내눈..\n당신의 앞이 눈이 감겨서 보이질 않습니다.\n", actorName, krtext.Particle(actorName, '1')))
	}
	return true, nil
}

func magicEffectFear(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 15 {
			ctx.WriteString("당신의 도력이 부족합니다.")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SFEARS", "fearSpell") {
			ctx.WriteString("당신은 아직 그런 주문을 터득하지 못했습니다.")
			return false, nil
		}
	}

	dur := 600 + mrand(1, 30)*10
	if how == howCast {
		dur = 600 + mrand(1, 30)*10 + legacyStatBonus(creatureStat(actor, "intelligence"))*150
	} else if how == howScroll {
		dur = 600 + mrand(1, 15)*10 + legacyStatBonus(creatureStat(actor, "intelligence"))*50
	}

	if err := magicEffectPrepayCastMP(world, actor, how, 15); err != nil {
		return false, err
	}
	if spellFail(actor) {
		return false, nil
	}

	isSelf := false
	targetArg := getArg(resolved, 1)
	var target magicEffectTarget
	var ok bool
	targetFromArg := false
	if targetArg == "" {
		isSelf = true
		target, ok = magicEffectSelfTarget(ctx, world, actor)
	} else {
		if how == howPotion {
			ctx.WriteString("그 물건은 자신에게만 사용할수 있습니다.")
			return false, nil
		}
		targetFromArg = true
		target, ok = magicEffectResolveMonsterFirstTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}
	if !ok || target.creature.ID.IsZero() || (targetFromArg && magicEffectRejectPlayerFallbackTarget(actor, targetArg, target)) {
		ctx.WriteString("그런 사람이 존재 하지 않습니다.")
		return false, nil
	}
	if how == howPotion && !isSelf {
		ctx.WriteString("그 물건은 자신에게만 사용할수 있습니다.")
		return false, nil
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		if creatureHasAnyFlag(target.creature, "PRMAGI", "resistMagic") {
			dur /= 2
		}
		if err := magicEffectUpdateTags(world, target, []string{"fearful", "PFEARS"}, nil); err != nil {
			return false, err
		}
		magicEffectSetCooldown(world, target.creature.ID, "fearful", dur)

		if how == howPotion {
			ctx.WriteString("갑자기 당신이 무서워하던 것들이 나타나 당신을 둘러쌉니다.\n악~~~ 저리가~~ 당신은 공포에 떨기 시작합니다.")
			return true, nil
		}
		ctx.WriteString("당신은 실수로 지옥구슬을 떨어뜨렸습니다.\n갑자기 당신이 무서워하던 것들이 나타나 당신을 둘러쌉니다.\n악~~~ 저리가~~ 당신은 공포에 떨기 시작합니다.\n")
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 실수로 지옥구슬을 떨어뜨렸습니다.\n갑자기 구슬이 펑하고 터지더니 갑자기 그가 괴성을\n지르기 시작합니다. 악~~~ 저리가~~\n그가 공포에 떨기 시작하는데 당신의 눈에는 아무것도 보이지 않습니다.")
		return true, nil
	}

	if target.creature.Kind != model.CreatureKindPlayer && creatureHasAnyFlag(target.creature, "MUNKIL", "unkillable", "cannotKill") {
		ctx.WriteString(fmt.Sprintf("당신은 %s에게 주술을 걸 수 없습니다.", getCreaturePronoun(target.creature)))
		return false, nil
	}
	targetName := attackCreatureName(target.creature)
	if target.creature.Kind != model.CreatureKindPlayer && creatureHasAnyFlag(target.creature, "MPERMT", "permanent") {
		ctx.WriteString(fmt.Sprintf("%s의 주위에 공포의 기운이 둘러쌉니다.\n하지만, 그가 기합을 지르자 금새 그 기운이 사라졌습니다.", targetName))
		return false, nil
	}
	if creatureHasAnyFlag(target.creature, "PRMAGI", "MRMAGI", "resistMagic") {
		dur /= 2
	}

	tags := []string{"fearful", "MFEARS"}
	if target.hasPlayer {
		tags = []string{"fearful", "PFEARS"}
	}
	if err := magicEffectUpdateTags(world, target, tags, nil); err != nil {
		return false, err
	}
	magicEffectSetCooldown(world, target.creature.ID, "fearful", dur)

	ctx.WriteString(fmt.Sprintf("당신은 지옥구술을 %s에게 던졌습니다.\n구슬이 펑하고 터지면서 공포의 기운이 그를 둘러쌉니다.\n악~~~ 저리가~~ 갑자기 그가 괴성을 지르면서 공포에\n떨기 시작합니다.\n", targetName))
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, fmt.Sprintf("\n%s%s %s에게 지옥구술을 던졌습니다.\n구슬이 펑하고 터지자 갑자기 그가 괴성을 지릅니다. 악~~~ 저리가~~\n그는 공포에 떨지만 당신의 눈에는 아무것도 보이지 않습니다.\n", actorName, krtext.Particle(actorName, '1'), targetName))
	if target.hasPlayer {
		_ = sendToPlayer(ctx, target.player.ID, fmt.Sprintf("\n%s%s 당신에게 지옥구슬을 던졌습니다.\n갑자기 당신이 무서워하던 것들이 나타나 당신을 둘러쌉니다.\n\"악~~~ 저리가~~\" 당신은 괴성을 지르며 공포에 떨기\n시작합니다.\n", actorName, krtext.Particle(actorName, '1')))
	}
	return true, nil
}

func magicEffectRemoveDisease(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 12 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		class := creatureClass(actor)
		if class != model.ClassCleric && class < model.ClassInvincible {
			ctx.WriteString("\n이 주술은 불제자만이 사용할 수 있습니다.\n")
			return false, nil
		}
		if class >= model.ClassInvincible && !creatureHasAnyFlag(actor, "SCLERIC", "clericTraining", "clericSpell", "clericMode") {
			ctx.WriteString("\n불제자를 무적수련하지 않았습니다..\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SRMDIS", "removeDiseaseSpell") {
			ctx.WriteString("\n당신은 아직 그런 주문을 터득하지 못했습니다.\n")
			return false, nil
		}
	}
	if spellFail(actor) {
		if how == howCast {
			if err := magicEffectConsumeMP(world, actor, 12); err != nil {
				return false, err
			}
		}
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
		ctx.WriteString("\n그런사람이 존재하지 않습니다 .\n")
		return false, nil
	}
	if how == howPotion && !isSelf {
		ctx.WriteString("\n이 물건은 자신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	wasDiseased := magicEffectTargetHasAnyFlag(target, "disease", "diseased", "PDISEA", "MDISEA")
	remove := []string{"disease", "diseased", "PDISEA", "MDISEA"}
	if err := magicEffectUpdateTags(world, target, nil, remove); err != nil {
		return false, err
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		if how == howCast {
			if err := magicEffectDeductMPLegacy(world, actor, 12); err != nil {
				return false, err
			}
		}
		if how == howPotion {
			if wasDiseased {
				ctx.WriteString("\n병마에 시달리던 당신의 몸이 활기를 띄기 시작합니다.\n")
			} else {
				ctx.WriteString("\n당신의 병이 해소되어 몸이 거뜬해집니다.\n")
			}
			return true, nil
		}
		ctx.WriteString("\n당신은 생사과를 먹으며 치료 주문을 외웁니다.\n병마에 시달리던 당신의 몸이 활기를 띄기 시작합니다.\n")
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 생사과를 먹으며 치료 주문을 외웠습니다.\n병마에 시달리던 그의 몸이 활기를 띄기 시작합니다.\n")
		return true, nil
	}

	targetName := attackCreatureName(target.creature)
	if how == howCast {
		if err := magicEffectDeductMPLegacy(world, actor, 12); err != nil {
			return false, err
		}
	}
	ctx.WriteString(fmt.Sprintf("\n당신은 %s의 혈도를 누르고 내공의 힘을 통해\n치료를 시작합니다.\n그의 몸에 막혀 있던 혈이 풀리면서 차츰 활기를 \n띄기 시작합니다.\n", targetName))
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, fmt.Sprintf("\n%s%s %s의 혈도를 누르고 내공의 힘을 통해\n치료를 시작합니다.\n그의 몸이 차츰 활기를 띄기 시작하는 것이 보입니다.\n", actorName, krtext.Particle(actorName, '1'), targetName))
	if target.hasPlayer {
		_ = sendToPlayer(ctx, target.player.ID, fmt.Sprintf("\n%s%s 당신의 혈도를 누르고 내공의 힘을 통해 치료를 시작합니다.\n당신의 몸에 기공이 들어와 막힌 혈을 풀자 차츰 \n활기를 띄기 시작합니다.\n", actorName, krtext.Particle(actorName, '1')))
	}
	return true, nil
}

func magicEffectRemoveBlindness(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	how := determineHow(world, object)
	if how == howCast {
		if creatureStat(actor, "mpCurrent") < 12 {
			ctx.WriteString("당신의 도력이 부족합니다")
			return false, nil
		}
		class := creatureClass(actor)
		if class != model.ClassCleric && class != model.ClassPaladin && class < model.ClassInvincible {
			ctx.WriteString("이 기술은 불제자와 무사만이 사용할 수 있습니다.")
			return false, nil
		}
		if class >= model.ClassInvincible &&
			!creatureHasAnyFlag(actor, "SCLERIC", "clericTraining", "clericSpell", "clericMode") &&
			!creatureHasAnyFlag(actor, "SPALADIN", "paladinTraining", "paladinSpell", "paladinMode") {
			ctx.WriteString("\n불제자나 무사를 무적수련하지 않았습니다..\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SRMBLD", "removeBlindnessSpell") {
			ctx.WriteString("당신은 아직 그런 주문을 터득하지 못했습니다.")
			return false, nil
		}
	}
	if spellFail(actor) {
		if how == howCast {
			if err := magicEffectConsumeMP(world, actor, 12); err != nil {
				return false, err
			}
		}
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
			ctx.WriteString("이 물건은 자신에게만 사용할수 있습니다.")
			return false, nil
		}
		target, ok = magicEffectResolveRoomPlayerThenMonsterTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	}
	if !ok || target.creature.ID.IsZero() {
		ctx.WriteString("그런 사람이 존재하지 않습니다.")
		return false, nil
	}
	if how == howPotion && !isSelf {
		ctx.WriteString("이 물건은 자신에게만 사용할수 있습니다.")
		return false, nil
	}

	wasBlind := magicEffectTargetHasAnyFlag(target, "blind", "blinded", "PBLIND", "MBLIND")
	remove := []string{"blind", "blinded", "PBLIND", "MBLIND"}
	if err := magicEffectUpdateTags(world, target, nil, remove); err != nil {
		return false, err
	}

	actorName := attackCreatureName(actor)
	if isSelf {
		if how == howCast {
			if err := magicEffectDeductMPLegacy(world, actor, 12); err != nil {
				return false, err
			}
		}
		if how == howPotion {
			if wasBlind {
				ctx.WriteString("감겼던 눈이 움찔거리다가 갑자기 눈앞이 밝아집니다.")
			} else {
				ctx.WriteString("약을 먹자 당신 눈에 걸린 주술이 스르르 풀리는 것을 느낍니다.")
			}
			return true, nil
		}
		ctx.WriteString("당신의 이마에 개안부를 붙히며 개안술 주문을 외웁니다.\n감겼던 눈이 움찔거리다가 갑자기 눈앞이 밝아집니다.")
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 자신의 이마에 개안부를 붙히며 개안술 \n주문을 외웁니다.\n감겼던 그의 눈이 움찔거리다가 갑자기 눈을 확 뜹니다.")
		return true, nil
	}

	targetName := attackCreatureName(target.creature)
	if how == howCast {
		if err := magicEffectDeductMPLegacy(world, actor, 12); err != nil {
			return false, err
		}
	}
	ctx.WriteString(fmt.Sprintf("당신은 %s의 이마에 개안부를 붙히고서 주문을 \n외웁니다.\n그의 감겼던 눈이 움찔거리다가 갑자기 확 뜹니다.", targetName))
	_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, fmt.Sprintf("\n%s%s %s의 이마에 개안부를 붙히고서 \n주문을 외웁니다.\n그의 감겼던 눈이 움찔거리다가 갑자기 확 뜹니다.\n", actorName, krtext.Particle(actorName, '1'), targetName))
	if target.hasPlayer {
		_ = sendToPlayer(ctx, target.player.ID, fmt.Sprintf("%s%s 당신의 이마에 개안부를 붙히고서 주문을\n외웁니다.\n감겼던 당신의 눈이 움찔거리다가 갑자기 밝아집니다.\n", actorName, krtext.Particle(actorName, '1')))
	}
	return true, nil
}

func magicEffectDamageDiceForPower(magicPower int) (magicEffectDamageDice, bool) {
	osp, ok := ospellTable[magicPower]
	if !ok {
		return magicEffectDamageDice{}, false
	}
	return magicEffectDamageDice{nDice: osp.nDice, sDice: osp.sDice, pDice: osp.pDice}, true
}

func magicEffectApplyDamage(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
	magicPower int,
	dice magicEffectDamageDice,
) (bool, error) {
	how := determineHow(world, object)
	targetArg := strings.TrimSpace(getArg(resolved, 1))
	if targetArg != "" && targetArg != "나" && how == howPotion {
		ctx.WriteString("\n당신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	target, ok := magicEffectResolveTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	if !ok || target.creature.ID.IsZero() {
		ctx.WriteString("\n그런 것은 여기에 존재하지 않습니다.\n")
		return false, nil
	}
	damageWorld, ok := world.(magicCreatureDamageWorld)
	if !ok {
		return false, nil
	}
	damage := rollDice(dice.nDice, dice.sDice, dice.pDice)
	if damage < 1 {
		damage = 1
	}

	damage = ApplyElementalResistance(target.creature, magicPower, damage)

	if _, _, _, err := damageWorld.ApplyCreatureDamage(target.creature.ID, damage); err != nil {
		return false, err
	}

	if target.creature.Kind == model.CreatureKindMonster {
		RegisterSpellAggro(world, target.creature.ID, actor.ID)
	}

	return true, nil
}

func magicEffectLight(ctx *Context, world StatusWorld, actor model.Creature, object model.ObjectInstance) (bool, error) {
	how := determineHow(world, object)
	if spellFail(actor) {
		if how == howCast {
			if err := magicEffectConsumeMP(world, actor, 5); err != nil {
				return false, err
			}
		}
		return false, nil
	}

	target, ok := magicEffectSelfTarget(ctx, world, actor)
	if !ok || target.creature.ID.IsZero() {
		return false, nil
	}
	if err := magicEffectUpdateTags(world, target, []string{"PLIGHT"}, nil); err != nil {
		return false, err
	}
	if expirer, ok := world.(magicEffectExpirationWorld); ok {
		expirer.SetEffectExpiration(target.creature.ID, "PLIGHT", timeNow().Unix()+magicLightDurationSeconds)
	}

	ctx.WriteString("당신의 왼손에 발광 주문을 걸었습니다.\n왼손에서 황금빛이 뿜어져 나와 주위를 밝혀 줍니다.\n")
	actorName := attackCreatureName(actor)
	if actorName == "" {
		actorName = string(actor.ID)
	}
	roomID := actor.RoomID
	if roomID.IsZero() {
		roomID = target.creature.RoomID
	}
	if err := roomBroadcast(ctx, roomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 한쪽 손에 발광 주문을 걸었습니다.\n그의 손에서 황금색의 찬란한 빛이 뿜어져 나옵니다.\n"); err != nil {
		return false, err
	}
	return true, nil
}

func magicEffectResolveTarget(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	target string,
	ordinal int64,
) (magicEffectTarget, bool) {
	target = strings.TrimSpace(target)
	if target == "" || target == "나" {
		return magicEffectSelfTarget(ctx, world, actor)
	}

	viewer := LookViewer{
		PlayerID:   InventoryPlayerIDFromContext(ctx),
		CreatureID: actor.ID,
	}
	if attackTargetMatchesSelf(world, viewer, actor, target) {
		return magicEffectSelfTarget(ctx, world, actor)
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
	if player, creature, ok := findLegacyPlayerCreatureTarget(world, room, viewer, legacyUpperFirstASCII(target), ordinal); ok {
		return magicEffectTarget{creature: creature, player: player, hasPlayer: true}, true
	}
	if creature, ok := findAttackCreatureTarget(world, room, viewer, target, ordinal); ok {
		return magicEffectTarget{creature: creature}, true
	}
	return magicEffectTarget{}, false
}

func magicEffectSelfTarget(ctx *Context, world StatusWorld, actor model.Creature) (magicEffectTarget, bool) {
	playerID := actor.PlayerID
	if playerID.IsZero() {
		playerID = InventoryPlayerIDFromContext(ctx)
	}
	if playerID.IsZero() {
		return magicEffectTarget{creature: actor}, true
	}
	player, ok := world.Player(playerID)
	if !ok {
		return magicEffectTarget{creature: actor}, true
	}
	return magicEffectTarget{creature: actor, player: player, hasPlayer: true}, true
}

func magicEffectHealCreature(world StatusWorld, target magicEffectTarget, amount int, full bool) (bool, error) {
	if target.creature.ID.IsZero() {
		return false, nil
	}
	if target.creature.Stats == nil {
		return true, nil
	}
	current, ok := target.creature.Stats["hpCurrent"]
	if !ok {
		return true, nil
	}
	maxHP, ok := target.creature.Stats["hpMax"]
	if !ok || maxHP < 1 {
		return true, nil
	}

	next := current
	if full {
		if current >= maxHP {
			return false, nil
		}
		next = maxHP
	} else {
		if amount < 1 {
			amount = 1
		}
		next = current + amount
		if next > maxHP {
			next = maxHP
		}
	}

	updater, ok := world.(magicCreatureStatWorld)
	if !ok {
		return true, nil
	}
	if err := updater.SetCreatureStat(target.creature.ID, "hpCurrent", next); err != nil {
		return false, err
	}
	return true, nil
}

func magicEffectUpdateTags(world StatusWorld, target magicEffectTarget, add []string, remove []string) error {
	if !target.creature.ID.IsZero() {
		updater, ok := world.(interface {
			UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
		})
		if !ok {
			return nil
		}
		if _, err := updater.UpdateCreatureTags(target.creature.ID, add, remove); err != nil {
			return err
		}
	}
	if target.hasPlayer {
		updater, ok := world.(interface {
			UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
		})
		if !ok {
			return nil
		}
		if _, err := updater.UpdatePlayerTags(target.player.ID, add, remove); err != nil {
			return err
		}
	}
	return nil
}

func magicEffectTargetHasAnyFlag(target magicEffectTarget, names ...string) bool {
	if creatureHasAnyFlag(target.creature, names...) {
		return true
	}
	return target.hasPlayer && hasAnyNormalizedFlag(target.player.Metadata.Tags, names...)
}

func magicEffectSetCooldown(world StatusWorld, creatureID model.CreatureID, key string, seconds int) {
	if creatureID.IsZero() || seconds <= 0 {
		return
	}
	updater, ok := world.(interface {
		SetCreatureCooldown(model.CreatureID, string, int64, int64) error
	})
	if !ok {
		return
	}
	_ = updater.SetCreatureCooldown(creatureID, key, timeNow().Unix(), int64(seconds))
}
