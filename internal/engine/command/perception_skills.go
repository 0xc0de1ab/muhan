package command

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const (
	redEyeCooldownKey        = "red_eye"
	thiefStatCooldownKey     = "thief_stat"
	redEyeMinimumCooldown    = int64(1)
	thiefStatMinimumCooldown = int64(1)
)

type RedEyeWorld interface {
	LookWorld
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
	RecordCreatureDamage(model.CreatureID, model.CreatureID, int) error
	FinalizeMonsterDeath(model.CreatureID) (bool, error)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
}

type ThiefStatWorld interface {
	LookWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
}

func NewRedEyeHandler(world RedEyeWorld, roll SearchRollFunc) Handler {
	return NewRedEyeHandlerWithDeathFinalizer(world, roll, nil)
}

func NewRedEyeHandlerWithDeathFinalizer(world RedEyeWorld, roll SearchRollFunc, finalizer AttackDeathFinalizer) Handler {
	if roll == nil {
		roll = perceptionDefaultRoll
	}
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("red_eye: actor creature %q not found", viewer.CreatureID)
		}

		if message := redEyeClassRejection(actor); message != "" {
			ctx.WriteString(message)
			return StatusDefault, nil
		}
		if len(resolved.Args) < 2 {
			ctx.WriteString("사용법: 혈마안 <대상> <괴물>\n")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, redEyeCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		targetPlayer, targetCreature, ok := findRedEyeTarget(world, getArg(resolved, 0))
		if !ok || redEyeTargetHiddenFromActor(actor, targetPlayer, targetCreature) {
			ctx.WriteString("그런 사람은 존재하지 않습니다.\n")
			return StatusDefault, nil
		}
		if checkGroupState(ctx, redEyeGroupActorID(viewer.PlayerID, actor)) {
			ctx.WriteString("그룹원들에게는 혈마를 할 수 없습니다.\n")
			return StatusDefault, nil
		}
		if checkGroupState(ctx, redEyeGroupTargetID(targetPlayer, targetCreature)) {
			ctx.WriteString("상대방이 그룹이 있네요. 혈마를 할 수 없어요!\n")
			return StatusDefault, nil
		}
		targetRoom, ok := world.Room(targetCreature.RoomID)
		if !ok {
			return StatusDefault, fmt.Errorf("red_eye: target room %q not found", targetCreature.RoomID)
		}
		if err := revealPerceptionActor(ctx, world, room.ID, viewer, actor, true); err != nil {
			return StatusDefault, err
		}

		enemyName := getArg(resolved, 1)
		enemyOrdinal := getOrdinal(resolved, 1)
		enemy, ok := findAttackCreatureTarget(world, targetRoom, viewer, enemyName, enemyOrdinal)
		if !ok {
			ctx.WriteString(redEyeTargetName(targetPlayer, targetCreature) + "의 방에 그런 괴물은 없습니다.\n")
			return StatusDefault, nil
		}
		if targetCreature.RoomID == room.ID {
			ctx.WriteString("같은 방에 있는 사람에게는 혈마안을 사용할 수 없습니다.\n")
			return StatusDefault, nil
		}
		if attackCreatureProtected(enemy) {
			ctx.WriteString("그 상대는 공격할 수 없습니다.\n")
			return StatusDefault, nil
		}
		if creatureStat(enemy, "hpCurrent") <= 0 {
			name := attackCreatureName(enemy)
			ctx.WriteString(name + krtext.Particle(name, '1') + " 이미 쓰러져 있습니다.\n")
			return StatusDefault, nil
		}

		actorName := attackCreatureName(actor)
		enemyName = attackCreatureName(enemy)
		_ = roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 혈마안을 사용합니다.\n")
		perceptionAddEnemy(world, enemy.ID, actor.ID)
		if roll(1, 22) > redEyeChance(actor, targetCreature) {
			if err := redEyeApplyBacklash(world, actor); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("혈마안이 빗나갔습니다.\n")
			_ = roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '0')+" 혈마안에 실패했습니다.\n")
			return StatusDefault, world.SetCreatureCooldown(actor.ID, redEyeCooldownKey, now, redEyeCooldownSeconds(actor))
		}

		damage := redEyeDamage(actor, targetCreature, enemy, roll)
		_, applied, dead, err := world.ApplyCreatureDamage(enemy.ID, damage)
		if err != nil {
			return StatusDefault, err
		}
		if err := recordRedEyeDamage(world, actor, targetCreature, enemy.ID, applied); err != nil {
			return StatusDefault, err
		}
		if targetCreature.ID != actor.ID {
			perceptionAddEnemy(world, enemy.ID, targetCreature.ID)
		}

		ctx.WriteString(fmt.Sprintf("혈마안이 %s에게 %d만큼의 피해를 주었습니다.\n", enemyName, applied))
		_ = roomBroadcast(ctx, targetRoom.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 혈마안으로 "+enemyName+krtext.Particle(enemyName, '3')+" 공격합니다.\n")
		if dead {
			if err := finalizeMonsterDeathWithOptionalFinalizer(ctx, world, finalizer, actor, enemy); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(enemyName + krtext.Particle(enemyName, '1') + " 쓰러졌습니다.\n")
		}
		return StatusDefault, world.SetCreatureCooldown(actor.ID, redEyeCooldownKey, now, redEyeCooldownSeconds(actor))
	}
}

func NewThiefStatHandler(world ThiefStatWorld, roll SearchRollFunc) Handler {
	if roll == nil {
		roll = perceptionDefaultRoll
	}
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("thief_stat: actor creature %q not found", viewer.CreatureID)
		}

		if message := thiefStatClassRejection(actor); message != "" {
			ctx.WriteString(message)
			return StatusDefault, nil
		}
		if len(resolved.Args) < 1 || getArg(resolved, 0) == "" {
			ctx.WriteString("무엇을 분석하시려구요?\n")
			return StatusDefault, nil
		}
		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, thiefStatCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}
		if attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
			ctx.WriteString("당신은 눈이 멀어 천안술을 펼칠 수 없습니다.\n")
			return StatusDefault, nil
		}
		if attackActorAlreadyFighting(world, room, viewer, actor) {
			ctx.WriteString("싸우는 도중에는 천안술을 펼칠 수 없습니다.")
			return StatusDefault, nil
		}

		if err := revealPerceptionActor(ctx, world, room.ID, viewer, actor, false); err != nil {
			return StatusDefault, err
		}
		if err := world.SetCreatureCooldown(actor.ID, thiefStatCooldownKey, now, thiefStatCooldownSeconds(actor)); err != nil {
			return StatusDefault, err
		}

		target := getArg(resolved, 0)
		owner := actor
		ownerRoom := room
		if ownerArg := getArg(resolved, 1); ownerArg != "" {
			if foundOwner, foundRoom, ok := findThiefStatOwner(world, room, viewer, actor, ownerArg, getOrdinal(resolved, 1)); ok {
				owner = foundOwner
				ownerRoom = foundRoom
			}
		}

		if object, _, ok := findThiefStatObject(world, owner, ownerRoom, target, getOrdinal(resolved, 0), viewerDetectsInvisible(world, viewer)); ok {
			if !thiefStatChanceSucceeds(actor, owner, 30, roll) {
				ctx.WriteString(attackCreatureName(owner) + "의 소지품을 살피는 데 실패했습니다!\n")
				if !attackCreatureIsPlayer(owner) && !attackCreatureProtected(owner) {
					perceptionAddEnemy(world, owner.ID, actor.ID)
				}
				return StatusDefault, nil
			}
			ctx.WriteString(renderThiefStatObject(world, object, actor, roll))
			return StatusDefault, nil
		}

		if creature, ok := findAttackCreatureTarget(world, ownerRoom, viewer, target, getOrdinal(resolved, 0)); ok {
			if attackCreatureProtected(creature) {
				ctx.WriteString(attackCreatureName(creature) + "의 신상정보는 알아낼 수 없습니다.\n")
				return StatusDefault, nil
			}
			if roll(1, 100) > thiefStatChance(actor, creature, 30) && creatureClass(actor) < creatureClass(creature) {
				ctx.WriteString(attackCreatureName(creature) + "의 신상정보를 알아보는 데 실패했습니다!\n")
				if !attackCreatureProtected(creature) {
					perceptionAddEnemy(world, creature.ID, actor.ID)
				}
				return StatusDefault, nil
			}
			ctx.WriteString(renderThiefStatCreature(world, actor, creature, roll))
			return StatusDefault, nil
		}
		if _, ok := findAttackPlayerTarget(world, ownerRoom, viewer, target, getOrdinal(resolved, 0)); ok {
			ctx.WriteString("다른 사람의 신상정보는 알아낼 수 없습니다.\n")
			return StatusDefault, nil
		}

		ctx.WriteString("그것은 없습니다.\n")
		return StatusDefault, nil
	}
}

func perceptionDefaultRoll(min int, max int) int {
	if max <= min {
		return min
	}
	return min + rand.Intn(max-min+1)
}

func redEyeClassRejection(actor model.Creature) string {
	if creatureClass(actor) < model.ClassInvincible {
		return "무적 이상만 사용할 수 있는 기술입니다.\n"
	}
	if !redEyeHasPaladinTraining(actor) {
		return "무사를 무적수련하지 않았습니다..\n"
	}
	return ""
}

func redEyeHasPaladinTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SPALADIN",
		"paladinTraining",
		"paladinSpell",
		"paladinMode",
	)
}

func thiefStatClassRejection(actor model.Creature) string {
	if creatureClass(actor) < model.ClassInvincible {
		return "무적 이상만 사용할 수 있는 기술입니다.\n"
	}
	if !thiefStatHasTraining(actor) {
		return "도둑을 무적수련하지 않았습니다..\n"
	}
	return ""
}

func thiefStatHasTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"STHIEF",
		"thiefTraining",
		"thiefSpell",
		"thiefMode",
	)
}

func redEyeCooldownSeconds(actor model.Creature) int64 {
	seconds := int64(15 - minInt(10, creatureStat(actor, "intelligence")/3))
	if seconds < redEyeMinimumCooldown {
		return redEyeMinimumCooldown
	}
	return seconds
}

func thiefStatCooldownSeconds(actor model.Creature) int64 {
	seconds := int64(20 - minInt(15, creatureStat(actor, "dexterity")/5+creatureStat(actor, "intelligence")/3))
	if seconds < thiefStatMinimumCooldown {
		return thiefStatMinimumCooldown
	}
	return seconds
}

func findRedEyeTarget(world LookWorld, target string) (model.Player, model.Creature, bool) {
	player, ok := findPerceptionPlayerByArgument(world, target)
	if !ok || player.CreatureID.IsZero() {
		return model.Player{}, model.Creature{}, false
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return model.Player{}, model.Creature{}, false
	}
	return player, creature, true
}

func findPerceptionPlayerByArgument(world LookWorld, target string) (model.Player, bool) {
	for _, id := range perceptionPlayerIDCandidates(target) {
		if player, ok := world.Player(id); ok {
			return player, true
		}
	}
	return model.Player{}, false
}

func perceptionPlayerIDCandidates(target string) []model.PlayerID {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	lower := strings.ToLower(target)
	candidates := []model.PlayerID{
		model.PlayerID(target),
		model.PlayerID(lower),
		model.PlayerID("player:" + lower),
	}
	seen := map[model.PlayerID]struct{}{}
	out := make([]model.PlayerID, 0, len(candidates))
	for _, id := range candidates {
		if id.IsZero() {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func redEyeTargetHiddenFromActor(actor model.Creature, targetPlayer model.Player, targetCreature model.Creature) bool {
	if attackCreatureHasFlag(targetCreature, "dmInvisible", "PDMINV", "pdminv") ||
		hasAnyNormalizedFlag(targetPlayer.Metadata.Tags, "dmInvisible", "PDMINV", "pdminv") {
		return true
	}
	if !attackCreatureHasFlag(actor, "detectInvisible", "detectInvis", "PDINVI", "pdinvi") &&
		(attackCreatureHasFlag(targetCreature, "invisible", "PINVIS", "pinvis") ||
			hasAnyNormalizedFlag(targetPlayer.Metadata.Tags, "invisible", "PINVIS", "pinvis")) {
		return true
	}
	return false
}

func redEyeTargetName(player model.Player, creature model.Creature) string {
	if name := cleanDisplayText(creature.DisplayName); name != "" {
		return name
	}
	if name := cleanDisplayText(player.DisplayName); name != "" {
		return name
	}
	if !player.ID.IsZero() {
		return string(player.ID)
	}
	return string(creature.ID)
}

func redEyeGroupActorID(playerID model.PlayerID, creature model.Creature) string {
	if !playerID.IsZero() {
		return string(playerID)
	}
	if !creature.PlayerID.IsZero() {
		return string(creature.PlayerID)
	}
	return string(creature.ID)
}

func redEyeGroupTargetID(player model.Player, creature model.Creature) string {
	if !player.ID.IsZero() {
		return string(player.ID)
	}
	if !creature.PlayerID.IsZero() {
		return string(creature.PlayerID)
	}
	return string(creature.ID)
}

func redEyeChance(actor model.Creature, target model.Creature) int {
	chance := (20 - creatureStat(actor, "thaco")) -
		(20 - creatureStat(target, "thaco")) +
		(attackCreatureLevel(actor)+29)/30 +
		legacyStatBonus(creatureStat(actor, "intelligence"))*3
	if chance > 20 {
		chance = 20
	}
	if chance < 5 {
		chance = 5
	}
	return chance
}

func redEyeDamage(actor model.Creature, target model.Creature, enemy model.Creature, roll SearchRollFunc) int {
	base := redEyeStatsDamage(target, roll)
	if base < 1 {
		base = maxInt(1, attackCreatureLevel(target)/2)
	}
	multiplierMax := (creatureStat(actor, "intelligence") + creatureStat(actor, "piety") + redEyeChance(actor, target)) / 10
	if multiplierMax < 1 {
		multiplierMax = 1
	}
	damage := base * (5 + roll(1, multiplierMax))
	if hp := creatureStat(enemy, "hpCurrent"); hp > 0 && damage > hp {
		damage = hp
	}
	if damage < 1 {
		damage = 1
	}
	return damage * 3
}

func redEyeStatsDamage(creature model.Creature, roll SearchRollFunc) int {
	nDice := creatureStat(creature, "nDice")
	sDice := creatureStat(creature, "sDice")
	pDice := creatureStat(creature, "pDice")
	if nDice < 0 {
		nDice = 0
	}
	if sDice < 0 {
		sDice = 0
	}
	damage := pDice
	if nDice > 0 && sDice > 0 {
		for i := 0; i < nDice; i++ {
			damage += roll(1, sDice)
		}
	}
	return damage
}

func redEyeApplyBacklash(world RedEyeWorld, actor model.Creature) error {
	if hp := creatureStat(actor, "hpCurrent"); hp > 0 {
		if err := world.SetCreatureStat(actor.ID, "hpCurrent", hp-hp/2); err != nil {
			return err
		}
	}
	if mp := creatureStat(actor, "mpCurrent"); mp > 0 {
		if err := world.SetCreatureStat(actor.ID, "mpCurrent", mp-mp/2); err != nil {
			return err
		}
	}
	return nil
}

func recordRedEyeDamage(world RedEyeWorld, actor model.Creature, target model.Creature, enemyID model.CreatureID, applied int) error {
	if applied <= 0 {
		return nil
	}
	actorShare := applied / 3
	if actorShare < 1 {
		actorShare = applied
	}
	if actorShare > applied {
		actorShare = applied
	}
	if err := world.RecordCreatureDamage(enemyID, actor.ID, actorShare); err != nil {
		return err
	}
	targetShare := applied - actorShare
	if targetShare > 0 && !target.ID.IsZero() && target.ID != actor.ID {
		return world.RecordCreatureDamage(enemyID, target.ID, targetShare)
	}
	return nil
}

func perceptionAddEnemy(world interface{}, attacker model.CreatureID, defender model.CreatureID) {
	if attacker.IsZero() || defender.IsZero() {
		return
	}
	if adder, ok := world.(interface {
		AddEnemy(attacker, defender model.CreatureID) (bool, error)
	}); ok {
		_, _ = adder.AddEnemy(attacker, defender)
	}
}

func revealPerceptionActor(
	ctx *Context,
	world interface {
		LookWorld
		UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
		UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
		SetCreatureStat(model.CreatureID, string, int) error
	},
	roomID model.RoomID,
	viewer LookViewer,
	actor model.Creature,
	removeHidden bool,
) error {
	removeTags := []string{"invisible", "pinvis", "PINVIS"}
	statKeys := []string{"PINVIS"}
	if removeHidden {
		removeTags = append(removeTags, "hidden", "phiddn", "PHIDDN")
		statKeys = append(statKeys, "PHIDDN")
	}

	wasInvisible := attackCreatureHasFlag(actor, "invisible", "pinvis", "PINVIS")
	if !viewer.PlayerID.IsZero() {
		if player, ok := world.Player(viewer.PlayerID); ok {
			if hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "pinvis", "PINVIS") {
				wasInvisible = true
			}
			if _, err := world.UpdatePlayerTags(viewer.PlayerID, nil, removeTags); err != nil {
				return err
			}
		}
	}
	if _, err := world.UpdateCreatureTags(actor.ID, nil, removeTags); err != nil {
		return err
	}
	for _, key := range statKeys {
		if actor.Stats[key] != 0 {
			if err := world.SetCreatureStat(actor.ID, key, 0); err != nil {
				return err
			}
		}
	}
	if !wasInvisible {
		return nil
	}
	name := attackCreatureName(actor)
	ctx.WriteString("당신은 모습을 드러냅니다.\n")
	return roomBroadcast(ctx, roomID, "\n"+name+krtext.Particle(name, '1')+" 모습을 드러냅니다.\n")
}

func findThiefStatOwner(
	world LookWorld,
	room model.Room,
	viewer LookViewer,
	actor model.Creature,
	target string,
	ordinal int64,
) (model.Creature, model.Room, bool) {
	if creature, ok := findAttackCreatureTarget(world, room, viewer, target, ordinal); ok {
		return creature, room, true
	}
	if player, ok := findAttackPlayerTarget(world, room, viewer, target, ordinal); ok {
		if creature, ok := attackPlayerCreature(world, player); ok {
			if thiefStatOwnerHiddenLikeLegacy(actor, creature) {
				return model.Creature{}, model.Room{}, false
			}
			return creature, room, true
		}
	}
	player, ok := findPerceptionPlayerByArgument(world, target)
	if !ok || player.RoomID != room.ID {
		return model.Creature{}, model.Room{}, false
	}
	creature, ok := attackPlayerCreature(world, player)
	if !ok {
		return model.Creature{}, model.Room{}, false
	}
	if thiefStatOwnerHiddenLikeLegacy(actor, creature) {
		return model.Creature{}, model.Room{}, false
	}
	return creature, room, true
}

func thiefStatOwnerHiddenLikeLegacy(actor model.Creature, owner model.Creature) bool {
	return creatureClass(actor) < model.ClassDM && attackCreatureHasFlag(owner, "PDMINV", "pdminv", "dmInvisible")
}

func findThiefStatObject(
	world InventoryWorld,
	owner model.Creature,
	room model.Room,
	target string,
	ordinal int64,
	detectInvisible bool,
) (model.ObjectInstance, string, bool) {
	if object, name, ok := findEquipInventoryObjectWithVisibility(world, owner, target, ordinal, detectInvisible); ok {
		return object, name, true
	}
	if object, name, ok := findEquippedObject(world, owner, target, ordinal); ok {
		return object, name, true
	}
	return findGetRoomObject(world, room, target, ordinal)
}

func thiefStatChance(actor model.Creature, target model.Creature, floor int) int {
	chance := 25 + ((attackCreatureLevel(actor)+3)/4)*10 - ((attackCreatureLevel(target)+3)/4)*5
	if chance < floor {
		return floor
	}
	return chance
}

func thiefStatChanceSucceeds(actor model.Creature, target model.Creature, floor int, roll SearchRollFunc) bool {
	return roll(1, 100) <= thiefStatChance(actor, target, floor)
}

func renderThiefStatObject(world InventoryWorld, object model.ObjectInstance, actor model.Creature, roll SearchRollFunc) string {
	var b strings.Builder
	name := objectDisplayName(world, object)
	if name == "" {
		name = string(object.ID)
	}
	fmt.Fprintf(&b, "이름: %s\n", name)
	if description := thiefStatObjectDescription(world, object); description != "" {
		fmt.Fprintf(&b, "설명: %s\n", description)
	}
	if useOutput := objectStringPropertyAny(world, object, "useOutput", "use_output", "effect", "효과"); useOutput != "" {
		fmt.Fprintf(&b, "효과: %s\n", useOutput)
	}

	shotsCurrent, _ := objectFirstIntProperty(world, object, "shotsCurrent", "shotscur", "charges")
	shotsMax, _ := objectFirstIntProperty(world, object, "shotsMax", "shotsmax")
	fmt.Fprintf(&b, "사용회수 %d/%d\n", shotsCurrent, shotsMax)

	legacyType := objectLegacyTypeOrKind(world, object)
	b.WriteString("종류: ")
	if legacyType >= legacyObjectSharp && legacyType <= legacyObjectMissile {
		b.WriteString(legacyWeaponTypeName(legacyType))
		b.WriteString(" 무기.\n")
		if thiefStatKnowledgeRoll(actor, roll, 10) {
			sDice := objectIntPropertyOrZero(world, object, "sDice")
			nDice := objectIntPropertyOrZero(world, object, "nDice")
			pDice := objectIntPropertyOrZero(world, object, "pDice")
			adjustment := objectIntPropertyOrZero(world, object, "adjustment")
			fmt.Fprintf(&b, "타격치: %d면%d굴림 더하기 %d", sDice, nDice, pDice)
			if adjustment != 0 {
				fmt.Fprintf(&b, " (+%d)\n", adjustment)
			} else {
				b.WriteByte('\n')
			}
		}
	} else {
		switch legacyType {
		case legacyObjectArmor:
			b.WriteString("방어구")
			if thiefStatKnowledgeRoll(actor, roll, 10) {
				fmt.Fprintf(&b, "\n방어력: %02d", objectIntPropertyOrZero(world, object, "armor"))
			}
		case legacyObjectPotion:
			b.WriteString("약")
		case legacyObjectScroll:
			b.WriteString("주문서")
		case legacyObjectWand:
			b.WriteString("주문걸린 물건")
		case legacyObjectContainer:
			b.WriteString("담는 종류")
		case legacyObjectKey:
			b.WriteString("열쇠")
		case legacyObjectLightSource:
			b.WriteString("광원")
		default:
			b.WriteString("모르겠음")
		}
		b.WriteByte('\n')
	}

	value, _ := objectFirstIntProperty(world, object, "value", "가격")
	weight, _ := objectFirstIntProperty(world, object, "weight", "무게")
	fmt.Fprintf(&b, "가치: %05d   무게: %02d", value, weight)
	if quest, ok := objectFirstIntProperty(world, object, "questNumber", "questnum"); ok && quest != 0 {
		fmt.Fprintf(&b, " 임무: %d\n", quest)
	} else {
		b.WriteByte('\n')
	}

	b.WriteString("특성: ")
	if traits := objectAppraisalTraits(world, object); len(traits) > 0 {
		b.WriteString(strings.Join(traits, ", "))
		b.WriteString(".\n")
	} else {
		b.WriteString("특성 없음.\n")
	}
	return b.String()
}

func thiefStatObjectDescription(world InventoryWorld, object model.ObjectInstance) string {
	if description := objectStringPropertyAny(world, object, "description", "desc", "설명"); description != "" {
		return description
	}
	if object.PrototypeID.IsZero() {
		return ""
	}
	proto, ok := world.ObjectPrototype(object.PrototypeID)
	if !ok {
		return ""
	}
	return cleanDescriptionText(proto.Description)
}

func objectStringPropertyAny(world InventoryWorld, object model.ObjectInstance, keys ...string) string {
	for _, key := range keys {
		if value, ok := objectStringProperty(world, object, key); ok {
			return cleanDisplayText(value)
		}
	}
	return ""
}

func renderThiefStatCreature(world LookWorld, actor model.Creature, target model.Creature, roll SearchRollFunc) string {
	var b strings.Builder
	name := attackCreatureName(target)
	fmt.Fprintf(&b, "이름: %s\n", name)
	if title := strings.TrimSpace(target.Properties[legacyTitleProperty]); title != "" {
		fmt.Fprintf(&b, "칭호: %s\n", cleanDisplayText(title))
	}
	fmt.Fprintf(&b, "레벨: %d\n", attackCreatureLevel(target))
	if race := thiefStatRaceName(target); race != "" {
		fmt.Fprintf(&b, "종족: %s\n", race)
	}
	fmt.Fprintf(&b, "직업: %s\n", thiefStatClassName(creatureClass(target)))
	fmt.Fprintf(&b, "성향: %s\n", thiefStatAlignment(target))

	if thiefStatKnowledgeRoll(actor, roll, 5) {
		fmt.Fprintf(&b, "체력: %s\n", formatCurrentMax(creatureStat(target, "hpCurrent"), creatureStat(target, "hpMax")))
		fmt.Fprintf(&b, "마력: %s\n", formatCurrentMax(creatureStat(target, "mpCurrent"), creatureStat(target, "mpMax")))
		fmt.Fprintf(&b, "경험치: %d\n", creatureStat(target, "experience"))
		fmt.Fprintf(&b, "금: %d\n", creatureStat(target, "gold"))
	}
	if thiefStatKnowledgeRoll(actor, roll, 10) {
		fmt.Fprintf(&b, "방어: %d\n", 100-creatureStat(target, "armor"))
		fmt.Fprintf(&b, "타격: %d면%d굴림 더하기 %d\n", creatureStat(target, "sDice"), creatureStat(target, "nDice"), creatureStat(target, "pDice"))
	}
	if thiefStatKnowledgeRoll(actor, roll, 15) {
		fmt.Fprintf(&b, "힘: %d  민첩: %d  체질: %d\n", creatureStat(target, "strength"), creatureStat(target, "dexterity"), creatureStat(target, "constitution"))
		fmt.Fprintf(&b, "지능: %d  신앙심: %d  명중: %d\n", creatureStat(target, "intelligence"), creatureStat(target, "piety"), 20-creatureStat(target, "thaco"))
	}

	if labels := activeStatusEffectLabels(thiefStatCreaturePlayer(world, target), target); len(labels) > 0 {
		fmt.Fprintf(&b, "상태: %s\n", strings.Join(labels, ", "))
	}

	if thiefStatChanceSucceeds(actor, target, 0, roll) {
		names := peekInventoryNames(world, actor, target)
		if len(names) == 0 {
			b.WriteString("소지품: 없음\n")
		} else {
			b.WriteString("소지품: ")
			b.WriteString(strings.Join(names, ", "))
			b.WriteByte('\n')
		}
	} else {
		b.WriteString("소지품을 살피는 데 실패했습니다!\n")
	}
	return b.String()
}

func thiefStatKnowledgeRoll(actor model.Creature, roll SearchRollFunc, threshold int) bool {
	maximum := creatureStat(actor, "intelligence")
	if maximum < 1 {
		return false
	}
	return roll(1, maximum) > threshold
}

func thiefStatRaceName(creature model.Creature) string {
	for _, key := range []string{"raceName", "race"} {
		if value := cleanDisplayText(creature.Properties[key]); value != "" {
			return value
		}
	}
	if race := creatureStat(creature, "race"); race != 0 {
		return fmt.Sprintf("%d", race)
	}
	return ""
}

func thiefStatClassName(class int) string {
	switch class {
	case model.ClassAssassin:
		return "자객"
	case model.ClassBarbarian:
		return "권법가"
	case model.ClassCleric:
		return "불제자"
	case model.ClassFighter:
		return "검사"
	case model.ClassMage:
		return "도술사"
	case model.ClassPaladin:
		return "무사"
	case model.ClassRanger:
		return "포졸"
	case model.ClassThief:
		return "도둑"
	case model.ClassInvincible:
		return "무적"
	case model.ClassCaretaker:
		return "관리자"
	case model.ClassBulsa:
		return "불사"
	case model.ClassSubDM:
		return "부운영자"
	case model.ClassDM:
		return "운영자"
	default:
		return fmt.Sprintf("%d", class)
	}
}

func thiefStatAlignment(creature model.Creature) string {
	alignment := creatureStat(creature, "alignment")
	base := "중립"
	switch {
	case alignment < -100:
		base = "악"
	case alignment > 100:
		base = "선"
	}
	if attackCreatureHasFlag(creature, "PCHAOS", "chaos") {
		return "카오스 " + base
	}
	return base
}

func thiefStatCreaturePlayer(world LookWorld, creature model.Creature) model.Player {
	if creature.PlayerID.IsZero() {
		return model.Player{}
	}
	player, _ := world.Player(creature.PlayerID)
	return player
}
