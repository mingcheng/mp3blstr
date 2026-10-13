//go:build darwin

package mediakeys

import (
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
)

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

func TestNativeMetadata(t *testing.T) {
	n := nativeTestPlayer(t)
	// Build dictionaries without publishing them to the system session.
	onMainSync(func() {
		for _, status := range []string{"Playing", "Paused", "Stopped"} {
			dict := n.metadata(Snapshot{Status: status, Title: "歌曲", Artist: "Artist", Album: "Album", Duration: 120, Position: 42})
			get := func(key string) objc.ID {
				return dict.Send(objc.RegisterName("objectForKey:"), n.keys[key])
			}
			for key, want := range map[string]string{
				"MPMediaItemPropertyTitle": "歌曲", "MPMediaItemPropertyArtist": "Artist", "MPMediaItemPropertyAlbumTitle": "Album",
			} {
				if got := objc.Send[string](get(key), objc.RegisterName("UTF8String")); got != want {
					t.Errorf("%s = %q", key, got)
				}
			}
			rate := int64(0)
			if status == "Playing" {
				rate = 1
			}
			for key, want := range map[string]int64{
				"MPMediaItemPropertyPlaybackDuration":         120,
				"MPNowPlayingInfoPropertyElapsedPlaybackTime": 42,
				"MPNowPlayingInfoPropertyPlaybackRate":        rate,
			} {
				if got := objc.Send[int64](get(key), objc.RegisterName("longLongValue")); got != want {
					t.Errorf("%s = %d, want %d", key, got, want)
				}
			}
		}
		dict := n.metadata(Snapshot{})
		if value := dict.Send(objc.RegisterName("objectForKey:"), n.keys["MPMediaItemPropertyPlaybackDuration"]); value != 0 {
			t.Error("unknown duration must be omitted")
		}
	})
}

func TestNativeInterruptionRegistration(t *testing.T) {
	events := make(chan Interruption, 1)
	monitor, err := WatchInterruptions(true, true, func(event Interruption) { events <- event })
	if err != nil {
		t.Fatal(err)
	}
	defer monitor.Close()
	n := monitor.(*interruptions)
	// Post only to this process's notification center; this does not put
	// the machine to sleep or change any system audio device.
	onMainSync(func() {
		n.center.Send(objc.RegisterName("postNotificationName:object:"), n.sleepName, objc.ID(0))
	})
	select {
	case got := <-events:
		if got != SystemSleep {
			t.Errorf("notification = %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sleep notification was not delivered")
	}
	if err := monitor.Close(); err != nil {
		t.Fatal(err)
	}
	n.notify(OutputChanged)
	select {
	case <-events:
		t.Fatal("closed monitor delivered an event")
	case <-time.After(20 * time.Millisecond):
	}
}
