package game

import (
	"fmt"
	"strconv"
	"strings"

	enginecmd "github.com/0xc0de1ab/muhan/internal/engine/command"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

// FamilyRegistryWorld is the state surface needed by the legacy list_family
// command.
type FamilyRegistryWorld interface {
	Families() []model.Family
}

type familyBankBalanceLookup interface {
	FamilyBankBalance(familyID int) (int, bool)
}

type familyBankObjectLookup interface {
	Bank(model.BankID) (model.BankAccount, bool)
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
}

// NewListFamilyHandler renders the legacy list_family/모든패거리 registry view.
func NewListFamilyHandler(world FamilyRegistryWorld) enginecmd.Handler {
	return func(ctx *enginecmd.Context, _ enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil {
			return enginecmd.StatusDefault, fmt.Errorf("game: family registry context missing")
		}
		if world == nil {
			return enginecmd.StatusDefault, fmt.Errorf("game: family registry world is nil")
		}

		families := activeRegistryFamilies(world.Families())
		ctx.WriteString("다음과 같은 패거리가 있습니다.\n")
		ctx.WriteString(fmt.Sprintf("%-14s %-14s %-17s %s\n", "패거리이름", "문주이름", "가입축하금", "패거리금고"))
		ctx.WriteString("---------------------------------------------------------------\n")
		for _, family := range families {
			ctx.WriteString(fmt.Sprintf("%-14s %-14s %6d만냥",
				strings.TrimSpace(family.DisplayName),
				strings.TrimSpace(family.BossName),
				family.JoinSubsidy,
			))
			if balance, ok := familyBankBalance(world, family); ok {
				ctx.WriteString(fmt.Sprintf(" %13d만냥", balance))
			}
			ctx.WriteString("\n")
		}
		ctx.WriteString(fmt.Sprintf("\n총 %d 개의 패거리가 활동중에 있습니다.", len(families)))
		return enginecmd.StatusDefault, nil
	}
}

func activeRegistryFamilies(families []model.Family) []model.Family {
	active := make([]model.Family, 0, len(families))
	for _, family := range families {
		if family.Slot <= 0 {
			continue
		}
		active = append(active, family)
	}
	return active
}

func familyBankBalance(world any, family model.Family) (int, bool) {
	if lookup, ok := world.(familyBankBalanceLookup); ok {
		if balance, found := lookup.FamilyBankBalance(family.ID); found {
			return balance, true
		}
	}

	lookup, ok := world.(familyBankObjectLookup)
	if !ok {
		return 0, false
	}
	for _, bankID := range familyBankCandidateIDs(family) {
		account, found := lookup.Bank(bankID)
		if !found {
			continue
		}
		if balance, ok := familyBankAccountValue(lookup, account); ok {
			return balance, true
		}
	}
	return 0, false
}

func familyBankCandidateIDs(family model.Family) []model.BankID {
	name := strings.TrimSpace(family.DisplayName)
	if name == "" {
		return nil
	}
	candidates := []string{
		"bank:family:" + name + "_0",
		"bank:family:" + name,
	}
	seen := make(map[string]struct{}, len(candidates))
	ids := make([]model.BankID, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		ids = append(ids, model.BankID(candidate))
	}
	return ids
}

func familyBankAccountValue(lookup familyBankObjectLookup, account model.BankAccount) (int, bool) {
	for _, objectID := range account.Objects.ObjectIDs {
		object, found := lookup.Object(objectID)
		if !found {
			continue
		}
		value, ok := familyObjectIntProperty(object, "value")
		if ok && value >= 0 {
			return value, true
		}
	}
	return 0, false
}

func familyObjectIntProperty(object model.ObjectInstance, key string) (int, bool) {
	raw := strings.TrimSpace(object.Properties[key])
	if raw == "" {
		return 0, false
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return value, true
}
