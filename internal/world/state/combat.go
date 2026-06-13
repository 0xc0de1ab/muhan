package state

import (
	"fmt"
	"log"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"strconv"
	"strings"
)

func (w *World) RecalculateAC(creatureID model.CreatureID) error {
	if w == nil {
		return fmt.Errorf("recalculate creature %q ac: world state is nil", creatureID)
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	fn := w.RecalculateACFunc
	w.rUnlockDomains(true, true, true, true, true, true, true)
	if fn != nil {
		return fn(creatureID)
	}
	_, err := w.RecalculateCreatureAC(creatureID)
	return err
}

func (w *World) RecalculateTHACO(creatureID model.CreatureID) error {
	if w == nil {
		return fmt.Errorf("recalculate creature %q thaco: world state is nil", creatureID)
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	fn := w.RecalculateTHACOFunc
	w.rUnlockDomains(true, true, true, true, true, true, true)
	if fn != nil {
		return fn(creatureID)
	}
	_, err := w.RecalculateCreatureTHACO(creatureID)
	return err
}

func (w *World) GetEffectExpiration(creatureID model.CreatureID, tag string) (int64, bool) {
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	m, ok := w.effectExpirations[creatureID]
	if !ok {
		return 0, false
	}
	expires, ok := m[tag]
	return expires, ok
}

func (w *World) SetEffectExpiration(creatureID model.CreatureID, tag string, expires int64) {
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	m, ok := w.effectExpirations[creatureID]
	if !ok {
		m = map[string]int64{}
		w.effectExpirations[creatureID] = m
	}
	m[tag] = expires
}

// LegacyValues returns the legacy AT_WAR, CALLWAR1, and CALLWAR2 integer values.
func (s FamilyWarSnapshot) LegacyValues() (atWar int, callWar1 int, callWar2 int) {
	if !s.Active.IsZero() {
		atWar = s.Active.First*16 + s.Active.Second
	}
	if !s.Pending.IsZero() {
		callWar1 = s.Pending.First
		callWar2 = s.Pending.Second
	}
	return atWar, callWar1, callWar2
}

func legacyCarryNumberFromCloneSource(sourceID model.ObjectInstanceID) (int, bool) {
	raw := strings.TrimSpace(string(sourceID))
	if !strings.HasPrefix(raw, "legacy-carry:") {
		return 0, false
	}
	idx := strings.LastIndex(raw, ":")
	if idx < 0 || idx == len(raw)-1 {
		return 0, false
	}
	number, err := strconv.Atoi(raw[idx+1:])
	return number, err == nil && number >= 0
}

func (w *World) applyLegacyRandomEnchantRollLocked(object *model.ObjectInstance, roll int) {
	if object == nil {
		return
	}
	adjustment := legacyRandomEnchantAdjustment(roll)
	currentAdjustment, _ := w.objectIntPropertyAnyLocked(*object, "adjustment", "adjust")
	pDice, _ := w.objectIntPropertyAnyLocked(*object, "pDice", "pdice")
	if adjustment > 0 {
		object.Metadata.Tags = addMetadataTags(object.Metadata.Tags, []string{"enchanted", "oencha"})
		currentAdjustment = adjustment
		pDice += adjustment
	}
	if pDice < currentAdjustment {
		pDice = currentAdjustment
	}
	if object.Properties == nil {
		object.Properties = map[string]string{}
	}
	if adjustment > 0 || currentAdjustment != 0 {
		object.Properties["adjustment"] = strconv.Itoa(currentAdjustment)
	}
	if pDice != 0 || currentAdjustment != 0 {
		object.Properties["pDice"] = strconv.Itoa(pDice)
	}
}

func legacyRandomEnchantAdjustment(roll int) int {
	switch {
	case roll > 98:
		return 4
	case roll > 90:
		return 3
	case roll > 80:
		return 2
	case roll > 60:
		return 1
	default:
		return 0
	}
}

func stateLegacyStatBonus(stat int) int {
	stat = clampInt(stat, 0, len(stateLegacyBonusTable)-1)
	return stateLegacyBonusTable[stat]
}

func buildLegacyAliasIndex(groups [][]string) map[string][]string {
	index := make(map[string][]string, len(groups)*2)
	for _, group := range groups {
		normalized := make([]string, 0, len(group))
		seen := make(map[string]struct{}, len(group))
		for _, name := range group {
			norm := normalizeFlagName(name)
			if norm == "" {
				continue
			}
			if _, ok := seen[norm]; ok {
				continue
			}
			seen[norm] = struct{}{}
			normalized = append(normalized, norm)
		}
		for _, norm := range normalized {
			index[norm] = normalized
		}
	}
	return index
}

// backgroundSaver is a minimal worker to pull I/O off the main game loop (C phase start).
func (w *World) backgroundSaver() {
	for req := range w.saveQueue {
		if req.done != nil {
			close(req.done)
			continue
		}
		if !req.playerID.IsZero() {
			if err := w.SavePlayer(req.playerID); err != nil {
				log.Printf("[PERSIST] background SavePlayer failed: %v", err)
			}
		}
		if req.bankID != "" {
			if err := w.SaveBank(req.bankID); err != nil {
				log.Printf("[PERSIST] background SaveBank failed: %v", err)
			}
		}

		if req.boardDir != "" {
			if err := w.SaveBoardPosts(req.boardDir); err != nil {
				log.Printf("[PERSIST] background SaveBoardPosts failed: %v", err)
			}
		}
		if req.familyID > 0 {

			_ = req.familyID
		}
	}
}
