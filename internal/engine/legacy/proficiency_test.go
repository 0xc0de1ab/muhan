package legacy

import "testing"

func TestLowerProficiencyDistributesLossAcrossWeaponAndRealmSlots(t *testing.T) {
	proficiency := [5]int{2000, 2000, 2000, 2000, 2000}
	realms := [4]int{2000, 2000, 2000, 2000}

	gotProf, gotRealms := LowerProficiency(proficiency, realms, 90)

	for i, got := range gotProf {
		if got != 1990 {
			t.Fatalf("proficiency[%d] = %d, want 1990", i, got)
		}
	}
	for i, got := range gotRealms {
		if got != 1990 {
			t.Fatalf("realm[%d] = %d, want 1990", i, got)
		}
	}
}

func TestLowerProficiencyPreservesLegacyWeaponFloor(t *testing.T) {
	gotProf, gotRealms := LowerProficiency([5]int{}, [4]int{}, 0)

	if gotProf[0] != 1024 {
		t.Fatalf("proficiency[0] = %d, want C floor 1024", gotProf[0])
	}
	for i := 1; i < len(gotProf); i++ {
		if gotProf[i] != 0 {
			t.Fatalf("proficiency[%d] = %d, want 0", i, gotProf[i])
		}
	}
	for i, got := range gotRealms {
		if got != 0 {
			t.Fatalf("realm[%d] = %d, want 0", i, got)
		}
	}
}
