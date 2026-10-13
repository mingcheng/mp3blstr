//go:build darwin

package mediakeys

import "testing"

func TestMain(m *testing.M) { Main(m.Run) }

func TestMainServicesMainThread(t *testing.T) {
	if !mainLoopRunning.Load() {
		t.Fatal("Main must service the main run loop")
	}
	if pthreadMainNP() == 1 {
		t.Fatal("tests must not run on the main thread")
	}
	var onMainThread bool
	onMainSync(func() { onMainThread = pthreadMainNP() == 1 })
	if !onMainThread {
		t.Fatal("onMainSync must run on the main thread")
	}
}

func TestPlaybackState(t *testing.T) {
	for status, want := range map[string]uint64{
		"Playing": statePlaying,
		"Paused":  statePaused,
		"Stopped": statePaused,
	} {
		if got := playbackState(status); got != want {
			t.Errorf("playbackState(%q) = %d, want %d", status, got, want)
		}
	}
}
