# Changelog

## 0.1.0 — 2026-10-12

### Added

- Windows x64 portable release: extract and run from Explorer without installing.
- Official, pinned CloakBrowser download with progress, cancel/retry, SHA-256
  verification and safe extraction. Engine binaries are not bundled.
- EXE-relative engine/profile/settings data, same-user folder relocation and
  explicit rejection of unwritable or redirected data directories.
- English/简体中文 profile management UI; native Launch/Open/Stop; stable profile
  path and fingerprint seed; DPAPI proxy passwords and loopback proxy bridge.

### Safety and limitations

- Proxy failures do not silently become Direct; active/unknown browser states
  block unsafe manager exit and network edits.
- Real Explorer/native lifecycle, authorized proxy egress, manual close,
  restart, relocation and isolated deletion acceptance passed.
- Requires an existing WebView2 Runtime. The EXE is unsigned.
- DPAPI relocation is same machine/user only; no cross-machine migration.
- Existing build-only source-map-js advisory is disclosed in the acceptance
  report. No source maps or Node runtime are included in the portable ZIP.
- No account login, OAuth or production residential-network approval is implied.

### Artifact

- `BrowserRuntime-v0.1.0-Windows-x64.zip`
- Size: 5,247,210 bytes
- SHA-256: `687800a83acdc22d2b1231f045b03c85f6e6342b414b2df2f7f38edd9e90adfa`
