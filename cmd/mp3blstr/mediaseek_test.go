package main

import (
	"math"
	"testing"

	"github.com/mingcheng/mp3blstr/internal/mediakeys"
)

func TestMediaSeekCommand(t *testing.T) {
	p := &mediaPlayerFake{trackID: 3, seekable: true}
	p.paused, p.playing = true, true
	for _, pos := range []float64{42.9, -3, 100} {
		if err := handleMediaCommand(p, mediakeys.Command{Action: mediakeys.Seek, TrackID: 3, Position: pos}); err != nil {
			t.Fatal(err)
		}
	}
	for i, want := range []int{42, 0, 59} {
		if p.seeks[i] != want {
			t.Fatalf("seek targets = %v", p.seeks)
		}
	}
	if !p.paused || p.plays != 0 {
		t.Fatal("seeking a paused song must not resume it")
	}
	for _, command := range []mediakeys.Command{
		{Action: mediakeys.Seek, TrackID: 2, Position: 10},
		{Action: mediakeys.Seek, TrackID: 3, Position: math.NaN()},
	} {
		if err := handleMediaCommand(p, command); err == nil {
			t.Fatal("stale or invalid seek must be rejected")
		}
	}
	p.seekable = false
	if err := handleMediaCommand(p, mediakeys.Command{Action: mediakeys.Seek, TrackID: 3, Position: 10}); err == nil || len(p.seeks) != 3 {
		t.Fatal("unseekable stream was seeked")
	}
	if err := handleMediaCommand(p, mediakeys.Command{Action: mediakeys.Next}); err != nil || p.next != 1 {
		t.Fatal("transport command behavior changed")
	}
}
