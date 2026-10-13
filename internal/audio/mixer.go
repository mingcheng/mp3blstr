package audio

import (
	"math"
	"sync"
	"sync/atomic"

	"github.com/gopxl/beep/v2"
)

// MixerDevice is one control of the software mixer.
type MixerDevice struct {
	Name  string
	Value int // 0..100
}

// Mixer is a software replacement for the OSS mixer of the original: it has
// a list of devices of which one is shown (and controlled) at a time. It is
// safe for concurrent use; the audio goroutine reads the gains while the UI
// changes them.
type Mixer struct {
	mu      sync.RWMutex
	devices []MixerDevice
	current int
	fade    atomic.Bool
}

// Mixer device indices.
const (
	DevVolume = iota
	DevBalance
)

// NewMixer returns a mixer with a master volume and a balance control.
func NewMixer() *Mixer {
	return &Mixer{devices: []MixerDevice{
		{Name: "Volume", Value: 100},
		{Name: "Balance", Value: 50},
	}}
}

// Current returns the device currently shown.
func (m *Mixer) Current() MixerDevice {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.devices[m.current]
}

// Device returns device dev (DevVolume, DevBalance).
func (m *Mixer) Device(dev int) MixerDevice {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if dev < 0 || dev >= len(m.devices) {
		return MixerDevice{}
	}
	return m.devices[dev]
}

// Next selects the next device (wrapping).
func (m *Mixer) Next() {
	m.mu.Lock()
	m.current = (m.current + 1) % len(m.devices)
	m.mu.Unlock()
}

// Prev selects the previous device (wrapping).
func (m *Mixer) Prev() {
	m.mu.Lock()
	m.current = (m.current + len(m.devices) - 1) % len(m.devices)
	m.mu.Unlock()
}

// Change adjusts the current device by delta.
func (m *Mixer) Change(delta int) {
	m.mu.Lock()
	d := &m.devices[m.current]
	d.Value = clampPercent(d.Value + delta)
	m.mu.Unlock()
}

// Set sets the current device to an absolute value.
func (m *Mixer) Set(v int) {
	m.mu.Lock()
	m.devices[m.current].Value = clampPercent(v)
	m.mu.Unlock()
}

// SetDevice sets device dev to v.
func (m *Mixer) SetDevice(dev, v int) {
	m.mu.Lock()
	if dev >= 0 && dev < len(m.devices) {
		m.devices[dev].Value = clampPercent(v)
	}
	m.mu.Unlock()
}

// SetFade enables or disables end-of-track fading without changing master volume.
func (m *Mixer) SetFade(enabled bool) { m.fade.Store(enabled) }

// Fade reports whether end-of-track fading is enabled.
func (m *Mixer) Fade() bool { return m.fade.Load() }

// fadeOut attenuates the final duration frames of a stream of known length.
func (m *Mixer) fadeOut(s beep.Streamer, total, duration int64, position *int64) beep.Streamer {
	if total <= 0 || duration <= 0 {
		return s
	}
	start := max(int64(0), total-duration)
	return beep.StreamerFunc(func(samples [][2]float64) (int, bool) {
		n, ok := s.Stream(samples)
		if m.Fade() {
			for i := range samples[:n] {
				frame := *position + int64(i)
				if frame >= start {
					gain := float64(max(int64(0), total-frame)) / float64(total-start)
					samples[i][0] *= gain
					samples[i][1] *= gain
				}
			}
		}
		*position += int64(n)
		return n, ok
	})
}

// gains returns the left and right gain factors.
func (m *Mixer) gains() (float64, float64) {
	m.mu.RLock()
	vol := float64(m.devices[DevVolume].Value) / 100
	bal := m.devices[DevBalance].Value
	m.mu.RUnlock()
	g := vol * vol // perceptual curve
	l, r := g, g
	if bal > 50 {
		l *= float64(100-bal) / 50
	} else if bal < 50 {
		r *= float64(bal) / 50
	}
	return l, r
}

// Streamer applies the mixer settings to s.
func (m *Mixer) Streamer(s beep.Streamer) beep.Streamer {
	return beep.StreamerFunc(func(samples [][2]float64) (int, bool) {
		n, ok := s.Stream(samples)
		l, r := m.gains()
		for i := range samples[:n] {
			samples[i][0] *= l
			samples[i][1] *= r
		}
		return n, ok
	})
}

func clampPercent(v int) int { return max(0, min(v, 100)) }

// eightBit reduces the resolution to 8 bits (the --8bits option).
func eightBit(s beep.Streamer) beep.Streamer {
	return beep.StreamerFunc(func(samples [][2]float64) (int, bool) {
		n, ok := s.Stream(samples)
		for i := range samples[:n] {
			for c := range samples[i] {
				samples[i][c] = math.Round(samples[i][c]*127) / 127
			}
		}
		return n, ok
	})
}
