package main

import (
	"github.com/mingcheng/mp3blasterr/internal/audio"
	"github.com/mingcheng/mp3blasterr/internal/config"
	"github.com/mingcheng/mp3blasterr/internal/playback"
)

// backend adapts the audio output to playback.Backend.
type backend struct {
	out  *audio.Output
	opts *config.Options
}

// Opener implements playback.Backend. The audio options are read when a
// song is opened, so run-time changes (such as the read-ahead size cycled
// with F10) apply to the next song.
func (b *backend) Opener(path string) func() (playback.Track, error) {
	o := audio.Options{
		PrefetchFrames: b.opts.Threads,
		Downsample:     b.opts.Downsample,
		EightBit:       b.opts.EightBits,
		FadeSeconds:    b.opts.FadeSeconds,
	}
	return func() (playback.Track, error) {
		p, err := b.out.Open(path, o)
		if err != nil {
			// Return a nil interface, not a nil *audio.Player.
			return nil, err
		}
		return p, nil
	}
}
