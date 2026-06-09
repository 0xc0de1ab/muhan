package dbimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"muhan/internal/migrate/dbschema"
	"muhan/internal/migrate/protoaudit"
	worldload "muhan/internal/world/load"
	"muhan/internal/world/model"
)

func TestBuildBatchesTinyWorldFKSafeOrderAndCounts(t *testing.T) {
	batches := buildTinyBatches(t)
	gotNames := batchNames(batches)
	wantNames := []string{
		"import_runs",
		"rooms",
		"players",
		"creatures",
		"banks",
		"object_prototypes",
		"object_instances",
		"room_exits",
		"room_objects",
		"room_creatures",
		"room_players",
		"creature_inventory",
		"bank_objects",
		"object_contents",
		"board_posts",
		"prototype_resolution_evidence",
		"worldload_findings",
		"artifact_files",
	}
	if strings.Join(gotNames, "\n") != strings.Join(wantNames, "\n") {
		t.Fatalf("batch order = %v, want %v", gotNames, wantNames)
	}

	wantRows := map[string]int{
		"import_runs":                   1,
		"rooms":                         2,
		"players":                       2,
		"creatures":                     2,
		"banks":                         2,
		"object_prototypes":             4,
		"object_instances":              4,
		"room_exits":                    2,
		"room_objects":                  1,
		"room_creatures":                2,
		"room_players":                  1,
		"creature_inventory":            1,
		"bank_objects":                  1,
		"object_contents":               1,
		"board_posts":                   1,
		"prototype_resolution_evidence": 1,
		"worldload_findings":            1,
		"artifact_files":                1,
	}
	for table, want := range wantRows {
		if got := len(batchByName(t, batches, table).Rows); got != want {
			t.Fatalf("%s rows = %d, want %d", table, got, want)
		}
	}

	objectColumns := batchByName(t, batches, "object_instances").Columns
	for _, column := range objectColumns {
		if column == "holder_count" {
			t.Fatal("object_instances insert columns include generated holder_count")
		}
	}
}

func TestBuildBatchesOrderFollowsSchemaForeignKeys(t *testing.T) {
	batches := buildTinyBatches(t)
	positions := map[string]int{}
	for i, batch := range batches {
		positions[batch.Table] = i
	}

	manifest, err := dbschema.Build(dbschema.Options{GeneratedAt: fixedTime()})
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range manifest.Tables {
		tablePos, ok := positions[table.Name]
		if !ok {
			t.Fatalf("missing batch for table %s", table.Name)
		}
		for _, fk := range table.ForeignKeys {
			refPos, ok := positions[fk.RefTable]
			if !ok {
				t.Fatalf("missing batch for referenced table %s", fk.RefTable)
			}
			if refPos >= tablePos {
				t.Fatalf("table %s appears at %d before referenced table %s at %d", table.Name, tablePos, fk.RefTable, refPos)
			}
		}
	}
}

func TestBuildBatchesColumnsMatchSchemaInsertColumns(t *testing.T) {
	batches := buildTinyBatches(t)
	manifest, err := dbschema.Build(dbschema.Options{GeneratedAt: fixedTime()})
	if err != nil {
		t.Fatal(err)
	}
	tables := map[string]dbschema.Table{}
	for _, table := range manifest.Tables {
		tables[table.Name] = table
	}
	for _, batch := range batches {
		table, ok := tables[batch.Table]
		if !ok {
			t.Fatalf("batch table %s is missing from schema manifest", batch.Table)
		}
		want := insertColumns(table)
		if strings.Join(batch.Columns, ",") != strings.Join(want, ",") {
			t.Fatalf("%s columns = %v, want schema insert columns %v", batch.Table, batch.Columns, want)
		}
	}
}

func TestBuildBatchesSortsMapsAndPreservesSliceOrder(t *testing.T) {
	batches := buildTinyBatches(t)

	rooms := batchByName(t, batches, "rooms")
	roomIDColumn := columnIndex(t, rooms, "room_id")
	if rooms.Rows[0][roomIDColumn] != "room:00001" || rooms.Rows[1][roomIDColumn] != "room:00002" {
		t.Fatalf("rooms are not sorted by id: %+v", rooms.Rows)
	}

	prototypes := batchByName(t, batches, "object_prototypes")
	protoIDColumn := columnIndex(t, prototypes, "prototype_id")
	gotProtoIDs := valuesAt(prototypes.Rows, protoIDColumn)
	wantProtoIDs := []any{"proto:bag", "proto:coin", "proto:gem", "proto:sword"}
	if strings.Join(anyStrings(gotProtoIDs), ",") != strings.Join(anyStrings(wantProtoIDs), ",") {
		t.Fatalf("prototype ids = %v, want %v", gotProtoIDs, wantProtoIDs)
	}

	roomCreatures := batchByName(t, batches, "room_creatures")
	if roomCreatures.Rows[0][2] != "creature:player:alice" || roomCreatures.Rows[0][3] != 0 ||
		roomCreatures.Rows[1][2] != "creature:npc:guide" || roomCreatures.Rows[1][3] != 1 {
		t.Fatalf("room creature slice order not preserved: %+v", roomCreatures.Rows)
	}

	roomExits := batchByName(t, batches, "room_exits")
	if roomExits.Rows[0][3] != "동" || roomExits.Rows[0][2] != 0 ||
		roomExits.Rows[1][3] != "서" || roomExits.Rows[1][2] != 1 {
		t.Fatalf("room exit slice order not preserved: %+v", roomExits.Rows)
	}
}

func TestBuildBatchesJSONAndNullableValues(t *testing.T) {
	batches := buildTinyBatches(t)

	rooms := batchByName(t, batches, "rooms")
	properties := rooms.Rows[0][columnIndex(t, rooms, "properties")]
	if properties != `{"a":"first","z":"last"}` {
		t.Fatalf("room properties JSON = %v", properties)
	}
	assertValidJSON(t, properties)
	if rooms.Rows[1][columnIndex(t, rooms, "short_description")] != nil ||
		rooms.Rows[1][columnIndex(t, rooms, "properties")] != "{}" {
		t.Fatalf("empty room nullable/default JSON row = %+v", rooms.Rows[1])
	}

	players := batchByName(t, batches, "players")
	bob := players.Rows[1]
	for _, column := range []string{"creature_id", "room_id", "account_name"} {
		if got := bob[columnIndex(t, players, column)]; got != nil {
			t.Fatalf("bob %s = %v, want nil", column, got)
		}
	}

	objects := batchByName(t, batches, "object_instances")
	for _, row := range objects.Rows {
		holders := 0
		for _, column := range []string{"room_id", "creature_id", "bank_id", "container_id"} {
			if row[columnIndex(t, objects, column)] != nil {
				holders++
			}
		}
		if holders != 1 {
			t.Fatalf("object row has %d holders: %+v", holders, row)
		}
		assertValidJSON(t, row[columnIndex(t, objects, "metadata")])
	}

	evidence := batchByName(t, batches, "prototype_resolution_evidence")
	row := evidence.Rows[0]
	if row[columnIndex(t, evidence, "status")] != "resolved" ||
		row[columnIndex(t, evidence, "candidate_count")] != 1 ||
		row[columnIndex(t, evidence, "candidate_cap")] != 1 {
		t.Fatalf("evidence projection row = %+v", row)
	}
	assertValidJSON(t, row[columnIndex(t, evidence, "source")])
	assertValidJSON(t, row[columnIndex(t, evidence, "c_format")])
	assertValidJSON(t, row[columnIndex(t, evidence, "resolution")])

	posts := batchByName(t, batches, "board_posts")
	post := posts.Rows[0]
	if post[columnIndex(t, posts, "title")] != "공지" ||
		post[columnIndex(t, posts, "author_name")] != "운영자" ||
		post[columnIndex(t, posts, "body")] != "본문" ||
		post[columnIndex(t, posts, "read_count")] != 7 {
		t.Fatalf("board post projection row = %+v", post)
	}
	if post[columnIndex(t, posts, "author_id")] != nil {
		t.Fatalf("board post author_id = %v, want nil", post[columnIndex(t, posts, "author_id")])
	}
	assertValidJSON(t, post[columnIndex(t, posts, "metadata")])
}

func TestBuildBatchesRejectsInvalidObjectHolderBeforeInsert(t *testing.T) {
	world := tinyWorld(t)
	object := world.Objects["object:room-bag"]
	object.Location = model.ObjectLocation{}
	world.Objects[object.ID] = object

	_, err := BuildBatches(world, Sidecar{}, Options{RunID: "run:test", GeneratedAt: fixedTime()})
	if err == nil {
		t.Fatal("BuildBatches succeeded with invalid object holder")
	}
	if !strings.Contains(err.Error(), "exactly one object holder") {
		t.Fatalf("error = %v", err)
	}
}

func TestImportBatchesUsesParameterizedInserts(t *testing.T) {
	batches := []Batch{{
		Table:   "rooms",
		Columns: []string{"run_id", "room_id", "display_name"},
		Rows: [][]any{
			{"run:test", "room:00001", "광장"},
			{"run:test", "room:00002", "시장"},
		},
	}}
	exec := &recordingExec{}

	result, err := ImportBatches(context.Background(), exec, batches)
	if err != nil {
		t.Fatal(err)
	}
	if result.Batches != 1 || result.Rows != 2 || result.TableRows["rooms"] != 2 {
		t.Fatalf("result = %+v", result)
	}
	wantQuery := "INSERT INTO rooms (run_id, room_id, display_name) VALUES ($1, $2, $3)"
	for _, call := range exec.calls {
		if call.query != wantQuery {
			t.Fatalf("query = %q, want %q", call.query, wantQuery)
		}
	}
}

func TestImportBatchesCanTargetSchema(t *testing.T) {
	batches := []Batch{{
		Table:   "rooms",
		Columns: []string{"run_id", "room_id", "display_name"},
		Rows:    [][]any{{"run:test", "room:00001", "광장"}},
	}}
	exec := &recordingExec{}

	_, err := ImportBatchesWithOptions(context.Background(), exec, batches, ImportOptions{Schema: "muhan_import"})
	if err != nil {
		t.Fatal(err)
	}
	wantQuery := `INSERT INTO "muhan_import"."rooms" ("run_id", "room_id", "display_name") VALUES ($1, $2, $3)`
	if len(exec.calls) != 1 || exec.calls[0].query != wantQuery {
		t.Fatalf("calls = %+v, want query %q", exec.calls, wantQuery)
	}
}

func TestInsertSQLRejectsUnsafeIdentifiers(t *testing.T) {
	if _, err := InsertSQL(Batch{Table: "rooms;drop", Columns: []string{"run_id"}}); err == nil {
		t.Fatal("InsertSQL accepted unsafe table identifier")
	}
	if _, err := InsertSQL(Batch{Table: "rooms", Columns: []string{"run_id", "bad-column"}}); err == nil {
		t.Fatal("InsertSQL accepted unsafe column identifier")
	}
	if _, err := InsertSQLForSchema(Batch{Table: "rooms", Columns: []string{"run_id"}}, "bad-schema"); err == nil {
		t.Fatal("InsertSQLForSchema accepted unsafe schema identifier")
	}
}

func buildTinyBatches(t *testing.T) []Batch {
	t.Helper()
	batches, err := BuildBatches(tinyWorld(t), tinySidecar(), Options{
		RunID:       "run:test",
		SourceRoot:  "/legacy/muhan",
		GeneratedAt: fixedTime(),
		Manifest: map[string]any{
			"source": "test",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return batches
}

func tinyWorld(t *testing.T) *worldload.World {
	t.Helper()
	world := worldload.NewWorld()
	mustAddRoom(t, world, model.Room{
		ID:          "room:00002",
		DisplayName: "시장",
	})
	mustAddRoom(t, world, model.Room{
		ID:          "room:00001",
		DisplayName: "광장",
		Exits: []model.Exit{{
			Name:     "동",
			ToRoomID: "room:00002",
			Flags:    []string{"open", "lit"},
			Metadata: model.Metadata{Tags: []string{"main"}},
		}, {
			Name:     "서",
			ToRoomID: "room:00002",
		}},
		CreatureIDs: []model.CreatureID{"creature:player:alice", "creature:npc:guide"},
		PlayerIDs:   []model.PlayerID{"player:alice"},
		Objects:     model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:room-bag"}},
		Properties:  map[string]string{"z": "last", "a": "first"},
		Metadata:    model.Metadata{LegacyKind: "room", LegacyPath: "rooms/r00/r00001"},
	})
	mustAddPlayer(t, world, model.Player{
		ID:          "player:alice",
		DisplayName: "앨리스",
		CreatureID:  "creature:player:alice",
		RoomID:      "room:00001",
		AccountName: "alice",
		Metadata:    model.Metadata{LegacyKind: "player", LegacyPath: "player/a/alice"},
	})
	mustAddPlayer(t, world, model.Player{
		ID:          "player:bob",
		DisplayName: "밥",
	})
	mustAddCreature(t, world, model.Creature{
		ID:          "creature:player:alice",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "앨리스",
		RoomID:      "room:00001",
		PlayerID:    "player:alice",
		Inventory:   model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:inv-sword"}},
		Equipment:   map[string]model.ObjectInstanceID{"right": "object:inv-sword"},
		Stats:       map[string]int{"hp": 10},
	})
	mustAddCreature(t, world, model.Creature{
		ID:          "creature:npc:guide",
		Kind:        model.CreatureKindNPC,
		DisplayName: "안내자",
		RoomID:      "room:00001",
	})
	mustAddBank(t, world, model.BankAccount{
		ID:            "bank:player:alice",
		Kind:          "player",
		OwnerName:     "앨리스",
		OwnerPlayerID: "player:alice",
		Objects:       model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:bank-coin"}},
	})
	mustAddBank(t, world, model.BankAccount{
		ID:        "bank:family:blue",
		Kind:      "family",
		OwnerName: "blue",
	})
	for _, proto := range []model.ObjectPrototype{
		{ID: "proto:sword", Kind: model.ObjectKindWeapon, DisplayName: "검", Keywords: []string{"검", "칼"}},
		{ID: "proto:bag", Kind: model.ObjectKindContainer, DisplayName: "가방"},
		{ID: "proto:gem", DisplayName: "보석"},
		{ID: "proto:coin", Kind: model.ObjectKindMoney, DisplayName: "동전"},
	} {
		mustAddObjectPrototype(t, world, proto)
	}
	mustAddObject(t, world, model.ObjectInstance{
		ID:          "object:room-bag",
		PrototypeID: "proto:bag",
		Quantity:    1,
		Location:    model.ObjectLocation{RoomID: "room:00001"},
		Contents:    model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{"object:bag-gem"}},
		Properties:  map[string]string{"weight": "1"},
		Metadata: model.Metadata{
			LegacyKind: "objectTreeObject",
			RawFields:  map[string][]byte{"name": {0xb0, 0xa1}},
			PrototypeResolution: &model.PrototypeResolutionMetadata{
				Status:              "resolved",
				SelectedPrototypeID: "proto:bag",
				CandidateCount:      1,
			},
		},
	})
	mustAddObject(t, world, model.ObjectInstance{
		ID:          "object:bag-gem",
		PrototypeID: "proto:gem",
		Quantity:    1,
		Location:    model.ObjectLocation{ContainerID: "object:room-bag"},
	})
	mustAddObject(t, world, model.ObjectInstance{
		ID:          "object:inv-sword",
		PrototypeID: "proto:sword",
		Quantity:    1,
		Location:    model.ObjectLocation{CreatureID: "creature:player:alice", Slot: "inventory"},
	})
	mustAddObject(t, world, model.ObjectInstance{
		ID:          "object:bank-coin",
		PrototypeID: "proto:coin",
		Quantity:    1,
		Location:    model.ObjectLocation{BankID: "bank:player:alice"},
	})
	return world
}

func tinySidecar() Sidecar {
	return Sidecar{
		BoardPosts: []model.BoardPost{{
			ID:         "post:notice:0001",
			BoardID:    "board:notice",
			Title:      "공지",
			AuthorName: "운영자",
			Body:       "본문",
			ReadCount:  7,
		}},
		Evidence: []protoaudit.EvidenceRecord{{
			SchemaVersion:    protoaudit.SchemaVersion,
			ResolverVersion:  protoaudit.ResolverVersion,
			EvidenceID:       "sha256:test",
			ObjectInstanceID: "object:room-bag",
			PrototypeID:      "proto:bag",
			Source: protoaudit.SourceEvidence{
				LegacyPath:     "rooms/r00/r00001",
				ObjectTreePath: "0",
				FileSHA256:     strings.Repeat("a", 64),
			},
			CFormat: protoaudit.CFormatEvidence{
				ObjectStructSizeBytes: 352,
			},
			Resolution: model.PrototypeResolutionMetadata{
				Status:               "resolved",
				Method:               "exact_record_without_pointers",
				Confidence:           "exact",
				SelectedPrototypeID:  "proto:bag",
				CandidateCount:       1,
				Fingerprint:          "abc123",
				FingerprintAlgorithm: "sha256",
				ComparableBytes:      336,
			},
			Tags:         []string{"prototype:resolved"},
			CandidateCap: 1,
		}},
		Findings: []protoaudit.FindingRecord{{
			SchemaVersion: protoaudit.SchemaVersion,
			Severity:      "warning",
			Kind:          "missing_room_ref",
			Path:          "rooms/r00/r00001",
			ID:            "room:00001",
			Ref:           "room:99999",
			Message:       "missing room",
		}},
		Artifacts: []ArtifactFile{{
			Path:     "prototype_resolution_evidence.jsonl",
			Format:   "jsonl",
			Records:  Int(1),
			Bytes:    Int64(123),
			SHA256:   strings.Repeat("b", 64),
			Metadata: map[string]string{"source": "protoaudit.index"},
		}},
	}
}

func fixedTime() time.Time {
	return time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC)
}

func mustAddRoom(t *testing.T, world *worldload.World, room model.Room) {
	t.Helper()
	if err := world.AddRoom(room); err != nil {
		t.Fatal(err)
	}
}

func mustAddPlayer(t *testing.T, world *worldload.World, player model.Player) {
	t.Helper()
	if err := world.AddPlayer(player); err != nil {
		t.Fatal(err)
	}
}

func mustAddCreature(t *testing.T, world *worldload.World, creature model.Creature) {
	t.Helper()
	if err := world.AddCreature(creature); err != nil {
		t.Fatal(err)
	}
}

func mustAddBank(t *testing.T, world *worldload.World, bank model.BankAccount) {
	t.Helper()
	if err := world.AddBank(bank); err != nil {
		t.Fatal(err)
	}
}

func mustAddObjectPrototype(t *testing.T, world *worldload.World, proto model.ObjectPrototype) {
	t.Helper()
	if err := world.AddObjectPrototype(proto); err != nil {
		t.Fatal(err)
	}
}

func mustAddObject(t *testing.T, world *worldload.World, object model.ObjectInstance) {
	t.Helper()
	if err := world.AddObjectInstance(object); err != nil {
		t.Fatal(err)
	}
}

func batchNames(batches []Batch) []string {
	names := make([]string, 0, len(batches))
	for _, batch := range batches {
		names = append(names, batch.Table)
	}
	return names
}

func batchByName(t *testing.T, batches []Batch, name string) Batch {
	t.Helper()
	for _, batch := range batches {
		if batch.Table == name {
			return batch
		}
	}
	t.Fatalf("missing batch %s", name)
	return Batch{}
}

func columnIndex(t *testing.T, batch Batch, name string) int {
	t.Helper()
	for i, column := range batch.Columns {
		if column == name {
			return i
		}
	}
	t.Fatalf("%s missing column %s", batch.Table, name)
	return -1
}

func valuesAt(rows [][]any, column int) []any {
	values := make([]any, 0, len(rows))
	for _, row := range rows {
		values = append(values, row[column])
	}
	return values
}

func anyStrings(values []any) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.(string))
	}
	return out
}

func insertColumns(table dbschema.Table) []string {
	columns := make([]string, 0, len(table.Columns))
	for _, column := range table.Columns {
		upperType := strings.ToUpper(column.Type)
		if strings.Contains(upperType, "GENERATED ALWAYS") || upperType == "BIGSERIAL" {
			continue
		}
		columns = append(columns, column.Name)
	}
	return columns
}

func assertValidJSON(t *testing.T, value any) {
	t.Helper()
	text, ok := value.(string)
	if !ok {
		t.Fatalf("JSON value has type %T, want string", value)
	}
	if !json.Valid([]byte(text)) {
		t.Fatalf("invalid JSON: %q", text)
	}
}

type recordingExec struct {
	calls []execCall
}

type execCall struct {
	query string
	args  []any
}

func (e *recordingExec) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	e.calls = append(e.calls, execCall{query: query, args: append([]any(nil), args...)})
	return fakeResult(1), nil
}

type fakeResult int64

func (r fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (r fakeResult) RowsAffected() (int64, error) {
	return int64(r), nil
}
