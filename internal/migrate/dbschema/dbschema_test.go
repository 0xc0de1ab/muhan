package dbschema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestBuildPostgresSchemaManifest(t *testing.T) {
	generatedAt := time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC)
	got, err := Build(Options{Dialect: DialectPostgres, GeneratedAt: generatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != SchemaVersion || got.Dialect != DialectPostgres || !got.GeneratedAt.Equal(generatedAt) {
		t.Fatalf("manifest header = %+v", got)
	}
	for _, name := range []string{"rooms", "object_instances", "object_prototypes", "prototype_resolution_evidence", "worldload_findings"} {
		if !hasTable(got.Tables, name) {
			t.Fatalf("missing table %q in %+v", name, tableNames(got.Tables))
		}
	}
	table := tableByName(got.Tables, "prototype_resolution_evidence")
	if !hasColumn(table.Columns, "evidence_id") || !hasColumn(table.Columns, "resolution") || !hasColumn(table.Columns, "c_format") {
		t.Fatalf("prototype evidence columns = %+v", table.Columns)
	}
}

func TestRunScopedTablesUseRunIDKeysAndIndexes(t *testing.T) {
	manifest, err := Build(Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range manifest.Tables {
		if table.Name == "import_runs" {
			continue
		}
		if len(table.Columns) == 0 || table.Columns[0].Name != "run_id" {
			t.Fatalf("%s first column = %+v, want run_id", table.Name, table.Columns)
		}
		if len(table.PrimaryKey) == 0 || table.PrimaryKey[0] != "run_id" {
			t.Fatalf("%s primary key = %+v, want run_id first", table.Name, table.PrimaryKey)
		}
		if !hasForeignKey(table.ForeignKeys, "import_runs", []string{"run_id"}, []string{"run_id"}) {
			t.Fatalf("%s missing import_runs run_id foreign key: %+v", table.Name, table.ForeignKeys)
		}
		for _, index := range table.Indexes {
			if index.Expression == "" && len(index.Columns) > 0 && index.Columns[0] != "run_id" {
				t.Fatalf("%s index %s columns = %+v, want run_id first", table.Name, index.Name, index.Columns)
			}
		}
	}
}

func TestPostgresDDLIsDeterministicAndStatic(t *testing.T) {
	manifest, err := Build(Options{GeneratedAt: time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	first, err := PostgresDDL(manifest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PostgresDDL(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("DDL is not deterministic")
	}
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS rooms",
		"CREATE TABLE IF NOT EXISTS object_instances",
		"holder_count INTEGER GENERATED ALWAYS AS (num_nonnulls(room_id, creature_id, bank_id, container_id)) STORED CHECK (holder_count = 1)",
		"FOREIGN KEY (run_id, prototype_id) REFERENCES object_prototypes (run_id, prototype_id) ON DELETE CASCADE",
		"CREATE INDEX IF NOT EXISTS idx_proto_evidence_resolution_gin ON prototype_resolution_evidence USING gin (resolution jsonb_path_ops);",
	} {
		if !strings.Contains(first, want) {
			t.Fatalf("DDL missing %q\n%s", want, first)
		}
	}
	if strings.Contains(first, "%!") || strings.Contains(first, "?") {
		t.Fatalf("DDL appears to contain formatting or placeholder residue:\n%s", first)
	}
}

func TestPostgresDDLForSchemaQualifiesObjects(t *testing.T) {
	manifest, err := Build(Options{})
	if err != nil {
		t.Fatal(err)
	}
	ddl, err := PostgresDDLForSchema(manifest, "muhan_import")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`CREATE SCHEMA IF NOT EXISTS "muhan_import";`,
		`CREATE TABLE IF NOT EXISTS "muhan_import"."rooms"`,
		`"run_id" TEXT COLLATE "C" NOT NULL`,
		`PRIMARY KEY ("run_id", "room_id")`,
		`FOREIGN KEY ("run_id") REFERENCES "muhan_import"."import_runs" ("run_id") ON DELETE CASCADE`,
		`CREATE INDEX IF NOT EXISTS "idx_room_exits_to_room_id" ON "muhan_import"."room_exits" ("run_id", "to_room_id");`,
	} {
		if !strings.Contains(ddl, want) {
			t.Fatalf("schema-qualified DDL missing %q\n%s", want, ddl)
		}
	}
	if _, err := PostgresDDLForSchema(manifest, "bad-schema"); err == nil {
		t.Fatal("PostgresDDLForSchema accepted unsafe schema")
	}
}

func TestWriteSchemaArtifacts(t *testing.T) {
	outdir := t.TempDir()
	manifest, err := Write(outdir, Options{GeneratedAt: time.Date(2026, 5, 20, 1, 2, 3, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 1 || manifest.Files[0].Path != postgresSchemaFile || manifest.Files[0].SHA256 == "" {
		t.Fatalf("files = %+v", manifest.Files)
	}
	if _, err := os.Stat(filepath.Join(outdir, postgresSchemaFile)); err != nil {
		t.Fatal(err)
	}
	var decoded Manifest
	data, err := os.ReadFile(filepath.Join(outdir, manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != SchemaVersion || len(decoded.Tables) != len(manifest.Tables) {
		t.Fatalf("decoded manifest = %+v", decoded)
	}
}

func TestQualifiedNameValidatesSchemaAndIdentifier(t *testing.T) {
	got, err := QualifiedName("muhan_import", "rooms")
	if err != nil {
		t.Fatal(err)
	}
	if got != `"muhan_import"."rooms"` {
		t.Fatalf("qualified name = %q", got)
	}
	if _, err := QualifiedName("bad-schema", "rooms"); err == nil {
		t.Fatal("QualifiedName accepted unsafe schema")
	}
	if _, err := QualifiedName("muhan_import", "rooms;drop"); err == nil {
		t.Fatal("QualifiedName accepted unsafe table")
	}
}

func TestValidateCatalogAcceptsCanonicalTablesAndColumns(t *testing.T) {
	manifest, err := Build(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if issues := ValidateCatalog(manifest, catalogFromManifest(manifest)); len(issues) != 0 {
		t.Fatalf("issues = %+v, want none", issues)
	}
}

func TestValidateCatalogReportsMissingTableAndColumn(t *testing.T) {
	manifest, err := Build(Options{})
	if err != nil {
		t.Fatal(err)
	}
	catalog := catalogFromManifest(manifest)
	delete(catalog.Tables, "rooms")
	delete(catalog.Tables["object_instances"].Columns, "holder_count")

	issues := ValidateCatalog(manifest, catalog)
	if !hasCatalogIssue(issues, "rooms", "", "missing table rooms") {
		t.Fatalf("issues = %+v, want missing rooms table", issues)
	}
	if !hasCatalogIssue(issues, "object_instances", "holder_count", "missing column object_instances.holder_count") {
		t.Fatalf("issues = %+v, want missing holder_count column", issues)
	}
}

func TestValidateCatalogReportsColumnShapeDrift(t *testing.T) {
	manifest, err := Build(Options{})
	if err != nil {
		t.Fatal(err)
	}
	catalog := catalogFromManifest(manifest)
	metadata := catalog.Tables["rooms"].Columns["metadata"]
	metadata.Type = "text"
	catalog.Tables["rooms"].Columns["metadata"] = metadata

	body := catalog.Tables["board_posts"].Columns["body"]
	body.Nullable = true
	catalog.Tables["board_posts"].Columns["body"] = body

	readCount := catalog.Tables["board_posts"].Columns["read_count"]
	readCount.Default = ""
	catalog.Tables["board_posts"].Columns["read_count"] = readCount

	holderCount := catalog.Tables["object_instances"].Columns["holder_count"]
	holderCount.Generated = false
	catalog.Tables["object_instances"].Columns["holder_count"] = holderCount

	findingID := catalog.Tables["worldload_findings"].Columns["finding_id"]
	findingID.Serial = false
	catalog.Tables["worldload_findings"].Columns["finding_id"] = findingID

	issues := ValidateCatalog(manifest, catalog)
	for _, want := range []CatalogIssue{
		{Table: "rooms", Column: "metadata", Message: "column rooms.metadata type = text, want jsonb"},
		{Table: "object_instances", Column: "holder_count", Message: "column object_instances.holder_count generated = false, want true"},
		{Table: "board_posts", Column: "body", Message: "column board_posts.body nullable = true, want false"},
		{Table: "board_posts", Column: "read_count", Message: `column board_posts.read_count default = "", want "0"`},
		{Table: "worldload_findings", Column: "finding_id", Message: "column worldload_findings.finding_id serial = false, want true"},
	} {
		if !hasCatalogIssue(issues, want.Table, want.Column, want.Message) {
			t.Fatalf("issues = %+v, want %+v", issues, want)
		}
	}
}

func TestExpectedCatalogColumnMappings(t *testing.T) {
	tests := []struct {
		name string
		in   Column
		want CatalogColumn
	}{
		{
			name: "text collation",
			in:   Column{Name: "id", Type: `TEXT COLLATE "C"`},
			want: CatalogColumn{Type: "text", CollationName: "C"},
		},
		{
			name: "jsonb default",
			in:   Column{Name: "metadata", Type: "JSONB", Default: "'{}'::jsonb"},
			want: CatalogColumn{Type: "jsonb", Default: "'{}'::jsonb"},
		},
		{
			name: "bigserial",
			in:   Column{Name: "finding_id", Type: "BIGSERIAL"},
			want: CatalogColumn{Type: "bigint", Serial: true},
		},
		{
			name: "generated integer",
			in:   Column{Name: "holder_count", Type: "INTEGER GENERATED ALWAYS AS (num_nonnulls(room_id, creature_id, bank_id, container_id)) STORED CHECK (holder_count = 1)"},
			want: CatalogColumn{Type: "integer", Generated: true},
		},
		{
			name: "fixed char",
			in:   Column{Name: "sha256", Type: "CHAR(64)", Nullable: true},
			want: CatalogColumn{Type: "character", Nullable: true, CharacterMaximumLength: 64},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expectedCatalogColumn(tt.in)
			if got.Type != tt.want.Type ||
				got.Nullable != tt.want.Nullable ||
				got.Default != tt.want.Default ||
				got.Generated != tt.want.Generated ||
				got.Serial != tt.want.Serial ||
				got.CollationName != tt.want.CollationName ||
				got.CharacterMaximumLength != tt.want.CharacterMaximumLength {
				t.Fatalf("expectedCatalogColumn(%+v) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSchemaIdentifiersAreLowerSnakeCase(t *testing.T) {
	manifest, err := Build(Options{})
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	for _, table := range manifest.Tables {
		if !re.MatchString(table.Name) {
			t.Fatalf("table identifier = %q", table.Name)
		}
		for _, column := range table.Columns {
			if !re.MatchString(column.Name) {
				t.Fatalf("column identifier = %q.%q", table.Name, column.Name)
			}
		}
		for _, index := range table.Indexes {
			if !re.MatchString(index.Name) {
				t.Fatalf("index identifier = %q", index.Name)
			}
		}
	}
}

func hasTable(tables []Table, name string) bool {
	return tableByName(tables, name).Name != ""
}

func tableByName(tables []Table, name string) Table {
	for _, table := range tables {
		if table.Name == name {
			return table
		}
	}
	return Table{}
}

func tableNames(tables []Table) []string {
	names := make([]string, 0, len(tables))
	for _, table := range tables {
		names = append(names, table.Name)
	}
	return names
}

func hasColumn(columns []Column, name string) bool {
	for _, column := range columns {
		if column.Name == name {
			return true
		}
	}
	return false
}

func hasForeignKey(fks []ForeignKey, refTable string, columns, refColumns []string) bool {
	for _, fk := range fks {
		if fk.RefTable == refTable && sameStrings(fk.Columns, columns) && sameStrings(fk.RefColumns, refColumns) {
			return true
		}
	}
	return false
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func catalogFromManifest(manifest Manifest) Catalog {
	catalog := Catalog{Tables: map[string]CatalogTable{}}
	for _, table := range manifest.Tables {
		columns := map[string]CatalogColumn{}
		for _, column := range table.Columns {
			columns[column.Name] = expectedCatalogColumn(column)
		}
		catalog.Tables[table.Name] = CatalogTable{Columns: columns}
	}
	return catalog
}

func hasCatalogIssue(issues []CatalogIssue, table, column, message string) bool {
	for _, issue := range issues {
		if issue.Table == table && issue.Column == column && issue.Message == message {
			return true
		}
	}
	return false
}
