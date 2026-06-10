#!/usr/bin/env bash
# =============================================================================
#  Muhan MUD - 자동 백업 스크립트
#
#  게임 데이터 디렉토리를 tar.gz로 압축 백업합니다.
#  오래된 백업은 자동으로 삭제됩니다.
#
#  사용법:
#    ./scripts/backup.sh
#
#  환경변수:
#    DATA_ROOT    - 게임 데이터 루트 디렉토리   (기본값: /data)
#    BACKUP_DIR   - 백업 파일 저장 디렉토리      (기본값: /data/backups)
#    BACKUP_KEEP  - 최대 백업 보관 개수          (기본값: 7)
#
#  크론 예시 (매일 새벽 4시):
#    0 4 * * * /opt/muhan/scripts/backup.sh >> /var/log/muhan-backup.log 2>&1
#
#  Docker 사이드카 예시:
#    docker compose --profile backup up -d backup
# =============================================================================

set -euo pipefail

# ─────────────────────────────────────────────────────────────
# 설정
# ─────────────────────────────────────────────────────────────
DATA_ROOT="${DATA_ROOT:-/data}"
BACKUP_DIR="${BACKUP_DIR:-/data/backups}"
BACKUP_KEEP="${BACKUP_KEEP:-7}"

# 백업 대상 디렉토리 목록
BACKUP_TARGETS=(
  "player"
  "rooms"
  "board"
  "room"
  "post"
  "objmon"
  "help"
)

TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
BACKUP_FILE="muhan_backup_${TIMESTAMP}.tar.gz"

# ─────────────────────────────────────────────────────────────
# 유틸리티
# ─────────────────────────────────────────────────────────────
log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

die() {
  log "ERROR: $*" >&2
  exit 1
}

# ─────────────────────────────────────────────────────────────
# 사전 검증
# ─────────────────────────────────────────────────────────────
[[ -d "${DATA_ROOT}" ]] || die "DATA_ROOT 디렉토리가 존재하지 않습니다: ${DATA_ROOT}"

# 백업 디렉토리 생성 (없으면)
mkdir -p "${BACKUP_DIR}"

# 백업할 디렉토리가 최소 1개 이상 존재하는지 확인
found_any=false
dirs_to_backup=()
for target in "${BACKUP_TARGETS[@]}"; do
  target_path="${DATA_ROOT}/${target}"
  if [[ -d "${target_path}" ]]; then
    dirs_to_backup+=("${target}")
    found_any=true
  else
    log "WARN: 디렉토리 없음 (건너뜀): ${target_path}"
  fi
done

if [[ "${found_any}" == "false" ]]; then
  die "백업할 디렉토리가 하나도 존재하지 않습니다."
fi

# ─────────────────────────────────────────────────────────────
# 백업 실행
# ─────────────────────────────────────────────────────────────
log "백업 시작 - 대상: ${dirs_to_backup[*]}"
log "DATA_ROOT: ${DATA_ROOT}"
log "BACKUP_DIR: ${BACKUP_DIR}"

tar -czf "${BACKUP_DIR}/${BACKUP_FILE}" \
  -C "${DATA_ROOT}" \
  "${dirs_to_backup[@]}"

backup_size="$(du -h "${BACKUP_DIR}/${BACKUP_FILE}" | cut -f1)"
log "백업 완료: ${BACKUP_FILE} (${backup_size})"

# ─────────────────────────────────────────────────────────────
# 오래된 백업 정리
# ─────────────────────────────────────────────────────────────
backup_count="$(find "${BACKUP_DIR}" -maxdepth 1 -name 'muhan_backup_*.tar.gz' -type f | wc -l)"

if [[ "${backup_count}" -gt "${BACKUP_KEEP}" ]]; then
  delete_count=$((backup_count - BACKUP_KEEP))
  log "보관 한도 초과: ${backup_count}개 존재 (최대 ${BACKUP_KEEP}개) - ${delete_count}개 삭제"

  # 이름 순 정렬 (timestamp 기반이므로 오래된 것이 먼저)
  find "${BACKUP_DIR}" -maxdepth 1 -name 'muhan_backup_*.tar.gz' -type f \
    | sort \
    | head -n "${delete_count}" \
    | while read -r old_backup; do
        log "삭제: $(basename "${old_backup}")"
        rm -f "${old_backup}"
      done
else
  log "보관 현황: ${backup_count}/${BACKUP_KEEP}개"
fi

log "백업 작업 완료"
