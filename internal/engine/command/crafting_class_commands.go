package command

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	legacyNewForgeRoomNumber = 611

	WeaponForgeModeForge    WeaponForgeMode = "forge"
	WeaponForgeModeNewForge WeaponForgeMode = "newforge"
)

type CraftingClassWorld interface {
	InventoryWorld
	Room(model.RoomID) (model.Room, bool)
}

type weaponForgeMutationWorld interface {
	CreateObjectFromPrototype(model.PrototypeID, model.CreatureID) (model.ObjectInstanceID, error)
	SetCreatureStat(model.CreatureID, string, int) error
	SetObjectDisplayName(model.ObjectInstanceID, string) (model.ObjectInstance, error)
	SetObjectProperty(model.ObjectInstanceID, string, string) (model.ObjectInstance, error)
}

type weaponForgeTagWorld interface {
	UpdateObjectTags(model.ObjectInstanceID, []string, []string) (model.ObjectInstance, error)
}

type classChangeMutationWorld interface {
	SetCreatureStat(model.CreatureID, string, int) error
	SetCreatureLevel(model.CreatureID, int) (model.Creature, error)
}

type classChangeClassSetterWorld interface {
	SetCreatureClass(model.CreatureID, int) (model.Creature, error)
}

type classChangeTagWorld interface {
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
}

type classChangeCombatStatsRecalculateWorld interface {
	RecalculateCreatureCombatStats(model.CreatureID) (model.Creature, error)
}

type classChangeTHACORecalculateWorld interface {
	RecalculateCreatureTHACO(model.CreatureID) (model.Creature, error)
}

type classChangeFamilyUpdateWorld interface {
	UpdateFamilyMemberAfterClassChange(name string, class int, dailyExpndMax int) error
}

type classChangeDBRootWorld interface {
	DBRoot() string
}

type classChangeFamilyListWorld interface {
	Families() []model.Family
}

type classChangeFamilyMembersUpdateWorld interface {
	UpdateFamilyMembers(int, []model.FamilyMember) error
}

type FamilyMemberClassChangePersister func(root string, familyID int, familyName string, memberName string, classID int) ([]model.FamilyMember, error)

var (
	classChangeFamilyMemberPersisterMu sync.RWMutex
	classChangeFamilyMemberPersister   FamilyMemberClassChangePersister
)

func RegisterFamilyMemberClassChangePersister(persister FamilyMemberClassChangePersister) func() {
	classChangeFamilyMemberPersisterMu.Lock()
	previous := classChangeFamilyMemberPersister
	classChangeFamilyMemberPersister = persister
	classChangeFamilyMemberPersisterMu.Unlock()
	return func() {
		classChangeFamilyMemberPersisterMu.Lock()
		classChangeFamilyMemberPersister = previous
		classChangeFamilyMemberPersisterMu.Unlock()
	}
}

func currentFamilyMemberClassChangePersister() FamilyMemberClassChangePersister {
	classChangeFamilyMemberPersisterMu.RLock()
	defer classChangeFamilyMemberPersisterMu.RUnlock()
	return classChangeFamilyMemberPersister
}

type WeaponForgeMode string

type WeaponForgeRequest struct {
	Mode         WeaponForgeMode
	PlayerID     model.PlayerID
	CreatureID   model.CreatureID
	RoomID       model.RoomID
	Room         model.Room
	CurrentClass int
}

type WeaponForgeStarter interface {
	BeginWeaponForge(*Context, WeaponForgeRequest) (Status, error)
}

type ClassChangeRequest struct {
	PlayerID     model.PlayerID
	CreatureID   model.CreatureID
	RoomID       model.RoomID
	Room         model.Room
	CurrentClass int
	TargetClass  int
	Experience   int
}

type ClassChangeStarter interface {
	BeginClassChange(*Context, ClassChangeRequest) (Status, error)
}

func NewForgeHandler(world CraftingClassWorld, starters ...WeaponForgeStarter) Handler {
	return newWeaponForgeHandler(world, WeaponForgeModeForge, firstWeaponForgeStarter(starters))
}

func NewNewForgeHandler(world CraftingClassWorld, starters ...WeaponForgeStarter) Handler {
	return newWeaponForgeHandler(world, WeaponForgeModeNewForge, firstWeaponForgeStarter(starters))
}

func NewChangeClassHandler(world CraftingClassWorld, starters ...ClassChangeStarter) Handler {
	starter := firstClassChangeStarter(starters)
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		player, creature, room, err := currentCraftingClassActor(world, ctx)
		if err != nil {
			return StatusDefault, err
		}
		if trainActorHasAnyFlag(player, creature, "PBLIND", "blind", "blinded") {
			ctx.WriteString("당신은 눈이 멀어 직업전환을 할 수 없습니다!\n")
			return StatusDefault, nil
		}
		if !roomHasAnyFlag(room, "train", "rtrain", "training") {
			ctx.WriteString("이 곳은 수련장이 아닙니다!\n")
			return StatusDefault, nil
		}

		targetClass := changeClassRoomTarget(room)
		currentClass := creatureClass(creature)
		if currentClass > model.ClassThief {
			ctx.WriteString("당신은 직업전환을 할 수 없는 직업을 갖고 있습니다.\n")
			return StatusDefault, nil
		}
		if targetClass == currentClass {
			ctx.WriteString("직업전환을 하려면 자신이 수련하는곳에서는 할 수 없습니다.\n")
			return StatusDefault, nil
		}
		experience := creatureStat(creature, "experience")
		if experience < 100000 {
			ctx.WriteString("직업전환을 하려면 경험치 10만이 필요합니다.\n")
			return StatusDefault, nil
		}

		request := ClassChangeRequest{
			PlayerID:     player.ID,
			CreatureID:   creature.ID,
			RoomID:       room.ID,
			Room:         room,
			CurrentClass: currentClass,
			TargetClass:  targetClass,
			Experience:   experience,
		}
		if starter != nil {
			return starter.BeginClassChange(ctx, request)
		}

		state := &classChangePromptState{world: world, request: request}
		return state.prompt(ctx)
	}
}

func firstWeaponForgeStarter(starters []WeaponForgeStarter) WeaponForgeStarter {
	if len(starters) == 0 {
		return nil
	}
	return starters[0]
}

func firstClassChangeStarter(starters []ClassChangeStarter) ClassChangeStarter {
	if len(starters) == 0 {
		return nil
	}
	return starters[0]
}

func newWeaponForgeHandler(world CraftingClassWorld, mode WeaponForgeMode, starter WeaponForgeStarter) Handler {
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		player, creature, room, err := currentCraftingClassActor(world, ctx)
		if err != nil {
			return StatusDefault, err
		}
		if !roomHasAnyFlag(room, "forge", "rforge", "RFORGE") {
			ctx.WriteString("여기는 대장간이 아닙니다.\n")
			return StatusDefault, nil
		}
		if mode == WeaponForgeModeNewForge && legacyRoomNumber(room) != legacyNewForgeRoomNumber {
			ctx.WriteString("여기서는 무기를 만들 수가 없습니다.\n")
			return StatusDefault, nil
		}

		request := WeaponForgeRequest{
			Mode:         mode,
			PlayerID:     player.ID,
			CreatureID:   creature.ID,
			RoomID:       room.ID,
			Room:         room,
			CurrentClass: creatureClass(creature),
		}
		if starter != nil {
			return starter.BeginWeaponForge(ctx, request)
		}

		state := &weaponForgePromptState{world: world, request: request}
		return state.promptWeaponType(ctx)
	}
}

func currentCraftingClassActor(world CraftingClassWorld, ctx *Context) (model.Player, model.Creature, model.Room, error) {
	playerID := InventoryPlayerIDFromContext(ctx)
	if playerID.IsZero() {
		return model.Player{}, model.Creature{}, model.Room{}, ErrInventoryActorRequired
	}
	player, creature, err := CurrentInventoryCreature(world, playerID)
	if err != nil {
		return model.Player{}, model.Creature{}, model.Room{}, err
	}
	roomID := trainActorRoomID(player, creature)
	room, ok := world.Room(roomID)
	if !ok {
		return model.Player{}, model.Creature{}, model.Room{}, fmt.Errorf("crafting/class actor %q room %q not found", creature.ID, roomID)
	}
	return player, creature, room, nil
}

func changeClassRoomTarget(room model.Room) int {
	target := 0
	for _, bit := range []int{4, 5, 6} {
		target *= 2
		if trainRoomTrainingBit(room, bit) {
			target |= 1
		}
	}
	return target + 1
}

func legacyRoomNumber(room model.Room) int {
	for _, key := range []string{"roomNumber", "legacyRoomNumber", "rom_num"} {
		if value, ok := parseCraftingClassInt(room.Properties[key]); ok {
			return value
		}
	}
	raw := strings.TrimSpace(string(room.ID))
	raw = strings.TrimPrefix(raw, "room:")
	if raw == "" {
		return 0
	}
	value, _ := strconv.Atoi(raw)
	return value
}

func parseCraftingClassInt(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	value, err := strconv.Atoi(raw)
	return value, err == nil
}

type weaponForgePromptState struct {
	world                           CraftingClassWorld
	request                         WeaponForgeRequest
	legacyNumber                    int
	prototypeID                     model.PrototypeID
	name                            string
	cost                            int
	nDice                           int
	sDice                           int
	pDice                           int
	shots                           int
	restrictToMundaneFighterClasses bool
}

func (s *weaponForgePromptState) promptWeaponType(ctx *Context) (Status, error) {
	ctx.WriteString("\n\n")
	ctx.WriteString("어떤 종류의 무기를 원하십니까?\n1. 도 2. 검 3. 봉 4. 창 5. 궁 ?\n:")
	if !SetPendingLineHandler(ctx, s.handleWeaponType) {
		ctx.WriteString("현재 세션에서는 대화형 제련을 진행할 수 없습니다.\n")
		return StatusDefault, nil
	}
	return StatusDoPrompt, nil
}

func (s *weaponForgePromptState) handleWeaponType(ctx *Context, line string) (Status, error) {
	choice := firstChoiceDigit(line)
	if choice < 1 || choice > 5 {
		ctx.WriteString("하나를 선택하시오: ")
		SetPendingLineHandler(ctx, s.handleWeaponType)
		return StatusDoPrompt, nil
	}

	s.legacyNumber = 899 + choice
	s.prototypeID = legacyWeaponForgePrototypeID(choice)
	if _, ok := s.world.ObjectPrototype(s.prototypeID); !ok {
		ClearPendingLineHandler(ctx)
		ctx.WriteString(fmt.Sprintf("Error (%d)\n", s.legacyNumber))
		return StatusDefault, nil
	}

	if s.request.Mode == WeaponForgeModeNewForge {
		ctx.WriteString("\n타격치에 영향을 주는 재료를 선택하십시요.\n")
		ctx.WriteString("1.에메랄드 100만냥 2.티타늄 200만냥  3.일루션 300만냥\n:")
	} else {
		ctx.WriteString("\n타격치에 영향을 주는 재료를 선택하십시요.\n")
		ctx.WriteString("1.강 철 오만냥 2.귀금속 이십만냥 3.금강석 삼십만냥\n:")
	}
	SetPendingLineHandler(ctx, s.handleMaterial)
	return StatusDoPrompt, nil
}

func (s *weaponForgePromptState) handleMaterial(ctx *Context, line string) (Status, error) {
	choice := firstChoiceDigit(line)
	if s.request.Mode == WeaponForgeModeNewForge {
		switch choice {
		case 1:
			s.nDice, s.sDice, s.pDice, s.cost = 4, 5, 3, 1000000
		case 2:
			s.nDice, s.sDice, s.pDice, s.cost = 4, 6, 3, 2000000
		case 3:
			s.nDice, s.sDice, s.pDice, s.cost = 4, 7, 3, 3000000
		default:
			ctx.WriteString("하나를 선택하시오: ")
			SetPendingLineHandler(ctx, s.handleMaterial)
			return StatusDoPrompt, nil
		}
	} else {
		switch choice {
		case 1:
			s.sDice, s.cost = 3, 50000
		case 2:
			s.sDice, s.cost = 4, 200000
		case 3:
			if classCannotUseDiamondForgeMaterial(s.request.CurrentClass) {
				ctx.WriteString("당신은 이런 무기를 사용할 능력이 없습니다.\n")
				ctx.WriteString("다른재료를 선택하십시요\n")
				SetPendingLineHandler(ctx, s.handleMaterial)
				return StatusDoPrompt, nil
			}
			s.sDice, s.cost = 5, 300000
			s.restrictToMundaneFighterClasses = true
		default:
			ctx.WriteString("하나를 선택하시오: ")
			SetPendingLineHandler(ctx, s.handleMaterial)
			return StatusDoPrompt, nil
		}
	}

	if s.request.Mode == WeaponForgeModeNewForge {
		ctx.WriteString("\n훌륭한  무기는  담금질로 내구력이  향상됩니다.\n몇번의  담금질을 원합니까?\n")
		ctx.WriteString("1. 100번  백만냥 2. 300번  2백만냥 3. 500번  3백만냥\n4. 700번 4백만냥 5. 900번 5백만냥\n")
	} else {
		ctx.WriteString("\n훌륭한  무기는  담금질로 내구력이  향상됩니다.\n몇번의  담금질을 원합니까?\n")
		ctx.WriteString("1. 100번 오만냥 2. 200번 이십만냥  3. 300번 오십만냥\n4. 400번 백만냥 5. 500번 이백만냥\n")
	}
	ctx.WriteString(": ")
	SetPendingLineHandler(ctx, s.handleTempering)
	return StatusDoPrompt, nil
}

func (s *weaponForgePromptState) handleTempering(ctx *Context, line string) (Status, error) {
	choice := firstChoiceDigit(line)
	if s.request.Mode == WeaponForgeModeNewForge {
		switch choice {
		case 1:
			s.shots, s.cost = 100, s.cost+1000000
		case 2:
			s.shots, s.cost = 300, s.cost+2000000
		case 3:
			s.shots, s.cost = 500, s.cost+3000000
		case 4:
			s.shots, s.cost = 700, s.cost+4000000
		case 5:
			s.shots, s.cost = 900, s.cost+5000000
		default:
			ctx.WriteString("하나를 선택하시오: ")
			SetPendingLineHandler(ctx, s.handleTempering)
			return StatusDoPrompt, nil
		}
	} else {
		switch choice {
		case 1:
			s.shots, s.cost = 100, s.cost+50000
		case 2:
			s.shots, s.cost = 200, s.cost+200000
		case 3:
			s.shots, s.cost = 300, s.cost+500000
		case 4:
			s.shots, s.cost = 400, s.cost+1000000
		case 5:
			s.shots, s.cost = 500, s.cost+2000000
		default:
			ctx.WriteString("하나를 선택하시오: ")
			SetPendingLineHandler(ctx, s.handleTempering)
			return StatusDoPrompt, nil
		}
	}

	ctx.WriteString("\n당신의 무기의 이름을 지으십시요.\n")
	ctx.WriteString("나중에 이름을 고칠수 없으니 조심해서 지으셔야 합니다\n")
	ctx.WriteString(": ")
	SetPendingLineHandler(ctx, s.handleName)
	return StatusDoPrompt, nil
}

func (s *weaponForgePromptState) handleName(ctx *Context, line string) (Status, error) {
	name := line
	if strings.ContainsAny(name, "()") {
		ctx.WriteString("이름에는 괄호가 들어갈수 없습니다.\n이름을 다시 넣으십시요:")
		SetPendingLineHandler(ctx, s.handleName)
		return StatusDoPrompt, nil
	}
	nameLen := legacyForgeNameByteLen(name)
	if nameLen > 20 {
		ctx.WriteString("입력된 이름이 너무  깁니다.\n이름을 다시 넣으십시요(3자이상 20자이하): ")
		SetPendingLineHandler(ctx, s.handleName)
		return StatusDoPrompt, nil
	}
	if nameLen < 3 {
		ctx.WriteString("입력된 이름이 너무  짧습니다.\n이름을 다시 넣으십시요(3자이상 20자이하): ")
		SetPendingLineHandler(ctx, s.handleName)
		return StatusDoPrompt, nil
	}

	s.name = name
	ctx.WriteString("\n모든것에 만족하십니까? (예/아니오)\n")
	ctx.WriteString(": ")
	SetPendingLineHandler(ctx, s.handleConfirm)
	return StatusDoPrompt, nil
}

func (s *weaponForgePromptState) handleConfirm(ctx *Context, line string) (Status, error) {
	ClearPendingLineHandler(ctx)
	if !legacyYes(line) {
		ctx.WriteString("무기 제련을 취소하였습니다.")
		return StatusDefault, nil
	}

	_, creature, err := CurrentInventoryCreature(s.world, s.request.PlayerID)
	if err != nil {
		return StatusDefault, err
	}
	if creatureStat(creature, "gold") < s.cost {
		ctx.WriteString("\n당신은 그 조건의 무기를  만들 만한 돈이 없습니다.\n")
		ctx.WriteString("주인이 당신에게 \"외상은 안되요~\"라고 말합니다.")
		return StatusDefault, nil
	}

	mutator, ok := s.world.(weaponForgeMutationWorld)
	if !ok {
		ctx.WriteString("주인이 말합니다: \"제작 절차는 끝났지만 현재 세계 상태에 무기를 반영할 수 없습니다.\"\n")
		return StatusDefault, nil
	}
	objectID, err := mutator.CreateObjectFromPrototype(s.prototypeID, creature.ID)
	if err != nil {
		return StatusDefault, err
	}
	if _, err := mutator.SetObjectDisplayName(objectID, s.name); err != nil {
		return StatusDefault, err
	}
	if err := s.applyObjectProperties(mutator, objectID); err != nil {
		return StatusDefault, err
	}
	if tagger, ok := s.world.(weaponForgeTagWorld); ok && s.restrictToMundaneFighterClasses {
		if _, err := tagger.UpdateObjectTags(objectID, []string{"OCLSEL", "OASSNO", "OBARBO", "OFIGHO", "ORNGRO", "OTHIEO"}, nil); err != nil {
			return StatusDefault, err
		}
	}
	if err := mutator.SetCreatureStat(creature.ID, "gold", creatureStat(creature, "gold")-s.cost); err != nil {
		return StatusDefault, err
	}

	ctx.WriteString("\n주인이 당신에게 새로 제작된 무기를 건네줍니다.")
	_ = roomBroadcast(ctx, s.request.RoomID, fmt.Sprintf("\n%s이   무기를  제련하였습니다.", creature.DisplayName))
	return StatusDefault, nil
}

func (s *weaponForgePromptState) applyObjectProperties(world weaponForgeMutationWorld, objectID model.ObjectInstanceID) error {
	props := map[string]int{
		"sDice":        s.sDice,
		"shotsMax":     s.shots,
		"shotsCurrent": s.shots,
	}
	if s.nDice > 0 {
		props["nDice"] = s.nDice
	}
	if s.pDice > 0 {
		props["pDice"] = s.pDice
	}
	for key, value := range props {
		if _, err := world.SetObjectProperty(objectID, key, strconv.Itoa(value)); err != nil {
			return err
		}
	}
	return nil
}

func classCannotUseDiamondForgeMaterial(class int) bool {
	return class == model.ClassCleric || class == model.ClassPaladin || class == model.ClassMage
}

func legacyWeaponForgePrototypeID(choice int) model.PrototypeID {
	return model.PrototypeID(fmt.Sprintf("object:o09:%d", choice-1))
}

type classChangePromptState struct {
	world   CraftingClassWorld
	request ClassChangeRequest
}

func (s *classChangePromptState) prompt(ctx *Context) (Status, error) {
	ctx.WriteString("직업전환을 하려면 경험치 10만이 필요합니다.\n")
	ctx.WriteString("정말로 직업전환을 하시겠습니까?(예/아니오): ")
	if !SetPendingLineHandler(ctx, s.handleConfirm) {
		ctx.WriteString("현재 세션에서는 대화형 직업전환을 진행할 수 없습니다.\n")
		return StatusDefault, nil
	}
	return StatusDoPrompt, nil
}

func (s *classChangePromptState) handleConfirm(ctx *Context, line string) (Status, error) {
	ClearPendingLineHandler(ctx)
	if !legacyYes(line) {
		ctx.WriteString("직업전환이 되지 않았습니다")
		return StatusDefault, nil
	}
	mutator, ok := s.world.(classChangeMutationWorld)
	if !ok {
		ctx.WriteString("사범이 말합니다: \"직업전환 절차는 준비되었지만 현재 세계 상태에 반영할 수 없습니다.\"\n")
		return StatusDefault, nil
	}
	if err := applyClassChangeMutation(mutator, s.world, s.request); err != nil {
		return StatusDefault, err
	}
	ctx.WriteString("\n당신의 직업이 전환되었습니다.")
	return StatusDefault, nil
}

func applyClassChangeMutation(mutator classChangeMutationWorld, world CraftingClassWorld, request ClassChangeRequest) error {
	player, creature, err := CurrentInventoryCreature(world, request.PlayerID)
	if err != nil {
		return err
	}
	nextExperience := creatureStat(creature, "experience") - 100000
	if nextExperience < 0 {
		nextExperience = 0
	}
	if err := mutator.SetCreatureStat(creature.ID, "experience", nextExperience); err != nil {
		return err
	}
	if setter, ok := world.(classChangeClassSetterWorld); ok {
		if _, err := setter.SetCreatureClass(creature.ID, request.TargetClass); err != nil {
			return err
		}
	} else {
		if err := mutator.SetCreatureStat(creature.ID, "class", request.TargetClass); err != nil {
			return err
		}
	}
	if err := applyClassChangeLevelDown(mutator, world, player, creature, request.TargetClass, nextExperience); err != nil {
		return err
	}
	return applyClassChangeLegacyEffects(world, player, creature, request.TargetClass)
}

func applyClassChangeLevelDown(mutator classChangeMutationWorld, world CraftingClassWorld, player model.Player, creature model.Creature, class int, experience int) error {
	level := trainCreatureLevel(creature)
	targetLevel := legacyExperienceToLevel(experience)
	if level <= targetLevel {
		return nil
	}

	hpMax := creatureStat(creature, "hpMax")
	mpMax := creatureStat(creature, "mpMax")
	hpCurrent := creatureStat(creature, "hpCurrent")
	mpCurrent := creatureStat(creature, "mpCurrent")
	pDice := creatureStat(creature, "pDice")
	stats := map[string]int{}
	for key, value := range creature.Stats {
		stats[key] = value
	}
	bonuses := legacyClassStatBonusesFor(class)
	upDamageCleared := false

	for level > targetLevel {
		level--
		if !upDamageCleared && trainActorHasAnyFlag(player, creature, "PUPDMG", "upDamage", "upDmg") {
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
			remove := []string{"PUPDMG", "upDamage", "upDmg"}
			if tagger, ok := world.(trainTagWorld); ok {
				if _, err := tagger.UpdateCreatureTags(creature.ID, nil, remove); err != nil {
					return err
				}
				if !player.ID.IsZero() {
					if _, err := tagger.UpdatePlayerTags(player.ID, nil, remove); err != nil {
						return err
					}
				}
			} else if tagger, ok := world.(classChangeTagWorld); ok {
				if _, err := tagger.UpdateCreatureTags(creature.ID, nil, remove); err != nil {
					return err
				}
			}
		}
		if (level-1)%2 != 0 {
			hpMax -= bonuses.hp
		} else {
			mpMax -= bonuses.mp
		}
		hpCurrent = hpMax
		mpCurrent = mpMax

		if (level+1)%4 == 0 {
			idx := (level - 1) % 10
			if idx < 0 {
				idx += 10
			}
			if statName := legacyStatName(legacyLevelCycleFor(class)[idx]); statName != "" {
				stats[statName] = stats[statName] - 1
			}
		}
	}

	if _, err := mutator.SetCreatureLevel(creature.ID, targetLevel); err != nil {
		return err
	}
	for key, value := range map[string]int{
		"hpMax":        hpMax,
		"mpMax":        mpMax,
		"hpCurrent":    hpCurrent,
		"mpCurrent":    mpCurrent,
		"pDice":        pDice,
		"strength":     stats["strength"],
		"dexterity":    stats["dexterity"],
		"constitution": stats["constitution"],
		"intelligence": stats["intelligence"],
		"piety":        stats["piety"],
	} {
		if err := mutator.SetCreatureStat(creature.ID, key, value); err != nil {
			return err
		}
	}
	return nil
}

func legacyExperienceToLevel(experience int) int {
	level := 1
	for level < legacyMaxAutoLevel && level-1 < len(legacyNeededExperience) && experience >= legacyNeededExperience[level-1] {
		level++
	}
	if level >= legacyMaxAutoLevel {
		base := legacyNeededExperience[legacyMaxAutoLevel-2]
		level = (experience-base)/5000000 + legacyMaxAutoLevel
	}
	if level < 1 {
		return 1
	}
	return level
}

func firstChoiceDigit(line string) int {
	if line == "" {
		return 0
	}
	switch line[0] {
	case '1', '2', '3', '4', '5':
		return int(line[0] - '0')
	default:
		return 0
	}
}

func legacyYes(line string) bool {
	return strings.HasPrefix(line, "예")
}

func legacyForgeNameByteLen(name string) int {
	encoded, err := legacykr.EncodeEUCKR(name)
	if err != nil {
		return len([]byte(name))
	}
	return len(encoded)
}

func applyClassChangeLegacyEffects(world CraftingClassWorld, player model.Player, creature model.Creature, targetClass int) error {
	if hook, ok := world.(classChangeCombatStatsRecalculateWorld); ok {
		if _, err := hook.RecalculateCreatureCombatStats(creature.ID); err != nil {
			return err
		}
	} else if hook, ok := world.(classChangeTHACORecalculateWorld); ok {
		if _, err := hook.RecalculateCreatureTHACO(creature.ID); err != nil {
			return err
		}
	}
	if !trainActorHasAnyFlag(player, creature, "PFAMIL", "familyFlag") {
		return nil
	}
	name := classChangeActorName(player, creature)
	familyID := classChangeDailyExpndMax(creature)
	if err := persistClassChangeFamilyMember(world, name, targetClass, familyID); err != nil {
		return err
	}
	hook, ok := world.(classChangeFamilyUpdateWorld)
	if !ok {
		return nil
	}
	return hook.UpdateFamilyMemberAfterClassChange(name, targetClass, familyID)
}

func classChangeActorName(player model.Player, creature model.Creature) string {
	if name := strings.TrimSpace(creature.DisplayName); name != "" {
		return name
	}
	if name := strings.TrimSpace(player.DisplayName); name != "" {
		return name
	}
	if !player.ID.IsZero() {
		return string(player.ID)
	}
	return string(creature.ID)
}

func classChangeDailyExpndMax(creature model.Creature) int {
	for _, key := range []string{"dailyExpndMax", "legacyDailyExpndMax", "daily_expnd_max", "familyID", "family_id"} {
		if value, ok := creature.Stats[key]; ok {
			return value
		}
		if value, ok := parseCraftingClassInt(creature.Properties[key]); ok {
			return value
		}
	}
	return 0
}

func persistClassChangeFamilyMember(world CraftingClassWorld, memberName string, classID int, familyID int) error {
	if familyID <= 0 {
		return nil
	}
	persister := currentFamilyMemberClassChangePersister()
	if persister == nil {
		return nil
	}
	rooted, ok := world.(classChangeDBRootWorld)
	if !ok {
		return nil
	}
	root := strings.TrimSpace(rooted.DBRoot())
	if root == "" {
		return nil
	}
	members, err := persister(root, familyID, classChangeFamilyName(world, familyID), memberName, classID)
	if err != nil {
		return err
	}
	if members == nil {
		return nil
	}
	if !classChangeFamilyMembersContain(members, memberName) {
		return nil
	}
	updater, ok := world.(classChangeFamilyMembersUpdateWorld)
	if !ok {
		return nil
	}
	return updater.UpdateFamilyMembers(familyID, members)
}

func classChangeFamilyMembersContain(members []model.FamilyMember, memberName string) bool {
	for _, member := range members {
		if strings.TrimSpace(member.DisplayName) == strings.TrimSpace(memberName) {
			return true
		}
	}
	return false
}

func classChangeFamilyName(world CraftingClassWorld, familyID int) string {
	lister, ok := world.(classChangeFamilyListWorld)
	if !ok {
		return ""
	}
	for _, family := range lister.Families() {
		if family.ID == familyID || family.Slot == familyID {
			return strings.TrimSpace(family.DisplayName)
		}
	}
	return ""
}
