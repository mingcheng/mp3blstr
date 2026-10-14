// Package media contains helpers for recognising audio and playlist files and
// for extracting metadata (ID3 / Vorbis tags, MPEG stream parameters).
package media

import (
	"fmt"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultAudioPatterns are used when AudiofileMatching isn't configured.
var DefaultAudioPatterns = []string{`\.mp[123]$`, `\.wav$`, `\.ogg$`, `\.oga$`, `\.flac$`, `^https?://`}

// DefaultPlaylistPatterns are used when PlaylistMatching isn't configured.
var DefaultPlaylistPatterns = []string{`\.lst$`, `\.m3u$`, `\.m3u8$`}

// Matcher decides which files are audio files and which are playlists.
type Matcher struct {
	audio    []*regexp.Regexp
	playlist []*regexp.Regexp
}

// NewMatcher compiles case-insensitive extended regular expressions. Empty
// lists fall back to the defaults.
func NewMatcher(audio, playlist []string) (*Matcher, error) {
	if len(audio) == 0 {
		audio = DefaultAudioPatterns
	}
	if len(playlist) == 0 {
		playlist = DefaultPlaylistPatterns
	}
	m := &Matcher{}
	var err error
	if m.audio, err = compileAll(audio); err != nil {
		return nil, err
	}
	if m.playlist, err = compileAll(playlist); err != nil {
		return nil, err
	}
	return m, nil
}

func compileAll(pats []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(pats))
	for _, p := range pats {
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			return nil, err
		}
		out = append(out, re)
	}
	return out, nil
}

// IsAudio reports whether name looks like a playable audio file or stream.
func (m *Matcher) IsAudio(name string) bool { return matchAny(m.audio, name) }

// IsPlaylist reports whether name looks like a playlist.
func (m *Matcher) IsPlaylist(name string) bool { return matchAny(m.playlist, name) }

func matchAny(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// IsHTTPStream reports whether the path is an http(s) URL.
func IsHTTPStream(p string) bool {
	l := strings.ToLower(p)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}

// AudioPaths validates a whole batch before callers change the playlist.
func (m *Matcher) AudioPaths(inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("no audio paths given")
	}
	paths := make([]string, len(inputs))
	for index, input := range inputs {
		path := ExpandPath(input)
		if strings.HasPrefix(strings.ToLower(path), "file:") {
			var err error
			path, err = localFileURIPath(path)
			if err != nil || !filepath.IsAbs(path) {
				return nil, fmt.Errorf("invalid local file URI: %s", input)
			}
		}
		if !m.IsAudio(path) {
			return nil, fmt.Errorf("not a supported audio file: %s", input)
		}
		if IsHTTPStream(path) {
			uri, err := url.Parse(path)
			if err != nil || uri.Host == "" {
				return nil, fmt.Errorf("invalid stream URI: %s", input)
			}
		} else {
			var err error
			path, err = filepath.Abs(path)
			if err != nil {
				return nil, err
			}
			info, err := os.Stat(path)
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("not a regular audio file: %s", input)
			}
		}
		paths[index] = path
	}
	return paths, nil
}

func localFileURIPath(input string) (string, error) {
	rest := input[len("file:"):]
	if strings.HasPrefix(rest, "//") {
		escapedPath := rest[2:]
		if !strings.ContainsAny(escapedPath, "?#") {
			path, err := url.PathUnescape(escapedPath)
			if err == nil && isWindowsDrivePath(path) {
				return path, nil
			}
		}
	}

	uri, err := url.Parse(input)
	if err != nil || (uri.Host != "" && uri.Host != "localhost") || uri.User != nil || uri.RawQuery != "" || uri.Fragment != "" {
		return "", fmt.Errorf("invalid local file URI")
	}
	path := uri.Path
	if len(path) > 0 && path[0] == '/' && filepath.VolumeName(path[1:]) != "" {
		path = path[1:]
	}
	return path, nil
}

func isWindowsDrivePath(path string) bool {
	return len(path) >= 3 &&
		((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) &&
		path[1] == ':' && (path[2] == '/' || path[2] == '\\')
}

// ChopPath returns the part of p after the last path separator. URLs are
// returned unchanged.
func ChopPath(p string) string {
	if IsHTTPStream(p) {
		return p
	}
	if i := strings.LastIndexAny(p, `/`+string(os.PathSeparator)); i >= 0 {
		return p[i+1:]
	}
	return p
}

// ExpandPath expands a leading ~ or ~user.
func ExpandPath(p string) string {
	if p == "" || p[0] != '~' {
		return p
	}
	rest := p[1:]
	name := rest
	if i := strings.IndexAny(rest, `/\`); i >= 0 {
		name, rest = rest[:i], rest[i:]
	} else {
		rest = ""
	}
	var home string
	if name == "" {
		home, _ = os.UserHomeDir()
	} else if u, err := user.Lookup(name); err == nil {
		home = u.HomeDir
	} else {
		return p
	}
	return filepath.Clean(home + rest)
}

// HomeDir returns the user's home directory or "/" when unknown.
func HomeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return string(os.PathSeparator)
}
