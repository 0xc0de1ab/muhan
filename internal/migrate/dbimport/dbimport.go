package dbimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/0xc0de1ab/muhan/internal/migrate/dbschema"
	"github.com/0xc0de1ab/muhan/internal/migrate/protoaudit"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type Options struct {
	RunID       string
	SourceRoot  string
	GeneratedAt time.Time
	Manifest    any
}

type Sidecar struct {
	BoardPosts []model.BoardPost
	Evidence   []protoaudit.EvidenceRecord
	Findings   []protoaudit.FindingRecord
	Artifacts  []ArtifactFile
}

type ArtifactFile struct {
	Path     string
	Format   string
	Records  *int
	Bytes    *int64
	SHA256   string
	Metadata any
}

type Batch struct {
	Table   string
	Columns []string
	Rows    [][]any
}

type Result struct {
	Batches   int
	Rows      int
	TableRows map[string]int
}

type ImportOptions struct {
	Schema string
}

type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

var identRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func Int(v int) *int {
	return &v
}

func Int64(v int64) *int64 {
	return &v
}

func BuildBatches(world *worldload.World, sidecar Sidecar, opts Options) ([]Batch, error) {
	if world == nil {
		return nil, fmt.Errorf("world is required")
	}
	if opts.RunID == "" {
		return nil, fmt.Errorf("run id is required")
	}
	generatedAt := opts.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	manifest, err := jsonText(opts.Manifest, "{}")
	if err != nil {
		return nil, fmt.Errorf("manifest JSON: %w", err)
	}

	builder := batchBuilder{runID: opts.RunID}
	batches := []Batch{{
		Table:   "import_runs",
		Columns: []string{"run_id", "schema_version", "generated_at", "source_root", "manifest"},
		Rows: [][]any{{
			opts.RunID,
			dbschema.SchemaVersion,
			generatedAt.UTC(),
			opts.SourceRoot,
			manifest,
		}},
	}}

	appendBatch := func(batch Batch, err error) error {
		if err != nil {
			return err
		}
		batches = append(batches, batch)
		return nil
	}
	if err := appendBatch(builder.rooms(world.Rooms)); err != nil {
		return nil, err
	}
	if err := appendBatch(builder.players(world.Players)); err != nil {
		return nil, err
	}
	if err := appendBatch(builder.creatures(world.Creatures)); err != nil {
		return nil, err
	}
	if err := appendBatch(builder.banks(world.Banks)); err != nil {
		return nil, err
	}
	if err := appendBatch(builder.objectPrototypes(world.ObjectPrototypes)); err != nil {
		return nil, err
	}
	if err := appendBatch(builder.objectInstances(world.Objects)); err != nil {
		return nil, err
	}
	if err := appendBatch(builder.roomExits(world.Rooms)); err != nil {
		return nil, err
	}
	if err := appendBatch(builder.roomObjects(world.Rooms)); err != nil {
		return nil, err
	}
	if err := appendBatch(builder.roomCreatures(world.Rooms)); err != nil {
		return nil, err
	}
	if err := appendBatch(builder.roomPlayers(world.Rooms)); err != nil {
		return nil, err
	}
	if err := appendBatch(builder.creatureInventory(world.Creatures)); err != nil {
		return nil, err
	}
	if err := appendBatch(builder.bankObjects(world.Banks)); err != nil {
		return nil, err
	}
	if err := appendBatch(builder.objectContents(world.Objects)); err != nil {
		return nil, err
	}
	if err := appendBatch(builder.boardPosts(sidecar.BoardPosts)); err != nil {
		return nil, err
	}
	if err := appendBatch(builder.prototypeResolutionEvidence(sidecar.Evidence)); err != nil {
		return nil, err
	}
	if err := appendBatch(builder.worldloadFindings(sidecar.Findings)); err != nil {
		return nil, err
	}
	if err := appendBatch(builder.artifactFiles(sidecar.Artifacts)); err != nil {
		return nil, err
	}
	return batches, nil
}

func ImportBatches(ctx context.Context, exec Execer, batches []Batch) (Result, error) {
	return ImportBatchesWithOptions(ctx, exec, batches, ImportOptions{})
}

func ImportBatchesWithOptions(ctx context.Context, exec Execer, batches []Batch, opts ImportOptions) (Result, error) {
	if exec == nil {
		return Result{}, fmt.Errorf("exec is required")
	}
	result := Result{TableRows: map[string]int{}}
	for _, batch := range batches {
		if len(batch.Rows) == 0 {
			continue
		}
		query, err := InsertSQLForSchema(batch, opts.Schema)
		if err != nil {
			return result, err
		}
		result.Batches++
		for i, row := range batch.Rows {
			if len(row) != len(batch.Columns) {
				return result, fmt.Errorf("table %s row %d has %d values for %d columns", batch.Table, i, len(row), len(batch.Columns))
			}
			if _, err := exec.ExecContext(ctx, query, row...); err != nil {
				return result, fmt.Errorf("insert %s row %d: %w", batch.Table, i, err)
			}
			result.Rows++
			result.TableRows[batch.Table]++
		}
	}
	return result, nil
}

func InsertSQL(batch Batch) (string, error) {
	return InsertSQLForSchema(batch, "")
}

func InsertSQLForSchema(batch Batch, schema string) (string, error) {
	if err := validateIdent(batch.Table); err != nil {
		return "", fmt.Errorf("table %q: %w", batch.Table, err)
	}
	if len(batch.Columns) == 0 {
		return "", fmt.Errorf("table %s has no columns", batch.Table)
	}
	for _, column := range batch.Columns {
		if err := validateIdent(column); err != nil {
			return "", fmt.Errorf("table %s column %q: %w", batch.Table, column, err)
		}
	}
	placeholders := make([]string, len(batch.Columns))
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	tableName, err := dbschema.QualifiedName(schema, batch.Table)
	if err != nil {
		return "", err
	}
	columnNames, err := insertColumnNames(batch.Columns, schema != "")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		tableName,
		strings.Join(columnNames, ", "),
		strings.Join(placeholders, ", "),
	), nil
}

func insertColumnNames(columns []string, quoted bool) ([]string, error) {
	out := make([]string, 0, len(columns))
	for _, column := range columns {
		if !quoted {
			out = append(out, column)
			continue
		}
		quotedColumn, err := dbschema.QuotedIdentifier(column)
		if err != nil {
			return nil, err
		}
		out = append(out, quotedColumn)
	}
	return out, nil
}

type batchBuilder struct {
	runID string
}

func (b batchBuilder) rooms(rooms map[model.RoomID]model.Room) (Batch, error) {
	batch := Batch{
		Table:   "rooms",
		Columns: []string{"run_id", "room_id", "display_name", "short_description", "long_description", "object_description", "properties", "metadata"},
	}
	for _, id := range sortedIDs(rooms) {
		room := rooms[id]
		if err := room.Validate(); err != nil {
			return Batch{}, fmt.Errorf("room %s: %w", room.ID, err)
		}
		properties, err := jsonText(room.Properties, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("room %s properties: %w", room.ID, err)
		}
		metadata, err := jsonText(room.Metadata, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("room %s metadata: %w", room.ID, err)
		}
		batch.Rows = append(batch.Rows, []any{
			b.runID,
			string(room.ID),
			room.DisplayName,
			nullableString(room.ShortDescription),
			nullableString(room.LongDescription),
			nullableString(room.ObjectDescription),
			properties,
			metadata,
		})
	}
	return batch, nil
}

func (b batchBuilder) roomExits(rooms map[model.RoomID]model.Room) (Batch, error) {
	batch := Batch{
		Table:   "room_exits",
		Columns: []string{"run_id", "room_id", "exit_index", "name", "to_room_id", "flags", "metadata"},
	}
	for _, id := range sortedIDs(rooms) {
		room := rooms[id]
		for i, exit := range room.Exits {
			flags, err := jsonText(exit.Flags, "[]")
			if err != nil {
				return Batch{}, fmt.Errorf("room %s exit %d flags: %w", room.ID, i, err)
			}
			metadata, err := jsonText(exit.Metadata, "{}")
			if err != nil {
				return Batch{}, fmt.Errorf("room %s exit %d metadata: %w", room.ID, i, err)
			}
			batch.Rows = append(batch.Rows, []any{
				b.runID,
				string(room.ID),
				i,
				exit.Name,
				string(exit.ToRoomID),
				flags,
				metadata,
			})
		}
	}
	return batch, nil
}

func (b batchBuilder) roomObjects(rooms map[model.RoomID]model.Room) (Batch, error) {
	return refRows(b.runID, "room_objects", "room_id", "object_id", sortedIDs(rooms), func(id model.RoomID) []model.ObjectInstanceID {
		return rooms[id].Objects.ObjectIDs
	})
}

func (b batchBuilder) roomCreatures(rooms map[model.RoomID]model.Room) (Batch, error) {
	return refRows(b.runID, "room_creatures", "room_id", "creature_id", sortedIDs(rooms), func(id model.RoomID) []model.CreatureID {
		return rooms[id].CreatureIDs
	})
}

func (b batchBuilder) roomPlayers(rooms map[model.RoomID]model.Room) (Batch, error) {
	return refRows(b.runID, "room_players", "room_id", "player_id", sortedIDs(rooms), func(id model.RoomID) []model.PlayerID {
		return rooms[id].PlayerIDs
	})
}

func (b batchBuilder) players(players map[model.PlayerID]model.Player) (Batch, error) {
	batch := Batch{
		Table:   "players",
		Columns: []string{"run_id", "player_id", "display_name", "creature_id", "room_id", "account_name", "metadata"},
	}
	for _, id := range sortedIDs(players) {
		player := players[id]
		if err := player.Validate(); err != nil {
			return Batch{}, fmt.Errorf("player %s: %w", player.ID, err)
		}
		metadata, err := jsonText(player.Metadata, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("player %s metadata: %w", player.ID, err)
		}
		batch.Rows = append(batch.Rows, []any{
			b.runID,
			string(player.ID),
			player.DisplayName,
			nullableID(player.CreatureID),
			nullableID(player.RoomID),
			nullableString(player.AccountName),
			metadata,
		})
	}
	return batch, nil
}

func (b batchBuilder) creatures(creatures map[model.CreatureID]model.Creature) (Batch, error) {
	batch := Batch{
		Table:   "creatures",
		Columns: []string{"run_id", "creature_id", "kind", "display_name", "description", "level", "room_id", "player_id", "equipment", "stats", "properties", "metadata"},
	}
	for _, id := range sortedIDs(creatures) {
		creature := creatures[id]
		if err := creature.Validate(); err != nil {
			return Batch{}, fmt.Errorf("creature %s: %w", creature.ID, err)
		}
		equipment, err := jsonText(creature.Equipment, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("creature %s equipment: %w", creature.ID, err)
		}
		stats, err := jsonText(creature.Stats, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("creature %s stats: %w", creature.ID, err)
		}
		properties, err := jsonText(creature.Properties, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("creature %s properties: %w", creature.ID, err)
		}
		metadata, err := jsonText(creature.Metadata, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("creature %s metadata: %w", creature.ID, err)
		}
		batch.Rows = append(batch.Rows, []any{
			b.runID,
			string(creature.ID),
			string(creature.Kind),
			creature.DisplayName,
			nullableString(creature.Description),
			nullablePositiveInt(creature.Level),
			nullableID(creature.RoomID),
			nullableID(creature.PlayerID),
			equipment,
			stats,
			properties,
			metadata,
		})
	}
	return batch, nil
}

func (b batchBuilder) creatureInventory(creatures map[model.CreatureID]model.Creature) (Batch, error) {
	return refRows(b.runID, "creature_inventory", "creature_id", "object_id", sortedIDs(creatures), func(id model.CreatureID) []model.ObjectInstanceID {
		return creatures[id].Inventory.ObjectIDs
	})
}

func (b batchBuilder) banks(banks map[model.BankID]model.BankAccount) (Batch, error) {
	batch := Batch{
		Table:   "banks",
		Columns: []string{"run_id", "bank_id", "kind", "owner_name", "owner_player_id", "metadata"},
	}
	for _, id := range sortedIDs(banks) {
		bank := banks[id]
		if err := bank.Validate(); err != nil {
			return Batch{}, fmt.Errorf("bank %s: %w", bank.ID, err)
		}
		metadata, err := jsonText(bank.Metadata, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("bank %s metadata: %w", bank.ID, err)
		}
		batch.Rows = append(batch.Rows, []any{
			b.runID,
			string(bank.ID),
			bank.Kind,
			bank.OwnerName,
			nullableID(bank.OwnerPlayerID),
			metadata,
		})
	}
	return batch, nil
}

func (b batchBuilder) bankObjects(banks map[model.BankID]model.BankAccount) (Batch, error) {
	return refRows(b.runID, "bank_objects", "bank_id", "object_id", sortedIDs(banks), func(id model.BankID) []model.ObjectInstanceID {
		return banks[id].Objects.ObjectIDs
	})
}

func (b batchBuilder) objectPrototypes(prototypes map[model.PrototypeID]model.ObjectPrototype) (Batch, error) {
	batch := Batch{
		Table:   "object_prototypes",
		Columns: []string{"run_id", "prototype_id", "kind", "display_name", "description", "keywords", "properties", "metadata"},
	}
	for _, id := range sortedIDs(prototypes) {
		proto := prototypes[id]
		if err := proto.Validate(); err != nil {
			return Batch{}, fmt.Errorf("object prototype %s: %w", proto.ID, err)
		}
		keywords, err := jsonText(proto.Keywords, "[]")
		if err != nil {
			return Batch{}, fmt.Errorf("object prototype %s keywords: %w", proto.ID, err)
		}
		properties, err := jsonText(proto.Properties, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("object prototype %s properties: %w", proto.ID, err)
		}
		metadata, err := jsonText(proto.Metadata, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("object prototype %s metadata: %w", proto.ID, err)
		}
		batch.Rows = append(batch.Rows, []any{
			b.runID,
			string(proto.ID),
			nullableString(string(proto.Kind)),
			proto.DisplayName,
			nullableString(proto.Description),
			keywords,
			properties,
			metadata,
		})
	}
	return batch, nil
}

func (b batchBuilder) objectInstances(objects map[model.ObjectInstanceID]model.ObjectInstance) (Batch, error) {
	batch := Batch{
		Table:   "object_instances",
		Columns: []string{"run_id", "object_id", "prototype_id", "display_name_override", "quantity", "room_id", "creature_id", "bank_id", "container_id", "slot", "properties", "metadata"},
	}
	for _, id := range sortedIDs(objects) {
		object := objects[id]
		if err := object.Validate(); err != nil {
			return Batch{}, fmt.Errorf("object %s: %w", object.ID, err)
		}
		properties, err := jsonText(object.Properties, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("object %s properties: %w", object.ID, err)
		}
		metadata, err := jsonText(object.Metadata, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("object %s metadata: %w", object.ID, err)
		}
		batch.Rows = append(batch.Rows, []any{
			b.runID,
			string(object.ID),
			string(object.PrototypeID),
			nullableString(object.DisplayNameOverride),
			object.Quantity,
			nullableID(object.Location.RoomID),
			nullableID(object.Location.CreatureID),
			nullableID(object.Location.BankID),
			nullableID(object.Location.ContainerID),
			nullableString(object.Location.Slot),
			properties,
			metadata,
		})
	}
	return batch, nil
}

func (b batchBuilder) objectContents(objects map[model.ObjectInstanceID]model.ObjectInstance) (Batch, error) {
	return refRows(b.runID, "object_contents", "container_id", "child_object_id", sortedIDs(objects), func(id model.ObjectInstanceID) []model.ObjectInstanceID {
		return objects[id].Contents.ObjectIDs
	})
}

func (b batchBuilder) boardPosts(posts []model.BoardPost) (Batch, error) {
	batch := Batch{
		Table:   "board_posts",
		Columns: []string{"run_id", "post_id", "board_id", "title", "author_id", "author_name", "body", "created_at", "read_count", "metadata"},
	}
	sorted := append([]model.BoardPost(nil), posts...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})
	for _, post := range sorted {
		if err := post.Validate(); err != nil {
			return Batch{}, fmt.Errorf("board post %s: %w", post.ID, err)
		}
		metadata, err := jsonText(post.Metadata, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("board post %s metadata: %w", post.ID, err)
		}
		batch.Rows = append(batch.Rows, []any{
			b.runID,
			string(post.ID),
			string(post.BoardID),
			post.Title,
			nullableID(post.AuthorID),
			nullableString(post.AuthorName),
			post.Body,
			nullableTime(post.CreatedAt),
			post.ReadCount,
			metadata,
		})
	}
	return batch, nil
}

func (b batchBuilder) prototypeResolutionEvidence(records []protoaudit.EvidenceRecord) (Batch, error) {
	batch := Batch{
		Table: "prototype_resolution_evidence",
		Columns: []string{
			"run_id", "evidence_id", "object_instance_id", "prototype_id", "status", "method", "confidence",
			"selected_prototype_id", "synthetic_prototype_id", "candidate_count", "candidate_cap", "candidates_truncated",
			"fingerprint", "fingerprint_algorithm", "comparable_bytes", "source", "c_format", "resolution", "tags",
		},
	}
	sorted := append([]protoaudit.EvidenceRecord(nil), records...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].EvidenceID < sorted[j].EvidenceID
	})
	for _, record := range sorted {
		if record.EvidenceID == "" {
			return Batch{}, fmt.Errorf("prototype resolution evidence id is required")
		}
		if record.ObjectInstanceID.IsZero() {
			return Batch{}, fmt.Errorf("prototype resolution evidence %s object instance id is required", record.EvidenceID)
		}
		if record.PrototypeID.IsZero() {
			return Batch{}, fmt.Errorf("prototype resolution evidence %s prototype id is required", record.EvidenceID)
		}
		if record.Resolution.Status == "" {
			return Batch{}, fmt.Errorf("prototype resolution evidence %s status is required", record.EvidenceID)
		}
		source, err := jsonText(record.Source, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("prototype resolution evidence %s source: %w", record.EvidenceID, err)
		}
		cFormat, err := jsonText(record.CFormat, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("prototype resolution evidence %s c format: %w", record.EvidenceID, err)
		}
		resolution, err := jsonText(record.Resolution, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("prototype resolution evidence %s resolution: %w", record.EvidenceID, err)
		}
		tags, err := jsonText(record.Tags, "[]")
		if err != nil {
			return Batch{}, fmt.Errorf("prototype resolution evidence %s tags: %w", record.EvidenceID, err)
		}
		batch.Rows = append(batch.Rows, []any{
			b.runID,
			record.EvidenceID,
			string(record.ObjectInstanceID),
			string(record.PrototypeID),
			record.Resolution.Status,
			nullableString(record.Resolution.Method),
			nullableString(record.Resolution.Confidence),
			nullableID(record.Resolution.SelectedPrototypeID),
			nullableID(record.Resolution.SyntheticPrototypeID),
			record.Resolution.CandidateCount,
			record.CandidateCap,
			record.CandidatesTruncated,
			nullableString(record.Resolution.Fingerprint),
			nullableString(record.Resolution.FingerprintAlgorithm),
			nullablePositiveInt(record.Resolution.ComparableBytes),
			source,
			cFormat,
			resolution,
			tags,
		})
	}
	return batch, nil
}

func (b batchBuilder) worldloadFindings(records []protoaudit.FindingRecord) (Batch, error) {
	batch := Batch{
		Table:   "worldload_findings",
		Columns: []string{"run_id", "severity", "kind", "path", "entity_id", "ref", "message", "source"},
	}
	sorted := append([]protoaudit.FindingRecord(nil), records...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Severity != sorted[j].Severity {
			return sorted[i].Severity < sorted[j].Severity
		}
		if sorted[i].Path != sorted[j].Path {
			return sorted[i].Path < sorted[j].Path
		}
		if sorted[i].Kind != sorted[j].Kind {
			return sorted[i].Kind < sorted[j].Kind
		}
		return sorted[i].Message < sorted[j].Message
	})
	for _, record := range sorted {
		if record.Severity == "" {
			return Batch{}, fmt.Errorf("worldload finding severity is required")
		}
		if record.Kind == "" {
			return Batch{}, fmt.Errorf("worldload finding kind is required")
		}
		if record.Message == "" {
			return Batch{}, fmt.Errorf("worldload finding message is required")
		}
		source, err := jsonText(record.Source, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("worldload finding %s source: %w", record.Kind, err)
		}
		batch.Rows = append(batch.Rows, []any{
			b.runID,
			record.Severity,
			record.Kind,
			nullableString(record.Path),
			nullableString(record.ID),
			nullableString(record.Ref),
			record.Message,
			source,
		})
	}
	return batch, nil
}

func (b batchBuilder) artifactFiles(files []ArtifactFile) (Batch, error) {
	batch := Batch{
		Table:   "artifact_files",
		Columns: []string{"run_id", "path", "format", "records", "bytes", "sha256", "metadata"},
	}
	sorted := append([]ArtifactFile(nil), files...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Path < sorted[j].Path
	})
	for _, file := range sorted {
		if file.Path == "" {
			return Batch{}, fmt.Errorf("artifact path is required")
		}
		if file.Format == "" {
			return Batch{}, fmt.Errorf("artifact %s format is required", file.Path)
		}
		metadata, err := jsonText(file.Metadata, "{}")
		if err != nil {
			return Batch{}, fmt.Errorf("artifact %s metadata: %w", file.Path, err)
		}
		batch.Rows = append(batch.Rows, []any{
			b.runID,
			file.Path,
			file.Format,
			nullableIntPtr(file.Records),
			nullableInt64Ptr(file.Bytes),
			nullableString(file.SHA256),
			metadata,
		})
	}
	return batch, nil
}

func refRows[K ~string, C ~string](runID, table, parentColumn, childColumn string, parents []K, childIDs func(K) []C) (Batch, error) {
	batch := Batch{
		Table:   table,
		Columns: []string{"run_id", parentColumn, childColumn, "ref_index", "metadata"},
	}
	for _, parentID := range parents {
		for i, childID := range childIDs(parentID) {
			batch.Rows = append(batch.Rows, []any{
				runID,
				string(parentID),
				string(childID),
				i,
				"{}",
			})
		}
	}
	return batch, nil
}

func sortedIDs[K ~string, V any](m map[K]V) []K {
	ids := make([]K, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	return ids
}

func jsonText(v any, fallback string) (string, error) {
	if v == nil {
		return fallback, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if string(data) == "null" {
		return fallback, nil
	}
	return string(data), nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableID[K ~string](value K) any {
	if value == "" {
		return nil
	}
	return string(value)
}

func nullablePositiveInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func nullableIntPtr(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt64Ptr(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func validateIdent(value string) error {
	if !identRE.MatchString(value) {
		return fmt.Errorf("identifier must match %s", identRE.String())
	}
	return nil
}
