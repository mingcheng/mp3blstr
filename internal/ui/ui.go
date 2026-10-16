// Package ui renders mp3blstr with tview/tcell: a single root primitive
// draws the classic screen layout, handles the keyboard and maps mouse
// clicks, double clicks, right clicks and the wheel onto commands.
package ui

import (
	"errors"
	"os"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
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
	tapp         *tview.Application
	screen       *Screen
	app          *app.App
	tickWake     chan struct{}
	tickInterval atomic.Int64
	pointer      pointerProtocol
}

// New creates the interface. Call Attach before Run.
func New(opts Options) *UI {
	u := &UI{
		tapp:     tview.NewApplication(),
		screen:   newScreen(opts.Theme, theme.For(opts.Theme, opts.NerdFont), opts.NerdFont, opts.ListWidth),
		tickWake: make(chan struct{}, 1),
	}
	u.tapp.EnableMouse(opts.Mouse)
	if opts.Mouse {
		u.pointer = detectPointer(os.Getenv)
	}
	u.tapp.EnablePaste(true)
	u.tapp.SetRoot(u.screen, true)
	u.tapp.SetAfterDrawFunc(func(tcell.Screen) { u.scheduleTick() })
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
	var pointer *pointerScreen
	if u.pointer != pointerUnsupported {
		scr, err := tcell.NewScreen()
		if err != nil {
			return err
		}
		pointer = &pointerScreen{Screen: scr, protocol: u.pointer}
		// SetScreen calls Init but discards its error.
		u.tapp.SetScreen(pointer)
		if pointer.initErr != nil {
			return pointer.initErr
		}
		pointer.EnableMouse()
		pointer.EnablePaste()
	}
	done := make(chan struct{})
	defer close(done)
	u.scheduleTick()
	go u.tick(done)
	err := u.tapp.Run()
	if pointer != nil {
		err = errors.Join(err, pointer.pointerError())
	}
	return err
}

// tick disables its timer while idle and redraws only when something changed.
func (u *UI) tick(done <-chan struct{}) {
	t := time.NewTimer(time.Hour)
	t.Stop()
	defer t.Stop()
	for {
		var ticks <-chan time.Time
		if interval := time.Duration(u.tickInterval.Load()); interval > 0 {
			t.Reset(interval)
			ticks = t.C
		}
		select {
		case <-done:
			return
		case <-u.tickWake:
			t.Stop()
		case <-ticks:
			u.tapp.QueueUpdate(func() {
				if u.app.Tick() {
					u.tapp.ForceDraw() // safe within a queued update
				}
			})
		}
	}
}

func (u *UI) scheduleTick() {
	if u.app == nil {
		return
	}
	interval := int64(u.app.TickInterval())
	if u.tickInterval.Swap(interval) == interval {
		return
	}
	select {
	case u.tickWake <- struct{}{}:
	default:
	}
}
