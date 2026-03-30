package analysis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// SummaryCache holds precomputed NilFunctionSummary
// entries keyed by fully-qualified function name
// (e.g., "bytes.NewBuffer",
// "(*net/http.Request).Context").
// It can be serialized to disk so
// that expensive interprocedural analysis of stdlib
// and dependencies
// is only done once per Go version.
type SummaryCache struct {
	GoVersion      string                `json:"go_version"`
	GoproveVersion string                `json:"goprove_version,omitempty"`
	Summaries      map[string][]NilState `json:"summaries"`
}

// NewSummaryCache creates an empty cache tagged with
// the current Go version.
func NewSummaryCache() *SummaryCache {
	return &SummaryCache{
		GoVersion: runtime.Version(),
		Summaries: make(map[string][]NilState),
	}
}

// SetGoproveVersion tags the cache with the goprove version
// that generated it.
func (c *SummaryCache) SetGoproveVersion(v string) {
	c.GoproveVersion = v
}

// Merge copies entries from other into c. Existing entries
// in c are not overwritten.
func (c *SummaryCache) Merge(other *SummaryCache) {
	for k, v := range other.Summaries {
		if _, exists := c.Summaries[k]; !exists {
			c.Summaries[k] = v
		}
	}
}

// Set stores a function summary in the cache.
func (c *SummaryCache) Set(funcName string, returns []NilState) {
	c.Summaries[funcName] = returns
}

// Get retrieves a function summary from the cache.
// Returns the summary and true if found, or nil and
// false if not.
func (c *SummaryCache) Get(funcName string) (NilFunctionSummary, bool) {
	returns, ok := c.Summaries[funcName]
	if !ok {
		return NilFunctionSummary{}, false
	}
	return NilFunctionSummary{Returns: returns}, true
}

// Len returns the number of entries in the cache.
func (c *SummaryCache) Len() int {
	return len(c.Summaries)
}

// Save writes the cache to the given file path as JSON.
// Creates parent directories if they don't exist.
func (c *SummaryCache) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling cache: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing cache file: %w", err)
	}

	return nil
}

// LoadSummaryCache reads a cache from disk. Returns
// an error if the file
// cannot be read or parsed. Returns an error if the
// cached Go version
// does not match the current runtime version
// (the cache is stale).
func LoadSummaryCache(path string) (*SummaryCache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading cache file: %w", err)
	}

	var cache SummaryCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("parsing cache file: %w", err)
	}

	if cache.Summaries == nil {
		cache.Summaries = make(map[string][]NilState)
	}

	if cache.GoVersion != runtime.Version() {
		return nil, fmt.Errorf(
			"cache Go version mismatch: cached %s, running %s",
			cache.GoVersion, runtime.Version(),
		)
	}

	return &cache, nil
}

// LoadAndValidateCache reads a cache from disk and
// validates both Go version and goprove version.
// The goprove version check is skipped when either
// the cached or running version is empty (backward
// compatibility with caches that predate versioning).
func LoadAndValidateCache(path, goproveVersion string) (*SummaryCache, error) {
	cache, err := LoadSummaryCache(path)
	if err != nil {
		return nil, err
	}
	if cache.GoproveVersion != "" && goproveVersion != "" &&
		cache.GoproveVersion != goproveVersion {
		return nil, fmt.Errorf(
			"cache goprove version mismatch: cached %s, running %s",
			cache.GoproveVersion, goproveVersion,
		)
	}
	return cache, nil
}

// DefaultCachePath returns the default location for
// the summary cache file:
// ~/.cache/goprove/summaries-<goversion>-<goproveversion>.json
// When goproveVersion is empty, only the Go version is
// included in the filename.
func DefaultCachePath(goproveVersion string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("getting user cache directory: %w", err)
	}
	var filename string
	if goproveVersion != "" {
		filename = fmt.Sprintf("summaries-%s-%s.json", runtime.Version(), goproveVersion)
	} else {
		filename = fmt.Sprintf("summaries-%s.json", runtime.Version())
	}
	return filepath.Join(cacheDir, "goprove", filename), nil
}
