package media

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/mingcheng/mp3blstr/internal/atomicfile"
)

// metadataEntry is what is known about a local file. It is valid while the
// file's size and modification time are unchanged; Tags and Name are only
// valid for the tag decoding Profile they were read with.
type metadataEntry struct {
	Size     int64   `json:"size"`
	Modified int64   `json:"modified"`
	Profile  string  `json:"profile,omitempty"`
	Tags     *Tags   `json:"tags,omitempty"`
	Name     *string `json:"name,omitempty"`
	Duration *int    `json:"duration,omitempty"`
}

// metadata is the in-memory cache, backed by one JSON file per song in dir
// when the disk cache is enabled.
var metadata = struct {
	sync.Mutex
	dir     string
	profile string
	entries map[string]metadataEntry
}{entries: map[string]metadataEntry{}}

// ConfigureMetadataCache enables disk storage in dir; an empty dir keeps
// results in memory only. profile identifies the active tag decoding settings.
func ConfigureMetadataCache(dir, profile string) {
	metadata.Lock()
	defer metadata.Unlock()
	metadata.dir = dir
	metadata.profile = profile
	metadata.entries = map[string]metadataEntry{}
}

// metadataProfile returns the active tag decoding profile.
func metadataProfile() string {
	metadata.Lock()
	defer metadata.Unlock()
	return metadata.profile
}

// clearMetadataMemory drops the in-memory entries, e.g. after the tag
// decoding changed. Disk entries are checked against the profile instead.
func clearMetadataMemory() {
	metadata.Lock()
	metadata.entries = map[string]metadataEntry{}
	metadata.Unlock()
}

// cachePath returns the disk cache file of path. The caller holds the lock.
func cachePath(path string) string {
	key := sha256.Sum256([]byte(path))
	return filepath.Join(metadata.dir, hex.EncodeToString(key[:])+".json")
}

// cachedEntry returns the entry of path from memory or disk, provided the
// file has not changed since it was stored.
func cachedEntry(path string) (metadataEntry, bool) {
	stat, err := os.Stat(path)
	if err != nil {
		return metadataEntry{}, false
	}
	metadata.Lock()
	defer metadata.Unlock()
	entry, ok := metadata.entries[path]
	if !ok && metadata.dir != "" {
		data, err := os.ReadFile(cachePath(path))
		if err == nil && json.Unmarshal(data, &entry) == nil {
			ok = true
		}
	}
	if !ok || entry.Size != stat.Size() || entry.Modified != stat.ModTime().UnixNano() {
		return metadataEntry{}, false
	}
	metadata.entries[path] = entry
	return entry, true
}

// storeEntry applies update to the entry of path (a fresh one when the file
// changed) and stores it. update runs with the cache locked.
func storeEntry(path string, update func(*metadataEntry)) {
	stat, err := os.Stat(path)
	if err != nil {
		return
	}
	metadata.Lock()
	defer metadata.Unlock()
	entry := metadata.entries[path]
	if entry.Size != stat.Size() || entry.Modified != stat.ModTime().UnixNano() {
		entry = metadataEntry{Size: stat.Size(), Modified: stat.ModTime().UnixNano()}
	}
	update(&entry)
	metadata.entries[path] = entry
	if metadata.dir != "" {
		if err := os.MkdirAll(metadata.dir, 0o700); err == nil {
			if data, err := json.Marshal(entry); err == nil {
				_ = atomicfile.Write(cachePath(path), data, 0o600)
			}
		}
	}
}

// CachedDuration reads a local file's length at most once per cache lifetime.
func CachedDuration(path string, read func(string) (int, error)) (int, error) {
	if entry, ok := cachedEntry(path); ok && entry.Duration != nil {
		return *entry.Duration, nil
	}
	seconds, err := read(path)
	if err == nil {
		storeEntry(path, func(entry *metadataEntry) { entry.Duration = &seconds })
	}
	return seconds, err
}

// forgetMetadata drops the entry of path from memory and disk.
func forgetMetadata(path string) {
	metadata.Lock()
	defer metadata.Unlock()
	delete(metadata.entries, path)
	if metadata.dir != "" {
		_ = os.Remove(cachePath(path))
	}
}
