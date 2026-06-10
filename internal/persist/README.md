# `internal/persist`

`internal/persist` 패키지는 메모리의 런타임 게임 상태를 영구 저장소에 기록하거나 암호화하는 영속성 계층(Persistence Layer)입니다.

## 주요 기능 및 분할 구조

1. **`store/` 및 `jsonstore/`**
   - 레거시 C MUD는 텍스트 파일과 `fwrite`를 혼용했지만, 포팅된 Go 버전에서는 안전한 `JSON 사이드카(Sidecar)` 파일 패턴을 사용합니다.
   - `World.FlushActivePlayersAndBanks()` 등 주기적(Periodic)으로 활성 유저의 데이터와 은행 잔고 상태 등을 `.json` 파일로 덤프하여 저장합니다.
   - 서버 다운 시 데이터 유실을 막는 핵심 레이어입니다.

2. **`legacycrypt/`**
   - 레거시 MUD에서 사용되던 구형 암호화 로직(DES 등) 모음입니다.
   - 구버전 텍스트 파일을 읽어들일 때 원본 패스워드를 해독하거나 비교하는 용도로만 사용되며, 최신 계정 체계에서는 `golang.org/x/crypto/bcrypt` 패키지가 래핑되어 강력한 해싱 체계로 전환되었습니다.

3. **`legacykr/`**
   - CP949 / EUC-KR 등의 레거시 인코딩 문자를 처리합니다.
   - 기존의 MUD 클라이언트들은 UTF-8이 아닌 구형 한글 인코딩을 기준으로 통신하였으며, 내부 로직 및 네트워크 송수신 시 인코딩/디코딩 브릿지 역할을 담당합니다.

4. **`cbin/`**
   - C 스타일 바이너리 직렬화 모듈입니다.
   - 방(Room) 데이터 등 극도로 크고 레거시가 깊은 바이너리를 맵핑하여 Go Struct로 덤프해오는 역직렬화(Unmarshalling) 처리를 돕습니다.
