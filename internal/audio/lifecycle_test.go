package audio

import (
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebitengine/oto/v3"

	"github.com/mingcheng/mp3blstr/internal/media"
)

type testContext struct {
	resumes, suspends atomic.Int32
	failed            atomic.Bool
}

func (*testContext) NewPlayer(io.Reader) *oto.Player { panic("not used by these tests") }
func (c *testContext) Resume() error                 { c.resumes.Add(1); return c.Err() }
func (c *testContext) Suspend() error                { c.suspends.Add(1); return c.Err() }
func (c *testContext) Err() error {
	if c.failed.Load() {
		return errors.New("test device disconnected")
	}
	return nil
}

type testOutputPlayer struct {
	checks   atomic.Int32
	buffered atomic.Int32
	pcm      *pcmReader
}

func (*testOutputPlayer) Play()                {}
func (*testOutputPlayer) Pause()               {}
func (*testOutputPlayer) PauseAndStopReading() {}
func (p *testOutputPlayer) BufferedSize() int  { return int(p.buffered.Load()) }
func (p *testOutputPlayer) Err() error         { p.checks.Add(1); return nil }
func (p *testOutputPlayer) Seek(offset int64, whence int) (int64, error) {
	p.buffered.Store(0)
	return p.pcm.Seek(offset, whence)
}

func testPlayer(t *testing.T, output *Output) (*Player, *testOutputPlayer) {
	t.Helper()
	source := &sliceSource{n: 44100 * 60}
	pcm := &pcmReader{src: source, s: source}
	out := &testOutputPlayer{pcm: pcm}
	p := &Player{
		output: output, out: out, pcm: pcm, rate: 44100, seekable: true,
		done: make(chan struct{}), stop: make(chan struct{}),
		wake: make(chan struct{}, 1), watched: make(chan struct{}),
	}
	go p.watch()
	t.Cleanup(p.Stop)
	waitFor(t, func() bool { return out.checks.Load() > 0 })
	return p, out
}

func waitFor(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for player")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestPausedPlayerDoesNotPoll(t *testing.T) {
	ctx := &testContext{}
	p, out := testPlayer(t, &Output{ctx: ctx})
	before := out.checks.Load()
	time.Sleep(70 * time.Millisecond)
	if out.checks.Load() != before {
		t.Fatal("a prepared/paused player must not poll")
	}
	p.Start()
	waitFor(t, func() bool { return out.checks.Load() > before+2 })
	p.Pause()
	// Allow the single pause notification to be consumed, then check for
	// the absence of timer-driven work over several former polling periods.
	time.Sleep(30 * time.Millisecond)
	before = out.checks.Load()
	time.Sleep(70 * time.Millisecond)
	if out.checks.Load() != before || ctx.resumes.Load() != 1 || ctx.suspends.Load() != 1 {
		t.Fatal("pause did not suspend the device and stop polling")
	}
	p.Start()
	if ctx.resumes.Load() != 2 {
		t.Fatal("manual resume did not restart the output")
	}
}

func TestOutputOwnership(t *testing.T) {
	ctx := &testContext{}
	output := &Output{ctx: ctx}
	first, _ := testPlayer(t, output)
	second, _ := testPlayer(t, output)
	first.Start()
	second.Start()
	first.Stop()
	if ctx.suspends.Load() != 0 {
		t.Fatal("stopping an older track suspended the current track")
	}
	second.Stop()
	second.Stop()
	if ctx.suspends.Load() != 1 || ctx.resumes.Load() != 1 {
		t.Fatal("output reference counting or idempotent stop failed")
	}
}

func TestDeviceErrorFinishesPlayback(t *testing.T) {
	ctx := &testContext{}
	p, _ := testPlayer(t, &Output{ctx: ctx})
	p.Start()
	ctx.failed.Store(true)
	select {
	case <-p.Done():
	case <-time.After(time.Second):
		t.Fatal("fatal device failure did not finish playback")
	}
	p.Stop()
	if !errors.Is(p.Err(), media.ErrDeviceLost) {
		t.Fatalf("device error = %v", p.Err())
	}
}

func TestEndWaitsForBufferedAudio(t *testing.T) {
	p, out := testPlayer(t, &Output{ctx: &testContext{}})
	out.buffered.Store(8)
	p.pcm.mu.Lock()
	p.pcm.eof = true
	p.pcm.mu.Unlock()
	p.Start()
	select {
	case <-p.Done():
		t.Fatal("track finished before buffered audio drained")
	case <-time.After(50 * time.Millisecond):
	}
	start := time.Now()
	out.buffered.Store(0)
	select {
	case <-p.Done():
		if time.Since(start) > 250*time.Millisecond {
			t.Fatal("end-of-track detection became too slow")
		}
	case <-time.After(time.Second):
		t.Fatal("drained track did not finish")
	}
}

func TestSeekUsesAudiblePosition(t *testing.T) {
	p, out := testPlayer(t, &Output{ctx: &testContext{}})
	p.pcm.src.Seek(20 * p.rate)
	out.buffered.Store(int32(frameBytes * p.rate * 2))
	if p.Elapsed() != 18 {
		t.Fatal("bad test setup")
	}
	if err := p.Seek(5); err != nil {
		t.Fatal(err)
	}
	if p.Elapsed() != 23 {
		t.Fatalf("seek landed at %d instead of the audible position + 5", p.Elapsed())
	}
}

func TestStopCancelsBeforeWaitingForReader(t *testing.T) {
	p, _ := testPlayer(t, &Output{ctx: &testContext{}})
	p.pcm.mu.Lock()
	p.cancel = func() { p.pcm.mu.Unlock() }
	p.Start()
	// Simulate a reader holding the PCM lock while blocked on network I/O.
	time.Sleep(30 * time.Millisecond)
	done := make(chan struct{})
	go func() { p.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop waited for the watcher before cancelling network I/O")
	}
	if p.Err() != nil {
		t.Fatalf("intentional cancellation was reported as an error: %v", p.Err())
	}
}
