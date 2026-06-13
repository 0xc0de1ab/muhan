package game

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	enginecmd "github.com/0xc0de1ab/muhan/internal/engine/command"
	"github.com/0xc0de1ab/muhan/internal/persist/jsonstore"
	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
	"github.com/0xc0de1ab/muhan/internal/world/model"
	"github.com/0xc0de1ab/muhan/internal/world/state"
)

const familyNewsHeader = "                      === 패거리 공지 ===\n\n"

// NewFamilyNewsHandler renders and edits the legacy family_news/패거리공지 notice.
func NewFamilyNewsHandler(world FamilyWorld, root string) enginecmd.Handler {
	return func(ctx *enginecmd.Context, resolved enginecmd.ResolvedCommand) (enginecmd.Status, error) {
		if ctx == nil || ctx.ActorID == "" {
			return enginecmd.StatusDefault, ErrSocialActorRequired
		}

		familyID, ok := playerFamilyMembership(world, model.PlayerID(ctx.ActorID))
		if !ok {
			ctx.WriteString("당신은 패거리에 가입되어 있지 않습니다.")
			return enginecmd.StatusDefault, nil
		}
		if familyID <= 0 {
			ctx.WriteString("잘못된 패거리입니다.\n")
			return enginecmd.StatusDefault, nil
		}
		args := familyNewsArgs(resolved.Args)
		if len(args) == 1 {
			switch familyNewsOption(args[0]) {
			case 'a':
				edit := &familyNewsEditState{
					world:    world,
					root:     root,
					familyID: familyID,
					path:     familyNewsPath(root, familyID),
				}
				ctx.WriteString("패거리 공지:\n->")
				if !enginecmd.SetPendingLineHandler(ctx, edit.handleLine) {
					return enginecmd.StatusDefault, fmt.Errorf("패거리 공지 입력 상태를 시작할 수 없습니다")
				}
				return enginecmd.StatusDoPrompt, nil
			case 'd':
				creature, ok := playerCreature(world, model.PlayerID(ctx.ActorID))
				if !ok || !familyMembershipCreatureIsBoss(creature) {
					ctx.WriteString("삭제는 두목만 가능합니다.")
					return enginecmd.StatusPrompt, nil
				}
				if err := deleteFamilyNews(root, familyID); err != nil {
					return enginecmd.StatusDefault, err
				}
				ctx.WriteString("공지 내용을 지웠습니다.\n")
				return enginecmd.StatusPrompt, nil
			default:
				ctx.WriteString("잘못된 옵션입니다.\n")
				return enginecmd.StatusPrompt, nil
			}
		}

		text, ok, err := readFamilyNews(root, familyID)
		if err != nil {
			return enginecmd.StatusDefault, err
		}
		if !ok {
			ctx.WriteString("패거리의 공지사항이 없습니다.")
			return enginecmd.StatusDoPrompt, nil
		}
		return enginecmd.RenderLegacyViewFile(ctx, text, "패거리 공지 읽기 상태를 시작할 수 없습니다")
	}
}

type familyNewsEditState struct {
	world    FamilyWorld
	root     string
	familyID int
	path     string
}

func (s *familyNewsEditState) handleLine(ctx *enginecmd.Context, line string) (enginecmd.Status, error) {
	if strings.HasPrefix(line, ".") {
		enginecmd.ClearPendingLineHandler(ctx)
		ctx.WriteString("공지를 남겼습니다.\n")
		return enginecmd.StatusDefault, nil
	}
	if err := appendFamilyNewsLine(s.world, s.root, s.familyID, s.path, line); err != nil {
		return enginecmd.StatusDefault, err
	}
	ctx.WriteString("->")
	if !enginecmd.SetPendingLineHandler(ctx, s.handleLine) {
		return enginecmd.StatusDefault, fmt.Errorf("패거리 공지 입력 상태를 계속할 수 없습니다")
	}
	return enginecmd.StatusDoPrompt, nil
}

func familyNewsPath(root string, familyID int) string {
	return filepath.Join(strings.TrimSpace(root), "player", "family", fmt.Sprintf("family_news_%d", familyID))
}

func familyNewsOption(arg string) byte {
	arg = strings.TrimSpace(strings.ToLower(arg))
	if arg == "" {
		return 0
	}
	switch arg[0] {
	case 'a':
		return 'a'
	case 'd':
		return 'd'
	default:
		return 0
	}
}

func familyNewsArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			filtered = append(filtered, arg)
		}
	}
	return filtered
}

func readFamilyNews(root string, familyID int) (string, bool, error) {
	path := familyNewsPath(root, familyID)
	text, err := readFamilyNewsLegacyText(path)
	if err == nil {
		return text, true, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}
	text, ok, err := readFamilyNewsSidecar(root, familyID)
	if err != nil || !ok || strings.TrimSpace(text) == "" {
		return "", false, err
	}
	return text, true, nil
}

func appendFamilyNewsLine(world FamilyWorld, root string, familyID int, path string, line string) error {
	if err := ensureFamilyNewsFile(root, familyID, path); err != nil {
		return err
	}
	line = familyNewsTrimLine(line)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open family news %q: %w", path, err)
	}
	if _, err := file.WriteString(line + "\n"); err != nil {
		_ = file.Close()
		return fmt.Errorf("append family news %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close family news %q: %w", path, err)
	}
	text, err := readFamilyNewsLegacyText(path)
	if err != nil {
		return err
	}
	return writeFamilyNewsSidecar(world, root, familyID, text)
}

func ensureFamilyNewsFile(root string, familyID int, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir family news dir: %w", err)
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if utf8.Valid(data) {
			return nil
		}
		text, err := legacykr.ValidUTF8OrDecodeContext(legacykr.Context{Path: path, Field: "family_news"}, data)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return fmt.Errorf("rewrite family news %q as utf-8: %w", path, err)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read family news %q: %w", path, err)
	}
	content := familyNewsHeader
	if sidecar, ok, err := readFamilyNewsSidecar(root, familyID); err != nil {
		return err
	} else if ok && strings.TrimSpace(sidecar) != "" {
		content = sidecar
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("create family news %q: %w", path, err)
	}
	return nil
}

func familyNewsTrimLine(line string) string {
	line = strings.TrimRight(line, "\r\n")
	runes := []rune(line)
	if len(runes) > 79 {
		runes = runes[:79]
	}
	return string(runes)
}

func readFamilyNewsLegacyText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		return "", fmt.Errorf("read family news %q: %w", path, err)
	}
	text, err := legacykr.ValidUTF8OrDecodeContext(legacykr.Context{Path: path, Field: "family_news"}, data)
	if err != nil {
		return "", err
	}
	return text, nil
}

func familyNewsSidecarPath(root string, familyID int) string {
	return filepath.Join(strings.TrimSpace(root), "player", "family", "json", fmt.Sprintf("family_news_%d.json", familyID))
}

func readFamilyNewsSidecar(root string, familyID int) (string, bool, error) {
	save, ok, err := state.LoadFamilyNews(root, familyID)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	if save.FamilyID != 0 && save.FamilyID != familyID {
		return "", false, fmt.Errorf("family news sidecar %q has familyID %d, want %d", familyNewsSidecarPath(root, familyID), save.FamilyID, familyID)
	}
	return save.Content, true, nil
}

func writeFamilyNewsSidecar(world FamilyWorld, root string, familyID int, content string) error {
	if saver, ok := world.(interface {
		MarkFamilyNewsDirty(int)
		SaveFamilyNews(int, string) error
	}); ok {
		saver.MarkFamilyNewsDirty(familyID)
		if err := saver.SaveFamilyNews(familyID, content); err == nil {
			return nil
		} else if dbRooter, ok := world.(interface{ DBRoot() string }); !ok || strings.TrimSpace(dbRooter.DBRoot()) != "" {
			return err
		}
	}

	path := familyNewsSidecarPath(root, familyID)
	save := state.FamilyNewsSave{
		SchemaVersion: state.CurrentSaveSchemaVersion,
		FamilyID:      familyID,
		Content:       content,
		UpdatedAt:     time.Now(),
	}
	if err := jsonstore.WriteJSON(path, save); err != nil {
		return fmt.Errorf("save family news sidecar %q: %w", path, err)
	}
	return nil
}

func deleteFamilyNews(root string, familyID int) error {
	for _, path := range []string{familyNewsPath(root, familyID), familyNewsSidecarPath(root, familyID)} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete family news %q: %w", path, err)
		}
	}
	return nil
}
