package command

import (
	"fmt"
	"math/rand"
	"reflect"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const (
	stealCooldownKey             = "steal"
	stealPlayerKillCooldownKey   = "plykl"
	stealPlayerKillPenaltyMinSec = 7 * 86400
	stealPlayerKillPenaltyMaxSec = 10 * 86400
)

type StealWorld interface {
	LookWorld
	StealCreatureInventoryObject(model.ObjectInstanceID, model.CreatureID, model.CreatureID) (bool, error)
	PickupMoneyObjectToCreatureGold(model.ObjectInstanceID, model.ObjectLocation, model.CreatureID) (int, int, bool, error)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
}

type StealRollFunc func(min int, max int) int

func NewStealHandler(world StealWorld, roll StealRollFunc) Handler {
	if roll == nil {
		roll = func(min int, max int) int {
			if max <= min {
				return min
			}
			return min + rand.Intn(max-min+1)
		}
	}
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("steal: actor creature %q not found", viewer.CreatureID)
		}

		objectTarget := getArg(resolved, 0)
		if objectTarget == "" {
			ctx.WriteString("무엇을 훔치려구요?")
			return StatusDefault, nil
		}
		target := getArg(resolved, 1)
		if target == "" {
			ctx.WriteString("누구한테서 훔치려구요?")
			return StatusDefault, nil
		}

		if ok, message := stealActorAllowed(actor); !ok {
			ctx.WriteString(message)
			return StatusDefault, nil
		}

		if err := clearStealActorHidden(world, viewer, actor); err != nil {
			return StatusDefault, err
		}
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, stealCooldownKey, time.Now().Unix(), 5); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}
		if err := revealStealActorIfInvisible(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}

		victim, ok := findStealTargetCreature(world, room, viewer, target, getOrdinal(resolved, 1))
		if !ok {
			ctx.WriteString("그런건 여기 없습니다.")
			return StatusDefault, nil
		}
		if creatureHasAnyFlag(actor, "blind", "pblind") {
			ctx.WriteString("당신은 눈이 멀어 훔칠 수 없습니다.")
			return StatusDefault, nil
		}
		if message, forbidden := stealTargetForbiddenMessage(world, viewer, room, actor, victim); forbidden {
			ctx.WriteString(message)
			return StatusDefault, nil
		}

		object, ok := findStealObject(world, victim, objectTarget, getOrdinal(resolved, 0), viewerDetectsInvisible(world, viewer))
		if !ok {
			ctx.WriteString(stealSubjectPronoun(victim) + "는 그런 물건을 갖고 있지 않습니다.")
			return StatusDefault, nil
		}

		chance := stealChance(world, actor, victim, object)
		if roll(1, 100) > chance {
			ctx.WriteString("실패하였습니다.")
			if !attackCreatureIsPlayer(victim) {
				ctx.WriteString("\n그가 당신을 공격합니다.")
			}
			actorName := attackCreatureName(actor)
			victimName := attackCreatureName(victim)
			_ = broadcastStealFailure(ctx, world, room.ID, victim, "\n"+actorName+"이 "+victimName+"에게서 물건을 훔치려고 합니다.")
			_ = notifyStealPlayerVictim(ctx, victim, actorName, objectDisplayName(world, object))
			return StatusDefault, nil
		}

		if objectKindIs(world, object, model.ObjectKindMoney) || objectLegacyType(world, object) == 10 {
			newGold, amount, picked, err := world.PickupMoneyObjectToCreatureGold(object.ID, object.Location, actor.ID)
			if err != nil {
				return StatusDefault, err
			}
			if !picked {
				ctx.WriteString(stealSubjectPronoun(victim) + "는 그런 물건을 갖고 있지 않습니다.")
				return StatusDefault, nil
			}
			if err := applyStealPlayerKillPenalty(world, actor, victim); err != nil {
				return StatusDefault, err
			}
			queueStealSuccessSaves(world, actor, victim)
			ctx.WriteString(fmt.Sprintf("당신은 %d냥을 훔쳐 %d냥을 갖고 있습니다.", amount, newGold))
			return StatusDefault, nil
		}

		moved, err := world.StealCreatureInventoryObject(object.ID, victim.ID, actor.ID)
		if err != nil {
			return StatusDefault, err
		}
		if !moved {
			ctx.WriteString(stealSubjectPronoun(victim) + "는 그런 물건을 갖고 있지 않습니다.")
			return StatusDefault, nil
		}
		if err := applyStealPlayerKillPenalty(world, actor, victim); err != nil {
			return StatusDefault, err
		}
		queueStealSuccessSaves(world, actor, victim)
		ctx.WriteString("훔쳤습니다.")
		return StatusDefault, nil
	}
}

func stealActorAllowed(actor model.Creature) (bool, string) {
	class := creatureStat(actor, "class")
	if class == model.ClassThief {
		return true, ""
	}
	if class < model.ClassInvincible {
		return false, "도둑만 훔칠수 있습니다."
	}
	if creatureHasAnyFlag(actor, "STHIEF", "thiefSpell", "thiefMode") || creatureStat(actor, "STHIEF") != 0 {
		return true, ""
	}
	return false, "\n도둑을 무적수련하지 않았습니다.\n"
}

func clearStealActorHidden(world StealWorld, viewer LookViewer, actor model.Creature) error {
	remove := []string{"hidden", "phiddn", "PHIDDN"}
	if _, err := world.UpdateCreatureTags(actor.ID, nil, remove); err != nil {
		return err
	}
	if !viewer.PlayerID.IsZero() {
		if _, err := world.UpdatePlayerTags(viewer.PlayerID, nil, remove); err != nil {
			return err
		}
	}
	if actor.Stats != nil && actor.Stats["PHIDDN"] != 0 {
		if err := world.SetCreatureStat(actor.ID, "PHIDDN", 0); err != nil {
			return err
		}
	}
	return nil
}

func revealStealActorIfInvisible(ctx *Context, world StealWorld, roomID model.RoomID, viewer LookViewer, actor model.Creature) error {
	invisible := creatureHasAnyFlag(actor, "invisible", "pinvis", "PINVIS")
	if !viewer.PlayerID.IsZero() {
		if player, ok := world.Player(viewer.PlayerID); ok && hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "pinvis", "PINVIS") {
			invisible = true
		}
	}
	if !invisible {
		return nil
	}
	if _, err := world.UpdateCreatureTags(actor.ID, nil, []string{"invisible", "pinvis", "PINVIS"}); err != nil {
		return err
	}
	if !viewer.PlayerID.IsZero() {
		if _, err := world.UpdatePlayerTags(viewer.PlayerID, nil, []string{"invisible", "pinvis", "PINVIS"}); err != nil {
			return err
		}
	}
	if actor.Stats != nil && actor.Stats["PINVIS"] != 0 {
		if err := world.SetCreatureStat(actor.ID, "PINVIS", 0); err != nil {
			return err
		}
	}
	name := attackCreatureName(actor)
	ctx.WriteString("당신의 모습이 서서히 드러납니다.")
	return roomBroadcast(ctx, roomID, "\n"+name+"의 모습이 서서히 드러납니다.")
}

func findStealTargetCreature(
	world LookWorld,
	room model.Room,
	viewer LookViewer,
	prefix string,
	ordinal int64,
) (model.Creature, bool) {
	if creature, ok := findLegacyMonsterTarget(world, room, viewer, prefix, ordinal); ok {
		return creature, true
	}
	_, creature, ok := findLegacyPlayerCreatureTarget(world, room, viewer, legacyUpperFirstASCII(prefix), ordinal)
	if ok {
		return creature, true
	}
	return model.Creature{}, false
}

func stealTargetForbiddenMessage(world StealWorld, viewer LookViewer, room model.Room, actor model.Creature, victim model.Creature) (string, bool) {
	if !attackCreatureIsPlayer(victim) {
		if creatureHasAnyFlag(victim, "unkillable", "cannotKill", "munkil") {
			return "당신은 " + stealObjectPronoun(victim) + " 해칠수 없습니다.", true
		}
		if sneakMonsterTargetsActor(world, victim.ID, viewer.PlayerID, actor) {
			return stealSubjectPronoun(victim) + "는 싸우는 중이 아닙니다.", true
		}
		return "", false
	}
	if roomHasAnyFlag(room, "noKill", "rnokil") {
		return "이 방에서는 훔칠 수 없습니다.", true
	}
	class := creatureStat(actor, "class")
	if class >= model.ClassSubDM {
		return "", false
	}
	if !creatureHasAnyFlag(actor, "chaos", "pchaos") {
		return "당신은 선해서 훔칠 수 없습니다.", true
	}
	if !creatureHasAnyFlag(victim, "chaos", "pchaos") {
		return "그 사용자는 선해서 보호받고 있습니다.", true
	}
	return "", false
}

func stealSubjectPronoun(creature model.Creature) string {
	if creatureHasAnyFlag(creature, "MMALES", "male", "pMale") {
		return "그"
	}
	return "그녀"
}

func stealObjectPronoun(creature model.Creature) string {
	if creatureHasAnyFlag(creature, "MMALES", "male", "pMale") {
		return "그를"
	}
	return "그녀를"
}

func findStealObject(world InventoryWorld, victim model.Creature, target string, ordinal int64, detectInvisible bool) (model.ObjectInstance, bool) {
	if ordinal < 1 {
		ordinal = 1
	}
	var seen int64
	for _, objectID := range victim.Inventory.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInCreatureInventory(object, victim.ID) {
			continue
		}
		if !detectInvisible && dropObjectIsInvisible(world, object) {
			continue
		}
		if !legacyObjectPrefixMatches(world, object, target) {
			continue
		}
		seen++
		if seen == ordinal {
			return object, true
		}
	}
	return model.ObjectInstance{}, false
}

func stealChance(world InventoryWorld, actor model.Creature, victim model.Creature, object model.ObjectInstance) int {
	class := creatureStat(actor, "class")
	chance := 3 * ((creatureLevel(actor) + 3) / 4)
	if class == model.ClassThief {
		chance = 4 * ((creatureLevel(actor) + 3) / 4)
	}
	chance += legacyStatBonus(creatureStat(actor, "dexterity")) * 3
	if creatureLevel(victim) > creatureLevel(actor) {
		chance -= 15 * (((creatureLevel(victim) + 3) / 4) - ((creatureLevel(actor) + 3) / 4))
	}
	chance = minInt(chance, 65)
	if stealObjectProtected(world, object) || creatureHasAnyFlag(victim, "noSteal", "munstl") {
		chance = 0
	}
	if class == model.ClassDM {
		chance = 100
	}
	if creatureStat(victim, "class") == model.ClassDM {
		chance = 0
	}
	if stealObjectHasTopLevelEventFlag(world, object) {
		chance = 0
	}
	if chance < 0 {
		return 0
	}
	return chance
}

func stealObjectProtected(world InventoryWorld, object model.ObjectInstance) bool {
	if objectIntPropertyOrZero(world, object, "questNumber") != 0 ||
		objectIntPropertyOrZero(world, object, "questnum") != 0 ||
		objectHasAnyTag(world, object, "event", "oevent") ||
		objectHasAnyPropertyFlag(world, object, "event", "oevent", "OEVENT") {
		return true
	}
	for _, objectID := range object.Contents.ObjectIDs {
		child, ok := world.Object(objectID)
		if !ok {
			continue
		}
		if objectIntPropertyOrZero(world, child, "questNumber") != 0 ||
			objectIntPropertyOrZero(world, child, "questnum") != 0 ||
			objectHasAnyTag(world, child, "event", "oevent") ||
			objectHasAnyPropertyFlag(world, child, "event", "oevent", "OEVENT") {
			return true
		}
	}
	return false
}

func stealObjectHasTopLevelEventFlag(world InventoryWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object, "event", "oevent") ||
		objectHasAnyPropertyFlag(world, object, "event", "oevent", "OEVENT")
}

func broadcastStealFailure(ctx *Context, world StealWorld, roomID model.RoomID, victim model.Creature, text string) error {
	if !attackCreatureIsPlayer(victim) || victim.PlayerID.IsZero() {
		return roomBroadcast(ctx, roomID, text)
	}
	active, send, ok := stealSessionHooks(ctx)
	if !ok {
		return roomBroadcast(ctx, roomID, text)
	}
	for _, activeSession := range active {
		if activeSession.ActorID == "" || activeSession.ActorID == string(victim.PlayerID) || string(activeSession.ID) == ctx.SessionID {
			continue
		}
		player, ok := world.Player(model.PlayerID(activeSession.ActorID))
		if !ok || player.RoomID != roomID {
			continue
		}
		if err := send(activeSession.ID, text); err != nil {
			return err
		}
	}
	return nil
}

func notifyStealPlayerVictim(ctx *Context, victim model.Creature, actorName string, objectName string) error {
	if !attackCreatureIsPlayer(victim) || victim.PlayerID.IsZero() {
		return nil
	}
	active, send, ok := stealSessionHooks(ctx)
	if !ok {
		return nil
	}
	text := "\n" + actorName + "이 당신에게서 " + objectName + krtext.Particle(objectName, '3') + " 훔치려고 합니다."
	for _, activeSession := range active {
		if activeSession.ActorID != string(victim.PlayerID) {
			continue
		}
		if string(activeSession.ID) == ctx.SessionID {
			ctx.WriteString(text)
			return nil
		}
		return send(activeSession.ID, text)
	}
	return nil
}

type stealActiveSession struct {
	ID      string
	ActorID string
}

func stealSessionHooks(ctx *Context) ([]stealActiveSession, func(string, string) error, bool) {
	if ctx == nil || ctx.Values == nil {
		return nil, nil, false
	}
	activeValue := reflect.ValueOf(ctx.Values["game.activeSessions"])
	sendValue := reflect.ValueOf(ctx.Values["game.sendToSession"])
	if !activeValue.IsValid() || activeValue.Kind() != reflect.Func ||
		activeValue.Type().NumIn() != 0 || activeValue.Type().NumOut() != 1 ||
		!sendValue.IsValid() || sendValue.Kind() != reflect.Func ||
		sendValue.Type().NumIn() != 2 || sendValue.Type().NumOut() != 1 {
		return nil, nil, false
	}
	if sendValue.Type().Out(0) != reflect.TypeOf((*error)(nil)).Elem() ||
		sendValue.Type().In(1).Kind() != reflect.Struct {
		return nil, nil, false
	}

	out := activeValue.Call(nil)[0]
	if out.Kind() != reflect.Slice {
		return nil, nil, false
	}
	active := make([]stealActiveSession, 0, out.Len())
	for i := 0; i < out.Len(); i++ {
		item := out.Index(i)
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
		active = append(active, stealActiveSession{
			ID:      fmt.Sprint(idField.Interface()),
			ActorID: actorField.String(),
		})
	}

	send := func(id string, text string) error {
		idValue := reflect.ValueOf(id)
		if !idValue.Type().AssignableTo(sendValue.Type().In(0)) {
			if !idValue.Type().ConvertibleTo(sendValue.Type().In(0)) {
				return fmt.Errorf("steal: send session id type %s is not compatible with %s", idValue.Type(), sendValue.Type().In(0))
			}
			idValue = idValue.Convert(sendValue.Type().In(0))
		}
		commandValue, ok := stealWriteCommandValue(sendValue.Type().In(1), text)
		if !ok {
			return fmt.Errorf("steal: send command type %s does not expose settable Write string field", sendValue.Type().In(1))
		}
		results := sendValue.Call([]reflect.Value{idValue, commandValue})
		if errValue := results[0]; !errValue.IsNil() {
			return errValue.Interface().(error)
		}
		return nil
	}
	return active, send, true
}

func stealWriteCommandValue(commandType reflect.Type, text string) (reflect.Value, bool) {
	if commandType.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	commandValue := reflect.New(commandType).Elem()
	writeField := commandValue.FieldByName("Write")
	if !writeField.IsValid() || !writeField.CanSet() || writeField.Kind() != reflect.String {
		return reflect.Value{}, false
	}
	writeField.SetString(text)
	return commandValue, true
}

func applyStealPlayerKillPenalty(world StealWorld, actor model.Creature, victim model.Creature) error {
	if !attackCreatureIsPlayer(victim) {
		return nil
	}
	interval := int64(rand.Intn(stealPlayerKillPenaltyMaxSec-stealPlayerKillPenaltyMinSec+1) + stealPlayerKillPenaltyMinSec)
	return world.SetCreatureCooldown(actor.ID, stealPlayerKillCooldownKey, time.Now().Unix(), interval)
}

func queueStealSuccessSaves(world StealWorld, actor model.Creature, victim model.Creature) {
	queueShopPlayerSave(world, actor.PlayerID)
	if attackCreatureIsPlayer(victim) {
		queueShopPlayerSave(world, victim.PlayerID)
	}
}
