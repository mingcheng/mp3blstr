// Package app implements mp3blstr's behaviour independently of the
// terminal toolkit: program modes, the command set, popups, the playlist
// window, the file manager and the help screen. The ui package renders this
// state and feeds key presses and mouse clicks into it.
package app

import (
	_ "embed"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mingcheng/mp3blstr/internal/audio"
	"github.com/mingcheng/mp3blstr/internal/config"
	"github.com/mingcheng/mp3blstr/internal/filemanager"
	"github.com/mingcheng/mp3blstr/internal/keys"
	"github.com/mingcheng/mp3blstr/internal/listview"
	"github.com/mingcheng/mp3blstr/internal/media"
	"github.com/mingcheng/mp3blstr/internal/playback"
	"github.com/mingcheng/mp3blstr/internal/playlist"
)

//go:embed help.txt
var helpText string

// View is implemented by the user interface.
type View interface {
	// Post runs f on the UI goroutine and redraws the screen. It may block
	// until f has run, so it must only be called from other goroutines.
	Post(f func())
	Beep()
	Sync() // redraw the whole screen (^L)
	Quit()
}

// InputMode tells how key presses are interpreted.
type InputMode int

// Input modes.
const (
	InputDefault InputMode = iota
	InputSearch
	InputText
	InputConfirm
	InputTask
)

// Message is the text in the message line at the bottom of the screen.
type Message struct {
	Text  string
	Error bool
}

// Popup is a dialog asking for a line of text, or, when Confirm is set, a
// yes/no question (Editor is nil then).
type Popup struct {
	Labels  []string
	Editor  *LineEditor
	Confirm bool
	submit  func(string) // called with the text on Enter
	onYes   func()       // called when a confirmation is answered with 'y'
}

// RowKind tells the renderer how to colour a line of the main window.
type RowKind int

// Row kinds.
const (
	RowPlain RowKind = iota
	RowSong
	RowGroup
	RowParent
	RowDir
	RowPlaylist
	RowOther
)

// Row is a line of the main window.
type Row struct {
	Text    string
	Right   string
	Kind    RowKind
	Marked  bool
	Playing bool
}

// groupView is the view state of a playlist group in the main window.
type groupView struct {
	state   listview.State
	display playlist.DisplayMode
}

// helpBarRows is the height of the key overview at the top of the screen.
const helpBarRows = 4

// App is the application state.
type App struct {
	Version          string
	Opts             *config.Options
	Keys             *keys.Table
	Matcher          *media.Matcher
	Mixer            *audio.Mixer // nil when the mixer is disabled
	fadeMixer        *audio.Mixer // audio gain control also present with --no-mixer
	Player           *playback.Controller
	OnPlaybackChange func()

	Root *playlist.Group
	Cur  *playlist.Group

	Mode  keys.Mode
	Input InputMode
	Popup *Popup
	Task  *Task // running background task, shown as a progress dialog
	Msg   Message

	// HelpBar is the scroll state of the key overview at the top.
	HelpBar listview.State

	views map[*playlist.Group]*groupView

	// File selection mode.
	fm        *filemanager.Manager // nil until the first fmBegin
	fmView    listview.State
	fmDisplay filemanager.DisplayMode
	fmDir     string
	startDir  string   // fmDir at start; LastDir is saved when it changed
	pending   []string // files selected in other directories
	captured  bool     // CaptureState ran

	// Help mode.
	help     []string
	helpView listview.State

	// Incremental search ('/').
	search    string
	searchGen int // invalidates the timeouts of earlier keystrokes

	statusFile    string
	statusSig     string // last state written to statusFile
	mainWidth     int    // columns of the main window, for panning
	durations     map[string]int
	durationQueue []string
	durationBusy  bool

	view  View
	log   *slog.Logger
	trash func(string) error
}

// defaultGroupTitle is the title of the root group and of new groups.
const defaultGroupTitle = "[Default]"

// Deps are the collaborators and settings an App is created with.
type Deps struct {
	Version    string // formatted build version for the UI and About text
	Options    *config.Options
	Keys       *keys.Table
	Matcher    *media.Matcher
	Mixer      *audio.Mixer     // nil disables the mixer
	FadeMixer  *audio.Mixer     // output mixer when different from the visible mixer
	Backend    playback.Backend // opens songs for the player
	View       View
	StartDir   string             // initial directory of the file manager
	StatusFile string             // --status-file; "" for none
	Logger     *slog.Logger       // nil discards debug output
	Trash      func(string) error // nil uses the native file manager
}

// New creates the application.
func New(d Deps) *App {
	root := playlist.NewGroup(defaultGroupTitle)
	log := d.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	a := &App{
		Version:    d.Version,
		Opts:       d.Options,
		Keys:       d.Keys,
		Matcher:    d.Matcher,
		Mixer:      d.Mixer,
		fadeMixer:  d.FadeMixer,
		Root:       root,
		Cur:        root,
		Mode:       keys.ModeNormal,
		HelpBar:    listview.State{Rows: helpBarRows},
		views:      map[*playlist.Group]*groupView{},
		durations:  map[string]int{},
		fmDir:      d.StartDir,
		startDir:   d.StartDir,
		statusFile: d.StatusFile,
		view:       d.View,
		log:        log,
		trash:      d.Trash,
		help:       append(strings.Split(strings.TrimRight(strings.ReplaceAll(helpText, "{{VERSION}}", d.Version), "\n"), "\n"), keyReference(d.Keys)...),
	}
	if a.trash == nil {
		a.trash = filemanager.Trash
	}
	a.Player = playback.New(root, d.Backend, d.View.Post, (*listener)(a))
	a.Player.Mode = d.Options.PlayMode
	a.Player.Repeat = d.Options.Repeat
	if a.fadeMixer == nil {
		a.fadeMixer = a.Mixer
	}
	if a.fadeMixer != nil {
		a.fadeMixer.SetFade(d.Options.FadeOut)
	}
	a.Player.SkipSeconds = d.Options.SkipSeconds
	a.SetMessage("Press '?' to get help on available commands")
	return a
}

// --- playback listener ------------------------------------------------------

// listener implements playback.Listener for the App without adding these
// methods to the App's own API.
type listener App

func (l *listener) Changed()           { (*App)(l).playerChanged() }
func (l *listener) Warning(m string)   { (*App)(l).Warning(m) }
func (l *listener) Message(m string)   { (*App)(l).SetMessage(m) }
func (l *listener) ClearMessage()      { (*App)(l).Msg = Message{} }
func (l *listener) EndOfPlaylistQuit() { (*App)(l).Quit() }
func (l *listener) CurrentGroup() *playlist.Group {
	return (*App)(l).Cur
}

// SongStarted shows the album and comment of the new song in the message
// line, like set_song_info does in the original.
func (l *listener) SongStarted(info media.SongInfo) {
	album, comment := info.Album, info.Comment
	if album == "" {
		album = "<Unknown Album>"
	}
	if comment == "" {
		comment = "no comments"
	}
	(*App)(l).SetMessage(album + " (" + comment + ")")
}

// playerChanged rewrites the status file when the state it shows changed.
func (a *App) playerChanged() {
	if a.OnPlaybackChange != nil {
		a.OnPlaybackChange()
	}
	if a.statusFile == "" {
		return
	}
	path, _, _ := a.Player.SongInfo()
	sig := fmt.Sprintf("%s\x00%s\x00%d\x00%v", path, a.Player.NextSongName(), a.Player.Status(), a.Player.Paused())
	if sig == a.statusSig {
		return
	}
	a.statusSig = sig
	if err := a.Player.WriteStatusFile(a.statusFile); err != nil {
		a.log.Debug("status file", "err", err)
	}
}

// --- messages -----------------------------------------------------------------

// SetMessage shows txt in the message line.
func (a *App) SetMessage(txt string) { a.Msg = Message{Text: txt} }

// Warning shows txt as an error in the message line.
func (a *App) Warning(txt string) { a.Msg = Message{Text: txt, Error: true} }

// Quit stops playback and leaves the program.
func (a *App) Quit() {
	a.endTask()
	a.CaptureState()
	a.Player.Shutdown()
	a.view.Quit()
}

// CaptureState copies the state that is remembered between sessions into
// Opts: the file manager directory, the song being played and its position,
// the mixer, repeat and the play mode. It must run before playback is shut
// down; later calls do nothing.
func (a *App) CaptureState() {
	if a.captured {
		return
	}
	a.captured = true
	if dir := filepath.Clean(a.fmDir); dir != filepath.Clean(a.startDir) {
		a.Opts.LastDir = dir
	}
	a.Opts.LastSong, a.Opts.LastPosition = a.Player.ResumePoint()
	if a.Mixer != nil {
		a.Opts.Volume = a.Mixer.Device(audio.DevVolume).Value
		a.Opts.Balance = a.Mixer.Device(audio.DevBalance).Value
	}
	a.Opts.Repeat = a.Player.Repeat
	a.Opts.PlayMode = a.Player.Mode
}

// RestoreSession loads the playlist saved by WriteSession and prepares the
// last song (Opts.LastSong at Opts.LastPosition) to continue on the next
// Play, without starting playback.
func (a *App) RestoreSession(path string) {
	if gs, err := playlist.Load(path, a.Root); err == nil {
		a.Root.Shuffle = gs.Shuffle
	}
	if a.Opts.LastSong == "" {
		return
	}
	a.Player.Resume(a.Opts.LastSong, a.Opts.LastPosition)
	a.SetMessage(fmt.Sprintf("Press '%s' to continue %s at %d:%02d",
		strings.TrimSpace(a.Keys.LabelFor(keys.CmdPlayPlay)), media.ChopPath(a.Opts.LastSong),
		a.Opts.LastPosition/60, a.Opts.LastPosition%60))
}

// WriteSession saves the playlist for RestoreSession.
func (a *App) WriteSession(path string) error {
	return playlist.Save(path, a.Root, a.Player.Mode.String())
}

// Tick is called periodically by the UI to update the play time. It
// reports whether the screen needs to be redrawn: the play time changed
// or a progress dialog is animating.
func (a *App) Tick() bool { return a.Player.Tick() || a.Task != nil }

// mpegFrameDuration is the length of an MPEG audio frame at 44.1 kHz.
const mpegFrameDuration = 26 * time.Millisecond

// TickInterval is zero when no animation is needed. --runframes controls
// scan animation; playback time only needs a once-per-second refresh.
func (a *App) TickInterval() time.Duration {
	if a.Task != nil {
		return time.Duration(a.Opts.FPL) * 2 * mpegFrameDuration
	}
	if !a.Player.HasSong() || a.Player.Paused() {
		return 0
	}
	return time.Second
}

// --- main window ------------------------------------------------------------

func (a *App) groupView(g *playlist.Group) *groupView {
	v, ok := a.views[g]
	if !ok {
		v = &groupView{display: a.Opts.DisplayMode}
		a.views[g] = v
	}
	return v
}

// MainView returns the scroll state of the main window.
func (a *App) MainView() *listview.State {
	switch a.Mode {
	case keys.ModeFileSelection:
		return &a.fmView
	case keys.ModeHelp:
		return &a.helpView
	default:
		return &a.groupView(a.Cur).state
	}
}

// MainLen returns the number of lines in the main window.
func (a *App) MainLen() int {
	switch a.Mode {
	case keys.ModeFileSelection:
		return a.fm.Len()
	case keys.ModeHelp:
		return len(a.help)
	default:
		return a.Cur.Len()
	}
}

// MainTitle returns the title drawn in the border of the main window.
func (a *App) MainTitle() string {
	switch a.Mode {
	case keys.ModeFileSelection:
		return a.fm.Dir()
	case keys.ModeHelp:
		return ""
	default:
		return a.Cur.Title
	}
}

// MainRow returns line i of the main window.
func (a *App) MainRow(i int) Row {
	switch a.Mode {
	case keys.ModeFileSelection:
		e := a.fm.Entry(i)
		if e == nil {
			return Row{}
		}
		r := Row{Text: a.fm.DisplayName(i, a.fmDisplay), Marked: e.Selected()}
		switch e.Kind {
		case filemanager.KindDir:
			r.Kind = RowDir
		case filemanager.KindAudio:
			r.Kind = RowSong
		case filemanager.KindPlaylist:
			r.Kind = RowPlaylist
		default:
			r.Kind = RowOther
		}
		return r
	case keys.ModeHelp:
		if i < 0 || i >= len(a.help) {
			return Row{}
		}
		return Row{Text: a.help[i]}
	default:
		it := a.Cur.Item(i)
		if it == nil {
			return Row{}
		}
		r := Row{Text: playlist.DisplayName(it, a.groupView(a.Cur).display), Marked: it.Selected()}
		switch it.Kind {
		case playlist.KindGroup:
			r.Kind = RowGroup
		case playlist.KindParent:
			r.Kind = RowParent
		default:
			r.Kind = RowSong
			if !media.IsHTTPStream(it.Path) {
				switch a.groupView(a.Cur).display {
				case playlist.DisplayFileName:
					if info, err := os.Stat(it.Path); err == nil {
						r.Right = filemanager.SizeDesc(info.Size())
					}
				case playlist.DisplayID3:
					if seconds, ok := a.durations[it.Path]; ok {
						if seconds >= 0 {
							r.Right = fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
							if seconds >= 3600 {
								r.Right = fmt.Sprintf("%d:%02d:%02d", seconds/3600, seconds%3600/60, seconds%60)
							}
						}
					} else {
						a.durations[it.Path] = -1
						a.durationQueue = append(a.durationQueue, it.Path)
						a.loadNextDuration()
					}
				}
			}
			if p, _, ok := a.Player.SongInfo(); ok && a.Player.HasSong() && p == it.Path {
				r.Playing = true
			}
		}
		return r
	}
}

func (a *App) loadNextDuration() {
	if a.durationBusy || len(a.durationQueue) == 0 {
		return
	}
	path := a.durationQueue[0]
	a.durationQueue = a.durationQueue[1:]
	a.durationBusy = true
	go func() {
		seconds, err := media.CachedDuration(path, audio.Duration)
		a.view.Post(func() {
			if err == nil {
				a.durations[path] = seconds
			}
			a.durationBusy = false
			a.loadNextDuration()
		})
	}()
}

// --- status area --------------------------------------------------------------

// ModeLine returns the text of the line below the help bar: the global play
// mode, or the sort mode in the file manager.
func (a *App) ModeLine() string {
	if a.Mode == keys.ModeFileSelection {
		return "Sorting mode   : " + a.fm.Sort().Description()
	}
	return a.Player.Mode.Description()
}

// HelpEntry is one item of the key overview.
type HelpEntry struct {
	Label string
	Desc  string
	Cmd   keys.Command
}

// HelpBarLines returns the key overview, three entries per line. Every
// category starts on a new line.
func (a *App) HelpBarLines() [][]HelpEntry {
	var lines [][]HelpEntry
	for _, group := range a.Keys.Grouped(a.Mode, true) {
		var line []HelpEntry
		for _, b := range group {
			line = append(line, HelpEntry{Label: b.Key.Label(), Desc: b.Desc, Cmd: b.Cmd})
			if len(line) == 3 {
				lines = append(lines, line)
				line = nil
			}
		}
		if len(line) > 0 {
			lines = append(lines, line)
		}
	}
	return lines
}

// keyReference lists all working key bindings by category for the end of
// the help screen.
func keyReference(km *keys.Table) []string {
	out := []string{"", "Key bindings", "============"}
	for _, group := range km.Grouped(keys.ModeAny, false) {
		out = append(out, "", group[0].Group.String())
		// The same key and text in several modes is listed once.
		var merged []keys.Binding
		for _, b := range group {
			dup := false
			for i := range merged {
				if merged[i].Key == b.Key && merged[i].Desc == b.Desc {
					merged[i].Modes |= b.Modes
					dup = true
				}
			}
			if !dup {
				merged = append(merged, b)
			}
		}
		for _, b := range merged {
			line := fmt.Sprintf("  %-5s %-20s %s", b.Key.Name(), b.Desc, modeNote(b.Modes))
			out = append(out, strings.TrimRight(line, " "))
		}
	}
	return out
}

func modeNote(m keys.Mode) string {
	switch m {
	case keys.ModeNormal:
		return "(Normal Mode)"
	case keys.ModeFileSelection:
		return "(File Mode)"
	case keys.ModeNormal | keys.ModeFileSelection:
		return "(Normal/File Mode)"
	}
	return ""
}

// Shuffle reports whether the current group plays in random order.
func (a *App) Shuffle() bool { return a.Cur.Shuffle }

// Repeat reports whether the playlist repeats.
func (a *App) Repeat() bool { return a.Player.Repeat }

func (a *App) SetRepeat(enabled, track bool) {
	a.Player.Repeat = enabled
	a.Player.RepeatTrack = enabled && track
	a.Opts.Repeat = enabled
	if enabled {
		a.Player.QuitAfterPlaylist = false
	}
	a.playerChanged()
}

func (a *App) SetShuffle(enabled bool) {
	a.Cur.Shuffle = enabled
	a.Player.ResetNextSong()
}
