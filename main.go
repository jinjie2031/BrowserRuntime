//go:build windows

package main

import (
	"context"
	"embed"
	"path/filepath"
	_ "time/tzdata" // Standalone Windows distributions cannot rely on GOROOT.
	"unsafe"

	"browserruntime/internal/enginebootstrap"
	"browserruntime/internal/portable"
	"golang.org/x/sys/windows"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	windowsoptions "github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	layout, err := portable.Current()
	if err != nil || layout.Prepare() != nil {
		startupError(portable.ErrUnavailable.Error() + "\n\nPortable 目录不可用或不可写。请将整个 BrowserRuntime 文件夹移动到可写的本地目录。")
		return
	}
	bootstrap := enginebootstrap.New(filepath.Join(layout.Engines, "cloakbrowser", enginebootstrap.Version))
	defer bootstrap.Shutdown()
	go bootstrap.Check()
	uiLanguage := &UILanguage{}
	service, err := buildPortableService(layout, bootstrap)
	if err != nil {
		startupError("BrowserRuntime could not open its portable data. No alternate data location was used.\n\n无法打开 Portable 数据目录，未使用其他存储位置。")
		return
	}
	if err := wails.Run(&options.App{
		Title:            "Browser Runtime",
		Width:            1080,
		Height:           720,
		MinWidth:         760,
		MinHeight:        520,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 246, G: 248, B: 250, A: 1},
		Bind:             []interface{}{service, uiLanguage, &EngineBootstrap{bootstrap}},
		Windows:          &windowsoptions.Options{WebviewUserDataPath: layout.WebView},
		OnBeforeClose: closeGuard(service.PrepareClose, func(ctx context.Context) {
			title, message := uiLanguage.exitWarning()
			_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
				Type:    runtime.WarningDialog,
				Title:   title,
				Message: message,
			})
		}),
	}); err != nil {
		startupError("BrowserRuntime desktop could not start. Microsoft Edge WebView2 Runtime is required.\n\n桌面窗口启动失败，请确认 Microsoft Edge WebView2 Runtime 可用。")
	}
}

// Startup failures before Wails can display a window must still be visible.
// Only fixed, non-secret messages enter this native dialog.
func startupError(message string) {
	title, _ := windows.UTF16PtrFromString("BrowserRuntime Portable")
	text, _ := windows.UTF16PtrFromString(message)
	_, _, _ = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
