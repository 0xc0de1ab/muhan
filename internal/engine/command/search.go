package command

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

var legacyBonus = [...]int{
	-4, -4, -4, -3, -3, -2, -2, -1, -1, -1,
	0, 0, 0, 0, 1, 1, 1, 2, 2, 2,
	3, 3, 3, 3, 4, 4, 4, 4, 4, 5,
	5, 5, 5, 5, 5, 6, 6, 6, 6, 6,
	6, 6, 6, 6, 7, 7, 7, 7, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 8, 8, 8,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
	9, 9, 9, 9, 9, 9, 9, 9, 9, 9,
	9, 9, 9, 9, 9, 9,
}

type SearchRollFunc func(min int, max int) int

type SearchWorld interface {
	LookWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
}

func NewSearchHandler(world SearchWorld, roll SearchRollFunc) Handler {
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
			return StatusDefault, fmt.Errorf("search: creature %q not found", viewer.CreatureID)
		}
		interval := int64(7)
		if creatureStat(creature, "class") == legacyClassRanger {
			interval = 3
		}
		if remaining, used, err := world.UseCreatureCooldown(creature.ID, "search", time.Now().Unix(), interval); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}
		if _, creature, err = clearCommandActorHidden(world, model.Player{ID: viewer.PlayerID}, creature); err != nil {
			return StatusDefault, err
		}

		chance := searchChance(creature)
		detectInvisible := viewerHasDetectInvisibleTag(creature)
		found := 0
		actorName := attackCreatureName(creature)
		for _, exit := range room.Exits {
			if !exitHasAnyFlag(exit, "secret", "xsecrt", "xsecret") {
				continue
			}
			if exitHasAnyFlag(exit, "noSee", "xnosee") {
				continue
			}
			if exitHasAnyFlag(exit, "invisible", "xinvis") && !detectInvisible {
				continue
			}
			if roll(1, 100) <= chance {
				found++
				ctx.WriteString(fmt.Sprintf("\n출구를 찾았습니다: %s.", exit.Name))
			}
		}

		for _, id := range room.Objects.ObjectIDs {
			object, ok := world.Object(id)
			if !ok || !objectLocatedInRoom(object, room.ID) || !searchObjectHidden(world, object) {
				continue
			}
			if searchObjectInvisible(world, object) && !detectInvisible {
				continue
			}
			if roll(1, 100) <= chance {
				found++
				name := objectDisplayName(world, object)
				ctx.WriteString("\n당신은 " + name + krtext.Particle(name, '3') + " 찾았습니다.")
			}
		}

		for _, id := range room.PlayerIDs {
			if id.IsZero() || id == viewer.PlayerID || !viewerAllowsPlayer(viewer, id) {
				continue
			}
			player, ok := world.Player(id)
			if !ok || player.RoomID != room.ID || searchPlayerDMIInvisible(world, player) || !searchPlayerHidden(world, player) {
				continue
			}
			if searchPlayerInvisible(world, player) && !detectInvisible {
				continue
			}
			if roll(1, 100) <= chance {
				found++
				name := searchPlayerName(world, player)
				ctx.WriteString("\n당신은 숨어있는 " + name + krtext.Particle(name, '3') + " 찾아내었습니다.")
			}
		}

		for _, id := range room.CreatureIDs {
			if id.IsZero() || id == viewer.CreatureID {
				continue
			}
			target, ok := world.Creature(id)
			if !ok || target.RoomID != room.ID || attackCreatureIsPlayer(target) || creatureHPDead(target) {
				continue
			}
			if !searchCreatureHidden(target) {
				continue
			}
			if searchCreatureInvisible(target) && !detectInvisible {
				continue
			}
			if roll(1, 100) <= chance {
				found++
				name := attackCreatureName(target)
				ctx.WriteString("\n당신은 숨어있는 " + name + krtext.Particle(name, '3') + " 찾아내었습니다.")
			}
		}

		_ = roomBroadcast(ctx, room.ID, "\n"+actorName+"이 주변을 샅샅이 뒤져봅니다.")
		if found == 0 {
			ctx.WriteString("당신은 아무것도 찾지 못했습니다.\n")
		} else {
			_ = roomBroadcast(ctx, room.ID, "\n"+lookCreaturePronoun(creature)+"가 뭘 발견한것 같군요!")
		}
		return StatusDefault, nil
	}
}

func searchChance(creature model.Creature) int {
	class := creatureStat(creature, "class")
	chance := 15 + 5*legacyStatBonus(creatureStat(creature, "piety")) + ((creature.Level+3)/4)*2
	if level := creatureStat(creature, "level"); level > creature.Level {
		chance = 15 + 5*legacyStatBonus(creatureStat(creature, "piety")) + ((level+3)/4)*2
	}
	chance = minInt(chance, 90)
	if class == legacyClassRanger {
		chance = 100
	}
	if creatureHasAnyFlag(creature, "blind", "pblind") {
		chance = minInt(chance, 20)
	}
	if class >= legacyClassCaretaker {
		chance = 100
	}
	return chance
}

func creatureStat(creature model.Creature, key string) int {
	if value, ok := creatureStatValue(creature, key); ok {
		return value
	}
	return 0
}

func creatureStatValue(creature model.Creature, key string) (int, bool) {
	if creature.Stats != nil {
		if value, ok := creature.Stats[key]; ok {
			return value, true
		}
	}
	if creature.Properties != nil {
		if raw, ok := creature.Properties[key]; ok {
			value, err := strconv.Atoi(strings.TrimSpace(raw))
			return value, err == nil
		}
	}
	target := normalizeFlagName(key)
	if target == "" {
		return 0, false
	}
	for statKey, value := range creature.Stats {
		if normalizeFlagName(statKey) == target {
			return value, true
		}
	}
	for propertyKey, raw := range creature.Properties {
		if normalizeFlagName(propertyKey) == target {
			value, err := strconv.Atoi(strings.TrimSpace(raw))
			return value, err == nil
		}
	}
	return 0, false
}

func searchObjectHidden(world InventoryWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object, "hidden", "ohiddn") ||
		objectHasAnyPropertyFlag(world, object, "hidden", "ohiddn", "OHIDDN")
}

func searchObjectInvisible(world InventoryWorld, object model.ObjectInstance) bool {
	return objectHasAnyTag(world, object, "invisible", "oinvis") ||
		objectHasAnyPropertyFlag(world, object, "invisible", "oinvis", "OINVIS")
}

func searchPlayerHidden(world LookWorld, player model.Player) bool {
	if hasAnyNormalizedFlag(player.Metadata.Tags, "hidden", "phiddn") {
		return true
	}
	if player.CreatureID.IsZero() {
		return false
	}
	creature, ok := world.Creature(player.CreatureID)
	return ok && searchCreatureHidden(creature)
}

func searchPlayerInvisible(world LookWorld, player model.Player) bool {
	if hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "pinvis") {
		return true
	}
	if player.CreatureID.IsZero() {
		return false
	}
	creature, ok := world.Creature(player.CreatureID)
	return ok && searchCreatureInvisible(creature)
}

func searchPlayerDMIInvisible(world LookWorld, player model.Player) bool {
	if hasAnyNormalizedFlag(player.Metadata.Tags, "dmInvisible", "pdminv") {
		return true
	}
	if player.CreatureID.IsZero() {
		return false
	}
	creature, ok := world.Creature(player.CreatureID)
	return ok && creatureHasAnyFlag(creature, "dmInvisible", "pdminv")
}

func searchPlayerName(world LookWorld, player model.Player) string {
	if world != nil && !player.CreatureID.IsZero() {
		if creature, ok := world.Creature(player.CreatureID); ok {
			if name := cleanDisplayText(creature.DisplayName); name != "" {
				return name
			}
		}
	}
	if name := cleanDisplayText(player.DisplayName); name != "" {
		return name
	}
	return string(player.ID)
}

func searchCreatureHidden(creature model.Creature) bool {
	return creatureHasAnyFlag(creature, "hidden", "phiddn", "mhiddn")
}

func searchCreatureInvisible(creature model.Creature) bool {
	return creatureHasAnyFlag(creature, "invisible", "pinvis", "minvis")
}
