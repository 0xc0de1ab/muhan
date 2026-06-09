package boardmap

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"muhan/internal/persist/cbin"
	"muhan/internal/persist/legacykr"
)

const (
	boardNumberOff    = 0
	boardUploaderOff  = 4
	boardYearOff      = 20
	boardMonthOff     = 24
	boardDayOff       = 28
	boardHourOff      = 32
	boardMinuteOff    = 36
	boardSecondOff    = 40
	boardLineOff      = 44
	boardReadCountOff = 48
	boardTitleOff     = 52
)

func TestScanRootMapsBoardIndexAndBodyDecode(t *testing.T) {
	root := t.TempDir()
	boardDir := filepath.Join(root, "board", "info")
	if err := os.MkdirAll(boardDir, 0700); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(boardDir, "board_index"), makeBoardIndexRecord(t, 1, "무한", "공지", 126, 5, 20, 1, 2, 3))
	body, err := legacykr.EncodeEUCKR("본문입니다\n둘째 줄")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(boardDir, "board.1"), body)

	report, err := ScanRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.Counts.BoardDirs != 1 || report.Counts.IndexRecords != 1 || report.Counts.PostFiles != 1 {
		t.Fatalf("counts = %+v, want 1 board dir, 1 index record, 1 post file", report.Counts)
	}
	if report.Counts.Warnings != 0 || report.Counts.Errors != 0 {
		t.Fatalf("warnings/errors = %d/%d, want 0/0", report.Counts.Warnings, report.Counts.Errors)
	}
	if len(report.Boards) != 1 {
		t.Fatalf("boards len = %d, want 1", len(report.Boards))
	}

	board := report.Boards[0]
	if board.ID != "board:info" || board.Path != "board/info" || board.IndexCount != 1 {
		t.Fatalf("board = %+v", board)
	}
	if len(board.Posts) != 1 {
		t.Fatalf("posts len = %d, want 1", len(board.Posts))
	}
	post := board.Posts[0]
	if post.Number != 1 || post.Title != "공지" || post.Uploader != "무한" {
		t.Fatalf("post metadata = %+v", post)
	}
	if post.ReadCount != 7 {
		t.Fatalf("read count = %d, want 7", post.ReadCount)
	}
	if post.LineCount != 2 || !post.Indexed {
		t.Fatalf("line/index metadata = %d/%v, want 2/true", post.LineCount, post.Indexed)
	}
	if post.CreatedAt == nil || post.CreatedAt.Year() != 2026 || int(post.CreatedAt.Month()) != 5 || post.CreatedAt.Day() != 20 {
		t.Fatalf("createdAt = %v, want 2026-05-20", post.CreatedAt)
	}
	if post.BodyBytes != len(body) || post.BodyPath != "board/info/board.1" {
		t.Fatalf("post body file = %+v, want bytes=%d path=board/info/board.1", post, len(body))
	}
	if !strings.Contains(post.BodyPreview, "본문입니다") {
		t.Fatalf("body preview = %q, want decoded Korean text", post.BodyPreview)
	}
	if post.Body != "본문입니다\n둘째 줄" {
		t.Fatalf("body = %q, want full decoded body", post.Body)
	}
	titleRaw, err := legacykr.EncodeEUCKR("공지")
	if err != nil {
		t.Fatal(err)
	}
	uploaderRaw, err := legacykr.EncodeEUCKR("무한")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(post.TitleRaw, titleRaw) || !bytes.Equal(post.UploaderRaw, uploaderRaw) || !bytes.Equal(post.BodyRaw, body) {
		t.Fatalf("summary raw fields = title % X uploader % X body % X", post.TitleRaw, post.UploaderRaw, post.BodyRaw)
	}

	rows := BoardPosts(report)
	if len(rows) != 1 {
		t.Fatalf("BoardPosts len = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.ID != "post:board:info:000001" || row.BoardID != "board:info" {
		t.Fatalf("board post ids = %s/%s", row.ID, row.BoardID)
	}
	if row.Title != "공지" || row.AuthorName != "무한" || row.Body != "본문입니다\n둘째 줄" || row.ReadCount != 7 {
		t.Fatalf("board post row = %+v", row)
	}
	if row.Metadata.Source != "boardmap" || row.Metadata.LegacyPath != "board/info/board.1" || row.Metadata.LegacyEncoding != "euc-kr/cp949" {
		t.Fatalf("metadata = %+v", row.Metadata)
	}
	if !bytes.Equal(row.Metadata.RawFields["board_index.title"], titleRaw) {
		t.Fatalf("raw title = % X, want % X", row.Metadata.RawFields["board_index.title"], titleRaw)
	}
	if !bytes.Equal(row.Metadata.RawFields["board_index.uploader"], uploaderRaw) {
		t.Fatalf("raw uploader = % X, want % X", row.Metadata.RawFields["board_index.uploader"], uploaderRaw)
	}
	if !bytes.Equal(row.Metadata.RawFields["body"], body) {
		t.Fatalf("raw body = % X, want % X", row.Metadata.RawFields["body"], body)
	}
}

func TestScanRootWarnsAndPreservesRawBodyPathOnDecodeFailure(t *testing.T) {
	root := t.TempDir()
	boardDir := filepath.Join(root, "board", "info")
	if err := os.MkdirAll(boardDir, 0700); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(boardDir, "board_index"), makeBoardIndexRecord(t, 1, "tester", "bad body", 126, 5, 20, 1, 2, 3))
	writeFile(t, filepath.Join(boardDir, "board.1"), []byte{0xff})

	report, err := ScanRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.Counts.Warnings != 1 || report.Counts.Errors != 0 {
		t.Fatalf("warnings/errors = %d/%d, want 1/0", report.Counts.Warnings, report.Counts.Errors)
	}
	post := report.Boards[0].Posts[0]
	if post.BodyRawPath != "board/info/board.1" {
		t.Fatalf("BodyRawPath = %q, want board/info/board.1", post.BodyRawPath)
	}
	if len(post.Warnings) != 1 || !strings.Contains(post.Warnings[0], "decode post body") {
		t.Fatalf("post warnings = %#v, want decode warning", post.Warnings)
	}
	rows := BoardPosts(report)
	if len(rows) != 1 {
		t.Fatalf("BoardPosts len = %d, want 1", len(rows))
	}
	if rows[0].Body != "" || !bytes.Equal(rows[0].Metadata.RawFields["body"], []byte{0xff}) {
		t.Fatalf("board post body/raw = %q/% X", rows[0].Body, rows[0].Metadata.RawFields["body"])
	}
	if len(rows[0].Metadata.Tags) != 1 || rows[0].Metadata.Tags[0] != "body:rawDecodeFailed" {
		t.Fatalf("metadata tags = %#v, want body decode failure tag", rows[0].Metadata.Tags)
	}
}

func TestScanRootReadBodyWarningUsesLogicalPath(t *testing.T) {
	root := t.TempDir()
	boardDir := filepath.Join(root, "board", "info")
	if err := os.MkdirAll(boardDir, 0700); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(boardDir, "board_index"), makeBoardIndexRecord(t, 1, "tester", "missing body", 126, 5, 20, 1, 2, 3))

	report, err := ScanRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.Counts.Warnings != 1 {
		t.Fatalf("warnings = %d, want 1", report.Counts.Warnings)
	}
	message := report.Warnings[0].Message
	if strings.Contains(message, root) {
		t.Fatalf("warning leaked absolute root %q in %q", root, message)
	}
	if !strings.Contains(message, "board/info/board.1") {
		t.Fatalf("warning = %q, want logical body path", message)
	}
}

func TestBoardPostsPreservesOrphanBodyRawBytes(t *testing.T) {
	root := t.TempDir()
	boardDir := filepath.Join(root, "board", "info")
	if err := os.MkdirAll(boardDir, 0700); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(boardDir, "board_index"), nil)
	body, err := legacykr.EncodeEUCKR("인덱스 없는 본문")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(boardDir, "board.3"), body)

	report, err := ScanRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.Counts.IndexRecords != 0 || report.Counts.PostFiles != 1 || report.Counts.Warnings != 1 {
		t.Fatalf("counts = %+v, want orphan body warning", report.Counts)
	}
	if len(report.Boards) != 1 || len(report.Boards[0].Posts) != 1 {
		t.Fatalf("boards/posts = %+v", report.Boards)
	}

	rows := BoardPosts(report)
	if len(rows) != 1 {
		t.Fatalf("BoardPosts len = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.ID != "post:board:info:000003" || row.Metadata.LegacyPath != "board/info/board.3" {
		t.Fatalf("orphan row ids/path = %s/%s", row.ID, row.Metadata.LegacyPath)
	}
	if row.Body != "인덱스 없는 본문" || !bytes.Equal(row.Metadata.RawFields["body"], body) {
		t.Fatalf("orphan body/raw = %q/% X", row.Body, row.Metadata.RawFields["body"])
	}
	if !containsString(row.Metadata.Notes, "post body has no board_index record") {
		t.Fatalf("metadata notes = %#v, want orphan warning", row.Metadata.Notes)
	}
}

func TestBoardPostSummaryBodyIsNotJSONEncoded(t *testing.T) {
	data, err := json.Marshal(Report{
		Boards: []BoardRecord{{
			ID: "board:info",
			Posts: []BoardPostSummary{{
				Number:      1,
				Title:       "공지",
				TitleRaw:    []byte("raw title marker"),
				UploaderRaw: []byte("raw uploader marker"),
				Body:        "full body must stay out of boardmap JSON",
				BodyRaw:     []byte("raw body marker"),
				BodyPreview: "preview only",
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "full body must stay out of boardmap JSON") {
		t.Fatalf("JSON leaked full body: %s", text)
	}
	if strings.Contains(text, "TitleRaw") || strings.Contains(text, "raw title marker") || strings.Contains(text, "cmF3IHRpdGxlIG1hcmtlcg==") {
		t.Fatalf("JSON leaked raw title bytes: %s", text)
	}
	if strings.Contains(text, "UploaderRaw") || strings.Contains(text, "raw uploader marker") || strings.Contains(text, "cmF3IHVwbG9hZGVyIG1hcmtlcg==") {
		t.Fatalf("JSON leaked raw uploader bytes: %s", text)
	}
	if strings.Contains(text, "BodyRaw") || strings.Contains(text, "raw body marker") || strings.Contains(text, "cmF3IGJvZHkgbWFya2Vy") {
		t.Fatalf("JSON leaked raw body bytes: %s", text)
	}
	if !strings.Contains(text, "preview only") {
		t.Fatalf("JSON = %s, want preview", text)
	}
}

func TestBoardPostsKeepsUntitledLegacyPosts(t *testing.T) {
	rows := BoardPosts(Report{
		Boards: []BoardRecord{{
			ID: "board:info",
			Posts: []BoardPostSummary{{
				Number: 1,
				Body:   "body",
			}},
		}},
	})
	if len(rows) != 1 {
		t.Fatalf("BoardPosts len = %d, want 1", len(rows))
	}
	if rows[0].Title != "" || rows[0].Body != "body" {
		t.Fatalf("board post = %+v, want empty title preserved with body", rows[0])
	}
}

func makeBoardIndexRecord(t *testing.T, number int, uploader, title string, year, month, day, hour, minute, second int) []byte {
	t.Helper()

	data := make([]byte, cbin.BoardIndexSize)
	binary.LittleEndian.PutUint32(data[boardNumberOff:], uint32(int32(number)))
	copyEncoded(t, data[boardUploaderOff:boardUploaderOff+16], uploader)
	binary.LittleEndian.PutUint32(data[boardYearOff:], uint32(int32(year)))
	binary.LittleEndian.PutUint32(data[boardMonthOff:], uint32(int32(month)))
	binary.LittleEndian.PutUint32(data[boardDayOff:], uint32(int32(day)))
	binary.LittleEndian.PutUint32(data[boardHourOff:], uint32(int32(hour)))
	binary.LittleEndian.PutUint32(data[boardMinuteOff:], uint32(int32(minute)))
	binary.LittleEndian.PutUint32(data[boardSecondOff:], uint32(int32(second)))
	binary.LittleEndian.PutUint32(data[boardLineOff:], 2)
	binary.LittleEndian.PutUint32(data[boardReadCountOff:], 7)
	copyEncoded(t, data[boardTitleOff:boardTitleOff+40], title)
	return data
}

func copyEncoded(t *testing.T, dst []byte, text string) {
	t.Helper()

	encoded, err := legacykr.EncodeEUCKR(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > len(dst) {
		t.Fatalf("encoded text %q is %d bytes, max %d", text, len(encoded), len(dst))
	}
	copy(dst, encoded)
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()

	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}
