package bundle

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/0xc0de1ab/muhan/internal/engine/command/table"
	"github.com/0xc0de1ab/muhan/internal/migrate/bankmap"
	"github.com/0xc0de1ab/muhan/internal/migrate/boardmap"
	"github.com/0xc0de1ab/muhan/internal/migrate/playermap"
	"github.com/0xc0de1ab/muhan/internal/migrate/protomap"
	"github.com/0xc0de1ab/muhan/internal/migrate/roommap"
	"github.com/0xc0de1ab/muhan/internal/persist/cbin"
	"github.com/0xc0de1ab/muhan/internal/report/repairplan"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
)

type Bundle struct {
	Root            string                 `json:"root"`
	GeneratedAt     time.Time              `json:"generatedAt"`
	CommandRegistry CommandRegistrySummary `json:"commandRegistry"`
	PrototypeCounts protomap.Counts        `json:"prototypeCounts"`
	RoomMap         RoomMapSummary         `json:"roomMap"`
	PlayerMap       PlayerMapSummary       `json:"playerMap"`
	BankMap         BankMapSummary         `json:"bankMap"`
	BoardMap        BoardMapSummary        `json:"boardMap"`
	WorldLoad       WorldLoadSummary       `json:"worldLoad"`
	ObjectTotals    ObjectTotals           `json:"objectTotals"`
	CreatureTotals  CreatureTotals         `json:"creatureTotals"`
	RepairActions   RepairActionSummary    `json:"repairActions"`
	FindingCounts   FindingCounts          `json:"findingCounts"`
}

type CommandRegistrySummary struct {
	Source         string `json:"source"`
	Count          int    `json:"count"`
	RegistryCount  int    `json:"registryCount"`
	Privileged     int    `json:"privileged"`
	Special        int    `json:"special"`
	EmptyNames     int    `json:"emptyNames"`
	BrokenNames    int    `json:"brokenNames"`
	DuplicateNames int    `json:"duplicateNames"`
	HasIssues      bool   `json:"hasIssues"`
	Error          string `json:"error,omitempty"`
}

type RoomMapSummary struct {
	Files         int        `json:"files"`
	MappedRooms   int        `json:"mappedRooms"`
	Creatures     int        `json:"creatures"`
	Objects       int        `json:"objects"`
	ContentErrors int        `json:"contentErrors"`
	Decoded       cbin.Stats `json:"decoded"`
	Skipped       int        `json:"skipped"`
	Warnings      int        `json:"warnings"`
	Errors        int        `json:"errors"`
}

type PlayerMapSummary struct {
	Counts playermap.Counts `json:"counts"`
	Error  string           `json:"error,omitempty"`
}

type BankMapSummary struct {
	Counts bankmap.Counts `json:"counts"`
	Error  string         `json:"error,omitempty"`
}

type BoardMapSummary struct {
	Counts boardmap.Counts `json:"counts"`
	Error  string          `json:"error,omitempty"`
}

type WorldLoadSummary struct {
	Counts worldload.Counts `json:"counts"`
	Error  string           `json:"error,omitempty"`
}

type ObjectTotals struct {
	RoomObjects                 int `json:"roomObjects"`
	PlayerObjects               int `json:"playerObjects"`
	BankObjects                 int `json:"bankObjects"`
	ObjectInstances             int `json:"objectInstances"`
	ObjectPrototypes            int `json:"objectPrototypes"`
	SyntheticObjectPrototypes   int `json:"syntheticObjectPrototypes"`
	WorldObjectPrototypes       int `json:"worldObjectPrototypes"`
	LegacyObjectRecords         int `json:"legacyObjectRecords"`
	PrototypeResolved           int `json:"prototypeResolved"`
	PrototypeSynthetic          int `json:"prototypeSynthetic"`
	PrototypeAmbiguousSynthetic int `json:"prototypeAmbiguousSynthetic"`
}

type CreatureTotals struct {
	RoomCreatures      int `json:"roomCreatures"`
	PlayerCreatures    int `json:"playerCreatures"`
	CreaturePrototypes int `json:"creaturePrototypes"`
	Creatures          int `json:"creatures"`
}

type RepairActionSummary struct {
	Total      int            `json:"total"`
	ByKind     map[string]int `json:"byKind,omitempty"`
	BySeverity map[string]int `json:"bySeverity,omitempty"`
	Error      string         `json:"error,omitempty"`
}

type FindingCounts struct {
	Warnings   int                     `json:"warnings"`
	Errors     int                     `json:"errors"`
	Components []ComponentFindingCount `json:"components"`
}

type ComponentFindingCount struct {
	Name     string `json:"name"`
	Warnings int    `json:"warnings"`
	Errors   int    `json:"errors"`
}

var roomFileRE = regexp.MustCompile(`^r[0-9]{5}$`)

func Build(root string) (Bundle, error) {
	return build(root, time.Now().UTC())
}

func build(root string, generatedAt time.Time) (Bundle, error) {
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Bundle{}, fmt.Errorf("resolve root: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return Bundle{}, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return Bundle{}, fmt.Errorf("root is not a directory: %s", absRoot)
	}

	bundle := Bundle{
		Root:        absRoot,
		GeneratedAt: generatedAt.UTC(),
		FindingCounts: FindingCounts{
			Components: []ComponentFindingCount{},
		},
	}

	var componentCounts []ComponentFindingCount

	commandSummary, commandFindings := buildCommandRegistrySummary(absRoot)
	bundle.CommandRegistry = commandSummary
	componentCounts = append(componentCounts, commandFindings)

	protoSnapshot, err := protomap.Build(protomap.Options{Root: absRoot})
	protoFindings := ComponentFindingCount{
		Name:     "protomap",
		Warnings: len(protoSnapshot.Warnings),
		Errors:   len(protoSnapshot.Errors),
	}
	if err != nil {
		protoFindings.Errors++
	}
	bundle.PrototypeCounts = protoSnapshot.Counts
	componentCounts = append(componentCounts, protoFindings)

	bundle.RoomMap = scanRooms(absRoot)
	componentCounts = append(componentCounts, ComponentFindingCount{
		Name:     "roommap",
		Warnings: bundle.RoomMap.Warnings,
		Errors:   bundle.RoomMap.Errors,
	})

	playerReport, err := playermap.ScanRoot(absRoot)
	playerFindings := ComponentFindingCount{
		Name:     "playermap",
		Warnings: playerReport.Counts.Warnings,
		Errors:   playerReport.Counts.Errors,
	}
	if err != nil {
		bundle.PlayerMap.Error = err.Error()
		playerFindings.Errors++
	}
	bundle.PlayerMap.Counts = playerReport.Counts
	componentCounts = append(componentCounts, playerFindings)

	bankSnapshot, err := bankmap.Build(bankmap.Options{Root: absRoot, IncludeObjects: true})
	bankFindings := ComponentFindingCount{
		Name:     "bankmap",
		Warnings: bankSnapshot.Counts.Warnings,
		Errors:   bankSnapshot.Counts.Errors,
	}
	if err != nil {
		bundle.BankMap.Error = err.Error()
		bankFindings.Errors++
	}
	bundle.BankMap.Counts = bankSnapshot.Counts
	componentCounts = append(componentCounts, bankFindings)

	boardReport, err := boardmap.ScanRoot(absRoot)
	boardFindings := ComponentFindingCount{
		Name:     "boardmap",
		Warnings: boardReport.Counts.Warnings,
		Errors:   boardReport.Counts.Errors,
	}
	if err != nil {
		bundle.BoardMap.Error = err.Error()
		boardFindings.Errors++
	}
	bundle.BoardMap.Counts = boardReport.Counts
	componentCounts = append(componentCounts, boardFindings)

	loadSummary, err := worldload.LoadRoot(absRoot)
	loadFindings := ComponentFindingCount{
		Name:     "worldload",
		Warnings: loadSummary.Counts.Warnings,
		Errors:   loadSummary.Counts.Errors,
	}
	if err != nil {
		bundle.WorldLoad.Error = err.Error()
		loadFindings.Errors++
	}
	bundle.WorldLoad.Counts = loadSummary.Counts
	componentCounts = append(componentCounts, loadFindings)

	repairSummary, repairFindings := buildRepairActionSummary(absRoot)
	bundle.RepairActions = repairSummary
	componentCounts = append(componentCounts, repairFindings)

	bundle.ObjectTotals = ObjectTotals{
		RoomObjects:                 bundle.RoomMap.Objects,
		PlayerObjects:               bundle.PlayerMap.Counts.InventoryObjects,
		BankObjects:                 bundle.BankMap.Counts.Objects,
		ObjectInstances:             bundle.WorldLoad.Counts.ObjectInstances,
		ObjectPrototypes:            bundle.PrototypeCounts.ObjectPrototypes,
		SyntheticObjectPrototypes:   bundle.WorldLoad.Counts.SyntheticObjectPrototypes,
		WorldObjectPrototypes:       bundle.WorldLoad.Counts.ObjectPrototypes,
		LegacyObjectRecords:         bundle.WorldLoad.Counts.ObjectInstances + bundle.PrototypeCounts.ObjectPrototypes,
		PrototypeResolved:           bundle.WorldLoad.Counts.PrototypeResolved,
		PrototypeSynthetic:          bundle.WorldLoad.Counts.PrototypeSynthetic,
		PrototypeAmbiguousSynthetic: bundle.WorldLoad.Counts.PrototypeAmbiguous,
	}
	bundle.CreatureTotals = CreatureTotals{
		RoomCreatures:      bundle.WorldLoad.Counts.RoomCreatures,
		PlayerCreatures:    bundle.PlayerMap.Counts.Decoded.Creatures,
		CreaturePrototypes: bundle.PrototypeCounts.CreaturePrototypes,
		Creatures:          bundle.WorldLoad.Counts.Creatures,
	}

	bundle.FindingCounts.Components = componentCounts
	for _, counts := range componentCounts {
		bundle.FindingCounts.Warnings += counts.Warnings
		bundle.FindingCounts.Errors += counts.Errors
	}

	return bundle, nil
}

func EncodeJSON(w io.Writer, bundle Bundle) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(bundle)
}

func buildCommandRegistrySummary(root string) (CommandRegistrySummary, ComponentFindingCount) {
	source := filepath.Join(root, "src", "global.c")
	registry, entries, err := table.LoadLegacyRegistry(root)
	analysis := table.Analyze(entries)

	summary := CommandRegistrySummary{
		Source:         source,
		Count:          len(entries),
		RegistryCount:  len(registry.Commands()),
		Privileged:     analysis.Privileged.Count,
		Special:        analysis.Special.Count,
		EmptyNames:     len(analysis.EmptyNames),
		BrokenNames:    len(analysis.BrokenNames),
		DuplicateNames: len(analysis.DuplicateNames),
		HasIssues:      analysis.HasIssues(),
	}

	findings := ComponentFindingCount{
		Name:     "commandRegistry",
		Warnings: summary.EmptyNames + summary.BrokenNames + summary.DuplicateNames,
	}
	if err != nil {
		summary.Error = err.Error()
		findings.Errors = 1
	}
	return summary, findings
}

func scanRooms(root string) RoomMapSummary {
	base := filepath.Join(root, "rooms")
	summary := RoomMapSummary{}
	info, err := os.Stat(base)
	if err != nil {
		if os.IsNotExist(err) {
			summary.Warnings++
			return summary
		}
		summary.Errors++
		return summary
	}
	if !info.IsDir() {
		summary.Errors++
		return summary
	}

	_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			summary.Errors++
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !roomFileRE.MatchString(d.Name()) {
			summary.Skipped++
			return nil
		}

		summary.Files++
		data, err := os.ReadFile(path)
		if err != nil {
			summary.Errors++
			return nil
		}

		bundle, err := roommap.MapRoomFileBundle(path, data)
		if err != nil {
			summary.Errors++
		} else {
			summary.MappedRooms++
			summary.Creatures += len(bundle.Creatures)
			summary.Objects += len(bundle.Objects)
			if bundle.ContentError != "" {
				summary.ContentErrors++
			} else {
				mergeStats(&summary.Decoded, bundle.Decoded)
			}
			summary.Warnings += len(bundle.Warnings)
		}
		return nil
	})
	return summary
}

func mergeStats(dst *cbin.Stats, src cbin.Stats) {
	dst.Objects += src.Objects
	dst.Creatures += src.Creatures
	dst.Rooms += src.Rooms
	dst.Exits += src.Exits
	dst.Descriptions += src.Descriptions
	dst.DescriptionBytes += src.DescriptionBytes
	dst.TrailingBytes += src.TrailingBytes
	if src.MaxDepth > dst.MaxDepth {
		dst.MaxDepth = src.MaxDepth
	}
}

func buildRepairActionSummary(root string) (RepairActionSummary, ComponentFindingCount) {
	plan, err := repairplan.Generate(root)
	summary := RepairActionSummary{
		ByKind:     map[string]int{},
		BySeverity: map[string]int{},
	}
	findings := ComponentFindingCount{Name: "repairplan"}
	if err != nil {
		summary.Error = err.Error()
		findings.Errors = 1
		return summary, findings
	}

	summary.Total = len(plan.Actions)
	for _, action := range plan.Actions {
		summary.ByKind[action.Kind]++
		summary.BySeverity[action.Severity]++
	}
	if len(summary.ByKind) == 0 {
		summary.ByKind = nil
	}
	if len(summary.BySeverity) == 0 {
		summary.BySeverity = nil
	}
	return summary, findings
}
