package app

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/mingcheng/mp3blstr/internal/audio"
	"github.com/mingcheng/mp3blstr/internal/config"
	"github.com/mingcheng/mp3blstr/internal/filemanager"
	"github.com/mingcheng/mp3blstr/internal/keys"
	"github.com/mingcheng/mp3blstr/internal/media"
	"github.com/mingcheng/mp3blstr/internal/playback"
	"github.com/mingcheng/mp3blstr/internal/playlist"
)

type fakeView struct {
	quit, beeps int

	mu    sync.Mutex
	queue []func() // functions posted to the UI goroutine
}

func (v *fakeView) Post(f func()) {
	v.mu.Lock()
	v.queue = append(v.queue, f)
	v.mu.Unlock()
}
func (v *fakeView) Beep() { v.beeps++ }
func (v *fakeView) Sync() {}
func (v *fakeView) Quit() { v.quit++ }

// runPosted runs the functions posted so far, like the UI event loop.
func (v *fakeView) runPosted() {
	v.mu.Lock()
	q := v.queue
	v.queue = nil
	v.mu.Unlock()
	for _, f := range q {
		f()
	}
}

// waitTask runs the UI loop until the background task has finished.
func waitTask(t *testing.T, a *App) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for a.Task != nil {
		if time.Now().After(deadline) {
			t.Fatal("task did not finish")
		}
		a.view.(*fakeView).runPosted()
		time.Sleep(time.Millisecond)
	}
}

type noBackend struct{}

func (noBackend) Opener(string) func() (playback.Track, error) {
	return func() (playback.Track, error) { return nil, os.ErrNotExist }
}

func newTestApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"a.mp3", "b.mp3", "c.txt", "Album/x.mp3", "Album/y.mp3"} {
		p := filepath.Join(dir, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	opts := config.Default()
	opts.PlaylistDir = dir
	m, _ := media.NewMatcher(nil, nil)
	a := New(Deps{Options: opts, Keys: keys.Default(), Matcher: m, Backend: noBackend{}, View: &fakeView{}, StartDir: dir})
	return a, dir
}

func press(a *App, ks ...any) {
	for _, k := range ks {
		switch k := k.(type) {
		case rune:
			a.HandleKey(keys.Rune(k))
		case tcell.Key:
			a.HandleKey(keys.Special(k))
		case string:
			for _, r := range k {
				a.HandleKey(keys.Rune(r))
			}
		}
	}
}

func TestSelectFilesAcrossDirectories(t *testing.T) {
	a, _ := newTestApp(t)
	press(a, tcell.KeyF1)
	if a.Mode != keys.ModeFileSelection {
		t.Fatal("F1 should enter file mode")
	}
	// ../ ./ Album/ a.mp3 b.mp3 c.txt
	a.MainView().Set(3, a.MainLen())
	press(a, ' ')
	a.MainView().Set(2, a.MainLen())
	press(a, tcell.KeyEnter) // into Album/
	if !strings.HasSuffix(a.MainTitle(), "Album"+string(filepath.Separator)) {
		t.Fatalf("title %q", a.MainTitle())
	}
	a.MainView().Set(2, a.MainLen()) // x.mp3
	press(a, ' ', tcell.KeyF1)
	if a.Mode != keys.ModeNormal || a.Cur.Len() != 2 {
		t.Fatalf("added %d songs", a.Cur.Len())
	}
	if filepath.Base(a.Cur.Item(0).Path) != "a.mp3" || filepath.Base(a.Cur.Item(1).Path) != "x.mp3" {
		t.Fatal("wrong songs added")
	}
}

func TestGroupsTitlesAndPlaylistRoundTrip(t *testing.T) {
	a, dir := newTestApp(t)
	press(a, tcell.KeyF1, tcell.KeyF5) // dirs as groups
	waitTask(t, a)
	if a.Cur.Len() != 1 || a.Cur.Item(0).Name() != "[Album]" {
		t.Fatalf("dirs as groups: %d", a.Cur.Len())
	}
	press(a, tcell.KeyF5, "Best", tcell.KeyEnter)
	if a.Cur.Item(0).Name() != "[Best]" {
		t.Fatalf("title %q", a.Cur.Item(0).Name())
	}
	press(a, tcell.KeyEnter) // enter the group
	if a.Cur.Title != "[Best]" || a.MainRow(0).Kind != RowParent {
		t.Fatal("enter group")
	}
	press(a, tcell.KeyEnter) // [..]
	if a.Cur != a.Root {
		t.Fatal("[..] should go back")
	}
	press(a, tcell.KeyF4, "saved", tcell.KeyEnter)
	if _, err := os.Stat(filepath.Join(dir, "saved.m3u")); err != nil {
		t.Fatal("playlist not written")
	}
	press(a, 'C')
	if a.Root.Len() != 0 {
		t.Fatal("clear playlist")
	}
	if !a.LoadPlaylist(filepath.Join(dir, "saved.m3u")) || a.Root.Len() != 1 || a.Root.SubGroup(0).Len() != 3 {
		t.Fatal("reload failed")
	}
}

func TestTogglesAndHelp(t *testing.T) {
	a, _ := newTestApp(t)
	press(a, tcell.KeyF6, tcell.KeyF7, tcell.KeyF8)
	if !a.Repeat() || !a.Shuffle() || a.Player.Mode != playback.PlayGroup {
		t.Fatal("toggles")
	}
	press(a, '?')
	if a.Mode != keys.ModeHelp || a.MainLen() == 0 {
		t.Fatal("help")
	}
	press(a, '?')
	if a.Mode != keys.ModeNormal {
		t.Fatal("leave help")
	}
	press(a, '+')
	if a.HelpBar.Offset == 0 {
		t.Fatal("help bar should scroll")
	}
	press(a, 'q')
	if a.Input != InputConfirm || a.view.(*fakeView).quit != 0 {
		t.Fatal("quit should ask for confirmation")
	}
	press(a, 'n')
	if a.Input != InputDefault || a.view.(*fakeView).quit != 0 {
		t.Fatal("quit should be cancelled")
	}
	press(a, 'q', 'y')
	if a.view.(*fakeView).quit != 1 {
		t.Fatal("quit")
	}
}

func TestQuitWithoutConfirmation(t *testing.T) {
	a, _ := newTestApp(t)
	a.Opts.ConfirmQuit = false
	press(a, 'q')
	if a.view.(*fakeView).quit != 1 {
		t.Fatal("quit")
	}
}

func TestDisplayModesSavedOnToggle(t *testing.T) {
	a, _ := newTestApp(t)
	press(a, 'f')
	if a.Opts.DisplayMode != playlist.DisplayID3 || a.groupView(a.Root).display != playlist.DisplayID3 {
		t.Fatal("playlist display mode was not updated")
	}
	press(a, tcell.KeyF1, 'f')
	if a.Opts.FileDisplayMode != filemanager.DisplayID3 {
		t.Fatal("file display mode was not updated")
	}
	a.CaptureState()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.SaveSettings(path, config.ChangedSettings(config.Default().StateSettings(), a.Opts.StateSettings())); err != nil {
		t.Fatal(err)
	}
	loaded := config.Default()
	if err := loaded.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	if loaded.DisplayMode != playlist.DisplayID3 || loaded.FileDisplayMode != filemanager.DisplayID3 {
		t.Fatalf("restored modes: playlist=%v file=%v", loaded.DisplayMode, loaded.FileDisplayMode)
	}
}

func TestRemoveDuplicateFilesCommand(t *testing.T) {
	a, _ := newTestApp(t)
	group := playlist.NewGroup("[Album]")
	a.Root.AddGroup(group)
	group.AddSong("/music/a.mp3")
	a.AddFiles([]string{"/music/b.mp3", "/music/a.mp3", "/music/b.mp3"})
	a.MainView().Set(3, a.MainLen())
	press(a, 'D')
	if group.Len() != 2 || a.Root.Len() != 2 || a.Root.Item(1).Path != "/music/b.mp3" {
		t.Fatalf("duplicates not removed across groups: group=%d root=%d", group.Len(), a.Root.Len())
	}
	if a.MainView().Cursor >= a.MainLen() || a.Msg.Text != "Removed 2 duplicate files" {
		t.Fatalf("cursor=%d message=%q", a.MainView().Cursor, a.Msg.Text)
	}
	press(a, 'D')
	if a.Msg.Text != "Removed 0 duplicate files" {
		t.Fatalf("second pass: %q", a.Msg.Text)
	}
	a.Root.AddSong("/music/a.mp3")
	a.Player.Play()
	press(a, 'D')
	if a.Root.Len() != 3 || !a.Msg.Error {
		t.Fatal("cannot remove duplicates during playback")
	}
}

func TestPlaylistRowDetails(t *testing.T) {
	a, dir := newTestApp(t)
	path := filepath.Join(dir, "tone.wav")
	data := make([]byte, 44+32000)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 8000)
	binary.LittleEndian.PutUint32(data[28:], 16000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], 32000)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	a.AddFiles([]string{path, "https://example.com/live"})
	if got := a.MainRow(0).Right; got != filemanager.SizeDesc(int64(len(data))) {
		t.Fatalf("filename mode size = %q", got)
	}
	if got := a.MainRow(1).Right; got != "" {
		t.Fatalf("stream size = %q", got)
	}
	press(a, 'f')
	if got := a.MainRow(0).Right; got != "" {
		t.Fatalf("duration should load asynchronously, got %q", got)
	}
	deadline := time.Now().Add(5 * time.Second)
	for a.MainRow(0).Right == "" && time.Now().Before(deadline) {
		a.view.(*fakeView).runPosted()
		time.Sleep(time.Millisecond)
	}
	if got := a.MainRow(0).Right; got != "0:02" {
		t.Fatalf("ID3 mode duration = %q", got)
	}
	if got := a.MainRow(1).Right; got != "" {
		t.Fatalf("stream duration = %q", got)
	}
	press(a, 'f')
	if got := a.MainRow(0).Right; got != "" {
		t.Fatalf("path mode detail = %q", got)
	}
}

func TestSearchAndPopupEditing(t *testing.T) {
	a, _ := newTestApp(t)
	press(a, tcell.KeyF1, '/', "b.")
	if a.MainView().Cursor != 4 || a.Msg.Text != "Search: b." {
		t.Fatalf("search cursor=%d msg=%q", a.MainView().Cursor, a.Msg.Text)
	}
	press(a, 'z')
	if a.view.(*fakeView).beeps != 1 {
		t.Fatal("no match should beep")
	}
	press(a, tcell.KeyEnter)

	press(a, 'S')
	e := a.Popup.Editor
	press(a, "abc", tcell.KeyLeft, tcell.KeyLeft, 'X') // replace mode
	if e.Text() != "aXc" {
		t.Fatalf("replace: %q", e.Text())
	}
	press(a, tcell.KeyUp, 'Y') // insert mode
	if e.Text() != "aXYc" || a.Msg.Text != "--  INSERT --" {
		t.Fatalf("insert: %q", e.Text())
	}
	press(a, tcell.KeyEsc)
	press(a, 'S', "*.mp3", tcell.KeyEnter)
	if !a.MainRow(3).Marked || !a.MainRow(4).Marked || a.MainRow(5).Marked {
		t.Fatal("pattern selection")
	}
}

func TestFileDeleteNeedsConfirmation(t *testing.T) {
	a, dir := newTestApp(t)
	target := filepath.Join(dir, "a.mp3")
	press(a, tcell.KeyF1)
	// ../ ./ Album/ a.mp3 b.mp3 c.txt
	a.MainView().Set(3, a.MainLen())

	for _, cancel := range []any{'n', tcell.KeyEsc, tcell.KeyEnter} {
		press(a, 'D')
		if a.Input != InputConfirm || a.Popup == nil || !a.Popup.Confirm {
			t.Fatal("delete should ask for confirmation")
		}
		press(a, 'x') // ignored
		if a.Input != InputConfirm {
			t.Fatal("unrelated key should keep the dialog open")
		}
		press(a, cancel)
		if a.Input != InputDefault || a.Popup != nil {
			t.Fatalf("%v should close the dialog", cancel)
		}
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("%v should not delete the file", cancel)
		}
	}

	press(a, 'D', 'y')
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("file should be deleted after confirming")
	}
	if a.Input != InputDefault || a.Msg.Text != "File deleted." {
		t.Fatalf("after delete: input=%v msg=%q", a.Input, a.Msg.Text)
	}
}

func TestThemeAndNerdFontToggles(t *testing.T) {
	a, _ := newTestApp(t)
	for _, m := range []keys.Mode{keys.ModeNormal, keys.ModeFileSelection, keys.ModeHelp} {
		found := map[keys.Command]bool{}
		for _, b := range a.Keys.ForMode(m) {
			found[b.Cmd] = true
		}
		if !found[keys.CmdToggleTheme] || !found[keys.CmdToggleNerdFont] {
			t.Fatalf("mode %d: toggles missing from the help bar", m)
		}
	}
	press(a, 'T')
	if a.Opts.Theme != "modern" || a.Msg.Text != "Theme: modern" {
		t.Fatalf("theme %q msg %q", a.Opts.Theme, a.Msg.Text)
	}
	press(a, 'T')
	if a.Opts.Theme != "classic" {
		t.Fatalf("theme should wrap around, got %q", a.Opts.Theme)
	}
	press(a, 'I')
	if !a.Opts.NerdFont || a.Msg.Text != "Nerd Font icons enabled" {
		t.Fatal("nerd font on")
	}
	a.Click(keys.CmdToggleNerdFont)
	if a.Opts.NerdFont {
		t.Fatal("nerd font off via click")
	}
}

func TestRepeatShuffleAndHelpBarCategories(t *testing.T) {
	a, _ := newTestApp(t)
	a.Mixer = audio.NewMixer()
	a.fadeMixer = a.Mixer
	a.Mixer.SetFade(a.Opts.FadeOut)
	press(a, 'V')
	if !a.Opts.FadeOut || !a.Mixer.Fade() {
		t.Fatal("V should enable fade out")
	}
	press(a, 'V')
	if a.Opts.FadeOut || a.Mixer.Fade() {
		t.Fatal("V should disable fade out")
	}
	press(a, tcell.KeyF1) // file mode: R/Z work everywhere
	press(a, 'R', 'Z')
	if !a.Repeat() || !a.Shuffle() {
		t.Fatal("R/Z should toggle repeat/shuffle in file mode")
	}
	press(a, tcell.KeyF1, tcell.KeyF6, tcell.KeyF7) // F6/F7 still work
	if a.Repeat() || a.Shuffle() {
		t.Fatal("F6/F7 should toggle repeat/shuffle")
	}

	lines := a.HelpBarLines()
	if lines[0][0].Desc != "Select Files" || lines[3][0].Desc != "Delete" || len(lines[3]) != 1 {
		t.Fatal("each category should start on a new line")
	}
	if lines[4][0].Cmd != keys.CmdToggleRepeat || lines[4][1].Cmd != keys.CmdToggleShuffle {
		t.Fatal("playback category should follow the playlist")
	}

	ref := strings.Join(a.help, "\n")
	for _, want := range []string{"Key bindings", "  R     Toggle Repeat", "  F6    Toggle Repeat        (Normal Mode)",
		"  spc   Select File          (Normal/File Mode)", "  dwn   Move Scrolbar Down"} {
		if !strings.Contains(ref, want) {
			t.Errorf("key reference lacks %q", want)
		}
	}
}

func TestFadeOutWithoutVisibleMixer(t *testing.T) {
	opts := config.Default()
	gains := audio.NewMixer()
	a := New(Deps{Options: opts, Keys: keys.Default(), FadeMixer: gains, Backend: noBackend{}, View: &fakeView{}})
	if a.Mixer != nil || gains.Fade() || opts.FadeOut {
		t.Fatal("fade should start disabled without a visible mixer")
	}
	a.HandleKey(keys.FromEvent(tcell.NewEventKey(tcell.KeyRune, 'V', tcell.ModShift)))
	if !opts.FadeOut || !gains.Fade() || a.Msg.Text != "Fade out enabled" {
		t.Fatalf("toggle on: enabled=%v mixer=%v message=%q", opts.FadeOut, gains.Fade(), a.Msg.Text)
	}
	a.HandleKey(keys.FromEvent(tcell.NewEventKey(tcell.KeyRune, 'V', tcell.ModShift)))
	if opts.FadeOut || gains.Fade() || a.Msg.Text != "Fade out disabled" {
		t.Fatalf("toggle off: enabled=%v mixer=%v message=%q", opts.FadeOut, gains.Fade(), a.Msg.Text)
	}
}

func TestSessionStateRoundTrip(t *testing.T) {
	a, dir := newTestApp(t)
	press(a, tcell.KeyF1)
	a.MainView().Set(2, a.MainLen()) // Album/
	press(a, tcell.KeyEnter)
	a.MainView().Set(2, a.MainLen()) // x.mp3
	press(a, ' ', tcell.KeyF1, 'R', 'Z')
	song := filepath.Join(dir, "Album", "x.mp3")
	a.Player.Resume(song, 75)

	a.Quit()
	if a.Opts.LastDir != filepath.Join(dir, "Album") || !a.Opts.Repeat {
		t.Fatalf("captured dir=%q repeat=%v", a.Opts.LastDir, a.Opts.Repeat)
	}
	if a.Opts.LastSong != song || a.Opts.LastPosition != 75 {
		t.Fatalf("captured song=%q pos=%d", a.Opts.LastSong, a.Opts.LastPosition)
	}
	session := filepath.Join(t.TempDir(), "session.lst")
	if err := a.WriteSession(session); err != nil {
		t.Fatal(err)
	}

	b, _ := newTestApp(t)
	b.Opts.LastSong, b.Opts.LastPosition = song, 75
	b.RestoreSession(session)
	if b.Root.Len() != 1 || b.Root.Item(0).Path != song || !b.Shuffle() {
		t.Fatalf("restored playlist: %d entries, shuffle=%v", b.Root.Len(), b.Shuffle())
	}
	if b.Player.Playing() || b.Player.NextSongName() != "x.mp3" {
		t.Fatal("the last song should be ready but not playing")
	}
	if b.Msg.Text != "Press '5' to continue x.mp3 at 1:15" {
		t.Fatalf("message %q", b.Msg.Text)
	}

	// Without a saved song, nothing is prepared; a missing file is fine.
	c, _ := newTestApp(t)
	c.RestoreSession(filepath.Join(t.TempDir(), "missing.lst"))
	if c.Root.Len() != 0 || c.Player.NextSongName() != "" {
		t.Fatal("nothing to restore")
	}
	c.CaptureState()
	if c.Opts.LastDir != "" || c.Opts.LastSong != "" {
		t.Fatal("unused file manager / no song must not be recorded")
	}
}

func TestBackgroundTaskProgressAndCancel(t *testing.T) {
	a, _ := newTestApp(t)
	press(a, tcell.KeyF1, tcell.KeyF3) // recursive select
	if a.Task == nil || a.Input != InputTask || a.Task.Title != "Recursively selecting files" {
		t.Fatal("recursive select should run as a task with a progress dialog")
	}
	press(a, 'q') // other keys are blocked while the dialog is shown
	if a.view.(*fakeView).quit != 0 || a.view.(*fakeView).beeps != 1 {
		t.Fatal("keys other than cancel must be ignored")
	}
	waitTask(t, a)
	if a.Mode != keys.ModeNormal || a.Cur.Len() != 4 || a.Input != InputDefault {
		t.Fatalf("added %d songs", a.Cur.Len())
	}
	if a.Msg.Text != "Added 4 files from this dir and all subdirs to current group." {
		t.Fatalf("message %q", a.Msg.Text)
	}

	// A task stuck in its work is cancelled at once; its result is dropped.
	release := make(chan struct{})
	called := false
	startTask(a, "Slow scan", func(ctx context.Context, report func(TaskProgress)) (int, error) {
		report(TaskProgress{Detail: "/slow", Status: "1 file", Fraction: 0.5})
		<-release
		return 1, nil
	}, func(int) { called = true })
	deadline := time.Now().Add(5 * time.Second)
	for a.Task.Detail != "/slow" {
		if time.Now().After(deadline) {
			t.Fatal("progress not shown")
		}
		a.view.(*fakeView).runPosted()
		time.Sleep(time.Millisecond)
	}
	if a.Task.Fraction != 0.5 || a.Task.Status != "1 file" {
		t.Fatalf("progress %+v", a.Task)
	}
	press(a, tcell.KeyEsc)
	if a.Task != nil || a.Input != InputDefault || a.Msg.Text != "Slow scan: cancelled." {
		t.Fatalf("cancel: task=%v msg=%q", a.Task, a.Msg.Text)
	}
	close(release)
	time.Sleep(20 * time.Millisecond)
	a.view.(*fakeView).runPosted()
	if called {
		t.Fatal("the result of a cancelled task must be discarded")
	}
}

func TestDeleteMoveAndMouse(t *testing.T) {
	a, _ := newTestApp(t)
	a.AddFiles([]string{"/m/1.mp3", "/m/2.mp3", "/m/3.mp3"})
	a.ToggleRow(2)
	a.ClickRow(0)
	press(a, 'M') // move before 1
	if filepath.Base(a.Cur.Item(0).Path) != "3.mp3" {
		t.Fatal("move before")
	}
	a.Scroll(1)
	press(a, 'd')
	if a.Cur.Len() != 2 || filepath.Base(a.Cur.Item(1).Path) != "2.mp3" {
		t.Fatal("delete")
	}
	a.Click(keys.CmdToggleRepeat)
	if !a.Repeat() {
		t.Fatal("click command")
	}
}
