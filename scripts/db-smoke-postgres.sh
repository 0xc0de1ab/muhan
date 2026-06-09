#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root_dir"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 127
fi
if ! command -v go >/dev/null 2>&1; then
  echo "go is required" >&2
  exit 127
fi

if docker compose version >/dev/null 2>&1; then
  compose=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  compose=(docker-compose)
else
  echo "docker compose is required" >&2
  exit 127
fi

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi

: "${MUHAN_POSTGRES_DB:=muhan}"
: "${MUHAN_POSTGRES_USER:=muhan}"
: "${MUHAN_POSTGRES_PASSWORD:=muhan_dev_password}"
: "${MUHAN_POSTGRES_PORT:=55432}"
: "${MUHAN_POSTGRES_SCHEMA:=muhan_import}"
: "${MUHAN_IMPORT_RUN_ID:=docker-smoke}"
: "${MUHAN_SOURCE_ROOT_LABEL:=docker-postgres}"

"${compose[@]}" up -d postgres

for _ in {1..60}; do
  if "${compose[@]}" exec -T postgres pg_isready -U "$MUHAN_POSTGRES_USER" -d "$MUHAN_POSTGRES_DB" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done

if [[ "${ready:-}" != 1 ]]; then
  echo "postgres did not become ready" >&2
  "${compose[@]}" ps postgres >&2
  exit 1
fi

dsn_file="$(mktemp "${TMPDIR:-/tmp}/muhan-dbimport.XXXXXX.dsn")"
trap 'rm -f "$dsn_file"' EXIT
chmod 600 "$dsn_file"
printf 'postgres://%s:%s@127.0.0.1:%s/%s?sslmode=disable\n' \
  "$MUHAN_POSTGRES_USER" \
  "$MUHAN_POSTGRES_PASSWORD" \
  "$MUHAN_POSTGRES_PORT" \
  "$MUHAN_POSTGRES_DB" >"$dsn_file"

go run ./cmd/muhan-dbimport \
  -root . \
  -run-id "$MUHAN_IMPORT_RUN_ID" \
  -source-root-label "$MUHAN_SOURCE_ROOT_LABEL" \
  -schema-mode ensure \
  -target-schema "$MUHAN_POSTGRES_SCHEMA" \
  -execute \
  -replace-run \
  -dsn-file "$dsn_file"

smoke_result="$("${compose[@]}" exec -T postgres psql \
  -U "$MUHAN_POSTGRES_USER" \
  -d "$MUHAN_POSTGRES_DB" \
  -v ON_ERROR_STOP=1 \
  -v schema="$MUHAN_POSTGRES_SCHEMA" \
  -v run_id="$MUHAN_IMPORT_RUN_ID" \
  -v source_root="$MUHAN_SOURCE_ROOT_LABEL" \
  -At <<'SQL'
WITH run AS (
  SELECT *
  FROM :"schema".import_runs
  WHERE run_id = :'run_id'
), verification_tables AS (
  SELECT
    count(*) AS tables,
    count(*) FILTER (WHERE (entry->>'actual')::int = (entry->>'planned')::int) AS matched_tables,
    coalesce(sum((entry->>'actual')::int), 0) AS actual_rows
  FROM run, jsonb_array_elements(run.manifest #> '{verification,tables}') AS entry
)
SELECT CASE WHEN
  EXISTS (SELECT 1 FROM run)
  AND (SELECT schema_version FROM run) = 'muhan-db-schema/v1'
  AND (SELECT source_root FROM run) = :'source_root'
  AND (SELECT manifest->>'schemaVersion' FROM run) = 'muhan-dbimport/v1'
  AND (SELECT jsonb_typeof(manifest->'verification') FROM run) = 'object'
  AND (SELECT tables FROM verification_tables) = 18
  AND (SELECT matched_tables FROM verification_tables) = 18
  AND (SELECT actual_rows FROM verification_tables) = (SELECT (manifest #>> '{verification,rows}')::int FROM run)
THEN 'ok' ELSE 'fail' END;
SQL
)"

if [[ "$smoke_result" != "ok" ]]; then
  echo "post-import metadata smoke failed: $smoke_result" >&2
  exit 1
fi
echo "post-import metadata smoke: ok"
