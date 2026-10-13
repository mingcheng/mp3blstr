// Package config holds mp3blstr's global options and reads them from the
// $HOME/.config/mp3blstr/config.yaml configuration file and from the
// command line.
package config

import (
	"fmt"
	"slices"
	"strings"

	"github.com/adrg/xdg"

	"github.com/mingcheng/mp3blstr/internal/filemanager"
	"github.com/mingcheng/mp3blstr/internal/keys"
	"github.com/mingcheng/mp3blstr/internal/listview"
	"github.com/mingcheng/mp3blstr/internal/media"
	"github.com/mingcheng/mp3blstr/internal/playback"
	"github.com/mingcheng/mp3blstr/internal/playlist"
)

// KeyBinding is a key rebinding read from the configuration file.
type KeyBinding struct {
	Cmd keys.Command
	Key keys.Key
}

// Options are the global settings (globalopts in the original).
type Options struct {
	NoMixer     bool
	FPL         int // legacy frames per loop, 1..10 (controls scan animation)
	Downsample  bool
	EightBits   bool
	SoundDevice string
	MixerDevice string

	SortMode            filemanager.SortMode
	HideOthers          bool
	PlayMode            playback.PlayMode
	MediaKeys           bool
	PauseOnSleep        bool
	PauseOnOutputChange bool
	UseTrash            bool
	Repeat              bool
	FadeOut             bool
	FadeSeconds         int // seconds of end-of-track fade, set in config only
	SkipSeconds         int // seconds skipped by forward/rewind
	PanSize             int // columns panned by left/right
	Debug               bool
	DisplayMode         playlist.DisplayMode
	FileDisplayMode     filemanager.DisplayMode
	Threads             int // read-ahead buffer (0..500 in steps of 50)

	AudioPatterns    []string
	PlaylistPatterns []string
	PlaylistDir      string
	CharsetTable     string

	ID3Names              bool
	GuessLegacyTags       bool
	MetadataCache         bool
	SelectUnselectFirst   bool
	SelectRegex           bool
	SelectCaseInsensitive bool
	ScanMP3               bool
	Wrap                  bool

	// Colours maps the ui.colors settings, as lower case "popupinput.fg"
	// style names without underscores, to colour names.
	Colours map[string]string
	Keys    []KeyBinding

	// Modern extensions.
	Theme       string // one of Themes
	NerdFont    bool
	Mouse       bool
	ListWidth   listview.Width // width of the main window
	ConfirmQuit bool           // ask before leaving the program

	// State saved in the configuration file on exit (see StateSettings).
	SaveState    bool
	Volume       int // mixer volume, 0..100
	Balance      int // mixer balance, 0..100 (50 = centre)
	LastDir      string
	LastSong     string
	LastPosition int // seconds into LastSong

	// Path is the configuration file that was read (or that will be
	// created when the state is saved).
	Path string
}

// Default returns the stock settings.
func Default() *Options {
	return &Options{
		FPL:                   5,
		SortMode:              filemanager.SortAlpha,
		PlayMode:              playback.PlayGroups,
		MediaKeys:             true,
		PauseOnSleep:          true,
		PauseOnOutputChange:   true,
		FadeSeconds:           5,
		SkipSeconds:           10,
		PanSize:               5,
		DisplayMode:           playlist.DisplayFileName,
		FileDisplayMode:       filemanager.DisplaySize,
		Threads:               100,
		PlaylistDir:           defaultPlaylistDir(),
		MetadataCache:         true,
		SelectCaseInsensitive: true,
		Wrap:                  true,
		Colours:               map[string]string{},
		Theme:                 Themes[0],
		Mouse:                 true,
		ListWidth:             listview.DefaultWidth,
		ConfirmQuit:           true,
		SaveState:             true,
		Volume:                100,
		Balance:               50,
	}
}

// defaultPlaylistDir returns the user's music folder (XDG_MUSIC_DIR, ~/Music
// on macOS, the Music known folder on Windows), or the home directory.
func defaultPlaylistDir() string {
	if d := xdg.UserDirs.Music; d != "" {
		return d
	}
	return media.HomeDir()
}

// Themes lists the built-in colour themes in switching order.
var Themes = []string{"classic", "modern"}

// ParseTheme validates a theme name (case-insensitive) and returns it in
// its canonical form.
func ParseTheme(name string) (string, error) {
	t := strings.ToLower(strings.TrimSpace(name))
	if !slices.Contains(Themes, t) {
		return "", fmt.Errorf("unknown theme %q (%s)", name, strings.Join(Themes, ", "))
	}
	return t, nil
}

// NextTheme returns the theme following name in Themes (wrapping around).
func NextTheme(name string) string {
	i := slices.Index(Themes, strings.ToLower(name))
	return Themes[(i+1)%len(Themes)]
}

// SetThreads validates and sets the read-ahead buffer size.
func (o *Options) SetThreads(t int) bool {
	if t < 0 || t > 500 || t%50 != 0 {
		return false
	}
	o.Threads = t
	return true
}

// SetFPL validates and sets the frames-per-loop value.
func (o *Options) SetFPL(n int) bool {
	if n < 1 || n > 10 {
		return false
	}
	o.FPL = n
	return true
}

// CycleThreads advances the thread count like the F10 key does.
func (o *Options) CycleThreads() {
	o.Threads += 50
	if o.Threads > 500 {
		o.Threads = 0
	}
}
