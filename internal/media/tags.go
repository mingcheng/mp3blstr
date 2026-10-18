package media

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/dhowden/tag"
	"github.com/saintfish/chardet"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/simplifiedchinese"
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

// Recoder translates tag text (files.charset_table setting). nil means identity.
type Recoder func(string) string

var (
	recodeMu        sync.RWMutex
	recoder         Recoder
	guessLegacyTags bool
)

// SetGuessLegacyTags enables per-field recovery of mislabeled UTF-8 and GB18030 ID3 text.
func SetGuessLegacyTags(enabled bool) {
	recodeMu.Lock()
	guessLegacyTags = enabled
	recodeMu.Unlock()
	clearMetadataMemory()
}

// SetRecoder installs a global tag text recoder.
func SetRecoder(r Recoder) {
	recodeMu.Lock()
	recoder = r
	recodeMu.Unlock()
	clearMetadataMemory()
}

// recode applies the legacy tag guessing and the configured recoder to
// tag text.
func recode(s string) string {
	recodeMu.RLock()
	r := recoder
	guess := guessLegacyTags
	recodeMu.RUnlock()
	if guess {
		s = guessTagText(s)
	}
	if r == nil {
		return s
	}
	return r(s)
}

// guessTagText recovers tag text that a tag library decoded as Latin-1
// although it holds UTF-8 or a legacy encoding (mostly GB18030): when every
// rune fits in a byte, the bytes are re-decoded as UTF-8 or as the charset
// chardet detects. s is returned unchanged when that fails.
func guessTagText(s string) string {
	if strings.IndexFunc(s, func(r rune) bool { return r > 127 }) < 0 {
		return s
	}
	var raw []byte
	for _, char := range s {
		if char > 255 {
			return s
		}
		raw = append(raw, byte(char))
	}
	if utf8.Valid(raw) {
		return string(raw)
	}
	sample := raw
	for len(sample) > 0 && sample[0] < 128 {
		sample = sample[1:]
	}
	for len(sample) > 0 && sample[len(sample)-1] < 128 {
		sample = sample[:len(sample)-1]
	}
	results, err := chardet.NewTextDetector().DetectAll(sample)
	if err != nil {
		return s
	}
	charset := results[0].Charset
	for _, result := range results {
		if result.Confidence < results[0].Confidence {
			break
		}
		if result.Charset == "GB-18030" {
			charset = result.Charset
			break
		}
	}
	encoding := simplifiedchinese.GB18030
	if charset != "GB-18030" {
		var err error
		encoding, err = htmlindex.Get(charset)
		if err != nil {
			return s
		}
	}
	decoded, err := encoding.NewDecoder().Bytes(raw)
	if err == nil && !strings.ContainsRune(string(decoded), utf8.RuneError) {
		return string(decoded)
	}
	return s
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
	if entry, ok := cachedEntry(path); ok && entry.Profile == metadataProfile() && entry.Tags != nil {
		return *entry.Tags, nil
	}
	tags, err := readTags(path)
	if err == nil {
		storeEntry(path, func(entry *metadataEntry) {
			if entry.Profile != metadata.profile {
				entry.Name = nil
			}
			entry.Tags = &tags
			entry.Profile = metadata.profile
		})
	}
	return tags, err
}

// readTags reads the tags of path without the cache.
func readTags(path string) (Tags, error) {
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

// clean trims padding from tag text and recodes it.
func clean(s string) string {
	return recode(strings.TrimSpace(strings.Trim(s, "\x00")))
}

// ID3Prefix marks names built from ID3 tags (see ID3Name).
const ID3Prefix = "[ID3] "

// ID3Name returns "[ID3] artist - title" for files with tags, or fallback
// when the file has no usable tag. Results are cached.
func ID3Name(path, fallback string) string {
	if IsHTTPStream(path) {
		return fallback
	}
	if entry, ok := cachedEntry(path); ok && entry.Name != nil && entry.Profile == metadataProfile() {
		if *entry.Name != "" {
			return *entry.Name
		}
		return fallback
	}
	name := ""
	if t, err := ReadTags(path); err == nil && (t.Artist != "" || t.Title != "") {
		artist, title := t.Artist, t.Title
		if artist == "" {
			artist = "Unknown Artist"
		}
		if title == "" {
			title = "Unknown Title"
		}
		name = ID3Prefix + artist + " - " + title
	}
	storeEntry(path, func(entry *metadataEntry) {
		entry.Name = &name
		entry.Profile = metadata.profile
	})
	if name == "" {
		return fallback
	}
	return name
}

// ForgetID3 drops a cached ID3 name (e.g. after a rename).
func ForgetID3(path string) { forgetMetadata(path) }
