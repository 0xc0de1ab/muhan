package objectmap

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"muhan/internal/migrate/protoresolve"
	"muhan/internal/persist/cbin"
	"muhan/internal/world/model"
)

const (
	legacySource                     = "legacy"
	legacyEncoding                   = "euc-kr/cp949"
	maxPrototypeResolutionCandidates = 5
)

type Options struct {
	PrototypeResolver protoresolve.ObjectPrototypeResolver
	SourcePath        string
}

type Result struct {
	Objects             []model.ObjectInstance    `json:"objects"`
	Warnings            []string                  `json:"warnings,omitempty"`
	PrototypeResolution PrototypeResolutionCounts `json:"prototypeResolution"`
}

type PrototypeResolutionCounts struct {
	ResolvedExact      int `json:"resolvedExact"`
	Synthetic          int `json:"synthetic"`
	AmbiguousSynthetic int `json:"ambiguousSynthetic"`
}

// MapObjectFile decodes one legacy object-tree file and maps it to canonical
// object instances. Bank files usually need allowTrailing because the legacy
// format can carry unrelated bytes after the object tree.
func MapObjectFile(rootIDPrefix string, location model.ObjectLocation, data []byte, allowTrailing bool) ([]model.ObjectInstance, []string, error) {
	result, err := MapObjectFileWithOptions(rootIDPrefix, location, data, allowTrailing, Options{})
	if err != nil {
		return nil, nil, err
	}
	return result.Objects, result.Warnings, nil
}

func MapObjectFileWithOptions(rootIDPrefix string, location model.ObjectLocation, data []byte, allowTrailing bool, opts Options) (Result, error) {
	node, err := cbin.DecodeObjectTree(data, allowTrailing)
	if err != nil {
		return Result{}, err
	}
	return MapObjectTreeWithOptions(rootIDPrefix, location, node, opts), nil
}

// MapObjectTree maps an offset-aware cbin object tree to a flat canonical
// instance list. The root receives the supplied location; every child is placed
// inside its parent through ObjectLocation.ContainerID and parent Contents.
func MapObjectTree(rootIDPrefix string, location model.ObjectLocation, node cbin.ObjectNode) ([]model.ObjectInstance, []string) {
	result := MapObjectTreeWithOptions(rootIDPrefix, location, node, Options{})
	return result.Objects, result.Warnings
}

func MapObjectTreeWithOptions(rootIDPrefix string, location model.ObjectLocation, node cbin.ObjectNode, opts Options) Result {
	m := mapper{
		prefix:            cleanPrefix(rootIDPrefix),
		useOffsets:        offsetsAreUnique(node),
		prototypeResolver: opts.PrototypeResolver,
		sourcePath:        filepath.ToSlash(opts.SourcePath),
	}
	objects := make([]model.ObjectInstance, 0, countObjects(node))
	var counts PrototypeResolutionCounts
	m.appendNode(&objects, &counts, location, node, nil)
	return Result{
		Objects:             objects,
		Warnings:            treeWarnings(node.Warnings),
		PrototypeResolution: counts,
	}
}

type mapper struct {
	prefix            string
	useOffsets        bool
	prototypeResolver protoresolve.ObjectPrototypeResolver
	sourcePath        string
}

func (m mapper) appendNode(objects *[]model.ObjectInstance, counts *PrototypeResolutionCounts, location model.ObjectLocation, node cbin.ObjectNode, path []int) model.ObjectInstanceID {
	id := m.instanceID(node.Offset, path)
	prototypeID, tags, notes, prototypeResolution := m.resolvePrototype(node, path, counts)
	tags = append(tags, objectFlagNames(node.Record.Flags)...)
	objectTreePath := treePath(path)
	legacyPath := objectTreePath
	if m.sourcePath != "" {
		legacyPath = m.sourcePath
		notes = append([]string{"objectTreePath=" + objectTreePath}, notes...)
	}
	instance := model.ObjectInstance{
		ID:                  id,
		PrototypeID:         prototypeID,
		DisplayNameOverride: textIfValid(node.Record.Name),
		Quantity:            1,
		Location:            location,
		Properties:          properties(node.Record),
		Metadata: model.Metadata{
			Source:              legacySource,
			LegacyKind:          "objectTreeObject",
			LegacyID:            string(id),
			LegacyPath:          legacyPath,
			LegacyEncoding:      legacyEncoding,
			RecordOffset:        int64(node.Offset),
			ObjectTreePath:      objectTreePath,
			RawFields:           rawFields(node.Record),
			Tags:                tags,
			Notes:               notes,
			PrototypeResolution: prototypeResolution,
		},
	}

	index := len(*objects)
	*objects = append(*objects, instance)
	for i, child := range node.Children {
		childPath := appendPath(path, i)
		childID := m.appendNode(objects, counts, model.ObjectLocation{ContainerID: id}, child, childPath)
		(*objects)[index].Contents.ObjectIDs = append((*objects)[index].Contents.ObjectIDs, childID)
	}
	return id
}

func (m mapper) resolvePrototype(node cbin.ObjectNode, path []int, counts *PrototypeResolutionCounts) (model.PrototypeID, []string, []string, *model.PrototypeResolutionMetadata) {
	syntheticID := m.prototypeID(node.Offset, path)
	if m.prototypeResolver == nil {
		counts.Synthetic++
		return syntheticID,
			[]string{"prototype:synthetic"},
			[]string{"prototypeMatch method=synthetic confidence=fallback"},
			m.prototypeResolution(node.Record, "synthetic", "synthetic", "fallback", syntheticID, syntheticID, nil)
	}

	resolution := m.prototypeResolver.ResolveObjectPrototype(node.Record)
	if resolution.Resolved {
		counts.ResolvedExact++
		return resolution.PrototypeID,
			[]string{"prototype:resolved"},
			[]string{fmt.Sprintf("prototypeMatch method=%s confidence=%s", resolution.Method, resolution.Confidence)},
			m.prototypeResolution(node.Record, "resolved", resolution.Method, resolution.Confidence, resolution.PrototypeID, "", resolution.Candidates)
	}

	counts.Synthetic++
	if resolution.Ambiguous {
		counts.AmbiguousSynthetic++
		return syntheticID,
			[]string{"prototype:ambiguous", "prototype:synthetic"},
			[]string{
				fmt.Sprintf("prototypeMatch method=%s confidence=%s", resolution.Method, resolution.Confidence),
				"prototypeCandidates=" + candidateList(resolution.Candidates),
			},
			m.prototypeResolution(node.Record, "ambiguous", resolution.Method, resolution.Confidence, syntheticID, syntheticID, resolution.Candidates)
	}

	return syntheticID,
		[]string{"prototype:synthetic"},
		[]string{fmt.Sprintf("prototypeMatch method=%s confidence=fallback", resolution.Method)},
		m.prototypeResolution(node.Record, "unresolved", resolution.Method, "fallback", syntheticID, syntheticID, nil)
}

func (m mapper) prototypeResolution(record cbin.ObjectRecord, status, method, confidence string, selectedID, syntheticID model.PrototypeID, candidates []protoresolve.ObjectCandidate) *model.PrototypeResolutionMetadata {
	if method == "" {
		method = "synthetic"
	}
	out := &model.PrototypeResolutionMetadata{
		Status:               status,
		Method:               method,
		Confidence:           confidence,
		SelectedPrototypeID:  selectedID,
		SyntheticPrototypeID: syntheticID,
		CandidateCount:       len(candidates),
		Candidates:           prototypeCandidates(candidates),
	}
	if fingerprint := protoresolve.FingerprintObjectRecord(record); fingerprint != "" {
		out.Fingerprint = fingerprint
		out.FingerprintAlgorithm = protoresolve.ObjectFingerprintAlgorithm
		out.ComparableBytes = protoresolve.ObjectFingerprintComparableBytes
	}
	return out
}

func prototypeCandidates(candidates []protoresolve.ObjectCandidate) []model.PrototypeResolutionCandidate {
	if len(candidates) == 0 {
		return nil
	}
	limit := len(candidates)
	if limit > maxPrototypeResolutionCandidates {
		limit = maxPrototypeResolutionCandidates
	}
	out := make([]model.PrototypeResolutionCandidate, 0, limit)
	for _, candidate := range candidates[:limit] {
		out = append(out, model.PrototypeResolutionCandidate{
			PrototypeID:  candidate.PrototypeID,
			Path:         candidate.Path,
			Index:        candidate.Index,
			LegacyNumber: candidate.LegacyNumber,
			RecordOffset: candidate.RecordOffset,
		})
	}
	return out
}

func (m mapper) instanceID(offset int, path []int) model.ObjectInstanceID {
	return model.ObjectInstanceID(m.prefix + ":" + m.suffix(offset, path))
}

func (m mapper) prototypeID(offset int, path []int) model.PrototypeID {
	return model.PrototypeID("object:" + m.prefix + ":" + m.suffix(offset, path))
}

func (m mapper) suffix(offset int, path []int) string {
	if m.useOffsets {
		return fmt.Sprintf("%08d", offset)
	}
	return treePath(path)
}

func cleanPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.TrimRight(prefix, ":/")
	if prefix == "" {
		return "objinst"
	}
	return prefix
}

func offsetsAreUnique(node cbin.ObjectNode) bool {
	seen := map[int]struct{}{}
	return walkOffsets(node, seen)
}

func walkOffsets(node cbin.ObjectNode, seen map[int]struct{}) bool {
	if _, ok := seen[node.Offset]; ok {
		return false
	}
	seen[node.Offset] = struct{}{}
	for _, child := range node.Children {
		if !walkOffsets(child, seen) {
			return false
		}
	}
	return true
}

func countObjects(node cbin.ObjectNode) int {
	count := 1
	for _, child := range node.Children {
		count += countObjects(child)
	}
	return count
}

func appendPath(path []int, child int) []int {
	out := make([]int, 0, len(path)+1)
	out = append(out, path...)
	out = append(out, child)
	return out
}

func treePath(path []int) string {
	if len(path) == 0 {
		return "0"
	}
	var b strings.Builder
	b.WriteByte('0')
	for _, part := range path {
		fmt.Fprintf(&b, ".%d", part)
	}
	return b.String()
}

func properties(record cbin.ObjectRecord) map[string]string {
	props := map[string]string{}
	addTextProperty(props, "name", record.Name)
	addTextProperty(props, "description", record.Description)
	for i, key := range record.Keys {
		addTextProperty(props, fmt.Sprintf("key[%d]", i), key)
	}
	addTextProperty(props, "useOutput", record.UseOutput)
	addInt32Property(props, "value", record.Value)
	if record.Weight != 0 {
		props["weight"] = strconv.Itoa(int(record.Weight))
	}
	addInt8Property(props, "type", record.Type)
	if objectKind(record.Type) == model.ObjectKindWand {
		props["kind"] = string(model.ObjectKindWand)
	}
	addInt8Property(props, "adjustment", record.Adjustment)
	addInt16Property(props, "shotsMax", record.ShotsMax)
	addInt16Property(props, "shotsCurrent", record.ShotsCurrent)
	addInt16Property(props, "nDice", record.NDice)
	addInt16Property(props, "sDice", record.SDice)
	addInt16Property(props, "pDice", record.PDice)
	addInt8Property(props, "armor", record.Armor)
	addInt8Property(props, "wearFlag", record.WearFlag)
	addInt8Property(props, "magicPower", record.MagicPower)
	addInt8Property(props, "magicRealm", record.MagicRealm)
	addInt16Property(props, "special", record.Special)
	addInt8Property(props, "questNumber", record.QuestNumber)
	if len(props) == 0 {
		return nil
	}
	return props
}

func addInt32Property(props map[string]string, name string, value int32) {
	if value != 0 {
		props[name] = strconv.FormatInt(int64(value), 10)
	}
}

func addInt16Property(props map[string]string, name string, value int16) {
	if value != 0 {
		props[name] = strconv.Itoa(int(value))
	}
}

func addInt8Property(props map[string]string, name string, value int8) {
	if value != 0 {
		props[name] = strconv.Itoa(int(value))
	}
}

func objectFlagNames(flags [8]byte) []string {
	names := []string{
		"permanent",
		"hidden",
		"invisible",
		"somePrefix",
		"noPluralSuffix",
		"noPrefix",
		"container",
		"weightless",
		"temporaryPermanent",
		"inventoryPermanent",
		"noMage",
		"lightSource",
		"goodOnly",
		"evilOnly",
		"enchanted",
		"noRepair",
		"climbGear",
		"noTake",
		"scenery",
		"sizeSmall",
		"sizeLarge",
		"randomEnchantment",
		"cursed",
		"worn",
		"useFromFloor",
		"containerDevours",
		"femaleOnly",
		"maleOnly",
		"damageDice",
		"pledgeOnly",
		"kingdomBound",
		"classSelective",
		"classAssassin",
		"classBarbarian",
		"classCleric",
		"classFighter",
		"classMage",
		"classPaladin",
		"classRanger",
		"classThief",
		"stunDice",
		"neverShatter",
		"alwaysCritical",
		"customName",
		"specialItem",
		"marriageOnly",
		"eventItem",
		"named",
		"noBurn",
		"wheld",
	}

	out := make([]string, 0, len(names))
	for bit, name := range names {
		if name == "" {
			continue
		}
		if flags[bit/8]&(1<<uint(bit%8)) != 0 {
			out = append(out, name)
		}
	}
	return out
}

func objectKind(legacyType int8) model.ObjectKind {
	switch legacyType {
	case 0, 1, 2, 3, 4:
		return model.ObjectKindWeapon
	case 5:
		return model.ObjectKindArmor
	case 6:
		return model.ObjectKindPotion
	case 7:
		return model.ObjectKindScroll
	case 8:
		return model.ObjectKindWand
	case 9, 14:
		return model.ObjectKindContainer
	case 10:
		return model.ObjectKindMoney
	case 11:
		return model.ObjectKindKey
	case 12:
		return model.ObjectKindLightSource
	default:
		return model.ObjectKindMisc
	}
}

func addTextProperty(props map[string]string, name string, field cbin.TextField) {
	if text := textIfValid(field); text != "" {
		props[name] = text
	}
}

func rawFields(record cbin.ObjectRecord) map[string][]byte {
	fields := map[string][]byte{}
	addRawField(fields, "name", record.Name)
	addRawField(fields, "description", record.Description)
	for i, key := range record.Keys {
		addRawField(fields, fmt.Sprintf("key[%d]", i), key)
	}
	addRawField(fields, "useOutput", record.UseOutput)
	addRawInt32Field(fields, "value", record.Value)
	addRawInt16Field(fields, "weight", record.Weight)
	addRawInt8Field(fields, "type", record.Type)
	addRawInt8Field(fields, "adjustment", record.Adjustment)
	addRawInt16Field(fields, "shotsMax", record.ShotsMax)
	addRawInt16Field(fields, "shotsCurrent", record.ShotsCurrent)
	addRawInt16Field(fields, "nDice", record.NDice)
	addRawInt16Field(fields, "sDice", record.SDice)
	addRawInt16Field(fields, "pDice", record.PDice)
	addRawInt8Field(fields, "armor", record.Armor)
	addRawInt8Field(fields, "wearFlag", record.WearFlag)
	addRawInt8Field(fields, "magicPower", record.MagicPower)
	addRawInt8Field(fields, "magicRealm", record.MagicRealm)
	addRawInt16Field(fields, "special", record.Special)
	addRawBytesField(fields, "flags", record.Flags[:])
	addRawInt8Field(fields, "questNumber", record.QuestNumber)
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func addRawInt32Field(fields map[string][]byte, name string, value int32) {
	if value == 0 {
		return
	}
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], uint32(value))
	fields[name] = raw[:]
}

func addRawInt16Field(fields map[string][]byte, name string, value int16) {
	if value == 0 {
		return
	}
	var raw [2]byte
	binary.LittleEndian.PutUint16(raw[:], uint16(value))
	fields[name] = raw[:]
}

func addRawInt8Field(fields map[string][]byte, name string, value int8) {
	if value == 0 {
		return
	}
	fields[name] = []byte{byte(value)}
}

func addRawBytesField(fields map[string][]byte, name string, raw []byte) {
	for _, b := range raw {
		if b != 0 {
			fields[name] = append([]byte(nil), raw...)
			return
		}
	}
}

func candidateList(candidates []protoresolve.ObjectCandidate) string {
	if len(candidates) == 0 {
		return ""
	}
	limit := len(candidates)
	if limit > maxPrototypeResolutionCandidates {
		limit = maxPrototypeResolutionCandidates
	}
	parts := make([]string, 0, limit)
	for _, candidate := range candidates[:limit] {
		parts = append(parts, string(candidate.PrototypeID))
	}
	if limit < len(candidates) {
		parts = append(parts, fmt.Sprintf("...%d more", len(candidates)-limit))
	}
	return strings.Join(parts, ",")
}

func addRawField(fields map[string][]byte, name string, field cbin.TextField) {
	if len(field.Raw) == 0 {
		return
	}
	raw := make([]byte, len(field.Raw))
	copy(raw, field.Raw)
	fields[name] = raw
}

func textIfValid(field cbin.TextField) string {
	if field.Err != nil {
		return ""
	}
	return field.Text
}

func treeWarnings(warnings []cbin.TreeWarning) []string {
	if len(warnings) == 0 {
		return nil
	}
	out := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, fmt.Sprintf("object tree offset %d %s decode failed: %s", warning.Offset, warning.Field, warning.Message))
	}
	return out
}
