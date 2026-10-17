package audio

import (
	"encoding/binary"
	"io"
	"math"
	"slices"
	"sync/atomic"
	"testing"
)

// sliceSource is an in-memory source of n samples whose value is the index.
type sliceSource struct {
	pos, n int
	closed bool
}

func (s *sliceSource) Stream(b [][2]float64) (int, bool) {
	if s.pos >= s.n {
		return 0, false
	}
	k := min(len(b), s.n-s.pos)
	for i := range k {
		v := float64(s.pos+i) / 8
		b[i] = [2]float64{v, -v}
	}
	s.pos += k
	return k, true
}
func (s *sliceSource) Err() error       { return nil }
func (s *sliceSource) Len() int         { return s.n }
func (s *sliceSource) Position() int    { return s.pos }
func (s *sliceSource) Seek(p int) error { s.pos = p; return nil }
func (s *sliceSource) Close() error     { s.closed = true; return nil }
func sample(b []byte, i, ch int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(b[i*frameBytes+ch*4:]))
}

func TestPCMReader(t *testing.T) {
	src := &sliceSource{n: 3}
	r := &pcmReader{src: src, s: src}
	b := make([]byte, 2*frameBytes+3) // trailing partial frame is left alone
	n, err := r.Read(b)
	if n != 2*frameBytes || err != nil {
		t.Fatalf("Read = %d, %v", n, err)
	}
	if sample(b, 1, 0) != 0.125 || sample(b, 1, 1) != -0.125 {
		t.Fatalf("bad samples %v %v", sample(b, 1, 0), sample(b, 1, 1))
	}
	if n, err = r.Read(b); n != frameBytes || err != nil {
		t.Fatalf("Read = %d, %v", n, err)
	}
	if n, err = r.Read(b); n != 0 || err != io.EOF || !r.atEOF() {
		t.Fatalf("want EOF, got %d, %v", n, err)
	}
	if _, err := r.Seek(1, io.SeekStart); err != nil || r.atEOF() || r.position() != 1 {
		t.Fatalf("Seek: err=%v eof=%v pos=%d", err, r.atEOF(), r.position())
	}
	if n, err = r.Read(b); n != 2*frameBytes || err != nil || sample(b, 0, 0) != 0.125 {
		t.Fatalf("Read after seek = %d, %v", n, err)
	}
}

func TestPCMReaderClose(t *testing.T) {
	src := &sliceSource{n: 100}
	r := &pcmReader{src: src, s: src}
	r.close()
	if !src.closed || !r.atEOF() {
		t.Fatal("close should close the source and end the stream")
	}
	if n, err := r.Read(make([]byte, frameBytes)); n != 0 || err != io.EOF || src.pos != 0 {
		t.Fatalf("Read after close = %d, %v (source read: %v)", n, err, src.pos != 0)
	}
}

func TestFadeOutSeekAndToggle(t *testing.T) {
	src := &sliceSource{n: 8}
	mixer := NewMixer()
	mixer.SetFade(true)
	r := &pcmReader{src: src, s: mixer.fadeOut(src, 8, 4, func() int64 { return int64(src.Position()) })}
	buf := make([]byte, 8*frameBytes)
	for offset := 0; offset < len(buf); offset += 2 * frameBytes {
		chunk := buf[offset : offset+2*frameBytes]
		if n, err := r.Read(chunk); n != len(chunk) || err != nil {
			t.Fatalf("Read = %d, %v", n, err)
		}
	}
	for index, gain := range []float32{1, 1, 1, 1, 1, .75, .5, .25} {
		want := float32(index) / 8 * gain
		if got := sample(buf, index, 0); got != want {
			t.Errorf("frame %d = %v, want %v", index, got, want)
		}
	}
	if _, err := r.Seek(6, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Read(buf[:frameBytes]); err != nil || sample(buf, 0, 0) != .375 {
		t.Fatalf("fade after seek = %v, %v", sample(buf, 0, 0), err)
	}
	mixer.SetFade(false)
	if _, err := r.Read(buf[:frameBytes]); err != nil || sample(buf, 0, 0) != .875 {
		t.Fatalf("fade disabled = %v, %v", sample(buf, 0, 0), err)
	}
}

// paddingSource is a sliceSource that, like the prefetcher on a buffer
// underrun, hands out silence without advancing its position on every
// other read.
type paddingSource struct {
	sliceSource
	reads int
}

func (s *paddingSource) Stream(b [][2]float64) (int, bool) {
	s.reads++
	if s.reads%2 == 1 && s.pos < s.n {
		clear(b)
		return len(b), true
	}
	return s.sliceSource.Stream(b)
}

// Silence padded in on buffer underruns must not move the fade forward:
// the fade follows the position in the song.
func TestFadeOutIgnoresUnderrunPadding(t *testing.T) {
	src := &paddingSource{sliceSource: sliceSource{n: 8}}
	mixer := NewMixer()
	mixer.SetFade(true)
	s := mixer.fadeOut(src, 8, 4, func() int64 { return int64(src.Position()) })
	var got []float64
	buf := make([][2]float64, 2)
	for len(got) < 8 {
		before := src.Position()
		n, ok := s.Stream(buf)
		if !ok {
			t.Fatal("stream ended early")
		}
		if src.Position() != before {
			for _, f := range buf[:n] {
				got = append(got, f[0])
			}
		}
	}
	for index, gain := range []float64{1, 1, 1, 1, 1, .75, .5, .25} {
		if want := float64(index) / 8 * gain; got[index] != want {
			t.Errorf("frame %d = %v, want %v", index, got[index], want)
		}
	}
}

func TestFadeIn(t *testing.T) {
	read := func(armed, fade bool, seek int) []float64 {
		src := &sliceSource{n: 8}
		mixer := NewMixer()
		mixer.SetFade(fade)
		var on atomic.Bool
		on.Store(armed)
		s := mixer.fadeIn(src, 4, func() int64 { return int64(src.Position()) }, &on)
		src.Seek(seek)
		var out []float64
		buf := make([][2]float64, 3)
		for {
			n, ok := s.Stream(buf)
			for _, f := range buf[:n] {
				out = append(out, f[0])
			}
			if !ok {
				return out
			}
		}
	}
	plain := []float64{0, .125, .25, .375, .5, .625, .75, .875}
	if got := read(false, true, 0); !slices.Equal(got, plain) {
		t.Fatalf("not armed: %v", got)
	}
	if got := read(true, false, 0); !slices.Equal(got, plain) {
		t.Fatalf("fading disabled: %v", got)
	}
	want := []float64{0, .125 * .25, .25 * .5, .375 * .75, .5, .625, .75, .875}
	if got := read(true, true, 0); !slices.Equal(got, want) {
		t.Fatalf("fade in: %v, want %v", got, want)
	}
	if got := read(true, true, 2); !slices.Equal(got, want[2:]) {
		t.Fatalf("after a seek the fade follows the position: %v", got)
	}
}

func TestFadeInFrames(t *testing.T) {
	sec := int64(OutputRate)
	for _, tc := range []struct {
		track, seconds, want int64
	}{
		{60 * sec, 10, 10 * sec},
		{0, 10, 10 * sec}, // streams of unknown length
		{4 * sec, 2, 0},   // very short songs
		{8 * sec, 10, 0},  // shorter than the fade
		{60 * sec, 0, 0},
	} {
		if got := fadeInFrames(tc.track, int(tc.seconds)); got != tc.want {
			t.Errorf("fadeInFrames(%d, %d) = %d, want %d", tc.track, tc.seconds, got, tc.want)
		}
	}
}

func TestFadeDurationFrames(t *testing.T) {
	for _, tc := range []struct {
		track, seconds, want int64
	}{
		{4*int64(OutputRate) + int64(OutputRate) - 1, 2, 0},
		{5 * int64(OutputRate), 2, 2 * int64(OutputRate)},
		{6 * int64(OutputRate), 7, 0},
		{6 * int64(OutputRate), 5, 5 * int64(OutputRate)},
		{6 * int64(OutputRate), 0, 0},
		{0, 5, 0},
	} {
		if got := fadeDurationFrames(tc.track, int(tc.seconds)); got != tc.want {
			t.Errorf("fadeDurationFrames(%d, %d) = %d, want %d", tc.track, tc.seconds, got, tc.want)
		}
	}
}
