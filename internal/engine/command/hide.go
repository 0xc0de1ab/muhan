package command

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type HideWorld interface {
	LookWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	UpdateObjectTags(model.ObjectInstanceID, []string, []string) (model.ObjectInstance, error)
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
}

func NewHideHandler(world HideWorld, roll SearchRollFunc) Handler {
	if roll == nil {
		roll = func(min int, max int) int {
			if max <= min {
				return min
			}
			return min + rand.Intn(max-min+1)
		}
	}
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		creature, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("hide: creature %q not found", viewer.CreatureID)
		}
		if remaining, used, err := world.UseCreatureCooldown(creature.ID, "hide", time.Now().Unix(), hideCooldownInterval(creature)); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		target := getArg(resolved, 0)
		if target == "" {
			return hideSelf(ctx, world, room.ID, viewer.PlayerID, creature, roll)
		}
		return hideRoomObject(ctx, world, creature, room, target, firstGetOrdinal(resolved), roll)
	}
}

func hideCooldownInterval(creature model.Creature) int64 {
	switch creatureStat(creature, "class") {
	case model.ClassThief, model.ClassAssassin, model.ClassRanger:
		return 5
	default:
		return 15
	}
}

func hideSelf(ctx *Context, world HideWorld, roomID model.RoomID, playerID model.PlayerID, creature model.Creature, roll SearchRollFunc) (Status, error) {
	actorName := attackCreatureName(creature)
	ctx.WriteString("당신은 애써 숨어보려고 합니다.")
	if roll(1, 100) <= hideSelfChance(creature) {
		if _, err := world.UpdateCreatureTags(creature.ID, []string{"hidden"}, nil); err != nil {
			return StatusDefault, err
		}
		if setter, ok := world.(interface {
			SetCreatureStat(model.CreatureID, string, int) error
		}); ok {
			if err := setter.SetCreatureStat(creature.ID, "PHIDDN", 1); err != nil {
				return StatusDefault, err
			}
		}
		if !playerID.IsZero() {
			if _, err := world.UpdatePlayerTags(playerID, []string{"hidden"}, nil); err != nil {
				return StatusDefault, err
			}
		}
		ctx.WriteString("\n당신은 성공적으로 숨었습니다.")
		_ = roomBroadcast(ctx, roomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 그림자 사이로 숨었습니다.")
		return StatusDefault, nil
	}
	_ = roomBroadcast(ctx, roomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 애써 숨어보려고 합니다.")
	if _, _, err := clearCommandActorHidden(world, model.Player{ID: playerID}, creature); err != nil {
		return StatusDefault, err
	}
	return StatusDefault, nil
}

func hideRoomObject(
	ctx *Context,
	world HideWorld,
	creature model.Creature,
	room model.Room,
	target string,
	ordinal int64,
	roll SearchRollFunc,
) (Status, error) {
	object, ok := findHideRoomObject(world, creature, room, target, ordinal)
	if !ok {
		ctx.WriteString("그런것은 여기 없어요.")
		return StatusDefault, nil
	}
	if objectHasAnyTag(world, object, "noTake", "notTake", "onotak", "notak") ||
		objectHasAnyPropertyFlag(world, object, "noTake", "notTake", "onotak", "ONOTAK") {
		ctx.WriteString("당신은 그것을 숨길 수 없습니다.")
		return StatusDefault, nil
	}

	actorName := attackCreatureName(creature)
	objectName := objectDisplayName(world, object)
	ctx.WriteString("당신은 그것을 숨겨보려고 합니다.")
	_ = roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+objectName+krtext.Particle(objectName, '3')+" 숨겨보려고 합니다.")
	if roll(1, 100) <= hideObjectChance(creature) {
		if _, err := world.UpdateObjectTags(object.ID, []string{"hidden"}, nil); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("\n당신은 성공적으로 숨겼습니다.")
		_ = roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+objectName+krtext.Particle(objectName, '3')+" 어딘가 숨깁니다.")
		return StatusDefault, nil
	}
	if _, err := world.UpdateObjectTags(object.ID, nil, []string{"hidden", "ohiddn"}); err != nil {
		return StatusDefault, err
	}
	return StatusDefault, nil
}

func findHideRoomObject(
	world HideWorld,
	creature model.Creature,
	room model.Room,
	prefix string,
	ordinal int64,
) (model.ObjectInstance, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return model.ObjectInstance{}, false
	}
	if ordinal < 1 {
		ordinal = 1
	}
	detectInvisible := viewerHasDetectInvisibleTag(creature)
	var seen int64
	for _, id := range room.Objects.ObjectIDs {
		object, ok := world.Object(id)
		if !ok || !objectLocatedInRoom(object, room.ID) || !legacyObjectPrefixMatches(world, object, prefix) {
			continue
		}
		if searchObjectInvisible(world, object) && !detectInvisible {
			continue
		}
		seen++
		if seen == ordinal {
			return object, true
		}
	}
	return model.ObjectInstance{}, false
}

func hideSelfChance(creature model.Creature) int {
	class := creatureStat(creature, "class")
	level := creature.Level
	if statsLevel := creatureStat(creature, "level"); statsLevel > level {
		level = statsLevel
	}
	dexBonus := legacyStatBonus(creatureStat(creature, "dexterity"))
	var chance int
	switch {
	case class == model.ClassThief || class == model.ClassAssassin || class >= model.ClassCaretaker:
		chance = minInt(90, 5+6*((level+3)/4)+3*dexBonus)
	case class == model.ClassRanger:
		chance = 5 + 10*((level+3)/4) + 3*dexBonus
	default:
		chance = minInt(90, 5+2*((level+3)/4)+3*dexBonus)
	}
	if creatureHasAnyFlag(creature, "blind", "pblind") {
		chance = minInt(chance, 20)
	}
	return chance
}

func hideObjectChance(creature model.Creature) int {
	class := creatureStat(creature, "class")
	level := creature.Level
	if statsLevel := creatureStat(creature, "level"); statsLevel > level {
		level = statsLevel
	}
	dexBonus := legacyStatBonus(creatureStat(creature, "dexterity"))
	switch {
	case class == model.ClassThief || class == model.ClassAssassin:
		return minInt(90, 10+5*((level+3)/4)+5*dexBonus)
	case class == model.ClassRanger:
		return 5 + 9*((level+3)/4) + 3*dexBonus
	default:
		return minInt(90, 5+3*((level+3)/4)+3*dexBonus)
	}
}
