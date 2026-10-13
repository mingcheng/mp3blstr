package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mingcheng/mp3blstr/internal/atomicfile"
)

// Setting is a setting of the configuration file, named by its dotted path
// ("playback.volume"). An empty Value removes the setting from the file.
type Setting struct {
	Name  string
	Value string
}

// savedComment heads a configuration file created by SaveSettings.
const savedComment = "Saved automatically by mp3blstr on exit (state.save: false disables this)."

// SessionPath returns the file holding the playlist of the last session.
func SessionPath() string { return filepath.Join(Dir(), "session.lst") }

// StateSettings returns the options that mp3blstr remembers between
// sessions, as configuration file settings.
func (o *Options) StateSettings() []Setting {
	pos := ""
	if o.LastSong != "" {
		pos = strconv.Itoa(o.LastPosition)
	}
	return []Setting{
		{"ui.theme", o.Theme},
		{"ui.nerd_font", strconv.FormatBool(o.NerdFont)},
		{"ui.wrap_around", strconv.FormatBool(o.Wrap)},
		{"ui.display_mode", o.DisplayMode.String()},
		{"playback.threads", strconv.Itoa(o.Threads)},
		{"files.sort_mode", o.SortMode.String()},
		{"files.display_mode", o.FileDisplayMode.String()},
		{"files.id3_names", strconv.FormatBool(o.ID3Names)},
		{"playback.play_mode", o.PlayMode.String()},
		{"playback.repeat", strconv.FormatBool(o.Repeat)},
		{"playback.fade_out", strconv.FormatBool(o.FadeOut)},
		{"playback.volume", strconv.Itoa(o.Volume)},
		{"playback.balance", strconv.Itoa(o.Balance)},
		{"state.last_dir", o.LastDir},
		{"state.last_song", o.LastSong},
		{"state.last_position", pos},
	}
}

// ChangedSettings returns the settings in now whose value differs from the
// same setting in before.
func ChangedSettings(before, now []Setting) []Setting {
	old := map[string]string{}
	for _, s := range before {
		old[strings.ToLower(s.Name)] = s.Value
	}
	var out []Setting
	for _, s := range now {
		if v, ok := old[strings.ToLower(s.Name)]; !ok || v != s.Value {
			out = append(out, s)
		}
	}
	return out
}

// SaveSettings writes settings into the YAML configuration file at path.
// Existing values are updated in place and removed settings lose their
// line; new settings are added to their section, which is created at the
// end of the file when needed. Comments, blank lines and all other content
// are kept (documents that cannot be edited line by line, such as flow
// style ones, are re-encoded instead). The file is created when it does
// not exist, and replaced atomically otherwise; symbolic links are
// followed.
func SaveSettings(path string, settings []Setting) error {
	if len(settings) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
	default:
		return err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	out, err := editText(text, settings)
	if errors.Is(err, errUnsupported) {
		out, err = rewriteDocument([]byte(text), settings)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return atomicfile.Write(path, []byte(out), 0o644)
}

// rewriteDocument applies settings to the decoded document and encodes it
// again. Comments survive, but blank lines and the original formatting do
// not.
func rewriteDocument(data []byte, settings []Setting) (string, error) {
	doc, err := editableDocument(data)
	if err != nil {
		return "", err
	}
	root := doc.Content[0]
	for _, s := range settings {
		segs := strings.Split(strings.ToLower(s.Name), ".")
		if s.Value == "" {
			removeSetting(root, segs)
			continue
		}
		setSetting(root, segs, &yaml.Node{Kind: yaml.ScalarNode, Tag: yamlTag(s.Name), Value: s.Value})
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// editableDocument decodes data into a document whose root is a mapping.
func editableDocument(data []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode {
		doc = yaml.Node{Kind: yaml.DocumentNode}
	}
	if len(doc.Content) == 0 {
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
		if len(bytes.TrimSpace(data)) == 0 {
			doc.HeadComment = savedComment
		}
	}
	root := doc.Content[0]
	if isNull(root) {
		*root = yaml.Node{Kind: yaml.MappingNode, HeadComment: root.HeadComment,
			LineComment: root.LineComment, FootComment: root.FootComment}
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("line %d: the configuration must be a YAML mapping of settings", root.Line)
	}
	return &doc, nil
}

// lookup returns the index of the value of name in the mapping m, or -1.
// Names are matched case-insensitively; the last match wins, as for
// duplicated keys.
func lookup(m *yaml.Node, name string) int {
	idx := -1
	for i := 0; i+1 < len(m.Content); i += 2 {
		if strings.EqualFold(m.Content[i].Value, name) {
			idx = i + 1
		}
	}
	return idx
}

// setSetting stores val at the path segs below the mapping m, creating the
// missing sections. Comments of a replaced value are kept.
func setSetting(m *yaml.Node, segs []string, val *yaml.Node) {
	for len(segs) > 1 {
		i := lookup(m, segs[0])
		if i < 0 {
			child := &yaml.Node{Kind: yaml.MappingNode}
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: segs[0]}, child)
			m, segs = child, segs[1:]
			continue
		}
		if child := m.Content[i]; child.Kind != yaml.MappingNode {
			m.Content[i] = &yaml.Node{Kind: yaml.MappingNode, LineComment: child.LineComment, FootComment: child.FootComment}
		}
		m, segs = m.Content[i], segs[1:]
	}
	if i := lookup(m, segs[0]); i >= 0 {
		old := m.Content[i]
		val.HeadComment, val.LineComment, val.FootComment = old.HeadComment, old.LineComment, old.FootComment
		m.Content[i] = val
		return
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: segs[0]}, val)
}

// removeSetting deletes the setting at the path segs below the mapping m,
// and the sections that become empty because of it.
func removeSetting(m *yaml.Node, segs []string) {
	i := lookup(m, segs[0])
	if i < 0 {
		return
	}
	if len(segs) > 1 {
		child := m.Content[i]
		if child.Kind != yaml.MappingNode {
			return
		}
		removeSetting(child, segs[1:])
		if len(child.Content) > 0 {
			return
		}
	}
	m.Content = append(m.Content[:i-1], m.Content[i+1:]...)
}
