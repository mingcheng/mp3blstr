// Package ui renders mp3blstr with tview/tcell: a single root primitive
// draws the classic screen layout, handles the keyboard and maps mouse
// clicks, double clicks, right clicks and the wheel onto commands.
package ui

import (
	"time"

	"github.com/rivo/tview"

	"github.com/mingcheng/mp3blstr/internal/app"
	"github.com/mingcheng/mp3blstr/internal/listview"
	"github.com/mingcheng/mp3blstr/internal/ui/theme"
)

// Options configure the user interface.
type Options struct {
	Theme    theme.Theme
	NerdFont bool
	Mouse    bool
	// ListWidth is the width of the main window including its border;
	// the remaining columns go to the information panel.
	ListWidth listview.Width
}

// UI is the terminal user interface. It implements app.View.
type UI struct {
	tapp   *tview.Application
	screen *Screen
	app    *app.App
}

// New creates the interface. Call Attach before Run.
func New(opts Options) *UI {
	u := &UI{
		tapp:   tview.NewApplication(),
		screen: newScreen(opts.Theme, theme.For(opts.Theme, opts.NerdFont), opts.NerdFont, opts.ListWidth),
	}
	u.tapp.EnableMouse(opts.Mouse)
	u.tapp.SetRoot(u.screen, true)
	return u
}

// Attach connects the application state.
func (u *UI) Attach(a *app.App) {
	u.app = a
	u.screen.app = a
}

// Post runs f on the UI goroutine and redraws the screen. It blocks until
// f has run, so it must not be called on the UI goroutine itself.
func (u *UI) Post(f func()) { u.tapp.QueueUpdateDraw(f) }

// Beep rings the terminal bell.
func (u *UI) Beep() {
	if u.screen.scr != nil {
		_ = u.screen.scr.Beep()
	}
}

// Sync repaints the whole terminal.
func (u *UI) Sync() { u.tapp.Sync() }

// Quit ends Run.
func (u *UI) Quit() { u.tapp.Stop() }

// Run starts the event loop; it returns when the program quits.
func (u *UI) Run() error {
	done := make(chan struct{})
	defer close(done)
	go u.tick(done)
	return u.tapp.Run()
}

// tick updates the play time periodically until done is closed. The screen
// is only redrawn when something changed, so an idle player uses no CPU.
func (u *UI) tick(done <-chan struct{}) {
	t := time.NewTicker(u.app.TickInterval())
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			u.tapp.QueueUpdate(func() {
				if u.app.Tick() {
					u.tapp.ForceDraw() // safe within a queued update
				}
			})
		}
	}
}
