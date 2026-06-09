package invitemap

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"muhan/internal/persist/legacykr"
)

const (
	MaxInviteNames = 10
)

var inviteFileRE = regexp.MustCompile(`^invite_([0-9]+)$`)

type Finding struct {
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type InviteList struct {
	Number   int       `json:"number"`
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	Names    []string  `json:"names"`
	RawNames [][]byte  `json:"rawNames,omitempty"`
	Warnings []Finding `json:"warnings,omitempty"`
}

type Report struct {
	Root     string       `json:"root,omitempty"`
	Dir      string       `json:"dir"`
	Counts   Counts       `json:"counts"`
	Invites  []InviteList `json:"invites"`
	Warnings []Finding    `json:"warnings,omitempty"`
	Errors   []Finding    `json:"errors,omitempty"`
}

type Counts struct {
	Files        int `json:"files"`
	MappedFiles  int `json:"mappedFiles"`
	Names        int `json:"names"`
	SkippedFiles int `json:"skippedFiles"`
	Warnings     int `json:"warnings"`
	Errors       int `json:"errors"`
}

func MapInviteFile(path string, data []byte) (InviteList, error) {
	displayPath := filepath.ToSlash(path)
	number, err := inviteNumber(filepath.Base(path))
	if err != nil {
		return InviteList{Path: displayPath}, err
	}

	names, rawNames, warnings, err := parseInviteNames(displayPath, data)
	if err != nil {
		return InviteList{
			Number: number,
			ID:     inviteID(number),
			Path:   displayPath,
		}, err
	}

	return InviteList{
		Number:   number,
		ID:       inviteID(number),
		Path:     displayPath,
		Names:    names,
		RawNames: rawNames,
		Warnings: warnings,
	}, nil
}

func ScanRoot(root string) (Report, error) {
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Report{}, fmt.Errorf("resolve root: %w", err)
	}

	report, err := ScanDir(filepath.Join(absRoot, "player", "invite"))
	if err != nil {
		return Report{}, err
	}
	report.Root = absRoot
	return report, nil
}

func ScanDir(dir string) (Report, error) {
	if dir == "" {
		dir = "."
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return Report{}, fmt.Errorf("resolve invite dir: %w", err)
	}
	info, err := os.Stat(absDir)
	if err != nil {
		return Report{}, fmt.Errorf("stat invite dir: %w", err)
	}
	if !info.IsDir() {
		return Report{}, fmt.Errorf("invite dir is not a directory: %s", absDir)
	}

	report := Report{
		Dir:      filepath.ToSlash(absDir),
		Invites:  []InviteList{},
		Warnings: []Finding{},
		Errors:   []Finding{},
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return Report{}, fmt.Errorf("read invite dir: %w", err)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		path := filepath.Join(absDir, entry.Name())
		displayPath := filepath.ToSlash(path)
		if entry.IsDir() {
			report.Counts.SkippedFiles++
			report.Warnings = append(report.Warnings, Finding{
				Path:    displayPath,
				Message: "invite entry is a directory; skipped",
			})
			continue
		}
		if !inviteFileRE.MatchString(entry.Name()) {
			report.Counts.SkippedFiles++
			continue
		}

		report.Counts.Files++
		data, err := os.ReadFile(path)
		if err != nil {
			report.Errors = append(report.Errors, Finding{Path: displayPath, Message: err.Error()})
			continue
		}

		invite, err := MapInviteFile(path, data)
		if err != nil {
			report.Errors = append(report.Errors, Finding{Path: displayPath, Message: err.Error()})
			continue
		}
		report.Counts.MappedFiles++
		report.Counts.Names += len(invite.Names)
		report.Invites = append(report.Invites, invite)
		report.Warnings = append(report.Warnings, invite.Warnings...)
	}

	report.Counts.Warnings = len(report.Warnings)
	report.Counts.Errors = len(report.Errors)
	return report, nil
}

func parseInviteNames(path string, data []byte) ([]string, [][]byte, []Finding, error) {
	tokens := bytes.Fields(data)
	names := make([]string, 0, min(len(tokens), MaxInviteNames))
	rawNames := make([][]byte, 0, min(len(tokens), MaxInviteNames))
	warnings := []Finding{}

	for i, token := range tokens {
		if isInviteSentinel(token) {
			break
		}
		if len(names) >= MaxInviteNames {
			if hasNameToken(tokens[i:]) {
				warnings = append(warnings, Finding{
					Path:    path,
					Message: fmt.Sprintf("invite file has more than %d names; extra tokens ignored", MaxInviteNames),
				})
			}
			break
		}

		name, err := legacykr.DecodeEUCKRContext(legacykr.Context{
			Path:  path,
			Field: fmt.Sprintf("invite name[%d]", len(names)),
		}, token)
		if err != nil {
			return nil, nil, warnings, fmt.Errorf("decode invite name %d: %w", len(names), err)
		}
		names = append(names, name)
		rawNames = append(rawNames, cloneBytes(token))
	}

	return names, rawNames, warnings, nil
}

func hasNameToken(tokens [][]byte) bool {
	for _, token := range tokens {
		if isInviteSentinel(token) {
			return false
		}
		return true
	}
	return false
}

func isInviteSentinel(token []byte) bool {
	return len(token) > 0 && token[0] == '0'
}

func inviteNumber(filename string) (int, error) {
	m := inviteFileRE.FindStringSubmatch(filename)
	if m == nil {
		return 0, fmt.Errorf("invite filename %q does not match invite_<number>", filename)
	}
	number, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("parse invite number %q: %w", m[1], err)
	}
	return number, nil
}

func inviteID(number int) string {
	return "invite:" + strconv.Itoa(number)
}

func cloneBytes(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	out := make([]byte, len(value))
	copy(out, value)
	return out
}
