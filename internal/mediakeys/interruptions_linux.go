//go:build linux

package mediakeys

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const SupportsInterruptions = true

const (
	loginService   = "org.freedesktop.login1"
	loginPath      = dbus.ObjectPath("/org/freedesktop/login1")
	loginInterface = "org.freedesktop.login1.Manager"
)

type sleepMonitor struct {
	conn           *dbus.Conn
	signals        chan *dbus.Signal
	done, finished chan struct{}
	once           sync.Once
	closeErr       error
	inhibitor      *os.File
}

func WatchInterruptions(sleep, _ bool, post func(Interruption)) (io.Closer, error) {
	if !sleep {
		return io.NopCloser(strings.NewReader("")), nil
	}
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	monitor, err := watchSleep(conn, loginService, post)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return monitor, nil
}

func sleepInhibitor(conn *dbus.Conn, service string) (*os.File, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var descriptor dbus.UnixFD
	err := conn.Object(service, loginPath).CallWithContext(ctx, loginInterface+".Inhibit", 0,
		"sleep", "mp3blstr", "Pause audio before system sleep", "delay").Store(&descriptor)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(descriptor), "mp3blstr-sleep-inhibitor"), nil
}

func watchSleep(conn *dbus.Conn, service string, post func(Interruption)) (*sleepMonitor, error) {
	monitor := &sleepMonitor{conn: conn, signals: make(chan *dbus.Signal, 8), done: make(chan struct{}), finished: make(chan struct{})}
	conn.Signal(monitor.signals)
	if err := conn.AddMatchSignal(dbus.WithMatchSender(service), dbus.WithMatchObjectPath(loginPath),
		dbus.WithMatchInterface(loginInterface), dbus.WithMatchMember("PrepareForSleep")); err != nil {
		conn.RemoveSignal(monitor.signals)
		return nil, err
	}
	var err error
	monitor.inhibitor, err = sleepInhibitor(conn, service)
	if err != nil {
		conn.RemoveSignal(monitor.signals)
		return nil, err
	}
	go monitor.watch(service, post)
	return monitor, nil
}

func (m *sleepMonitor) release() {
	if m.inhibitor != nil {
		_ = m.inhibitor.Close()
		m.inhibitor = nil
	}
}

func (m *sleepMonitor) watch(service string, post func(Interruption)) {
	defer close(m.finished)
	defer m.conn.RemoveSignal(m.signals)
	defer m.release()
	sleeping := false
	for {
		select {
		case <-m.done:
			return
		case signal, ok := <-m.signals:
			if !ok {
				return
			}
			if signal == nil || signal.Name != loginInterface+".PrepareForSleep" || len(signal.Body) != 1 {
				continue
			}
			preparing, ok := signal.Body[0].(bool)
			if !ok || preparing == sleeping {
				continue
			}
			sleeping = preparing
			if !preparing {
				// Re-arm the delay lock, but never resume music on wake.
				m.inhibitor, _ = sleepInhibitor(m.conn, service)
				continue
			}
			ack := make(chan struct{})
			go func() { post(SystemSleep); close(ack) }()
			// Wait for the UI to pause before releasing sleep; shutdown must
			// also unblock this wait when the UI event loop has already ended.
			select {
			case <-ack:
			case <-m.done:
				return
			}
			m.release()
		}
	}
}

func (m *sleepMonitor) Close() error {
	m.once.Do(func() {
		close(m.done)
		m.closeErr = m.conn.Close()
		<-m.finished
	})
	return m.closeErr
}
