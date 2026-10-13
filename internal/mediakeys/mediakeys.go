package mediakeys

// Action is a transport command delivered by the operating system.
type Action uint8

const (
	PlayPause Action = iota
	Play
	Pause
	Next
	Previous
	Stop
)

// Controller owns a platform media-key registration.
type Controller interface {
	Update(status string)
	Close() error
}
