// Package theme defines the colours and glyphs used to draw the interface:
// the "classic" theme reproduces the ncurses colours of the original, the
// "modern" theme uses true colour and rounded borders. Glyph sets provide
// plain ASCII-style symbols or Nerd Font icons.
package theme

import (
	"strings"

	"github.com/gdamore/tcell/v2"
)

// Theme is a set of styles.
type Theme struct {
	Name string

	Default    tcell.Style
	Border     tcell.Style
	Title      tcell.Style
	Popup      tcell.Style
	PopupInput tcell.Style
	Error      tcell.Style
	Button     tcell.Style // key labels in the help bar and next to buttons
	Label      tcell.Style // CD-player buttons
	Number     tcell.Style // times, MPEG info, threads
	FileMP3    tcell.Style
	FileDir    tcell.Style
	FileLst    tcell.Style
	FileWin    tcell.Style // groups

	MixerName    tcell.Style
	MixerBracket tcell.Style
	MixerArrow   tcell.Style
	MixerValue   tcell.Style

	// Cursor turns a row style into the style of the selection bar.
	Cursor func(tcell.Style) tcell.Style
	// Marked turns a row style into the style of a selected (marked) row.
	Marked func(tcell.Style) tcell.Style

	Rounded bool
}

// ncurses colour numbers.
var (
	black   = tcell.PaletteColor(0)
	red     = tcell.PaletteColor(1)
	green   = tcell.PaletteColor(2)
	yellow  = tcell.PaletteColor(3)
	blue    = tcell.PaletteColor(4)
	magenta = tcell.PaletteColor(5)
	white   = tcell.PaletteColor(7)
)

var colourByName = map[string]tcell.Color{
	"black":   black,
	"red":     red,
	"green":   green,
	"yellow":  yellow,
	"blue":    blue,
	"magenta": magenta,
	"cyan":    tcell.PaletteColor(6),
	"white":   white,
	"default": tcell.ColorDefault,
}

func parseColour(s string) (tcell.Color, bool) {
	s = strings.ToLower(s)
	if c, ok := colourByName[s]; ok {
		return c, true
	}
	if strings.HasPrefix(s, "#") {
		if c := tcell.GetColor(s); c != tcell.ColorDefault {
			return c, true
		}
	}
	return tcell.ColorDefault, false
}

// Classic returns the colours of the original program, drawn on the
// terminal's own background so translucent terminals stay see-through.
func Classic() Theme {
	bg := tcell.ColorDefault
	def := tcell.StyleDefault.Foreground(white).Background(bg)
	return Theme{
		Name:         "classic",
		Default:      def,
		Border:       def,
		Title:        def,
		Popup:        tcell.StyleDefault.Foreground(white).Background(blue).Bold(true),
		PopupInput:   def,
		Error:        tcell.StyleDefault.Foreground(white).Background(red).Bold(true),
		Button:       tcell.StyleDefault.Foreground(magenta).Background(bg),
		Label:        tcell.StyleDefault.Foreground(black).Background(yellow),
		Number:       tcell.StyleDefault.Foreground(yellow).Background(bg).Bold(true),
		FileMP3:      def.Foreground(green),
		FileDir:      def.Foreground(blue),
		FileLst:      def.Foreground(yellow),
		FileWin:      def.Foreground(magenta),
		MixerName:    def,
		MixerBracket: def,
		MixerArrow:   def.Foreground(magenta),
		MixerValue:   def.Foreground(yellow).Bold(true),
		Cursor:       func(s tcell.Style) tcell.Style { return s.Reverse(true) },
		Marked:       func(s tcell.Style) tcell.Style { return s.Bold(true) },
	}
}

// Modern returns a true colour theme on the terminal's own background.
func Modern() Theme {
	c := tcell.GetColor
	bg := tcell.ColorDefault
	def := tcell.StyleDefault.Foreground(c("#c8ccd4")).Background(bg)
	return Theme{
		Name:         "modern",
		Default:      def,
		Border:       def.Foreground(c("#5c6370")),
		Title:        def.Foreground(c("#e5c07b")).Bold(true),
		Popup:        tcell.StyleDefault.Foreground(c("#ffffff")).Background(c("#2c4a7a")).Bold(true),
		PopupInput:   tcell.StyleDefault.Foreground(c("#ffffff")).Background(c("#1c1f26")),
		Error:        tcell.StyleDefault.Foreground(c("#ffffff")).Background(c("#be5046")).Bold(true),
		Button:       def.Foreground(c("#c678dd")).Bold(true),
		Label:        tcell.StyleDefault.Foreground(c("#1c1f26")).Background(c("#e5c07b")).Bold(true),
		Number:       def.Foreground(c("#e5c07b")).Bold(true),
		FileMP3:      def.Foreground(c("#98c379")),
		FileDir:      def.Foreground(c("#61afef")),
		FileLst:      def.Foreground(c("#e5c07b")),
		FileWin:      def.Foreground(c("#c678dd")),
		MixerName:    def.Foreground(c("#abb2bf")),
		MixerBracket: def.Foreground(c("#5c6370")),
		MixerArrow:   def.Foreground(c("#c678dd")).Bold(true),
		MixerValue:   def.Foreground(c("#e5c07b")).Bold(true),
		Cursor: func(s tcell.Style) tcell.Style {
			fg, _, attr := s.Decompose()
			if fg == tcell.ColorDefault {
				fg = c("#ffffff")
			}
			return tcell.StyleDefault.Foreground(fg).Background(c("#3e4451")).Attributes(attr).Bold(true)
		},
		Marked:  func(s tcell.Style) tcell.Style { return s.Bold(true).Underline(true) },
		Rounded: true,
	}
}

// Canonical returns the name of the theme ByName would pick for name.
func Canonical(name string) string {
	if strings.EqualFold(name, "modern") {
		return "modern"
	}
	return "classic"
}

// ByName returns the named theme (classic when unknown).
func ByName(name string) Theme {
	if Canonical(name) == "modern" {
		return Modern()
	}
	return Classic()
}

// ApplyColours applies the ui.colors overrides from the configuration file.
// Keys are lower case without underscores, e.g. "popupinput.bg".
func (t *Theme) ApplyColours(m map[string]string) {
	fgbg := func(s *tcell.Style, key string) {
		if v, ok := m[key+".fg"]; ok {
			if c, ok := parseColour(v); ok {
				*s = s.Foreground(c)
			}
		}
		if v, ok := m[key+".bg"]; ok {
			if c, ok := parseColour(v); ok {
				*s = s.Background(c)
			}
		}
	}
	oldDef := t.Default
	fgbg(&t.Default, "default")
	if t.Default != oldDef {
		_, bg, _ := t.Default.Decompose()
		for _, s := range []*tcell.Style{&t.Border, &t.Title, &t.FileMP3, &t.FileDir, &t.FileLst, &t.FileWin,
			&t.MixerName, &t.MixerBracket, &t.MixerArrow, &t.MixerValue, &t.PopupInput, &t.Button, &t.Number} {
			*s = s.Background(bg)
		}
		fg, _, _ := t.Default.Decompose()
		t.Border = t.Border.Foreground(fg)
		t.MixerName = t.MixerName.Foreground(fg)
		t.MixerBracket = t.MixerBracket.Foreground(fg)
	}
	fgbg(&t.Popup, "popup")
	fgbg(&t.PopupInput, "popupinput")
	fgbg(&t.Error, "error")
	fgbg(&t.Button, "button")
	fgbg(&t.Label, "label")
	fgbg(&t.Number, "number")
	fgbg(&t.FileMP3, "filemp3")
	fgbg(&t.FileDir, "filedir")
	fgbg(&t.FileLst, "filelst")
	fgbg(&t.FileWin, "filewin")
}
