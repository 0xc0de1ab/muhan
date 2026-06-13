package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

var postFileMu sync.Mutex

const (
	postReadPageBytes = 800
	postReadPageLines = 19

	postReadContinuePrompt = "\n[엔터]를 누르세요. 그만보시려면 [.]을 치세요: "
)

// NOTE (Package C): Board posts + family news now have JSON sidecar + dirty + QueueBoardSave wiring.
// This (mail/post office) persistence is out of C scope (use legacy or future package). Board wiring in board.go.

type PostWorld interface {
	LookWorld
}

func NewPostSendHandler(world PostWorld, root string) Handler {
	return newPostSendHandler(world, root, time.Now)
}

func newPostSendHandler(world PostWorld, root string, now func() time.Time) Handler {
	if now == nil {
		now = time.Now
	}
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		if !roomHasAnyFlag(room, "postOffice", "rposto") {
			ctx.WriteString("여기는 우체국이 아닙니다.\n")
			return StatusDefault, nil
		}
		targetName := legacyPostLookupName(postTargetName(resolved))
		if targetName == "" {
			ctx.WriteString("누구한테 편지를 보내시려구요?\n")
			return StatusDefault, nil
		}
		target, ok := world.Player(model.PlayerID(targetName))
		if !ok {
			ctx.WriteString("그런 사용자는 없습니다.\n")
			return StatusDefault, nil
		}
		targetPostPath, ok := postWritableFilePath(root, targetName, target)
		if !ok {
			ctx.WriteString("그런 사용자는 없습니다.\n")
			return StatusDefault, nil
		}
		senderName := postActorName(world, viewer)
		if senderName == "" {
			senderName = string(viewer.PlayerID)
		}
		edit := &postEditState{
			path:       targetPostPath,
			senderName: senderName,
			now:        now,
			first:      true,
		}
		ctx.WriteString("편지 내용을 입력하십시요. 문장처음에 [.]을 치시면 편지쓰기를 종료합니다.\n")
		ctx.WriteString("각 행은 80자를 넘길 수 없습니다.\n")
		ctx.WriteString("-: ")
		if !SetPendingLineHandler(ctx, edit.handleLine) {
			return StatusDefault, fmt.Errorf("편지쓰기 상태를 시작할 수 없습니다")
		}
		return StatusDoPrompt, nil
	}
}

func NewPostReadHandler(world PostWorld, root string) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		if !roomHasAnyFlag(room, "postOffice", "rposto") {
			ctx.WriteString("이곳은 우체국이 아닙니다.\n")
			return StatusDefault, nil
		}
		player, ok := world.Player(viewer.PlayerID)
		if !ok {
			return StatusDefault, fmt.Errorf("postread: player %q not found", viewer.PlayerID)
		}
		path, ok := postExistingFilePath(root, postActorName(world, viewer), player)
		if !ok {
			ctx.WriteString("받은 편지가 없습니다.\n")
			return StatusDefault, nil
		}
		text, err := readPostFile(path)
		if err != nil {
			return StatusDefault, err
		}
		read := &postReadState{text: text}
		return read.render(ctx)
	}
}

func NewPostDeleteHandler(world PostWorld, root string) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		if !roomHasAnyFlag(room, "postOffice", "rposto") {
			ctx.WriteString("이곳은 우체국이 아닙니다.\n")
			return StatusDefault, nil
		}
		player, ok := world.Player(viewer.PlayerID)
		if !ok {
			return StatusDefault, fmt.Errorf("postdelete: player %q not found", viewer.PlayerID)
		}
		if path, ok := postExistingFilePath(root, postActorName(world, viewer), player); ok {
			if err := removePostFile(path); err != nil {
				return StatusDefault, err
			}
		}
		ctx.WriteString("편지가 삭제되었습니다.\n")
		return StatusDefault, nil
	}
}

type postEditState struct {
	path       string
	senderName string
	now        func() time.Time
	first      bool
}

type postReadState struct {
	text string
	next int
}

func (s *postEditState) handleLine(ctx *Context, line string) (Status, error) {
	if strings.HasPrefix(line, ".") {
		ClearPendingLineHandler(ctx)
		ctx.WriteString("편지를 보냈습니다.\n")
		return StatusDefault, nil
	}
	if err := appendPostLine(s.path, s.senderName, s.now(), s.first, line); err != nil {
		return StatusDefault, err
	}
	s.first = false
	ctx.WriteString(": ")
	SetPendingLineHandler(ctx, s.handleLine)
	return StatusDoPrompt, nil
}

func (s *postReadState) handleLine(ctx *Context, line string) (Status, error) {
	if strings.HasPrefix(line, ".") {
		ClearPendingLineHandler(ctx)
		ctx.WriteString("중단합니다.\n")
		return StatusDefault, nil
	}
	return s.render(ctx)
}

func (s *postReadState) render(ctx *Context) (Status, error) {
	if s == nil || s.next >= len(s.text) {
		ClearPendingLineHandler(ctx)
		return StatusDefault, nil
	}

	page, next := postReadPage(s.text, s.next)
	s.next = next
	ctx.WriteString(page)
	if s.next < len(s.text) {
		ctx.WriteString(postReadContinuePrompt)
		if !SetPendingLineHandler(ctx, s.handleLine) {
			return StatusDefault, fmt.Errorf("편지읽기 상태를 시작할 수 없습니다")
		}
		return StatusDoPrompt, nil
	}

	ClearPendingLineHandler(ctx)
	if s.text != "" && !strings.HasSuffix(s.text, "\n") {
		ctx.WriteString("\n")
	}
	return StatusDefault, nil
}

func postReadPage(text string, start int) (string, int) {
	if start < 0 {
		start = 0
	}
	if start >= len(text) {
		return "", len(text)
	}

	pos := start
	bytes := 0
	lines := 0
	for pos < len(text) {
		r, size := utf8.DecodeRuneInString(text[pos:])
		if r == utf8.RuneError && size == 1 {
			size = 1
		}
		if bytes > 0 && bytes+size > postReadPageBytes {
			break
		}
		pos += size
		bytes += size
		if r == '\n' {
			lines++
			if lines >= postReadPageLines {
				break
			}
		}
	}
	if pos == start {
		_, size := utf8.DecodeRuneInString(text[pos:])
		if size <= 0 {
			size = 1
		}
		pos += size
	}
	return text[start:pos], pos
}

func appendPostLine(path string, sender string, at time.Time, first bool, line string) error {
	postFileMu.Lock()
	defer postFileMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("편지함을 만들 수 없습니다: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("편지를 쓸 수 없습니다: %w", err)
	}
	defer file.Close()

	if first {
		header := fmt.Sprintf("\n---\n%s (%s)님에게서의 편지:\n\n", sender, at.Local().Format("Mon Jan _2 15:04:05 2006"))
		if _, err := file.WriteString(header); err != nil {
			return fmt.Errorf("편지를 쓸 수 없습니다: %w", err)
		}
	}
	line = firstNBytes(line, 79)
	if _, err := file.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("편지를 쓸 수 없습니다: %w", err)
	}
	return nil
}

func readPostFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("편지를 읽을 수 없습니다: %w", err)
	}
	text, err := legacykr.ValidUTF8OrDecodeContext(legacykr.Context{Path: path, Field: "post"}, data)
	if err != nil {
		return "", fmt.Errorf("편지를 읽을 수 없습니다: %w", err)
	}
	return text, nil
}

func removePostFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("편지를 삭제할 수 없습니다: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("편지를 삭제할 수 없습니다: 일반 파일이 아닙니다")
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("편지를 삭제할 수 없습니다: %w", err)
	}
	return nil
}

func postTargetName(resolved ResolvedCommand) string {
	if len(resolved.Args) == 0 {
		return ""
	}
	return strings.TrimSpace(resolved.Args[0])
}

func legacyPostLookupName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	bytes := []byte(name)
	for i, b := range bytes {
		if b >= 'A' && b <= 'Z' {
			bytes[i] = b + ('a' - 'A')
		}
	}
	if bytes[0] >= 'a' && bytes[0] <= 'z' {
		bytes[0] -= 'a' - 'A'
	}
	return string(bytes)
}

func postActorName(world PostWorld, viewer LookViewer) string {
	if !viewer.PlayerID.IsZero() {
		if player, ok := world.Player(viewer.PlayerID); ok && strings.TrimSpace(player.DisplayName) != "" {
			return cleanDisplayText(player.DisplayName)
		}
	}
	if !viewer.CreatureID.IsZero() {
		if creature, ok := world.Creature(viewer.CreatureID); ok && strings.TrimSpace(creature.DisplayName) != "" {
			return cleanDisplayText(creature.DisplayName)
		}
	}
	return ""
}

func postWritableFilePath(root string, name string, player model.Player) (string, bool) {
	for _, candidate := range postSafeFileNameCandidates(name, player) {
		if path, ok := safePostPath(root, candidate); ok {
			return path, true
		}
	}
	return "", false
}

func postExistingFilePath(root string, name string, player model.Player) (string, bool) {
	for _, candidate := range postFileNameCandidates(name, player) {
		path, ok := safePostPath(root, candidate)
		if !ok {
			continue
		}
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return path, true
		}
	}
	return "", false
}

func postFileNameCandidates(name string, player model.Player) []string {
	names := postSafeFileNameCandidates(name, player)
	if raw := player.Metadata.RawFields["filename"]; len(raw) != 0 && safePostFileName(string(raw)) {
		names = append(names, string(raw))
	}
	for _, candidate := range postSafeFileNameCandidates(name, player) {
		if encoded, err := legacykr.EncodeEUCKR(candidate); err == nil && safePostFileName(string(encoded)) {
			names = append(names, string(encoded))
		}
	}
	return uniquePostNames(names)
}

func postSafeFileNameCandidates(name string, player model.Player) []string {
	names := []string{}
	if display := strings.TrimSpace(player.DisplayName); display != "" {
		names = append(names, display)
	}
	if id := strings.TrimSpace(string(player.ID)); id != "" {
		names = append(names, id)
	}
	if name = strings.TrimSpace(name); name != "" {
		names = append(names, name)
	}
	return uniquePostNames(names)
}

func uniquePostNames(names []string) []string {
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

func safePostPath(root string, name string) (string, bool) {
	if !safePostFileName(name) {
		return "", false
	}
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	postRoot := filepath.Clean(filepath.Join(root, "post"))
	path := filepath.Clean(filepath.Join(postRoot, name))
	rel, err := filepath.Rel(postRoot, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", false
	}
	return path, true
}

func safePostFileName(name string) bool {
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

func firstNBytes(s string, max int) string {
	if max < 0 || len(s) <= max {
		return s
	}
	last := 0
	for i := range s {
		if i > max {
			return s[:last]
		}
		if i == max {
			return s[:i]
		}
		last = i
	}
	return s[:last]
}
