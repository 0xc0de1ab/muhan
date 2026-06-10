package state

import (
	"log"
	"time"
)

// MarkBoardDirty marks a board (by its legacy dir name) as having runtime post changes
// (new post or delete toggle). Used for Package C JSON sidecar dirty flush.
func (w *World) MarkBoardDirty(boardDir string) {
	if boardDir == "" {
		return
	}
	w.dirtyMu.Lock()
	if w.boardDirty == nil {
		w.boardDirty = make(map[string]int64)
	}
	w.boardDirty[boardDir] = time.Now().Unix()
	w.dirtyMu.Unlock()
}

// QueueBoardSave enqueues board posts / family news sidecar save (C Package).
// Non-blocking best effort; falls back to direct SaveBoardPosts (family requires direct SaveFamilyNews from site with content).
func (w *World) QueueBoardSave(boardDir string, familyID int) {
	select {
	case w.saveQueue <- saveRequest{boardDir: boardDir, familyID: familyID}:
	default:
		log.Printf("[PERSIST] WARN QueueBoardSave fallback sync (queue full) for board=%s fam=%d", boardDir, familyID)
		if boardDir != "" {
			if err := w.SaveBoardPosts(boardDir); err != nil {
				log.Printf("[PERSIST] ERROR fallback SaveBoardPosts %s: %v", boardDir, err)
			}
		}

	}
}
