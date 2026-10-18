//go:build darwin

package mediakeys

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/mingcheng/mp3blstr/internal/media"
)

// macOS routes media keys (and Control Center, headphones, …) to the current
// "Now Playing" application through MPRemoteCommandCenter. This needs no
// Accessibility or Input Monitoring permission, but the commands arrive on the
// main dispatch queue, so the main thread has to run its run loop; see Main.

// Keep main.main on the main thread so Main can run the main run loop.
func init() { runtime.LockOSThread() }

var (
	mainLoopRunning atomic.Bool
	mainQueue       uintptr
	dispatchAsync   func(queue uintptr, block objc.Block)
	pthreadMainNP   func() int32
	mainLoopError   error
)

// Main runs fn and exits with its result. It must be called from main.main:
// fn runs on another goroutine while the main thread runs the main run loop.
func Main(fn func() int) {
	run, err := loadMainLoop()
	if err != nil || pthreadMainNP() != 1 {
		mainLoopError = err
		if mainLoopError == nil {
			mainLoopError = errors.New("the main run loop must run on the process main thread")
		}
		fmt.Fprintln(os.Stderr, "macOS application initialization:", mainLoopError)
		os.Exit(fn())
	}
	mainLoopRunning.Store(true)
	go func() { os.Exit(fn()) }()
	// The main run loop services the main dispatch queue without creating
	// an NSApplication or changing the terminal process's activation policy.
	run()
	select {}
}

func loadMainLoop() (run func(), err error) {
	libSystem, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW)
	if err != nil {
		return nil, err
	}
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW)
	if err != nil {
		return nil, err
	}
	if _, err := purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_NOW); err != nil {
		return nil, err
	}
	if mainQueue, err = purego.Dlsym(libSystem, "_dispatch_main_q"); err != nil {
		return nil, err
	}
	purego.RegisterLibFunc(&pthreadMainNP, libSystem, "pthread_main_np")
	purego.RegisterLibFunc(&dispatchAsync, libSystem, "dispatch_async")
	purego.RegisterLibFunc(&run, cf, "CFRunLoopRun")
	return run, nil
}

// onMain runs fn asynchronously on the main thread.
func onMain(fn func()) {
	block := objc.NewBlock(func(objc.Block) {
		pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
		defer pool.Send(objc.RegisterName("drain"))
		fn()
	})
	dispatchAsync(mainQueue, block)
	block.Release()
}

func onMainSync(fn func()) {
	done := make(chan struct{})
	onMain(func() {
		defer close(done)
		fn()
	})
	<-done
}

// MPNowPlayingPlaybackState values.
const (
	statePlaying = 1
	statePaused  = 2
	stateStopped = 3
)

// playbackState maps a playback status to MPNowPlayingPlaybackState. Stopped
// is reported as paused: a stopped player stops receiving the play key.
func playbackState(status string) uint64 {
	if status == "Playing" {
		return statePlaying
	}
	return statePaused
}

type commandTarget struct {
	command, target objc.ID
	block           objc.Block
}

type nowPlaying struct {
	events          chan Command
	done            chan struct{}
	once            sync.Once
	info            objc.ID
	keys            map[string]objc.ID
	targets         []commandTarget
	closed          bool     // main thread only
	last            Snapshot // UI goroutine only
	updated         bool
	positionCommand objc.ID
	current         Snapshot // main thread only
	currentAt       time.Time
	artwork         objc.ID // owned, main thread only
	artworkTrack    artworkRequest
	artworkRequests chan artworkRequest
	readArtwork     func(string) (*media.Artwork, error)
	report          func(error)
}

func Start(post func(Command), report func(error)) (Controller, error) {
	if post == nil || report == nil {
		//lint:ignore ST1005 "Now Playing" is the name of the macOS feature.
		return nil, errors.New("Now Playing requires command and error callbacks")
	}
	if !mainLoopRunning.Load() {
		return nil, errors.Join(errors.New("macOS media keys require the main run loop (mediakeys.Main)"), mainLoopError)
	}
	keys, err := loadNowPlayingKeys()
	if err != nil {
		return nil, err
	}
	n := &nowPlaying{
		events: make(chan Command, 16), done: make(chan struct{}), keys: keys,
		artworkRequests: make(chan artworkRequest, 1), readArtwork: media.ReadArtwork, report: report,
	}
	onMainSync(func() { err = n.register() })
	if err != nil {
		n.Close()
		return nil, err
	}
	go n.loadArtwork()
	go func() {
		for {
			select {
			case <-n.done:
				return
			case command := <-n.events:
				select {
				case <-n.done:
					return
				default:
					post(command)
				}
			}
		}
	}()
	return n, nil
}

func loadNowPlayingKeys() (map[string]objc.ID, error) {
	mp, err := purego.Dlopen("/System/Library/Frameworks/MediaPlayer.framework/MediaPlayer", purego.RTLD_NOW)
	if err != nil {
		return nil, err
	}
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW); err != nil {
		return nil, err
	}
	keys := make(map[string]objc.ID)
	for _, name := range []string{
		"MPMediaItemPropertyTitle", "MPMediaItemPropertyArtist", "MPMediaItemPropertyAlbumTitle",
		"MPMediaItemPropertyPlaybackDuration", "MPNowPlayingInfoPropertyElapsedPlaybackTime",
		"MPNowPlayingInfoPropertyPlaybackRate",
		"MPMediaItemPropertyArtwork",
	} {
		value, err := stringConstant(mp, name)
		if err != nil {
			return nil, err
		}
		keys[name] = value
	}
	return keys, nil
}

func stringConstant(library uintptr, name string) (objc.ID, error) {
	symbol, err := purego.Dlsym(library, name)
	if err != nil {
		return 0, fmt.Errorf("native string %s: %w", name, err)
	}
	value := **(**objc.ID)(unsafe.Pointer(&symbol))
	if value == 0 {
		return 0, fmt.Errorf("native string %s is unavailable", name)
	}
	return value, nil
}

func (n *nowPlaying) register() error {
	center := objc.ID(objc.GetClass("MPRemoteCommandCenter")).Send(objc.RegisterName("sharedCommandCenter"))
	n.info = objc.ID(objc.GetClass("MPNowPlayingInfoCenter")).Send(objc.RegisterName("defaultCenter"))
	if center == 0 || n.info == 0 {
		return errors.New("macOS Now Playing controls are unavailable")
	}
	for name, action := range map[string]Action{
		"togglePlayPauseCommand":        PlayPause,
		"playCommand":                   Play,
		"pauseCommand":                  Pause,
		"nextTrackCommand":              Next,
		"previousTrackCommand":          Previous,
		"stopCommand":                   Stop,
		"changePlaybackPositionCommand": Seek,
	} {
		command := center.Send(objc.RegisterName(name))
		if err := n.registerCommand(command, action); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	// Keep transport commands available before the first song is loaded.
	n.publish(Snapshot{Status: "Stopped"})
	return nil
}

func (n *nowPlaying) registerCommand(command objc.ID, action Action) error {
	if command == 0 {
		return errors.New("native media command is unavailable")
	}
	block := objc.NewBlock(func(_ objc.Block, event objc.ID) int64 {
		return n.handleCommand(action, event)
	})
	target := command.Send(objc.RegisterName("addTargetWithHandler:"), block)
	if target == 0 {
		block.Release()
		return errors.New("native media command registration failed")
	}
	command.Send(objc.RegisterName("retain"))
	target.Send(objc.RegisterName("retain"))
	n.targets = append(n.targets, commandTarget{command, target, block})
	if action == Seek {
		n.positionCommand = command
		command.Send(objc.RegisterName("setEnabled:"), false)
	}
	return nil
}

func (n *nowPlaying) handleCommand(action Action, event objc.ID) int64 {
	if n.closed {
		return 200 // MPRemoteCommandHandlerStatusCommandFailed
	}
	command := Command{Action: action}
	if action == Seek {
		if !n.current.CanSeek || n.current.Status == "Stopped" {
			return 100 // MPRemoteCommandHandlerStatusNoSuchContent
		}
		selector := objc.RegisterName("positionTime")
		if event == 0 || !objc.Send[bool](event, objc.RegisterName("respondsToSelector:"), selector) {
			return 200
		}
		position := objc.Send[float64](event, selector)
		if _, err := SeekSeconds(position, n.current.Duration); err != nil {
			return 200
		}
		command.Position, command.TrackID = position, n.current.TrackID
	}
	select {
	case n.events <- command:
		return 0 // MPRemoteCommandHandlerStatusSuccess (accepted for the UI queue)
	default:
		return 200
	}
}

func (n *nowPlaying) setNowPlaying(info objc.ID, state uint64) {
	n.info.Send(objc.RegisterName("setNowPlayingInfo:"), info)
	n.info.Send(objc.RegisterName("setPlaybackState:"), state)
}

func (n *nowPlaying) Update(s Snapshot) {
	if n.updated && s.samePlayback(n.last) {
		return
	}
	n.last, n.updated = s, true
	onMain(func() {
		if !n.closed {
			n.current, n.currentAt = s, time.Now()
			n.updateArtwork(s)
			n.publish(s)
		}
	})
}

func (n *nowPlaying) metadata(s Snapshot) objc.ID {
	info := objc.ID(objc.GetClass("NSMutableDictionary")).Send(objc.RegisterName("dictionary"))
	set := func(key string, value objc.ID) {
		info.Send(objc.RegisterName("setObject:forKey:"), value, n.keys[key])
	}
	if s.Title == "" {
		s.Title = "mp3blstr"
	}
	for key, text := range map[string]string{
		"MPMediaItemPropertyTitle": s.Title, "MPMediaItemPropertyArtist": s.Artist,
		"MPMediaItemPropertyAlbumTitle": s.Album,
	} {
		set(key, objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), text))
	}
	number := func(value int) objc.ID {
		return objc.ID(objc.GetClass("NSNumber")).Send(objc.RegisterName("numberWithLongLong:"), int64(value))
	}
	if s.Duration > 0 {
		set("MPMediaItemPropertyPlaybackDuration", number(s.Duration))
	}
	set("MPNowPlayingInfoPropertyElapsedPlaybackTime", number(max(s.Position, 0)))
	rate := 0
	if s.Status == "Playing" {
		rate = 1
	}
	set("MPNowPlayingInfoPropertyPlaybackRate", number(rate))
	if n.artwork != 0 {
		set("MPMediaItemPropertyArtwork", n.artwork)
	}
	return info
}

func (n *nowPlaying) publish(s Snapshot) {
	if n.positionCommand != 0 {
		n.positionCommand.Send(objc.RegisterName("setEnabled:"), s.CanSeek && s.Status != "Stopped")
	}
	n.setNowPlaying(n.metadata(s), playbackState(s.Status))
}

func (n *nowPlaying) Close() error {
	n.once.Do(func() {
		close(n.done)
		onMainSync(func() {
			n.closed = true
			if n.positionCommand != 0 {
				n.positionCommand.Send(objc.RegisterName("setEnabled:"), false)
			}
			for _, t := range n.targets {
				t.command.Send(objc.RegisterName("removeTarget:"), t.target)
				t.target.Send(objc.RegisterName("release"))
				t.command.Send(objc.RegisterName("release"))
				t.block.Release()
			}
			n.setNowPlaying(0, stateStopped)
			n.clearArtwork()
		})
	})
	return nil
}
