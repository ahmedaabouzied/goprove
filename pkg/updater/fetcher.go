package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// FetchLatestVersion fetches the latest release tag from GitHub.
func FetchLatestVersion() (string, error) {
	return fetchFromURL(githubReleasesURL)
}

const githubReleasesURL = "https://api.github.com/repos/ahmedaabouzied/goprove/releases/latest"

func fetchFromURL(url string) (string, error) {
	client := http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Github API returned status %d", res.StatusCode)
	}

	release := githubRelease{}
	decoder := json.NewDecoder(res.Body)
	if err := decoder.Decode(&release); err != nil {
		return "", err
	}

	return release.TagName, nil
}

type githubRelease struct {
	TagName string `json:"tag_name"`
}
