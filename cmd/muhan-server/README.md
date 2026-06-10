# `cmd/muhan-server`

본 디렉토리는 무한(Muhan) 게임 서버의 메인 엔트리포인트 패키지를 담당합니다. 레거시 2500줄짜리 `main.go`를 도메인별 기능으로 안전하게 분할하여 관리성을 크게 높였습니다.

## 주요 파일 및 분할 구조

1. **`main.go` / `config.go`**
   - 서버 애플리케이션의 시작점(Entry Point).
   - `config.go`를 통해 CLI 아규먼트를 파싱하고 플래그 기반 설정을 구성합니다.
   - `slog`를 활용한 JSON 구조화 로깅 초기화, Prometheus `/metrics` 엔드포인트 활성화 및 백그라운드 데이터 복원 로직(B/C/D)을 관장합니다.

2. **`login.go` / `password.go`**
   - 사용자 인증 및 계정 생성 로직.
   - 텔넷 접속 시 입력 스트림을 가로채어 비밀번호를 인증하거나, 캐릭터 생성 마법사를 진행합니다.
   - bcrypt 자동 마이그레이션 및 5회 실패 시 IP 차단(Rate Limiting), 중복 로그인 접속 해제(Session Kick) 등 강력한 보안 정책이 포함되어 있습니다.

3. **`handlers.go`**
   - 서버 전체적인 디스패처 및 래퍼 핸들러 설정.
   - 게임 엔진 내 커맨드 시스템을 외부 연결(Session)과 매핑해주는 중간 다리 역할을 합니다.
   - DM(Admin) 권한 판별 등의 기능을 돕습니다.

4. **`ws_proxy.go`**
   - `golang.org/x/net/websocket`을 활용해 TCP 텔넷 포트로 들어오는 소켓 데이터를 WebSocket으로 프록시/변환합니다.
   - WebUI 브라우저 클라이언트가 직접 TCP 통신을 할 수 없는 점을 보완하기 위해 자체 내장 프록시를 띄우며, `Origin` 검증을 통해 보안을 챙깁니다.

5. **`suicide.go`**
   - 레거시 게임 내 캐릭터 삭제(자살/영구탈퇴) 처리 플로우.

## 데이터 흐름 요약

1. 서버 실행 시 `main.go`에서 `loadRuntimeInputs`로 레거시 바이너리(방, 몹) 데이터를 메모리에 탑재 (`internal/world/load`)
2. `net.Listen`을 통해 TCP 포트 리스닝 시작
3. 클라이언트 접근 시, `login.go`의 `serverLoginManager`가 인증을 먼저 통제
4. 인증이 끝나면 `internal/engine/game`의 `Loop`에 세션을 등록, 여기서부터 게임 루프와 커맨드 패턴 동작 시작.
