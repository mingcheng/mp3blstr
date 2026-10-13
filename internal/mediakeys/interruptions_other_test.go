//go:build !darwin && !linux

package mediakeys

import "testing"

func TestInterruptionsUnsupported(t *testing.T) {
	if SupportsInterruptions {
		t.Fatal("macOS interruption monitoring enabled on another platform")
	}
	for _, flags := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
		monitor, err := WatchInterruptions(flags[0], flags[1], func(Interruption) {
			t.Error("unsupported monitor delivered an interruption")
		})
		if err == nil || monitor != nil {
			t.Fatalf("WatchInterruptions%v = %v, %v; want nil monitor and error", flags, monitor, err)
		}
	}
}
