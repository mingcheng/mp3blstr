package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mingcheng/mp3blasterr/internal/filemanager"
	"github.com/mingcheng/mp3blasterr/internal/playback"
)

func TestSaveSettingsUpdatesInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	orig := `# my settings
ui:
  Theme: classic   # classic or modern
  # nerd_font: false
  mouse: false

playback:
  threads: 100
  repeat: 'no'

# Session
state:
  last_song: "/old.mp3"
  last_position: 12 # seconds

keys:
  play:
    next: n
`
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	err := SaveSettings(path, []Setting{
		{"ui.theme", "modern"},
		{"playback.threads", "250"},
		{"state.last_song", ""},
		{"state.last_position", ""},
		{"state.last_dir", "/music/a, b: c"},
		{"playback.repeat", "true"},
		{"playback.volume", "80"},
		{"files.sort_mode", "size-big"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	want := `# my settings
ui:
  Theme: modern   # classic or modern
  # nerd_font: false
  mouse: false

playback:
  threads: 250
  repeat: true
  volume: 80

# Session
state:
  last_dir: '/music/a, b: c'

keys:
  play:
    next: n

files:
  sort_mode: size-big
`
	if string(data) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", data, want)
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v", st.Mode().Perm())
		}
	}

	if err := SaveSettings(path, []Setting{{"playback.repeat", "false"}, {"playback.volume", "70"}, {"state.last_song", "100.mp3"}}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), "  repeat: false\n  volume: 70\n") || !strings.Contains(string(data), "state:\n  last_dir: '/music/a, b: c'\n  last_song: 100.mp3\n") {
		t.Fatalf("second save:\n%s", data)
	}

	o := Default()
	if err := o.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	if o.Theme != "modern" || o.Threads != 250 || o.LastDir != "/music/a, b: c" || o.Repeat || o.Volume != 70 ||
		o.LastSong != "100.mp3" || o.SortMode != filemanager.SortSizeBig || o.Mouse {
		t.Fatalf("reloaded %+v", o)
	}
	if o.Path != path {
		t.Fatalf("path %q", o.Path)
	}
}

func TestSaveSettingsCreatesFileAndFollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.yaml")
	if err := SaveSettings(path, []Setting{{"playback.volume", "70"}, {"state.last_song", "/a.mp3"}}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if want := "# " + savedComment + "\n\nplayback:\n  volume: 70\n\nstate:\n  last_song: /a.mp3\n"; string(data) != want {
		t.Fatalf("new file:\n%s\nwant:\n%s", data, want)
	}
	if err := SaveSettings(path, nil); err != nil {
		t.Fatal("nothing to save must not fail")
	}

	if runtime.GOOS == "windows" {
		return
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Skip(err)
	}
	if err := SaveSettings(link, []Setting{{"playback.volume", "60"}}); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Lstat(link); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symbolic link must be kept")
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "volume: 60") {
		t.Fatalf("target not updated:\n%s", data)
	}
}

func TestSaveSettingsLayouts(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"comments only", "# nothing yet\n", "# nothing yet\n\nui:\n  theme: modern\n"},
		{"crlf", "ui:\r\n  theme: classic\r\n", "ui:\n  theme: modern\n"},
		{"empty section", "ui:   # look\nstate:\n", "ui:   # look\n  theme: modern\nstate:\n"},
		{"indented by four", "ui:\n    mouse: no\n", "ui:\n    mouse: no\n    theme: modern\n"},
		{"after a nested section", "ui:\n  colors:\n    popup:\n      bg: red # x\n\n# keys\nkeys: {}\n",
			"ui:\n  colors:\n    popup:\n      bg: red # x\n  theme: modern\n\n# keys\nkeys: {}\n"},
	} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(tc.in), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := SaveSettings(path, []Setting{{"ui.theme", "modern"}}); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if data, _ := os.ReadFile(path); string(data) != tc.want {
			t.Errorf("%s: got\n%s\nwant\n%s", tc.name, data, tc.want)
		}
	}
}

// Layouts that cannot be edited line by line are re-encoded.
func TestSaveSettingsFallback(t *testing.T) {
	for _, in := range []string{
		"ui: {theme: classic, mouse: false}\nstate: {last_song: /old.mp3}\n",
		"# keep me\nui:\n  mouse: false\nstate:\n  last_song: >-\n    /old.mp3\n  last_position: 3\n",
		"ui:\n  theme: !!str classic\n  mouse: false\nstate:\n  last_song: \"/old\n    .mp3\"\n",
	} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(in), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := SaveSettings(path, []Setting{{"ui.theme", "modern"}, {"state.last_song", "/new.mp3"}, {"state.last_position", ""}}); err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		o := Default()
		if err := o.LoadFile(path); err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		data, _ := os.ReadFile(path)
		if o.Theme != "modern" || o.Mouse || o.LastSong != "/new.mp3" || o.LastPosition != 0 {
			t.Errorf("%q: reloaded %+v from\n%s", in, o, data)
		}
		if strings.HasPrefix(in, "# keep me") && !strings.HasPrefix(string(data), "# keep me") {
			t.Errorf("comment lost:\n%s", data)
		}
	}
}

func TestSaveSettingsKeepsBadFiles(t *testing.T) {
	for _, in := range []string{"ui: [\n", "- a\n- b\n"} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(in), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := SaveSettings(path, []Setting{{"ui.theme", "modern"}}); err == nil {
			t.Errorf("%q: expected error", in)
		}
		if data, _ := os.ReadFile(path); string(data) != in {
			t.Errorf("%q: file changed to %q", in, data)
		}
	}
}

func TestStateSettingsRoundTrip(t *testing.T) {
	o := Default()
	o.Theme, o.NerdFont, o.Wrap, o.Threads = "modern", true, false, 300
	o.SortMode, o.PlayMode, o.Repeat = filemanager.SortSizeBig, playback.PlaySongs, true
	o.FadeOut = true
	o.Volume, o.Balance = 42, 60
	o.LastDir, o.LastSong, o.LastPosition = "/music", "/music/a, b #1: 'x'.mp3", 95

	path := filepath.Join(t.TempDir(), "config.yaml")
	before := Default().StateSettings()
	changed := ChangedSettings(before, o.StateSettings())
	if len(changed) != 13 {
		t.Fatalf("changed %v", changed)
	}
	if err := SaveSettings(path, changed); err != nil {
		t.Fatal(err)
	}
	r := Default()
	if err := r.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	if got, want := r.StateSettings(), o.StateSettings(); len(ChangedSettings(want, got)) != 0 {
		t.Fatalf("round trip: %v", ChangedSettings(want, got))
	}
	if len(ChangedSettings(o.StateSettings(), o.StateSettings())) != 0 {
		t.Fatal("unchanged settings must not be written")
	}
	for _, s := range o.StateSettings() {
		if _, ok := keywords(Default())[s.Name]; !ok {
			t.Errorf("state setting %q is not a known setting", s.Name)
		}
	}
}

func TestStateKeywordErrors(t *testing.T) {
	for _, in := range []string{
		"playback: {volume: 101}", "playback: {balance: -1}", "playback: {play_mode: sometimes}",
		"playback: {fade_seconds: -1}", "playback: {fade_out: maybe}",
		"state: {last_position: -5}", "state: {save: maybe}",
	} {
		if err := Default().Parse(strings.NewReader(in)); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}

func TestFadeDurationConfigOnly(t *testing.T) {
	opts := Default()
	if err := opts.Parse(strings.NewReader("playback: {fade_seconds: 2}")); err != nil {
		t.Fatal(err)
	}
	if opts.FadeSeconds != 2 {
		t.Fatalf("fade seconds = %d", opts.FadeSeconds)
	}
	for _, setting := range opts.StateSettings() {
		if setting.Name == "playback.fade_seconds" {
			t.Fatal("fade duration should not be saved as runtime state")
		}
	}
}
