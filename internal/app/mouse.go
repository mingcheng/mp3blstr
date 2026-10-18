package app

import (
	"github.com/mingcheng/mp3blstr/internal/audio"
	"github.com/mingcheng/mp3blstr/internal/keys"
)

// The functions in this file are used by the mouse handlers of the UI.

// Busy reports whether a popup or search is active, in which case mouse
// actions on the main window are ignored.
func (a *App) Busy() bool { return a.Input != InputDefault }

// ClickRow moves the selection bar to line i of the main window.
func (a *App) ClickRow(i int) {
	if a.Busy() {
		return
	}
	a.MainView().Set(i, a.MainLen())
}

// ActivateRow selects line i and "presses enter" on it.
func (a *App) ActivateRow(i int) {
	if a.Busy() {
		return
	}
	a.MainView().Set(i, a.MainLen())
	switch a.Mode {
	case keys.ModeNormal:
		a.Exec(keys.CmdEnter)
	case keys.ModeFileSelection:
		a.Exec(keys.CmdFileEnter)
	}
}

// ToggleRow marks or unmarks line i without moving the selection bar.
func (a *App) ToggleRow(i int) {
	if a.Busy() {
		return
	}
	switch a.Mode {
	case keys.ModeNormal:
		if !a.Cur.IsGroup(i) {
			a.Cur.ToggleSelect(i)
		}
	case keys.ModeFileSelection:
		if e := a.fm.Entry(i); e != nil && e.Name != "./" && e.Name != "../" {
			a.fm.Toggle(i)
		}
	}
}

// Scroll moves the selection bar by delta lines (mouse wheel).
func (a *App) Scroll(delta int) {
	if a.Busy() {
		return
	}
	a.MainView().Move(delta, a.MainLen(), false)
}

// Click runs cmd as if its key had been pressed (clicks on buttons and on
// the key overview).
func (a *App) Click(cmd keys.Command) {
	if a.Busy() {
		return
	}
	a.Exec(cmd)
}

// ClickModeLine toggles the play mode or the sort mode.
func (a *App) ClickModeLine() {
	switch a.Mode {
	case keys.ModeNormal:
		a.Click(keys.CmdTogglePlaymode)
	case keys.ModeFileSelection:
		a.Click(keys.CmdFileToggleSort)
	}
}

// PopupCursor moves the popup's text cursor to rune index i.
func (a *App) PopupCursor(i int) {
	if a.Popup != nil && a.Popup.Editor != nil {
		a.Popup.Editor.SetCursor(i)
	}
}

// SeekTo jumps to secs seconds into the current song (progress bar).
func (a *App) SeekTo(secs int) {
	if !a.Busy() {
		a.Player.SeekTo(secs)
	}
}

// MixerSet sets the current mixer device to v (0..100).
func (a *App) MixerSet(v int) {
	if a.Mixer != nil {
		a.Mixer.Set(v)
		a.playerChanged()
	}
}

// MixerVolume adjusts the current mixer device by delta.
func (a *App) MixerVolume(delta int) {
	if a.Mixer != nil {
		a.Mixer.Change(delta)
		a.playerChanged()
	}
}

// outputMixer returns the mixer applied to the audio output: fadeMixer, or
// Mixer for an App not built by New.
func (a *App) outputMixer() *audio.Mixer {
	if a.fadeMixer != nil {
		return a.fadeMixer
	}
	return a.Mixer
}

// Volume returns the output volume (0..100). It uses the output mixer, so
// it works even when the mixer controls are hidden (--no-mixer).
func (a *App) Volume() int {
	if m := a.outputMixer(); m != nil {
		return m.Device(audio.DevVolume).Value
	}
	return 100
}

// SetVolume sets the output volume (0..100), e.g. from the desktop's media
// controls.
func (a *App) SetVolume(volume int) {
	if m := a.outputMixer(); m != nil {
		m.SetDevice(audio.DevVolume, volume)
		a.playerChanged()
	}
}
