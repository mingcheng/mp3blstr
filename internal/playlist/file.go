package playlist

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jamesnetherton/m3u"

	"github.com/mingcheng/mp3blasterr/internal/atomicfile"
	"github.com/mingcheng/mp3blasterr/internal/media"
)

// GlobalSettings carries the <GLOBAL ...> settings of a playlist file.
type GlobalSettings struct {
	PlayMode string // onegroup, allgroups, allrandom; empty when absent
	Shuffle  bool   // MODE="shuffle" for the root group
}

// maxGroupNameLen mirrors the 48 character limit of the original.
const maxGroupNameLen = 48

// Load reads a playlist file into group g. Relative song paths are resolved
// against the directory of the playlist file. The <GLOBAL> settings found
// are returned so the caller can apply them.
func Load(path string, g *Group) (GlobalSettings, error) {
	f, err := os.Open(path)
	if err != nil {
		return GlobalSettings{}, err
	}
	defer f.Close()
	base := filepath.Dir(path)
	if abs, err := filepath.Abs(base); err == nil {
		base = abs
	}
	isM3U := IsM3U(path)
	if isM3U {
		// The extended M3U parser keeps group-title tags but silently skips
		// entries without #EXTINF, so it is only trusted when it found every song.
		if pl, err := m3u.Parse(path); err == nil {
			probe := NewGroup("")
			if _, err := Parse(f, probe, base, true); err == nil && probe.UnplayedSongs(true) == len(pl.Tracks) {
				addTracks(g, pl.Tracks, base)
				return GlobalSettings{}, nil
			}
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return GlobalSettings{}, err
			}
		}
	}
	return Parse(f, g, base, isM3U)
}

// IsM3U reports whether path has an M3U playlist extension.
func IsM3U(path string) bool {
	ext := filepath.Ext(path)
	return strings.EqualFold(ext, ".m3u") || strings.EqualFold(ext, ".m3u8")
}

// m3uGroupTag is the #EXTINF attribute naming the group of a song; nested
// groups are separated by m3uGroupSep.
const (
	m3uGroupTag = "group-title"
	m3uGroupSep = "/"
)

// addTracks adds M3U tracks to g, recreating the groups named by their
// group-title tags.
func addTracks(g *Group, tracks []m3u.Track, base string) {
	groups := map[string]*Group{"": g}
	for _, t := range tracks {
		dst := g
		for _, tag := range t.Tags {
			if strings.EqualFold(tag.Name, m3uGroupTag) {
				dst = ensureGroup(groups, tag.Value)
			}
		}
		dst.AddSong(resolve(strings.TrimSpace(t.URI), base))
	}
}

// ensureGroup returns the group for the group-title path, creating it and
// its parents on first use.
func ensureGroup(groups map[string]*Group, path string) *Group {
	if g, ok := groups[path]; ok {
		return g
	}
	parent, name := "", path
	if i := strings.LastIndex(path, m3uGroupSep); i >= 0 {
		parent, name = path[:i], path[i+1:]
	}
	if r := []rune(name); len(r) > maxGroupNameLen {
		name = string(r[:maxGroupNameLen])
	}
	ng := NewGroup("[" + name + "]")
	ensureGroup(groups, parent).AddGroup(ng)
	groups[path] = ng
	return ng
}

// Parse reads playlist data from r into g. Besides song paths (one per
// line), mp3blaster playlists contain <GLOBAL>, <GROUP> and </GROUP> tags;
// in M3U playlists lines starting with '#' are comments. See Load.
func Parse(r io.Reader, g *Group, baseDir string, isM3U bool) (GlobalSettings, error) {
	var gs GlobalSettings
	cur := g
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if line == "" {
			continue
		}
		if isM3U && strings.HasPrefix(line, "#") {
			continue
		}
		if line[0] == '<' {
			tag, ok := parseTag(line)
			if !ok {
				continue
			}
			switch strings.ToLower(tag.name) {
			case "global":
				if v, ok := tag.attr("playmode"); ok {
					gs.PlayMode = v
				}
				if v, ok := tag.attr("mode"); ok && strings.EqualFold(v, "shuffle") {
					gs.Shuffle = true
				}
			case "group":
				name, ok := tag.attr("name")
				if !ok {
					continue
				}
				if r := []rune(name); len(r) > maxGroupNameLen {
					name = string(r[:maxGroupNameLen])
				}
				ng := NewGroup("[" + name + "]")
				if v, ok := tag.attr("mode"); ok && strings.EqualFold(v, "shuffle") {
					ng.Shuffle = true
				}
				cur.AddGroup(ng)
				cur = ng
			case "/group":
				if cur.Parent != nil && cur != g {
					cur = cur.Parent
				}
			}
			continue
		}
		cur.AddSong(resolve(line, baseDir))
	}
	return gs, sc.Err()
}

// resolve turns a song path of a playlist into a full path or URL.
func resolve(p, base string) string {
	if media.IsHTTPStream(p) || filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return p
	}
	if strings.HasPrefix(p, "~") {
		return media.ExpandPath(p)
	}
	return filepath.Join(base, filepath.FromSlash(p))
}

// Save writes root (recursively) to path in mp3blaster's playlist format.
// The file is replaced atomically, so a failure never leaves a truncated
// playlist behind.
func Save(path string, root *Group, playMode string) error {
	var b bytes.Buffer
	if err := Write(&b, root, playMode); err != nil {
		return err
	}
	return atomicfile.Write(path, b.Bytes(), 0o644)
}

// SaveM3U writes the songs of root (recursively) to path as an extended M3U
// playlist. Songs in sub groups carry a group-title tag naming their group.
// The file is replaced atomically.
func SaveM3U(path string, root *Group) error {
	var pl m3u.Playlist
	collectTracks(&pl, root, "")
	r, err := m3u.Marshall(pl)
	if err != nil {
		return err
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return atomicfile.Write(path, b, 0o644)
}

// collectTracks appends the songs of g to pl; group is the group-title of g.
func collectTracks(pl *m3u.Playlist, g *Group, group string) {
	for _, it := range g.Items {
		switch it.Kind {
		case KindGroup:
			name := strings.TrimSuffix(strings.TrimPrefix(it.Group.Title, "["), "]")
			if group != "" {
				name = group + m3uGroupSep + name
			}
			collectTracks(pl, it.Group, name)
		case KindSong:
			name := media.ChopPath(it.Path)
			if !media.IsHTTPStream(it.Path) {
				name = strings.TrimSuffix(name, filepath.Ext(name))
			}
			t := m3u.Track{Name: name, Length: -1, URI: it.Path}
			if group != "" {
				t.Tags = []m3u.Tag{{Name: m3uGroupTag, Value: group}}
			}
			pl.Tracks = append(pl.Tracks, t)
		}
	}
}

// Write serialises root to w. playMode is stored in the <GLOBAL> tag.
func Write(w io.Writer, root *Group, playMode string) error {
	shuffle := ""
	if root.Shuffle {
		shuffle = ` MODE="shuffle"`
	}
	if _, err := fmt.Fprintf(w, "<GLOBAL PLAYMODE=\"%s\"%s>\n", playMode, shuffle); err != nil {
		return err
	}
	return writeGroup(w, root, 0)
}

// writeGroup writes the entries of g, indented by depth tabs.
func writeGroup(w io.Writer, g *Group, depth int) error {
	indent := strings.Repeat("\t", depth)
	for _, it := range g.Items {
		switch it.Kind {
		case KindParent:
			continue
		case KindGroup:
			name := strings.TrimSuffix(strings.TrimPrefix(it.Group.Title, "["), "]")
			mode := ""
			if it.Group.Shuffle {
				mode = ` MODE="shuffle"`
			}
			if _, err := fmt.Fprintf(w, "%s<GROUP NAME=\"%s\"%s>\n", indent, escape(name), mode); err != nil {
				return err
			}
			if err := writeGroup(w, it.Group, depth+1); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "%s</GROUP>\n", indent); err != nil {
				return err
			}
		default:
			if _, err := fmt.Fprintf(w, "%s%s\n", indent, it.Path); err != nil {
				return err
			}
		}
	}
	return nil
}

// escape protects the characters that parseTag treats specially.
func escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// tag is a parsed playlist tag such as <GROUP NAME="x">.
type tag struct {
	name  string
	attrs [][2]string
}

// attr returns the value of attribute key (case-insensitive).
func (t tag) attr(key string) (string, bool) {
	for _, a := range t.attrs {
		if strings.EqualFold(a[0], key) {
			return a[1], true
		}
	}
	return "", false
}

// parseTag parses an HTML-like tag such as <GROUP NAME="x" MODE="shuffle">.
func parseTag(s string) (tag, bool) {
	if len(s) < 2 || s[0] != '<' || s[len(s)-1] != '>' {
		return tag{}, false
	}
	body := []rune(s[1 : len(s)-1])
	var t tag
	i := 0
	for i < len(body) && body[i] != ' ' && body[i] != '\t' {
		i++
	}
	t.name = string(body[:i])
	for {
		for i < len(body) && (body[i] == ' ' || body[i] == '\t') {
			i++
		}
		if i >= len(body) {
			return t, true
		}
		start := i
		for i < len(body) && body[i] != '=' && body[i] != ' ' && body[i] != '\t' {
			i++
		}
		key := string(body[start:i])
		for i < len(body) && body[i] != '"' {
			i++
		}
		if i >= len(body) {
			return tag{}, false
		}
		i++
		var val strings.Builder
		closed := false
		for i < len(body) {
			c := body[i]
			if c == '\\' && i+1 < len(body) {
				val.WriteRune(body[i+1])
				i += 2
				continue
			}
			i++
			if c == '"' {
				closed = true
				break
			}
			val.WriteRune(c)
		}
		if !closed {
			return tag{}, false
		}
		t.attrs = append(t.attrs, [2]string{key, val.String()})
	}
}
