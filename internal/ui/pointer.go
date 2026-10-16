/*!*
 * Copyright (c) 2026 Ming Lyu, aka mingcheng
 *
 * This source code is licensed under the MIT License,
 * which is located in the LICENSE file in the source tree's root directory.
 *
 * File: pointer.go
 * Author: mingcheng <mingcheng@apache.org>
 * File Created: 2026-10-16 20:48:44
 *
 * Modified By: mingcheng <mingcheng@apache.org>
 * Last Modified: 2026-10-16 20:48:47
 */

package ui

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
)

type pointerShape uint8

const (
	pointerPass pointerShape = iota
	pointerDefault
	pointerHand
	pointerText
	pointerCommand
)

type pointerProtocol uint8

const (
	pointerUnsupported pointerProtocol = iota
	pointerCSS
	pointerITerm
)

func detectPointer(getenv func(string) string) pointerProtocol {
	term := getenv("TERM")
	// Multiplexers may filter OSC 22 or share the pointer between panes.
	if getenv("TMUX") != "" || getenv("STY") != "" ||
		strings.HasPrefix(term, "screen") || strings.HasPrefix(term, "tmux") {
		return pointerUnsupported
	}
	switch getenv("TERM_PROGRAM") {
	case "kitty", "ghostty", "foot":
		return pointerCSS
	case "iTerm.app":
		parts := strings.Split(getenv("TERM_PROGRAM_VERSION"), ".")
		if len(parts) >= 2 {
			major, majorErr := strconv.Atoi(parts[0])
			minor, minorErr := strconv.Atoi(parts[1])
			if majorErr == nil && minorErr == nil && (major > 3 || major == 3 && minor >= 5) {
				return pointerITerm
			}
		}
		return pointerUnsupported
	case "":
		switch term {
		case "xterm-kitty", "xterm-ghostty", "foot", "foot-extra":
			return pointerCSS
		}
	}
	return pointerUnsupported
}

// pointerScreen keeps OSC 22 output on the screen's TTY, and restores it
// before tview finalizes or suspends the terminal (including Ctrl-C/panics).
type pointerScreen struct {
	tcell.Screen
	mu       sync.Mutex
	protocol pointerProtocol
	current  pointerShape
	active   bool
	dirty    bool
	initErr  error
	err      error
}

func (s *pointerScreen) Init() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initErr = s.Screen.Init()
	s.active = s.initErr == nil
	s.current = pointerDefault
	return s.initErr
}

func (s *pointerScreen) writePointer(shape pointerShape) error {
	tty, ok := s.Screen.Tty()
	if !ok {
		return nil
	}
	name := ""
	switch shape {
	case pointerHand:
		name = "pointer"
		if s.protocol == pointerITerm {
			name = "hand2"
		}
	case pointerText:
		name = "text"
		if s.protocol == pointerITerm {
			name = "xterm"
		}
	}
	sequence := "\x1b]22;" + name + "\x1b\\"
	n, err := io.WriteString(tty, sequence)
	if err == nil && n != len(sequence) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return fmt.Errorf("setting terminal mouse pointer: %w", err)
	}
	return nil
}

func (s *pointerScreen) setPointer(shape pointerShape) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active || s.err != nil || s.protocol == pointerUnsupported || shape == s.current {
		return nil
	}
	// Even a failed write may have changed the terminal; still reset on exit.
	s.dirty = true
	if err := s.writePointer(shape); err != nil {
		s.err = err
		return err
	}
	s.current = shape
	s.dirty = shape != pointerDefault
	return nil
}

func (s *pointerScreen) resetPointer() {
	if s.active && s.dirty {
		s.err = errors.Join(s.err, s.writePointer(pointerDefault))
	}
	s.current, s.dirty, s.active = pointerDefault, false, false
}

func (s *pointerScreen) Fini() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetPointer()
	s.Screen.Fini()
}

func (s *pointerScreen) Suspend() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetPointer()
	return s.Screen.Suspend()
}

func (s *pointerScreen) Resume() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.Screen.Resume()
	s.active = err == nil
	return err
}

func (s *pointerScreen) pointerError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *Screen) pointerAt(x, y int) pointerShape {
	for i := len(s.hits) - 1; i >= 0; i-- {
		hit := s.hits[i]
		if hit.pointer == pointerPass || !hit.r.contains(x, y) {
			continue
		}
		if hit.pointer == pointerCommand {
			if s.app.Busy() {
				return pointerDefault
			}
			return pointerHand
		}
		return hit.pointer
	}
	return pointerDefault
}

func (s *Screen) updatePointer() bool {
	if !s.mouseKnown {
		return false
	}
	if scr, ok := s.scr.(*pointerScreen); ok {
		if err := scr.setPointer(s.pointerAt(s.mouseX, s.mouseY)); err != nil {
			s.app.Warning(err.Error())
			return true
		}
	}
	return false
}
