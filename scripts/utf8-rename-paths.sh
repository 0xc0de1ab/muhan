#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

ROOT=${MUHAN_ROOT:-$(pwd)}
INCLUDE="all"
WRITE=0
ALLOW_ORIGINAL=0
MANIFEST=""

usage() {
  cat <<'USAGE'
Usage: scripts/utf8-rename-paths.sh [--root DIR] [--include family-bank|board|all] [--write] [--allow-original] [--manifest FILE]

Renames selected CP949 path components to UTF-8. The default mode is a
read-only dry-run. Use this on a disposable copy first.

Targets:
  family-bank: player/family/bank/* filenames
  board:       board/*/* non-index auxiliary filenames

Safety:
  --write is refused on /workspace/muhan unless --allow-original is provided.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --root)
      [[ $# -ge 2 ]] || { echo "--root requires a directory" >&2; exit 2; }
      ROOT=$2
      shift 2
      ;;
    --root=*)
      ROOT=${1#*=}
      shift
      ;;
    --include)
      [[ $# -ge 2 ]] || { echo "--include requires a value" >&2; exit 2; }
      INCLUDE=$2
      shift 2
      ;;
    --include=*)
      INCLUDE=${1#*=}
      shift
      ;;
    --write)
      WRITE=1
      shift
      ;;
    --allow-original)
      ALLOW_ORIGINAL=1
      shift
      ;;
    --manifest)
      [[ $# -ge 2 ]] || { echo "--manifest requires a file" >&2; exit 2; }
      MANIFEST=$2
      shift 2
      ;;
    --manifest=*)
      MANIFEST=${1#*=}
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "$INCLUDE" in
  family-bank|family|board|all) ;;
  *) echo "--include must be family-bank, board, or all" >&2; exit 2 ;;
esac

if ! command -v iconv >/dev/null 2>&1; then
  echo "iconv is required" >&2
  exit 2
fi

ROOT=$(cd "$ROOT" && pwd)
if ((WRITE)) && ((ALLOW_ORIGINAL == 0)) && [[ "$ROOT" == "/workspace/muhan" ]]; then
  echo "refusing to write to /workspace/muhan without --allow-original; use a disposable copy first" >&2
  exit 3
fi

if [[ -n "$MANIFEST" ]]; then
  mkdir -p "$(dirname "$MANIFEST")"
  : >"$MANIFEST"
fi

total=0
already_utf8=0
cp949_names=0
would_rename=0
renamed=0
collisions=0
failed=0

log_manifest() {
  [[ -n "$MANIFEST" ]] || return
  printf '%s\n' "$*" >>"$MANIFEST"
}

is_utf8_bytes() {
  printf '%s' "$1" | iconv -f utf-8 -t utf-8 >/dev/null 2>&1
}

decode_cp949_bytes() {
  printf '%s' "$1" | iconv -f cp949 -t utf-8 2>/dev/null
}

raw_hex() {
  printf '%s' "$1" | od -An -v -tx1 | tr -d ' \n'
}

relative_path() {
  printf '%s' "${1#"$ROOT"/}"
}

display_path() {
  local path="$1"
  local rel decoded
  rel=$(relative_path "$path")
  if is_utf8_bytes "$rel"; then
    printf '%s' "$rel"
    return
  fi
  if decoded=$(decode_cp949_bytes "$rel"); then
    printf '%s [raw:%s]' "$decoded" "$(raw_hex "$rel")"
    return
  fi
  printf '<non-utf8:%s>' "$(raw_hex "$rel")"
}

rename_path() {
  local path="$1"
  local base dir decoded target source_display target_display

  ((total += 1))
  base=$(basename "$path")
  if is_utf8_bytes "$base"; then
    ((already_utf8 += 1))
    log_manifest "already_utf8 $(relative_path "$path")"
    return
  fi
  if ! decoded=$(decode_cp949_bytes "$base"); then
    ((failed += 1))
    log_manifest "not_cp949_name $(display_path "$path")"
    return
  fi
  if [[ -z "$decoded" || "$decoded" == */* ]]; then
    ((failed += 1))
    log_manifest "invalid_decoded_name $(display_path "$path") -> $decoded"
    return
  fi

  ((cp949_names += 1))
  dir=$(dirname "$path")
  target="$dir/$decoded"
  source_display=$(display_path "$path")
  target_display=$(relative_path "$target")

  if [[ -e "$target" ]]; then
    ((collisions += 1))
    log_manifest "collision $source_display -> $target_display"
    return
  fi
  if ((WRITE == 0)); then
    ((would_rename += 1))
    log_manifest "would_rename $source_display -> $target_display"
    return
  fi

  if mv -- "$path" "$target"; then
    ((renamed += 1))
    log_manifest "renamed $source_display -> $target_display"
  else
    ((failed += 1))
    log_manifest "rename_failed $source_display -> $target_display"
  fi
}

scan_family_bank() {
  local paths=()
  local path
  [[ -d "$ROOT/player/family/bank" ]] || return
  while IFS= read -r -d '' path; do
    paths+=("$path")
  done < <(find "$ROOT/player/family/bank" -mindepth 1 -maxdepth 1 -type f -print0)
  for path in "${paths[@]}"; do
    rename_path "$path"
  done
}

scan_board() {
  local paths=()
  local path
  [[ -d "$ROOT/board" ]] || return
  while IFS= read -r -d '' path; do
    case "$(basename "$path")" in
      board_index|board.*) continue ;;
    esac
    paths+=("$path")
  done < <(find "$ROOT/board" -mindepth 2 -maxdepth 2 -type f -print0)
  for path in "${paths[@]}"; do
    rename_path "$path"
  done
}

case "$INCLUDE" in
  family|family-bank) scan_family_bank ;;
  board) scan_board ;;
  all)
    scan_family_bank
    scan_board
    ;;
esac

if ((WRITE)); then
  mode_label="Write"
else
  mode_label="Dry-Run"
fi

cat <<REPORT
# UTF-8 Path Rename $mode_label Report
root: $ROOT
include: $INCLUDE
write: $WRITE
paths_scanned: $total
already_utf8: $already_utf8
cp949_names: $cp949_names
would_rename: $would_rename
renamed: $renamed
collisions: $collisions
failed_or_unknown: $failed
manifest: ${MANIFEST:-not written}
REPORT
