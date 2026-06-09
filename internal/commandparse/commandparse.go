package commandparse

import (
	"strconv"
	"strings"
)

const (
	CommandMax  = 7
	DefaultVerb = "말"
)

// Command is the Go equivalent of the legacy cmd slots used by command1.c.
type Command struct {
	Num int
	Str [CommandMax]string
	Val [CommandMax]int64
}

// Parse tokenizes input using the legacy verb-final command convention.
func Parse(input string) Command {
	var cmd Command
	if input == "" {
		return cmd
	}

	tokens := splitTokens(input)
	n, m := 0, 0
	args := tokens

	if hasDefaultVerb(input) {
		n = setString(cmd.Str[:], n, DefaultVerb)
		cmd.Val[m] = 1
	} else if len(tokens) > 0 {
		n = setString(cmd.Str[:], n, tokens[len(tokens)-1])
		cmd.Val[m] = 1
		args = tokens[:len(tokens)-1]
	} else {
		return cmd
	}

	for _, token := range args {
		if n == m {
			n = setString(cmd.Str[:], n, token)
			if m < CommandMax {
				cmd.Val[m] = 1
			}
			continue
		}

		if value, ok := parseNumberToken(token); ok {
			if m >= CommandMax {
				break
			}
			cmd.Val[m] = value
			m++
			continue
		}

		n = setString(cmd.Str[:], n, token)
		if m >= CommandMax {
			break
		}
		cmd.Val[m] = 1
		m++
	}

	if n > m && m < CommandMax {
		cmd.Val[m] = 1
	}
	cmd.Num = n
	return cmd
}

// ParseCommandFirst tokenizes input using the more common verb-first command
// convention. It is intended as a compatibility fallback around the legacy
// verb-final parser, not as the primary command mode.
func ParseCommandFirst(input string) Command {
	var cmd Command
	tokens := splitTokens(input)
	if len(tokens) == 0 {
		return cmd
	}

	n, m := 0, 0
	n = setString(cmd.Str[:], n, tokens[0])
	cmd.Val[m] = 1

	for _, token := range tokens[1:] {
		if value, ok := parseNumberToken(token); ok {
			if m >= CommandMax {
				break
			}
			cmd.Val[m] = value
			m++
			continue
		}

		n = setString(cmd.Str[:], n, token)
		if m >= CommandMax {
			break
		}
		cmd.Val[m] = 1
		m++
	}

	if n > m && m < CommandMax {
		cmd.Val[m] = 1
	}
	cmd.Num = n
	return cmd
}

func splitTokens(input string) []string {
	return strings.FieldsFunc(input, func(r rune) bool {
		return r == ' ' || r == '#'
	})
}

func hasDefaultVerb(input string) bool {
	var last rune
	for _, r := range input {
		last = r
	}

	switch last {
	case ' ', '.', '!', '?':
		return true
	default:
		return false
	}
}

func parseNumberToken(token string) (int64, bool) {
	if token == "" {
		return 0, false
	}

	start := 0
	if token[0] == '-' {
		if len(token) == 1 {
			return 0, false
		}
		start = 1
	}
	for _, r := range token[start:] {
		if r < '0' || r > '9' {
			return 0, false
		}
	}

	value, err := strconv.ParseInt(token, 10, 64)
	return value, err == nil
}

func setString(slots []string, n int, value string) int {
	if n >= len(slots) {
		return n
	}
	slots[n] = value
	return n + 1
}
