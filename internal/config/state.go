package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Setting is a "Keyword = value" line of the configuration file. An empty
// Value removes the keyword from the file.
type Setting struct {
	Name  string
	Value string
}

// savedMarker introduces the settings appended by SaveSettings.
const savedMarker = "# Saved automatically by mp3blaster on exit (SaveState = no disables this)."

// SessionPath returns the file holding the playlist of the last session.
func SessionPath() string { return filepath.Join(Dir(), "session.lst") }

// StateSettings returns the options that mp3blaster remembers between
// sessions, as configuration file settings.
func (o *Options) StateSettings() []Setting {
	yn := func(v bool) string {
		if v {
			return "yes"
		}
		return "no"
	}
	playMode := o.PlayMode.String()
	if playMode == "unknown" {
		playMode = ""
	}
	pos := ""
	if o.LastSong != "" {
		pos = strconv.Itoa(o.LastPosition)
	}
	return []Setting{
		{"Theme", o.Theme},
		{"NerdFont", yn(o.NerdFont)},
		{"WrapAround", yn(o.Wrap)},
		{"Threads", strconv.Itoa(o.Threads)},
		{"File.SortMode", o.SortMode.String()},
		{"PlayMode", playMode},
		{"Repeat", yn(o.Repeat)},
		{"Volume", strconv.Itoa(o.Volume)},
		{"Balance", strconv.Itoa(o.Balance)},
		{"LastDir", o.LastDir},
		{"LastSong", o.LastSong},
		{"LastPosition", pos},
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

// escapeValue protects the characters that splitValues treats specially.
func escapeValue(v string) string {
	return strings.NewReplacer(`\`, `\\`, `,`, `\,`).Replace(v)
}

// lineKeyword returns the lower case keyword of an active configuration
// line, or "" for blank lines and comments.
func lineKeyword(line string) string {
	t := strings.TrimSpace(line)
	if t == "" || t[0] == '#' {
		return ""
	}
	name, _, ok := strings.Cut(t, "=")
	if !ok {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(name))
}

// SaveSettings writes settings into the configuration file at path. The
// last active line of each keyword is updated in place (keeping comments and
// the rest of the file); settings not present yet are appended after a
// marker comment. The file is created when it does not exist; symbolic links
// are followed.
func SaveSettings(path string, settings []Setting) error {
	if len(settings) == 0 {
		return nil
	}
	if p, err := filepath.EvalSymlinks(path); err == nil {
		path = p
	}
	mode := fs.FileMode(0o644)
	var lines []string
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if st, err := os.Stat(path); err == nil {
			mode = st.Mode().Perm()
		}
		text := strings.TrimSuffix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		if text != "" {
			lines = strings.Split(text, "\n")
		}
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
	default:
		return err
	}

	var appended []string
	for _, s := range settings {
		key := strings.ToLower(s.Name)
		last := -1
		for i, l := range lines {
			if lineKeyword(l) == key {
				last = i
			}
		}
		if s.Value == "" {
			kept := lines[:0]
			for _, l := range lines {
				if lineKeyword(l) != key {
					kept = append(kept, l)
				}
			}
			lines = kept
			continue
		}
		if last >= 0 {
			l := lines[last]
			indent := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
			name, _, _ := strings.Cut(strings.TrimSpace(l), "=")
			lines[last] = indent + strings.TrimSpace(name) + " = " + escapeValue(s.Value)
			continue
		}
		appended = append(appended, s.Name+" = "+escapeValue(s.Value))
	}
	if len(appended) > 0 {
		hasMarker := false
		for _, l := range lines {
			if l == savedMarker {
				hasMarker = true
				break
			}
		}
		if !hasMarker {
			if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
				lines = append(lines, "")
			}
			lines = append(lines, savedMarker)
		}
		lines = append(lines, appended...)
	}

	out := strings.Join(lines, "\n") + "\n"
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mp3blasterrc-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(out); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
