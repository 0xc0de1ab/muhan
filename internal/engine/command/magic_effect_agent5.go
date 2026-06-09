package command

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/persist/legacykr"
	"muhan/internal/world/model"
)

// LegacyDataRootOverride overrides the default data root directory lookup in tests.
var LegacyDataRootOverride string

// ApplyMagicPowerEffectAgent5 dispatches Agent 5's spell effects.
// Returns (handled, success, error).
func ApplyMagicPowerEffectAgent5(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
	magicPower int,
) (bool, bool, error) {
	switch magicPower {
	case magicPowerMagicTrack:
		success, err := magicEffectMagicTrack(ctx, world, actor, object, resolved)
		return true, success, err
	case magicPowerLocatePlayer:
		success, err := magicEffectLocatePlayer(ctx, world, actor, object, resolved)
		return true, success, err
	default:
		return false, false, nil
	}
}

func magicEffectMagicTrack(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	if actor.ID.IsZero() {
		return false, nil
	}

	how := determineHow(world, object)
	if how == howCast {
		class := creatureClass(actor)
		if class != legacyClassRanger && class < legacyClassInvincible {
			ctx.WriteString("\n포졸만이 이 주술을 사용할 수 있습니다.\n")
			return false, nil
		}
		if class >= legacyClassInvincible && !creatureHasAnyFlag(actor, "SRANGER", "rangerSpell", "rangerTraining", "rangerMode") {
			ctx.WriteString("\n포졸을 무적수련하지 않았습니다..\n")
			return false, nil
		}
		if creatureStat(actor, "mpCurrent") < 13 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "STRACK", "trackSpell") {
			ctx.WriteString("\n당신은 아직 그런 주술을 터득하지 못했습니다.\n")
			return false, nil
		}
	}

	targetArg := getArg(resolved, 1)
	targetArg = strings.TrimSpace(targetArg)
	if targetArg == "" {
		ctx.WriteString("\n당신 자신을 추적한다고요?.\n")
		return false, nil
	}
	if how == howPotion {
		ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	playerID := InventoryPlayerIDFromContext(ctx)
	if playerID.IsZero() {
		playerID = model.PlayerID(actor.ID)
	}

	player, hasPlayer := world.Player(playerID)
	if !hasPlayer {
		ctx.WriteString("\n그런 사람은 존재하지 않습니다.\n")
		return false, nil
	}

	// 1. Look up target player in the world
	targetPlayer, ok := findGlobalPlayer(ctx, world, targetArg)
	if !ok {
		ctx.WriteString("\n그런 사람은 존재하지 않습니다.\n")
		return false, nil
	}

	targetCreature, targetHasCreature := world.Creature(targetPlayer.CreatureID)
	if !targetHasCreature {
		ctx.WriteString("\n그런 사람은 존재하지 않습니다.\n")
		return false, nil
	}

	// Reject self or DM-invisible targets like C's find_who branch.
	if targetPlayer.ID == player.ID || targetCreature.ID == actor.ID ||
		creatureHasAnyFlag(targetCreature, "dmInvisible", "pdminv", "PDMINV") {
		ctx.WriteString("\n그런 사람은 존재하지 않습니다.\n")
		return false, nil
	}
	if creatureClass(targetCreature) > legacyClassCaretaker {
		ctx.WriteString("\n그 사람이 어디에 있는지 추적할수 없습니다.\n")
		return false, nil
	}

	// Reject if target has marriage tag and caster is not spouse
	if isPlayerMarried(targetPlayer, targetCreature) {
		targetName := strings.TrimSpace(targetCreature.DisplayName)
		if targetName == "" {
			targetName = strings.TrimPrefix(string(targetPlayer.ID), "player:")
		}
		spouseName, err := readSpouseNameForPlayer(player.ID)
		if err != nil || !strings.EqualFold(spouseName, targetName) {
			ctx.WriteString("\n그 사람이 어디에 있는지 추적할 수 없습니다.\n")
			return false, nil
		}
	}

	// 2. Verify target's room blocks
	targetRoom, roomFound := world.Room(targetCreature.RoomID)
	if !roomFound {
		ctx.WriteString("\n주문이 실패했습니다.\n")
		return false, nil
	}

	// noTeleport check
	if roomHasAnyFlag(targetRoom, "noTeleport", "rnotel") {
		ctx.WriteString("\n주문이 실패했습니다.\n")
		return false, nil
	}

	// Occupant limit checks
	n := magicEffectVisiblePlayerCount(world, targetRoom)
	if (roomHasAnyFlag(targetRoom, "onePlayer", "ronepl") && n > 0) ||
		(roomHasAnyFlag(targetRoom, "twoPlayers", "rtwopl") && n > 1) ||
		(roomHasAnyFlag(targetRoom, "threePlayers", "rthree") && n > 2) {
		ctx.WriteString("\n주문이 실패했습니다.\n")
		return false, nil
	}

	// family check (RFAMIL)
	if roomHasAnyFlag(targetRoom, "family", "rfamil") {
		if !creatureHasAnyFlag(actor, "PFAMIL", "family") {
			ctx.WriteString("그 사람이 있는 곳으로 갈 수가 없습니다.")
			return false, nil
		}
	}

	// onlyFamily check (RONFML)
	if roomHasAnyFlag(targetRoom, "onlyFamily", "familyOnly", "ronfml") {
		special, ok := roomPropertyValue(targetRoom, "special")
		if !ok {
			ctx.WriteString("그 사람이 있는 곳으로 갈 수가 없습니다.")
			return false, nil
		}
		val, ok := moveCreatureStatOrPropertyValue(actor, "familyID", "dailyExpndMax", "legacyDailyExpndMax")
		if !ok || !moveRestrictionValuesEqual(val, special) {
			ctx.WriteString("그 사람이 있는 곳으로 갈 수가 없습니다.")
			return false, nil
		}
	}

	// Level limits check
	casterLevel := getCreatureLevel(actor)
	if minL, ok := roomMinLevel(targetRoom); ok && casterLevel < minL {
		ctx.WriteString("\n주문이 실패했습니다.\n")
		return false, nil
	}
	if maxL, ok := roomMaxLevel(targetRoom); ok && maxL > 0 && casterLevel > maxL {
		ctx.WriteString("\n주문이 실패했습니다.\n")
		return false, nil
	}

	// 3. Decrement daily limits if applicable. C evaluates dec_daily before
	// checking how==CAST, so scrolls and wands preserve the side effect.
	dailyOK, err := decrementDailyTrackLimit(world, actor)
	if err != nil {
		return false, err
	}
	if !dailyOK && how == howCast && creatureClass(actor) < legacyClassCaretaker {
		ctx.WriteString("\n당신의 몸이 너무 피곤해 이 주술을 더 이상 펼칠 수 없습니다.\n")
		return false, nil
	}

	if how == howCast {
		if err := magicEffectDeductMPLegacy(world, actor, 13); err != nil {
			return false, err
		}
	}

	// 4. Send success messages
	actorName := attackCreatureName(actor)
	targetName := attackCreatureName(targetCreature)
	broadcastMsg := fmt.Sprintf("\n%s%s %s의 흔적을 찾아내는데 성공하여 \n추적을 시작했습니다.\n", actorName, krtext.Particle(actorName, '1'), targetName)
	_ = roomBroadcast(ctx, actor.RoomID, broadcastMsg)

	targetMsg := fmt.Sprintf("\n%s%s 당신의 흔적을 찾아 내는데 성공하여 당신을 \n찾아 왔습니다.\n", actorName, krtext.Particle(actorName, '1'))
	_ = sendToPlayer(ctx, targetPlayer.ID, targetMsg)

	ctx.WriteString(fmt.Sprintf("\n!!당신은 %s의 흔적을 찾아내는데 성공했습니다.!!\n그를 추적하여 달려갑니다.\n", targetName))

	// 5. Teleport the caster to target's room
	if mover, ok := world.(magicMoveWorld); ok {
		if err := mover.MovePlayerToRoom(player.ID, targetCreature.RoomID); err != nil {
			return false, err
		}
	} else {
		return false, fmt.Errorf("world does not support MovePlayerToRoom")
	}

	return true, nil
}

var locatePlayerRandIntn = rand.Intn

func magicEffectLocatePlayer(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	if actor.ID.IsZero() {
		return false, nil
	}

	how := determineHow(world, object)
	if how == howCast {
		if !creatureHasAnyFlag(actor, "SLOCAT", "locatePlayerSpell") {
			ctx.WriteString("\n당신은 아직 그런 주문을 터득하지 못했습니다.\n")
			return false, nil
		}
		if creatureStat(actor, "mpCurrent") < 15 {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
	}

	targetArg := getArg(resolved, 1)
	targetArg = strings.TrimSpace(targetArg)
	if targetArg == "" {
		ctx.WriteString("\n누구와 연결합니까?\n")
		return false, nil
	}

	// 1. Look up target player in the world
	targetPlayer, ok := findGlobalPlayer(ctx, world, targetArg)
	if !ok {
		ctx.WriteString("\n그런 사람은 존재하지 않습니다.\n")
		return false, nil
	}

	targetCreature, targetHasCreature := world.Creature(targetPlayer.CreatureID)
	if !targetHasCreature {
		ctx.WriteString("\n그런 사람은 존재하지 않습니다.\n")
		return false, nil
	}

	// Reject if target is DM Invisible
	if creatureHasAnyFlag(targetCreature, "dmInvisible", "pdminv", "PDMINV") {
		ctx.WriteString("\n그런 사람은 존재하지 않습니다.\n")
		return false, nil
	}

	// Reject if target is invisible and caster cannot detect invisible
	if creatureHasAnyFlag(targetCreature, "invisible", "pinvis", "PINVIS") &&
		!creatureHasAnyFlag(actor, "detectInvisible", "pdinvi", "PDINVI") {
		ctx.WriteString("\n그런 사람은 존재하지 않습니다.\n")
		return false, nil
	}

	// C magic7.c only blocks exact DM targets from non-DM casters.
	targetClass := getCreatureClass(targetCreature)
	casterClass := getCreatureClass(actor)
	if targetClass == legacyClassDM && casterClass != legacyClassDM {
		ctx.WriteString("그 사람의 정신력이 너무 높아 투시를 할 수 없습니다.")
		return false, nil
	}

	// Reject if target has marriage tag
	if isPlayerMarried(targetPlayer, targetCreature) {
		ctx.WriteString("그 사람의 사생활은 엿볼 수가 없습니다.")
		return false, nil
	}

	if failed, err := magicEffectSpellFail(world, actor, how, 15); failed || err != nil {
		return false, err
	}

	actorName := attackCreatureName(actor)
	_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 천리안 주문을 외웠습니다.\n")
	if how == howCast {
		ctx.WriteString(fmt.Sprintf("\n당신의 마음을 %s에게 집중했습니다.\n", targetCreature.DisplayName))
		if err := magicEffectDeductMPLegacy(world, actor, 15); err != nil {
			return false, err
		}
	}

	casterLevel := getCreatureLevel(actor)
	targetLevel := getCreatureLevel(targetCreature)
	casterIntel := creatureStat(actor, "intelligence")
	targetIntel := creatureStat(targetCreature, "intelligence")

	// Calculate success probability based on levels and intelligence
	chance := 50 + (((casterLevel+3)/4)-((targetLevel+3)/4))*5 + (legacyStatBonus(casterIntel)-legacyStatBonus(targetIntel))*5
	if casterClass == 5 { // MAGE
		chance += 5
	}
	chance = minInt(85, chance)

	// Success roll
	if targetClass < legacyClassSubDM && locatePlayerRandIntn(100)+1 < chance {
		// Look up target room and render
		targetRoom, found := world.Room(targetCreature.RoomID)
		if !found {
			ctx.WriteString("\n당신의 정신은 연결될수 없습니다.\n")
			return true, nil
		}

		viewer := LookViewerFromContext(ctx)
		displayRoom := RenderRoomLook(world, targetRoom, viewer)
		ctx.WriteString(displayRoom)

		// Notify target player (chance-based check)
		targetChance := 60 + (((targetLevel+3)/4)-((casterLevel+3)/4))*5 + (legacyStatBonus(targetIntel)-legacyStatBonus(casterIntel))*5
		if targetClass == 5 { // MAGE
			targetChance += 5
		}
		targetChance = minInt(85, targetChance)

		if locatePlayerRandIntn(100)+1 < targetChance {
			targetMsg := fmt.Sprintf("\n%s%s 당신의 눈으로 주위를 보고 있습니다.\n", actor.DisplayName, krtext.Particle(actor.DisplayName, '1'))
			_ = sendToPlayer(ctx, targetPlayer.ID, targetMsg)
		}
	} else {
		// Failure
		ctx.WriteString("\n당신의 정신은 연결될수 없습니다.\n")

		targetChance := 65 + (((targetLevel+3)/4)-((casterLevel+3)/4))*5 + (legacyStatBonus(targetIntel)-legacyStatBonus(casterIntel))*5
		if locatePlayerRandIntn(100)+1 < targetChance {
			targetMsg := fmt.Sprintf("\n%s%s 당신의 눈으로 보려합니다.\n", actor.DisplayName, krtext.Particle(actor.DisplayName, '1'))
			_ = sendToPlayer(ctx, targetPlayer.ID, targetMsg)
		}
	}

	return true, nil
}

func findGlobalPlayer(ctx *Context, world StatusWorld, targetName string) (model.Player, bool) {
	player, _, _, ok := legacyFindWhoActivePlayer(ctx, world, targetName)
	return player, ok
}

func getCreatureClass(c model.Creature) int {
	return creatureClass(c)
}

func getCreatureLevel(c model.Creature) int {
	if c.Level > 0 {
		return c.Level
	}
	if v, ok := creatureStatValue(c, "level"); ok {
		return v
	}
	return 1
}

func isPlayerMarried(player model.Player, creature model.Creature) bool {
	for _, tag := range player.Metadata.Tags {
		t := strings.ToUpper(tag)
		if t == "PMARRI" || t == "MARRIED" || t == "MARRIAGE" {
			return true
		}
	}
	return creatureHasAnyFlag(creature, "PMARRI", "married", "marriage", "marriageFlag")
}

func findLegacyDataRoot() string {
	if LegacyDataRootOverride != "" {
		return LegacyDataRootOverride
	}
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for i := 0; i < 5; i++ {
		checkPath := filepath.Join(dir, "player", "marriage")
		if info, err := os.Stat(checkPath); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "/workspace/muhan"
}

func readSpouseNameForPlayer(playerID model.PlayerID) (string, error) {
	root := findLegacyDataRoot()
	cleanName := strings.TrimPrefix(string(playerID), "player:")

	encodedActorBytes, err := legacykr.EncodeEUCKR(cleanName)
	if err != nil {
		return "", err
	}
	encodedActorName := string(encodedActorBytes)

	path := filepath.Join(root, "player", "marriage", encodedActorName)
	data, err := os.ReadFile(path)
	if err != nil {
		lowerName := strings.ToLower(cleanName)
		lowerEncodedBytes, err := legacykr.EncodeEUCKR(lowerName)
		if err != nil {
			return "", err
		}
		lowerEncodedName := string(lowerEncodedBytes)
		lowerPath := filepath.Join(root, "player", "marriage", lowerEncodedName)
		data, err = os.ReadFile(lowerPath)
		if err != nil {
			return "", err
		}
	}
	return strings.TrimSpace(string(data)), nil
}

func decrementDailyTrackLimit(world StatusWorld, caster model.Creature) (bool, error) {
	level := getCreatureLevel(caster)
	maxUses := 10 + (((level+3)/4)-5)/4
	if maxUses < 10 {
		maxUses = 10
	}

	dailyMax := maxUses
	if valStr, ok := caster.Properties["dailyTrackMax"]; ok {
		if val, err := strconv.Atoi(valStr); err == nil {
			dailyMax = val
		}
	}

	now := time.Now()
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.UTC
	}
	now = now.In(loc)

	lastTime := now
	if ltimeStr, ok := caster.Properties["dailyTrackLTime"]; ok {
		if ltimeVal, err := strconv.ParseInt(ltimeStr, 10, 64); err == nil {
			lastTime = time.Unix(ltimeVal, 0).In(loc)
		}
	}

	dailyCur := dailyMax
	if valStr, ok := caster.Properties["dailyTrackCur"]; ok {
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

	if _, err := updater.SetCreatureProperty(caster.ID, "dailyTrackMax", strconv.Itoa(dailyMax)); err != nil {
		return false, err
	}
	if _, err := updater.SetCreatureProperty(caster.ID, "dailyTrackCur", strconv.Itoa(dailyCur)); err != nil {
		return false, err
	}
	if _, err := updater.SetCreatureProperty(caster.ID, "dailyTrackLTime", strconv.FormatInt(now.Unix(), 10)); err != nil {
		return false, err
	}

	return true, nil
}
