package command

import (
	"fmt"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type ospellData struct {
	magicPower int
	realm      int
	mp         int
	nDice      int
	sDice      int
	pDice      int
	bonusType  int
}

var ospellTable = map[int]ospellData{
	// Tier 1: 1d8+0 or 1d7+1
	2:                    {magicPower: 2, realm: 2, mp: 3, nDice: 1, sDice: 8, pDice: 0, bonusType: 1},                     // SHURTS (삭풍, Wind)
	magicPowerRumble:     {magicPower: magicPowerRumble, realm: 1, mp: 3, nDice: 1, sDice: 8, pDice: 0, bonusType: 1},      // SRUMBL (지동술, Earth)
	magicPowerBurn:       {magicPower: magicPowerBurn, realm: 3, mp: 3, nDice: 1, sDice: 7, pDice: 1, bonusType: 1},        // SBURNS (화선도, Fire)
	magicPowerBlister:    {magicPower: magicPowerBlister, realm: 4, mp: 3, nDice: 1, sDice: 8, pDice: 0, bonusType: 1},     // SBLIST (탄수공, Water)
	7:                    {magicPower: 7, realm: 3, mp: 7, nDice: 2, sDice: 5, pDice: 8, bonusType: 2},                     // SFIREB (화궁, Fire)
	magicPowerDustGust:   {magicPower: magicPowerDustGust, realm: 2, mp: 7, nDice: 2, sDice: 5, pDice: 7, bonusType: 2},    // SDUSTG (풍마현, Wind)
	magicPowerWaterBolt:  {magicPower: magicPowerWaterBolt, realm: 4, mp: 7, nDice: 2, sDice: 5, pDice: 8, bonusType: 2},   // SWBOLT (파초식, Water)
	magicPowerStoneCrush: {magicPower: magicPowerStoneCrush, realm: 1, mp: 7, nDice: 2, sDice: 5, pDice: 7, bonusType: 2},  // SCRUSH (폭진, Earth)
	magicPowerShockbolt:  {magicPower: magicPowerShockbolt, realm: 2, mp: 10, nDice: 2, sDice: 5, pDice: 13, bonusType: 2}, // SSHOCK (권풍술, Wind)
	14:                   {magicPower: 14, realm: 2, mp: 15, nDice: 3, sDice: 4, pDice: 18, bonusType: 3},                  // SLGHTN (뇌전, Wind)
	15:                   {magicPower: 15, realm: 4, mp: 25, nDice: 4, sDice: 5, pDice: 30, bonusType: 4},                  // SICEBL (동설주, Water)

	// Tier 3: 2d5+13 (same tier as shockbolt)
	33: {magicPower: 33, realm: 1, mp: 10, nDice: 2, sDice: 5, pDice: 13, bonusType: 2}, // SENGUL (낙석, Earth)
	34: {magicPower: 34, realm: 3, mp: 10, nDice: 2, sDice: 5, pDice: 13, bonusType: 2}, // SBURST (화풍술, Fire)
	35: {magicPower: 35, realm: 4, mp: 10, nDice: 2, sDice: 5, pDice: 13, bonusType: 2}, // SSTEAM (화룡대천, Water)

	// Tier 4: 3d4+18/19 (same tier as lightning)
	36: {magicPower: 36, realm: 1, mp: 15, nDice: 3, sDice: 4, pDice: 19, bonusType: 3}, // SSHATT (토합술, Earth) - note pDice=19
	37: {magicPower: 37, realm: 3, mp: 15, nDice: 3, sDice: 4, pDice: 18, bonusType: 3}, // SIMMOL (주작현, Fire)
	38: {magicPower: 38, realm: 4, mp: 15, nDice: 3, sDice: 4, pDice: 18, bonusType: 3}, // SBLOOD (열사천, Water)

	// Tier 5: 4d5+30 (same tier as ice blade)
	39: {magicPower: 39, realm: 2, mp: 25, nDice: 4, sDice: 5, pDice: 30, bonusType: 4}, // STHUND (파천풍, Wind)
	40: {magicPower: 40, realm: 1, mp: 25, nDice: 4, sDice: 5, pDice: 30, bonusType: 4}, // SEQUAK (지옥패, Earth)
	41: {magicPower: 41, realm: 3, mp: 25, nDice: 4, sDice: 5, pDice: 30, bonusType: 4}, // SFLFIL (태양안, Fire)

	// Sixiang tier: 5d6+50
	58: {magicPower: 58, realm: 1, mp: 35, nDice: 5, sDice: 6, pDice: 50, bonusType: 5}, // SISIX1 (천지진동, Earth)
	59: {magicPower: 59, realm: 2, mp: 35, nDice: 5, sDice: 6, pDice: 50, bonusType: 5}, // SISIX2 (천상풍, Wind)
	60: {magicPower: 60, realm: 3, mp: 35, nDice: 5, sDice: 6, pDice: 50, bonusType: 5}, // SISIX3 (천마강기, Fire)
	61: {magicPower: 61, realm: 4, mp: 35, nDice: 5, sDice: 6, pDice: 50, bonusType: 5}, // SISIX4 (빙천파, Water)

	// Xixix tier: 11d12+70
	63: {magicPower: 63, realm: 1, mp: 60, nDice: 11, sDice: 12, pDice: 70, bonusType: 5}, // XIXIX1 (혈사천, Earth)
	64: {magicPower: 64, realm: 2, mp: 60, nDice: 11, sDice: 12, pDice: 70, bonusType: 5}, // XIXIX2 (빙설검, Wind)
	65: {magicPower: 65, realm: 3, mp: 60, nDice: 11, sDice: 12, pDice: 70, bonusType: 5}, // XIXIX3 (멸겁화궁, Fire)
	66: {magicPower: 66, realm: 4, mp: 60, nDice: 11, sDice: 12, pDice: 70, bonusType: 5}, // XIXIX4 (탄지수통, Water)
}

var bonusTable = []int{
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

func legacyStatBonus(stat int) int {
	if stat < 0 {
		return -4
	}
	if stat >= len(bonusTable) {
		return 9
	}
	return bonusTable[stat]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func mprofic(c model.Creature, index int) int {
	var n int
	if index == 0 {
		n = creatureProficiency(c, 4)
	} else if index >= 1 && index <= 4 {
		n = creatureRealm(c, index-1)
	}

	var profArray [12]int64
	class := creatureClass(c)
	switch class {
	case model.ClassMage, model.ClassInvincible, model.ClassCaretaker, model.ClassBulsa, model.ClassSubDM, model.ClassDM:
		profArray = [12]int64{0, 1024, 2048, 4096, 8192, 16384, 35768, 85536, 140000, 459410, 2073306, 500000000}
	case model.ClassCleric:
		profArray = [12]int64{0, 1024, 4092, 8192, 16384, 32768, 70536, 119000, 226410, 709410, 2973307, 500000000}
	case model.ClassPaladin, model.ClassRanger:
		profArray = [12]int64{0, 1024, 8192, 16384, 32768, 65536, 105000, 165410, 287306, 809410, 3538232, 500000000}
	default:
		profArray = [12]int64{0, 1024, 40000, 80000, 120000, 160000, 205000, 222000, 380000, 965410, 5495000, 500000000}
	}

	var i int
	var prof int
	for i = 0; i < 11; i++ {
		if int64(n) < profArray[i+1] {
			prof = 10 * i
			break
		}
	}
	if profArray[i+1] > profArray[i] {
		prof += int((int64(n) - profArray[i]) * 10 / (profArray[i+1] - profArray[i]))
	}
	return prof
}

func getCreaturePronoun(creature model.Creature) string {
	if hasAnyNormalizedFlag(creature.Metadata.Tags, "male", "MMALES") {
		return "그"
	}
	return "그녀"
}

func magicBasicOffensiveCharmBlocks(ctx *Context, world LookWorld, actor model.Creature, target magicEffectTarget) bool {
	if !target.hasPlayer {
		return false
	}
	actorPlayerID := magicBasicOffensiveActorPlayerID(ctx, actor)
	return kickActorHasPCharm(world, actor, actorPlayerID) &&
		kickCharmListContainsCreature(world, target.player, target.creature, actor, actorPlayerID)
}

func magicBasicOffensivePlayerGate(ctx *Context, world LookWorld, actor model.Creature, target magicEffectTarget) attackPlayerGateResult {
	if !target.hasPlayer || target.creature.ID == actor.ID {
		return attackPlayerGateResult{Allowed: true}
	}
	actorPlayerID := magicBasicOffensiveActorPlayerID(ctx, actor)
	if actorPlayerID.IsZero() {
		return attackPlayerGateResult{Allowed: true}
	}
	roomID := actor.RoomID
	if roomID.IsZero() {
		roomID = target.creature.RoomID
	}
	room, ok := world.Room(roomID)
	if !ok {
		return attackPlayerGateResult{Allowed: true}
	}

	atWar := attackLegacyAtWar(world)
	if atWar == 0 && roomHasAnyFlag(room, "noKill", "noPlayerKill", "RNOKIL") {
		return attackPlayerGateResult{Message: "\n이 방에선 전투가 금지되있습니다.\n"}
	}
	if ((attackCreatureLevel(actor)+3)/4) < 4 && ((attackCreatureLevel(target.creature)+3)/4) > 5 {
		return attackPlayerGateResult{Message: "\n!! 좋아요.. 죽을려면 무슨짓을 못하겠어요. !!\n"}
	}

	actorFamily := attackPlayerHasFlag(world, actorPlayerID, actor, "PFAMIL", "familyFlag")
	targetFamily := attackPlayerHasFlag(world, target.player.ID, target.creature, "PFAMIL", "familyFlag")
	checkChaos := !actorFamily || !targetFamily
	if !checkChaos {
		checkChaos = attackLegacyCheckWar(atWar, attackCreatureFamilyID(actor), attackCreatureFamilyID(target.creature))
	}
	if checkChaos && !roomHasAnyFlag(room, "survival", "RSUVIV") {
		if !attackPlayerHasFlag(world, actorPlayerID, actor, "PCHAOS", "chaos") {
			return attackPlayerGateResult{Message: "\n미안하지만 당신은 선합니다.\n"}
		}
		if !attackPlayerHasFlag(world, target.player.ID, target.creature, "PCHAOS", "chaos") {
			return attackPlayerGateResult{Message: "\n미안하지만 그사람은 선합니다.\n"}
		}
	}
	return attackPlayerGateResult{Allowed: true}
}

func magicBasicOffensiveActorPlayerID(ctx *Context, actor model.Creature) model.PlayerID {
	if !actor.PlayerID.IsZero() {
		return actor.PlayerID
	}
	return InventoryPlayerIDFromContext(ctx)
}

func magicBasicOffensiveResolveTarget(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	target string,
	ordinal int64,
) (magicEffectTarget, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return magicEffectSelfTarget(ctx, world, actor)
	}

	viewer := LookViewer{
		PlayerID:   InventoryPlayerIDFromContext(ctx),
		CreatureID: actor.ID,
	}
	roomID := actor.RoomID
	if roomID.IsZero() && !viewer.PlayerID.IsZero() {
		if player, ok := world.Player(viewer.PlayerID); ok {
			roomID = player.RoomID
		}
	}
	room, ok := world.Room(roomID)
	if !ok {
		return magicEffectTarget{}, false
	}
	if creature, ok := findAttackCreatureTarget(world, room, viewer, target, ordinal); ok {
		return magicEffectTarget{creature: creature}, true
	}
	if player, creature, ok := findLegacyPlayerCreatureTarget(world, room, viewer, legacyUpperFirstASCII(target), ordinal); ok {
		return magicEffectTarget{creature: creature, player: player, hasPlayer: true}, true
	}
	return magicEffectTarget{}, false
}

func magicBasicOffensiveRestrictionMessage(actor model.Creature, magicPower int) string {
	class := creatureClass(actor)
	switch magicPower {
	case 15, magicPowerThunderbolt, magicPowerEarthquake, magicPowerFlameFill:
		if class != model.ClassMage && class < model.ClassInvincible {
			return "\n도술사만이 쓸 수 있는 마법입니다.\n"
		}
	case magicPowerSisix1, magicPowerSisix2, magicPowerSisix3, magicPowerSisix4:
		if !creatureHasAnyFlag(actor, "SMAGE") {
			return "\n도술사를 무적수련한 사람만이 쓸 수 있는 마법입니다.\n"
		}
	case magicPowerXixix1, magicPowerXixix2, magicPowerXixix3, magicPowerXixix4:
		if class < model.ClassCaretaker {
			return "\n초인 이상만이 사용할수 있는 마법입니다.\n"
		}
	}
	return ""
}

func magicBasicOffensiveRevealInvisibility(ctx *Context, world StatusWorld, actor model.Creature) (model.Creature, error) {
	playerID := magicBasicOffensiveActorPlayerID(ctx, actor)
	wasInvisible := creatureHasAnyFlag(actor, "PINVIS", "pinvis", "invisible")
	if !playerID.IsZero() {
		if player, ok := world.Player(playerID); ok && hasAnyNormalizedFlag(player.Metadata.Tags, "PINVIS", "pinvis", "invisible") {
			wasInvisible = true
		}
	}
	if !wasInvisible {
		return actor, nil
	}

	if updater, ok := world.(interface {
		UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	}); ok {
		updated, err := updater.UpdateCreatureTags(actor.ID, nil, []string{"PINVIS", "pinvis", "invisible"})
		if err != nil {
			return actor, err
		}
		actor = updated
	}
	if !playerID.IsZero() {
		if updater, ok := world.(interface {
			UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
		}); ok {
			if _, err := updater.UpdatePlayerTags(playerID, nil, []string{"PINVIS", "pinvis", "invisible"}); err != nil {
				return actor, err
			}
		}
	}
	if actor.Stats != nil && actor.Stats["PINVIS"] != 0 {
		if setter, ok := world.(interface {
			SetCreatureStat(model.CreatureID, string, int) error
		}); ok {
			if err := setter.SetCreatureStat(actor.ID, "PINVIS", 0); err != nil {
				return actor, err
			}
			actor.Stats["PINVIS"] = 0
		}
	}
	if refreshed, ok := world.Creature(actor.ID); ok {
		actor = refreshed
	}

	ctx.WriteString("\n당신의 모습이 원래대로 돌아왔습니다.\n")
	actorName := attackCreatureName(actor)
	_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 모습이 나타납니다.\n")
	return actor, nil
}

func magicBasicOffensiveApplyRealmGain(world StatusWorld, actor model.Creature, target magicEffectTarget, osp ospellData, applied int) error {
	if target.hasPlayer || applied <= 0 {
		return nil
	}
	setter, ok := world.(interface {
		SetCreatureStat(model.CreatureID, string, int) error
	})
	if !ok {
		return nil
	}
	key := magicBasicOffensiveRealmStatKey(osp.realm)
	if key == "" {
		return nil
	}
	experience := creatureStat(target.creature, "experience")
	if experience <= 0 {
		return nil
	}
	hpMax := creatureStat(target.creature, "hpMax")
	if hpMax < 1 {
		hpMax = 1
	}
	gain := (applied * experience) / hpMax
	if gain > experience {
		gain = experience
	}
	if gain <= 0 {
		return nil
	}
	currentActor := actor
	if refreshed, ok := world.Creature(actor.ID); ok {
		currentActor = refreshed
	}
	return setter.SetCreatureStat(actor.ID, key, creatureStat(currentActor, key)+gain)
}

func magicBasicOffensiveConsumeMP(world StatusWorld, actor model.Creature, cost int) (model.Creature, error) {
	if cost <= 0 {
		return actor, nil
	}
	if err := magicEffectConsumeMP(world, actor, cost); err != nil {
		return actor, err
	}
	if refreshed, ok := world.Creature(actor.ID); ok {
		return refreshed, nil
	}
	return actor, nil
}

func magicBasicOffensiveRealmStatKey(realm int) string {
	switch realm {
	case 1:
		return "realmEarth"
	case 2:
		return "realmWind"
	case 3:
		return "realmFire"
	case 4:
		return "realmWater"
	default:
		return ""
	}
}

// magicKillFinalizeWorld records offensive-spell damage into the monster damage
// ledger (C add_enm_dmg) and finalizes a monster slain by a spell (C die()):
// XP share, corpse loot/gold drop, alignment shift, removal. Implemented by
// *state.World; the same methods back every melee/skill kill path.
type magicKillFinalizeWorld interface {
	RecordCreatureDamage(victimID, attackerID model.CreatureID, damage int) error
	FinalizeMonsterDeath(model.CreatureID) (bool, error)
}

func magicEffectApplyBasicOffensiveDamage(
	ctx *Context,
	world StatusWorld,
	actor model.Creature,
	object model.ObjectInstance,
	resolved ResolvedCommand,
	magicPower int,
) (bool, error) {
	how := determineHow(world, object)
	targetArg := strings.TrimSpace(getArg(resolved, 1))
	var err error
	actor, err = magicBasicOffensiveRevealInvisibility(ctx, world, actor)
	if err != nil {
		return false, err
	}
	if targetArg != "" && how == howPotion {
		ctx.WriteString("\n당신에게만 사용할수 있습니다.\n")
		return false, nil
	}

	target, ok := magicBasicOffensiveResolveTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
	if !ok || target.creature.ID.IsZero() {
		ctx.WriteString("\n그런 것은 여기에 존재하지 않습니다.\n")
		return false, nil
	}
	if targetArg != "" && target.creature.ID == actor.ID {
		ctx.WriteString("\n그런 것은 여기에 존재하지 않습니다.\n")
		return false, nil
	}
	if !target.hasPlayer && attackCreatureProtected(target.creature) {
		ctx.WriteString("\n당신은 " + getCreaturePronoun(target.creature) + "를 공격할 수 없습니다.\n")
		return false, nil
	}
	if gate := magicBasicOffensivePlayerGate(ctx, world, actor, target); !gate.Allowed {
		ctx.WriteString(gate.Message)
		return false, nil
	}
	if magicBasicOffensiveCharmBlocks(ctx, world, actor, target) {
		ctx.WriteString("You just can't bring yourself to do that.\n")
		return false, nil
	}
	isSelfTarget := target.creature.ID == actor.ID

	osp, found := ospellTable[magicPower]
	if !found {
		return false, nil
	}

	var bns int
	if how == howCast {
		intel := creatureStat(actor, "intelligence")
		var divisor int
		switch osp.bonusType {
		case 1:
			divisor = 10
		case 2:
			divisor = 6
		case 3:
			divisor = 4
		case 4:
			divisor = 3
		case 5:
			divisor = 2
		default:
			divisor = 1
		}
		bns = legacyStatBonus(intel) + mprofic(actor, osp.realm)/divisor
	}

	// Room element modifiers
	room, roomOK := world.Room(actor.RoomID)
	if roomOK {
		if roomHasAnyFlag(room, "RWATER", "water") {
			switch osp.realm {
			case 4: // WATER
				bns *= 2
			case 3: // FIRE
				bns = minInt(-bns, -5)
			}
		} else if roomHasAnyFlag(room, "RFIRER", "fire", "firer") {
			switch osp.realm {
			case 3: // FIRE
				bns *= 2
			case 4: // WATER
				bns = minInt(-bns, -5)
			}
		} else if roomHasAnyFlag(room, "RWINDR", "wind", "windr") {
			switch osp.realm {
			case 2: // WIND
				bns *= 2
			case 1: // EARTH
				bns = minInt(-bns, -5)
			}
		} else if roomHasAnyFlag(room, "REARTH", "earth") {
			switch osp.realm {
			case 1: // EARTH
				bns *= 2
			case 2: // WIND
				bns = minInt(-bns, -5)
			}
		}
	}

	if how == howCast && !isSelfTarget {
		actor, err = magicBasicOffensiveConsumeMP(world, actor, osp.mp)
		if err != nil {
			return false, err
		}
	}
	if !isSelfTarget && spellFail(actor) {
		return false, nil
	}

	dmg := rollDice(osp.nDice, osp.sDice, osp.pDice+bns)
	if dmg < 1 {
		dmg = 1
	}
	if message := magicBasicOffensiveRestrictionMessage(actor, magicPower); message != "" {
		ctx.WriteString(message)
		return false, nil
	}

	damageWorld, ok := world.(magicCreatureDamageWorld)
	if !ok {
		return false, nil
	}

	var updatedTarget model.Creature
	var died bool
	if isSelfTarget {
		updatedTarget, _, died, err = damageWorld.ApplyCreatureDamage(target.creature.ID, dmg)
		if err != nil {
			return false, err
		}
		if how == howCast {
			actor, err = magicBasicOffensiveConsumeMP(world, actor, osp.mp)
			if err != nil {
				return false, err
			}
		}
		target.creature = updatedTarget
		if died {
			if err := setCreatureStat(world, target.creature.ID, "hpCurrent", 1); err != nil {
				return false, err
			}
			target.creature.Stats["hpCurrent"] = 1
		}
	} else {
		dmg = ApplyElementalResistance(target.creature, magicPower, dmg)
		var applied int
		updatedTarget, applied, died, err = damageWorld.ApplyCreatureDamage(target.creature.ID, dmg)
		if err != nil {
			return false, err
		}
		if err := magicBasicOffensiveApplyRealmGain(world, actor, target, osp, applied); err != nil {
			return false, err
		}

		RegisterSpellAggro(world, target.creature.ID, actor.ID)
		// C offensive_spell (magic1.c:1274-1276) records the spell damage into the
		// monster's damage ledger (add_enm_dmg, with m = MIN(hpcur, dmg) = applied)
		// so the caster earns an XP share on the kill. Monster targets only, matching
		// C's crt_ptr->type != PLAYER guard.
		if !target.hasPlayer {
			if recorder, ok := world.(magicKillFinalizeWorld); ok {
				if err := recorder.RecordCreatureDamage(target.creature.ID, actor.ID, applied); err != nil {
					return false, err
				}
			}
		}
	}

	var spellname string
	switch magicPower {
	case 2:
		spellname = "삭풍"
	case magicPowerRumble:
		spellname = "지동술"
	case magicPowerBurn:
		spellname = "화선도"
	case magicPowerBlister:
		spellname = "탄수공"
	case 7:
		spellname = "화궁"
	case magicPowerShockbolt:
		spellname = "권풍술"
	case magicPowerDustGust:
		spellname = "풍마현"
	case magicPowerWaterBolt:
		spellname = "파초식"
	case magicPowerStoneCrush:
		spellname = "폭진"
	case 14:
		spellname = "뇌전"
	case 15:
		spellname = "동설주"
	case magicPowerEngulf:
		spellname = "낙석"
	case magicPowerBurstFlame:
		spellname = "화풍술"
	case magicPowerSteamBlast:
		spellname = "화룡대천"
	case magicPowerShatterStone:
		spellname = "토합술"
	case magicPowerImmolate:
		spellname = "주작현"
	case magicPowerBloodBoil:
		spellname = "열사천"
	case magicPowerThunderbolt:
		spellname = "파천풍"
	case magicPowerEarthquake:
		spellname = "지옥패"
	case magicPowerFlameFill:
		spellname = "태양안"
	case magicPowerSisix1:
		spellname = "천지진동"
	case magicPowerSisix2:
		spellname = "천상풍"
	case magicPowerSisix3:
		spellname = "천마강기"
	case magicPowerSisix4:
		spellname = "빙천파"
	case magicPowerXixix1:
		spellname = "혈사천"
	case magicPowerXixix2:
		spellname = "빙설검"
	case magicPowerXixix3:
		spellname = "멸겁화궁"
	case magicPowerXixix4:
		spellname = "탄지수통"
	}

	actorName := attackCreatureName(actor)
	targetName := attackCreatureName(target.creature)

	if target.creature.ID == actor.ID {
		// Cast on self
		spellDetail := offensiveCasterDetail(magicPower)
		if spellDetail != "" && !strings.HasPrefix(spellDetail, "\n") {
			spellDetail = "\n" + spellDetail
		}
		ctx.WriteString("\n당신은 " + spellname + " 주문을 자신에게 겁니다.\n" + spellDetail)
		ctx.WriteString(fmt.Sprintf("\n주술이 %d만큼의 피해를 주었습니다.\n", dmg))

		pronoun := getCreaturePronoun(actor)
		roomMsg := fmt.Sprintf("\n%s이 %s 주술을 %s 자신에게 걸고 있습니다.\n", actorName, spellname, pronoun)
		_ = roomBroadcast(ctx, actor.RoomID, roomMsg)
	} else {
		// Cast on monster or player
		spellDetail := offensiveCasterDetail(magicPower)

		ctx.WriteString("\n당신은 " + spellname + " 주문을 " + targetName + "에게 걸었습니다.\n" + spellDetail)
		ctx.WriteString(fmt.Sprintf("\n주문이 %d만큼의 피해를 주었습니다.\n", dmg))

		// If target has player
		if target.hasPlayer {
			targetMsg := fmt.Sprintf("\n%s이 %s 주술로 당신에게 %d만큼의 피해를 주었습니다.\n", actorName, spellname, dmg)
			_ = sendToPlayer(ctx, target.player.ID, targetMsg)
		}

		// Room broadcast
		initialRoomMsg := fmt.Sprintf("\n%s이 %s 주문을 %s에게 외웁니다.", actorName, spellname, targetName)
		_ = broadcastRom2(ctx, world, actor.RoomID, actor.PlayerID, target.player.ID, initialRoomMsg)
		if roomSpellMsg := offensiveRoomDetail(actorName, magicPower); roomSpellMsg != "" {
			_ = roomBroadcast(ctx, actor.RoomID, roomSpellMsg)
		}
	}

	if died {
		if isSelfTarget {
			ctx.WriteString("\n!! 좋아요.. 죽을려면 무슨짓을 못하겠어요. !!\n")
			return true, nil
		}
		ctx.WriteString("\n당신은 " + targetName + krtext.Particle(targetName, '3') + " 죽였습니다.\n")
		_ = roomBroadcast(ctx, actor.RoomID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+targetName+krtext.Particle(targetName, '3')+" 죽여버렸습니다.\n")

		// C offensive_spell (magic1.c:1425-1431) calls die() on the slain monster:
		// awards XP by damage share, drops the corpse loot/gold, applies the
		// alignment shift, and removes it. FinalizeMonsterDeath is the real finalizer
		// (used by every melee/skill kill path); the previous DieCreature type
		// assertion matched no world type, so spell kills were never reaped —
		// monsters were left in the room at 0 HP with no reward.
		if !target.hasPlayer {
			if finalizer, ok := world.(magicKillFinalizeWorld); ok {
				if _, err := finalizer.FinalizeMonsterDeath(updatedTarget.ID); err != nil {
					return false, err
				}
			}
		}
	}

	return true, nil
}

func offensiveCasterDetail(magicPower int) string {
	switch magicPower {
	case 2:
		return "삭풍소멸주... 삭풍이여! 너의 힘으로 모든 것을 소멸시켜라.\n주문을 외우자 북방으로부터 칼날과 같은 거센 바람이 불어 명령에 따라 공격합니다.\n"
	case 7:
		return "멸겁화궁주... 멸겁화의 화살이 모든 것을 태워버린다.\n손을 끌어 기를 모으자 손 끝에서 적색의 불꽃이 공간을 가르며 적에게 날라갑니다.\n"
	case 14:
		return "뇌락천주... 하늘의 분노가 눈길로 변하니 그것이 뇌락이로다.\n양손의 검지를 하늘로 향해 모으자 양쪽눈에서 푸른번개가 작렬하면서 날라 갑니다.\n"
	case 15:
		return "동설주... 북방동장군의 힘은 불도 얼려버리니... 거칠것이 없도다..\n주문을 외우며 부적에 도력을 모아 하늘로 날리자 모든 것을 얼릴 듯 한 눈보라가 휘몰아 칩니다.\n"
	case magicPowerShockbolt:
		return "권풍주... 북한의 기운이요, 동왕의 출수니 나의 손으로 오라.\n주문을 외우며 손을 내밀자 한풍이 소용둘이를 일으키며 주의를 쓸어 버립니다.\n"
	case magicPowerRumble:
		return "지동주... 대지의 정령들이여 나의 명령을 들어 적을 공격하라.\n땅위의 있는 수만마리의 벌레들이 적의 기운을 쫓아 공격을 합니다.\n"
	case magicPowerBurn:
		return "화기선주... 북두의 화두성이 깃발에 실리니 이것으로 모든 것을 태우리라.\n등에 숨겨져 있던 붉은 깃발들이 하늘로 날아올라 진을 형성하자 적의 몸이 불타 오릅니다.\n"
	case magicPowerBlister:
		return "탄지수통주... 관음의 눈물이 손 끝에 맺히니 마도 무릎을 꿇으리라.\n손 끝에 빛나는 이슬이 맺히면서 그것을 튕기자 총알같이 날라가 적의 몸을 꿰뚫어 버립니다.\n"
	case magicPowerDustGust:
		return "현무마압주... 현무의 검은 바람이 마를 삼키니 오행의 흑이 진동하리라.\n주문을 외무며 몸을 돌리자 갑자기 검은 태풍이 날라와 적의 몸을 삼켜 버립니다.\n"
	case magicPowerWaterBolt:
		return "파초식주... 수검도가 파도를 부르니 거대한 바다의 힘으로 적을 날려버리리라.\n소매에 숨겨져있던 얇은 검을 뽑아 검초를 뿌리자 그 안에 숨겨져 있던 수의 기운들이 상대방에게 분출됩니다.\n"
	case magicPowerStoneCrush:
		return "폭진호출주... 지진을 부르는 지룡이여 땅위로 올라와 너의 힘을 보여라.\n목검을 땅에 꽂자 땅이 갈라지면서 지룡이 올라와 날카로운 손톱으로 적을 공격합니다.\n"
	case magicPowerEngulf:
		return "낙석주... 백호의 힘은 산도 무너뜨리니 대암의 정령이여 암우를 떨어뜨려라.\n갑자기 옆에 있던 산만한 바위가 폭파하면서 커다란 바위들이 적에게 떨어집니다.\n"
	case magicPowerBurstFlame:
		return "화풍주... 화마도현의 입김이 바람으로 나타나 적을 태운다.\n수많은 부적을 태우며 하늘로 날리자 화염이 불타오르는 커다란 회오리 바람이 적을 둘러쌉니다.\n"
	case magicPowerSteamBlast:
		return "화룡호출주... 화산의 용이여 너의 입김으로 적을 소멸시켜라.\n당신의 몸이 화룡으로 변하면서 불꽃이 타오르는 몸으로 적을 공격합니다.\n"
	case magicPowerShatterStone:
		return "토분합주... 지옥의 문을 열어 삼켜버리니 모든 죄과를 심판하리...\n갑자기 적의 바로 밑으로 지진이 일어나 땅이 갈라지면서 적을 삼켜버립니다.\n"
	case magicPowerImmolate:
		return "주작호출주... 오행중 화는 주작의 현신이니 천상에서 내려와 마를 정화시키니...\n주문을 외우자 갑자기 불꽃을 내뿜는 주작이 내려와 대지를 불태워 버립니다.\n"
	case magicPowerBloodBoil:
		return "열사천주... 염라의 불꽃이 이곳까지 미치니 모든 마는 이곳에 빠지리라.\n잠잠하던 땅이 흔들리더니 갑자기 용암이 분출하자 적은 놀라 그곳에 빠집니다.\n"
	case magicPowerThunderbolt:
		return "파천운주... 지국천왕의 현신이 검은 구름으로 나타나 마를 심판한다.\n갑자기 검은 구름이 나타나 천지를 붉은 벼락이 진동하면서 적을 강타합니다.\n"
	case magicPowerEarthquake:
		return "지옥도주... 지옥의 야차들이여 생사부의 힘을 빌어 몸을 나타내라.\n주위가 검은 안개로 싸이며 검을 든 33명의 야차가 나타나 적을 무참히 도륙해 버립니다.\n"
	case magicPowerFlameFill:
		return "화안진노주... 천상태자의 눈빛이 나에게 나타나 모든 것을 소멸 시키리라.\n눈을 감고 주문을 외우자 강렬한 빛을 내뿜는 삼지안이 열리면서 모든 것을 불태워 버립니다.\n"
	case magicPowerSisix1:
		return "\n천지진동주... 당신은 땅의 지맥을 건들여 적이 있는 곳의 땅이 갈라집니다.\n 천지가 진동하며 땅의 기운이 적을 감쌉니다. \n"
	case magicPowerSisix2:
		return "\n천상풍주... 단군과 함계 내려온 풍신에게 기원하여 하늘의 기운으로 바람을 일으킨다.\n 갑자기 하늘이 갈라지며 목소리가 들린다. \"너 홍익인간의 정신을 위배한 자여 내가 너를 멸하리라 \"\n 갈라진 틈에서 태풍과 같은 바람이 쏟아져 나옵니다.\n"
	case magicPowerSisix3:
		return "\n천마강기주... .\n 단군과 함계 내려온 뇌신에게 기원하여 하늘의 기운으로 번개를 일으킨다.\n 갑자기 하늘이 갈라지며 목소리가 들린다. \"너 홍익인간의 정신을 위배한 자여 내가 너를 멸하리라 \"\n 갈라진 틈에서 번개가 적을 내려칩니다.\n\n"
	case magicPowerSisix4:
		return "\n빙천파주... 단군과 함계 내려온 우신에게 기원하여 하늘의 기운으로 물보라를 일으킨다.\n 갑자기 하늘이 갈라지며 목소리가 들린다. \"너 홍익인간의 정신을 위배한 자여 내가 너를 멸하리라 \"\n 갈라진 틈에서 산사태과 같은 우박이 쏟아져 내립니다.\n"
	case magicPowerXixix1:
		return "\n혈사천주... 염라의 불꽃이 이곳까지 미치니 모든 마는 이곳에 빠지리라.\n잠잠하던 땅이 흔들리더니 갑자기 용암이 분출하자 적은 놀라 그곳에 빠집니다.\n"
	case magicPowerXixix2:
		return "\n빙설검주... 북방동장군의 힘은 불도 얼려버리니... 거칠것이 없도다.\n주문을 외우며 부적에 도력을 모아 하늘로 날리자 모든 것을 얼릴 듯 한 눈보라가 휘몰아 칩니다.\n"
	case magicPowerXixix3:
		return "\n멸겁화궁주... 멸겁화의 화살이 모든 것을 태워버린다.\n손을 끌어 기를 모으자 손 끝에서 적색의 불꽃이 공간을 가르며 적에게 날라갑니다.\n"
	case magicPowerXixix4:
		return "\n탄지수통주... 관음의 눈물이 손 끝에 맺히니 마도 무릎을 꿇으리라.\n손 끝에 빛나는 이슬이 맺히면서 그것을 튕기자 총알같이 날라가 적의 몸을 꿰뚫어 버립니다.\n"
	default:
		return ""
	}
}

func offensiveRoomDetail(actorName string, magicPower int) string {
	subject := actorName + "이"
	possessive := actorName + "의"
	switch magicPower {
	case 2:
		return "\n" + subject + " 주문을 외우자 북방으로부터 칼날과 같은 거센 바람이\n불어 명령에 따라 공격합니다."
	case 7:
		return "\n" + subject + " 손을 끌어 기를 모으자 손 끝에서 적색의 불꽃이 공간을\n가르며 적에게 날라갑니다."
	case 14:
		return "\n" + subject + " 양손의 검지를 하늘로 향해 모으자 양쪽눈에서 푸른번개가\n작렬하면서 날라 갑니다."
	case 15:
		return "\n" + subject + " 주문을 외우며 부적에 도력을 모아 하늘로 날리자 모든 것을\n얼릴 듯 한 눈보라가 휘몰아 칩니다."
	case magicPowerShockbolt:
		return "\n" + subject + " 주문을 외우며 손을 내밀자 한풍이 소용둘이를 일으키며\n주의를 쓸어 버립니다."
	case magicPowerRumble:
		return "\n" + subject + " 주문을 외우자 땅위의 있는 수만마리의 벌레들이 적의 \n기운을 쫓아 공격을 합니다."
	case magicPowerBurn:
		return "\n" + possessive + " 등에 숨겨져 있던 붉은 깃발들이 하늘로 날아올라 진을\n형성하자 적의 몸이 불타 오릅니다."
	case magicPowerBlister:
		return "\n" + possessive + " 손 끝에 빛나는 이슬이 맺히면서 그것을 튕기자 총알같이\n날라가 적의 몸을 꿰뚫어 버립니다."
	case magicPowerDustGust:
		return "\n" + subject + " 주문을 외무며 몸을 돌리자 갑자기 검은 태풍이 날라와\n적의 몸을 삼켜 버립니다."
	case magicPowerWaterBolt:
		return "\n" + possessive + " 소매에 숨겨져있던 얇은 검을 뽑으며 검초를 뿌리자 \n그 안에 숨겨져 있던 수의 기운들이 상대방에게 분출됩니다."
	case magicPowerStoneCrush:
		return "\n" + subject + " 목검을 땅에 꽂자 땅이 갈라지면서 지룡이 올라와 날카로운\n손톱으로 적을 공격합니다."
	case magicPowerEngulf:
		return "\n" + possessive + " 옆에 있던 산만한 바위가 갑자기 폭파하면서 커다란\n바위들이 적에게 떨어집니다."
	case magicPowerBurstFlame:
		return "\n" + subject + " 수많은 부적을 태우며 하늘로 날리자 화염이 불타오르는\n커다란 회오리 바람이 적을 둘러쌉니다."
	case magicPowerSteamBlast:
		return "\n" + possessive + " 몸이 화룡으로 변하면서 불꽃이 타오르는 몸으로\n적을 공격합니다."
	case magicPowerShatterStone:
		return "\n" + subject + " 갑자기 적의 바로 밑으로 지진이 일어나 땅이 갈라지면서\n적을 삼켜버립니다."
	case magicPowerImmolate:
		return "\n" + subject + " 주문을 외우자 갑자기 불꽃을 내뿜는 주작이 내려와\n대지를 불태워 버립니다."
	case magicPowerBloodBoil:
		return "\n" + subject + " 주문을 외우자 잠잠하던 땅이 흔들리더니 갑자기 용암이\n분출하자 적은 놀라 그곳에 빠집니다."
	case magicPowerThunderbolt:
		return "\n" + subject + " 주문을 외우자 갑자기 검은 구름이 나타나 천지를 \n붉은 벼락이 진동하면서 적을 강타합니다."
	case magicPowerEarthquake:
		return "\n" + possessive + " 주위가 검은 안개로 싸이며 검을 든 33명의 야차가 나타나 적을 무참히 도륙해 버립니다."
	case magicPowerFlameFill:
		return "\n" + subject + " 눈을 감고 주문을 외우자 강렬한 빛을 내뿜는 삼지안이 열리면서 모든 것을 불태워 버립니다."
	default:
		return ""
	}
}
