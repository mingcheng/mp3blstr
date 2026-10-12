package media

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/dhowden/tag"
)

// SongInfo describes the song currently loaded by the player.
type SongInfo struct {
	Artist, Title, Album, Comment, Year string

	Mode         string // Stereo, JointStereo, DualChannel, Mono, stereo, mono
	Version      int    // MPEG version index (0 = MPEG 1, 1 = MPEG 2, 2 = MPEG 2.5)
	Layer        int    // MPEG layer, 0 for non-MPEG streams
	SampleRate   int    // Hz
	Bitrate      int    // kbit/s
	TotalSeconds int
}

// Tags holds the textual metadata of a file.
type Tags struct {
	Artist, Title, Album, Comment, Year string
}

// Recoder translates tag text (CharsetTable keyword). nil means identity.
type Recoder func(string) string

var (
	recodeMu sync.RWMutex
	recoder  Recoder
)

// SetRecoder installs a global tag text recoder.
func SetRecoder(r Recoder) {
	recodeMu.Lock()
	recoder = r
	recodeMu.Unlock()
}

func recode(s string) string {
	recodeMu.RLock()
	r := recoder
	recodeMu.RUnlock()
	if r == nil {
		return s
	}
	return r(s)
}

// LoadRecodeTable reads a character map file consisting of lines with two
// numbers ("in out", decimal, octal or hex) and returns a Recoder that maps
// characters below 256 accordingly.
func LoadRecodeTable(file string) (Recoder, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("cannot open char map file %q: %w", file, err)
	}
	defer f.Close()
	var table [256]rune
	for i := range table {
		table[i] = rune(i)
	}
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		text := sc.Text()
		if i := strings.IndexByte(text, '#'); i >= 0 {
			text = text[:i]
		}
		fields := strings.Fields(text)
		if len(fields) < 2 {
			continue
		}
		in, err1 := strconv.ParseUint(fields[0], 0, 32)
		out, err2 := strconv.ParseUint(fields[1], 0, 32)
		if err1 != nil || err2 != nil || in == 0 || out == 0 {
			continue
		}
		if in > 255 {
			return nil, fmt.Errorf("%s: line %d: char val too big", file, line)
		}
		table[in] = rune(out)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return func(s string) string {
		var b strings.Builder
		for _, r := range s {
			if r < 256 {
				r = table[r]
			}
			b.WriteRune(r)
		}
		return b.String()
	}, nil
}

// ReadTags reads ID3v1/ID3v2/Vorbis/FLAC/MP4 tags from a local file.
func ReadTags(path string) (Tags, error) {
	f, err := os.Open(path)
	if err != nil {
		return Tags{}, err
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if err != nil {
		return Tags{}, err
	}
	t := Tags{
		Artist:  clean(m.Artist()),
		Title:   clean(m.Title()),
		Album:   clean(m.Album()),
		Comment: clean(m.Comment()),
	}
	if y := m.Year(); y > 0 {
		t.Year = strconv.Itoa(y)
	}
	return t, nil
}

func clean(s string) string {
	return recode(strings.TrimSpace(strings.Trim(s, "\x00")))
}

var id3Cache sync.Map // path -> string

// ID3Name returns "[ID3] artist - title" for files with tags, or fallback
// when the file has no usable tag. Results are cached.
func ID3Name(path, fallback string) string {
	if v, ok := id3Cache.Load(path); ok {
		if s := v.(string); s != "" {
			return s
		}
		return fallback
	}
	name := ""
	if !IsHTTPStream(path) {
		if t, err := ReadTags(path); err == nil && (t.Artist != "" || t.Title != "") {
			artist, title := t.Artist, t.Title
			if artist == "" {
				artist = "Unknown Artist"
			}
			if title == "" {
				title = "Unknown Title"
			}
			name = "[ID3] " + artist + " - " + title
		}
	}
	id3Cache.Store(path, name)
	if name == "" {
		return fallback
	}
	return name
}

// ForgetID3 drops a cached ID3 name (e.g. after a rename).
func ForgetID3(path string) { id3Cache.Delete(path) }
