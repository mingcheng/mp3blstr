package playback

import (
	"errors"
	"testing"

	"github.com/mingcheng/mp3blstr/internal/media"
	"github.com/mingcheng/mp3blstr/internal/playlist"
)

func TestPauseWhileOpeningAndManualResume(t *testing.T) {
	root := playlist.NewGroup("test")
	root.AddSong("a.mp3")
	h := newHarness(root)
	defer h.c.Shutdown()
	h.c.Play()
	h.c.Pause()
	h.settle(t)
	tr := h.c.track.(*fakeTrack)
	if !h.c.Paused() || h.c.Status() != StatusPause || tr.starts != 0 {
		t.Fatal("in-flight open started after an interruption")
	}
	h.c.Pause()
	if tr.starts != 0 {
		t.Fatal("a repeated interruption resumed playback")
	}
	h.c.Play()
	if h.c.Paused() || tr.starts != 1 {
		t.Fatal("manual Play did not resume")
	}
	tr.pos = 12
	h.c.Pause()
	if h.c.Elapsed() != 12 || !tr.paused {
		t.Fatal("pause did not capture the current position")
	}
}

func TestDeviceFailureStopsPlaylistAndPreservesPosition(t *testing.T) {
	root := playlist.NewGroup("test")
	root.AddSong("a.mp3")
	root.AddSong("b.mp3")
	h := newHarness(root)
	defer h.c.Shutdown()
	h.c.QuitAfterPlaylist = true
	h.c.Repeat = true
	h.c.Play()
	h.settle(t)
	tr := h.c.track.(*fakeTrack)
	tr.pos, tr.err = 12, media.ErrDeviceLost
	h.c.Tick()
	h.finishSong(t)
	if h.c.Playing() || h.l.quit || len(h.b.opened) != 1 || h.l.warning == "" {
		t.Fatal("device error should stop, not skip, repeat or quit")
	}
	if path, pos := h.c.ResumePoint(); path != "a.mp3" || pos != 12 {
		t.Fatalf("resume point = %q, %d", path, pos)
	}
	h.b.err = media.ErrDeviceOpen
	h.c.Play()
	h.settle(t)
	if path, pos := h.c.ResumePoint(); path != "a.mp3" || pos != 12 {
		t.Fatalf("failed manual retry discarded resume point: %q, %d", path, pos)
	}
}

func TestSeekRevisionAndFailure(t *testing.T) {
	root := playlist.NewGroup("test")
	root.AddSong("a.mp3")
	h := newHarness(root)
	defer h.c.Shutdown()
	h.c.Play()
	h.settle(t)
	revision := h.c.Revision()
	tr := h.c.track.(*fakeTrack)
	tr.pos = 10
	h.c.Tick()
	if h.c.Revision() != revision {
		t.Fatal("normal progress must not republish native metadata")
	}
	h.c.SeekTo(10)
	if h.c.Revision() == revision {
		t.Fatal("even a seek within the same second must publish")
	}
	h.c.SeekTo(-1)
	if tr.pos != 0 {
		t.Fatal("negative seek was not clamped")
	}
	revision = h.c.Revision()
	tr.seekErr = errors.New("live stream is not seekable")
	h.c.Forward()
	if h.l.warning == "" || h.c.Revision() == revision || h.c.Elapsed() != tr.pos {
		t.Fatal("failed seek must report an error and resync the actual position")
	}
}
