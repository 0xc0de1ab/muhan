package command

import (
	"strconv"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type proficiencyWorld interface {
	InventoryWorld
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
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

// legacyWeaponProficiencyGain ports the C addprof formula shared by attack_crt
// (command5.c:391), backstab (command7.c:526), and bash: a landed hit against a
// monster grants (damage * experience) / hpMax proficiency, capped at the
// victim's experience. Callers gate it on the victim being a monster.
func legacyWeaponProficiencyGain(victim model.Creature, damage int) int {
	if damage <= 0 {
		return 0
	}
	experience := creatureStat(victim, "experience")
	hpMax := creatureStat(victim, "hpMax")
	if experience <= 0 || hpMax <= 0 {
		return 0
	}
	gain := damage * experience / hpMax
	if gain > experience {
		gain = experience
	}
	return gain
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
