package game

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	enginecmd "github.com/0xc0de1ab/muhan/internal/engine/command"
	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/session"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

const (
	familyBankGoldUnit        = 10000
	familyBankMaxBalanceCheck = 1000000000
)

// Full C bank.c semantics (P0-4 advanced guild/family bank):
// - Root value is stored in 만냥 units; per tx <=1000만, with C's
//   currentValue+requestedGold 10억 guard preserved.
// - Item banks: per family tier (daily[DL_EXPND].max / familyID) + room.special (0=gold,1=weapon...6=misc), shotsmax=200 count limit.
// - Non-boss restrictions by room special for item types (weapon/armor/potion etc).
// - Broadcast on large (>=100만) deposits.
// - Weight check on take (player carry limit), container no-nest.
// - Boss only for main gold withdraw/take special=0.
// Tier limits: higher tier families use distinct bank files/objects but same caps in legacy (Go mirrors via family keyed banks).
// War items / quest flags interact via separate systems.

// FamilyBankWorld is the state surface needed by the minimal legacy family
// bank commands.
type FamilyBankWorld interface {
	PlayerLookup
	Room(model.RoomID) (model.Room, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
	Bank(model.BankID) (model.BankAccount, bool)
	DepositCreatureGoldToObjectValueScaled(model.CreatureID, model.ObjectInstanceID, int, int, int, int) (int, int, bool, bool, error)
	WithdrawObjectValueToCreatureGoldScaled(model.ObjectInstanceID, model.CreatureID, int, int) (int, int, bool, error)
	StoreCreatureInventoryObjectInContainer(model.ObjectInstanceID, model.CreatureID, model.ObjectInstanceID, int) (int, bool, bool, error)
	TakeContainerObjectToCreatureInventory(model.ObjectInstanceID, model.ObjectInstanceID, model.CreatureID) (int, bool, error)
}

type familyBankFamiliesLookup interface {
	Families() []model.Family
}

// NewFamilyBankInventoryHandler lists or stores objects in the current family's
// room-special family bank container.
func NewFamilyBankInventoryHandler(world FamilyBankWorld) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		viewer, room, err := enginecmd.CurrentRoom(world, enginecmd.LookViewerFromContext(ctx))
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		if !familyBankRoomHasAll(room, []string{"depot", "rdepot"}, []string{"family", "rfamil"}) {
			ctx.WriteString("패거리 창고에서만 가능합니다.")
			return enginecmd.StatusDefault, nil
		}
		_, creature, err := enginecmd.CurrentInventoryCreature(world, viewer.PlayerID)
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		familyID, ok := familyBankCreatureFamilyID(creature)
		if !ok {
			ctx.WriteString("당신은 패거리에 속해있지 않습니다.\n")
			return enginecmd.StatusDefault, nil
		}
		special := familyBankRoomSpecial(room)
		target := strings.TrimSpace(strings.Join(resolved.Args, " "))
		root, ok := familyBankRootObjectFor(world, creature, familyID, special)
		if !ok {
			if target == "" {
				if _, _, err := ensureFamilyBankRoot(world, creature, familyID, special); err != nil {
					return enginecmd.StatusDefault, err
				}
			} else {
				return familyBankStoreObjectWithEnsuredRoot(ctx, world, creature, familyID, target, familyBankOrdinal(resolved), special)
			}
			ctx.WriteString("기증된 물건이 없습니다.")
			return enginecmd.StatusDefault, nil
		}

		if target != "" {
			return familyBankStoreObject(ctx, world, creature, root, target, familyBankOrdinal(resolved), special, room.ID)
		}

		ctx.WriteString("패거리 창고의 보관품입니다.\n")
		names := familyBankObjectNames(
			world,
			root,
			familyBankCreatureDetectsMagic(creature),
			familyBankCreatureDetectsInvisible(creature),
		)
		if len(names) == 0 {
			ctx.WriteString("패거리에 기증된 물건이 없습니다.")
			return enginecmd.StatusDefault, nil
		}
		ctx.WriteString("보관품의 목록 : ")
		ctx.WriteString(strings.Join(names, ", "))
		ctx.WriteString(".\n")
		return enginecmd.StatusDefault, nil
	}
}

// NewFamilyBankOutputHandler takes objects from the current family's
// room-special family bank container into the creature inventory.
func NewFamilyBankOutputHandler(world FamilyBankWorld) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		viewer, room, err := enginecmd.CurrentRoom(world, enginecmd.LookViewerFromContext(ctx))
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		if !familyBankRoomHasAll(room, []string{"depot", "rdepot"}, []string{"family", "rfamil"}) {
			ctx.WriteString("패거리 창고에서만 가능합니다.")
			return enginecmd.StatusDefault, nil
		}
		_, creature, err := enginecmd.CurrentInventoryCreature(world, viewer.PlayerID)
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		if familyBankRoomSpecial(room) == 0 && !creatureHasNormalizedFlag(creature, "PFMBOS", "familyBoss", "familyBossFlag") {
			ctx.WriteString("패거리의 문주만이 가능합니다.")
			return enginecmd.StatusDefault, nil
		}
		familyID, ok := familyBankCreatureFamilyID(creature)
		if !ok {
			ctx.WriteString("당신은 패거리에 속해있지 않습니다.\n")
			return enginecmd.StatusDefault, nil
		}

		target := strings.TrimSpace(strings.Join(resolved.Args, " "))
		if target == "" {
			ctx.WriteString("패거리 창고에서 무엇을 꺼내시려고요?")
			return enginecmd.StatusDefault, nil
		}

		root, ok := familyBankRootObjectFor(world, creature, familyID, familyBankRoomSpecial(room))
		if !ok {
			ctx.WriteString("그 안에 그런것은 없어요.")
			return enginecmd.StatusDefault, nil
		}

		object, name, ok := familyBankSelectContainerObject(world, root, target, familyBankOrdinal(resolved), familyBankCreatureDetectsInvisible(creature))
		if !ok {
			ctx.WriteString("그 안에 그런것은 없어요.")
			return enginecmd.StatusDefault, nil
		}
		if familyBankTakeCapacityExceeded(world, creature, object) {
			ctx.WriteString("당신은 더이상 가질 수 없습니다.")
			return enginecmd.StatusDefault, nil
		}
		if _, taken, err := world.TakeContainerObjectToCreatureInventory(object.ID, root.ID, creature.ID); err != nil {
			return enginecmd.StatusDefault, fmt.Errorf("output family bank object %q: %w", object.ID, err)
		} else if !taken {
			ctx.WriteString("그 안에 그런것은 없어요.")
			return enginecmd.StatusDefault, nil
		}
		queueFamilyBankSave(world, creature.PlayerID, root, creature, familyBankRoomSpecial(room))
		_ = familyBankRoomBroadcast(ctx, room.ID, familyBankOutputRoomMessage(familyBankActorDisplayName(world, ctx, creature), name))
		ctx.WriteString("당신은 " + name + krtext.Particle(name, '3') + " 반출했습니다.")
		return enginecmd.StatusDefault, nil
	}
}

// NewFamilyDepositHandler donates gold into the current family's value-backed
// bank object.
func NewFamilyDepositHandler(world FamilyBankWorld) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		viewer, room, err := enginecmd.CurrentRoom(world, enginecmd.LookViewerFromContext(ctx))
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		if !familyBankRoomHasAll(room, []string{"bank", "rbank"}, []string{"family", "rfamil"}) {
			ctx.WriteString("패거리 금고에서만 가능합니다.")
			return enginecmd.StatusDefault, nil
		}
		if len(resolved.Args) != 1 {
			ctx.WriteString("얼마를 기부하시려고요?")
			return enginecmd.StatusDefault, nil
		}
		if !strings.HasSuffix(resolved.Args[0], "만냥") {
			ctx.WriteString("사용법 : 몇만냥 기부")
			return enginecmd.StatusDefault, nil
		}
		_, creature, err := enginecmd.CurrentInventoryCreature(world, viewer.PlayerID)
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		familyID, ok := familyBankCreatureFamilyID(creature)
		if !ok {
			ctx.WriteString("당신은 패거리에 속해있지 않습니다.\n")
			return enginecmd.StatusDefault, nil
		}

		numStr := strings.TrimSpace(strings.TrimSuffix(resolved.Args[0], "만냥"))
		if numStr == "" {
			ctx.WriteString("사용법 : 몇만냥 기부")
			return enginecmd.StatusDefault, nil
		}
		amountInUnits := parseFamilyBankLegacyAtol(numStr)
		if amountInUnits < 1 {
			ctx.WriteString("돈의 단위는 음수가 될수 없습니다.")
			return enginecmd.StatusDefault, nil
		}
		if amountInUnits > 1000 {
			ctx.WriteString("기부는 1000만냥 이하만 가능합니다.\n")
			return enginecmd.StatusDefault, nil
		}

		amountGold := amountInUnits * familyBankGoldUnit
		gold := creature.Stats["gold"]
		if gold < amountGold {
			ctx.WriteString("당신은 그만큼의 돈을 가지고 있지 않습니다.\n")
			return enginecmd.StatusDefault, nil
		}

		root, ok, err := ensureFamilyBankRoot(world, creature, familyID, 0)
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		if !ok {
			ctx.WriteString("패거리 금고가 없습니다.\n")
			return enginecmd.StatusDefault, nil
		}
		currentBalanceUnits := familyBankObjectIntProperty(world, root, "value")
		if currentBalanceUnits+amountGold > familyBankMaxBalanceCheck {
			ctx.WriteString("패거리 금고의 총액은 10억 이상 될수 없습니다. \n")
			return enginecmd.StatusDefault, nil
		}

		_, balanceUnits, ok, withinLimit, err := world.DepositCreatureGoldToObjectValueScaled(
			creature.ID,
			root.ID,
			amountGold,
			amountInUnits,
			amountGold,
			familyBankMaxBalanceCheck,
		)
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		if !ok {
			ctx.WriteString("당신은 그만큼의 돈을 가지고 있지 않습니다.\n")
			return enginecmd.StatusDefault, nil
		}
		if !withinLimit {
			ctx.WriteString("패거리 금고의 총액은 10억 이상 될수 없습니다. \n")
			return enginecmd.StatusDefault, nil
		}

		if amountInUnits >= 100 {
			playerName := playerDisplayName(world, ctx.ActorID)
			if playerName == "" {
				playerName = ctx.ActorID
			}
			msg := fmt.Sprintf("\n### %s님이 패거리에 %d만냥을 기부하였습니다.", playerName, amountInUnits)
			_ = broadcastMessage(ctx, msg)
		}

		ctx.WriteString(fmt.Sprintf("당신은 %d만냥을 기부했습니다.\n", amountInUnits))
		balStr := fmt.Sprintf("패거리 금고의 총액이 %d만냥이 되었습니다.", balanceUnits)
		ctx.WriteString(balStr)

		queueFamilyBankSave(world, viewer.PlayerID, root, creature, 0)
		return enginecmd.StatusDefault, nil
	}
}

// NewFamilyWithdrawHandler withdraws gold from the current family's
// value-backed bank object. The legacy command is restricted to the family boss.
func NewFamilyWithdrawHandler(world FamilyBankWorld) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		viewer, room, err := enginecmd.CurrentRoom(world, enginecmd.LookViewerFromContext(ctx))
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		if !familyBankRoomHasAll(room, []string{"bank", "rbank"}, []string{"family", "rfamil"}) {
			ctx.WriteString("패거리 금고에서만 가능합니다.")
			return enginecmd.StatusDefault, nil
		}
		_, creature, err := enginecmd.CurrentInventoryCreature(world, viewer.PlayerID)
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		if !creatureHasNormalizedFlag(creature, "PFMBOS", "familyBoss", "familyBossFlag") {
			ctx.WriteString("패거리의 문주만이 가능합니다.")
			return enginecmd.StatusDefault, nil
		}
		if len(resolved.Args) != 1 {
			ctx.WriteString("얼마를 인출하시려고요?")
			return enginecmd.StatusDefault, nil
		}
		if !strings.HasSuffix(resolved.Args[0], "만냥") {
			ctx.WriteString("사용법 : 몇만냥 인출")
			return enginecmd.StatusDefault, nil
		}
		familyID, ok := familyBankCreatureFamilyID(creature)
		if !ok {
			ctx.WriteString("당신은 패거리에 속해있지 않습니다.\n")
			return enginecmd.StatusDefault, nil
		}
		root, ok := familyBankRootObjectFor(world, creature, familyID, 0)
		if !ok {
			ctx.WriteString("패거리 금고에 그만큼의 돈이 없습니다.")
			return enginecmd.StatusDefault, nil
		}

		numStr := strings.TrimSpace(strings.TrimSuffix(resolved.Args[0], "만냥"))
		if numStr == "" {
			ctx.WriteString("사용법 : 몇만냥 인출")
			return enginecmd.StatusDefault, nil
		}
		amountInUnits := parseFamilyBankLegacyAtol(numStr)
		if amountInUnits < 1 {
			ctx.WriteString("돈의 단위는 음수가 될수 없습니다.")
			return enginecmd.StatusDefault, nil
		}

		balanceUnits := familyBankObjectIntProperty(world, root, "value")
		if balanceUnits < amountInUnits {
			ctx.WriteString("패거리 금고에 그만큼의 돈이 없습니다.")
			return enginecmd.StatusDefault, nil
		}

		_, newBalanceUnits, enough, err := world.WithdrawObjectValueToCreatureGoldScaled(
			root.ID,
			creature.ID,
			amountInUnits,
			amountInUnits*familyBankGoldUnit,
		)
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		if !enough {
			ctx.WriteString("패거리 금고에 그만큼의 돈이 없습니다.")
			return enginecmd.StatusDefault, nil
		}
		ctx.WriteString(fmt.Sprintf("당신은 %d만냥을 인출했습니다.\n", amountInUnits))
		balStr := fmt.Sprintf("패거리 금고가 %d만냥이 되었습니다.", newBalanceUnits)
		ctx.WriteString(balStr)

		queueFamilyBankSave(world, viewer.PlayerID, root, creature, 0)
		return enginecmd.StatusDefault, nil
	}
}

func familyBankStoreObject(
	ctx *enginecmd.Context,
	world FamilyBankWorld,
	creature model.Creature,
	root model.ObjectInstance,
	target string,
	ordinal int64,
	special int,
	roomID model.RoomID,
) (enginecmd.Status, error) {
	object, name, ok := familyBankSelectInventoryObject(world, creature, target, ordinal, familyBankCreatureDetectsInvisible(creature))
	if !ok {
		ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
		return enginecmd.StatusDefault, nil
	}
	if familyBankRejectStoreObject(ctx, world, creature, object, special) {
		return enginecmd.StatusDefault, nil
	}
	return familyBankStoreSelectedObject(ctx, world, creature, root, object, name, special, roomID)
}

func familyBankStoreObjectWithEnsuredRoot(
	ctx *enginecmd.Context,
	world FamilyBankWorld,
	creature model.Creature,
	familyID int,
	target string,
	ordinal int64,
	special int,
) (enginecmd.Status, error) {
	object, name, ok := familyBankSelectInventoryObject(world, creature, target, ordinal, familyBankCreatureDetectsInvisible(creature))
	if !ok {
		ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
		return enginecmd.StatusDefault, nil
	}
	if familyBankRejectStoreObject(ctx, world, creature, object, special) {
		return enginecmd.StatusDefault, nil
	}
	root, ok, err := ensureFamilyBankRoot(world, creature, familyID, special)
	if err != nil {
		return enginecmd.StatusDefault, err
	}
	if !ok {
		ctx.WriteString("기증된 물건이 없습니다.")
		return enginecmd.StatusDefault, nil
	}
	return familyBankStoreSelectedObject(ctx, world, creature, root, object, name, special, creature.RoomID)
}

func familyBankRejectStoreObject(
	ctx *enginecmd.Context,
	world FamilyBankWorld,
	creature model.Creature,
	object model.ObjectInstance,
	special int,
) bool {
	isBoss := creatureHasNormalizedFlag(creature, "PFMBOS", "familyBoss", "familyBossFlag")
	if !isBoss {
		kind := familyBankObjectKind(world, object)
		isWeapon := strings.EqualFold(string(kind), string(model.ObjectKindWeapon))
		isArmor := strings.EqualFold(string(kind), string(model.ObjectKindArmor))
		isPotion := strings.EqualFold(string(kind), string(model.ObjectKindPotion))
		isScroll := strings.EqualFold(string(kind), string(model.ObjectKindScroll))
		isWand := strings.EqualFold(string(kind), string(model.ObjectKindWand))
		isKey := strings.EqualFold(string(kind), string(model.ObjectKindKey))
		isLightSource := strings.EqualFold(string(kind), string(model.ObjectKindLightSource))
		isMisc := strings.EqualFold(string(kind), string(model.ObjectKindMisc))

		if isWeapon && special != 1 {
			ctx.WriteString("\n무기류 창고에서만 가능합니다.")
			return true
		}
		if isArmor && special != 2 {
			ctx.WriteString("\n방어구류 창고에서만 가능합니다.")
			return true
		}
		if (isPotion || isScroll) && special != 3 {
			ctx.WriteString("\n주술구류 창고에서만 가능합니다.")
			return true
		}
		if isWand && special != 4 {
			ctx.WriteString("\n성구류 창고에서만 가능합니다.")
			return true
		}
		if isKey && special != 5 {
			ctx.WriteString("\n열쇠류 창고에서만 가능합니다.")
			return true
		}
		if (isLightSource || isMisc) && special != 6 {
			ctx.WriteString("\n기타류 창고에서만 가능합니다.")
			return true
		}
	}

	if familyBankObjectIsContainer(world, object) {
		ctx.WriteString("보따리 종류는 기증할 수 없습니다.\n")
		return true
	}
	return false
}

func familyBankStoreSelectedObject(
	ctx *enginecmd.Context,
	world FamilyBankWorld,
	creature model.Creature,
	root model.ObjectInstance,
	object model.ObjectInstance,
	name string,
	special int,
	roomID model.RoomID,
) (enginecmd.Status, error) {
	if _, stored, full, err := world.StoreCreatureInventoryObjectInContainer(object.ID, creature.ID, root.ID, familyBankRootMax(world, root)); err != nil {
		return enginecmd.StatusDefault, fmt.Errorf("store family bank object %q: %w", object.ID, err)
	} else if full {
		ctx.WriteString("패거리 창고 안에 더이상 넣을 수 없습니다.\n")
		return enginecmd.StatusDefault, nil
	} else if !stored {
		ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
		return enginecmd.StatusDefault, nil
	}
	queueFamilyBankSave(world, creature.PlayerID, root, creature, special)
	_ = familyBankRoomBroadcast(ctx, roomID, familyBankStoreRoomMessage(familyBankActorDisplayName(world, ctx, creature), name))
	ctx.WriteString("당신은 " + name + krtext.Particle(name, '3') + " 기증했습니다.\n")
	return enginecmd.StatusDefault, nil
}

func familyBankTakeCapacityExceeded(world FamilyBankWorld, creature model.Creature, object model.ObjectInstance) bool {
	return familyBankCreatureHeldCount(world, creature) > giveInventoryLimit ||
		familyBankCreatureCarriedWeight(world, creature)+familyBankObjectTotalWeight(world, object) > familyBankCreatureMaxWeight(creature)
}

func familyBankCreatureHeldCount(world FamilyBankWorld, creature model.Creature) int {
	count := 0
	for _, objectID := range creature.Equipment {
		if !objectID.IsZero() {
			count++
		}
	}
	inventoryCount := 0
	for _, objectID := range creature.Inventory.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		inventoryCount++
		if object, ok := world.Object(objectID); ok && familyBankObjectIsContainer(world, object) {
			inventoryCount += familyBankObjectContainerCount(world, object)
		}
	}
	if inventoryCount > 200 {
		inventoryCount = 200
	}
	return count + inventoryCount
}

func familyBankObjectContainerCount(world FamilyBankWorld, object model.ObjectInstance) int {
	for _, key := range []string{"shotsCurrent", "shotscur", "shotsCur", "contentsCount"} {
		if count := familyBankObjectIntProperty(world, object, key); count > 0 {
			return count
		}
	}
	return len(object.Contents.ObjectIDs)
}

func familyBankCreatureCarriedWeight(world FamilyBankWorld, creature model.Creature) int {
	seen := map[model.ObjectInstanceID]struct{}{}
	weight := 0
	for _, objectID := range creature.Inventory.ObjectIDs {
		weight += familyBankCarriedObjectWeight(world, objectID, true, seen)
	}
	for _, objectID := range creature.Equipment {
		weight += familyBankCarriedObjectWeight(world, objectID, false, seen)
	}
	return weight
}

func familyBankCarriedObjectWeight(world FamilyBankWorld, objectID model.ObjectInstanceID, skipWeightless bool, seen map[model.ObjectInstanceID]struct{}) int {
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
	if skipWeightless && familyBankObjectHasFlag(world, object, "weightless", "owtles") {
		return 0
	}
	weight := familyBankObjectOwnWeight(world, object)
	for _, childID := range object.Contents.ObjectIDs {
		weight += familyBankCarriedObjectWeight(world, childID, true, seen)
	}
	return weight
}

func familyBankObjectTotalWeight(world FamilyBankWorld, object model.ObjectInstance) int {
	seen := map[model.ObjectInstanceID]struct{}{}
	weight := familyBankObjectOwnWeight(world, object)
	for _, childID := range object.Contents.ObjectIDs {
		weight += familyBankCarriedObjectWeight(world, childID, true, seen)
	}
	return weight
}

func familyBankObjectOwnWeight(world FamilyBankWorld, object model.ObjectInstance) int {
	return familyBankObjectIntProperty(world, object, "weight")
}

func familyBankCreatureMaxWeight(creature model.Creature) int {
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

func familyBankObjectHasFlag(world FamilyBankWorld, object model.ObjectInstance, names ...string) bool {
	targets := familyBankNormalizedSet(names...)
	if familyBankHasAnyNormalized(object.Metadata.Tags, names...) {
		return true
	}
	for key, value := range object.Properties {
		if _, ok := targets[familyBankNormalizeName(key)]; ok && familyBankPropertyFlagEnabled(value) {
			return true
		}
		if objectFlagContainerProperty(key) && propertyFlagValueHasAnyToken(value, targets) {
			return true
		}
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := world.ObjectPrototype(object.PrototypeID)
	if !ok {
		return false
	}
	if familyBankHasAnyNormalized(proto.Metadata.Tags, names...) {
		return true
	}
	for key, value := range proto.Properties {
		if _, ok := targets[familyBankNormalizeName(key)]; ok && familyBankPropertyFlagEnabled(value) {
			return true
		}
		if objectFlagContainerProperty(key) && propertyFlagValueHasAnyToken(value, targets) {
			return true
		}
	}
	return false
}

func queueFamilyBankSave(world FamilyBankWorld, playerID model.PlayerID, root model.ObjectInstance, creature model.Creature, special int) {
	bankID := root.Location.BankID
	if bankID.IsZero() {
		if familyID, ok := familyBankCreatureFamilyID(creature); ok {
			for _, candidate := range familyBankIDCandidates(world, creature, familyID, special) {
				if _, exists := world.Bank(candidate); exists {
					bankID = candidate
					break
				}
			}
		}
	}
	if saver, ok := world.(interface {
		MarkPlayerDirty(model.PlayerID)
		MarkBankDirty(model.BankID)
		QueueSave(model.PlayerID, model.BankID)
	}); ok {
		if !playerID.IsZero() {
			saver.MarkPlayerDirty(playerID)
		}
		if !bankID.IsZero() {
			saver.MarkBankDirty(bankID)
		}
		saver.QueueSave(playerID, bankID)
		return
	}
	if saver, ok := world.(interface {
		SavePlayer(model.PlayerID) error
		SaveBank(model.BankID) error
	}); ok {
		if !playerID.IsZero() {
			if err := saver.SavePlayer(playerID); err != nil {
				log.Printf("[PERSIST] ERROR family bank SavePlayer %s: %v", playerID, err)
			}
		}
		if !bankID.IsZero() {
			if err := saver.SaveBank(bankID); err != nil {
				log.Printf("[PERSIST] ERROR family bank SaveBank %s: %v", bankID, err)
			}
		}
	}
}

func familyBankCreatureFamilyID(creature model.Creature) (int, bool) {
	if !creatureHasNormalizedFlag(creature, "familyFlag", "PFAMIL") {
		return 0, false
	}
	familyID, ok := creatureNormalizedInt(creature, "familyID", "dailyExpndMax", "legacyDailyExpndMax")
	return familyID, ok && familyID > 0
}

func familyBankRootObjectFor(world FamilyBankWorld, creature model.Creature, familyID int, special int) (model.ObjectInstance, bool) {
	for _, bankID := range familyBankIDCandidates(world, creature, familyID, special) {
		account, ok := world.Bank(bankID)
		if !ok {
			continue
		}
		for _, objectID := range account.Objects.ObjectIDs {
			object, found := world.Object(objectID)
			if found {
				return object, true
			}
		}
	}
	return model.ObjectInstance{}, false
}

type familyBankRootEnsurer interface {
	EnsureFamilyBankRoot(familyID int, special int) (model.BankAccount, model.ObjectInstance, error)
}

func ensureFamilyBankRoot(world FamilyBankWorld, creature model.Creature, familyID int, special int) (model.ObjectInstance, bool, error) {
	if root, ok := familyBankRootObjectFor(world, creature, familyID, special); ok {
		return root, true, nil
	}
	if ensurer, ok := world.(familyBankRootEnsurer); ok {
		_, root, err := ensurer.EnsureFamilyBankRoot(familyID, special)
		if err != nil {
			return model.ObjectInstance{}, false, err
		}
		return root, true, nil
	}
	return model.ObjectInstance{}, false, nil
}

func familyBankIDCandidates(world any, creature model.Creature, familyID int, special int) []model.BankID {
	names := []string{
		familyDisplayNameFromCreature(creature),
		familyDisplayNameFrom(world, familyID),
		strconv.Itoa(familyID),
	}
	if lookup, ok := world.(familyBankFamiliesLookup); ok {
		for _, family := range lookup.Families() {
			if family.ID == familyID || family.Slot == familyID {
				names = append(names, family.DisplayName)
			}
		}
	}
	seenNames := make(map[string]struct{}, len(names))
	seenIDs := make(map[model.BankID]struct{}, len(names)*2)
	ids := make([]model.BankID, 0, len(names)*2)
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seenNames[name]; ok {
			continue
		}
		seenNames[name] = struct{}{}
		candidates := []model.BankID{
			model.BankID(fmt.Sprintf("bank:family:%s_%d", name, special)),
		}
		if special == 0 {
			candidates = append(candidates, model.BankID("bank:family:"+name))
		}
		for _, id := range candidates {
			if _, ok := seenIDs[id]; ok {
				continue
			}
			seenIDs[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}

func familyBankRoomSpecial(room model.Room) int {
	value, ok := familyBankRoomProperty(room, "special")
	if !ok {
		return 0
	}
	special, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return special
}

func familyBankRoomHasAll(room model.Room, groups ...[]string) bool {
	for _, group := range groups {
		if !familyBankRoomHasAny(room, group...) {
			return false
		}
	}
	return true
}

func familyBankRoomHasAny(room model.Room, names ...string) bool {
	targets := familyBankNormalizedSet(names...)
	for _, tag := range room.Metadata.Tags {
		if _, ok := targets[familyBankNormalizeName(tag)]; ok {
			return true
		}
	}
	for key, value := range room.Properties {
		if _, ok := targets[familyBankNormalizeName(key)]; ok && familyBankPropertyFlagEnabled(value) {
			return true
		}
		for _, token := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ';' || r == '|' || r == ' '
		}) {
			if _, ok := targets[familyBankNormalizeName(token)]; ok {
				return true
			}
		}
	}
	return false
}

func familyBankRoomProperty(room model.Room, key string) (string, bool) {
	target := familyBankNormalizeName(key)
	for propertyKey, value := range room.Properties {
		if familyBankNormalizeName(propertyKey) == target {
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

func familyBankPropertyFlagEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}

type familyBankObjectGroup struct {
	Text       string
	BaseName   string
	Adjustment int
	Count      int
}

func familyBankObjectNames(world FamilyBankWorld, root model.ObjectInstance, detectMagic bool, detectInvisible bool) []string {
	groups := make([]familyBankObjectGroup, 0, len(root.Contents.ObjectIDs))
	for _, objectID := range root.Contents.ObjectIDs {
		object, ok := world.Object(objectID)
		if !ok || object.Location.ContainerID != root.ID || !familyBankObjectVisibleForListObj(world, object, detectInvisible) {
			continue
		}
		baseName := familyBankObjectDisplayName(world, object)
		if baseName == "" {
			continue
		}
		name := familyBankObjectMagicDisplayName(world, object, detectMagic)
		if name == "" {
			continue
		}
		adjustment := familyBankObjectIntPropertyAny(world, object, "adjustment", "adjust")
		last := len(groups) - 1
		if last >= 0 && groups[last].BaseName == baseName &&
			(!detectMagic || groups[last].Adjustment == adjustment) {
			groups[last].Count++
			groups[last].Text = name
			groups[last].Adjustment = adjustment
			continue
		}
		groups = append(groups, familyBankObjectGroup{
			Text:       name,
			BaseName:   baseName,
			Adjustment: adjustment,
			Count:      1,
		})
	}

	names := make([]string, 0, len(groups))
	for _, group := range groups {
		if group.Count > 1 {
			names = append(names, fmt.Sprintf("(x%d) %s", group.Count, group.Text))
			continue
		}
		names = append(names, group.Text)
	}
	return names
}

func familyBankSelectInventoryObject(world FamilyBankWorld, creature model.Creature, target string, ordinal int64, detectInvisible bool) (model.ObjectInstance, string, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return model.ObjectInstance{}, "", false
	}
	if ordinal < 1 {
		ordinal = 1
	}

	var seen int64
	for _, objectID := range creature.Inventory.ObjectIDs {
		object, ok := world.Object(objectID)
		if !ok || object.Location.CreatureID != creature.ID || (object.Location.Slot != "" && object.Location.Slot != "inventory") {
			continue
		}
		if !familyBankObjectVisibleForFindObj(world, object, detectInvisible) {
			continue
		}
		if !enginecmd.LegacyObjectPrefixMatches(world, object, target) {
			continue
		}
		seen++
		if seen == ordinal {
			return object, familyBankObjectDisplayName(world, object), true
		}
	}
	return model.ObjectInstance{}, "", false
}

func familyBankSelectContainerObject(world FamilyBankWorld, root model.ObjectInstance, target string, ordinal int64, detectInvisible bool) (model.ObjectInstance, string, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return model.ObjectInstance{}, "", false
	}
	if ordinal < 1 {
		ordinal = 1
	}

	var seen int64
	for _, objectID := range root.Contents.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || object.Location.ContainerID != root.ID || !familyBankObjectVisibleForFindObj(world, object, detectInvisible) {
			continue
		}
		if !enginecmd.LegacyObjectPrefixMatches(world, object, target) {
			continue
		}
		seen++
		if seen == ordinal {
			return object, familyBankObjectDisplayName(world, object), true
		}
	}
	return model.ObjectInstance{}, "", false
}

func familyBankObjectDisplayName(world FamilyBankWorld, object model.ObjectInstance) string {
	if name := familyBankCleanText(object.DisplayNameOverride); name != "" {
		return name
	}
	if name := familyBankCleanText(object.Properties["name"]); name != "" {
		return name
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if name := familyBankCleanText(proto.DisplayName); name != "" && !strings.HasPrefix(name, "object:") && !strings.HasPrefix(name, "objinst:") {
				return name
			}
			if name := familyBankCleanText(proto.Properties["name"]); name != "" {
				return name
			}
		}
	}
	return string(object.ID)
}

func familyBankCleanText(text string) string {
	return strings.TrimSpace(strings.ReplaceAll(text, "\x00", ""))
}

func familyBankObjectMagicDisplayName(world FamilyBankWorld, object model.ObjectInstance, detectMagic bool) string {
	name := familyBankObjectDisplayName(world, object)
	if !detectMagic || name == "" {
		return name
	}
	if adjustment := familyBankObjectIntPropertyAny(world, object, "adjustment", "adjust"); adjustment != 0 {
		return fmt.Sprintf("%s(%+d)", name, adjustment)
	}
	if familyBankObjectIntPropertyAny(world, object, "magicPower", "magicpower") != 0 {
		return name + "(주문)"
	}
	return name
}

func familyBankObjectVisibleForFindObj(world FamilyBankWorld, object model.ObjectInstance, detectInvisible bool) bool {
	return detectInvisible || !familyBankObjectHasFlag(world, object, "invisible", "oinvis", "OINVIS")
}

func familyBankObjectVisibleForListObj(world FamilyBankWorld, object model.ObjectInstance, detectInvisible bool) bool {
	if familyBankObjectHasFlag(world, object, "hidden", "ohiddn", "OHIDDN", "scenery", "scene", "oscene", "OSCENE") {
		return false
	}
	return detectInvisible || !familyBankObjectHasFlag(world, object, "invisible", "oinvis", "OINVIS")
}

func familyBankCreatureDetectsInvisible(creature model.Creature) bool {
	return creatureHasNormalizedFlag(creature, "PDINVI", "detectInvisible", "detectInvis")
}

func familyBankCreatureDetectsMagic(creature model.Creature) bool {
	return creatureHasNormalizedFlag(creature, "PDMAGI", "detectMagic", "dMagic")
}

func familyBankObjectIsContainer(world FamilyBankWorld, object model.ObjectInstance) bool {
	if strings.EqualFold(strings.TrimSpace(object.Properties["kind"]), string(model.ObjectKindContainer)) {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := world.ObjectPrototype(object.PrototypeID)
	return ok && (proto.Kind == model.ObjectKindContainer || strings.EqualFold(strings.TrimSpace(proto.Properties["kind"]), string(model.ObjectKindContainer)))
}

func familyBankRootMax(world FamilyBankWorld, root model.ObjectInstance) int {
	if maxCount := familyBankObjectIntProperty(world, root, "shotsMax"); maxCount > 0 {
		return maxCount
	}
	return 200
}

func familyBankObjectIntProperty(world FamilyBankWorld, object model.ObjectInstance, key string) int {
	if value, ok := familyBankParseInt(object.Properties[key]); ok {
		return value
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if value, ok := familyBankParseInt(proto.Properties[key]); ok {
				return value
			}
		}
	}
	return 0
}

func familyBankObjectIntPropertyAny(world FamilyBankWorld, object model.ObjectInstance, keys ...string) int {
	for _, key := range keys {
		if value := familyBankObjectIntProperty(world, object, key); value != 0 {
			return value
		}
	}
	return 0
}

func parseFamilyBankMoneyAmount(text string) (amount int, all bool, ok bool) {
	text = strings.TrimSpace(text)
	if text == "모두" {
		return 0, true, true
	}

	multiplier := 1
	switch {
	case strings.HasSuffix(text, "만냥"):
		multiplier = 10000
		text = strings.TrimSpace(strings.TrimSuffix(text, "만냥"))
	case strings.HasSuffix(text, "냥"):
		text = strings.TrimSpace(strings.TrimSuffix(text, "냥"))
	default:
		return 0, false, false
	}
	if text == "" {
		return 0, false, false
	}
	amount = parseFamilyBankLegacyAtol(text)
	return amount * multiplier, false, true
}

func parseFamilyBankLegacyAtol(text string) int {
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

func familyBankOrdinal(resolved enginecmd.ResolvedCommand) int64 {
	if len(resolved.Values) == 0 || resolved.Values[0] < 1 {
		return 1
	}
	return resolved.Values[0]
}

func familyBankHasAnyNormalized(values []string, names ...string) bool {
	targets := familyBankNormalizedSet(names...)
	for _, value := range values {
		if _, ok := targets[familyBankNormalizeName(value)]; ok {
			return true
		}
	}
	return false
}

func familyBankNormalizedSet(names ...string) map[string]struct{} {
	targets := make(map[string]struct{}, len(names))
	for _, name := range state.ExpandFlagNames(names...) {
		targets[name] = struct{}{}
	}
	return targets
}

func familyBankNormalizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, " ", "")
	return name
}

func familyBankParseInt(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	return parsed, err == nil
}

func familyBankActorDisplayName(world FamilyBankWorld, ctx *enginecmd.Context, creature model.Creature) string {
	if ctx != nil {
		if name := strings.TrimSpace(playerDisplayName(world, ctx.ActorID)); name != "" {
			return name
		}
		if name := strings.TrimSpace(ctx.ActorID); name != "" {
			return name
		}
	}
	if name := strings.TrimSpace(creature.DisplayName); name != "" {
		return name
	}
	if !creature.ID.IsZero() {
		return string(creature.ID)
	}
	return "누군가"
}

func familyBankStoreRoomMessage(actorName, objectName string) string {
	actorName = strings.TrimSpace(actorName)
	if actorName == "" {
		actorName = "누군가"
	}
	objectName = strings.TrimSpace(objectName)
	if objectName == "" {
		objectName = "물건"
	}
	return "\n" + actorName + "이 " + objectName + krtext.Particle(objectName, '3') + " 기증했습니다."
}

func familyBankOutputRoomMessage(actorName, objectName string) string {
	actorName = strings.TrimSpace(actorName)
	if actorName == "" {
		actorName = "누군가"
	}
	objectName = strings.TrimSpace(objectName)
	if objectName == "" {
		objectName = "물건"
	}
	return "\n패거리 창고에서 " + actorName + "이 " + objectName + krtext.Particle(objectName, '3') + " 꺼냈습니다."
}

func familyBankRoomBroadcast(ctx *enginecmd.Context, roomID model.RoomID, text string) error {
	if ctx == nil || ctx.Values == nil {
		return nil
	}
	broadcast, ok := ctx.Values[enginecmd.ContextRoomBroadcastKey].(enginecmd.RoomBroadcastFunc)
	if !ok || broadcast == nil {
		return nil
	}
	return broadcast(roomID, ctx.SessionID, text)
}

func broadcastMessage(ctx *enginecmd.Context, msg string) error {
	if ctx == nil || ctx.Values == nil {
		return nil
	}
	broadcast, ok := ctx.Values[ContextBroadcastKey].(func(session.Command) error)
	if !ok || broadcast == nil {
		return nil
	}
	return broadcast(session.Command{Write: msg})
}

func familyBankObjectKind(world FamilyBankWorld, object model.ObjectInstance) model.ObjectKind {
	if kindStr := strings.TrimSpace(object.Properties["kind"]); kindStr != "" {
		return model.ObjectKind(kindStr)
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if proto.Kind != "" {
				return proto.Kind
			}
			if kindStr := strings.TrimSpace(proto.Properties["kind"]); kindStr != "" {
				return model.ObjectKind(kindStr)
			}
		}
	}
	return ""
}
