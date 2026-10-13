//go:build darwin

package audio

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// This opt-in test opens the default device but plays only digital silence.
// It does not change system volume, routing, or sleep state.
func TestNativeOutputLifecycle(t *testing.T) {
	if os.Getenv("MP3BLSTR_TEST_NATIVE_AUDIO") != "1" {
		t.Skip("set MP3BLSTR_TEST_NATIVE_AUDIO=1 to exercise CoreAudio with silence")
	}
	const dataSize = 44100 * 4
	data := append(wavHeader(), make([]byte, dataSize)...)
	binary.LittleEndian.PutUint32(data[4:], dataSize+36)
	binary.LittleEndian.PutUint32(data[40:], dataSize)
	path := filepath.Join(t.TempDir(), "silence.wav")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	output := NewOutput(NewMixer())
	p, err := output.Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	p.Start()
	time.Sleep(100 * time.Millisecond)
	p.Pause()
	position := p.Elapsed()
	select {
	case <-p.Done():
		t.Fatal("a paused track must not drain")
	case <-time.After(1100 * time.Millisecond):
	}
	if got := p.Elapsed(); got != position {
		t.Fatalf("paused position advanced: %d -> %d", position, got)
	}
	if err := p.Seek(-p.Elapsed()); err != nil {
		t.Fatal(err)
	}
	p.Start()
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("native playback did not drain after resume")
	}
	p.Stop()
	if err := p.Err(); err != nil {
		t.Fatal(err)
	}
	if err := output.ctx.Err(); err != nil {
		t.Fatal(err)
	}
}
