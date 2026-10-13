//go:build darwin

package mediakeys

import (
	"os"
	"testing"

	"github.com/ebitengine/purego/objc"
)

// This test publishes to the real system session, unlike the private
// dictionary/command tests. Run it explicitly in a logged-in desktop session.
func TestSystemNowPlaying(t *testing.T) {
	if os.Getenv("MP3BLSTR_TEST_SYSTEM_MEDIA") != "1" {
		t.Skip("set MP3BLSTR_TEST_SYSTEM_MEDIA=1 to publish a real system session")
	}
	controls, err := Start(func(Command) {}, func(err error) { t.Error(err) })
	if err != nil {
		t.Fatal(err)
	}
	defer controls.Close()
	controls.Update(Snapshot{Status: "Playing", Title: "mp3blstr system integration test", Duration: 120, CanSeek: true, TrackID: 1})
	onMainSync(func() {
		n := controls.(*nowPlaying)
		dict := n.info.Send(objc.RegisterName("nowPlayingInfo"))
		value := dict.Send(objc.RegisterName("objectForKey:"), n.keys["MPMediaItemPropertyTitle"])
		if value == 0 {
			t.Error("system center did not retain the published title")
		} else {
			t.Logf("system title: %s", objc.Send[string](value, objc.RegisterName("UTF8String")))
		}
		state := objc.Send[uint64](n.info, objc.RegisterName("playbackState"))
		if state != statePlaying {
			t.Errorf("system playback state = %d, want playing", state)
		}
		t.Logf("system playback state: %d; registered commands: %d", state, len(n.targets))
	})
}
