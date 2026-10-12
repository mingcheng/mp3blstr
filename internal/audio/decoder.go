package audio

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/flac"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/vorbis"
	"github.com/gopxl/beep/v2/wav"

	"github.com/mingcheng/mp3blaster/internal/media"
)

// SoundError is one of the classic sound error messages of the original
// player. Errors returned by this package wrap one of them so callers can
// show the familiar text (errors.As).
type SoundError string

func (e SoundError) Error() string { return string(e) }

// Sound errors.
const (
	ErrDeviceOpen SoundError = "Failed to open sound device."
	ErrFileOpen   SoundError = "Failed to open file for reading."
	ErrFileRead   SoundError = "Failed to read file."
	ErrHTTP       SoundError = "Http request failed."
	ErrBadFormat  SoundError = "Bad sound file format."
)

type format int

const (
	formatMP3 format = iota
	formatWAV
	formatVorbis
	formatFLAC
)

func formatOf(name string) format {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".wav":
		return formatWAV
	case ".ogg", ".oga":
		return formatVorbis
	case ".flac":
		return formatFLAC
	default:
		return formatMP3
	}
}

// decoded is an opened audio stream together with its metadata.
type decoded struct {
	stream   beep.StreamSeekCloser
	format   beep.Format
	info     media.SongInfo
	seekable bool
}

var httpClient = &http.Client{Timeout: 0, Transport: &http.Transport{ResponseHeaderTimeout: 15 * time.Second}}

// decode opens path (a local file or an http(s) URL).
func decode(path string) (*decoded, error) {
	if media.IsHTTPStream(path) {
		return decodeHTTP(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w (%v)", ErrFileOpen, err)
	}
	d := &decoded{seekable: true}
	kind := formatOf(path)
	switch kind {
	case formatWAV:
		d.stream, d.format, err = wav.Decode(f)
	case formatVorbis:
		d.stream, d.format, err = vorbis.Decode(f)
	case formatFLAC:
		d.stream, d.format, err = flac.Decode(f)
	default:
		d.stream, d.format, err = mp3.Decode(f)
	}
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%w (%v)", ErrBadFormat, err)
	}
	d.info = describe(path, kind, d)
	if t, err := media.ReadTags(path); err == nil {
		d.info.Artist, d.info.Title, d.info.Album = t.Artist, t.Title, t.Album
		d.info.Comment, d.info.Year = t.Comment, t.Year
	}
	return d, nil
}

func describe(path string, kind format, d *decoded) media.SongInfo {
	info := media.SongInfo{SampleRate: int(d.format.SampleRate)}
	if n := d.stream.Len(); n > 0 && d.format.SampleRate > 0 {
		info.TotalSeconds = n / int(d.format.SampleRate)
	}
	if d.format.NumChannels > 1 {
		info.Mode = "stereo"
	} else {
		info.Mode = "mono"
	}
	switch kind {
	case formatMP3:
		if f, err := os.Open(path); err == nil {
			if h, ok := media.ReadMPEGHeader(f); ok {
				info.Version, info.Layer, info.Bitrate = h.Version, h.Layer, h.Bitrate
				info.SampleRate, info.Mode = h.SampleRate, h.Mode
			}
			f.Close()
		}
	case formatWAV:
		info.Bitrate = int(d.format.SampleRate) * d.format.NumChannels * d.format.Precision * 8 / 1000
	default:
		if st, err := os.Stat(path); err == nil && info.TotalSeconds > 0 {
			info.Bitrate = int(st.Size() * 8 / int64(info.TotalSeconds) / 1000)
		}
	}
	return info
}

// readCloser combines a reader with a closer (used for HTTP bodies that are
// pre-read to sniff the stream header).
type readCloser struct {
	io.Reader
	io.Closer
}

func decodeHTTP(url string) (*decoded, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("%w (%v)", ErrHTTP, err)
	}
	if resp.StatusCode/100 != 2 {
		resp.Body.Close()
		return nil, fmt.Errorf("%w (%s)", ErrHTTP, resp.Status)
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	kind := formatOf(url)
	switch {
	case strings.Contains(ct, "ogg"):
		kind = formatVorbis
	case strings.Contains(ct, "flac"):
		kind = formatFLAC
	case strings.Contains(ct, "wav"):
		kind = formatWAV
	case strings.Contains(ct, "mpeg"):
		kind = formatMP3
	}
	d := &decoded{}
	switch kind {
	case formatVorbis:
		d.stream, d.format, err = vorbis.Decode(resp.Body)
	case formatFLAC:
		d.stream, d.format, err = flac.Decode(resp.Body)
	case formatWAV:
		d.stream, d.format, err = wav.Decode(resp.Body)
	default:
		d.stream, d.format, err = mp3.Decode(resp.Body)
	}
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("%w (%v)", ErrBadFormat, err)
	}
	d.info = media.SongInfo{SampleRate: int(d.format.SampleRate), Mode: "stereo"}
	if name := resp.Header.Get("icy-name"); name != "" {
		d.info.Title = name
	}
	if desc := resp.Header.Get("icy-description"); desc != "" {
		d.info.Comment = desc
	}
	if br := resp.Header.Get("icy-br"); br != "" {
		fmt.Sscanf(br, "%d", &d.info.Bitrate)
	}
	return d, nil
}
