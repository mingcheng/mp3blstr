package mediakeys

import (
	"errors"
	"math"
)

// Action is a transport command delivered by the operating system.
type Action uint8

const (
	PlayPause Action = iota
	Play
	Pause
	Next
	Previous
	Stop
	Seek
	SetLoopStatus
	SetShuffle
)

// Command carries explicit seek and playback-mode targets. TrackID prevents
// delayed commands for a previous track from seeking the next one.
type Command struct {
	Action     Action
	Position   float64
	TrackID    int
	LoopStatus string
	Enabled    bool
}

// SeekSeconds validates a native target and converts it to the player's
// second-based timeline, keeping it within the playable range.
func SeekSeconds(position float64, duration int) (int, error) {
	if math.IsNaN(position) || math.IsInf(position, 0) || duration <= 0 {
		return 0, errors.New("invalid playback position or unknown duration")
	}
	if position >= float64(duration) {
		return duration - 1, nil
	}
	return int(math.Max(position, 0)), nil
}

// Controller owns a platform media-key registration.
type Controller interface {
	Update(Snapshot)
	Close() error
}

// Snapshot is captured on the UI goroutine. Revision changes on a new track
// or seek attempt, so native players resync even after a failed seek.
type Snapshot struct {
	Status                       string
	Path, Title, Artist, Album   string
	Duration, Position, Revision int
	TrackID                      int
	CanSeek                      bool
	LoopStatus                   string
	Shuffle                      bool
}

func (s Snapshot) samePlayback(other Snapshot) bool {
	s.Position, other.Position = 0, 0
	return s == other
}
