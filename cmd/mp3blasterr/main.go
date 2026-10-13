// Command mp3blasterr (mp3blaster reborn) is a Go re-implementation of the
// classic ncurses audio player mp3blaster by Bram Avontuur.
//
// This package only wires the layers together; the behaviour lives in the
// packages below internal/.
package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mingcheng/mp3blasterr/internal/app"
	"github.com/mingcheng/mp3blasterr/internal/audio"
	"github.com/mingcheng/mp3blasterr/internal/config"
	"github.com/mingcheng/mp3blasterr/internal/keys"
	"github.com/mingcheng/mp3blasterr/internal/media"
	"github.com/mingcheng/mp3blasterr/internal/mediakeys"
	"github.com/mingcheng/mp3blasterr/internal/ui"
	"github.com/mingcheng/mp3blasterr/internal/ui/theme"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "3.2.6-go"

// Exit codes.
const (
	exitOK      = 0
	exitFailure = 1
)

func main() { os.Exit(run(os.Args[1:])) }

// run starts the program with the command line arguments args and returns
// the exit code.
func run(args []string) int {
	cli, err := config.ParseArgs(args)
	if err != nil {
		return usage(err)
	}
	switch {
	case cli.Help:
		return usage(nil)
	case cli.Version:
		printVersion()
		return exitOK
	}
	if os.Getenv("MP3BLASTERR_NERDFONT") == "1" {
		cli.NerdFont = true
	}

	// Best effort: the directory only stores optional data (logs, session).
	_, _ = config.EnsureDir()

	opts := config.Default()
	if err := opts.LoadFile(cli.ConfigFile); err != nil {
		return fail(err)
	}
	if err := opts.ApplyCLI(cli); err != nil {
		return usage(err)
	}
	// Only changes made during this session are written back on exit, so
	// one-off command line options don't end up in the configuration file.
	persist := opts.SaveState && cli.Chroot == ""
	initialState := opts.StateSettings()
	// Opened before a chroot, which hides the configuration directory.
	logger := newLogger(opts.Debug)

	matcher, err := setupMedia(opts)
	if err != nil {
		return fail(err)
	}
	for _, f := range cli.Files {
		if !matcher.IsAudio(f) {
			return usage(fmt.Errorf("%s is not a valid audio file", f))
		}
	}
	if cli.Chroot != "" {
		if err := chroot(cli.Chroot); err != nil {
			return fail(fmt.Errorf("could not chroot to %s: %w", cli.Chroot, err))
		}
	}

	km := keys.Default()
	for _, kb := range opts.Keys {
		km.Bind(kb.Cmd, kb.Key)
	}
	mixer, gains := newMixer(opts)

	th := theme.ByName(opts.Theme)
	th.ApplyColours(opts.Colours)
	u := ui.New(ui.Options{Theme: th, NerdFont: opts.NerdFont, Mouse: opts.Mouse, ListWidth: opts.ListWidth})

	a := app.New(app.Deps{
		Options:    opts,
		Keys:       km,
		Matcher:    matcher,
		Mixer:      mixer,
		FadeMixer:  gains,
		Backend:    &backend{out: audio.NewOutput(gains), opts: opts},
		View:       u,
		StartDir:   startDir(opts, cli.Chroot != ""),
		StatusFile: absPath(cli.StatusFile),
		Logger:     logger,
	})
	u.Attach(a)
	if opts.MediaKeys {
		controls, err := mediakeys.Start(func(action mediakeys.Action) {
			u.Post(func() { handleMediaAction(a.Player, action) })
		})
		if err != nil {
			logger.Warn("media keys unavailable", "err", err)
		} else {
			defer controls.Close()
			a.OnPlaybackChange = func() {
				status := "Stopped"
				if a.Player.Playing() {
					status = "Playing"
					if a.Player.Paused() {
						status = "Paused"
					}
				}
				controls.Update(status)
			}
		}
	}
	start(a, cli, persist)

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
		return fail(err)
	}
	return exitOK
}

type mediaPlayer interface {
	Play()
	Playing() bool
	Paused() bool
	Next()
	Previous()
	Stop()
}

func handleMediaAction(p mediaPlayer, action mediakeys.Action) {
	switch action {
	case mediakeys.PlayPause:
		p.Play()
	case mediakeys.Play:
		if !p.Playing() || p.Paused() {
			p.Play()
		}
	case mediakeys.Pause:
		if p.Playing() && !p.Paused() {
			p.Play()
		}
	case mediakeys.Next:
		p.Next()
	case mediakeys.Previous:
		p.Previous()
	case mediakeys.Stop:
		p.Stop()
	}
}

// usage prints err (if any) and the help text, and returns the exit code.
func usage(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	config.Usage(os.Stderr, version)
	return exitFailure
}

// fail prints err and returns the exit code.
func fail(err error) int {
	fmt.Fprintln(os.Stderr, err)
	return exitFailure
}

func printVersion() {
	fmt.Printf("%s version %s - https://github.com/mingcheng/mp3blasterr\n", filepath.Base(os.Args[0]), version)
	fmt.Println("Supported audio formats: MP3 WAV OGG/Vorbis FLAC HTTP-streams")
	fmt.Println("Supported audio output drivers: native (CoreAudio/WASAPI/PulseAudio/ALSA via oto, no cgo)")
	fmt.Println("Build features: mouse, themes, nerd-font, read-ahead buffering")
}

// setupMedia installs the tag recoder (CharsetTable) and returns the
// matcher recognising audio files and playlists.
func setupMedia(opts *config.Options) (*media.Matcher, error) {
	media.SetGuessLegacyTags(opts.GuessLegacyTags)
	profile := fmt.Sprintf("%t:%s", opts.GuessLegacyTags, opts.CharsetTable)
	if opts.CharsetTable != "" {
		path := media.ExpandPath(opts.CharsetTable)
		rec, err := media.LoadRecodeTable(path)
		if err != nil {
			return nil, err
		}
		media.SetRecoder(rec)
		if data, err := os.ReadFile(path); err == nil {
			profile += fmt.Sprintf(":%x", sha256.Sum256(data))
		}
	}
	cacheDir := ""
	if opts.MetadataCache {
		cacheDir = filepath.Join(config.Dir(), "cache")
	}
	media.ConfigureMetadataCache(cacheDir, profile)
	m, err := media.NewMatcher(opts.AudioPatterns, opts.PlaylistPatterns)
	if err != nil {
		return nil, fmt.Errorf("invalid AudiofileMatching/PlaylistMatching: %w", err)
	}
	return m, nil
}

// newMixer returns the mixer shown in the interface (nil with --no-mixer)
// and the one applied to the audio output. Without the mixer interface the
// output plays at full volume.
func newMixer(opts *config.Options) (shown, gains *audio.Mixer) {
	gains = audio.NewMixer()
	if opts.NoMixer {
		return nil, gains
	}
	gains.SetDevice(audio.DevVolume, opts.Volume)
	gains.SetDevice(audio.DevBalance, opts.Balance)
	return gains, gains
}

// startDir is the initial directory of the file manager: the directory of
// the previous session if it still exists, else the working directory.
func startDir(opts *config.Options, chrooted bool) string {
	if !chrooted && opts.LastDir != "" {
		if st, err := os.Stat(opts.LastDir); err == nil && st.IsDir() {
			return opts.LastDir
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return media.HomeDir()
}

// absPath makes p absolute ("" stays empty).
func absPath(p string) string {
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// start loads what the command line asks for, or else restores the
// previous session.
func start(a *app.App, cli *config.CLI, persist bool) {
	switch {
	case len(cli.Files) > 0:
		a.AddFiles(cli.Files)
		a.Player.QuitAfterPlaylist = !cli.DontQuit
		a.Player.Play()
	case cli.List != "":
		a.LoadPlaylist(cli.List)
	case cli.AutoList != "":
		if a.LoadPlaylist(cli.AutoList) {
			a.Player.Play()
		}
	case persist:
		a.RestoreSession(config.SessionPath())
	}
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

// newLogger logs to $HOME/.config/mp3blasterr/debug.log when debugging is
// enabled, and discards everything otherwise.
func newLogger(debug bool) *slog.Logger {
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	if !debug {
		return discard
	}
	dir, err := config.EnsureDir()
	if err != nil {
		return discard
	}
	f, err := os.OpenFile(filepath.Join(dir, "debug.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return discard
	}
	l := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug}))
	l.Debug("Debugging of mp3blasterr started.")
	return l
}
