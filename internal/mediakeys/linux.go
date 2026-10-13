//go:build linux

package mediakeys

import (
	"fmt"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

const playerPath = dbus.ObjectPath("/org/mpris/MediaPlayer2")

func playerName() string {
	return fmt.Sprintf("org.mpris.MediaPlayer2.mp3blasterr.instance%d", os.Getpid())
}

type mpris struct {
	conn   *dbus.Conn
	post   func(Action)
	mu     sync.RWMutex
	status string
}

func Start(post func(Action)) (Controller, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	player := &mpris{conn: conn, post: post, status: "Stopped"}
	for _, iface := range []string{"org.mpris.MediaPlayer2", "org.mpris.MediaPlayer2.Player", "org.freedesktop.DBus.Properties"} {
		var err error
		if iface == "org.mpris.MediaPlayer2.Player" {
			err = conn.ExportWithMap(player, map[string]string{"SeekMPRIS": "Seek"}, playerPath, iface)
		} else {
			err = conn.Export(player, playerPath, iface)
		}
		if err != nil {
			conn.Close()
			return nil, err
		}
	}
	if err := conn.Export(introspect.NewIntrospectable(&introspect.Node{
		Name: string(playerPath),
		Interfaces: []introspect.Interface{
			{Name: "org.mpris.MediaPlayer2", Methods: []introspect.Method{{Name: "Raise"}, {Name: "Quit"}}},
			{Name: "org.mpris.MediaPlayer2.Player", Methods: []introspect.Method{
				{Name: "Next"}, {Name: "Previous"}, {Name: "Pause"}, {Name: "PlayPause"},
				{Name: "Stop"}, {Name: "Play"},
			}},
			introspect.IntrospectData,
		},
	}), playerPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		conn.Close()
		return nil, err
	}
	reply, err := conn.RequestName(playerName(), dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		conn.Close()
		return nil, fmt.Errorf("MPRIS name unavailable: %v (%v)", reply, err)
	}
	return player, nil
}

func (m *mpris) Close() error { return m.conn.Close() }

func (m *mpris) Update(status string) {
	m.mu.Lock()
	if m.status == status {
		m.mu.Unlock()
		return
	}
	m.status = status
	m.mu.Unlock()
	_ = m.conn.Emit(playerPath, "org.freedesktop.DBus.Properties.PropertiesChanged", "org.mpris.MediaPlayer2.Player", map[string]dbus.Variant{"PlaybackStatus": dbus.MakeVariant(status)}, []string{})
}

func (m *mpris) PlayPause() *dbus.Error                         { m.post(PlayPause); return nil }
func (m *mpris) Play() *dbus.Error                              { m.post(Play); return nil }
func (m *mpris) Pause() *dbus.Error                             { m.post(Pause); return nil }
func (m *mpris) Next() *dbus.Error                              { m.post(Next); return nil }
func (m *mpris) Previous() *dbus.Error                          { m.post(Previous); return nil }
func (m *mpris) Stop() *dbus.Error                              { m.post(Stop); return nil }
func (m *mpris) SeekMPRIS(int64) *dbus.Error                    { return nil }
func (m *mpris) SetPosition(dbus.ObjectPath, int64) *dbus.Error { return nil }
func (m *mpris) OpenUri(string) *dbus.Error                     { return nil }
func (m *mpris) Raise() *dbus.Error                             { return nil }
func (m *mpris) Quit() *dbus.Error                              { return nil }

func (m *mpris) Get(iface, name string) (dbus.Variant, *dbus.Error) {
	properties := m.properties(iface)
	if value, ok := properties[name]; ok {
		return value, nil
	}
	return dbus.Variant{}, dbus.NewError("org.freedesktop.DBus.Error.UnknownProperty", []any{name})
}

func (m *mpris) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	properties := m.properties(iface)
	if properties == nil {
		return nil, dbus.NewError("org.freedesktop.DBus.Error.UnknownInterface", []any{iface})
	}
	return properties, nil
}

func (m *mpris) Set(iface, name string, value dbus.Variant) *dbus.Error {
	return dbus.NewError("org.freedesktop.DBus.Error.PropertyReadOnly", []any{name})
}

func (m *mpris) properties(iface string) map[string]dbus.Variant {
	switch iface {
	case "org.mpris.MediaPlayer2":
		return map[string]dbus.Variant{
			"Identity":            dbus.MakeVariant("mp3blasterr"),
			"CanQuit":             dbus.MakeVariant(false),
			"CanRaise":            dbus.MakeVariant(false),
			"HasTrackList":        dbus.MakeVariant(false),
			"SupportedUriSchemes": dbus.MakeVariant([]string{}),
			"SupportedMimeTypes":  dbus.MakeVariant([]string{}),
		}
	case "org.mpris.MediaPlayer2.Player":
		m.mu.RLock()
		status := m.status
		m.mu.RUnlock()
		return map[string]dbus.Variant{
			"PlaybackStatus": dbus.MakeVariant(status),
			"LoopStatus":     dbus.MakeVariant("None"),
			"Rate":           dbus.MakeVariant(1.0),
			"Shuffle":        dbus.MakeVariant(false),
			"Metadata":       dbus.MakeVariant(map[string]dbus.Variant{}),
			"Volume":         dbus.MakeVariant(1.0),
			"Position":       dbus.MakeVariant(int64(0)),
			"MinimumRate":    dbus.MakeVariant(1.0),
			"MaximumRate":    dbus.MakeVariant(1.0),
			"CanGoNext":      dbus.MakeVariant(true),
			"CanGoPrevious":  dbus.MakeVariant(true),
			"CanPlay":        dbus.MakeVariant(true),
			"CanPause":       dbus.MakeVariant(true),
			"CanSeek":        dbus.MakeVariant(false),
			"CanControl":     dbus.MakeVariant(true),
		}
	}
	return nil
}
