package updater

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetchLatestVersion_Success(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name": "v0.2.0"}`))
	}))
	defer server.Close()

	version, err := fetchFromURL(server.URL)
	require.NoError(t, err)
	require.Equal(t, "v0.2.0", version)
}

func TestFetchLatestVersion_NotFound(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message": "Not Found"}`))
	}))
	defer server.Close()

	_, err := fetchFromURL(server.URL)
	require.Error(t, err)
	require.Contains(t, err.Error(), "404")
}

func TestFetchLatestVersion_RateLimited(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message": "API rate limit exceeded"}`))
	}))
	defer server.Close()

	_, err := fetchFromURL(server.URL)
	require.Error(t, err)
	require.Contains(t, err.Error(), "403")
}

func TestFetchLatestVersion_InvalidJSON(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	_, err := fetchFromURL(server.URL)
	require.Error(t, err)
}

func TestFetchLatestVersion_EmptyTagName(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name": ""}`))
	}))
	defer server.Close()

	version, err := fetchFromURL(server.URL)
	require.NoError(t, err)
	require.Empty(t, version)
}

func TestFetchLatestVersion_ExtraFieldsIgnored(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// GitHub returns many fields — we only care about tag_name.
		_, _ = w.Write([]byte(`{
			"url": "https://api.github.com/repos/ahmedaabouzied/goprove/releases/1",
			"tag_name": "v1.0.0",
			"name": "Release v1.0.0",
			"draft": false,
			"prerelease": false,
			"body": "release notes here"
		}`))
	}))
	defer server.Close()

	version, err := fetchFromURL(server.URL)
	require.NoError(t, err)
	require.Equal(t, "v1.0.0", version)
}

func TestFetchLatestVersion_ServerDown(t *testing.T) {
	t.Parallel()
	// Use a URL that will refuse connection.
	_, err := fetchFromURL("http://127.0.0.1:1")
	require.Error(t, err)
}

func TestFetchLatestVersion_EmptyBody(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Empty body — decoder will fail.
	}))
	defer server.Close()

	_, err := fetchFromURL(server.URL)
	require.Error(t, err)
}

func TestFetchLatestVersion_ServerError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := fetchFromURL(server.URL)
	require.Error(t, err)
	require.Contains(t, err.Error(), "500")
}
