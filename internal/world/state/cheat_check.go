package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

var dmLogMu sync.Mutex

// logDM writes DM warning messages to <root>/log/dm.log in a thread-safe manner.
func logDM(format string, args ...any) {
	dmLogMu.Lock()
	defer dmLogMu.Unlock()

	dir := "log"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	path := filepath.Join(dir, "dm.log")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	msg := fmt.Sprintf(format, args...)
	_, _ = f.WriteString(msg)
}

// CheckCreatureItems scans carried/equipped items of a creature and destroys any item
// having stats exceeding legacy limits:
// - armor > 50
// - attack power (ndice*sdice+pdice) > 100
// - bag slots (shotsmax) > 20 for containers
// - special potion pdice == 4 for OSPECI
// - potion shotscur > 50
// - shotscur >= 1000
// - shotsmax >= 1000
func (w *World) CheckCreatureItems(creatureID model.CreatureID) {
	if w == nil {
		return
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	creature, ok := w.creatures[creatureID]
	if !ok {
		return
	}

	parentName := creature.DisplayName
	if parentName == "" {
		parentName = "??????"
	}

	// Create copies of IDs to safely iterate and modify inside deleteObjectTreeLocked.
	equippedIDs := make([]model.ObjectInstanceID, 0, len(creature.Equipment))
	for _, objID := range creature.Equipment {
		equippedIDs = append(equippedIDs, objID)
	}

	inventoryIDs := append([]model.ObjectInstanceID(nil), creature.Inventory.ObjectIDs...)

	for _, objID := range equippedIDs {
		w.scanAndDestroyBadItemsLocked(parentName, objID)
	}

	for _, objID := range inventoryIDs {
		w.scanAndDestroyBadItemsLocked(parentName, objID)
	}
}

// scanAndDestroyBadItemsLocked recursively scans objects (checking container contents)
// and destroys any bad items.
func (w *World) scanAndDestroyBadItemsLocked(parentName string, objectID model.ObjectInstanceID) {
	if objectID.IsZero() {
		return
	}
	obj, ok := w.objects[objectID]
	if !ok {
		return
	}

	if w.isBadItemLocked(obj, parentName) {
		w.deleteObjectTreeLocked(objectID, map[model.ObjectInstanceID]struct{}{})
		return
	}

	if len(obj.Contents.ObjectIDs) > 0 {
		children := append([]model.ObjectInstanceID(nil), obj.Contents.ObjectIDs...)
		for _, childID := range children {
			w.scanAndDestroyBadItemsLocked(parentName, childID)
		}
	}
}

func (w *World) getObjectArmor(obj model.ObjectInstance) int {
	if val, ok := w.objectIntPropertyLocked(obj, "armor"); ok {
		return val
	}
	if val, ok := w.objectIntPropertyLocked(obj, "Armor"); ok {
		return val
	}
	return 0
}

func (w *World) getObjectNDice(obj model.ObjectInstance) int {
	if val, ok := w.objectIntPropertyLocked(obj, "nDice"); ok {
		return val
	}
	if val, ok := w.objectIntPropertyLocked(obj, "ndice"); ok {
		return val
	}
	return 0
}

func (w *World) getObjectSDice(obj model.ObjectInstance) int {
	if val, ok := w.objectIntPropertyLocked(obj, "sDice"); ok {
		return val
	}
	if val, ok := w.objectIntPropertyLocked(obj, "sdice"); ok {
		return val
	}
	return 0
}

func (w *World) getObjectPDice(obj model.ObjectInstance) int {
	if val, ok := w.objectIntPropertyLocked(obj, "pDice"); ok {
		return val
	}
	if val, ok := w.objectIntPropertyLocked(obj, "pdice"); ok {
		return val
	}
	return 0
}

func (w *World) getObjectShotsMax(obj model.ObjectInstance) int {
	if val, ok := w.objectIntPropertyLocked(obj, "shotsMax"); ok {
		return val
	}
	if val, ok := w.objectIntPropertyLocked(obj, "shotsmax"); ok {
		return val
	}
	return 0
}

func (w *World) getObjectShotsCur(obj model.ObjectInstance) int {
	if val, ok := w.objectIntPropertyLocked(obj, "shotsCurrent"); ok {
		return val
	}
	if val, ok := w.objectIntPropertyLocked(obj, "shotscur"); ok {
		return val
	}
	if val, ok := w.objectIntPropertyLocked(obj, "shotsCur"); ok {
		return val
	}
	return 0
}

func (w *World) isContainer(obj model.ObjectInstance) bool {
	if !obj.PrototypeID.IsZero() {
		if proto, ok := w.prototypes[obj.PrototypeID]; ok {
			if proto.Kind == model.ObjectKindContainer {
				return true
			}
			if hasAnyNormalizedFlag(proto.Metadata.Tags, "OCONTN", "container", "cont") ||
				objectHasAnyPropertyFlag(proto.Properties, "OCONTN", "container", "cont") {
				return true
			}
		}
	}
	return hasAnyNormalizedFlag(obj.Metadata.Tags, "OCONTN", "container", "cont") ||
		objectHasAnyPropertyFlag(obj.Properties, "OCONTN", "container", "cont")
}

func (w *World) isSpecialItem(obj model.ObjectInstance) bool {
	if !obj.PrototypeID.IsZero() {
		if proto, ok := w.prototypes[obj.PrototypeID]; ok {
			if hasAnyNormalizedFlag(proto.Metadata.Tags, "OSPECI", "specialItem", "special item") ||
				objectHasAnyPropertyFlag(proto.Properties, "OSPECI", "specialItem", "special item") {
				return true
			}
		}
	}
	return hasAnyNormalizedFlag(obj.Metadata.Tags, "OSPECI", "specialItem", "special item") ||
		objectHasAnyPropertyFlag(obj.Properties, "OSPECI", "specialItem", "special item")
}

func (w *World) isPotion(obj model.ObjectInstance) bool {
	if !obj.PrototypeID.IsZero() {
		if proto, ok := w.prototypes[obj.PrototypeID]; ok {
			if proto.Kind == model.ObjectKindPotion {
				return true
			}
			for _, tag := range proto.Metadata.Tags {
				if strings.ToLower(tag) == "potion" {
					return true
				}
			}
		}
	}
	for _, tag := range obj.Metadata.Tags {
		if strings.ToLower(tag) == "potion" {
			return true
		}
	}
	if t, ok := w.objectIntPropertyLocked(obj, "type"); ok && t == 6 {
		return true
	}
	return false
}

func (w *World) isBadItemLocked(obj model.ObjectInstance, parentName string) bool {
	objName := obj.DisplayNameOverride
	if objName == "" {
		if proto, ok := w.prototypes[obj.PrototypeID]; ok {
			objName = proto.DisplayName
		}
	}
	if objName == "" {
		objName = "?????"
	}

	// 1. armor > 50
	armor := w.getObjectArmor(obj)
	if armor > 50 {
		logDM("나쁜!! %s : %s 방어력 : %d\n", parentName, objName, armor)
		return true
	}

	// 2. attack power (ndice*sdice+pdice) > 100
	ndice := w.getObjectNDice(obj)
	sdice := w.getObjectSDice(obj)
	pdice := w.getObjectPDice(obj)
	haha := ndice*sdice + pdice
	if haha > 100 {
		logDM("나쁜!! %s : %s 공격력 : %d\n", parentName, objName, haha)
		return true
	}

	// 3. bag slots (shotsmax) > 20 for containers
	if w.isContainer(obj) {
		shotsmax := w.getObjectShotsMax(obj)
		if shotsmax > 20 {
			logDM("나쁜!! %s : %s 보따리 : %d\n", parentName, objName, shotsmax)
			return true
		}
	}

	// 4. special potion pdice == 4 for OSPECI
	if w.isSpecialItem(obj) {
		pdice := w.getObjectPDice(obj)
		if pdice == 4 {
			logDM("나쁜!! %s : %s 이상해 : %d\n", parentName, objName, pdice)
			return true
		}
	}

	// 5. potion shotscur > 50
	if w.isPotion(obj) {
		shotscur := w.getObjectShotsCur(obj)
		if shotscur > 50 {
			logDM("나쁜!! %s : %s 사용회수 : %d\n", parentName, objName, shotscur)
			return true
		}
	}

	// 6. shotscur >= 1000
	shotscur := w.getObjectShotsCur(obj)
	if shotscur >= 1000 {
		logDM("나쁜!! %s : %s 사용회수 : %d\n", parentName, objName, shotscur)
		return true
	}

	// 7. shotsmax >= 1000
	shotsmax := w.getObjectShotsMax(obj)
	if shotsmax >= 1000 {
		logDM("나쁜!! %s : %s 최대회수 : %d\n", parentName, objName, shotsmax)
		return true
	}

	return false
}
