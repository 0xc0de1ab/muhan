package main

import (
	"errors"
	"strings"
	"testing"

	enginecmd "github.com/0xc0de1ab/muhan/internal/engine/command"
	"github.com/0xc0de1ab/muhan/internal/engine/game"
	"github.com/0xc0de1ab/muhan/internal/persist/legacycrypt"
	"github.com/0xc0de1ab/muhan/internal/session"
)

// #20: DES→bcrypt auto-migration invariant. The fixture creature:alice ships
// with the legacy DES hash "WOCZU5Ja1Vg" (of "1234"); after a successful login
// the stored value must be a bcrypt hash of the same password, so there is no
// silent regression that keeps writing DES hashes for rehashed accounts.
func TestServerLoginMigratesDESHashToBcryptOnSuccess(t *testing.T) {
	inputs := serverTestRuntimeInputs(t)
	defer inputs.world.Close()

	player, ok := inputs.world.Player("player:alice")
	if !ok {
		t.Fatal("player:alice missing")
	}
	if got := legacyPasswordHash(inputs.world, player); legacycrypt.IsBcryptHash(got) {
		t.Fatalf("precondition: alice stored hash %q is already bcrypt", got)
	}

	// Drive the real login flow with the correct legacy password.
	login := newServerLoginManager(inputs.world)
	loop := game.NewLoop(serverDispatcher(inputs),
		game.WithPrompt(func(session.ID, *enginecmd.Context, enginecmd.Status) string {
			return "> "
		}),
		game.WithPromptPolicy(func(status enginecmd.Status, err error) bool {
			return status != enginecmd.StatusDisconnect && status != enginecmd.StatusDoPrompt
		}),
		game.WithErrorFormatter(func(err error) string {
			if errors.Is(err, enginecmd.ErrUnknownCommand) || errors.Is(err, enginecmd.ErrUnhandledCommand) {
				return "무슨 말인지 모르겠습니다.\n"
			}
			return err.Error() + "\n"
		}),
		game.WithUnauthenticatedLineHandler(login.HandleLine),
	)
	commands := make(chan session.Command, 4)
	registerServerTestSession(t, loop, "s-login-migrate", commands, "")
	login.Start("s-login-migrate")

	handleServerTestLine(t, loop, "s-login-migrate", "player:alice")
	assertServerCommandContains(t, commands, session.Command{}, "암호를 넣어 주십시요: ")
	handleServerTestLine(t, loop, "s-login-migrate", "1234")
	assertServerCommandContains(t, commands, session.Command{Prompt: "> "}, "무한에 접속했습니다.")

	after := legacyPasswordHash(inputs.world, player)
	if !strings.HasPrefix(after, "$2") {
		t.Fatalf("stored hash after login = %q, want a $2-prefixed bcrypt hash", after)
	}
	if !legacycrypt.IsBcryptHash(after) {
		t.Fatalf("IsBcryptHash(%q) = false after migration", after)
	}
	if !legacycrypt.VerifyBcrypt("1234", after) {
		t.Fatal("migrated bcrypt hash does not verify against the login password")
	}
	if !legacycrypt.Verify("1234", after) {
		t.Fatal("legacycrypt.Verify no longer accepts the migrated hash")
	}
}
