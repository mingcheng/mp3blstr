// Package playback drives the playlist: it decides which song plays next
// (in group order, shuffled groups or all songs at random), keeps the play
// history and reacts to user actions such as play/pause, next and stop.
//
// The Controller is not safe for concurrent use: every method must be called
// from one goroutine (the UI event loop). Background work (opening files,
// waiting for a song to finish) reports back through the Post function.
package playback

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/mingcheng/mp3blstr/internal/media"
	"github.com/mingcheng/mp3blstr/internal/playlist"
)

// PlayMode is the global playback mode.
type PlayMode int

// Play modes.
const (
	PlayNone   PlayMode = iota
	PlayGroup           // play the current group, not its sub groups
	PlayGroups          // play the current group including sub groups
	PlaySongs           // shuffle all songs from all groups
)

// Next cycles through the user selectable modes.
func (m PlayMode) Next() PlayMode {
	switch m {
	case PlayGroups:
		return PlayGroup
	case PlayGroup:
		return PlaySongs
	default:
		return PlayGroups
	}
}

// Description is shown on the "Global Playback" line.
func (m PlayMode) Description() string {
	switch m {
	case PlayGroup:
		return "Global Playback: Play current group, but not its subgroups"
	case PlayGroups:
		return "Global Playback: Play current group, including subgroups"
	case PlaySongs:
		return "Global Playback: Shuffle all songs from all groups"
	}
	return ""
}

// String returns the name used on the command line, in the configuration
// file and in playlists ("" for PlayNone).
func (m PlayMode) String() string {
	switch m {
	case PlayGroup:
		return "onegroup"
	case PlayGroups:
		return "allgroups"
	case PlaySongs:
		return "allrandom"
	}
	return ""
}

// ParsePlayMode parses onegroup, allgroups or allrandom.
func ParsePlayMode(s string) (PlayMode, error) {
	switch strings.ToLower(s) {
	case "onegroup":
		return PlayGroup, nil
	case "allgroups":
		return PlayGroups, nil
	case "allrandom":
		return PlaySongs, nil
	}
	return PlayNone, fmt.Errorf("invalid play mode %q", s)
}

// Status is the state shown in the bottom left corner.
type Status int

// Song states.
const (
	StatusNone Status = iota
	StatusPlay
	StatusPause
	StatusStop
)

// Track is a song opened by the Backend. Its methods are called on the UI
// goroutine.
type Track interface {
	Start()                     // start or resume playback
	Pause()                     // pause playback
	Stop()                      // end playback and release resources
	Done() <-chan struct{}      // closed when the song has been played
	Info() media.SongInfo       // song metadata and stream parameters
	Elapsed() int               // play position in seconds
	Seek(delta int) error       // move the position by delta seconds
	SeekFromEnd(secs int) error // jump to secs seconds before the end
	Seekable() bool             // whether the opened source supports seeking
	Err() error                 // the error that ended playback (after Stop)
}

// Backend opens songs. Errors should wrap a media.SoundError; wrapping
// media.ErrDeviceOpen stops the playlist instead of skipping to the next
// song.
type Backend interface {
	// Opener is called on the UI goroutine and returns a function that
	// opens path. That function may block and runs on its own goroutine.
	Opener(path string) func() (Track, error)
}

// Listener receives notifications. All methods are called on the UI
// goroutine.
type Listener interface {
	Changed()                        // display state changed
	SongStarted(info media.SongInfo) // a new song started playing
	Warning(msg string)              // show an error in the message line
	Message(msg string)              // show a message in the message line
	ClearMessage()                   // clear the message line
	EndOfPlaylistQuit()              // quit-after-playlist reached the end
	CurrentGroup() *playlist.Group   // the group shown in the main window
}

// single tells how the current song relates to the playlist.
type single int

const (
	singleNone     single = iota // a song chosen from the playlist
	singleThenList               // a single song; the playlist continues afterwards
	singleOnly                   // a single song; playback stops afterwards
)

// Controller implements the playlist logic.
type Controller struct {
	backend  Backend
	post     func(func())
	listener Listener
	root     *playlist.Group
	rnd      *rand.Rand

	// Settings, changed directly by the application.
	Mode              PlayMode
	Repeat            bool
	RepeatTrack       bool
	SkipSeconds       int
	QuitAfterPlaylist bool

	playing     bool // the user wants the playlist to play
	track       Track
	paused      bool
	opening     bool          // a song is being opened in the background
	gen         int           // invalidates callbacks of earlier songs
	trackQuit   chan struct{} // stops the goroutine waiting for Done
	oneMp3      string        // song requested by PlayOne, not started yet
	single      single
	allowRepeat bool // the last song ended normally; Repeat may restart the list
	history     History
	nextSong    int             // precomputed index of the next unplayed song, -1 if none
	playingGrp  *playlist.Group // group whose songs are being played
	groupWin    *playlist.Group // group whose sub groups are played (PlayGroups)

	// A song to continue with on the next Play (restored session).
	resumeSong string
	resumePos  int
	seekOnOpen int // seconds to skip when the song being opened starts

	// Display state.
	status   Status
	songPath string
	info     media.SongInfo
	haveInfo bool
	elapsed  int
	nextName string
	revision int
}

// New creates a controller for the playlist rooted at root.
func New(root *playlist.Group, backend Backend, post func(func()), l Listener) *Controller {
	return &Controller{
		backend:     backend,
		post:        post,
		listener:    l,
		root:        root,
		rnd:         rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
		Mode:        PlayGroups,
		SkipSeconds: 10,
		nextSong:    -1,
	}
}

// SetRand replaces the random source (for tests).
func (c *Controller) SetRand(r *rand.Rand) { c.rnd = r }

// --- queries ---------------------------------------------------------------

// Playing reports whether the playlist is active.
func (c *Controller) Playing() bool { return c.playing }

// HasSong reports whether a song is loaded.
func (c *Controller) HasSong() bool { return c.track != nil }

// Paused reports whether the loaded song is paused.
func (c *Controller) Paused() bool { return c.paused }

// ShowPlaySymbol reports whether the play/pause button should show "play".
func (c *Controller) ShowPlaySymbol() bool {
	return !c.playing || (c.track != nil && c.paused)
}

// Status returns the song status.
func (c *Controller) Status() Status { return c.status }

// SongInfo returns information about the last loaded song.
func (c *Controller) SongInfo() (path string, info media.SongInfo, ok bool) {
	return c.songPath, c.info, c.haveInfo
}

// Elapsed returns the playing time of the current song.
func (c *Controller) Elapsed() int { return c.elapsed }

// Revision changes when the track or its playback position is replaced.
func (c *Controller) Revision() int { return c.revision }

func (c *Controller) TrackID() int { return c.gen }

func (c *Controller) CanSeek() bool {
	return c.track != nil && !c.opening && c.info.TotalSeconds > 0 && c.track.Seekable()
}

// NextSongName returns the name of the song that will play next.
func (c *Controller) NextSongName() string { return c.nextName }

// --- user actions ----------------------------------------------------------

// Play starts the playlist, or toggles pause when a song is loaded.
func (c *Controller) Play() {
	if c.opening {
		return
	}

	c.playing = true
	if c.track != nil {
		if c.paused {
			c.paused = false
			c.track.Start()
			c.status = StatusPlay
		} else {
			c.Pause()
			return
		}
		c.changed()
		return
	}
	c.allowRepeat = false
	if c.resumeSong != "" {
		c.startResume()
		c.changed()
		return
	}
	c.advance()
	c.changed()
}

// Pause also applies to an in-flight open, preventing a delayed decoder from
// starting audio after a sleep or output-device change.
func (c *Controller) Pause() {
	if !c.playing || c.paused {
		return
	}
	c.paused = true
	if c.track != nil {
		c.track.Pause()
		c.elapsed = c.track.Elapsed()
	}
	c.status = StatusPause
	c.changed()
}

// Resume makes the next Play continue song path at pos seconds, and then
// the playlist after that song (used to restore the previous session).
func (c *Controller) Resume(path string, pos int) {
	c.resumeSong, c.resumePos = path, max(pos, 0)
	c.nextName = media.ChopPath(path)
	c.changed()
}

// ResumePoint returns the song and position to continue with in the next
// session: the loaded song, or a pending Resume. path is empty when there
// is nothing to resume.
func (c *Controller) ResumePoint() (path string, pos int) {
	switch {
	case c.track != nil:
		return c.songPath, c.track.Elapsed()
	case c.resumeSong != "":
		return c.resumeSong, c.resumePos
	}
	return "", 0
}

func (c *Controller) clearResume() {
	if c.resumeSong != "" {
		c.resumeSong, c.resumePos = "", 0
		c.resetNext()
	}
}

// startResume opens the song to resume and marks the playlist as if it had
// been played up to that song.
func (c *Controller) startResume() {
	song, pos := c.resumeSong, c.resumePos
	c.clearResume()
	c.resetPlaylist(true)
	if c.positionAt(song) {
		c.history.Add(song)
	} else {
		c.single = singleThenList // not in the playlist: play it, then the playlist
	}
	c.seekOnOpen = pos
	c.open(song)
}

// positionAt prepares the playlist state so that playback continues after
// song. It reports false when the song is not in the playlist.
func (c *Controller) positionAt(song string) bool {
	g, it := c.root.FindSong(song)
	if it == nil {
		return false
	}
	if c.Mode == PlaySongs {
		it.SetPlayed(true)
		return true
	}
	if c.Mode == PlayGroups {
		c.groupWin = c.listener.CurrentGroup()
		if before, ok := c.groupWin.GroupsBefore(g); ok && !c.groupWin.Shuffle {
			for _, b := range before {
				b.SetPlayed(true)
			}
		}
		g.SetPlayed(true)
	}
	c.playingGrp = g
	g.SetPlaying(true)
	if g.Shuffle {
		it.SetPlayed(true)
		return true
	}
	g.MarkPlayedThrough(it)
	c.nextSong = 0
	c.updateNextName()
	return true
}

// PlayOne plays a single song immediately. When a playlist was playing it
// continues afterwards.
func (c *Controller) PlayOne(path string) {
	c.clearResume()
	c.oneMp3 = path
	c.playing = true
	// The playlist continues afterwards when it was playing.
	wasPlaying := false
	if c.track != nil || c.opening {
		c.stopSong()
		wasPlaying = c.single != singleOnly
	}
	c.single = singleNone
	if !c.startSong(wasPlaying) {
		c.advance()
	}
	c.changed()
}

// Next skips to the next song.
func (c *Controller) Next() {
	if !c.playing {
		return
	}
	c.stopSong()
	c.advance()
	c.changed()
}

// Previous jumps back in the play history.
func (c *Controller) Previous() {
	if !c.playing || c.history.AtStart() {
		return
	}
	// The position is just past the current song: step back over it and
	// over the song before it, which advance then replays.
	c.history.Previous()
	c.history.Previous()
	c.stopSong()
	c.advance()
	c.changed()
}

// Forward skips SkipSeconds ahead.
func (c *Controller) Forward() {
	if c.playing && c.track != nil {
		c.seek(c.track.Elapsed() + c.SkipSeconds)
	}
}

// Rewind skips SkipSeconds back.
func (c *Controller) Rewind() {
	if c.playing && c.track != nil {
		c.seek(c.track.Elapsed() - c.SkipSeconds)
	}
}

// SeekTo jumps to an absolute position (in seconds) in the current song.
func (c *Controller) SeekTo(secs int) {
	if c.playing && c.track != nil {
		c.seek(secs)
	}
}

func (c *Controller) seek(secs int) {
	if err := c.track.Seek(max(secs, 0) - c.track.Elapsed()); err != nil {
		c.listener.Warning(fmt.Sprintf("Couldn't seek: %v", err))
	}
	// Resync the actual position even when the system optimistically moved
	// its timeline but the decoder rejected the seek.
	c.revision++
	c.elapsed = c.track.Elapsed()
	c.changed()
}

// SkipToEnd jumps to the last seconds of the song.
func (c *Controller) SkipToEnd() {
	if c.playing && c.track != nil {
		if err := c.track.SeekFromEnd(8); err != nil {
			c.listener.Warning(fmt.Sprintf("Couldn't seek: %v", err))
			return
		}
		c.revision++
		c.elapsed = c.track.Elapsed()
		c.changed()
	}
}

// Stop stops playback and resets the playlist.
func (c *Controller) Stop() {
	c.clearResume()
	if !c.playing && c.track == nil && !c.opening {
		return
	}
	c.stopSong()
	c.playing = false
	c.resetPlaylist(true)
	c.single = singleNone
	c.resetNext()
	c.changed()
}

// NextGroup skips the rest of the group being played.
func (c *Controller) NextGroup() {
	if (c.Mode == PlayGroup || c.Mode == PlayGroups) && c.track != nil && c.playingGrp != nil {
		grp := c.playingGrp
		c.stopSong()
		grp.SetAllSongsPlayed(false)
		c.advance()
		c.changed()
	}
}

// PreviousGroup is not implemented, just like in the original.
func (c *Controller) PreviousGroup() {
	c.listener.Message("Sorry, that doesn't work yet.")
}

// ResetNextSong forgets the precomputed next song (after the playlist or the
// play mode changed).
func (c *Controller) ResetNextSong() {
	c.resetNext()
	c.changed()
}

// ResetPlaylist marks every song and group as unplayed.
func (c *Controller) ResetPlaylist() {
	c.clearResume()
	c.resetPlaylist(true)
}

// Tick refreshes the elapsed time; it returns true when it changed.
func (c *Controller) Tick() bool {
	if c.track == nil {
		return false
	}
	e := c.track.Elapsed()
	if e == c.elapsed {
		return false
	}
	c.elapsed = e
	c.changed()
	return true
}

// Shutdown stops playback before the program exits.
func (c *Controller) Shutdown() {
	c.QuitAfterPlaylist = false
	c.stopSong()
	c.playing = false
}

// --- internals -------------------------------------------------------------

func (c *Controller) changed() { c.listener.Changed() }

func (c *Controller) resetNext() {
	c.nextSong = -1
	c.nextName = ""
}

// advance starts the next song when the playlist is active but idle. At
// the end of the playlist it quits, repeats or stops.
func (c *Controller) advance() {
	for c.playing && c.track == nil && !c.opening {
		if c.startSong(false) {
			return
		}
		allow := c.allowRepeat
		c.stopList()
		if c.QuitAfterPlaylist {
			c.listener.EndOfPlaylistQuit()
			return
		}
		if !(allow && c.Repeat) {
			return
		}
		c.resetPlaylist(true)
		c.playing = true
	}
}

// startSong starts opening the next song: the one requested by PlayOne,
// the next one in the history (after Previous) or a new one from the
// playlist. It reports false when there is nothing to play.
func (c *Controller) startSong(wasPlaying bool) bool {
	song := c.oneMp3
	switch {
	case song == "" && c.single == singleOnly:
		c.single = singleNone
		return false
	case song == "":
		if !c.history.AtEnd() {
			song = c.history.Element()
			c.history.Next()
		} else if s := c.determineSong(); s != "" {
			song = s
			c.history.Add(s)
		}
		c.single = singleNone
		if song == "" {
			return false
		}
	case wasPlaying:
		c.single = singleThenList
	default:
		c.single = singleOnly
	}
	c.oneMp3 = ""
	c.open(song)
	return true
}

// open opens song in the background; opened continues on the UI goroutine.
func (c *Controller) open(song string) {
	c.opening = true
	c.gen++
	gen := c.gen
	openFn := c.backend.Opener(song)
	go func() {
		tr, err := openFn()
		c.post(func() { c.opened(gen, song, tr, err) })
	}()
}

// opened starts playing a freshly opened song, unless it was superseded
// while it was being opened.
func (c *Controller) opened(gen int, song string, tr Track, err error) {
	if gen != c.gen {
		if tr != nil {
			tr.Stop()
		}
		return
	}
	c.opening = false
	if err != nil {
		position := c.seekOnOpen
		c.seekOnOpen = 0
		c.status = StatusStop
		c.listener.Warning(errorText(err))
		if errors.Is(err, media.ErrDeviceOpen) {
			// Every other song would fail as well.
			c.stopList()
			c.Resume(song, position)
		} else {
			c.advance()
		}
		c.changed()
		return
	}
	c.track = tr
	c.songPath = song
	c.revision++
	c.info = tr.Info()
	c.haveInfo = true
	c.elapsed = 0
	c.status = StatusPlay
	if c.seekOnOpen > 0 {
		_ = tr.Seek(c.seekOnOpen)
		c.seekOnOpen = 0
		c.elapsed = tr.Elapsed()
	}
	quit := make(chan struct{})
	c.trackQuit = quit
	if c.paused {
		c.status = StatusPause
	} else {
		tr.Start()
	}
	c.listener.SongStarted(c.info)
	go func() {
		select {
		case <-tr.Done():
			c.post(func() { c.finished(gen) })
		case <-quit:
		}
	}()
	c.changed()
}

// finished is called when song gen has been played completely.
func (c *Controller) finished(gen int) {
	if gen != c.gen || c.track == nil {
		return
	}
	if err := c.stopSong(); errors.Is(err, media.ErrDeviceLost) {
		c.stopList()
		c.Resume(c.songPath, c.elapsed)
	} else if err == nil && c.playing && c.Repeat && c.RepeatTrack {
		c.open(c.songPath)
	} else {
		c.advance()
	}
	c.changed()
}

// errorText returns the classic message for err.
func errorText(err error) string {
	if errors.Is(err, media.ErrDeviceLost) {
		return err.Error()
	}
	var se media.SoundError
	if errors.As(err, &se) {
		return se.Error()
	}
	return "Couldn't open file."
}

// stopSong stops the current song (or cancels opening one) without
// changing the playlist state.
func (c *Controller) stopSong() error {
	c.gen++
	c.opening = false
	c.seekOnOpen = 0
	var err error
	if c.track == nil {
		c.allowRepeat = true
	} else {
		if c.trackQuit != nil {
			close(c.trackQuit)
			c.trackQuit = nil
		}
		c.elapsed = c.track.Elapsed()
		c.track.Stop()
		if err = c.track.Err(); err == nil {
			c.allowRepeat = true
		}
		c.track = nil
	}
	c.paused = false
	c.status = StatusStop
	c.listener.ClearMessage()
	if err != nil {
		c.listener.Warning(errorText(err))
	}
	return err
}

// stopList ends playing the playlist.
func (c *Controller) stopList() {
	c.playing = false
	c.single = singleNone
	c.allowRepeat = false
	c.resetPlaylist(true)
	c.resetNext()
}

// resetPlaylist marks songs and groups as unplayed and clears the history:
// the whole playlist when full is set, otherwise only the group being
// played.
func (c *Controller) resetPlaylist(full bool) {
	if !full {
		if c.playingGrp != nil {
			c.playingGrp.ResetSongs(false, false)
			c.playingGrp.SetPlaying(false)
			c.playingGrp.SetPlayed(false)
			c.playingGrp = nil
		}
	} else {
		if c.playingGrp != nil {
			c.playingGrp.SetPlaying(false)
		}
		c.playingGrp = nil
		c.groupWin = nil
		c.root.ResetSongs(true, false)
		c.root.ResetGroups()
	}
	c.history = History{}
}

// ForgetGroup is called before group g is deleted from the playlist.
func (c *Controller) ForgetGroup(g *playlist.Group) {
	if c.playingGrp == g || c.groupWin == g {
		c.resetPlaylist(true)
	}
}

func (c *Controller) randN(n int) int {
	if n <= 0 {
		return 0
	}
	return c.rnd.IntN(n)
}

// determineSong selects (and marks as played) the next song to play.
func (c *Controller) determineSong() string {
	total := c.root.UnplayedSongs(true)
	if total == 0 {
		return ""
	}
	var song *playlist.Item
	switch c.Mode {
	case PlaySongs:
		song = c.root.UnplayedSong(c.takeIndex(total, true), true, true)
	case PlayGroup, PlayGroups:
		if !c.selectGroup() {
			return ""
		}
		g := c.playingGrp
		song = g.UnplayedSong(c.takeIndex(g.UnplayedSongs(false), g.Shuffle), true, false)
	}
	if song == nil {
		return ""
	}
	if c.nextSong > -1 {
		c.updateNextName()
	}
	return song.Path
}

// takeIndex returns the index of the song to play among n unplayed songs:
// the precomputed nextSong if any, else a random one (or the first when
// random is false). It then precomputes nextSong for the following call,
// so the next song can be shown in advance.
func (c *Controller) takeIndex(n int, random bool) int {
	idx := c.nextSong
	if !random {
		c.nextSong = 0
		return max(idx, 0)
	}
	if idx < 0 {
		idx = c.randN(n)
	}
	c.nextSong = -1
	if n > 1 {
		c.nextSong = c.randN(n - 1)
	}
	return idx
}

// selectGroup makes sure playingGrp is a group with unplayed songs,
// advancing to the next group when the current one is exhausted. It
// reports false when the play mode has no group left to play.
func (c *Controller) selectGroup() bool {
	if c.Mode == PlayGroups && c.groupWin == nil {
		c.groupWin = c.listener.CurrentGroup()
		c.playingGrp = nil
	}
	for c.playingGrp == nil || c.playingGrp.UnplayedSongs(false) == 0 {
		if c.playingGrp != nil {
			c.playingGrp.SetPlaying(false)
			c.playingGrp = nil
			if c.Mode == PlayGroup {
				return false // only the current group is played
			}
		}
		if c.Mode == PlayGroup {
			c.playingGrp = c.listener.CurrentGroup()
			c.playingGrp.SetPlaying(true)
			continue
		}
		// PlayGroups: the next unplayed group below groupWin, in order or
		// at random when groupWin is shuffled.
		if c.groupWin == nil {
			c.resetPlaylist(false)
			return false
		}
		n := c.groupWin.UnplayedGroups(true)
		if n == 0 {
			c.resetPlaylist(false)
			return false
		}
		idx := 0
		if c.groupWin.Shuffle {
			idx = c.randN(n)
		}
		c.playingGrp = c.groupWin.UnplayedGroup(idx, true, true)
		if c.playingGrp != nil && c.playingGrp.UnplayedSongs(false) > 0 {
			c.playingGrp.SetPlayed(true)
			c.playingGrp.SetPlaying(true)
		} else {
			c.playingGrp = nil
			c.groupWin = nil
		}
	}
	return true
}

// updateNextName sets the name of the song that nextSong points at.
func (c *Controller) updateNextName() {
	var ns string
	switch c.Mode {
	case PlaySongs:
		if it := c.root.UnplayedSong(c.nextSong, false, true); it != nil {
			ns = it.Path
		}
	case PlayGroup, PlayGroups:
		if c.playingGrp == nil {
			return
		}
		if it := c.playingGrp.UnplayedSong(c.nextSong, false, false); it != nil {
			ns = it.Path
		} else if c.Mode == PlayGroups {
			ns = "??? (can't know yet)"
		}
	}
	if ns == "" {
		return
	}
	c.nextName = media.ChopPath(ns)
}
