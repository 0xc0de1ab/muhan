package command

import (
	"fmt"
	"time"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	pobackCooldownKey     = "poback"
	lionScreamCooldownKey = "lion_scream"
)

var (
	pobackCharmStatusTags    = []string{"charmed", "MCHARM"}
	pobackBefuddleStatusTags = []string{"befuddled", "MBEFUD"}
)

type PobackWorld interface {
	LookWorld
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
	RecordCreatureDamage(model.CreatureID, model.CreatureID, int) error
	FinalizeMonsterDeath(model.CreatureID) (bool, error)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
	MoveObject(model.ObjectInstanceID, model.ObjectLocation) error
	ConsumeCreatureObjectCharge(model.ObjectInstanceID, model.CreatureID, bool) (model.ObjectInstance, bool, bool, error)
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
}

type LionScreamWorld interface {
	LookWorld
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
	RecordCreatureDamage(model.CreatureID, model.CreatureID, int) error
	FinalizeMonsterDeath(model.CreatureID) (bool, error)
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
}

func NewPobackHandler(world PobackWorld) Handler {
	return NewPobackHandlerWithDeathFinalizer(world, nil)
}

func NewPobackHandlerWithDeathFinalizer(world PobackWorld, finalizer AttackDeathFinalizer) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("poback: actor creature %q not found", viewer.CreatureID)
		}

		if reject := pobackClassRejection(actor); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}
		if len(resolved.Args) < 2 {
			ctx.WriteString("\n사용법 : 어디 몹이름 포박\n")
			return StatusDefault, nil
		}
		if attackCreatureHasFlag(actor, "blind", "pblind", "PBLIND") {
			ctx.WriteString("당신은 눈이 멀어 있습니다!\n")
			return StatusDefault, nil
		}

		weapon, ok := pobackWieldWeapon(world, actor)
		if !ok {
			ctx.WriteString("포박술을 구사하시려면 봉이나 창종류의 무기가 필요합니다.\n")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, pobackCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		exitName := getArg(resolved, 0)
		exit, ok := findLookTargetExitForViewer(world, viewer, room.Exits, exitName, getOrdinal(resolved, 0))
		if !ok {
			ctx.WriteString("\n" + exitName + "쪽으로는 지도가 없습니다.\n")
			return StatusDefault, nil
		}
		if exitHasAnyFlag(exit, "closed", "xclosd", "xclosed") {
			ctx.WriteString("그 출구는 닫혀 있습니다.\n")
			return StatusDefault, nil
		}
		destination, ok := world.Room(exit.ToRoomID)
		if !ok || destination.ID == room.ID {
			ctx.WriteString("지도가 없습니다.\n")
			return StatusDefault, nil
		}
		if roomHasAnyFlag(destination, "onlyMarried", "marriedOnly", "ronmar", "onlyFamily", "familyOnly", "ronfml") {
			ctx.WriteString("그 방은 볼 수가 없습니다.\n")
			return StatusDefault, nil
		}

		actorName := attackCreatureName(actor)
		if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 포박술을 구사하기 위해 정신을 집중합니다.\n"); err != nil {
			return StatusDefault, err
		}

		targetName := getArg(resolved, 1)
		victim, ok := findAttackCreatureTarget(world, destination, viewer, targetName, getOrdinal(resolved, 1))
		if !ok {
			name := cleanDisplayText(destination.DisplayName)
			if name == "" {
				name = string(destination.ID)
			}
			ctx.WriteString("\n" + name + "에 그런 것은 존재하지 않습니다.\n")
			return StatusDefault, nil
		}
		if attackCreatureProtected(victim) {
			ctx.WriteString("당신은 " + pobackCreaturePronoun(victim) + "를 해칠 수 없습니다.\n")
			return StatusDefault, nil
		}
		if victim.Stats == nil || creatureStat(victim, "hpCurrent") <= 0 {
			ctx.WriteString("그 상대는 공격할 수 없습니다.\n")
			return StatusDefault, nil
		}

		advancedCombatPrimeMonsterTarget(world, victim.ID, actor.ID)
		victimName := attackCreatureName(victim)

		ctx.WriteString("\n용투야의 난세에 살생이 들 끓는다.\n")
		ctx.WriteString("이에 전설의 장인 동방천인이 만든 포승줄 파옥쇄가 있으니\n")
		ctx.WriteString("세상에 부수지 못하고 묶어놓지 못하는게 없으며 죽을 때까지 풀리지\n")
		ctx.WriteString("않으니 어느 누가 두려워 하지 않으리요~~~\n")
		ctx.WriteString("\n당신은 파옥쇄를 " + victimName + "의 몸에 재빠르게 휘두릅니다.\n")

		chance := pobackChance(actor, victim)
		if attackRoll(1, 22) > chance {
			if err := pobackApplyFailureStatus(world, victim); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("\n당신의 포박술이 실패했습니다.\n")
			ctx.WriteString("당신은 포박술의 실패로 혼수상태에 빠집니다.\n")
			if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '0')+" 포박술이 실패했습니다.\n"); err != nil {
				return StatusDefault, err
			}
			return StatusDefault, world.SetCreatureCooldown(actor.ID, pobackCooldownKey, now, pobackCooldownSeconds(actor))
		}

		weapon, stopped, err := bashMaybeSpendWield(ctx, world, actor, weapon)
		if err != nil || stopped {
			return StatusDefault, err
		}

		damage := pobackDamage(world, actor, weapon)
		_, applied, dead, err := world.ApplyCreatureDamage(victim.ID, damage)
		if err != nil {
			return StatusDefault, err
		}
		if err := world.RecordCreatureDamage(victim.ID, actor.ID, applied); err != nil {
			return StatusDefault, err
		}
		if err := pobackApplySuccessStatus(world, actor, victim, chance, applied, now); err != nil {
			return StatusDefault, err
		}

		ctx.WriteString(fmt.Sprintf("당신은 포박술로 %s에게 %d점의 피해를 입혔습니다.\n", victimName, applied))
		if err := roomBroadcast(ctx, destination.ID, fmt.Sprintf("\n%s%s 포박술로 %s에게 %d점의 피해를 입혔습니다.\n", actorName, krtext.Particle(actorName, '1'), victimName, applied)); err != nil {
			return StatusDefault, err
		}
		if !dead {
			return StatusDefault, world.SetCreatureCooldown(actor.ID, pobackCooldownKey, now, pobackCooldownSeconds(actor))
		}

		if err := finalizeMonsterDeathWithOptionalFinalizer(ctx, world, finalizer, actor, victim); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("\n당신은 포박술 도중 " + victimName + krtext.Particle(victimName, '3') + " 죽였습니다.\n")
		if err := roomBroadcast(ctx, destination.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 포박술로 "+victimName+krtext.Particle(victimName, '3')+" 죽였습니다.\n"); err != nil {
			return StatusDefault, err
		}
		if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 포박술로 "+exit.Name+"쪽에 있는 "+victimName+krtext.Particle(victimName, '3')+" 죽였습니다.\n"); err != nil {
			return StatusDefault, err
		}
		return StatusDefault, world.SetCreatureCooldown(actor.ID, pobackCooldownKey, now, pobackCooldownSeconds(actor))
	}
}

func NewLionScreamHandler(world LionScreamWorld) Handler {
	return NewLionScreamHandlerWithDeathFinalizer(world, nil)
}

func NewLionScreamHandlerWithDeathFinalizer(world LionScreamWorld, finalizer AttackDeathFinalizer) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("lion_scream: actor creature %q not found", viewer.CreatureID)
		}

		if reject := lionScreamClassRejection(actor); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}
		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, lionScreamCooldownKey, now, 0); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}
		targets := lionScreamTargets(world, room, viewer)
		if len(targets) == 0 {
			ctx.WriteString("이 방에는 당신이 공격할 적이 없습니다.\n")
			return StatusDefault, nil
		}
		advancedCombatPrimeMonsterTargets(world, targets, actor.ID)
		if err := revealKickActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}

		actorName := attackCreatureName(actor)
		ctx.WriteString("\n심후한 공력으로 부터 나온 사자후의 소리가 천지를 진동하니\n")
		ctx.WriteString("어느누가 그 소리를 듣고서 성할수가 있으리오~\n")
		ctx.WriteString("\"내 앞의 모든 적들은 귀에서 피를 흘리며 쓰러지리라~\"\n\n")
		ctx.WriteString("당신은 뱃속에서부터 공력을 끌어올려 사자후를 내지릅니다.\n")

		chance := lionScreamChance(actor, targets)
		if attackRoll(1, 22) > chance {
			if err := lionScreamApplyFatigue(world, actor); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("당신의 사자후가 적의 공력을 이기지 못합니다.\n")
			ctx.WriteString("당신은 약간 피로해짐을 느낍니다.\n")
			if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '0')+" 사자후가 적의 공력을 이기지 못합니다.\n"); err != nil {
				return StatusDefault, err
			}
			return StatusDefault, world.SetCreatureCooldown(actor.ID, lionScreamCooldownKey, now, lionScreamCooldownSeconds(actor))
		}

		count := minInt((chance+1)/3, len(targets))
		for _, victim := range targets[:count] {
			victimName := attackCreatureName(victim)
			damage := lionScreamDamage(actor)
			if lionScreamMagicOnlyGlances(actor, victim) {
				ctx.WriteString("당신의 사자후가 " + victimName + "에게 아무런 상처도 내지 못합니다.\n")
				damage = 1
			}
			_, applied, dead, err := world.ApplyCreatureDamage(victim.ID, damage)
			if err != nil {
				return StatusDefault, err
			}
			if err := world.RecordCreatureDamage(victim.ID, actor.ID, applied); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString(fmt.Sprintf("당신은 사자후를 내질러 %s에게 %d점의 피해를 입혔습니다.\n", victimName, applied))
			if err := roomBroadcast(ctx, room.ID, fmt.Sprintf("\n%s%s 사자후를 내질러 %s에게 %d점의 피해를 입혔습니다.\n", actorName, krtext.Particle(actorName, '1'), victimName, applied)); err != nil {
				return StatusDefault, err
			}
			if !dead {
				continue
			}
			if err := finalizeMonsterDeathWithOptionalFinalizer(ctx, world, finalizer, actor, victim); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("\n당신의 뛰어난 공력으로 " + victimName + krtext.Particle(victimName, '3') + " 죽였습니다.\n")
			if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 사자후로 "+victimName+krtext.Particle(victimName, '3')+" 죽였습니다.\n"); err != nil {
				return StatusDefault, err
			}
		}
		for _, exit := range room.Exits {
			if exit.ToRoomID.IsZero() || exit.ToRoomID == room.ID {
				continue
			}
			if _, ok := world.Room(exit.ToRoomID); !ok {
				continue
			}
			if err := roomBroadcast(ctx, exit.ToRoomID, "\n근처에 있는 "+actorName+krtext.Particle(actorName, '0')+" 사자후가 여기까지 울려퍼집니다.\n"); err != nil {
				return StatusDefault, err
			}
		}
		return StatusDefault, world.SetCreatureCooldown(actor.ID, lionScreamCooldownKey, now, lionScreamCooldownSeconds(actor))
	}
}

func pobackClassRejection(actor model.Creature) string {
	if creatureClass(actor) < model.ClassInvincible {
		return "무적이상만 쓸수 있는 기술입니다.\n"
	}
	if !guardHasRangerTraining(actor) {
		return "아직 포졸을 무적수련하지 않았습니다.\n"
	}
	return ""
}

func pobackWieldWeapon(world InventoryWorld, actor model.Creature) (model.ObjectInstance, bool) {
	weaponID := equippedObjectID(actor, "wield")
	if weaponID.IsZero() {
		return model.ObjectInstance{}, false
	}
	weapon, ok := world.Object(weaponID)
	if !ok {
		return model.ObjectInstance{}, false
	}
	legacyType := objectLegacyType(world, weapon)
	return weapon, legacyType == legacyObjectBlunt || legacyType == legacyObjectPole
}

func pobackCooldownSeconds(actor model.Creature) int64 {
	interval := 15 - minInt(8, creatureStat(actor, "dexterity")/4)
	return int64(maxInt(1, interval))
}

func pobackChance(actor model.Creature, victim model.Creature) int {
	chance := creatureStat(victim, "thaco") - creatureStat(actor, "thaco")
	chance += legacyStatBonus(creatureStat(actor, "intelligence")) * 2
	chance += legacyStatBonus(creatureStat(actor, "dexterity")) * 3
	if chance > 20 {
		return 20
	}
	if chance < 5 {
		return 5
	}
	return chance
}

func pobackDamage(world InventoryWorld, actor model.Creature, weapon model.ObjectInstance) int {
	damage := creatureStat(actor, "dexterity")*2 + objectDamage(world, weapon)
	return normalizeAttackDamage(damage)
}

func pobackApplySuccessStatus(world PobackWorld, actor model.Creature, victim model.Creature, chance int, damage int, nowUnix int64) error {
	tags := make([]string, 0, len(pobackCharmStatusTags)+len(pobackBefuddleStatusTags))
	duration := damage
	attackDuration := chance
	if attackCreatureHasFlag(victim, "PRMAGI", "MRMAGI", "MNOCHA", "resistMagic", "magicResist", "noCharm") {
		duration /= 3
		attackDuration /= 2
	}
	if attackRoll(0, 1) == 1 || (attackCreatureLevel(actor) < attackCreatureLevel(victim) && !attackCreatureHasFlag(victim, "MNOCHA", "noCharm")) {
		tags = append(tags, pobackCharmStatusTags...)
		if err := world.SetCreatureCooldown(victim.ID, "charmed", nowUnix, int64(duration)); err != nil {
			return err
		}
	}
	if attackDuration > 15 {
		tags = append(tags, pobackBefuddleStatusTags...)
		if err := world.SetCreatureCooldown(victim.ID, "befuddled", nowUnix, int64(duration)); err != nil {
			return err
		}
	}
	if err := world.SetCreatureCooldown(victim.ID, "attack", nowUnix, int64(attackDuration)); err != nil {
		return err
	}
	if err := world.SetCreatureCooldown(victim.ID, "spell", nowUnix, int64(damage)); err != nil {
		return err
	}
	if len(tags) == 0 {
		return nil
	}
	_, err := world.UpdateCreatureTags(victim.ID, tags, nil)
	return err
}

func pobackApplyFailureStatus(world PobackWorld, victim model.Creature) error {
	tags := make([]string, 0, len(pobackCharmStatusTags)+len(pobackBefuddleStatusTags))
	tags = append(tags, pobackCharmStatusTags...)
	tags = append(tags, pobackBefuddleStatusTags...)
	_, err := world.UpdateCreatureTags(victim.ID, tags, nil)
	return err
}

func pobackCreaturePronoun(creature model.Creature) string {
	if attackCreatureHasFlag(creature, "male", "MMALES") {
		return "그"
	}
	return "그녀"
}

func lionScreamClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	if class < model.ClassInvincible {
		if class == model.ClassPaladin && attackCreatureLevel(actor) >= 50 {
			return ""
		}
		return "무사 레벨 50이상만 쓸수 있는 기술입니다.\n"
	}
	if !lionScreamHasPaladinTraining(actor) {
		return "아직 무사를 무적수련하지 않았습니다.\n"
	}
	return ""
}

func lionScreamHasPaladinTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SPALADIN",
		"paladinTraining",
		"paladinSpell",
		"paladinMode",
	)
}

func lionScreamTargets(world LookWorld, room model.Room, viewer LookViewer) []model.Creature {
	detectInvisible := viewerDetectsInvisible(world, viewer)
	targets := make([]model.Creature, 0, len(room.CreatureIDs))
	for _, id := range room.CreatureIDs {
		if id.IsZero() || id == viewer.CreatureID {
			continue
		}
		creature, ok := world.Creature(id)
		if !ok || creature.RoomID != room.ID || attackCreatureIsPlayer(creature) || creatureHPDead(creature) {
			continue
		}
		if attackCreatureProtected(creature) || !creatureVisibleInRoomLook(creature, viewer, detectInvisible) {
			continue
		}
		targets = append(targets, creature)
	}
	return targets
}

func lionScreamChance(actor model.Creature, targets []model.Creature) int {
	enmTHACOTotal := 0
	for _, target := range targets {
		enmTHACOTotal += 20 - creatureStat(target, "thaco")
	}
	chance := 20 - creatureStat(actor, "thaco")
	chance += legacyStatBonus(creatureStat(actor, "dexterity")) * 2
	if len(targets) > 0 {
		chance -= enmTHACOTotal / len(targets)
	}
	chance += (attackCreatureLevel(actor) + 29) / 30
	if chance > 20 {
		return 20
	}
	if chance < 5 {
		return 5
	}
	return chance
}

func lionScreamDamage(actor model.Creature) int {
	maxRoll := minInt(30, 20-creatureStat(actor, "thaco"))
	if maxRoll < 1 {
		maxRoll = 1
	}
	damage := 30 - creatureStat(actor, "thaco") + attackRoll(1, maxRoll)
	return normalizeAttackDamage(damage)
}

func lionScreamCooldownSeconds(actor model.Creature) int64 {
	interval := 15 - minInt(7, creatureStat(actor, "piety")/4+creatureStat(actor, "intelligence")/5)
	return int64(maxInt(1, interval))
}

func lionScreamApplyFatigue(world LionScreamWorld, actor model.Creature) error {
	hp := creatureStat(actor, "hpCurrent")
	loss := hp / 10
	if loss < 1 {
		return nil
	}
	next := hp - loss
	if next < 1 {
		next = 1
	}
	return world.SetCreatureStat(actor.ID, "hpCurrent", next)
}

func lionScreamMagicOnlyGlances(actor model.Creature, victim model.Creature) bool {
	if !attackCreatureHasFlag(victim, "magicOnly", "mmgonl", "MMGONL") {
		return false
	}
	if creatureStat(actor, "piety") >= creatureStat(victim, "piety") {
		return false
	}
	return attackRoll(0, 1) == 1
}
