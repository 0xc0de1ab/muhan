package dataissues

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/persist/cbin"
)

const (
	KindRoom   = "room"
	KindPlayer = "player"
	KindBank   = "bank"
	KindBoard  = "board"

	SeverityError = "error"
)

type Report struct {
	Root   string  `json:"root"`
	Counts Counts  `json:"counts"`
	Issues []Issue `json:"issues"`
}

type Counts struct {
	RoomFiles   int `json:"roomFiles"`
	PlayerFiles int `json:"playerFiles"`
	BankFiles   int `json:"bankFiles"`
	BoardFiles  int `json:"boardFiles"`
}

type Issue struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Size     int64  `json:"size"`
	Hint     string `json:"hint,omitempty"`
}

var roomFileRE = regexp.MustCompile(`^r[0-9]{5}$`)

func Scan(root string) (Report, error) {
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

	report := Report{Root: absRoot, Issues: []Issue{}}
	scanRooms(absRoot, &report)
	scanPlayers(absRoot, &report)
	scanBoards(absRoot, &report)
	return report, nil
}

func EncodeJSON(w io.Writer, report Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func WriteText(w io.Writer, report Report) {
	fmt.Fprintf(w, "root: %s\n", report.Root)
	fmt.Fprintf(w, "scanned: rooms=%d players=%d banks=%d boards=%d\n",
		report.Counts.RoomFiles,
		report.Counts.PlayerFiles,
		report.Counts.BankFiles,
		report.Counts.BoardFiles,
	)
	fmt.Fprintf(w, "issues: %d\n", len(report.Issues))
	for _, issue := range report.Issues {
		fmt.Fprintf(w, "- %s %s %s size=%d: %s\n",
			issue.Severity,
			issue.Kind,
			issue.Path,
			issue.Size,
			strings.TrimSpace(issue.Message),
		)
		if issue.Hint != "" {
			fmt.Fprintf(w, "  hint: %s\n", issue.Hint)
		}
	}
}

func scanRooms(root string, report *Report) {
	base := filepath.Join(root, "rooms")
	if missingDir(base, report, KindRoom) {
		return
	}

	_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			addIssue(report, root, path, KindRoom, 0, err.Error(), "")
			return nil
		}
		if d.IsDir() || !roomFileRE.MatchString(d.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			addIssue(report, root, path, KindRoom, fileSize(path), err.Error(), "")
			return nil
		}

		report.Counts.RoomFiles++
		if _, err := cbin.DecodeRoomFile(data); err != nil {
			addIssue(report, root, path, KindRoom, int64(len(data)), err.Error(), classifyHint(KindRoom, int64(len(data)), err.Error()))
		}
		return nil
	})
}

func scanPlayers(root string, report *Report) {
	playerRoot := filepath.Join(root, "player")
	entries, err := os.ReadDir(playerRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		addIssue(report, root, playerRoot, KindPlayer, 0, err.Error(), "")
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dir := filepath.Join(playerRoot, entry.Name())
		switch entry.Name() {
		case "alias", "fal", "family", "invite", "marriage", "simul", "temp", "vote":
			continue
		case "bank":
			scanBankDir(root, dir, report)
		default:
			scanPlayerShardDir(root, dir, report)
		}
	}

	scanBankDir(root, filepath.Join(playerRoot, "family", "bank"), report)
}

func scanPlayerShardDir(root, dir string, report *Report) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		addIssue(report, root, dir, KindPlayer, 0, err.Error(), "")
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			addIssue(report, root, path, KindPlayer, fileSize(path), err.Error(), "")
			continue
		}

		report.Counts.PlayerFiles++
		if _, err := cbin.DecodeCreatureFile(data); err != nil {
			addIssue(report, root, path, KindPlayer, int64(len(data)), err.Error(), classifyHint(KindPlayer, int64(len(data)), err.Error()))
		}
	}
}

func scanBankDir(root, dir string, report *Report) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		addIssue(report, root, dir, KindBank, 0, err.Error(), "")
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			addIssue(report, root, path, KindBank, fileSize(path), err.Error(), "")
			continue
		}

		report.Counts.BankFiles++
		if _, err := cbin.DecodeObjectFileAllowTrailing(data); err != nil {
			addIssue(report, root, path, KindBank, int64(len(data)), err.Error(), classifyHint(KindBank, int64(len(data)), err.Error()))
		}
	}
}

func scanBoards(root string, report *Report) {
	base := filepath.Join(root, "board")
	if missingDir(base, report, KindBoard) {
		return
	}

	_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			addIssue(report, root, path, KindBoard, 0, err.Error(), "")
			return nil
		}
		if d.IsDir() || d.Name() != "board_index" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			addIssue(report, root, path, KindBoard, fileSize(path), err.Error(), "")
			return nil
		}

		report.Counts.BoardFiles++
		if _, err := cbin.ValidateBoardIndexFile(data); err != nil {
			addIssue(report, root, path, KindBoard, int64(len(data)), err.Error(), "")
		}
		return nil
	})
}

func missingDir(path string, report *Report, kind string) bool {
	info, err := os.Stat(path)
	if err == nil {
		return !info.IsDir()
	}
	if os.IsNotExist(err) {
		return true
	}
	addIssue(report, report.Root, path, kind, 0, err.Error(), "")
	return true
}

func addIssue(report *Report, root, path, kind string, size int64, message, hint string) {
	report.Issues = append(report.Issues, Issue{
		Path:     relPath(root, path),
		Kind:     kind,
		Severity: SeverityError,
		Message:  message,
		Size:     size,
		Hint:     hint,
	})
}

func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func classifyHint(kind string, size int64, message string) string {
	switch {
	case kind == KindRoom && size == 0:
		return "0-byte room: restore the room file from backup or remove/regenerate it before migration."
	case strings.Contains(message, "room description") &&
		strings.Contains(message, "bytes: need") &&
		strings.Contains(message, "remaining"):
		return "length-prefixed description EOF: the description length says more bytes follow than the file contains."
	case strings.Contains(message, "object record: need") &&
		(strings.Contains(message, "room object") ||
			strings.Contains(message, "creature inventory object") ||
			strings.Contains(message, "object child")):
		return "truncated nested object: a nested object count points to an object record that is not fully present."
	case strings.Contains(message, "object child count: need int32") &&
		(strings.Contains(message, "room object") ||
			strings.Contains(message, "creature inventory object") ||
			strings.Contains(message, "object child")):
		return "truncated nested object: a nested object record is present but its child-count field is missing."
	default:
		return ""
	}
}
