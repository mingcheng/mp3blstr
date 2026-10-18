package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	info := func(v string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Version: v}}
	}
	tests := []struct {
		name    string
		stamped string
		info    *debug.BuildInfo
		want    string
	}{
		{"ldflags wins", "1.0.0", info("v0.9.0"), "1.0.0"},
		{"go install tag", "dev", info("v1.0.0"), "v1.0.0"},
		{"empty stamp", "", info("v1.2.3-rc.1"), "v1.2.3-rc.1"},
		{"no build info", "dev", nil, "dev"},
		{"devel", "dev", info("(devel)"), "dev"},
		{"pseudo-version", "dev", info("v0.0.0-20261018020042-e38be1c53ab5"), "dev"},
		{"pseudo-version after tag", "dev", info("v1.0.1-0.20261018020042-e38be1c53ab5"), "dev"},
		{"dirty tree", "dev", info("v1.0.0+dirty"), "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.stamped, tt.info); got != tt.want {
				t.Errorf("resolveVersion(%q, %v) = %q, want %q", tt.stamped, tt.info, got, tt.want)
			}
		})
	}
}

func TestFormatVersion(t *testing.T) {
	tests := []struct{ version, date, want string }{
		{"dev", "", "dev"},
		{"", "20261018", "dev(20261018)"},
		{"1.0.0", "20261018", "v1.0.0(20261018)"},
		{"v1.0.0", "", "v1.0.0"},
	}
	for _, tt := range tests {
		if got := formatVersion(tt.version, tt.date); got != tt.want {
			t.Errorf("formatVersion(%q, %q) = %q, want %q", tt.version, tt.date, got, tt.want)
		}
	}
}
