package command

import "muhan/internal/world/model"

const quitGoodbye = "안녕히 가세요.\n"

type QuitWorld interface {
	LookWorld
	CreatureCooldownExpires(model.CreatureID, string) (int64, bool, error)
}

type QuitLowLevelSink interface {
	CleanupLowLevelQuit(ctx *Context, playerID model.PlayerID) error
}

type QuitOption func(*quitConfig)

func WithQuitLowLevelSink(sink QuitLowLevelSink) QuitOption {
	return func(cfg *quitConfig) {
		cfg.lowLevelSink = sink
	}
}

type quitConfig struct {
	lowLevelSink QuitLowLevelSink
}

func NewQuitHandler(worlds ...QuitWorld) Handler {
	var world QuitWorld
	if len(worlds) > 0 {
		world = worlds[0]
	}
	return NewQuitHandlerWithOptions(world)
}

func NewQuitHandlerWithOptions(world QuitWorld, options ...QuitOption) Handler {
	cfg := quitConfig{}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		if ctx != nil && world != nil {
			if blocked, err := quitBlockedByCombat(ctx, world); err != nil || blocked {
				return StatusDefault, err
			}
			playerID, cleanup, err := quitLowLevelCleanup(ctx, world)
			if err != nil {
				return StatusDefault, err
			}
			if cleanup && cfg.lowLevelSink != nil {
				if err := cfg.lowLevelSink.CleanupLowLevelQuit(ctx, playerID); err != nil {
					return StatusDefault, err
				}
			}
		}
		if ctx != nil {
			ctx.WriteString(quitGoodbye)
		}
		return StatusDisconnect, nil
	}
}

func quitBlockedByCombat(ctx *Context, world QuitWorld) (bool, error) {
	viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
	if err != nil {
		return false, err
	}
	actor, ok := world.Creature(viewer.CreatureID)
	if !ok || !attackActorAlreadyFighting(world, room, viewer, actor) {
		return false, nil
	}
	expires, ok, err := world.CreatureCooldownExpires(actor.ID, "attack")
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	remaining := expires + 20 - timeNow().Unix()
	if remaining <= 0 {
		return false, nil
	}
	ctx.WriteString(renderPleaseWait(remaining))
	return true, nil
}

func quitLowLevelCleanup(ctx *Context, world QuitWorld) (model.PlayerID, bool, error) {
	viewer := LookViewerFromContext(ctx)
	if viewer.PlayerID.IsZero() {
		return "", false, nil
	}
	player, ok := world.Player(viewer.PlayerID)
	if !ok || player.CreatureID.IsZero() {
		return viewer.PlayerID, false, nil
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return viewer.PlayerID, false, nil
	}
	return viewer.PlayerID, creatureClass(creature) < model.ClassInvincible && attackCreatureLevel(creature) < 6, nil
}
