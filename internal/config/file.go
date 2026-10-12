package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mingcheng/mp3blaster/internal/filemanager"
	"github.com/mingcheng/mp3blaster/internal/keys"
	"github.com/mingcheng/mp3blaster/internal/listview"
	"github.com/mingcheng/mp3blaster/internal/media"
	"github.com/mingcheng/mp3blaster/internal/playback"
)

type valueType int

const (
	typeNumber valueType = iota
	typeBool
	typeKey
	typeColour
	typeAny
)

type keyword struct {
	typ   valueType
	multi bool
	apply func(o *Options, vals []string) error
}

var colourNames = map[string]bool{
	"black": true, "red": true, "green": true, "yellow": true,
	"blue": true, "magenta": true, "cyan": true, "white": true, "default": true,
}

// colourKeywords lists the colour settings (lower case, without "color.").
var colourKeywords = []string{
	"default.fg", "default.bg", "popup.fg", "popup.bg", "popupinput.fg",
	"popupinput.bg", "error.fg", "error.bg", "button.fg", "button.bg",
	"progressbar.bg", "bartoken.fg", "bartoken.bg", "shortcut.fg",
	"shortcut.bg", "label.fg", "label.bg", "number.fg", "number.bg",
	"filemp3.fg", "filedir.fg", "filelst.fg", "filewin.fg",
}

func num(dst *int, check func(int) bool) func(*Options, []string) error {
	return func(o *Options, v []string) error {
		n, _ := strconv.Atoi(v[0])
		if check != nil && !check(n) {
			return fmt.Errorf("bad value %q", v[0])
		}
		*dst = n
		return nil
	}
}

func percent(n int) bool { return n >= 0 && n <= 100 }

func keywords(o *Options) map[string]keyword {
	b := func(dst *bool) keyword {
		return keyword{typ: typeBool, apply: func(_ *Options, v []string) error {
			*dst = parseBool(v[0])
			return nil
		}}
	}
	s := func(dst *string) keyword {
		return keyword{typ: typeAny, apply: func(_ *Options, v []string) error {
			*dst = v[0]
			return nil
		}}
	}
	kw := map[string]keyword{
		"threads": {typ: typeNumber, apply: func(o *Options, v []string) error {
			n, _ := strconv.Atoi(v[0])
			if !o.SetThreads(n) {
				return fmt.Errorf("bad value %q (0..500, multiple of 50)", v[0])
			}
			return nil
		}},
		"downfrequency": b(&o.Downsample),
		"sounddevice":   s(&o.SoundDevice),
		"audiofilematching": {typ: typeAny, multi: true, apply: func(o *Options, v []string) error {
			o.AudioPatterns = v
			return nil
		}},
		"warndelay":  {typ: typeNumber, apply: num(&o.WarnDelay, nil)},
		"skiplength": {typ: typeNumber, apply: num(&o.SkipSeconds, func(n int) bool { return n >= 0 })},
		"wraparound": b(&o.Wrap),
		"playlistmatching": {typ: typeAny, multi: true, apply: func(o *Options, v []string) error {
			o.PlaylistPatterns = v
			return nil
		}},
		"playlistdir":    {typ: typeAny, apply: func(o *Options, v []string) error { o.PlaylistDir = media.ExpandPath(v[0]); return nil }},
		"hideotherfiles": b(&o.HideOthers),
		"file.sortmode": {typ: typeAny, apply: func(o *Options, v []string) error {
			m, err := filemanager.ParseSortMode(v[0])
			if err != nil {
				return err
			}
			o.SortMode = m
			return nil
		}},
		"file.id3names":                     b(&o.ID3Names),
		"selectitems.unselectfirst":         b(&o.SelectUnselectFirst),
		"selectitems.searchregex":           b(&o.SelectRegex),
		"selectitems.searchcaseinsensitive": b(&o.SelectCaseInsensitive),
		"scanmp3":                           b(&o.ScanMP3),
		"mixerdevice":                       s(&o.MixerDevice),
		"charsettable":                      s(&o.CharsetTable),
		"pansize":                           {typ: typeNumber, apply: num(&o.PanSize, func(n int) bool { return n >= 1 && n <= 40 })},
		"audiodriver":                       s(&o.AudioDriver),
		"theme": {typ: typeAny, apply: func(o *Options, v []string) error {
			t := strings.ToLower(v[0])
			if t != "classic" && t != "modern" {
				return fmt.Errorf("unknown theme %q", v[0])
			}
			o.Theme = t
			return nil
		}},
		"listwidth": {typ: typeAny, apply: func(o *Options, v []string) error {
			w, err := listview.ParseWidth(v[0])
			if err != nil {
				return err
			}
			o.ListWidth = w
			return nil
		}},
		"nerdfont": b(&o.NerdFont),
		"mouse":    b(&o.Mouse),

		// State saved on exit.
		"savestate": b(&o.SaveState),
		"repeat":    b(&o.Repeat),
		"playmode": {typ: typeAny, apply: func(o *Options, v []string) error {
			m, err := playback.ParsePlayMode(v[0])
			if err != nil {
				return err
			}
			o.PlayMode = m
			return nil
		}},
		"volume":       {typ: typeNumber, apply: num(&o.Volume, percent)},
		"balance":      {typ: typeNumber, apply: num(&o.Balance, percent)},
		"lastdir":      {typ: typeAny, apply: func(o *Options, v []string) error { o.LastDir = media.ExpandPath(v[0]); return nil }},
		"lastsong":     s(&o.LastSong),
		"lastposition": {typ: typeNumber, apply: num(&o.LastPosition, func(n int) bool { return n >= 0 })},
	}
	for _, c := range colourKeywords {
		name := c
		kw["color."+name] = keyword{typ: typeColour, apply: func(o *Options, v []string) error {
			o.Colours[name] = strings.ToLower(v[0])
			return nil
		}}
	}
	return kw
}

func parseBool(s string) bool {
	switch strings.ToLower(s) {
	case "yes", "y", "true", "on", "1":
		return true
	}
	return false
}

func validBool(s string) bool {
	switch strings.ToLower(s) {
	case "yes", "y", "true", "on", "1", "no", "n", "false", "off", "0":
		return true
	}
	return false
}

// Dir returns the directory holding the configuration file and other
// resources such as temporary data, caches and logs
// ($HOME/.config/mp3blaster).
func Dir() string {
	return filepath.Join(media.HomeDir(), ".config", "mp3blaster")
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
// ($HOME/.config/mp3blaster/mp3blasterrc).
func DefaultPath() string {
	return filepath.Join(Dir(), "mp3blasterrc")
}

// LegacyPath returns the configuration file used by earlier versions
// (~/.mp3blasterrc). It is read when DefaultPath does not exist.
func LegacyPath() string {
	return filepath.Join(media.HomeDir(), ".mp3blasterrc")
}

// LoadFile reads a configuration file. A missing default file is not an
// error; a missing explicitly requested file is.
func (o *Options) LoadFile(path string) error {
	explicit := path != ""
	if !explicit {
		path = DefaultPath()
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			if _, err := os.Stat(LegacyPath()); err == nil {
				path = LegacyPath()
			}
		}
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

// Parse reads configuration lines ("Keyword = value[, value...]").
func (o *Options) Parse(r io.Reader) error {
	kws := keywords(o)
	sc := bufio.NewScanner(r)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || text[0] == '#' {
			continue
		}
		name, rest, ok := strings.Cut(text, "=")
		name, rest = strings.TrimSpace(name), strings.TrimSpace(rest)
		if !ok || name == "" || rest == "" {
			return fmt.Errorf("line %d: Syntax error", line)
		}
		vals := splitValues(rest)
		lname := strings.ToLower(name)
		if strings.HasPrefix(lname, "key.") {
			cmd, ok := keys.CommandForConfigName(lname)
			if !ok {
				return fmt.Errorf("line %d: Unknown keyword %q", line, name)
			}
			if len(vals) != 1 {
				return fmt.Errorf("line %d: Too many values", line)
			}
			k, err := keys.Parse(vals[0])
			if err != nil {
				return fmt.Errorf("line %d: %v", line, err)
			}
			if cmd != keys.CmdNone {
				o.Keys = append(o.Keys, KeyBinding{Cmd: cmd, Key: k})
			}
			continue
		}
		kw, ok := kws[lname]
		if !ok {
			return fmt.Errorf("line %d: Unknown keyword %q", line, name)
		}
		if !kw.multi && len(vals) > 1 {
			return fmt.Errorf("line %d: Too many values", line)
		}
		for _, v := range vals {
			if err := checkType(kw.typ, v); err != nil {
				return fmt.Errorf("line %d: %v", line, err)
			}
		}
		if err := kw.apply(o, vals); err != nil {
			return fmt.Errorf("line %d: %v", line, err)
		}
	}
	return sc.Err()
}

func checkType(t valueType, v string) error {
	switch t {
	case typeNumber:
		if _, err := strconv.Atoi(v); err != nil {
			return fmt.Errorf("Bad value type: %q is not a number", v)
		}
	case typeBool:
		if !validBool(v) {
			return fmt.Errorf("Bad value type: %q is not a boolean", v)
		}
	case typeColour:
		l := strings.ToLower(v)
		if !colourNames[l] && !(strings.HasPrefix(l, "#") && len(l) == 7) {
			return fmt.Errorf("Bad value type: %q is not a colour", v)
		}
	}
	return nil
}

// splitValues splits a value list on commas. A backslash escapes a comma
// or a backslash; other escaped characters are dropped, as in the original.
func splitValues(s string) []string {
	var vals []string
	var cur strings.Builder
	esc := false
	for _, r := range s {
		switch {
		case esc:
			if r == '\\' || r == ',' {
				cur.WriteRune(r)
			}
			esc = false
		case r == '\\':
			esc = true
		case r == ',':
			vals = append(vals, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	return append(vals, strings.TrimSpace(cur.String()))
}
