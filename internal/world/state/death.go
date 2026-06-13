package state

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const legacyMaxAutoLevel = 128

const (
	playerDeathLegacyStatNone = 0
	playerDeathLegacyStatSTR  = 1
	playerDeathLegacyStatDEX  = 2
	playerDeathLegacyStatCON  = 3
	playerDeathLegacyStatINT  = 4
	playerDeathLegacyStatPTY  = 5
)

type playerDeathClassStatBonusEntry struct {
	hpStart int
	mpStart int
	hp      int
	mp      int
	nDice   int
	sDice   int
	pDice   int
}

var playerDeathClassStatBonuses = map[int]playerDeathClassStatBonusEntry{
	model.ClassAssassin:   {55, 40, 5, 2, 1, 6, 0},
	model.ClassBarbarian:  {57, 40, 7, 1, 2, 3, 1},
	model.ClassCleric:     {54, 50, 4, 3, 1, 4, 0},
	model.ClassFighter:    {56, 50, 6, 1, 1, 5, 0},
	model.ClassMage:       {54, 50, 4, 3, 1, 3, 0},
	model.ClassPaladin:    {55, 50, 5, 2, 1, 4, 0},
	model.ClassRanger:     {56, 40, 6, 2, 2, 2, 0},
	model.ClassThief:      {55, 50, 5, 2, 2, 2, 1},
	model.ClassInvincible: {400, 250, 4, 4, 2, 4, 0},
	model.ClassCaretaker:  {50, 50, 5, 5, 5, 5, 5},
	model.ClassBulsa:      {50, 50, 5, 5, 5, 5, 5},
	model.ClassSubDM:      {50, 50, 5, 5, 5, 5, 5},
	model.ClassDM:         {50, 50, 7, 4, 5, 5, 5},
	0:                     {1, 1, 1, 1, 1, 1, 1},
}

var playerDeathLevelCycleTable = map[int][10]int{
	0:                     {0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	model.ClassAssassin:   {playerDeathLegacyStatCON, playerDeathLegacyStatPTY, playerDeathLegacyStatSTR, playerDeathLegacyStatINT, playerDeathLegacyStatDEX, playerDeathLegacyStatINT, playerDeathLegacyStatDEX, playerDeathLegacyStatPTY, playerDeathLegacyStatSTR, playerDeathLegacyStatDEX},
	model.ClassBarbarian:  {playerDeathLegacyStatINT, playerDeathLegacyStatDEX, playerDeathLegacyStatPTY, playerDeathLegacyStatCON, playerDeathLegacyStatSTR, playerDeathLegacyStatCON, playerDeathLegacyStatDEX, playerDeathLegacyStatSTR, playerDeathLegacyStatPTY, playerDeathLegacyStatSTR},
	model.ClassCleric:     {playerDeathLegacyStatSTR, playerDeathLegacyStatDEX, playerDeathLegacyStatCON, playerDeathLegacyStatPTY, playerDeathLegacyStatINT, playerDeathLegacyStatPTY, playerDeathLegacyStatINT, playerDeathLegacyStatDEX, playerDeathLegacyStatCON, playerDeathLegacyStatINT},
	model.ClassFighter:    {playerDeathLegacyStatPTY, playerDeathLegacyStatINT, playerDeathLegacyStatDEX, playerDeathLegacyStatCON, playerDeathLegacyStatSTR, playerDeathLegacyStatCON, playerDeathLegacyStatINT, playerDeathLegacyStatSTR, playerDeathLegacyStatDEX, playerDeathLegacyStatSTR},
	model.ClassMage:       {playerDeathLegacyStatSTR, playerDeathLegacyStatDEX, playerDeathLegacyStatPTY, playerDeathLegacyStatCON, playerDeathLegacyStatINT, playerDeathLegacyStatCON, playerDeathLegacyStatINT, playerDeathLegacyStatDEX, playerDeathLegacyStatPTY, playerDeathLegacyStatINT},
	model.ClassPaladin:    {playerDeathLegacyStatDEX, playerDeathLegacyStatINT, playerDeathLegacyStatCON, playerDeathLegacyStatSTR, playerDeathLegacyStatPTY, playerDeathLegacyStatSTR, playerDeathLegacyStatINT, playerDeathLegacyStatPTY, playerDeathLegacyStatCON, playerDeathLegacyStatPTY},
	model.ClassRanger:     {playerDeathLegacyStatPTY, playerDeathLegacyStatSTR, playerDeathLegacyStatINT, playerDeathLegacyStatCON, playerDeathLegacyStatDEX, playerDeathLegacyStatCON, playerDeathLegacyStatDEX, playerDeathLegacyStatSTR, playerDeathLegacyStatINT, playerDeathLegacyStatDEX},
	model.ClassThief:      {playerDeathLegacyStatINT, playerDeathLegacyStatCON, playerDeathLegacyStatPTY, playerDeathLegacyStatSTR, playerDeathLegacyStatDEX, playerDeathLegacyStatSTR, playerDeathLegacyStatCON, playerDeathLegacyStatDEX, playerDeathLegacyStatPTY, playerDeathLegacyStatDEX},
	model.ClassInvincible: {playerDeathLegacyStatSTR, playerDeathLegacyStatDEX, playerDeathLegacyStatINT, playerDeathLegacyStatCON, playerDeathLegacyStatPTY, playerDeathLegacyStatSTR, playerDeathLegacyStatDEX, playerDeathLegacyStatINT, playerDeathLegacyStatCON, playerDeathLegacyStatPTY},
	model.ClassCaretaker:  {playerDeathLegacyStatSTR, playerDeathLegacyStatDEX, playerDeathLegacyStatINT, playerDeathLegacyStatCON, playerDeathLegacyStatPTY, playerDeathLegacyStatSTR, playerDeathLegacyStatDEX, playerDeathLegacyStatINT, playerDeathLegacyStatCON, playerDeathLegacyStatPTY},
	model.ClassBulsa:      {playerDeathLegacyStatSTR, playerDeathLegacyStatDEX, playerDeathLegacyStatINT, playerDeathLegacyStatCON, playerDeathLegacyStatPTY, playerDeathLegacyStatSTR, playerDeathLegacyStatDEX, playerDeathLegacyStatINT, playerDeathLegacyStatCON, playerDeathLegacyStatPTY},
	model.ClassSubDM:      {playerDeathLegacyStatSTR, playerDeathLegacyStatDEX, playerDeathLegacyStatINT, playerDeathLegacyStatCON, playerDeathLegacyStatPTY, playerDeathLegacyStatSTR, playerDeathLegacyStatDEX, playerDeathLegacyStatINT, playerDeathLegacyStatCON, playerDeathLegacyStatPTY},
	model.ClassDM:         {playerDeathLegacyStatSTR, playerDeathLegacyStatDEX, playerDeathLegacyStatINT, playerDeathLegacyStatCON, playerDeathLegacyStatPTY, playerDeathLegacyStatSTR, playerDeathLegacyStatDEX, playerDeathLegacyStatINT, playerDeathLegacyStatCON, playerDeathLegacyStatPTY},
}

func expToLev(exp int) int {
	level := 1
	for level < legacyMaxAutoLevel && level-1 < len(legacyNeededExperience) && exp >= legacyNeededExperience[level-1] {
		level++
	}
	if level >= legacyMaxAutoLevel {
		level = (exp-legacyNeededExperience[legacyMaxAutoLevel-2])/5000000 + legacyMaxAutoLevel
	}
	if level < 1 {
		return 1
	}
	return level
}

// PlayerDeath handles all aspects of player death under the world lock.
// Outside RSUVIV survival rooms, it processes equipped ready[] objects using
// C's die() rules while leaving ordinary inventory attached to the player.
// In RSUVIV survival rooms, C skips the item-drop block, so possessions stay
// attached to the player.
// It also deducts exp based on legacy formulas, resets active titles, moves the
// player to room 1008, and calls SavePlayer.
// Thread safety is ensured via the world's mutex.
func (w *World) PlayerDeath(playerID model.PlayerID, attackerID model.CreatureID) error {
	if w == nil {
		return fmt.Errorf("player death: world state is nil")
	}

	w.lockDomains(true, true, true, true, true, true, true)
	unlocked := false
	defer func() {
		if !unlocked {
			w.unlockDomains(true, true, true, true, true, true, true)
		}
	}()

	player, ok := w.players[playerID]
	if !ok {
		return fmt.Errorf("player %s not found", playerID)
	}

	if player.CreatureID.IsZero() {
		return fmt.Errorf("player %s has no creature", playerID)
	}
	creature, ok := w.creatures[player.CreatureID]
	if !ok {
		return fmt.Errorf("creature %s for player %s not found", player.CreatureID, playerID)
	}

	// 1. Determine player room ID
	playerRoomID := player.RoomID
	if playerRoomID.IsZero() {
		playerRoomID = creature.RoomID
	}
	if playerRoomID.IsZero() {
		return fmt.Errorf("player %s is not in any room", playerID)
	}
	survivalRoom := false
	if room, ok := w.rooms[playerRoomID]; ok {
		survivalRoom = roomHasAnyFlag(room, "RSUVIV", "survival")
	}

	attackerIsPlayer := false
	var attacker model.Creature
	attackerExists := false
	if !attackerID.IsZero() {
		if att, exists := w.creatures[attackerID]; exists {
			attacker = att
			attackerExists = true
			if att.Kind == model.CreatureKindPlayer || !att.PlayerID.IsZero() {
				attackerIsPlayer = true
			}
		}
	}

	deathPenaltyApplies := !survivalRoom && playerDeathCheckWarReturnsOneLocked(w, attacker, attackerExists, creature)
	if deathPenaltyApplies {
		playerDeathProcessReadyObjectsLocked(w, &creature, playerRoomID, attackerIsPlayer)
	}

	// 6. Deduct exp based on legacy player-death formulas.
	if !attackerIsPlayer || attackerID == creature.ID {
		experience, _ := stateCreatureIntValue(creature, "experience")
		class := creatureStateClass(creature)
		level := creatureStateLevel(creature)

		if level < 20 {
			experience -= experience / 20
		} else {
			if experience/15 > 200000 {
				experience -= 200000
			} else if class > 9 { // 9 is model.ClassInvincible (INVINCIBLE)
				experience -= 500000
			} else {
				experience -= experience / 15
			}

			// Level limit correction:
			n := ((level + 3) / 4) - expToLev(experience)
			if n > 1 {
				if level < legacyMaxAutoLevel+2 {
					idx := level - 3
					if idx >= 0 && idx < len(legacyNeededExperience) {
						experience = legacyNeededExperience[idx]
					} else {
						experience = 0
					}
				} else {
					experience = legacyNeededExperience[legacyMaxAutoLevel-2] + (level-legacyMaxAutoLevel-1)*5000000
				}
			}
		}
		if experience < 0 {
			experience = 0
		}
		creature.Stats["experience"] = experience
	}

	if deathPenaltyApplies {
		playerDeathApplyProficiencyLossLocked(&creature)
	}

	if level := creatureStateLevel(creature); level > 0 {
		newLevel := expToLev(creatureStateInt(creature, "experience"))
		if newLevel < level {
			playerDeathApplyLegacyDownLevelsLocked(&player, &creature, newLevel)
		}
	}

	// Restore HP/MP
	hpMax, _ := stateCreatureIntValue(creature, "hpMax")
	if hpMax < 1 {
		hpMax = 1
	}
	creature.Stats["hpCurrent"] = hpMax

	mpMax, _ := stateCreatureIntValue(creature, "mpMax")
	if mpMax < 0 {
		mpMax = 0
	}
	mpCur, _ := stateCreatureIntValue(creature, "mpCurrent")

	newMp := mpCur
	if attackerIsPlayer {
		if mpMax/10 > newMp {
			newMp = mpMax / 10
		}
	} else {
		newMp = mpMax
	}
	creature.Stats["mpCurrent"] = newMp

	// Clear poison and disease tags
	player.Metadata.Tags = removeMetadataTags(player.Metadata.Tags, []string{"PPOISN", "poison", "PDISEA", "disease"})
	creature.Metadata.Tags = removeMetadataTags(creature.Metadata.Tags, []string{"PPOISN", "poison", "PDISEA", "disease"})

	// Reset active titles
	if creature.Properties != nil {
		delete(creature.Properties, "legacyTitle")
	}

	w.recalculateCreatureCombatStatsLocked(&creature)

	// 7. Move the player and creature to room:1008 (or room 1008)
	targetRoomID := model.RoomID("room:1008")
	if _, exists := w.rooms[targetRoomID]; !exists {
		if _, exists2 := w.rooms[model.RoomID("1008")]; exists2 {
			targetRoomID = model.RoomID("1008")
		}
	}

	player.RoomID = targetRoomID
	creature.RoomID = targetRoomID

	w.players[player.ID] = player
	w.creatures[creature.ID] = creature

	for rID, room := range w.rooms {
		room.PlayerIDs = removeID(room.PlayerIDs, player.ID)
		room.CreatureIDs = removeID(room.CreatureIDs, creature.ID)
		w.rooms[rID] = room
	}

	toRoom, ok := w.rooms[targetRoomID]
	if ok {
		toRoom.PlayerIDs = w.insertPlayerIDLegacySortedLocked(toRoom.PlayerIDs, player.ID)
		toRoom.CreatureIDs = w.insertCreatureIDLegacySortedLocked(toRoom.CreatureIDs, creature.ID)
		w.rooms[targetRoomID] = toRoom
	}
	nowUnix := time.Now().Unix()
	w.refreshRoomPermanentSpawnsLocked(targetRoomID, nowUnix)
	w.checkRoomExitsLocked(targetRoomID, nowUnix)

	// B (A push): mark at mutation time (many changes: hp/mp restore, tags, titles, room, inv stripped)
	// Safe now because dirty uses separate dirtyMu (no deadlock with world.mu held).
	w.MarkPlayerDirty(playerID)
	// Unlock before SavePlayer (death is critical durability point - explicit sync Save kept)
	w.unlockDomains(true, true, true, true, true, true, true)
	unlocked = true

	// 8. Call SavePlayer (explicit for death; mutation already marked)
	return w.SavePlayer(playerID)
}

func playerDeathCheckWarReturnsOneLocked(w *World, attacker model.Creature, attackerExists bool, dead model.Creature) bool {
	firstFamily := 0
	if attackerExists {
		firstFamily = playerDeathCreatureFamilyID(attacker)
	}
	secondFamily := playerDeathCreatureFamilyID(dead)
	if firstFamily == 0 || secondFamily == 0 {
		return true
	}
	active := w.familyWar.Active
	if active.IsZero() {
		return true
	}
	return !((firstFamily == active.First && secondFamily == active.Second) ||
		(firstFamily == active.Second && secondFamily == active.First))
}

func playerDeathCreatureFamilyID(creature model.Creature) int {
	if value, ok := playerDeathCreatureIntValue(creature, "familyID", "dailyExpndMax", "legacyDailyExpndMax"); ok {
		return value
	}
	return 0
}

func playerDeathCreatureIntValue(creature model.Creature, names ...string) (int, bool) {
	targets := normalizedFlagSet(names...)
	for key, value := range creature.Stats {
		if _, ok := targets[normalizeFlagName(key)]; ok {
			return value, true
		}
	}
	for key, raw := range creature.Properties {
		if _, ok := targets[normalizeFlagName(key)]; !ok {
			continue
		}
		value, ok := parseStateInt(raw)
		return value, ok
	}
	return 0, false
}

func playerDeathProcessReadyObjectsLocked(w *World, creature *model.Creature, roomID model.RoomID, attackerIsPlayer bool) {
	if creature == nil || len(creature.Equipment) == 0 {
		return
	}
	slots := make([]string, 0, len(creature.Equipment))
	for slot, objectID := range creature.Equipment {
		if strings.TrimSpace(slot) != "" && !objectID.IsZero() {
			slots = append(slots, slot)
		}
	}
	slices.Sort(slots)

	for _, slot := range slots {
		objectID := creature.Equipment[slot]
		if objectID.IsZero() {
			continue
		}
		object, exists := w.objects[objectID]
		if !exists {
			continue
		}
		wield := playerDeathIsWieldSlot(slot)
		if !w.playerDeathReadyObjectDropsLocked(object, wield) {
			continue
		}

		delete(creature.Equipment, slot)
		creature.Inventory.ObjectIDs = removeID(creature.Inventory.ObjectIDs, objectID)

		if wield || attackerIsPlayer {
			object.Location = model.ObjectLocation{RoomID: roomID}
			w.objects[objectID] = object
			room := w.rooms[roomID]
			room.Objects.ObjectIDs = appendIDOnce(room.Objects.ObjectIDs, objectID)
			w.rooms[roomID] = room
			if wield {
				w.playerDeathMarkTempPermLocked(objectID)
			}
			w.MarkRoomObjectsDirty(roomID)
			continue
		}

		object.Location = model.ObjectLocation{CreatureID: creature.ID, Slot: "inventory"}
		w.objects[objectID] = object
		creature.Inventory.ObjectIDs = appendIDOnce(creature.Inventory.ObjectIDs, objectID)
	}
}

func playerDeathApplyProficiencyLossLocked(creature *model.Creature) {
	if creature == nil {
		return
	}
	var proficiency [5]int
	var realms [4]int
	for i := range proficiency {
		proficiency[i] = stateCreatureProficiencyValue(*creature, i)
	}
	for i := range realms {
		realms[i] = stateCreatureRealm(*creature, i)
	}

	proficiency, realms = playerDeathLowerProficiency(proficiency, realms, creatureStateInt(*creature, "experience"))

	for i, value := range proficiency {
		keys := playerDeathProficiencyKeys(i)
		playerDeathWriteLoweredValue(creature, keys, keys[0], value)
	}
	for i, value := range realms {
		keys := playerDeathRealmKeys(i)
		playerDeathWriteLoweredValue(creature, keys, keys[0], value)
	}
}

func playerDeathLowerProficiency(proficiency [5]int, realms [4]int, experience int) ([5]int, [4]int) {
	total := 0
	for _, value := range proficiency {
		total += value
	}
	for _, value := range realms {
		total += value
	}

	profLoss := total - experience - 1024
	if profLoss < 0 {
		profLoss = 0
	}
	belowZero := 0
	for profLoss > 9 && belowZero < 9 {
		belowZero = 0
		for n := 0; n < 9; n++ {
			part := profLoss / (9 - n)
			profLoss -= part
			if n < 5 {
				proficiency[n] -= part
				if proficiency[n] < 0 {
					belowZero++
					profLoss -= proficiency[n]
					proficiency[n] = 0
				}
				continue
			}
			idx := n - 5
			realms[idx] -= part
			if realms[idx] < 0 {
				belowZero++
				profLoss -= realms[idx]
				realms[idx] = 0
			}
		}
	}

	best := 0
	for i := 1; i < len(proficiency); i++ {
		if proficiency[i] > proficiency[best] {
			best = i
		}
	}
	if proficiency[best] < 1024 {
		proficiency[best] = 1024
	}
	return proficiency, realms
}

func playerDeathProficiencyKeys(idx int) []string {
	if idx < 0 || idx >= len(weaponProficiencyStatKeys) {
		return []string{fmt.Sprintf("proficiency/%d", idx)}
	}
	part := weaponProficiencyPropertyKeys[idx]
	return []string{
		weaponProficiencyStatKeys[idx],
		fmt.Sprintf("proficiency/%s", part),
		fmt.Sprintf("proficiency.%s", part),
		fmt.Sprintf("proficiency_%s", part),
		fmt.Sprintf("proficiency/%d", idx),
		fmt.Sprintf("proficiency.%d", idx),
		fmt.Sprintf("proficiency_%d", idx),
		fmt.Sprintf("proficiency%d", idx),
	}
}

func playerDeathRealmKeys(idx int) []string {
	keys := []string{"realmEarth", "realmWind", "realmFire", "realmWater"}
	if idx >= 0 && idx < len(keys) {
		return []string{
			keys[idx],
			fmt.Sprintf("realm/%d", idx+1),
			fmt.Sprintf("realm.%d", idx+1),
			fmt.Sprintf("realm_%d", idx+1),
			fmt.Sprintf("realm%d", idx+1),
		}
	}
	return []string{fmt.Sprintf("realm/%d", idx+1)}
}

func playerDeathWriteLoweredValue(creature *model.Creature, keys []string, defaultStatKey string, value int) {
	if creature == nil || len(keys) == 0 {
		return
	}
	wrote := false
	for _, key := range keys {
		if creature.Stats != nil {
			if _, ok := creature.Stats[key]; ok {
				creature.Stats[key] = value
				wrote = true
			}
		}
	}
	for _, key := range keys {
		if creature.Properties != nil {
			if _, ok := creature.Properties[key]; ok {
				creature.Properties[key] = strconv.Itoa(value)
				wrote = true
			}
		}
	}
	if wrote {
		return
	}
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	creature.Stats[defaultStatKey] = value
}

func playerDeathApplyLegacyDownLevelsLocked(player *model.Player, creature *model.Creature, targetLevel int) {
	if creature == nil {
		return
	}
	if creature.Stats == nil {
		creature.Stats = map[string]int{}
	}
	level := creatureStateLevel(*creature)
	if targetLevel < 1 {
		targetLevel = 1
	}
	if level <= targetLevel {
		return
	}

	class := creatureStateClass(*creature)
	bonuses := playerDeathClassStatBonusesFor(class)
	hpMax := creatureStateInt(*creature, "hpMax")
	mpMax := creatureStateInt(*creature, "mpMax")
	pDice := creatureStateInt(*creature, "pDice")
	upDamageCleared := false

	for level > targetLevel {
		level--
		if !upDamageCleared && playerDeathHasUpDamage(*player, *creature) {
			upDamageCleared = true
			if class < model.ClassInvincible {
				hpMax -= 50
				mpMax -= 50
				pDice -= 2
			} else {
				hpMax -= 100
				mpMax -= 100
				pDice -= 3
			}
			playerDeathClearUpDamage(player, creature)
		}

		if (level-1)%2 != 0 {
			hpMax -= bonuses.hp
		} else {
			mpMax -= bonuses.mp
		}

		if (level+1)%4 == 0 {
			idx := (level - 1) % 10
			if idx < 0 {
				idx += 10
			}
			if statName := playerDeathLegacyStatName(playerDeathLevelCycleFor(class)[idx]); statName != "" {
				creature.Stats[statName] = creatureStateInt(*creature, statName) - 1
			}
		}
	}

	creature.Level = targetLevel
	creature.Stats["level"] = targetLevel
	creature.Stats["hpMax"] = hpMax
	creature.Stats["mpMax"] = mpMax
	creature.Stats["hpCurrent"] = hpMax
	creature.Stats["mpCurrent"] = mpMax
	creature.Stats["pDice"] = pDice
}

func playerDeathHasUpDamage(player model.Player, creature model.Creature) bool {
	return creatureHasAnyFlag(creature, "PUPDMG", "upDamage", "upDmg") ||
		hasAnyNormalizedFlag(player.Metadata.Tags, "PUPDMG", "upDamage", "upDmg")
}

func playerDeathClearUpDamage(player *model.Player, creature *model.Creature) {
	remove := []string{"PUPDMG", "upDamage", "upDmg"}
	if player != nil {
		player.Metadata.Tags = removeMetadataTags(player.Metadata.Tags, remove)
	}
	if creature == nil {
		return
	}
	creature.Metadata.Tags = removeMetadataTags(creature.Metadata.Tags, remove)
	if creature.Stats != nil {
		for _, key := range remove {
			if _, ok := creature.Stats[key]; ok {
				creature.Stats[key] = 0
			}
		}
	}
	if creature.Properties != nil {
		for _, key := range remove {
			if _, ok := creature.Properties[key]; ok {
				creature.Properties[key] = "0"
			}
		}
	}
}

func playerDeathClassStatBonusesFor(class int) playerDeathClassStatBonusEntry {
	if bonuses, ok := playerDeathClassStatBonuses[class]; ok {
		return bonuses
	}
	return playerDeathClassStatBonuses[0]
}

func playerDeathLevelCycleFor(class int) [10]int {
	if cycle, ok := playerDeathLevelCycleTable[class]; ok {
		return cycle
	}
	return playerDeathLevelCycleTable[0]
}

func playerDeathLegacyStatName(id int) string {
	switch id {
	case playerDeathLegacyStatSTR:
		return "strength"
	case playerDeathLegacyStatDEX:
		return "dexterity"
	case playerDeathLegacyStatCON:
		return "constitution"
	case playerDeathLegacyStatINT:
		return "intelligence"
	case playerDeathLegacyStatPTY:
		return "piety"
	default:
		return ""
	}
}

func playerDeathIsWieldSlot(slot string) bool {
	switch normalizeFlagName(slot) {
	case "wield", "weapon", "mainhand", "right":
		return true
	default:
		return false
	}
}

func (w *World) playerDeathReadyObjectDropsLocked(object model.ObjectInstance, wield bool) bool {
	if w.playerDeathObjectIntLocked(object, "questNumber", "questnum", "quest") != 0 {
		return false
	}
	if w.playerDeathObjectHasAnyFlagLocked(object, "OEVENT", "event") {
		return false
	}
	if w.playerDeathObjectHasAnyFlagLocked(object, "OCURSE", "curse", "cursed") {
		return false
	}
	if wield && (w.objectKindIsLocked(object, model.ObjectKindContainer) ||
		w.playerDeathObjectHasAnyFlagLocked(object, "CONTAINER", "container", "OCONTN")) {
		return false
	}
	return true
}

func (w *World) playerDeathObjectIntLocked(object model.ObjectInstance, names ...string) int {
	if value, ok := playerDeathPropertiesInt(object.Properties, names...); ok {
		return value
	}
	if object.PrototypeID.IsZero() {
		return 0
	}
	proto, ok := w.prototypes[object.PrototypeID]
	if !ok {
		return 0
	}
	if value, ok := playerDeathPropertiesInt(proto.Properties, names...); ok {
		return value
	}
	return 0
}

func playerDeathPropertiesInt(properties map[string]string, names ...string) (int, bool) {
	if len(properties) == 0 {
		return 0, false
	}
	targets := normalizedFlagSet(names...)
	for key, raw := range properties {
		if _, ok := targets[normalizeFlagName(key)]; !ok {
			continue
		}
		return parseStateInt(raw)
	}
	return 0, false
}

func (w *World) playerDeathObjectHasAnyFlagLocked(object model.ObjectInstance, names ...string) bool {
	if hasAnyNormalizedFlag(object.Metadata.Tags, names...) || playerDeathPropertiesFlag(object.Properties, names...) {
		return true
	}
	if object.PrototypeID.IsZero() {
		return false
	}
	proto, ok := w.prototypes[object.PrototypeID]
	if !ok {
		return false
	}
	return hasAnyNormalizedFlag(proto.Metadata.Tags, names...) || playerDeathPropertiesFlag(proto.Properties, names...)
}

func playerDeathPropertiesFlag(properties map[string]string, names ...string) bool {
	if len(properties) == 0 {
		return false
	}
	targets := normalizedFlagSet(names...)
	for key, value := range properties {
		normalizedKey := normalizeFlagName(key)
		if _, ok := targets[normalizedKey]; ok && propertyFlagEnabled(value) {
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

func (w *World) playerDeathMarkTempPermLocked(objectID model.ObjectInstanceID) {
	object, ok := w.objects[objectID]
	if !ok {
		return
	}
	object.Metadata.Tags = addMetadataTags(object.Metadata.Tags, []string{"OPERM2", "OTEMPP"})
	w.objects[objectID] = object
	for _, childID := range object.Contents.ObjectIDs {
		w.playerDeathMarkTempPermLocked(childID)
	}
}

var legacyNeededExperience = []int{
	128, 256, 384, 512,
	640, 768, 896, 1024,
	1280, 1536, 1792, 2048,
	2560, 3072, 3584, 4096,
	5120, 6144, 7168, 8192,
	10240, 12288, 14336, 16384,
	20480, 24576, 28672, 32768,
	40960, 49152, 57344, 65536,
	74152, 82768, 91384, 100000,
	111602, 123205, 134807, 146410,
	161647, 176885, 192122, 207360,
	234062, 260765, 287468, 314171,
	350876, 387581, 424286, 460992,
	510275, 559558, 608841, 658125,
	715469, 772814, 830159, 887504,
	966331, 1045159, 1123987, 1202815,
	1327015, 1451215, 1575415, 1699616,
	1825576, 1951536, 2077496, 2203457,
	2352342, 2501228, 2650114, 2799000,
	2975534, 3152069, 3328604, 3505139,
	3745134, 3985129, 4225124, 4465120,
	4797005, 5128890, 5460775, 5792661,
	6224263, 6655866, 7087469, 7519072,
	7957897, 8396723, 8835549, 9274375,
	9605781, 9937187, 10768593, 11384959,
	12295756, 13279416, 14341769, 15489111,
	16728240, 18066499, 19511819, 21072765,
	22758586, 24579273, 26545614, 28669264,
	30962805, 33439829, 36115015, 37920766,
	41333635, 45053662, 49108492, 53528256,
	58345799, 63596921, 69320644, 75559502,
	82359857, 90000000, 100000000, 190000000,
}
