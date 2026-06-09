// Package table loads and reports on legacy command tables.
package table

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"muhan/internal/commandspec"
	"muhan/internal/commandspec/extract"
)

// EntryRef identifies a command table row in reports.
type EntryRef struct {
	Index      int    `json:"index"`
	Name       string `json:"name"`
	Number     int    `json:"number"`
	Handler    string `json:"handler"`
	Privileged bool   `json:"privileged"`
	Special    bool   `json:"special"`
}

// NameIssue reports an empty or broken command name.
type NameIssue struct {
	EntryRef
	Reason string `json:"reason"`
}

// DuplicateName reports all rows that share one command name.
type DuplicateName struct {
	Name        string     `json:"name"`
	Occurrences []EntryRef `json:"occurrences"`
}

// EntrySummary reports a counted subset of command table rows.
type EntrySummary struct {
	Count   int        `json:"count"`
	Entries []EntryRef `json:"entries,omitempty"`
}

// Analysis is a reportable summary for a parsed legacy command table.
type Analysis struct {
	Count          int             `json:"count"`
	EmptyNames     []NameIssue     `json:"empty_names,omitempty"`
	BrokenNames    []NameIssue     `json:"broken_names,omitempty"`
	DuplicateNames []DuplicateName `json:"duplicate_names,omitempty"`
	Privileged     EntrySummary    `json:"privileged"`
	Special        EntrySummary    `json:"special"`
}

// HasIssues reports whether the table contains name issues or duplicate names.
func (a Analysis) HasIssues() bool {
	return len(a.EmptyNames) > 0 || len(a.BrokenNames) > 0 || len(a.DuplicateNames) > 0
}

// LoadLegacyRegistry extracts src/global.c below root and builds an ordered
// command registry using legacy cmdlist[] order.
func LoadLegacyRegistry(root string) (commandspec.Registry, []extract.Entry, error) {
	entries, err := extract.ExtractRoot(root)
	if err != nil {
		return commandspec.Registry{}, nil, err
	}

	registry, err := commandspec.NewRegistry(extract.CommandSpecs(entries))
	if err != nil {
		return commandspec.Registry{}, entries, fmt.Errorf("build command registry: %w", err)
	}
	return registry, entries, nil
}

// Analyze builds a report for name quality, duplicate names, and
// special/privileged command rows.
func Analyze(entries []extract.Entry) Analysis {
	analysis := Analysis{Count: len(entries)}
	byName := make(map[string][]EntryRef, len(entries))

	for i, entry := range entries {
		ref := entryRef(i, entry)
		name := entry.Name

		if name == "" {
			analysis.EmptyNames = append(analysis.EmptyNames, NameIssue{
				EntryRef: ref,
				Reason:   "empty",
			})
		} else if reason := brokenNameReason(name); reason != "" {
			analysis.BrokenNames = append(analysis.BrokenNames, NameIssue{
				EntryRef: ref,
				Reason:   reason,
			})
		}

		byName[name] = append(byName[name], ref)
		if entry.Privileged {
			analysis.Privileged.Entries = append(analysis.Privileged.Entries, ref)
		}
		if entry.Special {
			analysis.Special.Entries = append(analysis.Special.Entries, ref)
		}
	}

	for name, refs := range byName {
		if len(refs) <= 1 {
			continue
		}
		analysis.DuplicateNames = append(analysis.DuplicateNames, DuplicateName{
			Name:        name,
			Occurrences: refs,
		})
	}
	sort.Slice(analysis.DuplicateNames, func(i, j int) bool {
		return analysis.DuplicateNames[i].Occurrences[0].Index < analysis.DuplicateNames[j].Occurrences[0].Index
	})

	analysis.Privileged.Count = len(analysis.Privileged.Entries)
	analysis.Special.Count = len(analysis.Special.Entries)
	return analysis
}

func entryRef(index int, entry extract.Entry) EntryRef {
	return EntryRef{
		Index:      index,
		Name:       entry.Name,
		Number:     entry.Number,
		Handler:    entry.Handler,
		Privileged: entry.Privileged,
		Special:    entry.Special,
	}
}

func brokenNameReason(name string) string {
	if !utf8.ValidString(name) {
		return "invalid_utf8"
	}
	if strings.ContainsRune(name, utf8.RuneError) {
		return "replacement_rune"
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "control_rune"
		}
	}
	return ""
}
