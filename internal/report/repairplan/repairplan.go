package repairplan

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/0xc0de1ab/muhan/internal/report/dataissues"
)

const (
	ActionRestoreOrDelete                       = "restore_or_delete"
	ActionRestoreOrTruncateDescriptionCandidate = "restore_or_truncate_description_candidate"
	ActionRestoreFromBackupRequired             = "restore_from_backup_required"
	ActionManualReviewRequired                  = "manual_review_required"
)

type Plan struct {
	Root        string    `json:"root"`
	GeneratedAt time.Time `json:"generatedAt"`
	Actions     []Action  `json:"actions"`
}

type Action struct {
	Path              string `json:"path"`
	Kind              string `json:"kind"`
	Severity          string `json:"severity"`
	Problem           string `json:"problem"`
	RecommendedAction string `json:"recommendedAction"`
	Rationale         string `json:"rationale"`
}

func Generate(root string) (Plan, error) {
	report, err := dataissues.Scan(root)
	if err != nil {
		return Plan{}, err
	}
	return FromReport(report, time.Now().UTC()), nil
}

func FromReport(report dataissues.Report, generatedAt time.Time) Plan {
	actions := make([]Action, 0, len(report.Issues))
	for _, issue := range report.Issues {
		actions = append(actions, actionForIssue(issue))
	}

	return Plan{
		Root:        report.Root,
		GeneratedAt: generatedAt,
		Actions:     actions,
	}
}

func EncodeJSON(w io.Writer, plan Plan) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(plan)
}

func WriteText(w io.Writer, plan Plan) {
	fmt.Fprintf(w, "root: %s\n", plan.Root)
	fmt.Fprintf(w, "generatedAt: %s\n", plan.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(w, "actions: %d\n", len(plan.Actions))
	for _, action := range plan.Actions {
		fmt.Fprintf(w, "- %s %s %s: %s\n",
			action.Severity,
			action.Kind,
			action.Path,
			strings.TrimSpace(action.Problem),
		)
		fmt.Fprintf(w, "  recommendedAction: %s\n", action.RecommendedAction)
		fmt.Fprintf(w, "  rationale: %s\n", action.Rationale)
	}
}

func actionForIssue(issue dataissues.Issue) Action {
	kind, recommendedAction, rationale := classifyIssue(issue)
	return Action{
		Path:              issue.Path,
		Kind:              kind,
		Severity:          issue.Severity,
		Problem:           issue.Message,
		RecommendedAction: recommendedAction,
		Rationale:         rationale,
	}
}

func classifyIssue(issue dataissues.Issue) (string, string, string) {
	switch {
	case isZeroByteRoom(issue):
		return ActionRestoreOrDelete,
			"Restore a valid room file from backup, or confirm the room should be deleted/regenerated before migration.",
			"The room file is 0 bytes, so migration cannot reconstruct a room record without a human data decision."
	case isDescriptionEOF(issue):
		return ActionRestoreOrTruncateDescriptionCandidate,
			"Restore the complete room file from backup; if no backup exists, inspect the room and only then truncate or remove the incomplete description.",
			"The length-prefixed room description claims more bytes than remain in the file, so the intended text boundary needs human confirmation."
	case isTruncatedNestedObject(issue):
		return ActionRestoreFromBackupRequired,
			"Restore the affected file from a known-good backup before migration.",
			"A nested object record or child-count field is incomplete, so object containment cannot be trusted from the current bytes."
	default:
		return ActionManualReviewRequired,
			"Inspect the source file and decide on a repair before migration.",
			"dataissues reported a decode or validation problem that does not match a narrower repair rule."
	}
}

func isZeroByteRoom(issue dataissues.Issue) bool {
	return issue.Kind == dataissues.KindRoom &&
		(issue.Size == 0 || strings.Contains(issue.Hint, "0-byte room"))
}

func isDescriptionEOF(issue dataissues.Issue) bool {
	return strings.Contains(issue.Hint, "length-prefixed description EOF") ||
		(strings.Contains(issue.Message, "room description") &&
			strings.Contains(issue.Message, "bytes: need") &&
			strings.Contains(issue.Message, "remaining"))
}

func isTruncatedNestedObject(issue dataissues.Issue) bool {
	if strings.Contains(issue.Hint, "truncated nested object") {
		return true
	}
	return (strings.Contains(issue.Message, "object record: need") ||
		strings.Contains(issue.Message, "object child count: need int32")) &&
		(strings.Contains(issue.Message, "room object") ||
			strings.Contains(issue.Message, "creature inventory object") ||
			strings.Contains(issue.Message, "object child"))
}
