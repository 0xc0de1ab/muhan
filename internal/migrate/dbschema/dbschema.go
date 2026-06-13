package dbschema

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/0xc0de1ab/muhan/internal/persist/jsonstore"
)

const (
	SchemaVersion   = "muhan-db-schema/v1"
	DialectPostgres = "postgresql"
	DefaultSchema   = "muhan_import"

	postgresSchemaFile = "schema.postgresql.sql"
	manifestFile       = "schema_manifest.json"
)

type Options struct {
	Dialect     string
	GeneratedAt time.Time
}

type Manifest struct {
	SchemaVersion string         `json:"schemaVersion"`
	Dialect       string         `json:"dialect"`
	GeneratedAt   time.Time      `json:"generatedAt"`
	Tables        []Table        `json:"tables"`
	Files         []ArtifactFile `json:"files,omitempty"`
}

type ArtifactFile struct {
	Path   string `json:"path"`
	Format string `json:"format"`
	Bytes  int64  `json:"bytes,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

type Table struct {
	Name        string       `json:"name"`
	Purpose     string       `json:"purpose"`
	Source      string       `json:"source,omitempty"`
	Columns     []Column     `json:"columns"`
	PrimaryKey  []string     `json:"primaryKey,omitempty"`
	ForeignKeys []ForeignKey `json:"foreignKeys,omitempty"`
	Indexes     []Index      `json:"indexes,omitempty"`
}

type Column struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Nullable    bool   `json:"nullable"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

type ForeignKey struct {
	Columns    []string `json:"columns"`
	RefTable   string   `json:"refTable"`
	RefColumns []string `json:"refColumns"`
	OnDelete   string   `json:"onDelete,omitempty"`
}

type Index struct {
	Name        string   `json:"name"`
	Columns     []string `json:"columns,omitempty"`
	Expression  string   `json:"expression,omitempty"`
	Method      string   `json:"method,omitempty"`
	Unique      bool     `json:"unique,omitempty"`
	Description string   `json:"description,omitempty"`
}

var identRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func Build(opts Options) (Manifest, error) {
	dialect := opts.Dialect
	if dialect == "" {
		dialect = DialectPostgres
	}
	if dialect != DialectPostgres {
		return Manifest{}, fmt.Errorf("unsupported dialect %q", dialect)
	}
	generatedAt := opts.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	manifest := Manifest{
		SchemaVersion: SchemaVersion,
		Dialect:       dialect,
		GeneratedAt:   generatedAt.UTC(),
		Tables:        postgresTables(),
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func PostgresDDL(manifest Manifest) (string, error) {
	return PostgresDDLForSchema(manifest, "")
}

func PostgresDDLForSchema(manifest Manifest, schema string) (string, error) {
	if manifest.Dialect != DialectPostgres {
		return "", fmt.Errorf("cannot render dialect %q as PostgreSQL", manifest.Dialect)
	}
	if err := validateManifest(manifest); err != nil {
		return "", err
	}
	renderer, err := newPostgresRenderer(schema)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("-- Muhan canonical import schema\n")
	b.WriteString("-- Generated from static Go schema definitions; do not concatenate user input into this DDL.\n\n")
	if renderer.schema != "" {
		fmt.Fprintf(&b, "CREATE SCHEMA IF NOT EXISTS %s;\n\n", renderer.quote(renderer.schema))
	}
	for _, table := range manifest.Tables {
		renderer.writeTableDDL(&b, table)
	}
	for _, table := range manifest.Tables {
		for _, index := range table.Indexes {
			renderer.writeIndexDDL(&b, table, index)
		}
	}
	return b.String(), nil
}

func Write(outdir string, opts Options) (Manifest, error) {
	if outdir == "" {
		return Manifest{}, fmt.Errorf("missing output directory")
	}
	manifest, err := Build(opts)
	if err != nil {
		return Manifest{}, err
	}
	ddl, err := PostgresDDL(manifest)
	if err != nil {
		return Manifest{}, err
	}
	if err := os.MkdirAll(outdir, 0700); err != nil {
		return Manifest{}, fmt.Errorf("create output directory %q: %w", outdir, err)
	}
	sqlPath := filepath.Join(outdir, postgresSchemaFile)
	if err := jsonstore.WriteBytes(sqlPath, []byte(ddl)); err != nil {
		return Manifest{}, fmt.Errorf("write %s: %w", sqlPath, err)
	}
	manifest.Files = []ArtifactFile{
		artifactFile(postgresSchemaFile, "sql", sqlPath),
	}
	if err := jsonstore.WriteJSON(filepath.Join(outdir, manifestFile), manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

type postgresRenderer struct {
	schema string
	quoted bool
}

func newPostgresRenderer(schema string) (postgresRenderer, error) {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return postgresRenderer{}, nil
	}
	if err := ValidateIdentifier(schema); err != nil {
		return postgresRenderer{}, fmt.Errorf("schema %q: %w", schema, err)
	}
	return postgresRenderer{schema: schema, quoted: true}, nil
}

func (r postgresRenderer) writeTableDDL(b *strings.Builder, table Table) {
	fmt.Fprintf(b, "CREATE TABLE IF NOT EXISTS %s (\n", r.tableName(table.Name))
	lines := make([]string, 0, len(table.Columns)+len(table.ForeignKeys)+1)
	for _, column := range table.Columns {
		line := fmt.Sprintf("  %s %s", r.ident(column.Name), column.Type)
		if !column.Nullable {
			line += " NOT NULL"
		}
		if column.Default != "" {
			line += " DEFAULT " + column.Default
		}
		lines = append(lines, line)
	}
	if len(table.PrimaryKey) > 0 {
		lines = append(lines, fmt.Sprintf("  PRIMARY KEY (%s)", strings.Join(r.identList(table.PrimaryKey), ", ")))
	}
	for _, fk := range table.ForeignKeys {
		line := fmt.Sprintf("  FOREIGN KEY (%s) REFERENCES %s (%s)", strings.Join(r.identList(fk.Columns), ", "), r.tableName(fk.RefTable), strings.Join(r.identList(fk.RefColumns), ", "))
		if fk.OnDelete != "" {
			line += " ON DELETE " + fk.OnDelete
		}
		lines = append(lines, line)
	}
	b.WriteString(strings.Join(lines, ",\n"))
	b.WriteString("\n);\n\n")
}

func (r postgresRenderer) writeIndexDDL(b *strings.Builder, table Table, index Index) {
	unique := ""
	if index.Unique {
		unique = "UNIQUE "
	}
	target := ""
	if index.Expression != "" {
		target = index.Expression
	} else {
		target = strings.Join(r.identList(index.Columns), ", ")
	}
	method := ""
	if index.Method != "" {
		method = " USING " + index.Method
	}
	fmt.Fprintf(b, "CREATE %sINDEX IF NOT EXISTS %s ON %s%s (%s);\n", unique, r.indexName(index.Name), r.tableName(table.Name), method, target)
}

func (r postgresRenderer) tableName(name string) string {
	if r.schema == "" {
		return name
	}
	return r.quote(r.schema) + "." + r.quote(name)
}

func (r postgresRenderer) indexName(name string) string {
	if !r.quoted {
		return name
	}
	return r.quote(name)
}

func (r postgresRenderer) ident(name string) string {
	if !r.quoted {
		return name
	}
	return r.quote(name)
}

func (r postgresRenderer) identList(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, r.ident(name))
	}
	return out
}

func (r postgresRenderer) quote(name string) string {
	return `"` + name + `"`
}

func validateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema version = %q, want %q", manifest.SchemaVersion, SchemaVersion)
	}
	if manifest.Dialect != DialectPostgres {
		return fmt.Errorf("unsupported dialect %q", manifest.Dialect)
	}
	seenTables := map[string]map[string]struct{}{}
	seenIndexes := map[string]struct{}{}
	for _, table := range manifest.Tables {
		if err := validateIdent(table.Name); err != nil {
			return fmt.Errorf("table %q: %w", table.Name, err)
		}
		if _, exists := seenTables[table.Name]; exists {
			return fmt.Errorf("duplicate table %q", table.Name)
		}
		seenColumns := map[string]struct{}{}
		for _, column := range table.Columns {
			if err := validateIdent(column.Name); err != nil {
				return fmt.Errorf("table %s column %q: %w", table.Name, column.Name, err)
			}
			if _, exists := seenColumns[column.Name]; exists {
				return fmt.Errorf("table %s duplicate column %q", table.Name, column.Name)
			}
			seenColumns[column.Name] = struct{}{}
		}
		for _, column := range table.PrimaryKey {
			if _, exists := seenColumns[column]; !exists {
				return fmt.Errorf("table %s primary key references missing column %q", table.Name, column)
			}
		}
		for _, fk := range table.ForeignKeys {
			refColumns, exists := seenTables[fk.RefTable]
			if !exists {
				return fmt.Errorf("table %s foreign key references table %q before definition", table.Name, fk.RefTable)
			}
			if len(fk.Columns) != len(fk.RefColumns) {
				return fmt.Errorf("table %s foreign key to %s has %d columns and %d referenced columns", table.Name, fk.RefTable, len(fk.Columns), len(fk.RefColumns))
			}
			for _, column := range fk.Columns {
				if _, exists := seenColumns[column]; !exists {
					return fmt.Errorf("table %s foreign key references missing column %q", table.Name, column)
				}
			}
			for _, column := range fk.RefColumns {
				if _, exists := refColumns[column]; !exists {
					return fmt.Errorf("table %s foreign key references missing column %q on %s", table.Name, column, fk.RefTable)
				}
			}
			if err := validateOnDelete(fk.OnDelete); err != nil {
				return fmt.Errorf("table %s foreign key to %s: %w", table.Name, fk.RefTable, err)
			}
		}
		for _, index := range table.Indexes {
			if err := validateIdent(index.Name); err != nil {
				return fmt.Errorf("index %q: %w", index.Name, err)
			}
			if _, exists := seenIndexes[index.Name]; exists {
				return fmt.Errorf("duplicate index %q", index.Name)
			}
			seenIndexes[index.Name] = struct{}{}
			if index.Method != "" {
				if err := validateIdent(index.Method); err != nil {
					return fmt.Errorf("index %s method %q: %w", index.Name, index.Method, err)
				}
			}
			if index.Expression == "" {
				for _, column := range index.Columns {
					if _, exists := seenColumns[column]; !exists {
						return fmt.Errorf("index %s references missing column %q", index.Name, column)
					}
				}
			}
		}
		seenTables[table.Name] = seenColumns
	}
	return nil
}

func validateIdent(value string) error {
	if !identRE.MatchString(value) {
		return fmt.Errorf("identifier must match %s", identRE.String())
	}
	return nil
}

func ValidateIdentifier(value string) error {
	return validateIdent(value)
}

func QualifiedName(schema, name string) (string, error) {
	if err := ValidateIdentifier(name); err != nil {
		return "", fmt.Errorf("identifier %q: %w", name, err)
	}
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return name, nil
	}
	if err := ValidateIdentifier(schema); err != nil {
		return "", fmt.Errorf("schema %q: %w", schema, err)
	}
	return `"` + schema + `"."` + name + `"`, nil
}

func QuotedIdentifier(name string) (string, error) {
	if err := ValidateIdentifier(name); err != nil {
		return "", err
	}
	return `"` + name + `"`, nil
}

func validateOnDelete(value string) error {
	switch value {
	case "", "CASCADE", "RESTRICT", "SET NULL", "SET DEFAULT", "NO ACTION":
		return nil
	default:
		return fmt.Errorf("unsupported ON DELETE action %q", value)
	}
}

func artifactFile(relPath, format, path string) ArtifactFile {
	file := ArtifactFile{Path: filepath.ToSlash(relPath), Format: format}
	info, err := os.Stat(path)
	if err == nil {
		file.Bytes = info.Size()
		data, err := os.ReadFile(path)
		if err == nil {
			sum := sha256.Sum256(data)
			file.SHA256 = hex.EncodeToString(sum[:])
		}
	}
	return file
}

func postgresTables() []Table {
	tables := []Table{
		importRunsTable(),
		withRunID(roomsTable()),
		withRunID(roomExitsTable()),
		withRunID(roomObjectsTable()),
		withRunID(roomCreaturesTable()),
		withRunID(roomPlayersTable()),
		withRunID(playersTable()),
		withRunID(creaturesTable()),
		withRunID(creatureInventoryTable()),
		withRunID(banksTable()),
		withRunID(bankObjectsTable()),
		withRunID(objectPrototypesTable()),
		withRunID(objectInstancesTable()),
		withRunID(objectContentsTable()),
		withRunID(boardPostsTable()),
		withRunID(prototypeResolutionEvidenceTable()),
		withRunID(worldloadFindingsTable()),
		withRunID(artifactFilesTable()),
	}
	sort.SliceStable(tables, func(i, j int) bool {
		return tableOrder(tables[i].Name) < tableOrder(tables[j].Name)
	})
	return tables
}

func tableOrder(name string) int {
	order := map[string]int{
		"import_runs":                   0,
		"rooms":                         10,
		"room_exits":                    20,
		"room_objects":                  30,
		"room_creatures":                40,
		"room_players":                  50,
		"players":                       60,
		"creatures":                     70,
		"creature_inventory":            80,
		"banks":                         90,
		"bank_objects":                  100,
		"object_prototypes":             110,
		"object_instances":              120,
		"object_contents":               130,
		"board_posts":                   140,
		"prototype_resolution_evidence": 150,
		"worldload_findings":            160,
		"artifact_files":                170,
	}
	if v, ok := order[name]; ok {
		return v
	}
	return 1000
}

func textID(name string) Column {
	return Column{Name: name, Type: `TEXT COLLATE "C"`}
}

func runIDColumn() Column {
	return textID("run_id")
}

func nullableTextID(name string) Column {
	return Column{Name: name, Type: `TEXT COLLATE "C"`, Nullable: true}
}

func jsonbColumn(name, def string) Column {
	return Column{Name: name, Type: "JSONB", Nullable: false, Default: def}
}

func withRunID(table Table) Table {
	table.Columns = append([]Column{runIDColumn()}, table.Columns...)
	table.PrimaryKey = append([]string{"run_id"}, table.PrimaryKey...)
	table.ForeignKeys = append([]ForeignKey{{
		Columns:    []string{"run_id"},
		RefTable:   "import_runs",
		RefColumns: []string{"run_id"},
		OnDelete:   "CASCADE",
	}}, table.ForeignKeys...)
	for i := range table.Indexes {
		if table.Indexes[i].Expression != "" || len(table.Indexes[i].Columns) == 0 || table.Indexes[i].Columns[0] == "run_id" {
			continue
		}
		table.Indexes[i].Columns = append([]string{"run_id"}, table.Indexes[i].Columns...)
	}
	return table
}

func importRunsTable() Table {
	return Table{
		Name:    "import_runs",
		Purpose: "One row per DB import or verification run.",
		Columns: []Column{
			textID("run_id"),
			{Name: "schema_version", Type: "TEXT", Nullable: false},
			{Name: "generated_at", Type: "TIMESTAMPTZ", Nullable: false},
			{Name: "source_root", Type: "TEXT", Nullable: false},
			jsonbColumn("manifest", "'{}'::jsonb"),
		},
		PrimaryKey: []string{"run_id"},
	}
}

func roomsTable() Table {
	return Table{
		Name:    "rooms",
		Purpose: "Canonical rooms decoded from legacy room files.",
		Source:  "world.rooms",
		Columns: []Column{
			textID("room_id"),
			{Name: "display_name", Type: "TEXT", Nullable: false},
			{Name: "short_description", Type: "TEXT", Nullable: true},
			{Name: "long_description", Type: "TEXT", Nullable: true},
			{Name: "object_description", Type: "TEXT", Nullable: true},
			jsonbColumn("properties", "'{}'::jsonb"),
			jsonbColumn("metadata", "'{}'::jsonb"),
		},
		PrimaryKey: []string{"room_id"},
		Indexes:    []Index{{Name: "idx_rooms_metadata_gin", Expression: "metadata jsonb_path_ops", Method: "gin"}},
	}
}

func roomExitsTable() Table {
	return Table{
		Name:    "room_exits",
		Purpose: "Ordered room exits.",
		Source:  "world.rooms[].exits",
		Columns: []Column{
			textID("room_id"),
			{Name: "exit_index", Type: "INTEGER", Nullable: false},
			{Name: "name", Type: "TEXT", Nullable: false},
			textID("to_room_id"),
			jsonbColumn("flags", "'[]'::jsonb"),
			jsonbColumn("metadata", "'{}'::jsonb"),
		},
		PrimaryKey:  []string{"room_id", "exit_index"},
		ForeignKeys: []ForeignKey{{Columns: []string{"run_id", "room_id"}, RefTable: "rooms", RefColumns: []string{"run_id", "room_id"}, OnDelete: "CASCADE"}},
		Indexes:     []Index{{Name: "idx_room_exits_to_room_id", Columns: []string{"to_room_id"}}},
	}
}

func roomObjectsTable() Table {
	return refTable("room_objects", "rooms", "room_id", "object_id", "Ordered root object references in rooms.")
}

func roomCreaturesTable() Table {
	return refTable("room_creatures", "rooms", "room_id", "creature_id", "Ordered creature references in rooms.")
}

func roomPlayersTable() Table {
	return refTable("room_players", "rooms", "room_id", "player_id", "Player references retained from room model.")
}

func playersTable() Table {
	return Table{
		Name:    "players",
		Purpose: "Canonical players decoded from legacy player files.",
		Source:  "world.players",
		Columns: []Column{
			textID("player_id"),
			{Name: "display_name", Type: "TEXT", Nullable: false},
			nullableTextID("creature_id"),
			nullableTextID("room_id"),
			{Name: "account_name", Type: "TEXT", Nullable: true},
			jsonbColumn("metadata", "'{}'::jsonb"),
		},
		PrimaryKey: []string{"player_id"},
		Indexes: []Index{
			{Name: "idx_players_creature_id", Columns: []string{"creature_id"}},
			{Name: "idx_players_room_id", Columns: []string{"room_id"}},
			{Name: "idx_players_metadata_gin", Expression: "metadata jsonb_path_ops", Method: "gin"},
		},
	}
}

func creaturesTable() Table {
	return Table{
		Name:    "creatures",
		Purpose: "Canonical players, NPCs, and monsters.",
		Source:  "world.creatures",
		Columns: []Column{
			textID("creature_id"),
			{Name: "kind", Type: "TEXT", Nullable: false},
			{Name: "display_name", Type: "TEXT", Nullable: false},
			{Name: "description", Type: "TEXT", Nullable: true},
			{Name: "level", Type: "INTEGER", Nullable: true},
			nullableTextID("room_id"),
			nullableTextID("player_id"),
			jsonbColumn("equipment", "'{}'::jsonb"),
			jsonbColumn("stats", "'{}'::jsonb"),
			jsonbColumn("properties", "'{}'::jsonb"),
			jsonbColumn("metadata", "'{}'::jsonb"),
		},
		PrimaryKey: []string{"creature_id"},
		Indexes: []Index{
			{Name: "idx_creatures_kind", Columns: []string{"kind"}},
			{Name: "idx_creatures_room_id", Columns: []string{"room_id"}},
			{Name: "idx_creatures_player_id", Columns: []string{"player_id"}},
		},
	}
}

func creatureInventoryTable() Table {
	return refTable("creature_inventory", "creatures", "creature_id", "object_id", "Ordered root inventory references for creatures.")
}

func banksTable() Table {
	return Table{
		Name:    "banks",
		Purpose: "Player and family bank accounts.",
		Source:  "world.banks",
		Columns: []Column{
			textID("bank_id"),
			{Name: "kind", Type: "TEXT", Nullable: false},
			{Name: "owner_name", Type: "TEXT", Nullable: false},
			nullableTextID("owner_player_id"),
			jsonbColumn("metadata", "'{}'::jsonb"),
		},
		PrimaryKey: []string{"bank_id"},
		Indexes: []Index{
			{Name: "idx_banks_kind", Columns: []string{"kind"}},
			{Name: "idx_banks_owner_player_id", Columns: []string{"owner_player_id"}},
		},
	}
}

func bankObjectsTable() Table {
	return refTable("bank_objects", "banks", "bank_id", "object_id", "Ordered root object references in bank accounts.")
}

func objectPrototypesTable() Table {
	return Table{
		Name:    "object_prototypes",
		Purpose: "Legacy objmon prototypes plus materialized synthetic prototypes.",
		Source:  "world.objectPrototypes",
		Columns: []Column{
			textID("prototype_id"),
			{Name: "kind", Type: "TEXT", Nullable: true},
			{Name: "display_name", Type: "TEXT", Nullable: false},
			{Name: "description", Type: "TEXT", Nullable: true},
			jsonbColumn("keywords", "'[]'::jsonb"),
			jsonbColumn("properties", "'{}'::jsonb"),
			jsonbColumn("metadata", "'{}'::jsonb"),
		},
		PrimaryKey: []string{"prototype_id"},
		Indexes:    []Index{{Name: "idx_object_prototypes_metadata_gin", Expression: "metadata jsonb_path_ops", Method: "gin"}},
	}
}

func objectInstancesTable() Table {
	return Table{
		Name:    "object_instances",
		Purpose: "Canonical object instances with exactly one holder location.",
		Source:  "world.objects",
		Columns: []Column{
			textID("object_id"),
			textID("prototype_id"),
			{Name: "display_name_override", Type: "TEXT", Nullable: true},
			{Name: "quantity", Type: "INTEGER CHECK (quantity >= 0)", Nullable: false, Default: "1"},
			nullableTextID("room_id"),
			nullableTextID("creature_id"),
			nullableTextID("bank_id"),
			nullableTextID("container_id"),
			{Name: "slot", Type: "TEXT", Nullable: true},
			jsonbColumn("properties", "'{}'::jsonb"),
			jsonbColumn("metadata", "'{}'::jsonb"),
			{Name: "holder_count", Type: "INTEGER GENERATED ALWAYS AS (num_nonnulls(room_id, creature_id, bank_id, container_id)) STORED CHECK (holder_count = 1)", Nullable: false},
		},
		PrimaryKey:  []string{"object_id"},
		ForeignKeys: []ForeignKey{{Columns: []string{"run_id", "prototype_id"}, RefTable: "object_prototypes", RefColumns: []string{"run_id", "prototype_id"}, OnDelete: "CASCADE"}},
		Indexes: []Index{
			{Name: "idx_object_instances_prototype_id", Columns: []string{"prototype_id"}},
			{Name: "idx_object_instances_room_id", Columns: []string{"room_id"}},
			{Name: "idx_object_instances_creature_id", Columns: []string{"creature_id"}},
			{Name: "idx_object_instances_bank_id", Columns: []string{"bank_id"}},
			{Name: "idx_object_instances_container_id", Columns: []string{"container_id"}},
			{Name: "idx_object_instances_metadata_gin", Expression: "metadata jsonb_path_ops", Method: "gin"},
		},
	}
}

func objectContentsTable() Table {
	return refTable("object_contents", "object_instances", "container_id", "child_object_id", "Ordered nested object contents.")
}

func boardPostsTable() Table {
	return Table{
		Name:    "board_posts",
		Purpose: "Board posts and notice data.",
		Source:  "world.boardPosts or boardmap output",
		Columns: []Column{
			textID("post_id"),
			textID("board_id"),
			{Name: "title", Type: "TEXT", Nullable: false},
			nullableTextID("author_id"),
			{Name: "author_name", Type: "TEXT", Nullable: true},
			{Name: "body", Type: "TEXT", Nullable: false},
			{Name: "created_at", Type: "TIMESTAMPTZ", Nullable: true},
			{Name: "read_count", Type: "INTEGER", Nullable: false, Default: "0"},
			jsonbColumn("metadata", "'{}'::jsonb"),
		},
		PrimaryKey: []string{"post_id"},
		Indexes: []Index{
			{Name: "idx_board_posts_board_id", Columns: []string{"board_id"}},
			{Name: "idx_board_posts_author_id", Columns: []string{"author_id"}},
		},
	}
}

func prototypeResolutionEvidenceTable() Table {
	return Table{
		Name:    "prototype_resolution_evidence",
		Purpose: "Sidecar audit rows from muhan-protoaudit.",
		Source:  "protoaudit.prototype_resolution_evidence.jsonl",
		Columns: []Column{
			textID("evidence_id"),
			textID("object_instance_id"),
			textID("prototype_id"),
			{Name: "status", Type: "TEXT", Nullable: false},
			{Name: "method", Type: "TEXT", Nullable: true},
			{Name: "confidence", Type: "TEXT", Nullable: true},
			nullableTextID("selected_prototype_id"),
			nullableTextID("synthetic_prototype_id"),
			{Name: "candidate_count", Type: "INTEGER", Nullable: false, Default: "0"},
			{Name: "candidate_cap", Type: "INTEGER", Nullable: false, Default: "0"},
			{Name: "candidates_truncated", Type: "BOOLEAN", Nullable: false, Default: "FALSE"},
			{Name: "fingerprint", Type: "TEXT", Nullable: true},
			{Name: "fingerprint_algorithm", Type: "TEXT", Nullable: true},
			{Name: "comparable_bytes", Type: "INTEGER", Nullable: true},
			jsonbColumn("source", "'{}'::jsonb"),
			jsonbColumn("c_format", "'{}'::jsonb"),
			jsonbColumn("resolution", "'{}'::jsonb"),
			jsonbColumn("tags", "'[]'::jsonb"),
		},
		PrimaryKey:  []string{"evidence_id"},
		ForeignKeys: []ForeignKey{{Columns: []string{"run_id", "object_instance_id"}, RefTable: "object_instances", RefColumns: []string{"run_id", "object_id"}, OnDelete: "CASCADE"}},
		Indexes: []Index{
			{Name: "idx_proto_evidence_object_instance_id", Columns: []string{"object_instance_id"}},
			{Name: "idx_proto_evidence_status", Columns: []string{"status"}},
			{Name: "idx_proto_evidence_fingerprint", Columns: []string{"fingerprint"}},
			{Name: "idx_proto_evidence_resolution_gin", Expression: "resolution jsonb_path_ops", Method: "gin"},
		},
	}
}

func worldloadFindingsTable() Table {
	return Table{
		Name:    "worldload_findings",
		Purpose: "Warnings and errors emitted by worldload and protoaudit.",
		Source:  "protoaudit.worldload_findings.jsonl",
		Columns: []Column{
			{Name: "finding_id", Type: "BIGSERIAL", Nullable: false},
			{Name: "severity", Type: "TEXT", Nullable: false},
			{Name: "kind", Type: "TEXT", Nullable: false},
			{Name: "path", Type: "TEXT", Nullable: true},
			{Name: "entity_id", Type: "TEXT", Nullable: true},
			{Name: "ref", Type: "TEXT", Nullable: true},
			{Name: "message", Type: "TEXT", Nullable: false},
			jsonbColumn("source", "'{}'::jsonb"),
		},
		PrimaryKey: []string{"finding_id"},
		Indexes: []Index{
			{Name: "idx_worldload_findings_severity", Columns: []string{"severity"}},
			{Name: "idx_worldload_findings_kind", Columns: []string{"kind"}},
			{Name: "idx_worldload_findings_path", Columns: []string{"path"}},
		},
	}
}

func artifactFilesTable() Table {
	return Table{
		Name:    "artifact_files",
		Purpose: "Generated migration artifact hashes and sizes.",
		Source:  "protoaudit.index or dbschema manifest",
		Columns: []Column{
			{Name: "path", Type: "TEXT COLLATE \"C\"", Nullable: false},
			{Name: "format", Type: "TEXT", Nullable: false},
			{Name: "records", Type: "INTEGER", Nullable: true},
			{Name: "bytes", Type: "BIGINT", Nullable: true},
			{Name: "sha256", Type: "CHAR(64)", Nullable: true},
			jsonbColumn("metadata", "'{}'::jsonb"),
		},
		PrimaryKey: []string{"path"},
	}
}

func refTable(name, parentTable, parentID, childID, purpose string) Table {
	columns := []Column{
		textID(parentID),
		textID(childID),
		{Name: "ref_index", Type: "INTEGER", Nullable: false},
		jsonbColumn("metadata", "'{}'::jsonb"),
	}
	table := Table{
		Name:       name,
		Purpose:    purpose,
		Columns:    columns,
		PrimaryKey: []string{parentID, childID},
		Indexes: []Index{
			{Name: "idx_" + name + "_child", Columns: []string{childID}},
		},
	}
	refColumn := parentID
	switch parentTable {
	case "rooms":
		refColumn = "room_id"
	case "creatures":
		refColumn = "creature_id"
	case "banks":
		refColumn = "bank_id"
	case "object_instances":
		refColumn = "object_id"
	}
	table.ForeignKeys = []ForeignKey{{Columns: []string{"run_id", parentID}, RefTable: parentTable, RefColumns: []string{"run_id", refColumn}, OnDelete: "CASCADE"}}
	return table
}
