package app

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
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

// fakeTrack is a stopped-in-time track for playback tests.
type fakeTrack struct {
	done chan struct{}
}

func (t *fakeTrack) Start()                {}
func (t *fakeTrack) Pause()                {}
func (t *fakeTrack) Stop()                 {}
func (t *fakeTrack) Done() <-chan struct{} { return t.done }
func (t *fakeTrack) Info() media.SongInfo  { return media.SongInfo{} }
func (t *fakeTrack) Elapsed() int          { return 0 }
func (t *fakeTrack) Seek(int) error        { return nil }
func (t *fakeTrack) SeekFromEnd(int) error { return nil }
func (t *fakeTrack) Seekable() bool        { return false }
func (t *fakeTrack) Err() error            { return nil }

// okBackend opens every song; with rec set it remembers the last track.
type okBackend struct{ rec *trackRecorder }

type trackRecorder struct {
	mu   sync.Mutex
	last *fakeTrack
}

func (b okBackend) Opener(string) func() (playback.Track, error) {
	return func() (playback.Track, error) {
		tr := &fakeTrack{done: make(chan struct{})}
		if b.rec != nil {
			b.rec.mu.Lock()
			b.rec.last = tr
			b.rec.mu.Unlock()
		}
		return tr, nil
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

func TestDesktopVolume(t *testing.T) {
	a, _ := newTestApp(t)
	mixer := audio.NewMixer()
	a.Mixer, a.fadeMixer = mixer, mixer
	changes := 0
	a.OnPlaybackChange = func() { changes++ }
	a.SetVolume(42)
	if a.Volume() != 42 || mixer.Device(audio.DevVolume).Value != 42 {
		t.Fatal("desktop volume did not reach output")
	}
	a.MixerSet(70)
	a.MixerVolume(-2)
	press(a, 'l')
	if a.Volume() != 70 || changes != 4 {
		t.Fatalf("terminal volume=%d notifications=%d", a.Volume(), changes)
	}
	mixer.Next()
	a.SetVolume(20)
	if mixer.Device(audio.DevBalance).Value != 50 || a.Volume() != 20 {
		t.Fatal("desktop volume changed selected balance control")
	}
	a.Mixer = nil
	a.SetVolume(35)
	if a.Volume() != 35 {
		t.Fatal("hidden mixer did not receive desktop volume")
	}
}

func TestSetPlaybackModes(t *testing.T) {
	a, _ := newTestApp(t)
	changes := 0
	a.OnPlaybackChange = func() { changes++ }
	a.Player.QuitAfterPlaylist = true
	a.SetRepeat(true, true)
	if !a.Repeat() || !a.Player.RepeatTrack || !a.Opts.Repeat || a.Player.QuitAfterPlaylist {
		t.Fatal("track repeat was not applied")
	}
	a.SetRepeat(true, false)
	if !a.Repeat() || a.Player.RepeatTrack {
		t.Fatal("playlist repeat was not applied")
	}
	a.SetRepeat(false, true)
	if a.Repeat() || a.Player.RepeatTrack || a.Opts.Repeat {
		t.Fatal("repeat was not disabled")
	}
	for _, enabled := range []bool{true, true, false} {
		a.SetShuffle(enabled)
		if a.Shuffle() != enabled {
			t.Fatal("shuffle target was toggled instead of applied")
		}
	}
	if changes != 6 {
		t.Fatalf("mode changes published %d times", changes)
	}
	press(a, 'R', 'Z')
	if !a.Repeat() || !a.Shuffle() || changes != 8 {
		t.Fatal("keyboard modes did not publish changes")
	}
}

// writeID3 creates an MP3 file at path holding only an ID3v2.3 tag.
func writeID3(t *testing.T, path, artist, title string) {
	t.Helper()
	var frames []byte
	for _, f := range []struct{ id, text string }{{"TPE1", "\x00" + artist}, {"TIT2", "\x00" + title}} {
		header := make([]byte, 10)
		copy(header, f.id)
		binary.BigEndian.PutUint32(header[4:8], uint32(len(f.text)))
		frames = append(append(frames, header...), f.text...)
	}
	header := []byte{'I', 'D', '3', 3, 0, 0, 0, 0, 0, 0}
	for i, size := 9, len(frames); i >= 6; i, size = i-1, size>>7 {
		header[i] = byte(size & 0x7f)
	}
	if err := os.WriteFile(path, append(header, frames...), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScrollPlayingName(t *testing.T) {
	a, dir := newTestApp(t)
	first, second := filepath.Join(dir, "first.mp3"), filepath.Join(dir, "second.mp3")
	writeID3(t, first, "Artist", "A Rather Long Title")
	writeID3(t, second, "Other", "Song")
	a.AddFiles([]string{first, second})
	a.groupView(a.Root).display = playlist.DisplayID3
	a.Opts.NerdFont = true
	a.Player = playback.New(a.Root, okBackend{}, a.view.Post, (*listener)(a))

	settle := func() {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !a.Player.HasSong() {
			if time.Now().After(deadline) {
				t.Fatal("song did not start")
			}
			a.view.(*fakeView).runPosted()
			time.Sleep(time.Millisecond)
		}
	}
	tick := func(n int) {
		for range n {
			a.Tick()
		}
	}

	if r := a.MainRow(0); r.Marquee || r.Prefix != "" || r.Text != "[ID3] Artist - A Rather Long Title" {
		t.Fatalf("idle row %+v", r)
	}
	a.Player.Play()
	settle()
	r := a.MainRow(0)
	if !r.Marquee || r.Prefix != media.ID3Prefix || r.Text != "Artist - A Rather Long Title" || r.Scroll != 0 {
		t.Fatalf("playing row %+v", r)
	}
	if other := a.MainRow(1); other.Marquee || other.Prefix != "" {
		t.Fatalf("only the playing song scrolls: %+v", other)
	}
	if got := a.TickInterval(); got != marqueeInterval {
		t.Fatalf("tick interval %v, want %v", got, marqueeInterval)
	}
	tick(3)
	if r := a.MainRow(0); r.Scroll != 3 {
		t.Fatalf("scroll %d after 3 ticks", r.Scroll)
	}

	// Pausing stops the scrolling where it is.
	a.Player.Pause()
	tick(3)
	if r := a.MainRow(0); !r.Marquee || r.Scroll != 3 || a.TickInterval() != 0 {
		t.Fatalf("paused: %+v interval %v", r, a.TickInterval())
	}
	a.Player.Play()
	tick(1)
	if r := a.MainRow(0); r.Scroll != 4 {
		t.Fatalf("resumed: scroll %d", r.Scroll)
	}

	// The next song starts at the beginning.
	a.Player.Next()
	settle()
	if r := a.MainRow(1); !r.Marquee || r.Scroll != 0 || r.Text != "Other - Song" {
		t.Fatalf("next song %+v", r)
	}

	// No scrolling outside the playlist.
	press(a, '?')
	before := a.marquee
	if a.Tick(); a.marquee != before {
		t.Fatal("help screen must not scroll")
	}
	press(a, '?')

	// The F key toggles the option.
	tick(2)
	press(a, 'F')
	if a.Opts.ScrollPlaying || !strings.Contains(a.Msg.Text, "disabled") {
		t.Fatalf("F should disable scrolling: %q", a.Msg.Text)
	}
	if r := a.MainRow(1); r.Marquee || r.Prefix != "" || r.Text != "[ID3] Other - Song" {
		t.Fatalf("scrolling off: %+v", r)
	}
	if got := a.TickInterval(); got != time.Second {
		t.Fatalf("scrolling off: tick interval %v", got)
	}
	press(a, 'F')
	if r := a.MainRow(1); !a.Opts.ScrollPlaying || !r.Marquee || r.Scroll != 0 {
		t.Fatalf("F should enable scrolling from the start: %+v", r)
	}

	// Without Nerd Font icons the name never scrolls.
	tick(3)
	press(a, 'I')
	if r := a.MainRow(1); a.Opts.NerdFont || !a.Opts.ScrollPlaying || r.Marquee || r.Text != "[ID3] Other - Song" {
		t.Fatalf("Nerd Font off: %+v", r)
	}
	if a.Tick(); a.marquee != 0 || a.TickInterval() != time.Second {
		t.Fatalf("Nerd Font off: steps %d, tick interval %v", a.marquee, a.TickInterval())
	}
	press(a, 'I')
	if r := a.MainRow(1); !r.Marquee || r.Scroll != 0 || a.TickInterval() != marqueeInterval {
		t.Fatalf("Nerd Font on again: %+v", r)
	}
}

// startPlayer replaces the player with one whose songs open successfully.
// settle runs the UI loop until a song plays; finish ends the playing song
// as if it had been played completely and waits for the next one.
func startPlayer(t *testing.T, a *App) (settle, finish func()) {
	t.Helper()
	rec := &trackRecorder{}
	a.Player = playback.New(a.Root, okBackend{rec: rec}, a.view.Post, (*listener)(a))
	a.Player.Mode = a.Opts.PlayMode
	wait := func(done func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !done() {
			if time.Now().After(deadline) {
				t.Fatal("song did not start")
			}
			a.view.(*fakeView).runPosted()
			time.Sleep(time.Millisecond)
		}
	}
	settle = func() {
		t.Helper()
		wait(a.Player.HasSong)
	}
	finish = func() {
		t.Helper()
		id := a.Player.TrackID()
		rec.mu.Lock()
		close(rec.last.done)
		rec.mu.Unlock()
		wait(func() bool {
			return (a.Player.HasSong() && a.Player.TrackID() != id) || !a.Player.Playing()
		})
	}
	return settle, finish
}

func playingPath(t *testing.T, a *App) string {
	t.Helper()
	path, _, ok := a.Player.SongInfo()
	if !ok {
		t.Fatal("no song")
	}
	return path
}

func TestSkipLocatesPlayingSong(t *testing.T) {
	a, dir := newTestApp(t)
	var paths []string
	for i := range 10 {
		paths = append(paths, filepath.Join(dir, string(rune('a'+i))+".mp3"))
	}
	a.AddFiles(paths)
	settle, _ := startPlayer(t, a)
	v := a.MainView()
	v.SetRows(3, a.MainLen())
	at := func(want int, what string) {
		t.Helper()
		if v.Cursor != want || a.Cur.Item(v.Cursor).Path != playingPath(t, a) {
			t.Fatalf("%s: cursor=%d want %d (playing %s)", what, v.Cursor, want, playingPath(t, a))
		}
		if v.Cursor < v.Offset || v.Cursor >= v.Offset+v.Rows {
			t.Fatalf("%s: cursor %d not visible (offset %d)", what, v.Cursor, v.Offset)
		}
	}

	// Skipping while stopped does nothing.
	v.Set(5, a.MainLen())
	press(a, '6')
	if a.Player.Playing() || v.Cursor != 5 {
		t.Fatal("skipping while stopped")
	}
	a.Player.Play()
	settle()
	at(0, "start")
	press(a, '6')
	if v.Cursor != 0 {
		t.Fatal("the bar must wait until the next song plays")
	}
	settle()
	at(1, "next")
	v.Set(9, a.MainLen())
	press(a, '6')
	settle()
	at(2, "next from elsewhere")
	press(a, '4')
	settle()
	at(1, "previous")

	// The clickable CD buttons and the media keys use the same path.
	v.Set(9, a.MainLen())
	a.Click(keys.CmdPlayNext)
	settle()
	at(2, "next button")
	v.Set(0, a.MainLen())
	a.PlayPrevious()
	settle()
	at(1, "media key previous")
}

func TestFollowPlayingSong(t *testing.T) {
	a, dir := newTestApp(t)
	var paths []string
	for i := range 20 {
		paths = append(paths, filepath.Join(dir, string(rune('a'+i))+".mp3"))
	}
	a.AddFiles(paths)
	settle, finish := startPlayer(t, a)
	v := a.MainView()
	v.SetRows(3, a.MainLen())
	on := func(what string) {
		t.Helper()
		if a.Cur.Item(v.Cursor).Path != playingPath(t, a) || v.Cursor < v.Offset || v.Cursor >= v.Offset+v.Rows {
			t.Fatalf("%s: cursor=%d offset=%d, playing %s", what, v.Cursor, v.Offset, playingPath(t, a))
		}
	}

	// Starting playback shows the first song; the bar then follows the
	// songs that come after it by themselves.
	v.Set(5, a.MainLen())
	a.Player.Play()
	settle()
	on("start")
	for range 4 {
		finish()
		on("next song by itself")
	}

	// A bar moved elsewhere is left alone until the user skips.
	v.Set(9, a.MainLen())
	finish()
	if v.Cursor != 9 {
		t.Fatalf("browsing: the bar moved to %d", v.Cursor)
	}
	press(a, '6')
	settle()
	on("skip")
	finish()
	on("following again")

	// Playing a song with Enter keeps following from there.
	v.Set(8, a.MainLen())
	press(a, tcell.KeyEnter)
	settle()
	on("enter")
	finish()
	on("after enter")

	// L turns following off: nothing moves the bar, not even a skip.
	press(a, 'L')
	if a.Opts.FollowPlaying || a.Msg.Text != "Not following the playing song" {
		t.Fatalf("L: %v %q", a.Opts.FollowPlaying, a.Msg.Text)
	}
	bar := v.Cursor
	finish()
	press(a, '6')
	settle()
	if v.Cursor != bar {
		t.Fatalf("following off: the bar moved to %d", v.Cursor)
	}
	v.Set(0, a.MainLen())
	press(a, 'L')
	if !a.Opts.FollowPlaying || a.Msg.Text != "Following the playing song" {
		t.Fatalf("L: %v %q", a.Opts.FollowPlaying, a.Msg.Text)
	}
	on("turned on")

	// After a stop, the next start is shown again.
	a.Player.Stop()
	v.Set(9, a.MainLen())
	a.Player.Play()
	settle()
	on("restart")
}

func TestSkipOpensGroupOfPlayingSong(t *testing.T) {
	a, dir := newTestApp(t)
	g := playlist.NewGroup("[Album]")
	a.Root.AddGroup(g)
	first, second := filepath.Join(dir, "Album/x.mp3"), filepath.Join(dir, "Album/y.mp3")
	g.AddSong(first)
	g.AddSong(second)
	single := filepath.Join(dir, "a.mp3")
	a.AddFiles([]string{single})
	a.groupView(a.Root).display = playlist.DisplayPath
	settle, finish := startPlayer(t, a)
	a.Player.Play()
	settle()
	if a.Cur != g || playingPath(t, a) != first || a.Cur.Item(a.MainView().Cursor).Path != first {
		t.Fatalf("starting playback must open %s at %s: cur=%s", g.Title, first, a.Cur.Title)
	}
	if a.groupView(g).display != playlist.DisplayPath {
		t.Fatal("the opened group keeps the display mode")
	}
	press(a, '6')
	settle()
	if a.Cur != g || playingPath(t, a) != second || a.Cur.Item(a.MainView().Cursor).Path != second {
		t.Fatalf("next must show %s: cur=%s cursor=%d", second, a.Cur.Title, a.MainView().Cursor)
	}
	// The group played next is opened when the bar followed the last song.
	finish()
	if a.Cur != a.Root || playingPath(t, a) != single || a.Cur.Item(a.MainView().Cursor).Path != single {
		t.Fatalf("the playlist must follow into %s: cur=%s", a.Root.Title, a.Cur.Title)
	}

	// In the file manager the group on screen is kept.
	press(a, tcell.KeyF1)
	press(a, '4')
	settle()
	if playingPath(t, a) != second || a.Cur != a.Root {
		t.Fatal("skipping in the file manager must not change the group")
	}
}

// writeWAV creates a silent 8 kHz mono WAV file of the given length.
func writeWAV(t *testing.T, path string, seconds int) {
	t.Helper()
	size := 16000 * seconds
	data := make([]byte, 44+size)
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
	binary.LittleEndian.PutUint32(data[40:], uint32(size))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSortPlaylistCycle(t *testing.T) {
	a, dir := newTestApp(t)
	p := func(name string) string { return filepath.Join(dir, name) }
	writeID3(t, p("x1.mp3"), "Zed", "Alpha")  // smallest file
	writeID3(t, p("x2.mp3"), "Abba", "Omega") // one byte larger
	writeWAV(t, p("long.wav"), 3)
	writeWAV(t, p("short.wav"), 1)
	sub := playlist.NewGroup("[Sub]")
	a.Root.AddGroup(sub)
	a.AddFiles([]string{p("x1.mp3"), p("short.wav"), p("x2.mp3"), p("long.wav")})
	a.MainView().Set(2, a.MainLen()) // short.wav
	order := func() []string {
		var out []string
		for _, it := range a.Root.Items {
			if it.Kind == playlist.KindSong {
				out = append(out, filepath.Base(it.Path))
			}
		}
		return out
	}

	for _, c := range []struct {
		mode string
		want []string
	}{
		{"name", []string{"long.wav", "short.wav", "x1.mp3", "x2.mp3"}},
		{"artist", []string{"x2.mp3", "x1.mp3", "long.wav", "short.wav"}},
		{"title", []string{"x1.mp3", "long.wav", "x2.mp3", "short.wav"}},
		{"duration", []string{"short.wav", "long.wav", "x1.mp3", "x2.mp3"}},
		{"size", []string{"x1.mp3", "x2.mp3", "short.wav", "long.wav"}},
	} {
		press(a, 'o')
		waitTask(t, a)
		if got := order(); !slices.Equal(got, c.want) {
			t.Fatalf("%s:\n got %q\nwant %q", c.mode, got, c.want)
		}
		if a.Root.Item(0).Group != sub {
			t.Fatalf("%s: the sub group must stay at the top", c.mode)
		}
		if a.Cur.Item(a.MainView().Cursor).Path != p("short.wav") {
			t.Fatalf("%s: the selection bar must stay on its song", c.mode)
		}
		if !strings.HasPrefix(a.Msg.Text, "Playlist sorted by "+c.mode+"; press 'o' for ") {
			t.Fatalf("%s: message %q", c.mode, a.Msg.Text)
		}
	}
	if a.durations[p("long.wav")] != 3 {
		t.Fatal("sorting by duration must fill in the displayed lengths")
	}
	press(a, 'o')
	if got := order(); len(got) != 4 || a.Msg.Text != "Playlist shuffled; press 'o' for name" {
		t.Fatalf("shuffle: %q %q", got, a.Msg.Text)
	}
	got := order()
	slices.Sort(got)
	if !slices.Equal(got, []string{"long.wav", "short.wav", "x1.mp3", "x2.mp3"}) {
		t.Fatalf("shuffle lost songs: %q", got)
	}
	press(a, 'o')
	if got := order(); !slices.Equal(got, []string{"long.wav", "short.wav", "x1.mp3", "x2.mp3"}) {
		t.Fatalf("the cycle starts over with name: %q", got)
	}

	// Each group keeps its own place in the cycle; the file manager has no
	// playlist sort.
	a.MainView().Set(0, a.MainLen())
	press(a, tcell.KeyEnter)
	if a.Cur != sub {
		t.Fatal("enter sub group")
	}
	sub.AddSong(p("x1.mp3"))
	press(a, 'o')
	if !strings.Contains(a.Msg.Text, "sorted by name") {
		t.Fatalf("sub group starts with name: %q", a.Msg.Text)
	}
	press(a, tcell.KeyF1, 'o')
	if a.Task != nil || a.Mode != keys.ModeFileSelection {
		t.Fatal("o must not sort in the file manager")
	}
}

func TestSortPlaylistCancel(t *testing.T) {
	a, dir := newTestApp(t)
	paths := []string{filepath.Join(dir, "b.mp3"), filepath.Join(dir, "a.mp3")}
	a.AddFiles(paths)
	a.groupView(a.Root).sort = playlist.SortSize
	press(a, 'o')
	if a.Task == nil {
		t.Fatal("sorting by size runs as a task")
	}
	press(a, tcell.KeyEsc)
	a.view.(*fakeView).runPosted()
	time.Sleep(10 * time.Millisecond)
	a.view.(*fakeView).runPosted()
	if a.Root.Item(0).Path != paths[0] || a.groupView(a.Root).sort != playlist.SortSize {
		t.Fatal("a cancelled sort changes nothing")
	}
}

func TestSortWhilePlaying(t *testing.T) {
	a, dir := newTestApp(t)
	var paths []string
	for _, name := range []string{"d.mp3", "c.mp3", "b.mp3", "a.mp3"} {
		paths = append(paths, filepath.Join(dir, name))
	}
	a.AddFiles(paths)
	settle, _ := startPlayer(t, a)
	a.Player.Play()
	settle()
	if playingPath(t, a) != paths[0] || a.Player.NextSongName() != "c.mp3" {
		t.Fatalf("playing %s, next %s", playingPath(t, a), a.Player.NextSongName())
	}
	press(a, 'o') // by name: a b c d
	if a.Player.NextSongName() != "a.mp3" {
		t.Fatalf("the songs not played yet follow the new order, next %s", a.Player.NextSongName())
	}
	press(a, '6')
	settle()
	if playingPath(t, a) != paths[3] {
		t.Fatalf("playing %s", playingPath(t, a))
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
	// AddFiles stores absolute paths, which gain a drive letter on Windows.
	wantB, err := filepath.Abs("/music/b.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if group.Len() != 2 || a.Root.Len() != 2 {
		t.Fatalf("duplicates not removed across groups: group=%d root=%d", group.Len(), a.Root.Len())
	}
	if got := a.Root.Item(1).Path; got != wantB {
		t.Fatalf("root song path = %q, want %q", got, wantB)
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
	if lines[0][0].Desc != "Select Files" || lines[2][1].Cmd != keys.CmdSortPlaylist ||
		lines[3][1].Desc != "Delete" || len(lines[3]) != 3 || lines[3][2].Cmd != keys.CmdAddPaths {
		t.Fatal("each category should start on a new line")
	}
	if lines[4][0].Cmd != keys.CmdToggleRepeat || lines[4][1].Cmd != keys.CmdToggleShuffle {
		t.Fatal("playback category should follow the playlist")
	}

	ref := strings.Join(a.help, "\n")
	for _, want := range []string{"Key bindings", "  R     Toggle Repeat", "  F6    Toggle Repeat        (Normal Mode)",
		"  spc   Select File          (Normal/File Mode)", "  dwn   Move Scrollbar Down"} {
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
	if !opts.FadeOut || !gains.Fade() || a.Msg.Text != "Fade in/out enabled" {
		t.Fatalf("toggle on: enabled=%v mixer=%v message=%q", opts.FadeOut, gains.Fade(), a.Msg.Text)
	}
	a.HandleKey(keys.FromEvent(tcell.NewEventKey(tcell.KeyRune, 'V', tcell.ModShift)))
	if opts.FadeOut || gains.Fade() || a.Msg.Text != "Fade in/out disabled" {
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

// Entering a group keeps the display mode of the group shown before and
// starts unpanned; "[..]" leads back to the parent.
func TestEnterGroupKeepsDisplay(t *testing.T) {
	a, _ := newTestApp(t)
	sub := playlist.NewGroup("[Sub]")
	sub.AddSong("/music/song.mp3")
	a.Root.AddGroup(sub)
	a.groupView(a.Root).display = playlist.DisplayPath
	a.groupView(sub).state.Pan = 7

	a.Exec(keys.CmdEnter)
	if a.Cur != sub {
		t.Fatal("enter must open the sub group")
	}
	if gv := a.groupView(sub); gv.display != playlist.DisplayPath || gv.state.Pan != 0 {
		t.Fatalf("sub group view: display %v pan %d", gv.display, gv.state.Pan)
	}
	if got := a.MainRow(1).Text; got != "/music/song.mp3" {
		t.Fatalf("song shown as %q", got)
	}

	a.Exec(keys.CmdEnter) // on "[..]"
	if a.Cur != a.Root {
		t.Fatal("[..] must lead back to the parent group")
	}
}

func TestToggleMessages(t *testing.T) {
	a, _ := newTestApp(t)
	for _, tc := range []struct {
		cmd  keys.Command
		want string
	}{
		{keys.CmdToggleNerdFont, "Nerd Font icons enabled"},
		{keys.CmdToggleNerdFont, "Nerd Font icons disabled"},
		{keys.CmdToggleFadeOut, "Fade in/out enabled"},
		{keys.CmdToggleFadeOut, "Fade in/out disabled"},
		{keys.CmdToggleScroll, "Scrolling of the playing song disabled"},
		{keys.CmdToggleScroll, "Scrolling of the playing song enabled (needs Nerd Font icons)"},
		{keys.CmdToggleNerdFont, "Nerd Font icons enabled"},
		{keys.CmdToggleScroll, "Scrolling of the playing song disabled"},
		{keys.CmdToggleScroll, "Scrolling of the playing song enabled"},
	} {
		a.Exec(tc.cmd)
		if a.Msg.Text != tc.want {
			t.Errorf("after command %d: message %q, want %q", tc.cmd, a.Msg.Text, tc.want)
		}
	}
}
