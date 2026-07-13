package command

import (
	"testing"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

// stubProficiencyWorld is a minimal InventoryWorld for exercising the melee
// proficiency helpers directly. The weapon carries its "type" inline so no
// prototype lookup is needed.
type stubProficiencyWorld struct{}

func (stubProficiencyWorld) Player(model.PlayerID) (model.Player, bool)      { return model.Player{}, false }
func (stubProficiencyWorld) Creature(model.CreatureID) (model.Creature, bool) { return model.Creature{}, false }
func (stubProficiencyWorld) Object(model.ObjectInstanceID) (model.ObjectInstance, bool) {
	return model.ObjectInstance{}, false
}
func (stubProficiencyWorld) ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool) {
	return model.ObjectPrototype{}, false
}

// TestWeaponProficiencyDamageBonusRanksRawLikeC guards the fix for the melee
// damage bug: C attack_crt (command5.c:241) adds profic(ply, weapon->type)/10,
// where profic() first ranks the raw accumulation to 0-100. A Fighter with raw
// weapon proficiency 1910 sits exactly on the class table's index-4 threshold
// (rank 40), so the C-faithful bonus is 40/10 = 4 — NOT the pre-fix raw/10 = 191.
func TestWeaponProficiencyDamageBonusRanksRawLikeC(t *testing.T) {
	attacker := model.Creature{
		Stats: map[string]int{
			"class":             model.ClassFighter,
			"proficiency/sword": 1910,
		},
	}
	weapon := model.ObjectInstance{Properties: map[string]string{"type": "sword"}}

	got := weaponProficiencyDamageBonus(stubProficiencyWorld{}, attacker, weapon)
	if want := 4; got != want {
		t.Fatalf("weaponProficiencyDamageBonus = %d, want %d (ranked profic()/10, not raw 1910/10=191)", got, want)
	}
}

// TestWeaponProficiencyDamageBonusMaxedRank confirms a maxed raw value clamps to
// the ranked ceiling (100/10 = 10) rather than exploding to raw/10.
func TestWeaponProficiencyDamageBonusMaxedRank(t *testing.T) {
	attacker := model.Creature{
		Stats: map[string]int{
			"class":             model.ClassFighter,
			"proficiency/sword": 934808, // top real threshold -> rank 100
		},
	}
	weapon := model.ObjectInstance{Properties: map[string]string{"type": "sword"}}

	got := weaponProficiencyDamageBonus(stubProficiencyWorld{}, attacker, weapon)
	if want := 10; got != want {
		t.Fatalf("weaponProficiencyDamageBonus = %d, want %d (ranked 100/10)", got, want)
	}
}

// TestAttackHitsIgnoresWeaponProficiency guards the fix for the melee to-hit bug:
// C attack_crt (command5.c:232) computes the to-hit target as thaco - armor/10
// using the precomputed thaco (proficiency already folded in once). The command
// layer must NOT subtract weapon proficiency a second time, so the outcome must
// be identical whether raw proficiency is zero or maxed.
func TestAttackHitsIgnoresWeaponProficiency(t *testing.T) {
	previous := attackRoll
	attackRoll = func(_, max int) int { return max } // best possible roll (30)
	defer func() { attackRoll = previous }()

	victim := model.Creature{Stats: map[string]int{"armor": 0}}
	// thaco 40 with the best roll (30) can never satisfy 30 >= 40 -> always a miss,
	// regardless of proficiency, once the double-subtraction is gone.
	base := model.Creature{
		Equipment: map[string]model.ObjectInstanceID{"wield": "object:sword"},
		Stats:     map[string]int{"class": model.ClassFighter, "thaco": 40},
	}
	trained := model.Creature{
		Equipment: map[string]model.ObjectInstanceID{"wield": "object:sword"},
		Stats:     map[string]int{"class": model.ClassFighter, "thaco": 40, "proficiency/sword": 934808},
	}

	if attackHits(base, victim) {
		t.Fatalf("untrained attacker unexpectedly hit (target should be 40 > roll 30)")
	}
	if attackHits(trained, victim) {
		t.Fatalf("trained attacker hit — weapon proficiency must not lower the to-hit target (double-subtraction regression)")
	}
}
