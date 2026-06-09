package command

import (
	"errors"
	"fmt"
	"log"

	"muhan/internal/world/model"
)

var ErrSaveGameActorRequired = errors.New("savegame actor required")

const saveGameSavedMessage = "저장하였습니다.\n"
const saveGameLowLevelMessage = "\n레벨이 5이하이면 저장되지 않습니다.\n"

type SaveGameSink interface {
	SaveGame(ctx *Context, playerID model.PlayerID) error
}

type SaveGameSinkFunc func(ctx *Context, playerID model.PlayerID) error

func (f SaveGameSinkFunc) SaveGame(ctx *Context, playerID model.PlayerID) error {
	if f == nil {
		return nil
	}
	return f(ctx, playerID)
}

type SaveGameOption func(*saveGameConfig)

func WithSaveGameSink(sink SaveGameSink) SaveGameOption {
	return func(cfg *saveGameConfig) {
		cfg.sink = sink
	}
}

type saveGameConfig struct {
	sink SaveGameSink
}

type saveGameMessageWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
}

func NewSaveGameHandler(options ...SaveGameOption) Handler {
	cfg := saveGameConfig{}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}

	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		playerID := saveGamePlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrSaveGameActorRequired
		}

		// B/C: Explicit "저장" command - log errors (still sync for user feedback)
		worldValue := any(nil)
		if ctx != nil && ctx.Values != nil {
			worldValue = ctx.Values["game.world"]
		}
		if w, ok := worldValue.(interface {
			SavePlayer(model.PlayerID) error
			SaveBank(model.BankID) error
			MarkPlayerDirty(model.PlayerID)
			MarkBankDirty(model.BankID)
		}); ok {
			w.MarkPlayerDirty(playerID)
			bankID := model.BankID("bank:player:" + string(playerID))
			w.MarkBankDirty(bankID)
			if err := w.SavePlayer(playerID); err != nil {
				log.Printf("[PERSIST] ERROR explicit savegame SavePlayer %s: %v", playerID, err)
			}
			if err := w.SaveBank(bankID); err != nil {
				log.Printf("[PERSIST] ERROR explicit savegame SaveBank %s: %v", bankID, err)
			}
		}

		output := saveGameSavedMessage
		if w, ok := worldValue.(saveGameMessageWorld); ok {
			output = saveGameLegacyMessage(w, playerID)
		}

		if cfg.sink != nil {
			if err := cfg.sink.SaveGame(ctx, playerID); err != nil {
				return StatusDefault, fmt.Errorf("savegame: %w", err)
			}
		}

		ctx.WriteString(output)
		return StatusDefault, nil
	}
}

func saveGameLegacyMessage(world saveGameMessageWorld, playerID model.PlayerID) string {
	player, ok := world.Player(playerID)
	if !ok || player.CreatureID.IsZero() {
		return saveGameSavedMessage
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return saveGameSavedMessage
	}
	class := creatureClass(creature)
	level := attackCreatureLevel(creature)
	if class < legacyClassInvincible && level < 6 {
		return saveGameLowLevelMessage
	}
	if class > 0 && level > 6 {
		return saveGameSavedMessage
	}
	return ""
}

func saveGamePlayerIDFromContext(ctx *Context) model.PlayerID {
	if ctx == nil || ctx.ActorID == "" {
		return ""
	}
	return model.PlayerID(ctx.ActorID)
}
