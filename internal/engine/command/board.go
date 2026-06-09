package command

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"muhan/internal/krtext"
	"muhan/internal/migrate/boardmap"
	"muhan/internal/persist/cbin"
	"muhan/internal/persist/legacykr"
	"muhan/internal/world/model"
)

const legacyBoardSpecial = 4
const legacyBoardDMClass = legacyClassDM
const legacyBoardSubDMClass = legacyClassSubDM

const (
	boardIndexNumberOff    = 0
	boardIndexUploaderOff  = 4
	boardIndexYearOff      = 20
	boardIndexMonthOff     = 24
	boardIndexDayOff       = 28
	boardIndexHourOff      = 32
	boardIndexMinuteOff    = 36
	boardIndexSecondOff    = 40
	boardIndexLineOff      = 44
	boardIndexReadCountOff = 48
	boardIndexTitleOff     = 52
	boardIndexUploaderSize = 16
	boardIndexTitleSize    = 40
)

const (
	boardListPageSize       = 18
	boardListPrompt         = "\n번호, 앞페이지(b), 다음페이지(f), 앞글(a), 다음글(n), 쓰기(w), 중단(q) >> "
	boardListQuitMessage    = "게시물을 그만 봅니다."
	boardNoBoardMessage     = "이곳에는 게시판이 없습니다."
	boardInvalidMessage     = "잘못된 게시판입니다."
	boardEmptyListMessage   = "등록된 게시물이 없습니다."
	boardDeletedPostMessage = "삭제된 게시물입니다. [엔터]를 눌러주세요. "
	boardOutOfRangeMessage  = "범위에 벗어나는 게시물입니다."
	boardDeleteUsageMessage = "사용법: 게시판 <번호> 글삭제"
	boardDeleteDenyMessage  = "당신에게는 삭제할 권한이 없습니다."
	boardDeleteMarkMessage  = "게시물이 삭제되었습니다."
	boardDeleteUndoMessage  = "삭제된 게시물을 복구하였습니다."
	boardWriteCancelMessage = "게시물 작성을 취소합니다."
	boardWriteDoneMessage   = "게시물이 등록되었습니다."
)

var boardFileMu sync.Mutex

var legacyBoardDirs = map[int]string{
	100: "info",
	101: "family1",
	102: "family2",
	103: "family3",
	104: "family4",
	105: "family5",
	106: "family6",
	107: "family7",
	108: "family8",
	109: "family9",
	110: "family10",
	111: "family11",
	112: "family12",
	113: "family13",
	114: "family14",
	115: "family15",
	116: "family",
	120: "user",
	121: "notice",
}

func NewBoardLookHandler(world LookWorld, root string) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		boardObject, ok := findRoomBoardObject(world, room, viewerDetectsInvisible(world, viewer))
		if !ok {
			ctx.WriteString(boardNoBoardMessage)
			return StatusDefault, nil
		}
		boardDir, ok := boardObjectDir(world, boardObject)
		if !ok {
			ctx.WriteString(boardInvalidMessage)
			return StatusDefault, nil
		}

		actorName := boardActorName(world, viewer)
		actorClass := boardActorClass(world, viewer)
		if number := boardPostNumberTarget(resolved); number > 0 {
			board, err := loadBoard(root, boardDir)
			if err != nil {
				return StatusDefault, err
			}
			return renderBoardPost(ctx, root, boardDir, board, number, actorName, actorClass, world)
		}
		if err := boardRoomBroadcast(ctx, room.ID, actorName, "게시판을 봅니다."); err != nil {
			return StatusDefault, err
		}
		list := &boardListState{
			world:      world,
			root:       root,
			dir:        boardDir,
			roomID:     room.ID,
			actorName:  actorName,
			actorClass: actorClass,
			noticeOnly: roomHasNoticeOnlyBoard(world, room, viewerDetectsInvisible(world, viewer)),
		}
		return list.render(ctx)
	}
}

func NewBoardReadAliasHandler(world LookWorld, root string) Handler {
	look := NewBoardLookHandler(world, root)
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		if !boardReadAliasTargetsBoard(resolved) {
			ctx.WriteString("무엇을 읽으시려구요?\n")
			return StatusDefault, nil
		}
		alias := resolved
		alias.Args = nil
		alias.Values = nil
		if number := boardReadAliasPostNumber(resolved); number > 0 {
			alias.Args = []string{strconv.Itoa(number)}
			alias.Values = []int64{int64(number)}
		}
		return look(ctx, alias)
	}
}

func NewBoardWriteHandler(world LookWorld, root string) Handler {
	return newBoardWriteHandler(world, root, time.Now)
}

func newBoardWriteHandler(world LookWorld, root string, now func() time.Time) Handler {
	if now == nil {
		now = time.Now
	}
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		boardObject, ok := findRoomBoardObject(world, room, viewerDetectsInvisible(world, viewer))
		if !ok {
			ctx.WriteString(boardNoBoardMessage)
			return StatusDefault, nil
		}
		boardDir, ok := boardObjectDir(world, boardObject)
		if !ok {
			ctx.WriteString(boardInvalidMessage)
			return StatusDefault, nil
		}
		if roomHasNoticeOnlyBoard(world, room, viewerDetectsInvisible(world, viewer)) && boardActorClass(world, viewer) != legacyBoardDMClass {
			ctx.WriteString("\n\n공지용 게시판입니다. [관리자]만이 쓸 수 있습니다.\n")
			return StatusDefault, nil
		}
		author := boardActorName(world, viewer)
		if author == "" {
			author = string(viewer.PlayerID)
		}
		if err := boardRoomBroadcast(ctx, room.ID, author, "게시판에 글을 씁니다."); err != nil {
			return StatusDefault, err
		}
		write := &boardWriteState{
			world:  world,
			root:   root,
			dir:    boardDir,
			author: author,
			now:    now,
		}
		ctx.WriteString("제목: ")
		if !SetPendingLineHandler(ctx, write.handleTitle) {
			return StatusDefault, fmt.Errorf("게시판 쓰기 상태를 시작할 수 없습니다")
		}
		return StatusDoPrompt, nil
	}
}

func NewBoardDeleteHandler(world LookWorld, root string) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		number, ok := boardDeleteNumberTarget(resolved)
		if !ok || number == 1 && boardActorClass(world, LookViewerFromContext(ctx)) < legacyBoardDMClass {
			ctx.WriteString(boardDeleteUsageMessage)
			return StatusDefault, nil
		}

		viewer, room, err := CurrentRoom(world, LookViewerFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		boardObject, ok := findRoomBoardObject(world, room, viewerDetectsInvisible(world, viewer))
		if !ok || !boardObjectLooksLikeBoard(world, boardObject) {
			ctx.WriteString(boardNoBoardMessage)
			return StatusDefault, nil
		}
		boardDir, ok := boardObjectDir(world, boardObject)
		if !ok {
			ctx.WriteString(boardInvalidMessage)
			return StatusDefault, nil
		}

		result, err := toggleBoardPostDeleted(root, boardDir, number, boardActorName(world, viewer), boardActorClass(world, viewer))
		if err != nil {
			return StatusDefault, err
		}
		switch result {
		case boardDeleteOutOfRange:
			ctx.WriteString(boardOutOfRangeMessage)
		case boardDeleteForbidden:
			ctx.WriteString(boardDeleteDenyMessage)
		case boardDeleteMarked:
			ctx.WriteString(boardDeleteMarkMessage)
		case boardDeleteRestored:
			ctx.WriteString(boardDeleteUndoMessage)
		}
		// C: Mark + queue sidecar for delete/restore too (Package C board persistence)
		if result == boardDeleteMarked || result == boardDeleteRestored {
			persistBoardMutation(world, boardDir)
		}
		return StatusDefault, nil
	}
}

type boardWriteState struct {
	world  LookWorld
	root   string
	dir    string
	author string
	title  string
	lines  []string
	now    func() time.Time
	after  func(*Context) (Status, error)
}

func (s *boardWriteState) handleTitle(ctx *Context, line string) (Status, error) {
	if line == "" {
		ClearPendingLineHandler(ctx)
		ctx.WriteString(boardWriteCancelMessage)
		if s.after != nil {
			return s.after(ctx)
		}
		return StatusDefault, nil
	}
	s.title = strings.TrimRight(line, "\r\n")
	ctx.WriteString("게시물을 작성합니다. 끝내시려면 행의 처음에 [.]을 입력하십시요.\n")
	ctx.WriteString("중간에 취소하시려면 행의 처음에 [!!]를 입력하십시요.\n\n")
	ctx.WriteString("  1: ")
	SetPendingLineHandler(ctx, s.handleBody)
	return StatusDoPrompt, nil
}

func (s *boardWriteState) handleBody(ctx *Context, line string) (Status, error) {
	if strings.HasPrefix(line, "!!") {
		ClearPendingLineHandler(ctx)
		ctx.WriteString(boardWriteCancelMessage)
		if s.after != nil {
			return s.after(ctx)
		}
		return StatusDefault, nil
	}
	if strings.HasPrefix(line, ".") {
		ClearPendingLineHandler(ctx)
		if _, err := appendBoardPost(s.root, s.dir, s.author, s.title, s.lines, s.now()); err != nil {
			return StatusDefault, err
		}
		ctx.WriteString(boardWriteDoneMessage)
		persistBoardMutation(s.world, s.dir)
		if s.after != nil {
			return s.after(ctx)
		}
		return StatusDefault, nil
	}
	s.lines = append(s.lines, line)
	ctx.WriteString(fmt.Sprintf("%3d: ", len(s.lines)+1))
	SetPendingLineHandler(ctx, s.handleBody)
	return StatusDoPrompt, nil
}

func persistBoardMutation(world any, boardDir string) {
	if boardDir == "" {
		return
	}
	if saver, ok := world.(interface {
		MarkBoardDirty(string)
		QueueBoardSave(string, int)
	}); ok {
		saver.MarkBoardDirty(boardDir)
		saver.QueueBoardSave(boardDir, 0)
		return
	}
	if saver, ok := world.(interface{ SaveBoardPosts(string) error }); ok {
		_ = saver.SaveBoardPosts(boardDir)
	}
}

func findRoomBoardObject(world LookWorld, room model.Room, detectInvisible bool) (model.ObjectInstance, bool) {
	for _, objectID := range room.Objects.ObjectIDs {
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInRoom(object, room.ID) {
			continue
		}
		if !detectInvisible && dropObjectIsInvisible(world, object) {
			continue
		}
		if !legacyObjectPrefixMatches(world, object, "게시판") {
			continue
		}
		if _, ok := boardObjectDir(world, object); ok {
			return object, true
		}
	}
	return model.ObjectInstance{}, false
}

func boardObjectLooksLikeBoard(world InventoryWorld, object model.ObjectInstance) bool {
	if special, ok := objectIntProperty(world, object, "special"); ok && special == legacyBoardSpecial {
		return true
	}
	return false
}

func boardObjectDir(world InventoryWorld, object model.ObjectInstance) (string, bool) {
	if boardType, ok := objectIntProperty(world, object, "type"); ok {
		dir, ok := legacyBoardDirs[boardType]
		return dir, ok
	}
	return "", false
}

func roomHasNoticeOnlyBoard(world LookWorld, room model.Room, detectInvisible bool) bool {
	for _, objectID := range room.Objects.ObjectIDs {
		object, ok := world.Object(objectID)
		if !ok || !objectLocatedInRoom(object, room.ID) {
			continue
		}
		if !detectInvisible && dropObjectIsInvisible(world, object) {
			continue
		}
		if legacyObjectPrefixMatches(world, object, "공지용") {
			return true
		}
	}
	return false
}

func boardActorName(world LookWorld, viewer LookViewer) string {
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

func boardActorClass(world LookWorld, viewer LookViewer) int {
	if !viewer.PlayerID.IsZero() {
		if player, ok := world.Player(viewer.PlayerID); ok && !player.CreatureID.IsZero() {
			viewer.CreatureID = player.CreatureID
		}
	}
	if viewer.CreatureID.IsZero() {
		return 0
	}
	creature, ok := world.Creature(viewer.CreatureID)
	if !ok {
		return 0
	}
	return creatureClass(creature)
}

func loadBoard(root string, dir string) (boardmap.BoardRecord, error) {
	boardDir, err := legacyBoardPath(root, dir)
	if err != nil {
		return boardmap.BoardRecord{}, err
	}
	board, _, err := boardmap.MapBoardDir(root, boardDir)
	if err != nil {
		return boardmap.BoardRecord{}, fmt.Errorf("게시판을 읽을 수 없습니다: %w", err)
	}
	return board, nil
}

func appendBoardPost(root string, dir string, author string, title string, lines []string, now time.Time) (int, error) {
	boardFileMu.Lock()
	defer boardFileMu.Unlock()

	boardDir, err := legacyBoardPath(root, dir)
	if err != nil {
		return 0, err
	}
	indexPath := filepath.Join(boardDir, "board_index")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return 0, fmt.Errorf("인덱스화일을 읽을수 없습니다: %w", err)
	}
	if _, err := cbin.ValidateBoardIndexFile(indexData); err != nil {
		return 0, fmt.Errorf("인덱스화일을 읽을수 없습니다: %w", err)
	}
	number := len(indexData)/cbin.BoardIndexSize + 1

	body := strings.Join(lines, "\n")
	if len(lines) > 0 {
		body += "\n"
	}
	bodyBytes, err := legacykr.EncodeEUCKR(body)
	if err != nil {
		return 0, fmt.Errorf("게시물 글자를 저장할 수 없습니다: %w", err)
	}
	bodyPath := filepath.Join(boardDir, fmt.Sprintf("board.%d", number))
	tempPath := filepath.Join(boardDir, fmt.Sprintf(".board.%d.tmp.%d", number, time.Now().UnixNano()))
	if err := os.WriteFile(tempPath, bodyBytes, 0o644); err != nil {
		return 0, fmt.Errorf("게시물 파일을 만들 수 없습니다: %w", err)
	}
	bodyWritten := false
	defer func() {
		if !bodyWritten {
			_ = os.Remove(tempPath)
		}
	}()
	if err := os.Rename(tempPath, bodyPath); err != nil {
		return 0, fmt.Errorf("게시물 파일을 등록할 수 없습니다: %w", err)
	}
	bodyWritten = true

	record, err := encodeBoardIndexRecord(number, author, title, len(lines)+1, 0, now)
	if err != nil {
		return 0, err
	}
	file, err := os.OpenFile(indexPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return 0, fmt.Errorf("인덱스화일을 읽을수 없습니다: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(record); err != nil {
		return 0, fmt.Errorf("인덱스화일을 쓸 수 없습니다: %w", err)
	}
	return number, nil
}

func legacyBoardPath(root string, dir string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	dir, err := safeLegacyBoardDir(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "board", dir), nil
}

func safeLegacyBoardDir(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", fmt.Errorf("unsafe board directory %q", dir)
	}
	if strings.ContainsAny(dir, `/\`) || strings.ContainsAny(dir, "\u2044\u2215\u29f5\ufe68\uff0f\uff3c") {
		return "", fmt.Errorf("unsafe board directory %q", dir)
	}
	clean := filepath.Clean(filepath.FromSlash(dir))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || clean != dir || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe board directory %q", dir)
	}
	return clean, nil
}

func boardPostNumberTarget(resolved ResolvedCommand) int {
	if number := boardPostNumberFromInput(resolved.Input, resolved.Command()); number > 0 {
		return number
	}
	for _, value := range resolved.Values {
		if value > 0 {
			return int(value)
		}
	}
	for _, arg := range resolved.Args {
		if number, err := strconv.Atoi(strings.TrimSpace(arg)); err == nil && number > 0 {
			return number
		}
	}
	return 0
}

func boardReadAliasTargetsBoard(resolved ResolvedCommand) bool {
	for _, arg := range resolved.Args {
		if strings.TrimSpace(arg) == "게시판" {
			return true
		}
	}
	fields := strings.FieldsFunc(resolved.Input, func(r rune) bool {
		return r == ' ' || r == '#'
	})
	for _, field := range fields {
		if field == "게시판" {
			return true
		}
	}
	return false
}

func boardReadAliasPostNumber(resolved ResolvedCommand) int {
	fields := strings.FieldsFunc(resolved.Input, func(r rune) bool {
		return r == ' ' || r == '#'
	})
	for _, field := range fields {
		if field == resolved.Command() || field == "게시판" {
			continue
		}
		if number := positiveInt(field); number > 0 {
			return number
		}
	}
	return 0
}

func boardDeleteNumberTarget(resolved ResolvedCommand) (int, bool) {
	if resolved.Parsed.Num < 2 || resolved.Parsed.Str[1] != "게시판" {
		return 0, false
	}
	number := int(resolved.Parsed.Val[1])
	if number <= 0 {
		return 0, false
	}
	return number, true
}

func boardPostNumberFromInput(input string, command string) int {
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return r == ' ' || r == '#'
	})
	if len(fields) != 2 {
		return 0
	}
	if fields[1] == command {
		return positiveInt(fields[0])
	}
	if fields[0] == command {
		return positiveInt(fields[1])
	}
	return 0
}

func positiveInt(text string) int {
	number, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || number <= 0 {
		return 0
	}
	return number
}

type boardDeleteResult int

const (
	boardDeleteOutOfRange boardDeleteResult = iota
	boardDeleteForbidden
	boardDeleteMarked
	boardDeleteRestored
)

func toggleBoardPostDeleted(root string, dir string, number int, actorName string, actorClass int) (boardDeleteResult, error) {
	boardFileMu.Lock()
	defer boardFileMu.Unlock()

	if number <= 0 {
		return boardDeleteOutOfRange, nil
	}
	boardDir, err := legacyBoardPath(root, dir)
	if err != nil {
		return boardDeleteOutOfRange, err
	}
	indexPath := filepath.Join(boardDir, "board_index")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return boardDeleteOutOfRange, fmt.Errorf("인덱스화일을 읽을수 없습니다: %w", err)
	}
	if _, err := cbin.ValidateBoardIndexFile(data); err != nil {
		return boardDeleteOutOfRange, fmt.Errorf("인덱스화일을 읽을수 없습니다: %w", err)
	}
	offset := (number - 1) * cbin.BoardIndexSize
	if offset < 0 || offset+cbin.BoardIndexSize > len(data) {
		return boardDeleteOutOfRange, nil
	}
	record, err := cbin.DecodeBoardIndexRecord(data[offset : offset+cbin.BoardIndexSize])
	if err != nil {
		return boardDeleteOutOfRange, fmt.Errorf("인덱스화일을 읽을수 없습니다: %w", err)
	}
	uploader := cleanDisplayText(record.Uploader.Text)
	if uploader != actorName && actorClass < legacyBoardSubDMClass {
		return boardDeleteForbidden, nil
	}

	readCount := -int(record.ReadCount)
	if readCount == 0 {
		readCount = -1
	}
	binary.LittleEndian.PutUint32(data[offset+boardIndexReadCountOff:], uint32(int32(readCount)))
	if err := os.WriteFile(indexPath, data, 0o644); err != nil {
		return boardDeleteOutOfRange, fmt.Errorf("인덱스화일을 쓸 수 없습니다: %w", err)
	}
	if readCount < 0 {
		return boardDeleteMarked, nil
	}
	return boardDeleteRestored, nil
}

func incrementBoardPostReadCount(root string, dir string, number int, actorName string) (bool, error) {
	boardFileMu.Lock()
	defer boardFileMu.Unlock()

	if number <= 0 {
		return false, nil
	}
	boardDir, err := legacyBoardPath(root, dir)
	if err != nil {
		return false, err
	}
	indexPath := filepath.Join(boardDir, "board_index")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return false, fmt.Errorf("인덱스화일을 읽을수 없습니다: %w", err)
	}
	if _, err := cbin.ValidateBoardIndexFile(data); err != nil {
		return false, fmt.Errorf("인덱스화일을 읽을수 없습니다: %w", err)
	}
	offset := (number - 1) * cbin.BoardIndexSize
	if offset < 0 || offset+cbin.BoardIndexSize > len(data) {
		return false, nil
	}
	record, err := cbin.DecodeBoardIndexRecord(data[offset : offset+cbin.BoardIndexSize])
	if err != nil {
		return false, fmt.Errorf("인덱스화일을 읽을수 없습니다: %w", err)
	}
	if cleanDisplayText(record.Uploader.Text) == actorName {
		return false, nil
	}
	readCount := int(record.ReadCount)
	if readCount < 0 {
		readCount--
	} else {
		readCount++
	}
	binary.LittleEndian.PutUint32(data[offset+boardIndexReadCountOff:], uint32(int32(readCount)))
	if err := os.WriteFile(indexPath, data, 0o644); err != nil {
		return false, fmt.Errorf("인덱스화일을 쓸 수 없습니다: %w", err)
	}
	return true, nil
}

func encodeBoardIndexRecord(number int, uploader string, title string, lineCount int, readCount int, now time.Time) ([]byte, error) {
	data := make([]byte, cbin.BoardIndexSize)
	binary.LittleEndian.PutUint32(data[boardIndexNumberOff:], uint32(int32(number)))
	if err := copyLegacyBoardField(data[boardIndexUploaderOff:boardIndexUploaderOff+boardIndexUploaderSize], uploader); err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	binary.LittleEndian.PutUint32(data[boardIndexYearOff:], uint32(int32(now.Year()-1900)))
	binary.LittleEndian.PutUint32(data[boardIndexMonthOff:], uint32(int32(now.Month())))
	binary.LittleEndian.PutUint32(data[boardIndexDayOff:], uint32(int32(now.Day())))
	binary.LittleEndian.PutUint32(data[boardIndexHourOff:], uint32(int32(now.Hour())))
	binary.LittleEndian.PutUint32(data[boardIndexMinuteOff:], uint32(int32(now.Minute())))
	binary.LittleEndian.PutUint32(data[boardIndexSecondOff:], uint32(int32(now.Second())))
	binary.LittleEndian.PutUint32(data[boardIndexLineOff:], uint32(int32(lineCount)))
	binary.LittleEndian.PutUint32(data[boardIndexReadCountOff:], uint32(int32(readCount)))
	if err := copyLegacyBoardField(data[boardIndexTitleOff:boardIndexTitleOff+boardIndexTitleSize], title); err != nil {
		return nil, err
	}
	return data, nil
}

func copyLegacyBoardField(dst []byte, text string) error {
	text = strings.ReplaceAll(text, "\x00", "")
	encoded, err := legacykr.EncodeEUCKR(text)
	if err != nil {
		return fmt.Errorf("게시판 글자를 저장할 수 없습니다: %w", err)
	}
	if len(encoded) > len(dst) {
		encoded = encoded[:len(dst)]
	}
	copy(dst, encoded)
	return nil
}

type boardListState struct {
	world       LookWorld
	root        string
	dir         string
	roomID      model.RoomID
	actorName   string
	actorClass  int
	noticeOnly  bool
	page        int
	readNumber  int
	restartNext bool
}

func (s *boardListState) handleLine(ctx *Context, line string) (Status, error) {
	if s.restartNext {
		s.restartNext = false
		s.page = 0
		switch {
		case boardListInputQuits(line):
			ClearPendingLineHandler(ctx)
			ctx.WriteString(boardListQuitMessage)
			return StatusDefault, nil
		case boardListLeadingNumber(line) > 0, boardListInputWrites(line), boardListInputReadsCurrent(line), boardListInputReadsPrevious(line), boardListInputReadsNext(line):
			return s.handleLine(ctx, line)
		default:
			return s.render(ctx)
		}
	}
	if boardListInputQuits(line) {
		ClearPendingLineHandler(ctx)
		ctx.WriteString(boardListQuitMessage)
		return StatusDefault, nil
	}
	if number := boardListLeadingNumber(line); number > 0 {
		s.readNumber = number
		return s.renderPostFromMenu(ctx, number)
	}
	switch {
	case boardListInputWrites(line):
		return s.startWrite(ctx)
	case boardListInputReadsCurrent(line):
		if s.readNumber <= 0 {
			s.readNumber = 1
		}
		return s.renderPostFromMenu(ctx, s.readNumber)
	case boardListInputReadsPrevious(line):
		s.readNumber++
		if s.readNumber <= 0 {
			s.readNumber = 1
		}
		return s.renderPostFromMenu(ctx, s.readNumber)
	case boardListInputReadsNext(line):
		if s.readNumber > 1 {
			s.readNumber--
		} else {
			s.readNumber = 1
		}
		return s.renderPostFromMenu(ctx, s.readNumber)
	case boardListInputPreviousPage(line):
		s.page--
	default:
		s.page++
	}
	return s.render(ctx)
}

func (s *boardListState) render(ctx *Context) (Status, error) {
	board, err := loadBoard(s.root, s.dir)
	if err != nil {
		ClearPendingLineHandler(ctx)
		return StatusDefault, err
	}
	interactive := contextSupportsPendingLineHandler(ctx)
	text, page, total := renderBoardListPage(board, s.actorClass, s.page, interactive)
	s.page = page
	ctx.WriteString(text)
	if total == 0 || !interactive {
		ClearPendingLineHandler(ctx)
		return StatusDefault, nil
	}
	if !SetPendingLineHandler(ctx, s.handleLine) {
		return StatusDefault, fmt.Errorf("게시판 목록 상태를 시작할 수 없습니다")
	}
	return StatusDoPrompt, nil
}

func (s *boardListState) renderPostFromMenu(ctx *Context, number int) (Status, error) {
	board, err := loadBoard(s.root, s.dir)
	if err != nil {
		ClearPendingLineHandler(ctx)
		return StatusDefault, err
	}
	read, err := renderBoardPostMenuPage(ctx, s.root, s.dir, board, number, s.actorName, s.actorClass)
	if err != nil {
		ClearPendingLineHandler(ctx)
		return StatusDefault, err
	}
	if read {
		s.restartNext = true
	}
	return s.continueMenu(ctx)
}

func (s *boardListState) startWrite(ctx *Context) (Status, error) {
	if s.noticeOnly && s.actorClass != legacyBoardDMClass {
		ctx.WriteString("\n\n공지용 게시판입니다. [관리자]만이 쓸 수 있습니다.\n")
		return s.continueMenu(ctx)
	}
	if err := boardRoomBroadcast(ctx, s.roomID, s.actorName, "게시판에 글을 씁니다."); err != nil {
		return StatusDefault, err
	}
	write := &boardWriteState{
		world:  s.world,
		root:   s.root,
		dir:    s.dir,
		author: s.actorName,
		now:    time.Now,
		after: func(ctx *Context) (Status, error) {
			s.restartNext = true
			return s.continueMenu(ctx)
		},
	}
	if write.author == "" {
		write.author = strings.TrimSpace(ctx.ActorID)
	}
	ctx.WriteString("제목: ")
	if !SetPendingLineHandler(ctx, write.handleTitle) {
		return StatusDefault, fmt.Errorf("게시판 쓰기 상태를 시작할 수 없습니다")
	}
	return StatusDoPrompt, nil
}

func (s *boardListState) continueMenu(ctx *Context) (Status, error) {
	if !SetPendingLineHandler(ctx, s.handleLine) {
		return StatusDefault, fmt.Errorf("게시판 목록 상태를 계속할 수 없습니다")
	}
	return StatusDoPrompt, nil
}

func contextSupportsPendingLineHandler(ctx *Context) bool {
	if ctx == nil || ctx.Values == nil {
		return false
	}
	setter, ok := ctx.Values[ContextPendingLineKey].(func(PendingLineHandler))
	return ok && setter != nil
}

func boardListLeadingNumber(line string) int {
	if line == "" || line[0] < '0' || line[0] > '9' {
		return 0
	}
	number := 0
	for i := 0; i < len(line) && i < 4; i++ {
		if line[i] < '0' || line[i] > '9' {
			break
		}
		number = number*10 + int(line[i]-'0')
	}
	return number
}

func boardListInputQuits(line string) bool {
	line = strings.TrimSpace(line)
	return line == "." || line == "ㅂ" || firstASCIIByteIs(line, 'q')
}

func boardListInputWrites(line string) bool {
	line = strings.TrimSpace(line)
	return line == "ㅈ" || firstASCIIByteIs(line, 'w')
}

func boardListInputPreviousPage(line string) bool {
	line = strings.TrimSpace(line)
	return line == "ㅠ" || firstASCIIByteIs(line, 'b')
}

func boardListInputReadsCurrent(line string) bool {
	line = strings.TrimSpace(line)
	return line == "ㅋ" || firstASCIIByteIs(line, 'z')
}

func boardListInputReadsPrevious(line string) bool {
	line = strings.TrimSpace(line)
	return line == "ㅁ" || firstASCIIByteIs(line, 'a')
}

func boardListInputReadsNext(line string) bool {
	line = strings.TrimSpace(line)
	return line == "ㅜ" || firstASCIIByteIs(line, 'n')
}

func firstASCIIByteIs(line string, lower byte) bool {
	if line == "" {
		return false
	}
	first := line[0]
	if first >= 'A' && first <= 'Z' {
		first += 'a' - 'A'
	}
	return first == lower
}

func renderBoardList(board boardmap.BoardRecord, actorClass int) string {
	text, _, _ := renderBoardListPage(board, actorClass, 0, false)
	return text
}

func renderBoardListPage(board boardmap.BoardRecord, actorClass int, page int, prompt bool) (string, int, int) {
	posts := sortedVisibleBoardPosts(board, actorClass)
	total := len(posts)
	if total == 0 {
		return boardEmptyListMessage, 0, 0
	}
	maxPage := (total - 1) / boardListPageSize
	if page < 0 {
		page = 0
	}
	if page > maxPage {
		page = maxPage
	}
	start := page * boardListPageSize
	end := start + boardListPageSize
	if end > total {
		end = total
	}

	var b strings.Builder
	b.WriteString("번호 올린이       날짜  줄수 조회 제목\n")
	b.WriteString("---------------------------------------------------------------\n")
	for _, post := range posts[start:end] {
		author := cleanDisplayText(post.Uploader)
		if author == "" {
			author = "-"
		}
		date := "--/--"
		if post.CreatedAt != nil {
			date = fmt.Sprintf("%02d/%02d", int(post.CreatedAt.Month()), post.CreatedAt.Day())
		}
		title := cleanDisplayText(post.Title)
		if title == "" {
			title = "(제목 없음)"
		}
		fmt.Fprintf(&b, "%4d %-12s %s %4d %4d %s\n",
			post.Number,
			truncateRunes(author, 12),
			date,
			boardPostLineCount(post),
			post.ReadCount,
			title,
		)
	}
	if prompt {
		b.WriteString(boardListPrompt)
	}
	return b.String(), page, total
}

func renderBoardPost(ctx *Context, root string, dir string, board boardmap.BoardRecord, number int, actorName string, actorClass int, world any) (Status, error) {
	for _, post := range board.Posts {
		if post.Number != number {
			continue
		}
		if post.ReadCount < 0 && actorClass < legacyBoardDMClass {
			ctx.WriteString(boardDeletedPostMessage)
			return StatusDefault, nil
		}
		ctx.WriteString(renderBoardPostHeader(post))
		if _, err := incrementBoardPostReadCount(root, dir, number, actorName); err != nil {
			return StatusDefault, err
		}
		return renderLegacyViewFile(ctx, normalizedBoardPostBody(post), "게시판 읽기 상태를 시작할 수 없습니다")
	}
	ctx.WriteString(boardOutOfRangeMessage)
	return StatusDefault, nil
}

func renderBoardPostMenuPage(ctx *Context, root string, dir string, board boardmap.BoardRecord, number int, actorName string, actorClass int) (bool, error) {
	for _, post := range board.Posts {
		if post.Number != number {
			continue
		}
		if post.ReadCount < 0 && actorClass < legacyBoardDMClass {
			ctx.WriteString(boardDeletedPostMessage)
			return false, nil
		}
		ctx.WriteString(renderBoardPostHeader(post))
		if _, err := incrementBoardPostReadCount(root, dir, number, actorName); err != nil {
			return false, err
		}
		body := normalizedBoardPostBody(post)
		page, next := postReadPage(body, 0)
		ctx.WriteString(page)
		if next < len(body) {
			ctx.WriteString(postReadContinuePrompt)
		}
		return true, nil
	}
	ctx.WriteString(boardOutOfRangeMessage)
	return false, nil
}

func renderBoardPostHeader(post boardmap.BoardPostSummary) string {
	title := cleanDisplayText(post.Title)
	if title == "" {
		title = "(제목 없음)"
	}
	author := cleanDisplayText(post.Uploader)
	if author == "" {
		author = "-"
	}
	created := "날짜 없음"
	if post.CreatedAt != nil {
		created = fmt.Sprintf("%04d년 %d월 %d일 %d시 %d분",
			post.CreatedAt.Year(),
			int(post.CreatedAt.Month()),
			post.CreatedAt.Day(),
			post.CreatedAt.Hour(),
			post.CreatedAt.Minute(),
		)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n번호: %d 올린이: %s 제목: %s\n", post.Number, author, title)
	fmt.Fprintf(&b, "올린날:  %s  총줄수: %d  읽은횟수: %d\n", created, boardPostLineCount(post), post.ReadCount)
	b.WriteString("---------------------------------------------------------------\n")
	return b.String()
}

func boardRoomBroadcast(ctx *Context, roomID model.RoomID, actorName string, action string) error {
	actorName = strings.TrimSpace(actorName)
	action = strings.TrimSpace(action)
	if actorName == "" || action == "" {
		return nil
	}
	return roomBroadcast(ctx, roomID, "\n"+actorName+krtext.Particle(actorName, '1')+" "+action)
}

func normalizedBoardPostBody(post boardmap.BoardPostSummary) string {
	body := strings.TrimRight(strings.ReplaceAll(post.Body, "\r\n", "\n"), "\n")
	if body != "" {
		body += "\n"
	}
	return body
}

func visibleBoardPosts(board boardmap.BoardRecord, actorClass int) []boardmap.BoardPostSummary {
	posts := make([]boardmap.BoardPostSummary, 0, len(board.Posts))
	for _, post := range board.Posts {
		if post.Number <= 0 || post.ReadCount < 0 && actorClass < legacyBoardDMClass {
			continue
		}
		posts = append(posts, post)
	}
	return posts
}

func sortedVisibleBoardPosts(board boardmap.BoardRecord, actorClass int) []boardmap.BoardPostSummary {
	posts := visibleBoardPosts(board, actorClass)
	sort.SliceStable(posts, func(i, j int) bool {
		return posts[i].Number > posts[j].Number
	})
	return posts
}

func boardPostLineCount(post boardmap.BoardPostSummary) int {
	if post.Indexed {
		return post.LineCount
	}
	body := strings.TrimRight(strings.ReplaceAll(post.Body, "\r\n", "\n"), "\n")
	if body == "" {
		return 0
	}
	return strings.Count(body, "\n") + 1
}

func truncateRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}
