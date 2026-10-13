package config

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/mingcheng/mp3blstr/internal/media"
	"go.yaml.in/yaml/v3"
)

// The sample configuration must parse as shipped and with all settings
// uncommented, and document the actual defaults.
func TestSampleConfig(t *testing.T) {
	raw, err := os.ReadFile("../../config.sample.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// Git may check the file out with CRLF line endings (e.g. on Windows).
	data := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if err := Default().Parse(strings.NewReader(data)); err != nil {
		t.Fatalf("as shipped: %v", err)
	}

	setting := regexp.MustCompile(`(?m)^(\s*)# (\s*[a-z][a-z0-9_]*:( .*)?)$`)
	all := setting.ReplaceAllString(data, "$1$2")
	o := Default()
	if err := o.Parse(strings.NewReader(all)); err != nil {
		t.Fatalf("uncommented: %v\n%s", err, all)
	}

	def := Default()
	for _, s := range ChangedSettings(def.StateSettings(), o.StateSettings()) {
		if !strings.HasPrefix(s.Name, "state.last_") {
			t.Errorf("sample value of %s differs from the default", s.Name)
		}
	}
	if strings.Join(o.AudioPatterns, " ") != strings.Join(media.DefaultAudioPatterns, " ") ||
		strings.Join(o.PlaylistPatterns, " ") != strings.Join(media.DefaultPlaylistPatterns, " ") {
		t.Errorf("patterns %q %q", o.AudioPatterns, o.PlaylistPatterns)
	}
	if o.Mouse != def.Mouse || o.ListWidth != def.ListWidth || o.PanSize != def.PanSize || o.SkipSeconds != def.SkipSeconds ||
		o.Downsample != def.Downsample || o.HideOthers != def.HideOthers || o.ID3Names != def.ID3Names || o.GuessLegacyTags != def.GuessLegacyTags || o.MetadataCache != def.MetadataCache || o.ScanMP3 != def.ScanMP3 ||
		o.UseTrash != def.UseTrash || o.PauseOnSleep != def.PauseOnSleep || o.PauseOnOutputChange != def.PauseOnOutputChange ||
		o.SelectUnselectFirst != def.SelectUnselectFirst || o.SelectRegex != def.SelectRegex ||
		o.SelectCaseInsensitive != def.SelectCaseInsensitive || o.SaveState != def.SaveState {
		t.Errorf("sample values differ from the defaults: %+v", o)
	}
	if len(o.Colours) != 18 || len(o.Keys) != 25 {
		t.Errorf("colours %d, keys %d", len(o.Colours), len(o.Keys))
	}
}

func TestDesktopPackaging(t *testing.T) {
	data, err := os.ReadFile("../../.goreleaser.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var release struct {
		Archives []struct{ Files []string } `yaml:"archives"`
	}
	if err := yaml.Unmarshal(data, &release); err != nil {
		t.Fatal(err)
	}
	packaged := false
	for _, archive := range release.Archives {
		for _, path := range archive.Files {
			packaged = packaged || path == "share/applications/mp3blstr.desktop"
		}
	}
	if !packaged {
		t.Fatal("desktop entry missing from release archives")
	}
	data, err = os.ReadFile("../../share/applications/mp3blstr.desktop")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []string{"Type=Application", "Terminal=true", "Icon=multimedia-player", "Exec=mp3blstr --reuse-instance --dont-quit %U", "--enqueue", "audio/mpeg;"} {
		if !strings.Contains(string(data), entry) {
			t.Fatalf("desktop entry missing %q", entry)
		}
	}
	for _, args := range [][]string{{"--reuse-instance", "--enqueue", "song.mp3"}, {"--reuse-instance"}} {
		if _, err := ParseArgs(args); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"--enqueue"}, {"--reuse-instance", "--chroot", "/"}} {
		if _, err := ParseArgs(args); err == nil {
			t.Fatalf("accepted invalid flags %v", args)
		}
	}
}

func TestMetadataCacheSetting(t *testing.T) {
	o := Default()
	if !o.MetadataCache {
		t.Fatal("metadata cache should be enabled by default")
	}
	if err := o.Parse(strings.NewReader("files:\n  metadata_cache: false\n")); err != nil {
		t.Fatal(err)
	}
	if o.MetadataCache {
		t.Fatal("metadata cache should be disabled by configuration")
	}
}
