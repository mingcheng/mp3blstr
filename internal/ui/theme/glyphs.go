package theme

import (
	"github.com/mingcheng/mp3blstr/internal/keys"
	"github.com/mingcheng/mp3blstr/internal/playback"
)

// Glyphs are the symbols used by the interface.
type Glyphs struct {
	// CD-player buttons, 3 cells each.
	Prev, Play, Pause, Next, Rewind, Stop, Forward string

	// Song status shown left of the song title, 2 cells.
	Status map[playback.Status]string

	// Row prefixes in the main window (empty for the classic look).
	Song, Playing, Group, Parent, Dir, Playlist, Other string

	MixerPrefix string

	// Checkboxes occupy 3 cells; mixer and help arrows occupy 1 cell.
	Checked, Unchecked          string
	MixerDown, MixerUp          string
	HelpPrev, HelpNext, HelpEnd string

	// Optional command icons in the help bar (empty without Nerd Font).
	Shortcuts map[keys.Command]string

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
		Status:  plainStatus,
		Checked: "[X]", Unchecked: "[ ]",
		MixerDown: "<", MixerUp: ">",
		HelpPrev: "(", HelpNext: "(", HelpEnd: ")",
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
		Checked:     " \U000F0132 ",
		Unchecked:   " \U000F0131 ",
		MixerDown:   "\U000F0141",
		MixerUp:     "\U000F0142",
		HelpPrev:    "\U000F0143",
		HelpNext:    "\U000F0140",
		HelpEnd:     " ",
		Shortcuts: map[keys.Command]string{
			keys.CmdSelectFiles:      "\U000F024B",
			keys.CmdAddGroup:         "\U000F0257",
			keys.CmdLoadPlaylist:     "\U000F0770",
			keys.CmdWritePlaylist:    "\U000F0193",
			keys.CmdClearPlaylist:    "\U000F0279",
			keys.CmdDel:              "\U000F01B4",
			keys.CmdAddPaths:         "\U000F0415",
			keys.CmdFileAddFiles:     "\U000F0752",
			keys.CmdFileDirsAsGroups: "\U000F1359",
			keys.CmdFileAddURL:       "\U000F0339",
			keys.CmdFileSetPath:      "\U000F024B",
			keys.CmdFileUpDir:        "\U000F005D",
			keys.CmdFileDelete:       "\U000F01B4",
			keys.CmdToggleRepeat:     "\U000F0456",
			keys.CmdToggleShuffle:    "\U000F049D",
			keys.CmdTogglePlaymode:   "\U000F040A",
			keys.CmdPlayPrevGroup:    "\U000F04AE",
			keys.CmdPlayNextGroup:    "\U000F04AD",
			keys.CmdPlaySkipEnd:      "\U000F0211",
			keys.CmdFileStartSearch:  "\U000F0349",
			keys.CmdSelect:           "\U000F0132",
			keys.CmdFileSelect:       "\U000F0132",
			keys.CmdToggleTheme:      "\U000F050E",
			keys.CmdToggleNerdFont:   "\U000F031C",
			keys.CmdQuitProgram:      "\U000F0343",
			keys.CmdRefresh:          "\U000F0450",
		},
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
