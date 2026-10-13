// Package filemanager implements the directory browser used in mp3blasterr's
// file selection mode.
package filemanager

import (
	"fmt"
	"strings"
)

// SortMode determines how files (not directories) are ordered.
type SortMode int

// Sort modes in the order the original cycles through them.
const (
	SortAlpha SortMode = iota
	SortAlphaCase
	SortModifyNew
	SortModifyOld
	SortSizeSmall
	SortSizeBig
	SortNone
)

var sortInfo = []struct{ name, desc string }{
	{"alpha", "Sort alphabetically, case-insensitive"},
	{"alpha-case", "Sort alphabetically, case-sensitive"},
	{"modify-new", "Sort by modification date, newest first"},
	{"modify-old", "Sort by modification date, oldest first"},
	{"size-small", "Sort by file size, smallest first"},
	{"size-big", "Sort by file size, biggest first"},
	{"none", "Don't sort"},
}

// Next returns the following sort mode.
func (s SortMode) Next() SortMode { return (s + 1) % SortMode(len(sortInfo)) }

// Description returns the human readable description.
func (s SortMode) Description() string { return sortInfo[s].desc }

// String returns the configuration name.
func (s SortMode) String() string { return sortInfo[s].name }

// ParseSortMode parses a File.SortMode configuration value.
func ParseSortMode(name string) (SortMode, error) {
	for i, si := range sortInfo {
		if strings.EqualFold(si.name, name) {
			return SortMode(i), nil
		}
	}
	return SortAlpha, fmt.Errorf("invalid sort mode %q", name)
}
