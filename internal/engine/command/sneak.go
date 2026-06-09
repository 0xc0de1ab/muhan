package command

import (
	"math/rand"
	"strings"
	"time"

	"muhan/internal/world/model"
)

type SneakWorld interface {
	MoveWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
}

func NewSneakHandler(world SneakWorld, roll SearchRollFunc) Handler {
	if roll == nil {
		roll = func(min int, max int) int {
			if max <= min {
				return min
			}
			return min + rand.Intn(max-min+1)
		}
	}
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := MovePlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrMoveActorRequired
		}

		viewer, currentRoom, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}

		creature, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, nil
		}

		class := creatureClass(creature)
		if class != legacyClassAssassin && class != legacyClassThief && class < legacyClassInvincible {
			ctx.WriteString("도둑과 자객만 사용할 수 있는 기술입니다.\n")
			return StatusDefault, nil
		}

		if class >= legacyClassInvincible && !backstabHasThiefOrAssassinTraining(creature) {
			ctx.WriteString("\n도둑이나 자객을 무적수련하지 않았습니다..\n")
			return StatusDefault, nil
		}

		if !attackCreatureHasFlag(creature, "hidden", "phiddn", "PHIDDN") {
			ctx.WriteString("먼저 숨어야 이 기술을 쓸 수 있습니다.\n")
			return StatusDefault, nil
		}

		if len(resolved.Args) == 0 {
			ctx.WriteString("어디로 몰래 가시려구요?\n")
			return StatusDefault, nil
		}

		exitName, exit, userMessage, err := selectMoveExit(world, viewer, currentRoom, resolved)
		if err != nil {
			return StatusDefault, err
		}
		if userMessage != "" {
			ctx.WriteString(userMessage)
			return StatusDefault, nil
		}

		if stop, err := handleMoveFall(ctx, world, viewer, currentRoom, exit, resolved.Spec.Handler); err != nil {
			return StatusDefault, err
		} else if stop {
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(creature.ID, "attack", now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		chance := sneakChance(creature)
		success := roll(1, 100) <= chance

		if !success {
			ctx.WriteString("당신은 은신술을 사용하는데 실패하였습니다.\n")
			if _, creature, err = clearCommandActorHidden(world, model.Player{ID: playerID}, creature); err != nil {
				return StatusDefault, err
			}

			// C only lets an MBLOCK monster stop failed sneak if that monster is
			// already an enemy of the player.
			for _, cid := range currentRoom.CreatureIDs {
				monster, ok := world.Creature(cid)
				if !ok || monster.Kind != model.CreatureKindMonster {
					continue
				}
				if creatureHasAnyFlag(monster, "blocksExits", "MBLOCK", "mblock") &&
					sneakMonsterTargetsActor(world, monster.ID, playerID, creature) &&
					!attackCreatureHasFlag(creature, "invisible", "pinvis", "PINVIS") &&
					class < legacyClassSubDM {
					monsterName := attackCreatureName(monster)
					ctx.WriteString(monsterName + "가 당신의 길을 가로막습니다.\n")
					return StatusDefault, nil
				}
			}
		}

		// Move player
		if err := recordMoveTrack(world, currentRoom, exitName); err != nil {
			return StatusDefault, err
		}
		if err := world.MovePlayer(playerID, exitName); err != nil {
			return StatusDefault, err
		}

		// Show look of new room
		viewer, room, err := CurrentRoom(world, viewer)
		if err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(RenderRoomLook(world, room, viewer))
		if err := checkMoveRoomTrap(ctx, world, viewer, room); err != nil {
			return StatusDefault, err
		}
		return StatusDefault, nil
	}
}

func sneakChance(creature model.Creature) int {
	level := creature.Level
	if statsLevel := creatureStat(creature, "level"); statsLevel > level {
		level = statsLevel
	}
	dexBonus := legacyStatBonus(creatureStat(creature, "dexterity"))
	chance := minInt(85, 5+6*((level+3)/4)+3*dexBonus)
	if attackCreatureHasFlag(creature, "blind", "pblind", "PBLIND") {
		chance = minInt(20, chance)
	}
	return chance
}

type sneakActorNameWorld interface {
	Player(model.PlayerID) (model.Player, bool)
}

func sneakMonsterTargetsActor(world sneakActorNameWorld, monsterID model.CreatureID, playerID model.PlayerID, actor model.Creature) bool {
	enemyWorld, ok := world.(interface {
		CreatureEnemies(model.CreatureID) ([]string, error)
	})
	if !ok {
		return false
	}
	enemies, err := enemyWorld.CreatureEnemies(monsterID)
	if err != nil || len(enemies) == 0 {
		return false
	}
	names := sneakActorEnemyNames(world, playerID, actor)
	for _, enemy := range enemies {
		if _, ok := names[strings.TrimSpace(enemy)]; ok {
			return true
		}
	}
	return false
}

func sneakActorEnemyNames(world sneakActorNameWorld, playerID model.PlayerID, actor model.Creature) map[string]struct{} {
	names := map[string]struct{}{}
	if player, ok := world.Player(playerID); ok {
		if name := strings.TrimSpace(player.DisplayName); name != "" {
			names[name] = struct{}{}
		}
	}
	if name := strings.TrimSpace(actor.DisplayName); name != "" {
		names[name] = struct{}{}
	}
	return names
}
