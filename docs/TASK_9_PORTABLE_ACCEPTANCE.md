# Task 9 — Windows x64 Portable release gate

Status: **LOCAL PORTABLE ACCEPTANCE PASS — READY FOR PRIVATE PUBLICATION**.

Acceptance completed: 2026-10-12. Implementation commit:
`a637b442ce6548509c0ede70fb9db8a0d26893c8`.

Stable product baseline: `d16f08998bcaba11c25eb0371459d82383f0661f`.
Development branch: `codex/windows-portable-v01`, isolated managed worktree.
Task 8 NSIS development is stopped; its uncommitted files remain untouched.

## Implementation and verification

- EXE-anchored `data/engines`, `data/profiles`, `data/settings/webview2`.
- Explicit writable-directory checks and safe, visible startup errors; no CWD,
  environment override, AppData, or shared engine cache fallback.
- Selective reuse of Task 8's pinned official download, progress, cancel/retry,
  archive hash, safe extraction, and engine-file integrity validation. No
  installation ownership, registry, uninstall, or NSIS module is imported.
- Native adapter and existing Profile/Network/ProcessManager remain unchanged.
- Standard-library IANA timezone data embedded for Windows without Go installed.
  A child-process test with unavailable GOROOT/ZONEINFO failed before embedding
  and passed after. Portable root tests also demonstrated RED/GREEN and caught
  an inverted absolute-path guard mutation.
- Go tests and vet: PASS. Frontend: 44 tests PASS, TypeScript/Vite build PASS.
- Wails production build: PASS, windows/amd64, devtools disabled, asInvoker,
  no automatic WebView2 installation. Existing WebView2 Runtime is required.
- UI diagnostic/redaction tests: PASS (canary only). Real authorized proxy
  credentials were used only in the disposable desktop instance, never fixtures,
  diagnostics, source, command-line arguments, or release artifacts.
- Same-user filesystem relocation tests: Profile name/path/seed and DPAPI secret
  preserved. This is unit/integration evidence, **not desktop acceptance**.
- ZIP allowlist/extraction equality: PASS, exactly EXE + README, no live data.
- `git diff --check`: PASS after normalizing generated binding whitespace.
- Final real-credential scan of the source allowlist and both uncompressed ZIP
  entries: zero username/password matches. Source contains no instance data,
  engine binaries, archives, PML or installer files. The generated Wails SDK
  `frontend/wailsjs/runtime` is application binding code, not live runtime data.

Existing build-only npm audit finding: source-map-js high-severity indexed
source-map DoS (GHSA-68fv-2mgg-jv7q). Frozen dependencies were not upgraded.
No source maps or Node runtime are shipped in the portable ZIP.

## Local candidate

- File: `BrowserRuntime-v0.1.0-Windows-x64.zip`
- Size: `5247210` bytes
- SHA-256: `687800a83acdc22d2b1231f045b03c85f6e6342b414b2df2f7f38edd9e90adfa`
- EXE SHA-256: `9369b916870b84585a5d8d3584447af09611212591b7eda56844fe16c823d54d`
- Authenticode: NOT SIGNED. No signing certificate is configured.
- Disposable extraction: `C:\Codex\BrowserRuntime-Task9-Portable-Test`.
- Expected data root: extraction folder + `\data` (initially absent).

## Real desktop acceptance

| Gate | Result | Evidence |
| --- | --- | --- |
| Explorer launch | PASS | Operator confirmed visible UI; manager parent was explorer.exe, Package Identity was absent (15700), not a packaged diagnostic process. |
| Official download | PASS | Operator confirmed completion; pinned archive identity and all 678 installed file hashes validated, with zero unsafe paths or reparse points. |
| Native Direct lifecycle | PASS | Operator confirmed visible browser/Running, Open, and Stop/Stopped; one main browser process, no automation/CDP flags, no proxy argument. |
| P-CTRL save/test | PASS | Existing authorized runtime-only input; UI progressed Needs Test to Ready; US / Los Angeles / America/Los_Angeles / en-US. Password absent from JSON and plaintext secret file bytes. |
| Browser proxy egress | PASS | Operator confirmed browser HTTPS exit matched the manager's confirmed P-CTRL exit. No raw endpoint, exit IP, username, or password retained in this report. |
| Proxy Open/Stop | PASS | Open foreground handle matched the existing browser; Stop left zero browser processes and zero manager listeners. |
| Reopen / identity | PASS | Same profile ID, relative browser-data path and seed hash; exactly one main process and a fresh manager-owned loopback bridge. |
| Manual close | PASS | Operator reported Stopped; read-only verification found zero browser processes and zero old bridge listeners. |
| Manager restart | PASS | Explorer restart restored the original profile/name/seed/path, locality and Chinese preference. |
| Whole-folder relocation | PASS | Moved the stopped complete folder to a second isolated path; profile file hashes unchanged. Explorer restart, UI restore, proxy native launch using the moved browser-data path, and Stop all succeeded. |
| Resource cleanup | PASS | Browser and manager exited normally; all three observed bridge ports released and no instance processes remained. |
| Isolation / deletion | PASS | Operator deleted only the moved disposable folder after automatic deletion was rejected by execution policy. Both disposable paths are absent; Task 8 installation site and original development files are preserved. |

The ordinary Explorer runs did not reproduce Windows 14001. No system change,
global SxS workaround, new engine version, account login, OAuth, detector suite,
Playwright or CDP was used. This does not resolve or reclassify Task 8's separate
installer failure.

Runtime observations (non-secret): initial manager PID 8676; proxy browser
PID 21980; reopened browser PID 21620; restarted manager PID 21196; relocated
manager PID 17680. Bridges listened only on 127.0.0.1, ports 64855, 58428 and
50734, and all were released. Profile identity SHA-256 remained
`f56c85e312a8b57ad951d019f02e076fa614e580bc6f8855a63de6ba678473f0`.

The temporary copy and its browser/profile/DPAPI data were deleted; only the
clean candidate ZIP and source worktree remain. The verification rebuild used a
different output filename and did not replace the accepted EXE or ZIP.

## Known limits

- Requires an existing WebView2 Runtime and a writable non-redirected folder.
- Unsigned EXE; no SmartScreen/security bypass is performed or recommended.
- DPAPI relocation is same Windows user/machine only; not a migration product.
- Engine download cancellation/retry, fail-closed negatives, OS profile lease
  and orphan protection have regression-test evidence, not repeated destructive
  desktop tests in this task. Normal proxy egress/cleanup has real evidence above.
- The existing build-tool dependency advisory above remains disclosed; no
  runtime dependency or frozen dependency version was changed for this release.
- Recovery/rollback: stop all browsers and manager, preserve the complete data
  folder, and keep the previously accepted ZIP. Never replace or delete live data
  while a browser is running. This is the first portable release, so no prior
  portable release is asserted to exist.

## Source/publication protection

Original working directory HEAD and empty staging preserved. Hash summary of
all original dirty/untracked files before and after development:
`E288EFBE919F75541B0812F1DC68FE4844A3419A2B893900F0127B4C74C20390`.
All 14 historical Bake-off/PoC paths and Task 8 changes are excluded from this
development commit.

Operator selected personal repository name **BrowserRuntime**, **Private**.
Authenticated GitHub identity and selected target: `jinjie2031/BrowserRuntime`.
The target was confirmed absent before publication; only a new private repository
may be created, with no overwrite of an existing repository.

After full real acceptance, publish a reviewed **source-only snapshot** to the
new private repository, not this repository's historical Bake-off/PoC history.
Allow only application Go source/tests, go.mod/sum, tracked frontend source and
bindings/config, Wails config/icons/manifest, portable build script and portable
documentation. Exclude engine-bakeoff, poc, historical scripts/reports, runtime,
data, secrets, binaries, raw diagnostics, NSIS files and installers. The portable
ZIP belongs solely in Release Assets. Do not push the existing historical branch
directly to GitHub. All required local real gates above have now passed.
