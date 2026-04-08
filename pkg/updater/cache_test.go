package updater

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ===========================================================================
// CacheEntry JSON round-trip tests
// ===========================================================================
func TestCacheEntry_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	original := CacheEntry{
		LatestVersion: "v0.2.1",
		CheckedAt:     time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored CacheEntry
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	require.Equal(t, original.LatestVersion, restored.LatestVersion)
	require.True(t, original.CheckedAt.Equal(restored.CheckedAt),
		"timestamps should survive JSON round-trip")
}

func TestHandler_ErrorChaining(t *testing.T) {
	t.Parallel()
	// Once an error occurs, all subsequent operations are skipped.
	h := cacheEntryHandler{}
	_ = h.readFile("/nonexistent/path")
	require.Error(t, h.err, "readFile on nonexistent path should set error")

	// Subsequent operations should be no-ops.
	data := h.marshal(CacheEntry{LatestVersion: "v1.0.0"})
	require.Empty(t, data, "marshal should be skipped after prior error")

	var entry CacheEntry
	h.unmarshal([]byte(`{"LatestVersion":"v2.0.0"}`), &entry)
	require.Empty(t, entry.LatestVersion, "unmarshal should be skipped after prior error")
}

func TestHandler_MarshalSkipsOnPriorError(t *testing.T) {
	t.Parallel()
	h := cacheEntryHandler{err: os.ErrNotExist}
	data := h.marshal(CacheEntry{LatestVersion: "v1.0.0"})
	require.Empty(t, data, "marshal should return empty bytes when prior error exists")
	require.ErrorIs(t, h.err, os.ErrNotExist, "original error should be preserved")
}

// ===========================================================================
// cacheEntryHandler tests
// ===========================================================================
func TestHandler_MarshalUnmarshal(t *testing.T) {
	t.Parallel()
	entry := CacheEntry{
		LatestVersion: "v1.0.0",
		CheckedAt:     time.Now().Truncate(time.Second),
	}

	h := cacheEntryHandler{}
	data := h.marshal(entry)
	require.NoError(t, h.err)
	require.NotEmpty(t, data)

	var restored CacheEntry
	h2 := cacheEntryHandler{}
	h2.unmarshal(data, &restored)
	require.NoError(t, h2.err)
	require.Equal(t, entry.LatestVersion, restored.LatestVersion)
}

func TestHandler_ReadFileNonexistent(t *testing.T) {
	t.Parallel()
	h := cacheEntryHandler{}
	data := h.readFile("/nonexistent/path/to/file")
	require.Nil(t, data)
	require.Error(t, h.err)
}

func TestHandler_ReadFileSkipsOnPriorError(t *testing.T) {
	t.Parallel()
	h := cacheEntryHandler{err: os.ErrPermission}
	data := h.readFile("/some/path")
	require.Nil(t, data)
	require.ErrorIs(t, h.err, os.ErrPermission, "original error should be preserved")
}

func TestHandler_UnmarshalInvalidJSON(t *testing.T) {
	t.Parallel()
	h := cacheEntryHandler{}
	var entry CacheEntry
	h.unmarshal([]byte(`not json`), &entry)
	require.Error(t, h.err, "invalid JSON should produce an error")
}

func TestHandler_UnmarshalSkipsOnPriorError(t *testing.T) {
	t.Parallel()
	h := cacheEntryHandler{err: os.ErrNotExist}
	var entry CacheEntry
	h.unmarshal([]byte(`{"LatestVersion":"v1.0.0"}`), &entry)
	require.Empty(t, entry.LatestVersion, "unmarshal should not execute with prior error")
	require.ErrorIs(t, h.err, os.ErrNotExist)
}

func TestHandler_WriteFileSkipsOnPriorError(t *testing.T) {
	t.Parallel()
	h := cacheEntryHandler{err: os.ErrPermission}
	h.writeFile("/some/path", []byte("data"))
	require.ErrorIs(t, h.err, os.ErrPermission, "original error should be preserved")
}

func TestIsStale_EmptyVersion(t *testing.T) {
	t.Parallel()
	entry := CacheEntry{
		LatestVersion: "",
		CheckedAt:     time.Now(),
	}
	// IsStale only checks time, not version content.
	require.False(t, IsStale(entry, 24*time.Hour),
		"IsStale should not care about version content")
}

func TestIsStale_ExactBoundary(t *testing.T) {
	t.Parallel()
	// At exactly maxAge, time.Since > maxAge depends on nanosecond timing.
	// Use a value just past the boundary.
	entry := CacheEntry{
		LatestVersion: "v0.1.0",
		CheckedAt:     time.Now().Add(-24*time.Hour - time.Second),
	}
	require.True(t, IsStale(entry, 24*time.Hour),
		"entry just past max age should be stale")
}

// ===========================================================================
// IsStale tests
// ===========================================================================
func TestIsStale_FreshEntry(t *testing.T) {
	t.Parallel()
	entry := CacheEntry{
		LatestVersion: "v0.1.0",
		CheckedAt:     time.Now(),
	}
	require.False(t, IsStale(entry, 24*time.Hour),
		"entry checked just now should not be stale")
}

func TestIsStale_OldEntry(t *testing.T) {
	t.Parallel()
	entry := CacheEntry{
		LatestVersion: "v0.1.0",
		CheckedAt:     time.Now().Add(-25 * time.Hour),
	}
	require.True(t, IsStale(entry, 24*time.Hour),
		"entry checked 25h ago should be stale with 24h max age")
}

// ===========================================================================
// IsStale + ReadCache integration
// ===========================================================================
func TestIsStale_WithReadCache(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Write a fresh cache entry.
	fresh := CacheEntry{
		LatestVersion: "v0.1.0",
		CheckedAt:     time.Now(),
	}
	require.NoError(t, WriteCache(fresh))

	got, err := ReadCache()
	require.NoError(t, err)
	require.False(t, IsStale(got, 24*time.Hour),
		"freshly written cache should not be stale")

	// Overwrite with a stale cache entry.
	stale := CacheEntry{
		LatestVersion: "v0.1.0",
		CheckedAt:     time.Now().Add(-48 * time.Hour),
	}
	require.NoError(t, WriteCache(stale))

	got, err = ReadCache()
	require.NoError(t, err)
	require.True(t, IsStale(got, 24*time.Hour),
		"cache written 48h ago should be stale")
}

func TestIsStale_ZeroDuration(t *testing.T) {
	t.Parallel()
	entry := CacheEntry{
		LatestVersion: "v0.1.0",
		// Use a past time to make it deterministic.
		CheckedAt: time.Now().Add(-time.Millisecond),
	}
	require.True(t, IsStale(entry, 0),
		"any past entry should be stale with zero max age")
}

func TestIsStale_ZeroTime(t *testing.T) {
	t.Parallel()
	entry := CacheEntry{
		LatestVersion: "v0.1.0",
		CheckedAt:     time.Time{}, // zero value
	}
	require.True(t, IsStale(entry, 24*time.Hour),
		"entry with zero time should always be stale")
}

func TestReadCache_CorruptedFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cacheDir := filepath.Join(tmpHome, ".goprove")
	require.NoError(t, os.MkdirAll(cacheDir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(cacheDir, "latest-version"),
		[]byte("not valid json"),
		0644,
	))

	_, err := ReadCache()
	require.Error(t, err, "corrupted cache file should return an error")
}

func TestReadCache_EmptyFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cacheDir := filepath.Join(tmpHome, ".goprove")
	require.NoError(t, os.MkdirAll(cacheDir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(cacheDir, "latest-version"),
		[]byte(""),
		0644,
	))

	_, err := ReadCache()
	require.Error(t, err, "empty cache file should return an error")
}

// ===========================================================================
// ReadCache / WriteCache integration tests
//
// These tests use t.Setenv to override HOME, so they cannot use t.Parallel().
// ===========================================================================
func TestReadCache_NoFileExists(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	entry, err := ReadCache()
	require.Error(t, err, "should error when cache file doesn't exist")
	require.Empty(t, entry.LatestVersion)
}

func TestReadCache_ValidCacheFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cacheDir := filepath.Join(tmpHome, ".goprove")
	require.NoError(t, os.MkdirAll(cacheDir, 0755))

	now := time.Now().Truncate(time.Second)
	entry := CacheEntry{
		LatestVersion: "v0.3.0",
		CheckedAt:     now,
	}
	data, err := json.Marshal(entry)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "latest-version"), data, 0644))

	got, err := ReadCache()
	require.NoError(t, err)
	require.Equal(t, "v0.3.0", got.LatestVersion)
	require.True(t, now.Equal(got.CheckedAt))
}

// ===========================================================================
// ReadCache + WriteCache round-trip
// ===========================================================================
func TestWriteAndReadCache_RoundTrip(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	original := CacheEntry{
		LatestVersion: "v1.2.3",
		CheckedAt:     time.Now().Truncate(time.Second),
	}

	require.NoError(t, WriteCache(original))

	got, err := ReadCache()
	require.NoError(t, err)
	require.Equal(t, original.LatestVersion, got.LatestVersion)
	require.True(t, original.CheckedAt.Equal(got.CheckedAt),
		"timestamp should survive write+read round-trip")
}

func TestWriteCache_CreatesFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// WriteCache should create ~/.goprove/ if it doesn't exist.
	entry := CacheEntry{
		LatestVersion: "v0.4.0",
		CheckedAt:     time.Now().Truncate(time.Second),
	}
	err := WriteCache(entry)
	require.NoError(t, err)

	// Verify the file was written correctly.
	cacheDir := filepath.Join(tmpHome, ".goprove")
	data, err := os.ReadFile(filepath.Join(cacheDir, "latest-version"))
	require.NoError(t, err)

	var got CacheEntry
	require.NoError(t, json.Unmarshal(data, &got))
	require.Equal(t, "v0.4.0", got.LatestVersion)
	require.True(t, entry.CheckedAt.Equal(got.CheckedAt))
}

func TestWriteCache_NoDirExists(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Don't create ~/.goprove/ — WriteCache should create it.
	entry := CacheEntry{
		LatestVersion: "v0.5.0",
		CheckedAt:     time.Now().Truncate(time.Second),
	}
	require.NoError(t, WriteCache(entry))

	// Verify directory was created and file is readable.
	got, err := ReadCache()
	require.NoError(t, err)
	require.Equal(t, "v0.5.0", got.LatestVersion)
}

func TestWriteCache_OverwritesExistingFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Write first version.
	entry1 := CacheEntry{
		LatestVersion: "v0.1.0",
		CheckedAt:     time.Now().Add(-24 * time.Hour).Truncate(time.Second),
	}
	require.NoError(t, WriteCache(entry1))

	// Write second version — should overwrite.
	entry2 := CacheEntry{
		LatestVersion: "v0.2.0",
		CheckedAt:     time.Now().Truncate(time.Second),
	}
	require.NoError(t, WriteCache(entry2))

	got, err := ReadCache()
	require.NoError(t, err)
	require.Equal(t, "v0.2.0", got.LatestVersion,
		"second write should overwrite the first")
}
