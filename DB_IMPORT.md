# DB Import

This project can run the PostgreSQL import target with Docker Compose.

## Start PostgreSQL

```sh
cp .env.example .env
docker compose up -d postgres
```

Defaults:

- image: `postgres:16-alpine`
- host: `127.0.0.1`
- port: `55432`
- database: `muhan`
- user: `muhan`
- target schema: `muhan_import`

The Compose service stores data in the `postgres-data` volume. Imported data can include legacy board bodies and raw migration metadata, so treat the local database as sensitive.

## Run A Live Import

The smoke script starts PostgreSQL if needed, waits for readiness, writes a temporary `0600` DSN file, runs the real importer with schema creation enabled, and then checks the committed import metadata:

```sh
./scripts/db-smoke-postgres.sh
```

Equivalent manual command:

```sh
set -a
. ./.env
set +a

printf 'postgres://%s:%s@127.0.0.1:%s/%s?sslmode=disable\n' \
  "$MUHAN_POSTGRES_USER" \
  "$MUHAN_POSTGRES_PASSWORD" \
  "$MUHAN_POSTGRES_PORT" \
  "$MUHAN_POSTGRES_DB" > /tmp/muhan-dbimport.dsn
chmod 600 /tmp/muhan-dbimport.dsn

go run ./cmd/muhan-dbimport \
  -root . \
  -run-id docker-smoke \
  -source-root-label docker-postgres \
  -schema-mode ensure \
  -target-schema "$MUHAN_POSTGRES_SCHEMA" \
  -execute \
  -replace-run \
  -dsn-file /tmp/muhan-dbimport.dsn
```

Schema targeting:

- `-target-schema`: PostgreSQL namespace for all import tables. The default is `muhan_import`.
- Do not use DSN `search_path` or `options` to redirect imports; the importer rejects those during `-execute`.

Schema modes:

- `verify`: validate the existing PostgreSQL schema against the generated `muhan-dbschema` manifest.
- `ensure`: run static `CREATE ... IF NOT EXISTS` DDL, then validate. It creates missing objects but does not repair incompatible drift.
- `skip`: bypass schema preflight; use only for explicit debugging.

Preflight currently checks required tables and column shape inside the target schema, including type, nullability, defaults, generated columns, serial columns, fixed character length, and explicit `C` collation. If an old local volume fails preflight, use `docker compose down -v` for disposable data or apply a real migration before importing preserved data.

## Sidecar JSON Schema Migration

Player, bank, room floor, board, and family-news JSON sidecars can be migrated from older supported schema versions to the current version with:

```sh
go run ./cmd/muhan-sidecarmigrate -root .
```

The default mode is a dry run. It copies only the sidecar JSON corpus to a temporary root, runs the migration there, and reports which source files would be rewritten. Use `-execute` to rewrite the real sidecar files in place. Unsupported future schema versions are reported as errors and make the command exit non-zero.

Use `-details` for a per-file text list, or `-json` for a machine-readable report.

## Reset

Stop the container but keep imported data:

```sh
docker compose down
```

Delete the local PostgreSQL volume:

```sh
docker compose down -v
```
