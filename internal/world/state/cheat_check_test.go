package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"muhan/internal/world/model"
)

func TestCheckCreatureItems(t *testing.T) {
	// Clean up any existing logs
	logPath := filepath.Join("log", "dm.log")
	_ = os.Remove(logPath)
	defer os.Remove(logPath)

	w := NewWorld(nil)

	// Create a test player/creature
	cID := model.CreatureID("creature:test_ply")
	c := model.Creature{
		ID:          cID,
		DisplayName: "테스터",
		Inventory:   model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{}},
		Equipment:   make(map[string]model.ObjectInstanceID),
	}

	// 1. Normal Item
	normalObjID := model.ObjectInstanceID("object:normal")
	normalObj := model.ObjectInstance{
		ID:                  normalObjID,
		DisplayNameOverride: "정상검",
		Location:            model.ObjectLocation{CreatureID: cID},
		Properties: map[string]string{
			"armor": "10",
			"nDice": "2",
			"sDice": "6",
			"pDice": "1",
		},
	}
	c.Inventory.ObjectIDs = append(c.Inventory.ObjectIDs, normalObjID)
	w.objects[normalObjID] = normalObj

	// 2. Bad Armor (armor > 50)
	badArmorObjID := model.ObjectInstanceID("object:bad_armor")
	badArmorObj := model.ObjectInstance{
		ID:                  badArmorObjID,
		DisplayNameOverride: "나쁜갑옷",
		Location:            model.ObjectLocation{CreatureID: cID},
		Properties: map[string]string{
			"armor": "51",
		},
	}
	c.Inventory.ObjectIDs = append(c.Inventory.ObjectIDs, badArmorObjID)
	w.objects[badArmorObjID] = badArmorObj

	// 3. Bad Attack (ndice*sdice+pdice > 100)
	badAtkObjID := model.ObjectInstanceID("object:bad_atk")
	badAtkObj := model.ObjectInstance{
		ID:                  badAtkObjID,
		DisplayNameOverride: "나쁜칼",
		Location:            model.ObjectLocation{CreatureID: cID},
		Properties: map[string]string{
			"nDice": "10",
			"sDice": "10",
			"pDice": "5", // 10*10 + 5 = 105 > 100
		},
	}
	c.Inventory.ObjectIDs = append(c.Inventory.ObjectIDs, badAtkObjID)
	w.objects[badAtkObjID] = badAtkObj

	// 4. Bad Container (container && shotsmax > 20)
	badBagObjID := model.ObjectInstanceID("object:bad_bag")
	badBagObj := model.ObjectInstance{
		ID:                  badBagObjID,
		DisplayNameOverride: "나쁜가방",
		Location:            model.ObjectLocation{CreatureID: cID},
		Properties: map[string]string{
			"container": "true",
			"shotsmax":  "21",
		},
	}
	c.Inventory.ObjectIDs = append(c.Inventory.ObjectIDs, badBagObjID)
	w.objects[badBagObjID] = badBagObj

	// 5. Bad Special (OSPECI && pdice == 4)
	badSpecialObjID := model.ObjectInstanceID("object:bad_special")
	badSpecialObj := model.ObjectInstance{
		ID:                  badSpecialObjID,
		DisplayNameOverride: "나쁜기믹",
		Location:            model.ObjectLocation{CreatureID: cID},
		Properties: map[string]string{
			"OSPECI": "true",
			"pDice":  "4",
		},
	}
	c.Inventory.ObjectIDs = append(c.Inventory.ObjectIDs, badSpecialObjID)
	w.objects[badSpecialObjID] = badSpecialObj

	// 6. Bad Potion (potion && shotscur > 50)
	badPotionObjID := model.ObjectInstanceID("object:bad_potion")
	badPotionObj := model.ObjectInstance{
		ID:                  badPotionObjID,
		DisplayNameOverride: "나쁜포션",
		Location:            model.ObjectLocation{CreatureID: cID},
		Properties: map[string]string{
			"type":         "6", // POTION type
			"shotsCurrent": "51",
		},
	}
	c.Inventory.ObjectIDs = append(c.Inventory.ObjectIDs, badPotionObjID)
	w.objects[badPotionObjID] = badPotionObj

	// 7. Bad ShotsCur limit (shotscur >= 1000)
	badShotsCurObjID := model.ObjectInstanceID("object:bad_shotscur")
	badShotsCurObj := model.ObjectInstance{
		ID:                  badShotsCurObjID,
		DisplayNameOverride: "과다사용아이템",
		Location:            model.ObjectLocation{CreatureID: cID},
		Properties: map[string]string{
			"shotscur": "1000",
		},
	}
	c.Inventory.ObjectIDs = append(c.Inventory.ObjectIDs, badShotsCurObjID)
	w.objects[badShotsCurObjID] = badShotsCurObj

	// 8. Bad ShotsMax limit (shotsmax >= 1000)
	badShotsMaxObjID := model.ObjectInstanceID("object:bad_shotsmax")
	badShotsMaxObj := model.ObjectInstance{
		ID:                  badShotsMaxObjID,
		DisplayNameOverride: "과다한도아이템",
		Location:            model.ObjectLocation{CreatureID: cID},
		Properties: map[string]string{
			"shotsMax": "1000",
		},
	}
	c.Inventory.ObjectIDs = append(c.Inventory.ObjectIDs, badShotsMaxObjID)
	w.objects[badShotsMaxObjID] = badShotsMaxObj

	// 9. Nested Bad Item inside a Container
	containerObjID := model.ObjectInstanceID("object:container")
	containerObj := model.ObjectInstance{
		ID:                  containerObjID,
		DisplayNameOverride: "정상가방",
		Location:            model.ObjectLocation{CreatureID: cID},
		Properties: map[string]string{
			"container": "true",
			"shotsmax":  "10", // Normal
		},
		Contents: model.ObjectRefList{
			ObjectIDs: []model.ObjectInstanceID{},
		},
	}
	nestedBadObjID := model.ObjectInstanceID("object:nested_bad")
	nestedBadObj := model.ObjectInstance{
		ID:                  nestedBadObjID,
		DisplayNameOverride: "안에든나쁜템",
		Location:            model.ObjectLocation{ContainerID: containerObjID},
		Properties: map[string]string{
			"armor": "60", // Bad
		},
	}
	containerObj.Contents.ObjectIDs = append(containerObj.Contents.ObjectIDs, nestedBadObjID)
	w.objects[nestedBadObjID] = nestedBadObj
	w.objects[containerObjID] = containerObj
	c.Inventory.ObjectIDs = append(c.Inventory.ObjectIDs, containerObjID)

	w.creatures[cID] = c

	// Execute Check
	w.CheckCreatureItems(cID)

	// Verification
	updatedCreature := w.creatures[cID]

	// 1. Inventory Check
	hasNormal := false
	hasContainer := false
	for _, id := range updatedCreature.Inventory.ObjectIDs {
		if id == normalObjID {
			hasNormal = true
		}
		if id == containerObjID {
			hasContainer = true
		}
		if id == badArmorObjID || id == badAtkObjID || id == badBagObjID || id == badSpecialObjID || id == badPotionObjID || id == badShotsCurObjID || id == badShotsMaxObjID {
			t.Errorf("Bad item %s was not destroyed from inventory", id)
		}
	}
	if !hasNormal {
		t.Errorf("Normal item was unexpectedly destroyed")
	}
	if !hasContainer {
		t.Errorf("Normal container was unexpectedly destroyed")
	}

	// 2. Container Contents Check
	updatedContainer, ok := w.objects[containerObjID]
	if !ok {
		t.Errorf("Container object itself was destroyed")
	} else {
		for _, id := range updatedContainer.Contents.ObjectIDs {
			if id == nestedBadObjID {
				t.Errorf("Nested bad item was not destroyed")
			}
		}
	}

	// 3. w.objects Map Check
	badIDs := []model.ObjectInstanceID{badArmorObjID, badAtkObjID, badBagObjID, badSpecialObjID, badPotionObjID, badShotsCurObjID, badShotsMaxObjID, nestedBadObjID}
	for _, id := range badIDs {
		if _, exists := w.objects[id]; exists {
			t.Errorf("Object %s still exists in world objects map", id)
		}
	}

	// 4. Log File Check
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(logBytes)

	expectedLogs := []string{
		"나쁜!! 테스터 : 나쁜갑옷 방어력 : 51",
		"나쁜!! 테스터 : 나쁜칼 공격력 : 105",
		"나쁜!! 테스터 : 나쁜가방 보따리 : 21",
		"나쁜!! 테스터 : 나쁜기믹 이상해 : 4",
		"나쁜!! 테스터 : 나쁜포션 사용회수 : 51",
		"나쁜!! 테스터 : 과다사용아이템 사용회수 : 1000",
		"나쁜!! 테스터 : 과다한도아이템 최대회수 : 1000",
		"나쁜!! 테스터 : 안에든나쁜템 방어력 : 60",
	}

	for _, expected := range expectedLogs {
		if !strings.Contains(logContent, expected) {
			t.Errorf("Log file missing message: %q, got log:\n%s", expected, logContent)
		}
	}
}

func TestCheatCheckObjectKindsUsePropertyFlagTokens(t *testing.T) {
	w := NewWorld(nil)
	flagged := model.ObjectInstance{
		ID:         "object:flagged",
		Properties: map[string]string{"flags": "OCONTN|OSPECI"},
	}
	if !w.isContainer(flagged) {
		t.Fatal("instance flags token OCONTN was not treated as container")
	}
	if !w.isSpecialItem(flagged) {
		t.Fatal("instance flags token OSPECI was not treated as special item")
	}

	prototypeID := model.PrototypeID("prototype:flagged")
	w.prototypes[prototypeID] = model.ObjectPrototype{
		ID:         prototypeID,
		Properties: map[string]string{"flags": "container specialItem"},
	}
	fromPrototype := model.ObjectInstance{ID: "object:from-prototype", PrototypeID: prototypeID}
	if !w.isContainer(fromPrototype) {
		t.Fatal("prototype flags token container was not treated as container")
	}
	if !w.isSpecialItem(fromPrototype) {
		t.Fatal("prototype flags token specialItem was not treated as special item")
	}
}
