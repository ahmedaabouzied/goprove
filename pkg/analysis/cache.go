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
	GoVersion string                `json:"go_version"`
	Summaries map[string][]NilState `json:"summaries"`
}

// NewSummaryCache creates an empty cache tagged with
// the current Go version.
func NewSummaryCache() *SummaryCache {
	return &SummaryCache{
		GoVersion: runtime.Version(),
		Summaries: make(map[string][]NilState),
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

// DefaultCachePath returns the default location for
// the summary cache file:
// ~/.cache/goprove/summaries-<goversion>.json
func DefaultCachePath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("getting user cache directory: %w", err)
	}
	filename := fmt.Sprintf("summaries-%s.json", runtime.Version())
	return filepath.Join(cacheDir, "goprove", filename), nil
}
