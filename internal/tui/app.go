package tui

import (
	"github.com/dandygardad/lazytfs/internal/tfs"
	"github.com/rivo/tview"
)

type App struct {
	tviewApp  *tview.Application
	tfsClient *tfs.Client
	layout    *Layout
}

func NewApp() *App {
	tApp := tview.NewApplication()
	client := tfs.NewClient()

	app := &App{
		tviewApp:  tApp,
		tfsClient: client,
	}
	app.layout = NewLayout(app)
	return app
}

func (a *App) Run() error {
	a.tviewApp.SetRoot(a.layout.Pages, true).EnableMouse(true)
	// Refresh asynchronously so UI loads instantly
	go func() {
		a.tviewApp.QueueUpdateDraw(func() {
			a.layout.RefreshAll()
		})
	}()
	return a.tviewApp.Run()
}
