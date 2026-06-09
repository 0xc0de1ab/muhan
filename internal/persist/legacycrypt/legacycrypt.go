package legacycrypt

import "strings"

func Verify(password, stored string) bool {
	stored = strings.TrimRight(strings.TrimSpace(stored), "\x00")
	if stored == "" {
		return false
	}
	hash, err := Hash(password)
	return err == nil && hash == stored
}
