package model

import (
	"encoding/json"
	"testing"
)

func TestObjectInstanceJSONRoundTrip(t *testing.T) {
	orig := ObjectInstance{
		ID:          "objinst:000001",
		PrototypeID: "proto:weapon:0001",
		Quantity:    1,
		Location: ObjectLocation{
			CreatureID: "creature:player:0001",
			Slot:       "inventory",
		},
		Contents: ObjectRefList{
			ObjectIDs: []ObjectInstanceID{"objinst:000002"},
		},
		Metadata: Metadata{
			LegacyKind:     "object",
			LegacyEncoding: "euc-kr",
			LegacyPath:     "objmon/o00",
			RecordIndex:    1,
			RawFields: map[string][]byte{
				"name": {0xb9, 0xab, 0xb1, 0xe2},
			},
			PrototypeResolution: &PrototypeResolutionMetadata{
				Status:               "ambiguous",
				Method:               "exact_record_without_pointers",
				Confidence:           "ambiguous",
				SelectedPrototypeID:  "proto:weapon:0001",
				SyntheticPrototypeID: "proto:weapon:0001",
				CandidateCount:       2,
				Candidates: []PrototypeResolutionCandidate{{
					PrototypeID: "object:o00:0",
					Path:        "objmon/o00",
					Index:       0,
				}},
				Fingerprint:                      "abc123",
				FingerprintAlgorithm:             "sha256",
				ComparableBytes:                  336,
				MaterializedFromObjectInstanceID: "objinst:source",
			},
		},
	}
	if err := orig.Validate(); err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}

	var got ObjectInstance
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != orig.ID || got.PrototypeID != orig.PrototypeID {
		t.Fatalf("roundtrip ids = %#v", got)
	}
	if string(got.Metadata.RawFields["name"]) != string(orig.Metadata.RawFields["name"]) {
		t.Fatalf("raw metadata did not roundtrip: %#v", got.Metadata.RawFields)
	}
	if got.Metadata.PrototypeResolution == nil ||
		got.Metadata.PrototypeResolution.Status != "ambiguous" ||
		got.Metadata.PrototypeResolution.CandidateCount != 2 ||
		len(got.Metadata.PrototypeResolution.Candidates) != 1 ||
		got.Metadata.PrototypeResolution.Candidates[0].PrototypeID != "object:o00:0" {
		t.Fatalf("prototype resolution did not roundtrip: %#v", got.Metadata.PrototypeResolution)
	}
}

func TestValidateObjectLocationRequiresSingleHolder(t *testing.T) {
	tests := []struct {
		name string
		loc  ObjectLocation
		ok   bool
	}{
		{name: "room", loc: ObjectLocation{RoomID: "room:00001"}, ok: true},
		{name: "creature", loc: ObjectLocation{CreatureID: "creature:00001"}, ok: true},
		{name: "bank", loc: ObjectLocation{BankID: "bank:player:테스트"}, ok: true},
		{name: "container", loc: ObjectLocation{ContainerID: "objinst:00001"}, ok: true},
		{name: "missing", loc: ObjectLocation{}, ok: false},
		{name: "ambiguous", loc: ObjectLocation{RoomID: "room:00001", CreatureID: "creature:00001"}, ok: false},
	}
	for _, tt := range tests {
		err := tt.loc.Validate()
		if tt.ok && err != nil {
			t.Fatalf("%s: unexpected error: %v", tt.name, err)
		}
		if !tt.ok && err == nil {
			t.Fatalf("%s: expected error", tt.name)
		}
	}
}

func TestValidateBankAccount(t *testing.T) {
	bank := BankAccount{
		ID:        "bank:player:테스트",
		Kind:      "player",
		OwnerName: "테스트",
		Objects:   ObjectRefList{ObjectIDs: []ObjectInstanceID{"objinst:bank:00001"}},
	}
	if err := bank.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateFamily(t *testing.T) {
	family := Family{
		ID:          2,
		Slot:        2,
		DisplayName: "무영문",
		BossName:    "무영풍",
		JoinSubsidy: 100,
		Members: []FamilyMember{{
			DisplayName: "무영풍",
			Class:       10,
		}},
	}
	if err := family.Validate(); err != nil {
		t.Fatal(err)
	}

	family.ID = -1
	if err := family.Validate(); err == nil {
		t.Fatal("expected negative family id error")
	}
}

func TestValidateStableIDsAndDisplayNames(t *testing.T) {
	room := Room{
		ID:          "room:00001",
		DisplayName: "무한대전 광장",
		Exits: []Exit{{
			Name:     "북",
			ToRoomID: "room:00002",
		}},
		Objects: ObjectRefList{ObjectIDs: []ObjectInstanceID{"objinst:00001"}},
	}
	if err := room.Validate(); err != nil {
		t.Fatal(err)
	}

	player := Player{
		ID:          "player:00001",
		DisplayName: "테스트",
		CreatureID:  "creature:player:00001",
		RoomID:      room.ID,
	}
	if err := player.Validate(); err != nil {
		t.Fatal(err)
	}

	creature := Creature{
		ID:          player.CreatureID,
		Kind:        CreatureKindPlayer,
		DisplayName: player.DisplayName,
		PlayerID:    player.ID,
		RoomID:      room.ID,
		Inventory:   ObjectRefList{ObjectIDs: []ObjectInstanceID{"objinst:00001"}},
	}
	if err := creature.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsDuplicateObjectRefs(t *testing.T) {
	refs := ObjectRefList{ObjectIDs: []ObjectInstanceID{"objinst:00001", "objinst:00001"}}
	if err := refs.Validate(); err == nil {
		t.Fatal("expected duplicate object ref error")
	}
}

func TestValidateRejectsObjectSelfReferences(t *testing.T) {
	locatedInsideSelf := ObjectInstance{
		ID:          "objinst:00001",
		PrototypeID: "object:o00:0",
		Location:    ObjectLocation{ContainerID: "objinst:00001"},
	}
	if err := locatedInsideSelf.Validate(); err == nil {
		t.Fatal("expected self container error")
	}

	containsSelf := ObjectInstance{
		ID:          "objinst:00001",
		PrototypeID: "object:o00:0",
		Location:    ObjectLocation{RoomID: "room:00001"},
		Contents:    ObjectRefList{ObjectIDs: []ObjectInstanceID{"objinst:00001"}},
	}
	if err := containsSelf.Validate(); err == nil {
		t.Fatal("expected self contents error")
	}
}

func TestBoardPostJSONRoundTrip(t *testing.T) {
	orig := BoardPost{
		ID:         "post:notice:0001",
		BoardID:    "board:notice",
		Title:      "공지",
		AuthorID:   "player:dm:0001",
		AuthorName: "운영자",
		Body:       "UTF-8 본문",
		Metadata: Metadata{
			LegacyKind: "board_index",
			LegacyPath: "board/info/board_index",
		},
	}
	if err := orig.Validate(); err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}

	var got BoardPost
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Title != orig.Title || got.Body != orig.Body || got.AuthorName != orig.AuthorName {
		t.Fatalf("roundtrip post = %#v", got)
	}
}

func TestBoardPostValidateAllowsEmptyLegacyTitle(t *testing.T) {
	post := BoardPost{
		ID:      "post:board:info:000001",
		BoardID: "board:info",
		Body:    "legacy body",
	}
	if err := post.Validate(); err != nil {
		t.Fatal(err)
	}
}
