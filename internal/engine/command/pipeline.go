// Package command connects command parsing with command spec resolution.
package command

import (
	"errors"
	"fmt"
	"strings"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
)

var (
	ErrEmptyCommand      = errors.New("empty command")
	ErrUnknownCommand    = errors.New("unknown command")
	ErrPrivilegedCommand = errors.New("privileged command")
)

// ErrorCode classifies ParseAndResolve failures.
type ErrorCode string

const (
	CodeEmptyCommand      ErrorCode = "empty_command"
	CodeUnknownCommand    ErrorCode = "unknown_command"
	CodePrivilegedCommand ErrorCode = "privileged_command"
)

// ResolveError is the typed error returned by the pre-execution command
// pipeline.
type ResolveError struct {
	Code    ErrorCode
	Input   string
	Command string
	Err     error
}

func (e *ResolveError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Command != "" {
		return fmt.Sprintf("%s %q: %v", e.Code, e.Command, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

func (e *ResolveError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ResolvedCommand is the immutable handoff from parsing/resolution to command
// execution.
type ResolvedCommand struct {
	// Input is the original caller-provided command line.
	Input string
	// Parsed is the legacy-style command slot structure.
	Parsed commandparse.Command
	// Spec is the matched command table row.
	Spec commandspec.CommandSpec
	// MatchIndex is the matched row's registry index.
	MatchIndex int
	// Exact reports whether the command slot exactly matched Spec.Name.
	Exact bool
	// Args contains parsed string slots after command slot 0.
	Args []string
	// Values contains parsed numeric values aligned with Args.
	Values []int64
	// CmdName is the matched command name.
	CmdName string
}

// Command returns parsed command slot 0.
func (c ResolvedCommand) Command() string {
	return c.Parsed.Str[0]
}

// Privileged reports whether the resolved command should be treated as a
// legacy '*' command.
func (c ResolvedCommand) Privileged() bool {
	command := c.Command()
	return c.Spec.Privileged || strings.HasPrefix(c.Spec.Name, "*") || strings.HasPrefix(command, "*")
}

// PrivilegePolicy is called only for resolved privileged commands. Return true
// to allow execution to proceed, or false to block with ErrPrivilegedCommand.
type PrivilegePolicy func(ResolvedCommand) bool

type resolveOptions struct {
	privilegePolicy PrivilegePolicy
}

// Option configures ParseAndResolveWithOptions.
type Option func(*resolveOptions)

// WithPrivilegePolicy installs a callback for legacy '*' command authorization.
func WithPrivilegePolicy(policy PrivilegePolicy) Option {
	return func(opts *resolveOptions) {
		opts.privilegePolicy = policy
	}
}

// DenyPrivileged blocks every privileged command.
func DenyPrivileged() Option {
	return WithPrivilegePolicy(func(ResolvedCommand) bool {
		return false
	})
}

// ParseAndResolve parses input, then resolves command slot 0 against registry.
func ParseAndResolve(input string, registry commandspec.Registry) (ResolvedCommand, error) {
	return ParseAndResolveWithOptions(input, registry)
}

// ParseAndResolveWithOptions parses input, resolves command slot 0 against
// registry, and optionally applies privileged-command policy before execution.
func ParseAndResolveWithOptions(input string, registry commandspec.Registry, opts ...Option) (ResolvedCommand, error) {
	options := resolveOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	parsed := commandparse.Parse(input)
	if parsed.Num == 0 || parsed.Str[0] == "" {
		return ResolvedCommand{}, &ResolveError{
			Code:  CodeEmptyCommand,
			Input: input,
			Err:   ErrEmptyCommand,
		}
	}

	parsed = legacyLowerASCIICommand(parsed)
	match, ok := registry.Resolve(parsed.Str[0])
	if !ok {
		commandFirstParsed := commandparse.ParseCommandFirst(input)
		commandFirstParsed = legacyLowerASCIICommand(commandFirstParsed)
		if commandFirstParsed.Num > 0 && commandFirstParsed.Str[0] != parsed.Str[0] {
			if commandFirstMatch, commandFirstOK := registry.Resolve(commandFirstParsed.Str[0]); commandFirstOK {
				parsed = commandFirstParsed
				match = commandFirstMatch
				ok = true
			}
		}
	}
	if !ok {
		return ResolvedCommand{}, &ResolveError{
			Code:    CodeUnknownCommand,
			Input:   input,
			Command: parsed.Str[0],
			Err:     ErrUnknownCommand,
		}
	}

	resolved := ResolvedCommand{
		Input:      input,
		Parsed:     parsed,
		Spec:       match.Command,
		MatchIndex: match.Index,
		Exact:      match.Exact,
		Args:       commandArgs(parsed),
		Values:     commandValues(parsed),
		CmdName:    match.Command.Name,
	}

	if resolved.Privileged() && options.privilegePolicy != nil && !options.privilegePolicy(resolved) {
		return ResolvedCommand{}, &ResolveError{
			Code:    CodePrivilegedCommand,
			Input:   input,
			Command: parsed.Str[0],
			Err:     ErrPrivilegedCommand,
		}
	}

	return resolved, nil
}

func legacyLowerASCIICommand(parsed commandparse.Command) commandparse.Command {
	for i := range parsed.Str {
		if parsed.Str[i] != "" {
			parsed.Str[i] = legacyLowerASCII(parsed.Str[i])
		}
	}
	return parsed
}

func legacyLowerASCII(text string) string {
	changed := false
	buf := []byte(text)
	for i, c := range buf {
		if c >= 'A' && c <= 'Z' {
			buf[i] = c + ('a' - 'A')
			changed = true
		}
	}
	if !changed {
		return text
	}
	return string(buf)
}

func commandArgs(parsed commandparse.Command) []string {
	if parsed.Num <= 1 {
		return nil
	}

	args := make([]string, parsed.Num-1)
	copy(args, parsed.Str[1:parsed.Num])
	return args
}

func commandValues(parsed commandparse.Command) []int64 {
	if parsed.Num <= 1 {
		return nil
	}

	values := make([]int64, parsed.Num-1)
	copy(values, parsed.Val[1:parsed.Num])
	return values
}
