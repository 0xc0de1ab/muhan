package command

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"muhan/internal/persist/legacykr"
	"muhan/internal/textfmt"
	"muhan/internal/world/model"
)

const (
	legacyDescriptionProperty = "description"
	legacyDescriptionMaxBytes = 60
)

var ErrDescriptionSetterRequired = errors.New("description setter required")

type DescriptionWorld interface {
	InventoryWorld
}

type creatureDescriptionSetter interface {
	SetCreatureDescription(model.CreatureID, string) (model.Creature, error)
}

type creaturePropertySetter interface {
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
}

func NewDescriptionHandler(world DescriptionWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		playerID := InventoryPlayerIDFromContext(ctx)
		if playerID.IsZero() {
			return StatusDefault, ErrInventoryActorRequired
		}
		_, creature, err := CurrentInventoryCreature(world, playerID)
		if err != nil {
			return StatusDefault, err
		}

		description := legacyDescriptionText(resolved)
		if description == "" {
			if err := setCreatureDescription(world, creature.ID, ""); err != nil {
				return StatusDefault, err
			}
			ctx.WriteString("당신은 서 있습니다.")
			return StatusDefault, nil
		}
		if legacyDescriptionByteLen(legacyDescriptionInputForLength(resolved, description)) > legacyDescriptionMaxBytes {
			ctx.WriteString("묘사가 너무 깁니다.")
			return StatusDefault, nil
		}

		stored := description + " "
		if err := setCreatureDescription(world, creature.ID, stored); err != nil {
			return StatusDefault, err
		}
		rendered := textfmt.RenderLegacyColors(stored, textOptionsFromContext(ctx))
		ctx.WriteString(fmt.Sprintf("당신은 이제부터 %s있습니다.", rendered))
		return StatusDefault, nil
	}
}

func setCreatureDescription(world DescriptionWorld, creatureID model.CreatureID, description string) error {
	if setter, ok := world.(creatureDescriptionSetter); ok {
		_, err := setter.SetCreatureDescription(creatureID, description)
		return err
	}
	if setter, ok := world.(creaturePropertySetter); ok {
		if description == "" {
			description = " "
		}
		_, err := setter.SetCreatureProperty(creatureID, legacyDescriptionProperty, description)
		return err
	}
	return ErrDescriptionSetterRequired
}

func legacyDescriptionText(resolved ResolvedCommand) string {
	if text := legacyDescriptionTextFromInput(resolved); text != "" {
		return text
	}
	return joinArgs(resolved.Args)
}

func legacyDescriptionTextFromInput(resolved ResolvedCommand) string {
	input := strings.TrimSpace(resolved.Input)
	if input == "" {
		return ""
	}

	for _, command := range dmCommandNameCandidates(resolved) {
		if stripped, ok := stripCommandAtTextEdge(input, command); ok {
			return stripped
		}
	}
	return ""
}

func legacyDescriptionInputForLength(resolved ResolvedCommand, description string) string {
	if input := strings.TrimSpace(resolved.Input); input != "" {
		return input
	}
	if command := strings.TrimSpace(resolved.Command()); command != "" {
		return strings.TrimSpace(description + " " + command)
	}
	return strings.TrimSpace(description + " 묘사")
}

func legacyDescriptionByteLen(text string) int {
	encoded, err := legacykr.EncodeEUCKR(text)
	if err != nil {
		return len([]byte(text))
	}
	return len(encoded)
}

func textAfterFirstToken(text string) (string, bool) {
	inToken := false
	for i, r := range text {
		if unicode.IsSpace(r) {
			if inToken {
				return text[i:], true
			}
			continue
		}
		inToken = true
	}
	return "", false
}

func textBeforeLastToken(text string) (string, bool) {
	inToken := false
	for i := len(text); i > 0; {
		r, size := utf8.DecodeLastRuneInString(text[:i])
		i -= size
		if unicode.IsSpace(r) {
			if inToken {
				return text[:i], true
			}
			continue
		}
		inToken = true
	}
	return "", false
}
