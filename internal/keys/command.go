// Package keys defines the commands understood by mp3blasterr, the program
// modes in which they are valid and the (re)configurable key bindings that
// trigger them.
package keys

import "strings"

// Command identifies a user action.
type Command int

// All commands, mirroring the command set of the original mp3blaster.
const (
	CmdNone Command = iota
	CmdSelectFiles
	CmdAddGroup
	CmdLoadPlaylist
	CmdWritePlaylist
	CmdSetGroupTitle
	CmdToggleRepeat
	CmdToggleFadeOut
	CmdToggleShuffle
	CmdEnter
	CmdTogglePlaymode
	CmdChangeThread
	CmdToggleMixer
	CmdMixerVolDown
	CmdMixerVolUp
	CmdMoveAfter
	CmdMoveBefore
	CmdQuitProgram
	CmdHelp
	CmdDel
	CmdSelect
	CmdRefresh
	CmdPrevPage
	CmdNextPage
	CmdUp
	CmdDown
	CmdFileAddFiles
	CmdFileInvSelection
	CmdFileRecursiveSelect
	CmdFileSetPath
	CmdFileDirsAsGroups
	CmdFileAddURL
	CmdFileStartSearch
	CmdFileEnter
	CmdFileUpDir
	CmdPlayPrevious
	CmdPlayPlay
	CmdPlayNext
	CmdPlayRewind
	CmdPlayStop
	CmdPlayForward
	CmdFileSelect
	CmdHelpPrev
	CmdHelpNext
	CmdClearPlaylist
	CmdFileToggleSort
	CmdFileDelete
	CmdPlaySkipEnd
	CmdPlayNextGroup
	CmdPlayPrevGroup
	CmdSelectItems
	CmdDeselectItems
	CmdFileRename
	CmdToggleDisplay
	CmdJumpTop
	CmdJumpBottom
	CmdToggleWrap
	CmdLeft
	CmdRight
	CmdToggleTheme
	CmdToggleNerdFont
	CmdRemoveDuplicates
)

// Mode is a bit mask of program modes, so a binding can be valid in several.
type Mode uint8

// Program modes.
const (
	ModeNormal        Mode = 1
	ModeFileSelection Mode = 2
	ModeHelp          Mode = 4
	ModeAny           Mode = 0xff
)

// configNames maps key binding names (case-insensitive) to the command they
// rebind. The configuration file uses them in snake_case below its keys
// section: keys.play.next is "key.play.next", keys.file.add_files is
// "key.file.addfiles". Deprecated aliases from the original are kept.
var configNames = map[string]Command{
	"key.selectfiles":          CmdSelectFiles,
	"key.addgroup":             CmdAddGroup,
	"key.loadplaylist":         CmdLoadPlaylist,
	"key.writeplaylist":        CmdWritePlaylist,
	"key.setgrouptitle":        CmdSetGroupTitle,
	"key.togglerepeat":         CmdToggleRepeat,
	"key.togglefadeout":        CmdToggleFadeOut,
	"key.toggleshuffle":        CmdToggleShuffle,
	"key.toggleplaymode":       CmdTogglePlaymode,
	"key.startplaylist":        CmdNone, // obsolete in the original as well
	"key.changethread":         CmdChangeThread,
	"key.togglemixer":          CmdToggleMixer,
	"key.mixervoldown":         CmdMixerVolDown,
	"key.mixervolup":           CmdMixerVolUp,
	"key.moveafter":            CmdMoveAfter,
	"key.movebefore":           CmdMoveBefore,
	"key.quitprogram":          CmdQuitProgram,
	"key.help":                 CmdHelp,
	"key.del":                  CmdDel,
	"key.select":               CmdSelect,
	"key.enter":                CmdEnter,
	"key.refresh":              CmdRefresh,
	"key.prevpage":             CmdPrevPage,
	"key.nextpage":             CmdNextPage,
	"key.up":                   CmdUp,
	"key.down":                 CmdDown,
	"key.file.down":            CmdDown,
	"key.file.up":              CmdUp,
	"key.file.prevpage":        CmdPrevPage,
	"key.file.nextpage":        CmdNextPage,
	"key.file.enter":           CmdFileEnter,
	"key.file.select":          CmdFileSelect,
	"key.file.addfiles":        CmdFileAddFiles,
	"key.file.invselection":    CmdFileInvSelection,
	"key.file.recursiveselect": CmdFileRecursiveSelect,
	"key.file.setpath":         CmdFileSetPath,
	"key.file.dirsasgroups":    CmdFileDirsAsGroups,
	"key.file.mp3towav":        CmdNone, // MP3 to WAV conversion was removed
	"key.file.addurl":          CmdFileAddURL,
	"key.startsearch":          CmdFileStartSearch,
	"key.file.startsearch":     CmdFileStartSearch,
	"key.file.updir":           CmdFileUpDir,
	"key.play.previous":        CmdPlayPrevious,
	"key.play.play":            CmdPlayPlay,
	"key.play.next":            CmdPlayNext,
	"key.play.rewind":          CmdPlayRewind,
	"key.play.stop":            CmdPlayStop,
	"key.play.forward":         CmdPlayForward,
	"key.helpprev":             CmdHelpPrev,
	"key.helpnext":             CmdHelpNext,
	"key.clearplaylist":        CmdClearPlaylist,
	"key.removeduplicates":     CmdRemoveDuplicates,
	"key.file.delete":          CmdFileDelete,
	"key.play.skipend":         CmdPlaySkipEnd,
	"key.play.nextgroup":       CmdPlayNextGroup,
	"key.play.prevgroup":       CmdPlayPrevGroup,
	"key.file.rename":          CmdFileRename,
	"key.togglesort":           CmdFileToggleSort,
	"key.toggledisplay":        CmdToggleDisplay,
	"key.left":                 CmdLeft,
	"key.right":                CmdRight,
	"key.home":                 CmdJumpTop,
	"key.end":                  CmdJumpBottom,
	"key.selectitems":          CmdSelectItems,
	"key.deselectitems":        CmdDeselectItems,
	"key.togglewrap":           CmdToggleWrap,
	"key.toggletheme":          CmdToggleTheme,
	"key.togglenerdfont":       CmdToggleNerdFont,
}

// CommandForConfigName resolves a "key.*" binding name (see configNames).
// Obsolete names resolve to CmdNone.
func CommandForConfigName(name string) (Command, bool) {
	c, ok := configNames[strings.ToLower(name)]
	return c, ok
}
