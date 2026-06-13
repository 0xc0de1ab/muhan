package game

import (
	"fmt"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/state"
)

// FamilyWarWorld is the state surface needed for the legacy call_war transition.
type FamilyWarWorld interface {
	FamilyWarSnapshot() state.FamilyWarSnapshot
	RequestFamilyWar(callerFamily, targetFamily int) (state.FamilyWarSnapshot, error)
	AcceptFamilyWar(acceptingFamily, callerFamily int) (state.FamilyWarSnapshot, error)
	CancelFamilyWar(callerFamily int) (state.FamilyWarSnapshot, error)
}

type CallWarTransition string

const (
	CallWarRequested       CallWarTransition = "requested"
	CallWarCanceled        CallWarTransition = "canceled"
	CallWarAccepted        CallWarTransition = "accepted"
	CallWarRejectedActive  CallWarTransition = "rejected_active"
	CallWarRejectedPending CallWarTransition = "rejected_pending"
)

type CallWarResult struct {
	Transition CallWarTransition
	Snapshot   state.FamilyWarSnapshot
}

type familyDisplayNameLookup interface {
	FamilyDisplayName(familyID int) (string, bool)
}

type familyDisplayIDLookup interface {
	FamilyIDByDisplayName(name string) (int, bool)
}

type familyBossNameLookup interface {
	FamilyBossName(familyID int) (string, bool)
}

type familyWarSnapshotLookup interface {
	FamilyWarSnapshot() state.FamilyWarSnapshot
}

// ResolveCallWarState applies the state-only portion of legacy call_war.
// Caller authority, family-name lookup, target boss online checks, and
// broadcasts are intentionally left to the command wiring layer.
func ResolveCallWarState(world FamilyWarWorld, callerFamily, targetFamily int) (CallWarResult, error) {
	if world == nil {
		return CallWarResult{}, fmt.Errorf("resolve call war: world state is nil")
	}
	if callerFamily <= 0 || targetFamily <= 0 {
		return CallWarResult{}, state.ErrFamilyWarInvalidFamily
	}
	if callerFamily == targetFamily {
		return CallWarResult{}, state.ErrFamilyWarSelf
	}

	snapshot := world.FamilyWarSnapshot()
	if snapshot.AtWar() {
		return CallWarResult{Transition: CallWarRejectedActive, Snapshot: snapshot}, nil
	}
	if !snapshot.HasPending() {
		next, err := world.RequestFamilyWar(callerFamily, targetFamily)
		return CallWarResult{Transition: CallWarRequested, Snapshot: next}, err
	}
	if snapshot.Pending.First == callerFamily {
		next, err := world.CancelFamilyWar(callerFamily)
		return CallWarResult{Transition: CallWarCanceled, Snapshot: next}, err
	}
	if snapshot.Pending.Second == callerFamily && snapshot.Pending.First == targetFamily {
		next, err := world.AcceptFamilyWar(callerFamily, targetFamily)
		return CallWarResult{Transition: CallWarAccepted, Snapshot: next}, err
	}
	return CallWarResult{Transition: CallWarRejectedPending, Snapshot: snapshot}, nil
}

func familyDisplayNameFrom(world any, familyID int) string {
	if familyID <= 0 {
		return "패거리0"
	}
	if lookup, ok := world.(familyDisplayNameLookup); ok {
		if name, found := lookup.FamilyDisplayName(familyID); found {
			if name = strings.TrimSpace(name); name != "" {
				return name
			}
		}
	}
	return fmt.Sprintf("패거리%d", familyID)
}

func familyIDFromDisplayName(world any, name string) (int, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, false
	}
	lookup, ok := world.(familyDisplayIDLookup)
	if !ok {
		return 0, false
	}
	familyID, found := lookup.FamilyIDByDisplayName(name)
	return familyID, found && familyID > 0
}

func familyBossNameFrom(world any, familyID int) (string, bool) {
	if familyID <= 0 {
		return "", false
	}
	if lookup, ok := world.(familyBossNameLookup); ok {
		name, found := lookup.FamilyBossName(familyID)
		name = strings.TrimSpace(name)
		return name, found && name != ""
	}
	return "", false
}

func familyWarStatusLine(world any) string {
	lookup, ok := world.(familyWarSnapshotLookup)
	if !ok {
		return ""
	}
	snapshot := lookup.FamilyWarSnapshot()
	if !snapshot.AtWar() {
		return ""
	}
	return fmt.Sprintf("\n%s 패거리는 %s 패거리와 전쟁중입니다.\n",
		familyDisplayNameFrom(world, snapshot.Active.First),
		familyDisplayNameFrom(world, snapshot.Active.Second))
}

// EndWar ends active war with reason (for end conditions like boss death).
func EndWar(world any, reason string) state.FamilyWarSnapshot {
	if ext, ok := world.(interface {
		EndActiveFamilyWar(string) state.FamilyWarSnapshot
	}); ok {
		return ext.EndActiveFamilyWar(reason)
	}
	return state.FamilyWarSnapshot{}
}
