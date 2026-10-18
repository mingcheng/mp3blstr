package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/rivo/tview"
	"github.com/rivo/uniseg"

	"github.com/mingcheng/mp3blstr/internal/media"
)

const cancelButton = "[ Cancel (Esc) ]"

var spinner = []string{"|", "/", "-", `\`}

// drawTask draws the progress dialog of a running background task over the
// main window. Everything under it is blocked for the mouse; the cancel
// button stops the task.
func (s *Screen) drawTask(c canvas, l layout) {
	t := s.app.Task
	if t == nil {
		return
	}
	w := min(l.list.w, 60)
	h := min(l.list.h, 7)
	x := l.list.x + (l.list.w-w)/2
	y := l.list.y + (l.list.h-h)/2
	c.fill(x, y, w, h, s.th.Popup)
	s.onMouse(rect{0, 0, l.w, l.h}, func(tview.MouseAction, int, int) bool { return true }, pointerDefault)

	inner := w - 2
	row := y + (h-5)/2
	elapsed := time.Since(t.Started)
	spin := spinner[int(elapsed/(250*time.Millisecond))%len(spinner)]
	c.put(x+1, row, spin+" "+t.Title+" ...", s.th.Popup, inner)
	c.put(x+1, row+1, truncLeft(t.Detail, inner), s.th.Popup, inner)

	pct := fmt.Sprintf(" %3d%%", int(t.Fraction*100))
	barW := max(inner-len(pct), 1)
	filled := min(barW, int(t.Fraction*float64(barW)))
	c.fill(x+1, row+2, barW, 1, s.th.PopupInput)
	c.put(x+1, row+2, strings.Repeat("█", filled), s.th.PopupInput, barW)
	c.put(x+1+barW, row+2, pct, s.th.Popup, len(pct))

	status := t.Status
	if status != "" {
		status += "  "
	}
	c.put(x+1, row+3, status+media.FormatDuration(int(elapsed.Seconds())), s.th.Popup, inner)

	bx := x + (w-len(cancelButton))/2
	c.put(bx, row+4, cancelButton, s.th.PopupInput, inner)
	s.onClick(rect{bx, row + 4, len(cancelButton), 1}, s.app.CancelTask)
}

// truncLeft shortens s to width cells by cutting its beginning, so the end
// of a long path stays visible.
func truncLeft(s string, width int) string {
	if uniseg.StringWidth(s) <= width {
		return s
	}
	r := []rune(s)
	used := 1 // the ellipsis
	i := len(r)
	for i > 0 {
		cw := uniseg.StringWidth(string(r[i-1]))
		if used+cw > width {
			break
		}
		used += cw
		i--
	}
	return "…" + string(r[i:])
}
