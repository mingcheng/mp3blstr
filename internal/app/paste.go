package app

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kballard/go-shellquote"
)

// HandlePaste treats a paste as text, never as a sequence of commands.
func (a *App) HandlePaste(text string) {
	if a.Input != InputText || a.Popup == nil || a.Popup.Editor == nil {
		a.Warning("Paste into a text dialog; use 'Add Audio Paths' to add files.")
		return
	}
	if !utf8.ValidString(text) || strings.ContainsFunc(text, func(r rune) bool {
		return unicode.IsControl(r) && r != '\r' && r != '\n' && r != '\t'
	}) {
		a.Warning("Paste contains unsupported control characters.")
		return
	}
	text = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", " ").Replace(text)
	e := a.Popup.Editor
	newLen := len(e.buf) + utf8.RuneCountInString(text)
	if !e.insert {
		newLen = max(len(e.buf), e.pos+utf8.RuneCountInString(text))
	}
	if e.maxLen > 0 && newLen > e.maxLen {
		a.Warning("Pasted text is too long for this dialog.")
		return
	}
	for _, r := range text {
		e.AddChar(r)
	}
}

func (a *App) addPaths(text string) {
	paths, err := shellquote.Split(text)
	if err != nil {
		a.Warning(fmt.Sprintf("Invalid audio paths: %v", err))
		return
	}
	if err := a.OpenAudioPaths(paths, true); err != nil {
		a.Warning(err.Error())
	}
}

// OpenAudioPaths is shared by text input and desktop file-open requests.
func (a *App) OpenAudioPaths(inputs []string, enqueue bool) error {
	paths, err := a.Matcher.AudioPaths(inputs)
	if err != nil {
		return err
	}
	a.AddFiles(paths)
	a.Player.QuitAfterPlaylist = false
	a.Player.ResetNextSong()
	if !enqueue {
		a.Player.Stop()
		a.Player.Resume(paths[0], 0)
		a.Player.Play()
	}
	a.SetMessage(fmt.Sprintf("Added %d audio files to the current group.", len(paths)))
	return nil
}
