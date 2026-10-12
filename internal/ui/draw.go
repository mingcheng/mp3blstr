package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

// canvas wraps a tcell screen with clipping helpers.
type canvas struct {
	scr  tcell.Screen
	w, h int
}

// put draws text at (x, y), clipped to maxWidth cells (no limit when
// maxWidth < 0). It returns the number of cells used.
func (c canvas) put(x, y int, text string, style tcell.Style, maxWidth int) int {
	if y < 0 || y >= c.h {
		return 0
	}
	used := 0
	g := uniseg.NewGraphemes(text)
	for g.Next() {
		runes := g.Runes()
		w := g.Width()
		if w == 0 {
			continue
		}
		if maxWidth >= 0 && used+w > maxWidth {
			break
		}
		if x+used >= c.w {
			break
		}
		if x+used >= 0 {
			c.scr.SetContent(x+used, y, runes[0], runes[1:], style)
			for i := 1; i < w; i++ {
				c.scr.SetContent(x+used+i, y, ' ', nil, style)
			}
		}
		used += w
	}
	return used
}

// putPadded draws text and pads it with spaces to exactly width cells.
func (c canvas) putPadded(x, y int, text string, style tcell.Style, width int) {
	n := c.put(x, y, text, style, width)
	c.fill(x+n, y, width-n, 1, style)
}

// fill paints a rectangle with spaces.
func (c canvas) fill(x, y, w, h int, style tcell.Style) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			if i >= 0 && j >= 0 && i < c.w && j < c.h {
				c.scr.SetContent(i, j, ' ', nil, style)
			}
		}
	}
}

func (c canvas) hline(x, y, n int, glyph string, style tcell.Style) {
	for i := 0; i < n; i++ {
		c.put(x+i, y, glyph, style, 1)
	}
}

func (c canvas) vline(x, y, n int, glyph string, style tcell.Style) {
	for j := 0; j < n; j++ {
		c.put(x, y+j, glyph, style, 1)
	}
}

// skipCells drops the first n cells of text (horizontal panning).
func skipCells(text string, n int) string {
	if n <= 0 {
		return text
	}
	g := uniseg.NewGraphemes(text)
	used := 0
	for g.Next() {
		if used >= n {
			from, _ := g.Positions()
			return text[from:]
		}
		used += g.Width()
	}
	return ""
}

// crunch shortens s to at most width cells, replacing the middle by "..".
func crunch(s string, width int) string {
	if uniseg.StringWidth(s) <= width {
		return s
	}
	if width < 4 {
		r := []rune(s)
		if len(r) > width && width >= 0 {
			return string(r[:width])
		}
		return s
	}
	r := []rune(s)
	keep := width - 2
	head := keep / 2
	tail := keep - head
	for uniseg.StringWidth(string(r[:head]))+uniseg.StringWidth(string(r[len(r)-tail:]))+2 > width && tail > 0 {
		tail--
	}
	return string(r[:head]) + ".." + string(r[len(r)-tail:])
}

type rect struct{ x, y, w, h int }

func (r rect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}
