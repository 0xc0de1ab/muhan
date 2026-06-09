package game

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode"

	enginecmd "muhan/internal/engine/command"
	"muhan/internal/persist/legacykr"
	"muhan/internal/world/model"
)

const legacyVoteInvincibleClass = 9

type VoteWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
	Room(model.RoomID) (model.Room, bool)
}

type VoteMemory struct {
	mu         sync.Mutex
	title      string
	candidates []string
	choices    map[string]string
}

func NewVoteMemory() *VoteMemory {
	return &VoteMemory{choices: map[string]string{}}
}

func (m *VoteMemory) SetIssue(title string, candidates []string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.title = strings.TrimSpace(title)
	m.candidates = cleanVoteCandidates(candidates)
	m.choices = map[string]string{}
}

func (m *VoteMemory) ClearIssue() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.title = ""
	m.candidates = nil
	m.choices = map[string]string{}
}

func (m *VoteMemory) Choice(actorID string) (string, bool) {
	if m == nil || strings.TrimSpace(actorID) == "" {
		return "", false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	choice := strings.TrimSpace(m.choices[actorID])
	return choice, choice != ""
}

func (m *VoteMemory) Choices() map[string]string {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	choices := make(map[string]string, len(m.choices))
	for actorID, choice := range m.choices {
		choices[actorID] = choice
	}
	return choices
}

func NewVoteHandler(world VoteWorld, memory *VoteMemory, roots ...string) enginecmd.Handler {
	if memory == nil {
		memory = NewVoteMemory()
	}
	var root string
	if len(roots) > 0 {
		root = roots[0]
	}
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}

		player, creature, room, err := voteActorState(world, model.PlayerID(ctx.ActorID))
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		if !voteActorEligible(creature) {
			ctx.WriteString("당신은 투표할 나이가 아닙니다.\n")
			return enginecmd.StatusDefault, nil
		}
		if !voteRoomEnabled(room) {
			ctx.WriteString("투표소가 아닙니다.\n")
			return enginecmd.StatusDefault, nil
		}

		issue, ok := memory.issueSnapshot()
		if !ok {
			var empty bool
			var err error
			issue, ok, empty, err = legacyVoteIssue(root)
			if err != nil {
				return enginecmd.StatusDefault, err
			}
			if empty {
				ctx.WriteString("현재 투표할 안건이 없습니다.\n")
				return enginecmd.StatusDefault, nil
			}
			if !ok {
				ctx.WriteString("투표할 안건이 없네요.\n")
				return enginecmd.StatusDefault, nil
			}
		}
		if issue.voteQuestionLimit() == 0 {
			ctx.WriteString("현재 투표할 안건이 없습니다.\n")
			return enginecmd.StatusDefault, nil
		}

		state := &voteSessionState{
			root:     root,
			memory:   memory,
			actorID:  string(player.ID),
			fileName: votePlayerFileName(player),
			issue:    issue,
		}
		return state.start(ctx)
	}
}

func legacyVoteIssue(root string) (voteIssueSnapshot, bool, bool, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return voteIssueSnapshot{}, false, false, nil
	}
	path := filepath.Join(root, "post", "ISSUE")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return voteIssueSnapshot{}, false, false, nil
		}
		return voteIssueSnapshot{}, false, false, fmt.Errorf("read vote issue %q: %w", path, err)
	}
	text, err := legacykr.ValidUTF8OrDecodeContext(legacykr.Context{Path: path, Field: "vote issue"}, data)
	if err != nil {
		return voteIssueSnapshot{}, false, false, fmt.Errorf("decode vote issue %q: %w", path, err)
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return voteIssueSnapshot{}, false, true, nil
	}
	fields := strings.Fields(lines[0])
	if len(fields) == 0 {
		return voteIssueSnapshot{}, false, true, nil
	}
	count, err := strconv.Atoi(fields[0])
	if err != nil || count <= 0 {
		return voteIssueSnapshot{}, false, true, nil
	}
	title := strings.TrimSpace(strings.Join(fields[1:], " "))
	candidates := make([]string, 0, 79)
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		candidates = append(candidates, line)
		if len(candidates) == 79 {
			break
		}
	}
	if len(candidates) == 0 {
		return voteIssueSnapshot{}, false, true, nil
	}
	return voteIssueSnapshot{title: title, questionCount: count, candidates: candidates}, true, false, nil
}

func votePlayerFileName(player model.Player) string {
	name := strings.TrimSpace(player.DisplayName)
	if name == "" {
		name = strings.TrimPrefix(strings.TrimSpace(string(player.ID)), "player:")
	}
	if encodedNameBytes, err := legacykr.EncodeEUCKR(name); err == nil {
		return string(encodedNameBytes)
	}
	return name
}

type voteIssueSnapshot struct {
	title         string
	questionCount int
	candidates    []string
}

func (m *VoteMemory) issueSnapshot() (voteIssueSnapshot, bool) {
	if m == nil {
		return voteIssueSnapshot{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.candidates) == 0 {
		return voteIssueSnapshot{}, false
	}
	return voteIssueSnapshot{
		title:         m.title,
		questionCount: len(m.candidates),
		candidates:    append([]string(nil), m.candidates...),
	}, true
}

func (m *VoteMemory) recordChoice(actorID string, choice string) bool {
	if m == nil || strings.TrimSpace(actorID) == "" || strings.TrimSpace(choice) == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.choices == nil {
		m.choices = map[string]string{}
	}
	_, changed := m.choices[actorID]
	m.choices[actorID] = choice
	return changed
}

func (m *VoteMemory) clearChoice(actorID string) {
	if m == nil || strings.TrimSpace(actorID) == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.choices, actorID)
}

type voteSessionState struct {
	root     string
	memory   *VoteMemory
	actorID  string
	fileName string
	issue    voteIssueSnapshot
	choices  []byte
}

func (s *voteSessionState) start(ctx *enginecmd.Context) (enginecmd.Status, error) {
	if s.existingVote() {
		ctx.WriteString("당신은 이미 투표를 했습니다.\n")
		ctx.WriteString("당신의 선택을 바꾸시겠습니까? (y/n): ")
		if !enginecmd.SetPendingLineHandler(ctx, s.handleChangeConfirmation) {
			return enginecmd.StatusDefault, fmt.Errorf("투표 입력 상태를 시작할 수 없습니다")
		}
		return enginecmd.StatusDoPrompt, nil
	}
	return s.promptQuestion(ctx, 0)
}

func (s *voteSessionState) handleChangeConfirmation(ctx *enginecmd.Context, line string) (enginecmd.Status, error) {
	if len(line) > 0 && (line[0] == 'y' || line[0] == 'Y') {
		if err := s.deleteExistingVote(); err != nil {
			enginecmd.ClearPendingLineHandler(ctx)
			return enginecmd.StatusDefault, err
		}
		return s.promptQuestion(ctx, 0)
	}
	enginecmd.ClearPendingLineHandler(ctx)
	ctx.WriteString("중단합니다.\n")
	return enginecmd.StatusDefault, nil
}

func (s *voteSessionState) handleChoice(ctx *enginecmd.Context, line string) (enginecmd.Status, error) {
	choice, ok := voteChoiceFromArg(line, 0)
	if !ok {
		enginecmd.ClearPendingLineHandler(ctx)
		ctx.WriteString("잘못된 선택입니다. 중단합니다.\n")
		return enginecmd.StatusDefault, nil
	}
	s.choices = append(s.choices, choice[0])
	if len(s.choices) >= s.issue.voteQuestionLimit() {
		return s.finish(ctx)
	}
	return s.promptQuestion(ctx, len(s.choices))
}

func (s *voteSessionState) promptQuestion(ctx *enginecmd.Context, index int) (enginecmd.Status, error) {
	question, ok := s.issue.voteQuestion(index)
	if !ok {
		return s.finish(ctx)
	}
	ctx.WriteString("\n")
	ctx.WriteString(question)
	ctx.WriteString("\n")
	ctx.WriteString("당신의 선택은? : ")
	if !enginecmd.SetPendingLineHandler(ctx, s.handleChoice) {
		return enginecmd.StatusDefault, fmt.Errorf("투표 입력 상태를 계속할 수 없습니다")
	}
	return enginecmd.StatusDoPrompt, nil
}

func (s *voteSessionState) finish(ctx *enginecmd.Context) (enginecmd.Status, error) {
	enginecmd.ClearPendingLineHandler(ctx)
	choice := string(s.choices)
	if s.memory != nil {
		s.memory.recordChoice(s.actorID, choice)
	}
	if path := s.voteFilePath(); path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return enginecmd.StatusDefault, err
		}
		if err := os.WriteFile(path, []byte(choice), 0644); err != nil {
			return enginecmd.StatusDefault, err
		}
	}
	ctx.WriteString("투표를 하였습니다.\n")
	return enginecmd.StatusDefault, nil
}

func (s *voteSessionState) existingVote() bool {
	if path := s.voteFilePath(); path != "" {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	_, ok := s.memory.Choice(s.actorID)
	return ok
}

func (s *voteSessionState) deleteExistingVote() error {
	s.memory.clearChoice(s.actorID)
	if path := s.voteFilePath(); path != "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	s.choices = nil
	return nil
}

func (s *voteSessionState) voteFilePath() string {
	if s == nil || strings.TrimSpace(s.root) == "" || strings.TrimSpace(s.fileName) == "" {
		return ""
	}
	return filepath.Join(s.root, "player", "vote", s.fileName+"_v")
}

func (i voteIssueSnapshot) voteQuestionLimit() int {
	limit := i.questionCount
	if limit <= 0 || limit > 79 {
		limit = 79
	}
	if len(i.candidates) < limit {
		limit = len(i.candidates)
	}
	if limit < 0 {
		return 0
	}
	return limit
}

func (i voteIssueSnapshot) voteQuestion(index int) (string, bool) {
	if index < 0 || index >= i.voteQuestionLimit() {
		return "", false
	}
	return i.candidates[index], true
}

func voteActorState(world VoteWorld, playerID model.PlayerID) (model.Player, model.Creature, model.Room, error) {
	if world == nil {
		return model.Player{}, model.Creature{}, model.Room{}, fmt.Errorf("vote: world is nil")
	}
	player, ok := world.Player(playerID)
	if !ok {
		return model.Player{}, model.Creature{}, model.Room{}, fmt.Errorf("vote: player %q not found", playerID)
	}
	if player.CreatureID.IsZero() {
		return player, model.Creature{}, model.Room{}, fmt.Errorf("vote: player %q has no creature", playerID)
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return player, model.Creature{}, model.Room{}, fmt.Errorf("vote: creature %q not found", player.CreatureID)
	}
	roomID := player.RoomID
	if roomID.IsZero() {
		roomID = creature.RoomID
	}
	if roomID.IsZero() {
		return player, creature, model.Room{}, fmt.Errorf("vote: player %q has no room", playerID)
	}
	room, ok := world.Room(roomID)
	if !ok {
		return player, creature, model.Room{}, fmt.Errorf("vote: room %q not found", roomID)
	}
	return player, creature, room, nil
}

func voteActorEligible(creature model.Creature) bool {
	if voteCreatureStat(creature, "class") >= legacyVoteInvincibleClass {
		return true
	}
	if interval, ok := voteCreatureStatAny(creature, "legacyHoursInterval", "LT_HOURS_interval", "lastHoursInterval"); ok {
		return 18+interval/86400 >= 21
	}
	if age, ok := voteCreatureStatAny(creature, "age", "legacyAge", "legacyAgeYears"); ok {
		return age >= 21
	}
	if days, ok := voteCreatureStatAny(creature, "playDays", "daysPlayed", "legacyPlayedDays"); ok {
		return 18+days >= 21
	}
	return true
}

func voteRoomEnabled(room model.Room) bool {
	names := []string{"RELECT", "election", "vote", "voting"}
	if voteHasAnyTag(room.Metadata.Tags, names...) {
		return true
	}
	targets := voteTagSet(names...)
	for key, value := range room.Properties {
		if _, ok := targets[voteNormalizeTag(key)]; ok && votePropertyFlagEnabled(value) {
			return true
		}
		for _, token := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ';' || r == '|' || r == ' '
		}) {
			if _, ok := targets[voteNormalizeTag(token)]; ok {
				return true
			}
		}
	}
	return false
}

func voteChoiceFromArg(arg string, candidateCount int) (string, bool) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", false
	}
	r, _ := utf8FirstRune(arg)
	r = unicode.ToUpper(r)
	if r < 'A' || r > 'G' {
		return "", false
	}
	if candidateCount > 0 && int(r-'A') >= candidateCount {
		return "", false
	}
	return string(r), true
}

func renderVoteUsage(issue voteIssueSnapshot) string {
	var b strings.Builder
	if issue.title != "" {
		b.WriteString("투표 안건: ")
		b.WriteString(issue.title)
		b.WriteByte('\n')
	}
	b.WriteString("당신의 선택은? : 투표 <선택>\n")
	b.WriteString("선택: ")
	for i, candidate := range issue.candidates {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteByte(byte('A' + i))
		if candidate != "" {
			b.WriteString("=")
			b.WriteString(candidate)
		}
	}
	b.WriteByte('\n')
	return b.String()
}

func cleanVoteCandidates(candidates []string) []string {
	cleaned := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		cleaned = append(cleaned, candidate)
		if len(cleaned) == 7 {
			break
		}
	}
	return cleaned
}

func voteCreatureStatAny(creature model.Creature, names ...string) (int, bool) {
	for _, name := range names {
		if creature.Stats != nil {
			if value, ok := creature.Stats[name]; ok {
				return value, true
			}
		}
		if creature.Properties != nil {
			if raw, ok := creature.Properties[name]; ok {
				var value int
				if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &value); err == nil {
					return value, true
				}
			}
		}
	}
	targets := voteTagSet(names...)
	for key, value := range creature.Stats {
		if _, ok := targets[voteNormalizeTag(key)]; ok {
			return value, true
		}
	}
	for key, raw := range creature.Properties {
		if _, ok := targets[voteNormalizeTag(key)]; ok {
			var value int
			if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &value); err == nil {
				return value, true
			}
		}
	}
	return 0, false
}

func voteCreatureStat(creature model.Creature, name string) int {
	value, _ := voteCreatureStatAny(creature, name)
	return value
}

func voteHasAnyTag(tags []string, names ...string) bool {
	targets := voteTagSet(names...)
	for _, tag := range tags {
		if _, ok := targets[voteNormalizeTag(tag)]; ok {
			return true
		}
	}
	return false
}

func voteTagSet(names ...string) map[string]struct{} {
	targets := make(map[string]struct{}, len(names))
	for _, name := range names {
		if normalized := voteNormalizeTag(name); normalized != "" {
			targets[normalized] = struct{}{}
		}
	}
	return targets
}

func votePropertyFlagEnabled(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	return value != "" && value != "0" && value != "false" && value != "no"
}

func voteNormalizeTag(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	tag = strings.ReplaceAll(tag, "-", "")
	tag = strings.ReplaceAll(tag, "_", "")
	tag = strings.ReplaceAll(tag, " ", "")
	return tag
}

func utf8FirstRune(value string) (rune, bool) {
	for _, r := range value {
		return r, true
	}
	return 0, false
}
