package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mingcheng/mp3blstr/internal/filemanager"
	"github.com/mingcheng/mp3blstr/internal/keys"
	"github.com/mingcheng/mp3blstr/internal/listview"
	"github.com/mingcheng/mp3blstr/internal/media"
	"github.com/mingcheng/mp3blstr/internal/playback"
	"github.com/mingcheng/mp3blstr/internal/playlist"
)

// valueType is the type of a configuration value, checked before the value
// is applied.
type valueType int

const (
	typeAny valueType = iota
	typeNumber
	typeBool
	typeColour
)

// keyword describes a configuration setting. apply stores the (type
// checked) values in the Options the keyword table was built for.
type keyword struct {
	typ   valueType
	multi bool // accepts a list of values
	apply func(vals []string) error
}

var colourNames = map[string]bool{
	"black": true, "red": true, "green": true, "yellow": true,
	"blue": true, "magenta": true, "cyan": true, "white": true, "default": true,
}

// colourSettings lists the colour settings below ui.colors. Options.Colours
// uses the same names without underscores ("popupinput.fg").
var colourSettings = []string{
	"default.fg", "default.bg", "popup.fg", "popup.bg", "popup_input.fg",
	"popup_input.bg", "error.fg", "error.bg", "button.fg", "button.bg",
	"progress_bar.bg", "bar_token.fg", "bar_token.bg", "shortcut.fg",
	"shortcut.bg", "label.fg", "label.bg", "number.fg", "number.bg",
	"file_mp3.fg", "file_dir.fg", "file_lst.fg", "file_win.fg",
}

// keysSection holds the key bindings; its settings are resolved by
// keys.CommandForConfigName instead of the keyword table.
const keysSection = "keys"

// Keyword constructors for the common cases.

func boolKw(dst *bool) keyword {
	return keyword{typ: typeBool, apply: func(v []string) error {
		*dst, _ = parseBool(v[0]) // validated by checkType
		return nil
	}}
}

func stringKw(dst *string) keyword {
	return keyword{typ: typeAny, apply: func(v []string) error {
		*dst = v[0]
		return nil
	}}
}

func pathKw(dst *string) keyword {
	return keyword{typ: typeAny, apply: func(v []string) error {
		*dst = media.ExpandPath(v[0])
		return nil
	}}
}

func listKw(dst *[]string) keyword {
	return keyword{typ: typeAny, multi: true, apply: func(v []string) error {
		*dst = v
		return nil
	}}
}

// numberKw stores a number; valid (optional) restricts the accepted range.
func numberKw(dst *int, valid func(int) bool) keyword {
	return keyword{typ: typeNumber, apply: func(v []string) error {
		n, _ := strconv.Atoi(v[0]) // validated by checkType
		if valid != nil && !valid(n) {
			return fmt.Errorf("bad value %q", v[0])
		}
		*dst = n
		return nil
	}}
}

// parsedKw stores the result of parse.
func parsedKw[T any](dst *T, parse func(string) (T, error)) keyword {
	return keyword{typ: typeAny, apply: func(v []string) error {
		x, err := parse(v[0])
		if err != nil {
			return err
		}
		*dst = x
		return nil
	}}
}

func percent(n int) bool     { return n >= 0 && n <= 100 }
func nonNegative(n int) bool { return n >= 0 }

// keywords returns the settings table for o, indexed by the dotted path of
// the setting in the YAML file ("playback.volume"). Key bindings (below
// keysSection) are handled separately.
func keywords(o *Options) map[string]keyword {
	kw := map[string]keyword{
		"ui.theme":          parsedKw(&o.Theme, ParseTheme),
		"ui.nerd_font":      boolKw(&o.NerdFont),
		"ui.mouse":          boolKw(&o.Mouse),
		"ui.list_width":     parsedKw(&o.ListWidth, listview.ParseWidth),
		"ui.display_mode":   parsedKw(&o.DisplayMode, playlist.ParseDisplayMode),
		"ui.wrap_around":    boolKw(&o.Wrap),
		"ui.scroll_playing": boolKw(&o.ScrollPlaying),
		"ui.follow_playing": boolKw(&o.FollowPlaying),
		"ui.confirm_quit":   boolKw(&o.ConfirmQuit),
		"ui.pan_size":       numberKw(&o.PanSize, func(n int) bool { return n >= 1 && n <= 40 }),

		"playback.play_mode":              parsedKw(&o.PlayMode, playback.ParsePlayMode),
		"playback.media_keys":             boolKw(&o.MediaKeys),
		"playback.pause_on_sleep":         boolKw(&o.PauseOnSleep),
		"playback.pause_on_output_change": boolKw(&o.PauseOnOutputChange),
		"playback.repeat":                 boolKw(&o.Repeat),
		"playback.fade_out":               boolKw(&o.FadeOut),
		"playback.fade_seconds":           numberKw(&o.FadeSeconds, nonNegative),
		"playback.fade_in_seconds":        numberKw(&o.FadeInSeconds, nonNegative),
		"playback.threads": {typ: typeNumber, apply: func(v []string) error {
			n, _ := strconv.Atoi(v[0])
			if !o.SetThreads(n) {
				return fmt.Errorf("bad value %q (0..500, multiple of 50)", v[0])
			}
			return nil
		}},
		"playback.skip_length":    numberKw(&o.SkipSeconds, nonNegative),
		"playback.down_frequency": boolKw(&o.Downsample),
		"playback.volume":         numberKw(&o.Volume, percent),
		"playback.balance":        numberKw(&o.Balance, percent),

		"files.sort_mode":         parsedKw(&o.SortMode, filemanager.ParseSortMode),
		"files.display_mode":      parsedKw(&o.FileDisplayMode, filemanager.ParseDisplayMode),
		"files.hide_others":       boolKw(&o.HideOthers),
		"files.use_trash":         boolKw(&o.UseTrash),
		"files.id3_names":         boolKw(&o.ID3Names),
		"files.guess_legacy_tags": boolKw(&o.GuessLegacyTags),
		"files.metadata_cache":    boolKw(&o.MetadataCache),
		"files.scan_mp3":          boolKw(&o.ScanMP3),
		"files.playlist_dir":      pathKw(&o.PlaylistDir),
		"files.audio_patterns":    listKw(&o.AudioPatterns),
		"files.playlist_patterns": listKw(&o.PlaylistPatterns),
		"files.charset_table":     stringKw(&o.CharsetTable),

		"selection.unselect_first":   boolKw(&o.SelectUnselectFirst),
		"selection.regex":            boolKw(&o.SelectRegex),
		"selection.case_insensitive": boolKw(&o.SelectCaseInsensitive),

		"state.save":          boolKw(&o.SaveState),
		"state.last_dir":      pathKw(&o.LastDir),
		"state.last_song":     stringKw(&o.LastSong),
		"state.last_position": numberKw(&o.LastPosition, nonNegative),
	}
	for _, name := range colourSettings {
		key := strings.ReplaceAll(name, "_", "")
		kw["ui.colors."+name] = keyword{typ: typeColour, apply: func(v []string) error {
			o.Colours[key] = strings.ToLower(v[0])
			return nil
		}}
	}
	return kw
}

// sections returns the paths of the YAML mappings that contain settings.
func sections(kws map[string]keyword) map[string]bool {
	s := map[string]bool{keysSection: true}
	for path := range kws {
		for i, r := range path {
			if r == '.' {
				s[path[:i]] = true
			}
		}
	}
	return s
}

// parseBool parses a boolean configuration value; ok is false when s is
// not a boolean at all.
func parseBool(s string) (v, ok bool) {
	switch strings.ToLower(s) {
	case "yes", "y", "true", "on", "1":
		return true, true
	case "no", "n", "false", "off", "0":
		return false, true
	}
	return false, false
}

// Dir returns the directory holding the configuration file and other
// resources such as temporary data, caches and logs
// ($HOME/.config/mp3blstr).
func Dir() string {
	return filepath.Join(media.HomeDir(), ".config", "mp3blstr")
}

// EnsureDir creates Dir if it does not exist yet and returns it.
func EnsureDir() (string, error) {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return dir, err
	}
	return dir, nil
}

// DefaultPath returns the default configuration file
// ($HOME/.config/mp3blstr/config.yaml).
func DefaultPath() string {
	return filepath.Join(Dir(), "config.yaml")
}

// LoadFile reads a configuration file. A missing default file is not an
// error; a missing explicitly requested file is.
func (o *Options) LoadFile(path string) error {
	explicit := path != ""
	if !explicit {
		path = DefaultPath()
	}
	f, err := os.Open(path)
	if err != nil {
		if !explicit && errors.Is(err, fs.ErrNotExist) {
			o.Path = path
			return nil
		}
		return fmt.Errorf("could not open configuration file: %w", err)
	}
	defer f.Close()
	o.Path = path
	if err := o.Parse(f); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// Parse reads a YAML configuration document. Setting names are matched
// case-insensitively; unknown settings are an error.
func (o *Options) Parse(r io.Reader) error {
	var doc yaml.Node
	if err := yaml.NewDecoder(r).Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	root := documentRoot(&doc)
	if root == nil || isNull(root) {
		return nil
	}
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: the configuration must be a YAML mapping of settings", root.Line)
	}
	kws := keywords(o)
	p := parser{o: o, kws: kws, sections: sections(kws)}
	return p.mapping(root, "")
}

type parser struct {
	o        *Options
	kws      map[string]keyword
	sections map[string]bool
}

// mapping applies the settings of the YAML mapping n found at path prefix.
func (p *parser) mapping(n *yaml.Node, prefix string) error {
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], resolve(n.Content[i+1])
		name := strings.ToLower(k.Value)
		if k.Kind != yaml.ScalarNode || name == "" {
			return fmt.Errorf("line %d: bad setting name", k.Line)
		}
		if seen[name] {
			return fmt.Errorf("line %d: %q is set more than once", k.Line, prefix+k.Value)
		}
		seen[name] = true
		path := prefix + name

		if path == keysSection || strings.HasPrefix(path, keysSection+".") {
			if err := p.key(v, path); err != nil {
				return err
			}
			continue
		}
		if p.sections[path] {
			if isNull(v) {
				continue
			}
			if v.Kind != yaml.MappingNode {
				return fmt.Errorf("line %d: %q must be a mapping of settings", v.Line, path)
			}
			if err := p.mapping(v, path+"."); err != nil {
				return err
			}
			continue
		}
		kw, ok := p.kws[path]
		if !ok {
			return fmt.Errorf("line %d: unknown setting %q", k.Line, path)
		}
		if kw.typ == typeColour && isNull(v) && v.Value == "" && hexComment(k, v) {
			return fmt.Errorf("line %d: %s: quote colour values (\"#rrggbb\"), # starts a YAML comment", k.Line, path)
		}
		if err := p.value(kw, v, path); err != nil {
			return err
		}
	}
	return nil
}

// value type checks and applies the value v of a setting. A null value
// leaves the setting unchanged.
func (p *parser) value(kw keyword, v *yaml.Node, path string) error {
	var vals []string
	switch {
	case isNull(v):
		return nil
	case v.Kind == yaml.ScalarNode:
		vals = []string{v.Value}
	case v.Kind == yaml.SequenceNode && kw.multi:
		for _, e := range v.Content {
			if e = resolve(e); e.Kind != yaml.ScalarNode || isNull(e) {
				return fmt.Errorf("line %d: %s: expected a list of values", e.Line, path)
			}
			vals = append(vals, e.Value)
		}
	case kw.multi:
		return fmt.Errorf("line %d: %s: expected a list of values", v.Line, path)
	default:
		return fmt.Errorf("line %d: %s: expected a single value", v.Line, path)
	}
	for _, s := range vals {
		if err := checkType(kw.typ, s); err != nil {
			return fmt.Errorf("line %d: %s: %v", v.Line, path, err)
		}
	}
	if err := kw.apply(vals); err != nil {
		return fmt.Errorf("line %d: %s: %v", v.Line, path, err)
	}
	return nil
}

// key applies the key binding (or the mapping of key bindings) v found at
// path below keysSection. Command names are the snake_case form of the
// keys.CommandForConfigName names: keys.play.next, keys.file.add_files.
func (p *parser) key(v *yaml.Node, path string) error {
	switch {
	case isNull(v):
		return nil
	case v.Kind == yaml.MappingNode:
		return p.mapping(v, path+".")
	case v.Kind != yaml.ScalarNode || path == keysSection:
		return fmt.Errorf("line %d: %s: expected a key binding", v.Line, path)
	}
	name := "key." + strings.ReplaceAll(strings.TrimPrefix(path, keysSection+"."), "_", "")
	cmd, ok := keys.CommandForConfigName(name)
	if !ok {
		return fmt.Errorf("line %d: unknown key binding %q", v.Line, path)
	}
	k, err := keys.Parse(v.Value)
	if err != nil {
		return fmt.Errorf("line %d: %s: %v", v.Line, path, err)
	}
	if cmd != keys.CmdNone {
		p.o.Keys = append(p.o.Keys, KeyBinding{Cmd: cmd, Key: k})
	}
	return nil
}

// hexComment reports whether the comment following key k (with the empty
// value v) looks like an unquoted "#rrggbb" colour.
func hexComment(k, v *yaml.Node) bool {
	for _, c := range []string{k.LineComment, v.LineComment} {
		c = strings.TrimSpace(c)
		if len(c) >= 7 && c[0] == '#' && strings.Trim(c[1:7], "0123456789abcdefABCDEF") == "" {
			return true
		}
	}
	return false
}

// documentRoot returns the top level node of a decoded document, or nil
// for an empty one.
func documentRoot(doc *yaml.Node) *yaml.Node {
	switch doc.Kind {
	case 0:
		return nil
	case yaml.DocumentNode:
		if len(doc.Content) == 0 {
			return nil
		}
		return resolve(doc.Content[0])
	}
	return doc
}

// resolve follows YAML aliases.
func resolve(n *yaml.Node) *yaml.Node {
	for n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	return n
}

func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null"
}

// checkType validates a value against the type of its keyword.
func checkType(t valueType, v string) error {
	switch t {
	case typeNumber:
		if _, err := strconv.Atoi(v); err != nil {
			return fmt.Errorf("bad value type: %q is not a number", v)
		}
	case typeBool:
		if _, ok := parseBool(v); !ok {
			return fmt.Errorf("bad value type: %q is not a boolean", v)
		}
	case typeColour:
		l := strings.ToLower(v)
		if !colourNames[l] && !(strings.HasPrefix(l, "#") && len(l) == 7) {
			return fmt.Errorf("bad value type: %q is not a colour", v)
		}
	}
	return nil
}

// yamlTag returns the YAML tag used to write the value of the setting at
// path.
func yamlTag(path string) string {
	kw := keywords(Default())[strings.ToLower(path)]
	switch kw.typ {
	case typeNumber:
		return "!!int"
	case typeBool:
		return "!!bool"
	}
	return "!!str"
}
