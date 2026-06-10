package command

import (
	"math/rand"

	"muhan/internal/world/model"
)

var spellFailRandIntn = rand.Intn

// GetSpellResistanceTag returns the resistance tag that corresponds to the given spell's magic power.
func GetSpellResistanceTag(magicPower int) string {
	switch magicPower {
	case magicPowerBurn, magicPowerFireball:
		return "resistFire"
	case magicPowerBlister, magicPowerWaterBolt, 15:
		return "resistCold"
	case magicPowerHurt, magicPowerShockbolt, magicPowerDustGust, 14:
		return "resistMagic"
	case magicPowerStoneCrush, magicPowerRumble:
		return "resistAcid"
	}
	return ""
}

// ApplyElementalResistance checks if the target has the resistance tag corresponding to the spell
// and applies the damage reduction formula: dmg -= (dmg * 2 * min(50, piety + intelligence)) / 100.
func ApplyElementalResistance(target model.Creature, magicPower int, damage int) int {
	tag := GetSpellResistanceTag(magicPower)
	if tag == "" {
		return damage
	}

	var hasResist bool
	switch tag {
	case "resistFire":
		hasResist = hasAnyNormalizedFlag(target.Metadata.Tags, "resistFire", "fireResistance", "PRFIRE")
	case "resistCold":
		hasResist = hasAnyNormalizedFlag(target.Metadata.Tags, "resistCold", "coldResistance", "PRCOLD")
	case "resistMagic":
		hasResist = hasAnyNormalizedFlag(target.Metadata.Tags, "resistMagic", "magicResistance", "PRMAGI", "MRMAGI")
	case "resistAcid":
		hasResist = hasAnyNormalizedFlag(target.Metadata.Tags, "resistAcid", "acidResistance", "PRACID", "MRACID")
	}

	if !hasResist {
		return damage
	}

	piety := target.Stats["piety"]
	intel := target.Stats["intelligence"]
	sum := piety + intel
	if sum > 50 {
		sum = 50
	}
	if sum < 0 {
		sum = 0
	}

	reduced := (damage * 2 * sum) / 100
	damage -= reduced
	if damage < 1 {
		damage = 1
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
		// DM/caretaker etc always succeed per C default:0
		return false
	}
	// mrand(1,100)
	n := spellFailRandIntn(100) + 1
	if n > chance {
		return true // fail
	}
	return false
}
