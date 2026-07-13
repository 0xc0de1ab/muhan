package model

const (
	ClassZoneMaker  = 0
	ClassAssassin   = 1
	ClassBarbarian  = 2
	ClassCleric     = 3
	ClassFighter    = 4
	ClassMage       = 5
	ClassPaladin    = 6
	ClassRanger     = 7
	ClassThief      = 8
	ClassInvincible = 9
	ClassCaretaker  = 10
	ClassBulsa      = 11
	ClassSubDM      = 12
	ClassDM         = 13
)

// weaponProficiencyThresholds returns the class-dependent prof_array from legacy
// C player.c profic(). Raw weapon-proficiency accumulation is compared against
// these frozen 1999 thresholds to derive a 0-100 percentage. Kept in sync with
// the copy in internal/world/state/creature.go (stateCreatureWeaponProficiency);
// both derive from the same C source.
func weaponProficiencyThresholds(class int) [12]int64 {
	switch class {
	case ClassFighter, ClassInvincible, ClassCaretaker, ClassBulsa, ClassSubDM, ClassDM:
		return [12]int64{0, 768, 1024, 1440, 1910, 16000, 31214, 167000, 268488, 695000, 934808, 500000000}
	case ClassBarbarian:
		return [12]int64{0, 1536, 2048, 2880, 3820, 32000, 62428, 334000, 536976, 1390000, 1869616, 500000000}
	case ClassThief, ClassRanger:
		return [12]int64{0, 2304, 3072, 4320, 5730, 48000, 93642, 501000, 805464, 2085000, 2804424, 500000000}
	case ClassCleric, ClassPaladin, ClassAssassin:
		return [12]int64{0, 3072, 4096, 5076, 7640, 64000, 124856, 668000, 1073952, 2780000, 3939232, 500000000}
	default:
		return [12]int64{0, 5376, 7168, 10080, 13370, 112000, 218498, 1169000, 1879416, 4865000, 6543656, 500000000}
	}
}

// WeaponProficiencyPercent converts a raw weapon-proficiency accumulation into
// the legacy 0-100 ranked percentage used by combat math, matching C profic()
// (player.c). class selects the threshold table. Combat command code must rank
// raw proficiency through this before use — the raw value ranges up to
// 500,000,000 and is meaningless when consumed directly.
func WeaponProficiencyPercent(class, rawValue int) int {
	table := weaponProficiencyThresholds(class)
	rank := 100
	i := 10
	for i = 0; i < 11; i++ {
		if int64(rawValue) < table[i+1] {
			rank = 10 * i
			break
		}
	}
	if table[i+1] > table[i] {
		rank += int((int64(rawValue) - table[i]) * 10 / (table[i+1] - table[i]))
	}
	return rank
}
