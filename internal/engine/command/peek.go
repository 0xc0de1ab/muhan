package command

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	peekClassBulsa = model.ClassBulsa
	peekClassDM    = model.ClassDM
)

type PeekRollFunc func(min int, max int) int

type PeekWorld interface {
	LookWorld
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
}

func NewPeekHandler(world PeekWorld, roll PeekRollFunc) Handler {
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
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("peek: actor creature %q not found", viewer.CreatureID)
		}

		target, ordinal := lookTarget(resolved)
		if target == "" {
			ctx.WriteString("누구의 소지품을 보려구요?")
			return StatusDefault, nil
		}
		class := creatureStat(actor, "class")
		if class != model.ClassThief && class < model.ClassInvincible {
			ctx.WriteString("당신 직업으로는 다른사람의 소지품을 볼 수 없습니다.")
			return StatusDefault, nil
		}
		if creatureHasAnyFlag(actor, "blind", "pblind") {
			ctx.WriteString("당신은 눈이 멀어 있습니다!")
			return StatusDefault, nil
		}

		victim, ok := findPeekTargetCreature(world, room, viewer, target, ordinal)
		if !ok {
			ctx.WriteString("그런 사람 없어요!")
			return StatusDefault, nil
		}
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, "peek", time.Now().Unix(), 5); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}
		if (creatureHasAnyFlag(victim, "noSteal", "munstl") ||
			creatureHasAnyFlag(victim, "tradeItems", "mtrade") ||
			creatureHasAnyFlag(victim, "purchaseItems", "mpurit")) && class < peekClassDM {
			ctx.WriteString("당신은 다른사람의 소지품을 볼 수 없습니다.\n다른사람이 당신보고 도둑이라고 생각할 것입니다.")
			return StatusDefault, nil
		}

		if roll(1, 100) > peekChance(actor, victim) {
			ctx.WriteString("실패하였습니다!")
			return StatusDefault, nil
		}

		if roll(1, 100) > peekCaughtChance(actor) && class < model.ClassCaretaker {
			actorName := attackCreatureName(actor)
			victimName := attackCreatureName(victim)
			_ = sendToPlayer(ctx, victim.PlayerID, actorName+"님이 당신의 소지품을 슬쩍 엿봅니다.")
			_ = broadcastRom2(ctx, world, room.ID, viewer.PlayerID, victim.PlayerID, "\n"+actorName+"이 "+victimName+"의 소지품을 슬쩍 엿봅니다.")
		}

		names := peekInventoryNames(world, actor, victim)
		pronoun := peekSubjectPronoun(victim)
		if len(names) == 0 {
			ctx.WriteString(pronoun + "는 아무것도 들고 있지 않습니다.")
			return StatusDefault, nil
		}
		ctx.WriteString(pronoun + "의 소지품: " + strings.Join(names, ", "))
		return StatusDefault, nil
	}
}

func findPeekTargetCreature(
	world LookWorld,
	room model.Room,
	viewer LookViewer,
	prefix string,
	ordinal int64,
) (model.Creature, bool) {
	if creature, ok := findAttackCreatureTarget(world, room, viewer, prefix, ordinal); ok {
		return creature, true
	}
	player, ok := findAttackPlayerTarget(world, room, viewer, prefix, ordinal)
	if !ok || player.CreatureID.IsZero() {
		return model.Creature{}, false
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok || creature.RoomID != room.ID {
		return model.Creature{}, false
	}
	return creature, true
}

func peekChance(actor model.Creature, target model.Creature) int {
	class := creatureStat(actor, "class")
	if class >= peekClassBulsa {
		return 100
	}
	if class == model.ClassCaretaker {
		return 90
	}
	chance := 25 + ((creatureLevel(actor)+3)/4)*10 - ((creatureLevel(target)+3)/4)*5
	if chance < 0 {
		return 0
	}
	return chance
}

func peekCaughtChance(actor model.Creature) int {
	return minInt(90, 15+((creatureLevel(actor)+3)/4)*5)
}

func peekSubjectPronoun(creature model.Creature) string {
	if creatureHasAnyFlag(creature, "PMALES", "MMALES", "male", "pMale") {
		return "그"
	}
	return "그녀"
}

func creatureLevel(creature model.Creature) int {
	if level := creatureStat(creature, "level"); level > 0 {
		return level
	}
	return creature.Level
}

func peekInventoryNames(world LookWorld, viewer model.Creature, target model.Creature) []string {
	names := make([]string, 0, len(target.Inventory.ObjectIDs))
	detectInvisible := viewerHasDetectInvisibleTag(viewer)
	for _, objectID := range target.Inventory.ObjectIDs {
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInCreatureInventory(object, target.ID) {
			continue
		}
		if !peekObjectVisible(world, object, detectInvisible) {
			continue
		}
		name := objectDisplayName(world, object)
		if name == "" {
			continue
		}
		if len(names) > 0 && names[len(names)-1] == name {
			continue
		}
		names = append(names, name)
	}
	return names
}

func peekObjectVisible(world InventoryWorld, object model.ObjectInstance, detectInvisible bool) bool {
	if objectHasAnyTag(world, object, "hidden", "ohiddn", "scenery", "scene", "oscene") ||
		objectHasAnyPropertyFlag(world, object, "hidden", "ohiddn", "OHIDDN", "scenery", "scene", "oscene", "OSCENE") {
		return false
	}
	if searchObjectInvisible(world, object) && !detectInvisible {
		return false
	}
	return true
}
