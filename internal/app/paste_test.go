package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/kballard/go-shellquote"

	"github.com/mingcheng/mp3blstr/internal/keys"
)

func TestPasteNeverExecutesCommands(t *testing.T) {
	a, _ := newTestApp(t)
	a.AddFiles([]string{"song.mp3"})
	a.HandlePaste("Cq5\n")
	if a.Cur.Len() != 1 || a.Player.Playing() || a.Popup != nil || !a.Msg.Error {
		t.Fatal("paste outside a text dialog must not execute commands")
	}
	a.openConfirm([]string{"Delete?"}, func() { t.Fatal("paste confirmed an operation") })
	a.HandlePaste("y\n")
	if a.Input != InputConfirm {
		t.Fatal("paste changed confirmation state")
	}
	a.ClosePopup()
	press(a, 'a')
	a.HandlePaste("'one song.mp3'\n'另一首.mp3'\t")
	if a.Input != InputText || a.Popup.Editor.Text() != "'one song.mp3' '另一首.mp3' " {
		t.Fatalf("paste text: %+v", a.Popup)
	}
	for _, bad := range []string{"\x1b[31m", "\x00", "\xff"} {
		before := a.Popup.Editor.Text()
		a.HandlePaste(bad)
		if !a.Msg.Error || a.Popup.Editor.Text() != before {
			t.Fatal("control/invalid input must be rejected atomically")
		}
	}
	a.openPopup([]string{"Short"}, "", 2, func(string) {})
	a.HandlePaste("long")
	if !a.Msg.Error || a.Popup.Editor.Text() != "" {
		t.Fatal("overlong paste must not be silently truncated")
	}
}

func TestAddPathsQuotedAndAtomic(t *testing.T) {
	a, dir := newTestApp(t)
	for _, name := range []string{"空 格.mp3", "it's music.mp3", "$(literal).mp3"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		press(a, 'a')
		a.HandlePaste(shellquote.Join(path))
		press(a, tcell.KeyEnter)
		if a.Msg.Error || a.Cur.Item(a.Cur.Len()-1).Path != path {
			t.Fatalf("adding %q: %s", path, a.Msg.Text)
		}
	}
	before := a.Cur.Len()
	a.addPaths(shellquote.Join(filepath.Join(dir, "a.mp3"), filepath.Join(dir, "missing.mp3")))
	if !a.Msg.Error || a.Cur.Len() != before {
		t.Fatal("invalid batch must add nothing")
	}
	for _, bad := range []string{"", "'", shellquote.Join(filepath.Join(dir, "c.txt")), shellquote.Join(dir)} {
		a.addPaths(bad)
		if !a.Msg.Error || a.Cur.Len() != before {
			t.Fatalf("invalid paths accepted: %q", bad)
		}
	}
	a.addPaths(shellquote.Join(filepath.Join(dir, "a.mp3"), filepath.Join(dir, "b.mp3")))
	if a.Cur.Len() != before+2 {
		t.Fatal("multiple paths were not added")
	}
}

func TestTrashConfirmationAndFailure(t *testing.T) {
	a, dir := newTestApp(t)
	a.Opts.UseTrash = true
	path := filepath.Join(dir, "a.mp3")
	calls := 0
	a.trash = func(got string) error {
		calls++
		if got != path {
			t.Fatalf("Trash path %q", got)
		}
		return errors.New("volume is read-only")
	}
	press(a, tcell.KeyF1)
	a.MainView().Set(a.fm.Find("a.mp3"), a.MainLen())
	press(a, 'D')
	if !strings.Contains(a.Popup.Labels[0], "Trash") || calls != 0 {
		t.Fatal("Trash must be confirmed")
	}
	press(a, 'y')
	if calls != 1 || !a.Msg.Error || !strings.Contains(a.Msg.Text, "read-only") {
		t.Fatalf("Trash failure was not surfaced: %s", a.Msg.Text)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("Trash failure must not fall back to permanent removal")
	}
	a.trash = func(string) error { return nil }
	a.deleteFile(path)
	if a.Msg.Error || a.Msg.Text != "File moved to the Trash." {
		t.Fatalf("Trash success message: %s", a.Msg.Text)
	}
}

func TestIdleAndTaskTickIntervals(t *testing.T) {
	a, _ := newTestApp(t)
	if got := a.TickInterval(); got != 0 {
		t.Fatalf("idle polling interval = %v, want disabled", got)
	}
	a.Task = &Task{}
	want := time.Duration(a.Opts.FPL) * 2 * mpegFrameDuration
	if got := a.TickInterval(); got != want {
		t.Fatalf("task interval = %v, want %v", got, want)
	}
	a.Task = nil
	a.Exec(keys.CmdPlayStop)
	if a.TickInterval() != 0 {
		t.Fatal("stopped player must not poll")
	}
}
