package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"muhan/internal/commandparse"
	"muhan/internal/commandspec"
	"muhan/internal/engine/command"
	"muhan/internal/engine/command/table"
)

type output struct {
	Root          string       `json:"root"`
	Input         string       `json:"input"`
	RegistryCount int          `json:"registry_count"`
	Parsed        parsedOutput `json:"parsed"`
	Match         *matchOutput `json:"match"`
	Args          []string     `json:"args"`
	Values        []int64      `json:"values"`
	Error         *errorOutput `json:"error,omitempty"`
}

type parsedOutput struct {
	Num     int      `json:"num"`
	Command string   `json:"command"`
	Strings []string `json:"strings"`
	Values  []int64  `json:"values"`
}

type matchOutput struct {
	Index int        `json:"index"`
	Exact bool       `json:"exact"`
	Spec  specOutput `json:"spec"`
}

type specOutput struct {
	Name       string `json:"name"`
	Number     int    `json:"number"`
	Privileged bool   `json:"privileged"`
	Special    bool   `json:"special"`
}

type errorOutput struct {
	Code    string `json:"code"`
	Command string `json:"command,omitempty"`
	Message string `json:"message"`
}

func main() {
	root := flag.String("root", ".", "legacy Muhan source root")
	denyPrivileged := flag.Bool("deny-privileged", false, "deny legacy '*' commands")
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: muhan-resolve [-root ROOT] [-deny-privileged] COMMAND_LINE")
		os.Exit(2)
	}
	input := strings.Join(flag.Args(), " ")

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve root: %v\n", err)
		os.Exit(2)
	}

	registry, entries, err := table.LoadLegacyRegistry(absRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load legacy registry: %v\n", err)
		os.Exit(2)
	}

	opts := []command.Option(nil)
	if *denyPrivileged {
		opts = append(opts, command.DenyPrivileged())
	}

	parsed := commandparse.Parse(input)
	out := output{
		Root:          absRoot,
		Input:         input,
		RegistryCount: len(entries),
		Parsed:        newParsedOutput(parsed),
		Args:          []string{},
		Values:        []int64{},
	}

	resolved, err := command.ParseAndResolveWithOptions(input, registry, opts...)
	if err != nil {
		out.Error = newErrorOutput(err)
		writeJSON(out)
		os.Exit(1)
	}

	out.Parsed = newParsedOutput(resolved.Parsed)
	out.Match = &matchOutput{
		Index: resolved.MatchIndex,
		Exact: resolved.Exact,
		Spec:  newSpecOutput(resolved.Spec),
	}
	out.Args = append([]string(nil), resolved.Args...)
	out.Values = append([]int64(nil), resolved.Values...)
	if out.Args == nil {
		out.Args = []string{}
	}
	if out.Values == nil {
		out.Values = []int64{}
	}
	writeJSON(out)
}

func newParsedOutput(parsed commandparse.Command) parsedOutput {
	slots := make([]string, parsed.Num)
	values := make([]int64, parsed.Num)
	copy(slots, parsed.Str[:parsed.Num])
	copy(values, parsed.Val[:parsed.Num])

	command := ""
	if parsed.Num > 0 {
		command = parsed.Str[0]
	}
	return parsedOutput{
		Num:     parsed.Num,
		Command: command,
		Strings: slots,
		Values:  values,
	}
}

func newSpecOutput(spec commandspec.CommandSpec) specOutput {
	return specOutput{
		Name:       spec.Name,
		Number:     spec.Number,
		Privileged: spec.Privileged,
		Special:    spec.Special,
	}
}

func newErrorOutput(err error) *errorOutput {
	var resolveErr *command.ResolveError
	if errors.As(err, &resolveErr) {
		return &errorOutput{
			Code:    string(resolveErr.Code),
			Command: resolveErr.Command,
			Message: resolveErr.Err.Error(),
		}
	}
	return &errorOutput{
		Code:    "resolve_error",
		Message: err.Error(),
	}
}

func writeJSON(out output) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "encode JSON: %v\n", err)
		os.Exit(2)
	}
}
