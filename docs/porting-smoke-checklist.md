# Porting Smoke Checklist

Use this checklist after each C parity wave. It is intentionally conservative:
read-only commands can run on the normal workspace, but destructive lifecycle
checks must use a disposable copy of the data root.

## 0. Automated Smoke Entry Point

For repeatable checks, prefer the script first and use the manual sections below
only when a failure needs interactive debugging.

```bash
scripts/porting-smoke.sh
```

Default behavior:

- Runs `go test ./... -count=1`, server build, server validate, command-list
  load, and sidecar migration dry-run.
- Writes artifacts under `/tmp/muhan-porting-smoke.*` unless
  `MUHAN_SMOKE_LOG_DIR` or `--log-dir` is provided.
- Emits `gap-report.txt`, which summarizes the validation signals and the C
  parity gates that still require disposable-root runtime smoke.
- Preserves tmux pane snapshots before cleanup as `server-exit.log`,
  `client-exit.log`, and `client-second-exit.log` when those sessions were
  started; ANSI runs also keep `client-exit-raw.log`.

Runtime login smoke is opt-in because it starts tmux sessions and logs in as
real characters through `cmd/muhan-client`.

```bash
scripts/porting-smoke.sh --runtime --skip-go-test
scripts/porting-smoke.sh --all --copy-root
scripts/porting-smoke.sh --runtime --leave-sessions --port 4040
```

Destructive lifecycle smoke is a separate opt-in scenario. It always copies the
data root first and runs against that disposable copy.

```bash
scripts/porting-smoke.sh --scenario --skip-go-test --no-sidecar
scripts/porting-smoke.sh --scenario-family --skip-go-test --no-sidecar
scripts/porting-smoke.sh --scenario-class-family --skip-go-test --no-sidecar
scripts/porting-smoke.sh --scenario-permdeath --skip-go-test --no-sidecar
scripts/porting-smoke.sh --scenario-trap --skip-go-test --no-sidecar
scripts/porting-smoke.sh --scenario-talk-give --skip-go-test --no-sidecar
scripts/porting-smoke.sh --scenario-talk-effects --skip-go-test --no-sidecar
scripts/porting-smoke.sh --scenario-objects --skip-go-test --no-sidecar
```

Open C parity gaps also have a non-mutating evidence mode. It scans the current
root for real data fixtures, records talk directive candidates, and, when paired
with `--runtime`, annotates the captured client transcript. It does not send
destructive commands by itself.

```bash
scripts/porting-smoke.sh --live-gap-report --no-preflight --no-sidecar
scripts/porting-smoke.sh --runtime --copy-root --live-gap-report --skip-go-test --no-sidecar
```

Runtime defaults:

- server tmux session: `muhan-server`
- client tmux session: `muhan-cli`
- second client tmux session: `muhan-cli-2`
- login: `인제로` / `1234`
- second login: `레터` / `1234`
- read-only commands: `봐`, `정보`, `점수`, `장비`, `소지`, `게시판`,
  `계석치무 봐`, `계석치무 임무 자세히 대화`, `밑`, `누구`
- Runtime login smoke advances the C-style post-login `log/news`/`DM_news`
  view-file prompt before sending gameplay commands.
- Unit coverage checks that a legacy `post/<player-name>` file makes successful
  login print `*** 우체국에 편지가 와있습니다.`, matching C `load_ply()`.

Use `--copy-root` for any run where persistence sidecars, shutdown flush, or
manual follow-up should not touch the normal workspace. The script checks for
the key transcript markers: `통계 무한 광장`, `생명의 나무`, `문이 닫혀 있습니다`,
`접속자`, the login names, and a same-room `말` broadcast from the first client
to the second. Successful runtime runs also write `server.log`, `client.log`,
`client-second.log`, and, for `--ansi`, `client-ansi-raw.log`.

The current `--scenario` path logs in as `인제로` / `1234` by default, runs the
legacy suicide command `목매달기`, verifies that a wrong password and `뻥으로`
confirmation leave files unchanged, then confirms `찐짜로` on the disposable
root, waits for the server-side `자살신청` audit signal, and checks player/bank
cleanup, `family_member_N` UTF-8 rewrite, and validate-after-restart.

The current `--scenario-family` path also uses a disposable root. If the smoke
login is a family member but not the configured boss, the script seeds only the
copied JSON sidecar with a temporary boss flag, then records a real
`패거리원` -> `패거리추방` -> validate -> restart -> `패거리원` transcript.
The repository root is never mutated by this seeded boss step.

The current `--scenario-trap` path uses a disposable root, seeds only the copied
login sidecar, and records two real movements: `room:00159 동 -> room:00165`
pit trap to `room:00149`, then `room:00226 동 -> room:00227` alarm trap from
`room:00231`. It checks the pit message, damage message, player death/drop
message, alarm message, moved guard visibility, and validate-after-run. The
repository root is never mutated by this seeding step.

The current `--scenario-talk-give` path uses a disposable root, seeds only the
copied login sidecar to start in `room:03208` with empty inventory, and records
the real `불의 용사 대화` MTALKS/GIVE path. It checks the `불의 공주` response,
`불의 감옥 열쇠` reward output, inventory visibility, and validate-after-run.

The current `--scenario-talk-effects` path uses a disposable root, seeds only
the copied login sidecar between restarts, and records real talk side-effect
transcripts: `아가씨 사랑 대화` ACTION in `room:01075`, `아미타불 치료 대화`
CAST in `room:01008`, and `떠돌이 싸우자 대화` ATTACK in `room:00542`. It
checks the visible social action, spell-cast output, attack primer output, and
validate-after-run.

The current `--scenario-objects` path uses a disposable root, seeds only copied
JSON sidecars, and records a real ORENCH + nested-object path. It promotes the
login to DM class in the copy, runs `*create 212` for `object:o02:12`/`자객검`,
seeds `까마귀`'s `작은 보따리` containing `솔개 검`, verifies look/drop/room-save,
then restarts and verifies the dropped container still renders its child.

The current `--scenario-permdeath` path uses a disposable root, seeds only the
copied login sidecar to DM class in `room:03566`, runs `*monster 98`, kills
`타타르의 머리` through the normal attack loop, repeats the same attack command
to drive the `MSUMMO` summon chain to `타타르의 몸`, verifies legacy `MDEATH`,
`MSUMMO`, quest reward output, persisted `perm_mon.0.ltime`, and
validate-after-run.

The current `--live-gap-report` path writes `live-gap-report.txt`. Use it before
manual live testing to pick real fixtures for family, permanent monster death,
ORENCH/nested object, trap, and talk ACTION/GIVE/ATTACK/CAST evidence.

UTF-8/legacy byte status is audited by a separate read-only tool. Run it next to
the live gap report when a fixture path or text looks mojibaked in shell output.
It does not modify source data.

```bash
scripts/utf8-audit.sh --root /workspace/muhan --output /tmp/muhan-utf8-audit.txt
```

See [utf8-migration-plan.md](/workspace/muhan/docs/utf8-migration-plan.md) for
the safe conversion order and interpretation of `legacy-cp949`,
`legacy-johab`, and `binary-or-nul` findings.

For a dry-run conversion manifest on a targeted text set:

```bash
scripts/utf8-convert-text.sh --root /workspace/muhan --include family --manifest /tmp/muhan-family-convert.manifest
scripts/utf8-rename-paths.sh --root /workspace/muhan --include all --manifest /tmp/muhan-path-rename.manifest
```

Do not pass `--write` against the repository root during smoke work. Use a
copied root and validate it after conversion.

## 1. Preflight

Run from `/workspace/muhan`.

```bash
scripts/porting-smoke.sh
go test ./... -count=1
go build -o /tmp/muhan-server-check ./cmd/muhan-server
go run ./cmd/muhan-server -root /workspace/muhan -validate -ansi=false
go run ./cmd/muhan-cmdlist -root /workspace/muhan
go run ./cmd/muhan-sidecarmigrate -root /workspace/muhan -json
```

Expected:

- Tests pass.
- Build succeeds.
- Validate reports `0 errors`.
- Command list still sees all legacy command entries.
- Sidecar dry-run exits with no migration errors.
- Smoke `gap-report.txt` includes the sidecar `byType` summary when `jq` is
  available.
- Server-side migration remains explicit opt-in via `-migrate-sidecars`; do not
  run it against the repository root unless intentionally rewriting sidecars.

If this fails, do not run runtime smoke until the failure is classified.

## 2. Start Server in tmux

Pick a local port that is not already in use.

```bash
tmux kill-session -t muhan-server 2>/dev/null || true
tmux new-session -d -s muhan-server 'cd /workspace/muhan && go run ./cmd/muhan-server -root /workspace/muhan -listen 127.0.0.1:4040 -ansi=false'
tmux capture-pane -pt muhan-server -S -80
```

Expected server output:

- `runtime world: initialized`
- `listening: 127.0.0.1:4040` or equivalent bound address
- No schema-version warning spam

## 3. Connect Client in tmux

```bash
tmux kill-session -t muhan-cli 2>/dev/null || true
tmux new-session -d -s muhan-cli 'cd /workspace/muhan && go run ./cmd/muhan-client -addr 127.0.0.1:4040'
tmux send-keys -t muhan-cli '인제로' C-m
tmux send-keys -t muhan-cli '1234' C-m
tmux capture-pane -pt muhan-cli -S -120
```

Expected client output:

- Login prompt accepts `인제로` / `1234`.
- `통계 무한 광장` renders.
- The 생명의 나무 ASCII block has aligned description text.
- The prompt returns as `>`.

For raw protocol debugging, `nc`, `socat`, or `telnet` can still be used, but
the checked smoke path uses the Go client.

## 4. Multi-Client Smoke

Start a second client in the same room, log in as `레터` / `1234`, then verify
same-room chat and `누구`.

```bash
tmux kill-session -t muhan-cli-2 2>/dev/null || true
tmux new-session -d -s muhan-cli-2 'cd /workspace/muhan && go run ./cmd/muhan-client -addr 127.0.0.1:4040'
tmux send-keys -t muhan-cli-2 '레터' C-m
tmux send-keys -t muhan-cli-2 '1234' C-m
tmux send-keys -t muhan-cli '멀티스모크 말' C-m
tmux send-keys -t muhan-cli-2 '누구' C-m
tmux capture-pane -pt muhan-cli-2 -S -160
```

Expected:

- The second client sees `인제로가 "멀티스모크"라고 말합니다.`.
- `누구` includes both `인제로` and `레터`.

## 5. Read-Only Player Smoke

Send one command at a time and capture after each group.

```bash
tmux send-keys -t muhan-cli '봐' C-m
tmux send-keys -t muhan-cli '정보' C-m
tmux send-keys -t muhan-cli C-m
tmux send-keys -t muhan-cli '점수' C-m
tmux send-keys -t muhan-cli '장비' C-m
tmux send-keys -t muhan-cli '소지' C-m
tmux send-keys -t muhan-cli '게시판' C-m
tmux send-keys -t muhan-cli '계석치무 봐' C-m
tmux send-keys -t muhan-cli '계석치무 임무 자세히 대화' C-m
tmux send-keys -t muhan-cli '밑' C-m
tmux capture-pane -pt muhan-cli -S -240
```

Expected:

- Room look, info first/second pages, score, equipment, and inventory render
  without panic.
- Board list renders or reports a controlled no-board/no-post state.
- NPC look renders the 계석치무 description.
- Talk topic uses the C-style single-word key, so `임무 자세히` resolves as
  topic `임무`.
- `밑` reports the current closed-door result and does not disconnect.

Do not include attack, steal, use, cast, forge, class change, suicide, board
write/delete, mail delete, or bank transfer in the normal smoke.

## 6. ANSI Toggle Smoke

The automated runtime smoke can repeat the two-client transcript with ANSI
enabled and fails if the raw client capture does not contain escape sequences.

```bash
scripts/porting-smoke.sh --runtime --copy-root --ansi --skip-go-test --no-sidecar
```

Manual equivalent: stop the server, then repeat login with ANSI enabled.

```bash
tmux send-keys -t muhan-cli C-c
tmux send-keys -t muhan-server C-c
tmux new-session -d -s muhan-server-ansi 'cd /workspace/muhan && go run ./cmd/muhan-server -root /workspace/muhan -listen 127.0.0.1:4041 -ansi=true'
tmux new-session -d -s muhan-cli-ansi 'cd /workspace/muhan && go run ./cmd/muhan-client -addr 127.0.0.1:4041'
tmux send-keys -t muhan-cli-ansi '인제로' C-m
tmux send-keys -t muhan-cli-ansi '1234' C-m
tmux send-keys -t muhan-cli-ansi '봐' C-m
tmux capture-pane -pt muhan-cli-ansi -S -160
```

Expected:

- Output is readable.
- ANSI escape sequences are present when inspecting raw capture/log output; the
  automated smoke records this as `client-ansi-raw.log`.
- The non-ANSI smoke remains the default for text comparison.

Clean up:

```bash
tmux kill-session -t muhan-server-ansi 2>/dev/null || true
tmux kill-session -t muhan-cli-ansi 2>/dev/null || true
```

## 6. Persistence and Migration Smoke

Use a disposable copy for mutation tests.

```bash
tmp_root="$(mktemp -d /tmp/muhan-smoke.XXXXXX)"
rsync -a --exclude '.git' /workspace/muhan/ "$tmp_root/"
go test ./internal/world/state -run 'Test.*PlayerSave|Test.*Persistence|Test.*Schema|Test.*Sidecar' -count=1
```

Then run server against `$tmp_root`, mutate only a throwaway account, stop with
SIGTERM, restart, and verify the mutation survived.

Checklist:

- Player sidecar loads.
- Bank sidecar loads.
- Room floor object sidecar loads.
- Board and family news sidecars load.
- v1 sidecars migrate to v2, including board/family-news rewrite details and
  type counts in CLI and startup migration summaries.
- v3+ sidecars are reported as unsupported and not rewritten.
- Malformed JSON sidecars fail loudly.
- Player, bank, board, and room-object sidecar filenames cannot escape their
  JSON directories through path traversal.
- SIGTERM path logs a full flush.

Do not run suicide, class change, or destructive bank/mail tests on the original
workspace root.

## 7. Gap-Specific Smoke

Run these only after the matching implementation lands.

The automated script records these gates in `gap-report.txt`; the entries below
define the runtime evidence needed before a gap can be moved out of the active
C parity list.

Suicide lifecycle:

- Covered by `scripts/porting-smoke.sh --scenario --skip-go-test --no-sidecar`
  for the default smoke account.
- Wrong password leaves save, bank, and aliases intact.
- `뻥으로` leaves all data intact.
- `찐짜로` deletes or disables save, bank, aliases, and family membership through
  the configured sink.
- The disposable root validates cleanly after the cleanup, so the rewritten
  family registry is restart-loadable.

change_class family registry:

- Join/leave/kick/class-change persistence paths are now implemented or
  unit-covered.
- Boss kick/restart visibility is covered by
  `scripts/porting-smoke.sh --scenario-family --skip-go-test --no-sidecar`.
- Family class-change/restart visibility is covered by
  `scripts/porting-smoke.sh --scenario-class-family --skip-go-test --no-sidecar`.
  The default scenario copies the root, seeds `소녀무사` into training room
  `room:01034`, runs `직업전환`, and verifies `family_member_N` after restart.
- Non-family player changes class and combat stats update.
- Family member changes class and `패거리원` reflects the new class.
- Family hook failure after confirmation is unit-covered: the error is reported
  after class/experience mutation, matching C's post-mutation `edit_member`.
- Boss kick removes the member from the runtime list and the UTF-8
  `family_member_N` file.
- Restart preserves the family member update.

Talk side effects:

- Unit-covered for `ACTION`, `GIVE`, `ATTACK`, and `CAST` ordering and side
  effects, including duplicate reward refusal and clone rollback.
- Korean `CAST` fallback is covered for a real `아미타불 치료 대화` fixture:
  `회복` heals the player and spends NPC MP when no custom runtime hook handles
  the spell.
- Offensive `CAST` fallback is unit-covered against the real `떠돌이 검객`
  `이놈 CAST 뇌전` fixture: the player takes monster-spell damage, MP is spent,
  mutual hatred is established, and the NPC is primed for follow-up combat.
  Hook-enabled test worlds also prove offensive spells route through the C
  offensive branch before any Go runtime cast hook can intercept them.
- Non-offensive `CAST` low/missing-MP failure is unit-covered: only the asking
  player receives the C apology, with no status effect and no room failure line.
- `MTALKS` gating and real `GIVE` prototype rewards are covered in real-data
  tests and by
  `scripts/porting-smoke.sh --scenario-talk-give --skip-go-test --no-sidecar`.
- Real ACTION/CAST/ATTACK runtime transcripts are covered by
  `scripts/porting-smoke.sh --scenario-talk-effects --skip-go-test --no-sidecar`.
  Start with `--live-gap-report` before changing those fixtures.
- Real-data ACTION names are unit-checked against the executable talk social
  action list, so parsed directives cannot silently become no-ops.

Player offensive spell output:

- Unit coverage checks successful player offensive casts emit C
  caster-facing spell-detail text before the damage line, low/mid-tier spells
  emit the additional C room detail broadcast after the initial cast line, and
  high-tier `SISIX1..4`/`XIXIX1..4` keep C's direct-only detail split.

Trap/death:

- Pit trap room renders before trap effect.
- Fatal pit trap follows the combat death path; runtime evidence is covered by
  `scripts/porting-smoke.sh --scenario-trap --skip-go-test --no-sidecar`.
- Alarm trap runtime evidence is covered by the same scenario through the
  `room:00226 동 -> room:00227` route and moved guard visibility.
- `PPREPA` trap-prepare cleanup is unit-covered for no-trap clearing, unknown
  nonzero trap preservation, and levitating pit no-damage clearing.
- `TRAP_RMSPL` spell-loss parity is unit-covered: canonical active spell
  expirations are set to zero like C `lasttime[LT_*].interval = 0`, while flags
  remain until the normal player update expiry path clears them.
- `TRAP_NAKED` destructive parity is unit-covered: non-cursed inventory and
  equipment are recursively destroyed, cursed equipment remains, and player
  dirty persistence is marked after object-tree destruction.

Talk GIVE/MTALKS:

- Topic matching uses the second token only.
- Numeric reward is created once.
- Numeric `GIVE` action text follows legacy `atoi()` behavior: suffixes like
  `25냥` are object-number attempts, and missing prototypes do not transfer
  gold.
- Duplicate quest reward is refused.
- Reward refusal due to inventory count or weight leaves state unchanged.
- Real `MTALKS + GIVE 104` transcript grants `불의 감옥 열쇠` through
  `--scenario-talk-give`.
- Board write pending-input safety is unit/server-loop covered: body files,
  index records, and JSON sidecars are not written before the final dot, and a
  disconnect before that dot cannot later run the pending writer.
- Board post-read pending-pager safety is unit/server-loop covered: long post
  bodies use the C `view_file()` page size and prompt, and disconnecting after
  page one prevents later continuation output.
- Mail `postedit` pending-input safety is unit/server-loop covered: each body
  line is persisted immediately like C, and a disconnect prevents later appends.
- Mail `postread` pending-pager safety is server-loop covered: disconnecting
  after the first page prevents later continuation output.
- Player help `view_file()` pending-pager safety is unit/server-loop covered:
  long help files use the C page size and prompt, and disconnecting after page
  one prevents later continuation output.
- `readscroll` special-map pending-pager safety is server-loop covered:
  long `SP_MAPSC` files use the C `view_file()` page size and prompt, and
  disconnecting after the first page prevents later continuation output while
  leaving the map object unconsumed.
- DM `*log` pending-pager safety is unit/server-loop covered: long logs use
  the C `view_file()` page size and prompt, and disconnecting after page one
  prevents later continuation output.
- DM notepad pending-input safety is server-loop covered: the first line is
  persisted immediately like C `noteedit`, a disconnect prevents later pending
  appends, and long plain reads use the C `view_file()` pager with closed-session
  continuation blocked.
- Family-news pending-input safety is server-loop covered: the first line
  updates the legacy notice file and sidecar immediately, a disconnect prevents
  later appends, and long reads use the C `view_file()` pager with
  closed-session continuation blocked.
- Vote pending-choice safety is server-loop covered: disconnecting before the
  first choice writes no vote file, and disconnecting after C-style change
  confirmation writes no replacement vote.
- Password change pending-confirm safety is server-loop covered: disconnecting
  before the re-entry confirmation leaves the legacy password hash unchanged.
- Invincible-training pending-confirm safety is server-loop covered:
  disconnecting before `예` leaves experience, class, `pDice`, and training tags
  unchanged.
- Class-change pending-confirm safety is server-loop covered: disconnecting
  before `예` leaves class and experience unchanged.
- Weapon-forge pending-confirm safety is server-loop covered for `forge` and
  `newforge`: disconnecting before final `예` leaves gold and inventory
  unchanged.

Permanent monster death:

- Covered by
  `scripts/porting-smoke.sh --scenario-permdeath --skip-go-test --no-sidecar`
  for the current `타타르의 머리` fixture.
- The smoke drives the full `타타르의 머리` summon chain with repeated normal
  attack commands, so it does not depend on active-combat tick timing.
- Killing a permanent monster prints the legacy `MDEATH` description when one
  exists.
- The production server loop is wired with `game.WithWorld`, so attack death
  finalizers have access to the live world for permanent death side effects.
- Special combat death paths share the same finalizer in the server:
  `poison_mon`, `magic_stop`, `absorb`, `red_eye`, `turn`, `eight`, `nahan`,
  `bnahan`, `tagu`, `chang`, `choi`, `invincible_kick`, `one_kill`, `poback`,
  and `lion_scream`.
- Creature prototypes loaded from `objmon/mNN` preserve C level, HP/MP, combat
  dice, special/quest fields, carry slots, raw flags, and both C/Go flag tags,
  so `*monster` and respawn/summon clones enter combat with real legacy stats.
- `*dm_set` creature flag toggles and `*status` flag rendering resolve the full
  63-bit monster flag table through both C names and Go aliases.
- `*charm`/`list_charm` reads the caster charm list recorded by successful charm
  effects, and removed creatures are pruned from that list.
- Respawn/ltime state is updated and survives the configured room sidecar
  persistence path.
- Quest reward, `MSUMMO` summon, and global broadcast side effects match the C
  path for the selected fixture.

ORENCH and nested object materialization:

- Creating an `ORENCH` prototype applies randomized enchant values in the same
  range and fields as C `rand_enchant`.
- A rewarded or cloned prototype with contained objects materializes child
  objects, and those children survive give/drop/save/restart.
- `--scenario-objects` covers a live DM create plus nested container
  look/drop/room-save/validate/restart transcript on a copied root.
- Rejected rewards leave player inventory, quest flag, and object counters
  unchanged.

Suggested evidence run:

```bash
scripts/porting-smoke.sh --runtime --copy-root --live-gap-report --skip-go-test --no-sidecar --leave-sessions --port 4042
tmux capture-pane -pt muhan-cli -S -500 > /tmp/muhan-live-gap-client.log
tmux capture-pane -pt muhan-server -S -500 > /tmp/muhan-live-gap-server.log
```

After the copied-root server is left running, drive exactly one gap scenario at
a time from `muhan-cli`, then restart the same copied root and compare the
before/after transcript. Do not run those mutating scenarios on the repository
root.

## 8. Capture Artifacts

For every smoke run, save:

```bash
tmux capture-pane -pt muhan-server -S -300 > /tmp/muhan-server-smoke.log
tmux capture-pane -pt muhan-cli -S -300 > /tmp/muhan-cli-smoke.log
```

Attach the logs to the wave report or summarize the exact failure line in
`PORTING-GAPS.md`.
