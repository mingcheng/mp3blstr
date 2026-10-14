package ui

import (
	"net/url"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/mingcheng/mp3blstr/internal/app"
	"github.com/mingcheng/mp3blstr/internal/keys"
	"github.com/mingcheng/mp3blstr/internal/listview"
	"github.com/mingcheng/mp3blstr/internal/media"
	"github.com/mingcheng/mp3blstr/internal/playback"
	"github.com/mingcheng/mp3blstr/internal/ui/theme"
)

// mouseFunc handles a mouse action at (x, y); it returns true when the
// event was consumed.
type mouseFunc func(action tview.MouseAction, x, y int) bool

type hitArea struct {
	r  rect
	fn mouseFunc
}

// Screen is the root primitive. It draws the complete mp3blstr screen
// using the original's character-cell layout and dispatches input to the
// application.
type Screen struct {
	*tview.Box
	app  *app.App
	th   theme.Theme
	gl   theme.Glyphs
	nerd bool
	// width is the width of the main window.
	width listview.Width
	scr   tcell.Screen
	hits  []hitArea
	title string
}

func newScreen(th theme.Theme, gl theme.Glyphs, nerd bool, width listview.Width) *Screen {
	return &Screen{Box: tview.NewBox(), th: th, gl: gl, nerd: nerd, width: width}
}

func (s *Screen) onMouse(r rect, fn mouseFunc) {
	if r.w > 0 && r.h > 0 {
		s.hits = append(s.hits, hitArea{r, fn})
	}
}

// onClick registers a left-click action.
func (s *Screen) onClick(r rect, fn func()) {
	s.onMouse(r, func(action tview.MouseAction, _, _ int) bool {
		if action == tview.MouseLeftClick || action == tview.MouseLeftDoubleClick {
			fn()
			return true
		}
		return false
	})
}

// clickCommand registers a click that runs cmd when it is valid in the
// current program mode.
func (s *Screen) clickCommand(r rect, cmd keys.Command) {
	s.onClick(r, func() {
		for _, b := range s.app.Keys.ForMode(s.app.Mode) {
			if b.Cmd == cmd {
				s.app.Click(cmd)
				return
			}
		}
	})
}

// syncLook rebuilds the theme and glyphs when the theme or Nerd Font option
// was switched at run time.
func (s *Screen) syncLook() {
	if s.app == nil || s.app.Opts == nil {
		return
	}
	o := s.app.Opts
	if theme.Canonical(o.Theme) == s.th.Name && o.NerdFont == s.nerd {
		return
	}
	th := theme.ByName(o.Theme)
	th.ApplyColours(o.Colours)
	s.th, s.nerd, s.gl = th, o.NerdFont, theme.For(th, o.NerdFont)
}

// Draw renders the whole screen.
func (s *Screen) Draw(scr tcell.Screen) {
	s.syncLook()
	if s.app != nil && s.app.Player != nil {
		path, info, _ := s.app.Player.SongInfo()
		title := terminalTitle(s.app.Player.Status(), path, info)
		if s.scr != scr || s.title != title {
			scr.SetTitle(title)
			s.title = title
		}
	}
	s.scr = scr
	w, h := scr.Size()
	s.SetRect(0, 0, w, h)
	s.hits = s.hits[:0]
	c := canvas{scr: scr, w: w, h: h}
	c.fill(0, 0, w, h, s.th.Default)
	l := newLayout(w, h, s.width)
	if l.tooSmall() {
		msg := "Terminal too small for mp3blstr"
		c.put((w-len(msg))/2, h/2, msg, s.th.Default, w)
		scr.HideCursor()
		return
	}
	s.drawFrame(c, l)
	s.drawHelpBar(c, l)
	s.drawInfo(c, l)
	s.drawPanel(c, l)
	s.drawButtons(c, l)
	s.drawMain(c, l)
	s.drawStatus(c, l)
	s.drawMixer(c, l)
	s.drawTask(c, l)
	if !s.drawPopup(c, l) {
		scr.HideCursor()
	}
}

func terminalTitle(status playback.Status, path string, info media.SongInfo) string {
	state := ""
	switch status {
	case playback.StatusPlay:
		state = "Playing"
	case playback.StatusPause:
		state = "Paused"
	case playback.StatusStop:
		return "mp3blstr - Stopped"
	default:
		return "mp3blstr"
	}
	name := terminalText(info.Title)
	if name == "" {
		if source, err := url.Parse(path); err == nil && (source.Scheme == "http" || source.Scheme == "https") {
			name = terminalText(filepath.Base(source.Path))
			if source.Path == "" || strings.HasSuffix(source.Path, "/") {
				name = terminalText(source.Hostname())
			}
		} else if path != "" {
			name = terminalText(filepath.Base(path))
		}
	}
	if artist := terminalText(info.Artist); artist != "" && name != "" {
		name = artist + " - " + name
	}
	title := "mp3blstr - " + state
	if name != "" {
		title += " - " + name
	}
	runes := []rune(title)
	if len(runes) > 256 {
		title = string(runes[:253]) + "..."
	}
	return title
}

func terminalText(text string) string {
	text = strings.Map(func(char rune) rune {
		if unicode.IsSpace(char) {
			return ' '
		}
		if unicode.IsControl(char) || unicode.Is(unicode.Cf, char) {
			return -1
		}
		return char
	}, text)
	return strings.Join(strings.Fields(text), " ")
}

// InputHandler forwards key presses to the application.
func (s *Screen) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return s.WrapInputHandler(func(ev *tcell.EventKey, _ func(p tview.Primitive)) {
		s.app.HandleKey(keys.FromEvent(ev))
	})
}

func (s *Screen) PasteHandler() func(string, func(tview.Primitive)) {
	return s.WrapPasteHandler(func(text string, _ func(tview.Primitive)) {
		s.app.HandlePaste(text)
	})
}

// MouseHandler dispatches mouse events to the areas registered while
// drawing (topmost first).
func (s *Screen) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	return s.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, _ func(p tview.Primitive)) (bool, tview.Primitive) {
		x, y := event.Position()
		for i := len(s.hits) - 1; i >= 0; i-- {
			if s.hits[i].r.contains(x, y) && s.hits[i].fn(action, x, y) {
				return true, nil
			}
		}
		return false, nil
	})
}
