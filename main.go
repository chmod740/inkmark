package main

import (
	"embed"
	"fmt"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	app := NewApp()
	title := appTitle
	aboutMessage := "版本 " + appVersion + "\n作者 " + appAuthor
	if app.currentLocale() == "en" {
		title = appTitleEnglish
		aboutMessage = "Version " + appVersion + "\nAuthor " + appAuthor
	}
	err := wails.Run(&options.App{
		Title:     title,
		Width:     1440,
		Height:    920,
		MinWidth:  940,
		MinHeight: 640,
		Menu:      app.applicationMenu(),
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 236, G: 238, B: 233, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "com.inkmark.editor.single-instance",
			OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
				app.handleSecondInstance(data.Args, data.WorkingDirectory)
			},
		},
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarDefault(),
			About: &mac.AboutInfo{
				Title:   title,
				Message: aboutMessage,
				Icon:    appIcon,
			},
			OnFileOpen: app.openFileFromOS,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
	if err != nil {
		fmt.Println("启动失败:", err)
	}
}
