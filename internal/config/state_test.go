package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mingcheng/mp3blaster/internal/filemanager"
	"github.com/mingcheng/mp3blaster/internal/playback"
)

func TestSaveSettingsUpdatesInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mp3blasterrc")
	orig := `# my settings
Threads = 100
  theme = classic
#Theme = modern
LastSong = /old.mp3
LastPosition = 12
Mouse = no
`
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	err := SaveSettings(path, []Setting{
		{"Theme", "modern"},
		{"Threads", "250"},
		{"LastSong", ""},
		{"LastPosition", ""},
		{"LastDir", `/music/a,b\c`},
		{"Repeat", "yes"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	want := `# my settings
Threads = 250
  theme = modern
#Theme = modern
Mouse = no

` + savedMarker + `
LastDir = /music/a\,b\\c
Repeat = yes
`
	if string(data) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", data, want)
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v", st.Mode().Perm())
		}
	}

	// A second save reuses the marker and the appended lines.
	if err := SaveSettings(path, []Setting{{"Repeat", "no"}, {"Volume", "80"}}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if strings.Count(string(data), savedMarker) != 1 || !strings.HasSuffix(string(data), "Repeat = no\nVolume = 80\n") {
		t.Fatalf("second save:\n%s", data)
	}

	o := Default()
	if err := o.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	if o.Theme != "modern" || o.Threads != 250 || o.LastDir != `/music/a,b\c` || o.Repeat || o.Volume != 80 || o.LastSong != "" {
		t.Fatalf("reloaded %+v", o)
	}
	if o.Path != path {
		t.Fatalf("path %q", o.Path)
	}
}

func TestSaveSettingsCreatesFileAndFollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "mp3blasterrc")
	if err := SaveSettings(path, []Setting{{"Volume", "70"}}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != savedMarker+"\nVolume = 70\n" {
		t.Fatalf("new file:\n%s", data)
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
	if err := SaveSettings(link, []Setting{{"Volume", "60"}}); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Lstat(link); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symbolic link must be kept")
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "Volume = 60") {
		t.Fatalf("target not updated:\n%s", data)
	}
}

func TestStateSettingsRoundTrip(t *testing.T) {
	o := Default()
	o.Theme, o.NerdFont, o.Wrap, o.Threads = "modern", true, false, 300
	o.SortMode, o.PlayMode, o.Repeat = filemanager.SortSizeBig, playback.PlaySongs, true
	o.Volume, o.Balance = 42, 60
	o.LastDir, o.LastSong, o.LastPosition = "/music", "/music/a, b.mp3", 95

	path := filepath.Join(t.TempDir(), "rc")
	before := Default().StateSettings()
	changed := ChangedSettings(before, o.StateSettings())
	if len(changed) != 12 {
		t.Fatalf("changed %v", changed)
	}
	if err := SaveSettings(path, changed); err != nil {
		t.Fatal(err)
	}
	r := Default()
	if err := r.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	r.Path = ""
	o.Path = ""
	if got, want := r.StateSettings(), o.StateSettings(); len(ChangedSettings(want, got)) != 0 {
		t.Fatalf("round trip: %v", ChangedSettings(want, got))
	}
	if len(ChangedSettings(o.StateSettings(), o.StateSettings())) != 0 {
		t.Fatal("unchanged settings must not be written")
	}
}

func TestStateKeywordErrors(t *testing.T) {
	for _, in := range []string{"Volume = 101", "Balance = -1", "PlayMode = sometimes", "LastPosition = -5", "SaveState = maybe"} {
		if err := Default().Parse(strings.NewReader(in)); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}
