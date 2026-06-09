package legacycrypt

import "testing"

func TestHashMatchesLegacyDESCryptWithoutSalt(t *testing.T) {
	tests := map[string]string{
		"abc":      "/H.d1bFP9CA",
		"password": "ReH/tr.OhIE",
		"test123":  "wuWWa61wlKg",
		"secret":   ".N7VsCCfkRc",
		"1234":     "WOCZU5Ja1Vg",
		"가나다라마바사":  "5uvZujm4.So",
	}
	for password, want := range tests {
		got, err := Hash(password)
		if err != nil {
			t.Fatalf("Hash(%q) error = %v", password, err)
		}
		if got != want {
			t.Fatalf("Hash(%q) = %q, want %q", password, got, want)
		}
		if !Verify(password, want) {
			t.Fatalf("Verify(%q, %q) = false, want true", password, want)
		}
	}
}

func TestVerifyRejectsWrongPassword(t *testing.T) {
	if Verify("wrong", "WOCZU5Ja1Vg") {
		t.Fatal("Verify accepted wrong password")
	}
}
