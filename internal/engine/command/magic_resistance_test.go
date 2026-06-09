package command

import (
	"testing"

	"muhan/internal/world/model"
)

type mockAggroWorld struct {
	addedEnemyAttacker model.CreatureID
	addedEnemyDefender model.CreatureID
	addEnemyCalled     bool
	appliedDamageID    model.CreatureID
	appliedDamageVal   int
	applyDamageCalled  bool
}

func (m *mockAggroWorld) AddEnemy(attacker, defender model.CreatureID) (bool, error) {
	m.addedEnemyAttacker = attacker
	m.addedEnemyDefender = defender
	m.addEnemyCalled = true
	return true, nil
}

func (m *mockAggroWorld) ApplyCreatureDamage(id model.CreatureID, dmg int) (model.Creature, int, bool, error) {
	m.appliedDamageID = id
	m.appliedDamageVal = dmg
	m.applyDamageCalled = true
	return model.Creature{}, 0, false, nil
}

func (m *mockAggroWorld) Player(id model.PlayerID) (model.Player, bool) {
	return model.Player{ID: id, CreatureID: model.CreatureID(id)}, true
}

func (m *mockAggroWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	return model.Creature{ID: id}, true
}

func (m *mockAggroWorld) Object(id model.ObjectInstanceID) (model.ObjectInstance, bool) {
	return model.ObjectInstance{}, false
}

func (m *mockAggroWorld) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	return model.ObjectPrototype{}, false
}

func (m *mockAggroWorld) Room(id model.RoomID) (model.Room, bool) {
	return model.Room{}, false
}

func TestGetSpellResistanceTag(t *testing.T) {
	tests := []struct {
		magicPower int
		want       string
	}{
		{magicPowerBurn, "resistFire"},
		{magicPowerFireball, "resistFire"},
		{magicPowerBlister, "resistCold"},
		{magicPowerWaterBolt, "resistCold"},
		{magicPowerHurt, "resistMagic"},
		{magicPowerShockbolt, "resistMagic"},
		{magicPowerDustGust, "resistMagic"},
		{magicPowerStoneCrush, "resistAcid"},
		{magicPowerRumble, "resistAcid"},
		{999, ""},
	}

	for _, tt := range tests {
		got := GetSpellResistanceTag(tt.magicPower)
		if got != tt.want {
			t.Errorf("GetSpellResistanceTag(%d) = %q, want %q", tt.magicPower, got, tt.want)
		}
	}
}

func TestApplyElementalResistance(t *testing.T) {
	tests := []struct {
		name       string
		tags       []string
		stats      map[string]int
		magicPower int
		damage     int
		wantDamage int
	}{
		{
			name:       "No resistance tags",
			tags:       []string{},
			stats:      map[string]int{"piety": 20, "intelligence": 20},
			magicPower: magicPowerFireball,
			damage:     100,
			wantDamage: 100,
		},
		{
			name:       "Has resistFire tag, normal reduction",
			tags:       []string{"resistFire"},
			stats:      map[string]int{"piety": 10, "intelligence": 15}, // sum = 25
			magicPower: magicPowerFireball,
			damage:     100,
			wantDamage: 50, // 100 - (100 * 2 * 25) / 100 = 50
		},
		{
			name:       "Has resistFire tag with alias",
			tags:       []string{"PRFIRE"},
			stats:      map[string]int{"piety": 10, "intelligence": 15}, // sum = 25
			magicPower: magicPowerFireball,
			damage:     100,
			wantDamage: 50,
		},
		{
			name:       "Learned resist fire spell is not active resistance",
			tags:       []string{"SRFIRE"},
			stats:      map[string]int{"piety": 10, "intelligence": 15},
			magicPower: magicPowerFireball,
			damage:     100,
			wantDamage: 100,
		},
		{
			name:       "Has resistCold tag, normal reduction",
			tags:       []string{"resistCold"},
			stats:      map[string]int{"piety": 20, "intelligence": 20}, // sum = 40
			magicPower: magicPowerWaterBolt,
			damage:     100,
			wantDamage: 20, // 100 - (100 * 2 * 40) / 100 = 20
		},
		{
			name:       "Has resistMagic tag, cap at 50",
			tags:       []string{"resistMagic"},
			stats:      map[string]int{"piety": 30, "intelligence": 30}, // sum = 60 -> capped at 50
			magicPower: magicPowerShockbolt,
			damage:     100,
			wantDamage: 1, // 100 - (100 * 2 * 50) / 100 = 0 -> minimum 1
		},
		{
			name:       "Learned resist magic spell is not active resistance",
			tags:       []string{"SRMAGI"},
			stats:      map[string]int{"piety": 30, "intelligence": 30},
			magicPower: magicPowerShockbolt,
			damage:     100,
			wantDamage: 100,
		},
		{
			name:       "Has resistAcid tag, normal reduction",
			tags:       []string{"resistAcid"},
			stats:      map[string]int{"piety": 5, "intelligence": 5}, // sum = 10
			magicPower: magicPowerStoneCrush,
			damage:     50,
			wantDamage: 40, // 50 - (50 * 2 * 10) / 100 = 40
		},
		{
			name:       "No corresponding tag",
			tags:       []string{"resistCold"},
			stats:      map[string]int{"piety": 20, "intelligence": 20},
			magicPower: magicPowerFireball,
			damage:     100,
			wantDamage: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := model.Creature{
				Metadata: model.Metadata{
					Tags: tt.tags,
				},
				Stats: tt.stats,
			}
			got := ApplyElementalResistance(target, tt.magicPower, tt.damage)
			if got != tt.wantDamage {
				t.Errorf("ApplyElementalResistance() = %d, want %d", got, tt.wantDamage)
			}
		})
	}
}

func TestRegisterSpellAggro(t *testing.T) {
	victim := model.CreatureID("creature:goblin")
	attacker := model.CreatureID("creature:alice")

	t.Run("World implements AddEnemy", func(t *testing.T) {
		world := &mockAggroWorld{}
		RegisterSpellAggro(world, victim, attacker)

		if !world.addEnemyCalled {
			t.Fatal("AddEnemy was not called")
		}
		if world.addedEnemyAttacker != victim {
			t.Errorf("addedEnemyAttacker = %q, want %q", world.addedEnemyAttacker, victim)
		}
		if world.addedEnemyDefender != attacker {
			t.Errorf("addedEnemyDefender = %q, want %q", world.addedEnemyDefender, attacker)
		}
	})

	t.Run("World does not implement AddEnemy", func(t *testing.T) {
		world := struct{}{}
		// Should not panic
		RegisterSpellAggro(world, victim, attacker)
	})
}

func TestMagicEffectApplyDamageIntegration(t *testing.T) {
	// Mock attackRoll to always return a deterministic dice result.
	// rollDice(n, s, p) returns n*s + p.
	// dice {nDice: 1, sDice: 1, pDice: 99} will result in 1*1 + 99 = 100.
	previousRoll := attackRoll
	attackRoll = func(min, max int) int {
		return max
	}
	defer func() {
		attackRoll = previousRoll
	}()

	t.Run("Apply damage with resistance and aggro registration (Monster target)", func(t *testing.T) {
		world := &mockAggroWorld{}
		actor := model.Creature{
			ID:   "creature:goblin",
			Kind: model.CreatureKindMonster,
			Stats: map[string]int{
				"piety":        10,
				"intelligence": 15,
				"hpCurrent":    100,
				"hpMax":        100,
			},
			Metadata: model.Metadata{
				Tags: []string{"resistFire"},
			},
		}

		ctx := &Context{ActorID: "player:goblin"}
		resolved := ResolvedCommand{
			Args: []string{"fireball", "나"}, // Cast on self
		}

		dice := magicEffectDamageDice{nDice: 1, sDice: 1, pDice: 99} // 100 base damage

		success, err := magicEffectApplyDamage(ctx, world, actor, model.ObjectInstance{}, resolved, magicPowerFireball, dice)
		if err != nil {
			t.Fatalf("magicEffectApplyDamage error = %v", err)
		}
		if !success {
			t.Fatal("magicEffectApplyDamage returned false")
		}

		// Verify damage reduction: 100 -> reduced by 50% (25+25) = 50 -> remaining 50
		if !world.applyDamageCalled {
			t.Fatal("ApplyCreatureDamage was not called")
		}
		if world.appliedDamageVal != 50 {
			t.Errorf("appliedDamageVal = %d, want 50", world.appliedDamageVal)
		}

		// Verify aggro registration for monster: actor is "creature:goblin"
		if !world.addEnemyCalled {
			t.Fatal("AddEnemy was not called")
		}
		if world.addedEnemyAttacker != "creature:goblin" || world.addedEnemyDefender != "creature:goblin" {
			t.Errorf("AddEnemy arguments = (%q, %q), want (creature:goblin, creature:goblin)", world.addedEnemyAttacker, world.addedEnemyDefender)
		}
	})

	t.Run("Apply damage without resistance and no aggro (Player target)", func(t *testing.T) {
		world := &mockAggroWorld{}
		actor := model.Creature{
			ID:   "creature:alice",
			Kind: model.CreatureKindPlayer, // Player type
			Stats: map[string]int{
				"piety":        25,
				"intelligence": 25,
				"hpCurrent":    100,
				"hpMax":        100,
			},
			Metadata: model.Metadata{
				Tags: []string{}, // No resistFire
			},
		}

		ctx := &Context{ActorID: "player:alice"}
		resolved := ResolvedCommand{
			Args: []string{"fireball", "나"},
		}

		dice := magicEffectDamageDice{nDice: 1, sDice: 1, pDice: 99} // 100 base damage

		success, err := magicEffectApplyDamage(ctx, world, actor, model.ObjectInstance{}, resolved, magicPowerFireball, dice)
		if err != nil {
			t.Fatalf("magicEffectApplyDamage error = %v", err)
		}
		if !success {
			t.Fatal("magicEffectApplyDamage returned false")
		}

		// Verify damage: should be 100 (no reduction)
		if !world.applyDamageCalled {
			t.Fatal("ApplyCreatureDamage was not called")
		}
		if world.appliedDamageVal != 100 {
			t.Errorf("appliedDamageVal = %d, want 100", world.appliedDamageVal)
		}

		// Verify no aggro registration since target is not a monster
		if world.addEnemyCalled {
			t.Fatal("AddEnemy should not have been called for player target")
		}
	})
}
