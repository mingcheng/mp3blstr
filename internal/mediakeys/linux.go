//go:build linux

package mediakeys

import (
	"fmt"
	"os"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	playerPath  = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	rootIface   = "org.mpris.MediaPlayer2"
	playerIface = "org.mpris.MediaPlayer2.Player"
)

func playerName() string {
	return fmt.Sprintf("org.mpris.MediaPlayer2.mp3blstr.instance%d", os.Getpid())
}

// root implements the org.mpris.MediaPlayer2 methods.
type root struct{}

func (root) Raise() *dbus.Error { return nil }
func (root) Quit() *dbus.Error  { return nil }

// player implements the org.mpris.MediaPlayer2.Player methods. Seek is
// renamed on export (see playerMethods) to keep go vet's io.Seeker check quiet.
type player struct{ post func(Action) }

func (p player) PlayPause() *dbus.Error                       { p.post(PlayPause); return nil }
func (p player) Play() *dbus.Error                            { p.post(Play); return nil }
func (p player) Pause() *dbus.Error                           { p.post(Pause); return nil }
func (p player) Next() *dbus.Error                            { p.post(Next); return nil }
func (p player) Previous() *dbus.Error                        { p.post(Previous); return nil }
func (p player) Stop() *dbus.Error                            { p.post(Stop); return nil }
func (player) SeekMPRIS(int64) *dbus.Error                    { return nil }
func (player) SetPosition(dbus.ObjectPath, int64) *dbus.Error { return nil }
func (player) OpenUri(string) *dbus.Error                     { return nil }

var playerMethods = map[string]string{"SeekMPRIS": "Seek"}

type mpris struct {
	conn  *dbus.Conn
	props *prop.Properties
}

func constant(v any) *prop.Prop { return &prop.Prop{Value: v, Emit: prop.EmitConst} }

func Start(post func(Action)) (Controller, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	c, err := export(conn, post)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

func export(conn *dbus.Conn, post func(Action)) (*mpris, error) {
	p := player{post}
	if err := conn.Export(root{}, playerPath, rootIface); err != nil {
		return nil, err
	}
	if err := conn.ExportWithMap(p, playerMethods, playerPath, playerIface); err != nil {
		return nil, err
	}
	props, err := prop.Export(conn, playerPath, prop.Map{
		rootIface: {
			"Identity":            constant("mp3blstr"),
			"CanQuit":             constant(false),
			"CanRaise":            constant(false),
			"HasTrackList":        constant(false),
			"SupportedUriSchemes": constant([]string{}),
			"SupportedMimeTypes":  constant([]string{}),
		},
		playerIface: {
			"PlaybackStatus": {Value: "Stopped", Emit: prop.EmitTrue},
			"LoopStatus":     constant("None"),
			"Rate":           constant(1.0),
			"Shuffle":        constant(false),
			"Metadata":       constant(map[string]dbus.Variant{}),
			"Volume":         constant(1.0),
			"Position":       {Value: int64(0), Emit: prop.EmitFalse},
			"MinimumRate":    constant(1.0),
			"MaximumRate":    constant(1.0),
			"CanGoNext":      constant(true),
			"CanGoPrevious":  constant(true),
			"CanPlay":        constant(true),
			"CanPause":       constant(true),
			"CanSeek":        constant(false),
			"CanControl":     constant(true),
		},
	})
	if err != nil {
		return nil, err
	}
	methods := introspect.Methods(p)
	for i, m := range methods {
		if name, ok := playerMethods[m.Name]; ok {
			methods[i].Name = name
		}
	}
	node := &introspect.Node{
		Name: string(playerPath),
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{Name: rootIface, Methods: introspect.Methods(root{}), Properties: props.Introspection(rootIface)},
			{Name: playerIface, Methods: methods, Properties: props.Introspection(playerIface)},
		},
	}
	if err := conn.Export(introspect.NewIntrospectable(node), playerPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return nil, err
	}
	reply, err := conn.RequestName(playerName(), dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		return nil, fmt.Errorf("MPRIS name unavailable: %v (%v)", reply, err)
	}
	return &mpris{conn: conn, props: props}, nil
}

func (m *mpris) Close() error { return m.conn.Close() }

func (m *mpris) Update(status string) {
	// SetMust panics when the change signal cannot be sent, e.g. after the
	// session bus went away; media keys must never take the player down.
	defer func() { _ = recover() }()
	if m.props.GetMust(playerIface, "PlaybackStatus") != status {
		m.props.SetMust(playerIface, "PlaybackStatus", status)
	}
}
