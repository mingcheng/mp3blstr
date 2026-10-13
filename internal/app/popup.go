package app

import (
	"github.com/gdamore/tcell/v2"

	"github.com/mingcheng/mp3blstr/internal/keys"
)

// The popups are modal: while one is open, HandleKey sends all keys to it.

// openPopup asks for a line of text (at most maxLen runes); submit runs
// with the text when the user presses Enter.
func (a *App) openPopup(labels []string, initial string, maxLen int, submit func(string)) {
	a.Popup = &Popup{Labels: labels, Editor: NewLineEditor(initial, maxLen), submit: submit}
	a.Input = InputText
	a.SetMessage("Use up and down arrows to toggle insert mode")
}

// openConfirm shows a yes/no dialog; onYes runs only if the user answers 'y'.
func (a *App) openConfirm(labels []string, onYes func()) {
	a.Popup = &Popup{Labels: labels, Confirm: true, onYes: onYes}
	a.Input = InputConfirm
	a.SetMessage("Press 'y' to confirm, 'n' or Esc to cancel")
}

// confirmKey handles keys while a yes/no question is shown.
func (a *App) confirmKey(k keys.Key) {
	switch {
	case k.Code == tcell.KeyRune && (k.Rune == 'y' || k.Rune == 'Y'):
		onYes := a.Popup.onYes
		a.ClosePopup()
		onYes()
	case k.Code == tcell.KeyRune && (k.Rune == 'n' || k.Rune == 'N'),
		k.Code == tcell.KeyEsc, k.Code == tcell.KeyEnter:
		a.ClosePopup()
		a.SetMessage("Cancelled.")
	}
}

// ClosePopup cancels the popup.
func (a *App) ClosePopup() {
	a.Popup = nil
	a.Input = InputDefault
	a.Msg = Message{}
}

// popupKey handles keys while a text popup is shown.
func (a *App) popupKey(k keys.Key) {
	e := a.Popup.Editor
	switch k.Code {
	case tcell.KeyLeft:
		e.Left()
	case tcell.KeyRight:
		e.Right()
	case tcell.KeyUp, tcell.KeyDown:
		if e.ToggleInsert() {
			a.SetMessage("--  INSERT --")
		} else {
			a.SetMessage("-- REPLACE --")
		}
	case tcell.KeyBackspace:
		e.Backspace()
	case tcell.KeyDelete:
		e.Delete()
	case tcell.KeyHome:
		e.Home()
	case tcell.KeyEnd:
		e.End()
	case tcell.KeyEsc:
		a.ClosePopup()
	case tcell.KeyEnter:
		submit, text := a.Popup.submit, e.Text()
		a.ClosePopup()
		submit(text)
	case tcell.KeyRune:
		e.AddChar(k.Rune)
	}
}
