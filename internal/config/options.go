// Package config holds mp3blaster's global options and reads them from the
// $HOME/.config/mp3blaster/mp3blasterrc configuration file and from the
// command line.
package config

import (
	"github.com/mingcheng/mp3blaster/internal/filemanager"
	"github.com/mingcheng/mp3blaster/internal/keys"
	"github.com/mingcheng/mp3blaster/internal/listview"
	"github.com/mingcheng/mp3blaster/internal/media"
	"github.com/mingcheng/mp3blaster/internal/playback"
)

// KeyBinding is a key rebinding read from the configuration file.
type KeyBinding struct {
	Cmd keys.Command
	Key keys.Key
}

// Options are the global settings (globalopts in the original).
type Options struct {
	NoMixer     bool
	FPL         int // frames per loop, 1..10 (controls the display refresh rate)
	Downsample  bool
	EightBits   bool
	SoundDevice string
	MixerDevice string
	AudioDriver string

	SortMode    filemanager.SortMode
	HideOthers  bool
	PlayMode    playback.PlayMode
	Repeat      bool
	WarnDelay   int
	SkipSeconds int
	PanSize     int
	Debug       bool
	DisplayMode int // playlist display mode: 0 path, 1 file name, 2 ID3
	Threads     int // read-ahead buffer (0..500 in steps of 50)

	AudioPatterns    []string
	PlaylistPatterns []string
	PlaylistDir      string
	CharsetTable     string

	ID3Names              bool
	SelectUnselectFirst   bool
	SelectRegex           bool
	SelectCaseInsensitive bool
	ScanMP3               bool
	Wrap                  bool

	// Colours maps "Color.X.fg"-style keywords (lower case, without the
	// "color." prefix) to colour names.
	Colours map[string]string
	Keys    []KeyBinding

	// Modern extensions.
	Theme     string // classic or modern
	NerdFont  bool
	Mouse     bool
	ListWidth listview.Width // width of the main window

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
		WarnDelay:             2,
		SkipSeconds:           10,
		PanSize:               5,
		DisplayMode:           1,
		Threads:               100,
		PlaylistDir:           media.HomeDir(),
		SelectCaseInsensitive: true,
		Wrap:                  true,
		Colours:               map[string]string{},
		Theme:                 "classic",
		Mouse:                 true,
		ListWidth:             listview.DefaultWidth,
		SaveState:             true,
		Volume:                100,
		Balance:               50,
	}
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
