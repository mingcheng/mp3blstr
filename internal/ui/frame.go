package ui

import (
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"

	"github.com/mingcheng/mp3blstr/internal/keys"
)

// drawFrame draws the static lines that divide the screen.
func (s *Screen) drawFrame(c canvas, l layout) {
	g, st := s.gl, s.th.Border
	w, h, x := l.w, l.h, l.px-1

	c.put(0, 0, g.TL, st, 1)
	c.hline(1, 0, w-2, g.H, st)
	c.put(w-1, 0, g.TR, st, 1)
	c.put(0, h-1, g.BL, st, 1)
	c.hline(1, h-1, w-2, g.H, st)
	c.put(w-1, h-1, g.BR, st, 1)
	c.vline(0, 1, h-2, g.V, st)
	c.vline(w-1, 1, h-2, g.V, st)

	for _, y := range []int{5, 10, h - 4} {
		c.put(0, y, g.LTee, st, 1)
		c.hline(1, y, w-2, g.H, st)
		c.put(w-1, y, g.RTee, st, 1)
	}
	c.vline(x, 6, h-7, g.V, st)
	c.put(x, 5, g.TTee, st, 1)
	c.put(x, 10, g.Cross, st, 1)
	c.put(x, h-4, g.Cross, st, 1)
	c.put(x, h-1, g.BTee, st, 1)

	if s.app.Version != "" {
		label := " " + crunch(s.app.Version, w-10) + " "
		c.put(w-6-uniseg.StringWidth(label), 0, label, s.th.Default, w-7)
	}

	// "( - )" and "( + )" scroll the help bar.
	for _, b := range []struct {
		y     int
		cmd   keys.Command
		glyph string
	}{{0, keys.CmdHelpPrev, g.HelpPrev}, {5, keys.CmdHelpNext, g.HelpNext}} {
		c.put(w-6, b.y, b.glyph, s.th.Default, 1)
		c.put(w-5, b.y, s.app.Keys.LabelFor(b.cmd), s.th.Button, 3)
		c.put(w-2, b.y, g.HelpEnd, s.th.Default, 1)
		s.clickCommand(rect{w - 6, b.y, 5, 1}, b.cmd)
	}
}

const (
	helpEntryWidth = 26
	helpKeyWidth   = 7
)

// drawHelpBar draws the key overview at the top of the screen.
func (s *Screen) drawHelpBar(c canvas, l layout) {
	lines := s.app.HelpBarLines()
	hb := &s.app.HelpBar
	hb.SetRows(l.help.h, len(lines))
	maxX := l.help.x + l.help.w
	for row := 0; row < l.help.h; row++ {
		i := hb.Offset + row
		if i >= len(lines) {
			break
		}
		y := l.help.y + row
		for j, e := range lines[i] {
			x := l.help.x + j*helpEntryWidth
			if x >= maxX {
				break
			}
			width := min(helpEntryWidth-1, maxX-x)
			keyWidth := 5
			if s.nerd {
				keyWidth = helpKeyWidth
				icon := s.gl.Shortcuts[e.Cmd]
				if icon == "" {
					icon = " "
				}
				s.drawHelpKey(c, x, y, icon+" "+e.Label, min(keyWidth, width))
			} else {
				c.put(x, y, "[", s.th.Default, min(1, width))
				c.put(x+1, y, s.app.Keys.KeyFor(e.Cmd).Label(), s.th.Button, min(3, max(0, width-1)))
				c.put(x+4, y, "]", s.th.Default, min(1, max(0, width-4)))
			}
			at := x + keyWidth + 1
			end := x + width
			if at < end {
				c.put(at, y, crunch(e.Desc, end-at), s.th.Default, end-at)
			}
			cmd := e.Cmd
			s.onClick(rect{x, y, width, 1}, func() { s.app.Click(cmd) })
		}
	}
	s.onMouse(l.help, func(action tview.MouseAction, _, _ int) bool {
		switch action {
		case tview.MouseScrollUp:
			s.app.Click(keys.CmdHelpPrev)
		case tview.MouseScrollDown:
			s.app.Click(keys.CmdHelpNext)
		default:
			return false
		}
		return true
	})
}

func (s *Screen) drawHelpKey(c canvas, x, y int, label string, width int) {
	style := s.th.Label.Bold(false)
	c.fill(x, y, width, 1, style)
	label = crunch(label, helpKeyWidth-2)
	pad := (helpKeyWidth - uniseg.StringWidth(label)) / 2
	c.put(x+pad, y, label, style, max(0, width-pad))
}

// drawInfo draws the play mode (or sort mode) and the next song.
func (s *Screen) drawInfo(c canvas, l layout) {
	c.put(l.modeLine.x, l.modeLine.y, s.app.ModeLine(), s.th.Default, l.modeLine.w)
	s.onClick(l.modeLine, s.app.ClickModeLine)
	c.put(2, 7, "Next Song      :", s.th.Default, -1)
	c.put(l.nextSong.x, l.nextSong.y, s.app.Player.NextSongName(), s.th.Default, l.nextSong.w)
}
