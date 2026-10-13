package main

import (
	"testing"

	"github.com/mingcheng/mp3blstr/internal/app"
	"github.com/mingcheng/mp3blstr/internal/config"
	"github.com/mingcheng/mp3blstr/internal/media"
	"github.com/mingcheng/mp3blstr/internal/mediakeys"
	"github.com/mingcheng/mp3blstr/internal/playback"
)

type mediaPlayerFake struct {
	playing, paused                      bool
	plays, pauses, next, previous, stops int
	trackID                              int
	seekable                             bool
	seeks                                []int
}

func (p *mediaPlayerFake) Playing() bool { return p.playing }
func (p *mediaPlayerFake) Paused() bool  { return p.paused }
func (p *mediaPlayerFake) Play() {
	p.plays++
	if !p.playing {
		p.playing = true
		p.paused = false
	} else {
		p.paused = !p.paused
	}
}
func (p *mediaPlayerFake) Pause() {
	if p.playing && !p.paused {
		p.paused = true
		p.pauses++
	}
}
func (p *mediaPlayerFake) Next()     { p.next++ }
func (p *mediaPlayerFake) Previous() { p.previous++ }
func (p *mediaPlayerFake) Stop()     { p.stops++ }

func (p *mediaPlayerFake) TrackID() int  { return p.trackID }
func (p *mediaPlayerFake) CanSeek() bool { return p.seekable }
func (p *mediaPlayerFake) SongInfo() (string, media.SongInfo, bool) {
	return "song.mp3", media.SongInfo{TotalSeconds: 60}, true
}
func (p *mediaPlayerFake) SeekTo(pos int) { p.seeks = append(p.seeks, pos) }

func TestMediaRepeatModes(t *testing.T) {
	a := &app.App{Player: &playback.Controller{}, Opts: config.Default()}
	var modes []string
	a.OnPlaybackChange = func() { modes = append(modes, playbackSnapshot(a.Player).LoopStatus) }
	for _, mode := range []string{"Playlist", "Track", "None"} {
		if err := handleAppMediaCommand(a, mediakeys.Command{Action: mediakeys.SetLoopStatus, LoopStatus: mode}); err != nil {
			t.Fatal(err)
		}
		if snapshot := playbackSnapshot(a.Player); snapshot.LoopStatus != mode {
			t.Fatalf("repeat snapshot = %q, want %q", snapshot.LoopStatus, mode)
		}
	}
	if len(modes) != 3 {
		t.Fatalf("repeat changes published %d times", len(modes))
	}
	if err := handleAppMediaCommand(a, mediakeys.Command{Action: mediakeys.SetLoopStatus, LoopStatus: "Invalid"}); err == nil || a.Repeat() {
		t.Fatal("invalid repeat mode changed playback")
	}
}

func TestMediaActions(t *testing.T) {
	p := &mediaPlayerFake{}
	send := func(action mediakeys.Action) {
		t.Helper()
		if err := handleMediaCommand(p, mediakeys.Command{Action: action}); err != nil {
			t.Fatal(err)
		}
	}
	send(mediakeys.Pause)
	if p.plays != 0 || p.pauses != 0 {
		t.Fatal("pause should not start playback")
	}
	send(mediakeys.Play)
	send(mediakeys.Play)
	if p.plays != 1 || !p.playing || p.paused {
		t.Fatalf("play should be idempotent: %+v", p)
	}
	send(mediakeys.Pause)
	send(mediakeys.Pause)
	if p.plays != 1 || p.pauses != 1 || !p.paused {
		t.Fatalf("pause should be idempotent: %+v", p)
	}
	send(mediakeys.PlayPause)
	send(mediakeys.Next)
	send(mediakeys.Previous)
	send(mediakeys.Stop)
	if p.plays != 2 || p.pauses != 1 || p.paused || p.next != 1 || p.previous != 1 || p.stops != 1 {
		t.Fatalf("unexpected media commands: %+v", p)
	}
}
