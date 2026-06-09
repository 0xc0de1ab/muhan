package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"muhan/internal/migrate/dbschema"
)

func main() {
	dialect := flag.String("dialect", dbschema.DialectPostgres, "database dialect; currently postgresql")
	outdir := flag.String("outdir", "", "output directory for schema files")
	targetSchema := flag.String("target-schema", "", "optional PostgreSQL schema qualifier for SQL output")
	jsonOut := flag.Bool("json", false, "print schema manifest JSON to stdout")
	sqlOut := flag.Bool("sql", false, "print PostgreSQL DDL to stdout")
	flag.Parse()

	manifest, err := dbschema.Build(dbschema.Options{Dialect: *dialect})
	if err != nil {
		fmt.Fprintf(os.Stderr, "dbschema: %v\n", err)
		os.Exit(2)
	}

	if *outdir != "" {
		manifest, err = dbschema.Write(*outdir, dbschema.Options{Dialect: *dialect, GeneratedAt: manifest.GeneratedAt})
		if err != nil {
			fmt.Fprintf(os.Stderr, "dbschema: %v\n", err)
			os.Exit(2)
		}
	}

	switch {
	case *sqlOut:
		ddl, err := dbschema.PostgresDDLForSchema(manifest, *targetSchema)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dbschema: %v\n", err)
			os.Exit(2)
		}
		fmt.Print(ddl)
	case *jsonOut:
		if err := writeJSON(os.Stdout, manifest); err != nil {
			fmt.Fprintf(os.Stderr, "dbschema: %v\n", err)
			os.Exit(2)
		}
	default:
		writeText(os.Stdout, manifest)
	}
}

func writeJSON(w io.Writer, manifest dbschema.Manifest) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(manifest)
}

func writeText(w io.Writer, manifest dbschema.Manifest) {
	fmt.Fprintf(w, "dialect: %s\n", manifest.Dialect)
	fmt.Fprintf(w, "schemaVersion: %s\n", manifest.SchemaVersion)
	fmt.Fprintf(w, "tables: %d\n", len(manifest.Tables))
	for _, file := range manifest.Files {
		fmt.Fprintf(w, "artifact: %s bytes=%d sha256=%s\n", file.Path, file.Bytes, file.SHA256)
	}
}
