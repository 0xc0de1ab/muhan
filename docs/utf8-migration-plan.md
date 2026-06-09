# UTF-8 Migration Plan

The Go port should treat player input, command names, runtime strings, and new
persistence writes as UTF-8. Some preserved legacy data is still byte-oriented:
family registries, board bodies or indexes, raw player filenames, and binary C
records can contain EUC-KR/CP949 or other legacy Korean bytes.

Do not mass-convert the original workspace data as a blind first step. The safe
path is audit, fixture selection, disposable-root conversion, then narrowly
scoped production repair.

## Read-Only Audit

Run the audit from the repository root:

```bash
scripts/utf8-audit.sh --root /workspace/muhan --output /tmp/muhan-utf8-audit.txt
```

The script scans:

- `player/family`
- `board`
- `room/json`
- `player/json`
- `player/bank/json`

It reports filename encoding, content encoding, JSON sidecar UTF-8 status, and
sample paths. It does not rewrite source data. Non-UTF-8 paths are displayed as
decoded Korean when possible plus raw hex, which avoids the mojibake that can
appear in generic `find` or `grep` output.

## Interpretation

- `content_utf8_text_or_empty`: safe from a text encoding perspective. JSON
  sidecars should be in this group.
- `content_legacy_cp949`: text that can be decoded through the same Korean
  legacy path already used by the Go loaders. This is a candidate for explicit
  conversion once the owning feature has rewrite coverage.
- `content_legacy_johab`: possible Johab/composed Korean legacy bytes. Treat it
  as high-risk until a specific C fixture proves the intended decoder.
- `content_binary_or_nul`: likely binary C records such as board indexes or
  fixed-size player/room/object records. Do not convert these with a text tool.
- `path_legacy_cp949` or `path_legacy_johab`: filesystem names still contain
  legacy bytes. Prefer resolving them through existing decoded-name lookup until
  every reader and writer has a UTF-8 filename strategy.
- `content_contains_replacement_char`: a valid UTF-8 file already contains
  U+FFFD. Review manually; it may be prior mojibake rather than original Korean.

## Repair Order

1. Keep original legacy binary records as source-of-truth fixtures until the Go
   runtime no longer depends on byte offsets.
2. Convert only append/rewrite text stores with existing Go writers first:
   `family_member_N`, family news, board post bodies, aliases, memos, notepads,
   and mail-like text files.
3. Run conversion only on a disposable copy at first:

   ```bash
   tmp_root="$(mktemp -d /tmp/muhan-utf8-repair.XXXXXX)"
   rsync -a --exclude '.git' /workspace/muhan/ "$tmp_root/"
   scripts/utf8-audit.sh --root "$tmp_root" --output "$tmp_root/utf8-audit-before.txt"
   scripts/utf8-convert-text.sh --root "$tmp_root" --include family --manifest "$tmp_root/utf8-convert-family.manifest"
   scripts/utf8-rename-paths.sh --root "$tmp_root" --include all --manifest "$tmp_root/utf8-rename-paths.manifest"
   ```

4. If the dry-run manifest is limited to the expected text files, run the write
   pass on the disposable copy:

   ```bash
   scripts/utf8-convert-text.sh --root "$tmp_root" --include family --write --manifest "$tmp_root/utf8-convert-family-write.manifest"
   scripts/utf8-rename-paths.sh --root "$tmp_root" --include all --write --manifest "$tmp_root/utf8-rename-paths-write.manifest"
   ```

5. After a targeted repair, verify:

   ```bash
   scripts/utf8-audit.sh --root "$tmp_root" --output "$tmp_root/utf8-audit-after.txt"
   go run ./cmd/muhan-server -root "$tmp_root" -validate -ansi=false
   scripts/porting-smoke.sh --runtime --copy-root --skip-go-test --no-sidecar
   ```

6. Promote a repair only when the before/after diff is limited to the intended
   files, validate still reports `0 errors`, and restart smoke proves the
   rewritten data is loadable.

## Conversion Helper

`scripts/utf8-convert-text.sh` is deliberately narrower than the audit. It only
converts selected text stores and defaults to dry-run mode:

```bash
scripts/utf8-convert-text.sh --root /workspace/muhan --include family --manifest /tmp/muhan-family-convert.manifest
```

Use `--write` only against a disposable copy. The script refuses to write to
`/workspace/muhan` unless `--allow-original` is explicitly passed. `board_index`,
family bank files, and fixed binary records are skipped by design.

`scripts/utf8-rename-paths.sh` handles the complementary path-name migration for
known safe directories:

```bash
scripts/utf8-rename-paths.sh --root /workspace/muhan --include all --manifest /tmp/muhan-path-rename.manifest
```

It renames only selected CP949 filename components, currently family bank files
and board auxiliary filenames. It also defaults to dry-run and refuses writes to
the repository root without `--allow-original`.

## Live Gap Report Mojibake

`scripts/porting-smoke.sh --live-gap-report` intentionally scans real fixtures
without mutating the root. The talk candidate section decodes CP949-compatible
talk files for display. Use `scripts/utf8-audit.sh` alongside it when choosing a
fixture that still has legacy path bytes: the audit report includes decoded
names and raw hex for paths that are not stable UTF-8.

The server-side loaders remain responsible for decoding actual game data
through `legacykr.ValidUTF8OrDecodeContext` or feature-specific binary record
decoders. The audit script is a visibility tool, not a replacement for those
loaders.
