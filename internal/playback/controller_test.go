package playback

import (
	"errors"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/mingcheng/mp3blstr/internal/media"
	"github.com/mingcheng/mp3blstr/internal/playlist"
)

type fakeTrack struct {
	path       string
	done       chan struct{}
	stopped    bool
	paused     bool
	pos        int
	err        error
	starts     int
	seekErr    error
	unseekable bool
}

func (t *fakeTrack) Start()                { t.paused = false; t.starts++ }
func (t *fakeTrack) Pause()                { t.paused = true }
func (t *fakeTrack) Stop()                 { t.stopped = true }
func (t *fakeTrack) Done() <-chan struct{} { return t.done }
func (t *fakeTrack) Info() media.SongInfo  { return media.SongInfo{TotalSeconds: 60} }
func (t *fakeTrack) Elapsed() int          { return t.pos }
func (t *fakeTrack) Seek(d int) error {
	if t.seekErr != nil {
		return t.seekErr
	}
	t.pos += d
	return nil
}
func (t *fakeTrack) SeekFromEnd(int) error { return nil }
func (t *fakeTrack) Seekable() bool        { return !t.unseekable }
func (t *fakeTrack) Err() error            { return t.err }

type fakeBackend struct {
	opened []string
	err    error
}

func (b *fakeBackend) Opener(p string) func() (Track, error) {
	b.opened = append(b.opened, p)
	err := b.err
	return func() (Track, error) {
		if err != nil {
			return nil, err
		}
		return &fakeTrack{path: p, done: make(chan struct{})}, nil
	}
}

type fakeListener struct {
	cur     *playlist.Group
	quit    bool
	warning string
}

func (l *fakeListener) Changed()                      {}
func (l *fakeListener) SongStarted(media.SongInfo)    {}
func (l *fakeListener) Warning(s string)              { l.warning = s }
func (l *fakeListener) Message(string)                {}
func (l *fakeListener) ClearMessage()                 {}
func (l *fakeListener) EndOfPlaylistQuit()            { l.quit = true }
func (l *fakeListener) CurrentGroup() *playlist.Group { return l.cur }

// harness runs posted functions synchronously in a queue.
type harness struct {
	c     *Controller
	b     *fakeBackend
	l     *fakeListener
	queue chan func()
}

func newHarness(root *playlist.Group) *harness {
	h := &harness{b: &fakeBackend{}, l: &fakeListener{cur: root}, queue: make(chan func(), 16)}
	h.c = New(root, h.b, func(f func()) { h.queue <- f }, h.l)
	h.c.SetRand(rand.New(rand.NewPCG(1, 2)))
	return h
}

// settle runs posted callbacks until the song is loaded.
func (h *harness) settle(t *testing.T) {
	t.Helper()
	for h.c.opening {
		(<-h.queue)()
	}
}

func (h *harness) finishSong(t *testing.T) {
	t.Helper()
	tr := h.c.track.(*fakeTrack)
	close(tr.done)
	(<-h.queue)()
	h.settle(t)
}

func current(c *Controller) string {
	if c.track == nil {
		return ""
	}
	return c.track.(*fakeTrack).path
}

func TestDesktopCapabilities(t *testing.T) {
	root := playlist.NewGroup("[Default]")
	h := newHarness(root)
	defer h.c.Shutdown()
	if play, pause, next, previous := h.c.Capabilities(); play || pause || next || previous {
		t.Fatal("empty player advertised transport capabilities")
	}
	root.AddSong("a")
	root.AddSong("b")
	if play, pause, next, previous := h.c.Capabilities(); !play || pause || next || previous {
		t.Fatal("idle playlist capabilities incorrect")
	}
	h.c.Play()
	h.settle(t)
	if play, pause, next, previous := h.c.Capabilities(); !play || !pause || !next || !previous {
		t.Fatal("playing playlist capabilities incorrect")
	}
	h.c.Next()
	h.settle(t)
	if _, _, next, _ := h.c.Capabilities(); next {
		t.Fatal("last song advertised a nonexistent next track")
	}
	h.c.Repeat = true
	if _, _, next, _ := h.c.Capabilities(); !next {
		t.Fatal("repeating playlist did not advertise next track")
	}
}

func TestGroupsInOrderIncludingSubgroups(t *testing.T) {
	root := playlist.NewGroup("[Default]")
	sub := playlist.NewGroup("[sub]")
	sub.AddSong("s1")
	root.AddGroup(sub)
	root.AddSong("r1")
	root.AddSong("r2")

	h := newHarness(root)
	h.c.Play()
	h.settle(t)
	var order []string
	for h.c.track != nil {
		order = append(order, current(h.c))
		h.finishSong(t)
	}
	want := []string{"s1", "r1", "r2"}
	if len(order) != len(want) {
		t.Fatalf("order %v", order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order %v, want %v", order, want)
		}
	}
	if h.c.Playing() {
		t.Fatal("playlist should have stopped")
	}
}

func TestPlayOneGroupOnly(t *testing.T) {
	root := playlist.NewGroup("[Default]")
	sub := playlist.NewGroup("[sub]")
	sub.AddSong("s1")
	root.AddGroup(sub)
	root.AddSong("r1")
	h := newHarness(root)
	h.c.Mode = PlayGroup
	h.c.Play()
	h.settle(t)
	if current(h.c) != "r1" {
		t.Fatalf("got %q", current(h.c))
	}
	h.finishSong(t)
	if h.c.track != nil {
		t.Fatalf("sub group must not play, got %q", current(h.c))
	}
}

func TestShuffleAllPlaysEverySongOnce(t *testing.T) {
	root := playlist.NewGroup("[Default]")
	sub := playlist.NewGroup("[sub]")
	root.AddGroup(sub)
	for _, s := range []string{"a", "b", "c"} {
		root.AddSong(s)
		sub.AddSong("sub-" + s)
	}
	h := newHarness(root)
	h.c.Mode = PlaySongs
	h.c.Play()
	h.settle(t)
	seen := map[string]bool{}
	for h.c.track != nil {
		if seen[current(h.c)] {
			t.Fatalf("%s played twice", current(h.c))
		}
		seen[current(h.c)] = true
		h.finishSong(t)
	}
	if len(seen) != 6 {
		t.Fatalf("played %d songs", len(seen))
	}
}

func TestRepeatTrack(t *testing.T) {
	root := playlist.NewGroup("[Default]")
	root.AddSong("a")
	root.AddSong("b")
	h := newHarness(root)
	defer h.c.Shutdown()
	h.c.Repeat, h.c.RepeatTrack = true, true
	h.c.Play()
	h.settle(t)
	for count := 0; count < 3; count++ {
		h.finishSong(t)
		if current(h.c) != "a" {
			t.Fatalf("track repeat played %q", current(h.c))
		}
	}
	h.c.Next()
	h.settle(t)
	if current(h.c) != "b" {
		t.Fatalf("manual Next played %q", current(h.c))
	}
	h.c.Repeat = false
	h.finishSong(t)
	if h.c.Playing() {
		t.Fatal("disabled repeat kept playing")
	}
}

func TestRepeatTrackDoesNotRetryPlaybackErrors(t *testing.T) {
	root := playlist.NewGroup("[Default]")
	root.AddSong("a")
	root.AddSong("b")
	h := newHarness(root)
	defer h.c.Shutdown()
	h.c.Repeat, h.c.RepeatTrack = true, true
	h.c.Play()
	h.settle(t)
	h.c.track.(*fakeTrack).err = errors.New("decode failed")
	h.finishSong(t)
	if current(h.c) != "b" {
		t.Fatalf("failed track was repeated: %q", current(h.c))
	}
}

func TestRepeatSingleTrack(t *testing.T) {
	h := newHarness(playlist.NewGroup("[Default]"))
	defer h.c.Shutdown()
	h.c.Repeat, h.c.RepeatTrack = true, true
	h.c.PlayOne("single")
	h.settle(t)
	h.finishSong(t)
	if current(h.c) != "single" {
		t.Fatal("single track did not repeat")
	}
	h.c.RepeatTrack = false
	h.c.Repeat = false
	h.finishSong(t)
	if h.c.Playing() {
		t.Fatal("single track did not stop after repeat disabled")
	}
}

func TestPauseStopRepeatAndQuit(t *testing.T) {
	root := playlist.NewGroup("[Default]")
	root.AddSong("a")
	h := newHarness(root)
	h.c.Play()
	h.settle(t)
	tr := h.c.track.(*fakeTrack)
	h.c.Play()
	if !tr.paused || h.c.Status() != StatusPause || !h.c.ShowPlaySymbol() {
		t.Fatal("expected pause")
	}
	h.c.Play()
	if tr.paused || h.c.Status() != StatusPlay {
		t.Fatal("expected resume")
	}

	h.c.Repeat = true
	h.finishSong(t)
	if current(h.c) != "a" {
		t.Fatal("repeat should restart the playlist")
	}

	h.c.Stop()
	if h.c.Playing() || !tr.stopped {
		t.Fatal("stop failed")
	}

	h.c.Repeat = false
	h.c.QuitAfterPlaylist = true
	h.c.Play()
	h.settle(t)
	h.finishSong(t)
	if !h.l.quit {
		t.Fatal("expected quit after playlist")
	}
}

func TestPreviousUsesHistory(t *testing.T) {
	root := playlist.NewGroup("[Default]")
	root.AddSong("a")
	root.AddSong("b")
	h := newHarness(root)
	h.c.Play()
	h.settle(t)
	h.c.Next()
	h.settle(t)
	if current(h.c) != "b" {
		t.Fatalf("got %q", current(h.c))
	}
	h.c.Previous()
	h.settle(t)
	if current(h.c) != "a" {
		t.Fatalf("previous: got %q", current(h.c))
	}
}

func TestPlayOneInterruptsAndContinues(t *testing.T) {
	root := playlist.NewGroup("[Default]")
	root.AddSong("a")
	root.AddSong("b")
	h := newHarness(root)
	h.c.Play()
	h.settle(t)
	h.c.PlayOne("x")
	h.settle(t)
	if current(h.c) != "x" {
		t.Fatalf("got %q", current(h.c))
	}
	h.finishSong(t)
	if current(h.c) != "b" {
		t.Fatalf("playlist should continue with b, got %q", current(h.c))
	}
}

func TestParsePlayMode(t *testing.T) {
	for _, s := range []string{"onegroup", "allgroups", "allrandom"} {
		m, err := ParsePlayMode(s)
		if err != nil || m.String() != s {
			t.Fatalf("%s: %v %v", s, m, err)
		}
	}
	if _, err := ParsePlayMode("x"); err == nil {
		t.Fatal("expected error")
	}
	if PlayGroups.Next() != PlayGroup || PlayGroup.Next() != PlaySongs || PlaySongs.Next() != PlayGroups {
		t.Fatal("cycle order")
	}
}

// playAll plays until the playlist stops and returns the order of songs.
func (h *harness) playAll(t *testing.T) []string {
	t.Helper()
	h.settle(t)
	var order []string
	for h.c.track != nil {
		order = append(order, current(h.c))
		h.finishSong(t)
	}
	return order
}

func TestResumeContinuesAfterSong(t *testing.T) {
	newList := func() *playlist.Group {
		root := playlist.NewGroup("[Default]")
		sub := playlist.NewGroup("[sub]")
		sub.AddSong("s1")
		sub.AddSong("s2")
		root.AddGroup(sub)
		root.AddSong("r1")
		root.AddSong("r2")
		return root
	}
	for _, tc := range []struct {
		mode PlayMode
		song string
		want []string
	}{
		{PlayGroups, "s1", []string{"s1", "s2", "r1", "r2"}},
		{PlayGroups, "r1", []string{"r1", "r2"}},
		{PlayGroup, "s1", []string{"s1", "s2"}},
		{PlayGroups, "elsewhere", []string{"elsewhere", "s1", "s2", "r1", "r2"}},
	} {
		h := newHarness(newList())
		h.c.Mode = tc.mode
		h.c.Resume(tc.song, 42)
		if h.c.NextSongName() != tc.song || h.c.Playing() {
			t.Fatalf("%v: resume must not start playback", tc)
		}
		if p, pos := h.c.ResumePoint(); p != tc.song || pos != 42 {
			t.Fatalf("pending resume point %q %d", p, pos)
		}
		h.c.Play()
		h.settle(t)
		if tr := h.c.track.(*fakeTrack); tr.pos != 42 || h.c.Elapsed() != 42 {
			t.Fatalf("%v: should start at 42s, got %d", tc, tr.pos)
		}
		if p, pos := h.c.ResumePoint(); p != tc.song || pos != 42 {
			t.Fatalf("resume point while playing %q %d", p, pos)
		}
		got := h.playAll(t)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Fatalf("mode %v from %s: played %v, want %v", tc.mode, tc.song, got, tc.want)
		}
	}
}

func TestResumeRandomSkipsResumedSong(t *testing.T) {
	root := playlist.NewGroup("[Default]")
	for _, s := range []string{"a", "b", "c"} {
		root.AddSong(s)
	}
	h := newHarness(root)
	h.c.Mode = PlaySongs
	h.c.Resume("b", 5)
	h.c.Play()
	got := h.playAll(t)
	if len(got) != 3 || got[0] != "b" || strings.Count(strings.Join(got, ""), "b") != 1 {
		t.Fatalf("played %v", got)
	}
}

func TestResumeCancelledByStopAndPlayOne(t *testing.T) {
	root := playlist.NewGroup("[Default]")
	root.AddSong("a")
	root.AddSong("b")
	h := newHarness(root)
	h.c.Resume("b", 5)
	h.c.Stop()
	if p, _ := h.c.ResumePoint(); p != "" || h.c.NextSongName() != "" {
		t.Fatal("stop should cancel the resume")
	}
	h.c.Play()
	h.settle(t)
	if current(h.c) != "a" || h.c.track.(*fakeTrack).pos != 0 {
		t.Fatalf("got %q", current(h.c))
	}

	h.c.Stop()
	h.c.Resume("b", 5)
	h.c.PlayOne("x")
	h.settle(t)
	if tr := h.c.track.(*fakeTrack); tr.path != "x" || tr.pos != 0 {
		t.Fatal("PlayOne should cancel the resume")
	}
	if p, _ := h.c.ResumePoint(); p != "x" {
		t.Fatalf("resume point %q", p)
	}
}
