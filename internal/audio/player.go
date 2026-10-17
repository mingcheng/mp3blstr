// Package audio is the sound back end: decoding (MP3, WAV, Ogg Vorbis,
// FLAC, http streams), read-ahead buffering, a software mixer and output to
// the system's audio device through oto (CoreAudio, WASAPI, PulseAudio,
// ALSA...) without cgo.
package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/gopxl/beep/v2"

	"github.com/mingcheng/mp3blstr/internal/media"
)

// OutputRate is the sample rate of the audio device.
const OutputRate beep.SampleRate = 44100

// frameBytes is the size of one stereo float32 sample frame sent to oto.
const frameBytes = 2 * 4

// Options influence how a song is opened.
type Options struct {
	PrefetchFrames int  // read-ahead in MPEG frames (the "threads" setting); 0 disables
	Downsample     bool // halve the sample rate (--downsample)
	EightBit       bool // reduce to 8 bit resolution (--8bits)
	FadeSeconds    int  // fade duration at the end of a known-length track
	FadeInSeconds  int  // fade duration at the start, see Player.FadeIn
}

// Output owns the audio device. It is safe for concurrent use.
type Output struct {
	mixer   *Mixer
	once    sync.Once
	ctx     outputContext
	initErr error
	mu      sync.Mutex
	active  int
}

type outputContext interface {
	NewPlayer(io.Reader) *oto.Player
	Suspend() error
	Resume() error
	Err() error
}

type outputPlayer interface {
	Play()
	Pause()
	PauseAndStopReading()
	BufferedSize() int
	Seek(int64, int) (int64, error)
	Err() error
}

// NewOutput creates the output. The device is opened lazily on first use.
func NewOutput(m *Mixer) *Output { return &Output{mixer: m} }

// init opens the device once; later calls return the first result.
func (o *Output) init() error {
	o.once.Do(func() {
		ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
			SampleRate:      int(OutputRate),
			ChannelCount:    2,
			Format:          oto.FormatFloat32LE,
			BufferSize:      100 * time.Millisecond,
			ApplicationName: "mp3blstr",
		})
		if err != nil {
			o.initErr = err
			return
		}
		<-ready
		if err := ctx.Err(); err != nil {
			o.initErr = err
			return
		}
		o.ctx = ctx
		o.initErr = ctx.Suspend()
	})
	return o.initErr
}

// Player plays one song. It implements playback.Track.
type Player struct {
	out      outputPlayer
	output   *Output
	pcm      *pcmReader
	rate     int // sample rate of the decoded source
	info     media.SongInfo
	seekable bool
	cancel   func() // aborts pending network I/O (nil for files)

	done     chan struct{} // closed by watch at the end of the song
	stop     chan struct{} // closed by Stop
	stopOnce sync.Once
	wake     chan struct{}
	watched  chan struct{}
	active   atomic.Bool
	fadeIn   atomic.Bool // the start of the song fades in
	errMu    sync.Mutex
	err      error
}

// Open decodes path and prepares it for playback (paused). It may block on
// network access for URLs, so call it outside the UI goroutine.
func (o *Output) Open(path string, opts Options) (*Player, error) {
	if err := o.init(); err != nil {
		return nil, fmt.Errorf("%w (%v)", media.ErrDeviceOpen, err)
	}
	if err := o.ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w: %v", media.ErrDeviceOpen, media.ErrDeviceLost, err)
	}
	d, err := decode(path)
	if err != nil {
		return nil, err
	}
	var src source
	if opts.PrefetchFrames > 0 {
		pf := newPrefetcher(d.stream, d.seekable, opts.PrefetchFrames*frameSamples)
		// Give the buffer a head start so playback doesn't begin with an
		// underrun.
		pf.waitFilled(min(pf.capacity, 8*frameSamples), 3*time.Second)
		src = pf
	} else {
		src = &direct{StreamSeekCloser: d.stream, seekable: d.seekable}
	}

	var s beep.Streamer = src
	totalFrames := int64(0)
	if n := src.Len(); n > 0 && d.format.SampleRate > 0 {
		totalFrames = int64(n) * int64(OutputRate) / int64(d.format.SampleRate)
	}
	rate := d.format.SampleRate
	if opts.Downsample && rate >= 16000 {
		s = beep.Resample(3, rate, rate/2, s)
		rate /= 2
	}
	if rate != OutputRate {
		s = beep.Resample(4, rate, OutputRate, s)
	}
	if opts.EightBit {
		s = eightBit(s)
	}
	sourceRate := int64(d.format.SampleRate)
	position := func() int64 {
		return int64(src.Position()) * int64(OutputRate) / max(sourceRate, 1)
	}
	s = o.mixer.fadeOut(s, totalFrames, fadeDurationFrames(totalFrames, opts.FadeSeconds), position)

	p := &Player{
		output:   o,
		rate:     int(d.format.SampleRate),
		info:     d.info,
		seekable: d.seekable,
		cancel:   d.cancel,
		done:     make(chan struct{}),
		stop:     make(chan struct{}),
		wake:     make(chan struct{}, 1),
		watched:  make(chan struct{}),
	}
	s = o.mixer.fadeIn(s, fadeInFrames(totalFrames, opts.FadeInSeconds), position, &p.fadeIn)
	p.pcm = &pcmReader{src: src, s: o.mixer.Streamer(s)}
	p.out = o.ctx.NewPlayer(p.pcm)
	go p.watch()
	return p, nil
}

func fadeDurationFrames(total int64, seconds int) int64 {
	duration := int64(seconds) * int64(OutputRate)
	if total < 5*int64(OutputRate) || duration <= 0 || total < duration {
		return 0
	}
	return duration
}

// fadeInFrames is the length of the fade in; streams of unknown length
// (total 0) fade in as well.
func fadeInFrames(total int64, seconds int) int64 {
	duration := int64(seconds) * int64(OutputRate)
	if duration <= 0 || total > 0 && (total < 5*int64(OutputRate) || total < duration) {
		return 0
	}
	return duration
}

// FadeIn makes the start of the song fade in from silence while fading is
// enabled (Mixer.SetFade). Call it before Start.
func (p *Player) FadeIn() { p.fadeIn.Store(true) }

// watch closes Done once the stream ended and oto played all buffered data.
func (p *Player) watch() {
	defer close(p.watched)
	t := time.NewTimer(20 * time.Millisecond)
	t.Stop()
	defer t.Stop()
	for {
		if isClosed(p.stop) {
			return
		}
		if err := p.playbackError(); err != nil {
			p.setError(err)
			close(p.done)
			return
		}
		var ticks <-chan time.Time
		if p.active.Load() {
			if p.pcm.atEOF() && p.out.BufferedSize() == 0 {
				close(p.done)
				return
			}
			t.Reset(20 * time.Millisecond)
			ticks = t.C
		}
		select {
		case <-p.stop:
			return
		case <-p.wake:
			t.Stop()
		case <-ticks:
		}
	}
}

func (p *Player) playbackError() error {
	if err := p.Err(); err != nil {
		return err
	}
	if err := p.output.ctx.Err(); err != nil {
		return fmt.Errorf("%w: %v", media.ErrDeviceLost, err)
	}
	if err := p.out.Err(); err != nil {
		return fmt.Errorf("%w: %v", media.ErrFileRead, err)
	}
	return nil
}

func (p *Player) setError(err error) {
	p.errMu.Lock()
	p.err = err
	p.errMu.Unlock()
}

func (p *Player) setActive(active bool) {
	o := p.output
	o.mu.Lock()
	if p.active.Swap(active) != active {
		var err error
		if active {
			o.active++
			if o.active == 1 {
				err = o.ctx.Resume()
			}
		} else {
			o.active--
			if o.active == 0 {
				err = o.ctx.Suspend()
			}
		}
		if err != nil {
			p.setError(fmt.Errorf("%w: %v", media.ErrDeviceLost, err))
		}
	}
	o.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Done is closed when the song has been played completely.
func (p *Player) Done() <-chan struct{} { return p.done }

// Info returns information about the song.
func (p *Player) Info() media.SongInfo { return p.info }

func (p *Player) Seekable() bool { return p.seekable }

// Start starts (or resumes) playback.
func (p *Player) Start() {
	p.out.Play()
	p.setActive(true)
}

// Pause pauses playback.
func (p *Player) Pause() {
	p.out.Pause()
	p.setActive(false)
}

// Elapsed returns the playing time in seconds, excluding audio that is
// decoded but still waiting in oto's buffer.
func (p *Player) Elapsed() int {
	secs := float64(p.pcm.position())/float64(p.rate) -
		float64(p.out.BufferedSize())/float64(frameBytes*int(OutputRate))
	return max(int(secs), 0)
}

// Seek moves the play position by delta seconds.
func (p *Player) Seek(delta int) error {
	if !p.seekable {
		return errNotSeekable
	}
	return p.seekTo((p.Elapsed() + delta) * p.rate)
}

// SeekFromEnd jumps to secs seconds before the end of the song.
func (p *Player) SeekFromEnd(secs int) error {
	n := p.pcm.src.Len()
	if !p.seekable || n <= 0 {
		return errNotSeekable
	}
	return p.seekTo(n - secs*p.rate)
}

// seekTo jumps to sample pos. oto drops its buffer and waits for an ongoing
// read before calling pcmReader.Seek.
func (p *Player) seekTo(pos int) error {
	if n := p.pcm.src.Len(); n > 0 && pos >= n {
		pos = n - 1
	}
	_, err := p.out.Seek(int64(max(pos, 0)), io.SeekStart)
	return err
}

// Err reports output or decoding errors; Stop collects the final decoder error.
func (p *Player) Err() error {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	return p.err
}

// Stop ends playback and releases the decoder. Pending network reads are
// aborted first, so stopping a stalled stream doesn't block.
func (p *Player) Stop() {
	p.stopOnce.Do(func() {
		close(p.stop)
		finished := isClosed(p.done)
		if p.cancel != nil {
			p.cancel()
		}
		<-p.watched
		if !finished && p.cancel != nil && !errors.Is(p.Err(), media.ErrDeviceLost) {
			p.setError(nil)
		}
		p.out.PauseAndStopReading()
		p.setActive(false)
		p.pcm.close()
		// Aborting a stream that is still playing makes its decoder fail;
		// that error is ours, not the stream's.
		if finished || p.cancel == nil {
			if err := p.pcm.src.Err(); err != nil {
				p.setError(errors.Join(p.Err(), fmt.Errorf("%w (%v)", media.ErrFileRead, err)))
			}
		}
	})
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// pcmReader feeds oto: it pulls samples through the streamer chain and
// encodes them as interleaved little endian float32. Seek offsets are sample
// positions of the decoded source.
type pcmReader struct {
	mu  sync.Mutex
	src source
	s   beep.Streamer
	buf [][2]float64
	eof bool
}

func (r *pcmReader) Read(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.eof {
		return 0, io.EOF
	}
	n := len(b) / frameBytes
	if cap(r.buf) < n {
		r.buf = make([][2]float64, n)
	}
	n, ok := r.s.Stream(r.buf[:n])
	for i, f := range r.buf[:n] {
		binary.LittleEndian.PutUint32(b[i*frameBytes:], math.Float32bits(float32(f[0])))
		binary.LittleEndian.PutUint32(b[i*frameBytes+4:], math.Float32bits(float32(f[1])))
	}
	if !ok {
		r.eof = true
		return n * frameBytes, io.EOF
	}
	return n * frameBytes, nil
}

func (r *pcmReader) Seek(offset int64, _ int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.src.Seek(int(offset)); err != nil {
		return offset, err
	}
	r.eof = false
	return offset, nil
}

func (r *pcmReader) position() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.src.Position()
}

// close releases the source. It waits for a Read in progress, so the
// decoder is never closed while it is being used.
func (r *pcmReader) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.eof = true
	r.src.Close()
}

func (r *pcmReader) atEOF() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.eof
}
