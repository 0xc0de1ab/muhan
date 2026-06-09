package game

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"muhan/internal/world/model"
)

// UpdateRandomWorld defines the world methods needed for random monster spawning.
type UpdateRandomWorld interface {
	AllRoomIDs() []model.RoomID
	Room(model.RoomID) (model.Room, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	CreaturePrototype(model.CreatureID) (model.Creature, bool)
	SpawnCreature(protoID model.CreatureID, roomID model.RoomID, carryItems bool) (model.CreatureID, error)
}

// UpdateRandomSpawns checks all player-occupied rooms and rolls against traffic to spawn random monsters.
// Matches C update_random exactly in:
// - unique player rooms (via AllRoomIDs + count>0 filter)
// - mrand(1,100) > traffic skip
// - random[0..9] pick from room.Properties["random"] CSV (C array)
// - num = 1 or mrand(1,playerCount) if RPLWAN/"playerWander", or numWander
// - carry item load (3/2/1  with 10% value rand, gold rand/10) -- delegated to SpawnCreature
// - init of LT_ATTCK/MSCAV/MWAND + attack interval based on dex (done inside SpawnCreature)
// Spawned monsters go on "active" list implicitly (processed in UpdateActive if players present).
// Korean broadcast etc handled by spawn if needed.
func UpdateRandomSpawns(world UpdateRandomWorld, t int64) {
	if world == nil {
		return
	}

	for _, roomID := range world.AllRoomIDs() {
		room, ok := world.Room(roomID)
		if !ok {
			continue
		}

		if countPlayersInRoom(world, room) == 0 {
			continue
		}

		trafficStr := room.Properties["traffic"]
		traffic, err := strconv.Atoi(trafficStr)
		if err != nil || traffic <= 0 {
			continue
		}

		roll := rand.Intn(100) + 1
		if roll > traffic {
			continue
		}

		randomIDs := updateRandomRoomRandomValues(room)
		n := rand.Intn(10)
		monsterID := randomIDs[n]
		if monsterID == 0 {
			continue
		}

		protoID := model.CreatureID(fmt.Sprintf("creature:m%02d:%d", monsterID/100, monsterID%100))
		proto, ok := world.CreaturePrototype(protoID)
		if !ok {
			continue
		}

		num := 1
		if hasRoomTag(room, "RPLWAN", "playerWander", "groupWander") {
			numPlayers := countPlayersInRoom(world, room)
			if numPlayers > 1 {
				num = rand.Intn(numPlayers) + 1
			} else {
				num = 1
			}
		} else if numWander, ok := proto.Stats["numWander"]; ok && numWander > 1 {
			num = rand.Intn(numWander) + 1
		} else {
			num = 1
		}

		for l := 0; l < num; l++ {
			_, _ = world.SpawnCreature(protoID, room.ID, true)
		}
	}
}

func updateRandomRoomRandomValues(room model.Room) [10]int {
	var out [10]int
	if room.Properties == nil {
		return out
	}
	if random := strings.TrimSpace(room.Properties["random"]); random != "" {
		parts := strings.Split(random, ",")
		for i := 0; i < len(parts) && i < len(out); i++ {
			if value, err := strconv.Atoi(strings.TrimSpace(parts[i])); err == nil {
				out[i] = value
			}
		}
	}
	for i := range out {
		for _, key := range []string{fmt.Sprintf("random[%d]", i), fmt.Sprintf("random%d", i)} {
			if value, err := strconv.Atoi(strings.TrimSpace(room.Properties[key])); err == nil {
				out[i] = value
			}
		}
	}
	return out
}

func countPlayersInRoom(world UpdateRandomWorld, room model.Room) int {
	if len(room.PlayerIDs) > 0 {
		return len(room.PlayerIDs)
	}
	count := 0
	for _, crtID := range room.CreatureIDs {
		if crt, ok := world.Creature(crtID); ok {
			if crt.Kind == model.CreatureKindPlayer || !crt.PlayerID.IsZero() {
				count++
			}
		}
	}
	return count
}

func hasRoomTag(room model.Room, tags ...string) bool {
	targets := normalizedFlagSet(tags...)
	for _, t := range room.Metadata.Tags {
		if _, ok := targets[normalizeFlagName(t)]; ok {
			return true
		}
	}
	for key, value := range room.Properties {
		if _, ok := targets[normalizeFlagName(key)]; ok && propertyFlagEnabled(value) {
			return true
		}
		for _, token := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ';' || r == '|' || r == ' '
		}) {
			if _, ok := targets[normalizeFlagName(token)]; ok {
				return true
			}
		}
	}
	return false
}
