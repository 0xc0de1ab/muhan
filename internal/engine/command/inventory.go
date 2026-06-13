package command

import (
	"errors"
	"fmt"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

var (
	ErrInventoryWorldRequired    = errors.New("inventory world required")
	ErrInventoryActorRequired    = errors.New("inventory actor required")
	ErrInventoryPlayerNotFound   = errors.New("inventory player not found")
	ErrInventoryCreatureRequired = errors.New("inventory creature required")
	ErrInventoryCreatureNotFound = errors.New("inventory creature not found")
)

type InventoryWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
}

func NewInventoryHandler(world InventoryWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}

		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}

		if statusEffectActive(player, creature, "blind", "blinded", "PBLIND", "MBLIND") {
			ctx.WriteString(colorText(textOptionsFromContext(ctx), "34", legacyInventoryBlindMessage))
			return StatusDefault, nil
		}

		ctx.WriteString(renderInventory(world, creature, inventoryViewerDetectsMagic(player, creature), inventoryViewerDetectsInvisible(player, creature)))
		return StatusDefault, nil
	}
}

func InventoryPlayerIDFromContext(ctx *Context) model.PlayerID {
	if ctx == nil || ctx.ActorID == "" {
		return ""
	}
	return model.PlayerID(ctx.ActorID)
}

func CurrentInventoryCreature(world InventoryWorld, playerID model.PlayerID) (model.Player, model.Creature, error) {
	if world == nil {
		return model.Player{}, model.Creature{}, ErrInventoryWorldRequired
	}
	if playerID.IsZero() {
		return model.Player{}, model.Creature{}, ErrInventoryActorRequired
	}

	player, ok := world.Player(playerID)
	if !ok {
		return model.Player{}, model.Creature{}, fmt.Errorf("%w: %q", ErrInventoryPlayerNotFound, playerID)
	}
	if player.CreatureID.IsZero() {
		return player, model.Creature{}, fmt.Errorf("%w: player %q", ErrInventoryCreatureRequired, playerID)
	}

	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return player, model.Creature{}, fmt.Errorf("%w: %q", ErrInventoryCreatureNotFound, player.CreatureID)
	}
	return player, creature, nil
}

func RenderInventory(world InventoryWorld, creature model.Creature) string {
	return renderInventory(world, creature, viewerHasDetectMagicCreatureTag(creature), viewerHasDetectInvisibleCreatureTag(creature))
}

const legacyInventoryBlindMessage = "당신은 눈이 멀어서 아무것도 볼 수가 없습니다!"

func renderInventory(world InventoryWorld, creature model.Creature, detectMagic bool, detectInvisible bool) string {
	inventoryNames := inventoryObjectNames(world, creature.Inventory.ObjectIDs, detectMagic, detectInvisible)
	if len(inventoryNames) == 0 {
		if len(creature.Inventory.ObjectIDs) == 0 {
			return "소지품:\n  없음."
		}
		return ""
	}

	return "소지품:\n  " + strings.Join(inventoryNames, ", ") + "."
}

func inventoryViewerDetectsMagic(player model.Player, creature model.Creature) bool {
	return viewerHasDetectMagicCreatureTag(creature) || viewerHasDetectMagicPlayerTag(player)
}

func inventoryViewerDetectsInvisible(player model.Player, creature model.Creature) bool {
	return viewerHasDetectInvisibleCreatureTag(creature) || viewerHasDetectInvisiblePlayerTag(player)
}

func viewerHasDetectInvisibleCreatureTag(creature model.Creature) bool {
	return creatureHasAnyFlag(creature, "detectInvisible", "detectInvis", "PDINVI")
}

func viewerHasDetectInvisiblePlayerTag(player model.Player) bool {
	return hasAnyNormalizedFlag(player.Metadata.Tags, "detectInvisible", "detectInvis", "PDINVI")
}

type inventoryObjectGroup struct {
	Text       string
	Count      int
	Name       string
	Adjustment int
}

func inventoryObjectNames(world InventoryWorld, ids []model.ObjectInstanceID, detectMagic bool, detectInvisible bool) []string {
	groups := make([]inventoryObjectGroup, 0, len(ids))
	for _, id := range ids {
		if id.IsZero() {
			continue
		}
		object, ok := world.Object(id)
		if !ok {
			groups = append(groups, inventoryObjectGroup{Text: string(id), Count: 1})
			continue
		}
		if !detectInvisible && searchObjectInvisible(world, object) {
			continue
		}
		name := objectDisplayName(world, object)
		text := objectMagicDisplayName(world, object, detectMagic)
		if text == "" {
			continue
		}
		adjustment := objectIntPropertyOrDefault(world, object, "adjustment", "adjust")
		last := len(groups) - 1
		if last >= 0 && groups[last].Name == name && groups[last].Adjustment == adjustment {
			groups[last].Count++
			groups[last].Text = text
			continue
		}
		groups = append(groups, inventoryObjectGroup{
			Text:       text,
			Count:      1,
			Name:       name,
			Adjustment: adjustment,
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

func inventoryObjectName(world InventoryWorld, id model.ObjectInstanceID) string {
	if name, ok := objectInstanceName(world, id); ok {
		return name
	}
	return string(id)
}

type ObjectNameWorld interface {
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
}

type objectNameWorld = ObjectNameWorld

// LegacyObjectPrefixMatches reports whether target matches the C find_obj
// name/key prefix terms for object.
func LegacyObjectPrefixMatches(world ObjectNameWorld, object model.ObjectInstance, target string) bool {
	return legacyObjectPrefixMatches(world, object, target)
}

func objectInstanceName(world objectNameWorld, id model.ObjectInstanceID) (string, bool) {
	object, ok := world.Object(id)
	if !ok {
		return "", false
	}
	return objectDisplayName(world, object), true
}

func objectDisplayName(world objectNameWorld, object model.ObjectInstance) string {
	if name := cleanDisplayText(object.DisplayNameOverride); name != "" {
		return name
	}
	if name := cleanDisplayText(object.Properties["name"]); name != "" {
		return name
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if name := cleanDisplayText(proto.DisplayName); name != "" && !looksLikeInternalObjectID(name) {
				return name
			}
			if name := cleanDisplayText(proto.Properties["name"]); name != "" {
				return name
			}
			if name := firstObjectKeyName(proto.Properties); name != "" {
				return name
			}
		}
	}
	if name := firstObjectKeyName(object.Properties); name != "" {
		return name
	}
	return string(object.ID)
}

func firstObjectKeyName(properties map[string]string) string {
	for _, key := range []string{"key[0]", "key[1]", "key[2]"} {
		if name := cleanDisplayText(properties[key]); name != "" {
			return name
		}
	}
	return ""
}

func looksLikeInternalObjectID(name string) bool {
	return strings.HasPrefix(name, "object:") || strings.HasPrefix(name, "objinst:")
}

func legacyObjectEqualTerms(world objectNameWorld, object model.ObjectInstance) []string {
	terms := make([]string, 0, 4)
	terms = appendTrimmedTerm(terms, legacyObjectEqualName(world, object))
	for i := 0; i < 3; i++ {
		terms = appendTrimmedTerm(terms, legacyObjectEqualKey(world, object, i))
	}
	return terms
}

func legacyObjectPrefixMatches(world objectNameWorld, object model.ObjectInstance, target string) bool {
	target = cleanDisplayText(target)
	if target == "" {
		return false
	}
	for _, term := range legacyObjectEqualTerms(world, object) {
		if strings.HasPrefix(term, target) {
			return true
		}
	}
	return false
}

func legacyObjectEqualName(world objectNameWorld, object model.ObjectInstance) string {
	if name := cleanDisplayText(object.DisplayNameOverride); name != "" {
		return name
	}
	if name := cleanDisplayText(object.Properties["name"]); name != "" {
		return name
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if name := cleanDisplayText(proto.Properties["name"]); name != "" {
				return name
			}
			if name := cleanDisplayText(proto.DisplayName); name != "" && !looksLikeInternalObjectID(name) {
				return name
			}
		}
	}
	return ""
}

func legacyObjectEqualKey(world objectNameWorld, object model.ObjectInstance, index int) string {
	if index < 0 || index > 2 {
		return ""
	}
	key := fmt.Sprintf("key[%d]", index)
	if name := cleanDisplayText(object.Properties[key]); name != "" {
		return name
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if name := cleanDisplayText(proto.Properties[key]); name != "" {
				return name
			}
			if index < len(proto.Keywords) {
				return cleanDisplayText(proto.Keywords[index])
			}
		}
	}
	return ""
}

func appendTrimmedTerm(terms []string, term string) []string {
	term = cleanDisplayText(term)
	if term == "" {
		return terms
	}
	return append(terms, term)
}

func objectIsContainer(world objectNameWorld, object model.ObjectInstance) bool {
	containerFlags := normalizedFlagSet("container", "ocontn", "OCONTN")
	if hasAnyNormalizedFlag(object.Metadata.Tags, "container", "ocontn", "OCONTN") ||
		objectPropertiesHaveAnyFlag(object.Properties, containerFlags) {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(object.Properties["kind"]), string(model.ObjectKindContainer)) {
		return true
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if hasAnyNormalizedFlag(proto.Metadata.Tags, "container", "ocontn", "OCONTN") ||
				objectPropertiesHaveAnyFlag(proto.Properties, containerFlags) {
				return true
			}
			if proto.Kind == model.ObjectKindContainer {
				return true
			}
			if strings.EqualFold(strings.TrimSpace(proto.Properties["kind"]), string(model.ObjectKindContainer)) {
				return true
			}
		}
	}
	return len(object.Contents.ObjectIDs) > 0
}

func joinArgs(args []string) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}
