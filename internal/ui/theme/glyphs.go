package theme

import "github.com/mingcheng/mp3blasterr/internal/playback"

// Glyphs are the symbols used by the interface.
type Glyphs struct {
	// CD-player buttons, 3 cells each.
	Prev, Play, Pause, Next, Rewind, Stop, Forward string

	// Song status shown left of the song title, 2 cells.
	Status map[playback.Status]string

	// Row prefixes in the main window (empty for the classic look).
	Song, Playing, Group, Parent, Dir, Playlist, Other string

	MixerPrefix string

	// Box drawing.
	H, V, TL, TR, BL, BR, LTee, RTee, TTee, BTee, Cross string
}

var plainStatus = map[playback.Status]string{
	playback.StatusPlay:  "|>",
	playback.StatusPause: "||",
	playback.StatusStop:  "[]",
}

// Plain returns the symbols of the original program.
func Plain(rounded bool) Glyphs {
	g := Glyphs{
		Prev: " |<", Play: " |>", Pause: " ||", Next: " >|",
		Rewind: " <<", Stop: " []", Forward: " >>",
		Status: plainStatus,
	}
	g.box(rounded)
	return g
}

// Nerd returns Nerd Font icons (https://www.nerdfonts.com).
func Nerd(rounded bool) Glyphs {
	g := Glyphs{
		Prev:    " \U000F04AE ",
		Play:    " \U000F040A ",
		Pause:   " \U000F03E4 ",
		Next:    " \U000F04AD ",
		Rewind:  " \U000F045F ",
		Stop:    " \U000F04DB ",
		Forward: " \U000F0211 ",
		Status: map[playback.Status]string{
			playback.StatusPlay:  "\U000F040A ",
			playback.StatusPause: "\U000F03E4 ",
			playback.StatusStop:  "\U000F04DB ",
		},
		Song:        "\U000F075A ",
		Playing:     "\U000F040A ",
		Group:       "\U000F1359 ",
		Parent:      "\U000F005D ",
		Dir:         "\U000F024B ",
		Playlist:    "\U000F0CB8 ",
		Other:       "\U000F0214 ",
		MixerPrefix: "\U000F057E ",
	}
	g.box(rounded)
	return g
}

// For returns the glyphs matching theme th, with Nerd Font icons if nerd.
func For(th Theme, nerd bool) Glyphs {
	if nerd {
		return Nerd(th.Rounded)
	}
	return Plain(th.Rounded)
}

// box sets the box drawing characters, with rounded corners if rounded.
func (g *Glyphs) box(rounded bool) {
	g.H, g.V = "─", "│"
	g.TL, g.TR, g.BL, g.BR = "┌", "┐", "└", "┘"
	if rounded {
		g.TL, g.TR, g.BL, g.BR = "╭", "╮", "╰", "╯"
	}
	g.LTee, g.RTee, g.TTee, g.BTee, g.Cross = "├", "┤", "┬", "┴", "┼"
}
