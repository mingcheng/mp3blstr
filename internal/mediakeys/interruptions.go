package mediakeys

// Interruption is a system event that may require pausing playback.
type Interruption uint8

const (
	SystemSleep Interruption = iota
	OutputChanged
)
