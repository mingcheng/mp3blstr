package config

import (
	"encoding/binary"
	"os"
	"path/filepath"
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
		o.SelectCaseInsensitive != def.SelectCaseInsensitive || o.SaveState != def.SaveState ||
		o.FadeSeconds != def.FadeSeconds || o.FadeInSeconds != def.FadeInSeconds {
		t.Errorf("sample values differ from the defaults: %+v", o)
	}
	if len(o.Colours) != 18 || len(o.Keys) != 26 {
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

func TestLegacyTagSetting(t *testing.T) {
	opts := Default()
	if !opts.GuessLegacyTags {
		t.Fatal("legacy tag decoding should be enabled by default")
	}
	if err := opts.Parse(strings.NewReader("files:\n  guess_legacy_tags: false\n")); err != nil {
		t.Fatal(err)
	}
	if opts.GuessLegacyTags {
		t.Fatal("legacy tag decoding should be disabled by configuration")
	}
}

func TestDefaultLegacyID3Decoding(t *testing.T) {
	media.SetGuessLegacyTags(Default().GuessLegacyTags)
	t.Cleanup(func() { media.SetGuessLegacyTags(false) })
	raw := "\xd5\xbe\xb3\xa4\xcb\xd8\xb2\xc4(sc.chinaz.com)"
	wantText := "\u7ad9\u957f\u7d20\u6750(sc.chinaz.com)"
	want := media.Tags{
		Artist: wantText, Title: wantText, Album: wantText,
		Comment: wantText, Year: "2021",
	}
	var frames []byte
	for _, frame := range []struct{ id, text string }{
		{"TPE1", "\x00" + raw}, {"TIT2", "\x00" + raw},
		{"TALB", "\x00" + raw}, {"COMM", "\x00chi\x00" + raw},
		{"TYER", "\x002021"},
	} {
		header := make([]byte, 10)
		copy(header, frame.id)
		binary.BigEndian.PutUint32(header[4:8], uint32(len(frame.text)))
		frames = append(frames, header...)
		frames = append(frames, frame.text...)
	}
	header := []byte{'I', 'D', '3', 3, 0, 0, 0, 0, 0, 0}
	for index, size := 9, len(frames); index >= 6; index, size = index-1, size>>7 {
		header[index] = byte(size & 0x7f)
	}
	generated := filepath.Join(t.TempDir(), "legacy.mp3")
	if err := os.WriteFile(generated, append(header, frames...), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct{ name, path string }{
		{"generated", generated}, {"local-mp3", "../../15134.mp3"},
	} {
		t.Run(sample.name, func(t *testing.T) {
			if _, err := os.Stat(sample.path); os.IsNotExist(err) {
				t.Skip("local MP3 sample is not available")
			}
			name := "[ID3] " + wantText + " - " + wantText
			if got := media.ID3Name(sample.path, "fallback"); got != name {
				t.Fatalf("list name = %q, want %q", got, name)
			}
			if got, err := media.ReadTags(sample.path); err != nil || got != want {
				t.Fatalf("detail tags = %+v, %v; want %+v", got, err, want)
			}
		})
	}
}
