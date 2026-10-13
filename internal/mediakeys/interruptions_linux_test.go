//go:build linux

package mediakeys

import (
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

type fakeLoginManager struct {
	file  *os.File
	calls chan []string
}

func (f fakeLoginManager) Inhibit(what, who, why, mode string) (dbus.UnixFD, *dbus.Error) {
	f.calls <- []string{what, who, why, mode}
	return dbus.UnixFD(f.file.Fd()), nil
}

func testSleepMonitor(t *testing.T, post func(Interruption)) (*sleepMonitor, *dbus.Conn, <-chan []string) {
	t.Helper()
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("requires a session bus")
	}
	file, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	server, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	service := fmt.Sprintf("org.mp3blstr.test.logind.instance%d", os.Getpid())
	if reply, err := server.RequestName(service, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("test logind name = %v, %v", reply, err)
	}
	calls := make(chan []string, 8)
	if err := server.Export(fakeLoginManager{file: file, calls: calls}, loginPath, loginInterface); err != nil {
		t.Fatal(err)
	}
	client, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	monitor, err := watchSleep(client, service, post)
	if err != nil {
		client.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { monitor.Close() })
	return monitor, server, calls
}

func waitInhibit(t *testing.T, calls <-chan []string) {
	t.Helper()
	select {
	case call := <-calls:
		want := []string{"sleep", "mp3blstr", "Pause audio before system sleep", "delay"}
		if !reflect.DeepEqual(call, want) {
			t.Fatalf("Inhibit = %v, want %v", call, want)
		}
	case <-time.After(time.Second):
		t.Fatal("sleep delay lock not acquired")
	}
}

func prepareSleep(t *testing.T, conn *dbus.Conn, preparing bool) {
	t.Helper()
	if err := conn.Emit(loginPath, loginInterface+".PrepareForSleep", preparing); err != nil {
		t.Fatal(err)
	}
}

func TestLinuxSleepLifecycle(t *testing.T) {
	events := make(chan Interruption, 4)
	monitor, server, calls := testSleepMonitor(t, func(event Interruption) { events <- event })
	waitInhibit(t, calls)
	prepareSleep(t, server, true)
	select {
	case event := <-events:
		if event != SystemSleep {
			t.Fatalf("sleep event = %v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("sleep did not pause playback")
	}
	prepareSleep(t, server, true)
	prepareSleep(t, server, false)
	waitInhibit(t, calls)
	select {
	case event := <-events:
		t.Fatalf("duplicate sleep or wake dispatched %v", event)
	default:
	}
	prepareSleep(t, server, true)
	select {
	case <-events:
	case <-time.After(time.Second):
		t.Fatal("second sleep did not pause playback")
	}
	if err := monitor.Close(); err != nil {
		t.Fatal(err)
	}
	if monitor.inhibitor != nil {
		t.Fatal("sleep lock retained after shutdown")
	}
	if err := monitor.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLinuxSleepCloseDuringUIWait(t *testing.T) {
	started, release, callbackDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	monitor, server, calls := testSleepMonitor(t, func(Interruption) {
		close(started)
		<-release
		close(callbackDone)
	})
	waitInhibit(t, calls)
	prepareSleep(t, server, true)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("UI callback not invoked")
	}
	closed := make(chan error, 1)
	go func() { closed <- monitor.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("closing sleep monitor blocked on UI callback")
	}
	close(release)
	<-callbackDone
	if monitor.inhibitor != nil {
		t.Fatal("shutdown retained sleep delay lock")
	}
}

func TestLinuxOutputOnlyMonitor(t *testing.T) {
	monitor, err := WatchInterruptions(false, true, func(Interruption) { t.Error("unexpected interruption") })
	if err != nil || monitor == nil {
		t.Fatalf("disabled sleep monitor = %v, %v", monitor, err)
	}
	if err := monitor.Close(); err != nil {
		t.Fatal(err)
	}
}
