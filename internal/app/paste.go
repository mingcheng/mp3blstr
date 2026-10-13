package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kballard/go-shellquote"

	"github.com/mingcheng/mp3blstr/internal/media"
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
	if len(paths) == 0 {
		a.Warning("No audio paths given.")
		return
	}
	for i, path := range paths {
		path = media.ExpandPath(path)
		if !a.Matcher.IsAudio(path) {
			a.Warning(fmt.Sprintf("Not a supported audio file: %s", path))
			return
		}
		if !media.IsHTTPStream(path) {
			if path, err = filepath.Abs(path); err != nil {
				a.Warning(fmt.Sprintf("Invalid audio path: %v", err))
				return
			}
			info, err := os.Stat(path)
			if err != nil {
				a.Warning(fmt.Sprintf("Cannot add audio file: %v", err))
				return
			}
			if !info.Mode().IsRegular() {
				a.Warning(fmt.Sprintf("Not a regular audio file: %s", path))
				return
			}
		}
		paths[i] = path
	}
	a.AddFiles(paths)
	a.Player.QuitAfterPlaylist = false
	a.Player.ResetNextSong()
	a.SetMessage(fmt.Sprintf("Added %d audio files to the current group.", len(paths)))
}
