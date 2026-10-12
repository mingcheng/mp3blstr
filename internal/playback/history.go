package playback

import (
	"bufio"
	"fmt"
	"os"

	"github.com/mingcheng/mp3blaster/internal/media"
)

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

// WriteStatusFile writes the state of the player to path in the format of
// the original --status-file option.
func (c *Controller) WriteStatusFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	if c.haveInfo {
		fmt.Fprintf(w, "path %s\n", media.ChopPath(c.songPath))
	}
	switch c.status {
	case StatusPlay:
		fmt.Fprintln(w, "status playing")
	case StatusPause:
		fmt.Fprintln(w, "status paused")
	case StatusRewind:
		fmt.Fprintln(w, "status rewinding")
	case StatusForward:
		fmt.Fprintln(w, "status fast forwarding")
	case StatusPrev:
		fmt.Fprintln(w, "status selecting previous song")
	case StatusNext:
		fmt.Fprintln(w, "status selecting next song")
	case StatusStop:
		fmt.Fprintln(w, "status stopped")
	case StatusRecord:
		fmt.Fprintln(w, "status recording")
	}
	si := c.info
	for _, kv := range [][2]string{
		{"title", si.Title}, {"artist", si.Artist}, {"album", si.Album},
		{"year", si.Year}, {"comment", si.Comment}, {"mode", si.Mode},
	} {
		if kv[1] != "" {
			fmt.Fprintf(w, "%s %s\n", kv[0], kv[1])
		}
	}
	fmt.Fprintf(w, "format MPEG %d layer %d\n", si.Version+1, si.Layer)
	fmt.Fprintf(w, "bitrate %d\n", si.Bitrate)
	fmt.Fprintf(w, "samplerate %d\n", si.SampleRate)
	fmt.Fprintf(w, "length %d\n", si.TotalSeconds)
	if c.nextName != "" {
		fmt.Fprintf(w, "next %s\n", c.nextName)
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
