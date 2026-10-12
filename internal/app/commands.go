package app

import (
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/mingcheng/mp3blaster/internal/keys"
	"github.com/mingcheng/mp3blaster/internal/listview"
	"github.com/mingcheng/mp3blaster/internal/playback"
	"github.com/mingcheng/mp3blaster/internal/playlist"
	"github.com/mingcheng/mp3blaster/internal/ui/theme"
)

// HandleKey processes a key press.
func (a *App) HandleKey(k keys.Key) {
	switch a.Input {
	case InputSearch:
		a.searchKey(k)
		return
	case InputText:
		a.popupKey(k)
		return
	case InputConfirm:
		a.confirmKey(k)
		return
	case InputTask:
		a.taskKey(k)
		return
	}
	cmd := a.Keys.Lookup(k, a.Mode)
	if cmd == keys.CmdNone {
		a.mixerKey(k)
		return
	}
	a.Exec(cmd)
}

// Exec runs a command.
func (a *App) Exec(cmd keys.Command) {
	p := a.Player
	switch cmd {
	case keys.CmdPlayPlay:
		p.Play()
	case keys.CmdPlayNext:
		p.Next()
	case keys.CmdPlayPrevious:
		p.Previous()
	case keys.CmdPlayForward:
		p.Forward()
	case keys.CmdPlayRewind:
		p.Rewind()
	case keys.CmdPlayStop:
		p.Stop()
	case keys.CmdPlaySkipEnd:
		p.SkipToEnd()
	case keys.CmdPlayNextGroup:
		p.NextGroup()
	case keys.CmdPlayPrevGroup:
		p.PreviousGroup()

	case keys.CmdMoveAfter, keys.CmdMoveBefore:
		if a.Mode == keys.ModeNormal {
			v := a.MainView()
			a.Cur.MoveSelected(v.Cursor, cmd == keys.CmdMoveBefore)
		}
	case keys.CmdQuitProgram:
		a.Quit()
	case keys.CmdHelp:
		if a.Mode == keys.ModeHelp {
			a.setMode(keys.ModeNormal)
			a.SetMessage("Visit http://mp3blaster.sourceforge.net/")
		} else {
			a.setMode(keys.ModeHelp)
			a.SetMessage("Press '?' to exit help screen.")
		}
	case keys.CmdDown:
		a.MainView().Move(1, a.MainLen(), a.Opts.Wrap)
	case keys.CmdUp:
		a.MainView().Move(-1, a.MainLen(), a.Opts.Wrap)
	case keys.CmdNextPage:
		a.MainView().PageDown(a.MainLen())
	case keys.CmdPrevPage:
		a.MainView().PageUp(a.MainLen())
	case keys.CmdJumpTop:
		a.MainView().Top(a.MainLen())
	case keys.CmdJumpBottom:
		a.MainView().Bottom(a.MainLen())
	case keys.CmdLeft:
		a.pan(-a.Opts.PanSize)
	case keys.CmdRight:
		a.pan(a.Opts.PanSize)
	case keys.CmdToggleWrap:
		a.Opts.Wrap = !a.Opts.Wrap
	case keys.CmdToggleTheme:
		a.Opts.Theme = theme.Next(a.Opts.Theme)
		a.SetMessage("Theme: " + a.Opts.Theme)
	case keys.CmdToggleNerdFont:
		a.Opts.NerdFont = !a.Opts.NerdFont
		if a.Opts.NerdFont {
			a.SetMessage("Nerd Font icons enabled")
		} else {
			a.SetMessage("Nerd Font icons disabled")
		}
	case keys.CmdDel, keys.CmdDelMark:
		a.deleteItem(cmd == keys.CmdDelMark)
	case keys.CmdSelect:
		v := a.MainView()
		if a.Mode == keys.ModeNormal && !a.Cur.IsGroup(v.Cursor) {
			a.Cur.ToggleSelect(v.Cursor)
			v.Move(1, a.MainLen(), a.Opts.Wrap)
		}
	case keys.CmdSelectItems:
		if a.Mode == keys.ModeHelp {
			break
		}
		a.openPopup([]string{"Enter search pattern"}, "", 256, a.selectByPattern)
	case keys.CmdDeselectItems:
		switch a.Mode {
		case keys.ModeNormal:
			a.Cur.DeselectAll()
		case keys.ModeFileSelection:
			a.fm.DeselectAll()
		}
	case keys.CmdRefresh:
		a.view.Sync()
	case keys.CmdEnter:
		a.enter()
	case keys.CmdToggleDisplay:
		switch a.Mode {
		case keys.ModeNormal:
			gv := a.groupView(a.Cur)
			gv.display = (gv.display + 1) % 3
		case keys.ModeFileSelection:
			a.fmDisplay = (a.fmDisplay + 1) % 3
		}
	case keys.CmdSelectFiles:
		a.fmBegin()
	case keys.CmdAddGroup:
		p.QuitAfterPlaylist = false
		a.Cur.AddGroup(playlist.NewGroup("[Default]"))
	case keys.CmdWritePlaylist:
		a.openPopup([]string{
			"Playlist will be saved in: ",
			a.Opts.PlaylistDir,
			"(Configuration keyword to set default dir: PlaylistDir)",
			"Example filename: myplaylist.lst",
		}, "", 40, a.writePlaylist)
	case keys.CmdLoadPlaylist:
		if a.Mode == keys.ModeNormal {
			a.fmBegin()
		}
		if a.Mode == keys.ModeFileSelection {
			a.fmChangeDir(a.Opts.PlaylistDir, false)
		}
	case keys.CmdSetGroupTitle:
		v := a.MainView()
		if a.Cur.IsGroup(v.Cursor) && !a.Cur.IsParentLink(v.Cursor) {
			a.openPopup([]string{"Enter Name:"}, "", 48, a.setGroupName)
		}
	case keys.CmdToggleRepeat:
		p.Repeat = !p.Repeat
		a.Opts.Repeat = p.Repeat
		if p.Repeat {
			p.QuitAfterPlaylist = false
		}
	case keys.CmdToggleShuffle:
		a.Cur.Shuffle = !a.Cur.Shuffle
		p.ResetNextSong()
	case keys.CmdTogglePlaymode:
		if p.Playing() {
			a.Warning("Not possible during playback")
			break
		}
		p.Mode = p.Mode.Next()
		a.Opts.PlayMode = p.Mode
		p.ResetNextSong()
	case keys.CmdChangeThread:
		a.Opts.CycleThreads()
	case keys.CmdClearPlaylist:
		if !p.Playing() {
			a.Root.Clear()
			a.Cur = a.Root
			a.views = map[*playlist.Group]*groupView{}
			p.ResetPlaylist()
			p.ResetNextSong()
		}
	case keys.CmdHelpPrev:
		a.HelpBar.PageUp(len(a.HelpBarLines()))
	case keys.CmdHelpNext:
		a.HelpBar.PageDown(len(a.HelpBarLines()))
	case keys.CmdFileStartSearch:
		if a.Mode != keys.ModeHelp {
			a.startSearch()
		}
	case keys.CmdToggleMixer:
		if a.Mixer != nil {
			a.Mixer.Next()
		}
	case keys.CmdMixerVolDown:
		if a.Mixer != nil {
			a.Mixer.Change(-2)
		}
	case keys.CmdMixerVolUp:
		if a.Mixer != nil {
			a.Mixer.Change(2)
		}
	default:
		a.execFileCommand(cmd)
	}
}

// mixerKey handles keys that are not bound to a command, like the original
// passes them to the mixer.
func (a *App) mixerKey(k keys.Key) {
	if a.Mixer == nil || k.Code != tcell.KeyRune {
		return
	}
	switch k.Rune {
	case 'T', 'k':
		a.Mixer.Prev()
	case 'j':
		a.Mixer.Next()
	case 'l':
		a.Mixer.Change(2)
	case 'h':
		a.Mixer.Change(-2)
	case '0':
		a.Mixer.Set(0)
	}
}

func (a *App) setMode(m keys.Mode) {
	a.Mode = m
	a.HelpBar = listview.State{Rows: 4}
	if m == keys.ModeHelp {
		a.helpView = listview.State{}
	}
}

func (a *App) pan(delta int) {
	if a.Mode == keys.ModeHelp {
		return
	}
	v := a.MainView()
	from, to := v.Visible(a.MainLen())
	longest := 0
	for i := from; i < to; i++ {
		if l := len([]rune(a.MainRow(i).Text)); l > longest {
			longest = l
		}
	}
	v.PanBy(delta, longest, a.mainWidth)
}

// SetMainWidth tells the app how many columns the main window shows; it is
// needed to limit horizontal panning.
func (a *App) SetMainWidth(w int) { a.mainWidth = w }

func (a *App) enter() {
	if a.Mode != keys.ModeNormal {
		return
	}
	v := a.MainView()
	if sub := a.Cur.SubGroup(v.Cursor); sub != nil {
		disp := a.groupView(a.Cur).display
		a.Cur = sub
		gv := a.groupView(sub)
		gv.display = disp
		gv.state.Pan = 0
		return
	}
	if it := a.Cur.Item(v.Cursor); it != nil {
		a.Player.PlayOne(it.Path)
	}
}

func (a *App) deleteItem(mark bool) {
	if a.Mode != keys.ModeNormal {
		return
	}
	v := a.MainView()
	idx := v.Cursor
	it := a.Cur.Item(idx)
	if it == nil || it.Kind == playlist.KindParent {
		return
	}
	if it.Kind == playlist.KindGroup && it.Group.IsPlaying() {
		if a.Player.Playing() {
			a.Warning("Can't delete an active group.")
			return
		}
		a.Player.ResetPlaylist()
	}
	if it.Kind == playlist.KindGroup {
		a.Player.ForgetGroup(it.Group)
	}
	if mark && it.Kind == playlist.KindSong && a.Matcher.IsAudio(it.Path) {
		if err := markBad(it.Path); err != nil {
			a.Warning("Failed to mark file as bad.")
		}
	}
	a.Cur.Delete(idx)
	v.Clamp(a.Cur.Len())
	a.Player.ResetNextSong()
}

func (a *App) setGroupName(name string) {
	v := a.MainView()
	if !a.Cur.IsGroup(v.Cursor) || a.Cur.IsParentLink(v.Cursor) || name == ".." {
		return
	}
	a.Cur.SubGroup(v.Cursor).Title = "[" + name + "]"
}

// --- popups -------------------------------------------------------------------

func (a *App) openPopup(labels []string, initial string, maxLen int, submit func(string)) {
	a.Popup = &Popup{Labels: labels, Editor: NewLineEditor(initial, maxLen), submit: submit}
	a.Input = InputText
	a.SetMessage("Use up and down arrows to toggle insert mode")
}

// openConfirm shows a yes/no dialog; onYes runs only if the user answers 'y'.
func (a *App) openConfirm(labels []string, onYes func()) {
	a.Popup = &Popup{Labels: labels, Confirm: true, onYes: onYes}
	a.Input = InputConfirm
	a.SetMessage("Press 'y' to confirm, 'n' or Esc to cancel")
}

func (a *App) confirmKey(k keys.Key) {
	switch {
	case k.Code == tcell.KeyRune && (k.Rune == 'y' || k.Rune == 'Y'):
		onYes := a.Popup.onYes
		a.ClosePopup()
		onYes()
	case k.Code == tcell.KeyRune && (k.Rune == 'n' || k.Rune == 'N'),
		k.Code == tcell.KeyEsc, k.Code == tcell.KeyEnter:
		a.ClosePopup()
		a.SetMessage("Cancelled.")
	}
}

// ClosePopup cancels the popup.
func (a *App) ClosePopup() {
	a.Popup = nil
	a.Input = InputDefault
	a.Msg = Message{}
}

func (a *App) popupKey(k keys.Key) {
	e := a.Popup.Editor
	switch k.Code {
	case tcell.KeyLeft:
		e.Left()
	case tcell.KeyRight:
		e.Right()
	case tcell.KeyUp, tcell.KeyDown:
		if e.ToggleInsert() {
			a.SetMessage("--  INSERT --")
		} else {
			a.SetMessage("-- REPLACE --")
		}
	case tcell.KeyBackspace:
		e.Backspace()
	case tcell.KeyDelete:
		e.Delete()
	case tcell.KeyHome:
		e.Home()
	case tcell.KeyEnd:
		e.End()
	case tcell.KeyEsc:
		a.ClosePopup()
	case tcell.KeyEnter:
		submit, text := a.Popup.submit, e.Text()
		a.ClosePopup()
		submit(text)
	case tcell.KeyRune:
		e.AddChar(k.Rune)
	}
}

// --- searching ------------------------------------------------------------------

func (a *App) startSearch() {
	a.search = ""
	a.Input = InputSearch
	a.SetMessage("Search:")
	a.armSearchTimeout()
}

func (a *App) armSearchTimeout() {
	a.searchGen++
	gen := a.searchGen
	time.AfterFunc(2*time.Second, func() {
		a.view.Post(func() {
			if a.Input == InputSearch && a.searchGen == gen {
				a.endSearch()
			}
		})
	})
}

func (a *App) endSearch() {
	a.search = ""
	a.Input = InputDefault
	a.SetMessage("")
}

const searchChars = " (){}[]<>,.?;:\"'=+-_!@#$%^&*"

func (a *App) searchKey(k keys.Key) {
	if k.Code == tcell.KeyEnter || k.Code == tcell.KeyEsc {
		a.endSearch()
		return
	}
	if k.Code != tcell.KeyRune {
		return
	}
	r := k.Rune
	if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(searchChars, r) || r > 127) {
		return
	}
	needle := a.search + string(r)
	found := false
	n := a.MainLen()
	for i := 0; i < n; i++ {
		var name string
		if a.Mode == keys.ModeNormal {
			name = a.Cur.Item(i).Name()
			if a.Cur.Item(i).Kind == playlist.KindSong {
				name = path.Base(strings.ReplaceAll(name, "\\", "/"))
			}
		} else {
			name = a.fm.Entry(i).Name
		}
		if strings.HasPrefix(name, needle) {
			a.MainView().Set(i, n)
			found = true
			break
		}
	}
	a.armSearchTimeout()
	if !found {
		a.view.Beep()
		return
	}
	a.search = needle
	a.SetMessage("Search: " + a.search)
}

// --- selection by pattern ---------------------------------------------------------

func (a *App) selectByPattern(pattern string) {
	if pattern == "" {
		a.Warning("No pattern given.")
		return
	}
	ci := a.Opts.SelectCaseInsensitive
	var match func(string) bool
	if a.Opts.SelectRegex {
		expr := pattern
		if ci {
			expr = "(?i)" + expr
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			a.Warning("Invalid regular expression.")
			return
		}
		match = re.MatchString
	} else {
		pat := pattern
		if ci {
			pat = strings.ToLower(pat)
		}
		match = func(s string) bool {
			if ci {
				s = strings.ToLower(s)
			}
			ok, _ := path.Match(pat, s)
			return ok
		}
	}
	switch a.Mode {
	case keys.ModeNormal:
		if a.Opts.SelectUnselectFirst {
			a.Cur.DeselectAll()
		}
		for i := 0; i < a.Cur.Len(); i++ {
			if match(a.MainRow(i).Text) {
				a.Cur.Select(i)
			}
		}
	case keys.ModeFileSelection:
		if a.Opts.SelectUnselectFirst {
			a.fm.DeselectAll()
		}
		for i := 0; i < a.fm.Len(); i++ {
			if match(a.MainRow(i).Text) {
				a.fm.Select(i)
			}
		}
	}
}

// PlayModeName returns the play mode for playlist files.
func playModeName(m playback.PlayMode) string { return m.String() }
