//go:build linux

package mediakeys

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/mingcheng/mp3blstr/internal/media"
)

func TestMPRISCommands(t *testing.T) {
	var commands []Command
	p := player{post: func(command Command) { commands = append(commands, command) }}
	for _, tt := range []struct {
		action Action
		call   func() *dbus.Error
	}{
		{PlayPause, p.PlayPause}, {Play, p.Play}, {Pause, p.Pause},
		{Next, p.Next}, {Previous, p.Previous}, {Stop, p.Stop},
	} {
		commands = nil
		if err := tt.call(); err != nil {
			t.Fatal(err)
		}
		if len(commands) != 1 || commands[0] != (Command{Action: tt.action}) {
			t.Fatalf("action %v delivered %v", tt.action, commands)
		}
	}
	commands = nil
	for _, err := range []*dbus.Error{p.SeekMPRIS(10), p.SetPosition(playerPath, 10), p.OpenUri("file:///song.mp3")} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(commands) != 0 {
		t.Fatal("unsupported MPRIS operations dispatched commands")
	}
}

func TestMPRISModeCommands(t *testing.T) {
	var commands []Command
	properties := mprisProperties{post: func(command Command) { commands = append(commands, command) }}
	for _, mode := range []string{"None", "Playlist", "Track"} {
		if err := properties.Set(playerIface, "LoopStatus", dbus.MakeVariant(mode)); err != nil {
			t.Fatal(err)
		}
	}
	for _, enabled := range []bool{true, true, false} {
		if err := properties.Set(playerIface, "Shuffle", dbus.MakeVariant(enabled)); err != nil {
			t.Fatal(err)
		}
	}
	want := []Command{
		{Action: SetLoopStatus, LoopStatus: "None"},
		{Action: SetLoopStatus, LoopStatus: "Playlist"},
		{Action: SetLoopStatus, LoopStatus: "Track"},
		{Action: SetShuffle, Enabled: true},
		{Action: SetShuffle, Enabled: true},
		{Action: SetShuffle, Enabled: false},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
	for _, entry := range []struct {
		name  string
		value any
	}{
		{"LoopStatus", "Invalid"}, {"LoopStatus", true}, {"Shuffle", "true"},
	} {
		if err := properties.Set(playerIface, entry.name, dbus.MakeVariant(entry.value)); err == nil {
			t.Fatalf("accepted invalid %s = %v", entry.name, entry.value)
		}
	}
	if len(commands) != len(want) {
		t.Fatal("invalid values dispatched commands")
	}
}

func TestMPRIS(t *testing.T) {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("requires a session bus")
	}
	events := make(chan Command, 6)
	controls, err := Start(func(command Command) { events <- command }, func(err error) { t.Error(err) })
	if err != nil {
		t.Fatal(err)
	}
	defer controls.Close()
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	path := filepath.Join(t.TempDir(), "song #1.mp3")
	if err := os.WriteFile(path, make([]byte, 256), 0o600); err != nil {
		t.Fatal(err)
	}
	obj := conn.Object(playerName(), playerPath)
	for _, want := range []string{"Playing", "Playing", "Paused", "Stopped"} {
		snapshot := Snapshot{Status: want, CanSeek: true, Position: 12, Duration: 120, Path: path, Title: "Linux metadata", Artist: "Artist", Album: "Album", TrackID: 1}
		controls.Update(snapshot)
		var all map[string]dbus.Variant
		if err := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, playerIface).Store(&all); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(all["Metadata"].Value(), metadata(snapshot)) {
			t.Fatalf("metadata = %v", all["Metadata"])
		}
		position := all["Position"].Value().(int64)
		if want == "Stopped" && position != 0 || want == "Paused" && position != 12_000_000 || want == "Playing" && (position < 12_000_000 || position >= 14_000_000) {
			t.Fatalf("%s position = %d", want, position)
		}
		for name, value := range map[string]any{"PlaybackStatus": want, "CanSeek": want != "Stopped"} {
			var got dbus.Variant
			if err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, playerIface, name).Store(&got); err != nil {
				t.Fatal(err)
			}
			if got.Value() != value {
				t.Fatalf("%s = %v, want %v", name, got.Value(), value)
			}
		}
	}
	for _, cmd := range []struct {
		method string
		action Action
	}{
		{"PlayPause", PlayPause}, {"Play", Play}, {"Pause", Pause},
		{"Next", Next}, {"Previous", Previous}, {"Stop", Stop},
	} {
		if err := obj.Call("org.mpris.MediaPlayer2.Player."+cmd.method, 0).Err; err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-events:
			if got != (Command{Action: cmd.action}) {
				t.Fatalf("%s delivered %v", cmd.method, got)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s did not deliver a command", cmd.method)
		}
	}
	if err := conn.AddMatchSignal(dbus.WithMatchObjectPath(playerPath), dbus.WithMatchSender(playerName()), dbus.WithMatchInterface("org.freedesktop.DBus.Properties")); err != nil {
		t.Fatal(err)
	}
	signals := make(chan *dbus.Signal, 32)
	conn.Signal(signals)
	defer conn.RemoveSignal(signals)
	snapshot := Snapshot{Status: "Stopped"}
	for _, entry := range []struct {
		name    string
		value   any
		command Command
	}{
		{"LoopStatus", "Playlist", Command{Action: SetLoopStatus, LoopStatus: "Playlist"}},
		{"LoopStatus", "Track", Command{Action: SetLoopStatus, LoopStatus: "Track"}},
		{"LoopStatus", "None", Command{Action: SetLoopStatus, LoopStatus: "None"}},
		{"Shuffle", true, Command{Action: SetShuffle, Enabled: true}},
		{"Shuffle", false, Command{Action: SetShuffle, Enabled: false}},
	} {
		if err := obj.Call("org.freedesktop.DBus.Properties.Set", 0, playerIface, entry.name, dbus.MakeVariant(entry.value)).Err; err != nil {
			t.Fatal(err)
		}
		select {
		case command := <-events:
			if command != entry.command {
				t.Fatalf("%s command = %v, want %v", entry.name, command, entry.command)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s did not dispatch", entry.name)
		}
		if entry.name == "LoopStatus" {
			snapshot.LoopStatus = entry.value.(string)
		} else {
			snapshot.Shuffle = entry.value.(bool)
		}
		controls.Update(snapshot)
		var value dbus.Variant
		if err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, playerIface, entry.name).Store(&value); err != nil {
			t.Fatal(err)
		}
		if value.Value() != entry.value {
			t.Fatalf("%s = %v, want %v", entry.name, value.Value(), entry.value)
		}
		select {
		case signal := <-signals:
			changed := signal.Body[1].(map[string]dbus.Variant)
			if value, ok := changed[entry.name]; !ok || value.Value() != entry.value {
				t.Fatalf("%s change signal = %v", entry.name, signal.Body)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s change signal missing", entry.name)
		}
	}
	if err := obj.Call("org.freedesktop.DBus.Properties.Set", 0, playerIface, "LoopStatus", dbus.MakeVariant("Invalid")).Err; err == nil {
		t.Fatal("D-Bus accepted invalid repeat mode")
	}
}

func TestMPRISSeeking(t *testing.T) {
	state := &mpris{current: Snapshot{Status: "Paused", Path: "/song.mp3", TrackID: 7, Position: 12, Duration: 120, CanSeek: true}, updatedAt: time.Now()}
	var commands []Command
	control := player{state: state, post: func(command Command) { commands = append(commands, command) }}
	control.SeekMPRIS(-5_000_000)
	control.SetPosition(trackPath(state.current), 30_500_000)
	want := []Command{{Action: Seek, TrackID: 7, Position: 7}, {Action: Seek, TrackID: 7, Position: 30.5}}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("seeks = %v, want %v", commands, want)
	}
	control.SetPosition("/org/mpris/MediaPlayer2/track/6", 10_000_000)
	control.SetPosition(trackPath(state.current), -1)
	control.SetPosition(trackPath(state.current), 121_000_000)
	state.current.CanSeek = false
	control.SeekMPRIS(1_000_000)
	state.current.CanSeek = true
	state.current.Status = "Stopped"
	control.SeekMPRIS(1_000_000)
	if !reflect.DeepEqual(commands, want) {
		t.Fatal("invalid or stale seek dispatched")
	}
}

func TestMPRISTimelineAndMetadata(t *testing.T) {
	state := &mpris{current: Snapshot{Status: "Playing", Duration: 20, Position: 12}, updatedAt: time.Now().Add(-3 * time.Second)}
	if position := state.position(); position < 15_000_000 || position >= 16_000_000 {
		t.Fatalf("live position = %d", position)
	}
	state.updatedAt = time.Now().Add(-time.Minute)
	if state.position() != 20_000_000 {
		t.Fatal("position exceeds duration")
	}
	stream := Snapshot{Status: "Playing", Path: "https://example.com/live", Title: "Live"}
	if _, ok := metadata(stream)["mpris:length"]; ok {
		t.Fatal("unknown stream duration published")
	}
	path := filepath.Join(t.TempDir(), "\u97f3\u4e50 #1.mp3")
	parsed, err := url.Parse(fileURI(path))
	if err != nil || parsed.Path != path || parsed.Fragment != "" || parsed.Scheme != "file" {
		t.Fatalf("invalid file URI: %v, %v", parsed, err)
	}
}

func TestMPRISArtworkAndSignals(t *testing.T) {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("requires a session bus")
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 8, 4))); err != nil {
		t.Fatal(err)
	}
	picture := append([]byte("\x00image/png\x00\x03\x00"), encoded.Bytes()...)
	frame := make([]byte, 10)
	copy(frame, "APIC")
	binary.BigEndian.PutUint32(frame[4:], uint32(len(picture)))
	frame = append(frame, picture...)
	header := []byte{'I', 'D', '3', 3, 0, 0, 0, 0, 0, 0}
	for index, size := 9, len(frame); index >= 6; index-- {
		header[index], size = byte(size&127), size>>7
	}
	path := filepath.Join(t.TempDir(), "cover song.mp3")
	if err := os.WriteFile(path, append(header, frame...), 0o600); err != nil {
		t.Fatal(err)
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.AddMatchSignal(dbus.WithMatchObjectPath(playerPath), dbus.WithMatchSender(playerName())); err != nil {
		t.Fatal(err)
	}
	signals := make(chan *dbus.Signal, 64)
	conn.Signal(signals)
	defer conn.RemoveSignal(signals)
	errors := make(chan error, 4)
	commands := make(chan Command, 4)
	controls, err := Start(func(command Command) { commands <- command }, func(err error) { errors <- err })
	if err != nil {
		t.Fatal(err)
	}
	defer controls.Close()
	snapshot := Snapshot{Status: "Paused", Path: path, Title: "Cover", TrackID: 9, Position: 12, Duration: 120, CanSeek: true}
	controls.Update(snapshot)
	var artURL string
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for artURL == "" {
		select {
		case signal := <-signals:
			if signal.Name != "org.freedesktop.DBus.Properties.PropertiesChanged" {
				continue
			}
			var iface string
			var changed map[string]dbus.Variant
			var invalidated []string
			if err := dbus.Store(signal.Body, &iface, &changed, &invalidated); err != nil {
				t.Fatal(err)
			}
			if value, ok := changed["Metadata"]; ok {
				values := value.Value().(map[string]dbus.Variant)
				if cover, ok := values["mpris:artUrl"]; ok {
					artURL = cover.Value().(string)
				}
			}
		case err := <-errors:
			t.Fatal(err)
		case <-timer.C:
			t.Fatal("no artwork PropertiesChanged signal")
		}
	}
	parsed, err := url.Parse(artURL)
	if err != nil || parsed.Scheme != "file" {
		t.Fatalf("art URL = %q, %v", artURL, err)
	}
	data, err := os.ReadFile(parsed.Path)
	if err != nil || !bytes.Equal(data, encoded.Bytes()) {
		t.Fatalf("published cover differs: %v", err)
	}
	obj := conn.Object(playerName(), playerPath)
	if err := obj.Call(playerIface+".SetPosition", 0, trackPath(snapshot), int64(30_000_000)).Err; err != nil {
		t.Fatal(err)
	}
	select {
	case command := <-commands:
		if command != (Command{Action: Seek, TrackID: 9, Position: 30}) {
			t.Fatalf("D-Bus seek = %v", command)
		}
	case <-time.After(time.Second):
		t.Fatal("D-Bus seek not delivered")
	}
	snapshot.Position, snapshot.Revision = 30, 1
	controls.Update(snapshot)
	for {
		select {
		case signal := <-signals:
			if signal.Name == playerIface+".Seeked" {
				if len(signal.Body) != 1 || signal.Body[0] != int64(30_000_000) {
					t.Fatalf("Seeked = %v", signal.Body)
				}
				goto seeked
			}
		case <-timer.C:
			t.Fatal("no Seeked signal")
		}
	}
seeked:
	state := controls.(*mpris)
	next := snapshot
	next.TrackID = 10
	next.Path = filepath.Join(t.TempDir(), "no-cover.mp3")
	if err := os.WriteFile(next.Path, make([]byte, 256), 0o600); err != nil {
		t.Fatal(err)
	}
	controls.Update(next)
	if err := state.applyArtwork(snapshot, &media.Artwork{PNG: encoded.Bytes()}); err != nil {
		t.Fatal(err)
	}
	var value dbus.Variant
	if err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, playerIface, "Metadata").Store(&value); err != nil {
		t.Fatal(err)
	}
	if _, ok := value.Value().(map[string]dbus.Variant)["mpris:artUrl"]; ok {
		t.Fatal("previous track cover was restored")
	}
	snapshot.Status = "Stopped"
	controls.Update(snapshot)
	if err := state.applyArtwork(Snapshot{Path: path, TrackID: 9}, &media.Artwork{PNG: encoded.Bytes()}); err != nil {
		t.Fatal(err)
	}
	if err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, playerIface, "Metadata").Store(&value); err != nil {
		t.Fatal(err)
	}
	if _, ok := value.Value().(map[string]dbus.Variant)["mpris:artUrl"]; ok {
		t.Fatal("stale cover was restored")
	}
	if _, err := os.Stat(parsed.Path); !os.IsNotExist(err) {
		t.Fatalf("old artwork not removed: %v", err)
	}
	if err := controls.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(parsed.Path)); !os.IsNotExist(err) {
		t.Fatalf("artwork directory not cleaned: %v", err)
	}
}
