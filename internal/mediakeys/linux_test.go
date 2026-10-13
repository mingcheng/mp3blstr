//go:build linux

package mediakeys

import (
	"os"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
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
	obj := conn.Object(playerName(), playerPath)
	for _, want := range []string{"Playing", "Playing", "Paused", "Stopped"} {
		controls.Update(Snapshot{Status: want, CanSeek: true, Position: 12, Title: "macOS metadata"})
		for name, value := range map[string]any{"PlaybackStatus": want, "CanSeek": false, "Position": int64(0)} {
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
}
