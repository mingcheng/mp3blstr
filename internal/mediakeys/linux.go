//go:build linux

package mediakeys

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"

	"github.com/mingcheng/mp3blstr/internal/media"
)

const (
	playerPath  = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	rootIface   = "org.mpris.MediaPlayer2"
	playerIface = "org.mpris.MediaPlayer2.Player"
	filesIface  = "org.mp3blstr.Player"
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
type player struct {
	post  func(Command)
	state *mpris
}

func (p player) PlayPause() *dbus.Error { p.post(Command{Action: PlayPause}); return nil }
func (p player) Play() *dbus.Error      { p.post(Command{Action: Play}); return nil }
func (p player) Pause() *dbus.Error     { p.post(Command{Action: Pause}); return nil }
func (p player) Next() *dbus.Error      { p.post(Command{Action: Next}); return nil }
func (p player) Previous() *dbus.Error  { p.post(Command{Action: Previous}); return nil }
func (p player) Stop() *dbus.Error      { p.post(Command{Action: Stop}); return nil }
func (p player) OpenUri(uri string) *dbus.Error {
	parsed, err := url.Parse(uri)
	if err != nil || (parsed.Scheme != "file" && parsed.Scheme != "http" && parsed.Scheme != "https") {
		return prop.ErrInvalidArg
	}
	return openFiles(p.post, []string{uri}, false)
}

type fileOpener struct{ post func(Command) }

func (f fileOpener) Open(paths []string, enqueue bool) *dbus.Error {
	return openFiles(f.post, paths, enqueue)
}

func openFiles(post func(Command), paths []string, enqueue bool) *dbus.Error {
	if len(paths) == 0 {
		return prop.ErrInvalidArg
	}
	request := &OpenRequest{Paths: paths, Enqueue: enqueue, Result: make(chan error, 1)}
	// No property lock may be held while waiting for the UI to update MPRIS.
	post(Command{Action: OpenURI, Request: request})
	if err := <-request.Result; err != nil {
		return dbus.MakeFailedError(err)
	}
	return nil
}

func (p player) SeekMPRIS(offset int64) *dbus.Error {
	if p.state == nil {
		return nil
	}
	p.state.mu.Lock()
	s := p.state.current
	position := p.state.position()
	p.state.mu.Unlock()
	if s.CanSeek && s.Status != "Stopped" && s.Duration > 0 {
		p.post(Command{Action: Seek, TrackID: s.TrackID, Position: float64(position)/1e6 + float64(offset)/1e6})
	}
	return nil
}

func (p player) SetPosition(track dbus.ObjectPath, position int64) *dbus.Error {
	if p.state == nil {
		return nil
	}
	p.state.mu.Lock()
	s := p.state.current
	p.state.mu.Unlock()
	if s.CanSeek && s.Status != "Stopped" && s.Duration > 0 && track == trackPath(s) && position >= 0 && float64(position)/1e6 <= float64(s.Duration) {
		p.post(Command{Action: Seek, TrackID: s.TrackID, Position: float64(position) / 1e6})
	}
	return nil
}

var playerMethods = map[string]string{"SeekMPRIS": "Seek"}

type mpris struct {
	conn                    *dbus.Conn
	props                   *prop.Properties
	mu                      sync.Mutex
	current                 Snapshot
	updatedAt               time.Time
	closed                  bool
	requests                chan Snapshot
	done                    chan struct{}
	finished                chan struct{}
	report                  func(error)
	artDir, artFile, artURL string
	publishedMetadata       map[string]dbus.Variant
}

type mprisProperties struct {
	*prop.Properties
	state *mpris
	post  func(Command)
}

func (p mprisProperties) Set(iface, name string, value dbus.Variant) *dbus.Error {
	if iface == playerIface {
		switch name {
		case "LoopStatus":
			mode, ok := value.Value().(string)
			if !ok || (mode != "None" && mode != "Playlist" && mode != "Track") {
				return prop.ErrInvalidArg
			}
			p.post(Command{Action: SetLoopStatus, LoopStatus: mode})
			return nil
		case "Shuffle":
			enabled, ok := value.Value().(bool)
			if !ok {
				return prop.ErrInvalidArg
			}
			p.post(Command{Action: SetShuffle, Enabled: enabled})
			return nil
		case "Volume":
			volume, ok := value.Value().(float64)
			if !ok || math.IsNaN(volume) || math.IsInf(volume, 0) {
				return prop.ErrInvalidArg
			}
			p.post(Command{Action: SetVolume, Volume: max(0, min(volume, 1))})
			return nil
		}
	}
	return p.Properties.Set(iface, name, value)
}

func (p mprisProperties) Get(iface, name string) (dbus.Variant, *dbus.Error) {
	if iface == playerIface && (name == "Position" || name == "Metadata") {
		p.state.mu.Lock()
		defer p.state.mu.Unlock()
		if name == "Metadata" {
			return dbus.MakeVariant(p.state.metadata()), nil
		}
		return dbus.MakeVariant(p.state.position()), nil
	}
	return p.Properties.Get(iface, name)
}

func (p mprisProperties) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	values, err := p.Properties.GetAll(iface)
	if err == nil && iface == playerIface {
		p.state.mu.Lock()
		values["Position"] = dbus.MakeVariant(p.state.position())
		values["Metadata"] = dbus.MakeVariant(p.state.metadata())
		p.state.mu.Unlock()
	}
	return values, err
}

func trackPath(s Snapshot) dbus.ObjectPath {
	if s.Status == "Stopped" || s.Path == "" {
		return "/org/mpris/MediaPlayer2/TrackList/NoTrack"
	}
	return dbus.ObjectPath(fmt.Sprintf("/org/mpris/MediaPlayer2/track/%d", s.TrackID))
}

func fileURI(path string) string {
	if parsed, err := url.Parse(path); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		return parsed.String()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return (&url.URL{Scheme: "file", Path: abs}).String()
}

func metadata(s Snapshot) map[string]dbus.Variant {
	values := map[string]dbus.Variant{"mpris:trackid": dbus.MakeVariant(trackPath(s))}
	if s.Status == "Stopped" || s.Path == "" {
		return values
	}
	values["xesam:url"] = dbus.MakeVariant(fileURI(s.Path))
	values["xesam:title"] = dbus.MakeVariant(s.Title)
	if s.Artist != "" {
		values["xesam:artist"] = dbus.MakeVariant([]string{s.Artist})
	}
	if s.Album != "" {
		values["xesam:album"] = dbus.MakeVariant(s.Album)
	}
	if s.Duration > 0 {
		values["mpris:length"] = dbus.MakeVariant(int64(s.Duration) * 1_000_000)
	}
	return values
}

func constant(v any) *prop.Prop { return &prop.Prop{Value: v, Emit: prop.EmitConst} }

func Start(post func(Command), report func(error)) (Controller, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	c, err := export(conn, post)
	if err != nil {
		conn.Close()
		return nil, err
	}
	c.report = report
	go c.loadArtwork()
	return c, nil
}

func export(conn *dbus.Conn, post func(Command)) (*mpris, error) {
	m := &mpris{conn: conn, requests: make(chan Snapshot, 1), done: make(chan struct{}), finished: make(chan struct{})}
	p := player{post: post, state: m}
	if err := conn.Export(root{}, playerPath, rootIface); err != nil {
		return nil, err
	}
	if err := conn.ExportWithMap(p, playerMethods, playerPath, playerIface); err != nil {
		return nil, err
	}
	if err := conn.Export(fileOpener{post: post}, playerPath, filesIface); err != nil {
		return nil, err
	}
	props, err := prop.Export(conn, playerPath, prop.Map{
		rootIface: {
			"Identity":            constant("mp3blstr"),
			"DesktopEntry":        constant("mp3blstr"),
			"CanQuit":             constant(false),
			"CanRaise":            constant(false),
			"HasTrackList":        constant(false),
			"SupportedUriSchemes": constant([]string{"file", "http", "https"}),
			"SupportedMimeTypes":  constant([]string{"audio/mpeg", "audio/flac", "audio/x-flac", "audio/ogg", "audio/x-vorbis+ogg", "audio/wav", "audio/x-wav"}),
		},
		playerIface: {
			"PlaybackStatus": {Value: "Stopped", Emit: prop.EmitTrue},
			"LoopStatus":     {Value: "None", Writable: true, Emit: prop.EmitTrue},
			"Rate":           constant(1.0),
			"Shuffle":        {Value: false, Writable: true, Emit: prop.EmitTrue},
			"Metadata":       {Value: map[string]dbus.Variant{}, Emit: prop.EmitTrue},
			"Volume":         {Value: 1.0, Writable: true, Emit: prop.EmitTrue},
			"Position":       {Value: int64(0), Emit: prop.EmitFalse},
			"MinimumRate":    constant(1.0),
			"MaximumRate":    constant(1.0),
			"CanGoNext":      {Value: false, Emit: prop.EmitTrue},
			"CanGoPrevious":  {Value: false, Emit: prop.EmitTrue},
			"CanPlay":        {Value: false, Emit: prop.EmitTrue},
			"CanPause":       {Value: false, Emit: prop.EmitTrue},
			"CanSeek":        {Value: false, Emit: prop.EmitTrue},
			"CanControl":     constant(true),
		},
	})
	if err != nil {
		return nil, err
	}
	m.props = props
	if err := conn.Export(mprisProperties{Properties: props, state: m, post: post}, playerPath, "org.freedesktop.DBus.Properties"); err != nil {
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
			{Name: filesIface, Methods: introspect.Methods(fileOpener{})},
			{Name: rootIface, Methods: introspect.Methods(root{}), Properties: props.Introspection(rootIface)},
			{Name: playerIface, Methods: methods, Properties: props.Introspection(playerIface), Signals: []introspect.Signal{{Name: "Seeked", Args: []introspect.Arg{{Name: "Position", Type: "x"}}}}},
		},
	}
	if err := conn.Export(introspect.NewIntrospectable(node), playerPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return nil, err
	}
	reply, err := conn.RequestName(playerName(), dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		return nil, fmt.Errorf("MPRIS name unavailable: %v (%v)", reply, err)
	}
	return m, nil
}

func (m *mpris) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	close(m.done)
	m.mu.Unlock()
	<-m.finished
	err := m.conn.Close()
	if m.artDir != "" {
		err = errors.Join(err, os.RemoveAll(m.artDir))
	}
	return err
}

func (m *mpris) Update(s Snapshot) {
	// SetMust panics when the change signal cannot be sent, e.g. after the
	// session bus went away; media keys must never take the player down.
	defer func() { _ = recover() }()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	previous := m.current
	m.current, m.updatedAt = s, time.Now()
	if previous.Path != s.Path || previous.TrackID != s.TrackID || (previous.Status == "Stopped") != (s.Status == "Stopped") {
		m.clearArtwork()
		select {
		case <-m.requests:
		default:
		}
		if s.Status != "Stopped" && s.Path != "" && !media.IsHTTPStream(s.Path) {
			m.requests <- s
		}
	}
	for name, value := range map[string]any{
		"PlaybackStatus": s.Status,
		"CanSeek":        s.CanSeek && s.Status != "Stopped" && s.Duration > 0,
		"LoopStatus":     loopStatus(s),
		"Shuffle":        s.Shuffle,
		"Volume":         s.Volume,
		"CanPlay":        s.CanPlay,
		"CanPause":       s.CanPause,
		"CanGoNext":      s.CanGoNext,
		"CanGoPrevious":  s.CanGoPrevious,
	} {
		if !reflect.DeepEqual(m.props.GetMust(playerIface, name), value) {
			m.props.SetMust(playerIface, name, value)
		}
	}
	_ = m.publishMetadata()
	if previous.TrackID == s.TrackID && previous.Revision != s.Revision && s.Status != "Stopped" {
		_ = m.conn.Emit(playerPath, playerIface+".Seeked", int64(s.Position)*1_000_000)
	}
}

func loopStatus(s Snapshot) string {
	if s.LoopStatus == "Playlist" || s.LoopStatus == "Track" {
		return s.LoopStatus
	}
	return "None"
}

func (m *mpris) metadata() map[string]dbus.Variant {
	values := metadata(m.current)
	if m.artURL != "" {
		values["mpris:artUrl"] = dbus.MakeVariant(m.artURL)
	}
	return values
}

func (m *mpris) publishMetadata() error {
	values := m.metadata()
	if reflect.DeepEqual(values, m.publishedMetadata) {
		return nil
	}
	if err := m.conn.Emit(playerPath, "org.freedesktop.DBus.Properties.PropertiesChanged", playerIface,
		map[string]dbus.Variant{"Metadata": dbus.MakeVariant(values)}, []string{}); err != nil {
		return err
	}
	m.publishedMetadata = values
	return nil
}

func (m *mpris) clearArtwork() {
	if m.artFile != "" {
		_ = os.Remove(m.artFile)
	}
	m.artFile, m.artURL = "", ""
}

func (m *mpris) loadArtwork() {
	defer close(m.finished)
	for {
		select {
		case <-m.done:
			return
		case s := <-m.requests:
			art, err := media.ReadArtwork(s.Path)
			if err == nil {
				err = m.applyArtwork(s, art)
			}
			if err != nil && m.report != nil {
				m.report(err)
			}
		}
	}
}

func (m *mpris) applyArtwork(s Snapshot, art *media.Artwork) (err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || s.Path != m.current.Path || s.TrackID != m.current.TrackID || m.current.Status == "Stopped" || art == nil {
		return nil
	}
	if m.artDir == "" {
		m.artDir, err = os.MkdirTemp("", "mp3blstr-artwork-")
		if err != nil {
			return err
		}
	}
	file, err := os.CreateTemp(m.artDir, "cover-*.png")
	if err != nil {
		return err
	}
	_, writeErr := file.Write(art.PNG)
	if err = errors.Join(writeErr, file.Close()); err != nil {
		_ = os.Remove(file.Name())
		return err
	}
	m.clearArtwork()
	m.artFile, m.artURL = file.Name(), fileURI(file.Name())
	return m.publishMetadata()
}

func (m *mpris) position() int64 {
	if m.current.Status == "Stopped" {
		return 0
	}
	position := int64(m.current.Position) * 1_000_000
	if m.current.Status == "Playing" {
		position += max(0, time.Since(m.updatedAt).Microseconds())
	}
	if m.current.Duration > 0 {
		position = min(position, int64(m.current.Duration)*1_000_000)
	}
	return max(0, position)
}
