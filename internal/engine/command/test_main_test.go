package command

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	spellFailRandIntn = func(int) int {
		return 0
	}
	os.Exit(m.Run())
}
