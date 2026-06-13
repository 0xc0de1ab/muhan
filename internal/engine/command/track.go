package command

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type TrackWorld interface {
	LookWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
}

func NewTrackHandler(world TrackWorld) Handler {
	return newTrackHandler(world, nil)
}

func newTrackHandler(world TrackWorld, roll SearchRollFunc) Handler {
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
			return StatusDefault, fmt.Errorf("track: creature %q not found", viewer.CreatureID)
		}
		if !trackActorAllowed(actor) {
			ctx.WriteString("포졸만 쓸수 있는 명령입니다.")
			return StatusDefault, nil
		}
		if err := trackClearActorHidden(world, viewer.PlayerID, actor); err != nil {
			return StatusDefault, err
		}
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, "track", time.Now().Unix(), trackCooldownInterval(actor)); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}
		if creatureHasAnyFlag(actor, "blind", "pblind", "PBLIND") {
			ctx.WriteString("당신은 눈이 멀어 있습니다. 도저히 추적을 할 수 없습니다.")
			return StatusDefault, nil
		}
		if roll(1, 100) > trackChance(actor) {
			ctx.WriteString("추적 실패!")
			return StatusDefault, nil
		}

		if direction := legacyRoomTrackDirection(room); direction != "" {
			ctx.WriteString(renderLegacyRoomTrackDirection(direction))
			_ = roomBroadcast(ctx, room.ID, "\n"+attackCreatureName(actor)+"이 적이 지나간 흔적을 찾았습니다.")
			return StatusDefault, nil
		}
		ctx.WriteString("아무런 흔적이 남아있지 않습니다.")
		return StatusDefault, nil
	}
}

func trackActorAllowed(creature model.Creature) bool {
	class := creatureClass(creature)
	return class == model.ClassRanger || class >= model.ClassInvincible
}

func trackClearActorHidden(world TrackWorld, playerID model.PlayerID, creature model.Creature) error {
	if _, err := world.UpdateCreatureTags(creature.ID, nil, []string{"hidden", "phiddn", "PHIDDN"}); err != nil {
		return err
	}
	if !playerID.IsZero() {
		if _, err := world.UpdatePlayerTags(playerID, nil, []string{"hidden", "phiddn", "PHIDDN"}); err != nil {
			return err
		}
	}
	if creatureStat(creature, "PHIDDN") != 0 {
		if err := world.SetCreatureStat(creature.ID, "PHIDDN", 0); err != nil {
			return err
		}
	}
	return nil
}

func trackCooldownInterval(creature model.Creature) int64 {
	return int64(5 - legacyStatBonus(creatureStat(creature, "dexterity")))
}

func trackChance(creature model.Creature) int {
	return 25 + (legacyStatBonus(creatureStat(creature, "dexterity"))+((creatureLevel(creature)+3)/4))*5
}

func legacyRoomTrackDirection(room model.Room) string {
	if room.Properties == nil {
		return ""
	}
	for _, key := range []string{"track", "legacyTrack", "roomTrack"} {
		if direction := trackDirectionName(room.Properties[key]); direction != "" {
			return direction
		}
	}
	return ""
}

func renderLegacyRoomTrackDirection(direction string) string {
	direction = trackDirectionName(direction)
	if direction == "" {
		return "아무런 흔적이 남아있지 않습니다."
	}
	if strings.HasSuffix(direction, "쪽") {
		return direction + "으로 흔적이 나 있습니다."
	}
	return direction + "쪽으로 흔적이 나 있습니다."
}

func trackDirectionName(direction string) string {
	direction = strings.TrimSpace(cleanDisplayText(direction))
	switch strings.ToLower(direction) {
	case "n", "north", "북":
		return "북쪽"
	case "s", "south", "남":
		return "남쪽"
	case "e", "east", "동":
		return "동쪽"
	case "w", "west", "서":
		return "서쪽"
	case "u", "up", "위":
		return "위"
	case "d", "down", "밑", "아래":
		return "밑"
	case "out", "밖", "나가":
		return "밖"
	default:
		return direction
	}
}
