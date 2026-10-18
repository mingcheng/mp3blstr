package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/mingcheng/mp3blstr/internal/ui/theme"
)

// Post must run its function on the event loop while it is running, and
// must neither run it nor block once the loop has ended: background
// goroutines (media keys, song loading) may still post during shutdown.
func TestPostAfterRunReturns(t *testing.T) {
	u := New(Options{Theme: theme.Classic()})
	u.Attach(newRenderApp(t, nullBackend{}, u))
	sim := tcell.NewSimulationScreen("UTF-8")
	sim.SetSize(80, 24)
	u.tapp.SetScreen(sim)

	errc := make(chan error, 1)
	go func() { errc <- u.Run() }()

	ran := false
	u.Post(func() { ran = true })
	if !ran {
		t.Fatal("Post returned before running its function")
	}

	u.Post(u.Quit)
	select {
	case err := <-errc:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Quit")
	}

	done := make(chan struct{})
	late := make(chan struct{}, 1)
	go func() {
		u.Post(func() { late <- struct{}{} })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Post blocked after the event loop ended")
	}
	select {
	case <-late:
		t.Fatal("Post ran its function after the event loop ended")
	default:
	}
}
