package playback

// History remembers the songs played from the playlist so the user can go
// back to previous songs.
type History struct {
	songs []string
	pos   int // index of the current element; len(songs) means "at end"
}

// Add appends a song and moves to the end.
func (h *History) Add(s string) {
	h.songs = append(h.songs, s)
	h.pos = len(h.songs)
}

// AtStart reports whether the position is at the first element.
func (h *History) AtStart() bool { return h.pos == 0 }

// AtEnd reports whether the position is past the last element.
func (h *History) AtEnd() bool { return h.pos >= len(h.songs) }

// Previous moves one position back.
func (h *History) Previous() {
	if h.pos > 0 {
		h.pos--
	}
}

// Next moves one position forward.
func (h *History) Next() {
	if h.pos < len(h.songs) {
		h.pos++
	}
}

// Element returns the song at the current position ("" at the end).
func (h *History) Element() string {
	if h.AtEnd() {
		return ""
	}
	return h.songs[h.pos]
}
