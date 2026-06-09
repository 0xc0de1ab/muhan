package game

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	enginecmd "muhan/internal/engine/command"
	"muhan/internal/krtext"
	"muhan/internal/session"
	"muhan/internal/world/model"
)

const maxLegacyFollowers = 4

type GroupWorld interface {
	ActionWorld
	MovePlayer(model.PlayerID, string) error
	Object(model.ObjectInstanceID) (model.ObjectInstance, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
}

type GroupMemory struct {
	mu              sync.Mutex
	following       map[string]string
	leaderFollowers map[string][]string
}

type GroupMembershipSnapshot struct {
	LeaderID    string
	FollowerIDs []string
}

func NewGroupMemory() *GroupMemory {
	return &GroupMemory{
		following:       map[string]string{},
		leaderFollowers: map[string][]string{},
	}
}

func (m *GroupMemory) Follow(follower, leader string) {
	if m == nil || follower == "" || leader == "" || follower == leader {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.following == nil {
		m.following = map[string]string{}
	}
	if m.leaderFollowers == nil {
		m.leaderFollowers = map[string][]string{}
	}
	if oldLeader := strings.TrimSpace(m.following[follower]); oldLeader != "" {
		m.removeFollowerLocked(oldLeader, follower)
	}
	m.following[follower] = leader
	m.leaderFollowers[leader] = append([]string{follower}, removeString(m.leaderFollowers[leader], follower)...)
}

func (m *GroupMemory) Unfollow(follower string) (string, bool) {
	if m == nil || follower == "" {
		return "", false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	leader := strings.TrimSpace(m.following[follower])
	if leader == "" {
		return "", false
	}
	delete(m.following, follower)
	m.removeFollowerLocked(leader, follower)
	return leader, true
}

func (m *GroupMemory) LeaderOf(actor string) (string, bool) {
	if m == nil || actor == "" {
		return "", false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	leader := strings.TrimSpace(m.following[actor])
	return leader, leader != ""
}

func (m *GroupMemory) FollowersOf(leader string) []string {
	if m == nil || leader == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.leaderFollowers == nil {
		return followersOfLeaderFromFollowing(m.following, leader)
	}
	followers := append([]string(nil), m.leaderFollowers[leader]...)
	return followers
}

func (m *GroupMemory) RemoveFollower(leader, follower string) bool {
	if m == nil || leader == "" || follower == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.following[follower] != leader {
		return false
	}
	delete(m.following, follower)
	m.removeFollowerLocked(leader, follower)
	return true
}

func (m *GroupMemory) removeFollowerLocked(leader, follower string) {
	if m == nil || m.leaderFollowers == nil {
		return
	}
	followers := removeString(m.leaderFollowers[leader], follower)
	if len(followers) == 0 {
		delete(m.leaderFollowers, leader)
		return
	}
	m.leaderFollowers[leader] = followers
}

func removeString(values []string, target string) []string {
	out := values[:0]
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}

func followersOfLeaderFromFollowing(following map[string]string, leader string) []string {
	var followers []string
	for follower, currentLeader := range following {
		if currentLeader == leader {
			followers = append(followers, follower)
		}
	}
	sort.Strings(followers)
	return followers
}

func (m *GroupMemory) Snapshot(actorID string) (GroupMembershipSnapshot, bool) {
	leader, followers, ok := groupForActor(m, actorID)
	if !ok {
		return GroupMembershipSnapshot{}, false
	}
	return GroupMembershipSnapshot{
		LeaderID:    leader,
		FollowerIDs: append([]string(nil), followers...),
	}, true
}

func (s GroupMembershipSnapshot) CreatureSnapshot(world PlayerLookup) (model.CreatureID, []model.CreatureID, bool) {
	leaderID := groupSnapshotCreatureID(world, s.LeaderID)
	if leaderID.IsZero() {
		return "", nil, false
	}

	followers := make([]model.CreatureID, 0, len(s.FollowerIDs))
	seen := map[model.CreatureID]struct{}{leaderID: {}}
	for _, actorID := range s.FollowerIDs {
		creatureID := groupSnapshotCreatureID(world, actorID)
		if creatureID.IsZero() {
			continue
		}
		if _, ok := seen[creatureID]; ok {
			continue
		}
		seen[creatureID] = struct{}{}
		followers = append(followers, creatureID)
	}
	if len(followers) == 0 {
		return leaderID, nil, false
	}
	return leaderID, followers, true
}

func NewFollowHandler(world GroupWorld, groups *GroupMemory) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" || ctx.SessionID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		send, ok := sendToSessionFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		if len(resolved.Args) == 0 || strings.TrimSpace(resolved.Args[0]) == "" {
			ctx.WriteString("누구를 따라 가시고 싶으세요?")
			return enginecmd.StatusDefault, nil
		}

		actorID := ctx.ActorID
		if err := revealGroupActor(world, actorID); err != nil {
			return enginecmd.StatusDefault, err
		}
		actorRoom := playerRoomID(world, model.PlayerID(actorID))
		targetArg := strings.TrimSpace(resolved.Args[0])
		target := ActiveSession{ActorID: actorID, ID: session.ID(ctx.SessionID)}
		targetName := playerDisplayName(world, actorID)
		if targetArg != "나" {
			var found bool
			target, targetName, found = findActivePlayerSessionInRoom(world, active(), targetArg, actorRoom)
			if !found {
				ctx.WriteString("그런 사람은 여기 없습니다.")
				return enginecmd.StatusDefault, nil
			}
		}

		if target.ActorID == actorID {
			if oldLeader, ok := groups.Unfollow(actorID); ok {
				oldName := playerDisplayName(world, oldLeader)
				ctx.WriteString(fmt.Sprintf("당신은 %s님을 그만 따라 다니기로 하였습니다.\n", oldName))
				if !actorHasGroupDMInvisible(world, actorID) {
					if err := notifyActor(ctx, active(), send, oldLeader, fmt.Sprintf("\n%s 이제 당신을 따라다니지 않습니다.", actorTopic(playerDisplayName(world, actorID)))); err != nil {
						return enginecmd.StatusDefault, err
					}
				}
				return enginecmd.StatusDefault, nil
			}
			ctx.WriteString("자기자신을 따라 갈순 없습니다.")
			return enginecmd.StatusDefault, nil
		}
		if leader, ok := groups.LeaderOf(target.ActorID); ok && leader == actorID {
			ctx.WriteString(fmt.Sprintf("이미 %s는 당신을 따라다니고 있습니다.", groupThirdPersonPronoun(world, target.ActorID)))
			return enginecmd.StatusDefault, nil
		}
		if len(groups.FollowersOf(target.ActorID)) >= maxLegacyFollowers {
			ctx.WriteString("이미 그룹원이 5명이 다 찼군요.\n")
			return enginecmd.StatusDefault, nil
		}
		if oldLeader, ok := groups.Unfollow(actorID); ok {
			oldName := playerDisplayName(world, oldLeader)
			ctx.WriteString(fmt.Sprintf("당신은 %s님을 그만 따라 다니기로 하였습니다.\n", oldName))
			if !actorHasGroupDMInvisible(world, actorID) {
				if err := notifyActor(ctx, active(), send, oldLeader, fmt.Sprintf("\n%s 이제 당신을 따라다니지 않습니다.", actorTopic(playerDisplayName(world, actorID)))); err != nil {
					return enginecmd.StatusDefault, err
				}
			}
		}

		groups.Follow(actorID, target.ActorID)
		actorName := playerDisplayName(world, actorID)
		ctx.WriteString(fmt.Sprintf("당신은 이제부터 %s님을 따라다닙니다.", targetName))
		if !actorHasGroupDMInvisible(world, actorID) {
			sessions := active()
			if err := notifyActor(ctx, sessions, send, target.ActorID, fmt.Sprintf("\n%s 이제부터 당신을 따라다닙니다.", actorSubject(actorName))); err != nil {
				return enginecmd.StatusDefault, err
			}
			roomOut := fmt.Sprintf("\n%s 이제부터 %s 따라다닙니다.", actorSubject(actorName), targetObject(targetName))
			for _, activeSession := range sessions {
				if activeSession.ActorID == "" || activeSession.ActorID == actorID || activeSession.ActorID == target.ActorID {
					continue
				}
				if playerRoomID(world, model.PlayerID(activeSession.ActorID)) == actorRoom {
					if err := send(activeSession.ID, session.Command{Write: roomOut}); err != nil {
						return enginecmd.StatusDefault, err
					}
				}
			}
		}
		return enginecmd.StatusDefault, nil
	}
}

func NewLoseHandler(world GroupWorld, groups *GroupMemory) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" || ctx.SessionID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		send, ok := sendToSessionFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}

		actorID := ctx.ActorID
		if len(resolved.Args) == 0 || strings.TrimSpace(resolved.Args[0]) == "" {
			leader, ok := groups.Unfollow(actorID)
			if !ok {
				ctx.WriteString("당신은 누구를 따라다니고 있지 않습니다.")
				return enginecmd.StatusDefault, nil
			}
			leaderName := playerDisplayName(world, leader)
			ctx.WriteString(fmt.Sprintf("당신은 %s 그만 따라다니기로 하였습니다.\n", targetObject(leaderName)))
			if !actorHasGroupDMInvisible(world, actorID) {
				if err := notifyActor(ctx, active(), send, leader, fmt.Sprintf("\n%s 이제 당신을 따라 다니지 않습니다.", actorTopic(playerDisplayName(world, actorID)))); err != nil {
					return enginecmd.StatusDefault, err
				}
			}
			return enginecmd.StatusDefault, nil
		}

		if err := revealGroupActor(world, actorID); err != nil {
			return enginecmd.StatusDefault, err
		}
		target, targetName, ok := findFollowerByName(world, groups, actorID, resolved.Args[0])
		if !ok {
			ctx.WriteString("그 사람은 당신을 따라다니고 있지 않습니다.")
			return enginecmd.StatusDefault, nil
		}
		if !groups.RemoveFollower(actorID, target) {
			ctx.WriteString("그 사람은 당신을 따라다니고 있지 않습니다.")
			return enginecmd.StatusDefault, nil
		}
		actorName := playerDisplayName(world, actorID)
		ctx.WriteString(fmt.Sprintf("당신은 %s 당신을 못따라 오도록 하였습니다.", actorSubject(targetName)))
		if !actorHasGroupDMInvisible(world, actorID) {
			sessions := active()
			if err := notifyActor(ctx, sessions, send, target, fmt.Sprintf("\n%s 당신이 못따라 오도록 하였습니다.", actorSubject(actorName))); err != nil {
				return enginecmd.StatusDefault, err
			}
			roomOut := fmt.Sprintf("\n%s %s 못따라 오도록 하였습니다.", actorSubject(actorName), targetObject(targetName))
			actorRoom := playerRoomID(world, model.PlayerID(actorID))
			for _, activeSession := range sessions {
				if activeSession.ActorID == "" || activeSession.ActorID == actorID || activeSession.ActorID == target {
					continue
				}
				if playerRoomID(world, model.PlayerID(activeSession.ActorID)) != actorRoom {
					continue
				}
				if err := send(activeSession.ID, session.Command{Write: roomOut}); err != nil {
					return enginecmd.StatusDefault, err
				}
			}
		}
		return enginecmd.StatusDefault, nil
	}
}

func NewGroupHandler(world GroupWorld, groups *GroupMemory) enginecmd.Handler {
	return func(ctx *enginecmd.Context, _ enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}
		leader, followers, ok := groupForActor(groups, ctx.ActorID)
		if !ok {
			ctx.WriteString("당신은 그룹에 속해 있지 않습니다.")
			return enginecmd.StatusDefault, nil
		}
		visibleFollowers := make([]string, 0, len(followers))
		for _, follower := range followers {
			if actorHasGroupDMInvisible(world, follower) {
				continue
			}
			visibleFollowers = append(visibleFollowers, follower)
		}
		if len(visibleFollowers) == 0 {
			ctx.WriteString("당신은 그룹에 속해 있지 않습니다.")
			return enginecmd.StatusDefault, nil
		}
		var b strings.Builder
		b.WriteString("그룹원:\n")
		b.WriteString(renderGroupMemberLine(world, leader, true))
		for _, follower := range visibleFollowers {
			b.WriteString(renderGroupMemberLine(world, follower, false))
		}
		out := strings.TrimRight(b.String(), "\n")
		ctx.WriteString(out)
		return enginecmd.StatusDefault, nil
	}
}

func NewGroupTalkHandler(world GroupWorld, groups *GroupMemory) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" || ctx.SessionID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		send, ok := sendToSessionFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		leader, followers, ok := groupForActor(groups, ctx.ActorID)
		if !ok {
			ctx.WriteString("당신은 그룹에 속해있지 않습니다.\n")
			return enginecmd.StatusDefault, nil
		}
		message := legacyCutCommandMessageFromResolved(resolved)
		if message == "" {
			ctx.WriteString("그룹원들에게 무슨말을 하시려구요?\n")
			return enginecmd.StatusDefault, nil
		}
		if actorHasGroupFlag(world, ctx.ActorID, "PSILNC", "silenced") {
			ctx.WriteString("입이 막혀 말이 나오질 않습니다.\n")
			return enginecmd.StatusDefault, nil
		}

		sessions := active()
		activeByActor := make(map[string]ActiveSession, len(sessions))
		for _, activeSession := range sessions {
			if activeSession.ActorID == "" {
				continue
			}
			activeByActor[activeSession.ActorID] = activeSession
		}
		writeToActor := func(actorID string, out string) error {
			activeSession, ok := activeByActor[actorID]
			if !ok {
				return nil
			}
			if string(activeSession.ID) == ctx.SessionID {
				ctx.WriteString(out)
				return nil
			}
			return send(activeSession.ID, session.Command{Write: out})
		}

		out := fmt.Sprintf("%s 그룹원들에게 \"%s\"라고 말합니다.\n", actorSubject(playerDisplayName(world, ctx.ActorID)), message)
		actorClass := actorGroupClass(world, ctx.ActorID)
		sendGroupTalk := func(actorID string, leaderTarget bool) error {
			if actorHasGroupFlag(world, actorID, "PIGNOR", "ignoreTalk", "talkIgnore", "ignoreAllTalk") &&
				actorClass < legacyClassCaretaker &&
				!actorHasGroupDMInvisible(world, actorID) {
				name := playerDisplayName(world, actorID)
				if leaderTarget {
					ctx.WriteString(fmt.Sprintf("%s님은 이야기 듣기 거부 상태입니다.\n", name))
				} else {
					ctx.WriteString(fmt.Sprintf("%s는 이야기 듣기 거부 상태입니다.\n", name))
				}
				return nil
			}
			return writeToActor(actorID, out)
		}
		foundVisible := false
		for _, follower := range followers {
			if !actorHasGroupDMInvisible(world, follower) {
				foundVisible = true
			}
			if err := sendGroupTalk(follower, false); err != nil {
				return enginecmd.StatusDefault, err
			}
		}
		if !foundVisible {
			ctx.WriteString("당신은 그룹에 속해있지 않습니다.\n")
			return enginecmd.StatusDefault, nil
		}
		if err := sendGroupTalk(leader, true); err != nil {
			return enginecmd.StatusDefault, err
		}
		return enginecmd.StatusDefault, nil
	}
}

func NewGroupMoveHandler(world GroupWorld, groups *GroupMemory, base enginecmd.Handler) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if base == nil {
			return enginecmd.StatusDefault, nil
		}
		actorID := ""
		fromRoom := model.RoomID("")
		if ctx != nil {
			actorID = ctx.ActorID
			fromRoom = playerRoomID(world, model.PlayerID(actorID))
		}
		status, err := base(ctx, resolved)
		if err != nil || actorID == "" || status == enginecmd.StatusDisconnect {
			return status, err
		}
		toRoom := playerRoomID(world, model.PlayerID(actorID))
		if fromRoom.IsZero() || toRoom.IsZero() || fromRoom == toRoom {
			return status, nil
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return status, nil
		}
		send, ok := sendToSessionFunc(ctx)
		if !ok {
			return status, nil
		}
		exitName := groupMoveExitName(resolved)
		if exitName == "" {
			return status, nil
		}
		activeByActor := map[string]ActiveSession{}
		for _, activeSession := range active() {
			if activeSession.ActorID != "" {
				activeByActor[activeSession.ActorID] = activeSession
			}
		}
		leaderName := playerDisplayName(world, actorID)
		for _, follower := range groups.FollowersOf(actorID) {
			if _, ok := activeByActor[follower]; !ok {
				continue
			}
			if playerRoomID(world, model.PlayerID(follower)) != fromRoom {
				continue
			}
			if err := world.MovePlayer(model.PlayerID(follower), exitName); err != nil {
				continue
			}
			viewer := enginecmd.LookViewerFromContext(&enginecmd.Context{
				ActorID: follower,
				Values:  ctx.Values,
			})
			viewer, room, err := enginecmd.CurrentRoom(world, viewer)
			if err != nil {
				continue
			}
			out := fmt.Sprintf("\n%s님을 따라갑니다.\n", leaderName)
			out += enginecmd.RenderRoomLook(world, room, viewer)
			if err := send(activeByActor[follower].ID, session.Command{Write: out}); err != nil {
				return status, err
			}
		}
		return status, nil
	}
}

func groupForActor(groups *GroupMemory, actorID string) (string, []string, bool) {
	if leader, ok := groups.LeaderOf(actorID); ok {
		followers := groups.FollowersOf(leader)
		return leader, followers, len(followers) > 0
	}
	followers := groups.FollowersOf(actorID)
	return actorID, followers, len(followers) > 0
}

func groupSnapshotCreatureID(world PlayerLookup, actorID string) model.CreatureID {
	if world == nil || actorID == "" {
		return ""
	}
	player, ok := world.Player(model.PlayerID(actorID))
	if !ok {
		return ""
	}
	return player.CreatureID
}

func renderGroupMemberLine(world GroupWorld, actorID string, leader bool) string {
	player, _ := world.Player(model.PlayerID(actorID))
	name := strings.TrimSpace(player.DisplayName)
	if name == "" {
		name = actorID
	}
	hpCur, hpMax, mpCur, mpMax := 0, 0, 0, 0
	if !player.CreatureID.IsZero() {
		if creature, ok := world.Creature(player.CreatureID); ok {
			hpCur = creature.Stats["hpCurrent"]
			hpMax = creature.Stats["hpMax"]
			mpCur = creature.Stats["mpCurrent"]
			mpMax = creature.Stats["mpMax"]
		}
	}
	suffix := ""
	if leader {
		suffix = " (대장)"
	}
	return fmt.Sprintf("  %14s  체력:%4d/%4d 도력:%4d/%4d%s\n", name, hpCur, hpMax, mpCur, mpMax, suffix)
}

func actorTopic(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "누군가"
	}
	return name + krtext.Particle(name, '0')
}

func findActivePlayerSessionInRoom(world PlayerLookup, sessions []ActiveSession, target string, roomID model.RoomID) (ActiveSession, string, bool) {
	found, name, ok := findActivePlayerSession(world, sessions, target)
	if !ok {
		return ActiveSession{}, "", false
	}
	if !roomID.IsZero() && playerRoomID(world, model.PlayerID(found.ActorID)) != roomID {
		return ActiveSession{}, "", false
	}
	return found, name, true
}

func findFollowerByName(world PlayerLookup, groups *GroupMemory, leader, target string) (string, string, bool) {
	target = legacyLowercizeASCII(target, true)
	for _, follower := range groups.FollowersOf(leader) {
		name := playerDisplayName(world, follower)
		if target == name || target == follower || strings.HasPrefix(name, target) || strings.HasPrefix(follower, target) {
			return follower, name, true
		}
	}
	return "", "", false
}

func actorHasGroupDMInvisible(world GroupWorld, actorID string) bool {
	return actorHasGroupFlag(world, actorID, "PDMINV", "dmInvisible")
}

func actorHasGroupFlag(world GroupWorld, actorID string, names ...string) bool {
	player, ok := world.Player(model.PlayerID(actorID))
	if !ok || player.CreatureID.IsZero() {
		return false
	}
	creature, ok := world.Creature(player.CreatureID)
	return ok && creatureHasNormalizedFlag(creature, names...)
}

func actorGroupClass(world GroupWorld, actorID string) int {
	player, ok := world.Player(model.PlayerID(actorID))
	if !ok || player.CreatureID.IsZero() {
		return 0
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return 0
	}
	return creatureStat(creature, "class")
}

func groupThirdPersonPronoun(world GroupWorld, actorID string) string {
	player, ok := world.Player(model.PlayerID(actorID))
	if !ok || player.CreatureID.IsZero() {
		return "그"
	}
	creature, ok := world.Creature(player.CreatureID)
	if ok && creatureHasNormalizedFlag(creature, "PMALES", "male") {
		return "그"
	}
	return "그녀"
}

func revealGroupActor(world GroupWorld, actorID string) error {
	player, ok := world.Player(model.PlayerID(actorID))
	if !ok {
		return nil
	}
	if !player.CreatureID.IsZero() {
		creature, creatureOK := world.Creature(player.CreatureID)
		if updater, ok := world.(interface {
			UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
		}); ok {
			if _, err := updater.UpdateCreatureTags(player.CreatureID, nil, []string{"hidden", "phiddn", "PHIDDN"}); err != nil {
				return err
			}
		}
		if creatureOK && creature.Stats != nil && creature.Stats["PHIDDN"] != 0 {
			if setter, ok := world.(interface {
				SetCreatureStat(model.CreatureID, string, int) error
			}); ok {
				if err := setter.SetCreatureStat(player.CreatureID, "PHIDDN", 0); err != nil {
					return err
				}
			}
		}
	}
	if !player.ID.IsZero() {
		if updater, ok := world.(interface {
			UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
		}); ok {
			if _, err := updater.UpdatePlayerTags(player.ID, nil, []string{"hidden", "phiddn", "PHIDDN"}); err != nil {
				return err
			}
		}
	}
	return nil
}

func legacyLowercizeASCII(value string, capitalizeFirst bool) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	buf := []byte(value)
	for i, ch := range buf {
		if ch >= 'A' && ch <= 'Z' {
			buf[i] = ch + ('a' - 'A')
		}
	}
	if capitalizeFirst && buf[0] >= 'a' && buf[0] <= 'z' {
		buf[0] -= 'a' - 'A'
	}
	return string(buf)
}

func notifyActor(ctx *enginecmd.Context, sessions []ActiveSession, send func(session.ID, session.Command) error, actorID string, text string) error {
	for _, activeSession := range sessions {
		if activeSession.ActorID != actorID {
			continue
		}
		if string(activeSession.ID) == ctx.SessionID {
			ctx.WriteString(text)
			return nil
		}
		return send(activeSession.ID, session.Command{Write: text})
	}
	return nil
}

func groupMoveExitName(resolved enginecmd.ResolvedCommand) string {
	if resolved.Spec.Handler == "move" {
		switch strings.TrimSpace(resolved.Command()) {
		case "8", "ㅂ":
			return "북"
		case "2", "ㄴ":
			return "남"
		case "6", "ㄷ":
			return "동"
		case "4", "ㅅ":
			return "서"
		case "9", "ㅇ":
			return "위"
		case "3", "ㅁ":
			return "밑"
		case "나가":
			return "밖"
		default:
			return strings.TrimSpace(resolved.Command())
		}
	}
	if len(resolved.Args) == 0 {
		return ""
	}
	return strings.TrimSpace(resolved.Args[0])
}
