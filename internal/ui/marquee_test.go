package ui

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rivo/uniseg"

	"github.com/mingcheng/mp3blstr/internal/app"
	"github.com/mingcheng/mp3blstr/internal/keys"
	"github.com/mingcheng/mp3blstr/internal/listview"
	"github.com/mingcheng/mp3blstr/internal/media"
	"github.com/mingcheng/mp3blstr/internal/playback"
)

func TestMarquee(t *testing.T) {
	// A name that fits travels across the whole window: out on the left,
	// back in on the right.
	if got := marquee("short", marqueeHold, 10); got != "short" {
		t.Fatalf("rest: %q", got)
	}
	if got := marquee("short", marqueeHold+1, 10); !strings.HasPrefix(got, "hort     s") {
		t.Fatalf("fitting text must scroll across the window: %q", got)
	}
	if got := marquee("short", marqueeHold+7, 10); !strings.HasPrefix(got, "   short  ") {
		t.Fatalf("fitting text must come back from the right: %q", got)
	}
	if got := marquee("short", marqueeHold+10, 10); got != "short" {
		t.Fatalf("fitting text cycle: %q", got)
	}

	const text = "abcdefghij" // 10 cells; period 13 with the gap
	for step := 0; step <= marqueeHold; step++ {
		if got := marquee(text, step, 5); got != text {
			t.Fatalf("step %d should rest at the start: %q", step, got)
		}
	}
	if got := marquee(text, marqueeHold+1, 5); !strings.HasPrefix(got, "bcdef") {
		t.Fatalf("first step: %q", got)
	}
	if got := marquee(text, marqueeHold+9, 5); !strings.HasPrefix(got, "j   a") {
		t.Fatalf("the gap must separate end and start: %q", got)
	}
	// One full period later the start is shown again.
	cycle := marqueeHold + uniseg.StringWidth(text) + len(marqueeGap)
	if got := marquee(text, cycle, 5); got != text {
		t.Fatalf("cycle restart: %q", got)
	}
	if got := marquee(text, cycle+marqueeHold+1, 5); !strings.HasPrefix(got, "bcdef") {
		t.Fatalf("second cycle: %q", got)
	}
	// A wide character cut at the left edge becomes a space.
	if got := marquee("中文字幕歌曲", marqueeHold+1, 5); !strings.HasPrefix(got, " 文字") {
		t.Fatalf("wide characters: %q", got)
	}
}

type queueView struct {
	mu    sync.Mutex
	queue []func()
}

func (v *queueView) Post(f func()) {
	v.mu.Lock()
	v.queue = append(v.queue, f)
	v.mu.Unlock()
}
func (v *queueView) Beep() {}
func (v *queueView) Sync() {}
func (v *queueView) Quit() {}

func (v *queueView) run() {
	v.mu.Lock()
	q := v.queue
	v.queue = nil
	v.mu.Unlock()
	for _, f := range q {
		f()
	}
}

type idleTrack struct{ done chan struct{} }

func (t *idleTrack) Start()                {}
func (t *idleTrack) Pause()                {}
func (t *idleTrack) Stop()                 {}
func (t *idleTrack) Done() <-chan struct{} { return t.done }
func (t *idleTrack) Info() media.SongInfo  { return media.SongInfo{} }
func (t *idleTrack) Elapsed() int          { return 0 }
func (t *idleTrack) Seek(int) error        { return nil }
func (t *idleTrack) SeekFromEnd(int) error { return nil }
func (t *idleTrack) Seekable() bool        { return false }
func (t *idleTrack) Err() error            { return nil }

type idleBackend struct{}

func (idleBackend) Opener(string) func() (playback.Track, error) {
	return func() (playback.Track, error) { return &idleTrack{done: make(chan struct{})}, nil }
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

// playID3 plays an ID3 tagged song shown with its ID3 name, with Nerd Font
// icons (the name scrolls only with them), and returns the app and a
// function drawing the playlist row of the song without its icon.
func playID3(t *testing.T, artist, title string) (*app.App, func() string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "song.mp3")
	writeID3(t, path, artist, title)
	view := &queueView{}
	a := newRenderApp(t, idleBackend{}, view)
	a.AddFiles([]string{path})
	a.Exec(keys.CmdToggleDisplay) // file names -> ID3 names
	a.Opts.NerdFont = true
	a.Player.Play()
	deadline := time.Now().Add(5 * time.Second)
	for !a.Player.HasSong() {
		if time.Now().After(deadline) {
			t.Fatal("song did not start")
		}
		view.run()
		time.Sleep(time.Millisecond)
	}
	return a, func() string {
		line := []rune(draw(t, a, 80, 24, listview.Width{})[11])
		if a.Opts.NerdFont {
			line = append(line[:1], line[3:]...) // the playing icon takes two cells
		}
		return string(line)
	}
}

// The name of the playing song scrolls while the [ID3] label stays put,
// and pausing stops the scrolling.
func TestPlayingNameScrolls(t *testing.T) {
	a, row := playID3(t, "Artist", strings.Repeat("0123456789", 8))
	if got := row(); !strings.HasPrefix(got, "│[ID3] Artist - 0123") {
		t.Fatalf("start: %q", got)
	}
	for range marqueeHold + 2 {
		a.Tick()
	}
	scrolled := row()
	if !strings.HasPrefix(scrolled, "│[ID3] tist - 0123") {
		t.Fatalf("the name must scroll behind the [ID3] label: %q", scrolled)
	}
	a.Player.Pause()
	for range 3 {
		a.Tick()
	}
	if got := row(); got != scrolled {
		t.Fatalf("pausing must stop the scrolling:\n got %q\nwant %q", got, scrolled)
	}
}

// A name that fits the window scrolls as well: it leaves on the left and
// comes back in on the right, behind the [ID3] label.
func TestShortPlayingNameScrolls(t *testing.T) {
	a, row := playID3(t, "Artist", "Song")
	if got := row(); !strings.HasPrefix(got, "│[ID3] Artist - Song   ") {
		t.Fatalf("start: %q", got)
	}
	for range marqueeHold + 3 {
		a.Tick()
	}
	got := []rune(row())
	// "│[ID3] " takes cells 0..6 (after the icon); the name scrolls up to the
	// border of the list.
	if name := string(got[7:65]); !strings.HasPrefix(name, "ist - Song   ") || !strings.HasSuffix(name, "   Art") {
		t.Fatalf("the name must leave on the left and re-enter on the right: %q", string(got))
	}
}

// Without Nerd Font icons the name never scrolls, for consoles that redraw
// poorly; turning the icons back on restarts it from the beginning.
func TestNoScrollingWithoutNerdFont(t *testing.T) {
	a, row := playID3(t, "Artist", strings.Repeat("0123456789", 8))
	a.Exec(keys.CmdToggleNerdFont)
	if a.Opts.NerdFont || !a.Opts.ScrollPlaying {
		t.Fatal("turning Nerd Font off keeps the scroll_playing setting")
	}
	start := row()
	if !strings.HasPrefix(start, "│[ID3] Artist - 0123") {
		t.Fatalf("start: %q", start)
	}
	for range marqueeHold + 5 {
		a.Tick()
	}
	if got := row(); got != start {
		t.Fatalf("the name must not scroll without Nerd Font:\n got %q\nwant %q", got, start)
	}
	a.Exec(keys.CmdToggleNerdFont)
	if got := row(); !strings.HasPrefix(got, "│[ID3] Artist - 0123") {
		t.Fatalf("scrolling restarts from the beginning: %q", got)
	}
	for range marqueeHold + 2 {
		a.Tick()
	}
	if got := row(); !strings.HasPrefix(got, "│[ID3] tist - 0123") {
		t.Fatalf("with Nerd Font the name scrolls again: %q", got)
	}
}
