package state

import (
	"strings"
	"testing"

	"muhan/internal/world/model"
)

func TestPersistenceSmoke(t *testing.T) {
	// Placeholder - real tests live in dm_support_test.go and player_save_test.go
}

func TestSavePlayerMissingDBRootErrorsByDefault(t *testing.T) {
	t.Setenv(testDisablePersistenceEnv, "")
	world := New(nil)
	playerID := model.PlayerID("player:alice")
	world.players[playerID] = model.Player{ID: playerID}

	err := world.SavePlayer(playerID)
	if err == nil {
		t.Fatal("SavePlayer without dbRoot succeeded without explicit test persistence disable")
	}
	if !strings.Contains(err.Error(), "dbRoot is not set") {
		t.Fatalf("SavePlayer error = %v, want dbRoot error", err)
	}
}

func TestSavePlayerMissingDBRootNoopsOnlyWhenTestHarnessOptedIn(t *testing.T) {
	t.Setenv(testDisablePersistenceEnv, "1")
	world := New(nil)
	playerID := model.PlayerID("player:alice")
	world.players[playerID] = model.Player{ID: playerID}

	if err := world.SavePlayer(playerID); err != nil {
		t.Fatalf("SavePlayer with test persistence disabled returned error: %v", err)
	}
}
