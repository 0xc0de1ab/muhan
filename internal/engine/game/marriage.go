package game

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	enginecmd "github.com/0xc0de1ab/muhan/internal/engine/command"
	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
	"github.com/0xc0de1ab/muhan/internal/session"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type MarriageInviteWorld interface {
	ActionWorld
	HasMarriageInvite(model.PlayerID, model.SpecialID) bool
	MarriageInvites(model.SpecialID) []string
	AddMarriageInvite(model.SpecialID, string) (bool, error)
	RemoveMarriageInvite(model.SpecialID, string) (bool, error)
}

type MarriageWorld interface {
	ActionWorld
	UpdateCreatureMarriageState(model.CreatureID, int, bool, bool) (model.Creature, error)
}

type marriageRequest struct {
	Requester model.PlayerID
	Target    model.PlayerID
}

type MarriageRequests struct {
	mu          sync.Mutex
	byRequester map[model.PlayerID]marriageRequest
	nextID      int
}

func NewMarriageRequests() *MarriageRequests {
	return &MarriageRequests{
		byRequester: map[model.PlayerID]marriageRequest{},
		nextID:      1,
	}
}

var defaultMarriageRequests = NewMarriageRequests()

func NewInviteHandler(world MarriageInviteWorld) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		player, creature, ok, err := marriageActor(world, ctx)
		if err != nil || !ok {
			return enginecmd.StatusDefault, err
		}
		marriageID, ok := marriageSpecialID(creature)
		if !ok {
			ctx.WriteString("당신은 사용할 권한이 없습니다.")
			return enginecmd.StatusDefault, nil
		}
		actorRoomID := marriageActorRoom(player, creature)
		room, ok := world.Room(actorRoomID)
		if !ok || !marriageRoomHasAnyFlag(room, "ronmar", "onlyMarried", "marriedOnly") {
			ctx.WriteString("당신의 집에서만 가능합니다.")
			return enginecmd.StatusDefault, nil
		}
		if roomSpecial, hasRoomSpecial := marriageRoomSpecial(room); hasRoomSpecial && roomSpecial != marriageID {
			ctx.WriteString("당신의 집에서만 가능합니다.")
			return enginecmd.StatusDefault, nil
		}

		if len(resolved.Args) != 1 {
			writeMarriageInviteList(ctx, world.MarriageInvites(marriageID))
			return enginecmd.StatusDefault, nil
		}
		targetArg := strings.TrimSpace(resolved.Args[0])
		if !krtext.IsAllHangulSyllables(targetArg) {
			ctx.WriteString("사람의 이름은 한글로 적어야 합니다.")
			return enginecmd.StatusDefault, nil
		}

		hadInvites := len(world.MarriageInvites(marriageID)) > 0
		removed, err := world.RemoveMarriageInvite(marriageID, targetArg)
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		if removed {
			ctx.WriteString("초대 대상에서 삭제하였습니다.\n")
			return enginecmd.StatusDefault, nil
		}
		if _, err := world.AddMarriageInvite(marriageID, targetArg); err != nil {
			return enginecmd.StatusDefault, err
		}
		if hadInvites {
			ctx.WriteString("초대 대상에 추가하였습니다.\n")
		} else {
			ctx.WriteString("초대 대상에 추가했습니다.\n")
		}
		return enginecmd.StatusDefault, nil
	}
}

func updateMarriageListFile(root string, aliceName, bobName string) error {
	listPath := filepath.Join(root, "player", "marriage", "list")
	existingContent, mnum := readMarriageListContent(root)

	newLine := fmt.Sprintf("%d : %s 님과 %s 님의 결혼\n", mnum+1, aliceName, bobName)
	newContent := newLine + existingContent

	encodedBytes, err := legacykr.EncodeEUCKR(newContent)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(listPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(listPath, encodedBytes, 0644)
}

func readMarriageListContent(root string) (string, int) {
	listPath := filepath.Join(root, "player", "marriage", "list")
	data, err := os.ReadFile(listPath)
	if err != nil || len(data) == 0 {
		return "", 0
	}
	decoded, err := legacykr.DecodeEUCKR(data)
	if err != nil {
		return "", 0
	}
	return decoded, marriageListLeadingNumber(decoded)
}

func marriageListLeadingNumber(decoded string) int {
	var sb strings.Builder
	for _, r := range decoded {
		if r >= '0' && r <= '9' {
			sb.WriteRune(r)
		} else if sb.Len() > 0 {
			break
		}
	}
	if sb.Len() == 0 {
		return 0
	}
	n, _ := strconv.Atoi(sb.String())
	return n
}

func nextMarriageID(root string, fallback int) int {
	if strings.TrimSpace(root) != "" {
		_, current := readMarriageListContent(root)
		if current > 0 {
			return current + 1
		}
	}
	if fallback > 0 {
		return fallback
	}
	return 1
}

func writeMarriagePendingRequestFile(root string, requester model.Player, target model.Player) {
	if strings.TrimSpace(root) == "" {
		return
	}
	_ = writeMarriageNameFile(root, marriageFileStem(requester), marriageFileStem(target))
}

func writeMarriagePartnerFiles(root string, first model.Player, second model.Player) {
	if strings.TrimSpace(root) == "" {
		return
	}
	firstName := marriageFileStem(first)
	secondName := marriageFileStem(second)
	_ = writeMarriageNameFile(root, firstName, secondName)
	_ = writeMarriageNameFile(root, secondName, firstName)
}

func writeMarriageNameFile(root string, ownerName string, partnerName string) error {
	encodedOwner, err := legacykr.EncodeEUCKR(ownerName)
	if err != nil {
		return err
	}
	encodedPartner, err := legacykr.EncodeEUCKR(partnerName)
	if err != nil {
		return err
	}
	path := filepath.Join(root, "player", "marriage", string(encodedOwner))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, append(encodedPartner, '\n'), 0644)
}

func readMarriagePendingRequester(root string, requester model.Player) (string, bool, error) {
	if strings.TrimSpace(root) == "" {
		return "", false, nil
	}
	for _, name := range marriageFileStemCandidates(requester) {
		encodedName, err := legacykr.EncodeEUCKR(name)
		if err != nil {
			continue
		}
		path := filepath.Join(root, "player", "marriage", string(encodedName))
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", false, err
		}
		decoded, err := legacykr.DecodeEUCKR(data)
		if err != nil {
			return "", false, err
		}
		return strings.TrimSpace(decoded), true, nil
	}
	return "", false, nil
}

func marriageRequesterMatchesPlayer(requester string, player model.Player) bool {
	requester = strings.TrimSpace(requester)
	if requester == "" {
		return false
	}
	for _, name := range marriageFileStemCandidates(player) {
		if requester == name {
			return true
		}
	}
	return false
}

func marriageFileStem(player model.Player) string {
	for _, name := range marriageFileStemCandidates(player) {
		return name
	}
	return strings.TrimSpace(string(player.ID))
}

func marriageFileStemCandidates(player model.Player) []string {
	return uniqueMarriageNames([]string{
		strings.TrimPrefix(strings.TrimSpace(string(player.ID)), "player:"),
		strings.TrimSpace(player.DisplayName),
		strings.TrimSpace(string(player.ID)),
	})
}

func uniqueMarriageNames(names []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func NewMarriageHandler(world MarriageWorld, store *MarriageRequests, roots ...string) enginecmd.Handler {
	if store == nil {
		store = defaultMarriageRequests
	}
	var root string
	if len(roots) > 0 {
		root = roots[0]
	}
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		player, creature, ok, err := marriageActor(world, ctx)
		if err != nil || !ok {
			return enginecmd.StatusDefault, err
		}
		actorRoomID := marriageActorRoom(player, creature)
		room, ok := world.Room(actorRoomID)
		if !ok || !marriageRoomHasAnyFlag(room, "rmarri", "marriageHall", "marriageCeremony") {
			ctx.WriteString("이곳은 결혼식장이 아닙니다.")
			return enginecmd.StatusDefault, nil
		}
		if age, ok := marriageCreatureAgeYears(creature); ok && age < 20 {
			ctx.WriteString("당신은 결혼할 수 있는 나이가 아닙니다.")
			return enginecmd.StatusDefault, nil
		}
		_, storePending := store.PendingTarget(player.ID)
		if storePending || marriagePlayerPendingRequest(player, creature) {
			store.Cancel(player.ID)
			if _, err := world.UpdateCreatureMarriageState(creature.ID, 0, false, false); err != nil {
				return enginecmd.StatusDefault, err
			}
			ctx.WriteString("결혼 신청을 취소합니다.")
			return enginecmd.StatusDefault, nil
		}
		if marriagePlayerMarried(player, creature) {
			ctx.WriteString("당신은 이미 결혼했잖아요!")
			return enginecmd.StatusDefault, nil
		}

		targetArg := strings.TrimSpace(strings.Join(resolved.Args, " "))
		if targetArg == "" {
			ctx.WriteString("누구와 결혼을 하시려고요?")
			return enginecmd.StatusDefault, nil
		}
		active, ok := activeSessionsFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		send, ok := sendToSessionFunc(ctx)
		if !ok {
			return enginecmd.StatusDefault, ErrSocialContextMissing
		}
		sessions := active()
		target, targetName, found := marriageFindActivePlayer(world, sessions, targetArg)
		if !found {
			ctx.WriteString("그런 사람을 찾을 수가 없군요.")
			return enginecmd.StatusDefault, nil
		}
		targetID := model.PlayerID(target.ActorID)
		if targetID == player.ID {
			ctx.WriteString("자기 자신과는 결혼할 수 없습니다.\n")
			return enginecmd.StatusDefault, nil
		}
		targetPlayer, targetCreature, ok := marriagePlayerCreature(world, targetID)
		if !ok {
			ctx.WriteString("그런 사람을 찾을 수가 없군요.")
			return enginecmd.StatusDefault, nil
		}
		if marriageTargetHiddenFromActor(creature, targetCreature) {
			ctx.WriteString("그런 사람을 찾을 수가 없군요.")
			return enginecmd.StatusDefault, nil
		}
		if marriageCreatureMale(creature) && marriageCreatureMale(targetCreature) {
			ctx.WriteString("남자끼리 결혼하시려고요?")
			return enginecmd.StatusDefault, nil
		}
		if !marriageCreatureMale(creature) && !marriageCreatureMale(targetCreature) {
			ctx.WriteString("여자끼리 결혼하시려고요?")
			return enginecmd.StatusDefault, nil
		}
		if age, ok := marriageCreatureAgeYears(targetCreature); ok && age < 20 {
			ctx.WriteString("그사람은 아직 결혼할 나이가 되지 않았습니다.")
			return enginecmd.StatusDefault, nil
		}
		if marriagePlayerMarried(targetPlayer, targetCreature) {
			ctx.WriteString("그사람은 이미 결혼한 사람입니다.")
			return enginecmd.StatusDefault, nil
		}

		if pendingTarget, pending := store.PendingTarget(targetID); pending {
			if pendingTarget != player.ID {
				ctx.WriteString("그사람은 다른 사람과 결혼을 준비중입니다.")
				return enginecmd.StatusDefault, nil
			}
			storeMarriageID, accepted := store.Accept(targetID, player.ID)
			if !accepted {
				ctx.WriteString("그사람은 당신에게 결혼을 신청하지 않았습니다.")
				return enginecmd.StatusDefault, nil
			}
			marriageID := nextMarriageID(root, storeMarriageID)
			return completeMarriageAcceptance(ctx, world, root, player, creature, targetPlayer, targetCreature, target, targetName, sessions, send, marriageID)
		}

		if marriagePlayerPendingRequest(targetPlayer, targetCreature) {
			requester, ok, err := readMarriagePendingRequester(root, targetPlayer)
			if err != nil {
				return enginecmd.StatusDefault, err
			}
			if !ok || !marriageRequesterMatchesPlayer(requester, player) {
				ctx.WriteString("그사람은 다른 사람과 결혼을 준비중입니다.")
				return enginecmd.StatusDefault, nil
			}
			marriageID := nextMarriageID(root, 1)
			return completeMarriageAcceptance(ctx, world, root, player, creature, targetPlayer, targetCreature, target, targetName, sessions, send, marriageID)
		}

		if existingTarget, pending := store.Request(player.ID, targetID); pending {
			ctx.WriteString(fmt.Sprintf("이미 %s님에게 결혼을 신청했습니다.", playerDisplayName(world, string(existingTarget))))
			return enginecmd.StatusDefault, nil
		}
		if _, err := world.UpdateCreatureMarriageState(creature.ID, 0, false, true); err != nil {
			store.Cancel(player.ID)
			return enginecmd.StatusDefault, err
		}
		actorName := playerDisplayName(world, string(player.ID))
		writeMarriagePendingRequestFile(root, player, targetPlayer)
		ctx.WriteString(fmt.Sprintf("당신은 %s님에게 결혼을 신청하였습니다.", targetName))
		return enginecmd.StatusDefault, notifyActor(ctx, sessions, send, target.ActorID, fmt.Sprintf("\n%s님이 당신에게 결혼을 신청합니다.", actorName))
	}
}

func completeMarriageAcceptance(
	ctx *enginecmd.Context,
	world MarriageWorld,
	root string,
	player model.Player,
	creature model.Creature,
	targetPlayer model.Player,
	targetCreature model.Creature,
	target ActiveSession,
	targetName string,
	sessions []ActiveSession,
	send func(session.ID, session.Command) error,
	marriageID int,
) (enginecmd.Status, error) {
	if marriageID <= 0 {
		marriageID = 1
	}
	if _, err := world.UpdateCreatureMarriageState(creature.ID, marriageID, true, false); err != nil {
		return enginecmd.StatusDefault, err
	}
	if _, err := world.UpdateCreatureMarriageState(targetCreature.ID, marriageID, true, false); err != nil {
		return enginecmd.StatusDefault, err
	}

	ctx.WriteString(fmt.Sprintf("당신은 %s님의 결혼신청을 받아들입니다.\n", targetName))

	actorName := playerDisplayName(world, string(player.ID))
	writeMarriagePartnerFiles(root, player, targetPlayer)
	if root != "" {
		_ = updateMarriageListFile(root, marriageFileStem(targetPlayer), marriageFileStem(player))
	}

	if err := notifyActor(ctx, sessions, send, target.ActorID, fmt.Sprintf("\n%s님이 당신의 결혼신청을 받아들였습니다.", actorName)); err != nil {
		return enginecmd.StatusDefault, err
	}

	broadcastMsg := fmt.Sprintf("\n### %s님과 %s님이 결혼을 하였습니다.", targetName, actorName)
	for _, activeSession := range sessions {
		if string(activeSession.ID) == ctx.SessionID {
			ctx.WriteString(broadcastMsg)
		} else {
			_ = send(activeSession.ID, session.Command{Write: broadcastMsg})
		}
	}

	return enginecmd.StatusDefault, nil
}

func marriageRequestStore(stores ...*MarriageRequests) *MarriageRequests {
	for _, store := range stores {
		if store != nil {
			return store
		}
	}
	return defaultMarriageRequests
}

func (m *MarriageRequests) Request(requester model.PlayerID, target model.PlayerID) (model.PlayerID, bool) {
	if m == nil || requester.IsZero() || target.IsZero() || requester == target {
		return "", false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.byRequester == nil {
		m.byRequester = map[model.PlayerID]marriageRequest{}
	}
	if existing, ok := m.byRequester[requester]; ok {
		return existing.Target, true
	}
	m.byRequester[requester] = marriageRequest{Requester: requester, Target: target}
	return "", false
}

func (m *MarriageRequests) PendingTarget(requester model.PlayerID) (model.PlayerID, bool) {
	if m == nil || requester.IsZero() {
		return "", false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	request, ok := m.byRequester[requester]
	return request.Target, ok
}

func (m *MarriageRequests) Cancel(requester model.PlayerID) (model.PlayerID, bool) {
	if m == nil || requester.IsZero() {
		return "", false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	request, ok := m.byRequester[requester]
	if ok {
		delete(m.byRequester, requester)
	}
	return request.Target, ok
}

func (m *MarriageRequests) Accept(requester model.PlayerID, target model.PlayerID) (int, bool) {
	if m == nil || requester.IsZero() || target.IsZero() {
		return 0, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	request, ok := m.byRequester[requester]
	if !ok || request.Target != target {
		return 0, false
	}
	delete(m.byRequester, requester)
	if m.nextID <= 0 {
		m.nextID = 1
	}
	marriageID := m.nextID
	m.nextID++
	return marriageID, true
}

func marriageActor(world ActionWorld, ctx *enginecmd.Context) (model.Player, model.Creature, bool, error) {
	if ctx == nil || ctx.ActorID == "" {
		return model.Player{}, model.Creature{}, false, ErrSocialActorRequired
	}
	if world == nil {
		return model.Player{}, model.Creature{}, false, fmt.Errorf("game: marriage world required")
	}
	player, ok := world.Player(model.PlayerID(ctx.ActorID))
	if !ok || player.CreatureID.IsZero() {
		ctx.WriteString("현재 사용자를 찾을 수 없습니다.\n")
		return model.Player{}, model.Creature{}, false, nil
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		ctx.WriteString("현재 사용자를 찾을 수 없습니다.\n")
		return model.Player{}, model.Creature{}, false, nil
	}
	return player, creature, true, nil
}

func marriagePlayerCreature(world ActionWorld, playerID model.PlayerID) (model.Player, model.Creature, bool) {
	if world == nil || playerID.IsZero() {
		return model.Player{}, model.Creature{}, false
	}
	player, ok := world.Player(playerID)
	if !ok || player.CreatureID.IsZero() {
		return model.Player{}, model.Creature{}, false
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return model.Player{}, model.Creature{}, false
	}
	return player, creature, true
}

func marriageActorRoom(player model.Player, creature model.Creature) model.RoomID {
	if !player.RoomID.IsZero() {
		return player.RoomID
	}
	return creature.RoomID
}

func marriageSpecialID(creature model.Creature) (model.SpecialID, bool) {
	for _, key := range []string{"marriageID", "dailyMarriageMax", "legacyDailyMarriageMax"} {
		if value, ok := creatureIntValue(creature, key); ok && value > 0 {
			return model.SpecialID(value), true
		}
	}
	return 0, false
}

func writeMarriageInviteList(ctx *enginecmd.Context, names []string) {
	if len(names) == 0 {
		ctx.WriteString("초대한 사람이 없습니다.")
		return
	}
	var out strings.Builder
	out.WriteString("당신이 초대한 사람들 : \n")
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out.WriteString(name)
		out.WriteByte('\n')
	}
	ctx.WriteString(out.String())
}

func marriageFindActivePlayerInRoom(world ActionWorld, sessions []ActiveSession, target string, roomID model.RoomID) (ActiveSession, string, bool) {
	if roomID.IsZero() {
		return ActiveSession{}, "", false
	}
	found, name, ok := findActivePlayerSession(world, sessions, target)
	if !ok {
		return ActiveSession{}, "", false
	}
	player, creature, ok := marriagePlayerCreature(world, model.PlayerID(found.ActorID))
	if !ok || marriageActorRoom(player, creature) != roomID {
		return ActiveSession{}, "", false
	}
	return found, name, true
}

func marriageFindActivePlayer(world ActionWorld, sessions []ActiveSession, target string) (ActiveSession, string, bool) {
	return findActivePlayerSession(world, sessions, target)
}

func marriagePlayerMarried(player model.Player, creature model.Creature) bool {
	if _, ok := marriageSpecialID(creature); ok {
		return true
	}
	return marriageHasAnyTag(player.Metadata.Tags, "PMARRI", "married", "marriage") ||
		marriageCreatureFlagEnabled(creature, "PMARRI", "married", "marriage", "marriageFlag")
}

func marriagePlayerPendingRequest(player model.Player, creature model.Creature) bool {
	return marriageHasAnyTag(player.Metadata.Tags, "PRDMAR", "marriagePending", "pendingMarriage") ||
		marriageCreatureFlagEnabled(creature, "PRDMAR", "marriagePending", "pendingMarriage")
}

func marriageCreatureAgeYears(creature model.Creature) (int, bool) {
	for _, key := range []string{"legacyHoursInterval", "LT_HOURS_interval", "lastHoursInterval"} {
		if value, ok := creatureIntValue(creature, key); ok {
			return 18 + value/86400, true
		}
	}
	for _, key := range []string{"age", "legacyAge", "legacyAgeYears", "ageYears"} {
		if value, ok := creatureIntValue(creature, key); ok {
			return value, true
		}
	}
	for _, key := range []string{"playDays", "daysPlayed", "legacyPlayedDays"} {
		if value, ok := creatureIntValue(creature, key); ok {
			return 18 + value, true
		}
	}
	return 0, false
}

func marriageCreatureMale(creature model.Creature) bool {
	return marriageCreatureFlagEnabled(creature, "PMALES", "male")
}

func marriageTargetHiddenFromActor(actor model.Creature, target model.Creature) bool {
	if marriageCreatureFlagEnabled(target, "PDMINV", "dmInvisible", "dmInvis") {
		return true
	}
	if marriageCreatureFlagEnabled(actor, "PBLIND", "blind", "blinded") {
		return true
	}
	return marriageCreatureFlagEnabled(target, "PINVIS", "invisible", "invisibility") &&
		!marriageCreatureFlagEnabled(actor, "PDINVI", "detectInvisible", "detectInvis")
}

func marriageCreatureFlagEnabled(creature model.Creature, names ...string) bool {
	if marriageHasAnyTag(creature.Metadata.Tags, names...) {
		return true
	}
	targets := marriageNormalizedFlagSet(names...)
	for key, value := range creature.Stats {
		if _, ok := targets[marriageNormalizeFlagName(key)]; ok && value != 0 {
			return true
		}
	}
	for key, value := range creature.Properties {
		if _, ok := targets[marriageNormalizeFlagName(key)]; ok && propertyFlagEnabled(value) {
			return true
		}
		for _, token := range strings.FieldsFunc(value, marriageFlagTokenSeparator) {
			if _, ok := targets[marriageNormalizeFlagName(token)]; ok {
				return true
			}
		}
	}
	return false
}

func marriageRoomHasAnyFlag(room model.Room, names ...string) bool {
	if marriageHasAnyTag(room.Metadata.Tags, names...) {
		return true
	}
	targets := marriageNormalizedFlagSet(names...)
	for key, value := range room.Properties {
		if _, ok := targets[marriageNormalizeFlagName(key)]; ok && propertyFlagEnabled(value) {
			return true
		}
		for _, token := range strings.FieldsFunc(value, marriageFlagTokenSeparator) {
			if _, ok := targets[marriageNormalizeFlagName(token)]; ok {
				return true
			}
		}
	}
	return false
}

func marriageHasAnyTag(tags []string, names ...string) bool {
	targets := marriageNormalizedFlagSet(names...)
	for _, tag := range tags {
		if _, ok := targets[marriageNormalizeFlagName(tag)]; ok {
			return true
		}
	}
	return false
}

func marriageNormalizedFlagSet(names ...string) map[string]struct{} {
	targets := make(map[string]struct{}, len(names))
	for _, name := range names {
		if normalized := marriageNormalizeFlagName(name); normalized != "" {
			targets[normalized] = struct{}{}
		}
	}
	return targets
}

func marriageNormalizeFlagName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, " ", "")
	return name
}

func marriageFlagTokenSeparator(r rune) bool {
	return r == ',' || r == ';' || r == '|' || r == ' '
}

func marriageRoomSpecial(room model.Room) (model.SpecialID, bool) {
	if room.Properties == nil {
		return 0, false
	}
	if raw, ok := room.Properties["special"]; ok {
		return parseMarriageRoomSpecial(raw)
	}
	for key, raw := range room.Properties {
		if marriageNormalizeFlagName(key) != "special" {
			continue
		}
		return parseMarriageRoomSpecial(raw)
	}
	return 0, false
}

func parseMarriageRoomSpecial(raw string) (model.SpecialID, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, false
	}
	return model.SpecialID(value), true
}
