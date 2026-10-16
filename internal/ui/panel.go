package ui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"

	"github.com/mingcheng/mp3blstr/internal/keys"
	"github.com/mingcheng/mp3blstr/internal/media"
)

// seg is a piece of text with its own style.
type seg struct {
	text  string
	style tcell.Style
}

// segs draws consecutive segments, clipped to width cells.
func (c canvas) segs(x, y, width int, parts ...seg) int {
	used := 0
	for _, p := range parts {
		if used >= width {
			break
		}
		used += c.put(x+used, y, p.text, p.style, width-used)
	}
	return used
}

func (s *Screen) checkbox(v bool) string {
	if v {
		return s.gl.Checked
	}
	return s.gl.Unchecked
}

// drawPanel draws the right hand panel: song information, the threads
// setting, the shuffle/repeat check boxes and the play times.
func (s *Screen) drawPanel(c canvas, l layout) {
	if l.wide() {
		s.drawWidePanel(c, l)
		return
	}
	r := l.cx()
	def := s.th.Default
	num := s.th.Number.Bold(false)
	_, info, ok := s.app.Player.SongInfo()
	if ok {
		c.put(r, 6, "Mpg  Layer", def, -1)
		c.put(r+3, 6, fmt.Sprintf("%1d", info.Version+1), num, 1)
		c.put(r+10, 6, fmt.Sprintf("%1d", info.Layer), num, 1)
		c.put(r, 7, "  Khz    kb", def, -1)
		sr := info.SampleRate
		if sr > 1000 {
			sr /= 1000
		}
		c.put(r, 7, fmt.Sprintf("%2d", sr), num, 2)
		c.put(r+6, 7, fmt.Sprintf("%-3d", info.Bitrate), num, 3)
		c.put(r, 8, info.Mode, num, 11)
	}
	c.put(r, 9, "Threads:", def, -1)
	c.put(r+8, 9, fmt.Sprintf("%03d", s.app.Opts.Threads), def, 3)
	s.clickCommand(rect{r, 9, 11, 1}, keys.CmdChangeThread)

	c.put(r, 11, s.checkbox(s.app.Shuffle())+" shuffle", def, 11)
	c.put(r, 12, s.checkbox(s.app.Repeat())+" repeat", def, 11)
	s.clickCommand(rect{r, 11, 11, 1}, keys.CmdToggleShuffle)
	s.clickCommand(rect{r, 12, 11, 1}, keys.CmdToggleRepeat)

	if ok {
		s.drawTime(c, r, 13, s.app.Player.Elapsed())
		s.drawTime(c, r, 14, info.TotalSeconds)
	}
}

// drawTime draws a play time the way set_song_time does: "  h:mm:ss" or
// "    m:ss" right aligned in the panel.
func (s *Screen) drawTime(c canvas, r, y, secs int) {
	num, def := s.th.Number, s.th.Default
	hours, minutes, seconds := secs/3600, (secs%3600)/60, secs%60
	if secs >= 3600 {
		c.put(r+3, y, fmt.Sprintf("%2d", hours), num, 2)
		c.put(r+5, y, ":", def, 1)
		c.put(r+6, y, fmt.Sprintf("%02d", minutes), num, 2)
	} else {
		c.put(r+6, y, fmt.Sprintf("%2d", minutes), num, 2)
	}
	c.put(r+8, y, ":", def, 1)
	c.put(r+9, y, fmt.Sprintf("%02d", seconds), num, 2)
}

func fmtTime(secs int) string {
	if secs < 0 {
		secs = 0
	}
	if secs >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", secs/3600, (secs%3600)/60, secs%60)
	}
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}

func fmtKHz(hz int) string {
	return strconv.FormatFloat(float64(hz)/1000, 'f', -1, 64)
}

// drawWidePanel is the extended panel used when the main window is limited
// in width and the panel has room for more information.
func (s *Screen) drawWidePanel(c canvas, l layout) {
	x, w := l.px+1, l.pw-2
	def, num, label := s.th.Default, s.th.Number, s.th.Button
	p := s.app.Player
	path, info, ok := p.SongInfo()

	if ok {
		if info.Layer > 0 {
			c.segs(x, 6, w, seg{"MPEG ", def}, seg{strconv.Itoa(info.Version + 1), num},
				seg{"  Layer ", def}, seg{strconv.Itoa(info.Layer), num})
		} else if ext := strings.TrimPrefix(filepath.Ext(path), "."); ext != "" {
			c.put(x, 6, strings.ToUpper(ext), num, w)
		}
		parts := []seg{{fmtKHz(info.SampleRate), num}, {" kHz", def}}
		if info.Bitrate > 0 {
			parts = append(parts, seg{"  ", def}, seg{strconv.Itoa(info.Bitrate), num}, seg{" kbit/s", def})
		}
		c.segs(x, 7, w, parts...)
		c.put(x, 8, info.Mode, num, w)
	}
	c.segs(x, 9, w, seg{"Threads: ", def}, seg{fmt.Sprintf("%03d", s.app.Opts.Threads), num})
	s.clickCommand(rect{x, 9, w, 1}, keys.CmdChangeThread)

	shuffle := s.checkbox(s.app.Shuffle()) + " shuffle"
	repeat := s.checkbox(s.app.Repeat()) + " repeat"
	c.put(x, 11, shuffle, def, w)
	rx := x + uniseg.StringWidth(shuffle) + 3
	c.put(rx, 11, repeat, def, x+w-rx)
	s.clickCommand(rect{x, 11, uniseg.StringWidth(shuffle), 1}, keys.CmdToggleShuffle)
	s.clickCommand(rect{rx, 11, uniseg.StringWidth(repeat), 1}, keys.CmdToggleRepeat)

	if !ok {
		return
	}
	elapsed, total := p.Elapsed(), info.TotalSeconds
	c.segs(x, 12, w, seg{fmtTime(elapsed), num}, seg{" / ", def}, seg{fmtTime(total), num})
	if total > 0 {
		rem := "-" + fmtTime(total-elapsed)
		c.put(x+w-len(rem), 12, rem, def, len(rem))
	}
	s.drawProgress(c, rect{x, 13, w, 1}, elapsed, total)

	rows := []struct{ k, v string }{
		{"Title", info.Title}, {"Artist", info.Artist}, {"Album", info.Album},
		{"Year", info.Year}, {"Comment", info.Comment}, {"File", media.ChopPath(path)},
	}
	y := 15
	for _, r := range rows {
		if y > l.buttonsTop()-2 {
			break
		}
		if r.v == "" {
			continue
		}
		c.segs(x, y, w, seg{fmt.Sprintf("%-8s", r.k), label}, seg{r.v, def})
		y++
	}
}

// drawProgress draws a clickable progress bar; clicking seeks.
func (s *Screen) drawProgress(c canvas, r rect, elapsed, total int) {
	filled := 0
	if total > 0 {
		filled = min(r.w, elapsed*r.w/total)
	}
	c.hline(r.x, r.y, filled, "━", s.th.Number)
	c.hline(r.x+filled, r.y, r.w-filled, "─", s.th.Border)
	if total <= 0 {
		return
	}
	s.onMouse(r, func(action tview.MouseAction, mx, _ int) bool {
		if action != tview.MouseLeftClick {
			return false
		}
		s.app.SeekTo((mx - r.x) * total / r.w)
		return true
	})
}

// drawButtons draws the CD-player style buttons with their key labels.
func (s *Screen) drawButtons(c canvas, l layout) {
	play := s.gl.Pause
	if s.app.Player.ShowPlaySymbol() {
		play = s.gl.Play
	}
	// The original: 3 column buttons 4 columns apart. In the wide panel
	// they are 5 columns wide and centred.
	bw, step, x0 := 3, 4, l.cx()
	if l.wide() {
		bw, step = 5, 7
		x0 = l.px + (l.pw-(2*step+bw))/2
	}
	buttons := []struct {
		col, y int
		symbol string
		cmd    keys.Command
	}{
		{0, l.buttonsTop(), s.gl.Prev, keys.CmdPlayPrevious},
		{1, l.buttonsTop(), play, keys.CmdPlayPlay},
		{2, l.buttonsTop(), s.gl.Next, keys.CmdPlayNext},
		{0, l.buttonsBottom(), s.gl.Rewind, keys.CmdPlayRewind},
		{1, l.buttonsBottom(), s.gl.Stop, keys.CmdPlayStop},
		{2, l.buttonsBottom(), s.gl.Forward, keys.CmdPlayForward},
	}
	pad := (bw - 3) / 2
	for _, b := range buttons {
		x := x0 + b.col*step
		c.fill(x, b.y, bw, 2, s.th.Label)
		c.put(x+pad, b.y, b.symbol, s.th.Label, 3)
		c.put(x+pad, b.y+1, s.app.Keys.LabelFor(b.cmd), s.th.Label, 3)
		cmd := b.cmd
		s.onClick(rect{x, b.y, bw, 2}, func() { s.app.Click(cmd) })
	}
}

// drawStatus draws the song status, the song title and the message line.
func (s *Screen) drawStatus(c canvas, l layout) {
	def := s.th.Default
	p := s.app.Player
	if sym, ok := s.gl.Status[p.Status()]; ok {
		c.put(l.status.x, l.status.y, sym, def, 2)
	}
	s.onClick(rect{l.status.x, l.status.y, 2, 1}, func() { s.app.Click(keys.CmdPlayPlay) })
	if path, info, ok := p.SongInfo(); ok {
		c.put(l.title.x, l.title.y, songTitle(path, info), def, l.title.w)
	}
	style := def
	if s.app.Msg.Error {
		style = s.th.Error
	}
	c.put(l.message.x, l.message.y, s.app.Msg.Text, style, l.message.w)
}

func songTitle(path string, info media.SongInfo) string {
	if info.Artist == "" {
		if info.Title != "" && media.IsHTTPStream(path) {
			return info.Title
		}
		return media.ChopPath(path)
	}
	title := info.Title
	if title == "" {
		title = "<Unknown Songname>"
	}
	return info.Artist + " - " + title
}
