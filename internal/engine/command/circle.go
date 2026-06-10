package command

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

type CircleWorld interface {
	LookWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
}

type circleStatUpdater interface {
	SetCreatureStat(model.CreatureID, string, int) error
}

func NewCircleHandler(world CircleWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("circle: actor creature %q not found", viewer.CreatureID)
		}

		target, ordinal := lookTarget(resolved)
		if target == "" {
			ctx.WriteString("누구를 교란시키려구요?\n")
			return StatusDefault, nil
		}
		if message := circleClassRejection(actor); message != "" {
			ctx.WriteString(message)
			return StatusDefault, nil
		}

		victim, ok := findCircleTarget(world, room, viewer, target, ordinal)
		if !ok {
			ctx.WriteString("그런것은 여기 없습니다.\n")
			return StatusDefault, nil
		}
		if victim.hasPlayer {
			gate := kickPlayerCombatGate(world, room, actor, viewer.PlayerID, victim.player, victim.creature)
			if !gate.Allowed {
				ctx.WriteString(gate.Message + "\n")
				return StatusDefault, nil
			}
			gate = kickPlayerCharmGate(world, actor, viewer.PlayerID, victim.player, victim.creature, kickCharmMessageCircle)
			if !gate.Allowed {
				ctx.WriteString(gate.Message + "\n")
				return StatusDefault, nil
			}
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, "attack", now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}
		if err := revealCircleActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}
		if !victim.hasPlayer && attackCreatureProtected(victim.creature) {
			ctx.WriteString("당신은 " + stealObjectPronoun(victim.creature) + " 해칠수 없습니다.\n")
			return StatusDefault, nil
		}

		if !victim.hasPlayer {
			if adder, ok := world.(interface {
				AddEnemy(attacker, defender model.CreatureID) (bool, error)
			}); ok {
				_, _ = adder.AddEnemy(victim.creature.ID, actor.ID)
			}
			_, _ = world.UpdateCreatureTags(victim.creature.ID, []string{"was_attacked"}, nil)
		}

		actorName := attackCreatureName(actor)
		victimName := attackCreatureName(victim.creature)
		if attackRoll(1, 100) > circleChance(actor, victim.creature) {
			if err := world.SetCreatureCooldown(actor.ID, "attack", now, 3); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("당신은 적을 교란시키는데 실패하였습니다.\n")
			_ = sendToPlayer(ctx, victim.creature.PlayerID, "\n"+actorName+"이 당신을 교란시키려고 합니다.\n")
			_ = broadcastRom2(ctx, world, room.ID, viewer.PlayerID, victim.creature.PlayerID, "\n"+actorName+"이 "+victimName+krtext.Particle(victimName, '3')+" 교란시키려고 합니다.")
			return StatusDefault, nil
		}

		delay := attackRoll(6, 12)
		if creatureClass(actor) == model.ClassBarbarian {
			delay = attackRoll(6, 9)
		}
		if err := circleApplyBefuddle(world, victim, delay, now); err != nil {
			return StatusDefault, err
		}
		if err := world.SetCreatureCooldown(actor.ID, "attack", now, 2); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("당신은 이리저리 왔다갔다 하면서 " + victimName + krtext.Particle(victimName, '3') + " 교란시킵니다.\n")
		_ = sendToPlayer(ctx, victim.creature.PlayerID, actorName+"이 당신주위를 어지럽게 돌아다닙니다.\n")
		_ = broadcastRom2(ctx, world, room.ID, viewer.PlayerID, victim.creature.PlayerID, "\n"+actorName+"이 "+victimName+" 주위를 뱅글뱅글 돕니다.")
		return StatusDefault, nil
	}
}

func circleClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	if class != model.ClassFighter && class != model.ClassBarbarian && class < model.ClassInvincible {
		return "권법가와 검사만 쓸수 있는 기술입니다.\n"
	}
	if class >= model.ClassInvincible && !circleHasFighterOrBarbarianTraining(actor) {
		return "\n권법가와 검사를 무적수련하지 않았습니다..\n"
	}
	return ""
}

func circleHasFighterOrBarbarianTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SFIGHTER",
		"SBARBARIAN",
		"fighterSpell",
		"barbarianSpell",
		"fighterTraining",
		"barbarianTraining",
		"fighterMode",
		"barbarianMode",
	)
}

func findCircleTarget(world LookWorld, room model.Room, viewer LookViewer, prefix string, ordinal int64) (kickTarget, bool) {
	if creature, ok := findAttackCreatureTarget(world, room, viewer, prefix, ordinal); ok {
		return kickTarget{creature: creature}, true
	}
	if legacyByteLen(strings.TrimSpace(prefix)) < 3 {
		return kickTarget{}, false
	}
	for _, candidate := range circlePlayerPrefixes(prefix) {
		player, ok := findAttackPlayerTarget(world, room, viewer, candidate, ordinal)
		if !ok || player.CreatureID.IsZero() {
			continue
		}
		creature, ok := world.Creature(player.CreatureID)
		if !ok || creature.RoomID != room.ID || creature.ID == viewer.CreatureID {
			continue
		}
		return kickTarget{creature: creature, player: player, hasPlayer: true}, true
	}
	return kickTarget{}, false
}

func circlePlayerPrefixes(prefix string) []string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}
	runes := []rune(prefix)
	titled := string(append([]rune{unicode.ToUpper(runes[0])}, runes[1:]...))
	if titled == prefix {
		return []string{prefix}
	}
	return []string{prefix, titled}
}

func revealCircleActor(ctx *Context, world CircleWorld, roomID model.RoomID, viewer LookViewer, actor model.Creature) error {
	invisible := attackCreatureHasFlag(actor, "invisible", "pinvis", "PINVIS")
	if !viewer.PlayerID.IsZero() {
		if player, ok := world.Player(viewer.PlayerID); ok {
			if hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "pinvis", "PINVIS") {
				invisible = true
			}
			if _, err := world.UpdatePlayerTags(viewer.PlayerID, nil, []string{"hidden", "phiddn", "PHIDDN", "invisible", "pinvis", "PINVIS"}); err != nil {
				return err
			}
		}
	}
	if _, err := world.UpdateCreatureTags(actor.ID, nil, []string{"hidden", "phiddn", "PHIDDN", "invisible", "pinvis", "PINVIS"}); err != nil {
		return err
	}
	if updater, ok := world.(circleStatUpdater); ok {
		for _, key := range []string{"PHIDDN", "PINVIS"} {
			if actor.Stats[key] != 0 {
				if err := updater.SetCreatureStat(actor.ID, key, 0); err != nil {
					return err
				}
			}
		}
	}
	if !invisible {
		return nil
	}
	actorName := attackCreatureName(actor)
	ctx.WriteString("당신의 모습이 서서히 드러납니다.\n")
	_ = roomBroadcast(ctx, roomID, "\n"+actorName+"의 모습이 서서히 드러납니다.")
	return nil
}

func circleChance(actor model.Creature, victim model.Creature) int {
	chance := 50 + (((attackCreatureLevel(actor)+3)/4)-((attackCreatureLevel(victim)+3)/4))*10
	chance += (legacyStatBonus(creatureStat(actor, "dexterity")) - legacyStatBonus(creatureStat(victim, "dexterity"))) * 2
	if !attackCreatureIsPlayer(victim) && attackCreatureHasFlag(victim, "undead", "munded", "MUNDED") {
		chance -= 5 + ((attackCreatureLevel(victim)+3)/4)*2
	}
	chance = minInt(80, chance)
	if attackCreatureHasFlag(victim, "noCircle", "mnocir", "MNOCIR") ||
		attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
		chance = 1
	}
	return chance
}

func circleApplyBefuddle(world CircleWorld, victim kickTarget, delay int, nowUnix int64) error {
	if err := world.SetCreatureCooldown(victim.creature.ID, "befuddled", nowUnix, int64(delay)); err != nil {
		return err
	}
	if victim.hasPlayer {
		return nil
	}
	if _, err := world.UpdateCreatureTags(victim.creature.ID, []string{"befuddled", "MBEFUD"}, nil); err != nil {
		return err
	}
	return nil
}
