package command

import (
	"testing"

	"github.com/0xc0de1ab/muhan/internal/world/model"
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

func TestApplyMagicResistanceDamage(t *testing.T) {
	tests := []struct {
		name   string
		tags   []string
		stats  map[string]int
		damage int
		want   int
	}{
		{
			name:   "no magic-resist flag takes full damage",
			stats:  map[string]int{"piety": 20, "intelligence": 20},
			damage: 100,
			want:   100,
		},
		{
			name:   "MRMAGI reduces (sum 25)",
			tags:   []string{"MRMAGI"},
			stats:  map[string]int{"piety": 10, "intelligence": 15},
			damage: 100,
			want:   50, // 100 - (100 * 2 * 25) / 100
		},
		{
			name:   "PRMAGI reduces (sum 40) — same alias group as MRMAGI",
			tags:   []string{"PRMAGI"},
			stats:  map[string]int{"piety": 20, "intelligence": 20},
			damage: 100,
			want:   20, // 100 - (100 * 2 * 40) / 100
		},
		{
			name:   "sum capped at 50 reaches 0 damage (no floor)",
			tags:   []string{"MRMAGI"},
			stats:  map[string]int{"piety": 30, "intelligence": 30}, // 60 -> capped 50
			damage: 100,
			want:   0, // 100 - (100 * 2 * 50) / 100 = 0; C applies no floor
		},
		{
			name:   "magic-resist stored as a stat flag",
			stats:  map[string]int{"piety": 5, "intelligence": 5, "PRMAGI": 1},
			damage: 50,
			want:   40, // 50 - (50 * 2 * 10) / 100
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := model.Creature{
				Metadata: model.Metadata{Tags: tt.tags},
				Stats:    tt.stats,
			}
			got := applyMagicResistanceDamage(target, tt.damage)
			if got != tt.want {
				t.Errorf("applyMagicResistanceDamage() = %d, want %d", got, tt.want)
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
				Tags: []string{"MRMAGI"},
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
