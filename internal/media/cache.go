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

type metadataEntry struct {
	Size     int64   `json:"size"`
	Modified int64   `json:"modified"`
	Profile  string  `json:"profile,omitempty"`
	Tags     *Tags   `json:"tags,omitempty"`
	Name     *string `json:"name,omitempty"`
	Duration *int    `json:"duration,omitempty"`
}

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

func cachePath(path string) string {
	key := sha256.Sum256([]byte(path))
	return filepath.Join(metadata.dir, hex.EncodeToString(key[:])+".json")
}

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

func forgetMetadata(path string) {
	metadata.Lock()
	defer metadata.Unlock()
	delete(metadata.entries, path)
	if metadata.dir != "" {
		_ = os.Remove(cachePath(path))
	}
}
