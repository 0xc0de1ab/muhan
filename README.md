# Muhan Server (무한 서버)

Muhan(무한)은 레거시 C 기반 MUD 게임을 Go 언어로 현대화한 프로젝트입니다. 
기존의 방대한 바이너리 맵, 플레이어 데이터, 오브젝트 데이터를 안정적으로 마이그레이션하고, 고도화된 동시성 제어 및 최신 보안 프로토콜을 탑재한 서버입니다.

## 🚀 시작하기

### 로컬 빌드 및 실행

서버 빌드에는 Go 1.21+ 환경이 필요합니다.

```bash
# 전체 패키지 빌드 및 테스트
go build ./...
go test ./...

# 서버 실행 (루트 디렉토리에서 실행)
go run ./cmd/muhan-server/
```

### Docker 빌드 및 실행

도커를 이용하면 서버와 뷰(Vue) 기반의 Web 클라이언트를 한 번에 구동할 수 있습니다. Nginx가 웹과 웹소켓 요청을 프록시합니다.

```bash
# Docker 이미지 빌드
docker build -t muhan-server:latest -f docker/Dockerfile .

# Docker 컨테이너 실행 (80번, 443번 포트 노출)
docker run -d --name muhan \
  -p 80:80 \
  -p 443:443 \
  -p 4000:4000 \
  -v muhan_data:/data \
  muhan-server:latest
```

#### Persistent Volume 마운트 (데이터 보존)

사용자 정보, 가족 게시판 등 지속되어야 하는 데이터는 반드시 Persistent Volume으로 마운트하여 보존해야 합니다.
`-v [호스트경로]:/data` 형태로 마운트하여 사용합니다.

```bash
# 호스트의 /srv/muhan-data 디렉토리를 컨테이너의 /data 경로에 마운트
docker run -d --name muhan \
  -p 80:80 \
  -v /srv/muhan-data:/data \
  -v /srv/muhan-backups:/data/backups \
  muhan-server:latest
```

## 🛠️ 주요 디렉토리 구조

프로젝트는 Go 언어의 표준 레이아웃을 따르며, 다음과 같이 분할되어 있습니다. 
각 디렉토리에 대한 자세한 정보는 해당 폴더 내의 `README.md`를 참고하세요.

- **`cmd/`**: 실행 가능한 애플리케이션 엔트리포인트 모음. 주 서버(`muhan-server`) 및 다양한 유틸리티 도구 포함.
- **`internal/`**: 외부 패키지에서 사용할 수 없는 비공개 패키지 모음.
  - **`engine/`**: 게임 루프, 커맨드 파싱, 소셜/대화 등 실제 게임 플레이 로직
  - **`world/`**: 객체, 몹, 플레이어, 방 등 가상 세계의 상태(State) 및 메모리 관리
  - **`persist/`**: 파일 I/O, 암호화, 구조체 마샬링 등 데이터베이스 시스템
  - **`metrics/`**: Prometheus 모니터링 메트릭 설정
- **`objmon/`**, **`help/`**: 원본 레거시 데이터 (몬스터 정의, 도움말 텍스트 등)
- **`scripts/`**: 백업 스크립트 등 운영 유틸리티
- **`docker/`**: Nginx, WebUI 및 Dockerfile 설정

## ⚙️ CLI 명령어 (cmd/muhan-server)

서버 실행 시 다양한 플래그를 통해 동작 방식을 제어할 수 있습니다.

| 플래그 | 기본값 | 설명 |
|---|---|---|
| `-listen` | `:4000` | 텔넷 클라이언트가 접속할 TCP 포트 |
| `-ws-listen` | `127.0.0.1:4041` | 웹소켓(WebUI) 클라이언트가 접속할 포트 |
| `-metrics-listen` | `:2112` | Prometheus 메트릭 수집을 위한 HTTP 포트 |
| `-root` | `.` | 레거시 Muhan 소스/데이터가 위치한 루트 경로 |
| `-ansi` | `true` | 클라이언트에 ANSI 색상 코드 전송 여부 |
| `-validate` | `false` | 서버를 실행하지 않고 런타임 입력값과 맵 유효성만 검증 후 종료 |
| `-dry-run` | `false` | 유효성 검증 후 리스닝 없이 종료 |
| `-migrate-sidecars` | `false` | 시작 전 기존 JSON 사이드카(플레이어 등) 스키마를 마이그레이션 |

### 실행 예시

```bash
# 다른 데이터 경로를 바라보고, 웹소켓 포트를 8080으로 실행
go run ./cmd/muhan-server/ -root=/data/muhan -ws-listen=0.0.0.0:8080 -metrics-listen=:2112
```

## 🔒 보안 및 모니터링 기능

- **동시 접속 제한**: 글로벌 256명 제한 및 단일 IP당 최대 5개 커넥션 허용으로 서버 마비(DDoS)를 방지.
- **Rate Limiting**: 5회 이상 로그인 실패 시 IP 5분 차단 조치.
- **구조화된 로깅**: `log/slog` 패키지를 통한 JSON 로깅 (운영 모니터링 용이성).
- **모니터링 연동**: Prometheus 연동(`/metrics`)으로 동시 접속자 수 및 로그인 실패, 커맨드 처리량 추적 가능.
- **최신 암호화 체계**: 레거시 DES 암호를 `golang.org/x/crypto/bcrypt`로 자동 마이그레이션.
