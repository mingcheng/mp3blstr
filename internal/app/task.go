package app

import (
	"context"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/mingcheng/mp3blasterr/internal/keys"
)

// Task is a long running operation (such as scanning directories) that runs
// in the background while a progress dialog is shown. The dialog blocks all
// other input; Esc or 'c' cancels the operation.
type Task struct {
	Title    string
	Detail   string    // what is being worked on, e.g. the current directory
	Status   string    // counters, e.g. "120 audio files in 14 folders"
	Fraction float64   // estimated part done, 0..1
	Started  time.Time // for the elapsed time

	cancel context.CancelFunc
}

// TaskProgress is reported by the work function of a task.
type TaskProgress struct {
	Detail   string
	Status   string
	Fraction float64
}

// taskReportInterval limits how often progress is passed to the UI.
const taskReportInterval = 50 * time.Millisecond

// startTask runs work on its own goroutine and shows a progress dialog until
// it ends. done runs on the UI goroutine with the result, unless the task
// was cancelled or failed. work must not touch the application state; it
// reports its progress through report and must stop when ctx is cancelled.
func startTask[T any](a *App, title string, work func(ctx context.Context, report func(TaskProgress)) (T, error), done func(T)) {
	ctx, cancel := context.WithCancel(context.Background())
	t := &Task{Title: title, Started: time.Now(), cancel: cancel}
	a.Task = t
	a.Input = InputTask
	a.SetMessage("Press Esc to cancel")
	go func() {
		var last time.Time
		report := func(p TaskProgress) {
			if now := time.Now(); now.Sub(last) >= taskReportInterval {
				last = now
				a.view.Post(func() {
					if a.Task == t {
						t.Detail, t.Status, t.Fraction = p.Detail, p.Status, p.Fraction
					}
				})
			}
		}
		res, err := work(ctx, report)
		a.view.Post(func() {
			if a.Task != t {
				return // cancelled; the result is discarded
			}
			a.endTask()
			if err != nil {
				a.Warning(t.Title + " failed: " + err.Error())
				return
			}
			a.Msg = Message{}
			done(res)
		})
	}()
}

func (a *App) endTask() {
	if a.Task != nil {
		a.Task.cancel()
	}
	a.Task = nil
	a.Input = InputDefault
}

// CancelTask stops the running task. The dialog closes immediately, even if
// the work is stuck in a slow system call; its result is thrown away.
func (a *App) CancelTask() {
	if a.Task == nil {
		return
	}
	title := a.Task.Title
	a.endTask()
	a.SetMessage(title + ": cancelled.")
}

func (a *App) taskKey(k keys.Key) {
	switch {
	case k.Code == tcell.KeyEsc, k.Code == tcell.KeyRune && (k.Rune == 'c' || k.Rune == 'C'):
		a.CancelTask()
	default:
		a.view.Beep()
	}
}
