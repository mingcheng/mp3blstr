// Command mp3blstr (mp3blaster reborn) is a Go re-implementation of the
// classic ncurses audio player mp3blaster by Bram Avontuur.
//
// This package only wires the layers together; the behaviour lives in the
// packages below internal/.
package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mingcheng/mp3blstr/internal/app"
	"github.com/mingcheng/mp3blstr/internal/audio"
	"github.com/mingcheng/mp3blstr/internal/config"
	"github.com/mingcheng/mp3blstr/internal/keys"
	"github.com/mingcheng/mp3blstr/internal/media"
	"github.com/mingcheng/mp3blstr/internal/mediakeys"
	"github.com/mingcheng/mp3blstr/internal/playback"
	"github.com/mingcheng/mp3blstr/internal/ui"
	"github.com/mingcheng/mp3blstr/internal/ui/theme"
)

// version and buildDate are set at build time with -ldflags -X.
var version = "dev"
var buildDate string

// formatVersion returns a human-readable version string based on the
// provided version and build date.
func formatVersion(version, date string) string {
	label := "dev"
	if version != "" && version != "dev" {
		label = "v" + strings.TrimPrefix(version, "v")
	}
	if date != "" {
		label += "(" + date + ")"
	}
	return label
}

// Exit codes.
const (
	exitOK      = 0
	exitFailure = 1
)

func main() {
	// macOS delivers media keys through the main thread's run loop.
	mediakeys.Main(func() int { return run(os.Args[1:]) })
}

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
	if os.Getenv("MP3BLSTR_NERDFONT") == "1" {
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
	if len(cli.Files) > 0 && (cli.ReuseInstance || cli.Enqueue) {
		cli.Files, err = matcher.AudioPaths(cli.Files)
		if err != nil {
			return usage(err)
		}
		if cli.ReuseInstance {
			forwarded, err := mediakeys.ForwardFiles(cli.Files, cli.Enqueue)
			if err != nil {
				return fail(err)
			}
			if forwarded {
				return exitOK
			}
		}
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
		Version:    formatVersion(version, buildDate),
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
		controls, err := mediakeys.Start(func(command mediakeys.Command) {
			u.Post(func() {
				err := handleAppMediaCommand(a, command)
				if command.Request != nil {
					command.Request.Result <- err
				}
				if err != nil {
					logger.Warn("media command rejected", "err", err)
					a.Warning(err.Error())
				}
			})
		}, func(err error) { logger.Warn("Now Playing artwork unavailable", "err", err) })
		if err != nil {
			logger.Warn("media keys unavailable", "err", err)
			a.Warning(fmt.Sprintf("System media controls unavailable: %v", err))
		} else {
			defer controls.Close()
			a.OnPlaybackChange = func() {
				snapshot := playbackSnapshot(a.Player)
				snapshot.Shuffle = a.Shuffle()
				snapshot.Volume = float64(a.Volume()) / 100
				controls.Update(snapshot)
			}
			a.OnPlaybackChange()
		}
	}
	start(a, cli, persist)
	if mediakeys.SupportsInterruptions && (opts.PauseOnSleep || opts.PauseOnOutputChange) {
		monitor, err := mediakeys.WatchInterruptions(opts.PauseOnSleep, opts.PauseOnOutputChange, func(event mediakeys.Interruption) {
			u.Post(func() {
				if !a.Player.Playing() {
					return
				}
				a.Player.Pause()
				message := "Paused before system sleep; press Play to continue."
				if event == mediakeys.OutputChanged {
					message = "Audio output changed; press Play to continue."
				}
				a.SetMessage(message)
			})
		})
		if err != nil {
			logger.Warn("system interruption monitoring unavailable", "err", err)
			a.Warning(fmt.Sprintf("Sleep/output auto-pause unavailable: %v", err))
		} else {
			defer func() {
				if err := monitor.Close(); err != nil {
					logger.Warn("closing system interruption monitor", "err", err)
				}
			}()
		}
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
		return fail(err)
	}
	return exitOK
}

type mediaPlayer interface {
	Play()
	Pause()
	Playing() bool
	Paused() bool
	Next()
	Previous()
	Stop()
	TrackID() int
	CanSeek() bool
	SongInfo() (string, media.SongInfo, bool)
	SeekTo(int)
}

func handleAppMediaCommand(a *app.App, command mediakeys.Command) error {
	switch command.Action {
	case mediakeys.SetLoopStatus:
		switch command.LoopStatus {
		case "None", "Playlist", "Track":
			a.SetRepeat(command.LoopStatus != "None", command.LoopStatus == "Track")
		default:
			return fmt.Errorf("invalid repeat mode %q", command.LoopStatus)
		}
	case mediakeys.SetShuffle:
		a.SetShuffle(command.Enabled)
	case mediakeys.SetVolume:
		if math.IsNaN(command.Volume) || math.IsInf(command.Volume, 0) {
			return errors.New("invalid volume")
		}
		a.SetVolume(int(math.Round(max(0, min(command.Volume, 1)) * 100)))
	case mediakeys.OpenURI:
		if command.Request == nil {
			return errors.New("missing file-open request")
		}
		return a.OpenAudioPaths(command.Request.Paths, command.Request.Enqueue)
	default:
		return handleMediaCommand(a.Player, command)
	}
	return nil
}

func handleMediaCommand(p mediaPlayer, command mediakeys.Command) error {
	switch command.Action {
	case mediakeys.PlayPause:
		p.Play()
	case mediakeys.Play:
		if !p.Playing() || p.Paused() {
			p.Play()
		}
	case mediakeys.Pause:
		p.Pause()
	case mediakeys.Next:
		p.Next()
	case mediakeys.Previous:
		p.Previous()
	case mediakeys.Stop:
		p.Stop()
	case mediakeys.Seek:
		if command.TrackID != p.TrackID() || !p.CanSeek() {
			return errors.New("Cannot seek: the track changed or is not seekable.")
		}
		_, info, _ := p.SongInfo()
		seconds, err := mediakeys.SeekSeconds(command.Position, info.TotalSeconds)
		if err != nil {
			return err
		}
		p.SeekTo(seconds)
	}
	return nil
}

func playbackSnapshot(p *playback.Controller) mediakeys.Snapshot {
	path, info, _ := p.SongInfo()
	if info.Title == "" && path != "" {
		info.Title = media.ChopPath(path)
	}
	s := mediakeys.Snapshot{
		Status: "Stopped", Path: path, Title: info.Title, Artist: info.Artist, Album: info.Album,
		Duration: info.TotalSeconds, Revision: p.Revision(), TrackID: p.TrackID(), CanSeek: p.CanSeek(),
	}
	s.LoopStatus = "None"
	if p.Repeat {
		s.LoopStatus = "Playlist"
		if p.RepeatTrack {
			s.LoopStatus = "Track"
		}
	}
	if p.HasSong() {
		s.Status, s.Position = "Playing", p.Elapsed()
		if p.Paused() {
			s.Status = "Paused"
		}
	}
	s.CanPlay, s.CanPause, s.CanGoNext, s.CanGoPrevious = p.Capabilities()
	return s
}

// usage prints err (if any) and the help text, and returns the exit code.
func usage(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	config.Usage(os.Stderr, formatVersion(version, buildDate))
	return exitFailure
}

// fail prints err and returns the exit code.
func fail(err error) int {
	fmt.Fprintln(os.Stderr, err)
	return exitFailure
}

func printVersion() {
	fmt.Printf("%s version %s - https://github.com/mingcheng/mp3blstr\n", filepath.Base(os.Args[0]), formatVersion(version, buildDate))
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
		if cli.Enqueue {
			if err := a.OpenAudioPaths(cli.Files, true); err != nil {
				a.Warning(err.Error())
			}
			return
		}
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

// newLogger logs to $HOME/.config/mp3blstr/debug.log when debugging is
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
	l.Debug("Debugging of mp3blstr started.")
	return l
}
