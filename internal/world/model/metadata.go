package model

type Metadata struct {
	Source              string                       `json:"source,omitempty"`
	LegacyKind          string                       `json:"legacyKind,omitempty"`
	LegacyID            string                       `json:"legacyId,omitempty"`
	LegacyPath          string                       `json:"legacyPath,omitempty"`
	LegacyEncoding      string                       `json:"legacyEncoding,omitempty"`
	RecordIndex         int                          `json:"recordIndex,omitempty"`
	RecordOffset        int64                        `json:"recordOffset,omitempty"`
	ObjectTreePath      string                       `json:"objectTreePath,omitempty"`
	RawFields           map[string][]byte            `json:"rawFields,omitempty"`
	Tags                []string                     `json:"tags,omitempty"`
	Notes               []string                     `json:"notes,omitempty"`
	PrototypeResolution *PrototypeResolutionMetadata `json:"prototypeResolution,omitempty"`
}

type PrototypeResolutionMetadata struct {
	Status                           string                         `json:"status,omitempty"`
	Method                           string                         `json:"method,omitempty"`
	Confidence                       string                         `json:"confidence,omitempty"`
	SelectedPrototypeID              PrototypeID                    `json:"selectedPrototypeId,omitempty"`
	SyntheticPrototypeID             PrototypeID                    `json:"syntheticPrototypeId,omitempty"`
	CandidateCount                   int                            `json:"candidateCount,omitempty"`
	Candidates                       []PrototypeResolutionCandidate `json:"candidates,omitempty"`
	Fingerprint                      string                         `json:"fingerprint,omitempty"`
	FingerprintAlgorithm             string                         `json:"fingerprintAlgorithm,omitempty"`
	ComparableBytes                  int                            `json:"comparableBytes,omitempty"`
	MaterializedFromObjectInstanceID ObjectInstanceID               `json:"materializedFromObjectInstanceId,omitempty"`
}

type PrototypeResolutionCandidate struct {
	PrototypeID  PrototypeID `json:"prototypeId"`
	Path         string      `json:"path,omitempty"`
	Index        int         `json:"index"`
	LegacyNumber int         `json:"legacyNumber"`
	RecordOffset int64       `json:"recordOffset"`
}
