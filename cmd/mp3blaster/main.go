// Command mp3blaster is a Go re-implementation of the classic ncurses audio
// player mp3blaster by Bram Avontuur.
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mingcheng/mp3blaster/internal/app"
	"github.com/mingcheng/mp3blaster/internal/audio"
	"github.com/mingcheng/mp3blaster/internal/config"
	"github.com/mingcheng/mp3blaster/internal/keys"
	"github.com/mingcheng/mp3blaster/internal/listview"
	"github.com/mingcheng/mp3blaster/internal/media"
	"github.com/mingcheng/mp3blaster/internal/playback"
	"github.com/mingcheng/mp3blaster/internal/ui"
	"github.com/mingcheng/mp3blaster/internal/ui/theme"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "3.2.6-go"

func main() { os.Exit(run()) }

func usage() int {
	config.Usage(os.Stderr, version)
	return 1
}

func run() int {
	cli, err := config.ParseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return usage()
	}
	if cli.Help {
		return usage()
	}
	if cli.Version {
		fmt.Printf("%s version %s - https://github.com/mingcheng/mp3blaster\n", filepath.Base(os.Args[0]), version)
		fmt.Println("Supported audio formats: MP3 WAV OGG/Vorbis FLAC HTTP-streams")
		fmt.Println("Supported audio output drivers: native (CoreAudio/WASAPI/PulseAudio/ALSA via oto, no cgo)")
		fmt.Println("Build features: mouse, themes, nerd-font, read-ahead buffering")
		return 0
	}

	// Best effort: the directory only stores optional data (logs, caches).
	_, _ = config.EnsureDir()

	opts := config.Default()
	if err := opts.LoadFile(cli.ConfigFile); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if code, ok := applyCLI(cli, opts); !ok {
		return code
	}
	// Only changes made during this session are written back on exit, so
	// one-off command line options don't end up in the configuration file.
	persist := opts.SaveState && cli.Chroot == ""
	initialState := opts.StateSettings()

	logger := newLogger(opts.Debug)
	if opts.CharsetTable != "" {
		rec, err := media.LoadRecodeTable(media.ExpandPath(opts.CharsetTable))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		media.SetRecoder(rec)
	}
	matcher, err := media.NewMatcher(opts.AudioPatterns, opts.PlaylistPatterns)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Invalid AudiofileMatching/PlaylistMatching:", err)
		return 1
	}
	for _, f := range cli.Files {
		if !matcher.IsAudio(f) {
			fmt.Fprintf(os.Stderr, "%s is not a valid audio file!\n", f)
			return usage()
		}
	}
	if cli.Chroot != "" {
		if err := chroot(cli.Chroot); err != nil {
			fmt.Fprintf(os.Stderr, "Could not chroot to %s! (%v)\n", cli.Chroot, err)
			return 1
		}
	}

	km := keys.Default()
	for _, kb := range opts.Keys {
		km.Bind(kb.Cmd, kb.Key)
	}

	gains := audio.NewMixer()
	var mixer *audio.Mixer
	if !opts.NoMixer {
		mixer = gains
		gains.SetDevice(audio.DevVolume, opts.Volume)
		gains.SetDevice(audio.DevBalance, opts.Balance)
	}
	backend := &backend{out: audio.NewOutput(gains), opts: opts}

	th := theme.ByName(opts.Theme)
	th.ApplyColours(opts.Colours)
	u := ui.New(ui.Options{Theme: th, NerdFont: opts.NerdFont, Mouse: opts.Mouse, ListWidth: opts.ListWidth})

	cwd, err := os.Getwd()
	if err != nil {
		cwd = media.HomeDir()
	}
	if cli.Chroot == "" && opts.LastDir != "" {
		if st, err := os.Stat(opts.LastDir); err == nil && st.IsDir() {
			cwd = opts.LastDir
		}
	}
	statusFile := cli.StatusFile
	if statusFile != "" {
		if abs, err := filepath.Abs(statusFile); err == nil {
			statusFile = abs
		}
	}
	a := app.New(opts, km, matcher, mixer, backend, u, cwd, statusFile, logger)
	u.Attach(a)

	switch {
	case len(cli.Files) > 0:
		a.AddFiles(cli.Files)
		a.Player.Play()
		a.Player.QuitAfterPlaylist = !cli.DontQuit
	case cli.List != "":
		a.LoadPlaylist(cli.List)
	case cli.AutoList != "":
		if a.LoadPlaylist(cli.AutoList) {
			a.Player.Play()
		}
	case persist:
		a.RestoreSession(config.SessionPath())
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigs
		u.Post(a.Quit)
	}()

	err = u.Run()
	a.CaptureState()
	a.Player.Shutdown()
	if persist {
		saveState(a, opts, initialState)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// saveState writes the playlist and the settings changed during this
// session, so the next start continues where this one ended.
func saveState(a *app.App, opts *config.Options, initial []config.Setting) {
	if _, err := config.EnsureDir(); err == nil {
		if err := a.WriteSession(config.SessionPath()); err != nil {
			fmt.Fprintln(os.Stderr, "Could not save the playlist:", err)
		}
	}
	path := opts.Path
	if path == "" {
		path = config.DefaultPath()
	}
	if err := config.SaveSettings(path, config.ChangedSettings(initial, opts.StateSettings())); err != nil {
		fmt.Fprintln(os.Stderr, "Could not save the settings:", err)
	}
}

// applyCLI applies command line options on top of the configuration file.
func applyCLI(cli *config.CLI, opts *config.Options) (int, bool) {
	opts.NoMixer = opts.NoMixer || cli.NoMixer
	opts.Downsample = opts.Downsample || cli.Downsample
	opts.EightBits = opts.EightBits || cli.EightBits
	opts.Repeat = opts.Repeat || cli.Repeat
	opts.Debug = opts.Debug || cli.Debug
	if cli.SoundDevice != "" {
		opts.SoundDevice = cli.SoundDevice
	}
	if cli.MixerDevice != "" {
		opts.MixerDevice = cli.MixerDevice
	}
	if cli.PlayMode != "" {
		m, err := playback.ParsePlayMode(cli.PlayMode)
		if err != nil {
			return usage(), false
		}
		opts.PlayMode = m
	}
	if cli.IsSet("runframes") && !opts.SetFPL(cli.RunFrames) {
		return usage(), false
	}
	if cli.IsSet("threads") && !opts.SetThreads(cli.Threads) {
		return usage(), false
	}
	if cli.Theme != "" {
		t := strings.ToLower(cli.Theme)
		if t != "classic" && t != "modern" {
			return usage(), false
		}
		opts.Theme = t
	}
	if cli.NerdFont || os.Getenv("MP3BLASTER_NERDFONT") == "1" {
		opts.NerdFont = true
	}
	if cli.IsSet("list-width") {
		w, err := listview.ParseWidth(cli.ListWidth)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return usage(), false
		}
		opts.ListWidth = w
	}
	if cli.NoMouse {
		opts.Mouse = false
	}
	return 0, true
}

// newLogger logs to $HOME/.config/mp3blaster/debug.log when debugging is
// enabled.
func newLogger(debug bool) *slog.Logger {
	if !debug {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	dir, err := config.EnsureDir()
	if err != nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	f, err := os.OpenFile(filepath.Join(dir, "debug.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	l := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug}))
	l.Debug("Debugging of mp3blaster started.")
	return l
}

// backend adapts the audio output to the playback controller.
type backend struct {
	out  *audio.Output
	opts *config.Options
}

func (b *backend) Opener(path string) func() (playback.Track, error) {
	o := audio.Options{
		PrefetchFrames: b.opts.Threads,
		Downsample:     b.opts.Downsample,
		EightBit:       b.opts.EightBits,
	}
	return func() (playback.Track, error) {
		p, err := b.out.Open(path, o)
		if err != nil {
			return nil, err
		}
		return p, nil
	}
}
