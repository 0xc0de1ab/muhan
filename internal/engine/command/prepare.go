package command

import (
	"fmt"
	"time"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	prepareCooldownKey     = "prepare"
	prepareCooldownSeconds = int64(15)
)

var prepareStatusTags = []string{"prepared", "PPREPA"}

type PrepareWorld interface {
	LookWorld
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
}

func NewPrepareHandler(world PrepareWorld) Handler {
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		actor, ok := world.Creature(viewer.CreatureID)
		if !ok {
			return StatusDefault, fmt.Errorf("prepare: actor creature %q not found", viewer.CreatureID)
		}

		if prepareActorPrepared(world, viewer, actor) {
			ctx.WriteString("당신은 이미 함정들을 주의하고 있습니다.")
			return StatusDefault, nil
		}

		now := time.Now().Unix()
		if remaining, used, err := world.UseCreatureCooldown(actor.ID, prepareCooldownKey, now, prepareCooldownInterval(actor)); err != nil {
			return StatusDefault, err
		} else if !used {
			ctx.WriteString(renderPleaseWait(remaining))
			return StatusDefault, nil
		}

		if _, err := world.UpdateCreatureTags(actor.ID, prepareStatusTags, nil); err != nil {
			return StatusDefault, err
		}
		if !viewer.PlayerID.IsZero() {
			if _, err := world.UpdatePlayerTags(viewer.PlayerID, prepareStatusTags, nil); err != nil {
				return StatusDefault, err
			}
		}
		if prepareActorBlind(world, viewer, actor) {
			if err := prepareClearPrepared(world, viewer); err != nil {
				return StatusDefault, err
			}
		}

		actorName := attackCreatureName(actor)
		ctx.WriteString("당신은 이제부터 함정이 있나 살펴보며 갑니다.")
		return StatusDefault, roomBroadcast(ctx, room.ID, "\n"+actorName+krtext.Particle(actorName, '1')+" 함정을 조심하며 갑니다.")
	}
}

func prepareCooldownInterval(actor model.Creature) int64 {
	if creatureClass(actor) == model.ClassDM {
		return 0
	}
	return prepareCooldownSeconds
}

func prepareActorPrepared(world PrepareWorld, viewer LookViewer, actor model.Creature) bool {
	if creatureHasAnyFlag(actor, "prepared", "prepare", "PPREPA") {
		return true
	}
	if viewer.PlayerID.IsZero() {
		return false
	}
	player, ok := world.Player(viewer.PlayerID)
	return ok && hasAnyNormalizedFlag(player.Metadata.Tags, "prepared", "prepare", "PPREPA")
}

func prepareActorBlind(world PrepareWorld, viewer LookViewer, actor model.Creature) bool {
	if creatureHasAnyFlag(actor, "blind", "blinded", "PBLIND") {
		return true
	}
	if viewer.PlayerID.IsZero() {
		return false
	}
	player, ok := world.Player(viewer.PlayerID)
	return ok && hasAnyNormalizedFlag(player.Metadata.Tags, "blind", "blinded", "PBLIND")
}

func prepareClearPrepared(world PrepareWorld, viewer LookViewer) error {
	remove := []string{"prepared", "prepare", "PPREPA"}
	if !viewer.CreatureID.IsZero() {
		if _, err := world.UpdateCreatureTags(viewer.CreatureID, nil, remove); err != nil {
			return err
		}
	}
	if !viewer.PlayerID.IsZero() {
		if _, err := world.UpdatePlayerTags(viewer.PlayerID, nil, remove); err != nil {
			return err
		}
	}
	return nil
}
