package command

import (
	"fmt"
	"math/rand"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const (
	powerCooldownKey               = "power"
	powerSuccessCooldownSeconds    = int64(600)
	powerFailureCooldownSeconds    = int64(10)
	powerStrengthBonus             = 3
	accurateCooldownKey            = "accurate"
	accurateSuccessCooldownSeconds = int64(700)
	accurateFailureCooldownSeconds = int64(110)
	accurateThacoBonus             = 3
	absorbCooldownKey              = "absorb"
	absorbCooldownSeconds          = int64(20)
)

var (
	powerStatusTags    = []string{"PPOWER", "power"}
	accurateStatusTags = []string{"PSLAYE", "accurate", "slayer"}
)

type EnergySkillBuffWorld interface {
	LookWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
	SetEffectExpiration(model.CreatureID, string, int64)
	RecalculateAC(model.CreatureID) error
	RecalculateTHACO(model.CreatureID) error
}

type AbsorbWorld interface {
	EnergySkillBuffWorld
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
	RecordCreatureDamage(model.CreatureID, model.CreatureID, int) error
	FinalizeMonsterDeath(model.CreatureID) (bool, error)
}

func NewPowerHandler(world EnergySkillBuffWorld, rng SearchRollFunc) Handler {
	rng = energySkillRollFunc(rng)
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, player, err := energySkillActor(world, viewer, "power")
		if err != nil {
			return StatusDefault, err
		}

		if reject := powerClassRejection(actor); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}
		if powerStatusActive(player, actor) {
			ctx.WriteString("당신은 지금 기공집결을 사용중입니다.\n")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		remaining, used, err := world.UseCreatureCooldown(actor.ID, powerCooldownKey, now, 0)
		if err != nil {
			return StatusDefault, err
		}
		if !used {
			ctx.WriteString(renderPowerWait(remaining))
			return StatusDefault, nil
		}

		actorName := attackCreatureName(actor)
		if rng(1, 100) > powerChance(actor) {
			if err := world.SetCreatureCooldown(actor.ID, powerCooldownKey, now, powerFailureCooldownSeconds); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("기공집결이 실패하였습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, actorName+"이 기공집결을 시도합니다.")
		}

		if err := powerApplySuccess(world, player, actor); err != nil {
			return StatusDefault, err
		}
		world.SetEffectExpiration(actor.ID, "PPOWER", now+powerStatusDurationSeconds(actor))
		if err := world.SetCreatureCooldown(actor.ID, powerCooldownKey, now, powerSuccessCooldownSeconds); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("당신은 가부좌를 틀고 기를 모으기 시작합니다.\n온몸으로 기가 퍼져나가는것을 느낍니다.\n")
		return StatusDefault, roomBroadcast(ctx, room.ID, actorName+"이 가부좌를 틀고 앉아 기를 모읍니다.")
	}
}

func NewAccurateHandler(world EnergySkillBuffWorld, rng SearchRollFunc) Handler {
	rng = energySkillRollFunc(rng)
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, player, err := energySkillActor(world, viewer, "accurate")
		if err != nil {
			return StatusDefault, err
		}

		if reject := accurateClassRejection(actor); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}
		if accurateStatusActive(player, actor) {
			ctx.WriteString("당신은 지금 살기충전을 사용중입니다.\n")
			return StatusDefault, nil
		}
		if equippedObjectID(actor, "wield").IsZero() {
			ctx.WriteString("장비하고 있는 무기가 없습니다!\n")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		remaining, used, err := world.UseCreatureCooldown(actor.ID, accurateCooldownKey, now, 0)
		if err != nil {
			return StatusDefault, err
		}
		if !used {
			ctx.WriteString(renderPowerWait(remaining))
			return StatusDefault, nil
		}

		actorName := attackCreatureName(actor)
		if rng(1, 100) > accurateChance(actor) {
			if err := world.SetCreatureCooldown(actor.ID, accurateCooldownKey, now, accurateFailureCooldownSeconds); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("살기충전이 실패하였습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, actorName+"이 그의 무기에 살기충전을 시도합니다.")
		}

		if err := accurateApplySuccess(world, player, actor); err != nil {
			return StatusDefault, err
		}
		world.SetEffectExpiration(actor.ID, "PSLAYE", now+accurateStatusDurationSeconds(actor))
		if err := world.SetCreatureCooldown(actor.ID, accurateCooldownKey, now, accurateSuccessCooldownSeconds); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("당신은 당신의 무기에 피를 먹입니다.\n무기에 살기가 감도는 것을 느낍니다.\n")
		return StatusDefault, roomBroadcast(ctx, room.ID, actorName+"이 그의 무기에 피를 먹입니다.")
	}
}

func NewAbsorbHandler(world AbsorbWorld, rng SearchRollFunc) Handler {
	return NewAbsorbHandlerWithDeathFinalizer(world, rng, nil)
}

func NewAbsorbHandlerWithDeathFinalizer(world AbsorbWorld, rng SearchRollFunc, finalizer AttackDeathFinalizer) Handler {
	rng = energySkillRollFunc(rng)
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, _, err := energySkillActor(world, viewer, "absorb")
		if err != nil {
			return StatusDefault, err
		}

		target, ordinal := lookTarget(resolved)
		if target == "" {
			ctx.WriteString("\n누구에게 주문을 거실려고요?\n")
			return StatusDefault, nil
		}
		if reject := absorbClassRejection(actor); reject != "" {
			ctx.WriteString(reject)
			return StatusDefault, nil
		}

		victim, ok := findAttackCreatureTarget(world, room, viewer, target, ordinal)
		if !ok {
			ctx.WriteString("\n그런 괴물은 존재하지 않습니다.\n")
			return StatusDefault, nil
		}
		if err := revealAbsorbActor(ctx, world, room.ID, viewer, actor); err != nil {
			return StatusDefault, err
		}

		now := time.Now().Unix()
		remaining, used, err := world.UseCreatureCooldown(actor.ID, absorbCooldownKey, now, 0)
		if err != nil {
			return StatusDefault, err
		}
		if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		if attackCreatureProtected(victim) {
			ctx.WriteString("\n당신은 " + turnCreaturePronoun(victim) + "의 기를 흡수할수 없습니다.\n")
			return StatusDefault, nil
		}
		if victim.Stats == nil || creatureStat(victim, "hpCurrent") <= 0 {
			ctx.WriteString("그 상대는 공격할 수 없습니다.\n")
			return StatusDefault, nil
		}

		if adder, ok := world.(interface {
			AddEnemy(attacker, defender model.CreatureID) (bool, error)
		}); ok {
			_, _ = adder.AddEnemy(victim.ID, actor.ID)
		}
		_, _ = world.UpdateCreatureTags(victim.ID, []string{"was_attacked"}, nil)
		if err := world.SetCreatureCooldown(actor.ID, absorbCooldownKey, now, absorbCooldownSeconds); err != nil {
			return StatusDefault, err
		}

		actorName := attackCreatureName(actor)
		victimName := attackCreatureName(victim)
		chance := absorbChance(actor, victim)
		if rng(1, 100) > chance {
			ctx.WriteString("\n당신은 흡성대법의 주문을 외쳤습니다. 하지만 주문이 튕겨져 나오면서 " + victimName + krtext.Particle(victimName, '1') + " 주문을 견뎌냈습니다.\n")
			return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 흡성대법의 주문을 외칩니다.\n하지만 주문이 튕겨져 나오면서 "+victimName+krtext.Particle(victimName, '1')+" 주문을 견뎌냈습니다.\n")
		}

		ctx.WriteString("\n당신은 흡성대법의 주문을 외칩니다.\n상대의 기가 당신에게 흘러 들어옵니다.\n")
		if err := roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 흡성대법의 주문을 외칩니다.\n"+victimName+"의 기가 흘러나와 "+actorName+"에게 스며듭니다.\n"); err != nil {
			return StatusDefault, err
		}
		if absorbTargetUndead(victim) {
			if err := world.SetCreatureStat(actor.ID, "mpCurrent", 0); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("\n앗! 더러운 기운이 당신에게 흘러듭니다.\n")
			return StatusDefault, nil
		}

		damage := absorbDamage(actor, rng)
		_, applied, dead, err := world.ApplyCreatureDamage(victim.ID, damage)
		if err != nil {
			return StatusDefault, err
		}
		if err := world.RecordCreatureDamage(victim.ID, actor.ID, applied); err != nil {
			return StatusDefault, err
		}
		if err := absorbHealActor(world, actor, damage); err != nil {
			return StatusDefault, err
		}

		ctx.WriteString(fmt.Sprintf("\n당신은 %s의 기를 %d만큼 흡수했습니다.\n", victimName, damage))
		if err := roomBroadcast(ctx, room.ID, fmt.Sprintf("\n%s%s %s의 기를 %d만큼 흡수했습니다.\n", actorName, krtext.Particle(actorName, '1'), victimName, damage)); err != nil {
			return StatusDefault, err
		}
		if !dead {
			return StatusDefault, nil
		}

		if err := finalizeMonsterDeathWithOptionalFinalizer(ctx, world, finalizer, actor, victim); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("\n당신은 " + victimName + "의 정기를 흡수했습니다.\n" + victimName + krtext.Particle(victimName, '1') + " 쓰러졌습니다.\n")
		return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+victimName+"의 정기를 흡수했습니다.\n"+victimName+krtext.Particle(victimName, '1')+" 쓰러졌습니다.\n")
	}
}

func energySkillRollFunc(rng SearchRollFunc) SearchRollFunc {
	if rng != nil {
		return rng
	}
	return func(min int, max int) int {
		if max <= min {
			return min
		}
		return min + rand.Intn(max-min+1)
	}
}

func energySkillActor(world LookWorld, viewer LookViewer, handler string) (model.Creature, model.Player, error) {
	actor, ok := world.Creature(viewer.CreatureID)
	if !ok {
		return model.Creature{}, model.Player{}, fmt.Errorf("%s: actor creature %q not found", handler, viewer.CreatureID)
	}
	var player model.Player
	if !viewer.PlayerID.IsZero() {
		player, _ = world.Player(viewer.PlayerID)
	}
	return actor, player, nil
}

func powerClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	if class != model.ClassFighter && class < model.ClassInvincible {
		return "검사만 사용할 수 있는 기술입니다.\n"
	}
	if class >= model.ClassInvincible && !powerHasFighterTraining(actor) {
		return "\n검사를 무적수련하지 않았습니다..\n"
	}
	return ""
}

func powerHasFighterTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SFIGHTER",
		"fighterSpell",
		"fighterTraining",
		"fighterMode",
	)
}

func powerStatusActive(player model.Player, actor model.Creature) bool {
	return statusEffectActive(player, actor, powerStatusTags...)
}

func powerChance(actor model.Creature) int {
	return minInt(85, ((attackCreatureLevel(actor)+3)/4)*20+legacyStatBonus(creatureStat(actor, "dexterity")))
}

func powerStatusDurationSeconds(actor model.Creature) int64 {
	return int64(120 + 60*(((attackCreatureLevel(actor)+3)/4)/5))
}

func powerApplySuccess(world EnergySkillBuffWorld, player model.Player, actor model.Creature) error {
	if err := world.SetCreatureStat(actor.ID, "strength", creatureStat(actor, "strength")+powerStrengthBonus); err != nil {
		return err
	}
	if _, err := world.UpdateCreatureTags(actor.ID, powerStatusTags, nil); err != nil {
		return err
	}
	if !player.ID.IsZero() {
		if _, err := world.UpdatePlayerTags(player.ID, powerStatusTags, nil); err != nil {
			return err
		}
	}
	return world.RecalculateAC(actor.ID)
}

func accurateClassRejection(actor model.Creature) string {
	class := creatureClass(actor)
	if class != model.ClassAssassin && class != model.ClassThief && class < model.ClassInvincible {
		return "자객과 도둑만 사용할 수 있는 기술입니다.\n"
	}
	if class >= model.ClassInvincible && !accurateHasAssassinOrThiefTraining(actor) {
		return "\n자객이나 도둑을 무적수련하지 않았습니다..\n"
	}
	return ""
}

func accurateHasAssassinOrThiefTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SASSASSIN",
		"STHIEF",
		"assassinSpell",
		"thiefSpell",
		"assassinTraining",
		"thiefTraining",
		"assassinMode",
		"thiefMode",
	)
}

func accurateStatusActive(player model.Player, actor model.Creature) bool {
	return statusEffectActive(player, actor, accurateStatusTags...)
}

func accurateChance(actor model.Creature) int {
	return minInt(85, ((attackCreatureLevel(actor)+3)/4)*20+legacyStatBonus(creatureStat(actor, "dexterity")))
}

func accurateStatusDurationSeconds(actor model.Creature) int64 {
	return int64(150 + 60*(((attackCreatureLevel(actor)+3)/4)/5))
}

func accurateApplySuccess(world EnergySkillBuffWorld, player model.Player, actor model.Creature) error {
	if _, err := world.UpdateCreatureTags(actor.ID, accurateStatusTags, nil); err != nil {
		return err
	}
	if !player.ID.IsZero() {
		if _, err := world.UpdatePlayerTags(player.ID, accurateStatusTags, nil); err != nil {
			return err
		}
	}
	if err := world.RecalculateTHACO(actor.ID); err != nil {
		return err
	}
	if err := world.RecalculateAC(actor.ID); err != nil {
		return err
	}
	return nil
}

func absorbClassRejection(actor model.Creature) string {
	if creatureClass(actor) < model.ClassInvincible {
		return "무적이상만 쓸 수 있는 기술입니다.\n"
	}
	if !absorbHasMageTraining(actor) {
		return "\n도술사를 무적수련하지 않았습니다..\n"
	}
	return ""
}

func absorbHasMageTraining(actor model.Creature) bool {
	return attackCreatureHasFlag(actor,
		"SMAGE",
		"mageSpell",
		"mageTraining",
		"mageMode",
	)
}

func absorbChance(actor model.Creature, target model.Creature) int {
	chance := (((attackCreatureLevel(actor) + 3) / 4) - ((attackCreatureLevel(target) + 3) / 4)) * 20
	chance += legacyStatBonus(creatureStat(actor, "intelligence")) * 5
	return minInt(chance, 80)
}

func absorbDamage(actor model.Creature, rng SearchRollFunc) int {
	maxRoll := 5
	class := creatureClass(actor)
	if class >= model.ClassBulsa {
		maxRoll = 15
	} else if class == model.ClassCaretaker {
		maxRoll = 10
	}
	damage := rng(1, maxRoll) * ((attackCreatureLevel(actor) + 3) / 4)
	if damage < 1 {
		return 1
	}
	return damage
}

func absorbHealActor(world EnergySkillBuffWorld, actor model.Creature, amount int) error {
	if amount < 1 {
		amount = 1
	}
	hpMax := creatureStat(actor, "hpMax")
	next := creatureStat(actor, "hpCurrent") + amount
	if hpMax > 0 && next > hpMax {
		next = hpMax
	}
	return world.SetCreatureStat(actor.ID, "hpCurrent", next)
}

func absorbTargetUndead(target model.Creature) bool {
	return attackCreatureHasFlag(target, "MUNDED", "undead", "turnable")
}

func revealAbsorbActor(ctx *Context, world EnergySkillBuffWorld, roomID model.RoomID, viewer LookViewer, actor model.Creature) error {
	invisible := attackCreatureHasFlag(actor, "invisible", "pinvis", "PINVIS")
	if !viewer.PlayerID.IsZero() {
		if player, ok := world.Player(viewer.PlayerID); ok && hasAnyNormalizedFlag(player.Metadata.Tags, "invisible", "pinvis", "PINVIS") {
			invisible = true
		}
		if _, err := world.UpdatePlayerTags(viewer.PlayerID, nil, []string{"invisible", "pinvis", "PINVIS"}); err != nil {
			return err
		}
	}
	if _, err := world.UpdateCreatureTags(actor.ID, nil, []string{"invisible", "pinvis", "PINVIS"}); err != nil {
		return err
	}
	if actor.Stats["PINVIS"] != 0 {
		if err := world.SetCreatureStat(actor.ID, "PINVIS", 0); err != nil {
			return err
		}
	}
	if !invisible {
		return nil
	}
	actorName := attackCreatureName(actor)
	ctx.WriteString("\n당신의 모습이 나타나기 시작합니다.\n")
	return roomBroadcast(ctx, roomID, "\n"+actorName+krtext.Particle(actorName, '1')+" 모습이 보이기 시작합니다.\n")
}

func renderPowerWait(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%d분 %02d초 기다리세요.\n", seconds/60, seconds%60)
}
