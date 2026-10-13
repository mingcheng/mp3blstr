//go:build darwin

package mediakeys

import (
	"errors"
	"runtime"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

const (
	systemDefinedEvent = 14
	tapDisabledTimeout = 0xfffffffe
	tapDisabledInput   = 0xffffffff
)

type eventTap struct {
	done    chan struct{}
	stopped chan struct{}
}

type mediaKeyState struct {
	held [Stop + 1]bool
}

func Start(post func(Action)) (Controller, error) {
	quartz, err := purego.Dlopen("/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices", purego.RTLD_NOW)
	if err != nil {
		return nil, err
	}
	coreFoundation, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW)
	if err != nil {
		return nil, err
	}
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW); err != nil {
		return nil, err
	}

	var trusted func() bool
	var createTap func(uint32, uint32, uint32, uint64, uintptr, uintptr) uintptr
	var enableTap func(uintptr, bool)
	var makeSource func(uintptr, uintptr, int32) uintptr
	var addSource func(uintptr, uintptr, uintptr)
	var removeSource func(uintptr, uintptr, uintptr)
	var currentRunLoop func() uintptr
	var runInMode func(uintptr, float64, bool) int32
	var createString func(uintptr, string, uint32) uintptr
	var release func(uintptr)
	purego.RegisterLibFunc(&trusted, quartz, "AXIsProcessTrusted")
	purego.RegisterLibFunc(&createTap, quartz, "CGEventTapCreate")
	purego.RegisterLibFunc(&enableTap, quartz, "CGEventTapEnable")
	purego.RegisterLibFunc(&makeSource, coreFoundation, "CFMachPortCreateRunLoopSource")
	purego.RegisterLibFunc(&addSource, coreFoundation, "CFRunLoopAddSource")
	purego.RegisterLibFunc(&removeSource, coreFoundation, "CFRunLoopRemoveSource")
	purego.RegisterLibFunc(&currentRunLoop, coreFoundation, "CFRunLoopGetCurrent")
	purego.RegisterLibFunc(&runInMode, coreFoundation, "CFRunLoopRunInMode")
	purego.RegisterLibFunc(&createString, coreFoundation, "CFStringCreateWithCString")
	purego.RegisterLibFunc(&release, coreFoundation, "CFRelease")
	if !trusted() {
		return nil, errors.New("media keys require macOS Accessibility permission")
	}
	mode := createString(0, "kCFRunLoopDefaultMode", 0x08000100)
	if mode == 0 {
		return nil, errors.New("failed to create macOS run loop mode")
	}

	controls := &eventTap{done: make(chan struct{}), stopped: make(chan struct{})}
	events := make(chan Action, 16)
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(controls.stopped)
		defer release(mode)

		eventWithCGEvent := objc.RegisterName("eventWithCGEvent:")
		subtype := objc.RegisterName("subtype")
		data1 := objc.RegisterName("data1")
		alloc := objc.RegisterName("alloc")
		init := objc.RegisterName("init")
		drain := objc.RegisterName("drain")
		poolClass := objc.ID(objc.GetClass("NSAutoreleasePool"))
		eventClass := objc.ID(objc.GetClass("NSEvent"))
		var keys mediaKeyState
		var tap uintptr
		callback := purego.NewCallback(func(_ uintptr, eventType uint32, event uintptr, _ uintptr) uintptr {
			if eventType == tapDisabledTimeout || eventType == tapDisabledInput {
				keys = mediaKeyState{}
				enableTap(tap, true)
				return event
			}
			if eventType != systemDefinedEvent {
				return event
			}
			pool := poolClass.Send(alloc).Send(init)
			defer pool.Send(drain)
			nsEvent := eventClass.Send(eventWithCGEvent, objc.ID(event))
			if nsEvent == 0 || objc.Send[int64](nsEvent, subtype) != 8 {
				return event
			}
			if !keys.handle(uint32(objc.Send[int64](nsEvent, data1)), func(action Action) bool {
				select {
				case events <- action:
					return true
				default:
					return false
				}
			}) {
				return event
			}
			return 0
		})
		tap = createTap(1, 0, 0, uint64(1)<<systemDefinedEvent, callback, 0)
		if tap == 0 {
			ready <- errors.New("failed to create macOS media key event tap; check Input Monitoring permission")
			return
		}
		defer release(tap)
		source := makeSource(0, tap, 0)
		if source == 0 {
			ready <- errors.New("failed to create macOS media key run loop source")
			return
		}
		defer release(source)
		loop := currentRunLoop()
		addSource(loop, source, mode)
		defer removeSource(loop, source, mode)
		enableTap(tap, true)
		defer enableTap(tap, false)
		ready <- nil
		for {
			select {
			case <-controls.done:
				return
			default:
				runInMode(mode, 0.1, false)
			}
		}
	}()
	if err := <-ready; err != nil {
		<-controls.stopped
		return nil, err
	}
	go func() {
		for {
			select {
			case <-controls.done:
				return
			case action := <-events:
				select {
				case <-controls.done:
					return
				default:
					post(action)
				}
			}
		}
	}()
	return controls, nil
}

func mediaAction(data uint32) (Action, bool, bool) {
	key := data >> 16
	state := (data >> 8) & 0xff
	if state != 0x0a && state != 0x0b {
		return 0, false, false
	}
	switch key {
	case 16:
		return PlayPause, state == 0x0a, true
	case 17:
		return Next, state == 0x0a, true
	case 18:
		return Previous, state == 0x0a, true
	}
	return 0, false, false
}

func (m *mediaKeyState) handle(data uint32, offer func(Action) bool) bool {
	action, down, ok := mediaAction(data)
	if !ok {
		return false
	}
	if down {
		if m.held[action] {
			return true
		}
		if !offer(action) {
			return false
		}
		m.held[action] = true
		return true
	}
	if !m.held[action] {
		return false
	}
	m.held[action] = false
	return true
}

func (*eventTap) Update(string) {}

func (e *eventTap) Close() error {
	close(e.done)
	<-e.stopped
	return nil
}
