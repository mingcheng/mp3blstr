//go:build linux

package mediakeys

import (
	"os"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestMPRIS(t *testing.T) {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("requires a session bus")
	}
	events := make(chan Action, 6)
	controls, err := Start(func(action Action) { events <- action })
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
	controls.Update("Playing")
	var status dbus.Variant
	if err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, "org.mpris.MediaPlayer2.Player", "PlaybackStatus").Store(&status); err != nil {
		t.Fatal(err)
	}
	if got := status.Value(); got != "Playing" {
		t.Fatalf("playback status: %v", got)
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
		if got := <-events; got != cmd.action {
			t.Fatalf("%s delivered %v", cmd.method, got)
		}
	}
}
