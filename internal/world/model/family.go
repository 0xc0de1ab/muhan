package model

// Family is a legacy family registry row loaded from player/family/family_list.
// ID is the legacy family_num value; Slot is the family_str/fmboss_str array
// index used by C callers.
type Family struct {
	ID          int            `json:"id"`
	Slot        int            `json:"slot"`
	DisplayName string         `json:"displayName"`
	BossName    string         `json:"bossName,omitempty"`
	JoinSubsidy int            `json:"joinSubsidy,omitempty"`
	Members     []FamilyMember `json:"members,omitempty"`
	Metadata    Metadata       `json:"metadata,omitempty"`
}

type FamilyMember struct {
	DisplayName string   `json:"displayName"`
	Class       int      `json:"class"`
	Metadata    Metadata `json:"metadata,omitempty"`
}
