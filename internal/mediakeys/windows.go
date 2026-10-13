//go:build windows

package mediakeys

import (
	"errors"

	"golang.design/x/hotkey"
)

type hotkeys struct {
	registered []*hotkey.Hotkey
	done       chan struct{}
}

func Start(post func(Action)) (Controller, error) {
	controls := &hotkeys{done: make(chan struct{})}
	for _, binding := range []struct {
		key    hotkey.Key
		action Action
	}{
		{hotkey.KeyMediaPlayPause, PlayPause},
		{hotkey.KeyMediaNext, Next},
		{hotkey.KeyMediaPrev, Previous},
		{hotkey.KeyMediaStop, Stop},
	} {
		key := hotkey.New(nil, binding.key)
		if err := key.Register(); err != nil {
			continue
		}
		controls.registered = append(controls.registered, key)
		keydown := key.Keydown()
		keyup := key.Keyup()
		go func(action Action) {
			done := controls.done
			for keydown != nil || keyup != nil {
				select {
				case _, ok := <-keydown:
					if !ok {
						keydown = nil
						continue
					}
					if done != nil {
						post(action)
					}
				case _, ok := <-keyup:
					if !ok {
						keyup = nil
					}
				case <-done:
					done = nil
				}
			}
		}(binding.action)
	}
	if len(controls.registered) == 0 {
		return nil, errors.New("media hotkeys are already registered")
	}
	return controls, nil
}

func (*hotkeys) Update(string) {}

func (h *hotkeys) Close() error {
	close(h.done)
	var errs []error
	for _, key := range h.registered {
		if err := key.Unregister(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
