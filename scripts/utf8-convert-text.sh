#!/usr/bin/env bash
set -euo pipefail

ROOT=${MUHAN_ROOT:-$(pwd)}
INCLUDE="family"
WRITE=0
ALLOW_ORIGINAL=0
MANIFEST=""

usage() {
  cat <<'USAGE'
Usage: scripts/utf8-convert-text.sh [--root DIR] [--include family|board|all] [--write] [--allow-original] [--manifest FILE]

Converts selected legacy CP949 text files to UTF-8. The default mode is a
read-only dry-run. Use this on a disposable copy first.

Targets:
  family: player/family/family_list, family_member_*, family_news_*
  board:  board/*/board.* post bodies only; board_index is intentionally skipped

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
  family|board|all) ;;
  *) echo "--include must be family, board, or all" >&2; exit 2 ;;
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
convertible=0
converted=0
already_utf8=0
skipped_binary=0
failed=0

log_manifest() {
  [[ -n "$MANIFEST" ]] || return
  printf '%s\n' "$*" >>"$MANIFEST"
}

has_nul() {
  LC_ALL=C od -An -v -tx1 "$1" 2>/dev/null | grep -Eq '(^|[[:space:]])00([[:space:]]|$)'
}

is_utf8() {
  iconv -f utf-8 -t utf-8 "$1" >/dev/null 2>&1
}

is_cp949() {
  iconv -f cp949 -t utf-8 "$1" >/dev/null 2>&1
}

relative_path() {
  printf '%s' "${1#"$ROOT"/}"
}

convert_file() {
  local file="$1"
  local rel tmp mode

  ((total += 1))
  rel=$(relative_path "$file")
  if [[ ! -s "$file" ]] || is_utf8 "$file"; then
    ((already_utf8 += 1))
    log_manifest "already_utf8 $rel"
    return
  fi
  if has_nul "$file"; then
    ((skipped_binary += 1))
    log_manifest "skip_binary $rel"
    return
  fi
  if ! is_cp949 "$file"; then
    ((failed += 1))
    log_manifest "not_cp949 $rel"
    return
  fi

  ((convertible += 1))
  if ((WRITE == 0)); then
    log_manifest "would_convert $rel"
    return
  fi

  tmp=$(mktemp "$(dirname "$file")/.utf8-convert.$(basename "$file").XXXXXX")
  mode=$(stat -c %a "$file")
  if iconv -f cp949 -t utf-8 "$file" >"$tmp"; then
    chmod "$mode" "$tmp"
    touch -r "$file" "$tmp"
    mv "$tmp" "$file"
    ((converted += 1))
    log_manifest "converted $rel"
  else
    rm -f "$tmp"
    ((failed += 1))
    log_manifest "convert_failed $rel"
  fi
}

scan_family() {
  [[ -d "$ROOT/player/family" ]] || return
  while IFS= read -r -d '' file; do
    convert_file "$file"
  done < <(
    find "$ROOT/player/family" -maxdepth 1 -type f \
      \( -name 'family_list' -o -name 'family_member_*' -o -name 'family_news_*' \) -print0
  )
}

scan_board() {
  [[ -d "$ROOT/board" ]] || return
  while IFS= read -r -d '' file; do
    convert_file "$file"
  done < <(find "$ROOT/board" -mindepth 2 -maxdepth 2 -type f -name 'board.*' -print0)
}

case "$INCLUDE" in
  family) scan_family ;;
  board) scan_board ;;
  all)
    scan_family
    scan_board
    ;;
esac

if ((WRITE)); then
  mode_label="Write"
else
  mode_label="Dry-Run"
fi

cat <<REPORT
# UTF-8 Text Conversion $mode_label Report
root: $ROOT
include: $INCLUDE
write: $WRITE
files_scanned: $total
already_utf8_or_empty: $already_utf8
convertible_cp949: $convertible
converted: $converted
skipped_binary_or_nul: $skipped_binary
failed_or_unknown: $failed
manifest: ${MANIFEST:-not written}
REPORT
