package command

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

type ShopWorld interface {
	StatusWorld
}

type ShopBuyWorld interface {
	ShopWorld
	PurchaseObjectToCreatureInventory(model.ObjectInstanceID, model.CreatureID, int) (model.ObjectInstanceID, int, bool, error)
}

type ShopSellWorld interface {
	ShopWorld
	SellObjectFromCreatureInventory(model.ObjectInstanceID, model.CreatureID, int) (int, bool, error)
}

func NewShopListHandler(world ShopWorld) Handler {
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, _, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		room, ok := world.Room(player.RoomID)
		if !ok {
			return StatusDefault, fmt.Errorf("shop list: room %q not found", player.RoomID)
		}
		if !roomHasAnyFlag(room, "shoppe", "shop") {
			ctx.WriteString("여기는 상점이 아닙니다.")
			return StatusDefault, nil
		}
		stockRoomID, ok := nextLegacyRoomID(room.ID)
		if !ok {
			ctx.WriteString("살 물건이 없습니다.")
			return StatusDefault, nil
		}
		stockRoom, ok := world.Room(stockRoomID)
		if !ok || len(stockRoom.Objects.ObjectIDs) == 0 {
			ctx.WriteString("살 물건이 없습니다.")
			return StatusDefault, nil
		}
		ctx.WriteString(RenderShopList(ctx, world, stockRoom))
		return StatusDefault, nil
	}
}

func NewShopBuyHandler(world ShopBuyWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		room, ok := world.Room(player.RoomID)
		if !ok {
			return StatusDefault, fmt.Errorf("shop buy: room %q not found", player.RoomID)
		}
		if !roomHasAnyFlag(room, "shoppe", "shop") {
			ctx.WriteString("여기는 상점이 아닙니다.")
			return StatusDefault, nil
		}

		target := getArg(resolved, 0)
		if target == "" {
			ctx.WriteString("무엇을 사시려구요?")
			return StatusDefault, nil
		}

		stockRoom, ok := shopStockRoom(world, room)
		if !ok {
			ctx.WriteString("살 물건이 없습니다.")
			return StatusDefault, nil
		}
		object, name, ok := findShopStockObject(world, stockRoom, target, getOrdinal(resolved, 0))
		if !ok {
			ctx.WriteString("그런 물건은 팔지 않습니다.")
			return StatusDefault, nil
		}

		price := shopObjectValue(world, object)
		if creature.Stats["gold"] < price {
			ctx.WriteString("돈도 없으면서... 외상사절!")
			return StatusDefault, nil
		}
		if shopBuyCapacityExceeded(world, creature, object) {
			ctx.WriteString("당신은 더이상 가질 수 없습니다.")
			return StatusDefault, nil
		}
		if err := clearShopActorHidden(world, player, creature); err != nil {
			return StatusDefault, err
		}
		_, _, affordable, err := world.PurchaseObjectToCreatureInventory(object.ID, creature.ID, price)
		if err != nil {
			return StatusDefault, fmt.Errorf("shop buy: purchase object %q: %w", object.ID, err)
		}
		if !affordable {
			ctx.WriteString("돈도 없으면서... 외상사절!")
			return StatusDefault, nil
		}

		queueShopPlayerSave(world, player.ID)
		ctx.WriteString(RenderShopBuyConfirmation(name, price))
		_ = roomBroadcast(ctx, room.ID, "\n"+commandActorDisplayName(player, creature)+"이 "+name+krtext.Particle(name, '3')+" 샀습니다.")
		return StatusDefault, nil
	}
}

func shopBuyCapacityExceeded(world ShopWorld, creature model.Creature, object model.ObjectInstance) bool {
	return shopBuyHeldCount(creature) > 200 ||
		getTakeCreatureCarriedWeight(world, creature)+getTakeObjectTotalWeight(world, object) > getTakeCreatureMaxWeight(creature)
}

func shopBuyHeldCount(creature model.Creature) int {
	inventoryCount := 0
	for _, objectID := range creature.Inventory.ObjectIDs {
		if !objectID.IsZero() {
			inventoryCount++
		}
	}
	if inventoryCount > 200 {
		inventoryCount = 200
	}
	equipmentCount := 0
	for _, objectID := range creature.Equipment {
		if !objectID.IsZero() {
			equipmentCount++
		}
	}
	return inventoryCount + equipmentCount
}

func NewShopSellHandler(world ShopSellWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		room, ok := world.Room(player.RoomID)
		if !ok {
			return StatusDefault, fmt.Errorf("shop sell: room %q not found", player.RoomID)
		}
		if !roomHasAnyFlag(room, "pawnShop", "pawn", "pawns", "rpawns") {
			ctx.WriteString("여기는 전당포가 아닙니다.")
			return StatusDefault, nil
		}

		target := getArg(resolved, 0)
		if target == "" {
			ctx.WriteString("무엇을 파시려구요?")
			return StatusDefault, nil
		}

		if err := clearShopActorHidden(world, player, creature); err != nil {
			return StatusDefault, err
		}

		objectID, name, ok := selectShopInventoryObject(world, creature.ID, creature.Inventory.ObjectIDs, target, getOrdinal(resolved, 0), inventoryViewerDetectsInvisible(player, creature))
		if !ok {
			ctx.WriteString("당신은 그런 물건을 갖고 있지 않습니다.")
			return StatusDefault, nil
		}
		object, ok := world.Object(objectID)
		if !ok {
			return StatusDefault, fmt.Errorf("shop sell: object %q not found", objectID)
		}
		if message := shopSellRejectMessage(world, object); message != "" {
			ctx.WriteString(message)
			return StatusDefault, nil
		}

		price := shopPawnValue(world, object)
		payout := price
		bonus := shopSellBonus(ctx)
		if bonus {
			payout *= 2
		}
		_, sold, err := world.SellObjectFromCreatureInventory(objectID, creature.ID, payout)
		if err != nil {
			return StatusDefault, fmt.Errorf("shop sell: sell object %q: %w", objectID, err)
		}
		if !sold {
			ctx.WriteString("당신은 그런 물건을 갖고 있지 않습니다.")
			return StatusDefault, nil
		}

		queueShopPlayerSave(world, player.ID)
		if bonus {
			ctx.WriteString(RenderShopSellBonusConfirmation(name, payout))
			_ = roomBroadcast(ctx, room.ID, "\n"+commandActorDisplayName(player, creature)+"이 전당포주인에게  "+name+krtext.Particle(name, '3')+" 팝니다.")
		} else {
			ctx.WriteString(RenderShopSellConfirmation(name, price))
			_ = roomBroadcast(ctx, room.ID, "\n"+commandActorDisplayName(player, creature)+"이 전당포주인에게 "+name+krtext.Particle(name, '3')+" 팝니다.")
		}
		return StatusDefault, nil
	}
}

func NewShopValueHandler(world ShopWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		room, ok := world.Room(player.RoomID)
		if !ok {
			return StatusDefault, fmt.Errorf("shop value: room %q not found", player.RoomID)
		}

		mode, ok := shopValueMode(room)
		if !ok {
			ctx.WriteString("전당포에 가셔서 물건의 가치를 알아보세요.")
			return StatusDefault, nil
		}

		target := getArg(resolved, 0)
		if target == "" {
			ctx.WriteString("어떤 물건의 가치를 알고 싶으세요?")
			return StatusDefault, nil
		}

		if err := clearShopActorHidden(world, player, creature); err != nil {
			return StatusDefault, err
		}

		objectID, name, ok := selectShopInventoryObject(world, creature.ID, creature.Inventory.ObjectIDs, target, getOrdinal(resolved, 0), inventoryViewerDetectsInvisible(player, creature))
		if !ok {
			ctx.WriteString("당신은 그런 물건을 갖고 있지 않습니다.")
			return StatusDefault, nil
		}
		object, ok := world.Object(objectID)
		if !ok {
			return StatusDefault, fmt.Errorf("shop value: object %q not found", objectID)
		}

		ctx.WriteString(RenderShopValue(name, shopObjectValue(world, object), mode))
		_ = roomBroadcast(ctx, room.ID, "\n"+commandActorDisplayName(player, creature)+"이 "+name+"의 가치를 알아봅니다.")
		return StatusDefault, nil
	}
}

func selectShopInventoryObject(world ShopWorld, creatureID model.CreatureID, ids []model.ObjectInstanceID, target string, ordinal int64, detectInvisible bool) (model.ObjectInstanceID, string, bool) {
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
		if !ok || !objectLocatedInCreatureInventory(object, creatureID) {
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

func clearShopActorHidden(world ShopWorld, player model.Player, creature model.Creature) error {
	if updater, ok := any(world).(interface {
		UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	}); ok {
		if _, err := updater.UpdateCreatureTags(creature.ID, nil, []string{"hidden", "phiddn", "PHIDDN"}); err != nil {
			return err
		}
	}
	if !player.ID.IsZero() {
		if updater, ok := any(world).(interface {
			UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
		}); ok {
			if _, err := updater.UpdatePlayerTags(player.ID, nil, []string{"hidden", "phiddn", "PHIDDN"}); err != nil {
				return err
			}
		}
	}
	if creature.Stats != nil && creature.Stats["PHIDDN"] != 0 {
		if setter, ok := any(world).(interface {
			SetCreatureStat(model.CreatureID, string, int) error
		}); ok {
			return setter.SetCreatureStat(creature.ID, "PHIDDN", 0)
		}
	}
	return nil
}

type shopListItem struct {
	Name  string
	Price int
}

func RenderShopList(ctx *Context, world ShopWorld, stockRoom model.Room) string {
	items := shopListItems(world, stockRoom)
	if len(items) == 0 {
		return "살 물건이 없습니다."
	}

	var b strings.Builder
	b.WriteString("상품들:")
	for _, item := range items {
		fmt.Fprintf(&b, "\n   %-30s   가격: %d", item.Name, item.Price)
	}
	return b.String()
}

func shopListItems(world ShopWorld, stockRoom model.Room) []shopListItem {
	items := make([]shopListItem, 0, len(stockRoom.Objects.ObjectIDs))
	for _, objectID := range stockRoom.Objects.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInRoom(object, stockRoom.ID) {
			continue
		}
		items = append(items, shopListItem{
			Name:  objectDisplayName(world, object),
			Price: shopObjectValue(world, object),
		})
	}
	return items
}

func shopStockRoom(world ShopWorld, shopRoom model.Room) (model.Room, bool) {
	stockRoomID, ok := nextLegacyRoomID(shopRoom.ID)
	if !ok {
		return model.Room{}, false
	}
	stockRoom, ok := world.Room(stockRoomID)
	if !ok || len(stockRoom.Objects.ObjectIDs) == 0 {
		return model.Room{}, false
	}
	return stockRoom, true
}

func findShopStockObject(world ShopWorld, stockRoom model.Room, target string, ordinal int64) (model.ObjectInstance, string, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return model.ObjectInstance{}, "", false
	}
	if ordinal < 1 {
		ordinal = 1
	}

	var seen int64
	for _, objectID := range stockRoom.Objects.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInRoom(object, stockRoom.ID) || !objectVisibleInRoomLook(world, object, false) {
			continue
		}
		if !legacyObjectPrefixMatches(world, object, target) {
			continue
		}
		seen++
		if seen == ordinal {
			return object, objectDisplayName(world, object), true
		}
	}
	return model.ObjectInstance{}, "", false
}

func queueShopPlayerSave(world any, playerID model.PlayerID) {
	if playerID.IsZero() {
		return
	}
	if saver, ok := world.(interface {
		MarkPlayerDirty(model.PlayerID)
		QueueSave(model.PlayerID, model.BankID)
	}); ok {
		saver.MarkPlayerDirty(playerID)
		saver.QueueSave(playerID, "")
		return
	}
	if saver, ok := world.(interface{ SavePlayer(model.PlayerID) error }); ok {
		_ = saver.SavePlayer(playerID)
	}
}

type shopValueKind int

const (
	shopValuePawn shopValueKind = iota + 1
	shopValueRepair
)

func shopValueMode(room model.Room) (shopValueKind, bool) {
	if roomHasAnyFlag(room, "pawnShop", "pawn", "pawns", "rpawns") {
		return shopValuePawn, true
	}
	if roomHasAnyFlag(room, "repair", "repairShop", "rrepai") {
		return shopValueRepair, true
	}
	return 0, false
}

func RenderShopValue(name string, value int, mode shopValueKind) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "물건"
	}
	switch mode {
	case shopValuePawn:
		return fmt.Sprintf("상점주인이 \"%s이라면 %d냥  정도는 드릴 수  있어요.\"라고 말합니다.", name, minInt(value/2, 100000))
	case shopValueRepair:
		return fmt.Sprintf("상점주인이 \"%s%s 수리하는데 %d냥이 듭니다.\"라고 말합니다.", name, krtext.Particle(name, '3'), value/4)
	default:
		return fmt.Sprintf("상점주인이 \"%s이라면 %d냥  정도는 드릴 수  있어요.\"라고 말합니다.", name, minInt(value/2, 100000))
	}
}

func RenderShopBuyConfirmation(name string, price int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "물건"
	}
	return fmt.Sprintf("당신은 %s%s 샀습니다", name, krtext.Particle(name, '3'))
}

func RenderShopSellConfirmation(name string, price int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "물건"
	}
	return fmt.Sprintf("제가 %s%s 사죠.\n전당포주인이 당신에게 %d냥을 줍니다.", name, krtext.Particle(name, '3'), price)
}

func RenderShopSellBonusConfirmation(name string, payout int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "물건"
	}
	return fmt.Sprintf("제가 %s%s 사죠.\n 오늘은 기분이 좋으니 두배로 드리죠.\n 전당포주인이 당신에게 %d냥을 줍니다.", name, krtext.Particle(name, '3'), payout)
}

func formatThousands(value int) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	digits := strconv.Itoa(value)
	if len(digits) <= 3 {
		return sign + digits
	}

	first := len(digits) % 3
	if first == 0 {
		first = 3
	}
	var b strings.Builder
	b.Grow(len(sign) + len(digits) + (len(digits)-1)/3)
	b.WriteString(sign)
	b.WriteString(digits[:first])
	for i := first; i < len(digits); i += 3 {
		b.WriteByte(',')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

func shopObjectValue(world InventoryWorld, object model.ObjectInstance) int {
	if value, ok := objectIntProperty(world, object, "value"); ok {
		return value
	}
	return 0
}

func shopPawnValue(world InventoryWorld, object model.ObjectInstance) int {
	return minInt(shopObjectValue(world, object)/2, 100000)
}

func shopSellBonus(ctx *Context) bool {
	if ctx != nil && ctx.Values != nil {
		switch value := ctx.Values[ContextShopSellBonusKey].(type) {
		case bool:
			return value
		case func() bool:
			return value()
		}
	}
	return (time.Now().Unix()+int64(rand.Intn(100)+1))%250 == 9
}

const (
	legacySellTypeMissile = 4
	legacySellTypeArmor   = 5
	legacySellTypePotion  = 6
	legacySellTypeScroll  = 7
	legacySellTypeWand    = 8
	legacySellTypeKey     = 11
)

func shopSellRejectMessage(world InventoryWorld, object model.ObjectInstance) string {
	legacyType := objectLegacyType(world, object)
	if shopPawnValue(world, object) < 20 || shopSellPoorQuality(world, object, legacyType) {
		return "전당포주인이 \"그런 쓰레기는 안사요!\"라고 말합니다."
	}
	if objectHasAnyTag(world, object, "event", "oevent") || objectHasAnyPropertyFlag(world, object, "event", "oevent", "OEVENT") {
		return "전당포주인이 \"그런건 안사요!\"라고 말합니다."
	}
	if len(object.Contents.ObjectIDs) != 0 {
		return "전당포주인이 \"그 안에 뭔가가 들어있군요.\"라고 말합니다."
	}
	if legacyType == legacySellTypePotion || legacyType == legacySellTypeScroll ||
		objectKindIs(world, object, model.ObjectKindPotion) || objectKindIs(world, object, model.ObjectKindScroll) {
		return "전당포주인이 \"두루마기나 독약같은것은 안사요!\"라고 말합니다."
	}
	return ""
}

func shopSellPoorQuality(world InventoryWorld, object model.ObjectInstance, legacyType int) bool {
	shotsCurrent, hasCurrent := objectIntProperty(world, object, "shotsCurrent")
	shotsMax, hasMax := objectIntProperty(world, object, "shotsMax")
	if legacyType >= 0 && (legacyType <= legacySellTypeMissile || legacyType == legacySellTypeArmor) {
		return hasCurrent && hasMax && shotsMax > 0 && shotsCurrent <= shotsMax/8
	}
	if legacyType == legacySellTypeWand || legacyType == legacySellTypeKey {
		return !hasCurrent || shotsCurrent < 1
	}
	return false
}

func objectHasAnyPropertyFlag(world InventoryWorld, object model.ObjectInstance, keys ...string) bool {
	targets := normalizedFlagSet(keys...)
	if objectPropertiesHaveAnyFlag(object.Properties, targets) {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := world.ObjectPrototype(object.PrototypeID)
	if !ok {
		return false
	}
	return objectPropertiesHaveAnyFlag(proto.Properties, targets)
}

func objectPropertiesHaveAnyFlag(properties map[string]string, targets map[string]struct{}) bool {
	for key, value := range properties {
		if _, ok := targets[normalizeFlagName(key)]; ok && propertyFlagEnabled(value) {
			return true
		}
		if objectFlagContainerProperty(key) && propertyFlagValueHasAnyToken(value, targets) {
			return true
		}
	}
	return false
}

func objectFlagContainerProperty(key string) bool {
	switch normalizeFlagName(key) {
	case "flag", "flags":
		return true
	default:
		return false
	}
}

func propertyFlagValueHasAnyToken(value string, targets map[string]struct{}) bool {
	for _, token := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == ' '
	}) {
		if _, ok := targets[normalizeFlagName(token)]; ok {
			return true
		}
	}
	return false
}

func nextLegacyRoomID(id model.RoomID) (model.RoomID, bool) {
	raw := strings.TrimSpace(string(id))
	prefix := "room:"
	if !strings.HasPrefix(raw, prefix) {
		return "", false
	}
	digits := raw[len(prefix):]
	if digits == "" {
		return "", false
	}
	number, err := strconv.Atoi(digits)
	if err != nil {
		return "", false
	}
	return model.RoomID(prefix + fmt.Sprintf("%0*d", len(digits), number+1)), true
}
