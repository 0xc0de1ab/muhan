package game

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	enginecmd "github.com/0xc0de1ab/muhan/internal/engine/command"
	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

var memoFileMu sync.Mutex

type MemoWorld interface {
	Player(model.PlayerID) (model.Player, bool)
}

type memoPlayerListWorld interface {
	Players() []model.Player
}

type memoRequest struct {
	targetArg   string
	text        string
	commandText string
}

func NewMemoHandler(world MemoWorld, root string) enginecmd.Handler {
	return newMemoHandler(world, root, time.Now)
}

func newMemoHandler(world MemoWorld, root string, now func() time.Time) enginecmd.Handler {
	if now == nil {
		now = time.Now
	}
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}

		request, ok := memoRequestFromResolved(resolved)
		if !ok {
			ctx.WriteString("누구에게 어떤 메모를 남기시려고요?\n")
			return enginecmd.StatusDefault, nil
		}

		target, ok := memoLookupPlayer(world, request.targetArg)
		if !ok {
			ctx.WriteString("그런 사용자는 없습니다.\n")
			return enginecmd.StatusDefault, nil
		}
		return saveTargetMemo(ctx, world, root, target, request.targetArg, now(), request.text, request.commandText)
	}
}

func memoRequestFromResolved(resolved enginecmd.ResolvedCommand) (memoRequest, bool) {
	args := trimmedMemoArgs(resolved.Args)
	if len(args) < 2 {
		return memoRequest{}, false
	}

	fallback := memoRequest{
		targetArg:   args[0],
		text:        strings.Join(args[1:], " "),
		commandText: args[0] + " " + strings.Join(args[1:], " "),
	}
	input := strings.TrimSpace(resolved.Input)
	if input == "" {
		return fallback, true
	}

	for _, command := range socialCommandNameCandidates(resolved) {
		if stripped, ok := stripSocialCommandAtTextEdge(input, command); ok {
			target, text, ok := legacyMemoTargetAndText(stripped)
			if !ok {
				return memoRequest{}, false
			}
			return memoRequest{
				targetArg:   target,
				text:        text,
				commandText: stripped,
			}, true
		}
	}
	return fallback, true
}

func legacyMemoTargetAndText(text string) (target string, memoText string, ok bool) {
	i := 0
	for i < len(text) && text[i] == ' ' {
		i++
	}
	start := i
	for i < len(text) && text[i] != ' ' {
		i++
	}
	if start == i {
		return "", "", false
	}
	target = text[start:i]
	for i < len(text) && text[i] == ' ' {
		if i+1 >= len(text) || text[i+1] != ' ' {
			if i+1 >= len(text) {
				return target, "", true
			}
			return target, text[i+1:], true
		}
		i++
	}
	return target, "", true
}

func saveTargetMemo(ctx *enginecmd.Context, world MemoWorld, root string, target model.Player, targetArg string, at time.Time, text string, commandText string) (enginecmd.Status, error) {
	line := strings.TrimRight(text, "\r\n")
	if memoLegacyCommandLength(commandText) > 80 {
		ctx.WriteString("메모의 내용이 너무 깁니다.")
		return enginecmd.StatusDefault, nil
	}
	if strings.TrimSpace(line) == "" {
		ctx.WriteString("무슨 말을 남기시려고요?")
		return enginecmd.StatusDefault, nil
	}
	if err := appendLegacyTargetMemo(ctx, world, root, target, at, memoActorDisplayName(world, ctx.ActorID), line); err != nil {
		return enginecmd.StatusDefault, err
	}
	ctx.WriteString("메모를 남겼습니다.")
	return enginecmd.StatusDefault, nil
}

func appendLegacyTargetMemo(ctx *enginecmd.Context, world MemoWorld, root string, target model.Player, at time.Time, author string, line string) error {
	path, ok := memoWritableFilePath(root, target)
	if !ok {
		return fmt.Errorf("메모 파일 이름을 만들 수 없습니다")
	}
	if author = strings.TrimSpace(author); author == "" {
		author = strings.TrimSpace(ctx.ActorID)
	}

	memoFileMu.Lock()
	defer memoFileMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("메모함을 만들 수 없습니다: %w", err)
	}
	if err := ensureMemoWritableRegular(path); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("메모를 쓸 수 없습니다: %w", err)
	}
	defer file.Close()
	if _, err := file.WriteString(formatLegacyTargetMemoEntry(at, author, line)); err != nil {
		return fmt.Errorf("메모를 쓸 수 없습니다: %w", err)
	}
	return nil
}

func formatLegacyTargetMemoEntry(at time.Time, author string, line string) string {
	return formatMemoLegacyCTime(at) + fmt.Sprintf(" 에 [%s] 님이 남기신 메모 : \n", strings.TrimSpace(author)) + ">>>>> " + strings.TrimRight(line, "\r\n") + "\n"
}

func formatMemoLegacyCTime(at time.Time) string {
	return at.Local().Format("Mon Jan _2 15:04:05 2006")
}

func memoLegacyCommandLength(commandText string) int {
	encoded, err := legacykr.EncodeEUCKR(commandText)
	if err != nil {
		return len(commandText)
	}
	return len(encoded)
}

func ensureMemoWritableRegular(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("메모를 쓸 수 없습니다: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("메모를 쓸 수 없습니다: 일반 파일이 아닙니다")
	}
	return nil
}

func memoWritableFilePath(root string, target model.Player) (string, bool) {
	for _, name := range memoPlayerFileNameCandidates(target, string(target.ID)) {
		if path, ok := safeMemoPath(root, name); ok {
			return path, true
		}
	}
	return "", false
}

func memoPlayerFileNameCandidates(player model.Player, fallback string) []string {
	names := []string{}
	if raw := player.Metadata.RawFields["filename"]; len(raw) != 0 {
		names = append(names, string(raw))
	}
	if display := strings.TrimSpace(player.DisplayName); display != "" {
		names = append(names, display)
	}
	if id := strings.TrimSpace(string(player.ID)); id != "" {
		names = append(names, id)
	}
	names = append(names, strings.TrimSpace(fallback))
	return uniqueMemoNames(names)
}

func memoActorDisplayName(world MemoWorld, actorID string) string {
	if world != nil {
		if player, ok := world.Player(model.PlayerID(actorID)); ok {
			if display := strings.TrimSpace(player.DisplayName); display != "" {
				return display
			}
		}
	}
	return strings.TrimSpace(actorID)
}

func memoLookupPlayer(world MemoWorld, name string) (model.Player, bool) {
	if world == nil {
		return model.Player{}, false
	}
	for _, candidate := range memoPlayerLookupIDCandidates(name) {
		if player, ok := world.Player(model.PlayerID(candidate)); ok {
			return player, true
		}
	}
	listWorld, ok := world.(memoPlayerListWorld)
	if !ok {
		return model.Player{}, false
	}
	wanted := memoLookupNameSet(name)
	for _, player := range listWorld.Players() {
		for _, candidate := range memoPlayerFileNameCandidates(player, string(player.ID)) {
			if _, ok := wanted[candidate]; ok {
				return player, true
			}
		}
	}
	return model.Player{}, false
}

func memoPlayerLookupIDCandidates(name string) []string {
	names := uniqueMemoNames([]string{name, legacyMemoLookupName(name)})
	candidates := make([]string, 0, len(names)*2)
	candidates = append(candidates, names...)
	for _, name := range names {
		candidates = append(candidates, "player:"+name)
	}
	return uniqueMemoNames(candidates)
}

func memoLookupNameSet(name string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, candidate := range uniqueMemoNames([]string{name, legacyMemoLookupName(name)}) {
		set[candidate] = struct{}{}
	}
	return set
}

func legacyMemoLookupName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	bytes := []byte(name)
	if bytes[0] >= 'a' && bytes[0] <= 'z' {
		bytes[0] -= 'a' - 'A'
	}
	return string(bytes)
}

func safeMemoPath(root string, name string) (string, bool) {
	if !safeMemoFileName(name) {
		return "", false
	}
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	memoRoot := filepath.Clean(filepath.Join(root, "player", "fal"))
	path := filepath.Clean(filepath.Join(memoRoot, name))
	rel, err := filepath.Rel(memoRoot, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", false
	}
	return path, true
}

func safeMemoFileName(name string) bool {
	if name == "" || strings.TrimSpace(name) != name {
		return false
	}
	if name == "." || name == ".." || filepath.IsAbs(name) || strings.ContainsAny(name, `/\`) {
		return false
	}
	for _, r := range name {
		if r == 0 || r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func uniqueMemoNames(names []string) []string {
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

func trimmedMemoArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			out = append(out, arg)
		}
	}
	return out
}
