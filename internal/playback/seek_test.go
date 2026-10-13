package playback

import (
	"testing"

	"github.com/mingcheng/mp3blstr/internal/playlist"
)

func TestSeekCapabilitiesAndIdentity(t *testing.T) {
	root := playlist.NewGroup("test")
	root.AddSong("a.mp3")
	root.AddSong("b.mp3")
	h := newHarness(root)
	defer h.c.Shutdown()
	if h.c.CanSeek() {
		t.Fatal("idle player is seekable")
	}
	h.c.Play()
	if h.c.CanSeek() {
		t.Fatal("opening track is seekable")
	}
	h.settle(t)
	id := h.c.TrackID()
	tr := h.c.track.(*fakeTrack)
	if !h.c.CanSeek() {
		t.Fatal("local known-duration track must support seeking")
	}
	h.c.Pause()
	h.c.SeekTo(21)
	if h.c.TrackID() != id || !h.c.Paused() || !tr.paused || tr.pos != 21 {
		t.Fatal("seek changed track identity or pause state")
	}
	tr.unseekable = true
	if h.c.CanSeek() {
		t.Fatal("unseekable track advertised position control")
	}
	tr.unseekable = false
	h.c.info.TotalSeconds = 0
	if h.c.CanSeek() {
		t.Fatal("unknown-duration track advertised position control")
	}
	h.c.Next()
	h.settle(t)
	if h.c.TrackID() == id || !h.c.CanSeek() {
		t.Fatal("new track did not reset identity and capabilities")
	}
	h.c.Stop()
	if h.c.CanSeek() {
		t.Fatal("stopped player advertised position control")
	}
}
