package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/mingcheng/mp3blstr/internal/app"
	"github.com/mingcheng/mp3blstr/internal/config"
	"github.com/mingcheng/mp3blstr/internal/keys"
	"github.com/mingcheng/mp3blstr/internal/media"
	"github.com/mingcheng/mp3blstr/internal/ui/theme"
)

func runningTestUI(t *testing.T) (*UI, *app.App, <-chan struct{}) {
	t.Helper()
	u := New(Options{Theme: theme.Classic()})
	matcher, err := media.NewMatcher(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := app.New(app.Deps{
		Options: config.Default(), Keys: keys.Default(), Matcher: matcher,
		Backend: nullBackend{}, View: u, StartDir: t.TempDir(),
	})
	u.Attach(a)
	screen := tcell.NewSimulationScreen("UTF-8")
	u.tapp.SetScreen(screen)
	draws := make(chan struct{}, 32)
	u.tapp.SetAfterDrawFunc(func(tcell.Screen) {
		u.scheduleTick()
		select {
		case draws <- struct{}{}:
		default:
		}
	})
	finished := make(chan error, 1)
	go func() { finished <- u.Run() }()
	t.Cleanup(func() {
		u.Quit()
		select {
		case err := <-finished:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(time.Second):
			t.Error("UI failed to stop")
		}
	})
	select {
	case <-draws:
	case <-time.After(time.Second):
		t.Fatal("UI did not start")
	}
	return u, a, draws
}

func TestBracketedPasteAndIdleScheduling(t *testing.T) {
	u, a, _ := runningTestUI(t)
	u.Post(func() { a.Exec(keys.CmdAddPaths) })
	u.tapp.QueueEvent(tcell.NewEventPaste(true))
	for _, r := range "qC5\nsong.mp3" {
		if r == '\n' {
			u.tapp.QueueEvent(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
		} else {
			u.tapp.QueueEvent(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone))
		}
	}
	u.tapp.QueueEvent(tcell.NewEventPaste(false))
	deadline := time.Now().Add(time.Second)
	for {
		var text string
		u.Post(func() {
			if a.Popup != nil && a.Popup.Editor != nil {
				text = a.Popup.Editor.Text()
			}
		})
		if text == "qC5 song.mp3" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("paste was not routed intact: %q", text)
		}
		time.Sleep(time.Millisecond)
	}
	u.Post(func() {
		if a.Player.Playing() || a.Cur.Len() != 0 || !strings.Contains(a.Popup.Editor.Text(), "song.mp3") {
			t.Error("pasted text executed a command")
		}
	})
	if u.tickInterval.Load() != 0 {
		t.Fatal("idle UI scheduled periodic work")
	}
}

func TestTaskTimerRestartsAndStops(t *testing.T) {
	u, a, draws := runningTestUI(t)
	drain := func() {
		for {
			select {
			case <-draws:
			default:
				return
			}
		}
	}
	for cycle := 0; cycle < 2; cycle++ {
		u.Post(func() {
			a.Opts.FPL = 1
			a.Task = &app.Task{}
		})
		if got := time.Duration(u.tickInterval.Load()); got != 52*time.Millisecond {
			t.Fatalf("task timer interval = %v", got)
		}
		drain()
		for i := 0; i < 2; i++ {
			select {
			case <-draws:
			case <-time.After(time.Second):
				t.Fatal("task timer did not redraw")
			}
		}
		u.Post(func() { a.Task = nil })
		if u.tickInterval.Load() != 0 {
			t.Fatal("completed task left a periodic timer enabled")
		}
		drain()
		select {
		case <-draws:
			t.Fatal("idle UI redrew without an event")
		case <-time.After(150 * time.Millisecond):
		}
	}
}
