// Package audio is the sound back end: decoding (MP3, WAV, Ogg Vorbis,
// FLAC, http streams), read-ahead buffering, a software mixer and output to
// the system's audio device through oto (CoreAudio, WASAPI, PulseAudio,
// ALSA...) without cgo.
package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/gopxl/beep/v2"

	"github.com/mingcheng/mp3blasterr/internal/media"
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
}

// Output owns the audio device. It is safe for concurrent use.
type Output struct {
	mixer   *Mixer
	once    sync.Once
	ctx     *oto.Context
	initErr error
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
			ApplicationName: "mp3blasterr",
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
	})
	return o.initErr
}

// Player plays one song. It implements playback.Track.
type Player struct {
	out      *oto.Player
	pcm      *pcmReader
	rate     int // sample rate of the decoded source
	info     media.SongInfo
	seekable bool
	cancel   func() // aborts pending network I/O (nil for files)

	done     chan struct{} // closed by watch at the end of the song
	stop     chan struct{} // closed by Stop
	stopOnce sync.Once
	err      error // set by Stop, see Err
}

// Open decodes path and prepares it for playback (paused). It may block on
// network access for URLs, so call it outside the UI goroutine.
func (o *Output) Open(path string, opts Options) (*Player, error) {
	if err := o.init(); err != nil {
		return nil, fmt.Errorf("%w (%v)", media.ErrDeviceOpen, err)
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
	var fadePosition int64
	s = o.mixer.fadeOut(s, totalFrames, fadeDurationFrames(totalFrames, opts.FadeSeconds), &fadePosition)

	p := &Player{
		pcm:      &pcmReader{src: src, s: o.mixer.Streamer(s), fadePosition: &fadePosition, sourceRate: int64(d.format.SampleRate)},
		rate:     int(d.format.SampleRate),
		info:     d.info,
		seekable: d.seekable,
		cancel:   d.cancel,
		done:     make(chan struct{}),
		stop:     make(chan struct{}),
	}
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

// watch closes Done once the stream ended and oto played all buffered data.
func (p *Player) watch() {
	t := time.NewTicker(20 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			if p.pcm.atEOF() && p.out.BufferedSize() == 0 {
				close(p.done)
				return
			}
		}
	}
}

// Done is closed when the song has been played completely.
func (p *Player) Done() <-chan struct{} { return p.done }

// Info returns information about the song.
func (p *Player) Info() media.SongInfo { return p.info }

// Start starts (or resumes) playback.
func (p *Player) Start() { p.out.Play() }

// Pause pauses playback.
func (p *Player) Pause() { p.out.Pause() }

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
	return p.seekTo(p.pcm.position() + delta*p.rate)
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

// Err returns the decoding error that ended playback, if any. It is only
// meaningful after Stop.
func (p *Player) Err() error { return p.err }

// Stop ends playback and releases the decoder. Pending network reads are
// aborted first, so stopping a stalled stream doesn't block.
func (p *Player) Stop() {
	p.stopOnce.Do(func() {
		close(p.stop)
		finished := isClosed(p.done)
		if p.cancel != nil {
			p.cancel()
		}
		p.out.PauseAndStopReading()
		p.pcm.close()
		// Aborting a stream that is still playing makes its decoder fail;
		// that error is ours, not the stream's.
		if finished || p.cancel == nil {
			if err := p.pcm.src.Err(); err != nil {
				p.err = fmt.Errorf("%w (%v)", media.ErrFileRead, err)
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
	mu           sync.Mutex
	src          source
	s            beep.Streamer
	fadePosition *int64
	sourceRate   int64
	buf          [][2]float64
	eof          bool
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
	if r.fadePosition != nil && r.sourceRate > 0 {
		*r.fadePosition = offset * int64(OutputRate) / r.sourceRate
	}
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
