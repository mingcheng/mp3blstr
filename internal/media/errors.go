package media

// SoundError is one of the classic sound error messages of the original
// player. Errors produced while opening or decoding a song wrap one of them,
// so callers can show the familiar text (errors.As) or react to a specific
// failure (errors.Is) without depending on the audio back end.
type SoundError string

func (e SoundError) Error() string { return string(e) }

// Sound errors.
const (
	ErrDeviceOpen SoundError = "Failed to open sound device."
	ErrDeviceLost SoundError = "Audio device failed; restart mp3blstr to reopen it."
	ErrFileOpen   SoundError = "Failed to open file for reading."
	ErrFileRead   SoundError = "Failed to read file."
	ErrHTTP       SoundError = "Http request failed."
	ErrBadFormat  SoundError = "Bad sound file format."
)
