package command

import (
	"slices"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

var legacyReadySlotOrder = []string{
	"body",
	"arms",
	"legs",
	"neck1",
	"neck2",
	"hands",
	"head",
	"feet",
	"finger1",
	"finger2",
	"finger3",
	"finger4",
	"finger5",
	"finger6",
	"finger7",
	"finger8",
	"held",
	"shield",
	"face",
	"wield",
}

var legacyEquipmentDisplaySlotOrder = []string{
	"head",
	"face",
	"neck1",
	"neck2",
	"body",
	"arms",
	"hands",
	"finger1",
	"finger2",
	"finger3",
	"finger4",
	"finger5",
	"finger6",
	"finger7",
	"finger8",
	"legs",
	"feet",
	"held",
	"shield",
	"wield",
}

func orderedEquipmentSlots(equipment map[string]model.ObjectInstanceID, preferred []string) []string {
	if len(equipment) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(preferred))
	slots := make([]string, 0, len(equipment))
	for _, slot := range preferred {
		seen[slot] = struct{}{}
		if objectID := equipment[slot]; strings.TrimSpace(slot) != "" && !objectID.IsZero() {
			slots = append(slots, slot)
		}
	}

	extra := make([]string, 0, len(equipment)-len(slots))
	for slot, objectID := range equipment {
		if _, ok := seen[slot]; ok {
			continue
		}
		if strings.TrimSpace(slot) == "" || objectID.IsZero() {
			continue
		}
		extra = append(extra, slot)
	}
	slices.Sort(extra)
	return append(slots, extra...)
}
