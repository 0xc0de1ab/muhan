package legacycrypt

import (
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// HashBcrypt returns a bcrypt hash of the given password using the default cost.
func HashBcrypt(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyBcrypt checks whether password matches the given bcrypt hash.
func VerifyBcrypt(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// IsBcryptHash reports whether hash looks like a bcrypt hash string.
// Bcrypt hashes start with "$2a$" or "$2b$".
func IsBcryptHash(hash string) bool {
	return strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$")
}
