package playback

import (
	"bytes"
	"fmt"
	"os"

	"github.com/mingcheng/mp3blasterr/internal/media"
)

// statusNames are the "status" values of the status file.
var statusNames = map[Status]string{
	StatusPlay:  "playing",
	StatusPause: "paused",
	StatusStop:  "stopped",
}

// WriteStatusFile writes the state of the player to path in the format of
// the original --status-file option. The file is written with a single
// write, so readers polling it rarely see partial contents; it is not
// replaced atomically because it may be a named pipe or a device.
func (c *Controller) WriteStatusFile(path string) error {
	var b bytes.Buffer
	if c.haveInfo {
		fmt.Fprintf(&b, "path %s\n", media.ChopPath(c.songPath))
	}
	if name, ok := statusNames[c.status]; ok {
		fmt.Fprintf(&b, "status %s\n", name)
	}
	si := c.info
	for _, kv := range [][2]string{
		{"title", si.Title}, {"artist", si.Artist}, {"album", si.Album},
		{"year", si.Year}, {"comment", si.Comment}, {"mode", si.Mode},
	} {
		if kv[1] != "" {
			fmt.Fprintf(&b, "%s %s\n", kv[0], kv[1])
		}
	}
	fmt.Fprintf(&b, "format MPEG %d layer %d\n", si.Version+1, si.Layer)
	fmt.Fprintf(&b, "bitrate %d\n", si.Bitrate)
	fmt.Fprintf(&b, "samplerate %d\n", si.SampleRate)
	fmt.Fprintf(&b, "length %d\n", si.TotalSeconds)
	if c.nextName != "" {
		fmt.Fprintf(&b, "next %s\n", c.nextName)
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}
