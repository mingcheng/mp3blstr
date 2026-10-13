package config

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/mingcheng/mp3blasterr/internal/media"
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
		o.Downsample != def.Downsample || o.HideOthers != def.HideOthers || o.ID3Names != def.ID3Names || o.ScanMP3 != def.ScanMP3 ||
		o.SelectUnselectFirst != def.SelectUnselectFirst || o.SelectRegex != def.SelectRegex ||
		o.SelectCaseInsensitive != def.SelectCaseInsensitive || o.SaveState != def.SaveState {
		t.Errorf("sample values differ from the defaults: %+v", o)
	}
	if len(o.Colours) != 18 || len(o.Keys) != 23 {
		t.Errorf("colours %d, keys %d", len(o.Colours), len(o.Keys))
	}
}
