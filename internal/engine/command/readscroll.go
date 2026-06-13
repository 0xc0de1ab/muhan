package command

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/textfmt"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type ReadScrollWorld interface {
	StatusWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	DestroyCreatureInventoryObject(model.ObjectInstanceID, model.CreatureID) (bool, error)
	MoveObject(model.ObjectInstanceID, model.ObjectLocation) error
}

type ScrollEffectFunc func(*Context, ReadScrollWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error)

const readScrollCooldownKey = "reads"
const readScrollCooldownSeconds int64 = 3

type readScrollCooldownWorld interface {
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
}

func NewReadScrollHandler(world ReadScrollWorld, root string, effect ScrollEffectFunc) Handler {
	boardAlias := NewBoardReadAliasHandler(world, root)
	if effect == nil {
		effect = defaultReadScrollMagicEffect
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
		room, ok := world.Room(player.RoomID)
		if !ok {
			return StatusDefault, fmt.Errorf("readscroll: room %q not found", player.RoomID)
		}

		if readScrollShouldUseBoard(world, room, resolved, inventoryViewerDetectsInvisible(player, creature)) {
			return boardAlias(ctx, readScrollBoardAliasResolved(resolved))
		}

		target := getArg(resolved, 0)
		if target == "" {
			ctx.WriteString("무엇을 읽습니까?\n")
			return StatusDefault, nil
		}
		if readScrollActorIsBlind(world, player, creature) {
			ctx.WriteString("\n당신은 그것을 읽을 수 있는 능력이 없습니다.\n")
			return StatusDefault, nil
		}

		object, name, ok := findReadScrollObject(world, creature, room, target, firstGetOrdinal(resolved), inventoryViewerDetectsInvisible(player, creature))
		if !ok {
			ctx.WriteString("\n 그런것이 존재하지 않습니다.\n")
			return StatusDefault, nil
		}
		if special, ok := legacyObjectSpecial(world, object); ok {
			switch special {
			case legacySpecialMapScroll:
				return readSpecialMapScroll(ctx, world, root, object)
			}
		}
		if !readScrollObjectIsScroll(world, object) {
			ctx.WriteString("\n이것은 문서구가 아닙니다.\n")
			return StatusDefault, nil
		}
		if readScrollLevelRestricted(world, creature, object) {
			ctx.WriteString("\n당신의 능력으로는 " + name + "의 내용을 파악하지 못해 연마할 수 없습니다.")
			return StatusDefault, nil
		}
		if magicObjectAlignmentRejected(world, creature, object) {
			if err := world.MoveObject(object.ID, model.ObjectLocation{RoomID: room.ID}); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(readScrollDustMessage(name))
			return StatusDefault, nil
		}
		if magicObjectClassRestricted(world, creature, object) {
			ctx.WriteString("\n이것은 당신의 직업에서 금하는 금서이기 때문에 내용을 읽을 수 없습니다.\n")
			return StatusDefault, nil
		}
		if readScrollMagicPower(world, object) < 1 {
			ctx.WriteString("\n아무런 일도 일어나지 않았습니다.\n")
			return StatusDefault, nil
		}
		if roomHasAnyFlag(room, "noMagic", "rnomag") {
			ctx.WriteString("\n아무런 일도 일어나지 않았습니다.\n")
			return StatusDefault, nil
		}

		if ok, err := readScrollUseCooldown(ctx, world, creature); err != nil {
			return StatusDefault, err
		} else if !ok {
			return StatusDefault, nil
		}

		player, creature, err = clearCommandActorHidden(world, player, creature)
		if err != nil {
			return StatusDefault, err
		}

		if spellFail(creature) {
			ctx.WriteString(readScrollDustMessage(name))
			if _, err := world.DestroyCreatureInventoryObject(object.ID, creature.ID); err != nil {
				return StatusDefault, err
			}
			return StatusDefault, nil
		}

		success, err := effect(ctx, world, creature, object, resolved)
		if err != nil {
			return StatusDefault, err
		}
		if !success {
			return StatusDefault, nil
		}
		if text := readScrollObjectUseOutput(world, object); text != "" {
			ctx.WriteString(ensureTrailingNewline(text))
		}
		ctx.WriteString(readScrollDustMessage(name))
		if _, err := world.DestroyCreatureInventoryObject(object.ID, creature.ID); err != nil {
			return StatusDefault, err
		}
		return StatusDefault, nil
	}
}

func readScrollUseCooldown(ctx *Context, world ReadScrollWorld, creature model.Creature) (bool, error) {
	cooldowns, ok := world.(readScrollCooldownWorld)
	if !ok {
		return true, nil
	}
	now := timeNow().Unix()
	remaining, ready, err := cooldowns.UseCreatureCooldown(creature.ID, readScrollCooldownKey, now, 0)
	if err != nil {
		return false, err
	}
	if !ready {
		ctx.WriteString(renderPleaseWait(remaining))
		return false, nil
	}
	if err := cooldowns.SetCreatureCooldown(creature.ID, readScrollCooldownKey, now, readScrollCooldownSeconds); err != nil {
		return false, err
	}
	return true, nil
}

func readScrollDustMessage(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "문서"
	}
	return "\n모든 것을 읽고 나자 " + name + "의 형체가 먼지로 변하면서 바람과 함께 사라져 버렸습니다.\n"
}

func readSpecialMapScroll(ctx *Context, world InventoryWorld, root string, object model.ObjectInstance) (Status, error) {
	path, ok := specialMapScrollPath(root, objectDisplayName(world, object))
	if !ok {
		ctx.WriteString("화일을 읽을 수 없습니다.\n")
		return StatusDoPrompt, nil
	}
	text, err := readLegacyTextFile(path, "special map scroll")
	if err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			ctx.WriteString("화일을 읽을 수 없습니다.\n")
			return StatusDoPrompt, nil
		}
		return StatusDefault, err
	}
	text = textfmt.RenderLegacyColors(text, textfmt.Options{})
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return renderLegacyViewFile(ctx, text, "특수 지도 읽기 상태를 시작할 수 없습니다")
}

func specialMapScrollPath(root string, name string) (string, bool) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	name = strings.ReplaceAll(strings.TrimSpace(name), " ", "_")
	if name == "" {
		return "", false
	}

	objmonRoot := filepath.Join(root, "objmon")
	path := filepath.Join(objmonRoot, filepath.FromSlash(name))
	rel, err := filepath.Rel(objmonRoot, path)
	safe := err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel)

	if !safe {
		return "", false
	}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path, true
	}
	return path, true
}

func readScrollShouldUseBoard(world ReadScrollWorld, room model.Room, resolved ResolvedCommand, detectInvisible bool) bool {
	if _, ok := findRoomBoardObject(world, room, detectInvisible); !ok {
		return false
	}
	return len(resolved.Args) == 0 || boardReadAliasTargetsBoard(resolved)
}

func readScrollBoardAliasResolved(resolved ResolvedCommand) ResolvedCommand {
	if boardReadAliasTargetsBoard(resolved) {
		return resolved
	}
	alias := resolved
	alias.Args = []string{"게시판"}
	alias.Values = []int64{1}
	return alias
}

func readScrollActorIsBlind(world ReadScrollWorld, player model.Player, creature model.Creature) bool {
	return settingsPlayerFlag(player, "blind", "pblind") ||
		creatureHasAnyFlag(creature, "blind", "pblind")
}

func findReadScrollObject(
	world InventoryWorld,
	creature model.Creature,
	_ model.Room,
	target string,
	ordinal int64,
	detectInvisible bool,
) (model.ObjectInstance, string, bool) {
	if object, name, ok := findEquipInventoryObjectWithVisibility(world, creature, target, ordinal, detectInvisible); ok {
		return object, name, true
	}
	if object, name, ok := findEquippedObject(world, creature, target, ordinal); ok {
		return object, name, true
	}
	return model.ObjectInstance{}, "", false
}

func readScrollObjectIsScroll(world InventoryWorld, object model.ObjectInstance) bool {
	return objectLegacyType(world, object) == legacyObjectScroll ||
		objectKindIs(world, object, model.ObjectKindScroll)
}

func readScrollMagicPower(world InventoryWorld, object model.ObjectInstance) int {
	if value, ok := objectIntProperty(world, object, "magicPower"); ok {
		return value
	}
	if value, ok := objectIntProperty(world, object, "magicpower"); ok {
		return value
	}
	return 0
}

func readScrollLevelRestricted(world InventoryWorld, creature model.Creature, object model.ObjectInstance) bool {
	required, ok := objectIntProperty(world, object, "nDice")
	if !ok {
		required, ok = objectIntProperty(world, object, "ndice")
	}
	return ok && required > magicCreatureLevel(creature)
}

func readScrollObjectUseOutput(world InventoryWorld, object model.ObjectInstance) string {
	if text := object.Properties["useOutput"]; strings.TrimSpace(text) != "" {
		return text
	}
	if object.PrototypeID.IsZero() {
		return ""
	}
	proto, ok := world.ObjectPrototype(object.PrototypeID)
	if !ok {
		return ""
	}
	if text := proto.Properties["useOutput"]; strings.TrimSpace(text) != "" {
		return text
	}
	return ""
}

func magicObjectAlignmentRejected(world InventoryWorld, creature model.Creature, object model.ObjectInstance) bool {
	alignment := creatureStat(creature, "alignment")
	if alignment < -100 && magicObjectHasFlag(world, object, "goodOnly", "ogoodo", "OGOODO") {
		return true
	}
	return alignment > 100 && magicObjectHasFlag(world, object, "evilOnly", "oevilo", "OEVILO")
}

func magicObjectClassRestricted(world InventoryWorld, creature model.Creature, object model.ObjectInstance) bool {
	if !magicObjectHasFlag(world, object, "classSelective", "oclsel", "OCLSEL") {
		return false
	}
	class := creatureStat(creature, "class")
	if class >= model.ClassCaretaker {
		return false
	}
	names := magicClassFlagNames(class)
	return len(names) == 0 || !magicObjectHasFlag(world, object, names...)
}

func magicObjectHasFlag(world InventoryWorld, object model.ObjectInstance, names ...string) bool {
	return objectHasAnyTag(world, object, names...) || objectHasAnyPropertyFlag(world, object, names...)
}

func magicClassFlagNames(class int) []string {
	switch class {
	case 0:
		return []string{"classSelective", "oclsel", "OCLSEL"}
	case 1:
		return []string{"classAssassin", "oclsel1", "OCLSEL1"}
	case 2:
		return []string{"classBarbarian", "oclsel2", "OCLSEL2"}
	case 3:
		return []string{"classCleric", "oclsel3", "OCLSEL3"}
	case 4:
		return []string{"classFighter", "oclsel4", "OCLSEL4"}
	case 5:
		return []string{"classMage", "oclsel5", "OCLSEL5"}
	case 6:
		return []string{"classPaladin", "oclsel6", "OCLSEL6"}
	case 7:
		return []string{"classRanger", "oclsel7", "OCLSEL7"}
	case 8:
		return []string{"classThief", "oclsel8", "OCLSEL8"}
	default:
		return nil
	}
}

func magicCreatureLevel(creature model.Creature) int {
	level := creature.Level
	if statsLevel := creatureStat(creature, "level"); statsLevel > level {
		level = statsLevel
	}
	return level
}
