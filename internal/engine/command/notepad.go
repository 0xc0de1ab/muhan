package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"muhan/internal/persist/legacykr"
	"muhan/internal/world/model"
)

var notepadMu sync.Mutex

// DMNotepadWorld defines the world dependencies for the notepad command.
type DMNotepadWorld interface {
	Player(model.PlayerID) (model.Player, bool)
	Creature(model.CreatureID) (model.Creature, bool)
}

// NewNotepadHandler creates a new Handler for the notepad command.
func NewNotepadHandler(world DMNotepadWorld, root string) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		return notepad(ctx, resolved, world, root)
	}
}

func notepad(ctx *Context, resolved ResolvedCommand, world DMNotepadWorld, root string) (Status, error) {
	if ctx == nil || strings.TrimSpace(ctx.ActorID) == "" || world == nil {
		return StatusDefault, nil
	}

	playerID := model.PlayerID(ctx.ActorID)
	var creatureID model.CreatureID
	if player, ok := world.Player(playerID); ok {
		creatureID = player.CreatureID
	} else {
		creatureID = model.CreatureID(ctx.ActorID)
	}

	creature, ok := world.Creature(creatureID)
	if !ok {
		return StatusDefault, nil
	}

	class := creatureClass(creature)
	if class < legacyClassCaretaker {
		return StatusPrompt, nil
	}

	filePath := filepath.Join(root, "post", "DM_pad")

	if len(resolved.Args) == 1 {
		arg := resolved.Args[0]
		if strings.HasPrefix(strings.ToLower(arg), "a") {
			ctx.WriteString("DM notepad:\n->")
			state := &notepadState{filePath: filePath}
			if !SetPendingLineHandler(ctx, state.handleLine) {
				return StatusDefault, fmt.Errorf("notepad: failed to set pending line handler")
			}
			return StatusDoPrompt, nil
		} else if strings.HasPrefix(strings.ToLower(arg), "d") {
			notepadMu.Lock()
			defer notepadMu.Unlock()
			_ = os.Remove(filePath)
			ctx.WriteString("Clearing DM notepad\n")
			return StatusPrompt, nil
		} else {
			ctx.WriteString("invalid option.\n")
			return StatusPrompt, nil
		}
	}

	// No arguments or other argument count (e.g. 0 arguments or >1 arguments)
	notepadMu.Lock()
	defer notepadMu.Unlock()

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return StatusDoPrompt, nil
		}
		return StatusDefault, err
	}

	text, err := legacykr.ValidUTF8OrDecodeContext(legacykr.Context{Path: filePath, Field: "notepad"}, data)
	if err != nil {
		return StatusDefault, err
	}
	return renderLegacyViewFile(ctx, text, "DM 메모 읽기 상태를 시작할 수 없습니다")
}

type notepadState struct {
	filePath string
}

func (s *notepadState) handleLine(ctx *Context, line string) (Status, error) {
	if strings.HasPrefix(line, ".") {
		ClearPendingLineHandler(ctx)
		ctx.WriteString("Message appended.\n")
		return StatusDefault, nil
	}

	notepadMu.Lock()
	defer notepadMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.filePath), 0o755); err != nil {
		return StatusDefault, err
	}

	_, err := os.Stat(s.filePath)
	if os.IsNotExist(err) {
		header := "            === DM Notepad ===\n\n"
		if err := os.WriteFile(s.filePath, []byte(header), 0o644); err != nil {
			return StatusDefault, err
		}
	}

	f, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return StatusDefault, err
	}
	defer f.Close()

	truncated := firstNBytes(line, 79)
	if _, err := f.WriteString(truncated + "\n"); err != nil {
		return StatusDefault, err
	}

	ctx.WriteString("->")
	if !SetPendingLineHandler(ctx, s.handleLine) {
		return StatusDefault, fmt.Errorf("notepad: failed to reset pending line handler")
	}
	return StatusDoPrompt, nil
}
