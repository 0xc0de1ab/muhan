package legacycrypt

import (
	"strings"
	"testing"
)

// #20: the bcrypt helpers are on the live authentication path (login verify,
// account creation, password change) but had no coverage at all.

func TestHashBcryptRoundTrip(t *testing.T) {
	hash, err := HashBcrypt("secret-password")
	if err != nil {
		t.Fatalf("HashBcrypt error = %v", err)
	}
	if hash == "" {
		t.Fatal("HashBcrypt returned an empty hash")
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Fatalf("HashBcrypt result %q does not start with $2", hash)
	}
	if !IsBcryptHash(hash) {
		t.Fatalf("IsBcryptHash(%q) = false, want true for HashBcrypt output", hash)
	}
	if !VerifyBcrypt("secret-password", hash) {
		t.Fatal("VerifyBcrypt rejected the password that was just hashed")
	}
}

func TestVerifyBcryptRejectsWrongPassword(t *testing.T) {
	hash, err := HashBcrypt("correct-password")
	if err != nil {
		t.Fatalf("HashBcrypt error = %v", err)
	}
	if VerifyBcrypt("wrong-password", hash) {
		t.Fatal("VerifyBcrypt accepted a wrong password")
	}
	if VerifyBcrypt("", hash) {
		t.Fatal("VerifyBcrypt accepted an empty password")
	}
}

func TestVerifyBcryptRejectsMalformedHash(t *testing.T) {
	// "WOCZU5Ja1Vg" is a legacy DES crypt output: bcrypt verification must
	// never succeed against a non-bcrypt stored value.
	for _, hash := range []string{"", "not-a-hash", "WOCZU5Ja1Vg", "$2a$10$tooshort"} {
		if VerifyBcrypt("1234", hash) {
			t.Fatalf("VerifyBcrypt accepted malformed hash %q", hash)
		}
	}
}

func TestHashBcryptUsesRandomSalt(t *testing.T) {
	first, err := HashBcrypt("same-password")
	if err != nil {
		t.Fatalf("HashBcrypt error = %v", err)
	}
	second, err := HashBcrypt("same-password")
	if err != nil {
		t.Fatalf("HashBcrypt error = %v", err)
	}
	if first == second {
		t.Fatal("HashBcrypt produced identical hashes for the same password; salt is not random")
	}
	if !VerifyBcrypt("same-password", first) || !VerifyBcrypt("same-password", second) {
		t.Fatal("VerifyBcrypt rejected one of the salted hashes")
	}
}

func TestIsBcryptHash(t *testing.T) {
	tests := []struct {
		hash string
		want bool
	}{
		{"$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", true},
		{"$2b$12$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", true},
		// Only $2a$/$2b$ are recognized; $2y$ is deliberately not treated as a
		// bcrypt hash so a $2y$ value would take the legacy path instead.
		{"$2y$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", false},
		{"WOCZU5Ja1Vg", false}, // legacy DES crypt output
		{"", false},
		{"$2", false},
		{"$2a", false},
		// Prefix-only check: a bare "$2a$" is classified as bcrypt even though
		// it is not a valid hash (VerifyBcrypt still rejects it).
		{"$2a$", true},
	}
	for _, tt := range tests {
		if got := IsBcryptHash(tt.hash); got != tt.want {
			t.Errorf("IsBcryptHash(%q) = %v, want %v", tt.hash, got, tt.want)
		}
	}
}
