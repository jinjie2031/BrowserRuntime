# BrowserRuntime 0.1.0 — Windows x64 Portable

A local, single-user browser profile manager with native CloakBrowser launch,
English/简体中文 UI, persistent profiles, stable profile identity, and optional
per-profile proxy networking.

## Run

Download `BrowserRuntime-v0.1.0-Windows-x64.zip` from this private repository's
Release Assets, extract it into a writable local folder, and double-click
`BrowserRuntime.exe` in Windows Explorer. An existing Microsoft Edge WebView2
Runtime is required; Go, Node, an installer and administrator rights are not.

The application is unsigned. No Windows security bypass is recommended.

On first run, use the official engine download button. The portable ZIP contains
no CloakBrowser binary. Downloads are pinned to `chromium-v146.0.7680.177.5` and
verified before native launch.

All instance data belongs to `<EXE folder>/data/`. Stop all browsers and close
the manager before moving the complete folder. DPAPI proxy secrets work only
for the same Windows user and machine. Never upload or share live `data/`.

See [portable instructions](docs/PORTABLE_README.md) and
[real acceptance evidence and limitations](docs/TASK_9_PORTABLE_ACCEPTANCE.md).

## Build from source

Use the versions pinned in `go.mod`, `frontend/package.json` and its lockfile.
Windows development requires Go, Node/npm, Wails and its Windows build tools.

```powershell
go test ./...
go vet ./...
cd frontend
npm ci
npm test
npm run build
cd ..
./scripts/build-portable.ps1
```

Output: `build/bin/BrowserRuntime-v0.1.0-Windows-x64.zip` (EXE + README only).

This repository starts from a reviewed source-only snapshot. Historical
Bake-off/PoC material, installer work, credentials, browser profiles, engine
archives and diagnostic captures are deliberately not included in Git history.

There is no REST server, browser automation, OAuth/account management,
Playwright, WebDriver, or CDP integration.
