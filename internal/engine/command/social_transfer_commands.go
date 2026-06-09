package command

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"muhan/internal/krtext"
	"muhan/internal/persist/legacykr"
	"muhan/internal/world/model"
)

type TradeWorld interface {
	Room(model.RoomID) (model.Room, bool)
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
	MoveObjectToCreatureInventory(model.ObjectInstanceID, model.CreatureID) error
	CloneObjectToCreatureInventory(model.ObjectInstanceID, model.CreatureID) (model.ObjectInstanceID, error)
	DestroyCreatureInventoryObject(model.ObjectInstanceID, model.CreatureID) (bool, error)
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
	SetCreatureStat(model.CreatureID, string, int) error
	SetObjectProperty(model.ObjectInstanceID, string, string) (model.ObjectInstance, error)
}

type TransExpWorld interface {
	Room(model.RoomID) (model.Room, bool)
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
	SetCreatureStat(model.CreatureID, string, int) error
}

type MarriageSendWorld interface {
	Creature(model.CreatureID) (model.Creature, bool)
	ActiveSessions() []string
	SessionActor(string) (string, bool)
	Player(model.PlayerID) (model.Player, bool)
}

type tradeRequest struct {
	Item          string
	ItemOrdinal   int64
	Target        string
	TargetOrdinal int64
}

type legacyTradeCarryItem struct {
	WantedNumber int
	ReturnNumber int
}

var questExp = []int{
	120, 500, 1000, 3000, 5000,
	8000, 10000, 20000, 30000, 50000,
	80000, 100000, 200000, 300000, 500000,
	700000, 800000, 900000, 1000000, 1300000,
}

func getQuestExp(questIndex int) int {
	if questIndex < 0 {
		return 0
	}
	if questIndex < len(questExp) {
		return questExp[questIndex]
	}
	return 125
}

func parseTradeRequest(resolved ResolvedCommand) (tradeRequest, bool) {
	if len(resolved.Args) < 2 {
		return tradeRequest{}, false
	}
	itemArg := getArg(resolved, 0)
	targetArg := getArg(resolved, 1)
	if itemArg == "" || targetArg == "" {
		return tradeRequest{}, false
	}
	return tradeRequest{
		Item:          itemArg,
		ItemOrdinal:   getOrdinal(resolved, 0),
		Target:        targetArg,
		TargetOrdinal: getOrdinal(resolved, 1),
	}, true
}

func findTradeCreatureTarget(
	world TradeWorld,
	room model.Room,
	viewer LookViewer,
	prefix string,
	ordinal int64,
) (model.Creature, bool) {
	return findLegacyMonsterTarget(world, room, viewer, prefix, ordinal)
}

func findLegacyMonsterTarget(
	world LookWorld,
	room model.Room,
	viewer LookViewer,
	prefix string,
	ordinal int64,
) (model.Creature, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return model.Creature{}, false
	}
	if ordinal < 1 {
		ordinal = 1
	}

	detectInvisible := viewerDetectsInvisible(world, viewer)
	var seen int64
	for _, id := range room.CreatureIDs {
		if id.IsZero() || id == viewer.CreatureID {
			continue
		}
		creature, ok := world.Creature(id)
		if !ok || creature.RoomID != room.ID || creature.Kind == model.CreatureKindPlayer {
			continue
		}
		if !legacyFindCrtVisible(creature, detectInvisible) || !legacyCreaturePrefixMatches(creature, prefix) {
			continue
		}
		seen++
		if seen == ordinal {
			return creature, true
		}
	}
	return model.Creature{}, false
}

func legacyFindCrtVisible(creature model.Creature, detectInvisible bool) bool {
	if creatureClass(creature) >= legacyClassCaretaker && creatureHasAnyFlag(creature, "PDMINV", "dmInvisible", "pdminv") {
		return false
	}
	if !detectInvisible && creatureHasAnyFlag(creature, "MINVIS", "minvis", "invisible") {
		return false
	}
	return true
}

func legacyCreaturePrefixMatches(creature model.Creature, target string) bool {
	target = cleanDisplayText(target)
	if len(target) < 2 {
		return false
	}
	for _, term := range legacyCreatureEqualTerms(creature) {
		if strings.HasPrefix(term, target) {
			return true
		}
	}
	return false
}

func legacyCreatureEqualTerms(creature model.Creature) []string {
	terms := make([]string, 0, 4)
	terms = appendTrimmedTerm(terms, creature.DisplayName)
	for _, key := range []string{"key[0]", "key[1]", "key[2]"} {
		terms = appendTrimmedTerm(terms, creature.Properties[key])
	}
	return terms
}

func findTransExpPlayerTarget(
	world TransExpWorld,
	room model.Room,
	viewer LookViewer,
	prefix string,
	ordinal int64,
) (model.Player, model.Creature, bool) {
	return findLegacyPlayerCreatureTarget(world, room, viewer, prefix, ordinal)
}

func findLegacyPlayerCreatureTarget(
	world LookWorld,
	room model.Room,
	viewer LookViewer,
	prefix string,
	ordinal int64,
) (model.Player, model.Creature, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return model.Player{}, model.Creature{}, false
	}
	if ordinal < 1 {
		ordinal = 1
	}

	detectInvisible := viewerDetectsInvisible(world, viewer)
	var seen int64
	for _, id := range room.PlayerIDs {
		if id.IsZero() || id == viewer.PlayerID {
			continue
		}
		player, ok := world.Player(id)
		if !ok || player.RoomID != room.ID {
			continue
		}
		creature, ok := world.Creature(player.CreatureID)
		if !ok {
			continue
		}
		if creature.DisplayName == "" {
			creature.DisplayName = player.DisplayName
		}
		if !legacyFindCrtVisible(creature, detectInvisible) || !legacyCreaturePrefixMatches(creature, prefix) {
			continue
		}
		seen++
		if seen == ordinal {
			return player, creature, true
		}
	}
	return model.Player{}, model.Creature{}, false
}

func matchTradeObject(world TradeWorld, instance model.ObjectInstance, wantedProtoID model.PrototypeID) bool {
	if _, ok := world.ObjectPrototype(wantedProtoID); !ok {
		return false
	}

	wanted := model.ObjectInstance{PrototypeID: wantedProtoID}
	instName := legacyObjectEqualName(world, instance)
	wantedName := legacyObjectEqualName(world, wanted)
	instKey := legacyObjectEqualKey(world, instance, 0)
	wantedKey := legacyObjectEqualKey(world, wanted, 0)

	return instName != "" && instName == wantedName && instKey != "" && instKey == wantedKey
}

func selectTradeInventoryObject(world TradeWorld, ids []model.ObjectInstanceID, target string, ordinal int64, detectInvisible bool) (model.ObjectInstanceID, string, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", "", false
	}
	if ordinal < 1 {
		ordinal = 1
	}

	var seen int64
	for _, id := range ids {
		if id.IsZero() {
			continue
		}
		object, ok := world.Object(id)
		if !ok {
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
			return id, objectDisplayName(world, object), true
		}
	}
	return "", "", false
}

func isTradeMiscObject(world TradeWorld, object model.ObjectInstance) bool {
	if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
		if proto.Kind == model.ObjectKindMisc {
			return true
		}
	}
	tVal := objectIntPropertyOrZero(world, object, "type")
	return tVal == 13 // legacyObjectMisc
}

func tradeObjectHasFlag(world TradeWorld, object model.ObjectInstance, names ...string) bool {
	return objectHasAnyTag(world, object, names...) || objectHasAnyPropertyFlag(world, object, names...)
}

func tradePlayerLegacyName(player model.Player) string {
	if name := cleanDisplayText(player.DisplayName); name != "" {
		return name
	}
	return strings.TrimPrefix(string(player.ID), "player:")
}

func creatureHasTradeFlag(creature model.Creature) bool {
	return creatureHasFlag(creature, "MTRADE", "trade")
}

func legacyCreatureFlagByte(creature model.Creature, index int) byte {
	if index >= 0 {
		if raw := creature.Metadata.RawFields["flags"]; len(raw) > index {
			return raw[index]
		}
	}
	var value byte
	for _, bit := range []int{36, 37, 38} {
		if bit/8 == index && legacyCreatureFlagSet(creature, bit) != 0 {
			value |= 1 << uint(bit%8)
		}
	}
	return value
}

func legacyCreatureFlagSet(creature model.Creature, bit int) int {
	if bit >= 0 {
		if raw := creature.Metadata.RawFields["flags"]; len(raw) > bit/8 && raw[bit/8]&(1<<uint(bit%8)) != 0 {
			return 1
		}
	}
	switch bit {
	case 36:
		if creatureHasFlag(creature, "MPURIT", "purchaseItems") {
			return 1
		}
	case 37:
		if creatureHasFlag(creature, "MTRADE", "tradeItems", "trade") {
			return 1
		}
	case 38:
		if creatureHasFlag(creature, "MPGUAR", "passiveExitGuard") {
			return 1
		}
	}
	return 0
}

func legacyTradeCarryItems(creature model.Creature) []legacyTradeCarryItem {
	var objList [5]legacyTradeCarryItem
	maxitem := 0
	for i := 0; i < 5; i++ {
		carryNum := creature.Stats[fmt.Sprintf("carry[%d]", i)]
		if carryNum <= 0 {
			continue
		}
		found := false
		for j := 0; j < maxitem; j++ {
			if carryNum == objList[j].WantedNumber {
				found = true
				break
			}
		}
		if found {
			continue
		}
		maxitem++
		objList[i] = legacyTradeCarryItem{
			WantedNumber: carryNum,
			ReturnNumber: creature.Stats[fmt.Sprintf("carry[%d]", i+5)],
		}
	}

	items := make([]legacyTradeCarryItem, 0, maxitem)
	for i := 0; i < maxitem; i++ {
		items = append(items, objList[i])
	}
	return items
}

func creatureHasFlag(creature model.Creature, names ...string) bool {
	return creatureHasAnyFlag(creature, names...)
}

func isCaretakerOrFamilyBoss(creature model.Creature) bool {
	class := creatureClass(creature)
	if class >= legacyClassCaretaker {
		return true
	}
	return creatureHasFlag(creature, "PFMBOS", "familyBoss", "familyBossFlag")
}

func addProf(world TradeWorld, creature model.Creature, exp int) error {
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
		valStr := creature.Properties[key]
		val, _ := strconv.Atoi(valStr)
		if _, err := world.SetCreatureProperty(creature.ID, key, strconv.Itoa(val+part)); err != nil {
			return err
		}
	}

	for i := 1; i <= 4; i++ {
		key := fmt.Sprintf("realm/%d", i)
		valStr := creature.Properties[key]
		val, _ := strconv.Atoi(valStr)
		if _, err := world.SetCreatureProperty(creature.ID, key, strconv.Itoa(val+part)); err != nil {
			return err
		}
	}

	return nil
}

func readSpouseName(root string, playerID model.PlayerID) (string, error) {
	cleanName := strings.TrimPrefix(string(playerID), "player:")
	encodedActorBytes, err := legacykr.EncodeEUCKR(cleanName)
	if err != nil {
		return "", err
	}
	encodedActorName := string(encodedActorBytes)

	path := filepath.Join(root, "player", "marriage", encodedActorName)
	data, err := os.ReadFile(path)
	if err != nil {
		lowerPath := filepath.Join(root, "player", "marriage", strings.ToLower(encodedActorName))
		data, err = os.ReadFile(lowerPath)
		if err != nil {
			return "", err
		}
	}

	spouseName, err := legacykr.DecodeEUCKR(data)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(spouseName), nil
}

func findSpousePlayer(world MarriageSendWorld, spouseName string) (model.Player, bool) {
	if p, ok := world.Player(model.PlayerID(spouseName)); ok {
		return p, true
	}
	if p, ok := world.Player(model.PlayerID("player:" + spouseName)); ok {
		return p, true
	}
	if p, ok := world.Player(model.PlayerID("player:" + strings.ToLower(spouseName))); ok {
		return p, true
	}
	return model.Player{}, false
}

func marriageSessionActorMatchesSpouse(actorID string, spouseName string, spouse model.Player) bool {
	actorID = strings.TrimSpace(actorID)
	spouseName = strings.TrimSpace(spouseName)
	actorName := strings.TrimPrefix(actorID, "player:")
	spouseID := strings.TrimSpace(string(spouse.ID))
	spouseIDName := strings.TrimPrefix(spouseID, "player:")
	for _, candidate := range []string{
		spouseName,
		spouseID,
		spouseIDName,
		spouse.DisplayName,
	} {
		if strings.EqualFold(actorID, candidate) || strings.EqualFold(actorName, candidate) {
			return true
		}
	}
	return false
}

func invokeBroadcast(ctx *Context, msg string) {
	if ctx == nil || ctx.Values == nil {
		return
	}
	fnVal := reflect.ValueOf(ctx.Values["game.broadcast"])
	if !fnVal.IsValid() || fnVal.IsNil() {
		return
	}
	fnType := fnVal.Type()
	if fnType.NumIn() != 1 {
		return
	}
	cmdType := fnType.In(0)
	var cmdVal reflect.Value
	if cmdType.Kind() == reflect.String {
		cmdVal = reflect.ValueOf(msg).Convert(cmdType)
	} else if cmdType.Kind() == reflect.Struct {
		cmdVal = reflect.New(cmdType).Elem()
		writeField := cmdVal.FieldByName("Write")
		if writeField.IsValid() && writeField.CanSet() {
			writeField.SetString(msg)
		}
	} else {
		return
	}
	fnVal.Call([]reflect.Value{cmdVal})
}

func invokeSendToSession(ctx *Context, targetID string, msg string) {
	if ctx == nil || ctx.Values == nil {
		return
	}
	fnVal := reflect.ValueOf(ctx.Values["game.sendToSession"])
	if !fnVal.IsValid() || fnVal.IsNil() {
		return
	}
	fnType := fnVal.Type()
	if fnType.NumIn() != 2 {
		return
	}
	idType := fnType.In(0)
	idVal := reflect.ValueOf(targetID).Convert(idType)

	cmdType := fnType.In(1)
	var cmdVal reflect.Value
	if cmdType.Kind() == reflect.String {
		cmdVal = reflect.ValueOf(msg).Convert(cmdType)
	} else if cmdType.Kind() == reflect.Struct {
		cmdVal = reflect.New(cmdType).Elem()
		writeField := cmdVal.FieldByName("Write")
		if writeField.IsValid() && writeField.CanSet() {
			writeField.SetString(msg)
		}
	} else {
		return
	}
	fnVal.Call([]reflect.Value{idVal, cmdVal})
}

func NewTradeHandler(world TradeWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		if world == nil {
			return StatusDefault, fmt.Errorf("trade: world is nil")
		}
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}

		request, ok := parseTradeRequest(resolved)
		if !ok {
			if len(resolved.Args) < 1 {
				ctx.WriteString("누구와 교환하시려구요?\n")
			} else {
				ctx.WriteString("사용법: <물건> <괴물이름> 교환\n")
			}
			return StatusDefault, nil
		}

		viewer := LookViewerFromContext(ctx)
		room, ok := world.Room(player.RoomID)
		if !ok {
			return StatusDefault, fmt.Errorf("trade: room %q not found", player.RoomID)
		}

		targetCrt, ok := findTradeCreatureTarget(world, room, viewer, request.Target, request.TargetOrdinal)
		if !ok {
			ctx.WriteString("그것은 여기 없습니다.\n")
			return StatusDefault, nil
		}

		if !creatureHasTradeFlag(targetCrt) {
			ctx.WriteString(fmt.Sprintf("당신은 %s%s 교역할 수 없습니다.\n", targetCrt.DisplayName, krtext.Particle(targetCrt.DisplayName, '2')))
			ctx.WriteString(fmt.Sprintf("%x %d %d %d\n",
				legacyCreatureFlagByte(targetCrt, 4),
				legacyCreatureFlagSet(targetCrt, 36),
				legacyCreatureFlagSet(targetCrt, 37),
				legacyCreatureFlagSet(targetCrt, 38),
			))
			return StatusDefault, nil
		}

		objectID, itemName, ok := selectTradeInventoryObject(world, creature.Inventory.ObjectIDs, request.Item, request.ItemOrdinal, viewerHasDetectInvisibleTag(creature))
		if !ok {
			ctx.WriteString("당신은 그런 물건을 갖고 있지 않습니다.\n")
			return StatusDefault, nil
		}

		object, ok := world.Object(objectID)
		if !ok {
			return StatusDefault, fmt.Errorf("trade: object %q not found", objectID)
		}

		if tradeObjectHasFlag(world, object, "named", "onamed", "ONAMED", "customName", "OCNAME") {
			ctx.WriteString("교역할 수 있는 물건이 아닙니다.\n")
			return StatusDefault, nil
		}

		tradeItems := legacyTradeCarryItems(targetCrt)
		if len(tradeItems) == 0 {
			ctx.WriteString(fmt.Sprintf("%s는 교역할 물건을 갖고있지 않습니다.\n", targetCrt.DisplayName))
			return StatusDefault, nil
		}

		foundIndex := -1
		for i, tradeItem := range tradeItems {
			if tradeItem.WantedNumber <= 0 {
				continue
			}
			if matchTradeObject(world, object, legacyCarryObjectPrototypeID(tradeItem.WantedNumber)) {
				foundIndex = i
				break
			}
		}

		shotsCurrent := objectIntPropertyOrZero(world, object, "shotsCurrent")
		if shotsCurrent == 0 {
			shotsCurrent = objectIntPropertyOrZero(world, object, "shotscur")
		}
		shotsMax := objectIntPropertyOrZero(world, object, "shotsMax")
		if shotsMax == 0 {
			shotsMax = objectIntPropertyOrZero(world, object, "shotsmax")
		}
		isMisc := isTradeMiscObject(world, object)

		if foundIndex == -1 || (shotsCurrent <= shotsMax/10 && !isMisc) {
			ctx.WriteString(fmt.Sprintf("%s가 \"난 그런거 필요없어요!\"라고 말합니다.\n", targetCrt.DisplayName))
			return StatusDefault, nil
		}

		tradeItem := tradeItems[foundIndex]
		if tradeItem.ReturnNumber == 0 {
			if _, err := world.DestroyCreatureInventoryObject(object.ID, creature.ID); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(fmt.Sprintf("%s가 \"고맙습니다! %s%s 필요했는데 잘됐군요.\n그런데 당신에게 줄게 없는데..\"라고 말합니다.\n", targetCrt.DisplayName, itemName, krtext.Particle(itemName, '1')))

			msg := fmt.Sprintf("%s이 %s에게 %s를 교환합니다.\n", tradePlayerLegacyName(player), targetCrt.DisplayName, itemName)
			if err := roomBroadcast(ctx, player.RoomID, msg); err != nil {
				return StatusDefault, err
			}
			return StatusDefault, nil
		}

		returnProtoID := legacyCarryObjectPrototypeID(tradeItem.ReturnNumber)
		trdProto, ok := world.ObjectPrototype(returnProtoID)
		if !ok {
			return StatusDefault, nil
		}

		questNumStr := trdProto.Properties["questNumber"]
		if questNumStr == "" {
			questNumStr = trdProto.Properties["questnum"]
		}
		questNum, _ := strconv.Atoi(questNumStr)

		if questNum > 0 && creature.Properties[fmt.Sprintf("quest_completed_%d", questNum)] == "1" {
			ctx.WriteString("당신은 이미 임무를 완수했습니다.\n")
			return StatusDefault, nil
		}

		if _, err := world.DestroyCreatureInventoryObject(object.ID, creature.ID); err != nil {
			return StatusDefault, err
		}

		clonedID, err := world.CloneObjectToCreatureInventory(model.ObjectInstanceID(returnProtoID), creature.ID)
		if err != nil {
			return StatusDefault, fmt.Errorf("trade: failed to clone reward item %q: %w", returnProtoID, err)
		}

		clonedObj, ok := world.Object(clonedID)
		var rewardName string
		if ok {
			if tradeObjectHasFlag(world, clonedObj, "event", "oevent", "OEVENT") {
				ownerName := tradePlayerLegacyName(player)
				updated, err := world.SetObjectProperty(clonedObj.ID, "key[2]", ownerName)
				if err != nil {
					return StatusDefault, err
				}
				clonedObj = updated
			}
			rewardName = objectDisplayName(world, clonedObj)
		} else {
			rewardName = trdProto.DisplayName
		}

		xpReward := 0
		if questNum > 0 {
			questIndex := questNum - 1
			if _, err := world.SetCreatureProperty(creature.ID, fmt.Sprintf("quest_completed_%d", questNum), "1"); err != nil {
				return StatusDefault, err
			}

			xpReward = getQuestExp(questIndex)
			newExp := creatureStat(creature, "experience") + xpReward
			if err := world.SetCreatureStat(creature.ID, "experience", newExp); err != nil {
				return StatusDefault, err
			}

			if err := addProf(world, creature, xpReward); err != nil {
				return StatusDefault, err
			}
		}

		ctx.WriteString(fmt.Sprintf("%s가 \"고맙습니다. 절 위해 %s를 찾아주시다니..\n", targetCrt.DisplayName, itemName))
		ctx.WriteString(fmt.Sprintf("당신에게 %s로 보답을 하고싶습니다.\"라고 말합니다.\n", rewardName))
		ctx.WriteString(fmt.Sprintf("%s가 당신에게 %s를 줍니다.\n", targetCrt.DisplayName, rewardName))

		if questNum > 0 {
			ctx.WriteString("임무를 완수했습니다! 버리지 마십시요!\n")
			ctx.WriteString("당신은 버리면 그걸 다시 주울 수 없습니다.\n")
			ctx.WriteString(fmt.Sprintf("당신은 경험치 %d 를 얻었습니다.\n", xpReward))
		}

		msg := fmt.Sprintf("%s이 %s에게 %s를 교환합니다.\n", tradePlayerLegacyName(player), targetCrt.DisplayName, itemName)
		if err := roomBroadcast(ctx, player.RoomID, msg); err != nil {
			return StatusDefault, err
		}

		return StatusDefault, nil
	}
}

func NewTransExpHandler(world TransExpWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		if world == nil {
			return StatusDefault, fmt.Errorf("trans_exp: world is nil")
		}
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		donorPlayer, ok := world.Player(playerID)
		if !ok {
			return StatusDefault, fmt.Errorf("trans_exp: donor player %q not found", playerID)
		}
		creature, ok := world.Creature(donorPlayer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("trans_exp: donor creature %q not found", donorPlayer.CreatureID)
		}

		amt := legacyAtoiPrefix(getArg(resolved, 0))

		if !isCaretakerOrFamilyBoss(creature) {
			ctx.WriteString("당신의 능력으로는 사용할수 없습니다.\n")
			ctx.WriteString("경험치 전수는 패거리 문주나 초인만이 가능합니다.\n")
			return StatusDefault, nil
		}

		if !creatureHasFlag(creature, "PFAMIL") {
			ctx.WriteString("패거리 가입자만 가능합니다.\n")
			return StatusDefault, nil
		}

		if len(resolved.Args) < 2 {
			ctx.WriteString("누구에게 경험치를 전수하시려고요?\n")
			ctx.WriteString("사용법 : xxx점 누구 경험치전수 \n\n")
			return StatusDefault, nil
		}

		if amt < 1000 || amt > 1000000 {
			ctx.WriteString("1000에서 1000000점 사이만 가능합니다.")
			return StatusDefault, nil
		}

		viewer := LookViewerFromContext(ctx)
		room, ok := world.Room(donorPlayer.RoomID)
		if !ok {
			return StatusDefault, fmt.Errorf("trans_exp: room %q not found", donorPlayer.RoomID)
		}

		targetName := getArg(resolved, 1)
		targetOrdinal := getOrdinal(resolved, 1)
		targetPlayer, targetCrt, ok := findTransExpPlayerTarget(world, room, viewer, targetName, targetOrdinal)
		if !ok {
			ctx.WriteString("그런 사람은 여기 없어요!")
			return StatusDefault, nil
		}

		actorFamilyID := transExpLegacyFamilyID(creature)
		targetFamilyID := transExpLegacyFamilyID(targetCrt)
		if !creatureHasFlag(targetCrt, "PFAMIL") || actorFamilyID != targetFamilyID {
			ctx.WriteString("당신의 패거리사람에게만 경험치전수가 가능합니다.")
			return StatusDefault, nil
		}

		targetClass := creatureClass(targetCrt)
		targetLevel := creatureStat(targetCrt, "level")
		if targetLevel > 80 || targetClass == legacyClassInvincible {
			ctx.WriteString("당신은 그사람에게 경험치를 줄 수 없습니다.")
			return StatusDefault, nil
		}

		actorExp := creatureStat(creature, "experience")
		if actorExp < 1000000 {
			ctx.WriteString("당신은 남에게 줄만큼의 경험치를 가지고 있지 않습니다.")
			return StatusDefault, nil
		}

		if actorExp-amt < 100000000 {
			ctx.WriteString("초인 기본 경험치 1억 이하로는 전수 할 수 없습니다.\n")
			return StatusDefault, nil
		}

		var targetGain int
		var broadcastTargetGain int

		if targetLevel > 25 {
			targetGain = amt / 20
			broadcastTargetGain = amt / 25
		} else {
			targetGain = amt / 30
			broadcastTargetGain = amt / 50
		}

		if err := world.SetCreatureStat(creature.ID, "experience", actorExp-amt); err != nil {
			return StatusDefault, err
		}

		targetExp := creatureStat(targetCrt, "experience")
		if err := world.SetCreatureStat(targetCrt.ID, "experience", targetExp+targetGain); err != nil {
			return StatusDefault, err
		}

		ctx.WriteString(fmt.Sprintf("당신은 %s님에게 자신의 경험치 %d점을 나눠주었습니다.", targetCrt.DisplayName, amt))

		targetMsg := fmt.Sprintf("\n$$$ %s님이 당신에게 경험치 %d점을 나눠주었습니다.\n", creature.DisplayName, targetGain)
		_ = sendToPlayer(ctx, targetPlayer.ID, targetMsg)

		broadcastMsg := fmt.Sprintf("\n### %s님이 경험치 %d점을 줘서 %s님은 %d점을 받았습니다.\n", creature.DisplayName, amt, targetCrt.DisplayName, broadcastTargetGain)
		invokeBroadcast(ctx, broadcastMsg)

		return StatusDefault, nil
	}
}

func transExpLegacyFamilyID(creature model.Creature) int {
	value, ok := moveCreatureStatOrPropertyInt(creature, "dailyExpndMax", "legacyDailyExpndMax", "daily_expnd_max", "familyID", "family_id")
	if !ok {
		return 0
	}
	return value
}

func hasPMARRITagOrFlag(player model.Player, creature model.Creature) bool {
	return hasAnyNormalizedFlag(player.Metadata.Tags, "PMARRI") ||
		creatureHasAnyFlag(creature, "PMARRI", "married", "marriage")
}

func NewMarriageSendHandler(world MarriageSendWorld, roots ...string) Handler {
	var root string
	if len(roots) > 0 {
		root = roots[0]
	}
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		if world == nil {
			return StatusDefault, fmt.Errorf("marriage_send: world is nil")
		}
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}

		senderPlayer, ok := world.Player(playerID)
		if !ok {
			return StatusDefault, fmt.Errorf("marriage_send: sender player %q not found", playerID)
		}
		senderCrt, ok := world.Creature(senderPlayer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("marriage_send: sender creature %q not found", senderPlayer.CreatureID)
		}

		if !hasPMARRITagOrFlag(senderPlayer, senderCrt) {
			ctx.WriteString("당신은 결혼하지 않았습니다.")
			return StatusDefault, nil
		}

		spouseName, err := readSpouseName(root, playerID)
		if err != nil || spouseName == "" {
			ctx.WriteString("당신은 결혼하지 않았습니다.")
			return StatusDefault, nil
		}

		spousePlayer, ok := findSpousePlayer(world, spouseName)
		if !ok {
			ctx.WriteString("당신의 배우자는 지금 이용중이 아닙니다.")
			return StatusDefault, nil
		}
		spouseCrt, ok := world.Creature(spousePlayer.CreatureID)
		if !ok {
			ctx.WriteString("당신의 배우자는 지금 이용중이 아닙니다.")
			return StatusDefault, nil
		}

		active := world.ActiveSessions()
		var spouseSessionID string
		var spouseFound bool
		for _, sID := range active {
			actorID, ok := world.SessionActor(sID)
			if ok && marriageSessionActorMatchesSpouse(actorID, spouseName, spousePlayer) {
				spouseSessionID = sID
				spouseFound = true
				break
			}
		}

		if !spouseFound {
			ctx.WriteString("당신의 배우자는 지금 이용중이 아닙니다.")
			return StatusDefault, nil
		}

		message := extractCommandMessage(resolved)
		if message == "" {
			message = strings.TrimSpace(joinArgs(resolved.Args))
		}
		if message == "" {
			ctx.WriteString("무슨 말을 전하시려고요?")
			return StatusDefault, nil
		}

		msgToSpouse := fmt.Sprintf("\n%s%s 당신에게 \"%s\"라고 이야기합니다.", senderCrt.DisplayName, krtext.Particle(senderCrt.DisplayName, '1'), message)
		invokeSendToSession(ctx, spouseSessionID, msgToSpouse)

		if creatureHasFlag(senderCrt, "PLECHO") {
			ctx.WriteString(fmt.Sprintf("당신은 %s에게 \"%s\"라고 이야기합니다.", spouseCrt.DisplayName, message))
		} else {
			ctx.WriteString(fmt.Sprintf("%s님에게 말을 전달하였습니다.", spouseCrt.DisplayName))
		}
		return StatusDefault, nil
	}
}
