package theme

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func bgOf(s tcell.Style) tcell.Color {
	_, bg, _ := s.Decompose()
	return bg
}

// TestClassicTransparentBackground checks that the default theme leaves the
// terminal background visible, and that Color.Default.bg restores an opaque one.
func TestClassicTransparentBackground(t *testing.T) {
	th := Classic()
	for name, s := range map[string]tcell.Style{
		"default": th.Default, "border": th.Border, "button": th.Button,
		"number": th.Number, "popupinput": th.PopupInput, "filemp3": th.FileMP3,
	} {
		if bg := bgOf(s); bg != tcell.ColorDefault {
			t.Errorf("%s background = %v, want terminal default", name, bg)
		}
	}

	th.ApplyColours(map[string]string{"default.bg": "black"})
	for name, s := range map[string]tcell.Style{
		"default": th.Default, "button": th.Button, "number": th.Number, "filedir": th.FileDir,
	} {
		if bg := bgOf(s); bg != black {
			t.Errorf("%s background = %v, want black", name, bg)
		}
	}
}
