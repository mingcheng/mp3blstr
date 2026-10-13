package audio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/flac"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/vorbis"
	"github.com/gopxl/beep/v2/wav"

	"github.com/mingcheng/mp3blasterr/internal/media"
)

// format is a supported container/codec.
type format int

const (
	formatMP3 format = iota
	formatWAV
	formatVorbis
	formatFLAC
)

// formatOf guesses the format from a file name; MP3 is the fallback.
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

// formatOfContentType refines the guess with the Content-Type of an HTTP
// response; fallback is returned when the type says nothing useful.
func formatOfContentType(ct string, fallback format) format {
	ct = strings.ToLower(ct)
	switch {
	case strings.Contains(ct, "ogg"):
		return formatVorbis
	case strings.Contains(ct, "flac"):
		return formatFLAC
	case strings.Contains(ct, "wav"):
		return formatWAV
	case strings.Contains(ct, "mpeg"):
		return formatMP3
	}
	return fallback
}

// newDecoder starts decoding rc as the given format. The decoder owns rc
// and closes it with its Close method.
func newDecoder(rc io.ReadCloser, f format) (beep.StreamSeekCloser, beep.Format, error) {
	switch f {
	case formatWAV:
		return wav.Decode(rc)
	case formatVorbis:
		return vorbis.Decode(rc)
	case formatFLAC:
		return flac.Decode(rc)
	default:
		return mp3.Decode(rc)
	}
}

// Duration returns the length of a local audio file without starting playback.
func Duration(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	stream, format, err := newDecoder(f, formatOf(path))
	if err != nil {
		f.Close()
		return 0, err
	}
	defer stream.Close()
	if format.SampleRate <= 0 || stream.Len() <= 0 {
		return -1, nil
	}
	return stream.Len() / int(format.SampleRate), nil
}

// decoded is an opened audio stream together with its metadata.
type decoded struct {
	stream   beep.StreamSeekCloser
	format   beep.Format
	info     media.SongInfo
	seekable bool
	// cancel aborts pending network I/O so that closing a stalled stream
	// never blocks; nil for local files.
	cancel context.CancelFunc
}

// httpClient fetches streams. There is no overall timeout because streams
// play indefinitely; only waiting for the response headers is limited. The
// default transport is cloned to keep proxy support (HTTP_PROXY etc.).
var httpClient = func() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 15 * time.Second
	return &http.Client{Transport: t}
}()

// decode opens path (a local file or an http(s) URL).
func decode(path string) (*decoded, error) {
	if media.IsHTTPStream(path) {
		return decodeHTTP(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w (%v)", media.ErrFileOpen, err)
	}
	kind := formatOf(path)
	d := &decoded{seekable: true}
	if d.stream, d.format, err = newDecoder(f, kind); err != nil {
		f.Close()
		return nil, fmt.Errorf("%w (%v)", media.ErrBadFormat, err)
	}
	d.info = describeFile(path, kind, d)
	if t, err := media.ReadTags(path); err == nil {
		d.info.Artist, d.info.Title, d.info.Album = t.Artist, t.Title, t.Album
		d.info.Comment, d.info.Year = t.Comment, t.Year
	}
	return d, nil
}

// describeFile collects the stream parameters of a local file.
func describeFile(path string, kind format, d *decoded) media.SongInfo {
	info := media.SongInfo{SampleRate: int(d.format.SampleRate), Mode: "mono"}
	if d.format.NumChannels > 1 {
		info.Mode = "stereo"
	}
	if n := d.stream.Len(); n > 0 && d.format.SampleRate > 0 {
		info.TotalSeconds = n / int(d.format.SampleRate)
	}
	switch kind {
	case formatMP3:
		// beep doesn't expose the MPEG header, so read it ourselves.
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
		// Compressed formats: the average bitrate.
		if st, err := os.Stat(path); err == nil && info.TotalSeconds > 0 {
			info.Bitrate = int(st.Size() * 8 / int64(info.TotalSeconds) / 1000)
		}
	}
	return info
}

// decodeHTTP opens a (shoutcast/icecast) stream. Streams are not seekable.
func decodeHTTP(url string) (*decoded, error) {
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("%w (%v)", media.ErrHTTP, err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("%w (%v)", media.ErrHTTP, err)
	}
	if resp.StatusCode/100 != 2 {
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("%w (%s)", media.ErrHTTP, resp.Status)
	}
	kind := formatOfContentType(resp.Header.Get("Content-Type"), formatOf(url))
	d := &decoded{cancel: cancel}
	if d.stream, d.format, err = newDecoder(resp.Body, kind); err != nil {
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("%w (%v)", media.ErrBadFormat, err)
	}
	d.info = media.SongInfo{
		SampleRate: int(d.format.SampleRate),
		Mode:       "stereo",
		Title:      resp.Header.Get("icy-name"),
		Comment:    resp.Header.Get("icy-description"),
	}
	// Some servers send "128,128"; the first value is the bitrate.
	br, _, _ := strings.Cut(resp.Header.Get("icy-br"), ",")
	if n, err := strconv.Atoi(strings.TrimSpace(br)); err == nil {
		d.info.Bitrate = n
	}
	return d, nil
}
