package keys

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// Key is a single key stroke: either a printable rune or a special key.
type Key struct {
	Code tcell.Key
	Rune rune
}

// Rune returns the Key for a printable character.
func Rune(r rune) Key { return Key{Code: tcell.KeyRune, Rune: r} }

// Special returns the Key for a non-printable key such as an arrow key.
func Special(k tcell.Key) Key { return Key{Code: k} }

// FromEvent normalises a terminal key event.
func FromEvent(ev *tcell.EventKey) Key {
	switch k := ev.Key(); k {
	case tcell.KeyRune:
		return Rune(ev.Rune())
	case tcell.KeyBackspace2:
		return Special(tcell.KeyBackspace)
	default:
		return Special(k)
	}
}

// IsZero reports whether the key is unset.
func (k Key) IsZero() bool { return k.Code == 0 && k.Rune == 0 }

var specialLabels = map[tcell.Key]string{
	tcell.KeyEnter:     "ent",
	tcell.KeyInsert:    "ins",
	tcell.KeyDelete:    "del",
	tcell.KeyEnd:       "end",
	tcell.KeyHome:      "hom",
	tcell.KeyPgUp:      "pup",
	tcell.KeyPgDn:      "pdn",
	tcell.KeyLeft:      "lft",
	tcell.KeyRight:     "rig",
	tcell.KeyUp:        " up",
	tcell.KeyDown:      "dwn",
	tcell.KeyBackspace: "bsp",
	tcell.KeyCtrlL:     " ^L",
	tcell.KeyTab:       "tab",
	tcell.KeyEsc:       "esc",
}

// Label renders the key as the 3 character label used in the help bar and
// next to the on-screen buttons.
func (k Key) Label() string {
	if k.Code == tcell.KeyRune {
		if k.Rune == ' ' {
			return "spc"
		}
		return " " + string(k.Rune) + " "
	}
	if k.Code >= tcell.KeyF1 && k.Code <= tcell.KeyF64 {
		return fmt.Sprintf("F%2d", int(k.Code-tcell.KeyF1)+1)
	}
	if l, ok := specialLabels[k.Code]; ok {
		return l
	}
	if k.Code >= tcell.KeyCtrlA && k.Code <= tcell.KeyCtrlZ {
		return fmt.Sprintf(" ^%c", 'A'+rune(k.Code-tcell.KeyCtrlA))
	}
	return fmt.Sprintf("s%02x", int(k.Code)&0xff)
}

// Name is the key as written in the key reference: the label without
// padding, with function keys written as F1..F64.
func (k Key) Name() string {
	if k.Code >= tcell.KeyF1 && k.Code <= tcell.KeyF64 {
		return fmt.Sprintf("F%d", int(k.Code-tcell.KeyF1)+1)
	}
	return strings.TrimSpace(k.Label())
}

var parseNames = map[string]Key{
	"spc": Rune(' '),
	"ent": Special(tcell.KeyEnter),
	"ins": Special(tcell.KeyInsert),
	"hom": Special(tcell.KeyHome),
	"del": Special(tcell.KeyDelete),
	"end": Special(tcell.KeyEnd),
	"pup": Special(tcell.KeyPgUp),
	"pdn": Special(tcell.KeyPgDn),
	"up":  Special(tcell.KeyUp),
	"dwn": Special(tcell.KeyDown),
	"lft": Special(tcell.KeyLeft),
	"rig": Special(tcell.KeyRight),
	"bsp": Special(tcell.KeyBackspace),
	"tab": Special(tcell.KeyTab),
	"esc": Special(tcell.KeyEsc),
}

// Parse converts a key description from the configuration file into a Key.
// Accepted forms: a single character, the canonical names (spc, ent, ins,
// hom, del, end, pup, pdn, up, dwn, lft, rig, bsp), f1..f64, kp0..kp9 and
// scancodes written as s<hex><hex>.
func Parse(s string) (Key, error) {
	if len([]rune(s)) == 1 {
		return Rune([]rune(s)[0]), nil
	}
	ls := strings.ToLower(s)
	if k, ok := parseNames[ls]; ok {
		return k, nil
	}
	if strings.HasPrefix(ls, "kp") && len(ls) == 3 && ls[2] >= '0' && ls[2] <= '9' {
		// Terminals do not report the keypad separately; use the digit.
		return Rune(rune(ls[2])), nil
	}
	if strings.HasPrefix(ls, "f") {
		if n, err := strconv.Atoi(ls[1:]); err == nil && n >= 1 && n <= 64 {
			return Special(tcell.KeyF1 + tcell.Key(n-1)), nil
		}
	}
	if strings.HasPrefix(ls, "s") && len(ls) == 3 {
		if n, err := strconv.ParseUint(ls[1:], 16, 8); err == nil {
			switch {
			case n == 13:
				return Special(tcell.KeyEnter), nil
			case n == 8 || n == 127:
				return Special(tcell.KeyBackspace), nil
			case n < 32:
				return Special(tcell.Key(n)), nil
			default:
				return Rune(rune(n)), nil
			}
		}
	}
	if ls == "^l" {
		return Special(tcell.KeyCtrlL), nil
	}
	return Key{}, fmt.Errorf("invalid key %q", s)
}
