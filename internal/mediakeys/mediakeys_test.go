package mediakeys

import (
	"math"
	"testing"
)

func TestSnapshotUpdates(t *testing.T) {
	base := Snapshot{Status: "Playing", Path: "a.mp3", Title: "A", Duration: 60, Position: 10, Revision: 1}
	elapsed := base
	elapsed.Position++
	if !base.samePlayback(elapsed) {
		t.Fatal("ordinary progress should be extrapolated by the OS")
	}
	for name, change := range map[string]func(*Snapshot){
		"seek":           func(s *Snapshot) { s.Revision++ },
		"pause":          func(s *Snapshot) { s.Status = "Paused" },
		"track":          func(s *Snapshot) { s.Path = "b.mp3" },
		"title":          func(s *Snapshot) { s.Title = "B" },
		"artist":         func(s *Snapshot) { s.Artist = "Artist" },
		"album":          func(s *Snapshot) { s.Album = "Album" },
		"seekability":    func(s *Snapshot) { s.CanSeek = true },
		"track identity": func(s *Snapshot) { s.TrackID++ },
	} {
		s := base
		change(&s)
		if base.samePlayback(s) {
			t.Errorf("%s must publish a new snapshot", name)
		}
	}
}

func TestSeekSeconds(t *testing.T) {
	for _, tc := range []struct {
		position       float64
		duration, want int
	}{
		{42.9, 60, 42}, {-10, 60, 0}, {60, 60, 59}, {math.MaxFloat64, 60, 59}, {0, 1, 0},
	} {
		got, err := SeekSeconds(tc.position, tc.duration)
		if err != nil || got != tc.want {
			t.Errorf("SeekSeconds(%v, %d) = %d, %v", tc.position, tc.duration, got, err)
		}
	}
	for _, position := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := SeekSeconds(position, 60); err == nil {
			t.Error("non-finite position accepted")
		}
	}
	if _, err := SeekSeconds(1, 0); err == nil {
		t.Error("unknown duration accepted")
	}
}
