package command

import (
	"fmt"
	"strings"

	"muhan/internal/world/model"
)

// DMForceWorld defines the world interface required by the dm_force command.
type DMForceWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	ForcePlayerCommand(playerID model.PlayerID, cmd string) error
}

type dmForceAvailabilityWorld interface {
	CanForcePlayerCommand(playerID model.PlayerID) bool
}

// NewDMForceHandler constructs the Handler for the dm_force command.
func NewDMForceHandler(world DMForceWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return dmForce(ctx, resolved, world)
	}
}

func dmForce(ctx *Context, resolved ResolvedCommand, world DMForceWorld) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	actorID := strings.TrimSpace(ctx.ActorID)
	creature, ok := resolveActorCreatureForForce(world, actorID)
	if !ok {
		return StatusDefault, nil
	}

	// 1. Validate player class permissions: SUB_DM (12+)
	class := creatureClass(creature)
	if class < legacyClassSubDM {
		return StatusPrompt, nil
	}

	// 2. C checks cmnd->num and reads cmnd->str[1] for the target name.
	targetArg, ok := dmForceTargetArg(resolved)
	if !ok {
		return StatusPrompt, nil
	}

	// 3. Find target player by name after C lowercize(..., 1) normalization.
	targetName := legacyLowercizeASCII(targetArg, true)
	targetPlayer, targetCreature, ok := dmForceFindOnlinePlayer(ctx, world, targetName)
	if !ok {
		ctx.WriteString(fmt.Sprintf("%s가 없습니다.\n", targetName))
		return StatusDefault, nil
	}

	// 4. Check DM rank protection: C protects exactly class DM, not every higher numeric class.
	targetClass := creatureClass(targetCreature)
	if targetClass == legacyClassDM && class < legacyClassDM {
		return StatusPrompt, nil
	}

	if availability, ok := world.(dmForceAvailabilityWorld); ok && !availability.CanForcePlayerCommand(targetPlayer.ID) {
		ctx.WriteString(fmt.Sprintf("%s를 현재 강요할수 없습니다.\n", targetName))
		return StatusDefault, nil
	}

	// 5. Extract the remainder of the command line after the target name
	forcedCmd := extractForcedCommand(resolved)

	// Force execution of that command on the target player connection
	err := world.ForcePlayerCommand(targetPlayer.ID, forcedCmd)
	if err != nil {
		return StatusDefault, err
	}

	return StatusPrompt, nil
}

// resolveActorCreatureForForce resolves the model.Creature for a given actorID.
func resolveActorCreatureForForce(world DMForceWorld, actorID string) (model.Creature, bool) {
	playerID := model.PlayerID(actorID)
	if player, ok := world.Player(playerID); ok {
		if !player.CreatureID.IsZero() {
			if creature, ok := world.Creature(player.CreatureID); ok {
				return creature, ok
			}
		}
	}
	creatureID := model.CreatureID(actorID)
	creature, ok := world.Creature(creatureID)
	return creature, ok
}

func dmForceFindOnlinePlayer(ctx *Context, world DMForceWorld, name string) (model.Player, model.Creature, bool) {
	player, creature, _, ok := legacyFindWhoActivePlayer(ctx, world, name)
	return player, creature, ok
}

func dmForceTargetArg(resolved ResolvedCommand) (string, bool) {
	if resolved.Parsed.Num > 0 {
		if resolved.Parsed.Num < 2 {
			return "", false
		}
		return resolved.Parsed.Str[1], true
	}
	if len(resolved.Args) < 1 {
		return "", false
	}
	return resolved.Args[0], true
}

// extractForcedCommand extracts the command line after the target name.
func extractForcedCommand(resolved ResolvedCommand) string {
	input := strings.TrimSpace(resolved.Input)
	command := strings.TrimSpace(resolved.Command())
	if command == "" {
		command = strings.TrimSpace(resolved.Spec.Name)
	}

	fields := strings.Fields(input)
	if input != "" && command != "" && len(fields) >= 3 {
		if strings.EqualFold(fields[0], command) {
			if afterCommand, ok := textAfterFirstToken(input); ok {
				if afterTarget, ok := textAfterFirstToken(strings.TrimSpace(afterCommand)); ok {
					return strings.TrimSpace(afterTarget)
				}
			}
			return ""
		}
		if strings.EqualFold(fields[len(fields)-1], command) {
			if beforeCommand, ok := textBeforeLastToken(input); ok {
				if afterTarget, ok := textAfterFirstToken(strings.TrimSpace(beforeCommand)); ok {
					return strings.TrimSpace(afterTarget)
				}
			}
			return ""
		}
	}

	if len(resolved.Args) > 1 {
		return strings.TrimSpace(strings.Join(resolved.Args[1:], " "))
	}
	return ""
}
