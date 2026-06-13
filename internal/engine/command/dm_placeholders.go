package command

import (
	"fmt"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	legacyClassZoneMaker = 0

	dmPlaceholderDeniedMessage = "그 명령은 DM 권한이 있어야 사용할 수 있습니다.\n"
)

// DefaultDMPlaceholderHandlerKeys is intentionally empty now that legacy DM
// handlers are either implemented or explicitly accounted for. The placeholder
// factory remains available for opt-in tests and future triage.
var DefaultDMPlaceholderHandlerKeys = []string{}

type DMPlaceholderWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
}

func NewDMPlaceholderHandlers(world DMPlaceholderWorld, keys ...string) map[string]Handler {
	if len(keys) == 0 {
		keys = DefaultDMPlaceholderHandlerKeys
	}

	handler := NewDMPlaceholderHandler(world)
	handlers := make(map[string]Handler, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		handlers[key] = handler
	}
	return handlers
}

func NewDMPlaceholderHandler(world DMPlaceholderWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		if !dmPlaceholderAuthorized(ctx, world) {
			return StatusPrompt, nil
		}

		handler := strings.TrimSpace(resolved.Spec.Handler)
		if handler == "" {
			handler = strings.TrimSpace(resolved.Command())
		}
		if handler == "" {
			handler = "DM"
		}
		ctx.WriteString(fmt.Sprintf("DM 명령 %s은 아직 구현되지 않았습니다.\n", handler))
		return StatusDefault, nil
	}
}

func dmPlaceholderAuthorized(ctx *Context, world DMPlaceholderWorld) bool {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return false
	}
	_, creature, ok := dmPlaceholderActor(world, strings.TrimSpace(ctx.ActorID))
	if !ok {
		return false
	}
	class, ok := dmPlaceholderClass(creature)
	if !ok {
		return false
	}
	return class == legacyClassZoneMaker || class >= model.ClassCaretaker
}

func dmPlaceholderActor(world DMPlaceholderWorld, actorID string) (model.Player, model.Creature, bool) {
	playerID := model.PlayerID(actorID)
	if player, ok := world.Player(playerID); ok {
		if player.CreatureID.IsZero() {
			return player, model.Creature{}, false
		}
		creature, ok := world.Creature(player.CreatureID)
		return player, creature, ok
	}

	creatureID := model.CreatureID(actorID)
	creature, ok := world.Creature(creatureID)
	if !ok {
		return model.Player{}, model.Creature{}, false
	}
	if !creature.PlayerID.IsZero() {
		if player, ok := world.Player(creature.PlayerID); ok {
			return player, creature, true
		}
	}
	return model.Player{}, creature, true
}

func dmPlaceholderClass(creature model.Creature) (int, bool) {
	for key, value := range creature.Stats {
		if normalizeFlagName(key) == "class" {
			return value, true
		}
	}
	for key, raw := range creature.Properties {
		if normalizeFlagName(key) != "class" {
			continue
		}
		value, ok := parseObjectInt(raw)
		return value, ok
	}
	return 0, false
}
