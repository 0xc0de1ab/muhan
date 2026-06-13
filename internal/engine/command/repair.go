package command

import (
	"fmt"
	"maps"
	"math/rand"
	"strconv"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type RepairWorld interface {
	StatusWorld
	RepairCreatureInventoryObject(model.ObjectInstanceID, model.CreatureID, int, map[string]string, []string) (int, bool, bool, error)
	DestroyCreatureInventoryObject(model.ObjectInstanceID, model.CreatureID) (bool, error)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
}

type RepairRollFunc func(min int, max int) int

func NewRepairHandler(world RepairWorld, roll RepairRollFunc) Handler {
	if roll == nil {
		roll = func(min int, max int) int {
			if max <= min {
				return min
			}
			return min + rand.Intn(max-min+1)
		}
	}
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
			ctx.WriteString("무엇을 수리하시려구요?\n")
			return StatusDefault, nil
		}
		room, ok := world.Room(player.RoomID)
		if !ok {
			return StatusDefault, fmt.Errorf("repair: room %q not found", player.RoomID)
		}
		if !roomHasAnyFlag(room, "repair", "repairShop", "rrepai") {
			ctx.WriteString("여기서는 수리할 수 없습니다.\n")
			return StatusDefault, nil
		}
		objectID, name, ok := selectRepairInventoryObject(world, creature.ID, creature.Inventory.ObjectIDs, target, getOrdinal(resolved, 0), inventoryViewerDetectsInvisible(player, creature))
		if !ok {
			ctx.WriteString("당신은 그런 물건을 갖고 있지 않습니다.\n")
			return StatusDefault, nil
		}
		object, ok := world.Object(objectID)
		if !ok {
			ctx.WriteString("당신은 그런 물건을 갖고 있지 않습니다.\n")
			return StatusDefault, nil
		}
		if err := revealRepairActor(world, player, creature); err != nil {
			return StatusDefault, err
		}
		if repairObjectNoFix(world, object) {
			ctx.WriteString("그것은 수리할수 없는 물건입니다.\n")
			return StatusDefault, nil
		}
		legacyType := objectLegacyType(world, object)
		if !repairObjectTypeAllowed(world, object, legacyType) {
			ctx.WriteString("수리점 주인이 \"무기나 방호구만 수리할 수 있네.\"라고 말합니다.\n")
			return StatusDefault, nil
		}

		shotsCurrent := repairObjectShotsCurrent(world, object)
		shotsMax := repairObjectShotsMax(world, object)
		if shotsCurrent > maxInt(3, shotsMax/10) {
			ctx.WriteString("수리점 주인이 \"그건 아직 멀쩡한데...\"라고 말합니다.\n")
			return StatusDefault, nil
		}
		cost := shopObjectValue(world, object) / 4
		if creature.Stats["gold"] < cost {
			ctx.WriteString("수리점 주인이 \"공짜로는 수리해줄 수 없네. 돈을 더 벌어오게나..\"라고 말합니다.\n")
			return StatusDefault, nil
		}

		ctx.WriteString(fmt.Sprintf("당신은 수리점 주인에게 수리비 %d냥을 건네주었습니다.\n", cost))
		_ = roomBroadcast(ctx, room.ID, repairPaymentBroadcast(creature, name))

		piety := creatureStat(creature, "piety")
		breakRoll := roll(1, 100) + legacyStatBonus(piety)
		if (breakRoll <= 15 && shotsCurrent < 1) || (breakRoll <= 5 && shotsCurrent > 0) {
			destroyed, err := world.DestroyCreatureInventoryObject(objectID, creature.ID)
			if err != nil {
				return StatusDefault, fmt.Errorf("repair destroy object %q: %w", objectID, err)
			}
			if !destroyed {
				ctx.WriteString("당신은 그런 물건을 갖고 있지 않습니다.\n")
				return StatusDefault, nil
			}
			ctx.WriteString("수리점 주인이 \"이런~~! 수리를 하다 부러뜨렸네. 미안하네\"라고 말합니다.\n")
			_ = roomBroadcast(ctx, room.ID, "이런 주인이 실수를 했습니다.")
			ctx.WriteString("수리점주인이 당신에게 돈을 돌려주었습니다.\n")
			return StatusDefault, nil
		}

		properties := maps.Clone(object.Properties)
		if properties == nil {
			properties = map[string]string{}
		}
		removeTags := []string(nil)
		if repairObjectEnchanted(world, object) && roll(1, 50) > piety {
			ctx.WriteString("수리점 주인이 \"수리가 다되었네.\"라고 말합니다.\n")
			repairStripEnchantment(world, object, legacyType, properties)
			removeTags = []string{"enchanted", "oencha"}
		}
		shotsMax = repairPropertiesShotsMax(properties, shotsMax)
		properties["shotsCurrent"] = strconv.Itoa((shotsMax * roll(5, 9)) / 10)

		_, repaired, affordable, err := world.RepairCreatureInventoryObject(objectID, creature.ID, cost, properties, removeTags)
		if err != nil {
			return StatusDefault, fmt.Errorf("repair object %q: %w", objectID, err)
		}
		if !affordable {
			ctx.WriteString("수리점 주인이 \"공짜로는 수리해줄 수 없네. 돈을 더 벌어오게나..\"라고 말합니다.\n")
			return StatusDefault, nil
		}
		if !repaired {
			ctx.WriteString("당신은 그런 물건을 갖고 있지 않습니다.\n")
			return StatusDefault, nil
		}

		ctx.WriteString("수리점 주인이 당신에게 " + name + krtext.Particle(name, '3') + " 되돌려 줍니다.\n")
		ctx.WriteString("그것은 거의 새것처럼 보입니다.\n")
		return StatusDefault, nil
	}
}

func selectRepairInventoryObject(world RepairWorld, creatureID model.CreatureID, ids []model.ObjectInstanceID, target string, ordinal int64, detectInvisible bool) (model.ObjectInstanceID, string, bool) {
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

func revealRepairActor(world RepairWorld, player model.Player, creature model.Creature) error {
	if _, err := world.UpdateCreatureTags(creature.ID, nil, []string{"hidden", "phiddn", "PHIDDN"}); err != nil {
		return err
	}
	if !player.ID.IsZero() {
		if _, err := world.UpdatePlayerTags(player.ID, nil, []string{"hidden", "phiddn", "PHIDDN"}); err != nil {
			return err
		}
	}
	if creature.Stats != nil && creature.Stats["PHIDDN"] != 0 {
		return world.SetCreatureStat(creature.ID, "PHIDDN", 0)
	}
	return nil
}

func repairPaymentBroadcast(creature model.Creature, objectName string) string {
	actorName := attackCreatureName(creature)
	return "\n" + actorName + "이" +
		" 수리를 위해 수리점 주인에게 " + objectName + "를" +
		" 건네주었습니다."
}

func repairObjectNoFix(world InventoryWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object, "noRepair", "onofix") ||
		objectHasAnyPropertyFlag(world, object, "noRepair", "onofix", "ONOFIX")
}

func repairObjectTypeAllowed(world InventoryWorld, object model.ObjectInstance, legacyType int) bool {
	if legacyType >= 0 {
		return (legacyType >= legacyObjectSharp && legacyType <= legacyObjectMissile) ||
			legacyType == legacyObjectArmor
	}
	return objectKindIs(world, object, model.ObjectKindWeapon) ||
		objectKindIs(world, object, model.ObjectKindArmor)
}

func repairObjectEnchanted(world InventoryWorld, object model.ObjectInstance) bool {
	return objectIntPropertyOrZero(world, object, "adjustment") != 0 ||
		objectHasAnyTag(world, object, "enchanted", "oencha") ||
		objectHasAnyPropertyFlag(world, object, "enchanted", "oencha", "OENCHA")
}

func repairObjectShotsCurrent(world InventoryWorld, object model.ObjectInstance) int {
	return objectIntPropertyOrDefault(world, object, "shotsCurrent", "shotscur", "shotsCur", "charges")
}

func repairObjectShotsMax(world InventoryWorld, object model.ObjectInstance) int {
	return objectIntPropertyOrDefault(world, object, "shotsMax", "shotsmax", "shotsmaximum")
}

func repairObjectPDice(world InventoryWorld, object model.ObjectInstance) int {
	return objectIntPropertyOrDefault(world, object, "pDice", "pdice")
}

func repairObjectWearFlag(world InventoryWorld, object model.ObjectInstance) int {
	return objectIntPropertyOrDefault(world, object, "wearFlag", "wearflag")
}

func repairPropertiesShotsMax(properties map[string]string, fallback int) int {
	for _, key := range []string{"shotsMax", "shotsmax", "shotsmaximum"} {
		if value, ok := parseObjectInt(properties[key]); ok {
			return value
		}
	}
	return fallback
}

func repairStripEnchantment(world InventoryWorld, object model.ObjectInstance, legacyType int, properties map[string]string) {
	adjustment := objectIntPropertyOrZero(world, object, "adjustment")
	if adjustment == 0 {
		properties["adjustment"] = "0"
		properties["enchanted"] = "0"
		return
	}
	if legacyType == legacyObjectArmor || (legacyType < 0 && objectKindIs(world, object, model.ObjectKindArmor)) {
		armor := objectIntPropertyOrZero(world, object, "armor")
		decrement := adjustment
		if repairObjectWearFlag(world, object) == legacyWearBody {
			decrement *= 2
		}
		properties["armor"] = strconv.Itoa(maxInt(armor-decrement, 0))
	} else {
		shotsMax := repairObjectShotsMax(world, object)
		pDice := repairObjectPDice(world, object)
		properties["shotsMax"] = strconv.Itoa(maxInt(shotsMax-adjustment*10, 0))
		properties["pDice"] = strconv.Itoa(maxInt(pDice-adjustment, 0))
	}
	properties["adjustment"] = "0"
	properties["enchanted"] = "0"
}

func parseRepairInt(value string, fallback int) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

// maxInt is provided by advancement.go (centralized for levelup + other uses)
