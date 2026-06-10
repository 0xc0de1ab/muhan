package command

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"muhan/internal/persist/legacycrypt"
	"muhan/internal/persist/legacykr"
	"muhan/internal/world/model"
)

const (
	legacyPasswordHashProperty = "legacyPasswordHash"
	legacyAliasMaxCount        = 100
	legacyAliasNameMaxBytes    = 30
	legacyAliasInputMaxBytes   = 200
)

var (
	ErrAccountWorldRequired    = errors.New("account world required")
	ErrAccountActorRequired    = errors.New("account actor required")
	ErrAccountPlayerNotFound   = errors.New("account player not found")
	ErrAccountCreatureNotFound = errors.New("account creature not found")
	ErrAccountCreatureRequired = errors.New("account creature required")
	ErrAliasStoreRequired      = errors.New("alias store required")
)

type AccountWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
}

type PasswordWorld interface {
	AccountWorld
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
}

type PasswordSink interface {
	SavePassword(ctx *Context, playerID model.PlayerID, hash string) error
}

type PasswordSinkFunc func(ctx *Context, playerID model.PlayerID, hash string) error

func (f PasswordSinkFunc) SavePassword(ctx *Context, playerID model.PlayerID, hash string) error {
	if f == nil {
		return nil
	}
	return f(ctx, playerID, hash)
}

type PasswdOption func(*passwdConfig)

func WithPasswordSink(sink PasswordSink) PasswdOption {
	return func(cfg *passwdConfig) {
		cfg.sink = sink
	}
}

type passwdConfig struct {
	sink PasswordSink
}

func NewPasswdHandler(world PasswordWorld, options ...PasswdOption) Handler {
	cfg := passwdConfig{}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}

	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		playerID := accountPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrAccountActorRequired
		}
		player, creature, err := accountCurrentCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		hash := accountLegacyPasswordHash(creature)
		if strings.TrimSpace(hash) == "" {
			ctx.WriteString("저장된 암호 정보를 찾을 수 없습니다.\n")
			return StatusDefault, nil
		}

		state := &passwdState{
			world:    world,
			sink:     cfg.sink,
			playerID: player.ID,
			creature: creature,
			hash:     hash,
		}
		ctx.WriteString("현재 암호를 입력하십시요: ")
		if !SetPendingLineHandler(ctx, state.currentPassword) {
			return StatusDefault, fmt.Errorf("암호 변경 상태를 시작할 수 없습니다")
		}
		return StatusDoPrompt, nil
	}
}

type passwdState struct {
	world       PasswordWorld
	sink        PasswordSink
	playerID    model.PlayerID
	creature    model.Creature
	hash        string
	newPassword string
}

func (s *passwdState) currentPassword(ctx *Context, line string) (Status, error) {
	if !legacycrypt.Verify(line, s.hash) {
		ClearPendingLineHandler(ctx)
		ctx.WriteString("암호가 틀렸습니다.\n")
		ctx.WriteString("암호가 변경되지 않았습니다.\n")
		return StatusDefault, nil
	}
	// Re-hash legacy DES password to bcrypt on successful verification.
	if !legacycrypt.IsBcryptHash(s.hash) {
		accountRehashBcrypt(s.world, s.sink, s.creature.ID, s.playerID, line)
	}
	ctx.WriteString("\n새 암호를 입력하십시요: ")
	if !SetPendingLineHandler(ctx, s.newPasswordLine) {
		return StatusDefault, fmt.Errorf("암호 변경 상태를 계속할 수 없습니다")
	}
	return StatusDoPrompt, nil
}

func (s *passwdState) newPasswordLine(ctx *Context, line string) (Status, error) {
	passwordLen := legacyByteLen(line)
	switch {
	case passwordLen < 3:
		ClearPendingLineHandler(ctx)
		ctx.WriteString("암호가 너무 짧습니다.\n")
		ctx.WriteString("암호가 변경되지 않았습니다.\n")
		return StatusDefault, nil
	case passwordLen > 14:
		ClearPendingLineHandler(ctx)
		ctx.WriteString("암호가 너무 깁니다.\n")
		ctx.WriteString("암호가 변경되지 않았습니다..\n")
		return StatusDefault, nil
	}

	s.newPassword = line
	ctx.WriteString("\n새 암호를 다시 넣으십시요: ")
	if !SetPendingLineHandler(ctx, s.confirmPasswordLine) {
		return StatusDefault, fmt.Errorf("암호 변경 상태를 계속할 수 없습니다")
	}
	return StatusDoPrompt, nil
}

func (s *passwdState) confirmPasswordLine(ctx *Context, line string) (Status, error) {
	ClearPendingLineHandler(ctx)
	if s.newPassword != line {
		ctx.WriteString("암호가 서로 틀립니다.\n")
		ctx.WriteString("암호가 변경되지 않았습니다.\n")
		return StatusDefault, nil
	}

	hash, err := legacycrypt.HashBcrypt(line)
	if err != nil {
		ctx.WriteString("암호 변경 기능을 사용할 수 없습니다.\n")
		ctx.WriteString("암호가 변경되지 않았습니다.\n")
		return StatusDefault, nil
	}
	if _, err := s.world.SetCreatureProperty(s.creature.ID, legacyPasswordHashProperty, hash); err != nil {
		return StatusDefault, err
	}
	if s.sink != nil {
		if err := s.sink.SavePassword(ctx, s.playerID, hash); err != nil {
			return StatusDefault, err
		}
	} else {
		accountQueuePlayerSave(s.world, s.playerID)
	}

	ctx.WriteString("암호가 변경되었습니다.\n")
	return StatusDefault, nil
}

type PlayerAlias struct {
	Alias   string
	Process string
}

type AliasStore interface {
	ListAliases(playerID model.PlayerID) ([]PlayerAlias, error)
	SaveAliases(playerID model.PlayerID, aliases []PlayerAlias) error
}

type AliasCleanupStore interface {
	AliasStore
	DeleteAliases(playerID model.PlayerID) error
}

type MemoryAliasStore struct {
	mu      sync.RWMutex
	aliases map[model.PlayerID][]PlayerAlias
}

func NewMemoryAliasStore() *MemoryAliasStore {
	return &MemoryAliasStore{
		aliases: map[model.PlayerID][]PlayerAlias{},
	}
}

func (s *MemoryAliasStore) ListAliases(playerID model.PlayerID) ([]PlayerAlias, error) {
	if s == nil {
		return nil, ErrAliasStoreRequired
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.aliases[playerID]), nil
}

func (s *MemoryAliasStore) SaveAliases(playerID model.PlayerID, aliases []PlayerAlias) error {
	if s == nil {
		return ErrAliasStoreRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aliases[playerID] = cloneAliases(aliases)
	return nil
}

func (s *MemoryAliasStore) DeleteAliases(playerID model.PlayerID) error {
	if s == nil {
		return ErrAliasStoreRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.aliases, playerID)
	return nil
}

type FileAliasStore struct {
	root  string
	world AccountWorld
	mu    sync.Mutex
}

func NewFileAliasStore(root string, world AccountWorld) *FileAliasStore {
	return &FileAliasStore{
		root:  root,
		world: world,
	}
}

func (s *FileAliasStore) ListAliases(playerID model.PlayerID) ([]PlayerAlias, error) {
	if s == nil {
		return nil, ErrAliasStoreRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.read(playerID)
	if err != nil {
		return nil, err
	}
	return cloneAliases(record.aliases), nil
}

func (s *FileAliasStore) SaveAliases(playerID model.PlayerID, aliases []PlayerAlias) error {
	if s == nil {
		return ErrAliasStoreRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.read(playerID)
	if err != nil {
		return err
	}
	record.aliases = cloneAliases(aliases)
	return s.write(playerID, record)
}

func (s *FileAliasStore) DeleteAliases(playerID model.PlayerID) error {
	if s == nil {
		return ErrAliasStoreRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path(playerID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

type fileAliasRecord struct {
	aliases []PlayerAlias
	title   string
}

func (s *FileAliasStore) read(playerID model.PlayerID) (fileAliasRecord, error) {
	path := s.path(playerID)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileAliasRecord{}, nil
	}
	if err != nil {
		return fileAliasRecord{}, err
	}
	text, err := legacykr.ValidUTF8OrDecodeContext(legacykr.Context{Path: path, Field: "alias"}, data)
	if err != nil {
		return fileAliasRecord{}, err
	}
	return parseFileAliasRecord(text), nil
}

func (s *FileAliasStore) write(playerID model.PlayerID, record fileAliasRecord) error {
	path := s.path(playerID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	var b strings.Builder
	for _, alias := range record.aliases {
		b.WriteString(alias.Alias)
		b.WriteByte('\n')
		b.WriteString(alias.Process)
		b.WriteByte('\n')
	}
	b.WriteString("~!\n")
	if record.title != "" {
		b.WriteString(record.title)
		b.WriteByte('\n')
	}
	b.WriteString("~!\n")

	// New writes are canonical UTF-8. Existing legacy EUC-KR/CP949 files remain
	// readable through read(), so old alias/title data is upgraded on the next save.
	return os.WriteFile(path, []byte(b.String()), 0o660)
}

func (s *FileAliasStore) path(playerID model.PlayerID) string {
	name := accountPlayerFileName(s.world, playerID)
	return filepath.Join(s.root, "player", "alias", name)
}

func parseFileAliasRecord(text string) fileAliasRecord {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	record := fileAliasRecord{}
	i := 0
	for i < len(lines) {
		alias := strings.TrimRight(lines[i], "\r")
		i++
		if alias == "" || alias == "~!" {
			break
		}
		if i >= len(lines) {
			break
		}
		process := strings.TrimRight(lines[i], "\r")
		i++
		if process == "~!" {
			break
		}
		if len(record.aliases) < legacyAliasMaxCount {
			record.aliases = append(record.aliases, PlayerAlias{Alias: alias, Process: process})
		}
	}
	if i < len(lines) {
		title := strings.TrimRight(lines[i], "\r")
		if title != "" && title != "~!" {
			record.title = title
		}
	}
	return record
}

func NewPlyAliasesHandler(store AliasStore) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := accountPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrAccountActorRequired
		}
		if store == nil {
			store = NewMemoryAliasStore()
		}

		aliases, err := store.ListAliases(playerID)
		if err != nil {
			return StatusDefault, err
		}

		if len(resolved.Args) == 0 {
			ctx.WriteString(renderAliasList(aliases))
			return StatusDefault, nil
		}

		aliasName := legacyAliasNameArg(resolved.Args[0])
		if len(resolved.Args) == 1 {
			for i, alias := range aliases {
				if alias.Alias != aliasName {
					continue
				}
				aliases = append(aliases[:i], aliases[i+1:]...)
				if err := store.SaveAliases(playerID, aliases); err != nil {
					return StatusDefault, err
				}
				ctx.WriteString("줄임말이 삭제되었습니다.")
				return StatusDefault, nil
			}
			ctx.WriteString("그 이름으로 설정된 줄임말이 없습니다.")
			return StatusDefault, nil
		}

		if aliasName == "~!" {
			ctx.WriteString("그 코드로는 줄임말로 설정할 수 없습니다.")
			return StatusDefault, nil
		}
		if legacyAliasInputByteLen(aliasName) > legacyAliasNameMaxBytes {
			ctx.WriteString("줄임말의 길이가 너무 깁니다.")
			return StatusDefault, nil
		}
		for _, alias := range aliases {
			if alias.Alias == aliasName {
				ctx.WriteString("그 줄임말은 이미 설정되어 있습니다.")
				return StatusDefault, nil
			}
		}
		if legacyAliasInputByteLen(resolved.Input) > legacyAliasInputMaxBytes {
			ctx.WriteString("줄임말의 전체적 길이가 너무 깁니다.")
			return StatusDefault, nil
		}
		if len(aliases) >= legacyAliasMaxCount {
			ctx.WriteString("더이상 줄임말을 설정할 수 없습니다.")
			return StatusDefault, nil
		}

		process := legacyAliasProcess(resolved)
		aliases = append(aliases, PlayerAlias{Alias: aliasName, Process: process})
		if err := store.SaveAliases(playerID, aliases); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString("줄임말이 설정되었습니다.")
		return StatusDefault, nil
	}
}

func renderAliasList(aliases []PlayerAlias) string {
	if len(aliases) == 0 {
		return "설정된 줄임말이 없습니다."
	}
	var b strings.Builder
	b.WriteString("줄임말:\n")
	for i, alias := range aliases {
		fmt.Fprintf(&b, "[%2d]  %-14s: %s\n", i+1, alias.Alias, alias.Process)
	}
	fmt.Fprintf(&b, "\n< %d / 100 >개의 줄임말이 있습니다.\n", len(aliases))
	return b.String()
}

func legacyAliasNameArg(arg string) string {
	if trimmed, ok := strings.CutPrefix(arg, "숫자"); ok && trimmed != "" {
		return legacyLowerASCII(trimmed)
	}
	return legacyLowerASCII(arg)
}

func legacyAliasProcess(resolved ResolvedCommand) string {
	input := strings.TrimSpace(resolved.Input)
	if input != "" {
		for _, command := range dmCommandNameCandidates(resolved) {
			if stripped, ok := stripCommandAtTextEdge(input, command); ok {
				if process, ok := legacyAliasProcessAfterName(stripped); ok {
					return process
				}
			}
		}
	}
	return strings.TrimSpace(strings.Join(resolved.Args[1:], " "))
}

func legacyAliasProcessAfterName(text string) (string, bool) {
	i := strings.IndexByte(text, ' ')
	if i < 0 {
		return "", false
	}
	return strings.TrimRight(text[i+1:], " "), true
}

func legacyAliasInputByteLen(input string) int {
	encoded, err := legacykr.EncodeEUCKR(input)
	if err != nil {
		return len([]byte(input))
	}
	return len(encoded)
}

type SuicideSink interface {
	RequestSuicide(ctx *Context, playerID model.PlayerID) error
}

type SuicideSinkFunc func(ctx *Context, playerID model.PlayerID) error

func (f SuicideSinkFunc) RequestSuicide(ctx *Context, playerID model.PlayerID) error {
	if f == nil {
		return nil
	}
	return f(ctx, playerID)
}

type SuicideOption func(*suicideConfig)

func WithSuicideSink(sink SuicideSink) SuicideOption {
	return func(cfg *suicideConfig) {
		cfg.sink = sink
	}
}

func WithSuicideAliasStore(store AliasStore) SuicideOption {
	return func(cfg *suicideConfig) {
		cfg.aliasStore = store
	}
}

type suicideConfig struct {
	sink       SuicideSink
	aliasStore AliasStore
}

func NewPlySuicideHandler(world AccountWorld, options ...SuicideOption) Handler {
	cfg := suicideConfig{}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}

	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		playerID := accountPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrAccountActorRequired
		}
		player, creature, err := accountCurrentCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}
		if creatureClass(creature) < legacyClassInvincible && accountCreatureLevel(creature) < 6 {
			ctx.WriteString("레벨 5이하는 자살 할 수 없습니다.\n")
			return StatusDefault, nil
		}
		hash := accountLegacyPasswordHash(creature)
		if strings.TrimSpace(hash) == "" {
			ctx.WriteString("저장된 암호 정보를 찾을 수 없습니다.\n")
			return StatusDefault, nil
		}

		state := &suicideState{
			sink:       cfg.sink,
			aliasStore: cfg.aliasStore,
			playerID:   player.ID,
			creatureID: creature.ID,
			hash:       hash,
		}
		// Pass world for bcrypt re-hash if it supports SetCreatureProperty.
		if pw, ok := world.(PasswordWorld); ok {
			state.world = pw
		}
		ctx.WriteString("당신에 관한 데이터를 완전히 삭제합니다.\n")
		ctx.WriteString("당신의 현재 암호를 넣어주십시요 : ")
		if !SetPendingLineHandler(ctx, state.passwordLine) {
			return StatusDefault, fmt.Errorf("자살 확인 상태를 시작할 수 없습니다")
		}
		return StatusDoPrompt, nil
	}
}

type suicideState struct {
	world      PasswordWorld
	sink       SuicideSink
	aliasStore AliasStore
	playerID   model.PlayerID
	creatureID model.CreatureID
	hash       string
}

func (s *suicideState) passwordLine(ctx *Context, line string) (Status, error) {
	if !legacycrypt.Verify(line, s.hash) {
		ClearPendingLineHandler(ctx)
		ctx.WriteString("암호가 틀립니다.\n삭제되지 않았습니다.")
		return StatusDefault, nil
	}
	// Re-hash legacy DES password to bcrypt on successful verification.
	if !legacycrypt.IsBcryptHash(s.hash) {
		accountRehashBcrypt(s.world, nil, s.creatureID, s.playerID, line)
	}
	ctx.WriteString("찐짜로? (찐짜로/뻥으로)")
	if !SetPendingLineHandler(ctx, s.confirmLine) {
		return StatusDefault, fmt.Errorf("자살 확인 상태를 계속할 수 없습니다")
	}
	return StatusDoPrompt, nil
}

func (s *suicideState) confirmLine(ctx *Context, line string) (Status, error) {
	ClearPendingLineHandler(ctx)
	if line != "찐짜로" {
		ctx.WriteString("삭제되지 않았습니다.")
		return StatusDefault, nil
	}
	if s.sink == nil {
		ctx.WriteString("자살 신청은 현재 지원하지 않습니다.\n삭제되지 않았습니다.")
		return StatusDefault, nil
	}
	if err := s.sink.RequestSuicide(ctx, s.playerID); err != nil {
		return StatusDefault, err
	}
	if err := cleanupSuicideAliases(s.aliasStore, s.playerID); err != nil {
		return StatusDefault, err
	}
	return StatusDisconnect, nil
}

func cleanupSuicideAliases(store AliasStore, playerID model.PlayerID) error {
	if store == nil {
		return nil
	}
	if cleanup, ok := store.(interface {
		DeleteAliases(model.PlayerID) error
	}); ok {
		return cleanup.DeleteAliases(playerID)
	}
	return store.SaveAliases(playerID, nil)
}

func accountPlayerIDFromContext(ctx *Context) model.PlayerID {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" {
		return ""
	}
	return model.PlayerID(ctx.ActorID)
}

func accountCurrentCreature(world AccountWorld, playerID model.PlayerID) (model.Player, model.Creature, error) {
	if world == nil {
		return model.Player{}, model.Creature{}, ErrAccountWorldRequired
	}
	player, ok := world.Player(playerID)
	if !ok {
		return model.Player{}, model.Creature{}, fmt.Errorf("%w: %q", ErrAccountPlayerNotFound, playerID)
	}
	if player.CreatureID.IsZero() {
		return player, model.Creature{}, fmt.Errorf("%w: player %q", ErrAccountCreatureRequired, playerID)
	}
	creature, ok := world.Creature(player.CreatureID)
	if !ok {
		return player, model.Creature{}, fmt.Errorf("%w: %q", ErrAccountCreatureNotFound, player.CreatureID)
	}
	return player, creature, nil
}

func accountLegacyPasswordHash(creature model.Creature) string {
	if hash := strings.TrimRight(strings.TrimSpace(creature.Properties[legacyPasswordHashProperty]), "\x00"); hash != "" {
		return hash
	}
	if hash := strings.TrimRight(strings.TrimSpace(creature.Properties["legacyPassword"]), "\x00"); hash != "" {
		return hash
	}
	if raw := creature.Metadata.RawFields["creature.password"]; len(raw) != 0 {
		return strings.TrimRight(strings.TrimSpace(string(raw)), "\x00")
	}
	if raw := creature.Metadata.RawFields["password"]; len(raw) != 0 {
		return strings.TrimRight(strings.TrimSpace(string(raw)), "\x00")
	}
	return ""
}

func accountCreatureLevel(creature model.Creature) int {
	if creature.Stats != nil {
		if level, ok := creature.Stats["level"]; ok {
			return level
		}
	}
	if creature.Level != 0 {
		return creature.Level
	}
	if level, ok := creatureStatValue(creature, "level"); ok {
		return level
	}
	return 0
}

func accountPlayerFileName(world AccountWorld, playerID model.PlayerID) string {
	if world != nil {
		if player, ok := world.Player(playerID); ok {
			if name := strings.TrimSpace(player.DisplayName); name != "" {
				return name
			}
			if name := strings.TrimSpace(player.AccountName); name != "" {
				return name
			}
		}
	}
	name := strings.TrimSpace(string(playerID))
	return strings.TrimPrefix(name, "player:")
}

func accountQueuePlayerSave(world any, playerID model.PlayerID) {
	if playerID.IsZero() {
		return
	}
	if saver, ok := world.(interface {
		MarkPlayerDirty(model.PlayerID)
		QueueSave(model.PlayerID, model.BankID)
	}); ok {
		saver.MarkPlayerDirty(playerID)
		saver.QueueSave(playerID, "")
	}
}

func cloneAliases(aliases []PlayerAlias) []PlayerAlias {
	if len(aliases) == 0 {
		return nil
	}
	return slices.Clone(aliases)
}

// accountRehashBcrypt transparently upgrades a legacy DES password hash to
// bcrypt. It is called after a successful DES password verification. Errors
// are silently ignored so that a re-hash failure never blocks the user.
func accountRehashBcrypt(world PasswordWorld, sink PasswordSink, creatureID model.CreatureID, playerID model.PlayerID, password string) {
	if world == nil {
		return
	}
	newHash, err := legacycrypt.HashBcrypt(password)
	if err != nil {
		return
	}
	if _, err := world.SetCreatureProperty(creatureID, legacyPasswordHashProperty, newHash); err != nil {
		return
	}
	if sink != nil {
		// Best-effort save through the provided sink; ignore errors.
		_ = sink.SavePassword(nil, playerID, newHash)
	} else {
		accountQueuePlayerSave(world, playerID)
	}
}
