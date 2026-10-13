package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"

	"github.com/mingcheng/mp3blasterr/internal/app"
)

// drawMain draws the main window: the title in its top border and the
// visible lines with the selection bar.
func (s *Screen) drawMain(c canvas, l layout) {
	s.drawMainTitle(c, l)

	a := s.app
	v := a.MainView()
	n := a.MainLen()
	v.SetRows(l.list.h, n)
	a.SetMainWidth(l.list.w)
	from, to := v.Visible(n)
	for i := from; i < to; i++ {
		row := a.MainRow(i)
		style := s.rowStyle(row.Kind)
		if row.Marked {
			style = s.th.Marked(style)
		}
		if i == v.Cursor {
			style = s.th.Cursor(style)
		}
		y := l.list.y + i - from
		x := l.list.x
		width := l.list.w
		c.fill(x, y, width, 1, style)
		if icon := s.rowIcon(row); icon != "" {
			used := c.put(x, y, icon, style, width)
			x += used
			width -= used
		}
		if right := uniseg.StringWidth(row.Right); right > 0 && width > right+1 {
			c.put(x+width-right, y, row.Right, style, right)
			width -= right + 1
		}
		c.put(x, y, skipCells(row.Text, v.Pan), style, width)
	}

	s.onMouse(l.list, func(action tview.MouseAction, _, y int) bool {
		idx := v.Offset + y - l.list.y
		inRange := idx >= 0 && idx < a.MainLen()
		switch action {
		case tview.MouseLeftClick:
			if inRange {
				a.ClickRow(idx)
			}
		case tview.MouseLeftDoubleClick:
			if inRange {
				a.ActivateRow(idx)
			}
		case tview.MouseRightClick:
			if inRange {
				a.ToggleRow(idx)
			}
		case tview.MouseScrollUp:
			a.Scroll(-1)
		case tview.MouseScrollDown:
			a.Scroll(1)
		default:
			return false
		}
		return true
	})
}

// drawMainTitle centres the window title in the top border, framed by tees
// like scrollWin::drawBorder does.
func (s *Screen) drawMainTitle(c canvas, l layout) {
	title := s.app.MainTitle()
	if title == "" {
		return
	}
	width := l.main.w
	nt := crunch(title, width-4)
	pad := width - 4 - uniseg.StringWidth(nt)
	if pad < 0 {
		pad = 0
	}
	lpad := pad / 2
	x := l.main.x + 1 + lpad
	c.put(x, l.main.y, s.gl.RTee, s.th.Border, 1)
	x += 1 + c.put(x+1, l.main.y, nt, s.th.Title, width-4)
	c.put(x, l.main.y, s.gl.LTee, s.th.Border, 1)
}

func (s *Screen) rowStyle(k app.RowKind) tcell.Style {
	switch k {
	case app.RowSong:
		return s.th.FileMP3
	case app.RowGroup, app.RowParent:
		return s.th.FileWin
	case app.RowDir:
		return s.th.FileDir
	case app.RowPlaylist:
		return s.th.FileLst
	default:
		return s.th.Default
	}
}

func (s *Screen) rowIcon(r app.Row) string {
	if !s.nerd {
		return ""
	}
	switch r.Kind {
	case app.RowSong:
		if r.Playing {
			return s.gl.Playing
		}
		return s.gl.Song
	case app.RowGroup:
		return s.gl.Group
	case app.RowParent:
		return s.gl.Parent
	case app.RowDir:
		if r.Text == "../" {
			return s.gl.Parent
		}
		return s.gl.Dir
	case app.RowPlaylist:
		return s.gl.Playlist
	case app.RowOther:
		return s.gl.Other
	}
	return ""
}
