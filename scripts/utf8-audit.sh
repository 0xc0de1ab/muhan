#!/usr/bin/env bash
set -euo pipefail

ROOT=${MUHAN_ROOT:-$(pwd)}
SAMPLE_LIMIT=20
OUTPUT=""

usage() {
  cat <<'USAGE'
Usage: scripts/utf8-audit.sh [--root DIR] [--sample-limit N] [--output FILE]

Read-only UTF-8/legacy byte audit for Muhan data files. The script scans
high-risk text and JSON persistence locations, reports content and filename
encoding status, and does not rewrite source data.

Targets:
  player/family
  board
  room/json
  player/json
  player/bank/json

Environment:
  MUHAN_ROOT  default root when --root is not provided
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
    --sample-limit)
      [[ $# -ge 2 ]] || { echo "--sample-limit requires a number" >&2; exit 2; }
      SAMPLE_LIMIT=$2
      shift 2
      ;;
    --sample-limit=*)
      SAMPLE_LIMIT=${1#*=}
      shift
      ;;
    --output)
      [[ $# -ge 2 ]] || { echo "--output requires a file" >&2; exit 2; }
      OUTPUT=$2
      shift 2
      ;;
    --output=*)
      OUTPUT=${1#*=}
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

if ! [[ "$SAMPLE_LIMIT" =~ ^[0-9]+$ ]]; then
  echo "--sample-limit must be a non-negative integer" >&2
  exit 2
fi

if ! command -v iconv >/dev/null 2>&1; then
  echo "iconv is required for UTF-8/legacy Korean audit" >&2
  exit 2
fi

ROOT=$(cd "$ROOT" && pwd)
TARGETS=(player/family board room/json player/json player/bank/json)

total=0
path_utf8=0
path_legacy_cp949=0
path_legacy_johab=0
path_unknown=0
path_replacement=0
content_utf8=0
content_legacy_cp949=0
content_legacy_johab=0
content_binary_or_nul=0
content_unknown=0
content_replacement=0
json_total=0
json_utf8=0
json_invalid_utf8=0

declare -A seen=()
declare -a missing_targets=()
declare -a samples_legacy_path=()
declare -a samples_legacy_content=()
declare -a samples_unknown_content=()
declare -a samples_replacement=()
declare -a samples_json_invalid=()

append_sample() {
  local name=$1
  local value=$2
  local -n array_ref="$name"
  if (( ${#array_ref[@]} < SAMPLE_LIMIT )); then
    array_ref+=("$value")
  fi
}

sanitize_line() {
  local value=$1
  value=${value//$'\n'/\\n}
  value=${value//$'\r'/\\r}
  printf '%s' "$value"
}

raw_hex() {
  printf '%s' "$1" | od -An -v -tx1 | tr -d ' \n'
}

safe_path() {
  local rel=$1
  local decoded hex
  if printf '%s' "$rel" | iconv -f UTF-8 -t UTF-8 >/dev/null 2>&1; then
    sanitize_line "$rel"
    return
  fi
  hex=$(raw_hex "$rel")
  if decoded=$(printf '%s' "$rel" | iconv -f CP949 -t UTF-8 2>/dev/null); then
    printf '%s [raw:%s]' "$(sanitize_line "$decoded")" "$hex"
    return
  fi
  if decoded=$(printf '%s' "$rel" | iconv -f JOHAB -t UTF-8 2>/dev/null); then
    printf '%s [raw:%s]' "$(sanitize_line "$decoded")" "$hex"
    return
  fi
  printf '[raw:%s]' "$hex"
}

path_encoding() {
  local rel=$1
  if printf '%s' "$rel" | iconv -f UTF-8 -t UTF-8 >/dev/null 2>&1; then
    printf 'utf8'
    return
  fi
  if printf '%s' "$rel" | iconv -f CP949 -t UTF-8 >/dev/null 2>&1; then
    printf 'legacy-cp949'
    return
  fi
  if printf '%s' "$rel" | iconv -f JOHAB -t UTF-8 >/dev/null 2>&1; then
    printf 'legacy-johab'
    return
  fi
  printf 'unknown'
}

contains_replacement_bytes() {
  LC_ALL=C grep -q "$(printf '\357\277\275')" "$1" 2>/dev/null
}

path_contains_replacement() {
  printf '%s' "$1" | LC_ALL=C grep -q "$(printf '\357\277\275')" 2>/dev/null
}

has_nul() {
  LC_ALL=C od -An -v -tx1 "$1" 2>/dev/null | grep -Eq '(^|[[:space:]])00([[:space:]]|$)'
}

content_encoding() {
  local file=$1
  if [[ ! -s "$file" ]]; then
    printf 'utf8'
    return
  fi
  if has_nul "$file"; then
    printf 'binary-or-nul'
    return
  fi
  if iconv -f UTF-8 -t UTF-8 "$file" >/dev/null 2>&1; then
    printf 'utf8'
    return
  fi
  if iconv -f CP949 -t UTF-8 "$file" >/dev/null 2>&1; then
    printf 'legacy-cp949'
    return
  fi
  if iconv -f JOHAB -t UTF-8 "$file" >/dev/null 2>&1; then
    printf 'legacy-johab'
    return
  fi
  printf 'unknown'
}

is_json_sidecar() {
  case "$1" in
    room/json/*|player/json/*|player/bank/json/*)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

scan_file() {
  local file=$1
  local rel=${file#"$ROOT"/}
  local display path_status content_status

  [[ ${seen[$file]+x} ]] && return
  seen[$file]=1

  display=$(safe_path "$rel")
  path_status=$(path_encoding "$rel")
  content_status=$(content_encoding "$file")

  (( total += 1 ))

  case "$path_status" in
    utf8) (( path_utf8 += 1 )) ;;
    legacy-cp949) (( path_legacy_cp949 += 1 )); append_sample samples_legacy_path "$display :: path=legacy-cp949" ;;
    legacy-johab) (( path_legacy_johab += 1 )); append_sample samples_legacy_path "$display :: path=legacy-johab" ;;
    *) (( path_unknown += 1 )); append_sample samples_legacy_path "$display :: path=unknown" ;;
  esac

  if path_contains_replacement "$rel"; then
    (( path_replacement += 1 ))
    append_sample samples_replacement "$display :: path contains U+FFFD"
  fi

  case "$content_status" in
    utf8) (( content_utf8 += 1 )) ;;
    legacy-cp949) (( content_legacy_cp949 += 1 )); append_sample samples_legacy_content "$display :: content=legacy-cp949" ;;
    legacy-johab) (( content_legacy_johab += 1 )); append_sample samples_legacy_content "$display :: content=legacy-johab" ;;
    binary-or-nul) (( content_binary_or_nul += 1 )); append_sample samples_unknown_content "$display :: content=binary-or-nul" ;;
    *) (( content_unknown += 1 )); append_sample samples_unknown_content "$display :: content=unknown" ;;
  esac

  if contains_replacement_bytes "$file"; then
    (( content_replacement += 1 ))
    append_sample samples_replacement "$display :: content contains U+FFFD"
  fi

  if is_json_sidecar "$rel"; then
    (( json_total += 1 ))
    if [[ "$content_status" == "utf8" ]]; then
      (( json_utf8 += 1 ))
    else
      (( json_invalid_utf8 += 1 ))
      append_sample samples_json_invalid "$display :: content=$content_status"
    fi
  fi
}

for target in "${TARGETS[@]}"; do
  if [[ ! -e "$ROOT/$target" ]]; then
    missing_targets+=("$target")
  fi
done

while IFS= read -r -d '' file; do
  scan_file "$file"
done < <(
  for target in "${TARGETS[@]}"; do
    [[ -e "$ROOT/$target" ]] || continue
    find "$ROOT/$target" -type f -print0
  done
)

emit_samples() {
  local title=$1
  local name=$2
  local -n array_ref="$name"
  echo
  echo "## $title"
  if (( ${#array_ref[@]} == 0 )); then
    echo "- none"
    return
  fi
  local item
  for item in "${array_ref[@]}"; do
    printf -- '- %s\n' "$item"
  done
}

emit_report() {
  echo "# UTF-8 Audit Report"
  echo
  printf 'root: %s\n' "$ROOT"
  printf 'generated_utc: %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  printf 'modified_source_data: no\n'
  echo
  echo "## Targets"
  local target
  for target in "${TARGETS[@]}"; do
    if [[ -e "$ROOT/$target" ]]; then
      printf -- '- %s\n' "$target"
    else
      printf -- '- %s (missing)\n' "$target"
    fi
  done

  echo
  echo "## Summary"
  printf 'files_total: %d\n' "$total"
  printf 'path_utf8: %d\n' "$path_utf8"
  printf 'path_legacy_cp949: %d\n' "$path_legacy_cp949"
  printf 'path_legacy_johab: %d\n' "$path_legacy_johab"
  printf 'path_unknown: %d\n' "$path_unknown"
  printf 'path_contains_replacement_char: %d\n' "$path_replacement"
  printf 'content_utf8_text_or_empty: %d\n' "$content_utf8"
  printf 'content_legacy_cp949: %d\n' "$content_legacy_cp949"
  printf 'content_legacy_johab: %d\n' "$content_legacy_johab"
  printf 'content_binary_or_nul: %d\n' "$content_binary_or_nul"
  printf 'content_unknown: %d\n' "$content_unknown"
  printf 'content_contains_replacement_char: %d\n' "$content_replacement"
  printf 'json_sidecar_total: %d\n' "$json_total"
  printf 'json_sidecar_utf8: %d\n' "$json_utf8"
  printf 'json_sidecar_invalid_utf8: %d\n' "$json_invalid_utf8"

  emit_samples "Legacy or Non-UTF-8 Filenames" samples_legacy_path
  emit_samples "Legacy Text Content" samples_legacy_content
  emit_samples "Binary or Unknown Content" samples_unknown_content
  emit_samples "Replacement Character Findings" samples_replacement
  emit_samples "Invalid JSON Sidecar UTF-8" samples_json_invalid

  echo
  echo "## Notes"
  echo "- This report is read-only. It never rewrites data files."
  echo "- binary-or-nul usually means legacy binary indexes or fixed C records, not a direct text-conversion target."
  echo "- legacy-cp949 covers EUC-KR/CP949-compatible Korean text as used by the current legacy decoder."
  echo "- legacy-johab is reported separately because original requirements mention composed Korean legacy bytes, but it should be converted only after fixture-level verification."
}

if [[ -n "$OUTPUT" ]]; then
  emit_report | tee "$OUTPUT"
else
  emit_report
fi
