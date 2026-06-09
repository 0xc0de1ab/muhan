package dbschema

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Catalog struct {
	Tables map[string]CatalogTable
}

type CatalogTable struct {
	Columns map[string]CatalogColumn
}

type CatalogColumn struct {
	Type                   string
	Nullable               bool
	Default                string
	Generated              bool
	GenerationExpression   string
	Serial                 bool
	CollationName          string
	CharacterMaximumLength int
}

type CatalogIssue struct {
	Table   string
	Column  string
	Message string
}

type CatalogQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func LoadPostgresCatalog(ctx context.Context, q CatalogQueryer) (Catalog, error) {
	return LoadPostgresCatalogForSchema(ctx, q, "")
}

func LoadPostgresCatalogForSchema(ctx context.Context, q CatalogQueryer, schema string) (Catalog, error) {
	if q == nil {
		return Catalog{}, fmt.Errorf("queryer is required")
	}
	schema = strings.TrimSpace(schema)
	if schema != "" {
		if err := ValidateIdentifier(schema); err != nil {
			return Catalog{}, fmt.Errorf("schema %q: %w", schema, err)
		}
	}
	rows, err := q.QueryContext(ctx, `
SELECT table_name,
       column_name,
       data_type,
       is_nullable,
       COALESCE(column_default, ''),
       is_generated,
       COALESCE(generation_expression, ''),
       COALESCE(collation_name, ''),
       COALESCE(character_maximum_length, 0)
FROM information_schema.columns
WHERE table_schema = CASE WHEN $1::text = '' THEN current_schema() ELSE $1::text END
ORDER BY table_name, ordinal_position`, schema)
	if err != nil {
		return Catalog{}, err
	}
	defer rows.Close()

	catalog := Catalog{Tables: map[string]CatalogTable{}}
	for rows.Next() {
		var tableName, columnName, dataType, isNullable, columnDefault, isGenerated, generationExpression, collationName string
		var characterMaximumLength int
		if err := rows.Scan(
			&tableName,
			&columnName,
			&dataType,
			&isNullable,
			&columnDefault,
			&isGenerated,
			&generationExpression,
			&collationName,
			&characterMaximumLength,
		); err != nil {
			return Catalog{}, err
		}
		table := catalog.Tables[tableName]
		if table.Columns == nil {
			table.Columns = map[string]CatalogColumn{}
		}
		table.Columns[columnName] = CatalogColumn{
			Type:                   normalizeCatalogType(dataType),
			Nullable:               strings.EqualFold(isNullable, "YES"),
			Default:                strings.TrimSpace(columnDefault),
			Generated:              !strings.EqualFold(isGenerated, "NEVER") && strings.TrimSpace(isGenerated) != "",
			GenerationExpression:   strings.TrimSpace(generationExpression),
			Serial:                 isSerialDefault(columnDefault),
			CollationName:          strings.TrimSpace(collationName),
			CharacterMaximumLength: characterMaximumLength,
		}
		catalog.Tables[tableName] = table
	}
	if err := rows.Err(); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func ValidateCatalog(manifest Manifest, catalog Catalog) []CatalogIssue {
	issues := make([]CatalogIssue, 0)
	for _, expectedTable := range manifest.Tables {
		actualTable, ok := catalog.Tables[expectedTable.Name]
		if !ok {
			issues = append(issues, CatalogIssue{
				Table:   expectedTable.Name,
				Message: fmt.Sprintf("missing table %s", expectedTable.Name),
			})
			continue
		}
		for _, expectedColumn := range expectedTable.Columns {
			actualColumn, ok := actualTable.Columns[expectedColumn.Name]
			if !ok {
				issues = append(issues, CatalogIssue{
					Table:   expectedTable.Name,
					Column:  expectedColumn.Name,
					Message: fmt.Sprintf("missing column %s.%s", expectedTable.Name, expectedColumn.Name),
				})
				continue
			}
			issues = append(issues, validateCatalogColumn(expectedTable.Name, expectedColumn, actualColumn)...)
		}
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Table != issues[j].Table {
			return tableOrder(issues[i].Table) < tableOrder(issues[j].Table)
		}
		return issues[i].Column < issues[j].Column
	})
	return issues
}

func validateCatalogColumn(table string, expected Column, actual CatalogColumn) []CatalogIssue {
	want := expectedCatalogColumn(expected)
	issues := make([]CatalogIssue, 0)
	add := func(message string) {
		issues = append(issues, CatalogIssue{Table: table, Column: expected.Name, Message: message})
	}
	if actual.Type != want.Type {
		add(fmt.Sprintf("column %s.%s type = %s, want %s", table, expected.Name, actual.Type, want.Type))
	}
	if actual.Nullable != want.Nullable {
		add(fmt.Sprintf("column %s.%s nullable = %t, want %t", table, expected.Name, actual.Nullable, want.Nullable))
	}
	if !want.Serial && normalizeDefault(actual.Default) != normalizeDefault(want.Default) {
		add(fmt.Sprintf("column %s.%s default = %q, want %q", table, expected.Name, actual.Default, want.Default))
	}
	if actual.Generated != want.Generated {
		add(fmt.Sprintf("column %s.%s generated = %t, want %t", table, expected.Name, actual.Generated, want.Generated))
	}
	if actual.Serial != want.Serial {
		add(fmt.Sprintf("column %s.%s serial = %t, want %t", table, expected.Name, actual.Serial, want.Serial))
	}
	if want.CollationName != "" && actual.CollationName != want.CollationName {
		add(fmt.Sprintf("column %s.%s collation = %q, want %q", table, expected.Name, actual.CollationName, want.CollationName))
	}
	if actual.CharacterMaximumLength != want.CharacterMaximumLength {
		add(fmt.Sprintf("column %s.%s character length = %d, want %d", table, expected.Name, actual.CharacterMaximumLength, want.CharacterMaximumLength))
	}
	return issues
}

func expectedCatalogColumn(column Column) CatalogColumn {
	typ := strings.ToUpper(strings.TrimSpace(column.Type))
	out := CatalogColumn{
		Type:     normalizeManifestColumnType(typ),
		Nullable: column.Nullable,
		Default:  strings.TrimSpace(column.Default),
	}
	if strings.Contains(typ, `COLLATE "C"`) {
		out.CollationName = "C"
	}
	if strings.Contains(typ, "GENERATED ALWAYS AS") {
		out.Generated = true
	}
	if typ == "BIGSERIAL" {
		out.Serial = true
	}
	if strings.HasPrefix(typ, "CHAR(") {
		out.CharacterMaximumLength = parseCharLength(typ)
	}
	return out
}

func normalizeManifestColumnType(typ string) string {
	switch {
	case strings.HasPrefix(typ, "TEXT"):
		return "text"
	case typ == "TIMESTAMPTZ":
		return "timestamp with time zone"
	case typ == "JSONB":
		return "jsonb"
	case strings.HasPrefix(typ, "INTEGER"):
		return "integer"
	case typ == "BIGSERIAL", typ == "BIGINT":
		return "bigint"
	case typ == "BOOLEAN":
		return "boolean"
	case strings.HasPrefix(typ, "CHAR("):
		return "character"
	default:
		return strings.ToLower(typ)
	}
}

func normalizeCatalogType(typ string) string {
	return strings.ToLower(strings.TrimSpace(typ))
}

func normalizeDefault(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isSerialDefault(value string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "nextval(")
}

func parseCharLength(typ string) int {
	start := strings.IndexByte(typ, '(')
	end := strings.IndexByte(typ, ')')
	if start < 0 || end <= start+1 {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(typ[start+1 : end]))
	if err != nil {
		return 0
	}
	return n
}
