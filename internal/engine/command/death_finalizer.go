package command

import "github.com/0xc0de1ab/muhan/internal/world/model"

type monsterDeathFinalizeWorld interface {
	FinalizeMonsterDeath(model.CreatureID) (bool, error)
}

func finalizeMonsterDeathWithOptionalFinalizer(ctx *Context, world monsterDeathFinalizeWorld, finalizer AttackDeathFinalizer, attacker, victim model.Creature) error {
	if finalizer != nil {
		return finalizer(ctx, attacker, victim)
	}
	_, err := world.FinalizeMonsterDeath(victim.ID)
	return err
}
