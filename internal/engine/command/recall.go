package command

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const defaultReturnRoomID = model.RoomID("room:00001")
const magicRecallSelfRoomID = model.RoomID("room:01001")
const legacyReturnFamilyRoomBase = 3300

type ReturnWorld interface {
	StatusWorld
	MovePlayerToRoom(model.PlayerID, model.RoomID) error
}

func NewReturnSquareHandler(world ReturnWorld) Handler {
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}

		roomID := returnCurrentRoomID(player, creature)
		familyReturn := creatureHasAnyFlag(creature, "PFRTUN", "familyReturn")
		if returnRoomNumber(roomID) == 10 {
			ctx.WriteString("사용자 감옥의 지킴이 [김 건모]가 당신을 붙잡으며 말합니다.\n\n\"당신은 잘못된 행동의 결과로 갇혀 있습니다. 참고 기다리십시요.!!\"\n")
			return StatusDefault, nil
		}
		if returnActorInCombat(world, roomID, player, creature) {
			ctx.WriteString("당신은 싸우고 있는 중입니다!!")
			return StatusDefault, nil
		}
		if returnRoomNumber(roomID) == 1 && !familyReturn {
			ctx.WriteString("당신은 이미 생명의 나무에 와 있습니다!")
			return StatusDefault, nil
		}
		if checkGroupState(ctx, string(player.ID)) {
			ctx.WriteString("먼저 그룹에서 나오세요.")
			return StatusDefault, nil
		}

		if creatureLevel(creature) > 20 && creatureClass(creature) < model.ClassInvincible {
			ctx.WriteString("당신이 귀환하려하자 흑암의 세력이 당신의 도력을 뺏습니다.\n")
			if setter, ok := world.(interface {
				SetCreatureStat(model.CreatureID, string, int) error
			}); ok {
				if err := setter.SetCreatureStat(creature.ID, "mpCurrent", 0); err != nil {
					return StatusDefault, err
				}
			}
		}

		targetRoomID := defaultReturnRoomID
		if familyReturn {
			familyID, _ := moveCreatureStatOrPropertyInt(creature, "dailyExpndMax", "legacyDailyExpndMax", "daily_expnd_max", "familyID", "family_id")
			targetRoomID = returnRoomIDForNumber(legacyReturnFamilyRoomBase + familyID)
		}

		name := returnActorName(player, creature)
		ctx.WriteString("당신이 \"귀환!\"이라고 외치자 이상한 힘에 의해 어딘가로 빨려들어갑니다.")
		if !creatureHasAnyFlag(creature, "PDMINV", "dmInvisible") {
			_ = roomBroadcast(ctx, roomID, "\n"+name+"님이 갑자기 사라집니다!")
		}

		if err := world.MovePlayerToRoom(player.ID, targetRoomID); err != nil {
			return StatusDefault, err
		}
		if !creatureHasAnyFlag(creature, "PDMINV", "dmInvisible") {
			_ = roomBroadcast(ctx, targetRoomID, "\n"+name+"님이 갑자기 자욱한 연기와 함께 나타났습니다!")
		}
		return StatusDefault, nil
	}
}

func returnCurrentRoomID(player model.Player, creature model.Creature) model.RoomID {
	if !creature.RoomID.IsZero() {
		return creature.RoomID
	}
	return player.RoomID
}

func returnRoomNumber(roomID model.RoomID) int {
	raw := strings.TrimSpace(string(roomID))
	raw = strings.TrimPrefix(raw, "room:")
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return n
}

func returnRoomIDForNumber(number int) model.RoomID {
	return model.RoomID(fmt.Sprintf("room:%05d", number))
}

func returnActorName(player model.Player, creature model.Creature) string {
	if name := strings.TrimSpace(creature.DisplayName); name != "" {
		return name
	}
	if name := strings.TrimSpace(player.DisplayName); name != "" {
		return name
	}
	if !player.ID.IsZero() {
		return strings.TrimPrefix(string(player.ID), "player:")
	}
	return strings.TrimPrefix(string(creature.ID), "creature:")
}

func returnActorInCombat(world ReturnWorld, roomID model.RoomID, player model.Player, actor model.Creature) bool {
	enemyWorld, ok := world.(interface {
		CreatureEnemies(model.CreatureID) ([]string, error)
	})
	if !ok || roomID.IsZero() {
		return false
	}
	room, ok := world.Room(roomID)
	if !ok {
		return false
	}
	actorNames := returnActorEnemyNames(player, actor)
	for _, creatureID := range room.CreatureIDs {
		if creatureID == actor.ID {
			continue
		}
		creature, ok := world.Creature(creatureID)
		if !ok || creature.Kind == model.CreatureKindPlayer || !creature.PlayerID.IsZero() {
			continue
		}
		enemies, err := enemyWorld.CreatureEnemies(creature.ID)
		if err != nil {
			continue
		}
		for _, enemy := range enemies {
			if _, ok := actorNames[strings.TrimSpace(enemy)]; ok {
				return true
			}
		}
	}
	return false
}

func returnActorEnemyNames(player model.Player, creature model.Creature) map[string]struct{} {
	names := map[string]struct{}{}
	for _, name := range []string{
		creature.DisplayName,
		player.DisplayName,
	} {
		name = strings.TrimSpace(name)
		if name != "" {
			names[name] = struct{}{}
		}
	}
	return names
}
