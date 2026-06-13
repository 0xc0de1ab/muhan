package load

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/persist/legacykr"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

const (
	legacyFamilyMax      = 16
	legacyFamilySentinel = 16
)

var legacyFamilyMemberFileRE = regexp.MustCompile(`^family_member_([0-9]+)$`)

func loadFamilies(root string, summary *Summary) {
	base := filepath.Join(root, "player", "family")
	listPath := filepath.Join(base, "family_list")
	data, err := os.ReadFile(listPath)
	if err != nil {
		if !os.IsNotExist(err) {
			summary.addWarning("read_family", displayPath(root, listPath), "", "", err.Error())
		}
		return
	}

	relPath := displayPath(root, listPath)
	families, warnings := parseFamilyList(relPath, data)
	for _, warning := range warnings {
		summary.addWarning("map_family", relPath, "", "", warning)
	}

	knownSlots := map[int]struct{}{}
	for _, family := range families {
		knownSlots[family.Slot] = struct{}{}
	}
	membersBySlot := loadFamilyMembers(root, base, knownSlots, summary)
	for i := range families {
		families[i].Members = slices.Clone(membersBySlot[families[i].Slot])
		summary.Counts.FamilyMembers += len(families[i].Members)
	}

	slices.SortStableFunc(families, func(a, b model.Family) int {
		if a.Slot != b.Slot {
			return a.Slot - b.Slot
		}
		return a.ID - b.ID
	})
	for _, family := range families {
		if err := summary.World.AddFamily(family); err != nil {
			summary.addError("add_family", family.Metadata.LegacyPath, strconv.Itoa(family.ID), "", err.Error())
		}
	}
}

func parseFamilyList(path string, data []byte) ([]model.Family, []string) {
	var families []model.Family
	var warnings []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNumber := 0
	slot := 0
	for scanner.Scan() {
		lineNumber++
		rawLine := slices.Clone(scanner.Bytes())
		line, err := decodeLegacyLine(path, "family_list", rawLine)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("line %d: %v", lineNumber, err))
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		id, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		if id == legacyFamilySentinel {
			break
		}
		if len(fields) < 4 {
			warnings = append(warnings, fmt.Sprintf("line %d: family record needs number name boss subsidy", lineNumber))
			continue
		}
		joinSubsidy, err := strconv.Atoi(fields[3])
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("line %d: invalid family join subsidy %q", lineNumber, fields[3]))
			continue
		}
		families = append(families, model.Family{
			ID:          id,
			Slot:        slot,
			DisplayName: fields[1],
			BossName:    fields[2],
			JoinSubsidy: joinSubsidy,
			Metadata: model.Metadata{
				Source:         "legacy",
				LegacyKind:     "family",
				LegacyID:       strconv.Itoa(id),
				LegacyPath:     path,
				LegacyEncoding: "euc-kr/cp949",
				RecordIndex:    slot,
				RawFields: map[string][]byte{
					"line": rawLine,
				},
			},
		})
		slot++
		if slot >= legacyFamilyMax {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		warnings = append(warnings, err.Error())
	}
	return families, warnings
}

func loadFamilyMembers(root, base string, knownSlots map[int]struct{}, summary *Summary) map[int][]model.FamilyMember {
	entries, err := os.ReadDir(base)
	if err != nil {
		if !os.IsNotExist(err) {
			summary.addWarning("read_family_members", displayPath(root, base), "", "", err.Error())
		}
		return nil
	}
	sortDirEntries(entries)

	membersBySlot := map[int][]model.FamilyMember{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := legacyFamilyMemberFileRE.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		slot, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		path := filepath.Join(base, entry.Name())
		relPath := displayPath(root, path)
		data, err := os.ReadFile(path)
		if err != nil {
			summary.addWarning("read_family_member", relPath, strconv.Itoa(slot), "", err.Error())
			continue
		}
		summary.Counts.FamilyMemberFiles++
		members, warnings := parseFamilyMemberFile(relPath, slot, data)
		for _, warning := range warnings {
			summary.addWarning("map_family_member", relPath, strconv.Itoa(slot), "", warning)
		}
		membersBySlot[slot] = members
		if _, ok := knownSlots[slot]; !ok && len(members) > 0 {
			summary.addWarning("map_family_member", relPath, strconv.Itoa(slot), "",
				fmt.Sprintf("member file has %d members but no family_list record for slot %d", len(members), slot))
		}
	}
	return membersBySlot
}

func parseFamilyMemberFile(path string, slot int, data []byte) ([]model.FamilyMember, []string) {
	var members []model.FamilyMember
	var warnings []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNumber := 0
	recordIndex := 0
	for scanner.Scan() {
		lineNumber++
		rawLine := slices.Clone(scanner.Bytes())
		line, err := decodeLegacyLine(path, "family_member", rawLine)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("line %d: %v", lineNumber, err))
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		classID, err := strconv.Atoi(fields[0])
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("line %d: invalid member class %q", lineNumber, fields[0]))
			continue
		}
		if classID == 0 {
			break
		}
		if len(fields) < 2 {
			warnings = append(warnings, fmt.Sprintf("line %d: family member record needs class and name", lineNumber))
			continue
		}
		members = append(members, model.FamilyMember{
			Class:       classID,
			DisplayName: fields[1],
			Metadata: model.Metadata{
				Source:         "legacy",
				LegacyKind:     "family_member",
				LegacyID:       fmt.Sprintf("%d:%d", slot, recordIndex),
				LegacyPath:     path,
				LegacyEncoding: "euc-kr/cp949",
				RecordIndex:    recordIndex,
				RawFields: map[string][]byte{
					"line": rawLine,
				},
			},
		})
		recordIndex++
	}
	if err := scanner.Err(); err != nil {
		warnings = append(warnings, err.Error())
	}
	return members, warnings
}

func decodeLegacyLine(path, field string, raw []byte) (string, error) {
	line, err := legacykr.ValidUTF8OrDecodeContext(legacykr.Context{Path: path, Field: field}, raw)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
