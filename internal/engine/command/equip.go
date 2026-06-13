package command

import (
	"strconv"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	legacyWearBody   = 1
	legacyWearArms   = 2
	legacyWearLegs   = 3
	legacyWearNeck   = 4
	legacyWearHands  = 6
	legacyWearHead   = 7
	legacyWearFeet   = 8
	legacyWearFinger = 9
	legacyWearHeld   = 17
	legacyWearShield = 18
	legacyWearFace   = 19
	legacyWearWield  = 20
)

type equipAction int

const (
	equipActionWear equipAction = iota
	equipActionReady
	equipActionHold
)

const (
	legacyRaceDwarf     = 1
	legacyRaceElf       = 2
	legacyRaceHalfElf   = 3
	legacyRaceHobbit    = 4
	legacyRaceHuman     = 5
	legacyRaceOrc       = 6
	legacyRaceHalfGiant = 7
	legacyRaceGnome     = 8
)

type EquipWorld interface {
	InventoryWorld
	MoveObject(model.ObjectInstanceID, model.ObjectLocation) error
}

func NewWearHandler(world EquipWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		target := equipTarget(resolved)
		if target == "" {
			ctx.WriteString("뭘 입으실려구요?\n")
			return StatusDefault, nil
		}
		player, creature, err = clearEquipActorHidden(world, player, creature)
		if err != nil {
			return StatusDefault, err
		}
		if target == "모두" {
			return wearAll(ctx, world, creature)
		}
		object, name, ok := findEquipInventoryObjectWithVisibility(world, creature, target, firstGetOrdinal(resolved), inventoryViewerDetectsInvisible(player, creature))
		if !ok {
			ctx.WriteString("당신은 " + target + krtext.Particle(target, '3') + " 가지고 있지 않습니다.")
			return StatusDefault, nil
		}
		if message, drop := equipRestrictionMessage(world, creature, object, equipActionWear); message != "" {
			if drop {
				if err := world.MoveObject(object.ID, model.ObjectLocation{RoomID: player.RoomID}); err != nil {
					return StatusDefault, err
				}
			}
			ctx.WriteString(message)
			return StatusDefault, nil
		}
		slot, ok, reason := wearSlotForObject(world, creature, object)
		if !ok {
			ctx.WriteString(reason)
			return StatusDefault, nil
		}
		if err := world.MoveObject(object.ID, model.ObjectLocation{CreatureID: creature.ID, Slot: slot}); err != nil {
			return StatusDefault, err
		}
		if err := markEquipObjectWorn(world, object.ID); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(wearSuccessMessage(world, object, name))
		writeEquipUseOutput(ctx, world, object, true)
		return StatusDefault, nil
	}
}

func NewRemoveObjectHandler(world EquipWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		target := equipTarget(resolved)
		if target == "" {
			ctx.WriteString("뭘 벗고 싶으세요?")
			return StatusDefault, nil
		}
		player, creature, err = clearEquipActorHidden(world, player, creature)
		if err != nil {
			return StatusDefault, err
		}
		if target == "모두" {
			return removeAllEquipment(ctx, world, creature)
		}
		object, name, ok := findEquippedObjectWithVisibility(world, creature, target, firstGetOrdinal(resolved), inventoryViewerDetectsInvisible(player, creature))
		if !ok {
			ctx.WriteString("당신은 그것을 입고 있지 않습니다.")
			return StatusDefault, nil
		}
		if objectIsCursed(world, object) {
			ctx.WriteString("이쿠! 그것이 몸에서 떨어지지 않습니다! 저주받은 물건 같군요.")
			return StatusDefault, nil
		}
		if err := world.MoveObject(object.ID, model.ObjectLocation{CreatureID: creature.ID, Slot: "inventory"}); err != nil {
			return StatusDefault, err
		}
		if err := clearEquipObjectWorn(world, object.ID); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("당신은 " + name + krtext.Particle(name, '3') + " 벗었습니다.")
		return StatusDefault, nil
	}
}

func NewReadyHandler(world EquipWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		target := equipTarget(resolved)
		if target == "" {
			ctx.WriteString("무엇을 무장하시려구요?")
			return StatusDefault, nil
		}
		player, creature, err = clearEquipActorHidden(world, player, creature)
		if err != nil {
			return StatusDefault, err
		}
		object, name, ok := findEquipInventoryObjectWithVisibility(world, creature, target, firstGetOrdinal(resolved), inventoryViewerDetectsInvisible(player, creature))
		if !ok {
			ctx.WriteString("당신은 그런 물건을 가지고 있지 않습니다.")
			return StatusDefault, nil
		}
		if !objectCanWield(world, object) {
			ctx.WriteString("당신은 그것을 무장할 수 없습니다.")
			return StatusDefault, nil
		}
		if message, drop := equipRestrictionMessage(world, creature, object, equipActionReady); message != "" {
			if drop {
				if err := world.MoveObject(object.ID, model.ObjectLocation{RoomID: player.RoomID}); err != nil {
					return StatusDefault, err
				}
			}
			ctx.WriteString(message)
			return StatusDefault, nil
		}
		replacedName, err := moveEquippedSlotToInventory(world, creature, "wield")
		if err != nil {
			return StatusDefault, err
		}
		if err := world.MoveObject(object.ID, model.ObjectLocation{CreatureID: creature.ID, Slot: "wield"}); err != nil {
			return StatusDefault, err
		}
		if err := markEquipObjectWorn(world, object.ID); err != nil {
			return StatusDefault, err
		}
		if replacedName != "" {
			ctx.WriteString("당신은 " + replacedName + krtext.Particle(replacedName, '3') + " 벗고, " + name + krtext.Particle(name, '4') + " 전투태세를 취합니다.")
		} else {
			ctx.WriteString("당신은 " + name + krtext.Particle(name, '4') + " 전투태세를 취합니다.")
		}
		writeEquipUseOutput(ctx, world, object, true)
		return StatusDefault, nil
	}
}

func NewHoldHandler(world EquipWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		target := equipTarget(resolved)
		if target == "" {
			ctx.WriteString("무엇을 쥐실려구요?")
			return StatusDefault, nil
		}
		player, creature, err = clearEquipActorHidden(world, player, creature)
		if err != nil {
			return StatusDefault, err
		}
		object, name, ok := findEquipInventoryObjectWithVisibility(world, creature, target, firstGetOrdinal(resolved), inventoryViewerDetectsInvisible(player, creature))
		if !ok {
			ctx.WriteString("당신은 그런 것을 가지고 있지 않습니다.")
			return StatusDefault, nil
		}
		if !objectCanHold(world, object) {
			ctx.WriteString("당신은 그것을 쥘 수 없습니다.")
			return StatusDefault, nil
		}
		if message, drop := equipRestrictionMessage(world, creature, object, equipActionHold); message != "" {
			if drop {
				if err := world.MoveObject(object.ID, model.ObjectLocation{RoomID: player.RoomID}); err != nil {
					return StatusDefault, err
				}
			}
			ctx.WriteString(message)
			return StatusDefault, nil
		}
		if equippedObjectID(creature, "held") != "" {
			ctx.WriteString("당신은 이미 다른것을 쥐고 있습니다.")
			return StatusDefault, nil
		}
		if err := world.MoveObject(object.ID, model.ObjectLocation{CreatureID: creature.ID, Slot: "held"}); err != nil {
			return StatusDefault, err
		}
		if err := markEquipObjectHeld(world, object); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("당신은 " + name + krtext.Particle(name, '3') + " 쥐었습니다.")
		if objectLegacyTypeOrKind(world, object) != legacyObjectPotion {
			writeEquipUseOutput(ctx, world, object, true)
		}
		return StatusDefault, nil
	}
}

func equipTarget(resolved ResolvedCommand) string {
	return joinArgs(resolved.Args)
}

func clearEquipActorHidden(world EquipWorld, player model.Player, creature model.Creature) (model.Player, model.Creature, error) {
	if updater, ok := world.(interface {
		UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	}); ok {
		updated, err := updater.UpdateCreatureTags(creature.ID, nil, []string{"hidden", "phiddn", "PHIDDN"})
		if err != nil {
			return player, creature, err
		}
		creature = updated
	}
	if setter, ok := world.(interface {
		SetCreatureStat(model.CreatureID, string, int) error
	}); ok && creature.Stats != nil {
		if _, exists := creature.Stats["PHIDDN"]; exists {
			if err := setter.SetCreatureStat(creature.ID, "PHIDDN", 0); err != nil {
				return player, creature, err
			}
			creature.Stats["PHIDDN"] = 0
		}
	}
	if updater, ok := world.(interface {
		UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	}); ok {
		updated, err := updater.UpdatePlayerTags(player.ID, nil, []string{"hidden", "phiddn", "PHIDDN"})
		if err != nil {
			return player, creature, err
		}
		player = updated
	}
	return player, creature, nil
}

func wearAll(ctx *Context, world EquipWorld, creature model.Creature) (Status, error) {
	found := false
	for _, objectID := range creature.Inventory.ObjectIDs {
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInCreatureInventory(object, creature.ID) {
			continue
		}
		if message, _ := equipRestrictionMessage(world, creature, object, equipActionWear); message != "" {
			continue
		}
		slot, ok, _ := wearSlotForObject(world, creature, object)
		if !ok {
			continue
		}
		if err := world.MoveObject(object.ID, model.ObjectLocation{CreatureID: creature.ID, Slot: slot}); err != nil {
			return StatusDefault, err
		}
		if err := markEquipObjectWorn(world, object.ID); err != nil {
			return StatusDefault, err
		}
		if wearFlagForObject(world, object) != legacyWearNeck && wearFlagForObject(world, object) != legacyWearFinger {
			writeEquipUseOutput(ctx, world, object, false)
		}
		found = true
	}
	if !found {
		ctx.WriteString("당신은 입을 물건을 가지고 있지 않습니다.")
		return StatusDefault, nil
	}
	ctx.WriteString("당신은 입을 수 있는 장비를 모두 입었습니다.")
	return StatusDefault, nil
}

func removeAllEquipment(ctx *Context, world EquipWorld, creature model.Creature) (Status, error) {
	if len(creature.Equipment) == 0 {
		ctx.WriteString(legacyEquipmentEmptyMessage)
		return StatusDefault, nil
	}
	names := make([]string, 0, len(creature.Equipment))
	for _, slot := range orderedEquipmentSlots(creature.Equipment, legacyReadySlotOrder) {
		objectID := creature.Equipment[slot]
		object, ok := world.Object(objectID)
		if !ok || objectIsCursed(world, object) {
			continue
		}
		names = append(names, objectDisplayName(world, object))
		if err := world.MoveObject(object.ID, model.ObjectLocation{CreatureID: creature.ID, Slot: "inventory"}); err != nil {
			return StatusDefault, err
		}
		if err := clearEquipObjectWornAndHeld(world, object.ID); err != nil {
			return StatusDefault, err
		}
	}
	if len(names) == 0 {
		ctx.WriteString(legacyEquipmentEmptyMessage)
		return StatusDefault, nil
	}
	text := strings.Join(names, ", ")
	ctx.WriteString("당신은 " + text + krtext.Particle(text, '3') + " 벗었습니다.")
	return StatusDefault, nil
}

func moveEquippedSlotToInventory(world EquipWorld, creature model.Creature, slot string) (string, error) {
	objectID := equippedObjectID(creature, slot)
	if objectID.IsZero() {
		return "", nil
	}
	object, ok := world.Object(objectID)
	if !ok {
		return "", nil
	}
	name := objectDisplayName(world, object)
	if err := world.MoveObject(objectID, model.ObjectLocation{CreatureID: creature.ID, Slot: "inventory"}); err != nil {
		return "", err
	}
	return name, nil
}

type equipObjectTagUpdater interface {
	UpdateObjectTags(model.ObjectInstanceID, []string, []string) (model.ObjectInstance, error)
}

func markEquipObjectWorn(world EquipWorld, objectID model.ObjectInstanceID) error {
	updater, ok := world.(equipObjectTagUpdater)
	if !ok {
		return nil
	}
	_, err := updater.UpdateObjectTags(objectID, []string{"worn", "OWEARS"}, nil)
	return err
}

func markEquipObjectHeld(world EquipWorld, object model.ObjectInstance) error {
	updater, ok := world.(equipObjectTagUpdater)
	if !ok {
		return nil
	}
	add := []string{"worn", "OWEARS"}
	if objectLegacyTypeOrKind(world, object) < legacyObjectArmor {
		add = append(add, "held", "OWHELD")
	}
	_, err := updater.UpdateObjectTags(object.ID, add, nil)
	return err
}

func clearEquipObjectWorn(world EquipWorld, objectID model.ObjectInstanceID) error {
	updater, ok := world.(equipObjectTagUpdater)
	if !ok {
		return nil
	}
	_, err := updater.UpdateObjectTags(objectID, nil, []string{"worn", "OWEARS"})
	return err
}

func clearEquipObjectWornAndHeld(world EquipWorld, objectID model.ObjectInstanceID) error {
	updater, ok := world.(equipObjectTagUpdater)
	if !ok {
		return nil
	}
	_, err := updater.UpdateObjectTags(objectID, nil, []string{"worn", "OWEARS", "held", "OWHELD"})
	return err
}

func writeEquipUseOutput(ctx *Context, world InventoryWorld, object model.ObjectInstance, leadingBlankLine bool) {
	text := equipObjectUseOutput(world, object)
	if text == "" {
		return
	}
	if leadingBlankLine {
		ctx.WriteString("\n")
		ctx.WriteString(text)
		return
	}
	ctx.WriteString(ensureTrailingNewline(text))
}

func equipObjectUseOutput(world InventoryWorld, object model.ObjectInstance) string {
	for _, key := range []string{"useOutput", "use_output"} {
		if text, ok := objectStringProperty(world, object, key); ok {
			return text
		}
	}
	return ""
}

func equipRestrictionMessage(world InventoryWorld, creature model.Creature, object model.ObjectInstance, action equipAction) (string, bool) {
	name := objectDisplayName(world, object)
	legacyType := objectLegacyTypeOrKind(world, object)

	switch action {
	case equipActionWear:
		if legacyType == legacyObjectArmor {
			if equipObjectHasFlag(world, object, "noMage", "onomag", "ONOMAG") && equipClassIs(creature, model.ClassMage, model.ClassCleric) {
				return "도술사, 불제자들은 사용할수 없습니다.", false
			}
			if equipObjectHasFlag(world, object, "maleOnly", "noFemale", "onofem", "ONOFEM") && !equipCreatureMale(creature) {
				return name + krtext.Particle(name, '0') + " 남자들만 입을 수 있습니다.", false
			}
			if equipObjectHasFlag(world, object, "femaleOnly", "noMale", "onomal", "ONOMAL") && equipCreatureMale(creature) {
				return name + krtext.Particle(name, '0') + " 여자들만 입을 수 있습니다.", false
			}
			if equipObjectHasFlag(world, object, "marriageOnly", "marriage", "omarri", "OMARRI") && !equipCreatureMarried(creature) {
				return name + krtext.Particle(name, '0') + " 결혼한 사람들만 입을 수 있습니다.", false
			}
		}
		if shots, ok := objectFirstIntProperty(world, object, "shotsCurrent", "shotscur"); ok && shots < 1 {
			return name + krtext.Particle(name, '0') + " 부서져서 입을 수 없게 되었습니다.", false
		}
		if equipObjectSizeRestricted(world, creature, object) {
			return name + krtext.Particle(name, '1') + " 당신 몸에 맞지 않습니다.", false
		}
	case equipActionReady:
		if equipReadyMageClericRestricted(world, creature, object) {
			return "도술사, 불제자는 사용할수 없습니다.", false
		}
		if equipReadyGenderRestricted(world, creature, object) {
			if equipObjectHasFlag(world, object, "maleOnly", "noFemale", "onofem", "ONOFEM") {
				return name + krtext.Particle(name, '0') + " 남자들만 무장할 수 있습니다.", false
			}
			return name + krtext.Particle(name, '0') + " 여자들만 무장할 수 있습니다.", false
		}
		if equipObjectSizeRestricted(world, creature, object) {
			return name + krtext.Particle(name, '0') + " 당신의 몸 크기와 맞지 않습니다.", false
		}
	case equipActionHold:
		if equipObjectHasFlag(world, object, "event", "eventItem", "oevent", "OEVENT") ||
			objectIntPropertyOrDefault(world, object, "questNumber", "questnum", "questNum") > 0 ||
			equipObjectDamageScore(world, object) > 100 {
			return "당신은 그것을 쥘 수 없습니다.", false
		}
	}

	if equipObjectClassRestricted(world, creature, object) {
		return name + krtext.Particle(name, '0') + " 당신의 직업에 맞지 않습니다.", false
	}
	if equipAlignmentRejected(world, creature, object) {
		return equipAlignmentRejectionMessage(action, name), true
	}
	return "", false
}

func equipReadyMageClericRestricted(world InventoryWorld, creature model.Creature, object model.ObjectInstance) bool {
	legacyType := objectLegacyTypeOrKind(world, object)
	if legacyType != legacyObjectSharp && legacyType != legacyObjectThrust {
		return false
	}
	if objectIntPropertyOrDefault(world, object, "questNumber", "questnum", "questNum") != 0 ||
		equipObjectHasFlag(world, object, "event", "eventItem", "oevent", "OEVENT") {
		return false
	}
	return equipObjectDamageScore(world, object) > 14 && equipClassIs(creature, model.ClassMage, model.ClassCleric)
}

func equipReadyGenderRestricted(world InventoryWorld, creature model.Creature, object model.ObjectInstance) bool {
	legacyType := objectLegacyTypeOrKind(world, object)
	if legacyType != legacyObjectSharp && legacyType != legacyObjectThrust {
		return false
	}
	if equipObjectHasFlag(world, object, "maleOnly", "noFemale", "onofem", "ONOFEM") && !equipCreatureMale(creature) {
		return true
	}
	return equipObjectHasFlag(world, object, "femaleOnly", "noMale", "onomal", "ONOMAL") && equipCreatureMale(creature)
}

func equipObjectDamageScore(world InventoryWorld, object model.ObjectInstance) int {
	nDice := objectIntPropertyOrDefault(world, object, "nDice", "ndice")
	sDice := objectIntPropertyOrDefault(world, object, "sDice", "sdice")
	pDice := objectIntPropertyOrDefault(world, object, "pDice", "pdice")
	return nDice*sDice + pDice
}

func equipObjectClassRestricted(world InventoryWorld, creature model.Creature, object model.ObjectInstance) bool {
	if !equipObjectHasFlag(world, object, "classSelective", "clsSel", "oclsel", "OCLSEL") {
		return false
	}
	class := creatureClass(creature)
	if class >= model.ClassInvincible {
		return false
	}
	names := equipClassFlagNames(class)
	return len(names) == 0 || !equipObjectHasFlag(world, object, names...)
}

func equipClassFlagNames(class int) []string {
	switch class {
	case model.ClassAssassin:
		return []string{"classAssassin", "assassinUsable", "OASSNO", "oclsel1", "OCLSEL1"}
	case model.ClassBarbarian:
		return []string{"classBarbarian", "barbarianUsable", "OBARBO", "oclsel2", "OCLSEL2"}
	case model.ClassCleric:
		return []string{"classCleric", "clericUsable", "OCLERO", "oclsel3", "OCLSEL3"}
	case model.ClassFighter:
		return []string{"classFighter", "fighterUsable", "OFIGHO", "oclsel4", "OCLSEL4"}
	case model.ClassMage:
		return []string{"classMage", "mageUsable", "OMAGEO", "oclsel5", "OCLSEL5"}
	case model.ClassPaladin:
		return []string{"classPaladin", "paladinUsable", "OPALAO", "oclsel6", "OCLSEL6"}
	case model.ClassRanger:
		return []string{"classRanger", "rangerUsable", "ORNGRO", "oclsel7", "OCLSEL7"}
	case model.ClassThief:
		return []string{"classThief", "thiefUsable", "OTHIEO", "oclsel8", "OCLSEL8"}
	default:
		return nil
	}
}

func equipAlignmentRejected(world InventoryWorld, creature model.Creature, object model.ObjectInstance) bool {
	alignment := creatureStat(creature, "alignment")
	if alignment < -50 && equipObjectHasFlag(world, object, "goodOnly", "good", "ogoodo", "OGOODO") {
		return true
	}
	return alignment > 50 && equipObjectHasFlag(world, object, "evilOnly", "evil", "oevilo", "OEVILO")
}

func equipAlignmentRejectionMessage(action equipAction, name string) string {
	switch action {
	case equipActionHold:
		return name + krtext.Particle(name, '1') + " 당신의 손에서 튕겨져 나가 땅에 떨어집니다."
	case equipActionReady:
		return name + krtext.Particle(name, '1') + " 당신의 몸에서 튕겨져 나가 바닥에 떨어집니다."
	default:
		return name + krtext.Particle(name, '1') + " 당신 몸에서 튕겨져 나가 바닥에 떨어집니다."
	}
}

func equipObjectSizeRestricted(world InventoryWorld, creature model.Creature, object model.ObjectInstance) bool {
	size1 := equipObjectHasFlag(world, object, "size1", "osize1", "OSIZE1")
	size2 := equipObjectHasFlag(world, object, "size2", "osize2", "OSIZE2")
	if !size1 && !size2 {
		return false
	}
	race := creatureStat(creature, "race")
	switch {
	case !size1 && size2:
		return race != legacyRaceGnome && race != legacyRaceHobbit && race != legacyRaceDwarf
	case size1 && !size2:
		return race != legacyRaceHuman && race != legacyRaceElf && race != legacyRaceHalfElf && race != legacyRaceOrc
	case size1 && size2:
		return race != legacyRaceHalfGiant
	default:
		return false
	}
}

func equipObjectHasFlag(world InventoryWorld, object model.ObjectInstance, names ...string) bool {
	return objectHasAnyFlagOrProperty(world, object, names...)
}

func equipCreatureMale(creature model.Creature) bool {
	if creatureHasAnyFlag(creature, "PMALES", "male") {
		return true
	}
	if value := creatureStat(creature, "PMALES"); value != 0 {
		return true
	}
	if value := creatureStat(creature, "male"); value != 0 {
		return true
	}
	return false
}

func equipCreatureMarried(creature model.Creature) bool {
	return creatureHasAnyFlag(creature, "PMARRI", "married", "marriage", "marriageFlag") ||
		creatureStat(creature, "PMARRI") != 0 ||
		creatureStat(creature, "married") != 0 ||
		creatureStat(creature, "marriage") != 0
}

func equipClassIs(creature model.Creature, classes ...int) bool {
	class := creatureClass(creature)
	for _, candidate := range classes {
		if class == candidate {
			return true
		}
	}
	return false
}

func findEquipInventoryObject(
	world InventoryWorld,
	creature model.Creature,
	target string,
	ordinal int64,
) (model.ObjectInstance, string, bool) {
	return findEquipInventoryObjectWithVisibility(world, creature, target, ordinal, true)
}

func findEquipInventoryObjectWithVisibility(
	world InventoryWorld,
	creature model.Creature,
	target string,
	ordinal int64,
	detectInvisible bool,
) (model.ObjectInstance, string, bool) {
	if ordinal < 1 {
		ordinal = 1
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return model.ObjectInstance{}, "", false
	}
	var seen int64
	for _, objectID := range creature.Inventory.ObjectIDs {
		if objectID.IsZero() {
			continue
		}
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInCreatureInventory(object, creature.ID) {
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
			return object, objectDisplayName(world, object), true
		}
	}
	return model.ObjectInstance{}, "", false
}

func findEquippedObject(
	world InventoryWorld,
	creature model.Creature,
	target string,
	ordinal int64,
) (model.ObjectInstance, string, bool) {
	return findEquippedObjectWithVisibility(world, creature, target, ordinal, true)
}

func findEquippedObjectWithVisibility(
	world InventoryWorld,
	creature model.Creature,
	target string,
	ordinal int64,
	detectInvisible bool,
) (model.ObjectInstance, string, bool) {
	if ordinal < 1 {
		ordinal = 1
	}
	var seen int64
	for _, slot := range orderedEquipmentSlots(creature.Equipment, legacyReadySlotOrder) {
		objectID := creature.Equipment[slot]
		object, ok := world.Object(objectID)
		if !ok || !legacyObjectPrefixMatches(world, object, target) {
			continue
		}
		if !detectInvisible && dropObjectIsInvisible(world, object) {
			continue
		}
		seen++
		if seen == ordinal {
			return object, objectDisplayName(world, object), true
		}
	}
	return model.ObjectInstance{}, "", false
}

func wearSlotForObject(world InventoryWorld, creature model.Creature, object model.ObjectInstance) (string, bool, string) {
	flag := wearFlagForObject(world, object)
	if flag == legacyWearWield || flag == legacyWearHeld {
		return "", false, objectDisplayName(world, object) + krtext.Particle(objectDisplayName(world, object), '0') + " 입는 물건이 아닙니다."
	}
	slot, ok := slotForWearFlag(creature, flag)
	if !ok {
		switch flag {
		case legacyWearNeck:
			return "", false, "더이상 목에 걸 수 없습니다."
		case legacyWearFinger:
			return "", false, "더이상 손가락에 낄 수 없습니다."
		}
		return "", false, objectDisplayName(world, object) + krtext.Particle(objectDisplayName(world, object), '0') + " 입는 물건이 아닙니다."
	}
	if equippedObjectID(creature, slot) != "" {
		name := inventoryObjectName(world, equippedObjectID(creature, slot))
		return "", false, wearOccupiedMessage(slot, name)
	}
	return slot, true, ""
}

func wearFlagForObject(world InventoryWorld, object model.ObjectInstance) int {
	flag := objectWearFlag(world, object)
	if flag == 0 && objectKindIs(world, object, model.ObjectKindArmor) {
		return legacyWearBody
	}
	return flag
}

func wearSuccessMessage(world InventoryWorld, object model.ObjectInstance, name string) string {
	switch wearFlagForObject(world, object) {
	case legacyWearFeet:
		return "당신은 " + name + krtext.Particle(name, '3') + " 신었습니다."
	case legacyWearShield:
		return "당신은 " + name + krtext.Particle(name, '3') + " 방패로 삼습니다."
	case legacyWearArms:
		return "당신은 " + name + krtext.Particle(name, '3') + " 팔에 장착합니다."
	case legacyWearHead:
		return "당신은 " + name + krtext.Particle(name, '3') + " 머리에 씁니다."
	case legacyWearNeck:
		return "당신은 " + name + krtext.Particle(name, '3') + " 목에 두릅니다."
	case legacyWearHands:
		return "당신은 " + name + krtext.Particle(name, '3') + " 손에 끼웁니다."
	case legacyWearLegs:
		return "당신은 " + name + krtext.Particle(name, '3') + " 입습니다."
	case legacyWearFace:
		return "당신은 " + name + krtext.Particle(name, '3') + " 얼굴에 씁니다."
	case legacyWearFinger:
		return "당신은 " + name + krtext.Particle(name, '3') + " 손가락에 끼웁니다."
	default:
		return "당신은 " + name + krtext.Particle(name, '3') + " 입었습니다."
	}
}

func wearOccupiedMessage(slot string, name string) string {
	if slot == "head" {
		return "당신은 이미 " + name + krtext.Particle(name, '3') + " 머리에 쓰고 있습니다."
	}
	return "당신은 이미 " + name + krtext.Particle(name, '3') + " 입고 있습니다."
}

func slotForWearFlag(creature model.Creature, flag int) (string, bool) {
	switch flag {
	case legacyWearBody:
		return "body", true
	case legacyWearArms:
		return "arms", true
	case legacyWearLegs:
		return "legs", true
	case legacyWearHands:
		return "hands", true
	case legacyWearHead:
		return "head", true
	case legacyWearFeet:
		return "feet", true
	case legacyWearShield:
		return "shield", true
	case legacyWearFace:
		return "face", true
	case legacyWearNeck:
		return firstFreeSlot(creature, "neck1", "neck2")
	case legacyWearFinger:
		return firstFreeSlot(creature, "finger1", "finger2", "finger3", "finger4", "finger5", "finger6", "finger7", "finger8")
	default:
		return "", false
	}
}

func firstFreeSlot(creature model.Creature, slots ...string) (string, bool) {
	for _, slot := range slots {
		if equippedObjectID(creature, slot).IsZero() {
			return slot, true
		}
	}
	return "", false
}

func equippedObjectID(creature model.Creature, slot string) model.ObjectInstanceID {
	if creature.Equipment == nil {
		return ""
	}
	return creature.Equipment[slot]
}

func objectCanWield(world InventoryWorld, object model.ObjectInstance) bool {
	return objectWearFlag(world, object) == legacyWearWield ||
		objectLegacyType(world, object) >= 0 && objectLegacyType(world, object) <= 4 ||
		objectKindIs(world, object, model.ObjectKindWeapon)
}

func objectCanHold(world InventoryWorld, object model.ObjectInstance) bool {
	flag := objectWearFlag(world, object)
	return flag == legacyWearHeld || flag == legacyWearWield
}

func objectIsCursed(world InventoryWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object, "cursed", "ocurse") ||
		objectHasAnyPropertyFlag(world, object, "cursed", "ocurse", "OCURSE")
}

func objectWearFlag(world InventoryWorld, object model.ObjectInstance) int {
	if value, ok := objectIntProperty(world, object, "wearFlag"); ok {
		return value
	}
	return 0
}

func objectLegacyType(world InventoryWorld, object model.ObjectInstance) int {
	if value, ok := objectIntProperty(world, object, "type"); ok {
		return value
	}
	return -1
}

func objectIntProperty(world InventoryWorld, object model.ObjectInstance, key string) (int, bool) {
	if value, ok := parseObjectInt(object.Properties[key]); ok {
		return value, true
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if value, ok := parseObjectInt(proto.Properties[key]); ok {
				return value, true
			}
		}
	}
	return 0, false
}

func parseObjectInt(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	return parsed, err == nil
}

func objectKindIs(world InventoryWorld, object model.ObjectInstance, kind model.ObjectKind) bool {
	if strings.EqualFold(object.Properties["kind"], string(kind)) {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := world.ObjectPrototype(object.PrototypeID)
	if !ok {
		return false
	}
	return proto.Kind == kind || strings.EqualFold(proto.Properties["kind"], string(kind))
}

func objectHasAnyTag(world InventoryWorld, object model.ObjectInstance, names ...string) bool {
	if hasAnyNormalizedFlag(object.Metadata.Tags, names...) {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := world.ObjectPrototype(object.PrototypeID)
	if !ok {
		return false
	}
	return hasAnyNormalizedFlag(proto.Metadata.Tags, names...)
}
