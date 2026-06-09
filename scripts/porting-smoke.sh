#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
data_root="$repo_root"
host="${MUHAN_SMOKE_HOST:-127.0.0.1}"
port="${MUHAN_SMOKE_PORT:-4040}"
login_name="${MUHAN_SMOKE_LOGIN:-인제로}"
login_password="${MUHAN_SMOKE_PASSWORD:-1234}"
second_login_name="${MUHAN_SMOKE_SECOND_LOGIN:-레터}"
second_login_password="${MUHAN_SMOKE_SECOND_PASSWORD:-1234}"
class_family_login="${MUHAN_SMOKE_CLASS_FAMILY_LOGIN:-소녀무사}"
class_family_password="${MUHAN_SMOKE_CLASS_FAMILY_PASSWORD:-1234}"
class_family_room="${MUHAN_SMOKE_CLASS_FAMILY_ROOM:-room:01034}"
class_family_source_class="${MUHAN_SMOKE_CLASS_FAMILY_SOURCE_CLASS:-6}"
class_family_target_class="${MUHAN_SMOKE_CLASS_FAMILY_TARGET_CLASS:-5}"
class_family_experience="${MUHAN_SMOKE_CLASS_FAMILY_EXPERIENCE:-6790307}"
permdeath_login="${MUHAN_SMOKE_PERMDEATH_LOGIN:-인제로}"
permdeath_password="${MUHAN_SMOKE_PERMDEATH_PASSWORD:-1234}"
permdeath_room="${MUHAN_SMOKE_PERMDEATH_ROOM:-room:03566}"
permdeath_monster_number="${MUHAN_SMOKE_PERMDEATH_MONSTER_NUMBER:-98}"
permdeath_monster_name="${MUHAN_SMOKE_PERMDEATH_MONSTER_NAME:-타타르의 머리}"
permdeath_attack_target="${MUHAN_SMOKE_PERMDEATH_ATTACK_TARGET:-타타르}"
permdeath_final_monster_name="${MUHAN_SMOKE_PERMDEATH_FINAL_MONSTER_NAME:-타타르의 몸}"
suicide_command="${MUHAN_SMOKE_SUICIDE_COMMAND:-목매달기}"
server_session="${MUHAN_SMOKE_SERVER_SESSION:-muhan-server}"
client_session="${MUHAN_SMOKE_CLIENT_SESSION:-muhan-cli}"
second_client_session="${MUHAN_SMOKE_SECOND_CLIENT_SESSION:-muhan-cli-2}"
log_dir="${MUHAN_SMOKE_LOG_DIR:-${TMPDIR:-/tmp}/muhan-porting-smoke.$(date +%Y%m%d-%H%M%S)}"
run_preflight=1
run_sidecar=1
run_runtime=0
run_scenario=0
run_scenario_family=0
run_scenario_class_family=0
run_scenario_permdeath=0
run_scenario_trap=0
run_scenario_talk_give=0
run_scenario_talk_effects=0
run_scenario_objects=0
run_live_gap_report=0
skip_go_test=0
copy_root=0
clean_sessions=1
ansi=false

usage() {
  cat <<'EOF'
Usage: scripts/porting-smoke.sh [options]

Runs repeatable Go porting smoke checks and writes artifacts under a log dir.

Options:
  --root DIR          Data/source root to load. Default: repository root.
  --port PORT         Runtime smoke TCP port. Default: 4040.
  --runtime           Also run tmux + muhan-client login/multi-client smoke.
  --scenario          Run destructive disposable-root scenario smoke.
  --scenario-family   Run disposable-root family kick/restart transcript smoke.
  --scenario-class-family Run disposable-root family class-change/restart smoke.
  --scenario-permdeath Run disposable-root permanent monster death smoke.
  --scenario-trap     Run disposable-root pit-trap/death transcript smoke.
  --scenario-talk-give Run disposable-root MTALKS + real GIVE transcript smoke.
  --scenario-talk-effects Run disposable-root real ACTION/CAST/ATTACK transcript smoke.
  --scenario-objects  Run disposable-root ORENCH + nested object transcript smoke.
  --live-gap-report   Write non-mutating live fixture/evidence notes for open gaps.
  --all               Run preflight, sidecar dry-run, and runtime smoke.
  --copy-root         Run runtime smoke against a disposable copy of --root.
  --ansi              Start runtime server with -ansi=true.
  --skip-go-test      Skip go test ./... in preflight.
  --no-preflight      Skip build/validate/cmdlist checks.
  --no-sidecar        Skip sidecar migration dry-run.
  --leave-sessions    Leave tmux sessions running after runtime smoke.
  --log-dir DIR       Artifact directory. Default: /tmp/muhan-porting-smoke.*
  -h, --help          Show this help.

Environment:
  MUHAN_SMOKE_LOGIN, MUHAN_SMOKE_PASSWORD, MUHAN_SMOKE_SECOND_LOGIN,
  MUHAN_SMOKE_SECOND_PASSWORD, MUHAN_SMOKE_CLASS_FAMILY_LOGIN,
  MUHAN_SMOKE_CLASS_FAMILY_PASSWORD, MUHAN_SMOKE_CLASS_FAMILY_ROOM,
  MUHAN_SMOKE_CLASS_FAMILY_SOURCE_CLASS, MUHAN_SMOKE_CLASS_FAMILY_TARGET_CLASS,
  MUHAN_SMOKE_CLASS_FAMILY_EXPERIENCE, MUHAN_SMOKE_PERMDEATH_LOGIN,
  MUHAN_SMOKE_PERMDEATH_PASSWORD, MUHAN_SMOKE_PERMDEATH_ROOM,
  MUHAN_SMOKE_PERMDEATH_MONSTER_NUMBER, MUHAN_SMOKE_PERMDEATH_MONSTER_NAME,
  MUHAN_SMOKE_PERMDEATH_ATTACK_TARGET, MUHAN_SMOKE_PERMDEATH_FINAL_MONSTER_NAME,
  MUHAN_SMOKE_SUICIDE_COMMAND, MUHAN_SMOKE_PORT, MUHAN_SMOKE_SERVER_SESSION,
  MUHAN_SMOKE_CLIENT_SESSION, MUHAN_SMOKE_SECOND_CLIENT_SESSION,
  MUHAN_SMOKE_LOG_DIR.
EOF
}

need_value() {
  if (($# < 2)); then
    echo "$1 requires a value" >&2
    usage >&2
    exit 2
  fi
}

while (($#)); do
  case "$1" in
    --root)
      need_value "$@"
      data_root="$2"
      shift 2
      ;;
    --port)
      need_value "$@"
      port="$2"
      shift 2
      ;;
    --runtime)
      run_runtime=1
      shift
      ;;
    --scenario)
      run_scenario=1
      shift
      ;;
    --scenario-family)
      run_scenario_family=1
      shift
      ;;
    --scenario-class-family)
      run_scenario_class_family=1
      shift
      ;;
    --scenario-permdeath)
      run_scenario_permdeath=1
      shift
      ;;
    --scenario-trap)
      run_scenario_trap=1
      shift
      ;;
    --scenario-talk-give)
      run_scenario_talk_give=1
      shift
      ;;
    --scenario-talk-effects)
      run_scenario_talk_effects=1
      shift
      ;;
    --scenario-objects)
      run_scenario_objects=1
      shift
      ;;
    --live-gap-report)
      run_live_gap_report=1
      shift
      ;;
    --all)
      run_preflight=1
      run_sidecar=1
      run_runtime=1
      shift
      ;;
    --copy-root)
      copy_root=1
      shift
      ;;
    --ansi)
      ansi=true
      shift
      ;;
    --skip-go-test)
      skip_go_test=1
      shift
      ;;
    --no-preflight)
      run_preflight=0
      shift
      ;;
    --no-sidecar)
      run_sidecar=0
      shift
      ;;
    --leave-sessions)
      clean_sessions=0
      shift
      ;;
    --log-dir)
      need_value "$@"
      log_dir="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

mkdir -p "$log_dir"
summary_log="$log_dir/summary.log"
gap_report="$log_dir/gap-report.txt"
live_gap_report="$log_dir/live-gap-report.txt"
family_scenario_report="$log_dir/scenario-family-report.txt"
class_family_scenario_report="$log_dir/scenario-class-family-report.txt"
permdeath_scenario_report="$log_dir/scenario-permdeath-report.txt"
trap_scenario_report="$log_dir/scenario-trap-report.txt"
talk_give_scenario_report="$log_dir/scenario-talk-give-report.txt"
talk_effects_scenario_report="$log_dir/scenario-talk-effects-report.txt"
objects_scenario_report="$log_dir/scenario-objects-report.txt"
runtime_root=""
scenario_root=""
family_scenario_root=""
class_family_scenario_root=""
permdeath_scenario_root=""
trap_scenario_root=""
talk_give_scenario_root=""
talk_effects_scenario_root=""
objects_scenario_root=""

log() {
  printf '%s\n' "$*" | tee -a "$summary_log"
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "$1 is required" >&2
    exit 127
  fi
}

run_to_log() {
  local out="$1"
  shift
  log "+ $*"
  "$@" >"$out" 2>&1
}

capture_tmux() {
  local session="$1"
  local out="$2"
  tmux capture-pane -pt "$session" -S -4000 >"$out" 2>/dev/null || true
}

capture_tmux_raw() {
  local session="$1"
  local out="$2"
  tmux capture-pane -e -p -t "$session" -S -4000 >"$out" 2>/dev/null || true
}

capture_exit_tmux_artifacts() {
  command -v tmux >/dev/null 2>&1 || return 0
  if tmux has-session -t "$server_session" 2>/dev/null; then
    capture_tmux "$server_session" "$log_dir/server-exit.log"
  fi
  if tmux has-session -t "$client_session" 2>/dev/null; then
    capture_tmux "$client_session" "$log_dir/client-exit.log"
    if [[ "$ansi" == "true" ]]; then
      capture_tmux_raw "$client_session" "$log_dir/client-exit-raw.log"
    fi
  fi
  if tmux has-session -t "$second_client_session" 2>/dev/null; then
    capture_tmux "$second_client_session" "$log_dir/client-second-exit.log"
  fi
}

assert_fixed() {
  local file="$1"
  local needle="$2"
  local label="$3"
  if ! grep -Fq "$needle" "$file"; then
    echo "missing expected output: $label" >&2
    echo "file: $file" >&2
    tail -80 "$file" >&2 || true
    exit 1
  fi
}

assert_regex() {
  local file="$1"
  local pattern="$2"
  local label="$3"
  if ! grep -Eq "$pattern" "$file"; then
    echo "missing expected output: $label" >&2
    echo "file: $file" >&2
    tail -80 "$file" >&2 || true
    exit 1
  fi
}

assert_ansi_escape() {
  local file="$1"
  local label="$2"
  if ! LC_ALL=C grep -q $'\033\\[' "$file"; then
    echo "missing ANSI escape output: $label" >&2
    echo "file: $file" >&2
    tail -80 "$file" >&2 || true
    exit 1
  fi
}

assert_absent_regex() {
  local file="$1"
  local pattern="$2"
  local label="$3"
  if grep -Eq "$pattern" "$file"; then
    echo "unexpected output: $label" >&2
    echo "file: $file" >&2
    tail -80 "$file" >&2 || true
    exit 1
  fi
}

assert_file_exists() {
  local path="$1"
  local label="$2"
  if [[ ! -e "$path" ]]; then
    echo "missing expected file: $label" >&2
    echo "path: $path" >&2
    exit 1
  fi
}

assert_file_absent() {
  local path="$1"
  local label="$2"
  if [[ -e "$path" ]]; then
    echo "unexpected file still exists: $label" >&2
    echo "path: $path" >&2
    exit 1
  fi
}

wait_for_tmux_output() {
  local session="$1"
  local needle="$2"
  local out="$3"
  local seconds="${4:-20}"
  local i
  for ((i = 0; i < seconds; i++)); do
    capture_tmux "$session" "$out"
    if grep -Fq "$needle" "$out"; then
      return 0
    fi
    if ! tmux has-session -t "$session" 2>/dev/null; then
      break
    fi
    sleep 1
  done
  echo "timed out waiting for '$needle' in tmux session $session" >&2
  tail -120 "$out" >&2 || true
  exit 1
}

tmux_output_has_prompt() {
  local out="$1"
  grep -Fq "> " "$out" || grep -Fxq ">" "$out"
}

wait_for_tmux_prompt() {
  local session="$1"
  local out="$2"
  local seconds="${3:-20}"
  local i
  for ((i = 0; i < seconds; i++)); do
    capture_tmux "$session" "$out"
    if tmux_output_has_prompt "$out"; then
      return 0
    fi
    if ! tmux has-session -t "$session" 2>/dev/null; then
      break
    fi
    sleep 1
  done
  echo "timed out waiting for prompt in tmux session $session" >&2
  tail -120 "$out" >&2 || true
  exit 1
}

copy_data_root() {
  local out_var="$1"
  local prefix="$2"
  local out_log="$3"
  local copied_root

  require_cmd tar
  copied_root="$(mktemp -d "${TMPDIR:-/tmp}/${prefix}.XXXXXX")"
  log "+ copy disposable root: $copied_root"
  {
    (
      cd "$data_root"
      tar \
        --exclude='./.git' \
        --exclude='./.gocache' \
        --exclude='./gocache' \
        --exclude='./go-cache' \
        --exclude='./tmp_gocache' \
        -cf - .
    ) | (
      cd "$copied_root"
      tar -xf -
    )
  } >"$out_log" 2>&1
  printf -v "$out_var" '%s' "$copied_root"
}

start_smoke_server() {
  local root="$1"
  local out="$2"
  local listen_addr="$host:$port"
  local server_cmd

  printf -v server_cmd 'cd %q && go run ./cmd/muhan-server -root %q -listen %q -ansi=%s' \
    "$repo_root" "$root" "$listen_addr" "$ansi"
  log "+ tmux new-session -d -s $server_session '$server_cmd'"
  tmux new-session -d -s "$server_session" "$server_cmd"
  wait_for_tmux_output "$server_session" "listening:" "$out" 40
}

connect_smoke_client() {
  local out="$1"
  local session="${2:-$client_session}"
  local client_cmd

  printf -v client_cmd 'cd %q && go run ./cmd/muhan-client -addr %q' \
    "$repo_root" "$host:$port"
  log "+ tmux new-session -d -s $session '$client_cmd'"
  tmux new-session -d -s "$session" "$client_cmd"
  wait_for_tmux_output "$session" "당신의 이름" "$out" 20
}

login_smoke_client() {
  local out="$1"
  local name="$2"
  local password="$3"
  local session="${4:-$client_session}"

  tmux send-keys -t "$session" "$name" C-m
  wait_for_tmux_output "$session" "암호" "$out" 20
  tmux send-keys -t "$session" "$password" C-m
  wait_for_tmux_output "$session" "무한에 접속했습니다" "$out" 20
  local i
  for ((i = 0; i < 10; i++)); do
    capture_tmux "$session" "$out"
    if tmux_output_has_prompt "$out"; then
      return 0
    fi
    if grep -Fq "엔터" "$out"; then
      tmux send-keys -t "$session" C-m
    fi
    sleep 1
  done
  wait_for_tmux_prompt "$session" "$out" 20
}

write_file_manifest() {
  local out="$1"
  shift

  : >"$out"
  local path
  for path in "$@"; do
    if [[ -e "$path" ]]; then
      printf 'present ' >>"$out"
      cksum "$path" >>"$out"
    else
      printf 'missing %s\n' "$path" >>"$out"
    fi
  done
}

assert_manifest_unchanged() {
  local before="$1"
  local after="$2"
  local label="$3"

  if ! cmp -s "$before" "$after"; then
    echo "scenario no-op changed files: $label" >&2
    diff -u "$before" "$after" >&2 || true
    exit 1
  fi
}

json_int_value() {
  local path="$1"
  local key="$2"

  sed -nE "s/.*\"${key}\"[[:space:]]*:[[:space:]]*([0-9]+).*/\\1/p" "$path" | head -1
}

decode_legacy_text_file() {
  local path="$1"

  if [[ ! -f "$path" ]]; then
    return 1
  fi
  if iconv -f utf-8 -t utf-8 "$path" >/dev/null 2>&1; then
    cat "$path"
  else
    iconv -f cp949 -t utf-8 "$path"
  fi
}

display_legacy_path() {
  local path="$1"
  local rel="${path#$data_root/}"
  local decoded

  if printf '%s' "$rel" | iconv -f utf-8 -t utf-8 >/dev/null 2>&1; then
    printf '%s' "$rel"
    return
  fi
  if decoded="$(printf '%s' "$rel" | iconv -f cp949 -t utf-8 2>/dev/null)"; then
    printf '%s' "$decoded"
    return
  fi
  printf '%s' "$rel"
}

detect_cmd_by_handler() {
  local cmdlist_file="$1"
  local handler="$2"

  awk -v handler="$handler" '$NF == handler || $(NF-1) == handler { print $1; exit }' "$cmdlist_file"
}

family_boss_name_for_id() {
  local family_list="$1"
  local family_id="$2"

  decode_legacy_text_file "$family_list" |
    awk -v family_id="$family_id" '$1 == family_id && NF >= 3 { print $3; exit }'
}

family_file_has_member() {
  local family_file="$1"
  local member_name="$2"

  decode_legacy_text_file "$family_file" |
    awk -v member_name="$member_name" '$1 != "0" && NF >= 2 && $2 == member_name { found = 1 } END { exit found ? 0 : 1 }'
}

family_file_member_class_is() {
  local family_file="$1"
  local member_name="$2"
  local class_id="$3"

  decode_legacy_text_file "$family_file" |
    awk -v member_name="$member_name" -v class_id="$class_id" '$1 == class_id && NF >= 2 && $2 == member_name { found = 1 } END { exit found ? 0 : 1 }'
}

family_file_kick_candidate() {
  local family_file="$1"
  local login="$2"
  local boss="$3"

  decode_legacy_text_file "$family_file" |
    awk -v login="$login" -v boss="$boss" '
      $1 != "0" && NF >= 2 && $2 != login && $2 != boss {
        print $2
        exit
      }'
}

json_has_positive_int_key() {
  local path="$1"
  local key="$2"
  local value

  value="$(json_int_value "$path" "$key" || true)"
  [[ -n "$value" && "$value" -gt 0 ]]
}

seed_family_scenario_boss_flag() {
  local player_json="$1"
  local tmp

  require_cmd jq
  tmp="$(mktemp "$(dirname "$player_json")/.family-boss-seed.XXXXXX")"
  if jq '
      .creature.stats.PFMBOS = 1
      | .creature.stats.familyBoss = 1
      | .creature.stats.familyBossFlag = 1
      | .creature.metadata.tags = (((.creature.metadata.tags // []) + ["PFMBOS"]) | unique)
    ' "$player_json" >"$tmp"; then
    chmod --reference="$player_json" "$tmp" 2>/dev/null || true
    mv "$tmp" "$player_json"
    return 0
  fi
  rm -f "$tmp"
  return 1
}

seed_class_family_scenario_player() {
  local player_json="$1"
  local room="$2"
  local source_class="$3"
  local experience="$4"
  local tmp

  require_cmd jq
  tmp="$(mktemp "$(dirname "$player_json")/.class-family-seed.XXXXXX")"
  if jq \
    --arg room "$room" \
    --argjson source_class "$source_class" \
    --argjson experience "$experience" '
      .player.roomId = $room
      | .creature.roomId = $room
      | .creature.stats.class = $source_class
      | .creature.stats.experience = $experience
      | .creature.stats.familyFlag = 1
      | .creature.stats.PFAMIL = 1
      | .creature.metadata.tags = (((.creature.metadata.tags // []) + ["PFAMIL"]) | unique)
    ' "$player_json" >"$tmp"; then
    chmod --reference="$player_json" "$tmp" 2>/dev/null || true
    mv "$tmp" "$player_json"
    return 0
  fi
  rm -f "$tmp"
  return 1
}

seed_permdeath_scenario_player() {
  local player_json="$1"
  local room="$2"
  local tmp

  require_cmd jq
  tmp="$(mktemp "$(dirname "$player_json")/.permdeath-seed.XXXXXX")"
  if jq \
    --arg room "$room" '
      .player.roomId = $room
      | .creature.roomId = $room
      | .creature.stats.class = 13
      | .creature.stats.hpCurrent = 3500
      | .creature.stats.hpMax = 3500
      | .creature.stats.mpCurrent = 2500
      | .creature.stats.mpMax = 2500
      | .creature.stats.strength = 100
      | .creature.stats.dexterity = 100
      | .creature.stats.thaco = -100
      | .creature.stats.nDice = 100
      | .creature.stats.sDice = 100
      | .creature.stats.pDice = 100
    ' "$player_json" >"$tmp"; then
    chmod --reference="$player_json" "$tmp" 2>/dev/null || true
    mv "$tmp" "$player_json"
    return 0
  fi
  rm -f "$tmp"
  return 1
}

seed_trap_scenario_player() {
  local player_json="$1"
  local room_id="$2"
  local hp_current="$3"
  local hp_max="$4"
  local dexterity="$5"
  local tmp

  require_cmd jq
  tmp="$(mktemp "$(dirname "$player_json")/.trap-scenario-seed.XXXXXX")"
  if jq \
    --arg room "$room_id" \
    --argjson hp_current "$hp_current" \
    --argjson hp_max "$hp_max" \
    --argjson dexterity "$dexterity" '
      .player.roomId = $room
      | .creature.roomId = $room
      | .creature.stats.hpCurrent = $hp_current
      | .creature.stats.hpMax = $hp_max
      | .creature.stats.dexterity = $dexterity
      | .player.metadata.tags = ((.player.metadata.tags // []) - ["prepared", "prepare", "PPREPA", "levitate", "levitation", "PLEVIT", "SLEVIT"])
      | .creature.metadata.tags = ((.creature.metadata.tags // []) - ["prepared", "prepare", "PPREPA", "levitate", "levitation", "PLEVIT", "SLEVIT"])
    ' "$player_json" >"$tmp"; then
    chmod --reference="$player_json" "$tmp" 2>/dev/null || true
    mv "$tmp" "$player_json"
    return 0
  fi
  rm -f "$tmp"
  return 1
}

seed_talk_give_scenario_player() {
  local player_json="$1"
  local room_id="$2"
  local tmp

  require_cmd jq
  tmp="$(mktemp "$(dirname "$player_json")/.talk-give-seed.XXXXXX")"
  if jq \
    --arg room "$room_id" '
      .player.roomId = $room
      | .creature.roomId = $room
      | .creature.inventory.objectIds = []
      | .creature.equipment = {}
      | .objects = []
      | .creature.stats.hpCurrent = 3500
      | .creature.stats.hpMax = 3500
      | .creature.stats.strength = 60
      | .creature.properties = ((.creature.properties // {}) | with_entries(select(.key | startswith("quest_completed_") | not)))
    ' "$player_json" >"$tmp"; then
    chmod --reference="$player_json" "$tmp" 2>/dev/null || true
    mv "$tmp" "$player_json"
    return 0
  fi
  rm -f "$tmp"
  return 1
}

seed_talk_effects_scenario_player() {
  local player_json="$1"
  local room_id="$2"
  local hp_current="$3"
  local hp_max="$4"
  local tmp

  require_cmd jq
  tmp="$(mktemp "$(dirname "$player_json")/.talk-effects-seed.XXXXXX")"
  if jq \
    --arg room "$room_id" \
    --argjson hp_current "$hp_current" \
    --argjson hp_max "$hp_max" '
      .player.roomId = $room
      | .creature.roomId = $room
      | .creature.stats.hpCurrent = $hp_current
      | .creature.stats.hpMax = $hp_max
      | .creature.stats.mpCurrent = 2500
      | .creature.stats.mpMax = 2500
      | .creature.stats.strength = 80
      | .creature.stats.dexterity = 80
      | .creature.stats.thaco = -20
      | .creature.metadata.tags = ((.creature.metadata.tags // []) - ["was_attacked", "hidden", "hide", "PHIDDN"])
    ' "$player_json" >"$tmp"; then
    chmod --reference="$player_json" "$tmp" 2>/dev/null || true
    mv "$tmp" "$player_json"
    return 0
  fi
  rm -f "$tmp"
  return 1
}

seed_objects_scenario_player() {
  local player_json="$1"
  local room_id="$2"
  local nested_source_json="$3"
  local nested_bag_id="$4"
  local nested_sword_id="$5"
  local tmp

  require_cmd jq
  tmp="$(mktemp "$(dirname "$player_json")/.objects-scenario-seed.XXXXXX")"
  if jq \
    --slurpfile src "$nested_source_json" \
    --arg room "$room_id" \
    --arg bag "$nested_bag_id" \
    --arg sword "$nested_sword_id" '
      ($src[0].objects[] | select(.id == "player:까마귀:inventory:20:00008308")) as $sourceBag
      | ($src[0].objects[] | select(.id == "player:까마귀:inventory:20:00008664")) as $sourceSword
      | (.creature.id // ("creature:" + .player.id)) as $creatureID
      | ($sourceBag
          | .id = $bag
          | .location = {creatureId: $creatureID, slot: "inventory"}
          | .contents.objectIds = [$sword]
          | .metadata.notes = (((.metadata.notes // []) + ["scenario-cloned-from=player:까마귀:inventory:20:00008308"]) | unique)
        ) as $bagObject
      | ($sourceSword
          | .id = $sword
          | .location = {containerId: $bag}
          | .metadata.notes = (((.metadata.notes // []) + ["scenario-cloned-from=player:까마귀:inventory:20:00008664"]) | unique)
        ) as $swordObject
      | .player.roomId = $room
      | .creature.roomId = $room
      | .creature.inventory.objectIds = [$bag]
      | .creature.equipment = {}
      | .objects = [$bagObject, $swordObject]
      | .creature.stats = ((.creature.stats // {}) + {
          class: 13,
          hpCurrent: 3500,
          hpMax: 3500,
          strength: 80,
          inventoryObjects: 1
        })
    ' "$player_json" >"$tmp"; then
    chmod --reference="$player_json" "$tmp" 2>/dev/null || true
    mv "$tmp" "$player_json"
    return 0
  fi
  rm -f "$tmp"
  return 1
}

count_files_under() {
  local dir="$1"

  if [[ ! -d "$dir" ]]; then
    echo 0
    return
  fi
  find "$dir" -type f | wc -l | tr -d ' '
}

count_text_pattern() {
  local dir="$1"
  local pattern="$2"

  if [[ ! -d "$dir" ]]; then
    echo 0
    return
  fi
  { LC_ALL=C grep -R -I -E "$pattern" "$dir" 2>/dev/null || true; } | wc -l | tr -d ' '
}

sample_text_pattern() {
  local dir="$1"
  local pattern="$2"
  local max_lines="${3:-20}"

  if [[ ! -d "$dir" ]]; then
    echo "  (directory missing: $dir)"
    return
  fi
  { LC_ALL=C grep -R -I -n -E "$pattern" "$dir" 2>/dev/null || true; } |
    sed "s#^$data_root/##" |
    awk -v max="$max_lines" 'NR <= max { print "  " $0 } END { if (NR == 0) print "  (no text matches)" }'
}

sample_legacy_text_pattern() {
  local dir="$1"
  local pattern="$2"
  local max_lines="${3:-20}"
  local emitted=0
  local file

  if [[ ! -d "$dir" ]]; then
    echo "  (directory missing: $dir)"
    return
  fi
  while IFS= read -r -d '' file; do
    while IFS= read -r match; do
      printf '  %s:%s\n' "$(display_legacy_path "$file")" "$match"
      ((emitted += 1))
      if ((emitted >= max_lines)); then
        return
      fi
    done < <(decode_legacy_text_file "$file" 2>/dev/null | grep -n -E "$pattern" || true)
  done < <(find "$dir" -type f -print0 | sort -z)
  if ((emitted == 0)); then
    echo "  (no text matches)"
  fi
}

kill_smoke_sessions() {
  tmux kill-session -t "$client_session" 2>/dev/null || true
  tmux kill-session -t "$second_client_session" 2>/dev/null || true
  tmux kill-session -t "$server_session" 2>/dev/null || true
  kill_server_processes_for_root "$runtime_root"
  kill_server_processes_for_root "$scenario_root"
  kill_server_processes_for_root "$family_scenario_root"
  kill_server_processes_for_root "$class_family_scenario_root"
  kill_server_processes_for_root "$permdeath_scenario_root"
  kill_server_processes_for_root "$trap_scenario_root"
  kill_server_processes_for_root "$talk_give_scenario_root"
  kill_server_processes_for_root "$talk_effects_scenario_root"
  kill_server_processes_for_root "$objects_scenario_root"
}

kill_server_processes_for_root() {
  local root="$1"
  local listen_arg="-listen $host:$port"
  local pid
  local cmd
  local matched=0

  if [[ -z "$root" ]]; then
    return
  fi
  while IFS= read -r pid; do
    [[ -n "$pid" ]] || continue
    cmd="$(ps -p "$pid" -o args= 2>/dev/null || true)"
    if [[ "$cmd" == *"muhan-server"* && "$cmd" == *" -root $root"* && "$cmd" == *" $listen_arg"* ]]; then
      kill "$pid" 2>/dev/null || true
      matched=1
    fi
  done < <(pgrep -f 'muhan-server' 2>/dev/null || true)
  if ((matched)); then
    sleep 1
    while IFS= read -r pid; do
      [[ -n "$pid" ]] || continue
      cmd="$(ps -p "$pid" -o args= 2>/dev/null || true)"
      if [[ "$cmd" == *"muhan-server"* && "$cmd" == *" -root $root"* && "$cmd" == *" $listen_arg"* ]]; then
        kill -9 "$pid" 2>/dev/null || true
      fi
    done < <(pgrep -f 'muhan-server' 2>/dev/null || true)
  fi
}

cleanup() {
  if ((clean_sessions)); then
    kill_smoke_sessions
  fi
  if [[ -n "$runtime_root" && "$runtime_root" != "$data_root" ]]; then
    rm -rf "$runtime_root"
  fi
  if [[ -n "$scenario_root" && "$scenario_root" != "$data_root" ]]; then
    rm -rf "$scenario_root"
  fi
  if [[ -n "$family_scenario_root" && "$family_scenario_root" != "$data_root" ]]; then
    rm -rf "$family_scenario_root"
  fi
  if [[ -n "$class_family_scenario_root" && "$class_family_scenario_root" != "$data_root" ]]; then
    rm -rf "$class_family_scenario_root"
  fi
  if [[ -n "$permdeath_scenario_root" && "$permdeath_scenario_root" != "$data_root" ]]; then
    rm -rf "$permdeath_scenario_root"
  fi
  if [[ -n "$trap_scenario_root" && "$trap_scenario_root" != "$data_root" ]]; then
    rm -rf "$trap_scenario_root"
  fi
  if [[ -n "$talk_give_scenario_root" && "$talk_give_scenario_root" != "$data_root" ]]; then
    rm -rf "$talk_give_scenario_root"
  fi
  if [[ -n "$talk_effects_scenario_root" && "$talk_effects_scenario_root" != "$data_root" ]]; then
    rm -rf "$talk_effects_scenario_root"
  fi
  if [[ -n "$objects_scenario_root" && "$objects_scenario_root" != "$data_root" ]]; then
    rm -rf "$objects_scenario_root"
  fi
}

write_gap_report() {
  {
    echo "# Porting Gap Smoke Report"
    echo
    echo "root: $data_root"
    echo "generated: $(date -Iseconds)"
    echo "artifacts: $log_dir"
    echo
    echo "## Executed Signals"
    if [[ -f "$log_dir/go-test.log" ]]; then
      grep -E '^(FAIL|ok  |[?]   )' "$log_dir/go-test.log" | tail -40 || true
    fi
    if [[ -f "$log_dir/go-build-server.log" ]]; then
      if [[ -s "$log_dir/go-build-server.log" ]]; then
        echo
        echo "server build diagnostics:"
        tail -40 "$log_dir/go-build-server.log" || true
      else
        echo "server build: ok"
      fi
    fi
    if [[ -f "$log_dir/validate.log" ]]; then
      grep -E '^(registry:|world:|findings:|runtime world:|mode:)' "$log_dir/validate.log" || true
    fi
    if [[ -f "$log_dir/cmdlist.log" ]]; then
      grep -m1 'command entries' "$log_dir/cmdlist.log" || true
    fi
    if [[ -f "$log_dir/sidecarmigrate.json" ]]; then
      grep -E '"(totalScanned|migrated|errors)"' "$log_dir/sidecarmigrate.json" || true
      local sidecar_types
      sidecar_types="$(jq -r '.byType // {} | to_entries | sort_by(.key) | map("\(.key)=\(.value)") | join(", ")' "$log_dir/sidecarmigrate.json" 2>/dev/null || true)"
      if [[ -n "$sidecar_types" ]]; then
        echo "sidecar by type: $sidecar_types"
      fi
    fi
    if [[ -f "$log_dir/client.log" ]]; then
      if [[ -f "$log_dir/client-second.log" ]]; then
        echo "runtime smoke: login/read-only/multi-client transcripts captured"
      else
        echo "runtime smoke: login/read-only command transcript captured"
      fi
    fi
    if [[ -f "$log_dir/client-ansi-raw.log" ]]; then
      echo "runtime ANSI smoke: raw client escape transcript captured"
    fi
    if [[ -f "$log_dir/server-exit.log" || -f "$log_dir/client-exit.log" || -f "$log_dir/client-second-exit.log" ]]; then
      echo "tmux exit capture: server/client pane snapshots preserved"
    fi
    if [[ -f "$log_dir/scenario-suicide.log" ]]; then
      echo "scenario smoke: suicide wrong-password/cancel/final cleanup on disposable root"
    fi
    if [[ -f "$family_scenario_report" ]]; then
      grep -m1 '^status:' "$family_scenario_report" | sed 's/^/family scenario smoke: /' || true
    fi
    if [[ -f "$class_family_scenario_report" ]]; then
      grep -m1 '^status:' "$class_family_scenario_report" | sed 's/^/class-family scenario smoke: /' || true
    fi
    if [[ -f "$permdeath_scenario_report" ]]; then
      grep -m1 '^status:' "$permdeath_scenario_report" | sed 's/^/permanent death scenario smoke: /' || true
    fi
    if [[ -f "$trap_scenario_report" ]]; then
      grep -m1 '^status:' "$trap_scenario_report" | sed 's/^/trap scenario smoke: /' || true
    fi
    if [[ -f "$talk_give_scenario_report" ]]; then
      grep -m1 '^status:' "$talk_give_scenario_report" | sed 's/^/talk GIVE scenario smoke: /' || true
    fi
    if [[ -f "$talk_effects_scenario_report" ]]; then
      grep -m1 '^status:' "$talk_effects_scenario_report" | sed 's/^/talk effects scenario smoke: /' || true
    fi
    if [[ -f "$objects_scenario_report" ]]; then
      grep -m1 '^status:' "$objects_scenario_report" | sed 's/^/object materialization scenario smoke: /' || true
    fi
    if [[ -f "$live_gap_report" ]]; then
      echo "live gap report: non-mutating fixture inventory captured"
    fi
    if [[ -f "$log_dir/scenario-validate.log" ]]; then
      grep -E '^(registry:|world:|findings:|runtime world:|mode:)' "$log_dir/scenario-validate.log" || true
    fi
    echo
    echo "## Dedicated Scenario Smoke Queue"
    echo "- suicide lifecycle: covered by scripts/porting-smoke.sh --scenario for wrong-password/no-op/final deletion on disposable root"
    if [[ -f "$family_scenario_report" ]]; then
      local family_scenario_status
      family_scenario_status="$(grep -m1 '^status:' "$family_scenario_report" | sed 's/^status:[[:space:]]*//' || true)"
      if [[ "$family_scenario_status" == "kicked" ]]; then
        echo "- family registry persistence: --scenario-family covered disposable kick/restart visibility; see $family_scenario_report"
      else
        echo "- family registry persistence: --scenario-family captured a disposable read transcript but skipped kick; still needs boss login or safe candidate. See $family_scenario_report"
      fi
    else
      echo "- family registry persistence: --scenario-family covers disposable kick/restart visibility; run it to refresh transcript artifacts"
    fi
    if [[ -f "$class_family_scenario_report" ]] && grep -Fq 'status: class-changed' "$class_family_scenario_report"; then
      echo "- family class-change persistence: --scenario-class-family covered disposable 직업전환, family_member_N rewrite, validate, and restart visibility"
    else
      echo "- family class-change persistence: --scenario-class-family covers disposable 직업전환/restart visibility; run it to refresh artifacts"
    fi
    if [[ -f "$trap_scenario_report" ]] && grep -Fq 'status: pit-death+alarm' "$trap_scenario_report"; then
      echo "- trap/death integration: --scenario-trap covered disposable legacy pit death and alarm guard transcripts plus validate"
    elif [[ -f "$trap_scenario_report" ]] && grep -Fq 'status: pit-death' "$trap_scenario_report"; then
      echo "- trap/death integration: --scenario-trap covered disposable legacy pit route, fatal damage, player death output, and validate; alarm live transcript still pending"
    else
      echo "- trap/death integration: --scenario-trap covers disposable legacy pit death and alarm guard transcripts; run it to refresh artifacts"
    fi
    if [[ -f "$permdeath_scenario_report" ]] && grep -Fq 'status: permanent-death' "$permdeath_scenario_report"; then
      echo "- permanent monster death: --scenario-permdeath covered live MDEATH/MSUMMO/quest transcript, room ltime sidecar, and validate"
    else
      echo "- permanent monster death: unit-covered MDEATH/respawn/quest/MSUMMO hooks; run --scenario-permdeath to refresh live combat transcript"
    fi
    if [[ -f "$objects_scenario_report" ]] && grep -Fq 'status: orench+nested-restart' "$objects_scenario_report"; then
      echo "- ORENCH/nested object materialization: --scenario-objects covered live DM create, nested container look/drop, room resave, validate, and restart transcript"
    else
      echo "- ORENCH/nested object materialization: unit-covered clone/enchant and save/restart paths; run --scenario-objects to refresh live create/drop/restart transcript"
    fi
    if [[ -f "$talk_give_scenario_report" ]] && grep -Fq 'status: give' "$talk_give_scenario_report" &&
      [[ -f "$talk_effects_scenario_report" ]] && grep -Fq 'status: action+cast+attack' "$talk_effects_scenario_report"; then
      echo "- talk side effects: --scenario-talk-give and --scenario-talk-effects covered disposable GIVE plus real ACTION/CAST/ATTACK transcripts and validate"
    elif [[ -f "$talk_give_scenario_report" ]] && grep -Fq 'status: give' "$talk_give_scenario_report"; then
      echo "- talk side effects: --scenario-talk-give covered disposable MTALKS + real GIVE transcript and validate"
    elif [[ -f "$talk_effects_scenario_report" ]] && grep -Fq 'status: action+cast+attack' "$talk_effects_scenario_report"; then
      echo "- talk side effects: --scenario-talk-effects covered disposable real ACTION/CAST/ATTACK transcripts and validate"
    else
      echo "- talk side effects: real-data unit-covered ACTION/GIVE/ATTACK/CAST, MTALKS, duplicate reward, and rollback; run --scenario-talk-give and --scenario-talk-effects to refresh live transcripts"
    fi
    if [[ -f "$live_gap_report" ]]; then
      echo
      echo "See live fixture inventory: $live_gap_report"
    else
      echo
      echo "Run scripts/porting-smoke.sh --live-gap-report --no-preflight --no-sidecar to refresh non-mutating fixture evidence."
    fi
  } >"$gap_report"
}

write_live_gap_report() {
  local talk_dir="$data_root/objmon/talk"
  local room_dir="$data_root/rooms"
  local room_sidecar_dir="$data_root/room/json"
  local object_dir="$data_root/objmon"
  local family_dir="$data_root/player/family"
  local player_json="$data_root/player/json/$login_name.json"
  local family_id=""
  local family_file=""

  if [[ -f "$player_json" ]]; then
    family_id="$(json_int_value "$player_json" "familyID" || true)"
  fi
  if [[ -n "$family_id" ]]; then
    family_file="$family_dir/family_member_$family_id"
  fi

  {
    echo "# Live Gap Evidence Report"
    echo
    echo "static inventory root: $data_root"
    if [[ -n "$runtime_root" ]]; then
      echo "runtime root: $runtime_root"
    else
      echo "runtime root: not started in this run"
    fi
    echo "generated: $(date -Iseconds)"
    echo "mode: non-mutating inventory; no destructive commands are sent by this option"
    echo
    echo "## Static Fixture Inventory"
    echo "- talk files: $(count_files_under "$talk_dir")"
    echo "- talk ACTION directives: $(count_text_pattern "$talk_dir" '[[:space:]]ACTION([[:space:]]|$)')"
    echo "- talk GIVE directives: $(count_text_pattern "$talk_dir" '[[:space:]]GIVE([[:space:]]|$)')"
    echo "- talk ATTACK directives: $(count_text_pattern "$talk_dir" '[[:space:]]ATTACK([[:space:]]|$)')"
    echo "- talk CAST directives: $(count_text_pattern "$talk_dir" '[[:space:]]CAST([[:space:]]|$)')"
    echo "- legacy room files: $(count_files_under "$room_dir")"
    echo "- room object sidecars: $(count_files_under "$room_sidecar_dir")"
    echo "- object/creature data files: $(count_files_under "$object_dir")"
    echo "- text ORENCH signatures: $(count_text_pattern "$object_dir" 'ORENCH')"
    echo "- text MDEATH/MSUMMO signatures: $(count_text_pattern "$object_dir" 'MDEATH|MSUMMO')"
    echo "- text trap/alarm signatures in rooms: $(count_text_pattern "$room_dir" 'TRAP|trap|ALARM|alarm')"
    echo "- note: ORENCH, MDEATH/MSUMMO, and trap counts are best-effort text scans; legacy binary records may not expose those tokens."
    echo
    echo "### Talk Side-Effect Candidates"
    sample_legacy_text_pattern "$talk_dir" '[[:space:]](ACTION|GIVE|ATTACK|CAST)([[:space:]]|$)' 30
    echo
    echo "### Family Candidate"
    echo "- login: $login_name"
    if [[ -f "$player_json" ]]; then
      echo "- player sidecar: ${player_json#$data_root/}"
      echo "- familyID: ${family_id:-not set}"
    else
      echo "- player sidecar: missing (${player_json#$data_root/})"
    fi
    if [[ -n "$family_file" && -f "$family_file" ]]; then
      echo "- family member file: ${family_file#$data_root/}"
      if iconv -f utf-8 -t utf-8 "$family_file" >/dev/null 2>&1; then
        echo "- family member UTF-8: yes"
      else
        echo "- family member UTF-8: no or legacy bytes still present"
      fi
      if grep -Fq "$login_name" "$family_file"; then
        echo "- login appears in family member file: yes"
      else
        echo "- login appears in family member file: no"
      fi
    elif [[ -n "$family_id" ]]; then
      echo "- family member file: missing (${family_file#$data_root/})"
    else
      echo "- family member file: not applicable without familyID"
    fi
    echo
    echo "### Runtime Transcript Evidence"
    if [[ -f "$log_dir/client.log" ]]; then
      echo "- client transcript: ${log_dir#$repo_root/}/client.log"
      for marker in "통계 무한 광장" "생명의 나무" "계석치무" "접속자" "$login_name"; do
        if grep -Fq "$marker" "$log_dir/client.log"; then
          echo "- marker '$marker': seen"
        else
          echo "- marker '$marker': not seen"
        fi
      done
    else
      echo "- client transcript: not captured in this run"
      echo "- recommended safe run: scripts/porting-smoke.sh --runtime --copy-root --live-gap-report --skip-go-test --no-sidecar"
    fi
    echo
    echo "## Remaining Live Evidence Queue"
    if [[ -f "$family_scenario_report" ]] && grep -Fq 'status: kicked' "$family_scenario_report"; then
      echo "- family registry: covered in this run by --scenario-family disposable 패거리원/패거리추방/restart transcript."
    else
      echo "- family registry: covered by --scenario-family; run it when transcript artifacts need refresh."
    fi
    if [[ -f "$trap_scenario_report" ]] && grep -Fq 'status: pit-death+alarm' "$trap_scenario_report"; then
      echo "- trap/death: pit/death and alarm guard visibility covered in this run by --scenario-trap disposable legacy room transcripts."
    elif [[ -f "$trap_scenario_report" ]] && grep -Fq 'status: pit-death' "$trap_scenario_report"; then
      echo "- trap/death: pit/death covered in this run by --scenario-trap disposable legacy room transcript; alarm transcript still pending."
    else
      echo "- trap/death: covered by --scenario-trap; run it when transcript artifacts need refresh."
    fi
    echo "- permanent monster death: --scenario-permdeath covers real 타타르 fixture MDEATH/MSUMMO/quest output, room sidecar ltime, and validate."
    if [[ -f "$objects_scenario_report" ]] && grep -Fq 'status: orench+nested-restart' "$objects_scenario_report"; then
      echo "- ORENCH/nested object: covered in this run by --scenario-objects disposable DM create + nested drop/restart transcript."
    else
      echo "- ORENCH/nested object: covered by --scenario-objects; run it when transcript artifacts need refresh."
    fi
    if [[ -f "$talk_give_scenario_report" ]] && grep -Fq 'status: give' "$talk_give_scenario_report" &&
      [[ -f "$talk_effects_scenario_report" ]] && grep -Fq 'status: action+cast+attack' "$talk_effects_scenario_report"; then
      echo "- talk side effects: MTALKS/GIVE plus real ACTION/CAST/ATTACK covered in this run by disposable transcripts."
    elif [[ -f "$talk_give_scenario_report" ]] && grep -Fq 'status: give' "$talk_give_scenario_report"; then
      echo "- talk side effects: MTALKS + real GIVE covered in this run by --scenario-talk-give disposable transcript."
    elif [[ -f "$talk_effects_scenario_report" ]] && grep -Fq 'status: action+cast+attack' "$talk_effects_scenario_report"; then
      echo "- talk side effects: real ACTION/CAST/ATTACK covered in this run by --scenario-talk-effects disposable transcripts."
    else
      echo "- talk side effects: run --scenario-talk-give for a disposable MTALKS + real GIVE transcript and --scenario-talk-effects for broader ACTION/CAST/ATTACK transcripts."
    fi
  } >"$live_gap_report"
}

run_suicide_scenario_smoke() {
  require_cmd tmux

  copy_data_root scenario_root "muhan-scenario-root" "$log_dir/scenario-copy.log"
  local scenario_server_log="$log_dir/scenario-server.log"
  local scenario_client_log="$log_dir/scenario-suicide.log"
  local manifest_before="$log_dir/scenario-suicide-before.manifest"
  local manifest_wrong="$log_dir/scenario-suicide-wrong-password.manifest"
  local manifest_cancel="$log_dir/scenario-suicide-cancel.manifest"
  local player_json="$scenario_root/player/json/$login_name.json"
  local family_id
  local family_file=""
  local family_had_member=0
  local original_files=()
  local manifest_files=()
  local path

  assert_file_exists "$player_json" "scenario login player sidecar"
  family_id="$(json_int_value "$player_json" "familyID" || true)"
  if [[ -n "$family_id" && -f "$scenario_root/player/family/family_member_$family_id" ]]; then
    family_file="$scenario_root/player/family/family_member_$family_id"
    if grep -Fq "$login_name" "$family_file"; then
      family_had_member=1
    fi
  fi

  while IFS= read -r path; do
    original_files+=("$path")
    manifest_files+=("$path")
  done < <(find "$scenario_root/player" -maxdepth 3 -type f \( -name "$login_name" -o -name "$login_name.json" \) | sort)
  if [[ -n "$family_file" ]]; then
    manifest_files+=("$family_file")
  fi
  if ((${#original_files[@]} == 0)); then
    echo "scenario found no player/bank/alias files for $login_name under $scenario_root/player" >&2
    exit 1
  fi

  write_file_manifest "$manifest_before" "${manifest_files[@]}"

  kill_smoke_sessions
  start_smoke_server "$scenario_root" "$scenario_server_log"
  connect_smoke_client "$scenario_client_log"
  login_smoke_client "$scenario_client_log" "$login_name" "$login_password"

  tmux send-keys -t "$client_session" "$suicide_command" C-m
  wait_for_tmux_output "$client_session" "당신의 현재 암호" "$scenario_client_log" 20
  tmux send-keys -t "$client_session" "wrong-password-for-smoke" C-m
  wait_for_tmux_output "$client_session" "암호가 틀립니다" "$scenario_client_log" 20
  write_file_manifest "$manifest_wrong" "${manifest_files[@]}"
  assert_manifest_unchanged "$manifest_before" "$manifest_wrong" "suicide wrong password"

  tmux send-keys -t "$client_session" "$suicide_command" C-m
  wait_for_tmux_output "$client_session" "당신의 현재 암호" "$scenario_client_log" 20
  tmux send-keys -t "$client_session" "$login_password" C-m
  wait_for_tmux_output "$client_session" "찐짜로? (찐짜로/뻥으로)" "$scenario_client_log" 20
  tmux send-keys -t "$client_session" "뻥으로" C-m
  sleep 1
  capture_tmux "$client_session" "$scenario_client_log"
  assert_fixed "$scenario_client_log" "삭제되지 않았습니다" "suicide cancel output"
  write_file_manifest "$manifest_cancel" "${manifest_files[@]}"
  assert_manifest_unchanged "$manifest_before" "$manifest_cancel" "suicide cancel"

  tmux send-keys -t "$client_session" "$suicide_command" C-m
  wait_for_tmux_output "$client_session" "당신의 현재 암호" "$scenario_client_log" 20
  tmux send-keys -t "$client_session" "$login_password" C-m
  wait_for_tmux_output "$client_session" "찐짜로? (찐짜로/뻥으로)" "$scenario_client_log" 20
  tmux send-keys -t "$client_session" "찐짜로" C-m
  wait_for_tmux_output "$server_session" "자살신청" "$scenario_server_log" 20
  sleep 1
  capture_tmux "$server_session" "$scenario_server_log"
  capture_tmux "$client_session" "$scenario_client_log"

  assert_absent_regex "$scenario_server_log" '(panic:|fatal error:)' "scenario server panic"
  assert_absent_regex "$scenario_client_log" '(panic:|fatal error:)' "scenario client panic"
  for path in "${original_files[@]}"; do
    assert_file_absent "$path" "suicide cleanup $(basename "$path")"
  done
  if ((family_had_member)); then
    if grep -Fq "$login_name" "$family_file"; then
      echo "scenario family_member still contains $login_name after suicide cleanup" >&2
      sed -n '1,80p' "$family_file" >&2 || true
      exit 1
    fi
    if ! iconv -f utf-8 -t utf-8 "$family_file" >/dev/null 2>&1; then
      echo "scenario family_member was not rewritten as valid UTF-8: $family_file" >&2
      exit 1
    fi
  fi

  kill_smoke_sessions
  run_to_log "$log_dir/scenario-validate.log" go run ./cmd/muhan-server -root "$scenario_root" -validate -ansi=false
  assert_fixed "$log_dir/scenario-validate.log" "runtime world: initialized" "scenario runtime world initialized"
  assert_regex "$log_dir/scenario-validate.log" 'findings: [0-9]+ warnings, 0 errors' "scenario validate has zero errors"
}

run_family_scenario_smoke() {
  require_cmd tmux

  copy_data_root family_scenario_root "muhan-family-scenario-root" "$log_dir/scenario-family-copy.log"
  if [[ -z "$family_scenario_root" || "$family_scenario_root" == "$data_root" || "$family_scenario_root" == "$repo_root" ]]; then
    echo "family scenario refuses to run on a non-disposable root: ${family_scenario_root:-unset}" >&2
    exit 1
  fi

  local family_server_log="$log_dir/scenario-family-server.log"
  local family_client_before_log="$log_dir/scenario-family-before.log"
  local family_client_after_kick_log="$log_dir/scenario-family-after-kick.log"
  local family_client_after_restart_log="$log_dir/scenario-family-after-restart.log"
  local family_cmdlist_log="$log_dir/scenario-family-cmdlist.log"
  local player_json="$family_scenario_root/player/json/$login_name.json"
  local family_id=""
  local family_file=""
  local family_list="$family_scenario_root/player/family/family_list"
  local family_boss=""
  local member_cmd=""
  local kick_cmd=""
  local kick_target=""
  local skip_reason=""
  local kicked=0
  local seeded_boss=0

  run_to_log "$family_cmdlist_log" go run ./cmd/muhan-cmdlist -root "$family_scenario_root"
  member_cmd="$(detect_cmd_by_handler "$family_cmdlist_log" "family_member" || true)"
  kick_cmd="$(detect_cmd_by_handler "$family_cmdlist_log" "fm_out" || true)"

  if [[ -f "$player_json" ]]; then
    family_id="$(json_int_value "$player_json" "familyID" || true)"
  fi
  if [[ -n "$family_id" ]]; then
    family_file="$family_scenario_root/player/family/family_member_$family_id"
  fi
  if [[ -n "$family_id" && -f "$family_list" ]]; then
    family_boss="$(family_boss_name_for_id "$family_list" "$family_id" || true)"
  fi

  if [[ -f "$player_json" && -n "$family_id" && -f "$family_file" ]] &&
    family_file_has_member "$family_file" "$login_name" &&
    [[ "$family_boss" != "$login_name" ]] &&
    ! json_has_positive_int_key "$player_json" "PFMBOS" &&
    ! json_has_positive_int_key "$player_json" "familyBoss" &&
    ! json_has_positive_int_key "$player_json" "familyBossFlag"; then
    if seed_family_scenario_boss_flag "$player_json"; then
      seeded_boss=1
    else
      skip_reason="failed to seed disposable boss flag in ${player_json#$family_scenario_root/}"
    fi
  fi

  kill_smoke_sessions
  start_smoke_server "$family_scenario_root" "$family_server_log"
  connect_smoke_client "$family_client_before_log"
  login_smoke_client "$family_client_before_log" "$login_name" "$login_password"

  if [[ -n "$member_cmd" ]]; then
    tmux send-keys -t "$client_session" "$member_cmd" C-m
    sleep 1
  fi
  capture_tmux "$client_session" "$family_client_before_log"
  capture_tmux "$server_session" "$family_server_log"
  assert_absent_regex "$family_server_log" '(panic:|fatal error:)' "family scenario server panic before kick"
  assert_absent_regex "$family_client_before_log" '(panic:|fatal error:)' "family scenario client panic before kick"

  if [[ -z "$member_cmd" ]]; then
    skip_reason="family_member command not found in cmdlist"
  elif [[ -z "$kick_cmd" ]]; then
    skip_reason="fm_out/family kick command not found in cmdlist"
  elif [[ ! -f "$player_json" ]]; then
    skip_reason="login player sidecar missing: ${player_json#$family_scenario_root/}"
  elif [[ -z "$family_id" || "$family_id" -le 0 ]]; then
    skip_reason="login player has no positive familyID"
  elif [[ ! -f "$family_file" ]]; then
    skip_reason="family member file missing: ${family_file#$family_scenario_root/}"
  elif ! family_file_has_member "$family_file" "$login_name"; then
    skip_reason="login player is not listed in ${family_file#$family_scenario_root/}"
  elif [[ "$family_boss" != "$login_name" ]] &&
    ! json_has_positive_int_key "$player_json" "PFMBOS" &&
    ! json_has_positive_int_key "$player_json" "familyBoss" &&
    ! json_has_positive_int_key "$player_json" "familyBossFlag"; then
    skip_reason="login $login_name is not the family boss${family_boss:+ (boss: $family_boss)}"
  else
    kick_target="$(family_file_kick_candidate "$family_file" "$login_name" "$family_boss" || true)"
    if [[ -z "$kick_target" ]]; then
      skip_reason="no safe non-login/non-boss family member candidate"
    fi
  fi

  if [[ -n "$skip_reason" ]]; then
    kill_smoke_sessions
    run_to_log "$log_dir/scenario-family-validate.log" go run ./cmd/muhan-server -root "$family_scenario_root" -validate -ansi=false
    assert_fixed "$log_dir/scenario-family-validate.log" "runtime world: initialized" "family scenario runtime world initialized"
    assert_regex "$log_dir/scenario-family-validate.log" 'findings: [0-9]+ warnings, 0 errors' "family scenario validate has zero errors"
    {
      echo "# Family Scenario Smoke Report"
      echo "status: skipped"
      echo "reason: $skip_reason"
      echo "root: $data_root"
      echo "disposable root: $family_scenario_root"
      echo "login: $login_name"
      echo "familyID: ${family_id:-not set}"
      echo "family boss: ${family_boss:-unknown}"
      echo "seeded boss flag: $seeded_boss"
      echo "family member command: ${member_cmd:-not found}"
      echo "family kick command: ${kick_cmd:-not found}"
      echo "before transcript: $family_client_before_log"
      echo "validate log: $log_dir/scenario-family-validate.log"
      echo "generated: $(date -Iseconds)"
    } >"$family_scenario_report"
    return
  fi

  tmux send-keys -t "$client_session" "$kick_cmd $kick_target" C-m
  wait_for_tmux_output "$client_session" "추방하였습니다" "$family_client_after_kick_log" 20
  sleep 1
  capture_tmux "$client_session" "$family_client_after_kick_log"
  capture_tmux "$server_session" "$family_server_log"
  assert_absent_regex "$family_server_log" '(panic:|fatal error:)' "family scenario server panic after kick"
  assert_absent_regex "$family_client_after_kick_log" '(panic:|fatal error:)' "family scenario client panic after kick"
  if family_file_has_member "$family_file" "$kick_target"; then
    echo "family scenario target still present in family_member file after kick: $kick_target" >&2
    decode_legacy_text_file "$family_file" >&2 || true
    exit 1
  fi
  if ! iconv -f utf-8 -t utf-8 "$family_file" >/dev/null 2>&1; then
    echo "family scenario family_member was not rewritten as valid UTF-8: $family_file" >&2
    exit 1
  fi
  kicked=1

  kill_smoke_sessions
  run_to_log "$log_dir/scenario-family-validate.log" go run ./cmd/muhan-server -root "$family_scenario_root" -validate -ansi=false
  assert_fixed "$log_dir/scenario-family-validate.log" "runtime world: initialized" "family scenario runtime world initialized"
  assert_regex "$log_dir/scenario-family-validate.log" 'findings: [0-9]+ warnings, 0 errors' "family scenario validate has zero errors"

  start_smoke_server "$family_scenario_root" "$family_server_log"
  connect_smoke_client "$family_client_after_restart_log"
  login_smoke_client "$family_client_after_restart_log" "$login_name" "$login_password"
  tmux send-keys -t "$client_session" "$member_cmd" C-m
  sleep 1
  capture_tmux "$client_session" "$family_client_after_restart_log"
  capture_tmux "$server_session" "$family_server_log"
  assert_absent_regex "$family_server_log" '(panic:|fatal error:)' "family scenario server panic after restart"
  assert_absent_regex "$family_client_after_restart_log" '(panic:|fatal error:)' "family scenario client panic after restart"
  assert_absent_regex "$family_client_after_restart_log" "$kick_target" "family scenario kicked member visible after restart"

  {
    echo "# Family Scenario Smoke Report"
    echo "status: kicked"
    echo "root: $data_root"
    echo "disposable root: $family_scenario_root"
    echo "login: $login_name"
    echo "familyID: $family_id"
    echo "family boss: ${family_boss:-unknown}"
    echo "seeded boss flag: $seeded_boss"
    echo "family member command: $member_cmd"
    echo "family kick command: $kick_cmd"
    echo "kicked target: $kick_target"
    echo "kick performed: $kicked"
    echo "before transcript: $family_client_before_log"
    echo "after kick transcript: $family_client_after_kick_log"
    echo "after restart transcript: $family_client_after_restart_log"
    echo "validate log: $log_dir/scenario-family-validate.log"
    echo "generated: $(date -Iseconds)"
  } >"$family_scenario_report"
}

run_class_family_scenario_smoke() {
  require_cmd tmux
  require_cmd jq

  copy_data_root class_family_scenario_root "muhan-class-family-scenario-root" "$log_dir/scenario-class-family-copy.log"
  if [[ -z "$class_family_scenario_root" || "$class_family_scenario_root" == "$data_root" || "$class_family_scenario_root" == "$repo_root" ]]; then
    echo "class-family scenario refuses to run on a non-disposable root: ${class_family_scenario_root:-unset}" >&2
    exit 1
  fi

  local class_server_log="$log_dir/scenario-class-family-server.log"
  local class_before_log="$log_dir/scenario-class-family-before.log"
  local class_after_log="$log_dir/scenario-class-family-after-change.log"
  local class_restart_log="$log_dir/scenario-class-family-after-restart.log"
  local class_cmdlist_log="$log_dir/scenario-class-family-cmdlist.log"
  local class_validate_log="$log_dir/scenario-class-family-validate.log"
  local player_json="$class_family_scenario_root/player/json/$class_family_login.json"
  local family_id=""
  local family_file=""
  local member_cmd=""
  local change_cmd=""
  local skip_reason=""

  run_to_log "$class_cmdlist_log" go run ./cmd/muhan-cmdlist -root "$class_family_scenario_root"
  member_cmd="$(detect_cmd_by_handler "$class_cmdlist_log" "family_member" || true)"
  change_cmd="$(detect_cmd_by_handler "$class_cmdlist_log" "change_class" || true)"

  if [[ -f "$player_json" ]]; then
    family_id="$(json_int_value "$player_json" "familyID" || true)"
  fi
  if [[ -n "$family_id" ]]; then
    family_file="$class_family_scenario_root/player/family/family_member_$family_id"
  fi

  if [[ -z "$member_cmd" ]]; then
    skip_reason="family_member command not found in cmdlist"
  elif [[ -z "$change_cmd" ]]; then
    skip_reason="change_class command not found in cmdlist"
  elif [[ ! -f "$player_json" ]]; then
    skip_reason="class-family player sidecar missing: ${player_json#$class_family_scenario_root/}"
  elif [[ -z "$family_id" || "$family_id" -le 0 ]]; then
    skip_reason="class-family player has no positive familyID"
  elif [[ ! -f "$family_file" ]]; then
    skip_reason="family member file missing: ${family_file#$class_family_scenario_root/}"
  elif ! family_file_has_member "$family_file" "$class_family_login"; then
    skip_reason="class-family player is not listed in ${family_file#$class_family_scenario_root/}"
  elif ! seed_class_family_scenario_player "$player_json" "$class_family_room" "$class_family_source_class" "$class_family_experience"; then
    skip_reason="failed to seed class-family player sidecar"
  fi

  if [[ -n "$skip_reason" ]]; then
    run_to_log "$class_validate_log" go run ./cmd/muhan-server -root "$class_family_scenario_root" -validate -ansi=false
    assert_fixed "$class_validate_log" "runtime world: initialized" "class-family scenario runtime world initialized"
    assert_regex "$class_validate_log" 'findings: [0-9]+ warnings, 0 errors' "class-family scenario validate has zero errors"
    {
      echo "# Class Family Scenario Smoke Report"
      echo "status: skipped"
      echo "reason: $skip_reason"
      echo "root: $data_root"
      echo "disposable root: $class_family_scenario_root"
      echo "login: $class_family_login"
      echo "familyID: ${family_id:-not set}"
      echo "family member command: ${member_cmd:-not found}"
      echo "class change command: ${change_cmd:-not found}"
      echo "validate log: $class_validate_log"
      echo "generated: $(date -Iseconds)"
    } >"$class_family_scenario_report"
    return
  fi

  kill_smoke_sessions
  start_smoke_server "$class_family_scenario_root" "$class_server_log"
  connect_smoke_client "$class_before_log"
  login_smoke_client "$class_before_log" "$class_family_login" "$class_family_password"
  tmux send-keys -t "$client_session" "$member_cmd" C-m
  wait_for_tmux_output "$client_session" "$class_family_login" "$class_before_log" 20
  capture_tmux "$client_session" "$class_before_log"
  capture_tmux "$server_session" "$class_server_log"
  assert_absent_regex "$class_server_log" '(panic:|fatal error:)' "class-family scenario server panic before change"
  assert_absent_regex "$class_before_log" '(panic:|fatal error:)' "class-family scenario client panic before change"

  tmux send-keys -t "$client_session" "$change_cmd" C-m
  wait_for_tmux_output "$client_session" "정말로 직업전환" "$class_after_log" 20
  tmux send-keys -t "$client_session" "예" C-m
  wait_for_tmux_output "$client_session" "당신의 직업이 전환되었습니다" "$class_after_log" 20
  tmux send-keys -t "$client_session" "저장" C-m
  wait_for_tmux_output "$client_session" "저장하였습니다" "$class_after_log" 20
  sleep 1
  capture_tmux "$client_session" "$class_after_log"
  capture_tmux "$server_session" "$class_server_log"
  assert_absent_regex "$class_server_log" '(panic:|fatal error:)' "class-family scenario server panic after change"
  assert_absent_regex "$class_after_log" '(panic:|fatal error:)' "class-family scenario client panic after change"
  assert_fixed "$class_after_log" "당신의 직업이 전환되었습니다" "class-family change output"

  if ! family_file_member_class_is "$family_file" "$class_family_login" "$class_family_target_class"; then
    echo "class-family scenario family_member file did not record class $class_family_target_class for $class_family_login" >&2
    decode_legacy_text_file "$family_file" >&2 || true
    exit 1
  fi
  if ! jq -e --argjson target "$class_family_target_class" '.creature.stats.class == $target' "$player_json" >/dev/null; then
    echo "class-family scenario player sidecar did not persist class $class_family_target_class: $player_json" >&2
    jq '.creature.stats.class' "$player_json" >&2 || true
    exit 1
  fi

  kill_smoke_sessions
  run_to_log "$class_validate_log" go run ./cmd/muhan-server -root "$class_family_scenario_root" -validate -ansi=false
  assert_fixed "$class_validate_log" "runtime world: initialized" "class-family scenario runtime world initialized"
  assert_regex "$class_validate_log" 'findings: [0-9]+ warnings, 0 errors' "class-family scenario validate has zero errors"

  start_smoke_server "$class_family_scenario_root" "$class_server_log"
  connect_smoke_client "$class_restart_log"
  login_smoke_client "$class_restart_log" "$class_family_login" "$class_family_password"
  tmux send-keys -t "$client_session" "$member_cmd" C-m
  wait_for_tmux_output "$client_session" "$class_family_login" "$class_restart_log" 20
  sleep 1
  capture_tmux "$client_session" "$class_restart_log"
  capture_tmux "$server_session" "$class_server_log"
  assert_absent_regex "$class_server_log" '(panic:|fatal error:)' "class-family scenario server panic after restart"
  assert_absent_regex "$class_restart_log" '(panic:|fatal error:)' "class-family scenario client panic after restart"
  assert_regex "$class_restart_log" '\[도술[[:space:]]*\][[:space:]]+소녀무사' "class-family restarted family_member class"

  {
    echo "# Class Family Scenario Smoke Report"
    echo "status: class-changed"
    echo "root: $data_root"
    echo "disposable root: $class_family_scenario_root"
    echo "login: $class_family_login"
    echo "familyID: $family_id"
    echo "seeded room: $class_family_room"
    echo "source class: $class_family_source_class"
    echo "target class: $class_family_target_class"
    echo "seeded experience: $class_family_experience"
    echo "family member command: $member_cmd"
    echo "class change command: $change_cmd"
    echo "before transcript: $class_before_log"
    echo "after change transcript: $class_after_log"
    echo "after restart transcript: $class_restart_log"
    echo "validate log: $class_validate_log"
    echo "generated: $(date -Iseconds)"
  } >"$class_family_scenario_report"
}

run_permdeath_scenario_smoke() {
  require_cmd tmux
  require_cmd jq
  copy_data_root permdeath_scenario_root "muhan-permdeath-scenario-root" "$log_dir/scenario-permdeath-copy.log"
  if [[ -z "$permdeath_scenario_root" || "$permdeath_scenario_root" == "$data_root" || "$permdeath_scenario_root" == "$repo_root" ]]; then
    echo "permanent death scenario refuses to run on a non-disposable root: ${permdeath_scenario_root:-unset}" >&2
    exit 1
  fi

  local perm_server_log="$log_dir/scenario-permdeath-server.log"
  local perm_client_log="$log_dir/scenario-permdeath.log"
  local perm_validate_log="$log_dir/scenario-permdeath-validate.log"
  local perm_cmdlist_log="$log_dir/scenario-permdeath-cmdlist.log"
  local player_json="$permdeath_scenario_root/player/json/$permdeath_login.json"
  local room_number="${permdeath_room#room:}"
  local room_sidecar="$permdeath_scenario_root/rooms/json/r${room_number}.json"
  local monster_cmd
  local save_cmd
  local attack_cmd="때려"

  run_to_log "$perm_cmdlist_log" go run ./cmd/muhan-cmdlist -root "$permdeath_scenario_root"
  monster_cmd="$(detect_cmd_by_handler "$perm_cmdlist_log" "dm_create_crt" || true)"
  save_cmd="$(detect_cmd_by_handler "$perm_cmdlist_log" "dm_resave" || true)"
  if [[ -z "$monster_cmd" ]]; then
    echo "permanent death scenario dm_create_crt command not found in cmdlist" >&2
    exit 1
  fi
  if [[ -z "$save_cmd" ]]; then
    echo "permanent death scenario dm_resave command not found in cmdlist" >&2
    exit 1
  fi
  if [[ ! -f "$player_json" ]]; then
    echo "permanent death scenario player sidecar missing: ${player_json#$permdeath_scenario_root/}" >&2
    exit 1
  fi
  seed_permdeath_scenario_player "$player_json" "$permdeath_room"

  kill_smoke_sessions
  start_smoke_server "$permdeath_scenario_root" "$perm_server_log"
  connect_smoke_client "$perm_client_log"
  login_smoke_client "$perm_client_log" "$permdeath_login" "$permdeath_password"

  tmux send-keys -t "$client_session" -l "봐"
  tmux send-keys -t "$client_session" C-m
  wait_for_tmux_output "$client_session" "이상한 방" "$perm_client_log" 20

  tmux send-keys -t "$client_session" -l "$monster_cmd $permdeath_monster_number"
  tmux send-keys -t "$client_session" C-m
  sleep 1
  tmux send-keys -t "$client_session" -l "봐"
  tmux send-keys -t "$client_session" C-m
  wait_for_tmux_output "$client_session" "$permdeath_monster_name" "$perm_client_log" 20

  tmux send-keys -t "$client_session" -l "$attack_cmd $permdeath_attack_target"
  tmux send-keys -t "$client_session" C-m
  wait_for_tmux_output "$client_session" "내가 죽었다고" "$perm_client_log" 30
  wait_for_tmux_output "$client_session" "무엇인가 환영" "$perm_client_log" 30
  wait_for_tmux_output "$client_session" "타타르의 머리가 쓰러졌습니다." "$perm_client_log" 30
  for _ in 1 2 3 4 5 6 7 8; do
    capture_tmux "$client_session" "$perm_client_log"
    if grep -Fq "$permdeath_final_monster_name" "$perm_client_log" &&
      grep -Fq "축하합니다. 당신은 임무를 달성하였습니다." "$perm_client_log"; then
      break
    fi
    tmux send-keys -t "$client_session" -l "$attack_cmd $permdeath_attack_target"
    tmux send-keys -t "$client_session" C-m
    sleep 2
  done
  wait_for_tmux_output "$client_session" "$permdeath_final_monster_name" "$perm_client_log" 90
  wait_for_tmux_output "$client_session" "축하합니다. 당신은 임무를 달성하였습니다." "$perm_client_log" 90

  tmux send-keys -t "$client_session" -l "$save_cmd"
  tmux send-keys -t "$client_session" C-m
  wait_for_tmux_output "$client_session" "Ok." "$perm_client_log" 20
  sleep 1
  capture_tmux "$client_session" "$perm_client_log"
  capture_tmux "$server_session" "$perm_server_log"
  assert_absent_regex "$perm_server_log" '(panic:|fatal error:)' "permanent death scenario server panic"
  assert_absent_regex "$perm_client_log" '(panic:|fatal error:)' "permanent death scenario client panic"
  assert_fixed "$perm_client_log" "내가 죽었다고" "permanent death MDEATH text"
  assert_fixed "$perm_client_log" "무엇인가 환영" "permanent death MSUMMO text"
  assert_fixed "$perm_client_log" "축하합니다. 당신은 임무를 달성하였습니다." "permanent death quest reward"
  assert_fixed "$perm_client_log" "$permdeath_final_monster_name" "permanent death summon chain"

  if [[ ! -f "$room_sidecar" ]]; then
    echo "permanent death scenario room sidecar missing: ${room_sidecar#$permdeath_scenario_root/}" >&2
    exit 1
  fi
  if ! jq -e --arg misc "$permdeath_monster_number" '
      (.properties["perm_mon.0.misc"] == $misc)
      and ((.properties["perm_mon.0.ltime"] | tonumber) > 0)
      and (.properties["perm_mon.0.interval"] == "720")
    ' "$room_sidecar" >/dev/null; then
    echo "permanent death scenario room sidecar did not persist perm_mon ltime/misc/interval" >&2
    jq '.properties' "$room_sidecar" >&2 || true
    exit 1
  fi

  kill_smoke_sessions
  run_to_log "$perm_validate_log" go run ./cmd/muhan-server -root "$permdeath_scenario_root" -validate -ansi=false
  assert_fixed "$perm_validate_log" "runtime world: initialized" "permanent death scenario runtime world initialized"
  assert_regex "$perm_validate_log" 'findings: [0-9]+ warnings, 0 errors' "permanent death scenario validate has zero errors"

  {
    echo "# Permanent Monster Death Scenario Smoke Report"
    echo "status: permanent-death"
    echo "root: $data_root"
    echo "disposable root: $permdeath_scenario_root"
    echo "login: $permdeath_login"
    echo "room: $permdeath_room"
    echo "monster number: $permdeath_monster_number"
    echo "monster command: $monster_cmd"
    echo "attack command: $attack_cmd $permdeath_attack_target"
    echo "save command: $save_cmd"
    echo "room sidecar: $room_sidecar"
    echo "perm_mon.0.ltime: $(jq -r '.properties["perm_mon.0.ltime"] // ""' "$room_sidecar")"
    echo "transcript: $perm_client_log"
    echo "validate log: $perm_validate_log"
    echo "generated: $(date -Iseconds)"
  } >"$permdeath_scenario_report"
}

run_trap_scenario_smoke() {
  require_cmd tmux
  require_cmd jq

  copy_data_root trap_scenario_root "muhan-trap-scenario-root" "$log_dir/scenario-trap-copy.log"
  if [[ -z "$trap_scenario_root" || "$trap_scenario_root" == "$data_root" || "$trap_scenario_root" == "$repo_root" ]]; then
    echo "trap scenario refuses to run on a non-disposable root: ${trap_scenario_root:-unset}" >&2
    exit 1
  fi

  local trap_server_log="$log_dir/scenario-trap-server.log"
  local trap_client_log="$log_dir/scenario-trap-pit-death.log"
  local trap_validate_log="$log_dir/scenario-trap-validate.log"
  local alarm_server_log="$log_dir/scenario-trap-alarm-server.log"
  local alarm_client_log="$log_dir/scenario-trap-alarm.log"
  local alarm_validate_log="$log_dir/scenario-trap-alarm-validate.log"
  local player_json="$trap_scenario_root/player/json/$login_name.json"
  local trap_start_room="room:00159"
  local trap_room="room:00165"
  local trap_exit_room="room:00149"
  local trap_command="동"
  local alarm_start_room="room:00226"
  local alarm_room="room:00227"
  local alarm_source_room="room:00231"
  local alarm_command="동"
  local alarm_guard_marker="건달"
  local seeded_hp=1
  local seeded_hp_max=20
  local seeded_dexterity=0
  local alarm_hp=3500
  local alarm_hp_max=3500

  assert_file_exists "$player_json" "trap scenario login player sidecar"
  seed_trap_scenario_player "$player_json" "$trap_start_room" "$seeded_hp" "$seeded_hp_max" "$seeded_dexterity"

  kill_smoke_sessions
  start_smoke_server "$trap_scenario_root" "$trap_server_log"
  connect_smoke_client "$trap_client_log"
  login_smoke_client "$trap_client_log" "$login_name" "$login_password"

  tmux send-keys -t "$client_session" -l "$trap_command"
  tmux send-keys -t "$client_session" C-m
  wait_for_tmux_output "$client_session" "당신은 구덩이에 빠졌습니다!" "$trap_client_log" 20
  wait_for_tmux_output "$client_session" "당신은 죽으면서 몇가지 물건을 떨어뜨렸습니다." "$trap_client_log" 20
  sleep 1
  capture_tmux "$client_session" "$trap_client_log"
  capture_tmux "$server_session" "$trap_server_log"

  assert_fixed "$trap_client_log" "당신은 구덩이에 빠졌습니다!" "trap pit message"
  assert_regex "$trap_client_log" '당신은 [0-9]+점의 피해를 입었습니다\.' "trap damage message"
  assert_fixed "$trap_client_log" "당신은 죽으면서 몇가지 물건을 떨어뜨렸습니다." "trap death output"
  assert_absent_regex "$trap_server_log" '(panic:|fatal error:)' "trap scenario server panic"
  assert_absent_regex "$trap_client_log" '(panic:|fatal error:)' "trap scenario client panic"

  kill_smoke_sessions
  run_to_log "$trap_validate_log" go run ./cmd/muhan-server -root "$trap_scenario_root" -validate -ansi=false
  assert_fixed "$trap_validate_log" "runtime world: initialized" "trap scenario runtime world initialized"
  assert_regex "$trap_validate_log" 'findings: [0-9]+ warnings, 0 errors' "trap scenario validate has zero errors"

  seed_trap_scenario_player "$player_json" "$alarm_start_room" "$alarm_hp" "$alarm_hp_max" "$seeded_dexterity"
  start_smoke_server "$trap_scenario_root" "$alarm_server_log"
  connect_smoke_client "$alarm_client_log"
  login_smoke_client "$alarm_client_log" "$login_name" "$login_password"

  tmux send-keys -t "$client_session" -l "$alarm_command"
  tmux send-keys -t "$client_session" C-m
  wait_for_tmux_output "$client_session" "경보장치가 울립니다!" "$alarm_client_log" 20
  wait_for_tmux_output "$client_session" "근처에 경비원들이 없길 바랍니다." "$alarm_client_log" 20
  tmux send-keys -t "$client_session" -l "봐"
  tmux send-keys -t "$client_session" C-m
  wait_for_tmux_output "$client_session" "$alarm_guard_marker" "$alarm_client_log" 20
  sleep 1
  capture_tmux "$client_session" "$alarm_client_log"
  capture_tmux "$server_session" "$alarm_server_log"

  assert_fixed "$alarm_client_log" "경보장치가 울립니다!" "trap alarm message"
  assert_fixed "$alarm_client_log" "근처에 경비원들이 없길 바랍니다." "trap alarm secondary message"
  assert_fixed "$alarm_client_log" "$alarm_guard_marker" "trap alarm guard visibility"
  assert_absent_regex "$alarm_server_log" '(panic:|fatal error:)' "trap alarm server panic"
  assert_absent_regex "$alarm_client_log" '(panic:|fatal error:)' "trap alarm client panic"

  kill_smoke_sessions
  run_to_log "$alarm_validate_log" go run ./cmd/muhan-server -root "$trap_scenario_root" -validate -ansi=false
  assert_fixed "$alarm_validate_log" "runtime world: initialized" "trap alarm runtime world initialized"
  assert_regex "$alarm_validate_log" 'findings: [0-9]+ warnings, 0 errors' "trap alarm validate has zero errors"

  {
    echo "# Trap Scenario Smoke Report"
    echo "status: pit-death+alarm"
    echo "root: $data_root"
    echo "disposable root: $trap_scenario_root"
    echo "login: $login_name"
    echo "seeded room: $trap_start_room"
    echo "seeded hpCurrent: $seeded_hp"
    echo "seeded hpMax: $seeded_hp_max"
    echo "seeded dexterity: $seeded_dexterity"
    echo "route: $trap_start_room $trap_command -> $trap_room trap=1 trapExit=$trap_exit_room"
    echo "pit transcript: $trap_client_log"
    echo "pit server log: $trap_server_log"
    echo "pit validate log: $trap_validate_log"
    echo "alarm seeded room: $alarm_start_room"
    echo "alarm seeded hpCurrent: $alarm_hp"
    echo "alarm seeded hpMax: $alarm_hp_max"
    echo "alarm route: $alarm_start_room $alarm_command -> $alarm_room trap=7 trapExit=$alarm_source_room"
    echo "alarm guard marker: $alarm_guard_marker"
    echo "alarm transcript: $alarm_client_log"
    echo "alarm server log: $alarm_server_log"
    echo "alarm validate log: $alarm_validate_log"
    echo "generated: $(date -Iseconds)"
  } >"$trap_scenario_report"
}

run_talk_give_scenario_smoke() {
  require_cmd tmux
  require_cmd jq

  copy_data_root talk_give_scenario_root "muhan-talk-give-scenario-root" "$log_dir/scenario-talk-give-copy.log"
  if [[ -z "$talk_give_scenario_root" || "$talk_give_scenario_root" == "$data_root" || "$talk_give_scenario_root" == "$repo_root" ]]; then
    echo "talk GIVE scenario refuses to run on a non-disposable root: ${talk_give_scenario_root:-unset}" >&2
    exit 1
  fi

  local talk_server_log="$log_dir/scenario-talk-give-server.log"
  local talk_client_log="$log_dir/scenario-talk-give.log"
  local talk_validate_log="$log_dir/scenario-talk-give-validate.log"
  local player_json="$talk_give_scenario_root/player/json/$login_name.json"
  local talk_room="room:03208"
  local talk_command="불의 용사 대화"
  local talk_reward="불의 감옥 열쇠"
  local talk_npc="불의 공주"
  local talk_topic="용사"

  assert_file_exists "$player_json" "talk GIVE scenario login player sidecar"
  seed_talk_give_scenario_player "$player_json" "$talk_room"

  kill_smoke_sessions
  start_smoke_server "$talk_give_scenario_root" "$talk_server_log"
  connect_smoke_client "$talk_client_log"
  login_smoke_client "$talk_client_log" "$login_name" "$login_password"

  for cmd in "봐" "소지"; do
    tmux send-keys -t "$client_session" -l "$cmd"
    tmux send-keys -t "$client_session" C-m
    sleep 0.5
  done
  wait_for_tmux_output "$client_session" "공주의 방" "$talk_client_log" 20
  wait_for_tmux_output "$client_session" "소지품:" "$talk_client_log" 20

  tmux send-keys -t "$client_session" -l "$talk_command"
  tmux send-keys -t "$client_session" C-m
  wait_for_tmux_output "$client_session" "$talk_npc" "$talk_client_log" 20
  wait_for_tmux_output "$client_session" "$talk_reward를 줍니다." "$talk_client_log" 20

  tmux send-keys -t "$client_session" -l "소지"
  tmux send-keys -t "$client_session" C-m
  wait_for_tmux_output "$client_session" "$talk_reward" "$talk_client_log" 20
  sleep 1
  capture_tmux "$client_session" "$talk_client_log"
  capture_tmux "$server_session" "$talk_server_log"

  assert_fixed "$talk_client_log" "공주의 방" "talk GIVE room render"
  assert_fixed "$talk_client_log" "$talk_npc" "talk GIVE NPC response"
  assert_fixed "$talk_client_log" "$talk_reward를 줍니다." "talk GIVE reward output"
  assert_fixed "$talk_client_log" "$talk_reward" "talk GIVE reward in inventory"
  assert_absent_regex "$talk_server_log" '(panic:|fatal error:)' "talk GIVE server panic"
  assert_absent_regex "$talk_client_log" '(panic:|fatal error:)' "talk GIVE client panic"

  kill_smoke_sessions
  run_to_log "$talk_validate_log" go run ./cmd/muhan-server -root "$talk_give_scenario_root" -validate -ansi=false
  assert_fixed "$talk_validate_log" "runtime world: initialized" "talk GIVE runtime world initialized"
  assert_regex "$talk_validate_log" 'findings: [0-9]+ warnings, 0 errors' "talk GIVE validate has zero errors"

  {
    echo "# Talk GIVE Scenario Smoke Report"
    echo "status: give"
    echo "root: $data_root"
    echo "disposable root: $talk_give_scenario_root"
    echo "login: $login_name"
    echo "seeded room: $talk_room"
    echo "npc: $talk_npc"
    echo "topic: $talk_topic"
    echo "command: $talk_command"
    echo "reward prototype: object:o01:4"
    echo "reward: $talk_reward"
    echo "transcript: $talk_client_log"
    echo "server log: $talk_server_log"
    echo "validate log: $talk_validate_log"
    echo "generated: $(date -Iseconds)"
  } >"$talk_give_scenario_report"
}

run_talk_effects_step() {
  local player_json="$1"
  local room_id="$2"
  local hp_current="$3"
  local hp_max="$4"
  local server_log="$5"
  local client_log="$6"
  local command="$7"
  local marker_one="$8"
  local marker_two="$9"
  local label="${10}"

  seed_talk_effects_scenario_player "$player_json" "$room_id" "$hp_current" "$hp_max"
  kill_smoke_sessions
  start_smoke_server "$talk_effects_scenario_root" "$server_log"
  connect_smoke_client "$client_log"
  login_smoke_client "$client_log" "$login_name" "$login_password"

  tmux send-keys -t "$client_session" -l "$command"
  tmux send-keys -t "$client_session" C-m
  wait_for_tmux_output "$client_session" "$marker_one" "$client_log" 20
  wait_for_tmux_output "$client_session" "$marker_two" "$client_log" 20
  sleep 1
  capture_tmux "$client_session" "$client_log"
  capture_tmux "$server_session" "$server_log"

  assert_fixed "$client_log" "$marker_one" "$label marker one"
  assert_fixed "$client_log" "$marker_two" "$label marker two"
  assert_absent_regex "$server_log" '(panic:|fatal error:)' "$label server panic"
  assert_absent_regex "$client_log" '(panic:|fatal error:)' "$label client panic"
  kill_smoke_sessions
}

run_talk_effects_scenario_smoke() {
  require_cmd tmux
  require_cmd jq

  copy_data_root talk_effects_scenario_root "muhan-talk-effects-scenario-root" "$log_dir/scenario-talk-effects-copy.log"
  if [[ -z "$talk_effects_scenario_root" || "$talk_effects_scenario_root" == "$data_root" || "$talk_effects_scenario_root" == "$repo_root" ]]; then
    echo "talk effects scenario refuses to run on a non-disposable root: ${talk_effects_scenario_root:-unset}" >&2
    exit 1
  fi

  local player_json="$talk_effects_scenario_root/player/json/$login_name.json"
  local validate_log="$log_dir/scenario-talk-effects-validate.log"
  local action_server_log="$log_dir/scenario-talk-effects-action-server.log"
  local action_client_log="$log_dir/scenario-talk-effects-action.log"
  local cast_server_log="$log_dir/scenario-talk-effects-cast-server.log"
  local cast_client_log="$log_dir/scenario-talk-effects-cast.log"
  local attack_server_log="$log_dir/scenario-talk-effects-attack-server.log"
  local attack_client_log="$log_dir/scenario-talk-effects-attack.log"
  local action_room="room:01075"
  local cast_room="room:01008"
  local attack_room="room:00542"
  local action_command="아가씨 사랑 대화"
  local cast_command="아미타불 치료 대화"
  local attack_command="떠돌이 싸우자 대화"

  assert_file_exists "$player_json" "talk effects scenario login player sidecar"

  run_talk_effects_step "$player_json" "$action_room" 3500 3500 "$action_server_log" "$action_client_log" "$action_command" "저도 사랑해요" "뽀뽀를 합니다" "talk ACTION"
  run_talk_effects_step "$player_json" "$cast_room" 20 3500 "$cast_server_log" "$cast_client_log" "$cast_command" "어때 많이 좋아졌는가" "회복 주문을 겁니다" "talk CAST"
  run_talk_effects_step "$player_json" "$attack_room" 3500 3500 "$attack_server_log" "$attack_client_log" "$attack_command" "떠돌이 검객" "당신을 공격합니다" "talk ATTACK"

  run_to_log "$validate_log" go run ./cmd/muhan-server -root "$talk_effects_scenario_root" -validate -ansi=false
  assert_fixed "$validate_log" "runtime world: initialized" "talk effects runtime world initialized"
  assert_regex "$validate_log" 'findings: [0-9]+ warnings, 0 errors' "talk effects validate has zero errors"

  {
    echo "# Talk Effects Scenario Smoke Report"
    echo "status: action+cast+attack"
    echo "root: $data_root"
    echo "disposable root: $talk_effects_scenario_root"
    echo "login: $login_name"
    echo "ACTION room: $action_room"
    echo "ACTION command: $action_command"
    echo "ACTION transcript: $action_client_log"
    echo "ACTION server log: $action_server_log"
    echo "CAST room: $cast_room"
    echo "CAST command: $cast_command"
    echo "CAST transcript: $cast_client_log"
    echo "CAST server log: $cast_server_log"
    echo "ATTACK room: $attack_room"
    echo "ATTACK command: $attack_command"
    echo "ATTACK transcript: $attack_client_log"
    echo "ATTACK server log: $attack_server_log"
    echo "validate log: $validate_log"
    echo "generated: $(date -Iseconds)"
  } >"$talk_effects_scenario_report"
}

run_objects_scenario_smoke() {
  require_cmd tmux
  require_cmd jq

  copy_data_root objects_scenario_root "muhan-objects-scenario-root" "$log_dir/scenario-objects-copy.log"
  if [[ -z "$objects_scenario_root" || "$objects_scenario_root" == "$data_root" || "$objects_scenario_root" == "$repo_root" ]]; then
    echo "objects scenario refuses to run on a non-disposable root: ${objects_scenario_root:-unset}" >&2
    exit 1
  fi

  local objects_server_log="$log_dir/scenario-objects-server.log"
  local objects_client_log="$log_dir/scenario-objects.log"
  local objects_restart_server_log="$log_dir/scenario-objects-restart-server.log"
  local objects_restart_client_log="$log_dir/scenario-objects-restart.log"
  local objects_validate_log="$log_dir/scenario-objects-validate.log"
  local player_json="$objects_scenario_root/player/json/$login_name.json"
  local nested_source_json="$objects_scenario_root/player/json/까마귀.json"
  local room_save="$objects_scenario_root/rooms/json/r01001.json"
  local start_room="room:01001"
  local orench_create_command="*create 212"
  local orench_proto="object:o02:12"
  local orench_name="자객검"
  local nested_bag_id="object:scenario:nested-bag"
  local nested_sword_id="object:scenario:nested-sword"
  local nested_bag_name="작은 보따리"
  local nested_sword_name="솔개 검"

  assert_file_exists "$player_json" "objects scenario login player sidecar"
  assert_file_exists "$nested_source_json" "objects scenario nested source sidecar"
  seed_objects_scenario_player "$player_json" "$start_room" "$nested_source_json" "$nested_bag_id" "$nested_sword_id"

  kill_smoke_sessions
  start_smoke_server "$objects_scenario_root" "$objects_server_log"
  connect_smoke_client "$objects_client_log"
  login_smoke_client "$objects_client_log" "$login_name" "$login_password"

  for cmd in "소지" "$nested_bag_name 봐"; do
    tmux send-keys -t "$client_session" -l "$cmd"
    tmux send-keys -t "$client_session" C-m
    sleep 0.5
  done
  wait_for_tmux_output "$client_session" "$nested_bag_name" "$objects_client_log" 20
  wait_for_tmux_output "$client_session" "내용물: $nested_sword_name" "$objects_client_log" 20

  tmux send-keys -t "$client_session" -l "$orench_create_command"
  tmux send-keys -t "$client_session" C-m
  wait_for_tmux_output "$client_session" "$orench_name를 소지품에 추가했습니다." "$objects_client_log" 20

  for cmd in "$orench_name 감정" "소지" "저장"; do
    tmux send-keys -t "$client_session" -l "$cmd"
    tmux send-keys -t "$client_session" C-m
    sleep 0.5
  done
  wait_for_tmux_output "$client_session" "이름: $orench_name" "$objects_client_log" 20
  wait_for_tmux_output "$client_session" "$orench_name" "$objects_client_log" 20
  wait_for_tmux_output "$client_session" "저장하였습니다." "$objects_client_log" 20
  sleep 1

  if ! jq -e --arg proto "$orench_proto" '
      .objects[]? | select(.prototypeId == $proto)
    ' "$player_json" >"$log_dir/scenario-objects-orench-created.json"; then
    echo "objects scenario did not persist DM-created prototype $orench_proto in $player_json" >&2
    exit 1
  fi

  tmux send-keys -t "$client_session" -l "$nested_bag_name 버려"
  tmux send-keys -t "$client_session" C-m
  wait_for_tmux_output "$client_session" "버렸습니다." "$objects_client_log" 20
  for cmd in "봐" "$nested_bag_name 봐" "저장" "*save"; do
    tmux send-keys -t "$client_session" -l "$cmd"
    tmux send-keys -t "$client_session" C-m
    sleep 0.5
  done
  wait_for_tmux_output "$client_session" "$nested_bag_name" "$objects_client_log" 20
  wait_for_tmux_output "$client_session" "내용물: $nested_sword_name" "$objects_client_log" 20
  wait_for_tmux_output "$client_session" "저장하였습니다." "$objects_client_log" 20
  wait_for_tmux_output "$client_session" "Ok." "$objects_client_log" 20
  sleep 1
  capture_tmux "$client_session" "$objects_client_log"
  capture_tmux "$server_session" "$objects_server_log"

  assert_fixed "$objects_client_log" "$orench_name를 소지품에 추가했습니다." "objects scenario ORENCH create output"
  assert_fixed "$objects_client_log" "이름: $orench_name" "objects scenario ORENCH appraisal"
  assert_fixed "$objects_client_log" "내용물: $nested_sword_name" "objects scenario nested container contents"
  assert_fixed "$objects_client_log" "Ok." "objects scenario room resave"
  assert_absent_regex "$objects_server_log" '(panic:|fatal error:)' "objects scenario server panic"
  assert_absent_regex "$objects_client_log" '(panic:|fatal error:)' "objects scenario client panic"

  assert_file_exists "$room_save" "objects scenario room save"
  if ! jq -e \
    --arg room "$start_room" \
    --arg bag "$nested_bag_id" \
    --arg sword "$nested_sword_id" '
      (any(.objects[]?; .id == $bag and .location.roomId == $room and ((.contents.objectIds // []) | index($sword) != null)))
      and
      (any(.objects[]?; .id == $sword and .location.containerId == $bag))
    ' "$room_save" >"$log_dir/scenario-objects-room-save-check.json"; then
    echo "objects scenario did not persist nested dropped bag/tree in $room_save" >&2
    exit 1
  fi

  kill_smoke_sessions
  run_to_log "$objects_validate_log" go run ./cmd/muhan-server -root "$objects_scenario_root" -validate -ansi=false
  assert_fixed "$objects_validate_log" "runtime world: initialized" "objects scenario runtime world initialized"
  assert_regex "$objects_validate_log" 'findings: [0-9]+ warnings, 0 errors' "objects scenario validate has zero errors"

  start_smoke_server "$objects_scenario_root" "$objects_restart_server_log"
  connect_smoke_client "$objects_restart_client_log"
  login_smoke_client "$objects_restart_client_log" "$login_name" "$login_password"
  for cmd in "봐" "$nested_bag_name 봐" "소지"; do
    tmux send-keys -t "$client_session" -l "$cmd"
    tmux send-keys -t "$client_session" C-m
    sleep 0.5
  done
  wait_for_tmux_output "$client_session" "$nested_bag_name" "$objects_restart_client_log" 20
  wait_for_tmux_output "$client_session" "내용물: $nested_sword_name" "$objects_restart_client_log" 20
  wait_for_tmux_output "$client_session" "$orench_name" "$objects_restart_client_log" 20
  sleep 1
  capture_tmux "$client_session" "$objects_restart_client_log"
  capture_tmux "$server_session" "$objects_restart_server_log"
  assert_absent_regex "$objects_restart_server_log" '(panic:|fatal error:)' "objects scenario restart server panic"
  assert_absent_regex "$objects_restart_client_log" '(panic:|fatal error:)' "objects scenario restart client panic"

  {
    echo "# Object Materialization Scenario Smoke Report"
    echo "status: orench+nested-restart"
    echo "root: $data_root"
    echo "disposable root: $objects_scenario_root"
    echo "login: $login_name"
    echo "seeded room: $start_room"
    echo "seeded class: 13"
    echo "ORENCH command: $orench_create_command"
    echo "ORENCH prototype: $orench_proto"
    echo "ORENCH object: $orench_name"
    echo "nested source player: 까마귀"
    echo "nested bag: $nested_bag_name ($nested_bag_id)"
    echo "nested child: $nested_sword_name ($nested_sword_id)"
    echo "transcript: $objects_client_log"
    echo "restart transcript: $objects_restart_client_log"
    echo "server log: $objects_server_log"
    echo "restart server log: $objects_restart_server_log"
    echo "validate log: $objects_validate_log"
    echo "ORENCH persisted JSON: $log_dir/scenario-objects-orench-created.json"
    echo "room save check JSON: $log_dir/scenario-objects-room-save-check.json"
    echo "generated: $(date -Iseconds)"
  } >"$objects_scenario_report"
}

on_exit() {
  status=$?
  capture_exit_tmux_artifacts || true
  write_gap_report || true
  if ((status != 0)); then
    log "porting smoke: failed (exit $status); artifacts: $log_dir" || true
  fi
  cleanup
  exit "$status"
}
trap on_exit EXIT

cd "$repo_root"
require_cmd go

log "artifacts: $log_dir"
log "root: $data_root"

if ((run_preflight)); then
  if ((skip_go_test)); then
    log "skip: go test ./... -count=1"
  else
    run_to_log "$log_dir/go-test.log" go test ./... -count=1
  fi
  run_to_log "$log_dir/go-build-server.log" go build -o "$log_dir/muhan-server-check" ./cmd/muhan-server
  run_to_log "$log_dir/validate.log" go run ./cmd/muhan-server -root "$data_root" -validate -ansi=false
  assert_fixed "$log_dir/validate.log" "runtime world: initialized" "runtime world initialized"
  assert_regex "$log_dir/validate.log" 'findings: [0-9]+ warnings, 0 errors' "validate has zero errors"
  run_to_log "$log_dir/cmdlist.log" go run ./cmd/muhan-cmdlist -root "$data_root"
  assert_fixed "$log_dir/cmdlist.log" "command entries" "legacy command list"
fi

if ((run_sidecar)); then
  run_to_log "$log_dir/sidecarmigrate.json" go run ./cmd/muhan-sidecarmigrate -root "$data_root" -json
fi

if ((run_runtime)); then
  require_cmd tmux
  runtime_root="$data_root"
  if ((copy_root)); then
    copy_data_root runtime_root "muhan-runtime-root" "$log_dir/copy.log"
  fi

  kill_smoke_sessions
  server_log="$log_dir/server.log"
  client_log="$log_dir/client.log"
  second_client_log="$log_dir/client-second.log"
  ansi_client_log="$log_dir/client-ansi-raw.log"
  start_smoke_server "$runtime_root" "$server_log"
  connect_smoke_client "$client_log"
  login_smoke_client "$client_log" "$login_name" "$login_password"
  connect_smoke_client "$second_client_log" "$second_client_session"
  login_smoke_client "$second_client_log" "$second_login_name" "$second_login_password" "$second_client_session"

  say_broadcast_suffix='"멀티스모크"라고 말합니다.'
  tmux send-keys -t "$client_session" "멀티스모크 말" C-m
  wait_for_tmux_output "$second_client_session" "$say_broadcast_suffix" "$second_client_log" 20

  for cmd in "봐" "정보" "" "점수" "장비" "소지" "게시판" "계석치무 봐" "계석치무 임무 자세히 대화" "밑" "누구"; do
    if [[ -n "$cmd" ]]; then
      tmux send-keys -t "$client_session" "$cmd" C-m
    else
      tmux send-keys -t "$client_session" C-m
    fi
    sleep 0.3
  done
  tmux send-keys -t "$second_client_session" "누구" C-m
  wait_for_tmux_output "$second_client_session" "$login_name" "$second_client_log" 20
  wait_for_tmux_output "$second_client_session" "$second_login_name" "$second_client_log" 20
  wait_for_tmux_output "$client_session" "문이 닫혀 있습니다" "$client_log" 20
  capture_tmux "$server_session" "$server_log"
  capture_tmux "$client_session" "$client_log"
  capture_tmux "$second_client_session" "$second_client_log"
  if [[ "$ansi" == "true" ]]; then
    capture_tmux_raw "$client_session" "$ansi_client_log"
  fi

  assert_fixed "$client_log" "통계 무한 광장" "square room rendering"
  assert_fixed "$client_log" "생명의 나무" "tree description rendering"
  assert_fixed "$client_log" "접속자" "who output"
  assert_fixed "$client_log" "$login_name" "login player appears in output"
  assert_fixed "$second_client_log" "$say_broadcast_suffix" "multi-client same-room say broadcast"
  assert_fixed "$second_client_log" "$second_login_name" "second login player appears in who output"
  if [[ "$ansi" == "true" ]]; then
    assert_ansi_escape "$ansi_client_log" "runtime client ANSI transcript"
  fi
  assert_absent_regex "$server_log" '(panic:|fatal error:)' "server panic"
  assert_absent_regex "$client_log" '(panic:|fatal error:)' "client panic"
  assert_absent_regex "$second_client_log" '(panic:|fatal error:)' "second client panic"
fi

if ((run_scenario)); then
  run_suicide_scenario_smoke
fi

if ((run_scenario_family)); then
  run_family_scenario_smoke
fi

if ((run_scenario_class_family)); then
  run_class_family_scenario_smoke
fi

if ((run_scenario_permdeath)); then
  run_permdeath_scenario_smoke
fi

if ((run_scenario_trap)); then
  run_trap_scenario_smoke
fi

if ((run_scenario_talk_give)); then
  run_talk_give_scenario_smoke
fi

if ((run_scenario_talk_effects)); then
  run_talk_effects_scenario_smoke
fi

if ((run_scenario_objects)); then
  run_objects_scenario_smoke
fi

if ((run_live_gap_report)); then
  write_live_gap_report
fi

write_gap_report
log "gap report: $gap_report"
if ((run_live_gap_report)); then
  log "live gap report: $live_gap_report"
fi
log "porting smoke: ok"
