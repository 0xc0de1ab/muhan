// Package commandspec describes the command table shape used by the
// legacy Mordor-derived command dispatcher.
//
// The C server's process_cmd function checks cmdlist in order. It gives
// exact matches priority, then accepts a command when the input is a prefix
// of a table entry. Its str_compare helper returns the consumed input depth,
// which is effectively the input length for every prefix match of the same
// input. In practice, ambiguous prefix matches are therefore resolved by
// command table order. Registry preserves that deterministic rule while using
// UTF-8-safe rune prefix matching.
package commandspec

import (
	"fmt"
	"unicode/utf8"
)

// CommandSpec is the declarative part of a legacy command table row.
// Number corresponds to cmdno/help number in the C cmdlist. Handler is the
// cmdfn symbol from the same row. For special legacy rows with negative cmdno,
// store the absolute special number and set Special.
type CommandSpec struct {
	Name       string
	Number     int
	Handler    string
	Privileged bool
	Special    bool
}

// Registry preserves command table order for legacy-compatible resolution.
type Registry struct {
	commands []CommandSpec
}

// Match describes how Resolve selected a command.
type Match struct {
	Command CommandSpec
	Index   int
	Exact   bool
}

// NewRegistry copies specs into an ordered registry.
func NewRegistry(specs []CommandSpec) (Registry, error) {
	commands := make([]CommandSpec, len(specs))
	for i, spec := range specs {
		if spec.Name == "" {
			return Registry{}, fmt.Errorf("command %d has empty name", i)
		}
		if !utf8.ValidString(spec.Name) {
			return Registry{}, fmt.Errorf("command %d name is not valid UTF-8", i)
		}
		commands[i] = spec
	}
	return Registry{commands: commands}, nil
}

// Commands returns a copy of the ordered command specs.
func (r Registry) Commands() []CommandSpec {
	commands := make([]CommandSpec, len(r.commands))
	copy(commands, r.commands)
	return commands
}

// Resolve finds the command for input using legacy process_cmd precedence:
// exact match first, then the first table entry whose name has input as a
// complete-rune UTF-8 prefix. Invalid UTF-8 input never matches.
func (r Registry) Resolve(input string) (Match, bool) {
	if input == "" || !utf8.ValidString(input) {
		return Match{}, false
	}

	for i, command := range r.commands {
		if command.Name == input {
			return Match{Command: command, Index: i, Exact: true}, true
		}
	}

	for i, command := range r.commands {
		if hasRunePrefix(command.Name, input) {
			return Match{Command: command, Index: i}, true
		}
	}

	return Match{}, false
}

func hasRunePrefix(s, prefix string) bool {
	if !utf8.ValidString(s) || !utf8.ValidString(prefix) {
		return false
	}

	sr := []rune(s)
	pr := []rune(prefix)
	if len(pr) > len(sr) {
		return false
	}
	for i := range pr {
		if sr[i] != pr[i] {
			return false
		}
	}
	return true
}
