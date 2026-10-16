package theme

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

func bgOf(s tcell.Style) tcell.Color {
	_, bg, _ := s.Decompose()
	return bg
}

func TestControlGlyphWidths(t *testing.T) {
	for _, nerd := range []bool{false, true} {
		g := For(Classic(), nerd)
		for _, glyph := range []string{g.Checked, g.Unchecked} {
			if got := uniseg.StringWidth(glyph); got != 3 {
				t.Errorf("checkbox %q width = %d, want 3", glyph, got)
			}
		}
		for _, glyph := range []string{g.MixerDown, g.MixerUp, g.HelpPrev, g.HelpNext, g.HelpEnd} {
			if got := uniseg.StringWidth(glyph); got != 1 {
				t.Errorf("control %q width = %d, want 1", glyph, got)
			}
		}
		if g.Checked == g.Unchecked {
			t.Error("checkbox states must be visually distinct")
		}
	}
}

// TestClassicTransparentBackground checks that the default theme leaves the
// terminal background visible, and that ui.colors.default.bg restores an opaque one.
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
