package command

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

type magicMoveObjectWorld interface {
	MoveObject(model.ObjectInstanceID, model.ObjectLocation) error
}

func magicEffectObjectSend(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	sourceObject model.ObjectInstance,
	resolved ResolvedCommand,
) (bool, error) {
	if actor.ID.IsZero() {
		return false, nil
	}

	how := determineHow(world, sourceObject)
	if how == howCast {
		if !creatureHasAnyFlag(actor, "STRANO", "objectSendSpell") {
			ctx.WriteString("\n당신은 아직 그런 주문을 터득하지 못했습니다.\n")
			return false, nil
		}
		class := creatureStat(actor, "class")
		if class != model.ClassMage && class < model.ClassInvincible {
			ctx.WriteString("\n이 주술은 도술사들만이 사용할 수 있습니다.\n")
			return false, nil
		}
		if class >= model.ClassInvincible && !attackCreatureHasFlag(actor, "SMAGE", "smage") {
			ctx.WriteString("\n도술사를 무적수련하지 않았습니다..\n")
			return false, nil
		}
	}

	level := getCreatureLevel(actor)
	if level < 20 {
		ctx.WriteString("\n당신의 능력이 부족합니다.\n")
		return false, nil
	}

	objectName := getArg(resolved, 1)
	targetPlayerName := getArg(resolved, 2)

	if objectName == "" || targetPlayerName == "" {
		ctx.WriteString("\n누구에게 무얼 보냅니까?\n")
		return false, nil
	}

	// 1. Resolve target player through C find_who()-style active sessions.
	targetPlayer, ok := findObjectSendActivePlayer(ctx, world, targetPlayerName)
	if !ok {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다 .\n")
		return false, nil
	}

	targetCreature, targetHasCreature := world.Creature(targetPlayer.CreatureID)
	if !targetHasCreature {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다 .\n")
		return false, nil
	}

	// Reject if target has DM Invisible flag
	if creatureHasAnyFlag(targetCreature, "dmInvisible", "pdminv", "PDMINV") {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다 .\n")
		return false, nil
	}

	// Reject if target is invisible and caster cannot detect invisible
	if creatureHasAnyFlag(targetCreature, "invisible", "pinvis", "PINVIS") &&
		!creatureHasAnyFlag(actor, "detectInvisible", "pdinvi", "PDINVI") {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다 .\n")
		return false, nil
	}

	// 2. Resolve object from caster's inventory
	object, objName, ok := findEquipInventoryObjectWithVisibility(world, actor, objectName, getOrdinal(resolved, 1), viewerHasDetectInvisibleCreatureTag(actor))
	if !ok {
		ctx.WriteString("\n그런 물건이 존재하지 않습니다.\n")
		return false, nil
	}

	// Quest item or event item checks
	if objectSendHasQuestNumber(world, object) || objectHasAnyTag(world, object, "quest") {
		ctx.WriteString("\n임무에 관련되어 다른자에게 보낼 수 없습니다.\n")
		return false, nil
	}
	if objectSendIsEvent(world, object) {
		ctx.WriteString("\n이벤트 아이템은 다른자에게 보낼 수 없습니다.\n")
		return false, nil
	}
	if creatureStat(actor, "class") < model.ClassDM && objectSendHasDirectQuestOrEventChild(world, object) {
		ctx.WriteString("\n전송에 실패했습니다.\n")
		return false, nil
	}

	// Weight limit checks
	intel := creatureStat(actor, "intelligence")
	intelBonus := legacyStatBonus(intel)
	maxWeightLimit := 5 + intelBonus + (((level+3)/4)-5)*2

	sendWeight := magicObjectOwnWeight(world, object)
	if sendWeight > maxWeightLimit {
		ctx.WriteString(fmt.Sprintf("\n%s%s 전송은 당신의 능력으로 너무 무거워 \n보낼 수 없습니다.\n", objName, krtext.Particle(objName, '2')))
		return false, nil
	}

	// MP Cost: 8 + weight / 4
	mpCost := 8 + sendWeight/4
	currentMP := creatureStat(actor, "mpCurrent")
	if how == howCast && currentMP < mpCost {
		ctx.WriteString("\n당신의 도력이 부족합니다.\n")
		return false, nil
	}

	if how == howCast {
		if err := setCreatureStat(world, actor.ID, "mpCurrent", currentMP-mpCost); err != nil {
			return false, err
		}
	}
	if spellFail(actor) {
		return false, nil
	}

	// Check destination capacity
	targetMaxWeight := getCreatureMaxWeight(targetCreature)
	targetCarriedWeight := getCreatureCarriedWeight(world, targetCreature)
	if targetMaxWeight < targetCarriedWeight+sendWeight {
		ctx.WriteString(fmt.Sprintf("\n%s은 %s을 가질 수 없습니다 .\n", targetCreature.DisplayName, objName))
		return false, nil
	}

	mover, ok := world.(magicMoveObjectWorld)
	if !ok {
		return false, fmt.Errorf("world does not support MoveObject")
	}

	// Move the object
	if err := mover.MoveObject(object.ID, model.ObjectLocation{CreatureID: targetCreature.ID, Slot: "inventory"}); err != nil {
		return false, err
	}
	queueShopPlayerSave(world, actor.PlayerID)
	queueShopPlayerSave(world, targetPlayer.ID)

	// Output messages
	isCasterDMInvis := creatureHasAnyFlag(actor, "dmInvisible", "pdminv", "PDMINV")
	actorName := attackCreatureName(actor)
	if !isCasterDMInvis {
		_ = roomBroadcast(ctx, actor.RoomID, fmt.Sprintf("\n%s%s 물건을 어디론가로 날려버렸습니다.\n", actorName, krtext.Particle(actorName, '1')))
	}

	ctx.WriteString(fmt.Sprintf("\n당신이 %s에 정신집중을 하면서 전송주를 외웁니다.\n그 물건이 공중으로 떠오르면서 어디론가로 총알같이\n날라갑니다.\n", objName))
	ctx.WriteString(fmt.Sprintf("\n당신은 %s%s %s에게 보냈습니다.\n", objName, krtext.Particle(objName, '3'), targetCreature.DisplayName))

	if !isCasterDMInvis {
		targetMsg := fmt.Sprintf("\n갑자기 하늘에서 무언가가 하늘에서 뚝 떨어졌습니다.\n자세히 보니 %s인데, %s%s 물건을 전송한 것 같습니다.\n", objName, actorName, krtext.Particle(actorName, '1'))

		var targetSessionID string
		sessions := getActiveSessionsAgent6(ctx)
		for _, s := range sessions {
			if s.ActorID == string(targetPlayer.ID) {
				targetSessionID = s.ID
				break
			}
		}
		if targetSessionID != "" {
			invokeSendToSession(ctx, targetSessionID, targetMsg)
		}
	}

	return true, nil
}

func getCreatureMaxWeight(creature model.Creature) int {
	strength := creatureStat(creature, "strength")
	class := creatureStat(creature, "class")
	level := getCreatureLevel(creature)

	n := 20 + strength*10
	if class == model.ClassBarbarian {
		n += ((level + 3) / 4) * 10
	}
	return n
}

func getCreatureCarriedWeight(world StatusWorld, creature model.Creature) int {
	if world == nil || creature.ID.IsZero() {
		return 0
	}
	seen := map[model.ObjectInstanceID]struct{}{}
	weight := 0
	for _, objectID := range creature.Inventory.ObjectIDs {
		weight += magicCarriedObjectWeight(world, objectID, true, seen)
	}
	for _, objectID := range creature.Equipment {
		weight += magicCarriedObjectWeight(world, objectID, false, seen)
	}
	return weight
}

func magicCarriedObjectWeight(world StatusWorld, objectID model.ObjectInstanceID, skipWeightless bool, seen map[model.ObjectInstanceID]struct{}) int {
	if objectID.IsZero() {
		return 0
	}
	if _, ok := seen[objectID]; ok {
		return 0
	}
	seen[objectID] = struct{}{}

	object, ok := world.Object(objectID)
	if !ok {
		return 0
	}
	if skipWeightless && magicObjectWeightless(world, object) {
		return 0
	}

	weight := magicObjectOwnWeight(world, object)
	for _, childID := range object.Contents.ObjectIDs {
		weight += magicCarriedObjectWeight(world, childID, true, seen)
	}
	return weight
}

func magicObjectTotalWeight(world StatusWorld, object model.ObjectInstance) int {
	seen := map[model.ObjectInstanceID]struct{}{object.ID: {}}
	weight := magicObjectOwnWeight(world, object)
	for _, childID := range object.Contents.ObjectIDs {
		weight += magicCarriedObjectWeight(world, childID, true, seen)
	}
	return weight
}

func findObjectSendActivePlayer(ctx *Context, world StatusWorld, targetName string) (model.Player, bool) {
	player, _, _, ok := legacyFindWhoActivePlayer(ctx, world, targetName)
	return player, ok
}

func objectSendHasDirectQuestOrEventChild(world StatusWorld, object model.ObjectInstance) bool {
	for _, childID := range object.Contents.ObjectIDs {
		child, ok := world.Object(childID)
		if !ok {
			continue
		}
		if objectSendHasQuestNumber(world, child) ||
			objectHasAnyTag(world, child, "quest") ||
			objectSendIsEvent(world, child) {
			return true
		}
	}
	return false
}

func objectSendHasQuestNumber(world StatusWorld, object model.ObjectInstance) bool {
	return objectIntPropertyOrZero(world, object, "questNumber") != 0 ||
		objectIntPropertyOrZero(world, object, "questnum") != 0 ||
		objectIntPropertyOrZero(world, object, "questNum") != 0
}

func objectSendIsEvent(world StatusWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object, "event", "oevent", "OEVENT") ||
		objectHasAnyPropertyFlag(world, object, "event", "oevent", "OEVENT")
}

func magicObjectOwnWeight(world StatusWorld, object model.ObjectInstance) int {
	if weight, ok := magicParseObjectWeight(object.Properties["weight"]); ok {
		return weight
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if weight, ok := magicParseObjectWeight(proto.Properties["weight"]); ok {
				return weight
			}
		}
	}
	return 0
}

func magicParseObjectWeight(value string) (int, bool) {
	weight, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return weight, true
}

func magicObjectWeightless(world StatusWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object, "weightless", "owtles") ||
		objectHasAnyPropertyFlag(world, object, "weightless", "owtles")
}

func getActiveSessionsAgent6(ctx *Context) []activeSession {
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
		idVal := elemVal.FieldByName("ID")
		actorIDVal := elemVal.FieldByName("ActorID")
		if !idVal.IsValid() || !actorIDVal.IsValid() {
			continue
		}
		sessions = append(sessions, activeSession{
			ID:      fmt.Sprint(idVal.Interface()),
			ActorID: actorIDVal.String(),
		})
	}
	return sessions
}
