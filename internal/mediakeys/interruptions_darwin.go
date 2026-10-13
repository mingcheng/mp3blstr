//go:build darwin

package mediakeys

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

const SupportsInterruptions = true

type propertyAddress struct {
	selector, scope, element uint32
}

var defaultOutputAddress = propertyAddress{selector: 0x644f7574, scope: 0x676c6f62} // dOut, glob

type interruptions struct {
	done             chan struct{}
	events           chan Interruption
	once             sync.Once
	center, observer objc.ID
	sleepBlock       objc.Block
	sleepName        objc.ID
	deviceBlock      objc.Block
	removeDevice     func(uint32, *propertyAddress, uintptr, objc.Block) int32
	closeErr         error
}

// WatchInterruptions subscribes to native notifications without polling.
// Wake notifications deliberately do not resume playback.
func WatchInterruptions(sleep, output bool, post func(Interruption)) (io.Closer, error) {
	if !mainLoopRunning.Load() {
		return nil, errors.New("system notifications require the main run loop")
	}
	n := &interruptions{done: make(chan struct{}), events: make(chan Interruption, 1)}
	var err error
	onMainSync(func() {
		if sleep {
			var appkit uintptr
			if appkit, err = purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW); err != nil {
				return
			}
			if n.sleepName, err = stringConstant(appkit, "NSWorkspaceWillSleepNotification"); err != nil {
				return
			}
			workspace := objc.ID(objc.GetClass("NSWorkspace")).Send(objc.RegisterName("sharedWorkspace"))
			n.center = workspace.Send(objc.RegisterName("notificationCenter"))
			n.sleepBlock = objc.NewBlock(func(objc.Block, objc.ID) { n.notify(SystemSleep) })
			n.observer = n.center.Send(objc.RegisterName("addObserverForName:object:queue:usingBlock:"),
				n.sleepName, objc.ID(0), objc.ID(0), n.sleepBlock)
			if n.observer == 0 {
				err = errors.New("could not register sleep notifications")
				return
			}
			n.observer.Send(objc.RegisterName("retain"))
		}
		if output {
			var lib uintptr
			lib, err = purego.Dlopen("/System/Library/Frameworks/CoreAudio.framework/CoreAudio", purego.RTLD_NOW)
			if err != nil {
				return
			}
			var add func(uint32, *propertyAddress, uintptr, objc.Block) int32
			purego.RegisterLibFunc(&add, lib, "AudioObjectAddPropertyListenerBlock")
			purego.RegisterLibFunc(&n.removeDevice, lib, "AudioObjectRemovePropertyListenerBlock")
			n.deviceBlock = objc.NewBlock(func(objc.Block, uint32, *propertyAddress) { n.notify(OutputChanged) })
			if status := add(1, &defaultOutputAddress, mainQueue, n.deviceBlock); status != 0 {
				n.deviceBlock.Release()
				n.deviceBlock = 0
				err = fmt.Errorf("registering default audio output listener: OSStatus %d", status)
			}
		}
	})
	if err != nil {
		return nil, errors.Join(err, n.Close())
	}
	go func() {
		for {
			select {
			case <-n.done:
				return
			case event := <-n.events:
				post(event)
			}
		}
	}()
	return n, nil
}

func (n *interruptions) notify(event Interruption) {
	select {
	case <-n.done:
		return
	default:
	}
	// All pending events have the same effect (pause), so coalesce a burst.
	select {
	case n.events <- event:
	default:
	}
}

func (n *interruptions) Close() error {
	n.once.Do(func() {
		close(n.done)
		onMainSync(func() {
			if n.observer != 0 {
				n.center.Send(objc.RegisterName("removeObserver:"), n.observer)
				n.observer.Send(objc.RegisterName("release"))
			}
			if n.sleepBlock != 0 {
				n.sleepBlock.Release()
			}
			if n.deviceBlock != 0 {
				if status := n.removeDevice(1, &defaultOutputAddress, mainQueue, n.deviceBlock); status != 0 {
					n.closeErr = fmt.Errorf("removing default audio output listener: OSStatus %d", status)
					return // The registered callback must remain alive.
				}
				n.deviceBlock.Release()
			}
		})
	})
	return n.closeErr
}
