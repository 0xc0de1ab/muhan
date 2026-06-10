package command

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const burnCooldownKey = "legacy:burn"

type BurnWorld interface {
	StatusWorld
	DestroyObject(model.ObjectInstanceID) error
	SetCreatureStat(model.CreatureID, string, int) error
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
}

var legacyBurnRoll SearchRollFunc = func(min int, max int) int {
	if max <= min {
		return min
	}
	return min + rand.Intn(max-min+1)
}

type MonsterPurchaseWorld interface {
	StatusWorld
	PurchaseObjectToCreatureInventory(model.ObjectInstanceID, model.CreatureID, int) (model.ObjectInstanceID, int, bool, error)
}

func NewBurnHandler(world BurnWorld) Handler {
	return NewBurnHandlerWithRoot(world, "")
}

func NewBurnHandlerWithRoot(world BurnWorld, root string) Handler {
	root = strings.TrimSpace(root)
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}

		target := getArg(resolved, 0)
		if target == "" {
			ctx.WriteString("무엇을 태우시려구요?")
			return StatusDefault, nil
		}
		now := time.Now().Unix()
		if remaining, available, err := world.UseCreatureCooldown(creature.ID, burnCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !available {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}
		player, creature, err = clearBurnActorHidden(world, player, creature)
		if err != nil {
			return StatusDefault, err
		}
		objectID, name, ok := selectDropObject(world, creature.Inventory.ObjectIDs, target, getOrdinal(resolved, 0))
		if !ok {
			ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
			return StatusDefault, nil
		}
		object, ok := world.Object(objectID)
		if !ok {
			return StatusDefault, fmt.Errorf("burn: object %q not found", objectID)
		}

		if isLegacyPlazaRoom(player.RoomID) {
			ctx.WriteString("광장에서는 소각할 수 없습니다.")
			return StatusDefault, nil
		}
		if objectHasAnyFlagOrProperty(world, object, "noBurn", "noburn", "ONOBUN") {
			ctx.WriteString("소각할수 없는 아이템입니다.")
			return StatusDefault, nil
		}
		if burnProtectedQuestObject(world, creature, object) {
			ctx.WriteString("임무 아이템은 태우지 못합니다.")
			return StatusDefault, nil
		}
		if burnProtectedEventObject(world, creature, object) {
			ctx.WriteString("이벤트 아이템은 소각할수 없습니다.")
			return StatusDefault, nil
		}
		if !objectLocatedInCreatureInventory(object, creature.ID) {
			ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
			return StatusDefault, nil
		}
		unlinkLegacyBurnMailScroll(root, world, object)
		if err := world.DestroyObject(objectID); err != nil {
			return StatusDefault, err
		}

		goldReward := 1
		experienceReward := 1
		ctx.WriteString(fmt.Sprintf("당신은 %s%s 태웠습니다.\n", name, krtext.Particle(name, '3')))
		_ = roomBroadcast(ctx, player.RoomID, fmt.Sprintf("\n%s%s %s%s 태웠습니다.", commandActorDisplayName(player, creature), krtext.Particle(commandActorDisplayName(player, creature), '1'), name, krtext.Particle(name, '3')))
		ctx.WriteString("당신은 약간의 상금과 경험을 받았습니다.")
		if legacyBurnRoll(1, 3000) == 1 {
			actorName := commandActorDisplayName(player, creature)
			if creatureClass(creature) >= model.ClassInvincible {
				ctx.WriteString("\n신이 당신의 정성이 갸륵해서 엄청난 경험치와 돈벼락을 내립니다.")
				invokeBroadcast(ctx, fmt.Sprintf("\n### 신이 %s에게 엄청난 경험치와 돈벼락을 내립니다.\n", actorName))
				goldReward += 3000000
				experienceReward += 300000
			} else {
				ctx.WriteString("\n신이 당신의 정성이 갸륵해서 경험치와 돈벼락을 내립니다.")
				invokeBroadcast(ctx, fmt.Sprintf("\n### 신이 %s에게 경험치와 돈벼락을 내립니다.\n", actorName))
				goldReward += 100000
				experienceReward += 10000
			}
		}
		if err := world.SetCreatureStat(creature.ID, "gold", creatureStat(creature, "gold")+goldReward); err != nil {
			return StatusDefault, err
		}
		if err := world.SetCreatureStat(creature.ID, "experience", creatureStat(creature, "experience")+experienceReward); err != nil {
			return StatusDefault, err
		}
		if err := world.SetCreatureCooldown(creature.ID, burnCooldownKey, now, 3); err != nil {
			return StatusDefault, err
		}
		return StatusDefault, nil
	}
}

func unlinkLegacyBurnMailScroll(root string, world InventoryWorld, object model.ObjectInstance) {
	if root == "" {
		return
	}
	if objectLegacyTypeOrKind(world, object) != legacyObjectScroll {
		return
	}
	if objectIntPropertyOrDefault(world, object, "adjustment", "adjust") != -100 {
		return
	}
	name := comboObjectUseOutput(world, object)
	if name == "" {
		return
	}
	path, ok := safePostPath(root, name)
	if !ok {
		return
	}
	_ = os.Remove(path)
}

func clearBurnActorHidden(world BurnWorld, player model.Player, creature model.Creature) (model.Player, model.Creature, error) {
	updatedCreature, err := world.UpdateCreatureTags(creature.ID, nil, []string{"hidden", "phiddn", "PHIDDN"})
	if err != nil {
		return player, creature, err
	}
	creature = updatedCreature
	if creature.Stats != nil {
		if _, ok := creature.Stats["PHIDDN"]; ok {
			if err := world.SetCreatureStat(creature.ID, "PHIDDN", 0); err != nil {
				return player, creature, err
			}
			creature.Stats["PHIDDN"] = 0
		}
	}
	if !player.ID.IsZero() {
		updatedPlayer, err := world.UpdatePlayerTags(player.ID, nil, []string{"hidden", "phiddn", "PHIDDN"})
		if err != nil {
			return player, creature, err
		}
		player = updatedPlayer
	}
	return player, creature, nil
}

func NewMonsterSelectionHandler(world StatusWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		target := getArg(resolved, 0)
		ordinal := getOrdinal(resolved, 0)
		if target == "" {
			ctx.WriteString("누구의 물건을 봅니까?\n")
			return StatusDefault, nil
		}
		vendor, ok := findLegacyMonsterTarget(world, room, viewer, target, ordinal)
		if !ok {
			ctx.WriteString("그런 사람은 없습니다.\n")
			return StatusDefault, nil
		}
		vendorName := attackCreatureName(vendor)
		if !creatureHasAnyFlag(vendor, "purchaseItems", "MPURIT") {
			ctx.WriteString(fmt.Sprintf("%s%s 아무것도 없습니다.\n", vendorName, krtext.Particle(vendorName, '0')))
			return StatusDefault, nil
		}

		items := monsterVendorStock(world, vendor)
		if len(items) == 0 {
			ctx.WriteString(fmt.Sprintf("%s%s 팔 물건이 없습니다.\n", vendorName, krtext.Particle(vendorName, '0')))
			return StatusDefault, nil
		}

		ctx.WriteString(RenderMonsterSelection(vendorName, items))
		return StatusDefault, nil
	}
}

func NewMonsterPurchaseHandler(world MonsterPurchaseWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, buyer, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}

		if len(resolved.Args) < 1 {
			ctx.WriteString("무엇을 구입하시려구요?\n")
			return StatusDefault, nil
		}
		if len(resolved.Args) < 2 {
			ctx.WriteString("사용법: <괴물이름> <물건> 구입\n")
			return StatusDefault, nil
		}

		vendorNameArg := getArg(resolved, 0)
		vendor, ok := findLegacyMonsterTarget(world, room, viewer, vendorNameArg, getOrdinal(resolved, 0))
		if !ok {
			ctx.WriteString("그것은 여기 없습니다.\n")
			return StatusDefault, nil
		}
		vendorName := attackCreatureName(vendor)
		if !creatureHasAnyFlag(vendor, "purchaseItems", "MPURIT") {
			ctx.WriteString(fmt.Sprintf("당신은 %s에게서 물건을 구입할 수 없습니다.\n", vendorName))
			return StatusDefault, nil
		}
		if len(monsterVendorStock(world, vendor)) == 0 {
			ctx.WriteString(fmt.Sprintf("%s%s 팔 물건을 갖고 있지 않습니다.\n", vendorName, krtext.Particle(vendorName, '0')))
			return StatusDefault, nil
		}

		item, ok := findMonsterVendorStockItem(world, vendor, monsterPurchaseItemTarget(resolved), monsterPurchaseItemOrdinal(resolved), viewerHasDetectInvisibleTag(buyer))
		if !ok || item.Missing {
			ctx.WriteString(fmt.Sprintf("%s%s \"미안합니다. 그런 물건은 갖고 있지 않습니다.\"라고 말합니다.\n", vendorName, krtext.Particle(vendorName, '1')))
			return StatusDefault, nil
		}
		if monsterPurchaseCapacityExceeded(world, buyer, item.Object) {
			ctx.WriteString(getInventoryFullMessage())
			return StatusDefault, nil
		}

		price := maxInt(10, item.Price)
		_, _, affordable, err := world.PurchaseObjectToCreatureInventory(item.Object.ID, buyer.ID, price)
		if err != nil {
			return StatusDefault, fmt.Errorf("monster purchase: purchase object %q: %w", item.Object.ID, err)
		}
		if !affordable {
			ctx.WriteString(fmt.Sprintf("%s%s \"%d냥입니다. 깎아줄순 없습니다.\"라고 말합니다.\n", vendorName, krtext.Particle(vendorName, '1'), price))
			return StatusDefault, nil
		}

		ctx.WriteString(fmt.Sprintf("당신은 %s에게 %d냥을 줍니다.\n", vendorName, price))
		ctx.WriteString(fmt.Sprintf("%s%s \"고맙습니다. 여기 %s%s 있습니다.\"라고 말합니다.\n", vendorName, krtext.Particle(vendorName, '1'), item.Name, krtext.Particle(item.Name, '1')))
		_ = roomBroadcast(ctx, player.RoomID, fmt.Sprintf("%s이 %s에게 %s를 구입할 돈 %d냥을 줍니다.\n", commandActorDisplayName(player, buyer), vendorName, item.Name, price))
		queueShopPlayerSave(world, playerID)
		return StatusDefault, nil
	}
}

type monsterVendorStockItem struct {
	Object        model.ObjectInstance
	Name          string
	Price         int
	Missing       bool
	PrototypeOnly bool
}

func RenderMonsterSelection(vendorName string, items []monsterVendorStockItem) string {
	vendorName = strings.TrimSpace(vendorName)
	if vendorName == "" {
		vendorName = "상인"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s의 물건들:\n", vendorName)
	for i, item := range items {
		if item.Missing {
			fmt.Fprintf(&b, "%d) 매진.\n", i+1)
			continue
		}
		fmt.Fprintf(&b, "%d) %s    %d냥\n", i+1, legacyLeftWidthBytes(item.Name, 22), maxInt(10, item.Price))
	}
	b.WriteString("\n")
	return b.String()
}

func monsterVendorStock(world StatusWorld, vendor model.Creature) []monsterVendorStockItem {
	if items := monsterVendorCarryStock(world, vendor); len(items) > 0 {
		return items
	}
	return monsterVendorInventoryStock(world, vendor)
}

func monsterVendorCarryStock(world StatusWorld, vendor model.Creature) []monsterVendorStockItem {
	maxitem := legacyMonsterCarryMaxItem(vendor)
	if maxitem == 0 {
		return nil
	}
	items := make([]monsterVendorStockItem, 0, maxitem)
	for i := 0; i < maxitem; i++ {
		number := creatureStat(vendor, fmt.Sprintf("carry[%d]", i))
		if number <= 0 {
			items = append(items, monsterVendorStockItem{Missing: true})
			continue
		}

		prototypeID := legacyCarryObjectPrototypeID(number)
		proto, ok := world.ObjectPrototype(prototypeID)
		if !ok {
			items = append(items, monsterVendorStockItem{Missing: true})
			continue
		}
		object := model.ObjectInstance{
			ID:          model.ObjectInstanceID(fmt.Sprintf("legacy-carry:%s:%d", vendor.ID, number)),
			PrototypeID: proto.ID,
			Quantity:    1,
			Location:    model.ObjectLocation{CreatureID: vendor.ID},
		}
		items = append(items, monsterVendorStockItem{
			Object:        object,
			Name:          objectDisplayName(world, object),
			Price:         shopObjectValue(world, object),
			PrototypeOnly: true,
		})
	}
	return items
}

func legacyMonsterCarryMaxItem(vendor model.Creature) int {
	objNum := make([]int, 10)
	maxitem := 0
	for i := 0; i < 10; i++ {
		number := creatureStat(vendor, fmt.Sprintf("carry[%d]", i))
		if number <= 0 {
			continue
		}
		found := false
		for j := 0; j < maxitem; j++ {
			if number == objNum[j] {
				found = true
				break
			}
		}
		if found {
			continue
		}
		maxitem++
		objNum[i] = number
	}
	return maxitem
}

func monsterVendorInventoryStock(world StatusWorld, vendor model.Creature) []monsterVendorStockItem {
	items := make([]monsterVendorStockItem, 0, len(vendor.Inventory.ObjectIDs))
	seen := map[string]struct{}{}
	for _, objectID := range vendor.Inventory.ObjectIDs {
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInCreatureInventory(object, vendor.ID) {
			continue
		}
		key := monsterVendorInventoryStockKey(world, object)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, monsterVendorStockItem{
			Object: object,
			Name:   objectDisplayName(world, object),
			Price:  shopObjectValue(world, object),
		})
	}
	return items
}

func findMonsterVendorStockItem(world StatusWorld, vendor model.Creature, target string, ordinal int64, detectInvisible bool) (monsterVendorStockItem, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return monsterVendorStockItem{}, false
	}
	if ordinal < 1 {
		ordinal = 1
	}
	items := monsterVendorStock(world, vendor)
	return findMonsterVendorStockItemBy(world, items, ordinal, func(term string) bool {
		return strings.HasPrefix(term, target)
	}, detectInvisible)
}

func findMonsterVendorStockItemBy(world StatusWorld, items []monsterVendorStockItem, ordinal int64, match func(string) bool, detectInvisible bool) (monsterVendorStockItem, bool) {
	var seen int64
	for _, item := range items {
		if item.Missing {
			continue
		}
		if !detectInvisible && objectHasAnyFlagOrProperty(world, item.Object, "invisible", "oinvis", "OINVIS") {
			continue
		}
		if !monsterVendorStockItemMatches(world, item, match) {
			continue
		}
		seen++
		if seen == ordinal {
			return item, true
		}
	}
	return monsterVendorStockItem{}, false
}

func monsterPurchaseCapacityExceeded(world StatusWorld, creature model.Creature, object model.ObjectInstance) bool {
	if monsterPurchaseInventoryCount(world, creature) > 150 {
		return true
	}
	return getTakeCreatureCarriedWeight(world, creature)+getTakeObjectTotalWeight(world, object) > getTakeCreatureMaxWeight(creature)
}

func monsterPurchaseInventoryCount(world StatusWorld, creature model.Creature) int {
	total := 0
	for _, objectID := range creature.Inventory.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		total++
		if object, ok := world.Object(objectID); ok && objectIsContainer(world, object) {
			total += getTakeObjectContainerCount(world, object)
		}
	}
	if total > 200 {
		return 200
	}
	return total
}

func monsterVendorStockItemMatches(world StatusWorld, item monsterVendorStockItem, match func(string) bool) bool {
	for _, term := range legacyObjectEqualTerms(world, item.Object) {
		if match(term) {
			return true
		}
	}
	return false
}

func monsterPurchaseItemTarget(resolved ResolvedCommand) string {
	if len(resolved.Args) <= 1 {
		return ""
	}
	return getArg(resolved, 1)
}

func monsterPurchaseItemOrdinal(resolved ResolvedCommand) int64 {
	if len(resolved.Args) <= 1 {
		return 1
	}
	return getOrdinal(resolved, 1)
}

func monsterVendorInventoryStockKey(world StatusWorld, object model.ObjectInstance) string {
	if !object.PrototypeID.IsZero() {
		return "prototype:" + string(object.PrototypeID)
	}
	return "name:" + objectDisplayName(world, object)
}

func legacyCarryObjectPrototypeID(number int) model.PrototypeID {
	return model.PrototypeID(fmt.Sprintf("object:o%02d:%d", number/100, number%100))
}

func burnProtectedQuestObject(world StatusWorld, creature model.Creature, object model.ObjectInstance) bool {
	if creatureClass(creature) >= model.ClassSubDM {
		return false
	}
	if objectIntPropertyOrZero(world, object, "questNumber") == 0 &&
		objectIntPropertyOrZero(world, object, "questnum") == 0 {
		return false
	}
	return objectIntPropertyOrZero(world, object, "shotsCurrent") > 0 ||
		objectIntPropertyOrZero(world, object, "shotscur") > 0
}

func burnProtectedEventObject(world StatusWorld, creature model.Creature, object model.ObjectInstance) bool {
	if creatureClass(creature) >= model.ClassSubDM {
		return false
	}
	if !objectHasAnyFlagOrProperty(world, object, "event", "OEVENT") {
		return false
	}
	return objectIntPropertyOrZero(world, object, "shotsCurrent") > 0 ||
		objectIntPropertyOrZero(world, object, "shotscur") > 0
}

func isLegacyPlazaRoom(roomID model.RoomID) bool {
	switch strings.TrimSpace(string(roomID)) {
	case "room:01001", "r01001", "1001":
		return true
	default:
		return false
	}
}
