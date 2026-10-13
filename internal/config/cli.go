package config

import (
	"fmt"
	"io"

	"github.com/spf13/pflag"

	"github.com/mingcheng/mp3blstr/internal/listview"
	"github.com/mingcheng/mp3blstr/internal/playback"
)

// CLI holds the command line options. Settings that also exist in the
// configuration file are applied on top of it with Options.ApplyCLI.
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

	Theme         string
	NerdFont      bool
	NoMouse       bool
	ReuseInstance bool
	Enqueue       bool
	ListWidth     string

	Files []string // songs to play

	set map[string]bool // long names of the options given
}

// IsSet reports whether the long option name was given.
func (c *CLI) IsSet(name string) bool { return c.set[name] }

// ParseArgs parses the command line (without the program name).
func ParseArgs(args []string) (*CLI, error) {
	c := &CLI{set: map[string]bool{}}
	fs := pflag.NewFlagSet("mp3blstr", pflag.ContinueOnError)
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
	fs.BoolVar(&c.ReuseInstance, "reuse-instance", false, "")
	fs.BoolVar(&c.Enqueue, "enqueue", false, "")
	fs.StringVar(&c.ListWidth, "list-width", "", "")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	fs.Visit(func(f *pflag.Flag) { c.set[f.Name] = true })
	c.Files = fs.Args()
	if c.Enqueue && len(c.Files) == 0 {
		return nil, fmt.Errorf("--enqueue requires audio files")
	}
	if (c.ReuseInstance || c.Enqueue) && c.Chroot != "" {
		return nil, fmt.Errorf("desktop file-open options cannot be combined with --chroot")
	}
	if (c.List != "" || c.AutoList != "") && len(c.Files) > 0 {
		return nil, fmt.Errorf("files can't be combined with --list/--autolist")
	}
	return c, nil
}

// ApplyCLI applies the command line options on top of the settings read
// from the configuration file. Boolean flags can only switch options on.
func (o *Options) ApplyCLI(c *CLI) error {
	o.NoMixer = o.NoMixer || c.NoMixer
	o.Downsample = o.Downsample || c.Downsample
	o.EightBits = o.EightBits || c.EightBits
	o.Repeat = o.Repeat || c.Repeat
	o.Debug = o.Debug || c.Debug
	o.NerdFont = o.NerdFont || c.NerdFont
	if c.NoMouse {
		o.Mouse = false
	}
	if c.SoundDevice != "" {
		o.SoundDevice = c.SoundDevice
	}
	if c.MixerDevice != "" {
		o.MixerDevice = c.MixerDevice
	}
	if c.PlayMode != "" {
		m, err := playback.ParsePlayMode(c.PlayMode)
		if err != nil {
			return err
		}
		o.PlayMode = m
	}
	if c.IsSet("runframes") && !o.SetFPL(c.RunFrames) {
		return fmt.Errorf("invalid --runframes %d (1..10)", c.RunFrames)
	}
	if c.IsSet("threads") && !o.SetThreads(c.Threads) {
		return fmt.Errorf("invalid --threads %d (0..500 in steps of 50)", c.Threads)
	}
	if c.Theme != "" {
		t, err := ParseTheme(c.Theme)
		if err != nil {
			return err
		}
		o.Theme = t
	}
	if c.IsSet("list-width") {
		w, err := listview.ParseWidth(c.ListWidth)
		if err != nil {
			return err
		}
		o.ListWidth = w
	}
	return nil
}

// Usage prints the help text of the original program (plus the options
// that are specific to this implementation), using the formatted version label.
func Usage(w io.Writer, version string) {
	fmt.Fprintf(w, "mp3blstr %s is a modern re-implementation of the classic mp3blaster, authored by mingcheng<mingcheng@outlook.com>.\n", version)
	fmt.Fprint(w, `Usage:
	mp3blstr [options]
	mp3blstr [options] [file1 ...]
		Play one or more mp3's
	mp3blstr [options] --list/-l <playlist.lst>
		Load a playlist but don't start playing.
	mp3blstr [options] --autolist/-a <playlist.lst>
		Load a playlist and start playing.

Options:
	--downsample/-2: Downsample (44->22Khz etc)
	--8bits/-8: 8bit audio
	--config-file/-c=file: Use other config file than the default
		$HOME/.config/mp3blstr/config.yaml
	--debug/-d: Log debug-info in $HOME/.config/mp3blstr/debug.log.
	--status-file/-f=file: Keep info on the mp3s being played, in the
		specified file.
	--help/-h: This help screen.
	--mixer-device/-m: Mixer device to use (accepted for compatibility)
	--no-mixer/-n: Don't start the built-in mixer.
	--chroot/-o=<rootdir>: Set <rootdir> as mp3blstr's root dir.
		This affects *ALL* file operations in mp3blstr!!(including
		playlist reading&writing!) Note that only users with uid 0
		can use this option (Unix only).
	--playmode/-p={onegroup,allgroups,allrandom}
		Default playing mode is resp. Play first group only, Play
		all groups, Play all songs in random order.
	--dont-quit/-q: Don't quit after playing mp3[s] (only makes sense
		in combination with --autolist or files from command-line)
	--repeat/-R: Repeat playlist.
	--runframes/-r=<number>: Legacy scan-animation refresh setting.
		Range: 1 to 10 (default=5). Playback time updates once a
		second; idle/paused players do not poll the display.
	--sound-device/-s=<device>: Device to use to output sound
		(accepted for compatibility; the system default is used).
	--threads/-t=<amount>: Size of the read-ahead buffer in frames.
		Range is 0..500 in increments of 50 only.
	--version,v: Display version number.

Extensions:
	--reuse-instance: On Linux, send files to an existing player if available.
	--enqueue: Add files without starting playback (use with --reuse-instance).
	--theme={classic,modern}: Colour theme (default: classic).
	--nerd-font: Use Nerd Font icons in the interface.
	--no-mouse: Disable mouse support.
	--list-width=<N%|columns>: Width of the file/playlist window, as a
		percentage of the terminal (default 75%, at least 60 columns)
		or in columns; 0 = terminal width - 12 like the original.
		The remaining columns show the song information panel.
`)
}
