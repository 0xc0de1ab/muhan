package command

import (
	"math/rand"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

var spellFailRandIntn = rand.Intn

// applyMagicResistanceDamage ports the offensive-spell magic-resistance reduction
// from C offensive_spell (magic1.c:1261-1266). It applies UNIFORMLY to every
// offensive spell (there is no per-element fire/cold/acid resistance in C's
// offensive_spell): a target that resists magic takes
//
//	dmg -= (dmg * 2 * MIN(50, piety+intelligence)) / 100
//
// C gates this on PRMAGI for players and MRMAGI for monsters — two distinct bit
// flags for the same concept applied to the two creature kinds. In the Go flag
// model those names (with resistMagic/magicResistance) are a single alias group,
// so a type check is unnecessary: presence of the flag is the resist condition.
// The result is NOT floored: C lets a fully-resisted hit reach 0 damage (only the
// pre-resistance dice roll is MAX(1, dmg)).
func applyMagicResistanceDamage(target model.Creature, damage int) int {
	if !creatureHasAnyFlag(target, "PRMAGI", "MRMAGI", "resistMagic", "magicResistance") {
		return damage
	}

	sum := creatureStat(target, "piety") + creatureStat(target, "intelligence")
	if sum > 50 {
		sum = 50
	}
	if sum < 0 {
		sum = 0
	}

	damage -= (damage * 2 * sum) / 100
	if damage < 0 {
		damage = 0
	}
	return damage
}

// RegisterSpellAggro adds the spell caster to the target creature's (monster's) enemy list.
func RegisterSpellAggro(world interface{}, victimID, attackerID model.CreatureID) {
	if adder, ok := world.(interface {
		AddEnemy(attacker, defender model.CreatureID) (bool, error)
	}); ok {
		wasNew, _ := adder.AddEnemy(victimID, attackerID)
		_ = wasNew // aggro gained via spell; spell cast already broadcasts
	}
}

// spellFail returns true if the spell cast fails per C spell_fail() in magic8.c .
// Uses exact legacy formula: chance based on class + level + int bonus, then mrand(1,100) > chance => fail.
// On fail, caller should typically consume MP (as in C) and skip effect.
// Matches player-visible fail message and rates for all classes.
func spellFail(actor model.Creature) bool {
	// C spell_fail (magic8.c:811) draws mrand(1,100) BEFORE the class switch, so the
	// roll is consumed for every class — including the default branch (DM/Caretaker/
	// Bulsa/Invincible/SubDM/ZoneMaker) that always succeeds. Roll first to keep the
	// shared RNG stream in step with C.
	n := spellFailRandIntn(100) + 1
	class := creatureClass(actor)
	level := getCreatureLevel(actor)
	intel := creatureStat(actor, "intelligence")
	bns := legacyStatBonus(intel)

	var chance int
	switch class {
	case model.ClassAssassin:
		chance = (((level+3)/4)+bns)*5 + 30
	case model.ClassBarbarian:
		chance = (((level + 3) / 4) + bns) * 5
	case model.ClassCleric:
		chance = (((level+3)/4)+bns)*5 + 65
	case model.ClassFighter:
		chance = (((level+3)/4)+bns)*5 + 10
	case model.ClassMage:
		chance = (((level+3)/4)+bns)*5 + 75
	case model.ClassPaladin:
		chance = (((level+3)/4)+bns)*5 + 50
	case model.ClassRanger:
		chance = (((level+3)/4)+bns)*4 + 56
	case model.ClassThief:
		chance = (((level+3)/4)+bns)*6 + 22
	default:
		// DM/caretaker etc always succeed per C default:0 (roll already consumed).
		return false
	}
	if n > chance {
		return true // fail
	}
	return false
}
