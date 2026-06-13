package command

import "github.com/0xc0de1ab/muhan/internal/world/model"

type commandHiddenWorld interface {
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
}

func clearCommandActorHidden(world commandHiddenWorld, player model.Player, creature model.Creature) (model.Player, model.Creature, error) {
	updatedCreature, err := world.UpdateCreatureTags(creature.ID, nil, []string{"hidden", "phiddn", "PHIDDN"})
	if err != nil {
		return player, creature, err
	}
	creature = updatedCreature
	if creature.Stats != nil {
		if _, ok := creature.Stats["PHIDDN"]; ok {
			if setter, ok := world.(interface {
				SetCreatureStat(model.CreatureID, string, int) error
			}); ok {
				if err := setter.SetCreatureStat(creature.ID, "PHIDDN", 0); err != nil {
					return player, creature, err
				}
			}
			creature.Stats["PHIDDN"] = 0
		}
	}
	if !player.ID.IsZero() {
		updatedPlayer, err := world.UpdatePlayerTags(player.ID, nil, []string{"hidden", "phiddn", "PHIDDN"})
		if err != nil {
			return player, creature, err
		}
		player = updatedPlayer
	}
	return player, creature, nil
}
