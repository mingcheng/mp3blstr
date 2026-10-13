package audio

import (
	"errors"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
)

// errNotSeekable is returned when seeking a live stream.
var errNotSeekable = errors.New("stream is not seekable")

// source is a decoded audio stream as consumed by the player pipeline.
type source interface {
	beep.Streamer
	Len() int
	Position() int
	Seek(int) error
	Close() error
}

// direct exposes a decoder without read-ahead.
type direct struct {
	beep.StreamSeekCloser
	seekable bool
}

func (d *direct) Seek(p int) error {
	if !d.seekable {
		return errNotSeekable
	}
	return d.StreamSeekCloser.Seek(p)
}

// frameSamples is the number of samples in one MPEG audio frame; the
// "threads" setting of the original counts read-ahead buffers of this size.
const frameSamples = 1152

// prefetcher decodes ahead of playback in its own goroutine, smoothing out
// slow disks and network streams. It is the counterpart of the buffering
// threads of the original mp3blaster.
//
// Lock order: srcMu before mu. mu is never held while decoding, so Stream
// (called by the audio output) never waits for the decoder.
type prefetcher struct {
	src      beep.StreamSeekCloser
	seekable bool
	capacity int // in samples

	srcMu sync.Mutex // serialises decoder access (fill vs. seek and close)

	mu     sync.Mutex // guards the fields below
	cond   *sync.Cond // signalled when buf, eof, closed or gen change
	buf    [][2]float64
	pos    int // stream position of buf[0]
	eof    bool
	closed bool
	gen    int // incremented by Seek; chunks decoded before are dropped
}

func newPrefetcher(src beep.StreamSeekCloser, seekable bool, capacity int) *prefetcher {
	p := &prefetcher{src: src, seekable: seekable, capacity: capacity}
	p.cond = sync.NewCond(&p.mu)
	go p.fill()
	return p
}

// fill runs on its own goroutine until Close, keeping the buffer full.
func (p *prefetcher) fill() {
	chunk := make([][2]float64, frameSamples)
	for {
		p.mu.Lock()
		for !p.closed && (p.eof || len(p.buf) >= p.capacity) {
			p.cond.Wait()
		}
		closed := p.closed
		p.mu.Unlock()
		if closed {
			return
		}

		p.srcMu.Lock()
		p.mu.Lock()
		gen := p.gen
		p.mu.Unlock()
		n, ok := p.src.Stream(chunk)
		p.srcMu.Unlock()

		p.mu.Lock()
		if gen == p.gen {
			p.buf = append(p.buf, chunk[:n]...)
			if !ok {
				p.eof = true
			}
			p.cond.Broadcast()
		}
		p.mu.Unlock()
	}
}

// waitFilled blocks until at least want samples are buffered, the stream
// ended or the timeout expired.
func (p *prefetcher) waitFilled(want int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		done := len(p.buf) >= want || p.eof || p.closed
		p.mu.Unlock()
		if done {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Stream hands out buffered samples; it never blocks on the decoder.
func (p *prefetcher) Stream(samples [][2]float64) (int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := copy(samples, p.buf)
	p.buf = p.buf[n:]
	p.pos += n
	if n > 0 {
		p.cond.Broadcast()
	}
	if n == len(samples) {
		return n, true
	}
	if p.eof {
		return n, n > 0
	}
	// Buffer underrun: pad with silence instead of ending the stream.
	clear(samples[n:])
	return len(samples), true
}

func (p *prefetcher) Err() error {
	p.srcMu.Lock()
	defer p.srcMu.Unlock()
	return p.src.Err()
}

func (p *prefetcher) Len() int { return p.src.Len() }

func (p *prefetcher) Position() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pos
}

func (p *prefetcher) Seek(pos int) error {
	if !p.seekable {
		return errNotSeekable
	}
	p.srcMu.Lock()
	defer p.srcMu.Unlock()
	p.mu.Lock()
	p.gen++
	p.buf = nil
	p.eof = false
	p.mu.Unlock()
	err := p.src.Seek(pos)
	p.mu.Lock()
	p.pos = p.src.Position()
	p.cond.Broadcast()
	p.mu.Unlock()
	return err
}

func (p *prefetcher) Close() error {
	p.mu.Lock()
	p.closed = true
	p.cond.Broadcast()
	p.mu.Unlock()
	p.srcMu.Lock()
	defer p.srcMu.Unlock()
	return p.src.Close()
}
