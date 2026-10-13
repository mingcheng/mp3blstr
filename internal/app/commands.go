package app

import (
	"github.com/gdamore/tcell/v2"

	"github.com/mingcheng/mp3blasterr/internal/config"
	"github.com/mingcheng/mp3blasterr/internal/keys"
	"github.com/mingcheng/mp3blasterr/internal/listview"
	"github.com/mingcheng/mp3blasterr/internal/playlist"
)

// HandleKey processes a key press. While a popup, a search or a task is
// active, it gets all keys; otherwise the key is looked up in the key table
// for the current mode.
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

// Exec runs a command. Commands that make no sense in the current mode are
// ignored.
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
			a.SetMessage("Visit https://github.com/mingcheng/mp3blasterr")
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
		a.Opts.Theme = config.NextTheme(a.Opts.Theme)
		a.SetMessage("Theme: " + a.Opts.Theme)
	case keys.CmdToggleNerdFont:
		a.Opts.NerdFont = !a.Opts.NerdFont
		if a.Opts.NerdFont {
			a.SetMessage("Nerd Font icons enabled")
		} else {
			a.SetMessage("Nerd Font icons disabled")
		}
	case keys.CmdDel:
		a.deleteItem()
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
			gv.display = gv.display.Next()
		case keys.ModeFileSelection:
			a.fmDisplay = a.fmDisplay.Next()
		}
	case keys.CmdSelectFiles:
		a.fmBegin()
	case keys.CmdAddGroup:
		p.QuitAfterPlaylist = false
		a.Cur.AddGroup(playlist.NewGroup(defaultGroupTitle))
	case keys.CmdWritePlaylist:
		a.openPopup([]string{
			"Playlist will be saved in: ",
			a.Opts.PlaylistDir,
			"(Default dir setting: files.playlist_dir)",
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
	case keys.CmdToggleFadeOut:
		a.Opts.FadeOut = !a.Opts.FadeOut
		if a.fadeMixer != nil {
			a.fadeMixer.SetFade(a.Opts.FadeOut)
		}
		if a.Opts.FadeOut {
			a.SetMessage("Fade out enabled")
		} else {
			a.SetMessage("Fade out disabled")
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

// setMode switches the program mode; the help bar scrolls back to the top.
func (a *App) setMode(m keys.Mode) {
	a.Mode = m
	a.HelpBar = listview.State{Rows: helpBarRows}
	if m == keys.ModeHelp {
		a.helpView = listview.State{}
	}
}

// pan scrolls the main window horizontally, up to the end of the longest
// visible line.
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

// enter opens the group under the cursor or plays the song under it.
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

// deleteItem removes the entry under the cursor from the playlist.
func (a *App) deleteItem() {
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
	a.Cur.Delete(idx)
	v.Clamp(a.Cur.Len())
	a.Player.ResetNextSong()
}

// setGroupName renames the group under the cursor.
func (a *App) setGroupName(name string) {
	v := a.MainView()
	if !a.Cur.IsGroup(v.Cursor) || a.Cur.IsParentLink(v.Cursor) || name == ".." {
		return
	}
	a.Cur.SubGroup(v.Cursor).Title = "[" + name + "]"
}
