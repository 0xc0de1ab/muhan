package command

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"muhan/internal/commandspec"
	"muhan/internal/persist/legacykr"
	"muhan/internal/textfmt"
)

var ErrHelpRootRequired = errors.New("help root required")

type helpTopic struct {
	Name   string
	Number int
}

type helpIndex struct {
	root     string
	registry commandspec.Registry

	once      sync.Once
	spells    []helpTopic
	spellErr  error
	helpFiles map[string]struct{}
}

func NewHelpHandler(root string, registry commandspec.Registry) Handler {
	index := &helpIndex{root: root, registry: registry}
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		if index.root == "" {
			return StatusDefault, ErrHelpRootRequired
		}

		file := "helpfile"
		if topic := helpTopicArg(resolved); topic != "" {
			switch topic {
			case "주술":
				file = "spellfile"
			case "정책":
				file = "policy"
			default:
				if match, ok := index.registry.Resolve(topic); ok {
					file = fmt.Sprintf("help.%d", match.Command.Number)
				} else if spell, ok := index.resolveSpell(topic); ok {
					file = fmt.Sprintf("spell.%d", spell.Number)
				} else {
					ctx.WriteString("그 명령어에 대한 도움말은 없습니다.\n")
					return StatusDefault, nil
				}
			}
		}

		text, err := index.read(file)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				ctx.WriteString("화일을 읽을 수 없습니다.\n")
				return StatusDoPrompt, nil
			}
			return StatusDefault, err
		}
		return renderLegacyViewFile(ctx, text, "도움말 읽기 상태를 시작할 수 없습니다")
	}
}

func NewWelcomeHandler(root string) Handler {
	index := &helpIndex{root: root}
	return func(ctx *Context, _ ResolvedCommand) (Status, error) {
		if index.root == "" {
			return StatusDefault, ErrHelpRootRequired
		}
		text, err := index.read("welcome")
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				ctx.WriteString("화일을 읽을 수 없습니다.\n")
				return StatusDoPrompt, nil
			}
			return StatusDefault, err
		}
		return renderLegacyViewFile(ctx, text, "환영말 읽기 상태를 시작할 수 없습니다")
	}
}

func helpTopicArg(resolved ResolvedCommand) string {
	if len(resolved.Args) == 0 {
		return ""
	}
	return strings.TrimSpace(resolved.Args[0])
}

func (i *helpIndex) read(name string) (string, error) {
	path := filepath.Join(i.root, "help", filepath.Clean(name))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text, err := legacykr.ValidUTF8OrDecodeContext(legacykr.Context{
		Path:  path,
		Field: "help text",
	}, data)
	if err != nil {
		return "", err
	}
	text = textfmt.RenderLegacyColors(text, textfmt.Options{})
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text, nil
}

func (i *helpIndex) resolveSpell(topic string) (helpTopic, bool) {
	i.once.Do(func() {
		i.spells, i.spellErr = loadSpellHelpTopics(i.root)
	})
	if i.spellErr != nil {
		return helpTopic{}, false
	}
	for _, spell := range i.spells {
		if spell.Name == topic || strings.HasPrefix(spell.Name, topic) {
			return spell, true
		}
	}
	return helpTopic{}, false
}

var (
	cDefineNumberRE = regexp.MustCompile(`(?m)^\s*#\s*define\s+([A-Z][A-Z0-9_]*)\s+(-?[0-9]+)\b`)
	spellRowRE      = regexp.MustCompile(`\{\s*"([^"]*)"\s*,\s*([A-Z][A-Z0-9_]*|-?[0-9]+)\s*,`)
)

func loadSpellHelpTopics(root string) ([]helpTopic, error) {
	symbols, err := loadCNumberDefines(filepath.Join(root, "src", "mtype.h"))
	if err != nil {
		return nil, err
	}
	source, err := readLegacyTextFile(filepath.Join(root, "src", "global.c"), "spell list source")
	if err != nil {
		return nil, err
	}

	start := strings.Index(source, "} spllist[] = {")
	if start < 0 {
		return nil, nil
	}
	body := source[start:]
	end := strings.Index(body, `		  { "@", -1,0,0 }`)
	if end >= 0 {
		body = body[:end]
	}

	var topics []helpTopic
	for _, match := range spellRowRE.FindAllStringSubmatch(body, -1) {
		name := strings.TrimSpace(match[1])
		if name == "" || name == "@" {
			continue
		}
		number, ok := parseSpellNumber(match[2], symbols)
		if !ok || number < 0 {
			continue
		}
		topics = append(topics, helpTopic{Name: name, Number: number})
	}
	return topics, nil
}

func loadCNumberDefines(path string) (map[string]int, error) {
	source, err := readLegacyTextFile(path, "C defines")
	if err != nil {
		return nil, err
	}
	defines := map[string]int{}
	for _, match := range cDefineNumberRE.FindAllStringSubmatch(source, -1) {
		value, err := strconv.Atoi(match[2])
		if err != nil {
			continue
		}
		defines[match[1]] = value
	}
	return defines, nil
}

func parseSpellNumber(value string, symbols map[string]int) (int, bool) {
	if number, err := strconv.Atoi(value); err == nil {
		return number, true
	}
	number, ok := symbols[value]
	return number, ok
}

func readLegacyTextFile(path, field string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return legacykr.ValidUTF8OrDecodeContext(legacykr.Context{Path: path, Field: field}, data)
}
