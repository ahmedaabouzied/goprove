package updater

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func IsStale(entry CacheEntry, maxAge time.Duration) bool {
	return time.Since(entry.CheckedAt) > maxAge
}

// ReadCache reads the content of "~/.goprove/latest-version"
// Which is just a JSON file with the latest version fetched
// from Github releases API.
// Each time the goprove command is run, we check against
// the content of this file.
// Once a day when a goprove command is run, a background
// process updates the version file contents so that
// subsequent runs show a notification to the user that
// a new version is available.
func ReadCache() (CacheEntry, error) {
	entry := CacheEntry{}
	h := cacheEntryHandler{}
	home := h.homeDir()
	data := h.readFile(filepath.Join(home, ".goprove", "latest-version"))
	h.unmarshal(data, &entry)
	return entry, h.err
}

func WriteCache(entry CacheEntry) error {
	h := cacheEntryHandler{}
	home := h.homeDir()
	h.mkdirAll(filepath.Join(home, ".goprove"))
	data := h.marshal(entry)
	h.writeFile(filepath.Join(home, ".goprove", "latest-version"), data)
	return h.err
}

type CacheEntry struct {
	LatestVersion string
	CheckedAt     time.Time
}

type cacheEntryHandler struct {
	err error
}

func (h *cacheEntryHandler) homeDir() string {
	if h.err != nil {
		return ""
	}
	var dir string
	dir, h.err = os.UserHomeDir()
	return dir
}

func (h *cacheEntryHandler) marshal(entry CacheEntry) []byte {
	if h.err != nil {
		return []byte{}
	}

	var b []byte
	b, h.err = json.Marshal(entry)
	return b
}

func (h *cacheEntryHandler) mkdirAll(path string) {
	if h.err != nil {
		return
	}
	h.err = os.MkdirAll(path, 0755)
}

func (h *cacheEntryHandler) readFile(name string) []byte {
	if h.err != nil {
		return nil
	}
	var b []byte
	h.err = nil
	b, h.err = os.ReadFile(name)
	return b
}

func (h *cacheEntryHandler) unmarshal(data []byte, v any) {
	if h.err != nil {
		return
	}
	h.err = json.Unmarshal(data, v)
}

func (h *cacheEntryHandler) writeFile(name string, data []byte) {
	if h.err != nil {
		return
	}
	h.err = os.WriteFile(name, data, 0644)
}
