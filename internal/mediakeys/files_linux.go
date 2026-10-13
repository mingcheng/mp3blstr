//go:build linux

package mediakeys

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// ForwardFiles is opt-in; normal terminal launches keep their own instance.
func ForwardFiles(paths []string, enqueue bool) (bool, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false, nil
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var names []string
	if err := conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return false, err
	}
	var candidates []string
	for _, name := range names {
		if strings.HasPrefix(name, "org.mpris.MediaPlayer2.mp3blstr.instance") && name != playerName() {
			candidates = append(candidates, name)
		}
	}
	return forwardFiles(ctx, conn, candidates, paths, enqueue)
}

func forwardFiles(ctx context.Context, conn *dbus.Conn, names, paths []string, enqueue bool) (bool, error) {
	type instance struct {
		name string
		rank int
	}
	var instances []instance
	for _, name := range names {
		var status dbus.Variant
		if err := conn.Object(name, playerPath).CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, playerIface, "PlaybackStatus").Store(&status); err != nil {
			continue
		}
		rank := 2
		if status.Value() == "Playing" {
			rank = 0
		} else if status.Value() == "Paused" {
			rank = 1
		}
		instances = append(instances, instance{name, rank})
	}
	sort.Slice(instances, func(left, right int) bool {
		if instances[left].rank != instances[right].rank {
			return instances[left].rank < instances[right].rank
		}
		return instances[left].name < instances[right].name
	})
	for _, instance := range instances {
		err := conn.Object(instance.name, playerPath).CallWithContext(ctx, filesIface+".Open", 0, paths, enqueue).Err
		if err == nil {
			return true, nil
		}
		var busError dbus.Error
		if errors.As(err, &busError) && (busError.Name == "org.freedesktop.DBus.Error.UnknownMethod" || busError.Name == "org.freedesktop.DBus.Error.UnknownInterface" || busError.Name == "org.freedesktop.DBus.Error.ServiceUnknown" || busError.Name == "org.freedesktop.DBus.Error.NameHasNoOwner") {
			continue
		}
		// Do not retry a rejected batch against another player or replay it.
		return false, err
	}
	return false, nil
}
