package game

import (
	"fmt"
	"strings"

	enginecmd "muhan/internal/engine/command"
	"muhan/internal/world/model"
)

// FamilyMemberWorld is the state surface needed by the legacy family_member
// command.
type FamilyMemberWorld interface {
	FamilyWorld
	Families() []model.Family
}

// NewFamilyMemberHandler renders the legacy family_member/패거리원 membership
// list from the loaded family registry.
func NewFamilyMemberHandler(world FamilyMemberWorld) enginecmd.Handler {
	return func(ctx *enginecmd.Context, _ enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}
		if world == nil {
			return enginecmd.StatusDefault, fmt.Errorf("game: family member world is nil")
		}

		familyID, ok := playerFamilyMembership(world, model.PlayerID(ctx.ActorID))
		if !ok {
			ctx.WriteString("당신은 패거리에 가입되어 있지 않습니다.")
			return enginecmd.StatusDefault, nil
		}

		family, found := familyMemberFamily(world.Families(), familyID)
		ctx.WriteString(fmt.Sprintf("당신은 [%s] 패거리에 가입되어 있습니다.\n", familyMemberDisplayName(world, familyID, family, found)))

		count := 0
		if found {
			for _, member := range family.Members {
				name := strings.TrimSpace(member.DisplayName)
				if name == "" {
					continue
				}
				ctx.WriteString(fmt.Sprintf("[%s]  %-15s  ", familyMemberClassName(member.Class), name))
				count++
				if count%3 == 0 {
					ctx.WriteString("\n")
				}
			}
		}
		if count%3 != 0 {
			ctx.WriteString("\n")
		}
		ctx.WriteString(fmt.Sprintf("총 %d명의 사람들이 가입되어 있습니다.\n", count))
		return enginecmd.StatusDefault, nil
	}
}

func familyMemberFamily(families []model.Family, familyID int) (model.Family, bool) {
	for _, family := range families {
		if family.ID == familyID {
			return family, true
		}
	}
	for _, family := range families {
		if family.Slot == familyID {
			return family, true
		}
	}
	return model.Family{}, false
}

func familyMemberDisplayName(world any, familyID int, family model.Family, found bool) string {
	if found {
		if name := strings.TrimSpace(family.DisplayName); name != "" {
			return name
		}
	}
	return familyDisplayNameFrom(world, familyID)
}

func familyMemberClassName(class int) string {
	return legacyFixedByteLabel(shortWhoisClassName(whoisClassName(class)), 4)
}
