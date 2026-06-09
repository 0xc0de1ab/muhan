package command

import (
	"strconv"
	"strings"

	"muhan/internal/world/model"
)

type proficiencyWorld interface {
	InventoryWorld
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
}

func getCreatureProficiency(c model.Creature, key string) int {
	if valStr, ok := c.Properties[key]; ok {
		if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
			return val
		}
	}
	if val, ok := c.Stats[key]; ok {
		return val
	}
	return 0
}

func weaponProficiencyPropertyKey(world InventoryWorld, weapon model.ObjectInstance) string {
	weaponType, ok := objectStringProperty(world, weapon, "type")
	if !ok {
		return ""
	}
	weaponType = strings.TrimSpace(weaponType)
	if name := legacyWeaponProficiencyName(weaponType); name != "" {
		return "proficiency/" + name
	}
	return "proficiency/" + weaponType
}

func getWeaponProficiency(world InventoryWorld, actor model.Creature, weapon model.ObjectInstance) int {
	key := weaponProficiencyPropertyKey(world, weapon)
	if key == "" {
		return 0
	}
	return getCreatureProficiency(actor, key)
}

func incrementCreaturePropertyProficiency(world proficiencyWorld, creature model.Creature, key string, amount int) (model.Creature, error) {
	currentVal := 0
	if valStr, ok := creature.Properties[key]; ok {
		if val, err := strconv.Atoi(strings.TrimSpace(valStr)); err == nil {
			currentVal = val
		}
	} else if val, ok := creature.Stats[key]; ok {
		currentVal = val
	}
	newVal := currentVal + amount
	return world.SetCreatureProperty(creature.ID, key, strconv.Itoa(newVal))
}

func incrementWeaponProficiency(world proficiencyWorld, attacker model.Creature, weapon model.ObjectInstance, amount int) (model.Creature, error) {
	keyName := weaponProficiencyPropertyKey(world, weapon)
	if keyName == "" {
		return attacker, nil
	}
	weaponType, ok := objectStringProperty(world, weapon, "type")
	if !ok {
		return incrementCreaturePropertyProficiency(world, attacker, keyName, amount)
	}
	weaponType = strings.TrimSpace(weaponType)
	keyNum := "proficiency/" + weaponType

	attacker, err := incrementCreaturePropertyProficiency(world, attacker, keyName, amount)
	if err != nil {
		return attacker, err
	}
	if keyNum != keyName {
		attacker, err = incrementCreaturePropertyProficiency(world, attacker, keyNum, amount)
	}
	return attacker, err
}
