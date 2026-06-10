package legacycrypt

import "strings"

func Verify(password, stored string) bool {
	stored = strings.TrimRight(strings.TrimSpace(stored), "\x00")
	if stored == "" {
		return false
	}
	if IsBcryptHash(stored) {
		return VerifyBcrypt(password, stored)
	}
	hash, err := Hash(password)
	return err == nil && hash == stored
}
