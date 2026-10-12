package config

import (
	"fmt"
	"io"

	"github.com/spf13/pflag"
)

// CLI holds the command line options.
type CLI struct {
	Downsample  bool
	EightBits   bool
	AutoList    string
	List        string
	ConfigFile  string
	Debug       bool
	StatusFile  string
	Help        bool
	MixerDevice string
	NoMixer     bool
	Chroot      string
	PlayMode    string
	DontQuit    bool
	Repeat      bool
	RunFrames   int
	SoundDevice string
	Threads     int
	Version     bool

	Theme     string
	NerdFont  bool
	NoMouse   bool
	ListWidth string

	Files []string

	set map[string]bool
}

// IsSet reports whether the long option name was given.
func (c *CLI) IsSet(name string) bool { return c.set[name] }

// ParseArgs parses the command line (without the program name).
func ParseArgs(args []string) (*CLI, error) {
	c := &CLI{set: map[string]bool{}}
	fs := pflag.NewFlagSet("mp3blaster", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.SortFlags = false
	fs.BoolVarP(&c.Downsample, "downsample", "2", false, "")
	fs.BoolVarP(&c.EightBits, "8bits", "8", false, "")
	fs.StringVarP(&c.AutoList, "autolist", "a", "", "")
	fs.StringVarP(&c.ConfigFile, "config-file", "c", "", "")
	fs.BoolVarP(&c.Debug, "debug", "d", false, "")
	fs.StringVarP(&c.StatusFile, "status-file", "f", "", "")
	fs.BoolVarP(&c.Help, "help", "h", false, "")
	fs.StringVarP(&c.List, "list", "l", "", "")
	fs.StringVarP(&c.MixerDevice, "mixer-device", "m", "", "")
	fs.BoolVarP(&c.NoMixer, "no-mixer", "n", false, "")
	fs.StringVarP(&c.Chroot, "chroot", "o", "", "")
	fs.StringVarP(&c.PlayMode, "playmode", "p", "", "")
	fs.BoolVarP(&c.DontQuit, "dont-quit", "q", false, "")
	fs.BoolVarP(&c.Repeat, "repeat", "R", false, "")
	fs.IntVarP(&c.RunFrames, "runframes", "r", 5, "")
	fs.StringVarP(&c.SoundDevice, "sound-device", "s", "", "")
	fs.IntVarP(&c.Threads, "threads", "t", 100, "")
	fs.BoolVarP(&c.Version, "version", "v", false, "")
	fs.StringVar(&c.Theme, "theme", "", "")
	fs.BoolVar(&c.NerdFont, "nerd-font", false, "")
	fs.BoolVar(&c.NoMouse, "no-mouse", false, "")
	fs.StringVar(&c.ListWidth, "list-width", "", "")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	fs.Visit(func(f *pflag.Flag) { c.set[f.Name] = true })
	c.Files = fs.Args()
	if (c.List != "" || c.AutoList != "") && len(c.Files) > 0 {
		return nil, fmt.Errorf("files can't be combined with --list/--autolist")
	}
	return c, nil
}

// Usage prints the help text of the original program (plus the options
// that are specific to this implementation).
func Usage(w io.Writer, version string) {
	fmt.Fprintf(w, "Mp3blaster v%s (C)1997 - 2009 Bram Avontuur, Go port.\n", version)
	fmt.Fprint(w, `Usage:
	mp3blaster [options]
	mp3blaster [options] [file1 ...]
		Play one or more mp3's
	mp3blaster [options] --list/-l <playlist.lst>
		Load a playlist but don't start playing.
	mp3blaster [options] --autolist/-a <playlist.lst>
		Load a playlist and start playing.

Options:
	--downsample/-2: Downsample (44->22Khz etc)
	--8bits/-8: 8bit audio
	--config-file/-c=file: Use other config file than the default
		$HOME/.config/mp3blaster/mp3blasterrc (falls back to
		~/.mp3blasterrc)
	--debug/-d: Log debug-info in $HOME/.config/mp3blaster/debug.log.
	--status-file/-f=file: Keep info on the mp3s being played, in the
		specified file.
	--help/-h: This help screen.
	--mixer-device/-m: Mixer device to use (accepted for compatibility)
	--no-mixer/-n: Don't start the built-in mixer.
	--chroot/-o=<rootdir>: Set <rootdir> as mp3blaster's root dir.
		This affects *ALL* file operations in mp3blaster!!(including
		playlist reading&writing!) Note that only users with uid 0
		can use this option (Unix only).
	--playmode/-p={onegroup,allgroups,allrandom}
		Default playing mode is resp. Play first group only, Play
		all groups, Play all songs in random order.
	--dont-quit/-q: Don't quit after playing mp3[s] (only makes sense
		in combination with --autolist or files from command-line)
	--repeat/-R: Repeat playlist.
	--runframes/-r=<number>: Number of frames to decode in one loop.
		Range: 1 to 10 (default=5). A low value means that the
		interface (while playing) reacts faster.
	--sound-device/-s=<device>: Device to use to output sound
		(accepted for compatibility; the system default is used).
	--threads/-t=<amount>: Size of the read-ahead buffer in frames.
		Range is 0..500 in increments of 50 only.
	--version,v: Display version number.

Extensions:
	--theme={classic,modern}: Colour theme (default: classic).
	--nerd-font: Use Nerd Font icons in the interface.
	--no-mouse: Disable mouse support.
	--list-width=<N%|columns>: Width of the file/playlist window, as a
		percentage of the terminal (default 75%, at least 60 columns)
		or in columns; 0 = terminal width - 12 like the original.
		The remaining columns show the song information panel.
`)
}
