//go:build darwin

package mediakeys

import (
	"errors"
	"testing"
	"time"
)

func TestArtworkTimelineAndErrors(t *testing.T) {
	n := nativeTestPlayer(t)
	now := time.Now()
	onMainSync(func() {
		n.current = Snapshot{Status: "Playing", Position: 10, Duration: 60}
		n.currentAt = now.Add(-3500 * time.Millisecond)
		if got := n.snapshotAt(now).Position; got != 13 {
			t.Errorf("late artwork would rewind the timeline: %d", got)
		}
		n.current.Status = "Paused"
		if got := n.snapshotAt(now).Position; got != 10 {
			t.Errorf("paused timeline advanced: %d", got)
		}
		n.current.Status = "Playing"
		n.currentAt = now.Add(-time.Hour)
		if got := n.snapshotAt(now).Position; got != 60 {
			t.Errorf("timeline exceeded track duration: %d", got)
		}
		n.artworkTrack = artworkRequest{"broken.mp3", 2}
		reported := 0
		n.report = func(error) { reported++ }
		n.applyArtwork(artworkRequest{"old.mp3", 1}, nil, errors.New("old error"))
		n.applyArtwork(n.artworkTrack, nil, errors.New("broken image"))
		if reported != 1 || n.artwork != 0 {
			t.Error("artwork errors were not reported or stale errors leaked")
		}
		n.applyArtwork(n.artworkTrack, nil, nil)
		if reported != 1 {
			t.Error("missing artwork was treated as an error")
		}
	})
}
