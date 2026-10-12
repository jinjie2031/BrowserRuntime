# BrowserRuntime 0.1.0 — Windows x64 Portable

Extract the entire ZIP into a writable local folder, then double-click
`BrowserRuntime.exe` in Windows Explorer. No Go, Node, installer, administrator
rights, install registration, or uninstaller is required. Microsoft Edge
WebView2 Runtime must already be available; the portable app does not install it.

解压到可写的本地文件夹，通过资源管理器双击 `BrowserRuntime.exe`。
无需安装、管理员权限、Go 或 Node。系统需已有 Microsoft Edge WebView2 Runtime。

## Engine

The ZIP does not include CloakBrowser. On first launch, choose **Download and
install / 下载安装**. Progress, cancellation, and retry are available. The app
downloads only the pinned official CloakHQ Windows x64 archive, verifies its
SHA-256, and safely extracts it into this instance's data directory. A missing
or invalid engine blocks browser launch; shared engine caches are never adopted.

- Release: `chromium-v146.0.7680.177.5`
- Official URL: <https://github.com/CloakHQ/CloakBrowser/releases/download/chromium-v146.0.7680.177.5/cloakbrowser-windows-x64.zip>
- SHA-256: `b213795cb32c3169f766c74ce1d0275fc89d3df256de39c04da7fb4c23b7fdbe`

## Portable data

All instance data is relative to the EXE, **not** the working directory:

```
BrowserRuntime.exe
data/
  engines/cloakbrowser/146.0.7680.177.5/
  profiles/
  settings/webview2/
```

The settings directory contains the manager's WebView2 data, including the UI
language preference. Browser Profile locale/timezone are independent of UI
language. Profile paths remain relative and fingerprint seeds remain stable.
An unavailable/unwritable or redirected data root is rejected, with no fallback
to AppData or any other location.

Stop **all** browsers and close the manager before moving the whole folder.
Move the entire folder, including `data`; never move only the EXE. Proxy secrets
use Windows DPAPI CurrentUser: portable folder relocation means relocation on
the **same Windows machine and user**, not credential portability to other users
or computers. This is not a cross-machine Profile migration feature.

Before deleting this portable instance, stop every browser and close the
manager. Delete only this extracted folder. No uninstaller or registry cleanup
is necessary. Deleting it permanently removes its profiles and settings.
Do not upload or share `data`: it may contain cookies, sessions, usernames and
encrypted proxy passwords. The release ZIP must never be repacked from live data.

## Runtime safety

Browsers launch as native processes without Playwright, WebDriver or CDP.
Proxy credentials remain behind the manager's loopback bridge and never enter
browser arguments. Proxy failures never silently fall back to Direct. Stop all
profiles before exiting; Starting/Running/Stopping/ExternalRunning/Unknown
states block normal exit and unsafe network edits. Stop timeout does not force
kill a browser or release its bridge prematurely. If the manager is forcibly
terminated, close any leftover browser manually before reopening its profile.

## Build and release gate

Developers: run `scripts/build-portable.ps1` with the pinned repository tools.
The packager includes only the EXE and this README, never live data or engines.
Publication requires real Explorer launch, official download, native lifecycle,
folder relocation, isolation, and cleanup acceptance; compilation/hash checks
alone are not a release PASS. Development-context SxS failures must not be
treated as ordinary Explorer results.
