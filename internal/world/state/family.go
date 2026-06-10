package state

import (
	"fmt"
	"muhan/internal/world/model"
	"slices"
	"strings"
	"time"
)

// MarkFamilyNewsDirty marks a family's news as changed (for JSON sidecar persistence).
func (w *World) MarkFamilyNewsDirty(familyID int) {
	if familyID <= 0 {
		return
	}
	w.dirtyMu.Lock()
	if w.familyNewsDirty == nil {
		w.familyNewsDirty = make(map[int]int64)
	}
	w.familyNewsDirty[familyID] = time.Now().Unix()
	w.dirtyMu.Unlock()
}

// AtWar reports whether a family war is active.
func (s FamilyWarSnapshot) AtWar() bool {
	return !s.Active.IsZero()
}

// Family returns a copy of the legacy family registry row with id.
func (w *World) Family(familyID int) (model.Family, bool) {
	if w == nil {
		return model.Family{}, false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	family, ok := w.families[familyID]
	if !ok {
		return model.Family{}, false
	}
	return cloneFamily(family), true
}

// FamilyDisplayName returns the legacy family_str value for familyID.
func (w *World) FamilyDisplayName(familyID int) (string, bool) {
	family, ok := w.Family(familyID)
	if !ok || strings.TrimSpace(family.DisplayName) == "" {
		return "", false
	}
	return family.DisplayName, true
}

// FamilyIDByDisplayName returns the legacy family number for an exact name.
func (w *World) FamilyIDByDisplayName(name string) (int, bool) {
	if w == nil {
		return 0, false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	ids := make([]int, 0, len(w.families))
	for id := range w.families {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if strings.TrimSpace(w.families[id].DisplayName) == name {
			return id, true
		}
	}
	return 0, false
}

// FamilyBossName returns the legacy fmboss_str value for familyID.
func (w *World) FamilyBossName(familyID int) (string, bool) {
	family, ok := w.Family(familyID)
	if !ok || strings.TrimSpace(family.BossName) == "" {
		return "", false
	}
	return family.BossName, true
}

// HasMarriageInvite reports whether playerID is invited for a marriage/special room ID.
func (w *World) HasMarriageInvite(playerID model.PlayerID, specialID model.SpecialID) bool {
	if w == nil || specialID.IsZero() {
		return false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	player, ok := w.players[playerID]
	if !ok {
		return false
	}
	return w.hasMarriageInviteLocked(player, specialID)
}

// MarriageInvites returns a copy of the runtime invite names for a marriage/special room ID.
func (w *World) MarriageInvites(specialID model.SpecialID) []string {
	if w == nil || specialID.IsZero() {
		return nil
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	return slices.Clone(w.marriageInvites[specialID])
}

// AddMarriageInvite appends name to a runtime marriage/special room invite list.
// It returns false when the trimmed name is already present.
func (w *World) AddMarriageInvite(specialID model.SpecialID, name string) (bool, error) {
	if w == nil {
		return false, fmt.Errorf("add marriage invite %q: world state is nil", specialID)
	}
	if specialID.IsZero() {
		return false, fmt.Errorf("add marriage invite: special id is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false, fmt.Errorf("add marriage invite %q: name is required", specialID)
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	if w.marriageInvites == nil {
		w.marriageInvites = map[model.SpecialID][]string{}
	}
	for _, existing := range w.marriageInvites[specialID] {
		if strings.TrimSpace(existing) == name {
			return false, nil
		}
	}
	w.marriageInvites[specialID] = append(w.marriageInvites[specialID], name)
	return true, nil
}

// FamilyWarSnapshot returns a copy of the current family war state.
func (w *World) FamilyWarSnapshot() FamilyWarSnapshot {
	if w == nil {
		return FamilyWarSnapshot{}
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)
	return w.familyWar
}

// FamilyWarLegacyValues returns the current AT_WAR/CALLWAR1/CALLWAR2 equivalents.
func (w *World) FamilyWarLegacyValues() (atWar int, callWar1 int, callWar2 int) {
	return w.FamilyWarSnapshot().LegacyValues()
}

// RequestFamilyWar creates a pending family war request under the world lock.
func (w *World) RequestFamilyWar(callerFamily, targetFamily int) (FamilyWarSnapshot, error) {
	if w == nil {
		return FamilyWarSnapshot{}, fmt.Errorf("request family war: world state is nil")
	}
	if err := validateFamilyWarPair(callerFamily, targetFamily); err != nil {
		return FamilyWarSnapshot{}, err
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	if !w.familyWar.Active.IsZero() {
		return w.familyWar, ErrFamilyWarActive
	}
	if !w.familyWar.Pending.IsZero() {
		return w.familyWar, ErrFamilyWarPending
	}
	w.familyWar.Pending = FamilyWarPair{First: callerFamily, Second: targetFamily}
	return w.familyWar, nil
}

// AcceptFamilyWar accepts a pending family war request under the world lock.
// acceptingFamily is the pending target and callerFamily is the pending caller.
func (w *World) AcceptFamilyWar(acceptingFamily, callerFamily int) (FamilyWarSnapshot, error) {
	if w == nil {
		return FamilyWarSnapshot{}, fmt.Errorf("accept family war: world state is nil")
	}
	if err := validateFamilyWarPair(acceptingFamily, callerFamily); err != nil {
		return FamilyWarSnapshot{}, err
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	if !w.familyWar.Active.IsZero() {
		return w.familyWar, ErrFamilyWarActive
	}
	if w.familyWar.Pending.IsZero() {
		return w.familyWar, ErrFamilyWarNoPending
	}
	if w.familyWar.Pending.Second != acceptingFamily {
		return w.familyWar, ErrFamilyWarNotTarget
	}
	if w.familyWar.Pending.First != callerFamily {
		return w.familyWar, ErrFamilyWarPending
	}
	w.familyWar.Active = FamilyWarPair{First: acceptingFamily, Second: callerFamily}
	w.familyWar.Pending = FamilyWarPair{}
	return w.familyWar, nil
}

// CancelFamilyWar cancels a pending family war request by its original caller.
func (w *World) CancelFamilyWar(callerFamily int) (FamilyWarSnapshot, error) {
	if w == nil {
		return FamilyWarSnapshot{}, fmt.Errorf("cancel family war: world state is nil")
	}
	if callerFamily <= 0 {
		return FamilyWarSnapshot{}, ErrFamilyWarInvalidFamily
	}

	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	if w.familyWar.Pending.IsZero() {
		return w.familyWar, ErrFamilyWarNoPending
	}
	if w.familyWar.Pending.First != callerFamily {
		return w.familyWar, ErrFamilyWarNotRequester
	}
	w.familyWar.Pending = FamilyWarPair{}
	return w.familyWar, nil
}

// ClearFamilyWar clears both active and pending family war state. This mirrors
// the legacy boss-death reset of AT_WAR/CALLWAR1/CALLWAR2.
func (w *World) ClearFamilyWar() FamilyWarSnapshot {
	if w == nil {
		return FamilyWarSnapshot{}
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)

	w.familyWar = FamilyWarSnapshot{}
	return w.familyWar
}

// EndActiveFamilyWar ends current war and returns the final active snapshot.
func (w *World) EndActiveFamilyWar(reason string) FamilyWarSnapshot {
	if w == nil {
		return FamilyWarSnapshot{}
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	if w.familyWar.Active.IsZero() {
		return w.familyWar
	}
	snap := w.familyWar
	w.familyWar = FamilyWarSnapshot{}
	return snap
}

// FamiliesAtWar reports whether the two family ids match the active war.
func (w *World) FamiliesAtWar(firstFamily, secondFamily int) bool {
	if w == nil || firstFamily <= 0 || secondFamily <= 0 {
		return false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	active := w.familyWar.Active
	if active.IsZero() {
		return false
	}
	return (firstFamily == active.First && secondFamily == active.Second) ||
		(firstFamily == active.Second && secondFamily == active.First)
}

// FamilyAtWar reports whether family id is one side of the active war.
func (w *World) FamilyAtWar(family int) bool {
	if w == nil || family <= 0 {
		return false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	active := w.familyWar.Active
	return family == active.First || family == active.Second
}

// UpdateFamilyMembers updates the in-memory members list of a family.
func (w *World) UpdateFamilyMembers(familyID int, members []model.FamilyMember) error {
	if w == nil {
		return fmt.Errorf("update family members: world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	family, ok := w.families[familyID]
	if !ok {

		found := false
		for id, f := range w.families {
			if f.Slot == familyID {
				family = f
				familyID = id
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("update family members: family %d not found", familyID)
		}
	}
	family.Members = members
	w.families[familyID] = family
	return nil
}

func (w *World) familyByLegacyNumberLocked(familyID int) (int, model.Family, bool) {
	family, ok := w.families[familyID]
	if ok {
		return familyID, family, true
	}
	for id, family := range w.families {
		if family.Slot == familyID {
			return id, family, true
		}
	}
	return 0, model.Family{}, false
}

// UpdateFamily updates a family in the world state.
func (w *World) UpdateFamily(family model.Family) error {
	if w == nil {
		return fmt.Errorf("update family: world is nil")
	}
	w.lockDomains(true, true, true, true, true, true, true)
	defer w.unlockDomains(true, true, true, true, true, true, true)
	w.families[family.ID] = family
	return nil
}

func (w *World) hasMarriageInviteLocked(player model.Player, specialID model.SpecialID) bool {
	if w == nil || specialID.IsZero() {
		return false
	}
	for _, name := range w.marriageInvites[specialID] {
		if movePlayerInviteNameMatches(player, name) {
			return true
		}
	}
	return false
}

func validateFamilyWarPair(firstFamily, secondFamily int) error {
	if firstFamily <= 0 || secondFamily <= 0 {
		return ErrFamilyWarInvalidFamily
	}
	if firstFamily == secondFamily {
		return ErrFamilyWarSelf
	}
	return nil
}

func cloneFamily(family model.Family) model.Family {
	family.Members = slices.Clone(family.Members)
	for i := range family.Members {
		family.Members[i].Metadata = cloneMetadata(family.Members[i].Metadata)
	}
	family.Metadata = cloneMetadata(family.Metadata)
	return family
}
