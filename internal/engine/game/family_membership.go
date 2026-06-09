package game

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	enginecmd "muhan/internal/engine/command"
	"muhan/internal/krtext"
	"muhan/internal/persist/legacykr"
	"muhan/internal/session"
	"muhan/internal/world/model"
)

// FamilyMembershipWorld is the mutable state surface needed by the safe subset
// of the legacy family join/leave commands.
type FamilyMembershipWorld interface {
	FamilyMemberWorld
	UpdateCreatureFamilyState(model.CreatureID, int, bool, bool, bool) (model.Creature, error)
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
	Bank(model.BankID) (model.BankAccount, bool)
	DepositCreatureGoldToObjectValueScaled(model.CreatureID, model.ObjectInstanceID, int, int, int, int) (int, int, bool, bool, error)
	WithdrawObjectValueToCreatureGoldScaled(model.ObjectInstanceID, model.CreatureID, int, int) (int, int, bool, error)
	UpdateCreatureGold(model.CreatureID, int) (model.Creature, error)
	UpdateFamilyMembers(int, []model.FamilyMember) error
	UpdateFamily(model.Family) error
	UpdatePlayer(model.Player) error
}

type familyMembershipPlayersWorld interface {
	Players() []model.Player
}

type familyMembershipDirtyWorld interface {
	MarkPlayerDirty(model.PlayerID)
}

type familyMembershipSaveWorld interface {
	MarkPlayerDirty(model.PlayerID)
	QueueSave(model.PlayerID, model.BankID)
}

type familyJoinRequest struct {
	PlayerID    model.PlayerID
	CreatureID  model.CreatureID
	DisplayName string
	FamilyID    int
	FamilyName  string
}

// FamilyMembershipRequests stores non-persistent join applications shared by
// the family/boss_family/fm_dis handlers.
type FamilyMembershipRequests struct {
	mu       sync.Mutex
	byPlayer map[model.PlayerID]familyJoinRequest
}

func NewFamilyMembershipRequests() *FamilyMembershipRequests {
	return &FamilyMembershipRequests{byPlayer: map[model.PlayerID]familyJoinRequest{}}
}

var defaultFamilyMembershipRequests = NewFamilyMembershipRequests()

func init() {
	enginecmd.RegisterFamilyMemberClassChangePersister(PersistFamilyMemberClassChange)
}

func NewFamilyJoinHandler(world FamilyMembershipWorld, stores ...*FamilyMembershipRequests) enginecmd.Handler {
	store := familyMembershipRequestStore(stores...)
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		player, creature, ok, err := familyMembershipActor(world, ctx)
		if err != nil || !ok {
			return enginecmd.StatusDefault, err
		}
		if handled := familyMembershipRejectExistingJoinState(ctx, world, store, player, creature); handled {
			return enginecmd.StatusDefault, nil
		}

		target := strings.TrimSpace(strings.Join(resolved.Args, " "))
		if target == "" {
			if !familyMembershipWriteJoinTargets(ctx, world) {
				return enginecmd.StatusDefault, nil
			}
			state := &familyJoinState{
				world:      world,
				store:      store,
				playerID:   player.ID,
				creatureID: creature.ID,
			}
			if !enginecmd.SetPendingLineHandler(ctx, state.chooseFamily) {
				return enginecmd.StatusDefault, fmt.Errorf("패거리 가입 선택 상태를 시작할 수 없습니다")
			}
			return enginecmd.StatusDoPrompt, nil
		}

		state := &familyJoinState{
			world:      world,
			store:      store,
			playerID:   player.ID,
			creatureID: creature.ID,
		}
		return state.startConfirmation(ctx, target, false)
	}
}

type familyJoinState struct {
	world      FamilyMembershipWorld
	store      *FamilyMembershipRequests
	playerID   model.PlayerID
	creatureID model.CreatureID
	familyID   int
	familyName string
}

func (s *familyJoinState) chooseFamily(ctx *enginecmd.Context, line string) (enginecmd.Status, error) {
	target := strings.TrimSpace(line)
	if target == "" {
		enginecmd.ClearPendingLineHandler(ctx)
		ctx.WriteString("\n잘못된 선택입니다.\n")
		return enginecmd.StatusDefault, nil
	}
	return s.startConfirmation(ctx, target, true)
}

func (s *familyJoinState) startConfirmation(ctx *enginecmd.Context, target string, promptInput bool) (enginecmd.Status, error) {
	player, creature, ok, err := familyMembershipActorByID(s.world, s.playerID, s.creatureID)
	if err != nil || !ok {
		return enginecmd.StatusDefault, err
	}
	if handled := familyMembershipRejectExistingJoinState(ctx, s.world, s.store, player, creature); handled {
		enginecmd.ClearPendingLineHandler(ctx)
		return enginecmd.StatusDefault, nil
	}
	family, found := familyMembershipResolveFamily(s.world, target)
	if !found {
		enginecmd.ClearPendingLineHandler(ctx)
		if promptInput {
			ctx.WriteString("\n잘못된 선택입니다.\n")
		} else {
			ctx.WriteString("잘못된 패거리입니다.\n")
		}
		return enginecmd.StatusDefault, nil
	}
	if familyMembershipFamilyClosed(family) {
		enginecmd.ClearPendingLineHandler(ctx)
		ctx.WriteString("단체의 패거리입니다.")
		return enginecmd.StatusDefault, nil
	}
	familyName := familyMembershipFamilyName(s.world, family)
	bossName := familyMembershipFamilyBossName(s.world, family)
	if !familyMembershipBossOnline(ctx, s.world, family.ID, bossName) {
		enginecmd.ClearPendingLineHandler(ctx)
		ctx.WriteString(fmt.Sprintf("패거리의 두목인 %s님은 현재 이용중이 아닙니다.", bossName))
		return enginecmd.StatusDefault, nil
	}

	s.familyID = family.ID
	s.familyName = familyName
	ctx.WriteString(fmt.Sprintf("%s에 가입을 하시겠습니까? (예/아니오) ", familyName))
	if !enginecmd.SetPendingLineHandler(ctx, s.confirm) {
		return enginecmd.StatusDefault, fmt.Errorf("패거리 가입 확인 상태를 시작할 수 없습니다")
	}
	return enginecmd.StatusDoPrompt, nil
}

func (s *familyJoinState) confirm(ctx *enginecmd.Context, line string) (enginecmd.Status, error) {
	enginecmd.ClearPendingLineHandler(ctx)
	player, creature, ok, err := familyMembershipActorByID(s.world, s.playerID, s.creatureID)
	if err != nil || !ok {
		return enginecmd.StatusDefault, err
	}
	if handled := familyMembershipRejectExistingJoinState(ctx, s.world, s.store, player, creature); handled {
		return enginecmd.StatusDefault, nil
	}
	if strings.TrimSpace(line) != "예" {
		ctx.WriteString("\n가입 신청을 취소합니다.")
		return enginecmd.StatusDefault, nil
	}
	family, found := familyMemberFamily(s.world.Families(), s.familyID)
	if !found {
		return enginecmd.StatusDefault, fmt.Errorf("family %d not found", s.familyID)
	}
	familyName := familyMembershipFamilyName(s.world, family)
	bossName := familyMembershipFamilyBossName(s.world, family)
	if !familyMembershipBossOnline(ctx, s.world, family.ID, bossName) {
		ctx.WriteString(fmt.Sprintf("패거리의 두목인 %s님은 현재 이용중이 아닙니다.", bossName))
		return enginecmd.StatusDefault, nil
	}

	request := familyJoinRequest{
		PlayerID:    player.ID,
		CreatureID:  creature.ID,
		DisplayName: familyMembershipPlayerName(player),
		FamilyID:    family.ID,
		FamilyName:  familyName,
	}
	if existing, exists := s.store.set(request); exists {
		ctx.WriteString(fmt.Sprintf("당신은 이미 [%s] 패거리 가입신청을 해두고 있습니다.\n", existing.FamilyName))
		return enginecmd.StatusDefault, nil
	}
	if _, err := s.world.UpdateCreatureFamilyState(creature.ID, family.ID, false, true, false); err != nil {
		s.store.delete(player.ID)
		return enginecmd.StatusDefault, err
	}

	ctx.WriteString("\n가입 신청을 하였습니다. \n")
	ctx.WriteString("패거리 두목의 허가를 기다리십시요.")
	return enginecmd.StatusDefault, familyMembershipNotifyBoss(ctx, s.world, request)
}

func NewFamilyJoinApproveHandler(world FamilyMembershipWorld, root string, stores ...*FamilyMembershipRequests) enginecmd.Handler {
	store := familyMembershipRequestStore(stores...)
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		_, bossCreature, ok, err := familyMembershipActor(world, ctx)
		if err != nil || !ok {
			return enginecmd.StatusDefault, err
		}
		bossFamily, ok, err := familyMembershipBossAuthorityWithMessage(world, ctx, "패거리의 문주만이 가능합니다.")
		if err != nil || !ok {
			return enginecmd.StatusDefault, err
		}
		target := strings.TrimSpace(strings.Join(resolved.Args, " "))
		if target == "" {
			ctx.WriteString("누구의 가입을 허가 하시겠습니까?")
			return enginecmd.StatusDefault, nil
		}
		targetPlayer, targetCreature, _, targetVisible := familyMembershipVisibleActiveTarget(ctx, world, bossCreature, target)
		if !targetVisible {
			ctx.WriteString("현재 이용중이 아닙니다.")
			return enginecmd.StatusDefault, nil
		}
		request, found := store.find(world, bossFamily, target)
		if !found {
			if pendingFamily, pending := familyMembershipPendingFamily(targetCreature); pending && pendingFamily == bossFamily {
				if _, member := familyMembershipMemberFamily(targetCreature); !member {
					request = familyJoinRequest{
						PlayerID:    targetPlayer.ID,
						CreatureID:  targetCreature.ID,
						DisplayName: familyMembershipPlayerName(targetPlayer),
						FamilyID:    bossFamily,
						FamilyName:  familyDisplayNameFrom(world, bossFamily),
					}
					found = true
				}
			}
		}
		if !found {
			ctx.WriteString("당신의 패거리에 가입신청을 한 사람이 아닙니다.")
			return enginecmd.StatusDefault, nil
		}
		applicant, creature, ok := familyMembershipRequestCreature(world, request)
		if !ok {
			store.delete(request.PlayerID)
			ctx.WriteString("현재 이용중이 아닙니다.")
			return enginecmd.StatusDefault, nil
		}
		if familyID, member := familyMembershipMemberFamily(creature); member {
			store.delete(request.PlayerID)
			ctx.WriteString(fmt.Sprintf("%s님은 이미 [%s] 패거리에 가입되어 있습니다.\n", familyMembershipPlayerName(applicant), familyDisplayNameFrom(world, familyID)))
			return enginecmd.StatusDefault, nil
		}

		family, foundFamily := familyMemberFamily(world.Families(), bossFamily)
		if !foundFamily {
			return enginecmd.StatusDefault, fmt.Errorf("family %d not found", bossFamily)
		}
		subsidyUnits := family.JoinSubsidy
		subsidyGold := subsidyUnits * familyBankGoldUnit
		balanceUnits := 0
		balanceKnown := false

		if subsidyUnits > 0 {
			rootObj, hasBank := membershipFamilyBankRootObjectFor(world, bossCreature, bossFamily, 0)
			bankFilePath := ""
			candidates := familyBankFileCandidates(root, family.DisplayName, 0)
			for _, path := range candidates {
				if _, err := os.Stat(path); err == nil {
					bankFilePath = path
					break
				}
			}

			if hasBank {
				valStr := rootObj.Properties["value"]
				balance, _ := strconv.Atoi(valStr)
				if balance < subsidyUnits {
					ctx.WriteString("당신의 패거리 자금사정으로는 패거리원을 받을수 없습니다.")
					return enginecmd.StatusDefault, nil
				}
				_, updatedBalance, okWithdraw, err := world.WithdrawObjectValueToCreatureGoldScaled(rootObj.ID, creature.ID, subsidyUnits, subsidyGold)
				if err != nil || !okWithdraw {
					ctx.WriteString("당신의 패거리 자금사정으로는 패거리원을 받을수 없습니다.")
					return enginecmd.StatusDefault, nil
				}
				balanceUnits = updatedBalance
				balanceKnown = true
				if bankFilePath != "" {
					_ = updateFamilyBankOnDisk(bankFilePath, updatedBalance)
				}
			} else {
				balance, foundDisk, path := loadFamilyBankFromDisk(root, family.DisplayName, 0)
				if !foundDisk || balance < subsidyUnits {
					ctx.WriteString("당신의 패거리 자금사정으로는 패거리원을 받을수 없습니다.")
					return enginecmd.StatusDefault, nil
				}
				balanceUnits = balance - subsidyUnits
				balanceKnown = true
				if err := updateFamilyBankOnDisk(path, balanceUnits); err != nil {
					return enginecmd.StatusDefault, err
				}
				if _, err := world.UpdateCreatureGold(creature.ID, subsidyGold); err != nil {
					return enginecmd.StatusDefault, err
				}
			}
		} else if balance, found := familyMembershipCurrentBankBalance(world, root, family.DisplayName, bossCreature, bossFamily); found {
			balanceUnits = balance
			balanceKnown = true
		}

		if _, err := world.UpdateCreatureFamilyState(creature.ID, bossFamily, true, false, false); err != nil {
			return enginecmd.StatusDefault, err
		}

		applicantName := familyMembershipPlayerName(applicant)
		applicantClass := creatureClass(creature)
		updatedMembers, err := PersistFamilyMemberJoin(root, bossFamily, family.DisplayName, applicantName, applicantClass)
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		if err := syncPersistedFamilyMembersInMemory(world, bossFamily, updatedMembers); err != nil {
			return enginecmd.StatusDefault, err
		}

		store.delete(request.PlayerID)
		familyMembershipQueuePlayerSave(world, applicant, creature)
		ctx.WriteString(fmt.Sprintf("%s님의 패거리 가입을 허가하였습니다.\n", applicantName))
		ctx.WriteString(fmt.Sprintf("%s님에게 가입축하금을 지급하였습니다.", applicantName))
		_ = broadcastMessage(ctx, fmt.Sprintf("\n### %s님이 %s에 가입을 하였습니다.", applicantName, familyMembershipFamilyName(world, family)))
		notifyErr := familyMembershipNotifyPlayer(ctx, request.PlayerID, fmt.Sprintf("\n당신은 당신의 패거리로부터 가입축하금을 받았습니다.\n당신은 이제 %d냥을 갖고 있습니다.", familyMembershipCreatureGold(world, creature.ID, creature.Stats["gold"])))
		if balanceKnown {
			ctx.WriteString(fmt.Sprintf("패거리 금고의 총액이 %d만냥이 되었습니다.", balanceUnits))
		}
		return enginecmd.StatusDefault, notifyErr
	}
}

func NewFamilyJoinCancelHandler(world FamilyMembershipWorld, stores ...*FamilyMembershipRequests) enginecmd.Handler {
	store := familyMembershipRequestStore(stores...)
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		_, bossCreature, actorOK, err := familyMembershipActor(world, ctx)
		if err != nil || !actorOK {
			return enginecmd.StatusDefault, err
		}
		bossFamily, ok, err := familyMembershipBossAuthorityWithMessage(world, ctx, "패거리의 문주만이 가능합니다.")
		if err != nil || !ok {
			return enginecmd.StatusDefault, err
		}
		target := strings.TrimSpace(strings.Join(resolved.Args, " "))
		if target == "" {
			ctx.WriteString("누구의 가입의 취소시키려구요?")
			return enginecmd.StatusDefault, nil
		}
		targetPlayer, targetCreature, _, targetVisible := familyMembershipVisibleActiveTarget(ctx, world, bossCreature, target)
		if !targetVisible {
			ctx.WriteString("현재 이용중이 아닙니다.")
			return enginecmd.StatusDefault, nil
		}
		request, found := store.find(world, bossFamily, target)
		if !found {
			if pendingFamily, pending := familyMembershipPendingFamily(targetCreature); pending && pendingFamily == bossFamily {
				if _, member := familyMembershipMemberFamily(targetCreature); !member {
					request = familyJoinRequest{
						PlayerID:    targetPlayer.ID,
						CreatureID:  targetCreature.ID,
						DisplayName: familyMembershipPlayerName(targetPlayer),
						FamilyID:    bossFamily,
						FamilyName:  familyDisplayNameFrom(world, bossFamily),
					}
					found = true
				}
			}
		}
		if !found {
			ctx.WriteString("당신의 패거리에 가입신청을 한 사람이 아닙니다.")
			return enginecmd.StatusDefault, nil
		}
		applicant, creature, ok := familyMembershipRequestCreature(world, request)
		if ok {
			if _, err := world.UpdateCreatureFamilyState(creature.ID, 0, false, false, false); err != nil {
				return enginecmd.StatusDefault, err
			}
		}
		store.delete(request.PlayerID)
		name := strings.TrimSpace(request.DisplayName)
		if ok {
			name = familyMembershipPlayerName(applicant)
		}
		ctx.WriteString(fmt.Sprintf("%s님의 패거리 가입을 취소하였습니다.\n", name))
		return enginecmd.StatusDefault, familyMembershipNotifyPlayer(ctx, request.PlayerID, "당신의 패거리 가입이 취소되었습니다.\n")
	}
}

func NewFamilyLeaveHandler(world FamilyMembershipWorld, root string, stores ...*FamilyMembershipRequests) enginecmd.Handler {
	store := familyMembershipRequestStore(stores...)
	return func(ctx *enginecmd.Context, _ enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		player, creature, ok, err := familyMembershipActor(world, ctx)
		if err != nil || !ok {
			return enginecmd.StatusDefault, err
		}
		if _, pending := store.get(player.ID); pending {
			if _, err := world.UpdateCreatureFamilyState(creature.ID, 0, false, false, false); err != nil {
				return enginecmd.StatusDefault, err
			}
			store.delete(player.ID)
			ctx.WriteString("패거리 가입신청을 취소합니다.")
			return enginecmd.StatusDefault, nil
		}
		if _, pending := familyMembershipPendingFamily(creature); pending {
			if _, err := world.UpdateCreatureFamilyState(creature.ID, 0, false, false, false); err != nil {
				return enginecmd.StatusDefault, err
			}
			ctx.WriteString("패거리 가입신청을 취소합니다.")
			return enginecmd.StatusDefault, nil
		}
		if familyMembershipCreatureIsBoss(creature) {
			ctx.WriteString("패거리의 두목은 탈퇴를 할수 없습니다.")
			return enginecmd.StatusDefault, nil
		}
		familyID, member := familyMembershipMemberFamily(creature)
		if !member {
			ctx.WriteString("당신은 어떤 패거리에도 가입이 되어 있지 않습니다.")
			return enginecmd.StatusDefault, nil
		}

		family, foundFamily := familyMemberFamily(world.Families(), familyID)
		if !foundFamily {
			return enginecmd.StatusDefault, fmt.Errorf("family %d not found", familyID)
		}
		state := &familyLeaveState{
			world:      world,
			root:       root,
			store:      store,
			playerID:   player.ID,
			creatureID: creature.ID,
			familyID:   familyID,
			familyName: family.DisplayName,
		}
		ctx.WriteString("당신은 지금 현재의 패거리를 탈퇴하실 생각입니까? (예/아니오) ")
		if !enginecmd.SetPendingLineHandler(ctx, state.confirm) {
			return enginecmd.StatusDefault, fmt.Errorf("패거리 탈퇴 확인 상태를 시작할 수 없습니다")
		}
		return enginecmd.StatusDoPrompt, nil
	}
}

type familyLeaveState struct {
	world      FamilyMembershipWorld
	root       string
	store      *FamilyMembershipRequests
	playerID   model.PlayerID
	creatureID model.CreatureID
	familyID   int
	familyName string
}

func (s *familyLeaveState) confirm(ctx *enginecmd.Context, line string) (enginecmd.Status, error) {
	enginecmd.ClearPendingLineHandler(ctx)
	player, ok := s.world.Player(s.playerID)
	if !ok {
		return enginecmd.StatusDefault, fmt.Errorf("game: family leave player %q not found", s.playerID)
	}
	creature, ok := s.world.Creature(s.creatureID)
	if !ok {
		return enginecmd.StatusDefault, fmt.Errorf("game: family leave creature %q not found", s.creatureID)
	}
	if familyMembershipCreatureIsBoss(creature) {
		ctx.WriteString("패거리의 두목은 탈퇴를 할수 없습니다.")
		return enginecmd.StatusDefault, nil
	}
	if _, pending := s.store.get(player.ID); pending {
		if _, err := s.world.UpdateCreatureFamilyState(creature.ID, 0, false, false, false); err != nil {
			return enginecmd.StatusDefault, err
		}
		s.store.delete(player.ID)
		ctx.WriteString("패거리 가입신청을 취소합니다.")
		return enginecmd.StatusDefault, nil
	}
	if _, pending := familyMembershipPendingFamily(creature); pending {
		if _, err := s.world.UpdateCreatureFamilyState(creature.ID, 0, false, false, false); err != nil {
			return enginecmd.StatusDefault, err
		}
		ctx.WriteString("패거리 가입신청을 취소합니다.")
		return enginecmd.StatusDefault, nil
	}
	familyID, member := familyMembershipMemberFamily(creature)
	if !member {
		ctx.WriteString("당신은 어떤 패거리에도 가입이 되어 있지 않습니다.")
		return enginecmd.StatusDefault, nil
	}
	family, foundFamily := familyMemberFamily(s.world.Families(), familyID)
	if !foundFamily {
		return enginecmd.StatusDefault, fmt.Errorf("family %d not found", familyID)
	}
	if strings.TrimSpace(line) != "예" {
		ctx.WriteString("패거리를 탈퇴하지 않았습니다.")
		return enginecmd.StatusDefault, nil
	}

	fee := family.JoinSubsidy * 20000
	gold := creature.Stats["gold"]
	if gold < fee {
		ctx.WriteString("당신이 가진 돈으로는 패거리탈퇴비를 낼수 없습니다.\n")
		ctx.WriteString("패거리를 탈퇴하지 않았습니다.")
		return enginecmd.StatusDefault, nil
	}
	updated, err := s.world.UpdateCreatureGold(creature.ID, -fee)
	if err != nil {
		return enginecmd.StatusDefault, err
	}
	if _, err := s.world.UpdateCreatureFamilyState(creature.ID, 0, false, false, false); err != nil {
		return enginecmd.StatusDefault, err
	}

	applicantName := familyMembershipPlayerName(player)
	members, err := PersistFamilyMemberLeave(s.root, familyID, family.DisplayName, applicantName)
	if err != nil {
		return enginecmd.StatusDefault, err
	}
	if err := syncPersistedFamilyMemberLeaveInMemory(s.world, familyID, applicantName, members); err != nil {
		return enginecmd.StatusDefault, err
	}

	s.store.delete(player.ID)
	ctx.WriteString("당신은 패거리에서 탈퇴를 하였습니다.\n")
	_ = broadcastMessage(ctx, fmt.Sprintf("\n### %s님이 %s에서 탈퇴를 하였습니다.", applicantName, familyMembershipFamilyName(s.world, family)))
	ctx.WriteString(fmt.Sprintf("\n당신은 이제 %d냥을 갖고 있습니다.", updated.Stats["gold"]))
	return enginecmd.StatusDefault, nil
}

func NewFamilyKickHandler(world FamilyMembershipWorld, root string, _ ...*FamilyMembershipRequests) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		bossFamily, ok, err := familyMembershipBossAuthorityWithMessage(world, ctx, "패거리의 두목만이 사용가능한 명령입니다.\n")
		if err != nil || !ok {
			return enginecmd.StatusDefault, err
		}
		bossFamilyName := familyDisplayNameFrom(world, bossFamily)
		target := strings.TrimSpace(strings.Join(resolved.Args, " "))
		if target == "" {
			ctx.WriteString("누구를 패거리에서 쫓아내시려고요?\n")
			return enginecmd.StatusDefault, nil
		}
		player, creature, name, found := familyMembershipResolveOnlineOrIDPlayer(ctx, world, target)
		if !found {
			ctx.WriteString("그런 사용자는 없습니다.\n")
			return enginecmd.StatusDefault, nil
		}
		if player.ID == model.PlayerID(ctx.ActorID) {
			ctx.WriteString("자기 자신을 추방하려고요?")
			return enginecmd.StatusDefault, nil
		}
		memberFamily, member := familyMembershipMemberFamily(creature)
		if !member || memberFamily != bossFamily {
			ctx.WriteString("그 사람은 당신의 패거리원이 아닙니다.\n")
			return enginecmd.StatusDefault, nil
		}
		if familyMembershipCreatureIsBoss(creature) {
			ctx.WriteString("패거리의 문주는 추방할 수 없습니다.\n")
			return enginecmd.StatusDefault, nil
		}

		isOnline := false
		if active, ok := activeSessionsFunc(ctx); ok {
			for _, activeSession := range active() {
				if activeSession.ActorID == string(player.ID) {
					isOnline = true
					break
				}
			}
		}

		if _, err := world.UpdateCreatureFamilyState(creature.ID, 0, false, false, false); err != nil {
			return enginecmd.StatusDefault, err
		}
		familyMembershipMarkPlayerDirty(world, player, creature)

		if !isOnline {
			filePath, foundPath := getOfflinePlayerFilePath(root, name, player)
			if foundPath {
				if err := updateOfflinePlayerFamilyState(filePath); err != nil {
					return enginecmd.StatusDefault, err
				}
			}
		}

		members, err := PersistFamilyMemberLeave(root, bossFamily, bossFamilyName, name)
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		if err := syncPersistedFamilyMemberLeaveInMemory(world, bossFamily, name, members); err != nil {
			return enginecmd.StatusDefault, err
		}

		ctx.WriteString(fmt.Sprintf("%s님을 패거리에서 추방하였습니다.\n", name))
		return enginecmd.StatusDefault, familyMembershipNotifyPlayer(ctx, player.ID, "당신은 패거리에서 추방되었습니다.\n")
	}
}

func familyMembershipRequestStore(stores ...*FamilyMembershipRequests) *FamilyMembershipRequests {
	if len(stores) > 0 && stores[0] != nil {
		return stores[0]
	}
	return defaultFamilyMembershipRequests
}

func (s *FamilyMembershipRequests) get(playerID model.PlayerID) (familyJoinRequest, bool) {
	if s == nil || playerID.IsZero() {
		return familyJoinRequest{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	request, ok := s.byPlayer[playerID]
	return request, ok
}

func (s *FamilyMembershipRequests) set(request familyJoinRequest) (familyJoinRequest, bool) {
	if s == nil || request.PlayerID.IsZero() {
		return familyJoinRequest{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byPlayer == nil {
		s.byPlayer = map[model.PlayerID]familyJoinRequest{}
	}
	if existing, ok := s.byPlayer[request.PlayerID]; ok {
		return existing, true
	}
	s.byPlayer[request.PlayerID] = request
	return familyJoinRequest{}, false
}

func (s *FamilyMembershipRequests) delete(playerID model.PlayerID) {
	if s == nil || playerID.IsZero() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byPlayer, playerID)
}

func (s *FamilyMembershipRequests) find(world PlayerLookup, familyID int, target string) (familyJoinRequest, bool) {
	if s == nil || familyID <= 0 {
		return familyJoinRequest{}, false
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return familyJoinRequest{}, false
	}

	s.mu.Lock()
	requests := make([]familyJoinRequest, 0, len(s.byPlayer))
	for _, request := range s.byPlayer {
		if request.FamilyID == familyID {
			requests = append(requests, request)
		}
	}
	s.mu.Unlock()

	var prefix familyJoinRequest
	prefixCount := 0
	for _, request := range requests {
		candidates := []string{string(request.PlayerID), request.DisplayName}
		if player, ok := world.Player(request.PlayerID); ok {
			candidates = append(candidates, familyMembershipPlayerName(player))
		}
		exact, partial := familyMembershipNamesMatch(candidates, target)
		if exact {
			return request, true
		}
		if partial {
			prefix = request
			prefixCount++
		}
	}
	if prefixCount == 1 {
		return prefix, true
	}
	return familyJoinRequest{}, false
}

func familyMembershipActor(world FamilyMembershipWorld, ctx *enginecmd.Context) (model.Player, model.Creature, bool, error) {
	if ctx == nil || ctx.ActorID == "" {
		return model.Player{}, model.Creature{}, false, ErrSocialActorRequired
	}
	if world == nil {
		return model.Player{}, model.Creature{}, false, fmt.Errorf("game: family membership world is nil")
	}
	player, ok := world.Player(model.PlayerID(ctx.ActorID))
	if !ok || player.CreatureID.IsZero() {
		return model.Player{}, model.Creature{}, false, fmt.Errorf("game: family membership actor %q not found", ctx.ActorID)
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return model.Player{}, model.Creature{}, false, fmt.Errorf("game: family membership creature %q not found", player.CreatureID)
	}
	return player, creature, true, nil
}

func familyMembershipBossAuthority(world FamilyMembershipWorld, ctx *enginecmd.Context) (int, bool, error) {
	return familyMembershipBossAuthorityWithMessage(world, ctx, "패거리의 문주만이 가능합니다.\n")
}

func familyMembershipBossAuthorityWithMessage(world FamilyMembershipWorld, ctx *enginecmd.Context, message string) (int, bool, error) {
	_, creature, ok, err := familyMembershipActor(world, ctx)
	if err != nil || !ok {
		return 0, false, err
	}
	familyID, member := familyMembershipMemberFamily(creature)
	if !member || !familyMembershipCreatureIsBoss(creature) {
		ctx.WriteString(message)
		return 0, false, nil
	}
	return familyID, true, nil
}

func familyMembershipActorByID(world FamilyMembershipWorld, playerID model.PlayerID, creatureID model.CreatureID) (model.Player, model.Creature, bool, error) {
	player, ok := world.Player(playerID)
	if !ok {
		return model.Player{}, model.Creature{}, false, fmt.Errorf("game: family player %q not found", playerID)
	}
	if creatureID.IsZero() {
		creatureID = player.CreatureID
	}
	if creatureID.IsZero() {
		return player, model.Creature{}, false, fmt.Errorf("game: family player %q has no creature", playerID)
	}
	creature, ok := world.Creature(creatureID)
	if !ok {
		return player, model.Creature{}, false, fmt.Errorf("game: family creature %q not found", creatureID)
	}
	return player, creature, true, nil
}

func familyMembershipRejectExistingJoinState(ctx *enginecmd.Context, world FamilyMembershipWorld, store *FamilyMembershipRequests, player model.Player, creature model.Creature) bool {
	if _, member := familyMembershipMemberFamily(creature); member {
		ctx.WriteString("당신은 이미 패거리에 가입되어 있습니다.\n")
		return true
	}
	if request, pending := store.get(player.ID); pending {
		ctx.WriteString(fmt.Sprintf("당신은 이미 [%s] 패거리 가입신청을 해두고 있습니다.\n", request.FamilyName))
		return true
	}
	if familyID, pending := familyMembershipPendingFamily(creature); pending {
		ctx.WriteString(fmt.Sprintf("당신은 이미 [%s] 패거리 가입신청을 해두고 있습니다.\n", familyDisplayNameFrom(world, familyID)))
		return true
	}
	return false
}

func familyMembershipMemberFamily(creature model.Creature) (int, bool) {
	if !creatureHasNormalizedFlag(creature, "familyFlag", "PFAMIL") {
		return 0, false
	}
	familyID, ok := creatureNormalizedInt(creature, "familyID", "dailyExpndMax", "legacyDailyExpndMax")
	return familyID, ok && familyID > 0
}

func familyMembershipPendingFamily(creature model.Creature) (int, bool) {
	if !creatureHasNormalizedFlag(creature, "PRDFML", "familyRequest", "familyPending") {
		return 0, false
	}
	familyID, ok := creatureNormalizedInt(creature, "familyID", "dailyExpndMax", "legacyDailyExpndMax")
	return familyID, ok && familyID > 0
}

func familyMembershipCreatureIsBoss(creature model.Creature) bool {
	return creatureHasNormalizedFlag(creature, "PFMBOS", "familyBoss", "familyBossFlag")
}

func familyMembershipResolveFamily(world FamilyMembershipWorld, target string) (model.Family, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return model.Family{}, false
	}
	families := familyMembershipActiveFamilies(world)
	if id, ok := familyMembershipParseFamilyID(target); ok {
		for _, family := range families {
			if family.ID == id || family.Slot == id {
				return family, true
			}
		}
	}
	for _, family := range families {
		if target == strings.TrimSpace(family.DisplayName) {
			return family, true
		}
	}
	var prefix model.Family
	prefixCount := 0
	for _, family := range families {
		if strings.HasPrefix(strings.TrimSpace(family.DisplayName), target) {
			prefix = family
			prefixCount++
		}
	}
	if prefixCount == 1 {
		return prefix, true
	}
	return model.Family{}, false
}

func familyMembershipParseFamilyID(target string) (int, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return 0, false
	}
	if id, err := strconv.Atoi(target); err == nil && id > 0 {
		return id, true
	}
	lower := strings.ToLower(target)
	for _, prefix := range []string{"패거리", "family"} {
		if strings.HasPrefix(lower, prefix) {
			id, err := strconv.Atoi(strings.TrimSpace(target[len(prefix):]))
			return id, err == nil && id > 0
		}
	}
	return 0, false
}

func familyMembershipActiveFamilies(world FamilyMembershipWorld) []model.Family {
	var active []model.Family
	for _, family := range world.Families() {
		if family.Slot <= 0 || strings.TrimSpace(family.DisplayName) == "" {
			continue
		}
		active = append(active, family)
	}
	return active
}

func familyMembershipWriteJoinTargets(ctx *enginecmd.Context, world FamilyMembershipWorld) bool {
	families := familyMembershipActiveFamilies(world)
	if len(families) == 0 {
		ctx.WriteString("가입할 수 있는 패거리가 없습니다.\n")
		return false
	}
	ctx.WriteString("다음과 같은 패거리가 있습니다.\n")
	ctx.WriteString("\n패거리이름             문주이름               가입축하금             \n\n")
	for _, family := range families {
		ctx.WriteString(fmt.Sprintf("%-20s  %-20s  %d만냥\n",
			familyMembershipFamilyName(world, family),
			familyMembershipFamilyBossName(world, family),
			family.JoinSubsidy,
		))
	}
	ctx.WriteString("\n당신은 어떤 패거리에 가입을 원하십니까? ")
	ctx.WriteString("\n패거리의 이름을 입력해 주십시요.  ")
	return true
}

func familyMembershipFamilyName(world any, family model.Family) string {
	if name := strings.TrimSpace(family.DisplayName); name != "" {
		return name
	}
	return familyDisplayNameFrom(world, family.ID)
}

func familyMembershipFamilyBossName(world any, family model.Family) string {
	if name := strings.TrimSpace(family.BossName); name != "" {
		return name
	}
	if name, ok := familyBossNameFrom(world, family.ID); ok {
		return name
	}
	return familyMembershipFamilyName(world, family)
}

func familyMembershipFamilyClosed(family model.Family) bool {
	bossName := strings.TrimSpace(family.BossName)
	return bossName == "*단체*" || bossName == "단체"
}

func familyMembershipPlayerName(player model.Player) string {
	if name := strings.TrimSpace(player.DisplayName); name != "" {
		return name
	}
	return string(player.ID)
}

func familyMembershipRequestCreature(world FamilyMembershipWorld, request familyJoinRequest) (model.Player, model.Creature, bool) {
	player, ok := world.Player(request.PlayerID)
	if !ok || player.CreatureID.IsZero() {
		return model.Player{}, model.Creature{}, false
	}
	creature, ok := world.Creature(player.CreatureID)
	return player, creature, ok
}

func familyMembershipCreatureGold(world FamilyMembershipWorld, creatureID model.CreatureID, fallback int) int {
	if creatureID.IsZero() || world == nil {
		return fallback
	}
	creature, ok := world.Creature(creatureID)
	if !ok {
		return fallback
	}
	return creature.Stats["gold"]
}

func familyMembershipCurrentBankBalance(world FamilyMembershipWorld, root string, familyName string, bossCreature model.Creature, familyID int) (int, bool) {
	if rootObj, ok := membershipFamilyBankRootObjectFor(world, bossCreature, familyID, 0); ok {
		balance, _ := strconv.Atoi(rootObj.Properties["value"])
		return balance, true
	}
	if strings.TrimSpace(root) == "" {
		return 0, false
	}
	balance, found, _ := loadFamilyBankFromDisk(root, familyName, 0)
	return balance, found
}

func familyMembershipResolveOnlineOrIDPlayer(ctx *enginecmd.Context, world FamilyMembershipWorld, target string) (model.Player, model.Creature, string, bool) {
	if active, ok := activeSessionsFunc(ctx); ok {
		if session, name, found := findActivePlayerSession(world, active(), target); found {
			player, ok := world.Player(model.PlayerID(session.ActorID))
			if ok && !player.CreatureID.IsZero() {
				creature, creatureOK := world.Creature(player.CreatureID)
				if creatureOK {
					return player, creature, name, true
				}
			}
		}
	}
	player, ok := world.Player(model.PlayerID(strings.TrimSpace(target)))
	if !ok || player.CreatureID.IsZero() {
		if resolved, found := familyMembershipResolveStoredPlayer(world, target); found {
			player = resolved
			ok = true
		}
	}
	if !ok || player.CreatureID.IsZero() {
		return model.Player{}, model.Creature{}, "", false
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return model.Player{}, model.Creature{}, "", false
	}
	return player, creature, familyMembershipPlayerName(player), true
}

func familyMembershipTargetIsActive(ctx *enginecmd.Context, world FamilyMembershipWorld, target string) bool {
	active, ok := activeSessionsFunc(ctx)
	if !ok {
		return true
	}
	_, _, found := findActivePlayerSession(world, active(), target)
	return found
}

func familyMembershipBossOnline(ctx *enginecmd.Context, world FamilyMembershipWorld, familyID int, bossName string) bool {
	active, ok := activeSessionsFunc(ctx)
	if !ok {
		return false
	}
	if _, _, found := findOnlineFamilyBoss(world, active(), familyID); found {
		return true
	}
	_, _, found := findActivePlayerSession(world, active(), bossName)
	return found
}

func familyMembershipTargetVisibleToBoss(ctx *enginecmd.Context, world FamilyMembershipWorld, boss model.Creature, target string) bool {
	_, _, _, ok := familyMembershipVisibleActiveTarget(ctx, world, boss, target)
	return ok
}

func familyMembershipVisibleActiveTarget(ctx *enginecmd.Context, world FamilyMembershipWorld, boss model.Creature, target string) (model.Player, model.Creature, string, bool) {
	active, ok := activeSessionsFunc(ctx)
	if !ok {
		return model.Player{}, model.Creature{}, "", true
	}
	activeSession, name, found := findActivePlayerSession(world, active(), target)
	if !found {
		return model.Player{}, model.Creature{}, "", false
	}
	player, ok := world.Player(model.PlayerID(activeSession.ActorID))
	if !ok || player.CreatureID.IsZero() {
		return model.Player{}, model.Creature{}, "", false
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return model.Player{}, model.Creature{}, "", false
	}
	if creatureHasNormalizedFlag(creature, "PDMINV", "dmInvisible", "dmInvis") {
		return model.Player{}, model.Creature{}, "", false
	}
	if creatureHasNormalizedFlag(boss, "PBLIND", "blind", "blinded") {
		return model.Player{}, model.Creature{}, "", false
	}
	if creatureHasNormalizedFlag(creature, "PINVIS", "invisible", "invisibility") &&
		!creatureHasNormalizedFlag(boss, "PDINVI", "detectInvisible", "detectInvis") {
		return model.Player{}, model.Creature{}, "", false
	}
	return player, creature, name, true
}

func familyMembershipResolveStoredPlayer(world FamilyMembershipWorld, target string) (model.Player, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return model.Player{}, false
	}
	playerWorld, ok := world.(familyMembershipPlayersWorld)
	if !ok {
		return model.Player{}, false
	}

	var exact []model.Player
	var prefix []model.Player
	for _, player := range playerWorld.Players() {
		names := familyMembershipPlayerLookupNames(player)
		exactMatch, prefixMatch := familyMembershipNamesMatch(names, target)
		if exactMatch {
			exact = append(exact, player)
			continue
		}
		if prefixMatch {
			prefix = append(prefix, player)
		}
	}
	if len(exact) == 1 {
		return exact[0], true
	}
	if len(exact) > 1 {
		return model.Player{}, false
	}
	if len(prefix) == 1 {
		return prefix[0], true
	}
	return model.Player{}, false
}

func familyMembershipPlayerLookupNames(player model.Player) []string {
	names := []string{string(player.ID), strings.TrimSpace(player.DisplayName)}
	if path := strings.TrimSpace(player.Metadata.LegacyPath); path != "" {
		names = append(names, filepath.Base(filepath.FromSlash(path)))
	}
	if rawName := player.Metadata.RawFields["filename"]; len(rawName) != 0 {
		names = append(names, string(rawName))
		if decoded, err := legacykr.ValidUTF8OrDecodeContext(legacykr.Context{Path: "player", Field: "filename"}, rawName); err == nil {
			names = append(names, decoded)
		}
	}
	return names
}

func familyMembershipMarkPlayerDirty(world FamilyMembershipWorld, player model.Player, creature model.Creature) {
	dirtyWorld, ok := world.(familyMembershipDirtyWorld)
	if !ok {
		return
	}
	playerID := player.ID
	if playerID.IsZero() {
		playerID = creature.PlayerID
	}
	dirtyWorld.MarkPlayerDirty(playerID)
}

func familyMembershipQueuePlayerSave(world FamilyMembershipWorld, player model.Player, creature model.Creature) {
	playerID := player.ID
	if playerID.IsZero() {
		playerID = creature.PlayerID
	}
	if playerID.IsZero() {
		return
	}
	if saveWorld, ok := world.(familyMembershipSaveWorld); ok {
		saveWorld.MarkPlayerDirty(playerID)
		saveWorld.QueueSave(playerID, "")
		return
	}
	familyMembershipMarkPlayerDirty(world, player, creature)
	if saveWorld, ok := world.(interface{ SavePlayer(model.PlayerID) error }); ok {
		_ = saveWorld.SavePlayer(playerID)
	}
}

func familyMembershipNamesMatch(candidates []string, target string) (exact bool, prefix bool) {
	target = strings.TrimSpace(target)
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if candidate == target || strings.EqualFold(candidate, target) {
			exact = true
		}
		if strings.HasPrefix(candidate, target) {
			prefix = true
		}
	}
	return exact, prefix
}

func familyMembershipNotifyBoss(ctx *enginecmd.Context, world FamilyMembershipWorld, request familyJoinRequest) error {
	active, activeOK := activeSessionsFunc(ctx)
	send, sendOK := sendToSessionFunc(ctx)
	if !activeOK || !sendOK {
		return nil
	}
	boss, _, found := findOnlineFamilyBoss(world, active(), request.FamilyID)
	if !found {
		return nil
	}
	message := fmt.Sprintf("\n>>> %s님이 당신의 패거리에 가입하기를 원합니다.", request.DisplayName)
	return familyMembershipWriteToSession(ctx, send, boss.ID, message)
}

func familyMembershipNotifyPlayer(ctx *enginecmd.Context, playerID model.PlayerID, message string) error {
	active, activeOK := activeSessionsFunc(ctx)
	send, sendOK := sendToSessionFunc(ctx)
	if !activeOK || !sendOK || playerID.IsZero() || message == "" {
		return nil
	}
	for _, activeSession := range active() {
		if activeSession.ActorID == string(playerID) {
			return familyMembershipWriteToSession(ctx, send, activeSession.ID, message)
		}
	}
	return nil
}

func familyMembershipWriteToSession(ctx *enginecmd.Context, send func(session.ID, session.Command) error, id session.ID, message string) error {
	if string(id) == ctx.SessionID {
		ctx.WriteString(message)
		return nil
	}
	return send(id, session.Command{Write: message})
}

type legacyMember struct {
	classID int
	name    string
}

type familyMemberEditMode int

const (
	familyMemberEditJoin familyMemberEditMode = iota + 1
	familyMemberEditLeave
	familyMemberEditClassChange
)

// PersistFamilyMemberJoin mirrors C edit_member(..., 1): append a member to
// player/family/family_member_N and return the post-write member list.
func PersistFamilyMemberJoin(root string, familyID int, familyName string, memberName string, classID int) ([]model.FamilyMember, error) {
	members, err := editFamilyMembersFile(root, familyID, familyName, memberName, classID, familyMemberEditJoin)
	return familyMembersForModel(familyID, members), err
}

// PersistFamilyMemberLeave mirrors C edit_member(..., 2): remove a member from
// player/family/family_member_N. The same C path is used by leave, kick, and
// suicide flows.
func PersistFamilyMemberLeave(root string, familyID int, familyName string, memberName string) ([]model.FamilyMember, error) {
	members, err := editFamilyMembersFile(root, familyID, familyName, memberName, 0, familyMemberEditLeave)
	return familyMembersForModel(familyID, members), err
}

// PersistFamilyMemberClassChange mirrors C edit_member(..., 3): update the
// stored class for an existing family member after a successful class change.
func PersistFamilyMemberClassChange(root string, familyID int, familyName string, memberName string, classID int) ([]model.FamilyMember, error) {
	members, err := editFamilyMembersFile(root, familyID, familyName, memberName, classID, familyMemberEditClassChange)
	return familyMembersForModel(familyID, members), err
}

func readFamilyMembersFile(path string) ([]legacyMember, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	members := []legacyMember{}
	var familyName string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		lineBytes := scanner.Bytes()
		decodedLine, err := legacykr.ValidUTF8OrDecodeContext(legacykr.Context{Path: path, Field: "family_member"}, lineBytes)
		if err != nil {
			decodedLine = string(lineBytes)
		}
		fields := strings.Fields(decodedLine)
		if len(fields) == 0 {
			continue
		}
		classID, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		if classID == 0 {
			if len(fields) >= 2 {
				familyName = fields[1]
			}
			break
		}
		if len(fields) >= 2 {
			members = append(members, legacyMember{
				classID: classID,
				name:    fields[1],
			})
		}
	}
	return members, familyName, nil
}

func writeFamilyMembersFile(path string, members []legacyMember, familyName string) error {
	var sb strings.Builder
	for _, m := range members {
		sb.WriteString(fmt.Sprintf("%d %s\n", m.classID, m.name))
	}
	sb.WriteString(fmt.Sprintf("0 %s\n", familyName))

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func addFamilyMemberToFile(root string, familyID int, familyName string, memberName string, classID int) ([]legacyMember, error) {
	return editFamilyMembersFile(root, familyID, familyName, memberName, classID, familyMemberEditJoin)
}

func removeFamilyMemberFromFile(root string, familyID int, familyName string, memberName string) error {
	_, err := editFamilyMembersFile(root, familyID, familyName, memberName, 0, familyMemberEditLeave)
	return err
}

func updateFamilyMemberClassInFile(root string, familyID int, familyName string, memberName string, classID int) ([]legacyMember, error) {
	return editFamilyMembersFile(root, familyID, familyName, memberName, classID, familyMemberEditClassChange)
}

func editFamilyMembersFile(root string, familyID int, familyName string, memberName string, classID int, mode familyMemberEditMode) ([]legacyMember, error) {
	if familyID <= 0 {
		return nil, fmt.Errorf("family member persistence: family id is required")
	}
	memberName = strings.TrimSpace(memberName)
	if memberName == "" {
		return nil, fmt.Errorf("family member persistence: member name is required")
	}
	path := filepath.Join(root, "player", "family", fmt.Sprintf("family_member_%d", familyID))
	members, fname, err := readFamilyMembersFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if mode != familyMemberEditJoin {
				return nil, nil
			}
			members = nil
		} else {
			return nil, err
		}
	}
	if fname != "" {
		familyName = fname
	}

	switch mode {
	case familyMemberEditJoin:
		exists := false
		for _, m := range members {
			if familyMemberNameEqual(m.name, memberName) {
				exists = true
				break
			}
		}
		if !exists {
			members = append(members, legacyMember{classID: classID, name: memberName})
		}
	case familyMemberEditLeave:
		updated := members[:0]
		for _, m := range members {
			if familyMemberNameEqual(m.name, memberName) {
				continue
			}
			updated = append(updated, m)
		}
		members = updated
	case familyMemberEditClassChange:
		for i := range members {
			if familyMemberNameEqual(members[i].name, memberName) {
				members[i].classID = classID
				break
			}
		}
	default:
		return nil, fmt.Errorf("family member persistence: unsupported edit mode %d", mode)
	}

	return members, writeFamilyMembersFile(path, members, familyName)
}

func syncFamilyMembersInMemory(world FamilyMembershipWorld, familyID int, members []legacyMember) error {
	return world.UpdateFamilyMembers(familyID, familyMembersForModel(familyID, members))
}

func syncPersistedFamilyMembersInMemory(world FamilyMembershipWorld, familyID int, members []model.FamilyMember) error {
	if members == nil {
		return nil
	}
	return world.UpdateFamilyMembers(familyID, members)
}

func syncPersistedFamilyMemberLeaveInMemory(world FamilyMembershipWorld, familyID int, memberName string, members []model.FamilyMember) error {
	if members != nil {
		return world.UpdateFamilyMembers(familyID, members)
	}
	family, ok := familyMemberFamily(world.Families(), familyID)
	if !ok {
		return nil
	}
	updated := make([]model.FamilyMember, 0, len(family.Members))
	for _, member := range family.Members {
		if familyMemberNameEqual(member.DisplayName, memberName) {
			continue
		}
		updated = append(updated, member)
	}
	return world.UpdateFamilyMembers(familyID, updated)
}

func familyMembersForModel(familyID int, members []legacyMember) []model.FamilyMember {
	if members == nil {
		return nil
	}
	inMemoryMembers := make([]model.FamilyMember, 0, len(members))
	for i, m := range members {
		inMemoryMembers = append(inMemoryMembers, model.FamilyMember{
			Class:       m.classID,
			DisplayName: m.name,
			Metadata: model.Metadata{
				Source:         "legacy",
				LegacyKind:     "family_member",
				LegacyID:       fmt.Sprintf("%d:%d", familyID, i),
				LegacyEncoding: "euc-kr/cp949",
				RecordIndex:    i,
			},
		})
	}
	return inMemoryMembers
}

func familyMemberNameEqual(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	return a == b || strings.EqualFold(a, b)
}

func getOfflinePlayerFilePath(root string, name string, player model.Player) (string, bool) {
	var candidates []string
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	rawName := player.Metadata.RawFields["filename"]
	rawShard := player.Metadata.RawFields["shard"]
	if len(rawName) != 0 && len(rawShard) != 0 {
		candidates = append(candidates, filepath.Join(root, "player", string(rawShard), string(rawName)))
	}
	if path := strings.TrimSpace(player.Metadata.LegacyPath); path != "" {
		candidates = append(candidates, filepath.Join(root, filepath.FromSlash(path)))
	}

	var names []string
	name = strings.TrimSpace(name)
	if name != "" {
		if encoded, err := legacykr.EncodeEUCKR(name); err == nil {
			names = append(names, string(encoded))
		}
		names = append(names, name)
	}
	if display := strings.TrimSpace(player.DisplayName); display != "" && display != name {
		if encoded, err := legacykr.EncodeEUCKR(display); err == nil {
			names = append(names, string(encoded))
		}
		names = append(names, display)
	}

	for _, candidateName := range names {
		candidates = append(candidates, filepath.Join(root, "player", krtext.FirstHangulBucket(name), candidateName))
	}

	seen := map[string]struct{}{}
	for _, p := range candidates {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, exists := seen[p]; exists {
			continue
		}
		seen[p] = struct{}{}
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

func updateOfflinePlayerFamilyState(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	if len(data) < 1184 {
		return fmt.Errorf("player file %q is too small: size=%d", filePath, len(data))
	}
	data[412+6] &= 0x7f
	data[612] = 0
	return os.WriteFile(filePath, data, 0600)
}

func familyBankFileCandidates(root string, familyName string, special int) []string {
	var names []string
	if familyName != "" {
		names = append(names, familyName)
	}
	var paths []string
	for _, name := range names {
		encoded, err := legacykr.EncodeEUCKR(name)
		var encodedStr string
		if err == nil {
			encodedStr = string(encoded)
		}
		for _, fn := range []string{
			fmt.Sprintf("%s_%d", name, special),
			name,
		} {
			paths = append(paths, filepath.Join(root, "player", "family", "bank", fn))
		}
		if encodedStr != "" {
			for _, fn := range []string{
				fmt.Sprintf("%s_%d", encodedStr, special),
				encodedStr,
			} {
				paths = append(paths, filepath.Join(root, "player", "family", "bank", fn))
			}
		}
	}

	seen := map[string]struct{}{}
	var out []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, exists := seen[p]; exists {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func loadFamilyBankFromDisk(root string, familyName string, special int) (int, bool, string) {
	candidates := familyBankFileCandidates(root, familyName, special)
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil {
			if len(data) >= 304 {
				balance := int(binary.LittleEndian.Uint32(data[300:304]))
				return balance, true, path
			}
		}
	}
	return 0, false, ""
}

func updateFamilyBankOnDisk(path string, newBalance int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) < 304 {
		return fmt.Errorf("invalid bank file size")
	}
	binary.LittleEndian.PutUint32(data[300:304], uint32(newBalance))
	return os.WriteFile(path, data, 0600)
}

func membershipFamilyBankRootObjectFor(world FamilyMembershipWorld, creature model.Creature, familyID int, special int) (model.ObjectInstance, bool) {
	for _, bankID := range familyBankIDCandidates(world, creature, familyID, special) {
		account, ok := world.Bank(bankID)
		if !ok {
			continue
		}
		for _, objectID := range account.Objects.ObjectIDs {
			object, found := world.Object(objectID)
			if found {
				return object, true
			}
		}
	}
	return model.ObjectInstance{}, false
}
