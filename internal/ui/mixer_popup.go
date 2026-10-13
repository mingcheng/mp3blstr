package ui

import (
	"fmt"

	"github.com/rivo/tview"
	"github.com/rivo/uniseg"

	"github.com/mingcheng/mp3blstr/internal/keys"
)

// drawMixer draws the mini mixer in the bottom right corner:
//
//	[ t ]Mixer
//	Volume
//	[<]+100%[>]
func (s *Screen) drawMixer(c canvas, l layout) {
	m := s.app.Mixer
	if m == nil {
		return
	}
	x, y := l.cx(), l.mixerY()
	def, th := s.th.Default, s.th
	c.put(x, y, "[", def, 1)
	c.put(x+1, y, s.app.Keys.LabelFor(keys.CmdToggleMixer), th.Button, 3)
	c.put(x+4, y, "]Mixer", def, 6)
	s.onClick(rect{x, y, 10, 1}, func() { s.app.Click(keys.CmdToggleMixer) })

	dev := m.Current()
	if l.wide() {
		s.drawWideMixer(c, l, dev.Name, dev.Value)
		return
	}
	c.put(x, y+1, s.gl.MixerPrefix+dev.Name, th.MixerName, 11)
	s.onClick(rect{x, y + 1, 11, 1}, func() { s.app.Click(keys.CmdToggleMixer) })

	c.put(x, y+2, "[", th.MixerBracket, 1)
	c.put(x+1, y+2, "<", th.MixerArrow, 1)
	c.put(x+2, y+2, "]", th.MixerBracket, 1)
	c.put(x+3, y+2, "+", th.MixerName.Bold(true), 1)
	c.put(x+4, y+2, fmt.Sprintf("%03d", dev.Value), th.MixerValue, 3)
	c.put(x+7, y+2, "%", th.MixerName.Bold(true), 1)
	c.put(x+8, y+2, "[", th.MixerBracket, 1)
	c.put(x+9, y+2, ">", th.MixerArrow, 1)
	c.put(x+10, y+2, "]", th.MixerBracket, 1)
	s.onClick(rect{x, y + 2, 3, 1}, func() { s.app.MixerVolume(-2) })
	s.onClick(rect{x + 8, y + 2, 3, 1}, func() { s.app.MixerVolume(2) })

	s.onMouse(rect{x, y, 11, 3}, func(action tview.MouseAction, _, _ int) bool {
		switch action {
		case tview.MouseScrollUp:
			s.app.MixerVolume(2)
		case tview.MouseScrollDown:
			s.app.MixerVolume(-2)
		default:
			return false
		}
		return true
	})
}

// drawWideMixer draws the mixer with a clickable volume bar:
//
//	Volume                      100%
//	[<]━━━━━━━━━━━━━━━━━━━━━━━━━━[>]
func (s *Screen) drawWideMixer(c canvas, l layout, name string, value int) {
	th := s.th
	x, y, w := l.px+1, l.mixerY()+1, l.pw-2
	c.put(x, y, s.gl.MixerPrefix+name, th.MixerName, w-5)
	pct := fmt.Sprintf("%3d%%", value)
	c.put(x+w-len(pct), y, pct, th.MixerValue, len(pct))
	s.onClick(rect{x, y, w - 5, 1}, func() { s.app.Click(keys.CmdToggleMixer) })

	y++
	c.segs(x, y, 3, seg{"[", th.MixerBracket}, seg{"<", th.MixerArrow}, seg{"]", th.MixerBracket})
	c.segs(x+w-3, y, 3, seg{"[", th.MixerBracket}, seg{">", th.MixerArrow}, seg{"]", th.MixerBracket})
	s.onClick(rect{x, y, 3, 1}, func() { s.app.MixerVolume(-2) })
	s.onClick(rect{x + w - 3, y, 3, 1}, func() { s.app.MixerVolume(2) })

	bar := rect{x + 3, y, w - 6, 1}
	filled := value * bar.w / 100
	c.hline(bar.x, y, filled, "━", th.MixerValue)
	c.hline(bar.x+filled, y, bar.w-filled, "─", th.Border)
	s.onMouse(bar, func(action tview.MouseAction, mx, _ int) bool {
		if action != tview.MouseLeftClick {
			return false
		}
		s.app.MixerSet((mx - bar.x + 1) * 100 / bar.w)
		return true
	})
	s.onMouse(rect{l.px, l.mixerY(), l.pw, 3}, func(action tview.MouseAction, _, _ int) bool {
		switch action {
		case tview.MouseScrollUp:
			s.app.MixerVolume(2)
		case tview.MouseScrollDown:
			s.app.MixerVolume(-2)
		default:
			return false
		}
		return true
	})
}

// drawPopup draws the input dialog over the main window. It returns true
// when the text cursor has been placed in a popup's input field.
func (s *Screen) drawPopup(c canvas, l layout) bool {
	p := s.app.Popup
	if p == nil {
		return false
	}
	r := l.popup
	c.fill(r.x, r.y, r.w, r.h, s.th.Popup)
	for i, label := range p.Labels {
		if y := r.y + 2 + i; y < r.y+r.h-2 {
			c.put(r.x+1, y, label, s.th.Popup, r.w-2)
		}
	}
	iy, ix, iw := r.y+r.h-2, r.x+1, r.w-2
	c.fill(ix, iy, iw, 1, s.th.PopupInput)
	if p.Confirm {
		c.put(ix, iy, "[y] Yes   [n] No", s.th.PopupInput, iw)
		s.onMouse(rect{0, 0, l.w, l.h}, func(tview.MouseAction, int, int) bool { return true })
		return false
	}
	text, col, first := p.Editor.Visible(iw)
	c.put(ix, iy, text, s.th.PopupInput, iw)
	s.scr.ShowCursor(ix+col, iy)

	// Clicks in the input field move the cursor; everything else under the
	// popup is blocked.
	s.onMouse(rect{0, 0, l.w, l.h}, func(tview.MouseAction, int, int) bool { return true })
	s.onMouse(rect{ix, iy, iw, 1}, func(action tview.MouseAction, x, _ int) bool {
		if action != tview.MouseLeftClick {
			return true
		}
		cells, idx := 0, first
		for _, rn := range text {
			w := uniseg.StringWidth(string(rn))
			if cells+w > x-ix {
				break
			}
			cells += w
			idx++
		}
		s.app.PopupCursor(idx)
		return true
	})
	return true
}
