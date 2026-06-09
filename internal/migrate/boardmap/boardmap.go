package boardmap

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"muhan/internal/persist/cbin"
	"muhan/internal/persist/legacykr"
	"muhan/internal/world/model"
)

const (
	boardIndexName = "board_index"
)

var (
	boardPostFileRE = regexp.MustCompile(`^board\.([0-9]+)$`)
	legacyLocation  = time.FixedZone("KST", 9*60*60)
)

type BoardRecord struct {
	ID         string             `json:"id"`
	Path       string             `json:"path"`
	IndexCount int                `json:"indexCount"`
	Posts      []BoardPostSummary `json:"posts,omitempty"`
	Warnings   []string           `json:"warnings,omitempty"`
}

type BoardPostSummary struct {
	Number      int        `json:"number"`
	Title       string     `json:"title,omitempty"`
	TitleRaw    []byte     `json:"-"`
	Uploader    string     `json:"uploader,omitempty"`
	UploaderRaw []byte     `json:"-"`
	CreatedAt   *time.Time `json:"createdAt,omitempty"`
	LineCount   int        `json:"lineCount,omitempty"`
	ReadCount   int        `json:"readCount,omitempty"`
	BodyPath    string     `json:"bodyPath,omitempty"`
	BodyRawPath string     `json:"bodyRawPath,omitempty"`
	BodyBytes   int        `json:"bodyBytes,omitempty"`
	Body        string     `json:"-"`
	BodyRaw     []byte     `json:"-"`
	BodyPreview string     `json:"bodyPreview,omitempty"`
	Warnings    []string   `json:"warnings,omitempty"`
	Indexed     bool       `json:"-"`
}

type Report struct {
	Root     string        `json:"root"`
	Counts   Counts        `json:"counts"`
	Boards   []BoardRecord `json:"boards,omitempty"`
	Warnings []Finding     `json:"warnings,omitempty"`
	Errors   []Finding     `json:"errors,omitempty"`
}

type Counts struct {
	BoardDirs    int `json:"boardDirs"`
	IndexFiles   int `json:"indexFiles"`
	IndexRecords int `json:"indexRecords"`
	PostFiles    int `json:"postFiles"`
	Warnings     int `json:"warnings"`
	Errors       int `json:"errors"`
}

type Finding struct {
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

func ScanRoot(root string) (Report, error) {
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Report{}, fmt.Errorf("resolve root: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return Report{}, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return Report{}, fmt.Errorf("root is not a directory: %s", absRoot)
	}

	report := Report{
		Root:     absRoot,
		Boards:   []BoardRecord{},
		Warnings: []Finding{},
		Errors:   []Finding{},
	}

	boardRoot := filepath.Join(absRoot, "board")
	indexPaths, err := findBoardIndexes(boardRoot)
	if err != nil {
		report.Errors = append(report.Errors, Finding{Path: displayPath(absRoot, boardRoot), Message: err.Error()})
		report.Counts.Errors = len(report.Errors)
		return report, nil
	}

	for _, indexPath := range indexPaths {
		board, postFiles, err := MapBoardDir(absRoot, filepath.Dir(indexPath))
		report.Counts.BoardDirs++
		report.Counts.IndexFiles++
		report.Counts.PostFiles += postFiles
		if err != nil {
			report.Errors = append(report.Errors, Finding{Path: displayPath(absRoot, indexPath), Message: err.Error()})
			continue
		}
		report.Counts.IndexRecords += board.IndexCount
		report.Boards = append(report.Boards, board)
		for _, warning := range board.Warnings {
			report.Warnings = append(report.Warnings, Finding{Path: board.Path, Message: warning})
		}
		for _, post := range board.Posts {
			for _, warning := range post.Warnings {
				path := post.BodyPath
				if path == "" {
					path = board.Path
				}
				report.Warnings = append(report.Warnings, Finding{Path: path, Message: warning})
			}
		}
	}

	report.Counts.Warnings = len(report.Warnings)
	report.Counts.Errors = len(report.Errors)
	return report, nil
}

func MapBoardDir(root, dir string) (BoardRecord, int, error) {
	relDir := displayPath(root, dir)
	board := BoardRecord{
		ID:       boardID(relDir),
		Path:     relDir,
		Posts:    []BoardPostSummary{},
		Warnings: []string{},
	}

	postFiles, err := listPostFiles(dir)
	if err != nil {
		return board, 0, fmt.Errorf("list board posts %s: %v", relDir, underlyingPathError(err))
	}

	indexPath := filepath.Join(dir, boardIndexName)
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return board, len(postFiles), fmt.Errorf("read board index %s: %v", displayPath(root, indexPath), underlyingPathError(err))
	}
	records, err := cbin.DecodeBoardIndexRecords(data)
	if err != nil {
		return board, len(postFiles), fmt.Errorf("decode board index: %w", err)
	}
	board.IndexCount = len(records)

	indexed := make(map[int]struct{}, len(records))
	for i, record := range records {
		post := mapIndexRecord(root, dir, i, record)
		indexed[post.Number] = struct{}{}
		readPostBody(root, filepath.Join(dir, fmt.Sprintf("board.%d", post.Number)), &post)
		board.Posts = append(board.Posts, post)
	}

	for _, postFile := range postFiles {
		if _, ok := indexed[postFile.Number]; ok {
			continue
		}
		post := BoardPostSummary{
			Number: postFile.Number,
			Warnings: []string{
				"post body has no board_index record",
			},
		}
		readPostBody(root, postFile.Path, &post)
		board.Posts = append(board.Posts, post)
	}

	sort.SliceStable(board.Posts, func(i, j int) bool {
		return board.Posts[i].Number < board.Posts[j].Number
	})

	return board, len(postFiles), nil
}

func EncodeJSON(w io.Writer, report Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func BoardPosts(report Report) []model.BoardPost {
	posts := make([]model.BoardPost, 0)
	for _, board := range report.Boards {
		for _, summary := range board.Posts {
			if summary.Number <= 0 {
				continue
			}
			var createdAt time.Time
			if summary.CreatedAt != nil {
				createdAt = *summary.CreatedAt
			}
			metadata := model.Metadata{
				Source:         "boardmap",
				LegacyKind:     "boardPost",
				LegacyID:       fmt.Sprintf("%s:%d", board.ID, summary.Number),
				LegacyPath:     summary.BodyPath,
				LegacyEncoding: "euc-kr/cp949",
				RawFields:      boardPostRawFields(summary),
				Notes:          append([]string(nil), summary.Warnings...),
			}
			if summary.BodyRawPath != "" {
				metadata.Tags = []string{"body:rawDecodeFailed"}
			}
			posts = append(posts, model.BoardPost{
				ID:         model.BoardPostID(boardPostID(board.ID, summary.Number)),
				BoardID:    model.BoardID(board.ID),
				Title:      summary.Title,
				AuthorName: summary.Uploader,
				Body:       summary.Body,
				CreatedAt:  createdAt,
				ReadCount:  summary.ReadCount,
				Metadata:   metadata,
			})
		}
	}
	sort.SliceStable(posts, func(i, j int) bool {
		return posts[i].ID < posts[j].ID
	})
	return posts
}

func WriteText(w io.Writer, report Report, maxFindings int) {
	fmt.Fprintf(w, "root: %s\n", report.Root)
	fmt.Fprintf(w, "board dirs: %d\n", report.Counts.BoardDirs)
	fmt.Fprintf(w, "index files: %d\n", report.Counts.IndexFiles)
	fmt.Fprintf(w, "index records: %d\n", report.Counts.IndexRecords)
	fmt.Fprintf(w, "post files: %d\n", report.Counts.PostFiles)
	fmt.Fprintf(w, "warnings: %d\n", len(report.Warnings))
	writeFindings(w, report.Warnings, maxFindings)
	fmt.Fprintf(w, "errors: %d\n", len(report.Errors))
	writeFindings(w, report.Errors, maxFindings)
}

func findBoardIndexes(boardRoot string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(boardRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != boardIndexName {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

type postFile struct {
	Number int
	Path   string
}

func listPostFiles(dir string) ([]postFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]postFile, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := boardPostFileRE.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		number, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		files = append(files, postFile{Number: number, Path: filepath.Join(dir, entry.Name())})
	}
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].Number < files[j].Number
	})
	return files, nil
}

func mapIndexRecord(root, dir string, index int, record cbin.BoardIndexRecord) BoardPostSummary {
	post := BoardPostSummary{
		Number:      int(record.Number),
		Title:       record.Title.Text,
		TitleRaw:    cloneBytes(record.Title.Raw),
		Uploader:    record.Uploader.Text,
		UploaderRaw: cloneBytes(record.Uploader.Raw),
		LineCount:   int(record.Line),
		ReadCount:   int(record.ReadCount),
		Warnings:    []string{},
		Indexed:     true,
	}
	if post.Number <= 0 {
		post.Warnings = append(post.Warnings, fmt.Sprintf("board_index record %d has non-positive number %d", index, post.Number))
	}
	if record.Title.Err != nil {
		post.Warnings = append(post.Warnings, fmt.Sprintf("board_index record %d title decode: %v", index, record.Title.Err))
	}
	if record.Uploader.Err != nil {
		post.Warnings = append(post.Warnings, fmt.Sprintf("board_index record %d uploader decode: %v", index, record.Uploader.Err))
	}
	createdAt, err := createdAt(record)
	if err != nil {
		post.Warnings = append(post.Warnings, fmt.Sprintf("board_index record %d createdAt: %v", index, err))
	} else {
		post.CreatedAt = &createdAt
	}
	bodyPath := filepath.Join(dir, fmt.Sprintf("board.%d", post.Number))
	post.BodyPath = displayPath(root, bodyPath)
	return post
}

func readPostBody(root, path string, post *BoardPostSummary) {
	post.BodyPath = displayPath(root, path)
	data, err := os.ReadFile(path)
	if err != nil {
		post.Warnings = append(post.Warnings, fmt.Sprintf("read post body %s: %v", post.BodyPath, underlyingPathError(err)))
		return
	}
	post.BodyBytes = len(data)
	post.BodyRaw = cloneBytes(data)

	body, err := legacykr.ValidUTF8OrDecodeContext(legacykr.Context{Path: post.BodyPath, Field: "body"}, data)
	if err != nil {
		post.BodyRawPath = post.BodyPath
		post.Warnings = append(post.Warnings, fmt.Sprintf("decode post body: %v", err))
		return
	}
	post.Body = body
	post.BodyPreview = preview(body, 160)
}

func createdAt(record cbin.BoardIndexRecord) (time.Time, error) {
	year := int(record.Year)
	if year < 0 {
		return time.Time{}, fmt.Errorf("invalid year %d", year)
	}
	if year < 1900 {
		year += 1900
	}
	month := int(record.Month)
	day := int(record.Day)
	hour := int(record.Hour)
	minute := int(record.Minute)
	second := int(record.Second)
	if month < 1 || month > 12 {
		return time.Time{}, fmt.Errorf("invalid month %d", month)
	}
	if day < 1 || day > 31 {
		return time.Time{}, fmt.Errorf("invalid day %d", day)
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 || second < 0 || second > 60 {
		return time.Time{}, fmt.Errorf("invalid time %02d:%02d:%02d", hour, minute, second)
	}

	t := time.Date(year, time.Month(month), day, hour, minute, second, 0, legacyLocation)
	if t.Year() != year || int(t.Month()) != month || t.Day() != day {
		return time.Time{}, fmt.Errorf("invalid date %04d-%02d-%02d", year, month, day)
	}
	return t, nil
}

func preview(body string, limit int) string {
	text := strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

func boardPostRawFields(summary BoardPostSummary) map[string][]byte {
	fields := map[string][]byte{}
	addRawBytes(fields, "board_index.title", summary.TitleRaw)
	addRawBytes(fields, "board_index.uploader", summary.UploaderRaw)
	addRawBytes(fields, "body", summary.BodyRaw)
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func addRawBytes(fields map[string][]byte, key string, value []byte) {
	if len(value) == 0 {
		return
	}
	fields[key] = cloneBytes(value)
}

func cloneBytes(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	out := make([]byte, len(value))
	copy(out, value)
	return out
}

func boardID(path string) string {
	if path == "board" {
		return "board:root"
	}
	if strings.HasPrefix(path, "board/") {
		return "board:" + strings.TrimPrefix(path, "board/")
	}
	return "board:" + strings.ReplaceAll(path, "/", ":")
}

func boardPostID(boardID string, number int) string {
	return fmt.Sprintf("post:%s:%06d", strings.ReplaceAll(boardID, "/", ":"), number)
}

func underlyingPathError(err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}

func displayPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func writeFindings(w io.Writer, findings []Finding, maxFindings int) {
	if len(findings) == 0 {
		return
	}
	limit := maxFindings
	if limit <= 0 || limit > len(findings) {
		limit = len(findings)
	}
	for _, finding := range findings[:limit] {
		if finding.Path == "" {
			fmt.Fprintf(w, "  - %s\n", strings.TrimSpace(finding.Message))
			continue
		}
		fmt.Fprintf(w, "  - %s: %s\n", finding.Path, strings.TrimSpace(finding.Message))
	}
	if limit < len(findings) {
		fmt.Fprintf(w, "  ... %d more\n", len(findings)-limit)
	}
}
