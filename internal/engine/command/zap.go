package command

import (
	"fmt"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type ZapWorld interface {
	InventoryWorld
	Room(model.RoomID) (model.Room, bool)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	ConsumeCreatureObjectCharge(model.ObjectInstanceID, model.CreatureID, bool) (model.ObjectInstance, bool, bool, error)
	MoveObject(model.ObjectInstanceID, model.ObjectLocation) error
}

type ZapEffectFunc func(*Context, ZapWorld, model.Creature, model.ObjectInstance, ResolvedCommand) (bool, error)

func NewZapHandler(world ZapWorld, effect ZapEffectFunc) Handler {
	if effect == nil {
		effect = defaultZapMagicEffect
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
			ctx.WriteString("\n무엇을 사용합니까?\n")
			return StatusDefault, nil
		}
		if creatureHasAnyFlag(creature, "blind", "pblind") {
			ctx.WriteString("아무 것도 보이지 않습니다!\n")
			return StatusDefault, nil
		}

		object, name, ok := findZapObject(world, creature, target, firstGetOrdinal(resolved), inventoryViewerDetectsInvisible(player, creature))
		if !ok {
			ctx.WriteString("\n그런것이 존재하지 않습니다.\n")
			return StatusDefault, nil
		}
		if !zapObjectIsWand(world, object) {
			ctx.WriteString("\n막대나 지팡이가 아닙니다.\n")
			return StatusDefault, nil
		}
		if objectIntPropertyOrZero(world, object, "shotsCurrent") < 1 {
			ctx.WriteString("\n모두 써버렸습니다.\n")
			return StatusDefault, nil
		}

		room, ok := world.Room(player.RoomID)
		if !ok {
			return StatusDefault, fmt.Errorf("zap: room %q not found", player.RoomID)
		}
		if magicObjectAlignmentRejected(world, creature, object) {
			if err := world.MoveObject(object.ID, model.ObjectLocation{RoomID: room.ID}); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("\n" + name + "의 수명이 다한 듯 수증기처럼 증발해 버렸습니다.\n")
			return StatusDefault, nil
		}
		if magicObjectClassRestricted(world, creature, object) {
			ctx.WriteString("\n당신직업세계에서 금하는 물건이기에 사용할 수 없습니다.\n")
			return StatusDefault, nil
		}
		if roomHasAnyFlag(room, "noMagic", "rnomag") || zapMagicPower(world, object) < 1 {
			ctx.WriteString("\n아무런 일도 일어나지 않습니다.\n")
			return StatusDefault, nil
		}

		player, creature, err = clearCommandActorHidden(world, player, creature)
		if err != nil {
			return StatusDefault, err
		}
		if spellFail(creature) {
			if _, _, _, err := world.ConsumeCreatureObjectCharge(object.ID, creature.ID, false); err != nil {
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

		if text := zapObjectUseOutput(world, object); text != "" {
			ctx.WriteString(ensureTrailingNewline(text))
		}
		_, _, consumed, err := world.ConsumeCreatureObjectCharge(object.ID, creature.ID, false)
		if err != nil {
			return StatusDefault, err
		}
		if !consumed {
			ctx.WriteString("\n모두 써버렸습니다.\n")
		}
		return StatusDefault, nil
	}
}

func findZapObject(world InventoryWorld, creature model.Creature, target string, ordinal int64, detectInvisible bool) (model.ObjectInstance, string, bool) {
	if object, name, ok := findEquipInventoryObjectWithVisibility(world, creature, target, ordinal, detectInvisible); ok {
		return object, name, true
	}
	return findEquippedObject(world, creature, target, ordinal)
}

func zapObjectIsWand(world InventoryWorld, object model.ObjectInstance) bool {
	return objectLegacyType(world, object) == legacyObjectWand ||
		objectKindIs(world, object, model.ObjectKindWand)
}

func zapMagicPower(world InventoryWorld, object model.ObjectInstance) int {
	if value, ok := objectIntProperty(world, object, "magicPower"); ok {
		return value
	}
	if value, ok := objectIntProperty(world, object, "magicpower"); ok {
		return value
	}
	return 0
}

func zapObjectUseOutput(world InventoryWorld, object model.ObjectInstance) string {
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
