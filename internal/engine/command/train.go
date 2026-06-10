package command

import (
	"fmt"
	"log"
	"strings"

	"muhan/internal/world/model"
)

const legacyMaxAutoLevel = 128

type TrainWorld interface {
	InventoryWorld
	Room(model.RoomID) (model.Room, bool)
	SetCreatureStat(model.CreatureID, string, int) error
	SetCreatureLevel(model.CreatureID, int) (model.Creature, error)
}

type trainTagWorld interface {
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
}

type trainClassSetterWorld interface {
	SetCreatureClass(model.CreatureID, int) (model.Creature, error)
}

type trainFamilyClassUpdater interface {
	UpdateFamilyMemberAfterClassChange(name string, class int, dailyExpndMax int) error
}

func NewTrainHandler(world TrainWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		player, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}

		class := creatureClass(creature)
		if class >= model.ClassInvincible && len(resolved.Args) > 0 {
			ctx.WriteString("무적수련을 하시려면 \"무적수련\"을 치세요.\n")
			return StatusDefault, nil
		}
		if class == model.ClassBulsa {
			ctx.WriteString("당신은 수련할수 없는 직업입니다.")
			return StatusDefault, nil
		}
		if class == model.ClassCaretaker && !trainActorHasAnyFlag(player, creature, "TRAINBUL", "trainBul") {
			ctx.WriteString("아직 당신은 모든능력치를 향상하지 않았습니다.\n")
			return StatusDefault, nil
		}
		if trainActorHasAnyFlag(player, creature, "PBLIND", "blind") {
			ctx.WriteString("당신은 눈이 멀어 수련할 수 없습니다!")
			return StatusDefault, nil
		}
		if err := trainClearUpDamage(world, player, creature); err != nil {
			return StatusDefault, err
		}
		if updatedCreature, ok := world.Creature(creature.ID); ok {
			creature = updatedCreature
		}

		roomID := trainActorRoomID(player, creature)
		room, ok := world.Room(roomID)
		if !ok {
			return StatusDefault, fmt.Errorf("train actor %q room %q not found", creature.ID, roomID)
		}
		if !roomHasAnyFlag(room, "train", "rtrain", "training") {
			ctx.WriteString("이 곳은 수련할 수 있는곳이 아닙니다!")
			return StatusDefault, nil
		}
		if !trainRoomMatchesClass(room, class) {
			ctx.WriteString("당신이 수련하는곳은 여기가 아닙니다.")
			return StatusDefault, nil
		}

		level := trainCreatureLevel(creature)
		expNeeded, ok := trainRequiredExperience(level)
		if !ok {
			ctx.WriteString("당신은 수련할수 없는 직업입니다.")
			return StatusDefault, nil
		}
		goldNeeded := trainRequiredGold(level, expNeeded)
		experience := creatureStat(creature, "experience")
		if expNeeded > experience {
			ctx.WriteString(fmt.Sprintf("당신은 %d 만큼의 경험치가 더 필요합니다.", expNeeded-experience))
			return StatusDefault, nil
		}
		gold := creatureStat(creature, "gold")
		if goldNeeded > gold {
			ctx.WriteString("돈도 없으면서... 돈 벌어오세요.\n")
			ctx.WriteString(fmt.Sprintf("당신은 수련하는데 %d냥이 듭니다.", goldNeeded))
			return StatusDefault, nil
		}
		if handled, err := trainApplyLegacyClassTransition(ctx, world, player, creature, class, level, gold, goldNeeded); err != nil || handled {
			return StatusDefault, err
		}
		if !trainSafeLevelUp(class, level) {
			ctx.WriteString("당신은 수련할수 없는 직업입니다.")
			return StatusDefault, nil
		}

		upNum, updated, err := trainApplyLegacyLevelUps(world, creature, class, level, experience, gold, goldNeeded)
		if err != nil {
			return StatusDefault, err
		}
		creature = updated

		// B/C: Queue after levelup (SetCreatureStat + apply already mark via A changes)
		if w, ok := world.(interface {
			MarkPlayerDirty(model.PlayerID)
			QueueSave(model.PlayerID, model.BankID)
		}); ok {
			w.MarkPlayerDirty(playerID)
			w.QueueSave(playerID, "")
		} else if saver, ok := world.(interface{ SavePlayer(model.PlayerID) error }); ok {
			if err := saver.SavePlayer(playerID); err != nil {
				log.Printf("[PERSIST] ERROR train levelup SavePlayer %s: %v", playerID, err)
			}
		}

		actorName := trainActorLegacyName(player, creature)
		if upNum <= 1 {
			invokeBroadcast(ctx, fmt.Sprintf("\n### %s님이 %d레벨로 올랐습니다!", actorName, trainCreatureLevel(creature)))
		} else {
			invokeBroadcast(ctx, fmt.Sprintf("\n### %s님이 %d레벨로 %d단계 올랐습니다!", actorName, trainCreatureLevel(creature), upNum))
		}
		ctx.WriteString("\n축하합니다! 당신의 레벨이 올랐습니다!")
		return StatusDefault, nil
	}
}

func trainApplyLegacyClassTransition(ctx *Context, world TrainWorld, player model.Player, creature model.Creature, class int, level int, gold int, goldNeeded int) (bool, error) {
	switch {
	case class == model.ClassInvincible && level >= legacyMaxAutoLevel-1:
		missing := trainMissingInvincibleTrainingNames(player, creature)
		if len(missing) > 0 {
			ctx.WriteString("\n당신은 " + strings.Join(missing, ", ") + " 직업을 무적수련하지 않았습니다.")
			return true, nil
		}
		if err := trainSetCreatureClass(world, creature.ID, model.ClassCaretaker); err != nil {
			return true, err
		}
		if _, err := world.SetCreatureLevel(creature.ID, legacyMaxAutoLevel-1); err != nil {
			return true, err
		}
		if err := world.SetCreatureStat(creature.ID, "gold", gold-goldNeeded); err != nil {
			return true, err
		}
		for _, update := range []struct {
			key   string
			value int
		}{
			{key: "hpMax", value: 800},
			{key: "hpCurrent", value: 800},
			{key: "mpMax", value: 600},
			{key: "mpCurrent", value: 600},
			{key: "nDice", value: 4},
			{key: "sDice", value: 3},
			{key: "pDice", value: 4},
		} {
			if err := world.SetCreatureStat(creature.ID, update.key, update.value); err != nil {
				return true, err
			}
		}
		if err := trainUpdateFamilyClass(world, player, creature, model.ClassCaretaker); err != nil {
			return true, err
		}
		actorName := trainActorLegacyName(player, creature)
		invokeBroadcast(ctx, fmt.Sprintf("\n### %s님께서 초인이 되셨습니다!!", actorName))
		ctx.WriteString("\n축하합니다! 당신은 초인이 되었습니다!!")
		return true, nil
	case class == model.ClassCaretaker && level >= legacyMaxAutoLevel-1:
		if err := trainSetCreatureClass(world, creature.ID, model.ClassBulsa); err != nil {
			return true, err
		}
		if _, err := world.SetCreatureLevel(creature.ID, legacyMaxAutoLevel-1); err != nil {
			return true, err
		}
		if err := world.SetCreatureStat(creature.ID, "gold", gold-goldNeeded); err != nil {
			return true, err
		}
		for _, update := range []struct {
			key   string
			value int
		}{
			{key: "hpMax", value: 3500},
			{key: "hpCurrent", value: 3500},
			{key: "mpMax", value: 2500},
			{key: "mpCurrent", value: 2500},
			{key: "nDice", value: 5},
			{key: "sDice", value: 5},
			{key: "pDice", value: 5},
		} {
			if err := world.SetCreatureStat(creature.ID, update.key, update.value); err != nil {
				return true, err
			}
		}
		if err := trainUpdateFamilyClass(world, player, creature, model.ClassBulsa); err != nil {
			return true, err
		}
		actorName := trainActorLegacyName(player, creature)
		invokeBroadcast(ctx, fmt.Sprintf("\n### %s님께서 불사가 되셨습니다!!", actorName))
		ctx.WriteString("\n축하합니다! 당신은 불사가 되었습니다!!")
		return true, nil
	default:
		return false, nil
	}
}

func trainSetCreatureClass(world TrainWorld, creatureID model.CreatureID, class int) error {
	if setter, ok := world.(trainClassSetterWorld); ok {
		_, err := setter.SetCreatureClass(creatureID, class)
		return err
	}
	return world.SetCreatureStat(creatureID, "class", class)
}

func trainUpdateFamilyClass(world TrainWorld, player model.Player, creature model.Creature, class int) error {
	if !trainActorHasAnyFlag(player, creature, "PFAMIL", "familyFlag") {
		return nil
	}
	updater, ok := world.(trainFamilyClassUpdater)
	if !ok {
		return nil
	}
	return updater.UpdateFamilyMemberAfterClassChange(trainActorLegacyName(player, creature), class, transExpLegacyFamilyID(creature))
}

func trainMissingInvincibleTrainingNames(player model.Player, creature model.Creature) []string {
	missing := make([]string, 0, len(invinceTrainingSpecs))
	for _, training := range invinceTrainingSpecs {
		if !trainActorHasAnyFlag(player, creature, training.tag) {
			missing = append(missing, invinceTrainingDisplayName(training))
		}
	}
	return missing
}

func trainClearUpDamage(world TrainWorld, player model.Player, creature model.Creature) error {
	if !trainActorHasAnyFlag(player, creature, "PUPDMG", "upDamage", "upDmg") {
		return nil
	}
	pDiceBonus, hpBonus, mpBonus := upDamageBonuses(creature)
	for _, update := range []struct {
		key   string
		value int
	}{
		{key: "pDice", value: creatureStat(creature, "pDice") - pDiceBonus},
		{key: "hpMax", value: creatureStat(creature, "hpMax") - hpBonus},
		{key: "mpMax", value: creatureStat(creature, "mpMax") - mpBonus},
		{key: "hpCurrent", value: creatureStat(creature, "hpMax") - hpBonus},
		{key: "mpCurrent", value: creatureStat(creature, "mpMax") - mpBonus},
	} {
		if err := world.SetCreatureStat(creature.ID, update.key, update.value); err != nil {
			return err
		}
	}
	if tagger, ok := world.(trainTagWorld); ok {
		remove := []string{"PUPDMG", "upDamage", "upDmg"}
		if _, err := tagger.UpdateCreatureTags(creature.ID, nil, remove); err != nil {
			return err
		}
		if !player.ID.IsZero() {
			if _, err := tagger.UpdatePlayerTags(player.ID, nil, remove); err != nil {
				return err
			}
		}
	}
	return nil
}

func trainApplyLegacyLevelUps(world TrainWorld, creature model.Creature, class int, level int, experience int, gold int, goldNeeded int) (int, model.Creature, error) {
	upNum := 0
	for {
		if level == 100 && class < model.ClassInvincible {
			break
		}
		gold -= goldNeeded
		if err := world.SetCreatureStat(creature.ID, "gold", gold); err != nil {
			return upNum, creature, err
		}
		updated, err := world.SetCreatureLevel(creature.ID, level+1)
		if err != nil {
			return upNum, creature, err
		}
		class = creatureClass(updated)
		if err := applyLegacyLevelUp(world, updated, class, level, level+1); err != nil {
			return upNum, creature, err
		}
		upNum++
		if current, ok := world.Creature(creature.ID); ok {
			creature = current
		} else {
			creature = updated
		}
		level = trainCreatureLevel(creature)
		class = creatureClass(creature)
		if level >= legacyMaxAutoLevel-1 {
			break
		}
		expNeeded, ok := trainRequiredExperience(level)
		if !ok {
			break
		}
		goldNeeded = trainRequiredGold(level, expNeeded)
		if expNeeded > experience || goldNeeded > gold {
			break
		}
	}
	return upNum, creature, nil
}

func trainActorLegacyName(player model.Player, creature model.Creature) string {
	if creature.DisplayName != "" {
		return creature.DisplayName
	}
	return player.DisplayName
}

func trainActorRoomID(player model.Player, creature model.Creature) model.RoomID {
	if !player.RoomID.IsZero() {
		return player.RoomID
	}
	return creature.RoomID
}

func trainCreatureLevel(creature model.Creature) int {
	level := creature.Level
	if statsLevel := creatureStat(creature, "level"); statsLevel > level {
		level = statsLevel
	}
	return level
}

func trainActorHasAnyFlag(player model.Player, creature model.Creature, names ...string) bool {
	return settingsPlayerFlag(player, names...) ||
		creatureHasAnyFlag(creature, names...) ||
		settingsCreatureFlag(creature, names...)
}

func trainRoomMatchesClass(room model.Room, class int) bool {
	if class > model.ClassThief {
		return true
	}
	if class < model.ClassAssassin {
		return false
	}

	value := class - 1
	return trainRoomTrainingBit(room, 4) == (value&4 != 0) &&
		trainRoomTrainingBit(room, 5) == (value&2 != 0) &&
		trainRoomTrainingBit(room, 6) == (value&1 != 0)
}

func trainRoomTrainingBit(room model.Room, bit int) bool {
	switch bit {
	case 4:
		return roomHasAnyFlag(room, "trainingBit4", "trainBit4", "rtrain4")
	case 5:
		return roomHasAnyFlag(room, "trainingBit5", "trainBit5", "rtrain5")
	case 6:
		return roomHasAnyFlag(room, "trainingBit6", "trainBit6", "rtrain6")
	default:
		return false
	}
}

func trainSafeLevelUp(class int, level int) bool {
	if level <= 0 {
		return false
	}
	switch {
	case class >= model.ClassAssassin && class < model.ClassInvincible:
		return level < 100
	case class == model.ClassInvincible || class == model.ClassCaretaker:
		return level < legacyMaxAutoLevel-1
	default:
		return false
	}
}

func trainRequiredExperience(level int) (int, bool) {
	if level <= 0 {
		return 0, false
	}
	if level <= len(legacyNeededExperience) {
		return legacyNeededExperience[level-1], true
	}
	return legacyNeededExperience[len(legacyNeededExperience)-1] + (level-legacyMaxAutoLevel+1)*5000000, true
}

func trainRequiredGold(level int, expNeeded int) int {
	if level < legacyMaxAutoLevel {
		return expNeeded / 20
	}
	return legacyNeededExperience[len(legacyNeededExperience)-1] / 20
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
