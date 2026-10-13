//go:build darwin

package mediakeys

import (
	"errors"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
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
)

// Main runs fn and exits with its result. It must be called from main.main:
// fn runs on another goroutine while the main thread runs the main run loop.
func Main(fn func() int) {
	run, err := loadMainLoop()
	if err != nil || pthreadMainNP() != 1 {
		os.Exit(fn())
	}
	mainLoopRunning.Store(true)
	go func() { os.Exit(fn()) }()
	// The main run loop always waits for the main dispatch queue, so this
	// only returns if something stops it explicitly.
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
	events  chan Action
	done    chan struct{}
	once    sync.Once
	info    objc.ID
	title   objc.ID
	targets []commandTarget
	closed  bool // main thread only
}

func Start(post func(Action)) (Controller, error) {
	if !mainLoopRunning.Load() {
		return nil, errors.New("macOS media keys need the main run loop (mediakeys.Main)")
	}
	mp, err := purego.Dlopen("/System/Library/Frameworks/MediaPlayer.framework/MediaPlayer", purego.RTLD_NOW)
	if err != nil {
		return nil, err
	}
	titleKey, err := purego.Dlsym(mp, "MPMediaItemPropertyTitle")
	if err != nil {
		return nil, err
	}
	n := &nowPlaying{events: make(chan Action, 16), done: make(chan struct{})}
	onMainSync(func() { err = n.register(**(**objc.ID)(unsafe.Pointer(&titleKey))) })
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			select {
			case <-n.done:
				return
			case action := <-n.events:
				select {
				case <-n.done:
					return
				default:
					post(action)
				}
			}
		}
	}()
	return n, nil
}

func (n *nowPlaying) register(titleKey objc.ID) error {
	center := objc.ID(objc.GetClass("MPRemoteCommandCenter")).Send(objc.RegisterName("sharedCommandCenter"))
	n.info = objc.ID(objc.GetClass("MPNowPlayingInfoCenter")).Send(objc.RegisterName("defaultCenter"))
	if center == 0 || n.info == 0 || titleKey == 0 {
		return errors.New("macOS Now Playing controls are unavailable")
	}
	addTarget := objc.RegisterName("addTargetWithHandler:")
	for name, action := range map[string]Action{
		"togglePlayPauseCommand": PlayPause,
		"playCommand":            Play,
		"pauseCommand":           Pause,
		"nextTrackCommand":       Next,
		"previousTrackCommand":   Previous,
		"stopCommand":            Stop,
	} {
		command := center.Send(objc.RegisterName(name))
		if command == 0 {
			continue
		}
		block := objc.NewBlock(func(objc.Block, objc.ID) int64 {
			select {
			case n.events <- action:
			default:
			}
			return 0 // MPRemoteCommandHandlerStatusSuccess
		})
		n.targets = append(n.targets, commandTarget{command, command.Send(addTarget, block), block})
	}
	if len(n.targets) == 0 {
		return errors.New("macOS Now Playing commands are unavailable")
	}
	// The title makes the player show up (and stay routable) in Now Playing.
	n.title = objc.ID(objc.GetClass("NSDictionary")).Send(objc.RegisterName("dictionaryWithObject:forKey:"),
		objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), "mp3blstr"), titleKey)
	n.title.Send(objc.RegisterName("retain"))
	n.setNowPlaying(n.title, statePaused)
	return nil
}

func (n *nowPlaying) setNowPlaying(info objc.ID, state uint64) {
	n.info.Send(objc.RegisterName("setNowPlayingInfo:"), info)
	n.info.Send(objc.RegisterName("setPlaybackState:"), state)
}

func (n *nowPlaying) Update(status string) {
	onMain(func() {
		if !n.closed {
			n.setNowPlaying(n.title, playbackState(status))
		}
	})
}

func (n *nowPlaying) Close() error {
	n.once.Do(func() {
		close(n.done)
		onMainSync(func() {
			n.closed = true
			for _, t := range n.targets {
				t.command.Send(objc.RegisterName("removeTarget:"), t.target)
				t.block.Release()
			}
			n.setNowPlaying(0, stateStopped)
			n.title.Send(objc.RegisterName("release"))
		})
	})
	return nil
}
