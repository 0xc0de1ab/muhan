package state

import (
	"fmt"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"maps"
	"slices"
	"strings"
)

// Object returns a copy of the object instance with id.
func (w *World) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	if w == nil {
		return model.ObjectInstance{}, false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	object, ok := w.objects[id]
	if !ok {
		return model.ObjectInstance{}, false
	}
	return cloneObject(object), true
}

// GetObject returns a copy of the object instance with id.
func (w *World) GetObject(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	return w.Object(id)
}

// ObjectPrototype returns a copy of the object prototype with id.
func (w *World) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	if w == nil {
		return model.ObjectPrototype{}, false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	proto, ok := w.prototypes[id]
	if !ok {
		return model.ObjectPrototype{}, false
	}
	return cloneObjectPrototype(proto), true
}

// GetObjectPrototype returns a copy of the object prototype with id.
func (w *World) GetObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	return w.ObjectPrototype(id)
}

func (w *World) objectPrototypeLegacySortNameLocked(protoID model.PrototypeID) string {
	proto, ok := w.prototypes[protoID]
	if !ok {
		return string(protoID)
	}
	if name := strings.TrimSpace(proto.Properties["name"]); name != "" {
		return name
	}
	if name := strings.TrimSpace(proto.DisplayName); name != "" {
		return name
	}
	if name := firstStateObjectKeyName(proto.Properties); name != "" {
		return name
	}
	return string(proto.ID)
}

func (w *World) objectHasAnyLegacyFlagLocked(object model.ObjectInstance, names ...string) bool {
	if hasAnyNormalizedFlag(object.Metadata.Tags, names...) || objectHasAnyPropertyFlag(object.Properties, names...) {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := w.prototypes[object.PrototypeID]
	if !ok {
		return false
	}
	return hasAnyNormalizedFlag(proto.Metadata.Tags, names...) || objectHasAnyPropertyFlag(proto.Properties, names...)
}

func (w *World) nextObjectCloneIDLocked(sourceID model.ObjectInstanceID) model.ObjectInstanceID {
	base := strings.TrimSpace(string(sourceID))
	if base == "" {
		base = "object"
	}
	for i := 1; ; i++ {
		id := model.ObjectInstanceID(fmt.Sprintf("%s:clone:%06d", base, i))
		if _, exists := w.objects[id]; !exists {
			return id
		}
	}
}

// cloneObjectSourceToLocationLocked clones a live object or materializes a
// prototype into location. skipRandomEnchant suppresses the ORENCH roll for
// callers that must not re-roll (the shop buy / monster purchase paths: C buy
// copies the depot object as-is and C purchase load_obj's a template, neither
// rolling rand_enchant — command7.c:169-217, command10.c:492+).
func (w *World) cloneObjectSourceToLocationLocked(sourceID model.ObjectInstanceID, location model.ObjectLocation, skipRandomEnchant bool) (model.ObjectInstanceID, error) {
	if _, ok := w.objects[sourceID]; ok {
		return w.cloneObjectTreeToLocationLocked(sourceID, location, map[model.ObjectInstanceID]struct{}{}, skipRandomEnchant)
	}
	protoID, ok := w.prototypeIDFromCloneSourceLocked(sourceID)
	if !ok {
		return "", fmt.Errorf("source object or prototype not found")
	}
	return w.createObjectFromPrototypeLocked(protoID, location, skipRandomEnchant)
}

func (w *World) prototypeIDFromCloneSourceLocked(sourceID model.ObjectInstanceID) (model.PrototypeID, bool) {
	protoID := model.PrototypeID(sourceID)
	if _, ok := w.prototypes[protoID]; ok {
		return protoID, true
	}
	if number, ok := legacyCarryNumberFromCloneSource(sourceID); ok {
		protoID = legacyCarryObjectPrototypeID(number)
		if _, ok := w.prototypes[protoID]; ok {
			return protoID, true
		}
	}
	return "", false
}

func (w *World) createObjectFromPrototypeLocked(protoID model.PrototypeID, location model.ObjectLocation, skipRandomEnchant bool) (model.ObjectInstanceID, error) {
	proto, ok := w.prototypes[protoID]
	if !ok {
		return "", fmt.Errorf("prototype %q not found", protoID)
	}
	if templateID, ok := w.prototypeTemplateObjectIDLocked(proto); ok {
		return w.cloneObjectTreeToLocationLocked(templateID, location, map[model.ObjectInstanceID]struct{}{}, skipRandomEnchant)
	}

	objectID := w.nextObjectCloneIDLocked(model.ObjectInstanceID(protoID))
	object := model.ObjectInstance{
		ID:          objectID,
		PrototypeID: protoID,
		Quantity:    1,
		Location:    location,
		Properties:  maps.Clone(proto.Properties),
		Metadata: model.Metadata{
			Tags: slices.Clone(proto.Metadata.Tags),
		},
	}
	if !skipRandomEnchant {
		w.applyRandomEnchantIfNeededLocked(&object)
	}
	if err := object.Validate(); err != nil {
		return "", err
	}
	w.objects[object.ID] = object
	w.addObjectToHolderLocked(object.ID, object.Location)
	return object.ID, nil
}

func (w *World) prototypeTemplateObjectIDLocked(proto model.ObjectPrototype) (model.ObjectInstanceID, bool) {
	if resolution := proto.Metadata.PrototypeResolution; resolution != nil {
		if templateID := resolution.MaterializedFromObjectInstanceID; !templateID.IsZero() {
			if object, ok := w.objects[templateID]; ok && object.PrototypeID == proto.ID {
				return templateID, true
			}
		}
	}
	if object, ok := w.objects[model.ObjectInstanceID(proto.ID)]; ok && object.PrototypeID == proto.ID {
		return object.ID, true
	}
	return "", false
}

func (w *World) cloneObjectTreeToLocationLocked(sourceID model.ObjectInstanceID, location model.ObjectLocation, seen map[model.ObjectInstanceID]struct{}, skipRandomEnchant bool) (model.ObjectInstanceID, error) {
	if _, ok := seen[sourceID]; ok {
		return "", fmt.Errorf("object tree cycle at %q", sourceID)
	}
	seen[sourceID] = struct{}{}

	source, ok := w.objects[sourceID]
	if !ok {
		return "", fmt.Errorf("source object not found")
	}
	clone := cloneObject(source)
	clone.ID = w.nextObjectCloneIDLocked(sourceID)
	clone.Location = location
	clone.Contents = model.ObjectRefList{}
	if !skipRandomEnchant {
		w.applyRandomEnchantIfNeededLocked(&clone)
	}
	if err := clone.Validate(); err != nil {
		delete(seen, sourceID)
		return "", err
	}

	w.objects[clone.ID] = clone
	w.addObjectToHolderLocked(clone.ID, clone.Location)
	for _, childID := range source.Contents.ObjectIDs {
		if _, err := w.cloneObjectTreeToLocationLocked(childID, model.ObjectLocation{ContainerID: clone.ID}, seen, skipRandomEnchant); err != nil {
			w.deleteObjectTreeLocked(clone.ID, map[model.ObjectInstanceID]struct{}{})
			delete(seen, sourceID)
			return "", fmt.Errorf("clone child %q: %w", childID, err)
		}
	}
	delete(seen, sourceID)
	return clone.ID, nil
}

func (w *World) objectHasRandomEnchantLocked(object model.ObjectInstance) bool {
	if hasAnyNormalizedFlag(object.Metadata.Tags, "ORENCH", "randomEnchantment", "randEnch") ||
		objectHasAnyPropertyFlag(object.Properties, "ORENCH", "randomEnchantment", "randEnch") ||
		metadataHasLegacyObjectFlag(object.Metadata, legacyObjectRandomEnchantmentFlagBit) {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := w.prototypes[object.PrototypeID]
	if !ok {
		return false
	}
	return hasAnyNormalizedFlag(proto.Metadata.Tags, "ORENCH", "randomEnchantment", "randEnch") ||
		objectHasAnyPropertyFlag(proto.Properties, "ORENCH", "randomEnchantment", "randEnch") ||
		metadataHasLegacyObjectFlag(proto.Metadata, legacyObjectRandomEnchantmentFlagBit)
}

func metadataHasLegacyObjectFlag(metadata model.Metadata, bit int) bool {
	if bit < 0 {
		return false
	}
	flags := metadata.RawFields["flags"]
	byteIndex := bit / 8
	if byteIndex >= len(flags) {
		return false
	}
	return flags[byteIndex]&(1<<uint(bit%8)) != 0
}

func objectHasAnyPropertyFlag(properties map[string]string, names ...string) bool {
	if len(properties) == 0 {
		return false
	}
	targets := normalizedFlagSet(names...)
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

func (w *World) objectIntPropertyAnyLocked(object model.ObjectInstance, keys ...string) (int, bool) {
	for _, key := range keys {
		if value, ok := w.objectIntPropertyLocked(object, key); ok {
			return value, true
		}
	}
	return 0, false
}

// UpdateObjectInstance updates an object instance in the world state.
func (w *World) UpdateObjectInstance(object model.ObjectInstance) error {
	if w == nil {
		return fmt.Errorf("update object: world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	w.objects[object.ID] = object
	return nil
}

// UpdateObjectTags adds and removes object metadata tags under the world lock.
func (w *World) UpdateObjectTags(objectID model.ObjectInstanceID, add []string, remove []string) (model.ObjectInstance, error) {
	if w == nil {
		return model.ObjectInstance{}, fmt.Errorf("update object %q tags: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return model.ObjectInstance{}, fmt.Errorf("update object tags: object id is required")
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	object, ok := w.objects[objectID]
	if !ok {
		return model.ObjectInstance{}, fmt.Errorf("update object %q tags: object not found", objectID)
	}
	object.Metadata.Tags = addMetadataTags(removeMetadataTags(object.Metadata.Tags, remove), add)
	w.objects[objectID] = object
	return cloneObject(object), nil
}

// SetObjectDisplayName sets an instance-specific display name.
func (w *World) SetObjectDisplayName(objectID model.ObjectInstanceID, name string) (model.ObjectInstance, error) {
	if w == nil {
		return model.ObjectInstance{}, fmt.Errorf("set object %q display name: world state is nil", objectID)
	}
	if objectID.IsZero() {
		return model.ObjectInstance{}, fmt.Errorf("set object display name: object id is required")
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	object, ok := w.objects[objectID]
	if !ok {
		return model.ObjectInstance{}, fmt.Errorf("set object %q display name: object not found", objectID)
	}
	object.DisplayNameOverride = name
	w.objects[objectID] = object
	return cloneObject(object), nil
}

// SetObjectProperty sets a string object instance property. An empty value
// removes the property.
func (w *World) SetObjectProperty(objectID model.ObjectInstanceID, key string, value string) (model.ObjectInstance, error) {
	if w == nil {
		return model.ObjectInstance{}, fmt.Errorf("set object %q property %q: world state is nil", objectID, key)
	}
	if objectID.IsZero() {
		return model.ObjectInstance{}, fmt.Errorf("set object property %q: object id is required", key)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return model.ObjectInstance{}, fmt.Errorf("set object %q property: key is required", objectID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	object, ok := w.objects[objectID]
	if !ok {
		return model.ObjectInstance{}, fmt.Errorf("set object %q property %q: object not found", objectID, key)
	}
	if value == "" {
		delete(object.Properties, key)
	} else {
		if object.Properties == nil {
			object.Properties = map[string]string{}
		}
		object.Properties[key] = value
	}
	w.objects[objectID] = object
	return cloneObject(object), nil
}

func (w *World) objectKindIsLocked(object model.ObjectInstance, kind model.ObjectKind) bool {
	if strings.EqualFold(strings.TrimSpace(object.Properties["kind"]), string(kind)) {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := w.prototypes[object.PrototypeID]
	if !ok {
		return false
	}
	return proto.Kind == kind || strings.EqualFold(strings.TrimSpace(proto.Properties["kind"]), string(kind))
}

func (w *World) objectIntPropertyLocked(object model.ObjectInstance, key string) (int, bool) {
	if value, ok := parseStateInt(object.Properties[key]); ok {
		return value, true
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := w.prototypes[object.PrototypeID]; ok {
			if value, ok := parseStateInt(proto.Properties[key]); ok {
				return value, true
			}
		}
	}
	return 0, false
}

func (w *World) objectIsMoneyLocked(object model.ObjectInstance) bool {
	if strings.EqualFold(strings.TrimSpace(object.Properties["kind"]), string(model.ObjectKindMoney)) {
		return true
	}
	if value, ok := w.objectIntPropertyLocked(object, "type"); ok && value == 10 {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := w.prototypes[object.PrototypeID]
	if !ok {
		return false
	}
	return proto.Kind == model.ObjectKindMoney ||
		strings.EqualFold(strings.TrimSpace(proto.Properties["kind"]), string(model.ObjectKindMoney))
}

func objectLocationEqual(a model.ObjectLocation, b model.ObjectLocation) bool {
	return a.RoomID == b.RoomID &&
		a.CreatureID == b.CreatureID &&
		a.BankID == b.BankID &&
		a.ContainerID == b.ContainerID &&
		a.Slot == b.Slot
}

func objectCountPropertyOrLenLocked(w *World, object model.ObjectInstance, key string) int {
	if w != nil {
		if value, ok := w.objectIntPropertyLocked(object, key); ok && value >= 0 {
			return value
		}
	}
	return len(object.Contents.ObjectIDs)
}

func (w *World) objectWeaponProficiencyStatKeyLocked(objectID model.ObjectInstanceID) (string, bool) {
	if objectID.IsZero() {
		return "", false
	}
	object, ok := w.objects[objectID]
	if !ok {
		return "", false
	}
	weaponType, ok := w.objectIntPropertyLocked(object, "type")
	if !ok || weaponType < 0 || weaponType >= len(weaponProficiencyStatKeys) {
		return "", false
	}
	return weaponProficiencyStatKeys[weaponType], true
}

func (w *World) deleteObjectTreeLocked(objectID model.ObjectInstanceID, seen map[model.ObjectInstanceID]struct{}) {
	if objectID.IsZero() {
		return
	}
	if _, ok := seen[objectID]; ok {
		return
	}
	seen[objectID] = struct{}{}

	object, ok := w.objects[objectID]
	if !ok {
		return
	}
	w.markObjectLocationDirtyLocked(object.Location, map[model.ObjectInstanceID]struct{}{})
	for _, childID := range append([]model.ObjectInstanceID(nil), object.Contents.ObjectIDs...) {
		w.deleteObjectTreeLocked(childID, seen)
	}
	w.removeObjectFromHolderLocked(objectID, object.Location)
	delete(w.objects, objectID)
}

func (w *World) markObjectLocationDirtyLocked(location model.ObjectLocation, seen map[model.ObjectInstanceID]struct{}) {
	switch {
	case !location.RoomID.IsZero():
		w.MarkRoomObjectsDirty(location.RoomID)
	case !location.CreatureID.IsZero():
		if creature, ok := w.creatures[location.CreatureID]; ok && !creature.PlayerID.IsZero() {
			w.MarkPlayerDirty(creature.PlayerID)
		}
	case !location.BankID.IsZero():
		w.MarkBankDirty(location.BankID)
	case !location.ContainerID.IsZero():
		if _, ok := seen[location.ContainerID]; ok {
			return
		}
		seen[location.ContainerID] = struct{}{}
		container, ok := w.objects[location.ContainerID]
		if !ok {
			return
		}
		w.markObjectLocationDirtyLocked(container.Location, seen)
	}
}

func (w *World) validateObjectDestinationLocked(objectID model.ObjectInstanceID, location model.ObjectLocation) error {
	switch {
	case !location.RoomID.IsZero():
		if _, ok := w.rooms[location.RoomID]; !ok {
			return fmt.Errorf("move object %q: target room %q not found", objectID, location.RoomID)
		}
	case !location.CreatureID.IsZero():
		if _, ok := w.creatures[location.CreatureID]; !ok {
			return fmt.Errorf("move object %q: target creature %q not found", objectID, location.CreatureID)
		}
	case !location.BankID.IsZero():
		if _, ok := w.banks[location.BankID]; !ok {
			return fmt.Errorf("move object %q: target bank %q not found", objectID, location.BankID)
		}
	case !location.ContainerID.IsZero():
		if _, ok := w.objects[location.ContainerID]; !ok {
			return fmt.Errorf("move object %q: target container %q not found", objectID, location.ContainerID)
		}
		if location.ContainerID == objectID {
			return fmt.Errorf("move object %q: object cannot contain itself", objectID)
		}
		if w.containsObjectAncestorLocked(location.ContainerID, objectID) {
			return fmt.Errorf("move object %q: object cannot move into descendant %q", objectID, location.ContainerID)
		}
	}
	return nil
}

func (w *World) containsObjectAncestorLocked(start, want model.ObjectInstanceID) bool {
	seen := map[model.ObjectInstanceID]struct{}{}
	for id := start; !id.IsZero(); {
		if id == want {
			return true
		}
		if _, ok := seen[id]; ok {
			return false
		}
		seen[id] = struct{}{}
		object, ok := w.objects[id]
		if !ok {
			return false
		}
		id = object.Location.ContainerID
	}
	return false
}

func (w *World) addObjectToHolderLocked(objectID model.ObjectInstanceID, location model.ObjectLocation) {
	switch {
	case !location.RoomID.IsZero():
		room := w.rooms[location.RoomID]
		room.Objects.ObjectIDs = w.insertObjectIDLegacySortedLocked(room.Objects.ObjectIDs, objectID)
		w.rooms[location.RoomID] = room
	case !location.CreatureID.IsZero():
		creature := w.creatures[location.CreatureID]
		creature.Inventory.ObjectIDs = w.insertObjectIDLegacySortedLocked(creature.Inventory.ObjectIDs, objectID)
		if location.Slot != "" && location.Slot != "inventory" {
			if creature.Equipment == nil {
				creature.Equipment = map[string]model.ObjectInstanceID{}
			}
			creature.Equipment[location.Slot] = objectID
		}
		w.creatures[location.CreatureID] = creature
	case !location.BankID.IsZero():
		account := w.banks[location.BankID]
		account.Objects.ObjectIDs = w.insertObjectIDLegacySortedLocked(account.Objects.ObjectIDs, objectID)
		w.banks[location.BankID] = account
	case !location.ContainerID.IsZero():
		container := w.objects[location.ContainerID]
		container.Contents.ObjectIDs = w.insertObjectIDLegacySortedLocked(container.Contents.ObjectIDs, objectID)
		w.objects[location.ContainerID] = container
	}
}

func (w *World) insertObjectIDLegacySortedLocked(ids []model.ObjectInstanceID, objectID model.ObjectInstanceID) []model.ObjectInstanceID {
	for _, existing := range ids {
		if existing == objectID {
			return ids
		}
	}
	out := make([]model.ObjectInstanceID, 0, len(ids)+1)
	inserted := false
	for _, existing := range ids {
		if !inserted && w.objectIDLegacyLessLocked(objectID, existing) {
			out = append(out, objectID)
			inserted = true
		}
		out = append(out, existing)
	}
	if !inserted {
		out = append(out, objectID)
	}
	return out
}

func (w *World) objectIDLegacyLessLocked(leftID, rightID model.ObjectInstanceID) bool {
	leftName := w.objectLegacySortNameLocked(leftID)
	rightName := w.objectLegacySortNameLocked(rightID)
	if cmp := strings.Compare(leftName, rightName); cmp != 0 {
		return cmp < 0
	}
	leftAdjustment := w.objectLegacyAdjustmentLocked(leftID)
	rightAdjustment := w.objectLegacyAdjustmentLocked(rightID)
	return leftAdjustment < rightAdjustment
}

func (w *World) objectLegacySortNameLocked(objectID model.ObjectInstanceID) string {
	object, ok := w.objects[objectID]
	if !ok {
		return string(objectID)
	}
	return w.objectLegacySortNameFromObjectLocked(object)
}

func (w *World) objectLegacySortNameFromObjectLocked(object model.ObjectInstance) string {
	if name := strings.TrimSpace(object.DisplayNameOverride); name != "" {
		return name
	}
	if name := strings.TrimSpace(object.Properties["name"]); name != "" {
		return name
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := w.prototypes[object.PrototypeID]; ok {
			if name := strings.TrimSpace(proto.Properties["name"]); name != "" {
				return name
			}
			if name := strings.TrimSpace(proto.DisplayName); name != "" {
				return name
			}
			if name := firstStateObjectKeyName(proto.Properties); name != "" {
				return name
			}
		}
	}
	if name := firstStateObjectKeyName(object.Properties); name != "" {
		return name
	}
	return string(object.ID)
}

func firstStateObjectKeyName(properties map[string]string) string {
	for _, key := range []string{"key[0]", "key[1]", "key[2]", "key/1", "key/2", "key/3"} {
		if name := strings.TrimSpace(properties[key]); name != "" {
			return name
		}
	}
	return ""
}

func (w *World) objectLegacyAdjustmentLocked(objectID model.ObjectInstanceID) int {
	object, ok := w.objects[objectID]
	if !ok {
		return 0
	}
	adjustment, _ := w.objectIntPropertyAnyLocked(object, "adjustment", "adjust")
	return adjustment
}

func (w *World) carriedObjectWeightLocked(objectID model.ObjectInstanceID, skipWeightless bool, seen map[model.ObjectInstanceID]struct{}) int {
	if objectID.IsZero() {
		return 0
	}
	if _, ok := seen[objectID]; ok {
		return 0
	}
	seen[objectID] = struct{}{}

	object, ok := w.objects[objectID]
	if !ok {
		return 0
	}
	if skipWeightless && w.objectWeightlessLocked(object) {
		return 0
	}

	weight := w.objectOwnWeightLocked(object)
	for _, childID := range object.Contents.ObjectIDs {
		weight += w.carriedObjectWeightLocked(childID, true, seen)
	}
	return weight
}

func (w *World) objectOwnWeightLocked(object model.ObjectInstance) int {
	if weight, ok := parseMoveObjectWeight(object.Properties["weight"]); ok {
		return weight
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := w.prototypes[object.PrototypeID]; ok {
			if weight, ok := parseMoveObjectWeight(proto.Properties["weight"]); ok {
				return weight
			}
		}
	}
	return 0
}

func (w *World) objectWeightlessLocked(object model.ObjectInstance) bool {
	if hasAnyNormalizedFlag(object.Metadata.Tags, "weightless", "owtles") {
		return true
	}
	if objectHasAnyPropertyFlag(object.Properties, "weightless", "owtles") {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := w.prototypes[object.PrototypeID]
	if !ok {
		return false
	}
	return hasAnyNormalizedFlag(proto.Metadata.Tags, "weightless", "owtles") ||
		objectHasAnyPropertyFlag(proto.Properties, "weightless", "owtles")
}

func objectFlagContainerProperty(key string) bool {
	switch normalizeFlagName(key) {
	case "flag", "flags":
		return true
	default:
		return false
	}
}

func cloneObject(object model.ObjectInstance) model.ObjectInstance {
	object.Contents = cloneObjectRefList(object.Contents)
	object.Properties = maps.Clone(object.Properties)
	object.Metadata = cloneMetadata(object.Metadata)
	return object
}

func cloneObjectPrototype(proto model.ObjectPrototype) model.ObjectPrototype {
	proto.Keywords = slices.Clone(proto.Keywords)
	proto.Properties = maps.Clone(proto.Properties)
	proto.Metadata = cloneMetadata(proto.Metadata)
	return proto
}

func cloneObjectRefList(refs model.ObjectRefList) model.ObjectRefList {
	refs.ObjectIDs = slices.Clone(refs.ObjectIDs)
	return refs
}

func clonePrototypeResolution(resolution *model.PrototypeResolutionMetadata) *model.PrototypeResolutionMetadata {
	if resolution == nil {
		return nil
	}
	cloned := *resolution
	cloned.Candidates = slices.Clone(resolution.Candidates)
	return &cloned
}
