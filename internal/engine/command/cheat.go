package command

import (
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

// CheatWorld는 치트 아이템 체크 및 삭제를 위한 인터페이스입니다.
type CheatWorld interface {
	InventoryWorld
	DestroyCreatureInventoryObject(model.ObjectInstanceID, model.CreatureID) (bool, error)
	MoveObject(model.ObjectInstanceID, model.ObjectLocation) error
}

// IsBadItem은 아이템이 치트/비정상 아이템인지 여부를 체크합니다.
func IsBadItem(world InventoryWorld, object model.ObjectInstance) bool {
	// 1. 방어력 > 50
	armor, ok := objectIntProperty(world, object, "armor")
	if !ok {
		armor, ok = objectIntProperty(world, object, "ac")
	}
	if ok && armor > 50 {
		return true
	}

	// 2. 공격력 = ndice * sdice + pdice > 100
	ndice := objectIntPropertyOrZero(world, object, "nDice")
	if ndice == 0 {
		ndice = objectIntPropertyOrZero(world, object, "ndice")
	}
	sdice := objectIntPropertyOrZero(world, object, "sDice")
	if sdice == 0 {
		sdice = objectIntPropertyOrZero(world, object, "sdice")
	}
	pdice := objectIntPropertyOrZero(world, object, "pDice")
	if pdice == 0 {
		pdice = objectIntPropertyOrZero(world, object, "pdice")
	}
	if ndice*sdice+pdice > 100 {
		return true
	}

	// 3. 컨테이너 && shotsmax > 20
	isContainer := objectHasAnyTag(world, object, "container", "ocontn", "OCONTN") ||
		objectHasAnyPropertyFlag(world, object, "container", "ocontn", "OCONTN")
	shotsmax := objectIntPropertyOrZero(world, object, "shotsMax")
	if shotsmax == 0 {
		shotsmax = objectIntPropertyOrZero(world, object, "shotsmax")
	}
	if isContainer && shotsmax > 20 {
		return true
	}

	// 4. OSPECI && pdice == 4
	isSpecial := objectHasAnyTag(world, object, "specialItem", "ospeci", "OSPECI") ||
		objectHasAnyPropertyFlag(world, object, "specialItem", "ospeci", "OSPECI")
	if isSpecial && pdice == 4 {
		return true
	}

	// 5. POTION && shotscur > 50
	isPotion := objectLegacyType(world, object) == 6 ||
		objectKindIs(world, object, model.ObjectKindPotion)

	shotscur := objectIntPropertyOrZero(world, object, "shotsCurrent")
	if shotscur == 0 {
		shotscur = objectIntPropertyOrZero(world, object, "shotscur")
	}
	if isPotion && shotscur > 50 {
		return true
	}

	// 6. shotscur >= 1000
	if shotscur >= 1000 {
		return true
	}

	// 7. shotsmax >= 1000
	if shotsmax >= 1000 {
		return true
	}

	return false
}

// CheckItem은 한 크리처(플레이어)의 장비 및 인벤토리를 순회하며 치트/비정상 아이템을 검사하고
// 나쁜 아이템이 발견되면 안전하게 해당 장비 슬롯 혹은 인벤토리에서 제거합니다.
func CheckItem(world CheatWorld, creatureID model.CreatureID) error {
	creature, ok := world.Creature(creatureID)
	if !ok {
		return nil
	}

	// 1. 장비 슬롯 검사
	if creature.Equipment != nil {
		for _, objectID := range creature.Equipment {
			if objectID.IsZero() {
				continue
			}
			object, ok := world.Object(objectID)
			if !ok {
				continue
			}
			if IsBadItem(world, object) {
				// 장비 해제하여 인벤토리로 보냄
				if err := world.MoveObject(objectID, model.ObjectLocation{CreatureID: creatureID, Slot: "inventory"}); err != nil {
					return err
				}
				// 인벤토리에서 삭제
				if _, err := world.DestroyCreatureInventoryObject(objectID, creatureID); err != nil {
					return err
				}
			}
		}
	}

	// 2. 인벤토리 검사
	{
		// 복사본을 만들어 순회 중 삭제 에러 방지
		objectIDs := append([]model.ObjectInstanceID(nil), creature.Inventory.ObjectIDs...)
		for _, objectID := range objectIDs {
			if objectID.IsZero() {
				continue
			}
			object, ok := world.Object(objectID)
			if !ok {
				continue
			}

			// 컨테이너이면 내부도 검사
			isContainer := objectHasAnyTag(world, object, "container", "ocontn", "OCONTN") ||
				objectHasAnyPropertyFlag(world, object, "container", "ocontn", "OCONTN")
			if isContainer && len(object.Contents.ObjectIDs) > 0 {
				if err := checkContain(world, object, creatureID); err != nil {
					return err
				}
			}

			// 아이템 자체 검사
			if IsBadItem(world, object) {
				if _, err := world.DestroyCreatureInventoryObject(objectID, creatureID); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// checkContain은 컨테이너 내부의 아이템들을 검사하여 나쁜 아이템을 삭제합니다.
func checkContain(world CheatWorld, container model.ObjectInstance, creatureID model.CreatureID) error {
	contents := append([]model.ObjectInstanceID(nil), container.Contents.ObjectIDs...)
	for _, childID := range contents {
		if childID.IsZero() {
			continue
		}
		child, ok := world.Object(childID)
		if !ok {
			continue
		}
		if IsBadItem(world, child) {
			if err := world.MoveObject(childID, model.ObjectLocation{CreatureID: creatureID, Slot: "inventory"}); err != nil {
				return err
			}
			if _, err := world.DestroyCreatureInventoryObject(childID, creatureID); err != nil {
				return err
			}
		}
	}
	return nil
}
