package command

import (
	"fmt"
	"math/rand"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	buyStatesExperienceBase = 100000000
	buyStatesCaretakerCost  = 3000000
	buyStatesBulsaCost      = 50000000
)

type BuyStatesWorld interface {
	InventoryWorld
	SetCreatureStat(model.CreatureID, string, int) error
}

func NewBuyStatesHandler(world BuyStatesWorld, roll SearchRollFunc) Handler {
	if roll == nil {
		roll = func(min int, max int) int {
			if max <= min {
				return min
			}
			return min + rand.Intn(max-min+1)
		}
	}
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		_, actor, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}

		class := creatureClass(actor)
		if class != model.ClassCaretaker && class != model.ClassBulsa {
			ctx.WriteString("초인, 불사만이 가능합니다.")
			return StatusDefault, nil
		}
		if len(resolved.Args) == 0 {
			ctx.WriteString("\"체력\" 과 \"도력\" 중 어느 것을 올리시려고요?")
			return StatusDefault, nil
		}

		target := resolved.Args[0]
		cost := buyStatesCost(class)
		amount, ok := buyStatesCommonAmount(ctx, actor, cost)
		if !ok {
			return StatusDefault, nil
		}
		if target == "체력" || target == "도력" {
			return buyStatesHPMP(ctx, world, actor, target, class, cost, amount, roll)
		}
		return buyStatesAttribute(ctx, world, actor, target, class, cost, roll)
	}
}

func buyStatesCommonAmount(ctx *Context, actor model.Creature, cost int) (int, bool) {
	exp := creatureStat(actor, "experience")
	gold := creatureStat(actor, "gold")
	amount := (exp - buyStatesExperienceBase) / cost
	if amount < 1 {
		ctx.WriteString("당신의 경험치로는 능력치 향상을 할 수 없습니다.")
		return 0, false
	}
	if affordable := gold / cost; affordable < amount {
		amount = affordable
	}
	if amount < 1 {
		ctx.WriteString("당신이 가진 돈으로는 향상을 할수 없습니다.\n")
		return 0, false
	}
	return amount, true
}

func buyStatesHPMP(ctx *Context, world BuyStatesWorld, actor model.Creature, target string, class int, cost int, amount int, roll SearchRollFunc) (Status, error) {
	statKey, currentKey, capValue, maxRoll := buyStatesHPMPConfig(class, target)
	if statKey == "" {
		ctx.WriteString("어떤 능력치를 올리시려고요?")
		return StatusDefault, nil
	}
	if creatureStat(actor, statKey) >= capValue {
		ctx.WriteString("더이상은 " + target + "을 올릴수 없습니다.")
		return StatusDefault, nil
	}

	increase := amount*2 - 1
	for i := 0; i < amount; i++ {
		increase += roll(0, maxRoll) + 1
	}
	nextMax := minInt(capValue, creatureStat(actor, statKey)+increase)
	if err := world.SetCreatureStat(actor.ID, statKey, nextMax); err != nil {
		return StatusDefault, err
	}
	if err := world.SetCreatureStat(actor.ID, currentKey, nextMax); err != nil {
		return StatusDefault, err
	}
	if err := buyStatesSpend(world, actor, amount*cost); err != nil {
		return StatusDefault, err
	}
	if err := buyStatesRefreshDamageDice(world, actor, class); err != nil {
		return StatusDefault, err
	}

	invokeBroadcast(ctx, fmt.Sprintf("\n### %s님의 능력치가 향상이 되었습니다!", attackCreatureName(actor)))
	ctx.WriteString("\n축하합니다! 당신의 능력치가 올랐습니다!")
	return StatusDefault, nil
}

func buyStatesAttribute(ctx *Context, world BuyStatesWorld, actor model.Creature, target string, class int, cost int, roll SearchRollFunc) (Status, error) {
	statKey := buyStatesAttributeKey(target)
	if statKey == "" {
		ctx.WriteString("어떤 능력치를 올리시려고요?")
		return StatusDefault, nil
	}
	if creatureStat(actor, statKey) > buyStatesAttributeLimit(class) {
		ctx.WriteString("더이상 올릴수 없는 능력치입니다.")
		return StatusDefault, nil
	}

	if err := world.SetCreatureStat(actor.ID, statKey, creatureStat(actor, statKey)+1); err != nil {
		return StatusDefault, err
	}
	if roll(0, 1) == 1 {
		bonus := buyStatesAttributeHPBonus(class, target, roll)
		if err := world.SetCreatureStat(actor.ID, "hpMax", creatureStat(actor, "hpMax")+bonus); err != nil {
			return StatusDefault, err
		}
	} else {
		if err := world.SetCreatureStat(actor.ID, "mpMax", creatureStat(actor, "mpMax")+buyStatesAttributeMPBonus(class, roll)); err != nil {
			return StatusDefault, err
		}
	}
	if err := buyStatesSpend(world, actor, cost); err != nil {
		return StatusDefault, err
	}
	if err := buyStatesRefreshDamageDice(world, actor, class); err != nil {
		return StatusDefault, err
	}

	invokeBroadcast(ctx, fmt.Sprintf("\n### %s님의 능력치가 향상이 되었습니다!", attackCreatureName(actor)))
	ctx.WriteString("\n축하합니다! 당신의 능력치가 올랐습니다!")
	return StatusDefault, nil
}

func buyStatesSpend(world BuyStatesWorld, actor model.Creature, cost int) error {
	if err := world.SetCreatureStat(actor.ID, "experience", creatureStat(actor, "experience")-cost); err != nil {
		return err
	}
	return world.SetCreatureStat(actor.ID, "gold", creatureStat(actor, "gold")-cost)
}

func buyStatesRefreshDamageDice(world BuyStatesWorld, actor model.Creature, class int) error {
	switch class {
	case model.ClassCaretaker:
		if err := world.SetCreatureStat(actor.ID, "nDice", 4); err != nil {
			return err
		}
		if err := world.SetCreatureStat(actor.ID, "sDice", 3); err != nil {
			return err
		}
		if !attackCreatureHasFlag(actor, "PUPDMG", "upDamage") {
			return world.SetCreatureStat(actor.ID, "pDice", 4)
		}
	case model.ClassBulsa:
		if err := world.SetCreatureStat(actor.ID, "nDice", 5); err != nil {
			return err
		}
		if err := world.SetCreatureStat(actor.ID, "sDice", 5); err != nil {
			return err
		}
		if !attackCreatureHasFlag(actor, "PUPDMG", "upDamage") {
			return world.SetCreatureStat(actor.ID, "pDice", 5)
		}
	default:
		return fmt.Errorf("buy states: unsupported class %d", class)
	}
	return nil
}

func buyStatesCost(class int) int {
	if class == model.ClassBulsa {
		return buyStatesBulsaCost
	}
	return buyStatesCaretakerCost
}

func buyStatesHPMPConfig(class int, target string) (statKey string, currentKey string, capValue int, maxRoll int) {
	switch target {
	case "체력":
		if class == model.ClassBulsa {
			return "hpMax", "hpCurrent", 5000, 5
		}
		return "hpMax", "hpCurrent", 3000, 4
	case "도력":
		if class == model.ClassBulsa {
			return "mpMax", "mpCurrent", 4000, 5
		}
		return "mpMax", "mpCurrent", 2000, 3
	default:
		return "", "", 0, 0
	}
}

func buyStatesAttributeKey(target string) string {
	switch target {
	case "힘":
		return "strength"
	case "민첩":
		return "dexterity"
	case "맷집":
		return "constitution"
	case "지식":
		return "intelligence"
	case "신앙심":
		return "piety"
	default:
		return ""
	}
}

func buyStatesAttributeLimit(class int) int {
	if class == model.ClassBulsa {
		return 59
	}
	return 44
}

func buyStatesAttributeHPBonus(class int, target string, roll SearchRollFunc) int {
	if class == model.ClassBulsa {
		return 4
	}
	if target == "민첩" {
		return 4
	}
	if target == "신앙심" {
		return roll(2, 4)
	}
	return roll(3, 4)
}

func buyStatesAttributeMPBonus(class int, roll SearchRollFunc) int {
	if class == model.ClassBulsa {
		return 3
	}
	return roll(2, 3)
}

// --- Legacy level-up formulas and tables (Package 6/6) ---
// Exact port from C src/player.c:up_level / down_level + global.c:class_stats + level_cycle
// Per-class HP/MP start + per-level bonus (note: formula uses (level-1)/2 * bonus + start for maxes)
// Stat cycle every 4 levels starting at lvl 4 (index based on (level-2)%10 )
// Dice set from table; pDice follows C's SASSASSIN..STHIEF training flag count.
// No "move" bonus in C class_stats table; task mention covered by HP/MP/dice.

const (
	legacyStatNone = 0
	legacyStatSTR  = 1
	legacyStatDEX  = 2
	legacyStatCON  = 3
	legacyStatINT  = 4
	legacyStatPTY  = 5
)

type legacyClassStatBonusEntry struct {
	hpStart int
	mpStart int
	hp      int
	mp      int
	nDice   int
	sDice   int
	pDice   int
}

var legacyClassStatBonuses = map[int]legacyClassStatBonusEntry{
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

var legacyLevelCycleTable = map[int][10]int{
	0:                     {0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	model.ClassAssassin:   {legacyStatCON, legacyStatPTY, legacyStatSTR, legacyStatINT, legacyStatDEX, legacyStatINT, legacyStatDEX, legacyStatPTY, legacyStatSTR, legacyStatDEX},
	model.ClassBarbarian:  {legacyStatINT, legacyStatDEX, legacyStatPTY, legacyStatCON, legacyStatSTR, legacyStatCON, legacyStatDEX, legacyStatSTR, legacyStatPTY, legacyStatSTR},
	model.ClassCleric:     {legacyStatSTR, legacyStatDEX, legacyStatCON, legacyStatPTY, legacyStatINT, legacyStatPTY, legacyStatINT, legacyStatDEX, legacyStatCON, legacyStatINT},
	model.ClassFighter:    {legacyStatPTY, legacyStatINT, legacyStatDEX, legacyStatCON, legacyStatSTR, legacyStatCON, legacyStatINT, legacyStatSTR, legacyStatDEX, legacyStatSTR},
	model.ClassMage:       {legacyStatSTR, legacyStatDEX, legacyStatPTY, legacyStatCON, legacyStatINT, legacyStatCON, legacyStatINT, legacyStatDEX, legacyStatPTY, legacyStatINT},
	model.ClassPaladin:    {legacyStatDEX, legacyStatINT, legacyStatCON, legacyStatSTR, legacyStatPTY, legacyStatSTR, legacyStatINT, legacyStatPTY, legacyStatCON, legacyStatPTY},
	model.ClassRanger:     {legacyStatPTY, legacyStatSTR, legacyStatINT, legacyStatCON, legacyStatDEX, legacyStatCON, legacyStatDEX, legacyStatSTR, legacyStatINT, legacyStatDEX},
	model.ClassThief:      {legacyStatINT, legacyStatCON, legacyStatPTY, legacyStatSTR, legacyStatDEX, legacyStatSTR, legacyStatCON, legacyStatDEX, legacyStatPTY, legacyStatDEX},
	model.ClassInvincible: {legacyStatSTR, legacyStatDEX, legacyStatINT, legacyStatCON, legacyStatPTY, legacyStatSTR, legacyStatDEX, legacyStatINT, legacyStatCON, legacyStatPTY},
	model.ClassCaretaker:  {legacyStatSTR, legacyStatDEX, legacyStatINT, legacyStatCON, legacyStatPTY, legacyStatSTR, legacyStatDEX, legacyStatINT, legacyStatCON, legacyStatPTY},
	model.ClassBulsa:      {legacyStatSTR, legacyStatDEX, legacyStatINT, legacyStatCON, legacyStatPTY, legacyStatSTR, legacyStatDEX, legacyStatINT, legacyStatCON, legacyStatPTY},
	model.ClassSubDM:      {legacyStatSTR, legacyStatDEX, legacyStatINT, legacyStatCON, legacyStatPTY, legacyStatSTR, legacyStatDEX, legacyStatINT, legacyStatCON, legacyStatPTY},
	model.ClassDM:         {legacyStatSTR, legacyStatDEX, legacyStatINT, legacyStatCON, legacyStatPTY, legacyStatSTR, legacyStatDEX, legacyStatINT, legacyStatCON, legacyStatPTY},
}

func legacyClassStatBonusesFor(class int) legacyClassStatBonusEntry {
	if b, ok := legacyClassStatBonuses[class]; ok {
		return b
	}
	return legacyClassStatBonuses[0]
}

func legacyLevelCycleFor(class int) [10]int {
	if c, ok := legacyLevelCycleTable[class]; ok {
		return c
	}
	return legacyLevelCycleTable[0]
}

func legacyStatName(id int) string {
	switch id {
	case legacyStatSTR:
		return "strength"
	case legacyStatDEX:
		return "dexterity"
	case legacyStatCON:
		return "constitution"
	case legacyStatINT:
		return "intelligence"
	case legacyStatPTY:
		return "piety"
	default:
		return ""
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var legacyLevelUpTrainingFlags = []string{
	"SASSASSIN",
	"SBARBARIAN",
	"SCLERIC",
	"SFIGHTER",
	"SMAGE",
	"SPALADIN",
	"SRANGER",
	"STHIEF",
}

func legacyLevelUpTrainingCount(creature model.Creature) int {
	count := 0
	for _, flag := range legacyLevelUpTrainingFlags {
		if creatureHasAnyFlag(creature, flag) {
			count++
		}
	}
	if count == 0 {
		return 1
	}
	return count
}

// applyLegacyLevelUp applies exact C up_level formulas when advancing from oldLevel to newLevel.
// Called from train after SetCreatureLevel. Sets dice, recomputes hp/mp max/current per formula,
// applies stat +1 on 4-level boundaries per cycle table.
func applyLegacyLevelUp(world interface {
	SetCreatureStat(model.CreatureID, string, int) error
}, crt model.Creature, class, oldLevel, newLevel int) error {
	if newLevel <= oldLevel || world == nil || crt.ID.IsZero() {
		return nil
	}
	bonuses := legacyClassStatBonusesFor(class)
	// Dice match C class_stats, with pDice raised by learned training spell flags.
	if err := world.SetCreatureStat(crt.ID, "nDice", bonuses.nDice); err != nil {
		return err
	}
	if err := world.SetCreatureStat(crt.ID, "sDice", bonuses.sDice); err != nil {
		return err
	}
	pd := maxInt(bonuses.pDice, (legacyLevelUpTrainingCount(crt)+1)/2)
	if err := world.SetCreatureStat(crt.ID, "pDice", pd); err != nil {
		return err
	}
	// full formula recalc (matches C lines 850-855)
	hpMax := bonuses.hpStart + (bonuses.hp * (newLevel - 1) / 2)
	mpMax := bonuses.mpStart + (bonuses.mp * (newLevel - 1) / 2)
	if err := world.SetCreatureStat(crt.ID, "hpMax", hpMax); err != nil {
		return err
	}
	if err := world.SetCreatureStat(crt.ID, "mpMax", mpMax); err != nil {
		return err
	}
	if err := world.SetCreatureStat(crt.ID, "hpCurrent", hpMax); err != nil {
		return err
	}
	if err := world.SetCreatureStat(crt.ID, "mpCurrent", mpMax); err != nil {
		return err
	}
	// stat cycle inc only on multiples of 4 (C: after ++, if(level%4) return else index=(level-2)%10 inc)
	if newLevel%4 == 0 {
		idx := (newLevel - 2) % 10
		if idx < 0 {
			idx += 10
		}
		cyc := legacyLevelCycleFor(class)
		if statName := legacyStatName(cyc[idx]); statName != "" {
			cur := creatureStat(crt, statName)
			if err := world.SetCreatureStat(crt.ID, statName, cur+1); err != nil {
				return err
			}
		}
	}
	return nil
}
