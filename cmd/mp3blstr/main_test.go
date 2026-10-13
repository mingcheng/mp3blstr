package main

import (
	"testing"

	"github.com/mingcheng/mp3blstr/internal/mediakeys"
)

type mediaPlayerFake struct {
	playing, paused              bool
	plays, next, previous, stops int
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
func (p *mediaPlayerFake) Next()     { p.next++ }
func (p *mediaPlayerFake) Previous() { p.previous++ }
func (p *mediaPlayerFake) Stop()     { p.stops++ }

func TestMediaActions(t *testing.T) {
	p := &mediaPlayerFake{}
	handleMediaAction(p, mediakeys.Pause)
	if p.plays != 0 {
		t.Fatal("pause should not start playback")
	}
	handleMediaAction(p, mediakeys.Play)
	handleMediaAction(p, mediakeys.Play)
	if p.plays != 1 || !p.playing || p.paused {
		t.Fatalf("play should be idempotent: %+v", p)
	}
	handleMediaAction(p, mediakeys.Pause)
	handleMediaAction(p, mediakeys.Pause)
	if p.plays != 2 || !p.paused {
		t.Fatalf("pause should be idempotent: %+v", p)
	}
	handleMediaAction(p, mediakeys.PlayPause)
	handleMediaAction(p, mediakeys.Next)
	handleMediaAction(p, mediakeys.Previous)
	handleMediaAction(p, mediakeys.Stop)
	if p.plays != 3 || p.paused || p.next != 1 || p.previous != 1 || p.stops != 1 {
		t.Fatalf("unexpected media commands: %+v", p)
	}
}
