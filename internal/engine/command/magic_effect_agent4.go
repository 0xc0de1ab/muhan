package command

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"

	"muhan/internal/world/model"
)

// MagicEffectAgent4World defines the world interface for space-time spells.
type MagicEffectAgent4World interface {
	StatusWorld
	MovePlayerToRoom(model.PlayerID, model.RoomID) error
	AllRoomIDs() []model.RoomID
	SetCreatureStat(model.CreatureID, string, int) error
}

type activeSession struct {
	ID      string
	ActorID string
}

// getActiveSessions uses reflection to extract active sessions from the context
// without creating a circular dependency with the game package.
func getActiveSessions(ctx *Context) []activeSession {
	if ctx == nil || ctx.Values == nil {
		return nil
	}
	fnVal := reflect.ValueOf(ctx.Values["game.activeSessions"])
	if !fnVal.IsValid() || fnVal.IsNil() {
		return nil
	}
	resVals := fnVal.Call(nil)
	if len(resVals) == 0 {
		return nil
	}
	sliceVal := resVals[0]
	if sliceVal.Kind() != reflect.Slice {
		return nil
	}
	length := sliceVal.Len()
	var sessions []activeSession
	for i := 0; i < length; i++ {
		elemVal := sliceVal.Index(i)
		if elemVal.Kind() == reflect.Interface {
			elemVal = elemVal.Elem()
		}
		if elemVal.Kind() == reflect.Pointer {
			if elemVal.IsNil() {
				continue
			}
			elemVal = elemVal.Elem()
		}
		if elemVal.Kind() != reflect.Struct {
			continue
		}
		idField := elemVal.FieldByName("ID")
		actorIDField := elemVal.FieldByName("ActorID")
		if idField.IsValid() && actorIDField.IsValid() {
			sessions = append(sessions, activeSession{
				ID:      idField.String(),
				ActorID: actorIDField.String(),
			})
		}
	}
	return sessions
}

// findActivePlayerTarget searches active sessions on the C find_who() surface.
func findActivePlayerTarget(ctx *Context, world StatusWorld, name string) (magicEffectTarget, bool) {
	player, creature, _, ok := legacyFindWhoActivePlayer(ctx, world, name)
	if !ok {
		return magicEffectTarget{}, false
	}
	return magicEffectTarget{
		creature:  creature,
		player:    player,
		hasPlayer: true,
	}, true
}

// findRandomTeleportRoom selects a random room from the world that is a valid teleport destination.
func findRandomTeleportRoom(world MagicEffectAgent4World, targetLevel int) (model.RoomID, error) {
	allIDs := world.AllRoomIDs()
	if len(allIDs) == 0 {
		return "", fmt.Errorf("no rooms in world")
	}
	var validIDs []model.RoomID
	for _, id := range allIDs {
		room, ok := world.Room(id)
		if !ok {
			continue
		}
		if roomHasAnyFlag(room, "RNOTEL", "rnotel", "noTeleport") {
			continue
		}
		if minLevel, ok := roomMinLevel(room); ok && targetLevel < minLevel {
			continue
		}
		if maxLevel, ok := roomMaxLevel(room); ok && maxLevel > 0 && targetLevel > maxLevel {
			continue
		}
		validIDs = append(validIDs, id)
	}
	if len(validIDs) == 0 {
		return "", fmt.Errorf("no valid teleport destination rooms")
	}
	return validIDs[rand.Intn(len(validIDs))], nil
}

func teleportTargetResistsLikeLegacy(actor model.Creature, target magicEffectTarget) bool {
	if !magicEffectTargetHasAnyFlag(target, "resistMagic", "PRMAGI", "prmagi") || attackCreatureLevel(actor) >= 128 {
		return false
	}
	casterQuarter := (attackCreatureLevel(actor) + 3) / 4
	targetQuarter := (attackCreatureLevel(target.creature) + 3) / 4
	return (casterQuarter-targetQuarter)*10 < 50
}

// ApplyMagicTeleport handles magicPowerTeleport (C's teleport in magic4.c).
func ApplyMagicTeleport(ctx *Context, world StatusWorld, actor model.Creature, object model.ObjectInstance, resolved ResolvedCommand) (bool, error) {
	mover, ok := world.(MagicEffectAgent4World)
	if !ok {
		return false, fmt.Errorf("world does not support teleport methods")
	}

	how := determineHow(world, object)
	if failed, err := magicEffectSpellFail(world, actor, how, 20); failed || err != nil {
		return false, err
	}

	targetStr := strings.TrimSpace(getArg(resolved, 1))
	isSelf := targetStr == ""
	if !isSelf && how == howPotion {
		ctx.WriteString("\n그 물건은 자신에게만 사용가능합니다.\n")
		return false, nil
	}

	if isSelf {
		playerID := actor.PlayerID
		if playerID.IsZero() {
			playerID = InventoryPlayerIDFromContext(ctx)
		}
		if playerID.IsZero() {
			return false, fmt.Errorf("caster player ID not found")
		}

		destRoomID, err := findRandomTeleportRoom(mover, actor.Level)
		if err != nil {
			return false, err
		}

		if how == howCast {
			if err := mover.SetCreatureStat(actor.ID, "mpCurrent", maxInt(0, creatureStat(actor, "mpCurrent")-20)); err != nil {
				return false, err
			}
		}
		if err := mover.MovePlayerToRoom(playerID, destRoomID); err != nil {
			return false, err
		}

		ctx.WriteString("\n당신은 공간이동술을 사용하기위해 발구름질을 시작합니다.\n공간이동술의 주문을 외우자 몸이 총알같이 빨라지면서\n어디론가로 날라갔습니다.\n")
		roomBroadcast(ctx, actor.RoomID, fmt.Sprintf("\n%s이 공간이동술을 사용하여 어디론가로 사라졌습니다.\n", actor.DisplayName))
		return true, nil
	}

	if targetStr == "나" || targetStr == "자신" {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다.\n")
		return false, nil
	}

	// Target is another player/creature.
	target, ok := magicEffectResolveTarget(ctx, world, actor, targetStr, getOrdinal(resolved, 1))
	if !ok || !target.hasPlayer {
		ctx.WriteString("\n그런 사람은 존재하지 않습니다.\n")
		return false, nil
	}

	if teleportTargetResistsLikeLegacy(actor, target) {
		ctx.WriteString(fmt.Sprintf("\n%s을 공간이동 시키기엔 당신의 주문이 너무 약합니다.\n", target.creature.DisplayName))
		targetMsg := fmt.Sprintf("\n%s이 공간이동술을 사용하여 당신을 이동 시키려 합니다.\n", actor.DisplayName)
		var targetSessionID string
		sessions := getActiveSessions(ctx)
		for _, s := range sessions {
			if s.ActorID == string(target.player.ID) {
				targetSessionID = s.ID
				break
			}
		}
		if targetSessionID != "" {
			invokeSendToSession(ctx, targetSessionID, targetMsg)
		}
		if how == howCast {
			if err := mover.SetCreatureStat(actor.ID, "mpCurrent", maxInt(0, creatureStat(actor, "mpCurrent")-20)); err != nil {
				return false, err
			}
		}
		return false, nil
	}

	destRoomID, err := findRandomTeleportRoom(mover, actor.Level)
	if err != nil {
		return false, err
	}

	if how == howCast {
		if err := mover.SetCreatureStat(actor.ID, "mpCurrent", maxInt(0, creatureStat(actor, "mpCurrent")-20)); err != nil {
			return false, err
		}
	}
	if err := mover.MovePlayerToRoom(target.player.ID, destRoomID); err != nil {
		return false, err
	}

	ctx.WriteString(fmt.Sprintf("\n공간이동술 주문을 %s에게 외웁니다.\n그의 몸이 안개에 휩싸이며 모습이 사라졌습니다.\n", target.creature.DisplayName))

	targetMsg := fmt.Sprintf("\n%s이 당신에게 공간이동술 주문을 외웠습니다.\n당신의 몸이 안개에 휩싸이며 어디론가로 이동됩니다.\n", actor.DisplayName)
	var targetSessionID string
	sessions := getActiveSessions(ctx)
	for _, s := range sessions {
		if s.ActorID == string(target.player.ID) {
			targetSessionID = s.ID
			break
		}
	}
	if targetSessionID != "" {
		invokeSendToSession(ctx, targetSessionID, targetMsg)
	}

	roomBroadcast(ctx, actor.RoomID, fmt.Sprintf("\n%s이 %s에게 공간이동술 주문을 외웠습니다.\n그의 몸이 안개에 휩싸이며 모습이 사라졌습니다.\n", actor.DisplayName, target.creature.DisplayName))
	return true, nil
}

// ApplyMagicSummon handles magicPowerSummon (C's summon in magic5.c).
func ApplyMagicSummon(ctx *Context, world StatusWorld, actor model.Creature, object model.ObjectInstance, resolved ResolvedCommand) (bool, error) {
	mover, ok := world.(MagicEffectAgent4World)
	if !ok {
		return false, fmt.Errorf("world does not support summon methods")
	}

	targetStr := strings.TrimSpace(getArg(resolved, 1))
	how := determineHow(world, object)
	class := creatureClass(actor)
	costsHundredMP := class == model.ClassInvincible || class == model.ClassCaretaker
	requiredMP := 50
	if costsHundredMP {
		requiredMP = 100
	}
	currentMP := creatureStat(actor, "mpCurrent")
	if how == howCast {
		if currentMP < requiredMP {
			ctx.WriteString("\n당신의 도력이 부족합니다.\n")
			return false, nil
		}
		if !creatureHasAnyFlag(actor, "SSUMMO", "ssummo") {
			ctx.WriteString("\n당신은 아직 그런 주술을 터득하지 못했습니다.\n")
			return false, nil
		}
	}

	if mrand(1, 100) < 51 {
		ctx.WriteString("\n소환에 실패를 하였습니다.\n")
		if err := magicEffectDeductMPLegacy(world, actor, 50); err != nil {
			return false, err
		}
		return false, nil
	}

	if targetStr == "" {
		ctx.WriteString("\n자신을 소환하다뇨?.\n")
		return false, nil
	}

	if how == howPotion {
		ctx.WriteString("\n그 물건은 자신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	// Resolve target player
	target, found := findActivePlayerTarget(ctx, world, targetStr)
	if !found || !target.hasPlayer || target.creature.ID == actor.ID ||
		creatureHasAnyFlag(target.creature, "PDMINV", "pdminv", "dmInvisible") {
		ctx.WriteString("\n그런 사람을 못 찾습니다.\n")
		return false, nil
	}

	if how == howCast {
		cost := 50
		if costsHundredMP {
			cost = 100
		}
		if err := magicEffectDeductMPLegacy(world, actor, cost); err != nil {
			return false, err
		}
	}

	// Destination room (caster's room) capacity checks
	casterRoomID := actor.RoomID
	if casterRoomID.IsZero() {
		return false, fmt.Errorf("caster has no room")
	}
	room, ok := mover.Room(casterRoomID)
	if !ok {
		return false, fmt.Errorf("caster room not found")
	}

	numPlayers := magicEffectVisiblePlayerCount(world, room)
	if roomHasAnyFlag(room, "RNOTEL", "noTeleport", "rnotel") ||
		(roomHasAnyFlag(room, "RONEPL", "onePlayer", "ronepl") && numPlayers > 0) ||
		(roomHasAnyFlag(room, "RTWOPL", "twoPlayers", "rtwopl") && numPlayers > 1) ||
		(roomHasAnyFlag(room, "RTHREE", "threePlayers", "rthree") && numPlayers > 2) {
		ctx.WriteString("주문이 공중으로 빨려듭니다.\n")
		return false, nil
	}

	// Destination room level limits check
	if roomHasAnyFlag(room, "family") && !moveCreatureHasFamilyFlag(target.creature, true) {
		ctx.WriteString("그사람은 패거리 가입자가 아닙니다.")
		return false, nil
	}
	if roomHasAnyFlag(room, "onlyFamily", "familyOnly", "ronfml") &&
		!moveCreatureMatchesRoomSpecial(room, target.creature, true, "familyID", "dailyExpndMax", "legacyDailyExpndMax") {
		ctx.WriteString("그사람은 이곳에 올수 없습니다.")
		return false, nil
	}

	targetLevel := target.creature.Level
	levelBlocked := false
	if minLevel, ok := roomMinLevel(room); ok && targetLevel < minLevel {
		levelBlocked = true
	}
	if maxLevel, ok := roomMaxLevel(room); ok && maxLevel > 0 && targetLevel > maxLevel {
		levelBlocked = true
	}
	noSummon := magicEffectTargetHasAnyFlag(target, "PNOSUM", "pnosum", "noSummon")
	if levelBlocked || noSummon {
		ctx.WriteString("\n주문이 실패했습니다.\n")
		if noSummon {
			ctx.WriteString("상대가 소환 거부 중입니다.")
		}
		return false, nil
	}

	// Target parent room has RNOLEA tag check
	targetParentRoom, ok := mover.Room(target.creature.RoomID)
	if ok && roomHasAnyFlag(targetParentRoom, "noSummonOut", "rnolea") {
		ctx.WriteString(fmt.Sprintf("\n소환 주문이 %s을 찾지 못합니다.\n", target.creature.DisplayName))
		return false, nil
	}

	// Move target to caster's room
	if err := mover.MovePlayerToRoom(target.player.ID, casterRoomID); err != nil {
		return false, err
	}

	ctx.WriteString(fmt.Sprintf("\n당신은 %s을 소환하기 위해 주문을 외웁니다.\n주문을 마치자 짙은 안개가 끼더니 갑자기 사라지면서 \n그가 나타났습니다.\n", target.creature.DisplayName))

	targetMsg := fmt.Sprintf("\n당신주위에 짙은 안개가 끼더니 알 수 없는 힘에 이끌려 어디론가 날라갑니다.\n안개가 걷히자 %s이 당신앞에 서 있습니다.\n", actor.DisplayName)
	var targetSessionID string
	sessions := getActiveSessions(ctx)
	for _, s := range sessions {
		if s.ActorID == string(target.player.ID) {
			targetSessionID = s.ID
			break
		}
	}
	if targetSessionID != "" {
		invokeSendToSession(ctx, targetSessionID, targetMsg)
	}

	_ = roomBroadcast2(ctx, world, casterRoomID, ctx.SessionID, target.player.ID, fmt.Sprintf("\n%s이 소환주문을 외우자 짙은 안개가 깔리더니 갑자기 %s이 나타났습니다.\n", actor.DisplayName, target.creature.DisplayName))
	return true, nil
}
