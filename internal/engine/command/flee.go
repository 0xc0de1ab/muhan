package command

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type FleeWorld interface {
	MoveWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	CreatureEnemies(model.CreatureID) ([]string, error)
}

func NewFleeHandler(world FleeWorld, roll SearchRollFunc) Handler {
	if roll == nil {
		roll = func(min int, max int) int {
			if max <= min {
				return min
			}
			return min + rand.Intn(max-min+1)
		}
	}
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		playerID := MovePlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrMoveActorRequired
		}
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		creature, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("flee: creature %q not found", viewer.CreatureID)
		}
		if !creatureHasAnyFlag(creature, "PFEARS", "fear", "fearful") {
			if remaining, ready, err := fleeAttackOrSpellReady(world, creature.ID, time.Now().Unix()); err != nil {
				return StatusDefault, err
			} else if !ready {
				ctx.WriteString(renderPleaseWait(remaining))
				return StatusDefault, nil
			}
		}
		if !roomHasVisibleFleeThreat(world, room, viewer, creature) {
			ctx.WriteString("누구에게서 도망가시려구요?")
			return StatusDefault, nil
		}

		exit, ok := chooseFleeExit(world, room, viewer, creature, roll)
		if !ok {
			ctx.WriteString("\n당신은 겁에 질려 다리가 떨어지지 않습니다!")
			return StatusDefault, nil
		}
		if _, err := world.UpdateCreatureTags(creature.ID, nil, []string{"hidden", "phiddn", "PHIDDN"}); err != nil {
			return StatusDefault, err
		}
		if creatureStat(creature, "PHIDDN") != 0 {
			if err := world.SetCreatureStat(creature.ID, "PHIDDN", 0); err != nil {
				return StatusDefault, err
			}
		}
		if !viewer.PlayerID.IsZero() {
			if _, err := world.UpdatePlayerTags(viewer.PlayerID, nil, []string{"hidden", "phiddn", "PHIDDN"}); err != nil {
				return StatusDefault, err
			}
		}
		ctx.WriteString("\n당신은 줄행랑을 칩니다.")
		if creatureHasAnyFlag(creature, "PFEARS", "fear", "fearful") {
			if viewer.TextOptions.ANSI {
				ctx.WriteString("\x1b[0;31m\n당신은 겁에 질린듯 얼굴이 창백하게 변해 도망을 갑니다!\x1b[0;37m")
			} else {
				ctx.WriteString("\n당신은 겁에 질린듯 얼굴이 창백하게 변해 도망을 갑니다!")
			}
		}
		actorName := attackCreatureName(creature)
		if err := recordMoveTrack(world, room, exit.Name); err != nil {
			return StatusDefault, err
		}
		_ = roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+exit.Name+"쪽으로 도망을 갑니다.")
		if err := applyFleeExperiencePenalty(ctx, world, creature); err != nil {
			return StatusDefault, err
		}
		if message, destinationID := blockedFleeDestinationMessage(world, viewer, exit); message != "" {
			ctx.WriteString(message)
			_ = roomBroadcast(ctx, destinationID, "\n"+actorName+krtext.Particle(actorName, '1')+" 도착하였습니다.")
			return StatusDefault, nil
		}
		if err := world.MovePlayer(playerID, exit.Name); err != nil {
			return StatusDefault, err
		}
		viewer, destination, err := CurrentRoom(world, viewer)
		if err != nil {
			return StatusDefault, fmt.Errorf("flee: render destination: %w", err)
		}
		ctx.WriteString("\n")
		ctx.WriteString(RenderRoomLook(world, destination, viewer))
		return StatusDefault, nil
	}
}

func fleeAttackOrSpellReady(world FleeWorld, creatureID model.CreatureID, now int64) (int64, bool, error) {
	attackRemaining, attackReady, err := world.UseCreatureCooldown(creatureID, "attack", now, 0)
	if err != nil {
		return 0, false, err
	}
	spellRemaining, spellReady, err := world.UseCreatureCooldown(creatureID, "spell", now, 0)
	if err != nil {
		return 0, false, err
	}
	if attackReady && spellReady {
		return 0, true, nil
	}
	if spellRemaining > attackRemaining {
		return spellRemaining, false, nil
	}
	return attackRemaining, false, nil
}

func roomHasVisibleFleeThreat(world FleeWorld, room model.Room, viewer LookViewer, actor model.Creature) bool {
	actorName := strings.TrimSpace(attackCreatureName(actor))
	if actorName == "" {
		return false
	}
	for _, id := range room.CreatureIDs {
		if id.IsZero() || id == viewer.CreatureID {
			continue
		}
		creature, ok := world.Creature(id)
		if !ok || creature.RoomID != room.ID || attackCreatureIsPlayer(creature) || creatureHPDead(creature) {
			continue
		}
		if !creatureVisibleInRoomLook(creature, viewer, viewerDetectsInvisible(world, viewer)) {
			continue
		}
		enemies, err := world.CreatureEnemies(creature.ID)
		if err != nil {
			continue
		}
		for _, enemy := range enemies {
			if strings.TrimSpace(enemy) == actorName {
				return true
			}
		}
	}
	return false
}

func chooseFleeExit(world MoveWorld, room model.Room, viewer LookViewer, creature model.Creature, roll SearchRollFunc) (model.Exit, bool) {
	chance := 65 + legacyStatBonus(creatureStat(creature, "dexterity"))*5
	for _, exit := range room.Exits {
		if !fleeExitVisible(world, room, viewer, exit) {
			continue
		}
		if roll(1, 100) < chance {
			return exit, true
		}
	}
	return model.Exit{}, false
}

func fleeExitVisible(world MoveWorld, room model.Room, viewer LookViewer, exit model.Exit) bool {
	if !moveExitSelectable(exit, viewerDetectsInvisible(world, viewer)) {
		return false
	}
	if exitHasAnyFlag(exit, "closed", "xclosd", "xclosed", "secret", "xsecrt", "xsecret", "noSee", "xnosee") {
		return false
	}
	if exitHasAnyFlag(exit, "invisible", "xinvis") {
		creature, ok := world.Creature(viewer.CreatureID)
		if !ok || !viewerHasDetectInvisibleTag(creature) {
			return false
		}
	}
	if blockedMoveExitMessage(world, viewer, room, exit, "move") != "" {
		return false
	}
	if _, ok := world.Room(exit.ToRoomID); !ok {
		return false
	}
	return true
}

func blockedFleeDestinationMessage(world MoveWorld, viewer LookViewer, exit model.Exit) (string, model.RoomID) {
	destination, ok := world.Room(exit.ToRoomID)
	if !ok {
		return "", ""
	}
	level := moveViewerLevel(world, viewer)
	if minLevel, ok := roomMinLevel(destination); ok && level < minLevel {
		return "어떤 힘에 의해 다시 되돌아 왔습니다.", destination.ID
	}
	if maxLevel, ok := roomMaxLevel(destination); ok && level > maxLevel {
		return "어떤 힘에 의해 다시 되돌아 왔습니다.", destination.ID
	}
	if playerLimit := roomPlayerLimit(destination); playerLimit > 0 && moveVisiblePlayerCount(world, destination) >= playerLimit {
		return "도망갈려는 방의 정원이 가득 찼습니다!", destination.ID
	}
	return "", destination.ID
}

func applyFleeExperiencePenalty(ctx *Context, world FleeWorld, creature model.Creature) error {
	class := creatureStat(creature, "class")
	level := creature.Level
	if statsLevel := creatureStat(creature, "level"); statsLevel > level {
		level = statsLevel
	}
	if class != model.ClassPaladin || level <= 20 {
		return nil
	}
	loss := ((level + 3) / 4) * 10
	experience := creatureStat(creature, "experience")
	if loss > experience {
		loss = experience
	}
	if loss < 0 {
		loss = 0
	}
	if err := world.SetCreatureStat(creature.ID, "experience", experience-loss); err != nil {
		return err
	}
	ctx.WriteString(fmt.Sprintf("당신은 도망을 간 댓가로 %d 만큼의 경험치를 잃었습니다.", loss))
	return nil
}
