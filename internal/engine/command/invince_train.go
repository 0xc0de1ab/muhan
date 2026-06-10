package command

import (
	"fmt"
	"strings"

	"muhan/internal/world/model"
)

type InvinceTrainWorld interface {
	StatusWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	SetCreatureLevel(model.CreatureID, int) (model.Creature, error)
	BroadcastAll(string) error
}

type invinceTrainingSpec struct {
	tag     string
	aliases []string
}

var invinceTrainingSpecs = []invinceTrainingSpec{
	{tag: "SASSASSIN", aliases: []string{"자객", "assassin"}},
	{tag: "SBARBARIAN", aliases: []string{"권법가", "barbarian"}},
	{tag: "SCLERIC", aliases: []string{"불제자", "cleric"}},
	{tag: "SFIGHTER", aliases: []string{"검사", "fighter"}},
	{tag: "SMAGE", aliases: []string{"도술사", "mage"}},
	{tag: "SPALADIN", aliases: []string{"무사", "paladin"}},
	{tag: "SRANGER", aliases: []string{"포졸", "ranger"}},
	{tag: "STHIEF", aliases: []string{"도둑", "thief"}},
}

func NewInvinceTrainHandler(world InvinceTrainWorld) Handler {
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		player, creature, err := CurrentInventoryCreature(world, InventoryPlayerIDFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}

		if trainActorHasAnyFlag(player, creature, "PBLIND", "blind") {
			ctx.WriteString("당신은 눈이 멀어 무적수련을 할 수 없습니다!")
			return StatusDefault, nil
		}
		roomID := trainActorRoomID(player, creature)
		room, ok := world.Room(roomID)
		if !ok {
			return StatusDefault, fmt.Errorf("invince_train actor %q room %q not found", creature.ID, roomID)
		}
		if !roomHasAnyFlag(room, "train", "rtrain", "training") {
			ctx.WriteString("이 곳은 수련장이 아닙니다!")
			return StatusDefault, nil
		}
		if creatureClass(creature) < model.ClassInvincible {
			ctx.WriteString("무적 이상만 가능합니다.")
			return StatusDefault, nil
		}

		training, ok := invinceTrainingForRoom(room)
		if !ok {
			ctx.WriteString("이 곳은 수련장이 아닙니다!")
			return StatusDefault, nil
		}
		if trainActorHasAnyFlag(player, creature, training.tag) {
			ctx.WriteString("이미 이 직업의 무적수련을 했습니다.")
			return StatusDefault, nil
		}

		trainingCount := invinceTrainingCostCount(player, creature)
		if creatureStat(creature, "experience") < 1000000*trainingCount {
			ctx.WriteString(fmt.Sprintf("무적수련을 하려면 경험치 %d만이 필요합니다.", 100*trainingCount))
			return StatusDefault, nil
		}

		state := &invinceTrainingConfirmState{world: world, playerID: player.ID}
		if !SetPendingLineHandler(ctx, state.handleLine) {
			return StatusDefault, fmt.Errorf("invince_train: failed to set pending line handler")
		}
		ctx.WriteString(invinceTrainingPrompt(creature, trainingCount))
		return StatusDoPrompt, nil
	}
}

type invinceTrainingConfirmState struct {
	world    InvinceTrainWorld
	playerID model.PlayerID
}

func (s *invinceTrainingConfirmState) handleLine(ctx *Context, line string) (Status, error) {
	ClearPendingLineHandler(ctx)
	if !strings.HasPrefix(strings.TrimSpace(line), "예") {
		ctx.WriteString("무적수련이 되지 않았습니다")
		return StatusDefault, nil
	}
	if err := completeInvinceTraining(ctx, s.world, s.playerID); err != nil {
		return StatusDefault, err
	}
	return StatusDefault, nil
}

func completeInvinceTraining(ctx *Context, world InvinceTrainWorld, playerID model.PlayerID) error {
	player, creature, err := CurrentInventoryCreature(world, playerID)
	if err != nil {
		return err
	}
	roomID := trainActorRoomID(player, creature)
	room, ok := world.Room(roomID)
	if !ok {
		return fmt.Errorf("invince_train actor %q room %q not found", creature.ID, roomID)
	}
	training, ok := invinceTrainingForRoom(room)
	if !ok {
		ctx.WriteString("이 곳은 수련장이 아닙니다!")
		return nil
	}

	trainingCount := invinceTrainingCostCount(player, creature)
	experience := creatureStat(creature, "experience") - 1000000*trainingCount
	if err := world.SetCreatureStat(creature.ID, "experience", experience); err != nil {
		return err
	}
	if creatureClass(creature) == model.ClassCaretaker && experience < 100000000 {
		if err := world.SetCreatureStat(creature.ID, "class", model.ClassInvincible); err != nil {
			return err
		}
	}
	if _, err := world.UpdateCreatureTags(creature.ID, []string{training.tag}, nil); err != nil {
		return err
	}
	if !player.ID.IsZero() {
		if _, err := world.UpdatePlayerTags(player.ID, []string{training.tag}, nil); err != nil {
			return err
		}
	}
	if creatureClass(creature) >= model.ClassInvincible && creatureStat(creature, "pDice") < 5 {
		if err := world.SetCreatureStat(creature.ID, "pDice", (trainingCount+1)/2); err != nil {
			return err
		}
	}
	if updated, ok := world.Creature(creature.ID); ok {
		creature = updated
	}
	if creatureClass(creature) == model.ClassInvincible {
		if err := applyClassChangeLevelDown(world, world, player, creature, model.ClassInvincible, experience); err != nil {
			return err
		}
	}

	ctx.WriteString("\n무적수련이 완료되었습니다.")
	_ = world.BroadcastAll(fmt.Sprintf("\n### %s님이 %s 무적수련을 완료했습니다.", attackCreatureName(creature), invinceTrainingDisplayName(training)))
	return nil
}

func invinceTrainingPrompt(creature model.Creature, trainingCount int) string {
	if creatureClass(creature) > model.ClassInvincible {
		return fmt.Sprintf("초인이 무적수련을 하려면 경험치 %d만이 필요합니다.\n무적수련 이후 경험치가 1억이 안되면 무적으로 직업이 바뀝니다.\n무적수련을 하시겠습니까?(예/아니오): ", 200*trainingCount)
	}
	return fmt.Sprintf("무적수련을 하려면 경험치 %d만이 필요합니다.\n무적수련을 하시겠습니까?(예/아니오): ", 100*trainingCount)
}

func invinceTrainingForRoom(room model.Room) (invinceTrainingSpec, bool) {
	value := 0
	if trainRoomTrainingBit(room, 4) {
		value |= 4
	}
	if trainRoomTrainingBit(room, 5) {
		value |= 2
	}
	if trainRoomTrainingBit(room, 6) {
		value |= 1
	}
	if value < 0 || value >= len(invinceTrainingSpecs) {
		return invinceTrainingSpec{}, false
	}
	return invinceTrainingSpecs[value], true
}

func invinceTrainingCostCount(player model.Player, creature model.Creature) int {
	count := 0
	for _, training := range invinceTrainingSpecs {
		if trainActorHasAnyFlag(player, creature, training.tag) {
			count++
		}
	}
	if count == 0 {
		return 1
	}
	return count
}

func invinceTrainingDisplayName(training invinceTrainingSpec) string {
	if len(training.aliases) > 0 {
		return training.aliases[0]
	}
	return training.tag
}

func invinceTrainingForClassName(name string) (invinceTrainingSpec, bool) {
	normalized := normalizeFlagName(name)
	if normalized == "" {
		return invinceTrainingSpec{}, false
	}
	for _, training := range invinceTrainingSpecs {
		if normalizeFlagName(training.tag) == normalized {
			return training, true
		}
		for _, alias := range training.aliases {
			if normalizeFlagName(alias) == normalized {
				return training, true
			}
		}
	}
	return invinceTrainingSpec{}, false
}
