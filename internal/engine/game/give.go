package game

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	enginecmd "muhan/internal/engine/command"
	"muhan/internal/krtext"
	"muhan/internal/session"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

const (
	giveDMClass         = 13
	giveInventoryLimit  = 150
	giveInventorySlot   = "inventory"
	giveCurrencyPostfix = "냥"
)

var (
	ErrGiveWorldRequired = errors.New("game: give world required")
	ErrGiveActorRequired = errors.New("game: give actor required")
)

type GiveWorld interface {
	PlayerLookup
	Room(model.RoomID) (model.Room, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
	MoveObject(model.ObjectInstanceID, model.ObjectLocation) error
	TransferCreatureGold(model.CreatureID, model.CreatureID, int) (int, int, bool, error)
}

type giveRequest struct {
	Item          string
	ItemOrdinal   int64
	Target        string
	TargetOrdinal int64
}

type giveTarget struct {
	Session  ActiveSession
	Name     string
	Player   model.Player
	Creature model.Creature
}

func NewGiveHandler(world GiveWorld) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if world == nil {
			return enginecmd.StatusDefault, ErrGiveWorldRequired
		}
		if ctx == nil || ctx.ActorID == "" || ctx.SessionID == "" {
			return enginecmd.StatusDefault, ErrGiveActorRequired
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		send, ok := sendToSessionFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}

		request, ok := parseGiveRequest(resolved)
		if !ok {
			ctx.WriteString("누구에게 주시려구요?\n")
			return enginecmd.StatusDefault, nil
		}

		actorPlayer, actorCreature, room, err := currentGiveActor(world, model.PlayerID(ctx.ActorID))
		if err != nil {
			return enginecmd.StatusDefault, err
		}

		if amount, isMoney := parseGiveMoney(request.Item); isMoney {
			return giveMoney(ctx, world, active(), send, actorPlayer, actorCreature, room, request, amount)
		}
		return giveObject(ctx, world, active(), send, actorPlayer, actorCreature, room, request)
	}
}

func parseGiveRequest(resolved enginecmd.ResolvedCommand) (giveRequest, bool) {
	rawArgs, rawValues := giveRequestArgs(resolved)
	args := make([]string, 0, len(rawArgs))
	values := make([]int64, 0, len(rawValues))
	for i, arg := range rawArgs {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		args = append(args, arg)
		values = append(values, giveValueAt(rawValues, i))
	}
	if len(args) < 2 {
		return giveRequest{}, false
	}

	if giveCommandFirst(resolved) {
		return giveRequest{
			Target:        args[0],
			TargetOrdinal: normalizeGiveOrdinal(values[0]),
			Item:          strings.Join(args[1:], " "),
			ItemOrdinal:   normalizeGiveOrdinal(values[1]),
		}, true
	}

	last := len(args) - 1
	return giveRequest{
		Item:          strings.Join(args[:last], " "),
		ItemOrdinal:   normalizeGiveOrdinal(values[0]),
		Target:        args[last],
		TargetOrdinal: normalizeGiveOrdinal(values[last]),
	}, true
}

func giveRequestArgs(resolved enginecmd.ResolvedCommand) ([]string, []int64) {
	if len(resolved.Args) > 0 {
		return resolved.Args, resolved.Values
	}
	num := resolved.Parsed.Num
	if num > len(resolved.Parsed.Str) {
		num = len(resolved.Parsed.Str)
	}
	if num <= 1 {
		return nil, nil
	}
	args := make([]string, num-1)
	values := make([]int64, num-1)
	copy(args, resolved.Parsed.Str[1:num])
	copy(values, resolved.Parsed.Val[1:num])
	return args, values
}

func giveCommandFirst(resolved enginecmd.ResolvedCommand) bool {
	fields := strings.Fields(resolved.Input)
	return len(fields) > 0 && fields[0] == resolved.Command()
}

func giveValueAt(values []int64, index int) int64 {
	if index < 0 || index >= len(values) {
		return 1
	}
	return values[index]
}

func normalizeGiveOrdinal(value int64) int64 {
	if value < 1 {
		return 1
	}
	return value
}

func parseGiveMoney(item string) (int, bool) {
	item = strings.TrimSpace(item)
	if !strings.HasSuffix(item, giveCurrencyPostfix) {
		return 0, false
	}
	amountText := strings.TrimSpace(strings.TrimSuffix(item, giveCurrencyPostfix))
	if amountText == "" {
		return 0, false
	}
	return parseGiveLegacyAtol(amountText), true
}

func parseGiveLegacyAtol(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	sign := 1
	switch text[0] {
	case '-':
		sign = -1
		text = text[1:]
	case '+':
		text = text[1:]
	}
	value := 0
	for _, r := range text {
		if r < '0' || r > '9' {
			break
		}
		value = value*10 + int(r-'0')
	}
	return sign * value
}

func currentGiveActor(world GiveWorld, playerID model.PlayerID) (model.Player, model.Creature, model.Room, error) {
	if playerID.IsZero() {
		return model.Player{}, model.Creature{}, model.Room{}, ErrGiveActorRequired
	}
	player, ok := world.Player(playerID)
	if !ok {
		return model.Player{}, model.Creature{}, model.Room{}, fmt.Errorf("give: player %q not found", playerID)
	}
	if player.CreatureID.IsZero() {
		return player, model.Creature{}, model.Room{}, fmt.Errorf("give: player %q has no creature", playerID)
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return player, model.Creature{}, model.Room{}, fmt.Errorf("give: creature %q not found", player.CreatureID)
	}
	roomID := player.RoomID
	if roomID.IsZero() {
		roomID = creature.RoomID
	}
	if roomID.IsZero() {
		return player, creature, model.Room{}, fmt.Errorf("give: player %q has no room", playerID)
	}
	room, ok := world.Room(roomID)
	if !ok {
		return player, creature, model.Room{}, fmt.Errorf("give: room %q not found", roomID)
	}
	return player, creature, room, nil
}

func giveObject(
	ctx *enginecmd.Context,
	world GiveWorld,
	sessions []ActiveSession,
	send func(session.ID, session.Command) error,
	actorPlayer model.Player,
	actorCreature model.Creature,
	room model.Room,
	request giveRequest,
) (enginecmd.Status, error) {
	detectInvisible := giveViewerDetectsInvisible(actorPlayer, actorCreature)
	object, objectName, ok := selectGiveObject(world, actorCreature.Inventory.ObjectIDs, request.Item, request.ItemOrdinal, detectInvisible)
	if !ok {
		ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.\n")
		return enginecmd.StatusDefault, nil
	}
	if err := revealGiveActor(world, actorPlayer, actorCreature); err != nil {
		return enginecmd.StatusDefault, err
	}

	target, ok := findGivePlayerTarget(world, sessions, legacyGiveUpperFirstASCII(request.Target), request.TargetOrdinal, room.ID, detectInvisible)
	if !ok {
		ctx.WriteString("그런 사람은 여기 없어요!\n")
		return enginecmd.StatusDefault, nil
	}
	if target.Player.ID == actorPlayer.ID {
		ctx.WriteString("자신에게는 물건을 줄 수 없습니다.\n")
		return enginecmd.StatusDefault, nil
	}
	if reject := giveObjectRejectMessage(world, object, giveCreatureClass(actorCreature), false); reject != "" {
		ctx.WriteString(reject)
		return enginecmd.StatusDefault, nil
	}
	if givePlayerCapacityExceeded(world, target.Creature, object) {
		ctx.WriteString(target.Name + "님은 더이상 가질 수 없습니다.\n")
		return enginecmd.StatusDefault, nil
	}

	if err := world.MoveObject(object.ID, model.ObjectLocation{CreatureID: target.Creature.ID, Slot: giveInventorySlot}); err != nil {
		return enginecmd.StatusDefault, fmt.Errorf("give object %q to %q: %w", object.ID, target.Creature.ID, err)
	}

	actorName := playerDisplayName(world, string(actorPlayer.ID))
	ctx.WriteString(renderGiveObjectActor(target.Name, objectName))
	_ = send(target.Session.ID, session.Command{Write: renderGiveObjectTarget(actorName, objectName)})
	broadcastGiveRoom(ctx, world, sessions, send, room.ID, actorPlayer.ID, target.Player.ID, renderGiveObjectRoom(actorName, target.Name, objectName))

	// B: Mark dirty + queue (A + C)
	if w, ok := world.(interface {
		MarkPlayerDirty(model.PlayerID)
		QueueSave(model.PlayerID, model.BankID)
	}); ok {
		w.MarkPlayerDirty(actorPlayer.ID)
		w.QueueSave(actorPlayer.ID, "")
		if target.Player.ID != "" {
			w.MarkPlayerDirty(target.Player.ID)
			w.QueueSave(target.Player.ID, "")
		}
	} else if saver, ok := world.(interface{ SavePlayer(model.PlayerID) error }); ok {
		if err := saver.SavePlayer(actorPlayer.ID); err != nil {
			log.Printf("[PERSIST] ERROR give fallback SavePlayer actor: %v", err)
		}
		if target.Player.ID != "" {
			if err := saver.SavePlayer(target.Player.ID); err != nil {
				log.Printf("[PERSIST] ERROR give fallback SavePlayer target: %v", err)
			}
		}
	}
	return enginecmd.StatusDefault, nil
}

func giveMoney(
	ctx *enginecmd.Context,
	world GiveWorld,
	sessions []ActiveSession,
	send func(session.ID, session.Command) error,
	actorPlayer model.Player,
	actorCreature model.Creature,
	room model.Room,
	request giveRequest,
	amount int,
) (enginecmd.Status, error) {
	if amount < 1 {
		ctx.WriteString("돈의 단위는 음수가 될수 없습니다.\n")
		return enginecmd.StatusDefault, nil
	}
	if amount > giveCreatureGold(actorCreature) {
		ctx.WriteString("당신은 그만큼의 돈을 가지고 있지 않습니다.\n")
		return enginecmd.StatusDefault, nil
	}

	target, targetIsPlayer, ok := findGiveMoneyTarget(world, sessions, request.Target, request.TargetOrdinal, room.ID, actorPlayer.ID, giveViewerDetectsInvisible(actorPlayer, actorCreature))
	if !ok {
		ctx.WriteString("그런 사람은 여기 없어요!\n")
		return enginecmd.StatusDefault, nil
	}

	_, _, transferred, err := world.TransferCreatureGold(actorCreature.ID, target.Creature.ID, amount)
	if err != nil {
		return enginecmd.StatusDefault, err
	}
	if !transferred {
		ctx.WriteString("당신은 그만큼의 돈을 가지고 있지 않습니다.\n")
		return enginecmd.StatusDefault, nil
	}

	actorName := playerDisplayName(world, string(actorPlayer.ID))
	ctx.WriteString(renderGiveMoneyActor(target.Name, amount))
	if targetIsPlayer {
		_ = send(target.Session.ID, session.Command{Write: renderGiveMoneyTarget(actorName, amount)})
	}
	excludedTarget := model.PlayerID("")
	if targetIsPlayer {
		excludedTarget = target.Player.ID
	}
	broadcastGiveRoom(ctx, world, sessions, send, room.ID, actorPlayer.ID, excludedTarget, renderGiveMoneyRoom(actorName, target.Name, amount))

	// B: Auto-save after successful money give
	if saver, ok := world.(interface{ SavePlayer(model.PlayerID) error }); ok {
		if err := saver.SavePlayer(actorPlayer.ID); err != nil {
			log.Printf("[PERSIST] ERROR give money SavePlayer actor: %v", err)
		}
		if targetIsPlayer && target.Player.ID != "" {
			if err := saver.SavePlayer(target.Player.ID); err != nil {
				log.Printf("[PERSIST] ERROR give money SavePlayer target: %v", err)
			}
		}
	}
	return enginecmd.StatusDefault, nil
}

func giveCreatureGold(creature model.Creature) int {
	return creature.Stats["gold"]
}

func findGivePlayerTarget(world GiveWorld, sessions []ActiveSession, target string, ordinal int64, roomID model.RoomID, detectInvisible bool) (giveTarget, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return giveTarget{}, false
	}
	ordinal = normalizeGiveOrdinal(ordinal)
	var seen int64
	for _, activeSession := range sessions {
		if activeSession.ActorID == "" {
			continue
		}
		if !roomID.IsZero() && playerRoomID(world, model.PlayerID(activeSession.ActorID)) != roomID {
			continue
		}
		player, ok := world.Player(model.PlayerID(activeSession.ActorID))
		if !ok || player.CreatureID.IsZero() {
			continue
		}
		creature, ok := world.Creature(player.CreatureID)
		if !ok {
			continue
		}
		if creature.DisplayName == "" {
			creature.DisplayName = player.DisplayName
		}
		if !legacyGiveFindCrtVisible(creature, detectInvisible) || !legacyGiveCreaturePrefixMatches(creature, target) {
			continue
		}
		name := giveCreatureDisplayName(creature, player.DisplayName, activeSession.ActorID)
		seen++
		if seen != ordinal {
			continue
		}
		return giveTarget{Session: activeSession, Name: name, Player: player, Creature: creature}, true
	}
	return giveTarget{}, false
}

func findGiveMoneyTarget(
	world GiveWorld,
	sessions []ActiveSession,
	target string,
	ordinal int64,
	roomID model.RoomID,
	actorID model.PlayerID,
	detectInvisible bool,
) (giveTarget, bool, bool) {
	if playerTarget, ok := findGivePlayerTarget(world, sessions, target, ordinal, roomID, detectInvisible); ok {
		if playerTarget.Player.ID != actorID {
			return playerTarget, true, true
		}
	}
	if creature, name, ok := findGiveCreatureTarget(world, roomID, legacyGiveLowerFirstASCII(target), ordinal, actorID, detectInvisible); ok {
		return giveTarget{Name: name, Creature: creature}, false, true
	}
	return giveTarget{}, false, false
}

func findGiveCreatureTarget(world GiveWorld, roomID model.RoomID, target string, ordinal int64, actorID model.PlayerID, detectInvisible bool) (model.Creature, string, bool) {
	room, ok := world.Room(roomID)
	if !ok {
		return model.Creature{}, "", false
	}
	ordinal = normalizeGiveOrdinal(ordinal)
	var seen int64
	for _, creatureID := range room.CreatureIDs {
		if creatureID.IsZero() {
			continue
		}
		creature, ok := world.Creature(creatureID)
		if !ok || creature.PlayerID == actorID || creature.Kind == model.CreatureKindPlayer {
			continue
		}
		if !legacyGiveFindCrtVisible(creature, detectInvisible) || !legacyGiveCreaturePrefixMatches(creature, target) {
			continue
		}
		seen++
		if seen == ordinal {
			return creature, giveCreatureDisplayName(creature, "", string(creature.ID)), true
		}
	}
	return model.Creature{}, "", false
}

func giveNameMatches(name, id, target string) bool {
	name = strings.TrimSpace(name)
	id = strings.TrimSpace(id)
	target = strings.TrimSpace(target)
	return target != "" && (target == name || target == id || strings.HasPrefix(name, target) || strings.HasPrefix(id, target))
}

func selectGiveObject(world GiveWorld, ids []model.ObjectInstanceID, target string, ordinal int64, detectInvisible bool) (model.ObjectInstance, string, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return model.ObjectInstance{}, "", false
	}
	ordinal = normalizeGiveOrdinal(ordinal)

	var seen int64
	for _, id := range ids {
		if id.IsZero() {
			continue
		}
		object, ok := world.Object(id)
		if !ok || !objectLocatedInGiveInventory(object) {
			continue
		}
		if !detectInvisible && giveObjectHasFlag(world, object, "invisible", "oinvis", "OINVIS") {
			continue
		}
		if !legacyGiveObjectPrefixMatches(world, object, target) {
			continue
		}
		seen++
		if seen == ordinal {
			return object, giveObjectDisplayName(world, object), true
		}
	}
	return model.ObjectInstance{}, "", false
}

func objectLocatedInGiveInventory(object model.ObjectInstance) bool {
	return !object.Location.CreatureID.IsZero() && (object.Location.Slot == "" || object.Location.Slot == giveInventorySlot)
}

func legacyGiveObjectPrefixMatches(world GiveWorld, object model.ObjectInstance, target string) bool {
	target = cleanGiveText(target)
	if target == "" {
		return false
	}
	for _, term := range legacyGiveObjectEqualTerms(world, object) {
		if strings.HasPrefix(term, target) {
			return true
		}
	}
	return false
}

func giveObjectDisplayName(world GiveWorld, object model.ObjectInstance) string {
	if name := cleanGiveText(object.DisplayNameOverride); name != "" {
		return name
	}
	if name := cleanGiveText(object.Properties["name"]); name != "" {
		return name
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if name := cleanGiveText(proto.DisplayName); name != "" && !strings.HasPrefix(name, "object:") && !strings.HasPrefix(name, "objinst:") {
				return name
			}
			if name := cleanGiveText(proto.Properties["name"]); name != "" {
				return name
			}
			if name := firstGiveObjectKeyName(proto.Properties); name != "" {
				return name
			}
		}
	}
	if name := firstGiveObjectKeyName(object.Properties); name != "" {
		return name
	}
	return string(object.ID)
}

func legacyGiveObjectEqualTerms(world GiveWorld, object model.ObjectInstance) []string {
	terms := make([]string, 0, 4)
	terms = appendGiveTerm(terms, legacyGiveObjectEqualName(world, object))
	for i := 0; i < 3; i++ {
		terms = appendGiveTerm(terms, legacyGiveObjectEqualKey(world, object, i))
	}
	return terms
}

func legacyGiveObjectEqualName(world GiveWorld, object model.ObjectInstance) string {
	if name := cleanGiveText(object.DisplayNameOverride); name != "" {
		return name
	}
	if name := cleanGiveText(object.Properties["name"]); name != "" {
		return name
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if name := cleanGiveText(proto.Properties["name"]); name != "" {
				return name
			}
			if name := cleanGiveText(proto.DisplayName); name != "" && !strings.HasPrefix(name, "object:") && !strings.HasPrefix(name, "objinst:") {
				return name
			}
		}
	}
	return ""
}

func legacyGiveObjectEqualKey(world GiveWorld, object model.ObjectInstance, index int) string {
	if index < 0 || index > 2 {
		return ""
	}
	key := fmt.Sprintf("key[%d]", index)
	if name := cleanGiveText(object.Properties[key]); name != "" {
		return name
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if name := cleanGiveText(proto.Properties[key]); name != "" {
				return name
			}
			if index < len(proto.Keywords) {
				return cleanGiveText(proto.Keywords[index])
			}
		}
	}
	return ""
}

func appendGiveTerm(terms []string, term string) []string {
	term = cleanGiveText(term)
	if term == "" {
		return terms
	}
	return append(terms, term)
}

func firstGiveObjectKeyName(properties map[string]string) string {
	for _, key := range []string{"key[0]", "key[1]", "key[2]"} {
		if name := cleanGiveText(properties[key]); name != "" {
			return name
		}
	}
	return ""
}

func cleanGiveText(text string) string {
	return strings.TrimSpace(strings.ReplaceAll(text, "\x00", ""))
}

func legacyGiveCreaturePrefixMatches(creature model.Creature, target string) bool {
	target = cleanGiveText(target)
	if len(target) < 2 {
		return false
	}
	for _, term := range legacyGiveCreatureEqualTerms(creature) {
		if legacyASCIIPrefixMatches(term, target) {
			return true
		}
	}
	return false
}

func legacyGiveCreatureEqualTerms(creature model.Creature) []string {
	terms := make([]string, 0, 4)
	terms = appendGiveTerm(terms, creature.DisplayName)
	for _, key := range []string{"key[0]", "key[1]", "key[2]"} {
		terms = appendGiveTerm(terms, creature.Properties[key])
	}
	return terms
}

func legacyGiveFindCrtVisible(creature model.Creature, detectInvisible bool) bool {
	if giveCreatureClass(creature) >= model.ClassCaretaker && giveCreatureHasFlag(creature, "PDMINV", "dmInvisible", "pdminv") {
		return false
	}
	if !detectInvisible && giveCreatureHasFlag(creature, "MINVIS", "minvis", "invisible") {
		return false
	}
	return true
}

func giveCreatureDisplayName(creature model.Creature, fallbackPlayerName string, fallbackID string) string {
	if name := cleanGiveText(creature.DisplayName); name != "" {
		return name
	}
	if name := cleanGiveText(fallbackPlayerName); name != "" {
		return name
	}
	return strings.TrimSpace(fallbackID)
}

func giveViewerDetectsInvisible(player model.Player, creature model.Creature) bool {
	return givePlayerHasFlag(player, "PDINVI", "detectInvisible", "detectInvis") ||
		giveCreatureHasFlag(creature, "PDINVI", "detectInvisible", "detectInvis")
}

func givePlayerHasFlag(player model.Player, names ...string) bool {
	for _, tag := range player.Metadata.Tags {
		if giveTagMatches(tag, names...) {
			return true
		}
	}
	return false
}

func giveCreatureHasFlag(creature model.Creature, names ...string) bool {
	names = state.ExpandFlagNames(names...)
	for _, tag := range creature.Metadata.Tags {
		if giveTagMatches(tag, names...) {
			return true
		}
	}
	targets := make(map[string]struct{}, len(names))
	for _, name := range names {
		if normalized := normalizeGiveTag(name); normalized != "" {
			targets[normalized] = struct{}{}
		}
	}
	for key, value := range creature.Stats {
		if value == 0 {
			continue
		}
		if _, ok := targets[normalizeGiveTag(key)]; ok {
			return true
		}
	}
	return givePropertiesHaveAnyFlag(creature.Properties, names)
}

func legacyGiveUpperFirstASCII(text string) string {
	if text == "" {
		return ""
	}
	if text[0] >= 'a' && text[0] <= 'z' {
		return string(text[0]-('a'-'A')) + text[1:]
	}
	return text
}

func legacyGiveLowerFirstASCII(text string) string {
	if text == "" {
		return ""
	}
	if text[0] >= 'A' && text[0] <= 'Z' {
		return string(text[0]+('a'-'A')) + text[1:]
	}
	return text
}

func giveObjectRejectMessage(world GiveWorld, object model.ObjectInstance, actorClass int, contained bool) string {
	if actorClass < giveDMClass {
		if value, ok := giveObjectQuestNumber(world, object); ok && value != 0 {
			if contained {
				return "임무에 관련되는 물건이 들어있으면 타인에게 줄 수 없습니다.\n"
			}
			return "임무에 관련되는 물건은 타인에게 줄 수 없습니다.\n"
		}
		if contained && giveObjectHasFlag(world, object, "event", "oevent") {
			return "이벤트 아이템이 들어있으면 타인에게 줄 수 없습니다.\n"
		}
		if giveObjectIsRestrictedEvent(world, object) {
			if contained {
				return "이벤트 아이템이 들어있으면 타인에게 줄 수 없습니다.\n"
			}
			return "이벤트 아이템은 타인에게 줄 수 없습니다.\n"
		}
	}
	if contained {
		return ""
	}
	for _, childID := range object.Contents.ObjectIDs {
		child, ok := world.Object(childID)
		if !ok {
			continue
		}
		if reject := giveObjectRejectMessage(world, child, actorClass, true); reject != "" {
			return reject
		}
	}
	return ""
}

func giveObjectIsRestrictedEvent(world GiveWorld, object model.ObjectInstance) bool {
	if !giveObjectHasFlag(world, object, "event", "oevent") {
		return false
	}
	return giveObjectProperty(world, object, "key[2]") != "이벤트"
}

func giveObjectHasTag(world GiveWorld, object model.ObjectInstance, names ...string) bool {
	names = state.ExpandFlagNames(names...)
	for _, tag := range object.Metadata.Tags {
		if giveTagMatches(tag, names...) {
			return true
		}
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			for _, tag := range proto.Metadata.Tags {
				if giveTagMatches(tag, names...) {
					return true
				}
			}
		}
	}
	return false
}

func giveObjectHasFlag(world GiveWorld, object model.ObjectInstance, names ...string) bool {
	names = state.ExpandFlagNames(names...)
	if giveObjectHasTag(world, object, names...) {
		return true
	}
	if givePropertiesHaveAnyFlag(object.Properties, names) {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := world.ObjectPrototype(object.PrototypeID)
	if !ok {
		return false
	}
	return givePropertiesHaveAnyFlag(proto.Properties, names)
}

func givePropertiesHaveAnyFlag(properties map[string]string, names []string) bool {
	for _, name := range names {
		if objectPropertyEnabled(properties, name) {
			return true
		}
	}
	targets := make(map[string]struct{}, len(names))
	for _, name := range names {
		if normalized := normalizeGiveTag(name); normalized != "" {
			targets[normalized] = struct{}{}
		}
	}
	for key, value := range properties {
		if !giveFlagContainerProperty(key) {
			continue
		}
		for _, token := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ';' || r == '|' || r == ' '
		}) {
			if _, ok := targets[normalizeGiveTag(token)]; ok {
				return true
			}
		}
	}
	return false
}

func giveFlagContainerProperty(key string) bool {
	switch normalizeGiveTag(key) {
	case "flag", "flags":
		return true
	default:
		return false
	}
}

func giveTagMatches(tag string, names ...string) bool {
	tag = normalizeGiveTag(tag)
	if tag == "" {
		return false
	}
	names = state.ExpandFlagNames(names...)
	for _, name := range names {
		if tag == normalizeGiveTag(name) {
			return true
		}
	}
	return false
}

func normalizeGiveTag(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	tag = strings.ReplaceAll(tag, "_", "")
	tag = strings.ReplaceAll(tag, "-", "")
	return tag
}

func giveObjectIntProperty(world GiveWorld, object model.ObjectInstance, key string) (int, bool) {
	if value, ok := parseGiveInt(object.Properties[key]); ok {
		return value, true
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if value, ok := parseGiveInt(proto.Properties[key]); ok {
				return value, true
			}
		}
	}
	return 0, false
}

func giveObjectQuestNumber(world GiveWorld, object model.ObjectInstance) (int, bool) {
	for _, key := range []string{"questNumber", "questnum", "questNum"} {
		if value, ok := giveObjectIntProperty(world, object, key); ok {
			return value, true
		}
	}
	return 0, false
}

func giveObjectProperty(world GiveWorld, object model.ObjectInstance, key string) string {
	if value := cleanGiveText(object.Properties[key]); value != "" {
		return value
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			return cleanGiveText(proto.Properties[key])
		}
	}
	return ""
}

func parseGiveInt(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	return parsed, err == nil
}

func giveCreatureClass(creature model.Creature) int {
	return creatureClass(creature)
}

func givePlayerCapacityExceeded(world GiveWorld, creature model.Creature, object model.ObjectInstance) bool {
	return giveCreatureInventoryCount(world, creature) > giveInventoryLimit ||
		giveCreatureCarriedWeight(world, creature)+giveObjectTotalWeight(world, object) > giveCreatureMaxWeight(creature)
}

func giveCreatureInventoryCount(world GiveWorld, creature model.Creature) int {
	total := 0
	for _, objectID := range creature.Inventory.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		total++
		if object, ok := world.Object(objectID); ok && giveObjectIsContainer(world, object) {
			total += giveObjectContainerCount(world, object)
		}
	}
	if total > 200 {
		return 200
	}
	return total
}

func giveObjectContainerCount(world GiveWorld, object model.ObjectInstance) int {
	for _, key := range []string{"shotsCurrent", "shotscur", "shotsCur", "contentsCount"} {
		if count, ok := giveObjectIntProperty(world, object, key); ok {
			return count
		}
	}
	return len(object.Contents.ObjectIDs)
}

func giveCreatureCarriedWeight(world GiveWorld, creature model.Creature) int {
	seen := map[model.ObjectInstanceID]struct{}{}
	weight := 0
	for _, objectID := range creature.Inventory.ObjectIDs {
		weight += giveCarriedObjectWeight(world, objectID, true, seen)
	}
	for _, objectID := range creature.Equipment {
		weight += giveCarriedObjectWeight(world, objectID, false, seen)
	}
	return weight
}

func giveCarriedObjectWeight(world GiveWorld, objectID model.ObjectInstanceID, skipWeightless bool, seen map[model.ObjectInstanceID]struct{}) int {
	if objectID.IsZero() {
		return 0
	}
	if _, exists := seen[objectID]; exists {
		return 0
	}
	seen[objectID] = struct{}{}
	object, ok := world.Object(objectID)
	if !ok {
		return 0
	}
	if skipWeightless && giveObjectHasFlag(world, object, "weightless", "owtles") {
		return 0
	}
	weight := giveObjectOwnWeight(world, object)
	for _, childID := range object.Contents.ObjectIDs {
		weight += giveCarriedObjectWeight(world, childID, true, seen)
	}
	return weight
}

func giveObjectTotalWeight(world GiveWorld, object model.ObjectInstance) int {
	seen := map[model.ObjectInstanceID]struct{}{}
	weight := giveObjectOwnWeight(world, object)
	for _, childID := range object.Contents.ObjectIDs {
		weight += giveCarriedObjectWeight(world, childID, true, seen)
	}
	return weight
}

func giveObjectOwnWeight(world GiveWorld, object model.ObjectInstance) int {
	if weight, ok := giveObjectIntProperty(world, object, "weight"); ok {
		return weight
	}
	return 0
}

func giveObjectIsContainer(world GiveWorld, object model.ObjectInstance) bool {
	if giveObjectHasFlag(world, object, "container", "ocontn") {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := world.ObjectPrototype(object.PrototypeID)
	return ok && proto.Kind == model.ObjectKindContainer
}

func giveCreatureMaxWeight(creature model.Creature) int {
	strength := creatureStat(creature, "strength")
	level := creature.Level
	if level == 0 {
		level = creatureStat(creature, "level")
	}
	maxWeight := 20 + strength*10
	if creatureStat(creature, "class") == model.ClassBarbarian {
		maxWeight += ((level + 3) / 4) * 10
	}
	return maxWeight
}

func revealGiveActor(world GiveWorld, player model.Player, creature model.Creature) error {
	if updater, ok := world.(interface {
		UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	}); ok {
		if _, err := updater.UpdateCreatureTags(creature.ID, nil, []string{"hidden", "phiddn", "PHIDDN"}); err != nil {
			return err
		}
	}
	if !player.ID.IsZero() {
		if updater, ok := world.(interface {
			UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
		}); ok {
			if _, err := updater.UpdatePlayerTags(player.ID, nil, []string{"hidden", "phiddn", "PHIDDN"}); err != nil {
				return err
			}
		}
	}
	if creature.Stats != nil && creature.Stats["PHIDDN"] != 0 {
		if setter, ok := world.(interface {
			SetCreatureStat(model.CreatureID, string, int) error
		}); ok {
			return setter.SetCreatureStat(creature.ID, "PHIDDN", 0)
		}
	}
	return nil
}

func broadcastGiveRoom(
	ctx *enginecmd.Context,
	world GiveWorld,
	sessions []ActiveSession,
	send func(session.ID, session.Command) error,
	roomID model.RoomID,
	actorID model.PlayerID,
	targetID model.PlayerID,
	text string,
) {
	for _, activeSession := range sessions {
		if activeSession.ActorID == "" {
			continue
		}
		playerID := model.PlayerID(activeSession.ActorID)
		if playerID == actorID || (!targetID.IsZero() && playerID == targetID) {
			continue
		}
		if playerRoomID(world, playerID) != roomID {
			continue
		}
		if string(activeSession.ID) == ctx.SessionID {
			ctx.WriteString(text)
			continue
		}
		_ = send(activeSession.ID, session.Command{Write: text})
	}
}

func renderGiveObjectActor(targetName, objectName string) string {
	return "당신은 " + strings.TrimSpace(targetName) + "에게 " + giveObjectWithParticle(objectName) + " 줍니다.\n"
}

func renderGiveObjectTarget(actorName, objectName string) string {
	return "\n" + actorSubject(actorName) + " 당신에게 " + giveObjectWithParticle(objectName) + " 줍니다.\n"
}

func renderGiveObjectRoom(actorName, targetName, objectName string) string {
	return "\n" + actorSubject(actorName) + " " + strings.TrimSpace(targetName) + "에게 " + giveObjectWithParticle(objectName) + " 줍니다.\n"
}

func giveObjectWithParticle(objectName string) string {
	objectName = strings.TrimSpace(objectName)
	if objectName == "" {
		objectName = "물건"
	}
	return objectName + krtext.Particle(objectName, '3')
}

func renderGiveMoneyActor(targetName string, amount int) string {
	return "당신은 " + strings.TrimSpace(targetName) + "에게 " + giveMoneyText(amount) + "을 주었습니다.\n"
}

func renderGiveMoneyTarget(actorName string, amount int) string {
	return "\n" + actorSubject(actorName) + " 당신에게 " + giveMoneyText(amount) + "을 주었습니다.\n"
}

func renderGiveMoneyRoom(actorName, targetName string, amount int) string {
	return "\n" + actorSubject(actorName) + " " + strings.TrimSpace(targetName) + "에게 " + giveMoneyText(amount) + "을 주었습니다.\n"
}

func giveMoneyText(amount int) string {
	return strconv.Itoa(amount) + giveCurrencyPostfix
}

// completeQuestDelivery mirrors trade/social quest complete + C Q_SET + quest_exp for item questnum given to quest NPC.
// Uses "quest_completed_N" prop (consistent with trade) + exp/prof + msg.
func completeQuestDelivery(ctx *enginecmd.Context, world GiveWorld, actorC model.Creature, actorP model.Player, questNum int, itemName string) {
	if questNum < 1 {
		return
	}
	if creatureQuestCompleted(actorC, questNum) {
		ctx.WriteString(questAlreadyCompletedMessage())
		return
	}
	// Set flag via type assert (GiveWorld minimal, state supports)
	if setter, ok := world.(interface {
		SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
	}); ok {
		if _, err := setter.SetCreatureProperty(actorC.ID, questCompletionKey(questNum), "1"); err != nil {
			log.Printf("quest set prop: %v", err)
		}
	}

	// Award exp and distribute proficiency exactly like C add_prof().
	xp := getQuestExpLocal(questNum - 1)
	if xp > 0 {
		newExp := creatureStat(actorC, "experience") + xp
		if setter, ok := world.(interface {
			SetCreatureStat(model.CreatureID, string, int) error
		}); ok {
			_ = setter.SetCreatureStat(actorC.ID, "experience", newExp)
		}
		_ = addProfLocal(world, actorC, xp)
		ctx.WriteString(fmt.Sprintf("당신은 임무를 완수하여 %d의 경험치를 받았습니다.\n", xp))
	} else {
		ctx.WriteString("NPC에게 퀘스트 아이템을 건네 퀘스트가 완료되었습니다.\n")
	}
}

func creatureQuestCompleted(creature model.Creature, questNum int) bool {
	if questNum < 1 || creature.Properties == nil {
		return false
	}
	return creature.Properties[questCompletionKey(questNum)] == "1"
}

func questCompletionKey(questNum int) string {
	return fmt.Sprintf("quest_completed_%d", questNum)
}

func questAlreadyCompletedMessage() string {
	return "당신은 그것을 받을 수 없습니다. 당신은 이미 그 임무를 달성했습니다.\n"
}

// Local copies of quest helpers (to avoid import cycle game<->command; exact parity values from social_transfer_commands.go:53)
var questExpLocal = []int{120, 500, 1000, 3000, 5000, 8000, 10000, 20000, 30000, 50000, 80000, 100000, 200000, 300000, 500000, 700000, 800000, 900000, 1000000, 1300000}

func getQuestExpLocal(questIndex int) int {
	if questIndex < 0 {
		return 0
	}
	if questIndex < len(questExpLocal) {
		return questExpLocal[questIndex]
	}
	return 125
}

func addProfLocal(world GiveWorld, creature model.Creature, exp int) error {
	if exp <= 0 {
		return nil
	}
	part := exp / 9
	if part <= 0 {
		return nil
	}
	weaponTypes := []string{"sharp", "thrust", "blunt", "pole", "missile"}
	for _, wt := range weaponTypes {
		key := "proficiency/" + wt
		valStr := ""
		if creature.Properties != nil {
			valStr = creature.Properties[key]
		}
		val, _ := strconv.Atoi(valStr)
		if setter, ok := world.(interface {
			SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
		}); ok {
			if _, err := setter.SetCreatureProperty(creature.ID, key, strconv.Itoa(val+part)); err != nil {
				return err
			}
		}
	}

	for i := 1; i <= 4; i++ {
		key := fmt.Sprintf("realm/%d", i)
		valStr := ""
		if creature.Properties != nil {
			valStr = creature.Properties[key]
		}
		val, _ := strconv.Atoi(valStr)
		if setter, ok := world.(interface {
			SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
		}); ok {
			if _, err := setter.SetCreatureProperty(creature.ID, key, strconv.Itoa(val+part)); err != nil {
				return err
			}
		}
	}
	return nil
}
